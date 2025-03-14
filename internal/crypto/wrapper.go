package crypto

type CryptoType int

const (
	ED25519 CryptoType = iota
	SECP256K1
)

type SignatureProvider interface {
	GenerateKeypair(seed []byte, isValidator bool) (privateKey, publicKey string, err error)
	SignMessage(message, privateKeyHex string) (signature string, err error)
	VerifySignature(message, publicKeyHex, signatureHex string) bool
}

type CryptoWrapper struct {
	provider   SignatureProvider
	cryptoType CryptoType
}

func NewCryptoWrapper(provider SignatureProvider, cryptoType CryptoType) *CryptoWrapper {
	return &CryptoWrapper{
		provider:   provider,
		cryptoType: cryptoType,
	}
}

func (w *CryptoWrapper) GetCryptoType() CryptoType {
	return w.cryptoType
}

func (w *CryptoWrapper) GenerateKeypair(seed []byte, isValidator bool) (string, string, error) {
	return w.provider.GenerateKeypair(seed, isValidator)
}

func (w *CryptoWrapper) SignMessage(message, privateKeyHex string) (string, error) {
	return w.provider.SignMessage(message, privateKeyHex)
}

func (w *CryptoWrapper) VerifySignature(message, publicKeyHex, signatureHex string) bool {
	return w.provider.VerifySignature(message, publicKeyHex, signatureHex)
}

// Helper constructors for specific implementations
func NewED25519Wrapper(provider SignatureProvider) *CryptoWrapper {
	return NewCryptoWrapper(provider, ED25519)
}

func NewSECP256K1Wrapper(provider SignatureProvider) *CryptoWrapper {
	return NewCryptoWrapper(provider, SECP256K1)
}
