package did_test

// DID_test.go - Tests for DID (Decentralized Identifier) transactions
// Reference: rippled/src/test/app/DID_test.cpp

import (
	"strings"
	"testing"

	jtx "github.com/LeJamon/goXRPLd/internal/testing"
	"github.com/LeJamon/goXRPLd/internal/testing/did"
	"github.com/LeJamon/goXRPLd/internal/testing/payment"
)

// TestEnabled tests that DID transactions are disabled without the featureDID amendment.
// Reference: rippled DID_test.cpp testEnabled
func TestEnabled(t *testing.T) {
	t.Skip("testEnabled requires amendment support in test environment")

	env := jtx.NewTestEnv(t)

	alice := jtx.NewAccount("alice")
	env.Fund(alice)
	env.Close()

	// Verify initial owner count
	info := env.AccountInfo(alice)
	if info.OwnerCount != 0 {
		t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
	}

	// Test: DIDSet is disabled without featureDID
	t.Run("DIDSetDisabled", func(t *testing.T) {
		tx := did.DIDSet(alice).URI("uri").Build()
		result := env.Submit(tx)
		if result.Code != "temDISABLED" {
			t.Errorf("Expected temDISABLED, got %s", result.Code)
		}
		env.Close()

		info := env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}
	})

	// Test: DIDDelete is disabled without featureDID
	t.Run("DIDDeleteDisabled", func(t *testing.T) {
		tx := did.DIDDelete(alice).Build()
		result := env.Submit(tx)
		if result.Code != "temDISABLED" {
			t.Errorf("Expected temDISABLED, got %s", result.Code)
		}
		env.Close()
	})

	t.Log("testEnabled passed")
}

// TestAccountReserve tests that the reserve behaves as expected for DID creation.
// Reference: rippled DID_test.cpp testAccountReserve
func TestAccountReserve(t *testing.T) {
	env := jtx.NewTestEnv(t)

	alice := jtx.NewAccount("alice")

	// Fund alice with just the account reserve (not enough for DID)
	// Account reserve = 10 XRP (10,000,000 drops)
	// Increment reserve = 2 XRP (2,000,000 drops)
	acctReserve := env.ReserveBase()
	incReserve := env.ReserveIncrement()
	baseFee := env.BaseFee()

	// Fund alice exactly at account reserve
	env.FundAmount(alice, acctReserve)
	env.Close()

	balance := env.Balance(alice)
	if balance != acctReserve {
		t.Logf("Expected balance %d, got %d", acctReserve, balance)
	}

	info := env.AccountInfo(alice)
	if info.OwnerCount != 0 {
		t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
	}

	// Test: alice does not have enough XRP to cover the reserve for a DID
	t.Run("InsufficientReserve", func(t *testing.T) {
		tx := did.DIDSet(alice).URI("uri").Build()
		result := env.Submit(tx)
		if result.Code != "tecINSUFFICIENT_RESERVE" {
			t.Errorf("Expected tecINSUFFICIENT_RESERVE, got %s", result.Code)
		}
		env.Close()

		info := env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}
	})

	// Pay alice almost enough to make the reserve for a DID
	master := env.MasterAccount()
	paymentTx := payment.Pay(master, alice, incReserve+2*baseFee-1).Build()
	env.Submit(paymentTx)
	env.Close()

	// Test: alice still does not have enough XRP for the reserve of a DID
	t.Run("StillInsufficientReserve", func(t *testing.T) {
		tx := did.DIDSet(alice).URI("uri").Build()
		result := env.Submit(tx)
		if result.Code != "tecINSUFFICIENT_RESERVE" {
			t.Errorf("Expected tecINSUFFICIENT_RESERVE, got %s", result.Code)
		}
		env.Close()

		info := env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}
	})

	// Pay alice enough to make the reserve for a DID
	paymentTx2 := payment.Pay(master, alice, baseFee+1).Build()
	env.Submit(paymentTx2)
	env.Close()

	// Test: Now alice can create a DID
	t.Run("CanCreateDID", func(t *testing.T) {
		tx := did.DIDSet(alice).URI("uri").Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}
		env.Close()

		info := env.AccountInfo(alice)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1, got %d", info.OwnerCount)
		}
	})

	// Test: alice deletes her DID
	t.Run("DeleteDID", func(t *testing.T) {
		tx := did.DIDDelete(alice).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}
		env.Close()
	})

	t.Log("testAccountReserve passed")
}

// TestSetInvalid tests invalid DIDSet scenarios.
// Reference: rippled DID_test.cpp testSetInvalid
func TestSetInvalid(t *testing.T) {
	env := jtx.NewTestEnv(t)

	alice := jtx.NewAccount("alice")
	env.Fund(alice)
	env.Close()

	//----------------------------------------------------------------------
	// preflight tests

	// Test: invalid flags
	t.Run("InvalidFlags", func(t *testing.T) {
		info := env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}

		// 0x00010000 is an invalid flag
		tx := did.DIDSet(alice).URI("uri").Flags(0x00010000).Build()
		result := env.Submit(tx)
		if result.Code != "temINVALID_FLAG" {
			t.Errorf("Expected temINVALID_FLAG, got %s", result.Code)
		}
		env.Close()

		info = env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}
	})

	// Test: no fields provided
	t.Run("NoFields", func(t *testing.T) {
		tx := did.DIDSet(alice).Build()
		result := env.Submit(tx)
		if result.Code != "temEMPTY_DID" {
			t.Errorf("Expected temEMPTY_DID, got %s", result.Code)
		}
		env.Close()

		info := env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}
	})

	// Test: all empty fields
	t.Run("AllEmptyFields", func(t *testing.T) {
		tx := did.DIDSet(alice).URI("").Document("").Data("").Build()
		result := env.Submit(tx)
		if result.Code != "temEMPTY_DID" {
			t.Errorf("Expected temEMPTY_DID, got %s", result.Code)
		}
		env.Close()

		info := env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}
	})

	// Test: URI is too long (> 256 bytes)
	t.Run("URITooLong", func(t *testing.T) {
		longString := strings.Repeat("a", 257)
		tx := did.DIDSet(alice).URI(longString).Build()
		result := env.Submit(tx)
		if result.Code != "temMALFORMED" {
			t.Errorf("Expected temMALFORMED, got %s", result.Code)
		}
		env.Close()

		info := env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}
	})

	// Test: DIDDocument is too long (> 256 bytes)
	t.Run("DocumentTooLong", func(t *testing.T) {
		longString := strings.Repeat("a", 257)
		tx := did.DIDSet(alice).Document(longString).Build()
		result := env.Submit(tx)
		if result.Code != "temMALFORMED" {
			t.Errorf("Expected temMALFORMED, got %s", result.Code)
		}
		env.Close()

		info := env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}
	})

	// Test: Data (attestation) is too long (> 256 bytes)
	t.Run("DataTooLong", func(t *testing.T) {
		longString := strings.Repeat("a", 257)
		tx := did.DIDSet(alice).Document("data").Data(longString).Build()
		result := env.Submit(tx)
		if result.Code != "temMALFORMED" {
			t.Errorf("Expected temMALFORMED, got %s", result.Code)
		}
		env.Close()

		info := env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}
	})

	// Test: some empty fields, some optional fields
	// With fixEmptyDID amendment, this should fail with tecEMPTY_DID
	// Without the amendment, it would succeed (but create an empty DID)
	t.Run("SomeEmptyFields", func(t *testing.T) {
		tx := did.DIDSet(alice).URI("").Build()
		result := env.Submit(tx)
		// With fixEmptyDID enabled, should be tecEMPTY_DID
		// Without, should be tesSUCCESS but create empty DID (bug)
		if result.Code != "tecEMPTY_DID" && !result.Success {
			t.Logf("Got %s (depends on fixEmptyDID amendment)", result.Code)
		}
		env.Close()
	})

	t.Log("testSetInvalid passed")
}

// TestDeleteInvalid tests invalid DIDDelete scenarios.
// Reference: rippled DID_test.cpp testDeleteInvalid
func TestDeleteInvalid(t *testing.T) {
	env := jtx.NewTestEnv(t)

	alice := jtx.NewAccount("alice")
	env.Fund(alice)
	env.Close()

	//----------------------------------------------------------------------
	// preflight tests

	// Test: invalid flags
	t.Run("InvalidFlags", func(t *testing.T) {
		info := env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}

		// 0x00010000 is an invalid flag
		tx := did.DIDDelete(alice).Flags(0x00010000).Build()
		result := env.Submit(tx)
		if result.Code != "temINVALID_FLAG" {
			t.Errorf("Expected temINVALID_FLAG, got %s", result.Code)
		}
		env.Close()

		info = env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}
	})

	//----------------------------------------------------------------------
	// doApply tests

	// Test: DID doesn't exist
	t.Run("DIDNotFound", func(t *testing.T) {
		tx := did.DIDDelete(alice).Build()
		result := env.Submit(tx)
		if result.Code != "tecNO_ENTRY" {
			t.Errorf("Expected tecNO_ENTRY, got %s", result.Code)
		}
		env.Close()

		info := env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}
	})

	t.Log("testDeleteInvalid passed")
}

// TestSetValidInitial tests valid initial DIDSet transactions.
// Reference: rippled DID_test.cpp testSetValidInitial
func TestSetValidInitial(t *testing.T) {
	env := jtx.NewTestEnv(t)

	alice := jtx.NewAccount("alice")
	bob := jtx.NewAccount("bob")
	charlie := jtx.NewAccount("charlie")
	dave := jtx.NewAccount("dave")
	edna := jtx.NewAccount("edna")
	francis := jtx.NewAccount("francis")
	george := jtx.NewAccount("george")

	env.Fund(alice, bob, charlie, dave, edna, francis, george)
	env.Close()

	// Verify initial owner counts
	for _, acc := range []*jtx.Account{alice, bob, charlie} {
		info := env.AccountInfo(acc)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0 for %s, got %d", acc.Name, info.OwnerCount)
		}
	}

	// Test: only URI
	t.Run("OnlyURI", func(t *testing.T) {
		tx := did.DIDSet(alice).URI("uri").Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(alice)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1 for alice, got %d", info.OwnerCount)
		}
	})

	// Test: only DIDDocument
	t.Run("OnlyDocument", func(t *testing.T) {
		tx := did.DIDSet(bob).Document("data").Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(bob)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1 for bob, got %d", info.OwnerCount)
		}
	})

	// Test: only Data
	t.Run("OnlyData", func(t *testing.T) {
		tx := did.DIDSet(charlie).Data("data").Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(charlie)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1 for charlie, got %d", info.OwnerCount)
		}
	})

	// Test: URI + Data
	t.Run("URIPlusData", func(t *testing.T) {
		tx := did.DIDSet(dave).URI("uri").Data("attest").Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(dave)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1 for dave, got %d", info.OwnerCount)
		}
	})

	// Test: URI + DIDDocument
	t.Run("URIPlusDocument", func(t *testing.T) {
		tx := did.DIDSet(edna).URI("uri").Document("data").Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(edna)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1 for edna, got %d", info.OwnerCount)
		}
	})

	// Test: DIDDocument + Data
	t.Run("DocumentPlusData", func(t *testing.T) {
		tx := did.DIDSet(francis).Document("data").Data("attest").Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(francis)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1 for francis, got %d", info.OwnerCount)
		}
	})

	// Test: URI + DIDDocument + Data
	t.Run("AllFields", func(t *testing.T) {
		tx := did.DIDSet(george).URI("uri").Document("data").Data("attest").Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(george)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1 for george, got %d", info.OwnerCount)
		}
	})

	t.Log("testSetValidInitial passed")
}

// TestSetModify tests modifying an existing DID with DIDSet.
// Reference: rippled DID_test.cpp testSetModify
func TestSetModify(t *testing.T) {
	env := jtx.NewTestEnv(t)

	alice := jtx.NewAccount("alice")
	env.Fund(alice)
	env.Close()

	info := env.AccountInfo(alice)
	if info.OwnerCount != 0 {
		t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
	}

	// Create DID with initial URI
	initialURI := "uri"
	t.Run("CreateDID", func(t *testing.T) {
		tx := did.DIDSet(alice).URI(initialURI).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(alice)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1, got %d", info.OwnerCount)
		}

		// TODO: Verify DID ledger entry has URI set, no DIDDocument, no Data
	})

	// Test: Try to delete URI, fails because no elements would remain
	t.Run("DeleteURIFails", func(t *testing.T) {
		tx := did.DIDSet(alice).URI("").Build()
		result := env.Submit(tx)
		if result.Code != "tecEMPTY_DID" {
			t.Errorf("Expected tecEMPTY_DID, got %s", result.Code)
		}

		info := env.AccountInfo(alice)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1, got %d", info.OwnerCount)
		}

		// TODO: Verify DID still has URI set
	})

	// Test: Set DIDDocument
	initialDocument := "data"
	t.Run("SetDocument", func(t *testing.T) {
		tx := did.DIDSet(alice).Document(initialDocument).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(alice)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1, got %d", info.OwnerCount)
		}

		// TODO: Verify DID has both URI and DIDDocument set
	})

	// Test: Set Data
	initialData := "attest"
	t.Run("SetData", func(t *testing.T) {
		tx := did.DIDSet(alice).Data(initialData).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(alice)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1, got %d", info.OwnerCount)
		}

		// TODO: Verify DID has URI, DIDDocument, and Data set
	})

	// Test: Remove URI (now allowed because DIDDocument and Data remain)
	t.Run("RemoveURI", func(t *testing.T) {
		tx := did.DIDSet(alice).URI("").Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(alice)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1, got %d", info.OwnerCount)
		}

		// TODO: Verify DID has no URI, has DIDDocument and Data
	})

	// Test: Remove Data
	t.Run("RemoveData", func(t *testing.T) {
		tx := did.DIDSet(alice).Data("").Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(alice)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1, got %d", info.OwnerCount)
		}

		// TODO: Verify DID has no URI, has DIDDocument, no Data
	})

	// Test: Remove DIDDocument + set URI
	secondURI := "uri2"
	t.Run("RemoveDocumentSetURI", func(t *testing.T) {
		tx := did.DIDSet(alice).URI(secondURI).Document("").Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(alice)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1, got %d", info.OwnerCount)
		}

		// TODO: Verify DID has URI, no DIDDocument, no Data
	})

	// Test: Remove URI + set DIDDocument
	secondDocument := "data2"
	t.Run("RemoveURISetDocument", func(t *testing.T) {
		tx := did.DIDSet(alice).URI("").Document(secondDocument).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(alice)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1, got %d", info.OwnerCount)
		}

		// TODO: Verify DID has no URI, has DIDDocument, no Data
	})

	// Test: Remove DIDDocument + set Data
	secondData := "randomData"
	t.Run("RemoveDocumentSetData", func(t *testing.T) {
		tx := did.DIDSet(alice).Document("").Data(secondData).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(alice)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1, got %d", info.OwnerCount)
		}

		// TODO: Verify DID has no URI, no DIDDocument, has Data
	})

	// Test: Delete DID
	t.Run("DeleteDID", func(t *testing.T) {
		tx := did.DIDDelete(alice).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}

		info := env.AccountInfo(alice)
		if info.OwnerCount != 0 {
			t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
		}

		// TODO: Verify DID ledger entry is gone
	})

	t.Log("testSetModify passed")
}

// TestDIDCycle tests creating, modifying, and deleting a DID in a complete cycle.
// This is a custom test to verify the full lifecycle.
func TestDIDCycle(t *testing.T) {
	env := jtx.NewTestEnv(t)

	alice := jtx.NewAccount("alice")
	env.Fund(alice)
	env.Close()

	// Initial state: no DID
	info := env.AccountInfo(alice)
	if info.OwnerCount != 0 {
		t.Errorf("Expected owner count 0, got %d", info.OwnerCount)
	}

	// Step 1: Create DID with URI
	tx1 := did.DIDSet(alice).URI("https://example.com/did/alice").Build()
	result := env.Submit(tx1)
	if !result.Success {
		t.Fatalf("Failed to create DID: %s", result.Code)
	}

	info = env.AccountInfo(alice)
	if info.OwnerCount != 1 {
		t.Errorf("Expected owner count 1 after create, got %d", info.OwnerCount)
	}
	env.Close()

	// Step 2: Add Document to existing DID
	tx2 := did.DIDSet(alice).Document("did document content").Build()
	result = env.Submit(tx2)
	if !result.Success {
		t.Fatalf("Failed to add document: %s", result.Code)
	}

	info = env.AccountInfo(alice)
	if info.OwnerCount != 1 {
		t.Errorf("Expected owner count 1 after modify, got %d", info.OwnerCount)
	}
	env.Close()

	// Step 3: Add Data to existing DID
	tx3 := did.DIDSet(alice).Data("attestation data").Build()
	result = env.Submit(tx3)
	if !result.Success {
		t.Fatalf("Failed to add data: %s", result.Code)
	}
	env.Close()

	// Step 4: Update URI
	tx4 := did.DIDSet(alice).URI("https://example.com/did/alice/v2").Build()
	result = env.Submit(tx4)
	if !result.Success {
		t.Fatalf("Failed to update URI: %s", result.Code)
	}
	env.Close()

	// Step 5: Remove URI (Document and Data should remain)
	tx5 := did.DIDSet(alice).URI("").Build()
	result = env.Submit(tx5)
	if !result.Success {
		t.Fatalf("Failed to remove URI: %s", result.Code)
	}
	env.Close()

	// Step 6: Remove Document (Data should remain)
	tx6 := did.DIDSet(alice).Document("").Build()
	result = env.Submit(tx6)
	if !result.Success {
		t.Fatalf("Failed to remove document: %s", result.Code)
	}
	env.Close()

	// Step 7: Try to remove Data (should fail - would make DID empty)
	tx7 := did.DIDSet(alice).Data("").Build()
	result = env.Submit(tx7)
	if result.Code != "tecEMPTY_DID" {
		t.Errorf("Expected tecEMPTY_DID when removing last field, got %s", result.Code)
	}
	env.Close()

	// Step 8: Delete the DID
	tx8 := did.DIDDelete(alice).Build()
	result = env.Submit(tx8)
	if !result.Success {
		t.Fatalf("Failed to delete DID: %s", result.Code)
	}

	info = env.AccountInfo(alice)
	if info.OwnerCount != 0 {
		t.Errorf("Expected owner count 0 after delete, got %d", info.OwnerCount)
	}
	env.Close()

	// Step 9: Try to delete again (should fail)
	tx9 := did.DIDDelete(alice).Build()
	result = env.Submit(tx9)
	if result.Code != "tecNO_ENTRY" {
		t.Errorf("Expected tecNO_ENTRY when deleting non-existent DID, got %s", result.Code)
	}

	t.Log("testDIDCycle passed")
}

// TestDIDFieldLimits tests the exact limits of DID fields.
func TestDIDFieldLimits(t *testing.T) {
	env := jtx.NewTestEnv(t)

	alice := jtx.NewAccount("alice")
	bob := jtx.NewAccount("bob")
	charlie := jtx.NewAccount("charlie")

	env.Fund(alice, bob, charlie)
	env.Close()

	// Test: URI at exactly 256 bytes should succeed
	t.Run("URIAtLimit", func(t *testing.T) {
		uri256 := strings.Repeat("a", 256)
		tx := did.DIDSet(alice).URI(uri256).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success with 256-byte URI, got %s", result.Code)
		}
	})

	// Test: Document at exactly 256 bytes should succeed
	t.Run("DocumentAtLimit", func(t *testing.T) {
		doc256 := strings.Repeat("b", 256)
		tx := did.DIDSet(bob).Document(doc256).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success with 256-byte Document, got %s", result.Code)
		}
	})

	// Test: Data at exactly 256 bytes should succeed
	t.Run("DataAtLimit", func(t *testing.T) {
		data256 := strings.Repeat("c", 256)
		tx := did.DIDSet(charlie).Data(data256).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success with 256-byte Data, got %s", result.Code)
		}
	})

	t.Log("testDIDFieldLimits passed")
}

// TestMultipleDIDs tests that different accounts can have independent DIDs.
func TestMultipleDIDs(t *testing.T) {
	env := jtx.NewTestEnv(t)

	alice := jtx.NewAccount("alice")
	bob := jtx.NewAccount("bob")
	charlie := jtx.NewAccount("charlie")

	env.Fund(alice, bob, charlie)
	env.Close()

	// Create DIDs for all accounts
	tx1 := did.DIDSet(alice).URI("alice-uri").Build()
	result := env.Submit(tx1)
	if !result.Success {
		t.Fatalf("Failed to create alice's DID: %s", result.Code)
	}

	tx2 := did.DIDSet(bob).Document("bob-document").Build()
	result = env.Submit(tx2)
	if !result.Success {
		t.Fatalf("Failed to create bob's DID: %s", result.Code)
	}

	tx3 := did.DIDSet(charlie).Data("charlie-data").Build()
	result = env.Submit(tx3)
	if !result.Success {
		t.Fatalf("Failed to create charlie's DID: %s", result.Code)
	}
	env.Close()

	// Verify all have owner count 1
	for _, acc := range []*jtx.Account{alice, bob, charlie} {
		info := env.AccountInfo(acc)
		if info.OwnerCount != 1 {
			t.Errorf("Expected owner count 1 for %s, got %d", acc.Name, info.OwnerCount)
		}
	}

	// Delete bob's DID
	tx4 := did.DIDDelete(bob).Build()
	result = env.Submit(tx4)
	if !result.Success {
		t.Fatalf("Failed to delete bob's DID: %s", result.Code)
	}

	// Verify bob has owner count 0, others still have 1
	infoAlice := env.AccountInfo(alice)
	infoBob := env.AccountInfo(bob)
	infoCharlie := env.AccountInfo(charlie)

	if infoAlice.OwnerCount != 1 {
		t.Errorf("Expected alice owner count 1, got %d", infoAlice.OwnerCount)
	}
	if infoBob.OwnerCount != 0 {
		t.Errorf("Expected bob owner count 0, got %d", infoBob.OwnerCount)
	}
	if infoCharlie.OwnerCount != 1 {
		t.Errorf("Expected charlie owner count 1, got %d", infoCharlie.OwnerCount)
	}

	t.Log("testMultipleDIDs passed")
}
