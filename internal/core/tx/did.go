package tx

import (
	"encoding/hex"
	"errors"
)

// DID constants matching rippled Protocol.h
const (
	// MaxDIDDocumentLength is the maximum length of a DID document (256 bytes)
	MaxDIDDocumentLength = 256

	// MaxDIDURILength is the maximum length of a DID URI (256 bytes)
	MaxDIDURILength = 256

	// MaxDIDAttestationLength is the maximum length of DID attestation/data (256 bytes)
	MaxDIDAttestationLength = 256
)

// DID validation errors
var (
	ErrDIDEmpty        = errors.New("temEMPTY_DID: DID transaction must have at least one non-empty field")
	ErrDIDURITooLong   = errors.New("temMALFORMED: URI exceeds maximum length of 256 bytes")
	ErrDIDDocTooLong   = errors.New("temMALFORMED: DIDDocument exceeds maximum length of 256 bytes")
	ErrDIDDataTooLong  = errors.New("temMALFORMED: Data exceeds maximum length of 256 bytes")
	ErrDIDInvalidHex   = errors.New("temMALFORMED: field must be valid hex string")
)

// DIDSet creates or updates a DID document.
type DIDSet struct {
	BaseTx

	// Data is the public attestations (optional, hex-encoded)
	Data string `json:"Data,omitempty"`

	// DIDDocument is the DID document content (optional, hex-encoded)
	DIDDocument string `json:"DIDDocument,omitempty"`

	// URI is the URI for the DID document (optional, hex-encoded)
	URI string `json:"URI,omitempty"`
}

// NewDIDSet creates a new DIDSet transaction
func NewDIDSet(account string) *DIDSet {
	return &DIDSet{
		BaseTx: *NewBaseTx(TypeDIDSet, account),
	}
}

// TxType returns the transaction type
func (d *DIDSet) TxType() Type {
	return TypeDIDSet
}

// Validate validates the DIDSet transaction.
// Implements the preflight checks from rippled DID.cpp DIDSet::preflight()
func (d *DIDSet) Validate() error {
	if err := d.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	if d.Common.Flags != nil && *d.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// Check: at least one field must be present
	// Reference: rippled DID.cpp:57-59
	uriPresent := d.URI != ""
	docPresent := d.DIDDocument != ""
	dataPresent := d.Data != ""

	if !uriPresent && !docPresent && !dataPresent {
		return ErrDIDEmpty
	}

	// Check: not all fields can be empty strings
	// Reference: rippled DID.cpp:61-64
	// If all fields are present but all are empty strings, that's also an error
	// (This handles the case where fields are explicitly set to "")
	if uriPresent && d.URI == "" &&
		docPresent && d.DIDDocument == "" &&
		dataPresent && d.Data == "" {
		return ErrDIDEmpty
	}

	// Validate field lengths (fields are hex-encoded in transactions)
	// Reference: rippled DID.cpp:66-75
	if d.URI != "" {
		decoded, err := hex.DecodeString(d.URI)
		if err != nil {
			return ErrDIDInvalidHex
		}
		if len(decoded) > MaxDIDURILength {
			return ErrDIDURITooLong
		}
	}

	if d.DIDDocument != "" {
		decoded, err := hex.DecodeString(d.DIDDocument)
		if err != nil {
			return ErrDIDInvalidHex
		}
		if len(decoded) > MaxDIDDocumentLength {
			return ErrDIDDocTooLong
		}
	}

	if d.Data != "" {
		decoded, err := hex.DecodeString(d.Data)
		if err != nil {
			return ErrDIDInvalidHex
		}
		if len(decoded) > MaxDIDAttestationLength {
			return ErrDIDDataTooLong
		}
	}

	return nil
}

// HasURI returns true if the URI field is present (even if empty string)
func (d *DIDSet) HasURI() bool {
	return d.URI != ""
}

// HasDIDDocument returns true if the DIDDocument field is present (even if empty string)
func (d *DIDSet) HasDIDDocument() bool {
	return d.DIDDocument != ""
}

// HasData returns true if the Data field is present (even if empty string)
func (d *DIDSet) HasData() bool {
	return d.Data != ""
}

// Flatten returns a flat map of all transaction fields
func (d *DIDSet) Flatten() (map[string]any, error) {
	m := d.Common.ToMap()

	if d.Data != "" {
		m["Data"] = d.Data
	}
	if d.DIDDocument != "" {
		m["DIDDocument"] = d.DIDDocument
	}
	if d.URI != "" {
		m["URI"] = d.URI
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (d *DIDSet) RequiredAmendments() []string {
	return []string{AmendmentDID}
}

// DIDDelete deletes a DID document.
type DIDDelete struct {
	BaseTx
}

// NewDIDDelete creates a new DIDDelete transaction
func NewDIDDelete(account string) *DIDDelete {
	return &DIDDelete{
		BaseTx: *NewBaseTx(TypeDIDDelete, account),
	}
}

// TxType returns the transaction type
func (d *DIDDelete) TxType() Type {
	return TypeDIDDelete
}

// Validate validates the DIDDelete transaction.
// Implements the preflight checks from rippled DID.cpp DIDDelete::preflight()
func (d *DIDDelete) Validate() error {
	if err := d.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	if d.Common.Flags != nil && *d.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (d *DIDDelete) Flatten() (map[string]any, error) {
	return d.Common.ToMap(), nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (d *DIDDelete) RequiredAmendments() []string {
	return []string{AmendmentDID}
}
