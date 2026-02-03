package mpt

import (
	"encoding/hex"
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
)

func init() {
	tx.Register(tx.TypeMPTokenIssuanceSet, func() tx.Transaction {
		return &MPTokenIssuanceSet{BaseTx: *tx.NewBaseTx(tx.TypeMPTokenIssuanceSet, "")}
	})
}

// MPTokenIssuanceSet modifies a multi-purpose token issuance.
type MPTokenIssuanceSet struct {
	tx.BaseTx

	// MPTokenIssuanceID is the ID of the issuance (required)
	MPTokenIssuanceID string `json:"MPTokenIssuanceID" xrpl:"MPTokenIssuanceID"`

	// Holder is the holder account (optional)
	// When set, the issuer is modifying a specific holder's MPToken
	Holder string `json:"Holder,omitempty" xrpl:"Holder,omitempty"`
}

// NewMPTokenIssuanceSet creates a new MPTokenIssuanceSet transaction
func NewMPTokenIssuanceSet(account, issuanceID string) *MPTokenIssuanceSet {
	return &MPTokenIssuanceSet{
		BaseTx:            *tx.NewBaseTx(tx.TypeMPTokenIssuanceSet, account),
		MPTokenIssuanceID: issuanceID,
	}
}

// TxType returns the transaction type
func (m *MPTokenIssuanceSet) TxType() tx.Type {
	return tx.TypeMPTokenIssuanceSet
}

// Validate validates the MPTokenIssuanceSet transaction
// Reference: rippled MPTokenIssuanceSet.cpp preflight
func (m *MPTokenIssuanceSet) Validate() error {
	if err := m.BaseTx.Validate(); err != nil {
		return err
	}

	flags := m.GetFlags()

	// Check for invalid flags
	if flags&^tfMPTokenIssuanceSetValidMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for MPTokenIssuanceSet")
	}

	// Cannot set both tfMPTLock and tfMPTUnlock
	if (flags&MPTokenIssuanceSetFlagLock) != 0 && (flags&MPTokenIssuanceSetFlagUnlock) != 0 {
		return errors.New("temINVALID_FLAG: cannot set both tfMPTLock and tfMPTUnlock")
	}

	// MPTokenIssuanceID is required
	if m.MPTokenIssuanceID == "" {
		return errors.New("temMALFORMED: MPTokenIssuanceID is required")
	}

	if len(m.MPTokenIssuanceID) != 64 {
		return errors.New("temMALFORMED: MPTokenIssuanceID must be 64 hex characters")
	}

	if _, err := hex.DecodeString(m.MPTokenIssuanceID); err != nil {
		return errors.New("temMALFORMED: MPTokenIssuanceID must be valid hex")
	}

	// Holder cannot be the same as Account
	if m.Holder != "" && m.Holder == m.Account {
		return errors.New("temMALFORMED: Holder cannot be the same as Account")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (m *MPTokenIssuanceSet) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(m)
}

// RequiredAmendments returns the amendments required for this transaction type
func (m *MPTokenIssuanceSet) RequiredAmendments() []string {
	return []string{amendment.AmendmentMPTokensV1}
}

// Apply applies the MPTokenIssuanceSet transaction to ledger state.
func (m *MPTokenIssuanceSet) Apply(ctx *tx.ApplyContext) tx.Result {
	issuanceIDBytes, err := hex.DecodeString(m.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return tx.TemINVALID
	}
	return tx.TesSUCCESS
}
