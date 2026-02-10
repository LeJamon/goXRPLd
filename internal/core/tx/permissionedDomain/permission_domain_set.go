package permissioneddomain

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/credential"
)

func init() {
	tx.Register(tx.TypePermissionedDomainSet, func() tx.Transaction {
		return &PermissionedDomainSet{BaseTx: *tx.NewBaseTx(tx.TypePermissionedDomainSet, "")}
	})
}

// PermissionedDomainSet creates or modifies a permissioned domain.
// Reference: rippled PermissionedDomainSet.cpp
type PermissionedDomainSet struct {
	tx.BaseTx

	// DomainID is the ID of the domain (optional, omit for creation)
	DomainID string `json:"DomainID,omitempty" xrpl:"DomainID,omitempty"`

	// AcceptedCredentials defines the credentials accepted by this domain (required)
	AcceptedCredentials []AcceptedCredential `json:"AcceptedCredentials" xrpl:"AcceptedCredentials,omitempty"`
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
func (p *PermissionedDomainSet) RequiredAmendments() [][32]byte {
	return [][32]byte{amendment.FeaturePermissionedDomains, amendment.FeatureCredentials}
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
