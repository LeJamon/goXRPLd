package amm

import (
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/amendment"
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
func (a *AMMWithdraw) RequiredAmendments() [][32]byte {
	return [][32]byte{amendment.FeatureAMM, amendment.FeatureFixUniversalNumber}
}

// Apply applies the AMMWithdraw transaction to ledger state.
// Reference: rippled AMMWithdraw.cpp applyGuts
func (a *AMMWithdraw) Apply(ctx *tx.ApplyContext) tx.Result {
	accountID := ctx.AccountID

	// Find the AMM
	ammKey := computeAMMKeylet(a.Asset, a.Asset2)

	ammRawData, err := ctx.View.Read(ammKey)
	if err != nil || ammRawData == nil {
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

	// Get amounts from transaction - use Amount type directly
	amount1 := zeroAmount(a.Asset)
	amount2 := zeroAmount(a.Asset2)
	lpTokensRequested := zeroAmount(tx.Asset{}) // LP tokens

	if a.Amount != nil {
		amount1 = *a.Amount
	}
	if a.Amount2 != nil {
		amount2 = *a.Amount2
	}
	if a.LPTokenIn != nil {
		lpTokensRequested = *a.LPTokenIn
	}

	// Get current AMM balances from actual state (not stored in AMM entry)
	// Reference: rippled ammHolds - reads from AccountRoot (XRP) and trustlines (IOU)
	assetBalance1, assetBalance2, lptBalance := AMMHolds(ctx.View, amm, false)

	if lptBalance.IsZero() {
		return tx.TecAMM_BALANCE // AMM empty
	}

	// Get withdrawer's LP token balance from trustline
	// Reference: rippled AMMWithdraw.cpp preclaim line 251, 255-259
	lpTokensHeld := ammLPHolds(ctx.View, amm, accountID)
	if lpTokensHeld.IsZero() {
		// Account is not a liquidity provider
		// Note: Withdraw returns tecAMM_BALANCE (unlike Vote/Bid which return tecAMM_INVALID_TOKENS)
		return tx.TecAMM_BALANCE
	}

	// Result amounts - use tx.Amount for precision
	var lpTokensToRedeem tx.Amount
	var withdrawAmount1, withdrawAmount2 tx.Amount

	// Handle different withdrawal modes
	switch {
	case flags&tfLPToken != 0:
		// Proportional withdrawal for specified LP tokens
		// Equations 5 and 6: a = (t/T) * A, b = (t/T) * B
		if lpTokensRequested.IsZero() || lptBalance.IsZero() {
			return tx.TecAMM_INVALID_TOKENS
		}
		if isGreater(lpTokensRequested, lpTokensHeld) || isGreater(lpTokensRequested, lptBalance) {
			return tx.TecAMM_INVALID_TOKENS
		}
		// Calculate proportional amounts using Amount arithmetic
		withdrawAmount1 = proportionalAmount(assetBalance1, lpTokensRequested, lptBalance)
		withdrawAmount2 = proportionalAmount(assetBalance2, lpTokensRequested, lptBalance)
		lpTokensToRedeem = lpTokensRequested

	case flags&tfWithdrawAll != 0:
		// Withdraw all - proportional withdrawal of all LP tokens held
		if lpTokensHeld.IsZero() {
			return tx.TecAMM_INVALID_TOKENS
		}
		if isLessOrEqual(lptBalance, lpTokensHeld) {
			// Last LP withdrawing everything
			withdrawAmount1 = assetBalance1
			withdrawAmount2 = assetBalance2
			lpTokensToRedeem = lptBalance
		} else {
			withdrawAmount1 = proportionalAmount(assetBalance1, lpTokensHeld, lptBalance)
			withdrawAmount2 = proportionalAmount(assetBalance2, lpTokensHeld, lptBalance)
			lpTokensToRedeem = lpTokensHeld
		}

	case flags&tfOneAssetWithdrawAll != 0:
		// Withdraw all LP tokens as a single asset
		// Use equation 8: ammAssetOut
		if lpTokensHeld.IsZero() {
			return tx.TecAMM_INVALID_TOKENS
		}
		isWithdrawAsset1 := matchesAsset(a.Amount, a.Asset)
		if isWithdrawAsset1 {
			withdrawAmount1 = ammAssetOut(assetBalance1, lptBalance, lpTokensHeld, tfee)
			// Compare using IOU for mixed-type consistency
			if isGreater(toIOUForCalc(withdrawAmount1), toIOUForCalc(assetBalance1)) {
				return tx.TecAMM_BALANCE
			}
			withdrawAmount2 = zeroAmount(a.Asset2)
		} else {
			withdrawAmount2 = ammAssetOut(assetBalance2, lptBalance, lpTokensHeld, tfee)
			// Compare using IOU for mixed-type consistency
			if isGreater(toIOUForCalc(withdrawAmount2), toIOUForCalc(assetBalance2)) {
				return tx.TecAMM_BALANCE
			}
			withdrawAmount1 = zeroAmount(a.Asset)
		}
		lpTokensToRedeem = lpTokensHeld

	case flags&tfSingleAsset != 0:
		// Single asset withdrawal - compute LP tokens from amount
		// Equation 7: lpTokensIn function
		if amount1.IsZero() {
			return tx.TemMALFORMED
		}
		isWithdrawAsset1 := matchesAsset(a.Amount, a.Asset)
		if isWithdrawAsset1 {
			// Compare using IOU for mixed-type consistency
			if isGreater(toIOUForCalc(amount1), toIOUForCalc(assetBalance1)) {
				return tx.TecAMM_BALANCE
			}
			lpTokensToRedeem = calcLPTokensIn(assetBalance1, amount1, lptBalance, tfee)
			withdrawAmount1 = amount1
			withdrawAmount2 = zeroAmount(a.Asset2)
		} else {
			// Compare using IOU for mixed-type consistency
			if isGreater(toIOUForCalc(amount1), toIOUForCalc(assetBalance2)) {
				return tx.TecAMM_BALANCE
			}
			lpTokensToRedeem = calcLPTokensIn(assetBalance2, amount1, lptBalance, tfee)
			withdrawAmount1 = zeroAmount(a.Asset)
			withdrawAmount2 = amount1
		}
		if lpTokensToRedeem.IsZero() || isGreater(lpTokensToRedeem, lpTokensHeld) {
			return tx.TecAMM_INVALID_TOKENS
		}

	case flags&tfTwoAsset != 0:
		// Two asset withdrawal with limits
		if amount1.IsZero() || amount2.IsZero() {
			return tx.TemMALFORMED
		}
		// Calculate fractions using Amount arithmetic
		// Convert to IOU for precise fractional calculations (XRP division is integer-only)
		frac1 := toIOUForCalc(amount1).Div(toIOUForCalc(assetBalance1), false)
		frac2 := toIOUForCalc(amount2).Div(toIOUForCalc(assetBalance2), false)

		// Use the smaller fraction
		var frac tx.Amount
		if !assetBalance2.IsZero() && frac2.Compare(frac1) < 0 {
			frac = frac2
		} else {
			frac = frac1
		}

		lpTokensToRedeem = toIOUForCalc(lptBalance).Mul(frac, false)
		if lpTokensToRedeem.IsZero() || isGreater(lpTokensToRedeem, lpTokensHeld) {
			return tx.TecAMM_INVALID_TOKENS
		}
		withdrawAmount1 = toIOUForCalc(assetBalance1).Mul(frac, false)
		withdrawAmount2 = toIOUForCalc(assetBalance2).Mul(frac, false)

	case flags&tfOneAssetLPToken != 0:
		// Single asset withdrawal for specific LP tokens
		// Equation 8: ammAssetOut
		if lpTokensRequested.IsZero() {
			return tx.TecAMM_INVALID_TOKENS
		}
		if isGreater(lpTokensRequested, lpTokensHeld) || isGreater(lpTokensRequested, lptBalance) {
			return tx.TecAMM_INVALID_TOKENS
		}
		isWithdrawAsset1 := matchesAsset(a.Amount, a.Asset)
		if isWithdrawAsset1 {
			withdrawAmount1 = ammAssetOut(assetBalance1, lptBalance, lpTokensRequested, tfee)
			// Compare using IOU for mixed-type consistency
			if isGreater(toIOUForCalc(withdrawAmount1), toIOUForCalc(assetBalance1)) {
				return tx.TecAMM_BALANCE
			}
			if !amount1.IsZero() && isGreater(toIOUForCalc(amount1), toIOUForCalc(withdrawAmount1)) {
				return tx.TecAMM_FAILED
			}
			withdrawAmount2 = zeroAmount(a.Asset2)
		} else {
			withdrawAmount2 = ammAssetOut(assetBalance2, lptBalance, lpTokensRequested, tfee)
			// Compare using IOU for mixed-type consistency
			if isGreater(toIOUForCalc(withdrawAmount2), toIOUForCalc(assetBalance2)) {
				return tx.TecAMM_BALANCE
			}
			if !amount1.IsZero() && isGreater(toIOUForCalc(amount1), toIOUForCalc(withdrawAmount2)) {
				return tx.TecAMM_FAILED
			}
			withdrawAmount1 = zeroAmount(a.Asset)
		}
		lpTokensToRedeem = lpTokensRequested

	case flags&tfLimitLPToken != 0:
		// Single asset withdrawal with effective price limit
		if amount1.IsZero() || a.EPrice == nil || a.EPrice.IsZero() {
			return tx.TemMALFORMED
		}

		isWithdrawAsset1 := matchesAsset(a.Amount, a.Asset)
		var assetBalance tx.Amount
		if isWithdrawAsset1 {
			assetBalance = assetBalance1
		} else {
			assetBalance = assetBalance2
		}

		lpTokensToRedeem = calcLPTokensIn(assetBalance, amount1, lptBalance, tfee)
		if lpTokensToRedeem.IsZero() || isGreater(lpTokensToRedeem, lpTokensHeld) {
			return tx.TecAMM_INVALID_TOKENS
		}

		// Check effective price: EP = lpTokens / amount
		effectivePrice := lpTokensToRedeem.Div(amount1, false)
		if isGreater(effectivePrice, *a.EPrice) {
			return tx.TecAMM_FAILED
		}

		if isWithdrawAsset1 {
			withdrawAmount1 = amount1
			withdrawAmount2 = zeroAmount(a.Asset2)
		} else {
			withdrawAmount1 = zeroAmount(a.Asset)
			withdrawAmount2 = amount1
		}

	default:
		return tx.TemMALFORMED
	}

	if lpTokensToRedeem.IsZero() {
		return tx.TecAMM_INVALID_TOKENS
	}

	// Verify withdrawal doesn't exceed balances
	// Convert to IOU for comparison since withdrawAmount may be IOU from calculations
	if isGreater(toIOUForCalc(withdrawAmount1), toIOUForCalc(assetBalance1)) {
		return tx.TecAMM_BALANCE
	}
	if isGreater(toIOUForCalc(withdrawAmount2), toIOUForCalc(assetBalance2)) {
		return tx.TecAMM_BALANCE
	}

	// Per rippled: Cannot withdraw one side of the pool while leaving the other
	isSingleOrTwoAsset := flags&(tfSingleAsset|tfTwoAsset|tfLimitLPToken) != 0
	if isSingleOrTwoAsset {
		// Convert to IOU for comparison since withdrawAmount may be IOU from calculations
		w1EqualsB1 := toIOUForCalc(withdrawAmount1).Compare(toIOUForCalc(assetBalance1)) == 0
		w2EqualsB2 := toIOUForCalc(withdrawAmount2).Compare(toIOUForCalc(assetBalance2)) == 0
		if (w1EqualsB1 && !w2EqualsB2) || (w2EqualsB2 && !w1EqualsB1) {
			return tx.TecAMM_BALANCE
		}
	}

	// Transfer assets from AMM to withdrawer
	isXRP1 := a.Asset.Currency == "" || a.Asset.Currency == "XRP"
	isXRP2 := a.Asset2.Currency == "" || a.Asset2.Currency == "XRP"

	if isXRP1 && !withdrawAmount1.IsZero() {
		// Convert to drops, handling IOU representation from calculations
		drops := uint64(iouToDrops(withdrawAmount1))
		ammAccount.Balance -= drops
		ctx.Account.Balance += drops
	}
	if isXRP2 && !withdrawAmount2.IsZero() {
		// Convert to drops, handling IOU representation from calculations
		drops := uint64(iouToDrops(withdrawAmount2))
		ammAccount.Balance -= drops
		ctx.Account.Balance += drops
	}

	// For IOU transfers: check reserve if trust line creation is needed,
	// then transfer tokens.
	// Reference: rippled AMMWithdraw.cpp lines 581-647
	enabledFixAMMv1_2 := ctx.Rules().Enabled(amendment.FeatureFixAMMv1_2)

	if !isXRP1 && !withdrawAmount1.IsZero() {
		issuerID, err := sle.DecodeAccountID(a.Asset.Issuer)
		if err != nil {
			return tx.TefINTERNAL
		}
		if result := withdrawIOUToAccount(ctx, accountID, issuerID, ammAccountID, a.Asset, withdrawAmount1, enabledFixAMMv1_2); result != tx.TesSUCCESS {
			return result
		}
	}
	if !isXRP2 && !withdrawAmount2.IsZero() {
		issuerID, err := sle.DecodeAccountID(a.Asset2.Issuer)
		if err != nil {
			return tx.TefINTERNAL
		}
		if result := withdrawIOUToAccount(ctx, accountID, issuerID, ammAccountID, a.Asset2, withdrawAmount2, enabledFixAMMv1_2); result != tx.TesSUCCESS {
			return result
		}
	}

	// Redeem LP tokens - subtract from AMM LP balance
	newLPBalance, err := amm.LPTokenBalance.Sub(lpTokensToRedeem)
	if err != nil {
		return tx.TefINTERNAL
	}
	amm.LPTokenBalance = newLPBalance

	// NOTE: Asset balances are NOT stored in AMM entry
	// They are updated by the balance transfers above:
	// - XRP: via ammAccount.Balance -= drops
	// - IOU: via trustline updates (createOrUpdateAMMTrustline)

	// Check if AMM should be deleted (empty)
	ammDeleted := false
	if newLPBalance.IsZero() {
		if err := ctx.View.Erase(ammKey); err != nil {
			return tx.TefINTERNAL
		}
		if err := ctx.View.Erase(ammAccountKey); err != nil {
			return tx.TefINTERNAL
		}
		ammDeleted = true
	}

	if !ammDeleted {
		ammBytes, err := serializeAMMData(amm)
		if err != nil {
			return tx.TefINTERNAL
		}
		if err := ctx.View.Update(ammKey, ammBytes); err != nil {
			return tx.TefINTERNAL
		}

		ammAccountBytes, err := sle.SerializeAccountRoot(ammAccount)
		if err != nil {
			return tx.TefINTERNAL
		}
		if err := ctx.View.Update(ammAccountKey, ammAccountBytes); err != nil {
			return tx.TefINTERNAL
		}
	}

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

// withdrawIOUToAccount handles IOU transfer from AMM to withdrawer, including
// reserve check and trust line creation when needed.
// Reference: rippled AMMWithdraw.cpp sufficientReserve (lines 581-603) +
// accountSend (lines 609-646)
func withdrawIOUToAccount(
	ctx *tx.ApplyContext,
	accountID, issuerID, ammAccountID [20]byte,
	asset tx.Asset,
	amount tx.Amount,
	enabledFixAMMv1_2 bool,
) tx.Result {
	// Check if withdrawer already has a trust line for this IOU.
	trustLineKey := keylet.Line(accountID, issuerID, asset.Currency)
	trustLineExists, err := ctx.View.Exists(trustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}

	if !trustLineExists {
		// Reserve check: with fixAMMv1_2, verify the withdrawer has enough
		// reserve for the new trust line before creating it.
		// Reference: rippled AMMWithdraw.cpp lines 583-601
		if enabledFixAMMv1_2 {
			ownerCount := ctx.Account.OwnerCount
			// See also SetTrust::doApply(): ownerCount < 2 → no reserve needed
			if ownerCount >= 2 {
				reserve := ctx.AccountReserve(ownerCount + 1)
				if ctx.Account.Balance < reserve {
					return tx.TecINSUFFICIENT_RESERVE
				}
			}
		}

		// Create trust line for the withdrawer.
		// Reference: rippled uses accountSend → rippleCredit → trustCreate
		if result := createWithdrawTrustLine(ctx, accountID, issuerID, asset, amount, trustLineKey); result != tx.TesSUCCESS {
			return result
		}
	} else {
		// Trust line exists — just credit the withdrawer's balance.
		if err := updateTrustlineBalanceInView(accountID, issuerID, asset.Currency, amount, ctx.View); err != nil {
			return tx.TefINTERNAL
		}
	}

	// Debit AMM's trust line (negative delta)
	if err := createOrUpdateAMMTrustline(ammAccountID, asset, amount.Negate(), ctx.View); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}

// createWithdrawTrustLine creates a new trust line between withdrawer and
// issuer, setting the initial balance to the withdraw amount.
// Reference: rippled trustCreate via accountSend path
func createWithdrawTrustLine(
	ctx *tx.ApplyContext,
	accountID, issuerID [20]byte,
	asset tx.Asset,
	amount tx.Amount,
	trustLineKey keylet.Keylet,
) tx.Result {
	// Determine low/high accounts
	accountIsLow := keylet.IsLowAccount(accountID, issuerID)
	var lowAccountID, highAccountID [20]byte
	if accountIsLow {
		lowAccountID = accountID
		highAccountID = issuerID
	} else {
		lowAccountID = issuerID
		highAccountID = accountID
	}

	lowAccountStr, err := sle.EncodeAccountID(lowAccountID)
	if err != nil {
		return tx.TefINTERNAL
	}
	highAccountStr, err := sle.EncodeAccountID(highAccountID)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Set balance: the withdrawer (receiver) gets the tokens.
	// Convention: positive balance = LOW account holds tokens.
	// When receiver (account) is LOW → positive balance
	// When receiver (account) is HIGH → negative balance
	var balance tx.Amount
	if accountIsLow {
		balance = sle.NewIssuedAmountFromValue(
			amount.Mantissa(), amount.Exponent(),
			asset.Currency, sle.AccountOneAddress,
		)
	} else {
		negated := amount.Negate()
		balance = sle.NewIssuedAmountFromValue(
			negated.Mantissa(), negated.Exponent(),
			asset.Currency, sle.AccountOneAddress,
		)
	}

	// Flags: receiver gets reserve flag + NoRipple per DefaultRipple setting
	// Reference: rippled trustCreate
	var flags uint32
	if accountIsLow {
		flags |= sle.LsfLowReserve
	} else {
		flags |= sle.LsfHighReserve
	}

	// Set NoRipple based on DefaultRipple for each side
	acctData, err := ctx.View.Read(keylet.Account(accountID))
	if err != nil || acctData == nil {
		return tx.TefINTERNAL
	}
	acct, err := sle.ParseAccountRoot(acctData)
	if err != nil {
		return tx.TefINTERNAL
	}
	if (acct.Flags & sle.LsfDefaultRipple) == 0 {
		if accountIsLow {
			flags |= sle.LsfLowNoRipple
		} else {
			flags |= sle.LsfHighNoRipple
		}
	}

	issuerAcctData, err := ctx.View.Read(keylet.Account(issuerID))
	if err != nil || issuerAcctData == nil {
		return tx.TefINTERNAL
	}
	issuerAcct, err := sle.ParseAccountRoot(issuerAcctData)
	if err != nil {
		return tx.TefINTERNAL
	}
	if (issuerAcct.Flags & sle.LsfDefaultRipple) == 0 {
		if !accountIsLow {
			flags |= sle.LsfLowNoRipple
		} else {
			flags |= sle.LsfHighNoRipple
		}
	}

	rs := &sle.RippleState{
		Balance:   balance,
		LowLimit:  tx.NewIssuedAmount(0, -100, asset.Currency, lowAccountStr),
		HighLimit: tx.NewIssuedAmount(0, -100, asset.Currency, highAccountStr),
		Flags:     flags,
	}

	// Insert into both owner directories
	lowDirKey := keylet.OwnerDir(lowAccountID)
	lowDirResult, err := sle.DirInsert(ctx.View, lowDirKey, trustLineKey.Key, func(dir *sle.DirectoryNode) {
		dir.Owner = lowAccountID
	})
	if err != nil {
		return tx.TefINTERNAL
	}
	rs.LowNode = lowDirResult.Page

	highDirKey := keylet.OwnerDir(highAccountID)
	highDirResult, err := sle.DirInsert(ctx.View, highDirKey, trustLineKey.Key, func(dir *sle.DirectoryNode) {
		dir.Owner = highAccountID
	})
	if err != nil {
		return tx.TefINTERNAL
	}
	rs.HighNode = highDirResult.Page

	// Serialize and insert
	rsBytes, err := sle.SerializeRippleState(rs)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Insert(trustLineKey, rsBytes); err != nil {
		return tx.TefINTERNAL
	}

	// Increment withdrawer's owner count for the new trust line
	ctx.Account.OwnerCount++

	return tx.TesSUCCESS
}
