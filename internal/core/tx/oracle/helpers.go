package oracle

import "fmt"

// TokenPairKey returns a unique key for this token pair (for deduplication)
func (p *PriceDataEntry) TokenPairKey() string {
	return p.BaseAsset + "/" + p.QuoteAsset
}

// IsDeleteRequest returns true if this entry represents a delete request
// (AssetPrice is not present)
func (p *PriceDataEntry) IsDeleteRequest() bool {
	return p.AssetPrice == nil
}

// TokenPairKey returns a unique key for this token pair
func (p *OraclePriceDataEntry) TokenPairKey() string {
	return currencyBytesToString(p.BaseAsset) + "/" + currencyBytesToString(p.QuoteAsset)
}

// currencyBytesToString converts a 20-byte currency to string representation
func currencyBytesToString(c [20]byte) string {
	// Check if it's a standard 3-char ISO currency (bytes 12-14)
	if c[0] == 0 && c[1] == 0 && c[2] == 0 {
		// Standard currency - extract 3 chars from bytes 12-14
		code := make([]byte, 0, 3)
		for i := 12; i < 15; i++ {
			if c[i] != 0 {
				code = append(code, c[i])
			}
		}
		if len(code) > 0 {
			return string(code)
		}
	}
	// Non-standard currency - return hex representation
	return fmt.Sprintf("%X", c)
}

// stringToCurrencyBytes converts a currency string to 20-byte representation
func stringToCurrencyBytes(currency string) [20]byte {
	var result [20]byte

	if len(currency) == 3 {
		// Standard currency code - ASCII in bytes 12-14
		result[12] = currency[0]
		result[13] = currency[1]
		result[14] = currency[2]
	} else if len(currency) == 40 {
		// Hex-encoded currency (non-standard)
		for i := 0; i < 20; i++ {
			result[i] = hexToByte(currency[i*2], currency[i*2+1])
		}
	}

	return result
}

// hexToByte converts two hex characters to a byte
func hexToByte(high, low byte) byte {
	return hexNibble(high)<<4 | hexNibble(low)
}

// hexNibble converts a single hex character to its value
func hexNibble(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 0
	}
}

// CalculateOwnerCountAdjustment calculates the owner count adjustment based on
// the number of price data entries. Oracle entries with >5 pairs count as 2,
// otherwise count as 1.
func CalculateOwnerCountAdjustment(priceDataCount int) int {
	if priceDataCount > 5 {
		return 2
	}
	return 1
}
