package tx

import (
	"encoding/hex"
	"strconv"
)

// preflight performs initial validation on the transaction.
// This includes syntax validation, signature verification, and memo validation.
// Preflight checks do not require ledger state access.
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
		if result := e.verifyTransactionSignature(tx); result != TesSUCCESS {
			return result
		}
	}

	// Transaction-specific validation
	if err := tx.Validate(); err != nil {
		return TemINVALID
	}

	return TesSUCCESS
}

// verifyTransactionSignature verifies single or multi-signature on the transaction
func (e *Engine) verifyTransactionSignature(tx Transaction) Result {
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

	return TesSUCCESS
}

// validateNetworkID validates the NetworkID field according to rippled rules.
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
