package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// AccountCurrenciesMethod handles the account_currencies RPC method
type AccountCurrenciesMethod struct{}

func (m *AccountCurrenciesMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		rpc_types.AccountParam
		rpc_types.LedgerSpecifier
		Strict bool `json:"strict,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Account == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing field 'account'.")
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

	// Get account currencies from the ledger service
	result, err := rpc_types.Services.Ledger.GetAccountCurrencies(
		request.Account,
		ledgerIndex,
	)
	if err != nil {
		if err.Error() == "account not found" {
			return nil, &rpc_types.RpcError{
				Code:    rpc_types.RpcACT_NOT_FOUND,
				Message: "Account not found.",
			}
		}
		// Check for malformed account address
		if len(err.Error()) > 24 && err.Error()[:24] == "invalid account address:" {
			return nil, &rpc_types.RpcError{
				Code:    rpc_types.RpcACT_NOT_FOUND,
				Message: "Account malformed.",
			}
		}
		return nil, rpc_types.RpcErrorInternal("Failed to get account currencies: " + err.Error())
	}

	// Build response
	response := map[string]interface{}{
		"ledger_hash":        FormatLedgerHash(result.LedgerHash),
		"ledger_index":       result.LedgerIndex,
		"receive_currencies": result.ReceiveCurrencies,
		"send_currencies":    result.SendCurrencies,
		"validated":          result.Validated,
	}

	return response, nil
}

func (m *AccountCurrenciesMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *AccountCurrenciesMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
