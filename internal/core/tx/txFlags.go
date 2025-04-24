// Package protocol provides definitions for transaction flags.
package transactions

// Universal Transaction flags:
const (
	TfFullyCanonicalSig = 0x80000000
	TfUniversal         = TfFullyCanonicalSig
	TfUniversalMask     = ^TfUniversal
)

// AccountSet flags:
const (
	TfRequireDestTag  = 0x00010000
	TfOptionalDestTag = 0x00020000
	TfRequireAuth     = 0x00040000
	TfOptionalAuth    = 0x00080000
	TfDisallowXRP     = 0x00100000
	TfAllowXRP        = 0x00200000
	TfAccountSetMask  = ^(TfUniversal | TfRequireDestTag | TfOptionalDestTag |
		TfRequireAuth | TfOptionalAuth | TfDisallowXRP | TfAllowXRP)
)

// AccountSet SetFlag/ClearFlag values
const (
	AsfRequireDest                  = 1
	AsfRequireAuth                  = 2
	AsfDisallowXRP                  = 3
	AsfDisableMaster                = 4
	AsfAccountTxnID                 = 5
	AsfNoFreeze                     = 6
	AsfGlobalFreeze                 = 7
	AsfDefaultRipple                = 8
	AsfDepositAuth                  = 9
	AsfAuthorizedNFTokenMinter      = 10
	AsfDisallowIncomingNFTokenOffer = 12
	AsfDisallowIncomingCheck        = 13
	AsfDisallowIncomingPayChan      = 14
	AsfDisallowIncomingTrustline    = 15
	AsfAllowTrustLineClawback       = 16
)

// OfferCreate flags:
const (
	TfPassive           = 0x00010000
	TfImmediateOrCancel = 0x00020000
	TfFillOrKill        = 0x00040000
	TfSell              = 0x00080000
	TfOfferCreateMask   = ^(TfUniversal | TfPassive | TfImmediateOrCancel | TfFillOrKill | TfSell)
)

// Payment flags:
const (
	TfNoRippleDirect = 0x00010000
	TfPartialPayment = 0x00020000
	TfLimitQuality   = 0x00040000
	TfPaymentMask    = ^(TfUniversal | TfPartialPayment | TfLimitQuality | TfNoRippleDirect)
	TfMPTPaymentMask = ^(TfUniversal | TfPartialPayment)
)

// TrustSet flags:
const (
	TfSetfAuth      = 0x00010000
	TfSetNoRipple   = 0x00020000
	TfClearNoRipple = 0x00040000
	TfSetFreeze     = 0x00100000
	TfClearFreeze   = 0x00200000
	TfTrustSetMask  = ^(TfUniversal | TfSetfAuth | TfSetNoRipple | TfClearNoRipple | TfSetFreeze | TfClearFreeze)
)

// EnableAmendment flags:
const (
	TfGotMajority  = 0x00010000
	TfLostMajority = 0x00020000
)

// PaymentChannelClaim flags:
const (
	TfRenew            = 0x00010000
	TfClose            = 0x00020000
	TfPayChanClaimMask = ^(TfUniversal | TfRenew | TfClose)
)

// NFTokenMint flags:
const (
	TfBurnable        = 0x00000001
	TfOnlyXRP         = 0x00000002
	TfTrustLine       = 0x00000004
	TfTransferable    = 0x00000008
	TfNFTokenMintMask = ^(TfUniversal | TfBurnable | TfOnlyXRP | TfTransferable)
)

// NFTokenCreateOffer flags:
const (
	TfSellNFToken            = 0x00000001
	TfNFTokenCreateOfferMask = ^(TfUniversal | TfSellNFToken)
)

// NFTokenCancelOffer flags:
const (
	TfNFTokenCancelOfferMask = ^TfUniversal
)

// NFTokenAcceptOffer flags:
const (
	TfNFTokenAcceptOfferMask = ^TfUniversal
)

// Clawback flags:
const (
	TfClawbackMask = ^TfUniversal
)

// AMM Flags:
const (
	TfLPToken             = 0x00010000
	TfWithdrawAll         = 0x00020000
	TfOneAssetWithdrawAll = 0x00040000
	TfSingleAsset         = 0x00080000
	TfTwoAsset            = 0x00100000
	TfOneAssetLPToken     = 0x00200000
	TfLimitLPToken        = 0x00400000
	TfTwoAssetIfEmpty     = 0x00800000
	TfWithdrawSubTx       = TfLPToken | TfSingleAsset | TfTwoAsset | TfOneAssetLPToken | TfLimitLPToken | TfWithdrawAll | TfOneAssetWithdrawAll
	TfDepositSubTx        = TfLPToken | TfSingleAsset | TfTwoAsset | TfOneAssetLPToken | TfLimitLPToken | TfTwoAssetIfEmpty
	TfWithdrawMask        = ^(TfUniversal | TfWithdrawSubTx)
	TfDepositMask         = ^(TfUniversal | TfDepositSubTx)
)

// AMMClawback flags:
const (
	TfClawTwoAssets   = 0x00000001
	TfAMMClawbackMask = ^(TfUniversal | TfClawTwoAssets)
)

// BridgeModify flags:
const (
	TfClearAccountCreateAmount = 0x00010000
	TfBridgeModifyMask         = ^(TfUniversal | TfClearAccountCreateAmount)
)
