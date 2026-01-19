// Package identity manages node identity for XRPL peer-to-peer networking.
// The identity consists of a secp256k1 keypair used for session signing
// and peer authentication.
package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
)

var (
	// ErrInvalidPrivateKey is returned when the private key is invalid
	ErrInvalidPrivateKey = errors.New("invalid private key")
	// ErrInvalidPublicKey is returned when the public key is invalid
	ErrInvalidPublicKey = errors.New("invalid public key")
	// ErrSignatureFailed is returned when signing fails
	ErrSignatureFailed = errors.New("failed to sign message")
)

// Identity represents a node's cryptographic identity for peer authentication.
type Identity struct {
	privateKey *btcec.PrivateKey
	publicKey  *btcec.PublicKey
}

// NewIdentity creates a new random identity.
func NewIdentity() (*Identity, error) {
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	return &Identity{
		privateKey: privateKey,
		publicKey:  privateKey.PubKey(),
	}, nil
}

// NewIdentityFromSeed creates an identity from a 16-byte seed.
func NewIdentityFromSeed(seed []byte) (*Identity, error) {
	if len(seed) < 16 {
		return nil, errors.New("seed must be at least 16 bytes")
	}

	// Derive private key from seed using SHA-512
	h := sha512.New()
	h.Write(seed)
	hash := h.Sum(nil)

	privateKey, _ := btcec.PrivKeyFromBytes(hash[:32])

	return &Identity{
		privateKey: privateKey,
		publicKey:  privateKey.PubKey(),
	}, nil
}

// NewIdentityFromPrivateKey creates an identity from a hex-encoded private key.
func NewIdentityFromPrivateKey(privKeyHex string) (*Identity, error) {
	// Must have content
	if len(privKeyHex) == 0 {
		return nil, ErrInvalidPrivateKey
	}

	// Remove 00 prefix if present
	if len(privKeyHex) == 66 && privKeyHex[:2] == "00" {
		privKeyHex = privKeyHex[2:]
	}

	// Must be exactly 64 hex chars (32 bytes) after removing prefix
	if len(privKeyHex) != 64 {
		return nil, ErrInvalidPrivateKey
	}

	privKeyBytes, err := hex.DecodeString(privKeyHex)
	if err != nil {
		return nil, ErrInvalidPrivateKey
	}

	privateKey, _ := btcec.PrivKeyFromBytes(privKeyBytes)
	if privateKey == nil {
		return nil, ErrInvalidPrivateKey
	}

	return &Identity{
		privateKey: privateKey,
		publicKey:  privateKey.PubKey(),
	}, nil
}

// GenerateSeed generates a random 16-byte seed for identity creation.
func GenerateSeed() ([]byte, error) {
	seed := make([]byte, 16)
	_, err := rand.Read(seed)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random seed: %w", err)
	}
	return seed, nil
}

// Sign signs a message (typically the TLS shared value) with the identity's private key.
// The signature is in DER format suitable for XRPL peer authentication.
func (i *Identity) Sign(message []byte) ([]byte, error) {
	if i.privateKey == nil {
		return nil, ErrInvalidPrivateKey
	}

	// Hash the message with SHA-512 and take first 32 bytes (SHA-512 Half)
	h := sha512.New()
	h.Write(message)
	hash := h.Sum(nil)[:32]

	// Sign the hash
	sig := ecdsa.Sign(i.privateKey, hash)
	if sig == nil {
		return nil, ErrSignatureFailed
	}

	return sig.Serialize(), nil
}

// PublicKey returns the raw compressed public key bytes.
func (i *Identity) PublicKey() []byte {
	return i.publicKey.SerializeCompressed()
}

// PublicKeyHex returns the public key as a hex string.
func (i *Identity) PublicKeyHex() string {
	return hex.EncodeToString(i.publicKey.SerializeCompressed())
}

// EncodedPublicKey returns the base58-encoded public key with the node public prefix.
// This is the format used in XRPL peer handshakes (e.g., "n9K1Z...").
func (i *Identity) EncodedPublicKey() string {
	// Node public key prefix for XRPL
	const nodePublicPrefix = 0x1C // 28 decimal, results in 'n' prefix

	pubKeyBytes := i.publicKey.SerializeCompressed()

	// Create payload with prefix
	payload := make([]byte, 1+len(pubKeyBytes))
	payload[0] = nodePublicPrefix
	copy(payload[1:], pubKeyBytes)

	// Calculate checksum (double SHA-256, take first 4 bytes)
	checksum := doubleSHA256(payload)[:4]

	// Combine payload and checksum
	full := append(payload, checksum...)

	return addresscodec.EncodeBase58(full)
}

// PrivateKeyHex returns the private key as a hex string (with 00 prefix).
func (i *Identity) PrivateKeyHex() string {
	return "00" + hex.EncodeToString(i.privateKey.Serialize())
}

// doubleSHA256 computes SHA256(SHA256(data)).
func doubleSHA256(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	first := h.Sum(nil)

	h.Reset()
	h.Write(first)
	return h.Sum(nil)
}
