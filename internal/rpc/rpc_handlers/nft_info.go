package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// NftInfoMethod handles the nft_info RPC method (Clio-specific)
type NftInfoMethod struct{}

func (m *NftInfoMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// TODO: Implement NFT info method (Clio extension)
	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_SUPPORTED, "notSupported", "notSupported",
		"nft_info is a Clio-specific method not supported in rippled mode")
}

func (m *NftInfoMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *NftInfoMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
