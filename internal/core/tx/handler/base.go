package handler

import (
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// BaseHandler provides common functionality for transaction handlers.
// Embed this in your handler implementation to get default preflight/preclaim behavior.
type BaseHandler struct {
	txType string
}

// NewBaseHandler creates a new base handler for the given transaction type.
func NewBaseHandler(txType string) BaseHandler {
	return BaseHandler{txType: txType}
}

// TransactionType returns the transaction type.
func (h BaseHandler) TransactionType() string {
	return h.txType
}

// Preflight performs default preflight validation.
// Override this in your handler if you need custom validation.
func (h BaseHandler) Preflight(tx Transaction, ctx *Context) Result {
	common := tx.GetCommon()

	if common.Account == "" {
		return TemBAD_SRC_ACCOUNT
	}

	if common.TransactionType == "" {
		return TemINVALID
	}

	if common.Fee != "" {
		fee, err := strconv.ParseUint(common.Fee, 10, 64)
		if err != nil || fee == 0 {
			return TemBAD_FEE
		}
	}

	if common.Sequence == nil && common.TicketSequence == nil {
		return TemBAD_SEQUENCE
	}

	if err := tx.Validate(); err != nil {
		return TemINVALID
	}

	return TesSUCCESS
}

// Preclaim performs default preclaim validation.
// Override this in your handler if you need custom validation.
func (h BaseHandler) Preclaim(tx Transaction, ctx *Context) Result {
	common := tx.GetCommon()

	accountID, err := DecodeAccountID(common.Account)
	if err != nil {
		return TemBAD_SRC_ACCOUNT
	}

	accountKey := keylet.Account(accountID)
	exists, err := ctx.View.Exists(accountKey)
	if err != nil {
		return TefINTERNAL
	}
	if !exists {
		return TerNO_ACCOUNT
	}

	accountData, err := ctx.View.Read(accountKey)
	if err != nil {
		return TefINTERNAL
	}

	account, err := ParseAccountRoot(accountData)
	if err != nil {
		return TefINTERNAL
	}

	if common.Sequence != nil {
		if *common.Sequence < account.Sequence {
			return TefPAST_SEQ
		}
		if *common.Sequence > account.Sequence {
			return TerPRE_SEQ
		}
	}

	fee := CalculateFee(common, ctx.Config.BaseFee)
	if account.Balance < fee {
		return TerINSUF_FEE_B
	}

	if common.LastLedgerSequence != nil {
		if ctx.Config.LedgerSequence > *common.LastLedgerSequence {
			return TefMAX_LEDGER
		}
	}

	return TesSUCCESS
}

// Apply is a no-op. Override this in your handler.
func (h BaseHandler) Apply(tx Transaction, account *AccountRoot, ctx *Context) Result {
	return TesSUCCESS
}

// DecodeAccountID decodes an address string to an account ID.
func DecodeAccountID(address string) ([20]byte, error) {
	var accountID [20]byte
	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(address)
	if err != nil {
		return accountID, err
	}
	copy(accountID[:], accountIDBytes)
	return accountID, nil
}

// ParseAccountRoot parses account data from bytes.
// This is a simplified parser - use the full parser from the tx package in production.
func ParseAccountRoot(data []byte) (*AccountRoot, error) {
	// This is a placeholder - the actual implementation should use
	// the binary codec to parse the account root.
	// For now, we return a minimal struct.
	return &AccountRoot{}, nil
}

// SerializeAccountRoot serializes an account root to bytes.
// This is a simplified serializer - use the full serializer from the tx package in production.
func SerializeAccountRoot(account *AccountRoot) ([]byte, error) {
	// This is a placeholder - the actual implementation should use
	// the binary codec to serialize the account root.
	return nil, nil
}

// CalculateFee calculates the transaction fee.
func CalculateFee(common *CommonFields, baseFee uint64) uint64 {
	if common.Fee != "" {
		fee, err := strconv.ParseUint(common.Fee, 10, 64)
		if err == nil {
			return fee
		}
	}
	return baseFee
}

// GetAccountFromLedger retrieves an account from the ledger.
func GetAccountFromLedger(view LedgerView, address string) (*AccountRoot, keylet.Keylet, error) {
	accountID, err := DecodeAccountID(address)
	if err != nil {
		return nil, keylet.Keylet{}, err
	}

	accountKey := keylet.Account(accountID)
	accountData, err := view.Read(accountKey)
	if err != nil {
		return nil, keylet.Keylet{}, err
	}

	account, err := ParseAccountRoot(accountData)
	if err != nil {
		return nil, keylet.Keylet{}, err
	}

	return account, accountKey, nil
}
