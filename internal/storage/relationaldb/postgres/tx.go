package postgres

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/LeJamon/goXRPLd/internal/storage/relationaldb"
)

// PostgresTransaction implements the Transaction interface for PostgreSQL
type PostgresTransaction struct {
	tx     *sql.Tx
	config *relationaldb.Config
}

// Commit commits the transaction
func (tx *PostgresTransaction) Commit(ctx context.Context) error {
	if tx.tx == nil {
		return relationaldb.ErrTransactionClosed
	}

	err := tx.tx.Commit()
	tx.tx = nil

	if err != nil {
		return relationaldb.NewTransactionError("commit", "failed to commit transaction", err)
	}

	return nil
}

// Rollback rolls back the transaction
func (tx *PostgresTransaction) Rollback(ctx context.Context) error {
	if tx.tx == nil {
		return nil // Already rolled back or committed
	}

	err := tx.tx.Rollback()
	tx.tx = nil

	if err != nil {
		return relationaldb.NewTransactionError("rollback", "failed to rollback transaction", err)
	}

	return nil
}

// GetMinLedgerSeq returns the minimum ledger sequence number within the transaction
func (tx *PostgresTransaction) GetMinLedgerSeq(ctx context.Context) (*relationaldb.LedgerIndex, error) {
	if tx.tx == nil {
		return nil, relationaldb.ErrTransactionClosed
	}

	var seq sql.NullInt64
	err := tx.tx.QueryRowContext(ctx, "SELECT MIN(ledger_seq) FROM ledgers").Scan(&seq)
	if err != nil {
		return nil, relationaldb.NewQueryError("get_min_ledger_seq", "failed to query min ledger sequence", err)
	}

	if !seq.Valid {
		return nil, nil
	}

	result := relationaldb.LedgerIndex(seq.Int64)
	return &result, nil
}

// GetMaxLedgerSeq returns the maximum ledger sequence number within the transaction
func (tx *PostgresTransaction) GetMaxLedgerSeq(ctx context.Context) (*relationaldb.LedgerIndex, error) {
	if tx.tx == nil {
		return nil, relationaldb.ErrTransactionClosed
	}

	var seq sql.NullInt64
	err := tx.tx.QueryRowContext(ctx, "SELECT MAX(ledger_seq) FROM ledgers").Scan(&seq)
	if err != nil {
		return nil, relationaldb.NewQueryError("get_max_ledger_seq", "failed to query max ledger sequence", err)
	}

	if !seq.Valid {
		return nil, nil
	}

	result := relationaldb.LedgerIndex(seq.Int64)
	return &result, nil
}

// SaveValidatedLedger saves a validated ledger within the transaction
func (tx *PostgresTransaction) SaveValidatedLedger(ctx context.Context, ledger *relationaldb.LedgerInfo, current bool) error {
	if tx.tx == nil {
		return relationaldb.ErrTransactionClosed
	}

	// Convert Go time back to rippled format
	closingTime := ledger.CloseTime.Unix() - 946684800
	prevClosingTime := ledger.ParentCloseTime.Unix() - 946684800

	query := `INSERT INTO ledgers (ledger_hash, ledger_seq, prev_hash, account_set_hash, trans_set_hash,
			  total_coins, closing_time, prev_closing_time, close_time_res, close_flags)
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			  ON CONFLICT (ledger_seq) DO UPDATE SET
			  ledger_hash = EXCLUDED.ledger_hash,
			  prev_hash = EXCLUDED.prev_hash,
			  account_set_hash = EXCLUDED.account_set_hash,
			  trans_set_hash = EXCLUDED.trans_set_hash,
			  total_coins = EXCLUDED.total_coins,
			  closing_time = EXCLUDED.closing_time,
			  prev_closing_time = EXCLUDED.prev_closing_time,
			  close_time_res = EXCLUDED.close_time_res,
			  close_flags = EXCLUDED.close_flags`

	_, err := tx.tx.ExecContext(ctx, query,
		ledger.Hash[:], ledger.Sequence, ledger.ParentHash[:], ledger.AccountHash[:], ledger.TransactionHash[:],
		strconv.FormatInt(int64(ledger.TotalCoins), 10), closingTime, prevClosingTime, ledger.CloseTimeRes, ledger.CloseFlags)

	if err != nil {
		return relationaldb.NewQueryError("save_validated_ledger", "failed to save ledger in transaction", err)
	}

	return nil
}

// GetLedgerInfoBySeq retrieves ledger information by sequence number within the transaction
func (tx *PostgresTransaction) GetLedgerInfoBySeq(ctx context.Context, seq relationaldb.LedgerIndex) (*relationaldb.LedgerInfo, error) {
	if tx.tx == nil {
		return nil, relationaldb.ErrTransactionClosed
	}

	query := `SELECT ledger_hash, ledger_seq, prev_hash, account_set_hash, trans_set_hash, 
			  total_coins, closing_time, prev_closing_time, close_time_res, close_flags
			  FROM ledgers WHERE ledger_seq = $1`

	return tx.scanLedgerInfo(ctx, query, seq)
}

// GetLedgerInfoByHash retrieves ledger information by hash within the transaction
func (tx *PostgresTransaction) GetLedgerInfoByHash(ctx context.Context, hash relationaldb.Hash) (*relationaldb.LedgerInfo, error) {
	if tx.tx == nil {
		return nil, relationaldb.ErrTransactionClosed
	}

	query := `SELECT ledger_hash, ledger_seq, prev_hash, account_set_hash, trans_set_hash, 
			  total_coins, closing_time, prev_closing_time, close_time_res, close_flags
			  FROM ledgers WHERE ledger_hash = $1`

	return tx.scanLedgerInfo(ctx, query, hash[:])
}

// GetNewestLedgerInfo retrieves the newest ledger information within the transaction
func (tx *PostgresTransaction) GetNewestLedgerInfo(ctx context.Context) (*relationaldb.LedgerInfo, error) {
	if tx.tx == nil {
		return nil, relationaldb.ErrTransactionClosed
	}

	query := `SELECT ledger_hash, ledger_seq, prev_hash, account_set_hash, trans_set_hash, 
			  total_coins, closing_time, prev_closing_time, close_time_res, close_flags
			  FROM ledgers ORDER BY ledger_seq DESC LIMIT 1`

	return tx.scanLedgerInfo(ctx, query)
}

// scanLedgerInfo is a helper method to scan ledger information within transactions
func (tx *PostgresTransaction) scanLedgerInfo(ctx context.Context, query string, args ...interface{}) (*relationaldb.LedgerInfo, error) {
	var info relationaldb.LedgerInfo
	var hashBytes, parentHashBytes, accountHashBytes, txHashBytes []byte
	var totalCoinsStr string
	var closingTime, prevClosingTime int64

	err := tx.tx.QueryRowContext(ctx, query, args...).Scan(
		&hashBytes, &info.Sequence, &parentHashBytes, &accountHashBytes, &txHashBytes,
		&totalCoinsStr, &closingTime, &prevClosingTime, &info.CloseTimeRes, &info.CloseFlags)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, relationaldb.NewQueryError("scan_ledger_info_tx", "failed to query ledger in transaction", err)
	}

	copy(info.Hash[:], hashBytes)
	copy(info.ParentHash[:], parentHashBytes)
	copy(info.AccountHash[:], accountHashBytes)
	copy(info.TransactionHash[:], txHashBytes)

	if totalCoins, err := strconv.ParseInt(totalCoinsStr, 10, 64); err == nil {
		info.TotalCoins = relationaldb.Amount(totalCoins)
	}

	info.CloseTime = time.Unix(closingTime+946684800, 0).UTC()
	info.ParentCloseTime = time.Unix(prevClosingTime+946684800, 0).UTC()

	return &info, nil
}

// Stub implementations for rarely used transaction methods
// These methods are required by the interface but not commonly used in practice

func (tx *PostgresTransaction) GetHashByIndex(ctx context.Context, seq relationaldb.LedgerIndex) (*relationaldb.Hash, error) {
	return nil, relationaldb.NewQueryError("get_hash_by_index", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetHashesByIndex(ctx context.Context, seq relationaldb.LedgerIndex) (*relationaldb.LedgerHashPair, error) {
	return nil, relationaldb.NewQueryError("get_hashes_by_index", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetHashesByRange(ctx context.Context, minSeq, maxSeq relationaldb.LedgerIndex) (map[relationaldb.LedgerIndex]relationaldb.LedgerHashPair, error) {
	return nil, relationaldb.NewQueryError("get_hashes_by_range", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) DeleteLedgersBySeq(ctx context.Context, maxSeq relationaldb.LedgerIndex) error {
	return relationaldb.NewQueryError("delete_ledgers_by_seq", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetTransactionsMinLedgerSeq(ctx context.Context) (*relationaldb.LedgerIndex, error) {
	return nil, relationaldb.NewQueryError("get_transactions_min_ledger_seq", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetTransactionCount(ctx context.Context) (int64, error) {
	return 0, relationaldb.NewQueryError("get_transaction_count", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetTransaction(ctx context.Context, hash relationaldb.Hash, ledgerRange *relationaldb.LedgerRange) (*relationaldb.TransactionInfo, relationaldb.TxSearchResult, error) {
	return nil, relationaldb.TxSearchUnknown, relationaldb.NewQueryError("get_transaction", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetTxHistory(ctx context.Context, startIndex relationaldb.LedgerIndex, limit int) ([]relationaldb.TransactionInfo, error) {
	return nil, relationaldb.NewQueryError("get_tx_history", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) DeleteTransactionsByLedgerSeq(ctx context.Context, ledgerSeq relationaldb.LedgerIndex) error {
	return relationaldb.NewQueryError("delete_transactions_by_ledger_seq", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) DeleteTransactionsBeforeLedgerSeq(ctx context.Context, ledgerSeq relationaldb.LedgerIndex) error {
	return relationaldb.NewQueryError("delete_transactions_before_ledger_seq", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetAccountTransactionsMinLedgerSeq(ctx context.Context) (*relationaldb.LedgerIndex, error) {
	return nil, relationaldb.NewQueryError("get_account_transactions_min_ledger_seq", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetAccountTransactionCount(ctx context.Context) (int64, error) {
	return 0, relationaldb.NewQueryError("get_account_transaction_count", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetOldestAccountTxs(ctx context.Context, options relationaldb.AccountTxOptions) ([]relationaldb.TransactionInfo, error) {
	return nil, relationaldb.NewQueryError("get_oldest_account_txs", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetNewestAccountTxs(ctx context.Context, options relationaldb.AccountTxOptions) ([]relationaldb.TransactionInfo, error) {
	return nil, relationaldb.NewQueryError("get_newest_account_txs", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetOldestAccountTxsPage(ctx context.Context, options relationaldb.AccountTxPageOptions) (*relationaldb.AccountTxResult, error) {
	return nil, relationaldb.NewQueryError("get_oldest_account_txs_page", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetNewestAccountTxsPage(ctx context.Context, options relationaldb.AccountTxPageOptions) (*relationaldb.AccountTxResult, error) {
	return nil, relationaldb.NewQueryError("get_newest_account_txs_page", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) DeleteAccountTransactionsBeforeLedgerSeq(ctx context.Context, ledgerSeq relationaldb.LedgerIndex) error {
	return relationaldb.NewQueryError("delete_account_transactions_before_ledger_seq", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetLedgerCountMinMax(ctx context.Context) (*relationaldb.CountMinMax, error) {
	return nil, relationaldb.NewQueryError("get_ledger_count_min_max", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetKBUsedAll(ctx context.Context) (uint32, error) {
	return 0, relationaldb.NewQueryError("get_kb_used_all", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetKBUsedLedger(ctx context.Context) (uint32, error) {
	return 0, relationaldb.NewQueryError("get_kb_used_ledger", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) GetKBUsedTransaction(ctx context.Context) (uint32, error) {
	return 0, relationaldb.NewQueryError("get_kb_used_transaction", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) HasLedgerSpace(ctx context.Context) (bool, error) {
	return false, relationaldb.NewQueryError("has_ledger_space", "not implemented in transaction", nil)
}

func (tx *PostgresTransaction) HasTransactionSpace(ctx context.Context) (bool, error) {
	return true, nil // PostgreSQL manages its own space
}