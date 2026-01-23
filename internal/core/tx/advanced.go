package tx

import "errors"

// DelegateSet sets up delegation for an account.
type DelegateSet struct {
	BaseTx

	// Authorize is the account to delegate to (optional)
	Authorize string `json:"Authorize,omitempty"`

	// Permissions defines what the delegate can do (optional)
	Permissions []Permission `json:"Permissions,omitempty"`
}

// Permission defines a permission grant
type Permission struct {
	Permission PermissionData `json:"Permission"`
}

// PermissionData contains permission details
type PermissionData struct {
	PermissionType   string `json:"PermissionType"`
	PermissionValue  string `json:"PermissionValue,omitempty"`
	PermissionedFlag uint32 `json:"PermissionedFlag,omitempty"`
}

// NewDelegateSet creates a new DelegateSet transaction
func NewDelegateSet(account string) *DelegateSet {
	return &DelegateSet{
		BaseTx: *NewBaseTx(TypeDelegateSet, account),
	}
}

// TxType returns the transaction type
func (d *DelegateSet) TxType() Type {
	return TypeDelegateSet
}

// Validate validates the DelegateSet transaction
func (d *DelegateSet) Validate() error {
	return d.BaseTx.Validate()
}

// Flatten returns a flat map of all transaction fields
func (d *DelegateSet) Flatten() (map[string]any, error) {
	m := d.Common.ToMap()

	if d.Authorize != "" {
		m["Authorize"] = d.Authorize
	}
	if len(d.Permissions) > 0 {
		m["Permissions"] = d.Permissions
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (d *DelegateSet) RequiredAmendments() []string {
	return []string{AmendmentPermissionDelegation}
}

// NFTokenModify modifies an existing NFToken.
type NFTokenModify struct {
	BaseTx

	// NFTokenID is the ID of the token to modify (required)
	NFTokenID string `json:"NFTokenID"`

	// Owner is the owner of the token (optional)
	Owner string `json:"Owner,omitempty"`

	// URI is the new URI for the token (optional)
	URI string `json:"URI,omitempty"`
}

// NewNFTokenModify creates a new NFTokenModify transaction
func NewNFTokenModify(account, nftokenID string) *NFTokenModify {
	return &NFTokenModify{
		BaseTx:    *NewBaseTx(TypeNFTokenModify, account),
		NFTokenID: nftokenID,
	}
}

// TxType returns the transaction type
func (n *NFTokenModify) TxType() Type {
	return TypeNFTokenModify
}

// Validate validates the NFTokenModify transaction
// Reference: rippled NFTokenModify.cpp preflight
func (n *NFTokenModify) Validate() error {
	if err := n.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (no flags are valid for NFTokenModify)
	// Reference: rippled NFTokenModify.cpp:38 - if (ctx.tx.getFlags() & tfUniversalMask)
	if n.GetFlags() != 0 {
		return errors.New("temINVALID_FLAG: NFTokenModify does not accept any flags")
	}

	if n.NFTokenID == "" {
		return errors.New("temMALFORMED: NFTokenID is required")
	}

	// Owner cannot be the same as Account
	// Reference: rippled NFTokenModify.cpp:41 - if (auto owner = ctx.tx[~sfOwner]; owner == ctx.tx[sfAccount])
	if n.Owner != "" && n.Owner == n.Account {
		return errors.New("temMALFORMED: Owner cannot be the same as Account")
	}

	// URI validation: if present, must not be empty and not exceed maxTokenURILength
	// Reference: rippled NFTokenModify.cpp:44-47
	if n.URI != "" {
		// URI in transactions is hex-encoded, so actual byte length is len/2
		uriBytes := len(n.URI) / 2
		if uriBytes == 0 {
			return errors.New("temMALFORMED: URI cannot be empty")
		}
		if uriBytes > maxTokenURILength {
			return errors.New("temMALFORMED: URI too long")
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (n *NFTokenModify) Flatten() (map[string]any, error) {
	m := n.Common.ToMap()

	m["NFTokenID"] = n.NFTokenID

	if n.Owner != "" {
		m["Owner"] = n.Owner
	}
	if n.URI != "" {
		m["URI"] = n.URI
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (n *NFTokenModify) RequiredAmendments() []string {
	return []string{AmendmentDynamicNFT}
}

// LedgerStateFix fix types
// Reference: rippled LedgerStateFix.h FixType enum
const (
	// LedgerFixTypeNFTokenPageLink repairs NFToken directory page links
	LedgerFixTypeNFTokenPageLink uint8 = 1
)

// LedgerStateFix errors
var (
	ErrLedgerFixInvalidType  = errors.New("tefINVALID_LEDGER_FIX_TYPE: invalid LedgerFixType")
	ErrLedgerFixOwnerRequired = errors.New("temINVALID: Owner is required for nfTokenPageLink fix")
)

// LedgerStateFix is a system transaction to fix ledger state issues.
// Reference: rippled LedgerStateFix.cpp
type LedgerStateFix struct {
	BaseTx

	// LedgerFixType identifies the type of fix (required)
	LedgerFixType uint8 `json:"LedgerFixType"`

	// Owner is the owner account (required for nfTokenPageLink fix)
	Owner string `json:"Owner,omitempty"`
}

// NewLedgerStateFix creates a new LedgerStateFix transaction
func NewLedgerStateFix(account string, fixType uint8) *LedgerStateFix {
	return &LedgerStateFix{
		BaseTx:        *NewBaseTx(TypeLedgerStateFix, account),
		LedgerFixType: fixType,
	}
}

// NewNFTokenPageLinkFix creates a LedgerStateFix for NFToken page link repair
func NewNFTokenPageLinkFix(account, owner string) *LedgerStateFix {
	return &LedgerStateFix{
		BaseTx:        *NewBaseTx(TypeLedgerStateFix, account),
		LedgerFixType: LedgerFixTypeNFTokenPageLink,
		Owner:         owner,
	}
}

// TxType returns the transaction type
func (l *LedgerStateFix) TxType() Type {
	return TypeLedgerStateFix
}

// Validate validates the LedgerStateFix transaction
// Reference: rippled LedgerStateFix.cpp preflight()
func (l *LedgerStateFix) Validate() error {
	if err := l.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (universal mask)
	// Reference: rippled LedgerStateFix.cpp:36-37
	if l.Common.Flags != nil && *l.Common.Flags&tfUniversal != 0 {
		return ErrInvalidFlags
	}

	// Validate LedgerFixType and required fields based on type
	// Reference: rippled LedgerStateFix.cpp:42-51
	switch l.LedgerFixType {
	case LedgerFixTypeNFTokenPageLink:
		// Owner is required for nfTokenPageLink fix
		// Reference: rippled LedgerStateFix.cpp:45-46
		if l.Owner == "" {
			return ErrLedgerFixOwnerRequired
		}
	default:
		// Invalid fix type
		// Reference: rippled LedgerStateFix.cpp:49-50
		return ErrLedgerFixInvalidType
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (l *LedgerStateFix) Flatten() (map[string]any, error) {
	m := l.Common.ToMap()

	m["LedgerFixType"] = l.LedgerFixType

	if l.Owner != "" {
		m["Owner"] = l.Owner
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (l *LedgerStateFix) RequiredAmendments() []string {
	return []string{AmendmentFixNFTokenPageLinks}
}

// Batch is a transaction that contains multiple inner transactions.
type Batch struct {
	BaseTx

	// RawTransactions contains the raw transaction blobs (required)
	RawTransactions []RawTransaction `json:"RawTransactions"`

	// BatchSigners are the batch-level signers (optional)
	BatchSigners []BatchSigner `json:"BatchSigners,omitempty"`
}

// RawTransaction contains a raw transaction blob
type RawTransaction struct {
	RawTransaction RawTransactionData `json:"RawTransaction"`
}

// RawTransactionData contains the transaction blob
type RawTransactionData struct {
	RawTxBlob string `json:"RawTxBlob"`
}

// BatchSigner is a signer for batch transactions
type BatchSigner struct {
	BatchSigner BatchSignerData `json:"BatchSigner"`
}

// BatchSignerData contains batch signer fields
type BatchSignerData struct {
	Account         string `json:"Account"`
	SigningPubKey   string `json:"SigningPubKey"`
	BatchTxnSignature string `json:"BatchTxnSignature"`
}

// Batch flags
const (
	// tfAllOrNothing fails the batch if any transaction fails
	BatchFlagAllOrNothing uint32 = 0x00000001
	// tfOnlyOne succeeds if exactly one transaction succeeds
	BatchFlagOnlyOne uint32 = 0x00000002
	// tfUntilFailure processes until the first failure
	BatchFlagUntilFailure uint32 = 0x00000004
	// tfIndependent processes all transactions independently
	BatchFlagIndependent uint32 = 0x00000008

	// tfBatchMask is the mask for invalid batch flags
	tfBatchMask uint32 = ^(BatchFlagAllOrNothing | BatchFlagOnlyOne | BatchFlagUntilFailure | BatchFlagIndependent)

	// MaxBatchTransactions is the maximum number of inner transactions
	MaxBatchTransactions = 8
)

// Batch errors
var (
	ErrBatchTooFewTxns        = errors.New("temARRAY_EMPTY: batch must have at least 2 transactions")
	ErrBatchTooManyTxns       = errors.New("temARRAY_TOO_LARGE: batch exceeds 8 transactions")
	ErrBatchInvalidFlags      = errors.New("temINVALID_FLAG: invalid batch flags")
	ErrBatchMustHaveOneFlag   = errors.New("temINVALID_FLAG: exactly one batch mode flag required")
	ErrBatchTooManySigners    = errors.New("temARRAY_TOO_LARGE: batch signers exceeds 8 entries")
	ErrBatchDuplicateSigner   = errors.New("temREDUNDANT: duplicate batch signer")
	ErrBatchSignerIsOuter     = errors.New("temBAD_SIGNER: batch signer cannot be outer account")
	ErrBatchEmptyRawTxBlob    = errors.New("temMALFORMED: RawTxBlob cannot be empty")
)

// NewBatch creates a new Batch transaction
func NewBatch(account string) *Batch {
	return &Batch{
		BaseTx: *NewBaseTx(TypeBatch, account),
	}
}

// TxType returns the transaction type
func (b *Batch) TxType() Type {
	return TypeBatch
}

// Validate validates the Batch transaction
// Reference: rippled Batch.cpp preflight()
func (b *Batch) Validate() error {
	if err := b.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags
	// Reference: rippled Batch.cpp:213-217
	if b.Common.Flags != nil && *b.Common.Flags&tfBatchMask != 0 {
		return ErrBatchInvalidFlags
	}

	// Must have exactly one of the mutually exclusive flags
	// Reference: rippled Batch.cpp:220-227
	flags := uint32(0)
	if b.Common.Flags != nil {
		flags = *b.Common.Flags
	}
	modeFlags := flags & (BatchFlagAllOrNothing | BatchFlagOnlyOne | BatchFlagUntilFailure | BatchFlagIndependent)
	popCount := 0
	for modeFlags != 0 {
		popCount += int(modeFlags & 1)
		modeFlags >>= 1
	}
	if popCount != 1 {
		return ErrBatchMustHaveOneFlag
	}

	// Must have at least 2 transactions
	// Reference: rippled Batch.cpp:229-234
	if len(b.RawTransactions) <= 1 {
		return ErrBatchTooFewTxns
	}

	// Max 8 transactions per batch
	// Reference: rippled Batch.cpp:237-241
	if len(b.RawTransactions) > MaxBatchTransactions {
		return ErrBatchTooManyTxns
	}

	// Validate each raw transaction has a non-empty blob
	for _, rt := range b.RawTransactions {
		if rt.RawTransaction.RawTxBlob == "" {
			return ErrBatchEmptyRawTxBlob
		}
	}

	// Validate BatchSigners if present
	// Reference: rippled Batch.cpp:394-398
	if len(b.BatchSigners) > MaxBatchTransactions {
		return ErrBatchTooManySigners
	}

	// Check for duplicate signers and signer being outer account
	// Reference: rippled Batch.cpp:406-432
	seenSigners := make(map[string]bool)
	for _, signer := range b.BatchSigners {
		acct := signer.BatchSigner.Account
		if acct == b.Account {
			return ErrBatchSignerIsOuter
		}
		if seenSigners[acct] {
			return ErrBatchDuplicateSigner
		}
		seenSigners[acct] = true
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (b *Batch) Flatten() (map[string]any, error) {
	m := b.Common.ToMap()

	m["RawTransactions"] = b.RawTransactions

	if len(b.BatchSigners) > 0 {
		m["BatchSigners"] = b.BatchSigners
	}

	return m, nil
}

// AddRawTransaction adds a raw transaction to the batch
func (b *Batch) AddRawTransaction(blob string) {
	b.RawTransactions = append(b.RawTransactions, RawTransaction{
		RawTransaction: RawTransactionData{
			RawTxBlob: blob,
		},
	})
}

// RequiredAmendments returns the amendments required for this transaction type
func (b *Batch) RequiredAmendments() []string {
	return []string{AmendmentBatch}
}
