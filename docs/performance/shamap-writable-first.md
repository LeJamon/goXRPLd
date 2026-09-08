# Writable-first SHAMap promotion

This extends the bounded batching measurements in [SHAMap promotions](shamap-promotions.md). The batching work in #1857 is already on main. This change addresses #1866 under #1853: avoid archive lookups and payload copies for keys that are present in the writable generation.

## Ownership and correctness

The batch pins the generation pair and holds archive deletion exclusion through commit. For each sorted key, it prefetches writable first and consults archive only on a miss, outside the mutation stripes. Both sources share one bounded payload prefix. A writable hit cannot disappear while deletion is excluded, so its archive lookup is unnecessary. A concurrent replacement still changes the mutation version and requires the existing fresh writable lookup before returning a value. Misses still consult the archive, and a later writable replacement can supersede an archive lazy-value error.

Node-count and payload limits, sorted prefix semantics, content-hash and type verification, checkpoint pacing, final Sync, and rotation ownership are unchanged. Superseded oversized archive values no longer shorten the prefix. Lazy value lengths are checked before fetching a payload that exceeds the remaining budget; its read and any associated error are deferred until that record is requested in the next batch. There is no persistent cache, frontier checkpoint, membership set, or restart state to invalidate. The worker default remains unchanged.

The prerequisite `70b6aad9` moves cold writable prefetch outside mutation stripes and uses the ledger read lock during SHAMap persistence so immutable header reads can proceed. The PR includes that prerequisite; it does not import the remaining consensus/recovery changes from `fix/consensus-storage-stalls`. Changed-version fallback reads can still occur under mutation stripes, as tracked separately in #1867.

Rippled 3.3.0 (`00a178fb92ca49521b937ae1a99d863765ea8a90`) checks writable before archive in `DatabaseRotatingImp::fetchNodeObject` and visits the validated state before rotation in `SHAMapStoreImp`. Sorted batched promotion and these metrics are specific to go-xrpl; they preserve full reachable-state verification and promotion.

## Verification

The archive-read regression test uses persisted, reopened SSTables and an injected error on archive SST reads. Counters are armed after background table statistics load. It fails against the prerequisite implementation, which reads one archive data block for a writable hit, and passes with zero archive reads on the new implementation. A second injected-error test proves an archive-filled prefix does not read the next oversized writable payload, while retrying that payload still propagates its read error. It fails against the previous separate-prefetch implementation. Concurrent writable replacement, empty values, superseded oversized archive records, required archive lazy-value failures, and deletion exclusion remain covered.

The storage, NodeStore, ledger service, SHAMap rotation, ledger persistence, and node shutdown packages pass under the race detector. Existing refresh tests independently compare the complete reachable and promoted hash sets and exercise partial batches, checkpoint failure, cancellation, recovery, and archive retirement.

## Refresh diagnostics

Each batched refresh now accumulates its own `uint64` counters and emits them in progress, completion, failure, and cancellation logs:

- `promotion_requested`, `promotion_consumed`, and `promotion_partial_prefixes` distinguish requested suffixes from returned records. Requested keys can exceed consumed keys because bounded partial batches retry their unreturned suffix.
- `promotion_writable_hits`, `promotion_writable_misses`, `promotion_archive_hits`, `promotion_archive_misses`, `promotion_archive_lookups`, and `promotion_archive_lookups_avoided` expose generation work. Prefetch lookups can include keys beyond the returned prefix.
- `promotion_promoted`, `promotion_promoted_bytes`, `promotion_buffered_bytes`, `promotion_batch_writes`, `promotion_batch_calls`, and `promotion_batch_errors` describe returned backend work. On failure, promoted counters can include staged writes that did not commit. Successful batch writes still precede the refresh's final durability Sync. Buffered bytes are cumulative returned payload, not peak memory.
- `promotion_fetch_elapsed` sums backend fetch durations; `promotion_wait_elapsed` sums persistence-admission durations. Concurrent workers' sums can exceed refresh wall time. Wait duration includes admission-check overhead.

These counters exclude scalar root/frontier bootstrap fetches and other NodeStore activity. Existing `nodes_checked` covers full traversal, while the existing NodeStore before/after counters remain shared activity. Source/write counters require a backend that supplies them; the scalar-only backend fallback supplies request, consumption, and payload counters only. No full-tree identity set is retained, so these counters do not claim exact unique-node or cross-refresh duplicate counts.

## Benchmark method

`BenchmarkPromotionReadAmplification` uses 1,024 deterministic hash-distributed keys with 1 KiB values. Keys present in writable have different values from their archive records. Each iteration clones an immutable, explicitly flushed two-generation fixture, reopens both generations, waits for background table-stat loading, optionally warms both generations, and runs GC before timing. The 0%, 50%, and 100% writable-hit distributions are checked on every iteration, together with exact keys, values, consumed counts, and promoted counts.

The matrix compares one, two, and four workers, cold/warm Pebble data blocks, and normal/delayed SST reads. Its 256-key requests use a deliberately small 128 KiB payload budget to exercise partial-prefix retries; this is a stress fixture, not a change to the production 4 MiB limit. A shared 8 MiB block cache can retain both fixture generations. Warm cases require cache hits and zero misses/SST reads. Cold cases require misses and SST reads. Cold refers to Pebble data blocks: metadata is loaded before timing and the OS page cache is not evicted.

A VFS wrapper counts successful SST read calls and returned bytes per generation. These are filesystem reads, not physical disk I/O or decompressed-byte counters. The delayed case adds 50 microseconds per SST read; scheduler delays can exceed that value. This is a repeatable contention model, not an HDD latency claim. Concurrent cold misses can read the same block more than once, so worker counts can change counted bytes even with the same requested keys.

Cold cases start a foreground Put while the first refresh read is held at a barrier. Foreground key/value preparation and expected-result construction are outside timing. Put quantiles include operations started before refresh completes; warm runs report the actual overlap sample count. The separate existing `BenchmarkService_RefreshWithPersistence` measures real ledger persistence, including headers and Sync. Neither benchmark simulates a consensus network.

CPU profiles label timed refresh/promotion goroutines with `phase=refresh` or `phase=promotion`; filtering by that label excludes fixture construction but does not attribute pre-existing Pebble background workers. Allocation profiles cover whole runs, while benchmark B/op and allocs/op use timed regions only. All before/after pairs use the same fixture code and workload.
## Results

Measured on an Apple M3 Pro, Darwin arm64, Go 1.26.1, with 11 Go scheduler processors. The promotion baseline is `180a35d2` (main `64b730a7` plus the cold-writable prerequisite); the final implementation is `3ff8641a`. A Go source overlay selects the baseline implementation while keeping the corrected benchmark fixture identical. Each of the 36 cases ran three iterations in each of three independent samples. The following values are medians of the three reported samples; [the complete CSV](shamap-writable-first-results.csv) includes warm cases, cache counts, allocations, worker utilization, and overlapping Put p50/p95/p99.

| Writable hits | Workers | SST delay | Archive bytes before → after | Total SST bytes before → after | Refresh ms before → after |
| --- | ---: | --- | ---: | ---: | ---: |
| 0% | 1 | none | 1,094,051 → 1,094,051 | 1,094,051 → 1,094,051 | 30.36 → 27.21 |
| 0% | 4 | 50 µs | 2,740,224 → 3,677,812 | 2,740,224 → 3,677,812 | 31.15 → 22.72 |
| 50% | 1 | none | 1,094,051 → 1,094,051 | 1,641,191 → 1,641,191 | 18.74 → 13.57 |
| 50% | 4 | 50 µs | 3,588,106 → 2,102,673 | 5,475,175 → 3,238,015 | 15.08 → 11.44 |
| 100% | 1 | none | 1,094,051 → 0 | 2,188,102 → 1,094,051 | 5.831 → 2.851 |
| 100% | 4 | none | 2,326,428 → 0 | 5,218,930 → 2,857,686 | 2.072 → 2.145 |
| 100% | 1 | 50 µs | 1,094,051 → 0 | 2,188,102 → 1,094,051 | 8.820 → 4.976 |
| 100% | 4 | 50 µs | 3,848,668 → 0 | 7,994,167 → 4,018,099 | 7.156 → 4.238 |

All rows above use cold Pebble data blocks. Every all-writable case eliminates archive SST reads. With one worker this halves counted SST bytes, while timed allocation drops from 2,471,416 to 1,476,970 B/op (40%) in the normal-I/O case. Warm cases read no SST bytes in either implementation. At 50% hits, avoiding half the key lookups need not avoid any blocks: unresolved hash-distributed keys still touch every archive data block with one worker.

Elapsed times do not show a universal speedup: the four-worker, all-writable normal-I/O case is slightly slower, and concurrent archive-only cases read more bytes despite finishing sooner. Multiple workers can duplicate cold block loads, so the delayed four-worker archive-only case's counted bytes rise 34%. Foreground Put quantiles also vary: archive-only normal-I/O one-worker p95 changes from 2.667 to 3.500 µs, while all-writable one-worker p95 changes from 2.833 to 1.458 µs. These short host-local runs support the archive-work reduction, not a stronger physical-HDD or consensus-latency claim. They do not justify increasing the worker default.

### Full refresh and ledger persistence

The end-to-end baseline uses current main `64b730a7`, so this comparison includes the cold-writable and ledger-lock prerequisite. The existing refresh fixture has 16,384 leaves, 256-node/4 MiB batches, and a 256 KiB cold or 8 MiB warm block cache. Each result below is one three-iteration sample; [the full end-to-end CSV](shamap-writable-first-service-results.csv) retains throughput, allocations, cache activity, WAL/materialization, and persistence statistics. These shorter runs are diagnostic comparisons, not statistical confidence intervals.

| Cache | Workers | Refresh ms before → after | Nodes/s before → after | B/op before → after |
| --- | ---: | ---: | ---: | ---: |
| cold | 1 | 144.35 → 126.21 | 121,517 → 138,979 | 16,413,864 → 17,473,213 |
| cold | 2 | 136.17 → 100.06 | 128,820 → 175,302 | 18,900,269 → 20,088,280 |
| cold | 4 | 104.64 → 62.16 | 167,634 → 282,175 | 19,066,066 → 20,162,341 |
| warm | 1 | 108.91 → 74.53 | 161,060 → 235,356 | 17,144,466 → 17,581,602 |
| warm | 2 | 103.97 → 62.74 | 168,720 → 279,578 | 18,934,437 → 19,960,922 |
| warm | 4 | 87.34 → 94.66 | 200,846 → 185,301 | 19,522,864 → 20,323,586 |

The persistence fixture overlaps 32 real ledger persists per iteration, with 32 modified state entries per ledger plus header writes and Sync. All 32 persists overlapped refresh in each worker configuration.

| Workers | Refresh ms before → after | Overlapping persist p95 ms before → after |
| ---: | ---: | ---: |
| 1 | 1323.26 → 454.49 | 48.65 → 16.04 |
| 2 | 858.45 → 421.40 | 39.40 → 17.08 |
| 4 | 448.27 → 366.88 | 18.61 → 15.61 |

The final full-refresh path allocates more than main because it retains writable prefetch records and aggregates refresh metrics. The warm four-worker sample also takes longer (87.34 → 94.66 ms). Persistence p95 improves in these runs, but no live consensus network or physical-HDD soak was measured. Issue #1853 remains open for that broader deployment evidence, exact decompression accounting, and unique/cross-refresh identity measurements.

### Profiles

Paired labeled CPU and allocation profiles were captured for the cold four-worker full refresh (three iterations) and cold all-writable one-worker promotion (30 iterations), using identical fixtures. The full-refresh labeled CPU samples total 0.53 s before and 0.51 s after; system calls remain dominant. Promotion labeled samples total 0.11 s before and 0.06 s after, with only 11 and 6 samples respectively. These counts are too small for a reliable percentage CPU-speedup claim.

Whole-run sampled allocation attributed directly to `pointIterator.get` is 67.57 MiB before and 30.53 MiB after in the promotion profile. Fixture setup is included in allocation profiles, so use the timed B/op measurements above for the per-operation comparison. Raw profiles and logs are retained in the local issue audit directory; no production database or live-node profile is needed to reproduce the synthetic workload.

## Reproduction

From the final checkout, run the same fixture against both implementations. The overlay below selects the prerequisite storage baseline without replacing the final benchmark or counter schema:

```sh
python3 - <<'PYTHON'
import json, subprocess, tempfile
from pathlib import Path
root = Path.cwd()
tmp = Path(tempfile.mkdtemp(prefix="promotion-baseline-"))
replacements = {}
for relative in ("storage/kvstore/pebble/rotating.go", "storage/kvstore/pebble/pebble.go"):
    source = tmp / Path(relative).name
    source.write_bytes(subprocess.check_output(["git", "show", "180a35d2:" + relative]))
    replacements[str(root / relative)] = str(source)
print(tmp / "overlay.json")
(tmp / "overlay.json").write_text(json.dumps({"Replace": replacements}))
PYTHON

# Set GOFLAGS to -overlay=<printed path> for the baseline run; unset it for after.
just test-pkg './storage/kvstore/pebble -run ^$ -bench BenchmarkPromotionReadAmplification -benchtime=3x -count=3'
just test-pkg './internal/ledger/service -run ^$ -bench BenchmarkService_RefreshValidatedState/warm=/workers=/batch=256 -benchtime=3x'
just test-pkg './internal/ledger/service -run ^$ -bench BenchmarkService_RefreshWithPersistence/workers=/batch=256 -benchtime=3x'
```

The full-refresh current-main baseline restores both Pebble files above plus `storage/kvstore/kvstore.go`, `internal/ledger/persistence.go`, `internal/ledger/service/persistence.go`, and `internal/ledger/service/fast_load_progress.go` from `64b730a7` and excludes the new `promotion_progress_test.go`. The benchmark fixture and profile labels stay unchanged. Run each pair sequentially without concurrent builds or tests.

For profiles, add `-cpuprofile cpu.pprof -memprofile alloc.pprof` to a single selected benchmark case. Inspect CPU with `go tool pprof -top -tagfocus=phase=promotion cpu.pprof` (or `phase=refresh` for the service fixture), and allocations with `go tool pprof -top -alloc_space alloc.pprof`.
