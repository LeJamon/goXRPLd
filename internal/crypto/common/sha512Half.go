package crypto

import "crypto/sha512"

// Returns the first 32 bytes of a sha512 hash of a message
func Sha512Half(msg []byte) [32]byte {
	h := sha512.Sum512(msg)
	var result [32]byte
	copy(result[:], h[:32])
	return result
}
