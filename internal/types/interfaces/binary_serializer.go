package interfaces

import "github.com/LeJamon/goXRPLd/internal/binary-codec/definitions"

// BinarySerializer is an interface that defines the methods for a binary serializer.
type BinarySerializer interface {
	WriteFieldAndValue(fieldInstance definitions.FieldInstance, value []byte) error
	GetSink() []byte
}
