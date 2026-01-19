package rpc_handlers

import (
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// ServerStateMethod handles the server_state RPC method
type ServerStateMethod struct{}

func (m *ServerStateMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// Check if ledger service is available
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	// Get server info from ledger service
	serverInfo := rpc_types.Services.Ledger.GetServerInfo()

	// Get fee settings
	baseFee, reserveBase, reserveIncrement := rpc_types.Services.Ledger.GetCurrentFees()

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

func (m *ServerStateMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *ServerStateMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
