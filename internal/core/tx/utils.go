package tx

import "strconv"

// ParseUint64Hex parses a hex string as uint64
func ParseUint64Hex(s string) (uint64, error) {
	return strconv.ParseUint(s, 16, 64)
}

// FormatUint64Hex formats a uint64 as lowercase hex without leading zeros
func FormatUint64Hex(v uint64) string {
	return strconv.FormatUint(v, 16)
}
