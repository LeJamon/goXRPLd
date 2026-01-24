// Package builders provides fluent transaction builder helpers for testing.
// These builders make it easy to construct transactions for test scenarios
// without dealing with the full complexity of the transaction types.
package builders

import (
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/payment"
)

// PaymentBuilder provides a fluent interface for building Payment transactions.
type PaymentBuilder struct {
	from       *Account
	to         *Account
	amount     uint64 // XRP in drops
	issuedAmt  *tx.Amount
	fee        uint64
	destTag    *uint32
	sourceTag  *uint32
	invoiceID  string
	sendMax    *tx.Amount
	deliverMin *tx.Amount
	sequence   *uint32
	flags      uint32
	memos      []tx.MemoWrapper
}

// Pay creates a new PaymentBuilder for an XRP payment.
// The amount is specified in drops (1 XRP = 1,000,000 drops).
func Pay(from, to *Account, amount uint64) *PaymentBuilder {
	return &PaymentBuilder{
		from:   from,
		to:     to,
		amount: amount,
		fee:    10, // Default fee: 10 drops
	}
}

// PayIssued creates a new PaymentBuilder for an issued currency payment.
func PayIssued(from, to *Account, amount tx.Amount) *PaymentBuilder {
	return &PaymentBuilder{
		from:      from,
		to:        to,
		issuedAmt: &amount,
		fee:       10, // Default fee: 10 drops
	}
}

// Fee sets the transaction fee in drops.
func (b *PaymentBuilder) Fee(f uint64) *PaymentBuilder {
	b.fee = f
	return b
}

// DestTag sets the destination tag.
func (b *PaymentBuilder) DestTag(tag uint32) *PaymentBuilder {
	b.destTag = &tag
	return b
}

// SourceTag sets the source tag.
func (b *PaymentBuilder) SourceTag(tag uint32) *PaymentBuilder {
	b.sourceTag = &tag
	return b
}

// InvoiceID sets the invoice ID (256-bit hash as hex string).
func (b *PaymentBuilder) InvoiceID(id string) *PaymentBuilder {
	b.invoiceID = id
	return b
}

// SendMax sets the maximum amount to send (for cross-currency payments).
func (b *PaymentBuilder) SendMax(amount tx.Amount) *PaymentBuilder {
	b.sendMax = &amount
	return b
}

// DeliverMin sets the minimum amount to deliver (for partial payments).
func (b *PaymentBuilder) DeliverMin(amount tx.Amount) *PaymentBuilder {
	b.deliverMin = &amount
	return b
}

// Sequence sets the sequence number explicitly.
func (b *PaymentBuilder) Sequence(seq uint32) *PaymentBuilder {
	b.sequence = &seq
	return b
}

// PartialPayment enables the partial payment flag.
func (b *PaymentBuilder) PartialPayment() *PaymentBuilder {
	b.flags |= tx.PaymentFlagPartialPayment
	return b
}

// NoDirectRipple enables the no direct ripple flag.
func (b *PaymentBuilder) NoDirectRipple() *PaymentBuilder {
	b.flags |= tx.PaymentFlagNoDirectRipple
	return b
}

// LimitQuality enables the limit quality flag.
func (b *PaymentBuilder) LimitQuality() *PaymentBuilder {
	b.flags |= tx.PaymentFlagLimitQuality
	return b
}

// WithMemo adds a memo to the transaction.
func (b *PaymentBuilder) WithMemo(memoType, memoData, memoFormat string) *PaymentBuilder {
	b.memos = append(b.memos, tx.MemoWrapper{
		Memo: tx.Memo{
			MemoType:   memoType,
			MemoData:   memoData,
			MemoFormat: memoFormat,
		},
	})
	return b
}

// Build constructs the Payment transaction.
func (b *PaymentBuilder) Build() tx.Transaction {
	var amount tx.Amount
	if b.issuedAmt != nil {
		amount = *b.issuedAmt
	} else {
		amount = tx.NewXRPAmount(fmt.Sprintf("%d", b.amount))
	}

	payment := payment.NewPayment(b.from.Address, b.to.Address, amount)
	payment.Fee = fmt.Sprintf("%d", b.fee)

	if b.destTag != nil {
		payment.DestinationTag = b.destTag
	}
	if b.sourceTag != nil {
		payment.SourceTag = b.sourceTag
	}
	if b.invoiceID != "" {
		payment.InvoiceID = b.invoiceID
	}
	if b.sendMax != nil {
		payment.SendMax = b.sendMax
	}
	if b.deliverMin != nil {
		payment.DeliverMin = b.deliverMin
	}
	if b.sequence != nil {
		payment.SetSequence(*b.sequence)
	}
	if b.flags != 0 {
		payment.SetFlags(b.flags)
	}
	if len(b.memos) > 0 {
		payment.Memos = b.memos
	}

	return payment
}

// BuildPayment is a convenience method that returns the concrete *payment.Payment type.
func (b *PaymentBuilder) BuildPayment() *payment.Payment {
	return b.Build().(*payment.Payment)
}
