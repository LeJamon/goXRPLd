package escrow

import (
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeEscrowCancel, func() tx.Transaction {
		return &EscrowCancel{BaseTx: *tx.NewBaseTx(tx.TypeEscrowCancel, "")}
	})
}

// EscrowCancel cancels an escrow, returning the escrowed XRP to the creator.
type EscrowCancel struct {
	tx.BaseTx

	// Owner is the account that created the escrow (required)
	Owner string `json:"Owner" xrpl:"Owner"`

	// OfferSequence is the sequence number of the EscrowCreate (required)
	OfferSequence uint32 `json:"OfferSequence" xrpl:"OfferSequence"`
}

// NewEscrowCancel creates a new EscrowCancel transaction
func NewEscrowCancel(account, owner string, offerSequence uint32) *EscrowCancel {
	return &EscrowCancel{
		BaseTx:        *tx.NewBaseTx(tx.TypeEscrowCancel, account),
		Owner:         owner,
		OfferSequence: offerSequence,
	}
}

// TxType returns the transaction type
func (e *EscrowCancel) TxType() tx.Type {
	return tx.TypeEscrowCancel
}

// Validate validates the EscrowCancel transaction
// Reference: rippled Escrow.cpp EscrowCancel::preflight()
func (e *EscrowCancel) Validate() error {
	if err := e.BaseTx.Validate(); err != nil {
		return err
	}

	if e.Owner == "" {
		return errors.New("temMALFORMED: Owner is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (e *EscrowCancel) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(e)
}

// Apply applies an EscrowCancel transaction
func (ec *EscrowCancel) Apply(ctx *tx.ApplyContext) tx.Result {
	// Get the escrow owner's account ID
	ownerID, err := sle.DecodeAccountID(ec.Owner)
	if err != nil {
		return tx.TemINVALID
	}

	// Find the escrow
	escrowKey := keylet.Escrow(ownerID, ec.OfferSequence)
	escrowData, err := ctx.View.Read(escrowKey)
	if err != nil {
		return tx.TecNO_TARGET
	}

	// Parse escrow
	escrowEntry, err := sle.ParseEscrow(escrowData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Check CancelAfter time (if set)
	if escrowEntry.CancelAfter > 0 {
		// In a full implementation, we'd check against the close time
		// For now, we'll allow it in standalone mode
		if !ctx.Config.Standalone {
			// Would check: if currentTime < escrow.CancelAfter return TecNO_PERMISSION
		}
	} else {
		// If no CancelAfter, only the creator can cancel (implied by having condition)
		if ec.Account != ec.Owner {
			return tx.TecNO_PERMISSION
		}
	}

	// Return the escrowed amount to the owner
	ownerKey := keylet.Account(ownerID)
	ownerData, err := ctx.View.Read(ownerKey)
	if err != nil {
		return tx.TefINTERNAL
	}

	ownerAccount, err := sle.ParseAccountRoot(ownerData)
	if err != nil {
		return tx.TefINTERNAL
	}

	ownerAccount.Balance += escrowEntry.Amount
	if ownerAccount.OwnerCount > 0 {
		ownerAccount.OwnerCount--
	}

	ownerUpdatedData, err := sle.SerializeAccountRoot(ownerAccount)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Update owner - modification tracked automatically by ApplyStateTable
	if err := ctx.View.Update(ownerKey, ownerUpdatedData); err != nil {
		return tx.TefINTERNAL
	}

	// Delete the escrow - deletion tracked automatically by ApplyStateTable
	if err := ctx.View.Erase(escrowKey); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}
