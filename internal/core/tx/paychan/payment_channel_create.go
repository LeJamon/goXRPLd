package paychan

import (
	"encoding/hex"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypePaymentChannelCreate, func() tx.Transaction {
		return &PaymentChannelCreate{BaseTx: *tx.NewBaseTx(tx.TypePaymentChannelCreate, "")}
	})
}

// PaymentChannelCreate creates a payment channel.
// Reference: rippled PayChan.cpp PayChanCreate
type PaymentChannelCreate struct {
	tx.BaseTx

	// Amount is the amount of XRP to lock in the channel (required)
	Amount tx.Amount `json:"Amount" xrpl:"Amount,amount"`

	// Destination is the account to receive channel payments (required)
	Destination string `json:"Destination" xrpl:"Destination"`

	// SettleDelay is the time in seconds to wait after close (required)
	SettleDelay uint32 `json:"SettleDelay" xrpl:"SettleDelay"`

	// PublicKey is the public key for verifying claims (required)
	PublicKey string `json:"PublicKey" xrpl:"PublicKey"`

	// CancelAfter is the time when the channel expires (optional)
	CancelAfter *uint32 `json:"CancelAfter,omitempty" xrpl:"CancelAfter,omitempty"`

	// DestinationTag is an arbitrary tag for the destination (optional)
	DestinationTag *uint32 `json:"DestinationTag,omitempty" xrpl:"DestinationTag,omitempty"`

	// SourceTag is an optional tag for the source (optional)
	SourceTag *uint32 `json:"SourceTag,omitempty" xrpl:"SourceTag,omitempty"`
}

// NewPaymentChannelCreate creates a new PaymentChannelCreate transaction
func NewPaymentChannelCreate(account, destination string, amount tx.Amount, settleDelay uint32, publicKey string) *PaymentChannelCreate {
	return &PaymentChannelCreate{
		BaseTx:      *tx.NewBaseTx(tx.TypePaymentChannelCreate, account),
		Amount:      amount,
		Destination: destination,
		SettleDelay: settleDelay,
		PublicKey:   publicKey,
	}
}

// TxType returns the transaction type
func (p *PaymentChannelCreate) TxType() tx.Type {
	return tx.TypePaymentChannelCreate
}

// Validate validates the PaymentChannelCreate transaction
// Reference: rippled PayChan.cpp PayChanCreate::preflight()
func (p *PaymentChannelCreate) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask) - fix1543
	if p.Common.Flags != nil && *p.Common.Flags&tx.TfUniversal != 0 {
		return tx.ErrInvalidFlags
	}

	// Destination is required
	if p.Destination == "" {
		return ErrPayChanDestRequired
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

	// Cannot create channel to self
	if p.Account == p.Destination {
		return ErrPayChanDestIsSrc
	}

	// PublicKey is required and must be valid
	if p.PublicKey == "" {
		return ErrPayChanPublicKeyRequired
	}

	// Validate PublicKey is valid hex and proper length
	pkBytes, err := hex.DecodeString(p.PublicKey)
	if err != nil {
		return ErrPayChanPublicKeyInvalid
	}
	if len(pkBytes) != 33 && len(pkBytes) != 65 {
		return ErrPayChanPublicKeyInvalid
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (p *PaymentChannelCreate) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(p)
}

// RequiredAmendments returns the amendments required for this transaction type
func (p *PaymentChannelCreate) RequiredAmendments() [][32]byte {
	return [][32]byte{amendment.FeaturePayChan}
}

// Apply applies a PaymentChannelCreate transaction
func (pc *PaymentChannelCreate) Apply(ctx *tx.ApplyContext) tx.Result {
	// Parse the amount
	amount := uint64(pc.Amount.Drops())

	// Check balance
	if ctx.Account.Balance < amount {
		return tx.TecUNFUNDED
	}

	// Verify destination exists
	destID, err := sle.DecodeAccountID(pc.Destination)
	if err != nil {
		return tx.TemINVALID
	}

	destKey := keylet.Account(destID)
	exists, _ := ctx.View.Exists(destKey)
	if !exists {
		return tx.TecNO_DST
	}

	// Deduct amount from account
	ctx.Account.Balance -= amount

	// Create pay channel
	accountID, _ := sle.DecodeAccountID(pc.Account)
	sequence := pc.GetCommon().SeqProxy()

	channelKey := keylet.PayChannel(accountID, destID, sequence)

	// Serialize pay channel
	channelData, err := serializePayChannel(pc, accountID, destID, amount)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Insert channel - creation tracked automatically by ApplyStateTable
	if err := ctx.View.Insert(channelKey, channelData); err != nil {
		return tx.TefINTERNAL
	}

	// Increase owner count
	ctx.Account.OwnerCount++

	return tx.TesSUCCESS
}
