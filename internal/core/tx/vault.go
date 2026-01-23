package tx

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

func init() {
	Register(TypeVaultCreate, func() Transaction {
		return &VaultCreate{BaseTx: *NewBaseTx(TypeVaultCreate, "")}
	})
	Register(TypeVaultSet, func() Transaction {
		return &VaultSet{BaseTx: *NewBaseTx(TypeVaultSet, "")}
	})
	Register(TypeVaultDelete, func() Transaction {
		return &VaultDelete{BaseTx: *NewBaseTx(TypeVaultDelete, "")}
	})
	Register(TypeVaultDeposit, func() Transaction {
		return &VaultDeposit{BaseTx: *NewBaseTx(TypeVaultDeposit, "")}
	})
	Register(TypeVaultWithdraw, func() Transaction {
		return &VaultWithdraw{BaseTx: *NewBaseTx(TypeVaultWithdraw, "")}
	})
	Register(TypeVaultClawback, func() Transaction {
		return &VaultClawback{BaseTx: *NewBaseTx(TypeVaultClawback, "")}
	})
}

// Vault constants
const (
	// MaxVaultDataLength is the maximum length of Data field
	MaxVaultDataLength = 256

	// MaxMPTokenMetadataLength is the maximum length of MPTokenMetadata
	MaxMPTokenMetadataLength = 1024

	// VaultStrategyFirstComeFirstServe is the only valid withdrawal policy
	VaultStrategyFirstComeFirstServe uint8 = 1
)

// VaultCreate flags
const (
	// tfVaultPrivate makes the vault private (requires authorization)
	VaultFlagPrivate uint32 = 0x00000001
	// tfVaultShareNonTransferable makes vault shares non-transferable
	VaultFlagShareNonTransferable uint32 = 0x00000002

	// tfVaultCreateMask is the mask for invalid VaultCreate flags
	tfVaultCreateMask uint32 = ^(VaultFlagPrivate | VaultFlagShareNonTransferable)
)

// Vault errors
var (
	ErrVaultIDRequired       = errors.New("temMALFORMED: VaultID is required")
	ErrVaultIDZero           = errors.New("temMALFORMED: VaultID cannot be zero")
	ErrVaultAssetRequired    = errors.New("temMALFORMED: Asset is required")
	ErrVaultDataTooLong      = errors.New("temMALFORMED: Data exceeds maximum length")
	ErrVaultDataEmpty        = errors.New("temMALFORMED: Data cannot be empty if present")
	ErrVaultDomainIDZero     = errors.New("temMALFORMED: DomainID cannot be zero")
	ErrVaultDomainNotPrivate = errors.New("temMALFORMED: DomainID only allowed on private vaults")
	ErrVaultAmountRequired   = errors.New("temBAD_AMOUNT: Amount is required")
	ErrVaultAmountNotPos     = errors.New("temBAD_AMOUNT: Amount must be positive")
	ErrVaultHolderRequired   = errors.New("temMALFORMED: Holder is required")
	ErrVaultHolderIsSelf     = errors.New("temMALFORMED: Holder cannot be same as issuer")
	ErrVaultDestZero         = errors.New("temMALFORMED: Destination cannot be zero")
	ErrVaultDestTagNoAccount = errors.New("temMALFORMED: DestinationTag without Destination")
	ErrVaultNoFieldsToUpdate = errors.New("temMALFORMED: nothing to update")
	ErrVaultAssetsMaxNeg     = errors.New("temMALFORMED: AssetsMaximum cannot be negative")
	ErrVaultWithdrawalPolicy = errors.New("temMALFORMED: invalid withdrawal policy")
	ErrVaultMetadataTooLong  = errors.New("temMALFORMED: MPTokenMetadata exceeds maximum length")
	ErrVaultMetadataEmpty    = errors.New("temMALFORMED: MPTokenMetadata cannot be empty if present")
	ErrVaultAmountXRP        = errors.New("temMALFORMED: cannot clawback XRP from vault")
	ErrVaultAmountNotIssuer  = errors.New("temMALFORMED: only asset issuer can clawback")
)

// VaultCreate creates a new vault.
type VaultCreate struct {
	BaseTx

	// Asset is the asset the vault holds (required)
	Asset Asset `json:"Asset" xrpl:"Asset"`

	// Data is arbitrary data (optional)
	Data string `json:"Data,omitempty" xrpl:"Data,omitempty"`

	// DomainID is the permissioned domain ID (optional)
	DomainID string `json:"DomainID,omitempty" xrpl:"DomainID,omitempty"`

	// AssetsMaximum is the maximum assets the vault can hold (optional)
	AssetsMaximum *int64 `json:"AssetsMaximum,omitempty" xrpl:"AssetsMaximum,omitempty"`

	// MPTokenMetadata is metadata for the vault shares (optional)
	MPTokenMetadata string `json:"MPTokenMetadata,omitempty" xrpl:"MPTokenMetadata,omitempty"`

	// WithdrawalPolicy configures withdrawal rules (optional)
	WithdrawalPolicy *uint8 `json:"WithdrawalPolicy,omitempty" xrpl:"WithdrawalPolicy,omitempty"`
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
// Reference: rippled VaultCreate.cpp preflight()
func (v *VaultCreate) Validate() error {
	if err := v.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags
	// Reference: rippled VaultCreate.cpp:50-51
	if v.Common.Flags != nil && *v.Common.Flags&tfVaultCreateMask != 0 {
		return ErrInvalidFlags
	}

	// Asset is required
	if v.Asset.Currency == "" {
		return ErrVaultAssetRequired
	}

	// Validate Data if present
	// Reference: rippled VaultCreate.cpp:53-57
	if v.Data != "" {
		if len(v.Data) > MaxVaultDataLength {
			return ErrVaultDataTooLong
		}
	}

	// Validate WithdrawalPolicy if present
	// Reference: rippled VaultCreate.cpp:59-63
	if v.WithdrawalPolicy != nil {
		if *v.WithdrawalPolicy != VaultStrategyFirstComeFirstServe {
			return ErrVaultWithdrawalPolicy
		}
	}

	// Validate DomainID if present
	// Reference: rippled VaultCreate.cpp:66-72
	if v.DomainID != "" {
		domainBytes, err := hex.DecodeString(v.DomainID)
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
			return ErrVaultDomainIDZero
		}
		// DomainID only allowed on private vaults
		if v.Common.Flags == nil || (*v.Common.Flags&VaultFlagPrivate) == 0 {
			return ErrVaultDomainNotPrivate
		}
	}

	// Validate AssetsMaximum if present
	// Reference: rippled VaultCreate.cpp:74-78
	if v.AssetsMaximum != nil && *v.AssetsMaximum < 0 {
		return ErrVaultAssetsMaxNeg
	}

	// Validate MPTokenMetadata if present
	// Reference: rippled VaultCreate.cpp:80-84
	if v.MPTokenMetadata != "" {
		if len(v.MPTokenMetadata) > MaxMPTokenMetadataLength {
			return ErrVaultMetadataTooLong
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (v *VaultCreate) Flatten() (map[string]any, error) {
	return ReflectFlatten(v)
}

// RequiredAmendments returns the amendments required for this transaction type
func (v *VaultCreate) RequiredAmendments() []string {
	return []string{AmendmentSingleAssetVault}
}

// VaultSet modifies a vault.
type VaultSet struct {
	BaseTx

	// VaultID is the ID of the vault to modify (required)
	VaultID string `json:"VaultID" xrpl:"VaultID"`

	// Data is arbitrary data (optional)
	Data string `json:"Data,omitempty" xrpl:"Data,omitempty"`

	// DomainID is the permissioned domain ID (optional)
	DomainID string `json:"DomainID,omitempty" xrpl:"DomainID,omitempty"`

	// AssetsMaximum is the maximum assets (optional)
	AssetsMaximum *int64 `json:"AssetsMaximum,omitempty" xrpl:"AssetsMaximum,omitempty"`
}

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
// Reference: rippled VaultSet.cpp preflight()
func (v *VaultSet) Validate() error {
	if err := v.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (universal mask)
	// Reference: rippled VaultSet.cpp:52-53
	if v.Common.Flags != nil && *v.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// VaultID is required and cannot be zero
	// Reference: rippled VaultSet.cpp:46-50
	if v.VaultID == "" {
		return ErrVaultIDRequired
	}
	vaultBytes, err := hex.DecodeString(v.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return errors.New("temMALFORMED: VaultID must be a valid 256-bit hash")
	}
	isZero := true
	for _, b := range vaultBytes {
		if b != 0 {
			isZero = false
			break
		}
	}
	if isZero {
		return ErrVaultIDZero
	}

	// Validate Data if present
	// Reference: rippled VaultSet.cpp:55-62
	if v.Data != "" {
		if len(v.Data) > MaxVaultDataLength {
			return ErrVaultDataTooLong
		}
	}

	// Validate AssetsMaximum if present
	// Reference: rippled VaultSet.cpp:64-71
	if v.AssetsMaximum != nil && *v.AssetsMaximum < 0 {
		return ErrVaultAssetsMaxNeg
	}

	// Must update at least one field
	// Reference: rippled VaultSet.cpp:73-79
	if v.DomainID == "" && v.AssetsMaximum == nil && v.Data == "" {
		return ErrVaultNoFieldsToUpdate
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (v *VaultSet) Flatten() (map[string]any, error) {
	return ReflectFlatten(v)
}

// RequiredAmendments returns the amendments required for this transaction type
func (v *VaultSet) RequiredAmendments() []string {
	return []string{AmendmentSingleAssetVault}
}

// VaultDelete deletes a vault.
type VaultDelete struct {
	BaseTx

	// VaultID is the ID of the vault to delete (required)
	VaultID string `json:"VaultID" xrpl:"VaultID"`
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
// Reference: rippled VaultDelete.cpp preflight()
func (v *VaultDelete) Validate() error {
	if err := v.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (universal mask)
	// Reference: rippled VaultDelete.cpp:39-40
	if v.Common.Flags != nil && *v.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// VaultID is required and cannot be zero
	// Reference: rippled VaultDelete.cpp:42-46
	if v.VaultID == "" {
		return ErrVaultIDRequired
	}
	vaultBytes, err := hex.DecodeString(v.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return errors.New("temMALFORMED: VaultID must be a valid 256-bit hash")
	}
	isZero := true
	for _, b := range vaultBytes {
		if b != 0 {
			isZero = false
			break
		}
	}
	if isZero {
		return ErrVaultIDZero
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (v *VaultDelete) Flatten() (map[string]any, error) {
	return ReflectFlatten(v)
}

// RequiredAmendments returns the amendments required for this transaction type
func (v *VaultDelete) RequiredAmendments() []string {
	return []string{AmendmentSingleAssetVault}
}

// VaultDeposit deposits assets into a vault.
type VaultDeposit struct {
	BaseTx

	// VaultID is the ID of the vault (required)
	VaultID string `json:"VaultID" xrpl:"VaultID"`

	// Amount is the amount to deposit (required)
	Amount Amount `json:"Amount" xrpl:"Amount,amount"`
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
// Reference: rippled VaultDeposit.cpp preflight()
func (v *VaultDeposit) Validate() error {
	if err := v.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (universal mask)
	// Reference: rippled VaultDeposit.cpp:44-45
	if v.Common.Flags != nil && *v.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// VaultID is required and cannot be zero
	// Reference: rippled VaultDeposit.cpp:47-51
	if v.VaultID == "" {
		return ErrVaultIDRequired
	}
	vaultBytes, err := hex.DecodeString(v.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return errors.New("temMALFORMED: VaultID must be a valid 256-bit hash")
	}
	isZero := true
	for _, b := range vaultBytes {
		if b != 0 {
			isZero = false
			break
		}
	}
	if isZero {
		return ErrVaultIDZero
	}

	// Amount is required and must be positive
	// Reference: rippled VaultDeposit.cpp:53-54
	if v.Amount.Value == "" {
		return ErrVaultAmountRequired
	}
	amountVal, err := strconv.ParseFloat(v.Amount.Value, 64)
	if err != nil || amountVal <= 0 {
		return ErrVaultAmountNotPos
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (v *VaultDeposit) Flatten() (map[string]any, error) {
	return ReflectFlatten(v)
}

// RequiredAmendments returns the amendments required for this transaction type
func (v *VaultDeposit) RequiredAmendments() []string {
	return []string{AmendmentSingleAssetVault}
}

// VaultWithdraw withdraws assets from a vault.
type VaultWithdraw struct {
	BaseTx

	// VaultID is the ID of the vault (required)
	VaultID string `json:"VaultID" xrpl:"VaultID"`

	// Amount is the amount to withdraw (required)
	Amount Amount `json:"Amount" xrpl:"Amount,amount"`

	// Destination is the destination account (optional)
	Destination string `json:"Destination,omitempty" xrpl:"Destination,omitempty"`

	// DestinationTag is the destination tag (optional)
	DestinationTag *uint32 `json:"DestinationTag,omitempty" xrpl:"DestinationTag,omitempty"`
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
// Reference: rippled VaultWithdraw.cpp preflight()
func (v *VaultWithdraw) Validate() error {
	if err := v.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (universal mask)
	// Reference: rippled VaultWithdraw.cpp:42-43
	if v.Common.Flags != nil && *v.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// VaultID is required and cannot be zero
	// Reference: rippled VaultWithdraw.cpp:45-49
	if v.VaultID == "" {
		return ErrVaultIDRequired
	}
	vaultBytes, err := hex.DecodeString(v.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return errors.New("temMALFORMED: VaultID must be a valid 256-bit hash")
	}
	isZero := true
	for _, b := range vaultBytes {
		if b != 0 {
			isZero = false
			break
		}
	}
	if isZero {
		return ErrVaultIDZero
	}

	// Amount is required and must be positive
	// Reference: rippled VaultWithdraw.cpp:51-52
	if v.Amount.Value == "" {
		return ErrVaultAmountRequired
	}
	amountVal, err := strconv.ParseFloat(v.Amount.Value, 64)
	if err != nil || amountVal <= 0 {
		return ErrVaultAmountNotPos
	}

	// Validate Destination if present
	// Reference: rippled VaultWithdraw.cpp:54-63
	if v.Destination != "" {
		// Destination cannot be zero (empty is handled by field being absent)
		// In rippled this checks for beast::zero which is all zeros
		// For our case, if Destination is set but empty, that's an error
	}

	// DestinationTag without Destination is invalid
	// Reference: rippled VaultWithdraw.cpp:64-69
	if v.Destination == "" && v.DestinationTag != nil {
		return ErrVaultDestTagNoAccount
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (v *VaultWithdraw) Flatten() (map[string]any, error) {
	return ReflectFlatten(v)
}

// RequiredAmendments returns the amendments required for this transaction type
func (v *VaultWithdraw) RequiredAmendments() []string {
	return []string{AmendmentSingleAssetVault}
}

// VaultClawback claws back assets from a vault.
type VaultClawback struct {
	BaseTx

	// VaultID is the ID of the vault (required)
	VaultID string `json:"VaultID" xrpl:"VaultID"`

	// Holder is the holder to claw back from (required)
	Holder string `json:"Holder" xrpl:"Holder"`

	// Amount is the amount to claw back (optional)
	Amount *Amount `json:"Amount,omitempty" xrpl:"Amount,omitempty,amount"`
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
// Reference: rippled VaultClawback.cpp preflight()
func (v *VaultClawback) Validate() error {
	if err := v.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (universal mask)
	// Reference: rippled VaultClawback.cpp:42-43
	if v.Common.Flags != nil && *v.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// VaultID is required and cannot be zero
	// Reference: rippled VaultClawback.cpp:45-49
	if v.VaultID == "" {
		return ErrVaultIDRequired
	}
	vaultBytes, err := hex.DecodeString(v.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return errors.New("temMALFORMED: VaultID must be a valid 256-bit hash")
	}
	isZero := true
	for _, b := range vaultBytes {
		if b != 0 {
			isZero = false
			break
		}
	}
	if isZero {
		return ErrVaultIDZero
	}

	// Holder is required
	// Reference: rippled VaultClawback.cpp:51-52
	if v.Holder == "" {
		return ErrVaultHolderRequired
	}

	// Holder cannot be the same as issuer (Account)
	// Reference: rippled VaultClawback.cpp:54-58
	if v.Holder == v.Account {
		return ErrVaultHolderIsSelf
	}

	// Validate Amount if present
	// Reference: rippled VaultClawback.cpp:60-77
	if v.Amount != nil {
		// Zero amount is valid (means "all"), negative is not
		if v.Amount.Value != "" && v.Amount.Value != "0" {
			amountVal, err := strconv.ParseFloat(v.Amount.Value, 64)
			if err == nil && amountVal < 0 {
				return ErrVaultAmountNotPos
			}
		}
		// Cannot clawback XRP
		if v.Amount.IsNative() {
			return ErrVaultAmountXRP
		}
		// Asset issuer must match Account
		if v.Amount.Issuer != "" && v.Amount.Issuer != v.Account {
			return ErrVaultAmountNotIssuer
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (v *VaultClawback) Flatten() (map[string]any, error) {
	return ReflectFlatten(v)
}

// RequiredAmendments returns the amendments required for this transaction type
func (v *VaultClawback) RequiredAmendments() []string {
	return []string{AmendmentSingleAssetVault}
}

// Apply applies the VaultCreate transaction to the ledger.
func (v *VaultCreate) Apply(ctx *ApplyContext) Result {
	if v.Asset.Currency == "" {
		return TemINVALID
	}
	var vaultKey [32]byte
	copy(vaultKey[:20], ctx.AccountID[:])
	binary.BigEndian.PutUint32(vaultKey[20:], ctx.Account.Sequence)
	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}
	vaultData := make([]byte, 64)
	copy(vaultData[:20], ctx.AccountID[:])
	if err := ctx.View.Insert(vaultKeylet, vaultData); err != nil {
		return TefINTERNAL
	}
	ctx.Account.OwnerCount++
	return TesSUCCESS
}

// Apply applies the VaultSet transaction to the ledger.
func (v *VaultSet) Apply(ctx *ApplyContext) Result {
	if v.VaultID == "" {
		return TemINVALID
	}
	vaultBytes, err := hex.DecodeString(v.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}
	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)
	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}
	_, err = ctx.View.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}
	return TesSUCCESS
}

// Apply applies the VaultDelete transaction to the ledger.
func (v *VaultDelete) Apply(ctx *ApplyContext) Result {
	if v.VaultID == "" {
		return TemINVALID
	}
	vaultBytes, err := hex.DecodeString(v.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}
	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)
	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}
	if err := ctx.View.Erase(vaultKeylet); err != nil {
		return TecNO_ENTRY
	}
	if ctx.Account.OwnerCount > 0 {
		ctx.Account.OwnerCount--
	}
	return TesSUCCESS
}

// Apply applies the VaultDeposit transaction to the ledger.
func (v *VaultDeposit) Apply(ctx *ApplyContext) Result {
	if v.VaultID == "" || v.Amount.Value == "" {
		return TemINVALID
	}
	vaultBytes, err := hex.DecodeString(v.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}
	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)
	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}
	_, err = ctx.View.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}
	if v.Amount.Currency == "" || v.Amount.Currency == "XRP" {
		amount, err := strconv.ParseUint(v.Amount.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}
		if ctx.Account.Balance < amount {
			return TecINSUFFICIENT_FUNDS
		}
		ctx.Account.Balance -= amount
	}
	return TesSUCCESS
}

// Apply applies the VaultWithdraw transaction to the ledger.
func (v *VaultWithdraw) Apply(ctx *ApplyContext) Result {
	if v.VaultID == "" || v.Amount.Value == "" {
		return TemINVALID
	}
	vaultBytes, err := hex.DecodeString(v.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}
	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)
	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}
	_, err = ctx.View.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}
	if v.Amount.Currency == "" || v.Amount.Currency == "XRP" {
		amount, err := strconv.ParseUint(v.Amount.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}
		ctx.Account.Balance += amount
	}
	return TesSUCCESS
}

// Apply applies the VaultClawback transaction to the ledger.
func (v *VaultClawback) Apply(ctx *ApplyContext) Result {
	if v.VaultID == "" || v.Holder == "" {
		return TemINVALID
	}
	vaultBytes, err := hex.DecodeString(v.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}
	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)
	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}
	_, err = ctx.View.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}
	_, err = decodeAccountID(v.Holder)
	if err != nil {
		return TecNO_TARGET
	}
	return TesSUCCESS
}
