package interfaces

import "github.com/LeJamon/goXRPLd/internal/codec/binary-codec/definitions"

type Definitions interface {
	GetFieldNameByFieldHeader(fh definitions.FieldHeader) (string, error)
	GetFieldInstanceByFieldName(fieldName string) (*definitions.FieldInstance, error)
	GetFieldHeaderByFieldName(fieldName string) (*definitions.FieldHeader, error)
	CreateFieldHeader(typecode, fieldcode int32) definitions.FieldHeader
}
