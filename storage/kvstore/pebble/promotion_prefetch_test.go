package pebble

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LeJamon/go-xrpl/storage/kvstore"
	p "github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	"github.com/cockroachdb/pebble/vfs/errorfs"
	"github.com/stretchr/testify/require"
)

type blockingPromotionRead struct {
	armed   atomic.Bool
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (b *blockingPromotionRead) MaybeError(op errorfs.Op, path string) error {
	if b.armed.Load() && filepath.Ext(path) == ".sst" && (op == errorfs.OpFileRead || op == errorfs.OpFileReadAt) {
		b.once.Do(func() { close(b.entered); <-b.release })
	}
	return nil
}

func TestPromotionColdWritableReadDoesNotBlockPut(t *testing.T) {
	fs := vfs.NewMem()
	require.NoError(t, fs.MkdirAll("writable", 0755))
	require.NoError(t, fs.MkdirAll("archive", 0755))
	db, err := p.Open("writable", &p.Options{FS: fs})
	require.NoError(t, err)
	require.NoError(t, db.Set([]byte("key"), []byte("old"), p.NoSync))
	require.NoError(t, db.Flush())
	require.NoError(t, db.Close())
	block := &blockingPromotionRead{entered: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once

	cache := p.NewCache(1 << 20)
	defer cache.Unref()
	writableLoaded := make(chan struct{})
	writable, err := p.Open("writable", &p.Options{
		FS: errorfs.Wrap(fs, block), Cache: cache,
		EventListener: &p.EventListener{TableStatsLoaded: func(p.TableStatsInfo) { close(writableLoaded) }},
	})
	require.NoError(t, err)
	archive, err := p.Open("archive", &p.Options{FS: fs, Cache: cache})
	require.NoError(t, err)
	require.NoError(t, archive.Set([]byte("key"), []byte("archive"), p.NoSync))
	require.NoError(t, archive.Flush())
	require.NoError(t, archive.Close())
	archiveReads := &rejectPromotionReads{}
	archiveLoaded := make(chan struct{})
	archive, err = p.Open("archive", &p.Options{
		FS: errorfs.Wrap(fs, archiveReads), Cache: cache,
		EventListener: &p.EventListener{TableStatsLoaded: func(p.TableStatsInfo) { close(archiveLoaded) }},
	})
	require.NoError(t, err)
	waitPromotionTableStats(t, archiveLoaded)
	archiveReads.armed.Store(true)
	store := &RotatingStore{writable: &Store{db: writable}, archive: &Store{db: archive}, blockCache: cache}
	defer writable.Close()
	defer archive.Close()
	defer release.Do(func() { close(block.release) })
	waitPromotionTableStats(t, writableLoaded)
	block.armed.Store(true)
	type result struct {
		records []kvstore.Promotion
		err     error
	}
	done := make(chan result, 1)
	go func() {
		records, _, err := store.PromoteBatch([][]byte{[]byte("key")}, 1024)
		done <- result{records, err}
	}()
	select {
	case <-block.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("did not reach cold writable read")
	}
	put := make(chan error, 1)
	go func() { put <- store.Put([]byte("key"), []byte("new")) }()
	select {
	case err := <-put:
		require.NoError(t, err)
	case <-time.After(time.Second):
		release.Do(func() { close(block.release) })
		<-done
		t.Fatal("foreground Put blocked behind promotion disk read")
	}
	release.Do(func() { close(block.release) })
	got := <-done
	require.NoError(t, got.err)
	require.Len(t, got.records, 1)
	require.Equal(t, []byte("new"), got.records[0].Value, "must recheck writes made during prefetch")
	require.Zero(t, archiveReads.reads.Load(), "writable replacement must not require archive reads")
}

type rejectPromotionReads struct {
	armed atomic.Bool
	reads atomic.Int64
}

func (r *rejectPromotionReads) MaybeError(op errorfs.Op, path string) error {
	if r.armed.Load() && filepath.Ext(path) == ".sst" && (op == errorfs.OpFileRead || op == errorfs.OpFileReadAt) {
		r.reads.Add(1)
		return errors.New("unexpected archive SST read")
	}
	return nil
}

func TestPromotionWritableHitsDoNotReadArchive(t *testing.T) {
	fs := vfs.NewMem()
	for _, generation := range []string{"writable", "archive"} {
		db, err := p.Open(generation, &p.Options{FS: fs})
		require.NoError(t, err)
		require.NoError(t, db.Set([]byte("key"), []byte(generation), p.NoSync))
		require.NoError(t, db.Flush())
		require.NoError(t, db.Close())
	}
	archiveReads := &rejectPromotionReads{}
	writable, err := p.Open("writable", &p.Options{FS: fs})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, writable.Close()) })
	archiveLoaded := make(chan struct{})
	archive, err := p.Open("archive", &p.Options{
		FS:            errorfs.Wrap(fs, archiveReads),
		EventListener: &p.EventListener{TableStatsLoaded: func(p.TableStatsInfo) { close(archiveLoaded) }},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, archive.Close()) })
	store := &RotatingStore{writable: &Store{db: writable}, archive: &Store{db: archive}}
	waitPromotionTableStats(t, archiveLoaded)
	archiveReads.armed.Store(true)

	records, stats, err := store.PromoteBatch([][]byte{[]byte("key")}, 1024)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, []byte("writable"), records[0].Value)
	require.Zero(t, archiveReads.reads.Load())
	require.Equal(t, 1, stats.ArchiveLookupsAvoided)
	require.Zero(t, stats.ArchiveLookups)
}

func waitPromotionTableStats(t *testing.T, loaded <-chan struct{}) {
	t.Helper()
	select {
	case <-loaded:
	case <-time.After(5 * time.Second):
		t.Fatal("table statistics did not finish loading")
	}
}

type stagedPromotionRead struct {
	firstArmed    atomic.Bool
	secondArmed   atomic.Bool
	firstOnce     sync.Once
	secondOnce    sync.Once
	firstEntered  chan struct{}
	firstRelease  chan struct{}
	secondEntered chan struct{}
	secondRelease chan struct{}
}

func (b *stagedPromotionRead) MaybeError(op errorfs.Op, path string) error {
	if filepath.Ext(path) != ".sst" || (op != errorfs.OpFileRead && op != errorfs.OpFileReadAt) {
		return nil
	}
	if b.firstArmed.Load() {
		b.firstOnce.Do(func() {
			close(b.firstEntered)
			<-b.firstRelease
		})
		return nil
	}
	if b.secondArmed.Load() {
		b.secondOnce.Do(func() { close(b.secondEntered); <-b.secondRelease })
	}
	return nil
}

func TestPromotionChangedVersionRereadDoesNotBlockSelectedStripePut(t *testing.T) {
	block := &stagedPromotionRead{
		firstEntered:  make(chan struct{}),
		firstRelease:  make(chan struct{}),
		secondEntered: make(chan struct{}),
		secondRelease: make(chan struct{}),
	}
	store := newPromotionReadStore(t, block, []byte("key"), []byte("old"), 1)
	block.firstArmed.Store(true)
	t.Cleanup(func() {
		closePromotionGate(block.firstRelease)
		closePromotionGate(block.secondRelease)
	})
	stripe := mutationStripe([]byte("key"))
	require.Greater(t, stripe, 2)
	gate := promotionKeyOnStripe(0, "gate")
	selected := promotionKeyOnStripe(1, "selected")
	control := promotionKeyOnStripe(2, "control")
	collision1 := promotionKeyOnStripe(stripe, "first")
	collision2 := promotionKeyOnStripe(stripe, "second")
	gateHeld := false
	t.Cleanup(func() {
		if gateHeld {
			store.mutations[0].Unlock()
		}
	})
	type result struct {
		records []kvstore.Promotion
		stats   kvstore.PromotionStats
		err     error
	}
	done := make(chan result, 1)
	go func() {
		records, stats, err := store.PromoteBatch([][]byte{[]byte("key"), gate, selected}, 1024)
		done <- result{records, stats, err}
	}()
	waitPromotionSignal(t, block.firstEntered, "initial writable prefetch")
	store.mutations[0].Lock()
	gateHeld = true
	closePromotionGate(block.firstRelease)
	iteratorClosed := make(chan struct{})
	go func() {
		store.writable.mu.Lock()
		store.writable.mu.Unlock()
		close(iteratorClosed)
	}()
	waitPromotionSignal(t, iteratorClosed, "completed writable prefetch")
	put1 := make(chan error, 1)
	go func() { put1 <- store.Put(collision1, []byte("first")) }()
	require.NoError(t, waitPromotionResult(t, put1, "first colliding Put"))
	block.firstArmed.Store(false)
	block.secondArmed.Store(true)
	store.mutations[0].Unlock()
	gateHeld = false
	waitPromotionSignal(t, block.secondEntered, "changed-version writable reread")
	for _, test := range []struct {
		key  []byte
		name string
	}{
		{control, "unselected-stripe Put"},
		{collision2, "colliding-stripe Put"},
		{selected, "other selected-stripe Put"},
	} {
		put := make(chan error, 1)
		go func() { put <- store.Put(test.key, []byte("second")) }()
		require.NoError(t, waitPromotionResult(t, put, test.name))
	}
	closePromotionGate(block.secondRelease)
	got := waitPromotionResultValue(t, done, "promotion")
	require.NoError(t, got.err)
	require.Len(t, got.records, 3)
	require.Equal(t, []byte("key"), got.records[0].Key)
	require.Equal(t, []byte("old"), got.records[0].Value)
	require.GreaterOrEqual(t, got.stats.VersionMismatches, 1)
	require.GreaterOrEqual(t, got.stats.Retries, 1)
}

func newPromotionReadStore(
	t *testing.T,
	injector errorfs.Injector,
	key, value []byte,
	cacheBytes int64,
) *RotatingStore {
	t.Helper()
	fs := vfs.NewMem()
	require.NoError(t, fs.MkdirAll("writable", 0755))
	require.NoError(t, fs.MkdirAll("archive", 0755))
	db, err := p.Open("writable", &p.Options{FS: fs})
	require.NoError(t, err)
	require.NoError(t, db.Set(key, value, p.NoSync))
	require.NoError(t, db.Flush())
	require.NoError(t, db.Close())
	cache := p.NewCache(cacheBytes)
	writable, err := p.Open("writable", &p.Options{FS: errorfs.Wrap(fs, injector), Cache: cache})
	require.NoError(t, err)
	archive, err := p.Open("archive", &p.Options{FS: fs, Cache: cache})
	require.NoError(t, err)
	store := &RotatingStore{writable: &Store{db: writable}, archive: &Store{db: archive}, blockCache: cache}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close promotion store: %v", err)
		}
	})
	return store
}

func promotionKeyOnStripe(stripe int, label string) []byte {
	for index := 0; ; index++ {
		candidate := []byte(fmt.Sprintf("promotion/%s/%d", label, index))
		if mutationStripe(candidate) == stripe {
			return candidate
		}
	}
}

func closePromotionGate(channel chan struct{}) {
	select {
	case <-channel:
	default:
		close(channel)
	}
}

func waitPromotionSignal(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("did not reach %s", operation)
	}
}

func waitPromotionResult(t *testing.T, result <-chan error, operation string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not complete", operation)
		return nil
	}
}

func waitPromotionResultValue[T any](t *testing.T, result <-chan T, operation string) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(5 * time.Second):
		var zero T
		t.Fatalf("%s did not complete", operation)
		return zero
	}
}

type sequencedPromotionRead struct {
	active atomic.Pointer[blockingPromotionRead]
}

func (s *sequencedPromotionRead) MaybeError(op errorfs.Op, path string) error {
	if gate := s.active.Load(); gate != nil {
		return gate.MaybeError(op, path)
	}
	return nil
}

func TestPromotionRetryExhaustionHoldsOnlyCurrentStripe(t *testing.T) {
	injector := &sequencedPromotionRead{}
	store := newPromotionReadStore(t, injector, []byte("key"), []byte("old"), 1)
	stages := make([]*blockingPromotionRead, promotionRetryLimit+2)
	for index := range stages {
		stages[index] = &blockingPromotionRead{entered: make(chan struct{}), release: make(chan struct{})}
		stages[index].armed.Store(true)
	}
	gateKey := promotionKeyOnStripe(0, "gate")
	stripe := mutationStripe([]byte("key"))
	require.NotZero(t, stripe)
	collision := promotionKeyOnStripe(stripe, "collision")
	gateHeld := false
	t.Cleanup(func() {
		if gateHeld {
			store.mutations[0].Unlock()
		}
		for _, stage := range stages {
			closePromotionGate(stage.release)
		}
	})
	injector.active.Store(stages[0])
	type result struct {
		records []kvstore.Promotion
		stats   kvstore.PromotionStats
		err     error
	}
	done := make(chan result, 1)
	go func() {
		records, stats, err := store.PromoteBatch([][]byte{[]byte("key"), gateKey}, 1024)
		done <- result{records, stats, err}
	}()
	for attempt := 0; attempt <= promotionRetryLimit; attempt++ {
		waitPromotionSignal(t, stages[attempt].entered, "off-lock writable read")
		store.mutations[0].Lock()
		gateHeld = true
		closePromotionGate(stages[attempt].release)
		iteratorClosed := make(chan struct{})
		go func() {
			store.writable.mu.Lock()
			store.writable.mu.Unlock()
			close(iteratorClosed)
		}()
		waitPromotionSignal(t, iteratorClosed, "completed writable prefetch")
		put := make(chan error, 1)
		go func() { put <- store.Put(collision, []byte("changed")) }()
		require.NoError(t, waitPromotionResult(t, put, "colliding mutation"))
		injector.active.Store(stages[attempt+1])
		store.mutations[0].Unlock()
		gateHeld = false
	}
	waitPromotionSignal(t, stages[len(stages)-1].entered, "single-stripe fallback")
	if store.mutations[stripe].TryLock() {
		store.mutations[stripe].Unlock()
		t.Fatal("fallback did not hold the current key's stripe")
	}
	put := make(chan error, 1)
	go func() { put <- store.Put(gateKey, []byte("foreground")) }()
	require.NoError(t, waitPromotionResult(t, put, "other selected-stripe Put"))
	closePromotionGate(stages[len(stages)-1].release)
	got := waitPromotionResultValue(t, done, "bounded promotion")
	require.NoError(t, got.err)
	require.Len(t, got.records, 2)
	require.Equal(t, []byte("old"), got.records[0].Value)
	require.Equal(t, []byte("foreground"), got.records[1].Value)
	require.Equal(t, promotionRetryLimit, got.stats.Retries)
	require.Positive(t, got.stats.Fallbacks)
}
