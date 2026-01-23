package tx

import (
	"math/big"
	"strconv"
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
	IOU IOUAmount
}

// NewXRPEitherAmount creates an EitherAmount for XRP
func NewXRPEitherAmount(drops int64) EitherAmount {
	return EitherAmount{
		IsNative: true,
		XRP:      drops,
	}
}

// NewIOUEitherAmount creates an EitherAmount for IOU
func NewIOUEitherAmount(amount IOUAmount) EitherAmount {
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
		IOU:      NewIOUAmount("0", currency, issuer),
	}
}

// IsZero returns true if the amount is zero
func (e EitherAmount) IsZero() bool {
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
	return NewIOUEitherAmount(e.IOU.Add(other.IOU))
}

// Sub subtracts other from e (must be same type)
func (e EitherAmount) Sub(other EitherAmount) EitherAmount {
	if e.IsNative {
		return NewXRPEitherAmount(e.XRP - other.XRP)
	}
	return NewIOUEitherAmount(e.IOU.Sub(other.IOU))
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

// QualityFromAmounts creates a Quality from input and output amounts
func QualityFromAmounts(in, out EitherAmount) Quality {
	// Quality = in / out
	// For now, use a simplified calculation
	// In production, this should match rippled's exact encoding

	if out.IsZero() {
		return Quality{Value: 0} // Invalid quality
	}

	var ratio *big.Float
	if in.IsNative && out.IsNative {
		// XRP to XRP
		ratio = new(big.Float).Quo(
			new(big.Float).SetInt64(in.XRP),
			new(big.Float).SetInt64(out.XRP),
		)
	} else if in.IsNative && !out.IsNative {
		// XRP to IOU
		inFloat := new(big.Float).SetInt64(in.XRP)
		ratio = new(big.Float).Quo(inFloat, out.IOU.Value)
	} else if !in.IsNative && out.IsNative {
		// IOU to XRP
		outFloat := new(big.Float).SetInt64(out.XRP)
		ratio = new(big.Float).Quo(in.IOU.Value, outFloat)
	} else {
		// IOU to IOU
		ratio = new(big.Float).Quo(in.IOU.Value, out.IOU.Value)
	}

	// Encode as uint64 - simplified version
	// In production, should use STAmount encoding
	f64, _ := ratio.Float64()
	if f64 <= 0 {
		return Quality{Value: 0}
	}

	// Scale to preserve precision
	scaled := f64 * float64(QualityOne)
	return Quality{Value: uint64(scaled)}
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

// Compose multiplies two qualities together
func (q Quality) Compose(other Quality) Quality {
	// Simplified multiplication - in production use proper fixed-point math
	product := (float64(q.Value) * float64(other.Value)) / float64(QualityOne)
	return Quality{Value: uint64(product)}
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
//
// The interface supports two modes of operation:
// - Rev (reverse): Given desired output, calculate required input
// - Fwd (forward): Given input, calculate and execute actual output
type Step interface {
	// Rev calculates the input needed to produce the requested output.
	// This is called during the reverse pass of strand execution.
	//
	// Parameters:
	//   sb: PaymentSandbox with strand's state of balances/offers
	//   afView: View of balances before strand runs (for unfunded offer detection)
	//   ofrsToRm: Set to collect unfunded/errored offers for removal
	//   out: Requested output amount
	//
	// Returns: (actualInput, actualOutput) - may be less than requested if liquidity limited
	Rev(sb *PaymentSandbox, afView *PaymentSandbox, ofrsToRm map[[32]byte]bool, out EitherAmount) (EitherAmount, EitherAmount)

	// Fwd calculates and executes the output given input.
	// This is called during the forward pass of strand execution.
	//
	// Parameters:
	//   sb: PaymentSandbox with strand's state
	//   afView: View of balances before strand runs
	//   ofrsToRm: Set to collect unfunded offers
	//   in: Input amount to process
	//
	// Returns: (actualInput, actualOutput)
	Fwd(sb *PaymentSandbox, afView *PaymentSandbox, ofrsToRm map[[32]byte]bool, in EitherAmount) (EitherAmount, EitherAmount)

	// CachedIn returns the input amount from the last Rev() call
	CachedIn() *EitherAmount

	// CachedOut returns the output amount from the last Rev() call
	CachedOut() *EitherAmount

	// DebtDirection returns whether this step is issuing or redeeming
	// based on current balances and flow direction
	DebtDirection(sb *PaymentSandbox, dir StrandDirection) DebtDirection

	// QualityUpperBound returns the worst-case quality for this step
	// and indicates whether this step redeems or issues.
	// Returns (nil, _) if the step is dry (no liquidity).
	QualityUpperBound(v *PaymentSandbox, prevStepDir DebtDirection) (*Quality, DebtDirection)

	// IsZero returns true if the given amount is effectively zero for this step
	IsZero(amt EitherAmount) bool

	// EqualIn returns true if the input portions of two amounts are equal
	EqualIn(a, b EitherAmount) bool

	// EqualOut returns true if the output portions of two amounts are equal
	EqualOut(a, b EitherAmount) bool

	// Inactive returns true if this step should not be used again
	// (e.g., consumed too many offers)
	Inactive() bool

	// OffersUsed returns the number of offers consumed in the last execution
	OffersUsed() uint32

	// DirectStepAccts returns the (src, dst) accounts if this is a DirectStep,
	// or nil if this is a BookStep
	DirectStepAccts() *[2][20]byte

	// BookStepBook returns the Book if this is a BookStep, or nil for DirectStep
	BookStepBook() *Book

	// LineQualityIn returns the QualityIn for the destination's trust line
	// (only meaningful for DirectStep, returns QualityOne for others)
	LineQualityIn(v *PaymentSandbox) uint32

	// ValidFwd validates that the step can correctly execute in forward direction
	// Returns (valid, output) where valid is true if step executed correctly
	ValidFwd(sb *PaymentSandbox, afView *PaymentSandbox, in EitherAmount) (bool, EitherAmount)
}

// StrandResult captures the outcome of executing a single strand
type StrandResult struct {
	// Success indicates whether the strand executed successfully
	Success bool

	// In is the total input consumed
	In EitherAmount

	// Out is the total output produced
	Out EitherAmount

	// Sandbox contains state changes (nil on failure)
	Sandbox *PaymentSandbox

	// OffsToRm contains offer hashes that should be removed (unfunded/expired)
	OffsToRm map[[32]byte]bool

	// OffersUsed is the total number of offers consumed
	OffersUsed uint32

	// Inactive indicates the strand is depleted of liquidity
	Inactive bool
}

// FlowResult captures the overall result of payment flow execution
type FlowResult struct {
	// In is the actual input amount consumed
	In EitherAmount

	// Out is the actual output amount delivered
	Out EitherAmount

	// Sandbox contains accumulated state changes
	Sandbox *PaymentSandbox

	// RemovableOffers contains offer hashes to remove
	RemovableOffers map[[32]byte]bool

	// Result is the transaction result code
	Result Result
}

// ToEitherAmount converts an Amount to EitherAmount
func ToEitherAmount(amt Amount) EitherAmount {
	if amt.IsNative() {
		drops, _ := strconv.ParseInt(amt.Value, 10, 64)
		return NewXRPEitherAmount(drops)
	}
	return NewIOUEitherAmount(amt.ToIOU())
}

// FromEitherAmount converts EitherAmount back to Amount
func FromEitherAmount(e EitherAmount) Amount {
	if e.IsNative {
		return NewXRPAmount(strconv.FormatInt(e.XRP, 10))
	}
	return Amount{
		Value:    formatIOUValue(e.IOU.Value),
		Currency: e.IOU.Currency,
		Issuer:   e.IOU.Issuer,
	}
}

// ToIOU converts an Amount to IOUAmount (panics if native)
func (a Amount) ToIOU() IOUAmount {
	if a.IsNative() {
		panic("Cannot convert native amount to IOU")
	}
	return NewIOUAmount(a.Value, a.Currency, a.Issuer)
}

// GetIssue extracts the Issue from an Amount
func GetIssue(amt Amount) Issue {
	if amt.IsNative() {
		return Issue{Currency: "XRP"}
	}

	var issuerBytes [20]byte
	if issuerID, err := decodeAccountID(amt.Issuer); err == nil {
		issuerBytes = issuerID
	}

	return Issue{
		Currency: amt.Currency,
		Issuer:   issuerBytes,
	}
}

// MulRatio multiplies an amount by a ratio (num/den)
// Used for quality calculations
func MulRatio(amt EitherAmount, num, den uint32, roundUp bool) EitherAmount {
	if den == 0 {
		return amt
	}

	if amt.IsNative {
		// For XRP, use integer math
		result := (int64(amt.XRP) * int64(num)) / int64(den)
		if roundUp && (int64(amt.XRP)*int64(num))%int64(den) != 0 {
			result++
		}
		return NewXRPEitherAmount(result)
	}

	// For IOU, use big.Float
	numF := new(big.Float).SetUint64(uint64(num))
	denF := new(big.Float).SetUint64(uint64(den))
	ratio := new(big.Float).Quo(numF, denF)
	result := new(big.Float).Mul(amt.IOU.Value, ratio)

	return NewIOUEitherAmount(IOUAmount{
		Value:    result,
		Currency: amt.IOU.Currency,
		Issuer:   amt.IOU.Issuer,
	})
}

// DivRatio divides an amount by a ratio (num/den) = amt * den / num
func DivRatio(amt EitherAmount, num, den uint32, roundUp bool) EitherAmount {
	if num == 0 {
		return amt
	}
	return MulRatio(amt, den, num, roundUp)
}
