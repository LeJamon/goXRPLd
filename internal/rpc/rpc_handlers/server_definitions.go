package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// ServerDefinitionsMethod handles the server_definitions RPC method
type ServerDefinitionsMethod struct{}

func (m *ServerDefinitionsMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
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

func (m *ServerDefinitionsMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *ServerDefinitionsMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
