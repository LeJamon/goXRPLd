# PR: Full LedgerTrie `getPreferred` (issue #268)

Replace the flat hash-count approximation at
`internal/consensus/rcl/validations.go:396-406` with a faithful port of
rippled's `LedgerTrie<Ledger>`.

## Why

`GetTrustedSupport` currently returns the count of trusted validations at
the *exact* ledger ID. Rippled's `LedgerTrie` instead returns
**branchSupport** — the sum of support across a ledger and every
descendant that chains through it. Under an overlapping-ancestor fork
topology, a minority near-tip otherwise outpolls a majority-further-back
branch that actually won network-wide, and a catchup node can strand on
the stale fork.

## Rippled reference

- `rippled/src/xrpld/consensus/LedgerTrie.h` — the whole file (853 LoC;
  853-line header, all core algorithms inline).
- `rippled/src/xrpld/consensus/Validations.h::getPreferred` — call-site.
- `rippled/src/test/consensus/LedgerTrie_test.cpp` — parity tuple
  tests + `testGetPreferred` (683 LoC).
- `rippled/src/test/csf/ledgers.h` — the test-only Ledger mock with
  `operator[](Seq)` ancestry lookup and a string-notation helper
  (`h["abc"]`).

## Design

### New package `internal/consensus/ledgertrie/`

A self-contained port of the trie, generic over a `Ledger` interface.
Not tied to the CSF simulator or the RCL ValidationTracker — so both
real node code and the test ledger can use it.

```go
package ledgertrie

type Ledger interface {
    ID() consensus.LedgerID
    Seq() uint32
    // Ancestor returns the ID of the ancestor at sequence s; for
    // s == Seq() it returns ID(); for s == 0 it returns the genesis ID.
    Ancestor(s uint32) consensus.LedgerID
}

// Mismatch is the free function from rippled: the first sequence where
// a and b disagree. Exposed as a package-level func so callers can
// supply a specialised implementation when they already have ancestry
// cached.
func Mismatch(a, b Ledger) uint32

type SpanTip struct {
    Seq uint32
    ID  consensus.LedgerID
    lgr Ledger  // for Ancestor lookup
}

func (t SpanTip) Ancestor(s uint32) consensus.LedgerID

type Trie struct { ... }  // exported type

func New() *Trie
func (t *Trie) Insert(l Ledger, count uint32)
func (t *Trie) Remove(l Ledger, count uint32) bool
func (t *Trie) TipSupport(l Ledger) uint32
func (t *Trie) BranchSupport(l Ledger) uint32
func (t *Trie) GetPreferred(largestIssued uint32) (SpanTip, bool)
func (t *Trie) Empty() bool
func (t *Trie) CheckInvariants() bool
```

Unexported: `span`, `node` structs (mirroring rippled's
`ledger_trie_detail`).

### Algorithm fidelity

Port verbatim, function-for-function:

- `span.before(spot) / from(spot) / sub(from, to)` → `clamp` + optional
  non-empty span.
- `span.diff(other Ledger)` → `clamp(Mismatch(span.ledger, other))`.
- `span.tip()` → reads ledger.Ancestor(end-1).
- Compression invariant: "a non-root 0-tip node must have 0 or ≥2
  children". Enforced in `Remove` via the same while-loop that merges
  single-child chains up the tree.
- `Insert` — exact three-suffix case analysis from rippled
  (`prefix`/`oldSuffix`/`newSuffix`). Rewire parent pointers when old
  suffix moves under the truncated prefix.
- `GetPreferred` — within-span advancement over `seqSupport`, then
  between-spans `partial_sort`(top 2 by branchSupport, tie-break
  descending by `span.startID()`), descend while `margin > uncommitted`
  or `uncommitted == 0`.

`seqSupport` uses a `map[uint32]uint32` plus a sorted `[]uint32` of
keys so `GetPreferred` can walk in order without allocating. (Rippled
relies on `std::map` ordered iteration; Go's map is unordered so we
keep the key list sorted on insert/remove.)

Partial_sort of two children: on ≥2 children, copy into a temp slice,
use `sort.Slice` to fully sort (N ≥ 2 is small in practice; not
worth a custom partial_sort). Primary key is `branchSupport DESC`;
tie-break is `startID DESC` (lexicographic on [32]byte).

### Integration with `ValidationTracker`

Add two fields to `ValidationTracker`:

```go
ancestry LedgerAncestryProvider   // injected; may be nil
trie     *ledgertrie.Trie         // nil when ancestry is nil
```

Where:

```go
// LedgerAncestryProvider resolves a LedgerID to a full Ledger
// (with ancestry). Returns (nil, false) if the ledger's history
// is unknown — e.g., validations arrive for ledgers we haven't
// acquired yet.
type LedgerAncestryProvider interface {
    LedgerByID(id consensus.LedgerID) (ledgertrie.Ledger, bool)
}
```

- `Add(v *Validation)` — after the existing accept/stale checks, if
  `trie != nil` and `ancestry.LedgerByID(v.LedgerID)` succeeds, and
  the validator is trusted, call `trie.Insert(lgr, 1)`. Mirror
  rippled's `updateTrie` which de-duplicates on (nodeID, ledgerID): we
  remove the validator's *previous* trusted ledger before inserting
  the new one. Store a `trieInserted map[NodeID]consensus.LedgerID` to
  know what to remove.
- `GetTrustedSupport(id)` — if `trie != nil` and ancestry resolves
  `id`, return `trie.BranchSupport(lgr)`. Otherwise fall back to the
  existing flat trusted count (so unit tests and nodes without an
  ancestry provider still work).
- New method `GetPreferred(largestIssued uint32) (consensus.LedgerID,
  uint32, bool)` — thin wrapper around `trie.GetPreferred`; returns
  `(id, seq, ok=false)` when the trie is empty or not wired.

NegUNL filtering: the trie counts trusted inserts; validators on the
negUNL are skipped at `Insert` time (same gate as
`countTrustedExcludingNegUNLLocked`). This matches the existing
post-filter semantics at the call site.

### Updating `checkLedger` (engine.go:932-938)

Keep the existing compare (`netSupport <= ourSupport`) — but now both
sides come from `GetTrustedSupport` which consults the trie. No API
change required at the call site.

## Files to add

- `internal/consensus/ledgertrie/trie.go` (core data structure)
- `internal/consensus/ledgertrie/span.go` (Span + Node internals)
- `internal/consensus/ledgertrie/testledger.go` (build-tag-free test
  helper: `TestLedger` with stored ancestors, `Builder` with `h["abc"]`
  string notation). Public so rcl tests can reuse it.
- `internal/consensus/ledgertrie/trie_test.go` (parity tests below)

## Files to modify

- `internal/consensus/rcl/validations.go` — add
  `LedgerAncestryProvider` field, insert into trie on `Add`, route
  `GetTrustedSupport` through the trie with fallback, add
  `GetPreferred`.
- `internal/consensus/rcl/validations_test.go` — add a case that
  exercises the trie branch (existing flat-count cases still pass).

## Tests

### Acceptance (required by issue)

1. **`TestLedgerTrie_ParityTable`** — port the tuple-style tests from
   `LedgerTrie_test.cpp` covering `insert`, `remove`, and simple
   `tipSupport`/`branchSupport` assertions on small trees. Concretely,
   port `testInsert`, `testRemove`, `testSupport`, and the
   invariant-checking cases from the C++ file as sub-tests driven off
   a table of (operation, ledger-string, expected state) tuples.
2. **`TestGetPreferred_PrefersDeepestSharedAncestor`** — the motivating
   scenario from the issue:

   ```
          /-> C       (1 trusted)
     G -> B
          \-> D -> E  (2 trusted at E)
   ```

   Flat count: C wins with 1 at exact hash (both D and E score 0 at
   B's sibling), actually rippled's flat alias would pick E (2 >
   other, but fails to compare at B's level). The trie picks B-through-D
   at seq 3 because branchSupport(D) = 2 > branchSupport(C) = 1.
3. **`TestGetPreferred_MinSeqRespected`** — when `largestIssued`
   skips the seqSupport entries below it, `uncommitted` is seeded
   from those entries so that ancient support cannot retroactively
   swing preference. Ported from `testGetPreferred` lines 480-504.

### Additional parity (not strictly required, but cheap)

- `TestInsertCompressionAndSplit` — verifies oldSuffix split leaves
  parent tipSupport=0 and children correctly rewired.
- `TestRemoveCompaction` — verifies single-child merge on removal
  (span.merge behaviour).
- `TestCheckInvariantsStress` — 1000 random insert/remove ops, each
  followed by `CheckInvariants()`.

## Out of scope

- WebSocket `path_find` subscription (separate issue).
- Threading `largestIssued` through `Engine.checkLedger` — the current
  approximation works off pairwise comparison; a future refactor can
  switch to `GetPreferred` directly.
- Populating a real `LedgerAncestryProvider` from the ledger
  store/manager. For this PR the provider is an injectable interface;
  default production construction passes `nil` (keeps flat-count
  behaviour until the ancestry provider is wired in). A follow-up
  can wire it to `internal/ledger/manager` — non-trivial because
  validations may reference ledgers the local node has not yet
  acquired.

## Verification

1. `go build ./...` clean.
2. `go test ./internal/consensus/ledgertrie/... -count=1 -race`
3. `go test ./internal/consensus/rcl/... -count=1 -race`
4. `go test ./... -count=1` with no regression vs main (pre-existing
   failures logged in `memory/MEMORY.md` remain).
