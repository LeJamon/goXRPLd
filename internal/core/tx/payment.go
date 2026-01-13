package tx

import (
	"errors"
)

// Payment transaction moves value from one account to another.
// It is the most fundamental transaction type in the XRPL.
type Payment struct {
	BaseTx

	// Amount is the amount of currency to deliver (required)
	Amount Amount `json:"Amount"`

	// Destination is the account receiving the payment (required)
	Destination string `json:"Destination"`

	// DestinationTag is an arbitrary tag for the destination (optional)
	DestinationTag *uint32 `json:"DestinationTag,omitempty"`

	// InvoiceID is a 256-bit hash for identifying this payment (optional)
	InvoiceID string `json:"InvoiceID,omitempty"`

	// Paths for cross-currency payments (optional)
	Paths [][]PathStep `json:"Paths,omitempty"`

	// SendMax is the maximum amount to send (optional, for cross-currency)
	SendMax *Amount `json:"SendMax,omitempty"`

	// DeliverMin is the minimum amount to deliver (optional, for partial payments)
	DeliverMin *Amount `json:"DeliverMin,omitempty"`
}

// PathStep represents a single step in a payment path
type PathStep struct {
	Account  string `json:"account,omitempty"`
	Currency string `json:"currency,omitempty"`
	Issuer   string `json:"issuer,omitempty"`
	Type     int    `json:"type,omitempty"`
	TypeHex  string `json:"type_hex,omitempty"`
}

// Payment flags
const (
	// tfNoDirectRipple prevents direct rippling
	PaymentFlagNoDirectRipple uint32 = 0x00010000
	// tfPartialPayment allows partial payments
	PaymentFlagPartialPayment uint32 = 0x00020000
	// tfLimitQuality limits quality of paths
	PaymentFlagLimitQuality uint32 = 0x00040000
)

// NewPayment creates a new Payment transaction
func NewPayment(account, destination string, amount Amount) *Payment {
	return &Payment{
		BaseTx:      *NewBaseTx(TypePayment, account),
		Amount:      amount,
		Destination: destination,
	}
}

// TxType returns the transaction type
func (p *Payment) TxType() Type {
	return TypePayment
}

// Validate validates the payment transaction
func (p *Payment) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	if p.Destination == "" {
		return errors.New("Destination is required")
	}

	if p.Amount.Value == "" {
		return errors.New("Amount is required")
	}

	// Cannot send XRP to self
	if p.Account == p.Destination && p.Amount.IsNative() {
		return errors.New("cannot send XRP to self")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (p *Payment) Flatten() (map[string]any, error) {
	m := p.Common.ToMap()

	m["Amount"] = flattenAmount(p.Amount)
	m["Destination"] = p.Destination

	if p.DestinationTag != nil {
		m["DestinationTag"] = *p.DestinationTag
	}
	if p.InvoiceID != "" {
		m["InvoiceID"] = p.InvoiceID
	}
	if len(p.Paths) > 0 {
		m["Paths"] = p.Paths
	}
	if p.SendMax != nil {
		m["SendMax"] = flattenAmount(*p.SendMax)
	}
	if p.DeliverMin != nil {
		m["DeliverMin"] = flattenAmount(*p.DeliverMin)
	}

	return m, nil
}

// SetPartialPayment enables partial payment flag
func (p *Payment) SetPartialPayment() {
	flags := p.GetFlags() | PaymentFlagPartialPayment
	p.SetFlags(flags)
}

// SetNoDirectRipple enables no direct ripple flag
func (p *Payment) SetNoDirectRipple() {
	flags := p.GetFlags() | PaymentFlagNoDirectRipple
	p.SetFlags(flags)
}

// flattenAmount converts an Amount to its JSON representation
func flattenAmount(a Amount) any {
	if a.IsNative() {
		return a.Value
	}
	return map[string]any{
		"value":    a.Value,
		"currency": a.Currency,
		"issuer":   a.Issuer,
	}
}
