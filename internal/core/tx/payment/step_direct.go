package payment

import (
	"fmt"
	"math/big"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	tx "github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

// DebugDirectStep enables debug output for DirectStepI
var DebugDirectStep = false

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
	in         sle.IOUAmount
	srcToDst   sle.IOUAmount // Amount transferred from src to dst
	out        sle.IOUAmount
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

	if DebugDirectStep {
		srcStr, _ := sle.EncodeAccountID(s.src)
		dstStr, _ := sle.EncodeAccountID(s.dst)
		fmt.Printf("DEBUG DirectStepI.Rev: %s -> %s currency=%s out=%+v\n", srcStr, dstStr, s.currency, out)
	}

	if out.IsNative {
		// Should never happen - DirectStepI only handles IOU
		if DebugDirectStep {
			fmt.Printf("DEBUG DirectStepI.Rev: out is native (unexpected)\n")
		}
		return ZeroIOUEitherAmount(s.currency, ""), ZeroIOUEitherAmount(s.currency, "")
	}

	// Get maximum flow and debt direction
	maxSrcToDst, srcDebtDir := s.maxPaymentFlow(sb)
	if DebugDirectStep {
		fmt.Printf("DEBUG DirectStepI.Rev: maxSrcToDst=%+v srcDebtDir=%d\n", maxSrcToDst, srcDebtDir)
	}

	// Get qualities
	srcQOut, dstQIn := s.qualities(sb, srcDebtDir, StrandDirectionReverse)

	// Determine issuer for srcToDst
	var issuer string
	if Redeems(srcDebtDir) {
		issuer = sle.EncodeAccountIDSafe(s.dst)
	} else {
		issuer = sle.EncodeAccountIDSafe(s.src)
	}

	if maxSrcToDst.Compare(sle.NewIOUAmount("0", s.currency, issuer)) <= 0 {
		// Dry - no liquidity
		s.cache = &directCache{
			in:         sle.NewIOUAmount("0", s.currency, issuer),
			srcToDst:   sle.NewIOUAmount("0", s.currency, issuer),
			out:        sle.NewIOUAmount("0", s.currency, issuer),
			srcDebtDir: srcDebtDir,
		}
		return ZeroIOUEitherAmount(s.currency, issuer), ZeroIOUEitherAmount(s.currency, issuer)
	}

	// Calculate srcToDst = out / dstQIn (round up)
	srcToDst := mulRatioIOU(out.IOU, QualityOne, dstQIn, true)

	if srcToDst.Compare(maxSrcToDst) <= 0 {
		// Non-limiting case
		in := mulRatioIOU(srcToDst, srcQOut, QualityOne, true)
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
	in := mulRatioIOU(maxSrcToDst, srcQOut, QualityOne, true)
	actualOut := mulRatioIOU(maxSrcToDst, dstQIn, QualityOne, false)

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

	if maxSrcToDst.Compare(sle.NewIOUAmount("0", s.currency, issuer)) <= 0 {
		// Dry
		s.cache = &directCache{
			in:         sle.NewIOUAmount("0", s.currency, issuer),
			srcToDst:   sle.NewIOUAmount("0", s.currency, issuer),
			out:        sle.NewIOUAmount("0", s.currency, issuer),
			srcDebtDir: srcDebtDir,
		}
		return ZeroIOUEitherAmount(s.currency, issuer), ZeroIOUEitherAmount(s.currency, issuer)
	}

	// Calculate srcToDst = in / srcQOut (round down)
	srcToDst := mulRatioIOU(in.IOU, QualityOne, srcQOut, false)

	if srcToDst.Compare(maxSrcToDst) <= 0 {
		// Non-limiting case
		out := mulRatioIOU(srcToDst, dstQIn, QualityOne, false)
		s.setCacheLimiting(in.IOU, srcToDst, out, srcDebtDir)

		// Execute the credit
		s.rippleCredit(sb, s.cache.srcToDst, issuer)

		return NewIOUEitherAmount(s.cache.in), NewIOUEitherAmount(s.cache.out)
	}

	// Limiting case
	actualIn := mulRatioIOU(maxSrcToDst, srcQOut, QualityOne, true)
	out := mulRatioIOU(maxSrcToDst, dstQIn, QualityOne, false)
	s.setCacheLimiting(actualIn, maxSrcToDst, out, srcDebtDir)

	// Execute the credit
	s.rippleCredit(sb, s.cache.srcToDst, issuer)

	return NewIOUEitherAmount(s.cache.in), NewIOUEitherAmount(s.cache.out)
}

// setCacheLimiting updates the cache, keeping minimum values to prevent
// the forward pass from delivering more than the reverse pass
func (s *DirectStepI) setCacheLimiting(fwdIn, fwdSrcToDst, fwdOut sle.IOUAmount, srcDebtDir DebtDirection) {
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
	if srcOwed.Compare(sle.NewIOUAmount("0", s.currency, "")) > 0 {
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
		NewIOUEitherAmount(sle.NewIOUAmount("1", "", "")),
		NewIOUEitherAmount(sle.NewIOUAmount("1", "", "")),
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
func (s *DirectStepI) maxPaymentFlow(sb *PaymentSandbox) (sle.IOUAmount, DebtDirection) {
	srcOwed := s.accountHolds(sb)

	if srcOwed.Compare(sle.NewIOUAmount("0", s.currency, "")) > 0 {
		// Source holds IOUs from dst - they can redeem up to their balance
		return srcOwed, DebtDirectionRedeems
	}

	// Source has issued IOUs to dst (srcOwed is negative or zero)
	// They can issue up to (credit limit - what they've already issued)
	creditLimit := s.creditLimit(sb)
	// srcOwed is negative, so creditLimit + srcOwed = creditLimit - |srcOwed|
	maxIssue := creditLimit.Add(srcOwed)
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
func (s *DirectStepI) accountHolds(sb *PaymentSandbox) sle.IOUAmount {
	trustLineKey := keylet.Line(s.src, s.dst, s.currency)
	if DebugDirectStep {
		srcStr, _ := sle.EncodeAccountID(s.src)
		dstStr, _ := sle.EncodeAccountID(s.dst)
		fmt.Printf("DEBUG DirectStepI.accountHolds: looking up trust line %s <-> %s currency=%s key=%x\n", srcStr, dstStr, s.currency, trustLineKey.Key)
	}
	data, err := sb.Read(trustLineKey)
	if err != nil || data == nil {
		if DebugDirectStep {
			fmt.Printf("DEBUG DirectStepI.accountHolds: trust line not found, err=%v\n", err)
		}
		return sle.NewIOUAmount("0", s.currency, "")
	}

	rs, err := sle.ParseRippleState(data)
	if err != nil {
		return sle.NewIOUAmount("0", s.currency, "")
	}

	// Balance is from low account's perspective (rippled convention):
	// Positive balance = LOW is OWED by HIGH (LOW has credit/can redeem from HIGH)
	// Negative balance = LOW OWES HIGH (LOW has debt to HIGH)
	balance := rs.Balance

	if DebugDirectStep {
		fmt.Printf("DEBUG DirectStepI.accountHolds: raw balance=%+v\n", balance)
	}

	if sle.CompareAccountIDs(s.src, s.dst) < 0 {
		// src is low account
		// If balance > 0, LOW is owed by HIGH -> src holds +balance
		// If balance < 0, LOW owes HIGH -> src holds balance (negative, they owe)
		if DebugDirectStep {
			fmt.Printf("DEBUG DirectStepI.accountHolds: src is LOW, returning balance=%+v\n", balance)
		}
		return balance
	}
	// src is high account
	// If balance > 0, LOW is owed by HIGH -> HIGH owes, so src holds -balance
	// If balance < 0, LOW owes HIGH -> HIGH is owed, so src holds -balance (positive result)
	result := balance.Negate()
	if DebugDirectStep {
		fmt.Printf("DEBUG DirectStepI.accountHolds: src is HIGH, returning negated balance=%+v\n", result)
	}
	return result
}

// creditLimit returns the credit limit dst has extended to src
func (s *DirectStepI) creditLimit(sb *PaymentSandbox) sle.IOUAmount {
	trustLineKey := keylet.Line(s.src, s.dst, s.currency)
	data, err := sb.Read(trustLineKey)
	if err != nil || data == nil {
		return sle.NewIOUAmount("0", s.currency, "")
	}

	rs, err := sle.ParseRippleState(data)
	if err != nil {
		return sle.NewIOUAmount("0", s.currency, "")
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
func (s *DirectStepI) rippleCredit(sb *PaymentSandbox, amount sle.IOUAmount, issuer string) error {
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
		rs.Balance = rs.Balance.Sub(amount)
	} else {
		// src is high, dst is low
		// HIGH pays LOW -> increases what HIGH owes LOW -> balance increases
		rs.Balance = rs.Balance.Add(amount)
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

	return tx.TesSUCCESS
}

// mulRatioIOU multiplies an IOU amount by num/den
func mulRatioIOU(amt sle.IOUAmount, num, den uint32, roundUp bool) sle.IOUAmount {
	if den == 0 || amt.Value == nil {
		return amt
	}

	numF := new(big.Float).SetUint64(uint64(num))
	denF := new(big.Float).SetUint64(uint64(den))
	ratio := new(big.Float).Quo(numF, denF)
	result := new(big.Float).Mul(amt.Value, ratio)

	// Note: For proper rounding, more complex logic would be needed
	// This is a simplified implementation

	return sle.IOUAmount{
		Value:    result,
		Currency: amt.Currency,
		Issuer:   amt.Issuer,
	}
}

// Note: CompareAccountIDs and EncodeAccountID are now accessed via the sle package
