// Package offer implements the OfferCreate and OfferCancel transactions.
// Reference: rippled CreateOffer.cpp, CancelOffer.cpp
package offer

import (
	"errors"
	"math/big"
	"strings"

	"github.com/LeJamon/goXRPLd/internal/core/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/payment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

// OfferCreate flag mask - invalid flags
// Reference: rippled TxFlags.h
const (
	// Universal transaction flags (valid on all transaction types)
	// Reference: rippled TxFlags.h lines 60-62
	tfFullyCanonicalSig uint32 = 0x80000000
	tfInnerBatchTxn     uint32 = 0x40000000
	tfUniversal         uint32 = tfFullyCanonicalSig | tfInnerBatchTxn // 0xC0000000

	// tfHybrid flag for permissioned DEX hybrid offers
	tfHybrid uint32 = 0x00100000

	// tfOfferCreateMask is the mask for INVALID flags.
	// Any flags set in this mask are invalid for OfferCreate.
	// Reference: rippled TxFlags.h lines 103-104
	// tfOfferCreateMask = ~(tfUniversal | tfPassive | tfImmediateOrCancel | tfFillOrKill | tfSell | tfHybrid)
	// Valid flags: 0xC0000000 | 0x00010000 | 0x00020000 | 0x00040000 | 0x00080000 | 0x00100000 = 0xC01F0000
	// Mask for invalid: ~0xC01F0000 = 0x3FE0FFFF
	tfOfferCreateMask uint32 = 0x3FE0FFFF
)

// Quality constants
const (
	maxTickSize uint8 = 15
)

// OfferCreate places an offer on the decentralized exchange.
type OfferCreate struct {
	tx.BaseTx

	// TakerGets is the amount and currency the offer creator receives (required)
	TakerGets tx.Amount `json:"TakerGets" xrpl:"TakerGets,amount"`

	// TakerPays is the amount and currency the offer creator pays (required)
	TakerPays tx.Amount `json:"TakerPays" xrpl:"TakerPays,amount"`

	// Expiration is the time when the offer expires (optional)
	Expiration *uint32 `json:"Expiration,omitempty" xrpl:"Expiration,omitempty"`

	// OfferSequence is the sequence number of an offer to cancel (optional)
	OfferSequence *uint32 `json:"OfferSequence,omitempty" xrpl:"OfferSequence,omitempty"`

	// DomainID is the permissioned domain for hybrid offers (optional, requires PermissionedDEX amendment)
	DomainID *[32]byte `json:"DomainID,omitempty" xrpl:"DomainID,omitempty"`
}

func init() {
	tx.Register(tx.TypeOfferCreate, func() tx.Transaction {
		return &OfferCreate{BaseTx: *tx.NewBaseTx(tx.TypeOfferCreate, "")}
	})
}

// NewOfferCreate creates a new OfferCreate transaction
func NewOfferCreate(account string, takerGets, takerPays tx.Amount) *OfferCreate {
	return &OfferCreate{
		BaseTx:    *tx.NewBaseTx(tx.TypeOfferCreate, account),
		TakerGets: takerGets,
		TakerPays: takerPays,
	}
}

// TxType returns the transaction type
func (o *OfferCreate) TxType() tx.Type {
	return tx.TypeOfferCreate
}

// Validate performs minimal validation to ensure the transaction struct is valid.
// This is called by the engine before Preflight. It only checks that required
// fields are present - all semantic validation is done in Preflight().
func (o *OfferCreate) Validate() error {
	if err := o.BaseTx.Validate(); err != nil {
		return err
	}

	// Check required fields are present
	if o.TakerGets.IsZero() {
		return errors.New("temBAD_OFFER: TakerGets is required")
	}
	if o.TakerPays.IsZero() {
		return errors.New("temBAD_OFFER: TakerPays is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (o *OfferCreate) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(o)
}

// SetPassive makes the offer passive
func (o *OfferCreate) SetPassive() {
	flags := o.GetFlags() | OfferCreateFlagPassive
	o.SetFlags(flags)
}

// SetImmediateOrCancel makes the offer immediate-or-cancel
func (o *OfferCreate) SetImmediateOrCancel() {
	flags := o.GetFlags() | OfferCreateFlagImmediateOrCancel
	o.SetFlags(flags)
}

// SetFillOrKill makes the offer fill-or-kill
func (o *OfferCreate) SetFillOrKill() {
	flags := o.GetFlags() | OfferCreateFlagFillOrKill
	o.SetFlags(flags)
}

// parsePreflightError converts a preflight error message to the appropriate TER code.
// Reference: rippled uses specific TER codes for different validation failures.
func parsePreflightError(err error) tx.Result {
	if err == nil {
		return tx.TesSUCCESS
	}
	msg := err.Error()

	// Map error message prefixes to result codes
	prefixes := []struct {
		prefix string
		result tx.Result
	}{
		{"temDISABLED", tx.TemDISABLED},
		{"temINVALID_FLAG", tx.TemINVALID_FLAG},
		{"temBAD_EXPIRATION", tx.TemBAD_EXPIRATION},
		{"temBAD_SEQUENCE", tx.TemBAD_SEQUENCE},
		{"temBAD_AMOUNT", tx.TemBAD_AMOUNT},
		{"temBAD_OFFER", tx.TemBAD_OFFER},
		{"temREDUNDANT", tx.TemREDUNDANT},
		{"temBAD_CURRENCY", tx.TemBAD_CURRENCY},
		{"temBAD_ISSUER", tx.TemBAD_ISSUER},
	}

	for _, p := range prefixes {
		if strings.HasPrefix(msg, p.prefix) {
			return p.result
		}
	}

	return tx.TemMALFORMED
}

// Apply applies an OfferCreate transaction to the ledger state.
// This implements the full rippled CreateOffer flow:
// 1. Preflight validation (with amendment rules)
// 2. Preclaim checks (frozen assets, funds, authorization)
// 3. Offer crossing via flow engine
// 4. Offer placement if not fully filled
// Reference: rippled CreateOffer.cpp doApply()
func (o *OfferCreate) Apply(ctx *tx.ApplyContext) tx.Result {

	// Run preflight validation with amendment rules
	// Reference: rippled CreateOffer.cpp preflight()
	if err := o.Preflight(ctx.Rules()); err != nil {
		// Convert preflight error to appropriate TER code
		return parsePreflightError(err)
	}

	// Run preclaim checks (frozen assets, authorization, funds, etc.)
	// Reference: rippled CreateOffer.cpp preclaim()
	result := o.Preclaim(ctx)
	if result != tx.TesSUCCESS {
		return result
	}

	// Run the main apply logic
	// Reference: rippled CreateOffer.cpp applyGuts()
	return o.ApplyCreate(ctx)
}

// badCurrency returns the "bad" currency code - using XRP as a non-native currency code
// Reference: rippled protocol/Issue.h badCurrency()
func badCurrency() string {
	return "XRP"
}

// Preflight performs all validation on the OfferCreate transaction.
// This matches rippled's preflight() which does ALL semantic validation.
// Reference: rippled CreateOffer.cpp preflight() lines 46-140
func (o *OfferCreate) Preflight(rules *amendment.Rules) error {
	// Check if DomainID field is present without PermissionedDEX amendment
	// Reference: lines 49-51
	if o.DomainID != nil && !rules.PermissionedDEXEnabled() {
		return errors.New("temDISABLED: DomainID requires PermissionedDEX amendment")
	}

	// Check for invalid flags
	// Reference: lines 61-65
	flags := o.GetFlags()
	if flags&tfOfferCreateMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags set")
	}

	// Check tfHybrid without PermissionedDEX
	// Reference: lines 67-68
	if !rules.PermissionedDEXEnabled() && (flags&tfHybrid != 0) {
		return errors.New("temINVALID_FLAG: tfHybrid requires PermissionedDEX amendment")
	}

	// tfHybrid requires DomainID
	// Reference: lines 70-71
	if (flags&tfHybrid != 0) && o.DomainID == nil {
		return errors.New("temINVALID_FLAG: tfHybrid requires DomainID")
	}

	// IoC and FoK are mutually exclusive
	// Reference: lines 73-80
	bImmediateOrCancel := (flags & OfferCreateFlagImmediateOrCancel) != 0
	bFillOrKill := (flags & OfferCreateFlagFillOrKill) != 0
	if bImmediateOrCancel && bFillOrKill {
		return errors.New("temINVALID_FLAG: cannot set both ImmediateOrCancel and FillOrKill")
	}

	// Check expiration
	// Reference: lines 82-88
	if o.Expiration != nil && *o.Expiration == 0 {
		return errors.New("temBAD_EXPIRATION: expiration cannot be zero")
	}

	// Check OfferSequence
	// Reference: lines 90-95
	if o.OfferSequence != nil && *o.OfferSequence == 0 {
		return errors.New("temBAD_SEQUENCE: OfferSequence cannot be zero")
	}

	// Validate amounts
	saTakerPays := o.TakerPays
	saTakerGets := o.TakerGets

	// Check amounts are present and valid
	// Reference: lines 97-101
	if !isLegalNetAmount(saTakerPays) || !isLegalNetAmount(saTakerGets) {
		return errors.New("temBAD_AMOUNT: invalid amount")
	}

	// Cannot exchange XRP for XRP
	// Reference: lines 103-107
	if saTakerPays.IsNative() && saTakerGets.IsNative() {
		return errors.New("temBAD_OFFER: cannot exchange XRP for XRP")
	}

	// Amounts must be positive
	// Reference: lines 108-112
	if isAmountZeroOrNegative(saTakerPays) || isAmountZeroOrNegative(saTakerGets) {
		return errors.New("temBAD_OFFER: amounts must be positive")
	}

	// Get currency and issuer info
	uPaysCurrency := saTakerPays.Currency
	uPaysIssuerID := saTakerPays.Issuer
	uGetsCurrency := saTakerGets.Currency
	uGetsIssuerID := saTakerGets.Issuer

	// Check for redundant offer (same currency and issuer)
	// Reference: lines 120-124
	if uPaysCurrency == uGetsCurrency && uPaysIssuerID == uGetsIssuerID {
		return errors.New("temREDUNDANT: cannot create offer with same currency and issuer on both sides")
	}

	// Check for bad currency (XRP as non-native currency code)
	// Reference: lines 126-130
	if !saTakerPays.IsNative() && uPaysCurrency == badCurrency() {
		return errors.New("temBAD_CURRENCY: cannot use XRP as non-native currency code")
	}
	if !saTakerGets.IsNative() && uGetsCurrency == badCurrency() {
		return errors.New("temBAD_CURRENCY: cannot use XRP as non-native currency code")
	}

	// Check issuer consistency
	// Reference: lines 132-137
	// Native amounts must have no issuer, non-native must have issuer
	if saTakerPays.IsNative() != (uPaysIssuerID == "") {
		return errors.New("temBAD_ISSUER: issuer mismatch for TakerPays")
	}
	if saTakerGets.IsNative() != (uGetsIssuerID == "") {
		return errors.New("temBAD_ISSUER: issuer mismatch for TakerGets")
	}

	return nil
}

// Preclaim validates the transaction against ledger state before application.
// Reference: rippled CreateOffer.cpp preclaim() lines 142-225
func (o *OfferCreate) Preclaim(ctx *tx.ApplyContext) tx.Result {
	rules := ctx.Rules()

	saTakerPays := o.TakerPays
	saTakerGets := o.TakerGets

	uPaysIssuerID := saTakerPays.Issuer
	uGetsIssuerID := saTakerGets.Issuer

	// Check global freeze on both issuers
	// Reference: lines 165-170
	if uPaysIssuerID != "" {
		if tx.IsGlobalFrozen(ctx.View, uPaysIssuerID) {
			return tx.TecFROZEN
		}
	}
	if uGetsIssuerID != "" {
		if tx.IsGlobalFrozen(ctx.View, uGetsIssuerID) {
			return tx.TecFROZEN
		}
	}

	// Check account has funds for the offer
	// Reference: lines 172-178
	funds := tx.AccountFunds(ctx.View, ctx.AccountID, saTakerGets, true)
	diff := sle.SubtractAmount(saTakerGets, funds)
	if diff.Signum() > 0 {
		return tx.TecUNFUNDED_OFFER
	}

	// Check cancel sequence is valid (must be less than current account sequence)
	// Reference: lines 182-187
	if o.OfferSequence != nil {
		if ctx.Account.Sequence <= *o.OfferSequence {
			return tx.TemBAD_SEQUENCE
		}
	}

	// Check if offer has expired
	// Reference: lines 189-200
	if hasExpired(ctx, o.Expiration) {
		if rules.DepositPreauthEnabled() {
			return tx.TecEXPIRED
		}
		return tx.TesSUCCESS
	}

	// Check we can accept what the taker will pay us (for non-native)
	// Reference: lines 203-213
	if !saTakerPays.IsNative() {
		paysIssuerID, err := sle.DecodeAccountID(uPaysIssuerID)
		if err != nil {
			return tx.TecNO_ISSUER
		}
		result := checkAcceptAsset(ctx, paysIssuerID, saTakerPays.Currency, rules)
		if result != tx.TesSUCCESS {
			return result
		}
	}

	// Check domain membership if DomainID is specified
	// Reference: lines 217-222
	if o.DomainID != nil {
		if !accountInDomain(ctx.View, ctx.AccountID, *o.DomainID) {
			return tx.TecNO_PERMISSION
		}
	}

	return tx.TesSUCCESS
}

// ApplyCreate applies the OfferCreate transaction to the ledger.
// This is the main entry point called by the engine.
// Reference: rippled CreateOffer.cpp doApply() lines 932-949
//
// This implements the two-sandbox pattern for FillOrKill (FoK) offers:
// - sb: main sandbox for crossing and offer placement
// - sbCancel: cancel sandbox for offer cancellation only
//
// For FoK offers that don't fully fill, we apply sbCancel instead of sb,
// ensuring the cancellation happens but the crossing changes are discarded.
func (o *OfferCreate) ApplyCreate(ctx *tx.ApplyContext) tx.Result {
	// Create TWO independent sandboxes from ctx.View
	// Reference: rippled CreateOffer.cpp lines 938-941
	sb := payment.NewPaymentSandbox(ctx.View)
	sbCancel := payment.NewPaymentSandbox(ctx.View)

	// Set transaction context on both sandboxes
	sb.SetTransactionContext(ctx.TxHash, ctx.Config.LedgerSequence)
	sbCancel.SetTransactionContext(ctx.TxHash, ctx.Config.LedgerSequence)

	// Execute applyGuts with both sandboxes
	result, applyMain := o.applyGuts(ctx, sb, sbCancel)

	// Apply the correct sandbox to the ledger view
	if applyMain {
		if err := sb.ApplyToView(ctx.View); err != nil {
			return tx.TefINTERNAL
		}
	} else {
		if err := sbCancel.ApplyToView(ctx.View); err != nil {
			return tx.TefINTERNAL
		}
	}

	return result
}

// applyGuts contains the main offer creation logic with two-sandbox pattern.
// Reference: rippled CreateOffer.cpp applyGuts() lines 576-929
//
// The two-sandbox pattern ensures FillOrKill offers that don't fully fill
// only apply the cancellation changes, not the crossing changes.
//
// Parameters:
//   - ctx: the apply context
//   - sb: main sandbox for crossing and offer placement
//   - sbCancel: cancel sandbox for offer cancellation only
//
// Returns:
//   - result: the transaction result code
//   - applyMain: true to apply sb, false to apply sbCancel
func (o *OfferCreate) applyGuts(ctx *tx.ApplyContext, sb, sbCancel *payment.PaymentSandbox) (tx.Result, bool) {
	rules := ctx.Rules()

	flags := o.GetFlags()
	bPassive := (flags & OfferCreateFlagPassive) != 0
	bImmediateOrCancel := (flags & OfferCreateFlagImmediateOrCancel) != 0
	bFillOrKill := (flags & OfferCreateFlagFillOrKill) != 0
	bSell := (flags & OfferCreateFlagSell) != 0
	bHybrid := (flags & tfHybrid) != 0

	saTakerPays := o.TakerPays
	saTakerGets := o.TakerGets

	// Calculate the original rate (quality) for the offer
	// Reference: line 601
	uRate := sle.GetRate(saTakerGets, saTakerPays)
	result := tx.TesSUCCESS

	// Process cancellation request if specified
	// Reference: lines 608-621
	// CRITICAL: Offer cancellation must happen in BOTH sandboxes
	if o.OfferSequence != nil {
		sleCancel := peekOffer(ctx.View, ctx.AccountID, *o.OfferSequence)
		if sleCancel != nil {
			// Delete in main sandbox
			result = offerDeleteInView(sb, sleCancel)
			// Delete in cancel sandbox (same operation)
			_ = offerDeleteInView(sbCancel, sleCancel)

			// Also update owner count (once, since we'll only apply one sandbox)
			if result == tx.TesSUCCESS && ctx.Account.OwnerCount > 0 {
				ctx.Account.OwnerCount--
			}
		}
	}

	// Check if offer has expired
	// Reference: lines 623-636
	if hasExpired(ctx, o.Expiration) {
		if rules.DepositPreauthEnabled() {
			return tx.TecEXPIRED, false // Apply cancel sandbox for expired offers
		}
		return tx.TesSUCCESS, true
	}

	crossed := false

	if result == tx.TesSUCCESS {
		// Apply tick size rounding if applicable
		// Reference: lines 643-685
		saTakerPays, saTakerGets = applyTickSize(ctx.View, saTakerPays, saTakerGets, bSell, rules)
		if isAmountZeroOrNegative(saTakerPays) || isAmountZeroOrNegative(saTakerGets) {
			// Offer rounded to zero
			return tx.TesSUCCESS, true
		}

		// Recalculate rate after tick size
		uRate = sle.GetRate(saTakerGets, saTakerPays)

		// Perform offer crossing using the main sandbox (sb)
		// Reference: lines 687-768
		// Note: Passive offers still cross, but only against offers with STRICTLY better quality.
		// The passive flag is passed to FlowCross which increments the quality threshold.
		// Reference: rippled CreateOffer.cpp lines 362-364
		var placeOffer struct {
			in  tx.Amount
			out tx.Amount
		}

		// FlowCross operates on the main sandbox (sb)
		crossResult := payment.FlowCross(
			sb, // Use main sandbox for crossing
			ctx.AccountID,
			saTakerGets, // What we're selling (taker pays to counterparty)
			saTakerPays, // What we want (taker receives from counterparty)
			ctx.TxHash,
			ctx.Config.LedgerSequence,
			bPassive, // For passive offers, only cross against strictly better quality
		)

		// Convert result amounts back
		// For remaining offer calculation:
		// - If GROSS >= originalTakerGets: offer fully consumed, no remaining
		// - Else: remaining = originalTakerGets - NET (use net delivered amount)
		// Reference: rippled uses the net amount delivered to matching offers for remaining calculation
		grossPaid := payment.FromEitherAmount(crossResult.TakerPaid)
		netPaid := payment.FromEitherAmount(crossResult.TakerPaidNet)

		// Use GROSS for the "fully consumed" check via subtractAmounts clamping
		// But for actual remaining calculation, use NET when we have a remaining offer
		// The subtractAmounts function clamps negative to zero, so:
		// - If GROSS >= original: remaining = 0 (no offer created)
		// - If GROSS < original: we need remaining = original - NET
		remainingWithGross := subtractAmounts(saTakerGets, grossPaid)
		if isAmountZeroOrNegative(remainingWithGross) {
			// Offer fully consumed with GROSS, use GROSS
			placeOffer.in = grossPaid
		} else {
			// Offer has remainder, use NET for accurate remaining calculation
			placeOffer.in = netPaid
		}
		placeOffer.out = payment.FromEitherAmount(crossResult.TakerGot) // What we received

		result = crossResult.Result

		// For offer crossing, tecPATH_DRY means no liquidity found to cross
		// This is not an error - we just place the offer with original amounts
		// Reference: rippled's flowCross always returns tesSUCCESS (CreateOffer.cpp line 509)
		if result == tx.TecPATH_DRY {
			result = tx.TesSUCCESS
		}

		if result != tx.TesSUCCESS {
			return result, false // Error during crossing - apply cancel sandbox
		}

		// Apply FlowCross sandbox changes to our main sandbox (sb)
		// Reference: rippled CreateOffer.cpp - sandbox changes must be applied
		// FlowCross creates a root sandbox, so we use ApplyToView with sb as the target
		if crossResult.Sandbox != nil {
			if err := crossResult.Sandbox.ApplyToView(sb); err != nil {
				return tx.TefINTERNAL, false
			}
		}

		// Update ctx.Account.Balance to reflect XRP changes during crossing
		// The engine writes ctx.Account after Apply(), so we must update it here
		// Reference: The taker may pay or receive XRP for the crossing
		if placeOffer.in.IsNative() {
			// Taker paid XRP
			paidDrops := uint64(placeOffer.in.Drops())
			if ctx.Account.Balance >= paidDrops {
				ctx.Account.Balance -= paidDrops
			}
		}
		if placeOffer.out.IsNative() {
			// Taker received XRP
			receivedDrops := uint64(placeOffer.out.Drops())
			ctx.Account.Balance += receivedDrops
		}

		// Remove unfunded/bad offers that were marked during crossing
		for offerKey := range crossResult.RemovableOffers {
			offerKeylet := keylet.Keylet{Key: offerKey}
			if err := sb.Erase(offerKeylet); err != nil {
				_ = err // Log but don't fail - cleanup operation
			}
		}

		// Check if account's funds were exhausted during crossing
		// Reference: rippled CreateOffer.cpp lines 432-441
		// If the balance for what we're selling is now zero, don't create the offer
		takerInBalance := tx.AccountFunds(sb, ctx.AccountID, saTakerGets, true)
		if isAmountZeroOrNegative(takerInBalance) {
			// Account funds exhausted - offer fully consumed, no remaining offer to place
			return tx.TesSUCCESS, true // Apply main sandbox with crossing results
		}

		// Check if any crossing happened
		// Reference: line 744-745
		// Use isAmountZeroOrNegative because FromEitherAmount returns "0" for zero amounts,
		// not empty string ""
		if !isAmountZeroOrNegative(placeOffer.in) || !isAmountZeroOrNegative(placeOffer.out) {
			crossed = true
		}

		// Calculate remaining amounts for the new offer
		// Reference: rippled CreateOffer.cpp lines 491-504 (flowCross)
		// Rippled preserves the offer quality when calculating remaining amounts.
		//
		// IMPORTANT: When the offer is fully consumed (GROSS >= originalTakerGets),
		// the taker has paid everything (or more with transfer fees), so NO remaining
		// offer should be created. We use subtraction which will give zero/negative.
		//
		// When no crossing happened at all (placeOffer is zero), return original amounts
		// directly to avoid float64 precision errors in ratio calculation.
		//
		// Only when partial crossing happened do we use quality preservation.
		//
		// For non-sell offers (Flags=0) with remaining:
		//   1. remainingPays = originalTakerPays - actualAmountOut (XRP received)
		//   2. remainingGets = remainingPays * (originalTakerGets / originalTakerPays) (quality preserved)
		//
		// For sell offers (tfSell):
		//   1. remainingGets = originalTakerGets - (actualAmountIn / transferRate) (non-gateway amount)
		//   2. remainingPays = remainingGets * (originalTakerPays / originalTakerGets) (quality preserved)
		var remainingGets, remainingPays tx.Amount

		noCrossingHappened := isAmountZeroOrNegative(placeOffer.in) && isAmountZeroOrNegative(placeOffer.out)

		if isAmountZeroOrNegative(remainingWithGross) {
			// Offer fully consumed - taker paid everything (GROSS >= original)
			// Use subtraction which will give zero/negative, triggering "fully crossed" below
			remainingGets = subtractAmounts(saTakerGets, placeOffer.in)
			remainingPays = subtractAmounts(saTakerPays, placeOffer.out)
		} else if noCrossingHappened {
			// No crossing happened - return original amounts directly
			// This avoids float64 precision errors in ratio calculation
			remainingGets = saTakerGets
			remainingPays = saTakerPays
		} else if bSell {
			// Sell offer with remaining: subtract from TakerGets, calculate TakerPays by ratio
			// Reference: rippled CreateOffer.cpp lines 474-489
			// The fixReducedOffersV1 amendment changes the rounding direction:
			// - Before: round UP (divRound with roundUp=true)
			// - After: round DOWN (divRoundStrict with roundUp=false)
			remainingGets = subtractAmounts(saTakerGets, placeOffer.in) // placeOffer.in is NET
			roundUp := !rules.Enabled(amendment.FeatureFixReducedOffersV1)
			remainingPays = multiplyByRatio(remainingGets, o.TakerPays, o.TakerGets, roundUp)
		} else {
			// Non-sell offer with remaining: subtract from TakerPays, calculate TakerGets by ratio
			// For non-sell offers, always round up
			remainingPays = subtractAmounts(saTakerPays, placeOffer.out)
			remainingGets = multiplyByRatio(remainingPays, o.TakerGets, o.TakerPays, true)
		}

		// Check if offer is fully filled
		// Reference: lines 757-761
		if isAmountZeroOrNegative(remainingGets) || isAmountZeroOrNegative(remainingPays) {
			// Offer fully crossed - FoK is satisfied
			return tx.TesSUCCESS, true
		}

		// Adjust amounts for remaining offer
		// Reference: lines 766-767
		saTakerPays = remainingPays
		saTakerGets = remainingGets
	}

	// Sanity check: amounts should be positive
	if isAmountZeroOrNegative(saTakerPays) || isAmountZeroOrNegative(saTakerGets) {
		return tx.TefINTERNAL, false
	}

	if result != tx.TesSUCCESS {
		return result, false
	}

	// Handle FillOrKill - offer was NOT fully filled if we reach here
	// Reference: lines 789-795
	// CRITICAL: For FoK, apply sbCancel to discard crossing changes
	if bFillOrKill {
		if rules.Enabled(amendment.FeatureFix1578) {
			return tx.TecKILLED, false // Apply cancel sandbox
		}
		return tx.TesSUCCESS, false // Pre-amendment: still apply cancel sandbox
	}

	// Handle ImmediateOrCancel
	// Reference: lines 799-809
	if bImmediateOrCancel {
		if !crossed && rules.Enabled(amendment.FeatureImmediateOfferKilled) {
			return tx.TecKILLED, false // No crossing - apply cancel sandbox
		}
		return tx.TesSUCCESS, true // Crossing happened - apply main sandbox
	}

	// Check reserve for new offer
	// Reference: lines 815-834
	reserve := ctx.AccountReserve(ctx.Account.OwnerCount + 1)
	priorBalance := ctx.Account.Balance + parseFee(ctx)
	if priorBalance < reserve {
		if !crossed {
			return tx.TecINSUF_RESERVE_OFFER, true
		}
		return tx.TesSUCCESS, true
	}

	// Create the offer in the ledger (in main sandbox)
	// Reference: lines 837-925
	offerSequence := o.getOfferSequence()
	offerKey := keylet.Offer(ctx.AccountID, offerSequence)

	// Calculate book directory fields first (needed for both owner and book directories
	// when SortedDirectories is not enabled)
	// Reference: lines 857-887
	takerPaysCurrency := sle.GetCurrencyBytes(saTakerPays.Currency)
	takerPaysIssuer := sle.GetIssuerBytes(saTakerPays.Issuer)
	takerGetsCurrency := sle.GetCurrencyBytes(saTakerGets.Currency)
	takerGetsIssuer := sle.GetIssuerBytes(saTakerGets.Issuer)

	bookBase := keylet.BookDir(takerPaysCurrency, takerPaysIssuer, takerGetsCurrency, takerGetsIssuer)
	bookDirKey := keylet.Quality(bookBase, uRate)

	// Add to owner directory
	// Reference: lines 839-848
	ownerDirKey := keylet.OwnerDir(ctx.AccountID)
	ownerDirResult, err := sle.DirInsert(sb, ownerDirKey, offerKey.Key, func(dir *sle.DirectoryNode) {
		dir.Owner = ctx.AccountID
	})
	if err != nil {
		return tx.TefINTERNAL, false
	}

	// Increment owner count
	// Reference: line 851
	ctx.Account.OwnerCount++

	// Check if book exists (for OrderBookDB tracking)
	bookExisted, _ := sb.Exists(bookDirKey)

	// Add to book directory
	// Reference: lines 884-893
	bookDirResult, err := sle.DirInsert(sb, bookDirKey, offerKey.Key, func(dir *sle.DirectoryNode) {
		dir.TakerPaysCurrency = takerPaysCurrency
		dir.TakerPaysIssuer = takerPaysIssuer
		dir.TakerGetsCurrency = takerGetsCurrency
		dir.TakerGetsIssuer = takerGetsIssuer
		dir.ExchangeRate = uRate
		// Note: DomainID is stored on the offer itself, not the directory
	})
	if err != nil {
		return tx.TefINTERNAL, false
	}

	// Create the offer SLE
	// Reference: lines 895-910
	ledgerOffer := &sle.LedgerOffer{
		Account:           ctx.Account.Account,
		Sequence:          offerSequence,
		TakerPays:         saTakerPays,
		TakerGets:         saTakerGets,
		BookDirectory:     bookDirKey.Key,
		BookNode:          bookDirResult.Page,
		OwnerNode:         ownerDirResult.Page,
		Flags:             0,
		PreviousTxnID:     ctx.TxHash,
		PreviousTxnLgrSeq: ctx.Config.LedgerSequence,
	}

	// Set expiration if specified
	// Reference: line 903-904
	if o.Expiration != nil {
		ledgerOffer.Expiration = *o.Expiration
	}

	// Set offer flags
	// Reference: lines 905-910
	if bPassive {
		ledgerOffer.Flags |= lsfOfferPassive
	}
	if bSell {
		ledgerOffer.Flags |= lsfOfferSell
	}

	// Set DomainID if specified
	if o.DomainID != nil {
		ledgerOffer.DomainID = *o.DomainID
	}

	// Handle hybrid offers
	// Reference: lines 912-919
	if bHybrid {
		result = applyHybridInSandbox(sb, ctx, ledgerOffer, offerKey, saTakerPays, saTakerGets, bookDirKey)
		if result != tx.TesSUCCESS {
			return result, false
		}
	}

	// Serialize and store the offer
	offerData, err := sle.SerializeLedgerOffer(ledgerOffer)
	if err != nil {
		return tx.TefINTERNAL, false
	}

	if err := sb.Insert(offerKey, offerData); err != nil {
		return tx.TefINTERNAL, false
	}

	// Track new book in OrderBookDB (not implemented yet)
	_ = bookExisted

	return tx.TesSUCCESS, true // Apply main sandbox
}

// ============================================================================
// Helper functions
// ============================================================================

// isLegalNetAmount checks if an amount is a valid net amount.
// Reference: rippled protocol/STAmount.h isLegalNet()
func isLegalNetAmount(amt tx.Amount) bool {
	// A legal net amount is non-zero
	return !amt.IsZero()
}

// isAmountZeroOrNegative checks if an amount is zero or negative.
func isAmountZeroOrNegative(amt tx.Amount) bool {
	return amt.IsZero() || amt.IsNegative()
}

// isAmountEmpty checks if an amount is empty/unset.
func isAmountEmpty(amt tx.Amount) bool {
	return amt.IsZero()
}

// subtractAmounts subtracts b from a.
// a - b = result
func subtractAmounts(a, b tx.Amount) tx.Amount {
	result, err := a.Sub(b)
	if err != nil {
		// Type mismatch - return zero amount of a's type
		if a.IsNative() {
			return tx.NewXRPAmount(0)
		}
		return tx.NewIssuedAmount(0, -100, a.Currency, a.Issuer)
	}

	// Clamp negative results to zero
	if result.IsNegative() {
		if result.IsNative() {
			return tx.NewXRPAmount(0)
		}
		return tx.NewIssuedAmount(0, -100, a.Currency, a.Issuer)
	}

	return result
}

// multiplyByRatioRoundUp calculates a * (num / den) with rounding up.
// Convenience wrapper for multiplyByRatio with roundUp=true.
func multiplyByRatioRoundUp(a, num, den tx.Amount) tx.Amount {
	return multiplyByRatio(a, num, den, true)
}

// multiplyByRatioRoundDown calculates a * (num / den) with rounding down.
// Convenience wrapper for multiplyByRatio with roundUp=false.
func multiplyByRatioRoundDown(a, num, den tx.Amount) tx.Amount {
	return multiplyByRatio(a, num, den, false)
}

// multiplyByRatio calculates a * (num / den), preserving quality.
// This is used to calculate remaining offer amounts while preserving the offer quality.
// Reference: rippled CreateOffer.cpp lines 502-503: afterCross.in = mulRound(afterCross.out, rate, ...)
// where rate = takerAmount.in / takerAmount.out (TakerGets / TakerPays)
//
// The result type is determined by `num` (the numerator), which represents the original amount
// we're trying to calculate the remaining for.
//
// For non-sell offers: remainingGets = remainingPays * (originalGets / originalPays)
//   - a = remainingPays (XRP), num = originalGets (IOU), den = originalPays (XRP)
//   - result = IOU (same type as num)
//
// For sell offers: remainingPays = remainingGets * (originalPays / originalGets)
//   - a = remainingGets (IOU), num = originalPays (XRP), den = originalGets (IOU)
//   - result = XRP (same type as num)
//
// The roundUp parameter controls rounding direction:
//   - true: round up (used before fixReducedOffersV1 for sell offers, and for non-sell offers)
//   - false: round down (used after fixReducedOffersV1 for sell offers)
//
// Reference: rippled CreateOffer.cpp lines 474-489 - fixReducedOffersV1 changes rounding direction
//
// Uses big.Rat for precise rational arithmetic to avoid floating-point precision issues.
func multiplyByRatio(a, num, den tx.Amount, roundUp bool) tx.Amount {
	// Handle zero denominator
	if den.IsZero() {
		if num.IsNative() {
			return tx.NewXRPAmount(0)
		}
		return tx.NewIssuedAmount(0, -100, num.Currency, num.Issuer)
	}

	// Handle zero inputs
	if a.IsZero() || num.IsZero() {
		if num.IsNative() {
			return tx.NewXRPAmount(0)
		}
		return tx.NewIssuedAmount(0, -100, num.Currency, num.Issuer)
	}

	// Use big.Rat for precise rational arithmetic: result = a * num / den
	// Convert each amount to a rational number

	// Helper to convert Amount to big.Rat
	toRat := func(amt tx.Amount) *big.Rat {
		if amt.IsNative() {
			// XRP: value in drops
			return new(big.Rat).SetInt64(amt.Drops())
		}
		// IOU: value = mantissa * 10^exponent
		mantissa := amt.Mantissa()
		exponent := amt.Exponent()

		// Create rational = mantissa / (10^-exponent) or mantissa * (10^exponent)
		rat := new(big.Rat).SetInt64(mantissa)
		if exponent > 0 {
			// Multiply by 10^exponent
			scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
			rat.Mul(rat, new(big.Rat).SetInt(scale))
		} else if exponent < 0 {
			// Divide by 10^(-exponent)
			scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exponent)), nil)
			rat.Quo(rat, new(big.Rat).SetInt(scale))
		}
		return rat
	}

	aRat := toRat(a)
	numRat := toRat(num)
	denRat := toRat(den)

	// Compute result = a * num / den
	result := new(big.Rat).Mul(aRat, numRat)
	result.Quo(result, denRat)

	// Convert result back to appropriate Amount type
	if num.IsNative() {
		// Result should be XRP (drops)
		// Get the float value and convert to drops
		f, _ := result.Float64()
		drops := int64(f)
		if roundUp && f > float64(drops) {
			drops++
		}
		return tx.NewXRPAmount(drops)
	}

	// Result should be IOU with num's currency and issuer
	// Convert big.Rat to mantissa/exponent form
	// Target: mantissa in range [10^15, 10^16) with appropriate exponent

	// Get the result as a decimal string with high precision
	f, _ := result.Float64()

	// For better precision, use the rational representation
	// Calculate mantissa and exponent
	if f == 0 {
		return tx.NewIssuedAmount(0, -100, num.Currency, num.Issuer)
	}

	// Determine sign
	negative := f < 0
	if negative {
		f = -f
		result.Neg(result)
	}

	// Scale to get mantissa in [10^15, 10^16)
	exponent := 0
	// Work with high precision by scaling the rational
	minMant := big.NewInt(1000000000000000)  // 10^15
	maxMant := big.NewInt(10000000000000000) // 10^16

	// Scale result to integer range
	scaled := new(big.Rat).Set(result)
	for {
		intPart := new(big.Int)
		intPart.Quo(scaled.Num(), scaled.Denom())
		if intPart.Cmp(maxMant) >= 0 {
			// Too large, divide by 10
			scaled.Quo(scaled, big.NewRat(10, 1))
			exponent++
		} else if intPart.Cmp(minMant) < 0 {
			// Too small (including zero), multiply by 10
			// But only if the scaled value itself is non-zero
			if scaled.Sign() == 0 {
				break
			}
			scaled.Mul(scaled, big.NewRat(10, 1))
			exponent--
		} else {
			// intPart is in range [minMant, maxMant)
			break
		}
		// Safety limit
		if exponent > 80 || exponent < -96 {
			break
		}
	}

	// Get final mantissa with rounding
	intPart := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	remainder := new(big.Int).Mod(scaled.Num(), scaled.Denom())

	// Round if needed
	if roundUp && remainder.Sign() != 0 {
		intPart.Add(intPart, big.NewInt(1))
	} else if !roundUp && remainder.Sign() != 0 {
		// Check if we should round (banker's rounding for >= 0.5)
		doubled := new(big.Int).Mul(remainder, big.NewInt(2))
		if doubled.Cmp(scaled.Denom()) >= 0 {
			// Don't round up for roundUp=false
		}
	}

	mantissa := intPart.Int64()
	if negative {
		mantissa = -mantissa
	}

	return tx.NewIssuedAmount(mantissa, exponent, num.Currency, num.Issuer)
}

// hasExpired checks if an offer has expired.
// Reference: rippled app/tx/impl/Transactor.cpp hasExpired()
func hasExpired(ctx *tx.ApplyContext, expiration *uint32) bool {
	if expiration == nil {
		return false
	}
	return *expiration <= ctx.Config.ParentCloseTime
}

// checkAcceptAsset validates that an account can receive an asset.
// Reference: rippled CreateOffer.cpp checkAcceptAsset() lines 227-312
func checkAcceptAsset(ctx *tx.ApplyContext, issuerID [20]byte, currency string, rules *amendment.Rules) tx.Result {
	// Read issuer account
	issuerKey := keylet.Account(issuerID)
	issuerData, err := ctx.View.Read(issuerKey)
	if err != nil || issuerData == nil {
		return tx.TecNO_ISSUER
	}

	issuerAccount, err := sle.ParseAccountRoot(issuerData)
	if err != nil {
		return tx.TecNO_ISSUER
	}

	// If account is the issuer, always allowed
	// Reference: lines 254-256
	if rules.DepositPreauthEnabled() && ctx.AccountID == issuerID {
		return tx.TesSUCCESS
	}

	// Check RequireAuth flag on issuer
	// Reference: lines 258-282
	if (issuerAccount.Flags & sle.LsfRequireAuth) != 0 {
		trustLineKey := keylet.Line(ctx.AccountID, issuerID, currency)
		trustLineData, err := ctx.View.Read(trustLineKey)
		if err != nil || trustLineData == nil {
			return tx.TecNO_LINE
		}

		rs, err := sle.ParseRippleState(trustLineData)
		if err != nil {
			return tx.TecNO_LINE
		}

		// Check authorization based on canonical ordering
		canonicalGT := sle.CompareAccountIDsForLine(ctx.AccountID, issuerID) > 0
		var isAuthorized bool
		if canonicalGT {
			isAuthorized = (rs.Flags & sle.LsfLowAuth) != 0
		} else {
			isAuthorized = (rs.Flags & sle.LsfHighAuth) != 0
		}

		if !isAuthorized {
			return tx.TecNO_AUTH
		}
	}

	// If account is issuer, always allowed (redundant check but matches rippled)
	// Reference: lines 288-291
	if ctx.AccountID == issuerID {
		return tx.TesSUCCESS
	}

	// Check for deep freeze on trustline
	// Reference: lines 293-309
	trustLineKey := keylet.Line(ctx.AccountID, issuerID, currency)
	trustLineData, err := ctx.View.Read(trustLineKey)
	if err != nil || trustLineData == nil {
		// No trustline = OK (will be created if needed)
		return tx.TesSUCCESS
	}

	rs, err := sle.ParseRippleState(trustLineData)
	if err != nil {
		return tx.TesSUCCESS
	}

	// Check deep freeze
	deepFrozen := (rs.Flags & (sle.LsfLowDeepFreeze | sle.LsfHighDeepFreeze)) != 0
	if deepFrozen {
		return tx.TecFROZEN
	}

	return tx.TesSUCCESS
}

// accountInDomain checks if an account is a member of a permissioned domain.
// Reference: rippled app/misc/PermissionedDEXHelpers.cpp accountInDomain()
func accountInDomain(view tx.LedgerView, accountID [20]byte, domainID [32]byte) bool {
	// TODO: Implement domain membership check when PermissionedDomains is fully implemented
	// For now, return true to allow offers (permissioned DEX is not yet active)
	return true
}

// applyTickSize applies tick size rounding to offer amounts.
// Reference: rippled CreateOffer.cpp lines 643-685
func applyTickSize(view tx.LedgerView, takerPays, takerGets tx.Amount, bSell bool, rules *amendment.Rules) (tx.Amount, tx.Amount) {
	// Get tick sizes from both issuers
	tickSize := maxTickSize

	if !takerPays.IsNative() {
		issuerTickSize := getTickSize(view, takerPays.Issuer)
		if issuerTickSize > 0 && issuerTickSize < tickSize {
			tickSize = issuerTickSize
		}
	}

	if !takerGets.IsNative() {
		issuerTickSize := getTickSize(view, takerGets.Issuer)
		if issuerTickSize > 0 && issuerTickSize < tickSize {
			tickSize = issuerTickSize
		}
	}

	// If no tick size applies, return unchanged
	if tickSize >= maxTickSize {
		return takerPays, takerGets
	}

	// Apply tick size rounding
	// Reference: lines 660-685
	quality := sle.CalculateQuality(takerGets, takerPays)
	roundedQuality := roundToTickSize(quality, tickSize)

	if bSell {
		// Round TakerPays
		takerPays = multiplyByQuality(takerGets, roundedQuality, takerPays.Currency, takerPays.Issuer)
	} else {
		// Round TakerGets
		takerGets = divideByQuality(takerPays, roundedQuality, takerGets.Currency, takerGets.Issuer)
	}

	return takerPays, takerGets
}

// getTickSize returns the tick size for an issuer.
func getTickSize(view tx.LedgerView, issuerAddress string) uint8 {
	if issuerAddress == "" {
		return 0
	}

	issuerID, err := sle.DecodeAccountID(issuerAddress)
	if err != nil {
		return 0
	}

	accountKey := keylet.Account(issuerID)
	data, err := view.Read(accountKey)
	if err != nil || data == nil {
		return 0
	}

	account, err := sle.ParseAccountRoot(data)
	if err != nil {
		return 0
	}

	return account.TickSize
}

// roundToTickSize rounds a quality value to the specified tick size.
// Reference: rippled Quality.cpp round() function lines 182-212
// The tick size determines how many significant digits are kept in the mantissa.
// Quality is encoded as: (exponent << 56) | mantissa where mantissa is in [10^15, 10^16)
func roundToTickSize(quality uint64, tickSize uint8) uint64 {
	// If tick size is max or zero, no rounding needed
	if tickSize >= maxTickSize || tickSize == 0 {
		return quality
	}

	// Modulus for mantissa - determines rounding granularity
	// These are powers of 10 that determine rounding precision
	mod := []uint64{
		10000000000000000, // 0: 10^16 (no rounding)
		1000000000000000,  // 1: 10^15
		100000000000000,   // 2: 10^14
		10000000000000,    // 3: 10^13
		1000000000000,     // 4: 10^12
		100000000000,      // 5: 10^11
		10000000000,       // 6: 10^10
		1000000000,        // 7: 10^9
		100000000,         // 8: 10^8
		10000000,          // 9: 10^7
		1000000,           // 10: 10^6
		100000,            // 11: 10^5
		10000,             // 12: 10^4
		1000,              // 13: 10^3
		100,               // 14: 10^2
		10,                // 15: 10^1
		1,                 // 16: 10^0
	}

	// Extract exponent (top 8 bits) and mantissa (lower 56 bits)
	exponent := quality >> 56
	mantissa := quality & 0x00ffffffffffffff

	// Round up: add (mod-1) then truncate
	mantissa += mod[tickSize] - 1
	mantissa -= mantissa % mod[tickSize]

	// Reconstruct quality
	return (exponent << 56) | mantissa
}

// qualityToRate converts a quality value (encoded as (exponent << 56) | mantissa) to a big.Rat.
// Quality encoding: exponent is stored as (actual_exponent + 100) in the top 8 bits,
// mantissa is in the lower 56 bits and is in range [10^15, 10^16).
func qualityToRate(quality uint64) *big.Rat {
	if quality == 0 {
		return new(big.Rat).SetInt64(0)
	}

	// Extract exponent (top 8 bits) and mantissa (lower 56 bits)
	storedExponent := quality >> 56
	mantissa := quality & 0x00ffffffffffffff

	// Actual exponent = storedExponent - 100
	actualExponent := int(storedExponent) - 100

	// Rate = mantissa * 10^actualExponent
	rat := new(big.Rat).SetInt64(int64(mantissa))

	if actualExponent > 0 {
		// Multiply by 10^actualExponent
		scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(actualExponent)), nil)
		rat.Mul(rat, new(big.Rat).SetInt(scale))
	} else if actualExponent < 0 {
		// Divide by 10^(-actualExponent)
		scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-actualExponent)), nil)
		rat.Quo(rat, new(big.Rat).SetInt(scale))
	}

	return rat
}

// multiplyByQuality multiplies an amount by a quality rate.
// Reference: rippled uses mulRound to multiply amount by quality rate.
// The result type is determined by currency/issuer parameters.
func multiplyByQuality(amount tx.Amount, quality uint64, currency, issuer string) tx.Amount {
	if quality == 0 || amount.IsZero() {
		if currency == "" || currency == "XRP" {
			return tx.NewXRPAmount(0)
		}
		return tx.NewIssuedAmount(0, -100, currency, issuer)
	}

	// Convert amount to big.Rat
	var amtRat *big.Rat
	if amount.IsNative() {
		amtRat = new(big.Rat).SetInt64(amount.Drops())
	} else {
		mantissa := amount.Mantissa()
		exponent := amount.Exponent()
		amtRat = new(big.Rat).SetInt64(mantissa)
		if exponent > 0 {
			scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
			amtRat.Mul(amtRat, new(big.Rat).SetInt(scale))
		} else if exponent < 0 {
			scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exponent)), nil)
			amtRat.Quo(amtRat, new(big.Rat).SetInt(scale))
		}
	}

	// Get quality as rate
	rateRat := qualityToRate(quality)

	// Result = amount * rate
	result := new(big.Rat).Mul(amtRat, rateRat)

	// Convert result to the target type
	if currency == "" || currency == "XRP" {
		// Return XRP amount
		f, _ := result.Float64()
		return tx.NewXRPAmount(int64(f + 0.5)) // Round
	}

	// Return IOU amount - normalize to mantissa/exponent form
	return ratToIssuedAmount(result, currency, issuer, true)
}

// divideByQuality divides an amount by a quality rate.
// Reference: rippled uses divRound to divide amount by quality rate.
// The result type is determined by currency/issuer parameters.
func divideByQuality(amount tx.Amount, quality uint64, currency, issuer string) tx.Amount {
	if quality == 0 {
		// Division by zero - return zero
		if currency == "" || currency == "XRP" {
			return tx.NewXRPAmount(0)
		}
		return tx.NewIssuedAmount(0, -100, currency, issuer)
	}

	if amount.IsZero() {
		if currency == "" || currency == "XRP" {
			return tx.NewXRPAmount(0)
		}
		return tx.NewIssuedAmount(0, -100, currency, issuer)
	}

	// Convert amount to big.Rat
	var amtRat *big.Rat
	if amount.IsNative() {
		amtRat = new(big.Rat).SetInt64(amount.Drops())
	} else {
		mantissa := amount.Mantissa()
		exponent := amount.Exponent()
		amtRat = new(big.Rat).SetInt64(mantissa)
		if exponent > 0 {
			scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
			amtRat.Mul(amtRat, new(big.Rat).SetInt(scale))
		} else if exponent < 0 {
			scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exponent)), nil)
			amtRat.Quo(amtRat, new(big.Rat).SetInt(scale))
		}
	}

	// Get quality as rate
	rateRat := qualityToRate(quality)

	// Result = amount / rate
	result := new(big.Rat).Quo(amtRat, rateRat)

	// Convert result to the target type
	if currency == "" || currency == "XRP" {
		// Return XRP amount
		f, _ := result.Float64()
		return tx.NewXRPAmount(int64(f + 0.5)) // Round
	}

	// Return IOU amount - normalize to mantissa/exponent form
	return ratToIssuedAmount(result, currency, issuer, true)
}

// ratToIssuedAmount converts a big.Rat to an IOU Amount with the given currency and issuer.
// The roundUp parameter controls rounding direction.
func ratToIssuedAmount(rat *big.Rat, currency, issuer string, roundUp bool) tx.Amount {
	if rat.Sign() == 0 {
		return tx.NewIssuedAmount(0, -100, currency, issuer)
	}

	// Handle sign
	negative := rat.Sign() < 0
	if negative {
		rat = new(big.Rat).Neg(rat)
	}

	// Normalize to mantissa in [10^15, 10^16)
	minMant := big.NewInt(1000000000000000)  // 10^15
	maxMant := big.NewInt(10000000000000000) // 10^16

	exponent := 0
	scaled := new(big.Rat).Set(rat)

	for {
		intPart := new(big.Int).Quo(scaled.Num(), scaled.Denom())
		if intPart.Cmp(maxMant) >= 0 {
			// Too large, divide by 10
			scaled.Quo(scaled, big.NewRat(10, 1))
			exponent++
		} else if intPart.Cmp(minMant) < 0 {
			// Too small, multiply by 10
			if scaled.Sign() == 0 {
				break
			}
			scaled.Mul(scaled, big.NewRat(10, 1))
			exponent--
		} else {
			break
		}
		// Safety limit
		if exponent > 80 || exponent < -96 {
			break
		}
	}

	// Get final mantissa with rounding
	intPart := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	remainder := new(big.Int).Mod(scaled.Num(), scaled.Denom())

	if roundUp && remainder.Sign() != 0 {
		intPart.Add(intPart, big.NewInt(1))
	}

	mantissa := intPart.Int64()
	if negative {
		mantissa = -mantissa
	}

	return tx.NewIssuedAmount(mantissa, exponent, currency, issuer)
}

// peekOffer reads an offer from the ledger without modifying it.
func peekOffer(view tx.LedgerView, accountID [20]byte, sequence uint32) *sle.LedgerOffer {
	offerKey := keylet.Offer(accountID, sequence)
	data, err := view.Read(offerKey)
	if err != nil || data == nil {
		return nil
	}

	offer, err := sle.ParseLedgerOffer(data)
	if err != nil {
		return nil
	}

	return offer
}

// offerDelete removes an offer from the ledger.
// Reference: rippled ledger/View.h offerDelete()
func offerDelete(ctx *tx.ApplyContext, offer *sle.LedgerOffer) tx.Result {
	result := offerDeleteInView(ctx.View, offer)
	if result == tx.TesSUCCESS {
		// Decrement owner count only on success
		if ctx.Account.OwnerCount > 0 {
			ctx.Account.OwnerCount--
		}
	}
	return result
}

// offerDeleteInView removes an offer from the given view without modifying account state.
// This is used by the two-sandbox pattern to delete offers in both sandboxes.
func offerDeleteInView(view tx.LedgerView, offer *sle.LedgerOffer) tx.Result {
	// Get offer key
	accountID, err := sle.DecodeAccountID(offer.Account)
	if err != nil {
		return tx.TefINTERNAL
	}
	offerKey := keylet.Offer(accountID, offer.Sequence)

	// Remove from owner directory
	ownerDirKey := keylet.OwnerDir(accountID)
	_, err = sle.DirRemove(view, ownerDirKey, offer.OwnerNode, offerKey.Key, false)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Remove from book directory
	bookDirKey := keylet.Keylet{Type: 100, Key: offer.BookDirectory}
	_, err = sle.DirRemove(view, bookDirKey, offer.BookNode, offerKey.Key, false)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Delete the offer
	if err := view.Erase(offerKey); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}

// getOfferSequence returns the sequence number to use for a new offer.
// Reference: rippled CreateOffer.cpp - uses transaction's Sequence or TicketSequence
func (o *OfferCreate) getOfferSequence() uint32 {
	// Use the transaction's Sequence field directly
	// If TicketSequence is used, that becomes the offer's sequence
	if o.TicketSequence != nil {
		return *o.TicketSequence
	}
	if o.Sequence != nil {
		return *o.Sequence
	}
	return 0
}

// parseFee extracts the fee from the transaction context.
func parseFee(ctx *tx.ApplyContext) uint64 {
	// The fee is already deducted in the engine before Apply is called
	// Return a reasonable default for reserve calculations
	return ctx.Config.BaseFee
}

// applyHybrid handles hybrid offer placement for permissioned DEX.
// Reference: rippled CreateOffer.cpp applyHybrid() lines 528-573
func applyHybrid(ctx *tx.ApplyContext, offer *sle.LedgerOffer, offerKey keylet.Keylet, takerPays, takerGets tx.Amount, domainBookDir keylet.Keylet) tx.Result {
	return applyHybridInSandbox(ctx.View, ctx, offer, offerKey, takerPays, takerGets, domainBookDir)
}

// applyHybridInSandbox handles hybrid offer placement in a specific view/sandbox.
// Reference: rippled CreateOffer.cpp applyHybrid() lines 528-573
func applyHybridInSandbox(view tx.LedgerView, ctx *tx.ApplyContext, offer *sle.LedgerOffer, offerKey keylet.Keylet, takerPays, takerGets tx.Amount, domainBookDir keylet.Keylet) tx.Result {
	// Set hybrid flag
	offer.Flags |= lsfHybrid

	// Also place in open book (without domain)
	takerPaysCurrency := sle.GetCurrencyBytes(takerPays.Currency)
	takerPaysIssuer := sle.GetIssuerBytes(takerPays.Issuer)
	takerGetsCurrency := sle.GetCurrencyBytes(takerGets.Currency)
	takerGetsIssuer := sle.GetIssuerBytes(takerGets.Issuer)

	uRate := sle.GetRate(takerGets, takerPays)

	bookBase := keylet.BookDir(takerPaysCurrency, takerPaysIssuer, takerGetsCurrency, takerGetsIssuer)
	openBookDirKey := keylet.Quality(bookBase, uRate)

	// Add to open book directory
	bookDirResult, err := sle.DirInsert(view, openBookDirKey, offerKey.Key, func(dir *sle.DirectoryNode) {
		dir.TakerPaysCurrency = takerPaysCurrency
		dir.TakerPaysIssuer = takerPaysIssuer
		dir.TakerGetsCurrency = takerGetsCurrency
		dir.TakerGetsIssuer = takerGetsIssuer
		dir.ExchangeRate = uRate
		// No DomainID for open book
	})
	if err != nil {
		return tx.TefINTERNAL
	}

	// Store additional book info in offer
	offer.AdditionalBookDirectory = openBookDirKey.Key
	offer.AdditionalBookNode = bookDirResult.Page

	return tx.TesSUCCESS
}

// lsfHybrid is the ledger flag for hybrid offers
const lsfHybrid uint32 = 0x00040000
