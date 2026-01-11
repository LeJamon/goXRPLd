package tx

import "errors"

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
	LastUpdateTime uint32 `json:"LastUpdateTime"`

	// PriceDataSeries is the price data (required)
	PriceDataSeries []PriceData `json:"PriceDataSeries"`
}

// PriceData represents a price data point
type PriceData struct {
	PriceData PriceDataEntry `json:"PriceData"`
}

// PriceDataEntry contains the price data fields
type PriceDataEntry struct {
	BaseAsset  string  `json:"BaseAsset"`
	QuoteAsset string  `json:"QuoteAsset"`
	AssetPrice *uint64 `json:"AssetPrice,omitempty"`
	Scale      *uint8  `json:"Scale,omitempty"`
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

// Validate validates the OracleSet transaction
func (o *OracleSet) Validate() error {
	if err := o.BaseTx.Validate(); err != nil {
		return err
	}

	if len(o.PriceDataSeries) == 0 {
		return errors.New("PriceDataSeries is required")
	}

	// Max 10 price data entries
	if len(o.PriceDataSeries) > 10 {
		return errors.New("cannot have more than 10 PriceDataSeries entries")
	}

	return nil
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

// AddPriceData adds a price data entry
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

// Validate validates the OracleDelete transaction
func (o *OracleDelete) Validate() error {
	return o.BaseTx.Validate()
}

// Flatten returns a flat map of all transaction fields
func (o *OracleDelete) Flatten() (map[string]any, error) {
	m := o.Common.ToMap()
	m["OracleDocumentID"] = o.OracleDocumentID
	return m, nil
}
