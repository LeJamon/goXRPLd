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

	// TakerPaid is the GROSS amount the taker paid (debited from their balance, including transfer fees)
	// Use this to check if the taker fully consumed their offer (GROSS >= originalTakerGets)
	TakerPaid EitherAmount

	// TakerPaidNet is the NET amount delivered to matching offers (after transfer fees)
	// Use this to calculate the remaining offer amount (remaining = original - NET)
	TakerPaidNet EitherAmount

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
//   - passive: If true, only cross offers with STRICTLY better quality
//
// Returns: FlowCrossResult with the amounts traded and state changes
func FlowCross(
	view tx.LedgerView,
	takerAccount [20]byte,
	takerGets, takerPays tx.Amount,
	txHash [32]byte,
	ledgerSeq uint32,
	passive bool,
) FlowCrossResult {

	// Create sandbox for tracking changes
	sandbox := NewPaymentSandbox(view)
	sandbox.SetTransactionContext(txHash, ledgerSeq)

	// Convert amounts to EitherAmount
	// takerGets = what taker is selling (output from taker's perspective)
	// takerPays = what taker wants (input from taker's perspective)
	inAmt := ToEitherAmount(takerPays) // What taker receives

	// Build the book step for crossing
	// The opposite book is: TakerPays currency -> TakerGets currency
	// i.e., offers where someone is selling what we want to buy
	inIssue := GetIssue(takerPays)  // What we want to receive
	outIssue := GetIssue(takerGets) // What we're paying

	// === Calculate sendMax: the maximum the taker can actually spend ===
	// Reference: rippled CreateOffer.cpp flowCross() lines 329-367
	//
	// The key insight from rippled:
	// 1. Start with takerGets (the offer amount)
	// 2. MULTIPLY by transfer rate to get gross spend needed for that delivery
	// 3. Limit to available balance
	//
	// sendMax = min(takerGets * transferRate, balance)
	//
	// This allows the taker to spend their full balance. The transfer rate means
	// they'll deliver less than they spend, but they should be able to spend up
	// to their balance.
	//
	// 1. Get taker's available balance for what they're selling
	inStartBalance := tx.AccountFunds(view, takerAccount, takerGets, true)

	// 2. Calculate sendMax accounting for transfer rate
	sendMax := ToEitherAmount(takerGets)

	if !takerGets.IsNative() {
		issuerID, err := sle.DecodeAccountID(takerGets.Issuer)
		if err == nil && takerAccount != issuerID {
			// Get transfer rate from issuer
			transferRate := GetTransferRate(view, issuerID)
			if transferRate != QualityOne {
				// sendMax = takerGets * (transferRate / QualityOne)
				// This is the gross spend needed to deliver takerGets amount
				sendMax = MulRatio(sendMax, transferRate, QualityOne, true)
			}
		}
	}

	// 3. Limit sendMax to available balance
	availableBalance := ToEitherAmount(inStartBalance)
	if sendMax.Compare(availableBalance) > 0 {
		sendMax = availableBalance
	}

	// Note: We pass GROSS amount (sendMax) to Flow, not NET
	// The BookStep will handle the GROSS→NET conversion internally via computeOutputFromInputWithTransferRate
	// Reference: rippled passes the gross available balance to flow
	flowSendMax := sendMax

	// Calculate quality limit for offer crossing
	// Quality = what you pay / what you get (from the crossing perspective)
	// Reference: rippled CreateOffer.cpp - Quality threshold{takerAmount.out, sendMax}
	// The threshold uses sendMax (adjusted for transfer rate) not the original takerGets
	// This makes the quality limit more permissive when there's a transfer rate
	// Quality = sendMax / inAmt = what we pay / what we get
	takerQuality := QualityFromAmounts(sendMax, inAmt)

	// For passive offers, increment the quality threshold so we only cross
	// against offers with STRICTLY better quality (not equal)
	// Reference: rippled CreateOffer.cpp lines 362-364
	if passive {
		takerQuality = takerQuality.Increment()
	}

	// Create a single BookStep for the opposite order book WITH quality limit
	// For offer crossing, we search the book where:
	//   - Book.In = takerGets (what we're paying = what matching offers TakerPays)
	//   - Book.Out = takerPays (what we want = what matching offers TakerGets)
	// This finds offers that are BUYING what we're SELLING (opposite side of market)
	// Pass quality limit to only cross offers at or better than taker's quality
	bookStep := NewBookStepWithQualityLimit(outIssue, inIssue, takerAccount, takerAccount, nil, false, &takerQuality)

	// Create strand with just the book step
	strand := Strand{bookStep}

	// Execute the flow with quality limit
	// We want to receive up to takerPays amount
	// Only cross offers at quality <= taker's quality
	// Pass flowSendMax (NET limit) to limit based on taker's available funds
	result := Flow(sandbox, []Strand{strand}, inAmt, true, &takerQuality, &flowSendMax)

	// Apply the flow sandbox changes to our root sandbox
	// Reference: rippled CreateOffer.cpp line 711: psbFlow.apply(sb)
	if result.Sandbox != nil {
		result.Sandbox.Apply(sandbox)
	}

	// result.In from Flow is the GROSS amount consumed from the taker
	// (what was debited from their balance, including transfer fees)
	// We need to calculate NET (what was delivered to matching offers)
	// NET = GROSS × QualityOne / transferRate
	takerPaidGross := result.In
	takerPaidNet := result.In // Start with GROSS, will convert to NET if needed

	if !takerGets.IsNative() {
		issuerID, err := sle.DecodeAccountID(takerGets.Issuer)
		if err == nil && takerAccount != issuerID {
			transferRate := GetTransferRate(view, issuerID)
			if transferRate != QualityOne && transferRate > 0 {
				// NET = GROSS * QualityOne / transferRate
				// This is the amount that was delivered to matching offers (after transfer fee)
				takerPaidNet = MulRatio(result.In, QualityOne, transferRate, false)
			}
		}
	}

	// TakerPaid is the GROSS amount the taker spent (debited from their balance, including transfer fees)
	// Use GROSS to check if offer is fully consumed: GROSS >= originalTakerGets
	// TakerPaidNet is the NET amount delivered to matching offers (after transfer fees)
	// Use NET to calculate remaining offer: remaining = originalTakerGets - NET
	// Reference: rippled CreateOffer.cpp - remaining calculation uses delivered amount
	return FlowCrossResult{
		TakerGot:        result.Out,       // What taker received (XRP)
		TakerPaid:       takerPaidGross,   // What taker spent (GROSS, including transfer fee)
		TakerPaidNet:    takerPaidNet,     // What was delivered to matching offers (NET)
		Sandbox:         sandbox,          // Return the root sandbox, not the child
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
