package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// PeersMethod handles the peers RPC method
type PeersMethod struct{}

func (m *PeersMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// TODO: Implement peer listing
	return map[string]interface{}{
		"peers": []interface{}{},
	}, nil
}

func (m *PeersMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *PeersMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// PeerReservationsAddMethod handles the peer_reservations_add RPC method
type PeerReservationsAddMethod struct{}

func (m *PeerReservationsAddMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// TODO: Implement peer reservation addition
	return map[string]interface{}{
		"previous": []interface{}{},
		"current":  []interface{}{},
	}, nil
}

func (m *PeerReservationsAddMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *PeerReservationsAddMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// PeerReservationsDelMethod handles the peer_reservations_del RPC method
type PeerReservationsDelMethod struct{}

func (m *PeerReservationsDelMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// TODO: Implement peer reservation deletion
	return map[string]interface{}{
		"previous": []interface{}{},
		"current":  []interface{}{},
	}, nil
}

func (m *PeerReservationsDelMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *PeerReservationsDelMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}

// PeerReservationsListMethod handles the peer_reservations_list RPC method
type PeerReservationsListMethod struct{}

func (m *PeerReservationsListMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	// TODO: Implement peer reservation listing
	return map[string]interface{}{
		"reservations": []interface{}{},
	}, nil
}

func (m *PeerReservationsListMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleAdmin
}

func (m *PeerReservationsListMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
