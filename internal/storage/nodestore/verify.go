package nodestore

import (
	"fmt"
	"sync/atomic"
)

// Verifier defines the interface for data verification operations.
type Verifier interface {
	// Verify checks the integrity of all nodes in the backend.
	// It returns an error if any node fails verification.
	Verify() error

	// VerifyNode verifies a single node by its hash.
	// It returns an error if the node doesn't exist or fails verification.
	VerifyNode(hash Hash256) error
}

// VerificationResult holds the result of a verification operation.
type VerificationResult struct {
	TotalNodes    int64     // Total number of nodes checked
	CorruptNodes  int64     // Number of corrupt nodes found
	MissingData   int64     // Number of nodes with missing data
	HashMismatch  int64     // Number of nodes with hash mismatches
	CorruptHashes []Hash256 // List of corrupt node hashes (limited to first 100)
}

// IsValid returns true if no corruption was detected.
func (r *VerificationResult) IsValid() bool {
	return r.CorruptNodes == 0 && r.MissingData == 0 && r.HashMismatch == 0
}

// String returns a formatted string representation of the verification result.
func (r *VerificationResult) String() string {
	status := "VALID"
	if !r.IsValid() {
		status = "CORRUPT"
	}

	return fmt.Sprintf(`Verification Result: %s
  Total Nodes: %d
  Corrupt Nodes: %d
  Missing Data: %d
  Hash Mismatches: %d`,
		status,
		r.TotalNodes,
		r.CorruptNodes,
		r.MissingData,
		r.HashMismatch)
}

// VerifyOptions holds options for verification operations.
type VerifyOptions struct {
	// StopOnFirstError stops verification when the first error is encountered.
	StopOnFirstError bool

	// MaxCorruptNodes limits the number of corrupt node hashes collected.
	// Default is 100.
	MaxCorruptNodes int

	// ProgressCallback is called periodically with the number of nodes verified.
	// Can be nil to disable progress reporting.
	ProgressCallback func(verified int64)

	// ProgressInterval specifies how often to call ProgressCallback.
	// Default is every 10000 nodes.
	ProgressInterval int64
}

// DefaultVerifyOptions returns default verification options.
func DefaultVerifyOptions() *VerifyOptions {
	return &VerifyOptions{
		StopOnFirstError: false,
		MaxCorruptNodes:  100,
		ProgressInterval: 10000,
	}
}

// Verify implements the Verifier interface for PebbleBackend.
// It iterates over all nodes and verifies that each node's hash matches its content.
func (p *PebbleBackend) Verify() error {
	return p.VerifyWithOptions(DefaultVerifyOptions())
}

// VerifyWithOptions performs verification with custom options.
func (p *PebbleBackend) VerifyWithOptions(opts *VerifyOptions) error {
	if opts == nil {
		opts = DefaultVerifyOptions()
	}

	result, err := p.VerifyAll(opts)
	if err != nil {
		return err
	}

	if !result.IsValid() {
		return fmt.Errorf("verification failed: %d corrupt nodes found", result.CorruptNodes)
	}

	return nil
}

// VerifyAll performs full verification and returns detailed results.
func (p *PebbleBackend) VerifyAll(opts *VerifyOptions) (*VerificationResult, error) {
	if !p.IsOpen() {
		return nil, ErrBackendClosed
	}

	if opts == nil {
		opts = DefaultVerifyOptions()
	}

	result := &VerificationResult{
		CorruptHashes: make([]Hash256, 0, opts.MaxCorruptNodes),
	}

	var verified int64

	err := p.ForEach(func(node *Node) error {
		verified++
		atomic.AddInt64(&result.TotalNodes, 1)

		// Report progress if callback is set
		if opts.ProgressCallback != nil && opts.ProgressInterval > 0 {
			if verified%opts.ProgressInterval == 0 {
				opts.ProgressCallback(verified)
			}
		}

		// Check for missing data
		if len(node.Data) == 0 {
			atomic.AddInt64(&result.MissingData, 1)
			atomic.AddInt64(&result.CorruptNodes, 1)
			if len(result.CorruptHashes) < opts.MaxCorruptNodes {
				result.CorruptHashes = append(result.CorruptHashes, node.Hash)
			}
			if opts.StopOnFirstError {
				return fmt.Errorf("node has missing data: %x", node.Hash)
			}
			return nil
		}

		// Verify hash matches content
		expectedHash := ComputeHash256(node.Data)
		if node.Hash != expectedHash {
			atomic.AddInt64(&result.HashMismatch, 1)
			atomic.AddInt64(&result.CorruptNodes, 1)
			if len(result.CorruptHashes) < opts.MaxCorruptNodes {
				result.CorruptHashes = append(result.CorruptHashes, node.Hash)
			}
			if opts.StopOnFirstError {
				return fmt.Errorf("hash mismatch for node %x: expected %x", node.Hash, expectedHash)
			}
		}

		return nil
	})

	if err != nil {
		return result, err
	}

	return result, nil
}

// VerifyNode implements the Verifier interface for PebbleBackend.
// It verifies a single node by fetching it and checking that its hash matches the content.
func (p *PebbleBackend) VerifyNode(hash Hash256) error {
	if !p.IsOpen() {
		return ErrBackendClosed
	}

	node, status := p.Fetch(hash)
	if status == NotFound {
		return fmt.Errorf("node not found: %x", hash)
	}
	if status != OK {
		return fmt.Errorf("failed to fetch node %x: %s", hash, status.String())
	}

	// Check for missing data
	if len(node.Data) == 0 {
		return fmt.Errorf("node has missing data: %x", hash)
	}

	// Verify hash matches content
	expectedHash := ComputeHash256(node.Data)
	if node.Hash != expectedHash {
		return fmt.Errorf("hash mismatch for node %x: computed %x", hash, expectedHash)
	}

	return nil
}

// Verify implements the Verifier interface for MemoryBackend.
func (m *MemoryBackend) Verify() error {
	return m.VerifyWithOptions(DefaultVerifyOptions())
}

// VerifyWithOptions performs verification with custom options for MemoryBackend.
func (m *MemoryBackend) VerifyWithOptions(opts *VerifyOptions) error {
	if opts == nil {
		opts = DefaultVerifyOptions()
	}

	result, err := m.VerifyAll(opts)
	if err != nil {
		return err
	}

	if !result.IsValid() {
		return fmt.Errorf("verification failed: %d corrupt nodes found", result.CorruptNodes)
	}

	return nil
}

// VerifyAll performs full verification and returns detailed results for MemoryBackend.
func (m *MemoryBackend) VerifyAll(opts *VerifyOptions) (*VerificationResult, error) {
	if !m.IsOpen() {
		return nil, ErrBackendClosed
	}

	if opts == nil {
		opts = DefaultVerifyOptions()
	}

	result := &VerificationResult{
		CorruptHashes: make([]Hash256, 0, opts.MaxCorruptNodes),
	}

	var verified int64

	err := m.ForEach(func(node *Node) error {
		verified++
		atomic.AddInt64(&result.TotalNodes, 1)

		// Report progress if callback is set
		if opts.ProgressCallback != nil && opts.ProgressInterval > 0 {
			if verified%opts.ProgressInterval == 0 {
				opts.ProgressCallback(verified)
			}
		}

		// Check for missing data
		if len(node.Data) == 0 {
			atomic.AddInt64(&result.MissingData, 1)
			atomic.AddInt64(&result.CorruptNodes, 1)
			if len(result.CorruptHashes) < opts.MaxCorruptNodes {
				result.CorruptHashes = append(result.CorruptHashes, node.Hash)
			}
			if opts.StopOnFirstError {
				return fmt.Errorf("node has missing data: %x", node.Hash)
			}
			return nil
		}

		// Verify hash matches content
		expectedHash := ComputeHash256(node.Data)
		if node.Hash != expectedHash {
			atomic.AddInt64(&result.HashMismatch, 1)
			atomic.AddInt64(&result.CorruptNodes, 1)
			if len(result.CorruptHashes) < opts.MaxCorruptNodes {
				result.CorruptHashes = append(result.CorruptHashes, node.Hash)
			}
			if opts.StopOnFirstError {
				return fmt.Errorf("hash mismatch for node %x: expected %x", node.Hash, expectedHash)
			}
		}

		return nil
	})

	if err != nil {
		return result, err
	}

	return result, nil
}

// VerifyNode implements the Verifier interface for MemoryBackend.
func (m *MemoryBackend) VerifyNode(hash Hash256) error {
	if !m.IsOpen() {
		return ErrBackendClosed
	}

	node, status := m.Fetch(hash)
	if status == NotFound {
		return fmt.Errorf("node not found: %x", hash)
	}
	if status != OK {
		return fmt.Errorf("failed to fetch node %x: %s", hash, status.String())
	}

	// Check for missing data
	if len(node.Data) == 0 {
		return fmt.Errorf("node has missing data: %x", hash)
	}

	// Verify hash matches content
	expectedHash := ComputeHash256(node.Data)
	if node.Hash != expectedHash {
		return fmt.Errorf("hash mismatch for node %x: computed %x", hash, expectedHash)
	}

	return nil
}

// BackendVerifier wraps any Backend to provide verification capabilities.
type BackendVerifier struct {
	backend Backend
}

// NewBackendVerifier creates a new verifier for the given backend.
func NewBackendVerifier(backend Backend) *BackendVerifier {
	return &BackendVerifier{backend: backend}
}

// Verify implements the Verifier interface.
func (v *BackendVerifier) Verify() error {
	return v.VerifyWithOptions(DefaultVerifyOptions())
}

// VerifyWithOptions performs verification with custom options.
func (v *BackendVerifier) VerifyWithOptions(opts *VerifyOptions) error {
	if opts == nil {
		opts = DefaultVerifyOptions()
	}

	result, err := v.VerifyAll(opts)
	if err != nil {
		return err
	}

	if !result.IsValid() {
		return fmt.Errorf("verification failed: %d corrupt nodes found", result.CorruptNodes)
	}

	return nil
}

// VerifyAll performs full verification and returns detailed results.
func (v *BackendVerifier) VerifyAll(opts *VerifyOptions) (*VerificationResult, error) {
	if !v.backend.IsOpen() {
		return nil, ErrBackendClosed
	}

	if opts == nil {
		opts = DefaultVerifyOptions()
	}

	result := &VerificationResult{
		CorruptHashes: make([]Hash256, 0, opts.MaxCorruptNodes),
	}

	var verified int64

	err := v.backend.ForEach(func(node *Node) error {
		verified++
		atomic.AddInt64(&result.TotalNodes, 1)

		// Report progress if callback is set
		if opts.ProgressCallback != nil && opts.ProgressInterval > 0 {
			if verified%opts.ProgressInterval == 0 {
				opts.ProgressCallback(verified)
			}
		}

		// Check for missing data
		if len(node.Data) == 0 {
			atomic.AddInt64(&result.MissingData, 1)
			atomic.AddInt64(&result.CorruptNodes, 1)
			if len(result.CorruptHashes) < opts.MaxCorruptNodes {
				result.CorruptHashes = append(result.CorruptHashes, node.Hash)
			}
			if opts.StopOnFirstError {
				return fmt.Errorf("node has missing data: %x", node.Hash)
			}
			return nil
		}

		// Verify hash matches content
		expectedHash := ComputeHash256(node.Data)
		if node.Hash != expectedHash {
			atomic.AddInt64(&result.HashMismatch, 1)
			atomic.AddInt64(&result.CorruptNodes, 1)
			if len(result.CorruptHashes) < opts.MaxCorruptNodes {
				result.CorruptHashes = append(result.CorruptHashes, node.Hash)
			}
			if opts.StopOnFirstError {
				return fmt.Errorf("hash mismatch for node %x: expected %x", node.Hash, expectedHash)
			}
		}

		return nil
	})

	if err != nil {
		return result, err
	}

	return result, nil
}

// VerifyNode verifies a single node by its hash.
func (v *BackendVerifier) VerifyNode(hash Hash256) error {
	if !v.backend.IsOpen() {
		return ErrBackendClosed
	}

	node, status := v.backend.Fetch(hash)
	if status == NotFound {
		return fmt.Errorf("node not found: %x", hash)
	}
	if status != OK {
		return fmt.Errorf("failed to fetch node %x: %s", hash, status.String())
	}

	// Check for missing data
	if len(node.Data) == 0 {
		return fmt.Errorf("node has missing data: %x", hash)
	}

	// Verify hash matches content
	expectedHash := ComputeHash256(node.Data)
	if node.Hash != expectedHash {
		return fmt.Errorf("hash mismatch for node %x: computed %x", hash, expectedHash)
	}

	return nil
}
