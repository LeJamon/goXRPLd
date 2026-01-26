package depositpreauth

import (
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeDepositPreauth, func() tx.Transaction {
		return &DepositPreauth{BaseTx: *tx.NewBaseTx(tx.TypeDepositPreauth, "")}
	})

}

// DepositPreauth preauthorizes an account for direct deposits.
type DepositPreauth struct {
	tx.BaseTx

	// Authorize is the account to preauthorize (mutually exclusive with Unauthorize)
	Authorize string `json:"Authorize,omitempty" xrpl:"Authorize,omitempty"`

	// Unauthorize is the account to remove preauthorization (mutually exclusive with Authorize)
	Unauthorize string `json:"Unauthorize,omitempty" xrpl:"Unauthorize,omitempty"`
}

// NewDepositPreauth creates a new DepositPreauth transaction
func NewDepositPreauth(account string) *DepositPreauth {
	return &DepositPreauth{
		BaseTx: *tx.NewBaseTx(tx.TypeDepositPreauth, account),
	}
}

// TxType returns the transaction type
func (d *DepositPreauth) TxType() tx.Type {
	return tx.TypeDepositPreauth
}

// Validate validates the DepositPreauth transaction
func (d *DepositPreauth) Validate() error {
	if err := d.BaseTx.Validate(); err != nil {
		return err
	}

	// Must have exactly one of Authorize or Unauthorize
	hasAuthorize := d.Authorize != ""
	hasUnauthorize := d.Unauthorize != ""

	if !hasAuthorize && !hasUnauthorize {
		return errors.New("must specify Authorize or Unauthorize")
	}

	if hasAuthorize && hasUnauthorize {
		return errors.New("cannot specify both Authorize and Unauthorize")
	}

	// Cannot authorize/unauthorize self
	if d.Authorize == d.Account || d.Unauthorize == d.Account {
		return errors.New("cannot preauthorize self")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (d *DepositPreauth) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(d)
}

// SetAuthorize sets the account to authorize
func (d *DepositPreauth) SetAuthorize(account string) {
	d.Authorize = account
	d.Unauthorize = ""
}

// SetUnauthorize sets the account to unauthorize
func (d *DepositPreauth) SetUnauthorize(account string) {
	d.Unauthorize = account
	d.Authorize = ""
}

// Apply applies the DepositPreauth transaction to ledger state.
func (d *DepositPreauth) Apply(ctx *tx.ApplyContext) tx.Result {
	if d.Authorize != "" {
		authorizedID, err := sle.DecodeAccountID(d.Authorize)
		if err != nil {
			return tx.TemINVALID
		}

		authorizedKey := keylet.Account(authorizedID)
		exists, _ := ctx.View.Exists(authorizedKey)
		if !exists {
			return tx.TecNO_TARGET
		}

		preauthKey := keylet.DepositPreauth(ctx.AccountID, authorizedID)

		exists, _ = ctx.View.Exists(preauthKey)
		if exists {
			return tx.TecDUPLICATE
		}

		preauthData, err := sle.SerializeDepositPreauth(ctx.AccountID, authorizedID)
		if err != nil {
			return tx.TefINTERNAL
		}

		if err := ctx.View.Insert(preauthKey, preauthData); err != nil {
			return tx.TefINTERNAL
		}

		ctx.Account.OwnerCount++
	} else if d.Unauthorize != "" {
		unauthorizedID, err := sle.DecodeAccountID(d.Unauthorize)
		if err != nil {
			return tx.TemINVALID
		}

		preauthKey := keylet.DepositPreauth(ctx.AccountID, unauthorizedID)

		exists, _ := ctx.View.Exists(preauthKey)
		if !exists {
			return tx.TecNO_ENTRY
		}

		if err := ctx.View.Erase(preauthKey); err != nil {
			return tx.TefINTERNAL
		}

		if ctx.Account.OwnerCount > 0 {
			ctx.Account.OwnerCount--
		}
	}

	return tx.TesSUCCESS
}
