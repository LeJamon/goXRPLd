package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// TransactionEntryMethod handles the transaction_entry RPC method
type TransactionEntryMethod struct{}

func (m *TransactionEntryMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		TxHash      string `json:"tx_hash"`
		LedgerHash  string `json:"ledger_hash,omitempty"`
		LedgerIndex string `json:"ledger_index,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.TxHash == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: tx_hash")
	}

	// TODO: Implement transaction entry lookup from specific ledger
	// 1. Determine target ledger (hash or index)
	// 2. Look up transaction in the specified ledger only
	// 3. Return transaction data with metadata from that specific ledger
	// 4. This is different from 'tx' method which searches across ledger range

	response := map[string]interface{}{
		"ledger_hash":  "PLACEHOLDER_LEDGER_HASH", // TODO: Use actual ledger hash
		"ledger_index": 1000,                       // TODO: Use actual ledger index
		"metadata": map[string]interface{}{
			"AffectedNodes":     []interface{}{}, // TODO: Load actual metadata
			"TransactionIndex":  0,
			"TransactionResult": "tesSUCCESS",
		},
		"tx_json": map[string]interface{}{
			// TODO: Load actual transaction JSON
			"Account":         "rAccount...",
			"TransactionType": "Payment",
			"hash":            request.TxHash,
		},
		"validated": true,
	}

	return response, nil
}

func (m *TransactionEntryMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *TransactionEntryMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
