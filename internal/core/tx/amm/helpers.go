package amm

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

// Internal constants (lowercase aliases of exported AMM constants)
const (
	voteMaxSlots          = VOTE_MAX_SLOTS
	voteWeightScaleFactor = VOTE_WEIGHT_SCALE_FACTOR

	auctionSlotDiscountedFee    = AUCTION_SLOT_DISCOUNTED_FEE_FRACTION
	auctionSlotMinFeeFraction   = AUCTION_SLOT_MIN_FEE_FRACTION
	auctionSlotTimeIntervals    = AUCTION_SLOT_TIME_INTERVALS
	auctionSlotTotalTimeSecs    = uint32(TOTAL_TIME_SLOT_SECS)
	auctionSlotIntervalDuration = auctionSlotTotalTimeSecs / auctionSlotTimeIntervals
)

// AccountRoot flags needed by AMMClawback
const (
	lsfAllowTrustLineClawback = sle.LsfAllowTrustLineClawback
	lsfNoFreeze               = sle.LsfNoFreeze
	lsfAMM                    = sle.LsfAMM
)

// Result code aliases for AMM-specific codes
var (
	TecUNFUNDED_AMM       = tx.TecUNFUNDED_AMM
	TecNO_LINE            = tx.TecNO_LINE
	TecINSUF_RESERVE_LINE = tx.TecINSUF_RESERVE_LINE
	TerNO_AMM             = tx.TerNO_AMM
	TerNO_ACCOUNT         = tx.TerNO_ACCOUNT
)

// AMMData holds the internal AMM ledger entry data.
type AMMData struct {
	Account        [20]byte
	Asset          [20]byte
	Asset2         [20]byte
	TradingFee     uint16
	LPTokenBalance uint64
	VoteSlots      []VoteSlotData
	AuctionSlot    *AuctionSlotData
}

// VoteSlotData holds a single vote slot entry.
type VoteSlotData struct {
	Account    [20]byte
	TradingFee uint16
	VoteWeight uint32
}

// AuctionSlotData holds the auction slot state.
type AuctionSlotData struct {
	Account      [20]byte
	Expiration   uint32
	Price        uint64
	AuthAccounts [][20]byte
}

// parseAmountFromTx parses a tx.Amount to uint64 (drops for XRP, scaled for IOU).
func parseAmountFromTx(amt *tx.Amount) uint64 {
	if amt == nil {
		return 0
	}
	if amt.IsNative() {
		drops := amt.Drops()
		if drops < 0 {
			return 0
		}
		return uint64(drops)
	}
	// For IOU, use float value and scale to drops-equivalent
	f := amt.Float64()
	if f < 0 {
		return 0
	}
	return uint64(f * 1000000)
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
func calculateLPTokens(amount1, amount2 uint64) uint64 {
	// Use float64 for the multiplication to avoid overflow
	product := float64(amount1) * float64(amount2)
	if product <= 0 {
		return 0
	}
	return uint64(math.Sqrt(product))
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

// getFee converts a trading fee in basis points (0-1000) to a fractional value.
// 1000 basis points = 1% = 0.01
func getFee(fee uint16) float64 {
	return float64(fee) / 100000.0
}

// lpTokensOut calculates LP tokens issued for a single-asset deposit.
// Equation 4: t = T * ((1 + a/(A*(1-tfee)))^0.5 - 1)
func lpTokensOut(assetBalance, amountIn, lptBalance uint64, tfee uint16) uint64 {
	if assetBalance == 0 || lptBalance == 0 {
		return 0
	}
	fee := getFee(tfee)
	effectiveAmount := float64(amountIn) / (float64(assetBalance) * (1.0 - fee))
	factor := math.Sqrt(1.0+effectiveAmount) - 1.0
	return uint64(float64(lptBalance) * factor)
}

// ammAssetIn calculates the asset amount needed for a specified LP token output (single-asset deposit).
// Equation 3 inverse: a = A * (1-tfee) * ((T+t)/T)^2 - 1)
func ammAssetIn(assetBalance, lptBalance, lpTokensOut uint64, tfee uint16) uint64 {
	if lptBalance == 0 {
		return 0
	}
	fee := getFee(tfee)
	ratio := float64(lptBalance+lpTokensOut) / float64(lptBalance)
	factor := ratio*ratio - 1.0
	return uint64(float64(assetBalance) * (1.0 - fee) * factor)
}

// ammAssetOut calculates the asset amount received for burning LP tokens (single-asset withdrawal).
// Equation 8: a = A * (1 - (1 - t/T)^2) * (1 - tfee)
func ammAssetOut(assetBalance, lptBalance, lpTokensIn uint64, tfee uint16) uint64 {
	if lptBalance == 0 || lpTokensIn > lptBalance {
		return 0
	}
	fee := getFee(tfee)
	ratio := 1.0 - float64(lpTokensIn)/float64(lptBalance)
	factor := (1.0 - ratio*ratio) * (1.0 - fee)
	return uint64(float64(assetBalance) * factor)
}

// calcLPTokensIn calculates LP tokens needed for a single-asset withdrawal amount.
// Equation 7: t = T * (1 - (1 - a/(A*(1-tfee)))^0.5)
func calcLPTokensIn(assetBalance, amountOut, lptBalance uint64, tfee uint16) uint64 {
	if assetBalance == 0 || lptBalance == 0 {
		return 0
	}
	fee := getFee(tfee)
	effectiveAmount := float64(amountOut) / (float64(assetBalance) * (1.0 - fee))
	if effectiveAmount >= 1.0 {
		return lptBalance // Would drain the pool
	}
	factor := 1.0 - math.Sqrt(1.0-effectiveAmount)
	return uint64(float64(lptBalance) * factor)
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

	// LPTokenBalance (8 bytes)
	if offset+8 > len(data) {
		return amm, nil
	}
	amm.LPTokenBalance = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

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

	if hasAuctionSlot != 0 && offset+32 <= len(data) {
		slot := &AuctionSlotData{
			AuthAccounts: make([][20]byte, 0),
		}
		copy(slot.Account[:], data[offset:offset+20])
		offset += 20
		slot.Expiration = binary.BigEndian.Uint32(data[offset : offset+4])
		offset += 4
		slot.Price = binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

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
	size := 20 + 20 + 20 + 2 + 8 + 1 // Account + Asset + Asset2 + TradingFee + LPTokenBalance + voteCount
	size += len(amm.VoteSlots) * 26  // Each vote slot: 20 + 2 + 4
	size += 1                        // hasAuctionSlot flag
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

// updateTrustlineBalance updates the balance of a trust line.
// This is a simplified implementation for the AMM sub-package.
func updateTrustlineBalance(accountID [20]byte, asset tx.Asset, delta int64) error {
	// In a full implementation, this would read the trust line,
	// update the balance, and write it back.
	_ = accountID
	_ = asset
	_ = delta
	return nil
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
