package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// VersionMethod handles the version RPC method
// This method returns API version information for the server.
//
// Reference: rippled/src/xrpld/rpc/handlers/Version.h (VersionHandler)
type VersionMethod struct{}

func (m *VersionMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// TODO: Implement version handler
	//
	// Request parameters:
	//   - None
	//
	// Response fields:
	//   - version: object containing API version information
	//     - first: minimum supported API version
	//     - last: maximum supported API version
	//     - good: recommended API version
	//
	// Implementation notes:
	// 1. Return the supported API version range
	// 2. In rippled, this uses RPC::setVersion() helper which populates:
	//    - first: apiMinimumSupportedVersion
	//    - last: apiMaximumSupportedVersion (or apiBetaVersion if beta enabled)
	//    - good: apiVersionIfUnspecified (the default/recommended version)

	response := map[string]interface{}{
		"version": map[string]interface{}{
			"first": rpc_types.ApiVersion1,
			"last":  rpc_types.ApiVersion3,
			"good":  rpc_types.ApiVersion2,
		},
	}

	return response, nil
}

func (m *VersionMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *VersionMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
