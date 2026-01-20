package tx

import "errors"

// VaultCreate creates a new vault.
type VaultCreate struct {
	BaseTx

	// Asset is the asset the vault holds (required)
	Asset Asset `json:"Asset"`

	// Data is arbitrary data (optional)
	Data string `json:"Data,omitempty"`

	// DomainID is the permissioned domain ID (optional)
	DomainID string `json:"DomainID,omitempty"`

	// WithdrawalPolicy configures withdrawal rules (optional)
	WithdrawalPolicy *Amount `json:"WithdrawalPolicy,omitempty"`
}

// NewVaultCreate creates a new VaultCreate transaction
func NewVaultCreate(account string, asset Asset) *VaultCreate {
	return &VaultCreate{
		BaseTx: *NewBaseTx(TypeVaultCreate, account),
		Asset:  asset,
	}
}

// TxType returns the transaction type
func (v *VaultCreate) TxType() Type {
	return TypeVaultCreate
}

// Validate validates the VaultCreate transaction
func (v *VaultCreate) Validate() error {
	if err := v.BaseTx.Validate(); err != nil {
		return err
	}

	if v.Asset.Currency == "" {
		return errors.New("Asset is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (v *VaultCreate) Flatten() (map[string]any, error) {
	m := v.Common.ToMap()

	m["Asset"] = v.Asset

	if v.Data != "" {
		m["Data"] = v.Data
	}
	if v.DomainID != "" {
		m["DomainID"] = v.DomainID
	}
	if v.WithdrawalPolicy != nil {
		m["WithdrawalPolicy"] = flattenAmount(*v.WithdrawalPolicy)
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (v *VaultCreate) RequiredAmendments() []string {
	return []string{AmendmentSingleAssetVault}
}

// VaultSet modifies a vault.
type VaultSet struct {
	BaseTx

	// VaultID is the ID of the vault to modify (required)
	VaultID string `json:"VaultID"`

	// Data is arbitrary data (optional)
	Data string `json:"Data,omitempty"`

	// DomainID is the permissioned domain ID (optional)
	DomainID string `json:"DomainID,omitempty"`
}

// VaultSet flags
const (
	// tfVaultPrivate makes the vault private
	VaultSetFlagPrivate uint32 = 0x00000001
)

// NewVaultSet creates a new VaultSet transaction
func NewVaultSet(account, vaultID string) *VaultSet {
	return &VaultSet{
		BaseTx:  *NewBaseTx(TypeVaultSet, account),
		VaultID: vaultID,
	}
}

// TxType returns the transaction type
func (v *VaultSet) TxType() Type {
	return TypeVaultSet
}

// Validate validates the VaultSet transaction
func (v *VaultSet) Validate() error {
	if err := v.BaseTx.Validate(); err != nil {
		return err
	}

	if v.VaultID == "" {
		return errors.New("VaultID is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (v *VaultSet) Flatten() (map[string]any, error) {
	m := v.Common.ToMap()

	m["VaultID"] = v.VaultID

	if v.Data != "" {
		m["Data"] = v.Data
	}
	if v.DomainID != "" {
		m["DomainID"] = v.DomainID
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (v *VaultSet) RequiredAmendments() []string {
	return []string{AmendmentSingleAssetVault}
}

// VaultDelete deletes a vault.
type VaultDelete struct {
	BaseTx

	// VaultID is the ID of the vault to delete (required)
	VaultID string `json:"VaultID"`
}

// NewVaultDelete creates a new VaultDelete transaction
func NewVaultDelete(account, vaultID string) *VaultDelete {
	return &VaultDelete{
		BaseTx:  *NewBaseTx(TypeVaultDelete, account),
		VaultID: vaultID,
	}
}

// TxType returns the transaction type
func (v *VaultDelete) TxType() Type {
	return TypeVaultDelete
}

// Validate validates the VaultDelete transaction
func (v *VaultDelete) Validate() error {
	if err := v.BaseTx.Validate(); err != nil {
		return err
	}

	if v.VaultID == "" {
		return errors.New("VaultID is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (v *VaultDelete) Flatten() (map[string]any, error) {
	m := v.Common.ToMap()
	m["VaultID"] = v.VaultID
	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (v *VaultDelete) RequiredAmendments() []string {
	return []string{AmendmentSingleAssetVault}
}

// VaultDeposit deposits assets into a vault.
type VaultDeposit struct {
	BaseTx

	// VaultID is the ID of the vault (required)
	VaultID string `json:"VaultID"`

	// Amount is the amount to deposit (required)
	Amount Amount `json:"Amount"`
}

// NewVaultDeposit creates a new VaultDeposit transaction
func NewVaultDeposit(account, vaultID string, amount Amount) *VaultDeposit {
	return &VaultDeposit{
		BaseTx:  *NewBaseTx(TypeVaultDeposit, account),
		VaultID: vaultID,
		Amount:  amount,
	}
}

// TxType returns the transaction type
func (v *VaultDeposit) TxType() Type {
	return TypeVaultDeposit
}

// Validate validates the VaultDeposit transaction
func (v *VaultDeposit) Validate() error {
	if err := v.BaseTx.Validate(); err != nil {
		return err
	}

	if v.VaultID == "" {
		return errors.New("VaultID is required")
	}

	if v.Amount.Value == "" {
		return errors.New("Amount is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (v *VaultDeposit) Flatten() (map[string]any, error) {
	m := v.Common.ToMap()

	m["VaultID"] = v.VaultID
	m["Amount"] = flattenAmount(v.Amount)

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (v *VaultDeposit) RequiredAmendments() []string {
	return []string{AmendmentSingleAssetVault}
}

// VaultWithdraw withdraws assets from a vault.
type VaultWithdraw struct {
	BaseTx

	// VaultID is the ID of the vault (required)
	VaultID string `json:"VaultID"`

	// Amount is the amount to withdraw (required)
	Amount Amount `json:"Amount"`

	// Destination is the destination account (optional)
	Destination string `json:"Destination,omitempty"`
}

// NewVaultWithdraw creates a new VaultWithdraw transaction
func NewVaultWithdraw(account, vaultID string, amount Amount) *VaultWithdraw {
	return &VaultWithdraw{
		BaseTx:  *NewBaseTx(TypeVaultWithdraw, account),
		VaultID: vaultID,
		Amount:  amount,
	}
}

// TxType returns the transaction type
func (v *VaultWithdraw) TxType() Type {
	return TypeVaultWithdraw
}

// Validate validates the VaultWithdraw transaction
func (v *VaultWithdraw) Validate() error {
	if err := v.BaseTx.Validate(); err != nil {
		return err
	}

	if v.VaultID == "" {
		return errors.New("VaultID is required")
	}

	if v.Amount.Value == "" {
		return errors.New("Amount is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (v *VaultWithdraw) Flatten() (map[string]any, error) {
	m := v.Common.ToMap()

	m["VaultID"] = v.VaultID
	m["Amount"] = flattenAmount(v.Amount)

	if v.Destination != "" {
		m["Destination"] = v.Destination
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (v *VaultWithdraw) RequiredAmendments() []string {
	return []string{AmendmentSingleAssetVault}
}

// VaultClawback claws back assets from a vault.
type VaultClawback struct {
	BaseTx

	// VaultID is the ID of the vault (required)
	VaultID string `json:"VaultID"`

	// Holder is the holder to claw back from (required)
	Holder string `json:"Holder"`

	// Amount is the amount to claw back (optional)
	Amount *Amount `json:"Amount,omitempty"`
}

// NewVaultClawback creates a new VaultClawback transaction
func NewVaultClawback(account, vaultID, holder string) *VaultClawback {
	return &VaultClawback{
		BaseTx:  *NewBaseTx(TypeVaultClawback, account),
		VaultID: vaultID,
		Holder:  holder,
	}
}

// TxType returns the transaction type
func (v *VaultClawback) TxType() Type {
	return TypeVaultClawback
}

// Validate validates the VaultClawback transaction
func (v *VaultClawback) Validate() error {
	if err := v.BaseTx.Validate(); err != nil {
		return err
	}

	if v.VaultID == "" {
		return errors.New("VaultID is required")
	}

	if v.Holder == "" {
		return errors.New("Holder is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (v *VaultClawback) Flatten() (map[string]any, error) {
	m := v.Common.ToMap()

	m["VaultID"] = v.VaultID
	m["Holder"] = v.Holder

	if v.Amount != nil {
		m["Amount"] = flattenAmount(*v.Amount)
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (v *VaultClawback) RequiredAmendments() []string {
	return []string{AmendmentSingleAssetVault}
}
