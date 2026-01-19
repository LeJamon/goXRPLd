package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// AccountNftsMethod handles the account_nfts RPC method
type AccountNftsMethod struct{}

func (m *AccountNftsMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		rpc_types.AccountParam
		rpc_types.LedgerSpecifier
		rpc_types.PaginationParams
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Account == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: account")
	}

	// TODO: Implement NFT retrieval
	// 1. Validate account address
	// 2. Determine target ledger
	// 3. Find all NFTokenPage objects owned by the account
	// 4. Extract individual NFTs from the pages
	// 5. Apply pagination using marker and limit
	// 6. Return NFT details including token ID, issuer, and metadata

	response := map[string]interface{}{
		"account":      request.Account,
		"account_nfts": []interface{}{
			// TODO: Load actual NFTs
			// Each NFT should have structure:
			// {
			//   "Flags": 0,
			//   "Issuer": "rIssuer...",
			//   "NFTokenID": "TOKEN_ID",
			//   "NFTokenTaxon": 0,
			//   "URI": "URI_HEX",
			//   "nft_serial": 1
			// }
		},
		"ledger_hash":  "PLACEHOLDER_LEDGER_HASH",
		"ledger_index": 1000,
		"validated":    true,
	}

	return response, nil
}

func (m *AccountNftsMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *AccountNftsMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
