package payment

import (
	"encoding/hex"
	"strconv"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx/handler"
)

// applyXRPPayment processes an XRP-to-XRP payment.
func (h *Handler) applyXRPPayment(payment *Payment, sender *handler.AccountRoot, ctx *handler.Context) handler.Result {
	// Parse the amount
	amountDrops, err := strconv.ParseUint(payment.Amount.Value, 10, 64)
	if err != nil || amountDrops == 0 {
		return handler.TemBAD_AMOUNT
	}

	// Check sender has enough balance (including reserve)
	requiredBalance := amountDrops + ctx.Config.ReserveBase
	if sender.Balance < requiredBalance {
		return handler.TecUNFUNDED_PAYMENT
	}

	// Get destination account
	destAccountID, err := handler.DecodeAccountID(payment.Destination)
	if err != nil {
		return handler.TemDST_NEEDED
	}
	destKey := keylet.Account(destAccountID)

	destExists, err := ctx.View.Exists(destKey)
	if err != nil {
		return handler.TefINTERNAL
	}

	if destExists {
		return h.creditExistingAccount(payment, sender, destKey, amountDrops, ctx)
	}

	return h.createNewAccount(payment, sender, destKey, amountDrops, ctx)
}

// creditExistingAccount credits an existing destination account.
func (h *Handler) creditExistingAccount(payment *Payment, sender *handler.AccountRoot, destKey keylet.Keylet, amountDrops uint64, ctx *handler.Context) handler.Result {
	destData, err := ctx.View.Read(destKey)
	if err != nil {
		return handler.TefINTERNAL
	}

	destAccount, err := handler.ParseAccountRoot(destData)
	if err != nil {
		return handler.TefINTERNAL
	}

	previousDestBalance := destAccount.Balance

	// Check if destination requires a tag
	if (destAccount.Flags&0x00020000) != 0 && payment.DestinationTag == nil {
		return handler.TecDST_TAG_NEEDED
	}

	// Credit destination
	destAccount.Balance += amountDrops

	// Debit sender
	sender.Balance -= amountDrops

	// Update destination
	updatedDestData, err := handler.SerializeAccountRoot(destAccount)
	if err != nil {
		return handler.TefINTERNAL
	}

	if err := ctx.View.Update(destKey, updatedDestData); err != nil {
		return handler.TefINTERNAL
	}

	// Record destination modification
	ctx.Metadata.AffectedNodes = append(ctx.Metadata.AffectedNodes, handler.AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(destKey.Key[:]),
		FinalFields: map[string]any{
			"Account": payment.Destination,
			"Balance": strconv.FormatUint(destAccount.Balance, 10),
		},
		PreviousFields: map[string]any{
			"Balance": strconv.FormatUint(previousDestBalance, 10),
		},
	})

	// Set delivered amount in metadata
	delivered := payment.Amount
	ctx.Metadata.DeliveredAmount = &delivered

	return handler.TesSUCCESS
}

// createNewAccount creates a new account for the destination.
func (h *Handler) createNewAccount(payment *Payment, sender *handler.AccountRoot, destKey keylet.Keylet, amountDrops uint64, ctx *handler.Context) handler.Result {
	// Check minimum amount for account creation
	if amountDrops < ctx.Config.ReserveBase {
		return handler.TecNO_DST_INSUF_XRP
	}

	// Create new account
	newAccount := &handler.AccountRoot{
		Account:  payment.Destination,
		Balance:  amountDrops,
		Sequence: 1,
		Flags:    0,
	}

	// Debit sender
	sender.Balance -= amountDrops

	// Serialize and insert new account
	newAccountData, err := handler.SerializeAccountRoot(newAccount)
	if err != nil {
		return handler.TefINTERNAL
	}

	if err := ctx.View.Insert(destKey, newAccountData); err != nil {
		return handler.TefINTERNAL
	}

	// Record account creation
	ctx.Metadata.AffectedNodes = append(ctx.Metadata.AffectedNodes, handler.AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(destKey.Key[:]),
		NewFields: map[string]any{
			"Account":  payment.Destination,
			"Balance":  strconv.FormatUint(amountDrops, 10),
			"Sequence": uint32(1),
		},
	})

	// Set delivered amount
	delivered := payment.Amount
	ctx.Metadata.DeliveredAmount = &delivered

	return handler.TesSUCCESS
}
