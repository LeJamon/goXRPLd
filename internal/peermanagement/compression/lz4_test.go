package compression

import (
	"bytes"
	"testing"
)

func TestCompressDecompressLZ4(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"repetitive_data", bytes.Repeat([]byte("ABCDEFGHIJ"), 100)},
		{"mixed_data", append(bytes.Repeat([]byte{0x00}, 500), bytes.Repeat([]byte{0xFF}, 500)...)},
		{"sequential", makeSequentialData(1000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressed, err := CompressLZ4(tt.data)
			if err != nil {
				t.Fatalf("CompressLZ4 error: %v", err)
			}

			if compressed == nil {
				t.Skip("Data was incompressible")
			}

			// Compressed should be smaller for compressible data
			if len(compressed) >= len(tt.data) {
				t.Logf("Warning: compressed size %d >= original %d", len(compressed), len(tt.data))
			}

			decompressed, err := DecompressLZ4(compressed, len(tt.data))
			if err != nil {
				t.Fatalf("DecompressLZ4 error: %v", err)
			}

			if !bytes.Equal(decompressed, tt.data) {
				t.Error("Decompressed data doesn't match original")
			}
		})
	}
}

func TestCompressLZ4TooSmall(t *testing.T) {
	// Data smaller than MinCompressibleSize should return nil
	smallData := bytes.Repeat([]byte{0x42}, MinCompressibleSize-1)
	compressed, err := CompressLZ4(smallData)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if compressed != nil {
		t.Error("Expected nil for too-small data")
	}
}

func TestCompressLZ4Incompressible(t *testing.T) {
	// Random-looking data that's hard to compress
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i * 17 % 256)
	}

	compressed, err := CompressLZ4(data)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	// If compression doesn't help, we get nil
	if compressed != nil && len(compressed) >= len(data) {
		t.Error("Compression should return nil if it doesn't save space")
	}
}

func TestDecompressLZ4InvalidSize(t *testing.T) {
	_, err := DecompressLZ4([]byte{1, 2, 3}, 0)
	if err != ErrDecompressionFailed {
		t.Errorf("Expected ErrDecompressionFailed, got %v", err)
	}

	_, err = DecompressLZ4([]byte{1, 2, 3}, -1)
	if err != ErrDecompressionFailed {
		t.Errorf("Expected ErrDecompressionFailed for negative size, got %v", err)
	}
}

func TestDecompressLZ4WrongSize(t *testing.T) {
	original := bytes.Repeat([]byte{0x42}, 200)
	compressed, err := CompressLZ4(original)
	if err != nil || compressed == nil {
		t.Skip("Compression failed")
	}

	// Try to decompress with wrong size
	_, err = DecompressLZ4(compressed, len(original)+100)
	if err == nil {
		t.Error("Expected error for wrong decompressed size")
	}
}

func TestShouldCompress(t *testing.T) {
	compressibleTypes := []uint16{
		2,  // mtMANIFESTS
		15, // mtENDPOINTS
		30, // mtTRANSACTION
		31, // mtGET_LEDGER
		32, // mtLEDGER_DATA
		42, // mtGET_OBJECTS
		54, // mtVALIDATORLIST
		56, // mtVALIDATORLISTCOLLECTION
		60, // mtREPLAY_DELTA_RESPONSE
		64, // mtTRANSACTIONS
	}

	for _, msgType := range compressibleTypes {
		if !ShouldCompress(msgType) {
			t.Errorf("ShouldCompress(%d) = false, want true", msgType)
		}
	}

	nonCompressibleTypes := []uint16{
		3,  // mtPING
		5,  // mtCLUSTER
		34, // mtSTATUS_CHANGE
		41, // mtVALIDATION
		55, // mtSQUELCH
	}

	for _, msgType := range nonCompressibleTypes {
		if ShouldCompress(msgType) {
			t.Errorf("ShouldCompress(%d) = true, want false", msgType)
		}
	}
}

func TestCompressIfWorthwhile(t *testing.T) {
	tests := []struct {
		name       string
		msgType    uint16
		data       []byte
		wantCompr  bool
	}{
		{
			"small_compressible_type",
			30, // mtTRANSACTION
			bytes.Repeat([]byte{0x42}, 50), // Too small
			false,
		},
		{
			"non_compressible_type",
			3, // mtPING
			bytes.Repeat([]byte{0x42}, 200),
			false,
		},
		{
			"large_compressible_type",
			30, // mtTRANSACTION
			bytes.Repeat([]byte{0x42}, 200),
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, compressed := CompressIfWorthwhile(tt.msgType, tt.data)
			if compressed != tt.wantCompr {
				t.Errorf("CompressIfWorthwhile compressed = %v, want %v", compressed, tt.wantCompr)
			}

			if compressed {
				if len(result) >= len(tt.data) {
					t.Error("Compressed data should be smaller")
				}
			} else {
				if !bytes.Equal(result, tt.data) {
					t.Error("Uncompressed data should be returned unchanged")
				}
			}
		})
	}
}

func TestMinCompressibleSize(t *testing.T) {
	// Test boundary
	if MinCompressibleSize != 70 {
		t.Errorf("MinCompressibleSize = %d, want 70 (rippled reference)", MinCompressibleSize)
	}
}

func BenchmarkCompressLZ4(b *testing.B) {
	data := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 100)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		CompressLZ4(data)
	}
}

func BenchmarkDecompressLZ4(b *testing.B) {
	original := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 100)
	compressed, _ := CompressLZ4(original)
	if compressed == nil {
		b.Skip("Compression failed")
	}

	b.SetBytes(int64(len(original)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		DecompressLZ4(compressed, len(original))
	}
}

// makeSequentialData creates data with sequential bytes
func makeSequentialData(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	return data
}
