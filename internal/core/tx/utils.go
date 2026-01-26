package tx

import (
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
	"strconv"
)

// ParseUint64Hex parses a hex string as uint64
func ParseUint64Hex(s string) (uint64, error) {
	return strconv.ParseUint(s, 16, 64)
}

// FormatUint64Hex formats a uint64 as lowercase hex without leading zeros
func FormatUint64Hex(v uint64) string {
	return strconv.FormatUint(v, 16)
}

// IsTrustlineFrozen checks if a specific trustline is frozen.
func IsTrustlineFrozen(view LedgerView, accountID, issuerID [20]byte, currency string) bool {
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

// IsGlobalFrozen checks if an issuer has globally frozen assets.
// Reference: rippled ledger/View.h isGlobalFrozen()
func IsGlobalFrozen(view LedgerView, issuerAddress string) bool {
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

// AccountFunds returns the amount of funds an account has available.
// If fhZeroIfFrozen is true, returns zero if the asset is frozen.
// Reference: rippled ledger/View.h accountFunds()
func AccountFunds(view LedgerView, accountID [20]byte, amount Amount, fhZeroIfFrozen bool) Amount {
	if amount.IsNative() {
		// XRP balance
		accountKey := keylet.Account(accountID)
		data, err := view.Read(accountKey)
		if err != nil || data == nil {
			return NewXRPAmount(0)
		}

		account, err := sle.ParseAccountRoot(data)
		if err != nil {
			return NewXRPAmount(0)
		}

		// Return balance
		//TODO return balance - reserve
		return NewXRPAmount(int64(account.Balance))
	}

	// IOU balance
	issuerID, err := sle.DecodeAccountID(amount.Issuer)
	if err != nil {
		return NewIssuedAmount(0, 0, amount.Currency, amount.Issuer)
	}

	// If account is issuer, they have unlimited funds
	if accountID == issuerID {
		// Return a very large amount (10^15 with exponent 0)
		return NewIssuedAmount(sle.MaxMantissa, 0, amount.Currency, amount.Issuer)
	}

	// Check for frozen if requested
	if fhZeroIfFrozen {
		if IsGlobalFrozen(view, amount.Issuer) {
			return NewIssuedAmount(0, 0, amount.Currency, amount.Issuer)
		}
		// Check individual trustline freeze
		if IsTrustlineFrozen(view, accountID, issuerID, amount.Currency) {
			return NewIssuedAmount(0, 0, amount.Currency, amount.Issuer)
		}
	}

	// Read trustline balance
	trustLineKey := keylet.Line(accountID, issuerID, amount.Currency)
	trustLineData, err := view.Read(trustLineKey)
	if err != nil || trustLineData == nil {
		return NewIssuedAmount(0, 0, amount.Currency, amount.Issuer)
	}

	rs, err := sle.ParseRippleState(trustLineData)
	if err != nil {
		return NewIssuedAmount(0, 0, amount.Currency, amount.Issuer)
	}

	// Determine balance based on canonical ordering
	accountIsLow := sle.CompareAccountIDsForLine(accountID, issuerID) < 0
	balance := rs.Balance
	if !accountIsLow {
		balance = balance.Negate()
	}

	// Only return positive balance as available funds
	if balance.Value == nil || balance.Value.Sign() <= 0 {
		return NewIssuedAmount(0, 0, amount.Currency, amount.Issuer)
	}

	return sle.NewIssuedAmountFromDecimalString(balance.String(), amount.Currency, amount.Issuer)
}
