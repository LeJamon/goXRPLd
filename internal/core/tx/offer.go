package tx

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
)

// OfferCreate places an offer on the decentralized exchange.
type OfferCreate struct {
	BaseTx

	// TakerGets is the amount and currency the offer creator receives (required)
	TakerGets Amount `json:"TakerGets"`

	// TakerPays is the amount and currency the offer creator pays (required)
	TakerPays Amount `json:"TakerPays"`

	// Expiration is the time when the offer expires (optional)
	Expiration *uint32 `json:"Expiration,omitempty"`

	// OfferSequence is the sequence number of an offer to cancel (optional)
	OfferSequence *uint32 `json:"OfferSequence,omitempty"`
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
	m := o.Common.ToMap()

	m["TakerGets"] = flattenAmount(o.TakerGets)
	m["TakerPays"] = flattenAmount(o.TakerPays)

	if o.Expiration != nil {
		m["Expiration"] = *o.Expiration
	}
	if o.OfferSequence != nil {
		m["OfferSequence"] = *o.OfferSequence
	}

	return m, nil
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
	OfferSequence uint32 `json:"OfferSequence"`
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
	m := o.Common.ToMap()
	m["OfferSequence"] = o.OfferSequence
	return m, nil
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
			case 3: // BookNode
				offer.BookNode = value
			case 2: // OwnerNode
				offer.OwnerNode = value
			}

		case fieldTypeHash256:
			if offset+32 > len(data) {
				return offer, nil
			}
			if fieldCode == 1 { // BookDirectory
				copy(offer.BookDirectory[:], data[offset:offset+32])
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
			if offset+20 > len(data) {
				return offer, nil
			}
			var accountID [20]byte
			copy(accountID[:], data[offset:offset+20])
			address, _ := encodeAccountID(accountID)
			if fieldCode == 3 { // Account
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
