package tx

import (
	"math/big"
	"strconv"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// encodeAccountIDSafe returns the account ID as string, ignoring errors
// Used for creating zero IOU amounts where the exact issuer encoding isn't critical
func encodeAccountIDSafe(id [20]byte) string {
	s, _ := encodeAccountID(id)
	return s
}

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
		inactive_:            false,
		offersUsed_:          0,
		cache:                nil,
	}
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
	prevStepDebtDir := DebtDirectionIssues
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
		totalIn = ZeroIOUEitherAmount(s.book.In.Currency, encodeAccountIDSafe(s.book.In.Issuer))
	}
	if s.book.Out.IsXRP() {
		totalOut = ZeroXRPEitherAmount()
	} else {
		totalOut = ZeroIOUEitherAmount(s.book.Out.Currency, encodeAccountIDSafe(s.book.Out.Issuer))
	}

	remainingOut := out

	// Iterate through offers
	for s.offersUsed_ < s.maxOffersToConsume && !remainingOut.IsZero() {
		// Get next offer at best quality
		offer, offerKey, err := s.getNextOffer(sb, afView)
		if err != nil || offer == nil {
			break // No more offers
		}

		// Check if offer is funded
		if !s.isOfferFunded(sb, offer) {
			ofrsToRm[offerKey] = true
			s.offersUsed_++
			continue
		}

		// Calculate how much we can get from this offer
		offerOut := s.offerTakerGets(offer)
		offerIn := s.offerTakerPays(offer)
		offerQuality := s.offerQuality(offer)

		// Limit by what we still need
		var actualOut, actualIn EitherAmount
		if offerOut.Compare(remainingOut) <= 0 {
			// Take entire offer
			actualOut = offerOut
			actualIn = s.applyQuality(actualOut, offerQuality, trIn, trOut, true)
		} else {
			// Partial take
			actualOut = remainingOut
			actualIn = s.applyQuality(actualOut, offerQuality, trIn, trOut, true)

			// Ensure we don't exceed offer's input
			maxIn := offerIn
			if actualIn.Compare(maxIn) > 0 {
				actualIn = maxIn
				actualOut = s.reverseQuality(actualIn, offerQuality, trIn, trOut, false)
			}
		}

		// Consume the offer
		err = s.consumeOffer(sb, offer, actualIn, actualOut)
		if err != nil {
			break
		}

		// Accumulate
		totalIn = totalIn.Add(actualIn)
		totalOut = totalOut.Add(actualOut)
		remainingOut = remainingOut.Sub(actualOut)
		s.offersUsed_++
	}

	// Check if we should become inactive
	if s.offersUsed_ >= s.maxOffersToConsume {
		s.inactive_ = true
	}

	s.cache = &bookCache{
		in:  totalIn,
		out: totalOut,
	}

	return totalIn, totalOut
}

// Fwd executes the step with the given input
func (s *BookStep) Fwd(
	sb *PaymentSandbox,
	afView *PaymentSandbox,
	ofrsToRm map[[32]byte]bool,
	in EitherAmount,
) (EitherAmount, EitherAmount) {
	if s.cache == nil {
		if s.book.In.IsXRP() {
			return ZeroXRPEitherAmount(), ZeroXRPEitherAmount()
		}
		issuer := encodeAccountIDSafe(s.book.In.Issuer)
		return ZeroIOUEitherAmount(s.book.In.Currency, issuer), ZeroIOUEitherAmount(s.book.Out.Currency, "")
	}

	s.offersUsed_ = 0

	// Get transfer rates
	prevStepDebtDir := DebtDirectionIssues
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
		totalIn = ZeroIOUEitherAmount(s.book.In.Currency, encodeAccountIDSafe(s.book.In.Issuer))
	}
	if s.book.Out.IsXRP() {
		totalOut = ZeroXRPEitherAmount()
	} else {
		totalOut = ZeroIOUEitherAmount(s.book.Out.Currency, encodeAccountIDSafe(s.book.Out.Issuer))
	}

	remainingIn := in

	// Iterate through offers
	for s.offersUsed_ < s.maxOffersToConsume && !remainingIn.IsZero() {
		offer, _, err := s.getNextOffer(sb, afView)
		if err != nil || offer == nil {
			break
		}

		if !s.isOfferFunded(sb, offer) {
			s.offersUsed_++
			continue
		}

		offerIn := s.offerTakerPays(offer)
		offerQuality := s.offerQuality(offer)

		// Calculate how much we can use from this offer
		var actualIn, actualOut EitherAmount
		if offerIn.Compare(remainingIn) >= 0 {
			// Partial use of offer
			actualIn = remainingIn
		} else {
			// Use entire offer
			actualIn = offerIn
		}

		actualOut = s.reverseQuality(actualIn, offerQuality, trIn, trOut, false)

		// Limit output to cached value to prevent forward > reverse
		if s.cache != nil {
			remainingCacheOut := s.cache.out.Sub(totalOut)
			if actualOut.Compare(remainingCacheOut) > 0 {
				actualOut = remainingCacheOut
				actualIn = s.applyQuality(actualOut, offerQuality, trIn, trOut, true)
			}
		}

		err = s.consumeOffer(sb, offer, actualIn, actualOut)
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

	return s.getAccountTransferRate(sb, s.book.In.Issuer)
}

// transferRateOut returns the transfer rate for outgoing currency
func (s *BookStep) transferRateOut(sb *PaymentSandbox) uint32 {
	if s.book.Out.IsXRP() {
		return QualityOne
	}

	if !s.ownerPaysTransferFee {
		return QualityOne
	}

	return s.getAccountTransferRate(sb, s.book.Out.Issuer)
}

// getAccountTransferRate gets the transfer rate from an account
func (s *BookStep) getAccountTransferRate(sb *PaymentSandbox, issuer [20]byte) uint32 {
	accountKey := keylet.Account(issuer)
	data, err := sb.Read(accountKey)
	if err != nil || data == nil {
		return QualityOne
	}

	account, err := parseAccountRoot(data)
	if err != nil {
		return QualityOne
	}

	if account.TransferRate == 0 {
		return QualityOne
	}
	return account.TransferRate
}

// getNextOffer returns the next offer at the best quality
func (s *BookStep) getNextOffer(sb *PaymentSandbox, afView *PaymentSandbox) (*LedgerOffer, [32]byte, error) {
	// Get the order book directory
	// Convert Issues to the format expected by keylet.BookDir
	takerPaysCurrency := getCurrencyBytes(s.book.In.Currency)
	takerPaysIssuer := s.book.In.Issuer
	takerGetsCurrency := getCurrencyBytes(s.book.Out.Currency)
	takerGetsIssuer := s.book.Out.Issuer
	bookDir := keylet.BookDir(takerPaysCurrency, takerPaysIssuer, takerGetsCurrency, takerGetsIssuer)

	data, err := sb.Read(bookDir)
	if err != nil || data == nil {
		return nil, [32]byte{}, nil // No offers
	}

	// Parse directory to get first offer
	// This is simplified - actual implementation needs to iterate through directory pages
	dir, err := parseDirectoryNode(data)
	if err != nil || len(dir.Indexes) == 0 {
		return nil, [32]byte{}, nil
	}

	// Get first offer
	var offerKey [32]byte
	copy(offerKey[:], dir.Indexes[0][:])

	offerData, err := sb.Read(keylet.Keylet{Key: offerKey})
	if err != nil || offerData == nil {
		return nil, [32]byte{}, nil
	}

	offer, err := parseLedgerOffer(offerData)
	if err != nil {
		return nil, [32]byte{}, err
	}

	return offer, offerKey, nil
}

// isOfferFunded checks if an offer has sufficient funding
func (s *BookStep) isOfferFunded(sb *PaymentSandbox, offer *LedgerOffer) bool {
	// Check the offer owner has the funds to back the offer
	// This is simplified - actual implementation checks balances
	if offer == nil {
		return false
	}
	if offer.TakerGets.Value == "" || offer.TakerGets.Value == "0" {
		return false
	}
	// For IOU, check if negative (shouldn't be in a valid offer)
	if len(offer.TakerGets.Value) > 0 && offer.TakerGets.Value[0] == '-' {
		return false
	}
	return true
}

// offerTakerGets returns what the taker gets from this offer
func (s *BookStep) offerTakerGets(offer *LedgerOffer) EitherAmount {
	if s.book.Out.IsXRP() {
		drops, _ := strconv.ParseInt(offer.TakerGets.Value, 10, 64)
		return NewXRPEitherAmount(drops)
	}
	return NewIOUEitherAmount(offer.TakerGets.ToIOU())
}

// offerTakerPays returns what the taker pays to this offer
func (s *BookStep) offerTakerPays(offer *LedgerOffer) EitherAmount {
	if s.book.In.IsXRP() {
		drops, _ := strconv.ParseInt(offer.TakerPays.Value, 10, 64)
		return NewXRPEitherAmount(drops)
	}
	return NewIOUEitherAmount(offer.TakerPays.ToIOU())
}

// offerQuality returns the quality of an offer
func (s *BookStep) offerQuality(offer *LedgerOffer) Quality {
	takerPays := s.offerTakerPays(offer)
	takerGets := s.offerTakerGets(offer)
	return QualityFromAmounts(takerPays, takerGets)
}

// applyQuality applies quality and transfer rates to convert output to input
// input = output * quality * trIn / trOut (for reverse: given output, find input)
func (s *BookStep) applyQuality(out EitherAmount, q Quality, trIn, trOut uint32, roundUp bool) EitherAmount {
	// Calculate: in = out * quality * trIn / trOut
	// Quality is already in/out ratio, so we multiply

	if out.IsNative {
		// XRP calculation
		result := float64(out.XRP) * (float64(q.Value) / float64(QualityOne))
		result = result * (float64(trIn) / float64(trOut))
		if roundUp && result != float64(int64(result)) {
			return NewXRPEitherAmount(int64(result) + 1)
		}
		return NewXRPEitherAmount(int64(result))
	}

	// IOU calculation
	qRatio := new(big.Float).Quo(
		new(big.Float).SetUint64(q.Value),
		new(big.Float).SetUint64(uint64(QualityOne)),
	)
	trRatio := new(big.Float).Quo(
		new(big.Float).SetUint64(uint64(trIn)),
		new(big.Float).SetUint64(uint64(trOut)),
	)
	result := new(big.Float).Mul(out.IOU.Value, qRatio)
	result = new(big.Float).Mul(result, trRatio)

	return NewIOUEitherAmount(IOUAmount{
		Value:    result,
		Currency: s.book.In.Currency,
		Issuer:   encodeAccountIDSafe(s.book.In.Issuer),
	})
}

// reverseQuality applies reverse quality to convert input to output
// output = input / quality * trOut / trIn
func (s *BookStep) reverseQuality(in EitherAmount, q Quality, trIn, trOut uint32, roundUp bool) EitherAmount {
	if in.IsNative {
		result := float64(in.XRP) / (float64(q.Value) / float64(QualityOne))
		result = result * (float64(trOut) / float64(trIn))
		if roundUp && result != float64(int64(result)) {
			return NewXRPEitherAmount(int64(result) + 1)
		}
		return NewXRPEitherAmount(int64(result))
	}

	qRatio := new(big.Float).Quo(
		new(big.Float).SetUint64(uint64(QualityOne)),
		new(big.Float).SetUint64(q.Value),
	)
	trRatio := new(big.Float).Quo(
		new(big.Float).SetUint64(uint64(trOut)),
		new(big.Float).SetUint64(uint64(trIn)),
	)
	result := new(big.Float).Mul(in.IOU.Value, qRatio)
	result = new(big.Float).Mul(result, trRatio)

	return NewIOUEitherAmount(IOUAmount{
		Value:    result,
		Currency: s.book.Out.Currency,
		Issuer:   encodeAccountIDSafe(s.book.Out.Issuer),
	})
}

// consumeOffer reduces the offer's amounts by the consumed amounts
func (s *BookStep) consumeOffer(sb *PaymentSandbox, offer *LedgerOffer, consumedIn, consumedOut EitherAmount) error {
	// Update the offer's remaining amounts
	// This is simplified - actual implementation updates the offer in the ledger

	// Note: In a real implementation, we would:
	// 1. Update the offer's TakerPays and TakerGets
	// 2. If fully consumed, delete the offer
	// 3. Transfer the currencies between accounts

	return nil
}

// getTipQuality gets the best quality available in the order book
func (s *BookStep) getTipQuality(sb *PaymentSandbox) *Quality {
	offer, _, err := s.getNextOffer(sb, sb)
	if err != nil || offer == nil {
		return nil
	}

	q := s.offerQuality(offer)
	return &q
}

// Check validates the BookStep before use
func (s *BookStep) Check(sb *PaymentSandbox) Result {
	// Check that the book has at least some liquidity
	offer, _, err := s.getNextOffer(sb, sb)
	if err != nil {
		return TefINTERNAL
	}
	if offer == nil {
		return TecPATH_DRY
	}

	return TesSUCCESS
}

// Note: parseOfferForBookStep removed - now using parseLedgerOffer directly
