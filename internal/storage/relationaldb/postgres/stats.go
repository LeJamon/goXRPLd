package postgres

import (
	"context"
	"database/sql"

	"github.com/LeJamon/goXRPLd/internal/storage/relationaldb"
)

// GetLedgerCountMinMax retrieves ledger count and range statistics
func (db *PostgresDatabase) GetLedgerCountMinMax(ctx context.Context) (*relationaldb.CountMinMax, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	var count int64
	var minSeq, maxSeq sql.NullInt64

	query := `SELECT COUNT(*), MIN(ledger_seq), MAX(ledger_seq) FROM ledgers`
	err := db.db.QueryRowContext(ctx, query).Scan(&count, &minSeq, &maxSeq)
	if err != nil {
		return nil, relationaldb.NewQueryError("get_ledger_count_min_max", "failed to query ledger statistics", err)
	}

	result := &relationaldb.CountMinMax{
		Count: count,
	}

	if minSeq.Valid {
		result.MinLedgerSeq = relationaldb.LedgerIndex(minSeq.Int64)
	}

	if maxSeq.Valid {
		result.MaxLedgerSeq = relationaldb.LedgerIndex(maxSeq.Int64)
	}

	return result, nil
}

// GetKBUsedAll returns the total database size in KB
func (db *PostgresDatabase) GetKBUsedAll(ctx context.Context) (uint32, error) {
	if db.db == nil {
		return 0, relationaldb.ErrDatabaseClosed
	}

	var size int64
	err := db.db.QueryRowContext(ctx,
		"SELECT pg_database_size(current_database())").Scan(&size)

	if err != nil {
		return 0, relationaldb.NewQueryError("get_kb_used_all", "failed to get database size", err)
	}

	return uint32(size / 1024), nil
}

// GetKBUsedLedger returns the ledger table size in KB
func (db *PostgresDatabase) GetKBUsedLedger(ctx context.Context) (uint32, error) {
	if db.db == nil {
		return 0, relationaldb.ErrDatabaseClosed
	}

	var size int64
	err := db.db.QueryRowContext(ctx,
		"SELECT pg_total_relation_size('ledgers')").Scan(&size)

	if err != nil {
		return 0, relationaldb.NewQueryError("get_kb_used_ledger", "failed to get ledgers table size", err)
	}

	return uint32(size / 1024), nil
}

// GetKBUsedTransaction returns the transaction tables size in KB
func (db *PostgresDatabase) GetKBUsedTransaction(ctx context.Context) (uint32, error) {
	if db.db == nil {
		return 0, relationaldb.ErrDatabaseClosed
	}

	var size int64
	err := db.db.QueryRowContext(ctx,
		"SELECT pg_total_relation_size('transactions') + pg_total_relation_size('account_transactions')").Scan(&size)

	if err != nil {
		return 0, relationaldb.NewQueryError("get_kb_used_transaction", "failed to get transaction tables size", err)
	}

	return uint32(size / 1024), nil
}

// HasLedgerSpace checks if there's space available for ledger data
func (db *PostgresDatabase) HasLedgerSpace(ctx context.Context) (bool, error) {
	if db.db == nil {
		return false, relationaldb.ErrDatabaseClosed
	}

	// For PostgreSQL, we assume space is managed by the database server
	// In production, you might want to implement actual disk space checks
	return true, nil
}

// HasTransactionSpace checks if there's space available for transaction data
func (db *PostgresDatabase) HasTransactionSpace(ctx context.Context) (bool, error) {
	// Same check as ledger space for PostgreSQL
	return db.HasLedgerSpace(ctx)
}