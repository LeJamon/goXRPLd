package payment

import (
	"sort"

	tx "github.com/LeJamon/goXRPLd/internal/core/tx"
)

// Flow executes payment across multiple strands, selecting the best quality paths.
//
// The algorithm:
//  1. Calculate quality upper bound for each strand
//  2. Sort strands by quality (best first)
//  3. For each iteration:
//     a. Execute each active strand with remaining output
//     b. Select the strand with best actual quality
//     c. Apply that strand's changes
//     d. Accumulate results
//     e. Remove exhausted strands
//  4. Continue until output satisfied or all strands dry
//
// Parameters:
//   - baseView: PaymentSandbox with ledger state
//   - strands: List of executable strands
//   - outReq: Requested output amount
//   - partialPayment: Whether partial payments are allowed
//   - limitQuality: Optional quality limit (nil means no limit)
//   - sendMax: Optional maximum input amount
//
// Returns: FlowResult with actual amounts and state changes
func Flow(
	baseView *PaymentSandbox,
	strands []Strand,
	outReq EitherAmount,
	partialPayment bool,
	limitQuality *Quality,
	sendMax *EitherAmount,
) FlowResult {
	if len(strands) == 0 {
		return FlowResult{
			In:              ZeroXRPEitherAmount(),
			Out:             ZeroXRPEitherAmount(),
			Sandbox:         nil,
			RemovableOffers: nil,
			Result:          tx.TecPATH_DRY,
		}
	}

	// Create the main sandbox that accumulates all changes
	accumSandbox := NewChildSandbox(baseView)
	allOfrsToRm := make(map[[32]byte]bool)

	// Initialize result accumulators
	// totalOut matches the type of outReq (what we're delivering)
	// totalIn matches the type of sendMax (what we're spending), or XRP if not specified
	var totalIn, totalOut EitherAmount
	if outReq.IsNative {
		totalOut = ZeroXRPEitherAmount()
	} else {
		totalOut = ZeroIOUEitherAmount(outReq.IOU.Currency, outReq.IOU.Issuer)
	}
	// Initialize totalIn based on sendMax type, or default to XRP
	if sendMax != nil {
		if sendMax.IsNative {
			totalIn = ZeroXRPEitherAmount()
		} else {
			totalIn = ZeroIOUEitherAmount(sendMax.IOU.Currency, sendMax.IOU.Issuer)
		}
	} else {
		// Default to XRP if no sendMax specified
		totalIn = ZeroXRPEitherAmount()
	}

	// Track remaining output needed
	remainingOut := outReq

	// Track remaining input available (if sendMax specified)
	var remainingIn *EitherAmount
	if sendMax != nil {
		ri := *sendMax
		remainingIn = &ri
	}

	// Sort strands by quality upper bound
	strandQualities := make([]strandQuality, 0, len(strands))
	for i, strand := range strands {
		q := GetStrandQuality(strand, baseView)
		if q != nil {
			strandQualities = append(strandQualities, strandQuality{
				index:   i,
				strand:  strand,
				quality: *q,
				active:  true,
			})
		}
	}

	// Sort by quality (lower value = better quality)
	sort.Slice(strandQualities, func(i, j int) bool {
		return strandQualities[i].quality.BetterThan(strandQualities[j].quality)
	})

	// Limit iterations to prevent infinite loops
	const maxIterations = 2000
	iterations := 0

	// Main flow loop
	for !remainingOut.IsZero() && hasActiveStrands(strandQualities) && iterations < maxIterations {
		iterations++

		// Execute each active strand and find the best one
		bestResult := StrandResult{}
		bestIndex := -1
		bestQuality := Quality{Value: ^uint64(0)} // Worst possible quality

		for i := range strandQualities {
			sq := &strandQualities[i]
			if !sq.active {
				continue
			}

			// Check quality limit
			if limitQuality != nil && sq.quality.WorseThan(*limitQuality) {
				sq.active = false
				continue
			}

			// Execute this strand
			result := ExecuteStrand(accumSandbox, sq.strand, remainingIn, remainingOut)

			if !result.Success || result.Out.IsZero() {
				sq.active = false
				// Collect offers to remove even from failed strands
				for k, v := range result.OffsToRm {
					allOfrsToRm[k] = v
				}
				continue
			}

			// Mark as inactive if strand is exhausted
			if result.Inactive {
				sq.active = false
			}

			// Calculate actual quality of this execution
			actualQuality := QualityFromAmounts(result.In, result.Out)

			// Check if this is better than current best
			if bestIndex < 0 || actualQuality.BetterThan(bestQuality) {
				bestResult = result
				bestIndex = i
				bestQuality = actualQuality
			}
		}

		// If no strand produced output, we're done
		if bestIndex < 0 {
			break
		}

		// Apply the best strand's changes
		if bestResult.Sandbox != nil {
			bestResult.Sandbox.Apply(accumSandbox)
		}

		// Collect offers to remove
		for k, v := range bestResult.OffsToRm {
			allOfrsToRm[k] = v
		}

		// Accumulate results
		totalIn = totalIn.Add(bestResult.In)
		totalOut = totalOut.Add(bestResult.Out)

		// Update remaining amounts
		remainingOut = remainingOut.Sub(bestResult.Out)
		if remainingOut.IsNegative() {
			// Over-delivered - shouldn't happen but handle gracefully
			remainingOut = ZeroXRPEitherAmount()
			if !outReq.IsNative {
				remainingOut = ZeroIOUEitherAmount(outReq.IOU.Currency, outReq.IOU.Issuer)
			}
		}

		if remainingIn != nil {
			*remainingIn = remainingIn.Sub(bestResult.In)
			if remainingIn.IsNegative() || remainingIn.IsZero() {
				break // No more input available
			}
		}
	}

	// Determine final result code
	resultCode := tx.TesSUCCESS

	if totalOut.IsZero() {
		resultCode = tx.TecPATH_DRY
	} else if totalOut.Compare(outReq) < 0 {
		// Didn't deliver full amount
		if !partialPayment {
			resultCode = tx.TecPATH_PARTIAL
		}
	}

	return FlowResult{
		In:              totalIn,
		Out:             totalOut,
		Sandbox:         accumSandbox,
		RemovableOffers: allOfrsToRm,
		Result:          tx.Result(resultCode),
	}
}

// strandQuality pairs a strand with its quality for sorting
type strandQuality struct {
	index   int
	strand  Strand
	quality Quality
	active  bool
}

// hasActiveStrands returns true if any strand is still active
func hasActiveStrands(sq []strandQuality) bool {
	for _, s := range sq {
		if s.active {
			return true
		}
	}
	return false
}

// RippleCalculate is the main entry point for path-based payments.
// It converts paths to strands and executes the Flow algorithm.
//
// Parameters:
//   - view: LedgerView for reading state
//   - srcAccount: Source account sending the payment
//   - dstAccount: Destination account receiving the payment
//   - dstAmount: Amount to deliver to destination
//   - srcAmount: Maximum amount source will send (SendMax)
//   - paths: Payment paths from transaction
//   - addDefaultPath: Whether to include direct path
//   - partialPayment: Whether partial delivery is allowed
//   - limitQuality: Whether to limit exchange quality
//
// Returns:
//   - actualIn: Actual amount sent
//   - actualOut: Actual amount delivered
//   - removableOffers: Offers that should be removed
//   - sandbox: The PaymentSandbox containing all state changes
//   - result: Transaction result code
func RippleCalculate(
	view tx.LedgerView,
	srcAccount, dstAccount [20]byte,
	dstAmount tx.Amount,
	srcAmount *tx.Amount,
	paths [][]PathStep,
	addDefaultPath bool,
	partialPayment bool,
	limitQuality bool,
	txHash [32]byte,
	ledgerSeq uint32,
) (EitherAmount, EitherAmount, map[[32]byte]bool, *PaymentSandbox, tx.Result) {
	// Create PaymentSandbox from view
	sandbox := NewPaymentSandbox(view)
	sandbox.SetTransactionContext(txHash, ledgerSeq)

	// Convert paths to strands
	strands, strandResult := ToStrands(sandbox, srcAccount, dstAccount, dstAmount, srcAmount, paths, addDefaultPath)
	if strandResult != tx.TesSUCCESS || len(strands) == 0 {
		if strandResult == tx.TesSUCCESS {
			strandResult = tx.TecPATH_DRY
		}
		return ZeroXRPEitherAmount(), ZeroXRPEitherAmount(), nil, nil, strandResult
	}

	// Convert amounts to EitherAmount
	outReq := ToEitherAmount(dstAmount)

	var sendMax *EitherAmount
	if srcAmount != nil {
		sm := ToEitherAmount(*srcAmount)
		sendMax = &sm
	}

	// Calculate limit quality if requested
	var qualityLimit *Quality
	if limitQuality && sendMax != nil {
		// Limit quality is srcAmount / dstAmount
		q := QualityFromAmounts(*sendMax, outReq)
		qualityLimit = &q
	}

	// Execute flow
	result := Flow(sandbox, strands, outReq, partialPayment, qualityLimit, sendMax)

	// Apply flow sandbox changes back to the main sandbox
	if result.Result == tx.TesSUCCESS || result.Result == tx.TecPATH_PARTIAL {
		if result.Sandbox != nil {
			result.Sandbox.Apply(sandbox)
		}
	}

	return result.In, result.Out, result.RemovableOffers, sandbox, result.Result
}

// FlowV2 is an alternative flow implementation that matches rippled's FlowV2.
// It uses a slightly different iteration strategy.
func FlowV2(
	baseView *PaymentSandbox,
	strands []Strand,
	outReq EitherAmount,
	partialPayment bool,
	limitQuality *Quality,
	sendMax *EitherAmount,
) FlowResult {
	// For now, delegate to Flow
	// A full implementation would match rippled's FlowV2 exactly
	return Flow(baseView, strands, outReq, partialPayment, limitQuality, sendMax)
}

