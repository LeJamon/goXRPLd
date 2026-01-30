package payment

import (
	"bytes"
	"errors"
	"math/big"
	"sort"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	tx "github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

// BookStep consumes liquidity from an order book.
// It iterates through offers at the best quality, consuming them until
// the requested amount is satisfied or liquidity is exhausted.
//
// Three variants exist based on in/out currency types:
// - BookStepII: IOU to IOU
// - BookStepIX: IOU to XRP
// - BookStepXI: XRP to IOU
//
// Based on rippled's BookStep implementation.
type BookStep struct {
	// book specifies the order book (in/out issues)
	book Book

	// strandSrc is the source account of the strand
	strandSrc [20]byte

	// strandDst is the destination account of the strand
	strandDst [20]byte

	// prevStep is the previous step (for transfer fee calculation)
	prevStep Step

	// ownerPaysTransferFee indicates if offer owner pays the transfer fee
	ownerPaysTransferFee bool

	// maxOffersToConsume is the limit on offers consumed per execution
	maxOffersToConsume uint32

	// qualityLimit is the worst quality offer that should be consumed.
	// If set, offers with worse quality (higher value) are not crossed.
	// This is used for offer crossing to only cross offers at or better
	// than the taker's quality.
	qualityLimit *Quality

	// inactive indicates the step is dry (too many offers consumed)
	inactive_ bool

	// offersUsed tracks offers consumed in last execution
	offersUsed_ uint32

	// cache holds results from the last Rev() call
	cache *bookCache
}

// bookCache holds cached values from the reverse pass
type bookCache struct {
	in  EitherAmount
	out EitherAmount
}

// NewBookStep creates a new BookStep for order book consumption
func NewBookStep(inIssue, outIssue Issue, strandSrc, strandDst [20]byte, prevStep Step, ownerPaysTransferFee bool) *BookStep {
	return &BookStep{
		book: Book{
			In:  inIssue,
			Out: outIssue,
		},
		strandSrc:            strandSrc,
		strandDst:            strandDst,
		prevStep:             prevStep,
		ownerPaysTransferFee: ownerPaysTransferFee,
		maxOffersToConsume:   1000, // fix1515 limit
		qualityLimit:         nil,
		inactive_:            false,
		offersUsed_:          0,
		cache:                nil,
	}
}

// NewBookStepWithQualityLimit creates a new BookStep with a quality limit.
// Offers with worse quality than the limit will not be consumed.
func NewBookStepWithQualityLimit(inIssue, outIssue Issue, strandSrc, strandDst [20]byte, prevStep Step, ownerPaysTransferFee bool, qualityLimit *Quality) *BookStep {
	step := NewBookStep(inIssue, outIssue, strandSrc, strandDst, prevStep, ownerPaysTransferFee)
	step.qualityLimit = qualityLimit
	return step
}

// Rev calculates the input needed to produce the requested output
// by consuming offers from the order book
func (s *BookStep) Rev(
	sb *PaymentSandbox,
	afView *PaymentSandbox,
	ofrsToRm map[[32]byte]bool,
	out EitherAmount,
) (EitherAmount, EitherAmount) {
	s.cache = nil
	s.offersUsed_ = 0

	// Get transfer rates
	// Default to DebtDirectionRedeems for first step - this applies transfer rate
	// when the source account is sending IOU through the issuer (typical case)
	// Reference: rippled BookStep.cpp - first step in strand typically redeems
	prevStepDebtDir := DebtDirectionRedeems
	if s.prevStep != nil {
		prevStepDebtDir = s.prevStep.DebtDirection(sb, StrandDirectionReverse)
	}

	trIn := s.transferRateIn(sb, prevStepDebtDir)
	trOut := s.transferRateOut(sb)

	// Initialize accumulators
	var totalIn, totalOut EitherAmount
	if s.book.In.IsXRP() {
		totalIn = ZeroXRPEitherAmount()
	} else {
		totalIn = ZeroIOUEitherAmount(s.book.In.Currency, sle.EncodeAccountIDSafe(s.book.In.Issuer))
	}
	if s.book.Out.IsXRP() {
		totalOut = ZeroXRPEitherAmount()
	} else {
		totalOut = ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
	}

	remainingOut := out

	// Track visited offers to avoid processing the same offer multiple times
	// This is separate from ofrsToRm which tracks offers to remove from ledger
	visited := make(map[[32]byte]bool)

	// Iterate through offers
	// fmt.Printf("DEBUG BookStep.Rev: starting, remainingOut=%+v\n", remainingOut)

	for s.offersUsed_ < s.maxOffersToConsume && !remainingOut.IsZero() {
		// Get next offer at best quality
		offer, offerKey, err := s.getNextOfferSkipVisited(sb, afView, ofrsToRm, visited)
		if err != nil {
			// fmt.Printf("DEBUG BookStep.Rev: getNextOffer error: %v\n", err)
			break
		}
		if offer == nil {
			// fmt.Printf("DEBUG BookStep.Rev: no more offers\n")
			break // No more offers
		}

		// fmt.Printf("DEBUG BookStep.Rev: found offer seq=%d, account=%s\n", offer.Sequence, offer.Account)

		// Mark as visited so we don't process it again in this execution
		visited[offerKey] = true

		// Check if offer is funded
		if !s.isOfferFunded(sb, offer) {
			// fmt.Printf("DEBUG BookStep.Rev: offer not funded, skipping\n")
			ofrsToRm[offerKey] = true
			s.offersUsed_++
			continue
		}
		// fmt.Printf("DEBUG BookStep.Rev: offer is funded\n")

		// Check quality limit - if offer quality is worse than limit, stop
		offerQuality := s.offerQuality(offer)
		// fmt.Printf("DEBUG BookStep.Rev: offerQuality=%v, qualityLimit=%v\n", offerQuality, s.qualityLimit)
		if s.qualityLimit != nil && offerQuality.WorseThan(*s.qualityLimit) {
			// fmt.Printf("DEBUG BookStep.Rev: offer quality %v worse than limit %v, stopping\n", offerQuality, s.qualityLimit)
			break
		}
		// fmt.Printf("DEBUG BookStep.Rev: quality check passed, continuing\n")

		// Calculate how much we can get from this offer
		// Use funded amount, which may be less than stated TakerGets
		offerTakerGetsStated := s.offerTakerGets(offer)
		offerOut := s.getOfferFundedAmount(sb, offer)
		offerIn := s.offerTakerPays(offer) // This is NET (what offer owner expects)
		// fmt.Printf("DEBUG BookStep.Rev: offerTakerGetsStated=%+v, offerOut(funded)=%+v, offerIn=%+v\n", offerTakerGetsStated, offerOut, offerIn)

		// Scale offerIn proportionally if funded amount is less than stated
		if offerOut.Compare(offerTakerGetsStated) < 0 && !offerTakerGetsStated.IsZero() {
			ratio := offerOut.DivideFloat(offerTakerGetsStated)
			offerIn = offerIn.MultiplyFloat(ratio)
		}

		// Convert offerIn (NET) to GROSS for proper accounting
		// GROSS = NET * trIn / QualityOne
		// Use roundUp=false to match rippled's behavior. In rippled, the multiplication
		// uses muldiv_round which divides by 10^14 with rounding, but subsequent
		// normalization (dividing by 10 repeatedly) truncates. The net effect is that
		// the rounding gets "washed away" for results that need normalization.
		grossOfferIn := offerIn
		if trIn != QualityOne && !s.book.In.IsXRP() {
			grossOfferIn = MulRatio(offerIn, trIn, QualityOne, false)
		}

		// Limit by what we still need
		var actualOut, actualIn, actualInNet EitherAmount
		if offerOut.Compare(remainingOut) <= 0 {
			// Take entire offer - use GROSS for proper transfer fee accounting
			actualOut = offerOut
			actualIn = grossOfferIn
			actualInNet = offerIn // Original offer's TakerPays (NET)
		} else {
			// Partial take - applyQuality returns GROSS
			actualOut = remainingOut
			actualIn = s.applyQuality(actualOut, offerQuality, trIn, trOut, true)

			// Ensure we don't exceed offer's GROSS input
			if actualIn.Compare(grossOfferIn) > 0 {
				actualIn = grossOfferIn
				actualOut = s.reverseQuality(actualIn, offerQuality, trIn, trOut, false)
			}

			// Calculate NET from GROSS for partial consumption
			actualInNet = actualIn
			if trIn != QualityOne && !actualIn.IsNative {
				actualInNet = MulRatio(actualIn, QualityOne, trIn, false)
			}
		}

		// Consume the offer - Rev DOES apply changes like rippled
		// Reference: rippled's BookStep::revImp() calls consumeOffer()
		if err := s.consumeOffer(sb, offer, actualIn, actualInNet, actualOut); err != nil {
			break
		}

		// Accumulate
		totalIn = totalIn.Add(actualIn)
		totalOut = totalOut.Add(actualOut)
		remainingOut = remainingOut.Sub(actualOut)
		s.offersUsed_++
		// fmt.Printf("DEBUG BookStep.Rev: actualIn=%+v, actualOut=%+v, totalIn=%+v, totalOut=%+v\n", actualIn, actualOut, totalIn, totalOut)
	}

	// Check if we should become inactive
	if s.offersUsed_ >= s.maxOffersToConsume {
		s.inactive_ = true
	}


	s.cache = &bookCache{
		in:  totalIn,
		out: totalOut,
	}

	// fmt.Printf("DEBUG BookStep.Rev: FINAL totalIn=%+v, totalOut=%+v\n", totalIn, totalOut)
	return totalIn, totalOut
}

// Fwd executes the step with the given input
func (s *BookStep) Fwd(
	sb *PaymentSandbox,
	afView *PaymentSandbox,
	ofrsToRm map[[32]byte]bool,
	in EitherAmount,
) (EitherAmount, EitherAmount) {

	// Clear cache from any previous execution to allow fresh computation
	prevCache := s.cache
	s.cache = nil
	s.offersUsed_ = 0
	_ = prevCache

	// Get transfer rates
	// Default to DebtDirectionRedeems for first step - this applies transfer rate
	// when the source account is sending IOU through the issuer (typical case)
	// Reference: rippled BookStep.cpp - first step in strand typically redeems
	prevStepDebtDir := DebtDirectionRedeems
	if s.prevStep != nil {
		prevStepDebtDir = s.prevStep.DebtDirection(sb, StrandDirectionForward)
	}

	trIn := s.transferRateIn(sb, prevStepDebtDir)
	trOut := s.transferRateOut(sb)


	// Initialize accumulators
	var totalIn, totalOut EitherAmount
	if s.book.In.IsXRP() {
		totalIn = ZeroXRPEitherAmount()
	} else {
		totalIn = ZeroIOUEitherAmount(s.book.In.Currency, sle.EncodeAccountIDSafe(s.book.In.Issuer))
	}
	if s.book.Out.IsXRP() {
		totalOut = ZeroXRPEitherAmount()
	} else {
		totalOut = ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
	}

	remainingIn := in

	// Track visited offers to avoid processing the same offer multiple times
	visited := make(map[[32]byte]bool)

	// Iterate through offers
	for s.offersUsed_ < s.maxOffersToConsume && !remainingIn.IsZero() {
		offer, offerKey, err := s.getNextOfferSkipVisited(sb, afView, ofrsToRm, visited)
		if err != nil {
			break
		}
		if offer == nil {
			break
		}

		// Mark as visited so we don't process it again in this execution
		visited[offerKey] = true

		if !s.isOfferFunded(sb, offer) {
			s.offersUsed_++
			continue
		}

		// Check quality limit - if offer quality is worse than limit, stop
		offerQuality := s.offerQuality(offer)
		if s.qualityLimit != nil && offerQuality.WorseThan(*s.qualityLimit) {
			break
		}

		// Get funded output amount (may be less than stated TakerGets)
		offerTakerGetsStated := s.offerTakerGets(offer)
		fundedOut := s.getOfferFundedAmount(sb, offer)
		offerIn := s.offerTakerPays(offer)

		// Scale offerIn proportionally if funded amount is less than stated
		if fundedOut.Compare(offerTakerGetsStated) < 0 && !offerTakerGetsStated.IsZero() {
			ratio := fundedOut.DivideFloat(offerTakerGetsStated)
			offerIn = offerIn.MultiplyFloat(ratio)
		}


		// Calculate how much we can use from this offer
		// offerIn is the NET amount (what offer owner expects to receive)
		// remainingIn is GROSS (what taker has left to spend including transfer fees)
		//
		// To compare properly, convert offerIn (NET) to GROSS for comparison:
		// grossOfferIn = offerIn * trIn / QualityOne
		// Use roundUp=false to match rippled's behavior (see Rev() comment for details).
		grossOfferIn := offerIn
		if trIn != QualityOne && !s.book.In.IsXRP() {
			grossOfferIn = MulRatio(offerIn, trIn, QualityOne, false)
		}

		var actualIn, actualInNet, actualOut EitherAmount
		var isFullConsumption bool
		if grossOfferIn.Compare(remainingIn) <= 0 {
			// Full consumption - can use entire offer (grossOfferIn <= remainingIn)
			actualIn = grossOfferIn
			actualInNet = offerIn // Use the original offer's TakerPays (NET) directly to avoid rounding errors
			isFullConsumption = true
		} else {
			// Partial use of offer - grossOfferIn > remainingIn
			actualIn = remainingIn
			// Calculate NET from GROSS for partial consumption
			actualInNet = actualIn
			if trIn != QualityOne && !actualIn.IsNative {
				actualInNet = MulRatio(actualIn, QualityOne, trIn, false)
			}
			isFullConsumption = false
		}

		// fmt.Printf("DEBUG BookStep.Fwd: offer TakerGets=%+v, TakerPays=%+v, fundedOut=%+v\n", offerTakerGetsStated, offerIn, fundedOut)
		// fmt.Printf("DEBUG BookStep.Fwd: offerIn=%+v, grossOfferIn=%+v, remainingIn=%+v, actualIn=%+v, actualInNet=%+v, trIn=%d, trOut=%d\n", offerIn, grossOfferIn, remainingIn, actualIn, actualInNet, trIn, trOut)

		// Calculate output:
		// actualIn is always GROSS (what taker pays including transfer fee)
		if isFullConsumption {
			// Full consumption: get the full funded output
			actualOut = fundedOut
			// fmt.Printf("DEBUG BookStep.Fwd: FULL consumption, actualOut=%+v\n", actualOut)
		} else {
			// Partial consumption: compute output from GROSS input
			actualOut = s.computeOutputFromInputWithTransferRate(actualIn, offerIn, fundedOut, trIn)
			// fmt.Printf("DEBUG BookStep.Fwd: PARTIAL consumption, actualOut=%+v\n", actualOut)
		}

		// Limit output to funded amount
		if actualOut.Compare(fundedOut) > 0 {
			// // fmt.Printf("DEBUG BookStep.Fwd: limiting to fundedOut=%+v\n", fundedOut)
			actualOut = fundedOut
			actualIn = s.applyQuality(actualOut, offerQuality, trIn, trOut, true)
			// Recalculate NET from GROSS when limited
			actualInNet = actualIn
			if trIn != QualityOne && !actualIn.IsNative {
				actualInNet = MulRatio(actualIn, QualityOne, trIn, false)
			}
		}

		// Limit output to cached value to prevent forward > reverse
		if s.cache != nil {
			remainingCacheOut := s.cache.out.Sub(totalOut)
			// // fmt.Printf("DEBUG BookStep.Fwd: cache check, cacheOut=%+v, totalOut=%+v, remaining=%+v\n", s.cache.out, totalOut, remainingCacheOut)
			if actualOut.Compare(remainingCacheOut) > 0 {
				actualOut = remainingCacheOut
				actualIn = s.applyQuality(actualOut, offerQuality, trIn, trOut, true)
				// Recalculate NET from GROSS when limited
				actualInNet = actualIn
				if trIn != QualityOne && !actualIn.IsNative {
					actualInNet = MulRatio(actualIn, QualityOne, trIn, false)
				}
			}
		}

		// fmt.Printf("DEBUG BookStep.Fwd: FINAL actualIn=%+v, actualInNet=%+v, actualOut=%+v (adding to total)\n", actualIn, actualInNet, actualOut)

		err = s.consumeOffer(sb, offer, actualIn, actualInNet, actualOut)
		if err != nil {
			break
		}

		totalIn = totalIn.Add(actualIn)
		totalOut = totalOut.Add(actualOut)
		remainingIn = remainingIn.Sub(actualIn)
		s.offersUsed_++

	}

	if s.offersUsed_ >= s.maxOffersToConsume {
		s.inactive_ = true
	}


	s.cache = &bookCache{
		in:  totalIn,
		out: totalOut,
	}

	return totalIn, totalOut
}

// CachedIn returns the input from the last Rev() call
func (s *BookStep) CachedIn() *EitherAmount {
	if s.cache == nil {
		return nil
	}
	return &s.cache.in
}

// CachedOut returns the output from the last Rev() call
func (s *BookStep) CachedOut() *EitherAmount {
	if s.cache == nil {
		return nil
	}
	return &s.cache.out
}

// DebtDirection returns the debt direction based on who pays transfer fee
func (s *BookStep) DebtDirection(sb *PaymentSandbox, dir StrandDirection) DebtDirection {
	if s.ownerPaysTransferFee {
		return DebtDirectionIssues
	}
	return DebtDirectionRedeems
}

// QualityUpperBound returns the worst-case quality for this step
func (s *BookStep) QualityUpperBound(v *PaymentSandbox, prevStepDir DebtDirection) (*Quality, DebtDirection) {
	// Get the tip of the order book
	tipQuality := s.getTipQuality(v)
	if tipQuality == nil {
		return nil, s.DebtDirection(v, StrandDirectionForward)
	}
	return tipQuality, s.DebtDirection(v, StrandDirectionForward)
}

// IsZero returns true if the amount is zero
func (s *BookStep) IsZero(amt EitherAmount) bool {
	return amt.IsZero()
}

// EqualIn compares input amounts
func (s *BookStep) EqualIn(a, b EitherAmount) bool {
	return a.Compare(b) == 0
}

// EqualOut compares output amounts
func (s *BookStep) EqualOut(a, b EitherAmount) bool {
	return a.Compare(b) == 0
}

// Inactive returns whether this step is inactive
func (s *BookStep) Inactive() bool {
	return s.inactive_
}

// OffersUsed returns the number of offers consumed
func (s *BookStep) OffersUsed() uint32 {
	return s.offersUsed_
}

// DirectStepAccts returns nil - this is not a direct step
func (s *BookStep) DirectStepAccts() *[2][20]byte {
	return nil
}

// BookStepBook returns the book for this step
func (s *BookStep) BookStepBook() *Book {
	return &s.book
}

// LineQualityIn returns QualityOne for book steps
func (s *BookStep) LineQualityIn(v *PaymentSandbox) uint32 {
	return QualityOne
}

// ValidFwd validates that the step can correctly execute in forward
func (s *BookStep) ValidFwd(sb *PaymentSandbox, afView *PaymentSandbox, in EitherAmount) (bool, EitherAmount) {
	if s.cache == nil {
		return false, ZeroXRPEitherAmount()
	}
	return true, s.cache.out
}

// transferRateIn returns the transfer rate for incoming currency
func (s *BookStep) transferRateIn(sb *PaymentSandbox, prevStepDir DebtDirection) uint32 {
	if s.book.In.IsXRP() {
		return QualityOne
	}

	// Only charge transfer fee when previous step redeems
	if !Redeems(prevStepDir) {
		return QualityOne
	}

	return s.GetAccountTransferRate(sb, s.book.In.Issuer)
}

// transferRateOut returns the transfer rate for outgoing currency
func (s *BookStep) transferRateOut(sb *PaymentSandbox) uint32 {
	if s.book.Out.IsXRP() {
		return QualityOne
	}

	if !s.ownerPaysTransferFee {
		return QualityOne
	}

	return s.GetAccountTransferRate(sb, s.book.Out.Issuer)
}

// GetAccountTransferRate gets the transfer rate from an account
func (s *BookStep) GetAccountTransferRate(sb *PaymentSandbox, issuer [20]byte) uint32 {
	accountKey := keylet.Account(issuer)
	data, err := sb.Read(accountKey)
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

// getNextOfferSkipVisited returns the next offer at the best quality, skipping offers in ofrsToRm and visited
func (s *BookStep) getNextOfferSkipVisited(sb *PaymentSandbox, afView *PaymentSandbox, ofrsToRm map[[32]byte]bool, visited map[[32]byte]bool) (*sle.LedgerOffer, [32]byte, error) {
	// Get the order book directory base key
	takerPaysCurrency := sle.GetCurrencyBytes(s.book.In.Currency)
	takerPaysIssuer := s.book.In.Issuer
	takerGetsCurrency := sle.GetCurrencyBytes(s.book.Out.Currency)
	takerGetsIssuer := s.book.Out.Issuer
	bookBase := keylet.BookDir(takerPaysCurrency, takerPaysIssuer, takerGetsCurrency, takerGetsIssuer)

	bookPrefix := bookBase.Key[:24]

	type dirEntry struct {
		key  [32]byte
		data []byte
	}
	var dirs []dirEntry

	err := sb.ForEach(func(key [32]byte, data []byte) bool {
		if bytes.Equal(key[:24], bookPrefix) {
			dirs = append(dirs, dirEntry{key: key, data: data})
		}
		return true
	})
	if err != nil {
		return nil, [32]byte{}, err
	}

	sort.Slice(dirs, func(i, j int) bool {
		return bytes.Compare(dirs[i].key[24:], dirs[j].key[24:]) < 0
	})

	for _, d := range dirs {
		dir, err := sle.ParseDirectoryNode(d.data)
		if err != nil || len(dir.Indexes) == 0 {
			continue
		}

		for _, idx := range dir.Indexes {
			var offerKey [32]byte
			copy(offerKey[:], idx[:])

			// Skip offers already in ofrsToRm or visited
			if ofrsToRm != nil && ofrsToRm[offerKey] {
				continue
			}
			if visited != nil && visited[offerKey] {
				continue
			}

			offerData, err := sb.Read(keylet.Keylet{Key: offerKey})
			if err != nil || offerData == nil {
				continue
			}

			offer, err := sle.ParseLedgerOffer(offerData)
			if err != nil {
				continue
			}

			return offer, offerKey, nil
		}
	}

	return nil, [32]byte{}, nil
}

// getNextOffer returns the next offer at the best quality, skipping offers in ofrsToRm
func (s *BookStep) getNextOffer(sb *PaymentSandbox, afView *PaymentSandbox, ofrsToRm map[[32]byte]bool) (*sle.LedgerOffer, [32]byte, error) {
	// Get the order book directory base key
	takerPaysCurrency := sle.GetCurrencyBytes(s.book.In.Currency)
	takerPaysIssuer := s.book.In.Issuer
	takerGetsCurrency := sle.GetCurrencyBytes(s.book.Out.Currency)
	takerGetsIssuer := s.book.Out.Issuer
	bookBase := keylet.BookDir(takerPaysCurrency, takerPaysIssuer, takerGetsCurrency, takerGetsIssuer)

	// fmt.Printf("DEBUG getNextOffer: book.In.Currency=%s, book.In.Issuer=%x, book.Out.Currency=%s, book.Out.Issuer=%x\n",
	// 	s.book.In.Currency, s.book.In.Issuer[:8], s.book.Out.Currency, s.book.Out.Issuer[:8])
	// fmt.Printf("DEBUG getNextOffer: bookBase.Key=%x\n", bookBase.Key)

	bookPrefix := bookBase.Key[:24]

	type dirEntry struct {
		key  [32]byte
		data []byte
	}
	var dirs []dirEntry
	entryCount := 0

	err := sb.ForEach(func(key [32]byte, data []byte) bool {
		entryCount++
		if bytes.Equal(key[:24], bookPrefix) {
			dirs = append(dirs, dirEntry{key: key, data: data})
		}
		return true
	})
	if err != nil {
		return nil, [32]byte{}, err
	}

	// fmt.Printf("DEBUG getNextOffer: scanned %d entries, found %d matching dirs\n", entryCount, len(dirs))

	sort.Slice(dirs, func(i, j int) bool {
		return bytes.Compare(dirs[i].key[24:], dirs[j].key[24:]) < 0
	})

	for _, d := range dirs {
		dir, err := sle.ParseDirectoryNode(d.data)
		if err != nil || len(dir.Indexes) == 0 {
			// fmt.Printf("DEBUG getNextOffer: dir parse error or empty, err=%v, indexes=%d\n", err, len(dir.Indexes))
			continue
		}

		// fmt.Printf("DEBUG getNextOffer: dir has %d indexes\n", len(dir.Indexes))

		for _, idx := range dir.Indexes {
			var offerKey [32]byte
			copy(offerKey[:], idx[:])

			// fmt.Printf("DEBUG getNextOffer: checking offer key=%x\n", offerKey[:8])

			if ofrsToRm != nil && ofrsToRm[offerKey] {
				// fmt.Printf("DEBUG getNextOffer: skipping offer in ofrsToRm\n")
				continue
			}

			offerData, err := sb.Read(keylet.Keylet{Key: offerKey})
			if err != nil || offerData == nil {
				// fmt.Printf("DEBUG getNextOffer: offer not found, err=%v\n", err)
				continue
			}

			offer, err := sle.ParseLedgerOffer(offerData)
			if err != nil {
				// fmt.Printf("DEBUG getNextOffer: offer parse error: %v\n", err)
				continue
			}
			// fmt.Printf("DEBUG getNextOffer: found offer seq=%d, account=%s\n", offer.Sequence, offer.Account)

			return offer, offerKey, nil
		}
	}

	return nil, [32]byte{}, nil
}

// isOfferFunded checks if an offer has sufficient funding
func (s *BookStep) isOfferFunded(sb *PaymentSandbox, offer *sle.LedgerOffer) bool {
	if offer == nil {
		return false
	}
	if offer.TakerGets.IsZero() {
		return false
	}
	funded := s.getOfferFundedAmount(sb, offer)
	return !funded.IsEffectivelyZero()
}

// getOfferFundedAmount returns the actual amount an offer can deliver based on owner's balance.
// This matches rippled's calculation of funded amounts for offers.
// Reference: rippled OfferStream.cpp uses accountFundsHelper which calls accountHolds.
func (s *BookStep) getOfferFundedAmount(sb *PaymentSandbox, offer *sle.LedgerOffer) EitherAmount {
	offerOwner, err := sle.DecodeAccountID(offer.Account)
	if err != nil {
		return ZeroXRPEitherAmount()
	}

	offerTakerGets := s.offerTakerGets(offer)

	if s.book.Out.IsXRP() {
		accountKey := keylet.Account(offerOwner)
		accountData, err := sb.Read(accountKey)
		if err != nil || accountData == nil {
			return ZeroXRPEitherAmount()
		}

		account, err := sle.ParseAccountRoot(accountData)
		if err != nil {
			return ZeroXRPEitherAmount()
		}

		// Use OwnerCountHook to get adjusted owner count (accounts for pending changes)
		// Reference: rippled View.cpp xrpLiquid() line 627-628
		ownerCount := sb.OwnerCountHook(offerOwner, account.OwnerCount)

		// Read reserve values from ledger's FeeSettings
		// Reference: rippled View.cpp xrpLiquid() reads reserves from fees keylet
		baseReserve, incrementReserve := GetLedgerReserves(sb)
		reserve := baseReserve + int64(ownerCount)*incrementReserve

		// Use BalanceHook to get adjusted balance (accounts for pending credits)
		// Reference: rippled View.cpp xrpLiquid() line 637
		// For XRP, issuer is the zero account (xrpAccount)
		xrpIssuer := [20]byte{}
		xrpAmount := tx.NewXRPAmount(int64(account.Balance))
		adjustedBalance := sb.BalanceHook(offerOwner, xrpIssuer, xrpAmount)
		available := adjustedBalance.Drops() - reserve

		if available <= 0 {
			return ZeroXRPEitherAmount()
		}

		if available < offerTakerGets.XRP {
			return NewXRPEitherAmount(available)
		}
		return offerTakerGets
	}

	// For IOU TakerGets: check owner's trustline balance with issuer
	issuer := s.book.Out.Issuer
	currency := s.book.Out.Currency

	ownerBalance := s.getIOUBalance(sb, offerOwner, issuer, currency)

	if ownerBalance.IsNegative() || ownerBalance.IsZero() {
		if offerOwner == issuer {
			return offerTakerGets
		}
		return ZeroIOUEitherAmount(currency, sle.EncodeAccountIDSafe(issuer))
	}

	if ownerBalance.Compare(offerTakerGets) < 0 {
		return ownerBalance
	}
	return offerTakerGets
}

// getIOUBalance returns an account's IOU balance with an issuer
func (s *BookStep) getIOUBalance(sb *PaymentSandbox, account, issuer [20]byte, currency string) EitherAmount {
	issuerStr := sle.EncodeAccountIDSafe(issuer)

	if account == issuer {
		// Issuer has unlimited balance for their own currency
		return NewIOUEitherAmount(tx.NewIssuedAmount(1000000000000000, 15, currency, issuerStr))
	}

	lineKey := keylet.Line(account, issuer, currency)
	lineData, err := sb.Read(lineKey)
	if err != nil || lineData == nil {
		return ZeroIOUEitherAmount(currency, issuerStr)
	}

	rs, err := sle.ParseRippleState(lineData)
	if err != nil {
		return ZeroIOUEitherAmount(currency, issuerStr)
	}

	// Balance is stored from the low account's perspective
	accountIsLow := sle.CompareAccountIDsForLine(account, issuer) < 0

	var balance tx.Amount
	if accountIsLow {
		balance = rs.Balance
	} else {
		balance = rs.Balance.Negate()
	}

	// Create new Amount with correct issuer
	return NewIOUEitherAmount(sle.NewIssuedAmountFromValue(balance.IOU().Mantissa(), balance.IOU().Exponent(), currency, issuerStr))
}

// offerTakerGets returns what the taker gets from this offer
func (s *BookStep) offerTakerGets(offer *sle.LedgerOffer) EitherAmount {
	if s.book.Out.IsXRP() {
		return NewXRPEitherAmount(offer.TakerGets.Drops())
	}
	return NewIOUEitherAmount(offer.TakerGets)
}

// offerTakerPays returns what the taker pays to this offer
func (s *BookStep) offerTakerPays(offer *sle.LedgerOffer) EitherAmount {
	if s.book.In.IsXRP() {
		return NewXRPEitherAmount(offer.TakerPays.Drops())
	}
	return NewIOUEitherAmount(offer.TakerPays)
}

// offerQuality returns the quality of an offer by extracting it from the BookDirectory key.
// The quality is stored in the last 8 bytes of the BookDirectory, encoded as big-endian uint64.
// This is the original quality set when the offer was created, which remains constant
// even as the offer is partially filled.
// Reference: rippled's getQuality() in Indexes.cpp
func (s *BookStep) offerQuality(offer *sle.LedgerOffer) Quality {
	// Compute quality from actual TakerPays/TakerGets for precision.
	// The BookDirectory quality is a "price tier" for ordering, but for
	// accurate calculations we need the exact ratio from the offer amounts.
	// Reference: rippled calculates quality as in/out for flow calculations
	takerPays := s.offerTakerPays(offer)
	takerGets := s.offerTakerGets(offer)
	return QualityFromAmounts(takerPays, takerGets)
}

// applyQuality applies quality and transfer rates to convert output to input
// input = output * quality_rate * trIn / trOut (for reverse: given output, find input)
// Quality rate = in/out, so: new_in = out * (in/out) = in
// Reference: rippled's Quality::ceil_out_impl which does: result.in = MulRound(limit, quality.rate(), ...)
func (s *BookStep) applyQuality(out EitherAmount, q Quality, trIn, trOut uint32, roundUp bool) EitherAmount {
	// Use precise Amount arithmetic instead of float64
	// Reference: rippled uses mulRound(limit, quality.rate(), asset, roundUp)

	// Convert output to Amount for precise multiplication
	var outAmt tx.Amount
	if out.IsNative {
		outAmt = tx.NewXRPAmount(out.XRP)
	} else {
		outAmt = out.IOU
	}

	// Multiply by quality rate using precise arithmetic
	// result = out * quality.rate()
	qRate := q.Rate()
	result := outAmt.Mul(qRate, roundUp)

	// Apply transfer rate: result = result * trIn / trOut
	if trIn != trOut && trOut != 0 {
		result = result.MulRatio(trIn, trOut, roundUp)
	}

	if s.book.In.IsXRP() {
		// Convert mantissa/exponent to drops with proper rounding
		// Reference: rippled's canonicalize for XRP
		drops := result.Mantissa()
		exp := result.Exponent()
		for exp > 0 {
			drops *= 10
			exp--
		}
		// When dividing (negative exponent), apply rounding
		if exp < 0 {
			for exp < -1 {
				drops /= 10
				exp++
			}
			// Last division with rounding
			if roundUp {
				drops = (drops + 9) / 10 // Round up
			} else {
				drops /= 10 // Round down (truncate)
			}
		}
		return NewXRPEitherAmount(drops)
	}

	// For IOU, ensure correct currency/issuer
	return NewIOUEitherAmount(tx.NewIssuedAmount(
		result.Mantissa(), result.Exponent(),
		s.book.In.Currency, sle.EncodeAccountIDSafe(s.book.In.Issuer)))
}

// reverseQuality applies reverse quality to convert input to output
// output = (input / transferRate) / quality_rate = NET_input / quality_rate
// Quality rate = in/out, so: new_out = NET_in / (in/out) = out
// Reference: rippled's Quality::ceil_in_impl which does: result.out = DivRound(limit, quality.rate(), ...)
func (s *BookStep) reverseQuality(in EitherAmount, q Quality, trIn, trOut uint32, roundUp bool) EitherAmount {
	// Use precise Amount arithmetic instead of float64
	// Reference: rippled uses divRound(limit, quality.rate(), asset, roundUp)

	qRate := q.Rate()
	if qRate.IsZero() {
		if s.book.Out.IsXRP() {
			return ZeroXRPEitherAmount()
		}
		return ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
	}

	// Convert input to Amount for precise division
	var inAmt tx.Amount
	if in.IsNative {
		inAmt = tx.NewXRPAmount(in.XRP)
	} else {
		inAmt = in.IOU
	}

	// Apply transfer rate to convert GROSS input to NET input
	// NET = GROSS * trOut / trIn (since trIn > QualityOne for fees)
	// This accounts for the transfer fee: offer owner receives less than taker sends
	// Round DOWN (not up) to be conservative - less NET means less output
	if trIn != trOut && trIn != 0 && !in.IsNative {
		inAmt = inAmt.MulRatio(trOut, trIn, roundUp)
	}

	// Divide by quality rate using precise arithmetic
	// result = NET_in / quality.rate()
	// The quality rate is in/out, so: out = NET_in / (in/out) = out
	result := inAmt.Div(qRate, roundUp)

	if s.book.Out.IsXRP() {
		// Convert mantissa/exponent to drops with proper rounding
		// Reference: rippled's canonicalize for XRP
		drops := result.Mantissa()
		exp := result.Exponent()
		for exp > 0 {
			drops *= 10
			exp--
		}
		// When dividing (negative exponent), apply rounding
		if exp < 0 {
			for exp < -1 {
				drops /= 10
				exp++
			}
			// Last division with rounding
			if roundUp {
				drops = (drops + 9) / 10 // Round up
			} else {
				drops /= 10 // Round down (truncate)
			}
		}
		return NewXRPEitherAmount(drops)
	}

	// For IOU, ensure correct currency/issuer
	return NewIOUEitherAmount(tx.NewIssuedAmount(
		result.Mantissa(), result.Exponent(),
		s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer)))
}

// computeOutputFromInputWithTransferRate calculates output for PARTIAL offer consumption.
// The input is GROSS (what taker pays including transfer fee).
// Steps:
// 1. Convert GROSS to NET: netIn = grossIn × QualityOne / trIn
// 2. Compute output: output = netIn × offerGets / offerPays
// Reference: rippled's limitStepIn with offer.limitIn using quality
func (s *BookStep) computeOutputFromInputWithTransferRate(input, offerPays, offerGets EitherAmount, trIn uint32) EitherAmount {
	// Convert input to Amount
	var inputAmt tx.Amount
	if input.IsNative {
		inputAmt = tx.NewXRPAmount(input.XRP)
	} else {
		inputAmt = input.IOU
	}

	// Convert offer amounts to Amount
	var paysAmt, getsAmt tx.Amount
	if offerPays.IsNative {
		paysAmt = tx.NewXRPAmount(offerPays.XRP)
	} else {
		paysAmt = offerPays.IOU
	}
	if offerGets.IsNative {
		getsAmt = tx.NewXRPAmount(offerGets.XRP)
	} else {
		getsAmt = offerGets.IOU
	}

	if paysAmt.IsZero() {
		if s.book.Out.IsXRP() {
			return ZeroXRPEitherAmount()
		}
		return ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
	}

	// For XRP output with IOU input: use big.Int for maximum precision
	if s.book.Out.IsXRP() && !inputAmt.IsNative() && offerGets.IsNative && !offerPays.IsNative {
		inputMant := big.NewInt(inputAmt.Mantissa())
		inputExp := inputAmt.Exponent()
		getsDrops := big.NewInt(offerGets.XRP)
		paysMant := big.NewInt(offerPays.IOU.Mantissa())
		paysExp := offerPays.IOU.Exponent()

		// numerator = inputMant × QualityOne × getsDrops
		// (we'll divide by trIn later, along with paysMant)
		numerator := new(big.Int).Set(inputMant)
		numerator.Mul(numerator, big.NewInt(int64(QualityOne)))
		numerator.Mul(numerator, getsDrops)

		// denominator = trIn × paysMant
		denominator := new(big.Int).SetInt64(int64(trIn))
		denominator.Mul(denominator, paysMant)

		// Apply exponent difference
		expDiff := inputExp - paysExp
		if expDiff > 0 {
			multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(expDiff)), nil)
			numerator.Mul(numerator, multiplier)
		} else if expDiff < 0 {
			multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-expDiff)), nil)
			denominator.Mul(denominator, multiplier)
		}

		// Round UP: (numerator + denominator - 1) / denominator
		numerator.Add(numerator, denominator)
		numerator.Sub(numerator, big.NewInt(1))
		result := new(big.Int).Div(numerator, denominator)

		return NewXRPEitherAmount(result.Int64())
	}

	// For other cases: convert GROSS to NET, then use Amount arithmetic
	// netIn = grossIn × QualityOne / trIn
	netInputAmt := inputAmt.MulRatio(QualityOne, trIn, false) // round down for NET

	// output = netIn × offerGets / offerPays (round up)
	temp := netInputAmt.Mul(getsAmt, true)
	result := temp.Div(paysAmt, true)

	if s.book.Out.IsXRP() {
		drops := result.Mantissa()
		exp := result.Exponent()
		for exp > 0 {
			drops *= 10
			exp--
		}
		if exp < 0 {
			for exp < -1 {
				drops /= 10
				exp++
			}
			drops = (drops + 9) / 10 // Round up
		}
		return NewXRPEitherAmount(drops)
	}

	return NewIOUEitherAmount(tx.NewIssuedAmount(
		result.Mantissa(), result.Exponent(),
		s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer)))
}

// computeOutputFromInputNoTransferRate calculates output from input using just offer quality.
// output = input * offerGets / offerPays
// Used for partial offer consumption where input is NET (what offer owner receives).
// Reference: rippled's Quality::ceil_in uses roundUp=true for output calculation.
func (s *BookStep) computeOutputFromInputNoTransferRate(input, offerPays, offerGets EitherAmount) EitherAmount {
	// Convert input to Amount
	var inputAmt tx.Amount
	if input.IsNative {
		inputAmt = tx.NewXRPAmount(input.XRP)
	} else {
		inputAmt = input.IOU
	}

	// Convert offer amounts to Amount
	var paysAmt, getsAmt tx.Amount
	if offerPays.IsNative {
		paysAmt = tx.NewXRPAmount(offerPays.XRP)
	} else {
		paysAmt = offerPays.IOU
	}
	if offerGets.IsNative {
		getsAmt = tx.NewXRPAmount(offerGets.XRP)
	} else {
		getsAmt = offerGets.IOU
	}

	if paysAmt.IsZero() {
		if s.book.Out.IsXRP() {
			return ZeroXRPEitherAmount()
		}
		return ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
	}

	// For XRP output with IOU input: use big.Int for maximum precision
	if s.book.Out.IsXRP() && !inputAmt.IsNative() && offerGets.IsNative && !offerPays.IsNative {
		inputMant := big.NewInt(inputAmt.Mantissa())
		inputExp := inputAmt.Exponent()
		getsDrops := big.NewInt(offerGets.XRP)
		paysMant := big.NewInt(offerPays.IOU.Mantissa())
		paysExp := offerPays.IOU.Exponent()

		// numerator = inputMant × getsDrops
		numerator := new(big.Int).Set(inputMant)
		numerator.Mul(numerator, getsDrops)

		// denominator = paysMant
		denominator := new(big.Int).Set(paysMant)

		// Apply exponent difference
		expDiff := inputExp - paysExp
		if expDiff > 0 {
			multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(expDiff)), nil)
			numerator.Mul(numerator, multiplier)
		} else if expDiff < 0 {
			multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-expDiff)), nil)
			denominator.Mul(denominator, multiplier)
		}

		// Round UP: (numerator + denominator - 1) / denominator
		numerator.Add(numerator, denominator)
		numerator.Sub(numerator, big.NewInt(1))
		result := new(big.Int).Div(numerator, denominator)

		return NewXRPEitherAmount(result.Int64())
	}

	// For other cases: use Amount arithmetic with roundUp=true
	temp := inputAmt.Mul(getsAmt, true)
	result := temp.Div(paysAmt, true)

	if s.book.Out.IsXRP() {
		drops := result.Mantissa()
		exp := result.Exponent()
		for exp > 0 {
			drops *= 10
			exp--
		}
		if exp < 0 {
			for exp < -1 {
				drops /= 10
				exp++
			}
			drops = (drops + 9) / 10 // Round up
		}
		return NewXRPEitherAmount(drops)
	}

	return NewIOUEitherAmount(tx.NewIssuedAmount(
		result.Mantissa(), result.Exponent(),
		s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer)))
}

// computeOutputFromInput calculates output amount directly from input using offer amounts.
// This avoids precision loss from quality encoding/decoding.
// output = (input / transferRate) * (offerGets / offerPays)
// Uses big.Int arithmetic with a single division at the end for maximum precision.
func (s *BookStep) computeOutputFromInput(input, offerPays, offerGets EitherAmount, trIn, trOut uint32) EitherAmount {
	// Convert input to Amount
	var inputAmt tx.Amount
	if input.IsNative {
		inputAmt = tx.NewXRPAmount(input.XRP)
	} else {
		inputAmt = input.IOU
	}

	// Convert offer amounts to Amount
	var paysAmt, getsAmt tx.Amount
	if offerPays.IsNative {
		paysAmt = tx.NewXRPAmount(offerPays.XRP)
	} else {
		paysAmt = offerPays.IOU
	}
	if offerGets.IsNative {
		getsAmt = tx.NewXRPAmount(offerGets.XRP)
	} else {
		getsAmt = offerGets.IOU
	}

	if paysAmt.IsZero() {
		if s.book.Out.IsXRP() {
			return ZeroXRPEitherAmount()
		}
		return ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
	}

	// output = (input * trOut / trIn) * offerGets / offerPays
	// For XRP output with IOU input: use big.Int with single division for maximum precision
	if s.book.Out.IsXRP() && !inputAmt.IsNative() && offerGets.IsNative && !offerPays.IsNative {
		// netInput is IOU (mantissa × 10^exp)
		// offerGets is XRP drops
		// offerPays is IOU (mantissa × 10^exp)
		//
		// output = (inputMant × 10^inputExp × trOut / trIn) × getsDrops / (paysMant × 10^paysExp)
		//
		// To minimize precision loss, we combine into one fraction:
		// numerator = inputMant × trOut × getsDrops × 10^(inputExp - paysExp) [if exp diff > 0]
		// denominator = paysMant × trIn × 10^(paysExp - inputExp) [if exp diff < 0]
		// Then perform ONE division at the end.

		inputMant := big.NewInt(inputAmt.Mantissa())
		inputExp := inputAmt.Exponent()
		getsDrops := big.NewInt(offerGets.XRP)
		paysMant := big.NewInt(offerPays.IOU.Mantissa())
		paysExp := offerPays.IOU.Exponent()

		// Build numerator: inputMant × trOut × getsDrops
		numerator := new(big.Int).Set(inputMant)
		if trIn != trOut && trIn != 0 {
			numerator.Mul(numerator, big.NewInt(int64(trOut)))
		}
		numerator.Mul(numerator, getsDrops)

		// Build denominator: paysMant × trIn
		denominator := new(big.Int).Set(paysMant)
		if trIn != trOut && trIn != 0 {
			denominator.Mul(denominator, big.NewInt(int64(trIn)))
		}

		// Apply exponent difference to either numerator or denominator
		// to avoid intermediate truncation
		expDiff := inputExp - paysExp
		if expDiff > 0 {
			// Multiply numerator by 10^expDiff
			multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(expDiff)), nil)
			numerator.Mul(numerator, multiplier)
		} else if expDiff < 0 {
			// Multiply denominator by 10^|expDiff|
			multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-expDiff)), nil)
			denominator.Mul(denominator, multiplier)
		}

		// Single division at the end with rounding UP
		// Reference: rippled's Quality::ceil_in uses roundUp=true for output
		// Round up: (numerator + denominator - 1) / denominator
		numerator.Add(numerator, denominator)
		numerator.Sub(numerator, big.NewInt(1))
		result := new(big.Int).Div(numerator, denominator)

		return NewXRPEitherAmount(result.Int64())
	}

	// Apply transfer rate for non-XRP output cases
	if trIn != trOut && trIn != 0 && !input.IsNative {
		inputAmt = inputAmt.MulRatio(trOut, trIn, false) // round down
	}

	// For other cases: use Amount arithmetic
	// Reference: rippled's Quality::ceil_in uses roundUp=true
	temp := inputAmt.Mul(getsAmt, true)
	result := temp.Div(paysAmt, true)

	if s.book.Out.IsXRP() {
		drops := result.Mantissa()
		exp := result.Exponent()
		for exp > 0 {
			drops *= 10
			exp--
		}
		// When converting with negative exponent, round UP
		if exp < 0 {
			for exp < -1 {
				drops /= 10
				exp++
			}
			// Last division with rounding up
			drops = (drops + 9) / 10
		}
		return NewXRPEitherAmount(drops)
	}

	// For IOU output
	return NewIOUEitherAmount(tx.NewIssuedAmount(
		result.Mantissa(), result.Exponent(),
		s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer)))
}

// consumeOffer reduces the offer's amounts by the consumed amounts and transfers funds.
// consumedInGross is the GROSS amount (what taker pays, includes transfer fee)
// consumedInNet is the NET amount (what offer owner receives, after transfer fee)
// consumedOut is the amount the taker receives (offer's TakerGets)
// Note: We pass both GROSS and NET to avoid rounding errors from recalculating NET from GROSS.
func (s *BookStep) consumeOffer(sb *PaymentSandbox, offer *sle.LedgerOffer, consumedInGross, consumedInNet, consumedOut EitherAmount) error {
	// // fmt.Printf("DEBUG consumeOffer: offer seq=%d, consumedInGross=%+v, consumedInNet=%+v, consumedOut=%+v\n", offer.Sequence, consumedInGross, consumedInNet, consumedOut)
	offerOwner, err := sle.DecodeAccountID(offer.Account)
	if err != nil {
		return err
	}

	txHash, ledgerSeq := sb.GetTransactionContext()

	grossIn := consumedInGross
	netIn := consumedInNet

	// 1. Transfer input currency with transfer fee:
	//    - Taker (strandSrc) is debited GROSS amount (grossIn)
	//    - Offer owner is credited NET amount (netIn)
	if err := s.transferFundsWithFee(sb, s.strandSrc, offerOwner, grossIn, netIn, s.book.In); err != nil {
		return err
	}

	// 2. Transfer output currency: offer owner -> taker (strandDst)
	if err := s.transferFunds(sb, offerOwner, s.strandDst, consumedOut, s.book.Out); err != nil {
		return err
	}

	// 3. Update offer's remaining amounts (use NET input for offer consumption)
	offerKey := keylet.Offer(offerOwner, offer.Sequence)

	newTakerPays := s.subtractFromAmount(s.offerTakerPays(offer), netIn)
	newTakerGets := s.subtractFromAmount(s.offerTakerGets(offer), consumedOut)

	// fmt.Printf("DEBUG consumeOffer: newTakerPays=%+v (isZero=%v), newTakerGets=%+v (isZero=%v)\n",
	//	newTakerPays, newTakerPays.IsZero(), newTakerGets, newTakerGets.IsZero())

	if newTakerPays.IsZero() || newTakerGets.IsZero() {
		// // fmt.Printf("DEBUG consumeOffer: deleting offer seq=%d\n", offer.Sequence)
		// Before deleting, update the offer with zero amounts
		// This is needed for correct metadata generation:
		// - PreviousFields should show the original (non-zero) amounts
		// - FinalFields should show the final (zero) amounts
		// Reference: rippled's metadata tracks the state changes before deletion
		offer.TakerPays = s.eitherAmountToTxAmount(newTakerPays, s.book.In)
		offer.TakerGets = s.eitherAmountToTxAmount(newTakerGets, s.book.Out)
		offerData, err := sle.SerializeLedgerOffer(offer)
		if err != nil {
			return err
		}
		if err := sb.Update(offerKey, offerData); err != nil {
			return err
		}
		if err := s.deleteOffer(sb, offer, offerOwner, txHash, ledgerSeq); err != nil {
			return err
		}
	} else {
		offer.TakerPays = s.eitherAmountToTxAmount(newTakerPays, s.book.In)
		offer.TakerGets = s.eitherAmountToTxAmount(newTakerGets, s.book.Out)

		remainingFunded := s.getOfferFundedAmount(sb, offer)
		if remainingFunded.IsEffectivelyZero() {
			offerData, err := sle.SerializeLedgerOffer(offer)
			if err != nil {
				return err
			}
			if err := sb.Update(offerKey, offerData); err != nil {
				return err
			}
			if err := s.deleteOffer(sb, offer, offerOwner, txHash, ledgerSeq); err != nil {
				return err
			}
		} else {
			offer.PreviousTxnID = txHash
			offer.PreviousTxnLgrSeq = ledgerSeq
			offerData, err := sle.SerializeLedgerOffer(offer)
			if err != nil {
				return err
			}
			if err := sb.Update(offerKey, offerData); err != nil {
				return err
			}
		}
	}

	return nil
}

// deleteOffer properly deletes an offer from the ledger.
func (s *BookStep) deleteOffer(sb *PaymentSandbox, offer *sle.LedgerOffer, owner [20]byte, txHash [32]byte, ledgerSeq uint32) error {
	offerKey := keylet.Offer(owner, offer.Sequence)

	// // fmt.Printf("DEBUG deleteOffer: seq=%d, offerKey=%x, bookDir=%x\n", offer.Sequence, offerKey.Key[:8], offer.BookDirectory[:8])

	ownerDirKey := keylet.OwnerDir(owner)
	ownerResult, err := sle.DirRemove(sb, ownerDirKey, offer.OwnerNode, offerKey.Key, false)
	if err != nil {
		// // fmt.Printf("DEBUG deleteOffer: ownerDir remove error: %v\n", err)
	}
	if ownerResult != nil {
		// // fmt.Printf("DEBUG deleteOffer: ownerDir removed, modified=%d, deleted=%d\n", len(ownerResult.ModifiedNodes), len(ownerResult.DeletedNodes))
		s.applyDirRemoveResult(sb, ownerResult)
	}

	bookDirKey := keylet.Keylet{Key: offer.BookDirectory}
	bookResult, err := sle.DirRemove(sb, bookDirKey, offer.BookNode, offerKey.Key, false)
	if err != nil {
		// // fmt.Printf("DEBUG deleteOffer: bookDir remove error: %v\n", err)
	}
	if bookResult != nil {
		// // fmt.Printf("DEBUG deleteOffer: bookDir removed, modified=%d, deleted=%d\n", len(bookResult.ModifiedNodes), len(bookResult.DeletedNodes))
		s.applyDirRemoveResult(sb, bookResult)
	}

	if err := s.adjustOwnerCount(sb, owner, -1, txHash, ledgerSeq); err != nil {
		return err
	}

	if err := sb.Erase(offerKey); err != nil {
		return err
	}

	return nil
}

// applyDirRemoveResult applies directory removal changes to the sandbox
func (s *BookStep) applyDirRemoveResult(sb *PaymentSandbox, result *sle.DirRemoveResult) {
	for _, mod := range result.ModifiedNodes {
		isBookDir := mod.NewState.TakerPaysCurrency != [20]byte{} || mod.NewState.TakerGetsCurrency != [20]byte{}
		data, err := sle.SerializeDirectoryNode(mod.NewState, isBookDir)
		if err != nil {
			continue
		}
		if err := sb.Update(keylet.Keylet{Key: mod.Key}, data); err != nil {
		}
	}

	for _, del := range result.DeletedNodes {
		if err := sb.Erase(keylet.Keylet{Key: del.Key}); err != nil {
		}
	}
}

// adjustOwnerCount adjusts the OwnerCount on an account
func (s *BookStep) adjustOwnerCount(sb *PaymentSandbox, account [20]byte, delta int, txHash [32]byte, ledgerSeq uint32) error {
	accountKey := keylet.Account(account)
	accountData, err := sb.Read(accountKey)
	if err != nil {
		return err
	}
	if accountData == nil {
		return errors.New("account not found for owner count adjustment")
	}

	accountRoot, err := sle.ParseAccountRoot(accountData)
	if err != nil {
		return err
	}

	newCount := int(accountRoot.OwnerCount) + delta
	if newCount < 0 {
		newCount = 0
	}
	accountRoot.OwnerCount = uint32(newCount)
	accountRoot.PreviousTxnID = txHash
	accountRoot.PreviousTxnLgrSeq = ledgerSeq

	newData, err := sle.SerializeAccountRoot(accountRoot)
	if err != nil {
		return err
	}
	return sb.Update(accountKey, newData)
}

// transferFunds transfers an amount between two accounts.
func (s *BookStep) transferFunds(sb *PaymentSandbox, from, to [20]byte, amount EitherAmount, issue Issue) error {
	if from == to {
		return nil
	}

	if amount.IsZero() {
		return nil
	}

	txHash, ledgerSeq := sb.GetTransactionContext()

	if issue.IsXRP() {
		return s.transferXRP(sb, from, to, amount.XRP, txHash, ledgerSeq)
	}

	return s.transferIOU(sb, from, to, amount.IOU, issue, txHash, ledgerSeq)
}

// transferFundsWithFee transfers an IOU amount with transfer fee handling.
// grossAmount is debited from sender, netAmount is credited to receiver.
// This handles the XRPL transfer fee mechanism where sender pays more than receiver gets.
func (s *BookStep) transferFundsWithFee(sb *PaymentSandbox, from, to [20]byte, grossAmount, netAmount EitherAmount, issue Issue) error {
	if from == to {
		return nil
	}

	if grossAmount.IsZero() || netAmount.IsZero() {
		return nil
	}

	txHash, ledgerSeq := sb.GetTransactionContext()

	// For XRP, there's no transfer fee - just use regular transfer
	if issue.IsXRP() {
		return s.transferXRP(sb, from, to, grossAmount.XRP, txHash, ledgerSeq)
	}

	// For IOUs: debit sender by gross, credit receiver by net
	issuer := issue.Issuer

	// Special case: if from is issuer, just credit receiver
	if from == issuer {
		return s.creditTrustline(sb, to, issuer, netAmount.IOU, txHash, ledgerSeq)
	}
	// Special case: if to is issuer, just debit sender
	if to == issuer {
		return s.debitTrustline(sb, from, issuer, grossAmount.IOU, txHash, ledgerSeq)
	}

	// Normal case: debit sender by gross, credit receiver by net
	if err := s.debitTrustline(sb, from, issuer, grossAmount.IOU, txHash, ledgerSeq); err != nil {
		return err
	}
	return s.creditTrustline(sb, to, issuer, netAmount.IOU, txHash, ledgerSeq)
}

// transferXRP transfers XRP between accounts
// Reference: rippled View.cpp accountSend() lines 1904-1939
func (s *BookStep) transferXRP(sb *PaymentSandbox, from, to [20]byte, drops int64, txHash [32]byte, ledgerSeq uint32) error {
	fromKey := keylet.Account(from)
	fromData, err := sb.Read(fromKey)
	if err != nil {
		return err
	}
	if fromData == nil {
		return errors.New("sender account not found")
	}

	fromAccount, err := sle.ParseAccountRoot(fromData)
	if err != nil {
		return err
	}

	if int64(fromAccount.Balance) < drops {
		return errors.New("insufficient XRP balance")
	}

	// Record the credit via CreditHook BEFORE updating balance
	// Reference: rippled View.cpp line 1923: view.creditHook(uSenderID, xrpAccount(), saAmount, sndBal)
	xrpIssuer := [20]byte{} // xrpAccount() is the zero account
	preCreditBalance := tx.NewXRPAmount(int64(fromAccount.Balance))
	amount := tx.NewXRPAmount(drops)
	sb.CreditHook(from, xrpIssuer, amount, preCreditBalance)

	fromAccount.Balance -= uint64(drops)
	fromAccount.PreviousTxnID = txHash
	fromAccount.PreviousTxnLgrSeq = ledgerSeq

	fromAccountData, err := sle.SerializeAccountRoot(fromAccount)
	if err != nil {
		return err
	}
	if err := sb.Update(fromKey, fromAccountData); err != nil {
		return err
	}

	toKey := keylet.Account(to)
	toData, err := sb.Read(toKey)
	if err != nil {
		return err
	}
	if toData == nil {
		return errors.New("receiver account not found")
	}

	toAccount, err := sle.ParseAccountRoot(toData)
	if err != nil {
		return err
	}

	// Record the credit to receiver
	// Reference: rippled View.cpp line 1936: view.creditHook(xrpAccount(), uReceiverID, saAmount, -rcvBal)
	receiverPreBalance := tx.NewXRPAmount(-int64(toAccount.Balance))
	sb.CreditHook(xrpIssuer, to, amount, receiverPreBalance)

	toAccount.Balance += uint64(drops)
	toAccount.PreviousTxnID = txHash
	toAccount.PreviousTxnLgrSeq = ledgerSeq

	toAccountData, err := sle.SerializeAccountRoot(toAccount)
	if err != nil {
		return err
	}
	if err := sb.Update(toKey, toAccountData); err != nil {
		return err
	}

	return nil
}

// transferIOU transfers IOU between accounts via trustline
func (s *BookStep) transferIOU(sb *PaymentSandbox, from, to [20]byte, amount tx.Amount, issue Issue, txHash [32]byte, ledgerSeq uint32) error {
	issuer := issue.Issuer

	if from == issuer {
		return s.creditTrustline(sb, to, issuer, amount, txHash, ledgerSeq)
	}
	if to == issuer {
		return s.debitTrustline(sb, from, issuer, amount, txHash, ledgerSeq)
	}

	if err := s.debitTrustline(sb, from, issuer, amount, txHash, ledgerSeq); err != nil {
		return err
	}
	return s.creditTrustline(sb, to, issuer, amount, txHash, ledgerSeq)
}

// creditTrustline increases an account's IOU balance
func (s *BookStep) creditTrustline(sb *PaymentSandbox, account, issuer [20]byte, amount tx.Amount, txHash [32]byte, ledgerSeq uint32) error {
	lineKey := keylet.Line(account, issuer, amount.Currency)
	lineData, err := sb.Read(lineKey)
	if err != nil {
		return err
	}
	if lineData == nil {
		return errors.New("trustline not found for credit")
	}

	rs, err := sle.ParseRippleState(lineData)
	if err != nil {
		return err
	}

	accountIsLow := sle.CompareAccountIDsForLine(account, issuer) < 0
	if accountIsLow {
		rs.Balance, _ = rs.Balance.Add(amount)
	} else {
		rs.Balance, _ = rs.Balance.Sub(amount)
	}

	rs.PreviousTxnID = txHash
	rs.PreviousTxnLgrSeq = ledgerSeq

	lineDataNew, err := sle.SerializeRippleState(rs)
	if err != nil {
		return err
	}
	return sb.Update(lineKey, lineDataNew)
}

// debitTrustline decreases an account's IOU balance
func (s *BookStep) debitTrustline(sb *PaymentSandbox, account, issuer [20]byte, amount tx.Amount, txHash [32]byte, ledgerSeq uint32) error {
	lineKey := keylet.Line(account, issuer, amount.Currency)
	lineData, err := sb.Read(lineKey)
	if err != nil {
		return err
	}
	if lineData == nil {
		return errors.New("trustline not found for debit")
	}

	rs, err := sle.ParseRippleState(lineData)
	if err != nil {
		return err
	}

	accountIsLow := sle.CompareAccountIDsForLine(account, issuer) < 0
	if accountIsLow {
		rs.Balance, _ = rs.Balance.Sub(amount)
	} else {
		rs.Balance, _ = rs.Balance.Add(amount)
	}

	rs.PreviousTxnID = txHash
	rs.PreviousTxnLgrSeq = ledgerSeq

	lineDataNew, err := sle.SerializeRippleState(rs)
	if err != nil {
		return err
	}
	return sb.Update(lineKey, lineDataNew)
}

// subtractFromAmount subtracts consumed from the original amount
func (s *BookStep) subtractFromAmount(original, consumed EitherAmount) EitherAmount {
	if original.IsNative {
		return NewXRPEitherAmount(original.XRP - consumed.XRP)
	}
	result, _ := original.IOU.Sub(consumed.IOU)
	return NewIOUEitherAmount(result)
}

// eitherAmountToTxAmount converts EitherAmount to tx.Amount
func (s *BookStep) eitherAmountToTxAmount(ea EitherAmount, issue Issue) tx.Amount {
	if ea.IsNative {
		return tx.NewXRPAmount(ea.XRP)
	}
	return ea.IOU
}

// getTipQuality gets the best quality available in the order book
func (s *BookStep) getTipQuality(sb *PaymentSandbox) *Quality {
	offer, _, err := s.getNextOffer(sb, sb, nil)
	if err != nil || offer == nil {
		return nil
	}

	q := s.offerQuality(offer)
	return &q
}

// Check validates the BookStep before use
func (s *BookStep) Check(sb *PaymentSandbox) tx.Result {
	offer, _, err := s.getNextOffer(sb, sb, nil)
	if err != nil {
		return tx.TefINTERNAL
	}
	if offer == nil {
		return tx.TecPATH_DRY
	}

	return tx.TesSUCCESS
}

// LedgerReader is an interface for reading ledger entries.
// PaymentSandbox and other views can implement this.
type LedgerReader interface {
	Read(key keylet.Keylet) ([]byte, error)
}

// GetLedgerReserves reads the reserve values from the ledger's FeeSettings entry.
// Returns (baseReserve, incrementReserve) in drops.
// If FeeSettings cannot be read, returns default values (10 XRP, 2 XRP).
// Reference: rippled View.cpp uses fees keylet to read reserves
func GetLedgerReserves(view LedgerReader) (baseReserve, incrementReserve int64) {
	// Default values (modern mainnet values)
	defaultBase := int64(10_000_000)      // 10 XRP
	defaultIncrement := int64(2_000_000)  // 2 XRP

	feesKey := keylet.Fees()
	feesData, err := view.Read(feesKey)
	if err != nil || feesData == nil {
		return defaultBase, defaultIncrement
	}

	feeSettings, err := sle.ParseFeeSettings(feesData)
	if err != nil {
		return defaultBase, defaultIncrement
	}

	return int64(feeSettings.GetReserveBase()), int64(feeSettings.GetReserveIncrement())
}
