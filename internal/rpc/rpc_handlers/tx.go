package rpc_handlers

import (
	"encoding/hex"
	"encoding/json"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// TxMethod handles the tx RPC method
type TxMethod struct{}

func (m *TxMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		rpc_types.TransactionParam
		Binary    bool   `json:"binary,omitempty"`
		MinLedger uint32 `json:"min_ledger,omitempty"`
		MaxLedger uint32 `json:"max_ledger,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Transaction == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: transaction")
	}

	// Check if ledger service is available
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	// Parse the transaction hash
	txHashBytes, err := hex.DecodeString(request.Transaction)
	if err != nil || len(txHashBytes) != 32 {
		return nil, rpc_types.RpcErrorInvalidParams("Invalid transaction hash")
	}

	var txHash [32]byte
	copy(txHash[:], txHashBytes)

	// Look up the transaction
	txInfo, err := rpc_types.Services.Ledger.GetTransaction(txHash)
	if err != nil {
		return nil, &rpc_types.RpcError{
			Code:        -1,
			ErrorString: "txnNotFound",
			Message:     "Transaction not found",
		}
	}

	// Parse the stored transaction data
	// The stored data includes both the transaction and its metadata
	var storedTx StoredTransaction
	if err := json.Unmarshal(txInfo.TxData, &storedTx); err != nil {
		return nil, rpc_types.RpcErrorInternal("Failed to parse transaction data")
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

func (m *TxMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *TxMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
