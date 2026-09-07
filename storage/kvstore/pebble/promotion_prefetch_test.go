package pebble

import (
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
	defer release.Do(func() { close(block.release) })
	cache := p.NewCache(1 << 20)
	defer cache.Unref()
	writable, err := p.Open("writable", &p.Options{FS: errorfs.Wrap(fs, block), Cache: cache})
	require.NoError(t, err)
	archive, err := p.Open("archive", &p.Options{FS: fs, Cache: cache})
	require.NoError(t, err)
	store := &RotatingStore{writable: &Store{db: writable}, archive: &Store{db: archive}, blockCache: cache}
	defer writable.Close()
	defer archive.Close()
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
}
