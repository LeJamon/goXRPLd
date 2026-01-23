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
	Register(TypeCheckCreate, func() Transaction {
		return &CheckCreate{BaseTx: *NewBaseTx(TypeCheckCreate, "")}
	})
	Register(TypeCheckCash, func() Transaction {
		return &CheckCash{BaseTx: *NewBaseTx(TypeCheckCash, "")}
	})
	Register(TypeCheckCancel, func() Transaction {
		return &CheckCancel{BaseTx: *NewBaseTx(TypeCheckCancel, "")}
	})
}

// CheckCreate creates a Check that can be cashed by the destination.
type CheckCreate struct {
	BaseTx

	// Destination is the account that can cash the check (required)
	Destination string `json:"Destination" xrpl:"Destination"`

	// SendMax is the maximum amount that can be debited from the sender (required)
	SendMax Amount `json:"SendMax" xrpl:"SendMax,amount"`

	// DestinationTag is an arbitrary tag for the destination (optional)
	DestinationTag *uint32 `json:"DestinationTag,omitempty" xrpl:"DestinationTag,omitempty"`

	// Expiration is the time when the check expires (optional)
	Expiration *uint32 `json:"Expiration,omitempty" xrpl:"Expiration,omitempty"`

	// InvoiceID is a 256-bit hash for identifying this check (optional)
	InvoiceID string `json:"InvoiceID,omitempty" xrpl:"InvoiceID,omitempty"`
}

// NewCheckCreate creates a new CheckCreate transaction
func NewCheckCreate(account, destination string, sendMax Amount) *CheckCreate {
	return &CheckCreate{
		BaseTx:      *NewBaseTx(TypeCheckCreate, account),
		Destination: destination,
		SendMax:     sendMax,
	}
}

// TxType returns the transaction type
func (c *CheckCreate) TxType() Type {
	return TypeCheckCreate
}

// Validate validates the CheckCreate transaction
func (c *CheckCreate) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	if c.Destination == "" {
		return errors.New("Destination is required")
	}

	if c.SendMax.Value == "" {
		return errors.New("SendMax is required")
	}

	// Cannot create check to self
	if c.Account == c.Destination {
		return errors.New("cannot create check to self")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (c *CheckCreate) Flatten() (map[string]any, error) {
	return ReflectFlatten(c)
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CheckCreate) RequiredAmendments() []string {
	return []string{AmendmentChecks}
}

// CheckCash cashes a Check, drawing from the sender's balance.
type CheckCash struct {
	BaseTx

	// CheckID is the ID of the check to cash (required)
	CheckID string `json:"CheckID" xrpl:"CheckID"`

	// Amount is the exact amount to receive (optional, mutually exclusive with DeliverMin)
	Amount *Amount `json:"Amount,omitempty" xrpl:"Amount,omitempty,amount"`

	// DeliverMin is the minimum amount to receive (optional, mutually exclusive with Amount)
	DeliverMin *Amount `json:"DeliverMin,omitempty" xrpl:"DeliverMin,omitempty,amount"`
}

// NewCheckCash creates a new CheckCash transaction
func NewCheckCash(account, checkID string) *CheckCash {
	return &CheckCash{
		BaseTx:  *NewBaseTx(TypeCheckCash, account),
		CheckID: checkID,
	}
}

// TxType returns the transaction type
func (c *CheckCash) TxType() Type {
	return TypeCheckCash
}

// Validate validates the CheckCash transaction
func (c *CheckCash) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	if c.CheckID == "" {
		return errors.New("CheckID is required")
	}

	// Must have exactly one of Amount or DeliverMin
	hasAmount := c.Amount != nil
	hasDeliverMin := c.DeliverMin != nil

	if !hasAmount && !hasDeliverMin {
		return errors.New("must specify Amount or DeliverMin")
	}

	if hasAmount && hasDeliverMin {
		return errors.New("cannot specify both Amount and DeliverMin")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (c *CheckCash) Flatten() (map[string]any, error) {
	return ReflectFlatten(c)
}

// SetExactAmount sets the exact amount to receive
func (c *CheckCash) SetExactAmount(amount Amount) {
	c.Amount = &amount
	c.DeliverMin = nil
}

// SetDeliverMin sets the minimum amount to receive
func (c *CheckCash) SetDeliverMin(amount Amount) {
	c.DeliverMin = &amount
	c.Amount = nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CheckCash) RequiredAmendments() []string {
	return []string{AmendmentChecks}
}

// CheckCancel cancels a Check.
type CheckCancel struct {
	BaseTx

	// CheckID is the ID of the check to cancel (required)
	CheckID string `json:"CheckID" xrpl:"CheckID"`
}

// NewCheckCancel creates a new CheckCancel transaction
func NewCheckCancel(account, checkID string) *CheckCancel {
	return &CheckCancel{
		BaseTx:  *NewBaseTx(TypeCheckCancel, account),
		CheckID: checkID,
	}
}

// TxType returns the transaction type
func (c *CheckCancel) TxType() Type {
	return TypeCheckCancel
}

// Validate validates the CheckCancel transaction
func (c *CheckCancel) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	if c.CheckID == "" {
		return errors.New("CheckID is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (c *CheckCancel) Flatten() (map[string]any, error) {
	return ReflectFlatten(c)
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CheckCancel) RequiredAmendments() []string {
	return []string{AmendmentChecks}
}

// Apply applies the CheckCreate transaction to ledger state.
func (c *CheckCreate) Apply(ctx *ApplyContext) Result {
	// Verify destination exists
	destID, err := decodeAccountID(c.Destination)
	if err != nil {
		return TemINVALID
	}

	destKey := keylet.Account(destID)
	exists, _ := ctx.View.Exists(destKey)
	if !exists {
		return TecNO_DST
	}

	// Parse SendMax - only XRP supported for now
	sendMax, err := strconv.ParseUint(c.SendMax.Value, 10, 64)
	if err != nil {
		// May be an IOU amount
		sendMax = 0
	}

	// Check balance for XRP checks
	if c.SendMax.Currency == "" && sendMax > 0 {
		if ctx.Account.Balance < sendMax {
			return TecUNFUNDED
		}
	}

	// Create the check entry
	accountID, _ := decodeAccountID(c.Account)
	sequence := *c.GetCommon().Sequence

	checkKey := keylet.Check(accountID, sequence)

	// Serialize check
	checkData, err := serializeCheck(c, accountID, destID, sequence, sendMax)
	if err != nil {
		return TefINTERNAL
	}

	// Insert check - creation tracked automatically by ApplyStateTable
	if err := ctx.View.Insert(checkKey, checkData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	ctx.Account.OwnerCount++

	return TesSUCCESS
}

// Apply applies the CheckCash transaction to ledger state.
func (c *CheckCash) Apply(ctx *ApplyContext) Result {
	// Parse check ID
	checkID, err := hex.DecodeString(c.CheckID)
	if err != nil || len(checkID) != 32 {
		return TemINVALID
	}

	var checkKeyBytes [32]byte
	copy(checkKeyBytes[:], checkID)
	checkKey := keylet.Keylet{Key: checkKeyBytes}

	// Read check
	checkData, err := ctx.View.Read(checkKey)
	if err != nil {
		return TecNO_ENTRY
	}

	// Parse check
	check, err := parseCheck(checkData)
	if err != nil {
		return TefINTERNAL
	}

	// Verify the account is the destination
	accountID, _ := decodeAccountID(c.Account)
	if check.DestinationID != accountID {
		return TecNO_PERMISSION
	}

	// Check expiration
	if check.Expiration > 0 {
		// In full implementation, check against close time
		// For standalone mode, we'll allow it
	}

	// Determine amount to cash
	var cashAmount uint64
	if c.Amount != nil {
		// Exact amount
		cashAmount, err = strconv.ParseUint(c.Amount.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}
		if cashAmount > check.SendMax {
			return TecPATH_PARTIAL
		}
	} else if c.DeliverMin != nil {
		// Minimum amount - use full SendMax for simplicity
		deliverMin, err := strconv.ParseUint(c.DeliverMin.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}
		if check.SendMax < deliverMin {
			return TecPATH_PARTIAL
		}
		cashAmount = check.SendMax
	}

	// Get the check creator's account
	creatorKey := keylet.Account(check.Account)
	creatorData, err := ctx.View.Read(creatorKey)
	if err != nil {
		return TefINTERNAL
	}

	creatorAccount, err := parseAccountRoot(creatorData)
	if err != nil {
		return TefINTERNAL
	}

	// Check if creator has sufficient balance
	if creatorAccount.Balance < cashAmount {
		return TecUNFUNDED_PAYMENT
	}

	// Transfer the funds
	creatorAccount.Balance -= cashAmount
	ctx.Account.Balance += cashAmount

	// Decrease creator's owner count
	if creatorAccount.OwnerCount > 0 {
		creatorAccount.OwnerCount--
	}

	// Update creator account - modification tracked automatically by ApplyStateTable
	creatorUpdatedData, err := serializeAccountRoot(creatorAccount)
	if err != nil {
		return TefINTERNAL
	}

	if err := ctx.View.Update(creatorKey, creatorUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Delete the check - deletion tracked automatically by ApplyStateTable
	if err := ctx.View.Erase(checkKey); err != nil {
		return TefINTERNAL
	}

	return TesSUCCESS
}

// Apply applies the CheckCancel transaction to ledger state.
func (c *CheckCancel) Apply(ctx *ApplyContext) Result {
	// Parse check ID
	checkID, err := hex.DecodeString(c.CheckID)
	if err != nil || len(checkID) != 32 {
		return TemINVALID
	}

	var checkKeyBytes [32]byte
	copy(checkKeyBytes[:], checkID)
	checkKey := keylet.Keylet{Key: checkKeyBytes}

	// Read check
	checkData, err := ctx.View.Read(checkKey)
	if err != nil {
		return TecNO_ENTRY
	}

	// Parse check
	check, err := parseCheck(checkData)
	if err != nil {
		return TefINTERNAL
	}

	accountID, _ := decodeAccountID(c.Account)
	isCreator := check.Account == accountID
	isDestination := check.DestinationID == accountID

	// Only creator or destination can cancel
	if !isCreator && !isDestination {
		// Unless the check is expired
		if check.Expiration == 0 {
			return TecNO_PERMISSION
		}
		// In full implementation, check if expired
		// For standalone mode, allow anyone to cancel expired checks
	}

	// Delete the check - deletion tracked automatically by ApplyStateTable
	if err := ctx.View.Erase(checkKey); err != nil {
		return TefINTERNAL
	}

	// If the canceller is also the creator, decrease their owner count
	if isCreator {
		if ctx.Account.OwnerCount > 0 {
			ctx.Account.OwnerCount--
		}
	} else {
		// Need to update the creator's owner count
		creatorKey := keylet.Account(check.Account)
		creatorData, err := ctx.View.Read(creatorKey)
		if err == nil {
			creatorAccount, err := parseAccountRoot(creatorData)
			if err == nil && creatorAccount.OwnerCount > 0 {
				creatorAccount.OwnerCount--
				creatorUpdatedData, _ := serializeAccountRoot(creatorAccount)
				ctx.View.Update(creatorKey, creatorUpdatedData)
			}
		}
	}

	return TesSUCCESS
}

// CheckData represents a Check ledger entry
type CheckData struct {
	Account        [20]byte
	DestinationID  [20]byte
	SendMax        uint64 // For XRP checks; IOU checks would need more fields
	Sequence       uint32
	Expiration     uint32
	InvoiceID      [32]byte
	DestinationTag uint32
	HasDestTag     bool
}

// serializeCheck serializes a Check ledger entry
func serializeCheck(tx *CheckCreate, ownerID, destID [20]byte, sequence uint32, sendMax uint64) ([]byte, error) {
	ownerAddress, err := addresscodec.EncodeAccountIDToClassicAddress(ownerID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode owner address: %w", err)
	}

	destAddress, err := addresscodec.EncodeAccountIDToClassicAddress(destID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode destination address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "Check",
		"Account":         ownerAddress,
		"Destination":     destAddress,
		"SendMax":         fmt.Sprintf("%d", sendMax),
		"Sequence":        sequence,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	if tx.Expiration != nil {
		jsonObj["Expiration"] = *tx.Expiration
	}

	if tx.DestinationTag != nil {
		jsonObj["DestinationTag"] = *tx.DestinationTag
	}

	if tx.InvoiceID != "" {
		jsonObj["InvoiceID"] = tx.InvoiceID
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode Check: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// parseCheck parses a Check ledger entry from binary data
func parseCheck(data []byte) (*CheckData, error) {
	check := &CheckData{}
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
				return check, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return check, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case fieldCodeSequence:
				check.Sequence = value
			case 10: // Expiration
				check.Expiration = value
			case 14: // DestinationTag
				check.DestinationTag = value
				check.HasDestTag = true
			}

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return check, nil
			}
			offset += 8

		case fieldTypeHash256:
			if offset+32 > len(data) {
				return check, nil
			}
			if fieldCode == 17 { // InvoiceID
				copy(check.InvoiceID[:], data[offset:offset+32])
			}
			offset += 32

		case fieldTypeAmount:
			if offset+8 > len(data) {
				return check, nil
			}
			if data[offset]&0x80 == 0 {
				// XRP amount
				rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
				if fieldCode == 9 { // SendMax
					check.SendMax = rawAmount & 0x3FFFFFFFFFFFFFFF
				}
				offset += 8
			} else {
				// IOU amount - skip 48 bytes
				offset += 48
			}

		case fieldTypeAccountID:
			if offset+21 > len(data) {
				return check, nil
			}
			length := data[offset]
			offset++
			if length == 20 {
				switch fieldCode {
				case 1: // Account
					copy(check.Account[:], data[offset:offset+20])
				case 3: // Destination
					copy(check.DestinationID[:], data[offset:offset+20])
				}
				offset += 20
			}

		case fieldTypeBlob:
			if offset >= len(data) {
				return check, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return check, nil
			}
			offset += length

		default:
			return check, nil
		}
	}

	return check, nil
}
