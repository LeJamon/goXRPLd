package amm

import (
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeAMMCreate, func() tx.Transaction {
		return &AMMCreate{BaseTx: *tx.NewBaseTx(tx.TypeAMMCreate, "")}
	})
}

// AMMCreate creates an Automated Market Maker (AMM) instance.
type AMMCreate struct {
	tx.BaseTx

	// Amount is the first asset to deposit (required)
	Amount tx.Amount `json:"Amount" xrpl:"Amount,amount"`

	// Amount2 is the second asset to deposit (required)
	Amount2 tx.Amount `json:"Amount2" xrpl:"Amount2,amount"`

	// TradingFee is the fee in basis points (0-1000, where 1000 = 1%)
	TradingFee uint16 `json:"TradingFee" xrpl:"TradingFee"`
}

// NewAMMCreate creates a new AMMCreate transaction
func NewAMMCreate(account string, amount1, amount2 tx.Amount, tradingFee uint16) *AMMCreate {
	return &AMMCreate{
		BaseTx:     *tx.NewBaseTx(tx.TypeAMMCreate, account),
		Amount:     amount1,
		Amount2:    amount2,
		TradingFee: tradingFee,
	}
}

// TxType returns the transaction type
func (a *AMMCreate) TxType() tx.Type {
	return tx.TypeAMMCreate
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
	if a.Amount.IsZero() {
		return errors.New("temMALFORMED: Amount is required")
	}

	// Amount2 is required and must be positive
	if a.Amount2.IsZero() {
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

// Flatten returns a flat map of all transaction fields
func (a *AMMCreate) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(a)
}

// RequiredAmendments returns the amendments required for this transaction type
func (a *AMMCreate) RequiredAmendments() []string {
	return []string{amendment.AmendmentAMM, amendment.AmendmentFixUniversalNumber}
}

// Apply applies the AMMCreate transaction to ledger state.
// Reference: rippled AMMCreate.cpp doApply / applyCreate
func (a *AMMCreate) Apply(ctx *tx.ApplyContext) tx.Result {
	accountID := ctx.AccountID

	// Build assets for keylet computation
	asset1 := tx.Asset{Currency: a.Amount.Currency, Issuer: a.Amount.Issuer}
	asset2 := tx.Asset{Currency: a.Amount2.Currency, Issuer: a.Amount2.Issuer}

	// Compute the AMM keylet from the asset pair
	ammKey := computeAMMKeylet(asset1, asset2)

	// Check if AMM already exists
	exists, _ := ctx.View.Exists(ammKey)
	if exists {
		return tx.TecDUPLICATE
	}

	// Compute the AMM account ID from keylet
	ammAccountID := computeAMMAccountID(ammKey.Key)
	ammAccountAddr, _ := encodeAccountID(ammAccountID)

	// Check if AMM account already exists (should not happen)
	ammAccountKey := keylet.Account(ammAccountID)
	acctExists, _ := ctx.View.Exists(ammAccountKey)
	if acctExists {
		return tx.TecDUPLICATE
	}

	// Parse amounts
	amount1 := parseAmountFromTx(&a.Amount)
	amount2 := parseAmountFromTx(&a.Amount2)

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
		return tx.TecAMM_BALANCE // AMM empty or invalid LP token calculation
	}

	// Generate LP token currency code
	lptCurrency := generateAMMLPTCurrency(a.Amount.Currency, a.Amount2.Currency)

	// Create the AMM pseudo-account with lsfAMM flag
	ammAccount := &sle.AccountRoot{
		Account:    ammAccountAddr,
		Balance:    0,
		Sequence:   0,
		OwnerCount: 1, // For the AMM entry itself
		Flags:      sle.LsfAMM,
	}

	// Create the AMM entry
	ammData := &AMMData{
		Account:        ammAccountID,
		TradingFee:     a.TradingFee,
		LPTokenBalance: lpTokenBalance,
		AssetBalance:   amount1,
		Asset2Balance:  amount2,
		VoteSlots:      make([]VoteSlotData, 0),
	}

	// Set asset currency bytes
	ammData.Asset = sle.GetCurrencyBytes(a.Amount.Currency)
	ammData.Asset2 = sle.GetCurrencyBytes(a.Amount2.Currency)

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
	ammAccountBytes, err := sle.SerializeAccountRoot(ammAccount)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Insert(ammAccountKey, ammAccountBytes); err != nil {
		return tx.TefINTERNAL
	}

	// Store the AMM entry
	ammBytes, err := serializeAMM(ammData, accountID)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Insert(ammKey, ammBytes); err != nil {
		return tx.TefINTERNAL
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
		issuerID1, _ := sle.DecodeAccountID(asset1.Issuer)
		if err := updateTrustlineBalanceInView(accountID, issuerID1, asset1.Currency, -int64(amount1), ctx.View); err != nil {
			return TecUNFUNDED_AMM
		}
	}
	if !isXRP2 {
		if err := createOrUpdateAMMTrustline(ammAccountID, asset2, amount2, ctx.View); err != nil {
			return TecNO_LINE
		}
		issuerID2, _ := sle.DecodeAccountID(asset2.Issuer)
		if err := updateTrustlineBalanceInView(accountID, issuerID2, asset2.Currency, -int64(amount2), ctx.View); err != nil {
			return TecUNFUNDED_AMM
		}
	}

	// Create LP token trustline for creator
	lptAsset := tx.Asset{
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
	accountBytes, err := sle.SerializeAccountRoot(ctx.Account)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Update(accountKey, accountBytes); err != nil {
		return tx.TefINTERNAL
	}

	// Update AMM account balance (for XRP)
	ammAccountBytes, err = sle.SerializeAccountRoot(ammAccount)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Update(ammAccountKey, ammAccountBytes); err != nil {
		return tx.TefINTERNAL
	}

	// Metadata for created AMM and AMM account tracked automatically by ApplyStateTable

	return tx.TesSUCCESS
}
