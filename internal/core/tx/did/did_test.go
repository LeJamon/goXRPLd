package did

import (
	"encoding/hex"
	"strings"
	"testing"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	tx "github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
)

// TestDIDSetValidation tests DIDSet transaction validation.
// These tests are based on rippled's DID_test.cpp testSetInvalid.
func TestDIDSetValidation(t *testing.T) {
	tests := []struct {
		name        string
		did         *DIDSet
		expectError bool
		errorType   error
	}{
		{
			name: "valid DIDSet with URI only",
			did: &DIDSet{
				BaseTx: *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				URI:    hex.EncodeToString([]byte("https://example.com/did/123")),
			},
			expectError: false,
		},
		{
			name: "valid DIDSet with DIDDocument only",
			did: &DIDSet{
				BaseTx:      *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				DIDDocument: hex.EncodeToString([]byte(`{"@context":"https://www.w3.org/ns/did/v1"}`)),
			},
			expectError: false,
		},
		{
			name: "valid DIDSet with Data only",
			did: &DIDSet{
				BaseTx: *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				Data:   hex.EncodeToString([]byte("attestation data")),
			},
			expectError: false,
		},
		{
			name: "valid DIDSet with all fields",
			did: &DIDSet{
				BaseTx:      *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				URI:         hex.EncodeToString([]byte("https://example.com/did")),
				DIDDocument: hex.EncodeToString([]byte("document")),
				Data:        hex.EncodeToString([]byte("data")),
			},
			expectError: false,
		},
		{
			name: "invalid - no fields - temEMPTY_DID",
			did: &DIDSet{
				BaseTx: *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				// No URI, DIDDocument, or Data
			},
			expectError: true,
			errorType:   ErrDIDEmpty,
		},
		{
			name: "invalid - URI too long - temMALFORMED",
			did: &DIDSet{
				BaseTx: *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				URI:    hex.EncodeToString(make([]byte, 257)), // 257 bytes > max 256
			},
			expectError: true,
			errorType:   ErrDIDURITooLong,
		},
		{
			name: "invalid - DIDDocument too long - temMALFORMED",
			did: &DIDSet{
				BaseTx:      *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				DIDDocument: hex.EncodeToString(make([]byte, 257)),
			},
			expectError: true,
			errorType:   ErrDIDDocTooLong,
		},
		{
			name: "invalid - Data too long - temMALFORMED",
			did: &DIDSet{
				BaseTx: *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				Data:   hex.EncodeToString(make([]byte, 257)),
			},
			expectError: true,
			errorType:   ErrDIDDataTooLong,
		},
		{
			name: "invalid - invalid hex in URI - temMALFORMED",
			did: &DIDSet{
				BaseTx: *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				URI:    "not-valid-hex!@#$",
			},
			expectError: true,
			errorType:   ErrDIDInvalidHex,
		},
		{
			name: "valid - maximum length URI (256 bytes)",
			did: &DIDSet{
				BaseTx: *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				URI:    hex.EncodeToString(make([]byte, 256)), // exactly at limit
			},
			expectError: false,
		},
		{
			name: "valid - maximum length DIDDocument (256 bytes)",
			did: &DIDSet{
				BaseTx:      *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				DIDDocument: hex.EncodeToString(make([]byte, 256)),
			},
			expectError: false,
		},
		{
			name: "valid - maximum length Data (256 bytes)",
			did: &DIDSet{
				BaseTx: *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				Data:   hex.EncodeToString(make([]byte, 256)),
			},
			expectError: false,
		},
		{
			name: "invalid - missing account",
			did: &DIDSet{
				BaseTx: tx.BaseTx{Common: tx.Common{TransactionType: "DIDSet"}},
				URI:    hex.EncodeToString([]byte("uri")),
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.did.Validate()
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if tt.errorType != nil && err != tt.errorType {
					t.Errorf("expected error %v, got %v", tt.errorType, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestDIDDeleteValidation tests DIDDelete transaction validation.
// Based on rippled's DID_test.cpp testDeleteInvalid.
func TestDIDDeleteValidation(t *testing.T) {
	tests := []struct {
		name        string
		did         *DIDDelete
		expectError bool
	}{
		{
			name: "valid DIDDelete",
			did: &DIDDelete{
				BaseTx: *tx.NewBaseTx(tx.TypeDIDDelete, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
			},
			expectError: false,
		},
		{
			name: "invalid - missing account",
			did: &DIDDelete{
				BaseTx: tx.BaseTx{Common: tx.Common{TransactionType: "DIDDelete"}},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.did.Validate()
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestDIDSetFlatten tests the Flatten method for DIDSet.
func TestDIDSetFlatten(t *testing.T) {
	tests := []struct {
		name     string
		did      *DIDSet
		checkMap func(t *testing.T, m map[string]any)
	}{
		{
			name: "DIDSet with URI only",
			did: &DIDSet{
				BaseTx: *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				URI:    hex.EncodeToString([]byte("https://example.com")),
			},
			checkMap: func(t *testing.T, m map[string]any) {
				if m["Account"] != "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY" {
					t.Errorf("expected Account, got %v", m["Account"])
				}
				if _, ok := m["URI"]; !ok {
					t.Error("expected URI to be present")
				}
				if _, ok := m["DIDDocument"]; ok {
					t.Error("DIDDocument should not be present")
				}
				if _, ok := m["Data"]; ok {
					t.Error("Data should not be present")
				}
			},
		},
		{
			name: "DIDSet with all fields",
			did: &DIDSet{
				BaseTx:      *tx.NewBaseTx(tx.TypeDIDSet, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
				URI:         hex.EncodeToString([]byte("uri")),
				DIDDocument: hex.EncodeToString([]byte("doc")),
				Data:        hex.EncodeToString([]byte("data")),
			},
			checkMap: func(t *testing.T, m map[string]any) {
				if _, ok := m["URI"]; !ok {
					t.Error("expected URI to be present")
				}
				if _, ok := m["DIDDocument"]; !ok {
					t.Error("expected DIDDocument to be present")
				}
				if _, ok := m["Data"]; !ok {
					t.Error("expected Data to be present")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := tt.did.Flatten()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.checkMap(t, m)
		})
	}
}

// TestDIDDeleteFlatten tests the Flatten method for DIDDelete.
func TestDIDDeleteFlatten(t *testing.T) {
	did := &DIDDelete{
		BaseTx: *tx.NewBaseTx(tx.TypeDIDDelete, "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY"),
	}

	m, err := did.Flatten()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m["Account"] != "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY" {
		t.Errorf("expected Account, got %v", m["Account"])
	}
	if m["TransactionType"] != "DIDDelete" {
		t.Errorf("expected TransactionType=DIDDelete, got %v", m["TransactionType"])
	}
}

// TestDIDTransactionTypes tests that transaction types are correctly returned.
func TestDIDTransactionTypes(t *testing.T) {
	t.Run("DIDSet type", func(t *testing.T) {
		d := NewDIDSet("rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY")
		if d.TxType() != tx.TypeDIDSet {
			t.Errorf("expected tx.TypeDIDSet, got %v", d.TxType())
		}
	})

	t.Run("DIDDelete type", func(t *testing.T) {
		d := NewDIDDelete("rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY")
		if d.TxType() != tx.TypeDIDDelete {
			t.Errorf("expected tx.TypeDIDDelete, got %v", d.TxType())
		}
	})
}

// TestDIDConstructors tests the constructor functions.
func TestDIDConstructors(t *testing.T) {
	t.Run("NewDIDSet", func(t *testing.T) {
		d := NewDIDSet("rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY")
		if d.Account != "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY" {
			t.Errorf("expected Account, got %v", d.Account)
		}
		if d.URI != "" {
			t.Error("URI should be empty by default")
		}
		if d.DIDDocument != "" {
			t.Error("DIDDocument should be empty by default")
		}
		if d.Data != "" {
			t.Error("Data should be empty by default")
		}
	})

	t.Run("NewDIDDelete", func(t *testing.T) {
		d := NewDIDDelete("rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY")
		if d.Account != "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY" {
			t.Errorf("expected Account, got %v", d.Account)
		}
	})
}

// TestDIDRequiredAmendments tests that DID transactions require the DID amendment.
func TestDIDRequiredAmendments(t *testing.T) {
	t.Run("DIDSet requires DID amendment", func(t *testing.T) {
		d := NewDIDSet("rTest")
		amendments := d.RequiredAmendments()
		if len(amendments) != 1 || amendments[0] != amendment.AmendmentDID {
			t.Errorf("expected [%s], got %v", amendment.AmendmentDID, amendments)
		}
	})

	t.Run("DIDDelete requires DID amendment", func(t *testing.T) {
		d := NewDIDDelete("rTest")
		amendments := d.RequiredAmendments()
		if len(amendments) != 1 || amendments[0] != amendment.AmendmentDID {
			t.Errorf("expected [%s], got %v", amendment.AmendmentDID, amendments)
		}
	})
}

// TestDIDConstants tests the DID-related constants.
func TestDIDConstants(t *testing.T) {
	// Values from rippled Protocol.h
	if MaxDIDDocumentLength != 256 {
		t.Errorf("expected MaxDIDDocumentLength=256, got %d", MaxDIDDocumentLength)
	}
	if MaxDIDURILength != 256 {
		t.Errorf("expected MaxDIDURILength=256, got %d", MaxDIDURILength)
	}
	if MaxDIDAttestationLength != 256 {
		t.Errorf("expected MaxDIDAttestationLength=256, got %d", MaxDIDAttestationLength)
	}
}

// TestDIDEntryHelpers tests the DID entry serialization helpers.
func TestDIDEntryHelpers(t *testing.T) {
	t.Run("DIDEntry HasAnyField", func(t *testing.T) {
		// Empty entry
		entry := &DIDEntry{}
		if entry.HasAnyField() {
			t.Error("empty entry should not have any field")
		}

		// Entry with URI
		uri := hex.EncodeToString([]byte("uri"))
		entry.URI = &uri
		if !entry.HasAnyField() {
			t.Error("entry with URI should have a field")
		}

		// Entry with empty URI string
		empty := ""
		entry.URI = &empty
		if entry.HasAnyField() {
			t.Error("entry with empty URI string should not have any field")
		}

		// Entry with DIDDocument
		doc := hex.EncodeToString([]byte("doc"))
		entry.DIDDocument = &doc
		if !entry.HasAnyField() {
			t.Error("entry with DIDDocument should have a field")
		}

		// Entry with Data
		entry.DIDDocument = nil
		data := hex.EncodeToString([]byte("data"))
		entry.Data = &data
		if !entry.HasAnyField() {
			t.Error("entry with Data should have a field")
		}
	})

	t.Run("DIDEntry round-trip serialization", func(t *testing.T) {
		_, accountIDBytes, _ := addresscodec.DecodeClassicAddressToAccountID("rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY")
		var accountID [20]byte
		copy(accountID[:], accountIDBytes)
		uri := strings.ToUpper(hex.EncodeToString([]byte("https://example.com")))
		doc := strings.ToUpper(hex.EncodeToString([]byte("document")))
		data := strings.ToUpper(hex.EncodeToString([]byte("attestation")))

		original := &DIDEntry{
			Account:           accountID,
			OwnerNode:         12345,
			URI:               &uri,
			DIDDocument:       &doc,
			Data:              &data,
			PreviousTxnLgrSeq: 1000,
		}

		// Serialize
		serialized, err := serializeDIDEntry(original)
		if err != nil {
			t.Fatalf("failed to serialize DID entry: %v", err)
		}

		// Parse back
		parsed, err := parseDIDEntry(serialized)
		if err != nil {
			t.Fatalf("failed to parse DID entry: %v", err)
		}

		// Verify fields
		if parsed.Account != original.Account {
			t.Errorf("Account mismatch: expected %x, got %x", original.Account, parsed.Account)
		}
		if parsed.OwnerNode != original.OwnerNode {
			t.Errorf("OwnerNode mismatch: expected %d, got %d", original.OwnerNode, parsed.OwnerNode)
		}
		if parsed.URI == nil || *parsed.URI != *original.URI {
			t.Errorf("URI mismatch")
		}
		if parsed.DIDDocument == nil || *parsed.DIDDocument != *original.DIDDocument {
			t.Errorf("DIDDocument mismatch")
		}
		if parsed.Data == nil || *parsed.Data != *original.Data {
			t.Errorf("Data mismatch")
		}
	})
}

// TestDIDSetInvalidFlags tests that invalid flags are rejected.
func TestDIDSetInvalidFlags(t *testing.T) {
	t.Run("DIDSet with invalid flags", func(t *testing.T) {
		flags := uint32(0x00000001) // non-universal flag
		did := &DIDSet{
			BaseTx: tx.BaseTx{
				Common: tx.Common{
					Account:         "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY",
					TransactionType: "DIDSet",
					Flags:           &flags,
				},
			},
			URI: hex.EncodeToString([]byte("uri")),
		}
		err := did.Validate()
		if err == nil {
			t.Error("expected error for invalid flags")
		}
		if err != tx.ErrInvalidFlags {
			t.Errorf("expected ErrInvalidFlags, got %v", err)
		}
	})

	t.Run("DIDDelete with invalid flags", func(t *testing.T) {
		flags := uint32(0x00000001) // non-universal flag
		did := &DIDDelete{
			BaseTx: tx.BaseTx{
				Common: tx.Common{
					Account:         "rN7n3473SaZBCG4dFL83w7a1RXtXtbM2shY",
					TransactionType: "DIDDelete",
					Flags:           &flags,
				},
			},
		}
		err := did.Validate()
		if err == nil {
			t.Error("expected error for invalid flags")
		}
		if err != tx.ErrInvalidFlags {
			t.Errorf("expected ErrInvalidFlags, got %v", err)
		}
	})
}

// TestTecEMPTY_DID tests that tecEMPTY_DID result code exists and has the correct value.
func TestTecEMPTY_DID(t *testing.T) {
	if tx.TecEMPTY_DID != 169 {
		t.Errorf("expected TecEMPTY_DID=169, got %d", tx.TecEMPTY_DID)
	}

	str := tx.TecEMPTY_DID.String()
	if str != "tecEMPTY_DID" {
		t.Errorf("expected String()=tecEMPTY_DID, got %s", str)
	}
}

// TestTemEMPTY_DID tests that temEMPTY_DID result code exists and has the correct value.
func TestTemEMPTY_DID(t *testing.T) {
	if tx.TemEMPTY_DID != -254 {
		t.Errorf("expected TemEMPTY_DID=-254, got %d", tx.TemEMPTY_DID)
	}
}
