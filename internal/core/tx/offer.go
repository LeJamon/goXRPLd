package tx

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

func init() {
	Register(TypeOfferCreate, func() Transaction {
		return &OfferCreate{BaseTx: *NewBaseTx(TypeOfferCreate, "")}
	})
	Register(TypeOfferCancel, func() Transaction {
		return &OfferCancel{BaseTx: *NewBaseTx(TypeOfferCancel, "")}
	})
}

// OfferCreate places an offer on the decentralized exchange.
type OfferCreate struct {
	BaseTx

	// TakerGets is the amount and currency the offer creator receives (required)
	TakerGets Amount `json:"TakerGets" xrpl:"TakerGets,amount"`

	// TakerPays is the amount and currency the offer creator pays (required)
	TakerPays Amount `json:"TakerPays" xrpl:"TakerPays,amount"`

	// Expiration is the time when the offer expires (optional)
	Expiration *uint32 `json:"Expiration,omitempty" xrpl:"Expiration,omitempty"`

	// OfferSequence is the sequence number of an offer to cancel (optional)
	OfferSequence *uint32 `json:"OfferSequence,omitempty" xrpl:"OfferSequence,omitempty"`
}

// OfferCreate flags
const (
	// tfPassive won't consume offers that match this one
	OfferCreateFlagPassive uint32 = 0x00010000
	// tfImmediateOrCancel treats offer as immediate-or-cancel
	OfferCreateFlagImmediateOrCancel uint32 = 0x00020000
	// tfFillOrKill treats offer as fill-or-kill
	OfferCreateFlagFillOrKill uint32 = 0x00040000
	// tfSell makes the offer a sell offer
	OfferCreateFlagSell uint32 = 0x00080000
)

// NewOfferCreate creates a new OfferCreate transaction
func NewOfferCreate(account string, takerGets, takerPays Amount) *OfferCreate {
	return &OfferCreate{
		BaseTx:    *NewBaseTx(TypeOfferCreate, account),
		TakerGets: takerGets,
		TakerPays: takerPays,
	}
}

// TxType returns the transaction type
func (o *OfferCreate) TxType() Type {
	return TypeOfferCreate
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
	// Reference: rippled CreateOffer.cpp:65-69
	if o.TakerGets.IsNative() && o.TakerPays.IsNative() {
		return errors.New("temBAD_OFFER: cannot exchange XRP for XRP")
	}

	// Check for negative amounts
	// Reference: rippled CreateOffer.cpp - temBAD_OFFER for negative amounts
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
	// Reference: rippled CreateOffer.cpp:75-80
	flags := o.GetFlags()
	if (flags&OfferCreateFlagImmediateOrCancel != 0) && (flags&OfferCreateFlagFillOrKill != 0) {
		return errors.New("temINVALID_FLAG: cannot set both ImmediateOrCancel and FillOrKill")
	}

	// Expiration of 0 is invalid
	// Reference: rippled CreateOffer.cpp:82-88
	if o.Expiration != nil && *o.Expiration == 0 {
		return errors.New("temBAD_EXPIRATION: expiration cannot be zero")
	}

	// OfferSequence of 0 is invalid
	// Reference: rippled CreateOffer.cpp:90-94
	if o.OfferSequence != nil && *o.OfferSequence == 0 {
		return errors.New("temBAD_SEQUENCE: OfferSequence cannot be zero")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (o *OfferCreate) Flatten() (map[string]any, error) {
	return ReflectFlatten(o)
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
	BaseTx

	// OfferSequence is the sequence number of the offer to cancel (required)
	OfferSequence uint32 `json:"OfferSequence" xrpl:"OfferSequence"`
}

// NewOfferCancel creates a new OfferCancel transaction
func NewOfferCancel(account string, offerSequence uint32) *OfferCancel {
	return &OfferCancel{
		BaseTx:        *NewBaseTx(TypeOfferCancel, account),
		OfferSequence: offerSequence,
	}
}

// TxType returns the transaction type
func (o *OfferCancel) TxType() Type {
	return TypeOfferCancel
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
	return ReflectFlatten(o)
}

// LedgerOffer represents an offer stored in the ledger
type LedgerOffer struct {
	Account          string
	Sequence         uint32
	TakerPays        Amount // What the offer creator wants
	TakerGets        Amount // What the offer creator is selling
	BookDirectory    [32]byte
	BookNode         uint64
	OwnerNode        uint64
	Expiration       uint32
	Flags            uint32
	PreviousTxnID    [32]byte
	PreviousTxnLgrSeq uint32
}

// Ledger offer flags
const (
	// lsfPassive - offer is passive (doesn't consume offers)
	lsfOfferPassive uint32 = 0x00010000
	// lsfSell - offer is a sell offer
	lsfOfferSell uint32 = 0x00020000
)

// serializeLedgerOffer serializes a LedgerOffer to binary for storage
func serializeLedgerOffer(offer *LedgerOffer) ([]byte, error) {
	// Helper function to convert Amount to JSON format
	amountToJSON := func(amt Amount) any {
		if amt.IsNative() {
			return amt.Value
		}
		return map[string]any{
			"value":    amt.Value,
			"currency": amt.Currency,
			"issuer":   amt.Issuer,
		}
	}

	jsonObj := map[string]any{
		"LedgerEntryType":   "Offer",
		"Account":           offer.Account,
		"Flags":             offer.Flags,
		"Sequence":          offer.Sequence,
		"TakerPays":         amountToJSON(offer.TakerPays),
		"TakerGets":         amountToJSON(offer.TakerGets),
		"BookDirectory":     strings.ToUpper(hex.EncodeToString(offer.BookDirectory[:])),
		"BookNode":          fmt.Sprintf("%x", offer.BookNode),
		"OwnerNode":         fmt.Sprintf("%x", offer.OwnerNode),
		"PreviousTxnID":     strings.ToUpper(hex.EncodeToString(offer.PreviousTxnID[:])),
		"PreviousTxnLgrSeq": offer.PreviousTxnLgrSeq,
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode Offer: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// parseDropsString parses an XRP drops value from string
func parseDropsString(s string) (uint64, error) {
	var drops uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid drops value")
		}
		drops = drops*10 + uint64(c-'0')
	}
	return drops, nil
}

// parseLedgerOffer parses a LedgerOffer from binary data
func parseLedgerOffer(data []byte) (*LedgerOffer, error) {
	if len(data) < 20 {
		return nil, errors.New("offer data too short")
	}

	offer := &LedgerOffer{}
	offset := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		header := data[offset]
		offset++

		typeCode := (header >> 4) & 0x0F
		fieldCode := header & 0x0F

		if typeCode == 0 {
			if offset >= len(data) {
				break
			}
			typeCode = data[offset]
			offset++
		}

		if fieldCode == 0 {
			if offset >= len(data) {
				break
			}
			fieldCode = data[offset]
			offset++
		}

		switch typeCode {
		case fieldTypeUInt16:
			if offset+2 > len(data) {
				return offer, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return offer, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case fieldCodeFlags:
				offer.Flags = value
			case 4: // Sequence
				offer.Sequence = value
			case 5: // PreviousTxnLgrSeq (nth=5 in sfields.macro)
				offer.PreviousTxnLgrSeq = value
			case 10: // Expiration
				offer.Expiration = value
			}

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return offer, nil
			}
			value := binary.BigEndian.Uint64(data[offset : offset+8])
			offset += 8
			switch fieldCode {
			case 3: // BookNode (nth=3 in definitions.json)
				offer.BookNode = value
			case 4: // OwnerNode (nth=4 in definitions.json)
				offer.OwnerNode = value
			}

		case fieldTypeHash256:
			if offset+32 > len(data) {
				return offer, nil
			}
			switch fieldCode {
			case 16: // BookDirectory (nth=16 in definitions.json)
				copy(offer.BookDirectory[:], data[offset:offset+32])
			case 5: // PreviousTxnID (nth=5 in definitions.json)
				copy(offer.PreviousTxnID[:], data[offset:offset+32])
			}
			offset += 32

		case fieldTypeAmount:
			// Determine if XRP (8 bytes) or IOU (48 bytes)
			if offset >= len(data) {
				return offer, nil
			}
			isIOU := (data[offset] & 0x80) != 0
			if isIOU {
				if offset+48 > len(data) {
					return offer, nil
				}
				iou, err := parseIOUAmount(data[offset : offset+48])
				if err == nil {
					amt := Amount{
						Value:    formatIOUValue(iou.Value),
						Currency: iou.Currency,
						Issuer:   iou.Issuer,
					}
					switch fieldCode {
					case 4: // TakerPays
						offer.TakerPays = amt
					case 5: // TakerGets
						offer.TakerGets = amt
					}
				}
				offset += 48
			} else {
				if offset+8 > len(data) {
					return offer, nil
				}
				drops := binary.BigEndian.Uint64(data[offset:offset+8]) & 0x3FFFFFFFFFFFFFFF
				amt := Amount{Value: formatDrops(drops)}
				switch fieldCode {
				case 4: // TakerPays
					offer.TakerPays = amt
				case 5: // TakerGets
					offer.TakerGets = amt
				}
				offset += 8
			}

		case fieldTypeAccountID:
			// AccountID is VL-encoded, first byte is length (should be 0x14 = 20)
			if offset >= len(data) {
				return offer, nil
			}
			length := int(data[offset])
			offset++
			if length != 20 || offset+20 > len(data) {
				return offer, nil
			}
			var accountID [20]byte
			copy(accountID[:], data[offset:offset+20])
			address, _ := encodeAccountID(accountID)
			if fieldCode == 1 { // Account (nth=1 in definitions.json)
				offer.Account = address
			}
			offset += 20

		default:
			// Unknown type - skip
			break
		}
	}

	return offer, nil
}

// formatDrops formats drops as a string
func formatDrops(drops uint64) string {
	if drops == 0 {
		return "0"
	}
	result := make([]byte, 20)
	i := len(result)
	for drops > 0 {
		i--
		result[i] = byte(drops%10) + '0'
		drops /= 10
	}
	return string(result[i:])
}

// ParseLedgerOfferFromBytes parses a LedgerOffer from binary data (exported)
func ParseLedgerOfferFromBytes(data []byte) (*LedgerOffer, error) {
	return parseLedgerOffer(data)
}

// Apply applies an OfferCancel transaction to the ledger state.
// Reference: rippled CancelOffer.cpp doApply
func (o *OfferCancel) Apply(ctx *ApplyContext) Result {
	// Find the offer
	accountID, _ := decodeAccountID(ctx.Account.Account)
	offerKey := keylet.Offer(accountID, o.OfferSequence)

	exists, err := ctx.View.Exists(offerKey)
	if err != nil {
		return TefINTERNAL
	}

	if !exists {
		// Offer doesn't exist - this is OK (maybe already filled/cancelled)
		return TesSUCCESS
	}

	// Read the offer to get its details for metadata and directory removal
	offerData, err := ctx.View.Read(offerKey)
	if err != nil {
		return TefINTERNAL
	}
	ledgerOffer, err := parseLedgerOffer(offerData)
	if err != nil {
		return TefINTERNAL
	}

	// Create SLE for the offer for metadata tracking
	sleOffer := NewSLEOffer(offerKey.Key)
	sleOffer.LoadFromLedgerOffer(ledgerOffer)
	sleOffer.MarkAsDeleted()

	// Remove from owner directory (keepRoot = false since owner dir should persist)
	ownerDirKey := keylet.OwnerDir(accountID)
	ownerDirResult, err := ctx.Engine.dirRemove(ctx.View, ownerDirKey, ledgerOffer.OwnerNode, offerKey.Key, false)
	if err != nil {
		return TefINTERNAL
	}
	if !ownerDirResult.Success {
		return TefBAD_LEDGER
	}

	// Remove from book directory (keepRoot = false - delete directory if empty)
	bookDirKey := keylet.Keylet{Type: 100, Key: ledgerOffer.BookDirectory} // DirectoryNode type
	bookDirResult, err := ctx.Engine.dirRemove(ctx.View, bookDirKey, ledgerOffer.BookNode, offerKey.Key, false)
	if err != nil {
		return TefINTERNAL
	}
	if !bookDirResult.Success {
		return TefBAD_LEDGER
	}

	// Delete the offer from ledger
	if err := ctx.View.Erase(offerKey); err != nil {
		return TefINTERNAL
	}

	// Decrement owner count
	if ctx.Account.OwnerCount > 0 {
		ctx.Account.OwnerCount--
	}

	// All metadata generation tracked automatically by ApplyStateTable

	return TesSUCCESS
}

// Apply applies an OfferCreate transaction to the ledger state.
// Reference: rippled CreateOffer.cpp doApply
func (o *OfferCreate) Apply(ctx *ApplyContext) Result {
	// Check if offer has expired
	// Reference: rippled CreateOffer.cpp:189-200 and 623-636
	if o.Expiration != nil && *o.Expiration > 0 {
		// The offer has expired if Expiration <= parent close time
		// parentCloseTime is the close time of the parent ledger
		parentCloseTime := ctx.Config.ParentCloseTime
		if *o.Expiration <= parentCloseTime {
			// Offer has expired - return tecEXPIRED
			// Note: in older rippled versions without featureDepositPreauth, this would return tesSUCCESS
			return TecEXPIRED
		}
	}

	// First, cancel any existing offer if OfferSequence is specified
	if o.OfferSequence != nil {
		accountID, _ := decodeAccountID(ctx.Account.Account)
		oldOfferKey := keylet.Offer(accountID, *o.OfferSequence)
		exists, _ := ctx.View.Exists(oldOfferKey)
		if exists {
			// Delete the old offer - tracked automatically by ApplyStateTable
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
	// Reference: rippled CreateOffer.cpp:172-178 (preclaim)
	// accountFunds checks if the account has ANY available funds for TakerGets
	// For XRP: available = balance - current_reserve
	// If available <= 0, return tecUNFUNDED_OFFER
	if takerGets.IsNative() {
		// For XRP offers, check if balance exceeds current reserve
		currentReserve := ctx.Config.ReserveBase + ctx.Config.ReserveIncrement*uint64(ctx.Account.OwnerCount)
		if ctx.Account.Balance <= currentReserve {
			return TecUNFUNDED_OFFER
		}
	} else {
		// For IOU offers, check if account has any of the token
		// Reference: rippled CreateOffer.cpp preclaim - accountFunds(... saTakerGets ...)
		// If account IS the issuer, they have unlimited funds
		accountID, _ := decodeAccountID(ctx.Account.Account)
		issuerID, _ := decodeAccountID(takerGets.Issuer)
		if accountID != issuerID {
			// Check trust line balance
			trustLineKey := keylet.Line(accountID, issuerID, takerGets.Currency)
			trustLineData, err := ctx.View.Read(trustLineKey)
			if err != nil {
				// No trust line = no funds
				return TecUNFUNDED_OFFER
			}
			rs, err := parseRippleState(trustLineData)
			if err != nil {
				return TecUNFUNDED_OFFER
			}
			// Determine balance from account's perspective
			accountIsLow := compareAccountIDsForLine(accountID, issuerID) < 0
			balance := rs.Balance
			if !accountIsLow {
				balance = balance.Negate()
			}
			// If balance <= 0, account has no funds
			if balance.Value.Sign() <= 0 {
				return TecUNFUNDED_OFFER
			}
		}
	}

	// Check account has reserve for new offer
	// Per rippled, first 2 objects don't need extra reserve
	reserveCreate := ctx.ReserveForNewObject(ctx.Account.OwnerCount)
	if ctx.Account.Balance < reserveCreate {
		return TecINSUF_RESERVE_OFFER
	}

	// Check for ImmediateOrCancel or FillOrKill flags
	flags := o.GetFlags()
	isPassive := (flags & OfferCreateFlagPassive) != 0
	isIOC := (flags & OfferCreateFlagImmediateOrCancel) != 0
	isFOK := (flags & OfferCreateFlagFillOrKill) != 0

	// Track how much was filled
	var takerGotTotal Amount
	var takerPaidTotal Amount

	// Simple order matching - look for crossing offers
	// This is a simplified implementation that checks if there are any offers to match
	if !isPassive {
		takerGotTotal, takerPaidTotal = ctx.Engine.matchOffers(o, ctx.Account, ctx.View)
		// Metadata consolidation now handled by ApplyStateTable
	}

	// Check if fully filled
	// XRPL semantics: TakerPays = what taker receives, TakerGets = what taker pays
	// takerGotTotal is what we received from matching (should compare with TakerPays)
	// takerPaidTotal is what we paid to matches (should compare with TakerGets)
	fullyFilled := false
	if takerGotTotal.Value != "" && takerPays.Value != "" {
		// Compare what we received with what we wanted to receive
		if takerPays.IsNative() {
			gotDrops, _ := parseDropsString(takerGotTotal.Value)
			wantDrops, _ := parseDropsString(takerPays.Value)
			fullyFilled = gotDrops >= wantDrops
		} else {
			gotIOU := NewIOUAmount(takerGotTotal.Value, takerGotTotal.Currency, takerGotTotal.Issuer)
			wantIOU := NewIOUAmount(takerPays.Value, takerPays.Currency, takerPays.Issuer)
			fullyFilled = gotIOU.Compare(wantIOU) >= 0
		}
	}

	// Also check if we exhausted what we're selling (TakerGets)
	// This can happen when transfer fees cause us to pay more than takerPaidTotal reports
	// (takerPaidTotal is what makers received, not what taker actually sent including fees)
	sellExhausted := false
	if takerPaidTotal.Value != "" && takerGets.Value != "" {
		if takerGets.IsNative() {
			// XRP has no transfer fee
			paidDrops, _ := parseDropsString(takerPaidTotal.Value)
			sellDrops, _ := parseDropsString(takerGets.Value)
			sellExhausted = paidDrops >= sellDrops
		} else {
			// IOU - need to account for transfer fee
			// Taker sent = takerPaidTotal * transferRate
			// If taker sent >= takerGets, we're exhausted
			paidIOU := NewIOUAmount(takerPaidTotal.Value, takerPaidTotal.Currency, takerPaidTotal.Issuer)
			sellIOU := NewIOUAmount(takerGets.Value, takerGets.Currency, takerGets.Issuer)
			transferRate := ctx.Engine.getTransferRate(takerGets.Issuer)
			takerSentIOU := applyTransferFee(paidIOU, transferRate)
			sellExhausted = takerSentIOU.Compare(sellIOU) >= 0
		}
	}

	// Fully filled if we got what we wanted OR exhausted what we're selling
	if sellExhausted {
		fullyFilled = true
	}

	// Handle FillOrKill - if not fully filled, fail
	if isFOK && !fullyFilled {
		return TecKILLED
	}

	// Handle ImmediateOrCancel - don't create offer if not fully filled
	if isIOC && !fullyFilled {
		// Partial fill is OK for IOC, just don't create remaining offer
		if takerGotTotal.Value != "" {
			return TesSUCCESS // Partial fill succeeded
		}
		return TecKILLED // No fill at all
	}

	// If fully filled, don't create a new offer
	if fullyFilled {
		return TesSUCCESS
	}

	// Create the offer in the ledger
	accountID, _ := decodeAccountID(ctx.Account.Account)
	offerSequence := ctx.Account.Sequence - 1 // Sequence was already incremented in preflight
	offerKey := keylet.Offer(accountID, offerSequence)

	// Calculate remaining amounts after partial fill
	// XRPL semantics: TakerPays = what taker receives, TakerGets = what taker pays
	// takerGotTotal = what we received (same currency as TakerPays)
	// takerPaidTotal = what we paid (same currency as TakerGets)
	// IMPORTANT: rippled calculates remainder at ORIGINAL quality to maintain consistency
	// Reference: rippled CreateOffer.cpp - remainder uses original offer's exchange rate
	remainingTakerGets := takerGets
	remainingTakerPays := takerPays
	if takerGotTotal.Value != "" {
		// Subtract what was already received from what we want to receive
		remainingTakerPays = subtractAmount(takerPays, takerGotTotal)
		// Calculate remaining TakerGets at original quality (not simple subtraction)
		// Quality = TakerPays / TakerGets
		// remainingTakerGets = remainingTakerPays / quality
		originalQuality := calculateQuality(takerPays, takerGets)
		if originalQuality > 0 {
			if remainingTakerPays.IsNative() {
				// XRP remaining
				remainingDrops, _ := parseDropsString(remainingTakerPays.Value)
				remainingGetsValue := float64(remainingDrops) / originalQuality
				if takerGets.IsNative() {
					remainingTakerGets = Amount{Value: formatDrops(uint64(remainingGetsValue))}
				} else {
					remainingTakerGets = Amount{
						Value:    formatIOUValuePrecise(remainingGetsValue),
						Currency: takerGets.Currency,
						Issuer:   takerGets.Issuer,
					}
				}
			} else {
				// IOU remaining
				remainingIOU := NewIOUAmount(remainingTakerPays.Value, remainingTakerPays.Currency, remainingTakerPays.Issuer)
				remainingPaysVal, _ := remainingIOU.Value.Float64()
				remainingGetsValue := remainingPaysVal / originalQuality
				if takerGets.IsNative() {
					remainingTakerGets = Amount{Value: formatDrops(uint64(remainingGetsValue))}
				} else {
					remainingTakerGets = Amount{
						Value:    formatIOUValuePrecise(remainingGetsValue),
						Currency: takerGets.Currency,
						Issuer:   takerGets.Issuer,
					}
				}
			}
		}
	}

	// Calculate quality (exchange rate) for the book directory
	quality := GetRate(remainingTakerPays, remainingTakerGets)

	// Get currency and issuer bytes for the book directory
	takerPaysCurrency := getCurrencyBytes(remainingTakerPays.Currency)
	takerPaysIssuer := getIssuerBytes(remainingTakerPays.Issuer)
	takerGetsCurrency := getCurrencyBytes(remainingTakerGets.Currency)
	takerGetsIssuer := getIssuerBytes(remainingTakerGets.Issuer)

	// Calculate the book directory key with quality
	bookBase := keylet.BookDir(takerPaysCurrency, takerPaysIssuer, takerGetsCurrency, takerGetsIssuer)
	bookDirKey := keylet.Quality(bookBase, quality)

	// Add offer to owner directory
	ownerDirKey := keylet.OwnerDir(accountID)
	ownerDirResult, err := ctx.Engine.dirInsert(ctx.View, ownerDirKey, offerKey.Key, func(dir *DirectoryNode) {
		dir.Owner = accountID
	})
	if err != nil {
		return TefINTERNAL
	}

	// Add offer to book directory
	bookDirResult, err := ctx.Engine.dirInsert(ctx.View, bookDirKey, offerKey.Key, func(dir *DirectoryNode) {
		dir.TakerPaysCurrency = takerPaysCurrency
		dir.TakerPaysIssuer = takerPaysIssuer
		dir.TakerGetsCurrency = takerGetsCurrency
		dir.TakerGetsIssuer = takerGetsIssuer
		dir.ExchangeRate = quality
	})
	if err != nil {
		return TefINTERNAL
	}

	// Create the ledger offer with directory info
	ledgerOffer := &LedgerOffer{
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
	offerData, err := serializeLedgerOffer(ledgerOffer)
	if err != nil {
		return TefINTERNAL
	}

	if err := ctx.View.Insert(offerKey, offerData); err != nil {
		return TefINTERNAL
	}

	// Increment owner count
	ctx.Account.OwnerCount++

	// Directory, book, and offer metadata tracked automatically by ApplyStateTable

	return TesSUCCESS
}

