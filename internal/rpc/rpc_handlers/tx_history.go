package rpc_handlers

import (
	"encoding/hex"
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// TxHistoryMethod handles the tx_history RPC method
type TxHistoryMethod struct{}

func (m *TxHistoryMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		Start uint32 `json:"start,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// Check if ledger service is available
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	// Get transaction history from the ledger service
	result, err := rpc_types.Services.Ledger.GetTransactionHistory(request.Start)
	if err != nil {
		if err.Error() == "transaction history not available (no database configured)" {
			return nil, &rpc_types.RpcError{
				Code:    73, // lgrNotFound
				Message: "Transaction history not available. Database not configured.",
			}
		}
		return nil, rpc_types.RpcErrorInternal("Failed to get transaction history: " + err.Error())
	}

	// Build transactions array
	txs := make([]map[string]interface{}, len(result.Transactions))
	for i, tx := range result.Transactions {
		txs[i] = map[string]interface{}{
			"hash":         hex.EncodeToString(tx.Hash[:]),
			"ledger_index": tx.LedgerIndex,
			"tx_blob":      hex.EncodeToString(tx.TxBlob),
		}
	}

	response := map[string]interface{}{
		"index": result.Index,
		"txs":   txs,
	}

	return response, nil
}

func (m *TxHistoryMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *TxHistoryMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
