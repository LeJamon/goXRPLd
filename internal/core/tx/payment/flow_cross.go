package payment

import (
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	tx "github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

// FlowCrossResult contains the result of an offer crossing operation
type FlowCrossResult struct {
	// TakerGot is the amount the taker received (what they wanted to buy)
	TakerGot EitherAmount

	// TakerPaid is the amount the taker paid (what they were selling)
	TakerPaid EitherAmount

	// Sandbox contains the state changes from the crossing
	Sandbox *PaymentSandbox

	// RemovableOffers contains offer keys that should be removed
	RemovableOffers map[[32]byte]bool

	// Result is the transaction result code
	Result tx.Result
}

// FlowCross executes offer crossing for an OfferCreate transaction.
// This is equivalent to rippled's flowCross() in CreateOffer.cpp.
//
// From the taker's (offer creator's) perspective:
//   - TakerGets = what the taker is SELLING (paying out)
//   - TakerPays = what the taker is BUYING (receiving)
//
// The function finds and crosses against existing offers on the opposite book.
//
// Parameters:
//   - view: The ledger view for reading/writing state
//   - takerAccount: The account creating the offer
//   - takerGets: Amount the taker is selling (what they'll pay)
//   - takerPays: Amount the taker wants to buy (what they'll receive)
//   - txHash: Current transaction hash for metadata
//   - ledgerSeq: Current ledger sequence
//
// Returns: FlowCrossResult with the amounts traded and state changes
func FlowCross(
	view tx.LedgerView,
	takerAccount [20]byte,
	takerGets, takerPays tx.Amount,
	txHash [32]byte,
	ledgerSeq uint32,
) FlowCrossResult {
	// Create sandbox for tracking changes
	sandbox := NewPaymentSandbox(view)
	sandbox.SetTransactionContext(txHash, ledgerSeq)

	// Convert amounts to EitherAmount
	// takerGets = what taker is selling (output from taker's perspective)
	// takerPays = what taker wants (input from taker's perspective)
	outAmt := ToEitherAmount(takerGets) // Taker pays this out
	inAmt := ToEitherAmount(takerPays)  // Taker receives this

	// Build the book step for crossing
	// The opposite book is: TakerPays currency -> TakerGets currency
	// i.e., offers where someone is selling what we want to buy
	inIssue := GetIssue(takerPays)  // What we want to receive
	outIssue := GetIssue(takerGets) // What we're paying

	// Create a single BookStep for the opposite order book
	// For offer crossing, we consume from the book where:
	//   - Book.In = what we're paying (takerGets)
	//   - Book.Out = what we're receiving (takerPays)
	// So we look at offers selling takerPays and buying takerGets
	bookStep := NewBookStep(outIssue, inIssue, takerAccount, takerAccount, nil, false)

	// Create strand with just the book step
	strand := Strand{bookStep}

	// Execute the flow
	// We want to receive up to takerPays amount
	result := Flow(sandbox, []Strand{strand}, inAmt, true, nil, &outAmt)

	return FlowCrossResult{
		TakerGot:        result.Out,  // What taker received
		TakerPaid:       result.In,   // What taker paid
		Sandbox:         result.Sandbox,
		RemovableOffers: result.RemovableOffers,
		Result:          result.Result,
	}
}

// GetTransferRate returns the transfer rate for an issuer account.
// Transfer rate is stored as a uint32 where QualityOne (1,000,000,000) means no fee.
// A value of 1,020,000,000 means a 2% transfer fee.
//
// Returns QualityOne if the issuer doesn't exist or has no transfer rate set.
func GetTransferRate(view tx.LedgerView, issuer [20]byte) uint32 {
	accountKey := keylet.Account(issuer)
	data, err := view.Read(accountKey)
	if err != nil || data == nil {
		return QualityOne
	}

	account, err := sle.ParseAccountRoot(data)
	if err != nil {
		return QualityOne
	}

	if account.TransferRate == 0 {
		return QualityOne
	}
	return account.TransferRate
}

// GetTransferRateByAddress returns the transfer rate for an issuer given its address string.
// This is a convenience wrapper around GetTransferRate.
func GetTransferRateByAddress(view tx.LedgerView, issuerAddress string) uint32 {
	if issuerAddress == "" {
		return QualityOne
	}

	issuerID, err := sle.DecodeAccountID(issuerAddress)
	if err != nil {
		return QualityOne
	}

	return GetTransferRate(view, issuerID)
}

// FlowCrossSimple is a simplified interface for offer crossing that takes tx.Amount
// and returns tx.Amount values.
//
// Parameters:
//   - view: The ledger view for reading/writing state
//   - takerAccountAddress: The account creating the offer (as string)
//   - takerGets: Amount the taker is selling
//   - takerPays: Amount the taker wants to buy
//   - txHash: Current transaction hash for metadata
//   - ledgerSeq: Current ledger sequence
//
// Returns:
//   - takerGot: Amount the taker received
//   - takerPaid: Amount the taker paid
//   - sandbox: State changes (may be nil)
//   - result: Transaction result code
func FlowCrossSimple(
	view tx.LedgerView,
	takerAccountAddress string,
	takerGets, takerPays tx.Amount,
	txHash [32]byte,
	ledgerSeq uint32,
) (takerGot, takerPaid tx.Amount, sandbox *PaymentSandbox, result tx.Result) {
	takerAccount, err := sle.DecodeAccountID(takerAccountAddress)
	if err != nil {
		return tx.Amount{}, tx.Amount{}, nil, tx.TefINTERNAL
	}

	crossResult := FlowCross(view, takerAccount, takerGets, takerPays, txHash, ledgerSeq)

	// Convert EitherAmount back to tx.Amount
	takerGot = FromEitherAmount(crossResult.TakerGot)
	takerPaid = FromEitherAmount(crossResult.TakerPaid)

	return takerGot, takerPaid, crossResult.Sandbox, crossResult.Result
}
