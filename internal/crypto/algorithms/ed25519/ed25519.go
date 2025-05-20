package ed25519

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"github.com/LeJamon/goXRPLd/internal/crypto/common"
	"strings"
)

// ED25519SignatureProvider implements digital signature operations using the ED25519 algorithm
type ED25519SignatureProvider struct {
	keyPrefix byte // Prefix used to identify ED25519 keys in XRPL
}

// Common error definitions
var (
	ErrValidatorNotSupported = errors.New("validator keypairs cannot use Ed25519")
	ErrInvalidPrivateKey     = errors.New("invalid private key format")
	ErrInvalidSignature      = errors.New("invalid signature format")
)

func NewED25519Provider() *ED25519SignatureProvider {
	return &ED25519SignatureProvider{
		keyPrefix: 0xED,
	}
}

func (p *ED25519SignatureProvider) GenerateKeypair(seed []byte, isValidator bool) (string, string, error) {
	if isValidator {
		return "", "", ErrValidatorNotSupported
	}

	keyMaterial := crypto.Sha512Half(seed)
	pubKey, privKey, err := ed25519.GenerateKey(bytes.NewBuffer(keyMaterial[:]))
	if err != nil {
		return "", "", err
	}

	prefixedPubKey := append([]byte{p.keyPrefix}, pubKey...)
	prefixedPrivKey := append([]byte{p.keyPrefix}, privKey...)

	public := strings.ToUpper(hex.EncodeToString(prefixedPubKey))
	private := strings.ToUpper(hex.EncodeToString(prefixedPrivKey[:32+1]))

	return private, public, nil
}

func (p *ED25519SignatureProvider) SignMessage(message, privateKeyHex string) (string, error) {
	privKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return "", ErrInvalidPrivateKey
	}

	signingKey := ed25519.NewKeyFromSeed(privKeyBytes[1:])
	signature := ed25519.Sign(signingKey, []byte(message))

	return strings.ToUpper(hex.EncodeToString(signature)), nil
}

func (p *ED25519SignatureProvider) VerifySignature(message, publicKeyHex, signatureHex string) bool {
	pubKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return false
	}

	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}

	return ed25519.Verify(ed25519.PublicKey(pubKeyBytes[1:]), []byte(message), sigBytes)
}
