package crypto

import (
	"crypto/sha256"

	"github.com/decred/dcrd/crypto/ripemd160"
)

// AccountIDSize is the size of an XRPL account ID in bytes.
const AccountIDSize = 20

// NodeIDSize is the size of an XRPL node ID in bytes.
const NodeIDSize = 20

// CalcAccountID computes the account ID from a public key.
// The account ID is a 160-bit identifier computed as RIPEMD160(SHA256(publicKey)).
//
// This follows Bitcoin's approach for address derivation to avoid:
//   - Length extension attacks (by using two different hashes)
//   - The only hash generally considered safe at 160 bits is RIPEMD160
//
// The same computation is used regardless of the cryptographic scheme
// (secp256k1 or Ed25519) - the entire public key including any prefix
// is hashed.
//
// See the rippled AccountID.cpp for the authoritative reference.
func CalcAccountID(publicKey []byte) [AccountIDSize]byte {
	// First compute SHA256
	sha256Hash := sha256.Sum256(publicKey)

	// Then RIPEMD160 of the SHA256 hash
	ripemd160Hasher := ripemd160.New()
	ripemd160Hasher.Write(sha256Hash[:])
	ripemd160Hash := ripemd160Hasher.Sum(nil)

	var result [AccountIDSize]byte
	copy(result[:], ripemd160Hash)
	return result
}

// CalcNodeID computes the node ID from a public key.
// Node IDs use the same computation as account IDs: RIPEMD160(SHA256(publicKey)).
//
// Node IDs identify nodes in the XRPL peer-to-peer network.
func CalcNodeID(publicKey []byte) [NodeIDSize]byte {
	return CalcAccountID(publicKey)
}

// AccountIDFromBytes creates an account ID from a byte slice.
// Returns a zero account ID if the slice is not exactly 20 bytes.
func AccountIDFromBytes(b []byte) [AccountIDSize]byte {
	var result [AccountIDSize]byte
	if len(b) == AccountIDSize {
		copy(result[:], b)
	}
	return result
}

// IsZeroAccountID returns true if the account ID is all zeros.
// The zero account ID represents XRP itself (the native currency).
func IsZeroAccountID(id [AccountIDSize]byte) bool {
	for _, b := range id {
		if b != 0 {
			return false
		}
	}
	return true
}

// AccountIDToBytes converts an account ID to a byte slice.
func AccountIDToBytes(id [AccountIDSize]byte) []byte {
	result := make([]byte, AccountIDSize)
	copy(result, id[:])
	return result
}
