package crypto

import "crypto/sha512"

// Returns the first 32 bytes of a sha512 hash of a message
func Sha512Half(msg []byte) []byte {
	h := sha512.Sum512(msg)
	return h[:32]
}
