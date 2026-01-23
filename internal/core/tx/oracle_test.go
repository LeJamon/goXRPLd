package tx

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create a valid hex string of specified byte length
func makeHex(byteLen int) string {
	return strings.Repeat("AB", byteLen)
}

// Helper to create valid PriceData with price
func validPriceData(base, quote string, price uint64) PriceData {
	p := price
	return PriceData{
		PriceData: PriceDataEntry{
			BaseAsset:  base,
			QuoteAsset: quote,
			AssetPrice: &p,
		},
	}
}

// Helper to create PriceData for deletion (no price)
func deletePriceData(base, quote string) PriceData {
	return PriceData{
		PriceData: PriceDataEntry{
			BaseAsset:  base,
			QuoteAsset: quote,
			AssetPrice: nil, // nil means delete
		},
	}
}

// =============================================================================
// OracleSet Validation Tests - Based on rippled Oracle_test.cpp testInvalidSet()
// =============================================================================

func TestOracleSetValidation(t *testing.T) {
	tests := []struct {
		name    string
		tx      *OracleSet
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{
			name: "valid - create with single price pair",
			tx: &OracleSet{
				BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
				OracleDocumentID: 1,
				Provider:         makeHex(32),
				AssetClass:       makeHex(8),
				LastUpdateTime:   750000000,
				PriceDataSeries:  []PriceData{validPriceData("XRP", "USD", 740)},
			},
			wantErr: false,
		},
		{
			name: "valid - create with URI",
			tx: &OracleSet{
				BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
				OracleDocumentID: 1,
				Provider:         makeHex(32),
				URI:              makeHex(64),
				AssetClass:       makeHex(8),
				LastUpdateTime:   750000000,
				PriceDataSeries:  []PriceData{validPriceData("XRP", "USD", 740)},
			},
			wantErr: false,
		},
		{
			name: "valid - maximum price data entries (10)",
			tx: func() *OracleSet {
				currencies := []string{"USD", "EUR", "GBP", "JPY", "CHF", "CAD", "AUD", "NZD", "CNY", "KRW"}
				entries := make([]PriceData, 10)
				for i, curr := range currencies {
					entries[i] = validPriceData("XRP", curr, uint64(740+i))
				}
				return &OracleSet{
					BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
					OracleDocumentID: 1,
					Provider:         makeHex(32),
					AssetClass:       makeHex(8),
					LastUpdateTime:   750000000,
					PriceDataSeries:  entries,
				}
			}(),
			wantErr: false,
		},
		{
			name: "valid - maximum scale (20)",
			tx: func() *OracleSet {
				scale := uint8(20)
				price := uint64(740)
				return &OracleSet{
					BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
					OracleDocumentID: 1,
					Provider:         makeHex(32),
					AssetClass:       makeHex(8),
					LastUpdateTime:   750000000,
					PriceDataSeries: []PriceData{{
						PriceData: PriceDataEntry{
							BaseAsset:  "XRP",
							QuoteAsset: "USD",
							AssetPrice: &price,
							Scale:      &scale,
						},
					}},
				}
			}(),
			wantErr: false,
		},

		// Invalid cases - Array validation
		{
			name: "invalid - empty PriceDataSeries (temARRAY_EMPTY)",
			tx: &OracleSet{
				BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
				OracleDocumentID: 1,
				Provider:         makeHex(32),
				AssetClass:       makeHex(8),
				LastUpdateTime:   750000000,
				PriceDataSeries:  []PriceData{},
			},
			wantErr: true,
			errMsg:  "temARRAY_EMPTY",
		},
		{
			name: "invalid - too many price data entries >10 (temARRAY_TOO_LARGE)",
			tx: func() *OracleSet {
				entries := make([]PriceData, 11)
				for i := 0; i < 11; i++ {
					entries[i] = validPriceData("XRP", "US"+string(rune('A'+i)), uint64(740+i))
				}
				return &OracleSet{
					BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
					OracleDocumentID: 1,
					Provider:         makeHex(32),
					AssetClass:       makeHex(8),
					LastUpdateTime:   750000000,
					PriceDataSeries:  entries,
				}
			}(),
			wantErr: true,
			errMsg:  "temARRAY_TOO_LARGE",
		},

		// Invalid cases - Field length validation
		{
			name: "invalid - Provider too long >256 bytes (temMALFORMED)",
			tx: &OracleSet{
				BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
				OracleDocumentID: 1,
				Provider:         makeHex(257),
				AssetClass:       makeHex(8),
				LastUpdateTime:   750000000,
				PriceDataSeries:  []PriceData{validPriceData("XRP", "USD", 740)},
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},
		{
			name: "invalid - URI too long >256 bytes (temMALFORMED)",
			tx: &OracleSet{
				BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
				OracleDocumentID: 1,
				Provider:         makeHex(32),
				URI:              makeHex(257),
				AssetClass:       makeHex(8),
				LastUpdateTime:   750000000,
				PriceDataSeries:  []PriceData{validPriceData("XRP", "USD", 740)},
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},
		{
			name: "invalid - AssetClass too long >16 bytes (temMALFORMED)",
			tx: &OracleSet{
				BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
				OracleDocumentID: 1,
				Provider:         makeHex(32),
				AssetClass:       makeHex(17),
				LastUpdateTime:   750000000,
				PriceDataSeries:  []PriceData{validPriceData("XRP", "USD", 740)},
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},

		// Invalid cases - Token pair validation
		{
			name: "invalid - same BaseAsset and QuoteAsset (temMALFORMED)",
			tx: &OracleSet{
				BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
				OracleDocumentID: 1,
				Provider:         makeHex(32),
				AssetClass:       makeHex(8),
				LastUpdateTime:   750000000,
				PriceDataSeries:  []PriceData{validPriceData("USD", "USD", 740)},
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},
		{
			name: "invalid - duplicate token pair (temMALFORMED)",
			tx: &OracleSet{
				BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
				OracleDocumentID: 1,
				Provider:         makeHex(32),
				AssetClass:       makeHex(8),
				LastUpdateTime:   750000000,
				PriceDataSeries: []PriceData{
					validPriceData("XRP", "USD", 740),
					validPriceData("XRP", "USD", 750), // duplicate
				},
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},
		{
			name: "invalid - token pair in both update and delete (temMALFORMED)",
			tx: &OracleSet{
				BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
				OracleDocumentID: 1,
				Provider:         makeHex(32),
				AssetClass:       makeHex(8),
				LastUpdateTime:   750000000,
				PriceDataSeries: []PriceData{
					validPriceData("XRP", "USD", 740),
					deletePriceData("XRP", "USD"), // same pair for deletion
				},
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},

		// Invalid cases - Scale validation
		{
			name: "invalid - Scale too large >20 (temMALFORMED)",
			tx: func() *OracleSet {
				scale := uint8(21)
				price := uint64(740)
				return &OracleSet{
					BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
					OracleDocumentID: 1,
					Provider:         makeHex(32),
					AssetClass:       makeHex(8),
					LastUpdateTime:   750000000,
					PriceDataSeries: []PriceData{{
						PriceData: PriceDataEntry{
							BaseAsset:  "XRP",
							QuoteAsset: "USD",
							AssetPrice: &price,
							Scale:      &scale,
						},
					}},
				}
			}(),
			wantErr: true,
			errMsg:  "temMALFORMED",
		},

		// Invalid cases - Common field validation
		{
			name: "invalid - missing account",
			tx: &OracleSet{
				BaseTx:           BaseTx{},
				OracleDocumentID: 1,
				Provider:         makeHex(32),
				AssetClass:       makeHex(8),
				LastUpdateTime:   750000000,
				PriceDataSeries:  []PriceData{validPriceData("XRP", "USD", 740)},
			},
			wantErr: true,
			errMsg:  "Account is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.tx.Common.Fee = "12"
			seq := uint32(1)
			tt.tx.Common.Sequence = &seq

			err := tt.tx.Validate()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// OracleDelete Validation Tests - Based on rippled Oracle_test.cpp testInvalidDelete()
// =============================================================================

func TestOracleDeleteValidation(t *testing.T) {
	tests := []struct {
		name    string
		tx      *OracleDelete
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid OracleDelete",
			tx: &OracleDelete{
				BaseTx:           *NewBaseTx(TypeOracleDelete, "rTestAccount"),
				OracleDocumentID: 1,
			},
			wantErr: false,
		},
		{
			name: "valid OracleDelete - high document ID",
			tx: &OracleDelete{
				BaseTx:           *NewBaseTx(TypeOracleDelete, "rTestAccount"),
				OracleDocumentID: 4294967295, // Max uint32
			},
			wantErr: false,
		},
		{
			name: "invalid - missing account",
			tx: &OracleDelete{
				BaseTx:           BaseTx{},
				OracleDocumentID: 1,
			},
			wantErr: true,
			errMsg:  "Account is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.tx.Common.Fee = "12"
			seq := uint32(1)
			tt.tx.Common.Sequence = &seq

			err := tt.tx.Validate()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// Flatten Tests
// =============================================================================

func TestOracleSetFlatten(t *testing.T) {
	price := uint64(1234567890)
	scale := uint8(6)

	tx := &OracleSet{
		BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
		OracleDocumentID: 42,
		Provider:         "4D79507269636550726F7669646572",
		URI:              "68747470733A2F2F6578616D706C652E636F6D",
		AssetClass:       "63757272656E6379",
		LastUpdateTime:   750000000,
		PriceDataSeries: []PriceData{{
			PriceData: PriceDataEntry{
				BaseAsset:  "USD",
				QuoteAsset: "XRP",
				AssetPrice: &price,
				Scale:      &scale,
			},
		}},
	}

	flat, err := tx.Flatten()
	require.NoError(t, err)

	assert.Equal(t, "rTestAccount", flat["Account"])
	assert.Equal(t, "OracleSet", flat["TransactionType"])
	assert.Equal(t, uint32(42), flat["OracleDocumentID"])
	assert.Equal(t, "4D79507269636550726F7669646572", flat["Provider"])
	assert.Equal(t, "68747470733A2F2F6578616D706C652E636F6D", flat["URI"])
	assert.Equal(t, "63757272656E6379", flat["AssetClass"])
	assert.Equal(t, uint32(750000000), flat["LastUpdateTime"])
}

func TestOracleDeleteFlatten(t *testing.T) {
	tx := &OracleDelete{
		BaseTx:           *NewBaseTx(TypeOracleDelete, "rTestAccount"),
		OracleDocumentID: 99,
	}

	flat, err := tx.Flatten()
	require.NoError(t, err)

	assert.Equal(t, "rTestAccount", flat["Account"])
	assert.Equal(t, "OracleDelete", flat["TransactionType"])
	assert.Equal(t, uint32(99), flat["OracleDocumentID"])
}

// =============================================================================
// Transaction Type Tests
// =============================================================================

func TestOracleTransactionTypes(t *testing.T) {
	tests := []struct {
		name     string
		tx       Transaction
		expected Type
	}{
		{
			name:     "OracleSet type",
			tx:       &OracleSet{BaseTx: *NewBaseTx(TypeOracleSet, "rTest")},
			expected: TypeOracleSet,
		},
		{
			name:     "OracleDelete type",
			tx:       &OracleDelete{BaseTx: *NewBaseTx(TypeOracleDelete, "rTest")},
			expected: TypeOracleDelete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.tx.TxType())
		})
	}
}

// =============================================================================
// Constructor Tests
// =============================================================================

func TestOracleConstructors(t *testing.T) {
	t.Run("NewOracleSet", func(t *testing.T) {
		tx := NewOracleSet("rAccount123", 42, 750000000)
		require.NotNil(t, tx)
		assert.Equal(t, "rAccount123", tx.Account)
		assert.Equal(t, uint32(42), tx.OracleDocumentID)
		assert.Equal(t, uint32(750000000), tx.LastUpdateTime)
		assert.Equal(t, TypeOracleSet, tx.TxType())
	})

	t.Run("NewOracleDelete", func(t *testing.T) {
		tx := NewOracleDelete("rAccount456", 99)
		require.NotNil(t, tx)
		assert.Equal(t, "rAccount456", tx.Account)
		assert.Equal(t, uint32(99), tx.OracleDocumentID)
		assert.Equal(t, TypeOracleDelete, tx.TxType())
	})
}

// =============================================================================
// Amendment Tests
// =============================================================================

func TestOracleRequiredAmendments(t *testing.T) {
	tests := []struct {
		name     string
		tx       Transaction
		expected []string
	}{
		{
			name:     "OracleSet requires PriceOracle amendment",
			tx:       &OracleSet{BaseTx: *NewBaseTx(TypeOracleSet, "rTest")},
			expected: []string{AmendmentPriceOracle},
		},
		{
			name:     "OracleDelete requires PriceOracle amendment",
			tx:       &OracleDelete{BaseTx: *NewBaseTx(TypeOracleDelete, "rTest")},
			expected: []string{AmendmentPriceOracle},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.tx.RequiredAmendments()
			assert.Equal(t, tt.expected, got)
		})
	}
}

// =============================================================================
// Price Data Deletion Tests
// =============================================================================

func TestOraclePriceDataDeletion(t *testing.T) {
	// When updating an oracle, you can delete a price pair by setting AssetPrice to nil
	tx := &OracleSet{
		BaseTx:           *NewBaseTx(TypeOracleSet, "rTestAccount"),
		OracleDocumentID: 1,
		LastUpdateTime:   750000000,
		PriceDataSeries: []PriceData{
			deletePriceData("USD", "XRP"), // Delete this pair
		},
	}

	tx.Common.Fee = "12"
	seq := uint32(1)
	tx.Common.Sequence = &seq

	// Deletion entries are allowed in updates (validation passes)
	err := tx.Validate()
	assert.NoError(t, err)
}

// =============================================================================
// Constants Tests
// =============================================================================

func TestOracleConstants(t *testing.T) {
	assert.Equal(t, 256, MaxOracleURI)
	assert.Equal(t, 256, MaxOracleProvider)
	assert.Equal(t, 10, MaxOracleDataSeries)
	assert.Equal(t, 16, MaxOracleSymbolClass)
	assert.Equal(t, 300, MaxLastUpdateTimeDelta)
	assert.Equal(t, 20, MaxPriceScale)
}

// =============================================================================
// Owner Count Reserve Tests (based on rippled testCreate/testDelete)
// =============================================================================

func TestOracleOwnerCountReserve(t *testing.T) {
	// Owner count is 1 for ≤5 pairs, 2 for >5 pairs
	t.Run("reserve count for 1 pair", func(t *testing.T) {
		count := CalculateOwnerCountAdjustment(1)
		assert.Equal(t, 1, count)
	})

	t.Run("reserve count for 5 pairs", func(t *testing.T) {
		count := CalculateOwnerCountAdjustment(5)
		assert.Equal(t, 1, count)
	})

	t.Run("reserve count for 6 pairs", func(t *testing.T) {
		count := CalculateOwnerCountAdjustment(6)
		assert.Equal(t, 2, count)
	})

	t.Run("reserve count for 10 pairs", func(t *testing.T) {
		count := CalculateOwnerCountAdjustment(10)
		assert.Equal(t, 2, count)
	})
}
