package batch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Batch Validation Tests
// Based on rippled Batch.cpp
// =============================================================================

func TestBatchValidation(t *testing.T) {
	// Helper to create a valid batch with minimum requirements
	makeValidBatch := func() *Batch {
		b := NewBatch("rOuter")
		b.AddRawTransaction("AABBCCDD")
		b.AddRawTransaction("EEFF0011")
		flags := BatchFlagAllOrNothing
		b.Common.Flags = &flags
		return b
	}

	tests := []struct {
		name    string
		tx      *Batch
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{
			name:    "valid - basic batch with AllOrNothing",
			tx:      makeValidBatch(),
			wantErr: false,
		},
		{
			name: "valid - batch with OnlyOne flag",
			tx: func() *Batch {
				b := NewBatch("rOuter")
				b.AddRawTransaction("AABBCCDD")
				b.AddRawTransaction("EEFF0011")
				flags := BatchFlagOnlyOne
				b.Common.Flags = &flags
				return b
			}(),
			wantErr: false,
		},
		{
			name: "valid - batch with UntilFailure flag",
			tx: func() *Batch {
				b := NewBatch("rOuter")
				b.AddRawTransaction("AABBCCDD")
				b.AddRawTransaction("EEFF0011")
				flags := BatchFlagUntilFailure
				b.Common.Flags = &flags
				return b
			}(),
			wantErr: false,
		},
		{
			name: "valid - batch with Independent flag",
			tx: func() *Batch {
				b := NewBatch("rOuter")
				b.AddRawTransaction("AABBCCDD")
				b.AddRawTransaction("EEFF0011")
				flags := BatchFlagIndependent
				b.Common.Flags = &flags
				return b
			}(),
			wantErr: false,
		},
		{
			name: "valid - maximum 8 transactions",
			tx: func() *Batch {
				b := NewBatch("rOuter")
				for i := 0; i < 8; i++ {
					b.AddRawTransaction("AABBCCDD")
				}
				flags := BatchFlagAllOrNothing
				b.Common.Flags = &flags
				return b
			}(),
			wantErr: false,
		},
		{
			name: "valid - batch with signers",
			tx: func() *Batch {
				b := makeValidBatch()
				b.BatchSigners = []BatchSigner{
					{BatchSigner: BatchSignerData{Account: "rSigner1", SigningPubKey: "ABC", BatchTxnSignature: "DEF"}},
					{BatchSigner: BatchSignerData{Account: "rSigner2", SigningPubKey: "GHI", BatchTxnSignature: "JKL"}},
				}
				return b
			}(),
			wantErr: false,
		},

		// Invalid cases - transaction count
		{
			name: "invalid - no transactions (empty array)",
			tx: func() *Batch {
				b := NewBatch("rOuter")
				flags := BatchFlagAllOrNothing
				b.Common.Flags = &flags
				return b
			}(),
			wantErr: true,
			errMsg:  "at least 2",
		},
		{
			name: "invalid - only 1 transaction",
			tx: func() *Batch {
				b := NewBatch("rOuter")
				b.AddRawTransaction("AABBCCDD")
				flags := BatchFlagAllOrNothing
				b.Common.Flags = &flags
				return b
			}(),
			wantErr: true,
			errMsg:  "at least 2",
		},
		{
			name: "invalid - too many transactions (>8)",
			tx: func() *Batch {
				b := NewBatch("rOuter")
				for i := 0; i < 9; i++ {
					b.AddRawTransaction("AABBCCDD")
				}
				flags := BatchFlagAllOrNothing
				b.Common.Flags = &flags
				return b
			}(),
			wantErr: true,
			errMsg:  "exceeds 8",
		},

		// Invalid cases - flags
		{
			name: "invalid - no mode flag set",
			tx: func() *Batch {
				b := NewBatch("rOuter")
				b.AddRawTransaction("AABBCCDD")
				b.AddRawTransaction("EEFF0011")
				flags := uint32(0)
				b.Common.Flags = &flags
				return b
			}(),
			wantErr: true,
			errMsg:  "exactly one",
		},
		{
			name: "invalid - multiple mode flags set",
			tx: func() *Batch {
				b := NewBatch("rOuter")
				b.AddRawTransaction("AABBCCDD")
				b.AddRawTransaction("EEFF0011")
				flags := BatchFlagAllOrNothing | BatchFlagOnlyOne
				b.Common.Flags = &flags
				return b
			}(),
			wantErr: true,
			errMsg:  "exactly one",
		},
		{
			name: "invalid - all mode flags set",
			tx: func() *Batch {
				b := NewBatch("rOuter")
				b.AddRawTransaction("AABBCCDD")
				b.AddRawTransaction("EEFF0011")
				flags := BatchFlagAllOrNothing | BatchFlagOnlyOne | BatchFlagUntilFailure | BatchFlagIndependent
				b.Common.Flags = &flags
				return b
			}(),
			wantErr: true,
			errMsg:  "exactly one",
		},

		// Invalid cases - raw transaction blobs
		{
			name: "invalid - empty RawTxBlob",
			tx: func() *Batch {
				b := NewBatch("rOuter")
				b.RawTransactions = []RawTransaction{
					{RawTransaction: RawTransactionData{RawTxBlob: "AABBCCDD"}},
					{RawTransaction: RawTransactionData{RawTxBlob: ""}}, // empty
				}
				flags := BatchFlagAllOrNothing
				b.Common.Flags = &flags
				return b
			}(),
			wantErr: true,
			errMsg:  "RawTxBlob cannot be empty",
		},

		// Invalid cases - batch signers
		{
			name: "invalid - too many batch signers",
			tx: func() *Batch {
				b := makeValidBatch()
				for i := 0; i < 9; i++ {
					b.BatchSigners = append(b.BatchSigners, BatchSigner{
						BatchSigner: BatchSignerData{Account: "rSigner" + string(rune('0'+i))},
					})
				}
				return b
			}(),
			wantErr: true,
			errMsg:  "exceeds 8",
		},
		{
			name: "invalid - duplicate batch signer",
			tx: func() *Batch {
				b := makeValidBatch()
				b.BatchSigners = []BatchSigner{
					{BatchSigner: BatchSignerData{Account: "rSigner1"}},
					{BatchSigner: BatchSignerData{Account: "rSigner1"}}, // duplicate
				}
				return b
			}(),
			wantErr: true,
			errMsg:  "duplicate",
		},
		{
			name: "invalid - batch signer is outer account",
			tx: func() *Batch {
				b := makeValidBatch()
				b.BatchSigners = []BatchSigner{
					{BatchSigner: BatchSignerData{Account: "rOuter"}}, // same as outer
				}
				return b
			}(),
			wantErr: true,
			errMsg:  "outer account",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.tx.Common.Fee = "12"
			seq := uint32(1)
			tt.tx.Common.Sequence = &seq

			err := tt.tx.Validate()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// Flatten Tests
// =============================================================================

func TestBatchFlatten(t *testing.T) {
	t.Run("basic batch", func(t *testing.T) {
		b := NewBatch("rOuter")
		b.AddRawTransaction("AABBCCDD")
		b.AddRawTransaction("EEFF0011")

		flat, err := b.Flatten()
		require.NoError(t, err)

		assert.Equal(t, "rOuter", flat["Account"])
		assert.Equal(t, "Batch", flat["TransactionType"])

		rawTxns, ok := flat["RawTransactions"].([]RawTransaction)
		require.True(t, ok)
		assert.Len(t, rawTxns, 2)
	})

	t.Run("batch with signers", func(t *testing.T) {
		b := NewBatch("rOuter")
		b.AddRawTransaction("AABBCCDD")
		b.AddRawTransaction("EEFF0011")
		b.BatchSigners = []BatchSigner{
			{BatchSigner: BatchSignerData{Account: "rSigner1", SigningPubKey: "ABC", BatchTxnSignature: "DEF"}},
		}

		flat, err := b.Flatten()
		require.NoError(t, err)

		signers, ok := flat["BatchSigners"].([]BatchSigner)
		require.True(t, ok)
		assert.Len(t, signers, 1)
	})
}

// =============================================================================
// Constructor Tests
// =============================================================================

func TestBatchConstructors(t *testing.T) {
	t.Run("NewBatch", func(t *testing.T) {
		tx := NewBatch("rOuter")
		require.NotNil(t, tx)
		assert.Equal(t, "rOuter", tx.Account)
		assert.Equal(t, tx.TypeBatch, tx.TxType())
		assert.Empty(t, tx.RawTransactions)
		assert.Empty(t, tx.BatchSigners)
	})
}

// =============================================================================
// AddRawTransaction Test
// =============================================================================

func TestBatchAddRawTransaction(t *testing.T) {
	b := NewBatch("rOuter")

	b.AddRawTransaction("AABBCCDD")
	b.AddRawTransaction("EEFF0011")

	require.Len(t, b.RawTransactions, 2)
	assert.Equal(t, "AABBCCDD", b.RawTransactions[0].RawTransaction.RawTxBlob)
	assert.Equal(t, "EEFF0011", b.RawTransactions[1].RawTransaction.RawTxBlob)
}

// =============================================================================
// Amendment Tests
// =============================================================================

func TestBatchRequiredAmendments(t *testing.T) {
	tx := NewBatch("rOuter")
	amendments := tx.RequiredAmendments()
	assert.Contains(t, amendments, AmendmentBatch)
}

// =============================================================================
// Constants Tests
// =============================================================================

func TestBatchConstants(t *testing.T) {
	assert.Equal(t, 8, MaxBatchTransactions)
	assert.Equal(t, uint32(0x00000001), BatchFlagAllOrNothing)
	assert.Equal(t, uint32(0x00000002), BatchFlagOnlyOne)
	assert.Equal(t, uint32(0x00000004), BatchFlagUntilFailure)
	assert.Equal(t, uint32(0x00000008), BatchFlagIndependent)
}
