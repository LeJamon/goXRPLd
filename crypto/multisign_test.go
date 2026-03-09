package crypto

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPrefix_Bytes(t *testing.T) {
	tests := []struct {
		name     string
		prefix   HashPrefix
		expected []byte
	}{
		{"TransactionID (TXN)", HashPrefixTransactionID, []byte{0x54, 0x58, 0x4E, 0x00}},
		{"TxMultiSign (SMT)", HashPrefixTxMultiSign, []byte{0x53, 0x4D, 0x54, 0x00}},
		{"TxSign (STX)", HashPrefixTxSign, []byte{0x53, 0x54, 0x58, 0x00}},
		{"Validation (VAL)", HashPrefixValidation, []byte{0x56, 0x41, 0x4C, 0x00}},
		{"LedgerMaster (LWR)", HashPrefixLedgerMaster, []byte{0x4C, 0x57, 0x52, 0x00}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.prefix.Bytes()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMultiSignPrefix(t *testing.T) {
	// Should be "SMT\0" in bytes
	expected := []byte{0x53, 0x4D, 0x54, 0x00}
	assert.Equal(t, expected, MultiSignPrefix)

	// Should match HashPrefixTxMultiSign
	assert.Equal(t, HashPrefixTxMultiSign.Bytes(), MultiSignPrefix)
}

func TestBuildMultiSigningData(t *testing.T) {
	t.Run("Prepends prefix to transaction data", func(t *testing.T) {
		txData := []byte{0x01, 0x02, 0x03, 0x04, 0x05}

		result := BuildMultiSigningData(txData)

		// Should start with MultiSignPrefix
		assert.True(t, bytes.HasPrefix(result, MultiSignPrefix))
		// Should end with tx data
		assert.True(t, bytes.HasSuffix(result, txData))
		// Total length should be prefix + data
		assert.Equal(t, len(MultiSignPrefix)+len(txData), len(result))
	})

	t.Run("Empty transaction data", func(t *testing.T) {
		result := BuildMultiSigningData([]byte{})
		assert.Equal(t, MultiSignPrefix, result)
	})

	t.Run("Nil transaction data", func(t *testing.T) {
		result := BuildMultiSigningData(nil)
		assert.Equal(t, MultiSignPrefix, result)
	})
}

func TestStartMultiSigningData(t *testing.T) {
	// StartMultiSigningData is an alias for BuildMultiSigningData
	txData := []byte{0xAA, 0xBB, 0xCC}

	result1 := StartMultiSigningData(txData)
	result2 := BuildMultiSigningData(txData)

	assert.Equal(t, result1, result2)
}

func TestFinishMultiSigningData(t *testing.T) {
	t.Run("Appends account ID to signing data", func(t *testing.T) {
		signingData := []byte{0x53, 0x4D, 0x54, 0x00, 0x01, 0x02, 0x03}
		var accountID [AccountIDSize]byte
		for i := range accountID {
			accountID[i] = byte(i + 0x10)
		}

		result := FinishMultiSigningData(signingData, accountID)

		// Should start with signing data
		assert.True(t, bytes.HasPrefix(result, signingData))
		// Should end with account ID
		assert.True(t, bytes.HasSuffix(result, accountID[:]))
		// Total length
		assert.Equal(t, len(signingData)+AccountIDSize, len(result))
	})

	t.Run("Empty signing data", func(t *testing.T) {
		var accountID [AccountIDSize]byte
		accountID[0] = 0xAB

		result := FinishMultiSigningData(nil, accountID)

		assert.Equal(t, accountID[:], result)
	})
}

func TestFinishMultiSigningDataBytes(t *testing.T) {
	t.Run("Valid account ID bytes", func(t *testing.T) {
		signingData := []byte{0x01, 0x02, 0x03}
		accountID := make([]byte, AccountIDSize)
		accountID[0] = 0xFF

		result := FinishMultiSigningDataBytes(signingData, accountID)
		require.NotNil(t, result)

		assert.True(t, bytes.HasPrefix(result, signingData))
		assert.True(t, bytes.HasSuffix(result, accountID))
	})

	t.Run("Invalid account ID length returns nil", func(t *testing.T) {
		signingData := []byte{0x01, 0x02, 0x03}

		// Too short
		result := FinishMultiSigningDataBytes(signingData, []byte{0x01, 0x02})
		assert.Nil(t, result)

		// Too long
		result = FinishMultiSigningDataBytes(signingData, make([]byte, 21))
		assert.Nil(t, result)

		// Empty
		result = FinishMultiSigningDataBytes(signingData, nil)
		assert.Nil(t, result)
	})
}

func TestBuildCompleteMultiSigningData(t *testing.T) {
	txData := []byte{0x01, 0x02, 0x03}
	var accountID [AccountIDSize]byte
	for i := range accountID {
		accountID[i] = byte(i)
	}

	// Build complete data in one call
	complete := BuildCompleteMultiSigningData(txData, accountID)

	// Build in two steps
	partial := BuildMultiSigningData(txData)
	stepped := FinishMultiSigningData(partial, accountID)

	// Should be equal
	assert.Equal(t, stepped, complete)

	// Verify structure
	assert.Equal(t, len(MultiSignPrefix)+len(txData)+AccountIDSize, len(complete))
	assert.True(t, bytes.HasPrefix(complete, MultiSignPrefix))
	assert.True(t, bytes.HasSuffix(complete, accountID[:]))
}

func TestPrependHashPrefix(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}

	tests := []struct {
		name   string
		prefix HashPrefix
	}{
		{"TransactionID", HashPrefixTransactionID},
		{"TxSign", HashPrefixTxSign},
		{"TxMultiSign", HashPrefixTxMultiSign},
		{"Validation", HashPrefixValidation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PrependHashPrefix(tt.prefix, data)

			// Should be 4 bytes prefix + original data
			assert.Equal(t, 4+len(data), len(result))

			// Extract the prefix
			extractedPrefix := binary.BigEndian.Uint32(result[:4])
			assert.Equal(t, uint32(tt.prefix), extractedPrefix)

			// Original data should follow
			assert.Equal(t, data, result[4:])
		})
	}
}

func TestHashPrefixValues(t *testing.T) {
	// Verify the hash prefix values match the expected ASCII representations
	// Format: first 3 chars + null byte

	// TXN\0
	assert.Equal(t, uint32(0x54584E00), uint32(HashPrefixTransactionID))

	// SMT\0
	assert.Equal(t, uint32(0x534D5400), uint32(HashPrefixTxMultiSign))

	// STX\0
	assert.Equal(t, uint32(0x53545800), uint32(HashPrefixTxSign))

	// VAL\0
	assert.Equal(t, uint32(0x56414C00), uint32(HashPrefixValidation))

	// LWR\0
	assert.Equal(t, uint32(0x4C575200), uint32(HashPrefixLedgerMaster))

	// MLN\0
	assert.Equal(t, uint32(0x4D4C4E00), uint32(HashPrefixLeafNode))

	// MIN\0
	assert.Equal(t, uint32(0x4D494E00), uint32(HashPrefixInnerNode))

	// SND\0
	assert.Equal(t, uint32(0x534E4400), uint32(HashPrefixTxNode))

	// PRP\0
	assert.Equal(t, uint32(0x50525000), uint32(HashPrefixProposal))

	// MAN\0
	assert.Equal(t, uint32(0x4D414E00), uint32(HashPrefixManifest))

	// CLM\0
	assert.Equal(t, uint32(0x434C4D00), uint32(HashPrefixPaymentChannelClaim))

	// CRD\0
	assert.Equal(t, uint32(0x43524400), uint32(HashPrefixCredential))

	// BCH\0
	assert.Equal(t, uint32(0x42434800), uint32(HashPrefixBatch))
}
