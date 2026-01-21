package tx

// Amendment names for required amendments.
// These match the amendment names registered in the amendment package.
const (
	// Check transactions
	AmendmentChecks = "Checks"

	// AMM transactions
	AmendmentAMM                = "AMM"
	AmendmentFixUniversalNumber = "fixUniversalNumber"
	AmendmentAMMClawback        = "AMMClawback"

	// NFToken transactions
	AmendmentNonFungibleTokensV1   = "NonFungibleTokensV1"
	AmendmentNonFungibleTokensV1_1 = "NonFungibleTokensV1_1"
	AmendmentDynamicNFT            = "DynamicNFT"

	// XChain transactions
	AmendmentXChainBridge = "XChainBridge"

	// DID transactions
	AmendmentDID = "DID"

	// Oracle transactions
	AmendmentPriceOracle = "PriceOracle"

	// Credential transactions
	AmendmentCredentials = "Credentials"

	// MPToken transactions
	AmendmentMPTokensV1 = "MPTokensV1"

	// Vault transactions
	AmendmentSingleAssetVault = "SingleAssetVault"

	// PermissionedDomain transactions
	AmendmentPermissionedDomains = "PermissionedDomains"

	// Clawback transaction
	AmendmentClawback = "Clawback"

	// Batch transaction
	AmendmentBatch = "Batch"

	// LedgerStateFix transaction
	AmendmentFixNFTokenPageLinks = "fixNFTokenPageLinks"

	// DelegateSet transaction
	AmendmentPermissionDelegation = "PermissionDelegation"

	// TokenEscrow amendment
	AmendmentTokenEscrow = "TokenEscrow"

	// DeepFreeze amendment
	AmendmentDeepFreeze = "DeepFreeze"

	// PayChan amendment (Payment Channels)
	AmendmentPayChan = "PayChan"

	// EscrowTx amendment (Escrow transactions)
	AmendmentEscrow = "Escrow"
)
