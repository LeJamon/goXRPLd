package interfaces

import "github.com/LeJamon/goXRPLd/internal/codec/binary-codec/definitions"

// BinaryParser is an interface that defines the methods for a binary parser.
type BinaryParser interface {
	ReadByte() (byte, error)
	ReadField() (*definitions.FieldInstance, error)
	Peek() (byte, error)
	ReadBytes(n int) ([]byte, error)
	HasMore() bool
	ReadVariableLength() (int, error)
}
