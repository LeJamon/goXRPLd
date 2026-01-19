package rpc_handlers

import (
	"encoding/hex"
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// AccountTxMethod handles the account_tx RPC method
type AccountTxMethod struct{}

func (m *AccountTxMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		rpc_types.AccountParam
		LedgerIndexMin int32  `json:"ledger_index_min,omitempty"`
		LedgerIndexMax int32  `json:"ledger_index_max,omitempty"`
		LedgerHash     string `json:"ledger_hash,omitempty"`
		LedgerIndex    string `json:"ledger_index,omitempty"`
		Binary         bool   `json:"binary,omitempty"`
		Forward        bool   `json:"forward,omitempty"`
		rpc_types.PaginationParams
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Account == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: account")
	}

	// Check if ledger service is available
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	// Parse marker if provided
	var marker *rpc_types.AccountTxMarker
	if request.Marker != nil {
		if markerMap, ok := request.Marker.(map[string]interface{}); ok {
			marker = &rpc_types.AccountTxMarker{}
			if ledger, ok := markerMap["ledger"].(float64); ok {
				marker.LedgerSeq = uint32(ledger)
			}
			if seq, ok := markerMap["seq"].(float64); ok {
				marker.TxnSeq = uint32(seq)
			}
		}
	}

	// Get account transactions from the ledger service
	result, err := rpc_types.Services.Ledger.GetAccountTransactions(
		request.Account,
		int64(request.LedgerIndexMin),
		int64(request.LedgerIndexMax),
		request.Limit,
		marker,
		request.Forward,
	)
	if err != nil {
		if err.Error() == "transaction history not available (no database configured)" {
			return nil, &rpc_types.RpcError{
				Code:    73, // lgrNotFound
				Message: "Transaction history not available. Database not configured.",
			}
		}
		if err.Error() == "account not found" {
			return nil, &rpc_types.RpcError{
				Code:    19, // actNotFound
				Message: "Account not found.",
			}
		}
		return nil, rpc_types.RpcErrorInternal("Failed to get account transactions: " + err.Error())
	}

	// Build transactions array
	transactions := make([]map[string]interface{}, len(result.Transactions))
	for i, tx := range result.Transactions {
		txEntry := map[string]interface{}{
			"ledger_index": tx.LedgerIndex,
			"validated":    true,
		}
		if request.Binary {
			txEntry["tx_blob"] = hex.EncodeToString(tx.TxBlob)
			txEntry["meta"] = hex.EncodeToString(tx.Meta)
		} else {
			// Parse tx_blob and meta as JSON if not binary
			txEntry["tx_blob"] = hex.EncodeToString(tx.TxBlob)
			txEntry["meta"] = hex.EncodeToString(tx.Meta)
		}
		transactions[i] = txEntry
	}

	response := map[string]interface{}{
		"account":          result.Account,
		"ledger_index_min": result.LedgerMin,
		"ledger_index_max": result.LedgerMax,
		"limit":            result.Limit,
		"transactions":     transactions,
		"validated":        result.Validated,
	}

	if result.Marker != nil {
		response["marker"] = map[string]interface{}{
			"ledger": result.Marker.LedgerSeq,
			"seq":    result.Marker.TxnSeq,
		}
	}

	return response, nil
}

func (m *AccountTxMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *AccountTxMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
