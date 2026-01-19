package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// ChannelAuthorizeMethod handles the channel_authorize RPC method
type ChannelAuthorizeMethod struct{}

func (m *ChannelAuthorizeMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		Secret     string `json:"secret,omitempty"`
		Seed       string `json:"seed,omitempty"`
		SeedHex    string `json:"seed_hex,omitempty"`
		Passphrase string `json:"passphrase,omitempty"`
		KeyType    string `json:"key_type,omitempty"`
		Channel    string `json:"channel"`
		Amount     string `json:"amount"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Channel == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: channel")
	}

	if request.Amount == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: amount")
	}

	// TODO: Implement payment channel authorization
	// 1. Validate channel ID and amount
	// 2. Retrieve channel information from ledger
	// 3. Verify signing credentials correspond to channel source
	// 4. Create payment channel claim signature
	// 5. Return signature that can be used to claim from channel

	response := map[string]interface{}{
		"signature": "CHANNEL_SIGNATURE", // TODO: Generate actual signature
	}

	return response, nil
}

func (m *ChannelAuthorizeMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleUser
}

func (m *ChannelAuthorizeMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
