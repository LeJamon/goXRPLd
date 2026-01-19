package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// PathFindMethod handles the path_find RPC method (WebSocket only)
type PathFindMethod struct{}

func (m *PathFindMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// This method is only available via WebSocket as it creates a persistent path-finding session
	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_SUPPORTED, "notSupported", "notSupported",
		"path_find is only available via WebSocket")
}

func (m *PathFindMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *PathFindMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
