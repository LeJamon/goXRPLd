package tx

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// Crypto-condition types
// Reference: https://tools.ietf.org/html/draft-thomas-crypto-conditions-02
const (
	// conditionTypePreimageSha256 is the type for PREIMAGE-SHA-256 conditions
	conditionTypePreimageSha256 = 0

	// maxPreimageLength is the maximum allowed length for a preimage
	// Reference: rippled PreimageSha256.h maxPreimageLength
	maxPreimageLength = 128

	// maxSerializedCondition is the maximum allowed size of a serialized condition
	maxSerializedCondition = 128

	// maxSerializedFulfillment is the maximum allowed size of a serialized fulfillment
	maxSerializedFulfillment = 256
)

// validateCryptoCondition verifies that a fulfillment matches its condition
// Reference: rippled Escrow.cpp checkCondition()
//
// Condition format (PREIMAGE-SHA-256):
//   - 0xA0: Type prefix (constructed, context-specific tag 0)
//   - length: VarInt length of condition body
//   - 0x80, 0x20: Fingerprint tag (primitive, context-specific tag 0) + length (32)
//   - <32 bytes>: SHA-256 hash of the preimage (the fingerprint)
//   - 0x81, 0x01: Cost tag (primitive, context-specific tag 1) + length (1)
//   - <cost>: The cost value (preimage length)
//
// Fulfillment format (PREIMAGE-SHA-256):
//   - 0xA0: Type prefix (constructed, context-specific tag 0)
//   - length: VarInt length of fulfillment body
//   - 0x80: Preimage tag (primitive, context-specific tag 0)
//   - length: Preimage length
//   - <preimage>: The actual preimage bytes
func validateCryptoCondition(fulfillmentHex, conditionHex string) error {
	fulfillment, err := hex.DecodeString(fulfillmentHex)
	if err != nil {
		return errors.New("invalid fulfillment encoding")
	}

	condition, err := hex.DecodeString(conditionHex)
	if err != nil {
		return errors.New("invalid condition encoding")
	}

	return checkCondition(fulfillment, condition)
}

// checkCondition verifies that a fulfillment matches the condition
// Reference: rippled Escrow.cpp checkCondition(Slice f, Slice c)
func checkCondition(fulfillment, condition []byte) error {
	// Validate sizes
	if len(condition) > maxSerializedCondition {
		return errors.New("condition too large")
	}
	if len(fulfillment) > maxSerializedFulfillment {
		return errors.New("fulfillment too large")
	}

	// Parse condition to extract fingerprint
	fingerprint, condType, err := parseCondition(condition)
	if err != nil {
		return fmt.Errorf("failed to parse condition: %w", err)
	}

	// Only PREIMAGE-SHA-256 is supported
	if condType != conditionTypePreimageSha256 {
		return errors.New("unsupported condition type")
	}

	// Parse fulfillment to extract preimage
	preimage, fulfType, err := parseFulfillment(fulfillment)
	if err != nil {
		return fmt.Errorf("failed to parse fulfillment: %w", err)
	}

	// Types must match
	if condType != fulfType {
		return errors.New("condition and fulfillment type mismatch")
	}

	// For PREIMAGE-SHA-256: fingerprint = SHA-256(preimage)
	// Compute SHA-256 of preimage and compare to fingerprint
	hash := sha256.Sum256(preimage)
	if len(fingerprint) != 32 {
		return errors.New("invalid fingerprint length")
	}

	for i := 0; i < 32; i++ {
		if hash[i] != fingerprint[i] {
			return errors.New("fulfillment does not match condition")
		}
	}

	return nil
}

// parseCondition parses a crypto-condition and extracts the fingerprint and type
// Reference: rippled Condition.h/cpp deserialize
func parseCondition(data []byte) (fingerprint []byte, condType uint8, err error) {
	if len(data) < 4 {
		return nil, 0, errors.New("condition too short")
	}

	offset := 0

	// Check type tag (0xA0 + type for constructed, context-specific)
	tag := data[offset]
	offset++

	// Extract condition type from tag
	// 0xA0 = 1010 0000 = constructed (0x20) + context-specific (0x80) + tag 0
	// Type is encoded in the low 5 bits after the class bits
	if (tag & 0xE0) != 0xA0 {
		return nil, 0, errors.New("invalid condition tag")
	}
	condType = tag & 0x1F

	// Parse length
	length, bytesRead, err := parseASN1Length(data[offset:])
	if err != nil {
		return nil, 0, err
	}
	offset += bytesRead

	if offset+length > len(data) {
		return nil, 0, errors.New("condition length exceeds data")
	}

	// Parse fingerprint (tag 0x80)
	if offset >= len(data) || data[offset] != 0x80 {
		return nil, 0, errors.New("expected fingerprint tag")
	}
	offset++

	// Parse fingerprint length
	fpLength, bytesRead, err := parseASN1Length(data[offset:])
	if err != nil {
		return nil, 0, err
	}
	offset += bytesRead

	if fpLength != 32 {
		return nil, 0, errors.New("invalid fingerprint length for PREIMAGE-SHA-256")
	}

	if offset+fpLength > len(data) {
		return nil, 0, errors.New("fingerprint exceeds condition data")
	}

	fingerprint = make([]byte, fpLength)
	copy(fingerprint, data[offset:offset+fpLength])

	return fingerprint, condType, nil
}

// parseFulfillment parses a crypto-fulfillment and extracts the preimage and type
// Reference: rippled Fulfillment.h/cpp deserialize, PreimageSha256.h deserialize
func parseFulfillment(data []byte) (preimage []byte, fulfType uint8, err error) {
	if len(data) < 4 {
		return nil, 0, errors.New("fulfillment too short")
	}

	offset := 0

	// Check type tag (0xA0 + type for constructed, context-specific)
	tag := data[offset]
	offset++

	// Extract fulfillment type from tag
	if (tag & 0xE0) != 0xA0 {
		return nil, 0, errors.New("invalid fulfillment tag")
	}
	fulfType = tag & 0x1F

	// Parse length
	_, bytesRead, err := parseASN1Length(data[offset:])
	if err != nil {
		return nil, 0, err
	}
	offset += bytesRead

	// For PREIMAGE-SHA-256, next is the preimage (tag 0x80)
	if fulfType == conditionTypePreimageSha256 {
		if offset >= len(data) || data[offset] != 0x80 {
			return nil, 0, errors.New("expected preimage tag")
		}
		offset++

		// Parse preimage length
		preimageLength, bytesRead, err := parseASN1Length(data[offset:])
		if err != nil {
			return nil, 0, err
		}
		offset += bytesRead

		if preimageLength > maxPreimageLength {
			return nil, 0, errors.New("preimage too long")
		}

		if offset+preimageLength > len(data) {
			return nil, 0, errors.New("preimage exceeds fulfillment data")
		}

		preimage = make([]byte, preimageLength)
		copy(preimage, data[offset:offset+preimageLength])

		return preimage, fulfType, nil
	}

	return nil, 0, errors.New("unsupported fulfillment type")
}

// parseASN1Length parses a DER-encoded length
// Returns the length value and the number of bytes consumed
func parseASN1Length(data []byte) (int, int, error) {
	if len(data) < 1 {
		return 0, 0, errors.New("no length byte")
	}

	first := data[0]
	if first < 0x80 {
		// Short form: length is directly in the first byte
		return int(first), 1, nil
	}

	// Long form: first byte indicates number of length bytes
	numBytes := int(first & 0x7F)
	if numBytes == 0 {
		return 0, 0, errors.New("indefinite length not supported")
	}
	if numBytes > 4 {
		return 0, 0, errors.New("length too large")
	}
	if len(data) < 1+numBytes {
		return 0, 0, errors.New("insufficient length bytes")
	}

	length := 0
	for i := 0; i < numBytes; i++ {
		length = (length << 8) | int(data[1+i])
	}

	return length, 1 + numBytes, nil
}

// EscrowData represents an Escrow ledger entry
type EscrowData struct {
	Account       [20]byte
	DestinationID [20]byte
	Amount        uint64
	Condition     string
	CancelAfter   uint32
	FinishAfter   uint32
}

// applyEscrowCreate applies an EscrowCreate transaction
func (e *Engine) applyEscrowCreate(tx *EscrowCreate, account *AccountRoot, view LedgerView) Result {
	// Parse the amount to escrow
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err != nil {
		return TemINVALID
	}

	// Check that account has sufficient balance (after fee)
	if account.Balance < amount {
		return TecUNFUNDED
	}

	// Verify destination exists
	destID, err := decodeAccountID(tx.Destination)
	if err != nil {
		return TemINVALID
	}

	destKey := keylet.Account(destID)
	exists, _ := view.Exists(destKey)
	if !exists {
		return TecNO_DST
	}

	// Deduct the escrow amount from the account
	account.Balance -= amount

	// Create the escrow entry
	accountID, _ := decodeAccountID(tx.Account)
	sequence := *tx.GetCommon().Sequence // Use the transaction sequence

	escrowKey := keylet.Escrow(accountID, sequence)

	// Serialize escrow
	escrowData, err := serializeEscrow(tx, accountID, destID, sequence, amount)
	if err != nil {
		return TefINTERNAL
	}

	// Insert escrow - creation tracked automatically by ApplyStateTable
	if err := view.Insert(escrowKey, escrowData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	account.OwnerCount++

	return TesSUCCESS
}

// applyEscrowFinish applies an EscrowFinish transaction
func (e *Engine) applyEscrowFinish(tx *EscrowFinish, account *AccountRoot, view LedgerView) Result {
	// Get the escrow owner's account ID
	ownerID, err := decodeAccountID(tx.Owner)
	if err != nil {
		return TemINVALID
	}

	// Find the escrow
	escrowKey := keylet.Escrow(ownerID, tx.OfferSequence)
	escrowData, err := view.Read(escrowKey)
	if err != nil {
		return TecNO_TARGET
	}

	// Parse escrow
	escrow, err := parseEscrow(escrowData)
	if err != nil {
		return TefINTERNAL
	}

	// Check FinishAfter time (if set)
	if escrow.FinishAfter > 0 {
		// In a full implementation, we'd check against the close time
		// For now, we'll allow it in standalone mode
		if !e.config.Standalone {
			// Would check: if currentTime < escrow.FinishAfter return TecNO_PERMISSION
		}
	}

	// Check condition/fulfillment with proper crypto-condition verification
	// Reference: rippled Escrow.cpp preclaim() and checkCondition()
	if escrow.Condition != "" {
		// If escrow has a condition, fulfillment must be provided
		if tx.Fulfillment == "" {
			return TecCRYPTOCONDITION_ERROR
		}

		// Verify the fulfillment matches the condition
		// The escrow stores condition as hex, tx provides fulfillment as hex
		if err := validateCryptoCondition(tx.Fulfillment, escrow.Condition); err != nil {
			return TecCRYPTOCONDITION_ERROR
		}
	}

	// Get destination account
	destKey := keylet.Account(escrow.DestinationID)
	destData, err := view.Read(destKey)
	if err != nil {
		return TecNO_DST
	}

	destAccount, err := parseAccountRoot(destData)
	if err != nil {
		return TefINTERNAL
	}

	// Transfer the escrowed amount to destination
	destAccount.Balance += escrow.Amount

	// Update destination - modification tracked automatically by ApplyStateTable
	destUpdatedData, err := serializeAccountRoot(destAccount)
	if err != nil {
		return TefINTERNAL
	}

	if err := view.Update(destKey, destUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Delete the escrow - deletion tracked automatically by ApplyStateTable
	if err := view.Erase(escrowKey); err != nil {
		return TefINTERNAL
	}

	// Decrease owner count for escrow owner
	if tx.Owner != tx.Account {
		// Need to update owner's account too
		ownerKey := keylet.Account(ownerID)
		ownerData, err := view.Read(ownerKey)
		if err == nil {
			ownerAccount, err := parseAccountRoot(ownerData)
			if err == nil && ownerAccount.OwnerCount > 0 {
				ownerAccount.OwnerCount--
				ownerUpdatedData, err := serializeAccountRoot(ownerAccount)
				if err == nil {
					view.Update(ownerKey, ownerUpdatedData)
				}
			}
		}
	} else {
		if account.OwnerCount > 0 {
			account.OwnerCount--
		}
	}

	return TesSUCCESS
}

// applyEscrowCancel applies an EscrowCancel transaction
func (e *Engine) applyEscrowCancel(tx *EscrowCancel, account *AccountRoot, view LedgerView) Result {
	// Get the escrow owner's account ID
	ownerID, err := decodeAccountID(tx.Owner)
	if err != nil {
		return TemINVALID
	}

	// Find the escrow
	escrowKey := keylet.Escrow(ownerID, tx.OfferSequence)
	escrowData, err := view.Read(escrowKey)
	if err != nil {
		return TecNO_TARGET
	}

	// Parse escrow
	escrow, err := parseEscrow(escrowData)
	if err != nil {
		return TefINTERNAL
	}

	// Check CancelAfter time (if set)
	if escrow.CancelAfter > 0 {
		// In a full implementation, we'd check against the close time
		// For now, we'll allow it in standalone mode
		if !e.config.Standalone {
			// Would check: if currentTime < escrow.CancelAfter return TecNO_PERMISSION
		}
	} else {
		// If no CancelAfter, only the creator can cancel (implied by having condition)
		if tx.Account != tx.Owner {
			return TecNO_PERMISSION
		}
	}

	// Return the escrowed amount to the owner
	ownerKey := keylet.Account(ownerID)
	ownerData, err := view.Read(ownerKey)
	if err != nil {
		return TefINTERNAL
	}

	ownerAccount, err := parseAccountRoot(ownerData)
	if err != nil {
		return TefINTERNAL
	}

	ownerAccount.Balance += escrow.Amount
	if ownerAccount.OwnerCount > 0 {
		ownerAccount.OwnerCount--
	}

	ownerUpdatedData, err := serializeAccountRoot(ownerAccount)
	if err != nil {
		return TefINTERNAL
	}

	// Update owner - modification tracked automatically by ApplyStateTable
	if err := view.Update(ownerKey, ownerUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Delete the escrow - deletion tracked automatically by ApplyStateTable
	if err := view.Erase(escrowKey); err != nil {
		return TefINTERNAL
	}

	return TesSUCCESS
}

// serializeEscrow serializes an Escrow ledger entry
func serializeEscrow(tx *EscrowCreate, ownerID, destID [20]byte, sequence uint32, amount uint64) ([]byte, error) {
	ownerAddress, err := addresscodec.EncodeAccountIDToClassicAddress(ownerID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode owner address: %w", err)
	}

	destAddress, err := addresscodec.EncodeAccountIDToClassicAddress(destID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode destination address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "Escrow",
		"Account":         ownerAddress,
		"Destination":     destAddress,
		"Amount":          fmt.Sprintf("%d", amount),
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	if tx.FinishAfter != nil {
		jsonObj["FinishAfter"] = *tx.FinishAfter
	}

	if tx.CancelAfter != nil {
		jsonObj["CancelAfter"] = *tx.CancelAfter
	}

	if tx.Condition != "" {
		jsonObj["Condition"] = tx.Condition
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode Escrow: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// parseEscrow parses an Escrow ledger entry from binary data
func parseEscrow(data []byte) (*EscrowData, error) {
	escrow := &EscrowData{}
	offset := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		header := data[offset]
		offset++

		typeCode := (header >> 4) & 0x0F
		fieldCode := header & 0x0F

		if typeCode == 0 {
			if offset >= len(data) {
				break
			}
			typeCode = data[offset]
			offset++
		}

		if fieldCode == 0 {
			if offset >= len(data) {
				break
			}
			fieldCode = data[offset]
			offset++
		}

		switch typeCode {
		case fieldTypeUInt16:
			if offset+2 > len(data) {
				return escrow, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return escrow, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case 36: // FinishAfter
				escrow.FinishAfter = value
			case 37: // CancelAfter
				escrow.CancelAfter = value
			}

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return escrow, nil
			}
			offset += 8

		case fieldTypeAmount:
			if offset+8 > len(data) {
				return escrow, nil
			}
			rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
			escrow.Amount = rawAmount & 0x3FFFFFFFFFFFFFFF
			offset += 8

		case fieldTypeAccountID:
			if offset+21 > len(data) {
				return escrow, nil
			}
			length := data[offset]
			offset++
			if length == 20 {
				switch fieldCode {
				case 1: // Account
					copy(escrow.Account[:], data[offset:offset+20])
				case 3: // Destination
					copy(escrow.DestinationID[:], data[offset:offset+20])
				}
				offset += 20
			}

		case fieldTypeHash256:
			// Hash256 fields are 32 bytes (e.g., PreviousTxnID)
			if offset+32 > len(data) {
				return escrow, nil
			}
			offset += 32

		case fieldTypeBlob:
			if offset >= len(data) {
				return escrow, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return escrow, nil
			}
			if fieldCode == 25 { // Condition
				escrow.Condition = hex.EncodeToString(data[offset : offset+length])
			}
			offset += length

		default:
			// Unknown type - try to skip safely
			return escrow, nil
		}
	}

	return escrow, nil
}
