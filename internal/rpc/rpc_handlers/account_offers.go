package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// AccountOffersMethod handles the account_offers RPC method
type AccountOffersMethod struct{}

func (m *AccountOffersMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		rpc_types.AccountParam
		rpc_types.LedgerSpecifier
		Strict bool `json:"strict,omitempty"`
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

	// Get account offers from the ledger service
	result, err := rpc_types.Services.Ledger.GetAccountOffers(request.Account, ledgerIndex, request.Limit)
	if err != nil {
		if err.Error() == "account not found" {
			return nil, &rpc_types.RpcError{
				Code:    19, // actNotFound
				Message: "Account not found.",
			}
		}
		return nil, rpc_types.RpcErrorInternal("Failed to get account offers: " + err.Error())
	}

	// Build response
	response := map[string]interface{}{
		"account":      result.Account,
		"offers":       result.Offers,
		"ledger_hash":  FormatLedgerHash(result.LedgerHash),
		"ledger_index": result.LedgerIndex,
		"validated":    result.Validated,
	}

	if result.Marker != "" {
		response["marker"] = result.Marker
	}

	return response, nil
}

func (m *AccountOffersMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *AccountOffersMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
