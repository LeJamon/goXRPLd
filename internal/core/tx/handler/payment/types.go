package payment

import (
	"github.com/LeJamon/goXRPLd/internal/core/tx/handler"
)

// Payment represents a Payment transaction.
type Payment struct {
	common         *handler.CommonFields
	Destination    string
	Amount         handler.Amount
	DestinationTag *uint32
	InvoiceID      string
	Paths          [][]PathStep
	SendMax        *handler.Amount
	DeliverMin     *handler.Amount
}

// PathStep represents a single step in a payment path.
type PathStep struct {
	Account  string
	Currency string
	Issuer   string
}

// GetCommon returns the common transaction fields.
func (p *Payment) GetCommon() *handler.CommonFields {
	return p.common
}

// Validate performs payment-specific validation.
func (p *Payment) Validate() error {
	return nil
}

// NewPayment creates a new Payment transaction.
func NewPayment(common *handler.CommonFields, destination string, amount handler.Amount) *Payment {
	return &Payment{
		common:      common,
		Destination: destination,
		Amount:      amount,
	}
}
