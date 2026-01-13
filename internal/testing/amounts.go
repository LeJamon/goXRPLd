// Package testing provides test infrastructure for XRPL transaction testing.
package testing

import (
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
)

// DropsPerXRP is the number of drops in one XRP.
const DropsPerXRP uint64 = 1_000_000

// XRP converts an XRP amount to drops.
// For example, XRP(100) returns 100,000,000 drops.
func XRP(n int64) uint64 {
	return uint64(n) * DropsPerXRP
}

// Drops returns the drop amount unchanged.
// This is a convenience function for clarity when specifying amounts in drops.
func Drops(n uint64) uint64 {
	return n
}

// XRPTxAmount creates an XRP tx.Amount from drops.
// This returns a tx.Amount suitable for use in transactions.
func XRPTxAmount(drops uint64) tx.Amount {
	return tx.NewXRPAmount(fmt.Sprintf("%d", drops))
}

// XRPTxAmountFromXRP creates an XRP tx.Amount from whole XRP units.
// For example, XRPTxAmountFromXRP(100) creates an amount of 100 XRP.
func XRPTxAmountFromXRP(xrp float64) tx.Amount {
	drops := uint64(xrp * float64(DropsPerXRP))
	return tx.NewXRPAmount(fmt.Sprintf("%d", drops))
}

// USD creates a USD issued currency amount with the specified gateway.
// The amount is specified as a float (e.g., 100.50 for $100.50).
func USD(gw *Account, amount float64) tx.Amount {
	return tx.NewIssuedAmount(fmt.Sprintf("%g", amount), "USD", gw.Address)
}

// EUR creates a EUR issued currency amount with the specified gateway.
func EUR(gw *Account, amount float64) tx.Amount {
	return tx.NewIssuedAmount(fmt.Sprintf("%g", amount), "EUR", gw.Address)
}

// BTC creates a BTC issued currency amount with the specified gateway.
func BTC(gw *Account, amount float64) tx.Amount {
	return tx.NewIssuedAmount(fmt.Sprintf("%g", amount), "BTC", gw.Address)
}

// IssuedCurrency creates an issued currency amount with custom currency code.
// The currency code must be 3 characters (e.g., "JPY", "GBP", "CNY").
func IssuedCurrency(gw *Account, currency string, amount float64) tx.Amount {
	return tx.NewIssuedAmount(fmt.Sprintf("%g", amount), currency, gw.Address)
}

// IssuedCurrencyStr creates an issued currency amount with string value.
// Use this when you need precise control over the amount string representation.
func IssuedCurrencyStr(gw *Account, currency, value string) tx.Amount {
	return tx.NewIssuedAmount(value, currency, gw.Address)
}
