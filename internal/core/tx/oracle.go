package tx

import (
	"encoding/hex"
	"errors"
)

// OracleSet creates or updates a price oracle.
// Reference: rippled SetOracle.cpp
type OracleSet struct {
	BaseTx

	// OracleDocumentID identifies this oracle (required)
	OracleDocumentID uint32 `json:"OracleDocumentID"`

	// Provider is the oracle provider name (required for creation, hex-encoded)
	Provider string `json:"Provider,omitempty"`

	// URI is the URI for the oracle data (optional, hex-encoded)
	URI string `json:"URI,omitempty"`

	// AssetClass is the asset class for pricing (required for creation, hex-encoded)
	AssetClass string `json:"AssetClass,omitempty"`

	// LastUpdateTime is the timestamp of the last update (required, Ripple epoch)
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
	BaseAsset  string  `json:"BaseAsset"`            // Currency code
	QuoteAsset string  `json:"QuoteAsset"`           // Currency code
	AssetPrice *uint64 `json:"AssetPrice,omitempty"` // Price (optional - absence means delete)
	Scale      *uint8  `json:"Scale,omitempty"`      // Decimal scale (optional)
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

// Validate validates the OracleSet transaction.
// Implements preflight checks from rippled SetOracle.cpp SetOracle::preflight()
// Reference: rippled SetOracle.cpp lines 40-67
func (o *OracleSet) Validate() error {
	if err := o.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	// Reference: rippled SetOracle.cpp:48-49
	if o.Common.Flags != nil && *o.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// Check PriceDataSeries is not empty
	// Reference: rippled SetOracle.cpp:51-53
	if len(o.PriceDataSeries) == 0 {
		return ErrOracleEmptyPriceData
	}

	// Check PriceDataSeries does not exceed maximum
	// Reference: rippled SetOracle.cpp:54-55
	if len(o.PriceDataSeries) > MaxOracleDataSeries {
		return ErrOracleTooManyPriceData
	}

	// Validate Provider field length
	// Reference: rippled SetOracle.cpp:57-65
	if o.Provider != "" {
		decoded, err := hex.DecodeString(o.Provider)
		if err != nil {
			return errors.New("temMALFORMED: Provider must be valid hex string")
		}
		if len(decoded) == 0 {
			return ErrOracleProviderEmpty
		}
		if len(decoded) > MaxOracleProvider {
			return ErrOracleProviderTooLong
		}
	}

	// Validate URI field length
	// Reference: rippled SetOracle.cpp:57-65
	if o.URI != "" {
		decoded, err := hex.DecodeString(o.URI)
		if err != nil {
			return errors.New("temMALFORMED: URI must be valid hex string")
		}
		if len(decoded) == 0 {
			return ErrOracleURIEmpty
		}
		if len(decoded) > MaxOracleURI {
			return ErrOracleURITooLong
		}
	}

	// Validate AssetClass field length
	// Reference: rippled SetOracle.cpp:57-65
	if o.AssetClass != "" {
		decoded, err := hex.DecodeString(o.AssetClass)
		if err != nil {
			return errors.New("temMALFORMED: AssetClass must be valid hex string")
		}
		if len(decoded) == 0 {
			return ErrOracleAssetClassEmpty
		}
		if len(decoded) > MaxOracleSymbolClass {
			return ErrOracleAssetClassTooLong
		}
	}

	// Validate each price data entry and check for duplicates
	// Reference: rippled SetOracle.cpp checkArray()
	seenPairs := make(map[string]bool)
	for _, pd := range o.PriceDataSeries {
		entry := pd.PriceData

		// Check BaseAsset and QuoteAsset are different
		// Reference: rippled SetOracle.cpp:103-104
		if entry.BaseAsset == entry.QuoteAsset {
			return ErrOracleSameAssets
		}

		// Check Scale is not too large
		// Reference: rippled SetOracle.cpp:105-106
		if entry.Scale != nil && *entry.Scale > MaxPriceScale {
			return ErrOracleScaleTooLarge
		}

		// Check for duplicate token pairs
		// Reference: rippled SetOracle.cpp:107-113
		pairKey := entry.BaseAsset + ":" + entry.QuoteAsset
		if seenPairs[pairKey] {
			return ErrOracleDuplicatePair
		}
		seenPairs[pairKey] = true
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (o *OracleSet) Flatten() (map[string]any, error) {
	m := o.Common.ToMap()

	m["OracleDocumentID"] = o.OracleDocumentID
	m["LastUpdateTime"] = o.LastUpdateTime

	// Convert PriceDataSeries to the format expected by binary codec
	priceDataArray := make([]map[string]any, 0, len(o.PriceDataSeries))
	for _, pd := range o.PriceDataSeries {
		innerData := map[string]any{
			"BaseAsset":  pd.PriceData.BaseAsset,
			"QuoteAsset": pd.PriceData.QuoteAsset,
		}
		if pd.PriceData.AssetPrice != nil {
			innerData["AssetPrice"] = formatUint64Hex(*pd.PriceData.AssetPrice)
		}
		if pd.PriceData.Scale != nil {
			innerData["Scale"] = *pd.PriceData.Scale
		}
		priceDataArray = append(priceDataArray, map[string]any{
			"PriceData": innerData,
		})
	}
	m["PriceDataSeries"] = priceDataArray

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

// AddPriceData adds a price data entry with an asset price
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

// AddPriceDataDelete adds a price data entry that signals deletion of a token pair
// (no AssetPrice means delete the pair from the oracle)
func (o *OracleSet) AddPriceDataDelete(baseAsset, quoteAsset string) {
	o.PriceDataSeries = append(o.PriceDataSeries, PriceData{
		PriceData: PriceDataEntry{
			BaseAsset:  baseAsset,
			QuoteAsset: quoteAsset,
			// No AssetPrice = delete
		},
	})
}

// RequiredAmendments returns the amendments required for this transaction type
func (o *OracleSet) RequiredAmendments() []string {
	return []string{AmendmentPriceOracle}
}

// OracleDelete deletes a price oracle.
// Reference: rippled DeleteOracle.cpp
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

// Validate validates the OracleDelete transaction.
// Implements preflight checks from rippled DeleteOracle.cpp DeleteOracle::preflight()
// Reference: rippled DeleteOracle.cpp lines 29-45
func (o *OracleDelete) Validate() error {
	if err := o.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	// Reference: rippled DeleteOracle.cpp:38-42
	if o.Common.Flags != nil && *o.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (o *OracleDelete) Flatten() (map[string]any, error) {
	m := o.Common.ToMap()
	m["OracleDocumentID"] = o.OracleDocumentID
	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (o *OracleDelete) RequiredAmendments() []string {
	return []string{AmendmentPriceOracle}
}
