package amm

import (
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeAMMDeposit, func() tx.Transaction {
		return &AMMDeposit{BaseTx: *tx.NewBaseTx(tx.TypeAMMDeposit, "")}
	})
}

// AMMDeposit deposits assets into an AMM.
type AMMDeposit struct {
	tx.BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset tx.Asset `json:"Asset" xrpl:"Asset,asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 tx.Asset `json:"Asset2" xrpl:"Asset2,asset"`

	// Amount is the amount of first asset to deposit (optional)
	Amount *tx.Amount `json:"Amount,omitempty" xrpl:"Amount,omitempty,amount"`

	// Amount2 is the amount of second asset to deposit (optional)
	Amount2 *tx.Amount `json:"Amount2,omitempty" xrpl:"Amount2,omitempty,amount"`

	// EPrice is the effective price limit (optional)
	EPrice *tx.Amount `json:"EPrice,omitempty" xrpl:"EPrice,omitempty,amount"`

	// LPTokenOut is the LP tokens to receive (optional)
	LPTokenOut *tx.Amount `json:"LPTokenOut,omitempty" xrpl:"LPTokenOut,omitempty,amount"`

	// TradingFee is the trading fee for tfTwoAssetIfEmpty mode (optional)
	// Only used when depositing into an empty AMM
	TradingFee uint16 `json:"TradingFee,omitempty" xrpl:"TradingFee,omitempty"`
}

// NewAMMDeposit creates a new AMMDeposit transaction
func NewAMMDeposit(account string, asset, asset2 tx.Asset) *AMMDeposit {
	return &AMMDeposit{
		BaseTx: *tx.NewBaseTx(tx.TypeAMMDeposit, account),
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMDeposit) TxType() tx.Type {
	return tx.TypeAMMDeposit
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
	// Per rippled: validZero is true only when EPrice is present
	hasEPrice := a.EPrice != nil
	validZeroAmount := hasEPrice

	if a.Amount != nil {
		if errCode := validateAMMAmountWithPair(*a.Amount, &a.Asset, &a.Asset2, validZeroAmount); errCode != "" {
			if errCode == "temBAD_AMM_TOKENS" {
				return errors.New("temBAD_AMM_TOKENS: invalid Amount")
			}
			return errors.New("temBAD_AMOUNT: invalid Amount")
		}
	}
	if a.Amount2 != nil {
		if errCode := validateAMMAmountWithPair(*a.Amount2, &a.Asset, &a.Asset2, false); errCode != "" {
			if errCode == "temBAD_AMM_TOKENS" {
				return errors.New("temBAD_AMM_TOKENS: invalid Amount2")
			}
			return errors.New("temBAD_AMOUNT: invalid Amount2")
		}
	}

	// Validate LPTokenOut if provided - per rippled AMMDeposit.cpp lines 115-119
	// LP tokens must be positive (not zero or negative)
	if a.LPTokenOut != nil {
		if a.LPTokenOut.IsZero() || a.LPTokenOut.IsNegative() {
			return errors.New("temBAD_AMM_TOKENS: invalid LPTokens")
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMDeposit) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(a)
}

// RequiredAmendments returns the amendments required for this transaction type
func (a *AMMDeposit) RequiredAmendments() []string {
	return []string{amendment.AmendmentAMM, amendment.AmendmentFixUniversalNumber}
}

// Apply applies the AMMDeposit transaction to ledger state.
// Reference: rippled AMMDeposit.cpp applyGuts
func (a *AMMDeposit) Apply(ctx *tx.ApplyContext) tx.Result {
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
		return tx.TefINTERNAL
	}

	// Get AMM account
	ammAccountID := computeAMMAccountID(ammKey.Key)
	ammAccountKey := keylet.Account(ammAccountID)
	ammAccountData, err := ctx.View.Read(ammAccountKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	ammAccount, err := sle.ParseAccountRoot(ammAccountData)
	if err != nil {
		return tx.TefINTERNAL
	}

	flags := a.GetFlags()
	tfee := amm.TradingFee

	// Parse amounts
	var amount1, amount2, lpTokensRequested uint64
	if a.Amount != nil {
		amount1 = parseAmountFromTx(a.Amount)
	}
	if a.Amount2 != nil {
		amount2 = parseAmountFromTx(a.Amount2)
	}
	if a.LPTokenOut != nil {
		lpTokensRequested = parseAmountFromTx(a.LPTokenOut)
	}

	// Get current AMM balances from AMM data
	assetBalance1 := amm.AssetBalance
	assetBalance2 := amm.Asset2Balance
	lptBalance := amm.LPTokenBalance

	var lpTokensToIssue uint64
	var depositAmount1, depositAmount2 uint64

	// Handle different deposit modes
	switch {
	case flags&tfLPToken != 0:
		// Proportional deposit for specified LP tokens
		if lpTokensRequested == 0 || lptBalance == 0 {
			return tx.TecAMM_INVALID_TOKENS
		}
		frac := float64(lpTokensRequested) / float64(lptBalance)
		depositAmount1 = uint64(float64(assetBalance1) * frac)
		depositAmount2 = uint64(float64(assetBalance2) * frac)
		lpTokensToIssue = lpTokensRequested

	case flags&tfSingleAsset != 0:
		// Single asset deposit - determine which asset is being deposited
		// by comparing the Amount's currency/issuer with Asset and Asset2
		isDepositForAsset1 := a.Amount != nil && matchesAsset(a.Amount, a.Asset)
		isDepositForAsset2 := a.Amount != nil && matchesAsset(a.Amount, a.Asset2)

		if isDepositForAsset1 {
			lpTokensToIssue = lpTokensOut(assetBalance1, amount1, lptBalance, tfee)
			if lpTokensToIssue == 0 {
				return tx.TecAMM_INVALID_TOKENS
			}
			depositAmount1 = amount1
		} else if isDepositForAsset2 {
			lpTokensToIssue = lpTokensOut(assetBalance2, amount1, lptBalance, tfee)
			if lpTokensToIssue == 0 {
				return tx.TecAMM_INVALID_TOKENS
			}
			depositAmount2 = amount1 // amount1 is parsed from Amount field
		} else {
			// Amount currency doesn't match either AMM asset
			return tx.TecAMM_INVALID_TOKENS
		}

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
		isDepositForAsset1 := matchesAsset(a.Amount, a.Asset)
		isDepositForAsset2 := matchesAsset(a.Amount, a.Asset2)

		if isDepositForAsset1 {
			depositAmount1 = ammAssetIn(assetBalance1, lptBalance, lpTokensRequested, tfee)
			if depositAmount1 > amount1 {
				return tx.TecAMM_FAILED
			}
		} else if isDepositForAsset2 {
			depositAmount2 = ammAssetIn(assetBalance2, lptBalance, lpTokensRequested, tfee)
			if depositAmount2 > amount1 { // amount1 is the max from Amount field
				return tx.TecAMM_FAILED
			}
		} else {
			return tx.TecAMM_INVALID_TOKENS
		}
		lpTokensToIssue = lpTokensRequested

	case flags&tfLimitLPToken != 0:
		// Single asset deposit with effective price limit
		isDepositForAsset1 := matchesAsset(a.Amount, a.Asset)
		isDepositForAsset2 := matchesAsset(a.Amount, a.Asset2)

		var assetBalance uint64
		if isDepositForAsset1 {
			assetBalance = assetBalance1
		} else if isDepositForAsset2 {
			assetBalance = assetBalance2
		} else {
			return tx.TecAMM_INVALID_TOKENS
		}

		lpTokensToIssue = lpTokensOut(assetBalance, amount1, lptBalance, tfee)
		if lpTokensToIssue == 0 {
			return tx.TecAMM_INVALID_TOKENS
		}
		// Check effective price
		if a.EPrice != nil {
			ePrice := parseAmountFromTx(a.EPrice)
			if ePrice > 0 && amount1/lpTokensToIssue > ePrice {
				return tx.TecAMM_FAILED
			}
		}
		if isDepositForAsset1 {
			depositAmount1 = amount1
		} else {
			depositAmount2 = amount1
		}

	case flags&tfTwoAssetIfEmpty != 0:
		// Deposit into empty AMM
		if lptBalance != 0 {
			return tx.TecAMM_NOT_EMPTY
		}
		lpTokensToIssue = calculateLPTokens(amount1, amount2)
		depositAmount1 = amount1
		depositAmount2 = amount2
		// Set trading fee if provided
		if a.TradingFee > 0 {
			amm.TradingFee = a.TradingFee
		}

	default:
		return tx.TemMALFORMED
	}

	if lpTokensToIssue == 0 {
		return tx.TecAMM_INVALID_TOKENS
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

	// For IOU transfers, update trust lines
	if !isXRP1 && depositAmount1 > 0 {
		// Get issuer account ID
		issuerID, err := sle.DecodeAccountID(a.Asset.Issuer)
		if err != nil {
			return tx.TefINTERNAL
		}
		// Deduct from depositor's trust line (negative delta)
		if err := updateTrustlineBalanceInView(accountID, issuerID, a.Asset.Currency, -int64(depositAmount1), ctx.View); err != nil {
			// Trust line update failed - may not have sufficient balance
			return TecUNFUNDED_AMM
		}
	}
	if !isXRP2 && depositAmount2 > 0 {
		issuerID, err := sle.DecodeAccountID(a.Asset2.Issuer)
		if err != nil {
			return tx.TefINTERNAL
		}
		if err := updateTrustlineBalanceInView(accountID, issuerID, a.Asset2.Currency, -int64(depositAmount2), ctx.View); err != nil {
			return TecUNFUNDED_AMM
		}
	}

	// Issue LP tokens to depositor
	amm.LPTokenBalance += lpTokensToIssue

	// Update AMM asset balances
	amm.AssetBalance += depositAmount1
	amm.Asset2Balance += depositAmount2

	// Update LP token trustline for depositor
	ammAccountAddr, _ := encodeAccountID(ammAccountID)
	lptCurrency := generateAMMLPTCurrency(a.Asset.Currency, a.Asset2.Currency)
	lptAsset := tx.Asset{Currency: lptCurrency, Issuer: ammAccountAddr}
	if err := createLPTokenTrustline(accountID, lptAsset, lpTokensToIssue, ctx.View); err != nil {
		return TecINSUF_RESERVE_LINE
	}

	// Persist updated AMM
	ammBytes, err := serializeAMMData(amm)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Update(ammKey, ammBytes); err != nil {
		return tx.TefINTERNAL
	}

	// Persist updated AMM account
	ammAccountBytes, err := sle.SerializeAccountRoot(ammAccount)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Update(ammAccountKey, ammAccountBytes); err != nil {
		return tx.TefINTERNAL
	}

	// Persist updated depositor account
	accountKey := keylet.Account(accountID)
	accountBytes, err := sle.SerializeAccountRoot(ctx.Account)
	if err != nil {
		return tx.TefINTERNAL
	}
	// Update tracked automatically by ApplyStateTable
	if err := ctx.View.Update(accountKey, accountBytes); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}
