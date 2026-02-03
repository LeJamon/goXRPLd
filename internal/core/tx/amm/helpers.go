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
func calculateLPTokens(amount1, amount2 tx.Amount) tx.Amount {
	if amount1.IsZero() || amount2.IsZero() {
		return sle.NewIssuedAmountFromValue(0, -100, "", "")
	}
	// product = amount1 * amount2
	product := amount1.Mul(amount2, false)
	// result = sqrt(product)
	return product.Sqrt()
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
	// fee = tfee / 100000
	fee := getFee(tfee)
	// oneMinusFee = 1 - fee
	oneMinusFee := subFromOne(fee)
	// effectiveDenom = A * (1 - fee)
	effectiveDenom := assetBalance.Mul(oneMinusFee, false)
	// effectiveAmount = a / effectiveDenom = a / (A * (1 - fee))
	effectiveAmount := amountIn.Div(effectiveDenom, false)
	// onePlusEff = 1 + effectiveAmount
	onePlusEff := addToOne(effectiveAmount)
	// sqrtVal = sqrt(1 + effectiveAmount)
	sqrtVal := onePlusEff.Sqrt()
	// factor = sqrtVal - 1
	factor, _ := sqrtVal.Sub(oneAmount())
	// result = T * factor
	return lptBalance.Mul(factor, false)
}

// ammAssetIn calculates the asset amount needed for a specified LP token output (single-asset deposit).
// Equation 3 inverse: a = A * (1-tfee) * (((T+t)/T)^2 - 1)
func ammAssetIn(assetBalance, lptBalance, lpTokensOutAmt tx.Amount, tfee uint16) tx.Amount {
	if lptBalance.IsZero() {
		return sle.NewIssuedAmountFromValue(0, -100, "", "")
	}
	// fee = tfee / 100000
	fee := getFee(tfee)
	// oneMinusFee = 1 - fee
	oneMinusFee := subFromOne(fee)
	// tPlusT = T + t (lptBalance + lpTokensOutAmt)
	tPlusT, _ := lptBalance.Add(lpTokensOutAmt)
	// ratio = (T + t) / T
	ratio := tPlusT.Div(lptBalance, false)
	// ratioSquared = ratio^2
	ratioSquared := ratio.Mul(ratio, false)
	// factor = ratioSquared - 1
	factor, _ := ratioSquared.Sub(oneAmount())
	// result = A * (1 - fee) * factor
	aTimesFee := assetBalance.Mul(oneMinusFee, false)
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
	// fee = tfee / 100000
	fee := getFee(tfee)
	// oneMinusFee = 1 - fee
	oneMinusFee := subFromOne(fee)
	// tDivT = t / T (lpTokensIn / lptBalance)
	tDivT := lpTokensIn.Div(lptBalance, false)
	// ratio = 1 - t/T
	ratio := subFromOne(tDivT)
	// ratioSquared = ratio^2
	ratioSquared := ratio.Mul(ratio, false)
	// factor = 1 - ratioSquared
	factor := subFromOne(ratioSquared)
	// factorWithFee = factor * (1 - fee)
	factorWithFee := factor.Mul(oneMinusFee, false)
	// result = A * factorWithFee
	return assetBalance.Mul(factorWithFee, false)
}

// calcLPTokensIn calculates LP tokens needed for a single-asset withdrawal amount.
// Equation 7: t = T * (1 - (1 - a/(A*(1-tfee)))^0.5)
func calcLPTokensIn(assetBalance, amountOut, lptBalance tx.Amount, tfee uint16) tx.Amount {
	if assetBalance.IsZero() || lptBalance.IsZero() {
		return sle.NewIssuedAmountFromValue(0, -100, "", "")
	}
	// fee = tfee / 100000
	fee := getFee(tfee)
	// oneMinusFee = 1 - fee
	oneMinusFee := subFromOne(fee)
	// effectiveDenom = A * (1 - fee)
	effectiveDenom := assetBalance.Mul(oneMinusFee, false)
	// effectiveAmount = amountOut / effectiveDenom
	effectiveAmount := amountOut.Div(effectiveDenom, false)
	// Check if effectiveAmount >= 1 (would drain the pool)
	if effectiveAmount.Compare(oneAmount()) >= 0 {
		return lptBalance // Would drain the pool
	}
	// oneMinusEff = 1 - effectiveAmount
	oneMinusEff := subFromOne(effectiveAmount)
	// sqrtVal = sqrt(1 - effectiveAmount)
	sqrtVal := oneMinusEff.Sqrt()
	// factor = 1 - sqrtVal
	factor := subFromOne(sqrtVal)
	// result = T * factor
	return lptBalance.Mul(factor, false)
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

// parseAMMData deserializes an AMM ledger entry from binary data.
func parseAMMData(data []byte) (*AMMData, error) {
	if len(data) < 62 {
		return nil, fmt.Errorf("AMM data too short: %d bytes", len(data))
	}

	amm := &AMMData{}

	offset := 0

	// Account (20 bytes)
	copy(amm.Account[:], data[offset:offset+20])
	offset += 20

	// Asset (20 bytes)
	copy(amm.Asset[:], data[offset:offset+20])
	offset += 20

	// Asset2 (20 bytes)
	copy(amm.Asset2[:], data[offset:offset+20])
	offset += 20

	// TradingFee (2 bytes)
	if offset+2 > len(data) {
		return amm, nil
	}
	amm.TradingFee = binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	// LPTokenBalance (variable: 9 or 13 bytes)
	if offset >= len(data) {
		return amm, nil
	}
	amt, consumed := deserializeAmount(data[offset:])
	amm.LPTokenBalance = amt
	offset += consumed

	// AssetBalance (variable: 9 or 13 bytes)
	if offset >= len(data) {
		return amm, nil
	}
	amt, consumed = deserializeAmount(data[offset:])
	amm.AssetBalance = amt
	offset += consumed

	// Asset2Balance (variable: 9 or 13 bytes)
	if offset >= len(data) {
		return amm, nil
	}
	amt, consumed = deserializeAmount(data[offset:])
	amm.Asset2Balance = amt
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

// serializeAMM serializes an AMMData entry with owner account for ledger storage.
func serializeAMM(amm *AMMData, ownerID [20]byte) ([]byte, error) {
	// Set the account to the owner
	amm.Account = ownerID
	return serializeAMMData(amm)
}

// serializeAMMData serializes an AMMData entry to binary.
func serializeAMMData(amm *AMMData) ([]byte, error) {
	// Calculate size
	size := 20 + 20 + 20 + 2 + 8 + 8 + 8 + 1 // Account + Asset + Asset2 + TradingFee + LPTokenBalance + AssetBalance + Asset2Balance + voteCount
	size += len(amm.VoteSlots) * 26          // Each vote slot: 20 + 2 + 4
	size += 1                                // hasAuctionSlot flag
	if amm.AuctionSlot != nil {
		size += 20 + 4 + 8 + 1 // Account + Expiration + Price + authCount
		size += len(amm.AuctionSlot.AuthAccounts) * 20
	}

	data := make([]byte, size)
	offset := 0

	// Account
	copy(data[offset:offset+20], amm.Account[:])
	offset += 20

	// Asset
	copy(data[offset:offset+20], amm.Asset[:])
	offset += 20

	// Asset2
	copy(data[offset:offset+20], amm.Asset2[:])
	offset += 20

	// TradingFee
	binary.BigEndian.PutUint16(data[offset:offset+2], amm.TradingFee)
	offset += 2

	// LPTokenBalance
	binary.BigEndian.PutUint64(data[offset:offset+8], amm.LPTokenBalance)
	offset += 8

	// AssetBalance
	binary.BigEndian.PutUint64(data[offset:offset+8], amm.AssetBalance)
	offset += 8

	// Asset2Balance
	binary.BigEndian.PutUint64(data[offset:offset+8], amm.Asset2Balance)
	offset += 8

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
		binary.BigEndian.PutUint64(data[offset:offset+8], amm.AuctionSlot.Price)
		offset += 8
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
// This is a simplified implementation for the AMM sub-package.
func createOrUpdateAMMTrustline(ammAccountID [20]byte, asset tx.Asset, amount uint64, view tx.LedgerView) error {
	// In a full implementation, this would create/update the trust line
	// between the AMM account and the asset issuer.
	// For now, this is a no-op as trust line operations are handled
	// at a higher level by the engine.
	_ = ammAccountID
	_ = asset
	_ = amount
	_ = view
	return nil
}

// updateTrustlineBalanceInView updates the balance of a trust line for IOU transfers.
// This reads the trust line, modifies the balance, and writes it back.
// delta is positive when transferring TO the account, negative when transferring FROM.
func updateTrustlineBalanceInView(accountID [20]byte, issuerID [20]byte, currency string, delta int64, view tx.LedgerView) error {
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
	currentBalance := rs.Balance.Float64()
	if !isLow {
		currentBalance = -currentBalance
	}

	// Apply delta (positive = receiving, negative = sending)
	newBalance := currentBalance + float64(delta)

	// Convert back to RippleState balance convention
	if !isLow {
		newBalance = -newBalance
	}

	// Update the balance
	rs.Balance = sle.NewIssuedAmountFromFloat64(newBalance, currency, sle.AccountOneAddress)

	// Serialize and write back
	serialized, err := sle.SerializeRippleState(rs)
	if err != nil {
		return err
	}

	return view.Update(lineKey, serialized)
}

// createLPTokenTrustline creates a trust line for LP tokens.
// This is a simplified implementation for the AMM sub-package.
func createLPTokenTrustline(accountID [20]byte, lptAsset tx.Asset, amount uint64, view tx.LedgerView) error {
	// In a full implementation, this would create the LP token trust line
	// for the depositor.
	_ = accountID
	_ = lptAsset
	_ = amount
	_ = view
	return nil
}
