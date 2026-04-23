package service

import (
	"testing"
	"time"

	"github.com/LeJamon/goXRPLd/internal/ledger/header"
	"github.com/LeJamon/goXRPLd/shamap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetValidatedLedger_StashesWhenSeqMissing_FiresOnAdopt pins F4:
// when SetValidatedLedger fires for a seq whose ledger has not yet been
// inserted into ledgerHistory (validation tracker leads the peer-adopt
// loop), the (seq, expectedHash) pair must be stashed and later promoted
// when the adopt path installs that ledger with a matching hash.
//
// Without this, a trusted validation for seq N that races ahead of the
// peer-adoption of N is silently dropped and validatedLedger never
// advances — the observable symptom is server_info.validated_ledger
// lagging behind closed_ledger indefinitely despite quorum being met.
func TestSetValidatedLedger_StashesWhenSeqMissing_FiresOnAdopt(t *testing.T) {
	cfg := DefaultConfig()
	svc, err := New(cfg)
	require.NoError(t, err)
	require.NoError(t, svc.Start())

	// Build a header+state that we intend to adopt shortly.
	txMap, err := shamap.New(shamap.TypeTransaction)
	require.NoError(t, err)
	txRoot, err := txMap.Hash()
	require.NoError(t, err)

	stateMap, err := shamap.New(shamap.TypeState)
	require.NoError(t, err)
	stateRoot, err := stateMap.Hash()
	require.NoError(t, err)

	var adoptedHash [32]byte
	adoptedHash[0] = 0xA1
	adoptedSeq := svc.GetClosedLedgerIndex() + 1
	hdr := &header.LedgerHeader{
		LedgerIndex: adoptedSeq,
		Hash:        adoptedHash,
		TxHash:      txRoot,
		AccountHash: stateRoot,
	}

	// Validation races ahead of adopt: call SetValidatedLedger *before*
	// the seq exists in ledgerHistory. Must NOT promote yet — just stash.
	startValidated := svc.GetValidatedLedgerIndex()
	svc.SetValidatedLedger(adoptedSeq, adoptedHash)

	assert.Equal(t, startValidated, svc.GetValidatedLedgerIndex(),
		"SetValidatedLedger must not advance validatedLedger when seq is not yet in history")

	// The stash must hold this seq pending a future adopt.
	svc.mu.RLock()
	pending, stashed := svc.pendingLedgerValidations[adoptedSeq]
	svc.mu.RUnlock()
	require.True(t, stashed,
		"SetValidatedLedger must stash (seq, expectedHash) when seq is not yet in history")
	assert.Equal(t, adoptedHash, pending.expectedHash)

	// When the adopt path installs this ledger, the stashed validation
	// must be drained and validatedLedger promoted to the adopted ledger.
	require.NoError(t, svc.AdoptLedgerWithState(hdr, stateMap, txMap))

	assert.Equal(t, adoptedSeq, svc.GetValidatedLedgerIndex(),
		"AdoptLedgerWithState must drain the stashed validation and promote validatedLedger")

	// Drain must remove the entry.
	svc.mu.RLock()
	_, stillStashed := svc.pendingLedgerValidations[adoptedSeq]
	svc.mu.RUnlock()
	assert.False(t, stillStashed,
		"adopt-drain must remove the stashed entry")
}

// TestSetValidatedLedger_StashExpires pins F4: a stashed validation older
// than pendingValidationTTL must be discarded on drain and must NOT
// promote the adopted ledger to validated. An expired stash is a staleness
// signal — by the time we adopt, the quorum gossip that produced the
// original validation is too old to trust without a fresh re-confirmation.
func TestSetValidatedLedger_StashExpires(t *testing.T) {
	cfg := DefaultConfig()
	svc, err := New(cfg)
	require.NoError(t, err)
	require.NoError(t, svc.Start())

	txMap, err := shamap.New(shamap.TypeTransaction)
	require.NoError(t, err)
	txRoot, err := txMap.Hash()
	require.NoError(t, err)

	stateMap, err := shamap.New(shamap.TypeState)
	require.NoError(t, err)
	stateRoot, err := stateMap.Hash()
	require.NoError(t, err)

	var adoptedHash [32]byte
	adoptedHash[0] = 0xB2
	adoptedSeq := svc.GetClosedLedgerIndex() + 1
	hdr := &header.LedgerHeader{
		LedgerIndex: adoptedSeq,
		Hash:        adoptedHash,
		TxHash:      txRoot,
		AccountHash: stateRoot,
	}

	startValidated := svc.GetValidatedLedgerIndex()

	// Stash a validation for this seq, then backdate `at` past the TTL.
	svc.SetValidatedLedger(adoptedSeq, adoptedHash)

	svc.mu.Lock()
	entry, ok := svc.pendingLedgerValidations[adoptedSeq]
	require.True(t, ok, "setup: stash must exist before backdate")
	entry.at = time.Now().Add(-2 * pendingValidationTTL)
	svc.pendingLedgerValidations[adoptedSeq] = entry
	svc.mu.Unlock()

	// Adopt — the drain must see the stale entry and refuse to promote.
	require.NoError(t, svc.AdoptLedgerWithState(hdr, stateMap, txMap))

	assert.Equal(t, startValidated, svc.GetValidatedLedgerIndex(),
		"expired stash must NOT promote validatedLedger on adopt")

	// The stale entry must be cleaned up regardless of promotion outcome.
	svc.mu.RLock()
	_, stillStashed := svc.pendingLedgerValidations[adoptedSeq]
	svc.mu.RUnlock()
	assert.False(t, stillStashed,
		"drain must delete expired entries even when refusing to promote")
}

// TestSetValidatedLedger_StashHashMismatch pins F4: a stashed validation
// for hash A must NOT promote an adopted ledger whose hash is B. This is
// the fork-safety guard — if peers validated a different hash at that seq
// than the one we adopted locally, our adopted ledger is on the wrong
// fork and silently promoting it to validated would be a correctness bug.
// Mirrors the inline-hash-match guard SetValidatedLedger already applies
// when the seq IS present in history.
func TestSetValidatedLedger_StashHashMismatch(t *testing.T) {
	cfg := DefaultConfig()
	svc, err := New(cfg)
	require.NoError(t, err)
	require.NoError(t, svc.Start())

	txMap, err := shamap.New(shamap.TypeTransaction)
	require.NoError(t, err)
	txRoot, err := txMap.Hash()
	require.NoError(t, err)

	stateMap, err := shamap.New(shamap.TypeState)
	require.NoError(t, err)
	stateRoot, err := stateMap.Hash()
	require.NoError(t, err)

	// Peer-adoption path will install hash B.
	var adoptedHashB [32]byte
	adoptedHashB[0] = 0xC3

	// But trusted validation arrived earlier referencing hash A.
	var validatedHashA [32]byte
	validatedHashA[0] = 0xC4

	adoptedSeq := svc.GetClosedLedgerIndex() + 1
	hdr := &header.LedgerHeader{
		LedgerIndex: adoptedSeq,
		Hash:        adoptedHashB,
		TxHash:      txRoot,
		AccountHash: stateRoot,
	}

	startValidated := svc.GetValidatedLedgerIndex()

	// Stash validation for A.
	svc.SetValidatedLedger(adoptedSeq, validatedHashA)

	svc.mu.RLock()
	entry, stashed := svc.pendingLedgerValidations[adoptedSeq]
	svc.mu.RUnlock()
	require.True(t, stashed, "setup: stash must exist before adopt")
	require.Equal(t, validatedHashA, entry.expectedHash)

	// Adopt B. Must NOT promote — fork signal.
	require.NoError(t, svc.AdoptLedgerWithState(hdr, stateMap, txMap))

	assert.Equal(t, startValidated, svc.GetValidatedLedgerIndex(),
		"hash mismatch between stashed validation and adopted ledger must NOT promote")

	// The stale-hash entry must be cleaned up so it can't accidentally
	// match a later adopt at the same seq.
	svc.mu.RLock()
	_, stillStashed := svc.pendingLedgerValidations[adoptedSeq]
	svc.mu.RUnlock()
	assert.False(t, stillStashed,
		"drain must delete hash-mismatched entries on the seq-adopt side")
}
