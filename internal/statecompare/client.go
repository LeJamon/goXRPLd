// Package statecompare provides a client for reading from the xrpl-state-compare PostgreSQL database.
// This is used for continuous replay testing, loading state and transactions from the database
// rather than from fixture files.
package statecompare

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// Client provides access to the xrpl-state-compare PostgreSQL database.
type Client struct {
	db *sql.DB
}

// LedgerSnapshot represents a ledger snapshot from the database.
type LedgerSnapshot struct {
	LedgerIndex         uint32
	LedgerHash          [32]byte
	ParentHash          [32]byte
	AccountHash         [32]byte
	TransactionHash     [32]byte
	TotalCoins          uint64
	CloseTime           int64
	CloseTimeResolution uint32
	CloseFlags          uint8
}

// StateEntry represents a state entry from the database.
type StateEntry struct {
	Index [32]byte
	Data  []byte
}

// Transaction represents a transaction from the database.
type Transaction struct {
	TxIndex  int
	TxHash   [32]byte
	TxBlob   []byte
	MetaBlob []byte
}

// Config holds the database configuration.
type Config struct {
	Host     string
	Port     string
	Database string
	User     string
	Password string
}

// ConfigFromEnv creates a Config from environment variables.
// Uses the same env vars as the Python xrpl-state-compare tool.
func ConfigFromEnv() Config {
	return Config{
		Host:     getEnvOrDefault("POSTGRES_HOST", "localhost"),
		Port:     getEnvOrDefault("POSTGRES_PORT", "5432"),
		Database: getEnvOrDefault("POSTGRES_DB", "xrpl_state"),
		User:     getEnvOrDefault("POSTGRES_USER", "postgres"),
		Password: getEnvOrDefault("POSTGRES_PASSWORD", "postgres"),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// NewClient creates a new database client from config.
func NewClient(cfg Config) (*Client, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%s dbname=%s user=%s password=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.Database, cfg.User, cfg.Password,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	return &Client{db: db}, nil
}

// NewClientFromEnv creates a new database client using environment variables.
func NewClientFromEnv() (*Client, error) {
	return NewClient(ConfigFromEnv())
}

// Close closes the database connection.
func (c *Client) Close() error {
	return c.db.Close()
}

// GetSnapshot retrieves a ledger snapshot by index.
func (c *Client) GetSnapshot(ctx context.Context, ledgerIndex uint32) (*LedgerSnapshot, error) {
	query := `
		SELECT ledger_index, ledger_hash, parent_hash, account_hash, transaction_hash,
		       total_coins, close_time, close_time_resolution, close_flags
		FROM ledger_snapshots
		WHERE ledger_index = $1
	`

	var snapshot LedgerSnapshot
	var ledgerHash, parentHash, accountHash, txHash []byte

	err := c.db.QueryRowContext(ctx, query, ledgerIndex).Scan(
		&snapshot.LedgerIndex,
		&ledgerHash,
		&parentHash,
		&accountHash,
		&txHash,
		&snapshot.TotalCoins,
		&snapshot.CloseTime,
		&snapshot.CloseTimeResolution,
		&snapshot.CloseFlags,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ledger %d not found", ledgerIndex)
	}
	if err != nil {
		return nil, fmt.Errorf("querying snapshot: %w", err)
	}

	copy(snapshot.LedgerHash[:], ledgerHash)
	copy(snapshot.ParentHash[:], parentHash)
	copy(snapshot.AccountHash[:], accountHash)
	copy(snapshot.TransactionHash[:], txHash)

	return &snapshot, nil
}

// GetStateEntries retrieves all state entries for a ledger.
func (c *Client) GetStateEntries(ctx context.Context, ledgerIndex uint32) ([]StateEntry, error) {
	query := `
		SELECT entry_index, data
		FROM ledger_state
		WHERE ledger_index = $1
		ORDER BY entry_index
	`

	rows, err := c.db.QueryContext(ctx, query, ledgerIndex)
	if err != nil {
		return nil, fmt.Errorf("querying state entries: %w", err)
	}
	defer rows.Close()

	var entries []StateEntry
	for rows.Next() {
		var indexBytes []byte
		var data []byte

		if err := rows.Scan(&indexBytes, &data); err != nil {
			return nil, fmt.Errorf("scanning state entry: %w", err)
		}

		var entry StateEntry
		copy(entry.Index[:], indexBytes)
		entry.Data = data
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating state entries: %w", err)
	}

	return entries, nil
}

// GetTransactions retrieves all transactions for a ledger.
func (c *Client) GetTransactions(ctx context.Context, ledgerIndex uint32) ([]Transaction, error) {
	query := `
		SELECT tx_index, tx_hash, tx_blob, meta_blob
		FROM ledger_transactions
		WHERE ledger_index = $1
		ORDER BY tx_index
	`

	rows, err := c.db.QueryContext(ctx, query, ledgerIndex)
	if err != nil {
		return nil, fmt.Errorf("querying transactions: %w", err)
	}
	defer rows.Close()

	var txs []Transaction
	for rows.Next() {
		var hashBytes []byte
		var tx Transaction

		if err := rows.Scan(&tx.TxIndex, &hashBytes, &tx.TxBlob, &tx.MetaBlob); err != nil {
			return nil, fmt.Errorf("scanning transaction: %w", err)
		}

		copy(tx.TxHash[:], hashBytes)
		txs = append(txs, tx)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating transactions: %w", err)
	}

	return txs, nil
}

// HasLedger checks if a ledger exists in the database.
func (c *Client) HasLedger(ctx context.Context, ledgerIndex uint32) (bool, error) {
	var exists bool
	err := c.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM ledger_snapshots WHERE ledger_index = $1)",
		ledgerIndex,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking ledger existence: %w", err)
	}
	return exists, nil
}

// GetStateEntryCount returns the number of state entries for a ledger.
func (c *Client) GetStateEntryCount(ctx context.Context, ledgerIndex uint32) (int, error) {
	var count int
	err := c.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM ledger_state WHERE ledger_index = $1",
		ledgerIndex,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting state entries: %w", err)
	}
	return count, nil
}

// GetTransactionCount returns the number of transactions for a ledger.
func (c *Client) GetTransactionCount(ctx context.Context, ledgerIndex uint32) (int, error) {
	var count int
	err := c.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM ledger_transactions WHERE ledger_index = $1",
		ledgerIndex,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting transactions: %w", err)
	}
	return count, nil
}

// ValidateRange checks that all ledgers in the given range exist in the database.
// Returns the first missing ledger index if any are missing.
func (c *Client) ValidateRange(ctx context.Context, from, to uint32) (bool, uint32, error) {
	for i := from; i <= to; i++ {
		exists, err := c.HasLedger(ctx, i)
		if err != nil {
			return false, i, err
		}
		if !exists {
			return false, i, nil
		}
	}
	return true, 0, nil
}
