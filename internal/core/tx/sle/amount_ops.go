package sle

import (
	"bytes"
	"fmt"
	"math/big"
)

// ToIOU converts an Amount to an IOUAmount.
// For native XRP amounts, this is not meaningful but returns a zero IOUAmount.
func (a Amount) ToIOU() IOUAmount {
	if a.IsNative() {
		drops, _ := ParseDropsString(a.Value)
		return IOUAmount{
			Value: new(big.Float).SetUint64(drops),
		}
	}
	v, _, _ := big.ParseFloat(a.Value, 10, 128, big.ToNearestEven)
	if v == nil {
		v = new(big.Float)
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
		aDrops, _ := ParseDropsString(a.Value)
		bDrops, _ := ParseDropsString(b.Value)
		if aDrops >= bDrops {
			return NewXRPAmount(FormatDrops(aDrops - bDrops))
		}
		return NewXRPAmount("0")
	}
	aVal, _, _ := big.ParseFloat(a.Value, 10, 128, big.ToNearestEven)
	bVal, _, _ := big.ParseFloat(b.Value, 10, 128, big.ToNearestEven)
	if aVal == nil {
		aVal = new(big.Float)
	}
	if bVal == nil {
		bVal = new(big.Float)
	}
	result := new(big.Float).Sub(aVal, bVal)
	return NewIssuedAmount(FormatIOUValue(result), a.Currency, a.Issuer)
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
	aVal, _, _ := big.ParseFloat(amount.Value, 10, 128, big.ToNearestEven)
	if aVal == nil {
		return amount
	}
	rate := new(big.Float).SetUint64(uint64(transferRate))
	billion := new(big.Float).SetUint64(1000000000)
	multiplier := new(big.Float).Quo(rate, billion)
	result := new(big.Float).Mul(aVal, multiplier)
	return NewIssuedAmount(FormatIOUValue(result), amount.Currency, amount.Issuer)
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
