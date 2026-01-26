package check

import (
	"encoding/hex"
	"errors"
	"fmt"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeCheckCreate, func() tx.Transaction {
		return &CheckCreate{BaseTx: *tx.NewBaseTx(tx.TypeCheckCreate, "")}
	})
	tx.Register(tx.TypeCheckCash, func() tx.Transaction {
		return &CheckCash{BaseTx: *tx.NewBaseTx(tx.TypeCheckCash, "")}
	})
	tx.Register(tx.TypeCheckCancel, func() tx.Transaction {
		return &CheckCancel{BaseTx: *tx.NewBaseTx(tx.TypeCheckCancel, "")}
	})
}

// CheckCreate creates a Check that can be cashed by the destination.
type CheckCreate struct {
	tx.BaseTx

	// Destination is the account that can cash the check (required)
	Destination string `json:"Destination" xrpl:"Destination"`

	// SendMax is the maximum amount that can be debited from the sender (required)
	SendMax tx.Amount `json:"SendMax" xrpl:"SendMax,amount"`

	// DestinationTag is an arbitrary tag for the destination (optional)
	DestinationTag *uint32 `json:"DestinationTag,omitempty" xrpl:"DestinationTag,omitempty"`

	// Expiration is the time when the check expires (optional)
	Expiration *uint32 `json:"Expiration,omitempty" xrpl:"Expiration,omitempty"`

	// InvoiceID is a 256-bit hash for identifying this check (optional)
	InvoiceID string `json:"InvoiceID,omitempty" xrpl:"InvoiceID,omitempty"`
}

// NewCheckCreate creates a new CheckCreate transaction
func NewCheckCreate(account, destination string, sendMax tx.Amount) *CheckCreate {
	return &CheckCreate{
		BaseTx:      *tx.NewBaseTx(tx.TypeCheckCreate, account),
		Destination: destination,
		SendMax:     sendMax,
	}
}

// TxType returns the transaction type
func (c *CheckCreate) TxType() tx.Type {
	return tx.TypeCheckCreate
}

// Validate validates the CheckCreate transaction
func (c *CheckCreate) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	if c.Destination == "" {
		return errors.New("Destination is required")
	}

	if c.SendMax.IsZero() {
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
	return tx.ReflectFlatten(c)
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CheckCreate) RequiredAmendments() []string {
	return []string{amendment.AmendmentChecks}
}

// CheckCash cashes a Check, drawing from the sender's balance.
type CheckCash struct {
	tx.BaseTx

	// CheckID is the ID of the check to cash (required)
	CheckID string `json:"CheckID" xrpl:"CheckID"`

	// Amount is the exact amount to receive (optional, mutually exclusive with DeliverMin)
	Amount *tx.Amount `json:"Amount,omitempty" xrpl:"Amount,omitempty,amount"`

	// DeliverMin is the minimum amount to receive (optional, mutually exclusive with Amount)
	DeliverMin *tx.Amount `json:"DeliverMin,omitempty" xrpl:"DeliverMin,omitempty,amount"`
}

// NewCheckCash creates a new CheckCash transaction
func NewCheckCash(account, checkID string) *CheckCash {
	return &CheckCash{
		BaseTx:  *tx.NewBaseTx(tx.TypeCheckCash, account),
		CheckID: checkID,
	}
}

// TxType returns the transaction type
func (c *CheckCash) TxType() tx.Type {
	return tx.TypeCheckCash
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
	return tx.ReflectFlatten(c)
}

// SetExactAmount sets the exact amount to receive
func (c *CheckCash) SetExactAmount(amount tx.Amount) {
	c.Amount = &amount
	c.DeliverMin = nil
}

// SetDeliverMin sets the minimum amount to receive
func (c *CheckCash) SetDeliverMin(amount tx.Amount) {
	c.DeliverMin = &amount
	c.Amount = nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CheckCash) RequiredAmendments() []string {
	return []string{amendment.AmendmentChecks}
}

// CheckCancel cancels a Check.
type CheckCancel struct {
	tx.BaseTx

	// CheckID is the ID of the check to cancel (required)
	CheckID string `json:"CheckID" xrpl:"CheckID"`
}

// NewCheckCancel creates a new CheckCancel transaction
func NewCheckCancel(account, checkID string) *CheckCancel {
	return &CheckCancel{
		BaseTx:  *tx.NewBaseTx(tx.TypeCheckCancel, account),
		CheckID: checkID,
	}
}

// TxType returns the transaction type
func (c *CheckCancel) TxType() tx.Type {
	return tx.TypeCheckCancel
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
	return tx.ReflectFlatten(c)
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CheckCancel) RequiredAmendments() []string {
	return []string{amendment.AmendmentChecks}
}

// Apply applies the CheckCreate transaction to ledger state.
func (c *CheckCreate) Apply(ctx *tx.ApplyContext) tx.Result {
	// Verify destination exists
	destID, err := sle.DecodeAccountID(c.Destination)
	if err != nil {
		return tx.TemINVALID
	}

	destKey := keylet.Account(destID)
	exists, _ := ctx.View.Exists(destKey)
	if !exists {
		return tx.TecNO_DST
	}

	// Parse SendMax - only XRP supported for now
	var sendMax uint64
	if c.SendMax.IsNative() {
		sendMax = uint64(c.SendMax.Drops())
	}

	// Check balance for XRP checks
	if c.SendMax.Currency == "" && sendMax > 0 {
		if ctx.Account.Balance < sendMax {
			return tx.TecUNFUNDED
		}
	}

	// Create the check entry
	accountID, _ := sle.DecodeAccountID(c.Account)
	sequence := *c.GetCommon().Sequence

	checkKey := keylet.Check(accountID, sequence)

	// Serialize check
	checkData, err := serializeCheck(c, accountID, destID, sequence, sendMax)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Insert check - creation tracked automatically by ApplyStateTable
	if err := ctx.View.Insert(checkKey, checkData); err != nil {
		return tx.TefINTERNAL
	}

	// Increase owner count
	ctx.Account.OwnerCount++

	return tx.TesSUCCESS
}

// Apply applies the CheckCash transaction to ledger state.
func (c *CheckCash) Apply(ctx *tx.ApplyContext) tx.Result {
	// Parse check ID
	checkID, err := hex.DecodeString(c.CheckID)
	if err != nil || len(checkID) != 32 {
		return tx.TemINVALID
	}

	var checkKeyBytes [32]byte
	copy(checkKeyBytes[:], checkID)
	checkKey := keylet.Keylet{Key: checkKeyBytes}

	// Read check
	checkData, err := ctx.View.Read(checkKey)
	if err != nil {
		return tx.TecNO_ENTRY
	}

	// Parse check
	check, err := sle.ParseCheck(checkData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Verify the account is the destination
	accountID, _ := sle.DecodeAccountID(c.Account)
	if check.DestinationID != accountID {
		return tx.TecNO_PERMISSION
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
		cashAmount = uint64(c.Amount.Drops())
		if cashAmount > check.SendMax {
			return tx.TecPATH_PARTIAL
		}
	} else if c.DeliverMin != nil {
		// Minimum amount - use full SendMax for simplicity
		deliverMin := uint64(c.DeliverMin.Drops())
		if check.SendMax < deliverMin {
			return tx.TecPATH_PARTIAL
		}
		cashAmount = check.SendMax
	}

	// Get the check creator's account
	creatorKey := keylet.Account(check.Account)
	creatorData, err := ctx.View.Read(creatorKey)
	if err != nil {
		return tx.TefINTERNAL
	}

	creatorAccount, err := sle.ParseAccountRoot(creatorData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Check if creator has sufficient balance
	if creatorAccount.Balance < cashAmount {
		return tx.TecUNFUNDED_PAYMENT
	}

	// Transfer the funds
	creatorAccount.Balance -= cashAmount
	ctx.Account.Balance += cashAmount

	// Decrease creator's owner count
	if creatorAccount.OwnerCount > 0 {
		creatorAccount.OwnerCount--
	}

	// Update creator account - modification tracked automatically by ApplyStateTable
	creatorUpdatedData, err := sle.SerializeAccountRoot(creatorAccount)
	if err != nil {
		return tx.TefINTERNAL
	}

	if err := ctx.View.Update(creatorKey, creatorUpdatedData); err != nil {
		return tx.TefINTERNAL
	}

	// Delete the check - deletion tracked automatically by ApplyStateTable
	if err := ctx.View.Erase(checkKey); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}

// Apply applies the CheckCancel transaction to ledger state.
func (c *CheckCancel) Apply(ctx *tx.ApplyContext) tx.Result {
	// Parse check ID
	checkID, err := hex.DecodeString(c.CheckID)
	if err != nil || len(checkID) != 32 {
		return tx.TemINVALID
	}

	var checkKeyBytes [32]byte
	copy(checkKeyBytes[:], checkID)
	checkKey := keylet.Keylet{Key: checkKeyBytes}

	// Read check
	checkData, err := ctx.View.Read(checkKey)
	if err != nil {
		return tx.TecNO_ENTRY
	}

	// Parse check
	check, err := sle.ParseCheck(checkData)
	if err != nil {
		return tx.TefINTERNAL
	}

	accountID, _ := sle.DecodeAccountID(c.Account)
	isCreator := check.Account == accountID
	isDestination := check.DestinationID == accountID

	// Only creator or destination can cancel
	if !isCreator && !isDestination {
		// Unless the check is expired
		if check.Expiration == 0 {
			return tx.TecNO_PERMISSION
		}
		// In full implementation, check if expired
		// For standalone mode, allow anyone to cancel expired checks
	}

	// Delete the check - deletion tracked automatically by ApplyStateTable
	if err := ctx.View.Erase(checkKey); err != nil {
		return tx.TefINTERNAL
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
			creatorAccount, err := sle.ParseAccountRoot(creatorData)
			if err == nil && creatorAccount.OwnerCount > 0 {
				creatorAccount.OwnerCount--
				creatorUpdatedData, _ := sle.SerializeAccountRoot(creatorAccount)
				ctx.View.Update(creatorKey, creatorUpdatedData)
			}
		}
	}

	return tx.TesSUCCESS
}

// serializeCheck serializes a Check ledger entry
func serializeCheck(checkTx *CheckCreate, ownerID, destID [20]byte, sequence uint32, sendMax uint64) ([]byte, error) {
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

	if checkTx.Expiration != nil {
		jsonObj["Expiration"] = *checkTx.Expiration
	}

	if checkTx.DestinationTag != nil {
		jsonObj["DestinationTag"] = *checkTx.DestinationTag
	}

	if checkTx.InvoiceID != "" {
		jsonObj["InvoiceID"] = checkTx.InvoiceID
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode Check: %w", err)
	}

	return hex.DecodeString(hexStr)
}
