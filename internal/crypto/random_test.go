package crypto

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomBytes(t *testing.T) {
	t.Run("Generates correct length", func(t *testing.T) {
		for _, n := range []int{1, 16, 32, 64, 128} {
			b, err := RandomBytes(n)
			require.NoError(t, err)
			assert.Equal(t, n, len(b))
		}
	})

	t.Run("Zero length returns nil", func(t *testing.T) {
		b, err := RandomBytes(0)
		require.NoError(t, err)
		assert.Nil(t, b)
	})

	t.Run("Negative length returns nil", func(t *testing.T) {
		b, err := RandomBytes(-1)
		require.NoError(t, err)
		assert.Nil(t, b)
	})

	t.Run("Generates different values", func(t *testing.T) {
		b1, err := RandomBytes(32)
		require.NoError(t, err)
		b2, err := RandomBytes(32)
		require.NoError(t, err)

		// Extremely unlikely to be equal
		assert.False(t, bytes.Equal(b1, b2))
	})
}

func TestRandomSecretKey(t *testing.T) {
	t.Run("Secp256k1 key", func(t *testing.T) {
		sk, err := RandomSecretKey(KeyTypeSecp256k1)
		require.NoError(t, err)
		require.NotNil(t, sk)
		defer sk.Close()

		assert.Equal(t, SecretKeySecp256k1Size, sk.Len())
		assert.False(t, sk.IsClosed())
	})

	t.Run("Ed25519 key", func(t *testing.T) {
		sk, err := RandomSecretKey(KeyTypeEd25519)
		require.NoError(t, err)
		require.NotNil(t, sk)
		defer sk.Close()

		assert.Equal(t, SecretKeyEd25519Size, sk.Len())
		assert.False(t, sk.IsClosed())
	})

	t.Run("Unknown key type returns error", func(t *testing.T) {
		sk, err := RandomSecretKey(KeyTypeUnknown)
		assert.Error(t, err)
		assert.Equal(t, ErrUnsupportedKeyType, err)
		assert.Nil(t, sk)
	})

	t.Run("Generates different keys", func(t *testing.T) {
		sk1, err := RandomSecretKey(KeyTypeSecp256k1)
		require.NoError(t, err)
		defer sk1.Close()

		sk2, err := RandomSecretKey(KeyTypeSecp256k1)
		require.NoError(t, err)
		defer sk2.Close()

		assert.False(t, bytes.Equal(sk1.Data(), sk2.Data()))
	})
}

func TestRandomKeyPair(t *testing.T) {
	t.Run("Secp256k1 key pair", func(t *testing.T) {
		pub, priv, err := RandomKeyPair(KeyTypeSecp256k1)
		require.NoError(t, err)

		// Public key should be 33 bytes (compressed secp256k1)
		assert.Equal(t, 33, len(pub))
		// Should have valid prefix (0x02 or 0x03)
		assert.True(t, pub[0] == 0x02 || pub[0] == 0x03)

		// Private key should be 33 bytes (0x00 prefix + 32 bytes)
		assert.Equal(t, SecretKeySecp256k1WithPrefixSize, len(priv))
		assert.Equal(t, byte(0x00), priv[0])

		// Detect as secp256k1
		assert.Equal(t, KeyTypeSecp256k1, PublicKeyType(pub))
	})

	t.Run("Ed25519 key pair", func(t *testing.T) {
		pub, priv, err := RandomKeyPair(KeyTypeEd25519)
		require.NoError(t, err)

		// Public key should be 33 bytes (0xED prefix + 32 bytes)
		assert.Equal(t, 33, len(pub))
		assert.Equal(t, byte(0xED), pub[0])

		// Private key should be 33 bytes (0xED prefix + 32 byte seed)
		assert.Equal(t, SecretKeyEd25519WithPrefixSize, len(priv))
		assert.Equal(t, byte(0xED), priv[0])

		// Detect as Ed25519
		assert.Equal(t, KeyTypeEd25519, PublicKeyType(pub))
	})

	t.Run("Unknown key type returns error", func(t *testing.T) {
		pub, priv, err := RandomKeyPair(KeyTypeUnknown)
		assert.Error(t, err)
		assert.Equal(t, ErrUnsupportedKeyType, err)
		assert.Nil(t, pub)
		assert.Nil(t, priv)
	})

	t.Run("Generates different key pairs", func(t *testing.T) {
		pub1, priv1, err := RandomKeyPair(KeyTypeSecp256k1)
		require.NoError(t, err)

		pub2, priv2, err := RandomKeyPair(KeyTypeSecp256k1)
		require.NoError(t, err)

		assert.False(t, bytes.Equal(pub1, pub2))
		assert.False(t, bytes.Equal(priv1, priv2))
	})
}

func TestRandomSeed(t *testing.T) {
	t.Run("Generates 16 byte seed", func(t *testing.T) {
		seed, err := RandomSeed()
		require.NoError(t, err)
		assert.Equal(t, 16, len(seed))
	})

	t.Run("Generates different seeds", func(t *testing.T) {
		seed1, err := RandomSeed()
		require.NoError(t, err)

		seed2, err := RandomSeed()
		require.NoError(t, err)

		assert.False(t, bytes.Equal(seed1, seed2))
	})
}
