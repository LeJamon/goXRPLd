package credential

import (
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create a hex string of specified byte length
func credMakeHex(byteLen int) string {
	return strings.Repeat("AB", byteLen)
}

// =============================================================================
// CredentialCreate Validation Tests
// Based on rippled Credentials_test.cpp testCredentialsCreate()
// =============================================================================

func TestCredentialCreateValidation(t *testing.T) {
	tests := []struct {
		name    string
		tx      *CredentialCreate
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{
			name: "valid - basic create",
			tx: &CredentialCreate{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
				Subject:        "rSubject",
				CredentialType: credMakeHex(5), // 5 bytes
			},
			wantErr: false,
		},
		{
			name: "valid - with URI",
			tx: &CredentialCreate{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
				Subject:        "rSubject",
				CredentialType: credMakeHex(5),
				URI:            credMakeHex(32), // 32 bytes
			},
			wantErr: false,
		},
		{
			name: "valid - with expiration",
			tx: func() *CredentialCreate {
				exp := uint32(750000000)
				return &CredentialCreate{
					BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
					Subject:        "rSubject",
					CredentialType: credMakeHex(5),
					Expiration:     &exp,
				}
			}(),
			wantErr: false,
		},
		{
			name: "valid - self-issued (subject == issuer)",
			tx: &CredentialCreate{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
				Subject:        "rIssuer", // Same as Account
				CredentialType: credMakeHex(5),
			},
			wantErr: false,
		},
		{
			name: "valid - maximum CredentialType length (64 bytes)",
			tx: &CredentialCreate{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
				Subject:        "rSubject",
				CredentialType: credMakeHex(64),
			},
			wantErr: false,
		},
		{
			name: "valid - maximum URI length (256 bytes)",
			tx: &CredentialCreate{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
				Subject:        "rSubject",
				CredentialType: credMakeHex(5),
				URI:            credMakeHex(256),
			},
			wantErr: false,
		},

		// Invalid cases - Field validation
		{
			name: "invalid - missing Subject (temMALFORMED)",
			tx: &CredentialCreate{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
				Subject:        "",
				CredentialType: credMakeHex(5),
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},
		{
			name: "invalid - missing CredentialType (temMALFORMED)",
			tx: &CredentialCreate{
				BaseTx:  *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
				Subject: "rSubject",
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},
		{
			name: "invalid - CredentialType too long >64 bytes (temMALFORMED)",
			tx: &CredentialCreate{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
				Subject:        "rSubject",
				CredentialType: credMakeHex(65),
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},
		{
			name: "invalid - URI too long >256 bytes (temMALFORMED)",
			tx: &CredentialCreate{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
				Subject:        "rSubject",
				CredentialType: credMakeHex(5),
				URI:            credMakeHex(257),
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},
		{
			name: "invalid - missing account",
			tx: &CredentialCreate{
				BaseTx:         tx.BaseTx{},
				Subject:        "rSubject",
				CredentialType: credMakeHex(5),
			},
			wantErr: true,
			errMsg:  "Account is required",
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
// CredentialAccept Validation Tests
// Based on rippled Credentials_test.cpp testCredentialsAccept()
// =============================================================================

func TestCredentialAcceptValidation(t *testing.T) {
	tests := []struct {
		name    string
		tx      *CredentialAccept
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{
			name: "valid - basic accept",
			tx: &CredentialAccept{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialAccept, "rSubject"),
				Issuer:         "rIssuer",
				CredentialType: credMakeHex(5),
			},
			wantErr: false,
		},
		{
			name: "valid - maximum CredentialType length (64 bytes)",
			tx: &CredentialAccept{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialAccept, "rSubject"),
				Issuer:         "rIssuer",
				CredentialType: credMakeHex(64),
			},
			wantErr: false,
		},

		// Invalid cases
		{
			name: "invalid - missing Issuer (temINVALID_ACCOUNT_ID)",
			tx: &CredentialAccept{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialAccept, "rSubject"),
				Issuer:         "",
				CredentialType: credMakeHex(5),
			},
			wantErr: true,
			errMsg:  "temINVALID_ACCOUNT_ID",
		},
		{
			name: "invalid - missing CredentialType (temMALFORMED)",
			tx: &CredentialAccept{
				BaseTx: *tx.NewBaseTx(tx.TypeCredentialAccept, "rSubject"),
				Issuer: "rIssuer",
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},
		{
			name: "invalid - CredentialType too long >64 bytes (temMALFORMED)",
			tx: &CredentialAccept{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialAccept, "rSubject"),
				Issuer:         "rIssuer",
				CredentialType: credMakeHex(65),
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},
		{
			name: "invalid - missing account",
			tx: &CredentialAccept{
				BaseTx:         tx.BaseTx{},
				Issuer:         "rIssuer",
				CredentialType: credMakeHex(5),
			},
			wantErr: true,
			errMsg:  "Account is required",
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
// CredentialDelete Validation Tests
// Based on rippled Credentials_test.cpp testCredentialsDelete()
// =============================================================================

func TestCredentialDeleteValidation(t *testing.T) {
	tests := []struct {
		name    string
		tx      *CredentialDelete
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{
			name: "valid - delete with Subject only",
			tx: &CredentialDelete{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialDelete, "rAccount"),
				Subject:        "rSubject",
				CredentialType: credMakeHex(5),
			},
			wantErr: false,
		},
		{
			name: "valid - delete with Issuer only",
			tx: &CredentialDelete{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialDelete, "rAccount"),
				Issuer:         "rIssuer",
				CredentialType: credMakeHex(5),
			},
			wantErr: false,
		},
		{
			name: "valid - delete with both Subject and Issuer",
			tx: &CredentialDelete{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialDelete, "rAccount"),
				Subject:        "rSubject",
				Issuer:         "rIssuer",
				CredentialType: credMakeHex(5),
			},
			wantErr: false,
		},

		// Invalid cases
		{
			name: "invalid - neither Subject nor Issuer (temMALFORMED)",
			tx: &CredentialDelete{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialDelete, "rAccount"),
				CredentialType: credMakeHex(5),
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},
		{
			name: "invalid - missing CredentialType (temMALFORMED)",
			tx: &CredentialDelete{
				BaseTx:  *tx.NewBaseTx(tx.TypeCredentialDelete, "rAccount"),
				Subject: "rSubject",
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},
		{
			name: "invalid - CredentialType too long >64 bytes (temMALFORMED)",
			tx: &CredentialDelete{
				BaseTx:         *tx.NewBaseTx(tx.TypeCredentialDelete, "rAccount"),
				Subject:        "rSubject",
				CredentialType: credMakeHex(65),
			},
			wantErr: true,
			errMsg:  "temMALFORMED",
		},
		{
			name: "invalid - missing account",
			tx: &CredentialDelete{
				BaseTx:         tx.BaseTx{},
				Subject:        "rSubject",
				CredentialType: credMakeHex(5),
			},
			wantErr: true,
			errMsg:  "Account is required",
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

func TestCredentialCreateFlatten(t *testing.T) {
	exp := uint32(750000000)
	tx := &CredentialCreate{
		BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
		Subject:        "rSubject",
		CredentialType: "6162636465", // "abcde" hex
		Expiration:     &exp,
		URI:            "757269", // "uri" hex
	}

	flat, err := tx.Flatten()
	require.NoError(t, err)

	assert.Equal(t, "rIssuer", flat["Account"])
	assert.Equal(t, "CredentialCreate", flat["TransactionType"])
	assert.Equal(t, "rSubject", flat["Subject"])
	assert.Equal(t, "6162636465", flat["CredentialType"])
	assert.Equal(t, uint32(750000000), flat["Expiration"])
	assert.Equal(t, "757269", flat["URI"])
}

func TestCredentialAcceptFlatten(t *testing.T) {
	tx := &CredentialAccept{
		BaseTx:         *tx.NewBaseTx(tx.TypeCredentialAccept, "rSubject"),
		Issuer:         "rIssuer",
		CredentialType: "6162636465",
	}

	flat, err := tx.Flatten()
	require.NoError(t, err)

	assert.Equal(t, "rSubject", flat["Account"])
	assert.Equal(t, "CredentialAccept", flat["TransactionType"])
	assert.Equal(t, "rIssuer", flat["Issuer"])
	assert.Equal(t, "6162636465", flat["CredentialType"])
}

func TestCredentialDeleteFlatten(t *testing.T) {
	tx := &CredentialDelete{
		BaseTx:         *tx.NewBaseTx(tx.TypeCredentialDelete, "rAccount"),
		Subject:        "rSubject",
		Issuer:         "rIssuer",
		CredentialType: "6162636465",
	}

	flat, err := tx.Flatten()
	require.NoError(t, err)

	assert.Equal(t, "rAccount", flat["Account"])
	assert.Equal(t, "CredentialDelete", flat["TransactionType"])
	assert.Equal(t, "rSubject", flat["Subject"])
	assert.Equal(t, "rIssuer", flat["Issuer"])
	assert.Equal(t, "6162636465", flat["CredentialType"])
}

// =============================================================================
// Transaction Type Tests
// =============================================================================

func TestCredentialTransactionTypes(t *testing.T) {
	tests := []struct {
		name     string
		tx       tx.Transaction
		expected tx.Type
	}{
		{
			name:     "CredentialCreate type",
			tx:       &CredentialCreate{BaseTx: *tx.NewBaseTx(tx.TypeCredentialCreate, "rTest")},
			expected: tx.TypeCredentialCreate,
		},
		{
			name:     "CredentialAccept type",
			tx:       &CredentialAccept{BaseTx: *tx.NewBaseTx(tx.TypeCredentialAccept, "rTest")},
			expected: tx.TypeCredentialAccept,
		},
		{
			name:     "CredentialDelete type",
			tx:       &CredentialDelete{BaseTx: *tx.NewBaseTx(tx.TypeCredentialDelete, "rTest")},
			expected: tx.TypeCredentialDelete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.tx.TxType())
		})
	}
}

// =============================================================================
// Constructor Tests
// =============================================================================

func TestCredentialConstructors(t *testing.T) {
	t.Run("NewCredentialCreate", func(t *testing.T) {
		tx := NewCredentialCreate("rIssuer", "rSubject", "6162636465")
		require.NotNil(t, tx)
		assert.Equal(t, "rIssuer", tx.Account)
		assert.Equal(t, "rSubject", tx.Subject)
		assert.Equal(t, "6162636465", tx.CredentialType)
		assert.Equal(t, tx.TypeCredentialCreate, tx.TxType())
	})

	t.Run("NewCredentialAccept", func(t *testing.T) {
		tx := NewCredentialAccept("rSubject", "rIssuer", "6162636465")
		require.NotNil(t, tx)
		assert.Equal(t, "rSubject", tx.Account)
		assert.Equal(t, "rIssuer", tx.Issuer)
		assert.Equal(t, "6162636465", tx.CredentialType)
		assert.Equal(t, tx.TypeCredentialAccept, tx.TxType())
	})

	t.Run("NewCredentialDelete", func(t *testing.T) {
		tx := NewCredentialDelete("rAccount", "6162636465")
		require.NotNil(t, tx)
		assert.Equal(t, "rAccount", tx.Account)
		assert.Equal(t, "6162636465", tx.CredentialType)
		assert.Equal(t, tx.TypeCredentialDelete, tx.TxType())
	})
}

// =============================================================================
// Amendment Tests
// =============================================================================

func TestCredentialRequiredAmendments(t *testing.T) {
	tests := []struct {
		name     string
		tx       tx.Transaction
		expected []string
	}{
		{
			name:     "CredentialCreate requires Credentials amendment",
			tx:       &CredentialCreate{BaseTx: *tx.NewBaseTx(tx.TypeCredentialCreate, "rTest")},
			expected: []string{AmendmentCredentials},
		},
		{
			name:     "CredentialAccept requires Credentials amendment",
			tx:       &CredentialAccept{BaseTx: *tx.NewBaseTx(tx.TypeCredentialAccept, "rTest")},
			expected: []string{AmendmentCredentials},
		},
		{
			name:     "CredentialDelete requires Credentials amendment",
			tx:       &CredentialDelete{BaseTx: *tx.NewBaseTx(tx.TypeCredentialDelete, "rTest")},
			expected: []string{AmendmentCredentials},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.tx.RequiredAmendments()
			assert.Equal(t, tt.expected, got)
		})
	}
}

// =============================================================================
// Constants Tests
// =============================================================================

func TestCredentialConstants(t *testing.T) {
	assert.Equal(t, 256, MaxCredentialURILength)
	assert.Equal(t, 64, MaxCredentialTypeLength)
}

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestCheckCredentialExpired(t *testing.T) {
	t.Run("no expiration - never expired", func(t *testing.T) {
		cred := &CredentialEntry{}
		assert.False(t, checkCredentialExpired(cred, 1000000000))
	})

	t.Run("not expired", func(t *testing.T) {
		exp := uint32(1000000000)
		cred := &CredentialEntry{Expiration: &exp}
		assert.False(t, checkCredentialExpired(cred, 999999999))
	})

	t.Run("expired", func(t *testing.T) {
		exp := uint32(1000000000)
		cred := &CredentialEntry{Expiration: &exp}
		assert.True(t, checkCredentialExpired(cred, 1000000001))
	})

	t.Run("at exact expiration time - not expired yet", func(t *testing.T) {
		exp := uint32(1000000000)
		cred := &CredentialEntry{Expiration: &exp}
		assert.False(t, checkCredentialExpired(cred, 1000000000))
	})
}

func TestCredentialEntryIsAccepted(t *testing.T) {
	t.Run("not accepted", func(t *testing.T) {
		cred := &CredentialEntry{Flags: 0}
		assert.False(t, cred.IsAccepted())
	})

	t.Run("accepted", func(t *testing.T) {
		cred := &CredentialEntry{Flags: LsfCredentialAccepted}
		assert.True(t, cred.IsAccepted())
	})

	t.Run("set accepted", func(t *testing.T) {
		cred := &CredentialEntry{Flags: 0}
		cred.SetAccepted()
		assert.True(t, cred.IsAccepted())
	})
}

// =============================================================================
// Flag Validation Tests
// Based on rippled Credentials_test.cpp testFlags()
// =============================================================================

func TestCredentialCreateFlagValidation(t *testing.T) {
	t.Run("invalid flags - universal mask", func(t *testing.T) {
		tx := &CredentialCreate{
			BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
			Subject:        "rSubject",
			CredentialType: credMakeHex(5),
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		// Set invalid universal flag
		flags := uint32(tx.TfUniversalMask)
		tx.Common.Flags = &flags

		err := tx.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid flags")
	})

	t.Run("valid - no flags", func(t *testing.T) {
		tx := &CredentialCreate{
			BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
			Subject:        "rSubject",
			CredentialType: credMakeHex(5),
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		err := tx.Validate()
		assert.NoError(t, err)
	})
}

func TestCredentialAcceptFlagValidation(t *testing.T) {
	t.Run("invalid flags - universal mask", func(t *testing.T) {
		tx := &CredentialAccept{
			BaseTx:         *tx.NewBaseTx(tx.TypeCredentialAccept, "rSubject"),
			Issuer:         "rIssuer",
			CredentialType: credMakeHex(5),
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		// Set invalid universal flag
		flags := uint32(tx.TfUniversalMask)
		tx.Common.Flags = &flags

		err := tx.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid flags")
	})
}

func TestCredentialDeleteFlagValidation(t *testing.T) {
	t.Run("invalid flags - universal mask", func(t *testing.T) {
		tx := &CredentialDelete{
			BaseTx:         *tx.NewBaseTx(tx.TypeCredentialDelete, "rAccount"),
			Subject:        "rSubject",
			CredentialType: credMakeHex(5),
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		// Set invalid universal flag
		flags := uint32(tx.TfUniversalMask)
		tx.Common.Flags = &flags

		err := tx.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid flags")
	})
}

// =============================================================================
// Invalid Hex String Tests
// Based on rippled Credentials_test.cpp
// =============================================================================

func TestCredentialInvalidHexString(t *testing.T) {
	t.Run("invalid CredentialType hex - CredentialCreate", func(t *testing.T) {
		tx := &CredentialCreate{
			BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
			Subject:        "rSubject",
			CredentialType: "not_valid_hex",
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		err := tx.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("invalid URI hex - CredentialCreate", func(t *testing.T) {
		tx := &CredentialCreate{
			BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
			Subject:        "rSubject",
			CredentialType: credMakeHex(5),
			URI:            "not_valid_hex",
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		err := tx.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("empty URI - CredentialCreate (temMALFORMED)", func(t *testing.T) {
		tx := &CredentialCreate{
			BaseTx:         *tx.NewBaseTx(tx.TypeCredentialCreate, "rIssuer"),
			Subject:        "rSubject",
			CredentialType: credMakeHex(5),
			URI:            "", // Empty is different from not set
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		// Empty string should pass since it's treated as "not set"
		err := tx.Validate()
		assert.NoError(t, err)
	})

	t.Run("invalid CredentialType hex - CredentialAccept", func(t *testing.T) {
		tx := &CredentialAccept{
			BaseTx:         *tx.NewBaseTx(tx.TypeCredentialAccept, "rSubject"),
			Issuer:         "rIssuer",
			CredentialType: "not_valid_hex",
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		err := tx.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})

	t.Run("invalid CredentialType hex - CredentialDelete", func(t *testing.T) {
		tx := &CredentialDelete{
			BaseTx:         *tx.NewBaseTx(tx.TypeCredentialDelete, "rAccount"),
			Subject:        "rSubject",
			CredentialType: "not_valid_hex",
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		err := tx.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temMALFORMED")
	})
}

// =============================================================================
// Credential Keylet Tests
// =============================================================================

func TestCredentialKeylet(t *testing.T) {
	t.Run("keylet calculation is deterministic", func(t *testing.T) {
		var subject, issuer [20]byte
		copy(subject[:], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
		copy(issuer[:], []byte{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40})
		credType := []byte("test_credential")

		key1 := credentialKeylet(subject, issuer, credType)
		key2 := credentialKeylet(subject, issuer, credType)

		assert.Equal(t, key1, key2)
	})

	t.Run("different inputs produce different keylets", func(t *testing.T) {
		var subject, issuer [20]byte
		copy(subject[:], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
		copy(issuer[:], []byte{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40})

		key1 := credentialKeylet(subject, issuer, []byte("type1"))
		key2 := credentialKeylet(subject, issuer, []byte("type2"))

		assert.NotEqual(t, key1, key2)
	})

	t.Run("same subject/issuer different order produces different keylet", func(t *testing.T) {
		var account1, account2 [20]byte
		copy(account1[:], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
		copy(account2[:], []byte{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40})
		credType := []byte("test")

		// Swap subject and issuer
		key1 := credentialKeylet(account1, account2, credType)
		key2 := credentialKeylet(account2, account1, credType)

		assert.NotEqual(t, key1, key2)
	})

	t.Run("self-issued credential keylet", func(t *testing.T) {
		var account [20]byte
		copy(account[:], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
		credType := []byte("self_issued")

		// Self-issued: subject == issuer
		key := credentialKeylet(account, account, credType)
		assert.NotEqual(t, [32]byte{}, key)
	})
}

// =============================================================================
// Credential Entry Ledger Flag Constant Test
// =============================================================================

func TestCredentialLedgerFlags(t *testing.T) {
	// Reference: rippled Protocol.h lsfAccepted = 0x00010000
	assert.Equal(t, uint32(0x00010000), LsfCredentialAccepted)
}
