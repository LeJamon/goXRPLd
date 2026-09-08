package pebble

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cockroachpebble "github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
)

const (
	promotionBenchmarkKeys     = 256
	promotionBenchmarkBatch    = 32
	promotionBenchmarkValueLen = 1024
	promotionBenchmarkCache    = 128 << 10
	promotionBenchmarkPutCount = 64
	promotionBenchmarkMaxBytes = 4 << 20
	promotionBenchmarkDelay    = 50 * time.Microsecond
)

type promotionReadCounters struct {
	calls atomic.Uint64
	bytes atomic.Uint64
}

type promotionReadSnapshot struct {
	calls uint64
	bytes uint64
}

func (c *promotionReadCounters) snapshot() promotionReadSnapshot {
	return promotionReadSnapshot{
		calls: c.calls.Load(),
		bytes: c.bytes.Load(),
	}
}

func (c *promotionReadCounters) reset() {
	c.calls.Store(0)
	c.bytes.Store(0)
}

// promotionReadFS counts successful reads of persisted SSTables by generation.
// The counters are armed only after Pebble's asynchronous table-stat scan and
// any warm-up traversal, keeping setup work outside the timed operation.
type promotionReadFS struct {
	vfs.FS
	writablePath string
	archivePath  string
	writable     promotionReadCounters
	archive      promotionReadCounters
	delay        time.Duration
	armed        atomic.Bool
}

func (f *promotionReadFS) Open(name string, opts ...vfs.OpenOption) (vfs.File, error) {
	file, err := f.FS.Open(name, opts...)
	if err != nil {
		return nil, err
	}
	return f.wrapFile(name, file), nil
}

func (f *promotionReadFS) OpenReadWrite(name string, opts ...vfs.OpenOption) (vfs.File, error) {
	file, err := f.FS.OpenReadWrite(name, opts...)
	if err != nil {
		return nil, err
	}
	return f.wrapFile(name, file), nil
}

func (f *promotionReadFS) wrapFile(name string, file vfs.File) vfs.File {
	path := filepath.Clean(name)
	var counters *promotionReadCounters
	switch {
	case path == f.writablePath || strings.HasPrefix(path, f.writablePath+string(os.PathSeparator)):
		counters = &f.writable
	case path == f.archivePath || strings.HasPrefix(path, f.archivePath+string(os.PathSeparator)):
		counters = &f.archive
	}
	return &promotionReadFile{
		File:     file,
		counters: counters,
		sstable:  filepath.Ext(path) == ".sst",
		armed:    &f.armed,
		delay:    f.delay,
	}
}

type promotionReadFile struct {
	vfs.File
	counters *promotionReadCounters
	armed    *atomic.Bool
	sstable  bool
	delay    time.Duration
}

func (f *promotionReadFile) Read(p []byte) (int, error) {
	f.beforeRead()
	n, err := f.File.Read(p)
	f.afterRead(n)
	return n, err
}

func (f *promotionReadFile) ReadAt(p []byte, offset int64) (int, error) {
	f.beforeRead()
	n, err := f.File.ReadAt(p, offset)
	f.afterRead(n)
	return n, err
}

func (f *promotionReadFile) beforeRead() {
	if f.sstable && f.counters != nil && f.armed.Load() && f.delay > 0 {
		time.Sleep(f.delay)
	}
}

func (f *promotionReadFile) afterRead(n int) {
	if n > 0 && f.sstable && f.counters != nil && f.armed.Load() {
		f.counters.calls.Add(1)
		f.counters.bytes.Add(uint64(n))
	}
}

func (f *promotionReadFS) reset() {
	f.writable.reset()
	f.archive.reset()
}

func (f *promotionReadFS) snapshot() (promotionReadSnapshot, promotionReadSnapshot) {
	return f.writable.snapshot(), f.archive.snapshot()
}

type promotionReadFixture struct {
	path    string
	keys    [][]byte
	options Options
}

func BenchmarkPromotionReadAmplification(b *testing.B) {
	for _, writableHits := range []int{0, 50, 100} {
		for _, warm := range []bool{false, true} {
			for _, delayed := range []bool{false, true} {
				for _, workers := range []int{1, 2, 4} {
					name := fmt.Sprintf(
						"writable-hits=%d/warm=%t/delayed=%t/workers=%d",
						writableHits,
						warm,
						delayed,
						workers,
					)
					b.Run(name, func(b *testing.B) {
						benchmarkPromotionReadAmplification(b, writableHits, warm, delayed, workers)
					})
				}
			}
		}
	}
}

func benchmarkPromotionReadAmplification(
	b *testing.B,
	writableHits int,
	warm, delayed bool,
	workers int,
) {
	b.Helper()
	fixture := newPromotionReadFixture(b, writableHits)
	var (
		refreshWall  time.Duration
		workerBusy   time.Duration
		writableRead promotionReadSnapshot
		archiveRead  promotionReadSnapshot
		putLatencies []time.Duration
		putSamples   int
	)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		store, reads, err := openMeasuredPromotionStore(fixture.path, fixture.options, delayed)
		if err != nil {
			b.Fatal(err)
		}
		if warm {
			if err := warmPromotionStore(store, fixture.keys); err != nil {
				reads.armed.Store(false)
				_ = store.Close()
				b.Fatal(err)
			}
		}
		reads.reset()
		reads.armed.Store(true)
		b.StartTimer()
		run := runPromotionWorkers(store, fixture.keys, workers)
		b.StopTimer()
		reads.armed.Store(false)
		writable, archive := reads.snapshot()
		closeErr := store.Close()
		if run.err != nil {
			b.Fatal(run.err)
		}
		if closeErr != nil {
			b.Fatal(closeErr)
		}
		refreshWall += run.wall
		workerBusy += run.busy
		writableRead.calls += writable.calls
		writableRead.bytes += writable.bytes
		archiveRead.calls += archive.calls
		archiveRead.bytes += archive.bytes
		putLatencies = append(putLatencies, run.putLatencies...)
		putSamples += len(run.putLatencies)
	}

	perOp := float64(b.N)
	b.ReportMetric(float64(refreshWall)/perOp/float64(time.Millisecond), "refresh-ms/op")
	b.ReportMetric(float64(writableRead.calls)/perOp, "writable-sst-reads/op")
	b.ReportMetric(float64(archiveRead.calls)/perOp, "archive-sst-reads/op")
	b.ReportMetric(float64(writableRead.calls+archiveRead.calls)/perOp, "sst-reads/op")
	b.ReportMetric(float64(writableRead.bytes)/perOp, "writable-sst-bytes/op")
	b.ReportMetric(float64(archiveRead.bytes)/perOp, "archive-sst-bytes/op")
	b.ReportMetric(float64(writableRead.bytes+archiveRead.bytes)/perOp, "sst-bytes/op")
	workerUtilization := 0.0
	if refreshWall > 0 {
		workerUtilization = float64(workerBusy) / float64(refreshWall) / float64(workers)
	}
	b.ReportMetric(workerUtilization, "worker-utilization")
	b.ReportMetric(float64(putSamples)/perOp, "overlap-put-samples/op")
	b.ReportMetric(percentileNanos(putLatencies, 0.50), "overlap-put-p50-ns")
	b.ReportMetric(percentileNanos(putLatencies, 0.95), "overlap-put-p95-ns")
	b.ReportMetric(percentileNanos(putLatencies, 0.99), "overlap-put-p99-ns")
}

func newPromotionReadFixture(b *testing.B, writableHits int) promotionReadFixture {
	b.Helper()
	options := Options{
		BlockCacheBytes: promotionBenchmarkCache,
		MaxOpenFiles:    200,
	}
	fixture := promotionReadFixture{
		path:    filepath.Join(b.TempDir(), "nodes"),
		options: options,
		keys:    makePromotionBenchmarkKeys(promotionBenchmarkKeys),
	}
	store, err := NewRotating(fixture.path, options)
	if err != nil {
		b.Fatal(err)
	}
	for index, key := range fixture.keys {
		if err := store.Put(key, promotionBenchmarkValue(index, 0)); err != nil {
			_ = store.Close()
			b.Fatal(err)
		}
	}
	if err := store.Sync(); err != nil {
		_ = store.Close()
		b.Fatal(err)
	}
	committed, err := store.Rotate(1, 1)
	if err != nil || !committed {
		_ = store.Close()
		if err == nil {
			err = fmt.Errorf("initial fixture rotation did not commit")
		}
		b.Fatal(err)
	}
	hitCount := len(fixture.keys) * writableHits / 100
	for index := 0; index < hitCount; index++ {
		if err := store.Put(fixture.keys[index], promotionBenchmarkValue(index, 1)); err != nil {
			_ = store.Close()
			b.Fatal(err)
		}
	}
	if err := store.Sync(); err != nil {
		_ = store.Close()
		b.Fatal(err)
	}
	if err := store.Close(); err != nil {
		b.Fatal(err)
	}
	return fixture
}

func makePromotionBenchmarkKeys(count int) [][]byte {
	keys := make([][]byte, count)
	for index := range keys {
		var input [16]byte
		binary.BigEndian.PutUint64(input[:8], uint64(index))
		binary.BigEndian.PutUint64(input[8:], 0x70726f6d6f74696f)
		hash := sha256.Sum256(input[:])
		keys[index] = append([]byte(nil), hash[:]...)
	}
	return keys
}

func promotionBenchmarkValue(index, generation int) []byte {
	value := make([]byte, promotionBenchmarkValueLen)
	for offset := 0; offset < len(value); offset += sha256.Size {
		var input [24]byte
		binary.BigEndian.PutUint64(input[:8], uint64(index))
		binary.BigEndian.PutUint64(input[8:16], uint64(generation))
		binary.BigEndian.PutUint64(input[16:], uint64(offset))
		hash := sha256.Sum256(input[:])
		copy(value[offset:], hash[:])
	}
	return value
}

func openMeasuredPromotionStore(
	path string,
	options Options,
	delayed bool,
) (*RotatingStore, *promotionReadFS, error) {
	resolved, perGeneration, err := resolveRotatingOptions(options)
	if err != nil {
		return nil, nil, err
	}
	store, found, err := prepareRotatingStore(path, perGeneration)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, fmt.Errorf("promotion benchmark fixture has no rotation manifest")
	}
	reads := &promotionReadFS{
		FS:           vfs.Default,
		writablePath: store.writablePath,
		archivePath:  store.archivePath,
	}
	if delayed {
		reads.delay = promotionBenchmarkDelay
	}
	loaded := make(chan struct{}, 2)
	openGeneration := func(path string, cache *cockroachpebble.Cache, options Options) (*Store, error) {
		pebbleOptions := makePebbleOptions(options, cache)
		pebbleOptions.FS = reads
		pebbleOptions.DisableAutomaticCompactions = true
		pebbleOptions.EventListener = &cockroachpebble.EventListener{
			TableStatsLoaded: func(cockroachpebble.TableStatsInfo) { loaded <- struct{}{} },
		}
		db, err := cockroachpebble.Open(path, pebbleOptions)
		if err != nil {
			return nil, err
		}
		return &Store{db: db}, nil
	}
	if err := store.openGenerations(resolved.BlockCacheBytes, true, openGeneration); err != nil {
		return nil, nil, err
	}
	for range 2 {
		select {
		case <-loaded:
		case <-time.After(10 * time.Second):
			_ = store.Close()
			return nil, nil, fmt.Errorf("timed out waiting for Pebble table stats")
		}
	}
	return store, reads, nil
}

func warmPromotionStore(store *RotatingStore, keys [][]byte) error {
	sorted := append([][]byte(nil), keys...)
	sort.Slice(sorted, func(i, j int) bool { return string(sorted[i]) < string(sorted[j]) })
	for _, generation := range []*Store{store.writable, store.archive} {
		iterator, err := generation.newPointIterator()
		if err != nil {
			return err
		}
		for _, key := range sorted {
			if _, _, _, err := iterator.get(key, int(^uint(0)>>1), true); err != nil {
				_ = iterator.Close()
				return err
			}
		}
		if err := iterator.Close(); err != nil {
			return err
		}
	}
	return nil
}

type promotionRunResult struct {
	err          error
	wall         time.Duration
	busy         time.Duration
	putLatencies []time.Duration
}

func runPromotionWorkers(store *RotatingStore, keys [][]byte, workers int) promotionRunResult {
	batches := make([][][]byte, 0, (len(keys)+promotionBenchmarkBatch-1)/promotionBenchmarkBatch)
	for start := 0; start < len(keys); start += promotionBenchmarkBatch {
		end := min(start+promotionBenchmarkBatch, len(keys))
		batches = append(batches, keys[start:end])
	}
	start := make(chan struct{})
	refreshDone := make(chan struct{})
	errors := make(chan error, workers+1)
	var group sync.WaitGroup
	var busy atomic.Int64
	group.Add(workers)
	for worker := range workers {
		go func(worker int) {
			defer group.Done()
			<-start
			started := time.Now()
			for batchIndex := worker; batchIndex < len(batches); batchIndex += workers {
				if _, _, err := store.PromoteBatch(batches[batchIndex], promotionBenchmarkMaxBytes); err != nil {
					errors <- err
					return
				}
			}
			busy.Add(int64(time.Since(started)))
		}(worker)
	}
	putLatencies := make(chan []time.Duration, 1)
	go func() {
		<-start
		latencies := make([]time.Duration, 0, promotionBenchmarkPutCount)
		for index := range promotionBenchmarkPutCount {
			select {
			case <-refreshDone:
				putLatencies <- latencies
				return
			default:
			}
			started := time.Now()
			if err := store.Put(promotionBenchmarkPutKey(index), promotionBenchmarkPutValue(index)); err != nil {
				errors <- err
				putLatencies <- latencies
				return
			}
			latencies = append(latencies, time.Since(started))
		}
		putLatencies <- latencies
	}()

	started := time.Now()
	close(start)
	group.Wait()
	close(refreshDone)
	latencies := <-putLatencies
	result := promotionRunResult{
		wall:         time.Since(started),
		busy:         time.Duration(busy.Load()),
		putLatencies: latencies,
	}
	select {
	case result.err = <-errors:
	default:
	}
	return result
}

func promotionBenchmarkPutKey(index int) []byte {
	var input [16]byte
	binary.BigEndian.PutUint64(input[:8], uint64(index))
	binary.BigEndian.PutUint64(input[8:], 0x7075742d6f766572)
	hash := sha256.Sum256(input[:])
	return hash[:]
}

func promotionBenchmarkPutValue(index int) []byte {
	return promotionBenchmarkValue(index+promotionBenchmarkKeys, 2)[:64]
}

func percentileNanos(values []time.Duration, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(math.Ceil(float64(len(sorted))*percentile)) - 1
	index = max(index, 0)
	index = min(index, len(sorted)-1)
	return float64(sorted[index].Nanoseconds())
}
