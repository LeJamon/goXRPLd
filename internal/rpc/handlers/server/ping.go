// Package server provides server-related RPC method handlers.
package server

import (
	"context"

	"github.com/LeJamon/goXRPLd/internal/rpc/handlers"
)

// PingHandler handles the "ping" RPC method.
type PingHandler struct{}

// NewPingHandler creates a new ping handler.
func NewPingHandler() *PingHandler {
	return &PingHandler{}
}

// Name returns the method name.
func (h *PingHandler) Name() string {
	return "ping"
}

// Handle processes the ping request.
func (h *PingHandler) Handle(ctx context.Context, params map[string]interface{}, services *handlers.Services) (interface{}, error) {
	return map[string]interface{}{
		"status": "success",
	}, nil
}

// RequiresAdmin returns false as ping is a public method.
func (h *PingHandler) RequiresAdmin() bool {
	return false
}

// AllowedRoles returns the roles that can call this method.
func (h *PingHandler) AllowedRoles() []handlers.Role {
	return []handlers.Role{handlers.RolePublic, handlers.RoleAdmin, handlers.RoleIdentified}
}

func init() {
	handlers.MustRegister(NewPingHandler())
}
