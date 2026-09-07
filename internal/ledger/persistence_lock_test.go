package ledger

import (
	"testing"
	"time"

	"github.com/LeJamon/go-xrpl/shamap"
)

func TestPersistenceDoesNotBlockHeaderReads(t *testing.T) {
	for _, txMap := range []bool{false, true} {
		t.Run(map[bool]string{false: "state", true: "transaction"}[txMap], func(t *testing.T) {
			m := shamap.New(shamap.TypeState)
			var key [32]byte
			key[0] = 1
			if err := m.Put(key, []byte("test state value")); err != nil {
				t.Fatal(err)
			}
			l := &Ledger{stateMap: m, txMap: m}
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			store := func([]shamap.FlushEntry) error { close(entered); <-release; return nil }
			go func() {
				if txMap {
					done <- l.StoreTransactionDirty(store)
				} else {
					done <- l.StoreStateDirty(store)
				}
			}()
			<-entered
			read := make(chan struct{})
			go func() { _ = l.Hash(); _ = l.Sequence(); close(read) }()
			select {
			case <-read:
			case <-time.After(time.Second):
				close(release)
				<-done
				t.Fatal("header read blocked by storage callback")
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
