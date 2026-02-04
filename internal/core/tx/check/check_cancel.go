package check

import (
	"encoding/hex"
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeCheckCancel, func() tx.Transaction {
		return &CheckCancel{BaseTx: *tx.NewBaseTx(tx.TypeCheckCancel, "")}
	})
}

// CheckCreate creates a Check that can be cashed by the destination.

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
