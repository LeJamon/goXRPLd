package rpc

import (
	"encoding/hex"
	"encoding/json"
	"strings"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	crypto "github.com/LeJamon/goXRPLd/internal/crypto/common"
)

// TxMethod handles the tx RPC method
type TxMethod struct{}

func (m *TxMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		TransactionParam
		Binary    bool   `json:"binary,omitempty"`
		MinLedger uint32 `json:"min_ledger,omitempty"`
		MaxLedger uint32 `json:"max_ledger,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Transaction == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: transaction")
	}

	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Parse the transaction hash
	txHashBytes, err := hex.DecodeString(request.Transaction)
	if err != nil || len(txHashBytes) != 32 {
		return nil, RpcErrorInvalidParams("Invalid transaction hash")
	}

	var txHash [32]byte
	copy(txHash[:], txHashBytes)

	// Look up the transaction
	txInfo, err := Services.Ledger.GetTransaction(txHash)
	if err != nil {
		return nil, &RpcError{
			Code:        -1,
			ErrorString: "txnNotFound",
			Message:     "Transaction not found",
		}
	}

	// Parse the stored transaction data
	// The stored data includes both the transaction and its metadata
	var storedTx StoredTransaction
	if err := json.Unmarshal(txInfo.TxData, &storedTx); err != nil {
		return nil, RpcErrorInternal("Failed to parse transaction data")
	}

	// Build the response
	response := map[string]interface{}{}

	// If binary mode, return binary encoded transaction
	if request.Binary {
		// Encode transaction to binary
		txBlob, err := binarycodec.Encode(storedTx.TxJSON)
		if err == nil {
			response["tx_blob"] = txBlob
		}
		// Encode metadata to binary
		if storedTx.Meta != nil {
			metaBlob, err := binarycodec.Encode(storedTx.Meta)
			if err == nil {
				response["meta"] = metaBlob
			}
		}
	} else {
		// Return JSON format
		for k, v := range storedTx.TxJSON {
			response[k] = v
		}
		if storedTx.Meta != nil {
			response["meta"] = storedTx.Meta
		}
	}

	// Add ledger info
	response["hash"] = request.Transaction
	response["inLedger"] = txInfo.LedgerIndex
	response["ledger_index"] = txInfo.LedgerIndex
	response["ledger_hash"] = txInfo.LedgerHash
	response["validated"] = txInfo.Validated

	return response, nil
}

// StoredTransaction represents a transaction stored in the ledger
type StoredTransaction struct {
	TxJSON map[string]interface{} `json:"tx_json"`
	Meta   map[string]interface{} `json:"meta"`
}

func (m *TxMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *TxMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// TxHistoryMethod handles the tx_history RPC method
type TxHistoryMethod struct{}

func (m *TxHistoryMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		Start uint32 `json:"start,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Get transaction history from the ledger service
	result, err := Services.Ledger.GetTransactionHistory(request.Start)
	if err != nil {
		if err.Error() == "transaction history not available (no database configured)" {
			return nil, &RpcError{
				Code:    73, // lgrNotFound
				Message: "Transaction history not available. Database not configured.",
			}
		}
		return nil, RpcErrorInternal("Failed to get transaction history: " + err.Error())
	}

	// Build transactions array
	txs := make([]map[string]interface{}, len(result.Transactions))
	for i, tx := range result.Transactions {
		txs[i] = map[string]interface{}{
			"hash":         hex.EncodeToString(tx.Hash[:]),
			"ledger_index": tx.LedgerIndex,
			"tx_blob":      hex.EncodeToString(tx.TxBlob),
		}
	}

	response := map[string]interface{}{
		"index": result.Index,
		"txs":   txs,
	}

	return response, nil
}

func (m *TxHistoryMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *TxHistoryMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// SubmitMethod handles the submit RPC method
type SubmitMethod struct{}

func (m *SubmitMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		TxBlob     string          `json:"tx_blob,omitempty"`
		TxJson     json.RawMessage `json:"tx_json,omitempty"`
		Secret     string          `json:"secret,omitempty"`
		Seed       string          `json:"seed,omitempty"`
		SeedHex    string          `json:"seed_hex,omitempty"`
		Passphrase string          `json:"passphrase,omitempty"`
		KeyType    string          `json:"key_type,omitempty"`
		FailHard   bool            `json:"fail_hard,omitempty"`
		Offline    bool            `json:"offline,omitempty"`
		BuildPath  bool            `json:"build_path,omitempty"`
		FeeMultMax uint32          `json:"fee_mult_max,omitempty"`
		FeeDivMax  uint32          `json:"fee_div_max,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.TxBlob == "" && len(request.TxJson) == 0 {
		return nil, RpcErrorInvalidParams("Either tx_blob or tx_json must be provided")
	}

	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	var txJSON []byte

	if len(request.TxJson) > 0 {
		// Submit using tx_json
		txJSON = request.TxJson

		// TODO: If signing credentials are provided, sign the transaction first
		// For now, we assume the transaction is either pre-signed or we skip signature validation
		if request.Secret != "" || request.Seed != "" || request.SeedHex != "" || request.Passphrase != "" {
			// TODO: Implement signing
			// For now, just use the tx_json as-is
		}
	} else {
		// TODO: Submit using tx_blob - decode hex to transaction
		// For now, return error since we only support tx_json
		return nil, RpcErrorInvalidParams("tx_blob submission not yet implemented, use tx_json instead")
	}

	// Parse tx_json to include in response and calculate hash
	var txJsonMap map[string]interface{}
	if err := json.Unmarshal(txJSON, &txJsonMap); err != nil {
		txJsonMap = map[string]interface{}{}
	}

	// Submit the transaction
	result, err := Services.Ledger.SubmitTransaction(txJSON)
	if err != nil {
		return nil, RpcErrorInternal("Failed to submit transaction: " + err.Error())
	}

	// If transaction was applied, store it for later lookup
	var txHashStr string
	if result.Applied {
		// Encode transaction to get binary for hash calculation
		txBlob, encErr := binarycodec.Encode(txJsonMap)
		if encErr == nil {
			txHashStr = calculateTxHash(txBlob)

			// Parse the hash back to bytes
			if txHashBytes, err := hex.DecodeString(txHashStr); err == nil && len(txHashBytes) == 32 {
				var txHash [32]byte
				copy(txHash[:], txHashBytes)

				// Store the transaction with metadata
				storedTx := StoredTransaction{
					TxJSON: txJsonMap,
					Meta: map[string]interface{}{
						"TransactionResult": result.EngineResult,
						"TransactionIndex":  0,
					},
				}
				storedData, _ := json.Marshal(storedTx)
				_ = Services.Ledger.StoreTransaction(txHash, storedData)
			}
		}
	}

	// Get current fees for response
	baseFee, _, _ := Services.Ledger.GetCurrentFees()

	response := map[string]interface{}{
		"engine_result":          result.EngineResult,
		"engine_result_code":     result.EngineResultCode,
		"engine_result_message":  result.EngineResultMessage,
		"tx_json":                txJsonMap,
		"accepted":               result.Applied,
		"applied":                result.Applied,
		"broadcast":              result.Applied, // In standalone mode, no broadcast
		"kept":                   result.Applied,
		"queued":                 false,
		"open_ledger_cost":       baseFee,
		"validated_ledger_index": result.ValidatedLedger,
		"current_ledger_index":   result.CurrentLedger,
	}

	// Add hash if we calculated it
	if txHashStr != "" {
		response["tx_hash"] = txHashStr
		txJsonMap["hash"] = txHashStr
	}

	// Include result-specific status based on engine result
	if result.EngineResultCode == 0 { // tesSUCCESS
		response["status"] = "success"
	} else if result.EngineResultCode >= 100 && result.EngineResultCode < 200 { // tec codes
		response["status"] = "success" // tec codes are still "successful" submissions
	} else {
		response["status"] = "error"
	}

	return response, nil
}

func (m *SubmitMethod) RequiredRole() Role {
	return RoleUser // Transaction submission requires user privileges
}

func (m *SubmitMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// SubmitMultisignedMethod handles the submit_multisigned RPC method
type SubmitMultisignedMethod struct{}

func (m *SubmitMultisignedMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		TxJson    json.RawMessage `json:"tx_json"`
		FailHard  bool           `json:"fail_hard,omitempty"`
	}
	
	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}
	
	if len(request.TxJson) == 0 {
		return nil, RpcErrorInvalidParams("Missing required parameter: tx_json")
	}
	
	// TODO: Implement multisigned transaction submission
	// 1. Parse transaction JSON with multiple signatures
	// 2. Validate that account has SignerList configured
	// 3. Verify each signature against corresponding signer
	// 4. Check that enough valid signatures are provided (quorum)
	// 5. Submit transaction if signature validation passes
	// 6. Handle partial signature scenarios for debugging
	
	response := map[string]interface{}{
		"engine_result":         "tesSUCCESS", // TODO: Get actual result
		"engine_result_code":    0,
		"engine_result_message": "The transaction was applied. Only final in a validated ledger.",
		"tx_blob":               "GENERATED_BLOB", // TODO: Generate actual blob
		"tx_json": map[string]interface{}{
			// TODO: Return processed multisigned transaction
			"Signers": []interface{}{
				// TODO: Include actual signer information
			},
		},
		"accepted":   true,
		"applied":    true,
		"broadcast":  true,
		"kept":       true,
		"queued":     false,
		"validated_ledger_index": 1000,
	}
	
	return response, nil
}

func (m *SubmitMultisignedMethod) RequiredRole() Role {
	return RoleUser
}

func (m *SubmitMultisignedMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// SignMethod handles the sign RPC method
type SignMethod struct{}

func (m *SignMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
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
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if len(request.TxJson) == 0 {
		return nil, RpcErrorInvalidParams("Missing required parameter: tx_json")
	}

	// Check for signing credentials
	if request.Secret == "" && request.Seed == "" && request.SeedHex == "" && request.Passphrase == "" {
		return nil, RpcErrorInvalidParams("Missing signing credentials")
	}

	// Determine the seed to use
	seed := request.Seed
	if seed == "" {
		seed = request.Secret // secret is an alias for seed
	}
	if seed == "" && request.Passphrase != "" {
		// TODO: Derive seed from passphrase
		return nil, RpcErrorInvalidParams("Passphrase-based signing not yet implemented")
	}
	if seed == "" && request.SeedHex != "" {
		// TODO: Handle hex-encoded seed
		return nil, RpcErrorInvalidParams("Hex seed signing not yet implemented")
	}

	if seed == "" {
		return nil, RpcErrorInvalidParams("No valid signing credential provided")
	}

	// Derive keypair from seed
	privateKey, publicKey, err := tx.DeriveKeypairFromSeed(seed)
	if err != nil {
		return nil, RpcErrorInvalidParams("Failed to derive keypair: " + err.Error())
	}

	// Derive address from public key
	address, err := tx.DeriveAddressFromPublicKey(publicKey)
	if err != nil {
		return nil, RpcErrorInternal("Failed to derive address: " + err.Error())
	}

	// Parse the transaction JSON
	var txMap map[string]interface{}
	if err := json.Unmarshal(request.TxJson, &txMap); err != nil {
		return nil, RpcErrorInvalidParams("Invalid tx_json: " + err.Error())
	}

	// Verify the account matches the signing key
	if txAccount, ok := txMap["Account"].(string); ok {
		if txAccount != address {
			return nil, RpcErrorInvalidParams("Account in tx_json does not match signing key")
		}
	} else {
		// Set the account if not present
		txMap["Account"] = address
	}

	// Fill in missing fields if not offline
	if !request.Offline {
		// Set Fee if not present
		if _, ok := txMap["Fee"]; !ok {
			baseFee, _, _ := Services.Ledger.GetCurrentFees()
			txMap["Fee"] = string(rune(baseFee)) // Convert to string
		}

		// Set Sequence if not present
		if _, ok := txMap["Sequence"]; !ok {
			// Get account info to find current sequence
			info, err := Services.Ledger.GetAccountInfo(address, "current")
			if err != nil {
				return nil, RpcErrorInternal("Failed to get account sequence: " + err.Error())
			}
			txMap["Sequence"] = info.Sequence
		}

		// Set LastLedgerSequence if not present (current + 4)
		if _, ok := txMap["LastLedgerSequence"]; !ok {
			currentLedger := Services.Ledger.GetCurrentLedgerIndex()
			txMap["LastLedgerSequence"] = currentLedger + 4
		}
	}

	// Add the signing public key
	txMap["SigningPubKey"] = publicKey

	// Parse the transaction to get a proper Transaction object
	txBytes, err := json.Marshal(txMap)
	if err != nil {
		return nil, RpcErrorInternal("Failed to marshal transaction: " + err.Error())
	}

	transaction, err := tx.ParseJSON(txBytes)
	if err != nil {
		return nil, RpcErrorInvalidParams("Failed to parse transaction: " + err.Error())
	}

	// Update the common fields with signing key
	common := transaction.GetCommon()
	common.SigningPubKey = publicKey

	// Sign the transaction
	signature, err := tx.SignTransaction(transaction, privateKey)
	if err != nil {
		return nil, RpcErrorInternal("Failed to sign transaction: " + err.Error())
	}

	// Add signature to transaction
	txMap["TxnSignature"] = signature

	// Encode the transaction to binary
	txBlob, err := binarycodec.Encode(txMap)
	if err != nil {
		return nil, RpcErrorInternal("Failed to encode transaction: " + err.Error())
	}

	// Calculate transaction hash
	txHash := calculateTxHash(txBlob)

	// Add hash to response
	txMap["hash"] = txHash

	response := map[string]interface{}{
		"tx_blob": txBlob,
		"tx_json": txMap,
	}

	return response, nil
}

// calculateTxHash calculates the hash of a signed transaction
func calculateTxHash(txBlobHex string) string {
	// The transaction hash is SHA512Half of prefix + transaction blob
	// Prefix is "TXN\x00" = 0x54584E00
	prefix := []byte{0x54, 0x58, 0x4E, 0x00}

	txBytes, err := hex.DecodeString(txBlobHex)
	if err != nil {
		return ""
	}

	data := append(prefix, txBytes...)
	hash := crypto.Sha512Half(data)
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

func (m *SignMethod) RequiredRole() Role {
	return RoleUser // Signing requires user privileges
}

func (m *SignMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// SignForMethod handles the sign_for RPC method
type SignForMethod struct{}

func (m *SignForMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
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
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}
	
	if request.Account == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: account")
	}
	
	if len(request.TxJson) == 0 {
		return nil, RpcErrorInvalidParams("Missing required parameter: tx_json")
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

func (m *SignForMethod) RequiredRole() Role {
	return RoleUser
}

func (m *SignForMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// TransactionEntryMethod handles the transaction_entry RPC method
type TransactionEntryMethod struct{}

func (m *TransactionEntryMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		TxHash      string `json:"tx_hash"`
		LedgerHash  string `json:"ledger_hash,omitempty"`
		LedgerIndex string `json:"ledger_index,omitempty"`
	}
	
	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}
	
	if request.TxHash == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: tx_hash")
	}
	
	// TODO: Implement transaction entry lookup from specific ledger
	// 1. Determine target ledger (hash or index)
	// 2. Look up transaction in the specified ledger only
	// 3. Return transaction data with metadata from that specific ledger
	// 4. This is different from 'tx' method which searches across ledger range
	
	response := map[string]interface{}{
		"ledger_hash":  "PLACEHOLDER_LEDGER_HASH", // TODO: Use actual ledger hash
		"ledger_index": 1000, // TODO: Use actual ledger index
		"metadata": map[string]interface{}{
			"AffectedNodes":      []interface{}{}, // TODO: Load actual metadata
			"TransactionIndex":   0,
			"TransactionResult":  "tesSUCCESS",
		},
		"tx_json": map[string]interface{}{
			// TODO: Load actual transaction JSON
			"Account":         "rAccount...",
			"TransactionType": "Payment",
			"hash":           request.TxHash,
		},
		"validated": true,
	}
	
	return response, nil
}

func (m *TransactionEntryMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *TransactionEntryMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}