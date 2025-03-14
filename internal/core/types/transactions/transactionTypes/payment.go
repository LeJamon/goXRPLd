package transactionTypes

import (
	"github.com/LeJamon/goXRPLd/internal/core/types/transactions"
)

type CurrencyAmount struct {
	Currency string `json:"currency"`
	Value    string `json:"value"`
	Issuer   string `json:"issuer,omitempty"` // Omit for XRP amounts
}

// PaymentTransaction represents a Payment-specific transaction.
type PaymentTransaction struct {
	transactions.TransactionCommonField
	Amount         *CurrencyAmount   `json:"Amount,omitempty"`     // Alias to DeliverMax
	DeliverMax     *CurrencyAmount   `json:"DeliverMax,omitempty"` // Maximum amount to deliver
	DeliverMin     *CurrencyAmount   `json:"DeliverMin,omitempty"` // Minimum amount to deliver (optional)
	Destination    string            `json:"Destination" validate:"required"`
	DestinationTag *uint32           `json:"DestinationTag,omitempty"` // Optional tag
	InvoiceID      string            `json:"InvoiceID,omitempty"`      // Optional invoice identifier
	Paths          [][][]PathElement `json:"Paths,omitempty"`          // Optional paths for non-XRP transactions
	SendMax        *CurrencyAmount   `json:"SendMax,omitempty"`        // Optional maximum cost
}

// PathElement represents a single element in a payment path.
type PathElement struct {
	Account  string `json:"account,omitempty"`  // Account for the path element
	Currency string `json:"currency,omitempty"` // Currency for the path element
	Issuer   string `json:"issuer,omitempty"`   // Issuer for the path element
}

// NewPaymentTransaction creates a new PaymentTransaction.
func NewPaymentTransaction(account string, destination string) *PaymentTransaction {
	return &PaymentTransaction{
		TransactionCommonField: transactions.TransactionCommonField{
			Account:         account,
			TransactionType: "Payment",
		},
		Destination: destination,
	}
}
