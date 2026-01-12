package rpc

import (
	"encoding/hex"
	"encoding/json"
)

// BookOffersMethod handles the book_offers RPC method
type BookOffersMethod struct{}

func (m *BookOffersMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		TakerGets json.RawMessage `json:"taker_gets"`
		TakerPays json.RawMessage `json:"taker_pays"`
		Taker     string          `json:"taker,omitempty"`
		LedgerSpecifier
		PaginationParams
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if len(request.TakerGets) == 0 || len(request.TakerPays) == 0 {
		return nil, RpcErrorInvalidParams("Both taker_gets and taker_pays are required")
	}

	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Parse taker_gets amount
	takerGets, err := parseAmountFromJSON(request.TakerGets)
	if err != nil {
		return nil, RpcErrorInvalidParams("Invalid taker_gets: " + err.Error())
	}

	// Parse taker_pays amount
	takerPays, err := parseAmountFromJSON(request.TakerPays)
	if err != nil {
		return nil, RpcErrorInvalidParams("Invalid taker_pays: " + err.Error())
	}

	// Determine ledger index to use
	ledgerIndex := "current"
	if request.LedgerIndex != "" {
		ledgerIndex = request.LedgerIndex.String()
	}

	// Get book offers from the ledger service
	result, err := Services.Ledger.GetBookOffers(takerGets, takerPays, ledgerIndex, request.Limit)
	if err != nil {
		return nil, RpcErrorInternal("Failed to get book offers: " + err.Error())
	}

	// Build response
	response := map[string]interface{}{
		"ledger_hash":  formatLedgerHashUtil(result.LedgerHash),
		"ledger_index": result.LedgerIndex,
		"offers":       result.Offers,
		"validated":    result.Validated,
	}

	return response, nil
}

// parseAmountFromJSON parses an amount from JSON (either XRP string or IOU object)
func parseAmountFromJSON(data json.RawMessage) (Amount, error) {
	// Try parsing as string first (XRP amount)
	var xrpAmount string
	if err := json.Unmarshal(data, &xrpAmount); err == nil {
		return Amount{Value: xrpAmount}, nil
	}

	// Try parsing as IOU object
	var iouAmount struct {
		Currency string `json:"currency"`
		Issuer   string `json:"issuer"`
		Value    string `json:"value,omitempty"`
	}
	if err := json.Unmarshal(data, &iouAmount); err != nil {
		return Amount{}, err
	}

	return Amount{
		Currency: iouAmount.Currency,
		Issuer:   iouAmount.Issuer,
		Value:    iouAmount.Value,
	}, nil
}

// formatLedgerHashUtil formats a 32-byte hash as uppercase hex string
func formatLedgerHashUtil(hash [32]byte) string {
	return hex.EncodeToString(hash[:])
}

func (m *BookOffersMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *BookOffersMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// PathFindMethod handles the path_find RPC method (WebSocket only)
type PathFindMethod struct{}

func (m *PathFindMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// This method is only available via WebSocket as it creates a persistent path-finding session
	return nil, NewRpcError(RpcNOT_SUPPORTED, "notSupported", "notSupported", 
		"path_find is only available via WebSocket")
}

func (m *PathFindMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *PathFindMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// RipplePathFindMethod handles the ripple_path_find RPC method
type RipplePathFindMethod struct{}

func (m *RipplePathFindMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		SourceAccount      string            `json:"source_account"`
		DestinationAccount string            `json:"destination_account"`
		DestinationAmount  json.RawMessage   `json:"destination_amount"`
		SendMax            json.RawMessage   `json:"send_max,omitempty"`
		SourceCurrencies   []json.RawMessage `json:"source_currencies,omitempty"`
		LedgerSpecifier
	}
	
	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}
	
	if request.SourceAccount == "" || request.DestinationAccount == "" {
		return nil, RpcErrorInvalidParams("source_account and destination_account are required")
	}
	
	if len(request.DestinationAmount) == 0 {
		return nil, RpcErrorInvalidParams("destination_amount is required")
	}
	
	// TODO: Implement payment path finding
	// 1. Parse source and destination accounts
	// 2. Parse destination amount and optional send_max limit
	// 3. Determine target ledger state
	// 4. Run path-finding algorithm to find payment paths:
	//    - Direct paths (if same currency)
	//    - Rippling paths through intermediary accounts
	//    - Order book paths through DEX
	//    - Combined paths using multiple mechanisms
	// 5. Calculate exchange rates and liquidity for each path
	// 6. Sort paths by cost (amount to send)
	// 7. Return viable paths with detailed step information
	
	response := map[string]interface{}{
		"source_account":      request.SourceAccount,
		"destination_account": request.DestinationAccount,
		"destination_amount":  request.DestinationAmount,
		"ledger_hash":         "PLACEHOLDER_LEDGER_HASH",
		"ledger_index":        1000,
		"alternatives": []interface{}{
			// TODO: Return actual payment paths
			// Each path should have structure:
			// {
			//   "paths_canonical": [
			//     [
			//       {
			//         "currency": "USD",
			//         "issuer": "rIssuer...",
			//         "type": 48,
			//         "type_hex": "0000000000000030"
			//       }
			//     ]
			//   ],
			//   "paths_computed": [
			//     [
			//       {
			//         "account": "rIntermediary...",
			//         "currency": "USD", 
			//         "issuer": "rIssuer...",
			//         "type": 49,
			//         "type_hex": "0000000000000031"
			//       }
			//     ]
			//   ],
			//   "source_amount": "1100000000"
			// }
		},
		"validated": true,
	}
	
	return response, nil
}

func (m *RipplePathFindMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *RipplePathFindMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// WalletProposeMethod handles the wallet_propose RPC method
type WalletProposeMethod struct{}

func (m *WalletProposeMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		Seed       string `json:"seed,omitempty"`
		SeedHex    string `json:"seed_hex,omitempty"`
		Passphrase string `json:"passphrase,omitempty"`
		KeyType    string `json:"key_type,omitempty"`
	}
	
	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}
	
	// Default key type to secp256k1 if not specified
	if request.KeyType == "" {
		request.KeyType = "secp256k1"
	}
	
	// Validate key type
	if request.KeyType != "secp256k1" && request.KeyType != "ed25519" {
		return nil, RpcErrorInvalidParams("key_type must be 'secp256k1' or 'ed25519'")
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
		"account_id":    "rGeneratedAccount...", // TODO: Generate actual account
		"key_type":      request.KeyType,
		"master_key":    "MASTER_KEY",          // TODO: Generate actual master key
		"master_seed":   "sSEED...",            // TODO: Generate actual seed
		"master_seed_hex": "HEX_SEED",          // TODO: Convert seed to hex
		"public_key":    "PUBLIC_KEY_HEX",      // TODO: Generate actual public key
		"public_key_hex": "PUBLIC_KEY_HEX",     // TODO: Same as public_key
	}
	
	return response, nil
}

func (m *WalletProposeMethod) RequiredRole() Role {
	return RoleGuest // Wallet generation is publicly available
}

func (m *WalletProposeMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// DepositAuthorizedMethod handles the deposit_authorized RPC method
type DepositAuthorizedMethod struct{}

func (m *DepositAuthorizedMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		SourceAccount      string `json:"source_account"`
		DestinationAccount string `json:"destination_account"`
		LedgerSpecifier
	}
	
	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}
	
	if request.SourceAccount == "" || request.DestinationAccount == "" {
		return nil, RpcErrorInvalidParams("source_account and destination_account are required")
	}
	
	// TODO: Implement deposit authorization checking
	// 1. Determine target ledger
	// 2. Check destination account's DepositAuth flag
	// 3. If DepositAuth is set, check for DepositPreauth object
	// 4. Verify if source account is authorized to send payments
	// 5. Consider special cases (same account, XRP vs IOU, etc.)
	
	response := map[string]interface{}{
		"source_account":      request.SourceAccount,
		"destination_account": request.DestinationAccount,
		"deposit_authorized":  true, // TODO: Calculate actual authorization status
		"ledger_hash":         "PLACEHOLDER_LEDGER_HASH",
		"ledger_index":        1000,
		"validated":           true,
	}
	
	return response, nil
}

func (m *DepositAuthorizedMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *DepositAuthorizedMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// ChannelAuthorizeMethod handles the channel_authorize RPC method  
type ChannelAuthorizeMethod struct{}

func (m *ChannelAuthorizeMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		Secret      string `json:"secret,omitempty"`
		Seed        string `json:"seed,omitempty"`
		SeedHex     string `json:"seed_hex,omitempty"`
		Passphrase  string `json:"passphrase,omitempty"`
		KeyType     string `json:"key_type,omitempty"`
		Channel     string `json:"channel"`
		Amount      string `json:"amount"`
	}
	
	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}
	
	if request.Channel == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: channel")
	}
	
	if request.Amount == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: amount")
	}
	
	// TODO: Implement payment channel authorization
	// 1. Validate channel ID and amount
	// 2. Retrieve channel information from ledger
	// 3. Verify signing credentials correspond to channel source
	// 4. Create payment channel claim signature
	// 5. Return signature that can be used to claim from channel
	
	response := map[string]interface{}{
		"signature": "CHANNEL_SIGNATURE", // TODO: Generate actual signature
	}
	
	return response, nil
}

func (m *ChannelAuthorizeMethod) RequiredRole() Role {
	return RoleUser
}

func (m *ChannelAuthorizeMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// ChannelVerifyMethod handles the channel_verify RPC method
type ChannelVerifyMethod struct{}

func (m *ChannelVerifyMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		Channel   string `json:"channel"`
		Signature string `json:"signature"`
		PublicKey string `json:"public_key"`
		Amount    string `json:"amount"`
	}
	
	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}
	
	if request.Channel == "" || request.Signature == "" || request.PublicKey == "" || request.Amount == "" {
		return nil, RpcErrorInvalidParams("channel, signature, public_key, and amount are required")
	}
	
	// TODO: Implement payment channel signature verification
	// 1. Validate channel ID, signature, public key, and amount formats
	// 2. Retrieve channel information from ledger  
	// 3. Verify that public key matches channel source account
	// 4. Reconstruct the signed message from channel ID and amount
	// 5. Verify signature against message using provided public key
	// 6. Return verification result
	
	response := map[string]interface{}{
		"signature_verified": true, // TODO: Perform actual verification
	}
	
	return response, nil
}

func (m *ChannelVerifyMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *ChannelVerifyMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// JsonMethod handles the json RPC method
type JsonMethod struct{}

func (m *JsonMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params,omitempty"`
	}
	
	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}
	
	if request.Method == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: method")
	}
	
	// TODO: Implement JSON method proxy
	// This method allows calling other RPC methods with JSON parameters
	// It's essentially a wrapper that forwards the call to the specified method
	// This can be useful for clients that need to call methods dynamically
	
	// Forward the call to the specified method
	// This is a recursive call through the same RPC system
	return nil, NewRpcError(RpcNOT_SUPPORTED, "notSupported", "notSupported",
		"json method forwarding not yet implemented")
}

func (m *JsonMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *JsonMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}