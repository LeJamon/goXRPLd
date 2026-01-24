package permissioneddomain

import (
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/credential"
)

func init() {
	tx.Register(tx.TypePermissionedDomainSet, func() tx.Transaction {
		return &PermissionedDomainSet{BaseTx: *tx.NewBaseTx(tx.TypePermissionedDomainSet, "")}
	})
	tx.Register(tx.TypePermissionedDomainDelete, func() tx.Transaction {
		return &PermissionedDomainDelete{BaseTx: *tx.NewBaseTx(tx.TypePermissionedDomainDelete, "")}
	})
}

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
	tx.BaseTx

	// DomainID is the ID of the domain (optional, omit for creation)
	DomainID string `json:"DomainID,omitempty" xrpl:"DomainID,omitempty"`

	// AcceptedCredentials defines the credentials accepted by this domain (required)
	AcceptedCredentials []AcceptedCredential `json:"AcceptedCredentials" xrpl:"AcceptedCredentials,omitempty"`
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
		BaseTx: *tx.NewBaseTx(tx.TypePermissionedDomainSet, account),
	}
}

// TxType returns the transaction type
func (p *PermissionedDomainSet) TxType() tx.Type {
	return tx.TypePermissionedDomainSet
}

// Validate validates the PermissionedDomainSet transaction
// Reference: rippled PermissionedDomainSet.cpp preflight()
func (p *PermissionedDomainSet) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	// Reference: rippled PermissionedDomainSet.cpp:41-45
	if p.Common.Flags != nil && *p.Common.Flags&tx.TfUniversal != 0 {
		return tx.ErrInvalidFlags
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
		if len(credTypeBytes) > credential.MaxCredentialTypeLength {
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
	return tx.ReflectFlatten(p)
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
	return []string{amendment.AmendmentPermissionedDomains, amendment.AmendmentCredentials}
}

// PermissionedDomainDelete deletes a permissioned domain.
// Reference: rippled PermissionedDomainDelete.cpp
type PermissionedDomainDelete struct {
	tx.BaseTx

	// DomainID is the ID of the domain to delete (required)
	DomainID string `json:"DomainID" xrpl:"DomainID"`
}

// NewPermissionedDomainDelete creates a new PermissionedDomainDelete transaction
func NewPermissionedDomainDelete(account, domainID string) *PermissionedDomainDelete {
	return &PermissionedDomainDelete{
		BaseTx:   *tx.NewBaseTx(tx.TypePermissionedDomainDelete, account),
		DomainID: domainID,
	}
}

// TxType returns the transaction type
func (p *PermissionedDomainDelete) TxType() tx.Type {
	return tx.TypePermissionedDomainDelete
}

// Validate validates the PermissionedDomainDelete transaction
// Reference: rippled PermissionedDomainDelete.cpp preflight()
func (p *PermissionedDomainDelete) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (tfUniversalMask)
	// Reference: rippled PermissionedDomainDelete.cpp:36-40
	if p.Common.Flags != nil && *p.Common.Flags&tx.TfUniversal != 0 {
		return tx.ErrInvalidFlags
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
	return tx.ReflectFlatten(p)
}

// RequiredAmendments returns the amendments required for this transaction type
func (p *PermissionedDomainDelete) RequiredAmendments() []string {
	return []string{amendment.AmendmentPermissionedDomains}
}

// Apply applies the PermissionedDomainSet transaction to the ledger.
func (p *PermissionedDomainSet) Apply(ctx *tx.ApplyContext) tx.Result {
	var domainKey [32]byte
	if p.DomainID != "" {
		domainBytes, err := hex.DecodeString(p.DomainID)
		if err != nil || len(domainBytes) != 32 {
			return tx.TemINVALID
		}
		copy(domainKey[:], domainBytes)
		domainKeylet := keylet.Keylet{Key: domainKey, Type: 0x0082}
		_, err = ctx.View.Read(domainKeylet)
		if err != nil {
			return tx.TecNO_ENTRY
		}
	} else {
		copy(domainKey[:20], ctx.AccountID[:])
		binary.BigEndian.PutUint32(domainKey[20:], ctx.Account.Sequence)
		domainKeylet := keylet.Keylet{Key: domainKey, Type: 0x0082}
		domainData := make([]byte, 64)
		copy(domainData[:20], ctx.AccountID[:])
		if err := ctx.View.Insert(domainKeylet, domainData); err != nil {
			return tx.TefINTERNAL
		}
		ctx.Account.OwnerCount++
	}
	return tx.TesSUCCESS
}

// Apply applies the PermissionedDomainDelete transaction to the ledger.
func (p *PermissionedDomainDelete) Apply(ctx *tx.ApplyContext) tx.Result {
	if p.DomainID == "" {
		return tx.TemINVALID
	}
	domainBytes, err := hex.DecodeString(p.DomainID)
	if err != nil || len(domainBytes) != 32 {
		return tx.TemINVALID
	}
	var domainKey [32]byte
	copy(domainKey[:], domainBytes)
	domainKeylet := keylet.Keylet{Key: domainKey, Type: 0x0082}
	if err := ctx.View.Erase(domainKeylet); err != nil {
		return tx.TecNO_ENTRY
	}
	if ctx.Account.OwnerCount > 0 {
		ctx.Account.OwnerCount--
	}
	return tx.TesSUCCESS
}
