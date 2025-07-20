package nodestore

import (
	"errors"
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/types"
)

var (
	// ErrNotFound indicates that a requested node was not found
	ErrNotFound = errors.New("node not found")

	// ErrDataCorrupt indicates that stored data is corrupted
	ErrDataCorrupt = errors.New("data corruption detected")

	// ErrBackendClosed indicates that the backend is closed
	ErrBackendClosed = errors.New("backend is closed")

	// ErrInvalidNode indicates that a node is invalid
	ErrInvalidNode = errors.New("invalid node")

	// ErrInvalidHash indicates that a hash is invalid
	ErrInvalidHash = errors.New("invalid hash")

	// ErrInvalidConfig indicates that the configuration is invalid
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrUnsupportedBackend indicates that a backend is not supported
	ErrUnsupportedBackend = errors.New("unsupported backend")

	// ErrUnsupportedCompressor indicates that a compressor is not supported
	ErrUnsupportedCompressor = errors.New("unsupported compressor")

	// ErrCompressionFailed indicates that compression failed
	ErrCompressionFailed = errors.New("compression failed")

	// ErrDecompressionFailed indicates that decompression failed
	ErrDecompressionFailed = errors.New("decompression failed")

	// ErrCacheFull indicates that the cache is full
	ErrCacheFull = errors.New("cache is full")

	// ErrTimeout indicates that an operation timed out
	ErrTimeout = errors.New("operation timed out")

	// ErrShutdown indicates that the database is shutting down
	ErrShutdown = errors.New("database is shutting down")
)

// NodeStoreError wraps an error with additional context specific to the NodeStore.
type NodeStoreError struct {
	Operation string        // The operation that failed
	Hash      types.Hash256 // The hash involved in the operation (if applicable)
	Backend   string        // The backend name
	Cause     error         // The underlying error
}

// Error implements the error interface.
func (e *NodeStoreError) Error() string {

	//TODO correct code here
	/*if e.Hash.IsZero() {
		return fmt.Sprintf("nodestore %s error on backend %s: %v",
			e.Operation, e.Backend, e.Cause)
	}*/
	return fmt.Sprintf("nodestore %s error on backend %s for hash %s: %v",
		e.Operation, e.Backend, "e.Hash.String()", e.Cause)
}

// Unwrap returns the underlying error.
func (e *NodeStoreError) Unwrap() error {
	return e.Cause
}

// Is checks if the error matches the target error.
func (e *NodeStoreError) Is(target error) bool {
	return errors.Is(e.Cause, target)
}

// NewError creates a new NodeStoreError.
func NewError(operation, backend string, hash types.Hash256, cause error) *NodeStoreError {
	return &NodeStoreError{
		Operation: operation,
		Hash:      hash,
		Backend:   backend,
		Cause:     cause,
	}
}

// NewErrorWithoutHash creates a new NodeStoreError without a hash.
func NewErrorWithoutHash(operation, backend string, cause error) *NodeStoreError {
	return &NodeStoreError{
		Operation: operation,
		Backend:   backend,
		Cause:     cause,
	}
}

// ValidationError represents a validation error.
type ValidationError struct {
	Field   string      // The field that failed validation
	Value   interface{} // The invalid value
	Message string      // Human-readable error message
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Value != nil {
		return fmt.Sprintf("validation error: %s (value: %v): %s",
			e.Field, e.Value, e.Message)
	}
	return fmt.Sprintf("validation error: %s: %s", e.Field, e.Message)
}

// NewValidationError creates a new ValidationError.
func NewValidationError(field string, value interface{}, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	}
}

// BackendError represents an error from a storage backend.
type NodeStoreBackendError struct {
	Backend   string        // The backend name
	Operation string        // The operation that failed
	Hash      types.Hash256 // The hash involved (if applicable)
	Status    Status        // The backend status code
	Message   string        // Error message
	Cause     error         // The underlying error
}

// Error implements the error interface.
func (e *NodeStoreBackendError) Error() string {
	if e.Hash.IsZero() {
		return fmt.Sprintf("backend %s %s error: %s (status: %s)",
			e.Backend, e.Operation, e.Message, e.Status.String())
	}
	return fmt.Sprintf("backend %s %s error for hash %s: %s (status: %s)",
		e.Backend, e.Operation, e.Hash.String(), e.Message, e.Status.String())
}

// Unwrap returns the underlying error.
func (e *NodeStoreBackendError) Unwrap() error {
	return e.Cause
}

// Is checks if the error matches the target error.
func (e *NodeStoreBackendError) Is(target error) bool {
	if e.Cause != nil {
		return errors.Is(e.Cause, target)
	}

	// Check against common errors based on status
	switch e.Status {
	case NotFound:
		return target == ErrNotFound
	case DataCorrupt:
		return target == ErrDataCorrupt
	case BackendError:
		return target == ErrBackendClosed
	}

	return false
}

// NewBackendError creates a new BackendError.
func NewBackendError(backend, operation string, hash types.Hash256, status Status, message string, cause error) *NodeStoreBackendError {
	return &NodeStoreBackendError{
		Backend:   backend,
		Operation: operation,
		Hash:      hash,
		Status:    status,
		Message:   message,
		Cause:     cause,
	}
}

// CompressionError represents a compression-related error.
type CompressionError struct {
	Compressor string // The compressor name
	Operation  string // "compress" or "decompress"
	DataSize   int    // Size of the data being processed
	Cause      error  // The underlying error
}

// Error implements the error interface.
func (e *CompressionError) Error() string {
	return fmt.Sprintf("compression error: %s %s failed for %d bytes: %v",
		e.Compressor, e.Operation, e.DataSize, e.Cause)
}

// Unwrap returns the underlying error.
func (e *CompressionError) Unwrap() error {
	return e.Cause
}

// NewCompressionError creates a new CompressionError.
func NewCompressionError(compressor, operation string, dataSize int, cause error) *CompressionError {
	return &CompressionError{
		Compressor: compressor,
		Operation:  operation,
		DataSize:   dataSize,
		Cause:      cause,
	}
}

// IsNotFound checks if an error indicates that a node was not found.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsDataCorrupt checks if an error indicates data corruption.
func IsDataCorrupt(err error) bool {
	return errors.Is(err, ErrDataCorrupt)
}

// IsBackendClosed checks if an error indicates that the backend is closed.
func IsBackendClosed(err error) bool {
	return errors.Is(err, ErrBackendClosed)
}

// IsTimeout checks if an error indicates a timeout.
func IsTimeout(err error) bool {
	return errors.Is(err, ErrTimeout)
}

// IsShutdown checks if an error indicates that the database is shutting down.
func IsShutdown(err error) bool {
	return errors.Is(err, ErrShutdown)
}

// WrapError wraps an error with additional context.
func WrapError(err error, operation, backend string, hash types.Hash256) error {
	if err == nil {
		return nil
	}
	return NewError(operation, backend, hash, err)
}

// WrapErrorWithoutHash wraps an error with additional context but without a hash.
func WrapErrorWithoutHash(err error, operation, backend string) error {
	if err == nil {
		return nil
	}
	return NewErrorWithoutHash(operation, backend, err)
}
