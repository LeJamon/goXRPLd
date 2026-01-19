package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// ValidationCreateMethod handles the validation_create RPC method
type ValidationCreateMethod struct{}

func (m *ValidationCreateMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		Secret  string `json:"secret,omitempty"`
		KeyType string `json:"key_type,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// TODO: Implement validation key creation
	// 1. Generate or use provided validation key pair
	// 2. Create validation key manifest
	// 3. Configure server to use validation keys
	// 4. Return public key information

	response := map[string]interface{}{
		"validation_key":        "VALIDATION_KEY",
		"validation_public_key": "PUBLIC_KEY",
		"validation_seed":       "SEED",
	}

	return response, nil
}

func (m *ValidationCreateMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *ValidationCreateMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
