package payment

import (
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	tx "github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

// DirectStepI handles IOU transfers between two accounts via trust lines.
// This step does not use the order book - it directly transfers IOUs along
// existing trust lines between accounts.
//
// The step considers:
// - Trust line balance and limits
// - QualityIn/QualityOut settings on trust lines
// - Transfer fees when the source is issuing and previous step redeems
//
// Based on rippled's DirectStepI implementation.
type DirectStepI struct {
	// src is the source account sending IOUs
	src [20]byte

	// dst is the destination account receiving IOUs
	dst [20]byte

	// currency is the currency code being transferred
	currency string

	// prevStep is the previous step in the strand (for transfer fee calculation)
	prevStep Step

	// isLast indicates if this is the last step in the strand
	isLast bool

	// cache holds the results from the last Rev() call
	cache *directCache
}

// directCache holds cached values from the reverse pass
type directCache struct {
	in         tx.Amount
	srcToDst   tx.Amount // Amount transferred from src to dst
	out        tx.Amount
	srcDebtDir DebtDirection
}

// NewDirectStepI creates a new DirectStepI for IOU transfers
func NewDirectStepI(src, dst [20]byte, currency string, prevStep Step, isLast bool) *DirectStepI {
	return &DirectStepI{
		src:      src,
		dst:      dst,
		currency: currency,
		prevStep: prevStep,
		isLast:   isLast,
		cache:    nil,
	}
}

// Rev calculates the input needed to produce the requested output
func (s *DirectStepI) Rev(
	sb *PaymentSandbox,
	afView *PaymentSandbox,
	ofrsToRm map[[32]byte]bool,
	out EitherAmount,
) (EitherAmount, EitherAmount) {
	s.cache = nil

	if out.IsNative {
		// Should never happen - DirectStepI only handles IOU
		return ZeroIOUEitherAmount(s.currency, ""), ZeroIOUEitherAmount(s.currency, "")
	}

	// Get maximum flow and debt direction
	maxSrcToDst, srcDebtDir := s.maxPaymentFlow(sb)

	// Get qualities
	srcQOut, dstQIn := s.qualities(sb, srcDebtDir, StrandDirectionReverse)

	// Determine issuer for srcToDst
	var issuer string
	if Redeems(srcDebtDir) {
		issuer = sle.EncodeAccountIDSafe(s.dst)
	} else {
		issuer = sle.EncodeAccountIDSafe(s.src)
	}

	zeroAmt := tx.NewIssuedAmount(0, -100, s.currency, issuer)
	if maxSrcToDst.Compare(zeroAmt) <= 0 {
		// Dry - no liquidity
		s.cache = &directCache{
			in:         zeroAmt,
			srcToDst:   zeroAmt,
			out:        zeroAmt,
			srcDebtDir: srcDebtDir,
		}
		return ZeroIOUEitherAmount(s.currency, issuer), ZeroIOUEitherAmount(s.currency, issuer)
	}

	// Calculate srcToDst = out / dstQIn (round up)
	srcToDst := mulRatioAmount(out.IOU, QualityOne, dstQIn, true)

	if srcToDst.Compare(maxSrcToDst) <= 0 {
		// Non-limiting case
		in := mulRatioAmount(srcToDst, srcQOut, QualityOne, true)
		s.cache = &directCache{
			in:         in,
			srcToDst:   srcToDst,
			out:        out.IOU,
			srcDebtDir: srcDebtDir,
		}

		// Execute the credit
		s.rippleCredit(sb, srcToDst, issuer)

		return NewIOUEitherAmount(in), out
	}

	// Limiting case
	in := mulRatioAmount(maxSrcToDst, srcQOut, QualityOne, true)
	actualOut := mulRatioAmount(maxSrcToDst, dstQIn, QualityOne, false)

	s.cache = &directCache{
		in:         in,
		srcToDst:   maxSrcToDst,
		out:        actualOut,
		srcDebtDir: srcDebtDir,
	}

	// Execute the credit
	s.rippleCredit(sb, maxSrcToDst, issuer)

	return NewIOUEitherAmount(in), NewIOUEitherAmount(actualOut)
}

// Fwd executes the step with the given input
func (s *DirectStepI) Fwd(
	sb *PaymentSandbox,
	afView *PaymentSandbox,
	ofrsToRm map[[32]byte]bool,
	in EitherAmount,
) (EitherAmount, EitherAmount) {
	if s.cache == nil {
		return ZeroIOUEitherAmount(s.currency, ""), ZeroIOUEitherAmount(s.currency, "")
	}

	if in.IsNative {
		return ZeroIOUEitherAmount(s.currency, ""), ZeroIOUEitherAmount(s.currency, "")
	}

	// Get maximum flow and debt direction
	maxSrcToDst, srcDebtDir := s.maxPaymentFlow(sb)

	// Get qualities
	srcQOut, dstQIn := s.qualities(sb, srcDebtDir, StrandDirectionForward)

	// Determine issuer
	var issuer string
	if Redeems(srcDebtDir) {
		issuer = sle.EncodeAccountIDSafe(s.dst)
	} else {
		issuer = sle.EncodeAccountIDSafe(s.src)
	}

	zeroAmt := tx.NewIssuedAmount(0, -100, s.currency, issuer)
	if maxSrcToDst.Compare(zeroAmt) <= 0 {
		// Dry
		s.cache = &directCache{
			in:         zeroAmt,
			srcToDst:   zeroAmt,
			out:        zeroAmt,
			srcDebtDir: srcDebtDir,
		}
		return ZeroIOUEitherAmount(s.currency, issuer), ZeroIOUEitherAmount(s.currency, issuer)
	}

	// Calculate srcToDst = in / srcQOut (round down)
	srcToDst := mulRatioAmount(in.IOU, QualityOne, srcQOut, false)

	if srcToDst.Compare(maxSrcToDst) <= 0 {
		// Non-limiting case
		out := mulRatioAmount(srcToDst, dstQIn, QualityOne, false)
		s.setCacheLimiting(in.IOU, srcToDst, out, srcDebtDir)

		// Execute the credit
		s.rippleCredit(sb, s.cache.srcToDst, issuer)

		return NewIOUEitherAmount(s.cache.in), NewIOUEitherAmount(s.cache.out)
	}

	// Limiting case
	actualIn := mulRatioAmount(maxSrcToDst, srcQOut, QualityOne, true)
	out := mulRatioAmount(maxSrcToDst, dstQIn, QualityOne, false)
	s.setCacheLimiting(actualIn, maxSrcToDst, out, srcDebtDir)

	// Execute the credit
	s.rippleCredit(sb, s.cache.srcToDst, issuer)

	return NewIOUEitherAmount(s.cache.in), NewIOUEitherAmount(s.cache.out)
}

// setCacheLimiting updates the cache, keeping minimum values to prevent
// the forward pass from delivering more than the reverse pass
func (s *DirectStepI) setCacheLimiting(fwdIn, fwdSrcToDst, fwdOut tx.Amount, srcDebtDir DebtDirection) {
	if s.cache == nil {
		s.cache = &directCache{
			in:         fwdIn,
			srcToDst:   fwdSrcToDst,
			out:        fwdOut,
			srcDebtDir: srcDebtDir,
		}
		return
	}

	s.cache.in = fwdIn
	if fwdSrcToDst.Compare(s.cache.srcToDst) < 0 {
		s.cache.srcToDst = fwdSrcToDst
	}
	if fwdOut.Compare(s.cache.out) < 0 {
		s.cache.out = fwdOut
	}
	s.cache.srcDebtDir = srcDebtDir
}

// CachedIn returns the input from the last Rev() call
func (s *DirectStepI) CachedIn() *EitherAmount {
	if s.cache == nil {
		return nil
	}
	result := NewIOUEitherAmount(s.cache.in)
	return &result
}

// CachedOut returns the output from the last Rev() call
func (s *DirectStepI) CachedOut() *EitherAmount {
	if s.cache == nil {
		return nil
	}
	result := NewIOUEitherAmount(s.cache.out)
	return &result
}

// DebtDirection returns whether this step redeems or issues
func (s *DirectStepI) DebtDirection(sb *PaymentSandbox, dir StrandDirection) DebtDirection {
	if dir == StrandDirectionForward && s.cache != nil {
		return s.cache.srcDebtDir
	}

	// Check src's balance toward dst
	srcOwed := s.accountHolds(sb)
	zeroAmt := tx.NewIssuedAmount(0, -100, s.currency, "")
	if srcOwed.Compare(zeroAmt) > 0 {
		return DebtDirectionRedeems
	}
	return DebtDirectionIssues
}

// QualityUpperBound returns the worst-case quality for this step
func (s *DirectStepI) QualityUpperBound(v *PaymentSandbox, prevStepDir DebtDirection) (*Quality, DebtDirection) {
	srcDebtDir := s.DebtDirection(v, StrandDirectionForward)
	srcQOut, dstQIn := s.qualities(v, srcDebtDir, StrandDirectionForward)

	// Quality = srcQOut / dstQIn
	q := QualityFromAmounts(
		NewIOUEitherAmount(tx.NewIssuedAmount(1000000000000000, 0, "", "")),
		NewIOUEitherAmount(tx.NewIssuedAmount(1000000000000000, 0, "", "")),
	)
	q.Value = uint64((float64(srcQOut) / float64(dstQIn)) * float64(QualityOne))

	return &q, srcDebtDir
}

// IsZero returns true if the amount is zero
func (s *DirectStepI) IsZero(amt EitherAmount) bool {
	if amt.IsNative {
		return true // Native is effectively zero for IOU step
	}
	return amt.IOU.IsZero()
}

// EqualIn returns true if input portions are equal
func (s *DirectStepI) EqualIn(a, b EitherAmount) bool {
	if a.IsNative != b.IsNative {
		return false
	}
	if a.IsNative {
		return a.XRP == b.XRP
	}
	return a.IOU.Compare(b.IOU) == 0
}

// EqualOut returns true if output portions are equal
func (s *DirectStepI) EqualOut(a, b EitherAmount) bool {
	return s.EqualIn(a, b)
}

// Inactive returns false - DirectStepI doesn't become inactive
func (s *DirectStepI) Inactive() bool {
	return false
}

// OffersUsed returns 0 - DirectStepI doesn't use offers
func (s *DirectStepI) OffersUsed() uint32 {
	return 0
}

// DirectStepAccts returns (src, dst) accounts
func (s *DirectStepI) DirectStepAccts() *[2][20]byte {
	return &[2][20]byte{s.src, s.dst}
}

// BookStepBook returns nil - this is not a book step
func (s *DirectStepI) BookStepBook() *Book {
	return nil
}

// LineQualityIn returns the QualityIn for the dst's trust line
func (s *DirectStepI) LineQualityIn(v *PaymentSandbox) uint32 {
	return s.quality(v, true) // true = QualityDirection::in
}

// ValidFwd validates that the step can correctly execute in forward
func (s *DirectStepI) ValidFwd(sb *PaymentSandbox, afView *PaymentSandbox, in EitherAmount) (bool, EitherAmount) {
	if s.cache == nil {
		return false, ZeroIOUEitherAmount(s.currency, "")
	}

	if in.IsNative {
		return false, ZeroIOUEitherAmount(s.currency, "")
	}

	maxSrcToDst, _ := s.maxPaymentFlow(sb)

	if maxSrcToDst.Compare(s.cache.srcToDst) < 0 {
		// Exceeded max
		return false, NewIOUEitherAmount(s.cache.out)
	}

	return true, NewIOUEitherAmount(s.cache.out)
}

// maxPaymentFlow returns the maximum amount that can flow and the debt direction
func (s *DirectStepI) maxPaymentFlow(sb *PaymentSandbox) (tx.Amount, DebtDirection) {
	srcOwed := s.accountHolds(sb)
	zeroAmt := tx.NewIssuedAmount(0, -100, s.currency, "")

	if srcOwed.Compare(zeroAmt) > 0 {
		// Source holds IOUs from dst - they can redeem up to their balance
		return srcOwed, DebtDirectionRedeems
	}

	// Source has issued IOUs to dst (srcOwed is negative or zero)
	// They can issue up to (credit limit - what they've already issued)
	creditLimit := s.creditLimit(sb)
	// srcOwed is negative, so creditLimit + srcOwed = creditLimit - |srcOwed|
	maxIssue, _ := creditLimit.Add(srcOwed)
	return maxIssue, DebtDirectionIssues
}

// qualities returns srcQOut and dstQIn based on debt direction
func (s *DirectStepI) qualities(sb *PaymentSandbox, srcDebtDir DebtDirection, dir StrandDirection) (uint32, uint32) {
	if Redeems(srcDebtDir) {
		return s.qualitiesSrcRedeems(sb)
	}
	return s.qualitiesSrcIssues(sb, dir)
}

// qualitiesSrcRedeems returns qualities when source redeems
func (s *DirectStepI) qualitiesSrcRedeems(sb *PaymentSandbox) (uint32, uint32) {
	if s.prevStep == nil {
		return QualityOne, QualityOne
	}

	prevStepQIn := s.prevStep.LineQualityIn(sb)
	srcQOut := s.quality(sb, false) // QualityDirection::out

	if prevStepQIn > srcQOut {
		srcQOut = prevStepQIn
	}
	return srcQOut, QualityOne
}

// qualitiesSrcIssues returns qualities when source issues
func (s *DirectStepI) qualitiesSrcIssues(sb *PaymentSandbox, dir StrandDirection) (uint32, uint32) {
	// Charge transfer rate when issuing and previous step redeems
	var srcQOut uint32 = QualityOne

	if s.prevStep != nil {
		prevDebtDir := s.prevStep.DebtDirection(sb, dir)
		if Redeems(prevDebtDir) {
			// Get transfer rate from src account
			srcQOut = s.transferRate(sb)
		}
	}

	dstQIn := s.quality(sb, true) // QualityDirection::in

	// If this is the last step, cap dstQIn at QUALITY_ONE
	if s.isLast && dstQIn > QualityOne {
		dstQIn = QualityOne
	}

	return srcQOut, dstQIn
}

// quality returns the quality setting from the trust line
// isIn: true for QualityIn (dst's perspective), false for QualityOut (src's perspective)
func (s *DirectStepI) quality(sb *PaymentSandbox, isIn bool) uint32 {
	if s.src == s.dst {
		return QualityOne
	}

	// Read trust line
	trustLineKey := keylet.Line(s.dst, s.src, s.currency)
	data, err := sb.Read(trustLineKey)
	if err != nil || data == nil {
		return QualityOne
	}

	rs, err := sle.ParseRippleState(data)
	if err != nil {
		return QualityOne
	}

	if isIn {
		// QualityIn for dst
		if sle.CompareAccountIDs(s.dst, s.src) < 0 {
			if rs.LowQualityIn != 0 {
				return rs.LowQualityIn
			}
		} else {
			if rs.HighQualityIn != 0 {
				return rs.HighQualityIn
			}
		}
	} else {
		// QualityOut for src
		if sle.CompareAccountIDs(s.src, s.dst) < 0 {
			if rs.LowQualityOut != 0 {
				return rs.LowQualityOut
			}
		} else {
			if rs.HighQualityOut != 0 {
				return rs.HighQualityOut
			}
		}
	}

	return QualityOne
}

// accountHolds returns the balance src holds of dst's IOUs
func (s *DirectStepI) accountHolds(sb *PaymentSandbox) tx.Amount {
	trustLineKey := keylet.Line(s.src, s.dst, s.currency)
	data, err := sb.Read(trustLineKey)
	if err != nil || data == nil {
		return tx.NewIssuedAmount(0, -100, s.currency, "")
	}

	rs, err := sle.ParseRippleState(data)
	if err != nil {
		return tx.NewIssuedAmount(0, -100, s.currency, "")
	}

	// Balance is from low account's perspective (rippled convention):
	// Positive balance = LOW is OWED by HIGH (LOW has credit/can redeem from HIGH)
	// Negative balance = LOW OWES HIGH (LOW has debt to HIGH)
	balance := rs.Balance

	srcIsLow := sle.CompareAccountIDs(s.src, s.dst) < 0

	if srcIsLow {
		// src is low account
		return balance
	}
	// src is high account
	return balance.Negate()
}

// creditLimit returns the credit limit dst has extended to src
func (s *DirectStepI) creditLimit(sb *PaymentSandbox) tx.Amount {
	trustLineKey := keylet.Line(s.src, s.dst, s.currency)
	data, err := sb.Read(trustLineKey)
	if err != nil || data == nil {
		return tx.NewIssuedAmount(0, -100, s.currency, "")
	}

	rs, err := sle.ParseRippleState(data)
	if err != nil {
		return tx.NewIssuedAmount(0, -100, s.currency, "")
	}

	// Return dst's limit (what dst allows src to owe)
	if sle.CompareAccountIDs(s.dst, s.src) < 0 {
		// dst is low account
		return rs.LowLimit
	}
	// dst is high account
	return rs.HighLimit
}

// transferRate returns the transfer rate for src account
func (s *DirectStepI) transferRate(sb *PaymentSandbox) uint32 {
	accountKey := keylet.Account(s.src)
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

// rippleCredit transfers IOUs from src to dst in the sandbox
func (s *DirectStepI) rippleCredit(sb *PaymentSandbox, amount tx.Amount, issuer string) error {
	if amount.IsZero() {
		return nil
	}

	trustLineKey := keylet.Line(s.src, s.dst, s.currency)
	data, err := sb.Read(trustLineKey)
	if err != nil || data == nil {
		return err
	}

	rs, err := sle.ParseRippleState(data)
	if err != nil {
		return err
	}

	// Record pre-credit balance for deferred credits
	preCreditBalance := rs.Balance

	// Update balance
	// Balance is from low account's perspective:
	// - Positive balance = LOW is OWED by HIGH (HIGH owes LOW)
	// - Negative balance = LOW OWES HIGH (LOW has debt to HIGH)
	//
	// When src transfers to dst:
	// - If src is LOW and pays dst (HIGH): LOW's credit from HIGH decreases, so balance DECREASES
	// - If src is HIGH and pays dst (LOW): LOW gets credit from HIGH, so balance INCREASES
	if sle.CompareAccountIDs(s.src, s.dst) < 0 {
		// src is low, dst is high
		// LOW pays HIGH -> reduces what HIGH owes LOW -> balance decreases
		rs.Balance, _ = rs.Balance.Sub(amount)
	} else {
		// src is high, dst is low
		// HIGH pays LOW -> increases what HIGH owes LOW -> balance increases
		rs.Balance, _ = rs.Balance.Add(amount)
	}

	// Update PreviousTxnID and PreviousTxnLgrSeq
	txHash, ledgerSeq := sb.GetTransactionContext()
	if txHash != [32]byte{} {
		rs.PreviousTxnID = txHash
		rs.PreviousTxnLgrSeq = ledgerSeq
	}

	// Serialize and update
	newData, err := sle.SerializeRippleState(rs)
	if err != nil {
		return err
	}
	sb.Update(trustLineKey, newData)

	// Record deferred credit
	sb.Credit(s.src, s.dst, amount, preCreditBalance)

	return nil
}

// DirectStepSrcAcct returns the source account for NoRipple checking
// Reference: rippled Steps.h directStepSrcAcct()
func (s *DirectStepI) DirectStepSrcAcct() *[20]byte {
	return &s.src
}

// Check validates the DirectStepI before use
func (s *DirectStepI) Check(sb *PaymentSandbox) tx.Result {
	// Check trust line exists
	trustLineKey := keylet.Line(s.src, s.dst, s.currency)
	exists, err := sb.Exists(trustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	if !exists {
		return tx.TerNO_LINE
	}

	// Check freeze status
	// Reference: rippled StepChecks.h checkFreeze()
	if result := checkFreeze(sb, s.src, s.dst, s.currency); result != tx.TesSUCCESS {
		return result
	}

	// Check authorization
	// Reference: rippled DirectStep.cpp checkAuth()
	if result := checkAuth(sb, s.src, s.dst, s.currency); result != tx.TesSUCCESS {
		return result
	}

	return tx.TesSUCCESS
}

// checkAuth checks if the trust line is properly authorized when RequireAuth is set.
// Reference: rippled DirectStep.cpp checkAuth()
func checkAuth(view *PaymentSandbox, src, dst [20]byte, currency string) tx.Result {
	// Get the trust line
	trustLineKey := keylet.Line(src, dst, currency)
	data, err := view.Read(trustLineKey)
	if err != nil || data == nil {
		return tx.TerNO_LINE
	}

	rs, err := sle.ParseRippleState(data)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Determine which account is low/high
	srcIsLow := sle.CompareAccountIDs(src, dst) < 0

	// Check both sides for RequireAuth
	srcKey := keylet.Account(src)
	dstKey := keylet.Account(dst)

	srcData, _ := view.Read(srcKey)
	dstData, _ := view.Read(dstKey)

	// If src has RequireAuth, check if dst is authorized
	if srcData != nil {
		srcAccount, err := sle.ParseAccountRoot(srcData)
		if err == nil && (srcAccount.Flags&sle.LsfRequireAuth) != 0 {
			// src requires auth, check if dst (the other side) is authorized
			if srcIsLow {
				// src is low, dst is high. Check if high side is authorized by low (src)
				if (rs.Flags & sle.LsfHighAuth) == 0 {
					return tx.TerNO_AUTH
				}
			} else {
				// src is high, dst is low. Check if low side is authorized by high (src)
				if (rs.Flags & sle.LsfLowAuth) == 0 {
					return tx.TerNO_AUTH
				}
			}
		}
	}

	// If dst has RequireAuth, check if src is authorized
	if dstData != nil {
		dstAccount, err := sle.ParseAccountRoot(dstData)
		if err == nil && (dstAccount.Flags&sle.LsfRequireAuth) != 0 {
			// dst requires auth, check if src (the other side) is authorized
			if srcIsLow {
				// src is low, dst is high. Check if low side is authorized by high (dst)
				if (rs.Flags & sle.LsfLowAuth) == 0 {
					return tx.TerNO_AUTH
				}
			} else {
				// src is high, dst is low. Check if high side is authorized by low (dst)
				if (rs.Flags & sle.LsfHighAuth) == 0 {
					return tx.TerNO_AUTH
				}
			}
		}
	}

	return tx.TesSUCCESS
}

// checkFreeze checks if a trust line is frozen.
// Reference: rippled StepChecks.h checkFreeze()
// Returns terNO_LINE if the trust line is frozen, tesSUCCESS otherwise.
func checkFreeze(view *PaymentSandbox, src, dst [20]byte, currency string) tx.Result {
	// Get the trust line
	trustLineKey := keylet.Line(src, dst, currency)
	data, err := view.Read(trustLineKey)
	if err != nil || data == nil {
		return tx.TerNO_LINE
	}

	rs, err := sle.ParseRippleState(data)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Determine which account is low/high
	srcIsLow := sle.CompareAccountIDs(src, dst) < 0

	// Check individual freeze
	// If src is low, check if high (dst) has frozen the line
	// If src is high, check if low (dst) has frozen the line
	if srcIsLow {
		// dst is high, check if high has frozen
		if (rs.Flags & sle.LsfHighFreeze) != 0 {
			return tx.TerNO_LINE
		}
	} else {
		// dst is low, check if low has frozen
		if (rs.Flags & sle.LsfLowFreeze) != 0 {
			return tx.TerNO_LINE
		}
	}

	// Check global freeze on both accounts
	// Reference: rippled StepChecks.h:51-56
	srcKey := keylet.Account(src)
	dstKey := keylet.Account(dst)

	srcData, _ := view.Read(srcKey)
	dstData, _ := view.Read(dstKey)

	if srcData != nil {
		srcAccount, err := sle.ParseAccountRoot(srcData)
		if err == nil && (srcAccount.Flags&sle.LsfGlobalFreeze) != 0 {
			return tx.TerNO_LINE
		}
	}

	if dstData != nil {
		dstAccount, err := sle.ParseAccountRoot(dstData)
		if err == nil && (dstAccount.Flags&sle.LsfGlobalFreeze) != 0 {
			return tx.TerNO_LINE
		}
	}

	return tx.TesSUCCESS
}

// CheckWithPrevStep validates the DirectStepI with NoRipple checking against previous step.
// Reference: rippled DirectStep.cpp make_DirectStepI() lines 918-923
func (s *DirectStepI) CheckWithPrevStep(sb *PaymentSandbox, prevStep Step) tx.Result {
	// First do basic check
	if result := s.Check(sb); result != tx.TesSUCCESS {
		return result
	}

	// If there's a previous DirectStep, check NoRipple constraint
	// Reference: rippled DirectStep.cpp:918-923
	if prevStep != nil {
		if prevDirectStep, ok := prevStep.(*DirectStepI); ok {
			prevSrc := prevDirectStep.src
			result := checkNoRipple(sb, prevSrc, s.src, s.dst, s.currency)
			if result != tx.TesSUCCESS {
				return result
			}
		}
	}

	return tx.TesSUCCESS
}

// checkNoRipple checks if the middle account (cur) has NoRipple set on both sides.
// Reference: rippled StepChecks.h checkNoRipple()
func checkNoRipple(view *PaymentSandbox, prev, cur, next [20]byte, currency string) tx.Result {
	// Fetch the ripple lines into and out of this node
	sleInKey := keylet.Line(prev, cur, currency)
	sleOutKey := keylet.Line(cur, next, currency)

	sleInData, err := view.Read(sleInKey)
	if err != nil || sleInData == nil {
		return tx.TerNO_LINE
	}

	sleOutData, err := view.Read(sleOutKey)
	if err != nil || sleOutData == nil {
		return tx.TerNO_LINE
	}

	sleIn, err := sle.ParseRippleState(sleInData)
	if err != nil {
		return tx.TefINTERNAL
	}

	sleOut, err := sle.ParseRippleState(sleOutData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Check NoRipple flags
	// Reference: rippled StepChecks.h:105-106
	// The flag to check depends on account ordering in the trust line
	curIsHighIn := sle.CompareAccountIDs(cur, prev) > 0
	curIsHighOut := sle.CompareAccountIDs(cur, next) > 0

	var noRippleIn, noRippleOut bool
	if curIsHighIn {
		noRippleIn = (sleIn.Flags & sle.LsfHighNoRipple) != 0
	} else {
		noRippleIn = (sleIn.Flags & sle.LsfLowNoRipple) != 0
	}

	if curIsHighOut {
		noRippleOut = (sleOut.Flags & sle.LsfHighNoRipple) != 0
	} else {
		noRippleOut = (sleOut.Flags & sle.LsfLowNoRipple) != 0
	}

	// If BOTH sides have NoRipple set, return terNO_RIPPLE
	if noRippleIn && noRippleOut {
		return tx.TerNO_RIPPLE
	}

	return tx.TesSUCCESS
}

// mulRatioAmount multiplies an Amount by num/den
func mulRatioAmount(amt tx.Amount, num, den uint32, roundUp bool) tx.Amount {
	return amt.MulRatio(num, den, roundUp)
}
