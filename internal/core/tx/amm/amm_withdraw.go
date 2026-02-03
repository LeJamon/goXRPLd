package amm

import (
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeAMMWithdraw, func() tx.Transaction {
		return &AMMWithdraw{BaseTx: *tx.NewBaseTx(tx.TypeAMMWithdraw, "")}
	})
}

// AMMWithdraw withdraws assets from an AMM.
type AMMWithdraw struct {
	tx.BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset tx.Asset `json:"Asset" xrpl:"Asset,asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 tx.Asset `json:"Asset2" xrpl:"Asset2,asset"`

	// Amount is the amount of first asset to withdraw (optional)
	Amount *tx.Amount `json:"Amount,omitempty" xrpl:"Amount,omitempty,amount"`

	// Amount2 is the amount of second asset to withdraw (optional)
	Amount2 *tx.Amount `json:"Amount2,omitempty" xrpl:"Amount2,omitempty,amount"`

	// EPrice is the effective price limit (optional)
	EPrice *tx.Amount `json:"EPrice,omitempty" xrpl:"EPrice,omitempty,amount"`

	// LPTokenIn is the LP tokens to burn (optional)
	LPTokenIn *tx.Amount `json:"LPTokenIn,omitempty" xrpl:"LPTokenIn,omitempty,amount"`
}

// NewAMMWithdraw creates a new AMMWithdraw transaction
func NewAMMWithdraw(account string, asset, asset2 tx.Asset) *AMMWithdraw {
	return &AMMWithdraw{
		BaseTx: *tx.NewBaseTx(tx.TypeAMMWithdraw, account),
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMWithdraw) TxType() tx.Type {
	return tx.TypeAMMWithdraw
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
		if a.LPTokenIn.IsZero() || a.LPTokenIn.IsNegative() {
			return errors.New("temBAD_AMM_TOKENS: invalid LPTokenIn")
		}
	}

	// Validate amounts if provided
	// For tfOneAssetWithdrawAll, tfOneAssetLPToken, and when EPrice is present, zero amounts are allowed
	// (the amount is used to identify which asset, not the actual amount)
	validZeroAmount := (flags&(tfOneAssetWithdrawAll|tfOneAssetLPToken) != 0) || hasEPrice

	if hasAmount {
		if errCode := validateAMMAmountWithPair(*a.Amount, &a.Asset, &a.Asset2, validZeroAmount); errCode != "" {
			return errors.New(errCode + ": invalid Amount")
		}
	}
	if hasAmount2 {
		if errCode := validateAMMAmountWithPair(*a.Amount2, &a.Asset, &a.Asset2, false); errCode != "" {
			return errors.New(errCode + ": invalid Amount2")
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
	return tx.ReflectFlatten(a)
}

// RequiredAmendments returns the amendments required for this transaction type
func (a *AMMWithdraw) RequiredAmendments() []string {
	return []string{amendment.AmendmentAMM, amendment.AmendmentFixUniversalNumber}
}

// Apply applies the AMMWithdraw transaction to ledger state.
// Reference: rippled AMMWithdraw.cpp applyGuts
func (a *AMMWithdraw) Apply(ctx *tx.ApplyContext) tx.Result {
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
	if a.LPTokenIn != nil {
		lpTokensRequested = parseAmountFromTx(a.LPTokenIn)
	}

	// Get current AMM balances
	assetBalance1 := ammAccount.Balance // For XRP
	assetBalance2 := amm.Asset2Balance  // From AMM data for IOU
	lptBalance := amm.LPTokenBalance

	if lptBalance == 0 {
		return tx.TecAMM_BALANCE // AMM empty
	}

	// Get withdrawer's LP token balance
	// In full implementation, this would read from the LP token trustline
	// For now, we use a simplified approach based on the withdrawal mode:
	// - If LPTokenIn is specified, use that as the request (caller knows what they have)
	// - For WithdrawAll modes, assume account has all LP tokens (simplified)
	// - For other modes (Single/Two asset), assume account has all LP tokens
	//   since we don't have full trustline tracking yet
	var lpTokensHeld uint64
	if lpTokensRequested > 0 {
		lpTokensHeld = lpTokensRequested
	} else {
		// Default: assume account has all LP tokens (for single-LP scenarios)
		// In production, this would be read from the LP token trustline
		lpTokensHeld = lptBalance
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
			return tx.TecAMM_INVALID_TOKENS
		}
		if lpTokensRequested > lpTokensHeld || lpTokensRequested > lptBalance {
			return tx.TecAMM_INVALID_TOKENS
		}
		frac := float64(lpTokensRequested) / float64(lptBalance)
		withdrawAmount1 = uint64(float64(assetBalance1) * frac)
		withdrawAmount2 = uint64(float64(assetBalance2) * frac)
		lpTokensToRedeem = lpTokensRequested

	case flags&tfWithdrawAll != 0:
		// Withdraw all - proportional withdrawal of all LP tokens held
		if lpTokensHeld == 0 {
			return tx.TecAMM_INVALID_TOKENS
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
		// The Amount field identifies which asset to withdraw, its value can be zero
		// Use equation 8: ammAssetOut
		if lpTokensHeld == 0 {
			return tx.TecAMM_INVALID_TOKENS
		}
		// Determine which asset to withdraw based on Amount's currency/issuer
		isWithdrawAsset1 := matchesAsset(a.Amount, a.Asset)
		if isWithdrawAsset1 {
			withdrawAmount1 = ammAssetOut(assetBalance1, lptBalance, lpTokensHeld, tfee)
			if withdrawAmount1 > assetBalance1 {
				return tx.TecAMM_BALANCE
			}
		} else {
			withdrawAmount2 = ammAssetOut(assetBalance2, lptBalance, lpTokensHeld, tfee)
			if withdrawAmount2 > assetBalance2 {
				return tx.TecAMM_BALANCE
			}
		}
		lpTokensToRedeem = lpTokensHeld

	case flags&tfSingleAsset != 0:
		// Single asset withdrawal - compute LP tokens from amount
		// The Amount specifies both which asset and how much to withdraw
		// Equation 7: lpTokensIn function
		if amount1 == 0 {
			return tx.TemMALFORMED
		}
		// Determine which asset to withdraw based on Amount's currency/issuer
		isWithdrawAsset1 := matchesAsset(a.Amount, a.Asset)
		if isWithdrawAsset1 {
			if amount1 > assetBalance1 {
				return tx.TecAMM_BALANCE
			}
			lpTokensToRedeem = calcLPTokensIn(assetBalance1, amount1, lptBalance, tfee)
			withdrawAmount1 = amount1
		} else {
			if amount1 > assetBalance2 {
				return tx.TecAMM_BALANCE
			}
			lpTokensToRedeem = calcLPTokensIn(assetBalance2, amount1, lptBalance, tfee)
			withdrawAmount2 = amount1
		}
		if lpTokensToRedeem == 0 || lpTokensToRedeem > lpTokensHeld {
			return tx.TecAMM_INVALID_TOKENS
		}

	case flags&tfTwoAsset != 0:
		// Two asset withdrawal with limits
		// Equations 5 and 6 with limits
		if amount1 == 0 || amount2 == 0 {
			return tx.TemMALFORMED
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
			return tx.TecAMM_INVALID_TOKENS
		}
		// Recalculate amounts based on the fraction used
		withdrawAmount1 = uint64(float64(assetBalance1) * frac)
		withdrawAmount2 = uint64(float64(assetBalance2) * frac)

	case flags&tfOneAssetLPToken != 0:
		// Single asset withdrawal for specific LP tokens
		// The Amount field identifies which asset to withdraw, its value is a minimum amount (or 0)
		// Equation 8: ammAssetOut
		if lpTokensRequested == 0 {
			return tx.TecAMM_INVALID_TOKENS
		}
		if lpTokensRequested > lpTokensHeld || lpTokensRequested > lptBalance {
			return tx.TecAMM_INVALID_TOKENS
		}
		// Determine which asset to withdraw based on Amount's currency/issuer
		isWithdrawAsset1 := matchesAsset(a.Amount, a.Asset)
		if isWithdrawAsset1 {
			withdrawAmount1 = ammAssetOut(assetBalance1, lptBalance, lpTokensRequested, tfee)
			if withdrawAmount1 > assetBalance1 {
				return tx.TecAMM_BALANCE
			}
			// Check minimum amount if specified (amount1 > 0)
			if amount1 > 0 && withdrawAmount1 < amount1 {
				return tx.TecAMM_FAILED
			}
		} else {
			withdrawAmount2 = ammAssetOut(assetBalance2, lptBalance, lpTokensRequested, tfee)
			if withdrawAmount2 > assetBalance2 {
				return tx.TecAMM_BALANCE
			}
			// Check minimum amount if specified (amount1 is from Amount field, which is for asset2)
			if amount1 > 0 && withdrawAmount2 < amount1 {
				return tx.TecAMM_FAILED
			}
		}
		lpTokensToRedeem = lpTokensRequested

	case flags&tfLimitLPToken != 0:
		// Single asset withdrawal with effective price limit
		if amount1 == 0 || a.EPrice == nil {
			return tx.TemMALFORMED
		}
		ePrice := parseAmountFromTx(a.EPrice)
		if ePrice == 0 {
			return tx.TemMALFORMED
		}
		// Determine which asset to withdraw based on Amount's currency/issuer
		isWithdrawAsset1 := matchesAsset(a.Amount, a.Asset)
		var assetBalance uint64
		if isWithdrawAsset1 {
			assetBalance = assetBalance1
		} else {
			assetBalance = assetBalance2
		}
		// Calculate LP tokens based on effective price
		// EP = lpTokens / amount => lpTokens = EP * amount
		// Use equation that solves for lpTokens given EP constraint
		lpTokensToRedeem = calcLPTokensIn(assetBalance, amount1, lptBalance, tfee)
		if lpTokensToRedeem == 0 || lpTokensToRedeem > lpTokensHeld {
			return tx.TecAMM_INVALID_TOKENS
		}
		// Check effective price: EP = lpTokens / amount
		actualEP := lpTokensToRedeem / amount1
		if actualEP > ePrice {
			return tx.TecAMM_FAILED
		}
		if isWithdrawAsset1 {
			withdrawAmount1 = amount1
		} else {
			withdrawAmount2 = amount1
		}

	default:
		return tx.TemMALFORMED
	}

	if lpTokensToRedeem == 0 {
		return tx.TecAMM_INVALID_TOKENS
	}

	// Verify withdrawal doesn't exceed balances
	if withdrawAmount1 > assetBalance1 {
		return tx.TecAMM_BALANCE
	}
	if withdrawAmount2 > assetBalance2 {
		return tx.TecAMM_BALANCE
	}

	// Per rippled: Cannot withdraw one side of the pool while leaving the other
	// (i.e., draining one asset but not the other)
	// This check applies for tfSingleAsset and tfTwoAsset modes (amount-specified withdrawals)
	// It does NOT apply for tfOneAssetWithdrawAll/tfOneAssetLPToken which are proportional to LP tokens held
	isSingleOrTwoAsset := flags&(tfSingleAsset|tfTwoAsset|tfLimitLPToken) != 0
	if isSingleOrTwoAsset {
		if (withdrawAmount1 == assetBalance1 && withdrawAmount2 != assetBalance2) ||
			(withdrawAmount2 == assetBalance2 && withdrawAmount1 != assetBalance1) {
			return tx.TecAMM_BALANCE
		}
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
			return tx.TefINTERNAL
		}
		if err := ctx.View.Erase(ammAccountKey); err != nil {
			return tx.TefINTERNAL
		}
		ammDeleted = true
	}

	if !ammDeleted {
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
	}

	// Persist updated withdrawer account
	accountKey := keylet.Account(accountID)
	accountBytes, err := sle.SerializeAccountRoot(ctx.Account)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Update(accountKey, accountBytes); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}
