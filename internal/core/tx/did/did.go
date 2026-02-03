package did

import (
	"encoding/hex"
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeDIDSet, func() tx.Transaction {
		return &DIDSet{BaseTx: *tx.NewBaseTx(tx.TypeDIDSet, "")}
	})
	tx.Register(tx.TypeDIDDelete, func() tx.Transaction {
		return &DIDDelete{BaseTx: *tx.NewBaseTx(tx.TypeDIDDelete, "")}
	})
}

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

// DID validation errors
var (
	ErrDIDEmpty       = errors.New("temEMPTY_DID: DID transaction must have at least one non-empty field")
	ErrDIDURITooLong  = errors.New("temMALFORMED: URI exceeds maximum length of 256 bytes")
	ErrDIDDocTooLong  = errors.New("temMALFORMED: DIDDocument exceeds maximum length of 256 bytes")
	ErrDIDDataTooLong = errors.New("temMALFORMED: Data exceeds maximum length of 256 bytes")
	ErrDIDInvalidHex  = errors.New("temMALFORMED: field must be valid hex string")
)

// DIDSet creates or updates a DID document.
type DIDSet struct {
	tx.BaseTx

	// Data is the public attestations (optional, hex-encoded)
	Data string `json:"Data,omitempty" xrpl:"Data,omitempty"`

	// DIDDocument is the DID document content (optional, hex-encoded)
	DIDDocument string `json:"DIDDocument,omitempty" xrpl:"DIDDocument,omitempty"`

	// URI is the URI for the DID document (optional, hex-encoded)
	URI string `json:"URI,omitempty" xrpl:"URI,omitempty"`
}

// NewDIDSet creates a new DIDSet transaction
func NewDIDSet(account string) *DIDSet {
	return &DIDSet{
		BaseTx: *tx.NewBaseTx(tx.TypeDIDSet, account),
	}
}

// TxType returns the transaction type
func (d *DIDSet) TxType() tx.Type {
	return tx.TypeDIDSet
}

// Validate validates the DIDSet transaction
// Reference: rippled DID.cpp DIDSet::preflight
func (d *DIDSet) Validate() error {
	if err := d.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	// Reference: DID.cpp line 51-52
	flags := d.GetFlags()
	if flags&tx.TfUniversalMask != 0 {
		return tx.ErrInvalidFlags
	}

	// Check if any field is present (even if empty)
	// Reference: DID.cpp line 57-59
	uriPresent := d.URI != "" || d.Common.HasField("URI")
	docPresent := d.DIDDocument != "" || d.Common.HasField("DIDDocument")
	dataPresent := d.Data != "" || d.Common.HasField("Data")

	// At least one field must be present
	if !uriPresent && !docPresent && !dataPresent {
		return ErrDIDEmpty
	}

	// If all present fields are empty, that's also an error
	// Reference: DID.cpp line 61-64
	if uriPresent && d.URI == "" &&
		docPresent && d.DIDDocument == "" &&
		dataPresent && d.Data == "" {
		return ErrDIDEmpty
	}

	// Check field lengths (after hex decode)
	// Reference: DID.cpp line 66-75
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

// Flatten returns a flat map of all transaction fields
func (d *DIDSet) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(d)
}

// RequiredAmendments returns the amendments required for this transaction type
func (d *DIDSet) RequiredAmendments() []string {
	return []string{amendment.AmendmentDID}
}

// DIDDelete deletes a DID document.
type DIDDelete struct {
	tx.BaseTx
}

// NewDIDDelete creates a new DIDDelete transaction
func NewDIDDelete(account string) *DIDDelete {
	return &DIDDelete{
		BaseTx: *tx.NewBaseTx(tx.TypeDIDDelete, account),
	}
}

// TxType returns the transaction type
func (d *DIDDelete) TxType() tx.Type {
	return tx.TypeDIDDelete
}

// Validate validates the DIDDelete transaction
// Reference: rippled DID.cpp DIDDelete::preflight
func (d *DIDDelete) Validate() error {
	if err := d.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	flags := d.GetFlags()
	if flags&tx.TfUniversalMask != 0 {
		return tx.ErrInvalidFlags
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (d *DIDDelete) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(d)
}

// RequiredAmendments returns the amendments required for this transaction type
func (d *DIDDelete) RequiredAmendments() []string {
	return []string{amendment.AmendmentDID}
}

// Apply applies a DIDSet transaction to the ledger state.
// Reference: rippled DID.cpp DIDSet::doApply
func (d *DIDSet) Apply(ctx *tx.ApplyContext) tx.Result {
	didKey := keylet.DID(ctx.AccountID)

	// Check if DID already exist
	existingData, err := ctx.View.Read(didKey)
	if err == nil && existingData != nil {
		// Update existing DID
		did, err := sle.ParseDID(existingData)
		if err != nil {
			return tx.TefINTERNAL
		}

		// Update fields based on what's provided in transaction
		if d.URI != "" {
			did.URI = d.URI
		} else if d.URI == "" && d.Common.HasField("URI") {
			did.URI = ""
		}

		if d.DIDDocument != "" {
			did.DIDDocument = d.DIDDocument
		} else if d.DIDDocument == "" && d.Common.HasField("DIDDocument") {
			did.DIDDocument = ""
		}

		if d.Data != "" {
			did.Data = d.Data
		} else if d.Data == "" && d.Common.HasField("Data") {
			did.Data = ""
		}

		// Check that at least one field remains after update
		if did.URI == "" && did.DIDDocument == "" && did.Data == "" {
			return tx.TecEMPTY_DID
		}

		// Serialize and update the DID - modification tracked automatically by ApplyStateTable
		updatedData, err := sle.SerializeDID(did, d.Account)
		if err != nil {
			return tx.TefINTERNAL
		}

		if err := ctx.View.Update(didKey, updatedData); err != nil {
			return tx.TefINTERNAL
		}

		return tx.TesSUCCESS
	}

	// Create new DID
	reserve := ctx.AccountReserve(ctx.Account.OwnerCount + 1)
	if ctx.Account.Balance < reserve {
		return tx.TecINSUFFICIENT_RESERVE
	}

	did := &sle.DIDData{
		Account:   ctx.AccountID,
		OwnerNode: 0,
	}

	if d.URI != "" {
		did.URI = d.URI
	}
	if d.DIDDocument != "" {
		did.DIDDocument = d.DIDDocument
	}
	if d.Data != "" {
		did.Data = d.Data
	}

	// Check that at least one field is set (fixEmptyDID amendment)
	if did.URI == "" && did.DIDDocument == "" && did.Data == "" {
		return tx.TecEMPTY_DID
	}

	didData, err := sle.SerializeDID(did, d.Account)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Insert the DID - creation tracked automatically by ApplyStateTable
	if err := ctx.View.Insert(didKey, didData); err != nil {
		return tx.TefINTERNAL
	}

	ctx.Account.OwnerCount++

	return tx.TesSUCCESS
}

// Apply applies a DIDDelete transaction to the ledger state.
// Reference: rippled DID.cpp DIDDelete::doApply
func (d *DIDDelete) Apply(ctx *tx.ApplyContext) tx.Result {
	didKey := keylet.DID(ctx.AccountID)

	existingData, err := ctx.View.Read(didKey)
	if err != nil || existingData == nil {
		return tx.TecNO_ENTRY
	}

	// Delete the DID entry - deletion tracked automatically by ApplyStateTable
	if err := ctx.View.Erase(didKey); err != nil {
		return tx.TefINTERNAL
	}

	if ctx.Account.OwnerCount > 0 {
		ctx.Account.OwnerCount--
	}

	return tx.TesSUCCESS
}
