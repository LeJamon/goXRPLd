package credential

import (
	"encoding/hex"
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

func init() {
	tx.Register(tx.TypeCredentialCreate, func() tx.Transaction {
		return &CredentialCreate{BaseTx: *tx.NewBaseTx(tx.TypeCredentialCreate, "")}
	})
	tx.Register(tx.TypeCredentialAccept, func() tx.Transaction {
		return &CredentialAccept{BaseTx: *tx.NewBaseTx(tx.TypeCredentialAccept, "")}
	})
	tx.Register(tx.TypeCredentialDelete, func() tx.Transaction {
		return &CredentialDelete{BaseTx: *tx.NewBaseTx(tx.TypeCredentialDelete, "")}
	})
}

// Credential constants matching rippled Protocol.h
const (
	// MaxCredentialURILength is the maximum length of a URI inside a Credential (256 bytes)
	MaxCredentialURILength = 256

	// MaxCredentialTypeLength is the maximum length of CredentialType (64 bytes)
	MaxCredentialTypeLength = 64
)

// Credential validation errors
var (
	ErrCredentialTypeTooLong = errors.New("temMALFORMED: CredentialType exceeds maximum length")
	ErrCredentialTypeEmpty   = errors.New("temMALFORMED: CredentialType is empty")
	ErrCredentialURITooLong  = errors.New("temMALFORMED: URI exceeds maximum length")
	ErrCredentialURIEmpty    = errors.New("temMALFORMED: URI is empty")
	ErrCredentialNoSubject   = errors.New("temMALFORMED: Subject is required")
	ErrCredentialNoIssuer    = errors.New("temINVALID_ACCOUNT_ID: Issuer field zeroed")
	ErrCredentialNoFields    = errors.New("temMALFORMED: No Subject or Issuer fields")
	ErrCredentialZeroAccount = errors.New("temINVALID_ACCOUNT_ID: Subject or Issuer field zeroed")
)

// CredentialCreate creates a credential.
type CredentialCreate struct {
	tx.BaseTx

	// Subject is the account the credential is about (required)
	Subject string `json:"Subject" xrpl:"Subject"`

	// CredentialType is the type of credential (required, hex-encoded)
	CredentialType string `json:"CredentialType" xrpl:"CredentialType"`

	// Expiration is when the credential expires (optional)
	Expiration *uint32 `json:"Expiration,omitempty" xrpl:"Expiration,omitempty"`

	// URI is the URI for credential details (optional)
	URI string `json:"URI,omitempty" xrpl:"URI,omitempty"`
}

// NewCredentialCreate creates a new CredentialCreate transaction
func NewCredentialCreate(account, subject, credentialType string) *CredentialCreate {
	return &CredentialCreate{
		BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, account),
		Subject:        subject,
		CredentialType: credentialType,
	}
}

// TxType returns the transaction type
func (c *CredentialCreate) TxType() tx.Type {
	return tx.TypeCredentialCreate
}

// Validate validates the CredentialCreate transaction
// Reference: rippled Credentials.cpp CredentialCreate::preflight()
func (c *CredentialCreate) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	// Reference: rippled Credentials.cpp:66-71
	if c.Common.Flags != nil && *c.Common.Flags&tx.TfUniversalMask != 0 {
		return tx.ErrInvalidFlags
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
	return tx.ReflectFlatten(c)
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CredentialCreate) RequiredAmendments() []string {
	return []string{amendment.AmendmentCredentials}
}

// CredentialAccept accepts a credential.
type CredentialAccept struct {
	tx.BaseTx

	// Issuer is the account that issued the credential (required)
	Issuer string `json:"Issuer" xrpl:"Issuer"`

	// CredentialType is the type of credential (required, hex-encoded)
	CredentialType string `json:"CredentialType" xrpl:"CredentialType"`
}

// NewCredentialAccept creates a new CredentialAccept transaction
func NewCredentialAccept(account, issuer, credentialType string) *CredentialAccept {
	return &CredentialAccept{
		BaseTx:         *tx.NewBaseTx(tx.TypeCredentialAccept, account),
		Issuer:         issuer,
		CredentialType: credentialType,
	}
}

// TxType returns the transaction type
func (c *CredentialAccept) TxType() tx.Type {
	return tx.TypeCredentialAccept
}

// Validate validates the CredentialAccept transaction
// Reference: rippled Credentials.cpp CredentialAccept::preflight()
func (c *CredentialAccept) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	// Reference: rippled Credentials.cpp:304-308
	if c.Common.Flags != nil && *c.Common.Flags&tx.TfUniversalMask != 0 {
		return tx.ErrInvalidFlags
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
	return tx.ReflectFlatten(c)
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CredentialAccept) RequiredAmendments() []string {
	return []string{amendment.AmendmentCredentials}
}

// CredentialDelete deletes a credential.
type CredentialDelete struct {
	tx.BaseTx

	// Subject is the account the credential is about (optional, defaults to Account)
	Subject string `json:"Subject,omitempty" xrpl:"Subject,omitempty"`

	// Issuer is the account that issued the credential (optional, defaults to Account)
	Issuer string `json:"Issuer,omitempty" xrpl:"Issuer,omitempty"`

	// CredentialType is the type of credential (required, hex-encoded)
	CredentialType string `json:"CredentialType" xrpl:"CredentialType"`
}

// NewCredentialDelete creates a new CredentialDelete transaction
func NewCredentialDelete(account, credentialType string) *CredentialDelete {
	return &CredentialDelete{
		BaseTx:         *tx.NewBaseTx(tx.TypeCredentialDelete, account),
		CredentialType: credentialType,
	}
}

// TxType returns the transaction type
func (c *CredentialDelete) TxType() tx.Type {
	return tx.TypeCredentialDelete
}

// Validate validates the CredentialDelete transaction
// Reference: rippled Credentials.cpp CredentialDelete::preflight()
func (c *CredentialDelete) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	// Reference: rippled Credentials.cpp:217-222
	if c.Common.Flags != nil && *c.Common.Flags&tx.TfUniversalMask != 0 {
		return tx.ErrInvalidFlags
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
	return tx.ReflectFlatten(c)
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CredentialDelete) RequiredAmendments() []string {
	return []string{amendment.AmendmentCredentials}
}

// Apply applies the CredentialCreate transaction to ledger state.
func (c *CredentialCreate) Apply(ctx *tx.ApplyContext) tx.Result {
	if c.Subject == "" || c.CredentialType == "" {
		return tx.TemINVALID
	}
	subjectID, err := sle.DecodeAccountID(c.Subject)
	if err != nil {
		return tx.TecNO_TARGET
	}
	var credKey [32]byte
	copy(credKey[:20], ctx.AccountID[:])
	copy(credKey[20:], subjectID[:12])
	credKeylet := keylet.Keylet{Key: credKey, Type: 0x0081}
	credData := make([]byte, 64)
	copy(credData[:20], ctx.AccountID[:])
	copy(credData[20:40], subjectID[:])
	if err := ctx.View.Insert(credKeylet, credData); err != nil {
		return tx.TefINTERNAL
	}
	ctx.Account.OwnerCount++
	return tx.TesSUCCESS
}

// Apply applies the CredentialAccept transaction to ledger state.
func (c *CredentialAccept) Apply(ctx *tx.ApplyContext) tx.Result {
	if c.Issuer == "" || c.CredentialType == "" {
		return tx.TemINVALID
	}
	issuerID, err := sle.DecodeAccountID(c.Issuer)
	if err != nil {
		return tx.TecNO_TARGET
	}
	var credKey [32]byte
	copy(credKey[:20], issuerID[:])
	copy(credKey[20:], ctx.AccountID[:12])
	credKeylet := keylet.Keylet{Key: credKey, Type: 0x0081}
	_, err = ctx.View.Read(credKeylet)
	if err != nil {
		return tx.TecNO_ENTRY
	}
	return tx.TesSUCCESS
}

// Apply applies the CredentialDelete transaction to ledger state.
func (c *CredentialDelete) Apply(ctx *tx.ApplyContext) tx.Result {
	if c.CredentialType == "" {
		return tx.TemINVALID
	}
	var subjectID [20]byte
	if c.Subject != "" {
		subjectID, _ = sle.DecodeAccountID(c.Subject)
	} else {
		subjectID = ctx.AccountID
	}
	var credKey [32]byte
	copy(credKey[:20], ctx.AccountID[:])
	copy(credKey[20:], subjectID[:12])
	credKeylet := keylet.Keylet{Key: credKey, Type: 0x0081}
	if err := ctx.View.Erase(credKeylet); err != nil {
		return tx.TecNO_ENTRY
	}
	if ctx.Account.OwnerCount > 0 {
		ctx.Account.OwnerCount--
	}
	return tx.TesSUCCESS
}
