package tx

import (
	"encoding/json"
	"sort"
)

// ApplyResult contains the result of applying a transaction
type ApplyResult struct {
	// Result is the transaction result code
	Result Result

	// Applied indicates if the transaction was applied to the ledger
	Applied bool

	// Fee is the fee charged (in drops)
	Fee uint64

	// Metadata contains the changes made by the transaction
	Metadata *Metadata

	// Message is a human-readable result message
	Message string
}

// Metadata tracks changes made by a transaction
type Metadata struct {
	// AffectedNodes lists all nodes that were created, modified, or deleted
	AffectedNodes []AffectedNode

	// TransactionIndex is the index in the ledger
	TransactionIndex uint32

	// TransactionResult is the result code
	TransactionResult Result

	// DeliveredAmount is the actual amount delivered (for partial payments)
	DeliveredAmount *Amount
}

// AffectedNode represents a ledger entry that was changed
type AffectedNode struct {
	// NodeType is "CreatedNode", "ModifiedNode", or "DeletedNode"
	NodeType string

	// LedgerEntryType is the type of ledger entry
	LedgerEntryType string

	// LedgerIndex is the key of the entry
	LedgerIndex string

	// PreviousTxnLgrSeq is the ledger sequence of the previous transaction that modified this entry
	PreviousTxnLgrSeq uint32

	// PreviousTxnID is the hash of the previous transaction that modified this entry
	PreviousTxnID string

	// FinalFields contains the final state (for Modified/Deleted)
	FinalFields map[string]any

	// PreviousFields contains the previous state (for Modified)
	PreviousFields map[string]any

	// NewFields contains the new state (for Created)
	NewFields map[string]any
}

// MarshalJSON implements custom JSON marshaling for Metadata to match rippled format
func (m Metadata) MarshalJSON() ([]byte, error) {
	// Build the output structure matching rippled's format
	output := make(map[string]any)

	// Sort AffectedNodes by LedgerIndex (ascending) to match rippled's ordering
	sortedNodes := make([]AffectedNode, len(m.AffectedNodes))
	copy(sortedNodes, m.AffectedNodes)
	sort.Slice(sortedNodes, func(i, j int) bool {
		return sortedNodes[i].LedgerIndex < sortedNodes[j].LedgerIndex
	})

	// AffectedNodes with nested structure
	affectedNodes := make([]map[string]any, 0, len(sortedNodes))
	for _, node := range sortedNodes {
		nodeJSON, err := node.toRippledFormat()
		if err != nil {
			return nil, err
		}
		affectedNodes = append(affectedNodes, nodeJSON)
	}
	output["AffectedNodes"] = affectedNodes

	// TransactionIndex
	output["TransactionIndex"] = m.TransactionIndex

	// TransactionResult as string
	output["TransactionResult"] = m.TransactionResult.String()

	// delivered_amount (snake_case per rippled format)
	// Use "unavailable" for legacy compatibility when not explicitly set
	if m.DeliveredAmount != nil {
		output["delivered_amount"] = m.DeliveredAmount
	}

	return json.Marshal(output)
}

// toRippledFormat converts an AffectedNode to rippled's nested format
func (n AffectedNode) toRippledFormat() (map[string]any, error) {
	// Build the inner node content
	inner := make(map[string]any)

	// FinalFields (for ModifiedNode and DeletedNode)
	if n.FinalFields != nil {
		inner["FinalFields"] = n.FinalFields
	}

	// LedgerEntryType
	inner["LedgerEntryType"] = n.LedgerEntryType

	// LedgerIndex
	inner["LedgerIndex"] = n.LedgerIndex

	// PreviousFields (for ModifiedNode only, omit if nil/empty)
	if n.PreviousFields != nil && len(n.PreviousFields) > 0 {
		inner["PreviousFields"] = n.PreviousFields
	}

	// PreviousTxnID (omit if empty)
	if n.PreviousTxnID != "" {
		inner["PreviousTxnID"] = n.PreviousTxnID
	}

	// PreviousTxnLgrSeq (omit if zero, which means not set)
	if n.PreviousTxnLgrSeq != 0 {
		inner["PreviousTxnLgrSeq"] = n.PreviousTxnLgrSeq
	}

	// NewFields (for CreatedNode only, omit if nil)
	if n.NewFields != nil {
		inner["NewFields"] = n.NewFields
	}

	// Wrap in NodeType (e.g., "ModifiedNode": {...})
	return map[string]any{
		n.NodeType: inner,
	}, nil
}

// NewMetadata creates a new empty Metadata structure
func NewMetadata() *Metadata {
	return &Metadata{
		AffectedNodes:     make([]AffectedNode, 0),
		TransactionResult: TesSUCCESS,
	}
}

// AddCreatedNode adds a created node to the metadata
func (m *Metadata) AddCreatedNode(entryType, ledgerIndex string, newFields map[string]any) {
	m.AffectedNodes = append(m.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: entryType,
		LedgerIndex:     ledgerIndex,
		NewFields:       newFields,
	})
}

// AddModifiedNode adds a modified node to the metadata
func (m *Metadata) AddModifiedNode(entryType, ledgerIndex string, finalFields, previousFields map[string]any, prevTxnID string, prevTxnLgrSeq uint32) {
	m.AffectedNodes = append(m.AffectedNodes, AffectedNode{
		NodeType:          "ModifiedNode",
		LedgerEntryType:   entryType,
		LedgerIndex:       ledgerIndex,
		FinalFields:       finalFields,
		PreviousFields:    previousFields,
		PreviousTxnID:     prevTxnID,
		PreviousTxnLgrSeq: prevTxnLgrSeq,
	})
}

// AddDeletedNode adds a deleted node to the metadata
func (m *Metadata) AddDeletedNode(entryType, ledgerIndex string, finalFields map[string]any, prevTxnID string, prevTxnLgrSeq uint32) {
	m.AffectedNodes = append(m.AffectedNodes, AffectedNode{
		NodeType:          "DeletedNode",
		LedgerEntryType:   entryType,
		LedgerIndex:       ledgerIndex,
		FinalFields:       finalFields,
		PreviousTxnID:     prevTxnID,
		PreviousTxnLgrSeq: prevTxnLgrSeq,
	})
}

// PrependAffectedNode adds a node at the beginning of the affected nodes list
// This is used for the sender's account, which should appear first
func (m *Metadata) PrependAffectedNode(node AffectedNode) {
	m.AffectedNodes = append([]AffectedNode{node}, m.AffectedNodes...)
}
