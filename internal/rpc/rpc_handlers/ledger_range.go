package rpc_handlers

import (
	"encoding/hex"
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// LedgerRangeMethod handles the ledger_range RPC method
type LedgerRangeMethod struct{}

func (m *LedgerRangeMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// Parse parameters
	var request struct {
		StartLedger uint32 `json:"start_ledger"`
		StopLedger  uint32 `json:"stop_ledger"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// Validate range
	if request.StartLedger == 0 || request.StopLedger == 0 {
		return nil, rpc_types.RpcErrorInvalidParams("start_ledger and stop_ledger are required")
	}

	if request.StartLedger > request.StopLedger {
		return nil, rpc_types.RpcErrorInvalidParams("start_ledger cannot be greater than stop_ledger")
	}

	// Limit range size to prevent abuse
	if request.StopLedger-request.StartLedger > 1000 {
		return nil, rpc_types.RpcErrorInvalidParams("Ledger range too large (max 1000 ledgers)")
	}

	// Check if ledger service is available
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	// Get ledger range from the ledger service
	result, err := rpc_types.Services.Ledger.GetLedgerRange(request.StartLedger, request.StopLedger)
	if err != nil {
		return nil, rpc_types.RpcErrorInternal("Failed to get ledger range: " + err.Error())
	}

	// Build ledgers array
	ledgers := make([]map[string]interface{}, 0, len(result.Hashes))
	for seq, hash := range result.Hashes {
		ledgers = append(ledgers, map[string]interface{}{
			"ledger_index": seq,
			"ledger_hash":  hex.EncodeToString(hash[:]),
		})
	}

	response := map[string]interface{}{
		"ledger_first": result.LedgerFirst,
		"ledger_last":  result.LedgerLast,
		"ledgers":      ledgers,
	}

	return response, nil
}

func (m *LedgerRangeMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin // This method requires admin privileges
}

func (m *LedgerRangeMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
