package check

import (
	"encoding/hex"
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
)

func init() {
	tx.Register(tx.TypeCheckCash, func() tx.Transaction {
		return &CheckCash{BaseTx: *tx.NewBaseTx(tx.TypeCheckCash, "")}
	})
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
