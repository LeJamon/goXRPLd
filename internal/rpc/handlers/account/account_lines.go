package account

import (
	"context"
	"errors"

	"github.com/LeJamon/goXRPLd/internal/rpc/handlers"
)

// AccountLinesHandler handles the "account_lines" RPC method.
type AccountLinesHandler struct{}

// NewAccountLinesHandler creates a new account_lines handler.
func NewAccountLinesHandler() *AccountLinesHandler {
	return &AccountLinesHandler{}
}

// Name returns the method name.
func (h *AccountLinesHandler) Name() string {
	return "account_lines"
}

// Handle processes the account_lines request.
func (h *AccountLinesHandler) Handle(ctx context.Context, params map[string]interface{}, services *handlers.Services) (interface{}, error) {
	// Extract account parameter
	account, ok := params["account"].(string)
	if !ok || account == "" {
		return nil, errors.New("account parameter required")
	}

	// Extract optional parameters
	ledgerIndex := "validated"
	if li, ok := params["ledger_index"].(string); ok {
		ledgerIndex = li
	}

	peer := ""
	if p, ok := params["peer"].(string); ok {
		peer = p
	}

	limit := uint32(200)
	if l, ok := params["limit"].(float64); ok {
		limit = uint32(l)
	}

	// Get account lines from service
	if services == nil || services.Account == nil {
		return nil, errors.New("account service not available")
	}

	lines, err := services.Account.GetAccountLines(account, ledgerIndex, peer, limit)
	if err != nil {
		return nil, err
	}

	return lines, nil
}

// RequiresAdmin returns false as account_lines is a public method.
func (h *AccountLinesHandler) RequiresAdmin() bool {
	return false
}

// AllowedRoles returns the roles that can call this method.
func (h *AccountLinesHandler) AllowedRoles() []handlers.Role {
	return []handlers.Role{handlers.RolePublic, handlers.RoleAdmin, handlers.RoleIdentified}
}

func init() {
	handlers.MustRegister(NewAccountLinesHandler())
}
