package ed25519

import (
	"encoding/hex"
	"testing"
)

func TestED25519GenerateKeypair(t *testing.T) {
	provider := NewED25519Provider()
	seed := []byte("test seed for ed25519")

	privateKey, publicKey, err := provider.GenerateKeypair(seed, false)
	if err != nil {
		t.Fatalf("Failed to generate keypair: %v", err)
	}

	// Check if keys are valid hex strings
	if _, err := hex.DecodeString(privateKey); err != nil {
		t.Errorf("Invalid private key format: %v", err)
	}
	if _, err := hex.DecodeString(publicKey); err != nil {
		t.Errorf("Invalid public key format: %v", err)
	}

	// Check if keys have correct prefix
	if privateKey[:2] != "ED" {
		t.Errorf("Private key has wrong prefix. Got %s, want ED", privateKey[:2])
	}
	if publicKey[:2] != "ED" {
		t.Errorf("Public key has wrong prefix. Got %s, want ED", publicKey[:2])
	}
}

func TestED25519SignAndVerify(t *testing.T) {
	provider := NewED25519Provider()
	seed := []byte("test seed for ed25519")
	message := "test message"

	// Generate keypair
	privateKey, publicKey, err := provider.GenerateKeypair(seed, false)
	if err != nil {
		t.Fatalf("Failed to generate keypair: %v", err)
	}

	// Sign message
	signature, err := provider.SignMessage(message, privateKey)
	if err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	// Verify signature
	if !provider.VerifySignature(message, publicKey, signature) {
		t.Error("Signature verification failed")
	}

	// Verify with wrong message
	if provider.VerifySignature("wrong message", publicKey, signature) {
		t.Error("Verification should fail with wrong message")
	}
}

func TestED25519ValidatorNotSupported(t *testing.T) {
	provider := NewED25519Provider()
	seed := []byte("test seed")

	_, _, err := provider.GenerateKeypair(seed, true)
	if err != ErrValidatorNotSupported {
		t.Errorf("Expected validator not supported error, got %v", err)
	}
}
