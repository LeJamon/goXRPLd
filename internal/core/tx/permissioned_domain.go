package tx

import (
	"encoding/hex"
	"errors"
)

// Permissioned domain constants
const (
	// MaxPermissionedDomainCredentials is the maximum number of credentials per domain
	MaxPermissionedDomainCredentials = 10
)

// Permissioned domain errors
var (
	ErrPermDomainDomainIDZero        = errors.New("temMALFORMED: DomainID cannot be zero")
	ErrPermDomainTooManyCredentials  = errors.New("temMALFORMED: too many AcceptedCredentials")
	ErrPermDomainDuplicateCredential = errors.New("temMALFORMED: duplicate credential in AcceptedCredentials")
	ErrPermDomainEmptyCredType       = errors.New("temMALFORMED: CredentialType cannot be empty")
	ErrPermDomainCredTypeTooLong     = errors.New("temMALFORMED: CredentialType exceeds maximum length")
	ErrPermDomainNoIssuer            = errors.New("temMALFORMED: Issuer is required for each credential")
	ErrPermDomainIDRequired          = errors.New("temMALFORMED: DomainID is required for delete")
)

// PermissionedDomainSet creates or modifies a permissioned domain.
// Reference: rippled PermissionedDomainSet.cpp
type PermissionedDomainSet struct {
	BaseTx

	// DomainID is the ID of the domain (optional, omit for creation)
	DomainID string `json:"DomainID,omitempty"`

	// AcceptedCredentials defines the credentials accepted by this domain (required)
	AcceptedCredentials []AcceptedCredential `json:"AcceptedCredentials"`
}

// AcceptedCredential defines an accepted credential type (wrapper for XRPL STArray format)
type AcceptedCredential struct {
	AcceptedCredential AcceptedCredentialData `json:"AcceptedCredential"`
}

// AcceptedCredentialData contains the credential data
type AcceptedCredentialData struct {
	Issuer         string `json:"Issuer"`
	CredentialType string `json:"CredentialType"`
}

// NewPermissionedDomainSet creates a new PermissionedDomainSet transaction
func NewPermissionedDomainSet(account string) *PermissionedDomainSet {
	return &PermissionedDomainSet{
		BaseTx: *NewBaseTx(TypePermissionedDomainSet, account),
	}
}

// TxType returns the transaction type
func (p *PermissionedDomainSet) TxType() Type {
	return TypePermissionedDomainSet
}

// Validate validates the PermissionedDomainSet transaction
// Reference: rippled PermissionedDomainSet.cpp preflight()
func (p *PermissionedDomainSet) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	// Reference: rippled PermissionedDomainSet.cpp:41-45
	if p.Common.Flags != nil && *p.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// If DomainID is present, it must not be zero
	// Reference: rippled PermissionedDomainSet.cpp:54-56
	if p.DomainID != "" {
		domainBytes, err := hex.DecodeString(p.DomainID)
		if err != nil || len(domainBytes) != 32 {
			return errors.New("temMALFORMED: DomainID must be a valid 256-bit hash")
		}
		// Check if zero
		isZero := true
		for _, b := range domainBytes {
			if b != 0 {
				isZero = false
				break
			}
		}
		if isZero {
			return ErrPermDomainDomainIDZero
		}
	}

	// Validate AcceptedCredentials array
	// Reference: rippled PermissionedDomainSet.cpp checkArray()
	if len(p.AcceptedCredentials) > MaxPermissionedDomainCredentials {
		return ErrPermDomainTooManyCredentials
	}

	// Check for duplicates and validate each credential
	seen := make(map[string]bool)
	for _, cred := range p.AcceptedCredentials {
		data := cred.AcceptedCredential

		// Issuer is required
		if data.Issuer == "" {
			return ErrPermDomainNoIssuer
		}

		// CredentialType is required and must be valid
		if data.CredentialType == "" {
			return ErrPermDomainEmptyCredType
		}

		// Validate CredentialType is valid hex
		credTypeBytes, err := hex.DecodeString(data.CredentialType)
		if err != nil {
			return errors.New("temMALFORMED: CredentialType must be valid hex string")
		}
		if len(credTypeBytes) == 0 {
			return ErrPermDomainEmptyCredType
		}
		if len(credTypeBytes) > MaxCredentialTypeLength {
			return ErrPermDomainCredTypeTooLong
		}

		// Check for duplicate
		key := data.Issuer + ":" + data.CredentialType
		if seen[key] {
			return ErrPermDomainDuplicateCredential
		}
		seen[key] = true
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (p *PermissionedDomainSet) Flatten() (map[string]any, error) {
	m := p.Common.ToMap()

	if p.DomainID != "" {
		m["DomainID"] = p.DomainID
	}
	if len(p.AcceptedCredentials) > 0 {
		m["AcceptedCredentials"] = p.AcceptedCredentials
	}

	return m, nil
}

// AddAcceptedCredential adds an accepted credential
func (p *PermissionedDomainSet) AddAcceptedCredential(issuer, credentialType string) {
	p.AcceptedCredentials = append(p.AcceptedCredentials, AcceptedCredential{
		AcceptedCredential: AcceptedCredentialData{
			Issuer:         issuer,
			CredentialType: credentialType,
		},
	})
}

// RequiredAmendments returns the amendments required for this transaction type
func (p *PermissionedDomainSet) RequiredAmendments() []string {
	return []string{AmendmentPermissionedDomains, AmendmentCredentials}
}

// PermissionedDomainDelete deletes a permissioned domain.
// Reference: rippled PermissionedDomainDelete.cpp
type PermissionedDomainDelete struct {
	BaseTx

	// DomainID is the ID of the domain to delete (required)
	DomainID string `json:"DomainID"`
}

// NewPermissionedDomainDelete creates a new PermissionedDomainDelete transaction
func NewPermissionedDomainDelete(account, domainID string) *PermissionedDomainDelete {
	return &PermissionedDomainDelete{
		BaseTx:   *NewBaseTx(TypePermissionedDomainDelete, account),
		DomainID: domainID,
	}
}

// TxType returns the transaction type
func (p *PermissionedDomainDelete) TxType() Type {
	return TypePermissionedDomainDelete
}

// Validate validates the PermissionedDomainDelete transaction
// Reference: rippled PermissionedDomainDelete.cpp preflight()
func (p *PermissionedDomainDelete) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	// Reference: rippled PermissionedDomainDelete.cpp:36-40
	if p.Common.Flags != nil && *p.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// DomainID is required
	// Reference: rippled PermissionedDomainDelete.cpp:42-44
	if p.DomainID == "" {
		return ErrPermDomainIDRequired
	}

	// Validate DomainID is valid 256-bit hash and not zero
	domainBytes, err := hex.DecodeString(p.DomainID)
	if err != nil || len(domainBytes) != 32 {
		return errors.New("temMALFORMED: DomainID must be a valid 256-bit hash")
	}

	// Check if zero
	isZero := true
	for _, b := range domainBytes {
		if b != 0 {
			isZero = false
			break
		}
	}
	if isZero {
		return ErrPermDomainDomainIDZero
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (p *PermissionedDomainDelete) Flatten() (map[string]any, error) {
	m := p.Common.ToMap()
	m["DomainID"] = p.DomainID
	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (p *PermissionedDomainDelete) RequiredAmendments() []string {
	return []string{AmendmentPermissionedDomains}
}
