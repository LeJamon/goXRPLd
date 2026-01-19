package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// SignForMethod handles the sign_for RPC method
type SignForMethod struct{}

func (m *SignForMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		Account    string          `json:"account"`
		TxJson     json.RawMessage `json:"tx_json"`
		Secret     string          `json:"secret,omitempty"`
		Seed       string          `json:"seed,omitempty"`
		SeedHex    string          `json:"seed_hex,omitempty"`
		Passphrase string          `json:"passphrase,omitempty"`
		KeyType    string          `json:"key_type,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Account == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: account")
	}

	if len(request.TxJson) == 0 {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: tx_json")
	}

	// TODO: Implement multisigning
	// 1. Parse transaction JSON
	// 2. Verify that the specified account has SignerList configured
	// 3. Derive signing key from provided credentials
	// 4. Verify that signing key corresponds to one of the authorized signers
	// 5. Create signature for the transaction on behalf of the account
	// 6. Return transaction with additional signature in Signers array
	// 7. Handle cases where transaction already has other signatures

	response := map[string]interface{}{
		"tx_blob": "MULTISIGNED_TRANSACTION_HEX", // TODO: Generate actual blob
		"tx_json": map[string]interface{}{
			// TODO: Return transaction with additional signature
			"Account": request.Account,
			"Signers": []interface{}{
				map[string]interface{}{
					"Signer": map[string]interface{}{
						"Account":       "rSigner...", // TODO: Get signer account
						"SigningPubKey": "PUBLIC_KEY", // TODO: Get signing public key
						"TxnSignature":  "SIGNATURE",  // TODO: Generate signature
					},
				},
			},
		},
	}

	return response, nil
}

func (m *SignForMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleUser
}

func (m *SignForMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
