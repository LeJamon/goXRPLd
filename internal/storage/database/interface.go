package database

import (
	"context"
)

// DB defines the basic operations any database implementation must support
type DB interface {
	// Basic operations
	Read(ctx context.Context, key []byte) ([]byte, error)
	Write(ctx context.Context, key []byte, value []byte) error
	Delete(ctx context.Context, key []byte) error

	// Batch operations
	Batch(ctx context.Context, ops []BatchOperation) error

	// Iteration
	Iterator(ctx context.Context, start, end []byte) (Iterator, error)
}

// Iterator allows traversing over database entries
type Iterator interface {
	Next() bool
	Key() []byte
	Value() []byte
	Error() error
	Close() error
}

// BatchOperation represents a single operation in a batch
type BatchOperation struct {
	Type  BatchOpType
	Key   []byte
	Value []byte
}

type BatchOpType int

const (
	BatchPut BatchOpType = iota
	BatchDelete
)
