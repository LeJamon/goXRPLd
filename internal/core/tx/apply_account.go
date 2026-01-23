package tx

import (
	"encoding/hex"
	"fmt"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
)

// parseSignerList parses a SignerList ledger entry from binary data
func parseSignerList(data []byte) (*SignerListInfo, error) {
	// Decode the binary data to a map using the binary codec
	hexStr := hex.EncodeToString(data)
	decoded, err := binarycodec.Decode(hexStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode SignerList: %w", err)
	}

	signerList := &SignerListInfo{
		SignerListID: 0, // Always 0 currently
	}

	// Parse SignerQuorum
	if quorum, ok := decoded["SignerQuorum"]; ok {
		switch v := quorum.(type) {
		case float64:
			signerList.SignerQuorum = uint32(v)
		case int:
			signerList.SignerQuorum = uint32(v)
		case uint32:
			signerList.SignerQuorum = v
		}
	}

	// Parse SignerEntries
	if entries, ok := decoded["SignerEntries"]; ok {
		if entriesArray, ok := entries.([]interface{}); ok {
			for _, entryWrapper := range entriesArray {
				if entryMap, ok := entryWrapper.(map[string]interface{}); ok {
					// Handle wrapped SignerEntry
					var signerEntry map[string]interface{}
					if se, ok := entryMap["SignerEntry"]; ok {
						signerEntry, _ = se.(map[string]interface{})
					} else {
						signerEntry = entryMap
					}

					if signerEntry != nil {
						entry := AccountSignerEntry{}
						if account, ok := signerEntry["Account"].(string); ok {
							entry.Account = account
						}
						if weight, ok := signerEntry["SignerWeight"]; ok {
							switch v := weight.(type) {
							case float64:
								entry.SignerWeight = uint16(v)
							case int:
								entry.SignerWeight = uint16(v)
							case uint16:
								entry.SignerWeight = v
							}
						}
						if walletLocator, ok := signerEntry["WalletLocator"].(string); ok {
							entry.WalletLocator = walletLocator
						}
						signerList.SignerEntries = append(signerList.SignerEntries, entry)
					}
				}
			}
		}
	}

	return signerList, nil
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
