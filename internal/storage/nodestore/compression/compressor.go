package compression

import (
	"fmt"
	"sync"
)

// Compressor defines the interface for compression algorithms.
type Compressor interface {
	// Name returns the name of the compression algorithm.
	Name() string

	// Compress compresses the input data.
	// Returns the compressed data and any error that occurred.
	Compress(data []byte, level int) ([]byte, error)

	// Decompress decompresses the input data.
	// Returns the decompressed data and any error that occurred.
	Decompress(data []byte) ([]byte, error)

	// MaxCompressedSize returns the maximum size of compressed data
	// for the given uncompressed size.
	MaxCompressedSize(uncompressedSize int) int
}

// Factory is a function that creates a new compressor instance.
type Factory func() Compressor

var (
	mu          sync.RWMutex
	compressors = make(map[string]Factory)
)

// Register registers a compressor factory with the given name.
func Register(name string, factory Factory) {
	mu.Lock()
	defer mu.Unlock()
	compressors[name] = factory
}

// Get returns a new compressor instance for the given name.
func Get(name string) (Compressor, error) {
	mu.RLock()
	factory, ok := compressors[name]
	mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown compressor: %s", name)
	}

	return factory(), nil
}

// Available returns a list of available compressor names.
func Available() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(compressors))
	for name := range compressors {
		names = append(names, name)
	}
	return names
}

// IsAvailable checks if a compressor with the given name is available.
func IsAvailable(name string) bool {
	mu.RLock()
	_, ok := compressors[name]
	mu.RUnlock()
	return ok
}

// init registers the built-in compressors.
func init() {
	Register("none", func() Compressor { return &NoCompressor{} })
	Register("lz4", func() Compressor { return &LZ4Compressor{} })
}
