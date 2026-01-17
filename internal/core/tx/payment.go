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
	// tfNoDirectRipple prevents direct rippling (tfNoRippleDirect in rippled)
	PaymentFlagNoDirectRipple uint32 = 0x00010000
	// tfPartialPayment allows partial payments
	PaymentFlagPartialPayment uint32 = 0x00020000
	// tfLimitQuality limits quality of paths
	PaymentFlagLimitQuality uint32 = 0x00040000
)

// Path constraints matching rippled
const (
	// MaxPathSize is the maximum number of paths in a payment (rippled: MaxPathSize = 7)
	MaxPathSize = 7
	// MaxPathLength is the maximum number of steps per path (rippled: MaxPathLength = 8)
	MaxPathLength = 8
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

	// Determine if this is an XRP-to-XRP (direct) payment
	xrpDirect := p.Amount.IsNative() && (p.SendMax == nil || p.SendMax.IsNative())

	// Check flags based on payment type
	flags := p.GetFlags()
	partialPaymentAllowed := (flags & PaymentFlagPartialPayment) != 0

	// tfPartialPayment flag is invalid for XRP-to-XRP payments (temBAD_SEND_XRP_PARTIAL)
	// Reference: rippled Payment.cpp:182-188
	if xrpDirect && partialPaymentAllowed {
		return errors.New("temBAD_SEND_XRP_PARTIAL: Partial payment specified for XRP to XRP")
	}

	// DeliverMin can only be used with tfPartialPayment flag (temBAD_AMOUNT)
	// Reference: rippled Payment.cpp:206-214
	if p.DeliverMin != nil && !partialPaymentAllowed {
		return errors.New("temBAD_AMOUNT: DeliverMin requires tfPartialPayment flag")
	}

	// Validate DeliverMin if present
	// Reference: rippled Payment.cpp:216-238
	if p.DeliverMin != nil {
		// DeliverMin must be positive (not zero, not empty, not negative)
		if p.DeliverMin.Value == "" || p.DeliverMin.Value == "0" {
			return errors.New("temBAD_AMOUNT: DeliverMin must be positive")
		}
		// Check for negative values
		if len(p.DeliverMin.Value) > 0 && p.DeliverMin.Value[0] == '-' {
			return errors.New("temBAD_AMOUNT: DeliverMin must be positive")
		}

		// DeliverMin currency must match Amount currency
		if p.DeliverMin.Currency != p.Amount.Currency || p.DeliverMin.Issuer != p.Amount.Issuer {
			return errors.New("temBAD_AMOUNT: DeliverMin currency must match Amount")
		}
	}

	// Paths array max length is 7 (temMALFORMED if exceeded)
	// Reference: rippled Payment.cpp:353-359 (MaxPathSize)
	if len(p.Paths) > MaxPathSize {
		return errors.New("temMALFORMED: Paths array exceeds maximum size of 7")
	}

	// Each path can have max 8 steps (temMALFORMED if exceeded)
	// Reference: rippled Payment.cpp:354-358 (MaxPathLength)
	for i, path := range p.Paths {
		if len(path) > MaxPathLength {
			return errors.New("temMALFORMED: Path " + string(rune('0'+i)) + " exceeds maximum length of 8 steps")
		}
	}

	// Cannot send XRP to self without paths (temREDUNDANT)
	// Reference: rippled Payment.cpp:159-167
	if p.Account == p.Destination && p.Amount.IsNative() && len(p.Paths) == 0 {
		return errors.New("temREDUNDANT: cannot send XRP to self without path")
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
