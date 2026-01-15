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
func (e *Engine) applyEscrowCreate(tx *EscrowCreate, account *AccountRoot, metadata *Metadata) Result {
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
	exists, _ := e.view.Exists(destKey)
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

	// Insert escrow
	if err := e.view.Insert(escrowKey, escrowData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	account.OwnerCount++

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "Escrow",
		LedgerIndex:     hex.EncodeToString(escrowKey.Key[:]),
		NewFields: map[string]any{
			"Account":     tx.Account,
			"Destination": tx.Destination,
			"Amount":      tx.Amount.Value,
		},
	})

	return TesSUCCESS
}

// applyEscrowFinish applies an EscrowFinish transaction
func (e *Engine) applyEscrowFinish(tx *EscrowFinish, account *AccountRoot, metadata *Metadata) Result {
	// Get the escrow owner's account ID
	ownerID, err := decodeAccountID(tx.Owner)
	if err != nil {
		return TemINVALID
	}

	// Find the escrow
	escrowKey := keylet.Escrow(ownerID, tx.OfferSequence)
	escrowData, err := e.view.Read(escrowKey)
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

	// Check condition/fulfillment (simplified - in reality, would verify crypto-condition)
	if escrow.Condition != "" {
		if tx.Fulfillment == "" {
			return TecCRYPTOCONDITION_ERROR
		}
		// Would verify: fulfillment matches condition
	}

	// Get destination account
	destKey := keylet.Account(escrow.DestinationID)
	destData, err := e.view.Read(destKey)
	if err != nil {
		return TecNO_DST
	}

	destAccount, err := parseAccountRoot(destData)
	if err != nil {
		return TefINTERNAL
	}

	// Transfer the escrowed amount to destination
	destAccount.Balance += escrow.Amount

	// Update destination
	destUpdatedData, err := serializeAccountRoot(destAccount)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(destKey, destUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Delete the escrow
	if err := e.view.Erase(escrowKey); err != nil {
		return TefINTERNAL
	}

	// Decrease owner count for escrow owner
	if tx.Owner != tx.Account {
		// Need to update owner's account too
		ownerKey := keylet.Account(ownerID)
		ownerData, err := e.view.Read(ownerKey)
		if err == nil {
			ownerAccount, err := parseAccountRoot(ownerData)
			if err == nil && ownerAccount.OwnerCount > 0 {
				ownerAccount.OwnerCount--
				ownerUpdatedData, err := serializeAccountRoot(ownerAccount)
				if err == nil {
					e.view.Update(ownerKey, ownerUpdatedData)
				}
			}
		}
	} else {
		if account.OwnerCount > 0 {
			account.OwnerCount--
		}
	}

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Escrow",
		LedgerIndex:     hex.EncodeToString(escrowKey.Key[:]),
	})

	destAddr, _ := encodeAccountID(escrow.DestinationID)
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(destKey.Key[:]),
		FinalFields: map[string]any{
			"Account": destAddr,
			"Balance": strconv.FormatUint(destAccount.Balance, 10),
		},
	})

	return TesSUCCESS
}

// applyEscrowCancel applies an EscrowCancel transaction
func (e *Engine) applyEscrowCancel(tx *EscrowCancel, account *AccountRoot, metadata *Metadata) Result {
	// Get the escrow owner's account ID
	ownerID, err := decodeAccountID(tx.Owner)
	if err != nil {
		return TemINVALID
	}

	// Find the escrow
	escrowKey := keylet.Escrow(ownerID, tx.OfferSequence)
	escrowData, err := e.view.Read(escrowKey)
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
	ownerData, err := e.view.Read(ownerKey)
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

	if err := e.view.Update(ownerKey, ownerUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Delete the escrow
	if err := e.view.Erase(escrowKey); err != nil {
		return TefINTERNAL
	}

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Escrow",
		LedgerIndex:     hex.EncodeToString(escrowKey.Key[:]),
	})

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(ownerKey.Key[:]),
		FinalFields: map[string]any{
			"Account": tx.Owner,
			"Balance": strconv.FormatUint(ownerAccount.Balance, 10),
		},
	})

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
