package tx

import (
	"encoding/hex"
	"errors"
	"strings"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	ed25519algo "github.com/LeJamon/goXRPLd/internal/crypto/algorithms/ed25519"
	secp256k1algo "github.com/LeJamon/goXRPLd/internal/crypto/algorithms/secp256k1"
)

// Signature verification errors
var (
	ErrMissingSignature    = errors.New("transaction is not signed")
	ErrMissingPublicKey    = errors.New("signing public key is missing")
	ErrInvalidSignature    = errors.New("signature is invalid")
	ErrPublicKeyMismatch   = errors.New("public key does not match account")
	ErrUnknownKeyType      = errors.New("unknown public key type")
)

// VerifySignature verifies that a transaction is properly signed
// Returns nil if the signature is valid, or an error describing the problem
func VerifySignature(tx Transaction) error {
	common := tx.GetCommon()

	// Check that we have a signature
	if common.TxnSignature == "" {
		return ErrMissingSignature
	}

	// Check that we have a public key
	if common.SigningPubKey == "" {
		return ErrMissingPublicKey
	}

	// Verify the public key corresponds to the account
	if err := verifyPublicKeyMatchesAccount(common.SigningPubKey, common.Account); err != nil {
		return err
	}

	// Get the message that was signed
	signingPayload, err := getSigningPayload(tx)
	if err != nil {
		return errors.New("failed to get signing payload: " + err.Error())
	}

	// Verify the signature based on the key type
	valid := verifySignatureForKey(signingPayload, common.SigningPubKey, common.TxnSignature)
	if !valid {
		return ErrInvalidSignature
	}

	return nil
}

// verifyPublicKeyMatchesAccount checks that the public key derives to the given account
func verifyPublicKeyMatchesAccount(pubKeyHex, account string) error {
	// Derive the account address from the public key
	derivedAddress, err := addresscodec.EncodeClassicAddressFromPublicKeyHex(pubKeyHex)
	if err != nil {
		return errors.New("failed to derive address from public key: " + err.Error())
	}

	if derivedAddress != account {
		return ErrPublicKeyMismatch
	}

	return nil
}

// getSigningPayload returns the binary data that should be signed
func getSigningPayload(tx Transaction) (string, error) {
	// Flatten the transaction to a map
	txMap, err := tx.Flatten()
	if err != nil {
		return "", err
	}

	// Encode for signing (this adds the signing prefix and removes non-signing fields)
	return binarycodec.EncodeForSigning(txMap)
}

// verifySignatureForKey verifies a signature using the appropriate algorithm
func verifySignatureForKey(messageHex, pubKeyHex, signatureHex string) bool {
	// Decode the public key to determine the algorithm
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil || len(pubKeyBytes) == 0 {
		return false
	}

	// The first byte indicates the key type
	// 0xED = ED25519
	// 0x02 or 0x03 = SECP256K1 (compressed)
	keyType := pubKeyBytes[0]

	// Decode the message hex to bytes for signing
	msgBytes, err := hex.DecodeString(messageHex)
	if err != nil {
		return false
	}

	// Convert message bytes to string for the crypto functions
	// The crypto functions expect the raw message bytes as a string
	msgStr := string(msgBytes)

	switch keyType {
	case 0xED:
		// ED25519
		algo := ed25519algo.ED25519()
		return algo.Validate(msgStr, pubKeyHex, signatureHex)

	case 0x02, 0x03:
		// SECP256K1 (compressed public key)
		algo := secp256k1algo.SECP256K1()
		return algo.Validate(msgStr, pubKeyHex, signatureHex)

	default:
		return false
	}
}

// SignTransaction signs a transaction with the given private key
// Returns the signature as a hex string
func SignTransaction(tx Transaction, privateKeyHex string) (string, error) {
	// Get the signing payload
	signingPayload, err := getSigningPayload(tx)
	if err != nil {
		return "", errors.New("failed to get signing payload: " + err.Error())
	}

	// Decode the private key to determine the algorithm
	privKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil || len(privKeyBytes) == 0 {
		return "", errors.New("invalid private key")
	}

	// Decode the message hex to bytes
	msgBytes, err := hex.DecodeString(signingPayload)
	if err != nil {
		return "", errors.New("failed to decode signing payload")
	}

	// Convert message bytes to string for the crypto functions
	msgStr := string(msgBytes)

	// The first byte indicates the key type
	keyType := privKeyBytes[0]

	var signature string

	switch keyType {
	case 0xED:
		// ED25519
		algo := ed25519algo.ED25519()
		signature, err = algo.Sign(msgStr, privateKeyHex)
		if err != nil {
			return "", errors.New("ED25519 signing failed: " + err.Error())
		}

	case 0x00:
		// SECP256K1
		algo := secp256k1algo.SECP256K1()
		signature, err = algo.Sign(msgStr, privateKeyHex)
		if err != nil {
			return "", errors.New("SECP256K1 signing failed: " + err.Error())
		}

	default:
		return "", ErrUnknownKeyType
	}

	return strings.ToUpper(signature), nil
}

// DeriveKeypairFromSeed derives a public/private keypair from a seed
func DeriveKeypairFromSeed(seed string) (privateKey, publicKey string, err error) {
	// Decode the seed to get the entropy and algorithm
	entropy, algo, err := addresscodec.DecodeSeed(seed)
	if err != nil {
		return "", "", errors.New("invalid seed: " + err.Error())
	}

	// Derive the keypair
	privateKey, publicKey, err = algo.DeriveKeypair(entropy, false)
	if err != nil {
		return "", "", errors.New("failed to derive keypair: " + err.Error())
	}

	return privateKey, publicKey, nil
}

// DeriveAddressFromPublicKey derives a classic address from a public key
func DeriveAddressFromPublicKey(publicKeyHex string) (string, error) {
	return addresscodec.EncodeClassicAddressFromPublicKeyHex(publicKeyHex)
}
