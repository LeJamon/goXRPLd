package shamap

import (
	"github.com/LeJamon/goXRPLd/internal/crypto/common"
)

type Hasher interface {
	Hash(data []byte) [32]byte
}

type Sha512HalfHasher struct{}

func (h Sha512HalfHasher) Hash(data []byte) [32]byte {
	return crypto.Sha512Half(data)
}
