package bbolt

import (
	"context"
	"errors"
	"fmt"
	"github.com/LeJamon/goXRPLd/internal/storage/keyValueDb"
	"go.etcd.io/bbolt"
)

var (
	ErrDBClosed    = errors.New("keyValueDb is closed")
	ErrKeyNotFound = errors.New("key not found")
)

type BBoltDB struct {
	db     *bbolt.DB
	bucket []byte
}

func NewBBoltDB(db *bbolt.DB, bucket []byte) *BBoltDB {
	return &BBoltDB{
		db:     db,
		bucket: bucket,
	}
}

func (b *BBoltDB) Read(ctx context.Context, key []byte) ([]byte, error) {
	if b.db == nil {
		return nil, ErrDBClosed
	}

	var value []byte
	err := b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(b.bucket)
		if bucket == nil {
			return fmt.Errorf("bucket %s not found", string(b.bucket))
		}

		value = bucket.Get(key)
		if value == nil {
			return ErrKeyNotFound
		}

		// Make a copy of the value since bbolt's value is only valid during the transaction
		valueCopy := make([]byte, len(value))
		copy(valueCopy, value)
		value = valueCopy

		return nil
	})

	if err != nil {
		return nil, err
	}

	return value, nil
}

func (b *BBoltDB) Write(ctx context.Context, key []byte, value []byte) error {
	if b.db == nil {
		return ErrDBClosed
	}

	return b.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(b.bucket)
		if bucket == nil {
			return fmt.Errorf("bucket %s not found", string(b.bucket))
		}
		return bucket.Put(key, value)
	})
}

func (b *BBoltDB) Delete(ctx context.Context, key []byte) error {
	if b.db == nil {
		return ErrDBClosed
	}

	return b.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(b.bucket)
		if bucket == nil {
			return fmt.Errorf("bucket %s not found", string(b.bucket))
		}
		return bucket.Delete(key)
	})
}

func (b *BBoltDB) Batch(ctx context.Context, ops []keyValueDb.BatchOperation) error {
	if b.db == nil {
		return ErrDBClosed
	}

	return b.db.Batch(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(b.bucket)
		if bucket == nil {
			return fmt.Errorf("bucket %s not found", string(b.bucket))
		}

		for _, op := range ops {
			var err error
			switch op.Type {
			case keyValueDb.BatchPut:
				err = bucket.Put(op.Key, op.Value)
			case keyValueDb.BatchDelete:
				err = bucket.Delete(op.Key)
			default:
				return fmt.Errorf("unknown batch operation type: %d", op.Type)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

type BBoltIterator struct {
	tx      *bbolt.Tx
	cursor  *bbolt.Cursor
	current struct {
		key, value []byte
	}
	start, end []byte
	err        error
}

func (b *BBoltDB) Iterator(ctx context.Context, start, end []byte) (keyValueDb.Iterator, error) {
	if b.db == nil {
		return nil, ErrDBClosed
	}

	tx, err := b.db.Begin(false) // Read-only transaction
	if err != nil {
		return nil, err
	}

	bucket := tx.Bucket(b.bucket)
	if bucket == nil {
		tx.Rollback()
		return nil, fmt.Errorf("bucket %s not found", string(b.bucket))
	}

	return &BBoltIterator{
		tx:     tx,
		cursor: bucket.Cursor(),
		start:  start,
		end:    end,
	}, nil
}

func (it *BBoltIterator) Next() bool {
	var k, v []byte
	if it.current.key == nil {
		// First iteration
		if it.start == nil {
			k, v = it.cursor.First()
		} else {
			k, v = it.cursor.Seek(it.start)
		}
	} else {
		k, v = it.cursor.Next()
	}

	if k == nil || (it.end != nil && string(k) > string(it.end)) {
		it.current.key = nil
		it.current.value = nil
		return false
	}

	it.current.key = k
	it.current.value = v
	return true
}

func (it *BBoltIterator) Key() []byte {
	return it.current.key
}

func (it *BBoltIterator) Value() []byte {
	return it.current.value
}

func (it *BBoltIterator) Error() error {
	return it.err
}

func (it *BBoltIterator) Close() error {
	return it.tx.Rollback()
}
