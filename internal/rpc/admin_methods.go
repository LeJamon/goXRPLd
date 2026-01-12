package rpc

import (
	"encoding/json"
)

// LedgerAcceptMethod handles the ledger_accept RPC method
// This is a standalone-mode only command that manually closes and validates
// the current open ledger, allowing progression without consensus.
type LedgerAcceptMethod struct{}

func (m *LedgerAcceptMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Check if services are initialized
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not initialized")
	}

	// Check if running in standalone mode
	if !Services.Ledger.IsStandalone() {
		return nil, NewRpcError(RpcNOT_STANDALONE, "notStandalone", "notStandalone",
			"ledger_accept is only available in standalone mode")
	}

	// Accept the ledger
	closedSeq, err := Services.Ledger.AcceptLedger()
	if err != nil {
		return nil, RpcErrorInternal("Failed to accept ledger: " + err.Error())
	}

	response := map[string]interface{}{
		"ledger_current_index": closedSeq + 1, // Return the new open ledger index
	}

	return response, nil
}

func (m *LedgerAcceptMethod) RequiredRole() Role {
	return RoleAdmin // ledger_accept requires admin privileges
}

func (m *LedgerAcceptMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}
