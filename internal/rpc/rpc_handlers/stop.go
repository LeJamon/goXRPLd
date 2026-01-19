package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// StopMethod handles the stop RPC method
type StopMethod struct{}

func (m *StopMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// TODO: Implement graceful server shutdown
	// 1. Validate admin credentials
	// 2. Stop accepting new connections
	// 3. Complete pending transactions
	// 4. Close database connections
	// 5. Shut down server components

	response := map[string]interface{}{
		"message": "rippled server stopping",
	}

	return response, nil
}

func (m *StopMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *StopMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
