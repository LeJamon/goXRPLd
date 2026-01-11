package rpc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// serverStartTime tracks when the server started for uptime calculation
var serverStartTime = time.Now()

// ServerInfoMethod handles the server_info RPC method
type ServerInfoMethod struct{}

func (m *ServerInfoMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Get server info from ledger service
	serverInfo := Services.Ledger.GetServerInfo()

	// Get fee settings
	baseFee, reserveBase, reserveIncrement := Services.Ledger.GetCurrentFees()

	// Calculate uptime
	uptime := int64(time.Since(serverStartTime).Seconds())

	// Build complete ledgers string
	completeLedgers := serverInfo.CompleteLedgers
	if completeLedgers == "" {
		completeLedgers = "empty"
	}

	// Get validated ledger info
	validatedLedgerHash := hex.EncodeToString(serverInfo.ValidatedLedgerHash[:])
	validatedLedgerSeq := serverInfo.ValidatedLedgerSeq

	// Determine server state
	serverState := "full"
	if serverInfo.Standalone {
		serverState = "standalone"
	}

	// Calculate base fee in XRP (drops / 1,000,000)
	baseFeeXRP := float64(baseFee) / 1000000.0
	reserveBaseXRP := float64(reserveBase) / 1000000.0
	reserveIncXRP := float64(reserveIncrement) / 1000000.0

	response := map[string]interface{}{
		"info": map[string]interface{}{
			"build_version":              "2.0.0-goXRPLd",
			"complete_ledgers":           completeLedgers,
			"hostid":                     "goXRPLd",
			"io_latency_ms":              1,
			"jq_trans_overflow":          "0",
			"last_close": map[string]interface{}{
				"converge_time_s": 0.0,
				"proposers":       0,
			},
			"load_factor":                1.0,
			"peer_disconnects":           "0",
			"peer_disconnects_resources": "0",
			"peers":                      0, // Standalone mode has no peers
			"pubkey_node":                "n9KnrcCmL5psyKtk2KWP6jy14Hj4EXuZDg7XMdQJ9cSDoFSp53hu",
			"server_state":               serverState,
			"server_state_duration_us":   fmt.Sprintf("%d", uptime*1000000),
			"state_accounting": map[string]interface{}{
				"connected": map[string]interface{}{
					"duration_us": "0",
					"transitions": "0",
				},
				"disconnected": map[string]interface{}{
					"duration_us": "0",
					"transitions": "0",
				},
				"full": map[string]interface{}{
					"duration_us": fmt.Sprintf("%d", uptime*1000000),
					"transitions": "1",
				},
				"syncing": map[string]interface{}{
					"duration_us": "0",
					"transitions": "0",
				},
				"tracking": map[string]interface{}{
					"duration_us": "0",
					"transitions": "0",
				},
			},
			"time":   time.Now().UTC().Format("2006-Jan-02 15:04:05.000000 MST"),
			"uptime": uptime,
			"validated_ledger": map[string]interface{}{
				"age":              0, // In standalone, always fresh
				"base_fee_xrp":     baseFeeXRP,
				"hash":             validatedLedgerHash,
				"reserve_base_xrp": reserveBaseXRP,
				"reserve_inc_xrp":  reserveIncXRP,
				"seq":              validatedLedgerSeq,
			},
			"validation_quorum": 1, // Standalone needs only 1
		},
	}

	return response, nil
}

func (m *ServerInfoMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *ServerInfoMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// ServerStateMethod handles the server_state RPC method
type ServerStateMethod struct{}

func (m *ServerStateMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Get server info from ledger service
	serverInfo := Services.Ledger.GetServerInfo()

	// Get fee settings
	baseFee, reserveBase, reserveIncrement := Services.Ledger.GetCurrentFees()

	// Calculate uptime
	uptime := int64(time.Since(serverStartTime).Seconds())

	// Build complete ledgers string
	completeLedgers := serverInfo.CompleteLedgers
	if completeLedgers == "" {
		completeLedgers = "empty"
	}

	// Get validated ledger info
	validatedLedgerHash := hex.EncodeToString(serverInfo.ValidatedLedgerHash[:])
	validatedLedgerSeq := serverInfo.ValidatedLedgerSeq

	// Determine server state
	serverState := "full"
	if serverInfo.Standalone {
		serverState = "standalone"
	}

	// Calculate base fee in XRP
	baseFeeXRP := float64(baseFee) / 1000000.0
	reserveBaseXRP := float64(reserveBase) / 1000000.0
	reserveIncXRP := float64(reserveIncrement) / 1000000.0

	response := map[string]interface{}{
		"state": map[string]interface{}{
			"build_version":     "2.0.0-goXRPLd",
			"complete_ledgers":  completeLedgers,
			"io_latency_ms":     1,
			"jq_trans_overflow": 0,
			"load_base":         256,
			"load_factor":       1.0,
			"peers":             0,
			"pubkey_node":       "n9KnrcCmL5psyKtk2KWP6jy14Hj4EXuZDg7XMdQJ9cSDoFSp53hu",
			"server_state":      serverState,
			"time":              time.Now().UTC().Format(time.RFC3339),
			"uptime":            uptime,
			"validated_ledger": map[string]interface{}{
				"age":              0,
				"base_fee_xrp":     baseFeeXRP,
				"hash":             validatedLedgerHash,
				"reserve_base_xrp": reserveBaseXRP,
				"reserve_inc_xrp":  reserveIncXRP,
				"seq":              validatedLedgerSeq,
			},
			"validation_quorum": 1,
		},
	}

	return response, nil
}

func (m *ServerStateMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *ServerStateMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// PingMethod handles the ping RPC method
type PingMethod struct{}

func (m *PingMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Ping method is used to test connectivity and measure round-trip time
	// It simply returns an empty success response

	response := map[string]interface{}{
		// Empty response indicates successful ping
	}

	return response, nil
}

func (m *PingMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *PingMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// RandomMethod handles the random RPC method
type RandomMethod struct{}

func (m *RandomMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Generate 256 bits (32 bytes) of cryptographically secure random data
	randomBytes := make([]byte, 32)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return nil, RpcErrorInternal("Failed to generate random data: " + err.Error())
	}

	response := map[string]interface{}{
		"random": strings.ToUpper(hex.EncodeToString(randomBytes)),
	}

	return response, nil
}

func (m *RandomMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *RandomMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// ServerDefinitionsMethod handles the server_definitions RPC method
type ServerDefinitionsMethod struct{}

func (m *ServerDefinitionsMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// TODO: Implement server definitions retrieval
	// This method returns the transaction and ledger format definitions
	// that the server uses, including:
	// - Transaction types and their fields
	// - Ledger object types and their fields
	// - Field types and their encoding rules
	// - Amendment status
	// This data should come from the definitions system used by the binary codec

	response := map[string]interface{}{
		"FIELDS": map[string]interface{}{
			// TODO: Load actual field definitions from binary codec
			// Example structure:
			// "Account": [8, 1],
			// "Destination": [8, 3],
			// etc.
		},
		"TYPES": map[string]interface{}{
			// TODO: Load actual type definitions
			// Example structure:
			// "UInt8": 1,
			// "UInt16": 2,
			// etc.
		},
		"TRANSACTION_RESULTS": map[string]interface{}{
			// TODO: Load transaction result codes
			// Example:
			// "tesSUCCESS": 0,
			// "tecCLAIM": 100,
			// etc.
		},
		"TRANSACTION_TYPES": map[string]interface{}{
			// TODO: Load transaction type definitions
			// Example:
			// "Payment": 0,
			// "EscrowCreate": 1,
			// etc.
		},
		"LEDGER_ENTRY_TYPES": map[string]interface{}{
			// TODO: Load ledger entry type definitions
			// Example:
			// "AccountRoot": 97,
			// "DirectoryNode": 100,
			// etc.
		},
	}

	return response, nil
}

func (m *ServerDefinitionsMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *ServerDefinitionsMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// FeatureMethod handles the feature RPC method
type FeatureMethod struct{}

func (m *FeatureMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// TODO: Implement amendment/feature status retrieval
	// This method returns information about:
	// - Known amendments and their status (enabled/disabled/voting)
	// - Server's voting preferences
	// - Amendment blocking status
	// This data should come from the amendment table tracking system

	response := map[string]interface{}{
		// TODO: Return actual amendment status
		// Example structure should match rippled format:
		// "amendmentname": {
		//     "enabled": true,
		//     "name": "amendmentname",
		//     "supported": true,
		//     "vetoed": false
		// }
	}
	return response, nil
}

func (m *FeatureMethod) RequiredRole() Role {
	return RoleAdmin // Amendment info requires admin privileges
}

func (m *FeatureMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}

// FeeMethod handles the fee RPC method
type FeeMethod struct{}

func (m *FeeMethod) Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError) {
	// Check if ledger service is available
	if Services == nil || Services.Ledger == nil {
		return nil, RpcErrorInternal("Ledger service not available")
	}

	// Get fee settings from ledger service
	baseFee, _, _ := Services.Ledger.GetCurrentFees()

	// Get current ledger index
	currentLedgerIndex := Services.Ledger.GetCurrentLedgerIndex()

	// Format base fee as string
	baseFeeStr := fmt.Sprintf("%d", baseFee)

	response := map[string]interface{}{
		"current_ledger_size": "0",
		"current_queue_size":  "0",
		"drops": map[string]interface{}{
			"base_fee":        baseFeeStr,
			"median_fee":      baseFeeStr, // In standalone, no median calculation
			"minimum_fee":     baseFeeStr,
			"open_ledger_fee": baseFeeStr,
		},
		"expected_ledger_size": "0",
		"ledger_current_index": currentLedgerIndex,
		"levels": map[string]interface{}{
			"median_level":      "256",
			"minimum_level":     "256",
			"open_ledger_level": "256",
			"reference_level":   "256",
		},
		"max_queue_size": "2000",
	}

	return response, nil
}

func (m *FeeMethod) RequiredRole() Role {
	return RoleGuest
}

func (m *FeeMethod) SupportedApiVersions() []int {
	return []int{ApiVersion1, ApiVersion2, ApiVersion3}
}
