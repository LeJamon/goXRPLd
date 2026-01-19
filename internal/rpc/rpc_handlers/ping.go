package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// PingMethod handles the ping RPC method
type PingMethod struct{}

func (m *PingMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// Ping method is used to test connectivity and measure round-trip time
	// It simply returns an empty success response

	response := map[string]interface{}{
		// Empty response indicates successful ping
	}

	return response, nil
}

func (m *PingMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *PingMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
