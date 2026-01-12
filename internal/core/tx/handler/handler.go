// Package handler provides the transaction handler interface and registry
// for processing XRPL transactions with proper separation of concerns.
package handler

import (
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// Context provides access to ledger state and configuration during transaction processing.
type Context struct {
	// View provides read/write access to ledger state
	View LedgerView

	// Config holds engine configuration
	Config Config

	// Metadata tracks changes made by the transaction
	Metadata *Metadata
}

// Config holds configuration for transaction processing.
type Config struct {
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

	// Standalone indicates if running in standalone mode
	Standalone bool
}

// LedgerView provides read/write access to ledger state.
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

	// ForEach iterates over all state entries
	ForEach(fn func(key [32]byte, data []byte) bool) error
}

// Handler defines the interface for transaction type handlers.
// Each transaction type implements this interface to handle its specific logic.
type Handler interface {
	// TransactionType returns the transaction type this handler processes
	TransactionType() string

	// Preflight performs initial syntax validation
	Preflight(tx Transaction, ctx *Context) Result

	// Preclaim validates the transaction against ledger state
	Preclaim(tx Transaction, ctx *Context) Result

	// Apply executes the transaction and modifies ledger state
	Apply(tx Transaction, account *AccountRoot, ctx *Context) Result
}

// Transaction represents a generic XRPL transaction.
type Transaction interface {
	// GetCommon returns the common transaction fields
	GetCommon() *CommonFields

	// Validate performs transaction-specific validation
	Validate() error
}

// CommonFields contains fields common to all transactions.
type CommonFields struct {
	Account            string
	TransactionType    string
	Fee                string
	Sequence           *uint32
	TicketSequence     *uint32
	LastLedgerSequence *uint32
	SourceTag          *uint32
	SigningPubKey      string
	TxnSignature       string
	Memos              []Memo
}

// Memo represents a transaction memo.
type Memo struct {
	MemoType string
	MemoData string
}

// AccountRoot represents an account in the ledger.
type AccountRoot struct {
	Account      string
	Balance      uint64
	Sequence     uint32
	Flags        uint32
	OwnerCount   uint32
	RegularKey   string
	Domain       string
	EmailHash    string
	MessageKey   string
	TransferRate uint32
	TickSize     uint8
}

// Metadata tracks changes made by a transaction.
type Metadata struct {
	AffectedNodes     []AffectedNode
	TransactionIndex  uint32
	TransactionResult Result
	DeliveredAmount   *Amount
}

// AffectedNode represents a ledger entry that was changed.
type AffectedNode struct {
	NodeType        string
	LedgerEntryType string
	LedgerIndex     string
	FinalFields     map[string]any
	PreviousFields  map[string]any
	NewFields       map[string]any
}

// Amount represents an XRPL amount (either XRP or IOU).
type Amount struct {
	Value    string
	Currency string
	Issuer   string
}

// IsNative returns true if this is an XRP amount.
func (a Amount) IsNative() bool {
	return a.Currency == "" || a.Currency == "XRP"
}

// Result represents a transaction result code.
type Result int

// Result codes
const (
	TesSUCCESS             Result = 0
	TefINTERNAL            Result = -199
	TefPAST_SEQ            Result = -198
	TefMAX_LEDGER          Result = -197
	TerPRE_SEQ             Result = -92
	TerINSUF_FEE_B         Result = -91
	TerNO_ACCOUNT          Result = -90
	TemBAD_SRC_ACCOUNT     Result = -96
	TemINVALID             Result = -95
	TemBAD_FEE             Result = -94
	TemBAD_SEQUENCE        Result = -93
	TemBAD_SIGNATURE       Result = -92
	TemBAD_AMOUNT          Result = -91
	TemDST_NEEDED          Result = -90
	TemDST_IS_SRC          Result = -89
	TemBAD_ISSUER          Result = -88
	TecNO_DST              Result = 124
	TecNO_DST_INSUF_XRP    Result = 125
	TecDST_TAG_NEEDED      Result = 143
	TecUNFUNDED_PAYMENT    Result = 104
	TecPATH_DRY            Result = 128
	TecPATH_PARTIAL        Result = 101
	TecNO_ISSUER           Result = 133
	TecNO_ALTERNATIVE_KEY  Result = 130
	TecINSUF_RESERVE_LINE  Result = 122
	TecINSUF_RESERVE_OFFER Result = 123
	TecKILLED              Result = 150
)

// IsSuccess returns true if the result indicates success.
func (r Result) IsSuccess() bool {
	return r == TesSUCCESS
}

// IsTec returns true if the result is a tec code (claimed but not applied).
func (r Result) IsTec() bool {
	return r >= 100 && r < 200
}

// IsApplied returns true if the transaction should be applied.
func (r Result) IsApplied() bool {
	return r.IsSuccess() || r.IsTec()
}

// Message returns a human-readable message for the result.
func (r Result) Message() string {
	switch r {
	case TesSUCCESS:
		return "The transaction was applied."
	case TefINTERNAL:
		return "Internal error."
	case TefPAST_SEQ:
		return "The sequence number is in the past."
	case TefMAX_LEDGER:
		return "Ledger sequence too high."
	case TerPRE_SEQ:
		return "The sequence number is in the future."
	case TerINSUF_FEE_B:
		return "Insufficient balance to pay fee."
	case TerNO_ACCOUNT:
		return "The source account does not exist."
	case TemBAD_SRC_ACCOUNT:
		return "Invalid source account."
	case TemINVALID:
		return "The transaction is malformed."
	case TemBAD_FEE:
		return "Invalid fee."
	case TemBAD_SEQUENCE:
		return "Invalid sequence number."
	case TemBAD_SIGNATURE:
		return "Invalid signature."
	case TemBAD_AMOUNT:
		return "Invalid amount."
	case TemDST_NEEDED:
		return "Destination required."
	case TecNO_DST:
		return "Destination does not exist."
	case TecNO_DST_INSUF_XRP:
		return "Insufficient XRP to create destination."
	case TecDST_TAG_NEEDED:
		return "Destination tag required."
	case TecUNFUNDED_PAYMENT:
		return "Insufficient funds."
	case TecPATH_DRY:
		return "No path found."
	case TecPATH_PARTIAL:
		return "Path only partially found."
	default:
		return "Unknown result."
	}
}
