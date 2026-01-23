package tx

import (
	"encoding/hex"
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/entry"
)

// MPTokenIssuanceCreate creates a new multi-purpose token issuance.
type MPTokenIssuanceCreate struct {
	BaseTx

	// AssetScale is the scale for the token (0-10, decimal places)
	AssetScale *uint8 `json:"AssetScale,omitempty"`

	// MaximumAmount is the maximum amount that can be issued (optional)
	// Must be within unsigned 63-bit range (0x7FFFFFFFFFFFFFFF)
	MaximumAmount *uint64 `json:"MaximumAmount,omitempty"`

	// TransferFee is the fee for transfers (0-50000, where 50000 = 50%)
	TransferFee *uint16 `json:"TransferFee,omitempty"`

	// MPTokenMetadata is metadata for the token (optional, 1-1024 bytes as hex)
	MPTokenMetadata string `json:"MPTokenMetadata,omitempty"`
}

// MPTokenIssuanceCreate flags (transaction flags, tf prefix)
// Reference: rippled TxFlags.h
const (
	// tfMPTCanLock allows the issuer to lock tokens
	MPTokenIssuanceCreateFlagCanLock uint32 = 0x00000002
	// tfMPTRequireAuth requires holder authorization
	MPTokenIssuanceCreateFlagRequireAuth uint32 = 0x00000004
	// tfMPTCanEscrow allows escrow
	MPTokenIssuanceCreateFlagCanEscrow uint32 = 0x00000008
	// tfMPTCanTrade allows trading on DEX
	MPTokenIssuanceCreateFlagCanTrade uint32 = 0x00000010
	// tfMPTCanTransfer allows transfers
	MPTokenIssuanceCreateFlagCanTransfer uint32 = 0x00000020
	// tfMPTCanClawback allows issuer clawback
	MPTokenIssuanceCreateFlagCanClawback uint32 = 0x00000040
)

// MPTokenIssuanceCreate flag mask
const (
	tfUniversal                     uint32 = 0x80000000
	tfMPTokenIssuanceCreateValidMask uint32 = tfUniversal |
		MPTokenIssuanceCreateFlagCanLock |
		MPTokenIssuanceCreateFlagRequireAuth |
		MPTokenIssuanceCreateFlagCanEscrow |
		MPTokenIssuanceCreateFlagCanTrade |
		MPTokenIssuanceCreateFlagCanTransfer |
		MPTokenIssuanceCreateFlagCanClawback
)

// NewMPTokenIssuanceCreate creates a new MPTokenIssuanceCreate transaction
func NewMPTokenIssuanceCreate(account string) *MPTokenIssuanceCreate {
	return &MPTokenIssuanceCreate{
		BaseTx: *NewBaseTx(TypeMPTokenIssuanceCreate, account),
	}
}

// TxType returns the transaction type
func (m *MPTokenIssuanceCreate) TxType() Type {
	return TypeMPTokenIssuanceCreate
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
	result := m.Common.ToMap()

	if m.AssetScale != nil {
		result["AssetScale"] = *m.AssetScale
	}
	if m.MaximumAmount != nil {
		result["MaximumAmount"] = *m.MaximumAmount
	}
	if m.TransferFee != nil {
		result["TransferFee"] = *m.TransferFee
	}
	if m.MPTokenMetadata != "" {
		result["MPTokenMetadata"] = m.MPTokenMetadata
	}

	return result, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (m *MPTokenIssuanceCreate) RequiredAmendments() []string {
	return []string{AmendmentMPTokensV1}
}

// MPTokenIssuanceDestroy destroys a multi-purpose token issuance.
type MPTokenIssuanceDestroy struct {
	BaseTx

	// MPTokenIssuanceID is the ID of the issuance to destroy (required)
	// 64-character hex string (32 bytes)
	MPTokenIssuanceID string `json:"MPTokenIssuanceID"`
}

// MPTokenIssuanceDestroy flag mask (only universal flags allowed)
const (
	tfMPTokenIssuanceDestroyValidMask uint32 = tfUniversal
)

// NewMPTokenIssuanceDestroy creates a new MPTokenIssuanceDestroy transaction
func NewMPTokenIssuanceDestroy(account, issuanceID string) *MPTokenIssuanceDestroy {
	return &MPTokenIssuanceDestroy{
		BaseTx:            *NewBaseTx(TypeMPTokenIssuanceDestroy, account),
		MPTokenIssuanceID: issuanceID,
	}
}

// TxType returns the transaction type
func (m *MPTokenIssuanceDestroy) TxType() Type {
	return TypeMPTokenIssuanceDestroy
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
	result := m.Common.ToMap()
	result["MPTokenIssuanceID"] = m.MPTokenIssuanceID
	return result, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (m *MPTokenIssuanceDestroy) RequiredAmendments() []string {
	return []string{AmendmentMPTokensV1}
}

// MPTokenIssuanceSet modifies a multi-purpose token issuance.
type MPTokenIssuanceSet struct {
	BaseTx

	// MPTokenIssuanceID is the ID of the issuance (required)
	MPTokenIssuanceID string `json:"MPTokenIssuanceID"`

	// Holder is the holder account (optional)
	// When set, the issuer is modifying a specific holder's MPToken
	Holder string `json:"Holder,omitempty"`
}

// MPTokenIssuanceSet flags (transaction flags, tf prefix)
const (
	// tfMPTLock locks the token (sets lsfMPTLocked)
	MPTokenIssuanceSetFlagLock uint32 = 0x00000001
	// tfMPTUnlock unlocks the token (clears lsfMPTLocked)
	MPTokenIssuanceSetFlagUnlock uint32 = 0x00000002
)

// MPTokenIssuanceSet flag mask
const (
	tfMPTokenIssuanceSetValidMask uint32 = tfUniversal |
		MPTokenIssuanceSetFlagLock |
		MPTokenIssuanceSetFlagUnlock
)

// NewMPTokenIssuanceSet creates a new MPTokenIssuanceSet transaction
func NewMPTokenIssuanceSet(account, issuanceID string) *MPTokenIssuanceSet {
	return &MPTokenIssuanceSet{
		BaseTx:            *NewBaseTx(TypeMPTokenIssuanceSet, account),
		MPTokenIssuanceID: issuanceID,
	}
}

// TxType returns the transaction type
func (m *MPTokenIssuanceSet) TxType() Type {
	return TypeMPTokenIssuanceSet
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
	result := m.Common.ToMap()

	result["MPTokenIssuanceID"] = m.MPTokenIssuanceID

	if m.Holder != "" {
		result["Holder"] = m.Holder
	}

	return result, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (m *MPTokenIssuanceSet) RequiredAmendments() []string {
	return []string{AmendmentMPTokensV1}
}

// MPTokenAuthorize authorizes or unauthorizes MPToken operations.
type MPTokenAuthorize struct {
	BaseTx

	// MPTokenIssuanceID is the ID of the issuance (required)
	MPTokenIssuanceID string `json:"MPTokenIssuanceID"`

	// Holder is the holder account (optional)
	// When the issuer submits: Holder specifies which account to authorize/unauthorize
	// When a holder submits: Holder should not be set (or set to own account to delete)
	Holder string `json:"Holder,omitempty"`
}

// MPTokenAuthorize flags (transaction flags, tf prefix)
const (
	// tfMPTUnauthorize - holder wants to delete MPToken, or issuer wants to unauthorize holder
	MPTokenAuthorizeFlagUnauthorize uint32 = 0x00000001
)

// MPTokenAuthorize flag mask
const (
	tfMPTokenAuthorizeValidMask uint32 = tfUniversal | MPTokenAuthorizeFlagUnauthorize
)

// NewMPTokenAuthorize creates a new MPTokenAuthorize transaction
func NewMPTokenAuthorize(account, issuanceID string) *MPTokenAuthorize {
	return &MPTokenAuthorize{
		BaseTx:            *NewBaseTx(TypeMPTokenAuthorize, account),
		MPTokenIssuanceID: issuanceID,
	}
}

// TxType returns the transaction type
func (m *MPTokenAuthorize) TxType() Type {
	return TypeMPTokenAuthorize
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
	result := m.Common.ToMap()

	result["MPTokenIssuanceID"] = m.MPTokenIssuanceID

	if m.Holder != "" {
		result["Holder"] = m.Holder
	}

	return result, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (m *MPTokenAuthorize) RequiredAmendments() []string {
	return []string{AmendmentMPTokensV1}
}
