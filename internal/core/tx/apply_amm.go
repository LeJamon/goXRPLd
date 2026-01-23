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

// parseAmount parses an amount string to uint64
func parseAmount(value string) uint64 {
	amount, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		// Try parsing as float for IOU amounts
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0
		}
		// Convert to drops (assuming 6 decimal places for IOUs)
		return uint64(f * 1000000)
	}
	return amount
}

// AMM data structures

// AMMData represents an AMM ledger entry
type AMMData struct {
	Account        [20]byte // AMM account
	Asset          [20]byte // First asset currency (20 bytes)
	Asset2         [20]byte // Second asset currency (20 bytes)
	TradingFee     uint16
	LPTokenBalance uint64
	VoteSlots      []VoteSlotData
	AuctionSlot    *AuctionSlotData
}

// VoteSlotData represents a voting slot in an AMM
type VoteSlotData struct {
	Account    [20]byte
	TradingFee uint16
	VoteWeight uint32
}

// AuctionSlotData represents the auction slot in an AMM
type AuctionSlotData struct {
	Account      [20]byte
	Expiration   uint32
	Price        uint64
	AuthAccounts [][20]byte
}

// computeAMMAccountID derives the AMM account ID from the AMM keylet
// Reference: rippled AMMUtils.cpp createPseudoAccount
func computeAMMAccountID(ammKeyletKey [32]byte) [20]byte {
	var result [20]byte
	copy(result[:], ammKeyletKey[:20])
	return result
}

// computeAMMKeylet computes the AMM keylet from asset pair
func computeAMMKeylet(asset1, asset2 Asset) keylet.Keylet {
	// Convert assets to [20]byte format for keylet computation
	var issuer1, currency1, issuer2, currency2 [20]byte

	// Parse asset1
	if asset1.Currency == "" || asset1.Currency == "XRP" {
		// XRP - zero issuer and currency
	} else {
		currency1 = currencyToBytes(asset1.Currency)
		if asset1.Issuer != "" {
			issuerID, _ := decodeAccountID(asset1.Issuer)
			issuer1 = issuerID
		}
	}

	// Parse asset2
	if asset2.Currency == "" || asset2.Currency == "XRP" {
		// XRP - zero issuer and currency
	} else {
		currency2 = currencyToBytes(asset2.Currency)
		if asset2.Issuer != "" {
			issuerID, _ := decodeAccountID(asset2.Issuer)
			issuer2 = issuerID
		}
	}

	return keylet.AMM(issuer1, currency1, issuer2, currency2)
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

// calculateLPTokensFloat calculates LP tokens for IOU amounts
func calculateLPTokensFloat(amount1, amount2 float64) float64 {
	return math.Sqrt(amount1 * amount2)
}

// generateAMMLPTCurrency generates the LP token currency code
// Format: 03 + first 19 bytes of sha512half(currency1 + currency2)
func generateAMMLPTCurrency(currency1, currency2 string) string {
	// Normalize currencies
	c1 := currency1
	c2 := currency2
	if c1 == "" {
		c1 = "XRP"
	}
	if c2 == "" {
		c2 = "XRP"
	}

	// Hash the concatenation
	data := []byte(c1 + c2)
	hash := crypto.Sha512Half(data)

	// Build currency: 03 prefix + first 19 bytes of hash
	var lptCurrency [20]byte
	lptCurrency[0] = 0x03
	copy(lptCurrency[1:], hash[:19])

	return strings.ToUpper(hex.EncodeToString(lptCurrency[:]))
}


// createOrUpdateAMMTrustline creates or updates a trustline for the AMM account
func createOrUpdateAMMTrustline(ammAccountID [20]byte, asset Asset, balance uint64, view LedgerView) error {
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

	return view.Insert(lineKey, decoded)
}

// updateTrustlineBalance updates a trustline balance
func updateTrustlineBalance(accountID [20]byte, asset Asset, delta int64) error {
	if asset.Currency == "" || asset.Currency == "XRP" {
		return nil
	}
	// In a full implementation, this would read the trustline,
	// update the balance, and re-serialize
	return nil
}

// createLPTokenTrustline creates an LP token trustline for a liquidity provider
func createLPTokenTrustline(accountID [20]byte, lptAsset Asset, balance uint64, view LedgerView) error {
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

	return view.Insert(lineKey, decoded)
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

// AMM Math Helper Functions
// Reference: rippled AMMHelpers.cpp

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

// lpTokensOut calculates LP tokens for single asset deposit (Equation 3)
// t = T * (b/B - x) / (1 + x)
// where x = sqrt(f2² + b/(B*f1)) - f2, f1 = 1-tfee, f2 = (1-tfee/2)/f1
func lpTokensOut(assetBalance, assetDeposit, lptBalance uint64, tfee uint16) uint64 {
	if assetBalance == 0 || lptBalance == 0 {
		return 0
	}

	f1 := feeMult(tfee)
	f2 := feeMultHalf(tfee) / f1
	r := float64(assetDeposit) / float64(assetBalance)

	x := math.Sqrt(f2*f2+r/f1) - f2
	t := float64(lptBalance) * (r - x) / (1 + x)

	if t < 0 {
		return 0
	}
	return uint64(t)
}

// ammAssetIn calculates asset needed for desired LP tokens (Equation 4)
// Solves equation 3 for b (asset deposit amount)
func ammAssetIn(assetBalance, lptBalance, lpTokens uint64, tfee uint16) uint64 {
	if lptBalance == 0 {
		return 0
	}

	f1 := feeMult(tfee)
	f2 := feeMultHalf(tfee) / f1
	t1 := float64(lpTokens) / float64(lptBalance)
	t2 := 1 + t1
	d := f2 - t1/t2
	a := 1 / (t2 * t2)
	b := 2*d/t2 - 1/f1
	c := d*d - f2*f2

	// Solve quadratic: ax² + bx + c = 0
	discriminant := b*b - 4*a*c
	if discriminant < 0 {
		return 0
	}
	R := (-b + math.Sqrt(discriminant)) / (2 * a)

	return uint64(float64(assetBalance) * R)
}

// calcLPTokensIn calculates LP tokens to burn for single asset withdrawal (Equation 7)
// t = T * (c - sqrt(c² - 4*R)) / 2
// where R = b/B, c = R*fee + 2 - fee
func calcLPTokensIn(assetBalance, assetWithdraw, lptBalance uint64, tfee uint16) uint64 {
	if assetBalance == 0 || lptBalance == 0 {
		return 0
	}

	R := float64(assetWithdraw) / float64(assetBalance)
	f := getFee(tfee)
	c := R*f + 2 - f

	discriminant := c*c - 4*R
	if discriminant < 0 {
		return 0
	}
	t := float64(lptBalance) * (c - math.Sqrt(discriminant)) / 2

	if t < 0 {
		return 0
	}
	return uint64(t)
}

// ammAssetOut calculates asset amount for LP tokens burned (Equation 8)
// b = B * (t1² - t1*(2-f)) / (t1*f - 1)
// where t1 = t/T
func ammAssetOut(assetBalance, lptBalance, lpTokens uint64, tfee uint16) uint64 {
	if lptBalance == 0 {
		return 0
	}

	f := getFee(tfee)
	t1 := float64(lpTokens) / float64(lptBalance)

	denominator := t1*f - 1
	if denominator == 0 {
		return 0
	}

	b := float64(assetBalance) * (t1*t1 - t1*(2-f)) / denominator

	if b < 0 {
		return 0
	}
	return uint64(b)
}

// calculateWeightedFee calculates the weighted average trading fee from vote slots
func calculateWeightedFee(voteSlots []VoteSlotData) uint16 {
	if len(voteSlots) == 0 {
		return 0
	}

	var totalWeight uint64
	var weightedSum uint64

	for _, slot := range voteSlots {
		totalWeight += uint64(slot.VoteWeight)
		weightedSum += uint64(slot.VoteWeight) * uint64(slot.TradingFee)
	}

	if totalWeight == 0 {
		return 0
	}

	return uint16(weightedSum / totalWeight)
}

// parseAMMData parses AMM data from binary format
func parseAMMData(data []byte) (*AMMData, error) {
	// Simplified parsing - in production would use binary codec
	amm := &AMMData{
		VoteSlots: make([]VoteSlotData, 0),
	}
	// For now, return empty AMM data structure
	// In a full implementation, this would parse the binary format
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
		"LPTokenBalance":  fmt.Sprintf("%d", amm.LPTokenBalance),
		"Flags":           uint32(0),
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode AMM: %w", err)
	}

	return hex.DecodeString(hexStr)
}



// AMM vote constants
// Reference: rippled AMMCore.h
const (
	voteMaxSlots             = 8      // Maximum vote slots
	voteWeightScaleFactor    = 100000 // Scale factor for vote weight
	auctionSlotDiscountedFee = 10     // Discounted fee fraction (tradingFee / 10)
)


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

// AMM auction slot constants
// Reference: rippled AMMCore.h
const (
	auctionSlotTotalTimeSecs    = 86400 // 24 hours in seconds
	auctionSlotTimeIntervals    = 20    // Number of time intervals
	auctionSlotMinFeeFraction   = 25    // Min slot price = lptBalance * fee / 25
	auctionSlotMaxAuthAccounts  = 4     // Maximum authorized accounts
	auctionSlotIntervalDuration = auctionSlotTotalTimeSecs / auctionSlotTimeIntervals
)




// serializeAMM serializes an AMM ledger entry
func serializeAMM(amm *AMMData, ownerID [20]byte) ([]byte, error) {
	accountAddress, err := addresscodec.EncodeAccountIDToClassicAddress(amm.Account[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode AMM account address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "AMM",
		"Account":         accountAddress,
		"TradingFee":      amm.TradingFee,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode AMM: %w", err)
	}

	return hex.DecodeString(hexStr)
}
