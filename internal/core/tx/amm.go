package tx

import (
	"errors"
	"math"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

func init() {
	Register(TypeAMMCreate, func() Transaction {
		return &AMMCreate{BaseTx: *NewBaseTx(TypeAMMCreate, "")}
	})
	Register(TypeAMMDeposit, func() Transaction {
		return &AMMDeposit{BaseTx: *NewBaseTx(TypeAMMDeposit, "")}
	})
	Register(TypeAMMWithdraw, func() Transaction {
		return &AMMWithdraw{BaseTx: *NewBaseTx(TypeAMMWithdraw, "")}
	})
	Register(TypeAMMVote, func() Transaction {
		return &AMMVote{BaseTx: *NewBaseTx(TypeAMMVote, "")}
	})
	Register(TypeAMMBid, func() Transaction {
		return &AMMBid{BaseTx: *NewBaseTx(TypeAMMBid, "")}
	})
	Register(TypeAMMDelete, func() Transaction {
		return &AMMDelete{BaseTx: *NewBaseTx(TypeAMMDelete, "")}
	})
	Register(TypeAMMClawback, func() Transaction {
		return &AMMClawback{BaseTx: *NewBaseTx(TypeAMMClawback, "")}
	})
}

// AMM constants matching rippled
const (
	// TRADING_FEE_THRESHOLD is the maximum trading fee (1000 = 1%)
	TRADING_FEE_THRESHOLD uint16 = 1000

	// AMM vote slot constants
	VOTE_MAX_SLOTS         = 8
	VOTE_WEIGHT_SCALE_FACTOR = 100000

	// AMM auction slot constants
	AUCTION_SLOT_MAX_AUTH_ACCOUNTS           = 4
	AUCTION_SLOT_TIME_INTERVALS              = 20
	AUCTION_SLOT_DISCOUNTED_FEE_FRACTION     = 10 // 1/10 of fee
	AUCTION_SLOT_MIN_FEE_FRACTION            = 25 // 1/25 of fee
	TOTAL_TIME_SLOT_SECS                     = 24 * 60 * 60 // 24 hours

	// AMMCreate has no valid transaction flags
	tfAMMCreateMask uint32 = 0xFFFFFFFF

	// AMMDeposit flags
	tfLPToken          uint32 = 0x00010000
	tfSingleAsset      uint32 = 0x00080000
	tfTwoAsset         uint32 = 0x00100000
	tfOneAssetLPToken  uint32 = 0x00200000
	tfLimitLPToken     uint32 = 0x00400000
	tfTwoAssetIfEmpty  uint32 = 0x00800000
	tfAMMDepositMask   uint32 = ^(tfLPToken | tfSingleAsset | tfTwoAsset | tfOneAssetLPToken | tfLimitLPToken | tfTwoAssetIfEmpty)

	// AMMWithdraw flags
	tfWithdrawAll          uint32 = 0x00020000
	tfOneAssetWithdrawAll  uint32 = 0x00040000
	tfAMMWithdrawMask      uint32 = ^(tfLPToken | tfWithdrawAll | tfOneAssetWithdrawAll | tfSingleAsset | tfTwoAsset | tfOneAssetLPToken | tfLimitLPToken)

	// AMMVote has no valid transaction flags
	tfAMMVoteMask uint32 = 0xFFFFFFFF

	// AMMBid has no valid transaction flags
	tfAMMBidMask uint32 = 0xFFFFFFFF

	// AMMDelete has no valid transaction flags
	tfAMMDeleteMask uint32 = 0xFFFFFFFF

	// AMMClawback flags
	tfClawTwoAssets    uint32 = 0x00000001
	tfAMMClawbackMask  uint32 = ^tfClawTwoAssets
)

// AMMCreate creates an Automated Market Maker (AMM) instance.
type AMMCreate struct {
	BaseTx

	// Amount is the first asset to deposit (required)
	Amount Amount `json:"Amount" xrpl:"Amount,amount"`

	// Amount2 is the second asset to deposit (required)
	Amount2 Amount `json:"Amount2" xrpl:"Amount2,amount"`

	// TradingFee is the fee in basis points (0-1000, where 1000 = 1%)
	TradingFee uint16 `json:"TradingFee" xrpl:"TradingFee"`
}

// NewAMMCreate creates a new AMMCreate transaction
func NewAMMCreate(account string, amount1, amount2 Amount, tradingFee uint16) *AMMCreate {
	return &AMMCreate{
		BaseTx:     *NewBaseTx(TypeAMMCreate, account),
		Amount:     amount1,
		Amount2:    amount2,
		TradingFee: tradingFee,
	}
}

// TxType returns the transaction type
func (a *AMMCreate) TxType() Type {
	return TypeAMMCreate
}

// Validate validates the AMMCreate transaction
// Reference: rippled AMMCreate.cpp preflight
func (a *AMMCreate) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags - no flags are valid for AMMCreate
	if a.GetFlags()&tfAMMCreateMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMCreate")
	}

	// Amount is required and must be positive
	if a.Amount.Value == "" {
		return errors.New("temMALFORMED: Amount is required")
	}

	// Amount2 is required and must be positive
	if a.Amount2.Value == "" {
		return errors.New("temMALFORMED: Amount2 is required")
	}

	// Assets cannot be the same (same currency and issuer)
	if a.Amount.Currency == a.Amount2.Currency && a.Amount.Issuer == a.Amount2.Issuer {
		return errors.New("temBAD_AMM_TOKENS: tokens cannot have the same currency/issuer")
	}

	// TradingFee must be 0-1000 (0-1%)
	if a.TradingFee > TRADING_FEE_THRESHOLD {
		return errors.New("temBAD_FEE: TradingFee must be 0-1000")
	}

	// Validate amounts are positive (not zero or negative)
	if err := validateAMMAmount(a.Amount); err != nil {
		return errors.New("temBAD_AMOUNT: invalid Amount - " + err.Error())
	}
	if err := validateAMMAmount(a.Amount2); err != nil {
		return errors.New("temBAD_AMOUNT: invalid Amount2 - " + err.Error())
	}

	return nil
}

// validateAMMAmount validates an AMM amount
func validateAMMAmount(amt Amount) error {
	if amt.Value == "" {
		return errors.New("amount is required")
	}
	// For XRP (no currency), value must be positive drops
	if amt.Currency == "" {
		// XRP amount - should be positive integer drops
		if amt.Value == "0" {
			return errors.New("amount must be positive")
		}
		if len(amt.Value) > 0 && amt.Value[0] == '-' {
			return errors.New("amount must be positive")
		}
	}
	// For IOU, value must be positive
	// Note: Further IOU validation would check issuer existence, etc.
	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMCreate) Flatten() (map[string]any, error) {
	return ReflectFlatten(a)
}

// RequiredAmendments returns the amendments required for this transaction type
func (a *AMMCreate) RequiredAmendments() []string {
	return []string{AmendmentAMM, AmendmentFixUniversalNumber}
}

// Apply applies the AMMCreate transaction to ledger state.
// Reference: rippled AMMCreate.cpp doApply / applyCreate
func (a *AMMCreate) Apply(ctx *ApplyContext) Result {
	accountID := ctx.AccountID

	// Build assets for keylet computation
	asset1 := Asset{Currency: a.Amount.Currency, Issuer: a.Amount.Issuer}
	asset2 := Asset{Currency: a.Amount2.Currency, Issuer: a.Amount2.Issuer}

	// Compute the AMM keylet from the asset pair
	ammKey := computeAMMKeylet(asset1, asset2)

	// Check if AMM already exists
	exists, _ := ctx.View.Exists(ammKey)
	if exists {
		return TecDUPLICATE
	}

	// Compute the AMM account ID from keylet
	ammAccountID := computeAMMAccountID(ammKey.Key)
	ammAccountAddr, _ := encodeAccountID(ammAccountID)

	// Check if AMM account already exists (should not happen)
	ammAccountKey := keylet.Account(ammAccountID)
	acctExists, _ := ctx.View.Exists(ammAccountKey)
	if acctExists {
		return TecDUPLICATE
	}

	// Parse amounts
	amount1 := parseAmount(a.Amount.Value)
	amount2 := parseAmount(a.Amount2.Value)

	// Check creator has sufficient balance
	isXRP1 := a.Amount.Currency == "" || a.Amount.Currency == "XRP"
	isXRP2 := a.Amount2.Currency == "" || a.Amount2.Currency == "XRP"

	// For XRP amounts, verify balance
	totalXRPNeeded := uint64(0)
	if isXRP1 {
		totalXRPNeeded += amount1
	}
	if isXRP2 {
		totalXRPNeeded += amount2
	}
	if totalXRPNeeded > 0 && ctx.Account.Balance < totalXRPNeeded {
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
	lptCurrency := generateAMMLPTCurrency(a.Amount.Currency, a.Amount2.Currency)

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
		TradingFee:     a.TradingFee,
		LPTokenBalance: lpTokenBalance,
		VoteSlots:      make([]VoteSlotData, 0),
	}

	// Set asset currency bytes
	ammData.Asset = currencyToBytes(a.Amount.Currency)
	ammData.Asset2 = currencyToBytes(a.Amount2.Currency)

	// Initialize creator's vote slot with their LP token weight
	creatorVote := VoteSlotData{
		Account:    accountID,
		TradingFee: a.TradingFee,
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
	if err := ctx.View.Insert(ammAccountKey, ammAccountBytes); err != nil {
		return TefINTERNAL
	}

	// Store the AMM entry
	ammBytes, err := serializeAMM(ammData, accountID)
	if err != nil {
		return TefINTERNAL
	}
	if err := ctx.View.Insert(ammKey, ammBytes); err != nil {
		return TefINTERNAL
	}

	// Transfer XRP from creator to AMM account
	if isXRP1 {
		ctx.Account.Balance -= amount1
		ammAccount.Balance += amount1
	}
	if isXRP2 {
		ctx.Account.Balance -= amount2
		ammAccount.Balance += amount2
	}

	// For IOU transfers, update trustlines
	if !isXRP1 {
		if err := createOrUpdateAMMTrustline(ammAccountID, asset1, amount1, ctx.View); err != nil {
			return TecNO_LINE
		}
		// Debit from creator's trustline
		if err := updateTrustlineBalance(accountID, asset1, -int64(amount1)); err != nil {
			return TecUNFUNDED_AMM
		}
	}
	if !isXRP2 {
		if err := createOrUpdateAMMTrustline(ammAccountID, asset2, amount2, ctx.View); err != nil {
			return TecNO_LINE
		}
		if err := updateTrustlineBalance(accountID, asset2, -int64(amount2)); err != nil {
			return TecUNFUNDED_AMM
		}
	}

	// Create LP token trustline for creator
	lptAsset := Asset{
		Currency: lptCurrency,
		Issuer:   ammAccountAddr,
	}
	if err := createLPTokenTrustline(accountID, lptAsset, lpTokenBalance, ctx.View); err != nil {
		return TecINSUF_RESERVE_LINE
	}

	// Update creator account (owner count increases for LP token trustline)
	ctx.Account.OwnerCount++

	// Persist updated creator account
	accountKey := keylet.Account(accountID)
	accountBytes, err := serializeAccountRoot(ctx.Account)
	if err != nil {
		return TefINTERNAL
	}
	if err := ctx.View.Update(accountKey, accountBytes); err != nil {
		return TefINTERNAL
	}

	// Update AMM account balance (for XRP)
	ammAccountBytes, err = serializeAccountRoot(ammAccount)
	if err != nil {
		return TefINTERNAL
	}
	if err := ctx.View.Update(ammAccountKey, ammAccountBytes); err != nil {
		return TefINTERNAL
	}

	// Metadata for created AMM and AMM account tracked automatically by ApplyStateTable

	return TesSUCCESS
}

// AMMDeposit deposits assets into an AMM.
type AMMDeposit struct {
	BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset Asset `json:"Asset" xrpl:"Asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 Asset `json:"Asset2" xrpl:"Asset2"`

	// Amount is the amount of first asset to deposit (optional)
	Amount *Amount `json:"Amount,omitempty" xrpl:"Amount,omitempty,amount"`

	// Amount2 is the amount of second asset to deposit (optional)
	Amount2 *Amount `json:"Amount2,omitempty" xrpl:"Amount2,omitempty,amount"`

	// EPrice is the effective price limit (optional)
	EPrice *Amount `json:"EPrice,omitempty" xrpl:"EPrice,omitempty,amount"`

	// LPTokenOut is the LP tokens to receive (optional)
	LPTokenOut *Amount `json:"LPTokenOut,omitempty" xrpl:"LPTokenOut,omitempty,amount"`

	// TradingFee is the trading fee for tfTwoAssetIfEmpty mode (optional)
	// Only used when depositing into an empty AMM
	TradingFee uint16 `json:"TradingFee,omitempty" xrpl:"TradingFee,omitempty"`
}

// Asset identifies an asset in an AMM
type Asset struct {
	Currency string `json:"currency"`
	Issuer   string `json:"issuer,omitempty"`
}

// NewAMMDeposit creates a new AMMDeposit transaction
func NewAMMDeposit(account string, asset, asset2 Asset) *AMMDeposit {
	return &AMMDeposit{
		BaseTx: *NewBaseTx(TypeAMMDeposit, account),
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMDeposit) TxType() Type {
	return TypeAMMDeposit
}

// Validate validates the AMMDeposit transaction
// Reference: rippled AMMDeposit.cpp preflight
func (a *AMMDeposit) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags
	if a.GetFlags()&tfAMMDepositMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMDeposit")
	}

	if a.Asset.Currency == "" {
		return errors.New("temMALFORMED: Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("temMALFORMED: Asset2 is required")
	}

	// Validate flag combinations - must have exactly one deposit mode
	flags := a.GetFlags()
	flagCount := 0
	if flags&tfLPToken != 0 {
		flagCount++
	}
	if flags&tfSingleAsset != 0 {
		flagCount++
	}
	if flags&tfTwoAsset != 0 {
		flagCount++
	}
	if flags&tfOneAssetLPToken != 0 {
		flagCount++
	}
	if flags&tfLimitLPToken != 0 {
		flagCount++
	}
	if flags&tfTwoAssetIfEmpty != 0 {
		flagCount++
	}

	// At least one flag must be set (deposit mode)
	if flagCount == 0 {
		return errors.New("temMALFORMED: must specify deposit mode flag")
	}

	// Validate amounts if provided
	if a.Amount != nil {
		if err := validateAMMAmount(*a.Amount); err != nil {
			return errors.New("temBAD_AMOUNT: invalid Amount - " + err.Error())
		}
	}
	if a.Amount2 != nil {
		if err := validateAMMAmount(*a.Amount2); err != nil {
			return errors.New("temBAD_AMOUNT: invalid Amount2 - " + err.Error())
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMDeposit) Flatten() (map[string]any, error) {
	return ReflectFlatten(a)
}

// RequiredAmendments returns the amendments required for this transaction type
func (a *AMMDeposit) RequiredAmendments() []string {
	return []string{AmendmentAMM, AmendmentFixUniversalNumber}
}

// Apply applies the AMMDeposit transaction to ledger state.
// Reference: rippled AMMDeposit.cpp applyGuts
func (a *AMMDeposit) Apply(ctx *ApplyContext) Result {
	accountID := ctx.AccountID

	// Find the AMM
	ammKey := computeAMMKeylet(a.Asset, a.Asset2)

	ammRawData, err := ctx.View.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	// Parse AMM data
	amm, err := parseAMMData(ammRawData)
	if err != nil {
		return TefINTERNAL
	}

	// Get AMM account
	ammAccountID := computeAMMAccountID(ammKey.Key)
	ammAccountKey := keylet.Account(ammAccountID)
	ammAccountData, err := ctx.View.Read(ammAccountKey)
	if err != nil {
		return TefINTERNAL
	}
	ammAccount, err := parseAccountRoot(ammAccountData)
	if err != nil {
		return TefINTERNAL
	}

	flags := a.GetFlags()
	tfee := amm.TradingFee

	// Parse amounts
	var amount1, amount2, lpTokensRequested uint64
	if a.Amount != nil {
		amount1 = parseAmount(a.Amount.Value)
	}
	if a.Amount2 != nil {
		amount2 = parseAmount(a.Amount2.Value)
	}
	if a.LPTokenOut != nil {
		lpTokensRequested = parseAmount(a.LPTokenOut.Value)
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
		if a.EPrice != nil {
			ePrice := parseAmount(a.EPrice.Value)
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
		if a.TradingFee > 0 {
			amm.TradingFee = a.TradingFee
		}

	default:
		return TemMALFORMED
	}

	if lpTokensToIssue == 0 {
		return TecAMM_INVALID_TOKENS
	}

	// Check depositor has sufficient balance
	isXRP1 := a.Asset.Currency == "" || a.Asset.Currency == "XRP"
	isXRP2 := a.Asset2.Currency == "" || a.Asset2.Currency == "XRP"

	totalXRPNeeded := uint64(0)
	if isXRP1 && depositAmount1 > 0 {
		totalXRPNeeded += depositAmount1
	}
	if isXRP2 && depositAmount2 > 0 {
		totalXRPNeeded += depositAmount2
	}
	if totalXRPNeeded > 0 && ctx.Account.Balance < totalXRPNeeded {
		return TecUNFUNDED_AMM
	}

	// Transfer assets from depositor to AMM
	if isXRP1 && depositAmount1 > 0 {
		ctx.Account.Balance -= depositAmount1
		ammAccount.Balance += depositAmount1
	}
	if isXRP2 && depositAmount2 > 0 {
		ctx.Account.Balance -= depositAmount2
		ammAccount.Balance += depositAmount2
	}

	// Issue LP tokens to depositor
	amm.LPTokenBalance += lpTokensToIssue

	// Update LP token trustline for depositor
	ammAccountAddr, _ := encodeAccountID(ammAccountID)
	lptCurrency := generateAMMLPTCurrency(a.Asset.Currency, a.Asset2.Currency)
	lptAsset := Asset{Currency: lptCurrency, Issuer: ammAccountAddr}
	if err := createLPTokenTrustline(accountID, lptAsset, lpTokensToIssue, ctx.View); err != nil {
		return TecINSUF_RESERVE_LINE
	}

	// Persist updated AMM
	ammBytes, err := serializeAMMData(amm)
	if err != nil {
		return TefINTERNAL
	}
	if err := ctx.View.Update(ammKey, ammBytes); err != nil {
		return TefINTERNAL
	}

	// Persist updated AMM account
	ammAccountBytes, err := serializeAccountRoot(ammAccount)
	if err != nil {
		return TefINTERNAL
	}
	if err := ctx.View.Update(ammAccountKey, ammAccountBytes); err != nil {
		return TefINTERNAL
	}

	// Persist updated depositor account
	accountKey := keylet.Account(accountID)
	accountBytes, err := serializeAccountRoot(ctx.Account)
	if err != nil {
		return TefINTERNAL
	}
	// Update tracked automatically by ApplyStateTable
	if err := ctx.View.Update(accountKey, accountBytes); err != nil {
		return TefINTERNAL
	}

	return TesSUCCESS
}

// AMMWithdraw withdraws assets from an AMM.
type AMMWithdraw struct {
	BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset Asset `json:"Asset" xrpl:"Asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 Asset `json:"Asset2" xrpl:"Asset2"`

	// Amount is the amount of first asset to withdraw (optional)
	Amount *Amount `json:"Amount,omitempty" xrpl:"Amount,omitempty,amount"`

	// Amount2 is the amount of second asset to withdraw (optional)
	Amount2 *Amount `json:"Amount2,omitempty" xrpl:"Amount2,omitempty,amount"`

	// EPrice is the effective price limit (optional)
	EPrice *Amount `json:"EPrice,omitempty" xrpl:"EPrice,omitempty,amount"`

	// LPTokenIn is the LP tokens to burn (optional)
	LPTokenIn *Amount `json:"LPTokenIn,omitempty" xrpl:"LPTokenIn,omitempty,amount"`
}

// NewAMMWithdraw creates a new AMMWithdraw transaction
func NewAMMWithdraw(account string, asset, asset2 Asset) *AMMWithdraw {
	return &AMMWithdraw{
		BaseTx: *NewBaseTx(TypeAMMWithdraw, account),
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMWithdraw) TxType() Type {
	return TypeAMMWithdraw
}

// Validate validates the AMMWithdraw transaction
// Reference: rippled AMMWithdraw.cpp preflight
func (a *AMMWithdraw) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags
	if a.GetFlags()&tfAMMWithdrawMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMWithdraw")
	}

	if a.Asset.Currency == "" {
		return errors.New("temMALFORMED: Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("temMALFORMED: Asset2 is required")
	}

	flags := a.GetFlags()

	// Withdrawal sub-transaction flags (exactly one must be set)
	tfWithdrawSubTx := tfLPToken | tfWithdrawAll | tfOneAssetWithdrawAll | tfSingleAsset | tfTwoAsset | tfOneAssetLPToken | tfLimitLPToken
	subTxFlags := flags & tfWithdrawSubTx

	// Count number of mode flags set using popcount
	flagCount := 0
	for f := subTxFlags; f != 0; f &= f - 1 {
		flagCount++
	}
	if flagCount != 1 {
		return errors.New("temMALFORMED: exactly one withdraw mode flag must be set")
	}

	// Validate field requirements for each mode
	hasAmount := a.Amount != nil
	hasAmount2 := a.Amount2 != nil
	hasEPrice := a.EPrice != nil
	hasLPTokenIn := a.LPTokenIn != nil

	if flags&tfLPToken != 0 {
		// LPToken mode: LPTokenIn required, no amount/amount2/ePrice
		if !hasLPTokenIn || hasAmount || hasAmount2 || hasEPrice {
			return errors.New("temMALFORMED: tfLPToken requires LPTokenIn only")
		}
	} else if flags&tfWithdrawAll != 0 {
		// WithdrawAll mode: no fields needed
		if hasLPTokenIn || hasAmount || hasAmount2 || hasEPrice {
			return errors.New("temMALFORMED: tfWithdrawAll requires no amount fields")
		}
	} else if flags&tfOneAssetWithdrawAll != 0 {
		// OneAssetWithdrawAll mode: Amount required (identifies which asset)
		if !hasAmount || hasLPTokenIn || hasAmount2 || hasEPrice {
			return errors.New("temMALFORMED: tfOneAssetWithdrawAll requires Amount only")
		}
	} else if flags&tfSingleAsset != 0 {
		// SingleAsset mode: Amount required
		if !hasAmount || hasLPTokenIn || hasAmount2 || hasEPrice {
			return errors.New("temMALFORMED: tfSingleAsset requires Amount only")
		}
	} else if flags&tfTwoAsset != 0 {
		// TwoAsset mode: Amount and Amount2 required
		if !hasAmount || !hasAmount2 || hasLPTokenIn || hasEPrice {
			return errors.New("temMALFORMED: tfTwoAsset requires Amount and Amount2")
		}
	} else if flags&tfOneAssetLPToken != 0 {
		// OneAssetLPToken mode: Amount and LPTokenIn required
		if !hasAmount || !hasLPTokenIn || hasAmount2 || hasEPrice {
			return errors.New("temMALFORMED: tfOneAssetLPToken requires Amount and LPTokenIn")
		}
	} else if flags&tfLimitLPToken != 0 {
		// LimitLPToken mode: Amount and EPrice required
		if !hasAmount || !hasEPrice || hasLPTokenIn || hasAmount2 {
			return errors.New("temMALFORMED: tfLimitLPToken requires Amount and EPrice")
		}
	}

	// Amount and Amount2 cannot have the same issue if both present
	if hasAmount && hasAmount2 {
		if a.Amount.Currency == a.Amount2.Currency && a.Amount.Issuer == a.Amount2.Issuer {
			return errors.New("temBAD_AMM_TOKENS: Amount and Amount2 cannot have the same issue")
		}
	}

	// Validate LPTokenIn is positive
	if hasLPTokenIn {
		if err := validateAMMAmount(*a.LPTokenIn); err != nil {
			return errors.New("temBAD_AMM_TOKENS: invalid LPTokenIn - " + err.Error())
		}
	}

	// Validate amounts if provided
	if hasAmount {
		if err := validateAMMAmount(*a.Amount); err != nil {
			return errors.New("temBAD_AMOUNT: invalid Amount - " + err.Error())
		}
	}
	if hasAmount2 {
		if err := validateAMMAmount(*a.Amount2); err != nil {
			return errors.New("temBAD_AMOUNT: invalid Amount2 - " + err.Error())
		}
	}
	if hasEPrice {
		if err := validateAMMAmount(*a.EPrice); err != nil {
			return errors.New("temBAD_AMOUNT: invalid EPrice - " + err.Error())
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMWithdraw) Flatten() (map[string]any, error) {
	return ReflectFlatten(a)
}

// RequiredAmendments returns the amendments required for this transaction type
func (a *AMMWithdraw) RequiredAmendments() []string {
	return []string{AmendmentAMM, AmendmentFixUniversalNumber}
}

// Apply applies the AMMWithdraw transaction to ledger state.
// Reference: rippled AMMWithdraw.cpp applyGuts
func (a *AMMWithdraw) Apply(ctx *ApplyContext) Result {
	accountID := ctx.AccountID

	// Find the AMM
	ammKey := computeAMMKeylet(a.Asset, a.Asset2)

	ammRawData, err := ctx.View.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	// Parse AMM data
	amm, err := parseAMMData(ammRawData)
	if err != nil {
		return TefINTERNAL
	}

	// Get AMM account
	ammAccountID := computeAMMAccountID(ammKey.Key)
	ammAccountKey := keylet.Account(ammAccountID)
	ammAccountData, err := ctx.View.Read(ammAccountKey)
	if err != nil {
		return TefINTERNAL
	}
	ammAccount, err := parseAccountRoot(ammAccountData)
	if err != nil {
		return TefINTERNAL
	}

	flags := a.GetFlags()
	tfee := amm.TradingFee

	// Parse amounts
	var amount1, amount2, lpTokensRequested uint64
	if a.Amount != nil {
		amount1 = parseAmount(a.Amount.Value)
	}
	if a.Amount2 != nil {
		amount2 = parseAmount(a.Amount2.Value)
	}
	if a.LPTokenIn != nil {
		lpTokensRequested = parseAmount(a.LPTokenIn.Value)
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
		if amount1 == 0 || a.EPrice == nil {
			return TemMALFORMED
		}
		ePrice := parseAmount(a.EPrice.Value)
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
	isXRP1 := a.Asset.Currency == "" || a.Asset.Currency == "XRP"
	isXRP2 := a.Asset2.Currency == "" || a.Asset2.Currency == "XRP"

	if isXRP1 && withdrawAmount1 > 0 {
		ammAccount.Balance -= withdrawAmount1
		ctx.Account.Balance += withdrawAmount1
	}
	if isXRP2 && withdrawAmount2 > 0 {
		ammAccount.Balance -= withdrawAmount2
		ctx.Account.Balance += withdrawAmount2
	}

	// Redeem LP tokens
	newLPBalance := lptBalance - lpTokensToRedeem
	amm.LPTokenBalance = newLPBalance

	// Check if AMM should be deleted (empty)
	ammDeleted := false
	if newLPBalance == 0 {
		// Delete AMM and AMM account
		if err := ctx.View.Erase(ammKey); err != nil {
			return TefINTERNAL
		}
		if err := ctx.View.Erase(ammAccountKey); err != nil {
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
		if err := ctx.View.Update(ammKey, ammBytes); err != nil {
			return TefINTERNAL
		}

		// Persist updated AMM account
		ammAccountBytes, err := serializeAccountRoot(ammAccount)
		if err != nil {
			return TefINTERNAL
		}
		if err := ctx.View.Update(ammAccountKey, ammAccountBytes); err != nil {
			return TefINTERNAL
		}
	}

	// Persist updated withdrawer account
	accountKey := keylet.Account(accountID)
	accountBytes, err := serializeAccountRoot(ctx.Account)
	if err != nil {
		return TefINTERNAL
	}
	if err := ctx.View.Update(accountKey, accountBytes); err != nil {
		return TefINTERNAL
	}

	return TesSUCCESS
}

// AMMVote votes on the trading fee for an AMM.
type AMMVote struct {
	BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset Asset `json:"Asset" xrpl:"Asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 Asset `json:"Asset2" xrpl:"Asset2"`

	// TradingFee is the proposed fee in basis points (0-1000)
	TradingFee uint16 `json:"TradingFee" xrpl:"TradingFee"`
}

// NewAMMVote creates a new AMMVote transaction
func NewAMMVote(account string, asset, asset2 Asset, tradingFee uint16) *AMMVote {
	return &AMMVote{
		BaseTx:     *NewBaseTx(TypeAMMVote, account),
		Asset:      asset,
		Asset2:     asset2,
		TradingFee: tradingFee,
	}
}

// TxType returns the transaction type
func (a *AMMVote) TxType() Type {
	return TypeAMMVote
}

// Validate validates the AMMVote transaction
// Reference: rippled AMMVote.cpp preflight
func (a *AMMVote) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags - no flags are valid for AMMVote
	if a.GetFlags()&tfAMMVoteMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMVote")
	}

	// Validate asset pair
	if a.Asset.Currency == "" {
		return errors.New("temMALFORMED: Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("temMALFORMED: Asset2 is required")
	}

	// TradingFee must be within threshold
	if a.TradingFee > TRADING_FEE_THRESHOLD {
		return errors.New("temBAD_FEE: TradingFee must be 0-1000")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMVote) Flatten() (map[string]any, error) {
	return ReflectFlatten(a)
}

// RequiredAmendments returns the amendments required for this transaction type
func (a *AMMVote) RequiredAmendments() []string {
	return []string{AmendmentAMM, AmendmentFixUniversalNumber}
}

// Apply applies the AMMVote transaction to ledger state.
// Reference: rippled AMMVote.cpp applyVote
func (a *AMMVote) Apply(ctx *ApplyContext) Result {
	accountID := ctx.AccountID

	// Find the AMM
	ammKey := computeAMMKeylet(a.Asset, a.Asset2)

	ammRawData, err := ctx.View.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	// Parse AMM data
	amm, err := parseAMMData(ammRawData)
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

	feeNew := a.TradingFee

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

	// Persist updated AMM - update tracked automatically by ApplyStateTable
	ammBytes, err := serializeAMMData(amm)
	if err != nil {
		return TefINTERNAL
	}
	if err := ctx.View.Update(ammKey, ammBytes); err != nil {
		return TefINTERNAL
	}

	return TesSUCCESS
}

// AMMBid places a bid on an AMM auction slot.
type AMMBid struct {
	BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset Asset `json:"Asset" xrpl:"Asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 Asset `json:"Asset2" xrpl:"Asset2"`

	// BidMin is the minimum bid amount (optional)
	BidMin *Amount `json:"BidMin,omitempty" xrpl:"BidMin,omitempty,amount"`

	// BidMax is the maximum bid amount (optional)
	BidMax *Amount `json:"BidMax,omitempty" xrpl:"BidMax,omitempty,amount"`

	// AuthAccounts are accounts to authorize for discounted trading (optional)
	AuthAccounts []AuthAccount `json:"AuthAccounts,omitempty" xrpl:"AuthAccounts,omitempty"`
}

// AuthAccount is an authorized account for AMM slot trading
type AuthAccount struct {
	AuthAccount AuthAccountData `json:"AuthAccount"`
}

// AuthAccountData contains the account address
type AuthAccountData struct {
	Account string `json:"Account"`
}

// NewAMMBid creates a new AMMBid transaction
func NewAMMBid(account string, asset, asset2 Asset) *AMMBid {
	return &AMMBid{
		BaseTx: *NewBaseTx(TypeAMMBid, account),
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMBid) TxType() Type {
	return TypeAMMBid
}

// Validate validates the AMMBid transaction
// Reference: rippled AMMBid.cpp preflight
func (a *AMMBid) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags - no flags are valid for AMMBid
	if a.GetFlags()&tfAMMBidMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMBid")
	}

	// Validate asset pair
	if a.Asset.Currency == "" {
		return errors.New("temMALFORMED: Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("temMALFORMED: Asset2 is required")
	}

	// Validate BidMin if present
	if a.BidMin != nil {
		if err := validateAMMAmount(*a.BidMin); err != nil {
			return errors.New("temMALFORMED: invalid BidMin - " + err.Error())
		}
	}

	// Validate BidMax if present
	if a.BidMax != nil {
		if err := validateAMMAmount(*a.BidMax); err != nil {
			return errors.New("temMALFORMED: invalid BidMax - " + err.Error())
		}
	}

	// Max 4 auth accounts
	if len(a.AuthAccounts) > AUCTION_SLOT_MAX_AUTH_ACCOUNTS {
		return errors.New("temMALFORMED: cannot have more than 4 AuthAccounts")
	}

	// Check for duplicate auth accounts and self-authorization
	if len(a.AuthAccounts) > 0 {
		seen := make(map[string]bool)
		for _, authAcct := range a.AuthAccounts {
			acct := authAcct.AuthAccount.Account
			// Cannot authorize self
			if acct == a.Common.Account {
				return errors.New("temMALFORMED: cannot authorize self in AuthAccounts")
			}
			// Check for duplicates
			if seen[acct] {
				return errors.New("temMALFORMED: duplicate account in AuthAccounts")
			}
			seen[acct] = true
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMBid) Flatten() (map[string]any, error) {
	return ReflectFlatten(a)
}

// RequiredAmendments returns the amendments required for this transaction type
func (a *AMMBid) RequiredAmendments() []string {
	return []string{AmendmentAMM, AmendmentFixUniversalNumber}
}

// Apply applies the AMMBid transaction to ledger state.
// Reference: rippled AMMBid.cpp applyBid
func (a *AMMBid) Apply(ctx *ApplyContext) Result {
	accountID := ctx.AccountID

	// Find the AMM
	ammKey := computeAMMKeylet(a.Asset, a.Asset2)

	ammRawData, err := ctx.View.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	// Parse AMM data
	amm, err := parseAMMData(ammRawData)
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
	if a.BidMin != nil {
		bidMin = parseAmount(a.BidMin.Value)
		if bidMin > lpTokens || bidMin >= lptAMMBalance {
			return TecAMM_INVALID_TOKENS
		}
	}
	if a.BidMax != nil {
		bidMax = parseAmount(a.BidMax.Value)
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
	currentTime := uint32(0) // Would be ctx.View.info().parentCloseTime

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
			exists, _ := ctx.View.Exists(ownerKey)
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
	if a.AuthAccounts != nil {
		amm.AuctionSlot.AuthAccounts = make([][20]byte, 0, len(a.AuthAccounts))
		for _, authAccountEntry := range a.AuthAccounts {
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
	// Update tracked automatically by ApplyStateTable
	if err := ctx.View.Update(ammKey, ammBytes); err != nil {
		return TefINTERNAL
	}

	return TesSUCCESS
}

// AMMDelete deletes an empty AMM.
type AMMDelete struct {
	BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset Asset `json:"Asset" xrpl:"Asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 Asset `json:"Asset2" xrpl:"Asset2"`
}

// NewAMMDelete creates a new AMMDelete transaction
func NewAMMDelete(account string, asset, asset2 Asset) *AMMDelete {
	return &AMMDelete{
		BaseTx: *NewBaseTx(TypeAMMDelete, account),
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMDelete) TxType() Type {
	return TypeAMMDelete
}

// Validate validates the AMMDelete transaction
// Reference: rippled AMMDelete.cpp preflight
func (a *AMMDelete) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags - no flags are valid for AMMDelete
	if a.GetFlags()&tfAMMDeleteMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMDelete")
	}

	// Validate asset pair
	if a.Asset.Currency == "" {
		return errors.New("temMALFORMED: Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("temMALFORMED: Asset2 is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMDelete) Flatten() (map[string]any, error) {
	return ReflectFlatten(a)
}

// RequiredAmendments returns the amendments required for this transaction type
func (a *AMMDelete) RequiredAmendments() []string {
	return []string{AmendmentAMM, AmendmentFixUniversalNumber}
}

// Apply applies the AMMDelete transaction to ledger state.
func (a *AMMDelete) Apply(ctx *ApplyContext) Result {
	// Find the AMM
	ammKey := computeAMMKeylet(a.Asset, a.Asset2)

	exists, _ := ctx.View.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Delete the AMM (only works if empty) - deletion tracked automatically by ApplyStateTable
	if err := ctx.View.Erase(ammKey); err != nil {
		return TefINTERNAL
	}

	return TesSUCCESS
}

// AMMClawback claws back tokens from an AMM.
type AMMClawback struct {
	BaseTx

	// Holder is the account holding LP tokens (required)
	Holder string `json:"Holder" xrpl:"Holder"`

	// Asset identifies the first asset of the AMM (required)
	Asset Asset `json:"Asset" xrpl:"Asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 Asset `json:"Asset2" xrpl:"Asset2"`

	// Amount is the amount to claw back (optional)
	Amount *Amount `json:"Amount,omitempty" xrpl:"Amount,omitempty,amount"`
}

// NewAMMClawback creates a new AMMClawback transaction
func NewAMMClawback(account, holder string, asset, asset2 Asset) *AMMClawback {
	return &AMMClawback{
		BaseTx: *NewBaseTx(TypeAMMClawback, account),
		Holder: holder,
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMClawback) TxType() Type {
	return TypeAMMClawback
}

// Validate validates the AMMClawback transaction
// Reference: rippled AMMClawback.cpp preflight
func (a *AMMClawback) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags
	if a.GetFlags()&tfAMMClawbackMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMClawback")
	}

	// Holder is required
	if a.Holder == "" {
		return errors.New("temMALFORMED: Holder is required")
	}

	// Holder cannot be the same as issuer (Account)
	if a.Holder == a.Common.Account {
		return errors.New("temMALFORMED: Holder cannot be the same as issuer")
	}

	// Validate asset pair
	if a.Asset.Currency == "" {
		return errors.New("temMALFORMED: Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("temMALFORMED: Asset2 is required")
	}

	// Asset cannot be XRP (must be issued currency)
	if a.Asset.Currency == "XRP" || a.Asset.Currency == "" && a.Asset.Issuer == "" {
		return errors.New("temMALFORMED: Asset cannot be XRP")
	}

	// Asset issuer must match the transaction account (issuer)
	if a.Asset.Issuer != a.Common.Account {
		return errors.New("temMALFORMED: Asset issuer must match Account")
	}

	// If tfClawTwoAssets is set, both assets must be issued by the same issuer
	if a.GetFlags()&tfClawTwoAssets != 0 {
		if a.Asset.Issuer != a.Asset2.Issuer {
			return errors.New("temINVALID_FLAG: tfClawTwoAssets requires both assets to have the same issuer")
		}
	}

	// Validate Amount if provided
	if a.Amount != nil {
		// Amount must be positive
		if err := validateAMMAmount(*a.Amount); err != nil {
			return errors.New("temBAD_AMOUNT: invalid Amount - " + err.Error())
		}
		// Amount's issue must match Asset
		if a.Amount.Currency != a.Asset.Currency || a.Amount.Issuer != a.Asset.Issuer {
			return errors.New("temBAD_AMOUNT: Amount issue must match Asset")
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMClawback) Flatten() (map[string]any, error) {
	return ReflectFlatten(a)
}

// RequiredAmendments returns the amendments required for this transaction type
func (a *AMMClawback) RequiredAmendments() []string {
	return []string{AmendmentAMM, AmendmentFixUniversalNumber, AmendmentAMMClawback}
}

// Apply applies the AMMClawback transaction to ledger state.
// Reference: rippled AMMClawback.cpp applyGuts
func (a *AMMClawback) Apply(ctx *ApplyContext) Result {
	issuerID := ctx.AccountID

	// Verify issuer has lsfAllowTrustLineClawback and NOT lsfNoFreeze
	// Reference: rippled AMMClawback.cpp preclaim
	if (ctx.Account.Flags & lsfAllowTrustLineClawback) == 0 {
		return TecNO_PERMISSION
	}
	if (ctx.Account.Flags & lsfNoFreeze) != 0 {
		return TecNO_PERMISSION
	}

	// Find the holder
	holderID, err := decodeAccountID(a.Holder)
	if err != nil {
		return TemINVALID
	}

	holderKey := keylet.Account(holderID)
	holderData, err := ctx.View.Read(holderKey)
	if err != nil {
		return TerNO_ACCOUNT
	}
	holderAccount, err := parseAccountRoot(holderData)
	if err != nil {
		return TefINTERNAL
	}

	// Find the AMM
	ammKey := computeAMMKeylet(a.Asset, a.Asset2)
	ammRawData, err := ctx.View.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	// Parse AMM data
	amm, err := parseAMMData(ammRawData)
	if err != nil {
		return TefINTERNAL
	}

	// Get AMM account
	ammAccountID := computeAMMAccountID(ammKey.Key)
	ammAccountKey := keylet.Account(ammAccountID)
	ammAccountData, err := ctx.View.Read(ammAccountKey)
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

	flags := a.GetFlags()

	var lpTokensToWithdraw uint64
	var withdrawAmount1, withdrawAmount2 uint64

	if a.Amount == nil {
		// No amount specified - withdraw all LP tokens the holder has
		// This is a proportional two-asset withdrawal
		lpTokensToWithdraw = holdLPTokens

		// Calculate proportional withdrawal amounts
		frac := float64(holdLPTokens) / float64(lptAMMBalance)
		withdrawAmount1 = uint64(float64(assetBalance1) * frac)
		withdrawAmount2 = uint64(float64(assetBalance2) * frac)
	} else {
		// Amount specified - calculate proportional withdrawal based on specified amount
		clawAmount := parseAmount(a.Amount.Value)

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
	isXRP1 := a.Asset.Currency == "" || a.Asset.Currency == "XRP"
	isXRP2 := a.Asset2.Currency == "" || a.Asset2.Currency == "XRP"

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
		ctx.Account.Balance += withdrawAmount1
	}

	// If tfClawTwoAssets is set, also claw back asset2
	if flags&tfClawTwoAssets != 0 {
		if isXRP2 && withdrawAmount2 > 0 {
			ctx.Account.Balance += withdrawAmount2
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
		if err := ctx.View.Erase(ammKey); err != nil {
			return TefINTERNAL
		}
		if err := ctx.View.Erase(ammAccountKey); err != nil {
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
		if err := ctx.View.Update(ammKey, ammBytes); err != nil {
			return TefINTERNAL
		}

		// Persist updated AMM account
		ammAccountBytes, err := serializeAccountRoot(ammAccount)
		if err != nil {
			return TefINTERNAL
		}
		if err := ctx.View.Update(ammAccountKey, ammAccountBytes); err != nil {
			return TefINTERNAL
		}
	}

	// Persist updated issuer account
	accountKey := keylet.Account(issuerID)
	accountBytes, err := serializeAccountRoot(ctx.Account)
	if err != nil {
		return TefINTERNAL
	}
	if err := ctx.View.Update(accountKey, accountBytes); err != nil {
		return TefINTERNAL
	}

	// Persist updated holder account
	holderBytes, err := serializeAccountRoot(holderAccount)
	if err != nil {
		return TefINTERNAL
	}
	if err := ctx.View.Update(holderKey, holderBytes); err != nil {
		return TefINTERNAL
	}

	return TesSUCCESS
}
