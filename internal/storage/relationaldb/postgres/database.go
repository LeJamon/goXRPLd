package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/storage/relationaldb"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgresDatabase implements the Database interface for PostgreSQL
// Based on rippled's SQLiteDatabase but adapted for PostgreSQL
type PostgresDatabase struct {
	db     *sql.DB
	config *relationaldb.Config
}

// NewDatabase creates a new PostgreSQL database instance
func NewDatabase(config *relationaldb.Config) (relationaldb.Database, error) {
	if err := config.Validate(); err != nil {
		return nil, relationaldb.NewConfigurationError("new_database", "invalid configuration", err)
	}

	return &PostgresDatabase{
		config: config,
	}, nil
}

// Open opens the database connection and initializes schema
func (db *PostgresDatabase) Open(ctx context.Context) error {
	connStr, err := db.config.BuildConnectionString()
	if err != nil {
		return relationaldb.NewConfigurationError("open", "failed to build connection string", err)
	}

	sqlDB, err := sql.Open(db.config.Driver, connStr)
	if err != nil {
		return relationaldb.NewConnectionError("open", "failed to open database connection", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(db.config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(db.config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(db.config.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(db.config.ConnMaxIdleTime)

	// Test connection
	ctx, cancel := context.WithTimeout(ctx, db.config.DefaultTimeout)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return relationaldb.NewConnectionError("open", "failed to ping database", err)
	}

	db.db = sqlDB

	// Initialize schema (matches rippled's table structure)
	if err := db.initSchema(ctx); err != nil {
		db.db.Close()
		db.db = nil
		return relationaldb.NewSchemaError("open", "failed to initialize schema", err)
	}

	return nil
}

// Close closes the database connection
func (db *PostgresDatabase) Close(ctx context.Context) error {
	if db.db == nil {
		return nil
	}

	err := db.db.Close()
	db.db = nil

	if err != nil {
		return relationaldb.NewConnectionError("close", "failed to close database connection", err)
	}

	return nil
}

// Ping tests the database connection
func (db *PostgresDatabase) Ping(ctx context.Context) error {
	if db.db == nil {
		return relationaldb.ErrDatabaseClosed
	}

	ctx, cancel := context.WithTimeout(ctx, db.config.DefaultTimeout)
	defer cancel()

	if err := db.db.PingContext(ctx); err != nil {
		return relationaldb.NewConnectionError("ping", "database ping failed", err)
	}

	return nil
}

// Begin starts a new transaction
func (db *PostgresDatabase) Begin(ctx context.Context) (relationaldb.Transaction, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	ctx, cancel := context.WithTimeout(ctx, db.config.DefaultTimeout)
	defer cancel()

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, relationaldb.NewTransactionError("begin", "failed to begin transaction", err)
	}

	return &PostgresTransaction{
		tx:     tx,
		config: db.config,
	}, nil
}

// initSchema initializes the database schema matching rippled's structure
func (db *PostgresDatabase) initSchema(ctx context.Context) error {
	// Schema based on rippled's SQLite tables but adapted for PostgreSQL
	queries := []string{
		// Ledgers table - matches rippled's Ledgers table structure
		`CREATE TABLE IF NOT EXISTS ledgers (
			ledger_hash BYTEA PRIMARY KEY,
			ledger_seq BIGINT UNIQUE NOT NULL,
			prev_hash BYTEA NOT NULL,
			total_coins DECIMAL(20,0) NOT NULL,
			closing_time BIGINT NOT NULL,
			prev_closing_time BIGINT NOT NULL,
			close_time_res INTEGER NOT NULL,
			close_flags INTEGER NOT NULL,
			account_set_hash BYTEA NOT NULL,
			trans_set_hash BYTEA NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,

		// Transactions table - matches rippled's Transactions table
		`CREATE TABLE IF NOT EXISTS transactions (
			trans_id BYTEA PRIMARY KEY,
			ledger_seq BIGINT NOT NULL,
			status VARCHAR(50) NOT NULL,
			raw_txn BYTEA NOT NULL,
			txn_meta BYTEA,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,

		// AccountTransactions table - matches rippled's AccountTransactions table
		`CREATE TABLE IF NOT EXISTS account_transactions (
			trans_id BYTEA NOT NULL,
			account VARCHAR(34) NOT NULL,
			ledger_seq BIGINT NOT NULL,
			txn_seq INTEGER NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			PRIMARY KEY (trans_id, account)
		)`,

		// Indexes matching rippled's performance optimizations
		`CREATE INDEX IF NOT EXISTS idx_ledgers_seq ON ledgers(ledger_seq)`,
		`CREATE INDEX IF NOT EXISTS idx_ledgers_closing_time ON ledgers(closing_time)`,
		`CREATE INDEX IF NOT EXISTS idx_transactions_ledger_seq ON transactions(ledger_seq)`,
		`CREATE INDEX IF NOT EXISTS idx_account_transactions_account ON account_transactions(account)`,
		`CREATE INDEX IF NOT EXISTS idx_account_transactions_ledger_seq ON account_transactions(ledger_seq)`,
		`CREATE INDEX IF NOT EXISTS idx_account_transactions_account_ledger_txn ON account_transactions(account, ledger_seq, txn_seq)`,
	}

	for _, query := range queries {
		if _, err := db.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to execute schema query: %w", err)
		}
	}

	return nil
}

// CloseLedgerDB closes the ledger database connection.
// In PostgreSQL, this is equivalent to closing the main connection.
func (db *PostgresDatabase) CloseLedgerDB(ctx context.Context) error {
	return db.Close(ctx)
}

// CloseTransactionDB closes the transaction database connection.
// In PostgreSQL, ledger and transaction data share the same connection.
func (db *PostgresDatabase) CloseTransactionDB(ctx context.Context) error {
	return db.Close(ctx)
}