package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// NftSellOffersMethod handles the nft_sell_offers RPC method
type NftSellOffersMethod struct{}

func (m *NftSellOffersMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		NFTokenID string `json:"nft_id"`
		rpc_types.LedgerSpecifier
		rpc_types.PaginationParams
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.NFTokenID == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: nft_id")
	}

	// TODO: Implement NFT sell offer retrieval - similar to buy offers

	response := map[string]interface{}{
		"nft_id": request.NFTokenID,
		"offers": []interface{}{
			// TODO: Load actual sell offers
		},
		"ledger_hash":  "PLACEHOLDER_LEDGER_HASH",
		"ledger_index": 1000,
		"validated":    true,
	}

	return response, nil
}

func (m *NftSellOffersMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *NftSellOffersMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
