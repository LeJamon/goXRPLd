package mpt

import (
	"encoding/hex"
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
)

func init() {
	tx.Register(tx.TypeMPTokenAuthorize, func() tx.Transaction {
		return &MPTokenAuthorize{BaseTx: *tx.NewBaseTx(tx.TypeMPTokenAuthorize, "")}
	})
}

// MPTokenAuthorize authorizes or unauthorizes MPToken operations.
type MPTokenAuthorize struct {
	tx.BaseTx

	// MPTokenIssuanceID is the ID of the issuance (required)
	MPTokenIssuanceID string `json:"MPTokenIssuanceID" xrpl:"MPTokenIssuanceID"`

	// Holder is the holder account (optional)
	// When the issuer submits: Holder specifies which account to authorize/unauthorize
	// When a holder submits: Holder should not be set (or set to own account to delete)
	Holder string `json:"Holder,omitempty" xrpl:"Holder,omitempty"`
}

// NewMPTokenAuthorize creates a new MPTokenAuthorize transaction
func NewMPTokenAuthorize(account, issuanceID string) *MPTokenAuthorize {
	return &MPTokenAuthorize{
		BaseTx:            *tx.NewBaseTx(tx.TypeMPTokenAuthorize, account),
		MPTokenIssuanceID: issuanceID,
	}
}

// TxType returns the transaction type
func (m *MPTokenAuthorize) TxType() tx.Type {
	return tx.TypeMPTokenAuthorize
}

// Validate validates the MPTokenAuthorize transaction
// Reference: rippled MPTokenAuthorize.cpp preflight
func (m *MPTokenAuthorize) Validate() error {
	if err := m.BaseTx.Validate(); err != nil {
		return err
	}

	flags := m.GetFlags()

	// Check for invalid flags
	if flags&^tfMPTokenAuthorizeValidMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for MPTokenAuthorize")
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
func (m *MPTokenAuthorize) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(m)
}

// RequiredAmendments returns the amendments required for this transaction type
func (m *MPTokenAuthorize) RequiredAmendments() []string {
	return []string{amendment.AmendmentMPTokensV1}
}

// Apply applies the MPTokenAuthorize transaction to ledger state.
func (m *MPTokenAuthorize) Apply(ctx *tx.ApplyContext) tx.Result {
	issuanceIDBytes, err := hex.DecodeString(m.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return tx.TemINVALID
	}
	flags := m.GetFlags()
	if flags&MPTokenAuthorizeFlagUnauthorize != 0 {
		if ctx.Account.OwnerCount > 0 {
			ctx.Account.OwnerCount--
		}
	} else {
		ctx.Account.OwnerCount++
	}
	return tx.TesSUCCESS
}
