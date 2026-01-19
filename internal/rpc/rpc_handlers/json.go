package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// JsonMethod handles the json RPC method
type JsonMethod struct{}

func (m *JsonMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Method == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: method")
	}

	// TODO: Implement JSON method proxy
	// This method allows calling other RPC methods with JSON parameters
	// It's essentially a wrapper that forwards the call to the specified method
	// This can be useful for clients that need to call methods dynamically

	// Forward the call to the specified method
	// This is a recursive call through the same RPC system
	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_SUPPORTED, "notSupported", "notSupported",
		"json method forwarding not yet implemented")
}

func (m *JsonMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *JsonMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
