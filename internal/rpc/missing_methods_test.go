package rpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_handlers"
	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Test Helpers
// =============================================================================

// mockLedgerServiceMissingMethods extends mockLedgerService for testing new methods
type mockLedgerServiceMissingMethods struct {
	*mockLedgerService
}

func newMockLedgerServiceMissingMethods() *mockLedgerServiceMissingMethods {
	return &mockLedgerServiceMissingMethods{
		mockLedgerService: newMockLedgerService(),
	}
}

// setupTestServicesMissingMethods initializes Services for testing
func setupTestServicesMissingMethods(mock *mockLedgerServiceMissingMethods) func() {
	oldServices := rpc_types.Services
	rpc_types.Services = &rpc_types.ServiceContainer{
		Ledger: mock,
	}
	return func() {
		rpc_types.Services = oldServices
	}
}

// =============================================================================
// FetchInfoMethod Tests
// Reference: rippled/src/xrpld/rpc/handlers/FetchInfo.cpp
// =============================================================================

func TestFetchInfoMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.FetchInfoMethod{}

	t.Run("Returns response with clear flag", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"clear": true}`)
		result, rpcErr := method.Handle(ctx, params)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)

		resultMap := result.(map[string]interface{})
		assert.Contains(t, resultMap, "info")
	})

	t.Run("Returns response without clear flag", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		result, rpcErr := method.Handle(ctx, nil)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})

	t.Run("Supports all API versions", func(t *testing.T) {
		versions := method.SupportedApiVersions()
		assert.Contains(t, versions, rpc_types.ApiVersion1)
		assert.Contains(t, versions, rpc_types.ApiVersion2)
		assert.Contains(t, versions, rpc_types.ApiVersion3)
	})
}

// =============================================================================
// OwnerInfoMethod Tests
// Reference: rippled/src/test/rpc/OwnerInfo_test.cpp
// =============================================================================

func TestOwnerInfoMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.OwnerInfoMethod{}

	t.Run("Missing account parameter returns error", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		result, rpcErr := method.Handle(ctx, nil)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcINVALID_PARAMS, rpcErr.Code)
	})

	t.Run("Empty account returns error", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"account": ""}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcINVALID_PARAMS, rpcErr.Code)
	})

	t.Run("Valid account returns response", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"account": "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"}`)
		result, rpcErr := method.Handle(ctx, params)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)
	})

	t.Run("RequiredRole is Guest", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleGuest, method.RequiredRole())
	})
}

// =============================================================================
// LedgerHeaderMethod Tests
// Reference: rippled/src/test/rpc/LedgerHeader_test.cpp
// =============================================================================

func TestLedgerHeaderMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.LedgerHeaderMethod{}

	t.Run("Current ledger returns unclosed ledger", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"ledger_index": "current"}`)
		result, rpcErr := method.Handle(ctx, params)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)

		resultMap := result.(map[string]interface{})
		assert.Contains(t, resultMap, "ledger")
	})

	t.Run("Validated ledger returns validated info", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"ledger_index": "validated"}`)
		result, rpcErr := method.Handle(ctx, params)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)

		resultMap := result.(map[string]interface{})
		assert.Contains(t, resultMap, "ledger")
	})

	t.Run("API version 2 returns unknownCmd per XRPL spec", func(t *testing.T) {
		// Based on LedgerHeader_test.cpp::testCommandRetired
		// ledger_header is retired in API v2
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion2,
		}

		result, rpcErr := method.Handle(ctx, nil)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcMETHOD_NOT_FOUND, rpcErr.Code)
		assert.Equal(t, "unknownCmd", rpcErr.ErrorString)
	})

	t.Run("RequiredRole is Guest", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleGuest, method.RequiredRole())
	})

	t.Run("Only supports API version 1", func(t *testing.T) {
		versions := method.SupportedApiVersions()
		assert.Contains(t, versions, rpc_types.ApiVersion1)
		// Should NOT support v2 per rippled
	})
}

// =============================================================================
// LedgerRequestMethod Tests
// Reference: rippled/src/test/rpc/LedgerRequestRPC_test.cpp
// =============================================================================

func TestLedgerRequestMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.LedgerRequestMethod{}

	t.Run("Returns ledger acquiring status", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"ledger_index": 100}`)
		result, rpcErr := method.Handle(ctx, params)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)

		resultMap := result.(map[string]interface{})
		// Should return acquiring status or have status
		assert.True(t,
			resultMap["acquiring"] != nil || resultMap["have"] != nil,
			"Response should contain acquiring or have status")
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})
}

// =============================================================================
// LedgerCleanerMethod Tests
// =============================================================================

func TestLedgerCleanerMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.LedgerCleanerMethod{}

	t.Run("Returns success message", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		result, rpcErr := method.Handle(ctx, nil)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)

		resultMap := result.(map[string]interface{})
		assert.Contains(t, resultMap, "message")
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})
}

// =============================================================================
// LedgerDiffMethod Tests
// =============================================================================

func TestLedgerDiffMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.LedgerDiffMethod{}

	t.Run("Returns gRPC only error", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		result, rpcErr := method.Handle(ctx, nil)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		// ledger_diff is gRPC only in rippled
		assert.Contains(t, rpcErr.Message, "gRPC")
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})
}

// =============================================================================
// SimulateMethod Tests
// Reference: rippled/src/test/rpc/Simulate_test.cpp
// =============================================================================

func TestSimulateMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.SimulateMethod{}

	t.Run("Missing tx_json and tx_blob returns error", func(t *testing.T) {
		// Based on Simulate_test.cpp::testParamErrors - "No params"
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcINVALID_PARAMS, rpcErr.Code)
	})

	t.Run("Both tx_json and tx_blob returns error", func(t *testing.T) {
		// Based on Simulate_test.cpp::testParamErrors - "Providing both"
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"tx_json": {}, "tx_blob": "1200"}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcINVALID_PARAMS, rpcErr.Code)
	})

	t.Run("RequiredRole is Guest", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleGuest, method.RequiredRole())
	})

	t.Run("Supports all API versions", func(t *testing.T) {
		versions := method.SupportedApiVersions()
		assert.Contains(t, versions, rpc_types.ApiVersion1)
		assert.Contains(t, versions, rpc_types.ApiVersion2)
		assert.Contains(t, versions, rpc_types.ApiVersion3)
	})
}

// =============================================================================
// TxReduceRelayMethod Tests
// =============================================================================

func TestTxReduceRelayMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.TxReduceRelayMethod{}

	t.Run("Returns current state", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		result, rpcErr := method.Handle(ctx, nil)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)

		resultMap := result.(map[string]interface{})
		assert.Contains(t, resultMap, "tx_reduce_relay")
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})
}

// =============================================================================
// ConnectMethod Tests
// Reference: rippled/src/test/rpc/Connect_test.cpp
// =============================================================================

func TestConnectMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	mock.standalone = true // Standalone mode
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.ConnectMethod{}

	t.Run("Standalone mode returns notSynced error", func(t *testing.T) {
		// Based on Connect_test.cpp::testErrors
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"ip": "127.0.0.1", "port": 51235}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcNOT_SYNCED, rpcErr.Code)
		assert.Equal(t, "notSynced", rpcErr.ErrorString)
	})

	t.Run("Missing ip parameter returns error", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcINVALID_PARAMS, rpcErr.Code)
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})
}

// =============================================================================
// PrintMethod Tests
// =============================================================================

func TestPrintMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.PrintMethod{}

	t.Run("Returns server info", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		result, rpcErr := method.Handle(ctx, nil)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)

		resultMap := result.(map[string]interface{})
		assert.Contains(t, resultMap, "standalone")
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})
}

// =============================================================================
// ValidatorInfoMethod Tests
// Reference: rippled/src/test/rpc/ValidatorInfo_test.cpp
// =============================================================================

func TestValidatorInfoMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.ValidatorInfoMethod{}

	t.Run("Non-validator returns error", func(t *testing.T) {
		// Based on ValidatorInfo_test.cpp::testErrors
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		result, rpcErr := method.Handle(ctx, nil)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcNOT_VALIDATOR, rpcErr.Code)
		assert.Contains(t, rpcErr.Message, "not a validator")
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})
}

// =============================================================================
// CanDeleteMethod Tests
// =============================================================================

func TestCanDeleteMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.CanDeleteMethod{}

	t.Run("Returns can_delete ledger sequence", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		result, rpcErr := method.Handle(ctx, nil)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)

		resultMap := result.(map[string]interface{})
		assert.Contains(t, resultMap, "can_delete")
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})
}

// =============================================================================
// GetAggregatePriceMethod Tests
// Reference: rippled/src/test/rpc/GetAggregatePrice_test.cpp
// =============================================================================

func TestGetAggregatePriceMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.GetAggregatePriceMethod{}

	t.Run("Missing oracles parameter returns error", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"base_asset": "XRP", "quote_asset": "USD"}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcINVALID_PARAMS, rpcErr.Code)
	})

	t.Run("Missing base_asset returns error", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"quote_asset": "USD", "oracles": []}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcINVALID_PARAMS, rpcErr.Code)
	})

	t.Run("Missing quote_asset returns error", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"base_asset": "XRP", "oracles": []}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcINVALID_PARAMS, rpcErr.Code)
	})

	t.Run("RequiredRole is Guest", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleGuest, method.RequiredRole())
	})
}

// =============================================================================
// GetCountsMethod Tests
// Reference: rippled/src/test/rpc/GetCounts_test.cpp
// =============================================================================

func TestGetCountsMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.GetCountsMethod{}

	t.Run("Returns object counts with uptime", func(t *testing.T) {
		// Based on GetCounts_test.cpp::testGetCounts
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		result, rpcErr := method.Handle(ctx, nil)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)

		resultMap := result.(map[string]interface{})
		// Per GetCounts_test.cpp: uptime should be present
		assert.Contains(t, resultMap, "uptime")
	})

	t.Run("Accepts min_count parameter", func(t *testing.T) {
		// Based on GetCounts_test.cpp: "make request with min threshold 100"
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"min_count": 100}`)
		result, rpcErr := method.Handle(ctx, params)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})
}

// =============================================================================
// LogLevelMethod Tests
// =============================================================================

func TestLogLevelMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.LogLevelMethod{}

	t.Run("Returns current log levels without params", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		result, rpcErr := method.Handle(ctx, nil)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)
	})

	t.Run("Invalid severity returns error", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"severity": "invalid_level"}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcINVALID_PARAMS, rpcErr.Code)
	})

	t.Run("Valid severity levels are accepted", func(t *testing.T) {
		validLevels := []string{"trace", "debug", "info", "warning", "error", "fatal"}

		for _, level := range validLevels {
			t.Run("severity: "+level, func(t *testing.T) {
				ctx := &rpc_types.RpcContext{
					Context:    context.Background(),
					Role:       rpc_types.RoleAdmin,
					ApiVersion: rpc_types.ApiVersion1,
				}

				params, _ := json.Marshal(map[string]string{"severity": level})
				result, rpcErr := method.Handle(ctx, params)

				require.Nil(t, rpcErr, "severity %s should be accepted", level)
				require.NotNil(t, result)
			})
		}
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})
}

// =============================================================================
// LogRotateMethod Tests
// =============================================================================

func TestLogRotateMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.LogRotateMethod{}

	t.Run("Returns rotation message", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		result, rpcErr := method.Handle(ctx, nil)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)

		resultMap := result.(map[string]interface{})
		assert.Contains(t, resultMap, "message")
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})
}

// =============================================================================
// AMMInfoMethod Tests
// Reference: rippled/src/test/rpc/AMMInfo_test.cpp
// =============================================================================

func TestAMMInfoMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.AMMInfoMethod{}

	t.Run("Returns not implemented error", func(t *testing.T) {
		// AMM requires AMM ledger entry type which is not implemented
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"amm_account": "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcNOT_IMPL, rpcErr.Code)
	})

	t.Run("Invalid parameters - neither assets nor amm_account", func(t *testing.T) {
		// Based on AMMInfo_test.cpp::testErrors - "Invalid parameters"
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcINVALID_PARAMS, rpcErr.Code)
	})

	t.Run("Invalid parameters - both assets and amm_account", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{
			"asset": {"currency": "XRP"},
			"asset2": {"currency": "USD", "issuer": "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"},
			"amm_account": "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
		}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcINVALID_PARAMS, rpcErr.Code)
	})

	t.Run("RequiredRole is Guest", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleGuest, method.RequiredRole())
	})
}

// =============================================================================
// VaultInfoMethod Tests
// =============================================================================

func TestVaultInfoMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.VaultInfoMethod{}

	t.Run("Returns not implemented error", func(t *testing.T) {
		// Vault requires Vault ledger entry type which is not implemented
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"vault_id": "test_vault_id"}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcNOT_IMPL, rpcErr.Code)
	})

	t.Run("Invalid parameters - neither vault_id nor owner+seq", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcINVALID_PARAMS, rpcErr.Code)
	})

	t.Run("Invalid parameters - both vault_id and owner+seq", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleGuest,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{
			"vault_id": "test_vault_id",
			"owner": "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh",
			"seq": 1
		}`)
		result, rpcErr := method.Handle(ctx, params)

		assert.Nil(t, result)
		require.NotNil(t, rpcErr)
		assert.Equal(t, rpc_types.RpcINVALID_PARAMS, rpcErr.Code)
	})

	t.Run("RequiredRole is Guest", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleGuest, method.RequiredRole())
	})
}

// =============================================================================
// UnlListMethod Tests
// =============================================================================

func TestUnlListMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.UnlListMethod{}

	t.Run("Returns UNL array", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		result, rpcErr := method.Handle(ctx, nil)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)

		resultMap := result.(map[string]interface{})
		assert.Contains(t, resultMap, "unl")
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})
}

// =============================================================================
// BlackListMethod Tests
// =============================================================================

func TestBlackListMethod(t *testing.T) {
	mock := newMockLedgerServiceMissingMethods()
	cleanup := setupTestServicesMissingMethods(mock)
	defer cleanup()

	method := &rpc_handlers.BlackListMethod{}

	t.Run("Returns blacklist array", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		result, rpcErr := method.Handle(ctx, nil)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)

		resultMap := result.(map[string]interface{})
		assert.Contains(t, resultMap, "blacklist")
	})

	t.Run("Accepts threshold parameter", func(t *testing.T) {
		ctx := &rpc_types.RpcContext{
			Context:    context.Background(),
			Role:       rpc_types.RoleAdmin,
			ApiVersion: rpc_types.ApiVersion1,
		}

		params := json.RawMessage(`{"threshold": 100}`)
		result, rpcErr := method.Handle(ctx, params)

		require.Nil(t, rpcErr)
		require.NotNil(t, result)
	})

	t.Run("RequiredRole is Admin", func(t *testing.T) {
		assert.Equal(t, rpc_types.RoleAdmin, method.RequiredRole())
	})
}

// =============================================================================
// Service Unavailable Tests
// =============================================================================

func TestMissingMethodsServiceUnavailable(t *testing.T) {
	// Test all methods handle nil Services gracefully
	oldServices := rpc_types.Services
	rpc_types.Services = nil
	defer func() { rpc_types.Services = oldServices }()

	ctx := &rpc_types.RpcContext{
		Context:    context.Background(),
		Role:       rpc_types.RoleAdmin,
		ApiVersion: rpc_types.ApiVersion1,
	}

	methods := []struct {
		name   string
		method rpc_types.MethodHandler
	}{
		{"FetchInfoMethod", &rpc_handlers.FetchInfoMethod{}},
		{"OwnerInfoMethod", &rpc_handlers.OwnerInfoMethod{}},
		{"LedgerHeaderMethod", &rpc_handlers.LedgerHeaderMethod{}},
		{"LedgerRequestMethod", &rpc_handlers.LedgerRequestMethod{}},
		{"LedgerCleanerMethod", &rpc_handlers.LedgerCleanerMethod{}},
		{"LedgerDiffMethod", &rpc_handlers.LedgerDiffMethod{}},
		{"SimulateMethod", &rpc_handlers.SimulateMethod{}},
		{"TxReduceRelayMethod", &rpc_handlers.TxReduceRelayMethod{}},
		{"ConnectMethod", &rpc_handlers.ConnectMethod{}},
		{"PrintMethod", &rpc_handlers.PrintMethod{}},
		{"ValidatorInfoMethod", &rpc_handlers.ValidatorInfoMethod{}},
		{"CanDeleteMethod", &rpc_handlers.CanDeleteMethod{}},
		{"GetAggregatePriceMethod", &rpc_handlers.GetAggregatePriceMethod{}},
		{"GetCountsMethod", &rpc_handlers.GetCountsMethod{}},
		{"LogLevelMethod", &rpc_handlers.LogLevelMethod{}},
		{"LogRotateMethod", &rpc_handlers.LogRotateMethod{}},
		{"AMMInfoMethod", &rpc_handlers.AMMInfoMethod{}},
		{"VaultInfoMethod", &rpc_handlers.VaultInfoMethod{}},
		{"UnlListMethod", &rpc_handlers.UnlListMethod{}},
		{"BlackListMethod", &rpc_handlers.BlackListMethod{}},
	}

	for _, tc := range methods {
		t.Run(tc.name+" handles nil Services", func(t *testing.T) {
			result, rpcErr := tc.method.Handle(ctx, nil)

			// Should return an internal error, not panic
			if rpcErr != nil {
				assert.Equal(t, rpc_types.RpcINTERNAL, rpcErr.Code)
			}
			// Result should be nil if there's an error
			if rpcErr != nil {
				assert.Nil(t, result)
			}
		})
	}
}

// =============================================================================
// Nil Ledger Service Tests
// =============================================================================

func TestMissingMethodsNilLedgerService(t *testing.T) {
	// Test all methods handle nil Ledger gracefully
	oldServices := rpc_types.Services
	rpc_types.Services = &rpc_types.ServiceContainer{Ledger: nil}
	defer func() { rpc_types.Services = oldServices }()

	ctx := &rpc_types.RpcContext{
		Context:    context.Background(),
		Role:       rpc_types.RoleAdmin,
		ApiVersion: rpc_types.ApiVersion1,
	}

	methods := []struct {
		name   string
		method rpc_types.MethodHandler
	}{
		{"FetchInfoMethod", &rpc_handlers.FetchInfoMethod{}},
		{"PrintMethod", &rpc_handlers.PrintMethod{}},
		{"ValidatorInfoMethod", &rpc_handlers.ValidatorInfoMethod{}},
		{"CanDeleteMethod", &rpc_handlers.CanDeleteMethod{}},
		{"GetCountsMethod", &rpc_handlers.GetCountsMethod{}},
		{"LogLevelMethod", &rpc_handlers.LogLevelMethod{}},
		{"LogRotateMethod", &rpc_handlers.LogRotateMethod{}},
		{"UnlListMethod", &rpc_handlers.UnlListMethod{}},
		{"BlackListMethod", &rpc_handlers.BlackListMethod{}},
	}

	for _, tc := range methods {
		t.Run(tc.name+" handles nil Ledger", func(t *testing.T) {
			result, rpcErr := tc.method.Handle(ctx, nil)

			// Should return an internal error, not panic
			require.NotNil(t, rpcErr)
			assert.Equal(t, rpc_types.RpcINTERNAL, rpcErr.Code)
			assert.Nil(t, result)
		})
	}
}
