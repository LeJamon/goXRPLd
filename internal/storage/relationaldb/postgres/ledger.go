package postgres

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/LeJamon/goXRPLd/internal/storage/relationaldb"
)

// GetMinLedgerSeq returns the minimum ledger sequence number
func (db *PostgresDatabase) GetMinLedgerSeq(ctx context.Context) (*relationaldb.LedgerIndex, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	var seq sql.NullInt64
	err := db.db.QueryRowContext(ctx, "SELECT MIN(ledger_seq) FROM ledgers").Scan(&seq)
	if err != nil {
		return nil, relationaldb.NewQueryError("get_min_ledger_seq", "failed to query min ledger sequence", err)
	}

	if !seq.Valid {
		return nil, nil
	}

	result := relationaldb.LedgerIndex(seq.Int64)
	return &result, nil
}

// GetMaxLedgerSeq returns the maximum ledger sequence number
func (db *PostgresDatabase) GetMaxLedgerSeq(ctx context.Context) (*relationaldb.LedgerIndex, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	var seq sql.NullInt64
	err := db.db.QueryRowContext(ctx, "SELECT MAX(ledger_seq) FROM ledgers").Scan(&seq)
	if err != nil {
		return nil, relationaldb.NewQueryError("get_max_ledger_seq", "failed to query max ledger sequence", err)
	}

	if !seq.Valid {
		return nil, nil
	}

	result := relationaldb.LedgerIndex(seq.Int64)
	return &result, nil
}

// GetLedgerInfoBySeq retrieves ledger information by sequence number
func (db *PostgresDatabase) GetLedgerInfoBySeq(ctx context.Context, seq relationaldb.LedgerIndex) (*relationaldb.LedgerInfo, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	query := `SELECT ledger_hash, ledger_seq, prev_hash, account_set_hash, trans_set_hash, 
			  total_coins, closing_time, prev_closing_time, close_time_res, close_flags
			  FROM ledgers WHERE ledger_seq = $1`

	return db.scanLedgerInfo(ctx, query, seq)
}

// GetLedgerInfoByHash retrieves ledger information by hash
func (db *PostgresDatabase) GetLedgerInfoByHash(ctx context.Context, hash relationaldb.Hash) (*relationaldb.LedgerInfo, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	query := `SELECT ledger_hash, ledger_seq, prev_hash, account_set_hash, trans_set_hash, 
			  total_coins, closing_time, prev_closing_time, close_time_res, close_flags
			  FROM ledgers WHERE ledger_hash = $1`

	return db.scanLedgerInfo(ctx, query, hash[:])
}

// GetNewestLedgerInfo retrieves the newest ledger information
func (db *PostgresDatabase) GetNewestLedgerInfo(ctx context.Context) (*relationaldb.LedgerInfo, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	query := `SELECT ledger_hash, ledger_seq, prev_hash, account_set_hash, trans_set_hash, 
			  total_coins, closing_time, prev_closing_time, close_time_res, close_flags
			  FROM ledgers ORDER BY ledger_seq DESC LIMIT 1`

	return db.scanLedgerInfo(ctx, query)
}

// GetLimitedOldestLedgerInfo retrieves the oldest ledger with minimum sequence constraint
func (db *PostgresDatabase) GetLimitedOldestLedgerInfo(ctx context.Context, minSeq relationaldb.LedgerIndex) (*relationaldb.LedgerInfo, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	query := `SELECT ledger_hash, ledger_seq, prev_hash, account_set_hash, trans_set_hash, 
			  total_coins, closing_time, prev_closing_time, close_time_res, close_flags
			  FROM ledgers WHERE ledger_seq >= $1 ORDER BY ledger_seq ASC LIMIT 1`

	return db.scanLedgerInfo(ctx, query, minSeq)
}

// GetLimitedNewestLedgerInfo retrieves the newest ledger with minimum sequence constraint
func (db *PostgresDatabase) GetLimitedNewestLedgerInfo(ctx context.Context, minSeq relationaldb.LedgerIndex) (*relationaldb.LedgerInfo, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	query := `SELECT ledger_hash, ledger_seq, prev_hash, account_set_hash, trans_set_hash, 
			  total_coins, closing_time, prev_closing_time, close_time_res, close_flags
			  FROM ledgers WHERE ledger_seq >= $1 ORDER BY ledger_seq DESC LIMIT 1`

	return db.scanLedgerInfo(ctx, query, minSeq)
}

// scanLedgerInfo is a helper method to scan ledger information from database rows
func (db *PostgresDatabase) scanLedgerInfo(ctx context.Context, query string, args ...interface{}) (*relationaldb.LedgerInfo, error) {
	var info relationaldb.LedgerInfo
	var hashBytes, parentHashBytes, accountHashBytes, txHashBytes []byte
	var totalCoinsStr string
	var closingTime, prevClosingTime int64

	err := db.db.QueryRowContext(ctx, query, args...).Scan(
		&hashBytes, &info.Sequence, &parentHashBytes, &accountHashBytes, &txHashBytes,
		&totalCoinsStr, &closingTime, &prevClosingTime, &info.CloseTimeRes, &info.CloseFlags)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, relationaldb.NewQueryError("scan_ledger_info", "failed to query ledger", err)
	}

	// Convert data to proper formats
	copy(info.Hash[:], hashBytes)
	copy(info.ParentHash[:], parentHashBytes)
	copy(info.AccountHash[:], accountHashBytes)
	copy(info.TransactionHash[:], txHashBytes)

	// Parse total coins as decimal string to int64
	if totalCoins, err := strconv.ParseInt(totalCoinsStr, 10, 64); err == nil {
		info.TotalCoins = relationaldb.Amount(totalCoins)
	}

	// Convert rippled time format (seconds since 2000-01-01) to Go time
	info.CloseTime = time.Unix(closingTime+946684800, 0).UTC() // Add Ripple epoch offset
	info.ParentCloseTime = time.Unix(prevClosingTime+946684800, 0).UTC()

	return &info, nil
}

// GetHashByIndex retrieves a ledger hash by sequence number
func (db *PostgresDatabase) GetHashByIndex(ctx context.Context, seq relationaldb.LedgerIndex) (*relationaldb.Hash, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	var hashBytes []byte
	err := db.db.QueryRowContext(ctx, "SELECT ledger_hash FROM ledgers WHERE ledger_seq = $1", seq).Scan(&hashBytes)

	if err == sql.ErrNoRows {
		return nil, relationaldb.NewDataError("get_hash_by_index", "ledger not found", relationaldb.ErrLedgerNotFound)
	}
	if err != nil {
		return nil, relationaldb.NewQueryError("get_hash_by_index", "failed to query ledger hash", err)
	}

	var hash relationaldb.Hash
	copy(hash[:], hashBytes)
	return &hash, nil
}

// GetHashesByIndex retrieves ledger and parent hashes by sequence number
func (db *PostgresDatabase) GetHashesByIndex(ctx context.Context, seq relationaldb.LedgerIndex) (*relationaldb.LedgerHashPair, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	var ledgerHashBytes, parentHashBytes []byte
	err := db.db.QueryRowContext(ctx,
		"SELECT ledger_hash, prev_hash FROM ledgers WHERE ledger_seq = $1", seq).Scan(&ledgerHashBytes, &parentHashBytes)

	if err == sql.ErrNoRows {
		return nil, relationaldb.NewDataError("get_hashes_by_index", "ledger not found", relationaldb.ErrLedgerNotFound)
	}
	if err != nil {
		return nil, relationaldb.NewQueryError("get_hashes_by_index", "failed to query ledger hashes", err)
	}

	var pair relationaldb.LedgerHashPair
	copy(pair.LedgerHash[:], ledgerHashBytes)
	copy(pair.ParentHash[:], parentHashBytes)
	return &pair, nil
}

// GetHashesByRange retrieves ledger hashes for a range of sequence numbers
func (db *PostgresDatabase) GetHashesByRange(ctx context.Context, minSeq, maxSeq relationaldb.LedgerIndex) (map[relationaldb.LedgerIndex]relationaldb.LedgerHashPair, error) {
	if db.db == nil {
		return nil, relationaldb.ErrDatabaseClosed
	}

	query := `SELECT ledger_seq, ledger_hash, prev_hash FROM ledgers 
			  WHERE ledger_seq >= $1 AND ledger_seq <= $2 ORDER BY ledger_seq`

	rows, err := db.db.QueryContext(ctx, query, minSeq, maxSeq)
	if err != nil {
		return nil, relationaldb.NewQueryError("get_hashes_by_range", "failed to query ledger hashes", err)
	}
	defer rows.Close()

	result := make(map[relationaldb.LedgerIndex]relationaldb.LedgerHashPair)

	for rows.Next() {
		var seq relationaldb.LedgerIndex
		var ledgerHashBytes, parentHashBytes []byte

		if err := rows.Scan(&seq, &ledgerHashBytes, &parentHashBytes); err != nil {
			return nil, relationaldb.NewQueryError("get_hashes_by_range", "failed to scan row", err)
		}

		var pair relationaldb.LedgerHashPair
		copy(pair.LedgerHash[:], ledgerHashBytes)
		copy(pair.ParentHash[:], parentHashBytes)
		result[seq] = pair
	}

	if err := rows.Err(); err != nil {
		return nil, relationaldb.NewQueryError("get_hashes_by_range", "error iterating rows", err)
	}

	return result, nil
}

// SaveValidatedLedger saves a validated ledger to the database
func (db *PostgresDatabase) SaveValidatedLedger(ctx context.Context, ledger *relationaldb.LedgerInfo, current bool) error {
	if db.db == nil {
		return relationaldb.ErrDatabaseClosed
	}

	// Convert Go time back to rippled format (seconds since 2000-01-01)
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

	_, err := db.db.ExecContext(ctx, query,
		ledger.Hash[:], ledger.Sequence, ledger.ParentHash[:], ledger.AccountHash[:], ledger.TransactionHash[:],
		strconv.FormatInt(int64(ledger.TotalCoins), 10), closingTime, prevClosingTime, ledger.CloseTimeRes, ledger.CloseFlags)

	if err != nil {
		return relationaldb.NewQueryError("save_validated_ledger", "failed to save ledger", err)
	}

	return nil
}

// DeleteLedgersBySeq deletes ledgers up to a specified sequence number
func (db *PostgresDatabase) DeleteLedgersBySeq(ctx context.Context, maxSeq relationaldb.LedgerIndex) error {
	if db.db == nil {
		return relationaldb.ErrDatabaseClosed
	}

	_, err := db.db.ExecContext(ctx, "DELETE FROM ledgers WHERE ledger_seq <= $1", maxSeq)
	if err != nil {
		return relationaldb.NewQueryError("delete_ledgers_by_seq", "failed to delete ledgers", err)
	}

	return nil
}