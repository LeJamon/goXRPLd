package tx

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
)

// RippleState represents a trust line between two accounts
type RippleState struct {
	// Balance is the current balance of the trust line
	// Positive means LowAccount owes HighAccount
	// Negative means HighAccount owes LowAccount
	Balance IOUAmount

	// LowLimit is the trust limit set by the low account
	LowLimit IOUAmount

	// HighLimit is the trust limit set by the high account
	HighLimit IOUAmount

	// LowNode is the directory node for the low account
	LowNode uint64

	// HighNode is the directory node for the high account
	HighNode uint64

	// Flags for the trust line
	Flags uint32

	// LowQualityIn/Out and HighQualityIn/Out for transfer rates
	LowQualityIn   uint32
	LowQualityOut  uint32
	HighQualityIn  uint32
	HighQualityOut uint32
}

// IOUAmount represents an issued currency amount with decimal precision
type IOUAmount struct {
	Value    *big.Float
	Currency string
	Issuer   string
}

// RippleState flags
const (
	// lsfLowReserve - low account has reserve
	lsfLowReserve uint32 = 0x00010000
	// lsfHighReserve - high account has reserve
	lsfHighReserve uint32 = 0x00020000
	// lsfLowAuth - low account has authorized
	lsfLowAuth uint32 = 0x00040000
	// lsfHighAuth - high account has authorized
	lsfHighAuth uint32 = 0x00080000
	// lsfLowNoRipple - low account has NoRipple flag
	lsfLowNoRipple uint32 = 0x00100000
	// lsfHighNoRipple - high account has NoRipple flag
	lsfHighNoRipple uint32 = 0x00200000
	// lsfLowFreeze - low side is frozen
	lsfLowFreeze uint32 = 0x00400000
	// lsfHighFreeze - high side is frozen
	lsfHighFreeze uint32 = 0x00800000
)

// Ledger entry type code for RippleState
const ledgerEntryTypeRippleState = 0x0072

// Field codes for RippleState
const (
	fieldCodeRSBalance   = 1 // Amount field
	fieldCodeLowLimit    = 2 // Amount field (but using different encoding for limits)
	fieldCodeHighLimit   = 3 // Amount field
	fieldCodeLowNode     = 2 // UInt64
	fieldCodeHighNode    = 3 // UInt64
)

// NewIOUAmount creates a new IOU amount
func NewIOUAmount(value string, currency, issuer string) IOUAmount {
	v, _, _ := big.ParseFloat(value, 10, 128, big.ToNearestEven)
	if v == nil {
		v = big.NewFloat(0)
	}
	return IOUAmount{
		Value:    v,
		Currency: currency,
		Issuer:   issuer,
	}
}

// IsZero returns true if the amount is zero
func (a IOUAmount) IsZero() bool {
	return a.Value == nil || a.Value.Sign() == 0
}

// IsNegative returns true if the amount is negative
func (a IOUAmount) IsNegative() bool {
	return a.Value != nil && a.Value.Sign() < 0
}

// Negate returns the negated amount
func (a IOUAmount) Negate() IOUAmount {
	if a.Value == nil {
		return a
	}
	negated := new(big.Float).Neg(a.Value)
	return IOUAmount{
		Value:    negated,
		Currency: a.Currency,
		Issuer:   a.Issuer,
	}
}

// Add adds two IOU amounts (must have same currency/issuer)
func (a IOUAmount) Add(b IOUAmount) IOUAmount {
	if a.Value == nil {
		return b
	}
	if b.Value == nil {
		return a
	}
	result := new(big.Float).Add(a.Value, b.Value)
	return IOUAmount{
		Value:    result,
		Currency: a.Currency,
		Issuer:   a.Issuer,
	}
}

// Sub subtracts two IOU amounts
func (a IOUAmount) Sub(b IOUAmount) IOUAmount {
	return a.Add(b.Negate())
}

// Compare compares two IOU amounts
// Returns -1 if a < b, 0 if a == b, 1 if a > b
func (a IOUAmount) Compare(b IOUAmount) int {
	if a.Value == nil && b.Value == nil {
		return 0
	}
	if a.Value == nil {
		return -1
	}
	if b.Value == nil {
		return 1
	}
	return a.Value.Cmp(b.Value)
}

// parseRippleState parses a RippleState from binary data
func parseRippleState(data []byte) (*RippleState, error) {
	if len(data) < 20 {
		return nil, errors.New("ripple state data too short")
	}

	rs := &RippleState{}
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
				return rs, nil
			}
			value := binary.BigEndian.Uint16(data[offset : offset+2])
			offset += 2
			if fieldCode == fieldCodeLedgerEntryType {
				if value != ledgerEntryTypeRippleState {
					return nil, errors.New("not a RippleState entry")
				}
			}

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return rs, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case fieldCodeFlags:
				rs.Flags = value
			}

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return rs, nil
			}
			value := binary.BigEndian.Uint64(data[offset : offset+8])
			offset += 8
			switch fieldCode {
			case fieldCodeLowNode:
				rs.LowNode = value
			case fieldCodeHighNode:
				rs.HighNode = value
			}

		case fieldTypeAmount:
			// IOU amounts are 48 bytes
			if offset+48 > len(data) {
				// Try parsing as XRP (8 bytes) - shouldn't happen for RippleState
				if offset+8 > len(data) {
					return rs, nil
				}
				offset += 8
				continue
			}

			// Parse IOU amount
			iou, err := parseIOUAmount(data[offset : offset+48])
			if err != nil {
				offset += 48
				continue
			}

			switch fieldCode {
			case fieldCodeRSBalance:
				rs.Balance = iou
			case fieldCodeLowLimit:
				rs.LowLimit = iou
			case fieldCodeHighLimit:
				rs.HighLimit = iou
			}
			offset += 48

		default:
			// Unknown type - skip
			break
		}
	}

	return rs, nil
}

// parseIOUAmount parses an IOU amount from 48 bytes
func parseIOUAmount(data []byte) (IOUAmount, error) {
	if len(data) != 48 {
		return IOUAmount{}, errors.New("invalid IOU amount length")
	}

	// First 8 bytes: value (mantissa + exponent)
	// Bytes 8-28: currency code (160 bits)
	// Bytes 28-48: issuer account ID (160 bits)

	// Parse currency (bytes 12-15 for standard 3-char codes)
	currency := ""
	if data[8] == 0 && data[9] == 0 && data[10] == 0 && data[11] == 0 {
		// Standard currency code
		currency = string(data[12:15])
	} else {
		// Non-standard currency - hex encode
		// For now, just use first 3 visible chars
		currency = "???"
	}

	// Parse issuer (last 20 bytes)
	var issuerID [20]byte
	copy(issuerID[:], data[28:48])
	issuer, _ := encodeAccountID(issuerID)

	// Parse value from first 8 bytes
	// Bit 63: not XRP (always 1 for IOU)
	// Bit 62: sign (1 = positive)
	// Bits 54-61: exponent (8 bits, biased by 97)
	// Bits 0-53: mantissa (54 bits)
	rawValue := binary.BigEndian.Uint64(data[0:8])

	if rawValue == 0x8000000000000000 { // Zero
		return IOUAmount{
			Value:    big.NewFloat(0),
			Currency: currency,
			Issuer:   issuer,
		}, nil
	}

	positive := (rawValue & 0x4000000000000000) != 0
	exponent := int((rawValue>>54)&0xFF) - 97
	mantissa := rawValue & 0x003FFFFFFFFFFFFF

	// Convert to big.Float
	value := big.NewFloat(float64(mantissa))

	// Apply exponent
	if exponent > 0 {
		multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil))
		value.Mul(value, multiplier)
	} else if exponent < 0 {
		divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exponent)), nil))
		value.Quo(value, divisor)
	}

	if !positive {
		value.Neg(value)
	}

	return IOUAmount{
		Value:    value,
		Currency: currency,
		Issuer:   issuer,
	}, nil
}

// serializeRippleState serializes a RippleState to binary
func serializeRippleState(rs *RippleState) ([]byte, error) {
	// Helper function to convert IOUAmount to JSON map
	iouToJSON := func(amount IOUAmount) map[string]any {
		valueStr := "0"
		if amount.Value != nil {
			valueStr = amount.Value.Text('f', -1)
		}
		return map[string]any{
			"value":    valueStr,
			"currency": amount.Currency,
			"issuer":   amount.Issuer,
		}
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "RippleState",
		"Flags":           rs.Flags,
		"Balance":         iouToJSON(rs.Balance),
		"LowLimit":        iouToJSON(rs.LowLimit),
		"HighLimit":       iouToJSON(rs.HighLimit),
		"LowNode":         fmt.Sprintf("%d", rs.LowNode),
		"HighNode":        fmt.Sprintf("%d", rs.HighNode),
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode RippleState: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// normalizeIOUValue normalizes a value to mantissa and exponent
func normalizeIOUValue(value *big.Float) (uint64, int) {
	if value.Sign() == 0 {
		return 0, 0
	}

	// Get approximate mantissa and exponent
	f, _ := value.Float64()
	if f == 0 {
		return 0, 0
	}

	// Scale to get ~15 digits of precision
	exponent := 0
	for f >= 1e16 {
		f /= 10
		exponent++
	}
	for f < 1e15 && f != 0 {
		f *= 10
		exponent--
	}

	mantissa := uint64(f)
	return mantissa, exponent
}

// ParseRippleStateFromBytes parses a RippleState from binary data (exported)
func ParseRippleStateFromBytes(data []byte) (*RippleState, error) {
	return parseRippleState(data)
}
