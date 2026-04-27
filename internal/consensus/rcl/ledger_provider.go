package rcl

import (
	"sync"

	"github.com/LeJamon/goXRPLd/internal/consensus"
	"github.com/LeJamon/goXRPLd/internal/consensus/ledgertrie"
	"github.com/LeJamon/goXRPLd/internal/ledger"
	"github.com/LeJamon/goXRPLd/internal/ledger/service"
)

// LedgerHeader is the narrow slice of *ledger.Ledger the provider
// reads. Exposed as an interface so unit tests can stub it without
// constructing a full ledger. *ledger.Ledger satisfies it.
type LedgerHeader interface {
	Sequence() uint32
	Hash() [32]byte
	ParentHash() [32]byte
}

// Static assertion that *ledger.Ledger satisfies LedgerHeader.
var _ LedgerHeader = (*ledger.Ledger)(nil)

// hashLookupFunc resolves a ledger hash to its header. Returning an
// interface (rather than the concrete *ledger.Ledger) is what lets
// tests inject fake lookups; the production constructor adapts
// *service.Service.GetLedgerByHash into this shape.
type hashLookupFunc func(hash [32]byte) (LedgerHeader, error)

// LedgerProvider satisfies LedgerAncestryProvider by resolving a
// LedgerID via the ledger service and materializing the ledger's full
// ancestor chain on demand.
//
// The trie calls Ancestor(s) many times per insert (binary search in
// ledgertrie.Mismatch plus the span operations). Walking back via
// ParentHash on every call would be O(depth²) with the service's
// current O(n) hash scan — so we materialize the ancestor slice once
// per (LedgerID) query and cache the result. Ancestor slices are
// immutable once a ledger is closed, so the cache needs no
// invalidation.
//
// Ancestry walk terminates when it reaches sequence 1 (genesis per
// XRPL convention) or when a parent lookup fails. On a failed parent
// lookup partway up the chain, LedgerByID returns (nil, false): we
// cannot produce a correct trie-insertable ledger without the full
// chain. The tracker then silently skips the trie insert and the
// validation still lands in byNode / flat-count.
type LedgerProvider struct {
	lookup hashLookupFunc

	mu    sync.Mutex
	cache map[consensus.LedgerID]*providerLedger
}

// NewLedgerProvider wraps the production ledger service into a
// provider suitable for ValidationTracker.SetLedgerAncestryProvider.
// nil svc yields a provider that always returns (nil, false) —
// useful as a disabled placeholder without special-casing at the
// call site.
func NewLedgerProvider(svc *service.Service) *LedgerProvider {
	if svc == nil {
		return &LedgerProvider{cache: make(map[consensus.LedgerID]*providerLedger)}
	}
	return newLedgerProviderFromLookup(func(hash [32]byte) (LedgerHeader, error) {
		l, err := svc.GetLedgerByHash(hash)
		if err != nil {
			return nil, err
		}
		return l, nil
	})
}

// newLedgerProviderFromLookup is the internal constructor used by
// production (NewLedgerProvider wraps *service.Service) and by tests
// (pass a closure backed by a fake header map).
func newLedgerProviderFromLookup(fn hashLookupFunc) *LedgerProvider {
	return &LedgerProvider{
		lookup: fn,
		cache:  make(map[consensus.LedgerID]*providerLedger),
	}
}

// LedgerByID implements LedgerAncestryProvider.
func (p *LedgerProvider) LedgerByID(id consensus.LedgerID) (ledgertrie.Ledger, bool) {
	if p == nil || p.lookup == nil {
		return nil, false
	}
	p.mu.Lock()
	if cached, ok := p.cache[id]; ok {
		p.mu.Unlock()
		return cached, true
	}
	p.mu.Unlock()

	built := p.buildChain(id)
	if built == nil {
		return nil, false
	}

	p.mu.Lock()
	// Double-check: another goroutine may have populated concurrently.
	if cached, ok := p.cache[id]; ok {
		p.mu.Unlock()
		return cached, true
	}
	p.cache[id] = built
	p.mu.Unlock()
	return built, true
}

// buildChain walks parent hashes from `id` back to seq 1, producing
// a fully-populated ancestor slice. Returns nil when any link in the
// chain is missing from the store.
//
// Returned ancestors[0] is the all-zero LedgerID (pre-genesis
// placeholder — the trie's root span expects this). ancestors[s]
// for s >= 1 is the hash of the ledger at that sequence on the
// chain ending at `id`.
func (p *LedgerProvider) buildChain(id consensus.LedgerID) *providerLedger {
	tip, err := p.lookup([32]byte(id))
	if err != nil || tip == nil {
		return nil
	}
	tipSeq := tip.Sequence()
	if tipSeq == 0 {
		// Seq-0 is our pre-genesis fiction; a real ledger must have
		// seq >= 1. If the service somehow returned a seq-0 ledger
		// we cannot represent it faithfully.
		return nil
	}

	ancestors := make([]consensus.LedgerID, tipSeq+1)
	// ancestors[0] stays zero by construction.
	ancestors[tipSeq] = consensus.LedgerID(tip.Hash())

	// Walk back: each iteration populates ancestors[s-1] and then
	// loads the ledger whose hash is ancestors[s-1] so its ParentHash
	// gives the next seat.
	curr := tip
	for s := tipSeq; s > 1; s-- {
		parentHash := consensus.LedgerID(curr.ParentHash())
		ancestors[s-1] = parentHash

		// If a cached ancestor already has ancestors materialized
		// back to seq 0/1, splice its prefix in rather than walking
		// the rest of the chain.
		p.mu.Lock()
		cachedParent, hit := p.cache[parentHash]
		p.mu.Unlock()
		if hit && uint32(len(cachedParent.ancestors)) >= s {
			copy(ancestors[:s], cachedParent.ancestors[:s])
			break
		}

		parent, err := p.lookup([32]byte(parentHash))
		if err != nil || parent == nil {
			return nil
		}
		curr = parent
	}

	return &providerLedger{
		id:        id,
		seq:       tipSeq,
		ancestors: ancestors,
	}
}

// providerLedger is the trie's view of a production ledger: just the
// (id, seq, ancestors) triple. Satisfies ledgertrie.Ledger.
type providerLedger struct {
	id        consensus.LedgerID
	seq       uint32
	ancestors []consensus.LedgerID
}

func (l *providerLedger) ID() consensus.LedgerID { return l.id }
func (l *providerLedger) Seq() uint32            { return l.seq }

// Ancestor returns the ID of the ancestor at sequence s. Panics when
// s is beyond the chain — matches the TestLedger contract and the
// trie's expectation (LedgerTrie.h:SpanTip::ancestor uses XRPL_ASSERT
// for s <= seq).
func (l *providerLedger) Ancestor(s uint32) consensus.LedgerID {
	if s > l.seq {
		panic("providerLedger.Ancestor: s > seq")
	}
	return l.ancestors[s]
}
