package tx

import (
	"errors"
	"strconv"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

func init() {
	Register(TypeXChainCreateBridge, func() Transaction {
		return &XChainCreateBridge{BaseTx: *NewBaseTx(TypeXChainCreateBridge, "")}
	})
	Register(TypeXChainModifyBridge, func() Transaction {
		return &XChainModifyBridge{BaseTx: *NewBaseTx(TypeXChainModifyBridge, "")}
	})
	Register(TypeXChainCreateClaimID, func() Transaction {
		return &XChainCreateClaimID{BaseTx: *NewBaseTx(TypeXChainCreateClaimID, "")}
	})
	Register(TypeXChainCommit, func() Transaction {
		return &XChainCommit{BaseTx: *NewBaseTx(TypeXChainCommit, "")}
	})
	Register(TypeXChainClaim, func() Transaction {
		return &XChainClaim{BaseTx: *NewBaseTx(TypeXChainClaim, "")}
	})
	Register(TypeXChainAccountCreateCommit, func() Transaction {
		return &XChainAccountCreateCommit{BaseTx: *NewBaseTx(TypeXChainAccountCreateCommit, "")}
	})
	Register(TypeXChainAddClaimAttestation, func() Transaction {
		return &XChainAddClaimAttestation{BaseTx: *NewBaseTx(TypeXChainAddClaimAttestation, "")}
	})
	Register(TypeXChainAddAccountCreateAttest, func() Transaction {
		return &XChainAddAccountCreateAttestation{BaseTx: *NewBaseTx(TypeXChainAddAccountCreateAttest, "")}
	})
}

// XChainBridge identifies a cross-chain bridge
type XChainBridge struct {
	LockingChainDoor  string `json:"LockingChainDoor"`
	LockingChainIssue Asset  `json:"LockingChainIssue"`
	IssuingChainDoor  string `json:"IssuingChainDoor"`
	IssuingChainIssue Asset  `json:"IssuingChainIssue"`
}

// XChainCreateBridge creates a new cross-chain bridge.
type XChainCreateBridge struct {
	BaseTx

	// XChainBridge identifies the bridge (required)
	XChainBridge XChainBridge `json:"XChainBridge" xrpl:"XChainBridge"`

	// SignatureReward is the reward for witnesses (required)
	SignatureReward Amount `json:"SignatureReward" xrpl:"SignatureReward,amount"`

	// MinAccountCreateAmount is the min amount for account creation (optional)
	MinAccountCreateAmount *Amount `json:"MinAccountCreateAmount,omitempty" xrpl:"MinAccountCreateAmount,omitempty,amount"`
}

// NewXChainCreateBridge creates a new XChainCreateBridge transaction
func NewXChainCreateBridge(account string, bridge XChainBridge, signatureReward Amount) *XChainCreateBridge {
	return &XChainCreateBridge{
		BaseTx:          *NewBaseTx(TypeXChainCreateBridge, account),
		XChainBridge:    bridge,
		SignatureReward: signatureReward,
	}
}

// TxType returns the transaction type
func (x *XChainCreateBridge) TxType() Type {
	return TypeXChainCreateBridge
}

// Validate validates the XChainCreateBridge transaction
func (x *XChainCreateBridge) Validate() error {
	if err := x.BaseTx.Validate(); err != nil {
		return err
	}

	if x.SignatureReward.Value == "" {
		return errors.New("SignatureReward is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (x *XChainCreateBridge) Flatten() (map[string]any, error) {
	return ReflectFlatten(x)
}

// RequiredAmendments returns the amendments required for this transaction type
func (x *XChainCreateBridge) RequiredAmendments() []string {
	return []string{AmendmentXChainBridge}
}

// XChainModifyBridge modifies an existing cross-chain bridge.
type XChainModifyBridge struct {
	BaseTx

	// XChainBridge identifies the bridge (required)
	XChainBridge XChainBridge `json:"XChainBridge" xrpl:"XChainBridge"`

	// SignatureReward is the new reward for witnesses (optional)
	SignatureReward *Amount `json:"SignatureReward,omitempty" xrpl:"SignatureReward,omitempty,amount"`

	// MinAccountCreateAmount is the new min amount (optional)
	MinAccountCreateAmount *Amount `json:"MinAccountCreateAmount,omitempty" xrpl:"MinAccountCreateAmount,omitempty,amount"`
}

// XChainModifyBridge flags
const (
	// tfClearAccountCreateAmount clears the min account create amount
	XChainModifyBridgeFlagClearAccountCreateAmount uint32 = 0x00010000
)

// NewXChainModifyBridge creates a new XChainModifyBridge transaction
func NewXChainModifyBridge(account string, bridge XChainBridge) *XChainModifyBridge {
	return &XChainModifyBridge{
		BaseTx:       *NewBaseTx(TypeXChainModifyBridge, account),
		XChainBridge: bridge,
	}
}

// TxType returns the transaction type
func (x *XChainModifyBridge) TxType() Type {
	return TypeXChainModifyBridge
}

// Validate validates the XChainModifyBridge transaction
func (x *XChainModifyBridge) Validate() error {
	return x.BaseTx.Validate()
}

// Flatten returns a flat map of all transaction fields
func (x *XChainModifyBridge) Flatten() (map[string]any, error) {
	return ReflectFlatten(x)
}

// RequiredAmendments returns the amendments required for this transaction type
func (x *XChainModifyBridge) RequiredAmendments() []string {
	return []string{AmendmentXChainBridge}
}

// XChainCreateClaimID creates a claim ID for cross-chain transfers.
type XChainCreateClaimID struct {
	BaseTx

	// XChainBridge identifies the bridge (required)
	XChainBridge XChainBridge `json:"XChainBridge" xrpl:"XChainBridge"`

	// SignatureReward is the reward for witnesses (required)
	SignatureReward Amount `json:"SignatureReward" xrpl:"SignatureReward,amount"`

	// OtherChainSource is the source account on the other chain (required)
	OtherChainSource string `json:"OtherChainSource" xrpl:"OtherChainSource"`
}

// NewXChainCreateClaimID creates a new XChainCreateClaimID transaction
func NewXChainCreateClaimID(account string, bridge XChainBridge, signatureReward Amount, otherChainSource string) *XChainCreateClaimID {
	return &XChainCreateClaimID{
		BaseTx:           *NewBaseTx(TypeXChainCreateClaimID, account),
		XChainBridge:     bridge,
		SignatureReward:  signatureReward,
		OtherChainSource: otherChainSource,
	}
}

// TxType returns the transaction type
func (x *XChainCreateClaimID) TxType() Type {
	return TypeXChainCreateClaimID
}

// Validate validates the XChainCreateClaimID transaction
func (x *XChainCreateClaimID) Validate() error {
	if err := x.BaseTx.Validate(); err != nil {
		return err
	}

	if x.OtherChainSource == "" {
		return errors.New("OtherChainSource is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (x *XChainCreateClaimID) Flatten() (map[string]any, error) {
	return ReflectFlatten(x)
}

// RequiredAmendments returns the amendments required for this transaction type
func (x *XChainCreateClaimID) RequiredAmendments() []string {
	return []string{AmendmentXChainBridge}
}

// XChainCommit commits assets to a cross-chain transfer.
type XChainCommit struct {
	BaseTx

	// XChainBridge identifies the bridge (required)
	XChainBridge XChainBridge `json:"XChainBridge" xrpl:"XChainBridge"`

	// XChainClaimID is the claim ID (required)
	XChainClaimID uint64 `json:"XChainClaimID" xrpl:"XChainClaimID"`

	// Amount is the amount to transfer (required)
	Amount Amount `json:"Amount" xrpl:"Amount,amount"`

	// OtherChainDestination is the destination on the other chain (optional)
	OtherChainDestination string `json:"OtherChainDestination,omitempty" xrpl:"OtherChainDestination,omitempty"`
}

// NewXChainCommit creates a new XChainCommit transaction
func NewXChainCommit(account string, bridge XChainBridge, claimID uint64, amount Amount) *XChainCommit {
	return &XChainCommit{
		BaseTx:        *NewBaseTx(TypeXChainCommit, account),
		XChainBridge:  bridge,
		XChainClaimID: claimID,
		Amount:        amount,
	}
}

// TxType returns the transaction type
func (x *XChainCommit) TxType() Type {
	return TypeXChainCommit
}

// Validate validates the XChainCommit transaction
func (x *XChainCommit) Validate() error {
	if err := x.BaseTx.Validate(); err != nil {
		return err
	}

	if x.Amount.Value == "" {
		return errors.New("Amount is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (x *XChainCommit) Flatten() (map[string]any, error) {
	return ReflectFlatten(x)
}

// RequiredAmendments returns the amendments required for this transaction type
func (x *XChainCommit) RequiredAmendments() []string {
	return []string{AmendmentXChainBridge}
}

// XChainClaim claims assets from a cross-chain transfer.
type XChainClaim struct {
	BaseTx

	// XChainBridge identifies the bridge (required)
	XChainBridge XChainBridge `json:"XChainBridge" xrpl:"XChainBridge"`

	// XChainClaimID is the claim ID (required)
	XChainClaimID uint64 `json:"XChainClaimID" xrpl:"XChainClaimID"`

	// Destination is the account to receive the assets (required)
	Destination string `json:"Destination" xrpl:"Destination"`

	// DestinationTag is an arbitrary tag (optional)
	DestinationTag *uint32 `json:"DestinationTag,omitempty" xrpl:"DestinationTag,omitempty"`

	// Amount is the amount to claim (required)
	Amount Amount `json:"Amount" xrpl:"Amount,amount"`
}

// NewXChainClaim creates a new XChainClaim transaction
func NewXChainClaim(account string, bridge XChainBridge, claimID uint64, destination string, amount Amount) *XChainClaim {
	return &XChainClaim{
		BaseTx:        *NewBaseTx(TypeXChainClaim, account),
		XChainBridge:  bridge,
		XChainClaimID: claimID,
		Destination:   destination,
		Amount:        amount,
	}
}

// TxType returns the transaction type
func (x *XChainClaim) TxType() Type {
	return TypeXChainClaim
}

// Validate validates the XChainClaim transaction
func (x *XChainClaim) Validate() error {
	if err := x.BaseTx.Validate(); err != nil {
		return err
	}

	if x.Destination == "" {
		return errors.New("Destination is required")
	}

	if x.Amount.Value == "" {
		return errors.New("Amount is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (x *XChainClaim) Flatten() (map[string]any, error) {
	return ReflectFlatten(x)
}

// RequiredAmendments returns the amendments required for this transaction type
func (x *XChainClaim) RequiredAmendments() []string {
	return []string{AmendmentXChainBridge}
}

// XChainAccountCreateCommit commits to create an account on the other chain.
type XChainAccountCreateCommit struct {
	BaseTx

	// XChainBridge identifies the bridge (required)
	XChainBridge XChainBridge `json:"XChainBridge" xrpl:"XChainBridge"`

	// Destination is the account to create (required)
	Destination string `json:"Destination" xrpl:"Destination"`

	// Amount is the amount to send (required)
	Amount Amount `json:"Amount" xrpl:"Amount,amount"`

	// SignatureReward is the reward for witnesses (required)
	SignatureReward Amount `json:"SignatureReward" xrpl:"SignatureReward,amount"`
}

// NewXChainAccountCreateCommit creates a new XChainAccountCreateCommit transaction
func NewXChainAccountCreateCommit(account string, bridge XChainBridge, destination string, amount, signatureReward Amount) *XChainAccountCreateCommit {
	return &XChainAccountCreateCommit{
		BaseTx:          *NewBaseTx(TypeXChainAccountCreateCommit, account),
		XChainBridge:    bridge,
		Destination:     destination,
		Amount:          amount,
		SignatureReward: signatureReward,
	}
}

// TxType returns the transaction type
func (x *XChainAccountCreateCommit) TxType() Type {
	return TypeXChainAccountCreateCommit
}

// Validate validates the XChainAccountCreateCommit transaction
func (x *XChainAccountCreateCommit) Validate() error {
	if err := x.BaseTx.Validate(); err != nil {
		return err
	}

	if x.Destination == "" {
		return errors.New("Destination is required")
	}

	if x.Amount.Value == "" {
		return errors.New("Amount is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (x *XChainAccountCreateCommit) Flatten() (map[string]any, error) {
	return ReflectFlatten(x)
}

// RequiredAmendments returns the amendments required for this transaction type
func (x *XChainAccountCreateCommit) RequiredAmendments() []string {
	return []string{AmendmentXChainBridge}
}

// XChainAddClaimAttestation adds a witness attestation for a claim.
type XChainAddClaimAttestation struct {
	BaseTx

	// XChainBridge identifies the bridge (required)
	XChainBridge XChainBridge `json:"XChainBridge" xrpl:"XChainBridge"`

	// XChainClaimID is the claim ID (required)
	XChainClaimID uint64 `json:"XChainClaimID" xrpl:"XChainClaimID"`

	// OtherChainSource is the source on the other chain (required)
	OtherChainSource string `json:"OtherChainSource" xrpl:"OtherChainSource"`

	// Amount is the amount attested (required)
	Amount Amount `json:"Amount" xrpl:"Amount,amount"`

	// AttestationRewardAccount receives the reward (required)
	AttestationRewardAccount string `json:"AttestationRewardAccount" xrpl:"AttestationRewardAccount"`

	// AttestationSignerAccount is the signer account (required)
	AttestationSignerAccount string `json:"AttestationSignerAccount" xrpl:"AttestationSignerAccount"`

	// Destination is the destination account (optional)
	Destination string `json:"Destination,omitempty" xrpl:"Destination,omitempty"`

	// PublicKey is the signer's public key (required)
	PublicKey string `json:"PublicKey" xrpl:"PublicKey"`

	// Signature is the attestation signature (required)
	Signature string `json:"Signature" xrpl:"Signature"`

	// WasLockingChainSend indicates if this was a locking chain send (required)
	WasLockingChainSend bool `json:"WasLockingChainSend" xrpl:"WasLockingChainSend,boolint"`
}

// NewXChainAddClaimAttestation creates a new XChainAddClaimAttestation transaction
func NewXChainAddClaimAttestation(account string, bridge XChainBridge, claimID uint64) *XChainAddClaimAttestation {
	return &XChainAddClaimAttestation{
		BaseTx:        *NewBaseTx(TypeXChainAddClaimAttestation, account),
		XChainBridge:  bridge,
		XChainClaimID: claimID,
	}
}

// TxType returns the transaction type
func (x *XChainAddClaimAttestation) TxType() Type {
	return TypeXChainAddClaimAttestation
}

// Validate validates the XChainAddClaimAttestation transaction
func (x *XChainAddClaimAttestation) Validate() error {
	if err := x.BaseTx.Validate(); err != nil {
		return err
	}

	if x.OtherChainSource == "" {
		return errors.New("OtherChainSource is required")
	}

	if x.AttestationRewardAccount == "" {
		return errors.New("AttestationRewardAccount is required")
	}

	if x.AttestationSignerAccount == "" {
		return errors.New("AttestationSignerAccount is required")
	}

	if x.PublicKey == "" {
		return errors.New("PublicKey is required")
	}

	if x.Signature == "" {
		return errors.New("Signature is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (x *XChainAddClaimAttestation) Flatten() (map[string]any, error) {
	return ReflectFlatten(x)
}

// RequiredAmendments returns the amendments required for this transaction type
func (x *XChainAddClaimAttestation) RequiredAmendments() []string {
	return []string{AmendmentXChainBridge}
}

// XChainAddAccountCreateAttestation adds a witness attestation for account creation.
type XChainAddAccountCreateAttestation struct {
	BaseTx

	// XChainBridge identifies the bridge (required)
	XChainBridge XChainBridge `json:"XChainBridge" xrpl:"XChainBridge"`

	// OtherChainSource is the source on the other chain (required)
	OtherChainSource string `json:"OtherChainSource" xrpl:"OtherChainSource"`

	// Destination is the destination account (required)
	Destination string `json:"Destination" xrpl:"Destination"`

	// Amount is the amount attested (required)
	Amount Amount `json:"Amount" xrpl:"Amount,amount"`

	// SignatureReward is the signature reward (required)
	SignatureReward Amount `json:"SignatureReward" xrpl:"SignatureReward,amount"`

	// AttestationRewardAccount receives the reward (required)
	AttestationRewardAccount string `json:"AttestationRewardAccount" xrpl:"AttestationRewardAccount"`

	// AttestationSignerAccount is the signer account (required)
	AttestationSignerAccount string `json:"AttestationSignerAccount" xrpl:"AttestationSignerAccount"`

	// PublicKey is the signer's public key (required)
	PublicKey string `json:"PublicKey" xrpl:"PublicKey"`

	// Signature is the attestation signature (required)
	Signature string `json:"Signature" xrpl:"Signature"`

	// WasLockingChainSend indicates if this was a locking chain send (required)
	WasLockingChainSend bool `json:"WasLockingChainSend" xrpl:"WasLockingChainSend,boolint"`

	// XChainAccountCreateCount is the create count (required)
	XChainAccountCreateCount uint64 `json:"XChainAccountCreateCount" xrpl:"XChainAccountCreateCount"`
}

// NewXChainAddAccountCreateAttestation creates a new XChainAddAccountCreateAttestation transaction
func NewXChainAddAccountCreateAttestation(account string, bridge XChainBridge) *XChainAddAccountCreateAttestation {
	return &XChainAddAccountCreateAttestation{
		BaseTx:       *NewBaseTx(TypeXChainAddAccountCreateAttest, account),
		XChainBridge: bridge,
	}
}

// TxType returns the transaction type
func (x *XChainAddAccountCreateAttestation) TxType() Type {
	return TypeXChainAddAccountCreateAttest
}

// Validate validates the XChainAddAccountCreateAttestation transaction
func (x *XChainAddAccountCreateAttestation) Validate() error {
	if err := x.BaseTx.Validate(); err != nil {
		return err
	}

	if x.OtherChainSource == "" {
		return errors.New("OtherChainSource is required")
	}

	if x.Destination == "" {
		return errors.New("Destination is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (x *XChainAddAccountCreateAttestation) Flatten() (map[string]any, error) {
	return ReflectFlatten(x)
}

// RequiredAmendments returns the amendments required for this transaction type
func (x *XChainAddAccountCreateAttestation) RequiredAmendments() []string {
	return []string{AmendmentXChainBridge}
}

// Apply applies the XChainCreateBridge transaction to the ledger.
func (x *XChainCreateBridge) Apply(ctx *ApplyContext) Result {
	ctx.Account.OwnerCount++
	return TesSUCCESS
}

// Apply applies the XChainModifyBridge transaction to the ledger.
func (x *XChainModifyBridge) Apply(ctx *ApplyContext) Result {
	return TesSUCCESS
}

// Apply applies the XChainCreateClaimID transaction to the ledger.
func (x *XChainCreateClaimID) Apply(ctx *ApplyContext) Result {
	ctx.Account.OwnerCount++
	return TesSUCCESS
}

// Apply applies the XChainCommit transaction to the ledger.
func (x *XChainCommit) Apply(ctx *ApplyContext) Result {
	amount, err := strconv.ParseUint(x.Amount.Value, 10, 64)
	if err == nil && x.Amount.Currency == "" {
		if ctx.Account.Balance < amount {
			return TecUNFUNDED
		}
		ctx.Account.Balance -= amount
	}
	return TesSUCCESS
}

// Apply applies the XChainClaim transaction to the ledger.
func (x *XChainClaim) Apply(ctx *ApplyContext) Result {
	amount, err := strconv.ParseUint(x.Amount.Value, 10, 64)
	if err == nil && x.Amount.Currency == "" {
		destID, err := decodeAccountID(x.Destination)
		if err != nil {
			return TemINVALID
		}
		destKey := keylet.Account(destID)
		destData, err := ctx.View.Read(destKey)
		if err == nil {
			destAccount, err := parseAccountRoot(destData)
			if err == nil {
				destAccount.Balance += amount
				destUpdatedData, _ := serializeAccountRoot(destAccount)
				ctx.View.Update(destKey, destUpdatedData)
			}
		}
	}
	return TesSUCCESS
}

// Apply applies the XChainAccountCreateCommit transaction to the ledger.
func (x *XChainAccountCreateCommit) Apply(ctx *ApplyContext) Result {
	amount, err := strconv.ParseUint(x.Amount.Value, 10, 64)
	if err == nil && x.Amount.Currency == "" {
		if ctx.Account.Balance < amount {
			return TecUNFUNDED
		}
		ctx.Account.Balance -= amount
	}
	return TesSUCCESS
}

// Apply applies the XChainAddClaimAttestation transaction to the ledger.
func (x *XChainAddClaimAttestation) Apply(ctx *ApplyContext) Result {
	return TesSUCCESS
}

// Apply applies the XChainAddAccountCreateAttestation transaction to the ledger.
func (x *XChainAddAccountCreateAttestation) Apply(ctx *ApplyContext) Result {
	return TesSUCCESS
}
