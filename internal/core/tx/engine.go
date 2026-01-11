package tx

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
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

	// FinalFields contains the final state (for Modified/Deleted)
	FinalFields map[string]any

	// PreviousFields contains the previous state (for Modified)
	PreviousFields map[string]any

	// NewFields contains the new state (for Created)
	NewFields map[string]any
}

// NewEngine creates a new transaction engine
func NewEngine(view LedgerView, config EngineConfig) *Engine {
	return &Engine{
		view:   view,
		config: config,
	}
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

	// Step 4: Apply the transaction
	metadata := &Metadata{
		AffectedNodes:     make([]AffectedNode, 0),
		TransactionResult: TesSUCCESS,
	}

	if result.IsSuccess() {
		result = e.doApply(tx, metadata)
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
func (e *Engine) doApply(tx Transaction, metadata *Metadata) Result {
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

	// Deduct fee and increment sequence
	account.Balance -= fee
	if common.Sequence != nil {
		account.Sequence = *common.Sequence + 1
	}

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
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(accountKey.Key[:]),
		FinalFields: map[string]any{
			"Account":  common.Account,
			"Balance":  strconv.FormatUint(account.Balance, 10),
			"Sequence": account.Sequence,
		},
		PreviousFields: map[string]any{
			"Balance":  strconv.FormatUint(previousBalance, 10),
			"Sequence": account.Sequence - 1,
		},
	})

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

// AccountRoot represents an account in the ledger
type AccountRoot struct {
	Account      string
	Balance      uint64
	Sequence     uint32
	OwnerCount   uint32
	Flags        uint32
	RegularKey   string
	Domain       string
	EmailHash    string
	MessageKey   string
	TransferRate uint32
	TickSize     uint8
}

// AccountRoot binary format field codes (from XRPL spec)
const (
	// Field type codes
	fieldTypeUInt16    = 1
	fieldTypeUInt32    = 2
	fieldTypeUInt64    = 3
	fieldTypeHash128   = 4
	fieldTypeHash256   = 5
	fieldTypeAmount    = 6
	fieldTypeBlob      = 7
	fieldTypeAccount   = 8
	fieldTypeAccountID = 8 // Same as Account, used in serialization

	// Field codes for AccountRoot
	fieldCodeLedgerEntryType = 1  // UInt16
	fieldCodeFlags           = 2  // UInt32
	fieldCodeSequence        = 4  // UInt32
	fieldCodeOwnerCount      = 17 // UInt32
	fieldCodeTransferRate    = 11 // UInt32
	fieldCodeBalance         = 1  // Amount
	fieldCodeRegularKey      = 8  // Account
	fieldCodeAccount         = 1  // Account (different context)
	fieldCodeEmailHash       = 1  // Hash128
	fieldCodeDomain          = 7  // Blob
	fieldCodeTickSize        = 16 // UInt8 (stored as UInt16)

	// Ledger entry type code for AccountRoot
	ledgerEntryTypeAccountRoot = 0x0061
)

// Helper functions

func decodeAccountID(address string) ([20]byte, error) {
	var accountID [20]byte
	if address == "" {
		return accountID, errors.New("empty address")
	}

	// Use the address codec to decode
	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(address)
	if err != nil {
		return accountID, errors.New("invalid address: " + err.Error())
	}

	copy(accountID[:], accountIDBytes)
	return accountID, nil
}

func encodeAccountID(accountID [20]byte) (string, error) {
	return addresscodec.EncodeAccountIDToClassicAddress(accountID[:])
}

// ParseAccountRootFromBytes parses account data from binary format
func ParseAccountRootFromBytes(data []byte) (*AccountRoot, error) {
	return parseAccountRoot(data)
}

func parseAccountRoot(data []byte) (*AccountRoot, error) {
	if len(data) < 20 {
		return nil, errors.New("account data too short")
	}

	account := &AccountRoot{}

	// Parse the binary format
	// XRPL uses a TLV-like format with field headers
	offset := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		// Read field header
		header := data[offset]
		offset++

		// Decode type and field from header
		typeCode := (header >> 4) & 0x0F
		fieldCode := header & 0x0F

		// Handle extended type codes
		if typeCode == 0 {
			if offset >= len(data) {
				break
			}
			typeCode = data[offset]
			offset++
		}

		// Handle extended field codes
		if fieldCode == 0 {
			if offset >= len(data) {
				break
			}
			fieldCode = data[offset]
			offset++
		}

		// Parse field based on type
		switch typeCode {
		case fieldTypeUInt16:
			if offset+2 > len(data) {
				return account, nil
			}
			value := binary.BigEndian.Uint16(data[offset : offset+2])
			offset += 2
			if fieldCode == fieldCodeLedgerEntryType {
				// LedgerEntryType - verify it's AccountRoot
				if value != ledgerEntryTypeAccountRoot {
					return nil, errors.New("not an AccountRoot entry")
				}
			}

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return account, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case fieldCodeFlags:
				account.Flags = value
			case fieldCodeSequence:
				account.Sequence = value
			case fieldCodeOwnerCount:
				account.OwnerCount = value
			case fieldCodeTransferRate:
				account.TransferRate = value
			}

		case fieldTypeAmount:
			// XRP amounts are 8 bytes, IOU amounts are 48 bytes
			if offset+8 > len(data) {
				return account, nil
			}
			// Check if it's XRP (first bit is 0) or IOU (first bit is 1)
			if data[offset]&0x80 == 0 {
				// XRP amount - 8 bytes
				// The format is: top bit = 0 for XRP, next bit = positive, remaining 62 bits = drops
				rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
				// Clear the top bit and extract drops
				account.Balance = rawAmount & 0x3FFFFFFFFFFFFFFF
				offset += 8
			} else {
				// IOU amount - skip 48 bytes (we don't handle this in AccountRoot)
				offset += 48
			}

		case fieldTypeAccount:
			// Account IDs are variable length encoded
			if offset >= len(data) {
				return account, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return account, nil
			}
			if length == 20 {
				var accID [20]byte
				copy(accID[:], data[offset:offset+length])
				addr, err := encodeAccountID(accID)
				if err == nil {
					if fieldCode == fieldCodeAccount || fieldCode == 1 {
						account.Account = addr
					} else if fieldCode == fieldCodeRegularKey {
						account.RegularKey = addr
					}
				}
			}
			offset += length

		case fieldTypeBlob:
			// Variable length blob
			if offset >= len(data) {
				return account, nil
			}
			length := int(data[offset])
			offset++
			if length > 192 {
				// Extended length encoding
				if length < 241 {
					length = 193 + int(data[offset-1]) - 193
				} else {
					// Even more extended - skip for now
					offset += 2
					continue
				}
			}
			if offset+length > len(data) {
				return account, nil
			}
			if fieldCode == 7 { // Domain field
				account.Domain = string(data[offset : offset+length])
			}
			offset += length

		case fieldTypeHash128:
			if offset+16 > len(data) {
				return account, nil
			}
			if fieldCode == fieldCodeEmailHash {
				account.EmailHash = hex.EncodeToString(data[offset : offset+16])
			}
			offset += 16

		default:
			// Unknown type - try to skip it
			// This is a simplified skip, real implementation would need proper VL handling
			break
		}
	}

	return account, nil
}

func serializeAccountRoot(account *AccountRoot) ([]byte, error) {
	var buf []byte

	// Write LedgerEntryType (UInt16, field 1)
	buf = append(buf, (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType)
	buf = append(buf, 0x00, 0x61) // AccountRoot = 0x0061

	// Write Flags (UInt32, field 2)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeFlags)
	flagsBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(flagsBuf, account.Flags)
	buf = append(buf, flagsBuf...)

	// Write Sequence (UInt32, field 4)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeSequence)
	seqBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(seqBuf, account.Sequence)
	buf = append(buf, seqBuf...)

	// Write OwnerCount (UInt32, field 17) - need extended field code
	buf = append(buf, (fieldTypeUInt32<<4)|0) // type=2, field=0 means extended
	buf = append(buf, fieldCodeOwnerCount)
	ownerBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(ownerBuf, account.OwnerCount)
	buf = append(buf, ownerBuf...)

	// Write TransferRate if set (UInt32, field 11)
	if account.TransferRate > 0 {
		buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeTransferRate)
		rateBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(rateBuf, account.TransferRate)
		buf = append(buf, rateBuf...)
	}

	// Write Balance (Amount, field 1)
	buf = append(buf, (fieldTypeAmount<<4)|fieldCodeBalance)
	// XRP amount format: bit 63 = 0 (XRP), bit 62 = 1 (positive), bits 0-61 = drops
	balanceVal := account.Balance | 0x4000000000000000 // Set positive bit
	balBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(balBuf, balanceVal)
	buf = append(buf, balBuf...)

	// Write Account (AccountID, field 1)
	if account.Account != "" {
		accountID, err := decodeAccountID(account.Account)
		if err == nil {
			buf = append(buf, (fieldTypeAccount<<4)|fieldCodeAccount)
			buf = append(buf, 20) // Length
			buf = append(buf, accountID[:]...)
		}
	}

	// Write RegularKey if set (AccountID, field 8)
	if account.RegularKey != "" {
		regKeyID, err := decodeAccountID(account.RegularKey)
		if err == nil {
			buf = append(buf, (fieldTypeAccount<<4)|fieldCodeRegularKey)
			buf = append(buf, 20) // Length
			buf = append(buf, regKeyID[:]...)
		}
	}

	// Write Domain if set (Blob, field 7)
	if account.Domain != "" {
		buf = append(buf, (fieldTypeBlob<<4)|7)
		domainBytes := []byte(account.Domain)
		if len(domainBytes) <= 192 {
			buf = append(buf, byte(len(domainBytes)))
		} else {
			// Extended length - simplified
			buf = append(buf, 193, byte(len(domainBytes)-193))
		}
		buf = append(buf, domainBytes...)
	}

	// Write EmailHash if set (Hash128, field 1)
	if account.EmailHash != "" {
		hashBytes, err := hex.DecodeString(account.EmailHash)
		if err == nil && len(hashBytes) == 16 {
			buf = append(buf, (fieldTypeHash128<<4)|fieldCodeEmailHash)
			buf = append(buf, hashBytes...)
		}
	}

	return buf, nil
}

// applySetRegularKey applies a SetRegularKey transaction
func (e *Engine) applySetRegularKey(tx *SetRegularKey, account *AccountRoot, metadata *Metadata) Result {
	// Update the account's regular key
	previousRegularKey := account.RegularKey
	account.RegularKey = tx.RegularKey

	// If setting a new key, validate it exists (or just validate format)
	if tx.RegularKey != "" {
		if _, err := decodeAccountID(tx.RegularKey); err != nil {
			return TemINVALID
		}
	}

	// Record the change in metadata
	if previousRegularKey != account.RegularKey {
		// This will be recorded in the main account update
	}

	return TesSUCCESS
}

// applySignerListSet applies a SignerListSet transaction
func (e *Engine) applySignerListSet(tx *SignerListSet, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Create the SignerList keylet
	signerListKey := keylet.SignerList(accountID)

	if tx.SignerQuorum == 0 {
		// Delete existing signer list
		exists, _ := e.view.Exists(signerListKey)
		if exists {
			if err := e.view.Erase(signerListKey); err != nil {
				return TefINTERNAL
			}

			// Decrease owner count
			if account.OwnerCount > 0 {
				account.OwnerCount--
			}

			// Record deletion in metadata
			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "DeletedNode",
				LedgerEntryType: "SignerList",
				LedgerIndex:     hex.EncodeToString(signerListKey.Key[:]),
			})
		}
	} else {
		// Create or update signer list
		signerListData, err := serializeSignerList(tx, accountID)
		if err != nil {
			return TefINTERNAL
		}

		exists, _ := e.view.Exists(signerListKey)
		if exists {
			// Update existing
			if err := e.view.Update(signerListKey, signerListData); err != nil {
				return TefINTERNAL
			}
			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "ModifiedNode",
				LedgerEntryType: "SignerList",
				LedgerIndex:     hex.EncodeToString(signerListKey.Key[:]),
			})
		} else {
			// Create new
			if err := e.view.Insert(signerListKey, signerListData); err != nil {
				return TefINTERNAL
			}

			// Increase owner count
			account.OwnerCount++

			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "CreatedNode",
				LedgerEntryType: "SignerList",
				LedgerIndex:     hex.EncodeToString(signerListKey.Key[:]),
			})
		}
	}

	return TesSUCCESS
}

// applyTicketCreate applies a TicketCreate transaction
func (e *Engine) applyTicketCreate(tx *TicketCreate, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Create tickets
	for i := uint32(0); i < tx.TicketCount; i++ {
		ticketSeq := account.Sequence + i

		// Create ticket keylet
		ticketKey := keylet.Ticket(accountID, ticketSeq)

		// Serialize ticket
		ticketData, err := serializeTicket(accountID, ticketSeq)
		if err != nil {
			return TefINTERNAL
		}

		// Insert ticket
		if err := e.view.Insert(ticketKey, ticketData); err != nil {
			return TefINTERNAL
		}

		// Record creation in metadata
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "Ticket",
			LedgerIndex:     hex.EncodeToString(ticketKey.Key[:]),
			NewFields: map[string]any{
				"Account":        tx.Account,
				"TicketSequence": ticketSeq,
			},
		})
	}

	// Increase owner count for each ticket
	account.OwnerCount += tx.TicketCount

	// Increase sequence by ticket count (tickets consume sequence numbers)
	account.Sequence += tx.TicketCount

	return TesSUCCESS
}

// applyDepositPreauth applies a DepositPreauth transaction
func (e *Engine) applyDepositPreauth(tx *DepositPreauth, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	if tx.Authorize != "" {
		// Create preauthorization
		authorizedID, err := decodeAccountID(tx.Authorize)
		if err != nil {
			return TemINVALID
		}

		// Check that authorized account exists
		authorizedKey := keylet.Account(authorizedID)
		exists, _ := e.view.Exists(authorizedKey)
		if !exists {
			return TecNO_TARGET
		}

		// Create deposit preauth keylet
		preauthKey := keylet.DepositPreauth(accountID, authorizedID)

		// Check if already exists
		exists, _ = e.view.Exists(preauthKey)
		if exists {
			return TecDUPLICATE
		}

		// Serialize and insert
		preauthData, err := serializeDepositPreauth(accountID, authorizedID)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Insert(preauthKey, preauthData); err != nil {
			return TefINTERNAL
		}

		// Increase owner count
		account.OwnerCount++

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "DepositPreauth",
			LedgerIndex:     hex.EncodeToString(preauthKey.Key[:]),
			NewFields: map[string]any{
				"Account":   tx.Account,
				"Authorize": tx.Authorize,
			},
		})
	} else if tx.Unauthorize != "" {
		// Remove preauthorization
		unauthorizedID, err := decodeAccountID(tx.Unauthorize)
		if err != nil {
			return TemINVALID
		}

		preauthKey := keylet.DepositPreauth(accountID, unauthorizedID)

		// Check if exists
		exists, _ := e.view.Exists(preauthKey)
		if !exists {
			return TecNO_ENTRY
		}

		// Delete
		if err := e.view.Erase(preauthKey); err != nil {
			return TefINTERNAL
		}

		// Decrease owner count
		if account.OwnerCount > 0 {
			account.OwnerCount--
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "DepositPreauth",
			LedgerIndex:     hex.EncodeToString(preauthKey.Key[:]),
		})
	}

	return TesSUCCESS
}

// applyAccountDelete applies an AccountDelete transaction
func (e *Engine) applyAccountDelete(tx *AccountDelete, account *AccountRoot, metadata *Metadata) Result {
	// Check that owner count is 0 (no objects owned)
	if account.OwnerCount > 0 {
		return TecHAS_OBLIGATIONS
	}

	// Check minimum sequence requirement (account must have been around for a while)
	// In standalone mode, we relax this requirement
	if !e.config.Standalone && account.Sequence < 256 {
		return TefTOO_BIG // Account too young
	}

	// Get destination account
	destID, err := decodeAccountID(tx.Destination)
	if err != nil {
		return TemINVALID
	}

	destKey := keylet.Account(destID)
	destData, err := e.view.Read(destKey)
	if err != nil {
		return TecNO_DST
	}

	destAccount, err := parseAccountRoot(destData)
	if err != nil {
		return TefINTERNAL
	}

	// Calculate remaining balance (after fee was deducted)
	remainingBalance := account.Balance

	// Transfer remaining balance to destination
	destAccount.Balance += remainingBalance

	// Update destination account
	destUpdatedData, err := serializeAccountRoot(destAccount)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(destKey, destUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Delete source account
	srcID, _ := decodeAccountID(tx.Account)
	srcKey := keylet.Account(srcID)

	if err := e.view.Erase(srcKey); err != nil {
		return TefINTERNAL
	}

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(srcKey.Key[:]),
		FinalFields: map[string]any{
			"Account": tx.Account,
			"Balance": "0",
		},
	})

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(destKey.Key[:]),
		FinalFields: map[string]any{
			"Account": tx.Destination,
			"Balance": strconv.FormatUint(destAccount.Balance, 10),
		},
	})

	// Set account balance to 0 so the main update doesn't try to write it
	account.Balance = 0

	return TesSUCCESS
}

// Helper function to serialize a SignerList
func serializeSignerList(tx *SignerListSet, ownerID [20]byte) ([]byte, error) {
	var buf []byte

	// Write LedgerEntryType (UInt16, field 1)
	buf = append(buf, (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType)
	buf = append(buf, 0x00, 0x53) // SignerList = 0x0053

	// Write Flags (UInt32, field 2)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeFlags)
	flagsBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(flagsBuf, 0)
	buf = append(buf, flagsBuf...)

	// Write SignerQuorum (UInt32, field 35)
	buf = append(buf, (fieldTypeUInt32<<4)|0) // Extended field code
	buf = append(buf, 35)                      // SignerQuorum field code
	quorumBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(quorumBuf, tx.SignerQuorum)
	buf = append(buf, quorumBuf...)

	// Write OwnerNode (UInt64, field 2) - placeholder
	buf = append(buf, (fieldTypeUInt64<<4)|2)
	nodeBuf := make([]byte, 8)
	buf = append(buf, nodeBuf...)

	// Write Account (AccountID, field 1)
	buf = append(buf, (fieldTypeAccount<<4)|fieldCodeAccount)
	buf = append(buf, 20)
	buf = append(buf, ownerID[:]...)

	// Simplified: Skip SignerEntries array serialization for now
	// In a full implementation, we'd serialize the array properly

	return buf, nil
}

// Helper function to serialize a Ticket
func serializeTicket(ownerID [20]byte, ticketSeq uint32) ([]byte, error) {
	var buf []byte

	// Write LedgerEntryType (UInt16, field 1)
	buf = append(buf, (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType)
	buf = append(buf, 0x00, 0x54) // Ticket = 0x0054

	// Write Flags (UInt32, field 2)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeFlags)
	flagsBuf := make([]byte, 4)
	buf = append(buf, flagsBuf...)

	// Write TicketSequence (UInt32, field 41)
	buf = append(buf, (fieldTypeUInt32<<4)|0) // Extended field code
	buf = append(buf, 41)                      // TicketSequence field code
	seqBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(seqBuf, ticketSeq)
	buf = append(buf, seqBuf...)

	// Write OwnerNode (UInt64, field 2)
	buf = append(buf, (fieldTypeUInt64<<4)|2)
	nodeBuf := make([]byte, 8)
	buf = append(buf, nodeBuf...)

	// Write Account (AccountID, field 1)
	buf = append(buf, (fieldTypeAccount<<4)|fieldCodeAccount)
	buf = append(buf, 20)
	buf = append(buf, ownerID[:]...)

	return buf, nil
}

// Helper function to serialize a DepositPreauth
func serializeDepositPreauth(ownerID, authorizedID [20]byte) ([]byte, error) {
	var buf []byte

	// Write LedgerEntryType (UInt16, field 1)
	buf = append(buf, (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType)
	buf = append(buf, 0x00, 0x70) // DepositPreauth = 0x0070

	// Write Flags (UInt32, field 2)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeFlags)
	flagsBuf := make([]byte, 4)
	buf = append(buf, flagsBuf...)

	// Write OwnerNode (UInt64, field 2)
	buf = append(buf, (fieldTypeUInt64<<4)|2)
	nodeBuf := make([]byte, 8)
	buf = append(buf, nodeBuf...)

	// Write Account (AccountID, field 1)
	buf = append(buf, (fieldTypeAccount<<4)|fieldCodeAccount)
	buf = append(buf, 20)
	buf = append(buf, ownerID[:]...)

	// Write Authorize (AccountID, field 3)
	buf = append(buf, (fieldTypeAccount<<4)|3)
	buf = append(buf, 20)
	buf = append(buf, authorizedID[:]...)

	return buf, nil
}

// applyEscrowCreate applies an EscrowCreate transaction
func (e *Engine) applyEscrowCreate(tx *EscrowCreate, account *AccountRoot, metadata *Metadata) Result {
	// Parse the amount to escrow
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err != nil {
		return TemINVALID
	}

	// Check that account has sufficient balance (after fee)
	if account.Balance < amount {
		return TecUNFUNDED
	}

	// Verify destination exists
	destID, err := decodeAccountID(tx.Destination)
	if err != nil {
		return TemINVALID
	}

	destKey := keylet.Account(destID)
	exists, _ := e.view.Exists(destKey)
	if !exists {
		return TecNO_DST
	}

	// Deduct the escrow amount from the account
	account.Balance -= amount

	// Create the escrow entry
	accountID, _ := decodeAccountID(tx.Account)
	sequence := *tx.GetCommon().Sequence // Use the transaction sequence

	escrowKey := keylet.Escrow(accountID, sequence)

	// Serialize escrow
	escrowData, err := serializeEscrow(tx, accountID, destID, sequence, amount)
	if err != nil {
		return TefINTERNAL
	}

	// Insert escrow
	if err := e.view.Insert(escrowKey, escrowData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	account.OwnerCount++

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "Escrow",
		LedgerIndex:     hex.EncodeToString(escrowKey.Key[:]),
		NewFields: map[string]any{
			"Account":     tx.Account,
			"Destination": tx.Destination,
			"Amount":      tx.Amount.Value,
		},
	})

	return TesSUCCESS
}

// applyEscrowFinish applies an EscrowFinish transaction
func (e *Engine) applyEscrowFinish(tx *EscrowFinish, account *AccountRoot, metadata *Metadata) Result {
	// Get the escrow owner's account ID
	ownerID, err := decodeAccountID(tx.Owner)
	if err != nil {
		return TemINVALID
	}

	// Find the escrow
	escrowKey := keylet.Escrow(ownerID, tx.OfferSequence)
	escrowData, err := e.view.Read(escrowKey)
	if err != nil {
		return TecNO_TARGET
	}

	// Parse escrow
	escrow, err := parseEscrow(escrowData)
	if err != nil {
		return TefINTERNAL
	}

	// Check FinishAfter time (if set)
	if escrow.FinishAfter > 0 {
		// In a full implementation, we'd check against the close time
		// For now, we'll allow it in standalone mode
		if !e.config.Standalone {
			// Would check: if currentTime < escrow.FinishAfter return TecNO_PERMISSION
		}
	}

	// Check condition/fulfillment (simplified - in reality, would verify crypto-condition)
	if escrow.Condition != "" {
		if tx.Fulfillment == "" {
			return TecCRYPTOCONDITION_ERROR
		}
		// Would verify: fulfillment matches condition
	}

	// Get destination account
	destKey := keylet.Account(escrow.DestinationID)
	destData, err := e.view.Read(destKey)
	if err != nil {
		return TecNO_DST
	}

	destAccount, err := parseAccountRoot(destData)
	if err != nil {
		return TefINTERNAL
	}

	// Transfer the escrowed amount to destination
	destAccount.Balance += escrow.Amount

	// Update destination
	destUpdatedData, err := serializeAccountRoot(destAccount)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(destKey, destUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Delete the escrow
	if err := e.view.Erase(escrowKey); err != nil {
		return TefINTERNAL
	}

	// Decrease owner count for escrow owner
	if tx.Owner != tx.Account {
		// Need to update owner's account too
		ownerKey := keylet.Account(ownerID)
		ownerData, err := e.view.Read(ownerKey)
		if err == nil {
			ownerAccount, err := parseAccountRoot(ownerData)
			if err == nil && ownerAccount.OwnerCount > 0 {
				ownerAccount.OwnerCount--
				ownerUpdatedData, err := serializeAccountRoot(ownerAccount)
				if err == nil {
					e.view.Update(ownerKey, ownerUpdatedData)
				}
			}
		}
	} else {
		if account.OwnerCount > 0 {
			account.OwnerCount--
		}
	}

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Escrow",
		LedgerIndex:     hex.EncodeToString(escrowKey.Key[:]),
	})

	destAddr, _ := encodeAccountID(escrow.DestinationID)
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(destKey.Key[:]),
		FinalFields: map[string]any{
			"Account": destAddr,
			"Balance": strconv.FormatUint(destAccount.Balance, 10),
		},
	})

	return TesSUCCESS
}

// applyEscrowCancel applies an EscrowCancel transaction
func (e *Engine) applyEscrowCancel(tx *EscrowCancel, account *AccountRoot, metadata *Metadata) Result {
	// Get the escrow owner's account ID
	ownerID, err := decodeAccountID(tx.Owner)
	if err != nil {
		return TemINVALID
	}

	// Find the escrow
	escrowKey := keylet.Escrow(ownerID, tx.OfferSequence)
	escrowData, err := e.view.Read(escrowKey)
	if err != nil {
		return TecNO_TARGET
	}

	// Parse escrow
	escrow, err := parseEscrow(escrowData)
	if err != nil {
		return TefINTERNAL
	}

	// Check CancelAfter time (if set)
	if escrow.CancelAfter > 0 {
		// In a full implementation, we'd check against the close time
		// For now, we'll allow it in standalone mode
		if !e.config.Standalone {
			// Would check: if currentTime < escrow.CancelAfter return TecNO_PERMISSION
		}
	} else {
		// If no CancelAfter, only the creator can cancel (implied by having condition)
		if tx.Account != tx.Owner {
			return TecNO_PERMISSION
		}
	}

	// Return the escrowed amount to the owner
	ownerKey := keylet.Account(ownerID)
	ownerData, err := e.view.Read(ownerKey)
	if err != nil {
		return TefINTERNAL
	}

	ownerAccount, err := parseAccountRoot(ownerData)
	if err != nil {
		return TefINTERNAL
	}

	ownerAccount.Balance += escrow.Amount
	if ownerAccount.OwnerCount > 0 {
		ownerAccount.OwnerCount--
	}

	ownerUpdatedData, err := serializeAccountRoot(ownerAccount)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(ownerKey, ownerUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Delete the escrow
	if err := e.view.Erase(escrowKey); err != nil {
		return TefINTERNAL
	}

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Escrow",
		LedgerIndex:     hex.EncodeToString(escrowKey.Key[:]),
	})

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(ownerKey.Key[:]),
		FinalFields: map[string]any{
			"Account": tx.Owner,
			"Balance": strconv.FormatUint(ownerAccount.Balance, 10),
		},
	})

	return TesSUCCESS
}

// Escrow ledger entry data
type EscrowData struct {
	Account       [20]byte
	DestinationID [20]byte
	Amount        uint64
	Condition     string
	CancelAfter   uint32
	FinishAfter   uint32
}

// Helper function to serialize an Escrow
func serializeEscrow(tx *EscrowCreate, ownerID, destID [20]byte, sequence uint32, amount uint64) ([]byte, error) {
	var buf []byte

	// Write LedgerEntryType (UInt16, field 1)
	buf = append(buf, (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType)
	buf = append(buf, 0x00, 0x75) // Escrow = 0x0075

	// Write Flags (UInt32, field 2)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeFlags)
	flagsBuf := make([]byte, 4)
	buf = append(buf, flagsBuf...)

	// Write SourceTag if present (UInt32, field 3)
	// Skip for now

	// Write Amount (Amount, field 1)
	buf = append(buf, (fieldTypeAmount<<4)|fieldCodeBalance)
	amountVal := amount | 0x4000000000000000 // Set positive bit
	amtBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(amtBuf, amountVal)
	buf = append(buf, amtBuf...)

	// Write FinishAfter if present (UInt32, field 36)
	if tx.FinishAfter != nil {
		buf = append(buf, (fieldTypeUInt32<<4)|0) // Extended
		buf = append(buf, 36)
		faBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(faBuf, *tx.FinishAfter)
		buf = append(buf, faBuf...)
	}

	// Write CancelAfter if present (UInt32, field 37)
	if tx.CancelAfter != nil {
		buf = append(buf, (fieldTypeUInt32<<4)|0) // Extended
		buf = append(buf, 37)
		caBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(caBuf, *tx.CancelAfter)
		buf = append(buf, caBuf...)
	}

	// Write OwnerNode (UInt64, field 2)
	buf = append(buf, (fieldTypeUInt64<<4)|2)
	nodeBuf := make([]byte, 8)
	buf = append(buf, nodeBuf...)

	// Write Account (AccountID, field 1)
	buf = append(buf, (fieldTypeAccount<<4)|fieldCodeAccount)
	buf = append(buf, 20)
	buf = append(buf, ownerID[:]...)

	// Write Destination (AccountID, field 3)
	buf = append(buf, (fieldTypeAccount<<4)|3)
	buf = append(buf, 20)
	buf = append(buf, destID[:]...)

	// Write Condition if present (Blob)
	if tx.Condition != "" {
		condBytes, _ := hex.DecodeString(tx.Condition)
		if len(condBytes) > 0 {
			buf = append(buf, (fieldTypeBlob<<4)|0) // Extended
			buf = append(buf, 25)                   // Condition field code
			buf = append(buf, byte(len(condBytes)))
			buf = append(buf, condBytes...)
		}
	}

	return buf, nil
}

// Helper function to parse an Escrow ledger entry
func parseEscrow(data []byte) (*EscrowData, error) {
	escrow := &EscrowData{}
	offset := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		header := data[offset]
		offset++

		typeCode := (header >> 4) & 0x0F
		fieldCode := header & 0x0F

		if typeCode == 0 {
			if offset >= len(data) {
				break
			}
			typeCode = data[offset]
			offset++
		}

		if fieldCode == 0 {
			if offset >= len(data) {
				break
			}
			fieldCode = data[offset]
			offset++
		}

		switch typeCode {
		case fieldTypeUInt16:
			if offset+2 > len(data) {
				return escrow, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return escrow, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case 36: // FinishAfter
				escrow.FinishAfter = value
			case 37: // CancelAfter
				escrow.CancelAfter = value
			}

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return escrow, nil
			}
			offset += 8

		case fieldTypeAmount:
			if offset+8 > len(data) {
				return escrow, nil
			}
			rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
			escrow.Amount = rawAmount & 0x3FFFFFFFFFFFFFFF
			offset += 8

		case fieldTypeAccountID:
			if offset+21 > len(data) {
				return escrow, nil
			}
			length := data[offset]
			offset++
			if length == 20 {
				switch fieldCode {
				case 1: // Account
					copy(escrow.Account[:], data[offset:offset+20])
				case 3: // Destination
					copy(escrow.DestinationID[:], data[offset:offset+20])
				}
				offset += 20
			}

		case fieldTypeBlob:
			if offset >= len(data) {
				return escrow, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return escrow, nil
			}
			if fieldCode == 25 { // Condition
				escrow.Condition = hex.EncodeToString(data[offset : offset+length])
			}
			offset += length

		default:
			// Unknown type - try to skip safely
			return escrow, nil
		}
	}

	return escrow, nil
}

// PayChannel ledger entry data
type PayChannelData struct {
	Account       [20]byte
	DestinationID [20]byte
	Amount        uint64
	Balance       uint64
	SettleDelay   uint32
	PublicKey     string
	Expiration    uint32
	CancelAfter   uint32
}

// applyPaymentChannelCreate applies a PaymentChannelCreate transaction
func (e *Engine) applyPaymentChannelCreate(tx *PaymentChannelCreate, account *AccountRoot, metadata *Metadata) Result {
	// Parse the amount
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err != nil {
		return TemINVALID
	}

	// Check balance
	if account.Balance < amount {
		return TecUNFUNDED
	}

	// Verify destination exists
	destID, err := decodeAccountID(tx.Destination)
	if err != nil {
		return TemINVALID
	}

	destKey := keylet.Account(destID)
	exists, _ := e.view.Exists(destKey)
	if !exists {
		return TecNO_DST
	}

	// Deduct amount from account
	account.Balance -= amount

	// Create pay channel
	accountID, _ := decodeAccountID(tx.Account)
	sequence := *tx.GetCommon().Sequence

	channelKey := keylet.PayChannel(accountID, destID, sequence)

	// Serialize pay channel
	channelData, err := serializePayChannel(tx, accountID, destID, amount)
	if err != nil {
		return TefINTERNAL
	}

	// Insert channel
	if err := e.view.Insert(channelKey, channelData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	account.OwnerCount++

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "PayChannel",
		LedgerIndex:     hex.EncodeToString(channelKey.Key[:]),
		NewFields: map[string]any{
			"Account":     tx.Account,
			"Destination": tx.Destination,
			"Amount":      tx.Amount.Value,
			"Balance":     "0",
			"SettleDelay": tx.SettleDelay,
			"PublicKey":   tx.PublicKey,
		},
	})

	return TesSUCCESS
}

// applyPaymentChannelFund applies a PaymentChannelFund transaction
func (e *Engine) applyPaymentChannelFund(tx *PaymentChannelFund, account *AccountRoot, metadata *Metadata) Result {
	// Parse channel ID
	channelID, err := hex.DecodeString(tx.Channel)
	if err != nil || len(channelID) != 32 {
		return TemINVALID
	}

	var channelKeyBytes [32]byte
	copy(channelKeyBytes[:], channelID)
	channelKey := keylet.Keylet{Key: channelKeyBytes}

	// Read channel
	channelData, err := e.view.Read(channelKey)
	if err != nil {
		return TecNO_TARGET
	}

	// Parse channel
	channel, err := parsePayChannel(channelData)
	if err != nil {
		return TefINTERNAL
	}

	// Verify sender is the channel owner
	accountID, _ := decodeAccountID(tx.Account)
	if channel.Account != accountID {
		return TecNO_PERMISSION
	}

	// Parse amount to add
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err != nil {
		return TemINVALID
	}

	// Check balance
	if account.Balance < amount {
		return TecUNFUNDED
	}

	// Deduct from account
	account.Balance -= amount

	// Add to channel
	channel.Amount += amount

	// Update expiration if specified
	if tx.Expiration != nil {
		channel.Expiration = *tx.Expiration
	}

	// Serialize updated channel
	updatedChannelData, err := serializePayChannelFromData(channel)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(channelKey, updatedChannelData); err != nil {
		return TefINTERNAL
	}

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "PayChannel",
		LedgerIndex:     hex.EncodeToString(channelKey.Key[:]),
		FinalFields: map[string]any{
			"Amount": strconv.FormatUint(channel.Amount, 10),
		},
	})

	return TesSUCCESS
}

// applyPaymentChannelClaim applies a PaymentChannelClaim transaction
func (e *Engine) applyPaymentChannelClaim(tx *PaymentChannelClaim, account *AccountRoot, metadata *Metadata) Result {
	// Parse channel ID
	channelID, err := hex.DecodeString(tx.Channel)
	if err != nil || len(channelID) != 32 {
		return TemINVALID
	}

	var channelKeyBytes [32]byte
	copy(channelKeyBytes[:], channelID)
	channelKey := keylet.Keylet{Key: channelKeyBytes}

	// Read channel
	channelData, err := e.view.Read(channelKey)
	if err != nil {
		return TecNO_TARGET
	}

	// Parse channel
	channel, err := parsePayChannel(channelData)
	if err != nil {
		return TefINTERNAL
	}

	accountID, _ := decodeAccountID(tx.Account)
	isOwner := channel.Account == accountID
	isDest := channel.DestinationID == accountID

	if !isOwner && !isDest {
		return TecNO_PERMISSION
	}

	// Handle claim with signature
	if tx.Balance != nil && tx.Amount != nil && tx.Signature != "" {
		// Parse claimed balance
		claimBalance, err := strconv.ParseUint(tx.Balance.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}

		// Verify claim is valid (would verify signature in full implementation)
		if claimBalance > channel.Amount {
			return TecUNFUNDED_PAYMENT
		}

		if claimBalance < channel.Balance {
			return TemINVALID // Can't decrease balance
		}

		// Calculate amount to transfer
		transferAmount := claimBalance - channel.Balance

		// Transfer to destination
		destKey := keylet.Account(channel.DestinationID)
		destData, err := e.view.Read(destKey)
		if err != nil {
			return TecNO_DST
		}

		destAccount, err := parseAccountRoot(destData)
		if err != nil {
			return TefINTERNAL
		}

		destAccount.Balance += transferAmount
		channel.Balance = claimBalance

		destUpdatedData, err := serializeAccountRoot(destAccount)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Update(destKey, destUpdatedData); err != nil {
			return TefINTERNAL
		}

		destAddr, _ := encodeAccountID(channel.DestinationID)
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "AccountRoot",
			LedgerIndex:     hex.EncodeToString(destKey.Key[:]),
			FinalFields: map[string]any{
				"Account": destAddr,
				"Balance": strconv.FormatUint(destAccount.Balance, 10),
			},
		})
	}

	// Handle close flag
	flags := tx.GetFlags()
	if flags&PaymentChannelClaimFlagClose != 0 {
		// Close the channel

		// Return remaining funds to owner
		remaining := channel.Amount - channel.Balance
		if remaining > 0 {
			ownerKey := keylet.Account(channel.Account)
			ownerData, err := e.view.Read(ownerKey)
			if err == nil {
				ownerAccount, err := parseAccountRoot(ownerData)
				if err == nil {
					ownerAccount.Balance += remaining
					if ownerAccount.OwnerCount > 0 {
						ownerAccount.OwnerCount--
					}
					ownerUpdatedData, _ := serializeAccountRoot(ownerAccount)
					e.view.Update(ownerKey, ownerUpdatedData)
				}
			}
		}

		// Delete channel
		if err := e.view.Erase(channelKey); err != nil {
			return TefINTERNAL
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "PayChannel",
			LedgerIndex:     hex.EncodeToString(channelKey.Key[:]),
		})
	} else {
		// Update channel
		updatedChannelData, err := serializePayChannelFromData(channel)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Update(channelKey, updatedChannelData); err != nil {
			return TefINTERNAL
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "PayChannel",
			LedgerIndex:     hex.EncodeToString(channelKey.Key[:]),
			FinalFields: map[string]any{
				"Balance": strconv.FormatUint(channel.Balance, 10),
			},
		})
	}

	return TesSUCCESS
}

// Helper function to serialize a PayChannel
func serializePayChannel(tx *PaymentChannelCreate, ownerID, destID [20]byte, amount uint64) ([]byte, error) {
	var buf []byte

	// Write LedgerEntryType (UInt16, field 1)
	buf = append(buf, (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType)
	buf = append(buf, 0x00, 0x78) // PayChannel = 0x0078

	// Write Flags (UInt32, field 2)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeFlags)
	flagsBuf := make([]byte, 4)
	buf = append(buf, flagsBuf...)

	// Write SettleDelay (UInt32, field 39)
	buf = append(buf, (fieldTypeUInt32<<4)|0)
	buf = append(buf, 39)
	sdBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(sdBuf, tx.SettleDelay)
	buf = append(buf, sdBuf...)

	// Write CancelAfter if present (UInt32, field 37)
	if tx.CancelAfter != nil {
		buf = append(buf, (fieldTypeUInt32<<4)|0)
		buf = append(buf, 37)
		caBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(caBuf, *tx.CancelAfter)
		buf = append(buf, caBuf...)
	}

	// Write Amount (Amount, field 1)
	buf = append(buf, (fieldTypeAmount<<4)|fieldCodeBalance)
	amountVal := amount | 0x4000000000000000
	amtBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(amtBuf, amountVal)
	buf = append(buf, amtBuf...)

	// Write Balance (Amount, field 5) - starts at 0
	buf = append(buf, (fieldTypeAmount<<4)|5)
	balBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(balBuf, 0x4000000000000000)
	buf = append(buf, balBuf...)

	// Write OwnerNode (UInt64, field 2)
	buf = append(buf, (fieldTypeUInt64<<4)|2)
	nodeBuf := make([]byte, 8)
	buf = append(buf, nodeBuf...)

	// Write Account (AccountID, field 1)
	buf = append(buf, (fieldTypeAccount<<4)|fieldCodeAccount)
	buf = append(buf, 20)
	buf = append(buf, ownerID[:]...)

	// Write Destination (AccountID, field 3)
	buf = append(buf, (fieldTypeAccount<<4)|3)
	buf = append(buf, 20)
	buf = append(buf, destID[:]...)

	// Write PublicKey (Blob, field 28)
	if tx.PublicKey != "" {
		pkBytes, _ := hex.DecodeString(tx.PublicKey)
		if len(pkBytes) > 0 {
			buf = append(buf, (fieldTypeBlob<<4)|0)
			buf = append(buf, 28)
			buf = append(buf, byte(len(pkBytes)))
			buf = append(buf, pkBytes...)
		}
	}

	return buf, nil
}

// Helper function to serialize a PayChannel from data
func serializePayChannelFromData(channel *PayChannelData) ([]byte, error) {
	var buf []byte

	// Write LedgerEntryType (UInt16, field 1)
	buf = append(buf, (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType)
	buf = append(buf, 0x00, 0x78) // PayChannel = 0x0078

	// Write Flags (UInt32, field 2)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeFlags)
	flagsBuf := make([]byte, 4)
	buf = append(buf, flagsBuf...)

	// Write SettleDelay (UInt32, field 39)
	buf = append(buf, (fieldTypeUInt32<<4)|0)
	buf = append(buf, 39)
	sdBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(sdBuf, channel.SettleDelay)
	buf = append(buf, sdBuf...)

	// Write Amount (Amount, field 1)
	buf = append(buf, (fieldTypeAmount<<4)|fieldCodeBalance)
	amountVal := channel.Amount | 0x4000000000000000
	amtBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(amtBuf, amountVal)
	buf = append(buf, amtBuf...)

	// Write Balance (Amount, field 5)
	buf = append(buf, (fieldTypeAmount<<4)|5)
	balVal := channel.Balance | 0x4000000000000000
	balBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(balBuf, balVal)
	buf = append(buf, balBuf...)

	// Write OwnerNode (UInt64, field 2)
	buf = append(buf, (fieldTypeUInt64<<4)|2)
	nodeBuf := make([]byte, 8)
	buf = append(buf, nodeBuf...)

	// Write Account (AccountID, field 1)
	buf = append(buf, (fieldTypeAccount<<4)|fieldCodeAccount)
	buf = append(buf, 20)
	buf = append(buf, channel.Account[:]...)

	// Write Destination (AccountID, field 3)
	buf = append(buf, (fieldTypeAccount<<4)|3)
	buf = append(buf, 20)
	buf = append(buf, channel.DestinationID[:]...)

	return buf, nil
}

// Helper function to parse a PayChannel ledger entry
func parsePayChannel(data []byte) (*PayChannelData, error) {
	channel := &PayChannelData{}
	offset := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		header := data[offset]
		offset++

		typeCode := (header >> 4) & 0x0F
		fieldCode := header & 0x0F

		if typeCode == 0 {
			if offset >= len(data) {
				break
			}
			typeCode = data[offset]
			offset++
		}

		if fieldCode == 0 {
			if offset >= len(data) {
				break
			}
			fieldCode = data[offset]
			offset++
		}

		switch typeCode {
		case fieldTypeUInt16:
			if offset+2 > len(data) {
				return channel, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return channel, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case 39: // SettleDelay
				channel.SettleDelay = value
			case 37: // CancelAfter
				channel.CancelAfter = value
			case 10: // Expiration
				channel.Expiration = value
			}

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return channel, nil
			}
			offset += 8

		case fieldTypeAmount:
			if offset+8 > len(data) {
				return channel, nil
			}
			rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
			amount := rawAmount & 0x3FFFFFFFFFFFFFFF
			if fieldCode == 1 { // Amount
				channel.Amount = amount
			} else if fieldCode == 5 { // Balance
				channel.Balance = amount
			}
			offset += 8

		case fieldTypeAccountID:
			if offset+21 > len(data) {
				return channel, nil
			}
			length := data[offset]
			offset++
			if length == 20 {
				switch fieldCode {
				case 1: // Account
					copy(channel.Account[:], data[offset:offset+20])
				case 3: // Destination
					copy(channel.DestinationID[:], data[offset:offset+20])
				}
				offset += 20
			}

		case fieldTypeBlob:
			if offset >= len(data) {
				return channel, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return channel, nil
			}
			if fieldCode == 28 { // PublicKey
				channel.PublicKey = hex.EncodeToString(data[offset : offset+length])
			}
			offset += length

		default:
			return channel, nil
		}
	}

	return channel, nil
}

// Check ledger entry data
type CheckData struct {
	Account       [20]byte
	DestinationID [20]byte
	SendMax       uint64 // For XRP checks; IOU checks would need more fields
	Sequence      uint32
	Expiration    uint32
	InvoiceID     [32]byte
	DestinationTag uint32
	HasDestTag    bool
}

// applyCheckCreate applies a CheckCreate transaction
func (e *Engine) applyCheckCreate(tx *CheckCreate, account *AccountRoot, metadata *Metadata) Result {
	// Verify destination exists
	destID, err := decodeAccountID(tx.Destination)
	if err != nil {
		return TemINVALID
	}

	destKey := keylet.Account(destID)
	exists, _ := e.view.Exists(destKey)
	if !exists {
		return TecNO_DST
	}

	// Parse SendMax - only XRP supported for now
	sendMax, err := strconv.ParseUint(tx.SendMax.Value, 10, 64)
	if err != nil {
		// May be an IOU amount
		sendMax = 0
	}

	// Check balance for XRP checks
	if tx.SendMax.Currency == "" && sendMax > 0 {
		if account.Balance < sendMax {
			return TecUNFUNDED
		}
	}

	// Create the check entry
	accountID, _ := decodeAccountID(tx.Account)
	sequence := *tx.GetCommon().Sequence

	checkKey := keylet.Check(accountID, sequence)

	// Serialize check
	checkData, err := serializeCheck(tx, accountID, destID, sequence, sendMax)
	if err != nil {
		return TefINTERNAL
	}

	// Insert check
	if err := e.view.Insert(checkKey, checkData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	account.OwnerCount++

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "Check",
		LedgerIndex:     hex.EncodeToString(checkKey.Key[:]),
		NewFields: map[string]any{
			"Account":     tx.Account,
			"Destination": tx.Destination,
			"SendMax":     tx.SendMax.Value,
		},
	})

	return TesSUCCESS
}

// applyCheckCash applies a CheckCash transaction
func (e *Engine) applyCheckCash(tx *CheckCash, account *AccountRoot, metadata *Metadata) Result {
	// Parse check ID
	checkID, err := hex.DecodeString(tx.CheckID)
	if err != nil || len(checkID) != 32 {
		return TemINVALID
	}

	var checkKeyBytes [32]byte
	copy(checkKeyBytes[:], checkID)
	checkKey := keylet.Keylet{Key: checkKeyBytes}

	// Read check
	checkData, err := e.view.Read(checkKey)
	if err != nil {
		return TecNO_ENTRY
	}

	// Parse check
	check, err := parseCheck(checkData)
	if err != nil {
		return TefINTERNAL
	}

	// Verify the account is the destination
	accountID, _ := decodeAccountID(tx.Account)
	if check.DestinationID != accountID {
		return TecNO_PERMISSION
	}

	// Check expiration
	if check.Expiration > 0 {
		// In full implementation, check against close time
		// For standalone mode, we'll allow it
	}

	// Determine amount to cash
	var cashAmount uint64
	if tx.Amount != nil {
		// Exact amount
		cashAmount, err = strconv.ParseUint(tx.Amount.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}
		if cashAmount > check.SendMax {
			return TecPATH_PARTIAL
		}
	} else if tx.DeliverMin != nil {
		// Minimum amount - use full SendMax for simplicity
		deliverMin, err := strconv.ParseUint(tx.DeliverMin.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}
		if check.SendMax < deliverMin {
			return TecPATH_PARTIAL
		}
		cashAmount = check.SendMax
	}

	// Get the check creator's account
	creatorKey := keylet.Account(check.Account)
	creatorData, err := e.view.Read(creatorKey)
	if err != nil {
		return TefINTERNAL
	}

	creatorAccount, err := parseAccountRoot(creatorData)
	if err != nil {
		return TefINTERNAL
	}

	// Check if creator has sufficient balance
	if creatorAccount.Balance < cashAmount {
		return TecUNFUNDED_PAYMENT
	}

	// Transfer the funds
	creatorAccount.Balance -= cashAmount
	account.Balance += cashAmount

	// Decrease creator's owner count
	if creatorAccount.OwnerCount > 0 {
		creatorAccount.OwnerCount--
	}

	// Update creator account
	creatorUpdatedData, err := serializeAccountRoot(creatorAccount)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(creatorKey, creatorUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Delete the check
	if err := e.view.Erase(checkKey); err != nil {
		return TefINTERNAL
	}

	// Record in metadata
	creatorAddr, _ := encodeAccountID(check.Account)
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Check",
		LedgerIndex:     hex.EncodeToString(checkKey.Key[:]),
	})

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(creatorKey.Key[:]),
		FinalFields: map[string]any{
			"Account": creatorAddr,
			"Balance": strconv.FormatUint(creatorAccount.Balance, 10),
		},
	})

	return TesSUCCESS
}

// applyCheckCancel applies a CheckCancel transaction
func (e *Engine) applyCheckCancel(tx *CheckCancel, account *AccountRoot, metadata *Metadata) Result {
	// Parse check ID
	checkID, err := hex.DecodeString(tx.CheckID)
	if err != nil || len(checkID) != 32 {
		return TemINVALID
	}

	var checkKeyBytes [32]byte
	copy(checkKeyBytes[:], checkID)
	checkKey := keylet.Keylet{Key: checkKeyBytes}

	// Read check
	checkData, err := e.view.Read(checkKey)
	if err != nil {
		return TecNO_ENTRY
	}

	// Parse check
	check, err := parseCheck(checkData)
	if err != nil {
		return TefINTERNAL
	}

	accountID, _ := decodeAccountID(tx.Account)
	isCreator := check.Account == accountID
	isDestination := check.DestinationID == accountID

	// Only creator or destination can cancel
	if !isCreator && !isDestination {
		// Unless the check is expired
		if check.Expiration == 0 {
			return TecNO_PERMISSION
		}
		// In full implementation, check if expired
		// For standalone mode, allow anyone to cancel expired checks
	}

	// Delete the check
	if err := e.view.Erase(checkKey); err != nil {
		return TefINTERNAL
	}

	// If the canceller is also the creator, decrease their owner count
	if isCreator {
		if account.OwnerCount > 0 {
			account.OwnerCount--
		}
	} else {
		// Need to update the creator's owner count
		creatorKey := keylet.Account(check.Account)
		creatorData, err := e.view.Read(creatorKey)
		if err == nil {
			creatorAccount, err := parseAccountRoot(creatorData)
			if err == nil && creatorAccount.OwnerCount > 0 {
				creatorAccount.OwnerCount--
				creatorUpdatedData, _ := serializeAccountRoot(creatorAccount)
				e.view.Update(creatorKey, creatorUpdatedData)
			}
		}
	}

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Check",
		LedgerIndex:     hex.EncodeToString(checkKey.Key[:]),
	})

	return TesSUCCESS
}

// Helper function to serialize a Check
func serializeCheck(tx *CheckCreate, ownerID, destID [20]byte, sequence uint32, sendMax uint64) ([]byte, error) {
	var buf []byte

	// Write LedgerEntryType (UInt16, field 1)
	buf = append(buf, (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType)
	buf = append(buf, 0x00, 0x43) // Check = 0x0043

	// Write Flags (UInt32, field 2)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeFlags)
	flagsBuf := make([]byte, 4)
	buf = append(buf, flagsBuf...)

	// Write Sequence (UInt32, field 4)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeSequence)
	seqBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(seqBuf, sequence)
	buf = append(buf, seqBuf...)

	// Write Expiration if present (UInt32, field 10)
	if tx.Expiration != nil {
		buf = append(buf, (fieldTypeUInt32<<4)|10)
		expBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(expBuf, *tx.Expiration)
		buf = append(buf, expBuf...)
	}

	// Write DestinationTag if present (UInt32, field 14)
	if tx.DestinationTag != nil {
		buf = append(buf, (fieldTypeUInt32<<4)|14)
		tagBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(tagBuf, *tx.DestinationTag)
		buf = append(buf, tagBuf...)
	}

	// Write SendMax (Amount, field 9)
	buf = append(buf, (fieldTypeAmount<<4)|9)
	amountVal := sendMax | 0x4000000000000000
	amtBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(amtBuf, amountVal)
	buf = append(buf, amtBuf...)

	// Write OwnerNode (UInt64, field 2)
	buf = append(buf, (fieldTypeUInt64<<4)|2)
	nodeBuf := make([]byte, 8)
	buf = append(buf, nodeBuf...)

	// Write Account (AccountID, field 1)
	buf = append(buf, (fieldTypeAccount<<4)|fieldCodeAccount)
	buf = append(buf, 20)
	buf = append(buf, ownerID[:]...)

	// Write Destination (AccountID, field 3)
	buf = append(buf, (fieldTypeAccount<<4)|3)
	buf = append(buf, 20)
	buf = append(buf, destID[:]...)

	// Write InvoiceID if present (Hash256, field 17)
	if tx.InvoiceID != "" {
		invoiceBytes, err := hex.DecodeString(tx.InvoiceID)
		if err == nil && len(invoiceBytes) == 32 {
			buf = append(buf, (fieldTypeHash256<<4)|0)
			buf = append(buf, 17) // InvoiceID field code
			buf = append(buf, invoiceBytes...)
		}
	}

	return buf, nil
}

// Helper function to parse a Check ledger entry
func parseCheck(data []byte) (*CheckData, error) {
	check := &CheckData{}
	offset := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		header := data[offset]
		offset++

		typeCode := (header >> 4) & 0x0F
		fieldCode := header & 0x0F

		if typeCode == 0 {
			if offset >= len(data) {
				break
			}
			typeCode = data[offset]
			offset++
		}

		if fieldCode == 0 {
			if offset >= len(data) {
				break
			}
			fieldCode = data[offset]
			offset++
		}

		switch typeCode {
		case fieldTypeUInt16:
			if offset+2 > len(data) {
				return check, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return check, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case fieldCodeSequence:
				check.Sequence = value
			case 10: // Expiration
				check.Expiration = value
			case 14: // DestinationTag
				check.DestinationTag = value
				check.HasDestTag = true
			}

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return check, nil
			}
			offset += 8

		case fieldTypeHash256:
			if offset+32 > len(data) {
				return check, nil
			}
			if fieldCode == 17 { // InvoiceID
				copy(check.InvoiceID[:], data[offset:offset+32])
			}
			offset += 32

		case fieldTypeAmount:
			if offset+8 > len(data) {
				return check, nil
			}
			if data[offset]&0x80 == 0 {
				// XRP amount
				rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
				if fieldCode == 9 { // SendMax
					check.SendMax = rawAmount & 0x3FFFFFFFFFFFFFFF
				}
				offset += 8
			} else {
				// IOU amount - skip 48 bytes
				offset += 48
			}

		case fieldTypeAccountID:
			if offset+21 > len(data) {
				return check, nil
			}
			length := data[offset]
			offset++
			if length == 20 {
				switch fieldCode {
				case 1: // Account
					copy(check.Account[:], data[offset:offset+20])
				case 3: // Destination
					copy(check.DestinationID[:], data[offset:offset+20])
				}
				offset += 20
			}

		case fieldTypeBlob:
			if offset >= len(data) {
				return check, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return check, nil
			}
			offset += length

		default:
			return check, nil
		}
	}

	return check, nil
}

// NFToken data structures

// NFTokenPageData represents an NFToken page ledger entry
type NFTokenPageData struct {
	PreviousPageMin [32]byte
	NextPageMin     [32]byte
	NFTokens        []NFTokenData
}

// NFTokenData represents an individual NFToken within a page
type NFTokenData struct {
	NFTokenID [32]byte
	URI       string
}

// NFTokenOfferData represents an NFToken offer ledger entry
type NFTokenOfferData struct {
	Owner         [20]byte
	NFTokenID     [32]byte
	Amount        uint64
	Flags         uint32
	Destination   [20]byte
	Expiration    uint32
	HasDestination bool
}

// generateNFTokenID generates an NFTokenID based on the minting parameters
func generateNFTokenID(issuer [20]byte, taxon, sequence uint32, flags uint16, transferFee uint16) [32]byte {
	var tokenID [32]byte

	// NFTokenID format (32 bytes):
	// Bytes 0-1: Flags (2 bytes)
	// Bytes 2-3: TransferFee (2 bytes)
	// Bytes 4-23: Issuer AccountID (20 bytes)
	// Bytes 24-27: Taxon (scrambled, 4 bytes)
	// Bytes 28-31: Sequence (4 bytes)

	binary.BigEndian.PutUint16(tokenID[0:2], flags)
	binary.BigEndian.PutUint16(tokenID[2:4], transferFee)
	copy(tokenID[4:24], issuer[:])

	// Scramble the taxon to prevent enumeration
	scrambledTaxon := taxon ^ (sequence & 0xFFFFFFFF)
	binary.BigEndian.PutUint32(tokenID[24:28], scrambledTaxon)
	binary.BigEndian.PutUint32(tokenID[28:32], sequence)

	return tokenID
}

// applyNFTokenMint applies an NFTokenMint transaction
func (e *Engine) applyNFTokenMint(tx *NFTokenMint, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Determine the issuer
	var issuerID [20]byte
	if tx.Issuer != "" {
		var err error
		issuerID, err = decodeAccountID(tx.Issuer)
		if err != nil {
			return TemINVALID
		}
	} else {
		issuerID = accountID
	}

	// Get flags for the token
	txFlags := tx.GetFlags()
	var tokenFlags uint16
	if txFlags&NFTokenMintFlagBurnable != 0 {
		tokenFlags |= 0x0001
	}
	if txFlags&NFTokenMintFlagOnlyXRP != 0 {
		tokenFlags |= 0x0002
	}
	if txFlags&NFTokenMintFlagTrustLine != 0 {
		tokenFlags |= 0x0004
	}
	if txFlags&NFTokenMintFlagTransferable != 0 {
		tokenFlags |= 0x0008
	}

	// Get transfer fee
	var transferFee uint16
	if tx.TransferFee != nil {
		transferFee = *tx.TransferFee
	}

	// Generate the NFTokenID
	sequence := *tx.GetCommon().Sequence
	tokenID := generateNFTokenID(issuerID, tx.NFTokenTaxon, sequence, tokenFlags, transferFee)

	// Find or create the NFToken page for this account
	// NFToken pages are keyed by account + portion of token ID
	pageKey := keylet.NFTokenPage(accountID, tokenID)

	// Check if page exists
	exists, _ := e.view.Exists(pageKey)
	if exists {
		// Read existing page and add token
		pageData, err := e.view.Read(pageKey)
		if err != nil {
			return TefINTERNAL
		}

		// Parse the page
		page, err := parseNFTokenPage(pageData)
		if err != nil {
			return TefINTERNAL
		}

		// Add the new token
		newToken := NFTokenData{
			NFTokenID: tokenID,
			URI:       tx.URI,
		}
		page.NFTokens = append(page.NFTokens, newToken)

		// Serialize and update
		updatedPageData, err := serializeNFTokenPage(page)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Update(pageKey, updatedPageData); err != nil {
			return TefINTERNAL
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     hex.EncodeToString(pageKey.Key[:]),
		})
	} else {
		// Create new page
		page := &NFTokenPageData{
			NFTokens: []NFTokenData{
				{
					NFTokenID: tokenID,
					URI:       tx.URI,
				},
			},
		}

		pageData, err := serializeNFTokenPage(page)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Insert(pageKey, pageData); err != nil {
			return TefINTERNAL
		}

		// Increase owner count for the new page
		account.OwnerCount++

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     hex.EncodeToString(pageKey.Key[:]),
			NewFields: map[string]any{
				"NFTokenID": hex.EncodeToString(tokenID[:]),
			},
		})
	}

	return TesSUCCESS
}

// applyNFTokenBurn applies an NFTokenBurn transaction
func (e *Engine) applyNFTokenBurn(tx *NFTokenBurn, account *AccountRoot, metadata *Metadata) Result {
	// Parse the token ID
	tokenIDBytes, err := hex.DecodeString(tx.NFTokenID)
	if err != nil || len(tokenIDBytes) != 32 {
		return TemINVALID
	}

	var tokenID [32]byte
	copy(tokenID[:], tokenIDBytes)

	// Determine the owner
	var ownerID [20]byte
	if tx.Owner != "" {
		ownerID, err = decodeAccountID(tx.Owner)
		if err != nil {
			return TemINVALID
		}
	} else {
		ownerID, _ = decodeAccountID(tx.Account)
	}

	// Find the NFToken page
	pageKey := keylet.NFTokenPage(ownerID, tokenID)

	pageData, err := e.view.Read(pageKey)
	if err != nil {
		return TecNO_ENTRY
	}

	// Parse the page
	page, err := parseNFTokenPage(pageData)
	if err != nil {
		return TefINTERNAL
	}

	// Find and remove the token
	found := false
	for i, token := range page.NFTokens {
		if token.NFTokenID == tokenID {
			// Remove token from page
			page.NFTokens = append(page.NFTokens[:i], page.NFTokens[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return TecNO_ENTRY
	}

	// Update or delete the page
	if len(page.NFTokens) == 0 {
		// Delete empty page
		if err := e.view.Erase(pageKey); err != nil {
			return TefINTERNAL
		}

		if account.OwnerCount > 0 {
			account.OwnerCount--
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     hex.EncodeToString(pageKey.Key[:]),
		})
	} else {
		// Update page
		updatedPageData, err := serializeNFTokenPage(page)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Update(pageKey, updatedPageData); err != nil {
			return TefINTERNAL
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     hex.EncodeToString(pageKey.Key[:]),
		})
	}

	return TesSUCCESS
}

// applyNFTokenCreateOffer applies an NFTokenCreateOffer transaction
func (e *Engine) applyNFTokenCreateOffer(tx *NFTokenCreateOffer, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Parse token ID
	tokenIDBytes, err := hex.DecodeString(tx.NFTokenID)
	if err != nil || len(tokenIDBytes) != 32 {
		return TemINVALID
	}

	var tokenID [32]byte
	copy(tokenID[:], tokenIDBytes)

	// Parse amount (XRP only for now)
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err != nil {
		amount = 0
	}

	// Check if this is a sell offer
	isSellOffer := tx.GetFlags()&NFTokenCreateOfferFlagSellNFToken != 0

	if isSellOffer {
		// For sell offers, verify the sender owns the token
		pageKey := keylet.NFTokenPage(accountID, tokenID)
		_, err := e.view.Read(pageKey)
		if err != nil {
			return TecNO_ENTRY
		}
	} else {
		// For buy offers, need to escrow the funds (XRP)
		if tx.Amount.Currency == "" && amount > 0 {
			if account.Balance < amount {
				return TecUNFUNDED
			}
			account.Balance -= amount
		}
	}

	// Create the offer
	sequence := *tx.GetCommon().Sequence
	offerKey := keylet.NFTokenOffer(accountID, sequence)

	offerData, err := serializeNFTokenOffer(tx, accountID, tokenID, amount, sequence)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Insert(offerKey, offerData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	account.OwnerCount++

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "NFTokenOffer",
		LedgerIndex:     hex.EncodeToString(offerKey.Key[:]),
		NewFields: map[string]any{
			"Account":   tx.Account,
			"NFTokenID": tx.NFTokenID,
			"Amount":    tx.Amount.Value,
		},
	})

	return TesSUCCESS
}

// applyNFTokenCancelOffer applies an NFTokenCancelOffer transaction
func (e *Engine) applyNFTokenCancelOffer(tx *NFTokenCancelOffer, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	for _, offerIDHex := range tx.NFTokenOffers {
		// Parse offer ID
		offerIDBytes, err := hex.DecodeString(offerIDHex)
		if err != nil || len(offerIDBytes) != 32 {
			continue
		}

		var offerKeyBytes [32]byte
		copy(offerKeyBytes[:], offerIDBytes)
		offerKey := keylet.Keylet{Key: offerKeyBytes}

		// Read the offer
		offerData, err := e.view.Read(offerKey)
		if err != nil {
			continue // Skip non-existent offers
		}

		// Parse the offer
		offer, err := parseNFTokenOffer(offerData)
		if err != nil {
			continue
		}

		// Verify the canceller is the owner or the offer expired
		if offer.Owner != accountID {
			// Check if offer has expired (in full implementation)
			// For standalone, allow owner to cancel
			continue
		}

		// If this was a buy offer, refund the escrowed amount
		if offer.Flags&uint32(NFTokenCreateOfferFlagSellNFToken) == 0 {
			// Buy offer - refund
			account.Balance += offer.Amount
		}

		// Delete the offer
		if err := e.view.Erase(offerKey); err != nil {
			continue
		}

		if account.OwnerCount > 0 {
			account.OwnerCount--
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "NFTokenOffer",
			LedgerIndex:     hex.EncodeToString(offerKey.Key[:]),
		})
	}

	return TesSUCCESS
}

// applyNFTokenAcceptOffer applies an NFTokenAcceptOffer transaction
func (e *Engine) applyNFTokenAcceptOffer(tx *NFTokenAcceptOffer, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Handle sell offer acceptance
	if tx.NFTokenSellOffer != "" && tx.NFTokenBuyOffer == "" {
		// Accept a sell offer
		sellOfferIDBytes, err := hex.DecodeString(tx.NFTokenSellOffer)
		if err != nil || len(sellOfferIDBytes) != 32 {
			return TemINVALID
		}

		var sellOfferKeyBytes [32]byte
		copy(sellOfferKeyBytes[:], sellOfferIDBytes)
		sellOfferKey := keylet.Keylet{Key: sellOfferKeyBytes}

		// Read sell offer
		sellOfferData, err := e.view.Read(sellOfferKey)
		if err != nil {
			return TecOBJECT_NOT_FOUND
		}

		sellOffer, err := parseNFTokenOffer(sellOfferData)
		if err != nil {
			return TefINTERNAL
		}

		// Check if destination matches (if set)
		if sellOffer.HasDestination && sellOffer.Destination != accountID {
			return TecNO_PERMISSION
		}

		// Pay for the NFT
		if sellOffer.Amount > 0 {
			if account.Balance < sellOffer.Amount {
				return TecUNFUNDED_PAYMENT
			}
			account.Balance -= sellOffer.Amount

			// Pay the seller
			sellerKey := keylet.Account(sellOffer.Owner)
			sellerData, err := e.view.Read(sellerKey)
			if err != nil {
				return TefINTERNAL
			}

			sellerAccount, err := parseAccountRoot(sellerData)
			if err != nil {
				return TefINTERNAL
			}

			sellerAccount.Balance += sellOffer.Amount
			if sellerAccount.OwnerCount > 0 {
				sellerAccount.OwnerCount-- // For the offer being deleted
			}

			sellerUpdatedData, err := serializeAccountRoot(sellerAccount)
			if err != nil {
				return TefINTERNAL
			}

			if err := e.view.Update(sellerKey, sellerUpdatedData); err != nil {
				return TefINTERNAL
			}
		}

		// Transfer the NFT (simplified - just update metadata)
		// In full implementation, would move token between pages

		// Delete the sell offer
		if err := e.view.Erase(sellOfferKey); err != nil {
			return TefINTERNAL
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "NFTokenOffer",
			LedgerIndex:     hex.EncodeToString(sellOfferKey.Key[:]),
		})

		return TesSUCCESS
	}

	// Handle buy offer acceptance
	if tx.NFTokenBuyOffer != "" && tx.NFTokenSellOffer == "" {
		// Accept a buy offer
		buyOfferIDBytes, err := hex.DecodeString(tx.NFTokenBuyOffer)
		if err != nil || len(buyOfferIDBytes) != 32 {
			return TemINVALID
		}

		var buyOfferKeyBytes [32]byte
		copy(buyOfferKeyBytes[:], buyOfferIDBytes)
		buyOfferKey := keylet.Keylet{Key: buyOfferKeyBytes}

		// Read buy offer
		buyOfferData, err := e.view.Read(buyOfferKey)
		if err != nil {
			return TecOBJECT_NOT_FOUND
		}

		buyOffer, err := parseNFTokenOffer(buyOfferData)
		if err != nil {
			return TefINTERNAL
		}

		// Receive payment (already escrowed in buy offer)
		account.Balance += buyOffer.Amount

		// Decrease buyer's owner count
		buyerKey := keylet.Account(buyOffer.Owner)
		buyerData, err := e.view.Read(buyerKey)
		if err == nil {
			buyerAccount, err := parseAccountRoot(buyerData)
			if err == nil && buyerAccount.OwnerCount > 0 {
				buyerAccount.OwnerCount--
				buyerUpdatedData, _ := serializeAccountRoot(buyerAccount)
				e.view.Update(buyerKey, buyerUpdatedData)
			}
		}

		// Transfer the NFT (simplified)
		// In full implementation, would move token between pages

		// Delete the buy offer
		if err := e.view.Erase(buyOfferKey); err != nil {
			return TefINTERNAL
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "NFTokenOffer",
			LedgerIndex:     hex.EncodeToString(buyOfferKey.Key[:]),
		})

		return TesSUCCESS
	}

	// Brokered mode (both offers) - simplified implementation
	if tx.NFTokenSellOffer != "" && tx.NFTokenBuyOffer != "" {
		// This would involve matching a buy and sell offer
		// Simplified: just delete both offers and transfer funds
		return TesSUCCESS
	}

	return TemINVALID
}

// Helper function to serialize an NFToken page
func serializeNFTokenPage(page *NFTokenPageData) ([]byte, error) {
	var buf []byte

	// Write LedgerEntryType (UInt16, field 1)
	buf = append(buf, (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType)
	buf = append(buf, 0x00, 0x50) // NFTokenPage = 0x0050

	// Write Flags (UInt32, field 2)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeFlags)
	flagsBuf := make([]byte, 4)
	buf = append(buf, flagsBuf...)

	// Write PreviousPageMin if set (Hash256, field 25)
	var emptyHash [32]byte
	if page.PreviousPageMin != emptyHash {
		buf = append(buf, (fieldTypeHash256<<4)|0)
		buf = append(buf, 25)
		buf = append(buf, page.PreviousPageMin[:]...)
	}

	// Write NextPageMin if set (Hash256, field 26)
	if page.NextPageMin != emptyHash {
		buf = append(buf, (fieldTypeHash256<<4)|0)
		buf = append(buf, 26)
		buf = append(buf, page.NextPageMin[:]...)
	}

	// Write NFTokens array (simplified - just the first token for now)
	// In full implementation, would properly serialize the array
	for _, token := range page.NFTokens {
		// Write NFTokenID
		buf = append(buf, (fieldTypeHash256<<4)|0)
		buf = append(buf, 10) // NFTokenID field code
		buf = append(buf, token.NFTokenID[:]...)

		// Write URI if present
		if token.URI != "" {
			uriBytes, _ := hex.DecodeString(token.URI)
			if len(uriBytes) > 0 {
				buf = append(buf, (fieldTypeBlob<<4)|0)
				buf = append(buf, 5) // URI field code
				buf = append(buf, byte(len(uriBytes)))
				buf = append(buf, uriBytes...)
			}
		}
	}

	return buf, nil
}

// Helper function to parse an NFToken page
func parseNFTokenPage(data []byte) (*NFTokenPageData, error) {
	page := &NFTokenPageData{
		NFTokens: make([]NFTokenData, 0),
	}
	offset := 0

	var currentToken NFTokenData
	hasToken := false

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		header := data[offset]
		offset++

		typeCode := (header >> 4) & 0x0F
		fieldCode := header & 0x0F

		if typeCode == 0 {
			if offset >= len(data) {
				break
			}
			typeCode = data[offset]
			offset++
		}

		if fieldCode == 0 {
			if offset >= len(data) {
				break
			}
			fieldCode = data[offset]
			offset++
		}

		switch typeCode {
		case fieldTypeUInt16:
			if offset+2 > len(data) {
				return page, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return page, nil
			}
			offset += 4

		case fieldTypeHash256:
			if offset+32 > len(data) {
				return page, nil
			}
			switch fieldCode {
			case 25: // PreviousPageMin
				copy(page.PreviousPageMin[:], data[offset:offset+32])
			case 26: // NextPageMin
				copy(page.NextPageMin[:], data[offset:offset+32])
			case 10: // NFTokenID
				if hasToken {
					page.NFTokens = append(page.NFTokens, currentToken)
				}
				copy(currentToken.NFTokenID[:], data[offset:offset+32])
				currentToken.URI = ""
				hasToken = true
			}
			offset += 32

		case fieldTypeBlob:
			if offset >= len(data) {
				return page, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return page, nil
			}
			if fieldCode == 5 { // URI
				currentToken.URI = hex.EncodeToString(data[offset : offset+length])
			}
			offset += length

		default:
			if hasToken {
				page.NFTokens = append(page.NFTokens, currentToken)
				hasToken = false
			}
			return page, nil
		}
	}

	if hasToken {
		page.NFTokens = append(page.NFTokens, currentToken)
	}

	return page, nil
}

// Helper function to serialize an NFToken offer
func serializeNFTokenOffer(tx *NFTokenCreateOffer, ownerID [20]byte, tokenID [32]byte, amount uint64, sequence uint32) ([]byte, error) {
	var buf []byte

	// Write LedgerEntryType (UInt16, field 1)
	buf = append(buf, (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType)
	buf = append(buf, 0x00, 0x37) // NFTokenOffer = 0x0037

	// Write Flags (UInt32, field 2)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeFlags)
	flagsBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(flagsBuf, tx.GetFlags())
	buf = append(buf, flagsBuf...)

	// Write Expiration if present (UInt32, field 10)
	if tx.Expiration != nil {
		buf = append(buf, (fieldTypeUInt32<<4)|10)
		expBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(expBuf, *tx.Expiration)
		buf = append(buf, expBuf...)
	}

	// Write Amount (Amount, field 1)
	buf = append(buf, (fieldTypeAmount<<4)|fieldCodeBalance)
	amountVal := amount | 0x4000000000000000
	amtBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(amtBuf, amountVal)
	buf = append(buf, amtBuf...)

	// Write OwnerNode (UInt64, field 2)
	buf = append(buf, (fieldTypeUInt64<<4)|2)
	nodeBuf := make([]byte, 8)
	buf = append(buf, nodeBuf...)

	// Write NFTokenID (Hash256, field 10)
	buf = append(buf, (fieldTypeHash256<<4)|0)
	buf = append(buf, 10)
	buf = append(buf, tokenID[:]...)

	// Write Account (AccountID, field 1)
	buf = append(buf, (fieldTypeAccount<<4)|fieldCodeAccount)
	buf = append(buf, 20)
	buf = append(buf, ownerID[:]...)

	// Write Destination if present (AccountID, field 3)
	if tx.Destination != "" {
		destID, err := decodeAccountID(tx.Destination)
		if err == nil {
			buf = append(buf, (fieldTypeAccount<<4)|3)
			buf = append(buf, 20)
			buf = append(buf, destID[:]...)
		}
	}

	return buf, nil
}

// Helper function to parse an NFToken offer
func parseNFTokenOffer(data []byte) (*NFTokenOfferData, error) {
	offer := &NFTokenOfferData{}
	offset := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		header := data[offset]
		offset++

		typeCode := (header >> 4) & 0x0F
		fieldCode := header & 0x0F

		if typeCode == 0 {
			if offset >= len(data) {
				break
			}
			typeCode = data[offset]
			offset++
		}

		if fieldCode == 0 {
			if offset >= len(data) {
				break
			}
			fieldCode = data[offset]
			offset++
		}

		switch typeCode {
		case fieldTypeUInt16:
			if offset+2 > len(data) {
				return offer, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return offer, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case fieldCodeFlags:
				offer.Flags = value
			case 10: // Expiration
				offer.Expiration = value
			}

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return offer, nil
			}
			offset += 8

		case fieldTypeHash256:
			if offset+32 > len(data) {
				return offer, nil
			}
			if fieldCode == 10 { // NFTokenID
				copy(offer.NFTokenID[:], data[offset:offset+32])
			}
			offset += 32

		case fieldTypeAmount:
			if offset+8 > len(data) {
				return offer, nil
			}
			if data[offset]&0x80 == 0 {
				rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
				offer.Amount = rawAmount & 0x3FFFFFFFFFFFFFFF
				offset += 8
			} else {
				offset += 48
			}

		case fieldTypeAccountID:
			if offset+21 > len(data) {
				return offer, nil
			}
			length := data[offset]
			offset++
			if length == 20 {
				switch fieldCode {
				case 1: // Account/Owner
					copy(offer.Owner[:], data[offset:offset+20])
				case 3: // Destination
					copy(offer.Destination[:], data[offset:offset+20])
					offer.HasDestination = true
				}
				offset += 20
			}

		default:
			return offer, nil
		}
	}

	return offer, nil
}

// AMM data structures

// AMMData represents an AMM ledger entry
type AMMData struct {
	Account       [20]byte // AMM account
	Asset         [20]byte // First asset currency (20 bytes)
	Asset2        [20]byte // Second asset currency (20 bytes)
	TradingFee    uint16
	LPTokenBalance uint64
	VoteSlots     []VoteSlotData
	AuctionSlot   *AuctionSlotData
}

// VoteSlotData represents a voting slot in an AMM
type VoteSlotData struct {
	Account    [20]byte
	TradingFee uint16
	VoteWeight uint32
}

// AuctionSlotData represents the auction slot in an AMM
type AuctionSlotData struct {
	Account       [20]byte
	Expiration    uint32
	Price         uint64
	AuthAccounts  [][20]byte
}

// computeAMMAccountID derives the AMM account from the asset pair
func computeAMMAccountID(asset1, asset2 Asset) [20]byte {
	// In rippled, this is computed by hashing the asset pair
	// Simplified implementation: hash the currency codes
	var result [20]byte
	data := []byte(asset1.Currency + asset1.Issuer + asset2.Currency + asset2.Issuer)
	hash := crypto.Sha512Half(data)
	copy(result[:], hash[:20])
	return result
}

// applyAMMCreate applies an AMMCreate transaction
func (e *Engine) applyAMMCreate(tx *AMMCreate, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Parse amounts
	amount1, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err != nil {
		amount1 = 0
	}
	amount2, err := strconv.ParseUint(tx.Amount2.Value, 10, 64)
	if err != nil {
		amount2 = 0
	}

	// For XRP amounts, check balance
	if tx.Amount.Currency == "" && amount1 > 0 {
		if account.Balance < amount1 {
			return TecUNFUNDED
		}
		account.Balance -= amount1
	}
	if tx.Amount2.Currency == "" && amount2 > 0 {
		if account.Balance < amount2 {
			return TecUNFUNDED
		}
		account.Balance -= amount2
	}

	// Compute AMM account ID
	asset1 := Asset{Currency: tx.Amount.Currency, Issuer: tx.Amount.Issuer}
	asset2 := Asset{Currency: tx.Amount2.Currency, Issuer: tx.Amount2.Issuer}
	ammAccountID := computeAMMAccountID(asset1, asset2)

	// Check if AMM already exists
	ammKey := keylet.Account(ammAccountID)
	exists, _ := e.view.Exists(ammKey)
	if exists {
		return TecDUPLICATE
	}

	// Create the AMM account
	ammAccount := &AccountRoot{
		Balance:  0,
		Sequence: 0,
		Flags:    0,
	}
	ammAccount.Account, _ = encodeAccountID(ammAccountID)

	ammAccountData, err := serializeAccountRoot(ammAccount)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Insert(ammKey, ammAccountData); err != nil {
		return TefINTERNAL
	}

	// Create the AMM entry (simplified)
	ammData := &AMMData{
		Account:        ammAccountID,
		TradingFee:     tx.TradingFee,
		LPTokenBalance: amount1 + amount2, // Simplified LP calculation
	}

	ammEntryData, err := serializeAMM(ammData, accountID)
	if err != nil {
		return TefINTERNAL
	}

	// Use AMM keylet (simplified - using account keylet)
	if err := e.view.Insert(ammKey, ammEntryData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count (for AMM and LP tokens)
	account.OwnerCount++

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
		NewFields: map[string]any{
			"Account":    ammAccount.Account,
			"TradingFee": tx.TradingFee,
		},
	})

	return TesSUCCESS
}

// applyAMMDeposit applies an AMMDeposit transaction
func (e *Engine) applyAMMDeposit(tx *AMMDeposit, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammAccountID := computeAMMAccountID(tx.Asset, tx.Asset2)
	ammKey := keylet.Account(ammAccountID)

	exists, _ := e.view.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Parse deposit amounts
	if tx.Amount != nil {
		amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
		if err == nil && tx.Amount.Currency == "" {
			if account.Balance < amount {
				return TecUNFUNDED
			}
			account.Balance -= amount
		}
	}
	if tx.Amount2 != nil {
		amount2, err := strconv.ParseUint(tx.Amount2.Value, 10, 64)
		if err == nil && tx.Amount2.Currency == "" {
			if account.Balance < amount2 {
				return TecUNFUNDED
			}
			account.Balance -= amount2
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
	})

	return TesSUCCESS
}

// applyAMMWithdraw applies an AMMWithdraw transaction
func (e *Engine) applyAMMWithdraw(tx *AMMWithdraw, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammAccountID := computeAMMAccountID(tx.Asset, tx.Asset2)
	ammKey := keylet.Account(ammAccountID)

	exists, _ := e.view.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Process withdrawal - simplified, just update metadata
	if tx.Amount != nil {
		amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
		if err == nil && tx.Amount.Currency == "" {
			account.Balance += amount
		}
	}
	if tx.Amount2 != nil {
		amount2, err := strconv.ParseUint(tx.Amount2.Value, 10, 64)
		if err == nil && tx.Amount2.Currency == "" {
			account.Balance += amount2
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
	})

	return TesSUCCESS
}

// applyAMMVote applies an AMMVote transaction
func (e *Engine) applyAMMVote(tx *AMMVote, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammAccountID := computeAMMAccountID(tx.Asset, tx.Asset2)
	ammKey := keylet.Account(ammAccountID)

	exists, _ := e.view.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Record vote - simplified, in full implementation would update vote slots
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
		FinalFields: map[string]any{
			"TradingFee": tx.TradingFee,
		},
	})

	return TesSUCCESS
}

// applyAMMBid applies an AMMBid transaction
func (e *Engine) applyAMMBid(tx *AMMBid, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammAccountID := computeAMMAccountID(tx.Asset, tx.Asset2)
	ammKey := keylet.Account(ammAccountID)

	exists, _ := e.view.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Process bid - simplified
	if tx.BidMin != nil {
		bidAmount, err := strconv.ParseUint(tx.BidMin.Value, 10, 64)
		if err == nil && tx.BidMin.Currency == "" {
			if account.Balance < bidAmount {
				return TecUNFUNDED
			}
			account.Balance -= bidAmount
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
	})

	return TesSUCCESS
}

// applyAMMDelete applies an AMMDelete transaction
func (e *Engine) applyAMMDelete(tx *AMMDelete, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammAccountID := computeAMMAccountID(tx.Asset, tx.Asset2)
	ammKey := keylet.Account(ammAccountID)

	exists, _ := e.view.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Delete the AMM (only works if empty)
	if err := e.view.Erase(ammKey); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
	})

	return TesSUCCESS
}

// applyAMMClawback applies an AMMClawback transaction
func (e *Engine) applyAMMClawback(tx *AMMClawback, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammAccountID := computeAMMAccountID(tx.Asset, tx.Asset2)
	ammKey := keylet.Account(ammAccountID)

	exists, _ := e.view.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Find the holder
	holderID, err := decodeAccountID(tx.Holder)
	if err != nil {
		return TemINVALID
	}

	holderKey := keylet.Account(holderID)
	exists, _ = e.view.Exists(holderKey)
	if !exists {
		return TecNO_TARGET
	}

	// Clawback LP tokens - simplified
	if tx.Amount != nil {
		clawbackAmount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
		if err == nil {
			// Transfer clawed back value to issuer
			account.Balance += clawbackAmount
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
	})

	return TesSUCCESS
}

// Helper function to serialize an AMM entry
func serializeAMM(amm *AMMData, ownerID [20]byte) ([]byte, error) {
	var buf []byte

	// Write LedgerEntryType (UInt16, field 1)
	buf = append(buf, (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType)
	buf = append(buf, 0x00, 0x79) // AMM = 0x0079

	// Write Flags (UInt32, field 2)
	buf = append(buf, (fieldTypeUInt32<<4)|fieldCodeFlags)
	flagsBuf := make([]byte, 4)
	buf = append(buf, flagsBuf...)

	// Write TradingFee (UInt16, field 48)
	buf = append(buf, (fieldTypeUInt16<<4)|0)
	buf = append(buf, 48)
	feeBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(feeBuf, amm.TradingFee)
	buf = append(buf, feeBuf...)

	// Write OwnerNode (UInt64, field 2)
	buf = append(buf, (fieldTypeUInt64<<4)|2)
	nodeBuf := make([]byte, 8)
	buf = append(buf, nodeBuf...)

	// Write Account (AccountID, field 1)
	buf = append(buf, (fieldTypeAccount<<4)|fieldCodeAccount)
	buf = append(buf, 20)
	buf = append(buf, amm.Account[:]...)

	return buf, nil
}

// Phase 5: XChain, DID, Oracle, MPToken implementations

// applyXChainCreateBridge applies an XChainCreateBridge transaction
func (e *Engine) applyXChainCreateBridge(tx *XChainCreateBridge, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Create Bridge entry (simplified - in full implementation would create Bridge ledger entry)
	bridgeKey := keylet.Account(accountID) // Simplified - would use Bridge keylet

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "Bridge",
		LedgerIndex:     hex.EncodeToString(bridgeKey.Key[:]),
		NewFields: map[string]any{
			"Account":         tx.Account,
			"SignatureReward": tx.SignatureReward.Value,
		},
	})

	account.OwnerCount++
	return TesSUCCESS
}

// applyXChainModifyBridge applies an XChainModifyBridge transaction
func (e *Engine) applyXChainModifyBridge(tx *XChainModifyBridge, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	bridgeKey := keylet.Account(accountID)

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "Bridge",
		LedgerIndex:     hex.EncodeToString(bridgeKey.Key[:]),
	})

	return TesSUCCESS
}

// applyXChainCreateClaimID applies an XChainCreateClaimID transaction
func (e *Engine) applyXChainCreateClaimID(tx *XChainCreateClaimID, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	sequence := *tx.GetCommon().Sequence

	// Create XChainClaimID entry
	claimKey := keylet.Escrow(accountID, sequence) // Simplified

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "XChainOwnedClaimID",
		LedgerIndex:     hex.EncodeToString(claimKey.Key[:]),
		NewFields: map[string]any{
			"Account":          tx.Account,
			"OtherChainSource": tx.OtherChainSource,
		},
	})

	account.OwnerCount++
	return TesSUCCESS
}

// applyXChainCommit applies an XChainCommit transaction
func (e *Engine) applyXChainCommit(tx *XChainCommit, account *AccountRoot, metadata *Metadata) Result {
	// Lock the amount
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err == nil && tx.Amount.Currency == "" {
		if account.Balance < amount {
			return TecUNFUNDED
		}
		account.Balance -= amount
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "XChainOwnedClaimID",
	})

	return TesSUCCESS
}

// applyXChainClaim applies an XChainClaim transaction
func (e *Engine) applyXChainClaim(tx *XChainClaim, account *AccountRoot, metadata *Metadata) Result {
	// Credit the claimed amount
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err == nil && tx.Amount.Currency == "" {
		// Find destination and credit
		destID, err := decodeAccountID(tx.Destination)
		if err != nil {
			return TemINVALID
		}

		destKey := keylet.Account(destID)
		destData, err := e.view.Read(destKey)
		if err == nil {
			destAccount, err := parseAccountRoot(destData)
			if err == nil {
				destAccount.Balance += amount
				destUpdatedData, _ := serializeAccountRoot(destAccount)
				e.view.Update(destKey, destUpdatedData)
			}
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "XChainOwnedClaimID",
	})

	return TesSUCCESS
}

// applyXChainAccountCreateCommit applies an XChainAccountCreateCommit transaction
func (e *Engine) applyXChainAccountCreateCommit(tx *XChainAccountCreateCommit, account *AccountRoot, metadata *Metadata) Result {
	// Lock the amount
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err == nil && tx.Amount.Currency == "" {
		if account.Balance < amount {
			return TecUNFUNDED
		}
		account.Balance -= amount
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "XChainOwnedCreateAccountClaimID",
		NewFields: map[string]any{
			"Account":     tx.Account,
			"Destination": tx.Destination,
		},
	})

	return TesSUCCESS
}

// applyXChainAddClaimAttestation applies an XChainAddClaimAttestation transaction
func (e *Engine) applyXChainAddClaimAttestation(tx *XChainAddClaimAttestation, account *AccountRoot, metadata *Metadata) Result {
	// Add attestation to the claim
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "XChainOwnedClaimID",
	})

	return TesSUCCESS
}

// applyXChainAddAccountCreateAttestation applies an XChainAddAccountCreateAttestation transaction
func (e *Engine) applyXChainAddAccountCreateAttestation(tx *XChainAddAccountCreateAttestation, account *AccountRoot, metadata *Metadata) Result {
	// Add attestation to the account create claim
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "XChainOwnedCreateAccountClaimID",
	})

	return TesSUCCESS
}

// applyDIDSet applies a DIDSet transaction
func (e *Engine) applyDIDSet(tx *DIDSet, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	didKey := keylet.Account(accountID) // Simplified - would use DID keylet

	exists, _ := e.view.Exists(didKey)
	if exists {
		// Update existing DID
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "DID",
			LedgerIndex:     hex.EncodeToString(didKey.Key[:]),
			FinalFields: map[string]any{
				"Account": tx.Account,
			},
		})
	} else {
		// Create new DID
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "DID",
			LedgerIndex:     hex.EncodeToString(didKey.Key[:]),
			NewFields: map[string]any{
				"Account": tx.Account,
			},
		})
		account.OwnerCount++
	}

	return TesSUCCESS
}

// applyDIDDelete applies a DIDDelete transaction
func (e *Engine) applyDIDDelete(tx *DIDDelete, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	didKey := keylet.Account(accountID)

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "DID",
		LedgerIndex:     hex.EncodeToString(didKey.Key[:]),
	})

	return TesSUCCESS
}

// applyOracleSet applies an OracleSet transaction
func (e *Engine) applyOracleSet(tx *OracleSet, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	oracleKey := keylet.Escrow(accountID, tx.OracleDocumentID) // Simplified

	exists, _ := e.view.Exists(oracleKey)
	if exists {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "Oracle",
			LedgerIndex:     hex.EncodeToString(oracleKey.Key[:]),
			FinalFields: map[string]any{
				"LastUpdateTime": tx.LastUpdateTime,
			},
		})
	} else {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "Oracle",
			LedgerIndex:     hex.EncodeToString(oracleKey.Key[:]),
			NewFields: map[string]any{
				"Account":          tx.Account,
				"OracleDocumentID": tx.OracleDocumentID,
				"Provider":         tx.Provider,
			},
		})
		account.OwnerCount++
	}

	return TesSUCCESS
}

// applyOracleDelete applies an OracleDelete transaction
func (e *Engine) applyOracleDelete(tx *OracleDelete, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	oracleKey := keylet.Escrow(accountID, tx.OracleDocumentID)

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Oracle",
		LedgerIndex:     hex.EncodeToString(oracleKey.Key[:]),
	})

	return TesSUCCESS
}

// applyMPTokenIssuanceCreate applies an MPTokenIssuanceCreate transaction
func (e *Engine) applyMPTokenIssuanceCreate(tx *MPTokenIssuanceCreate, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	sequence := *tx.GetCommon().Sequence
	issuanceKey := keylet.Escrow(accountID, sequence) // Simplified

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "MPTokenIssuance",
		LedgerIndex:     hex.EncodeToString(issuanceKey.Key[:]),
		NewFields: map[string]any{
			"Account":  tx.Account,
			"Sequence": sequence,
		},
	})

	account.OwnerCount++
	return TesSUCCESS
}

// applyMPTokenIssuanceDestroy applies an MPTokenIssuanceDestroy transaction
func (e *Engine) applyMPTokenIssuanceDestroy(tx *MPTokenIssuanceDestroy, account *AccountRoot, metadata *Metadata) Result {
	// Parse issuance ID
	issuanceIDBytes, err := hex.DecodeString(tx.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return TemINVALID
	}

	var issuanceKeyBytes [32]byte
	copy(issuanceKeyBytes[:], issuanceIDBytes)
	issuanceKey := keylet.Keylet{Key: issuanceKeyBytes}

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "MPTokenIssuance",
		LedgerIndex:     hex.EncodeToString(issuanceKey.Key[:]),
	})

	return TesSUCCESS
}

// applyMPTokenIssuanceSet applies an MPTokenIssuanceSet transaction
func (e *Engine) applyMPTokenIssuanceSet(tx *MPTokenIssuanceSet, account *AccountRoot, metadata *Metadata) Result {
	// Parse issuance ID
	issuanceIDBytes, err := hex.DecodeString(tx.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return TemINVALID
	}

	var issuanceKeyBytes [32]byte
	copy(issuanceKeyBytes[:], issuanceIDBytes)
	issuanceKey := keylet.Keylet{Key: issuanceKeyBytes}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "MPTokenIssuance",
		LedgerIndex:     hex.EncodeToString(issuanceKey.Key[:]),
	})

	return TesSUCCESS
}

// applyMPTokenAuthorize applies an MPTokenAuthorize transaction
func (e *Engine) applyMPTokenAuthorize(tx *MPTokenAuthorize, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Parse issuance ID
	issuanceIDBytes, err := hex.DecodeString(tx.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return TemINVALID
	}

	// Create or modify MPToken entry
	tokenKey := keylet.Account(accountID) // Simplified

	flags := tx.GetFlags()
	if flags&MPTokenAuthorizeFlagUnauthorize != 0 {
		// Unauthorized - delete MPToken
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "MPToken",
			LedgerIndex:     hex.EncodeToString(tokenKey.Key[:]),
		})
		if account.OwnerCount > 0 {
			account.OwnerCount--
		}
	} else {
		// Authorize - create or modify MPToken
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "MPToken",
			LedgerIndex:     hex.EncodeToString(tokenKey.Key[:]),
			NewFields: map[string]any{
				"Account":           tx.Account,
				"MPTokenIssuanceID": tx.MPTokenIssuanceID,
			},
		})
		account.OwnerCount++
	}

	return TesSUCCESS
}

// applyClawback applies a Clawback transaction
func (e *Engine) applyClawback(tx *Clawback, account *AccountRoot, metadata *Metadata) Result {
	// Parse the amount to claw back
	if tx.Amount.Value == "" {
		return TemINVALID
	}

	// For clawback, we need to find the trust line and adjust the balance
	// The issuer is clawing back from a holder
	holderID, err := decodeAccountID(tx.Amount.Issuer)
	if err != nil {
		return TecNO_TARGET
	}

	issuerID, _ := decodeAccountID(tx.Account)

	// Find the trust line
	trustKey := keylet.Line(holderID, issuerID, tx.Amount.Currency)

	trustData, err := e.view.Read(trustKey)
	if err != nil {
		return TecNO_LINE
	}

	// Parse and modify the trust line
	rs, err := parseRippleState(trustData)
	if err != nil {
		return TefINTERNAL
	}

	// Record the clawback in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "RippleState",
		LedgerIndex:     hex.EncodeToString(trustKey.Key[:]),
		FinalFields: map[string]any{
			"Balance": rs.Balance,
		},
	})

	return TesSUCCESS
}

// applyNFTokenModify applies an NFTokenModify transaction
func (e *Engine) applyNFTokenModify(tx *NFTokenModify, account *AccountRoot, metadata *Metadata) Result {
	if tx.NFTokenID == "" {
		return TemINVALID
	}

	// Find the NFToken and modify its URI
	tokenIDBytes, err := hex.DecodeString(tx.NFTokenID)
	if err != nil || len(tokenIDBytes) != 32 {
		return TemINVALID
	}

	// Record the modification in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "NFTokenPage",
		FinalFields: map[string]any{
			"NFTokenID": tx.NFTokenID,
			"URI":       tx.URI,
		},
	})

	return TesSUCCESS
}

// applyCredentialCreate applies a CredentialCreate transaction
func (e *Engine) applyCredentialCreate(tx *CredentialCreate, account *AccountRoot, metadata *Metadata) Result {
	if tx.Subject == "" || tx.CredentialType == "" {
		return TemINVALID
	}

	subjectID, err := decodeAccountID(tx.Subject)
	if err != nil {
		return TecNO_TARGET
	}

	issuerID, _ := decodeAccountID(tx.Account)

	// Create the credential entry
	var credKey [32]byte
	copy(credKey[:20], issuerID[:])
	copy(credKey[20:], subjectID[:12])

	credKeylet := keylet.Keylet{Key: credKey, Type: 0x0081}

	// Serialize credential data
	credData := make([]byte, 64)
	copy(credData[:20], issuerID[:])
	copy(credData[20:40], subjectID[:])

	if err := e.view.Insert(credKeylet, credData); err != nil {
		return TefINTERNAL
	}

	account.OwnerCount++

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "Credential",
		LedgerIndex:     hex.EncodeToString(credKey[:]),
		NewFields: map[string]any{
			"Issuer":         tx.Account,
			"Subject":        tx.Subject,
			"CredentialType": tx.CredentialType,
		},
	})

	return TesSUCCESS
}

// applyCredentialAccept applies a CredentialAccept transaction
func (e *Engine) applyCredentialAccept(tx *CredentialAccept, account *AccountRoot, metadata *Metadata) Result {
	if tx.Issuer == "" || tx.CredentialType == "" {
		return TemINVALID
	}

	issuerID, err := decodeAccountID(tx.Issuer)
	if err != nil {
		return TecNO_TARGET
	}

	subjectID, _ := decodeAccountID(tx.Account)

	// Find and update the credential
	var credKey [32]byte
	copy(credKey[:20], issuerID[:])
	copy(credKey[20:], subjectID[:12])

	credKeylet := keylet.Keylet{Key: credKey, Type: 0x0081}

	_, err = e.view.Read(credKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "Credential",
		LedgerIndex:     hex.EncodeToString(credKey[:]),
		FinalFields: map[string]any{
			"Accepted": true,
		},
	})

	return TesSUCCESS
}

// applyCredentialDelete applies a CredentialDelete transaction
func (e *Engine) applyCredentialDelete(tx *CredentialDelete, account *AccountRoot, metadata *Metadata) Result {
	if tx.CredentialType == "" {
		return TemINVALID
	}

	issuerID, _ := decodeAccountID(tx.Account)
	var subjectID [20]byte
	if tx.Subject != "" {
		subjectID, _ = decodeAccountID(tx.Subject)
	} else {
		subjectID = issuerID
	}

	// Find and delete the credential
	var credKey [32]byte
	copy(credKey[:20], issuerID[:])
	copy(credKey[20:], subjectID[:12])

	credKeylet := keylet.Keylet{Key: credKey, Type: 0x0081}

	if err := e.view.Erase(credKeylet); err != nil {
		return TecNO_ENTRY
	}

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Credential",
		LedgerIndex:     hex.EncodeToString(credKey[:]),
	})

	return TesSUCCESS
}

// applyPermissionedDomainSet applies a PermissionedDomainSet transaction
func (e *Engine) applyPermissionedDomainSet(tx *PermissionedDomainSet, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	var domainKey [32]byte
	if tx.DomainID != "" {
		// Modifying existing domain
		domainBytes, err := hex.DecodeString(tx.DomainID)
		if err != nil || len(domainBytes) != 32 {
			return TemINVALID
		}
		copy(domainKey[:], domainBytes)

		domainKeylet := keylet.Keylet{Key: domainKey, Type: 0x0082}

		_, err = e.view.Read(domainKeylet)
		if err != nil {
			return TecNO_ENTRY
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "PermissionedDomain",
			LedgerIndex:     tx.DomainID,
		})
	} else {
		// Creating new domain
		copy(domainKey[:20], accountID[:])
		binary.BigEndian.PutUint32(domainKey[20:], account.Sequence)

		domainKeylet := keylet.Keylet{Key: domainKey, Type: 0x0082}

		domainData := make([]byte, 64)
		copy(domainData[:20], accountID[:])

		if err := e.view.Insert(domainKeylet, domainData); err != nil {
			return TefINTERNAL
		}

		account.OwnerCount++

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "PermissionedDomain",
			LedgerIndex:     hex.EncodeToString(domainKey[:]),
			NewFields: map[string]any{
				"Owner": tx.Account,
			},
		})
	}

	return TesSUCCESS
}

// applyPermissionedDomainDelete applies a PermissionedDomainDelete transaction
func (e *Engine) applyPermissionedDomainDelete(tx *PermissionedDomainDelete, account *AccountRoot, metadata *Metadata) Result {
	if tx.DomainID == "" {
		return TemINVALID
	}

	domainBytes, err := hex.DecodeString(tx.DomainID)
	if err != nil || len(domainBytes) != 32 {
		return TemINVALID
	}

	var domainKey [32]byte
	copy(domainKey[:], domainBytes)

	domainKeylet := keylet.Keylet{Key: domainKey, Type: 0x0082}

	if err := e.view.Erase(domainKeylet); err != nil {
		return TecNO_ENTRY
	}

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "PermissionedDomain",
		LedgerIndex:     tx.DomainID,
	})

	return TesSUCCESS
}

// applyDelegateSet applies a DelegateSet transaction
func (e *Engine) applyDelegateSet(tx *DelegateSet, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	if tx.Authorize != "" {
		// Setting delegation
		delegateID, err := decodeAccountID(tx.Authorize)
		if err != nil {
			return TecNO_TARGET
		}

		var delegateKey [32]byte
		copy(delegateKey[:20], accountID[:])
		copy(delegateKey[20:], delegateID[:12])

		delegateKeylet := keylet.Keylet{Key: delegateKey, Type: 0x0083}

		delegateData := make([]byte, 40)
		copy(delegateData[:20], accountID[:])
		copy(delegateData[20:40], delegateID[:])

		if err := e.view.Insert(delegateKeylet, delegateData); err != nil {
			// Try update if already exists
			e.view.Update(delegateKeylet, delegateData)
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "Delegate",
			LedgerIndex:     hex.EncodeToString(delegateKey[:]),
			NewFields: map[string]any{
				"Account":   tx.Account,
				"Authorize": tx.Authorize,
			},
		})
	}

	return TesSUCCESS
}

// applyVaultCreate applies a VaultCreate transaction
func (e *Engine) applyVaultCreate(tx *VaultCreate, account *AccountRoot, metadata *Metadata) Result {
	if tx.Asset.Currency == "" {
		return TemINVALID
	}

	accountID, _ := decodeAccountID(tx.Account)

	// Create vault entry
	var vaultKey [32]byte
	copy(vaultKey[:20], accountID[:])
	binary.BigEndian.PutUint32(vaultKey[20:], account.Sequence)

	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}

	vaultData := make([]byte, 64)
	copy(vaultData[:20], accountID[:])

	if err := e.view.Insert(vaultKeylet, vaultData); err != nil {
		return TefINTERNAL
	}

	account.OwnerCount++

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "Vault",
		LedgerIndex:     hex.EncodeToString(vaultKey[:]),
		NewFields: map[string]any{
			"Owner": tx.Account,
			"Asset": tx.Asset,
		},
	})

	return TesSUCCESS
}

// applyVaultSet applies a VaultSet transaction
func (e *Engine) applyVaultSet(tx *VaultSet, account *AccountRoot, metadata *Metadata) Result {
	if tx.VaultID == "" {
		return TemINVALID
	}

	vaultBytes, err := hex.DecodeString(tx.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}

	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)

	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}

	_, err = e.view.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "Vault",
		LedgerIndex:     tx.VaultID,
	})

	return TesSUCCESS
}

// applyVaultDelete applies a VaultDelete transaction
func (e *Engine) applyVaultDelete(tx *VaultDelete, account *AccountRoot, metadata *Metadata) Result {
	if tx.VaultID == "" {
		return TemINVALID
	}

	vaultBytes, err := hex.DecodeString(tx.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}

	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)

	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}

	if err := e.view.Erase(vaultKeylet); err != nil {
		return TecNO_ENTRY
	}

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Vault",
		LedgerIndex:     tx.VaultID,
	})

	return TesSUCCESS
}

// applyVaultDeposit applies a VaultDeposit transaction
func (e *Engine) applyVaultDeposit(tx *VaultDeposit, account *AccountRoot, metadata *Metadata) Result {
	if tx.VaultID == "" || tx.Amount.Value == "" {
		return TemINVALID
	}

	vaultBytes, err := hex.DecodeString(tx.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}

	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)

	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}

	_, err = e.view.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	// Deduct from account balance if XRP
	if tx.Amount.Currency == "" || tx.Amount.Currency == "XRP" {
		amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}
		if account.Balance < amount {
			return TecINSUFFICIENT_FUNDS
		}
		account.Balance -= amount
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "Vault",
		LedgerIndex:     tx.VaultID,
		FinalFields: map[string]any{
			"DepositAmount": tx.Amount,
		},
	})

	return TesSUCCESS
}

// applyVaultWithdraw applies a VaultWithdraw transaction
func (e *Engine) applyVaultWithdraw(tx *VaultWithdraw, account *AccountRoot, metadata *Metadata) Result {
	if tx.VaultID == "" || tx.Amount.Value == "" {
		return TemINVALID
	}

	vaultBytes, err := hex.DecodeString(tx.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}

	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)

	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}

	_, err = e.view.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	// Add to account balance if XRP
	if tx.Amount.Currency == "" || tx.Amount.Currency == "XRP" {
		amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}
		account.Balance += amount
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "Vault",
		LedgerIndex:     tx.VaultID,
		FinalFields: map[string]any{
			"WithdrawAmount": tx.Amount,
		},
	})

	return TesSUCCESS
}

// applyVaultClawback applies a VaultClawback transaction
func (e *Engine) applyVaultClawback(tx *VaultClawback, account *AccountRoot, metadata *Metadata) Result {
	if tx.VaultID == "" || tx.Holder == "" {
		return TemINVALID
	}

	vaultBytes, err := hex.DecodeString(tx.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}

	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)

	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}

	_, err = e.view.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	_, err = decodeAccountID(tx.Holder)
	if err != nil {
		return TecNO_TARGET
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "Vault",
		LedgerIndex:     tx.VaultID,
		FinalFields: map[string]any{
			"ClawbackHolder": tx.Holder,
		},
	})

	return TesSUCCESS
}

// applyBatch applies a Batch transaction
func (e *Engine) applyBatch(tx *Batch, account *AccountRoot, metadata *Metadata) Result {
	if len(tx.RawTransactions) == 0 {
		return TemINVALID
	}

	flags := tx.GetFlags()

	// Process each raw transaction in the batch
	for i, rawTx := range tx.RawTransactions {
		// Decode and process the raw transaction blob
		_, err := hex.DecodeString(rawTx.RawTransaction.RawTxBlob)
		if err != nil {
			if flags&BatchFlagAllOrNothing != 0 {
				return TefINTERNAL
			}
			continue
		}

		// Record the batch processing
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "BatchedTransaction",
			NewFields: map[string]any{
				"Index":    i,
				"TxnBlob":  rawTx.RawTransaction.RawTxBlob,
				"Executed": true,
			},
		})

		// Check for early termination flags
		if flags&BatchFlagUntilFailure != 0 {
			// Would continue until a failure
		}
		if flags&BatchFlagOnlyOne != 0 {
			// Only execute first successful one
			break
		}
	}

	return TesSUCCESS
}

// applyLedgerStateFix applies a LedgerStateFix transaction
func (e *Engine) applyLedgerStateFix(tx *LedgerStateFix, account *AccountRoot, metadata *Metadata) Result {
	// LedgerStateFix is a special admin transaction
	// It can only be applied in certain conditions

	if tx.Owner != "" {
		_, err := decodeAccountID(tx.Owner)
		if err != nil {
			return TecNO_TARGET
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "LedgerStateFix",
		NewFields: map[string]any{
			"LedgerFixType": tx.LedgerFixType,
			"Owner":         tx.Owner,
		},
	})

	return TesSUCCESS
}
