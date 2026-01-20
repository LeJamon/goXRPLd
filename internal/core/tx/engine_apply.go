package tx

import (
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// ApplyFunc is the signature for transaction apply functions.
// Each transaction type implements this to apply its specific logic.
type ApplyFunc func(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result

// applyRegistry maps transaction types to their apply functions
var applyRegistry = make(map[Type]ApplyFunc)

// RegisterApplyFunc registers an apply function for a transaction type.
// This is called during package initialization to register all apply functions.
func RegisterApplyFunc(txType Type, fn ApplyFunc) {
	applyRegistry[txType] = fn
}

// init registers all built-in apply functions
func init() {
	// Payment
	RegisterApplyFunc(TypePayment, applyPaymentWrapper)

	// Account operations
	RegisterApplyFunc(TypeAccountSet, applyAccountSetWrapper)
	RegisterApplyFunc(TypeRegularKeySet, applySetRegularKeyWrapper)
	RegisterApplyFunc(TypeSignerListSet, applySignerListSetWrapper)
	RegisterApplyFunc(TypeDepositPreauth, applyDepositPreauthWrapper)
	RegisterApplyFunc(TypeAccountDelete, applyAccountDeleteWrapper)

	// Trust lines
	RegisterApplyFunc(TypeTrustSet, applyTrustSetWrapper)

	// Offers
	RegisterApplyFunc(TypeOfferCreate, applyOfferCreateWrapper)
	RegisterApplyFunc(TypeOfferCancel, applyOfferCancelWrapper)

	// Escrow
	RegisterApplyFunc(TypeEscrowCreate, applyEscrowCreateWrapper)
	RegisterApplyFunc(TypeEscrowFinish, applyEscrowFinishWrapper)
	RegisterApplyFunc(TypeEscrowCancel, applyEscrowCancelWrapper)

	// Payment channels
	RegisterApplyFunc(TypePaymentChannelCreate, applyPaymentChannelCreateWrapper)
	RegisterApplyFunc(TypePaymentChannelFund, applyPaymentChannelFundWrapper)
	RegisterApplyFunc(TypePaymentChannelClaim, applyPaymentChannelClaimWrapper)

	// Checks
	RegisterApplyFunc(TypeCheckCreate, applyCheckCreateWrapper)
	RegisterApplyFunc(TypeCheckCash, applyCheckCashWrapper)
	RegisterApplyFunc(TypeCheckCancel, applyCheckCancelWrapper)

	// Tickets
	RegisterApplyFunc(TypeTicketCreate, applyTicketCreateWrapper)

	// NFTokens
	RegisterApplyFunc(TypeNFTokenMint, applyNFTokenMintWrapper)
	RegisterApplyFunc(TypeNFTokenBurn, applyNFTokenBurnWrapper)
	RegisterApplyFunc(TypeNFTokenCreateOffer, applyNFTokenCreateOfferWrapper)
	RegisterApplyFunc(TypeNFTokenCancelOffer, applyNFTokenCancelOfferWrapper)
	RegisterApplyFunc(TypeNFTokenAcceptOffer, applyNFTokenAcceptOfferWrapper)
	RegisterApplyFunc(TypeNFTokenModify, applyNFTokenModifyWrapper)

	// AMM
	RegisterApplyFunc(TypeAMMCreate, applyAMMCreateWrapper)
	RegisterApplyFunc(TypeAMMDeposit, applyAMMDepositWrapper)
	RegisterApplyFunc(TypeAMMWithdraw, applyAMMWithdrawWrapper)
	RegisterApplyFunc(TypeAMMVote, applyAMMVoteWrapper)
	RegisterApplyFunc(TypeAMMBid, applyAMMBidWrapper)
	RegisterApplyFunc(TypeAMMDelete, applyAMMDeleteWrapper)
	RegisterApplyFunc(TypeAMMClawback, applyAMMClawbackWrapper)

	// XChain
	RegisterApplyFunc(TypeXChainCreateBridge, applyXChainCreateBridgeWrapper)
	RegisterApplyFunc(TypeXChainModifyBridge, applyXChainModifyBridgeWrapper)
	RegisterApplyFunc(TypeXChainCreateClaimID, applyXChainCreateClaimIDWrapper)
	RegisterApplyFunc(TypeXChainCommit, applyXChainCommitWrapper)
	RegisterApplyFunc(TypeXChainClaim, applyXChainClaimWrapper)
	RegisterApplyFunc(TypeXChainAccountCreateCommit, applyXChainAccountCreateCommitWrapper)
	RegisterApplyFunc(TypeXChainAddClaimAttestation, applyXChainAddClaimAttestationWrapper)
	RegisterApplyFunc(TypeXChainAddAccountCreateAttest, applyXChainAddAccountCreateAttestationWrapper)

	// DID
	RegisterApplyFunc(TypeDIDSet, applyDIDSetWrapper)
	RegisterApplyFunc(TypeDIDDelete, applyDIDDeleteWrapper)

	// Oracle
	RegisterApplyFunc(TypeOracleSet, applyOracleSetWrapper)
	RegisterApplyFunc(TypeOracleDelete, applyOracleDeleteWrapper)

	// MPToken
	RegisterApplyFunc(TypeMPTokenIssuanceCreate, applyMPTokenIssuanceCreateWrapper)
	RegisterApplyFunc(TypeMPTokenIssuanceDestroy, applyMPTokenIssuanceDestroyWrapper)
	RegisterApplyFunc(TypeMPTokenIssuanceSet, applyMPTokenIssuanceSetWrapper)
	RegisterApplyFunc(TypeMPTokenAuthorize, applyMPTokenAuthorizeWrapper)

	// Clawback
	RegisterApplyFunc(TypeClawback, applyClawbackWrapper)

	// Credentials
	RegisterApplyFunc(TypeCredentialCreate, applyCredentialCreateWrapper)
	RegisterApplyFunc(TypeCredentialAccept, applyCredentialAcceptWrapper)
	RegisterApplyFunc(TypeCredentialDelete, applyCredentialDeleteWrapper)

	// Permissioned domains
	RegisterApplyFunc(TypePermissionedDomainSet, applyPermissionedDomainSetWrapper)
	RegisterApplyFunc(TypePermissionedDomainDelete, applyPermissionedDomainDeleteWrapper)

	// Delegate
	RegisterApplyFunc(TypeDelegateSet, applyDelegateSetWrapper)

	// Vault
	RegisterApplyFunc(TypeVaultCreate, applyVaultCreateWrapper)
	RegisterApplyFunc(TypeVaultSet, applyVaultSetWrapper)
	RegisterApplyFunc(TypeVaultDelete, applyVaultDeleteWrapper)
	RegisterApplyFunc(TypeVaultDeposit, applyVaultDepositWrapper)
	RegisterApplyFunc(TypeVaultWithdraw, applyVaultWithdrawWrapper)
	RegisterApplyFunc(TypeVaultClawback, applyVaultClawbackWrapper)

	// Batch
	RegisterApplyFunc(TypeBatch, applyBatchWrapper)

	// LedgerStateFix
	RegisterApplyFunc(TypeLedgerStateFix, applyLedgerStateFixWrapper)
}

// doApply applies the transaction to the ledger.
// This method uses a registry-based dispatch instead of a switch statement.
func (e *Engine) doApply(tx Transaction, metadata *Metadata, txHash [32]byte) Result {
	// Store txHash for use by apply functions
	e.currentTxHash = txHash

	// Deduct fee from sender
	common := tx.GetCommon()
	accountID, _ := decodeAccountID(common.Account)
	accountKey := keylet.Account(accountID)

	accountData, err := e.view.Read(accountKey)
	if err != nil {
		return TefINTERNAL
	}

	account, err := parseAccountRoot(accountData)
	if err != nil {
		return TefINTERNAL
	}

	fee := e.calculateFee(tx)
	previousBalance := account.Balance
	previousSequence := account.Sequence
	previousOwnerCount := account.OwnerCount
	previousTxnID := account.PreviousTxnID
	previousTxnLgrSeq := account.PreviousTxnLgrSeq

	// Deduct fee and increment sequence
	account.Balance -= fee
	if common.Sequence != nil {
		account.Sequence = *common.Sequence + 1
	}

	// Update PreviousTxnID and PreviousTxnLgrSeq (thread the account)
	account.PreviousTxnID = txHash
	account.PreviousTxnLgrSeq = e.config.LedgerSequence

	// Type-specific application using registry
	var result Result
	txType := tx.TxType()
	if applyFn, ok := applyRegistry[txType]; ok {
		result = applyFn(e, tx, account, metadata)
	} else {
		// For unimplemented transaction types, just update the account
		result = TesSUCCESS
	}

	// Update the source account
	updatedData, err := serializeAccountRoot(account)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(accountKey, updatedData); err != nil {
		return TefINTERNAL
	}

	// Record account modification in metadata
	// Include all fields that rippled marks with sMD_Always (Flags, OwnerCount)
	// and the previous transaction threading info
	// IMPORTANT: Sender's node should be FIRST in the list (prepend)
	prevFields := map[string]any{
		"Balance":  strconv.FormatUint(previousBalance, 10),
		"Sequence": previousSequence,
	}
	// Only include OwnerCount in PreviousFields if it changed
	if account.OwnerCount != previousOwnerCount {
		prevFields["OwnerCount"] = previousOwnerCount
	}
	senderNode := AffectedNode{
		NodeType:          "ModifiedNode",
		LedgerEntryType:   "AccountRoot",
		LedgerIndex:       strings.ToUpper(hex.EncodeToString(accountKey.Key[:])),
		PreviousTxnLgrSeq: previousTxnLgrSeq,
		PreviousTxnID:     strings.ToUpper(hex.EncodeToString(previousTxnID[:])),
		FinalFields: map[string]any{
			"Account":    common.Account,
			"Balance":    strconv.FormatUint(account.Balance, 10),
			"Flags":      account.Flags,
			"OwnerCount": account.OwnerCount,
			"Sequence":   account.Sequence,
		},
		PreviousFields: prevFields,
	}
	// Prepend sender node (sender should be first in AffectedNodes, like rippled does)
	metadata.PrependAffectedNode(senderNode)

	return result
}

// Wrapper functions that cast the generic Transaction to the specific type
// These are registered in init() above

func applyPaymentWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyPayment(tx.(*Payment), sender, metadata)
}

func applyAccountSetWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyAccountSet(tx.(*AccountSet), sender, metadata)
}

func applySetRegularKeyWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applySetRegularKey(tx.(*SetRegularKey), sender, metadata)
}

func applySignerListSetWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applySignerListSet(tx.(*SignerListSet), sender, metadata)
}

func applyDepositPreauthWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyDepositPreauth(tx.(*DepositPreauth), sender, metadata)
}

func applyAccountDeleteWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyAccountDelete(tx.(*AccountDelete), sender, metadata)
}

func applyTrustSetWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyTrustSet(tx.(*TrustSet), sender, metadata)
}

func applyOfferCreateWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyOfferCreate(tx.(*OfferCreate), sender, metadata)
}

func applyOfferCancelWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyOfferCancel(tx.(*OfferCancel), sender, metadata)
}

func applyEscrowCreateWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyEscrowCreate(tx.(*EscrowCreate), sender, metadata)
}

func applyEscrowFinishWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyEscrowFinish(tx.(*EscrowFinish), sender, metadata)
}

func applyEscrowCancelWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyEscrowCancel(tx.(*EscrowCancel), sender, metadata)
}

func applyPaymentChannelCreateWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyPaymentChannelCreate(tx.(*PaymentChannelCreate), sender, metadata)
}

func applyPaymentChannelFundWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyPaymentChannelFund(tx.(*PaymentChannelFund), sender, metadata)
}

func applyPaymentChannelClaimWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyPaymentChannelClaim(tx.(*PaymentChannelClaim), sender, metadata)
}

func applyCheckCreateWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyCheckCreate(tx.(*CheckCreate), sender, metadata)
}

func applyCheckCashWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyCheckCash(tx.(*CheckCash), sender, metadata)
}

func applyCheckCancelWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyCheckCancel(tx.(*CheckCancel), sender, metadata)
}

func applyTicketCreateWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyTicketCreate(tx.(*TicketCreate), sender, metadata)
}

func applyNFTokenMintWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyNFTokenMint(tx.(*NFTokenMint), sender, metadata)
}

func applyNFTokenBurnWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyNFTokenBurn(tx.(*NFTokenBurn), sender, metadata)
}

func applyNFTokenCreateOfferWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyNFTokenCreateOffer(tx.(*NFTokenCreateOffer), sender, metadata)
}

func applyNFTokenCancelOfferWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyNFTokenCancelOffer(tx.(*NFTokenCancelOffer), sender, metadata)
}

func applyNFTokenAcceptOfferWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyNFTokenAcceptOffer(tx.(*NFTokenAcceptOffer), sender, metadata)
}

func applyNFTokenModifyWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyNFTokenModify(tx.(*NFTokenModify), sender, metadata)
}

func applyAMMCreateWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyAMMCreate(tx.(*AMMCreate), sender, metadata)
}

func applyAMMDepositWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyAMMDeposit(tx.(*AMMDeposit), sender, metadata)
}

func applyAMMWithdrawWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyAMMWithdraw(tx.(*AMMWithdraw), sender, metadata)
}

func applyAMMVoteWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyAMMVote(tx.(*AMMVote), sender, metadata)
}

func applyAMMBidWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyAMMBid(tx.(*AMMBid), sender, metadata)
}

func applyAMMDeleteWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyAMMDelete(tx.(*AMMDelete), sender, metadata)
}

func applyAMMClawbackWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyAMMClawback(tx.(*AMMClawback), sender, metadata)
}

func applyXChainCreateBridgeWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyXChainCreateBridge(tx.(*XChainCreateBridge), sender, metadata)
}

func applyXChainModifyBridgeWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyXChainModifyBridge(tx.(*XChainModifyBridge), sender, metadata)
}

func applyXChainCreateClaimIDWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyXChainCreateClaimID(tx.(*XChainCreateClaimID), sender, metadata)
}

func applyXChainCommitWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyXChainCommit(tx.(*XChainCommit), sender, metadata)
}

func applyXChainClaimWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyXChainClaim(tx.(*XChainClaim), sender, metadata)
}

func applyXChainAccountCreateCommitWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyXChainAccountCreateCommit(tx.(*XChainAccountCreateCommit), sender, metadata)
}

func applyXChainAddClaimAttestationWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyXChainAddClaimAttestation(tx.(*XChainAddClaimAttestation), sender, metadata)
}

func applyXChainAddAccountCreateAttestationWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyXChainAddAccountCreateAttestation(tx.(*XChainAddAccountCreateAttestation), sender, metadata)
}

func applyDIDSetWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyDIDSet(tx.(*DIDSet), sender, metadata)
}

func applyDIDDeleteWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyDIDDelete(tx.(*DIDDelete), sender, metadata)
}

func applyOracleSetWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyOracleSet(tx.(*OracleSet), sender, metadata)
}

func applyOracleDeleteWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyOracleDelete(tx.(*OracleDelete), sender, metadata)
}

func applyMPTokenIssuanceCreateWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyMPTokenIssuanceCreate(tx.(*MPTokenIssuanceCreate), sender, metadata)
}

func applyMPTokenIssuanceDestroyWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyMPTokenIssuanceDestroy(tx.(*MPTokenIssuanceDestroy), sender, metadata)
}

func applyMPTokenIssuanceSetWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyMPTokenIssuanceSet(tx.(*MPTokenIssuanceSet), sender, metadata)
}

func applyMPTokenAuthorizeWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyMPTokenAuthorize(tx.(*MPTokenAuthorize), sender, metadata)
}

func applyClawbackWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyClawback(tx.(*Clawback), sender, metadata)
}

func applyCredentialCreateWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyCredentialCreate(tx.(*CredentialCreate), sender, metadata)
}

func applyCredentialAcceptWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyCredentialAccept(tx.(*CredentialAccept), sender, metadata)
}

func applyCredentialDeleteWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyCredentialDelete(tx.(*CredentialDelete), sender, metadata)
}

func applyPermissionedDomainSetWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyPermissionedDomainSet(tx.(*PermissionedDomainSet), sender, metadata)
}

func applyPermissionedDomainDeleteWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyPermissionedDomainDelete(tx.(*PermissionedDomainDelete), sender, metadata)
}

func applyDelegateSetWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyDelegateSet(tx.(*DelegateSet), sender, metadata)
}

func applyVaultCreateWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyVaultCreate(tx.(*VaultCreate), sender, metadata)
}

func applyVaultSetWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyVaultSet(tx.(*VaultSet), sender, metadata)
}

func applyVaultDeleteWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyVaultDelete(tx.(*VaultDelete), sender, metadata)
}

func applyVaultDepositWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyVaultDeposit(tx.(*VaultDeposit), sender, metadata)
}

func applyVaultWithdrawWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyVaultWithdraw(tx.(*VaultWithdraw), sender, metadata)
}

func applyVaultClawbackWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyVaultClawback(tx.(*VaultClawback), sender, metadata)
}

func applyBatchWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyBatch(tx.(*Batch), sender, metadata)
}

func applyLedgerStateFixWrapper(e *Engine, tx Transaction, sender *AccountRoot, metadata *Metadata) Result {
	return e.applyLedgerStateFix(tx.(*LedgerStateFix), sender, metadata)
}
