package escrow

import (
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeEscrowFinish, func() tx.Transaction {
		return &EscrowFinish{BaseTx: *tx.NewBaseTx(tx.TypeEscrowFinish, "")}
	})
}

// EscrowFinish completes an escrow, releasing the escrowed XRP.
type EscrowFinish struct {
	tx.BaseTx

	// Owner is the account that created the escrow (required)
	Owner string `json:"Owner" xrpl:"Owner"`

	// OfferSequence is the sequence number of the EscrowCreate (required)
	OfferSequence uint32 `json:"OfferSequence" xrpl:"OfferSequence"`

	// Condition is the crypto-condition that was fulfilled (optional)
	Condition string `json:"Condition,omitempty" xrpl:"Condition,omitempty"`

	// Fulfillment is the fulfillment for the condition (optional)
	Fulfillment string `json:"Fulfillment,omitempty" xrpl:"Fulfillment,omitempty"`
}

// NewEscrowFinish creates a new EscrowFinish transaction
func NewEscrowFinish(account, owner string, offerSequence uint32) *EscrowFinish {
	return &EscrowFinish{
		BaseTx:        *tx.NewBaseTx(tx.TypeEscrowFinish, account),
		Owner:         owner,
		OfferSequence: offerSequence,
	}
}

// TxType returns the transaction type
func (e *EscrowFinish) TxType() tx.Type {
	return tx.TypeEscrowFinish
}

// Validate validates the EscrowFinish transaction
// Reference: rippled Escrow.cpp EscrowFinish::preflight()
func (e *EscrowFinish) Validate() error {
	if err := e.BaseTx.Validate(); err != nil {
		return err
	}

	if e.Owner == "" {
		return errors.New("temMALFORMED: Owner is required")
	}

	// Both Condition and Fulfillment must be present or absent together
	// Reference: rippled Escrow.cpp:644-646
	hasCondition := e.Condition != ""
	hasFulfillment := e.Fulfillment != ""
	if hasCondition != hasFulfillment {
		return errors.New("temMALFORMED: Condition and Fulfillment must be provided together")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (e *EscrowFinish) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(e)
}

// Apply applies an EscrowFinish transaction
func (ef *EscrowFinish) Apply(ctx *tx.ApplyContext) tx.Result {
	// Get the escrow owner's account ID
	ownerID, err := sle.DecodeAccountID(ef.Owner)
	if err != nil {
		return tx.TemINVALID
	}

	// Find the escrow
	escrowKey := keylet.Escrow(ownerID, ef.OfferSequence)
	escrowData, err := ctx.View.Read(escrowKey)
	if err != nil {
		return tx.TecNO_TARGET
	}

	// Parse escrow
	escrowEntry, err := sle.ParseEscrow(escrowData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Check FinishAfter time (if set)
	if escrowEntry.FinishAfter > 0 {
		// In a full implementation, we'd check against the close time
		// For now, we'll allow it in standalone mode
		if !ctx.Config.Standalone {
			// Would check: if currentTime < escrow.FinishAfter return TecNO_PERMISSION
		}
	}

	// Check condition/fulfillment with proper crypto-condition verification
	// Reference: rippled Escrow.cpp preclaim() and checkCondition()
	if escrowEntry.Condition != "" {
		// If escrow has a condition, fulfillment must be provided
		if ef.Fulfillment == "" {
			return tx.TecCRYPTOCONDITION_ERROR
		}

		// Verify the fulfillment matches the condition
		// The escrow stores condition as hex, tx provides fulfillment as hex
		if err := validateCryptoCondition(ef.Fulfillment, escrowEntry.Condition); err != nil {
			return tx.TecCRYPTOCONDITION_ERROR
		}
	}

	// Get destination account
	destKey := keylet.Account(escrowEntry.DestinationID)
	destData, err := ctx.View.Read(destKey)
	if err != nil {
		return tx.TecNO_DST
	}

	destAccount, err := sle.ParseAccountRoot(destData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Transfer the escrowed amount to destination
	destAccount.Balance += escrowEntry.Amount

	// Update destination - modification tracked automatically by ApplyStateTable
	destUpdatedData, err := sle.SerializeAccountRoot(destAccount)
	if err != nil {
		return tx.TefINTERNAL
	}

	if err := ctx.View.Update(destKey, destUpdatedData); err != nil {
		return tx.TefINTERNAL
	}

	// Delete the escrow - deletion tracked automatically by ApplyStateTable
	if err := ctx.View.Erase(escrowKey); err != nil {
		return tx.TefINTERNAL
	}

	// Decrease owner count for escrow owner
	if ef.Owner != ef.Account {
		// Need to update owner's account too
		ownerKey := keylet.Account(ownerID)
		ownerData, err := ctx.View.Read(ownerKey)
		if err == nil {
			ownerAccount, err := sle.ParseAccountRoot(ownerData)
			if err == nil && ownerAccount.OwnerCount > 0 {
				ownerAccount.OwnerCount--
				ownerUpdatedData, err := sle.SerializeAccountRoot(ownerAccount)
				if err == nil {
					ctx.View.Update(ownerKey, ownerUpdatedData)
				}
			}
		}
	} else {
		if ctx.Account.OwnerCount > 0 {
			ctx.Account.OwnerCount--
		}
	}

	return tx.TesSUCCESS
}
