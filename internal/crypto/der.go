package crypto

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

var (
	// ErrInvalidHexString is returned when the hex string is invalid.
	ErrInvalidHexString = errors.New("invalid hex string")

	// ErrInvalidDERNotEnoughData is returned when the DER data is not enough.
	ErrInvalidDERNotEnoughData = errors.New("invalid DER: not enough data")
	// ErrInvalidDERIntegerTag is returned when the DER integer tag is invalid.
	ErrInvalidDERIntegerTag = errors.New("invalid DER: expected integer tag")
	// ErrInvalidDERSignature is returned when the DER signature is invalid.
	ErrInvalidDERSignature = errors.New("invalid signature: incorrect length")
	// ErrLeftoverBytes is returned when there are leftover bytes after parsing the DER signature.
	ErrLeftoverBytes = errors.New("invalid signature: left bytes after parsing")
)

// DERHexFromSig converts r and s hex strings to a DER-encoded signature hex string.
// It returns the DER-encoded signature as a hex string and an error if any occurred during the process.
func DERHexFromSig(rHex, sHex string) (string, error) {
	// Helper function to add leading zero if first byte has negative bit enabled
	slice := func(s string) string {
		if len(s) > 0 && (s[0] >= '8' && s[0] <= 'f') {
			return "00" + s
		}
		return s
	}

	// Helper function to ensure even-length hex string
	ensureEven := func(s string) string {
		if len(s)%2 != 0 {
			return "0" + s
		}
		return s
	}

	// Convert hex strings to big.Int
	r, ok := new(big.Int).SetString(rHex, 16)
	if !ok {
		return "", ErrInvalidHexString
	}
	s, ok := new(big.Int).SetString(sHex, 16)
	if !ok {
		return "", ErrInvalidHexString
	}

	// Convert r and s to sliced hex strings
	rStr := slice(ensureEven(r.Text(16)))
	sStr := slice(ensureEven(s.Text(16)))

	rLen := len(rStr) / 2
	sLen := len(sStr) / 2

	// Convert lengths to hex
	rLenHex := ensureEven(fmt.Sprintf("%x", rLen))
	sLenHex := ensureEven(fmt.Sprintf("%x", sLen))

	// Calculate total length
	totalLen := rLen + sLen + 4
	totalLenHex := ensureEven(fmt.Sprintf("%x", totalLen))

	// Construct the final hex string
	result := strings.Join([]string{
		"30", totalLenHex,
		"02", rLenHex, rStr,
		"02", sLenHex, sStr,
	}, "")

	return result, nil
}

// parseInt parses an integer from DER-encoded data.
// It returns a *big.Int representing the parsed integer, a byte slice containing the remaining data after parsing,
// and an error if any occurred during parsing.
func parseInt(data []byte) (*big.Int, []byte, error) {
	if len(data) < 2 {
		return nil, nil, ErrInvalidDERNotEnoughData
	}
	if data[0] != 0x02 {
		return nil, nil, ErrInvalidDERIntegerTag
	}
	length := int(data[1])
	if len(data) < 2+length {
		return nil, nil, ErrInvalidDERNotEnoughData
	}
	number := new(big.Int).SetBytes(data[2 : 2+length])
	return number, data[2+length:], nil
}

// DERHexToSig converts a DER-encoded signature hex string to r and s byte slices.
// It returns the r and s byte slices and an error if any occurred during the process.
func DERHexToSig(hexSignature string) ([]byte, []byte, error) {
	data, err := hex.DecodeString(hexSignature)
	if err != nil {
		return nil, nil, ErrInvalidHexString
	}

	if len(data) < 2 || data[0] != 0x30 {
		return nil, nil, ErrInvalidDERSignature
	}
	if int(data[1]) != len(data)-2 {
		return nil, nil, ErrInvalidDERSignature
	}

	r, sBytes, err := parseInt(data[2:])
	if err != nil {
		return nil, nil, ErrInvalidDERSignature
	}

	s, leftover, err := parseInt(sBytes)
	if err != nil {
		return nil, nil, ErrInvalidDERSignature
	}

	if len(leftover) > 0 {
		return nil, nil, ErrLeftoverBytes
	}

	return r.Bytes(), s.Bytes(), nil
}
