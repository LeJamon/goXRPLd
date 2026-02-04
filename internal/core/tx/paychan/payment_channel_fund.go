// TODO missing sle method related to payment chanel
package paychan

import (
	"encoding/hex"
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypePaymentChannelFund, func() tx.Transaction {
		return &PaymentChannelFund{BaseTx: *tx.NewBaseTx(tx.TypePaymentChannelFund, "")}
	})
}

// PaymentChannelFund adds more XRP to a payment channel.
// Reference: rippled PayChan.cpp PayChanFund
type PaymentChannelFund struct {
	tx.BaseTx

	// Channel is the channel ID (required)
	Channel string `json:"Channel" xrpl:"Channel"`

	// Amount is the amount of XRP to add (required)
	Amount tx.Amount `json:"Amount" xrpl:"Amount,amount"`

	// Expiration is the new expiration time (optional)
	Expiration *uint32 `json:"Expiration,omitempty" xrpl:"Expiration,omitempty"`
}

// NewPaymentChannelFund creates a new PaymentChannelFund transaction
func NewPaymentChannelFund(account, channel string, amount tx.Amount) *PaymentChannelFund {
	return &PaymentChannelFund{
		BaseTx:  *tx.NewBaseTx(tx.TypePaymentChannelFund, account),
		Channel: channel,
		Amount:  amount,
	}
}

// TxType returns the transaction type
func (p *PaymentChannelFund) TxType() tx.Type {
	return tx.TypePaymentChannelFund
}

// Validate validates the PaymentChannelFund transaction
// Reference: rippled PayChan.cpp PayChanFund::preflight()
func (p *PaymentChannelFund) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask) - fix1543
	if p.Common.Flags != nil && *p.Common.Flags&tx.TfUniversal != 0 {
		return tx.ErrInvalidFlags
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
	if p.Amount.IsZero() {
		return ErrPayChanAmountRequired
	}

	if !p.Amount.IsNative() {
		return ErrPayChanAmountNotXRP
	}

	// Amount must be positive
	if p.Amount.Drops() <= 0 {
		return ErrPayChanAmountNotPositive
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (p *PaymentChannelFund) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(p)
}

// RequiredAmendments returns the amendments required for this transaction type
func (p *PaymentChannelFund) RequiredAmendments() []string {
	return []string{amendment.AmendmentPayChan}
}

// Apply applies a PaymentChannelFund transaction
func (pf *PaymentChannelFund) Apply(ctx *tx.ApplyContext) tx.Result {
	// Parse channel ID
	channelID, err := hex.DecodeString(pf.Channel)
	if err != nil || len(channelID) != 32 {
		return tx.TemINVALID
	}

	var channelKeyBytes [32]byte
	copy(channelKeyBytes[:], channelID)
	channelKey := keylet.Keylet{Key: channelKeyBytes}

	// Read channel
	channelData, err := ctx.View.Read(channelKey)
	if err != nil {
		return tx.TecNO_TARGET
	}

	// Parse channel
	channel, err := sle.ParsePayChannel(channelData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Verify sender is the channel owner
	accountID, _ := sle.DecodeAccountID(pf.Account)
	if channel.Account != accountID {
		return tx.TecNO_PERMISSION
	}

	// Parse amount to add
	amount := uint64(pf.Amount.Drops())

	// Check balance
	if ctx.Account.Balance < amount {
		return tx.TecUNFUNDED
	}

	// Deduct from account
	ctx.Account.Balance -= amount

	// Add to channel
	channel.Amount += amount

	// Update expiration if specified
	if pf.Expiration != nil {
		channel.Expiration = *pf.Expiration
	}

	// Serialize updated channel - modification tracked automatically by ApplyStateTable
	updatedChannelData, err := sle.SerializePayChannelFromData(channel)
	if err != nil {
		return tx.TefINTERNAL
	}

	if err := ctx.View.Update(channelKey, updatedChannelData); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}
