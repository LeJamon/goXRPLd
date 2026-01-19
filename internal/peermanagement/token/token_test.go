package token

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test vectors from rippled PublicKey_test.cpp testBase58
var base58TestVectors = []struct {
	name       string
	publicKey  string // hex-encoded compressed public key
	encoded    string // expected base58 encoding
}{
	{
		name:      "masterpassphrase_secp256k1",
		publicKey: "0330E7FC9D56BB25D6893BA3F317AE5BCF33B3291BD63DB32654A313222F7FD020",
		// Reference: rippled PublicKey_test.cpp - n94a1u4jAz288pZLtw6yFWVbi89YamiC6JBXPVUj5zmExe5fTVg9
	},
}

// TestNewPublicKey tests creating a PublicKey from raw bytes
// Reference: rippled PublicKey_test.cpp testBase58
func TestNewPublicKey(t *testing.T) {
	for _, tv := range base58TestVectors {
		t.Run(tv.name, func(t *testing.T) {
			pubKeyBytes, err := hex.DecodeString(tv.publicKey)
			require.NoError(t, err)

			pk, err := NewPublicKey(pubKeyBytes)
			require.NoError(t, err)
			require.NotNil(t, pk)

			// Round-trip: bytes should match
			assert.Equal(t, pubKeyBytes, pk.Bytes())
		})
	}
}

// TestNewPublicKey_Invalid tests invalid public key inputs
// Reference: rippled PublicKey_test.cpp testBase58 - short strings test
func TestNewPublicKey_Invalid(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"too_short", make([]byte, 32)},
		{"too_long", make([]byte, 34)},
		{"invalid_prefix", []byte{0x04, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
			0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
			0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
			0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPublicKey(tt.data)
			assert.Error(t, err)
		})
	}
}

// TestParsePublicKey_Empty tests empty string parsing
// Reference: rippled PublicKey_test.cpp - parseBase58<PublicKey>(TokenType::NodePublic, "")
func TestParsePublicKey_Empty(t *testing.T) {
	_, err := ParsePublicKey("")
	assert.Error(t, err)
}

// TestParsePublicKey_Whitespace tests whitespace-only string
// Reference: rippled PublicKey_test.cpp - parseBase58<PublicKey>(TokenType::NodePublic, " ")
func TestParsePublicKey_Whitespace(t *testing.T) {
	_, err := ParsePublicKey(" ")
	assert.Error(t, err)
}

// TestParsePublicKey_Malformed tests malformed input
// Reference: rippled PublicKey_test.cpp - parseBase58<PublicKey>(TokenType::NodePublic, "!ty89234gh45")
func TestParsePublicKey_Malformed(t *testing.T) {
	_, err := ParsePublicKey("!ty89234gh45")
	assert.Error(t, err)
}

// TestParsePublicKey_InvalidBase58Chars tests strings with invalid Base58 characters
// Reference: rippled PublicKey_test.cpp - Strings with invalid Base58 characters (0, I, O, l)
func TestParsePublicKey_InvalidBase58Chars(t *testing.T) {
	// These characters are not valid in Base58: 0, I, O, l
	invalidChars := []string{"0", "I", "O", "l"}

	for _, c := range invalidChars {
		t.Run("char_"+c, func(t *testing.T) {
			// Create a string that looks like it could be a key but has invalid char
			invalidKey := "n" + c + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			_, err := ParsePublicKey(invalidKey)
			assert.Error(t, err)
		})
	}
}

// TestParsePublicKey_WrongPrefix tests strings with incorrect prefix
// Reference: rippled PublicKey_test.cpp - Strings with incorrect prefix (apsrJqtv7)
func TestParsePublicKey_WrongPrefix(t *testing.T) {
	wrongPrefixes := []string{"a", "p", "s", "r", "J", "q", "t", "v", "7"}

	for _, prefix := range wrongPrefixes {
		t.Run("prefix_"+prefix, func(t *testing.T) {
			// Generate a valid key first, then change prefix
			privKey, _ := btcec.NewPrivateKey()
			pk := NewPublicKeyFromBtcec(privKey.PubKey())
			encoded := pk.Encode()

			// Replace first character with wrong prefix
			wrongEncoded := prefix + encoded[1:]
			_, err := ParsePublicKey(wrongEncoded)
			assert.Error(t, err, "should reject prefix %s", prefix)
		})
	}
}

// TestEncode tests encoding a public key to Base58
// Reference: rippled PublicKey_test.cpp testBase58 - toBase58(TokenType::NodePublic, ...)
func TestEncode(t *testing.T) {
	for _, tv := range base58TestVectors {
		t.Run(tv.name, func(t *testing.T) {
			pubKeyBytes, err := hex.DecodeString(tv.publicKey)
			require.NoError(t, err)

			pk, err := NewPublicKey(pubKeyBytes)
			require.NoError(t, err)

			encoded := pk.Encode()

			// Must start with 'n' (node public key prefix)
			assert.True(t, strings.HasPrefix(encoded, "n"),
				"encoded key should start with 'n', got: %s", encoded)

			// Should be non-empty
			assert.NotEmpty(t, encoded)

			t.Logf("Public key %s encodes to: %s", tv.name, encoded)
		})
	}
}

// TestEncodeDecodeRoundTrip tests round-trip encoding and decoding
// Reference: rippled PublicKey_test.cpp testBase58 - Try some random secret keys
func TestEncodeDecodeRoundTrip(t *testing.T) {
	// Generate multiple random keys and verify round-trip
	for i := 0; i < 32; i++ {
		t.Run("key_"+string(rune('A'+i)), func(t *testing.T) {
			// Generate random key
			privKey, err := btcec.NewPrivateKey()
			require.NoError(t, err)

			pk := NewPublicKeyFromBtcec(privKey.PubKey())

			// Encode
			encoded := pk.Encode()
			assert.NotEmpty(t, encoded)
			assert.True(t, strings.HasPrefix(encoded, "n"))

			// Decode
			decoded, err := ParsePublicKey(encoded)
			require.NoError(t, err)

			// Should be equal
			assert.True(t, pk.Equal(decoded),
				"round-trip failed: original != decoded")

			// Bytes should match
			assert.Equal(t, pk.Bytes(), decoded.Bytes())
		})
	}
}

// TestParsePublicKey_ShortStrings tests progressively shorter strings
// Reference: rippled PublicKey_test.cpp - Short (non-empty) strings test
func TestParsePublicKey_ShortStrings(t *testing.T) {
	// Generate a valid encoded key
	privKey, _ := btcec.NewPrivateKey()
	pk := NewPublicKeyFromBtcec(privKey.PubKey())
	valid := pk.Encode()

	// Try progressively shorter strings (all should fail)
	for length := len(valid) - 1; length > 0; length-- {
		short := valid[:length]
		_, err := ParsePublicKey(short)
		assert.Error(t, err, "should reject shortened key of length %d", length)
	}
}

// TestParsePublicKey_LongStrings tests strings longer than valid
// Reference: rippled PublicKey_test.cpp - Long strings test
func TestParsePublicKey_LongStrings(t *testing.T) {
	// Generate a valid encoded key
	privKey, _ := btcec.NewPrivateKey()
	pk := NewPublicKeyFromBtcec(privKey.PubKey())
	valid := pk.Encode()

	// Try progressively longer strings (all should fail)
	for i := 1; i <= 16; i++ {
		// Append characters from the valid string
		long := valid + valid[:i]
		_, err := ParsePublicKey(long)
		assert.Error(t, err, "should reject extended key with %d extra chars", i)
	}
}

// TestPublicKeyEquality tests the Equal method
// Reference: rippled PublicKey_test.cpp testMiscOperations
func TestPublicKeyEquality(t *testing.T) {
	// Generate two different keys
	privKey1, _ := btcec.NewPrivateKey()
	privKey2, _ := btcec.NewPrivateKey()

	pk1 := NewPublicKeyFromBtcec(privKey1.PubKey())
	pk2 := NewPublicKeyFromBtcec(privKey2.PubKey())

	// Same key should be equal to itself
	assert.True(t, pk1.Equal(pk1))
	assert.True(t, pk2.Equal(pk2))

	// Different keys should not be equal
	assert.False(t, pk1.Equal(pk2))
	assert.False(t, pk2.Equal(pk1))

	// Copy should be equal
	pk1Copy, _ := NewPublicKey(pk1.Bytes())
	assert.True(t, pk1.Equal(pk1Copy))
}

// TestPublicKeyEquality_Nil tests Equal with nil values
func TestPublicKeyEquality_Nil(t *testing.T) {
	privKey, _ := btcec.NewPrivateKey()
	pk := NewPublicKeyFromBtcec(privKey.PubKey())

	assert.False(t, pk.Equal(nil))

	var nilPk *PublicKey
	assert.True(t, nilPk.Equal(nil))
}

// TestBtcecKey tests accessing the underlying btcec key
func TestBtcecKey(t *testing.T) {
	privKey, _ := btcec.NewPrivateKey()
	original := privKey.PubKey()

	pk := NewPublicKeyFromBtcec(original)
	retrieved := pk.BtcecKey()

	assert.True(t, original.IsEqual(retrieved))
}

// TestChecksumValidation tests that invalid checksums are rejected
func TestChecksumValidation(t *testing.T) {
	// Generate a valid encoded key
	privKey, _ := btcec.NewPrivateKey()
	pk := NewPublicKeyFromBtcec(privKey.PubKey())
	valid := pk.Encode()

	// Corrupt the last character (part of checksum)
	corrupted := valid[:len(valid)-1] + "X"
	_, err := ParsePublicKey(corrupted)
	assert.Error(t, err)
}

// TestNewPublicKeyFromBtcec tests creating from btcec.PublicKey
func TestNewPublicKeyFromBtcec(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	btcecPk := privKey.PubKey()
	pk := NewPublicKeyFromBtcec(btcecPk)

	assert.NotNil(t, pk)
	assert.Equal(t, btcecPk.SerializeCompressed(), pk.Bytes())
}
