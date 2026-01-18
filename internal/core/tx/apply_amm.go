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

// applyAMMCreate applies an AMMCreate transaction
// Reference: rippled AMMCreate.cpp doApply / applyCreate
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

	// Check if AMM account already exists (should not happen)
	ammAccountKey := keylet.Account(ammAccountID)
	acctExists, _ := e.view.Exists(ammAccountKey)
	if acctExists {
		return TecDUPLICATE
	}

	// Parse amounts
	amount1 := parseAmount(tx.Amount.Value)
	amount2 := parseAmount(tx.Amount2.Value)

	// Check creator has sufficient balance
	isXRP1 := tx.Amount.Currency == "" || tx.Amount.Currency == "XRP"
	isXRP2 := tx.Amount2.Currency == "" || tx.Amount2.Currency == "XRP"

	// For XRP amounts, verify balance
	totalXRPNeeded := uint64(0)
	if isXRP1 {
		totalXRPNeeded += amount1
	}
	if isXRP2 {
		totalXRPNeeded += amount2
	}
	if totalXRPNeeded > 0 && account.Balance < totalXRPNeeded {
		return TecUNFUNDED_AMM
	}

	// Calculate initial LP token balance: sqrt(amount1 * amount2)
	var lpTokenBalance uint64
	if amount1 > 0 && amount2 > 0 {
		lpTokenBalance = calculateLPTokens(amount1, amount2)
	}
	if lpTokenBalance == 0 {
		return TecAMM_BALANCE // AMM empty or invalid LP token calculation
	}

	// Generate LP token currency code
	lptCurrency := generateAMMLPTCurrency(tx.Amount.Currency, tx.Amount2.Currency)

	// Create the AMM pseudo-account with lsfAMM flag
	ammAccount := &AccountRoot{
		Account:    ammAccountAddr,
		Balance:    0,
		Sequence:   0,
		OwnerCount: 1, // For the AMM entry itself
		Flags:      lsfAMM,
	}

	// Create the AMM entry
	ammData := &AMMData{
		Account:        ammAccountID,
		TradingFee:     tx.TradingFee,
		LPTokenBalance: lpTokenBalance,
		VoteSlots:      make([]VoteSlotData, 0),
	}

	// Set asset currency bytes
	ammData.Asset = currencyToBytes(tx.Amount.Currency)
	ammData.Asset2 = currencyToBytes(tx.Amount2.Currency)

	// Initialize creator's vote slot with their LP token weight
	creatorVote := VoteSlotData{
		Account:    accountID,
		TradingFee: tx.TradingFee,
		VoteWeight: uint32(lpTokenBalance), // Truncate for vote weight
	}
	ammData.VoteSlots = append(ammData.VoteSlots, creatorVote)

	// Initialize auction slot (creator gets initial slot)
	ammData.AuctionSlot = &AuctionSlotData{
		Account:      accountID,
		Expiration:   0, // No expiration initially
		Price:        0,
		AuthAccounts: make([][20]byte, 0),
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

	// Transfer XRP from creator to AMM account
	if isXRP1 {
		account.Balance -= amount1
		ammAccount.Balance += amount1
	}
	if isXRP2 {
		account.Balance -= amount2
		ammAccount.Balance += amount2
	}

	// For IOU transfers, update trustlines
	if !isXRP1 {
		if err := e.createOrUpdateAMMTrustline(ammAccountID, asset1, amount1); err != nil {
			return TecNO_LINE
		}
		// Debit from creator's trustline
		if err := e.updateTrustlineBalance(accountID, asset1, -int64(amount1)); err != nil {
			return TecUNFUNDED_AMM
		}
	}
	if !isXRP2 {
		if err := e.createOrUpdateAMMTrustline(ammAccountID, asset2, amount2); err != nil {
			return TecNO_LINE
		}
		if err := e.updateTrustlineBalance(accountID, asset2, -int64(amount2)); err != nil {
			return TecUNFUNDED_AMM
		}
	}

	// Create LP token trustline for creator
	lptAsset := Asset{
		Currency: lptCurrency,
		Issuer:   ammAccountAddr,
	}
	if err := e.createLPTokenTrustline(accountID, lptAsset, lpTokenBalance); err != nil {
		return TecINSUF_RESERVE_LINE
	}

	// Update creator account (owner count increases for LP token trustline)
	account.OwnerCount++

	// Persist updated creator account
	accountKey := keylet.Account(accountID)
	accountBytes, err := serializeAccountRoot(account)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(accountKey, accountBytes); err != nil {
		return TefINTERNAL
	}

	// Update AMM account balance (for XRP)
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
			"LPTokenBalance": fmt.Sprintf("%d", lpTokenBalance),
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
	// In a full implementation, this would read the trustline,
	// update the balance, and re-serialize
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

// applyAMMDeposit applies an AMMDeposit transaction
// Reference: rippled AMMDeposit.cpp applyGuts
func (e *Engine) applyAMMDeposit(tx *AMMDeposit, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Find the AMM
	ammKey := computeAMMKeylet(tx.Asset, tx.Asset2)

	ammData, err := e.view.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	// Parse AMM data
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
	var amount1, amount2, lpTokensRequested uint64
	if tx.Amount != nil {
		amount1 = parseAmount(tx.Amount.Value)
	}
	if tx.Amount2 != nil {
		amount2 = parseAmount(tx.Amount2.Value)
	}
	if tx.LPTokenOut != nil {
		lpTokensRequested = parseAmount(tx.LPTokenOut.Value)
	}

	// Get current AMM balances (simplified - using stored balance)
	assetBalance1 := ammAccount.Balance // For XRP
	assetBalance2 := uint64(0)          // Would come from trustline
	lptBalance := amm.LPTokenBalance

	var lpTokensToIssue uint64
	var depositAmount1, depositAmount2 uint64

	// Handle different deposit modes
	switch {
	case flags&tfLPToken != 0:
		// Proportional deposit for specified LP tokens
		if lpTokensRequested == 0 || lptBalance == 0 {
			return TecAMM_INVALID_TOKENS
		}
		frac := float64(lpTokensRequested) / float64(lptBalance)
		depositAmount1 = uint64(float64(assetBalance1) * frac)
		depositAmount2 = uint64(float64(assetBalance2) * frac)
		lpTokensToIssue = lpTokensRequested

	case flags&tfSingleAsset != 0:
		// Single asset deposit
		lpTokensToIssue = lpTokensOut(assetBalance1, amount1, lptBalance, tfee)
		if lpTokensToIssue == 0 {
			return TecAMM_INVALID_TOKENS
		}
		depositAmount1 = amount1

	case flags&tfTwoAsset != 0:
		// Two asset deposit with limits
		frac1 := float64(amount1) / float64(assetBalance1)
		frac2 := float64(amount2) / float64(assetBalance2)
		// Use the smaller fraction to maintain ratio
		frac := frac1
		if assetBalance2 > 0 && frac2 < frac1 {
			frac = frac2
		}
		lpTokensToIssue = uint64(float64(lptBalance) * frac)
		depositAmount1 = uint64(float64(assetBalance1) * frac)
		depositAmount2 = uint64(float64(assetBalance2) * frac)

	case flags&tfOneAssetLPToken != 0:
		// Single asset deposit for specific LP tokens
		depositAmount1 = ammAssetIn(assetBalance1, lptBalance, lpTokensRequested, tfee)
		if depositAmount1 > amount1 {
			return TecAMM_FAILED
		}
		lpTokensToIssue = lpTokensRequested

	case flags&tfLimitLPToken != 0:
		// Single asset deposit with effective price limit
		lpTokensToIssue = lpTokensOut(assetBalance1, amount1, lptBalance, tfee)
		if lpTokensToIssue == 0 {
			return TecAMM_INVALID_TOKENS
		}
		// Check effective price
		if tx.EPrice != nil {
			ePrice := parseAmount(tx.EPrice.Value)
			if ePrice > 0 && amount1/lpTokensToIssue > ePrice {
				return TecAMM_FAILED
			}
		}
		depositAmount1 = amount1

	case flags&tfTwoAssetIfEmpty != 0:
		// Deposit into empty AMM
		if lptBalance != 0 {
			return TecAMM_NOT_EMPTY
		}
		lpTokensToIssue = calculateLPTokens(amount1, amount2)
		depositAmount1 = amount1
		depositAmount2 = amount2
		// Set trading fee if provided
		if tx.TradingFee > 0 {
			amm.TradingFee = tx.TradingFee
		}

	default:
		return TemMALFORMED
	}

	if lpTokensToIssue == 0 {
		return TecAMM_INVALID_TOKENS
	}

	// Check depositor has sufficient balance
	isXRP1 := tx.Asset.Currency == "" || tx.Asset.Currency == "XRP"
	isXRP2 := tx.Asset2.Currency == "" || tx.Asset2.Currency == "XRP"

	totalXRPNeeded := uint64(0)
	if isXRP1 && depositAmount1 > 0 {
		totalXRPNeeded += depositAmount1
	}
	if isXRP2 && depositAmount2 > 0 {
		totalXRPNeeded += depositAmount2
	}
	if totalXRPNeeded > 0 && account.Balance < totalXRPNeeded {
		return TecUNFUNDED_AMM
	}

	// Transfer assets from depositor to AMM
	if isXRP1 && depositAmount1 > 0 {
		account.Balance -= depositAmount1
		ammAccount.Balance += depositAmount1
	}
	if isXRP2 && depositAmount2 > 0 {
		account.Balance -= depositAmount2
		ammAccount.Balance += depositAmount2
	}

	// Issue LP tokens to depositor
	amm.LPTokenBalance += lpTokensToIssue

	// Update LP token trustline for depositor
	ammAccountAddr, _ := encodeAccountID(ammAccountID)
	lptCurrency := generateAMMLPTCurrency(tx.Asset.Currency, tx.Asset2.Currency)
	lptAsset := Asset{Currency: lptCurrency, Issuer: ammAccountAddr}
	e.createLPTokenTrustline(accountID, lptAsset, lpTokensToIssue)

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

	// Persist updated depositor account
	accountKey := keylet.Account(accountID)
	accountBytes, err := serializeAccountRoot(account)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(accountKey, accountBytes); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(ammKey.Key[:])),
		FinalFields: map[string]any{
			"LPTokenBalance": fmt.Sprintf("%d", amm.LPTokenBalance),
		},
	})

	return TesSUCCESS
}

// applyAMMWithdraw applies an AMMWithdraw transaction
// Reference: rippled AMMWithdraw.cpp applyGuts
func (e *Engine) applyAMMWithdraw(tx *AMMWithdraw, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Find the AMM
	ammKey := computeAMMKeylet(tx.Asset, tx.Asset2)

	ammData, err := e.view.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	// Parse AMM data
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
	var amount1, amount2, lpTokensRequested uint64
	if tx.Amount != nil {
		amount1 = parseAmount(tx.Amount.Value)
	}
	if tx.Amount2 != nil {
		amount2 = parseAmount(tx.Amount2.Value)
	}
	if tx.LPTokenIn != nil {
		lpTokensRequested = parseAmount(tx.LPTokenIn.Value)
	}

	// Get current AMM balances
	assetBalance1 := ammAccount.Balance // For XRP
	assetBalance2 := uint64(0)          // Would come from trustline for IOU
	lptBalance := amm.LPTokenBalance

	if lptBalance == 0 {
		return TecAMM_BALANCE // AMM empty
	}

	// Get withdrawer's LP token balance (simplified - use what they're trying to withdraw)
	// In full implementation, would read from trustline
	lpTokensHeld := lpTokensRequested
	if flags&(tfWithdrawAll|tfOneAssetWithdrawAll) != 0 {
		lpTokensHeld = lptBalance // For withdraw all, use full balance
	}

	var lpTokensToRedeem uint64
	var withdrawAmount1, withdrawAmount2 uint64

	// Handle different withdrawal modes
	// Reference: rippled AMMWithdraw.cpp applyGuts switch
	switch {
	case flags&tfLPToken != 0:
		// Proportional withdrawal for specified LP tokens
		// Equations 5 and 6: a = (t/T) * A, b = (t/T) * B
		if lpTokensRequested == 0 || lptBalance == 0 {
			return TecAMM_INVALID_TOKENS
		}
		if lpTokensRequested > lpTokensHeld || lpTokensRequested > lptBalance {
			return TecAMM_INVALID_TOKENS
		}
		frac := float64(lpTokensRequested) / float64(lptBalance)
		withdrawAmount1 = uint64(float64(assetBalance1) * frac)
		withdrawAmount2 = uint64(float64(assetBalance2) * frac)
		lpTokensToRedeem = lpTokensRequested

	case flags&tfWithdrawAll != 0:
		// Withdraw all - proportional withdrawal of all LP tokens held
		if lpTokensHeld == 0 {
			return TecAMM_INVALID_TOKENS
		}
		if lpTokensHeld >= lptBalance {
			// Last LP withdrawing everything
			withdrawAmount1 = assetBalance1
			withdrawAmount2 = assetBalance2
			lpTokensToRedeem = lptBalance
		} else {
			frac := float64(lpTokensHeld) / float64(lptBalance)
			withdrawAmount1 = uint64(float64(assetBalance1) * frac)
			withdrawAmount2 = uint64(float64(assetBalance2) * frac)
			lpTokensToRedeem = lpTokensHeld
		}

	case flags&tfOneAssetWithdrawAll != 0:
		// Withdraw all LP tokens as a single asset
		// Use equation 8: ammAssetOut
		if lpTokensHeld == 0 || amount1 == 0 {
			return TecAMM_INVALID_TOKENS
		}
		withdrawAmount1 = ammAssetOut(assetBalance1, lptBalance, lpTokensHeld, tfee)
		if withdrawAmount1 > assetBalance1 {
			return TecAMM_BALANCE
		}
		lpTokensToRedeem = lpTokensHeld

	case flags&tfSingleAsset != 0:
		// Single asset withdrawal - compute LP tokens from amount
		// Equation 7: lpTokensIn function
		if amount1 == 0 {
			return TemMALFORMED
		}
		if amount1 > assetBalance1 {
			return TecAMM_BALANCE
		}
		lpTokensToRedeem = calcLPTokensIn(assetBalance1, amount1, lptBalance, tfee)
		if lpTokensToRedeem == 0 || lpTokensToRedeem > lpTokensHeld {
			return TecAMM_INVALID_TOKENS
		}
		withdrawAmount1 = amount1

	case flags&tfTwoAsset != 0:
		// Two asset withdrawal with limits
		// Equations 5 and 6 with limits
		if amount1 == 0 || amount2 == 0 {
			return TemMALFORMED
		}
		// Calculate proportional withdrawal
		frac1 := float64(amount1) / float64(assetBalance1)
		frac2 := float64(amount2) / float64(assetBalance2)
		// Use the smaller fraction
		frac := frac1
		if assetBalance2 > 0 && frac2 < frac1 {
			frac = frac2
		}
		lpTokensToRedeem = uint64(float64(lptBalance) * frac)
		if lpTokensToRedeem == 0 || lpTokensToRedeem > lpTokensHeld {
			return TecAMM_INVALID_TOKENS
		}
		// Recalculate amounts based on the fraction used
		withdrawAmount1 = uint64(float64(assetBalance1) * frac)
		withdrawAmount2 = uint64(float64(assetBalance2) * frac)

	case flags&tfOneAssetLPToken != 0:
		// Single asset withdrawal for specific LP tokens
		// Equation 8: ammAssetOut
		if lpTokensRequested == 0 {
			return TecAMM_INVALID_TOKENS
		}
		if lpTokensRequested > lpTokensHeld || lpTokensRequested > lptBalance {
			return TecAMM_INVALID_TOKENS
		}
		withdrawAmount1 = ammAssetOut(assetBalance1, lptBalance, lpTokensRequested, tfee)
		if withdrawAmount1 > assetBalance1 {
			return TecAMM_BALANCE
		}
		// Check minimum amount if specified
		if amount1 > 0 && withdrawAmount1 < amount1 {
			return TecAMM_FAILED
		}
		lpTokensToRedeem = lpTokensRequested

	case flags&tfLimitLPToken != 0:
		// Single asset withdrawal with effective price limit
		if amount1 == 0 || tx.EPrice == nil {
			return TemMALFORMED
		}
		ePrice := parseAmount(tx.EPrice.Value)
		if ePrice == 0 {
			return TemMALFORMED
		}
		// Calculate LP tokens based on effective price
		// EP = lpTokens / amount => lpTokens = EP * amount
		// Use equation that solves for lpTokens given EP constraint
		lpTokensToRedeem = calcLPTokensIn(assetBalance1, amount1, lptBalance, tfee)
		if lpTokensToRedeem == 0 || lpTokensToRedeem > lpTokensHeld {
			return TecAMM_INVALID_TOKENS
		}
		// Check effective price: EP = lpTokens / amount
		actualEP := lpTokensToRedeem / amount1
		if actualEP > ePrice {
			return TecAMM_FAILED
		}
		withdrawAmount1 = amount1

	default:
		return TemMALFORMED
	}

	if lpTokensToRedeem == 0 {
		return TecAMM_INVALID_TOKENS
	}

	// Verify withdrawal doesn't exceed balances
	if withdrawAmount1 > assetBalance1 {
		return TecAMM_BALANCE
	}
	if withdrawAmount2 > assetBalance2 {
		return TecAMM_BALANCE
	}

	// Transfer assets from AMM to withdrawer
	isXRP1 := tx.Asset.Currency == "" || tx.Asset.Currency == "XRP"
	isXRP2 := tx.Asset2.Currency == "" || tx.Asset2.Currency == "XRP"

	if isXRP1 && withdrawAmount1 > 0 {
		ammAccount.Balance -= withdrawAmount1
		account.Balance += withdrawAmount1
	}
	if isXRP2 && withdrawAmount2 > 0 {
		ammAccount.Balance -= withdrawAmount2
		account.Balance += withdrawAmount2
	}

	// Redeem LP tokens
	newLPBalance := lptBalance - lpTokensToRedeem
	amm.LPTokenBalance = newLPBalance

	// Check if AMM should be deleted (empty)
	ammDeleted := false
	if newLPBalance == 0 {
		// Delete AMM and AMM account
		if err := e.view.Erase(ammKey); err != nil {
			return TefINTERNAL
		}
		if err := e.view.Erase(ammAccountKey); err != nil {
			return TefINTERNAL
		}
		ammDeleted = true
	}

	if !ammDeleted {
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
	}

	// Persist updated withdrawer account
	accountKey := keylet.Account(accountID)
	accountBytes, err := serializeAccountRoot(account)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(accountKey, accountBytes); err != nil {
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
				"LPTokenBalance": fmt.Sprintf("%d", amm.LPTokenBalance),
			},
		})
	}

	return TesSUCCESS
}

// AMM vote constants
// Reference: rippled AMMCore.h
const (
	voteMaxSlots             = 8      // Maximum vote slots
	voteWeightScaleFactor    = 100000 // Scale factor for vote weight
	auctionSlotDiscountedFee = 10     // Discounted fee fraction (tradingFee / 10)
)

// applyAMMVote applies an AMMVote transaction
// Reference: rippled AMMVote.cpp applyVote
func (e *Engine) applyAMMVote(tx *AMMVote, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Find the AMM
	ammKey := computeAMMKeylet(tx.Asset, tx.Asset2)

	ammData, err := e.view.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	// Parse AMM data
	amm, err := parseAMMData(ammData)
	if err != nil {
		return TefINTERNAL
	}

	lptAMMBalance := amm.LPTokenBalance
	if lptAMMBalance == 0 {
		return TecAMM_BALANCE // AMM empty
	}

	// Get voter's LP token balance (simplified - in full implementation would read from trustline)
	// For now, assume voter has tokens proportional to their vote weight
	lpTokensNew := uint64(1000000) // Placeholder - in production would read from trustline

	feeNew := tx.TradingFee

	// Track minimum token holder for potential replacement
	var minTokens uint64 = math.MaxUint64
	var minPos int = -1
	var minAccount [20]byte
	var minFee uint16

	// Build updated vote slots
	updatedVoteSlots := make([]VoteSlotData, 0, voteMaxSlots)
	foundAccount := false

	// Running totals for weighted fee calculation
	var numerator uint64 = 0
	var denominator uint64 = 0

	// Iterate over current vote entries
	for i, slot := range amm.VoteSlots {
		lpTokens := uint64(slot.VoteWeight) * lptAMMBalance / voteWeightScaleFactor
		if lpTokens == 0 {
			// Skip entries with no tokens
			continue
		}

		feeVal := slot.TradingFee

		// Check if this is the voting account
		if slot.Account == accountID {
			lpTokens = lpTokensNew
			feeVal = feeNew
			foundAccount = true
		}

		// Calculate new vote weight
		voteWeight := lpTokens * voteWeightScaleFactor / lptAMMBalance

		// Update running totals for weighted fee
		numerator += uint64(feeVal) * lpTokens
		denominator += lpTokens

		// Track minimum for potential replacement
		if lpTokens < minTokens ||
			(lpTokens == minTokens && feeVal < minFee) ||
			(lpTokens == minTokens && feeVal == minFee && compareAccountIDs(slot.Account, minAccount) < 0) {
			minTokens = lpTokens
			minPos = i
			minAccount = slot.Account
			minFee = feeVal
		}

		updatedVoteSlots = append(updatedVoteSlots, VoteSlotData{
			Account:    slot.Account,
			TradingFee: feeVal,
			VoteWeight: uint32(voteWeight),
		})
	}

	// If account doesn't have a vote entry yet
	if !foundAccount {
		voteWeight := lpTokensNew * voteWeightScaleFactor / lptAMMBalance

		if len(updatedVoteSlots) < voteMaxSlots {
			// Add new entry if slots available
			updatedVoteSlots = append(updatedVoteSlots, VoteSlotData{
				Account:    accountID,
				TradingFee: feeNew,
				VoteWeight: uint32(voteWeight),
			})
			numerator += uint64(feeNew) * lpTokensNew
			denominator += lpTokensNew
		} else if lpTokensNew > minTokens || (lpTokensNew == minTokens && feeNew > minFee) {
			// Replace minimum token holder if new account has more tokens
			if minPos >= 0 && minPos < len(updatedVoteSlots) {
				// Remove min holder's contribution from totals
				numerator -= uint64(minFee) * minTokens
				denominator -= minTokens

				// Replace with new voter
				updatedVoteSlots[minPos] = VoteSlotData{
					Account:    accountID,
					TradingFee: feeNew,
					VoteWeight: uint32(voteWeight),
				}

				// Add new voter's contribution
				numerator += uint64(feeNew) * lpTokensNew
				denominator += lpTokensNew
			}
		}
		// else: all slots full and account doesn't have more tokens - vote not recorded
	}

	// Calculate weighted average trading fee
	var newTradingFee uint16 = 0
	if denominator > 0 {
		newTradingFee = uint16(numerator / denominator)
	}

	// Update AMM data
	amm.VoteSlots = updatedVoteSlots
	amm.TradingFee = newTradingFee

	// Update discounted fee in auction slot
	if amm.AuctionSlot != nil {
		discountedFee := newTradingFee / auctionSlotDiscountedFee
		// Discounted fee would be stored in auction slot
		_ = discountedFee
	}

	// Persist updated AMM
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

// applyAMMBid applies an AMMBid transaction
// Reference: rippled AMMBid.cpp applyBid
func (e *Engine) applyAMMBid(tx *AMMBid, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Find the AMM
	ammKey := computeAMMKeylet(tx.Asset, tx.Asset2)

	ammData, err := e.view.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	// Parse AMM data
	amm, err := parseAMMData(ammData)
	if err != nil {
		return TefINTERNAL
	}

	lptAMMBalance := amm.LPTokenBalance
	if lptAMMBalance == 0 {
		return TecAMM_BALANCE // AMM empty
	}

	// Get bidder's LP token balance (simplified)
	lpTokens := uint64(1000000) // Placeholder - would read from trustline

	// Parse bid amounts
	var bidMin, bidMax uint64
	if tx.BidMin != nil {
		bidMin = parseAmount(tx.BidMin.Value)
		if bidMin > lpTokens || bidMin >= lptAMMBalance {
			return TecAMM_INVALID_TOKENS
		}
	}
	if tx.BidMax != nil {
		bidMax = parseAmount(tx.BidMax.Value)
		if bidMax > lpTokens || bidMax >= lptAMMBalance {
			return TecAMM_INVALID_TOKENS
		}
	}
	if bidMin > 0 && bidMax > 0 && bidMin > bidMax {
		return TecAMM_INVALID_TOKENS
	}

	// Calculate trading fee fraction
	tradingFee := getFee(amm.TradingFee)

	// Minimum slot price = lptAMMBalance * tradingFee / 25
	minSlotPrice := float64(lptAMMBalance) * tradingFee / float64(auctionSlotMinFeeFraction)

	// Calculate discounted fee
	discountedFee := amm.TradingFee / uint16(auctionSlotDiscountedFee)

	// Get current time (simplified - would use ledger close time)
	currentTime := uint32(0) // Would be ctx.view.info().parentCloseTime

	// Initialize auction slot if needed
	if amm.AuctionSlot == nil {
		amm.AuctionSlot = &AuctionSlotData{
			AuthAccounts: make([][20]byte, 0),
		}
	}

	// Calculate time slot (0-19)
	var timeSlot *int
	if amm.AuctionSlot.Expiration > 0 && currentTime < amm.AuctionSlot.Expiration {
		elapsed := amm.AuctionSlot.Expiration - auctionSlotTotalTimeSecs
		if currentTime >= elapsed {
			slot := int((currentTime - elapsed) / auctionSlotIntervalDuration)
			if slot >= 0 && slot < auctionSlotTimeIntervals {
				timeSlot = &slot
			}
		}
	}

	// Check if current owner is valid
	validOwner := false
	if timeSlot != nil && *timeSlot < auctionSlotTimeIntervals-1 {
		// Check if owner account exists
		var zeroAccount [20]byte
		if amm.AuctionSlot.Account != zeroAccount {
			ownerKey := keylet.Account(amm.AuctionSlot.Account)
			exists, _ := e.view.Exists(ownerKey)
			validOwner = exists
		}
	}

	// Calculate pay price based on slot state
	var computedPrice float64
	var fractionRemaining float64 = 0.0
	pricePurchased := float64(amm.AuctionSlot.Price)

	if !validOwner || timeSlot == nil {
		// Slot is unowned or expired - pay minimum price
		computedPrice = minSlotPrice
	} else {
		// Slot is owned - calculate price based on time interval
		fractionUsed := (float64(*timeSlot) + 1) / float64(auctionSlotTimeIntervals)
		fractionRemaining = 1.0 - fractionUsed

		if *timeSlot == 0 {
			// First interval: price = pricePurchased * 1.05 + minSlotPrice
			computedPrice = pricePurchased*1.05 + minSlotPrice
		} else {
			// Other intervals: price = pricePurchased * 1.05 * (1 - fractionUsed^60) + minSlotPrice
			computedPrice = pricePurchased*1.05*(1-math.Pow(fractionUsed, 60)) + minSlotPrice
		}
	}

	// Determine actual pay price based on bidMin/bidMax
	var payPrice float64
	if bidMin > 0 && bidMax > 0 {
		// Both min/max specified
		if computedPrice <= float64(bidMax) {
			payPrice = math.Max(computedPrice, float64(bidMin))
		} else {
			return TecAMM_FAILED
		}
	} else if bidMin > 0 {
		// Only min specified
		payPrice = math.Max(computedPrice, float64(bidMin))
	} else if bidMax > 0 {
		// Only max specified
		if computedPrice <= float64(bidMax) {
			payPrice = computedPrice
		} else {
			return TecAMM_FAILED
		}
	} else {
		// Neither specified - pay computed price
		payPrice = computedPrice
	}

	// Check bidder has enough tokens
	if uint64(payPrice) > lpTokens {
		return TecAMM_INVALID_TOKENS
	}

	// Calculate refund and burn amounts
	var refund float64 = 0.0
	var burn float64 = payPrice

	if validOwner && timeSlot != nil {
		// Refund previous owner
		refund = fractionRemaining * pricePurchased
		if refund > payPrice {
			return TefINTERNAL // Should not happen
		}
		burn = payPrice - refund

		// Transfer refund to previous owner
		// In full implementation, would use accountSend
		_ = refund
	}

	// Burn tokens (reduce LP balance)
	burnAmount := uint64(burn)
	if burnAmount >= lptAMMBalance {
		return TefINTERNAL
	}
	amm.LPTokenBalance -= burnAmount

	// Update auction slot
	amm.AuctionSlot.Account = accountID
	amm.AuctionSlot.Expiration = currentTime + auctionSlotTotalTimeSecs
	amm.AuctionSlot.Price = uint64(payPrice)

	// Parse auth accounts if provided
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

	// Set discounted fee
	_ = discountedFee // Would be stored in auction slot

	// Persist updated AMM
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
			"LPTokenBalance": fmt.Sprintf("%d", amm.LPTokenBalance),
			"AuctionSlot": map[string]any{
				"Account":    tx.Account,
				"Expiration": amm.AuctionSlot.Expiration,
				"Price":      amm.AuctionSlot.Price,
			},
		},
	})

	return TesSUCCESS
}

// applyAMMDelete applies an AMMDelete transaction
func (e *Engine) applyAMMDelete(tx *AMMDelete, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammKey := computeAMMKeylet(tx.Asset, tx.Asset2)

	exists, _ := e.view.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Delete the AMM (only works if empty)
	if err := e.view.Erase(ammKey); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
	})

	return TesSUCCESS
}

// applyAMMClawback applies an AMMClawback transaction
// Reference: rippled AMMClawback.cpp applyGuts
func (e *Engine) applyAMMClawback(tx *AMMClawback, account *AccountRoot, metadata *Metadata) Result {
	issuerID, _ := decodeAccountID(tx.Account)

	// Verify issuer has lsfAllowTrustLineClawback and NOT lsfNoFreeze
	// Reference: rippled AMMClawback.cpp preclaim
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

	// Parse AMM data
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

	// Get current AMM balances
	assetBalance1 := ammAccount.Balance // For XRP in asset1
	assetBalance2 := uint64(0)          // Would come from trustline for IOU
	lptAMMBalance := amm.LPTokenBalance

	if lptAMMBalance == 0 {
		return TecAMM_BALANCE // AMM is empty
	}

	// Get holder's LP token balance
	// In full implementation, would read from LP token trustline
	// For now, use a portion of the AMM LP token balance as holder's balance
	holdLPTokens := lptAMMBalance / 2 // Simplified - would read from trustline

	if holdLPTokens == 0 {
		return TecAMM_BALANCE // Holder has no LP tokens
	}

	flags := tx.GetFlags()

	var lpTokensToWithdraw uint64
	var withdrawAmount1, withdrawAmount2 uint64

	if tx.Amount == nil {
		// No amount specified - withdraw all LP tokens the holder has
		// This is a proportional two-asset withdrawal
		lpTokensToWithdraw = holdLPTokens

		// Calculate proportional withdrawal amounts
		frac := float64(holdLPTokens) / float64(lptAMMBalance)
		withdrawAmount1 = uint64(float64(assetBalance1) * frac)
		withdrawAmount2 = uint64(float64(assetBalance2) * frac)
	} else {
		// Amount specified - calculate proportional withdrawal based on specified amount
		clawAmount := parseAmount(tx.Amount.Value)

		// Calculate fraction based on the clawback amount relative to asset1 balance
		if assetBalance1 == 0 {
			return TecAMM_BALANCE
		}
		frac := float64(clawAmount) / float64(assetBalance1)

		// Calculate LP tokens needed for this withdrawal
		lpTokensNeeded := uint64(float64(lptAMMBalance) * frac)

		// If holder doesn't have enough LP tokens, clawback all they have
		if lpTokensNeeded > holdLPTokens {
			lpTokensToWithdraw = holdLPTokens
			frac = float64(holdLPTokens) / float64(lptAMMBalance)
			withdrawAmount1 = uint64(float64(assetBalance1) * frac)
			withdrawAmount2 = uint64(float64(assetBalance2) * frac)
		} else {
			lpTokensToWithdraw = lpTokensNeeded
			withdrawAmount1 = clawAmount
			withdrawAmount2 = uint64(float64(assetBalance2) * frac)
		}
	}

	// Verify withdrawal amounts don't exceed balances
	if withdrawAmount1 > assetBalance1 {
		withdrawAmount1 = assetBalance1
	}
	if withdrawAmount2 > assetBalance2 {
		withdrawAmount2 = assetBalance2
	}

	// Perform the withdrawal from AMM
	isXRP1 := tx.Asset.Currency == "" || tx.Asset.Currency == "XRP"
	isXRP2 := tx.Asset2.Currency == "" || tx.Asset2.Currency == "XRP"

	// Transfer asset1 from AMM to holder (intermediate step)
	if isXRP1 && withdrawAmount1 > 0 {
		ammAccount.Balance -= withdrawAmount1
	}
	// Transfer asset2 from AMM to holder (intermediate step)
	if isXRP2 && withdrawAmount2 > 0 {
		ammAccount.Balance -= withdrawAmount2
	}

	// Now claw back: transfer asset1 from holder to issuer
	// For XRP, this is a balance transfer (though clawback is typically for IOUs)
	// In rippled, this uses rippleCredit to transfer the IOU balance
	if isXRP1 && withdrawAmount1 > 0 {
		// XRP clawback to issuer - add to issuer balance
		account.Balance += withdrawAmount1
	}

	// If tfClawTwoAssets is set, also claw back asset2
	if flags&tfClawTwoAssets != 0 {
		if isXRP2 && withdrawAmount2 > 0 {
			account.Balance += withdrawAmount2
		}
	} else {
		// Asset2 goes to holder (not clawed back)
		if isXRP2 && withdrawAmount2 > 0 {
			holderAccount.Balance += withdrawAmount2
		}
	}

	// Reduce LP token balance
	newLPBalance := lptAMMBalance - lpTokensToWithdraw
	amm.LPTokenBalance = newLPBalance

	// Check if AMM should be deleted (empty)
	ammDeleted := false
	if newLPBalance == 0 {
		// Delete AMM and AMM account
		if err := e.view.Erase(ammKey); err != nil {
			return TefINTERNAL
		}
		if err := e.view.Erase(ammAccountKey); err != nil {
			return TefINTERNAL
		}
		ammDeleted = true
	}

	if !ammDeleted {
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
	}

	// Persist updated issuer account
	accountKey := keylet.Account(issuerID)
	accountBytes, err := serializeAccountRoot(account)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(accountKey, accountBytes); err != nil {
		return TefINTERNAL
	}

	// Persist updated holder account
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
				"LPTokenBalance": fmt.Sprintf("%d", amm.LPTokenBalance),
			},
		})
	}

	// Record holder account modification
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(holderKey.Key[:])),
	})

	return TesSUCCESS
}

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
