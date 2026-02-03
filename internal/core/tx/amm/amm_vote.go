package amm

import (
	"errors"
	"math"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
)

func init() {
	tx.Register(tx.TypeAMMVote, func() tx.Transaction {
		return &AMMVote{BaseTx: *tx.NewBaseTx(tx.TypeAMMVote, "")}
	})
}

// AMMVote votes on the trading fee for an AMM.
type AMMVote struct {
	tx.BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset tx.Asset `json:"Asset" xrpl:"Asset,asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 tx.Asset `json:"Asset2" xrpl:"Asset2,asset"`

	// TradingFee is the proposed fee in basis points (0-1000)
	TradingFee uint16 `json:"TradingFee" xrpl:"TradingFee"`
}

// NewAMMVote creates a new AMMVote transaction
func NewAMMVote(account string, asset, asset2 tx.Asset, tradingFee uint16) *AMMVote {
	return &AMMVote{
		BaseTx:     *tx.NewBaseTx(tx.TypeAMMVote, account),
		Asset:      asset,
		Asset2:     asset2,
		TradingFee: tradingFee,
	}
}

// TxType returns the transaction type
func (a *AMMVote) TxType() tx.Type {
	return tx.TypeAMMVote
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
	return tx.ReflectFlatten(a)
}

// RequiredAmendments returns the amendments required for this transaction type
func (a *AMMVote) RequiredAmendments() []string {
	return []string{amendment.AmendmentAMM, amendment.AmendmentFixUniversalNumber}
}

// Apply applies the AMMVote transaction to ledger state.
// Reference: rippled AMMVote.cpp applyVote
func (a *AMMVote) Apply(ctx *tx.ApplyContext) tx.Result {
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

	lptAMMBalance := amm.LPTokenBalance
	if lptAMMBalance == 0 {
		return tx.TecAMM_BALANCE // AMM empty
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
		return tx.TefINTERNAL
	}
	if err := ctx.View.Update(ammKey, ammBytes); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}
