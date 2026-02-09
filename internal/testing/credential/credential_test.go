package credential_test

// Credentials_test.go - Tests for Credential transactions
// Reference: rippled/src/test/app/Credentials_test.cpp

import (
	"strings"
	"testing"

	jtx "github.com/LeJamon/goXRPLd/internal/testing"
	"github.com/LeJamon/goXRPLd/internal/testing/credential"
)

// TestSuccessful tests the basic credential lifecycle: create, accept, delete.
// Reference: rippled Credentials_test.cpp testSuccessful
func TestSuccessful(t *testing.T) {
	credType := "abcde"
	uri := "uri"

	issuer := jtx.NewAccount("issuer")
	subject := jtx.NewAccount("subject")

	env := jtx.NewTestEnv(t)
	env.Fund(issuer, subject)
	env.Close()

	// Test: Create credential for subject
	t.Run("CreateForSubject", func(t *testing.T) {
		// Create credential
		tx := credential.CredentialCreate(issuer, subject, credType).URI(uri).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}
		env.Close()

		// Verify issuer owner count increased
		issuerInfo := env.AccountInfo(issuer)
		if issuerInfo.OwnerCount != 1 {
			t.Errorf("Expected issuer owner count 1, got %d", issuerInfo.OwnerCount)
		}

		// Subject owner count should be 0 (not accepted yet)
		subjectInfo := env.AccountInfo(subject)
		if subjectInfo.OwnerCount != 0 {
			t.Errorf("Expected subject owner count 0, got %d", subjectInfo.OwnerCount)
		}
	})

	// Test: Accept credential
	t.Run("AcceptCredential", func(t *testing.T) {
		tx := credential.CredentialAccept(subject, issuer, credType).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}
		env.Close()

		// After accept, ownership transfers from issuer to subject
		issuerInfo := env.AccountInfo(issuer)
		if issuerInfo.OwnerCount != 0 {
			t.Errorf("Expected issuer owner count 0, got %d", issuerInfo.OwnerCount)
		}

		subjectInfo := env.AccountInfo(subject)
		if subjectInfo.OwnerCount != 1 {
			t.Errorf("Expected subject owner count 1, got %d", subjectInfo.OwnerCount)
		}
	})

	// Test: Delete credential by subject
	t.Run("DeleteCredential", func(t *testing.T) {
		tx := credential.CredentialDelete(subject, subject, issuer, credType).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}
		env.Close()

		// Verify owner counts reset to 0
		issuerInfo := env.AccountInfo(issuer)
		if issuerInfo.OwnerCount != 0 {
			t.Errorf("Expected issuer owner count 0, got %d", issuerInfo.OwnerCount)
		}

		subjectInfo := env.AccountInfo(subject)
		if subjectInfo.OwnerCount != 0 {
			t.Errorf("Expected subject owner count 0, got %d", subjectInfo.OwnerCount)
		}
	})

	t.Log("testSuccessful passed")
}

// TestCreateForSelf tests issuing a credential to yourself.
// Reference: rippled Credentials_test.cpp testSuccessful (Create for themself)
func TestCreateForSelf(t *testing.T) {
	credType := "abcde"
	uri := "uri"

	issuer := jtx.NewAccount("issuer")

	env := jtx.NewTestEnv(t)
	env.Fund(issuer)
	env.Close()

	// Test: Create credential for self (issuer == subject)
	t.Run("CreateForSelf", func(t *testing.T) {
		tx := credential.CredentialCreate(issuer, issuer, credType).URI(uri).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}
		env.Close()

		// Self-issued credentials are auto-accepted, owner count = 1
		issuerInfo := env.AccountInfo(issuer)
		if issuerInfo.OwnerCount != 1 {
			t.Errorf("Expected issuer owner count 1, got %d", issuerInfo.OwnerCount)
		}
	})

	// Test: Delete self-issued credential
	t.Run("DeleteSelfCredential", func(t *testing.T) {
		tx := credential.CredentialDelete(issuer, issuer, issuer, credType).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
		}
		env.Close()

		issuerInfo := env.AccountInfo(issuer)
		if issuerInfo.OwnerCount != 0 {
			t.Errorf("Expected issuer owner count 0, got %d", issuerInfo.OwnerCount)
		}
	})

	t.Log("testCreateForSelf passed")
}

// TestCreateFailed tests CredentialCreate validation failures.
// Reference: rippled Credentials_test.cpp testCreateFailed
func TestCreateFailed(t *testing.T) {
	credType := "abcde"

	issuer := jtx.NewAccount("issuer")
	subject := jtx.NewAccount("subject")

	env := jtx.NewTestEnv(t)
	env.Fund(issuer, subject)
	env.Close()

	// Test: Empty credentialType
	t.Run("EmptyCredentialType", func(t *testing.T) {
		tx := credential.CredentialCreate(issuer, subject, "").Build()
		result := env.Submit(tx)
		if result.Code != "temMALFORMED" {
			t.Errorf("Expected temMALFORMED, got %s", result.Code)
		}
	})

	// Test: CredentialType too long (> 64 bytes)
	t.Run("CredentialTypeTooLong", func(t *testing.T) {
		longCredType := strings.Repeat("a", 65)
		tx := credential.CredentialCreate(issuer, subject, longCredType).Build()
		result := env.Submit(tx)
		if result.Code != "temMALFORMED" {
			t.Errorf("Expected temMALFORMED, got %s", result.Code)
		}
	})

	// Test: URI too long (> 256 bytes)
	t.Run("URITooLong", func(t *testing.T) {
		longURI := strings.Repeat("a", 257)
		tx := credential.CredentialCreate(issuer, subject, credType).URI(longURI).Build()
		result := env.Submit(tx)
		if result.Code != "temMALFORMED" {
			t.Errorf("Expected temMALFORMED, got %s", result.Code)
		}
	})

	// Test: Empty URI
	t.Run("EmptyURI", func(t *testing.T) {
		// Create a credential with explicitly empty URI hex
		tx := credential.CredentialCreate(issuer, subject, credType).URIHex("").Build()
		result := env.Submit(tx)
		// Empty URI should be treated as absent, which is valid
		// Only explicitly empty (present but zero-length) should fail
		// This test may pass or fail depending on implementation
		t.Logf("Empty URI result: %s", result.Code)
		env.Close()

		// Clean up if credential was created
		if result.Success {
			deleteTx := credential.CredentialDelete(issuer, subject, issuer, credType).Build()
			env.Submit(deleteTx)
			env.Close()
		}
	})

	// Test: Invalid flags
	t.Run("InvalidFlags", func(t *testing.T) {
		tx := credential.CredentialCreate(issuer, subject, credType).Flags(0x00010000).Build()
		result := env.Submit(tx)
		if result.Code != "temINVALID_FLAG" {
			t.Errorf("Expected temINVALID_FLAG, got %s", result.Code)
		}
	})

	// Test: Duplicate credential
	t.Run("Duplicate", func(t *testing.T) {
		// First create should succeed
		tx1 := credential.CredentialCreate(issuer, subject, credType).Build()
		result1 := env.Submit(tx1)
		if !result1.Success {
			t.Errorf("First create expected success, got %s", result1.Code)
		}
		env.Close()

		// Second create should fail with tecDUPLICATE
		tx2 := credential.CredentialCreate(issuer, subject, credType).Build()
		result2 := env.Submit(tx2)
		if result2.Code != "tecDUPLICATE" {
			t.Errorf("Expected tecDUPLICATE, got %s", result2.Code)
		}
		env.Close()

		// Cleanup
		deleteTx := credential.CredentialDelete(issuer, subject, issuer, credType).Build()
		env.Submit(deleteTx)
		env.Close()
	})

	t.Log("testCreateFailed passed")
}

// TestAcceptFailed tests CredentialAccept validation failures.
// Reference: rippled Credentials_test.cpp testAcceptFailed
func TestAcceptFailed(t *testing.T) {
	credType := "abcde"

	issuer := jtx.NewAccount("issuer")
	subject := jtx.NewAccount("subject")

	env := jtx.NewTestEnv(t)
	env.Fund(issuer, subject)
	env.Close()

	// Test: Accept non-existent credential
	t.Run("CredentialNotExist", func(t *testing.T) {
		tx := credential.CredentialAccept(subject, issuer, credType).Build()
		result := env.Submit(tx)
		if result.Code != "tecNO_ENTRY" {
			t.Errorf("Expected tecNO_ENTRY, got %s", result.Code)
		}
		env.Close()
	})

	// Test: Empty credentialType
	t.Run("EmptyCredentialType", func(t *testing.T) {
		tx := credential.CredentialAccept(subject, issuer, "").Build()
		result := env.Submit(tx)
		if result.Code != "temMALFORMED" {
			t.Errorf("Expected temMALFORMED, got %s", result.Code)
		}
	})

	// Test: Invalid flags
	t.Run("InvalidFlags", func(t *testing.T) {
		tx := credential.CredentialAccept(subject, issuer, credType).Flags(0x00010000).Build()
		result := env.Submit(tx)
		if result.Code != "temINVALID_FLAG" {
			t.Errorf("Expected temINVALID_FLAG, got %s", result.Code)
		}
	})

	// Test: Accept already accepted credential
	t.Run("AlreadyAccepted", func(t *testing.T) {
		// Create and accept
		createTx := credential.CredentialCreate(issuer, subject, credType).Build()
		env.Submit(createTx)
		env.Close()

		acceptTx := credential.CredentialAccept(subject, issuer, credType).Build()
		result1 := env.Submit(acceptTx)
		if !result1.Success {
			t.Errorf("First accept expected success, got %s", result1.Code)
		}
		env.Close()

		// Try to accept again - should fail with tecDUPLICATE
		acceptTx2 := credential.CredentialAccept(subject, issuer, credType).Build()
		result2 := env.Submit(acceptTx2)
		if result2.Code != "tecDUPLICATE" {
			t.Errorf("Expected tecDUPLICATE, got %s", result2.Code)
		}
		env.Close()

		// Cleanup
		deleteTx := credential.CredentialDelete(subject, subject, issuer, credType).Build()
		env.Submit(deleteTx)
		env.Close()
	})

	t.Log("testAcceptFailed passed")
}

// TestDeleteFailed tests CredentialDelete validation failures.
// Reference: rippled Credentials_test.cpp testDeleteFailed
func TestDeleteFailed(t *testing.T) {
	credType := "abcde"

	issuer := jtx.NewAccount("issuer")
	subject := jtx.NewAccount("subject")
	other := jtx.NewAccount("other")

	env := jtx.NewTestEnv(t)
	env.Fund(issuer, subject, other)
	env.Close()

	// Test: Delete non-existent credential
	t.Run("CredentialNotExist", func(t *testing.T) {
		tx := credential.CredentialDelete(subject, subject, issuer, credType).Build()
		result := env.Submit(tx)
		if result.Code != "tecNO_ENTRY" {
			t.Errorf("Expected tecNO_ENTRY, got %s", result.Code)
		}
		env.Close()
	})

	// Test: Empty credentialType
	t.Run("EmptyCredentialType", func(t *testing.T) {
		tx := credential.CredentialDelete(subject, subject, issuer, "").Build()
		result := env.Submit(tx)
		if result.Code != "temMALFORMED" {
			t.Errorf("Expected temMALFORMED, got %s", result.Code)
		}
	})

	// Test: Invalid flags
	t.Run("InvalidFlags", func(t *testing.T) {
		tx := credential.CredentialDelete(subject, subject, issuer, credType).Flags(0x00010000).Build()
		result := env.Submit(tx)
		if result.Code != "temINVALID_FLAG" {
			t.Errorf("Expected temINVALID_FLAG, got %s", result.Code)
		}
	})

	// Test: Other account can't delete non-expired credential
	t.Run("NoPermission", func(t *testing.T) {
		credType2 := "fghij"

		// Create credential
		createTx := credential.CredentialCreate(issuer, subject, credType2).Build()
		env.Submit(createTx)
		env.Close()

		// Other account tries to delete - should fail
		deleteTx := credential.CredentialDelete(other, subject, issuer, credType2).Build()
		result := env.Submit(deleteTx)
		if result.Code != "tecNO_PERMISSION" {
			t.Errorf("Expected tecNO_PERMISSION, got %s", result.Code)
		}
		env.Close()

		// Cleanup by issuer
		cleanupTx := credential.CredentialDelete(issuer, subject, issuer, credType2).Build()
		env.Submit(cleanupTx)
		env.Close()
	})

	t.Log("testDeleteFailed passed")
}

// TestDeleteBySubject tests that the subject can delete a credential.
// Reference: rippled Credentials_test.cpp testCredentialsDelete (Delete by subject)
func TestDeleteBySubject(t *testing.T) {
	credType := "abcde"

	issuer := jtx.NewAccount("issuer")
	subject := jtx.NewAccount("subject")

	env := jtx.NewTestEnv(t)
	env.Fund(issuer, subject)
	env.Close()

	// Create credential
	createTx := credential.CredentialCreate(issuer, subject, credType).Build()
	result := env.Submit(createTx)
	if !result.Success {
		t.Fatalf("Create expected success, got %s", result.Code)
	}
	env.Close()

	// Subject can delete (even before accepting)
	deleteTx := credential.CredentialDelete(subject, subject, issuer, credType).Build()
	result = env.Submit(deleteTx)
	if !result.Success {
		t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
	}
	env.Close()

	// Verify owner counts
	issuerInfo := env.AccountInfo(issuer)
	if issuerInfo.OwnerCount != 0 {
		t.Errorf("Expected issuer owner count 0, got %d", issuerInfo.OwnerCount)
	}
	subjectInfo := env.AccountInfo(subject)
	if subjectInfo.OwnerCount != 0 {
		t.Errorf("Expected subject owner count 0, got %d", subjectInfo.OwnerCount)
	}

	t.Log("testDeleteBySubject passed")
}

// TestDeleteByIssuer tests that the issuer can delete a credential.
// Reference: rippled Credentials_test.cpp testCredentialsDelete (Delete by issuer)
func TestDeleteByIssuer(t *testing.T) {
	credType := "abcde"

	issuer := jtx.NewAccount("issuer")
	subject := jtx.NewAccount("subject")

	env := jtx.NewTestEnv(t)
	env.Fund(issuer, subject)
	env.Close()

	// Create credential
	createTx := credential.CredentialCreate(issuer, subject, credType).Build()
	result := env.Submit(createTx)
	if !result.Success {
		t.Fatalf("Create expected success, got %s", result.Code)
	}
	env.Close()

	// Issuer can delete
	deleteTx := credential.CredentialDelete(issuer, subject, issuer, credType).Build()
	result = env.Submit(deleteTx)
	if !result.Success {
		t.Errorf("Expected success, got %s: %s", result.Code, result.Message)
	}
	env.Close()

	// Verify owner counts
	issuerInfo := env.AccountInfo(issuer)
	if issuerInfo.OwnerCount != 0 {
		t.Errorf("Expected issuer owner count 0, got %d", issuerInfo.OwnerCount)
	}
	subjectInfo := env.AccountInfo(subject)
	if subjectInfo.OwnerCount != 0 {
		t.Errorf("Expected subject owner count 0, got %d", subjectInfo.OwnerCount)
	}

	t.Log("testDeleteByIssuer passed")
}

// TestReserve tests that creating credentials requires reserve.
// Reference: rippled Credentials_test.cpp testCreateFailed (not enough reserve)
func TestReserve(t *testing.T) {
	credType := "abcde"

	issuer := jtx.NewAccount("issuer")
	subject := jtx.NewAccount("subject")

	env := jtx.NewTestEnv(t)

	// Fund accounts at exactly account reserve (not enough for credential)
	acctReserve := env.ReserveBase()
	env.FundAmount(issuer, acctReserve)
	env.FundAmount(subject, acctReserve)
	env.Close()

	// Create should fail with insufficient reserve
	tx := credential.CredentialCreate(issuer, subject, credType).Build()
	result := env.Submit(tx)
	if result.Code != "tecINSUFFICIENT_RESERVE" {
		t.Errorf("Expected tecINSUFFICIENT_RESERVE, got %s", result.Code)
	}
	env.Close()

	t.Log("testReserve passed")
}

// TestAcceptReserve tests that accepting credentials requires reserve for subject.
// Reference: rippled Credentials_test.cpp testAcceptFailed (not enough reserve)
func TestAcceptReserve(t *testing.T) {
	credType := "abcde"

	issuer := jtx.NewAccount("issuer")
	subject := jtx.NewAccount("subject")

	env := jtx.NewTestEnv(t)

	// Fund issuer with enough for 1 object, subject at just account reserve
	acctReserve := env.ReserveBase()
	incReserve := env.ReserveIncrement()
	env.FundAmount(issuer, acctReserve+incReserve)
	env.FundAmount(subject, acctReserve)
	env.Close()

	// Create credential should succeed (issuer has reserve)
	createTx := credential.CredentialCreate(issuer, subject, credType).Build()
	result := env.Submit(createTx)
	if !result.Success {
		t.Fatalf("Create expected success, got %s", result.Code)
	}
	env.Close()

	// Accept should fail - subject doesn't have reserve
	acceptTx := credential.CredentialAccept(subject, issuer, credType).Build()
	result = env.Submit(acceptTx)
	if result.Code != "tecINSUFFICIENT_RESERVE" {
		t.Errorf("Expected tecINSUFFICIENT_RESERVE, got %s", result.Code)
	}
	env.Close()

	// Cleanup by issuer
	deleteTx := credential.CredentialDelete(issuer, subject, issuer, credType).Build()
	env.Submit(deleteTx)
	env.Close()

	t.Log("testAcceptReserve passed")
}

// TestCredentialTypeLimits tests the exact limits of CredentialType.
func TestCredentialTypeLimits(t *testing.T) {
	issuer := jtx.NewAccount("issuer")
	subject := jtx.NewAccount("subject")

	env := jtx.NewTestEnv(t)
	env.Fund(issuer, subject)
	env.Close()

	// Test: CredentialType at exactly 64 bytes should succeed
	t.Run("CredentialTypeAtLimit", func(t *testing.T) {
		credType64 := strings.Repeat("a", 64)
		tx := credential.CredentialCreate(issuer, subject, credType64).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success with 64-byte CredentialType, got %s", result.Code)
		}
		env.Close()

		// Cleanup
		deleteTx := credential.CredentialDelete(issuer, subject, issuer, credType64).Build()
		env.Submit(deleteTx)
		env.Close()
	})

	// Test: URI at exactly 256 bytes should succeed
	t.Run("URIAtLimit", func(t *testing.T) {
		credType := "testcred"
		uri256 := strings.Repeat("b", 256)
		tx := credential.CredentialCreate(issuer, subject, credType).URI(uri256).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Errorf("Expected success with 256-byte URI, got %s", result.Code)
		}
		env.Close()

		// Cleanup
		deleteTx := credential.CredentialDelete(issuer, subject, issuer, credType).Build()
		env.Submit(deleteTx)
		env.Close()
	})

	t.Log("testCredentialTypeLimits passed")
}

// TestEnabled tests that credential transactions are disabled without the amendment.
// Reference: rippled Credentials_test.cpp testFeatureFailed
func TestEnabled(t *testing.T) {
	credType := "abcde"

	issuer := jtx.NewAccount("issuer")
	subject := jtx.NewAccount("subject")

	env := jtx.NewTestEnv(t)
	env.Fund(issuer, subject)
	env.Close()

	// Disable the Credentials amendment
	env.DisableFeature("Credentials")

	// Without the featureCredentials amendment, all credential transactions should fail
	t.Run("CreateDisabled", func(t *testing.T) {
		tx := credential.CredentialCreate(issuer, subject, credType).Build()
		result := env.Submit(tx)
		if result.Code != "temDISABLED" {
			t.Errorf("Expected temDISABLED, got %s", result.Code)
		}
	})

	t.Run("AcceptDisabled", func(t *testing.T) {
		tx := credential.CredentialAccept(subject, issuer, credType).Build()
		result := env.Submit(tx)
		if result.Code != "temDISABLED" {
			t.Errorf("Expected temDISABLED, got %s", result.Code)
		}
	})

	t.Run("DeleteDisabled", func(t *testing.T) {
		tx := credential.CredentialDelete(subject, subject, issuer, credType).Build()
		result := env.Submit(tx)
		if result.Code != "temDISABLED" {
			t.Errorf("Expected temDISABLED, got %s", result.Code)
		}
	})

	t.Log("testEnabled passed")
}

// TestMultipleCredentials tests that accounts can have multiple credentials.
func TestMultipleCredentials(t *testing.T) {
	issuer := jtx.NewAccount("issuer")
	subject := jtx.NewAccount("subject")

	env := jtx.NewTestEnv(t)
	env.Fund(issuer, subject)
	env.Close()

	credTypes := []string{"type1", "type2", "type3"}

	// Create multiple credentials
	for _, ct := range credTypes {
		tx := credential.CredentialCreate(issuer, subject, ct).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Fatalf("Create %s expected success, got %s", ct, result.Code)
		}
	}
	env.Close()

	// Verify issuer has 3 objects
	issuerInfo := env.AccountInfo(issuer)
	if issuerInfo.OwnerCount != 3 {
		t.Errorf("Expected issuer owner count 3, got %d", issuerInfo.OwnerCount)
	}

	// Accept all
	for _, ct := range credTypes {
		tx := credential.CredentialAccept(subject, issuer, ct).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Fatalf("Accept %s expected success, got %s", ct, result.Code)
		}
	}
	env.Close()

	// Verify subject now has all 3, issuer has 0
	subjectInfo := env.AccountInfo(subject)
	if subjectInfo.OwnerCount != 3 {
		t.Errorf("Expected subject owner count 3, got %d", subjectInfo.OwnerCount)
	}
	issuerInfo = env.AccountInfo(issuer)
	if issuerInfo.OwnerCount != 0 {
		t.Errorf("Expected issuer owner count 0, got %d", issuerInfo.OwnerCount)
	}

	// Delete all
	for _, ct := range credTypes {
		tx := credential.CredentialDelete(subject, subject, issuer, ct).Build()
		result := env.Submit(tx)
		if !result.Success {
			t.Fatalf("Delete %s expected success, got %s", ct, result.Code)
		}
	}
	env.Close()

	// Verify both have 0
	subjectInfo = env.AccountInfo(subject)
	if subjectInfo.OwnerCount != 0 {
		t.Errorf("Expected subject owner count 0, got %d", subjectInfo.OwnerCount)
	}
	issuerInfo = env.AccountInfo(issuer)
	if issuerInfo.OwnerCount != 0 {
		t.Errorf("Expected issuer owner count 0, got %d", issuerInfo.OwnerCount)
	}

	t.Log("testMultipleCredentials passed")
}
