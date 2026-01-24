//go:build ignore

package offer

import (
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
)

func init() {
	tx.Register(tx.TypeOfferCreate, func() tx.Transaction {
		return &OfferCreate{BaseTx: *tx.NewBaseTx(tx.TypeOfferCreate, "")}
	})
	tx.Register(tx.TypeOfferCancel, func() tx.Transaction {
		return &OfferCancel{BaseTx: *tx.NewBaseTx(tx.TypeOfferCancel, "")}
	})
}

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
}

// OfferCreate flags
const (
	// OfferCreateFlagPassive won't consume offers that match this one
	OfferCreateFlagPassive uint32 = 0x00010000
	// OfferCreateFlagImmediateOrCancel treats offer as immediate-or-cancel
	OfferCreateFlagImmediateOrCancel uint32 = 0x00020000
	// OfferCreateFlagFillOrKill treats offer as fill-or-kill
	OfferCreateFlagFillOrKill uint32 = 0x00040000
	// OfferCreateFlagSell makes the offer a sell offer
	OfferCreateFlagSell uint32 = 0x00080000
)

// Ledger offer flags
const (
	lsfOfferPassive uint32 = 0x00010000
	lsfOfferSell    uint32 = 0x00020000
)

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

// Validate validates the OfferCreate transaction
// Reference: rippled CreateOffer.cpp preflight()
func (o *OfferCreate) Validate() error {
	if err := o.BaseTx.Validate(); err != nil {
		return err
	}

	if o.TakerGets.Value == "" {
		return errors.New("temBAD_OFFER: TakerGets is required")
	}

	if o.TakerPays.Value == "" {
		return errors.New("temBAD_OFFER: TakerPays is required")
	}

	// Cannot have both XRP on both sides
	if o.TakerGets.IsNative() && o.TakerPays.IsNative() {
		return errors.New("temBAD_OFFER: cannot exchange XRP for XRP")
	}

	// Check for negative amounts
	if len(o.TakerGets.Value) > 0 && o.TakerGets.Value[0] == '-' {
		return errors.New("temBAD_OFFER: TakerGets cannot be negative")
	}
	if len(o.TakerPays.Value) > 0 && o.TakerPays.Value[0] == '-' {
		return errors.New("temBAD_OFFER: TakerPays cannot be negative")
	}

	// Check for zero amounts
	if o.TakerGets.Value == "0" {
		return errors.New("temBAD_OFFER: TakerGets cannot be zero")
	}
	if o.TakerPays.Value == "0" {
		return errors.New("temBAD_OFFER: TakerPays cannot be zero")
	}

	// tfImmediateOrCancel and tfFillOrKill are mutually exclusive
	flags := o.GetFlags()
	if (flags&OfferCreateFlagImmediateOrCancel != 0) && (flags&OfferCreateFlagFillOrKill != 0) {
		return errors.New("temINVALID_FLAG: cannot set both ImmediateOrCancel and FillOrKill")
	}

	// Expiration of 0 is invalid
	if o.Expiration != nil && *o.Expiration == 0 {
		return errors.New("temBAD_EXPIRATION: expiration cannot be zero")
	}

	// OfferSequence of 0 is invalid
	if o.OfferSequence != nil && *o.OfferSequence == 0 {
		return errors.New("temBAD_SEQUENCE: OfferSequence cannot be zero")
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

// OfferCancel cancels an existing offer on the decentralized exchange.
type OfferCancel struct {
	tx.BaseTx

	// OfferSequence is the sequence number of the offer to cancel (required)
	OfferSequence uint32 `json:"OfferSequence" xrpl:"OfferSequence"`
}

// NewOfferCancel creates a new OfferCancel transaction
func NewOfferCancel(account string, offerSequence uint32) *OfferCancel {
	return &OfferCancel{
		BaseTx:        *tx.NewBaseTx(tx.TypeOfferCancel, account),
		OfferSequence: offerSequence,
	}
}

// TxType returns the transaction type
func (o *OfferCancel) TxType() tx.Type {
	return tx.TypeOfferCancel
}

// Validate validates the OfferCancel transaction
func (o *OfferCancel) Validate() error {
	if err := o.BaseTx.Validate(); err != nil {
		return err
	}

	if o.OfferSequence == 0 {
		return errors.New("OfferSequence is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (o *OfferCancel) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(o)
}

// Apply applies an OfferCancel transaction to the ledger state.
// Reference: rippled CancelOffer.cpp doApply
func (o *OfferCancel) Apply(ctx *tx.ApplyContext) tx.Result {
	// Find the offer
	accountID, _ := tx.DecodeAccountID(ctx.Account.Account)
	offerKey := keylet.Offer(accountID, o.OfferSequence)

	exists, err := ctx.View.Exists(offerKey)
	if err != nil {
		return tx.TefINTERNAL
	}

	if !exists {
		// Offer doesn't exist - this is OK (maybe already filled/cancelled)
		return tx.TesSUCCESS
	}

	// Read the offer to get its details for metadata and directory removal
	offerData, err := ctx.View.Read(offerKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	ledgerOffer, err := sle.ParseLedgerOffer(offerData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Create SLE for the offer for metadata tracking
	sleOffer := sle.NewSLEOffer(offerKey.Key)
	sleOffer.LoadFromLedgerOffer(ledgerOffer)
	sleOffer.MarkAsDeleted()

	// Remove from owner directory (keepRoot = false since owner dir should persist)
	ownerDirKey := keylet.OwnerDir(accountID)
	ownerDirResult, err := ctx.Engine.DirRemove(ctx.View, ownerDirKey, ledgerOffer.OwnerNode, offerKey.Key, false)
	if err != nil {
		return tx.TefINTERNAL
	}
	if !ownerDirResult.Success {
		return tx.TefBAD_LEDGER
	}

	// Remove from book directory (keepRoot = false - delete directory if empty)
	bookDirKey := keylet.Keylet{Type: 100, Key: ledgerOffer.BookDirectory} // DirectoryNode type
	bookDirResult, err := ctx.Engine.DirRemove(ctx.View, bookDirKey, ledgerOffer.BookNode, offerKey.Key, false)
	if err != nil {
		return tx.TefINTERNAL
	}
	if !bookDirResult.Success {
		return tx.TefBAD_LEDGER
	}

	// Delete the offer from ledger
	if err := ctx.View.Erase(offerKey); err != nil {
		return tx.TefINTERNAL
	}

	// Decrement owner count
	if ctx.Account.OwnerCount > 0 {
		ctx.Account.OwnerCount--
	}

	return tx.TesSUCCESS
}

// Apply applies an OfferCreate transaction to the ledger state.
// Reference: rippled CreateOffer.cpp doApply
func (o *OfferCreate) Apply(ctx *tx.ApplyContext) tx.Result {
	// Check if offer has expired
	if o.Expiration != nil && *o.Expiration > 0 {
		parentCloseTime := ctx.Config.ParentCloseTime
		if *o.Expiration <= parentCloseTime {
			return tx.TecEXPIRED
		}
	}

	// First, cancel any existing offer if OfferSequence is specified
	if o.OfferSequence != nil {
		accountID, _ := tx.DecodeAccountID(ctx.Account.Account)
		oldOfferKey := keylet.Offer(accountID, *o.OfferSequence)
		exists, _ := ctx.View.Exists(oldOfferKey)
		if exists {
			if err := ctx.View.Erase(oldOfferKey); err == nil {
				if ctx.Account.OwnerCount > 0 {
					ctx.Account.OwnerCount--
				}
			}
		}
	}

	// Get the amounts
	takerGets := o.TakerGets
	takerPays := o.TakerPays

	// Check if account has funds to back the offer
	if takerGets.IsNative() {
		currentReserve := ctx.Config.ReserveBase + ctx.Config.ReserveIncrement*uint64(ctx.Account.OwnerCount)
		if ctx.Account.Balance <= currentReserve {
			return tx.TecUNFUNDED_OFFER
		}
	} else {
		accountID, _ := tx.DecodeAccountID(ctx.Account.Account)
		issuerID, _ := tx.DecodeAccountID(takerGets.Issuer)
		if accountID != issuerID {
			trustLineKey := keylet.Line(accountID, issuerID, takerGets.Currency)
			trustLineData, err := ctx.View.Read(trustLineKey)
			if err != nil {
				return tx.TecUNFUNDED_OFFER
			}
			rs, err := sle.ParseRippleState(trustLineData)
			if err != nil {
				return tx.TecUNFUNDED_OFFER
			}
			accountIsLow := tx.CompareAccountIDsForLine(accountID, issuerID) < 0
			balance := rs.Balance
			if !accountIsLow {
				balance = balance.Negate()
			}
			if balance.Value.Sign() <= 0 {
				return tx.TecUNFUNDED_OFFER
			}
		}
	}

	// Check account has reserve for new offer
	reserveCreate := ctx.ReserveForNewObject(ctx.Account.OwnerCount)
	if ctx.Account.Balance < reserveCreate {
		return tx.TecINSUF_RESERVE_OFFER
	}

	// Check for ImmediateOrCancel or FillOrKill flags
	flags := o.GetFlags()
	isPassive := (flags & OfferCreateFlagPassive) != 0
	isIOC := (flags & OfferCreateFlagImmediateOrCancel) != 0
	isFOK := (flags & OfferCreateFlagFillOrKill) != 0

	// Track how much was filled
	var takerGotTotal tx.Amount
	var takerPaidTotal tx.Amount

	// Simple order matching - look for crossing offers
	if !isPassive {
		takerGotTotal, takerPaidTotal = ctx.Engine.MatchOffers(o.TakerGets, o.TakerPays, ctx.Account, ctx.View)
	}

	// Check if fully filled
	fullyFilled := false
	if takerGotTotal.Value != "" && takerPays.Value != "" {
		if takerPays.IsNative() {
			gotDrops, _ := sle.ParseDropsString(takerGotTotal.Value)
			wantDrops, _ := sle.ParseDropsString(takerPays.Value)
			fullyFilled = gotDrops >= wantDrops
		} else {
			gotIOU := tx.NewIOUAmount(takerGotTotal.Value, takerGotTotal.Currency, takerGotTotal.Issuer)
			wantIOU := tx.NewIOUAmount(takerPays.Value, takerPays.Currency, takerPays.Issuer)
			fullyFilled = gotIOU.Compare(wantIOU) >= 0
		}
	}

	// Check if we exhausted what we're selling (TakerGets)
	sellExhausted := false
	if takerPaidTotal.Value != "" && takerGets.Value != "" {
		if takerGets.IsNative() {
			paidDrops, _ := sle.ParseDropsString(takerPaidTotal.Value)
			sellDrops, _ := sle.ParseDropsString(takerGets.Value)
			sellExhausted = paidDrops >= sellDrops
		} else {
			paidIOU := sle.NewIOUAmount(takerPaidTotal.Value, takerPaidTotal.Currency, takerPaidTotal.Issuer)
			sellIOU := sle.NewIOUAmount(takerGets.Value, takerGets.Currency, takerGets.Issuer)
			transferRate := ctx.Engine.GetTransferRate(takerGets.Issuer)
			takerSentIOU := tx.ApplyTransferFee(paidIOU, transferRate)
			sellExhausted = takerSentIOU.Compare(sellIOU) >= 0
		}
	}

	if sellExhausted {
		fullyFilled = true
	}

	// Handle FillOrKill - if not fully filled, fail
	if isFOK && !fullyFilled {
		return tx.TecKILLED
	}

	// Handle ImmediateOrCancel - don't create offer if not fully filled
	if isIOC && !fullyFilled {
		if takerGotTotal.Value != "" {
			return tx.TesSUCCESS // Partial fill succeeded
		}
		return tx.TecKILLED // No fill at all
	}

	// If fully filled, don't create a new offer
	if fullyFilled {
		return tx.TesSUCCESS
	}

	// Create the offer in the ledger
	accountID, _ := tx.DecodeAccountID(ctx.Account.Account)
	offerSequence := ctx.Account.Sequence - 1
	offerKey := keylet.Offer(accountID, offerSequence)

	// Calculate remaining amounts after partial fill
	remainingTakerGets := takerGets
	remainingTakerPays := takerPays
	if takerGotTotal.Value != "" {
		remainingTakerPays = tx.SubtractAmount(takerPays, takerGotTotal)
		originalQuality := tx.CalculateQuality(takerPays, takerGets)
		if originalQuality > 0 {
			if remainingTakerPays.IsNative() {
				remainingDrops, _ := sle.ParseDropsString(remainingTakerPays.Value)
				remainingGetsValue := float64(remainingDrops) / originalQuality
				if takerGets.IsNative() {
					remainingTakerGets = tx.Amount{Value: tx.FormatDrops(uint64(remainingGetsValue))}
				} else {
					remainingTakerGets = tx.Amount{
						Value:    tx.FormatIOUValuePrecise(remainingGetsValue),
						Currency: takerGets.Currency,
						Issuer:   takerGets.Issuer,
					}
				}
			} else {
				remainingIOU := tx.NewIOUAmount(remainingTakerPays.Value, remainingTakerPays.Currency, remainingTakerPays.Issuer)
				remainingPaysVal, _ := remainingIOU.Value.Float64()
				remainingGetsValue := remainingPaysVal / originalQuality
				if takerGets.IsNative() {
					remainingTakerGets = tx.Amount{Value: tx.FormatDrops(uint64(remainingGetsValue))}
				} else {
					remainingTakerGets = tx.Amount{
						Value:    tx.FormatIOUValuePrecise(remainingGetsValue),
						Currency: takerGets.Currency,
						Issuer:   takerGets.Issuer,
					}
				}
			}
		}
	}

	// Calculate quality for the book directory
	quality := sle.GetRate(remainingTakerPays, remainingTakerGets)

	// Get currency and issuer bytes for the book directory
	takerPaysCurrency := sle.GetCurrencyBytes(remainingTakerPays.Currency)
	takerPaysIssuer := tx.GetIssuerBytes(remainingTakerPays.Issuer)
	takerGetsCurrency := sle.GetCurrencyBytes(remainingTakerGets.Currency)
	takerGetsIssuer := tx.GetIssuerBytes(remainingTakerGets.Issuer)

	// Calculate the book directory key with quality
	bookBase := keylet.BookDir(takerPaysCurrency, takerPaysIssuer, takerGetsCurrency, takerGetsIssuer)
	bookDirKey := keylet.Quality(bookBase, quality)

	// Add offer to owner directory
	ownerDirKey := keylet.OwnerDir(accountID)
	ownerDirResult, err := ctx.Engine.DirInsert(ctx.View, ownerDirKey, offerKey.Key, func(dir *sle.DirectoryNode) {
		dir.Owner = accountID
	})
	if err != nil {
		return tx.TefINTERNAL
	}

	// Add offer to book directory
	bookDirResult, err := ctx.Engine.DirInsert(ctx.View, bookDirKey, offerKey.Key, func(dir *sle.DirectoryNode) {
		dir.TakerPaysCurrency = takerPaysCurrency
		dir.TakerPaysIssuer = takerPaysIssuer
		dir.TakerGetsCurrency = takerGetsCurrency
		dir.TakerGetsIssuer = takerGetsIssuer
		dir.ExchangeRate = quality
	})
	if err != nil {
		return tx.TefINTERNAL
	}

	// Create the ledger offer with directory info
	ledgerOffer := &sle.LedgerOffer{
		Account:           ctx.Account.Account,
		Sequence:          offerSequence,
		TakerPays:         remainingTakerPays,
		TakerGets:         remainingTakerGets,
		BookDirectory:     bookDirKey.Key,
		BookNode:          bookDirResult.Page,
		OwnerNode:         ownerDirResult.Page,
		Flags:             0,
		PreviousTxnID:     ctx.TxHash,
		PreviousTxnLgrSeq: ctx.Config.LedgerSequence,
	}

	// Set offer flags
	if isPassive {
		ledgerOffer.Flags |= lsfOfferPassive
	}
	if (flags & OfferCreateFlagSell) != 0 {
		ledgerOffer.Flags |= lsfOfferSell
	}

	// Serialize and store the offer
	offerData, err := tx.SerializeLedgerOffer(ledgerOffer)
	if err != nil {
		return tx.TefINTERNAL
	}

	if err := ctx.View.Insert(offerKey, offerData); err != nil {
		return tx.TefINTERNAL
	}

	// Increment owner count
	ctx.Account.OwnerCount++

	return tx.TesSUCCESS
}
