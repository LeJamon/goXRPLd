package tx

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// Action represents the type of modification to a ledger entry
type Action int

const (
	// ActionCache means the entry was read but not modified
	ActionCache Action = iota
	// ActionInsert means a new entry was created
	ActionInsert
	// ActionModify means an existing entry was modified
	ActionModify
	// ActionErase means an entry was deleted
	ActionErase
)

// TrackedEntry represents a ledger entry being tracked for changes
type TrackedEntry struct {
	Action   Action
	Original []byte // Original state (nil for inserts)
	Current  []byte // Current state (nil for deletes after erase)
}

// ApplyStateTable wraps a LedgerView and tracks all modifications
// for automatic metadata generation, similar to rippled's ApplyStateTable
type ApplyStateTable struct {
	base    LedgerView
	items   map[[32]byte]*TrackedEntry
	drops   XRPAmount.XRPAmount
	txHash  [32]byte
	txSeq   uint32
}

// NewApplyStateTable creates a new ApplyStateTable wrapping the given base view
func NewApplyStateTable(base LedgerView, txHash [32]byte, txSeq uint32) *ApplyStateTable {
	return &ApplyStateTable{
		base:   base,
		items:  make(map[[32]byte]*TrackedEntry),
		txHash: txHash,
		txSeq:  txSeq,
	}
}

// Read reads a ledger entry, tracking it as cached
func (t *ApplyStateTable) Read(k keylet.Keylet) ([]byte, error) {
	// Check if already tracked
	if entry, exists := t.items[k.Key]; exists {
		if entry.Action == ActionErase {
			return nil, fmt.Errorf("entry not found (deleted)")
		}
		return entry.Current, nil
	}

	// Read from base
	data, err := t.base.Read(k)
	if err != nil {
		return nil, err
	}

	// Track as cached (read but not modified)
	t.items[k.Key] = &TrackedEntry{
		Action:   ActionCache,
		Original: data,
		Current:  data,
	}

	return data, nil
}

// Exists checks if an entry exists
func (t *ApplyStateTable) Exists(k keylet.Keylet) (bool, error) {
	// Check if tracked
	if entry, exists := t.items[k.Key]; exists {
		return entry.Action != ActionErase, nil
	}

	// Check base
	return t.base.Exists(k)
}

// Insert adds a new entry
func (t *ApplyStateTable) Insert(k keylet.Keylet, data []byte) error {
	// Check if already exists in tracking
	if entry, exists := t.items[k.Key]; exists {
		if entry.Action != ActionErase {
			return fmt.Errorf("entry already exists")
		}
		// Re-inserting a deleted entry becomes a modify
		entry.Action = ActionModify
		entry.Current = data
		return nil
	}

	// Check base
	exists, err := t.base.Exists(k)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("entry already exists")
	}

	// Track as insert
	t.items[k.Key] = &TrackedEntry{
		Action:   ActionInsert,
		Original: nil,
		Current:  data,
	}

	return nil
}

// Update modifies an existing entry
func (t *ApplyStateTable) Update(k keylet.Keylet, data []byte) error {
	// Check if tracked
	if entry, exists := t.items[k.Key]; exists {
		if entry.Action == ActionErase {
			return fmt.Errorf("entry not found (deleted)")
		}
		if entry.Action == ActionCache {
			entry.Action = ActionModify
		}
		// For insert, keep it as insert with new data
		entry.Current = data
		return nil
	}

	// Read original from base to track it
	original, err := t.base.Read(k)
	if err != nil {
		return err
	}

	// Track as modified
	t.items[k.Key] = &TrackedEntry{
		Action:   ActionModify,
		Original: original,
		Current:  data,
	}

	return nil
}

// Erase removes an entry
func (t *ApplyStateTable) Erase(k keylet.Keylet) error {
	// Check if tracked
	if entry, exists := t.items[k.Key]; exists {
		if entry.Action == ActionErase {
			return fmt.Errorf("entry already deleted")
		}
		if entry.Action == ActionInsert {
			// Inserting then deleting = no change, remove from tracking
			delete(t.items, k.Key)
			return nil
		}
		// Cache or Modify -> Erase
		entry.Action = ActionErase
		entry.Current = nil
		return nil
	}

	// Read original from base
	original, err := t.base.Read(k)
	if err != nil {
		return err
	}

	// Track as erased
	t.items[k.Key] = &TrackedEntry{
		Action:   ActionErase,
		Original: original,
		Current:  nil,
	}

	return nil
}

// AdjustDropsDestroyed records destroyed XRP
func (t *ApplyStateTable) AdjustDropsDestroyed(drops XRPAmount.XRPAmount) {
	t.drops = t.drops.Add(drops)
}

// ForEach iterates over all state entries
func (t *ApplyStateTable) ForEach(fn func(key [32]byte, data []byte) bool) error {
	// This needs to iterate over base plus our modifications
	// For now, delegate to base (this is typically only used for debugging)
	return t.base.ForEach(fn)
}

// Apply commits all changes to the base view and returns generated metadata
func (t *ApplyStateTable) Apply() (*Metadata, error) {
	metadata := &Metadata{
		AffectedNodes: make([]AffectedNode, 0),
	}

	// Process each tracked item
	for key, entry := range t.items {
		switch entry.Action {
		case ActionCache:
			// No change, skip
			continue

		case ActionInsert:
			node, err := t.buildCreatedNode(key, entry.Current)
			if err != nil {
				return nil, err
			}
			metadata.AffectedNodes = append(metadata.AffectedNodes, node)

			// Apply to base
			if err := t.base.Insert(keylet.Keylet{Key: key}, entry.Current); err != nil {
				return nil, err
			}

		case ActionModify:
			// Skip if no actual change
			if bytesEqual(entry.Original, entry.Current) {
				continue
			}

			node, err := t.buildModifiedNode(key, entry.Original, entry.Current)
			if err != nil {
				return nil, err
			}
			metadata.AffectedNodes = append(metadata.AffectedNodes, node)

			// Apply to base
			if err := t.base.Update(keylet.Keylet{Key: key}, entry.Current); err != nil {
				return nil, err
			}

		case ActionErase:
			node, err := t.buildDeletedNode(key, entry.Original)
			if err != nil {
				return nil, err
			}
			metadata.AffectedNodes = append(metadata.AffectedNodes, node)

			// Apply to base
			if err := t.base.Erase(keylet.Keylet{Key: key}); err != nil {
				return nil, err
			}
		}
	}

	// Apply destroyed drops
	if t.drops.IsPositive() {
		t.base.AdjustDropsDestroyed(t.drops)
	}

	return metadata, nil
}

// DropsDestroyed returns the amount of XRP destroyed
func (t *ApplyStateTable) DropsDestroyed() XRPAmount.XRPAmount {
	return t.drops
}

// buildCreatedNode creates metadata for a newly created entry
func (t *ApplyStateTable) buildCreatedNode(key [32]byte, data []byte) (AffectedNode, error) {
	entryType := getLedgerEntryType(data)

	node := AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: entryType,
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(key[:])),
		NewFields:       make(map[string]any),
	}

	// Extract fields for NewFields based on entry type
	fields, err := extractLedgerFields(data, entryType)
	if err != nil {
		return node, err
	}

	// For CreatedNode, include all non-default fields with sMD_Create | sMD_Always
	for name, value := range fields {
		if shouldIncludeInCreate(name) && !isDefaultValue(value) {
			node.NewFields[name] = value
		}
	}

	return node, nil
}

// buildModifiedNode creates metadata for a modified entry
func (t *ApplyStateTable) buildModifiedNode(key [32]byte, original, current []byte) (AffectedNode, error) {
	entryType := getLedgerEntryType(current)

	node := AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: entryType,
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(key[:])),
		FinalFields:     make(map[string]any),
		PreviousFields:  make(map[string]any),
	}

	// Extract fields from original and current
	origFields, err := extractLedgerFields(original, entryType)
	if err != nil {
		return node, err
	}

	currFields, err := extractLedgerFields(current, entryType)
	if err != nil {
		return node, err
	}

	// Extract PreviousTxnID and PreviousTxnLgrSeq from original
	if prevTxnID, ok := origFields["PreviousTxnID"]; ok {
		node.PreviousTxnID = fmt.Sprintf("%v", prevTxnID)
	}
	if prevTxnLgrSeq, ok := origFields["PreviousTxnLgrSeq"]; ok {
		if seq, ok := prevTxnLgrSeq.(uint32); ok {
			node.PreviousTxnLgrSeq = seq
		}
	}

	// PreviousFields: fields that changed (sMD_ChangeOrig)
	for name, origValue := range origFields {
		if shouldIncludeInPreviousFields(name) {
			if currValue, exists := currFields[name]; exists {
				if !fieldsEqual(origValue, currValue) {
					node.PreviousFields[name] = origValue
				}
			} else {
				// Field was removed
				node.PreviousFields[name] = origValue
			}
		}
	}

	// FinalFields: fields with sMD_Always | sMD_ChangeNew
	for name, currValue := range currFields {
		if shouldIncludeInFinalFields(name) {
			node.FinalFields[name] = currValue
		}
	}

	// Clean up empty maps
	if len(node.PreviousFields) == 0 {
		node.PreviousFields = nil
	}
	if len(node.FinalFields) == 0 {
		node.FinalFields = nil
	}

	return node, nil
}

// buildDeletedNode creates metadata for a deleted entry
func (t *ApplyStateTable) buildDeletedNode(key [32]byte, original []byte) (AffectedNode, error) {
	entryType := getLedgerEntryType(original)

	node := AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: entryType,
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(key[:])),
		FinalFields:     make(map[string]any),
		PreviousFields:  make(map[string]any),
	}

	// Extract fields from original
	origFields, err := extractLedgerFields(original, entryType)
	if err != nil {
		return node, err
	}

	// Extract PreviousTxnID and PreviousTxnLgrSeq
	if prevTxnID, ok := origFields["PreviousTxnID"]; ok {
		node.PreviousTxnID = fmt.Sprintf("%v", prevTxnID)
	}
	if prevTxnLgrSeq, ok := origFields["PreviousTxnLgrSeq"]; ok {
		if seq, ok := prevTxnLgrSeq.(uint32); ok {
			node.PreviousTxnLgrSeq = seq
		}
	}

	// FinalFields: fields with sMD_Always | sMD_DeleteFinal
	for name, value := range origFields {
		if shouldIncludeInDeleteFinal(name) {
			node.FinalFields[name] = value
		}
	}

	// Clean up empty maps
	if len(node.PreviousFields) == 0 {
		node.PreviousFields = nil
	}
	if len(node.FinalFields) == 0 {
		node.FinalFields = nil
	}

	return node, nil
}

// getLedgerEntryType extracts the ledger entry type from binary data
func getLedgerEntryType(data []byte) string {
	if len(data) < 4 {
		return "Unknown"
	}

	// Parse the binary to find LedgerEntryType field
	// LedgerEntryType is a UInt16 with type code 1 and field code 1
	// Header byte: 0x11 (type 1, field 1)
	offset := 0
	for offset < len(data)-2 {
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

		// Check for LedgerEntryType (type 1 = UInt16, field 1)
		if typeCode == 1 && fieldCode == 1 {
			if offset+2 > len(data) {
				break
			}
			entryType := binary.BigEndian.Uint16(data[offset : offset+2])
			return ledgerEntryTypeName(entryType)
		}

		// Skip field value based on type
		switch typeCode {
		case 1: // UInt16
			offset += 2
		case 2: // UInt32
			offset += 4
		case 3: // UInt64
			offset += 8
		case 4: // Hash128
			offset += 16
		case 5: // Hash256
			offset += 32
		case 6: // Amount
			if offset < len(data) && (data[offset]&0x80) == 0 {
				offset += 8 // XRP
			} else {
				offset += 48 // IOU
			}
		case 7: // Blob (variable length)
			if offset >= len(data) {
				return "Unknown"
			}
			length := int(data[offset])
			offset++
			if length > 192 {
				if offset >= len(data) {
					return "Unknown"
				}
				length = 193 + ((length-193)<<8 | int(data[offset]))
				offset++
			}
			offset += length
		case 8: // AccountID
			offset += 20
		default:
			// Unknown type, can't continue
			return "Unknown"
		}
	}

	return "Unknown"
}

// ledgerEntryTypeName converts entry type code to name
func ledgerEntryTypeName(code uint16) string {
	switch code {
	case 0x0061: // 'a'
		return "AccountRoot"
	case 0x0063: // 'c'
		return "Contract"
	case 0x0064: // 'd'
		return "DirectoryNode"
	case 0x0066: // 'f'
		return "FeeSettings"
	case 0x0068: // 'h'
		return "LedgerHashes"
	case 0x006E: // 'n'
		return "NegativeUNL"
	case 0x006F: // 'o'
		return "Offer"
	case 0x0072: // 'r'
		return "RippleState"
	case 0x0073: // 's'
		return "SignerList"
	case 0x0074: // 't'
		return "Ticket"
	case 0x0075: // 'u'
		return "Escrow"
	case 0x0078: // 'x'
		return "PayChannel"
	case 0x0079: // 'y'
		return "Check"
	case 0x007A: // 'z'
		return "DepositPreauth"
	case 0x0050: // 'P'
		return "NFTokenPage"
	case 0x0051: // 'Q'
		return "NFTokenOffer"
	case 0x0041: // 'A'
		return "Amendments"
	default:
		return fmt.Sprintf("Unknown(0x%04x)", code)
	}
}

// bytesEqual compares two byte slices
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// fieldsEqual compares two field values
func fieldsEqual(a, b any) bool {
	// For maps (like Amount), compare recursively
	aMap, aIsMap := a.(map[string]any)
	bMap, bIsMap := b.(map[string]any)
	if aIsMap && bIsMap {
		if len(aMap) != len(bMap) {
			return false
		}
		for k, v := range aMap {
			if bv, ok := bMap[k]; !ok || !fieldsEqual(v, bv) {
				return false
			}
		}
		return true
	}

	// Direct comparison
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// Note: isDefaultValue is defined in sle.go
