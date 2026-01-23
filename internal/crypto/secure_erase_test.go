package crypto

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecureErase(t *testing.T) {
	t.Run("Erases data", func(t *testing.T) {
		data := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
		original := make([]byte, len(data))
		copy(original, data)

		SecureErase(data)

		// All bytes should be zero
		assert.True(t, bytes.Equal(data, make([]byte, len(data))))
		// Should have been modified
		assert.False(t, bytes.Equal(data, original))
	})

	t.Run("Handles empty slice", func(t *testing.T) {
		// Should not panic
		SecureErase([]byte{})
		SecureErase(nil)
	})

	t.Run("Erases large buffer", func(t *testing.T) {
		data := make([]byte, 1024)
		for i := range data {
			data[i] = byte(i % 256)
		}

		SecureErase(data)

		for i := range data {
			assert.Equal(t, byte(0), data[i])
		}
	})
}

func TestSecretKey(t *testing.T) {
	t.Run("NewSecretKey", func(t *testing.T) {
		data := []byte{0x01, 0x02, 0x03, 0x04}
		sk := NewSecretKey(data)
		require.NotNil(t, sk)
		assert.Equal(t, data, sk.Data())
		assert.Equal(t, 4, sk.Len())
		assert.False(t, sk.IsClosed())
	})

	t.Run("NewSecretKeyWithCopy", func(t *testing.T) {
		data := []byte{0x01, 0x02, 0x03, 0x04}
		sk := NewSecretKeyWithCopy(data)
		require.NotNil(t, sk)

		// Modify original data
		data[0] = 0xFF

		// SecretKey should have its own copy
		assert.NotEqual(t, data[0], sk.Data()[0])
		assert.Equal(t, byte(0x01), sk.Data()[0])
	})

	t.Run("Close erases data", func(t *testing.T) {
		data := []byte{0x01, 0x02, 0x03, 0x04}
		sk := NewSecretKey(data)

		sk.Close()

		// Data should be erased
		assert.True(t, sk.IsClosed())
		assert.Nil(t, sk.Data())
		assert.Equal(t, 0, sk.Len())

		// Original slice should be zeroed
		assert.True(t, bytes.Equal(data, make([]byte, len(data))))
	})

	t.Run("Close is idempotent", func(t *testing.T) {
		sk := NewSecretKey([]byte{0x01, 0x02})

		sk.Close()
		sk.Close()
		sk.Close()

		assert.True(t, sk.IsClosed())
	})

	t.Run("Copy returns new slice", func(t *testing.T) {
		data := []byte{0x01, 0x02, 0x03, 0x04}
		sk := NewSecretKey(data)

		copied := sk.Copy()
		require.NotNil(t, copied)
		assert.Equal(t, data, copied)

		// Modifying copy should not affect original
		copied[0] = 0xFF
		assert.NotEqual(t, copied[0], sk.Data()[0])
	})

	t.Run("Copy returns nil after close", func(t *testing.T) {
		sk := NewSecretKey([]byte{0x01, 0x02})
		sk.Close()

		assert.Nil(t, sk.Copy())
	})

	t.Run("Nil SecretKey handling", func(t *testing.T) {
		var sk *SecretKey

		assert.Nil(t, sk.Data())
		assert.Equal(t, 0, sk.Len())
		assert.True(t, sk.IsClosed())
		assert.Nil(t, sk.Copy())

		// Close should not panic
		sk.Close()
	})
}

func TestSecretKeyConstants(t *testing.T) {
	assert.Equal(t, 32, SecretKeySecp256k1Size)
	assert.Equal(t, 32, SecretKeyEd25519Size)
	assert.Equal(t, 33, SecretKeySecp256k1WithPrefixSize)
	assert.Equal(t, 33, SecretKeyEd25519WithPrefixSize)
}
