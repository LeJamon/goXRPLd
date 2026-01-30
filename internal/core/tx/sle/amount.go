package sle

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Constants matching rippled's STAmount.h
const (
	// Exponent range for normalized IOU amounts
	MinExponent = -96
	MaxExponent = 80

	// Mantissa range for normalized IOU amounts [10^15, 10^16 - 1]
	MinMantissa int64 = 1_000_000_000_000_000
	MaxMantissa int64 = 9_999_999_999_999_999

	// Maximum native XRP in drops that can exist on the network
	MaxNativeDrops uint64 = 100_000_000_000_000_000

	// Drops per XRP
	DropsPerXRP int64 = 1_000_000

	// Zero exponent value used for IOU zero amounts
	zeroExponent = -100
)

// XRPAmount represents XRP in drops (the smallest unit)
// Uses int64 to match rippled's XRPAmount (allows negative for debt calculations)
type XRPAmount struct {
	drops int64
}

// NewXRPAmountFromDrops creates an XRP amount from drops
func NewXRPAmountFromDrops(drops int64) XRPAmount {
	return XRPAmount{drops: drops}
}

// Drops returns the amount in drops
func (x XRPAmount) Drops() int64 {
	return x.drops
}

// IsZero returns true if the amount is zero
func (x XRPAmount) IsZero() bool {
	return x.drops == 0
}

// IsNegative returns true if the amount is negative
func (x XRPAmount) IsNegative() bool {
	return x.drops < 0
}

// Signum returns the sign of the amount (-1, 0, or 1)
func (x XRPAmount) Signum() int {
	if x.drops < 0 {
		return -1
	}
	if x.drops > 0 {
		return 1
	}
	return 0
}

// Negate returns the negated amount
func (x XRPAmount) Negate() XRPAmount {
	return XRPAmount{drops: -x.drops}
}

// Add adds two XRP amounts
func (x XRPAmount) Add(other XRPAmount) XRPAmount {
	return XRPAmount{drops: x.drops + other.drops}
}

// Sub subtracts two XRP amounts
func (x XRPAmount) Sub(other XRPAmount) XRPAmount {
	return XRPAmount{drops: x.drops - other.drops}
}

// String returns the drops as a string
func (x XRPAmount) String() string {
	return strconv.FormatInt(x.drops, 10)
}

// DecimalXRP returns the amount in XRP (not drops)
func (x XRPAmount) DecimalXRP() float64 {
	return float64(x.drops) / float64(DropsPerXRP)
}

// IOUAmountValue represents an issued currency amount using mantissa/exponent
// Matches rippled's IOUAmount representation
type IOUAmountValue struct {
	mantissa int64 // Signed mantissa, allows negative values
	exponent int   // Exponent in range [-96, 80] for non-zero values
}

// NewIOUAmountValue creates a new IOU amount value and normalizes it
func NewIOUAmountValue(mantissa int64, exponent int) IOUAmountValue {
	v := IOUAmountValue{mantissa: mantissa, exponent: exponent}
	v.normalize()
	return v
}

// ZeroIOUValue returns a zero IOU amount value
func ZeroIOUValue() IOUAmountValue {
	return IOUAmountValue{mantissa: 0, exponent: zeroExponent}
}

// normalize adjusts the mantissa and exponent to the proper range
// Matches rippled's IOUAmount::normalize()
func (v *IOUAmountValue) normalize() {
	if v.mantissa == 0 {
		v.mantissa = 0
		v.exponent = zeroExponent
		return
	}

	negative := v.mantissa < 0
	if negative {
		v.mantissa = -v.mantissa
	}

	// Scale up if mantissa is too small
	for v.mantissa < MinMantissa && v.exponent > MinExponent {
		v.mantissa *= 10
		v.exponent--
	}

	// Scale down if mantissa is too large
	for v.mantissa > MaxMantissa {
		if v.exponent >= MaxExponent {
			panic("IOUAmount overflow")
		}
		v.mantissa /= 10
		v.exponent++
	}

	// Underflow to zero
	if v.exponent < MinExponent || v.mantissa < MinMantissa {
		v.mantissa = 0
		v.exponent = zeroExponent
		return
	}

	// Overflow check
	if v.exponent > MaxExponent {
		panic("IOUAmount overflow")
	}

	if negative {
		v.mantissa = -v.mantissa
	}
}

// Mantissa returns the mantissa
func (v IOUAmountValue) Mantissa() int64 {
	return v.mantissa
}

// Exponent returns the exponent
func (v IOUAmountValue) Exponent() int {
	return v.exponent
}

// IsZero returns true if the amount is zero
func (v IOUAmountValue) IsZero() bool {
	return v.mantissa == 0
}

// IsNegative returns true if the amount is negative
func (v IOUAmountValue) IsNegative() bool {
	return v.mantissa < 0
}

// Signum returns the sign of the amount (-1, 0, or 1)
func (v IOUAmountValue) Signum() int {
	if v.mantissa < 0 {
		return -1
	}
	if v.mantissa > 0 {
		return 1
	}
	return 0
}

// Negate returns the negated amount
func (v IOUAmountValue) Negate() IOUAmountValue {
	return IOUAmountValue{mantissa: -v.mantissa, exponent: v.exponent}
}

// Float64 returns an approximate float64 representation
func (v IOUAmountValue) Float64() float64 {
	if v.mantissa == 0 {
		return 0
	}
	return float64(v.mantissa) * math.Pow10(v.exponent)
}

// String returns a decimal string representation
func (v IOUAmountValue) String() string {
	if v.mantissa == 0 {
		return "0"
	}

	negative := v.mantissa < 0
	mantissa := v.mantissa
	if negative {
		mantissa = -mantissa
	}

	// Convert mantissa to string
	mantissaStr := strconv.FormatInt(mantissa, 10)
	mantissaLen := len(mantissaStr)

	// Calculate where the decimal point should be
	// The value is mantissa * 10^exponent
	decimalPos := mantissaLen + v.exponent

	var result string
	if decimalPos <= 0 {
		// Need leading zeros: 0.000...digits
		result = "0." + strings.Repeat("0", -decimalPos) + mantissaStr
	} else if decimalPos >= mantissaLen {
		// No decimal point needed, or trailing zeros
		if v.exponent >= 0 {
			result = mantissaStr + strings.Repeat("0", v.exponent)
		} else {
			result = mantissaStr
		}
	} else {
		// Decimal point in the middle
		result = mantissaStr[:decimalPos] + "." + mantissaStr[decimalPos:]
	}

	// Remove trailing zeros after decimal point
	if strings.Contains(result, ".") {
		result = strings.TrimRight(result, "0")
		result = strings.TrimRight(result, ".")
	}

	if negative {
		result = "-" + result
	}

	return result
}

// Amount represents either XRP (as drops) or an issued currency amount
// Matches rippled's STAmount which can hold any asset type
type Amount struct {
	// For XRP amounts
	xrp XRPAmount

	// For issued currency amounts
	iou      IOUAmountValue
	Currency string
	Issuer   string

	// Native indicates if this is XRP (true) or issued currency (false)
	Native bool
}

// NewXRPAmountFromInt creates an XRP amount from drops as int64
func NewXRPAmountFromInt(drops int64) Amount {
	return Amount{
		xrp:    XRPAmount{drops: drops},
		Native: true,
	}
}

// NewIssuedAmountFromValue creates an issued currency amount from mantissa/exponent
func NewIssuedAmountFromValue(mantissa int64, exponent int, currency, issuer string) Amount {
	iouVal := NewIOUAmountValue(mantissa, exponent)
	result := Amount{
		iou:      iouVal,
		Currency: currency,
		Issuer:   issuer,
		Native:   false,
	}
	return result
}

// NewIssuedAmountFromFloat64 creates an issued currency amount from a float64 value.
// This is a convenience function that converts the float to mantissa/exponent internally.
func NewIssuedAmountFromFloat64(value float64, currency, issuer string) Amount {
	mantissa, exponent := Float64ToMantissaExponent(value)
	return NewIssuedAmountFromValue(mantissa, exponent, currency, issuer)
}

// Float64ToMantissaExponent converts a float64 to mantissa and exponent.
// Returns (mantissa, exponent) where value = mantissa * 10^exponent.
func Float64ToMantissaExponent(value float64) (int64, int) {
	if value == 0 {
		return 0, zeroExponent
	}

	negative := value < 0
	if negative {
		value = -value
	}

	// Find the exponent to normalize value to [1, 10)
	exponent := 0
	if value >= 1 {
		for value >= 10 {
			value /= 10
			exponent++
		}
	} else {
		for value < 1 {
			value *= 10
			exponent--
		}
	}

	// Scale to get ~15 significant digits in mantissa
	// Mantissa should be in range [10^15, 10^16)
	targetMantissa := value * math.Pow10(15)
	mantissa := int64(math.Round(targetMantissa))
	exponent = exponent - 15

	if negative {
		mantissa = -mantissa
	}

	return mantissa, exponent
}

// NewIssuedAmountFromDecimalString creates an Amount from a decimal string value.
// This avoids precision loss that occurs when going through float64.
func NewIssuedAmountFromDecimalString(value, currency, issuer string) Amount {
	iou := parseIOUValueFromString(value)
	return Amount{
		iou:      iou,
		Currency: currency,
		Issuer:   issuer,
		Native:   false,
	}
}

// IsNative returns true if this is an XRP amount
func (a Amount) IsNative() bool {
	// Only check the Native field, which is explicitly set during construction.
	// Do NOT check for empty Currency/Issuer as that would incorrectly classify
	// arithmetic-only Amounts (like Quality rates) as native XRP.
	return a.Native
}

// Drops returns the XRP amount in drops (only valid for native amounts)
func (a Amount) Drops() int64 {
	if !a.IsNative() {
		return 0
	}
	return a.xrp.drops
}

// XRP returns the XRPAmount (only valid for native amounts)
func (a Amount) XRP() XRPAmount {
	return a.xrp
}

// IOU returns the IOUAmountValue (only valid for issued amounts)
func (a Amount) IOU() IOUAmountValue {
	return a.iou
}

// IsZero returns true if the amount is zero
func (a Amount) IsZero() bool {
	if a.IsNative() {
		return a.xrp.IsZero()
	}
	return a.iou.IsZero()
}

// IsNegative returns true if the amount is negative
func (a Amount) IsNegative() bool {
	if a.IsNative() {
		return a.xrp.IsNegative()
	}
	return a.iou.IsNegative()
}

// Signum returns the sign of the amount (-1, 0, or 1)
func (a Amount) Signum() int {
	if a.IsNative() {
		return a.xrp.Signum()
	}
	return a.iou.Signum()
}

// Value returns the value as a string (for JSON serialization)
func (a Amount) Value() string {
	if a.IsNative() {
		return a.xrp.String()
	}
	return a.iou.String()
}

// Float64 returns an approximate float64 representation
func (a Amount) Float64() float64 {
	if a.IsNative() {
		return float64(a.xrp.drops)
	}
	return a.iou.Float64()
}

// Negate returns the negated amount
func (a Amount) Negate() Amount {
	if a.IsNative() {
		return Amount{
			xrp:    a.xrp.Negate(),
			Native: true,
		}
	}
	return Amount{
		iou:      a.iou.Negate(),
		Currency: a.Currency,
		Issuer:   a.Issuer,
		Native:   false,
	}
}

// MarshalJSON implements custom JSON marshaling
func (a Amount) MarshalJSON() ([]byte, error) {
	if a.IsNative() {
		return json.Marshal(a.xrp.String())
	}
	return json.Marshal(map[string]string{
		"value":    a.iou.String(),
		"currency": a.Currency,
		"issuer":   a.Issuer,
	})
}

// UnmarshalJSON implements custom JSON unmarshaling
func (a *Amount) UnmarshalJSON(data []byte) error {
	// Try as string first (XRP drops)
	var strVal string
	if err := json.Unmarshal(data, &strVal); err == nil {
		drops, err := strconv.ParseInt(strVal, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid XRP drops value: %w", err)
		}
		a.xrp = XRPAmount{drops: drops}
		a.Native = true
		return nil
	}

	// Try as object (issued currency)
	var objVal struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
		Issuer   string `json:"issuer"`
	}
	if err := json.Unmarshal(data, &objVal); err != nil {
		return err
	}

	a.iou = parseIOUValueFromString(objVal.Value)
	a.Currency = objVal.Currency
	a.Issuer = objVal.Issuer
	a.Native = false
	return nil
}

// parseIOUValueFromString parses a decimal string into IOUAmountValue (for JSON unmarshaling)
func parseIOUValueFromString(value string) IOUAmountValue {
	if value == "" || value == "0" {
		return ZeroIOUValue()
	}

	negative := false
	if strings.HasPrefix(value, "-") {
		negative = true
		value = value[1:]
	}

	// Split on decimal point
	parts := strings.Split(value, ".")
	intPart := parts[0]
	fracPart := ""
	if len(parts) > 1 {
		fracPart = parts[1]
	}

	// Remove leading zeros from int part
	intPart = strings.TrimLeft(intPart, "0")
	if intPart == "" {
		intPart = "0"
	}

	// Combine digits
	digits := intPart + fracPart

	// Remove trailing zeros (we'll account for them in exponent)
	digits = strings.TrimRight(digits, "0")
	if digits == "" {
		return ZeroIOUValue()
	}

	// Parse mantissa
	mantissa, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return ZeroIOUValue()
	}

	// Calculate exponent
	originalDigits := intPart + fracPart
	trailingZeros := len(originalDigits) - len(digits)
	exponent := -len(fracPart) + trailingZeros

	if negative {
		mantissa = -mantissa
	}

	return NewIOUAmountValue(mantissa, exponent)
}

// flattenAmount converts an Amount to its JSON-compatible representation
func flattenAmount(a Amount) any {
	if a.IsNative() {
		return a.xrp.String()
	}
	return map[string]string{
		"value":    a.iou.String(),
		"currency": a.Currency,
		"issuer":   a.Issuer,
	}
}

// Add adds two amounts (must be same type)
func (a Amount) Add(b Amount) (Amount, error) {
	if a.IsNative() != b.IsNative() {
		return Amount{}, errors.New("cannot add XRP and IOU amounts")
	}
	if a.IsNative() {
		return Amount{
			xrp:    a.xrp.Add(b.xrp),
			Native: true,
		}, nil
	}
	result := addIOUValues(a.iou, b.iou)
	return Amount{
		iou:      result,
		Currency: a.Currency,
		Issuer:   a.Issuer,
		Native:   false,
	}, nil
}

// Sub subtracts two amounts (must be same type)
func (a Amount) Sub(b Amount) (Amount, error) {
	return a.Add(b.Negate())
}

// addIOUValues adds two IOU values with proper exponent handling
func addIOUValues(a, b IOUAmountValue) IOUAmountValue {
	if a.IsZero() {
		return b
	}
	if b.IsZero() {
		return a
	}

	// Align exponents
	aExp := a.exponent
	bExp := b.exponent
	aMant := a.mantissa
	bMant := b.mantissa

	// Align to the larger exponent
	for aExp < bExp {
		aMant /= 10
		aExp++
	}
	for bExp < aExp {
		bMant /= 10
		bExp++
	}

	result := aMant + bMant

	// Handle near-zero results
	if result >= -10 && result <= 10 {
		return ZeroIOUValue()
	}

	return NewIOUAmountValue(result, aExp)
}

// Compare compares two amounts
// Returns -1 if a < b, 0 if a == b, 1 if a > b
func (a Amount) Compare(b Amount) int {
	if a.IsNative() && b.IsNative() {
		if a.xrp.drops < b.xrp.drops {
			return -1
		}
		if a.xrp.drops > b.xrp.drops {
			return 1
		}
		return 0
	}
	if !a.IsNative() && !b.IsNative() {
		return compareIOUValues(a.iou, b.iou)
	}
	// Mixed types - XRP comes first
	if a.IsNative() {
		return -1
	}
	return 1
}

// compareIOUValues compares two IOU values using mantissa/exponent without float64 conversion.
func compareIOUValues(a, b IOUAmountValue) int {
	// Handle signs first
	aSign := a.Signum()
	bSign := b.Signum()
	if aSign < bSign {
		return -1
	}
	if aSign > bSign {
		return 1
	}
	if aSign == 0 && bSign == 0 {
		return 0
	}

	// Same sign - compare magnitudes
	// For positive values: larger exponent = larger value (if mantissas are normalized)
	if a.exponent > b.exponent {
		if aSign > 0 {
			return 1
		}
		return -1
	}
	if a.exponent < b.exponent {
		if aSign > 0 {
			return -1
		}
		return 1
	}

	// Same exponent - compare mantissas
	if a.mantissa < b.mantissa {
		return -1
	}
	if a.mantissa > b.mantissa {
		return 1
	}
	return 0
}

// MulRatio multiplies this amount by num/den with optional rounding up.
// Uses big.Int arithmetic to avoid overflow on large mantissa * num products.
func (a Amount) MulRatio(num, den uint32, roundUp bool) Amount {
	if a.IsNative() {
		// For XRP, use integer arithmetic on drops
		drops := a.Drops()
		result := drops * int64(num) / int64(den)
		if roundUp && drops*int64(num)%int64(den) != 0 {
			result++
		}
		return NewXRPAmountFromInt(result)
	}

	if den == 0 || a.IsZero() {
		return a
	}

	// For IOU: multiply mantissa by num/den
	// Use big.Int to avoid overflow: mantissa can be ~10^15, num ~10^9
	// so mantissa * num can be ~10^24 which exceeds int64 (~9.2×10^18)
	mantissa := a.iou.Mantissa()
	negative := mantissa < 0
	if negative {
		mantissa = -mantissa
	}

	// Use big.Int for the multiplication to avoid overflow
	bigMant := new(big.Int).SetInt64(mantissa)
	bigNum := new(big.Int).SetUint64(uint64(num))
	bigDen := new(big.Int).SetUint64(uint64(den))

	// bigProd = mantissa * num
	bigProd := new(big.Int).Mul(bigMant, bigNum)

	// result = bigProd / den
	bigResult := new(big.Int).Div(bigProd, bigDen)

	// Handle rounding up if needed
	if roundUp {
		remainder := new(big.Int).Mod(bigProd, bigDen)
		if remainder.Sign() != 0 {
			bigResult.Add(bigResult, big.NewInt(1))
		}
	}

	// Convert back to int64 - may need to adjust exponent if result is too large
	resultMant := bigResult.Int64()
	resultExp := a.iou.Exponent()

	// Normalize if mantissa is too large for int64 representation
	// The mantissa should be in range [10^15, 10^16) for normalized form
	for resultMant >= 1e16 {
		resultMant /= 10
		resultExp++
	}

	if negative {
		resultMant = -resultMant
	}

	return NewIssuedAmountFromValue(resultMant, resultExp, a.Currency, a.Issuer)
}

// Mul multiplies this Amount by another Amount.
// Reference: rippled's mulRound() in STAmount.cpp
// For IOU * IOU: result = (m1 * m2) * 10^(e1 + e2)
func (a Amount) Mul(other Amount, roundUp bool) Amount {
	if a.IsZero() || other.IsZero() {
		if a.IsNative() {
			return NewXRPAmountFromInt(0)
		}
		return NewIssuedAmountFromValue(0, -100, a.Currency, a.Issuer)
	}

	// Handle XRP * XRP case
	if a.IsNative() && other.IsNative() {
		result := a.Drops() * other.Drops()
		return NewXRPAmountFromInt(result)
	}

	// For IOU multiplication, use precise big.Int arithmetic
	// result = (a.mantissa * other.mantissa) * 10^(a.exp + other.exp)
	m1 := a.Mantissa()
	e1 := a.Exponent()
	m2 := other.Mantissa()
	e2 := other.Exponent()

	// Handle sign
	negative := (m1 < 0) != (m2 < 0)
	if m1 < 0 {
		m1 = -m1
	}
	if m2 < 0 {
		m2 = -m2
	}

	// Use big.Int for multiplication to avoid overflow
	bigM1 := new(big.Int).SetInt64(m1)
	bigM2 := new(big.Int).SetInt64(m2)
	bigProduct := new(big.Int).Mul(bigM1, bigM2)

	resultExp := e1 + e2

	// Normalize the result to mantissa in [10^15, 10^16)
	minMantissa := new(big.Int).SetInt64(1000000000000000)  // 10^15
	maxMantissa := new(big.Int).SetInt64(10000000000000000) // 10^16
	ten := big.NewInt(10)
	five := big.NewInt(5)

	for bigProduct.Cmp(maxMantissa) >= 0 {
		// Use DivMod to get remainder for rounding
		remainder := new(big.Int)
		bigProduct.DivMod(bigProduct, ten, remainder)
		// Round up if remainder >= 5 (or if roundUp is true and remainder > 0)
		if remainder.Cmp(five) >= 0 || (roundUp && remainder.Sign() > 0) {
			bigProduct.Add(bigProduct, big.NewInt(1))
		}
		resultExp++
	}
	for bigProduct.Cmp(minMantissa) < 0 && bigProduct.Sign() != 0 {
		bigProduct.Mul(bigProduct, ten)
		resultExp--
	}

	resultMant := bigProduct.Int64()
	if negative {
		resultMant = -resultMant
	}

	if a.IsNative() {
		// Result is XRP - convert from mantissa/exponent to drops
		for resultExp > 0 {
			resultMant *= 10
			resultExp--
		}
		for resultExp < 0 {
			resultMant /= 10
			resultExp++
		}
		return NewXRPAmountFromInt(resultMant)
	}

	return NewIssuedAmountFromValue(resultMant, resultExp, a.Currency, a.Issuer)
}

// Div divides this Amount by another Amount.
// Reference: rippled's divRound() in STAmount.cpp
// For IOU / IOU: result = (m1 / m2) * 10^(e1 - e2)
func (a Amount) Div(other Amount, roundUp bool) Amount {
	if other.IsZero() {
		// Division by zero - return zero
		if a.IsNative() {
			return NewXRPAmountFromInt(0)
		}
		return NewIssuedAmountFromValue(0, -100, a.Currency, a.Issuer)
	}

	if a.IsZero() {
		if a.IsNative() {
			return NewXRPAmountFromInt(0)
		}
		return NewIssuedAmountFromValue(0, -100, a.Currency, a.Issuer)
	}

	// Handle XRP / XRP case
	if a.IsNative() && other.IsNative() {
		result := a.Drops() / other.Drops()
		if roundUp && a.Drops()%other.Drops() != 0 {
			result++
		}
		return NewXRPAmountFromInt(result)
	}

	// For IOU division, use precise big.Int arithmetic
	m1 := a.Mantissa()
	e1 := a.Exponent()
	m2 := other.Mantissa()
	e2 := other.Exponent()

	// Handle sign
	negative := (m1 < 0) != (m2 < 0)
	if m1 < 0 {
		m1 = -m1
	}
	if m2 < 0 {
		m2 = -m2
	}

	// Normalize numerator to have enough precision for division
	// We need m1 to be large enough that m1/m2 stays in range [10^15, 10^16)
	bigM1 := new(big.Int).SetInt64(m1)
	bigM2 := new(big.Int).SetInt64(m2)

	// Scale up m1 to get more precision in the division
	// Multiply m1 by 10^16 to get enough precision
	scale := big.NewInt(10000000000000000) // 10^16
	bigM1.Mul(bigM1, scale)

	resultExp := e1 - e2 - 16 // -16 because we scaled up by 10^16

	// Perform division
	bigResult := new(big.Int).Div(bigM1, bigM2)

	// Handle rounding up
	if roundUp {
		remainder := new(big.Int).Mod(bigM1, bigM2)
		if remainder.Sign() != 0 {
			bigResult.Add(bigResult, big.NewInt(1))
		}
	}

	// Normalize the result to mantissa in [10^15, 10^16)
	minMantissa := new(big.Int).SetInt64(1000000000000000)  // 10^15
	maxMantissa := new(big.Int).SetInt64(10000000000000000) // 10^16
	ten := big.NewInt(10)

	for bigResult.Cmp(maxMantissa) >= 0 {
		bigResult.Div(bigResult, ten)
		resultExp++
	}
	for bigResult.Cmp(minMantissa) < 0 && bigResult.Sign() != 0 {
		bigResult.Mul(bigResult, ten)
		resultExp--
	}

	resultMant := bigResult.Int64()
	if negative {
		resultMant = -resultMant
	}

	if a.IsNative() {
		// Result is XRP - convert from mantissa/exponent to drops
		for resultExp > 0 {
			resultMant *= 10
			resultExp--
		}
		for resultExp < 0 {
			resultMant /= 10
			resultExp++
		}
		return NewXRPAmountFromInt(resultMant)
	}

	return NewIssuedAmountFromValue(resultMant, resultExp, a.Currency, a.Issuer)
}

// Mantissa returns the mantissa of the Amount (for IOU) or drops (for XRP).
func (a Amount) Mantissa() int64 {
	if a.IsNative() {
		return a.Drops()
	}
	return a.iou.Mantissa()
}

// Exponent returns the exponent of the Amount (for IOU) or 0 (for XRP).
func (a Amount) Exponent() int {
	if a.IsNative() {
		return 0
	}
	return a.iou.Exponent()
}
