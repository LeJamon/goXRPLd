package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// DepositAuthorizedMethod handles the deposit_authorized RPC method
type DepositAuthorizedMethod struct{}

func (m *DepositAuthorizedMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		SourceAccount      string `json:"source_account"`
		DestinationAccount string `json:"destination_account"`
		rpc_types.LedgerSpecifier
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.SourceAccount == "" || request.DestinationAccount == "" {
		return nil, rpc_types.RpcErrorInvalidParams("source_account and destination_account are required")
	}

	// TODO: Implement deposit authorization checking
	// 1. Determine target ledger
	// 2. Check destination account's DepositAuth flag
	// 3. If DepositAuth is set, check for DepositPreauth object
	// 4. Verify if source account is authorized to send payments
	// 5. Consider special cases (same account, XRP vs IOU, etc.)

	response := map[string]interface{}{
		"source_account":      request.SourceAccount,
		"destination_account": request.DestinationAccount,
		"deposit_authorized":  true, // TODO: Calculate actual authorization status
		"ledger_hash":         "PLACEHOLDER_LEDGER_HASH",
		"ledger_index":        1000,
		"validated":           true,
	}

	return response, nil
}

func (m *DepositAuthorizedMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *DepositAuthorizedMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
