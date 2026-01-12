package server

import (
	"context"
	"runtime"
	"time"

	"github.com/LeJamon/goXRPLd/internal/rpc/handlers"
)

// ServerInfoHandler handles the "server_info" RPC method.
type ServerInfoHandler struct {
	buildVersion string
	startTime    time.Time
}

// NewServerInfoHandler creates a new server_info handler.
func NewServerInfoHandler(buildVersion string) *ServerInfoHandler {
	return &ServerInfoHandler{
		buildVersion: buildVersion,
		startTime:    time.Now(),
	}
}

// Name returns the method name.
func (h *ServerInfoHandler) Name() string {
	return "server_info"
}

// Handle processes the server_info request.
func (h *ServerInfoHandler) Handle(ctx context.Context, params map[string]interface{}, services *handlers.Services) (interface{}, error) {
	info := make(map[string]interface{})

	// Build info
	info["build_version"] = h.buildVersion
	info["hostid"] = "goXRPLd"

	// Server state
	info["server_state"] = "full"
	info["server_state_duration_us"] = time.Since(h.startTime).Microseconds()

	// Ledger info
	if services != nil && services.Ledger != nil {
		info["validated_ledger"] = map[string]interface{}{
			"seq": services.Ledger.GetValidatedLedgerIndex(),
		}
		info["closed_ledger"] = map[string]interface{}{
			"seq": services.Ledger.GetClosedLedgerIndex(),
		}
	}

	// Runtime info
	info["io_latency_ms"] = 1
	info["time"] = time.Now().UTC().Format(time.RFC3339)
	info["uptime"] = int64(time.Since(h.startTime).Seconds())

	// Platform info
	info["network_id"] = 1
	info["amendment_blocked"] = false

	// Memory info
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	info["jq_trans_overflow"] = "0"
	info["peers"] = 0

	return map[string]interface{}{
		"info": info,
	}, nil
}

// RequiresAdmin returns false as server_info is a public method.
func (h *ServerInfoHandler) RequiresAdmin() bool {
	return false
}

// AllowedRoles returns the roles that can call this method.
func (h *ServerInfoHandler) AllowedRoles() []handlers.Role {
	return []handlers.Role{handlers.RolePublic, handlers.RoleAdmin, handlers.RoleIdentified}
}

func init() {
	handlers.MustRegister(NewServerInfoHandler("1.0.0"))
}
