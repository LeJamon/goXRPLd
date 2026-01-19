package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// UnsubscribeMethod handles the unsubscribe RPC command (WebSocket only)
type UnsubscribeMethod struct{}

func (m *UnsubscribeMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// This method should only be called through WebSocket context
	// The actual implementation is in the WebSocket handler
	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_SUPPORTED, "notSupported", "notSupported",
		"unsubscribe is only available via WebSocket")
}

func (m *UnsubscribeMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *UnsubscribeMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
