package permissioneddomain

import (
	"encoding/hex"
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
)

func init() {
	tx.Register(tx.TypePermissionedDomainDelete, func() tx.Transaction {
		return &PermissionedDomainDelete{BaseTx: *tx.NewBaseTx(tx.TypePermissionedDomainDelete, "")}
	})
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
