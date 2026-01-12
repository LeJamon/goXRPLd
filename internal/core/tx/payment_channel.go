package tx

import "errors"

// PaymentChannelCreate creates a payment channel.
type PaymentChannelCreate struct {
	BaseTx

	// Amount is the amount of XRP to lock in the channel (required)
	Amount Amount `json:"Amount"`

	// Destination is the account to receive channel payments (required)
	Destination string `json:"Destination"`

	// SettleDelay is the time in seconds to wait after close (required)
	SettleDelay uint32 `json:"SettleDelay"`

	// PublicKey is the public key for verifying claims (required)
	PublicKey string `json:"PublicKey"`

	// CancelAfter is the time when the channel expires (optional)
	CancelAfter *uint32 `json:"CancelAfter,omitempty"`

	// DestinationTag is an arbitrary tag for the destination (optional)
	DestinationTag *uint32 `json:"DestinationTag,omitempty"`
}

// NewPaymentChannelCreate creates a new PaymentChannelCreate transaction
func NewPaymentChannelCreate(account, destination string, amount Amount, settleDelay uint32, publicKey string) *PaymentChannelCreate {
	return &PaymentChannelCreate{
		BaseTx:      *NewBaseTx(TypePaymentChannelCreate, account),
		Amount:      amount,
		Destination: destination,
		SettleDelay: settleDelay,
		PublicKey:   publicKey,
	}
}

// TxType returns the transaction type
func (p *PaymentChannelCreate) TxType() Type {
	return TypePaymentChannelCreate
}

// Validate validates the PaymentChannelCreate transaction
func (p *PaymentChannelCreate) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	if p.Destination == "" {
		return errors.New("Destination is required")
	}

	if p.Amount.Value == "" {
		return errors.New("Amount is required")
	}

	// Must be XRP
	if !p.Amount.IsNative() {
		return errors.New("payment channels can only hold XRP")
	}

	if p.PublicKey == "" {
		return errors.New("PublicKey is required")
	}

	// Cannot create channel to self
	if p.Account == p.Destination {
		return errors.New("cannot create payment channel to self")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (p *PaymentChannelCreate) Flatten() (map[string]any, error) {
	m := p.Common.ToMap()

	m["Amount"] = p.Amount.Value
	m["Destination"] = p.Destination
	m["SettleDelay"] = p.SettleDelay
	m["PublicKey"] = p.PublicKey

	if p.CancelAfter != nil {
		m["CancelAfter"] = *p.CancelAfter
	}
	if p.DestinationTag != nil {
		m["DestinationTag"] = *p.DestinationTag
	}

	return m, nil
}

// PaymentChannelFund adds more XRP to a payment channel.
type PaymentChannelFund struct {
	BaseTx

	// Channel is the channel ID (required)
	Channel string `json:"Channel"`

	// Amount is the amount of XRP to add (required)
	Amount Amount `json:"Amount"`

	// Expiration is the new expiration time (optional)
	Expiration *uint32 `json:"Expiration,omitempty"`
}

// NewPaymentChannelFund creates a new PaymentChannelFund transaction
func NewPaymentChannelFund(account, channel string, amount Amount) *PaymentChannelFund {
	return &PaymentChannelFund{
		BaseTx:  *NewBaseTx(TypePaymentChannelFund, account),
		Channel: channel,
		Amount:  amount,
	}
}

// TxType returns the transaction type
func (p *PaymentChannelFund) TxType() Type {
	return TypePaymentChannelFund
}

// Validate validates the PaymentChannelFund transaction
func (p *PaymentChannelFund) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	if p.Channel == "" {
		return errors.New("Channel is required")
	}

	if p.Amount.Value == "" {
		return errors.New("Amount is required")
	}

	if !p.Amount.IsNative() {
		return errors.New("payment channels can only hold XRP")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (p *PaymentChannelFund) Flatten() (map[string]any, error) {
	m := p.Common.ToMap()

	m["Channel"] = p.Channel
	m["Amount"] = p.Amount.Value

	if p.Expiration != nil {
		m["Expiration"] = *p.Expiration
	}

	return m, nil
}

// PaymentChannelClaim claims XRP from a payment channel.
type PaymentChannelClaim struct {
	BaseTx

	// Channel is the channel ID (required)
	Channel string `json:"Channel"`

	// Balance is the total amount delivered by this channel (optional)
	Balance *Amount `json:"Balance,omitempty"`

	// Amount is the amount of XRP authorized by the signature (optional)
	Amount *Amount `json:"Amount,omitempty"`

	// Signature is the signature for this claim (optional)
	Signature string `json:"Signature,omitempty"`

	// PublicKey is the public key for verifying the signature (optional)
	PublicKey string `json:"PublicKey,omitempty"`
}

// PaymentChannelClaim flags
const (
	// tfRenew resets the settle delay
	PaymentChannelClaimFlagRenew uint32 = 0x00010000
	// tfClose requests to close the channel
	PaymentChannelClaimFlagClose uint32 = 0x00020000
)

// NewPaymentChannelClaim creates a new PaymentChannelClaim transaction
func NewPaymentChannelClaim(account, channel string) *PaymentChannelClaim {
	return &PaymentChannelClaim{
		BaseTx:  *NewBaseTx(TypePaymentChannelClaim, account),
		Channel: channel,
	}
}

// TxType returns the transaction type
func (p *PaymentChannelClaim) TxType() Type {
	return TypePaymentChannelClaim
}

// Validate validates the PaymentChannelClaim transaction
func (p *PaymentChannelClaim) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	if p.Channel == "" {
		return errors.New("Channel is required")
	}

	// If Signature is provided, PublicKey and Amount must also be provided
	if p.Signature != "" {
		if p.PublicKey == "" {
			return errors.New("PublicKey is required with Signature")
		}
		if p.Amount == nil {
			return errors.New("Amount is required with Signature")
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (p *PaymentChannelClaim) Flatten() (map[string]any, error) {
	m := p.Common.ToMap()

	m["Channel"] = p.Channel

	if p.Balance != nil {
		m["Balance"] = p.Balance.Value
	}
	if p.Amount != nil {
		m["Amount"] = p.Amount.Value
	}
	if p.Signature != "" {
		m["Signature"] = p.Signature
	}
	if p.PublicKey != "" {
		m["PublicKey"] = p.PublicKey
	}

	return m, nil
}

// SetClose sets the close flag
func (p *PaymentChannelClaim) SetClose() {
	flags := p.GetFlags() | PaymentChannelClaimFlagClose
	p.SetFlags(flags)
}

// SetRenew sets the renew flag
func (p *PaymentChannelClaim) SetRenew() {
	flags := p.GetFlags() | PaymentChannelClaimFlagRenew
	p.SetFlags(flags)
}
