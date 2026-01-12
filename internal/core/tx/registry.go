package tx

import (
	"encoding/json"
	"errors"
)

// ErrUnknownTransactionType is returned when a transaction type is unknown
var ErrUnknownTransactionType = errors.New("unknown transaction type")

// FromJSON creates a Transaction from a JSON object
func FromJSON(data []byte) (Transaction, error) {
	// First, unmarshal to get the TransactionType
	var raw struct {
		TransactionType string `json:"TransactionType"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	txType, ok := TypeFromName(raw.TransactionType)
	if !ok {
		return nil, ErrUnknownTransactionType
	}

	// Create the appropriate transaction type
	tx, err := NewFromType(txType)
	if err != nil {
		return nil, err
	}

	// Unmarshal into the specific type
	if err := json.Unmarshal(data, tx); err != nil {
		return nil, err
	}

	return tx, nil
}

// NewFromType creates a new transaction of the given type
func NewFromType(txType Type) (Transaction, error) {
	switch txType {
	case TypePayment:
		return &Payment{BaseTx: *NewBaseTx(TypePayment, "")}, nil
	case TypeAccountSet:
		return &AccountSet{BaseTx: *NewBaseTx(TypeAccountSet, "")}, nil
	case TypeTrustSet:
		return &TrustSet{BaseTx: *NewBaseTx(TypeTrustSet, "")}, nil
	case TypeOfferCreate:
		return &OfferCreate{BaseTx: *NewBaseTx(TypeOfferCreate, "")}, nil
	case TypeOfferCancel:
		return &OfferCancel{BaseTx: *NewBaseTx(TypeOfferCancel, "")}, nil
	case TypeEscrowCreate:
		return &EscrowCreate{BaseTx: *NewBaseTx(TypeEscrowCreate, "")}, nil
	case TypeEscrowFinish:
		return &EscrowFinish{BaseTx: *NewBaseTx(TypeEscrowFinish, "")}, nil
	case TypeEscrowCancel:
		return &EscrowCancel{BaseTx: *NewBaseTx(TypeEscrowCancel, "")}, nil
	case TypePaymentChannelCreate:
		return &PaymentChannelCreate{BaseTx: *NewBaseTx(TypePaymentChannelCreate, "")}, nil
	case TypePaymentChannelFund:
		return &PaymentChannelFund{BaseTx: *NewBaseTx(TypePaymentChannelFund, "")}, nil
	case TypePaymentChannelClaim:
		return &PaymentChannelClaim{BaseTx: *NewBaseTx(TypePaymentChannelClaim, "")}, nil
	case TypeCheckCreate:
		return &CheckCreate{BaseTx: *NewBaseTx(TypeCheckCreate, "")}, nil
	case TypeCheckCash:
		return &CheckCash{BaseTx: *NewBaseTx(TypeCheckCash, "")}, nil
	case TypeCheckCancel:
		return &CheckCancel{BaseTx: *NewBaseTx(TypeCheckCancel, "")}, nil
	case TypeNFTokenMint:
		return &NFTokenMint{BaseTx: *NewBaseTx(TypeNFTokenMint, "")}, nil
	case TypeNFTokenBurn:
		return &NFTokenBurn{BaseTx: *NewBaseTx(TypeNFTokenBurn, "")}, nil
	case TypeNFTokenCreateOffer:
		return &NFTokenCreateOffer{BaseTx: *NewBaseTx(TypeNFTokenCreateOffer, "")}, nil
	case TypeNFTokenCancelOffer:
		return &NFTokenCancelOffer{BaseTx: *NewBaseTx(TypeNFTokenCancelOffer, "")}, nil
	case TypeNFTokenAcceptOffer:
		return &NFTokenAcceptOffer{BaseTx: *NewBaseTx(TypeNFTokenAcceptOffer, "")}, nil
	case TypeNFTokenModify:
		return &NFTokenModify{BaseTx: *NewBaseTx(TypeNFTokenModify, "")}, nil
	case TypeSignerListSet:
		return &SignerListSet{BaseTx: *NewBaseTx(TypeSignerListSet, "")}, nil
	case TypeRegularKeySet:
		return &SetRegularKey{BaseTx: *NewBaseTx(TypeRegularKeySet, "")}, nil
	case TypeDepositPreauth:
		return &DepositPreauth{BaseTx: *NewBaseTx(TypeDepositPreauth, "")}, nil
	case TypeAccountDelete:
		return &AccountDelete{BaseTx: *NewBaseTx(TypeAccountDelete, "")}, nil
	case TypeTicketCreate:
		return &TicketCreate{BaseTx: *NewBaseTx(TypeTicketCreate, "")}, nil
	case TypeClawback:
		return &Clawback{BaseTx: *NewBaseTx(TypeClawback, "")}, nil
	case TypeAMMCreate:
		return &AMMCreate{BaseTx: *NewBaseTx(TypeAMMCreate, "")}, nil
	case TypeAMMDeposit:
		return &AMMDeposit{BaseTx: *NewBaseTx(TypeAMMDeposit, "")}, nil
	case TypeAMMWithdraw:
		return &AMMWithdraw{BaseTx: *NewBaseTx(TypeAMMWithdraw, "")}, nil
	case TypeAMMVote:
		return &AMMVote{BaseTx: *NewBaseTx(TypeAMMVote, "")}, nil
	case TypeAMMBid:
		return &AMMBid{BaseTx: *NewBaseTx(TypeAMMBid, "")}, nil
	case TypeAMMDelete:
		return &AMMDelete{BaseTx: *NewBaseTx(TypeAMMDelete, "")}, nil
	case TypeAMMClawback:
		return &AMMClawback{BaseTx: *NewBaseTx(TypeAMMClawback, "")}, nil
	case TypeDIDSet:
		return &DIDSet{BaseTx: *NewBaseTx(TypeDIDSet, "")}, nil
	case TypeDIDDelete:
		return &DIDDelete{BaseTx: *NewBaseTx(TypeDIDDelete, "")}, nil
	case TypeOracleSet:
		return &OracleSet{BaseTx: *NewBaseTx(TypeOracleSet, "")}, nil
	case TypeOracleDelete:
		return &OracleDelete{BaseTx: *NewBaseTx(TypeOracleDelete, "")}, nil
	case TypeXChainCreateBridge:
		return &XChainCreateBridge{BaseTx: *NewBaseTx(TypeXChainCreateBridge, "")}, nil
	case TypeXChainModifyBridge:
		return &XChainModifyBridge{BaseTx: *NewBaseTx(TypeXChainModifyBridge, "")}, nil
	case TypeXChainCreateClaimID:
		return &XChainCreateClaimID{BaseTx: *NewBaseTx(TypeXChainCreateClaimID, "")}, nil
	case TypeXChainCommit:
		return &XChainCommit{BaseTx: *NewBaseTx(TypeXChainCommit, "")}, nil
	case TypeXChainClaim:
		return &XChainClaim{BaseTx: *NewBaseTx(TypeXChainClaim, "")}, nil
	case TypeXChainAccountCreateCommit:
		return &XChainAccountCreateCommit{BaseTx: *NewBaseTx(TypeXChainAccountCreateCommit, "")}, nil
	case TypeXChainAddClaimAttestation:
		return &XChainAddClaimAttestation{BaseTx: *NewBaseTx(TypeXChainAddClaimAttestation, "")}, nil
	case TypeXChainAddAccountCreateAttest:
		return &XChainAddAccountCreateAttestation{BaseTx: *NewBaseTx(TypeXChainAddAccountCreateAttest, "")}, nil
	case TypeCredentialCreate:
		return &CredentialCreate{BaseTx: *NewBaseTx(TypeCredentialCreate, "")}, nil
	case TypeCredentialAccept:
		return &CredentialAccept{BaseTx: *NewBaseTx(TypeCredentialAccept, "")}, nil
	case TypeCredentialDelete:
		return &CredentialDelete{BaseTx: *NewBaseTx(TypeCredentialDelete, "")}, nil
	case TypeMPTokenIssuanceCreate:
		return &MPTokenIssuanceCreate{BaseTx: *NewBaseTx(TypeMPTokenIssuanceCreate, "")}, nil
	case TypeMPTokenIssuanceDestroy:
		return &MPTokenIssuanceDestroy{BaseTx: *NewBaseTx(TypeMPTokenIssuanceDestroy, "")}, nil
	case TypeMPTokenIssuanceSet:
		return &MPTokenIssuanceSet{BaseTx: *NewBaseTx(TypeMPTokenIssuanceSet, "")}, nil
	case TypeMPTokenAuthorize:
		return &MPTokenAuthorize{BaseTx: *NewBaseTx(TypeMPTokenAuthorize, "")}, nil
	case TypePermissionedDomainSet:
		return &PermissionedDomainSet{BaseTx: *NewBaseTx(TypePermissionedDomainSet, "")}, nil
	case TypePermissionedDomainDelete:
		return &PermissionedDomainDelete{BaseTx: *NewBaseTx(TypePermissionedDomainDelete, "")}, nil
	case TypeDelegateSet:
		return &DelegateSet{BaseTx: *NewBaseTx(TypeDelegateSet, "")}, nil
	case TypeVaultCreate:
		return &VaultCreate{BaseTx: *NewBaseTx(TypeVaultCreate, "")}, nil
	case TypeVaultSet:
		return &VaultSet{BaseTx: *NewBaseTx(TypeVaultSet, "")}, nil
	case TypeVaultDelete:
		return &VaultDelete{BaseTx: *NewBaseTx(TypeVaultDelete, "")}, nil
	case TypeVaultDeposit:
		return &VaultDeposit{BaseTx: *NewBaseTx(TypeVaultDeposit, "")}, nil
	case TypeVaultWithdraw:
		return &VaultWithdraw{BaseTx: *NewBaseTx(TypeVaultWithdraw, "")}, nil
	case TypeVaultClawback:
		return &VaultClawback{BaseTx: *NewBaseTx(TypeVaultClawback, "")}, nil
	case TypeBatch:
		return &Batch{BaseTx: *NewBaseTx(TypeBatch, "")}, nil
	case TypeLedgerStateFix:
		return &LedgerStateFix{BaseTx: *NewBaseTx(TypeLedgerStateFix, "")}, nil
	default:
		return nil, ErrUnknownTransactionType
	}
}

// ToJSON converts a Transaction to JSON
func ToJSON(tx Transaction) ([]byte, error) {
	flat, err := tx.Flatten()
	if err != nil {
		return nil, err
	}
	return json.Marshal(flat)
}

// Validate validates a transaction and returns any errors
func Validate(tx Transaction) error {
	return tx.Validate()
}

// SupportedTypes returns all supported transaction types
func SupportedTypes() []Type {
	return []Type{
		TypePayment,
		TypeEscrowCreate,
		TypeEscrowFinish,
		TypeAccountSet,
		TypeEscrowCancel,
		TypeRegularKeySet,
		TypeOfferCreate,
		TypeOfferCancel,
		TypeTicketCreate,
		TypeSignerListSet,
		TypePaymentChannelCreate,
		TypePaymentChannelFund,
		TypePaymentChannelClaim,
		TypeCheckCreate,
		TypeCheckCash,
		TypeCheckCancel,
		TypeDepositPreauth,
		TypeTrustSet,
		TypeAccountDelete,
		TypeNFTokenMint,
		TypeNFTokenBurn,
		TypeNFTokenCreateOffer,
		TypeNFTokenCancelOffer,
		TypeNFTokenAcceptOffer,
		TypeNFTokenModify,
		TypeClawback,
		TypeAMMClawback,
		TypeAMMCreate,
		TypeAMMDeposit,
		TypeAMMWithdraw,
		TypeAMMVote,
		TypeAMMBid,
		TypeAMMDelete,
		TypeXChainCreateClaimID,
		TypeXChainCommit,
		TypeXChainClaim,
		TypeXChainAccountCreateCommit,
		TypeXChainAddClaimAttestation,
		TypeXChainAddAccountCreateAttest,
		TypeXChainModifyBridge,
		TypeXChainCreateBridge,
		TypeDIDSet,
		TypeDIDDelete,
		TypeOracleSet,
		TypeOracleDelete,
		TypeLedgerStateFix,
		TypeMPTokenIssuanceCreate,
		TypeMPTokenIssuanceDestroy,
		TypeMPTokenIssuanceSet,
		TypeMPTokenAuthorize,
		TypeCredentialCreate,
		TypeCredentialAccept,
		TypeCredentialDelete,
		TypePermissionedDomainSet,
		TypePermissionedDomainDelete,
		TypeDelegateSet,
		TypeVaultCreate,
		TypeVaultSet,
		TypeVaultDelete,
		TypeVaultDeposit,
		TypeVaultWithdraw,
		TypeVaultClawback,
		TypeBatch,
	}
}
