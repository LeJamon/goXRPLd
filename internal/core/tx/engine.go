package tx

import (
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	crypto "github.com/LeJamon/goXRPLd/internal/crypto/common"
)

// Engine processes transactions against a ledger
type Engine struct {
	// View provides access to ledger state
	view LedgerView

	// Config holds engine configuration
	config EngineConfig

	// currentTxHash is the hash of the transaction currently being applied
	// Used to set PreviousTxnID on modified ledger entries
	currentTxHash [32]byte
}

// EngineConfig holds configuration for the transaction engine
type EngineConfig struct {
	// BaseFee is the current base fee in drops
	BaseFee uint64

	// ReserveBase is the base reserve in drops
	ReserveBase uint64

	// ReserveIncrement is the owner reserve increment in drops
	ReserveIncrement uint64

	// LedgerSequence is the current ledger sequence
	LedgerSequence uint32

	// SkipSignatureVerification skips signature checks (for testing/standalone)
	SkipSignatureVerification bool

	// Standalone indicates if running in standalone mode (relaxes some validation)
	Standalone bool
}

// LedgerView provides read/write access to ledger state
type LedgerView interface {
	// Read reads a ledger entry
	Read(k keylet.Keylet) ([]byte, error)

	// Exists checks if an entry exists
	Exists(k keylet.Keylet) (bool, error)

	// Insert adds a new entry
	Insert(k keylet.Keylet, data []byte) error

	// Update modifies an existing entry
	Update(k keylet.Keylet, data []byte) error

	// Erase removes an entry
	Erase(k keylet.Keylet) error

	// AdjustDropsDestroyed records destroyed XRP
	AdjustDropsDestroyed(drops XRPAmount.XRPAmount)

	// ForEach iterates over all state entries
	// If fn returns false, iteration stops early
	ForEach(fn func(key [32]byte, data []byte) bool) error
}

// ApplyResult contains the result of applying a transaction
type ApplyResult struct {
	// Result is the transaction result code
	Result Result

	// Applied indicates if the transaction was applied to the ledger
	Applied bool

	// Fee is the fee charged (in drops)
	Fee uint64

	// Metadata contains the changes made by the transaction
	Metadata *Metadata

	// Message is a human-readable result message
	Message string
}

// Metadata tracks changes made by a transaction
type Metadata struct {
	// AffectedNodes lists all nodes that were created, modified, or deleted
	AffectedNodes []AffectedNode

	// TransactionIndex is the index in the ledger
	TransactionIndex uint32

	// TransactionResult is the result code
	TransactionResult Result

	// DeliveredAmount is the actual amount delivered (for partial payments)
	DeliveredAmount *Amount
}

// AffectedNode represents a ledger entry that was changed
type AffectedNode struct {
	// NodeType is "CreatedNode", "ModifiedNode", or "DeletedNode"
	NodeType string

	// LedgerEntryType is the type of ledger entry
	LedgerEntryType string

	// LedgerIndex is the key of the entry
	LedgerIndex string

	// PreviousTxnLgrSeq is the ledger sequence of the previous transaction that modified this entry
	PreviousTxnLgrSeq uint32

	// PreviousTxnID is the hash of the previous transaction that modified this entry
	PreviousTxnID string

	// FinalFields contains the final state (for Modified/Deleted)
	FinalFields map[string]any

	// PreviousFields contains the previous state (for Modified)
	PreviousFields map[string]any

	// NewFields contains the new state (for Created)
	NewFields map[string]any
}

// MarshalJSON implements custom JSON marshaling for Metadata to match rippled format
func (m Metadata) MarshalJSON() ([]byte, error) {
	// Build the output structure matching rippled's format
	output := make(map[string]any)

	// Sort AffectedNodes by LedgerIndex (ascending) to match rippled's ordering
	sortedNodes := make([]AffectedNode, len(m.AffectedNodes))
	copy(sortedNodes, m.AffectedNodes)
	sort.Slice(sortedNodes, func(i, j int) bool {
		return sortedNodes[i].LedgerIndex < sortedNodes[j].LedgerIndex
	})

	// AffectedNodes with nested structure
	affectedNodes := make([]map[string]any, 0, len(sortedNodes))
	for _, node := range sortedNodes {
		nodeJSON, err := node.toRippledFormat()
		if err != nil {
			return nil, err
		}
		affectedNodes = append(affectedNodes, nodeJSON)
	}
	output["AffectedNodes"] = affectedNodes

	// TransactionIndex
	output["TransactionIndex"] = m.TransactionIndex

	// TransactionResult as string
	output["TransactionResult"] = m.TransactionResult.String()

	// delivered_amount (snake_case per rippled format)
	// Use "unavailable" for legacy compatibility when not explicitly set
	if m.DeliveredAmount != nil {
		output["delivered_amount"] = m.DeliveredAmount
	}

	return json.Marshal(output)
}

// toRippledFormat converts an AffectedNode to rippled's nested format
func (n AffectedNode) toRippledFormat() (map[string]any, error) {
	// Build the inner node content
	inner := make(map[string]any)

	// FinalFields (for ModifiedNode and DeletedNode)
	if n.FinalFields != nil {
		inner["FinalFields"] = n.FinalFields
	}

	// LedgerEntryType
	inner["LedgerEntryType"] = n.LedgerEntryType

	// LedgerIndex
	inner["LedgerIndex"] = n.LedgerIndex

	// PreviousFields (for ModifiedNode only, omit if nil/empty)
	if n.PreviousFields != nil && len(n.PreviousFields) > 0 {
		inner["PreviousFields"] = n.PreviousFields
	}

	// PreviousTxnID (omit if empty)
	if n.PreviousTxnID != "" {
		inner["PreviousTxnID"] = n.PreviousTxnID
	}

	// PreviousTxnLgrSeq (omit if zero, which means not set)
	if n.PreviousTxnLgrSeq != 0 {
		inner["PreviousTxnLgrSeq"] = n.PreviousTxnLgrSeq
	}

	// NewFields (for CreatedNode only, omit if nil)
	if n.NewFields != nil {
		inner["NewFields"] = n.NewFields
	}

	// Wrap in NodeType (e.g., "ModifiedNode": {...})
	return map[string]any{
		n.NodeType: inner,
	}, nil
}

// NewEngine creates a new transaction engine
func NewEngine(view LedgerView, config EngineConfig) *Engine {
	return &Engine{
		view:   view,
		config: config,
	}
}

// computeTransactionHash computes the hash of a transaction
// The hash is SHA512Half of the "TXN\x00" prefix + serialized transaction
func computeTransactionHash(tx Transaction) ([32]byte, error) {
	var hash [32]byte
	var txBytes []byte

	// Use raw bytes if available (from parsing), otherwise re-serialize
	if rawBytes := tx.GetRawBytes(); len(rawBytes) > 0 {
		txBytes = rawBytes
	} else {
		// Serialize the transaction using Flatten
		txMap, err := tx.Flatten()
		if err != nil {
			return hash, err
		}

		// Encode to binary using the binary codec
		hexStr, err := binarycodec.Encode(txMap)
		if err != nil {
			return hash, err
		}

		txBytes, err = hex.DecodeString(hexStr)
		if err != nil {
			return hash, err
		}
	}

	// Prefix is "TXN\x00" = 0x54584E00
	prefix := []byte{0x54, 0x58, 0x4E, 0x00}
	data := append(prefix, txBytes...)

	hash = crypto.Sha512Half(data)
	return hash, nil
}

// Apply processes a transaction and applies it to the ledger
func (e *Engine) Apply(tx Transaction) ApplyResult {
	// Step 1: Preflight checks (syntax validation)
	result := e.preflight(tx)
	if !result.IsSuccess() {
		return ApplyResult{
			Result:  result,
			Applied: false,
			Message: result.Message(),
		}
	}

	// Step 2: Preclaim checks (validate against ledger state)
	result = e.preclaim(tx)
	if !result.IsSuccess() && !result.IsTec() {
		return ApplyResult{
			Result:  result,
			Applied: false,
			Message: result.Message(),
		}
	}

	// Step 3: Calculate and apply fee
	fee := e.calculateFee(tx)

	// Step 4: Compute transaction hash
	txHash, err := computeTransactionHash(tx)
	if err != nil {
		return ApplyResult{
			Result:  TefINTERNAL,
			Applied: false,
			Fee:     fee,
			Message: "failed to compute transaction hash: " + err.Error(),
		}
	}

	// Step 5: Apply the transaction
	metadata := &Metadata{
		AffectedNodes:     make([]AffectedNode, 0),
		TransactionResult: TesSUCCESS,
	}

	if result.IsSuccess() {
		result = e.doApply(tx, metadata, txHash)
	}

	metadata.TransactionResult = result

	// Record fee as destroyed
	if result.IsApplied() {
		e.view.AdjustDropsDestroyed(XRPAmount.XRPAmount(fee))
	}

	return ApplyResult{
		Result:   result,
		Applied:  result.IsApplied(),
		Fee:      fee,
		Metadata: metadata,
		Message:  result.Message(),
	}
}

// preflight performs initial validation on the transaction
func (e *Engine) preflight(tx Transaction) Result {
	// Validate common fields
	common := tx.GetCommon()

	// Account is required
	if common.Account == "" {
		return TemBAD_SRC_ACCOUNT
	}

	// TransactionType is required
	if common.TransactionType == "" {
		return TemINVALID
	}

	// Fee must be valid
	if common.Fee != "" {
		fee, err := strconv.ParseUint(common.Fee, 10, 64)
		if err != nil || fee == 0 {
			return TemBAD_FEE
		}
	}

	// Sequence must be present (unless using tickets)
	if common.Sequence == nil && common.TicketSequence == nil {
		return TemBAD_SEQUENCE
	}

	// Verify signature (unless skipped for testing)
	if !e.config.SkipSignatureVerification {
		if err := VerifySignature(tx); err != nil {
			switch err {
			case ErrMissingSignature:
				return TemBAD_SIGNATURE
			case ErrMissingPublicKey:
				return TemBAD_SIGNATURE
			case ErrInvalidSignature:
				return TemBAD_SIGNATURE
			case ErrPublicKeyMismatch:
				return TemBAD_SRC_ACCOUNT
			default:
				return TemBAD_SIGNATURE
			}
		}
	}

	// Transaction-specific validation
	if err := tx.Validate(); err != nil {
		return TemINVALID
	}

	return TesSUCCESS
}

// preclaim validates the transaction against the current ledger state
func (e *Engine) preclaim(tx Transaction) Result {
	common := tx.GetCommon()

	// Check that the source account exists
	accountID, err := decodeAccountID(common.Account)
	if err != nil {
		return TemBAD_SRC_ACCOUNT
	}

	accountKey := keylet.Account(accountID)
	exists, err := e.view.Exists(accountKey)
	if err != nil {
		return TefINTERNAL
	}
	if !exists {
		return TerNO_ACCOUNT
	}

	// Read account data
	accountData, err := e.view.Read(accountKey)
	if err != nil {
		return TefINTERNAL
	}

	// Parse account and check sequence
	account, err := parseAccountRoot(accountData)
	if err != nil {
		return TefINTERNAL
	}

	// Check sequence number
	if common.Sequence != nil {
		if *common.Sequence < account.Sequence {
			return TefPAST_SEQ
		}
		if *common.Sequence > account.Sequence {
			return TerPRE_SEQ
		}
	}

	// Check that account can pay the fee
	fee := e.calculateFee(tx)
	if account.Balance < fee {
		return TerINSUF_FEE_B
	}

	// LastLedgerSequence check
	if common.LastLedgerSequence != nil {
		if e.config.LedgerSequence > *common.LastLedgerSequence {
			return TefMAX_LEDGER
		}
	}

	return TesSUCCESS
}

// doApply applies the transaction to the ledger
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

	// Type-specific application
	var result Result
	switch t := tx.(type) {
	case *Payment:
		result = e.applyPayment(t, account, metadata)
	case *AccountSet:
		result = e.applyAccountSet(t, account, metadata)
	case *TrustSet:
		result = e.applyTrustSet(t, account, metadata)
	case *OfferCreate:
		result = e.applyOfferCreate(t, account, metadata)
	case *OfferCancel:
		result = e.applyOfferCancel(t, account, metadata)
	case *SetRegularKey:
		result = e.applySetRegularKey(t, account, metadata)
	case *SignerListSet:
		result = e.applySignerListSet(t, account, metadata)
	case *TicketCreate:
		result = e.applyTicketCreate(t, account, metadata)
	case *DepositPreauth:
		result = e.applyDepositPreauth(t, account, metadata)
	case *AccountDelete:
		result = e.applyAccountDelete(t, account, metadata)
	case *EscrowCreate:
		result = e.applyEscrowCreate(t, account, metadata)
	case *EscrowFinish:
		result = e.applyEscrowFinish(t, account, metadata)
	case *EscrowCancel:
		result = e.applyEscrowCancel(t, account, metadata)
	case *PaymentChannelCreate:
		result = e.applyPaymentChannelCreate(t, account, metadata)
	case *PaymentChannelFund:
		result = e.applyPaymentChannelFund(t, account, metadata)
	case *PaymentChannelClaim:
		result = e.applyPaymentChannelClaim(t, account, metadata)
	case *CheckCreate:
		result = e.applyCheckCreate(t, account, metadata)
	case *CheckCash:
		result = e.applyCheckCash(t, account, metadata)
	case *CheckCancel:
		result = e.applyCheckCancel(t, account, metadata)
	case *NFTokenMint:
		result = e.applyNFTokenMint(t, account, metadata)
	case *NFTokenBurn:
		result = e.applyNFTokenBurn(t, account, metadata)
	case *NFTokenCreateOffer:
		result = e.applyNFTokenCreateOffer(t, account, metadata)
	case *NFTokenCancelOffer:
		result = e.applyNFTokenCancelOffer(t, account, metadata)
	case *NFTokenAcceptOffer:
		result = e.applyNFTokenAcceptOffer(t, account, metadata)
	case *AMMCreate:
		result = e.applyAMMCreate(t, account, metadata)
	case *AMMDeposit:
		result = e.applyAMMDeposit(t, account, metadata)
	case *AMMWithdraw:
		result = e.applyAMMWithdraw(t, account, metadata)
	case *AMMVote:
		result = e.applyAMMVote(t, account, metadata)
	case *AMMBid:
		result = e.applyAMMBid(t, account, metadata)
	case *AMMDelete:
		result = e.applyAMMDelete(t, account, metadata)
	case *AMMClawback:
		result = e.applyAMMClawback(t, account, metadata)
	case *XChainCreateBridge:
		result = e.applyXChainCreateBridge(t, account, metadata)
	case *XChainModifyBridge:
		result = e.applyXChainModifyBridge(t, account, metadata)
	case *XChainCreateClaimID:
		result = e.applyXChainCreateClaimID(t, account, metadata)
	case *XChainCommit:
		result = e.applyXChainCommit(t, account, metadata)
	case *XChainClaim:
		result = e.applyXChainClaim(t, account, metadata)
	case *XChainAccountCreateCommit:
		result = e.applyXChainAccountCreateCommit(t, account, metadata)
	case *XChainAddClaimAttestation:
		result = e.applyXChainAddClaimAttestation(t, account, metadata)
	case *XChainAddAccountCreateAttestation:
		result = e.applyXChainAddAccountCreateAttestation(t, account, metadata)
	case *DIDSet:
		result = e.applyDIDSet(t, account, metadata)
	case *DIDDelete:
		result = e.applyDIDDelete(t, account, metadata)
	case *OracleSet:
		result = e.applyOracleSet(t, account, metadata)
	case *OracleDelete:
		result = e.applyOracleDelete(t, account, metadata)
	case *MPTokenIssuanceCreate:
		result = e.applyMPTokenIssuanceCreate(t, account, metadata)
	case *MPTokenIssuanceDestroy:
		result = e.applyMPTokenIssuanceDestroy(t, account, metadata)
	case *MPTokenIssuanceSet:
		result = e.applyMPTokenIssuanceSet(t, account, metadata)
	case *MPTokenAuthorize:
		result = e.applyMPTokenAuthorize(t, account, metadata)
	case *Clawback:
		result = e.applyClawback(t, account, metadata)
	case *NFTokenModify:
		result = e.applyNFTokenModify(t, account, metadata)
	case *CredentialCreate:
		result = e.applyCredentialCreate(t, account, metadata)
	case *CredentialAccept:
		result = e.applyCredentialAccept(t, account, metadata)
	case *CredentialDelete:
		result = e.applyCredentialDelete(t, account, metadata)
	case *PermissionedDomainSet:
		result = e.applyPermissionedDomainSet(t, account, metadata)
	case *PermissionedDomainDelete:
		result = e.applyPermissionedDomainDelete(t, account, metadata)
	case *DelegateSet:
		result = e.applyDelegateSet(t, account, metadata)
	case *VaultCreate:
		result = e.applyVaultCreate(t, account, metadata)
	case *VaultSet:
		result = e.applyVaultSet(t, account, metadata)
	case *VaultDelete:
		result = e.applyVaultDelete(t, account, metadata)
	case *VaultDeposit:
		result = e.applyVaultDeposit(t, account, metadata)
	case *VaultWithdraw:
		result = e.applyVaultWithdraw(t, account, metadata)
	case *VaultClawback:
		result = e.applyVaultClawback(t, account, metadata)
	case *Batch:
		result = e.applyBatch(t, account, metadata)
	case *LedgerStateFix:
		result = e.applyLedgerStateFix(t, account, metadata)
	default:
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
		PreviousFields: map[string]any{
			"Balance":  strconv.FormatUint(previousBalance, 10),
			"Sequence": previousSequence,
		},
	}
	// Prepend sender node (sender should be first in AffectedNodes, like rippled does)
	metadata.AffectedNodes = append([]AffectedNode{senderNode}, metadata.AffectedNodes...)

	return result
}

// calculateFee calculates the fee for a transaction
func (e *Engine) calculateFee(tx Transaction) uint64 {
	common := tx.GetCommon()
	if common.Fee != "" {
		fee, err := strconv.ParseUint(common.Fee, 10, 64)
		if err == nil {
			return fee
		}
	}
	return e.config.BaseFee
}
