package builders

import (
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	offertx "github.com/LeJamon/goXRPLd/internal/core/tx/offer"
)

// OfferCreateBuilder provides a fluent interface for building OfferCreate transactions.
type OfferCreateBuilder struct {
	account       *Account
	takerPays     tx.Amount
	takerGets     tx.Amount
	expiration    *uint32
	offerSequence *uint32
	fee           uint64
	sequence      *uint32
	flags         uint32
}

// OfferCreate creates a new OfferCreateBuilder.
// takerPays is what the offer creator receives, takerGets is what they pay.
func OfferCreate(account *Account, takerPays, takerGets tx.Amount) *OfferCreateBuilder {
	return &OfferCreateBuilder{
		account:   account,
		takerPays: takerPays,
		takerGets: takerGets,
		fee:       10, // Default fee: 10 drops
	}
}

// OfferCreateXRP creates an offer where one side is XRP.
// If buyXRP is true, creates an offer to buy XRP with issued currency.
// If buyXRP is false, creates an offer to sell XRP for issued currency.
func OfferCreateXRP(account *Account, xrpAmount uint64, issuedAmount tx.Amount, buyXRP bool) *OfferCreateBuilder {
	xrp := tx.NewXRPAmount(fmt.Sprintf("%d", xrpAmount))
	if buyXRP {
		// Offer creator receives XRP, pays issued currency
		return OfferCreate(account, xrp, issuedAmount)
	}
	// Offer creator receives issued currency, pays XRP
	return OfferCreate(account, issuedAmount, xrp)
}

// Expiration sets the expiration time (in Ripple epoch seconds).
func (b *OfferCreateBuilder) Expiration(exp uint32) *OfferCreateBuilder {
	b.expiration = &exp
	return b
}

// OfferSequence sets an existing offer to cancel when this one is created.
func (b *OfferCreateBuilder) OfferSequence(seq uint32) *OfferCreateBuilder {
	b.offerSequence = &seq
	return b
}

// Fee sets the transaction fee in drops.
func (b *OfferCreateBuilder) Fee(f uint64) *OfferCreateBuilder {
	b.fee = f
	return b
}

// Sequence sets the sequence number explicitly.
func (b *OfferCreateBuilder) Sequence(seq uint32) *OfferCreateBuilder {
	b.sequence = &seq
	return b
}

// Passive makes the offer passive (won't consume matching offers).
func (b *OfferCreateBuilder) Passive() *OfferCreateBuilder {
	b.flags |= offertx.OfferCreateFlagPassive
	return b
}

// ImmediateOrCancel makes the offer immediate-or-cancel.
// The offer will only take what's available immediately and cancel the rest.
func (b *OfferCreateBuilder) ImmediateOrCancel() *OfferCreateBuilder {
	b.flags |= offertx.OfferCreateFlagImmediateOrCancel
	return b
}

// FillOrKill makes the offer fill-or-kill.
// The offer must be fully filled or it will be cancelled entirely.
func (b *OfferCreateBuilder) FillOrKill() *OfferCreateBuilder {
	b.flags |= offertx.OfferCreateFlagFillOrKill
	return b
}

// Sell makes this a sell offer.
// The taker gets at least as much as TakerGets, possibly more.
func (b *OfferCreateBuilder) Sell() *OfferCreateBuilder {
	b.flags |= offertx.OfferCreateFlagSell
	return b
}

// Build constructs the OfferCreate transaction.
func (b *OfferCreateBuilder) Build() tx.Transaction {
	offer := offertx.NewOfferCreate(b.account.Address, b.takerGets, b.takerPays)
	offer.Fee = fmt.Sprintf("%d", b.fee)

	if b.expiration != nil {
		offer.Expiration = b.expiration
	}
	if b.offerSequence != nil {
		offer.OfferSequence = b.offerSequence
	}
	if b.sequence != nil {
		offer.SetSequence(*b.sequence)
	}
	if b.flags != 0 {
		offer.SetFlags(b.flags)
	}

	return offer
}

// BuildOfferCreate is a convenience method that returns the concrete *offertx.OfferCreate type.
func (b *OfferCreateBuilder) BuildOfferCreate() *offertx.OfferCreate {
	return b.Build().(*offertx.OfferCreate)
}

// OfferCancelBuilder provides a fluent interface for building OfferCancel transactions.
type OfferCancelBuilder struct {
	account  *Account
	offerSeq uint32
	fee      uint64
	sequence *uint32
}

// OfferCancel creates a new OfferCancelBuilder.
// offerSeq is the sequence number of the OfferCreate transaction to cancel.
func OfferCancel(account *Account, offerSeq uint32) *OfferCancelBuilder {
	return &OfferCancelBuilder{
		account:  account,
		offerSeq: offerSeq,
		fee:      10, // Default fee: 10 drops
	}
}

// Fee sets the transaction fee in drops.
func (b *OfferCancelBuilder) Fee(f uint64) *OfferCancelBuilder {
	b.fee = f
	return b
}

// Sequence sets the sequence number explicitly.
func (b *OfferCancelBuilder) Sequence(seq uint32) *OfferCancelBuilder {
	b.sequence = &seq
	return b
}

// Build constructs the OfferCancel transaction.
func (b *OfferCancelBuilder) Build() tx.Transaction {
	offer := offertx.NewOfferCancel(b.account.Address, b.offerSeq)
	offer.Fee = fmt.Sprintf("%d", b.fee)

	if b.sequence != nil {
		offer.SetSequence(*b.sequence)
	}

	return offer
}

// BuildOfferCancel is a convenience method that returns the concrete *offertx.OfferCancel type.
func (b *OfferCancelBuilder) BuildOfferCancel() *offertx.OfferCancel {
	return b.Build().(*offertx.OfferCancel)
}

// Amount helpers for creating amounts in tests

// XRP creates an XRP amount from drops.
func XRP(drops uint64) tx.Amount {
	return tx.NewXRPAmount(fmt.Sprintf("%d", drops))
}

// XRPFromAmount creates an XRP amount from a whole XRP value (e.g., 100 XRP).
func XRPFromAmount(xrp float64) tx.Amount {
	drops := uint64(xrp * 1_000_000)
	return tx.NewXRPAmount(fmt.Sprintf("%d", drops))
}

// IssuedCurrency creates an issued currency amount.
func IssuedCurrency(value, currency, issuer string) tx.Amount {
	return tx.NewIssuedAmount(value, currency, issuer)
}

// USD creates a USD amount with the specified value and issuer.
func USD(value string, issuer *Account) tx.Amount {
	return tx.NewIssuedAmount(value, "USD", issuer.Address)
}

// EUR creates a EUR amount with the specified value and issuer.
func EUR(value string, issuer *Account) tx.Amount {
	return tx.NewIssuedAmount(value, "EUR", issuer.Address)
}

// BTC creates a BTC amount with the specified value and issuer.
func BTC(value string, issuer *Account) tx.Amount {
	return tx.NewIssuedAmount(value, "BTC", issuer.Address)
}
