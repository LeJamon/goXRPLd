package metrics

import (
	"testing"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/message"
)

func TestTrafficCountBasic(t *testing.T) {
	tc := NewTrafficCount()

	// Add some inbound traffic
	tc.AddCount(CategoryTransaction, true, 100)
	tc.AddCount(CategoryTransaction, true, 200)

	// Add some outbound traffic
	tc.AddCount(CategoryTransaction, false, 150)

	stats := tc.GetStats(CategoryTransaction)
	if stats == nil {
		t.Fatal("Stats should not be nil")
	}

	if stats.BytesIn != 300 {
		t.Errorf("Expected BytesIn 300, got %d", stats.BytesIn)
	}

	if stats.BytesOut != 150 {
		t.Errorf("Expected BytesOut 150, got %d", stats.BytesOut)
	}

	if stats.MessagesIn != 2 {
		t.Errorf("Expected MessagesIn 2, got %d", stats.MessagesIn)
	}

	if stats.MessagesOut != 1 {
		t.Errorf("Expected MessagesOut 1, got %d", stats.MessagesOut)
	}
}

func TestTrafficCountCategorize(t *testing.T) {
	tests := []struct {
		msgType  message.MessageType
		inbound  bool
		expected Category
	}{
		{message.TypePing, true, CategoryBase},
		{message.TypeCluster, true, CategoryCluster},
		{message.TypeEndpoints, true, CategoryOverlay},
		{message.TypeManifests, true, CategoryManifests},
		{message.TypeTransaction, true, CategoryTransaction},
		{message.TypeValidation, true, CategoryValidation},
		{message.TypeSquelch, true, CategorySquelch},
		{message.TypeUnknown, true, CategoryUnknown},
	}

	for _, tc := range tests {
		result := Categorize(tc.msgType, tc.inbound)
		if result != tc.expected {
			t.Errorf("Categorize(%v, %v) = %v, expected %v", tc.msgType, tc.inbound, result, tc.expected)
		}
	}
}

func TestTrafficCountGetAllStats(t *testing.T) {
	tc := NewTrafficCount()

	tc.AddCount(CategoryTransaction, true, 100)
	tc.AddCount(CategoryValidation, true, 50)

	allStats := tc.GetAllStats()

	if len(allStats) == 0 {
		t.Error("GetAllStats should return all categories")
	}

	txStats := allStats[CategoryTransaction]
	if txStats == nil || txStats.BytesIn != 100 {
		t.Error("Transaction stats should be correct")
	}
}

func TestTrafficCountReset(t *testing.T) {
	tc := NewTrafficCount()

	tc.AddCount(CategoryTransaction, true, 100)
	tc.Reset()

	stats := tc.GetStats(CategoryTransaction)
	if stats.BytesIn != 0 {
		t.Errorf("Expected BytesIn 0 after reset, got %d", stats.BytesIn)
	}
}

func TestCategoryString(t *testing.T) {
	tests := []struct {
		cat      Category
		expected string
	}{
		{CategoryBase, "overhead"},
		{CategoryTransaction, "transactions"},
		{CategoryValidation, "validations"},
		{CategoryTotal, "total"},
		{CategoryUnknown, "unknown"},
	}

	for _, tc := range tests {
		if tc.cat.String() != tc.expected {
			t.Errorf("Category(%d).String() = %s, expected %s", tc.cat, tc.cat.String(), tc.expected)
		}
	}
}
