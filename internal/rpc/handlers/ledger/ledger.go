// Package ledger provides ledger-related RPC method handlers.
package ledger

import (
	"context"
	"errors"

	"github.com/LeJamon/goXRPLd/internal/rpc/handlers"
)

// LedgerHandler handles the "ledger" RPC method.
type LedgerHandler struct{}

// NewLedgerHandler creates a new ledger handler.
func NewLedgerHandler() *LedgerHandler {
	return &LedgerHandler{}
}

// Name returns the method name.
func (h *LedgerHandler) Name() string {
	return "ledger"
}

// Handle processes the ledger request.
func (h *LedgerHandler) Handle(ctx context.Context, params map[string]interface{}, services *handlers.Services) (interface{}, error) {
	if services == nil || services.Ledger == nil {
		return nil, errors.New("ledger service not available")
	}

	// Extract ledger_index parameter
	ledgerIndex := "validated"
	if li, ok := params["ledger_index"].(string); ok {
		ledgerIndex = li
	} else if li, ok := params["ledger_index"].(float64); ok {
		ledgerIndex = formatUint32(uint32(li))
	}

	// Extract optional flags
	full := false
	if f, ok := params["full"].(bool); ok {
		full = f
	}

	accounts := false
	if a, ok := params["accounts"].(bool); ok {
		accounts = a
	}

	transactions := false
	if t, ok := params["transactions"].(bool); ok {
		transactions = t
	}

	expand := false
	if e, ok := params["expand"].(bool); ok {
		expand = e
	}

	binary := false
	if b, ok := params["binary"].(bool); ok {
		binary = b
	}

	// Get ledger info
	_ = full
	_ = accounts
	_ = transactions
	_ = expand
	_ = binary

	// For now, return basic info
	var seq uint32
	switch ledgerIndex {
	case "current":
		seq = services.Ledger.GetCurrentLedgerIndex()
	case "closed":
		seq = services.Ledger.GetClosedLedgerIndex()
	case "validated":
		seq = services.Ledger.GetValidatedLedgerIndex()
	default:
		// Parse as number
		for _, c := range ledgerIndex {
			if c >= '0' && c <= '9' {
				seq = seq*10 + uint32(c-'0')
			}
		}
	}

	info, err := services.Ledger.GetLedgerInfo(seq)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"ledger": info,
	}, nil
}

// RequiresAdmin returns false as ledger is a public method.
func (h *LedgerHandler) RequiresAdmin() bool {
	return false
}

// AllowedRoles returns the roles that can call this method.
func (h *LedgerHandler) AllowedRoles() []handlers.Role {
	return []handlers.Role{handlers.RolePublic, handlers.RoleAdmin, handlers.RoleIdentified}
}

// formatUint32 converts a uint32 to string.
func formatUint32(n uint32) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func init() {
	handlers.MustRegister(NewLedgerHandler())
}
