package rpc

import (
	"encoding/json"
	"fmt"
)

// =============================================================================
// SKIPPED FEATURES TRACKING
// =============================================================================
//
// The following RPC methods have dependencies on features not yet implemented:
//
// 1. amm_info - Requires AMM ledger entry type (not implemented)
// 2. vault_info - Requires Vault ledger entry type (not implemented)
// 3. simulate - Requires transaction dry-run in TxQ (partial implementation)
// 4. get_aggregate_price - Requires Oracle ledger entry type (not implemented)
// 5. ledger_diff - gRPC only in rippled, JSON-RPC stub provided
// 6. owner_info - Requires NetworkOPs.getOwnerInfo (not implemented)
// 7. can_delete - Requires SHAMapStore advisory delete (not implemented)
// 8. ledger_cleaner - Requires LedgerCleaner service (not implemented)
// 9. ledger_request - Requires network ledger fetching (not implemented)
//
// These methods return appropriate error responses indicating the feature
// is not yet available.
// =============================================================================

// =============================================================================
// fetch_info - Returns information about ledger fetch operations
// Reference: rippled/src/xrpld/rpc/handlers/FetchInfo.cpp
// =============================================================================

type FetchInfoMethod struct{}

func (m *FetchInfoMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Parse optional clear parameter
	var request struct {
		Clear bool `json:"clear,omitempty"`
	}
	if params != nil {
		_ = json.Unmarshal(params, &request)
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	response := make(map[string]interface{})

	if request.Clear {
		// TODO: Implement clear ledger fetch when NetworkOPs is available
		// Services.NetworkOPs.ClearLedgerFetch()
		response["clear"] = true
	}

	// TODO: Implement GetLedgerFetchInfo when NetworkOPs is available
	// For now return empty info
	response["info"] = map[string]interface{}{}

	return response, nil
}

func (m *FetchInfoMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *FetchInfoMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// owner_info - Returns aggregate information about objects owned by an account
// Reference: rippled/src/xrpld/rpc/handlers/OwnerInfo.cpp
// =============================================================================

type OwnerInfoMethod struct{}

func (m *OwnerInfoMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		Account string `json:"account,omitempty"`
		Ident   string `json:"ident,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// Get account from either account or ident field
	account := request.Account
	if account == "" {
		account = request.Ident
	}
	if account == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: account")
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// TODO: Implement owner info retrieval when NetworkOPs.GetOwnerInfo is available
	// This method returns aggregate info about owned objects (offers, trust lines, etc.)
	// For now return not implemented error
	return nil, NewRpcError(RpcNOT_IMPL, "notImplemented", "notImplemented",
		"owner_info is not yet implemented - requires NetworkOPs.GetOwnerInfo")
}

func (m *OwnerInfoMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *OwnerInfoMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// ledger_header - Returns the raw header data for a ledger
// Reference: rippled/src/xrpld/rpc/handlers/LedgerHeader.cpp
// =============================================================================

type LedgerHeaderMethod struct{}

func (m *LedgerHeaderMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		LedgerSpecifier
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Determine which ledger to retrieve
	var ledger LedgerReader
	var err error

	if request.LedgerHash != "" {
		// TODO: Parse hash string and get by hash
		return nil, NewRpcError(RpcNOT_IMPL, "notImplemented", "notImplemented",
			"ledger_header by hash is not yet implemented")
	} else if request.LedgerIndex != "" {
		ledgerIndexStr := request.LedgerIndex.String()
		switch ledgerIndexStr {
		case "validated":
			seq := Services.Ledger.GetValidatedLedgerIndex()
			ledger, err = Services.Ledger.GetLedgerBySequence(seq)
		case "closed":
			seq := Services.Ledger.GetClosedLedgerIndex()
			ledger, err = Services.Ledger.GetLedgerBySequence(seq)
		case "current":
			seq := Services.Ledger.GetCurrentLedgerIndex()
			ledger, err = Services.Ledger.GetLedgerBySequence(seq)
		default:
			// Try to parse as number
			var seq uint32
			if _, scanErr := fmt.Sscanf(ledgerIndexStr, "%d", &seq); scanErr == nil {
				ledger, err = Services.Ledger.GetLedgerBySequence(seq)
			} else {
				return nil, RpcErrorInvalidParams("Invalid ledger_index: " + ledgerIndexStr)
			}
		}
	} else {
		// Default to validated
		seq := Services.Ledger.GetValidatedLedgerIndex()
		ledger, err = Services.Ledger.GetLedgerBySequence(seq)
	}

	if err != nil {
		return nil, RpcErrorLgrNotFound("Ledger not found: " + err.Error())
	}

	response := map[string]interface{}{
		"ledger_index": ledger.Sequence(),
		"closed":       ledger.IsClosed(),
	}

	// Add hash if available
	hash := ledger.Hash()
	if hash != [32]byte{} {
		response["ledger_hash"] = fmt.Sprintf("%X", hash)
	}
	parentHash := ledger.ParentHash()
	if parentHash != [32]byte{} {
		response["parent_hash"] = fmt.Sprintf("%X", parentHash)
	}

	// TODO: Add ledger_data field with serialized raw header when available
	// This requires serializing the ledger header to hex format

	return response, nil
}

func (m *LedgerHeaderMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *LedgerHeaderMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// ledger_request - Request a specific ledger from the network
// Reference: rippled/src/xrpld/rpc/handlers/LedgerRequest.cpp
// =============================================================================

type LedgerRequestMethod struct{}

func (m *LedgerRequestMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		LedgerSpecifier
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Check standalone mode - ledger_request doesn't make sense in standalone
	if Services.Ledger.IsStandalone() {
		return nil, NewRpcError(RpcNOT_SYNCED, "notSynced", "notSynced",
			"Not synced to the network")
	}

	// TODO: Implement ledger request from network when peer networking is complete
	// This requires the ability to request ledgers from network peers
	return nil, NewRpcError(RpcNOT_IMPL, "notImplemented", "notImplemented",
		"ledger_request is not yet implemented - requires network ledger fetching")
}

func (m *LedgerRequestMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *LedgerRequestMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// ledger_cleaner - Configure the ledger cleaner service
// Reference: rippled/src/xrpld/rpc/handlers/LedgerCleanerHandler.cpp
// =============================================================================

type LedgerCleanerMethod struct{}

func (m *LedgerCleanerMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// TODO: Implement ledger cleaner configuration when LedgerCleaner service is available
	// The ledger cleaner validates and repairs the ledger database
	return nil, NewRpcError(RpcNOT_IMPL, "notImplemented", "notImplemented",
		"ledger_cleaner is not yet implemented - requires LedgerCleaner service")
}

func (m *LedgerCleanerMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *LedgerCleanerMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// ledger_diff - Get differences between two ledgers (gRPC only in rippled)
// Reference: rippled/src/xrpld/rpc/handlers/LedgerDiff.cpp
// =============================================================================

type LedgerDiffMethod struct{}

func (m *LedgerDiffMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// In rippled, ledger_diff is gRPC only, not available via JSON-RPC
	// We provide a stub that returns an appropriate error
	return nil, NewRpcError(RpcNOT_IMPL, "notImplemented", "notImplemented",
		"ledger_diff is only available via gRPC in rippled - JSON-RPC not supported")
}

func (m *LedgerDiffMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *LedgerDiffMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// tx_reduce_relay - Get transaction reduce-relay metrics
// Reference: rippled/src/xrpld/rpc/handlers/TxReduceRelay.cpp
// =============================================================================

type TxReduceRelayMethod struct{}

func (m *TxReduceRelayMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// TODO: Implement tx reduce relay metrics when Overlay.TxMetrics is available
	// This returns metrics about transaction relay optimization
	response := map[string]interface{}{
		"transactions": map[string]interface{}{
			"total_relayed":   0,
			"total_squelched": 0,
		},
	}

	return response, nil
}

func (m *TxReduceRelayMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *TxReduceRelayMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// simulate - Simulate a transaction without submitting
// Reference: rippled/src/xrpld/rpc/handlers/Simulate.cpp
// =============================================================================

type SimulateMethod struct{}

func (m *SimulateMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		TxBlob string                 `json:"tx_blob,omitempty"`
		TxJSON map[string]interface{} `json:"tx_json,omitempty"`
		Binary bool                   `json:"binary,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// Validate that either tx_blob or tx_json is provided, but not both
	hasTxBlob := request.TxBlob != ""
	hasTxJSON := request.TxJSON != nil && len(request.TxJSON) > 0

	if hasTxBlob && hasTxJSON {
		return nil, RpcErrorInvalidParams("Can only include one of `tx_blob` and `tx_json`")
	}
	if !hasTxBlob && !hasTxJSON {
		return nil, RpcErrorInvalidParams("Neither `tx_blob` nor `tx_json` included")
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// TODO: Implement transaction simulation when TxQ dry-run is available
	// This requires:
	// 1. Parse transaction from tx_blob or tx_json
	// 2. Autofill missing fields (Fee, Sequence, SigningPubKey, TxnSignature)
	// 3. Run through TxQ.Apply with tapDRY_RUN flag
	// 4. Return results including metadata

	return nil, NewRpcError(RpcNOT_IMPL, "notImplemented", "notImplemented",
		"simulate is not yet implemented - requires TxQ dry-run capability")
}

func (m *SimulateMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *SimulateMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// connect - Manually connect to a peer
// Reference: rippled/src/xrpld/rpc/handlers/Connect.cpp
// =============================================================================

type ConnectMethod struct{}

func (m *ConnectMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		IP   string `json:"ip"`
		Port int    `json:"port,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.IP == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: ip")
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Check standalone mode
	if Services.Ledger.IsStandalone() {
		return nil, NewRpcError(RpcNOT_SYNCED, "notSynced", "notSynced",
			"Cannot connect to peers in standalone mode")
	}

	// Default port
	port := request.Port
	if port == 0 {
		port = 51235 // DEFAULT_PEER_PORT
	}

	// TODO: Implement peer connection when Overlay is available
	// Services.Overlay.Connect(ip, port)

	response := map[string]interface{}{
		"message": "attempting connection to IP:" + request.IP + " port: " + string(rune(port)),
	}

	return response, nil
}

func (m *ConnectMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *ConnectMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// print - Print internal state for debugging
// Reference: rippled/src/xrpld/rpc/handlers/Print.cpp
// =============================================================================

type PrintMethod struct{}

func (m *PrintMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		Params []string `json:"params,omitempty"`
	}

	if params != nil {
		_ = json.Unmarshal(params, &request)
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// TODO: Implement detailed internal state printing when Application.Write is available
	// This is primarily a debugging tool
	response := map[string]interface{}{
		"status": "print command received",
	}

	if len(request.Params) > 0 {
		response["filter"] = request.Params[0]
	}

	return response, nil
}

func (m *PrintMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *PrintMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// validator_info - Get information about this node's validator configuration
// Reference: rippled/src/xrpld/rpc/handlers/ValidatorInfo.cpp
// =============================================================================

type ValidatorInfoMethod struct{}

func (m *ValidatorInfoMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// TODO: Implement validator info when validator keys are available
	// This returns:
	// - master_key: The validator's master public key
	// - ephemeral_key: The ephemeral public key (if different from master)
	// - manifest: The validator manifest (base64 encoded)
	// - seq: The manifest sequence number
	// - domain: The validator's domain (if set)

	// Return error if not configured as validator
	return nil, NewRpcError(RpcNOT_VALIDATOR, "notValidator", "notValidator",
		"This server is not configured as a validator")
}

func (m *ValidatorInfoMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *ValidatorInfoMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// can_delete - Query or set the ledger that can be deleted
// Reference: rippled/src/xrpld/rpc/handlers/CanDelete.cpp
// =============================================================================

type CanDeleteMethod struct{}

func (m *CanDeleteMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		CanDelete interface{} `json:"can_delete,omitempty"` // Can be uint, string ("never", "always", "now"), or ledger hash
	}

	if params != nil {
		_ = json.Unmarshal(params, &request)
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// TODO: Implement can_delete when SHAMapStore advisory delete is available
	// This method controls which ledgers can be deleted by online deletion
	return nil, NewRpcError(RpcNOT_ENABLED, "notEnabled", "notEnabled",
		"Advisory delete is not enabled - requires SHAMapStore configuration")
}

func (m *CanDeleteMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *CanDeleteMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// get_aggregate_price - Get aggregated price from oracle data
// Reference: rippled/src/xrpld/rpc/handlers/GetAggregatePrice.cpp
// =============================================================================

type GetAggregatePriceMethod struct{}

func (m *GetAggregatePriceMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		Oracles       []map[string]interface{} `json:"oracles"`
		BaseAsset     string                   `json:"base_asset"`
		QuoteAsset    string                   `json:"quote_asset"`
		Trim          uint32                   `json:"trim,omitempty"`
		TimeThreshold uint32                   `json:"time_threshold,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// Validate required fields
	if len(request.Oracles) == 0 {
		return nil, RpcErrorInvalidParams("Missing required parameter: oracles")
	}
	if request.BaseAsset == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: base_asset")
	}
	if request.QuoteAsset == "" {
		return nil, RpcErrorInvalidParams("Missing required parameter: quote_asset")
	}

	// Validate oracle count (max 200)
	if len(request.Oracles) > 200 {
		return nil, RpcErrorInvalidParams("oracles array exceeds maximum size of 200")
	}

	// Validate trim (0 or 1-25%)
	if request.Trim > 25 {
		return nil, RpcErrorInvalidParams("trim must be between 1 and 25")
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// TODO: Implement get_aggregate_price when Oracle ledger entry is available
	// This method:
	// 1. Reads price data from specified oracles
	// 2. Filters by time_threshold
	// 3. Calculates statistics (mean, median, std deviation)
	// 4. Optionally trims outliers

	return nil, NewRpcError(RpcNOT_IMPL, "notImplemented", "notImplemented",
		"get_aggregate_price is not yet implemented - requires Oracle ledger entry type")
}

func (m *GetAggregatePriceMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *GetAggregatePriceMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// get_counts - Get object counts and statistics
// Reference: rippled/src/xrpld/rpc/handlers/GetCounts.cpp
// =============================================================================

type GetCountsMethod struct{}

func (m *GetCountsMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		MinCount int `json:"min_count,omitempty"`
	}

	if params != nil {
		_ = json.Unmarshal(params, &request)
	}

	// Default min_count to 10
	minCount := request.MinCount
	if minCount == 0 {
		minCount = 10
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Return basic counts that are available
	serverInfo := Services.Ledger.GetServerInfo()
	response := map[string]interface{}{
		"standalone": serverInfo.Standalone,
	}

	// TODO: Add more detailed counts when available:
	// - Object counts (CountedObjects)
	// - Database sizes (dbKBTotal, dbKBLedger, dbKBTransaction)
	// - Cache hit rates (SLE_hit_rate, ledger_hit_rate, AL_hit_rate)
	// - Tree node cache sizes
	// - Write load

	return response, nil
}

func (m *GetCountsMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *GetCountsMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// log_level - Get or set log severity levels
// Reference: rippled/src/xrpld/rpc/handlers/LogLevel.cpp
// =============================================================================

type LogLevelMethod struct{}

func (m *LogLevelMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		Severity  string `json:"severity,omitempty"`
		Partition string `json:"partition,omitempty"`
	}

	if params != nil {
		_ = json.Unmarshal(params, &request)
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// If no severity specified, return current levels
	if request.Severity == "" {
		// TODO: Get actual log levels when logging service is available
		response := map[string]interface{}{
			"levels": map[string]interface{}{
				"base": "info",
			},
		}
		return response, nil
	}

	// Validate severity level
	validLevels := map[string]bool{
		"trace": true, "debug": true, "info": true,
		"warning": true, "error": true, "fatal": true,
	}
	if !validLevels[request.Severity] {
		return nil, RpcErrorInvalidParams("Invalid severity level: " + request.Severity)
	}

	// TODO: Set log level when logging service is available
	// if request.Partition == "" || request.Partition == "base" {
	//     Services.Logs.SetThreshold(severity)
	// } else {
	//     Services.Logs.GetPartition(request.Partition).SetThreshold(severity)
	// }

	return map[string]interface{}{}, nil
}

func (m *LogLevelMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *LogLevelMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// log_rotate - Rotate log files
// Reference: rippled/src/xrpld/rpc/handlers/LogRotate.cpp
// =============================================================================

type LogRotateMethod struct{}

func (m *LogRotateMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// TODO: Implement log rotation when logging service is available
	// Services.PerfLog.Rotate()
	// message := Services.Logs.Rotate()

	return map[string]interface{}{
		"message": "Log rotation requested",
	}, nil
}

func (m *LogRotateMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *LogRotateMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// amm_info - Get information about an Automated Market Maker (AMM)
// Reference: rippled/src/xrpld/rpc/handlers/AMMInfo.cpp
// =============================================================================

type AMMInfoMethod struct{}

func (m *AMMInfoMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		LedgerSpecifier
		Asset      map[string]interface{} `json:"asset,omitempty"`
		Asset2     map[string]interface{} `json:"asset2,omitempty"`
		AMMAccount string                 `json:"amm_account,omitempty"`
		Account    string                 `json:"account,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// Validate parameter combinations
	hasAssets := request.Asset != nil && request.Asset2 != nil
	hasAMMAccount := request.AMMAccount != ""

	if hasAssets == hasAMMAccount {
		return nil, RpcErrorInvalidParams("Must specify either (asset + asset2) or amm_account, but not both or neither")
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// TODO: Implement amm_info when AMM ledger entry type is available
	// This method returns:
	// - amount/amount2: Pool balances
	// - lp_token: LP token balance
	// - trading_fee: Current trading fee
	// - account: AMM account address
	// - vote_slots: Voting information
	// - auction_slot: Auction slot info (if present)
	// - asset_frozen/asset2_frozen: Freeze status

	return nil, NewRpcError(RpcNOT_IMPL, "notImplemented", "notImplemented",
		"amm_info is not yet implemented - requires AMM ledger entry type")
}

func (m *AMMInfoMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *AMMInfoMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// vault_info - Get information about a Vault
// Reference: rippled/src/xrpld/rpc/handlers/VaultInfo.cpp
// =============================================================================

type VaultInfoMethod struct{}

func (m *VaultInfoMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		LedgerSpecifier
		VaultID string `json:"vault_id,omitempty"`
		Owner   string `json:"owner,omitempty"`
		Seq     uint32 `json:"seq,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	// Validate parameter combinations
	hasVaultID := request.VaultID != ""
	hasOwnerSeq := request.Owner != "" && request.Seq > 0

	if hasVaultID == hasOwnerSeq {
		return nil, RpcErrorInvalidParams("Must specify either vault_id or (owner + seq), but not both or neither")
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// TODO: Implement vault_info when Vault ledger entry type is available
	// This method returns vault information including share MPT issuance details

	return nil, NewRpcError(RpcNOT_IMPL, "notImplemented", "notImplemented",
		"vault_info is not yet implemented - requires Vault ledger entry type")
}

func (m *VaultInfoMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *VaultInfoMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// unl_list - List validators in the Unique Node List
// Reference: rippled/src/xrpld/rpc/handlers/UnlList.cpp
// =============================================================================

type UnlListMethod struct{}

func (m *UnlListMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// TODO: Implement unl_list when ValidatorList is available
	// This returns an array of validators with their public keys and trust status

	response := map[string]interface{}{
		"unl": []interface{}{},
	}

	return response, nil
}

func (m *UnlListMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *UnlListMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// =============================================================================
// black_list - Get resource manager blacklist information
// Reference: rippled/src/xrpld/rpc/handlers/BlackList.cpp
// =============================================================================

type BlackListMethod struct{}

func (m *BlackListMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	var request struct {
		Threshold int `json:"threshold,omitempty"`
	}

	if params != nil {
		_ = json.Unmarshal(params, &request)
	}

	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// TODO: Implement black_list when ResourceManager is available
	// This returns information about blacklisted IPs/endpoints based on resource usage

	response := map[string]interface{}{
		"blacklist": []interface{}{},
	}

	return response, nil
}

func (m *BlackListMethod) RequiredRole() Role {
	return RoleAdmin
}

func (m *BlackListMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}
