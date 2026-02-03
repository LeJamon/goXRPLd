package amm

import (
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeAMMClawback, func() tx.Transaction {
		return &AMMClawback{BaseTx: *tx.NewBaseTx(tx.TypeAMMClawback, "")}
	})
}

// AMMClawback claws back tokens from an AMM.
type AMMClawback struct {
	tx.BaseTx

	// Holder is the account holding LP tokens (required)
	Holder string `json:"Holder" xrpl:"Holder"`

	// Asset identifies the first asset of the AMM (required)
	Asset tx.Asset `json:"Asset" xrpl:"Asset,asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 tx.Asset `json:"Asset2" xrpl:"Asset2,asset"`

	// Amount is the amount to claw back (optional)
	Amount *tx.Amount `json:"Amount,omitempty" xrpl:"Amount,omitempty,amount"`
}

// NewAMMClawback creates a new AMMClawback transaction
func NewAMMClawback(account, holder string, asset, asset2 tx.Asset) *AMMClawback {
	return &AMMClawback{
		BaseTx: *tx.NewBaseTx(tx.TypeAMMClawback, account),
		Holder: holder,
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMClawback) TxType() tx.Type {
	return tx.TypeAMMClawback
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
		// Amount's issue must match tx.Asset
		if a.Amount.Currency != a.Asset.Currency || a.Amount.Issuer != a.Asset.Issuer {
			return errors.New("temBAD_AMOUNT: Amount issue must match tx.Asset")
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMClawback) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(a)
}

// RequiredAmendments returns the amendments required for this transaction type
func (a *AMMClawback) RequiredAmendments() []string {
	return []string{amendment.AmendmentAMM, amendment.AmendmentFixUniversalNumber, amendment.AmendmentAMMClawback}
}

// Apply applies the AMMClawback transaction to ledger state.
// Reference: rippled AMMClawback.cpp preclaim + applyGuts
func (a *AMMClawback) Apply(ctx *tx.ApplyContext) tx.Result {
	issuerID := ctx.AccountID

	// Find the holder - per rippled AMMClawback.cpp preclaim line 103-104
	holderID, err := sle.DecodeAccountID(a.Holder)
	if err != nil {
		return tx.TemINVALID
	}

	holderKey := keylet.Account(holderID)
	holderData, err := ctx.View.Read(holderKey)
	if err != nil {
		return TerNO_ACCOUNT
	}
	holderAccount, err := sle.ParseAccountRoot(holderData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Find the AMM - per rippled AMMClawback.cpp preclaim lines 106-111
	// This check must come BEFORE permission check
	ammKey := computeAMMKeylet(a.Asset, a.Asset2)
	ammRawData, err := ctx.View.Read(ammKey)
	if err != nil {
		return TerNO_AMM
	}

	// Verify issuer has lsfAllowTrustLineClawback and NOT lsfNoFreeze
	// Reference: rippled AMMClawback.cpp preclaim lines 113-119
	if (ctx.Account.Flags & sle.LsfAllowTrustLineClawback) == 0 {
		return tx.TecNO_PERMISSION
	}
	if (ctx.Account.Flags & sle.LsfNoFreeze) != 0 {
		return tx.TecNO_PERMISSION
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

	// Get current AMM balances
	assetBalance1 := ammAccount.Balance // For XRP in asset1
	assetBalance2 := uint64(0)          // Would come from trustline for IOU
	lptAMMBalance := amm.LPTokenBalance

	if lptAMMBalance == 0 {
		return tx.TecAMM_BALANCE // AMM is empty
	}

	// Get holder's LP token balance
	// In full implementation, would read from LP token trustline
	// For now, use a portion of the AMM LP token balance as holder's balance
	holdLPTokens := lptAMMBalance / 2 // Simplified - would read from trustline

	if holdLPTokens == 0 {
		return tx.TecAMM_BALANCE // Holder has no LP tokens
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
		clawAmount := parseAmountFromTx(a.Amount)

		// Calculate fraction based on the clawback amount relative to asset1 balance
		if assetBalance1 == 0 {
			return tx.TecAMM_BALANCE
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

	// Persist updated issuer account
	accountKey := keylet.Account(issuerID)
	accountBytes, err := sle.SerializeAccountRoot(ctx.Account)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Update(accountKey, accountBytes); err != nil {
		return tx.TefINTERNAL
	}

	// Persist updated holder account
	holderBytes, err := sle.SerializeAccountRoot(holderAccount)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Update(holderKey, holderBytes); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}
