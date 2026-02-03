package mpt

import (
	"encoding/hex"
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
)

func init() {
	tx.Register(tx.TypeMPTokenIssuanceDestroy, func() tx.Transaction {
		return &MPTokenIssuanceDestroy{BaseTx: *tx.NewBaseTx(tx.TypeMPTokenIssuanceDestroy, "")}
	})
}

// MPTokenIssuanceDestroy destroys a multi-purpose token issuance.
type MPTokenIssuanceDestroy struct {
	tx.BaseTx

	// MPTokenIssuanceID is the ID of the issuance to destroy (required)
	// 64-character hex string (32 bytes)
	MPTokenIssuanceID string `json:"MPTokenIssuanceID" xrpl:"MPTokenIssuanceID"`
}

// MPTokenIssuanceDestroy flag mask (only universal flags allowed)
const (
	tfMPTokenIssuanceDestroyValidMask uint32 = tx.TfUniversal
)

// NewMPTokenIssuanceDestroy creates a new MPTokenIssuanceDestroy transaction
func NewMPTokenIssuanceDestroy(account, issuanceID string) *MPTokenIssuanceDestroy {
	return &MPTokenIssuanceDestroy{
		BaseTx:            *tx.NewBaseTx(tx.TypeMPTokenIssuanceDestroy, account),
		MPTokenIssuanceID: issuanceID,
	}
}

// TxType returns the transaction type
func (m *MPTokenIssuanceDestroy) TxType() tx.Type {
	return tx.TypeMPTokenIssuanceDestroy
}

// Validate validates the MPTokenIssuanceDestroy transaction
// Reference: rippled MPTokenIssuanceDestroy.cpp preflight
func (m *MPTokenIssuanceDestroy) Validate() error {
	if err := m.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags
	flags := m.GetFlags()
	if flags&^tfMPTokenIssuanceDestroyValidMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for MPTokenIssuanceDestroy")
	}

	// MPTokenIssuanceID is required and must be valid hex
	if m.MPTokenIssuanceID == "" {
		return errors.New("temMALFORMED: MPTokenIssuanceID is required")
	}

	// MPTokenIssuanceID should be 64 hex characters (32 bytes)
	if len(m.MPTokenIssuanceID) != 64 {
		return errors.New("temMALFORMED: MPTokenIssuanceID must be 64 hex characters")
	}

	if _, err := hex.DecodeString(m.MPTokenIssuanceID); err != nil {
		return errors.New("temMALFORMED: MPTokenIssuanceID must be valid hex")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (m *MPTokenIssuanceDestroy) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(m)
}

// RequiredAmendments returns the amendments required for this transaction type
func (m *MPTokenIssuanceDestroy) RequiredAmendments() []string {
	return []string{amendment.AmendmentMPTokensV1}
}

// Apply applies the MPTokenIssuanceDestroy transaction to ledger state.
func (m *MPTokenIssuanceDestroy) Apply(ctx *tx.ApplyContext) tx.Result {
	issuanceIDBytes, err := hex.DecodeString(m.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return tx.TemINVALID
	}
	if ctx.Account.OwnerCount > 0 {
		ctx.Account.OwnerCount--
	}
	return tx.TesSUCCESS
}
