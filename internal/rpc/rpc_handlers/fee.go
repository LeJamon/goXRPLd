package rpc_handlers

import (
	"encoding/json"
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// FeeMethod handles the fee RPC method
type FeeMethod struct{}

func (m *FeeMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// Check if ledger service is available
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	// Get fee settings from ledger service
	baseFee, _, _ := rpc_types.Services.Ledger.GetCurrentFees()

	// Get current ledger index
	currentLedgerIndex := rpc_types.Services.Ledger.GetCurrentLedgerIndex()

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

func (m *FeeMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *FeeMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
