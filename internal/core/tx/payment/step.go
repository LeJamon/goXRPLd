package payment

import (
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"

	tx "github.com/LeJamon/goXRPLd/internal/core/tx"
)

// DebtDirection indicates whether a step is issuing or redeeming currency
type DebtDirection int

const (
	// DebtDirectionIssues means the step is creating new debt (issuing)
	DebtDirectionIssues DebtDirection = iota
	// DebtDirectionRedeems means the step is reducing existing debt (redeeming)
	DebtDirectionRedeems
)

// Redeems returns true if the direction is redeeming
func Redeems(dir DebtDirection) bool {
	return dir == DebtDirectionRedeems
}

// Issues returns true if the direction is issuing
func Issues(dir DebtDirection) bool {
	return dir == DebtDirectionIssues
}

// StrandDirection indicates the direction of flow through a strand
type StrandDirection int

const (
	// StrandDirectionForward means executing from source to destination
	StrandDirectionForward StrandDirection = iota
	// StrandDirectionReverse means calculating from destination back to source
	StrandDirectionReverse
)

// QualityOne is the standard quality representing 1:1 ratio (1 billion)
const QualityOne uint32 = 1_000_000_000

// EitherAmount holds either an XRP amount (in drops) or an IOU amount
// This allows unified handling in the flow algorithm regardless of currency type
type EitherAmount struct {
	// IsNative is true if this is an XRP amount, false for IOU
	IsNative bool

	// XRP holds the amount in drops (only valid if IsNative is true)
	XRP int64

	// IOU holds the IOU amount (only valid if IsNative is false)
	IOU tx.Amount
}

// NewXRPEitherAmount creates an EitherAmount for XRP
func NewXRPEitherAmount(drops int64) EitherAmount {
	return EitherAmount{
		IsNative: true,
		XRP:      drops,
	}
}

// NewIOUEitherAmount creates an EitherAmount for IOU
func NewIOUEitherAmount(amount tx.Amount) EitherAmount {
	return EitherAmount{
		IsNative: false,
		IOU:      amount,
	}
}

// ZeroXRPEitherAmount creates a zero XRP EitherAmount
func ZeroXRPEitherAmount() EitherAmount {
	return EitherAmount{
		IsNative: true,
		XRP:      0,
	}
}

// ZeroIOUEitherAmount creates a zero IOU EitherAmount with the given currency/issuer
func ZeroIOUEitherAmount(currency, issuer string) EitherAmount {
	return EitherAmount{
		IsNative: false,
		IOU:      tx.NewIssuedAmount(0, -100, currency, issuer),
	}
}

// IsZero returns true if the amount is zero
func (e EitherAmount) IsZero() bool {
	if e.IsNative {
		return e.XRP == 0
	}
	return e.IOU.IsZero()
}

// IsEffectivelyZero returns true if the amount is effectively zero
func (e EitherAmount) IsEffectivelyZero() bool {
	if e.IsNative {
		return e.XRP == 0
	}
	return e.IOU.IsZero()
}

// IsNegative returns true if the amount is negative
func (e EitherAmount) IsNegative() bool {
	if e.IsNative {
		return e.XRP < 0
	}
	return e.IOU.IsNegative()
}

// Add adds two EitherAmounts (must be same type - both XRP or both IOU)
func (e EitherAmount) Add(other EitherAmount) EitherAmount {
	if e.IsNative {
		return NewXRPEitherAmount(e.XRP + other.XRP)
	}
	result, _ := e.IOU.Add(other.IOU)
	return NewIOUEitherAmount(result)
}

// Sub subtracts other from e (must be same type)
func (e EitherAmount) Sub(other EitherAmount) EitherAmount {
	if e.IsNative {
		return NewXRPEitherAmount(e.XRP - other.XRP)
	}
	result, _ := e.IOU.Sub(other.IOU)
	return NewIOUEitherAmount(result)
}

// Compare compares two EitherAmounts
// Returns -1 if e < other, 0 if equal, 1 if e > other
func (e EitherAmount) Compare(other EitherAmount) int {
	if e.IsNative {
		if e.XRP < other.XRP {
			return -1
		}
		if e.XRP > other.XRP {
			return 1
		}
		return 0
	}
	return e.IOU.Compare(other.IOU)
}

// Min returns the minimum of two EitherAmounts
func (e EitherAmount) Min(other EitherAmount) EitherAmount {
	if e.Compare(other) <= 0 {
		return e
	}
	return other
}

// Max returns the maximum of two EitherAmounts
func (e EitherAmount) Max(other EitherAmount) EitherAmount {
	if e.Compare(other) >= 0 {
		return e
	}
	return other
}

// DivideFloat divides this amount by another, returning a float64 ratio
func (e EitherAmount) DivideFloat(other EitherAmount) float64 {
	var eVal, otherVal float64
	if e.IsNative {
		eVal = float64(e.XRP)
	} else {
		eVal = e.IOU.Float64()
	}
	if other.IsNative {
		otherVal = float64(other.XRP)
	} else {
		otherVal = other.IOU.Float64()
	}
	if otherVal == 0 {
		return 0
	}
	return eVal / otherVal
}

// MultiplyFloat multiplies this amount by a float64 factor
func (e EitherAmount) MultiplyFloat(factor float64) EitherAmount {
	if e.IsNative {
		return NewXRPEitherAmount(int64(float64(e.XRP) * factor))
	}
	val := e.IOU.Float64()
	newVal := val * factor
	return NewIOUEitherAmount(tx.NewIssuedAmountFromFloat64(newVal, e.IOU.Currency, e.IOU.Issuer))
}

// Quality represents an exchange rate as output/input ratio.
// Internally stored as a uint64 where higher values represent lower quality
// (worse exchange rate for the taker).
//
// Quality is computed as: in/out, so lower quality means better deal
// (less input required for the same output).
type Quality struct {
	// Value is the encoded quality (same representation as STAmount)
	Value uint64
}

// QualityFromAmounts creates a Quality from input and output amounts.
// Quality = in / out, encoded using STAmount-like floating point representation.
// Reference: rippled's getRate(offerOut, offerIn) in STAmount.cpp divides offerIn / offerOut.
// Despite the parameter order (out, in), it returns in / out.
// Lower quality value means you pay less per unit received (better for taker).
func QualityFromAmounts(in, out EitherAmount) Quality {
	if out.IsZero() {
		return Quality{Value: 0}
	}

	if in.IsZero() {
		return Quality{Value: 0}
	}

	var inVal, outVal float64
	if in.IsNative {
		inVal = float64(in.XRP)
	} else {
		inVal = in.IOU.Float64()
	}
	if out.IsNative {
		outVal = float64(out.XRP)
	} else {
		outVal = out.IOU.Float64()
	}

	if outVal == 0 {
		return Quality{Value: 0}
	}

	// Quality = in / out (matching rippled's getRate which does offerIn / offerOut)
	f64 := inVal / outVal
	if f64 <= 0 {
		return Quality{Value: 0}
	}

	// Calculate exponent and mantissa
	// We want mantissa in range [10^15, 10^16)
	exponent := 0
	mantissa := f64

	// Normalize: scale mantissa to [10^15, 10^16)
	minMantissa := 1e15
	maxMantissa := 1e16

	if mantissa != 0 {
		for mantissa < minMantissa {
			mantissa *= 10
			exponent--
		}
		for mantissa >= maxMantissa {
			mantissa /= 10
			exponent++
		}
	}

	// Clamp exponent to valid range [-100, 155]
	if exponent < -100 {
		return Quality{Value: 0}
	}
	if exponent > 155 {
		return Quality{Value: ^uint64(0)}
	}

	storedExponent := uint64(exponent + 100)
	storedMantissa := uint64(mantissa)

	return Quality{Value: (storedExponent << 56) | (storedMantissa & 0x00FFFFFFFFFFFFFF)}
}

// Compare compares two qualities
// Returns -1 if q < other (better), 0 if equal, 1 if q > other (worse)
func (q Quality) Compare(other Quality) int {
	if q.Value < other.Value {
		return -1
	}
	if q.Value > other.Value {
		return 1
	}
	return 0
}

// BetterThan returns true if q is better quality than other
// Lower value = better quality (less input for same output)
func (q Quality) BetterThan(other Quality) bool {
	return q.Value < other.Value
}

// WorseThan returns true if q is worse quality than other
func (q Quality) WorseThan(other Quality) bool {
	return q.Value > other.Value
}

// Increment returns a Quality that is slightly worse (higher value).
// This is used for passive offers where we only want to cross against
// offers with STRICTLY better quality.
// Reference: rippled CreateOffer.cpp line 364: ++threshold
func (q Quality) Increment() Quality {
	if q.Value == ^uint64(0) {
		return q // Already at max, can't increment
	}
	return Quality{Value: q.Value + 1}
}

// Float64 decodes the quality value to a float64 ratio (in/out).
// The quality is stored in STAmount format: top 8 bits = exponent+100, lower 56 bits = mantissa.
// Reference: rippled's amountFromQuality() in STAmount.cpp
func (q Quality) Float64() float64 {
	if q.Value == 0 {
		return 0
	}
	mantissa := q.Value & 0x00FFFFFFFFFFFFFF
	exponent := int((q.Value >> 56)) - 100

	// The encoding already normalized mantissa to [10^15, 10^16) and adjusted
	// exponent accordingly, so we just decode directly: value = mantissa * 10^exponent
	return float64(mantissa) * pow10(exponent)
}

// Rate returns the quality rate as an Amount for precise arithmetic.
// This is equivalent to rippled's quality.rate() which returns an STAmount.
// Reference: rippled's amountFromQuality() in STAmount.cpp
func (q Quality) Rate() tx.Amount {
	if q.Value == 0 {
		return tx.NewIssuedAmount(0, -100, "", "")
	}
	mantissa := int64(q.Value & 0x00FFFFFFFFFFFFFF)
	exponent := int((q.Value >> 56)) - 100
	result := tx.NewIssuedAmount(mantissa, exponent, "", "")
	return result
}

// pow10 returns 10^n for small n values
func pow10(n int) float64 {
	if n == 0 {
		return 1
	}
	if n > 0 {
		result := 1.0
		for i := 0; i < n; i++ {
			result *= 10
		}
		return result
	}
	// n < 0
	result := 1.0
	for i := 0; i > n; i-- {
		result /= 10
	}
	return result
}

// Compose multiplies two qualities together
func (q Quality) Compose(other Quality) Quality {
	// Get the actual rates
	rate1 := q.Float64()
	rate2 := other.Float64()
	composedRate := rate1 * rate2

	// Encode back to quality format
	return qualityFromFloat64(composedRate)
}

// qualityFromFloat64 encodes a float64 rate back to Quality format
func qualityFromFloat64(rate float64) Quality {
	if rate <= 0 {
		return Quality{Value: 0}
	}

	// Normalize mantissa to [10^15, 10^16)
	exponent := 0
	mantissa := rate

	minMantissa := 1e15
	maxMantissa := 1e16

	if mantissa != 0 {
		for mantissa < minMantissa {
			mantissa *= 10
			exponent--
		}
		for mantissa >= maxMantissa {
			mantissa /= 10
			exponent++
		}
	}

	// Clamp exponent
	if exponent < -100 {
		return Quality{Value: 0}
	}
	if exponent > 155 {
		return Quality{Value: ^uint64(0)}
	}

	storedExponent := uint64(exponent + 100)
	storedMantissa := uint64(mantissa)

	return Quality{Value: (storedExponent << 56) | (storedMantissa & 0x00FFFFFFFFFFFFFF)}
}

// Issue represents a currency/issuer pair
type Issue struct {
	Currency string
	Issuer   [20]byte
}

// IsXRP returns true if this issue represents XRP
func (i Issue) IsXRP() bool {
	return i.Currency == "XRP" || i.Currency == ""
}

// Book represents an order book (input/output issue pair)
type Book struct {
	In  Issue
	Out Issue
}

// Strand is a sequence of Steps forming a complete payment path
type Strand []Step

// Step is a single unit of payment flow in a strand.
// Steps transform amounts from one currency/account to another.
type Step interface {
	Rev(sb *PaymentSandbox, afView *PaymentSandbox, ofrsToRm map[[32]byte]bool, out EitherAmount) (EitherAmount, EitherAmount)
	Fwd(sb *PaymentSandbox, afView *PaymentSandbox, ofrsToRm map[[32]byte]bool, in EitherAmount) (EitherAmount, EitherAmount)
	CachedIn() *EitherAmount
	CachedOut() *EitherAmount
	DebtDirection(sb *PaymentSandbox, dir StrandDirection) DebtDirection
	QualityUpperBound(v *PaymentSandbox, prevStepDir DebtDirection) (*Quality, DebtDirection)
	IsZero(amt EitherAmount) bool
	EqualIn(a, b EitherAmount) bool
	EqualOut(a, b EitherAmount) bool
	Inactive() bool
	OffersUsed() uint32
	DirectStepAccts() *[2][20]byte
	BookStepBook() *Book
	LineQualityIn(v *PaymentSandbox) uint32
	ValidFwd(sb *PaymentSandbox, afView *PaymentSandbox, in EitherAmount) (bool, EitherAmount)
}

// StrandResult captures the outcome of executing a single strand
type StrandResult struct {
	Success    bool
	In         EitherAmount
	Out        EitherAmount
	Sandbox    *PaymentSandbox
	OffsToRm   map[[32]byte]bool
	OffersUsed uint32
	Inactive   bool
}

// FlowResult captures the overall result of payment flow execution
type FlowResult struct {
	In              EitherAmount
	Out             EitherAmount
	Sandbox         *PaymentSandbox
	RemovableOffers map[[32]byte]bool
	Result          tx.Result
}

// ToEitherAmount converts a tx.Amount to EitherAmount
func ToEitherAmount(amt tx.Amount) EitherAmount {
	if amt.IsNative() {
		return NewXRPEitherAmount(amt.Drops())
	}
	return NewIOUEitherAmount(amt)
}

// FromEitherAmount converts EitherAmount back to tx.Amount
func FromEitherAmount(e EitherAmount) tx.Amount {
	if e.IsNative {
		return tx.NewXRPAmount(e.XRP)
	}
	return e.IOU
}

// GetIssue extracts the Issue from a tx.Amount
func GetIssue(amt tx.Amount) Issue {
	if amt.IsNative() {
		return Issue{Currency: "XRP"}
	}

	var issuerBytes [20]byte
	if issuerID, err := sle.DecodeAccountID(amt.Issuer); err == nil {
		issuerBytes = issuerID
	}

	return Issue{
		Currency: amt.Currency,
		Issuer:   issuerBytes,
	}
}

// MulRatio multiplies an amount by a ratio (num/den)
func MulRatio(amt EitherAmount, num, den uint32, roundUp bool) EitherAmount {
	if den == 0 {
		return amt
	}

	if amt.IsNative {
		result := (int64(amt.XRP) * int64(num)) / int64(den)
		if roundUp && (int64(amt.XRP)*int64(num))%int64(den) != 0 {
			result++
		}
		return NewXRPEitherAmount(result)
	}

	return NewIOUEitherAmount(amt.IOU.MulRatio(num, den, roundUp))
}

// DivRatio divides an amount by a ratio (num/den) = amt * den / num
func DivRatio(amt EitherAmount, num, den uint32, roundUp bool) EitherAmount {
	if num == 0 {
		return amt
	}
	return MulRatio(amt, den, num, roundUp)
}
