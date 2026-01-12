// Package handlers provides the RPC method handler interface and registry.
package handlers

import (
	"context"
)

// Handler defines the interface for RPC method handlers.
type Handler interface {
	// Name returns the RPC method name.
	Name() string

	// Handle processes the RPC request and returns a response.
	Handle(ctx context.Context, params map[string]interface{}, services *Services) (interface{}, error)

	// RequiresAdmin returns true if the method requires admin privileges.
	RequiresAdmin() bool

	// AllowedRoles returns the roles allowed to call this method.
	AllowedRoles() []Role
}

// Role represents an RPC role.
type Role int

const (
	// RolePublic is the default role for public methods.
	RolePublic Role = iota

	// RoleAdmin is for administrative methods.
	RoleAdmin

	// RoleIdentified is for methods requiring user identification.
	RoleIdentified
)

// Services provides access to all services needed by RPC handlers.
type Services struct {
	// Ledger provides access to ledger operations.
	Ledger LedgerService

	// Account provides access to account operations.
	Account AccountService

	// Transaction provides access to transaction operations.
	Transaction TransactionService

	// Subscription provides access to subscription management.
	Subscription SubscriptionService
}

// LedgerService defines ledger-related operations.
type LedgerService interface {
	GetCurrentLedgerIndex() uint32
	GetClosedLedgerIndex() uint32
	GetValidatedLedgerIndex() uint32
	AcceptLedger() (uint32, error)
	GetLedgerInfo(seq uint32) (interface{}, error)
	GetLedgerData(ledgerIndex string, limit uint32, marker string) (interface{}, error)
	GetLedgerEntry(entryKey [32]byte, ledgerIndex string) (interface{}, error)
}

// AccountService defines account-related operations.
type AccountService interface {
	GetAccountInfo(account string, ledgerIndex string) (interface{}, error)
	GetAccountLines(account string, ledgerIndex string, peer string, limit uint32) (interface{}, error)
	GetAccountOffers(account string, ledgerIndex string, limit uint32) (interface{}, error)
	GetAccountObjects(account string, ledgerIndex string, objType string, limit uint32) (interface{}, error)
	GetAccountTransactions(account string, ledgerMin, ledgerMax int64, limit uint32, forward bool) (interface{}, error)
}

// TransactionService defines transaction-related operations.
type TransactionService interface {
	SubmitTransaction(txBlob string) (interface{}, error)
	GetTransaction(txHash [32]byte) (interface{}, error)
	SignTransaction(tx interface{}, secret string) (interface{}, error)
}

// SubscriptionService defines subscription-related operations.
type SubscriptionService interface {
	Subscribe(streams []string, accounts []string, accountsProposed []string, books []interface{}) error
	Unsubscribe(streams []string, accounts []string, accountsProposed []string, books []interface{}) error
}
