package secp256k1

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	internalCrypto "github.com/LeJamon/goXRPLd/internal/crypto/common"
	gethCrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"strings"
)

const (
	SECP256K1_PREFIX_HEX = 0x00
)

type SECP256K1SignatureProvider struct {
	keyPrefix byte
}

func NewSECP256K1Provider() *SECP256K1SignatureProvider {
	return &SECP256K1SignatureProvider{
		keyPrefix: SECP256K1_PREFIX_HEX,
	}
}

// Common error definitions
var (
	ErrValidatorNotSupported = errors.New("validator keypairs cannot use Ed25519")
	ErrInvalidPrivateKey     = errors.New("invalid private key format")
	ErrInvalidSignature      = errors.New("invalid signature format")
)

func (p *SECP256K1SignatureProvider) GenerateKeypair(seed []byte, isValidator bool) (string, string, error) {
	privateKey, err := gethCrypto.GenerateKey()
	privateKeyBytes := gethCrypto.FromECDSA(privateKey)

	if err != nil {
		return "", "", err
	}
	publicKey := privateKey.Public()
	publicKeyECDSA, _ := publicKey.(*ecdsa.PublicKey)

	compressedPubKey := gethCrypto.CompressPubkey(publicKeyECDSA)
	fmt.Println("COMPRESSED PUBKEY: ", compressedPubKey)
	prefixedPubKey := append([]byte{p.keyPrefix}, compressedPubKey...)
	prefixedPrivKey := append([]byte{p.keyPrefix}, privateKeyBytes...)

	public := strings.ToUpper(hex.EncodeToString(prefixedPubKey))
	fmt.Println("PUBLIC HEX: ", public)
	private := strings.ToUpper(hex.EncodeToString(prefixedPrivKey))

	return private, public, nil
}

func (p *SECP256K1SignatureProvider) SignMessage(message, privateKeyHex string) (string, error) {
	var privKeyBytes []byte
	var err error

	if len(privateKeyHex) == 66 && strings.HasPrefix(privateKeyHex, "00") {
		privKeyBytes, err = hex.DecodeString(privateKeyHex[2:])
	} else if len(privateKeyHex) == 64 {
		privKeyBytes, err = hex.DecodeString(privateKeyHex)
	} else {
		return "", ErrInvalidPrivateKey
	}

	if err != nil {
		return "", fmt.Errorf("failed to decode private key: %w", err)
	}

	messageHash := internalCrypto.Sha512Half([]byte(message))
	signature, err := secp256k1.Sign(messageHash, privKeyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to sign message: %w", err)
	}
	signature = signature[:64]

	return strings.ToUpper(hex.EncodeToString(signature)), nil
}

func (p *SECP256K1SignatureProvider) VerifySignature(message, publicKeyHex, signatureHex string) bool {
	pubKeyBytes, err := hex.DecodeString(publicKeyHex[2:])
	if err != nil {
		return false
	}

	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}
	if len(sigBytes) > 64 {
		sigBytes = sigBytes[:64]
	}
	messageHash := internalCrypto.Sha512Half([]byte(message))
	result := secp256k1.VerifySignature(pubKeyBytes, messageHash, sigBytes)

	return result
}
