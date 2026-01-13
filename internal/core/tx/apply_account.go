package tx

import (
	"encoding/hex"
	"fmt"
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// applySetRegularKey applies a SetRegularKey transaction
func (e *Engine) applySetRegularKey(tx *SetRegularKey, account *AccountRoot, metadata *Metadata) Result {
	// Update the account's regular key
	previousRegularKey := account.RegularKey
	account.RegularKey = tx.RegularKey

	// If setting a new key, validate it exists (or just validate format)
	if tx.RegularKey != "" {
		if _, err := decodeAccountID(tx.RegularKey); err != nil {
			return TemINVALID
		}
	}

	// Record the change in metadata
	if previousRegularKey != account.RegularKey {
		// This will be recorded in the main account update
	}

	return TesSUCCESS
}

// applySignerListSet applies a SignerListSet transaction
func (e *Engine) applySignerListSet(tx *SignerListSet, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Create the SignerList keylet
	signerListKey := keylet.SignerList(accountID)

	if tx.SignerQuorum == 0 {
		// Delete existing signer list
		exists, _ := e.view.Exists(signerListKey)
		if exists {
			if err := e.view.Erase(signerListKey); err != nil {
				return TefINTERNAL
			}

			// Decrease owner count
			if account.OwnerCount > 0 {
				account.OwnerCount--
			}

			// Record deletion in metadata
			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "DeletedNode",
				LedgerEntryType: "SignerList",
				LedgerIndex:     hex.EncodeToString(signerListKey.Key[:]),
			})
		}
	} else {
		// Create or update signer list
		signerListData, err := serializeSignerList(tx, accountID)
		if err != nil {
			return TefINTERNAL
		}

		exists, _ := e.view.Exists(signerListKey)
		if exists {
			// Update existing
			if err := e.view.Update(signerListKey, signerListData); err != nil {
				return TefINTERNAL
			}
			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "ModifiedNode",
				LedgerEntryType: "SignerList",
				LedgerIndex:     hex.EncodeToString(signerListKey.Key[:]),
			})
		} else {
			// Create new
			if err := e.view.Insert(signerListKey, signerListData); err != nil {
				return TefINTERNAL
			}

			// Increase owner count
			account.OwnerCount++

			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "CreatedNode",
				LedgerEntryType: "SignerList",
				LedgerIndex:     hex.EncodeToString(signerListKey.Key[:]),
			})
		}
	}

	return TesSUCCESS
}

// applyTicketCreate applies a TicketCreate transaction
func (e *Engine) applyTicketCreate(tx *TicketCreate, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Create tickets
	for i := uint32(0); i < tx.TicketCount; i++ {
		ticketSeq := account.Sequence + i

		// Create ticket keylet
		ticketKey := keylet.Ticket(accountID, ticketSeq)

		// Serialize ticket
		ticketData, err := serializeTicket(accountID, ticketSeq)
		if err != nil {
			return TefINTERNAL
		}

		// Insert ticket
		if err := e.view.Insert(ticketKey, ticketData); err != nil {
			return TefINTERNAL
		}

		// Record creation in metadata
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "Ticket",
			LedgerIndex:     hex.EncodeToString(ticketKey.Key[:]),
			NewFields: map[string]any{
				"Account":        tx.Account,
				"TicketSequence": ticketSeq,
			},
		})
	}

	// Increase owner count for each ticket
	account.OwnerCount += tx.TicketCount

	// Increase sequence by ticket count (tickets consume sequence numbers)
	account.Sequence += tx.TicketCount

	return TesSUCCESS
}

// applyDepositPreauth applies a DepositPreauth transaction
func (e *Engine) applyDepositPreauth(tx *DepositPreauth, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	if tx.Authorize != "" {
		// Create preauthorization
		authorizedID, err := decodeAccountID(tx.Authorize)
		if err != nil {
			return TemINVALID
		}

		// Check that authorized account exists
		authorizedKey := keylet.Account(authorizedID)
		exists, _ := e.view.Exists(authorizedKey)
		if !exists {
			return TecNO_TARGET
		}

		// Create deposit preauth keylet
		preauthKey := keylet.DepositPreauth(accountID, authorizedID)

		// Check if already exists
		exists, _ = e.view.Exists(preauthKey)
		if exists {
			return TecDUPLICATE
		}

		// Serialize and insert
		preauthData, err := serializeDepositPreauth(accountID, authorizedID)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Insert(preauthKey, preauthData); err != nil {
			return TefINTERNAL
		}

		// Increase owner count
		account.OwnerCount++

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "DepositPreauth",
			LedgerIndex:     hex.EncodeToString(preauthKey.Key[:]),
			NewFields: map[string]any{
				"Account":   tx.Account,
				"Authorize": tx.Authorize,
			},
		})
	} else if tx.Unauthorize != "" {
		// Remove preauthorization
		unauthorizedID, err := decodeAccountID(tx.Unauthorize)
		if err != nil {
			return TemINVALID
		}

		preauthKey := keylet.DepositPreauth(accountID, unauthorizedID)

		// Check if exists
		exists, _ := e.view.Exists(preauthKey)
		if !exists {
			return TecNO_ENTRY
		}

		// Delete
		if err := e.view.Erase(preauthKey); err != nil {
			return TefINTERNAL
		}

		// Decrease owner count
		if account.OwnerCount > 0 {
			account.OwnerCount--
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "DepositPreauth",
			LedgerIndex:     hex.EncodeToString(preauthKey.Key[:]),
		})
	}

	return TesSUCCESS
}

// applyAccountDelete applies an AccountDelete transaction
func (e *Engine) applyAccountDelete(tx *AccountDelete, account *AccountRoot, metadata *Metadata) Result {
	// Check that owner count is 0 (no objects owned)
	if account.OwnerCount > 0 {
		return TecHAS_OBLIGATIONS
	}

	// Check minimum sequence requirement (account must have been around for a while)
	// In standalone mode, we relax this requirement
	if !e.config.Standalone && account.Sequence < 256 {
		return TefTOO_BIG // Account too young
	}

	// Get destination account
	destID, err := decodeAccountID(tx.Destination)
	if err != nil {
		return TemINVALID
	}

	destKey := keylet.Account(destID)
	destData, err := e.view.Read(destKey)
	if err != nil {
		return TecNO_DST
	}

	destAccount, err := parseAccountRoot(destData)
	if err != nil {
		return TefINTERNAL
	}

	// Calculate remaining balance (after fee was deducted)
	remainingBalance := account.Balance

	// Transfer remaining balance to destination
	destAccount.Balance += remainingBalance

	// Update destination account
	destUpdatedData, err := serializeAccountRoot(destAccount)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(destKey, destUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Delete source account
	srcID, _ := decodeAccountID(tx.Account)
	srcKey := keylet.Account(srcID)

	if err := e.view.Erase(srcKey); err != nil {
		return TefINTERNAL
	}

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(srcKey.Key[:]),
		FinalFields: map[string]any{
			"Account": tx.Account,
			"Balance": "0",
		},
	})

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(destKey.Key[:]),
		FinalFields: map[string]any{
			"Account": tx.Destination,
			"Balance": strconv.FormatUint(destAccount.Balance, 10),
		},
	})

	// Set account balance to 0 so the main update doesn't try to write it
	account.Balance = 0

	return TesSUCCESS
}

// serializeSignerList serializes a SignerList ledger entry
func serializeSignerList(tx *SignerListSet, ownerID [20]byte) ([]byte, error) {
	// Convert owner ID to classic address
	ownerAddress, err := addresscodec.EncodeAccountIDToClassicAddress(ownerID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode owner address: %w", err)
	}

	// Build the JSON representation for the binary codec
	jsonObj := map[string]any{
		"LedgerEntryType": "SignerList",
		"Account":         ownerAddress,
		"SignerQuorum":    tx.SignerQuorum,
		"OwnerNode":       "0", // UInt64 as string
	}

	// Add SignerEntries if present
	if len(tx.SignerEntries) > 0 {
		signerEntries := make([]map[string]any, len(tx.SignerEntries))
		for i, entry := range tx.SignerEntries {
			signerEntries[i] = map[string]any{
				"SignerEntry": map[string]any{
					"Account":      entry.SignerEntry.Account,
					"SignerWeight": entry.SignerEntry.SignerWeight,
				},
			}
		}
		jsonObj["SignerEntries"] = signerEntries
	}

	// Encode using the binary codec
	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode SignerList: %w", err)
	}

	// Convert hex string to bytes
	return hex.DecodeString(hexStr)
}

// serializeTicket serializes a Ticket ledger entry
func serializeTicket(ownerID [20]byte, ticketSeq uint32) ([]byte, error) {
	ownerAddress, err := addresscodec.EncodeAccountIDToClassicAddress(ownerID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode owner address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "Ticket",
		"Account":         ownerAddress,
		"TicketSequence":  ticketSeq,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode Ticket: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// serializeDepositPreauth serializes a DepositPreauth ledger entry
func serializeDepositPreauth(ownerID, authorizedID [20]byte) ([]byte, error) {
	ownerAddress, err := addresscodec.EncodeAccountIDToClassicAddress(ownerID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode owner address: %w", err)
	}

	authorizedAddress, err := addresscodec.EncodeAccountIDToClassicAddress(authorizedID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode authorized address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "DepositPreauth",
		"Account":         ownerAddress,
		"Authorize":       authorizedAddress,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode DepositPreauth: %w", err)
	}

	return hex.DecodeString(hexStr)
}
