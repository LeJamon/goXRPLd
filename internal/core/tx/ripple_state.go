package tx

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

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

	// PreviousTxnID is the hash of the previous transaction that modified this entry
	PreviousTxnID [32]byte

	// PreviousTxnLgrSeq is the ledger sequence of the previous transaction
	PreviousTxnLgrSeq uint32
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
	// lsfLowDeepFreeze - low side has deep freeze
	lsfLowDeepFreeze uint32 = 0x02000000
	// lsfHighDeepFreeze - high side has deep freeze
	lsfHighDeepFreeze uint32 = 0x04000000

	// Exported freeze constants for external use
	LsfLowFreeze  uint32 = lsfLowFreeze
	LsfHighFreeze uint32 = lsfHighFreeze

	// Exported NoRipple constants for external use
	LsfLowNoRipple  uint32 = lsfLowNoRipple
	LsfHighNoRipple uint32 = lsfHighNoRipple
)

// Ledger entry type code for RippleState
const ledgerEntryTypeRippleState = 0x0072

// Field codes for RippleState (based on XRPL binary serialization format)
const (
	fieldCodeRSBalance     = 2  // Amount field code for Balance
	fieldCodeLowLimit      = 6  // Amount field code for LowLimit
	fieldCodeHighLimit     = 7  // Amount field code for HighLimit
	fieldCodeLowNode       = 7  // UInt64 field code for LowNode
	fieldCodeHighNode      = 8  // UInt64 field code for HighNode
	fieldCodePrevTxnID     = 5  // Hash256 field code for PreviousTxnID
	fieldCodePrevTxnLgrSeq = 5  // UInt32 field code for PreviousTxnLgrSeq
	fieldCodeLowQualityIn  = 20 // UInt32 field code for LowQualityIn
	fieldCodeLowQualityOut = 21 // UInt32 field code for LowQualityOut
	fieldCodeHighQualityIn = 22 // UInt32 field code for HighQualityIn
	fieldCodeHighQualityOut = 23 // UInt32 field code for HighQualityOut
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

// formatIOUValue formats an IOU value for JSON output, matching rippled's format
// XRPL IOU amounts can have up to 16 significant digits
func formatIOUValue(value *big.Float) string {
	if value == nil || value.Sign() == 0 {
		return "0"
	}

	// Use 'g' format with enough precision to preserve all significant digits
	// XRPL uses 54-bit mantissa which can represent up to ~16 decimal digits
	str := value.Text('g', 16)

	// Remove trailing zeros after decimal point, but keep at least one digit after decimal
	// if there is a decimal point
	if strings.Contains(str, ".") {
		str = strings.TrimRight(str, "0")
		str = strings.TrimRight(str, ".")
	}

	return str
}

// formatIOUValuePrecise formats a float64 value with XRPL precision
// Used for remainder calculations that use float64 arithmetic
func formatIOUValuePrecise(value float64) string {
	if value == 0 {
		return "0"
	}

	// Convert to big.Float for precise formatting
	bf := new(big.Float).SetPrec(128).SetFloat64(value)
	return formatIOUValue(bf)
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
			case fieldCodePrevTxnLgrSeq: // field code 5
				rs.PreviousTxnLgrSeq = value
			case fieldCodeLowQualityIn:
				rs.LowQualityIn = value
			case fieldCodeLowQualityOut:
				rs.LowQualityOut = value
			case fieldCodeHighQualityIn:
				rs.HighQualityIn = value
			case fieldCodeHighQualityOut:
				rs.HighQualityOut = value
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

		case fieldTypeHash256:
			// Hash256 fields are 32 bytes
			if offset+32 > len(data) {
				return rs, nil
			}
			if fieldCode == fieldCodePrevTxnID {
				copy(rs.PreviousTxnID[:], data[offset:offset+32])
			}
			offset += 32

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
			// Unknown type - can't skip properly without knowing size, break loop
			return rs, nil
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
	// Bytes 8-27: currency code (20 bytes / 160 bits)
	// Bytes 28-47: issuer account ID (20 bytes / 160 bits)

	// Parse currency from the 20-byte currency section (bytes 8-27)
	// Standard 3-char codes format: [12 zero bytes][3-char code][5 zero bytes]
	// So within the 48-byte array: bytes 8-19 are zeros, bytes 20-22 are the code
	currency := ""
	isStandardCode := true
	for i := 8; i < 20; i++ {
		if data[i] != 0 {
			isStandardCode = false
			break
		}
	}
	if isStandardCode {
		// Standard currency code at bytes 20-22 (offset 12-14 within currency section)
		currency = string(data[20:23])
	} else {
		// Non-standard currency - hex encode the full 20 bytes
		currency = strings.ToUpper(hex.EncodeToString(data[8:28]))
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

	// Convert mantissa to big.Int first to avoid float64 precision loss
	mantissaBigInt := new(big.Int).SetUint64(mantissa)

	// Convert to big.Float with high precision (128 bits)
	value := new(big.Float).SetPrec(128).SetInt(mantissaBigInt)

	// Apply exponent
	if exponent > 0 {
		multiplier := new(big.Float).SetPrec(128).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil))
		value.Mul(value, multiplier)
	} else if exponent < 0 {
		divisor := new(big.Float).SetPrec(128).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exponent)), nil))
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

// ACCOUNT_ONE is the special issuer address used for Balance in RippleState
const accountOne = "rrrrrrrrrrrrrrrrrrrrBZbvji"

// serializeRippleState serializes a RippleState to binary
func serializeRippleState(rs *RippleState) ([]byte, error) {
	// Use Balance's currency for all amounts (LowLimit/HighLimit may have been parsed with null currency)
	currency := rs.Balance.Currency
	if currency == "" || currency == "\x00\x00\x00" {
		// Fallback to LowLimit or HighLimit currency if Balance has null currency
		if rs.LowLimit.Currency != "" && rs.LowLimit.Currency != "\x00\x00\x00" {
			currency = rs.LowLimit.Currency
		} else if rs.HighLimit.Currency != "" && rs.HighLimit.Currency != "\x00\x00\x00" {
			currency = rs.HighLimit.Currency
		}
	}

	// Helper to create amount with consistent currency
	makeAmount := func(amount IOUAmount, useAccountOne bool) map[string]any {
		valueStr := "0"
		if amount.Value != nil && amount.Value.Sign() != 0 {
			// XRPL uses 54-bit mantissa = up to 16 decimal digits
			valueStr = amount.Value.Text('g', 16)
		}
		curr := currency
		if curr == "" {
			curr = amount.Currency
		}
		issuer := amount.Issuer
		if useAccountOne {
			issuer = accountOne
		}
		return map[string]any{
			"value":    valueStr,
			"currency": curr,
			"issuer":   issuer,
		}
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "RippleState",
		"Flags":           rs.Flags,
		"Balance":         makeAmount(rs.Balance, true), // Balance always uses ACCOUNT_ONE
		"LowLimit":        makeAmount(rs.LowLimit, false),
		"HighLimit":       makeAmount(rs.HighLimit, false),
		"LowNode":         fmt.Sprintf("%x", rs.LowNode),
		"HighNode":        fmt.Sprintf("%x", rs.HighNode),
	}

	// Add quality fields if set (non-zero values)
	if rs.LowQualityIn != 0 {
		jsonObj["LowQualityIn"] = rs.LowQualityIn
	}
	if rs.LowQualityOut != 0 {
		jsonObj["LowQualityOut"] = rs.LowQualityOut
	}
	if rs.HighQualityIn != 0 {
		jsonObj["HighQualityIn"] = rs.HighQualityIn
	}
	if rs.HighQualityOut != 0 {
		jsonObj["HighQualityOut"] = rs.HighQualityOut
	}

	// Add PreviousTxnID if set
	if rs.PreviousTxnID != [32]byte{} {
		jsonObj["PreviousTxnID"] = strings.ToUpper(hex.EncodeToString(rs.PreviousTxnID[:]))
	}

	// Add PreviousTxnLgrSeq if set
	if rs.PreviousTxnLgrSeq != 0 {
		jsonObj["PreviousTxnLgrSeq"] = rs.PreviousTxnLgrSeq
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
