package rpc_handlers

import (
	"encoding/json"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// SubmitMultisignedMethod handles the submit_multisigned RPC method
// This submits a multi-signed transaction to the network
type SubmitMultisignedMethod struct{}

func (m *SubmitMultisignedMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		TxJson   json.RawMessage `json:"tx_json"`
		FailHard bool            `json:"fail_hard,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if len(request.TxJson) == 0 {
		return nil, rpc_types.RpcErrorMissingField("tx_json")
	}

	// Check if ledger service is available
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	// Parse the transaction JSON
	var txMap map[string]interface{}
	if err := json.Unmarshal(request.TxJson, &txMap); err != nil {
		return nil, rpc_types.RpcErrorInvalidParams("Invalid tx_json: " + err.Error())
	}

	// Validate required fields for multi-signed transaction
	if _, ok := txMap["Account"]; !ok {
		return nil, rpc_types.RpcErrorMissingField("Account")
	}

	// Check that SigningPubKey is empty (required for multi-signed transactions)
	if signingPubKey, ok := txMap["SigningPubKey"].(string); !ok || signingPubKey != "" {
		return nil, rpc_types.RpcErrorInvalidParams("Multi-signed transactions must have empty SigningPubKey")
	}

	// Check that Signers array exists and is not empty
	signers, ok := txMap["Signers"].([]interface{})
	if !ok || len(signers) == 0 {
		return nil, rpc_types.RpcErrorInvalidParams("Multi-signed transaction must have at least one Signer")
	}

	// Validate signer entries
	var prevAccount string
	for i, signerEntry := range signers {
		signerWrapper, ok := signerEntry.(map[string]interface{})
		if !ok {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid Signer entry format")
		}

		signer, ok := signerWrapper["Signer"].(map[string]interface{})
		if !ok {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid Signer entry format")
		}

		// Check required signer fields
		account, ok := signer["Account"].(string)
		if !ok || account == "" {
			return nil, rpc_types.RpcErrorInvalidParams("Signer entry missing Account")
		}

		if _, ok := signer["SigningPubKey"].(string); !ok {
			return nil, rpc_types.RpcErrorInvalidParams("Signer entry missing SigningPubKey")
		}

		if _, ok := signer["TxnSignature"].(string); !ok {
			return nil, rpc_types.RpcErrorInvalidParams("Signer entry missing TxnSignature")
		}

		// Check signers are sorted by account (XRPL protocol requirement)
		if i > 0 && account < prevAccount {
			return nil, rpc_types.RpcErrorInvalidParams("Signers must be sorted by Account")
		}
		prevAccount = account
	}

	// Encode the transaction to binary
	txBlob, err := binarycodec.Encode(txMap)
	if err != nil {
		return nil, rpc_types.RpcErrorInternal("Failed to encode transaction: " + err.Error())
	}

	// Calculate transaction hash
	txHash := CalculateTxHash(txBlob)

	// Submit the transaction
	txJSON, err := json.Marshal(txMap)
	if err != nil {
		return nil, rpc_types.RpcErrorInternal("Failed to marshal transaction: " + err.Error())
	}

	result, err := rpc_types.Services.Ledger.SubmitTransaction(txJSON)
	if err != nil {
		// Return submission error with details
		return nil, rpc_types.RpcErrorInternal("Transaction submission failed: " + err.Error())
	}

	// Add hash to response tx_json
	txMap["hash"] = txHash

	// Build response
	response := map[string]interface{}{
		"engine_result":         result.EngineResult,
		"engine_result_code":    result.EngineResultCode,
		"engine_result_message": result.EngineResultMessage,
		"tx_blob":               txBlob,
		"tx_json":               txMap,
	}

	// Add applied status from result
	if result.Applied {
		response["applied"] = result.Applied
	}

	return response, nil
}

func (m *SubmitMultisignedMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleUser
}

func (m *SubmitMultisignedMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
