package secp256k1

import (
	"encoding/hex"
	"fmt"
	"testing"
)

var testPrivHex = "289c2857d4598e37fb9647507e47a309d6133539bf21a8b9cb6df88fd5232032"

func TestSECP256K1GenerateKeypair(t *testing.T) {
	provider := NewSECP256K1Provider()
	seed := []byte("test seed for secp256k1")

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
	if privateKey[:2] != "00" {
		t.Errorf("Private key has wrong prefix. Got %s, want 00", privateKey[:2])
	}
	if publicKey[:2] != "00" {
		t.Errorf("Public key has wrong prefix. Got %s, want 00", publicKey[:2])
	}
}

func TestSECP256K1SignAndVerify(t *testing.T) {
	provider := NewSECP256K1Provider()
	seed := []byte("test seed for secp256k1")
	message := "test message"

	// Generate keypair
	privateKey, publicKey, err := provider.GenerateKeypair(seed, false)
	if err != nil {
		t.Fatalf("Failed to generate keypair: %v", err)
	}

	fmt.Printf("Generated Private Key: %s\n", privateKey)
	fmt.Printf("Generated Public Key: %s\n", publicKey)

	// Sign message
	signature, err := provider.SignMessage(message, privateKey)
	if err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}
	fmt.Printf("Message: %s\n", message)
	fmt.Printf("Signature: %s\n", signature)

	// Verify signature
	isValid := provider.VerifySignature(message, publicKey, signature)
	fmt.Printf("Signature verification result: %v\n", isValid)

	// Test with wrong message
	wrongIsValid := provider.VerifySignature("wrong message", publicKey, signature)
	fmt.Printf("Wrong message verification result (should be false): %v\n", wrongIsValid)
	if wrongIsValid {
		t.Error("Verification should fail with wrong message")
	}
}
