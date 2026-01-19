// Package metrics implements traffic counting and metrics for XRPL peer connections.
package metrics

import (
	"sync"
	"sync/atomic"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/message"
)

// Category represents a traffic category for counting.
type Category int

const (
	// CategoryBase is basic peer overhead.
	CategoryBase Category = iota
	// CategoryCluster is cluster overhead.
	CategoryCluster
	// CategoryOverlay is overlay management.
	CategoryOverlay
	// CategoryManifests is manifest management.
	CategoryManifests
	// CategoryTransaction is transaction messages.
	CategoryTransaction
	// CategoryTransactionDuplicate is duplicate transaction messages.
	CategoryTransactionDuplicate
	// CategoryProposal is proposal messages.
	CategoryProposal
	// CategoryProposalUntrusted is proposals from untrusted validators.
	CategoryProposalUntrusted
	// CategoryProposalDuplicate is proposals seen previously.
	CategoryProposalDuplicate
	// CategoryValidation is validation messages.
	CategoryValidation
	// CategoryValidationUntrusted is validations from untrusted validators.
	CategoryValidationUntrusted
	// CategoryValidationDuplicate is validations seen previously.
	CategoryValidationDuplicate
	// CategoryValidatorList is validator list messages.
	CategoryValidatorList
	// CategorySquelch is squelch messages.
	CategorySquelch
	// CategorySquelchSuppressed is egress traffic suppressed by squelching.
	CategorySquelchSuppressed
	// CategorySquelchIgnored is traffic from peers ignoring squelch.
	CategorySquelchIgnored
	// CategoryGetSet is transaction sets we try to get.
	CategoryGetSet
	// CategoryShareSet is transaction sets we receive.
	CategoryShareSet
	// CategoryLedgerData is ledger data messages.
	CategoryLedgerData
	// CategoryGetLedger is get ledger messages.
	CategoryGetLedger
	// CategoryProofPath is proof path messages.
	CategoryProofPath
	// CategoryReplayDelta is replay delta messages.
	CategoryReplayDelta
	// CategoryHaveTransactions is have transactions messages.
	CategoryHaveTransactions
	// CategoryRequestedTransactions is requested transactions.
	CategoryRequestedTransactions
	// CategoryTotal is total traffic.
	CategoryTotal
	// CategoryUnknown is unknown message types.
	CategoryUnknown
)

// String returns the string representation of a category.
func (c Category) String() string {
	names := map[Category]string{
		CategoryBase:                  "overhead",
		CategoryCluster:               "overhead_cluster",
		CategoryOverlay:               "overhead_overlay",
		CategoryManifests:             "overhead_manifest",
		CategoryTransaction:           "transactions",
		CategoryTransactionDuplicate:  "transactions_duplicate",
		CategoryProposal:              "proposals",
		CategoryProposalUntrusted:     "proposals_untrusted",
		CategoryProposalDuplicate:     "proposals_duplicate",
		CategoryValidation:            "validations",
		CategoryValidationUntrusted:   "validations_untrusted",
		CategoryValidationDuplicate:   "validations_duplicate",
		CategoryValidatorList:         "validator_lists",
		CategorySquelch:               "squelch",
		CategorySquelchSuppressed:     "squelch_suppressed",
		CategorySquelchIgnored:        "squelch_ignored",
		CategoryGetSet:                "set_get",
		CategoryShareSet:              "set_share",
		CategoryLedgerData:            "ledger_data",
		CategoryGetLedger:             "ledger_get",
		CategoryProofPath:             "proof_path",
		CategoryReplayDelta:           "replay_delta",
		CategoryHaveTransactions:      "have_transactions",
		CategoryRequestedTransactions: "requested_transactions",
		CategoryTotal:                 "total",
		CategoryUnknown:               "unknown",
	}
	if name, ok := names[c]; ok {
		return name
	}
	return "unknown"
}

// Stats holds traffic statistics for a category.
type Stats struct {
	Name        string
	BytesIn     uint64
	BytesOut    uint64
	MessagesIn  uint64
	MessagesOut uint64
}

// atomicStats holds atomic counters for thread-safe updates.
type atomicStats struct {
	bytesIn     atomic.Uint64
	bytesOut    atomic.Uint64
	messagesIn  atomic.Uint64
	messagesOut atomic.Uint64
}

// TrafficCount tracks ingress and egress traffic by category.
type TrafficCount struct {
	mu     sync.RWMutex
	counts map[Category]*atomicStats
}

// NewTrafficCount creates a new TrafficCount.
func NewTrafficCount() *TrafficCount {
	tc := &TrafficCount{
		counts: make(map[Category]*atomicStats),
	}

	// Initialize all categories
	categories := []Category{
		CategoryBase, CategoryCluster, CategoryOverlay, CategoryManifests,
		CategoryTransaction, CategoryTransactionDuplicate,
		CategoryProposal, CategoryProposalUntrusted, CategoryProposalDuplicate,
		CategoryValidation, CategoryValidationUntrusted, CategoryValidationDuplicate,
		CategoryValidatorList,
		CategorySquelch, CategorySquelchSuppressed, CategorySquelchIgnored,
		CategoryGetSet, CategoryShareSet,
		CategoryLedgerData, CategoryGetLedger,
		CategoryProofPath, CategoryReplayDelta,
		CategoryHaveTransactions, CategoryRequestedTransactions,
		CategoryTotal, CategoryUnknown,
	}

	for _, cat := range categories {
		tc.counts[cat] = &atomicStats{}
	}

	return tc
}

// AddCount records traffic for a category.
func (tc *TrafficCount) AddCount(cat Category, inbound bool, bytes int) {
	tc.mu.RLock()
	stats, exists := tc.counts[cat]
	tc.mu.RUnlock()

	if !exists {
		return
	}

	if inbound {
		stats.bytesIn.Add(uint64(bytes))
		stats.messagesIn.Add(1)
	} else {
		stats.bytesOut.Add(uint64(bytes))
		stats.messagesOut.Add(1)
	}
}

// Categorize determines the traffic category for a message type.
func Categorize(msgType message.MessageType, inbound bool) Category {
	switch msgType {
	case message.TypePing:
		return CategoryBase
	case message.TypeCluster:
		return CategoryCluster
	case message.TypeEndpoints:
		return CategoryOverlay
	case message.TypeManifests:
		return CategoryManifests
	case message.TypeTransaction:
		return CategoryTransaction
	case message.TypeProposeLedger:
		return CategoryProposal
	case message.TypeValidation:
		return CategoryValidation
	case message.TypeValidatorList, message.TypeValidatorListCollection:
		return CategoryValidatorList
	case message.TypeSquelch:
		return CategorySquelch
	case message.TypeHaveSet:
		if inbound {
			return CategoryGetSet
		}
		return CategoryShareSet
	case message.TypeGetLedger, message.TypeLedgerData:
		if inbound {
			return CategoryGetLedger
		}
		return CategoryLedgerData
	case message.TypeProofPathReq, message.TypeProofPathResponse:
		return CategoryProofPath
	case message.TypeReplayDeltaReq, message.TypeReplayDeltaResponse:
		return CategoryReplayDelta
	case message.TypeHaveTransactions:
		return CategoryHaveTransactions
	case message.TypeTransactions:
		return CategoryRequestedTransactions
	default:
		return CategoryUnknown
	}
}

// GetStats returns statistics for a category.
func (tc *TrafficCount) GetStats(cat Category) *Stats {
	tc.mu.RLock()
	stats, exists := tc.counts[cat]
	tc.mu.RUnlock()

	if !exists {
		return nil
	}

	return &Stats{
		Name:        cat.String(),
		BytesIn:     stats.bytesIn.Load(),
		BytesOut:    stats.bytesOut.Load(),
		MessagesIn:  stats.messagesIn.Load(),
		MessagesOut: stats.messagesOut.Load(),
	}
}

// GetAllStats returns statistics for all categories.
func (tc *TrafficCount) GetAllStats() map[Category]*Stats {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	result := make(map[Category]*Stats)
	for cat, stats := range tc.counts {
		result[cat] = &Stats{
			Name:        cat.String(),
			BytesIn:     stats.bytesIn.Load(),
			BytesOut:    stats.bytesOut.Load(),
			MessagesIn:  stats.messagesIn.Load(),
			MessagesOut: stats.messagesOut.Load(),
		}
	}
	return result
}

// GetTotalStats returns the total traffic statistics.
func (tc *TrafficCount) GetTotalStats() *Stats {
	return tc.GetStats(CategoryTotal)
}

// Reset resets all counters.
func (tc *TrafficCount) Reset() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	for _, stats := range tc.counts {
		stats.bytesIn.Store(0)
		stats.bytesOut.Store(0)
		stats.messagesIn.Store(0)
		stats.messagesOut.Store(0)
	}
}
