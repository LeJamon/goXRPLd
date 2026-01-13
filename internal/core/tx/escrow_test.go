package tx

import (
	"testing"
)

// TestEscrowCreateValidation tests EscrowCreate transaction validation.
// These tests are translated from rippled's Escrow_test.cpp focusing on
// validation logic and error conditions.
func TestEscrowCreateValidation(t *testing.T) {
	tests := []struct {
		name        string
		escrow      *EscrowCreate
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid escrow with finish time",
			escrow: &EscrowCreate{
				BaseTx:      *NewBaseTx(TypeEscrowCreate, "rAlice"),
				Amount:      NewXRPAmount("1000000000"), // 1000 XRP in drops
				Destination: "rBob",
				FinishAfter: ptrUint32(700000000), // Some future time
			},
			expectError: false,
		},
		{
			name: "valid escrow with cancel time and condition",
			escrow: &EscrowCreate{
				BaseTx:      *NewBaseTx(TypeEscrowCreate, "rAlice"),
				Amount:      NewXRPAmount("1000000000"),
				Destination: "rBob",
				CancelAfter: ptrUint32(700000100),
				Condition:   "A0258020E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855810100",
			},
			expectError: false,
		},
		{
			name: "valid escrow with both times",
			escrow: &EscrowCreate{
				BaseTx:      *NewBaseTx(TypeEscrowCreate, "rAlice"),
				Amount:      NewXRPAmount("1000000000"),
				Destination: "rBob",
				FinishAfter: ptrUint32(700000000),
				CancelAfter: ptrUint32(700000100), // CancelAfter > FinishAfter
			},
			expectError: false,
		},
		{
			name: "missing destination - temDST_NEEDED equivalent",
			escrow: &EscrowCreate{
				BaseTx:      *NewBaseTx(TypeEscrowCreate, "rAlice"),
				Amount:      NewXRPAmount("1000000000"),
				Destination: "",
				FinishAfter: ptrUint32(700000000),
			},
			expectError: true,
			errorMsg:    "Destination is required",
		},
		{
			name: "missing amount - temBAD_AMOUNT equivalent",
			escrow: &EscrowCreate{
				BaseTx:      *NewBaseTx(TypeEscrowCreate, "rAlice"),
				Amount:      Amount{},
				Destination: "rBob",
				FinishAfter: ptrUint32(700000000),
			},
			expectError: true,
			errorMsg:    "Amount is required",
		},
		{
			name: "non-XRP amount - temBAD_AMOUNT equivalent",
			escrow: &EscrowCreate{
				BaseTx:      *NewBaseTx(TypeEscrowCreate, "rAlice"),
				Amount:      NewIssuedAmount("100", "USD", "rGateway"),
				Destination: "rBob",
				FinishAfter: ptrUint32(700000000),
			},
			expectError: true,
			errorMsg:    "escrow can only hold XRP",
		},
		{
			name: "missing both times and condition - temBAD_EXPIRATION equivalent",
			escrow: &EscrowCreate{
				BaseTx:      *NewBaseTx(TypeEscrowCreate, "rAlice"),
				Amount:      NewXRPAmount("1000000000"),
				Destination: "rBob",
				// No CancelAfter, FinishAfter, or Condition
			},
			expectError: true,
			errorMsg:    "must specify CancelAfter, FinishAfter, or Condition",
		},
		{
			name: "cancel time equals finish time - temBAD_EXPIRATION equivalent",
			escrow: &EscrowCreate{
				BaseTx:      *NewBaseTx(TypeEscrowCreate, "rAlice"),
				Amount:      NewXRPAmount("1000000000"),
				Destination: "rBob",
				FinishAfter: ptrUint32(700000000),
				CancelAfter: ptrUint32(700000000), // Equal, should fail
			},
			expectError: true,
			errorMsg:    "CancelAfter must be after FinishAfter",
		},
		{
			name: "cancel time before finish time - temBAD_EXPIRATION equivalent",
			escrow: &EscrowCreate{
				BaseTx:      *NewBaseTx(TypeEscrowCreate, "rAlice"),
				Amount:      NewXRPAmount("1000000000"),
				Destination: "rBob",
				FinishAfter: ptrUint32(700000100),
				CancelAfter: ptrUint32(700000000), // Before, should fail
			},
			expectError: true,
			errorMsg:    "CancelAfter must be after FinishAfter",
		},
		{
			name: "valid escrow with only condition (fix1571 behavior)",
			escrow: &EscrowCreate{
				BaseTx:      *NewBaseTx(TypeEscrowCreate, "rAlice"),
				Amount:      NewXRPAmount("1000000000"),
				Destination: "rBob",
				CancelAfter: ptrUint32(700000100),
				Condition:   "A0258020E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855810100",
			},
			expectError: false,
		},
		{
			name: "missing account - temBAD_SRC_ACCOUNT equivalent",
			escrow: &EscrowCreate{
				BaseTx:      BaseTx{Common: Common{TransactionType: "EscrowCreate"}},
				Amount:      NewXRPAmount("1000000000"),
				Destination: "rBob",
				FinishAfter: ptrUint32(700000000),
			},
			expectError: true,
			errorMsg:    "Account is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.escrow.Validate()
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

// TestEscrowFinishValidation tests EscrowFinish transaction validation.
// Tests translated from rippled's testLockup and testEscrowConditions.
func TestEscrowFinishValidation(t *testing.T) {
	tests := []struct {
		name        string
		escrow      *EscrowFinish
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid unconditional finish",
			escrow: &EscrowFinish{
				BaseTx:        *NewBaseTx(TypeEscrowFinish, "rBob"),
				Owner:         "rAlice",
				OfferSequence: 12345,
			},
			expectError: false,
		},
		{
			name: "valid conditional finish with both condition and fulfillment",
			escrow: &EscrowFinish{
				BaseTx:        *NewBaseTx(TypeEscrowFinish, "rBob"),
				Owner:         "rAlice",
				OfferSequence: 12345,
				Condition:     "A0258020E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855810100",
				Fulfillment:   "A0028000",
			},
			expectError: false,
		},
		{
			name: "missing owner",
			escrow: &EscrowFinish{
				BaseTx:        *NewBaseTx(TypeEscrowFinish, "rBob"),
				Owner:         "",
				OfferSequence: 12345,
			},
			expectError: true,
			errorMsg:    "Owner is required",
		},
		{
			name: "condition without fulfillment - temMALFORMED equivalent",
			escrow: &EscrowFinish{
				BaseTx:        *NewBaseTx(TypeEscrowFinish, "rBob"),
				Owner:         "rAlice",
				OfferSequence: 12345,
				Condition:     "A0258020E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855810100",
				// Missing Fulfillment
			},
			expectError: true,
			errorMsg:    "Condition and Fulfillment must be provided together",
		},
		{
			name: "fulfillment without condition - temMALFORMED equivalent",
			escrow: &EscrowFinish{
				BaseTx:        *NewBaseTx(TypeEscrowFinish, "rBob"),
				Owner:         "rAlice",
				OfferSequence: 12345,
				Fulfillment:   "A0028000",
				// Missing Condition
			},
			expectError: true,
			errorMsg:    "Condition and Fulfillment must be provided together",
		},
		{
			name: "missing account",
			escrow: &EscrowFinish{
				BaseTx:        BaseTx{Common: Common{TransactionType: "EscrowFinish"}},
				Owner:         "rAlice",
				OfferSequence: 12345,
			},
			expectError: true,
			errorMsg:    "Account is required",
		},
		{
			name: "self-finish allowed (unconditional)",
			escrow: &EscrowFinish{
				BaseTx:        *NewBaseTx(TypeEscrowFinish, "rAlice"),
				Owner:         "rAlice",
				OfferSequence: 12345,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.escrow.Validate()
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

// TestEscrowCancelValidation tests EscrowCancel transaction validation.
func TestEscrowCancelValidation(t *testing.T) {
	tests := []struct {
		name        string
		escrow      *EscrowCancel
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid cancel",
			escrow: &EscrowCancel{
				BaseTx:        *NewBaseTx(TypeEscrowCancel, "rBob"),
				Owner:         "rAlice",
				OfferSequence: 12345,
			},
			expectError: false,
		},
		{
			name: "self cancel allowed",
			escrow: &EscrowCancel{
				BaseTx:        *NewBaseTx(TypeEscrowCancel, "rAlice"),
				Owner:         "rAlice",
				OfferSequence: 12345,
			},
			expectError: false,
		},
		{
			name: "missing owner",
			escrow: &EscrowCancel{
				BaseTx:        *NewBaseTx(TypeEscrowCancel, "rBob"),
				Owner:         "",
				OfferSequence: 12345,
			},
			expectError: true,
			errorMsg:    "Owner is required",
		},
		{
			name: "missing account",
			escrow: &EscrowCancel{
				BaseTx:        BaseTx{Common: Common{TransactionType: "EscrowCancel"}},
				Owner:         "rAlice",
				OfferSequence: 12345,
			},
			expectError: true,
			errorMsg:    "Account is required",
		},
		{
			name: "sequence zero is valid",
			escrow: &EscrowCancel{
				BaseTx:        *NewBaseTx(TypeEscrowCancel, "rBob"),
				Owner:         "rAlice",
				OfferSequence: 0,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.escrow.Validate()
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

// TestEscrowCreateFlatten tests the Flatten method for EscrowCreate.
func TestEscrowCreateFlatten(t *testing.T) {
	tests := []struct {
		name     string
		escrow   *EscrowCreate
		checkMap func(t *testing.T, m map[string]any)
	}{
		{
			name: "basic escrow",
			escrow: &EscrowCreate{
				BaseTx:      *NewBaseTx(TypeEscrowCreate, "rAlice"),
				Amount:      NewXRPAmount("1000000000"),
				Destination: "rBob",
				FinishAfter: ptrUint32(700000000),
			},
			checkMap: func(t *testing.T, m map[string]any) {
				if m["Account"] != "rAlice" {
					t.Errorf("expected Account=rAlice, got %v", m["Account"])
				}
				if m["Destination"] != "rBob" {
					t.Errorf("expected Destination=rBob, got %v", m["Destination"])
				}
				if m["Amount"] != "1000000000" {
					t.Errorf("expected Amount=1000000000, got %v", m["Amount"])
				}
				if m["FinishAfter"] != uint32(700000000) {
					t.Errorf("expected FinishAfter=700000000, got %v", m["FinishAfter"])
				}
			},
		},
		{
			name: "escrow with all optional fields",
			escrow: &EscrowCreate{
				BaseTx:         *NewBaseTx(TypeEscrowCreate, "rAlice"),
				Amount:         NewXRPAmount("2000000000"),
				Destination:    "rBob",
				DestinationTag: ptrUint32(42),
				FinishAfter:    ptrUint32(700000000),
				CancelAfter:    ptrUint32(800000000),
				Condition:      "DEADBEEF",
			},
			checkMap: func(t *testing.T, m map[string]any) {
				if m["DestinationTag"] != uint32(42) {
					t.Errorf("expected DestinationTag=42, got %v", m["DestinationTag"])
				}
				if m["CancelAfter"] != uint32(800000000) {
					t.Errorf("expected CancelAfter=800000000, got %v", m["CancelAfter"])
				}
				if m["Condition"] != "DEADBEEF" {
					t.Errorf("expected Condition=DEADBEEF, got %v", m["Condition"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := tt.escrow.Flatten()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.checkMap(t, m)
		})
	}
}

// TestEscrowFinishFlatten tests the Flatten method for EscrowFinish.
func TestEscrowFinishFlatten(t *testing.T) {
	tests := []struct {
		name     string
		escrow   *EscrowFinish
		checkMap func(t *testing.T, m map[string]any)
	}{
		{
			name: "unconditional finish",
			escrow: &EscrowFinish{
				BaseTx:        *NewBaseTx(TypeEscrowFinish, "rBob"),
				Owner:         "rAlice",
				OfferSequence: 12345,
			},
			checkMap: func(t *testing.T, m map[string]any) {
				if m["Owner"] != "rAlice" {
					t.Errorf("expected Owner=rAlice, got %v", m["Owner"])
				}
				if m["OfferSequence"] != uint32(12345) {
					t.Errorf("expected OfferSequence=12345, got %v", m["OfferSequence"])
				}
				if _, ok := m["Condition"]; ok {
					t.Error("Condition should not be present")
				}
			},
		},
		{
			name: "conditional finish",
			escrow: &EscrowFinish{
				BaseTx:        *NewBaseTx(TypeEscrowFinish, "rBob"),
				Owner:         "rAlice",
				OfferSequence: 12345,
				Condition:     "DEADBEEF",
				Fulfillment:   "CAFEBABE",
			},
			checkMap: func(t *testing.T, m map[string]any) {
				if m["Condition"] != "DEADBEEF" {
					t.Errorf("expected Condition=DEADBEEF, got %v", m["Condition"])
				}
				if m["Fulfillment"] != "CAFEBABE" {
					t.Errorf("expected Fulfillment=CAFEBABE, got %v", m["Fulfillment"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := tt.escrow.Flatten()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.checkMap(t, m)
		})
	}
}

// TestEscrowCancelFlatten tests the Flatten method for EscrowCancel.
func TestEscrowCancelFlatten(t *testing.T) {
	escrow := &EscrowCancel{
		BaseTx:        *NewBaseTx(TypeEscrowCancel, "rBob"),
		Owner:         "rAlice",
		OfferSequence: 12345,
	}

	m, err := escrow.Flatten()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m["Owner"] != "rAlice" {
		t.Errorf("expected Owner=rAlice, got %v", m["Owner"])
	}
	if m["OfferSequence"] != uint32(12345) {
		t.Errorf("expected OfferSequence=12345, got %v", m["OfferSequence"])
	}
}

// TestEscrowTransactionTypes tests that transaction types are correctly returned.
func TestEscrowTransactionTypes(t *testing.T) {
	t.Run("EscrowCreate type", func(t *testing.T) {
		e := NewEscrowCreate("rAlice", "rBob", NewXRPAmount("1000000"))
		if e.TxType() != TypeEscrowCreate {
			t.Errorf("expected TypeEscrowCreate, got %v", e.TxType())
		}
	})

	t.Run("EscrowFinish type", func(t *testing.T) {
		e := NewEscrowFinish("rBob", "rAlice", 123)
		if e.TxType() != TypeEscrowFinish {
			t.Errorf("expected TypeEscrowFinish, got %v", e.TxType())
		}
	})

	t.Run("EscrowCancel type", func(t *testing.T) {
		e := NewEscrowCancel("rBob", "rAlice", 123)
		if e.TxType() != TypeEscrowCancel {
			t.Errorf("expected TypeEscrowCancel, got %v", e.TxType())
		}
	})
}

// TestNewEscrowConstructors tests the constructor functions.
func TestNewEscrowConstructors(t *testing.T) {
	t.Run("NewEscrowCreate", func(t *testing.T) {
		e := NewEscrowCreate("rAlice", "rBob", NewXRPAmount("1000000"))
		if e.Account != "rAlice" {
			t.Errorf("expected Account=rAlice, got %v", e.Account)
		}
		if e.Destination != "rBob" {
			t.Errorf("expected Destination=rBob, got %v", e.Destination)
		}
		if e.Amount.Value != "1000000" {
			t.Errorf("expected Amount=1000000, got %v", e.Amount.Value)
		}
	})

	t.Run("NewEscrowFinish", func(t *testing.T) {
		e := NewEscrowFinish("rBob", "rAlice", 123)
		if e.Account != "rBob" {
			t.Errorf("expected Account=rBob, got %v", e.Account)
		}
		if e.Owner != "rAlice" {
			t.Errorf("expected Owner=rAlice, got %v", e.Owner)
		}
		if e.OfferSequence != 123 {
			t.Errorf("expected OfferSequence=123, got %v", e.OfferSequence)
		}
	})

	t.Run("NewEscrowCancel", func(t *testing.T) {
		e := NewEscrowCancel("rBob", "rAlice", 456)
		if e.Account != "rBob" {
			t.Errorf("expected Account=rBob, got %v", e.Account)
		}
		if e.Owner != "rAlice" {
			t.Errorf("expected Owner=rAlice, got %v", e.Owner)
		}
		if e.OfferSequence != 456 {
			t.Errorf("expected OfferSequence=456, got %v", e.OfferSequence)
		}
	})
}

// Helper function to create a pointer to uint32
func ptrUint32(v uint32) *uint32 {
	return &v
}
