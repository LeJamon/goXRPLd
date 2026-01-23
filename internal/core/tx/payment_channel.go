package tx

import (
	"encoding/hex"
	"errors"
	"strconv"
)

// Payment channel constants
const (
	// MaxPayChanPublicKeyLength is the maximum length of a public key (33 bytes compressed)
	MaxPayChanPublicKeyLength = 66 // 33 bytes * 2 hex chars
)

// Payment channel errors
var (
	ErrPayChanAmountRequired    = errors.New("temBAD_AMOUNT: Amount is required")
	ErrPayChanAmountNotXRP      = errors.New("temBAD_AMOUNT: payment channels can only hold XRP")
	ErrPayChanAmountNotPositive = errors.New("temBAD_AMOUNT: Amount must be positive")
	ErrPayChanDestRequired      = errors.New("temDST_NEEDED: Destination is required")
	ErrPayChanDestIsSrc         = errors.New("temDST_IS_SRC: cannot create payment channel to self")
	ErrPayChanPublicKeyRequired = errors.New("temMALFORMED: PublicKey is required")
	ErrPayChanPublicKeyInvalid  = errors.New("temMALFORMED: PublicKey is not a valid public key")
	ErrPayChanChannelRequired   = errors.New("temMALFORMED: Channel is required")
	ErrPayChanBadExpiration     = errors.New("temBAD_EXPIRATION: Expiration is invalid")
	ErrPayChanBalanceGTAmount   = errors.New("temBAD_AMOUNT: Balance cannot exceed Amount")
	ErrPayChanCloseAndRenew     = errors.New("temMALFORMED: cannot set both tfClose and tfRenew")
	ErrPayChanSigNeedsKey       = errors.New("temMALFORMED: PublicKey is required with Signature")
	ErrPayChanSigNeedsBalance   = errors.New("temMALFORMED: Balance is required with Signature")
	ErrPayChanSigNeedsAmount    = errors.New("temMALFORMED: Amount is required with Signature")
)

// PaymentChannelCreate creates a payment channel.
// Reference: rippled PayChan.cpp PayChanCreate
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

	// SourceTag is an optional tag for the source (optional)
	SourceTag *uint32 `json:"SourceTag,omitempty"`
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
// Reference: rippled PayChan.cpp PayChanCreate::preflight()
func (p *PaymentChannelCreate) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask) - fix1543
	// Reference: rippled PayChan.cpp:177-178
	if p.Common.Flags != nil && *p.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// Destination is required
	if p.Destination == "" {
		return ErrPayChanDestRequired
	}

	// Amount is required and must be XRP
	// Reference: rippled PayChan.cpp:183-184
	if p.Amount.Value == "" {
		return ErrPayChanAmountRequired
	}

	if !p.Amount.IsNative() {
		return ErrPayChanAmountNotXRP
	}

	// Amount must be positive
	amountVal, err := strconv.ParseInt(p.Amount.Value, 10, 64)
	if err != nil || amountVal <= 0 {
		return ErrPayChanAmountNotPositive
	}

	// Cannot create channel to self
	// Reference: rippled PayChan.cpp:186-187
	if p.Account == p.Destination {
		return ErrPayChanDestIsSrc
	}

	// PublicKey is required and must be valid
	// Reference: rippled PayChan.cpp:189-190
	if p.PublicKey == "" {
		return ErrPayChanPublicKeyRequired
	}

	// Validate PublicKey is valid hex and proper length
	pkBytes, err := hex.DecodeString(p.PublicKey)
	if err != nil {
		return ErrPayChanPublicKeyInvalid
	}
	// Valid public key lengths: 33 bytes (compressed) or 65 bytes (uncompressed)
	if len(pkBytes) != 33 && len(pkBytes) != 65 {
		return ErrPayChanPublicKeyInvalid
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
	if p.SourceTag != nil {
		m["SourceTag"] = *p.SourceTag
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (p *PaymentChannelCreate) RequiredAmendments() []string {
	return []string{AmendmentPayChan}
}

// PaymentChannelFund adds more XRP to a payment channel.
// Reference: rippled PayChan.cpp PayChanFund
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
// Reference: rippled PayChan.cpp PayChanFund::preflight()
func (p *PaymentChannelFund) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask) - fix1543
	// Reference: rippled PayChan.cpp:332-333
	if p.Common.Flags != nil && *p.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// Channel is required
	if p.Channel == "" {
		return ErrPayChanChannelRequired
	}

	// Validate Channel is valid hex (256-bit hash)
	channelBytes, err := hex.DecodeString(p.Channel)
	if err != nil || len(channelBytes) != 32 {
		return errors.New("temMALFORMED: Channel must be a valid 256-bit hash")
	}

	// Amount is required and must be XRP
	// Reference: rippled PayChan.cpp:338-339
	if p.Amount.Value == "" {
		return ErrPayChanAmountRequired
	}

	if !p.Amount.IsNative() {
		return ErrPayChanAmountNotXRP
	}

	// Amount must be positive
	amountVal, err := strconv.ParseInt(p.Amount.Value, 10, 64)
	if err != nil || amountVal <= 0 {
		return ErrPayChanAmountNotPositive
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

// RequiredAmendments returns the amendments required for this transaction type
func (p *PaymentChannelFund) RequiredAmendments() []string {
	return []string{AmendmentPayChan}
}

// PaymentChannelClaim claims XRP from a payment channel.
// Reference: rippled PayChan.cpp PayChanClaim
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
	// tfPayChanRenew resets the settle delay
	tfPayChanRenew uint32 = 0x00010000
	// tfPayChanClose requests to close the channel
	tfPayChanClose uint32 = 0x00020000
)

// Deprecated flag constants for backwards compatibility
const (
	PaymentChannelClaimFlagRenew = tfPayChanRenew
	PaymentChannelClaimFlagClose = tfPayChanClose
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
// Reference: rippled PayChan.cpp PayChanClaim::preflight()
func (p *PaymentChannelClaim) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	// Channel is required
	if p.Channel == "" {
		return ErrPayChanChannelRequired
	}

	// Validate Channel is valid hex (256-bit hash)
	channelBytes, err := hex.DecodeString(p.Channel)
	if err != nil || len(channelBytes) != 32 {
		return errors.New("temMALFORMED: Channel must be a valid 256-bit hash")
	}

	// Validate flags - fix1543
	// Reference: rippled PayChan.cpp:443-444
	// Only tfPayChanRenew, tfPayChanClose, and tfFullyCanonicalSig are valid
	flags := p.GetFlags()
	validFlags := tfPayChanRenew | tfPayChanClose | tfUniversal
	if flags & ^validFlags != 0 {
		return ErrInvalidFlags
	}

	// Cannot set both tfClose and tfRenew
	// Reference: rippled PayChan.cpp:446-447
	if (flags&tfPayChanClose != 0) && (flags&tfPayChanRenew != 0) {
		return ErrPayChanCloseAndRenew
	}

	// Validate Balance if present
	// Reference: rippled PayChan.cpp:429-431
	if p.Balance != nil {
		if !p.Balance.IsNative() {
			return errors.New("temBAD_AMOUNT: Balance must be XRP")
		}
		balVal, err := strconv.ParseInt(p.Balance.Value, 10, 64)
		if err != nil || balVal <= 0 {
			return errors.New("temBAD_AMOUNT: Balance must be positive")
		}
	}

	// Validate Amount if present
	// Reference: rippled PayChan.cpp:433-435
	if p.Amount != nil {
		if !p.Amount.IsNative() {
			return errors.New("temBAD_AMOUNT: Amount must be XRP")
		}
		amtVal, err := strconv.ParseInt(p.Amount.Value, 10, 64)
		if err != nil || amtVal <= 0 {
			return errors.New("temBAD_AMOUNT: Amount must be positive")
		}
	}

	// Balance cannot exceed Amount
	// Reference: rippled PayChan.cpp:437-438
	if p.Balance != nil && p.Amount != nil {
		balVal, _ := strconv.ParseInt(p.Balance.Value, 10, 64)
		amtVal, _ := strconv.ParseInt(p.Amount.Value, 10, 64)
		if balVal > amtVal {
			return ErrPayChanBalanceGTAmount
		}
	}

	// If Signature is provided, PublicKey and Balance must also be provided
	// Reference: rippled PayChan.cpp:450-453
	if p.Signature != "" {
		if p.PublicKey == "" {
			return ErrPayChanSigNeedsKey
		}
		if p.Balance == nil {
			return ErrPayChanSigNeedsBalance
		}

		// Validate PublicKey is valid hex
		pkBytes, err := hex.DecodeString(p.PublicKey)
		if err != nil {
			return ErrPayChanPublicKeyInvalid
		}
		if len(pkBytes) != 33 && len(pkBytes) != 65 {
			return ErrPayChanPublicKeyInvalid
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

// RequiredAmendments returns the amendments required for this transaction type
func (p *PaymentChannelClaim) RequiredAmendments() []string {
	return []string{AmendmentPayChan}
}

// SetClose sets the close flag
func (p *PaymentChannelClaim) SetClose() {
	flags := p.GetFlags() | tfPayChanClose
	p.SetFlags(flags)
}

// SetRenew sets the renew flag
func (p *PaymentChannelClaim) SetRenew() {
	flags := p.GetFlags() | tfPayChanRenew
	p.SetFlags(flags)
}

// IsClose returns true if the close flag is set
func (p *PaymentChannelClaim) IsClose() bool {
	return p.GetFlags()&tfPayChanClose != 0
}

// IsRenew returns true if the renew flag is set
func (p *PaymentChannelClaim) IsRenew() bool {
	return p.GetFlags()&tfPayChanRenew != 0
}
