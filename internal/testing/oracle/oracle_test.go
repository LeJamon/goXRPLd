package oracle_test

import (
	"testing"

	"github.com/LeJamon/goXRPLd/internal/core/tx/oracle"
	jtx "github.com/LeJamon/goXRPLd/internal/testing"
	oracletest "github.com/LeJamon/goXRPLd/internal/testing/oracle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Oracle Tests - Based on rippled Oracle_test.cpp
// =============================================================================
//
// Reference: rippled/src/test/app/Oracle_test.cpp
//
// Test suites:
// - testInvalidSet(): Invalid OracleSet transactions (lines 29-398)
// - testInvalidDelete(): Invalid OracleDelete transactions (lines 457-500)
// - testCreate(): Successful oracle creation (lines 400-455)
// - testDelete(): Successful oracle deletion (lines 502-591)
// - testUpdate(): Successful oracle updates (lines 594-735)
// - testAmendment(): Amendment feature toggle (lines 835-857)
// =============================================================================

// =============================================================================
// testInvalidSet() - Based on rippled Oracle_test.cpp lines 29-398
// =============================================================================

func TestInvalidSet(t *testing.T) {
	alice := jtx.NewAccount("alice")

	t.Run("Invalid flag (temINVALID_FLAG)", func(t *testing.T) {
		// rippled line 96-99
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			Flags(0x00000001). // tfSellNFToken equivalent
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temINVALID_FLAG")
	})

	t.Run("Duplicate token pair (temMALFORMED)", func(t *testing.T) {
		// rippled lines 101-105
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			AddPrice("XRP", "USD", 750, 1). // duplicate
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("Price not included on create (temMALFORMED)", func(t *testing.T) {
		// rippled lines 107-112: Price is not included
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			AddDelete("XRP", "EUR"). // No price = delete request, invalid on create
			Sequence(1).
			BuildOracleSet()

		// This should pass preflight validation but fail preclaim
		// ValidatePriceDataSeries catches it
		_, _, err := oset.ValidatePriceDataSeries(false) // false = create
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("Token pair in update and delete - same pair twice (temMALFORMED)", func(t *testing.T) {
		// rippled lines 114-119
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			AddDelete("XRP", "USD"). // same pair for deletion
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("Token pair in add and delete (temMALFORMED)", func(t *testing.T) {
		// rippled lines 120-125
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "EUR", 740, 1).
			AddDelete("XRP", "EUR"). // same pair for deletion
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("Array exceeds 10 (temARRAY_TOO_LARGE)", func(t *testing.T) {
		// rippled lines 127-142
		builder := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			Sequence(1)

		// Add 11 entries
		currencies := []string{"US1", "US2", "US3", "US4", "US5", "US6", "US7", "US8", "US9", "U10", "U11"}
		for _, curr := range currencies {
			builder = builder.AddPrice("XRP", curr, 740, 1)
		}
		oset := builder.BuildOracleSet()

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temARRAY_TOO_LARGE")
	})

	t.Run("Empty array (temARRAY_EMPTY)", func(t *testing.T) {
		// rippled lines 143-144
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			Sequence(1).
			BuildOracleSet()
		// No price data added

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temARRAY_EMPTY")
	})

	t.Run("AssetClass too long >16 bytes (temMALFORMED)", func(t *testing.T) {
		// rippled lines 223-228
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(17). // 17 bytes > 16
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("Provider too long >256 bytes (temMALFORMED)", func(t *testing.T) {
		// rippled lines 229-232
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(257). // 257 bytes > 256
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("URI too long >256 bytes (temMALFORMED)", func(t *testing.T) {
		// rippled lines 233-235
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			URIHex(257). // 257 bytes > 256
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("Empty AssetClass explicitly present (temMALFORMED)", func(t *testing.T) {
		// rippled lines 237-239
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClass(""). // empty but explicitly set
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("Empty Provider explicitly present (temMALFORMED)", func(t *testing.T) {
		// rippled lines 240-242
		oset := oracletest.OracleSet(alice, 1, 750000000).
			Provider(""). // empty but explicitly set
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("Empty URI explicitly present (temMALFORMED)", func(t *testing.T) {
		// rippled lines 243-245
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			URI(""). // empty but explicitly set
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("Same BaseAsset and QuoteAsset (temMALFORMED)", func(t *testing.T) {
		// rippled lines 331-343
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("USD", "USD", 740, 1). // same asset
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("Scale greater than maxPriceScale (temMALFORMED)", func(t *testing.T) {
		// rippled lines 345-357: Scale is greater than maxPriceScale
		// maxPriceScale = 8, so scale 9 should fail
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("USD", "BTC", 740, 9). // scale 9 > max 8
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("Valid maximum scale (success)", func(t *testing.T) {
		// Maximum valid scale should succeed
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("USD", "BTC", 740, 8). // scale 8 is max valid
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)
	})
}

// =============================================================================
// testInvalidDelete() - Based on rippled Oracle_test.cpp lines 457-500
// =============================================================================

func TestInvalidDelete(t *testing.T) {
	alice := jtx.NewAccount("alice")

	t.Run("Invalid flags (temINVALID_FLAG)", func(t *testing.T) {
		// rippled lines 493-496
		odel := oracletest.OracleDelete(alice, 1).
			Flags(0x00000001). // tfSellNFToken equivalent
			Sequence(1).
			BuildOracleDelete()

		err := odel.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temINVALID_FLAG")
	})

	t.Run("Valid delete (success)", func(t *testing.T) {
		odel := oracletest.OracleDelete(alice, 1).
			Sequence(1).
			BuildOracleDelete()

		err := odel.Validate()
		assert.NoError(t, err)
	})
}

// =============================================================================
// testCreate() - Based on rippled Oracle_test.cpp lines 400-455
// =============================================================================

func TestCreate(t *testing.T) {
	alice := jtx.NewAccount("alice")
	bob := jtx.NewAccount("bob")

	t.Run("Create with single price pair (owner count +1)", func(t *testing.T) {
		// rippled lines 419-422
		oset := oracletest.OracleSet(alice, 1, 946694810).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)

		// Verify owner count adjustment is 1
		assert.Equal(t, 1, oracle.CalculateOwnerCountAdjustment(len(oset.PriceDataSeries)))
	})

	t.Run("Create with 6 price pairs (owner count +2)", func(t *testing.T) {
		// rippled lines 424-437
		oset := oracletest.OracleSet(alice, 1, 946694810).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			AddPrice("BTC", "USD", 740, 1).
			AddPrice("ETH", "USD", 740, 1).
			AddPrice("CAN", "USD", 740, 1).
			AddPrice("YAN", "USD", 740, 1).
			AddPrice("GBP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)

		// Verify owner count adjustment is 2 for >5 pairs
		assert.Equal(t, 2, oracle.CalculateOwnerCountAdjustment(len(oset.PriceDataSeries)))
	})

	t.Run("Create with maximum 10 price pairs", func(t *testing.T) {
		oset := oracletest.OracleSet(alice, 1, 946694810).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			AddPrice("BTC", "USD", 740, 1).
			AddPrice("ETH", "USD", 740, 1).
			AddPrice("CAN", "USD", 740, 1).
			AddPrice("YAN", "USD", 740, 1).
			AddPrice("GBP", "USD", 740, 1).
			AddPrice("EUR", "USD", 740, 1).
			AddPrice("JPY", "USD", 740, 1).
			AddPrice("CHF", "USD", 740, 1).
			AddPrice("AUD", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)

		// Verify owner count adjustment is 2 for 10 pairs
		assert.Equal(t, 2, oracle.CalculateOwnerCountAdjustment(len(oset.PriceDataSeries)))
	})

	t.Run("Different owner can create oracle with same document ID", func(t *testing.T) {
		// rippled lines 439-454
		// Oracle keys include the owner account, so different owners can have
		// oracles with the same document ID
		oset1 := oracletest.OracleSet(alice, 1, 946694810).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset1.Validate()
		assert.NoError(t, err)

		oset2 := oracletest.OracleSet(bob, 1, 946694810). // Same document ID, different owner
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("912810RR9", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err = oset2.Validate()
		assert.NoError(t, err)
	})
}

// =============================================================================
// testDelete() - Based on rippled Oracle_test.cpp lines 502-591
// =============================================================================

func TestDelete(t *testing.T) {
	t.Run("Delete oracle with 1 pair (owner count -1)", func(t *testing.T) {
		// rippled lines 522-525
		priceCount := 1
		assert.Equal(t, 1, oracle.CalculateOwnerCountAdjustment(priceCount))
	})

	t.Run("Delete oracle with 6 pairs (owner count -2)", func(t *testing.T) {
		// rippled lines 527-542
		priceCount := 6
		assert.Equal(t, 2, oracle.CalculateOwnerCountAdjustment(priceCount))
	})
}

// =============================================================================
// testUpdate() - Based on rippled Oracle_test.cpp lines 594-735
// =============================================================================

func TestUpdate(t *testing.T) {
	alice := jtx.NewAccount("alice")

	t.Run("Update: change existing pair", func(t *testing.T) {
		// rippled lines 611-613
		oset := oracletest.OracleSet(alice, 1, 750000001). // Must be > previous update time
			AddPrice("XRP", "USD", 740, 2).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)
	})

	t.Run("Update: add new pair", func(t *testing.T) {
		// rippled lines 620-621
		oset := oracletest.OracleSet(alice, 1, 750000001).
			AddPrice("XRP", "EUR", 700, 2).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)
	})

	t.Run("Update: delete a pair (via ValidatePriceDataSeries)", func(t *testing.T) {
		// rippled lines 650-651
		oset := oracletest.OracleSet(alice, 1, 750000001).
			AddDelete("BTC", "USD").
			Sequence(1).
			BuildOracleSet()

		// Validate passes at preflight level
		err := oset.Validate()
		assert.NoError(t, err)

		// ValidatePriceDataSeries should allow deletes on updates
		toAdd, toDelete, err := oset.ValidatePriceDataSeries(true) // true = update
		assert.NoError(t, err)
		assert.Empty(t, toAdd)
		assert.Len(t, toDelete, 1)
		assert.Contains(t, toDelete, "BTC/USD")
	})

	t.Run("Update: add and delete in same transaction", func(t *testing.T) {
		// rippled lines 653-660
		oset := oracletest.OracleSet(alice, 1, 750000001).
			AddPrice("XRP", "USD", 742, 2).
			AddPrice("XRP", "EUR", 711, 2).
			AddDelete("ETH", "EUR").
			AddDelete("YAN", "EUR").
			AddDelete("CAN", "EUR").
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)

		toAdd, toDelete, err := oset.ValidatePriceDataSeries(true)
		assert.NoError(t, err)
		assert.Len(t, toAdd, 2)
		assert.Len(t, toDelete, 3)
	})

	t.Run("Update: owner count boundary at 5 pairs", func(t *testing.T) {
		// When going from ≤5 to >5 pairs, owner count should increase by 1
		// When going from >5 to ≤5 pairs, owner count should decrease by 1
		assert.Equal(t, 1, oracle.CalculateOwnerCountAdjustment(5))
		assert.Equal(t, 2, oracle.CalculateOwnerCountAdjustment(6))
	})

	t.Run("Update without Provider/AssetClass is valid", func(t *testing.T) {
		// On updates, Provider and AssetClass are optional
		// (they must match existing values if provided)
		oset := oracletest.OracleSet(alice, 1, 750000001).
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()
		// No Provider, no AssetClass - valid for update

		err := oset.Validate()
		assert.NoError(t, err)
	})

	t.Run("Update with Provider must match existing (preclaim check)", func(t *testing.T) {
		// rippled lines 203-207
		// This check happens in preclaim, not preflight
		// Provider included on update must match existing value
		oset := oracletest.OracleSet(alice, 1, 750000001).
			ProviderHex(32). // Provider included - must match existing
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		// Preflight passes
		err := oset.Validate()
		assert.NoError(t, err)
		// Actual match check would be done in preclaim against ledger state
	})

	t.Run("Update with AssetClass must match existing (preclaim check)", func(t *testing.T) {
		// rippled lines 208-212
		oset := oracletest.OracleSet(alice, 1, 750000001).
			AssetClassHex(8). // AssetClass included - must match existing
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		// Preflight passes
		err := oset.Validate()
		assert.NoError(t, err)
		// Actual match check would be done in preclaim against ledger state
	})
}

// =============================================================================
// testAmendment() - Based on rippled Oracle_test.cpp lines 835-857
// =============================================================================

func TestAmendment(t *testing.T) {
	alice := jtx.NewAccount("alice")

	t.Run("OracleSet requires PriceOracle amendment", func(t *testing.T) {
		oset := oracletest.OracleSet(alice, 1, 750000000).
			BuildOracleSet()

		amendments := oset.RequiredAmendments()
		require.Len(t, amendments, 1)
		assert.Contains(t, amendments[0], "PriceOracle")
	})

	t.Run("OracleDelete requires PriceOracle amendment", func(t *testing.T) {
		odel := oracletest.OracleDelete(alice, 1).
			BuildOracleDelete()

		amendments := odel.RequiredAmendments()
		require.Len(t, amendments, 1)
		assert.Contains(t, amendments[0], "PriceOracle")
	})
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func TestEdgeCases(t *testing.T) {
	alice := jtx.NewAccount("alice")

	t.Run("Maximum field lengths (valid)", func(t *testing.T) {
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(256). // Max 256 bytes
			URIHex(256).      // Max 256 bytes
			AssetClassHex(16). // Max 16 bytes
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)
	})

	t.Run("Minimum field lengths (1 byte)", func(t *testing.T) {
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(1). // 1 byte
			AssetClassHex(1). // 1 byte
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)
	})

	t.Run("Non-standard 40-char hex currency codes", func(t *testing.T) {
		// XRPL supports non-standard currency codes as 40-character hex strings
		hexCurrency := "0000000000000000000000005553442B2B000000" // USD++ equivalent
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice(hexCurrency, "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)
	})

	t.Run("Scale value 0 is valid", func(t *testing.T) {
		oset := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 0). // scale 0
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)
	})

	t.Run("OracleDocumentID can be 0", func(t *testing.T) {
		oset := oracletest.OracleSet(alice, 0, 750000000). // Zero is valid
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)
	})

	t.Run("OracleDocumentID can be max uint32", func(t *testing.T) {
		oset := oracletest.OracleSet(alice, 4294967295, 750000000). // Max uint32
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)
	})
}

// =============================================================================
// Owner Count Tests - Boundary conditions
// =============================================================================

func TestOwnerCountBoundary(t *testing.T) {
	// Test the exact boundary at 5 pairs
	tests := []struct {
		pairCount   int
		expectedAdj int
		description string
	}{
		{1, 1, "1 pair = 1 reserve unit"},
		{2, 1, "2 pairs = 1 reserve unit"},
		{3, 1, "3 pairs = 1 reserve unit"},
		{4, 1, "4 pairs = 1 reserve unit"},
		{5, 1, "5 pairs = 1 reserve unit (boundary)"},
		{6, 2, "6 pairs = 2 reserve units (crosses boundary)"},
		{7, 2, "7 pairs = 2 reserve units"},
		{8, 2, "8 pairs = 2 reserve units"},
		{9, 2, "9 pairs = 2 reserve units"},
		{10, 2, "10 pairs = 2 reserve units (max)"},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			adj := oracle.CalculateOwnerCountAdjustment(tc.pairCount)
			assert.Equal(t, tc.expectedAdj, adj)
		})
	}
}

// =============================================================================
// PriceDataEntry Helper Tests
// =============================================================================

func TestPriceDataEntryHelpers(t *testing.T) {
	t.Run("TokenPairKey returns consistent key", func(t *testing.T) {
		entry := oracle.PriceDataEntry{
			BaseAsset:  "XRP",
			QuoteAsset: "USD",
		}
		key := entry.TokenPairKey()
		assert.Equal(t, "XRP/USD", key)
	})

	t.Run("IsDeleteRequest returns true when no price", func(t *testing.T) {
		entry := oracle.PriceDataEntry{
			BaseAsset:  "XRP",
			QuoteAsset: "USD",
			// AssetPrice is nil
		}
		assert.True(t, entry.IsDeleteRequest())
	})

	t.Run("IsDeleteRequest returns false when price present", func(t *testing.T) {
		price := uint64(740)
		entry := oracle.PriceDataEntry{
			BaseAsset:  "XRP",
			QuoteAsset: "USD",
			AssetPrice: &price,
		}
		assert.False(t, entry.IsDeleteRequest())
	})
}

// =============================================================================
// Complete Transaction Validation
// =============================================================================

func TestCompleteTransaction(t *testing.T) {
	alice := jtx.NewAccount("alice")

	t.Run("Complete valid OracleSet for creation", func(t *testing.T) {
		oset := oracletest.OracleSet(alice, 42, 750000000).
			ProviderHex(32).
			URIHex(64).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 74500, 2).
			Sequence(1).
			BuildOracleSet()

		err := oset.Validate()
		assert.NoError(t, err)

		// Also test ValidatePriceDataSeries for create
		toAdd, toDelete, err := oset.ValidatePriceDataSeries(false)
		assert.NoError(t, err)
		assert.Len(t, toAdd, 1)
		assert.Empty(t, toDelete)
	})
}

// =============================================================================
// Flatten Tests
// =============================================================================

func TestFlatten(t *testing.T) {
	t.Run("OracleSet flatten", func(t *testing.T) {
		alice := jtx.NewAccount("alice")
		oset := oracletest.OracleSet(alice, 42, 750000000).
			Provider("4D79507269636550726F7669646572").
			URI("68747470733A2F2F6578616D706C652E636F6D").
			AssetClass("63757272656E6379").
			AddPrice("USD", "XRP", 1234567890, 6).
			Sequence(1).
			BuildOracleSet()

		flat, err := oset.Flatten()
		require.NoError(t, err)

		assert.Equal(t, alice.Address, flat["Account"])
		assert.Equal(t, "OracleSet", flat["TransactionType"])
		assert.Equal(t, uint32(42), flat["OracleDocumentID"])
		assert.Equal(t, "4D79507269636550726F7669646572", flat["Provider"])
		assert.Equal(t, "68747470733A2F2F6578616D706C652E636F6D", flat["URI"])
		assert.Equal(t, "63757272656E6379", flat["AssetClass"])
		assert.Equal(t, uint32(750000000), flat["LastUpdateTime"])
	})

	t.Run("OracleDelete flatten", func(t *testing.T) {
		alice := jtx.NewAccount("alice")
		odel := oracletest.OracleDelete(alice, 99).
			Sequence(1).
			BuildOracleDelete()

		flat, err := odel.Flatten()
		require.NoError(t, err)

		assert.Equal(t, alice.Address, flat["Account"])
		assert.Equal(t, "OracleDelete", flat["TransactionType"])
		assert.Equal(t, uint32(99), flat["OracleDocumentID"])
	})
}

// =============================================================================
// Transaction Type Tests
// =============================================================================

func TestTransactionTypes(t *testing.T) {
	alice := jtx.NewAccount("alice")

	t.Run("OracleSet type", func(t *testing.T) {
		oset := oracletest.OracleSet(alice, 1, 750000000).BuildOracleSet()
		assert.Equal(t, "OracleSet", oset.TxType().String())
	})

	t.Run("OracleDelete type", func(t *testing.T) {
		odel := oracletest.OracleDelete(alice, 1).BuildOracleDelete()
		assert.Equal(t, "OracleDelete", odel.TxType().String())
	})
}

// =============================================================================
// Constructor Tests
// =============================================================================

func TestConstructors(t *testing.T) {
	t.Run("NewOracleSet", func(t *testing.T) {
		oset := oracle.NewOracleSet("rAccount123", 42, 750000000)
		require.NotNil(t, oset)
		assert.Equal(t, "rAccount123", oset.Account)
		assert.Equal(t, uint32(42), oset.OracleDocumentID)
		assert.Equal(t, uint32(750000000), oset.LastUpdateTime)
	})

	t.Run("NewOracleDelete", func(t *testing.T) {
		odel := oracle.NewOracleDelete("rAccount456", 99)
		require.NotNil(t, odel)
		assert.Equal(t, "rAccount456", odel.Account)
		assert.Equal(t, uint32(99), odel.OracleDocumentID)
	})
}

// =============================================================================
// Constants Tests
// =============================================================================

func TestConstants(t *testing.T) {
	assert.Equal(t, 256, oracle.MaxOracleURI)
	assert.Equal(t, 256, oracle.MaxOracleProvider)
	assert.Equal(t, 10, oracle.MaxOracleDataSeries)
	assert.Equal(t, 16, oracle.MaxOracleSymbolClass)
	assert.Equal(t, 300, oracle.MaxLastUpdateTimeDelta)
	// MaxPriceScale is 8 per rippled Oracle_test.cpp line 354
	assert.Equal(t, 8, oracle.MaxPriceScale)
}

// =============================================================================
// TestEnabled - Amendment Tests
// Reference: rippled Oracle_test.cpp testAmendment (lines 835-857)
// =============================================================================

// TestEnabled tests that Oracle operations are disabled without the PriceOracle amendment.
func TestEnabled(t *testing.T) {
	// Test 1: With amendment DISABLED, all Oracle transactions should return temDISABLED
	t.Run("Disabled", func(t *testing.T) {
		env := jtx.NewTestEnv(t)

		alice := jtx.NewAccount("alice")
		env.Fund(alice)
		env.Close()

		// Disable PriceOracle amendment
		env.DisableFeature("PriceOracle")

		// OracleSet should fail
		oracleSetTx := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			Build()
		result := env.Submit(oracleSetTx)
		if result.Code != "temDISABLED" {
			t.Errorf("OracleSet: expected temDISABLED, got %s", result.Code)
		}

		// OracleDelete should fail
		oracleDeleteTx := oracletest.OracleDelete(alice, 1).Build()
		result = env.Submit(oracleDeleteTx)
		if result.Code != "temDISABLED" {
			t.Errorf("OracleDelete: expected temDISABLED, got %s", result.Code)
		}
	})

	// Test 2: With amendment ENABLED, Oracle transactions should work (may fail for other reasons)
	t.Run("Enabled", func(t *testing.T) {
		env := jtx.NewTestEnv(t)

		alice := jtx.NewAccount("alice")
		env.Fund(alice)
		env.Close()

		// Create Oracle (amendment enabled by default)
		oracleSetTx := oracletest.OracleSet(alice, 1, 750000000).
			ProviderHex(32).
			AssetClassHex(8).
			AddPrice("XRP", "USD", 740, 1).
			Build()
		result := env.Submit(oracleSetTx)
		// Should not be temDISABLED since amendment is enabled
		if result.Code == "temDISABLED" {
			t.Errorf("OracleSet with amendment enabled should not return temDISABLED")
		}
		// Note: May fail for other reasons (e.g., LastUpdateTime validation),
		// but the important thing is it's not temDISABLED
		env.Close()
	})

	t.Log("testEnabled passed")
}
