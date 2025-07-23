package postgres

import (
	"context"
	"database/sql"

	"github.com/LeJamon/goXRPLd/internal/storage/relationaldb"
)

// GetTransactionsMinLedgerSeq returns the minimum ledger sequence with transactions
func (db *PostgresDatabase) GetTransactionsMinLedgerSeq(ctx context.Context) (*relationaldb.LedgerIndex, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	var seq sql.NullInt64
	err := db.db.QueryRowContext(ctx, "SELECT MIN(ledger_seq) FROM transactions").Scan(&seq)
	if err != nil {
		return nil, relationaldb.NewQueryError("get_transactions_min_ledger_seq", "failed to query min transaction ledger sequence", err)
	}

	if !seq.Valid {
		return nil, nil
	}

	result := relationaldb.LedgerIndex(seq.Int64)
	return &result, nil
}

// GetTransactionCount returns the total number of transactions
func (db *PostgresDatabase) GetTransactionCount(ctx context.Context) (int64, error) {
	if db.db == nil {
		return 0, relationaldb.ErrDatabaseClosed
	}

	var count int64
	err := db.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM transactions").Scan(&count)
	if err != nil {
		return 0, relationaldb.NewQueryError("get_transaction_count", "failed to count transactions", err)
	}

	return count, nil
}

// GetTransaction retrieves a specific transaction by hash
func (db *PostgresDatabase) GetTransaction(ctx context.Context, hash relationaldb.Hash, ledgerRange *relationaldb.LedgerRange) (*relationaldb.TransactionInfo, relationaldb.TxSearchResult, error) {
	if db.db == nil {
		return nil, relationaldb.TxSearchUnknown, relationaldb.ErrDatabaseClosed
	}

	query := `SELECT trans_id, ledger_seq, status, raw_txn, txn_meta
			  FROM transactions WHERE trans_id = $1`

	var info relationaldb.TransactionInfo
	var hashBytes []byte
	var txnMeta sql.NullString

	err := db.db.QueryRowContext(ctx, query, hash[:]).Scan(
		&hashBytes, &info.LedgerSeq, &info.Status, &info.RawTxn, &txnMeta)

	if err == sql.ErrNoRows {
		if ledgerRange != nil {
			// Check if all ledgers in range are present (rippled behavior)
			var count int64
			countQuery := `SELECT COUNT(DISTINCT ledger_seq) FROM ledgers 
						   WHERE ledger_seq >= $1 AND ledger_seq <= $2`
			if err := db.db.QueryRowContext(ctx, countQuery, ledgerRange.Min, ledgerRange.Max).Scan(&count); err != nil {
				return nil, relationaldb.TxSearchUnknown, relationaldb.NewQueryError("get_transaction", "failed to count ledgers in range", err)
			}

			expectedCount := int64(ledgerRange.Max - ledgerRange.Min + 1)
			if count == expectedCount {
				return nil, relationaldb.TxSearchAll, nil
			}
			return nil, relationaldb.TxSearchSome, nil
		}
		return nil, relationaldb.TxSearchUnknown, nil
	}

	if err != nil {
		return nil, relationaldb.TxSearchUnknown, relationaldb.NewQueryError("get_transaction", "failed to query transaction", err)
	}

	copy(info.Hash[:], hashBytes)
	if txnMeta.Valid {
		info.TxnMeta = []byte(txnMeta.String)
	}

	return &info, relationaldb.TxSearchAll, nil
}

// GetTxHistory retrieves transaction history starting from a given index
func (db *PostgresDatabase) GetTxHistory(ctx context.Context, startIndex relationaldb.LedgerIndex, limit int) ([]relationaldb.TransactionInfo, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	// Match rippled's getTxHistory behavior - most recent transactions
	query := `SELECT trans_id, ledger_seq, status, raw_txn, txn_meta
			  FROM transactions 
			  ORDER BY ledger_seq DESC
			  OFFSET $1 LIMIT $2`

	rows, err := db.db.QueryContext(ctx, query, startIndex, limit)
	if err != nil {
		return nil, relationaldb.NewQueryError("get_tx_history", "failed to query transaction history", err)
	}
	defer rows.Close()

	var results []relationaldb.TransactionInfo

	for rows.Next() {
		var info relationaldb.TransactionInfo
		var hashBytes []byte
		var txnMeta sql.NullString

		if err := rows.Scan(&hashBytes, &info.LedgerSeq, &info.Status, &info.RawTxn, &txnMeta); err != nil {
			return nil, relationaldb.NewQueryError("get_tx_history", "failed to scan row", err)
		}

		copy(info.Hash[:], hashBytes)
		if txnMeta.Valid {
			info.TxnMeta = []byte(txnMeta.String)
		}
		results = append(results, info)
	}

	if err := rows.Err(); err != nil {
		return nil, relationaldb.NewQueryError("get_tx_history", "error iterating rows", err)
	}

	return results, nil
}

// DeleteTransactionsByLedgerSeq deletes transactions for a specific ledger sequence
func (db *PostgresDatabase) DeleteTransactionsByLedgerSeq(ctx context.Context, ledgerSeq relationaldb.LedgerIndex) error {
	if db.db == nil {
		return relationaldb.ErrDatabaseClosed
	}

	// Delete from both tables, matching rippled's behavior
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return relationaldb.NewTransactionError("delete_transactions_by_ledger_seq", "failed to begin transaction", err)
	}
	defer tx.Rollback()

	// Delete account transactions first (foreign key constraint)
	if _, err := tx.ExecContext(ctx, "DELETE FROM account_transactions WHERE ledger_seq = $1", ledgerSeq); err != nil {
		return relationaldb.NewQueryError("delete_transactions_by_ledger_seq", "failed to delete account transactions", err)
	}

	// Delete transactions
	if _, err := tx.ExecContext(ctx, "DELETE FROM transactions WHERE ledger_seq = $1", ledgerSeq); err != nil {
		return relationaldb.NewQueryError("delete_transactions_by_ledger_seq", "failed to delete transactions", err)
	}

	if err := tx.Commit(); err != nil {
		return relationaldb.NewTransactionError("delete_transactions_by_ledger_seq", "failed to commit transaction", err)
	}

	return nil
}

// DeleteTransactionsBeforeLedgerSeq deletes transactions before a specified ledger sequence
func (db *PostgresDatabase) DeleteTransactionsBeforeLedgerSeq(ctx context.Context, ledgerSeq relationaldb.LedgerIndex) error {
	if db.db == nil {
		return relationaldb.ErrDatabaseClosed
	}

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return relationaldb.NewTransactionError("delete_transactions_before_ledger_seq", "failed to begin transaction", err)
	}
	defer tx.Rollback()

	// Delete account transactions first
	if _, err := tx.ExecContext(ctx, "DELETE FROM account_transactions WHERE ledger_seq < $1", ledgerSeq); err != nil {
		return relationaldb.NewQueryError("delete_transactions_before_ledger_seq", "failed to delete account transactions", err)
	}

	// Delete transactions
	if _, err := tx.ExecContext(ctx, "DELETE FROM transactions WHERE ledger_seq < $1", ledgerSeq); err != nil {
		return relationaldb.NewQueryError("delete_transactions_before_ledger_seq", "failed to delete transactions", err)
	}

	if err := tx.Commit(); err != nil {
		return relationaldb.NewTransactionError("delete_transactions_before_ledger_seq", "failed to commit transaction", err)
	}

	return nil
}
