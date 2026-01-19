package rpc_handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// RandomMethod handles the random RPC method
type RandomMethod struct{}

func (m *RandomMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// Generate 256 bits (32 bytes) of cryptographically secure random data
	randomBytes := make([]byte, 32)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return nil, rpc_types.RpcErrorInternal("Failed to generate random data: " + err.Error())
	}

	response := map[string]interface{}{
		"random": strings.ToUpper(hex.EncodeToString(randomBytes)),
	}

	return response, nil
}

func (m *RandomMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *RandomMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
