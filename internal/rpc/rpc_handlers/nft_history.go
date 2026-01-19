package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// NftHistoryMethod handles the nft_history RPC method
type NftHistoryMethod struct{}

func (m *NftHistoryMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		NFTokenID      string `json:"nft_id"`
		LedgerIndexMin uint32 `json:"ledger_index_min,omitempty"`
		LedgerIndexMax uint32 `json:"ledger_index_max,omitempty"`
		Binary         bool   `json:"binary,omitempty"`
		Forward        bool   `json:"forward,omitempty"`
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

	// TODO: Implement NFT transaction history
	// Similar to account_tx but filtered for a specific NFT

	response := map[string]interface{}{
		"nft_id": request.NFTokenID,
		"transactions": []interface{}{
			// TODO: Load NFT transaction history
		},
		"validated": true,
	}

	return response, nil
}

func (m *NftHistoryMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *NftHistoryMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
