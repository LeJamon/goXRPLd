package ticket

import (
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeTicketCreate, func() tx.Transaction {
		return &TicketCreate{BaseTx: *tx.NewBaseTx(tx.TypeTicketCreate, "")}
	})
}

// TicketCreate creates tickets for future transactions.
type TicketCreate struct {
	tx.BaseTx

	// TicketCount is the number of tickets to create (required, 1-250)
	TicketCount uint32 `json:"TicketCount" xrpl:"TicketCount"`
}

// NewTicketCreate creates a new TicketCreate transaction
func NewTicketCreate(account string, count uint32) *TicketCreate {
	return &TicketCreate{
		BaseTx:      *tx.NewBaseTx(tx.TypeTicketCreate, account),
		TicketCount: count,
	}
}

// TxType returns the transaction type
func (t *TicketCreate) TxType() tx.Type {
	return tx.TypeTicketCreate
}

// Validate validates the TicketCreate transaction
func (t *TicketCreate) Validate() error {
	if err := t.BaseTx.Validate(); err != nil {
		return err
	}

	if t.TicketCount == 0 || t.TicketCount > 250 {
		return errors.New("TicketCount must be 1-250")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (t *TicketCreate) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(t)
}

// Apply applies the TicketCreate transaction to ledger state.
// Reference: rippled CreateTicket.cpp doApply()
func (t *TicketCreate) Apply(ctx *tx.ApplyContext) tx.Result {
	ownerDirKey := keylet.OwnerDir(ctx.AccountID)

	for i := uint32(0); i < t.TicketCount; i++ {
		ticketSeq := ctx.Account.Sequence + i

		ticketKey := keylet.Ticket(ctx.AccountID, ticketSeq)

		ticketData, err := sle.SerializeTicket(ctx.AccountID, ticketSeq)
		if err != nil {
			return tx.TefINTERNAL
		}

		if err := ctx.View.Insert(ticketKey, ticketData); err != nil {
			return tx.TefINTERNAL
		}

		// Add ticket to owner directory
		_, err = sle.DirInsert(ctx.View, ownerDirKey, ticketKey.Key, func(dir *sle.DirectoryNode) {
			dir.Owner = ctx.AccountID
		})
		if err != nil {
			return tx.TecDIR_FULL
		}
	}

	ctx.Account.OwnerCount += t.TicketCount
	ctx.Account.Sequence += t.TicketCount

	return tx.TesSUCCESS
}
