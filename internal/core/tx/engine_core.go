package tx

import (
	"encoding/hex"
	"errors"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	crypto "github.com/LeJamon/goXRPLd/internal/crypto/common"
)

// Engine processes transactions against a ledger.
// It implements the complete transaction lifecycle: preflight, preclaim, and apply.
type Engine struct {
	// view provides access to ledger state
	view LedgerView

	// config holds engine configuration
	config EngineConfig

	// currentTxHash is the hash of the transaction currently being applied
	// Used to set PreviousTxnID on modified ledger entries
	currentTxHash [32]byte
}

// NewEngine creates a new transaction engine with the given ledger view and configuration.
func NewEngine(view LedgerView, config EngineConfig) *Engine {
	return &Engine{
		view:   view,
		config: config,
	}
}

// Apply processes a transaction and applies it to the ledger.
// It performs the complete transaction lifecycle:
//  1. Preflight: syntax validation, signature verification
//  2. Preclaim: ledger state validation
//  3. Fee calculation
//  4. Apply: type-specific transaction logic
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
	metadata := NewMetadata()

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

// View returns the ledger view used by this engine.
func (e *Engine) View() LedgerView {
	return e.view
}

// Config returns the engine configuration.
func (e *Engine) Config() EngineConfig {
	return e.config
}

// CurrentTxHash returns the hash of the currently processing transaction.
func (e *Engine) CurrentTxHash() [32]byte {
	return e.currentTxHash
}

// computeTransactionHash computes the hash of a transaction.
// The hash is SHA512Half of the "TXN\x00" prefix + serialized transaction.
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
