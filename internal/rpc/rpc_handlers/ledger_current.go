package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// LedgerCurrentMethod handles the ledger_current RPC method
type LedgerCurrentMethod struct{}

func (m *LedgerCurrentMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// Check if ledger service is available
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	// Get the current (open) ledger index
	seq := rpc_types.Services.Ledger.GetCurrentLedgerIndex()
	if seq == 0 {
		return nil, &rpc_types.RpcError{Code: -1, ErrorString: "lgrNotFound", Message: "No current ledger"}
	}

	response := map[string]interface{}{
		"ledger_current_index": seq,
	}

	return response, nil
}

func (m *LedgerCurrentMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *LedgerCurrentMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
