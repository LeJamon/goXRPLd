package rpc_handlers

import (
	"encoding/json"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// SignMethod handles the sign RPC method
type SignMethod struct{}

func (m *SignMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		TxJson     json.RawMessage `json:"tx_json"`
		Secret     string          `json:"secret,omitempty"`
		Seed       string          `json:"seed,omitempty"`
		SeedHex    string          `json:"seed_hex,omitempty"`
		Passphrase string          `json:"passphrase,omitempty"`
		KeyType    string          `json:"key_type,omitempty"`
		Offline    bool            `json:"offline,omitempty"`
		BuildPath  bool            `json:"build_path,omitempty"`
		FeeMultMax uint32          `json:"fee_mult_max,omitempty"`
		FeeDivMax  uint32          `json:"fee_div_max,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if len(request.TxJson) == 0 {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: tx_json")
	}

	// Check for signing credentials
	if request.Secret == "" && request.Seed == "" && request.SeedHex == "" && request.Passphrase == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing signing credentials")
	}

	// Check if ledger service is available
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	// Determine the seed to use
	seed := request.Seed
	if seed == "" {
		seed = request.Secret // secret is an alias for seed
	}
	if seed == "" && request.Passphrase != "" {
		// TODO: Derive seed from passphrase
		return nil, rpc_types.RpcErrorInvalidParams("Passphrase-based signing not yet implemented")
	}
	if seed == "" && request.SeedHex != "" {
		// TODO: Handle hex-encoded seed
		return nil, rpc_types.RpcErrorInvalidParams("Hex seed signing not yet implemented")
	}

	if seed == "" {
		return nil, rpc_types.RpcErrorInvalidParams("No valid signing credential provided")
	}

	// Derive keypair from seed
	privateKey, publicKey, err := tx.DeriveKeypairFromSeed(seed)
	if err != nil {
		return nil, rpc_types.RpcErrorInvalidParams("Failed to derive keypair: " + err.Error())
	}

	// Derive address from public key
	address, err := tx.DeriveAddressFromPublicKey(publicKey)
	if err != nil {
		return nil, rpc_types.RpcErrorInternal("Failed to derive address: " + err.Error())
	}

	// Parse the transaction JSON
	var txMap map[string]interface{}
	if err := json.Unmarshal(request.TxJson, &txMap); err != nil {
		return nil, rpc_types.RpcErrorInvalidParams("Invalid tx_json: " + err.Error())
	}

	// Verify the account matches the signing key
	if txAccount, ok := txMap["Account"].(string); ok {
		if txAccount != address {
			return nil, rpc_types.RpcErrorInvalidParams("Account in tx_json does not match signing key")
		}
	} else {
		// Set the account if not present
		txMap["Account"] = address
	}

	// Fill in missing fields if not offline
	if !request.Offline {
		// Set Fee if not present
		if _, ok := txMap["Fee"]; !ok {
			baseFee, _, _ := rpc_types.Services.Ledger.GetCurrentFees()
			txMap["Fee"] = string(rune(baseFee)) // Convert to string
		}

		// Set Sequence if not present
		if _, ok := txMap["Sequence"]; !ok {
			// Get account info to find current sequence
			info, err := rpc_types.Services.Ledger.GetAccountInfo(address, "current")
			if err != nil {
				return nil, rpc_types.RpcErrorInternal("Failed to get account sequence: " + err.Error())
			}
			txMap["Sequence"] = info.Sequence
		}

		// Set LastLedgerSequence if not present (current + 4)
		if _, ok := txMap["LastLedgerSequence"]; !ok {
			currentLedger := rpc_types.Services.Ledger.GetCurrentLedgerIndex()
			txMap["LastLedgerSequence"] = currentLedger + 4
		}
	}

	// Add the signing public key
	txMap["SigningPubKey"] = publicKey

	// Parse the transaction to get a proper Transaction object
	txBytes, err := json.Marshal(txMap)
	if err != nil {
		return nil, rpc_types.RpcErrorInternal("Failed to marshal transaction: " + err.Error())
	}

	transaction, err := tx.ParseJSON(txBytes)
	if err != nil {
		return nil, rpc_types.RpcErrorInvalidParams("Failed to parse transaction: " + err.Error())
	}

	// Update the common fields with signing key
	common := transaction.GetCommon()
	common.SigningPubKey = publicKey

	// Sign the transaction
	signature, err := tx.SignTransaction(transaction, privateKey)
	if err != nil {
		return nil, rpc_types.RpcErrorInternal("Failed to sign transaction: " + err.Error())
	}

	// Add signature to transaction
	txMap["TxnSignature"] = signature

	// Encode the transaction to binary
	txBlob, err := binarycodec.Encode(txMap)
	if err != nil {
		return nil, rpc_types.RpcErrorInternal("Failed to encode transaction: " + err.Error())
	}

	// Calculate transaction hash
	txHash := CalculateTxHash(txBlob)

	// Add hash to response
	txMap["hash"] = txHash

	response := map[string]interface{}{
		"tx_blob": txBlob,
		"tx_json": txMap,
	}

	return response, nil
}

func (m *SignMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleUser // Signing requires user privileges
}

func (m *SignMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
