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
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: account")
	}

	// TODO: Implement currency retrieval
	// 1. Validate account address
	// 2. Determine target ledger
	// 3. Find all RippleState objects for the account
	// 4. Extract unique currencies that the account can send/receive
	// 5. Separate into send_currencies and receive_currencies
	// 6. Handle strict mode (only currencies with positive balance/trust)

	response := map[string]interface{}{
		"ledger_hash":        "PLACEHOLDER_LEDGER_HASH",
		"ledger_index":       1000,
		"receive_currencies": []string{
			// TODO: Load actual receivable currencies
			// Example: ["USD", "EUR", "BTC"]
		},
		"send_currencies": []string{
			// TODO: Load actual sendable currencies
			// Example: ["USD", "EUR"]
		},
		"validated": true,
	}

	return response, nil
}

func (m *AccountCurrenciesMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *AccountCurrenciesMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
