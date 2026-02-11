package tx

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
	"github.com/LeJamon/goXRPLd/internal/core/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
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

	// QualityOne Per rippled: QUALITY_ONE (1e9 = 1000000000) is treated as default (stored as 0)
	QualityOne uint32 = 1000000000
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
func (l *engineSignerListLookup) GetSignerList(account string) (*sle.SignerListInfo, error) {
	accountID, err := sle.DecodeAccountID(account)
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

	signerList, err := sle.ParseSignerList(signerListData)
	if err != nil {
		return nil, err
	}

	return signerList, nil
}

// GetAccountInfo returns account information needed for signer validation
func (l *engineSignerListLookup) GetAccountInfo(account string) (flags uint32, regularKey string, err error) {
	accountID, err := sle.DecodeAccountID(account)
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

	accountRoot, err := sle.ParseAccountRoot(accountData)
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

// AffectedNode is an alias for sle.AffectedNode
type AffectedNode = sle.AffectedNode

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
		nodeJSON, err := affectedNodeToRippledFormat(node)
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
func affectedNodeToRippledFormat(n AffectedNode) (map[string]any, error) {
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
	// Check if this is a pseudo-transaction (Amendment, SetFee, UNLModify)
	txType := tx.TxType()
	if txType.IsPseudoTransaction() {
		return e.applyPseudoTransaction(tx)
	}

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

// applyPseudoTransaction handles pseudo-transactions (Amendment, SetFee, UNLModify).
// These transactions have special handling:
// - No source account (account is zero/empty)
// - No fee (fee is 0)
// - No signature
// - No sequence number checks
// Reference: rippled Change.cpp
func (e *Engine) applyPseudoTransaction(tx Transaction) ApplyResult {
	// Compute transaction hash
	txHash, err := computeTransactionHash(tx)
	if err != nil {
		return ApplyResult{
			Result:  TefINTERNAL,
			Applied: false,
			Message: "failed to compute transaction hash: " + err.Error(),
		}
	}

	// Create metadata
	metadata := &Metadata{
		AffectedNodes:     make([]AffectedNode, 0),
		TransactionResult: TesSUCCESS,
	}

	// Create ApplyStateTable to track changes
	table := NewApplyStateTable(e.view, txHash, e.config.LedgerSequence)

	// Create a minimal ApplyContext for pseudo-transactions
	ctx := &ApplyContext{
		View:     table,
		Account:  nil, // No account for pseudo-transactions
		Config:   e.config,
		TxHash:   txHash,
		Metadata: metadata,
		Engine:   e,
	}

	// Apply the transaction
	var result Result
	if appliable, ok := tx.(Appliable); ok {
		result = appliable.Apply(ctx)
	} else {
		result = TesSUCCESS
	}

	metadata.TransactionResult = result

	// Apply all tracked changes to the base view and generate metadata
	if result.IsSuccess() {
		generatedMeta, err := table.Apply()
		if err != nil {
			return ApplyResult{
				Result:   TefINTERNAL,
				Applied:  false,
				Metadata: metadata,
				Message:  "failed to apply state changes: " + err.Error(),
			}
		}
		metadata.AffectedNodes = generatedMeta.AffectedNodes
	}

	return ApplyResult{
		Result:   result,
		Applied:  result.IsApplied(),
		Fee:      0, // Pseudo-transactions have no fee
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

	// Amendment check - verify all required amendments are enabled
	// Reference: rippled checks this in each transaction's preflight() method
	for _, featureID := range tx.RequiredAmendments() {
		if !e.rules().Enabled(featureID) {
			return TemDISABLED
		}
	}

	// tfInnerBatchTxn flag validation
	// Reference: rippled Transactor.cpp preflight0() - tfInnerBatchTxn can only be set
	// when processing inner batch transactions, never on directly submitted transactions.
	if common.Flags != nil && *common.Flags&TfInnerBatchTxn != 0 {
		return TemINVALID_FLAG
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
			// Single-signed transaction — verify cryptographic signature validity.
			// The signing key authorization (master vs regular key) is checked in preclaim.
			if err := VerifySignature(tx); err != nil {
				return TemBAD_SIGNATURE
			}
		}
	}

	// Transaction-specific validation
	if err := tx.Validate(); err != nil {
		// Try to extract a specific TER code from the error message
		// Many Validate() implementations include the TER code as a prefix (e.g., "temREDUNDANT: message")
		return parseValidationError(err)
	}

	return TesSUCCESS
}

// parseValidationError extracts a TER result code from a validation error message.
// If the error message starts with a valid TER code prefix (e.g., "temREDUNDANT:"),
// it returns the corresponding Result. Otherwise, it returns TemINVALID.
func parseValidationError(err error) Result {
	msg := err.Error()

	// Check for known TER code prefixes
	// Common tem (malformed) codes
	terCodes := map[string]Result{
		"temMALFORMED":              TemMALFORMED,
		"temBAD_AMOUNT":             TemBAD_AMOUNT,
		"temBAD_CURRENCY":           TemBAD_CURRENCY,
		"temBAD_EXPIRATION":         TemBAD_EXPIRATION,
		"temBAD_FEE":                TemBAD_FEE,
		"temBAD_ISSUER":             TemBAD_ISSUER,
		"temBAD_LIMIT":              TemBAD_LIMIT,
		"temBAD_OFFER":              TemBAD_OFFER,
		"temBAD_PATH":               TemBAD_PATH,
		"temBAD_PATH_LOOP":          TemBAD_PATH_LOOP,
		"temBAD_REGKEY":             TemBAD_REGKEY,
		"temBAD_SEQUENCE":           TemBAD_SEQUENCE,
		"temBAD_SIGNATURE":          TemBAD_SIGNATURE,
		"temBAD_SRC_ACCOUNT":        TemBAD_SRC_ACCOUNT,
		"temBAD_TRANSFER_RATE":      TemBAD_TRANSFER_RATE,
		"temDST_IS_SRC":             TemDST_IS_SRC,
		"temDST_NEEDED":             TemDST_NEEDED,
		"temINVALID":                TemINVALID,
		"temINVALID_FLAG":           TemINVALID_FLAG,
		"temREDUNDANT":              TemREDUNDANT,
		"temRIPPLE_EMPTY":           TemRIPPLE_EMPTY,
		"temDISABLED":               TemDISABLED,
		"temBAD_SIGNER":             TemBAD_SIGNER,
		"temBAD_QUORUM":             TemBAD_QUORUM,
		"temBAD_WEIGHT":             TemBAD_WEIGHT,
		"temBAD_TICK_SIZE":          TemBAD_TICK_SIZE,
		"temINVALID_ACCOUNT_ID":     TemINVALID_ACCOUNT_ID,
		"temUNCERTAIN":              TemUNCERTAIN,
		"temUNKNOWN":                TemUNKNOWN,
		"temSEQ_AND_TICKET":         TemSEQ_AND_TICKET,
		"temBAD_SEND_XRP_MAX":       TemBAD_SEND_XRP_MAX,
		"temBAD_SEND_XRP_PARTIAL":   TemBAD_SEND_XRP_PARTIAL,
		"temBAD_SEND_XRP_PATHS":     TemBAD_SEND_XRP_PATHS,
		"temBAD_SEND_XRP_LIMIT":     TemBAD_SEND_XRP_LIMIT,
		"temBAD_SEND_XRP_NO_DIRECT": TemBAD_SEND_XRP_NO_DIRECT,
		"temCAN_NOT_PREAUTH_SELF":   TemCAN_NOT_PREAUTH_SELF,
		"temEMPTY_DID":              TemEMPTY_DID,
		"temARRAY_EMPTY":            TemARRAY_EMPTY,
		"temARRAY_TOO_LARGE":        TemARRAY_TOO_LARGE,
		"temBAD_AMM_TOKENS":         TemBAD_AMM_TOKENS,
	}

	// Check if the message starts with any known TER code
	for code, result := range terCodes {
		if len(msg) >= len(code) && msg[:len(code)] == code {
			// Check that it's followed by a colon, space, or is the entire message
			if len(msg) == len(code) || msg[len(code)] == ':' || msg[len(code)] == ' ' {
				return result
			}
		}
	}

	// Default to temINVALID
	return TemINVALID
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
	accountID, err := sle.DecodeAccountID(common.Account)
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
	account, err := sle.ParseAccountRoot(accountData)
	if err != nil {
		return TefINTERNAL
	}

	// Check single-sign authorization (master key or regular key)
	// Reference: rippled Transactor::checkSingleSign in Transactor.cpp
	// Multi-signed transactions are already verified in preflight via VerifyMultiSignature.
	if !e.config.SkipSignatureVerification && !IsMultiSigned(tx) && common.SigningPubKey != "" {
		signerAddress, addrErr := addresscodec.EncodeClassicAddressFromPublicKeyHex(common.SigningPubKey)
		if addrErr != nil {
			return TefBAD_AUTH
		}

		isMasterDisabled := (account.Flags & sle.LsfDisableMaster) != 0

		if signerAddress == account.RegularKey {
			// Signed with regular key — allowed
		} else if !isMasterDisabled && signerAddress == common.Account {
			// Signed with enabled master key — allowed
		} else if isMasterDisabled && signerAddress == common.Account {
			// Signed with disabled master key
			return TefMASTER_DISABLED
		} else {
			// Signed with an unauthorized key
			return TefBAD_AUTH
		}
	}

	// Check sequence number or ticket
	// Reference: rippled Transactor::checkSeqProxy in Transactor.cpp
	if common.TicketSequence != nil {
		// Ticket-based transaction: validate the ticket exists
		if *common.TicketSequence >= account.Sequence {
			// Ticket hasn't been created yet
			return TerPRE_TICKET
		}
		ticketKey := keylet.Ticket(accountID, *common.TicketSequence)
		ticketExists, ticketErr := e.view.Exists(ticketKey)
		if ticketErr != nil || !ticketExists {
			return TefNO_TICKET
		}
	} else if common.Sequence != nil {
		if *common.Sequence < account.Sequence {
			return TefPAST_SEQ
		}
		if *common.Sequence > account.Sequence {
			return TerPRE_SEQ
		}
	}

	// Batch minimum fee validation
	// Reference: rippled Transactor::checkFee() - telINSUF_FEE_P for insufficient batch fee
	fee := e.calculateFee(tx)
	if feeCalc, ok := tx.(BatchFeeCalculator); ok {
		minFee := feeCalc.CalculateMinimumFee(e.config.BaseFee)
		if fee < minFee {
			return TelINSUF_FEE_P
		}
	}

	// Check that account can pay the fee
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
// For tec results, only fee/sequence changes are applied; transaction effects are discarded.
// Reference: rippled Transactor.cpp - tec results claim fee but don't apply effects
func (e *Engine) doApply(tx Transaction, metadata *Metadata, txHash [32]byte) Result {
	// Store txHash for use by apply functions
	e.currentTxHash = txHash

	// Deduct fee from sender first (this always happens for applied transactions)
	common := tx.GetCommon()
	accountID, _ := sle.DecodeAccountID(common.Account)
	accountKey := keylet.Account(accountID)

	// Read sender account directly from view
	accountData, err := e.view.Read(accountKey)
	if err != nil {
		return TefINTERNAL
	}

	account, err := sle.ParseAccountRoot(accountData)
	if err != nil {
		return TefINTERNAL
	}

	fee := e.calculateFee(tx)

	// Store original account state for fee-only application
	originalBalance := account.Balance
	originalSequence := account.Sequence

	// Deduct fee and handle sequence/ticket
	// Reference: rippled Transactor::consumeSeqProxy in Transactor.cpp
	account.Balance -= fee
	isTicket := common.TicketSequence != nil
	if !isTicket && common.Sequence != nil {
		account.Sequence = *common.Sequence + 1
	}

	// Update PreviousTxnID and PreviousTxnLgrSeq (thread the account)
	account.PreviousTxnID = txHash
	account.PreviousTxnLgrSeq = e.config.LedgerSequence

	// Create ApplyStateTable for transaction-specific changes
	table := NewApplyStateTable(e.view, txHash, e.config.LedgerSequence)

	// Type-specific application - all operations go through the table
	var result Result

	// All transaction types implement Appliable
	ctx := &ApplyContext{
		View:      table,
		Account:   account,
		AccountID: accountID,
		Config:    e.config,
		TxHash:    txHash,
		Metadata:  metadata,
		Engine:    e,
	}
	if appliable, ok := tx.(Appliable); ok {
		result = appliable.Apply(ctx)
	} else {
		result = TesSUCCESS
	}

	// Consume ticket on success or tec results
	// Reference: rippled Transactor::consumeSeqProxy + ticketDelete
	if isTicket && (result == TesSUCCESS || result.IsTec()) {
		ticketKey := keylet.Ticket(accountID, *common.TicketSequence)
		ownerDirKey := keylet.OwnerDir(accountID)
		if result == TesSUCCESS {
			// Remove ticket from owner directory (page 0)
			sle.DirRemove(table, ownerDirKey, 0, ticketKey.Key, true)
			if err := table.Erase(ticketKey); err != nil {
				return TefINTERNAL
			}
		} else {
			// For tec, operate directly on view
			sle.DirRemove(e.view, ownerDirKey, 0, ticketKey.Key, true)
			if err := e.view.Erase(ticketKey); err != nil {
				return TefINTERNAL
			}
		}
		if account.OwnerCount > 0 {
			account.OwnerCount--
		}
	}

	// For tec results, only apply fee/sequence changes, not transaction effects
	// Reference: rippled - tec codes claim the fee but don't apply transaction effects
	if result.IsTec() {
		// Apply only fee and sequence changes to the source account
		account.Balance = originalBalance - fee
		account.Sequence = originalSequence
		if !isTicket && common.Sequence != nil {
			account.Sequence = *common.Sequence + 1
		}

		updatedData, err := sle.SerializeAccountRoot(account)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Update(accountKey, updatedData); err != nil {
			return TefINTERNAL
		}

		// Special case: tecEXPIRED allows side-effects (e.g., expired credential deletion)
		// Reference: rippled Transactor.cpp - tecEXPIRED re-applies removeExpiredCredentials
		if result == TecEXPIRED {
			if tecApplier, ok := tx.(TecApplier); ok {
				tecCtx := &ApplyContext{
					View:      e.view,
					Account:   account,
					AccountID: accountID,
					Config:    e.config,
					TxHash:    txHash,
					Metadata:  metadata,
					Engine:    e,
				}
				tecApplier.ApplyOnTec(tecCtx)
				// Re-read the account from view after side-effects
				// (ApplyOnTec may have modified owner count via credential deletion)
				accountData, err := e.view.Read(accountKey)
				if err != nil || accountData == nil {
					return TefINTERNAL
				}
				account, err = sle.ParseAccountRoot(accountData)
				if err != nil {
					return TefINTERNAL
				}
			}
		}

		// Generate minimal metadata for fee-only application
		metadata.AffectedNodes = []AffectedNode{
			{
				NodeType:        "ModifiedNode",
				LedgerEntryType: "AccountRoot",
				LedgerIndex:     fmt.Sprintf("%X", accountKey.Key),
			},
		}

		return result
	}

	// For success, apply all changes through the table
	// Update the source account through the table (unless erased by e.g. AccountDelete)
	if !table.IsErased(accountKey) {
		updatedData, err := sle.SerializeAccountRoot(account)
		if err != nil {
			return TefINTERNAL
		}

		if err := table.Update(accountKey, updatedData); err != nil {
			return TefINTERNAL
		}
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
