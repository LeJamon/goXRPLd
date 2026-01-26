package account

import (
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeAccountDelete, func() tx.Transaction {
		return &AccountDelete{BaseTx: *tx.NewBaseTx(tx.TypeAccountDelete, "")}
	})

}

// AccountDelete deletes an account from the ledger.
type AccountDelete struct {
	tx.BaseTx

	// Destination is the account to receive remaining XRP (required)
	Destination string `json:"Destination" xrpl:"Destination"`

	// DestinationTag is an arbitrary tag for the destination (optional)
	DestinationTag *uint32 `json:"DestinationTag,omitempty" xrpl:"DestinationTag,omitempty"`
}

// NewAccountDelete creates a new AccountDelete transaction
func NewAccountDelete(account, destination string) *AccountDelete {
	return &AccountDelete{
		BaseTx:      *tx.NewBaseTx(tx.TypeAccountDelete, account),
		Destination: destination,
	}
}

// TxType returns the transaction type
func (a *AccountDelete) TxType() tx.Type {
	return tx.TypeAccountDelete
}

// Validate validates the AccountDelete transaction
func (a *AccountDelete) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	if a.Destination == "" {
		return errors.New("Destination is required")
	}

	// Cannot delete to self
	if a.Account == a.Destination {
		return errors.New("cannot delete account to self")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AccountDelete) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(a)
}

// Apply applies the AccountDelete transaction to ledger state.
func (a *AccountDelete) Apply(ctx *tx.ApplyContext) tx.Result {
	if ctx.Account.OwnerCount > 0 {
		return tx.TecHAS_OBLIGATIONS
	}

	if !ctx.Config.Standalone && ctx.Account.Sequence < 256 {
		return tx.TefTOO_BIG
	}

	destID, err := sle.DecodeAccountID(a.Destination)
	if err != nil {
		return tx.TemINVALID
	}

	destKey := keylet.Account(destID)
	destData, err := ctx.View.Read(destKey)
	if err != nil {
		return tx.TecNO_DST
	}

	destAccount, err := sle.ParseAccountRoot(destData)
	if err != nil {
		return tx.TefINTERNAL
	}

	destAccount.Balance += ctx.Account.Balance

	destUpdatedData, err := sle.SerializeAccountRoot(destAccount)
	if err != nil {
		return tx.TefINTERNAL
	}

	if err := ctx.View.Update(destKey, destUpdatedData); err != nil {
		return tx.TefINTERNAL
	}

	srcKey := keylet.Account(ctx.AccountID)
	if err := ctx.View.Erase(srcKey); err != nil {
		return tx.TefINTERNAL
	}

	ctx.Account.Balance = 0

	return tx.TesSUCCESS
}
