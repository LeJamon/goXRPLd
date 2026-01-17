// Package compression implements message compression for the XRPL peer protocol.
// Currently supports LZ4 compression as used by rippled.
package compression

import (
	"errors"

	"github.com/pierrec/lz4"
)

const (
	// MinCompressibleSize is the minimum message size worth compressing.
	// Messages smaller than this are sent uncompressed.
	// Reference: rippled Message.cpp - messageBytes <= 70
	MinCompressibleSize = 70
)

var (
	// ErrDecompressionFailed is returned when decompression fails.
	ErrDecompressionFailed = errors.New("decompression failed")
	// ErrCompressionFailed is returned when compression fails.
	ErrCompressionFailed = errors.New("compression failed")
)

// CompressLZ4 compresses data using LZ4.
// Returns the compressed data or nil if compression wouldn't save space.
func CompressLZ4(data []byte) ([]byte, error) {
	if len(data) < MinCompressibleSize {
		return nil, nil // Too small to compress
	}

	// Allocate buffer for worst case
	maxSize := lz4.CompressBlockBound(len(data))
	compressed := make([]byte, maxSize)

	n, err := lz4.CompressBlock(data, compressed, nil)
	if err != nil {
		return nil, err
	}

	if n == 0 {
		return nil, nil // Incompressible
	}

	// Only use compression if it actually saves space
	if n >= len(data) {
		return nil, nil
	}

	return compressed[:n], nil
}

// DecompressLZ4 decompresses LZ4 compressed data.
// uncompressedSize is the expected size of the uncompressed data.
func DecompressLZ4(compressed []byte, uncompressedSize int) ([]byte, error) {
	if uncompressedSize <= 0 {
		return nil, ErrDecompressionFailed
	}

	decompressed := make([]byte, uncompressedSize)

	n, err := lz4.UncompressBlock(compressed, decompressed)
	if err != nil {
		return nil, err
	}

	if n != uncompressedSize {
		return nil, ErrDecompressionFailed
	}

	return decompressed, nil
}

// ShouldCompress returns true if the message type should be compressed.
// Reference: rippled Message.cpp compress() - compressible types
func ShouldCompress(msgType uint16) bool {
	switch msgType {
	case 2:  // mtMANIFESTS
		return true
	case 15: // mtENDPOINTS
		return true
	case 30: // mtTRANSACTION
		return true
	case 31: // mtGET_LEDGER
		return true
	case 32: // mtLEDGER_DATA
		return true
	case 42: // mtGET_OBJECTS
		return true
	case 54: // mtVALIDATORLIST
		return true
	case 56: // mtVALIDATORLISTCOLLECTION
		return true
	case 60: // mtREPLAY_DELTA_RESPONSE
		return true
	case 64: // mtTRANSACTIONS
		return true
	default:
		return false
	}
}

// CompressIfWorthwhile compresses data if it would be beneficial.
// Returns (compressed data, true) if compression was applied,
// or (original data, false) if compression was skipped.
func CompressIfWorthwhile(msgType uint16, data []byte) ([]byte, bool) {
	if !ShouldCompress(msgType) {
		return data, false
	}

	if len(data) < MinCompressibleSize {
		return data, false
	}

	compressed, err := CompressLZ4(data)
	if err != nil || compressed == nil {
		return data, false
	}

	return compressed, true
}
