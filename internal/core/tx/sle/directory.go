package sle

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
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
// This matches rippled's getRate(offerOut, offerIn) which returns in/out.
// Reference: rippled STAmount.h line 693-694:
//   "Rate: smaller is better, the taker wants the most out: in/out"
// Lower rate value = better for taker (they pay less per unit they get)
// Returns uint64 encoded as: (exponent+100) << 56 | mantissa
func GetRate(offerOut, offerIn Amount) uint64 {
	// Handle zero case - check offerOut since we divide by it
	if offerOut.IsZero() {
		return 0
	}

	// Convert amounts to big.Float for precise calculation
	var outFloat, inFloat *big.Float

	if offerOut.IsNative() {
		// XRP amount in drops
		outFloat = new(big.Float).SetPrec(128).SetInt64(offerOut.Drops())
	} else {
		// IOU amount - use mantissa/exponent for precision
		// value = mantissa × 10^exponent
		iou := offerOut.IOU()
		mantissa := iou.Mantissa()
		exponent := iou.Exponent()
		if mantissa == 0 {
			return 0
		}
		// Create big.Float from mantissa
		outFloat = new(big.Float).SetPrec(128).SetInt64(mantissa)
		// Apply exponent: multiply or divide by powers of 10
		if exponent > 0 {
			multiplier := new(big.Float).SetPrec(128).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil))
			outFloat.Mul(outFloat, multiplier)
		} else if exponent < 0 {
			divisor := new(big.Float).SetPrec(128).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exponent)), nil))
			outFloat.Quo(outFloat, divisor)
		}
	}

	if offerIn.IsNative() {
		// XRP amount in drops
		inFloat = new(big.Float).SetPrec(128).SetInt64(offerIn.Drops())
	} else {
		// IOU amount - use mantissa/exponent for precision
		iou := offerIn.IOU()
		mantissa := iou.Mantissa()
		exponent := iou.Exponent()
		// Create big.Float from mantissa
		inFloat = new(big.Float).SetPrec(128).SetInt64(mantissa)
		// Apply exponent
		if exponent > 0 {
			multiplier := new(big.Float).SetPrec(128).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil))
			inFloat.Mul(inFloat, multiplier)
		} else if exponent < 0 {
			divisor := new(big.Float).SetPrec(128).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exponent)), nil))
			inFloat.Quo(inFloat, divisor)
		}
	}

	// Calculate rate = in / out (rippled convention: offerIn/offerOut)
	rate := new(big.Float).SetPrec(128).Quo(inFloat, outFloat)

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

// SerializeDirectoryNode serializes a DirectoryNode to binary format
func SerializeDirectoryNode(dir *DirectoryNode, isBookDir bool) ([]byte, error) {
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

	// Include Owner field if set
	if dir.Owner != [20]byte{} {
		ownerAddr, err := encodeAccountID(dir.Owner)
		if err == nil {
			jsonObj["Owner"] = ownerAddr
		}
	}

	// Include book directory fields if they exist
	// These fields may exist even on owner directory pages (they're stored in ledger state)
	hasBookFields := isBookDir || dir.ExchangeRate != 0 ||
		dir.TakerPaysCurrency != [20]byte{} || dir.TakerPaysIssuer != [20]byte{} ||
		dir.TakerGetsCurrency != [20]byte{} || dir.TakerGetsIssuer != [20]byte{}

	if hasBookFields {
		// Include all four currency/issuer fields
		jsonObj["TakerPaysCurrency"] = strings.ToUpper(hex.EncodeToString(dir.TakerPaysCurrency[:]))
		jsonObj["TakerPaysIssuer"] = strings.ToUpper(hex.EncodeToString(dir.TakerPaysIssuer[:]))
		jsonObj["TakerGetsCurrency"] = strings.ToUpper(hex.EncodeToString(dir.TakerGetsCurrency[:]))
		jsonObj["TakerGetsIssuer"] = strings.ToUpper(hex.EncodeToString(dir.TakerGetsIssuer[:]))
		if dir.ExchangeRate != 0 {
			jsonObj["ExchangeRate"] = formatUint64Hex(dir.ExchangeRate)
		}
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, err
	}

	return hex.DecodeString(hexStr)
}

// ParseDirectoryNode parses a DirectoryNode from binary data
func ParseDirectoryNode(data []byte) (*DirectoryNode, error) {
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

	if indexPrev, ok := jsonObj["IndexPrevious"].(string); ok {
		dir.IndexPrevious = parseUint64Hex(indexPrev)
	}

	if owner, ok := jsonObj["Owner"].(string); ok {
		ownerID, _ := decodeAccountID(owner)
		dir.Owner = ownerID
	}

	// Parse book directory fields (must preserve these even if not used)
	if exchangeRate, ok := jsonObj["ExchangeRate"].(string); ok {
		dir.ExchangeRate = parseUint64Hex(exchangeRate)
	}
	if takerPaysCurrency, ok := jsonObj["TakerPaysCurrency"].(string); ok {
		decoded, _ := hex.DecodeString(takerPaysCurrency)
		copy(dir.TakerPaysCurrency[:], decoded)
	}
	if takerPaysIssuer, ok := jsonObj["TakerPaysIssuer"].(string); ok {
		decoded, _ := hex.DecodeString(takerPaysIssuer)
		copy(dir.TakerPaysIssuer[:], decoded)
	}
	if takerGetsCurrency, ok := jsonObj["TakerGetsCurrency"].(string); ok {
		decoded, _ := hex.DecodeString(takerGetsCurrency)
		copy(dir.TakerGetsCurrency[:], decoded)
	}
	if takerGetsIssuer, ok := jsonObj["TakerGetsIssuer"].(string); ok {
		decoded, _ := hex.DecodeString(takerGetsIssuer)
		copy(dir.TakerGetsIssuer[:], decoded)
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
	// For multi-page support:
	RootModified      bool           // True if root was modified (for IndexPrevious update)
	RootPrevState     *DirectoryNode // Previous state of root (if root was modified)
	RootNewState      *DirectoryNode // New state of root
	NewPageCreated    bool           // True if a new page was created
	NewPageKey        [32]byte       // Key of the new page created
	NewPageState      *DirectoryNode // State of the new page
	PrevPageModified  bool           // True if previous page was modified (IndexNext update)
	PrevPageKey       [32]byte       // Key of the previous page
	PrevPagePrevState *DirectoryNode // Previous state of prev page
	PrevPageNewState  *DirectoryNode // New state of prev page
}

// dirNodeMaxEntries is the maximum number of entries per directory page (matches rippled)
const dirNodeMaxEntries = 32

// dirInsert adds an item to a directory, creating the directory if needed.
// Returns the page number where the item was inserted.
// Follows rippled's dirAdd algorithm for multi-page directory support.
func DirInsert(view LedgerView, dirKey keylet.Keylet, itemKey [32]byte, setupFunc func(*DirectoryNode)) (*DirInsertResult, error) {
	result := &DirInsertResult{
		DirKey: dirKey.Key,
	}

	// Check if root directory exists
	exists, err := view.Exists(dirKey)
	if err != nil {
		return nil, err
	}

	// Determine if this is a book directory based on setup function behavior
	var isBookDir bool
	testDir := &DirectoryNode{}
	if setupFunc != nil {
		setupFunc(testDir)
		isBookDir = testDir.TakerPaysCurrency != [20]byte{} || testDir.TakerGetsCurrency != [20]byte{}
	}

	if !exists {
		// No root exists - create it with the item
		dir := &DirectoryNode{
			RootIndex: dirKey.Key,
			Indexes:   [][32]byte{itemKey},
		}
		if setupFunc != nil {
			setupFunc(dir)
		}
		result.Created = true
		result.Page = 0
		result.NewState = dir
		result.DirKey = dirKey.Key

		// Serialize and store
		data, err := SerializeDirectoryNode(dir, isBookDir)
		if err != nil {
			return nil, err
		}
		if err := view.Insert(dirKey, data); err != nil {
			return nil, err
		}
		return result, nil
	}

	// Root exists - read it
	rootData, err := view.Read(dirKey)
	if err != nil {
		return nil, err
	}
	root, err := ParseDirectoryNode(rootData)
	if err != nil {
		return nil, err
	}

	// Get the last page number from root's IndexPrevious
	page := root.IndexPrevious
	node := root
	nodeKey := dirKey.Key

	// If page != 0, load that page as the node to insert into
	if page != 0 {
		pageKeylet := keylet.DirPage(dirKey.Key, page)
		nodeKey = pageKeylet.Key
		pageData, err := view.Read(pageKeylet)
		if err != nil {
			return nil, err
		}
		node, err = ParseDirectoryNode(pageData)
		if err != nil {
			return nil, err
		}
	}

	// Check if current page has space
	if len(node.Indexes) < dirNodeMaxEntries {
		// Has space - add item to current page
		prevNode := *node
		node.Indexes = append(node.Indexes, itemKey)

		result.Modified = true
		result.Page = page
		result.DirKey = nodeKey
		result.PreviousState = &prevNode
		result.NewState = node

		// Serialize and update
		nodeIsBookDir := node.TakerPaysCurrency != [20]byte{} || node.TakerGetsCurrency != [20]byte{}
		data, err := SerializeDirectoryNode(node, nodeIsBookDir)
		if err != nil {
			return nil, err
		}
		if err := view.Update(keylet.Keylet{Type: dirKey.Type, Key: nodeKey}, data); err != nil {
			return nil, err
		}
		return result, nil
	}

	// Current page is full - need to create a new page
	newPage := page + 1
	newPageKeylet := keylet.DirPage(dirKey.Key, newPage)

	// Save previous states
	prevNode := *node
	prevRoot := *root

	// Update current node's IndexNext to point to new page
	node.IndexNext = newPage

	// Update root's IndexPrevious to point to new page
	root.IndexPrevious = newPage

	// Create new page
	newPageNode := &DirectoryNode{
		RootIndex: dirKey.Key,
		Indexes:   [][32]byte{itemKey},
	}
	// Set IndexPrevious on new page (unless it's page 1)
	if newPage != 1 {
		newPageNode.IndexPrevious = newPage - 1
	}
	// Copy book directory fields if applicable
	if setupFunc != nil {
		setupFunc(newPageNode)
	}

	// Store results
	result.Page = newPage
	result.DirKey = newPageKeylet.Key
	result.NewPageCreated = true
	result.NewPageKey = newPageKeylet.Key
	result.NewPageState = newPageNode

	// Track root modification
	result.RootModified = true
	result.RootPrevState = &prevRoot
	result.RootNewState = root

	// Track previous page modification (if not root)
	if page != 0 {
		result.PrevPageModified = true
		result.PrevPageKey = nodeKey
		result.PrevPagePrevState = &prevNode
		result.PrevPageNewState = node
	} else {
		// Previous page was root, already tracked above
		result.PrevPageModified = false
	}

	// Serialize and store all changes

	// 1. Update current page (node) with new IndexNext
	nodeIsBookDir := node.TakerPaysCurrency != [20]byte{} || node.TakerGetsCurrency != [20]byte{}
	nodeData, err := SerializeDirectoryNode(node, nodeIsBookDir)
	if err != nil {
		return nil, err
	}
	if err := view.Update(keylet.Keylet{Type: dirKey.Type, Key: nodeKey}, nodeData); err != nil {
		return nil, err
	}

	// 2. Update root with new IndexPrevious (only if root != node)
	if page != 0 {
		rootIsBookDir := root.TakerPaysCurrency != [20]byte{} || root.TakerGetsCurrency != [20]byte{}
		rootData, err := SerializeDirectoryNode(root, rootIsBookDir)
		if err != nil {
			return nil, err
		}
		if err := view.Update(dirKey, rootData); err != nil {
			return nil, err
		}
	}

	// 3. Insert new page
	newPageIsBookDir := newPageNode.TakerPaysCurrency != [20]byte{} || newPageNode.TakerGetsCurrency != [20]byte{}
	newPageData, err := SerializeDirectoryNode(newPageNode, newPageIsBookDir)
	if err != nil {
		return nil, err
	}
	if err := view.Insert(newPageKeylet, newPageData); err != nil {
		return nil, err
	}

	return result, nil
}

// GetCurrencyBytes converts a currency code to 20 bytes
// For standard 3-char codes: 12 zero bytes + 3 char bytes + 5 zero bytes
// For XRP: all zeros
func GetCurrencyBytes(currency string) [20]byte {
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

// GetIssuerBytes converts an issuer address to 20-byte account ID
func GetIssuerBytes(issuer string) [20]byte {
	if issuer == "" {
		return [20]byte{} // All zeros for XRP
	}
	accountID, _ := DecodeAccountID(issuer)
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

// formatUint64HexPadded formats a uint64 as 16-char uppercase hex with leading zeros
// Used for fields like OwnerNode and BookNode that require zero-padding in metadata
func formatUint64HexPadded(v uint64) string {
	return strings.ToUpper(hex.EncodeToString(uint64ToBytes(v)))
}

// DirRemoveResult contains the result of a directory remove operation
type DirRemoveResult struct {
	Success       bool                    // True if the item was found and removed
	PageModified  bool                    // True if the page was modified but not deleted
	PageDeleted   bool                    // True if the page was deleted (became empty)
	RootDeleted   bool                    // True if the entire directory was deleted
	ModifiedNodes []DirRemoveModifiedNode // Nodes that were modified
	DeletedNodes  []DirRemoveDeletedNode  // Nodes that were deleted
}

// DirRemoveModifiedNode tracks a modified directory node
type DirRemoveModifiedNode struct {
	Key       [32]byte
	PrevState *DirectoryNode
	NewState  *DirectoryNode
}

// DirRemoveDeletedNode tracks a deleted directory node
type DirRemoveDeletedNode struct {
	Key        [32]byte
	FinalState *DirectoryNode
}

// dirRemove removes an item from a directory.
// Follows rippled's dirRemove algorithm for proper page cleanup.
// Parameters:
//   - directory: keylet for the directory (root)
//   - page: the page number where the item is located (from OwnerNode/BookNode field)
//   - key: the item key to remove
//   - keepRoot: if true, don't delete the root even if empty
func DirRemove(view LedgerView, directory keylet.Keylet, page uint64, itemKey [32]byte, keepRoot bool) (*DirRemoveResult, error) {
	result := &DirRemoveResult{
		ModifiedNodes: make([]DirRemoveModifiedNode, 0),
		DeletedNodes:  make([]DirRemoveDeletedNode, 0),
	}

	const rootPage uint64 = 0

	// Get the page where the item should be
	pageKeylet := keylet.DirPage(directory.Key, page)
	pageData, err := view.Read(pageKeylet)
	if err != nil {
		return result, nil // Page not found, return success=false
	}
	node, err := ParseDirectoryNode(pageData)
	if err != nil {
		return nil, err
	}

	// Find and remove the item from Indexes
	found := false
	newIndexes := make([][32]byte, 0, len(node.Indexes))
	for _, idx := range node.Indexes {
		if idx == itemKey {
			found = true
		} else {
			newIndexes = append(newIndexes, idx)
		}
	}

	if !found {
		return result, nil // Item not found
	}

	result.Success = true
	prevNode := *node
	node.Indexes = newIndexes

	// If page still has entries, just update it
	if len(node.Indexes) > 0 {
		result.PageModified = true
		result.ModifiedNodes = append(result.ModifiedNodes, DirRemoveModifiedNode{
			Key:       pageKeylet.Key,
			PrevState: &prevNode,
			NewState:  node,
		})

		// Serialize and update
		isBookDir := node.TakerPaysCurrency != [20]byte{} || node.TakerGetsCurrency != [20]byte{}
		data, err := SerializeDirectoryNode(node, isBookDir)
		if err != nil {
			return nil, err
		}
		if err := view.Update(pageKeylet, data); err != nil {
			return nil, err
		}
		return result, nil
	}

	// Page is now empty - need to handle page deletion
	prevPage := node.IndexPrevious
	nextPage := node.IndexNext

	// Handle root page specially
	if page == rootPage {
		// Check for consistency
		if nextPage == page && prevPage != page {
			return nil, fmt.Errorf("directory chain: fwd link broken")
		}
		if prevPage == page && nextPage != page {
			return nil, fmt.Errorf("directory chain: rev link broken")
		}

		// Handle legacy empty trailing pages
		if nextPage == prevPage && nextPage != page {
			lastPageKeylet := keylet.DirPage(directory.Key, nextPage)
			lastPageData, err := view.Read(lastPageKeylet)
			if err != nil {
				return nil, fmt.Errorf("directory chain: fwd link broken")
			}
			lastPage, err := ParseDirectoryNode(lastPageData)
			if err != nil {
				return nil, err
			}

			if len(lastPage.Indexes) == 0 {
				// Update root's linked list
				node.IndexNext = rootPage
				node.IndexPrevious = rootPage

				// Track root modification
				result.ModifiedNodes = append(result.ModifiedNodes, DirRemoveModifiedNode{
					Key:       pageKeylet.Key,
					PrevState: &prevNode,
					NewState:  node,
				})

				// Track last page deletion
				result.DeletedNodes = append(result.DeletedNodes, DirRemoveDeletedNode{
					Key:        lastPageKeylet.Key,
					FinalState: lastPage,
				})

				// Serialize root update
				isBookDir := node.TakerPaysCurrency != [20]byte{} || node.TakerGetsCurrency != [20]byte{}
				data, err := SerializeDirectoryNode(node, isBookDir)
				if err != nil {
					return nil, err
				}
				if err := view.Update(pageKeylet, data); err != nil {
					return nil, err
				}

				// Erase last page
				if err := view.Erase(lastPageKeylet); err != nil {
					return nil, err
				}

				nextPage = rootPage
				prevPage = rootPage
			}
		}

		if keepRoot {
			// Just mark as modified if we changed it
			if prevNode.IndexNext != node.IndexNext || prevNode.IndexPrevious != node.IndexPrevious {
				// Already tracked above
			} else {
				// Track modification for removing the item
				result.PageModified = true
				result.ModifiedNodes = append(result.ModifiedNodes, DirRemoveModifiedNode{
					Key:       pageKeylet.Key,
					PrevState: &prevNode,
					NewState:  node,
				})

				isBookDir := node.TakerPaysCurrency != [20]byte{} || node.TakerGetsCurrency != [20]byte{}
				data, err := SerializeDirectoryNode(node, isBookDir)
				if err != nil {
					return nil, err
				}
				if err := view.Update(pageKeylet, data); err != nil {
					return nil, err
				}
			}
			return result, nil
		}

		// If no other pages, erase the root
		if nextPage == rootPage && prevPage == rootPage {
			result.PageDeleted = true
			result.RootDeleted = true
			result.DeletedNodes = append(result.DeletedNodes, DirRemoveDeletedNode{
				Key:        pageKeylet.Key,
				FinalState: &prevNode, // Use state before item removal
			})

			if err := view.Erase(pageKeylet); err != nil {
				return nil, err
			}
		} else {
			// Root not empty but we removed an item - just update
			result.PageModified = true
			result.ModifiedNodes = append(result.ModifiedNodes, DirRemoveModifiedNode{
				Key:       pageKeylet.Key,
				PrevState: &prevNode,
				NewState:  node,
			})

			isBookDir := node.TakerPaysCurrency != [20]byte{} || node.TakerGetsCurrency != [20]byte{}
			data, err := SerializeDirectoryNode(node, isBookDir)
			if err != nil {
				return nil, err
			}
			if err := view.Update(pageKeylet, data); err != nil {
				return nil, err
			}
		}

		return result, nil
	}

	// Non-root page - need to unlink from chain and delete

	// Consistency checks
	if nextPage == page {
		return nil, fmt.Errorf("directory chain: fwd link broken")
	}
	if prevPage == page {
		return nil, fmt.Errorf("directory chain: rev link broken")
	}

	// Get prev and next pages
	prevPageKeylet := keylet.DirPage(directory.Key, prevPage)
	prevPageData, err := view.Read(prevPageKeylet)
	if err != nil {
		return nil, fmt.Errorf("directory chain: fwd link broken")
	}
	prev, err := ParseDirectoryNode(prevPageData)
	if err != nil {
		return nil, err
	}
	prevPrev := *prev

	nextPageKeylet := keylet.DirPage(directory.Key, nextPage)
	nextPageData, err := view.Read(nextPageKeylet)
	if err != nil {
		return nil, fmt.Errorf("directory chain: rev link broken")
	}
	next, err := ParseDirectoryNode(nextPageData)
	if err != nil {
		return nil, err
	}
	nextPrev := *next

	// Unlink: prev.IndexNext = nextPage
	prev.IndexNext = nextPage
	// Unlink: next.IndexPrevious = prevPage
	next.IndexPrevious = prevPage

	// Track prev modification
	result.ModifiedNodes = append(result.ModifiedNodes, DirRemoveModifiedNode{
		Key:       prevPageKeylet.Key,
		PrevState: &prevPrev,
		NewState:  prev,
	})

	// Track next modification (only if different from prev)
	if nextPageKeylet.Key != prevPageKeylet.Key {
		result.ModifiedNodes = append(result.ModifiedNodes, DirRemoveModifiedNode{
			Key:       nextPageKeylet.Key,
			PrevState: &nextPrev,
			NewState:  next,
		})
	}

	// Serialize prev update
	prevIsBookDir := prev.TakerPaysCurrency != [20]byte{} || prev.TakerGetsCurrency != [20]byte{}
	prevData, err := SerializeDirectoryNode(prev, prevIsBookDir)
	if err != nil {
		return nil, err
	}
	if err := view.Update(prevPageKeylet, prevData); err != nil {
		return nil, err
	}

	// Serialize next update (only if different from prev)
	if nextPageKeylet.Key != prevPageKeylet.Key {
		nextIsBookDir := next.TakerPaysCurrency != [20]byte{} || next.TakerGetsCurrency != [20]byte{}
		nextData, err := SerializeDirectoryNode(next, nextIsBookDir)
		if err != nil {
			return nil, err
		}
		if err := view.Update(nextPageKeylet, nextData); err != nil {
			return nil, err
		}
	}

	// Delete the now-empty page
	result.PageDeleted = true
	result.DeletedNodes = append(result.DeletedNodes, DirRemoveDeletedNode{
		Key:        pageKeylet.Key,
		FinalState: &prevNode,
	})
	if err := view.Erase(pageKeylet); err != nil {
		return nil, err
	}

	// Check if next page is now the last page and empty - clean it up
	if nextPage != rootPage && next.IndexNext == rootPage && len(next.Indexes) == 0 {
		// Delete next as well
		result.DeletedNodes = append(result.DeletedNodes, DirRemoveDeletedNode{
			Key:        nextPageKeylet.Key,
			FinalState: &nextPrev,
		})
		if err := view.Erase(nextPageKeylet); err != nil {
			return nil, err
		}

		// Update prev to point to root
		prev.IndexNext = rootPage
		// Re-serialize prev
		prevData, err := SerializeDirectoryNode(prev, prevIsBookDir)
		if err != nil {
			return nil, err
		}
		if err := view.Update(prevPageKeylet, prevData); err != nil {
			return nil, err
		}

		// Update root's IndexPrevious
		rootKeylet := keylet.DirPage(directory.Key, rootPage)
		rootData, err := view.Read(rootKeylet)
		if err != nil {
			return nil, err
		}
		root, err := ParseDirectoryNode(rootData)
		if err != nil {
			return nil, err
		}
		rootPrev := *root
		root.IndexPrevious = prevPage

		result.ModifiedNodes = append(result.ModifiedNodes, DirRemoveModifiedNode{
			Key:       rootKeylet.Key,
			PrevState: &rootPrev,
			NewState:  root,
		})

		rootIsBookDir := root.TakerPaysCurrency != [20]byte{} || root.TakerGetsCurrency != [20]byte{}
		rootData, err = SerializeDirectoryNode(root, rootIsBookDir)
		if err != nil {
			return nil, err
		}
		if err := view.Update(rootKeylet, rootData); err != nil {
			return nil, err
		}

		nextPage = rootPage
	}

	// If not keeping root, check if prev is root and now empty
	if !keepRoot && nextPage == rootPage && prevPage == rootPage {
		if len(prev.Indexes) == 0 {
			// Delete root as well
			result.RootDeleted = true
			result.DeletedNodes = append(result.DeletedNodes, DirRemoveDeletedNode{
				Key:        prevPageKeylet.Key,
				FinalState: &prevPrev,
			})
			if err := view.Erase(prevPageKeylet); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}
