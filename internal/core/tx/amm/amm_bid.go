package amm

import (
	"errors"
	"math"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeAMMBid, func() tx.Transaction {
		return &AMMBid{BaseTx: *tx.NewBaseTx(tx.TypeAMMBid, "")}
	})
}

// AMMBid places a bid on an AMM auction slot.
type AMMBid struct {
	tx.BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset tx.Asset `json:"Asset" xrpl:"Asset,asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 tx.Asset `json:"Asset2" xrpl:"Asset2,asset"`

	// BidMin is the minimum bid amount (optional)
	BidMin *tx.Amount `json:"BidMin,omitempty" xrpl:"BidMin,omitempty,amount"`

	// BidMax is the maximum bid amount (optional)
	BidMax *tx.Amount `json:"BidMax,omitempty" xrpl:"BidMax,omitempty,amount"`

	// AuthAccounts are accounts to authorize for discounted trading (optional)
	AuthAccounts []AuthAccount `json:"AuthAccounts,omitempty" xrpl:"AuthAccounts,omitempty"`
}

// NewAMMBid creates a new AMMBid transaction
func NewAMMBid(account string, asset, asset2 tx.Asset) *AMMBid {
	return &AMMBid{
		BaseTx: *tx.NewBaseTx(tx.TypeAMMBid, account),
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMBid) TxType() tx.Type {
	return tx.TypeAMMBid
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

	// Validate BidMin if present - per rippled AMMBid.cpp lines 57-63
	// Uses invalidAMMAmount() which returns temBAD_AMOUNT for invalid amounts
	if a.BidMin != nil {
		if err := validateAMMAmount(*a.BidMin); err != nil {
			return errors.New("temBAD_AMOUNT: invalid min slot price")
		}
	}

	// Validate BidMax if present - per rippled AMMBid.cpp lines 65-71
	if a.BidMax != nil {
		if err := validateAMMAmount(*a.BidMax); err != nil {
			return errors.New("temBAD_AMOUNT: invalid max slot price")
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
	return tx.ReflectFlatten(a)
}

// RequiredAmendments returns the amendments required for this transaction type
func (a *AMMBid) RequiredAmendments() []string {
	return []string{amendment.AmendmentAMM, amendment.AmendmentFixUniversalNumber}
}

// Apply applies the AMMBid transaction to ledger state.
// Reference: rippled AMMBid.cpp applyBid
func (a *AMMBid) Apply(ctx *tx.ApplyContext) tx.Result {
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

	// Get bidder's LP token balance (simplified)
	lpTokens := uint64(1000000) // Placeholder - would read from trustline

	// Parse bid amounts
	var bidMin, bidMax uint64
	if a.BidMin != nil {
		bidMin = parseAmountFromTx(a.BidMin)
		if bidMin > lpTokens || bidMin >= lptAMMBalance {
			return tx.TecAMM_INVALID_TOKENS
		}
	}
	if a.BidMax != nil {
		bidMax = parseAmountFromTx(a.BidMax)
		if bidMax > lpTokens || bidMax >= lptAMMBalance {
			return tx.TecAMM_INVALID_TOKENS
		}
	}
	if bidMin > 0 && bidMax > 0 && bidMin > bidMax {
		return tx.TecAMM_INVALID_TOKENS
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
			return tx.TecAMM_FAILED
		}
	} else if bidMin > 0 {
		// Only min specified
		payPrice = math.Max(computedPrice, float64(bidMin))
	} else if bidMax > 0 {
		// Only max specified
		if computedPrice <= float64(bidMax) {
			payPrice = computedPrice
		} else {
			return tx.TecAMM_FAILED
		}
	} else {
		// Neither specified - pay computed price
		payPrice = computedPrice
	}

	// Check bidder has enough tokens
	if uint64(payPrice) > lpTokens {
		return tx.TecAMM_INVALID_TOKENS
	}

	// Calculate refund and burn amounts
	var refund float64 = 0.0
	var burn float64 = payPrice

	if validOwner && timeSlot != nil {
		// Refund previous owner
		refund = fractionRemaining * pricePurchased
		if refund > payPrice {
			return tx.TefINTERNAL // Should not happen
		}
		burn = payPrice - refund

		// Transfer refund to previous owner
		// In full implementation, would use accountSend
		_ = refund
	}

	// Burn tokens (reduce LP balance)
	burnAmount := uint64(burn)
	if burnAmount >= lptAMMBalance {
		return tx.TefINTERNAL
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
			authAccountID, err := sle.DecodeAccountID(authAccountEntry.AuthAccount.Account)
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
		return tx.TefINTERNAL
	}
	// Update tracked automatically by ApplyStateTable
	if err := ctx.View.Update(ammKey, ammBytes); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}
