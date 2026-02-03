package oracle

import (
	"errors"
	"fmt"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
)

func init() {
	tx.Register(tx.TypeOracleSet, func() tx.Transaction {
		return &OracleSet{BaseTx: *tx.NewBaseTx(tx.TypeOracleSet, "")}
	})
}

// OracleSet creates or updates a price oracle.
type OracleSet struct {
	tx.BaseTx

	// OracleDocumentID identifies this oracle (required)
	OracleDocumentID uint32 `json:"OracleDocumentID" xrpl:"OracleDocumentID"`

	// Provider is the oracle provider name (required for creation)
	Provider string `json:"Provider,omitempty" xrpl:"Provider,omitempty"`

	// URI is the URI for the oracle data (optional)
	URI string `json:"URI,omitempty" xrpl:"URI,omitempty"`

	// AssetClass is the asset class for pricing (required for creation)
	AssetClass string `json:"AssetClass,omitempty" xrpl:"AssetClass,omitempty"`

	// LastUpdateTime is the timestamp of the last update (required)
	// This is in Ripple epoch (seconds since Jan 1, 2000)
	LastUpdateTime uint32 `json:"LastUpdateTime" xrpl:"LastUpdateTime"`

	// PriceDataSeries is the price data (required)
	PriceDataSeries []PriceData `json:"PriceDataSeries" xrpl:"PriceDataSeries"`

	// Presence tracking fields - used to detect explicitly empty fields
	// In rippled, an explicitly set empty field (sfURI present but empty) is invalid
	// These are set during JSON/binary deserialization
	ProviderPresent   bool `json:"-" xrpl:"-"` // true if Provider field was explicitly set
	URIPresent        bool `json:"-" xrpl:"-"` // true if URI field was explicitly set
	AssetClassPresent bool `json:"-" xrpl:"-"` // true if AssetClass field was explicitly set
}

// NewOracleSet creates a new OracleSet transaction
func NewOracleSet(account string, oracleDocID uint32, lastUpdateTime uint32) *OracleSet {
	return &OracleSet{
		BaseTx:           *tx.NewBaseTx(tx.TypeOracleSet, account),
		OracleDocumentID: oracleDocID,
		LastUpdateTime:   lastUpdateTime,
	}
}

// TxType returns the transaction type
func (o *OracleSet) TxType() tx.Type {
	return tx.TypeOracleSet
}

// Validate validates the OracleSet transaction (preflight validation)
// This matches rippled's SetOracle::preflight()
func (o *OracleSet) Validate() error {
	if err := o.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags - only universal flags allowed
	if o.Flags != nil && *o.Flags&tx.TfUniversalMask != 0 {
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

	// Validate Provider field
	// Reference: rippled Oracle_test.cpp lines 229-232 (too long) and 240-242 (empty)
	// If Provider is present (either via ProviderPresent flag or non-empty string), validate it
	// Note: Provider is stored as hex string, so byte length = len/2
	if o.ProviderPresent || o.Provider != "" {
		if len(o.Provider) == 0 {
			return errors.New("temMALFORMED: Provider cannot be empty")
		}
		// Provider is hex-encoded, so byte length = string length / 2
		byteLen := len(o.Provider) / 2
		if len(o.Provider)%2 != 0 {
			byteLen = (len(o.Provider) + 1) / 2 // Round up for odd-length strings
		}
		if byteLen > MaxOracleProvider {
			return fmt.Errorf("temMALFORMED: Provider length must be between 1 and %d bytes", MaxOracleProvider)
		}
	}

	// Validate URI field
	// Reference: rippled Oracle_test.cpp lines 233-235 (too long) and 243-245 (empty)
	// If URI is present (either via URIPresent flag or non-empty string), validate it
	// Note: URI is stored as hex string, so byte length = len/2
	if o.URIPresent || o.URI != "" {
		if len(o.URI) == 0 {
			return errors.New("temMALFORMED: URI cannot be empty")
		}
		// URI is hex-encoded, so byte length = string length / 2
		byteLen := len(o.URI) / 2
		if len(o.URI)%2 != 0 {
			byteLen = (len(o.URI) + 1) / 2 // Round up for odd-length strings
		}
		if byteLen > MaxOracleURI {
			return fmt.Errorf("temMALFORMED: URI length must be between 1 and %d bytes", MaxOracleURI)
		}
	}

	// Validate AssetClass field
	// Reference: rippled Oracle_test.cpp lines 223-228 (too long) and 237-239 (empty)
	// If AssetClass is present (either via AssetClassPresent flag or non-empty string), validate it
	// Note: AssetClass is stored as hex string, so byte length = len/2
	if o.AssetClassPresent || o.AssetClass != "" {
		if len(o.AssetClass) == 0 {
			return errors.New("temMALFORMED: AssetClass cannot be empty")
		}
		// AssetClass is hex-encoded, so byte length = string length / 2
		byteLen := len(o.AssetClass) / 2
		if len(o.AssetClass)%2 != 0 {
			byteLen = (len(o.AssetClass) + 1) / 2 // Round up for odd-length strings
		}
		if byteLen > MaxOracleSymbolClass {
			return fmt.Errorf("temMALFORMED: AssetClass length must be between 1 and %d bytes", MaxOracleSymbolClass)
		}
	}

	// Validate each price data entry and check for duplicates
	// Reference: rippled SetOracle.cpp checkArray()
	seenPairs := make(map[string]bool)
	for _, pd := range o.PriceDataSeries {
		entry := pd.PriceData

		// BaseAsset and QuoteAsset must be different
		if entry.BaseAsset == entry.QuoteAsset {
			return errors.New("temMALFORMED: BaseAsset and QuoteAsset must be different")
		}

		// Scale cannot exceed MaxPriceScale
		if entry.Scale != nil && *entry.Scale > MaxPriceScale {
			return fmt.Errorf("temMALFORMED: Scale cannot exceed %d", MaxPriceScale)
		}

		// Check for duplicate token pairs
		pairKey := entry.BaseAsset + ":" + entry.QuoteAsset
		if seenPairs[pairKey] {
			return errors.New("temMALFORMED: duplicate token pair in PriceDataSeries")
		}
		seenPairs[pairKey] = true
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
	return tx.ReflectFlatten(o)
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

// RequiredAmendments returns the amendments required for this transaction type
func (o *OracleSet) RequiredAmendments() []string {
	return []string{amendment.AmendmentPriceOracle}
}

// Apply applies an OracleSet transaction to the ledger state.
// Reference: rippled SetOracle.cpp SetOracle::doApply
func (o *OracleSet) Apply(ctx *tx.ApplyContext) tx.Result {
	oracleKey := keylet.Escrow(ctx.AccountID, o.OracleDocumentID)
	exists, _ := ctx.View.Exists(oracleKey)
	if !exists {
		ctx.Account.OwnerCount++
	}
	return tx.TesSUCCESS
}
