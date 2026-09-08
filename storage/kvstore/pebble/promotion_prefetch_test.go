package pebble

import (
	"errors"
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
