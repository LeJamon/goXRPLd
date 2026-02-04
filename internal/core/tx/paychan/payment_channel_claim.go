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
	tx.Register(tx.TypePaymentChannelClaim, func() tx.Transaction {
		return &PaymentChannelClaim{BaseTx: *tx.NewBaseTx(tx.TypePaymentChannelClaim, "")}
	})
}

// PaymentChannelClaim claims XRP from a payment channel.
// Reference: rippled PayChan.cpp PayChanClaim
type PaymentChannelClaim struct {
	tx.BaseTx

	// Channel is the channel ID (required)
	Channel string `json:"Channel" xrpl:"Channel"`

	// Balance is the total amount delivered by this channel (optional)
	Balance *tx.Amount `json:"Balance,omitempty" xrpl:"Balance,omitempty,amount"`

	// Amount is the amount of XRP authorized by the signature (optional)
	Amount *tx.Amount `json:"Amount,omitempty" xrpl:"Amount,omitempty,amount"`

	// Signature is the signature for this claim (optional)
	Signature string `json:"Signature,omitempty" xrpl:"Signature,omitempty"`

	// PublicKey is the public key for verifying the signature (optional)
	PublicKey string `json:"PublicKey,omitempty" xrpl:"PublicKey,omitempty"`
}

// NewPaymentChannelClaim creates a new PaymentChannelClaim transaction
func NewPaymentChannelClaim(account, channel string) *PaymentChannelClaim {
	return &PaymentChannelClaim{
		BaseTx:  *tx.NewBaseTx(tx.TypePaymentChannelClaim, account),
		Channel: channel,
	}
}

// TxType returns the transaction type
func (p *PaymentChannelClaim) TxType() tx.Type {
	return tx.TypePaymentChannelClaim
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
	flags := p.GetFlags()
	validFlags := tfPayChanRenew | tfPayChanClose | tx.TfUniversal
	if flags & ^validFlags != 0 {
		return tx.ErrInvalidFlags
	}

	// Cannot set both tfClose and tfRenew
	if (flags&tfPayChanClose != 0) && (flags&tfPayChanRenew != 0) {
		return ErrPayChanCloseAndRenew
	}

	// Validate Balance if present
	if p.Balance != nil {
		if !p.Balance.IsNative() {
			return errors.New("temBAD_AMOUNT: Balance must be XRP")
		}
		balVal := p.Balance.Drops()
		if balVal <= 0 {
			return errors.New("temBAD_AMOUNT: Balance must be positive")
		}
	}

	// Validate Amount if present
	if p.Amount != nil {
		if !p.Amount.IsNative() {
			return errors.New("temBAD_AMOUNT: Amount must be XRP")
		}
		amtVal := p.Amount.Drops()
		if amtVal <= 0 {
			return errors.New("temBAD_AMOUNT: Amount must be positive")
		}
	}

	// Balance cannot exceed Amount
	if p.Balance != nil && p.Amount != nil {
		balVal := p.Balance.Drops()
		amtVal := p.Amount.Drops()
		if balVal > amtVal {
			return ErrPayChanBalanceGTAmount
		}
	}

	// If Signature is provided, PublicKey and Balance must also be provided
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
	return tx.ReflectFlatten(p)
}

// RequiredAmendments returns the amendments required for this transaction type
func (p *PaymentChannelClaim) RequiredAmendments() []string {
	return []string{amendment.AmendmentPayChan}
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

// Apply applies a PaymentChannelClaim transaction
func (pcl *PaymentChannelClaim) Apply(ctx *tx.ApplyContext) tx.Result {
	// Parse channel ID
	channelID, err := hex.DecodeString(pcl.Channel)
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

	accountID, _ := sle.DecodeAccountID(pcl.Account)
	isOwner := channel.Account == accountID
	isDest := channel.DestinationID == accountID

	if !isOwner && !isDest {
		return tx.TecNO_PERMISSION
	}

	// Handle claim with signature
	if pcl.Balance != nil && pcl.Amount != nil && pcl.Signature != "" {
		// Parse claimed balance
		claimBalance := uint64(pcl.Balance.Drops())

		// Verify claim is valid (would verify signature in full implementation)
		if claimBalance > channel.Amount {
			return tx.TecUNFUNDED_PAYMENT
		}

		if claimBalance < channel.Balance {
			return tx.TemINVALID // Can't decrease balance
		}

		// Calculate amount to transfer
		transferAmount := claimBalance - channel.Balance

		// Transfer to destination
		destKey := keylet.Account(channel.DestinationID)
		destData, err := ctx.View.Read(destKey)
		if err != nil {
			return tx.TecNO_DST
		}

		destAccount, err := sle.ParseAccountRoot(destData)
		if err != nil {
			return tx.TefINTERNAL
		}

		destAccount.Balance += transferAmount
		channel.Balance = claimBalance

		// Update destination - modification tracked automatically by ApplyStateTable
		destUpdatedData, err := sle.SerializeAccountRoot(destAccount)
		if err != nil {
			return tx.TefINTERNAL
		}

		if err := ctx.View.Update(destKey, destUpdatedData); err != nil {
			return tx.TefINTERNAL
		}
	}

	// Handle close flag
	flags := pcl.GetFlags()
	if flags&PaymentChannelClaimFlagClose != 0 {
		// Close the channel

		// Return remaining funds to owner
		remaining := channel.Amount - channel.Balance
		if remaining > 0 {
			ownerKey := keylet.Account(channel.Account)
			ownerData, err := ctx.View.Read(ownerKey)
			if err == nil {
				ownerAccount, err := sle.ParseAccountRoot(ownerData)
				if err == nil {
					ownerAccount.Balance += remaining
					if ownerAccount.OwnerCount > 0 {
						ownerAccount.OwnerCount--
					}
					ownerUpdatedData, _ := sle.SerializeAccountRoot(ownerAccount)
					ctx.View.Update(ownerKey, ownerUpdatedData)
				}
			}
		}

		// Delete channel - deletion tracked automatically by ApplyStateTable
		if err := ctx.View.Erase(channelKey); err != nil {
			return tx.TefINTERNAL
		}
	} else {
		// Update channel - modification tracked automatically by ApplyStateTable
		updatedChannelData, err := sle.SerializePayChannelFromData(channel)
		if err != nil {
			return tx.TefINTERNAL
		}

		if err := ctx.View.Update(channelKey, updatedChannelData); err != nil {
			return tx.TefINTERNAL
		}
	}

	return tx.TesSUCCESS
}
