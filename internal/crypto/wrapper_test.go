package crypto_test

import (
	"github.com/LeJamon/goXRPLd/internal/crypto"
	"github.com/LeJamon/goXRPLd/internal/crypto/algorithms/ed25519"
	"github.com/LeJamon/goXRPLd/internal/crypto/algorithms/secp256k1"
	"testing"
)

type MockSignatureProvider struct {
	generateKeypairCalled bool
	signMessageCalled     bool
	verifySignatureCalled bool
}

func (m *MockSignatureProvider) GenerateKeypair(seed []byte, isValidator bool) (string, string, error) {
	m.generateKeypairCalled = true
	return "private", "public", nil
}

func (m *MockSignatureProvider) SignMessage(message, privateKeyHex string) (string, error) {
	m.signMessageCalled = true
	return "signature", nil
}

func (m *MockSignatureProvider) VerifySignature(message, publicKeyHex, signatureHex string) bool {
	m.verifySignatureCalled = true
	return true
}

func TestWrapperWithBothProviders(t *testing.T) {
	seed := []byte("test seed for wrapper")
	message := "test message"

	// Test with ED25519
	ed25519Wrapper := crypto.NewED25519Wrapper(ed25519.NewED25519Provider())
	if ed25519Wrapper.GetCryptoType() != crypto.ED25519 {
		t.Error("Wrong crypto type for ED25519 wrapper")
	}

	edPrivate, edPublic, err := ed25519Wrapper.GenerateKeypair(seed, false)
	if err != nil {
		t.Fatalf("ED25519 keypair generation failed: %v", err)
	}

	edSignature, err := ed25519Wrapper.SignMessage(message, edPrivate)
	if err != nil {
		t.Fatalf("ED25519 signing failed: %v", err)
	}

	if !ed25519Wrapper.VerifySignature(message, edPublic, edSignature) {
		t.Error("ED25519 signature verification failed")
	}

	// Test with SECP256K1
	secp256k1Wrapper := crypto.NewSECP256K1Wrapper(secp256k1.NewSECP256K1Provider())
	if secp256k1Wrapper.GetCryptoType() != crypto.SECP256K1 {
		t.Error("Wrong crypto type for SECP256K1 wrapper")
	}

	secpPrivate, secpPublic, err := secp256k1Wrapper.GenerateKeypair(seed, false)
	if err != nil {
		t.Fatalf("SECP256K1 keypair generation failed: %v", err)
	}

	secpSignature, err := secp256k1Wrapper.SignMessage(message, secpPrivate)
	if err != nil {
		t.Fatalf("SECP256K1 signing failed: %v", err)
	}

	if !secp256k1Wrapper.VerifySignature(message, secpPublic, secpSignature) {
		t.Error("SECP256K1 signature verification failed")
	}
}
