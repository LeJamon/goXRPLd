package amm

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

// validateAMMAmount validates an AMM amount
func validateAMMAmount(amt tx.Amount) error {
	if amt.IsZero() {
		return errors.New("amount must be positive")
	}
	if amt.IsNegative() {
		return errors.New("amount must be positive")
	}
	return nil
}

// validateAMMAmountWithPair validates an AMM amount including optional asset pair matching.
// If pair is provided, the amount's issue must match either asset.
// If validZero is true, zero amounts are allowed.
// Returns:
// - "temBAD_AMM_TOKENS" if amount's issue doesn't match the asset pair
// - "temBAD_AMOUNT" if amount is negative or zero (when validZero is false)
// - "" on success
func validateAMMAmountWithPair(amt tx.Amount, asset1, asset2 *tx.Asset, validZero bool) string {
	// Check if amount's issue matches either asset in the pair
	if asset1 != nil && asset2 != nil {
		if !matchesAsset(&amt, *asset1) && !matchesAsset(&amt, *asset2) {
			return "temBAD_AMM_TOKENS"
		}
	}

	// Check amount value
	if amt.IsNegative() {
		return "temBAD_AMOUNT"
	}
	if !validZero && amt.IsZero() {
		return "temBAD_AMOUNT"
	}

	return ""
}

// matchesAsset checks if an Amount matches an Asset
// Handles XRP being represented as either "" or "XRP" for currency
func matchesAsset(amt *tx.Amount, asset tx.Asset) bool {
	if amt == nil {
		return false
	}
	// Check if both are XRP (currency empty or "XRP", no issuer)
	amtIsXRP := amt.IsNative() || amt.Currency == "" || amt.Currency == "XRP"
	assetIsXRP := asset.Currency == "" || asset.Currency == "XRP"
	if amtIsXRP && assetIsXRP {
		return true
	}
	// For IOUs, compare currency and issuer
	return amt.Currency == asset.Currency && amt.Issuer == asset.Issuer
}

// zeroAmount returns a zero amount for the given asset
func zeroAmount(asset tx.Asset) tx.Amount {
	if asset.Currency == "" || asset.Currency == "XRP" {
		return sle.NewXRPAmountFromInt(0)
	}
	return sle.NewIssuedAmountFromValue(0, -100, asset.Currency, asset.Issuer)
}

// ComputeAMMAccountAddress computes the AMM account address from the asset pair.
// Exported for use in test helpers.
func ComputeAMMAccountAddress(asset1, asset2 tx.Asset) string {
	ammKey := computeAMMKeylet(asset1, asset2)
	ammAccountID := computeAMMAccountID(ammKey.Key)
	addr, _ := encodeAccountID(ammAccountID)
	return addr
}

// computeAMMKeylet computes the AMM keylet from the asset pair.
func computeAMMKeylet(asset1, asset2 tx.Asset) keylet.Keylet {
	issuer1 := getIssuerBytes(asset1.Issuer)
	currency1 := sle.GetCurrencyBytes(asset1.Currency)
	issuer2 := getIssuerBytes(asset2.Issuer)
	currency2 := sle.GetCurrencyBytes(asset2.Currency)

	return keylet.AMM(issuer1, currency1, issuer2, currency2)
}

// getIssuerBytes converts an issuer address string to a 20-byte account ID.
func getIssuerBytes(issuer string) [20]byte {
	if issuer == "" {
		return [20]byte{}
	}
	id, _ := sle.DecodeAccountID(issuer)
	return id
}

// computeAMMAccountID derives the AMM pseudo-account ID from the AMM keylet key.
// The AMM account is the first 20 bytes of the 32-byte AMM hash.
func computeAMMAccountID(ammKey [32]byte) [20]byte {
	var accountID [20]byte
	copy(accountID[:], ammKey[:20])
	return accountID
}

// calculateLPTokens calculates initial LP token balance as sqrt(amount1 * amount2).
// Uses tx.Amount arithmetic for precision with IOU values.
// LP tokens are always IOU (never XRP), so we ensure the result is IOU.
// Note: rippled uses XRP in drops (not XRP units) for this calculation.
// So sqrt(10000 XRP * 10000 USD) = sqrt(10000000000 drops * 10000) = sqrt(10^14) = 10^7 = 10,000,000 LP tokens
func calculateLPTokens(amount1, amount2 tx.Amount) tx.Amount {
	if amount1.IsZero() || amount2.IsZero() {
		return sle.NewIssuedAmountFromValue(0, -100, "", "")
	}

	// Convert amounts to IOU representation for consistent calculation
	// IMPORTANT: XRP uses drops directly (NOT converted to XRP units)
	// This matches rippled behavior where sqrt(drops * IOU) gives LP tokens
	var iou1, iou2 tx.Amount

	if amount1.IsNative() {
		// XRP: use drops directly as the value
		// e.g., 10,000 XRP = 10,000,000,000 drops, represented as mantissa=1e15, exp=-5
		drops := amount1.Drops()
		mantissa := drops
		exp := 0 // Start with exponent 0 for the raw drops value
		// Normalize mantissa to [1e15, 1e16)
		for mantissa >= 1e16 {
			mantissa /= 10
			exp++
		}
		for mantissa > 0 && mantissa < 1e15 {
			mantissa *= 10
			exp--
		}
		iou1 = sle.NewIssuedAmountFromValue(mantissa, exp, "", "")
	} else {
		iou1 = sle.NewIssuedAmountFromValue(amount1.Mantissa(), amount1.Exponent(), "", "")
	}

	if amount2.IsNative() {
		drops := amount2.Drops()
		mantissa := drops
		exp := 0
		for mantissa >= 1e16 {
			mantissa /= 10
			exp++
		}
		for mantissa > 0 && mantissa < 1e15 {
			mantissa *= 10
			exp--
		}
		iou2 = sle.NewIssuedAmountFromValue(mantissa, exp, "", "")
	} else {
		iou2 = sle.NewIssuedAmountFromValue(amount2.Mantissa(), amount2.Exponent(), "", "")
	}

	// product = iou1 * iou2
	product := iou1.Mul(iou2, false)
	// result = sqrt(product)
	return product.Sqrt()
}

// GenerateAMMLPTCurrency generates the LP token currency code from two asset currencies.
// The LP token currency is a hex-encoded 20-byte value derived from the asset pair.
// Exported for use in test helpers.
func GenerateAMMLPTCurrency(currency1, currency2 string) string {
	return generateAMMLPTCurrency(currency1, currency2)
}

// generateAMMLPTCurrency generates the LP token currency code from two asset currencies.
// The LP token currency is a hex-encoded 20-byte value derived from the asset pair.
func generateAMMLPTCurrency(currency1, currency2 string) string {
	c1 := sle.GetCurrencyBytes(currency1)
	c2 := sle.GetCurrencyBytes(currency2)

	// XOR the two currency bytes to create a unique LP token currency
	var lptCurrency [20]byte
	// Set high nibble to 0x03 to indicate LP token
	lptCurrency[0] = 0x03

	for i := 1; i < 20; i++ {
		lptCurrency[i] = c1[i] ^ c2[i]
	}

	return fmt.Sprintf("%X", lptCurrency)
}

// compareAccountIDs compares two account IDs lexicographically.
func compareAccountIDs(a, b [20]byte) int {
	return sle.CompareAccountIDs(a, b)
}

// encodeAccountID encodes a 20-byte account ID to an XRPL address string.
func encodeAccountID(accountID [20]byte) (string, error) {
	return sle.EncodeAccountID(accountID)
}

// getFee converts a trading fee in basis points (0-1000) to a fractional Amount.
// 1000 basis points = 1% = 0.01
// Returns fee as an IOU Amount for precise arithmetic.
func getFee(fee uint16) tx.Amount {
	// fee is in range 0-1000, representing 0-1% (0.00 to 0.01)
	// Convert to Amount: fee/100000
	if fee == 0 {
		return sle.NewIssuedAmountFromValue(0, -100, "", "")
	}
	// fee / 100000 = fee * 10^-5
	// For normalized form: mantissa in [10^15, 10^16), so fee * 10^10 with exp -15
	mantissa := int64(fee) * 1e10
	return sle.NewIssuedAmountFromValue(mantissa, -15, "", "")
}

// oneAmount returns the Amount value 1.0 as an IOU for arithmetic.
func oneAmount() tx.Amount {
	return sle.NewIssuedAmountFromValue(1e15, -15, "", "")
}

// subFromOne calculates (1 - x) where x is a fractional Amount
func subFromOne(x tx.Amount) tx.Amount {
	one := oneAmount()
	result, _ := one.Sub(x)
	return result
}

// addToOne calculates (1 + x) where x is a fractional Amount
func addToOne(x tx.Amount) tx.Amount {
	one := oneAmount()
	result, _ := one.Add(x)
	return result
}

// lpTokensOut calculates LP tokens issued for a single-asset deposit.
// Equation 4: t = T * ((1 + a/(A*(1-tfee)))^0.5 - 1)
func lpTokensOut(assetBalance, amountIn, lptBalance tx.Amount, tfee uint16) tx.Amount {
	if assetBalance.IsZero() || lptBalance.IsZero() {
		return sle.NewIssuedAmountFromValue(0, -100, "", "")
	}

	// Convert XRP amounts to IOU for precise fractional calculations
	// This is necessary because XRP division uses integer arithmetic
	assetBalanceIOU := toIOUForCalc(assetBalance)
	amountInIOU := toIOUForCalc(amountIn)
	lptBalanceIOU := toIOUForCalc(lptBalance)

	// fee = tfee / 100000
	fee := getFee(tfee)
	// oneMinusFee = 1 - fee
	oneMinusFee := subFromOne(fee)
	// effectiveDenom = A * (1 - fee)
	effectiveDenom := assetBalanceIOU.Mul(oneMinusFee, false)
	// effectiveAmount = a / effectiveDenom = a / (A * (1 - fee))
	effectiveAmount := amountInIOU.Div(effectiveDenom, false)
	// onePlusEff = 1 + effectiveAmount
	onePlusEff := addToOne(effectiveAmount)
	// sqrtVal = sqrt(1 + effectiveAmount)
	sqrtVal := onePlusEff.Sqrt()
	// factor = sqrtVal - 1
	factor, _ := sqrtVal.Sub(oneAmount())
	// result = T * factor
	return lptBalanceIOU.Mul(factor, false)
}

// ammAssetIn calculates the asset amount needed for a specified LP token output (single-asset deposit).
// Equation 3 inverse: a = A * (1-tfee) * (((T+t)/T)^2 - 1)
func ammAssetIn(assetBalance, lptBalance, lpTokensOutAmt tx.Amount, tfee uint16) tx.Amount {
	if lptBalance.IsZero() {
		return sle.NewIssuedAmountFromValue(0, -100, "", "")
	}

	// Convert XRP amounts to IOU for precise fractional calculations
	assetBalanceIOU := toIOUForCalc(assetBalance)
	lptBalanceIOU := toIOUForCalc(lptBalance)
	lpTokensOutIOU := toIOUForCalc(lpTokensOutAmt)

	// fee = tfee / 100000
	fee := getFee(tfee)
	// oneMinusFee = 1 - fee
	oneMinusFee := subFromOne(fee)
	// tPlusT = T + t (lptBalance + lpTokensOutAmt)
	tPlusT, _ := lptBalanceIOU.Add(lpTokensOutIOU)
	// ratio = (T + t) / T
	ratio := tPlusT.Div(lptBalanceIOU, false)
	// ratioSquared = ratio^2
	ratioSquared := ratio.Mul(ratio, false)
	// factor = ratioSquared - 1
	factor, _ := ratioSquared.Sub(oneAmount())
	// result = A * (1 - fee) * factor
	aTimesFee := assetBalanceIOU.Mul(oneMinusFee, false)
	return aTimesFee.Mul(factor, false)
}

// ammAssetOut calculates the asset amount received for burning LP tokens (single-asset withdrawal).
// Equation 8: a = A * (1 - (1 - t/T)^2) * (1 - tfee)
func ammAssetOut(assetBalance, lptBalance, lpTokensIn tx.Amount, tfee uint16) tx.Amount {
	if lptBalance.IsZero() {
		return sle.NewIssuedAmountFromValue(0, -100, "", "")
	}
	// Check if lpTokensIn > lptBalance
	if lpTokensIn.Compare(lptBalance) > 0 {
		return sle.NewIssuedAmountFromValue(0, -100, "", "")
	}

	// Convert XRP amounts to IOU for precise fractional calculations
	assetBalanceIOU := toIOUForCalc(assetBalance)
	lptBalanceIOU := toIOUForCalc(lptBalance)
	lpTokensInIOU := toIOUForCalc(lpTokensIn)

	// fee = tfee / 100000
	fee := getFee(tfee)
	// oneMinusFee = 1 - fee
	oneMinusFee := subFromOne(fee)
	// tDivT = t / T (lpTokensIn / lptBalance)
	tDivT := lpTokensInIOU.Div(lptBalanceIOU, false)
	// ratio = 1 - t/T
	ratio := subFromOne(tDivT)
	// ratioSquared = ratio^2
	ratioSquared := ratio.Mul(ratio, false)
	// factor = 1 - ratioSquared
	factor := subFromOne(ratioSquared)
	// factorWithFee = factor * (1 - fee)
	factorWithFee := factor.Mul(oneMinusFee, false)
	// result = A * factorWithFee
	return assetBalanceIOU.Mul(factorWithFee, false)
}

// calcLPTokensIn calculates LP tokens needed for a single-asset withdrawal amount.
// Equation 7: t = T * (1 - (1 - a/(A*(1-tfee)))^0.5)
func calcLPTokensIn(assetBalance, amountOut, lptBalance tx.Amount, tfee uint16) tx.Amount {
	if assetBalance.IsZero() || lptBalance.IsZero() {
		return sle.NewIssuedAmountFromValue(0, -100, "", "")
	}

	// Convert XRP amounts to IOU for precise fractional calculations
	assetBalanceIOU := toIOUForCalc(assetBalance)
	amountOutIOU := toIOUForCalc(amountOut)
	lptBalanceIOU := toIOUForCalc(lptBalance)

	// fee = tfee / 100000
	fee := getFee(tfee)
	// oneMinusFee = 1 - fee
	oneMinusFee := subFromOne(fee)
	// effectiveDenom = A * (1 - fee)
	effectiveDenom := assetBalanceIOU.Mul(oneMinusFee, false)
	// effectiveAmount = amountOut / effectiveDenom
	effectiveAmount := amountOutIOU.Div(effectiveDenom, false)
	// Check if effectiveAmount >= 1 (would drain the pool)
	if effectiveAmount.Compare(oneAmount()) >= 0 {
		return lptBalanceIOU // Would drain the pool
	}
	// oneMinusEff = 1 - effectiveAmount
	oneMinusEff := subFromOne(effectiveAmount)
	// sqrtVal = sqrt(1 - effectiveAmount)
	sqrtVal := oneMinusEff.Sqrt()
	// factor = 1 - sqrtVal
	factor := subFromOne(sqrtVal)
	// result = T * factor
	return lptBalanceIOU.Mul(factor, false)
}

// proportionalAmount calculates balance * (numerator / denominator) using Amount arithmetic.
// This replaces float64 fraction calculations like: balance * (tokens / totalTokens)
func proportionalAmount(balance, numerator, denominator tx.Amount) tx.Amount {
	if denominator.IsZero() {
		return zeroAmount(tx.Asset{Currency: balance.Currency, Issuer: balance.Issuer})
	}
	// fraction = numerator / denominator
	fraction := numerator.Div(denominator, false)
	// result = balance * fraction
	return balance.Mul(fraction, false)
}

// toIOUForCalc converts an Amount to IOU representation for precise calculations.
// This is necessary because XRP/XRP division uses integer arithmetic and loses precision.
// For example, 1000 XRP / 10000 XRP = 0 with integer division, but should be 0.1 as a fraction.
func toIOUForCalc(amt tx.Amount) tx.Amount {
	if !amt.IsNative() {
		return amt
	}
	// Convert XRP drops to IOU representation
	drops := amt.Drops()
	if drops == 0 {
		return sle.NewIssuedAmountFromValue(0, -100, "", "")
	}
	// Normalize mantissa to [10^15, 10^16) range
	mantissa := drops
	exp := 0
	for mantissa >= 1e16 {
		mantissa /= 10
		exp++
	}
	for mantissa > 0 && mantissa < 1e15 {
		mantissa *= 10
		exp--
	}
	return sle.NewIssuedAmountFromValue(mantissa, exp, "", "")
}

// iouToDrops converts an IOU representation back to XRP drops.
// This is the reverse of toIOUForCalc for XRP amounts.
func iouToDrops(amt tx.Amount) int64 {
	if amt.IsNative() {
		return amt.Drops()
	}
	// Convert IOU mantissa/exponent to drops
	mantissa := amt.Mantissa()
	exp := amt.Exponent()
	// Result = mantissa * 10^exp
	for exp > 0 {
		mantissa *= 10
		exp--
	}
	for exp < 0 {
		mantissa /= 10
		exp++
	}
	return mantissa
}

// minAmount returns the smaller of two amounts.
// Assumes both amounts are of the same type (both XRP or same IOU).
func minAmount(a, b tx.Amount) tx.Amount {
	if a.Compare(b) < 0 {
		return a
	}
	return b
}

// maxAmount returns the larger of two amounts.
// Assumes both amounts are of the same type (both XRP or same IOU).
func maxAmount(a, b tx.Amount) tx.Amount {
	if a.Compare(b) > 0 {
		return a
	}
	return b
}

// isGreater returns true if a > b
func isGreater(a, b tx.Amount) bool {
	return a.Compare(b) > 0
}

// isLessOrEqual returns true if a <= b
func isLessOrEqual(a, b tx.Amount) bool {
	return a.Compare(b) <= 0
}

// amountFromXRPDrops creates an XRP Amount from drops (for balance tracking).
func amountFromXRPDrops(drops uint64) tx.Amount {
	return sle.NewXRPAmountFromInt(int64(drops))
}

// serializeAmount serializes a tx.Amount to binary.
// Format: 1 byte type (0=XRP, 1=IOU), then:
//   - XRP: 8 bytes int64 drops
//   - IOU: 8 bytes int64 mantissa + 4 bytes int32 exponent
func serializeAmount(amt tx.Amount) []byte {
	if amt.IsNative() {
		buf := make([]byte, 9) // 1 type + 8 drops
		buf[0] = 0             // XRP type
		binary.BigEndian.PutUint64(buf[1:9], uint64(amt.Drops()))
		return buf
	}
	buf := make([]byte, 13) // 1 type + 8 mantissa + 4 exponent
	buf[0] = 1              // IOU type
	binary.BigEndian.PutUint64(buf[1:9], uint64(amt.Mantissa()))
	binary.BigEndian.PutUint32(buf[9:13], uint32(amt.Exponent()+128)) // offset exponent to avoid negative
	return buf
}

// deserializeAmount deserializes a tx.Amount from binary.
// Returns the Amount and bytes consumed.
func deserializeAmount(data []byte) (tx.Amount, int) {
	if len(data) < 1 {
		return sle.NewXRPAmountFromInt(0), 0
	}
	amtType := data[0]
	if amtType == 0 {
		// XRP
		if len(data) < 9 {
			return sle.NewXRPAmountFromInt(0), 0
		}
		drops := int64(binary.BigEndian.Uint64(data[1:9]))
		return sle.NewXRPAmountFromInt(drops), 9
	}
	// IOU
	if len(data) < 13 {
		return sle.NewIssuedAmountFromValue(0, -100, "", ""), 0
	}
	mantissa := int64(binary.BigEndian.Uint64(data[1:9]))
	exponent := int(binary.BigEndian.Uint32(data[9:13])) - 128 // reverse offset
	return sle.NewIssuedAmountFromValue(mantissa, exponent, "", ""), 13
}

// serializeIssue serializes an Issue (currency + issuer) to binary.
// Format: 20 bytes currency + 20 bytes issuer = 40 bytes total
func serializeIssue(asset tx.Asset) []byte {
	buf := make([]byte, 40)
	// Currency (20 bytes)
	currency := sle.GetCurrencyBytes(asset.Currency)
	copy(buf[0:20], currency[:])
	// Issuer (20 bytes)
	issuer := getIssuerBytes(asset.Issuer)
	copy(buf[20:40], issuer[:])
	return buf
}

// deserializeIssue deserializes an Issue from binary.
// Returns the Asset and bytes consumed (always 40).
func deserializeIssue(data []byte) (tx.Asset, int) {
	if len(data) < 40 {
		return tx.Asset{}, 0
	}
	// Currency (20 bytes)
	var currencyBytes [20]byte
	copy(currencyBytes[:], data[0:20])
	currency := sle.GetCurrencyString(currencyBytes)

	// Issuer (20 bytes)
	var issuerBytes [20]byte
	copy(issuerBytes[:], data[20:40])
	issuer := ""
	if issuerBytes != [20]byte{} {
		issuer, _ = sle.EncodeAccountID(issuerBytes)
	}

	return tx.Asset{Currency: currency, Issuer: issuer}, 40
}

// parseAMMData deserializes an AMM ledger entry from binary data.
// This matches the rippled AMM ledger entry format exactly.
// Reference: rippled include/xrpl/protocol/detail/ledger_entries.macro ltAMM
func parseAMMData(data []byte) (*AMMData, error) {
	// Minimum size: Account(20) + Asset(40) + Asset2(40) + TradingFee(2) + OwnerNode(8) = 110 bytes
	if len(data) < 110 {
		return nil, fmt.Errorf("AMM data too short: %d bytes", len(data))
	}

	amm := &AMMData{}
	offset := 0

	// Account (20 bytes)
	copy(amm.Account[:], data[offset:offset+20])
	offset += 20

	// Asset (40 bytes: currency + issuer)
	amm.Asset, _ = deserializeIssue(data[offset:])
	offset += 40

	// Asset2 (40 bytes: currency + issuer)
	amm.Asset2, _ = deserializeIssue(data[offset:])
	offset += 40

	// TradingFee (2 bytes)
	amm.TradingFee = binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	// OwnerNode (8 bytes)
	amm.OwnerNode = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	// LPTokenBalance (variable: 9 or 13 bytes)
	if offset >= len(data) {
		return amm, nil
	}
	amt, consumed := deserializeAmount(data[offset:])
	amm.LPTokenBalance = amt
	offset += consumed

	// VoteSlots count (1 byte)
	if offset+1 > len(data) {
		amm.VoteSlots = make([]VoteSlotData, 0)
		return amm, nil
	}
	voteCount := int(data[offset])
	offset++

	amm.VoteSlots = make([]VoteSlotData, 0, voteCount)
	for i := 0; i < voteCount && offset+26 <= len(data); i++ {
		var slot VoteSlotData
		copy(slot.Account[:], data[offset:offset+20])
		offset += 20
		slot.TradingFee = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
		slot.VoteWeight = binary.BigEndian.Uint32(data[offset : offset+4])
		offset += 4
		amm.VoteSlots = append(amm.VoteSlots, slot)
	}

	// AuctionSlot (optional)
	if offset+1 > len(data) {
		return amm, nil
	}
	hasAuctionSlot := data[offset]
	offset++

	if hasAuctionSlot != 0 && offset+24 <= len(data) {
		slot := &AuctionSlotData{
			AuthAccounts: make([][20]byte, 0),
		}
		copy(slot.Account[:], data[offset:offset+20])
		offset += 20
		slot.Expiration = binary.BigEndian.Uint32(data[offset : offset+4])
		offset += 4

		// DiscountedFee (2 bytes)
		if offset+2 <= len(data) {
			slot.DiscountedFee = binary.BigEndian.Uint16(data[offset : offset+2])
			offset += 2
		}

		// Price (variable: 9 or 13 bytes)
		if offset < len(data) {
			price, consumed := deserializeAmount(data[offset:])
			slot.Price = price
			offset += consumed
		}

		// Auth accounts count
		if offset+1 <= len(data) {
			authCount := int(data[offset])
			offset++
			for i := 0; i < authCount && offset+20 <= len(data); i++ {
				var authID [20]byte
				copy(authID[:], data[offset:offset+20])
				offset += 20
				slot.AuthAccounts = append(slot.AuthAccounts, authID)
			}
		}
		amm.AuctionSlot = slot
	}

	return amm, nil
}

// serializeAMMData serializes an AMMData entry to binary.
// This matches the rippled AMM ledger entry format exactly.
// Reference: rippled include/xrpl/protocol/detail/ledger_entries.macro ltAMM
// IMPORTANT: Asset balances are NOT stored - they are read from AccountRoot/trustlines.
func serializeAMMData(amm *AMMData) ([]byte, error) {
	// Pre-serialize amounts to get their sizes
	lptBalanceBytes := serializeAmount(amm.LPTokenBalance)

	// Calculate size
	// Account(20) + Asset(40) + Asset2(40) + TradingFee(2) + OwnerNode(8) + LPTokenBalance(variable)
	size := 20 + 40 + 40 + 2 + 8 + len(lptBalanceBytes)
	size += 1                       // voteCount
	size += len(amm.VoteSlots) * 26 // Each vote slot: 20 + 2 + 4
	size += 1                       // hasAuctionSlot flag
	if amm.AuctionSlot != nil {
		priceBytes := serializeAmount(amm.AuctionSlot.Price)
		// Account(20) + Expiration(4) + DiscountedFee(2) + Price(variable) + authCount(1)
		size += 20 + 4 + 2 + len(priceBytes) + 1
		size += len(amm.AuctionSlot.AuthAccounts) * 20
	}

	data := make([]byte, size)
	offset := 0

	// Account (20 bytes)
	copy(data[offset:offset+20], amm.Account[:])
	offset += 20

	// Asset (40 bytes: currency + issuer)
	assetBytes := serializeIssue(amm.Asset)
	copy(data[offset:offset+40], assetBytes)
	offset += 40

	// Asset2 (40 bytes: currency + issuer)
	asset2Bytes := serializeIssue(amm.Asset2)
	copy(data[offset:offset+40], asset2Bytes)
	offset += 40

	// TradingFee (2 bytes)
	binary.BigEndian.PutUint16(data[offset:offset+2], amm.TradingFee)
	offset += 2

	// OwnerNode (8 bytes)
	binary.BigEndian.PutUint64(data[offset:offset+8], amm.OwnerNode)
	offset += 8

	// LPTokenBalance (using serializeAmount for proper Amount serialization)
	copy(data[offset:], lptBalanceBytes)
	offset += len(lptBalanceBytes)

	// VoteSlots
	data[offset] = byte(len(amm.VoteSlots))
	offset++

	for _, slot := range amm.VoteSlots {
		copy(data[offset:offset+20], slot.Account[:])
		offset += 20
		binary.BigEndian.PutUint16(data[offset:offset+2], slot.TradingFee)
		offset += 2
		binary.BigEndian.PutUint32(data[offset:offset+4], slot.VoteWeight)
		offset += 4
	}

	// AuctionSlot
	if amm.AuctionSlot != nil {
		data[offset] = 1
		offset++
		copy(data[offset:offset+20], amm.AuctionSlot.Account[:])
		offset += 20
		binary.BigEndian.PutUint32(data[offset:offset+4], amm.AuctionSlot.Expiration)
		offset += 4
		// DiscountedFee (2 bytes)
		binary.BigEndian.PutUint16(data[offset:offset+2], amm.AuctionSlot.DiscountedFee)
		offset += 2
		// Price (using serializeAmount for proper Amount serialization)
		priceBytes := serializeAmount(amm.AuctionSlot.Price)
		copy(data[offset:], priceBytes)
		offset += len(priceBytes)
		data[offset] = byte(len(amm.AuctionSlot.AuthAccounts))
		offset++
		for _, authID := range amm.AuctionSlot.AuthAccounts {
			copy(data[offset:offset+20], authID[:])
			offset += 20
		}
	} else {
		data[offset] = 0
		offset++
	}

	return data[:offset], nil
}

// createOrUpdateAMMTrustline creates or updates a trust line for an AMM asset.
// This creates the trustline between the AMM account and the asset issuer,
// following rippled's trustCreate logic.
// Reference: rippled View.cpp trustCreate lines 1329-1445
func createOrUpdateAMMTrustline(ammAccountID [20]byte, asset tx.Asset, amount tx.Amount, view tx.LedgerView) error {
	// XRP doesn't need a trustline
	if asset.Currency == "" || asset.Currency == "XRP" {
		return nil
	}

	issuerID, err := sle.DecodeAccountID(asset.Issuer)
	if err != nil {
		return err
	}

	// Get trustline keylet
	trustLineKey := keylet.Line(ammAccountID, issuerID, asset.Currency)

	// Check if trustline already exists
	exists, err := view.Exists(trustLineKey)
	if err != nil {
		return err
	}

	if exists {
		// Trustline exists - update the balance
		// Reference: rippled rippleCreditIOU lines 1668-1748
		data, err := view.Read(trustLineKey)
		if err != nil {
			return err
		}

		rs, err := sle.ParseRippleState(data)
		if err != nil {
			return err
		}

		// Determine if AMM is low or high account
		ammIsLow := keylet.IsLowAccount(ammAccountID, issuerID)

		// Update balance - positive balance means low owes high
		// AMM is receiving tokens from issuer (or being credited), so:
		// If AMM is low: balance should increase (AMM holds more)
		// If AMM is high: balance should decrease (AMM holds more, from their perspective)
		currentBalance := rs.Balance
		var newBalance tx.Amount

		if ammIsLow {
			// AMM is low - positive balance means AMM holds tokens
			newBalance, err = currentBalance.Add(amount)
			if err != nil {
				return err
			}
		} else {
			// AMM is high - negative balance means AMM holds tokens
			newBalance, err = currentBalance.Sub(amount)
			if err != nil {
				return err
			}
		}

		// Update balance preserving currency/issuer
		rs.Balance = sle.NewIssuedAmountFromValue(
			newBalance.Mantissa(),
			newBalance.Exponent(),
			rs.Balance.Currency,
			rs.Balance.Issuer,
		)

		// Ensure lsfAMMNode flag is set (for AMM-owned trustlines)
		rs.Flags |= sle.LsfAMMNode

		// Serialize and update
		rsBytes, err := sle.SerializeRippleState(rs)
		if err != nil {
			return err
		}

		return view.Update(trustLineKey, rsBytes)
	}

	// Trustline doesn't exist - create it
	// Reference: rippled trustCreate lines 1347-1445

	// Determine low/high account ordering
	var lowAccountID, highAccountID [20]byte
	ammIsLow := keylet.IsLowAccount(ammAccountID, issuerID)
	if ammIsLow {
		lowAccountID = ammAccountID
		highAccountID = issuerID
	} else {
		lowAccountID = issuerID
		highAccountID = ammAccountID
	}

	lowAccountStr, _ := sle.EncodeAccountID(lowAccountID)
	highAccountStr, _ := sle.EncodeAccountID(highAccountID)

	// Create the RippleState entry
	// For AMM trustlines:
	// - Balance represents how much the low account "owes" the high account
	// - If AMM is low, positive balance = AMM holds tokens
	// - If AMM is high, negative balance = AMM holds tokens
	// - Balance issuer is always ACCOUNT_ONE (no account)
	var balance tx.Amount
	if ammIsLow {
		// AMM is low - positive balance
		balance = sle.NewIssuedAmountFromValue(
			amount.Mantissa(),
			amount.Exponent(),
			asset.Currency,
			sle.AccountOneAddress,
		)
	} else {
		// AMM is high - negative balance
		negated := amount.Negate()
		balance = sle.NewIssuedAmountFromValue(
			negated.Mantissa(),
			negated.Exponent(),
			asset.Currency,
			sle.AccountOneAddress,
		)
	}

	// Create RippleState
	// Reference: rippled trustCreate - limits are set based on who set the limit
	// For AMM trustlines, the limits are 0 on both sides (AMM doesn't set limits)
	rs := &sle.RippleState{
		Balance:  balance,
		LowLimit: sle.NewIssuedAmountFromValue(0, -100, asset.Currency, lowAccountStr),
		HighLimit: sle.NewIssuedAmountFromValue(0, -100, asset.Currency, highAccountStr),
		Flags:    0,
		LowNode:  0,
		HighNode: 0,
	}

	// Set reserve flag for the side that is NOT the issuer
	// Reference: rippled trustCreate line 1409
	// For AMM, the AMM account should have reserve set
	if ammIsLow {
		rs.Flags |= sle.LsfLowReserve
	} else {
		rs.Flags |= sle.LsfHighReserve
	}

	// Set lsfAMMNode flag - this identifies it as an AMM-owned trustline
	// Reference: rippled AMMCreate.cpp line 297-306
	rs.Flags |= sle.LsfAMMNode

	// Insert into low account's owner directory
	lowDirKey := keylet.OwnerDir(lowAccountID)
	lowDirResult, err := sle.DirInsert(view, lowDirKey, trustLineKey.Key, func(dir *sle.DirectoryNode) {
		dir.Owner = lowAccountID
	})
	if err != nil {
		return err
	}

	// Insert into high account's owner directory
	highDirKey := keylet.OwnerDir(highAccountID)
	highDirResult, err := sle.DirInsert(view, highDirKey, trustLineKey.Key, func(dir *sle.DirectoryNode) {
		dir.Owner = highAccountID
	})
	if err != nil {
		return err
	}

	// Set deletion hints (page numbers where the trustline is stored)
	rs.LowNode = lowDirResult.Page
	rs.HighNode = highDirResult.Page

	// Serialize and insert the trustline
	rsBytes, err := sle.SerializeRippleState(rs)
	if err != nil {
		return err
	}

	return view.Insert(trustLineKey, rsBytes)
}

// updateTrustlineBalanceInView updates the balance of a trust line for IOU transfers.
// This reads the trust line, modifies the balance, and writes it back.
// delta is the amount to add (positive) or subtract (negative) from the account's perspective.
func updateTrustlineBalanceInView(accountID [20]byte, issuerID [20]byte, currency string, delta tx.Amount, view tx.LedgerView) error {
	// Get trust line keylet
	lineKey := keylet.Line(accountID, issuerID, currency)

	// Check if trust line exists
	exists, err := view.Exists(lineKey)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("trust line does not exist")
	}

	// Read trust line data
	data, err := view.Read(lineKey)
	if err != nil {
		return err
	}

	// Parse trust line
	rs, err := sle.ParseRippleState(data)
	if err != nil {
		return err
	}

	// Determine if account is low or high in the trust line
	// Balance convention: positive means low owes high
	isLow := keylet.IsLowAccount(accountID, issuerID)

	// Get current balance from holder's perspective
	currentBalance := rs.Balance
	if !isLow {
		currentBalance = currentBalance.Negate()
	}

	// Apply delta (positive = receiving, negative = sending)
	newBalance, err := currentBalance.Add(delta)
	if err != nil {
		return err
	}

	// Convert back to RippleState balance convention
	if !isLow {
		newBalance = newBalance.Negate()
	}

	// Update the balance - preserve currency and issuer from original
	rs.Balance = sle.NewIssuedAmountFromValue(
		newBalance.Mantissa(),
		newBalance.Exponent(),
		rs.Balance.Currency,
		rs.Balance.Issuer,
	)

	// Serialize and write back
	serialized, err := sle.SerializeRippleState(rs)
	if err != nil {
		return err
	}

	return view.Update(lineKey, serialized)
}

// createLPTokenTrustline creates or updates a trust line for LP tokens.
// This creates the trustline between the depositor and the AMM account (LP token issuer).
// Reference: rippled View.cpp trustCreate
func createLPTokenTrustline(accountID [20]byte, lptAsset tx.Asset, amount tx.Amount, view tx.LedgerView) error {
	// LP token issuer is the AMM account
	ammAccountID, err := sle.DecodeAccountID(lptAsset.Issuer)
	if err != nil {
		return err
	}

	// Get trustline keylet
	trustLineKey := keylet.Line(accountID, ammAccountID, lptAsset.Currency)

	// Check if trustline already exists
	exists, err := view.Exists(trustLineKey)
	if err != nil {
		return err
	}

	if exists {
		// Trustline exists - update the balance
		data, err := view.Read(trustLineKey)
		if err != nil {
			return err
		}

		rs, err := sle.ParseRippleState(data)
		if err != nil {
			return err
		}

		// Determine if holder is low or high account
		holderIsLow := keylet.IsLowAccount(accountID, ammAccountID)

		// Update balance - holder is receiving LP tokens
		currentBalance := rs.Balance
		var newBalance tx.Amount

		if holderIsLow {
			// Holder is low - positive balance means holder holds tokens
			newBalance, err = currentBalance.Add(amount)
			if err != nil {
				return err
			}
		} else {
			// Holder is high - negative balance means holder holds tokens
			newBalance, err = currentBalance.Sub(amount)
			if err != nil {
				return err
			}
		}

		// Update balance preserving currency/issuer
		rs.Balance = sle.NewIssuedAmountFromValue(
			newBalance.Mantissa(),
			newBalance.Exponent(),
			rs.Balance.Currency,
			rs.Balance.Issuer,
		)

		// Serialize and update
		rsBytes, err := sle.SerializeRippleState(rs)
		if err != nil {
			return err
		}

		return view.Update(trustLineKey, rsBytes)
	}

	// Trustline doesn't exist - create it

	// Determine low/high account ordering
	var lowAccountID, highAccountID [20]byte
	holderIsLow := keylet.IsLowAccount(accountID, ammAccountID)
	if holderIsLow {
		lowAccountID = accountID
		highAccountID = ammAccountID
	} else {
		lowAccountID = ammAccountID
		highAccountID = accountID
	}

	lowAccountStr, _ := sle.EncodeAccountID(lowAccountID)
	highAccountStr, _ := sle.EncodeAccountID(highAccountID)

	// Create balance - holder receives LP tokens
	var balance tx.Amount
	if holderIsLow {
		// Holder is low - positive balance
		balance = sle.NewIssuedAmountFromValue(
			amount.Mantissa(),
			amount.Exponent(),
			lptAsset.Currency,
			sle.AccountOneAddress,
		)
	} else {
		// Holder is high - negative balance
		negated := amount.Negate()
		balance = sle.NewIssuedAmountFromValue(
			negated.Mantissa(),
			negated.Exponent(),
			lptAsset.Currency,
			sle.AccountOneAddress,
		)
	}

	// Create RippleState
	// For LP token trustlines, the holder side gets reserve, AMM side doesn't
	rs := &sle.RippleState{
		Balance:   balance,
		LowLimit:  sle.NewIssuedAmountFromValue(0, -100, lptAsset.Currency, lowAccountStr),
		HighLimit: sle.NewIssuedAmountFromValue(0, -100, lptAsset.Currency, highAccountStr),
		Flags:     0,
		LowNode:   0,
		HighNode:  0,
	}

	// Set reserve flag for the LP token holder (not the AMM)
	if holderIsLow {
		rs.Flags |= sle.LsfLowReserve
	} else {
		rs.Flags |= sle.LsfHighReserve
	}

	// Insert into low account's owner directory
	lowDirKey := keylet.OwnerDir(lowAccountID)
	lowDirResult, err := sle.DirInsert(view, lowDirKey, trustLineKey.Key, func(dir *sle.DirectoryNode) {
		dir.Owner = lowAccountID
	})
	if err != nil {
		return err
	}

	// Insert into high account's owner directory
	highDirKey := keylet.OwnerDir(highAccountID)
	highDirResult, err := sle.DirInsert(view, highDirKey, trustLineKey.Key, func(dir *sle.DirectoryNode) {
		dir.Owner = highAccountID
	})
	if err != nil {
		return err
	}

	// Set deletion hints
	rs.LowNode = lowDirResult.Page
	rs.HighNode = highDirResult.Page

	// Serialize and insert
	rsBytes, err := sle.SerializeRippleState(rs)
	if err != nil {
		return err
	}

	return view.Insert(trustLineKey, rsBytes)
}

// initializeFeeAuctionVote initializes the vote slots and auction slot for an AMM.
// This is called when creating an AMM or when depositing into an empty AMM.
// Reference: rippled AMMUtils.cpp initializeFeeAuctionVote lines 340-384
func initializeFeeAuctionVote(amm *AMMData, accountID [20]byte, lptCurrency string, ammAccountAddr string, tfee uint16, parentCloseTime uint32) {
	// Clear existing vote slots and add creator's vote
	amm.VoteSlots = []VoteSlotData{
		{
			Account:    accountID,
			TradingFee: tfee,
			VoteWeight: uint32(VOTE_WEIGHT_SCALE_FACTOR),
		},
	}

	// Set trading fee
	amm.TradingFee = tfee

	// Calculate discounted fee
	discountedFee := uint16(0)
	if tfee > 0 {
		discountedFee = tfee / uint16(AUCTION_SLOT_DISCOUNTED_FEE_FRACTION)
	}

	// Calculate expiration: parentCloseTime + TOTAL_TIME_SLOT_SECS (24 hours)
	expiration := parentCloseTime + uint32(TOTAL_TIME_SLOT_SECS)

	// Initialize auction slot
	amm.AuctionSlot = &AuctionSlotData{
		Account:       accountID,
		Expiration:    expiration,
		Price:         zeroAmount(tx.Asset{Currency: lptCurrency, Issuer: ammAccountAddr}),
		DiscountedFee: discountedFee,
		AuthAccounts:  make([][20]byte, 0),
	}
}

// ammAccountHolds returns the amount held by the AMM account for a specific issue.
// For XRP: reads from the AMM account's AccountRoot.Balance
// For IOU: reads from the trustline between AMM account and issuer
// Reference: rippled AMMUtils.cpp ammAccountHolds
func ammAccountHolds(view tx.LedgerView, ammAccountID [20]byte, asset tx.Asset) tx.Amount {
	if asset.Currency == "" || asset.Currency == "XRP" {
		// XRP: read from AccountRoot
		accountKey := keylet.Account(ammAccountID)
		data, err := view.Read(accountKey)
		if err != nil || data == nil {
			return sle.NewXRPAmountFromInt(0)
		}
		account, err := sle.ParseAccountRoot(data)
		if err != nil {
			return sle.NewXRPAmountFromInt(0)
		}
		return sle.NewXRPAmountFromInt(int64(account.Balance))
	}

	// IOU: read from trustline
	issuerID, err := sle.DecodeAccountID(asset.Issuer)
	if err != nil {
		return sle.NewIssuedAmountFromValue(0, -100, asset.Currency, asset.Issuer)
	}

	trustLineKey := keylet.Line(ammAccountID, issuerID, asset.Currency)
	data, err := view.Read(trustLineKey)
	if err != nil || data == nil {
		return sle.NewIssuedAmountFromValue(0, -100, asset.Currency, asset.Issuer)
	}

	rs, err := sle.ParseRippleState(data)
	if err != nil {
		return sle.NewIssuedAmountFromValue(0, -100, asset.Currency, asset.Issuer)
	}

	// Determine balance based on canonical ordering
	// Balance is stored from low account's perspective (positive = low owes high)
	// For AMM: if AMM is low, positive balance means AMM holds tokens
	ammIsLow := sle.CompareAccountIDsForLine(ammAccountID, issuerID) < 0
	balance := rs.Balance
	if !ammIsLow {
		balance = balance.Negate()
	}

	// Return absolute balance with proper currency/issuer
	if balance.Signum() <= 0 {
		return sle.NewIssuedAmountFromValue(0, -100, asset.Currency, asset.Issuer)
	}

	return sle.NewIssuedAmountFromValue(balance.Mantissa(), balance.Exponent(), asset.Currency, asset.Issuer)
}

// ammPoolHolds returns the balances of both assets in the AMM pool.
// Reference: rippled AMMUtils.cpp ammPoolHolds
func ammPoolHolds(view tx.LedgerView, ammAccountID [20]byte, asset1, asset2 tx.Asset, fhZeroIfFrozen bool) (tx.Amount, tx.Amount) {
	// Get balance of first asset
	balance1 := ammAccountHolds(view, ammAccountID, asset1)

	// Get balance of second asset
	balance2 := ammAccountHolds(view, ammAccountID, asset2)

	// Check for frozen assets if requested
	if fhZeroIfFrozen {
		if tx.IsGlobalFrozen(view, asset1.Issuer) || tx.IsIndividualFrozen(view, ammAccountID, asset1) {
			balance1 = zeroAmount(asset1)
		}
		if tx.IsGlobalFrozen(view, asset2.Issuer) || tx.IsIndividualFrozen(view, ammAccountID, asset2) {
			balance2 = zeroAmount(asset2)
		}
	}

	return balance1, balance2
}

// AMMHolds returns the pool balances and LP token balance for an AMM.
// This is the main function to get current AMM state.
// Reference: rippled AMMUtils.cpp ammHolds
func AMMHolds(view tx.LedgerView, amm *AMMData, fhZeroIfFrozen bool) (asset1Balance, asset2Balance, lptBalance tx.Amount) {
	// Get pool balances from actual state
	asset1Balance, asset2Balance = ammPoolHolds(view, amm.Account, amm.Asset, amm.Asset2, fhZeroIfFrozen)

	// LP token balance is stored in the AMM entry
	lptBalance = amm.LPTokenBalance

	return asset1Balance, asset2Balance, lptBalance
}

// IsAMMEmpty returns true if the AMM has no LP tokens outstanding.
// An empty AMM can be deleted or reinitialized on deposit.
// Reference: rippled checks lpTokens == 0 for empty AMM
func IsAMMEmpty(amm *AMMData) bool {
	return amm.LPTokenBalance.IsZero()
}

// ammLPHolds returns the LP token balance held by an account for an AMM.
// LP tokens are stored in a trustline between the LP account and the AMM account.
// Reference: rippled AMMUtils.cpp ammLPHolds lines 113-160
func ammLPHolds(view tx.LedgerView, amm *AMMData, lpAccountID [20]byte) tx.Amount {
	// LP token currency is derived from the two asset currencies
	lptCurrency := generateAMMLPTCurrency(amm.Asset.Currency, amm.Asset2.Currency)
	ammAccountID := amm.Account

	// Read the trustline between LP account and AMM account
	trustLineKey := keylet.Line(lpAccountID, ammAccountID, lptCurrency)
	data, err := view.Read(trustLineKey)
	if err != nil || data == nil {
		// No trustline = no LP tokens held
		ammAccountAddr, _ := sle.EncodeAccountID(ammAccountID)
		return sle.NewIssuedAmountFromValue(0, -100, lptCurrency, ammAccountAddr)
	}

	// Parse the trustline
	rs, err := sle.ParseRippleState(data)
	if err != nil {
		ammAccountAddr, _ := sle.EncodeAccountID(ammAccountID)
		return sle.NewIssuedAmountFromValue(0, -100, lptCurrency, ammAccountAddr)
	}

	// Determine balance based on canonical ordering
	// Balance is stored from low account's perspective (positive = low owes high)
	// For LP tokens: if LP is low, positive balance means LP holds tokens
	lpIsLow := sle.CompareAccountIDsForLine(lpAccountID, ammAccountID) < 0
	balance := rs.Balance
	if !lpIsLow {
		balance = balance.Negate()
	}

	// Return balance with proper issuer (AMM account)
	ammAccountAddr, _ := sle.EncodeAccountID(ammAccountID)
	if balance.Signum() <= 0 {
		return sle.NewIssuedAmountFromValue(0, -100, lptCurrency, ammAccountAddr)
	}

	return sle.NewIssuedAmountFromValue(balance.Mantissa(), balance.Exponent(), lptCurrency, ammAccountAddr)
}
