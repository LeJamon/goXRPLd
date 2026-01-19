package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// ConsensusInfoMethod handles the consensus_info RPC method
type ConsensusInfoMethod struct{}

func (m *ConsensusInfoMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// TODO: Implement consensus info
	return map[string]interface{}{
		"info": map[string]interface{}{},
	}, nil
}

func (m *ConsensusInfoMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *ConsensusInfoMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
