package pebble

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LeJamon/go-xrpl/storage/kvstore"
	cockroachpebble "github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
)

const (
	promotionBenchmarkKeyCount           = 256
	promotionBenchmarkValueBytes         = (4 << 20) / promotionBenchmarkKeyCount
	promotionBenchmarkPayload            = 4 << 20
	promotionBenchmarkIterations         = 32
	promotionBenchmarkCacheBytes         = 16 << 20
	promotionBenchmarkOpenFiles          = 128
	promotionBenchmarkReadDelay          = 250 * time.Microsecond
	promotionBenchmarkOverlapWait        = time.Second
	promotionBenchmarkMaxConflictRetries = 16
)

// TestPromotionBatchOfflineReport is intentionally opt-in. It creates a fresh,
// flushed, reopened two-generation store for every sample so archive-only keys
// promoted by one sample cannot become writable hits in another sample.
//
// Run with:
//
//	GOMAXPROCS=3 GOXRPL_PROMOTION_BENCH=1 go test -count=1 -run '^TestPromotionBatchOfflineReport$' -v ./storage/kvstore/pebble
//
// The report labels SST VFS reads as logical file API reads. They are not
// physical HDD reads, and the OS page cache is retained between fresh fixtures.
func TestPromotionBatchOfflineReport(t *testing.T) {
	if os.Getenv("GOXRPL_PROMOTION_BENCH") != "1" {
		t.Skip("set GOXRPL_PROMOTION_BENCH=1 to run the fixed offline measurement")
	}

	previousProcs := runtime.GOMAXPROCS(3)
	defer runtime.GOMAXPROCS(previousProcs)

	report, reportPath, err := newPromotionBenchmarkReport()
	if err != nil {
		t.Fatal(err)
	}
	defer report.Close()

	profiles := []promotionBenchmarkProfile{
		{name: "ssd-vfs", delay: 0},
		{name: "delayed-vfs", delay: promotionBenchmarkReadDelay},
	}
	hitCases := []promotionBenchmarkHitCase{
		{name: "0", writableKeys: 0},
		{name: "50", writableKeys: promotionBenchmarkKeyCount / 2},
		{name: "100", writableKeys: promotionBenchmarkKeyCount},
	}
	iterations, err := promotionBenchmarkIterationCount()
	if err != nil {
		t.Fatal(err)
	}

	fmt.Fprintf(report, "# gomaxprocs=3 os_cache=retained cache=logical_shared_block_cache reads=logical_sst_vfs_reads_not_physical_hdd_reads foreground=Put_NoSync\n")
	fmt.Fprintln(report, "storage\tos_cache\tsynthetic_delay\thit_rate\titerations\tpromote_p50_ns\tpromote_p95_ns\tpromote_p99_ns\tallocs_per_op\tbytes_alloc_per_op\tarchive_sst_reads_per_batch\tarchive_sst_bytes_per_batch\twritable_sst_reads_per_batch\twritable_sst_bytes_per_batch\tblock_cache_misses_per_batch\tblock_cache_hits_per_batch\toverlap_promote_p50_ns\toverlap_promote_p95_ns\toverlap_promote_p99_ns\tforeground_put_p50_ns\tforeground_put_p95_ns\tforeground_put_p99_ns\toverlap_samples\toverlap_complete_samples\tpromotion_conflicts_per_op\tarchive_lookups_per_batch\tarchive_lookups_avoided_per_batch\tprefetch_bytes_per_batch")

	t.Logf("promotion benchmark report: %s", reportPath)
	t.Logf("gomaxprocs=3 os_cache=retained reads=logical SST VFS reads, not physical HDD reads")
	for _, profile := range profiles {
		for _, hitCase := range hitCases {
			t.Run(profile.name+"/hits="+hitCase.name, func(t *testing.T) {
				samples := make([]promotionBenchmarkSample, 0, iterations)
				overlaps := make([]promotionBenchmarkOverlapSample, 0, iterations)
				for iteration := 0; iteration < iterations; iteration++ {
					sample, err := runPromotionBenchmarkSample(iteration, hitCase.writableKeys, profile.delay)
					if err != nil {
						t.Fatalf("%s %s%% promotion sample %d: %v", profile.name, hitCase.name, iteration, err)
					}
					samples = append(samples, sample)

					overlap, err := runPromotionBenchmarkOverlapSample(iteration, hitCase.writableKeys, profile.delay)
					if err != nil {
						t.Fatalf("%s %s%% overlap sample %d: %v", profile.name, hitCase.name, iteration, err)
					}
					overlaps = append(overlaps, overlap)
				}

				promoteSummary := summarizePromotionBenchmarkSamples(samples)
				overlapSummary := summarizePromotionBenchmarkOverlaps(overlaps)
				optional := summarizeOptionalPromotionStats(samples)
				line := formatPromotionBenchmarkReportLine(profile, hitCase, iterations, promoteSummary, overlapSummary, optional)
				fmt.Fprintln(report, line)
				t.Logf("%s", line)
			})
		}
	}
}

func promotionBenchmarkIterationCount() (int, error) {
	value := os.Getenv("GOXRPL_PROMOTION_BENCH_ITERATIONS")
	if value == "" {
		return promotionBenchmarkIterations, nil
	}
	iterations, err := strconv.Atoi(value)
	if err != nil || iterations < 1 {
		return 0, fmt.Errorf("GOXRPL_PROMOTION_BENCH_ITERATIONS must be a positive integer, got %q", value)
	}
	return iterations, nil
}

type promotionBenchmarkProfile struct {
	name  string
	delay time.Duration
}

type promotionBenchmarkHitCase struct {
	name         string
	writableKeys int
}

type promotionBenchmarkReadMetrics struct {
	readCalls atomic.Uint64
	readBytes atomic.Uint64
}

func (m *promotionBenchmarkReadMetrics) reset() {
	m.readCalls.Store(0)
	m.readBytes.Store(0)
}

func (m *promotionBenchmarkReadMetrics) snapshot() promotionBenchmarkReadSnapshot {
	return promotionBenchmarkReadSnapshot{
		readCalls: m.readCalls.Load(),
		readBytes: m.readBytes.Load(),
	}
}

type promotionBenchmarkReadSnapshot struct {
	readCalls uint64
	readBytes uint64
}

type promotionBenchmarkReadBarrier struct {
	once sync.Once
	read chan struct{}
}

func newPromotionBenchmarkReadBarrier() *promotionBenchmarkReadBarrier {
	return &promotionBenchmarkReadBarrier{read: make(chan struct{})}
}

func (b *promotionBenchmarkReadBarrier) signal() {
	if b == nil {
		return
	}
	b.once.Do(func() { close(b.read) })
}

// promotionBenchmarkFS counts reads of SST files and can add a deterministic
// delay to those reads. It delegates every other filesystem operation to the
// real filesystem, preserving Pebble's normal persistence behavior.
type promotionBenchmarkFS struct {
	vfs.FS
	metrics *promotionBenchmarkReadMetrics
	delay   time.Duration
	barrier atomic.Pointer[promotionBenchmarkReadBarrier]
}

type promotionBenchmarkNoopLogger struct{}

func (promotionBenchmarkNoopLogger) Infof(string, ...interface{}) {}
func (promotionBenchmarkNoopLogger) Fatalf(format string, args ...interface{}) {
	panic(fmt.Sprintf(format, args...))
}

func (f *promotionBenchmarkFS) Open(name string, options ...vfs.OpenOption) (vfs.File, error) {
	file, err := f.FS.Open(name, options...)
	if err != nil {
		return nil, err
	}
	return f.wrap(name, file), nil
}

func (f *promotionBenchmarkFS) OpenReadWrite(name string, options ...vfs.OpenOption) (vfs.File, error) {
	file, err := f.FS.OpenReadWrite(name, options...)
	if err != nil {
		return nil, err
	}
	return f.wrap(name, file), nil
}

func (f *promotionBenchmarkFS) wrap(name string, file vfs.File) vfs.File {
	return &promotionBenchmarkFile{
		File:    file,
		track:   filepath.Ext(name) == ".sst",
		metrics: f.metrics,
		delay:   f.delay,
		barrier: &f.barrier,
	}
}

type promotionBenchmarkFile struct {
	vfs.File
	track   bool
	metrics *promotionBenchmarkReadMetrics
	delay   time.Duration
	barrier *atomic.Pointer[promotionBenchmarkReadBarrier]
}

func (f *promotionBenchmarkFile) Read(p []byte) (int, error) {
	f.beforeRead()
	n, err := f.File.Read(p)
	f.afterRead(n)
	return n, err
}

func (f *promotionBenchmarkFile) ReadAt(p []byte, offset int64) (int, error) {
	f.beforeRead()
	n, err := f.File.ReadAt(p, offset)
	f.afterRead(n)
	return n, err
}

func (f *promotionBenchmarkFile) beforeRead() {
	if !f.track {
		return
	}
	f.metrics.readCalls.Add(1)
	if barrier := f.barrier.Load(); barrier != nil {
		barrier.signal()
	}
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
}

func (f *promotionBenchmarkFile) afterRead(bytesRead int) {
	if f.track && bytesRead > 0 {
		f.metrics.readBytes.Add(uint64(bytesRead))
	}
}

func (f *promotionBenchmarkFile) Flush() error {
	flusher, ok := f.File.(interface{ Flush() error })
	if !ok {
		return nil
	}
	return flusher.Flush()
}

type promotionBenchmarkFixture struct {
	root        string
	store       *RotatingStore
	keys        [][]byte
	writableFS  *promotionBenchmarkFS
	archiveFS   *promotionBenchmarkFS
	keepFixture bool
}

func openPromotionBenchmarkFixture(group, writableKeys int, delay time.Duration) (*promotionBenchmarkFixture, error) {
	rootBase := os.Getenv("GOXRPL_PROMOTION_BENCH_DIR")
	if rootBase == "" {
		rootBase = os.TempDir()
	}
	if err := os.MkdirAll(rootBase, 0o755); err != nil {
		return nil, fmt.Errorf("create benchmark root %s: %w", rootBase, err)
	}
	root, err := os.MkdirTemp(rootBase, "issue-1866-promotion-")
	if err != nil {
		return nil, fmt.Errorf("create benchmark fixture: %w", err)
	}
	fixture := &promotionBenchmarkFixture{
		root:        root,
		keepFixture: os.Getenv("GOXRPL_PROMOTION_BENCH_KEEP") == "1",
	}
	cleanup := func(openErr error) (*promotionBenchmarkFixture, error) {
		if fixture.store != nil {
			openErr = errors.Join(openErr, fixture.store.Close())
		}
		if !fixture.keepFixture {
			openErr = errors.Join(openErr, os.RemoveAll(root))
		}
		return nil, openErr
	}

	keys, values := promotionBenchmarkEntries(group)
	fixture.keys = keys
	if err := createPromotionBenchmarkDB(filepath.Join(root, "archive"), keys, values, promotionBenchmarkKeyCount); err != nil {
		return cleanup(err)
	}
	if err := createPromotionBenchmarkDB(filepath.Join(root, "writable"), keys, values, writableKeys); err != nil {
		return cleanup(err)
	}

	cache := cockroachpebble.NewCache(promotionBenchmarkCacheBytes)
	options := makePebbleOptions(Options{
		BlockCacheBytes: promotionBenchmarkCacheBytes,
		MaxOpenFiles:    promotionBenchmarkOpenFiles,
	}, cache)
	options.Logger = promotionBenchmarkNoopLogger{}

	fixture.writableFS = &promotionBenchmarkFS{
		FS:      vfs.Default,
		metrics: &promotionBenchmarkReadMetrics{},
		delay:   delay,
	}
	fixture.archiveFS = &promotionBenchmarkFS{
		FS:      vfs.Default,
		metrics: &promotionBenchmarkReadMetrics{},
		delay:   delay,
	}

	writableOptions := *options
	writableOptions.FS = fixture.writableFS
	writableDB, err := cockroachpebble.Open(filepath.Join(root, "writable"), &writableOptions)
	if err != nil {
		cache.Unref()
		return cleanup(fmt.Errorf("reopen writable generation: %w", err))
	}

	archiveOptions := *options
	archiveOptions.FS = readOnlyFS{FS: fixture.archiveFS}
	archiveOptions.ReadOnly = true
	archiveDB, err := cockroachpebble.Open(filepath.Join(root, "archive"), &archiveOptions)
	if err != nil {
		_ = writableDB.Close()
		cache.Unref()
		return cleanup(fmt.Errorf("reopen archive generation: %w", err))
	}

	fixture.store = &RotatingStore{
		writable:   &Store{db: writableDB},
		archive:    &Store{db: archiveDB, readOnly: true},
		blockCache: cache,
	}
	fixture.writableFS.metrics.reset()
	fixture.archiveFS.metrics.reset()
	return fixture, nil
}

func createPromotionBenchmarkDB(path string, keys, values [][]byte, count int) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	db, err := cockroachpebble.Open(path, &cockroachpebble.Options{
		FS:                 vfs.Default,
		MaxOpenFiles:       promotionBenchmarkOpenFiles,
		MemTableSize:       64 << 20,
		FormatMajorVersion: cockroachpebble.FormatMajorVersion(1),
		Logger:             promotionBenchmarkNoopLogger{},
	})
	if err != nil {
		return fmt.Errorf("open fixture generation %s: %w", path, err)
	}
	batch := db.NewBatch()
	for index := 0; index < count; index++ {
		if err := batch.Set(keys[index], values[index], nil); err != nil {
			_ = batch.Close()
			_ = db.Close()
			return fmt.Errorf("write fixture generation %s key %d: %w", path, index, err)
		}
	}
	if err := batch.Commit(cockroachpebble.NoSync); err != nil {
		_ = batch.Close()
		_ = db.Close()
		return fmt.Errorf("commit fixture generation %s: %w", path, err)
	}
	if err := batch.Close(); err != nil {
		_ = db.Close()
		return fmt.Errorf("close fixture batch %s: %w", path, err)
	}
	if err := db.Flush(); err != nil {
		_ = db.Close()
		return fmt.Errorf("flush fixture generation %s: %w", path, err)
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("close fixture generation %s: %w", path, err)
	}
	return nil
}

func (f *promotionBenchmarkFixture) close() error {
	var err error
	if f.store != nil {
		err = f.store.Close()
	}
	if !f.keepFixture {
		err = errors.Join(err, os.RemoveAll(f.root))
	}
	return err
}

func promotionBenchmarkEntries(group int) ([][]byte, [][]byte) {
	keys := make([][]byte, promotionBenchmarkKeyCount)
	values := make([][]byte, promotionBenchmarkKeyCount)
	for index := range keys {
		keys[index] = []byte(fmt.Sprintf("issue-1866/%08d/%03d", group, index))
		values[index] = promotionBenchmarkValue(group, index)
	}
	return keys, values
}

func promotionBenchmarkValue(group, index int) []byte {
	value := make([]byte, promotionBenchmarkValueBytes)
	seed := uint64(group+1)*0x9e3779b97f4a7c15 ^ uint64(index+1)*0xd1b54a32d192ed03
	for offset := range value {
		seed ^= seed >> 12
		seed ^= seed << 25
		seed ^= seed >> 27
		value[offset] = byte(seed * 0x2545f4914f6cdd1d >> 56)
	}
	return value
}

type promotionBenchmarkSample struct {
	promote        time.Duration
	allocs         uint64
	allocatedBytes uint64
	archiveReads   promotionBenchmarkReadSnapshot
	writableReads  promotionBenchmarkReadSnapshot
	cacheHits      uint64
	cacheMisses    uint64
	optional       promotionBenchmarkOptionalStats
}

type promotionBenchmarkPromotionRun struct {
	promotions []kvstore.Promotion
	stats      kvstore.PromotionStats
	optional   promotionBenchmarkOptionalStats
	conflicts  int
	complete   bool
}

// runPromotionBenchmarkGroup consumes prefixes until the complete 256-key
// group has been processed. A conflict returns no committed work on the
// patched implementation, so retry the same prefix and never count a partial
// result as a completed promotion.
func runPromotionBenchmarkGroup(store *RotatingStore, keys [][]byte) (promotionBenchmarkPromotionRun, error) {
	remaining := keys
	run := promotionBenchmarkPromotionRun{
		stats: kvstore.PromotionStats{Requested: len(keys)},
	}
	for len(remaining) > 0 {
		promotions, stats, err := store.PromoteBatch(remaining, promotionBenchmarkPayload)
		if err != nil {
			if !isPromotionBenchmarkConflict(err) {
				return promotionBenchmarkPromotionRun{}, err
			}
			run.conflicts++
			if run.conflicts >= promotionBenchmarkMaxConflictRetries {
				return run, nil
			}
			continue
		}
		if stats.Consumed <= 0 || stats.Consumed > len(remaining) {
			return promotionBenchmarkPromotionRun{}, fmt.Errorf(
				"promotion made no valid progress: consumed=%d remaining=%d",
				stats.Consumed,
				len(remaining),
			)
		}
		if len(promotions) != stats.Consumed {
			return promotionBenchmarkPromotionRun{}, fmt.Errorf(
				"promotion prefix length=%d, consumed=%d",
				len(promotions),
				stats.Consumed,
			)
		}
		addPromotionBenchmarkStats(&run.stats, stats)
		run.optional.add(readOptionalPromotionStats(stats))
		run.promotions = append(run.promotions, promotions...)
		remaining = remaining[stats.Consumed:]
	}
	run.complete = true
	return run, nil
}

func addPromotionBenchmarkStats(dst *kvstore.PromotionStats, src kvstore.PromotionStats) {
	dst.Consumed += src.Consumed
	dst.WritableHits += src.WritableHits
	dst.WritableMisses += src.WritableMisses
	dst.ArchiveHits += src.ArchiveHits
	dst.ArchiveMisses += src.ArchiveMisses
	dst.Promoted += src.Promoted
	dst.PromotedBytes += src.PromotedBytes
	dst.BufferedBytes += src.BufferedBytes
	dst.Batches += src.Batches
}

func runPromotionBenchmarkSample(group, writableKeys int, delay time.Duration) (promotionBenchmarkSample, error) {
	fixture, err := openPromotionBenchmarkFixture(group, writableKeys, delay)
	if err != nil {
		return promotionBenchmarkSample{}, err
	}
	defer fixture.close()

	runtime.GC()
	fixture.writableFS.metrics.reset()
	fixture.archiveFS.metrics.reset()
	cacheBefore := fixture.store.CacheMetrics()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	promotionRun, err := runPromotionBenchmarkGroup(fixture.store, fixture.keys)
	elapsed := time.Since(started)
	runtime.ReadMemStats(&after)
	if err != nil {
		return promotionBenchmarkSample{}, err
	}
	if !promotionRun.complete {
		return promotionBenchmarkSample{}, fmt.Errorf(
			"promotion did not complete after %d conflicts",
			promotionRun.conflicts,
		)
	}
	if err := validatePromotionBenchmarkDistribution(promotionRun.promotions, promotionRun.stats, writableKeys); err != nil {
		return promotionBenchmarkSample{}, err
	}
	cacheAfter := fixture.store.CacheMetrics()
	return promotionBenchmarkSample{
		promote:        elapsed,
		allocs:         after.Mallocs - before.Mallocs,
		allocatedBytes: after.TotalAlloc - before.TotalAlloc,
		archiveReads:   fixture.archiveFS.metrics.snapshot(),
		writableReads:  fixture.writableFS.metrics.snapshot(),
		cacheHits:      uint64(nonNegativePromotionBenchmarkDelta(cacheAfter.Hits - cacheBefore.Hits)),
		cacheMisses:    uint64(nonNegativePromotionBenchmarkDelta(cacheAfter.Misses - cacheBefore.Misses)),
		optional:       promotionRun.optional,
	}, nil
}

type promotionBenchmarkOverlapSample struct {
	promote       time.Duration
	foregroundPut time.Duration
	overlapped    bool
	conflicts     int
	complete      bool
}

type promotionBenchmarkResult struct {
	started  time.Time
	finished time.Time
	duration time.Duration
	run      promotionBenchmarkPromotionRun
	err      error
}

func runPromotionBenchmarkOverlapSample(group, writableKeys int, delay time.Duration) (promotionBenchmarkOverlapSample, error) {
	fixture, err := openPromotionBenchmarkFixture(group, writableKeys, delay)
	if err != nil {
		return promotionBenchmarkOverlapSample{}, err
	}
	defer fixture.close()

	fixture.writableFS.metrics.reset()
	fixture.archiveFS.metrics.reset()
	barrier := newPromotionBenchmarkReadBarrier()
	fixture.writableFS.barrier.Store(barrier)
	fixture.archiveFS.barrier.Store(barrier)
	resultChannel := make(chan promotionBenchmarkResult, 1)
	promotionStarted := make(chan struct{})
	go func() {
		started := time.Now()
		close(promotionStarted)
		promotionRun, err := runPromotionBenchmarkGroup(fixture.store, fixture.keys)
		if err == nil && promotionRun.complete {
			err = validatePromotionBenchmarkDistribution(promotionRun.promotions, promotionRun.stats, writableKeys)
		}
		finished := time.Now()
		resultChannel <- promotionBenchmarkResult{
			started:  started,
			finished: finished,
			duration: finished.Sub(started),
			run:      promotionRun,
			err:      err,
		}
	}()

	<-promotionStarted
	var result *promotionBenchmarkResult
	select {
	case <-barrier.read:
	case completed := <-resultChannel:
		result = &completed
	case <-time.After(promotionBenchmarkOverlapWait):
	}

	started := time.Now()
	foregroundErr := fixture.store.Put(
		[]byte(fmt.Sprintf("issue-1866/foreground/%08d", group)),
		[]byte("foreground-value"),
	)
	foregroundDuration := time.Since(started)
	if foregroundErr != nil {
		return promotionBenchmarkOverlapSample{}, foregroundErr
	}
	if result == nil {
		completed := <-resultChannel
		result = &completed
	}
	if result.err != nil && !isPromotionBenchmarkConflict(result.err) {
		return promotionBenchmarkOverlapSample{}, result.err
	}
	return promotionBenchmarkOverlapSample{
		promote:       result.duration,
		foregroundPut: foregroundDuration,
		overlapped:    !started.Before(result.started) && started.Before(result.finished),
		conflicts:     result.run.conflicts,
		complete:      result.run.complete,
	}, nil
}

func isPromotionBenchmarkConflict(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "promotion conflict")
}

func validatePromotionBenchmarkDistribution(promotions []kvstore.Promotion, stats kvstore.PromotionStats, writableKeys int) error {
	if len(promotions) != promotionBenchmarkKeyCount {
		return fmt.Errorf("promotion count = %d, want %d", len(promotions), promotionBenchmarkKeyCount)
	}
	wantArchive := promotionBenchmarkKeyCount - writableKeys
	if stats.Requested != promotionBenchmarkKeyCount ||
		stats.Consumed != promotionBenchmarkKeyCount ||
		stats.WritableHits != writableKeys ||
		stats.ArchiveHits != wantArchive ||
		stats.Promoted != wantArchive ||
		stats.PromotedBytes != wantArchive*promotionBenchmarkValueBytes {
		return fmt.Errorf("promotion distribution = %+v, want writable=%d archive=%d", stats, writableKeys, wantArchive)
	}
	return nil
}

type promotionBenchmarkSummary struct {
	p50, p95, p99       time.Duration
	allocsPerOp         float64
	allocatedBytesPerOp float64
	archiveReads        float64
	archiveBytes        float64
	writableReads       float64
	writableBytes       float64
	cacheHits           float64
	cacheMisses         float64
}

func summarizePromotionBenchmarkSamples(samples []promotionBenchmarkSample) promotionBenchmarkSummary {
	durations := make([]time.Duration, len(samples))
	for i, sample := range samples {
		durations[i] = sample.promote
	}
	var summary promotionBenchmarkSummary
	summary.p50 = promotionBenchmarkPercentile(durations, 0.50)
	summary.p95 = promotionBenchmarkPercentile(durations, 0.95)
	summary.p99 = promotionBenchmarkPercentile(durations, 0.99)
	for _, sample := range samples {
		summary.allocsPerOp += float64(sample.allocs)
		summary.allocatedBytesPerOp += float64(sample.allocatedBytes)
		summary.archiveReads += float64(sample.archiveReads.readCalls)
		summary.archiveBytes += float64(sample.archiveReads.readBytes)
		summary.writableReads += float64(sample.writableReads.readCalls)
		summary.writableBytes += float64(sample.writableReads.readBytes)
		summary.cacheHits += float64(sample.cacheHits)
		summary.cacheMisses += float64(sample.cacheMisses)
	}
	count := float64(len(samples))
	summary.allocsPerOp /= count
	summary.allocatedBytesPerOp /= count
	summary.archiveReads /= count
	summary.archiveBytes /= count
	summary.writableReads /= count
	summary.writableBytes /= count
	summary.cacheHits /= count
	summary.cacheMisses /= count
	return summary
}

func nonNegativePromotionBenchmarkDelta(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

type promotionBenchmarkOverlapSummary struct {
	promoteP50, promoteP95, promoteP99          time.Duration
	foregroundP50, foregroundP95, foregroundP99 time.Duration
	overlappedSamples                           int
	completeSamples                             int
	conflictsPerOp                              float64
}

func summarizePromotionBenchmarkOverlaps(samples []promotionBenchmarkOverlapSample) promotionBenchmarkOverlapSummary {
	promotions := make([]time.Duration, 0, len(samples))
	foreground := make([]time.Duration, 0, len(samples))
	var summary promotionBenchmarkOverlapSummary
	for _, sample := range samples {
		if sample.complete {
			summary.completeSamples++
		}
		if sample.overlapped {
			foreground = append(foreground, sample.foregroundPut)
			if sample.complete {
				promotions = append(promotions, sample.promote)
			}
			summary.overlappedSamples++
		}
		summary.conflictsPerOp += float64(sample.conflicts)
	}
	summary.promoteP50 = promotionBenchmarkPercentile(promotions, 0.50)
	summary.promoteP95 = promotionBenchmarkPercentile(promotions, 0.95)
	summary.promoteP99 = promotionBenchmarkPercentile(promotions, 0.99)
	summary.foregroundP50 = promotionBenchmarkPercentile(foreground, 0.50)
	summary.foregroundP95 = promotionBenchmarkPercentile(foreground, 0.95)
	summary.foregroundP99 = promotionBenchmarkPercentile(foreground, 0.99)
	if len(samples) > 0 {
		summary.conflictsPerOp /= float64(len(samples))
	}
	return summary
}

func promotionBenchmarkPercentile(values []time.Duration, percentile float64) time.Duration {
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	if len(ordered) == 0 {
		return 0
	}
	index := int(math.Ceil(percentile*float64(len(ordered)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(ordered) {
		index = len(ordered) - 1
	}
	return ordered[index]
}

type promotionBenchmarkOptionalStats struct {
	archiveLookups        uint64
	archiveLookupsAvoided uint64
	prefetchBytes         uint64
	present               bool
}

func readOptionalPromotionStats(stats kvstore.PromotionStats) promotionBenchmarkOptionalStats {
	value := reflect.ValueOf(stats)
	result := promotionBenchmarkOptionalStats{}
	var present bool
	result.archiveLookups, present = readOptionalPromotionStat(value, "ArchiveLookups")
	result.present = result.present || present
	result.archiveLookupsAvoided, present = readOptionalPromotionStat(value, "ArchiveLookupsAvoided")
	result.present = result.present || present
	result.prefetchBytes, present = readOptionalPromotionStat(value, "PrefetchBytes")
	result.present = result.present || present
	return result
}

func readOptionalPromotionStat(value reflect.Value, name string) (uint64, bool) {
	field := value.FieldByName(name)
	if !field.IsValid() {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Int, reflect.Int64:
		if field.Int() >= 0 {
			return uint64(field.Int()), true
		}
	}
	return 0, false
}

func (s *promotionBenchmarkOptionalStats) add(other promotionBenchmarkOptionalStats) {
	s.archiveLookups += other.archiveLookups
	s.archiveLookupsAvoided += other.archiveLookupsAvoided
	s.prefetchBytes += other.prefetchBytes
	s.present = s.present || other.present
}

func summarizeOptionalPromotionStats(samples []promotionBenchmarkSample) promotionBenchmarkOptionalStats {
	var summary promotionBenchmarkOptionalStats
	for _, sample := range samples {
		if sample.optional.present {
			summary.present = true
		}
		summary.archiveLookups += sample.optional.archiveLookups
		summary.archiveLookupsAvoided += sample.optional.archiveLookupsAvoided
		summary.prefetchBytes += sample.optional.prefetchBytes
	}
	if len(samples) > 0 {
		summary.archiveLookups /= uint64(len(samples))
		summary.archiveLookupsAvoided /= uint64(len(samples))
		summary.prefetchBytes /= uint64(len(samples))
	}
	return summary
}

func formatPromotionBenchmarkReportLine(
	profile promotionBenchmarkProfile,
	hitCase promotionBenchmarkHitCase,
	iterations int,
	promote promotionBenchmarkSummary,
	overlap promotionBenchmarkOverlapSummary,
	optional promotionBenchmarkOptionalStats,
) string {
	optionalValues := []string{"-", "-", "-"}
	if optional.present {
		optionalValues = []string{
			strconv.FormatUint(optional.archiveLookups, 10),
			strconv.FormatUint(optional.archiveLookupsAvoided, 10),
			strconv.FormatUint(optional.prefetchBytes, 10),
		}
	}
	values := []string{
		profile.name,
		"retained",
		profile.delay.String(),
		hitCase.name + "%",
		strconv.Itoa(iterations),
		strconv.FormatInt(promote.p50.Nanoseconds(), 10),
		strconv.FormatInt(promote.p95.Nanoseconds(), 10),
		strconv.FormatInt(promote.p99.Nanoseconds(), 10),
		fmt.Sprintf("%.2f", promote.allocsPerOp),
		fmt.Sprintf("%.2f", promote.allocatedBytesPerOp),
		fmt.Sprintf("%.2f", promote.archiveReads),
		fmt.Sprintf("%.2f", promote.archiveBytes),
		fmt.Sprintf("%.2f", promote.writableReads),
		fmt.Sprintf("%.2f", promote.writableBytes),
		fmt.Sprintf("%.2f", promote.cacheMisses),
		fmt.Sprintf("%.2f", promote.cacheHits),
		strconv.FormatInt(overlap.promoteP50.Nanoseconds(), 10),
		strconv.FormatInt(overlap.promoteP95.Nanoseconds(), 10),
		strconv.FormatInt(overlap.promoteP99.Nanoseconds(), 10),
		strconv.FormatInt(overlap.foregroundP50.Nanoseconds(), 10),
		strconv.FormatInt(overlap.foregroundP95.Nanoseconds(), 10),
		strconv.FormatInt(overlap.foregroundP99.Nanoseconds(), 10),
		strconv.Itoa(overlap.overlappedSamples),
		strconv.Itoa(overlap.completeSamples),
		fmt.Sprintf("%.2f", overlap.conflictsPerOp),
	}
	values = append(values, optionalValues...)
	return strings.Join(values, "\t")
}

func newPromotionBenchmarkReport() (*os.File, string, error) {
	path := os.Getenv("GOXRPL_PROMOTION_BENCH_REPORT")
	if path == "" {
		root := os.TempDir()
		if err := os.MkdirAll(root, 0o755); err != nil {
			return nil, "", fmt.Errorf("create benchmark report directory: %w", err)
		}
		file, err := os.CreateTemp(root, "issue-1866-promotion-report-*.tsv")
		if err != nil {
			return nil, "", fmt.Errorf("create benchmark report: %w", err)
		}
		return file, file.Name(), nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, "", fmt.Errorf("open benchmark report %s: %w", path, err)
	}
	return file, path, nil
}
