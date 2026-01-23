package tx

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

func init() {
	Register(TypePaymentChannelCreate, func() Transaction {
		return &PaymentChannelCreate{BaseTx: *NewBaseTx(TypePaymentChannelCreate, "")}
	})
	Register(TypePaymentChannelFund, func() Transaction {
		return &PaymentChannelFund{BaseTx: *NewBaseTx(TypePaymentChannelFund, "")}
	})
	Register(TypePaymentChannelClaim, func() Transaction {
		return &PaymentChannelClaim{BaseTx: *NewBaseTx(TypePaymentChannelClaim, "")}
	})
}

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
	Amount Amount `json:"Amount" xrpl:"Amount,amount"`

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
	return ReflectFlatten(p)
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
	Channel string `json:"Channel" xrpl:"Channel"`

	// Amount is the amount of XRP to add (required)
	Amount Amount `json:"Amount" xrpl:"Amount,amount"`

	// Expiration is the new expiration time (optional)
	Expiration *uint32 `json:"Expiration,omitempty" xrpl:"Expiration,omitempty"`
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
	return ReflectFlatten(p)
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
	Channel string `json:"Channel" xrpl:"Channel"`

	// Balance is the total amount delivered by this channel (optional)
	Balance *Amount `json:"Balance,omitempty" xrpl:"Balance,omitempty,amount"`

	// Amount is the amount of XRP authorized by the signature (optional)
	Amount *Amount `json:"Amount,omitempty" xrpl:"Amount,omitempty,amount"`

	// Signature is the signature for this claim (optional)
	Signature string `json:"Signature,omitempty" xrpl:"Signature,omitempty"`

	// PublicKey is the public key for verifying the signature (optional)
	PublicKey string `json:"PublicKey,omitempty" xrpl:"PublicKey,omitempty"`
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
	return ReflectFlatten(p)
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

// Apply applies a PaymentChannelCreate transaction
func (pc *PaymentChannelCreate) Apply(ctx *ApplyContext) Result {
	// Parse the amount
	amount, err := strconv.ParseUint(pc.Amount.Value, 10, 64)
	if err != nil {
		return TemINVALID
	}

	// Check balance
	if ctx.Account.Balance < amount {
		return TecUNFUNDED
	}

	// Verify destination exists
	destID, err := decodeAccountID(pc.Destination)
	if err != nil {
		return TemINVALID
	}

	destKey := keylet.Account(destID)
	exists, _ := ctx.View.Exists(destKey)
	if !exists {
		return TecNO_DST
	}

	// Deduct amount from account
	ctx.Account.Balance -= amount

	// Create pay channel
	accountID, _ := decodeAccountID(pc.Account)
	sequence := *pc.GetCommon().Sequence

	channelKey := keylet.PayChannel(accountID, destID, sequence)

	// Serialize pay channel
	channelData, err := serializePayChannel(pc, accountID, destID, amount)
	if err != nil {
		return TefINTERNAL
	}

	// Insert channel - creation tracked automatically by ApplyStateTable
	if err := ctx.View.Insert(channelKey, channelData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	ctx.Account.OwnerCount++

	return TesSUCCESS
}

// Apply applies a PaymentChannelFund transaction
func (pf *PaymentChannelFund) Apply(ctx *ApplyContext) Result {
	// Parse channel ID
	channelID, err := hex.DecodeString(pf.Channel)
	if err != nil || len(channelID) != 32 {
		return TemINVALID
	}

	var channelKeyBytes [32]byte
	copy(channelKeyBytes[:], channelID)
	channelKey := keylet.Keylet{Key: channelKeyBytes}

	// Read channel
	channelData, err := ctx.View.Read(channelKey)
	if err != nil {
		return TecNO_TARGET
	}

	// Parse channel
	channel, err := parsePayChannel(channelData)
	if err != nil {
		return TefINTERNAL
	}

	// Verify sender is the channel owner
	accountID, _ := decodeAccountID(pf.Account)
	if channel.Account != accountID {
		return TecNO_PERMISSION
	}

	// Parse amount to add
	amount, err := strconv.ParseUint(pf.Amount.Value, 10, 64)
	if err != nil {
		return TemINVALID
	}

	// Check balance
	if ctx.Account.Balance < amount {
		return TecUNFUNDED
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
	updatedChannelData, err := serializePayChannelFromData(channel)
	if err != nil {
		return TefINTERNAL
	}

	if err := ctx.View.Update(channelKey, updatedChannelData); err != nil {
		return TefINTERNAL
	}

	return TesSUCCESS
}

// Apply applies a PaymentChannelClaim transaction
func (pcl *PaymentChannelClaim) Apply(ctx *ApplyContext) Result {
	// Parse channel ID
	channelID, err := hex.DecodeString(pcl.Channel)
	if err != nil || len(channelID) != 32 {
		return TemINVALID
	}

	var channelKeyBytes [32]byte
	copy(channelKeyBytes[:], channelID)
	channelKey := keylet.Keylet{Key: channelKeyBytes}

	// Read channel
	channelData, err := ctx.View.Read(channelKey)
	if err != nil {
		return TecNO_TARGET
	}

	// Parse channel
	channel, err := parsePayChannel(channelData)
	if err != nil {
		return TefINTERNAL
	}

	accountID, _ := decodeAccountID(pcl.Account)
	isOwner := channel.Account == accountID
	isDest := channel.DestinationID == accountID

	if !isOwner && !isDest {
		return TecNO_PERMISSION
	}

	// Handle claim with signature
	if pcl.Balance != nil && pcl.Amount != nil && pcl.Signature != "" {
		// Parse claimed balance
		claimBalance, err := strconv.ParseUint(pcl.Balance.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}

		// Verify claim is valid (would verify signature in full implementation)
		if claimBalance > channel.Amount {
			return TecUNFUNDED_PAYMENT
		}

		if claimBalance < channel.Balance {
			return TemINVALID // Can't decrease balance
		}

		// Calculate amount to transfer
		transferAmount := claimBalance - channel.Balance

		// Transfer to destination
		destKey := keylet.Account(channel.DestinationID)
		destData, err := ctx.View.Read(destKey)
		if err != nil {
			return TecNO_DST
		}

		destAccount, err := parseAccountRoot(destData)
		if err != nil {
			return TefINTERNAL
		}

		destAccount.Balance += transferAmount
		channel.Balance = claimBalance

		// Update destination - modification tracked automatically by ApplyStateTable
		destUpdatedData, err := serializeAccountRoot(destAccount)
		if err != nil {
			return TefINTERNAL
		}

		if err := ctx.View.Update(destKey, destUpdatedData); err != nil {
			return TefINTERNAL
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
				ownerAccount, err := parseAccountRoot(ownerData)
				if err == nil {
					ownerAccount.Balance += remaining
					if ownerAccount.OwnerCount > 0 {
						ownerAccount.OwnerCount--
					}
					ownerUpdatedData, _ := serializeAccountRoot(ownerAccount)
					ctx.View.Update(ownerKey, ownerUpdatedData)
				}
			}
		}

		// Delete channel - deletion tracked automatically by ApplyStateTable
		if err := ctx.View.Erase(channelKey); err != nil {
			return TefINTERNAL
		}
	} else {
		// Update channel - modification tracked automatically by ApplyStateTable
		updatedChannelData, err := serializePayChannelFromData(channel)
		if err != nil {
			return TefINTERNAL
		}

		if err := ctx.View.Update(channelKey, updatedChannelData); err != nil {
			return TefINTERNAL
		}
	}

	return TesSUCCESS
}

// PayChannelData represents a PayChannel ledger entry
type PayChannelData struct {
	Account        [20]byte
	DestinationID  [20]byte
	Amount         uint64
	Balance        uint64
	SettleDelay    uint32
	PublicKey      string
	Expiration     uint32
	CancelAfter    uint32
	SourceTag      uint32
	DestinationTag uint32
	HasSourceTag   bool
	HasDestTag     bool
}

// serializePayChannel serializes a PayChannel ledger entry from a transaction
func serializePayChannel(tx *PaymentChannelCreate, ownerID, destID [20]byte, amount uint64) ([]byte, error) {
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
		"SettleDelay":     tx.SettleDelay,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	if tx.CancelAfter != nil {
		jsonObj["CancelAfter"] = *tx.CancelAfter
	}

	if tx.PublicKey != "" {
		jsonObj["PublicKey"] = tx.PublicKey
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode PayChannel: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// serializePayChannelFromData serializes a PayChannel ledger entry from data
func serializePayChannelFromData(channel *PayChannelData) ([]byte, error) {
	ownerAddress, err := addresscodec.EncodeAccountIDToClassicAddress(channel.Account[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode owner address: %w", err)
	}

	destAddress, err := addresscodec.EncodeAccountIDToClassicAddress(channel.DestinationID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode destination address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "PayChannel",
		"Account":         ownerAddress,
		"Destination":     destAddress,
		"Amount":          fmt.Sprintf("%d", channel.Amount),
		"Balance":         fmt.Sprintf("%d", channel.Balance),
		"SettleDelay":     channel.SettleDelay,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode PayChannel: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// parsePayChannel parses a PayChannel ledger entry from binary data (internal use)
func parsePayChannel(data []byte) (*PayChannelData, error) {
	return ParsePayChannelFromBytes(data)
}

// ParsePayChannelFromBytes parses a PayChannel ledger entry from binary data
func ParsePayChannelFromBytes(data []byte) (*PayChannelData, error) {
	channel := &PayChannelData{}
	offset := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		header := data[offset]
		offset++

		typeCode := (header >> 4) & 0x0F
		fieldCode := header & 0x0F

		if typeCode == 0 {
			if offset >= len(data) {
				break
			}
			typeCode = data[offset]
			offset++
		}

		if fieldCode == 0 {
			if offset >= len(data) {
				break
			}
			fieldCode = data[offset]
			offset++
		}

		switch typeCode {
		case fieldTypeUInt16:
			if offset+2 > len(data) {
				return channel, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return channel, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case 39: // SettleDelay
				channel.SettleDelay = value
			case 37: // CancelAfter
				channel.CancelAfter = value
			case 10: // Expiration
				channel.Expiration = value
			case 3: // SourceTag
				channel.SourceTag = value
				channel.HasSourceTag = true
			case 14: // DestinationTag
				channel.DestinationTag = value
				channel.HasDestTag = true
			}

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return channel, nil
			}
			offset += 8

		case fieldTypeAmount:
			if offset+8 > len(data) {
				return channel, nil
			}
			rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
			amount := rawAmount & 0x3FFFFFFFFFFFFFFF
			if fieldCode == 1 { // Amount
				channel.Amount = amount
			} else if fieldCode == 5 { // Balance
				channel.Balance = amount
			}
			offset += 8

		case fieldTypeAccountID:
			if offset+21 > len(data) {
				return channel, nil
			}
			length := data[offset]
			offset++
			if length == 20 {
				switch fieldCode {
				case 1: // Account
					copy(channel.Account[:], data[offset:offset+20])
				case 3: // Destination
					copy(channel.DestinationID[:], data[offset:offset+20])
				}
				offset += 20
			}

		case fieldTypeHash256:
			// Hash256 fields are 32 bytes (e.g., PreviousTxnID)
			if offset+32 > len(data) {
				return channel, nil
			}
			offset += 32

		case fieldTypeBlob:
			if offset >= len(data) {
				return channel, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return channel, nil
			}
			if fieldCode == 28 { // PublicKey
				channel.PublicKey = hex.EncodeToString(data[offset : offset+length])
			}
			offset += length

		default:
			return channel, nil
		}
	}

	return channel, nil
}
