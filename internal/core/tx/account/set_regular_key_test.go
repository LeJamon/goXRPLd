package account

import (
	tx2 "github.com/LeJamon/goXRPLd/internal/core/tx"
	"testing"
)

// TestSetRegularKeyValidation tests SetRegularKey transaction validation.
// These tests are translated from rippled's SetRegularKey_test.cpp focusing on
// validation logic. Note that signature verification and master key interactions
// require ledger state and are tested at a higher level.
func TestSetRegularKeyValidation(t *testing.T) {
	tests := []struct {
		name        string
		tx          *tx2.SetRegularKey
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid set regular key",
			tx: &tx2.SetRegularKey{
				BaseTx:     *tx2.NewBaseTx(tx2.TypeRegularKeySet, "rAlice"),
				RegularKey: "rBob",
			},
			expectError: false,
		},
		{
			name: "valid clear regular key (empty)",
			tx: &tx2.SetRegularKey{
				BaseTx:     *tx2.NewBaseTx(tx2.TypeRegularKeySet, "rAlice"),
				RegularKey: "",
			},
			expectError: false,
		},
		{
			name: "missing account",
			tx: &tx2.SetRegularKey{
				BaseTx:     tx2.BaseTx{Common: tx2.Common{TransactionType: "SetRegularKey"}},
				RegularKey: "rBob",
			},
			expectError: true,
			errorMsg:    "Account is required",
		},
		{
			name: "valid with ed25519 key format",
			tx: &tx2.SetRegularKey{
				BaseTx:     *tx2.NewBaseTx(tx2.TypeRegularKeySet, "rAlice"),
				RegularKey: "rEd25519Address",
			},
			expectError: false,
		},
		{
			name: "valid with secp256k1 key format",
			tx: &tx2.SetRegularKey{
				BaseTx:     *tx2.NewBaseTx(tx2.TypeRegularKeySet, "rAlice"),
				RegularKey: "rSecp256k1Address",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tx.Validate()
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestSetRegularKeyFlatten tests the Flatten method for SetRegularKey.
func TestSetRegularKeyFlatten(t *testing.T) {
	tests := []struct {
		name     string
		tx       *tx2.SetRegularKey
		checkMap func(t *testing.T, m map[string]any)
	}{
		{
			name: "set regular key",
			tx: &tx2.SetRegularKey{
				BaseTx:     *tx2.NewBaseTx(tx2.TypeRegularKeySet, "rAlice"),
				RegularKey: "rBob",
			},
			checkMap: func(t *testing.T, m map[string]any) {
				if m["Account"] != "rAlice" {
					t.Errorf("expected Account=rAlice, got %v", m["Account"])
				}
				if m["TransactionType"] != "SetRegularKey" {
					t.Errorf("expected TransactionType=SetRegularKey, got %v", m["TransactionType"])
				}
				if m["RegularKey"] != "rBob" {
					t.Errorf("expected RegularKey=rBob, got %v", m["RegularKey"])
				}
			},
		},
		{
			name: "clear regular key (no RegularKey field in output)",
			tx: &tx2.SetRegularKey{
				BaseTx:     *tx2.NewBaseTx(tx2.TypeRegularKeySet, "rAlice"),
				RegularKey: "",
			},
			checkMap: func(t *testing.T, m map[string]any) {
				if m["Account"] != "rAlice" {
					t.Errorf("expected Account=rAlice, got %v", m["Account"])
				}
				if _, ok := m["RegularKey"]; ok {
					t.Error("RegularKey should not be present when clearing")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := tt.tx.Flatten()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.checkMap(t, m)
		})
	}
}

// TestSetRegularKeyTransactionType tests that the transaction type is correctly returned.
func TestSetRegularKeyTransactionType(t *testing.T) {
	tx := tx2.NewSetRegularKey("rAlice")
	if tx.TxType() != tx2.TypeRegularKeySet {
		t.Errorf("expected TypeRegularKeySet, got %v", tx.TxType())
	}
}

// TestNewSetRegularKeyConstructor tests the constructor function.
func TestNewSetRegularKeyConstructor(t *testing.T) {
	tx := tx2.NewSetRegularKey("rAlice")
	if tx.Account != "rAlice" {
		t.Errorf("expected Account=rAlice, got %v", tx.Account)
	}
	if tx.TransactionType != "SetRegularKey" {
		t.Errorf("expected TransactionType=SetRegularKey, got %v", tx.TransactionType)
	}
	if tx.RegularKey != "" {
		t.Errorf("expected RegularKey to be empty by default, got %v", tx.RegularKey)
	}
}

// TestSetRegularKeyHelperMethods tests the SetKey and ClearKey helper methods.
func TestSetRegularKeyHelperMethods(t *testing.T) {
	t.Run("SetKey", func(t *testing.T) {
		tx := tx2.NewSetRegularKey("rAlice")
		tx.SetKey("rBob")
		if tx.RegularKey != "rBob" {
			t.Errorf("expected RegularKey=rBob, got %v", tx.RegularKey)
		}
	})

	t.Run("ClearKey", func(t *testing.T) {
		tx := tx2.NewSetRegularKey("rAlice")
		tx.SetKey("rBob")
		tx.ClearKey()
		if tx.RegularKey != "" {
			t.Errorf("expected RegularKey to be empty, got %v", tx.RegularKey)
		}
	})

	t.Run("SetKey after ClearKey", func(t *testing.T) {
		tx := tx2.NewSetRegularKey("rAlice")
		tx.ClearKey()
		tx.SetKey("rCarol")
		if tx.RegularKey != "rCarol" {
			t.Errorf("expected RegularKey=rCarol, got %v", tx.RegularKey)
		}
	})
}

// TestSignerListSetValidation tests SignerListSet transaction validation.
// This is included here as it's related to signing and authorization.
func TestSignerListSetValidation(t *testing.T) {
	tests := []struct {
		name        string
		tx          *tx2.SignerListSet
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid signer list with two signers",
			tx: &tx2.SignerListSet{
				BaseTx:       *tx2.NewBaseTx(tx2.TypeSignerListSet, "rAlice"),
				SignerQuorum: 2,
				SignerEntries: []tx2.SignerEntry{
					{SignerEntry: tx2.SignerEntryData{Account: "rBob", SignerWeight: 1}},
					{SignerEntry: tx2.SignerEntryData{Account: "rCarol", SignerWeight: 1}},
				},
			},
			expectError: false,
		},
		{
			name: "valid delete signer list (quorum=0)",
			tx: &tx2.SignerListSet{
				BaseTx:        *tx2.NewBaseTx(tx2.TypeSignerListSet, "rAlice"),
				SignerQuorum:  0,
				SignerEntries: nil,
			},
			expectError: false,
		},
		{
			name: "invalid: quorum=0 with entries",
			tx: &tx2.SignerListSet{
				BaseTx:       *tx2.NewBaseTx(tx2.TypeSignerListSet, "rAlice"),
				SignerQuorum: 0,
				SignerEntries: []tx2.SignerEntry{
					{SignerEntry: tx2.SignerEntryData{Account: "rBob", SignerWeight: 1}},
				},
			},
			expectError: true,
			errorMsg:    "cannot have SignerEntries when deleting signer list",
		},
		{
			name: "invalid: quorum>0 without entries",
			tx: &tx2.SignerListSet{
				BaseTx:        *tx2.NewBaseTx(tx2.TypeSignerListSet, "rAlice"),
				SignerQuorum:  2,
				SignerEntries: nil,
			},
			expectError: true,
			errorMsg:    "SignerEntries is required when setting signer list",
		},
		{
			name: "invalid: too many signers (>32)",
			tx: func() *tx2.SignerListSet {
				s := &tx2.SignerListSet{
					BaseTx:       *tx2.NewBaseTx(tx2.TypeSignerListSet, "rAlice"),
					SignerQuorum: 33,
				}
				for i := 0; i < 33; i++ {
					s.SignerEntries = append(s.SignerEntries, tx2.SignerEntry{
						SignerEntry: tx2.SignerEntryData{Account: "rSigner" + string(rune('A'+i)), SignerWeight: 1},
					})
				}
				return s
			}(),
			expectError: true,
			errorMsg:    "cannot have more than 32 signers",
		},
		{
			name: "invalid: duplicate signer",
			tx: &tx2.SignerListSet{
				BaseTx:       *tx2.NewBaseTx(tx2.TypeSignerListSet, "rAlice"),
				SignerQuorum: 2,
				SignerEntries: []tx2.SignerEntry{
					{SignerEntry: tx2.SignerEntryData{Account: "rBob", SignerWeight: 1}},
					{SignerEntry: tx2.SignerEntryData{Account: "rBob", SignerWeight: 1}},
				},
			},
			expectError: true,
			errorMsg:    "duplicate signer account",
		},
		{
			name: "invalid: self as signer",
			tx: &tx2.SignerListSet{
				BaseTx:       *tx2.NewBaseTx(tx2.TypeSignerListSet, "rAlice"),
				SignerQuorum: 2,
				SignerEntries: []tx2.SignerEntry{
					{SignerEntry: tx2.SignerEntryData{Account: "rAlice", SignerWeight: 1}},
					{SignerEntry: tx2.SignerEntryData{Account: "rBob", SignerWeight: 1}},
				},
			},
			expectError: true,
			errorMsg:    "cannot include self in signer list",
		},
		{
			name: "invalid: zero weight signer",
			tx: &tx2.SignerListSet{
				BaseTx:       *tx2.NewBaseTx(tx2.TypeSignerListSet, "rAlice"),
				SignerQuorum: 2,
				SignerEntries: []tx2.SignerEntry{
					{SignerEntry: tx2.SignerEntryData{Account: "rBob", SignerWeight: 0}},
					{SignerEntry: tx2.SignerEntryData{Account: "rCarol", SignerWeight: 2}},
				},
			},
			expectError: true,
			errorMsg:    "signer weight must be non-zero",
		},
		{
			name: "invalid: weights less than quorum",
			tx: &tx2.SignerListSet{
				BaseTx:       *tx2.NewBaseTx(tx2.TypeSignerListSet, "rAlice"),
				SignerQuorum: 5,
				SignerEntries: []tx2.SignerEntry{
					{SignerEntry: tx2.SignerEntryData{Account: "rBob", SignerWeight: 1}},
					{SignerEntry: tx2.SignerEntryData{Account: "rCarol", SignerWeight: 1}},
				},
			},
			expectError: true,
			errorMsg:    "total signer weight is less than quorum",
		},
		{
			name: "valid: weights equal to quorum",
			tx: &tx2.SignerListSet{
				BaseTx:       *tx2.NewBaseTx(tx2.TypeSignerListSet, "rAlice"),
				SignerQuorum: 3,
				SignerEntries: []tx2.SignerEntry{
					{SignerEntry: tx2.SignerEntryData{Account: "rBob", SignerWeight: 1}},
					{SignerEntry: tx2.SignerEntryData{Account: "rCarol", SignerWeight: 2}},
				},
			},
			expectError: false,
		},
		{
			name: "valid: weights greater than quorum",
			tx: &tx2.SignerListSet{
				BaseTx:       *tx2.NewBaseTx(tx2.TypeSignerListSet, "rAlice"),
				SignerQuorum: 2,
				SignerEntries: []tx2.SignerEntry{
					{SignerEntry: tx2.SignerEntryData{Account: "rBob", SignerWeight: 2}},
					{SignerEntry: tx2.SignerEntryData{Account: "rCarol", SignerWeight: 2}},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tx.Validate()
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestSignerListSetFlatten tests the Flatten method for SignerListSet.
func TestSignerListSetFlatten(t *testing.T) {
	tests := []struct {
		name     string
		tx       *tx2.SignerListSet
		checkMap func(t *testing.T, m map[string]any)
	}{
		{
			name: "with signers",
			tx: &tx2.SignerListSet{
				BaseTx:       *tx2.NewBaseTx(tx2.TypeSignerListSet, "rAlice"),
				SignerQuorum: 2,
				SignerEntries: []tx2.SignerEntry{
					{SignerEntry: tx2.SignerEntryData{Account: "rBob", SignerWeight: 1}},
					{SignerEntry: tx2.SignerEntryData{Account: "rCarol", SignerWeight: 1}},
				},
			},
			checkMap: func(t *testing.T, m map[string]any) {
				if m["SignerQuorum"] != uint32(2) {
					t.Errorf("expected SignerQuorum=2, got %v", m["SignerQuorum"])
				}
				entries, ok := m["SignerEntries"].([]tx2.SignerEntry)
				if !ok {
					t.Fatalf("SignerEntries should be []SignerEntry, got %T", m["SignerEntries"])
				}
				if len(entries) != 2 {
					t.Errorf("expected 2 entries, got %d", len(entries))
				}
			},
		},
		{
			name: "delete (no entries)",
			tx: &tx2.SignerListSet{
				BaseTx:       *tx2.NewBaseTx(tx2.TypeSignerListSet, "rAlice"),
				SignerQuorum: 0,
			},
			checkMap: func(t *testing.T, m map[string]any) {
				if m["SignerQuorum"] != uint32(0) {
					t.Errorf("expected SignerQuorum=0, got %v", m["SignerQuorum"])
				}
				if _, ok := m["SignerEntries"]; ok {
					t.Error("SignerEntries should not be present when deleting")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := tt.tx.Flatten()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.checkMap(t, m)
		})
	}
}

// TestSignerListSetAddSigner tests the AddSigner helper method.
func TestSignerListSetAddSigner(t *testing.T) {
	s := tx2.NewSignerListSet("rAlice", 2)
	s.AddSigner("rBob", 1)
	s.AddSigner("rCarol", 2)

	if len(s.SignerEntries) != 2 {
		t.Errorf("expected 2 signers, got %d", len(s.SignerEntries))
	}

	if s.SignerEntries[0].SignerEntry.Account != "rBob" {
		t.Errorf("expected first signer=rBob, got %v", s.SignerEntries[0].SignerEntry.Account)
	}
	if s.SignerEntries[0].SignerEntry.SignerWeight != 1 {
		t.Errorf("expected first weight=1, got %v", s.SignerEntries[0].SignerEntry.SignerWeight)
	}

	if s.SignerEntries[1].SignerEntry.Account != "rCarol" {
		t.Errorf("expected second signer=rCarol, got %v", s.SignerEntries[1].SignerEntry.Account)
	}
	if s.SignerEntries[1].SignerEntry.SignerWeight != 2 {
		t.Errorf("expected second weight=2, got %v", s.SignerEntries[1].SignerEntry.SignerWeight)
	}
}

// TestNewSignerListSetConstructor tests the constructor function.
func TestNewSignerListSetConstructor(t *testing.T) {
	s := tx2.NewSignerListSet("rAlice", 3)
	if s.Account != "rAlice" {
		t.Errorf("expected Account=rAlice, got %v", s.Account)
	}
	if s.SignerQuorum != 3 {
		t.Errorf("expected SignerQuorum=3, got %v", s.SignerQuorum)
	}
	if len(s.SignerEntries) != 0 {
		t.Errorf("expected empty SignerEntries, got %d entries", len(s.SignerEntries))
	}
}

// TestSignerListSetTransactionType tests that the transaction type is correctly returned.
func TestSignerListSetTransactionType(t *testing.T) {
	s := tx2.NewSignerListSet("rAlice", 2)
	if s.TxType() != tx2.TypeSignerListSet {
		t.Errorf("expected TypeSignerListSet, got %v", s.TxType())
	}
}
