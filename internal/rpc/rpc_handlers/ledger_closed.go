package rpc_handlers

import (
	"encoding/hex"
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// LedgerClosedMethod handles the ledger_closed RPC method
type LedgerClosedMethod struct{}

func (m *LedgerClosedMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// Check if ledger service is available
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	// Get the closed ledger index
	seq := rpc_types.Services.Ledger.GetClosedLedgerIndex()
	if seq == 0 {
		return nil, &rpc_types.RpcError{Code: -1, ErrorString: "lgrNotFound", Message: "No closed ledger"}
	}

	// Get the ledger to retrieve its hash
	ledger, err := rpc_types.Services.Ledger.GetLedgerBySequence(seq)
	if err != nil {
		return nil, &rpc_types.RpcError{Code: -1, ErrorString: "lgrNotFound", Message: "Closed ledger not found"}
	}

	hash := ledger.Hash()
	response := map[string]interface{}{
		"ledger_hash":  hex.EncodeToString(hash[:]),
		"ledger_index": seq,
	}

	return response, nil
}

func (m *LedgerClosedMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *LedgerClosedMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
