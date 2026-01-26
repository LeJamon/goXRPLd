// TODO missing sle method related to payment chanel
// TODO split this file
package paychan

import (
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
)

func init() {
	tx.Register(tx.TypePaymentChannelCreate, func() tx.Transaction {
		return &PaymentChannelCreate{BaseTx: *tx.NewBaseTx(tx.TypePaymentChannelCreate, "")}
	})
	tx.Register(tx.TypePaymentChannelFund, func() tx.Transaction {
		return &PaymentChannelFund{BaseTx: *tx.NewBaseTx(tx.TypePaymentChannelFund, "")}
	})
	tx.Register(tx.TypePaymentChannelClaim, func() tx.Transaction {
		return &PaymentChannelClaim{BaseTx: *tx.NewBaseTx(tx.TypePaymentChannelClaim, "")}
	})
}

// Payment channel constants
const (
	// MaxPayChanPublicKeyLength is the maximum length of a public key (33 bytes compressed)
	MaxPayChanPublicKeyLength = 66 // 33 bytes * 2 hex chars
)

// Payment channel flags
const (
	// tfPayChanRenew resets the settle delay
	tfPayChanRenew uint32 = 0x00010000
	// tfPayChanClose requests to close the channel
	tfPayChanClose uint32 = 0x00020000
)

// Exported flag constants for backwards compatibility
const (
	PaymentChannelClaimFlagRenew = tfPayChanRenew
	PaymentChannelClaimFlagClose = tfPayChanClose
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
func (p *PaymentChannelCreate) RequiredAmendments() []string {
	return []string{amendment.AmendmentPayChan}
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
	return tx.ReflectFlatten(p)
}

// RequiredAmendments returns the amendments required for this transaction type
func (p *PaymentChannelFund) RequiredAmendments() []string {
	return []string{amendment.AmendmentPayChan}
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
		balVal, err := strconv.ParseInt(p.Balance.Value, 10, 64)
		if err != nil || balVal <= 0 {
			return errors.New("temBAD_AMOUNT: Balance must be positive")
		}
	}

	// Validate Amount if present
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
	if p.Balance != nil && p.Amount != nil {
		balVal, _ := strconv.ParseInt(p.Balance.Value, 10, 64)
		amtVal, _ := strconv.ParseInt(p.Amount.Value, 10, 64)
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

// Apply applies a PaymentChannelCreate transaction
func (pc *PaymentChannelCreate) Apply(ctx *tx.ApplyContext) tx.Result {
	// Parse the amount
	amount, err := strconv.ParseUint(pc.Amount.Value, 10, 64)
	if err != nil {
		return tx.TemINVALID
	}

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
	sequence := *pc.GetCommon().Sequence

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
	amount, err := strconv.ParseUint(pf.Amount.Value, 10, 64)
	if err != nil {
		return tx.TemINVALID
	}

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
		claimBalance, err := strconv.ParseUint(pcl.Balance.Value, 10, 64)
		if err != nil {
			return tx.TemINVALID
		}

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

// serializePayChannel serializes a PayChannel ledger entry from a transaction
func serializePayChannel(pcTx *PaymentChannelCreate, ownerID, destID [20]byte, amount uint64) ([]byte, error) {
	ownerAddress, err := addresscodec.EncodeAccountIDToClassicAddress(ownerID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode owner address: %w", err)
	}

	destAddress, err := addresscodec.EncodeAccountIDToClassicAddress(destID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode destination address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "PayChannel",
		"Account":         ownerAddress,
		"Destination":     destAddress,
		"Amount":          fmt.Sprintf("%d", amount),
		"Balance":         "0",
		"SettleDelay":     pcTx.SettleDelay,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	if pcTx.CancelAfter != nil {
		jsonObj["CancelAfter"] = *pcTx.CancelAfter
	}

	if pcTx.PublicKey != "" {
		jsonObj["PublicKey"] = pcTx.PublicKey
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode PayChannel: %w", err)
	}

	return hex.DecodeString(hexStr)
}
