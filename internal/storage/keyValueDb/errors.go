package keyValueDb

import "errors"

var (
	// ErrDBClosed is returned when trying to operate on a closed keyValueDb
	ErrDBClosed = errors.New("keyValueDb is closed")

	// ErrKeyNotFound is returned when a key doesn't exist in the keyValueDb
	ErrKeyNotFound = errors.New("key not found")

	// ErrNamespaceNotFound is returned when trying to access a non-existent namespace
	ErrNamespaceNotFound = errors.New("namespace not found")

	// ErrBatchOperationFailed is returned when a batch operation fails
	ErrBatchOperationFailed = errors.New("batch operation failed")
)
