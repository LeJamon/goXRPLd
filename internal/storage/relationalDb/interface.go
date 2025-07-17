package relational_db

import (
	"context"
	"time"

	"github.com/your-project/xrpl-go/types"
)

// LedgerIndex represents a ledger sequence number
type LedgerIndex uint32

// AccountTxOptions contains criteria for querying account transactions
type AccountTxOptions struct {
	Account        types.AccountID
	LedgerMinSeq   *LedgerIndex
	LedgerMaxSeq   *LedgerIndex
	Offset         uint32
	Limit          uint32
	UnlimitedLimit bool
	Forward        bool // true for ascending, false for descending
}

// AccountTxPageOptions contains criteria for paginated account transaction queries
type AccountTxPageOptions struct {
	Account        types.AccountID
	LedgerMinSeq   *LedgerIndex
	LedgerMaxSeq   *LedgerIndex
	Marker         *AccountTxMarker
	Limit          uint32
	UnlimitedLimit bool
	Forward        bool
}

// AccountTxMarker represents a pagination marker for account transactions
type AccountTxMarker struct {
	LedgerSeq  LedgerIndex
	TxnSeq     uint32
	AccountSeq uint32
}

// AccountTx represents a transaction and its metadata
type AccountTx struct {
	Transaction []byte
	Metadata    []byte
	LedgerSeq   LedgerIndex
	TxnSeq      uint32
}

// AccountTxs is a slice of account transactions
type AccountTxs []AccountTx

// MetaTxsList represents transactions in binary form with account sequences
type MetaTxsList []struct {
	Transaction []byte
	Metadata    []byte
	AccountSeq  uint32
	LedgerSeq   LedgerIndex
}

// LedgerInfo contains information about a ledger
type LedgerInfo struct {
	Hash            types.Hash256
	LedgerSeq       LedgerIndex
	ParentHash      types.Hash256
	TotalCoins      uint64
	ClosingTime     time.Time
	PrevClosingTime time.Time
	CloseTimeRes    uint32
	CloseFlags      uint32
	AccountHash     types.Hash256
	TxHash          types.Hash256
}

// CountMinMax contains min/max counts and totals
type CountMinMax struct {
	Min   LedgerIndex
	Max   LedgerIndex
	Count uint64
}

// TxSearched represents the result of a transaction search
type TxSearched int

const (
	TxSearchedUnknown TxSearched = iota
	TxSearchedSome
	TxSearchedAll
)

// ClosedInterval represents a closed interval [min, max]
type ClosedInterval[T any] struct {
	Min T
	Max T
}

// RelationalDatabase defines the interface for relational database operations
// This interface abstracts the underlying database implementation
type RelationalDatabase interface {
	// Lifecycle management
	Open(ctx context.Context) error
	Close(ctx context.Context) error
	IsOpen() bool

	// Transaction management
	Begin(ctx context.Context) (Transaction, error)

	// Ledger operations
	SaveValidatedLedger(ctx context.Context, ledger *types.Ledger, current bool) error
	GetLedgerInfoBySeq(ctx context.Context, seq LedgerIndex) (*LedgerInfo, error)
	GetLedgerInfoByHash(ctx context.Context, hash types.Hash256) (*LedgerInfo, error)
	GetLimitedOldestLedgerInfo(ctx context.Context, ledgerFirstIndex LedgerIndex) (*LedgerInfo, error)
	GetLimitedNewestLedgerInfo(ctx context.Context, ledgerFirstIndex LedgerIndex) (*LedgerInfo, error)
	GetLedgerCountMinMax(ctx context.Context) (*CountMinMax, error)
	DeleteBeforeLedgerSeq(ctx context.Context, ledgerSeq LedgerIndex) error

	// Transaction operations
	GetTransaction(ctx context.Context, id types.Hash256, ledgerRange *ClosedInterval[uint32]) (interface{}, error) // Returns AccountTx or TxSearched
	GetTransactionCount(ctx context.Context) (uint64, error)
	GetTransactionsMinLedgerSeq(ctx context.Context) (*LedgerIndex, error)
	DeleteTransactionByLedgerSeq(ctx context.Context, ledgerSeq LedgerIndex) error
	DeleteTransactionsBeforeLedgerSeq(ctx context.Context, ledgerSeq LedgerIndex) error

	// Account transaction operations
	GetOldestAccountTxs(ctx context.Context, options *AccountTxOptions) (AccountTxs, error)
	GetNewestAccountTxs(ctx context.Context, options *AccountTxOptions) (AccountTxs, error)
	GetOldestAccountTxsB(ctx context.Context, options *AccountTxOptions) (MetaTxsList, error)
	GetNewestAccountTxsB(ctx context.Context, options *AccountTxOptions) (MetaTxsList, error)

	// Paginated account transaction operations
	OldestAccountTxPage(ctx context.Context, options *AccountTxPageOptions) (AccountTxs, *AccountTxMarker, error)
	NewestAccountTxPage(ctx context.Context, options *AccountTxPageOptions) (AccountTxs, *AccountTxMarker, error)
	OldestAccountTxPageB(ctx context.Context, options *AccountTxPageOptions) (MetaTxsList, *AccountTxMarker, error)
	NewestAccountTxPageB(ctx context.Context, options *AccountTxPageOptions) (MetaTxsList, *AccountTxMarker, error)

	GetAccountTransactionCount(ctx context.Context) (uint64, error)
	GetAccountTransactionsMinLedgerSeq(ctx context.Context) (*LedgerIndex, error)
	DeleteAccountTransactionsBeforeLedgerSeq(ctx context.Context, ledgerSeq LedgerIndex) error

	// Storage statistics
	GetKBUsedAll(ctx context.Context) (uint32, error)
	GetKBUsedLedger(ctx context.Context) (uint32, error)
	GetKBUsedTransaction(ctx context.Context) (uint32, error)

	// Database-specific operations
	CloseLedgerDB(ctx context.Context) error
	CloseTransactionDB(ctx context.Context) error
}

// Transaction represents a database transaction
type Transaction interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error

	// RelationalDatabase All the same methods as RelationalDatabase but within transaction context
	RelationalDatabase
}

// DatabaseConfig holds configuration for database connections
type DatabaseConfig struct {
	Path                  string
	MaxOpenConnections    int
	MaxIdleConnections    int
	ConnectionMaxLifetime time.Duration
	ConnectionMaxIdleTime time.Duration
	EnableWAL             bool
	EnableForeignKeys     bool
	BusyTimeout           time.Duration
	PragmaSettings        map[string]string
}

type DatabaseDriver string

const (
	DriverSQLite   DatabaseDriver = "sqlite3"
	DriverPostgres DatabaseDriver = "postgres"
	DriverMySQL    DatabaseDriver = "mysql"
)
