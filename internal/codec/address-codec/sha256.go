package addresscodec

import (
	"crypto/sha256"

	"github.com/decred/dcrd/crypto/ripemd160"
)

// Returns byte slice of a double hashed given byte slice.
// The given byte slice is SHA256 hashed, then the result is RIPEMD160 hashed.
func Sha256RipeMD160(b []byte) []byte {
	sha256 := sha256.New()
	sha256.Write(b)

	ripemd160 := ripemd160.New()
	ripemd160.Write(sha256.Sum(nil))

	return ripemd160.Sum(nil)
}
