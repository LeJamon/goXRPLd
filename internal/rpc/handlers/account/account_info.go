// Package account provides account-related RPC method handlers.
package account

import (
	"context"
	"errors"

	"github.com/LeJamon/goXRPLd/internal/rpc/handlers"
)

// AccountInfoHandler handles the "account_info" RPC method.
type AccountInfoHandler struct{}

// NewAccountInfoHandler creates a new account_info handler.
func NewAccountInfoHandler() *AccountInfoHandler {
	return &AccountInfoHandler{}
}

// Name returns the method name.
func (h *AccountInfoHandler) Name() string {
	return "account_info"
}

// Handle processes the account_info request.
func (h *AccountInfoHandler) Handle(ctx context.Context, params map[string]interface{}, services *handlers.Services) (interface{}, error) {
	// Extract account parameter
	account, ok := params["account"].(string)
	if !ok || account == "" {
		return nil, errors.New("account parameter required")
	}

	// Extract optional ledger_index
	ledgerIndex := "validated"
	if li, ok := params["ledger_index"].(string); ok {
		ledgerIndex = li
	}

	// Get account info from service
	if services == nil || services.Account == nil {
		return nil, errors.New("account service not available")
	}

	info, err := services.Account.GetAccountInfo(account, ledgerIndex)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"account_data": info,
	}, nil
}

// RequiresAdmin returns false as account_info is a public method.
func (h *AccountInfoHandler) RequiresAdmin() bool {
	return false
}

// AllowedRoles returns the roles that can call this method.
func (h *AccountInfoHandler) AllowedRoles() []handlers.Role {
	return []handlers.Role{handlers.RolePublic, handlers.RoleAdmin, handlers.RoleIdentified}
}

func init() {
	handlers.MustRegister(NewAccountInfoHandler())
}
