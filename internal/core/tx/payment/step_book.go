package payment

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"

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
		totalIn = ZeroIOUEitherAmount(s.book.In.Currency, sle.EncodeAccountIDSafe(s.book.In.Issuer))
	}
	if s.book.Out.IsXRP() {
		totalOut = ZeroXRPEitherAmount()
	} else {
		totalOut = ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
	}

	remainingOut := out

	fmt.Printf("DEBUG Rev: remainingOut=%+v, book.In=%v, book.Out=%v\n", remainingOut, s.book.In, s.book.Out)

	// Iterate through offers
	for s.offersUsed_ < s.maxOffersToConsume && !remainingOut.IsZero() {
		// Get next offer at best quality
		offer, offerKey, err := s.getNextOffer(sb, afView, ofrsToRm)
		if err != nil {
			fmt.Printf("DEBUG Rev: getNextOffer error: %v\n", err)
			break
		}
		if offer == nil {
			fmt.Printf("DEBUG Rev: no more offers\n")
			break // No more offers
		}

		fmt.Printf("DEBUG Rev: got offer from %s, TakerPays=%+v, TakerGets=%+v\n",
			offer.Account, offer.TakerPays, offer.TakerGets)

		// Check if offer is funded
		if !s.isOfferFunded(sb, offer) {
			fmt.Printf("DEBUG Rev: offer not funded, removing\n")
			ofrsToRm[offerKey] = true
			s.offersUsed_++
			continue
		}

		fmt.Printf("DEBUG Rev: offer is funded, processing\n")

		// Check quality limit - if offer quality is worse than limit, stop
		offerQuality := s.offerQuality(offer)
		if s.qualityLimit != nil && offerQuality.WorseThan(*s.qualityLimit) {
			fmt.Printf("DEBUG Rev: offer quality %d exceeds limit %d, stopping\n", offerQuality.Value, s.qualityLimit.Value)
			break
		}

		// Calculate how much we can get from this offer
		// Use funded amount, which may be less than stated TakerGets
		offerTakerGetsStated := s.offerTakerGets(offer)
		offerOut := s.getOfferFundedAmount(sb, offer)
		offerIn := s.offerTakerPays(offer)

		// Scale offerIn proportionally if funded amount is less than stated
		if offerOut.Compare(offerTakerGetsStated) < 0 && !offerTakerGetsStated.IsZero() {
			// fundedRatio = fundedOut / statedOut
			// fundedIn = statedIn * fundedRatio
			ratio := offerOut.DivideFloat(offerTakerGetsStated)
			offerIn = offerIn.MultiplyFloat(ratio)
		}

		fmt.Printf("DEBUG Rev: offerOut=%+v (funded), offerIn=%+v, remainingOut=%+v\n", offerOut, offerIn, remainingOut)

		// Limit by what we still need
		var actualOut, actualIn EitherAmount
		if offerOut.Compare(remainingOut) <= 0 {
			// Take entire offer - use pre-calculated offerIn to avoid rounding errors
			actualOut = offerOut
			actualIn = offerIn
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

	fmt.Printf("DEBUG Rev: final totalIn=%+v, totalOut=%+v, offersUsed=%d\n", totalIn, totalOut, s.offersUsed_)

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
	fmt.Printf("DEBUG Fwd: in=%+v, book.In=%v, book.Out=%v\n", in, s.book.In, s.book.Out)

	// Clear cache from any previous execution to allow fresh computation
	// This is needed when Fwd is called with different constraints than a prior Rev
	prevCache := s.cache
	s.cache = nil
	s.offersUsed_ = 0

	// We'll restore the old cache for limiting ONLY if this is a continuation
	// of a Rev pass (indicated by prevCache being set and matching constraints)
	_ = prevCache

	// Get transfer rates
	prevStepDebtDir := DebtDirectionIssues
	if s.prevStep != nil {
		prevStepDebtDir = s.prevStep.DebtDirection(sb, StrandDirectionForward)
	}

	trIn := s.transferRateIn(sb, prevStepDebtDir)
	trOut := s.transferRateOut(sb)

	fmt.Printf("DEBUG Fwd: trIn=%d, trOut=%d\n", trIn, trOut)

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

	fmt.Printf("DEBUG Fwd: starting loop, remainingIn=%+v\n", remainingIn)

	// Iterate through offers
	for s.offersUsed_ < s.maxOffersToConsume && !remainingIn.IsZero() {
		offer, offerKey, err := s.getNextOffer(sb, afView, ofrsToRm)
		if err != nil {
			fmt.Printf("DEBUG Fwd: getNextOffer error: %v\n", err)
			break
		}
		if offer == nil {
			fmt.Printf("DEBUG Fwd: no more offers\n")
			break
		}

		fmt.Printf("DEBUG Fwd: got offer from %s, TakerPays=%+v, TakerGets=%+v, offerKey=%x\n",
			offer.Account, offer.TakerPays, offer.TakerGets, offerKey)

		if !s.isOfferFunded(sb, offer) {
			fmt.Printf("DEBUG Fwd: offer not funded, skipping\n")
			s.offersUsed_++
			continue
		}

		// Check quality limit - if offer quality is worse than limit, stop
		offerQuality := s.offerQuality(offer)
		if s.qualityLimit != nil && offerQuality.WorseThan(*s.qualityLimit) {
			fmt.Printf("DEBUG Fwd: offer quality %d exceeds limit %d, stopping\n", offerQuality.Value, s.qualityLimit.Value)
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

		fmt.Printf("DEBUG Fwd: fundedOut=%+v, offerIn=%+v (scaled)\n", fundedOut, offerIn)

		// Calculate how much we can use from this offer
		var actualIn, actualOut EitherAmount
		if offerIn.Compare(remainingIn) >= 0 {
			// Partial use of offer
			actualIn = remainingIn
		} else {
			// Use entire offer (up to funded amount)
			actualIn = offerIn
		}

		actualOut = s.reverseQuality(actualIn, offerQuality, trIn, trOut, false)

		// Limit output to funded amount
		if actualOut.Compare(fundedOut) > 0 {
			actualOut = fundedOut
			actualIn = s.applyQuality(actualOut, offerQuality, trIn, trOut, true)
		}

		// Limit output to cached value to prevent forward > reverse
		if s.cache != nil {
			remainingCacheOut := s.cache.out.Sub(totalOut)
			if actualOut.Compare(remainingCacheOut) > 0 {
				actualOut = remainingCacheOut
				actualIn = s.applyQuality(actualOut, offerQuality, trIn, trOut, true)
			}
		}

		fmt.Printf("DEBUG Fwd: consuming offer, actualIn=%+v, actualOut=%+v\n", actualIn, actualOut)

		err = s.consumeOffer(sb, offer, actualIn, actualOut)
		if err != nil {
			fmt.Printf("DEBUG Fwd: consumeOffer error: %v\n", err)
			break
		}

		totalIn = totalIn.Add(actualIn)
		totalOut = totalOut.Add(actualOut)
		remainingIn = remainingIn.Sub(actualIn)
		s.offersUsed_++

		fmt.Printf("DEBUG Fwd: after consume, totalIn=%+v, totalOut=%+v, remainingIn=%+v\n", totalIn, totalOut, remainingIn)
	}

	if s.offersUsed_ >= s.maxOffersToConsume {
		s.inactive_ = true
	}

	fmt.Printf("DEBUG Fwd: final totalIn=%+v, totalOut=%+v, offersUsed=%d\n", totalIn, totalOut, s.offersUsed_)

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

// getNextOffer returns the next offer at the best quality, skipping offers in ofrsToRm
func (s *BookStep) getNextOffer(sb *PaymentSandbox, afView *PaymentSandbox, ofrsToRm map[[32]byte]bool) (*sle.LedgerOffer, [32]byte, error) {
	// Get the order book directory base key
	// Convert Issues to the format expected by keylet.BookDir
	takerPaysCurrency := sle.GetCurrencyBytes(s.book.In.Currency)
	takerPaysIssuer := s.book.In.Issuer
	takerGetsCurrency := sle.GetCurrencyBytes(s.book.Out.Currency)
	takerGetsIssuer := s.book.Out.Issuer
	bookBase := keylet.BookDir(takerPaysCurrency, takerPaysIssuer, takerGetsCurrency, takerGetsIssuer)

	// The first 24 bytes of the book key are the book prefix
	// Book directories are sorted by quality (encoded in last 8 bytes)
	// We need to find directories that match this prefix
	bookPrefix := bookBase.Key[:24]

	// Collect all book directories sorted by quality
	type dirEntry struct {
		key  [32]byte
		data []byte
	}
	var dirs []dirEntry
	entryCount := 0

	err := sb.ForEach(func(key [32]byte, data []byte) bool {
		entryCount++
		// Check if this key matches our book prefix
		if bytes.Equal(key[:24], bookPrefix) {
			dirs = append(dirs, dirEntry{key: key, data: data})
		}
		return true // Continue iterating
	})
	if err != nil {
		return nil, [32]byte{}, err
	}

	// Sort directories by quality (lower value = better quality)
	sort.Slice(dirs, func(i, j int) bool {
		return bytes.Compare(dirs[i].key[24:], dirs[j].key[24:]) < 0
	})

	fmt.Printf("DEBUG getNextOffer: bookPrefix=%x, entries=%d, dirs=%d\n",
		bookPrefix, entryCount, len(dirs))

	// Iterate through directories to find first non-removed offer
	for _, d := range dirs {
		dir, err := sle.ParseDirectoryNode(d.data)
		if err != nil || len(dir.Indexes) == 0 {
			continue
		}

		// Iterate through offers in this directory
		for _, idx := range dir.Indexes {
			var offerKey [32]byte
			copy(offerKey[:], idx[:])

			// Skip if marked for removal
			if ofrsToRm != nil && ofrsToRm[offerKey] {
				continue
			}

			offerData, err := sb.Read(keylet.Keylet{Key: offerKey})
			if err != nil || offerData == nil {
				// Offer was deleted, skip
				continue
			}

			offer, err := sle.ParseLedgerOffer(offerData)
			if err != nil {
				continue
			}

			return offer, offerKey, nil
		}
	}

	return nil, [32]byte{}, nil // No offers found
}

// isOfferFunded checks if an offer has sufficient funding
func (s *BookStep) isOfferFunded(sb *PaymentSandbox, offer *sle.LedgerOffer) bool {
	if offer == nil {
		return false
	}
	if offer.TakerGets.Value == "" || offer.TakerGets.Value == "0" {
		return false
	}
	// Check if actual funded amount is effectively > 0
	funded := s.getOfferFundedAmount(sb, offer)
	return !funded.IsEffectivelyZero()
}

// getOfferFundedAmount returns the actual amount an offer can deliver based on owner's balance.
// The offer's TakerGets may be larger than what the owner can actually fund.
// Reference: rippled's offer_auto.cpp and accountHolds()
func (s *BookStep) getOfferFundedAmount(sb *PaymentSandbox, offer *sle.LedgerOffer) EitherAmount {
	offerOwner, err := sle.DecodeAccountID(offer.Account)
	if err != nil {
		return ZeroXRPEitherAmount()
	}

	// Get the offer's stated TakerGets
	offerTakerGets := s.offerTakerGets(offer)

	if s.book.Out.IsXRP() {
		// For XRP TakerGets: check owner's XRP balance
		accountKey := keylet.Account(offerOwner)
		accountData, err := sb.Read(accountKey)
		if err != nil || accountData == nil {
			return ZeroXRPEitherAmount()
		}

		account, err := sle.ParseAccountRoot(accountData)
		if err != nil {
			return ZeroXRPEitherAmount()
		}

		// Available XRP = balance - reserve
		// Reserve = base_reserve + owner_count * increment_reserve
		// Use approximate values for now
		baseReserve := int64(10000000)       // 10 XRP base reserve
		incrementReserve := int64(2000000)   // 2 XRP per owned object
		reserve := baseReserve + int64(account.OwnerCount)*incrementReserve
		available := int64(account.Balance) - reserve

		if available <= 0 {
			return ZeroXRPEitherAmount()
		}

		// Return minimum of available and TakerGets
		if available < offerTakerGets.XRP {
			return NewXRPEitherAmount(available)
		}
		return offerTakerGets
	}

	// For IOU TakerGets: check owner's trustline balance with issuer
	issuer := s.book.Out.Issuer
	currency := s.book.Out.Currency

	// Get owner's balance on trustline with issuer
	ownerBalance := s.getIOUBalance(sb, offerOwner, issuer, currency)

	// The owner can sell up to their positive balance
	// If balance is negative (owes issuer), they can't sell
	if ownerBalance.IsNegative() || ownerBalance.IsZero() {
		// Check if owner IS the issuer (can issue unlimited)
		if offerOwner == issuer {
			return offerTakerGets
		}
		return ZeroIOUEitherAmount(currency, sle.EncodeAccountIDSafe(issuer))
	}

	// Return minimum of owner balance and TakerGets
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
		// Return a very large amount
		return NewIOUEitherAmount(sle.IOUAmount{
			Value:    new(big.Float).SetFloat64(1e15),
			Currency: currency,
			Issuer:   issuerStr,
		})
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

	var balanceValue *big.Float
	if accountIsLow {
		// Balance is from account's perspective: positive means account owns that IOU
		balanceValue = rs.Balance.Value
	} else {
		// Account is high: balance is negated
		balanceValue = new(big.Float).Neg(rs.Balance.Value)
	}

	// Return with correct issuer (not the trustline placeholder)
	return NewIOUEitherAmount(sle.IOUAmount{
		Value:    balanceValue,
		Currency: currency,
		Issuer:   issuerStr,
	})
}

// offerTakerGets returns what the taker gets from this offer
func (s *BookStep) offerTakerGets(offer *sle.LedgerOffer) EitherAmount {
	if s.book.Out.IsXRP() {
		drops, _ := strconv.ParseInt(offer.TakerGets.Value, 10, 64)
		return NewXRPEitherAmount(drops)
	}
	return NewIOUEitherAmount(offer.TakerGets.ToIOU())
}

// offerTakerPays returns what the taker pays to this offer
func (s *BookStep) offerTakerPays(offer *sle.LedgerOffer) EitherAmount {
	if s.book.In.IsXRP() {
		drops, _ := strconv.ParseInt(offer.TakerPays.Value, 10, 64)
		return NewXRPEitherAmount(drops)
	}
	return NewIOUEitherAmount(offer.TakerPays.ToIOU())
}

// offerQuality returns the quality of an offer
func (s *BookStep) offerQuality(offer *sle.LedgerOffer) Quality {
	takerPays := s.offerTakerPays(offer)
	takerGets := s.offerTakerGets(offer)
	return QualityFromAmounts(takerPays, takerGets)
}

// applyQuality applies quality and transfer rates to convert output to input
// input = output * quality * trIn / trOut (for reverse: given output, find input)
func (s *BookStep) applyQuality(out EitherAmount, q Quality, trIn, trOut uint32, roundUp bool) EitherAmount {
	// Calculate: in = out * quality * trIn / trOut
	// Quality is already in/out ratio, so we multiply
	// The result type depends on book.In (what we're paying), not on what we're receiving

	// Get the output value as a float for calculation
	var outValue float64
	if out.IsNative {
		outValue = float64(out.XRP)
	} else {
		outValue, _ = out.IOU.Value.Float64()
	}

	// Calculate the input amount
	result := outValue * (float64(q.Value) / float64(QualityOne))
	result = result * (float64(trIn) / float64(trOut))

	// Return appropriate type based on book.In (what we're paying)
	if s.book.In.IsXRP() {
		// XRP result
		if roundUp && result != float64(int64(result)) {
			return NewXRPEitherAmount(int64(result) + 1)
		}
		return NewXRPEitherAmount(int64(result))
	}

	// IOU result
	return NewIOUEitherAmount(sle.IOUAmount{
		Value:    new(big.Float).SetFloat64(result),
		Currency: s.book.In.Currency,
		Issuer:   sle.EncodeAccountIDSafe(s.book.In.Issuer),
	})
}

// reverseQuality applies reverse quality to convert input to output
// output = input / quality * trOut / trIn
// The output type is always book.Out (what we're receiving)
func (s *BookStep) reverseQuality(in EitherAmount, q Quality, trIn, trOut uint32, roundUp bool) EitherAmount {
	// Get input value as float for calculation
	var inValue float64
	if in.IsNative {
		inValue = float64(in.XRP)
	} else {
		inValue, _ = in.IOU.Value.Float64()
	}

	// Calculate: output = input / quality * trOut / trIn
	result := inValue / (float64(q.Value) / float64(QualityOne))
	result = result * (float64(trOut) / float64(trIn))

	// Return appropriate type based on book.Out (what we're receiving)
	if s.book.Out.IsXRP() {
		// XRP result
		if roundUp && result != float64(int64(result)) {
			return NewXRPEitherAmount(int64(result) + 1)
		}
		return NewXRPEitherAmount(int64(result))
	}

	// IOU result
	return NewIOUEitherAmount(sle.IOUAmount{
		Value:    new(big.Float).SetFloat64(result),
		Currency: s.book.Out.Currency,
		Issuer:   sle.EncodeAccountIDSafe(s.book.Out.Issuer),
	})
}

// consumeOffer reduces the offer's amounts by the consumed amounts and transfers funds.
// Reference: rippled BookStep.cpp consumeOffer()
func (s *BookStep) consumeOffer(sb *PaymentSandbox, offer *sle.LedgerOffer, consumedIn, consumedOut EitherAmount) error {
	offerOwner, err := sle.DecodeAccountID(offer.Account)
	if err != nil {
		return err
	}

	// Get transaction context for PreviousTxnID updates
	txHash, ledgerSeq := sb.GetTransactionContext()

	// 1. Transfer input currency: taker (strandSrc) -> offer owner
	// The taker pays consumedIn to the offer owner
	if err := s.transferFunds(sb, s.strandSrc, offerOwner, consumedIn, s.book.In); err != nil {
		return err
	}

	// 2. Transfer output currency: offer owner -> taker (strandDst)
	// The offer owner pays consumedOut to the taker
	if err := s.transferFunds(sb, offerOwner, s.strandDst, consumedOut, s.book.Out); err != nil {
		return err
	}

	// 3. Update offer's remaining amounts
	offerKey := keylet.Offer(offerOwner, offer.Sequence)

	// Calculate new amounts
	newTakerPays := s.subtractFromAmount(s.offerTakerPays(offer), consumedIn)
	newTakerGets := s.subtractFromAmount(s.offerTakerGets(offer), consumedOut)

	// Check if offer is fully consumed OR becomes unfunded after partial consume
	if newTakerPays.IsZero() || newTakerGets.IsZero() {
		// Offer fully consumed - delete it
		// Reference: rippled offerDelete() in View.cpp
		if err := s.deleteOffer(sb, offer, offerOwner, txHash, ledgerSeq); err != nil {
			return err
		}
	} else {
		// Update offer with remaining amounts temporarily to check if still funded
		offer.TakerPays = s.eitherAmountToTxAmount(newTakerPays, s.book.In)
		offer.TakerGets = s.eitherAmountToTxAmount(newTakerGets, s.book.Out)
		// Note: DON'T update PreviousTxnID/PreviousTxnLgrSeq here yet - threading
		// will be applied by ApplyStateTable for entries that survive

		// Check if the remaining offer is still funded
		remainingFunded := s.getOfferFundedAmount(sb, offer)
		if remainingFunded.IsEffectivelyZero() {
			// Offer became unfunded after partial consume - delete it
			// First update the offer with reduced amounts so metadata shows PreviousFields correctly
			// Reference: rippled records the partial consumption before deletion
			// Note: For deleted entries, DON'T update PreviousTxnID/PreviousTxnLgrSeq
			// as rippled's FinalFields shows the original values
			fmt.Printf("DEBUG consumeOffer: remaining offer unfunded (funded=%+v), deleting\n", remainingFunded)
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
			// Offer still funded - update with remaining amounts AND threading info
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
// Reference: rippled offerDelete() in View.cpp
// Steps:
// 1. Remove from owner's directory
// 2. Remove from book directory (delete directory if empty)
// 3. Adjust owner count
// 4. Erase the offer
func (s *BookStep) deleteOffer(sb *PaymentSandbox, offer *sle.LedgerOffer, owner [20]byte, txHash [32]byte, ledgerSeq uint32) error {
	offerKey := keylet.Offer(owner, offer.Sequence)

	// 1. Remove from owner's directory
	ownerDirKey := keylet.OwnerDir(owner)
	ownerResult, err := sle.DirRemove(sb, ownerDirKey, offer.OwnerNode, offerKey.Key, false)
	if err != nil {
		fmt.Printf("DEBUG deleteOffer: error removing from owner dir: %v\n", err)
	}
	// Apply the directory changes to sandbox
	if ownerResult != nil {
		s.applyDirRemoveResult(sb, ownerResult)
	}

	// 2. Remove from book directory (keepRoot=false - delete if empty)
	bookDirKey := keylet.Keylet{Key: offer.BookDirectory}
	bookResult, err := sle.DirRemove(sb, bookDirKey, offer.BookNode, offerKey.Key, false)
	if err != nil {
		fmt.Printf("DEBUG deleteOffer: error removing from book dir: %v\n", err)
	}
	// Apply the directory changes to sandbox
	if bookResult != nil {
		s.applyDirRemoveResult(sb, bookResult)
	}

	// 3. Adjust owner count (decrement by 1)
	if err := s.adjustOwnerCount(sb, owner, -1, txHash, ledgerSeq); err != nil {
		return err
	}

	// 4. Erase the offer
	if err := sb.Erase(offerKey); err != nil {
		return err
	}

	return nil
}

// applyDirRemoveResult applies directory removal changes to the sandbox
func (s *BookStep) applyDirRemoveResult(sb *PaymentSandbox, result *sle.DirRemoveResult) {
	// Apply modifications
	for _, mod := range result.ModifiedNodes {
		isBookDir := mod.NewState.TakerPaysCurrency != [20]byte{} || mod.NewState.TakerGetsCurrency != [20]byte{}
		data, err := sle.SerializeDirectoryNode(mod.NewState, isBookDir)
		if err != nil {
			fmt.Printf("DEBUG applyDirRemoveResult: error serializing: %v\n", err)
			continue
		}
		if err := sb.Update(keylet.Keylet{Key: mod.Key}, data); err != nil {
			fmt.Printf("DEBUG applyDirRemoveResult: error updating: %v\n", err)
		}
	}

	// Apply deletions
	for _, del := range result.DeletedNodes {
		if err := sb.Erase(keylet.Keylet{Key: del.Key}); err != nil {
			fmt.Printf("DEBUG applyDirRemoveResult: error erasing: %v\n", err)
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

	// Adjust owner count
	newCount := int(accountRoot.OwnerCount) + delta
	if newCount < 0 {
		newCount = 0
	}
	accountRoot.OwnerCount = uint32(newCount)
	accountRoot.PreviousTxnID = txHash
	accountRoot.PreviousTxnLgrSeq = ledgerSeq

	// Serialize and update
	newData, err := sle.SerializeAccountRoot(accountRoot)
	if err != nil {
		return err
	}
	return sb.Update(accountKey, newData)
}

// transferFunds transfers an amount between two accounts.
// Reference: rippled accountSend()
func (s *BookStep) transferFunds(sb *PaymentSandbox, from, to [20]byte, amount EitherAmount, issue Issue) error {
	if from == to {
		return nil // No transfer needed
	}

	if amount.IsZero() {
		return nil
	}

	txHash, ledgerSeq := sb.GetTransactionContext()

	if issue.IsXRP() {
		// XRP transfer
		return s.transferXRP(sb, from, to, amount.XRP, txHash, ledgerSeq)
	}

	// IOU transfer
	return s.transferIOU(sb, from, to, amount.IOU, issue, txHash, ledgerSeq)
}

// transferXRP transfers XRP between accounts
func (s *BookStep) transferXRP(sb *PaymentSandbox, from, to [20]byte, drops int64, txHash [32]byte, ledgerSeq uint32) error {
	// Update sender balance
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

	// Update receiver balance
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
func (s *BookStep) transferIOU(sb *PaymentSandbox, from, to [20]byte, amount sle.IOUAmount, issue Issue, txHash [32]byte, ledgerSeq uint32) error {
	issuer := issue.Issuer

	// Handle direct issuer transfers (minting/burning)
	if from == issuer {
		// Issuer sending to someone - credit the receiver's trustline
		return s.creditTrustline(sb, to, issuer, amount, txHash, ledgerSeq)
	}
	if to == issuer {
		// Someone sending to issuer - debit the sender's trustline
		return s.debitTrustline(sb, from, issuer, amount, txHash, ledgerSeq)
	}

	// Third-party transfer: debit sender, credit receiver
	if err := s.debitTrustline(sb, from, issuer, amount, txHash, ledgerSeq); err != nil {
		return err
	}
	return s.creditTrustline(sb, to, issuer, amount, txHash, ledgerSeq)
}

// creditTrustline increases an account's IOU balance
func (s *BookStep) creditTrustline(sb *PaymentSandbox, account, issuer [20]byte, amount sle.IOUAmount, txHash [32]byte, ledgerSeq uint32) error {
	lineKey := keylet.Line(account, issuer, amount.Currency)
	lineData, err := sb.Read(lineKey)
	if err != nil {
		return err
	}
	if lineData == nil {
		// No trustline exists - this would need to be created
		// For now, return error (trustline should exist for offer crossing)
		return errors.New("trustline not found for credit")
	}

	rs, err := sle.ParseRippleState(lineData)
	if err != nil {
		return err
	}

	// Determine balance direction based on canonical ordering
	accountIsLow := sle.CompareAccountIDsForLine(account, issuer) < 0
	if accountIsLow {
		// Account is low, balance is from account's perspective
		rs.Balance = rs.Balance.Add(amount)
	} else {
		// Account is high, balance is negated
		rs.Balance = rs.Balance.Sub(amount)
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
func (s *BookStep) debitTrustline(sb *PaymentSandbox, account, issuer [20]byte, amount sle.IOUAmount, txHash [32]byte, ledgerSeq uint32) error {
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

	// Determine balance direction based on canonical ordering
	accountIsLow := sle.CompareAccountIDsForLine(account, issuer) < 0
	if accountIsLow {
		// Account is low, balance is from account's perspective
		rs.Balance = rs.Balance.Sub(amount)
	} else {
		// Account is high, balance is negated
		rs.Balance = rs.Balance.Add(amount)
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
	return NewIOUEitherAmount(sle.IOUAmount{
		Value:    new(big.Float).Sub(original.IOU.Value, consumed.IOU.Value),
		Currency: original.IOU.Currency,
		Issuer:   original.IOU.Issuer,
	})
}

// eitherAmountToTxAmount converts EitherAmount to tx.Amount
func (s *BookStep) eitherAmountToTxAmount(ea EitherAmount, issue Issue) tx.Amount {
	if ea.IsNative {
		return tx.Amount{Value: strconv.FormatInt(ea.XRP, 10)}
	}
	return tx.Amount{
		Value:    sle.FormatIOUValue(ea.IOU.Value),
		Currency: issue.Currency,
		Issuer:   sle.EncodeAccountIDSafe(issue.Issuer),
	}
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
	// Check that the book has at least some liquidity
	offer, _, err := s.getNextOffer(sb, sb, nil)
	if err != nil {
		return tx.TefINTERNAL
	}
	if offer == nil {
		return tx.TecPATH_DRY
	}

	return tx.TesSUCCESS
}

// Note: parseOfferForBookStep removed - now using tx.ParseLedgerOffer directly
