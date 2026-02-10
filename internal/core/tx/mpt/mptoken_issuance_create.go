package mpt

import (
	"encoding/hex"
	"errors"
	
	"github.com/LeJamon/goXRPLd/internal/core/ledger/entry"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/amendment"
)

func init() {
	tx.Register(tx.TypeMPTokenIssuanceCreate, func() tx.Transaction {
		return &MPTokenIssuanceCreate{BaseTx: *tx.NewBaseTx(tx.TypeMPTokenIssuanceCreate, "")}
	})
}

// MPTokenIssuanceCreate creates a new multi-purpose token issuance.
type MPTokenIssuanceCreate struct {
	tx.BaseTx

	// AssetScale is the scale for the token (0-10, decimal places)
	AssetScale *uint8 `json:"AssetScale,omitempty" xrpl:"AssetScale,omitempty"`

	// MaximumAmount is the maximum amount that can be issued (optional)
	// Must be within unsigned 63-bit range (0x7FFFFFFFFFFFFFFF)
	MaximumAmount *uint64 `json:"MaximumAmount,omitempty" xrpl:"MaximumAmount,omitempty"`

	// TransferFee is the fee for transfers (0-50000, where 50000 = 50%)
	TransferFee *uint16 `json:"TransferFee,omitempty" xrpl:"TransferFee,omitempty"`

	// MPTokenMetadata is metadata for the token (optional, 1-1024 bytes as hex)
	MPTokenMetadata string `json:"MPTokenMetadata,omitempty" xrpl:"MPTokenMetadata,omitempty"`
}

// NewMPTokenIssuanceCreate creates a new MPTokenIssuanceCreate transaction
func NewMPTokenIssuanceCreate(account string) *MPTokenIssuanceCreate {
	return &MPTokenIssuanceCreate{
		BaseTx: *tx.NewBaseTx(tx.TypeMPTokenIssuanceCreate, account),
	}
}

// TxType returns the transaction type
func (m *MPTokenIssuanceCreate) TxType() tx.Type {
	return tx.TypeMPTokenIssuanceCreate
}

// Validate validates the MPTokenIssuanceCreate transaction
// Reference: rippled MPTokenIssuanceCreate.cpp preflight
func (m *MPTokenIssuanceCreate) Validate() error {
	if err := m.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags
	// Any flags other than the valid ones are not allowed
	flags := m.GetFlags()
	if flags&^tfMPTokenIssuanceCreateValidMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for MPTokenIssuanceCreate")
	}

	// Validate TransferFee
	if m.TransferFee != nil {
		if *m.TransferFee > entry.MaxTransferFee {
			return errors.New("temBAD_TRANSFER_FEE: TransferFee cannot exceed 50000")
		}
		// If a non-zero TransferFee is set, tfMPTCanTransfer must also be set
		if *m.TransferFee > 0 && (flags&MPTokenIssuanceCreateFlagCanTransfer) == 0 {
			return errors.New("temMALFORMED: TransferFee requires tfMPTCanTransfer flag")
		}
	}

	// Validate MPTokenMetadata
	if m.MPTokenMetadata != "" {
		metadataBytes, err := hex.DecodeString(m.MPTokenMetadata)
		if err != nil {
			return errors.New("temMALFORMED: MPTokenMetadata must be valid hex")
		}
		if len(metadataBytes) == 0 || len(metadataBytes) > entry.MaxMPTokenMetadataLength {
			return errors.New("temMALFORMED: MPTokenMetadata length must be 1-1024 bytes")
		}
	}

	// Validate MaximumAmount
	if m.MaximumAmount != nil {
		if *m.MaximumAmount == 0 {
			return errors.New("temMALFORMED: MaximumAmount cannot be zero")
		}
		if *m.MaximumAmount > entry.MaxMPTokenAmount {
			return errors.New("temMALFORMED: MaximumAmount exceeds maximum allowed")
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (m *MPTokenIssuanceCreate) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(m)
}

// RequiredAmendments returns the amendments required for this transaction type
func (m *MPTokenIssuanceCreate) RequiredAmendments() [][32]byte {
	return [][32]byte{amendment.FeatureMPTokensV1}
}

// Apply applies the MPTokenIssuanceCreate transaction to ledger state.
func (m *MPTokenIssuanceCreate) Apply(ctx *tx.ApplyContext) tx.Result {
	ctx.Account.OwnerCount++
	return tx.TesSUCCESS
}
