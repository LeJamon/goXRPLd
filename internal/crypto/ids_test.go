package crypto

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalcAccountID(t *testing.T) {
	// These test vectors are derived from known XRPL accounts
	tests := []struct {
		name      string
		publicKey string
		accountID string
	}{
		{
			name: "Ed25519 public key",
			// This is a well-known test vector
			publicKey: "ED9434799226374926EDA3B54B1B461B4ABF7237962EAE18528FEA67595397FA32",
			accountID: "7f58b19358f8e497c8a9ded3e6db3bc23a13c1a5",
		},
		{
			name: "Secp256k1 public key",
			// Another test vector
			publicKey: "0330E7FC9D56BB25D6893BA3F317AE5BCF33B3291BD63DB32654A313222F7FD020",
			accountID: "b5f762798a53d543a014caf8b297cff8f2f937e8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pubKey, err := hex.DecodeString(tt.publicKey)
			require.NoError(t, err)

			accountID := CalcAccountID(pubKey)

			expectedID, err := hex.DecodeString(tt.accountID)
			require.NoError(t, err)

			assert.Equal(t, expectedID, accountID[:])
		})
	}
}

func TestCalcNodeID(t *testing.T) {
	// NodeID uses the same computation as AccountID
	publicKey, _ := hex.DecodeString("ED9434799226374926EDA3B54B1B461B4ABF7237962EAE18528FEA67595397FA32")

	accountID := CalcAccountID(publicKey)
	nodeID := CalcNodeID(publicKey)

	assert.Equal(t, accountID, nodeID)
}

func TestAccountIDFromBytes(t *testing.T) {
	t.Run("Valid 20 byte input", func(t *testing.T) {
		input := make([]byte, 20)
		for i := range input {
			input[i] = byte(i)
		}

		result := AccountIDFromBytes(input)
		assert.Equal(t, input, result[:])
	})

	t.Run("Wrong length returns zero", func(t *testing.T) {
		shortInput := []byte{0x01, 0x02, 0x03}
		result := AccountIDFromBytes(shortInput)
		assert.True(t, IsZeroAccountID(result))
	})

	t.Run("Empty input returns zero", func(t *testing.T) {
		result := AccountIDFromBytes(nil)
		assert.True(t, IsZeroAccountID(result))
	})
}

func TestIsZeroAccountID(t *testing.T) {
	t.Run("Zero account ID", func(t *testing.T) {
		var zeroID [AccountIDSize]byte
		assert.True(t, IsZeroAccountID(zeroID))
	})

	t.Run("Non-zero account ID", func(t *testing.T) {
		id := [AccountIDSize]byte{0x01}
		assert.False(t, IsZeroAccountID(id))
	})

	t.Run("Account ID with last byte non-zero", func(t *testing.T) {
		var id [AccountIDSize]byte
		id[AccountIDSize-1] = 0x01
		assert.False(t, IsZeroAccountID(id))
	})
}

func TestAccountIDToBytes(t *testing.T) {
	var id [AccountIDSize]byte
	for i := range id {
		id[i] = byte(i)
	}

	result := AccountIDToBytes(id)
	assert.Equal(t, AccountIDSize, len(result))
	assert.Equal(t, id[:], result)

	// Verify it's a copy
	result[0] = 0xFF
	assert.NotEqual(t, id[0], result[0])
}

func TestAccountIDConstants(t *testing.T) {
	assert.Equal(t, 20, AccountIDSize)
	assert.Equal(t, 20, NodeIDSize)
}

func TestCalcAccountID_Deterministic(t *testing.T) {
	// The same public key should always produce the same account ID
	publicKey, _ := hex.DecodeString("0330E7FC9D56BB25D6893BA3F317AE5BCF33B3291BD63DB32654A313222F7FD020")

	id1 := CalcAccountID(publicKey)
	id2 := CalcAccountID(publicKey)
	id3 := CalcAccountID(publicKey)

	assert.Equal(t, id1, id2)
	assert.Equal(t, id2, id3)
}
