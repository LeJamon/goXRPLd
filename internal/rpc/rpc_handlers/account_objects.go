package rpc_handlers

import (
	"encoding/hex"
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// AccountObjectsMethod handles the account_objects RPC method
type AccountObjectsMethod struct{}

func (m *AccountObjectsMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		rpc_types.AccountParam
		rpc_types.LedgerSpecifier
		Type                 string `json:"type,omitempty"`
		DeletionBlockersOnly bool   `json:"deletion_blockers_only,omitempty"`
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

	// Determine ledger index to use
	ledgerIndex := "current"
	if request.LedgerIndex != "" {
		ledgerIndex = request.LedgerIndex.String()
	}

	// Get account objects from the ledger service
	result, err := rpc_types.Services.Ledger.GetAccountObjects(request.Account, ledgerIndex, request.Type, request.Limit)
	if err != nil {
		if err.Error() == "account not found" {
			return nil, &rpc_types.RpcError{
				Code:    19, // actNotFound
				Message: "Account not found.",
			}
		}
		return nil, rpc_types.RpcErrorInternal("Failed to get account objects: " + err.Error())
	}

	// Build account_objects array
	objects := make([]map[string]interface{}, len(result.AccountObjects))
	for i, obj := range result.AccountObjects {
		objects[i] = map[string]interface{}{
			"index":           obj.Index,
			"LedgerEntryType": obj.LedgerEntryType,
			"data":            hex.EncodeToString(obj.Data),
		}
	}

	response := map[string]interface{}{
		"account":         result.Account,
		"account_objects": objects,
		"ledger_hash":     FormatLedgerHash(result.LedgerHash),
		"ledger_index":    result.LedgerIndex,
		"validated":       result.Validated,
	}

	if result.Marker != "" {
		response["marker"] = result.Marker
	}

	return response, nil
}

func (m *AccountObjectsMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *AccountObjectsMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
