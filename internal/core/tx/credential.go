package tx

import (
	"encoding/hex"
	"errors"
)

// Credential constants matching rippled Protocol.h
const (
	// MaxCredentialURILength is the maximum length of a URI inside a Credential (256 bytes)
	MaxCredentialURILength = 256

	// MaxCredentialTypeLength is the maximum length of CredentialType (64 bytes)
	MaxCredentialTypeLength = 64
)

// Credential validation errors
var (
	ErrCredentialTypeTooLong  = errors.New("temMALFORMED: CredentialType exceeds maximum length")
	ErrCredentialTypeEmpty    = errors.New("temMALFORMED: CredentialType is empty")
	ErrCredentialURITooLong   = errors.New("temMALFORMED: URI exceeds maximum length")
	ErrCredentialURIEmpty     = errors.New("temMALFORMED: URI is empty")
	ErrCredentialNoSubject    = errors.New("temMALFORMED: Subject is required")
	ErrCredentialNoIssuer     = errors.New("temINVALID_ACCOUNT_ID: Issuer field zeroed")
	ErrCredentialNoFields     = errors.New("temMALFORMED: No Subject or Issuer fields")
	ErrCredentialZeroAccount  = errors.New("temINVALID_ACCOUNT_ID: Subject or Issuer field zeroed")
)

// CredentialCreate creates a credential.
type CredentialCreate struct {
	BaseTx

	// Subject is the account the credential is about (required)
	Subject string `json:"Subject"`

	// CredentialType is the type of credential (required, hex-encoded)
	CredentialType string `json:"CredentialType"`

	// Expiration is when the credential expires (optional)
	Expiration *uint32 `json:"Expiration,omitempty"`

	// URI is the URI for credential details (optional)
	URI string `json:"URI,omitempty"`
}

// NewCredentialCreate creates a new CredentialCreate transaction
func NewCredentialCreate(account, subject, credentialType string) *CredentialCreate {
	return &CredentialCreate{
		BaseTx:         *NewBaseTx(TypeCredentialCreate, account),
		Subject:        subject,
		CredentialType: credentialType,
	}
}

// TxType returns the transaction type
func (c *CredentialCreate) TxType() Type {
	return TypeCredentialCreate
}

// Validate validates the CredentialCreate transaction
// Reference: rippled Credentials.cpp CredentialCreate::preflight()
func (c *CredentialCreate) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	// Reference: rippled Credentials.cpp:66-71
	if c.Common.Flags != nil && *c.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// Subject is required
	// Reference: rippled Credentials.cpp:73-77
	if c.Subject == "" {
		return ErrCredentialNoSubject
	}

	// Validate URI field length (optional but if present must be valid)
	// Reference: rippled Credentials.cpp:79-84
	if c.URI != "" {
		decoded, err := hex.DecodeString(c.URI)
		if err != nil {
			return errors.New("temMALFORMED: URI must be valid hex string")
		}
		if len(decoded) == 0 {
			return ErrCredentialURIEmpty
		}
		if len(decoded) > MaxCredentialURILength {
			return ErrCredentialURITooLong
		}
	}

	// Validate CredentialType field (required, max 64 bytes)
	// Reference: rippled Credentials.cpp:86-92
	if c.CredentialType == "" {
		return ErrCredentialTypeEmpty
	}
	decoded, err := hex.DecodeString(c.CredentialType)
	if err != nil {
		return errors.New("temMALFORMED: CredentialType must be valid hex string")
	}
	if len(decoded) == 0 {
		return ErrCredentialTypeEmpty
	}
	if len(decoded) > MaxCredentialTypeLength {
		return ErrCredentialTypeTooLong
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (c *CredentialCreate) Flatten() (map[string]any, error) {
	m := c.Common.ToMap()

	m["Subject"] = c.Subject
	m["CredentialType"] = c.CredentialType

	if c.Expiration != nil {
		m["Expiration"] = *c.Expiration
	}
	if c.URI != "" {
		m["URI"] = c.URI
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CredentialCreate) RequiredAmendments() []string {
	return []string{AmendmentCredentials}
}

// CredentialAccept accepts a credential.
type CredentialAccept struct {
	BaseTx

	// Issuer is the account that issued the credential (required)
	Issuer string `json:"Issuer"`

	// CredentialType is the type of credential (required, hex-encoded)
	CredentialType string `json:"CredentialType"`
}

// NewCredentialAccept creates a new CredentialAccept transaction
func NewCredentialAccept(account, issuer, credentialType string) *CredentialAccept {
	return &CredentialAccept{
		BaseTx:         *NewBaseTx(TypeCredentialAccept, account),
		Issuer:         issuer,
		CredentialType: credentialType,
	}
}

// TxType returns the transaction type
func (c *CredentialAccept) TxType() Type {
	return TypeCredentialAccept
}

// Validate validates the CredentialAccept transaction
// Reference: rippled Credentials.cpp CredentialAccept::preflight()
func (c *CredentialAccept) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	// Reference: rippled Credentials.cpp:304-308
	if c.Common.Flags != nil && *c.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// Issuer is required and must not be zero
	// Reference: rippled Credentials.cpp:310-314
	if c.Issuer == "" {
		return ErrCredentialNoIssuer
	}

	// Validate CredentialType field (required, max 64 bytes)
	// Reference: rippled Credentials.cpp:316-323
	if c.CredentialType == "" {
		return ErrCredentialTypeEmpty
	}
	decoded, err := hex.DecodeString(c.CredentialType)
	if err != nil {
		return errors.New("temMALFORMED: CredentialType must be valid hex string")
	}
	if len(decoded) == 0 {
		return ErrCredentialTypeEmpty
	}
	if len(decoded) > MaxCredentialTypeLength {
		return ErrCredentialTypeTooLong
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (c *CredentialAccept) Flatten() (map[string]any, error) {
	m := c.Common.ToMap()

	m["Issuer"] = c.Issuer
	m["CredentialType"] = c.CredentialType

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CredentialAccept) RequiredAmendments() []string {
	return []string{AmendmentCredentials}
}

// CredentialDelete deletes a credential.
type CredentialDelete struct {
	BaseTx

	// Subject is the account the credential is about (optional, defaults to Account)
	Subject string `json:"Subject,omitempty"`

	// Issuer is the account that issued the credential (optional, defaults to Account)
	Issuer string `json:"Issuer,omitempty"`

	// CredentialType is the type of credential (required, hex-encoded)
	CredentialType string `json:"CredentialType"`
}

// NewCredentialDelete creates a new CredentialDelete transaction
func NewCredentialDelete(account, credentialType string) *CredentialDelete {
	return &CredentialDelete{
		BaseTx:         *NewBaseTx(TypeCredentialDelete, account),
		CredentialType: credentialType,
	}
}

// TxType returns the transaction type
func (c *CredentialDelete) TxType() Type {
	return TypeCredentialDelete
}

// Validate validates the CredentialDelete transaction
// Reference: rippled Credentials.cpp CredentialDelete::preflight()
func (c *CredentialDelete) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	// Reference: rippled Credentials.cpp:217-222
	if c.Common.Flags != nil && *c.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// At least one of Subject or Issuer must be present
	// Reference: rippled Credentials.cpp:224-233
	if c.Subject == "" && c.Issuer == "" {
		return ErrCredentialNoFields
	}

	// If present, Subject and Issuer must not be zero accounts
	// Reference: rippled Credentials.cpp:235-241
	// (In Go, empty string already handles this case)

	// Validate CredentialType field (required, max 64 bytes)
	// Reference: rippled Credentials.cpp:243-249
	if c.CredentialType == "" {
		return ErrCredentialTypeEmpty
	}
	decoded, err := hex.DecodeString(c.CredentialType)
	if err != nil {
		return errors.New("temMALFORMED: CredentialType must be valid hex string")
	}
	if len(decoded) == 0 {
		return ErrCredentialTypeEmpty
	}
	if len(decoded) > MaxCredentialTypeLength {
		return ErrCredentialTypeTooLong
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (c *CredentialDelete) Flatten() (map[string]any, error) {
	m := c.Common.ToMap()

	m["CredentialType"] = c.CredentialType

	if c.Subject != "" {
		m["Subject"] = c.Subject
	}
	if c.Issuer != "" {
		m["Issuer"] = c.Issuer
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CredentialDelete) RequiredAmendments() []string {
	return []string{AmendmentCredentials}
}
