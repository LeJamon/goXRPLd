package bbolt

import (
	"context"
	"errors"
	"fmt"
	"github.com/LeJamon/goXRPLd/internal/storage/database"
	"go.etcd.io/bbolt"
)

var (
	ErrDBClosed    = errors.New("database is closed")
	ErrKeyNotFound = errors.New("key not found")
)

type DB struct {
	db     *bbolt.DB
	bucket []byte
}

func NewDB(db *bbolt.DB, bucket []byte) *DB {
	return &DB{
		db:     db,
		bucket: bucket,
	}
}

func (b *DB) Read(ctx context.Context, key []byte) ([]byte, error) {
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

		// Make a copy of the value since bbolt's val
		//ue is only valid during the transaction
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

func (b *DB) Write(ctx context.Context, key []byte, value []byte) error {
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

func (b *DB) Delete(ctx context.Context, key []byte) error {
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

func (b *DB) Batch(ctx context.Context, ops []database.BatchOperation) error {
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
			case database.BatchPut:
				err = bucket.Put(op.Key, op.Value)
			case database.BatchDelete:
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

type Iterator struct {
	tx      *bbolt.Tx
	cursor  *bbolt.Cursor
	current struct {
		key, value []byte
	}
	start, end []byte
	err        error
}

func (b *DB) Iterator(ctx context.Context, start, end []byte) (database.Iterator, error) {
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

	return &Iterator{
		tx:     tx,
		cursor: bucket.Cursor(),
		start:  start,
		end:    end,
	}, nil
}

func (it *Iterator) Next() bool {
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

func (it *Iterator) Key() []byte {
	return it.current.key
}

func (it *Iterator) Value() []byte {
	return it.current.value
}

func (it *Iterator) Error() error {
	return it.err
}

func (it *Iterator) Close() error {
	return it.tx.Rollback()
}
