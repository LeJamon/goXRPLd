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

	if request.SourceAccount == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing field 'source_account'.")
	}

	if request.DestinationAccount == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing field 'destination_account'.")
	}

	// Check if service is available
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	// Determine ledger index to use
	ledgerIndex := "validated"
	if request.LedgerIndex != "" {
		ledgerIndex = request.LedgerIndex.String()
	}

	// Call the service
	result, err := rpc_types.Services.Ledger.GetDepositAuthorized(
		request.SourceAccount,
		request.DestinationAccount,
		ledgerIndex,
	)
	if err != nil {
		// Handle specific errors
		errMsg := err.Error()

		// Source account not found
		if errMsg == "source account not found" {
			return nil, &rpc_types.RpcError{
				Code:    rpc_types.RpcSRC_ACT_NOT_FOUND,
				Message: "Source account not found.",
			}
		}

		// Destination account not found
		if errMsg == "destination account not found" {
			return nil, &rpc_types.RpcError{
				Code:    rpc_types.RpcDST_ACT_NOT_FOUND,
				Message: "Destination account not found.",
			}
		}

		// Check for malformed source_account address
		if len(errMsg) > 32 && errMsg[:32] == "invalid source_account address: " {
			return nil, &rpc_types.RpcError{
				Code:    rpc_types.RpcACT_MALFORMED,
				Message: "Account malformed.",
			}
		}

		// Check for malformed destination_account address
		if len(errMsg) > 37 && errMsg[:37] == "invalid destination_account address: " {
			return nil, &rpc_types.RpcError{
				Code:    rpc_types.RpcACT_MALFORMED,
				Message: "Account malformed.",
			}
		}

		return nil, rpc_types.RpcErrorInternal(errMsg)
	}

	// Build response
	response := map[string]interface{}{
		"source_account":      result.SourceAccount,
		"destination_account": result.DestinationAccount,
		"deposit_authorized":  result.DepositAuthorized,
		"ledger_hash":         FormatLedgerHash(result.LedgerHash),
		"ledger_index":        result.LedgerIndex,
		"validated":           result.Validated,
	}

	return response, nil
}

func (m *DepositAuthorizedMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *DepositAuthorizedMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
