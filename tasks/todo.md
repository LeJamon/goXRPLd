# Issue #268 — Full LedgerTrie `getPreferred`

Replace the flat trusted-count approximation in `internal/consensus/rcl/validations.go`'s `GetTrustedSupport` with a faithful Go port of rippled's `LedgerTrie<Ledger>` (rippled/src/xrpld/consensus/LedgerTrie.h).

Design lives in [`pr-ledgertrie.md`](./pr-ledgertrie.md). This file tracks execution.

## Plan

### Phase 1 — Branch hygiene
- [x] Save WIP modifications as patch
- [x] Reset `feature/ledgertrie-268` to `origin/main` (cb373ac)
- [x] Reapply patch with 3-way merge
- [x] Resolve conflicts in `startup.go`, `engine.go`, `validations.go`
- [x] Build clean from worktree

### Phase 2 — Audit implementation vs rippled
- [x] `Mismatch` (binary search) — matches rippled's `mismatch` (csf/impl/ledgers.cpp:57-83)
- [x] `Insert` three-suffix case analysis — matches LedgerTrie.h:452-531
- [x] `Remove` compaction loop — matches LedgerTrie.h:540-589
- [x] `GetPreferred` within-span advancement + best-child + tie-break +1 margin — matches LedgerTrie.h:684-778
- [x] `Span` helpers, `node` struct — matches `ledger_trie_detail` (LedgerTrie.h:75-269)

### Phase 3 — Test coverage
- [x] Consolidate Insert/Remove/Support discrete tests into `TestLedgerTrie_ParityTable` with `t.Run` subtests
- [x] `TestGetPreferred_PrefersDeepestSharedAncestor` (issue-required) — passes
- [x] `TestGetPreferred_MinSeqRespected` (issue-required) — passes
- [x] `TestLedgerTrie_Empty` (rippled testEmpty) — kept as top-level
- [x] `TestLedgerTrie_RootRelated` (rippled testRootRelated) — kept as top-level
- [x] `TestLedgerTrie_Stress_InvariantsHold` (rippled testStress) — kept as top-level
- [x] `TestLedgerTrie_GetPreferred_*` (rippled testGetPreferred subscenarios) — kept as discrete tests

### Phase 4 — Integration + regression check
- [x] `go build ./...` clean
- [x] `go test ./internal/consensus/ledgertrie/... -count=1 -race` passes
- [x] `go test ./internal/consensus/rcl/... -count=1 -race` passes
- [ ] `go test ./... -count=1` — no new regressions vs baseline (in progress)

### Phase 5 — Commit + PR
- [ ] `feat(consensus/ledgertrie): port rippled LedgerTrie<Ledger>`
- [ ] `feat(consensus/rcl): wire LedgerTrie into ValidationTracker`
- [ ] `feat(consensus/rcl): expose LedgerTrie via Engine.SetLedgerAncestryProvider`
- [ ] Push branch and open PR referencing #268

## Out of scope

- Threading `largestIssued` through `Engine.checkLedger` — flat-pair compare still works.
- WebSocket `path_find` subscription — separate issue.
- Production-path coverage of the ancestry provider beyond `adaptor/startup.go` — wired to `ledgerSvc` already.

## Review (filled in after Phase 5)

_TBD_
