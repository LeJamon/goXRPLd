package tx

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
	"github.com/LeJamon/goXRPLd/internal/core/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	crypto "github.com/LeJamon/goXRPLd/internal/crypto/common"
)

// Validation constants matching rippled
const (
	// MaxMemoSize is the maximum total serialized size of memos (in bytes)
	MaxMemoSize = 1024

	// MaxMemoTypeSize is the maximum size of MemoType field (in bytes)
	MaxMemoTypeSize = 256

	// MaxMemoDataSize is the maximum size of MemoData field (in bytes)
	MaxMemoDataSize = 1024

	// LegacyNetworkIDThreshold is the threshold for legacy network IDs
	// Networks with ID <= this value are legacy networks
	LegacyNetworkIDThreshold = 1024

	// DefaultMaxFee is the default maximum fee (1 XRP = 1,000,000 drops)
	DefaultMaxFee = 1000000
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

// engineSignerListLookup implements SignerListLookup using the engine's ledger view
type engineSignerListLookup struct {
	view LedgerView
}

// GetSignerList returns the signer list for an account
func (l *engineSignerListLookup) GetSignerList(account string) (*SignerListInfo, error) {
	accountID, err := decodeAccountID(account)
	if err != nil {
		return nil, err
	}

	// Look up the signer list (SignerListID is always 0 currently)
	signerListKey := keylet.SignerList(accountID)
	exists, err := l.view.Exists(signerListKey)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil // No signer list
	}

	// Read and parse the signer list
	signerListData, err := l.view.Read(signerListKey)
	if err != nil {
		return nil, err
	}

	signerList, err := parseSignerList(signerListData)
	if err != nil {
		return nil, err
	}

	return signerList, nil
}

// GetAccountInfo returns account information needed for signer validation
func (l *engineSignerListLookup) GetAccountInfo(account string) (flags uint32, regularKey string, err error) {
	accountID, err := decodeAccountID(account)
	if err != nil {
		return 0, "", err
	}

	accountKey := keylet.Account(accountID)
	exists, err := l.view.Exists(accountKey)
	if err != nil {
		return 0, "", err
	}
	if !exists {
		return 0, "", errors.New("account not found")
	}

	accountData, err := l.view.Read(accountKey)
	if err != nil {
		return 0, "", err
	}

	accountRoot, err := parseAccountRoot(accountData)
	if err != nil {
		return 0, "", err
	}

	return accountRoot.Flags, accountRoot.RegularKey, nil
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

	// NetworkID is the network identifier for this node
	// Networks with ID > 1024 require NetworkID in transactions
	// Networks with ID <= 1024 are legacy networks and cannot have NetworkID in transactions
	NetworkID uint32

	// MaxFee is the maximum allowed fee in drops (default 1 XRP = 1000000 drops)
	// Transactions with fees exceeding this will be rejected in preflight
	MaxFee uint64

	// ParentCloseTime is the close time of the parent ledger (in Ripple epoch seconds)
	// This is used for checking offer/escrow expiration
	ParentCloseTime uint32

	// Rules contains the amendment rules for this ledger.
	// If nil, defaults to all amendments enabled (for backwards compatibility).
	Rules *amendment.Rules
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

// rules returns the amendment rules, defaulting to all amendments enabled if nil.
// This provides backwards compatibility for code that doesn't set Rules.
func (e *Engine) rules() *amendment.Rules {
	if e.config.Rules != nil {
		return e.config.Rules
	}
	// Default to all supported amendments enabled for backwards compatibility
	return amendment.AllSupportedRules()
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

	// NetworkID validation (matching rippled's preflight0)
	if result := e.validateNetworkID(common); result != TesSUCCESS {
		return result
	}

	// Fee validation
	if result := e.validateFee(common); result != TesSUCCESS {
		return result
	}

	// Sequence must be present (unless using tickets)
	if common.Sequence == nil && common.TicketSequence == nil {
		return TemBAD_SEQUENCE
	}

	// SourceTag validation - if present, it's already a uint32 via JSON parsing
	// No additional validation needed as the type system ensures it's valid

	// Memo validation
	if result := e.validateMemos(common); result != TesSUCCESS {
		return result
	}

	// Verify signature (unless skipped for testing)
	if !e.config.SkipSignatureVerification {
		// Check if this is a multi-signed transaction
		if IsMultiSigned(tx) {
			// Multi-signed transactions require signer list lookup
			lookup := &engineSignerListLookup{view: e.view}
			if err := VerifyMultiSignature(tx, lookup); err != nil {
				switch err {
				case ErrNotMultiSigning:
					return TefNOT_MULTI_SIGNING
				case ErrBadQuorum:
					return TefBAD_QUORUM
				case ErrBadSignature:
					return TefBAD_SIGNATURE
				case ErrMasterDisabled:
					return TefMASTER_DISABLED
				case ErrNoSigners:
					return TemBAD_SIGNATURE
				case ErrDuplicateSigner:
					return TemBAD_SIGNATURE
				case ErrSignersNotSorted:
					return TemBAD_SIGNATURE
				default:
					return TefBAD_SIGNATURE
				}
			}
		} else {
			// Single-signed transaction
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
	}

	// Transaction-specific validation
	if err := tx.Validate(); err != nil {
		return TemINVALID
	}

	return TesSUCCESS
}

// validateNetworkID validates the NetworkID field according to rippled rules
// - Legacy networks (ID <= 1024) cannot have NetworkID in transactions
// - New networks (ID > 1024) require NetworkID and it must match
func (e *Engine) validateNetworkID(common *Common) Result {
	nodeNetworkID := e.config.NetworkID
	txNetworkID := common.NetworkID

	if nodeNetworkID <= LegacyNetworkIDThreshold {
		// Legacy networks cannot specify NetworkID in transactions
		if txNetworkID != nil {
			return TelNETWORK_ID_MAKES_TX_NON_CANONICAL
		}
	} else {
		// New networks require NetworkID to be present and match
		if txNetworkID == nil {
			return TelREQUIRES_NETWORK_ID
		}
		if *txNetworkID != nodeNetworkID {
			return TelWRONG_NETWORK
		}
	}

	return TesSUCCESS
}

// validateFee validates the Fee field
func (e *Engine) validateFee(common *Common) Result {
	if common.Fee == "" {
		return TesSUCCESS // Fee will be checked later if needed
	}

	// Parse fee as signed int first to detect negative values
	feeInt, err := strconv.ParseInt(common.Fee, 10, 64)
	if err != nil {
		return TemBAD_FEE
	}

	// Fee cannot be negative
	if feeInt < 0 {
		return TemBAD_FEE
	}

	fee := uint64(feeInt)

	// Fee cannot be zero (must pay something)
	if fee == 0 {
		return TemBAD_FEE
	}

	// Fee cannot exceed maximum allowed fee
	maxFee := e.config.MaxFee
	if maxFee == 0 {
		maxFee = DefaultMaxFee
	}
	if fee > maxFee {
		return TemBAD_FEE
	}

	return TesSUCCESS
}

// validateMemos validates the Memos array according to rippled rules
func (e *Engine) validateMemos(common *Common) Result {
	if len(common.Memos) == 0 {
		return TesSUCCESS
	}

	// Calculate total serialized size of memos
	totalSize := 0

	for _, memoWrapper := range common.Memos {
		memo := memoWrapper.Memo

		// Validate MemoType if present
		if memo.MemoType != "" {
			// MemoType must be a valid hex string
			memoTypeBytes, err := hex.DecodeString(memo.MemoType)
			if err != nil {
				return TemINVALID
			}
			// MemoType max size is 256 bytes (decoded)
			if len(memoTypeBytes) > MaxMemoTypeSize {
				return TemINVALID
			}
			totalSize += len(memoTypeBytes)

			// MemoType characters (when decoded) must be valid URL characters per RFC 3986
			if !isValidURLBytes(memoTypeBytes) {
				return TemINVALID
			}
		}

		// Validate MemoData if present
		if memo.MemoData != "" {
			// MemoData must be a valid hex string
			memoDataBytes, err := hex.DecodeString(memo.MemoData)
			if err != nil {
				return TemINVALID
			}
			// MemoData max size is 1024 bytes (decoded)
			if len(memoDataBytes) > MaxMemoDataSize {
				return TemINVALID
			}
			totalSize += len(memoDataBytes)
			// Note: MemoData can contain any data, no character restrictions
		}

		// Validate MemoFormat if present
		if memo.MemoFormat != "" {
			// MemoFormat must be a valid hex string
			memoFormatBytes, err := hex.DecodeString(memo.MemoFormat)
			if err != nil {
				return TemINVALID
			}
			totalSize += len(memoFormatBytes)

			// MemoFormat characters (when decoded) must be valid URL characters per RFC 3986
			if !isValidURLBytes(memoFormatBytes) {
				return TemINVALID
			}
		}
	}

	// Total memo size check
	if totalSize > MaxMemoSize {
		return TemINVALID
	}

	return TesSUCCESS
}

// isValidURLBytes checks if the bytes contain only characters allowed in URLs per RFC 3986
// Allowed: alphanumerics and -._~:/?#[]@!$&'()*+,;=%
func isValidURLBytes(data []byte) bool {
	for _, b := range data {
		if !isURLChar(b) {
			return false
		}
	}
	return true
}

// isURLChar returns true if the byte is a valid URL character per RFC 3986
func isURLChar(c byte) bool {
	// Alphanumerics
	if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
		return true
	}
	// Special characters allowed in URLs: -._~:/?#[]@!$&'()*+,;=%
	switch c {
	case '-', '.', '_', '~', ':', '/', '?', '#', '[', ']', '@', '!', '$', '&', '\'', '(', ')', '*', '+', ',', ';', '=', '%':
		return true
	}
	return false
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

	// Create ApplyStateTable to track all changes and auto-generate metadata
	table := NewApplyStateTable(e.view, txHash, e.config.LedgerSequence)

	// Deduct fee from sender
	common := tx.GetCommon()
	accountID, _ := decodeAccountID(common.Account)
	accountKey := keylet.Account(accountID)

	// Read sender account through the table (tracks it for metadata)
	accountData, err := table.Read(accountKey)
	if err != nil {
		return TefINTERNAL
	}

	account, err := parseAccountRoot(accountData)
	if err != nil {
		return TefINTERNAL
	}

	fee := e.calculateFee(tx)

	// Deduct fee and increment sequence
	account.Balance -= fee
	if common.Sequence != nil {
		account.Sequence = *common.Sequence + 1
	}

	// Update PreviousTxnID and PreviousTxnLgrSeq (thread the account)
	account.PreviousTxnID = txHash
	account.PreviousTxnLgrSeq = e.config.LedgerSequence

	// Type-specific application - all operations go through the table
	var result Result

	// Prefer the Appliable interface (self-contained transaction types)
	if appliable, ok := tx.(Appliable); ok {
		ctx := &ApplyContext{
			View:      table,
			Account:   account,
			AccountID: accountID,
			Config:    e.config,
			TxHash:    txHash,
			Metadata:  metadata,
		}
		result = appliable.Apply(ctx)
	} else {
		// Fallback to legacy switch (removed as types are migrated)
		result = e.legacyApply(tx, account, metadata, table)
	}

	// Update the source account through the table
	updatedData, err := serializeAccountRoot(account)
	if err != nil {
		return TefINTERNAL
	}

	if err := table.Update(accountKey, updatedData); err != nil {
		return TefINTERNAL
	}

	// Apply all tracked changes to the base view and generate metadata automatically
	generatedMeta, err := table.Apply()
	if err != nil {
		return TefINTERNAL
	}

	// Copy generated metadata to the output
	metadata.AffectedNodes = generatedMeta.AffectedNodes

	return result
}

// legacyApply dispatches to type-specific apply methods for complex types not yet migrated to Appliable.
func (e *Engine) legacyApply(tx Transaction, account *AccountRoot, metadata *Metadata, table *ApplyStateTable) Result {
	var result Result
	switch t := tx.(type) {
	case *Payment:
		result = e.applyPayment(t, account, metadata, table)
	case *TrustSet:
		result = e.applyTrustSet(t, account, table)
	case *OfferCreate:
		result = e.applyOfferCreate(t, account, table)
	case *OfferCancel:
		result = e.applyOfferCancel(t, account, table)
	case *EscrowCreate:
		result = e.applyEscrowCreate(t, account, table)
	case *EscrowFinish:
		result = e.applyEscrowFinish(t, account, table)
	case *EscrowCancel:
		result = e.applyEscrowCancel(t, account, table)
	case *PaymentChannelCreate:
		result = e.applyPaymentChannelCreate(t, account, table)
	case *PaymentChannelFund:
		result = e.applyPaymentChannelFund(t, account, table)
	case *PaymentChannelClaim:
		result = e.applyPaymentChannelClaim(t, account, table)
	case *CheckCreate:
		result = e.applyCheckCreate(t, account, table)
	case *CheckCash:
		result = e.applyCheckCash(t, account, table)
	case *CheckCancel:
		result = e.applyCheckCancel(t, account, table)
	case *NFTokenMint:
		result = e.applyNFTokenMint(t, account, table)
	case *NFTokenBurn:
		result = e.applyNFTokenBurn(t, account, table)
	case *NFTokenCreateOffer:
		result = e.applyNFTokenCreateOffer(t, account, table)
	case *NFTokenCancelOffer:
		result = e.applyNFTokenCancelOffer(t, account, table)
	case *NFTokenAcceptOffer:
		result = e.applyNFTokenAcceptOffer(t, account, table)
	case *AMMCreate:
		result = e.applyAMMCreate(t, account, table)
	case *AMMDeposit:
		result = e.applyAMMDeposit(t, account, table)
	case *AMMWithdraw:
		result = e.applyAMMWithdraw(t, account, table)
	case *AMMVote:
		result = e.applyAMMVote(t, account, table)
	case *AMMBid:
		result = e.applyAMMBid(t, account, table)
	case *AMMDelete:
		result = e.applyAMMDelete(t, account, table)
	case *AMMClawback:
		result = e.applyAMMClawback(t, account, table)
	default:
		result = TesSUCCESS
	}

	return result
}

// calculateFee calculates the fee for a transaction
// For multi-signed transactions, the minimum required fee is baseFee * (1 + numSigners)
func (e *Engine) calculateFee(tx Transaction) uint64 {
	common := tx.GetCommon()
	if common.Fee != "" {
		fee, err := strconv.ParseUint(common.Fee, 10, 64)
		if err == nil {
			return fee
		}
	}
	// If no fee specified, use base fee (adjusted for multi-sig if applicable)
	baseFee := e.config.BaseFee
	if IsMultiSigned(tx) {
		numSigners := len(common.Signers)
		return CalculateMultiSigFee(baseFee, numSigners)
	}
	return baseFee
}

// calculateMinimumFee calculates the minimum required fee for a transaction
// This is used to validate that the provided fee meets the minimum threshold
func (e *Engine) calculateMinimumFee(tx Transaction) uint64 {
	baseFee := e.config.BaseFee
	if IsMultiSigned(tx) {
		common := tx.GetCommon()
		numSigners := len(common.Signers)
		return CalculateMultiSigFee(baseFee, numSigners)
	}
	return baseFee
}

// AccountReserve calculates the total reserve required for an account with the given owner count.
// This matches rippled's accountReserve(ownerCount) calculation.
// Reserve = ReserveBase + (ownerCount * ReserveIncrement)
func (e *Engine) AccountReserve(ownerCount uint32) uint64 {
	return e.config.ReserveBase + (uint64(ownerCount) * e.config.ReserveIncrement)
}

// ReserveForNewObject calculates the reserve required for creating a new ledger object.
// This matches rippled's logic where the first 2 objects don't require extra reserve.
// Reference: rippled SetTrust.cpp:405-407
//
//	XRPAmount const reserveCreate(
//	    (uOwnerCount < 2) ? XRPAmount(beast::zero)
//	                      : view().fees().accountReserve(uOwnerCount + 1));
func (e *Engine) ReserveForNewObject(currentOwnerCount uint32) uint64 {
	if currentOwnerCount < 2 {
		// First 2 objects are free (no extra reserve needed)
		return 0
	}
	// For 3rd object and beyond, require reserve for (ownerCount + 1) objects
	return e.AccountReserve(currentOwnerCount + 1)
}

// CanCreateNewObject checks if an account has enough balance to create a new ledger object.
// This should be used before creating trust lines, offers, tickets, etc.
// It uses mPriorBalance (balance before fee deduction) to match rippled's behavior.
// Reference: rippled SetTrust.cpp:681,710 - mPriorBalance < reserveCreate
func (e *Engine) CanCreateNewObject(priorBalance uint64, currentOwnerCount uint32) bool {
	reserveNeeded := e.ReserveForNewObject(currentOwnerCount)
	return priorBalance >= reserveNeeded
}

// CheckReserveIncrease validates that an account can afford the reserve increase
// for creating a new ledger object. Returns tecINSUFFICIENT_RESERVE if not enough funds.
func (e *Engine) CheckReserveIncrease(priorBalance uint64, currentOwnerCount uint32) Result {
	if !e.CanCreateNewObject(priorBalance, currentOwnerCount) {
		return TecINSUFFICIENT_RESERVE
	}
	return TesSUCCESS
}
