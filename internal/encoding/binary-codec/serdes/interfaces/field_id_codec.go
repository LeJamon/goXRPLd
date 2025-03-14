package interfaces

type FieldIDCodec interface {
	Encode(fieldName string) ([]byte, error)
	Decode(h string) (string, error)
}
