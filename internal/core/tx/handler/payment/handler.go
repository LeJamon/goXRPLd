// Package payment implements the Payment transaction handler.
package payment

import (
	"github.com/LeJamon/goXRPLd/internal/core/tx/handler"
)

// TransactionType is the XRPL transaction type for payments.
const TransactionType = "Payment"

// Handler processes Payment transactions.
type Handler struct {
	handler.BaseHandler
}

// New creates a new payment handler.
func New() *Handler {
	return &Handler{
		BaseHandler: handler.NewBaseHandler(TransactionType),
	}
}

// Preflight performs payment-specific preflight validation.
func (h *Handler) Preflight(tx handler.Transaction, ctx *handler.Context) handler.Result {
	// First do base validation
	if result := h.BaseHandler.Preflight(tx, ctx); !result.IsSuccess() {
		return result
	}

	// Payment-specific validation
	payment, ok := tx.(*Payment)
	if !ok {
		return handler.TemINVALID
	}

	// Destination is required
	if payment.Destination == "" {
		return handler.TemDST_NEEDED
	}

	// Cannot pay self
	if payment.Destination == payment.GetCommon().Account {
		return handler.TemDST_IS_SRC
	}

	// Amount must be valid
	if payment.Amount.Value == "" {
		return handler.TemBAD_AMOUNT
	}

	return handler.TesSUCCESS
}

// Apply processes the payment and updates the ledger.
func (h *Handler) Apply(tx handler.Transaction, account *handler.AccountRoot, ctx *handler.Context) handler.Result {
	payment, ok := tx.(*Payment)
	if !ok {
		return handler.TefINTERNAL
	}

	// Dispatch to appropriate payment type
	if payment.Amount.IsNative() {
		return h.applyXRPPayment(payment, account, ctx)
	}
	return h.applyIOUPayment(payment, account, ctx)
}

// Register registers the payment handler with the default registry.
func Register() {
	handler.MustRegister(New())
}

func init() {
	// Auto-register when package is imported
	Register()
}
