package pebble

import (
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

type promotionReadGate struct {
	armed       atomic.Bool
	entered     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func newPromotionReadGate() *promotionReadGate {
	return &promotionReadGate{entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *promotionReadGate) arm() { g.armed.Store(true) }

func (g *promotionReadGate) unblock() {
	g.releaseOnce.Do(func() { close(g.release) })
}

type stagedPromotionRead struct {
	gates []*promotionReadGate
}

func (s *stagedPromotionRead) MaybeError(op errorfs.Op, path string) error {
	if filepath.Ext(path) != ".sst" || (op != errorfs.OpFileRead && op != errorfs.OpFileReadAt) {
		return nil
	}
	for _, gate := range s.gates {
		if gate.armed.CompareAndSwap(true, false) {
			close(gate.entered)
			<-gate.release
			// Leave the block uncached so retries still exercise I/O.
			return errorfs.ErrInjected
		}
	}
	return nil
}

func (b *blockingPromotionRead) MaybeError(op errorfs.Op, path string) error {
	if b.armed.Load() && filepath.Ext(path) == ".sst" && (op == errorfs.OpFileRead || op == errorfs.OpFileReadAt) {
		b.once.Do(func() { close(b.entered); <-b.release })
	}
	return nil
}

func TestPromotionColdWritableReadDoesNotBlockPut(t *testing.T) {
	for _, batchWrite := range []bool{false, true} {
		t.Run(fmt.Sprintf("batch=%t", batchWrite), func(t *testing.T) {
			testPromotionColdWritableRead(t, batchWrite)
		})
	}
}

func testPromotionColdWritableRead(t *testing.T, batchWrite bool) {
	t.Helper()
	block := &blockingPromotionRead{entered: make(chan struct{}), release: make(chan struct{})}
	store := openPromotionPrefetchStore(t, block)
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(block.release) }) })
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
	go func() {
		if !batchWrite {
			put <- store.Put([]byte("key"), []byte("new"))
			return
		}
		batch, err := store.NewBatch()
		if err != nil {
			put <- err
			return
		}
		defer batch.Close()
		if err := batch.Put([]byte("key"), []byte("new")); err != nil {
			put <- err
			return
		}
		put <- batch.Write()
	}()
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
}

func openPromotionPrefetchStore(t *testing.T, injector errorfs.Injector) *RotatingStore {
	t.Helper()
	fs := vfs.NewMem()
	require.NoError(t, fs.MkdirAll("writable", 0755))
	require.NoError(t, fs.MkdirAll("archive", 0755))
	db, err := p.Open("writable", &p.Options{FS: fs})
	require.NoError(t, err)
	require.NoError(t, db.Set([]byte("key"), []byte("old"), p.NoSync))
	require.NoError(t, db.Flush())
	require.NoError(t, db.Close())
	cache := p.NewCache(0)
	t.Cleanup(cache.Unref)
	statsLoaded := make(chan struct{})
	writable, err := p.Open("writable", &p.Options{
		FS: errorfs.Wrap(fs, injector), Cache: cache, DisableAutomaticCompactions: true,
		EventListener: &p.EventListener{TableStatsLoaded: func(p.TableStatsInfo) { close(statsLoaded) }},
	})
	require.NoError(t, err)
	archive, err := p.Open("archive", &p.Options{FS: fs, Cache: cache})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, writable.Close())
		require.NoError(t, archive.Close())
	})
	select {
	case <-statsLoaded:
	case <-time.After(5 * time.Second):
		t.Fatal("initial table statistics did not finish loading")
	}
	return &RotatingStore{
		writable:   &Store{db: writable},
		archive:    &Store{db: archive},
		blockCache: cache,
	}
}

func TestPromotionSecondPrefetchReadStillAllowsPut(t *testing.T) {
	first := newPromotionReadGate()
	second := newPromotionReadGate()
	injector := &stagedPromotionRead{gates: []*promotionReadGate{first, second}}
	store := openPromotionPrefetchStore(t, injector)
	t.Cleanup(func() {
		for _, gate := range injector.gates {
			gate.unblock()
		}
	})
	first.arm()
	type result struct {
		records []kvstore.Promotion
		err     error
	}
	done := make(chan result, 1)
	go func() {
		records, _, err := store.PromoteBatch([][]byte{[]byte("key")}, 1024)
		done <- result{records: records, err: err}
	}()
	select {
	case <-first.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("did not reach first cold writable read")
	}
	require.NoError(t, store.Put([]byte("key"), []byte("new")))
	require.NoError(t, store.writable.db.Flush())
	second.arm()
	first.unblock()
	select {
	case <-second.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("did not reach second cold writable read")
	}
	put := make(chan error, 1)
	go func() { put <- store.Put([]byte("key"), []byte("newer")) }()
	select {
	case err := <-put:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Put blocked during retry read")
	}
	second.unblock()
	got := <-done
	require.NoError(t, got.err)
	require.Len(t, got.records, 1)
	require.Equal(t, []byte("newer"), got.records[0].Value)
}

func TestPromotionConflictReturnsAfterBoundedRetries(t *testing.T) {
	var store *RotatingStore
	var armed atomic.Bool
	var reads atomic.Int32
	key := []byte("key")
	var collision []byte
	for i := 0; ; i++ {
		collision = []byte(fmt.Sprintf("other-%d", i))
		if mutationStripe(collision) == mutationStripe(key) {
			break
		}
	}
	putErrors := make(chan error, 1)
	injector := errorfs.InjectorFunc(func(op errorfs.Op, path string) error {
		if !armed.Load() || filepath.Ext(path) != ".sst" || (op != errorfs.OpFileRead && op != errorfs.OpFileReadAt) {
			return nil
		}
		reads.Add(1)
		if err := store.Put(collision, []byte("concurrent")); err != nil {
			select {
			case putErrors <- err:
			default:
			}
		}
		return errorfs.ErrInjected
	})
	store = openPromotionPrefetchStore(t, injector)
	armed.Store(true)
	type result struct {
		records []kvstore.Promotion
		stats   kvstore.PromotionStats
		err     error
	}
	done := make(chan result, 1)
	go func() {
		records, stats, err := store.PromoteBatch([][]byte{key}, 1024)
		done <- result{records, stats, err}
	}()
	select {
	case got := <-done:
		require.ErrorIs(t, got.err, ErrPromotionConflict)
		require.Nil(t, got.records)
		require.Zero(t, got.stats.Consumed)
		require.Zero(t, got.stats.Promoted)
		require.Zero(t, got.stats.PromotedBytes)
		require.Zero(t, got.stats.Batches)
	case <-time.After(5 * time.Second):
		t.Fatal("promotion did not stop after bounded retries")
	}
	armed.Store(false)
	select {
	case err := <-putErrors:
		t.Fatal(err)
	default:
	}
	require.GreaterOrEqual(t, reads.Load(), int32(promotionPrefetchPasses))
	value, err := store.Get(key)
	require.NoError(t, err)
	require.Equal(t, []byte("old"), value)
	value, err = store.Get(collision)
	require.NoError(t, err)
	require.Equal(t, []byte("concurrent"), value)
}
