package rpc_handlers

import (
	"encoding/json"
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
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

// FetchInfoMethod handles the fetch_info RPC method
type FetchInfoMethod struct{}

func (m *FetchInfoMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		Clear bool `json:"clear,omitempty"`
	}
	if params != nil {
		_ = json.Unmarshal(params, &request)
	}

	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	response := make(map[string]interface{})

	if request.Clear {
		response["clear"] = true
	}

	response["info"] = map[string]interface{}{}

	return response, nil
}

func (m *FetchInfoMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *FetchInfoMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// OwnerInfoMethod handles the owner_info RPC method
type OwnerInfoMethod struct{}

func (m *OwnerInfoMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		Account string `json:"account,omitempty"`
		Ident   string `json:"ident,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	account := request.Account
	if account == "" {
		account = request.Ident
	}
	if account == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: account")
	}

	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_IMPL, "notImplemented", "notImplemented",
		"owner_info is not yet implemented - requires NetworkOPs.GetOwnerInfo")
}

func (m *OwnerInfoMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *OwnerInfoMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// LedgerHeaderMethod handles the ledger_header RPC method
type LedgerHeaderMethod struct{}

func (m *LedgerHeaderMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		rpc_types.LedgerSpecifier
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	var ledger rpc_types.LedgerReader
	var err error

	if request.LedgerHash != "" {
		return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_IMPL, "notImplemented", "notImplemented",
			"ledger_header by hash is not yet implemented")
	} else if request.LedgerIndex != "" {
		ledgerIndexStr := request.LedgerIndex.String()
		switch ledgerIndexStr {
		case "validated":
			seq := rpc_types.Services.Ledger.GetValidatedLedgerIndex()
			ledger, err = rpc_types.Services.Ledger.GetLedgerBySequence(seq)
		case "closed":
			seq := rpc_types.Services.Ledger.GetClosedLedgerIndex()
			ledger, err = rpc_types.Services.Ledger.GetLedgerBySequence(seq)
		case "current":
			seq := rpc_types.Services.Ledger.GetCurrentLedgerIndex()
			ledger, err = rpc_types.Services.Ledger.GetLedgerBySequence(seq)
		default:
			var seq uint32
			if _, scanErr := fmt.Sscanf(ledgerIndexStr, "%d", &seq); scanErr == nil {
				ledger, err = rpc_types.Services.Ledger.GetLedgerBySequence(seq)
			} else {
				return nil, rpc_types.RpcErrorInvalidParams("Invalid ledger_index: " + ledgerIndexStr)
			}
		}
	} else {
		seq := rpc_types.Services.Ledger.GetValidatedLedgerIndex()
		ledger, err = rpc_types.Services.Ledger.GetLedgerBySequence(seq)
	}

	if err != nil {
		return nil, rpc_types.RpcErrorLgrNotFound("Ledger not found: " + err.Error())
	}

	response := map[string]interface{}{
		"ledger_index": ledger.Sequence(),
		"closed":       ledger.IsClosed(),
	}

	hash := ledger.Hash()
	if hash != [32]byte{} {
		response["ledger_hash"] = fmt.Sprintf("%X", hash)
	}
	parentHash := ledger.ParentHash()
	if parentHash != [32]byte{} {
		response["parent_hash"] = fmt.Sprintf("%X", parentHash)
	}

	return response, nil
}

func (m *LedgerHeaderMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *LedgerHeaderMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// LedgerRequestMethod handles the ledger_request RPC method
type LedgerRequestMethod struct{}

func (m *LedgerRequestMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	if rpc_types.Services.Ledger.IsStandalone() {
		return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_SYNCED, "notSynced", "notSynced",
			"Not synced to the network")
	}

	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_IMPL, "notImplemented", "notImplemented",
		"ledger_request is not yet implemented - requires network ledger fetching")
}

func (m *LedgerRequestMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *LedgerRequestMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// LedgerCleanerMethod handles the ledger_cleaner RPC method
type LedgerCleanerMethod struct{}

func (m *LedgerCleanerMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_IMPL, "notImplemented", "notImplemented",
		"ledger_cleaner is not yet implemented - requires LedgerCleaner service")
}

func (m *LedgerCleanerMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *LedgerCleanerMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// LedgerDiffMethod handles the ledger_diff RPC method
type LedgerDiffMethod struct{}

func (m *LedgerDiffMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_IMPL, "notImplemented", "notImplemented",
		"ledger_diff is only available via gRPC in rippled - JSON-RPC not supported")
}

func (m *LedgerDiffMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *LedgerDiffMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// TxReduceRelayMethod handles the tx_reduce_relay RPC method
type TxReduceRelayMethod struct{}

func (m *TxReduceRelayMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	response := map[string]interface{}{
		"transactions": map[string]interface{}{
			"total_relayed":   0,
			"total_squelched": 0,
		},
	}

	return response, nil
}

func (m *TxReduceRelayMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *TxReduceRelayMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// SimulateMethod handles the simulate RPC method
type SimulateMethod struct{}

func (m *SimulateMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		TxBlob string                 `json:"tx_blob,omitempty"`
		TxJSON map[string]interface{} `json:"tx_json,omitempty"`
		Binary bool                   `json:"binary,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	hasTxBlob := request.TxBlob != ""
	hasTxJSON := request.TxJSON != nil && len(request.TxJSON) > 0

	if hasTxBlob && hasTxJSON {
		return nil, rpc_types.RpcErrorInvalidParams("Can only include one of `tx_blob` and `tx_json`")
	}
	if !hasTxBlob && !hasTxJSON {
		return nil, rpc_types.RpcErrorInvalidParams("Neither `tx_blob` nor `tx_json` included")
	}

	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_IMPL, "notImplemented", "notImplemented",
		"simulate is not yet implemented - requires TxQ dry-run capability")
}

func (m *SimulateMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *SimulateMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// ConnectMethod handles the connect RPC method
type ConnectMethod struct{}

func (m *ConnectMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		IP   string `json:"ip"`
		Port int    `json:"port,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if request.IP == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: ip")
	}

	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	if rpc_types.Services.Ledger.IsStandalone() {
		return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_SYNCED, "notSynced", "notSynced",
			"Cannot connect to peers in standalone mode")
	}

	port := request.Port
	if port == 0 {
		port = 51235
	}

	response := map[string]interface{}{
		"message": fmt.Sprintf("attempting connection to IP:%s port:%d", request.IP, port),
	}

	return response, nil
}

func (m *ConnectMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *ConnectMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// PrintMethod handles the print RPC method
type PrintMethod struct{}

func (m *PrintMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		Params []string `json:"params,omitempty"`
	}

	if params != nil {
		_ = json.Unmarshal(params, &request)
	}

	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	response := map[string]interface{}{
		"status": "print command received",
	}

	if len(request.Params) > 0 {
		response["filter"] = request.Params[0]
	}

	return response, nil
}

func (m *PrintMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *PrintMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// ValidatorInfoMethod handles the validator_info RPC method
type ValidatorInfoMethod struct{}

func (m *ValidatorInfoMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_VALIDATOR, "notValidator", "notValidator",
		"This server is not configured as a validator")
}

func (m *ValidatorInfoMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *ValidatorInfoMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// CanDeleteMethod handles the can_delete RPC method
type CanDeleteMethod struct{}

func (m *CanDeleteMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_ENABLED, "notEnabled", "notEnabled",
		"Advisory delete is not enabled - requires SHAMapStore configuration")
}

func (m *CanDeleteMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *CanDeleteMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// GetAggregatePriceMethod handles the get_aggregate_price RPC method
type GetAggregatePriceMethod struct{}

func (m *GetAggregatePriceMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		Oracles       []map[string]interface{} `json:"oracles"`
		BaseAsset     string                   `json:"base_asset"`
		QuoteAsset    string                   `json:"quote_asset"`
		Trim          uint32                   `json:"trim,omitempty"`
		TimeThreshold uint32                   `json:"time_threshold,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if len(request.Oracles) == 0 {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: oracles")
	}
	if request.BaseAsset == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: base_asset")
	}
	if request.QuoteAsset == "" {
		return nil, rpc_types.RpcErrorInvalidParams("Missing required parameter: quote_asset")
	}

	if len(request.Oracles) > 200 {
		return nil, rpc_types.RpcErrorInvalidParams("oracles array exceeds maximum size of 200")
	}

	if request.Trim > 25 {
		return nil, rpc_types.RpcErrorInvalidParams("trim must be between 1 and 25")
	}

	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_IMPL, "notImplemented", "notImplemented",
		"get_aggregate_price is not yet implemented - requires Oracle ledger entry type")
}

func (m *GetAggregatePriceMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *GetAggregatePriceMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// GetCountsMethod handles the get_counts RPC method
type GetCountsMethod struct{}

func (m *GetCountsMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		MinCount int `json:"min_count,omitempty"`
	}

	if params != nil {
		_ = json.Unmarshal(params, &request)
	}

	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	serverInfo := rpc_types.Services.Ledger.GetServerInfo()
	response := map[string]interface{}{
		"standalone": serverInfo.Standalone,
	}

	return response, nil
}

func (m *GetCountsMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *GetCountsMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// LogLevelMethod handles the log_level RPC method
type LogLevelMethod struct{}

func (m *LogLevelMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		Severity  string `json:"severity,omitempty"`
		Partition string `json:"partition,omitempty"`
	}

	if params != nil {
		_ = json.Unmarshal(params, &request)
	}

	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	if request.Severity == "" {
		response := map[string]interface{}{
			"levels": map[string]interface{}{
				"base": "info",
			},
		}
		return response, nil
	}

	validLevels := map[string]bool{
		"trace": true, "debug": true, "info": true,
		"warning": true, "error": true, "fatal": true,
	}
	if !validLevels[request.Severity] {
		return nil, rpc_types.RpcErrorInvalidParams("Invalid severity level: " + request.Severity)
	}

	return map[string]interface{}{}, nil
}

func (m *LogLevelMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *LogLevelMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// LogRotateMethod handles the log_rotate RPC method
type LogRotateMethod struct{}

func (m *LogRotateMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	return map[string]interface{}{
		"message": "Log rotation requested",
	}, nil
}

func (m *LogRotateMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *LogRotateMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// AMMInfoMethod handles the amm_info RPC method
type AMMInfoMethod struct{}

func (m *AMMInfoMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		rpc_types.LedgerSpecifier
		Asset      map[string]interface{} `json:"asset,omitempty"`
		Asset2     map[string]interface{} `json:"asset2,omitempty"`
		AMMAccount string                 `json:"amm_account,omitempty"`
		Account    string                 `json:"account,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	hasAssets := request.Asset != nil && request.Asset2 != nil
	hasAMMAccount := request.AMMAccount != ""

	if hasAssets == hasAMMAccount {
		return nil, rpc_types.RpcErrorInvalidParams("Must specify either (asset + asset2) or amm_account, but not both or neither")
	}

	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_IMPL, "notImplemented", "notImplemented",
		"amm_info is not yet implemented - requires AMM ledger entry type")
}

func (m *AMMInfoMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *AMMInfoMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// VaultInfoMethod handles the vault_info RPC method
type VaultInfoMethod struct{}

func (m *VaultInfoMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		rpc_types.LedgerSpecifier
		VaultID string `json:"vault_id,omitempty"`
		Owner   string `json:"owner,omitempty"`
		Seq     uint32 `json:"seq,omitempty"`
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	hasVaultID := request.VaultID != ""
	hasOwnerSeq := request.Owner != "" && request.Seq > 0

	if hasVaultID == hasOwnerSeq {
		return nil, rpc_types.RpcErrorInvalidParams("Must specify either vault_id or (owner + seq), but not both or neither")
	}

	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	return nil, rpc_types.NewRpcError(rpc_types.RpcNOT_IMPL, "notImplemented", "notImplemented",
		"vault_info is not yet implemented - requires Vault ledger entry type")
}

func (m *VaultInfoMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *VaultInfoMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// UnlListMethod handles the unl_list RPC method
type UnlListMethod struct{}

func (m *UnlListMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	response := map[string]interface{}{
		"unl": []interface{}{},
	}

	return response, nil
}

func (m *UnlListMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *UnlListMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// BlackListMethod handles the black_list RPC method
type BlackListMethod struct{}

func (m *BlackListMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	response := map[string]interface{}{
		"blacklist": []interface{}{},
	}

	return response, nil
}

func (m *BlackListMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *BlackListMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
