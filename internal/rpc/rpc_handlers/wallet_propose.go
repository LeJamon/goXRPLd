package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// WalletProposeMethod handles the wallet_propose RPC method
type WalletProposeMethod struct{}

func (m *WalletProposeMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		Seed       string `json:"seed,omitempty"`
		SeedHex    string `json:"seed_hex,omitempty"`
		Passphrase string `json:"passphrase,omitempty"`
		KeyType    string `json:"key_type,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// Default key type to secp256k1 if not specified
	if request.KeyType == "" {
		request.KeyType = "secp256k1"
	}

	// Validate key type
	if request.KeyType != "secp256k1" && request.KeyType != "ed25519" {
		return nil, rpc_types.RpcErrorInvalidParams("key_type must be 'secp256k1' or 'ed25519'")
	}

	// TODO: Implement wallet generation
	// 1. Generate or use provided seed:
	//    - If seed provided: validate and use it
	//    - If seed_hex provided: decode and use it
	//    - If passphrase provided: derive seed from passphrase
	//    - If none provided: generate random seed
	// 2. Derive master key from seed using specified algorithm
	// 3. Generate public key from private key
	// 4. Calculate XRPL account address from public key
	// 5. Return all wallet components for account creation
	//
	// Security considerations:
	// - Use cryptographically secure random generation
	// - Follow XRPL key derivation standards
	// - Never log or persist private keys
	// - Validate entropy of provided seeds

	response := map[string]interface{}{
		"account_id":      "rGeneratedAccount...", // TODO: Generate actual account
		"key_type":        request.KeyType,
		"master_key":      "MASTER_KEY",      // TODO: Generate actual master key
		"master_seed":     "sSEED...",        // TODO: Generate actual seed
		"master_seed_hex": "HEX_SEED",        // TODO: Convert seed to hex
		"public_key":      "PUBLIC_KEY_HEX",  // TODO: Generate actual public key
		"public_key_hex":  "PUBLIC_KEY_HEX",  // TODO: Same as public_key
	}

	return response, nil
}

func (m *WalletProposeMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest // Wallet generation is publicly available
}

func (m *WalletProposeMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
