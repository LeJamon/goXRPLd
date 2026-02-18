package payment

import (
	"bytes"
	"errors"
	"fmt"
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

	// parentCloseTime is the parent ledger close time (Ripple epoch seconds)
	// Used to check offer expiration during iteration
	parentCloseTime uint32

	// defaultPath indicates this step is on the default path (not an explicit path).
	// Used for self-cross detection during offer crossing.
	// Reference: rippled BookOfferCrossingStep::defaultPath_
	defaultPath bool

	// fixReducedOffersV2 gates ceil_in_strict vs ceil_in in limitStepIn.
	// When enabled, uses strict rounding (roundUp=false) to prevent order book blocking.
	// Reference: rippled Offer.h TOffer::limitIn() and fixReducedOffersV2 amendment
	fixReducedOffersV2 bool

	// fixReducedOffersV1 gates roundUp in CeilOutStrict calls for underfunded offers.
	// When enabled (roundUp=false), rounding down prevents quality degradation when
	// an offer is partially filled and the remaining amounts are adjusted.
	// Without the fix (roundUp=true), rounding up can make the remaining offer's
	// rate worse than the original, "polluting" the order book.
	// Reference: rippled fixReducedOffersV1 amendment + Offer.h TOffer::limitOut()
	fixReducedOffersV1 bool

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
// by consuming offers from the order book.
// Matches rippled's BookStep::revImp() + forEachOffer() flow.
// Reference: BookStep.cpp lines 1014-1131 (revImp) + 717-873 (forEachOffer)
func (s *BookStep) Rev(
	sb *PaymentSandbox,
	afView *PaymentSandbox,
	ofrsToRm map[[32]byte]bool,
	out EitherAmount,
) (EitherAmount, EitherAmount) {
	s.cache = nil
	s.offersUsed_ = 0

	// Get transfer rates
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

	// Track visited offers
	visited := make(map[[32]byte]bool)

	fmt.Printf("[BookStep.Rev] book In=%s/%x Out=%s/%x remainingOut=%v\n",
		s.book.In.Currency, s.book.In.Issuer, s.book.Out.Currency, s.book.Out.Issuer, remainingOut)

	// Iterate through offers — combined forEachOffer + revImp callback
	for s.offersUsed_ < s.maxOffersToConsume && !remainingOut.IsZero() {
		offer, offerKey, err := s.getNextOfferSkipVisited(sb, afView, ofrsToRm, visited)
		if err != nil {
			break
		}
		if offer == nil {
			break
		}

		visited[offerKey] = true

		// Self-cross detection (default path only)
		if s.defaultPath && s.qualityLimit != nil {
			offerOwner, ownerErr := sle.DecodeAccountID(offer.Account)
			if ownerErr == nil {
				offerQuality := s.offerQuality(offer)
				if !offerQuality.WorseThan(*s.qualityLimit) &&
					s.strandSrc == offerOwner && s.strandDst == offerOwner {
					ofrsToRm[offerKey] = true
					s.offersUsed_++
					continue
				}
			}
		}

		if !s.isOfferFunded(sb, offer) {
			ofrsToRm[offerKey] = true
			s.offersUsed_++
			continue
		}

		// Quality check
		offerQuality := s.offerQuality(offer)
		if s.qualityLimit != nil && offerQuality.WorseThan(*s.qualityLimit) {
			break
		}

		// === forEachOffer: compute ofrAmt, stpAmt, ownerGives ===
		// Reference: BookStep.cpp lines 802-834

		// ofrAmt = offer.amount() (NET: TakerPays/TakerGets)
		ofrIn := s.offerTakerPays(offer)
		ofrOut := s.offerTakerGets(offer)

		// stpAmt.in = mulRatio(ofrAmt.in, ofrInRate, QUALITY_ONE, true) (GROSS input)
		// stpAmt.out = ofrAmt.out
		stpIn := MulRatio(ofrIn, trIn, QualityOne, true)
		stpOut := ofrOut

		// ownerGives = mulRatio(ofrAmt.out, ofrOutRate, QUALITY_ONE, false)
		ownerGives := MulRatio(ofrOut, trOut, QualityOne, false)

		// Funding cap: if funds < ownerGives, adjust all amounts
		// Reference: BookStep.cpp lines 816-830
		funds := s.getOfferFundedAmount(sb, offer)
		offerOwner, _ := sle.DecodeAccountID(offer.Account)
		isFunded := offerOwner == s.book.Out.Issuer // offer owner is issuer = unlimited funds
		if !isFunded {
			if funds.Compare(ownerGives) < 0 {
				ownerGives = funds
				stpOut = MulRatio(ownerGives, QualityOne, trOut, false)
				// fixReducedOffersV1: roundUp=false preserves quality (WITH fix).
				// Without fix, roundUp=true rounds up TakerPays, degrading the remaining offer's rate.
				// Reference: rippled Offer.h TOffer::limitOut() roundUp parameter
				ofrIn, ofrOut = offerQuality.CeilOutStrict(ofrIn, ofrOut, stpOut, !s.fixReducedOffersV1)
				stpIn = MulRatio(ofrIn, trIn, QualityOne, true)
			}
		}

		fmt.Printf("[BookStep.Rev] offer: ofrIn=%v ofrOut=%v stpIn=%v stpOut=%v ownerGives=%v\n",
			ofrIn, ofrOut, stpIn, stpOut, ownerGives)

		// === revImp callback: decide full take vs partial take ===
		// Reference: BookStep.cpp lines 1044-1081
		if stpOut.Compare(remainingOut) <= 0 {
			// Full take: consume entire offer as-is
			totalIn = totalIn.Add(stpIn)
			totalOut = totalOut.Add(stpOut)
			remainingOut = out.Sub(totalOut)

			fmt.Printf("[BookStep.Rev] full take: stpIn=%v stpOut=%v remaining=%v\n", stpIn, stpOut, remainingOut)
			if err := s.consumeOffer(sb, offer, stpIn, ofrIn, stpOut); err != nil {
				break
			}
		} else {
			// Partial take: limitStepOut
			// Reference: BookStep.cpp limitStepOut lines 688-712
			ofrAdjIn, ofrAdjOut := ofrIn, ofrOut
			stpAdjIn, stpAdjOut := stpIn, stpOut
			_, _ = stpAdjIn, stpAdjOut

			// limitStepOut: limit = remainingOut
			stpAdjOut = remainingOut
			ofrAdjIn, ofrAdjOut = offerQuality.CeilOutStrict(ofrAdjIn, ofrAdjOut, stpAdjOut, true)
			stpAdjIn = MulRatio(ofrAdjIn, trIn, QualityOne, true)

			totalIn = totalIn.Add(stpAdjIn)
			totalOut = out // result.out = out (force exact)
			remainingOut = s.zeroOut()

			fmt.Printf("[BookStep.Rev] partial take: trIn=%d trOut=%d\n", trIn, trOut)
		fmt.Printf("  ofrAdjIn:  %s\n", fmtEA(ofrAdjIn))
		fmt.Printf("  ofrAdjOut: %s\n", fmtEA(ofrAdjOut))
		fmt.Printf("  stpAdjIn:  %s\n", fmtEA(stpAdjIn))
		fmt.Printf("  stpAdjOut: %s\n", fmtEA(stpAdjOut))
			if err := s.consumeOffer(sb, offer, stpAdjIn, ofrAdjIn, stpAdjOut); err != nil {
				break
			}
		}

		s.offersUsed_++
	}

	// Check if we should become inactive
	if s.offersUsed_ >= s.maxOffersToConsume {
		s.inactive_ = true
	}

	// Handle remainingOut == 0 but totalOut != out (normalization artifact)
	// Reference: BookStep.cpp lines 1122-1126
	if remainingOut.IsZero() || remainingOut.IsNegative() {
		totalOut = out
	}

	s.cache = &bookCache{
		in:  totalIn,
		out: totalOut,
	}

	fmt.Printf("[BookStep.Rev] returning totalIn=%v totalOut=%v\n", totalIn, totalOut)
	return totalIn, totalOut
}

// Fwd executes the step with the given input.
// Matches rippled's BookStep::fwdImp() + forEachOffer() flow.
// Reference: BookStep.cpp lines 1133-1299 (fwdImp) + 717-873 (forEachOffer)
func (s *BookStep) Fwd(
	sb *PaymentSandbox,
	afView *PaymentSandbox,
	ofrsToRm map[[32]byte]bool,
	in EitherAmount,
) (EitherAmount, EitherAmount) {

	prevCache := s.cache
	s.cache = nil
	s.offersUsed_ = 0

	// Get transfer rates
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
	fmt.Printf("[BookStep.Fwd] START in=%v book=%v→%v trIn=%d trOut=%d\n", in, s.book.In, s.book.Out, trIn, trOut)

	visited := make(map[[32]byte]bool)

	for s.offersUsed_ < s.maxOffersToConsume && !remainingIn.IsZero() {
		offer, offerKey, err := s.getNextOfferSkipVisited(sb, afView, ofrsToRm, visited)
		if err != nil {
			break
		}
		if offer == nil {
			break
		}

		visited[offerKey] = true

		// Self-cross detection
		if s.defaultPath && s.qualityLimit != nil {
			offerOwner, ownerErr := sle.DecodeAccountID(offer.Account)
			if ownerErr == nil {
				offerQuality := s.offerQuality(offer)
				if !offerQuality.WorseThan(*s.qualityLimit) &&
					s.strandSrc == offerOwner && s.strandDst == offerOwner {
					ofrsToRm[offerKey] = true
					s.offersUsed_++
					continue
				}
			}
		}

		if !s.isOfferFunded(sb, offer) {
			s.offersUsed_++
			continue
		}

		offerQuality := s.offerQuality(offer)
		if s.qualityLimit != nil && offerQuality.WorseThan(*s.qualityLimit) {
			break
		}

		// === forEachOffer: compute ofrAmt, stpAmt, ownerGives ===
		ofrIn := s.offerTakerPays(offer)
		ofrOut := s.offerTakerGets(offer)

		stpIn := MulRatio(ofrIn, trIn, QualityOne, true)
		stpOut := ofrOut
		ownerGives := MulRatio(ofrOut, trOut, QualityOne, false)

		// Funding cap
		funds := s.getOfferFundedAmount(sb, offer)
		offerOwner, _ := sle.DecodeAccountID(offer.Account)
		isFunded := offerOwner == s.book.Out.Issuer
		if !isFunded {
			if funds.Compare(ownerGives) < 0 {
				ownerGives = funds
				stpOut = MulRatio(ownerGives, QualityOne, trOut, false)
				// fixReducedOffersV1: roundUp=false preserves quality (WITH fix).
				// Without fix, roundUp=true rounds up TakerPays, degrading the remaining offer's rate.
				// Reference: rippled Offer.h TOffer::limitOut() roundUp parameter
				ofrIn, ofrOut = offerQuality.CeilOutStrict(ofrIn, ofrOut, stpOut, !s.fixReducedOffersV1)
				stpIn = MulRatio(ofrIn, trIn, QualityOne, true)
			}
		}

		// === fwdImp callback ===
		// Reference: BookStep.cpp lines 1172-1252
		if stpIn.Compare(remainingIn) <= 0 {
			// Full take
			totalIn = totalIn.Add(stpIn)
			totalOut = totalOut.Add(stpOut)

			// Check if forward produced more output than reverse cache
			if prevCache != nil && totalOut.Compare(prevCache.out) > 0 && totalIn.Compare(prevCache.in) <= 0 {
				// Recompute using limitStepOut with remaining cache output
				remainingCacheOut := prevCache.out.Sub(totalOut.Sub(stpOut))
				adjOfrIn, adjOfrOut := ofrIn, ofrOut
				adjStpOut := remainingCacheOut
				_ = MulRatio(adjStpOut, trOut, QualityOne, false) // ownerGivesAdj
				adjOfrIn, adjOfrOut = offerQuality.CeilOutStrict(adjOfrIn, adjOfrOut, adjStpOut, true)
				adjStpIn := MulRatio(adjOfrIn, trIn, QualityOne, true)

				if adjStpIn.Compare(remainingIn) == 0 {
					totalIn = in
					totalOut = prevCache.out
					if err := s.consumeOffer(sb, offer, adjStpIn, adjOfrIn, adjStpOut); err != nil {
						break
					}
					remainingIn = s.zeroIn()
					s.offersUsed_++
					continue
				}
			}

			remainingIn = in.Sub(totalIn)
			if err := s.consumeOffer(sb, offer, stpIn, ofrIn, stpOut); err != nil {
				break
			}
		} else {
			// Partial take: limitStepIn
			// Reference: BookStep.cpp limitStepIn lines 660-685
			stpAdjIn := remainingIn
			inLmt := MulRatio(stpAdjIn, QualityOne, trIn, false)
			// fixReducedOffersV2 gates ceil_in (roundUp=true, non-strict) vs
			// ceil_in_strict (roundUp=false, strict). With the fix, rounding down
			// the output prevents the remaining offer from having a worse rate.
			// Reference: rippled Offer.h TOffer::limitIn() + fixReducedOffersV2
			var ofrAdjIn, ofrAdjOut EitherAmount
			if s.fixReducedOffersV2 {
				ofrAdjIn, ofrAdjOut = offerQuality.CeilInStrict(ofrIn, ofrOut, inLmt, false)
			} else {
				ofrAdjIn, ofrAdjOut = offerQuality.CeilIn(ofrIn, ofrOut, inLmt)
			}
			stpAdjOut := ofrAdjOut
			_ = MulRatio(ofrAdjOut, trOut, QualityOne, false) // ownerGivesAdj

			fmt.Printf("[BookStep.Fwd] partial take: trIn=%d trOut=%d\n", trIn, trOut)
			fmt.Printf("  stpAdjIn:  %s\n", fmtEA(stpAdjIn))
			fmt.Printf("  inLmt:     %s\n", fmtEA(inLmt))
			fmt.Printf("  ofrAdjIn:  %s\n", fmtEA(ofrAdjIn))
			fmt.Printf("  ofrAdjOut: %s\n", fmtEA(ofrAdjOut))
			fmt.Printf("  stpAdjOut: %s\n", fmtEA(stpAdjOut))

			totalOut = totalOut.Add(stpAdjOut)
			totalIn = in

			// Check forward > reverse
			if prevCache != nil && totalOut.Compare(prevCache.out) > 0 && totalIn.Compare(prevCache.in) <= 0 {
				remainingCacheOut := prevCache.out.Sub(totalOut.Sub(stpAdjOut))
				revOfrIn, revOfrOut := ofrIn, ofrOut
				revStpOut := remainingCacheOut
				revOfrIn, revOfrOut = offerQuality.CeilOutStrict(revOfrIn, revOfrOut, revStpOut, true)
				revStpIn := MulRatio(revOfrIn, trIn, QualityOne, true)
				_ = revOfrOut

				if revStpIn.Compare(remainingIn) == 0 {
					totalIn = in
					totalOut = prevCache.out
					if err := s.consumeOffer(sb, offer, revStpIn, revOfrIn, revStpOut); err != nil {
						break
					}
					remainingIn = s.zeroIn()
					s.offersUsed_++
					continue
				}
			}

			remainingIn = s.zeroIn()
			if err := s.consumeOffer(sb, offer, stpAdjIn, ofrAdjIn, stpAdjOut); err != nil {
				break
			}
		}

		s.offersUsed_++
	}

	if s.offersUsed_ >= s.maxOffersToConsume {
		s.inactive_ = true
	}

	// Handle remainingIn == 0 but totalIn != in
	if remainingIn.IsZero() || remainingIn.IsNegative() {
		totalIn = in
	}

	s.cache = &bookCache{
		in:  totalIn,
		out: totalOut,
	}

	fmt.Printf("[BookStep.Fwd] DONE totalIn=%v totalOut=%v\n", totalIn, totalOut)
	return totalIn, totalOut
}

// fmtEA formats an EitherAmount showing mantissa/exponent for IOU or drops for XRP.
// Used for debug prints only.
func fmtEA(a EitherAmount) string {
	if a.IsNative {
		return fmt.Sprintf("XRP drops=%d", a.XRP)
	}
	return fmt.Sprintf("IOU man=%d exp=%d", a.IOU.Mantissa(), a.IOU.Exponent())
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

			// Check offer expiration
			// Reference: rippled OfferStream.cpp lines 256-265
			if s.parentCloseTime > 0 && offer.Expiration > 0 &&
				offer.Expiration <= s.parentCloseTime {
				s.removeExpiredOffer(sb, offer, offerKey)
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


			if ofrsToRm != nil && ofrsToRm[offerKey] {
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

			// Check offer expiration
			// Reference: rippled OfferStream.cpp lines 256-265
			if s.parentCloseTime > 0 && offer.Expiration > 0 &&
				offer.Expiration <= s.parentCloseTime {
				s.removeExpiredOffer(sb, offer, offerKey)
				continue
			}

			return offer, offerKey, nil
		}
	}

	return nil, [32]byte{}, nil
}

// removeExpiredOffer removes an expired offer from the ledger.
// Reference: rippled OfferStream::permRmOffer
func (s *BookStep) removeExpiredOffer(sb *PaymentSandbox, offer *sle.LedgerOffer, offerKey [32]byte) {
	ownerID, err := sle.DecodeAccountID(offer.Account)
	if err != nil {
		return
	}

	txHash, ledgerSeq := sb.GetTransactionContext()

	// Remove from owner directory
	ownerDirKey := keylet.OwnerDir(ownerID)
	sle.DirRemove(sb, ownerDirKey, offer.OwnerNode, offerKey, false)

	// Remove from book directory
	bookDirKey := keylet.Keylet{Type: 100, Key: offer.BookDirectory}
	sle.DirRemove(sb, bookDirKey, offer.BookNode, offerKey, false)

	// Erase the offer
	sb.Erase(keylet.Keylet{Key: offerKey})

	// Decrement owner count
	s.adjustOwnerCount(sb, ownerID, -1, txHash, ledgerSeq)
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

	// Convert output to IOU-style Amount for precise multiplication.
	// XRP amounts MUST be converted to IOU representation because Amount.Mul()
	// determines result type from the first operand — if it's XRP, the product
	// is truncated to drops, destroying sub-drop precision (e.g., 0.333 USD → 0).
	// Reference: rippled's mulRound takes explicit output Issue parameter.
	var outAmt tx.Amount
	if out.IsNative {
		outAmt = sle.NewIssuedAmountFromValue(out.XRP, 0, "", "")
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

	// Convert input to IOU-style Amount for precise division.
	// XRP amounts MUST be converted to avoid drops truncation in Div().
	var inAmt tx.Amount
	if in.IsNative {
		inAmt = sle.NewIssuedAmountFromValue(in.XRP, 0, "", "")
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

	// For IOU output with XRP input: use big.Int for maximum precision.
	// Amount.Mul(native, IOU) returns native, losing IOU precision for small amounts.
	// Reference: rippled mulRound takes an asset parameter to determine output type.
	if !s.book.Out.IsXRP() && inputAmt.IsNative() && !offerGets.IsNative && offerPays.IsNative {
		inputDrops := big.NewInt(input.XRP)
		getsMant := big.NewInt(offerGets.IOU.Mantissa())
		getsExp := offerGets.IOU.Exponent()
		paysDrops := big.NewInt(offerPays.XRP)

		if paysDrops.Sign() == 0 {
			return ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
		}

		// output = (inputDrops × QualityOne / trIn) × getsMant × 10^getsExp / paysDrops
		// Combine into single fraction to minimize precision loss:
		// numerator = inputDrops × QualityOne × getsMant
		// denominator = trIn × paysDrops
		// result mantissa = numerator / denominator, result exp = getsExp
		numerator := new(big.Int).Set(inputDrops)
		numerator.Mul(numerator, big.NewInt(int64(QualityOne)))
		numerator.Mul(numerator, getsMant)

		denominator := new(big.Int).SetInt64(int64(trIn))
		denominator.Mul(denominator, paysDrops)

		// Round UP: (numerator + denominator - 1) / denominator
		numerator.Add(numerator, denominator)
		numerator.Sub(numerator, big.NewInt(1))
		resultMant := new(big.Int).Div(numerator, denominator)
		resultExp := getsExp

		// Normalize mantissa to IOU range [10^15, 10^16)
		minMant := big.NewInt(1000000000000000)  // 10^15
		maxMant := big.NewInt(10000000000000000) // 10^16
		ten := big.NewInt(10)
		for resultMant.Cmp(maxMant) >= 0 {
			resultMant.Div(resultMant, ten)
			resultExp++
		}
		for resultMant.Sign() > 0 && resultMant.Cmp(minMant) < 0 {
			resultMant.Mul(resultMant, ten)
			resultExp--
		}

		if resultMant.Sign() == 0 {
			return ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
		}

		return NewIOUEitherAmount(tx.NewIssuedAmount(
			resultMant.Int64(), resultExp,
			s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer)))
	}

	// For other cases: convert GROSS to NET, then use Amount arithmetic
	// netIn = grossIn × QualityOne / trIn
	netInputAmt := inputAmt.MulRatio(QualityOne, trIn, false) // round down for NET

	// output = netIn × offerGets / offerPays (round up)
	// When both are IOU, Mul returns IOU correctly.
	// When both are XRP, Mul returns XRP correctly.
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

	// For IOU output with XRP input: use big.Int for maximum precision.
	// Amount.Mul(native, IOU) returns native, losing IOU precision for small amounts.
	if !s.book.Out.IsXRP() && inputAmt.IsNative() && !offerGets.IsNative && offerPays.IsNative {
		inputDrops := big.NewInt(input.XRP)
		getsMant := big.NewInt(offerGets.IOU.Mantissa())
		getsExp := offerGets.IOU.Exponent()
		paysDrops := big.NewInt(offerPays.XRP)

		if paysDrops.Sign() == 0 {
			return ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
		}

		// output = inputDrops × getsMant / paysDrops, with result exp = getsExp
		numerator := new(big.Int).Set(inputDrops)
		numerator.Mul(numerator, getsMant)

		denominator := new(big.Int).Set(paysDrops)

		// Round UP
		numerator.Add(numerator, denominator)
		numerator.Sub(numerator, big.NewInt(1))
		resultMant := new(big.Int).Div(numerator, denominator)
		resultExp := getsExp

		// Normalize mantissa to IOU range [10^15, 10^16)
		minMant := big.NewInt(1000000000000000)
		maxMant := big.NewInt(10000000000000000)
		ten := big.NewInt(10)
		for resultMant.Cmp(maxMant) >= 0 {
			resultMant.Div(resultMant, ten)
			resultExp++
		}
		for resultMant.Sign() > 0 && resultMant.Cmp(minMant) < 0 {
			resultMant.Mul(resultMant, ten)
			resultExp--
		}

		if resultMant.Sign() == 0 {
			return ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
		}

		return NewIOUEitherAmount(tx.NewIssuedAmount(
			resultMant.Int64(), resultExp,
			s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer)))
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

	// For IOU output with XRP input: use big.Int for maximum precision.
	// Amount.Mul(native, IOU) returns native, losing IOU precision for small amounts.
	if !s.book.Out.IsXRP() && inputAmt.IsNative() && !offerGets.IsNative && offerPays.IsNative {
		inputDrops := big.NewInt(input.XRP)
		getsMant := big.NewInt(offerGets.IOU.Mantissa())
		getsExp := offerGets.IOU.Exponent()
		paysDrops := big.NewInt(offerPays.XRP)

		if paysDrops.Sign() == 0 {
			return ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
		}

		// output = (inputDrops × trOut / trIn) × getsMant / paysDrops
		numerator := new(big.Int).Set(inputDrops)
		if trIn != trOut && trIn != 0 {
			numerator.Mul(numerator, big.NewInt(int64(trOut)))
		}
		numerator.Mul(numerator, getsMant)

		denominator := new(big.Int).Set(paysDrops)
		if trIn != trOut && trIn != 0 {
			denominator.Mul(denominator, big.NewInt(int64(trIn)))
		}

		// Round UP
		numerator.Add(numerator, denominator)
		numerator.Sub(numerator, big.NewInt(1))
		resultMant := new(big.Int).Div(numerator, denominator)
		resultExp := getsExp

		// Normalize mantissa to IOU range [10^15, 10^16)
		minMant := big.NewInt(1000000000000000)
		maxMant := big.NewInt(10000000000000000)
		ten := big.NewInt(10)
		for resultMant.Cmp(maxMant) >= 0 {
			resultMant.Div(resultMant, ten)
			resultExp++
		}
		for resultMant.Sign() > 0 && resultMant.Cmp(minMant) < 0 {
			resultMant.Mul(resultMant, ten)
			resultExp--
		}

		if resultMant.Sign() == 0 {
			return ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
		}

		return NewIOUEitherAmount(tx.NewIssuedAmount(
			resultMant.Int64(), resultExp,
			s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer)))
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
	offerOwner, err := sle.DecodeAccountID(offer.Account)
	if err != nil {
		return err
	}

	txHash, ledgerSeq := sb.GetTransactionContext()

	grossIn := consumedInGross
	netIn := consumedInNet

	// 1. Transfer input currency with transfer fee:
	//    - For IOU: Transfer from input issuer (book.In.Issuer) to offer owner
	//    - For XRP: Transfer from XRP pseudo-account (zero) to offer owner.
	//      The XRPEndpointStep before BookStep handles deducting XRP from the source account.
	//    Reference: rippled BookStep.cpp - sends from book_.in.account (issuer for IOU, zero for XRP)
	inSource := s.book.In.Issuer // For XRP: zero account; for IOU: the issuer
	if err := s.transferFundsWithFee(sb, inSource, offerOwner, grossIn, netIn, s.book.In); err != nil {
		return err
	}

	// 2. Transfer output currency: offer owner -> book.out issuer (for IOU) or XRP pseudo-account (for XRP)
	//    The DirectStepI or XRPEndpointStep after BookStep handles delivery to the actual destination.
	//    Reference: rippled BookStep.cpp - sends to book_.out.account (issuer for IOU, zero for XRP)
	outRecipient := s.book.Out.Issuer // For XRP: zero account; for IOU: the issuer
	if err := s.transferFunds(sb, offerOwner, outRecipient, consumedOut, s.book.Out); err != nil {
		return err
	}

	// 3. Update offer's remaining amounts (use NET input for offer consumption)
	offerKey := keylet.Offer(offerOwner, offer.Sequence)

	newTakerPays := s.subtractFromAmount(s.offerTakerPays(offer), netIn)
	newTakerGets := s.subtractFromAmount(s.offerTakerGets(offer), consumedOut)

	//	newTakerPays, newTakerPays.IsZero(), newTakerGets, newTakerGets.IsZero())

	if newTakerPays.IsZero() || newTakerGets.IsZero() {
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

// zeroOut returns a zero EitherAmount for the output currency.
func (s *BookStep) zeroOut() EitherAmount {
	if s.book.Out.IsXRP() {
		return ZeroXRPEitherAmount()
	}
	return ZeroIOUEitherAmount(s.book.Out.Currency, sle.EncodeAccountIDSafe(s.book.Out.Issuer))
}

// zeroIn returns a zero EitherAmount for the input currency.
func (s *BookStep) zeroIn() EitherAmount {
	if s.book.In.IsXRP() {
		return ZeroXRPEitherAmount()
	}
	return ZeroIOUEitherAmount(s.book.In.Currency, sle.EncodeAccountIDSafe(s.book.In.Issuer))
}

// deleteOffer properly deletes an offer from the ledger.
func (s *BookStep) deleteOffer(sb *PaymentSandbox, offer *sle.LedgerOffer, owner [20]byte, txHash [32]byte, ledgerSeq uint32) error {
	offerKey := keylet.Offer(owner, offer.Sequence)


	ownerDirKey := keylet.OwnerDir(owner)
	ownerResult, err := sle.DirRemove(sb, ownerDirKey, offer.OwnerNode, offerKey.Key, false)
	if err != nil {
	}
	if ownerResult != nil {
		s.applyDirRemoveResult(sb, ownerResult)
	}

	bookDirKey := keylet.Keylet{Key: offer.BookDirectory}
	bookResult, err := sle.DirRemove(sb, bookDirKey, offer.BookNode, offerKey.Key, false)
	if err != nil {
	}
	if bookResult != nil {
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

// transferXRP transfers XRP between accounts.
// When from or to is the XRP pseudo-account (zero), that side is skipped.
// The XRPEndpointStep handles the actual source/destination account balance changes.
// Reference: rippled View.cpp accountSend() lines 1904-1939
func (s *BookStep) transferXRP(sb *PaymentSandbox, from, to [20]byte, drops int64, txHash [32]byte, ledgerSeq uint32) error {
	var xrpAccount [20]byte
	amount := tx.NewXRPAmount(drops)

	// Debit sender (skip if XRP pseudo-account)
	if from != xrpAccount {
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
		preCreditBalance := tx.NewXRPAmount(int64(fromAccount.Balance))
		sb.CreditHook(from, xrpAccount, amount, preCreditBalance)

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
	}

	// Credit receiver (skip if XRP pseudo-account)
	if to != xrpAccount {
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
		receiverPreBalance := tx.NewXRPAmount(-int64(toAccount.Balance))
		sb.CreditHook(xrpAccount, to, amount, receiverPreBalance)

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

// creditTrustline increases an account's IOU balance.
// If the trust line doesn't exist (e.g., during offer crossing), creates one automatically.
// Reference: rippled View.cpp rippleCredit() → trustCreate()
func (s *BookStep) creditTrustline(sb *PaymentSandbox, account, issuer [20]byte, amount tx.Amount, txHash [32]byte, ledgerSeq uint32) error {
	lineKey := keylet.Line(account, issuer, amount.Currency)
	lineData, err := sb.Read(lineKey)
	if err != nil {
		return err
	}
	if lineData == nil {
		// Trust line doesn't exist — create one (offer crossing creates trust lines on demand).
		// Reference: rippled rippleCredit() → trustCreate() in View.cpp
		return s.trustCreateForCredit(sb, account, issuer, amount, txHash, ledgerSeq)
	}

	rs, err := sle.ParseRippleState(lineData)
	if err != nil {
		return err
	}

	accountIsLow := sle.CompareAccountIDsForLine(account, issuer) < 0
	fmt.Printf("[DEBUG BookStep.creditTrustline] account=%x issuer=%x accountIsLow=%v\n", account[:4], issuer[:4], accountIsLow)
	fmt.Printf("[DEBUG BookStep.creditTrustline] balance BEFORE: mantissa=%d exp=%d val=%s\n", rs.Balance.IOU().Mantissa(), rs.Balance.IOU().Exponent(), rs.Balance.Value())
	fmt.Printf("[DEBUG BookStep.creditTrustline] amount: mantissa=%d exp=%d val=%s\n", amount.IOU().Mantissa(), amount.IOU().Exponent(), amount.Value())
	if accountIsLow {
		rs.Balance, _ = rs.Balance.Add(amount)
	} else {
		rs.Balance, _ = rs.Balance.Sub(amount)
	}
	fmt.Printf("[DEBUG BookStep.creditTrustline] balance AFTER: mantissa=%d exp=%d val=%s\n", rs.Balance.IOU().Mantissa(), rs.Balance.IOU().Exponent(), rs.Balance.Value())

	rs.PreviousTxnID = txHash
	rs.PreviousTxnLgrSeq = ledgerSeq

	lineDataNew, err := sle.SerializeRippleState(rs)
	if err != nil {
		return err
	}
	return sb.Update(lineKey, lineDataNew)
}

// trustCreateForCredit creates a new trust line between account and issuer with initial balance.
// This is used when creditTrustline encounters a missing trust line during offer crossing.
// Reference: rippled View.cpp trustCreate() lines 1329-1445
func (s *BookStep) trustCreateForCredit(sb *PaymentSandbox, account, issuer [20]byte, amount tx.Amount, txHash [32]byte, ledgerSeq uint32) error {
	// Determine low and high accounts
	accountIsLow := sle.CompareAccountIDsForLine(account, issuer) < 0
	var lowAccountID, highAccountID [20]byte
	if accountIsLow {
		lowAccountID = account
		highAccountID = issuer
	} else {
		lowAccountID = issuer
		highAccountID = account
	}

	lowAccountStr := sle.EncodeAccountIDSafe(lowAccountID)
	highAccountStr := sle.EncodeAccountIDSafe(highAccountID)

	// Calculate the initial balance from low account's perspective
	// The issuer sends to account:
	// - If account is LOW: issuer (HIGH) pays account (LOW) → balance increases (positive)
	// - If account is HIGH: issuer (LOW) pays account (HIGH) → balance decreases (negative)
	var balance tx.Amount
	if accountIsLow {
		balance = amount // account is low, receives credit → positive balance
	} else {
		balance = amount.Negate() // account is high, receives credit → negative balance
	}

	// Check receiver account's DefaultRipple flag for NoRipple setting
	var noRipple bool
	accountKey := keylet.Account(account)
	accountData, err := sb.Read(accountKey)
	if err == nil && accountData != nil {
		acct, parseErr := sle.ParseAccountRoot(accountData)
		if parseErr == nil {
			const lsfDefaultRipple = 0x00800000
			noRipple = (acct.Flags & lsfDefaultRipple) == 0
		}
	}

	// Build the trust line flags — set reserve flag for the receiver (account) side
	var flags uint32
	if accountIsLow {
		// account is LOW
		if noRipple {
			flags |= sle.LsfLowNoRipple
		}
		flags |= sle.LsfLowReserve
	} else {
		// account is HIGH
		if noRipple {
			flags |= sle.LsfHighNoRipple
		}
		flags |= sle.LsfHighReserve
	}

	// Create the RippleState
	rs := &sle.RippleState{
		Balance:           tx.NewIssuedAmount(balance.IOU().Mantissa(), balance.IOU().Exponent(), amount.Currency, sle.AccountOneAddress),
		LowLimit:          tx.NewIssuedAmount(0, -100, amount.Currency, lowAccountStr),
		HighLimit:         tx.NewIssuedAmount(0, -100, amount.Currency, highAccountStr),
		Flags:             flags,
		LowNode:           0,
		HighNode:          0,
		PreviousTxnID:     txHash,
		PreviousTxnLgrSeq: ledgerSeq,
	}

	lineKey := keylet.Line(account, issuer, amount.Currency)

	// Insert into LOW account's owner directory
	lowDirKey := keylet.OwnerDir(lowAccountID)
	lowDirResult, err := sle.DirInsert(sb, lowDirKey, lineKey.Key, func(dir *sle.DirectoryNode) {
		dir.Owner = lowAccountID
	})
	if err != nil {
		return err
	}

	// Insert into HIGH account's owner directory
	highDirKey := keylet.OwnerDir(highAccountID)
	highDirResult, err := sle.DirInsert(sb, highDirKey, lineKey.Key, func(dir *sle.DirectoryNode) {
		dir.Owner = highAccountID
	})
	if err != nil {
		return err
	}

	// Set directory node hints
	rs.LowNode = lowDirResult.Page
	rs.HighNode = highDirResult.Page

	// Serialize and insert
	lineData, err := sle.SerializeRippleState(rs)
	if err != nil {
		return err
	}

	if err := sb.Insert(lineKey, lineData); err != nil {
		return err
	}

	// Increment receiver's OwnerCount
	return s.adjustOwnerCountForTrustCreate(sb, account, 1, txHash, ledgerSeq)
}

// adjustOwnerCountForTrustCreate modifies an account's OwnerCount by delta during trust line creation.
func (s *BookStep) adjustOwnerCountForTrustCreate(sb *PaymentSandbox, account [20]byte, delta int32, txHash [32]byte, ledgerSeq uint32) error {
	accountKey := keylet.Account(account)
	data, err := sb.Read(accountKey)
	if err != nil || data == nil {
		return err
	}

	acct, err := sle.ParseAccountRoot(data)
	if err != nil {
		return err
	}

	if delta > 0 {
		acct.OwnerCount += uint32(delta)
	} else if uint32(-delta) <= acct.OwnerCount {
		acct.OwnerCount -= uint32(-delta)
	}

	acct.PreviousTxnID = txHash
	acct.PreviousTxnLgrSeq = ledgerSeq

	newData, err := sle.SerializeAccountRoot(acct)
	if err != nil {
		return err
	}

	sb.Update(accountKey, newData)
	return nil
}

// debitTrustline decreases an account's IOU balance.
// After updating, checks if the trust line should be deleted (zero balance, auto-created).
// Reference: rippled View.cpp rippleCreditIOU() lines 1688-1745
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

	// The "sender" is account (their balance decreases)
	accountIsLow := sle.CompareAccountIDsForLine(account, issuer) < 0

	// Compute sender's balance BEFORE update (from sender's perspective)
	var saBefore tx.Amount
	if accountIsLow {
		saBefore = rs.Balance
	} else {
		saBefore = rs.Balance.Negate()
	}

	// Update balance
	if accountIsLow {
		rs.Balance, _ = rs.Balance.Sub(amount)
	} else {
		rs.Balance, _ = rs.Balance.Add(amount)
	}

	// Compute sender's balance AFTER update
	var saBalance tx.Amount
	if accountIsLow {
		saBalance = rs.Balance
	} else {
		saBalance = rs.Balance.Negate()
	}

	// Check trust line deletion conditions
	// Reference: rippled rippleCreditIOU() lines 1688-1745
	bDelete := false
	uFlags := rs.Flags

	if saBefore.Signum() > 0 && saBalance.Signum() <= 0 {
		var senderReserve, senderNoRipple, senderFreeze uint32
		var senderLimit tx.Amount
		var senderQualityIn, senderQualityOut uint32

		if accountIsLow {
			senderReserve = sle.LsfLowReserve
			senderNoRipple = sle.LsfLowNoRipple
			senderFreeze = sle.LsfLowFreeze
			senderLimit = rs.LowLimit
			senderQualityIn = rs.LowQualityIn
			senderQualityOut = rs.LowQualityOut
		} else {
			senderReserve = sle.LsfHighReserve
			senderNoRipple = sle.LsfHighNoRipple
			senderFreeze = sle.LsfHighFreeze
			senderLimit = rs.HighLimit
			senderQualityIn = rs.HighQualityIn
			senderQualityOut = rs.HighQualityOut
		}

		// Read sender's DefaultRipple flag
		senderDefaultRipple := false
		senderKey := keylet.Account(account)
		senderData, sErr := sb.Read(senderKey)
		if sErr == nil && senderData != nil {
			senderAcct, pErr := sle.ParseAccountRoot(senderData)
			if pErr == nil {
				senderDefaultRipple = (senderAcct.Flags & sle.LsfDefaultRipple) != 0
			}
		}

		hasNoRipple := (uFlags & senderNoRipple) != 0
		noRippleMatchesDefault := hasNoRipple != senderDefaultRipple

		if (uFlags&senderReserve) != 0 &&
			noRippleMatchesDefault &&
			(uFlags&senderFreeze) == 0 &&
			senderLimit.Signum() == 0 &&
			senderQualityIn == 0 &&
			senderQualityOut == 0 {

			// Clear sender's reserve flag and decrement OwnerCount
			rs.Flags &= ^senderReserve
			s.adjustOwnerCount(sb, account, -1, txHash, ledgerSeq)

			// Check final deletion condition
			var receiverReserve uint32
			if accountIsLow {
				receiverReserve = sle.LsfHighReserve
			} else {
				receiverReserve = sle.LsfLowReserve
			}
			bDelete = saBalance.Signum() == 0 && (uFlags&receiverReserve) == 0
		}
	}

	rs.PreviousTxnID = txHash
	rs.PreviousTxnLgrSeq = ledgerSeq

	lineDataNew, err := sle.SerializeRippleState(rs)
	if err != nil {
		return err
	}

	if bDelete {
		// Update first (for metadata), then delete
		sb.Update(lineKey, lineDataNew)

		var lowAccount, highAccount [20]byte
		if accountIsLow {
			lowAccount = account
			highAccount = issuer
		} else {
			lowAccount = issuer
			highAccount = account
		}
		return trustDeleteLine(sb, lineKey, rs, lowAccount, highAccount)
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
		fmt.Printf("[BookStep.getTipQuality] book %v→%v: no offers (err=%v)\n", s.book.In.Currency, s.book.Out.Currency, err)
		return nil
	}

	q := s.offerQuality(offer)
	fmt.Printf("[BookStep.getTipQuality] book %v→%v: quality=%v\n", s.book.In.Currency, s.book.Out.Currency, q)
	return &q
}

// Check validates the BookStep before use
// Reference: rippled BookStep.cpp check() lines 1343-1380
func (s *BookStep) Check(sb *PaymentSandbox) tx.Result {
	// Check for same in/out issue - this is invalid
	// Reference: rippled BookStep.cpp lines 1346-1351
	if s.book.In.Currency == s.book.Out.Currency && s.book.In.Issuer == s.book.Out.Issuer {
		return tx.TemBAD_PATH
	}

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
		fmt.Printf("[DEBUG GetLedgerReserves] Read failed or nil: err=%v data=%v\n", err, feesData == nil)
		return defaultBase, defaultIncrement
	}

	feeSettings, err := sle.ParseFeeSettings(feesData)
	if err != nil {
		fmt.Printf("[DEBUG GetLedgerReserves] Parse failed: err=%v\n", err)
		return defaultBase, defaultIncrement
	}

	base := int64(feeSettings.GetReserveBase())
	inc := int64(feeSettings.GetReserveIncrement())
	fmt.Printf("[DEBUG GetLedgerReserves] ReserveBaseDrops=%d ReserveIncrementDrops=%d ReserveBase=%v ReserveInc=%v → base=%d inc=%d\n",
		feeSettings.ReserveBaseDrops, feeSettings.ReserveIncrementDrops,
		feeSettings.ReserveBase, feeSettings.ReserveIncrement, base, inc)
	return base, inc
}
