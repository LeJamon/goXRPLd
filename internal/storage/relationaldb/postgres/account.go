package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/storage/relationaldb"
)

// GetAccountTransactionsMinLedgerSeq returns the minimum ledger sequence with account transactions
func (db *PostgresDatabase) GetAccountTransactionsMinLedgerSeq(ctx context.Context) (*relationaldb.LedgerIndex, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	var seq sql.NullInt64
	err := db.db.QueryRowContext(ctx, "SELECT MIN(ledger_seq) FROM account_transactions").Scan(&seq)
	if err != nil {
		return nil, relationaldb.NewQueryError("get_account_transactions_min_ledger_seq", "failed to query min account transaction ledger sequence", err)
	}

	if !seq.Valid {
		return nil, nil
	}

	result := relationaldb.LedgerIndex(seq.Int64)
	return &result, nil
}

// GetAccountTransactionCount returns the total number of account transactions
func (db *PostgresDatabase) GetAccountTransactionCount(ctx context.Context) (int64, error) {
	if db.db == nil {
		return 0, relationaldb.ErrDatabaseClosed
	}

	var count int64
	err := db.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM account_transactions").Scan(&count)
	if err != nil {
		return 0, relationaldb.NewQueryError("get_account_transaction_count", "failed to count account transactions", err)
	}

	return count, nil
}

// GetOldestAccountTxs retrieves the oldest account transactions
func (db *PostgresDatabase) GetOldestAccountTxs(ctx context.Context, options relationaldb.AccountTxOptions) ([]relationaldb.TransactionInfo, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	return db.getAccountTxs(ctx, options, "ASC")
}

// GetNewestAccountTxs retrieves the newest account transactions
func (db *PostgresDatabase) GetNewestAccountTxs(ctx context.Context, options relationaldb.AccountTxOptions) ([]relationaldb.TransactionInfo, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	return db.getAccountTxs(ctx, options, "DESC")
}

// getAccountTxs is a helper method for getting account transactions with specified order
func (db *PostgresDatabase) getAccountTxs(ctx context.Context, options relationaldb.AccountTxOptions, order string) ([]relationaldb.TransactionInfo, error) {
	query := `SELECT t.trans_id, t.ledger_seq, t.status, t.raw_txn, t.txn_meta, at.txn_seq
			  FROM account_transactions at
			  INNER JOIN transactions t ON t.trans_id = at.trans_id
			  WHERE at.account = $1`

	args := []interface{}{options.Account.String()}
	argCount := 1

	if options.MinLedger > 0 {
		argCount++
		query += fmt.Sprintf(" AND at.ledger_seq >= $%d", argCount)
		args = append(args, options.MinLedger)
	}

	if options.MaxLedger > 0 {
		argCount++
		query += fmt.Sprintf(" AND at.ledger_seq <= $%d", argCount)
		args = append(args, options.MaxLedger)
	}

	query += fmt.Sprintf(" ORDER BY at.ledger_seq %s, at.txn_seq %s", order, order)

	if !options.Unlimited && options.Limit > 0 {
		argCount++
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, options.Limit)

		if options.Offset > 0 {
			argCount++
			query += fmt.Sprintf(" OFFSET $%d", argCount)
			args = append(args, options.Offset)
		}
	}

	rows, err := db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, relationaldb.NewQueryError("get_account_txs", "failed to query account transactions", err)
	}
	defer rows.Close()

	var results []relationaldb.TransactionInfo

	for rows.Next() {
		var info relationaldb.TransactionInfo
		var hashBytes []byte
		var txnMeta sql.NullString

		if err := rows.Scan(&hashBytes, &info.LedgerSeq, &info.Status, &info.RawTxn, &txnMeta, &info.TxnSeq); err != nil {
			return nil, relationaldb.NewQueryError("get_account_txs", "failed to scan row", err)
		}

		copy(info.Hash[:], hashBytes)
		copy(info.Account[:], options.Account[:])
		if txnMeta.Valid {
			info.TxnMeta = []byte(txnMeta.String)
		}
		results = append(results, info)
	}

	if err := rows.Err(); err != nil {
		return nil, relationaldb.NewQueryError("get_account_txs", "error iterating rows", err)
	}

	return results, nil
}

// GetOldestAccountTxsPage retrieves paginated oldest account transactions
func (db *PostgresDatabase) GetOldestAccountTxsPage(ctx context.Context, options relationaldb.AccountTxPageOptions) (*relationaldb.AccountTxResult, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	return db.getAccountTxsPage(ctx, options, "ASC", ">")
}

// GetNewestAccountTxsPage retrieves paginated newest account transactions
func (db *PostgresDatabase) GetNewestAccountTxsPage(ctx context.Context, options relationaldb.AccountTxPageOptions) (*relationaldb.AccountTxResult, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	return db.getAccountTxsPage(ctx, options, "DESC", "<")
}

// getAccountTxsPage is a helper method for paginated account transaction queries
func (db *PostgresDatabase) getAccountTxsPage(ctx context.Context, options relationaldb.AccountTxPageOptions, order, markerOp string) (*relationaldb.AccountTxResult, error) {
	query := `SELECT t.trans_id, t.ledger_seq, t.status, t.raw_txn, t.txn_meta, at.txn_seq
			  FROM account_transactions at
			  INNER JOIN transactions t ON t.trans_id = at.trans_id
			  WHERE at.account = $1`

	args := []interface{}{options.Account.String()}
	argCount := 1

	if options.MinLedger > 0 {
		argCount++
		query += fmt.Sprintf(" AND at.ledger_seq >= $%d", argCount)
		args = append(args, options.MinLedger)
	}

	if options.MaxLedger > 0 {
		argCount++
		query += fmt.Sprintf(" AND at.ledger_seq <= $%d", argCount)
		args = append(args, options.MaxLedger)
	}

	// Add marker-based pagination
	if options.Marker != nil {
		argCount++
		query += fmt.Sprintf(" AND (at.ledger_seq %s $%d OR (at.ledger_seq = $%d AND at.txn_seq %s $%d))",
			markerOp, argCount, argCount, markerOp, argCount+1)
		args = append(args, options.Marker.LedgerSeq, options.Marker.TxnSeq)
		argCount++
	}

	query += fmt.Sprintf(" ORDER BY at.ledger_seq %s, at.txn_seq %s", order, order)

	// Fetch one extra to determine if there are more results
	limit := options.Limit + 1
	argCount++
	query += fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, limit)

	rows, err := db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, relationaldb.NewQueryError("get_account_txs_page", "failed to query account transactions", err)
	}
	defer rows.Close()

	var transactions []relationaldb.TransactionInfo

	for rows.Next() {
		var info relationaldb.TransactionInfo
		var hashBytes []byte
		var txnMeta sql.NullString

		if err := rows.Scan(&hashBytes, &info.LedgerSeq, &info.Status, &info.RawTxn, &txnMeta, &info.TxnSeq); err != nil {
			return nil, relationaldb.NewQueryError("get_account_txs_page", "failed to scan row", err)
		}

		copy(info.Hash[:], hashBytes)
		copy(info.Account[:], options.Account[:])
		if txnMeta.Valid {
			info.TxnMeta = []byte(txnMeta.String)
		}
		transactions = append(transactions, info)
	}

	if err := rows.Err(); err != nil {
		return nil, relationaldb.NewQueryError("get_account_txs_page", "error iterating rows", err)
	}

	result := &relationaldb.AccountTxResult{
		LedgerRange: relationaldb.LedgerRange{
			Min: options.MinLedger,
			Max: options.MaxLedger,
		},
		Limit: options.Limit,
	}

	// Check if there are more results
	if len(transactions) > int(options.Limit) {
		// Remove the extra transaction and set marker
		transactions = transactions[:options.Limit]
		lastTx := transactions[len(transactions)-1]
		result.Marker = &relationaldb.AccountTxMarker{
			LedgerSeq: lastTx.LedgerSeq,
			TxnSeq:    lastTx.TxnSeq,
		}
	}

	result.Transactions = transactions
	return result, nil
}

// DeleteAccountTransactionsBeforeLedgerSeq deletes account transactions before a specified ledger sequence
func (db *PostgresDatabase) DeleteAccountTransactionsBeforeLedgerSeq(ctx context.Context, ledgerSeq relationaldb.LedgerIndex) error {
	if db.db == nil {
		return relationaldb.ErrDatabaseClosed
	}

	_, err := db.db.ExecContext(ctx, "DELETE FROM account_transactions WHERE ledger_seq < $1", ledgerSeq)
	if err != nil {
		return relationaldb.NewQueryError("delete_account_transactions_before_ledger_seq", "failed to delete account transactions", err)
	}

	return nil
}