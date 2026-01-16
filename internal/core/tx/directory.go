package tx

import (
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"strings"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// DirectoryNode represents a directory ledger entry
type DirectoryNode struct {
	// Common fields
	Flags         uint32
	RootIndex     [32]byte
	Indexes       [][32]byte // List of object keys in this directory page
	IndexNext     uint64     // Next page index (0 if none)
	IndexPrevious uint64     // Previous page index (0 if none)

	// Owner directory specific
	Owner [20]byte // Account that owns this directory (only for owner dirs)

	// Book directory specific (for offer books)
	TakerPaysCurrency [20]byte
	TakerPaysIssuer   [20]byte
	TakerGetsCurrency [20]byte
	TakerGetsIssuer   [20]byte
	ExchangeRate      uint64 // Quality encoded as uint64
}

// GetRate calculates the quality/exchange rate for an offer.
// Quality = TakerPays / TakerGets (what taker pays per unit they get)
// Lower quality = better for taker
// Returns uint64 encoded as: (exponent+100) << 56 | mantissa
func GetRate(takerPays, takerGets Amount) uint64 {
	// Handle zero case
	if takerGets.Value == "" || takerGets.Value == "0" {
		return 0
	}

	// Convert amounts to big.Float for precise calculation
	var paysFloat, getsFloat *big.Float

	if takerPays.IsNative() {
		// XRP amount in drops
		drops, _ := parseDropsString(takerPays.Value)
		paysFloat = new(big.Float).SetUint64(drops)
	} else {
		// IOU amount
		paysFloat, _ = new(big.Float).SetString(takerPays.Value)
		if paysFloat == nil {
			return 0
		}
	}

	if takerGets.IsNative() {
		// XRP amount in drops
		drops, _ := parseDropsString(takerGets.Value)
		getsFloat = new(big.Float).SetUint64(drops)
	} else {
		// IOU amount
		getsFloat, _ = new(big.Float).SetString(takerGets.Value)
		if getsFloat == nil {
			return 0
		}
	}

	// Calculate rate = pays / gets
	rate := new(big.Float).Quo(paysFloat, getsFloat)

	// Convert to mantissa and exponent
	// XRPL uses: mantissa × 10^exponent where mantissa is 10^15 to 10^16-1
	// Quality encoding: (exponent+100) << 56 | mantissa

	if rate.Sign() == 0 {
		return 0
	}

	// Normalize to get mantissa in range [10^15, 10^16)
	mantissa, exponent := normalizeForQuality(rate)

	// Encode: upper 8 bits = exponent+100, lower 56 bits = mantissa
	return uint64(exponent+100)<<56 | mantissa
}

// normalizeForQuality converts a big.Float to mantissa and exponent
// where mantissa is in range [10^15, 10^16)
func normalizeForQuality(f *big.Float) (uint64, int) {
	if f.Sign() == 0 {
		return 0, 0
	}

	// Get the value as a string with high precision
	text := f.Text('e', 15) // Scientific notation with 15 digits

	// Parse the mantissa and exponent from scientific notation
	// Format: "1.234567890123456e+05" or "1.234567890123456e-05"
	var mantissaStr string
	var expStr string
	if idx := strings.Index(text, "e"); idx >= 0 {
		mantissaStr = text[:idx]
		expStr = text[idx+1:]
	} else {
		mantissaStr = text
		expStr = "0"
	}

	// Remove decimal point and parse mantissa
	mantissaStr = strings.Replace(mantissaStr, ".", "", 1)

	// Parse exponent
	var exp int
	if len(expStr) > 0 {
		if expStr[0] == '+' {
			expStr = expStr[1:]
		}
		for _, c := range expStr {
			if c == '-' {
				continue
			}
			exp = exp*10 + int(c-'0')
		}
		if len(expStr) > 0 && expStr[0] == '-' {
			exp = -exp
		}
	}

	// Adjust for the decimal point position
	// We want mantissa to be an integer in [10^15, 10^16)
	mantissaLen := len(mantissaStr)
	if mantissaLen > 16 {
		mantissaStr = mantissaStr[:16]
	}

	// Parse mantissa as uint64
	var mantissa uint64
	for _, c := range mantissaStr {
		if c >= '0' && c <= '9' {
			mantissa = mantissa*10 + uint64(c-'0')
		}
	}

	// Adjust exponent based on mantissa normalization
	// Original: mantissa × 10^exp where mantissa has a decimal point after first digit
	// (e.g., "6.648e+09" means 6.648 × 10^9)
	// After removing decimal, we have integer mantissa (e.g., 6648000000000000)
	// The relationship: original = integer_mantissa × 10^(exp - (digits_after_decimal))
	// Since we have 15 digits after decimal, newExp = exp - 15
	newExp := exp - (mantissaLen - 1)

	// Ensure mantissa is in proper range
	for mantissa < 1000000000000000 && mantissa > 0 {
		mantissa *= 10
		newExp--
	}
	for mantissa >= 10000000000000000 {
		mantissa /= 10
		newExp++
	}

	return mantissa, newExp
}

// serializeDirectoryNode serializes a DirectoryNode to binary format
func serializeDirectoryNode(dir *DirectoryNode, isBookDir bool) ([]byte, error) {
	jsonObj := map[string]any{
		"LedgerEntryType": "DirectoryNode",
		"Flags":           dir.Flags,
		"RootIndex":       strings.ToUpper(hex.EncodeToString(dir.RootIndex[:])),
	}

	// Add Indexes if present
	if len(dir.Indexes) > 0 {
		indexes := make([]string, len(dir.Indexes))
		for i, idx := range dir.Indexes {
			indexes[i] = strings.ToUpper(hex.EncodeToString(idx[:]))
		}
		jsonObj["Indexes"] = indexes
	}

	// Add pagination fields if set
	if dir.IndexNext != 0 {
		jsonObj["IndexNext"] = formatUint64Hex(dir.IndexNext)
	}
	if dir.IndexPrevious != 0 {
		jsonObj["IndexPrevious"] = formatUint64Hex(dir.IndexPrevious)
	}

	if isBookDir {
		// Book directory fields - always include all four fields, even if zero
		// XRPL serialization includes these for book directories
		jsonObj["TakerPaysCurrency"] = strings.ToUpper(hex.EncodeToString(dir.TakerPaysCurrency[:]))
		jsonObj["TakerPaysIssuer"] = strings.ToUpper(hex.EncodeToString(dir.TakerPaysIssuer[:]))
		jsonObj["TakerGetsCurrency"] = strings.ToUpper(hex.EncodeToString(dir.TakerGetsCurrency[:]))
		jsonObj["TakerGetsIssuer"] = strings.ToUpper(hex.EncodeToString(dir.TakerGetsIssuer[:]))
		if dir.ExchangeRate != 0 {
			jsonObj["ExchangeRate"] = formatUint64Hex(dir.ExchangeRate)
		}
	} else {
		// Owner directory - add Owner field
		if dir.Owner != [20]byte{} {
			ownerAddr, err := encodeAccountID(dir.Owner)
			if err == nil {
				jsonObj["Owner"] = ownerAddr
			}
		}
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, err
	}

	return hex.DecodeString(hexStr)
}

// parseDirectoryNode parses a DirectoryNode from binary data
func parseDirectoryNode(data []byte) (*DirectoryNode, error) {
	hexStr := hex.EncodeToString(data)
	jsonObj, err := binarycodec.Decode(hexStr)
	if err != nil {
		return nil, err
	}

	dir := &DirectoryNode{}

	if flags, ok := jsonObj["Flags"].(float64); ok {
		dir.Flags = uint32(flags)
	}

	if rootIndex, ok := jsonObj["RootIndex"].(string); ok {
		rootBytes, _ := hex.DecodeString(rootIndex)
		copy(dir.RootIndex[:], rootBytes)
	}

	// Handle both []string and []any for Indexes (binary codec may return either)
	if indexes, ok := jsonObj["Indexes"].([]string); ok {
		dir.Indexes = make([][32]byte, len(indexes))
		for i, idxStr := range indexes {
			idxBytes, _ := hex.DecodeString(idxStr)
			copy(dir.Indexes[i][:], idxBytes)
		}
	} else if indexes, ok := jsonObj["Indexes"].([]any); ok {
		dir.Indexes = make([][32]byte, len(indexes))
		for i, idx := range indexes {
			if idxStr, ok := idx.(string); ok {
				idxBytes, _ := hex.DecodeString(idxStr)
				copy(dir.Indexes[i][:], idxBytes)
			}
		}
	}

	if indexNext, ok := jsonObj["IndexNext"].(string); ok {
		dir.IndexNext = parseUint64Hex(indexNext)
	}

	if owner, ok := jsonObj["Owner"].(string); ok {
		ownerID, _ := decodeAccountID(owner)
		dir.Owner = ownerID
	}

	return dir, nil
}

// uint64ToBytes converts uint64 to big-endian bytes
func uint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

// parseUint64Hex parses a hex string as uint64
func parseUint64Hex(s string) uint64 {
	// Pad to 16 chars
	for len(s) < 16 {
		s = "0" + s
	}
	b, _ := hex.DecodeString(s)
	return binary.BigEndian.Uint64(b)
}

// DirInsertResult contains the result of a directory insert operation
type DirInsertResult struct {
	Page          uint64   // Page where the item was inserted
	Created       bool     // True if the directory was created
	Modified      bool     // True if an existing directory was modified
	DirKey        [32]byte // Key of the directory node that was modified/created
	PreviousState *DirectoryNode
	NewState      *DirectoryNode
}

// dirInsert adds an item to a directory, creating the directory if needed.
// Returns the page number where the item was inserted.
func (e *Engine) dirInsert(dirKey keylet.Keylet, itemKey [32]byte, setupFunc func(*DirectoryNode)) (*DirInsertResult, error) {
	result := &DirInsertResult{
		DirKey: dirKey.Key,
	}

	// Check if directory exists
	exists, err := e.view.Exists(dirKey)
	if err != nil {
		return nil, err
	}

	var dir *DirectoryNode

	if !exists {
		// Create new directory
		dir = &DirectoryNode{
			RootIndex: dirKey.Key,
			Indexes:   [][32]byte{itemKey},
		}
		if setupFunc != nil {
			setupFunc(dir)
		}
		result.Created = true
		result.Page = 0
	} else {
		// Read existing directory
		data, err := e.view.Read(dirKey)
		if err != nil {
			return nil, err
		}

		dir, err = parseDirectoryNode(data)
		if err != nil {
			return nil, err
		}

		// Save previous state for metadata
		prevDir := *dir
		result.PreviousState = &prevDir

		// Add item to indexes
		dir.Indexes = append(dir.Indexes, itemKey)
		result.Modified = true
		result.Page = 0 // For simplicity, always use page 0
	}

	result.NewState = dir

	// Serialize and store
	// Determine if this is a book directory (has currency fields set)
	isBookDir := dir.TakerPaysCurrency != [20]byte{} || dir.TakerGetsCurrency != [20]byte{}
	data, err := serializeDirectoryNode(dir, isBookDir)
	if err != nil {
		return nil, err
	}

	if result.Created {
		if err := e.view.Insert(dirKey, data); err != nil {
			return nil, err
		}
	} else {
		if err := e.view.Update(dirKey, data); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// getCurrencyBytes converts a currency code to 20 bytes
// For standard 3-char codes: 12 zero bytes + 3 char bytes + 5 zero bytes
// For XRP: all zeros
func getCurrencyBytes(currency string) [20]byte {
	var result [20]byte
	if currency == "" || currency == "XRP" {
		return result // All zeros for XRP
	}

	// Standard 3-character currency code
	if len(currency) == 3 {
		// Format: 12 zero bytes + 3 ASCII bytes + 5 zero bytes
		copy(result[12:15], []byte(currency))
	} else if len(currency) == 40 {
		// Hex-encoded currency (160-bit)
		decoded, _ := hex.DecodeString(currency)
		copy(result[:], decoded)
	}
	return result
}

// getIssuerBytes converts an issuer address to 20-byte account ID
func getIssuerBytes(issuer string) [20]byte {
	if issuer == "" {
		return [20]byte{} // All zeros for XRP
	}
	accountID, _ := decodeAccountID(issuer)
	return accountID
}

// formatUint64Hex formats a uint64 as lowercase hex without leading zeros
func formatUint64Hex(v uint64) string {
	h := hex.EncodeToString(uint64ToBytes(v))
	// Trim leading zeros but keep at least one digit
	h = strings.TrimLeft(h, "0")
	if h == "" {
		h = "0"
	}
	return strings.ToLower(h)
}
