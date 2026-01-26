package batch

import (
	"encoding/hex"
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
)

func init() {
	tx.Register(tx.TypeBatch, func() tx.Transaction {
		return &Batch{BaseTx: *tx.NewBaseTx(tx.TypeBatch, "")}
	})
}

// Batch is a transaction that contains multiple inner transactions.
type Batch struct {
	tx.BaseTx

	// RawTransactions contains the raw transaction blobs (required)
	RawTransactions []RawTransaction `json:"RawTransactions" xrpl:"RawTransactions"`

	// BatchSigners are the batch-level signers (optional)
	BatchSigners []BatchSigner `json:"BatchSigners,omitempty" xrpl:"BatchSigners,omitempty"`
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
	Account           string `json:"Account"`
	SigningPubKey     string `json:"SigningPubKey"`
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
	ErrBatchTooFewTxns      = errors.New("temARRAY_EMPTY: batch must have at least 2 transactions")
	ErrBatchTooManyTxns     = errors.New("temARRAY_TOO_LARGE: batch exceeds 8 transactions")
	ErrBatchInvalidFlags    = errors.New("temINVALID_FLAG: invalid batch flags")
	ErrBatchMustHaveOneFlag = errors.New("temINVALID_FLAG: exactly one batch mode flag required")
	ErrBatchTooManySigners  = errors.New("temARRAY_TOO_LARGE: batch signers exceeds 8 entries")
	ErrBatchDuplicateSigner = errors.New("temREDUNDANT: duplicate batch signer")
	ErrBatchSignerIsOuter   = errors.New("temBAD_SIGNER: batch signer cannot be outer account")
	ErrBatchEmptyRawTxBlob  = errors.New("temMALFORMED: RawTxBlob cannot be empty")
)

// NewBatch creates a new Batch transaction
func NewBatch(account string) *Batch {
	return &Batch{
		BaseTx: *tx.NewBaseTx(tx.TypeBatch, account),
	}
}

// TxType returns the transaction type
func (b *Batch) TxType() tx.Type {
	return tx.TypeBatch
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
	return tx.ReflectFlatten(b)
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
	return []string{amendment.AmendmentBatch}
}

// Apply applies the Batch transaction to the ledger.
func (b *Batch) Apply(ctx *tx.ApplyContext) tx.Result {
	if len(b.RawTransactions) == 0 {
		return tx.TemINVALID
	}
	flags := b.GetFlags()
	for _, rawTx := range b.RawTransactions {
		_, err := hex.DecodeString(rawTx.RawTransaction.RawTxBlob)
		if err != nil {
			if flags&BatchFlagAllOrNothing != 0 {
				return tx.TefINTERNAL
			}
			continue
		}
		if flags&BatchFlagUntilFailure != 0 {
		}
		if flags&BatchFlagOnlyOne != 0 {
			break
		}
	}
	return tx.TesSUCCESS
}
