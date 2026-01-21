package tx

import (
	"encoding/hex"
	"errors"
)

// DID field length constants
// Reference: rippled Protocol.h
const (
	// MaxDIDURILength is the maximum length of the URI field (in bytes after hex decode)
	MaxDIDURILength = 256

	// MaxDIDDocumentLength is the maximum length of the DIDDocument field (in bytes after hex decode)
	MaxDIDDocumentLength = 256

	// MaxDIDAttestationLength is the maximum length of the Data field (in bytes after hex decode)
	MaxDIDAttestationLength = 256
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

// Validate validates the DIDSet transaction
// Reference: rippled DID.cpp DIDSet::preflight
func (d *DIDSet) Validate() error {
	if err := d.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	flags := d.GetFlags()
	if flags&TfUniversalMask != 0 {
		return errors.New("temINVALID_FLAG: Invalid flags set")
	}

	// At least one field must be present
	// Reference: DID.cpp line 57-59
	if d.URI == "" && d.DIDDocument == "" && d.Data == "" {
		return errors.New("temEMPTY_DID: At least one of URI, DIDDocument, or Data is required")
	}

	// Check if all present fields are empty (all fields present but empty)
	// Reference: DID.cpp line 61-64
	uriPresent := d.URI != ""
	docPresent := d.DIDDocument != ""
	dataPresent := d.Data != ""

	// If URI is present but empty string
	uriEmpty := uriPresent && d.URI == ""
	docEmpty := docPresent && d.DIDDocument == ""
	dataEmpty := dataPresent && d.Data == ""

	// Note: This case cannot actually happen given the earlier check,
	// but we keep the logic for completeness with rippled

	// Check field lengths (after hex decode)
	// Reference: DID.cpp line 66-75
	if d.URI != "" {
		decoded, err := hex.DecodeString(d.URI)
		if err != nil {
			return errors.New("temMALFORMED: URI must be valid hex")
		}
		if len(decoded) > MaxDIDURILength {
			return errors.New("temMALFORMED: URI too long")
		}
	}

	if d.DIDDocument != "" {
		decoded, err := hex.DecodeString(d.DIDDocument)
		if err != nil {
			return errors.New("temMALFORMED: DIDDocument must be valid hex")
		}
		if len(decoded) > MaxDIDDocumentLength {
			return errors.New("temMALFORMED: DIDDocument too long")
		}
	}

	if d.Data != "" {
		decoded, err := hex.DecodeString(d.Data)
		if err != nil {
			return errors.New("temMALFORMED: Data must be valid hex")
		}
		if len(decoded) > MaxDIDAttestationLength {
			return errors.New("temMALFORMED: Data too long")
		}
	}

	// Suppress unused variable warnings
	_ = uriEmpty
	_ = docEmpty
	_ = dataEmpty

	return nil
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

// Validate validates the DIDDelete transaction
// Reference: rippled DID.cpp DIDDelete::preflight
func (d *DIDDelete) Validate() error {
	if err := d.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	flags := d.GetFlags()
	if flags&TfUniversalMask != 0 {
		return errors.New("temINVALID_FLAG: Invalid flags set")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (d *DIDDelete) Flatten() (map[string]any, error) {
	return d.Common.ToMap(), nil
}
