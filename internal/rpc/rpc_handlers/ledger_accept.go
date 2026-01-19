package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// LedgerAcceptMethod handles the ledger_accept RPC method
// This is a standalone-mode only command that manually closes and validates
// the current open ledger, allowing progression without consensus.
type LedgerAcceptMethod struct{}

func (m *LedgerAcceptMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// Check if services are initialized
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not initialized")
	}

	// Check if running in standalone mode
	if !rpc_types.Services.Ledger.IsStandalone() {
		return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_STANDALONE, "notStandalone", "notStandalone",
			"ledger_accept is only available in standalone mode")
	}

	// Accept the ledger
	closedSeq, err := rpc_types.Services.Ledger.AcceptLedger()
	if err != nil {
		return nil, rpc_types.RpcErrorInternal("Failed to accept ledger: " + err.Error())
	}

	response := map[string]interface{}{
		"ledger_current_index": closedSeq + 1, // Return the new open ledger index
	}

	return response, nil
}

func (m *LedgerAcceptMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin // ledger_accept requires admin privileges
}

func (m *LedgerAcceptMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
