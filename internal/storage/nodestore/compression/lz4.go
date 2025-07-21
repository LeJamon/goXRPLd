package compression

import (
	"fmt"
	"github.com/pierrec/lz4"
)

// NoCompressor implements a pass-through compressor that doesn't compress data.
type NoCompressor struct{}

// Name returns the name of the compressor.
func (c *NoCompressor) Name() string {
	return "none"
}

// Compress returns the data unchanged.
func (c *NoCompressor) Compress(data []byte, level int) ([]byte, error) {
	// Return a copy to ensure the caller can modify it safely
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// Decompress returns the data unchanged.
func (c *NoCompressor) Decompress(data []byte) ([]byte, error) {
	// Return a copy to ensure the caller can modify it safely
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// MaxCompressedSize returns the same size since no compression is performed.
func (c *NoCompressor) MaxCompressedSize(uncompressedSize int) int {
	return uncompressedSize
}

// LZ4Compressor implements LZ4 compression (XRPL compatible).
type LZ4Compressor struct{}

// Name returns the name of the compressor.
func (c *LZ4Compressor) Name() string {
	return "lz4"
}

// Compress compresses data using LZ4.
func (c *LZ4Compressor) Compress(data []byte, level int) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	// Allocate buffer for compressed data
	maxSize := lz4.CompressBlockBound(len(data))
	compressed := make([]byte, maxSize)

	// Compress the data
	compressedSize, err := lz4.CompressBlock(data, compressed, nil)
	if err != nil {
		return nil, fmt.Errorf("lz4 compression failed: %w", err)
	}

	// Return only the used portion
	return compressed[:compressedSize], nil
}

// Decompress decompresses LZ4 data.
func (c *LZ4Compressor) Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	// Try with increasing buffer sizes.
	for bufferSize := len(data) * 2; bufferSize <= len(data)*10; bufferSize *= 2 {
		decompressed := make([]byte, bufferSize)
		n, err := lz4.UncompressBlock(data, decompressed)
		if err == nil {
			return decompressed[:n], nil
		}
	}
	return nil, fmt.Errorf("lz4 decompression failed after multiple attempts")
}

// MaxCompressedSize returns the maximum compressed size for a given uncompressed size using LZ4.
func (c *LZ4Compressor) MaxCompressedSize(uncompressedSize int) int {
	return lz4.CompressBlockBound(uncompressedSize)
}
