package credential

import (
	"encoding/hex"
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
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
// Reference: rippled Credentials.cpp CredentialCreate::doApply()
func (c *CredentialCreate) Apply(ctx *tx.ApplyContext) tx.Result {
	if c.Subject == "" || c.CredentialType == "" {
		return tx.TemINVALID
	}

	subjectID, err := sle.DecodeAccountID(c.Subject)
	if err != nil {
		return tx.TecNO_TARGET
	}

	// Decode credential type from hex to bytes
	credTypeBytes, err := hex.DecodeString(c.CredentialType)
	if err != nil {
		return tx.TemINVALID
	}

	// Compute correct keylet: credential(subject, issuer, credType)
	// where issuer = ctx.AccountID (the transaction sender)
	credKeylet := keylet.Credential(subjectID, ctx.AccountID, credTypeBytes)

	// Preclaim check: verify subject account exists
	subjectAccountKeylet := keylet.Account(subjectID)
	subjectExists, err := ctx.View.Exists(subjectAccountKeylet)
	if err != nil || !subjectExists {
		return tx.TecNO_TARGET
	}

	// Preclaim check: verify credential doesn't already exist
	exists, err := ctx.View.Exists(credKeylet)
	if err != nil {
		return tx.TefINTERNAL
	}
	if exists {
		return tx.TecDUPLICATE
	}

	// Check expiration (if set, must be in the future)
	if c.Expiration != nil {
		closeTime := ctx.Config.ParentCloseTime
		if closeTime > *c.Expiration {
			return tx.TecEXPIRED
		}
	}

	// Check reserve for issuer (ctx.Account)
	// Use prior balance (before fee deduction) to match rippled's behavior
	// Reference: rippled Credentials.cpp line 154: if (mPriorBalance < reserve)
	priorBalance := ctx.Account.Balance + ctx.Config.BaseFee
	reserve := ctx.AccountReserve(ctx.Account.OwnerCount + 1)
	if priorBalance < reserve {
		return tx.TecINSUFFICIENT_RESERVE
	}

	// Create the credential entry
	cred := &CredentialEntry{
		Subject:        subjectID,
		Issuer:         ctx.AccountID,
		CredentialType: credTypeBytes,
		IssuerNode:     0, // Would be set by dirInsert
	}

	// Set expiration if provided
	if c.Expiration != nil {
		cred.Expiration = c.Expiration
	}

	// Set URI if provided
	if c.URI != "" {
		uriBytes, err := hex.DecodeString(c.URI)
		if err == nil {
			cred.URI = uriBytes
		}
	}

	// Self-issue: if subject == issuer, auto-accept
	if subjectID == ctx.AccountID {
		cred.SetAccepted()
	}

	// Serialize the credential entry
	credData, err := serializeCredentialEntry(cred)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Insert the credential
	if err := ctx.View.Insert(credKeylet, credData); err != nil {
		return tx.TefINTERNAL
	}

	// Increase issuer's owner count
	ctx.Account.OwnerCount++

	return tx.TesSUCCESS
}

// Apply applies the CredentialAccept transaction to ledger state.
// Reference: rippled Credentials.cpp CredentialAccept::doApply()
func (c *CredentialAccept) Apply(ctx *tx.ApplyContext) tx.Result {
	if c.Issuer == "" || c.CredentialType == "" {
		return tx.TemINVALID
	}

	issuerID, err := sle.DecodeAccountID(c.Issuer)
	if err != nil {
		return tx.TecNO_TARGET
	}

	// Decode credential type from hex to bytes
	credTypeBytes, err := hex.DecodeString(c.CredentialType)
	if err != nil {
		return tx.TemINVALID
	}

	// Preclaim check: verify issuer account exists
	issuerAccountKeylet := keylet.Account(issuerID)
	issuerExists, err := ctx.View.Exists(issuerAccountKeylet)
	if err != nil || !issuerExists {
		return tx.TecNO_ISSUER
	}

	// Compute correct keylet: credential(subject, issuer, credType)
	// where subject = ctx.AccountID (the transaction sender)
	credKeylet := keylet.Credential(ctx.AccountID, issuerID, credTypeBytes)

	// Read the credential
	credData, err := ctx.View.Read(credKeylet)
	if err != nil || credData == nil {
		return tx.TecNO_ENTRY
	}

	// Parse the credential entry
	cred, err := parseCredentialEntry(credData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Check if already accepted
	if cred.IsAccepted() {
		return tx.TecDUPLICATE
	}

	// Check if credential is expired
	closeTime := ctx.Config.ParentCloseTime
	if checkCredentialExpired(cred, closeTime) {
		// Delete expired credentials even if the transaction failed
		if err := ctx.View.Erase(credKeylet); err != nil {
			return tx.TefINTERNAL
		}
		// Decrease issuer's owner count
		issuerData, err := ctx.View.Read(issuerAccountKeylet)
		if err == nil && issuerData != nil {
			issuerAccount, err := sle.ParseAccountRoot(issuerData)
			if err == nil && issuerAccount.OwnerCount > 0 {
				issuerAccount.OwnerCount--
				updatedIssuerData, err := sle.SerializeAccountRoot(issuerAccount)
				if err == nil {
					ctx.View.Update(issuerAccountKeylet, updatedIssuerData)
				}
			}
		}
		return tx.TecEXPIRED
	}

	// Check reserve for subject (ctx.Account)
	// Use prior balance (before fee deduction) to match rippled's behavior
	// Reference: rippled Credentials.cpp line 376: if (mPriorBalance < reserve)
	priorBalance := ctx.Account.Balance + ctx.Config.BaseFee
	reserve := ctx.AccountReserve(ctx.Account.OwnerCount + 1)
	if priorBalance < reserve {
		return tx.TecINSUFFICIENT_RESERVE
	}

	// Set accepted flag
	cred.SetAccepted()

	// Serialize and update the credential
	updatedCredData, err := serializeCredentialEntry(cred)
	if err != nil {
		return tx.TefINTERNAL
	}

	if err := ctx.View.Update(credKeylet, updatedCredData); err != nil {
		return tx.TefINTERNAL
	}

	// Transfer ownership: decrease issuer's owner count, increase subject's owner count
	// Read issuer account
	issuerData, err := ctx.View.Read(issuerAccountKeylet)
	if err != nil || issuerData == nil {
		return tx.TefINTERNAL
	}

	issuerAccount, err := sle.ParseAccountRoot(issuerData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Decrease issuer's owner count
	if issuerAccount.OwnerCount > 0 {
		issuerAccount.OwnerCount--
	}

	// Serialize and update issuer account
	updatedIssuerData, err := sle.SerializeAccountRoot(issuerAccount)
	if err != nil {
		return tx.TefINTERNAL
	}

	if err := ctx.View.Update(issuerAccountKeylet, updatedIssuerData); err != nil {
		return tx.TefINTERNAL
	}

	// Increase subject's owner count
	ctx.Account.OwnerCount++

	return tx.TesSUCCESS
}

// Apply applies the CredentialDelete transaction to ledger state.
// Reference: rippled Credentials.cpp CredentialDelete::doApply()
func (c *CredentialDelete) Apply(ctx *tx.ApplyContext) tx.Result {
	if c.CredentialType == "" {
		return tx.TemINVALID
	}

	// Decode credential type from hex to bytes
	credTypeBytes, err := hex.DecodeString(c.CredentialType)
	if err != nil {
		return tx.TemINVALID
	}

	// Default subject/issuer to Account if not specified
	var subjectID, issuerID [20]byte

	if c.Subject != "" {
		subjectID, err = sle.DecodeAccountID(c.Subject)
		if err != nil {
			return tx.TecNO_TARGET
		}
	} else {
		subjectID = ctx.AccountID
	}

	if c.Issuer != "" {
		issuerID, err = sle.DecodeAccountID(c.Issuer)
		if err != nil {
			return tx.TecNO_TARGET
		}
	} else {
		issuerID = ctx.AccountID
	}

	// Compute correct keylet: credential(subject, issuer, credType)
	credKeylet := keylet.Credential(subjectID, issuerID, credTypeBytes)

	// Preclaim check: verify credential exists
	credData, err := ctx.View.Read(credKeylet)
	if err != nil || credData == nil {
		return tx.TecNO_ENTRY
	}

	// Parse the credential entry
	cred, err := parseCredentialEntry(credData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Permission check: only subject or issuer can delete non-expired credentials
	// Anyone can delete expired credentials
	closeTime := ctx.Config.ParentCloseTime
	isExpired := checkCredentialExpired(cred, closeTime)
	isSubject := subjectID == ctx.AccountID
	isIssuer := issuerID == ctx.AccountID

	if !isSubject && !isIssuer && !isExpired {
		return tx.TecNO_PERMISSION
	}

	// Delete the credential
	if err := ctx.View.Erase(credKeylet); err != nil {
		return tx.TefINTERNAL
	}

	// Adjust owner count based on who owns the credential
	// If accepted, subject owns it. If not accepted, issuer owns it.
	if cred.IsAccepted() {
		// Credential was accepted, subject owns it
		if isSubject {
			// Transaction sender is the subject (owner)
			if ctx.Account.OwnerCount > 0 {
				ctx.Account.OwnerCount--
			}
		} else {
			// Need to decrease subject's owner count
			subjectAccountKeylet := keylet.Account(subjectID)
			subjectData, err := ctx.View.Read(subjectAccountKeylet)
			if err == nil && subjectData != nil {
				subjectAccount, err := sle.ParseAccountRoot(subjectData)
				if err == nil && subjectAccount.OwnerCount > 0 {
					subjectAccount.OwnerCount--
					updatedData, err := sle.SerializeAccountRoot(subjectAccount)
					if err == nil {
						ctx.View.Update(subjectAccountKeylet, updatedData)
					}
				}
			}
		}
	} else {
		// Credential was not accepted, issuer owns it
		if isIssuer {
			// Transaction sender is the issuer (owner)
			if ctx.Account.OwnerCount > 0 {
				ctx.Account.OwnerCount--
			}
		} else {
			// Need to decrease issuer's owner count
			issuerAccountKeylet := keylet.Account(issuerID)
			issuerData, err := ctx.View.Read(issuerAccountKeylet)
			if err == nil && issuerData != nil {
				issuerAccount, err := sle.ParseAccountRoot(issuerData)
				if err == nil && issuerAccount.OwnerCount > 0 {
					issuerAccount.OwnerCount--
					updatedData, err := sle.SerializeAccountRoot(issuerAccount)
					if err == nil {
						ctx.View.Update(issuerAccountKeylet, updatedData)
					}
				}
			}
		}
	}

	return tx.TesSUCCESS
}
