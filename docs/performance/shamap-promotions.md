# SHAMap promotion measurements

## Method

The benchmark compares scalar promotion (`batch=0`) with bounded batches of 64 and 256 nodes at one, two, and four workers. The byte budget is 4 MiB; a single oversized record may exceed it so traversal always progresses.

Each fixture has 16,384 synthetic state leaves and a deterministic genesis and close time. Both generations are closed, explicitly flushed offline, and reopened before each timed traversal. For cold cases the 256 KiB shared Pebble block cache is smaller than the persisted SSTables, checked at runtime. Warm cases use an 8 MiB cache and assert cache hits during the timed traversal. NodeStore decoded caches are disabled. `warm=false` starts with a fresh Pebble cache; `warm=true` performs one untimed traversal first. The OS page cache is not evicted, so cold refers to Pebble blocks and decompression, not cold physical storage.

Fixture creation, warming, metric snapshots, rotation, offline flush, reopen, and cleanup are excluded from benchmark timing. GC runs after fixture preparation and before each timed iteration so construction garbage does not carry into the measurement. Timing includes traversal, verification, promotion, and final Sync. Three iterations per result and three independent repetitions are collected in CI; tables use medians. Hardware performance assertions are deliberately absent from tests.

The concurrent benchmark starts real `persistToNodeStore` operations after refresh begins. Each iteration writes 32 successive closed ledgers with 32 changes each through the same Service and writable NodeStore, including ledger headers and Sync. It reports all persistence latencies and, separately, operations whose start overlaps refresh. The latter prevents post-refresh operations from hiding contention. Overlap means wall-clock coexistence: it includes intervals where refresh waits at the persistence-priority gate, not necessarily simultaneous backend I/O. This isolates NodeStore persistence and does not simulate a full consensus network.

Block-cache misses indicate block fetches, not physical disk reads. The pinned Pebble API exposes compaction read bytes but no foreground physical-read-byte counter. The materialized SSTable footprint is sampled after the untimed rotation and flush, when the old source archive has been retired. WAL amplification is WAL bytes divided by logical write bytes (Pebble WAL.BytesIn); it is not total long-term LSM write amplification. Flush and compaction counters cover the timed refresh interval, excluding later fixture preparation. CPU, allocation, block, and mutex profiles are captured separately from the unprofiled measurements. Go profiles include untimed fixture setup and teardown; use them as whole-run diagnostic evidence, not per-refresh attribution.

The prefetch phase pins the generation pair and holds an archive read lock to exclude deletion. It reads a bounded sorted prefix without acquiring writable mutation stripes. The commit phase acquires only that prefix's stripes in ascending order, creates a fresh writable iterator, applies writable precedence, and writes the archive hits in one batch. Delete batches acquire the archive write lock before stripes. Prefetched values and returned values have separate bounded payload budgets; the Pebble write batch also owns its encoded copy. Each budget permits one oversized first value. The 4 MiB limit is not a claim that the whole operation allocates only 4 MiB.

Correctness CI compares complete reachable and promoted hash sets after retiring the old archive, exercises byte-limited partial batches, cancellation and failures after actual promotion, lazy-value read faults, and a paused-prefetch same-key write followed by deletion. The missing-key test also covers an oversized first value.

Native refresh batches wait, with context cancellation, while that Service has active ledger persistence. Foreground writes never wait on this admission mechanism; already-admitted batches can finish. Sustained foreground work can therefore extend refresh duration. The scalar baseline keeps the original scheduling behavior.

## Results and defaults

Measured revision: `518a187b5d95018a343be3fe5a1b3e62b82be76a`, [CI run 34091933927, attempt 2](https://github.com/LeJamon/go-xrpl/actions/runs/34091933927/attempts/2), September 7, 2026. Environment: Go 1.24.4, Linux amd64, four virtual CPUs on AMD EPYC 7763. Each cell below is the median of three benchmark results, each with three timed iterations. Persistence quantiles pool the writes within those iterations, then the table takes the median reported quantile across the three results.

| Workers | Batch nodes | Cold refresh (ms) | Warm refresh (ms) | Refresh with persistence (ms) | Overlap persist p50 (ms) | Overlap persist p95 (ms) |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 0 | 522.956 | 109.290 | 582.453 | 1.184 | 2.203 |
| 1 | 64 | 273.046 | 77.222 | 355.733 | 1.039 | 1.916 |
| 1 | 256 | 144.983 | 65.631 | 226.322 | 1.039 | 2.022 |
| 2 | 0 | 302.800 | 83.948 | 332.697 | 1.357 | 2.688 |
| 2 | 64 | 159.406 | 83.853 | 217.828 | 1.064 | 1.962 |
| 2 | 256 | 95.733 | 53.412 | 148.322 | 1.013 | 1.862 |
| 4 | 0 | 272.732 | 67.554 | 288.546 | 1.770 | 3.731 |
| 4 | 64 | 116.297 | 68.323 | 165.352 | 1.045 | 1.921 |
| 4 | 256 | 69.044 | 45.168 | 119.243 | 1.045 | 1.855 |

At four workers, 256-node batches reduced cold refresh time by 75% and overlapping persistence p95 by 50% against the scalar path in this run. The 256-node choice also outperformed 64 nodes for cold refresh at each worker count, while persistence p95 stayed below scalar at every worker count. Keep the existing worker-count default. The 4 MiB payload cap is a safety limit, not a tuned throughput optimum: the largest observed batch payload in this fixture was 133,376 bytes. Oversized-record and partial-batch correctness tests exercise the byte-limit behavior separately.

Four-worker cold storage and allocation measurements:

| Metric per refresh unless stated | Scalar | 64 nodes | 256 nodes |
| --- | ---: | ---: | ---: |
| Block-cache misses per node | 3 | 0.6855 | 0.2886 |
| WAL bytes | 2,488,865 | 2,093,904 | 2,089,285 |
| Materialized SSTable bytes | 1,497,873 | 1,496,605 | 1,493,818 |
| Allocated bytes | 13,011,597 | 16,430,688 | 18,506,693 |
| Allocation count | 206,323 | 195,260 | 191,041 |

The batch path lowers block fetch/decompression demand, WAL overhead, and allocation count, but allocates more total bytes. WAL amplification falls from 1.084 to 1.001; the materialized SSTable footprint does not increase. No flush or compaction bytes were observed inside these short timed refresh intervals, so this does not establish long-running compaction amplification. Whole-run CPU profiles include fixture construction; they are diagnostic support rather than isolated refresh CPU measurements.

### Run variability

[Attempt 1 on the identical revision](https://github.com/LeJamon/go-xrpl/actions/runs/34091933927/attempts/1) used an Intel Xeon 6973P-C runner and produced unstable wall-clock comparisons. Four-worker scalar/64/256 cold times were 367.369/157.854/325.175 ms; warm times were 162.523/340.155/175.544 ms. Overlapping persistence p95 was 162.745/62.972/121.928 ms at four workers, but 91.168/91.122/175.917 ms at one worker. These results do not support a universal latency guarantee. Stable cache, batch, WAL, and SSTable counts and the unchanged-head repeat support the storage-work reduction; elapsed-time results remain host and workload dependent. The repeat was run without tuning code between attempts.

## Reproduction

The `Promotion performance` workflow records the revision and CPU environment and uploads the full matrix and comparable CPU, allocation, block, and mutex profiles. Its unprofiled command is:

```sh
go test ./internal/ledger/service -run '^$' \
  -bench '^BenchmarkService_Refresh' -benchmem -benchtime=3x -count=3 -timeout 20m
```

Run it in CI for the finalization workflow. A green performance job means the benchmark and its correctness checks completed; timing acceptance requires inspecting the artifact.
