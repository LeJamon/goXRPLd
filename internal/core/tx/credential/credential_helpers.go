package credential

import (
	"encoding/hex"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	crypto "github.com/LeJamon/goXRPLd/internal/crypto/common"
)

// Credential ledger entry flags
const (
	// LsfAccepted indicates the credential has been accepted by the subject
	LsfCredentialAccepted uint32 = 0x00010000
)

// CredentialEntry represents a Credential ledger entry
// Reference: rippled ledger_entries.macro ltCREDENTIAL (0x0081)
type CredentialEntry struct {
	Subject        [20]byte // Account the credential is about
	Issuer         [20]byte // Account that issued the credential
	CredentialType []byte   // Type of credential (max 64 bytes)
	Expiration     *uint32  // Optional expiration time
	URI            []byte   // Optional URI (max 256 bytes)
	Flags          uint32   // Credential flags (lsfAccepted)

	// Directory node hints
	IssuerNode  uint64
	SubjectNode uint64

	// Transaction threading
	PreviousTxnID     [32]byte
	PreviousTxnLgrSeq uint32
}

// IsAccepted returns true if the credential has been accepted
func (c *CredentialEntry) IsAccepted() bool {
	return c.Flags&LsfCredentialAccepted != 0
}

// SetAccepted sets the accepted flag
func (c *CredentialEntry) SetAccepted() {
	c.Flags |= LsfCredentialAccepted
}

// credentialKeylet calculates the keylet for a credential
// Keylet = SHA512Half(spaceCredential + subject + issuer + credentialType)
func credentialKeylet(subject, issuer [20]byte, credentialType []byte) [32]byte {
	// Space for Credential is 0x0044 ('D')
	const spaceCredential = 0x0044

	// Build the hash input: space (2 bytes) + subject (20 bytes) + issuer (20 bytes) + credentialType
	input := make([]byte, 2+20+20+len(credentialType))
	input[0] = byte(spaceCredential >> 8)
	input[1] = byte(spaceCredential & 0xFF)
	copy(input[2:22], subject[:])
	copy(input[22:42], issuer[:])
	copy(input[42:], credentialType)

	return crypto.Sha512Half(input)
}

// parseCredentialEntry parses a Credential ledger entry from binary data
func parseCredentialEntry(data []byte) (*CredentialEntry, error) {
	hexStr := hex.EncodeToString(data)
	jsonObj, err := binarycodec.Decode(hexStr)
	if err != nil {
		return nil, err
	}

	cred := &CredentialEntry{}

	// Parse Subject
	if subject, ok := jsonObj["Subject"].(string); ok {
		subjectID, err := sle.DecodeAccountID(subject)
		if err == nil {
			cred.Subject = subjectID
		}
	}

	// Parse Issuer
	if issuer, ok := jsonObj["Issuer"].(string); ok {
		issuerID, err := sle.DecodeAccountID(issuer)
		if err == nil {
			cred.Issuer = issuerID
		}
	}

	// Parse CredentialType (Blob/VL field stored as hex)
	if credType, ok := jsonObj["CredentialType"].(string); ok {
		decoded, err := hex.DecodeString(credType)
		if err == nil {
			cred.CredentialType = decoded
		}
	}

	// Parse Expiration (optional)
	if exp, ok := jsonObj["Expiration"].(float64); ok {
		expVal := uint32(exp)
		cred.Expiration = &expVal
	}

	// Parse URI (optional, Blob/VL field stored as hex)
	if uri, ok := jsonObj["URI"].(string); ok {
		decoded, err := hex.DecodeString(uri)
		if err == nil {
			cred.URI = decoded
		}
	}

	// Parse Flags
	if flags, ok := jsonObj["Flags"].(float64); ok {
		cred.Flags = uint32(flags)
	}

	// Parse IssuerNode
	if issuerNode, ok := jsonObj["IssuerNode"].(string); ok {
		cred.IssuerNode, _ = tx.ParseUint64Hex(issuerNode)
	}

	// Parse SubjectNode
	if subjectNode, ok := jsonObj["SubjectNode"].(string); ok {
		cred.SubjectNode, _ = tx.ParseUint64Hex(subjectNode)
	}

	// Parse PreviousTxnID
	if prevTxnID, ok := jsonObj["PreviousTxnID"].(string); ok {
		bytes, _ := hex.DecodeString(prevTxnID)
		copy(cred.PreviousTxnID[:], bytes)
	}

	// Parse PreviousTxnLgrSeq
	if prevSeq, ok := jsonObj["PreviousTxnLgrSeq"].(float64); ok {
		cred.PreviousTxnLgrSeq = uint32(prevSeq)
	}

	return cred, nil
}

// serializeCredentialEntry serializes a Credential entry to binary format
func serializeCredentialEntry(cred *CredentialEntry) ([]byte, error) {
	jsonObj := map[string]any{
		"LedgerEntryType": "Credential",
	}

	// Add Subject
	subjectStr, err := sle.EncodeAccountID(cred.Subject)
	if err == nil && subjectStr != "" {
		jsonObj["Subject"] = subjectStr
	}

	// Add Issuer
	issuerStr, err := sle.EncodeAccountID(cred.Issuer)
	if err == nil && issuerStr != "" {
		jsonObj["Issuer"] = issuerStr
	}

	// Add CredentialType (hex-encoded)
	if len(cred.CredentialType) > 0 {
		jsonObj["CredentialType"] = hex.EncodeToString(cred.CredentialType)
	}

	// Add Expiration (optional)
	if cred.Expiration != nil {
		jsonObj["Expiration"] = *cred.Expiration
	}

	// Add URI (optional, hex-encoded)
	if len(cred.URI) > 0 {
		jsonObj["URI"] = hex.EncodeToString(cred.URI)
	}

	// Add Flags
	if cred.Flags != 0 {
		jsonObj["Flags"] = cred.Flags
	}

	// Add IssuerNode
	jsonObj["IssuerNode"] = tx.FormatUint64Hex(cred.IssuerNode)

	// Add SubjectNode (if subject != issuer)
	if cred.Subject != cred.Issuer {
		jsonObj["SubjectNode"] = tx.FormatUint64Hex(cred.SubjectNode)
	}

	// Add PreviousTxnID
	var zeroHash [32]byte
	if cred.PreviousTxnID != zeroHash {
		jsonObj["PreviousTxnID"] = hex.EncodeToString(cred.PreviousTxnID[:])
	}

	// Add PreviousTxnLgrSeq
	if cred.PreviousTxnLgrSeq > 0 {
		jsonObj["PreviousTxnLgrSeq"] = cred.PreviousTxnLgrSeq
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, err
	}

	return hex.DecodeString(hexStr)
}

// checkCredentialExpired checks if a credential has expired
// Reference: rippled CredentialHelpers.cpp checkExpired()
func checkCredentialExpired(cred *CredentialEntry, closeTime uint32) bool {
	if cred.Expiration == nil {
		return false
	}
	return closeTime > *cred.Expiration
}
