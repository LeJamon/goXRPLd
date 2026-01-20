package tx

import "errors"

// MPTokenIssuanceCreate creates a new multi-purpose token issuance.
type MPTokenIssuanceCreate struct {
	BaseTx

	// AssetScale is the scale for the token (optional)
	AssetScale *uint8 `json:"AssetScale,omitempty"`

	// MaximumAmount is the maximum amount that can be issued (optional)
	MaximumAmount *uint64 `json:"MaximumAmount,omitempty"`

	// TransferFee is the fee for transfers (0-50000, where 50000 = 50%)
	TransferFee *uint16 `json:"TransferFee,omitempty"`

	// MPTokenMetadata is metadata for the token (optional)
	MPTokenMetadata string `json:"MPTokenMetadata,omitempty"`
}

// MPTokenIssuanceCreate flags
const (
	// lsfMPTLocked indicates the token is locked
	MPTokenIssuanceCreateFlagLocked uint32 = 0x00000001
	// lsfMPTCanLock allows locking the token
	MPTokenIssuanceCreateFlagCanLock uint32 = 0x00000002
	// lsfMPTRequireAuth requires authorization
	MPTokenIssuanceCreateFlagRequireAuth uint32 = 0x00000004
	// lsfMPTCanEscrow allows escrow
	MPTokenIssuanceCreateFlagCanEscrow uint32 = 0x00000008
	// lsfMPTCanTrade allows trading
	MPTokenIssuanceCreateFlagCanTrade uint32 = 0x00000010
	// lsfMPTCanTransfer allows transfer
	MPTokenIssuanceCreateFlagCanTransfer uint32 = 0x00000020
	// lsfMPTCanClawback allows clawback
	MPTokenIssuanceCreateFlagCanClawback uint32 = 0x00000040
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
func (m *MPTokenIssuanceCreate) Validate() error {
	if err := m.BaseTx.Validate(); err != nil {
		return err
	}

	// TransferFee must be <= 50000
	if m.TransferFee != nil && *m.TransferFee > 50000 {
		return errors.New("TransferFee cannot exceed 50000")
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
	MPTokenIssuanceID string `json:"MPTokenIssuanceID"`
}

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
func (m *MPTokenIssuanceDestroy) Validate() error {
	if err := m.BaseTx.Validate(); err != nil {
		return err
	}

	if m.MPTokenIssuanceID == "" {
		return errors.New("MPTokenIssuanceID is required")
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
	Holder string `json:"Holder,omitempty"`
}

// MPTokenIssuanceSet flags
const (
	// tfMPTLock locks the token
	MPTokenIssuanceSetFlagLock uint32 = 0x00000001
	// tfMPTUnlock unlocks the token
	MPTokenIssuanceSetFlagUnlock uint32 = 0x00000002
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
func (m *MPTokenIssuanceSet) Validate() error {
	if err := m.BaseTx.Validate(); err != nil {
		return err
	}

	if m.MPTokenIssuanceID == "" {
		return errors.New("MPTokenIssuanceID is required")
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
	Holder string `json:"Holder,omitempty"`
}

// MPTokenAuthorize flags
const (
	// tfMPTUnauthorize unauthorizes the holder
	MPTokenAuthorizeFlagUnauthorize uint32 = 0x00000001
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
func (m *MPTokenAuthorize) Validate() error {
	if err := m.BaseTx.Validate(); err != nil {
		return err
	}

	if m.MPTokenIssuanceID == "" {
		return errors.New("MPTokenIssuanceID is required")
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
