package oracle

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

// OracleEntry represents a price oracle ledger entry
// This mirrors the structure in entry/entries/oracle.go for use in transaction processing
type OracleEntry struct {
	Owner             [20]byte               // Account that owns this oracle
	Provider          []byte                 // Provider identifier (up to 256 bytes)
	AssetClass        []byte                 // Asset class identifier (up to 16 bytes)
	PriceDataSeries   []OraclePriceDataEntry // Array of price data points
	LastUpdateTime    uint32                 // Ripple epoch timestamp of last update
	OwnerNode         uint64                 // Directory node hint
	URI               []byte                 // Optional URI (up to 256 bytes)
	PreviousTxnID     [32]byte               // Hash of previous modifying transaction
	PreviousTxnLgrSeq uint32                 // Ledger sequence of previous modifying transaction
}

// OraclePriceDataEntry is the stored format for price data
type OraclePriceDataEntry struct {
	BaseAsset  [20]byte // Currency code as 20 bytes
	QuoteAsset [20]byte // Currency code as 20 bytes
	AssetPrice uint64   // Price value (0 if not updated)
	Scale      uint8    // Decimal scale
	HasPrice   bool     // Whether this entry has a current price
}
