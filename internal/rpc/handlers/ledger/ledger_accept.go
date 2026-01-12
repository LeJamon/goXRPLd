package ledger

import (
	"context"
	"errors"

	"github.com/LeJamon/goXRPLd/internal/rpc/handlers"
)

// LedgerAcceptHandler handles the "ledger_accept" RPC method.
// This is an admin-only method for advancing ledgers in standalone mode.
type LedgerAcceptHandler struct{}

// NewLedgerAcceptHandler creates a new ledger_accept handler.
func NewLedgerAcceptHandler() *LedgerAcceptHandler {
	return &LedgerAcceptHandler{}
}

// Name returns the method name.
func (h *LedgerAcceptHandler) Name() string {
	return "ledger_accept"
}

// Handle processes the ledger_accept request.
func (h *LedgerAcceptHandler) Handle(ctx context.Context, params map[string]interface{}, services *handlers.Services) (interface{}, error) {
	if services == nil || services.Ledger == nil {
		return nil, errors.New("ledger service not available")
	}

	seq, err := services.Ledger.AcceptLedger()
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"ledger_current_index": seq,
	}, nil
}

// RequiresAdmin returns true as ledger_accept requires admin privileges.
func (h *LedgerAcceptHandler) RequiresAdmin() bool {
	return true
}

// AllowedRoles returns the roles that can call this method.
func (h *LedgerAcceptHandler) AllowedRoles() []handlers.Role {
	return []handlers.Role{handlers.RoleAdmin}
}

func init() {
	handlers.MustRegister(NewLedgerAcceptHandler())
}
