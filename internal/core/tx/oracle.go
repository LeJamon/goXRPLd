package tx

import (
	"errors"
	"fmt"
)

// Transaction flag constants matching rippled TxFlags.h
const (
	// TfFullyCanonicalSig indicates the signature is fully canonical
	TfFullyCanonicalSig uint32 = 0x80000000

	// TfInnerBatchTxn indicates this is an inner batch transaction
	TfInnerBatchTxn uint32 = 0x40000000

	// TfUniversal is the combination of flags allowed on all transactions
	TfUniversal uint32 = TfFullyCanonicalSig | TfInnerBatchTxn

	// TfUniversalMask is used to check for invalid flags (any bit not in TfUniversal)
	TfUniversalMask uint32 = ^TfUniversal
)

// Oracle constants matching rippled Protocol.h
const (
	// MaxOracleURI is the maximum length of the URI field (in bytes)
	MaxOracleURI = 256

	// MaxOracleProvider is the maximum length of the Provider field (in bytes)
	MaxOracleProvider = 256

	// MaxOracleDataSeries is the maximum number of price data entries
	MaxOracleDataSeries = 10

	// MaxOracleSymbolClass is the maximum length of the AssetClass field (in bytes)
	MaxOracleSymbolClass = 16

	// MaxLastUpdateTimeDelta is the maximum allowed delta between LastUpdateTime
	// and the ledger close time (in seconds)
	MaxLastUpdateTimeDelta = 300

	// MaxPriceScale is the maximum allowed scale value for price data
	MaxPriceScale = 20

	// RippleEpochOffset is the number of seconds between Unix epoch (Jan 1, 1970)
	// and Ripple epoch (Jan 1, 2000). This equals 946684800 seconds.
	RippleEpochOffset = 946684800
)

// OracleSet creates or updates a price oracle.
type OracleSet struct {
	BaseTx

	// OracleDocumentID identifies this oracle (required)
	OracleDocumentID uint32 `json:"OracleDocumentID"`

	// Provider is the oracle provider name (required for creation)
	Provider string `json:"Provider,omitempty"`

	// URI is the URI for the oracle data (optional)
	URI string `json:"URI,omitempty"`

	// AssetClass is the asset class for pricing (required for creation)
	AssetClass string `json:"AssetClass,omitempty"`

	// LastUpdateTime is the timestamp of the last update (required)
	// This is in Ripple epoch (seconds since Jan 1, 2000)
	LastUpdateTime uint32 `json:"LastUpdateTime"`

	// PriceDataSeries is the price data (required)
	PriceDataSeries []PriceData `json:"PriceDataSeries"`
}

// PriceData represents a price data point wrapper
type PriceData struct {
	PriceData PriceDataEntry `json:"PriceData"`
}

// PriceDataEntry contains the price data fields
type PriceDataEntry struct {
	BaseAsset  string  `json:"BaseAsset"`
	QuoteAsset string  `json:"QuoteAsset"`
	AssetPrice *uint64 `json:"AssetPrice,omitempty"` // Omitted = delete this pair
	Scale      *uint8  `json:"Scale,omitempty"`
}

// TokenPairKey returns a unique key for this token pair (for deduplication)
func (p *PriceDataEntry) TokenPairKey() string {
	return p.BaseAsset + "/" + p.QuoteAsset
}

// IsDeleteRequest returns true if this entry represents a delete request
// (AssetPrice is not present)
func (p *PriceDataEntry) IsDeleteRequest() bool {
	return p.AssetPrice == nil
}

// NewOracleSet creates a new OracleSet transaction
func NewOracleSet(account string, oracleDocID uint32, lastUpdateTime uint32) *OracleSet {
	return &OracleSet{
		BaseTx:           *NewBaseTx(TypeOracleSet, account),
		OracleDocumentID: oracleDocID,
		LastUpdateTime:   lastUpdateTime,
	}
}

// TxType returns the transaction type
func (o *OracleSet) TxType() Type {
	return TypeOracleSet
}

// Validate validates the OracleSet transaction (preflight validation)
// This matches rippled's SetOracle::preflight()
func (o *OracleSet) Validate() error {
	if err := o.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags - only universal flags allowed
	if o.Flags != nil && *o.Flags&TfUniversalMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags set")
	}

	// PriceDataSeries must not be empty
	if len(o.PriceDataSeries) == 0 {
		return errors.New("temARRAY_EMPTY: PriceDataSeries is required")
	}

	// Max 10 price data entries
	if len(o.PriceDataSeries) > MaxOracleDataSeries {
		return fmt.Errorf("temARRAY_TOO_LARGE: cannot have more than %d PriceDataSeries entries", MaxOracleDataSeries)
	}

	// Validate field lengths
	if o.Provider != "" {
		if len(o.Provider) == 0 || len(o.Provider) > MaxOracleProvider {
			return fmt.Errorf("temMALFORMED: Provider length must be between 1 and %d bytes", MaxOracleProvider)
		}
	}

	if o.URI != "" {
		if len(o.URI) == 0 || len(o.URI) > MaxOracleURI {
			return fmt.Errorf("temMALFORMED: URI length must be between 1 and %d bytes", MaxOracleURI)
		}
	}

	if o.AssetClass != "" {
		if len(o.AssetClass) == 0 || len(o.AssetClass) > MaxOracleSymbolClass {
			return fmt.Errorf("temMALFORMED: AssetClass length must be between 1 and %d bytes", MaxOracleSymbolClass)
		}
	}

	return nil
}

// ValidatePriceDataSeries performs detailed validation of the price data series
// This is called during preclaim when we know whether this is a create or update
// Returns: pairsToAdd, pairsToDelete, error
func (o *OracleSet) ValidatePriceDataSeries(isUpdate bool) (map[string]PriceDataEntry, map[string]struct{}, error) {
	pairsToAdd := make(map[string]PriceDataEntry)
	pairsToDelete := make(map[string]struct{})

	for _, pd := range o.PriceDataSeries {
		entry := pd.PriceData

		// BaseAsset and QuoteAsset must be different
		if entry.BaseAsset == entry.QuoteAsset {
			return nil, nil, errors.New("temMALFORMED: BaseAsset and QuoteAsset must be different")
		}

		key := entry.TokenPairKey()

		// Check for duplicates in both sets
		if _, exists := pairsToAdd[key]; exists {
			return nil, nil, errors.New("temMALFORMED: duplicate token pair in PriceDataSeries")
		}
		if _, exists := pairsToDelete[key]; exists {
			return nil, nil, errors.New("temMALFORMED: duplicate token pair in PriceDataSeries")
		}

		// Validate Scale if present
		if entry.Scale != nil && *entry.Scale > MaxPriceScale {
			return nil, nil, fmt.Errorf("temMALFORMED: Scale cannot exceed %d", MaxPriceScale)
		}

		if entry.AssetPrice != nil {
			// This is an add/update operation
			pairsToAdd[key] = entry
		} else {
			// This is a delete operation (AssetPrice not present)
			if isUpdate {
				pairsToDelete[key] = struct{}{}
			} else {
				// Cannot delete on create
				return nil, nil, errors.New("temMALFORMED: cannot delete token pair on oracle creation")
			}
		}
	}

	return pairsToAdd, pairsToDelete, nil
}

// Flatten returns a flat map of all transaction fields
func (o *OracleSet) Flatten() (map[string]any, error) {
	m := o.Common.ToMap()

	m["OracleDocumentID"] = o.OracleDocumentID
	m["LastUpdateTime"] = o.LastUpdateTime
	m["PriceDataSeries"] = o.PriceDataSeries

	if o.Provider != "" {
		m["Provider"] = o.Provider
	}
	if o.URI != "" {
		m["URI"] = o.URI
	}
	if o.AssetClass != "" {
		m["AssetClass"] = o.AssetClass
	}

	return m, nil
}

// AddPriceData adds a price data entry with price and scale
func (o *OracleSet) AddPriceData(baseAsset, quoteAsset string, price uint64, scale uint8) {
	o.PriceDataSeries = append(o.PriceDataSeries, PriceData{
		PriceData: PriceDataEntry{
			BaseAsset:  baseAsset,
			QuoteAsset: quoteAsset,
			AssetPrice: &price,
			Scale:      &scale,
		},
	})
}

// AddPriceDataDelete adds a delete request for a token pair
func (o *OracleSet) AddPriceDataDelete(baseAsset, quoteAsset string) {
	o.PriceDataSeries = append(o.PriceDataSeries, PriceData{
		PriceData: PriceDataEntry{
			BaseAsset:  baseAsset,
			QuoteAsset: quoteAsset,
			// AssetPrice and Scale are nil, indicating delete
		},
	})
}

// OracleDelete deletes a price oracle.
type OracleDelete struct {
	BaseTx

	// OracleDocumentID identifies the oracle to delete (required)
	OracleDocumentID uint32 `json:"OracleDocumentID"`
}

// NewOracleDelete creates a new OracleDelete transaction
func NewOracleDelete(account string, oracleDocID uint32) *OracleDelete {
	return &OracleDelete{
		BaseTx:           *NewBaseTx(TypeOracleDelete, account),
		OracleDocumentID: oracleDocID,
	}
}

// TxType returns the transaction type
func (o *OracleDelete) TxType() Type {
	return TypeOracleDelete
}

// Validate validates the OracleDelete transaction (preflight validation)
// This matches rippled's DeleteOracle::preflight()
func (o *OracleDelete) Validate() error {
	if err := o.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags - only universal flags allowed
	if o.Flags != nil && *o.Flags&TfUniversalMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags set")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (o *OracleDelete) Flatten() (map[string]any, error) {
	m := o.Common.ToMap()
	m["OracleDocumentID"] = o.OracleDocumentID
	return m, nil
}

// OracleEntry represents a price oracle ledger entry
// This mirrors the structure in entry/entries/oracle.go for use in transaction processing
type OracleEntry struct {
	Owner           [20]byte               // Account that owns this oracle
	Provider        []byte                 // Provider identifier (up to 256 bytes)
	AssetClass      []byte                 // Asset class identifier (up to 16 bytes)
	PriceDataSeries []OraclePriceDataEntry // Array of price data points
	LastUpdateTime  uint32                 // Ripple epoch timestamp of last update
	OwnerNode       uint64                 // Directory node hint
	URI             []byte                 // Optional URI (up to 256 bytes)
	PreviousTxnID   [32]byte               // Hash of previous modifying transaction
	PreviousTxnLgrSeq uint32               // Ledger sequence of previous modifying transaction
}

// OraclePriceDataEntry is the stored format for price data
type OraclePriceDataEntry struct {
	BaseAsset   [20]byte // Currency code as 20 bytes
	QuoteAsset  [20]byte // Currency code as 20 bytes
	AssetPrice  uint64   // Price value (0 if not updated)
	Scale       uint8    // Decimal scale
	HasPrice    bool     // Whether this entry has a current price
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
