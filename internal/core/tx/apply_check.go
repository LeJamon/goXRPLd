package tx

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// CheckData represents a Check ledger entry
type CheckData struct {
	Account        [20]byte
	DestinationID  [20]byte
	SendMax        uint64 // For XRP checks; IOU checks would need more fields
	Sequence       uint32
	Expiration     uint32
	InvoiceID      [32]byte
	DestinationTag uint32
	HasDestTag     bool
}

// applyCheckCreate applies a CheckCreate transaction
func (e *Engine) applyCheckCreate(tx *CheckCreate, account *AccountRoot, metadata *Metadata) Result {
	// Verify destination exists
	destID, err := decodeAccountID(tx.Destination)
	if err != nil {
		return TemINVALID
	}

	destKey := keylet.Account(destID)
	exists, _ := e.view.Exists(destKey)
	if !exists {
		return TecNO_DST
	}

	// Parse SendMax - only XRP supported for now
	sendMax, err := strconv.ParseUint(tx.SendMax.Value, 10, 64)
	if err != nil {
		// May be an IOU amount
		sendMax = 0
	}

	// Check balance for XRP checks
	if tx.SendMax.Currency == "" && sendMax > 0 {
		if account.Balance < sendMax {
			return TecUNFUNDED
		}
	}

	// Create the check entry
	accountID, _ := decodeAccountID(tx.Account)
	sequence := *tx.GetCommon().Sequence

	checkKey := keylet.Check(accountID, sequence)

	// Serialize check
	checkData, err := serializeCheck(tx, accountID, destID, sequence, sendMax)
	if err != nil {
		return TefINTERNAL
	}

	// Insert check
	if err := e.view.Insert(checkKey, checkData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	account.OwnerCount++

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "Check",
		LedgerIndex:     hex.EncodeToString(checkKey.Key[:]),
		NewFields: map[string]any{
			"Account":     tx.Account,
			"Destination": tx.Destination,
			"SendMax":     tx.SendMax.Value,
		},
	})

	return TesSUCCESS
}

// applyCheckCash applies a CheckCash transaction
func (e *Engine) applyCheckCash(tx *CheckCash, account *AccountRoot, metadata *Metadata) Result {
	// Parse check ID
	checkID, err := hex.DecodeString(tx.CheckID)
	if err != nil || len(checkID) != 32 {
		return TemINVALID
	}

	var checkKeyBytes [32]byte
	copy(checkKeyBytes[:], checkID)
	checkKey := keylet.Keylet{Key: checkKeyBytes}

	// Read check
	checkData, err := e.view.Read(checkKey)
	if err != nil {
		return TecNO_ENTRY
	}

	// Parse check
	check, err := parseCheck(checkData)
	if err != nil {
		return TefINTERNAL
	}

	// Verify the account is the destination
	accountID, _ := decodeAccountID(tx.Account)
	if check.DestinationID != accountID {
		return TecNO_PERMISSION
	}

	// Check expiration
	if check.Expiration > 0 {
		// In full implementation, check against close time
		// For standalone mode, we'll allow it
	}

	// Determine amount to cash
	var cashAmount uint64
	if tx.Amount != nil {
		// Exact amount
		cashAmount, err = strconv.ParseUint(tx.Amount.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}
		if cashAmount > check.SendMax {
			return TecPATH_PARTIAL
		}
	} else if tx.DeliverMin != nil {
		// Minimum amount - use full SendMax for simplicity
		deliverMin, err := strconv.ParseUint(tx.DeliverMin.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}
		if check.SendMax < deliverMin {
			return TecPATH_PARTIAL
		}
		cashAmount = check.SendMax
	}

	// Get the check creator's account
	creatorKey := keylet.Account(check.Account)
	creatorData, err := e.view.Read(creatorKey)
	if err != nil {
		return TefINTERNAL
	}

	creatorAccount, err := parseAccountRoot(creatorData)
	if err != nil {
		return TefINTERNAL
	}

	// Check if creator has sufficient balance
	if creatorAccount.Balance < cashAmount {
		return TecUNFUNDED_PAYMENT
	}

	// Transfer the funds
	creatorAccount.Balance -= cashAmount
	account.Balance += cashAmount

	// Decrease creator's owner count
	if creatorAccount.OwnerCount > 0 {
		creatorAccount.OwnerCount--
	}

	// Update creator account
	creatorUpdatedData, err := serializeAccountRoot(creatorAccount)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(creatorKey, creatorUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Delete the check
	if err := e.view.Erase(checkKey); err != nil {
		return TefINTERNAL
	}

	// Record in metadata
	creatorAddr, _ := encodeAccountID(check.Account)
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Check",
		LedgerIndex:     hex.EncodeToString(checkKey.Key[:]),
	})

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(creatorKey.Key[:]),
		FinalFields: map[string]any{
			"Account": creatorAddr,
			"Balance": strconv.FormatUint(creatorAccount.Balance, 10),
		},
	})

	return TesSUCCESS
}

// applyCheckCancel applies a CheckCancel transaction
func (e *Engine) applyCheckCancel(tx *CheckCancel, account *AccountRoot, metadata *Metadata) Result {
	// Parse check ID
	checkID, err := hex.DecodeString(tx.CheckID)
	if err != nil || len(checkID) != 32 {
		return TemINVALID
	}

	var checkKeyBytes [32]byte
	copy(checkKeyBytes[:], checkID)
	checkKey := keylet.Keylet{Key: checkKeyBytes}

	// Read check
	checkData, err := e.view.Read(checkKey)
	if err != nil {
		return TecNO_ENTRY
	}

	// Parse check
	check, err := parseCheck(checkData)
	if err != nil {
		return TefINTERNAL
	}

	accountID, _ := decodeAccountID(tx.Account)
	isCreator := check.Account == accountID
	isDestination := check.DestinationID == accountID

	// Only creator or destination can cancel
	if !isCreator && !isDestination {
		// Unless the check is expired
		if check.Expiration == 0 {
			return TecNO_PERMISSION
		}
		// In full implementation, check if expired
		// For standalone mode, allow anyone to cancel expired checks
	}

	// Delete the check
	if err := e.view.Erase(checkKey); err != nil {
		return TefINTERNAL
	}

	// If the canceller is also the creator, decrease their owner count
	if isCreator {
		if account.OwnerCount > 0 {
			account.OwnerCount--
		}
	} else {
		// Need to update the creator's owner count
		creatorKey := keylet.Account(check.Account)
		creatorData, err := e.view.Read(creatorKey)
		if err == nil {
			creatorAccount, err := parseAccountRoot(creatorData)
			if err == nil && creatorAccount.OwnerCount > 0 {
				creatorAccount.OwnerCount--
				creatorUpdatedData, _ := serializeAccountRoot(creatorAccount)
				e.view.Update(creatorKey, creatorUpdatedData)
			}
		}
	}

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Check",
		LedgerIndex:     hex.EncodeToString(checkKey.Key[:]),
	})

	return TesSUCCESS
}

// serializeCheck serializes a Check ledger entry
func serializeCheck(tx *CheckCreate, ownerID, destID [20]byte, sequence uint32, sendMax uint64) ([]byte, error) {
	ownerAddress, err := addresscodec.EncodeAccountIDToClassicAddress(ownerID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode owner address: %w", err)
	}

	destAddress, err := addresscodec.EncodeAccountIDToClassicAddress(destID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode destination address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "Check",
		"Account":         ownerAddress,
		"Destination":     destAddress,
		"SendMax":         fmt.Sprintf("%d", sendMax),
		"Sequence":        sequence,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	if tx.Expiration != nil {
		jsonObj["Expiration"] = *tx.Expiration
	}

	if tx.DestinationTag != nil {
		jsonObj["DestinationTag"] = *tx.DestinationTag
	}

	if tx.InvoiceID != "" {
		jsonObj["InvoiceID"] = tx.InvoiceID
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode Check: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// parseCheck parses a Check ledger entry from binary data
func parseCheck(data []byte) (*CheckData, error) {
	check := &CheckData{}
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
				return check, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return check, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case fieldCodeSequence:
				check.Sequence = value
			case 10: // Expiration
				check.Expiration = value
			case 14: // DestinationTag
				check.DestinationTag = value
				check.HasDestTag = true
			}

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return check, nil
			}
			offset += 8

		case fieldTypeHash256:
			if offset+32 > len(data) {
				return check, nil
			}
			if fieldCode == 17 { // InvoiceID
				copy(check.InvoiceID[:], data[offset:offset+32])
			}
			offset += 32

		case fieldTypeAmount:
			if offset+8 > len(data) {
				return check, nil
			}
			if data[offset]&0x80 == 0 {
				// XRP amount
				rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
				if fieldCode == 9 { // SendMax
					check.SendMax = rawAmount & 0x3FFFFFFFFFFFFFFF
				}
				offset += 8
			} else {
				// IOU amount - skip 48 bytes
				offset += 48
			}

		case fieldTypeAccountID:
			if offset+21 > len(data) {
				return check, nil
			}
			length := data[offset]
			offset++
			if length == 20 {
				switch fieldCode {
				case 1: // Account
					copy(check.Account[:], data[offset:offset+20])
				case 3: // Destination
					copy(check.DestinationID[:], data[offset:offset+20])
				}
				offset += 20
			}

		case fieldTypeBlob:
			if offset >= len(data) {
				return check, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return check, nil
			}
			offset += length

		default:
			return check, nil
		}
	}

	return check, nil
}
