package tx

import (
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	crypto "github.com/LeJamon/goXRPLd/internal/crypto/common"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// ============================================================================
// High-Precision Number type for AMM calculations
// Reference: rippled Number.h - maintains 16 significant digits with proper rounding
// ============================================================================

// Number represents a high-precision number for AMM calculations
// Uses int64 mantissa with explicit exponent to maintain precision
type Number struct {
	mantissa int64
	exponent int
}

// RoundingMode defines how to round numbers
type RoundingMode int

const (
	RoundToNearest RoundingMode = iota
	RoundDownward
	RoundUpward
	RoundTowardZero
)

// DefaultRounding is the default rounding mode
var DefaultRounding RoundingMode = RoundToNearest

// NumberZero is the zero value
var NumberZero = Number{mantissa: 0, exponent: 0}

// NumberOne is the one value
var NumberOne = Number{mantissa: 1000000000000000, exponent: -15}

// maxMantissa is the maximum mantissa value (10^16 - 1)
const maxMantissa int64 = 9999999999999999

// minMantissa is the minimum mantissa value for normalization
const minMantissa int64 = 1000000000000000

// NewNumber creates a new Number from float64
func NewNumber(v float64) Number {
	if v == 0 {
		return NumberZero
	}

	negative := v < 0
	if negative {
		v = -v
	}

	// Calculate exponent and mantissa
	exp := 0
	for v >= 10 {
		v /= 10
		exp++
	}
	for v < 1 {
		v *= 10
		exp--
	}

	// Scale to 16 digits
	mantissa := int64(v * 1e15)
	exp -= 15

	if negative {
		mantissa = -mantissa
	}

	return Number{mantissa: mantissa, exponent: exp}.normalize()
}

// NewNumberFromInt creates a Number from an integer
func NewNumberFromInt(v int64) Number {
	if v == 0 {
		return NumberZero
	}
	return Number{mantissa: v * minMantissa, exponent: -15}.normalize()
}

// NewNumberFromUint64 creates a Number from uint64
func NewNumberFromUint64(v uint64) Number {
	if v == 0 {
		return NumberZero
	}
	return NewNumber(float64(v))
}

// normalize adjusts mantissa to proper range
func (n Number) normalize() Number {
	if n.mantissa == 0 {
		return NumberZero
	}

	// Normalize mantissa to [minMantissa, maxMantissa]
	for n.mantissa > maxMantissa || n.mantissa < -maxMantissa {
		n.mantissa /= 10
		n.exponent++
	}
	for n.mantissa != 0 && (n.mantissa < minMantissa && n.mantissa > -minMantissa) {
		n.mantissa *= 10
		n.exponent--
	}

	return n
}

// Float64 converts Number to float64
func (n Number) Float64() float64 {
	if n.mantissa == 0 {
		return 0
	}
	return float64(n.mantissa) * math.Pow10(n.exponent)
}

// Int64 converts Number to int64
func (n Number) Int64() int64 {
	return int64(n.Float64())
}

// Uint64 converts Number to uint64
func (n Number) Uint64() uint64 {
	f := n.Float64()
	if f < 0 {
		return 0
	}
	return uint64(f)
}

// IsZero returns true if the number is zero
func (n Number) IsZero() bool {
	return n.mantissa == 0
}

// IsNegative returns true if the number is negative
func (n Number) IsNegative() bool {
	return n.mantissa < 0
}

// Neg returns the negation of the number
func (n Number) Neg() Number {
	return Number{mantissa: -n.mantissa, exponent: n.exponent}
}

// Add returns n + o
func (n Number) Add(o Number) Number {
	if n.IsZero() {
		return o
	}
	if o.IsZero() {
		return n
	}

	// Align exponents
	if n.exponent > o.exponent {
		diff := n.exponent - o.exponent
		if diff > 30 {
			return n
		}
		for i := 0; i < diff; i++ {
			o.mantissa /= 10
		}
		o.exponent = n.exponent
	} else if o.exponent > n.exponent {
		diff := o.exponent - n.exponent
		if diff > 30 {
			return o
		}
		for i := 0; i < diff; i++ {
			n.mantissa /= 10
		}
		n.exponent = o.exponent
	}

	return Number{mantissa: n.mantissa + o.mantissa, exponent: n.exponent}.normalize()
}

// Sub returns n - o
func (n Number) Sub(o Number) Number {
	return n.Add(o.Neg())
}

// Mul returns n * o
func (n Number) Mul(o Number) Number {
	if n.IsZero() || o.IsZero() {
		return NumberZero
	}

	// Use big.Int for multiplication to avoid overflow
	m1 := big.NewInt(n.mantissa)
	m2 := big.NewInt(o.mantissa)
	product := new(big.Int).Mul(m1, m2)

	// Normalize back to our format
	exp := n.exponent + o.exponent

	// Scale down the product
	divisor := big.NewInt(minMantissa)
	product.Div(product, divisor)
	exp += 15

	mantissa := product.Int64()
	return Number{mantissa: mantissa, exponent: exp}.normalize()
}

// Div returns n / o
func (n Number) Div(o Number) Number {
	if o.IsZero() {
		// Return a very large number or handle as error
		return Number{mantissa: maxMantissa, exponent: 308}
	}
	if n.IsZero() {
		return NumberZero
	}

	// Scale up n for precision
	m1 := big.NewInt(n.mantissa)
	m1.Mul(m1, big.NewInt(minMantissa))
	m2 := big.NewInt(o.mantissa)

	quotient := new(big.Int).Div(m1, m2)
	exp := n.exponent - o.exponent - 15

	mantissa := quotient.Int64()
	return Number{mantissa: mantissa, exponent: exp}.normalize()
}

// Cmp compares n with o, returns -1 if n < o, 0 if n == o, 1 if n > o
func (n Number) Cmp(o Number) int {
	diff := n.Sub(o)
	if diff.IsZero() {
		return 0
	}
	if diff.IsNegative() {
		return -1
	}
	return 1
}

// Lt returns true if n < o
func (n Number) Lt(o Number) bool {
	return n.Cmp(o) < 0
}

// Gt returns true if n > o
func (n Number) Gt(o Number) bool {
	return n.Cmp(o) > 0
}

// Le returns true if n <= o
func (n Number) Le(o Number) bool {
	return n.Cmp(o) <= 0
}

// Ge returns true if n >= o
func (n Number) Ge(o Number) bool {
	return n.Cmp(o) >= 0
}

// Sqrt returns the square root using Newton's method
func (n Number) Sqrt() Number {
	if n.IsZero() {
		return NumberZero
	}
	if n.IsNegative() {
		return NumberZero // Invalid, return zero
	}

	// Initial estimate
	x := NewNumber(math.Sqrt(n.Float64()))

	// Newton-Raphson iterations for precision
	for i := 0; i < 5; i++ {
		// x = (x + n/x) / 2
		xNew := x.Add(n.Div(x)).Div(NewNumberFromInt(2))
		if x.Sub(xNew).Float64() < 1e-15 {
			break
		}
		x = xNew
	}

	return x
}

// Power returns n^exp (for small integer exponents)
func (n Number) Power(exp int) Number {
	if exp == 0 {
		return NumberOne
	}
	if exp == 1 {
		return n
	}
	if n.IsZero() {
		return NumberZero
	}

	result := NumberOne
	base := n
	negative := exp < 0
	if negative {
		exp = -exp
	}

	for exp > 0 {
		if exp&1 == 1 {
			result = result.Mul(base)
		}
		base = base.Mul(base)
		exp >>= 1
	}

	if negative {
		return NumberOne.Div(result)
	}
	return result
}

// PowerFloat returns n^exp for fractional exponents
func (n Number) PowerFloat(exp float64) Number {
	if exp == 0 {
		return NumberOne
	}
	if n.IsZero() {
		return NumberZero
	}
	return NewNumber(math.Pow(n.Float64(), exp))
}

// Max returns the maximum of n and o
func (n Number) Max(o Number) Number {
	if n.Gt(o) {
		return n
	}
	return o
}

// Min returns the minimum of n and o
func (n Number) Min(o Number) Number {
	if n.Lt(o) {
		return n
	}
	return o
}

// ============================================================================
// AMM Data Structures
// Reference: rippled ltAMM.h, SLE.h
// ============================================================================

// AMMData represents an AMM ledger entry
type AMMData struct {
	Account        [20]byte        // AMM account
	Asset          Asset           // First asset
	Asset2         Asset           // Second asset
	TradingFee     uint16          // Trading fee in basis points (0-1000)
	LPTokenBalance Number          // LP token balance
	VoteSlots      []VoteSlotData  // Vote slots for fee voting
	AuctionSlot    *AuctionSlotData // Auction slot
	OwnerNode      uint64          // Directory node
}

// VoteSlotData represents a voting slot in an AMM
type VoteSlotData struct {
	Account    [20]byte
	TradingFee uint16
	VoteWeight uint32
}

// AuctionSlotData represents the auction slot in an AMM
type AuctionSlotData struct {
	Account       [20]byte
	Expiration    uint32
	Price         Number
	DiscountedFee uint16
	AuthAccounts  [][20]byte
}

// ============================================================================
// AMM Constants
// Reference: rippled AMMCore.h
// ============================================================================

const (
	// TRADING_FEE_THRESHOLD is maximum trading fee (1000 = 1%)
	TRADING_FEE_THRESHOLD_VAL uint16 = 1000

	// Fee calculation constants
	FEE_MULTIPLIER = 100000 // tfee is in 1/100000 units

	// Vote slot constants
	VOTE_MAX_SLOTS_VAL       = 8
	VOTE_WEIGHT_SCALE_FACTOR_VAL = 100000

	// Auction slot constants
	AUCTION_SLOT_TIME_INTERVALS_VAL  = 20
	AUCTION_SLOT_TOTAL_TIME_SECS_VAL = 24 * 60 * 60 // 24 hours
	AUCTION_SLOT_INTERVAL_DURATION_VAL = AUCTION_SLOT_TOTAL_TIME_SECS_VAL / AUCTION_SLOT_TIME_INTERVALS_VAL
	AUCTION_SLOT_MIN_FEE_FRACTION_VAL  = 25    // minPrice = lptBalance * fee / 25
	AUCTION_SLOT_DISCOUNTED_FEE_FRACTION_VAL = 10 // discountedFee = fee / 10
	AUCTION_SLOT_MAX_AUTH_ACCOUNTS_VAL = 4
)

// ============================================================================
// AMM Math Functions
// Reference: rippled AMMHelpers.cpp
// ============================================================================

// parseAmount parses an amount string to Number
func parseAmountToNumber(value string) Number {
	if value == "" {
		return NumberZero
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return NumberZero
	}
	return NewNumber(f)
}

// parseAmount parses an amount string to uint64 (for backward compatibility)
func parseAmount(value string) uint64 {
	if value == "" {
		return 0
	}
	// Try parsing as integer first (for XRP drops)
	if i, err := strconv.ParseUint(value, 10, 64); err == nil {
		return i
	}
	// Try parsing as float (for IOU amounts)
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		if f < 0 {
			return 0
		}
		return uint64(f)
	}
	return 0
}

// feeMult returns (1 - fee/100000) as a Number
// Reference: rippled AMMCore.h feeMult
func feeMultNumber(tfee uint16) Number {
	fee := NewNumber(float64(tfee) / FEE_MULTIPLIER)
	return NumberOne.Sub(fee)
}

// getFeeNumber returns fee as a fraction
func getFeeNumber(tfee uint16) Number {
	return NewNumber(float64(tfee) / FEE_MULTIPLIER)
}

// feeMult returns (1 - fee/100000) for fee calculations
// tfee is in units of 1/100000 (e.g., 1000 = 1%)
func feeMult(tfee uint16) float64 {
	return 1.0 - float64(tfee)/100000.0
}

// feeMultHalf returns (1 - fee/200000) for fee calculations
func feeMultHalf(tfee uint16) float64 {
	return 1.0 - float64(tfee)/200000.0
}

// getFee returns fee as a fraction (e.g., 1000 -> 0.01)
func getFee(tfee uint16) float64 {
	return float64(tfee) / 100000.0
}

// ammLPTokens calculates initial LP tokens: sqrt(amount1 * amount2)
// Reference: rippled AMMHelpers.cpp ammLPTokens
func ammLPTokens(amount1, amount2 Number) Number {
	return amount1.Mul(amount2).Sqrt()
}

// calculateLPTokens calculates initial LP tokens using sqrt(amount1 * amount2)
// Reference: rippled AMMHelpers.cpp ammLPTokens
func calculateLPTokens(amount1, amount2 uint64) uint64 {
	// Use big.Int to avoid overflow in multiplication
	a1 := new(big.Int).SetUint64(amount1)
	a2 := new(big.Int).SetUint64(amount2)
	product := new(big.Int).Mul(a1, a2)

	// Calculate square root
	sqrt := new(big.Int).Sqrt(product)
	return sqrt.Uint64()
}

// lpTokensOut calculates LP tokens for single asset deposit
// Reference: rippled AMMHelpers.cpp lpTokensOut (Equation 3 from XLS-30d)
// Formula: tokens = lptBalance * ((sqrt(1 + deposit/balance * (1-fee)) - 1))
// Simplified: t = T * (b/B - x) / (1 + x), where x = sqrt(f2² + b/(B*f1)) - f2
func lpTokensOut(assetBalance, assetDeposit, lptBalance Number, tfee uint16) Number {
	if assetBalance.IsZero() || lptBalance.IsZero() {
		return NumberZero
	}

	f1 := feeMultNumber(tfee)                  // 1 - fee
	feeHalf := NewNumber(float64(tfee) / 200000.0) // fee/2
	f2 := NumberOne.Sub(feeHalf).Div(f1)       // (1 - fee/2) / (1 - fee)

	r := assetDeposit.Div(assetBalance) // deposit / balance

	// x = sqrt(f2² + r/f1) - f2
	f2Squared := f2.Mul(f2)
	rOverF1 := r.Div(f1)
	x := f2Squared.Add(rOverF1).Sqrt().Sub(f2)

	// t = T * (r - x) / (1 + x)
	numerator := r.Sub(x)
	denominator := NumberOne.Add(x)
	tokens := lptBalance.Mul(numerator).Div(denominator)

	if tokens.IsNegative() {
		return NumberZero
	}
	return tokens
}

// ammAssetIn calculates asset needed for desired LP tokens
// Reference: rippled AMMHelpers.cpp ammAssetIn (Equation 4 from XLS-30d)
func ammAssetIn(assetBalance, lptBalance, lpTokens Number, tfee uint16) Number {
	if lptBalance.IsZero() {
		return NumberZero
	}

	f1 := feeMultNumber(tfee)
	feeHalf := NewNumber(float64(tfee) / 200000.0)
	f2 := NumberOne.Sub(feeHalf).Div(f1)

	t1 := lpTokens.Div(lptBalance) // tokens / total
	t2 := NumberOne.Add(t1)        // 1 + t1
	d := f2.Sub(t1.Div(t2))        // f2 - t1/(1+t1)

	// Quadratic coefficients: a*R² + b*R + c = 0
	a := NumberOne.Div(t2.Mul(t2))                   // 1 / (1+t1)²
	b := NewNumber(2).Mul(d).Div(t2).Sub(NumberOne.Div(f1)) // 2d/(1+t1) - 1/f1
	c := d.Mul(d).Sub(f2.Mul(f2))                    // d² - f2²

	// Solve quadratic: R = (-b + sqrt(b² - 4ac)) / 2a
	discriminant := b.Mul(b).Sub(NewNumber(4).Mul(a).Mul(c))
	if discriminant.IsNegative() {
		return NumberZero
	}
	R := b.Neg().Add(discriminant.Sqrt()).Div(NewNumber(2).Mul(a))

	return assetBalance.Mul(R)
}

// lpTokensIn calculates LP tokens to burn for single asset withdrawal
// Reference: rippled AMMHelpers.cpp lpTokensIn (Equation 7 from XLS-30d)
// Formula: t = T * (c - sqrt(c² - 4*R)) / 2, where R = b/B, c = R*fee + 2 - fee
func lpTokensIn(assetBalance, assetWithdraw, lptBalance Number, tfee uint16) Number {
	if assetBalance.IsZero() || lptBalance.IsZero() {
		return NumberZero
	}

	R := assetWithdraw.Div(assetBalance)
	fee := getFeeNumber(tfee)
	c := R.Mul(fee).Add(NewNumber(2)).Sub(fee) // R*fee + 2 - fee

	discriminant := c.Mul(c).Sub(NewNumber(4).Mul(R))
	if discriminant.IsNegative() {
		return NumberZero
	}

	tokens := lptBalance.Mul(c.Sub(discriminant.Sqrt())).Div(NewNumber(2))
	if tokens.IsNegative() {
		return NumberZero
	}
	return tokens
}

// ammAssetOut calculates asset amount for LP tokens burned
// Reference: rippled AMMHelpers.cpp ammAssetOut (Equation 8 from XLS-30d)
// Formula: b = B * (t1² - t1*(2-f)) / (t1*f - 1), where t1 = t/T
func ammAssetOut(assetBalance, lptBalance, lpTokens Number, tfee uint16) Number {
	if lptBalance.IsZero() {
		return NumberZero
	}

	fee := getFeeNumber(tfee)
	t1 := lpTokens.Div(lptBalance) // t/T

	// Denominator: t1*f - 1
	denominator := t1.Mul(fee).Sub(NumberOne)
	if denominator.IsZero() {
		return NumberZero
	}

	// Numerator: t1² - t1*(2-f)
	twoMinusFee := NewNumber(2).Sub(fee)
	numerator := t1.Mul(t1).Sub(t1.Mul(twoMinusFee))

	asset := assetBalance.Mul(numerator).Div(denominator)
	if asset.IsNegative() {
		return NumberZero
	}
	return asset
}

// equalDepositTokens calculates LP tokens for equal (proportional) deposit
// Reference: rippled AMMDeposit.cpp equalDepositTokens
func equalDepositTokens(assetBalance1, assetBalance2, lptBalance, amount1, amount2 Number) Number {
	if assetBalance1.IsZero() || assetBalance2.IsZero() || lptBalance.IsZero() {
		return NumberZero
	}

	// Calculate the fraction deposited (use smaller fraction to maintain ratio)
	frac1 := amount1.Div(assetBalance1)
	frac2 := amount2.Div(assetBalance2)

	frac := frac1
	if frac2.Lt(frac1) {
		frac = frac2
	}

	return lptBalance.Mul(frac)
}

// equalWithdrawTokens calculates withdrawal amounts for proportional withdrawal
func equalWithdrawTokens(assetBalance1, assetBalance2, lptBalance, lpTokens Number) (Number, Number) {
	if lptBalance.IsZero() {
		return NumberZero, NumberZero
	}

	frac := lpTokens.Div(lptBalance)
	return assetBalance1.Mul(frac), assetBalance2.Mul(frac)
}

// calculateWeightedFee calculates the weighted average trading fee from vote slots
// Reference: rippled AMMVote.cpp
func calculateWeightedFee(voteSlots []VoteSlotData, lptBalance Number) uint16 {
	if len(voteSlots) == 0 || lptBalance.IsZero() {
		return 0
	}

	numerator := NumberZero
	denominator := NumberZero

	for _, slot := range voteSlots {
		// Convert vote weight back to LP tokens
		lpTokens := NewNumberFromUint64(uint64(slot.VoteWeight)).Mul(lptBalance).Div(NewNumberFromInt(VOTE_WEIGHT_SCALE_FACTOR_VAL))
		if lpTokens.IsZero() {
			continue
		}

		fee := NewNumber(float64(slot.TradingFee))
		numerator = numerator.Add(fee.Mul(lpTokens))
		denominator = denominator.Add(lpTokens)
	}

	if denominator.IsZero() {
		return 0
	}

	avgFee := numerator.Div(denominator)
	result := uint16(avgFee.Float64())
	if result > TRADING_FEE_THRESHOLD_VAL {
		return TRADING_FEE_THRESHOLD_VAL
	}
	return result
}

// ============================================================================
// AMM Keylet and Account Functions
// ============================================================================

// computeAMMKeylet computes the AMM keylet from asset pair
func computeAMMKeylet(asset1, asset2 Asset) keylet.Keylet {
	var issuer1, currency1, issuer2, currency2 [20]byte

	if asset1.Currency != "" && asset1.Currency != "XRP" {
		currency1 = currencyToBytes(asset1.Currency)
		if asset1.Issuer != "" {
			issuerID, _ := decodeAccountID(asset1.Issuer)
			issuer1 = issuerID
		}
	}

	if asset2.Currency != "" && asset2.Currency != "XRP" {
		currency2 = currencyToBytes(asset2.Currency)
		if asset2.Issuer != "" {
			issuerID, _ := decodeAccountID(asset2.Issuer)
			issuer2 = issuerID
		}
	}

	return keylet.AMM(issuer1, currency1, issuer2, currency2)
}

// computeAMMAccountID derives the AMM account ID from the AMM keylet
// Reference: rippled AMMUtils.cpp createPseudoAccount
func computeAMMAccountID(ammKeyletKey [32]byte) [20]byte {
	var result [20]byte
	copy(result[:], ammKeyletKey[:20])
	return result
}

// currencyToBytes converts a currency code to 20-byte representation
func currencyToBytes(currency string) [20]byte {
	var result [20]byte
	if len(currency) == 3 {
		// Standard 3-char code - ASCII in bytes 12-14
		result[12] = currency[0]
		result[13] = currency[1]
		result[14] = currency[2]
	} else if len(currency) == 40 {
		// Hex-encoded currency
		decoded, _ := hex.DecodeString(currency)
		copy(result[:], decoded)
	}
	return result
}

// generateAMMLPTCurrency generates the LP token currency code
// Reference: rippled AMMCore.cpp ammLPTCurrency
// Format: 03 + first 19 bytes of sha512half(min(currency1, currency2) + max(currency1, currency2))
func generateAMMLPTCurrency(currency1, currency2 string) string {
	c1 := currencyToBytes(currency1)
	c2 := currencyToBytes(currency2)

	// Sort currencies (minmax)
	var minC, maxC [20]byte
	if compareCurrencies(c1, c2) < 0 {
		minC, maxC = c1, c2
	} else {
		minC, maxC = c2, c1
	}

	// Hash the concatenation
	data := make([]byte, 40)
	copy(data[:20], minC[:])
	copy(data[20:], maxC[:])
	hash := crypto.Sha512Half(data)

	// Build currency: 03 prefix + first 19 bytes of hash
	var lptCurrency [20]byte
	lptCurrency[0] = 0x03
	copy(lptCurrency[1:], hash[:19])

	return strings.ToUpper(hex.EncodeToString(lptCurrency[:]))
}

// compareCurrencies compares two currency byte arrays
func compareCurrencies(a, b [20]byte) int {
	for i := 0; i < 20; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// ============================================================================
// AMM Parsing and Serialization
// ============================================================================

// parseAMMData parses AMM data from binary format
func parseAMMData(data []byte) (*AMMData, error) {
	amm := &AMMData{
		VoteSlots:      make([]VoteSlotData, 0),
		LPTokenBalance: NumberZero,
	}
	// Simplified parsing - in production would use full binary codec
	return amm, nil
}

// serializeAMMData serializes AMM data to binary format
func serializeAMMData(amm *AMMData) ([]byte, error) {
	accountAddress, err := addresscodec.EncodeAccountIDToClassicAddress(amm.Account[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode AMM account address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "AMM",
		"Account":         accountAddress,
		"TradingFee":      amm.TradingFee,
		"Flags":           uint32(0),
	}

	// Add LPTokenBalance
	lpBalance := amm.LPTokenBalance.Uint64()
	jsonObj["LPTokenBalance"] = map[string]any{
		"currency": amm.Asset.Currency,
		"issuer":   accountAddress,
		"value":    fmt.Sprintf("%d", lpBalance),
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode AMM: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// serializeAMM serializes an AMM ledger entry
func serializeAMM(amm *AMMData, ownerID [20]byte) ([]byte, error) {
	return serializeAMMData(amm)
}

// formatAsset formats an asset for metadata
func formatAsset(asset Asset) map[string]string {
	if asset.Currency == "" || asset.Currency == "XRP" {
		return map[string]string{"currency": "XRP"}
	}
	return map[string]string{
		"currency": asset.Currency,
		"issuer":   asset.Issuer,
	}
}

// compareAccountIDs compares two account IDs for ordering
func compareAccountIDs(a, b [20]byte) int {
	for i := 0; i < 20; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// ============================================================================
// AMMCreate Implementation
// Reference: rippled AMMCreate.cpp
// ============================================================================

// applyAMMCreate applies an AMMCreate transaction
func (e *Engine) applyAMMCreate(tx *AMMCreate, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Build assets for keylet computation
	asset1 := Asset{Currency: tx.Amount.Currency, Issuer: tx.Amount.Issuer}
	asset2 := Asset{Currency: tx.Amount2.Currency, Issuer: tx.Amount2.Issuer}

	// Compute the AMM keylet from the asset pair
	ammKey := computeAMMKeylet(asset1, asset2)

	// Check if AMM already exists
	exists, _ := e.view.Exists(ammKey)
	if exists {
		return TecDUPLICATE
	}

	// Compute the AMM account ID from keylet
	ammAccountID := computeAMMAccountID(ammKey.Key)
	ammAccountAddr, _ := encodeAccountID(ammAccountID)

	// Check if AMM account already exists
	ammAccountKey := keylet.Account(ammAccountID)
	acctExists, _ := e.view.Exists(ammAccountKey)
	if acctExists {
		return TecDUPLICATE
	}

	// Parse amounts
	amount1 := parseAmountToNumber(tx.Amount.Value)
	amount2 := parseAmountToNumber(tx.Amount2.Value)

	// Validate amounts are positive
	if amount1.Le(NumberZero) || amount2.Le(NumberZero) {
		return TemBAD_AMOUNT
	}

	// Check for XRP amounts
	isXRP1 := tx.Amount.Currency == "" || tx.Amount.Currency == "XRP"
	isXRP2 := tx.Amount2.Currency == "" || tx.Amount2.Currency == "XRP"

	// Verify sufficient balance for XRP
	totalXRPNeeded := uint64(0)
	if isXRP1 {
		totalXRPNeeded += amount1.Uint64()
	}
	if isXRP2 {
		totalXRPNeeded += amount2.Uint64()
	}
	if totalXRPNeeded > 0 && account.Balance < totalXRPNeeded {
		return TecUNFUNDED_AMM
	}

	// Calculate initial LP token balance: sqrt(amount1 * amount2)
	lpTokenBalance := ammLPTokens(amount1, amount2)
	if lpTokenBalance.Le(NumberZero) {
		return TecAMM_BALANCE
	}

	// Generate LP token currency code
	lptCurrency := generateAMMLPTCurrency(tx.Amount.Currency, tx.Amount2.Currency)

	// Create the AMM pseudo-account with lsfAMM flag
	ammAccount := &AccountRoot{
		Account:    ammAccountAddr,
		Balance:    0,
		Sequence:   0,
		OwnerCount: 1,
		Flags:      lsfAMM,
	}

	// Create the AMM entry
	ammData := &AMMData{
		Account:        ammAccountID,
		Asset:          asset1,
		Asset2:         asset2,
		TradingFee:     tx.TradingFee,
		LPTokenBalance: lpTokenBalance,
		VoteSlots:      make([]VoteSlotData, 0),
	}

	// Initialize creator's vote slot
	voteWeight := uint32(VOTE_WEIGHT_SCALE_FACTOR_VAL) // Creator holds all LP tokens initially
	creatorVote := VoteSlotData{
		Account:    accountID,
		TradingFee: tx.TradingFee,
		VoteWeight: voteWeight,
	}
	ammData.VoteSlots = append(ammData.VoteSlots, creatorVote)

	// Initialize auction slot with zero price (unowned)
	ammData.AuctionSlot = &AuctionSlotData{
		Expiration:    0,
		Price:         NumberZero,
		DiscountedFee: tx.TradingFee / AUCTION_SLOT_DISCOUNTED_FEE_FRACTION_VAL,
		AuthAccounts:  make([][20]byte, 0),
	}

	// Store the AMM pseudo-account
	ammAccountBytes, err := serializeAccountRoot(ammAccount)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Insert(ammAccountKey, ammAccountBytes); err != nil {
		return TefINTERNAL
	}

	// Store the AMM entry
	ammBytes, err := serializeAMM(ammData, accountID)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Insert(ammKey, ammBytes); err != nil {
		return TefINTERNAL
	}

	// Transfer assets from creator to AMM account
	if isXRP1 {
		account.Balance -= amount1.Uint64()
		ammAccount.Balance += amount1.Uint64()
	}
	if isXRP2 {
		account.Balance -= amount2.Uint64()
		ammAccount.Balance += amount2.Uint64()
	}

	// For IOU transfers, update trustlines
	if !isXRP1 {
		if err := e.createOrUpdateAMMTrustline(ammAccountID, asset1, amount1.Uint64()); err != nil {
			return TecNO_LINE
		}
		if err := e.updateTrustlineBalance(accountID, asset1, -int64(amount1.Uint64())); err != nil {
			return TecUNFUNDED_AMM
		}
	}
	if !isXRP2 {
		if err := e.createOrUpdateAMMTrustline(ammAccountID, asset2, amount2.Uint64()); err != nil {
			return TecNO_LINE
		}
		if err := e.updateTrustlineBalance(accountID, asset2, -int64(amount2.Uint64())); err != nil {
			return TecUNFUNDED_AMM
		}
	}

	// Create LP token trustline for creator
	lptAsset := Asset{
		Currency: lptCurrency,
		Issuer:   ammAccountAddr,
	}
	if err := e.createLPTokenTrustline(accountID, lptAsset, lpTokenBalance.Uint64()); err != nil {
		return TecINSUF_RESERVE_LINE
	}

	// Update creator account owner count
	account.OwnerCount++

	// Update AMM account balance
	ammAccountBytes, err = serializeAccountRoot(ammAccount)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(ammAccountKey, ammAccountBytes); err != nil {
		return TefINTERNAL
	}

	// Record metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(ammKey.Key[:])),
		NewFields: map[string]any{
			"Account":        ammAccountAddr,
			"Asset":          formatAsset(asset1),
			"Asset2":         formatAsset(asset2),
			"LPTokenBalance": lpTokenBalance.Uint64(),
			"TradingFee":     tx.TradingFee,
		},
	})

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(ammAccountKey.Key[:])),
		NewFields: map[string]any{
			"Account": ammAccountAddr,
			"Flags":   lsfAMM,
		},
	})

	return TesSUCCESS
}

// createOrUpdateAMMTrustline creates or updates a trustline for the AMM account
func (e *Engine) createOrUpdateAMMTrustline(ammAccountID [20]byte, asset Asset, balance uint64) error {
	if asset.Currency == "" || asset.Currency == "XRP" {
		return nil
	}

	issuerID, err := decodeAccountID(asset.Issuer)
	if err != nil {
		return err
	}

	lineKey := keylet.Line(ammAccountID, issuerID, asset.Currency)

	trustlineData := map[string]any{
		"LedgerEntryType": "RippleState",
		"Balance": map[string]any{
			"currency": asset.Currency,
			"issuer":   asset.Issuer,
			"value":    fmt.Sprintf("%d", balance),
		},
		"Flags": uint32(0x00020000), // lsfAMMNode
	}

	encoded, err := binarycodec.Encode(trustlineData)
	if err != nil {
		return err
	}
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return err
	}

	return e.view.Insert(lineKey, decoded)
}

// updateTrustlineBalance updates a trustline balance
func (e *Engine) updateTrustlineBalance(accountID [20]byte, asset Asset, delta int64) error {
	if asset.Currency == "" || asset.Currency == "XRP" {
		return nil
	}
	// In a full implementation, this would read, update, and re-serialize the trustline
	return nil
}

// createLPTokenTrustline creates an LP token trustline for a liquidity provider
func (e *Engine) createLPTokenTrustline(accountID [20]byte, lptAsset Asset, balance uint64) error {
	issuerID, err := decodeAccountID(lptAsset.Issuer)
	if err != nil {
		return err
	}

	lineKey := keylet.Line(accountID, issuerID, lptAsset.Currency)

	trustlineData := map[string]any{
		"LedgerEntryType": "RippleState",
		"Balance": map[string]any{
			"currency": lptAsset.Currency,
			"issuer":   lptAsset.Issuer,
			"value":    fmt.Sprintf("%d", balance),
		},
		"Flags": uint32(0),
	}

	encoded, err := binarycodec.Encode(trustlineData)
	if err != nil {
		return err
	}
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return err
	}

	return e.view.Insert(lineKey, decoded)
}

// ============================================================================
// AMMDeposit Implementation
// Reference: rippled AMMDeposit.cpp
// ============================================================================

// applyAMMDeposit applies an AMMDeposit transaction
func (e *Engine) applyAMMDeposit(tx *AMMDeposit, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Find the AMM
	ammKey := computeAMMKeylet(tx.Asset, tx.Asset2)
	ammData, err := e.view.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	amm, err := parseAMMData(ammData)
	if err != nil {
		return TefINTERNAL
	}

	// Get AMM account
	ammAccountID := computeAMMAccountID(ammKey.Key)
	ammAccountKey := keylet.Account(ammAccountID)
	ammAccountData, err := e.view.Read(ammAccountKey)
	if err != nil {
		return TefINTERNAL
	}
	ammAccount, err := parseAccountRoot(ammAccountData)
	if err != nil {
		return TefINTERNAL
	}

	flags := tx.GetFlags()
	tfee := amm.TradingFee

	// Parse amounts
	var amount1, amount2, lpTokensRequested Number
	if tx.Amount != nil {
		amount1 = parseAmountToNumber(tx.Amount.Value)
	}
	if tx.Amount2 != nil {
		amount2 = parseAmountToNumber(tx.Amount2.Value)
	}
	if tx.LPTokenOut != nil {
		lpTokensRequested = parseAmountToNumber(tx.LPTokenOut.Value)
	}

	// Get current AMM balances
	assetBalance1 := NewNumberFromUint64(ammAccount.Balance) // For XRP
	assetBalance2 := NumberZero                              // Would come from trustline for IOU
	lptBalance := amm.LPTokenBalance

	var lpTokensToIssue, depositAmount1, depositAmount2 Number

	// Handle different deposit modes
	switch {
	case flags&tfLPToken != 0:
		// Proportional deposit for specified LP tokens
		if lpTokensRequested.Le(NumberZero) || lptBalance.Le(NumberZero) {
			return TecAMM_INVALID_TOKENS
		}
		frac := lpTokensRequested.Div(lptBalance)
		depositAmount1 = assetBalance1.Mul(frac)
		depositAmount2 = assetBalance2.Mul(frac)
		lpTokensToIssue = lpTokensRequested

	case flags&tfSingleAsset != 0:
		// Single asset deposit
		lpTokensToIssue = lpTokensOut(assetBalance1, amount1, lptBalance, tfee)
		if lpTokensToIssue.Le(NumberZero) {
			return TecAMM_INVALID_TOKENS
		}
		depositAmount1 = amount1

	case flags&tfTwoAsset != 0:
		// Two asset deposit with limits
		if assetBalance1.IsZero() || assetBalance2.IsZero() {
			return TecAMM_BALANCE
		}
		frac1 := amount1.Div(assetBalance1)
		frac2 := amount2.Div(assetBalance2)
		frac := frac1.Min(frac2)
		lpTokensToIssue = lptBalance.Mul(frac)
		depositAmount1 = assetBalance1.Mul(frac)
		depositAmount2 = assetBalance2.Mul(frac)

	case flags&tfOneAssetLPToken != 0:
		// Single asset deposit for specific LP tokens
		depositAmount1 = ammAssetIn(assetBalance1, lptBalance, lpTokensRequested, tfee)
		if depositAmount1.Gt(amount1) {
			return TecAMM_FAILED
		}
		lpTokensToIssue = lpTokensRequested

	case flags&tfLimitLPToken != 0:
		// Single asset deposit with effective price limit
		lpTokensToIssue = lpTokensOut(assetBalance1, amount1, lptBalance, tfee)
		if lpTokensToIssue.Le(NumberZero) {
			return TecAMM_INVALID_TOKENS
		}
		if tx.EPrice != nil {
			ePrice := parseAmountToNumber(tx.EPrice.Value)
			if !ePrice.IsZero() {
				effectivePrice := amount1.Div(lpTokensToIssue)
				if effectivePrice.Gt(ePrice) {
					return TecAMM_FAILED
				}
			}
		}
		depositAmount1 = amount1

	case flags&tfTwoAssetIfEmpty != 0:
		// Deposit into empty AMM
		if !lptBalance.IsZero() {
			return TecAMM_NOT_EMPTY
		}
		lpTokensToIssue = ammLPTokens(amount1, amount2)
		depositAmount1 = amount1
		depositAmount2 = amount2
		if tx.TradingFee > 0 {
			amm.TradingFee = tx.TradingFee
		}

	default:
		return TemMALFORMED
	}

	if lpTokensToIssue.Le(NumberZero) {
		return TecAMM_INVALID_TOKENS
	}

	// Check depositor has sufficient balance
	isXRP1 := tx.Asset.Currency == "" || tx.Asset.Currency == "XRP"
	isXRP2 := tx.Asset2.Currency == "" || tx.Asset2.Currency == "XRP"

	totalXRPNeeded := uint64(0)
	if isXRP1 && !depositAmount1.IsZero() {
		totalXRPNeeded += depositAmount1.Uint64()
	}
	if isXRP2 && !depositAmount2.IsZero() {
		totalXRPNeeded += depositAmount2.Uint64()
	}
	if totalXRPNeeded > 0 && account.Balance < totalXRPNeeded {
		return TecUNFUNDED_AMM
	}

	// Transfer assets from depositor to AMM
	if isXRP1 && !depositAmount1.IsZero() {
		account.Balance -= depositAmount1.Uint64()
		ammAccount.Balance += depositAmount1.Uint64()
	}
	if isXRP2 && !depositAmount2.IsZero() {
		account.Balance -= depositAmount2.Uint64()
		ammAccount.Balance += depositAmount2.Uint64()
	}

	// Issue LP tokens to depositor
	amm.LPTokenBalance = amm.LPTokenBalance.Add(lpTokensToIssue)

	// Update LP token trustline for depositor
	ammAccountAddr, _ := encodeAccountID(ammAccountID)
	lptCurrency := generateAMMLPTCurrency(tx.Asset.Currency, tx.Asset2.Currency)
	lptAsset := Asset{Currency: lptCurrency, Issuer: ammAccountAddr}
	e.createLPTokenTrustline(accountID, lptAsset, lpTokensToIssue.Uint64())

	// Persist updated AMM
	ammBytes, err := serializeAMMData(amm)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(ammKey, ammBytes); err != nil {
		return TefINTERNAL
	}

	// Persist updated AMM account
	ammAccountBytes, err := serializeAccountRoot(ammAccount)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(ammAccountKey, ammAccountBytes); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(ammKey.Key[:])),
		FinalFields: map[string]any{
			"LPTokenBalance": amm.LPTokenBalance.Uint64(),
		},
	})

	return TesSUCCESS
}

// ============================================================================
// AMMWithdraw Implementation
// Reference: rippled AMMWithdraw.cpp
// ============================================================================

// applyAMMWithdraw applies an AMMWithdraw transaction
func (e *Engine) applyAMMWithdraw(tx *AMMWithdraw, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	_ = accountID // Used for LP token trustline operations (simplified)

	// Find the AMM
	ammKey := computeAMMKeylet(tx.Asset, tx.Asset2)
	ammData, err := e.view.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	amm, err := parseAMMData(ammData)
	if err != nil {
		return TefINTERNAL
	}

	// Get AMM account
	ammAccountID := computeAMMAccountID(ammKey.Key)
	ammAccountKey := keylet.Account(ammAccountID)
	ammAccountData, err := e.view.Read(ammAccountKey)
	if err != nil {
		return TefINTERNAL
	}
	ammAccount, err := parseAccountRoot(ammAccountData)
	if err != nil {
		return TefINTERNAL
	}

	flags := tx.GetFlags()
	tfee := amm.TradingFee

	// Parse amounts
	var amount1, amount2, lpTokensRequested Number
	if tx.Amount != nil {
		amount1 = parseAmountToNumber(tx.Amount.Value)
	}
	if tx.Amount2 != nil {
		amount2 = parseAmountToNumber(tx.Amount2.Value)
	}
	if tx.LPTokenIn != nil {
		lpTokensRequested = parseAmountToNumber(tx.LPTokenIn.Value)
	}

	// Get current AMM balances
	assetBalance1 := NewNumberFromUint64(ammAccount.Balance)
	assetBalance2 := NumberZero
	lptBalance := amm.LPTokenBalance

	if lptBalance.Le(NumberZero) {
		return TecAMM_BALANCE
	}

	// Get withdrawer's LP token balance (simplified)
	lpTokensHeld := lpTokensRequested
	if flags&(tfWithdrawAll|tfOneAssetWithdrawAll) != 0 {
		lpTokensHeld = lptBalance
	}

	var lpTokensToRedeem, withdrawAmount1, withdrawAmount2 Number

	// Handle different withdrawal modes
	switch {
	case flags&tfLPToken != 0:
		// Proportional withdrawal for specified LP tokens
		if lpTokensRequested.Le(NumberZero) || lptBalance.Le(NumberZero) {
			return TecAMM_INVALID_TOKENS
		}
		if lpTokensRequested.Gt(lpTokensHeld) || lpTokensRequested.Gt(lptBalance) {
			return TecAMM_INVALID_TOKENS
		}
		withdrawAmount1, withdrawAmount2 = equalWithdrawTokens(assetBalance1, assetBalance2, lptBalance, lpTokensRequested)
		lpTokensToRedeem = lpTokensRequested

	case flags&tfWithdrawAll != 0:
		// Withdraw all LP tokens held
		if lpTokensHeld.Le(NumberZero) {
			return TecAMM_INVALID_TOKENS
		}
		if lpTokensHeld.Ge(lptBalance) {
			withdrawAmount1 = assetBalance1
			withdrawAmount2 = assetBalance2
			lpTokensToRedeem = lptBalance
		} else {
			withdrawAmount1, withdrawAmount2 = equalWithdrawTokens(assetBalance1, assetBalance2, lptBalance, lpTokensHeld)
			lpTokensToRedeem = lpTokensHeld
		}

	case flags&tfOneAssetWithdrawAll != 0:
		// Withdraw all LP tokens as single asset
		if lpTokensHeld.Le(NumberZero) {
			return TecAMM_INVALID_TOKENS
		}
		withdrawAmount1 = ammAssetOut(assetBalance1, lptBalance, lpTokensHeld, tfee)
		if withdrawAmount1.Gt(assetBalance1) {
			return TecAMM_BALANCE
		}
		lpTokensToRedeem = lpTokensHeld

	case flags&tfSingleAsset != 0:
		// Single asset withdrawal
		if amount1.Le(NumberZero) {
			return TemMALFORMED
		}
		if amount1.Gt(assetBalance1) {
			return TecAMM_BALANCE
		}
		lpTokensToRedeem = lpTokensIn(assetBalance1, amount1, lptBalance, tfee)
		if lpTokensToRedeem.Le(NumberZero) || lpTokensToRedeem.Gt(lpTokensHeld) {
			return TecAMM_INVALID_TOKENS
		}
		withdrawAmount1 = amount1

	case flags&tfTwoAsset != 0:
		// Two asset withdrawal with limits
		if amount1.Le(NumberZero) || amount2.Le(NumberZero) {
			return TemMALFORMED
		}
		frac1 := amount1.Div(assetBalance1)
		frac2 := amount2.Div(assetBalance2)
		frac := frac1.Min(frac2)
		lpTokensToRedeem = lptBalance.Mul(frac)
		if lpTokensToRedeem.Le(NumberZero) || lpTokensToRedeem.Gt(lpTokensHeld) {
			return TecAMM_INVALID_TOKENS
		}
		withdrawAmount1, withdrawAmount2 = equalWithdrawTokens(assetBalance1, assetBalance2, lptBalance, lpTokensToRedeem)

	case flags&tfOneAssetLPToken != 0:
		// Single asset withdrawal for specific LP tokens
		if lpTokensRequested.Le(NumberZero) {
			return TecAMM_INVALID_TOKENS
		}
		if lpTokensRequested.Gt(lpTokensHeld) || lpTokensRequested.Gt(lptBalance) {
			return TecAMM_INVALID_TOKENS
		}
		withdrawAmount1 = ammAssetOut(assetBalance1, lptBalance, lpTokensRequested, tfee)
		if withdrawAmount1.Gt(assetBalance1) {
			return TecAMM_BALANCE
		}
		if !amount1.IsZero() && withdrawAmount1.Lt(amount1) {
			return TecAMM_FAILED
		}
		lpTokensToRedeem = lpTokensRequested

	case flags&tfLimitLPToken != 0:
		// Single asset withdrawal with effective price limit
		if amount1.Le(NumberZero) || tx.EPrice == nil {
			return TemMALFORMED
		}
		ePrice := parseAmountToNumber(tx.EPrice.Value)
		if ePrice.Le(NumberZero) {
			return TemMALFORMED
		}
		lpTokensToRedeem = lpTokensIn(assetBalance1, amount1, lptBalance, tfee)
		if lpTokensToRedeem.Le(NumberZero) || lpTokensToRedeem.Gt(lpTokensHeld) {
			return TecAMM_INVALID_TOKENS
		}
		// Check effective price: EP = lpTokens / amount
		actualEP := lpTokensToRedeem.Div(amount1)
		if actualEP.Gt(ePrice) {
			return TecAMM_FAILED
		}
		withdrawAmount1 = amount1

	default:
		return TemMALFORMED
	}

	if lpTokensToRedeem.Le(NumberZero) {
		return TecAMM_INVALID_TOKENS
	}

	// Verify withdrawal doesn't exceed balances
	if withdrawAmount1.Gt(assetBalance1) || withdrawAmount2.Gt(assetBalance2) {
		return TecAMM_BALANCE
	}

	// Transfer assets from AMM to withdrawer
	isXRP1 := tx.Asset.Currency == "" || tx.Asset.Currency == "XRP"
	isXRP2 := tx.Asset2.Currency == "" || tx.Asset2.Currency == "XRP"

	if isXRP1 && !withdrawAmount1.IsZero() {
		ammAccount.Balance -= withdrawAmount1.Uint64()
		account.Balance += withdrawAmount1.Uint64()
	}
	if isXRP2 && !withdrawAmount2.IsZero() {
		ammAccount.Balance -= withdrawAmount2.Uint64()
		account.Balance += withdrawAmount2.Uint64()
	}

	// Redeem LP tokens
	newLPBalance := lptBalance.Sub(lpTokensToRedeem)
	amm.LPTokenBalance = newLPBalance

	// Check if AMM should be deleted
	ammDeleted := false
	if newLPBalance.Le(NumberZero) {
		if err := e.view.Erase(ammKey); err != nil {
			return TefINTERNAL
		}
		if err := e.view.Erase(ammAccountKey); err != nil {
			return TefINTERNAL
		}
		ammDeleted = true
	}

	if !ammDeleted {
		ammBytes, err := serializeAMMData(amm)
		if err != nil {
			return TefINTERNAL
		}
		if err := e.view.Update(ammKey, ammBytes); err != nil {
			return TefINTERNAL
		}

		ammAccountBytes, err := serializeAccountRoot(ammAccount)
		if err != nil {
			return TefINTERNAL
		}
		if err := e.view.Update(ammAccountKey, ammAccountBytes); err != nil {
			return TefINTERNAL
		}
	}

	if ammDeleted {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "AMM",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(ammKey.Key[:])),
		})
	} else {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "AMM",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(ammKey.Key[:])),
			FinalFields: map[string]any{
				"LPTokenBalance": amm.LPTokenBalance.Uint64(),
			},
		})
	}

	return TesSUCCESS
}

// ============================================================================
// AMMVote Implementation
// Reference: rippled AMMVote.cpp
// ============================================================================

// applyAMMVote applies an AMMVote transaction
func (e *Engine) applyAMMVote(tx *AMMVote, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Find the AMM
	ammKey := computeAMMKeylet(tx.Asset, tx.Asset2)
	ammData, err := e.view.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	amm, err := parseAMMData(ammData)
	if err != nil {
		return TefINTERNAL
	}

	lptAMMBalance := amm.LPTokenBalance
	if lptAMMBalance.Le(NumberZero) {
		return TecAMM_BALANCE
	}

	// Get voter's LP token balance (simplified - would read from trustline)
	lpTokensNew := NewNumberFromUint64(1000000)
	feeNew := tx.TradingFee

	// Track minimum token holder for potential replacement
	var minTokens Number = NewNumber(math.MaxFloat64)
	var minPos int = -1
	var minAccount [20]byte
	var minFee uint16

	// Build updated vote slots
	updatedVoteSlots := make([]VoteSlotData, 0, VOTE_MAX_SLOTS_VAL)
	foundAccount := false

	numerator := NumberZero
	denominator := NumberZero

	for i, slot := range amm.VoteSlots {
		lpTokens := NewNumberFromUint64(uint64(slot.VoteWeight)).Mul(lptAMMBalance).Div(NewNumberFromInt(VOTE_WEIGHT_SCALE_FACTOR_VAL))
		if lpTokens.Le(NumberZero) {
			continue
		}

		feeVal := slot.TradingFee

		if slot.Account == accountID {
			lpTokens = lpTokensNew
			feeVal = feeNew
			foundAccount = true
		}

		voteWeight := lpTokens.Mul(NewNumberFromInt(VOTE_WEIGHT_SCALE_FACTOR_VAL)).Div(lptAMMBalance)

		numerator = numerator.Add(NewNumber(float64(feeVal)).Mul(lpTokens))
		denominator = denominator.Add(lpTokens)

		if lpTokens.Lt(minTokens) ||
			(lpTokens.Float64() == minTokens.Float64() && feeVal < minFee) ||
			(lpTokens.Float64() == minTokens.Float64() && feeVal == minFee && compareAccountIDs(slot.Account, minAccount) < 0) {
			minTokens = lpTokens
			minPos = i
			minAccount = slot.Account
			minFee = feeVal
		}

		updatedVoteSlots = append(updatedVoteSlots, VoteSlotData{
			Account:    slot.Account,
			TradingFee: feeVal,
			VoteWeight: uint32(voteWeight.Uint64()),
		})
	}

	if !foundAccount {
		voteWeight := lpTokensNew.Mul(NewNumberFromInt(VOTE_WEIGHT_SCALE_FACTOR_VAL)).Div(lptAMMBalance)

		if len(updatedVoteSlots) < VOTE_MAX_SLOTS_VAL {
			updatedVoteSlots = append(updatedVoteSlots, VoteSlotData{
				Account:    accountID,
				TradingFee: feeNew,
				VoteWeight: uint32(voteWeight.Uint64()),
			})
			numerator = numerator.Add(NewNumber(float64(feeNew)).Mul(lpTokensNew))
			denominator = denominator.Add(lpTokensNew)
		} else if lpTokensNew.Gt(minTokens) || (lpTokensNew.Float64() == minTokens.Float64() && feeNew > minFee) {
			if minPos >= 0 && minPos < len(updatedVoteSlots) {
				numerator = numerator.Sub(NewNumber(float64(minFee)).Mul(minTokens))
				denominator = denominator.Sub(minTokens)

				updatedVoteSlots[minPos] = VoteSlotData{
					Account:    accountID,
					TradingFee: feeNew,
					VoteWeight: uint32(voteWeight.Uint64()),
				}

				numerator = numerator.Add(NewNumber(float64(feeNew)).Mul(lpTokensNew))
				denominator = denominator.Add(lpTokensNew)
			}
		}
	}

	// Calculate weighted average trading fee
	var newTradingFee uint16 = 0
	if !denominator.IsZero() {
		avgFee := numerator.Div(denominator)
		newTradingFee = uint16(avgFee.Uint64())
		if newTradingFee > TRADING_FEE_THRESHOLD_VAL {
			newTradingFee = TRADING_FEE_THRESHOLD_VAL
		}
	}

	amm.VoteSlots = updatedVoteSlots
	amm.TradingFee = newTradingFee

	// Update discounted fee in auction slot
	if amm.AuctionSlot != nil {
		amm.AuctionSlot.DiscountedFee = newTradingFee / AUCTION_SLOT_DISCOUNTED_FEE_FRACTION_VAL
	}

	ammBytes, err := serializeAMMData(amm)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(ammKey, ammBytes); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(ammKey.Key[:])),
		FinalFields: map[string]any{
			"TradingFee": newTradingFee,
		},
	})

	return TesSUCCESS
}

// ============================================================================
// AMMBid Implementation
// Reference: rippled AMMBid.cpp
// ============================================================================

// applyAMMBid applies an AMMBid transaction
func (e *Engine) applyAMMBid(tx *AMMBid, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Find the AMM
	ammKey := computeAMMKeylet(tx.Asset, tx.Asset2)
	ammData, err := e.view.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	amm, err := parseAMMData(ammData)
	if err != nil {
		return TefINTERNAL
	}

	lptAMMBalance := amm.LPTokenBalance
	if lptAMMBalance.Le(NumberZero) {
		return TecAMM_BALANCE
	}

	// Get bidder's LP token balance (simplified)
	lpTokens := NewNumberFromUint64(1000000)

	// Parse bid amounts
	var bidMin, bidMax Number
	if tx.BidMin != nil {
		bidMin = parseAmountToNumber(tx.BidMin.Value)
		if bidMin.Gt(lpTokens) || bidMin.Ge(lptAMMBalance) {
			return TecAMM_INVALID_TOKENS
		}
	}
	if tx.BidMax != nil {
		bidMax = parseAmountToNumber(tx.BidMax.Value)
		if bidMax.Gt(lpTokens) || bidMax.Ge(lptAMMBalance) {
			return TecAMM_INVALID_TOKENS
		}
	}
	if !bidMin.IsZero() && !bidMax.IsZero() && bidMin.Gt(bidMax) {
		return TecAMM_INVALID_TOKENS
	}

	tradingFee := getFeeNumber(amm.TradingFee)
	minSlotPrice := lptAMMBalance.Mul(tradingFee).Div(NewNumberFromInt(AUCTION_SLOT_MIN_FEE_FRACTION_VAL))
	discountedFee := amm.TradingFee / AUCTION_SLOT_DISCOUNTED_FEE_FRACTION_VAL

	// Get current time (simplified)
	currentTime := e.config.ParentCloseTime

	// Initialize auction slot if needed
	if amm.AuctionSlot == nil {
		amm.AuctionSlot = &AuctionSlotData{
			AuthAccounts: make([][20]byte, 0),
		}
	}

	// Calculate time slot
	var timeSlot *int
	if amm.AuctionSlot.Expiration > 0 && currentTime < amm.AuctionSlot.Expiration {
		start := amm.AuctionSlot.Expiration - AUCTION_SLOT_TOTAL_TIME_SECS_VAL
		if currentTime >= start {
			slot := int((currentTime - start) / AUCTION_SLOT_INTERVAL_DURATION_VAL)
			if slot >= 0 && slot < AUCTION_SLOT_TIME_INTERVALS_VAL {
				timeSlot = &slot
			}
		}
	}

	// Check if current owner is valid
	validOwner := false
	if timeSlot != nil && *timeSlot < AUCTION_SLOT_TIME_INTERVALS_VAL-1 {
		var zeroAccount [20]byte
		if amm.AuctionSlot.Account != zeroAccount {
			ownerKey := keylet.Account(amm.AuctionSlot.Account)
			exists, _ := e.view.Exists(ownerKey)
			validOwner = exists
		}
	}

	// Calculate pay price
	var computedPrice Number
	var fractionRemaining Number = NumberZero
	pricePurchased := amm.AuctionSlot.Price

	if !validOwner || timeSlot == nil {
		computedPrice = minSlotPrice
	} else {
		fractionUsed := NewNumber(float64(*timeSlot+1) / float64(AUCTION_SLOT_TIME_INTERVALS_VAL))
		fractionRemaining = NumberOne.Sub(fractionUsed)

		if *timeSlot == 0 {
			computedPrice = pricePurchased.Mul(NewNumber(1.05)).Add(minSlotPrice)
		} else {
			decay := NumberOne.Sub(fractionUsed.PowerFloat(60))
			computedPrice = pricePurchased.Mul(NewNumber(1.05)).Mul(decay).Add(minSlotPrice)
		}
	}

	// Determine actual pay price
	var payPrice Number
	if !bidMin.IsZero() && !bidMax.IsZero() {
		if computedPrice.Le(bidMax) {
			payPrice = computedPrice.Max(bidMin)
		} else {
			return TecAMM_FAILED
		}
	} else if !bidMin.IsZero() {
		payPrice = computedPrice.Max(bidMin)
	} else if !bidMax.IsZero() {
		if computedPrice.Le(bidMax) {
			payPrice = computedPrice
		} else {
			return TecAMM_FAILED
		}
	} else {
		payPrice = computedPrice
	}

	if payPrice.Gt(lpTokens) {
		return TecAMM_INVALID_TOKENS
	}

	// Calculate refund and burn
	var refund Number = NumberZero
	burn := payPrice

	if validOwner && timeSlot != nil {
		refund = fractionRemaining.Mul(pricePurchased)
		if refund.Gt(payPrice) {
			return TefINTERNAL
		}
		burn = payPrice.Sub(refund)
	}

	// Burn tokens
	if burn.Ge(lptAMMBalance) {
		return TefINTERNAL
	}
	amm.LPTokenBalance = amm.LPTokenBalance.Sub(burn)

	// Update auction slot
	amm.AuctionSlot.Account = accountID
	amm.AuctionSlot.Expiration = currentTime + AUCTION_SLOT_TOTAL_TIME_SECS_VAL
	amm.AuctionSlot.Price = payPrice
	amm.AuctionSlot.DiscountedFee = discountedFee

	// Parse auth accounts
	if tx.AuthAccounts != nil {
		amm.AuctionSlot.AuthAccounts = make([][20]byte, 0, len(tx.AuthAccounts))
		for _, authAccountEntry := range tx.AuthAccounts {
			authAccountID, err := decodeAccountID(authAccountEntry.AuthAccount.Account)
			if err == nil {
				amm.AuctionSlot.AuthAccounts = append(amm.AuctionSlot.AuthAccounts, authAccountID)
			}
		}
	} else {
		amm.AuctionSlot.AuthAccounts = make([][20]byte, 0)
	}

	ammBytes, err := serializeAMMData(amm)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(ammKey, ammBytes); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(ammKey.Key[:])),
		FinalFields: map[string]any{
			"LPTokenBalance": amm.LPTokenBalance.Uint64(),
			"AuctionSlot": map[string]any{
				"Account":    tx.Account,
				"Expiration": amm.AuctionSlot.Expiration,
				"Price":      amm.AuctionSlot.Price.Uint64(),
			},
		},
	})

	return TesSUCCESS
}

// ============================================================================
// AMMDelete Implementation
// Reference: rippled AMMDelete.cpp
// ============================================================================

// applyAMMDelete applies an AMMDelete transaction
func (e *Engine) applyAMMDelete(tx *AMMDelete, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammKey := computeAMMKeylet(tx.Asset, tx.Asset2)

	ammData, err := e.view.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	amm, err := parseAMMData(ammData)
	if err != nil {
		return TefINTERNAL
	}

	// Can only delete empty AMM (LP token balance = 0)
	if !amm.LPTokenBalance.IsZero() {
		return TecAMM_NOT_EMPTY
	}

	// Delete the AMM entry
	if err := e.view.Erase(ammKey); err != nil {
		return TefINTERNAL
	}

	// Delete the AMM account
	ammAccountID := computeAMMAccountID(ammKey.Key)
	ammAccountKey := keylet.Account(ammAccountID)
	if err := e.view.Erase(ammAccountKey); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(ammKey.Key[:])),
	})

	return TesSUCCESS
}

// ============================================================================
// AMMClawback Implementation
// Reference: rippled AMMClawback.cpp
// ============================================================================

// applyAMMClawback applies an AMMClawback transaction
func (e *Engine) applyAMMClawback(tx *AMMClawback, account *AccountRoot, metadata *Metadata) Result {
	issuerID, _ := decodeAccountID(tx.Account)

	// Verify issuer permissions
	if (account.Flags & lsfAllowTrustLineClawback) == 0 {
		return TecNO_PERMISSION
	}
	if (account.Flags & lsfNoFreeze) != 0 {
		return TecNO_PERMISSION
	}

	// Find the holder
	holderID, err := decodeAccountID(tx.Holder)
	if err != nil {
		return TemINVALID
	}

	holderKey := keylet.Account(holderID)
	holderData, err := e.view.Read(holderKey)
	if err != nil {
		return TerNO_ACCOUNT
	}
	holderAccount, err := parseAccountRoot(holderData)
	if err != nil {
		return TefINTERNAL
	}

	// Find the AMM
	ammKey := computeAMMKeylet(tx.Asset, tx.Asset2)
	ammData, err := e.view.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	amm, err := parseAMMData(ammData)
	if err != nil {
		return TefINTERNAL
	}

	// Get AMM account
	ammAccountID := computeAMMAccountID(ammKey.Key)
	ammAccountKey := keylet.Account(ammAccountID)
	ammAccountData, err := e.view.Read(ammAccountKey)
	if err != nil {
		return TefINTERNAL
	}
	ammAccount, err := parseAccountRoot(ammAccountData)
	if err != nil {
		return TefINTERNAL
	}

	// Get AMM balances
	assetBalance1 := NewNumberFromUint64(ammAccount.Balance)
	assetBalance2 := NumberZero
	lptAMMBalance := amm.LPTokenBalance

	if lptAMMBalance.Le(NumberZero) {
		return TecAMM_BALANCE
	}

	// Get holder's LP tokens (simplified)
	holdLPTokens := lptAMMBalance.Div(NewNumber(2))
	if holdLPTokens.Le(NumberZero) {
		return TecAMM_BALANCE
	}

	flags := tx.GetFlags()
	var lpTokensToWithdraw, withdrawAmount1, withdrawAmount2 Number

	if tx.Amount == nil {
		// Withdraw all holder's LP tokens
		lpTokensToWithdraw = holdLPTokens
		withdrawAmount1, withdrawAmount2 = equalWithdrawTokens(assetBalance1, assetBalance2, lptAMMBalance, holdLPTokens)
	} else {
		clawAmount := parseAmountToNumber(tx.Amount.Value)
		if assetBalance1.IsZero() {
			return TecAMM_BALANCE
		}
		frac := clawAmount.Div(assetBalance1)
		lpTokensNeeded := lptAMMBalance.Mul(frac)

		if lpTokensNeeded.Gt(holdLPTokens) {
			lpTokensToWithdraw = holdLPTokens
			withdrawAmount1, withdrawAmount2 = equalWithdrawTokens(assetBalance1, assetBalance2, lptAMMBalance, holdLPTokens)
		} else {
			lpTokensToWithdraw = lpTokensNeeded
			withdrawAmount1 = clawAmount
			withdrawAmount2 = assetBalance2.Mul(frac)
		}
	}

	// Clamp withdrawal amounts
	if withdrawAmount1.Gt(assetBalance1) {
		withdrawAmount1 = assetBalance1
	}
	if withdrawAmount2.Gt(assetBalance2) {
		withdrawAmount2 = assetBalance2
	}

	// Perform withdrawal from AMM
	isXRP1 := tx.Asset.Currency == "" || tx.Asset.Currency == "XRP"
	isXRP2 := tx.Asset2.Currency == "" || tx.Asset2.Currency == "XRP"

	if isXRP1 && !withdrawAmount1.IsZero() {
		ammAccount.Balance -= withdrawAmount1.Uint64()
	}
	if isXRP2 && !withdrawAmount2.IsZero() {
		ammAccount.Balance -= withdrawAmount2.Uint64()
	}

	// Claw back asset1 to issuer
	if isXRP1 && !withdrawAmount1.IsZero() {
		account.Balance += withdrawAmount1.Uint64()
	}

	// Handle asset2 based on tfClawTwoAssets flag
	if flags&tfClawTwoAssets != 0 {
		if isXRP2 && !withdrawAmount2.IsZero() {
			account.Balance += withdrawAmount2.Uint64()
		}
	} else {
		if isXRP2 && !withdrawAmount2.IsZero() {
			holderAccount.Balance += withdrawAmount2.Uint64()
		}
	}

	// Reduce LP token balance
	newLPBalance := lptAMMBalance.Sub(lpTokensToWithdraw)
	amm.LPTokenBalance = newLPBalance

	// Check if AMM should be deleted
	ammDeleted := false
	if newLPBalance.Le(NumberZero) {
		if err := e.view.Erase(ammKey); err != nil {
			return TefINTERNAL
		}
		if err := e.view.Erase(ammAccountKey); err != nil {
			return TefINTERNAL
		}
		ammDeleted = true
	}

	if !ammDeleted {
		ammBytes, err := serializeAMMData(amm)
		if err != nil {
			return TefINTERNAL
		}
		if err := e.view.Update(ammKey, ammBytes); err != nil {
			return TefINTERNAL
		}

		ammAccountBytes, err := serializeAccountRoot(ammAccount)
		if err != nil {
			return TefINTERNAL
		}
		if err := e.view.Update(ammAccountKey, ammAccountBytes); err != nil {
			return TefINTERNAL
		}
	}

	// Persist issuer account
	accountKey := keylet.Account(issuerID)
	accountBytes, err := serializeAccountRoot(account)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(accountKey, accountBytes); err != nil {
		return TefINTERNAL
	}

	// Persist holder account
	holderBytes, err := serializeAccountRoot(holderAccount)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(holderKey, holderBytes); err != nil {
		return TefINTERNAL
	}

	// Record metadata
	if ammDeleted {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "AMM",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(ammKey.Key[:])),
		})
	} else {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "AMM",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(ammKey.Key[:])),
			FinalFields: map[string]any{
				"LPTokenBalance": amm.LPTokenBalance.Uint64(),
			},
		})
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(holderKey.Key[:])),
	})

	return TesSUCCESS
}

// ============================================================================
// Helper function for backward compatibility
// ============================================================================

// calculateLPTokensFloat calculates LP tokens for IOU amounts (float version)
func calculateLPTokensFloat(amount1, amount2 float64) float64 {
	return math.Sqrt(amount1 * amount2)
}
