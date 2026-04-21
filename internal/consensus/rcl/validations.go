package rcl

import (
	"sync"
	"time"

	"github.com/LeJamon/goXRPLd/internal/consensus"
)

// ValidationTracker tracks validations and determines ledger finality.
type ValidationTracker struct {
	mu sync.RWMutex

	// validations maps ledger ID to validations for that ledger
	validations map[consensus.LedgerID]map[consensus.NodeID]*consensus.Validation

	// byNode maps node ID to their latest validation
	byNode map[consensus.NodeID]*consensus.Validation

	// trusted is the set of trusted validators
	trusted map[consensus.NodeID]bool

	// quorum is the number of validations needed for finality
	quorum int

	// freshness is how long validations are considered fresh
	freshness time.Duration

	// fired records ledgers we've already reported as fully validated,
	// so the callback fires exactly once per ledger even if more
	// validations keep arriving after the threshold is crossed.
	fired map[consensus.LedgerID]struct{}

	// callbacks
	onFullyValidated func(ledgerID consensus.LedgerID, ledgerSeq uint32)
}

// NewValidationTracker creates a new validation tracker.
func NewValidationTracker(quorum int, freshness time.Duration) *ValidationTracker {
	return &ValidationTracker{
		validations: make(map[consensus.LedgerID]map[consensus.NodeID]*consensus.Validation),
		byNode:      make(map[consensus.NodeID]*consensus.Validation),
		trusted:     make(map[consensus.NodeID]bool),
		quorum:      quorum,
		freshness:   freshness,
		fired:       make(map[consensus.LedgerID]struct{}),
	}
}

// SetTrusted updates the set of trusted validators.
func (vt *ValidationTracker) SetTrusted(nodes []consensus.NodeID) {
	vt.mu.Lock()
	defer vt.mu.Unlock()

	vt.trusted = make(map[consensus.NodeID]bool)
	for _, node := range nodes {
		vt.trusted[node] = true
	}
}

// SetQuorum updates the quorum requirement.
func (vt *ValidationTracker) SetQuorum(quorum int) {
	vt.mu.Lock()
	defer vt.mu.Unlock()
	vt.quorum = quorum
}

// SetFullyValidatedCallback sets the callback for when a ledger is fully validated.
// Fired once per ledger the first time trusted-validation count crosses the quorum
// threshold. Seq is passed alongside the ledger ID so the callee can look up or
// stamp the ledger without a secondary map lookup.
func (vt *ValidationTracker) SetFullyValidatedCallback(fn func(ledgerID consensus.LedgerID, ledgerSeq uint32)) {
	vt.mu.Lock()
	defer vt.mu.Unlock()
	vt.onFullyValidated = fn
}

// Add adds a validation to the tracker.
// Returns true if this is a new validation (not duplicate).
func (vt *ValidationTracker) Add(validation *consensus.Validation) bool {
	vt.mu.Lock()
	defer vt.mu.Unlock()

	// Check if this is a newer validation from this node
	existing, hasExisting := vt.byNode[validation.NodeID]
	if hasExisting {
		if validation.LedgerSeq <= existing.LedgerSeq {
			return false // Not newer, ignore
		}
	}

	// Update by-node tracking
	vt.byNode[validation.NodeID] = validation

	// Add to ledger validations
	ledgerVals, exists := vt.validations[validation.LedgerID]
	if !exists {
		ledgerVals = make(map[consensus.NodeID]*consensus.Validation)
		vt.validations[validation.LedgerID] = ledgerVals
	}
	ledgerVals[validation.NodeID] = validation

	// Check for full validation
	vt.checkFullValidation(validation.LedgerID)

	return true
}

// checkFullValidation checks if a ledger has reached full validation.
// Fires the callback exactly once per ledger — the first time trusted
// count crosses the quorum threshold. Subsequent adds for the same
// ledger are ignored to avoid repeatedly flipping server_info's
// validated_ledger on every late-arriving peer validation.
//
// Zero-quorum edge case (empty UNL): requires at least one tracked
// validation for the ledger before firing, so we don't spuriously
// promote a ledger hash we haven't seen any validator sign.
func (vt *ValidationTracker) checkFullValidation(ledgerID consensus.LedgerID) {
	if vt.onFullyValidated == nil {
		return
	}
	if _, done := vt.fired[ledgerID]; done {
		return
	}
	ledgerVals, exists := vt.validations[ledgerID]
	if !exists || len(ledgerVals) == 0 {
		return
	}

	trustedCount := 0
	var sampleSeq uint32
	for nodeID, v := range ledgerVals {
		if sampleSeq == 0 {
			sampleSeq = v.LedgerSeq
		}
		if vt.trusted[nodeID] {
			trustedCount++
		}
	}

	if trustedCount >= vt.quorum {
		vt.fired[ledgerID] = struct{}{}
		vt.onFullyValidated(ledgerID, sampleSeq)
	}
}

// GetValidations returns all validations for a ledger.
func (vt *ValidationTracker) GetValidations(ledgerID consensus.LedgerID) []*consensus.Validation {
	vt.mu.RLock()
	defer vt.mu.RUnlock()

	ledgerVals, exists := vt.validations[ledgerID]
	if !exists {
		return nil
	}

	result := make([]*consensus.Validation, 0, len(ledgerVals))
	for _, v := range ledgerVals {
		result = append(result, v)
	}
	return result
}

// GetTrustedValidations returns trusted validations for a ledger.
func (vt *ValidationTracker) GetTrustedValidations(ledgerID consensus.LedgerID) []*consensus.Validation {
	vt.mu.RLock()
	defer vt.mu.RUnlock()

	ledgerVals, exists := vt.validations[ledgerID]
	if !exists {
		return nil
	}

	var result []*consensus.Validation
	for nodeID, v := range ledgerVals {
		if vt.trusted[nodeID] {
			result = append(result, v)
		}
	}
	return result
}

// GetValidationCount returns the count of validations for a ledger.
func (vt *ValidationTracker) GetValidationCount(ledgerID consensus.LedgerID) int {
	vt.mu.RLock()
	defer vt.mu.RUnlock()

	ledgerVals, exists := vt.validations[ledgerID]
	if !exists {
		return 0
	}
	return len(ledgerVals)
}

// GetTrustedValidationCount returns the count of trusted validations.
func (vt *ValidationTracker) GetTrustedValidationCount(ledgerID consensus.LedgerID) int {
	vt.mu.RLock()
	defer vt.mu.RUnlock()

	ledgerVals, exists := vt.validations[ledgerID]
	if !exists {
		return 0
	}

	count := 0
	for nodeID := range ledgerVals {
		if vt.trusted[nodeID] {
			count++
		}
	}
	return count
}

// IsFullyValidated returns true if the ledger has reached full validation.
func (vt *ValidationTracker) IsFullyValidated(ledgerID consensus.LedgerID) bool {
	return vt.GetTrustedValidationCount(ledgerID) >= vt.quorum
}

// GetLatestValidation returns the latest validation from a node.
func (vt *ValidationTracker) GetLatestValidation(nodeID consensus.NodeID) *consensus.Validation {
	vt.mu.RLock()
	defer vt.mu.RUnlock()
	return vt.byNode[nodeID]
}

// GetCurrentValidators returns nodes that have recently validated.
func (vt *ValidationTracker) GetCurrentValidators() []consensus.NodeID {
	vt.mu.RLock()
	defer vt.mu.RUnlock()

	cutoff := time.Now().Add(-vt.freshness)
	var result []consensus.NodeID

	for nodeID, v := range vt.byNode {
		if v.SignTime.After(cutoff) {
			result = append(result, nodeID)
		}
	}
	return result
}

// ExpireOld removes old validations.
func (vt *ValidationTracker) ExpireOld(minSeq uint32) {
	vt.mu.Lock()
	defer vt.mu.Unlock()

	for ledgerID, ledgerVals := range vt.validations {
		for _, v := range ledgerVals {
			if v.LedgerSeq < minSeq {
				delete(vt.validations, ledgerID)
				delete(vt.fired, ledgerID)
			}
			break
		}
	}
}

// Clear removes all tracked validations.
func (vt *ValidationTracker) Clear() {
	vt.mu.Lock()
	defer vt.mu.Unlock()

	vt.validations = make(map[consensus.LedgerID]map[consensus.NodeID]*consensus.Validation)
	vt.byNode = make(map[consensus.NodeID]*consensus.Validation)
	vt.fired = make(map[consensus.LedgerID]struct{})
}

// Stats returns statistics about tracked validations.
type ValidationStats struct {
	TotalValidations   int
	TrustedValidations int
	ValidatorsActive   int
	LedgersTracked     int
}

// GetStats returns current validation statistics.
func (vt *ValidationTracker) GetStats() ValidationStats {
	vt.mu.RLock()
	defer vt.mu.RUnlock()

	totalValidations := 0
	trustedValidations := 0

	for _, ledgerVals := range vt.validations {
		for nodeID := range ledgerVals {
			totalValidations++
			if vt.trusted[nodeID] {
				trustedValidations++
			}
		}
	}

	return ValidationStats{
		TotalValidations:   totalValidations,
		TrustedValidations: trustedValidations,
		ValidatorsActive:   len(vt.byNode),
		LedgersTracked:     len(vt.validations),
	}
}
