// Package token handles XRPL node public key encoding and decoding.
// XRPL uses Base58Check encoding with specific prefixes for different key types.
package token

import (
	"crypto/sha256"
	"errors"

	"github.com/btcsuite/btcd/btcec/v2"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
)

const (
	// NodePublicKeyPrefix is the prefix byte for XRPL node public keys (results in 'n' prefix)
	NodePublicKeyPrefix = 0x1C // 28 decimal

	// AccountPublicKeyPrefix is for account public keys (results in 'a' prefix)
	AccountPublicKeyPrefix = 0x23 // 35 decimal

	// CompressedPubKeyLen is the length of a compressed secp256k1 public key
	CompressedPubKeyLen = 33

	// ChecksumLen is the length of the Base58Check checksum
	ChecksumLen = 4
)

var (
	// ErrInvalidPublicKey is returned when the public key format is invalid
	ErrInvalidPublicKey = errors.New("invalid public key")
	// ErrInvalidChecksum is returned when the checksum doesn't match
	ErrInvalidChecksum = errors.New("invalid checksum")
	// ErrInvalidPrefix is returned when the key prefix is unexpected
	ErrInvalidPrefix = errors.New("invalid key prefix")
)

// PublicKey represents an XRPL node public key.
type PublicKey struct {
	key *btcec.PublicKey
}

// NewPublicKey creates a PublicKey from raw compressed bytes.
func NewPublicKey(data []byte) (*PublicKey, error) {
	if len(data) != CompressedPubKeyLen {
		return nil, ErrInvalidPublicKey
	}

	key, err := btcec.ParsePubKey(data)
	if err != nil {
		return nil, ErrInvalidPublicKey
	}

	return &PublicKey{key: key}, nil
}

// NewPublicKeyFromBtcec creates a PublicKey from a btcec.PublicKey.
func NewPublicKeyFromBtcec(key *btcec.PublicKey) *PublicKey {
	return &PublicKey{key: key}
}

// ParsePublicKey decodes a Base58Check encoded node public key (e.g., "n9K1Z...").
func ParsePublicKey(encoded string) (*PublicKey, error) {
	// Decode Base58
	data := addresscodec.DecodeBase58(encoded)
	if len(data) == 0 {
		return nil, ErrInvalidPublicKey
	}

	// Minimum length: prefix (1) + key (33) + checksum (4) = 38
	if len(data) < 1+CompressedPubKeyLen+ChecksumLen {
		return nil, ErrInvalidPublicKey
	}

	// Split into payload and checksum
	payloadLen := len(data) - ChecksumLen
	payload := data[:payloadLen]
	checksum := data[payloadLen:]

	// Verify checksum
	expectedChecksum := doubleSHA256(payload)[:ChecksumLen]
	if !equalBytes(checksum, expectedChecksum) {
		return nil, ErrInvalidChecksum
	}

	// Check prefix
	if payload[0] != NodePublicKeyPrefix {
		return nil, ErrInvalidPrefix
	}

	// Extract public key bytes
	keyBytes := payload[1:]
	return NewPublicKey(keyBytes)
}

// Bytes returns the raw compressed public key bytes.
func (p *PublicKey) Bytes() []byte {
	return p.key.SerializeCompressed()
}

// Encode returns the Base58Check encoded public key with node prefix.
func (p *PublicKey) Encode() string {
	keyBytes := p.key.SerializeCompressed()

	// Create payload with prefix
	payload := make([]byte, 1+len(keyBytes))
	payload[0] = NodePublicKeyPrefix
	copy(payload[1:], keyBytes)

	// Calculate checksum
	checksum := doubleSHA256(payload)[:ChecksumLen]

	// Combine and encode
	full := append(payload, checksum...)
	return addresscodec.EncodeBase58(full)
}

// BtcecKey returns the underlying btcec public key.
func (p *PublicKey) BtcecKey() *btcec.PublicKey {
	return p.key
}

// Equal returns true if two public keys are equal.
func (p *PublicKey) Equal(other *PublicKey) bool {
	if p == nil || other == nil {
		return p == other
	}
	return p.key.IsEqual(other.key)
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

// equalBytes returns true if two byte slices are equal.
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
