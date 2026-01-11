package tx

import (
	"encoding/binary"
	"errors"
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
func (o *OfferCreate) Validate() error {
	if err := o.BaseTx.Validate(); err != nil {
		return err
	}

	if o.TakerGets.Value == "" {
		return errors.New("TakerGets is required")
	}

	if o.TakerPays.Value == "" {
		return errors.New("TakerPays is required")
	}

	// Cannot have both XRP on both sides
	if o.TakerGets.IsNative() && o.TakerPays.IsNative() {
		return errors.New("cannot exchange XRP for XRP")
	}

	// tfImmediateOrCancel and tfFillOrKill are mutually exclusive
	flags := o.GetFlags()
	if (flags&OfferCreateFlagImmediateOrCancel != 0) && (flags&OfferCreateFlagFillOrKill != 0) {
		return errors.New("cannot set both ImmediateOrCancel and FillOrKill")
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
	Account       string
	Sequence      uint32
	TakerPays     Amount // What the offer creator wants
	TakerGets     Amount // What the offer creator is selling
	BookDirectory [32]byte
	BookNode      uint64
	OwnerNode     uint64
	Expiration    uint32
	Flags         uint32
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
	var buf []byte

	// Write LedgerEntryType (UInt16, field 1)
	buf = append(buf, (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType)
	buf = append(buf, 0x00, 0x6F) // Offer = 0x006F

	// Write Flags (UInt32, field 2)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeFlags)
	flagsBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(flagsBuf, offer.Flags)
	buf = append(buf, flagsBuf...)

	// Write Sequence (UInt32, field 4)
	buf = append(buf, (fieldTypeUInt32<<4)|4)
	seqBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(seqBuf, offer.Sequence)
	buf = append(buf, seqBuf...)

	// Write TakerPays (Amount, field 4)
	buf = append(buf, (fieldTypeAmount<<4)|4)
	takerPaysBytes, err := serializeAmountBytes(offer.TakerPays)
	if err != nil {
		return nil, err
	}
	buf = append(buf, takerPaysBytes...)

	// Write TakerGets (Amount, field 5)
	buf = append(buf, (fieldTypeAmount<<4)|5)
	takerGetsBytes, err := serializeAmountBytes(offer.TakerGets)
	if err != nil {
		return nil, err
	}
	buf = append(buf, takerGetsBytes...)

	// Write Account (AccountID, field 3)
	buf = append(buf, (fieldTypeAccountID<<4)|3)
	accountID, err := decodeAccountID(offer.Account)
	if err != nil {
		return nil, err
	}
	buf = append(buf, accountID[:]...)

	// Write BookNode (UInt64, field 3)
	buf = append(buf, (fieldTypeUInt64<<4)|3)
	bookNodeBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(bookNodeBuf, offer.BookNode)
	buf = append(buf, bookNodeBuf...)

	// Write OwnerNode (UInt64, field 2)
	buf = append(buf, (fieldTypeUInt64<<4)|2)
	ownerNodeBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(ownerNodeBuf, offer.OwnerNode)
	buf = append(buf, ownerNodeBuf...)

	return buf, nil
}

// serializeAmountBytes serializes an Amount to binary
func serializeAmountBytes(amt Amount) ([]byte, error) {
	if amt.IsNative() {
		// XRP amount - 8 bytes
		drops, err := parseDropsString(amt.Value)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, 8)
		// Set bit 62 (positive XRP amount, not IOU)
		value := drops | 0x4000000000000000
		binary.BigEndian.PutUint64(buf, value)
		return buf, nil
	}

	// IOU amount - 48 bytes
	iou := NewIOUAmount(amt.Value, amt.Currency, amt.Issuer)
	return serializeIOUAmountForOffer(iou)
}

// serializeIOUAmountForOffer serializes an IOUAmount to bytes
func serializeIOUAmountForOffer(amount IOUAmount) ([]byte, error) {
	buf := make([]byte, 48)

	if amount.IsZero() {
		// Zero representation
		buf[0] = 0x80
		return buf, nil
	}

	// Calculate mantissa and exponent
	value := amount.Value
	positive := value.Sign() >= 0
	if !positive {
		value = value.Neg(value)
	}

	// Normalize to mantissa with 15-16 significant digits
	mantissa, exponent := normalizeIOUValue(value)

	// Build the 8-byte value
	var rawValue uint64 = 0x8000000000000000 // Set "not XRP" bit
	if positive {
		rawValue |= 0x4000000000000000 // Set positive bit
	}
	rawValue |= uint64((exponent+97)&0xFF) << 54 // Add exponent
	rawValue |= mantissa & 0x003FFFFFFFFFFFFF   // Add mantissa

	binary.BigEndian.PutUint64(buf[0:8], rawValue)

	// Write currency (standard 3-char code at bytes 12-15)
	if len(amount.Currency) == 3 {
		buf[12] = amount.Currency[0]
		buf[13] = amount.Currency[1]
		buf[14] = amount.Currency[2]
	}

	// Write issuer
	issuerID, _ := decodeAccountID(amount.Issuer)
	copy(buf[28:48], issuerID[:])

	return buf, nil
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
						Value:    iou.Value.Text('f', 15),
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
