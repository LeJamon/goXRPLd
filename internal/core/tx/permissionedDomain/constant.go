package permissioneddomain

import "errors"

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

// AcceptedCredential defines an accepted credential type (wrapper for XRPL STArray format)
type AcceptedCredential struct {
	AcceptedCredential AcceptedCredentialData `json:"AcceptedCredential"`
}

// AcceptedCredentialData contains the credential data
type AcceptedCredentialData struct {
	Issuer         string `json:"Issuer"`
	CredentialType string `json:"CredentialType"`
}
