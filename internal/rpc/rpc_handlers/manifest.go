package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// ManifestMethod handles the manifest RPC method
type ManifestMethod struct{}

func (m *ManifestMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		PublicKey string `json:"public_key"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// TODO: Implement validator manifest retrieval
	// 1. Look up manifest for specified validator public key
	// 2. Return manifest details including ephemeral keys and signature

	response := map[string]interface{}{
		"details": map[string]interface{}{
			// TODO: Load actual manifest details
		},
		"manifest":  "MANIFEST_DATA",
		"requested": request.PublicKey,
	}

	return response, nil
}

func (m *ManifestMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *ManifestMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
