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

// LedgerStateFix is a system transaction to fix ledger state issues.
type LedgerStateFix struct {
	BaseTx

	// LedgerFixType identifies the type of fix (required)
	LedgerFixType uint8 `json:"LedgerFixType"`

	// Owner is the owner account (optional)
	Owner string `json:"Owner,omitempty"`
}

// NewLedgerStateFix creates a new LedgerStateFix transaction
func NewLedgerStateFix(account string, fixType uint8) *LedgerStateFix {
	return &LedgerStateFix{
		BaseTx:        *NewBaseTx(TypeLedgerStateFix, account),
		LedgerFixType: fixType,
	}
}

// TxType returns the transaction type
func (l *LedgerStateFix) TxType() Type {
	return TypeLedgerStateFix
}

// Validate validates the LedgerStateFix transaction
func (l *LedgerStateFix) Validate() error {
	return l.BaseTx.Validate()
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
func (b *Batch) Validate() error {
	if err := b.BaseTx.Validate(); err != nil {
		return err
	}

	if len(b.RawTransactions) == 0 {
		return errors.New("RawTransactions is required")
	}

	// Max 8 transactions per batch
	if len(b.RawTransactions) > 8 {
		return errors.New("cannot have more than 8 transactions in a batch")
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
