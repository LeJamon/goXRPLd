package rpc

import (
	"encoding/hex"
	"encoding/json"
)

// formatLedgerHash formats a 32-byte hash as uppercase hex string
func formatLedgerHash(hash [32]byte) string {
	return hex.EncodeToString(hash[:])
}

// AccountInfoMethod handles the account_info RPC method
type AccountInfoMethod struct{}

func (m *AccountInfoMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Parse parameters
	var request struct {
		AccountParam
		LedgerSpecifier
		Queue       bool `json:"queue,omitempty"`
		SignerLists bool `json:"signer_lists,omitempty"`
		Strict      bool `json:"strict,omitempty"`
	}
	println("PARAMS:", params)

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}
	println("PARAMS:", params)

	if request.Account == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: account")
	}

	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Determine ledger index to use
	ledgerIndex := "current"
	if request.LedgerIndex != "" {
		ledgerIndex = request.LedgerIndex
	} else if request.LedgerHash != "" {
		// TODO: Support lookup by hash
		ledgerIndex = "validated"
	}

	// Get account info from the ledger
	info, err := Services.Ledger.GetAccountInfo(request.Account, ledgerIndex)
	if err != nil {
		// Check for specific error types
		if err.Error() == "account not found" {
			return nil, &RpcError{
				Code:    19, // actNotFound
				Message: "Account not found.",
			}
		}
		return nil, RpcErrorInternal("Failed to get account info: " + err.Error())
	}

	// Build account_data response
	accountData := map[string]interface{}{
		"Account":         info.Account,
		"Balance":         info.Balance,
		"Flags":           info.Flags,
		"LedgerEntryType": "AccountRoot",
		"OwnerCount":      info.OwnerCount,
		"Sequence":        info.Sequence,
	}

	// Add optional fields if present
	if info.RegularKey != "" {
		accountData["RegularKey"] = info.RegularKey
	}
	if info.Domain != "" {
		accountData["Domain"] = info.Domain
	}
	if info.EmailHash != "" {
		accountData["EmailHash"] = info.EmailHash
	}
	if info.TransferRate > 0 {
		accountData["TransferRate"] = info.TransferRate
	}
	if info.TickSize > 0 {
		accountData["TickSize"] = info.TickSize
	}

	response := map[string]interface{}{
		"account_data": accountData,
		"ledger_hash":  info.LedgerHash,
		"ledger_index": info.LedgerIndex,
		"validated":    info.Validated,
	}

	// Add queue data if requested and this is current ledger
	if request.Queue && ledgerIndex == "current" {
		response["queue_data"] = map[string]interface{}{
			"auth_change_queued":    false,
			"highest_sequence":      info.Sequence,
			"lowest_sequence":       info.Sequence,
			"max_spend_drops_total": info.Balance,
			"transactions":          []interface{}{},
			"txn_count":             0,
		}
	}

	// Add signer lists if requested
	if request.SignerLists {
		response["signer_lists"] = []interface{}{
			// TODO: Load actual signer lists from ledger
		}
	}

	return response, nil
}

func (m *AccountInfoMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *AccountInfoMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// AccountChannelsMethod handles the account_channels RPC method
type AccountChannelsMethod struct{}

func (m *AccountChannelsMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		AccountParam
		LedgerSpecifier
		DestinationAccount string `json:"destination_account,omitempty"`
		PaginationParams
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Account == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: account")
	}

	// TODO: Implement payment channel retrieval
	// 1. Validate account address
	// 2. Determine target ledger
	// 3. Find all PayChannel objects where account is source or destination
	// 4. Filter by destination_account if provided
	// 5. Apply pagination using marker and limit
	// 6. Return channel details including balances and expiration

	response := map[string]interface{}{
		"account":  request.Account,
		"channels": []interface{}{
			// TODO: Load actual payment channels
			// Each channel should have structure:
			// {
			//   "account": "rSource...",
			//   "amount": "1000000000",
			//   "balance": "0",
			//   "channel_id": "CHANNEL_ID",
			//   "destination_account": "rDest...",
			//   "expiration": 12345678,
			//   "public_key": "PUBLIC_KEY",
			//   "public_key_hex": "HEX_KEY",
			//   "settle_delay": 3600
			// }
		},
		"ledger_hash":  "PLACEHOLDER_LEDGER_HASH",
		"ledger_index": 1000,
		"validated":    true,
	}

	return response, nil
}

func (m *AccountChannelsMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *AccountChannelsMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// AccountCurrenciesMethod handles the account_currencies RPC method
type AccountCurrenciesMethod struct{}

func (m *AccountCurrenciesMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		AccountParam
		LedgerSpecifier
		Strict bool `json:"strict,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Account == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: account")
	}

	// TODO: Implement currency retrieval
	// 1. Validate account address
	// 2. Determine target ledger
	// 3. Find all RippleState objects for the account
	// 4. Extract unique currencies that the account can send/receive
	// 5. Separate into send_currencies and receive_currencies
	// 6. Handle strict mode (only currencies with positive balance/trust)

	response := map[string]interface{}{
		"ledger_hash":        "PLACEHOLDER_LEDGER_HASH",
		"ledger_index":       1000,
		"receive_currencies": []string{
			// TODO: Load actual receivable currencies
			// Example: ["USD", "EUR", "BTC"]
		},
		"send_currencies": []string{
			// TODO: Load actual sendable currencies
			// Example: ["USD", "EUR"]
		},
		"validated": true,
	}

	return response, nil
}

func (m *AccountCurrenciesMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *AccountCurrenciesMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// AccountLinesMethod handles the account_lines RPC method
type AccountLinesMethod struct{}

func (m *AccountLinesMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		AccountParam
		LedgerSpecifier
		Peer string `json:"peer,omitempty"`
		PaginationParams
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Account == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: account")
	}

	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Determine ledger index to use
	ledgerIndex := "current"
	if request.LedgerIndex != "" {
		ledgerIndex = request.LedgerIndex
	}

	// Get account lines from the ledger service
	result, err := Services.Ledger.GetAccountLines(request.Account, ledgerIndex, request.Peer, request.Limit)
	if err != nil {
		if err.Error() == "account not found" {
			return nil, &RpcError{
				Code:    19, // actNotFound
				Message: "Account not found.",
			}
		}
		return nil, RpcErrorInternal("Failed to get account lines: " + err.Error())
	}

	// Build response
	response := map[string]interface{}{
		"account":      result.Account,
		"lines":        result.Lines,
		"ledger_hash":  formatLedgerHash(result.LedgerHash),
		"ledger_index": result.LedgerIndex,
		"validated":    result.Validated,
	}

	if result.Marker != "" {
		response["marker"] = result.Marker
	}

	return response, nil
}

func (m *AccountLinesMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *AccountLinesMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// AccountNftsMethod handles the account_nfts RPC method
type AccountNftsMethod struct{}

func (m *AccountNftsMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		AccountParam
		LedgerSpecifier
		PaginationParams
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Account == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: account")
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

func (m *AccountNftsMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *AccountNftsMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// AccountObjectsMethod handles the account_objects RPC method
type AccountObjectsMethod struct{}

func (m *AccountObjectsMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		AccountParam
		LedgerSpecifier
		Type                 string `json:"type,omitempty"`
		DeletionBlockersOnly bool   `json:"deletion_blockers_only,omitempty"`
		PaginationParams
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Account == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: account")
	}

	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Determine ledger index to use
	ledgerIndex := "current"
	if request.LedgerIndex != "" {
		ledgerIndex = request.LedgerIndex
	}

	// Get account objects from the ledger service
	result, err := Services.Ledger.GetAccountObjects(request.Account, ledgerIndex, request.Type, request.Limit)
	if err != nil {
		if err.Error() == "account not found" {
			return nil, &RpcError{
				Code:    19, // actNotFound
				Message: "Account not found.",
			}
		}
		return nil, RpcErrorInternal("Failed to get account objects: " + err.Error())
	}

	// Build account_objects array
	objects := make([]map[string]interface{}, len(result.AccountObjects))
	for i, obj := range result.AccountObjects {
		objects[i] = map[string]interface{}{
			"index":           obj.Index,
			"LedgerEntryType": obj.LedgerEntryType,
			"data":            hex.EncodeToString(obj.Data),
		}
	}

	response := map[string]interface{}{
		"account":         result.Account,
		"account_objects": objects,
		"ledger_hash":     formatLedgerHash(result.LedgerHash),
		"ledger_index":    result.LedgerIndex,
		"validated":       result.Validated,
	}

	if result.Marker != "" {
		response["marker"] = result.Marker
	}

	return response, nil
}

func (m *AccountObjectsMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *AccountObjectsMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// AccountOffersMethod handles the account_offers RPC method
type AccountOffersMethod struct{}

func (m *AccountOffersMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		AccountParam
		LedgerSpecifier
		Strict bool `json:"strict,omitempty"`
		PaginationParams
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Account == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: account")
	}

	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Determine ledger index to use
	ledgerIndex := "current"
	if request.LedgerIndex != "" {
		ledgerIndex = request.LedgerIndex
	}

	// Get account offers from the ledger service
	result, err := Services.Ledger.GetAccountOffers(request.Account, ledgerIndex, request.Limit)
	if err != nil {
		if err.Error() == "account not found" {
			return nil, &RpcError{
				Code:    19, // actNotFound
				Message: "Account not found.",
			}
		}
		return nil, RpcErrorInternal("Failed to get account offers: " + err.Error())
	}

	// Build response
	response := map[string]interface{}{
		"account":      result.Account,
		"offers":       result.Offers,
		"ledger_hash":  formatLedgerHash(result.LedgerHash),
		"ledger_index": result.LedgerIndex,
		"validated":    result.Validated,
	}

	if result.Marker != "" {
		response["marker"] = result.Marker
	}

	return response, nil
}

func (m *AccountOffersMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *AccountOffersMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// AccountTxMethod handles the account_tx RPC method
type AccountTxMethod struct{}

func (m *AccountTxMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		AccountParam
		LedgerIndexMin int32  `json:"ledger_index_min,omitempty"`
		LedgerIndexMax int32  `json:"ledger_index_max,omitempty"`
		LedgerHash     string `json:"ledger_hash,omitempty"`
		LedgerIndex    string `json:"ledger_index,omitempty"`
		Binary         bool   `json:"binary,omitempty"`
		Forward        bool   `json:"forward,omitempty"`
		PaginationParams
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Account == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: account")
	}

	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Parse marker if provided
	var marker *AccountTxMarker
	if request.Marker != nil {
		if markerMap, ok := request.Marker.(map[string]interface{}); ok {
			marker = &AccountTxMarker{}
			if ledger, ok := markerMap["ledger"].(float64); ok {
				marker.LedgerSeq = uint32(ledger)
			}
			if seq, ok := markerMap["seq"].(float64); ok {
				marker.TxnSeq = uint32(seq)
			}
		}
	}

	// Get account transactions from the ledger service
	result, err := Services.Ledger.GetAccountTransactions(
		request.Account,
		int64(request.LedgerIndexMin),
		int64(request.LedgerIndexMax),
		request.Limit,
		marker,
		request.Forward,
	)
	if err != nil {
		if err.Error() == "transaction history not available (no database configured)" {
			return nil, &RpcError{
				Code:    73, // lgrNotFound
				Message: "Transaction history not available. Database not configured.",
			}
		}
		if err.Error() == "account not found" {
			return nil, &RpcError{
				Code:    19, // actNotFound
				Message: "Account not found.",
			}
		}
		return nil, RpcErrorInternal("Failed to get account transactions: " + err.Error())
	}

	// Build transactions array
	transactions := make([]map[string]interface{}, len(result.Transactions))
	for i, tx := range result.Transactions {
		txEntry := map[string]interface{}{
			"ledger_index": tx.LedgerIndex,
			"validated":    true,
		}
		if request.Binary {
			txEntry["tx_blob"] = hex.EncodeToString(tx.TxBlob)
			txEntry["meta"] = hex.EncodeToString(tx.Meta)
		} else {
			// Parse tx_blob and meta as JSON if not binary
			txEntry["tx_blob"] = hex.EncodeToString(tx.TxBlob)
			txEntry["meta"] = hex.EncodeToString(tx.Meta)
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

func (m *AccountTxMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *AccountTxMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// GatewayBalancesMethod handles the gateway_balances RPC method
type GatewayBalancesMethod struct{}

func (m *GatewayBalancesMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		AccountParam
		LedgerSpecifier
		Strict    bool     `json:"strict,omitempty"`
		HotWallet []string `json:"hotwallet,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Account == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: account")
	}

	// TODO: Implement gateway balance calculation
	// 1. Validate account address (should be a gateway/issuer)
	// 2. Determine target ledger
	// 3. Find all RippleState objects where account is the issuer
	// 4. Calculate total issued amounts by currency
	// 5. Separate hot wallet balances if hot wallet addresses provided
	// 6. Calculate net balances and obligations

	response := map[string]interface{}{
		"account":     request.Account,
		"obligations": map[string]interface{}{
			// TODO: Calculate actual obligations by currency
			// Example:
			// "USD": "12345.67",
			// "EUR": "9876.54"
		},
		"balances": map[string]interface{}{
			// TODO: Calculate actual balances
		},
		"assets": map[string]interface{}{
			// TODO: Calculate assets (positive balances)
		},
		"ledger_hash":  "PLACEHOLDER_LEDGER_HASH",
		"ledger_index": 1000,
		"validated":    true,
	}

	return response, nil
}

func (m *GatewayBalancesMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *GatewayBalancesMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// NoRippleCheckMethod handles the noripple_check RPC method
type NoRippleCheckMethod struct{}

func (m *NoRippleCheckMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		AccountParam
		LedgerSpecifier
		Role         string `json:"role,omitempty"` // "gateway" or "user"
		Transactions bool   `json:"transactions,omitempty"`
		Limit        uint32 `json:"limit,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.Account == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: account")
	}

	// TODO: Implement NoRipple flag checking
	// 1. Validate account address and role (gateway or user)
	// 2. Determine target ledger
	// 3. Analyze trust lines for proper NoRipple flag settings
	// 4. Identify problematic trust lines that should have NoRipple set
	// 5. Generate suggested transactions to fix NoRipple issues if requested
	// 6. Return analysis results and recommendations

	response := map[string]interface{}{
		"account":  request.Account,
		"problems": []string{
			// TODO: List actual NoRipple problems found
		},
		"transactions": []interface{}{
			// TODO: Generate fix transactions if requested
		},
		"ledger_hash":  "PLACEHOLDER_LEDGER_HASH",
		"ledger_index": 1000,
		"validated":    true,
	}

	return response, nil
}

func (m *NoRippleCheckMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *NoRippleCheckMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}
