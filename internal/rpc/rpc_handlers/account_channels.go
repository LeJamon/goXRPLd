package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// AccountChannelsMethod handles the account_channels RPC method
type AccountChannelsMethod struct{}

func (m *AccountChannelsMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		rpc_types.AccountParam
		rpc_types.LedgerSpecifier
		DestinationAccount string `json:"destination_account,omitempty"`
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

	// Get account channels from the ledger service
	result, err := rpc_types.Services.Ledger.GetAccountChannels(
		request.Account,
		request.DestinationAccount,
		ledgerIndex,
		request.Limit,
	)
	if err != nil {
		if err.Error() == "account not found" {
			return nil, &rpc_types.RpcError{
				Code:    rpc_types.RpcACT_NOT_FOUND,
				Message: "Account not found.",
			}
		}
		// Handle malformed destination_account address
		if len(err.Error()) > 32 && err.Error()[:32] == "invalid destination_account addr" {
			return nil, rpc_types.RpcErrorInvalidParams("Destination account malformed.")
		}
		return nil, rpc_types.RpcErrorInternal("Failed to get account channels: " + err.Error())
	}

	// Build channels array with proper field handling
	channels := make([]map[string]interface{}, len(result.Channels))
	for i, ch := range result.Channels {
		channel := map[string]interface{}{
			"channel_id":          ch.ChannelID,
			"account":             ch.Account,
			"destination_account": ch.DestinationAccount,
			"amount":              ch.Amount,
			"balance":             ch.Balance,
			"settle_delay":        ch.SettleDelay,
		}

		// Add optional fields only if they have values
		if ch.PublicKey != "" {
			channel["public_key"] = ch.PublicKey
		}
		if ch.PublicKeyHex != "" {
			channel["public_key_hex"] = ch.PublicKeyHex
		}
		if ch.Expiration > 0 {
			channel["expiration"] = ch.Expiration
		}
		if ch.CancelAfter > 0 {
			channel["cancel_after"] = ch.CancelAfter
		}
		if ch.HasSourceTag {
			channel["source_tag"] = ch.SourceTag
		}
		if ch.HasDestTag {
			channel["destination_tag"] = ch.DestinationTag
		}

		channels[i] = channel
	}

	// Build response
	response := map[string]interface{}{
		"account":      result.Account,
		"channels":     channels,
		"ledger_hash":  FormatLedgerHash(result.LedgerHash),
		"ledger_index": result.LedgerIndex,
		"validated":    result.Validated,
	}

	if result.Marker != "" {
		response["marker"] = result.Marker
	}

	return response, nil
}

func (m *AccountChannelsMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *AccountChannelsMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
