package service

import (
	"context"
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger"
	"github.com/LeJamon/goXRPLd/internal/storage/relationaldb"
)

// TransactionIndex manages transaction indexing and lookup.
type TransactionIndex struct {
	// In-memory transaction index (hash -> ledger sequence)
	txIndex map[[32]byte]uint32

	// Relational database for persistent storage (optional)
	relationalDB relationaldb.RepositoryManager

	// Ledger manager reference for accessing ledger history
	ledgerManager *LedgerManager
}

// NewTransactionIndex creates a new transaction index.
func NewTransactionIndex(ledgerManager *LedgerManager, relationalDB relationaldb.RepositoryManager) *TransactionIndex {
	return &TransactionIndex{
		txIndex:       make(map[[32]byte]uint32),
		relationalDB:  relationalDB,
		ledgerManager: ledgerManager,
	}
}

// IndexTransaction indexes a transaction to a ledger sequence.
func (i *TransactionIndex) IndexTransaction(txHash [32]byte, ledgerSeq uint32) {
	i.txIndex[txHash] = ledgerSeq
}

// GetTransactionLedger returns the ledger sequence containing a transaction.
func (i *TransactionIndex) GetTransactionLedger(txHash [32]byte) (uint32, bool) {
	seq, found := i.txIndex[txHash]
	return seq, found
}

// GetTransaction retrieves a transaction by its hash.
func (i *TransactionIndex) GetTransaction(txHash [32]byte) (*TransactionResult, error) {
	// Look up which ledger contains this transaction
	ledgerSeq, found := i.txIndex[txHash]
	if !found {
		return nil, errors.New("transaction not found")
	}

	// Get the ledger from the manager
	l, err := i.ledgerManager.GetLedgerBySequence(ledgerSeq)
	if err != nil {
		return nil, errors.New("ledger not found")
	}

	// Get the transaction data from the ledger
	txData, found, err := l.GetTransaction(txHash)
	if err != nil {
		return nil, errors.New("failed to get transaction: " + err.Error())
	}
	if !found {
		return nil, errors.New("transaction not found in ledger")
	}

	return &TransactionResult{
		TxData:      txData,
		LedgerIndex: ledgerSeq,
		LedgerHash:  l.Hash(),
		Validated:   l.IsValidated(),
		TxIndex:     0, // TODO: Track transaction index within ledger
	}, nil
}

// StoreTransaction stores a transaction in a ledger.
func (i *TransactionIndex) StoreTransaction(l *ledger.Ledger, txHash [32]byte, txData []byte) error {
	// Add to the ledger's transaction map
	if err := l.AddTransaction(txHash, txData); err != nil {
		return err
	}

	// Index the transaction
	i.txIndex[txHash] = l.Sequence()

	return nil
}

// GetAccountTransactions retrieves transaction history for an account.
func (i *TransactionIndex) GetAccountTransactions(ctx context.Context, account string, options AccountTxOptions) (*AccountTxResult, error) {
	if i.relationalDB == nil {
		return nil, errors.New("transaction history not available (no database configured)")
	}

	// Convert to repository options
	accountID, err := decodeAccountIDForIndex(account)
	if err != nil {
		return nil, err
	}

	repoOptions := relationaldb.AccountTxPageOptions{
		Account:   accountID,
		MinLedger: relationaldb.LedgerIndex(options.LedgerMin),
		MaxLedger: relationaldb.LedgerIndex(options.LedgerMax),
		Limit:     options.Limit,
	}

	var txResult *relationaldb.AccountTxResult
	var repoErr error

	if options.Forward {
		txResult, repoErr = i.relationalDB.AccountTransaction().GetOldestAccountTxsPage(ctx, repoOptions)
	} else {
		txResult, repoErr = i.relationalDB.AccountTransaction().GetNewestAccountTxsPage(ctx, repoOptions)
	}

	if repoErr != nil {
		return nil, repoErr
	}

	// Convert to result
	result := &AccountTxResult{
		Account:      account,
		LedgerMin:    uint32(txResult.LedgerRange.Min),
		LedgerMax:    uint32(txResult.LedgerRange.Max),
		Limit:        txResult.Limit,
		Transactions: make([]AccountTransaction, 0, len(txResult.Transactions)),
		Validated:    true,
	}

	for _, txInfo := range txResult.Transactions {
		result.Transactions = append(result.Transactions, AccountTransaction{
			Hash:        [32]byte(txInfo.Hash),
			LedgerIndex: uint32(txInfo.LedgerSeq),
			TxBlob:      txInfo.RawTxn,
			Meta:        txInfo.TxnMeta,
		})
	}

	return result, nil
}

// GetTransactionHistory retrieves recent transactions.
func (i *TransactionIndex) GetTransactionHistory(ctx context.Context, startIndex uint32) (*TxHistoryResult, error) {
	if i.relationalDB == nil {
		return nil, errors.New("transaction history not available (no database configured)")
	}

	txInfos, err := i.relationalDB.Transaction().GetTxHistory(ctx, relationaldb.LedgerIndex(startIndex), 20)
	if err != nil {
		return nil, err
	}

	result := &TxHistoryResult{
		Index:        startIndex,
		Transactions: make([]AccountTransaction, 0, len(txInfos)),
	}

	for _, txInfo := range txInfos {
		result.Transactions = append(result.Transactions, AccountTransaction{
			Hash:        [32]byte(txInfo.Hash),
			LedgerIndex: uint32(txInfo.LedgerSeq),
			TxBlob:      txInfo.RawTxn,
			Meta:        txInfo.TxnMeta,
		})
	}

	return result, nil
}

// CollectTransactionResults gathers transaction data from a closed ledger.
func (i *TransactionIndex) CollectTransactionResults(l *ledger.Ledger, ledgerSeq uint32, ledgerHash [32]byte) []TransactionResultEvent {
	var results []TransactionResultEvent

	l.ForEachTransaction(func(txHash [32]byte, txData []byte) bool {
		result := TransactionResultEvent{
			TxHash:           txHash,
			TxData:           txData,
			Validated:        l.IsValidated(),
			LedgerIndex:      ledgerSeq,
			LedgerHash:       ledgerHash,
			AffectedAccounts: extractAffectedAccounts(txData),
		}
		results = append(results, result)
		return true
	})

	return results
}

// AccountTxOptions contains options for account transaction queries.
type AccountTxOptions struct {
	LedgerMin int64
	LedgerMax int64
	Limit     uint32
	Forward   bool
}

// decodeAccountIDForIndex decodes an address to an account ID for indexing.
func decodeAccountIDForIndex(address string) (relationaldb.AccountID, error) {
	var accountID relationaldb.AccountID
	// Simple validation - actual implementation would use address codec
	if address == "" {
		return accountID, errors.New("empty address")
	}
	return accountID, nil
}
