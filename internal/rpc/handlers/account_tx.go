package handlers

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	binarycodec "github.com/LeJamon/goXRPLd/codec/binarycodec"
	"github.com/LeJamon/goXRPLd/internal/rpc/types"
)

// AccountTxMethod handles the account_tx RPC method
type AccountTxMethod struct{ BaseHandler }

func (m *AccountTxMethod) Handle(ctx *types.RpcContext, params json.RawMessage) (interface{}, *types.RpcError) {
	var request struct {
		types.AccountParam
		LedgerIndexMin int32  `json:"ledger_index_min,omitempty"`
		LedgerIndexMax int32  `json:"ledger_index_max,omitempty"`
		LedgerHash     string `json:"ledger_hash,omitempty"`
		LedgerIndex    string `json:"ledger_index,omitempty"`
		Binary         bool   `json:"binary,omitempty"`
		Forward        bool   `json:"forward,omitempty"`
		types.PaginationParams
	}

	if err := ParseParams(params, &request); err != nil {
		return nil, err
	}

	if err := ValidateAccount(request.Account); err != nil {
		return nil, err
	}

	// Validate the account address format
	if !types.IsValidXRPLAddress(request.Account) {
		return nil, &types.RpcError{Code: 35, Message: "Account malformed."}
	}

	if err := RequireLedgerService(); err != nil {
		return nil, err
	}

	// Parse marker if provided
	var marker *types.AccountTxMarker
	if request.Marker != nil {
		if markerMap, ok := request.Marker.(map[string]interface{}); ok {
			marker = &types.AccountTxMarker{}
			if ledger, ok := markerMap["ledger"].(float64); ok {
				marker.LedgerSeq = uint32(ledger)
			}
			if seq, ok := markerMap["seq"].(float64); ok {
				marker.TxnSeq = uint32(seq)
			}
		}
	}

	result, err := types.Services.Ledger.GetAccountTransactions(
		request.Account,
		int64(request.LedgerIndexMin),
		int64(request.LedgerIndexMax),
		request.Limit,
		marker,
		request.Forward,
	)
	if err != nil {
		if err.Error() == "transaction history not available (no database configured)" {
			return nil, &types.RpcError{
				Code:    73,
				Message: "Transaction history not available. Database not configured.",
			}
		}
		if err.Error() == "account not found" {
			return nil, &types.RpcError{
				Code:    19,
				Message: "Account not found.",
			}
		}
		return nil, types.RpcErrorInternal("Failed to get account transactions: " + err.Error())
	}

	// Cache for ledger lookups by sequence, to avoid repeated lookups
	// for transactions in the same ledger.
	type ledgerCacheEntry struct {
		hash         [32]byte
		closeTimeSec int64
		found        bool
	}
	ledgerCache := make(map[uint32]*ledgerCacheEntry)

	lookupLedger := func(seq uint32) *ledgerCacheEntry {
		if entry, ok := ledgerCache[seq]; ok {
			return entry
		}
		entry := &ledgerCacheEntry{}
		ledger, lookupErr := types.Services.Ledger.GetLedgerBySequence(seq)
		if lookupErr == nil && ledger != nil {
			entry.hash = ledger.Hash()
			entry.closeTimeSec = ledger.CloseTime()
			entry.found = true
		}
		ledgerCache[seq] = entry
		return entry
	}

	// Build transactions array
	transactions := make([]map[string]interface{}, len(result.Transactions))
	for i, tx := range result.Transactions {
		txEntry := map[string]interface{}{
			"validated": true,
		}

		txHashHex := strings.ToUpper(hex.EncodeToString(tx.Hash[:]))

		if request.Binary {
			// Binary mode (API v2): tx_blob, meta_blob, ledger_index, validated
			txEntry["tx_blob"] = strings.ToUpper(hex.EncodeToString(tx.TxBlob))
			txEntry["meta_blob"] = strings.ToUpper(hex.EncodeToString(tx.Meta))
			txEntry["ledger_index"] = tx.LedgerIndex
		} else {
			// JSON mode (API v2): tx_json, hash, ledger_index, ledger_hash, close_time_iso, meta

			// Decode tx_blob into JSON
			txBlobHex := hex.EncodeToString(tx.TxBlob)
			txJSON, decErr := binarycodec.Decode(txBlobHex)
			if decErr != nil {
				// Fallback to hex if decode fails
				txEntry["tx_blob"] = strings.ToUpper(txBlobHex)
			} else {
				// Add fields inside tx_json
				txJSON["ledger_index"] = tx.LedgerIndex
				txJSON["hash"] = txHashHex

				// Look up containing ledger for date and CTID
				ledgerInfo := lookupLedger(tx.LedgerIndex)
				if ledgerInfo.found && ledgerInfo.closeTimeSec > 0 {
					txJSON["date"] = ledgerInfo.closeTimeSec
				}

				// Add CTID inside tx_json
				txJSON["ctid"] = encodeCTID(tx.LedgerIndex, uint16(tx.TxnSeq))

				// Inject DeliveredAmount for Payment transactions
				InjectDeliveredAmount(txJSON, nil)

				txEntry["tx_json"] = txJSON
			}

			// Decode metadata
			metaHex := hex.EncodeToString(tx.Meta)
			metaJSON, metaErr := binarycodec.Decode(metaHex)
			if metaErr != nil {
				txEntry["meta"] = strings.ToUpper(metaHex)
			} else {
				// Inject DeliveredAmount into metadata if this is a Payment
				if txJSON, ok := txEntry["tx_json"].(map[string]interface{}); ok {
					InjectDeliveredAmount(txJSON, metaJSON)
				}
				txEntry["meta"] = metaJSON
			}

			// Add entry-level fields (API v2)
			txEntry["hash"] = txHashHex
			txEntry["ledger_index"] = tx.LedgerIndex

			// Look up containing ledger for ledger_hash and close_time_iso
			ledgerInfo := lookupLedger(tx.LedgerIndex)
			if ledgerInfo.found {
				txEntry["ledger_hash"] = strings.ToUpper(hex.EncodeToString(ledgerInfo.hash[:]))
				if ledgerInfo.closeTimeSec > 0 {
					closeTime := rippleEpochTime.Add(time.Duration(ledgerInfo.closeTimeSec) * time.Second)
					txEntry["close_time_iso"] = closeTime.UTC().Format("2006-01-02T15:04:05Z")
				}
			}
		}

		transactions[i] = txEntry
	}

	response := map[string]interface{}{
		"account":          result.Account,
		"ledger_index_min": result.LedgerMin,
		"ledger_index_max": result.LedgerMax,
		"limit":            result.Limit,
		"transactions":     transactions,
		"validated":        result.Validated,
	}

	if result.Marker != nil {
		response["marker"] = map[string]interface{}{
			"ledger": result.Marker.LedgerSeq,
			"seq":    result.Marker.TxnSeq,
		}
	}

	return response, nil
}

