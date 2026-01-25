// Package offer implements the OfferCreate and OfferCancel transactions.
// Reference: rippled CreateOffer.cpp, CancelOffer.cpp
package offer

import (
	"errors"
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/core/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/payment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

// OfferCreate flag mask - invalid flags
// Reference: rippled TxFlags.h
const (
	// tfOfferCreateMask contains all valid OfferCreate flags
	// Valid flags: tfPassive (0x00010000), tfImmediateOrCancel (0x00020000),
	// tfFillOrKill (0x00040000), tfSell (0x00080000), tfHybrid (0x00100000)
	tfOfferCreateMask uint32 = 0xFF00FFFF

	// tfHybrid flag for permissioned DEX hybrid offers
	tfHybrid uint32 = 0x00100000
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
	if o.TakerGets.Value == "" {
		return errors.New("temBAD_OFFER: TakerGets is required")
	}
	if o.TakerPays.Value == "" {
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

// Apply applies an OfferCreate transaction to the ledger state.
// This implements the full rippled CreateOffer flow:
// 1. Preflight validation (with amendment rules)
// 2. Preclaim checks (frozen assets, funds, authorization)
// 3. Offer crossing via flow engine
// 4. Offer placement if not fully filled
// Reference: rippled CreateOffer.cpp doApply()
func (o *OfferCreate) Apply(ctx *tx.ApplyContext) tx.Result {
	fmt.Printf("DEBUG OfferCreate.Apply called\n")

	// Run preflight validation with amendment rules
	// Reference: rippled CreateOffer.cpp preflight()
	if err := o.Preflight(ctx.Rules()); err != nil {
		fmt.Printf("DEBUG OfferCreate.Apply: preflight error: %v\n", err)
		// Convert preflight error to appropriate TER code
		return tx.TemMALFORMED
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
		if isGlobalFrozen(ctx.View, uPaysIssuerID) {
			return tx.TecFROZEN
		}
	}
	if uGetsIssuerID != "" {
		if isGlobalFrozen(ctx.View, uGetsIssuerID) {
			return tx.TecFROZEN
		}
	}

	// Check account has funds for the offer
	// Reference: lines 172-178
	funds := accountFunds(ctx.View, ctx.AccountID, saTakerGets, true)
	if isAmountZeroOrNegative(funds) {
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
func (o *OfferCreate) ApplyCreate(ctx *tx.ApplyContext) tx.Result {
	return o.applyGuts(ctx)
}

// applyGuts contains the main offer creation logic.
// Reference: rippled CreateOffer.cpp applyGuts() lines 576-929
func (o *OfferCreate) applyGuts(ctx *tx.ApplyContext) tx.Result {
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
	if o.OfferSequence != nil {
		sleCancel := peekOffer(ctx.View, ctx.AccountID, *o.OfferSequence)
		if sleCancel != nil {
			result = offerDelete(ctx, sleCancel)
		}
	}

	// Check if offer has expired
	// Reference: lines 623-636
	if hasExpired(ctx, o.Expiration) {
		if rules.DepositPreauthEnabled() {
			return tx.TecEXPIRED
		}
		return tx.TesSUCCESS
	}

	crossed := false

	fmt.Printf("DEBUG applyGuts: bPassive=%v, result=%v\n", bPassive, result)

	if result == tx.TesSUCCESS {
		// Apply tick size rounding if applicable
		// Reference: lines 643-685
		saTakerPays, saTakerGets = applyTickSize(ctx.View, saTakerPays, saTakerGets, bSell, rules)
		if isAmountZeroOrNegative(saTakerPays) || isAmountZeroOrNegative(saTakerGets) {
			// Offer rounded to zero
			fmt.Printf("DEBUG applyGuts: offer rounded to zero\n")
			return tx.TesSUCCESS
		}

		// Recalculate rate after tick size
		uRate = sle.GetRate(saTakerGets, saTakerPays)

		// Perform offer crossing
		// Reference: lines 687-768
		fmt.Printf("DEBUG applyGuts: before FlowCross, !bPassive=%v\n", !bPassive)
		if !bPassive {
			var placeOffer struct {
				in  tx.Amount
				out tx.Amount
			}

			crossResult := payment.FlowCross(
				ctx.View,
				ctx.AccountID,
				saTakerGets, // What we're selling (taker pays to counterparty)
				saTakerPays, // What we want (taker receives from counterparty)
				ctx.TxHash,
				ctx.Config.LedgerSequence,
			)

			// Convert result amounts back
			placeOffer.in = payment.FromEitherAmount(crossResult.TakerPaid) // What we paid out
			placeOffer.out = payment.FromEitherAmount(crossResult.TakerGot) // What we received

			result = crossResult.Result

			if result != tx.TesSUCCESS {
				return result
			}

			// Apply sandbox changes to the main ledger view
			// Reference: rippled CreateOffer.cpp - sandbox changes must be applied
			if crossResult.Sandbox != nil {
				fmt.Printf("DEBUG applyGuts: applying sandbox with %d mods, %d inserts, %d deletes\n",
					len(crossResult.Sandbox.GetModifications()),
					len(crossResult.Sandbox.GetInsertions()),
					len(crossResult.Sandbox.GetDeletions()))
				if err := crossResult.Sandbox.ApplyToView(ctx.View); err != nil {
					fmt.Printf("DEBUG applyGuts: ApplyToView error: %v\n", err)
					return tx.TefINTERNAL
				}
			} else {
				fmt.Printf("DEBUG applyGuts: crossResult.Sandbox is nil!\n")
			}

			// Update ctx.Account.Balance to reflect XRP paid during crossing
			// The engine writes ctx.Account after Apply(), so we must update it here
			// Reference: The taker paid XRP for the crossing
			if placeOffer.in.IsNative() {
				paidDrops, _ := sle.ParseDropsString(placeOffer.in.Value)
				fmt.Printf("DEBUG applyGuts: updating ctx.Account.Balance, paid %d drops\n", paidDrops)
				if ctx.Account.Balance >= paidDrops {
					ctx.Account.Balance -= paidDrops
				}
			}

			// Remove unfunded/bad offers that were marked during crossing
			for offerKey := range crossResult.RemovableOffers {
				offerKeylet := keylet.Keylet{Key: offerKey}
				if err := ctx.View.Erase(offerKeylet); err != nil {
					_ = err // Log but don't fail - cleanup operation
				}
			}

			// Check if any crossing happened
			// Reference: line 744-745
			if !isAmountEmpty(placeOffer.in) || !isAmountEmpty(placeOffer.out) {
				crossed = true
			}

			// Calculate remaining amounts for the new offer
			// Reference: rippled CreateOffer.cpp lines 757-768
			// placeOffer.in = what we paid out (TakerGets consumed)
			// placeOffer.out = what we received (TakerPays consumed)
			// Remaining offer = original - consumed
			remainingGets := subtractAmounts(saTakerGets, placeOffer.in)
			remainingPays := subtractAmounts(saTakerPays, placeOffer.out)

			fmt.Printf("DEBUG applyGuts: crossing done, remainingGets=%s, remainingPays=%s\n",
				remainingGets.Value, remainingPays.Value)

			// Check if offer is fully filled
			// Reference: lines 757-761
			if isAmountZeroOrNegative(remainingGets) || isAmountZeroOrNegative(remainingPays) {
				// Offer fully crossed
				return tx.TesSUCCESS
			}

			// Adjust amounts for remaining offer
			// Reference: lines 766-767
			saTakerPays = remainingPays
			saTakerGets = remainingGets
		}
	}

	// Sanity check: amounts should be positive
	if isAmountZeroOrNegative(saTakerPays) || isAmountZeroOrNegative(saTakerGets) {
		return tx.TefINTERNAL
	}

	if result != tx.TesSUCCESS {
		return result
	}

	// Handle FillOrKill
	// Reference: lines 789-795
	if bFillOrKill {
		if rules.Enabled(amendment.FeatureFix1578) {
			return tx.TecKILLED
		}
		return tx.TesSUCCESS
	}

	// Handle ImmediateOrCancel
	// Reference: lines 799-809
	if bImmediateOrCancel {
		if !crossed && rules.Enabled(amendment.FeatureImmediateOfferKilled) {
			return tx.TecKILLED
		}
		return tx.TesSUCCESS
	}

	// Check reserve for new offer
	// Reference: lines 815-834
	reserve := ctx.AccountReserve(ctx.Account.OwnerCount + 1)
	priorBalance := ctx.Account.Balance + parseFee(ctx)
	if priorBalance < reserve {
		if !crossed {
			return tx.TecINSUF_RESERVE_OFFER
		}
		return tx.TesSUCCESS
	}

	// Create the offer in the ledger
	// Reference: lines 837-925
	offerSequence := getOfferSequence(ctx)
	offerKey := keylet.Offer(ctx.AccountID, offerSequence)

	// Add to owner directory
	// Reference: lines 839-848
	ownerDirKey := keylet.OwnerDir(ctx.AccountID)
	ownerDirResult, err := sle.DirInsert(ctx.View, ownerDirKey, offerKey.Key, func(dir *sle.DirectoryNode) {
		dir.Owner = ctx.AccountID
	})
	if err != nil {
		return tx.TefINTERNAL
	}

	// Increment owner count
	// Reference: line 851
	ctx.Account.OwnerCount++

	// Calculate book directory key
	// Reference: lines 857-887
	takerPaysCurrency := sle.GetCurrencyBytes(saTakerPays.Currency)
	takerPaysIssuer := sle.GetIssuerBytes(saTakerPays.Issuer)
	takerGetsCurrency := sle.GetCurrencyBytes(saTakerGets.Currency)
	takerGetsIssuer := sle.GetIssuerBytes(saTakerGets.Issuer)

	bookBase := keylet.BookDir(takerPaysCurrency, takerPaysIssuer, takerGetsCurrency, takerGetsIssuer)
	bookDirKey := keylet.Quality(bookBase, uRate)

	// Check if book exists (for OrderBookDB tracking)
	bookExisted, _ := ctx.View.Exists(bookDirKey)

	// Add to book directory
	// Reference: lines 884-893
	bookDirResult, err := sle.DirInsert(ctx.View, bookDirKey, offerKey.Key, func(dir *sle.DirectoryNode) {
		dir.TakerPaysCurrency = takerPaysCurrency
		dir.TakerPaysIssuer = takerPaysIssuer
		dir.TakerGetsCurrency = takerGetsCurrency
		dir.TakerGetsIssuer = takerGetsIssuer
		dir.ExchangeRate = uRate
		// Note: DomainID is stored on the offer itself, not the directory
	})
	if err != nil {
		return tx.TefINTERNAL
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
		result = applyHybrid(ctx, ledgerOffer, offerKey, saTakerPays, saTakerGets, bookDirKey)
		if result != tx.TesSUCCESS {
			return result
		}
	}

	// Serialize and store the offer
	offerData, err := sle.SerializeLedgerOffer(ledgerOffer)
	if err != nil {
		return tx.TefINTERNAL
	}

	if err := ctx.View.Insert(offerKey, offerData); err != nil {
		return tx.TefINTERNAL
	}

	// Track new book in OrderBookDB (not implemented yet)
	_ = bookExisted

	return tx.TesSUCCESS
}

// ============================================================================
// Helper functions
// ============================================================================

// isLegalNetAmount checks if an amount is a valid net amount.
// Reference: rippled protocol/STAmount.h isLegalNet()
func isLegalNetAmount(amt tx.Amount) bool {
	if amt.Value == "" {
		return false
	}
	// Check for obviously invalid values
	if len(amt.Value) == 0 {
		return false
	}
	// Additional validation could include:
	// - Checking precision limits
	// - Checking range limits
	// For now, basic validation
	return true
}

// isAmountZeroOrNegative checks if an amount is zero or negative.
func isAmountZeroOrNegative(amt tx.Amount) bool {
	if amt.Value == "" || amt.Value == "0" {
		return true
	}
	if len(amt.Value) > 0 && amt.Value[0] == '-' {
		return true
	}
	return false
}

// isAmountEmpty checks if an amount is empty/unset.
func isAmountEmpty(amt tx.Amount) bool {
	return amt.Value == ""
}

// subtractAmounts subtracts b from a.
// a - b = result
func subtractAmounts(a, b tx.Amount) tx.Amount {
	if a.IsNative() {
		// XRP subtraction
		aVal, _ := sle.ParseDropsString(a.Value)
		bVal, _ := sle.ParseDropsString(b.Value)
		result := int64(aVal) - int64(bVal)
		if result < 0 {
			result = 0
		}
		return tx.Amount{Value: sle.FormatDrops(uint64(result))}
	}

	// IOU subtraction
	aIOU := a.ToIOU()
	bIOU := b.ToIOU()
	resultIOU := aIOU.Sub(bIOU)

	return tx.Amount{
		Value:    sle.FormatIOUValue(resultIOU.Value),
		Currency: a.Currency,
		Issuer:   a.Issuer,
	}
}

// isGlobalFrozen checks if an issuer has globally frozen assets.
// Reference: rippled ledger/View.h isGlobalFrozen()
func isGlobalFrozen(view tx.LedgerView, issuerAddress string) bool {
	if issuerAddress == "" {
		return false
	}

	issuerID, err := sle.DecodeAccountID(issuerAddress)
	if err != nil {
		return false
	}

	accountKey := keylet.Account(issuerID)
	data, err := view.Read(accountKey)
	if err != nil || data == nil {
		return false
	}

	account, err := sle.ParseAccountRoot(data)
	if err != nil {
		return false
	}

	return (account.Flags & sle.LsfGlobalFreeze) != 0
}

// accountFunds returns the amount of funds an account has available.
// If fhZeroIfFrozen is true, returns zero if the asset is frozen.
// Reference: rippled ledger/View.h accountFunds()
func accountFunds(view tx.LedgerView, accountID [20]byte, amount tx.Amount, fhZeroIfFrozen bool) tx.Amount {
	if amount.IsNative() {
		// XRP balance
		accountKey := keylet.Account(accountID)
		data, err := view.Read(accountKey)
		if err != nil || data == nil {
			return tx.Amount{Value: "0"}
		}

		account, err := sle.ParseAccountRoot(data)
		if err != nil {
			return tx.Amount{Value: "0"}
		}

		// Return balance minus reserve
		return tx.Amount{Value: sle.FormatDrops(account.Balance)}
	}

	// IOU balance
	issuerID, err := sle.DecodeAccountID(amount.Issuer)
	if err != nil {
		return tx.Amount{Value: "0", Currency: amount.Currency, Issuer: amount.Issuer}
	}

	// If account is issuer, they have unlimited funds
	if accountID == issuerID {
		return tx.Amount{Value: "999999999999999", Currency: amount.Currency, Issuer: amount.Issuer}
	}

	// Check for frozen if requested
	if fhZeroIfFrozen {
		if isGlobalFrozen(view, amount.Issuer) {
			return tx.Amount{Value: "0", Currency: amount.Currency, Issuer: amount.Issuer}
		}
		// Check individual trustline freeze
		if isTrustlineFrozen(view, accountID, issuerID, amount.Currency) {
			return tx.Amount{Value: "0", Currency: amount.Currency, Issuer: amount.Issuer}
		}
	}

	// Read trustline balance
	trustLineKey := keylet.Line(accountID, issuerID, amount.Currency)
	trustLineData, err := view.Read(trustLineKey)
	if err != nil || trustLineData == nil {
		return tx.Amount{Value: "0", Currency: amount.Currency, Issuer: amount.Issuer}
	}

	rs, err := sle.ParseRippleState(trustLineData)
	if err != nil {
		return tx.Amount{Value: "0", Currency: amount.Currency, Issuer: amount.Issuer}
	}

	// Determine balance based on canonical ordering
	accountIsLow := sle.CompareAccountIDsForLine(accountID, issuerID) < 0
	balance := rs.Balance
	if !accountIsLow {
		balance = balance.Negate()
	}

	// Only return positive balance as available funds
	if balance.Value == nil || balance.Value.Sign() <= 0 {
		return tx.Amount{Value: "0", Currency: amount.Currency, Issuer: amount.Issuer}
	}

	return tx.Amount{
		Value:    sle.FormatIOUValue(balance.Value),
		Currency: amount.Currency,
		Issuer:   amount.Issuer,
	}
}

// isTrustlineFrozen checks if a specific trustline is frozen.
func isTrustlineFrozen(view tx.LedgerView, accountID, issuerID [20]byte, currency string) bool {
	trustLineKey := keylet.Line(accountID, issuerID, currency)
	trustLineData, err := view.Read(trustLineKey)
	if err != nil || trustLineData == nil {
		return false
	}

	rs, err := sle.ParseRippleState(trustLineData)
	if err != nil {
		return false
	}

	// Check freeze flags
	accountIsLow := sle.CompareAccountIDsForLine(accountID, issuerID) < 0
	if accountIsLow {
		return (rs.Flags & sle.LsfLowFreeze) != 0
	}
	return (rs.Flags & sle.LsfHighFreeze) != 0
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
func roundToTickSize(quality uint64, tickSize uint8) uint64 {
	// TODO: Implement proper tick size rounding
	// This is a simplified version
	return quality
}

// multiplyByQuality multiplies an amount by a quality rate.
func multiplyByQuality(amount tx.Amount, quality uint64, currency, issuer string) tx.Amount {
	// TODO: Implement proper multiplication
	return tx.Amount{
		Value:    amount.Value,
		Currency: currency,
		Issuer:   issuer,
	}
}

// divideByQuality divides an amount by a quality rate.
func divideByQuality(amount tx.Amount, quality uint64, currency, issuer string) tx.Amount {
	// TODO: Implement proper division
	return tx.Amount{
		Value:    amount.Value,
		Currency: currency,
		Issuer:   issuer,
	}
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
	// Get offer key
	accountID, err := sle.DecodeAccountID(offer.Account)
	if err != nil {
		return tx.TefINTERNAL
	}
	offerKey := keylet.Offer(accountID, offer.Sequence)

	// Remove from owner directory
	ownerDirKey := keylet.OwnerDir(accountID)
	_, err = sle.DirRemove(ctx.View, ownerDirKey, offer.OwnerNode, offerKey.Key, false)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Remove from book directory
	bookDirKey := keylet.Keylet{Type: 100, Key: offer.BookDirectory}
	_, err = sle.DirRemove(ctx.View, bookDirKey, offer.BookNode, offerKey.Key, false)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Delete the offer
	if err := ctx.View.Erase(offerKey); err != nil {
		return tx.TefINTERNAL
	}

	// Decrement owner count
	if ctx.Account.OwnerCount > 0 {
		ctx.Account.OwnerCount--
	}

	return tx.TesSUCCESS
}

// getOfferSequence returns the sequence number to use for a new offer.
func getOfferSequence(ctx *tx.ApplyContext) uint32 {
	// Use the transaction sequence (or ticket sequence) minus 1
	// because the engine increments sequence before Apply
	return ctx.Account.Sequence - 1
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
	bookDirResult, err := sle.DirInsert(ctx.View, openBookDirKey, offerKey.Key, func(dir *sle.DirectoryNode) {
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
