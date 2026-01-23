package tx

import (
	"encoding/hex"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
)

// Field metadata flags matching rippled's SField metadata flags
const (
	// sMD_Never - never include in metadata
	sMD_Never uint8 = 0x00
	// sMD_ChangeOrig - include original value when field changes (goes to PreviousFields)
	sMD_ChangeOrig uint8 = 0x01
	// sMD_ChangeNew - include new value when field changes (goes to FinalFields)
	sMD_ChangeNew uint8 = 0x02
	// sMD_DeleteFinal - include final value when node is deleted
	sMD_DeleteFinal uint8 = 0x04
	// sMD_Create - include value when node is created (goes to NewFields)
	sMD_Create uint8 = 0x08
	// sMD_Always - always include if node is affected
	sMD_Always uint8 = 0x10
	// sMD_Default - default flags for most fields
	sMD_Default uint8 = sMD_ChangeOrig | sMD_ChangeNew | sMD_DeleteFinal | sMD_Create
)

// fieldMetadata maps field names to their metadata flags
// Based on rippled's sfields.macro definitions
// Only non-default flags are listed here; all other fields use sMD_Default
var fieldMetadata = map[string]uint8{
	// sMD_Never (0x00) - never appear in metadata
	"LedgerEntryType": sMD_Never,
	"Indexes":         sMD_Never,

	// sMD_Always (0x10) - always appear if node is affected
	"RootIndex": sMD_Always,

	// sMD_DeleteFinal (0x04) - only in FinalFields when deleted
	// These fields are special: they are updated by threading logic,
	// not by standard metadata inclusion rules
	"PreviousTxnLgrSeq": sMD_DeleteFinal,
	"PreviousTxnID":     sMD_DeleteFinal,
}

// getFieldMetadata returns the metadata flags for a field
func getFieldMetadata(fieldName string) uint8 {
	if flags, ok := fieldMetadata[fieldName]; ok {
		return flags
	}
	return sMD_Default
}

// shouldIncludeInPreviousFields returns true if a changed field should be in PreviousFields
func shouldIncludeInPreviousFields(fieldName string) bool {
	flags := getFieldMetadata(fieldName)
	return (flags & sMD_ChangeOrig) != 0
}

// shouldIncludeInFinalFields returns true if a field should be in FinalFields
func shouldIncludeInFinalFields(fieldName string) bool {
	flags := getFieldMetadata(fieldName)
	return (flags & (sMD_Always | sMD_ChangeNew)) != 0
}

// shouldIncludeInCreate returns true if a field should be in NewFields for created nodes
func shouldIncludeInCreate(fieldName string) bool {
	flags := getFieldMetadata(fieldName)
	return (flags & (sMD_Create | sMD_Always)) != 0
}

// shouldIncludeInDeleteFinal returns true if a field should be in FinalFields for deleted nodes
func shouldIncludeInDeleteFinal(fieldName string) bool {
	flags := getFieldMetadata(fieldName)
	return (flags & (sMD_Always | sMD_DeleteFinal)) != 0
}

// extractLedgerFields extracts fields from binary ledger entry data
// Uses the binary codec to generically decode any ledger entry type
func extractLedgerFields(data []byte, entryType string) (map[string]any, error) {
	// Convert binary data to hex string for the codec
	hexStr := hex.EncodeToString(data)

	// Use the binary codec to decode the ledger entry
	fields, err := binarycodec.Decode(hexStr)
	if err != nil {
		// If decoding fails, return empty map (graceful degradation)
		return make(map[string]any), nil
	}

	return fields, nil
}
