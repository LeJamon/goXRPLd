package check

import (
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
)

func init() {
	tx.Register(tx.TypeCheckCreate, func() tx.Transaction {
		return &CheckCreate{BaseTx: *tx.NewBaseTx(tx.TypeCheckCreate, "")}
	})
}

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
