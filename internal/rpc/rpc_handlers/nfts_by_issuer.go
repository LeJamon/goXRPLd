package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// NftsByIssuerMethod handles the nfts_by_issuer RPC method
type NftsByIssuerMethod struct{}

func (m *NftsByIssuerMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		Issuer       string `json:"issuer"`
		NFTokenTaxon uint32 `json:"nft_taxon,omitempty"`
		rpc_types.LedgerSpecifier
		rpc_types.PaginationParams
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Issuer == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: issuer")
	}

	// TODO: Implement NFTs by issuer retrieval

	response := map[string]interface{}{
		"issuer": request.Issuer,
		"nfts": []interface{}{
			// TODO: Load NFTs by issuer
		},
		"ledger_hash":  "PLACEHOLDER_LEDGER_HASH",
		"ledger_index": 1000,
		"validated":    true,
	}

	return response, nil
}

func (m *NftsByIssuerMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *NftsByIssuerMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
