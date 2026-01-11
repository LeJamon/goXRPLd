package rpc

import (
	"encoding/hex"
	"strconv"

	"github.com/LeJamon/goXRPLd/internal/core/ledger"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/service"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/storage/relationaldb"
)

// LedgerServiceAdapter adapts the ledger service to the RPC LedgerService interface
type LedgerServiceAdapter struct {
	svc *service.Service
}

// NewLedgerServiceAdapter creates a new adapter
func NewLedgerServiceAdapter(svc *service.Service) *LedgerServiceAdapter {
	return &LedgerServiceAdapter{svc: svc}
}

// GetCurrentLedgerIndex returns the current open ledger index
func (a *LedgerServiceAdapter) GetCurrentLedgerIndex() uint32 {
	return a.svc.GetCurrentLedgerIndex()
}

// GetClosedLedgerIndex returns the last closed ledger index
func (a *LedgerServiceAdapter) GetClosedLedgerIndex() uint32 {
	return a.svc.GetClosedLedgerIndex()
}

// GetValidatedLedgerIndex returns the highest validated ledger index
func (a *LedgerServiceAdapter) GetValidatedLedgerIndex() uint32 {
	return a.svc.GetValidatedLedgerIndex()
}

// AcceptLedger closes the current open ledger (standalone mode only)
func (a *LedgerServiceAdapter) AcceptLedger() (uint32, error) {
	return a.svc.AcceptLedger()
}

// IsStandalone returns true if running in standalone mode
func (a *LedgerServiceAdapter) IsStandalone() bool {
	return a.svc.IsStandalone()
}

// GetServerInfo returns server status information
func (a *LedgerServiceAdapter) GetServerInfo() LedgerServerInfo {
	info := a.svc.GetServerInfo()
	return LedgerServerInfo{
		Standalone:          info.Standalone,
		OpenLedgerSeq:       info.OpenLedgerSeq,
		ClosedLedgerSeq:     info.ClosedLedgerSeq,
		ClosedLedgerHash:    info.ClosedLedgerHash,
		ValidatedLedgerSeq:  info.ValidatedLedgerSeq,
		ValidatedLedgerHash: info.ValidatedLedgerHash,
		CompleteLedgers:     info.CompleteLedgers,
	}
}

// GetLedgerBySequence returns a ledger by its sequence number
func (a *LedgerServiceAdapter) GetLedgerBySequence(seq uint32) (LedgerReader, error) {
	l, err := a.svc.GetLedgerBySequence(seq)
	if err != nil {
		return nil, err
	}
	return &ledgerReaderAdapter{l: l}, nil
}

// GetLedgerByHash returns a ledger by its hash
func (a *LedgerServiceAdapter) GetLedgerByHash(hash [32]byte) (LedgerReader, error) {
	l, err := a.svc.GetLedgerByHash(hash)
	if err != nil {
		return nil, err
	}
	return &ledgerReaderAdapter{l: l}, nil
}

// GetGenesisAccount returns the genesis account address
func (a *LedgerServiceAdapter) GetGenesisAccount() (string, error) {
	return a.svc.GetGenesisAccount()
}

// ledgerReaderAdapter adapts ledger.Ledger to LedgerReader interface
type ledgerReaderAdapter struct {
	l *ledger.Ledger
}

func (a *ledgerReaderAdapter) Sequence() uint32 {
	return a.l.Sequence()
}

func (a *ledgerReaderAdapter) Hash() [32]byte {
	return a.l.Hash()
}

func (a *ledgerReaderAdapter) ParentHash() [32]byte {
	return a.l.ParentHash()
}

func (a *ledgerReaderAdapter) IsClosed() bool {
	return a.l.IsClosed()
}

func (a *ledgerReaderAdapter) IsValidated() bool {
	return a.l.IsValidated()
}

func (a *ledgerReaderAdapter) TotalDrops() uint64 {
	return a.l.TotalDrops()
}

// SubmitTransaction submits a transaction to the open ledger
func (a *LedgerServiceAdapter) SubmitTransaction(txJSON []byte) (*SubmitResult, error) {
	// Parse the transaction from JSON
	transaction, err := tx.ParseJSON(txJSON)
	if err != nil {
		return &SubmitResult{
			EngineResult:        "temMALFORMED",
			EngineResultCode:    -299,
			EngineResultMessage: "Transaction is malformed: " + err.Error(),
			Applied:             false,
		}, nil
	}

	// Submit to the service
	result, err := a.svc.SubmitTransaction(transaction)
	if err != nil {
		return &SubmitResult{
			EngineResult:        "tefINTERNAL",
			EngineResultCode:    -199,
			EngineResultMessage: "Internal error: " + err.Error(),
			Applied:             false,
		}, nil
	}

	return &SubmitResult{
		EngineResult:        result.Result.String(),
		EngineResultCode:    int(result.Result),
		EngineResultMessage: result.Message,
		Applied:             result.Applied,
		Fee:                 result.Fee,
		CurrentLedger:       result.CurrentLedger,
		ValidatedLedger:     result.ValidatedLedger,
	}, nil
}

// GetCurrentFees returns the current fee settings
func (a *LedgerServiceAdapter) GetCurrentFees() (baseFee, reserveBase, reserveIncrement uint64) {
	return a.svc.GetCurrentFees()
}

// GetAccountInfo retrieves account information from the ledger
func (a *LedgerServiceAdapter) GetAccountInfo(account string, ledgerIndex string) (*AccountInfo, error) {
	result, err := a.svc.GetAccountInfo(account, ledgerIndex)
	if err != nil {
		return nil, err
	}

	return &AccountInfo{
		Account:      result.Account,
		Balance:      strconv.FormatUint(result.Balance, 10),
		Flags:        result.Flags,
		OwnerCount:   result.OwnerCount,
		Sequence:     result.Sequence,
		RegularKey:   result.RegularKey,
		Domain:       result.Domain,
		EmailHash:    result.EmailHash,
		TransferRate: result.TransferRate,
		TickSize:     result.TickSize,
		LedgerIndex:  result.LedgerIndex,
		LedgerHash:   hex.EncodeToString(result.LedgerHash[:]),
		Validated:    result.Validated,
	}, nil
}

// GetTransaction retrieves a transaction by its hash
func (a *LedgerServiceAdapter) GetTransaction(txHash [32]byte) (*TransactionInfo, error) {
	result, err := a.svc.GetTransaction(txHash)
	if err != nil {
		return nil, err
	}

	return &TransactionInfo{
		TxData:      result.TxData,
		LedgerIndex: result.LedgerIndex,
		LedgerHash:  hex.EncodeToString(result.LedgerHash[:]),
		Validated:   result.Validated,
		TxIndex:     result.TxIndex,
	}, nil
}

// StoreTransaction stores a transaction in the current ledger
func (a *LedgerServiceAdapter) StoreTransaction(txHash [32]byte, txData []byte) error {
	return a.svc.StoreTransaction(txHash, txData)
}

// GetAccountLines retrieves trust lines for an account
func (a *LedgerServiceAdapter) GetAccountLines(account string, ledgerIndex string, peer string, limit uint32) (*AccountLinesResult, error) {
	result, err := a.svc.GetAccountLines(account, ledgerIndex, peer, limit)
	if err != nil {
		return nil, err
	}

	// Convert service types to RPC types
	lines := make([]TrustLine, len(result.Lines))
	for i, line := range result.Lines {
		lines[i] = TrustLine{
			Account:        line.Account,
			Balance:        line.Balance,
			Currency:       line.Currency,
			Limit:          line.Limit,
			LimitPeer:      line.LimitPeer,
			QualityIn:      line.QualityIn,
			QualityOut:     line.QualityOut,
			NoRipple:       line.NoRipple,
			NoRipplePeer:   line.NoRipplePeer,
			Authorized:     line.Authorized,
			PeerAuthorized: line.PeerAuthorized,
			Freeze:         line.Freeze,
			FreezePeer:     line.FreezePeer,
		}
	}

	return &AccountLinesResult{
		Account:     result.Account,
		Lines:       lines,
		LedgerIndex: result.LedgerIndex,
		LedgerHash:  result.LedgerHash,
		Validated:   result.Validated,
		Marker:      result.Marker,
	}, nil
}

// GetAccountOffers retrieves offers for an account
func (a *LedgerServiceAdapter) GetAccountOffers(account string, ledgerIndex string, limit uint32) (*AccountOffersResult, error) {
	result, err := a.svc.GetAccountOffers(account, ledgerIndex, limit)
	if err != nil {
		return nil, err
	}

	// Convert service types to RPC types
	offers := make([]AccountOffer, len(result.Offers))
	for i, offer := range result.Offers {
		offers[i] = AccountOffer{
			Flags:      offer.Flags,
			Seq:        offer.Seq,
			TakerGets:  offer.TakerGets,
			TakerPays:  offer.TakerPays,
			Quality:    offer.Quality,
			Expiration: offer.Expiration,
		}
	}

	return &AccountOffersResult{
		Account:     result.Account,
		Offers:      offers,
		LedgerIndex: result.LedgerIndex,
		LedgerHash:  result.LedgerHash,
		Validated:   result.Validated,
		Marker:      result.Marker,
	}, nil
}

// GetBookOffers retrieves offers from an order book
func (a *LedgerServiceAdapter) GetBookOffers(takerGets, takerPays Amount, ledgerIndex string, limit uint32) (*BookOffersResult, error) {
	// Convert RPC Amount to tx.Amount
	txTakerGets := tx.Amount{
		Value:    takerGets.Value,
		Currency: takerGets.Currency,
		Issuer:   takerGets.Issuer,
	}
	txTakerPays := tx.Amount{
		Value:    takerPays.Value,
		Currency: takerPays.Currency,
		Issuer:   takerPays.Issuer,
	}

	result, err := a.svc.GetBookOffers(txTakerGets, txTakerPays, ledgerIndex, limit)
	if err != nil {
		return nil, err
	}

	// Convert service types to RPC types
	offers := make([]BookOffer, len(result.Offers))
	for i, offer := range result.Offers {
		offers[i] = BookOffer{
			Account:         offer.Account,
			BookDirectory:   offer.BookDirectory,
			BookNode:        offer.BookNode,
			Flags:           offer.Flags,
			LedgerEntryType: offer.LedgerEntryType,
			OwnerNode:       offer.OwnerNode,
			Sequence:        offer.Sequence,
			TakerGets:       offer.TakerGets,
			TakerPays:       offer.TakerPays,
			Index:           offer.Index,
			Quality:         offer.Quality,
			OwnerFunds:      offer.OwnerFunds,
			TakerGetsFunded: offer.TakerGetsFunded,
			TakerPaysFunded: offer.TakerPaysFunded,
		}
	}

	return &BookOffersResult{
		LedgerIndex: result.LedgerIndex,
		LedgerHash:  result.LedgerHash,
		Offers:      offers,
		Validated:   result.Validated,
	}, nil
}

// GetAccountTransactions retrieves transaction history for an account
func (a *LedgerServiceAdapter) GetAccountTransactions(account string, ledgerMin, ledgerMax int64, limit uint32, marker *AccountTxMarker, forward bool) (*AccountTxResult, error) {
	// Convert RPC marker to service marker
	var svcMarker *relationaldb.AccountTxMarker
	if marker != nil {
		svcMarker = &relationaldb.AccountTxMarker{
			LedgerSeq: relationaldb.LedgerIndex(marker.LedgerSeq),
			TxnSeq:    marker.TxnSeq,
		}
	}

	result, err := a.svc.GetAccountTransactions(account, ledgerMin, ledgerMax, limit, svcMarker, forward)
	if err != nil {
		return nil, err
	}

	// Convert service result to RPC result
	txs := make([]AccountTransaction, len(result.Transactions))
	for i, tx := range result.Transactions {
		txs[i] = AccountTransaction{
			Hash:        tx.Hash,
			LedgerIndex: tx.LedgerIndex,
			TxBlob:      tx.TxBlob,
			Meta:        tx.Meta,
		}
	}

	var rpcMarker *AccountTxMarker
	if result.Marker != nil {
		rpcMarker = &AccountTxMarker{
			LedgerSeq: uint32(result.Marker.LedgerSeq),
			TxnSeq:    result.Marker.TxnSeq,
		}
	}

	return &AccountTxResult{
		Account:      result.Account,
		LedgerMin:    result.LedgerMin,
		LedgerMax:    result.LedgerMax,
		Limit:        result.Limit,
		Marker:       rpcMarker,
		Transactions: txs,
		Validated:    result.Validated,
	}, nil
}

// GetTransactionHistory retrieves recent transactions
func (a *LedgerServiceAdapter) GetTransactionHistory(startIndex uint32) (*TxHistoryResult, error) {
	result, err := a.svc.GetTransactionHistory(startIndex)
	if err != nil {
		return nil, err
	}

	// Convert service result to RPC result
	txs := make([]AccountTransaction, len(result.Transactions))
	for i, tx := range result.Transactions {
		txs[i] = AccountTransaction{
			Hash:        tx.Hash,
			LedgerIndex: tx.LedgerIndex,
			TxBlob:      tx.TxBlob,
			Meta:        tx.Meta,
		}
	}

	return &TxHistoryResult{
		Index:        result.Index,
		Transactions: txs,
	}, nil
}

// GetLedgerRange retrieves ledger hashes for a range of sequences
func (a *LedgerServiceAdapter) GetLedgerRange(minSeq, maxSeq uint32) (*LedgerRangeResult, error) {
	result, err := a.svc.GetLedgerRange(minSeq, maxSeq)
	if err != nil {
		return nil, err
	}

	return &LedgerRangeResult{
		LedgerFirst: result.LedgerFirst,
		LedgerLast:  result.LedgerLast,
		Hashes:      result.Hashes,
	}, nil
}

// GetLedgerEntry retrieves a specific ledger entry by its index/key
func (a *LedgerServiceAdapter) GetLedgerEntry(entryKey [32]byte, ledgerIndex string) (*LedgerEntryResult, error) {
	result, err := a.svc.GetLedgerEntry(entryKey, ledgerIndex)
	if err != nil {
		return nil, err
	}

	return &LedgerEntryResult{
		Index:       result.Index,
		LedgerIndex: result.LedgerIndex,
		LedgerHash:  result.LedgerHash,
		Node:        result.Node,
		NodeBinary:  hex.EncodeToString(result.Node),
		Validated:   result.Validated,
	}, nil
}

// GetLedgerData retrieves all ledger state entries with pagination
func (a *LedgerServiceAdapter) GetLedgerData(ledgerIndex string, limit uint32, marker string) (*LedgerDataResult, error) {
	result, err := a.svc.GetLedgerData(ledgerIndex, limit, marker)
	if err != nil {
		return nil, err
	}

	// Convert service result to RPC result
	state := make([]LedgerDataItem, len(result.State))
	for i, item := range result.State {
		state[i] = LedgerDataItem{
			Index: item.Index,
			Data:  item.Data,
		}
	}

	rpcResult := &LedgerDataResult{
		LedgerIndex: result.LedgerIndex,
		LedgerHash:  result.LedgerHash,
		State:       state,
		Marker:      result.Marker,
		Validated:   result.Validated,
	}

	// Convert ledger header info if present
	if result.LedgerHeader != nil {
		rpcResult.LedgerHeader = &LedgerHeaderInfo{
			AccountHash:         result.LedgerHeader.AccountHash,
			CloseFlags:          result.LedgerHeader.CloseFlags,
			CloseTime:           result.LedgerHeader.CloseTime,
			CloseTimeHuman:      result.LedgerHeader.CloseTimeHuman,
			CloseTimeISO:        result.LedgerHeader.CloseTimeISO,
			CloseTimeResolution: result.LedgerHeader.CloseTimeResolution,
			Closed:              result.LedgerHeader.Closed,
			LedgerHash:          result.LedgerHeader.LedgerHash,
			LedgerIndex:         result.LedgerHeader.LedgerIndex,
			ParentCloseTime:     result.LedgerHeader.ParentCloseTime,
			ParentHash:          result.LedgerHeader.ParentHash,
			TotalCoins:          result.LedgerHeader.TotalCoins,
			TransactionHash:     result.LedgerHeader.TransactionHash,
		}
	}

	return rpcResult, nil
}

// GetAccountObjects retrieves all objects owned by an account
func (a *LedgerServiceAdapter) GetAccountObjects(account string, ledgerIndex string, objType string, limit uint32) (*AccountObjectsResult, error) {
	result, err := a.svc.GetAccountObjects(account, ledgerIndex, objType, limit)
	if err != nil {
		return nil, err
	}

	// Convert service result to RPC result
	objects := make([]AccountObjectItem, len(result.AccountObjects))
	for i, obj := range result.AccountObjects {
		objects[i] = AccountObjectItem{
			Index:           obj.Index,
			LedgerEntryType: obj.LedgerEntryType,
			Data:            obj.Data,
		}
	}

	return &AccountObjectsResult{
		Account:        result.Account,
		AccountObjects: objects,
		LedgerIndex:    result.LedgerIndex,
		LedgerHash:     result.LedgerHash,
		Validated:      result.Validated,
		Marker:         result.Marker,
	}, nil
}
