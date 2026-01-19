package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// ValidatorsMethod handles the validators RPC method
type ValidatorsMethod struct{}

func (m *ValidatorsMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	return map[string]interface{}{"validators": []interface{}{}}, nil
}

func (m *ValidatorsMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *ValidatorsMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// ValidatorListSitesMethod handles the validator_list_sites RPC method
type ValidatorListSitesMethod struct{}

func (m *ValidatorListSitesMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	return map[string]interface{}{"validator_sites": []interface{}{}}, nil
}

func (m *ValidatorListSitesMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *ValidatorListSitesMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
