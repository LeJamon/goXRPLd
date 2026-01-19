package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// DownloadShardMethod handles the download_shard RPC method
type DownloadShardMethod struct{}

func (m *DownloadShardMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	return map[string]interface{}{"message": "shard download initiated"}, nil
}

func (m *DownloadShardMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *DownloadShardMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// CrawlShardsMethod handles the crawl_shards RPC method
type CrawlShardsMethod struct{}

func (m *CrawlShardsMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	return map[string]interface{}{"shards": []interface{}{}}, nil
}

func (m *CrawlShardsMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *CrawlShardsMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
