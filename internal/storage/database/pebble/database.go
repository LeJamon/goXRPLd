package pebble

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/storage/database"
	"github.com/cockroachdb/pebble"
)

var (
	ErrDBClosed    = errors.New("database is closed")
	ErrKeyNotFound = errors.New("key not found")
)

type DB struct {
	db *pebble.DB
}

func NewDB(db *pebble.DB) *DB {
	return &DB{db: db}
}

func (p *DB) Read(ctx context.Context, key []byte) ([]byte, error) {
	if p.db == nil {
		return nil, ErrDBClosed
	}

	val, closer, err := p.db.Get(key)
	if err != nil {
		if errors.Is(err, pebble.ErrNotFound) {
			return nil, ErrKeyNotFound
		}
		return nil, err
	}
	defer closer.Close()

	// Copy the value out
	valCopy := make([]byte, len(val))
	copy(valCopy, val)
	return valCopy, nil
}

func (p *DB) Write(ctx context.Context, key, value []byte) error {
	if p.db == nil {
		return ErrDBClosed
	}
	return p.db.Set(key, value, pebble.Sync)
}

func (p *DB) Delete(ctx context.Context, key []byte) error {
	if p.db == nil {
		return ErrDBClosed
	}
	return p.db.Delete(key, pebble.Sync)
}

func (p *DB) Batch(ctx context.Context, ops []database.BatchOperation) error {
	if p.db == nil {
		return ErrDBClosed
	}

	batch := p.db.NewBatch()
	defer batch.Close()

	for _, op := range ops {
		switch op.Type {
		case database.BatchPut:
			if err := batch.Set(op.Key, op.Value, nil); err != nil {
				return err
			}
		case database.BatchDelete:
			if err := batch.Delete(op.Key, nil); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown batch operation type: %d", op.Type)
		}
	}

	return batch.Commit(pebble.Sync)
}

type Iterator struct {
	iter *pebble.Iterator
	db   *pebble.DB

	start, end []byte
	err        error
	current    struct {
		key, value []byte
	}
}

func (p *DB) Iterator(ctx context.Context, start, end []byte) (database.Iterator, error) {
	if p.db == nil {
		return nil, ErrDBClosed
	}

	iter, _ := p.db.NewIter(&pebble.IterOptions{
		LowerBound: start,
		UpperBound: end,
	})

	return &Iterator{
		iter:  iter,
		db:    p.db,
		start: start,
		end:   end,
	}, nil
}

func (it *Iterator) Next() bool {
	if it.current.key == nil {
		if it.start == nil {
			it.iter.First()
		} else {
			it.iter.SeekGE(it.start)
		}
	} else {
		it.iter.Next()
	}

	if !it.iter.Valid() {
		return false
	}

	key := it.iter.Key()
	if it.end != nil && bytes.Compare(key, it.end) > 0 {
		return false
	}

	val := it.iter.Value()
	valCopy := make([]byte, len(val))
	copy(valCopy, val)

	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	it.current.key = keyCopy
	it.current.value = valCopy
	return true
}

func (it *Iterator) Key() []byte {
	return it.current.key
}

func (it *Iterator) Value() []byte {
	return it.current.value
}

func (it *Iterator) Error() error {
	if it.iter.Error() != nil {
		return it.iter.Error()
	}
	return it.err
}

func (it *Iterator) Close() error {
	it.iter.Close()
	return nil
}
