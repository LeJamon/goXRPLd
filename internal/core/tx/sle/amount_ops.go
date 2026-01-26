package sle

import (
	"bytes"
	"fmt"
	"math/big"
)

// ToIOUAmountLegacy converts an Amount to the legacy IOUAmount type (big.Float based).
// For native XRP amounts, this is not meaningful but returns a zero IOUAmount.
func (a Amount) ToIOUAmountLegacy() IOUAmount {
	if a.IsNative() {
		return IOUAmount{
			Value: new(big.Float).SetPrec(128).SetInt64(a.Drops()),
		}
	}
	// Convert mantissa/exponent to big.Float WITHOUT going through float64
	// value = mantissa × 10^exponent
	iou := a.iou
	mantissa := iou.Mantissa()
	exponent := iou.Exponent()

	// Create big.Float from mantissa with high precision
	v := new(big.Float).SetPrec(128).SetInt64(mantissa)

	// Apply exponent using precise integer powers of 10
	if exponent > 0 {
		multiplier := new(big.Float).SetPrec(128).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil))
		v.Mul(v, multiplier)
	} else if exponent < 0 {
		divisor := new(big.Float).SetPrec(128).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exponent)), nil))
		v.Quo(v, divisor)
	}

	return IOUAmount{
		Value:    v,
		Currency: a.Currency,
		Issuer:   a.Issuer,
	}
}

// CompareAccountIDs compares two 20-byte account IDs lexicographically.
// Returns -1, 0, or 1.
func CompareAccountIDs(a, b [20]byte) int {
	return bytes.Compare(a[:], b[:])
}

// CompareAccountIDsForLine compares two account IDs for trust line ordering.
// The "low" account is the one that sorts first lexicographically.
func CompareAccountIDsForLine(a, b [20]byte) int {
	return bytes.Compare(a[:], b[:])
}

// FormatDrops formats a uint64 drops value as a string
func FormatDrops(drops uint64) string {
	return fmt.Sprintf("%d", drops)
}

// SubtractAmount subtracts b from a, returning the result.
// Both amounts must be the same type (both XRP or same IOU currency).
func SubtractAmount(a, b Amount) Amount {
	if a.IsNative() {
		aDrops := a.Drops()
		bDrops := b.Drops()
		if aDrops >= bDrops {
			return NewXRPAmountFromInt(aDrops - bDrops)
		}
		return NewXRPAmountFromInt(0)
	}
	// For IOU amounts, use the built-in subtraction
	result, _ := a.Sub(b)
	return result
}

// ApplyTransferFee applies a transfer rate to an amount.
// transferRate is the rate as uint32 (1000000000 = no fee, 1100000000 = 10% fee).
func ApplyTransferFee(amount Amount, transferRate uint32) Amount {
	if transferRate == 0 || transferRate == 1000000000 {
		return amount
	}
	if amount.IsNative() {
		return amount // No transfer fee on XRP
	}
	// Apply transfer rate: newAmount = amount * transferRate / 1000000000
	// Use float64 for the calculation
	value := amount.Float64()
	multiplier := float64(transferRate) / 1000000000.0
	result := value * multiplier
	return NewIssuedAmountFromFloat64(result, amount.Currency, amount.Issuer)
}

// EncodeAccountIDSafe encodes a 20-byte account ID, returning empty string on error
func EncodeAccountIDSafe(accountID [20]byte) string {
	s, _ := EncodeAccountID(accountID)
	return s
}

// CalculateQuality calculates the quality (exchange rate) for an offer.
// Quality = TakerPays / TakerGets
func CalculateQuality(takerPays, takerGets Amount) uint64 {
	return GetRate(takerPays, takerGets)
}
