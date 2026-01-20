package tx

import (
	"encoding/hex"
	"errors"
	"strings"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
)

// Oracle constants matching rippled Protocol.h
const (
	// MaxOracleURI is the maximum length of a URI inside an Oracle
	MaxOracleURI = 256

	// MaxOracleProvider is the maximum length of a Provider inside an Oracle
	MaxOracleProvider = 256

	// MaxOracleDataSeries is the maximum size of a data series array inside an Oracle
	MaxOracleDataSeries = 10

	// MaxOracleSymbolClass (AssetClass) is the maximum length of an AssetClass inside an Oracle
	MaxOracleSymbolClass = 16

	// MaxLastUpdateTimeDelta is the maximum allowed time difference between lastUpdateTime
	// and the time of the last closed ledger (in seconds)
	MaxLastUpdateTimeDelta = 300

	// MaxPriceScale is the maximum price scaling factor
	MaxPriceScale = 20
)

// Oracle validation errors
var (
	ErrOracleEmptyPriceData     = errors.New("temARRAY_EMPTY: PriceDataSeries is empty")
	ErrOracleTooManyPriceData   = errors.New("temARRAY_TOO_LARGE: PriceDataSeries exceeds maximum")
	ErrOracleProviderTooLong    = errors.New("temMALFORMED: Provider exceeds maximum length")
	ErrOracleProviderEmpty      = errors.New("temMALFORMED: Provider is empty")
	ErrOracleURITooLong         = errors.New("temMALFORMED: URI exceeds maximum length")
	ErrOracleURIEmpty           = errors.New("temMALFORMED: URI is empty")
	ErrOracleAssetClassTooLong  = errors.New("temMALFORMED: AssetClass exceeds maximum length")
	ErrOracleAssetClassEmpty    = errors.New("temMALFORMED: AssetClass is empty")
	ErrOracleSameAssets         = errors.New("temMALFORMED: BaseAsset and QuoteAsset must be different")
	ErrOracleDuplicatePair      = errors.New("temMALFORMED: duplicate token pair in PriceDataSeries")
	ErrOracleScaleTooLarge      = errors.New("temMALFORMED: Scale exceeds maximum")
	ErrOracleDeleteWithoutPrice = errors.New("temMALFORMED: cannot delete token pair when creating oracle")
	ErrOracleMissingProvider    = errors.New("temMALFORMED: Provider is required when creating oracle")
	ErrOracleMissingAssetClass  = errors.New("temMALFORMED: AssetClass is required when creating oracle")
	ErrOracleInvalidUpdateTime  = errors.New("tecINVALID_UPDATE_TIME: LastUpdateTime is invalid")
	ErrOracleNotNewer           = errors.New("tecINVALID_UPDATE_TIME: LastUpdateTime must be more recent")
	ErrOracleTokenPairNotFound  = errors.New("tecTOKEN_PAIR_NOT_FOUND: token pair to delete does not exist")
	ErrOracleArrayEmpty         = errors.New("tecARRAY_EMPTY: resulting PriceDataSeries would be empty")
	ErrOracleArrayTooLarge      = errors.New("tecARRAY_TOO_LARGE: resulting PriceDataSeries exceeds maximum")
	ErrOracleProviderMismatch   = errors.New("temMALFORMED: Provider does not match existing oracle")
	ErrOracleAssetClassMismatch = errors.New("temMALFORMED: AssetClass does not match existing oracle")
)

// OraclePriceDataEntry represents a single price data entry in an Oracle ledger entry.
// Unlike the transaction PriceDataEntry, this always has AssetPrice (no deletions stored).
type OraclePriceDataEntry struct {
	BaseAsset  string // Currency code for base asset
	QuoteAsset string // Currency code for quote asset
	AssetPrice uint64 // The price (scaled)
	Scale      uint8  // Decimal scale (optional, defaults to 0)
	HasScale   bool   // Whether scale was explicitly set
}

// OracleEntry represents an Oracle ledger entry
// Reference: rippled ledger_entries.macro ltORACLE (0x0080)
type OracleEntry struct {
	// Required fields
	Owner           [20]byte               // Account that owns this oracle
	Provider        string                 // Provider identifier (hex-encoded, up to 256 bytes decoded)
	AssetClass      string                 // Asset class identifier (hex-encoded, up to 16 bytes decoded)
	LastUpdateTime  uint32                 // Unix timestamp of last update (Ripple epoch)
	PriceDataSeries []OraclePriceDataEntry // Array of price data points (max 10)
	OwnerNode       uint64                 // Directory page hint

	// Optional fields
	URI *string // URI for additional oracle information (hex-encoded)

	// Transaction threading
	PreviousTxnID     [32]byte
	PreviousTxnLgrSeq uint32
}

// TokenPairKey returns a unique key for a token pair
func TokenPairKey(baseAsset, quoteAsset string) string {
	return baseAsset + ":" + quoteAsset
}

// parseOracleEntry parses an Oracle ledger entry from binary data
func parseOracleEntry(data []byte) (*OracleEntry, error) {
	hexStr := hex.EncodeToString(data)
	jsonObj, err := binarycodec.Decode(hexStr)
	if err != nil {
		return nil, err
	}

	oracle := &OracleEntry{}

	// Parse Owner
	if owner, ok := jsonObj["Owner"].(string); ok {
		ownerID, err := decodeAccountID(owner)
		if err == nil {
			oracle.Owner = ownerID
		}
	}

	// Parse Provider (Blob/VL field stored as hex)
	if provider, ok := jsonObj["Provider"].(string); ok {
		oracle.Provider = provider
	}

	// Parse AssetClass (Blob/VL field stored as hex)
	if assetClass, ok := jsonObj["AssetClass"].(string); ok {
		oracle.AssetClass = assetClass
	}

	// Parse LastUpdateTime
	if lastUpdateTime, ok := jsonObj["LastUpdateTime"].(float64); ok {
		oracle.LastUpdateTime = uint32(lastUpdateTime)
	}

	// Parse OwnerNode
	if ownerNode, ok := jsonObj["OwnerNode"].(string); ok {
		oracle.OwnerNode = parseUint64Hex(ownerNode)
	}

	// Parse URI (optional)
	if uri, ok := jsonObj["URI"].(string); ok {
		oracle.URI = &uri
	}

	// Parse PriceDataSeries
	if priceData, ok := jsonObj["PriceDataSeries"].([]any); ok {
		for _, item := range priceData {
			if pdMap, ok := item.(map[string]any); ok {
				// The structure is {"PriceData": {...}}
				var innerMap map[string]any
				if inner, ok := pdMap["PriceData"].(map[string]any); ok {
					innerMap = inner
				} else {
					innerMap = pdMap
				}

				entry := OraclePriceDataEntry{}

				// Parse BaseAsset (Currency field)
				if baseAsset, ok := innerMap["BaseAsset"].(string); ok {
					entry.BaseAsset = baseAsset
				}

				// Parse QuoteAsset (Currency field)
				if quoteAsset, ok := innerMap["QuoteAsset"].(string); ok {
					entry.QuoteAsset = quoteAsset
				}

				// Parse AssetPrice
				if assetPrice, ok := innerMap["AssetPrice"].(string); ok {
					// AssetPrice is encoded as hex uint64
					entry.AssetPrice = parseUint64Hex(assetPrice)
				} else if assetPrice, ok := innerMap["AssetPrice"].(float64); ok {
					entry.AssetPrice = uint64(assetPrice)
				}

				// Parse Scale (optional)
				if scale, ok := innerMap["Scale"].(float64); ok {
					entry.Scale = uint8(scale)
					entry.HasScale = true
				}

				oracle.PriceDataSeries = append(oracle.PriceDataSeries, entry)
			}
		}
	}

	// Parse PreviousTxnID
	if prevTxnID, ok := jsonObj["PreviousTxnID"].(string); ok {
		bytes, _ := hex.DecodeString(prevTxnID)
		copy(oracle.PreviousTxnID[:], bytes)
	}

	// Parse PreviousTxnLgrSeq
	if prevSeq, ok := jsonObj["PreviousTxnLgrSeq"].(float64); ok {
		oracle.PreviousTxnLgrSeq = uint32(prevSeq)
	}

	return oracle, nil
}

// serializeOracleEntry serializes an Oracle entry to binary format
func serializeOracleEntry(oracle *OracleEntry) ([]byte, error) {
	jsonObj := map[string]any{
		"LedgerEntryType": "Oracle",
	}

	// Add Owner
	ownerStr, err := encodeAccountID(oracle.Owner)
	if err == nil && ownerStr != "" {
		jsonObj["Owner"] = ownerStr
	}

	// Add Provider (hex-encoded)
	if oracle.Provider != "" {
		jsonObj["Provider"] = strings.ToUpper(oracle.Provider)
	}

	// Add AssetClass (hex-encoded)
	if oracle.AssetClass != "" {
		jsonObj["AssetClass"] = strings.ToUpper(oracle.AssetClass)
	}

	// Add LastUpdateTime
	jsonObj["LastUpdateTime"] = oracle.LastUpdateTime

	// Add OwnerNode
	jsonObj["OwnerNode"] = formatUint64Hex(oracle.OwnerNode)

	// Add URI (optional, hex-encoded)
	if oracle.URI != nil && *oracle.URI != "" {
		jsonObj["URI"] = strings.ToUpper(*oracle.URI)
	}

	// Add PriceDataSeries
	priceDataArray := make([]map[string]any, 0, len(oracle.PriceDataSeries))
	for _, pd := range oracle.PriceDataSeries {
		innerData := map[string]any{
			"BaseAsset":  pd.BaseAsset,
			"QuoteAsset": pd.QuoteAsset,
		}
		// Only include AssetPrice and Scale if they are present
		// (for entries that had their price removed, we don't include these)
		if pd.AssetPrice > 0 {
			innerData["AssetPrice"] = formatUint64Hex(pd.AssetPrice)
		}
		if pd.HasScale {
			innerData["Scale"] = pd.Scale
		}

		priceDataArray = append(priceDataArray, map[string]any{
			"PriceData": innerData,
		})
	}
	jsonObj["PriceDataSeries"] = priceDataArray

	// Add PreviousTxnID
	var zeroHash [32]byte
	if oracle.PreviousTxnID != zeroHash {
		jsonObj["PreviousTxnID"] = strings.ToUpper(hex.EncodeToString(oracle.PreviousTxnID[:]))
	}

	// Add PreviousTxnLgrSeq
	if oracle.PreviousTxnLgrSeq > 0 {
		jsonObj["PreviousTxnLgrSeq"] = oracle.PreviousTxnLgrSeq
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, err
	}

	return hex.DecodeString(hexStr)
}

// calculateOracleReserveCount calculates the number of reserve units needed for an oracle.
// Oracle with >5 price pairs requires 2 reserve units, otherwise 1.
// Reference: rippled SetOracle.cpp line 156-158, 167, 322
func calculateOracleReserveCount(numPairs int) int {
	if numPairs > 5 {
		return 2
	}
	return 1
}
