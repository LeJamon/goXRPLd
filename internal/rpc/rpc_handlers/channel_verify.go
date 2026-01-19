package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// ChannelVerifyMethod handles the channel_verify RPC method
type ChannelVerifyMethod struct{}

func (m *ChannelVerifyMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		Channel   string `json:"channel"`
		Signature string `json:"signature"`
		PublicKey string `json:"public_key"`
		Amount    string `json:"amount"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Channel == "" || request.Signature == "" || request.PublicKey == "" || request.Amount == "" {
		return nil, rpc_types.RpcErrorInvalidParams("channel, signature, public_key, and amount are required")
	}

	// TODO: Implement payment channel signature verification
	// 1. Validate channel ID, signature, public key, and amount formats
	// 2. Retrieve channel information from ledger
	// 3. Verify that public key matches channel source account
	// 4. Reconstruct the signed message from channel ID and amount
	// 5. Verify signature against message using provided public key
	// 6. Return verification result

	response := map[string]interface{}{
		"signature_verified": true, // TODO: Perform actual verification
	}

	return response, nil
}

func (m *ChannelVerifyMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *ChannelVerifyMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
