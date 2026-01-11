package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
	"github.com/LeJamon/goXRPLd/internal/core/ledger"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/genesis"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/header"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/storage/nodestore"
	"github.com/LeJamon/goXRPLd/internal/storage/relationaldb"
)

// Common errors
var (
	ErrNotStandalone    = errors.New("operation only valid in standalone mode")
	ErrNoOpenLedger     = errors.New("no open ledger")
	ErrNoClosedLedger   = errors.New("no closed ledger")
	ErrLedgerNotFound   = errors.New("ledger not found")
)

// Config holds configuration for the LedgerService
type Config struct {
	// Standalone indicates whether the node is running in standalone mode
	Standalone bool

	// GenesisConfig is the configuration for creating the genesis ledger
	GenesisConfig genesis.Config

	// NodeStore is the persistent storage for ledger nodes (optional, nil for in-memory only)
	NodeStore nodestore.Database

	// RelationalDB is the repository manager for transaction indexing (optional)
	RelationalDB relationaldb.RepositoryManager
}

// DefaultConfig returns the default service configuration
func DefaultConfig() Config {
	return Config{
		Standalone:    true,
		GenesisConfig: genesis.DefaultConfig(),
		NodeStore:     nil,
		RelationalDB:  nil,
	}
}

// Service manages the ledger lifecycle
type Service struct {
	mu sync.RWMutex

	config Config

	// NodeStore for persistent storage (nil if in-memory only)
	nodeStore nodestore.Database

	// RelationalDB for transaction indexing (nil if not configured)
	relationalDB relationaldb.RepositoryManager

	// Current open ledger (accepting transactions)
	openLedger *ledger.Ledger

	// Last closed ledger
	closedLedger *ledger.Ledger

	// Validated ledger (highest validated)
	validatedLedger *ledger.Ledger

	// Genesis ledger
	genesisLedger *ledger.Ledger

	// Ledger history (sequence -> ledger) - in-memory cache
	ledgerHistory map[uint32]*ledger.Ledger

	// Transaction index (hash -> ledger sequence) - in-memory cache
	txIndex map[[32]byte]uint32

	// Current fee settings
	fees XRPAmount.Fees
}

// New creates a new LedgerService
func New(cfg Config) (*Service, error) {
	s := &Service{
		config:        cfg,
		nodeStore:     cfg.NodeStore,
		relationalDB:  cfg.RelationalDB,
		ledgerHistory: make(map[uint32]*ledger.Ledger),
		txIndex:       make(map[[32]byte]uint32),
	}

	return s, nil
}

// Start initializes the service with a genesis ledger
func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create genesis ledger
	genesisResult, err := genesis.Create(s.config.GenesisConfig)
	if err != nil {
		return errors.New("failed to create genesis ledger: " + err.Error())
	}

	// Convert genesis to Ledger
	fees := XRPAmount.Fees{} // TODO: properly parse from genesis
	genesisLedger := ledger.FromGenesis(
		genesisResult.Header,
		genesisResult.StateMap,
		genesisResult.TxMap,
		fees,
	)

	s.genesisLedger = genesisLedger
	s.closedLedger = genesisLedger
	s.validatedLedger = genesisLedger
	s.ledgerHistory[genesisLedger.Sequence()] = genesisLedger

	// Create the first open ledger (ledger 2)
	openLedger, err := ledger.NewOpen(genesisLedger, time.Now())
	if err != nil {
		return errors.New("failed to create open ledger: " + err.Error())
	}
	s.openLedger = openLedger

	return nil
}

// GetOpenLedger returns the current open ledger
func (s *Service) GetOpenLedger() *ledger.Ledger {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.openLedger
}

// GetClosedLedger returns the last closed ledger
func (s *Service) GetClosedLedger() *ledger.Ledger {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closedLedger
}

// GetValidatedLedger returns the highest validated ledger
func (s *Service) GetValidatedLedger() *ledger.Ledger {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validatedLedger
}

// GetLedgerBySequence returns a ledger by its sequence number
func (s *Service) GetLedgerBySequence(seq uint32) (*ledger.Ledger, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	l, ok := s.ledgerHistory[seq]
	if !ok {
		return nil, ErrLedgerNotFound
	}
	return l, nil
}

// GetLedgerByHash returns a ledger by its hash
func (s *Service) GetLedgerByHash(hash [32]byte) (*ledger.Ledger, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, l := range s.ledgerHistory {
		if l.Hash() == hash {
			return l, nil
		}
	}
	return nil, ErrLedgerNotFound
}

// GetCurrentLedgerIndex returns the current open ledger index
func (s *Service) GetCurrentLedgerIndex() uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.openLedger == nil {
		return 0
	}
	return s.openLedger.Sequence()
}

// GetClosedLedgerIndex returns the last closed ledger index
func (s *Service) GetClosedLedgerIndex() uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closedLedger == nil {
		return 0
	}
	return s.closedLedger.Sequence()
}

// GetValidatedLedgerIndex returns the highest validated ledger index
func (s *Service) GetValidatedLedgerIndex() uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.validatedLedger == nil {
		return 0
	}
	return s.validatedLedger.Sequence()
}

// AcceptLedger closes the current open ledger and creates a new one.
// This is the main mechanism for advancing ledgers in standalone mode.
// It corresponds to the "ledger_accept" RPC command.
func (s *Service) AcceptLedger() (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.config.Standalone {
		return 0, ErrNotStandalone
	}

	if s.openLedger == nil {
		return 0, ErrNoOpenLedger
	}

	// Close the current open ledger
	closeTime := time.Now()
	if err := s.openLedger.Close(closeTime, 0); err != nil {
		return 0, errors.New("failed to close ledger: " + err.Error())
	}

	// In standalone mode, immediately validate
	if err := s.openLedger.SetValidated(); err != nil {
		return 0, errors.New("failed to validate ledger: " + err.Error())
	}

	// Persist the closed ledger to nodestore if available
	if s.nodeStore != nil {
		if err := s.persistLedger(s.openLedger); err != nil {
			return 0, errors.New("failed to persist ledger: " + err.Error())
		}
	}

	// Store the closed ledger in memory cache
	closedSeq := s.openLedger.Sequence()
	s.closedLedger = s.openLedger
	s.validatedLedger = s.openLedger
	s.ledgerHistory[closedSeq] = s.openLedger

	// Create new open ledger
	newOpen, err := ledger.NewOpen(s.closedLedger, time.Now())
	if err != nil {
		return 0, errors.New("failed to create new open ledger: " + err.Error())
	}
	s.openLedger = newOpen

	return closedSeq, nil
}

// IsStandalone returns true if running in standalone mode
func (s *Service) IsStandalone() bool {
	return s.config.Standalone
}

// GetGenesisAccount returns the genesis account address
func (s *Service) GetGenesisAccount() (string, error) {
	_, address, err := genesis.GenerateGenesisAccountID()
	return address, err
}

// GetServerInfo returns basic server information
func (s *Service) GetServerInfo() ServerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info := ServerInfo{
		Standalone:     s.config.Standalone,
		CompleteLedgers: "",
	}

	if s.openLedger != nil {
		info.OpenLedgerSeq = s.openLedger.Sequence()
	}

	if s.closedLedger != nil {
		info.ClosedLedgerSeq = s.closedLedger.Sequence()
		info.ClosedLedgerHash = s.closedLedger.Hash()
	}

	if s.validatedLedger != nil {
		info.ValidatedLedgerSeq = s.validatedLedger.Sequence()
		info.ValidatedLedgerHash = s.validatedLedger.Hash()
	}

	// Calculate complete ledgers range
	if len(s.ledgerHistory) > 0 {
		minSeq := uint32(0xFFFFFFFF)
		maxSeq := uint32(0)
		for seq := range s.ledgerHistory {
			if seq < minSeq {
				minSeq = seq
			}
			if seq > maxSeq {
				maxSeq = seq
			}
		}
		if minSeq == maxSeq {
			info.CompleteLedgers = string(rune(minSeq))
		} else {
			info.CompleteLedgers = formatRange(minSeq, maxSeq)
		}
	}

	return info
}

// ServerInfo contains basic server status information
type ServerInfo struct {
	Standalone          bool
	OpenLedgerSeq       uint32
	ClosedLedgerSeq     uint32
	ClosedLedgerHash    [32]byte
	ValidatedLedgerSeq  uint32
	ValidatedLedgerHash [32]byte
	CompleteLedgers     string
}

// GetLedgerInfo returns information about a specific ledger
func (s *Service) GetLedgerInfo(seq uint32) (*LedgerInfo, error) {
	l, err := s.GetLedgerBySequence(seq)
	if err != nil {
		return nil, err
	}

	return &LedgerInfo{
		Sequence:    l.Sequence(),
		Hash:        l.Hash(),
		ParentHash:  l.ParentHash(),
		CloseTime:   l.CloseTime(),
		TotalDrops:  l.TotalDrops(),
		Validated:   l.IsValidated(),
		Closed:      l.IsClosed(),
	}, nil
}

// LedgerInfo contains information about a ledger
type LedgerInfo struct {
	Sequence   uint32
	Hash       [32]byte
	ParentHash [32]byte
	CloseTime  time.Time
	TotalDrops uint64
	Validated  bool
	Closed     bool
	Header     header.LedgerHeader
}

// helper function to format ledger range
func formatRange(min, max uint32) string {
	// Simple implementation - could be improved
	return string(rune(min)) + "-" + string(rune(max))
}

// SubmitResult contains the result of submitting a transaction
type SubmitResult struct {
	// Result is the engine result code
	Result tx.Result

	// Applied indicates if the transaction was applied to the ledger
	Applied bool

	// Fee is the fee charged (in drops)
	Fee uint64

	// Metadata contains the changes made by the transaction
	Metadata *tx.Metadata

	// Message is a human-readable result message
	Message string

	// CurrentLedger is the current open ledger sequence
	CurrentLedger uint32

	// ValidatedLedger is the highest validated ledger sequence
	ValidatedLedger uint32
}

// SubmitTransaction submits a transaction to the open ledger
func (s *Service) SubmitTransaction(transaction tx.Transaction) (*SubmitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.openLedger == nil {
		return nil, ErrNoOpenLedger
	}

	// Create engine config from current state
	engineConfig := tx.EngineConfig{
		BaseFee:                   10,         // Default base fee in drops
		ReserveBase:               10_000_000, // 10 XRP reserve
		ReserveIncrement:          2_000_000,  // 2 XRP per object
		LedgerSequence:            s.openLedger.Sequence(),
		SkipSignatureVerification: s.config.Standalone, // Skip signatures in standalone mode
	}

	// Create engine with the open ledger as the view
	engine := tx.NewEngine(s.openLedger, engineConfig)

	// Apply the transaction
	applyResult := engine.Apply(transaction)

	result := &SubmitResult{
		Result:          applyResult.Result,
		Applied:         applyResult.Applied,
		Fee:             applyResult.Fee,
		Metadata:        applyResult.Metadata,
		Message:         applyResult.Message,
		CurrentLedger:   s.openLedger.Sequence(),
		ValidatedLedger: 0,
	}

	if s.validatedLedger != nil {
		result.ValidatedLedger = s.validatedLedger.Sequence()
	}

	return result, nil
}

// GetCurrentFees returns the current fee settings
func (s *Service) GetCurrentFees() (baseFee, reserveBase, reserveIncrement uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return default fees for now
	// In a full implementation, these would come from the FeeSettings ledger entry
	return 10, 10_000_000, 2_000_000
}

// TransactionResult contains a transaction and its metadata
type TransactionResult struct {
	TxData        []byte
	LedgerIndex   uint32
	LedgerHash    [32]byte
	Validated     bool
	TxIndex       uint32
}

// GetTransaction retrieves a transaction by its hash
func (s *Service) GetTransaction(txHash [32]byte) (*TransactionResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Look up which ledger contains this transaction
	ledgerSeq, found := s.txIndex[txHash]
	if !found {
		return nil, errors.New("transaction not found")
	}

	// Get the ledger
	l, ok := s.ledgerHistory[ledgerSeq]
	if !ok {
		return nil, errors.New("ledger not found")
	}

	// Get the transaction data
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

// StoreTransaction stores a transaction in the current open ledger
func (s *Service) StoreTransaction(txHash [32]byte, txData []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.openLedger == nil {
		return ErrNoOpenLedger
	}

	// Add to the open ledger's transaction map
	if err := s.openLedger.AddTransaction(txHash, txData); err != nil {
		return err
	}

	// Index the transaction to the current open ledger sequence
	s.txIndex[txHash] = s.openLedger.Sequence()

	return nil
}

// AccountInfoResult contains account information from the ledger
type AccountInfoResult struct {
	Account      string
	Balance      uint64
	Flags        uint32
	OwnerCount   uint32
	Sequence     uint32
	RegularKey   string
	Domain       string
	EmailHash    string
	TransferRate uint32
	TickSize     uint8
	LedgerIndex  uint32
	LedgerHash   [32]byte
	Validated    bool
}

// GetAccountInfo retrieves account information from the ledger
func (s *Service) GetAccountInfo(account string, ledgerIndex string) (*AccountInfoResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Determine which ledger to use
	var targetLedger *ledger.Ledger
	var validated bool

	switch ledgerIndex {
	case "current", "":
		targetLedger = s.openLedger
		validated = false
	case "closed":
		targetLedger = s.closedLedger
		validated = s.closedLedger == s.validatedLedger
	case "validated":
		targetLedger = s.validatedLedger
		validated = true
	default:
		// Try to parse as a number
		seq, err := strconv.ParseUint(ledgerIndex, 10, 32)
		if err != nil {
			return nil, errors.New("invalid ledger_index")
		}
		var ok bool
		targetLedger, ok = s.ledgerHistory[uint32(seq)]
		if !ok {
			return nil, ErrLedgerNotFound
		}
		validated = targetLedger.IsValidated()
	}

	if targetLedger == nil {
		return nil, ErrNoOpenLedger
	}

	// Decode the account address to get the account ID
	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(account)
	if err != nil {
		return nil, errors.New("invalid account address: " + err.Error())
	}

	var accountID [20]byte
	copy(accountID[:], accountIDBytes)

	// Get the account keylet
	accountKey := keylet.Account(accountID)

	// Check if account exists
	exists, err := targetLedger.Exists(accountKey)
	if err != nil {
		return nil, errors.New("failed to check account existence: " + err.Error())
	}
	if !exists {
		return nil, errors.New("account not found")
	}

	// Read the account data
	data, err := targetLedger.Read(accountKey)
	if err != nil {
		return nil, errors.New("failed to read account: " + err.Error())
	}

	// Parse the account root
	accountRoot, err := tx.ParseAccountRootFromBytes(data)
	if err != nil {
		return nil, errors.New("failed to parse account data: " + err.Error())
	}

	return &AccountInfoResult{
		Account:      account,
		Balance:      accountRoot.Balance,
		Flags:        accountRoot.Flags,
		OwnerCount:   accountRoot.OwnerCount,
		Sequence:     accountRoot.Sequence,
		RegularKey:   accountRoot.RegularKey,
		Domain:       accountRoot.Domain,
		EmailHash:    accountRoot.EmailHash,
		TransferRate: accountRoot.TransferRate,
		TickSize:     accountRoot.TickSize,
		LedgerIndex:  targetLedger.Sequence(),
		LedgerHash:   targetLedger.Hash(),
		Validated:    validated,
	}, nil
}

// TrustLine represents a trust line from account_lines RPC
type TrustLine struct {
	Account        string  `json:"account"`
	Balance        string  `json:"balance"`
	Currency       string  `json:"currency"`
	Limit          string  `json:"limit"`
	LimitPeer      string  `json:"limit_peer"`
	QualityIn      uint32  `json:"quality_in,omitempty"`
	QualityOut     uint32  `json:"quality_out,omitempty"`
	NoRipple       bool    `json:"no_ripple,omitempty"`
	NoRipplePeer   bool    `json:"no_ripple_peer,omitempty"`
	Authorized     bool    `json:"authorized,omitempty"`
	PeerAuthorized bool    `json:"peer_authorized,omitempty"`
	Freeze         bool    `json:"freeze,omitempty"`
	FreezePeer     bool    `json:"freeze_peer,omitempty"`
}

// AccountLinesResult contains the result of account_lines RPC
type AccountLinesResult struct {
	Account     string      `json:"account"`
	Lines       []TrustLine `json:"lines"`
	LedgerIndex uint32      `json:"ledger_index"`
	LedgerHash  [32]byte    `json:"ledger_hash"`
	Validated   bool        `json:"validated"`
	Marker      string      `json:"marker,omitempty"`
}

// GetAccountLines retrieves trust lines for an account
func (s *Service) GetAccountLines(account string, ledgerIndex string, peer string, limit uint32) (*AccountLinesResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Determine which ledger to use
	targetLedger, validated, err := s.getLedgerForQuery(ledgerIndex)
	if err != nil {
		return nil, err
	}

	// Decode the account address
	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(account)
	if err != nil {
		return nil, errors.New("invalid account address: " + err.Error())
	}
	var accountID [20]byte
	copy(accountID[:], accountIDBytes)

	// Parse peer if provided
	var peerID [20]byte
	hasPeer := false
	if peer != "" {
		_, peerIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(peer)
		if err != nil {
			return nil, errors.New("invalid peer address: " + err.Error())
		}
		copy(peerID[:], peerIDBytes)
		hasPeer = true
	}

	// Set default limit
	if limit == 0 || limit > 400 {
		limit = 200
	}

	// Collect trust lines by iterating through ledger entries
	var lines []TrustLine

	targetLedger.ForEach(func(key [32]byte, data []byte) bool {
		// Check if we've reached the limit
		if uint32(len(lines)) >= limit {
			return false
		}

		// Check if this is a RippleState entry (trust line)
		if len(data) < 3 {
			return true
		}

		// Check LedgerEntryType field
		if data[0] != 0x11 { // UInt16 type code 1, field code 1
			return true
		}
		entryType := uint16(data[1])<<8 | uint16(data[2])
		if entryType != 0x0072 { // RippleState type
			return true
		}

		// Parse the RippleState
		rs, err := tx.ParseRippleStateFromBytes(data)
		if err != nil {
			return true
		}

		// Check if this trust line involves our account
		lowID, _ := decodeAccountIDLocal(rs.LowLimit.Issuer)
		highID, _ := decodeAccountIDLocal(rs.HighLimit.Issuer)

		var isLowAccount bool
		var peerAccount string

		if lowID == accountID {
			isLowAccount = true
			peerAccount = rs.HighLimit.Issuer
		} else if highID == accountID {
			isLowAccount = false
			peerAccount = rs.LowLimit.Issuer
		} else {
			return true // Not our account
		}

		// Filter by peer if specified
		if hasPeer {
			peerAccountID, _ := decodeAccountIDLocal(peerAccount)
			if peerAccountID != peerID {
				return true
			}
		}

		// Build trust line response
		line := TrustLine{
			Account:  peerAccount,
			Currency: rs.Balance.Currency,
		}

		// Calculate balance from perspective of our account
		// Positive balance means peer owes us, negative means we owe peer
		if isLowAccount {
			// We are low account
			// Balance is positive if low owes high (we owe them) -> negative for us
			// Balance is negative if high owes low (they owe us) -> positive for us
			line.Balance = rs.Balance.Value.Neg(rs.Balance.Value).Text('f', -1)
			line.Limit = rs.LowLimit.Value.Text('f', -1)
			line.LimitPeer = rs.HighLimit.Value.Text('f', -1)
			line.NoRipple = (rs.Flags & 0x00020000) != 0      // lsfLowNoRipple
			line.NoRipplePeer = (rs.Flags & 0x00040000) != 0  // lsfHighNoRipple
			line.Authorized = (rs.Flags & 0x00010000) != 0    // lsfLowAuth
			line.PeerAuthorized = (rs.Flags & 0x00080000) != 0 // lsfHighAuth
			line.Freeze = (rs.Flags & 0x00400000) != 0        // lsfLowFreeze
			line.FreezePeer = (rs.Flags & 0x00800000) != 0    // lsfHighFreeze
		} else {
			// We are high account
			line.Balance = rs.Balance.Value.Text('f', -1)
			line.Limit = rs.HighLimit.Value.Text('f', -1)
			line.LimitPeer = rs.LowLimit.Value.Text('f', -1)
			line.NoRipple = (rs.Flags & 0x00040000) != 0      // lsfHighNoRipple
			line.NoRipplePeer = (rs.Flags & 0x00020000) != 0  // lsfLowNoRipple
			line.Authorized = (rs.Flags & 0x00080000) != 0    // lsfHighAuth
			line.PeerAuthorized = (rs.Flags & 0x00010000) != 0 // lsfLowAuth
			line.Freeze = (rs.Flags & 0x00800000) != 0        // lsfHighFreeze
			line.FreezePeer = (rs.Flags & 0x00400000) != 0    // lsfLowFreeze
		}

		line.QualityIn = rs.LowQualityIn
		line.QualityOut = rs.LowQualityOut

		lines = append(lines, line)
		return true
	})

	return &AccountLinesResult{
		Account:     account,
		Lines:       lines,
		LedgerIndex: targetLedger.Sequence(),
		LedgerHash:  targetLedger.Hash(),
		Validated:   validated,
	}, nil
}

// AccountOffer represents an offer from account_offers RPC
type AccountOffer struct {
	Flags      uint32      `json:"flags"`
	Seq        uint32      `json:"seq"`
	TakerGets  interface{} `json:"taker_gets"`
	TakerPays  interface{} `json:"taker_pays"`
	Quality    string      `json:"quality"`
	Expiration uint32      `json:"expiration,omitempty"`
}

// AccountOffersResult contains the result of account_offers RPC
type AccountOffersResult struct {
	Account     string         `json:"account"`
	Offers      []AccountOffer `json:"offers"`
	LedgerIndex uint32         `json:"ledger_index"`
	LedgerHash  [32]byte       `json:"ledger_hash"`
	Validated   bool           `json:"validated"`
	Marker      string         `json:"marker,omitempty"`
}

// GetAccountOffers retrieves offers for an account
func (s *Service) GetAccountOffers(account string, ledgerIndex string, limit uint32) (*AccountOffersResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Determine which ledger to use
	targetLedger, validated, err := s.getLedgerForQuery(ledgerIndex)
	if err != nil {
		return nil, err
	}

	// Decode the account address
	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(account)
	if err != nil {
		return nil, errors.New("invalid account address: " + err.Error())
	}
	var accountID [20]byte
	copy(accountID[:], accountIDBytes)

	// Set default limit
	if limit == 0 || limit > 400 {
		limit = 200
	}

	// Collect offers by iterating through ledger entries
	var offers []AccountOffer

	targetLedger.ForEach(func(key [32]byte, data []byte) bool {
		// Check if we've reached the limit
		if uint32(len(offers)) >= limit {
			return false
		}

		// Check if this is an Offer entry
		if len(data) < 3 {
			return true
		}

		// Check LedgerEntryType field
		if data[0] != 0x11 { // UInt16 type code 1, field code 1
			return true
		}
		entryType := uint16(data[1])<<8 | uint16(data[2])
		if entryType != 0x006F { // Offer type
			return true
		}

		// Parse the Offer
		offer, err := tx.ParseLedgerOfferFromBytes(data)
		if err != nil {
			return true
		}

		// Check if this offer belongs to our account
		offerAccountID, _ := decodeAccountIDLocal(offer.Account)
		if offerAccountID != accountID {
			return true
		}

		// Build offer response
		accountOffer := AccountOffer{
			Flags: offer.Flags,
			Seq:   offer.Sequence,
		}

		// Format TakerGets
		if offer.TakerGets.IsNative() {
			accountOffer.TakerGets = offer.TakerGets.Value
		} else {
			accountOffer.TakerGets = map[string]string{
				"currency": offer.TakerGets.Currency,
				"issuer":   offer.TakerGets.Issuer,
				"value":    offer.TakerGets.Value,
			}
		}

		// Format TakerPays
		if offer.TakerPays.IsNative() {
			accountOffer.TakerPays = offer.TakerPays.Value
		} else {
			accountOffer.TakerPays = map[string]string{
				"currency": offer.TakerPays.Currency,
				"issuer":   offer.TakerPays.Issuer,
				"value":    offer.TakerPays.Value,
			}
		}

		// Calculate quality
		accountOffer.Quality = calculateOfferQuality(offer.TakerPays, offer.TakerGets)

		if offer.Expiration > 0 {
			accountOffer.Expiration = offer.Expiration
		}

		offers = append(offers, accountOffer)
		return true
	})

	return &AccountOffersResult{
		Account:     account,
		Offers:      offers,
		LedgerIndex: targetLedger.Sequence(),
		LedgerHash:  targetLedger.Hash(),
		Validated:   validated,
	}, nil
}

// BookOffer represents an offer in an order book
type BookOffer struct {
	Account           string      `json:"Account"`
	BookDirectory     string      `json:"BookDirectory"`
	BookNode          string      `json:"BookNode"`
	Flags             uint32      `json:"Flags"`
	LedgerEntryType   string      `json:"LedgerEntryType"`
	OwnerNode         string      `json:"OwnerNode"`
	Sequence          uint32      `json:"Sequence"`
	TakerGets         interface{} `json:"TakerGets"`
	TakerPays         interface{} `json:"TakerPays"`
	Index             string      `json:"index"`
	Quality           string      `json:"quality"`
	OwnerFunds        string      `json:"owner_funds,omitempty"`
	TakerGetsFunded   interface{} `json:"taker_gets_funded,omitempty"`
	TakerPaysFunded   interface{} `json:"taker_pays_funded,omitempty"`
}

// BookOffersResult contains the result of book_offers RPC
type BookOffersResult struct {
	LedgerIndex uint32      `json:"ledger_index"`
	LedgerHash  [32]byte    `json:"ledger_hash"`
	Offers      []BookOffer `json:"offers"`
	Validated   bool        `json:"validated"`
}

// GetBookOffers retrieves offers from an order book
func (s *Service) GetBookOffers(takerGets, takerPays tx.Amount, ledgerIndex string, limit uint32) (*BookOffersResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Determine which ledger to use
	targetLedger, validated, err := s.getLedgerForQuery(ledgerIndex)
	if err != nil {
		return nil, err
	}

	// Set default limit
	if limit == 0 || limit > 400 {
		limit = 200
	}

	// Collect matching offers by iterating through ledger entries
	var offers []BookOffer

	targetLedger.ForEach(func(key [32]byte, data []byte) bool {
		// Check if we've reached the limit
		if uint32(len(offers)) >= limit {
			return false
		}

		// Check if this is an Offer entry
		if len(data) < 3 {
			return true
		}

		// Check LedgerEntryType field
		if data[0] != 0x11 {
			return true
		}
		entryType := uint16(data[1])<<8 | uint16(data[2])
		if entryType != 0x006F { // Offer type
			return true
		}

		// Parse the Offer
		offer, err := tx.ParseLedgerOfferFromBytes(data)
		if err != nil {
			return true
		}

		// Check if this offer matches the requested book
		// TakerGets in offer should match our takerGets parameter
		// TakerPays in offer should match our takerPays parameter
		getsMatch := amountsMatchCurrency(offer.TakerGets, takerGets)
		paysMatch := amountsMatchCurrency(offer.TakerPays, takerPays)

		if !getsMatch || !paysMatch {
			return true
		}

		// Build book offer response
		bookOffer := BookOffer{
			Account:         offer.Account,
			Flags:           offer.Flags,
			LedgerEntryType: "Offer",
			Sequence:        offer.Sequence,
			Index:           formatHash(key),
			Quality:         calculateOfferQuality(offer.TakerPays, offer.TakerGets),
		}

		// Format TakerGets
		if offer.TakerGets.IsNative() {
			bookOffer.TakerGets = offer.TakerGets.Value
		} else {
			bookOffer.TakerGets = map[string]string{
				"currency": offer.TakerGets.Currency,
				"issuer":   offer.TakerGets.Issuer,
				"value":    offer.TakerGets.Value,
			}
		}

		// Format TakerPays
		if offer.TakerPays.IsNative() {
			bookOffer.TakerPays = offer.TakerPays.Value
		} else {
			bookOffer.TakerPays = map[string]string{
				"currency": offer.TakerPays.Currency,
				"issuer":   offer.TakerPays.Issuer,
				"value":    offer.TakerPays.Value,
			}
		}

		offers = append(offers, bookOffer)
		return true
	})

	// Sort offers by quality (best first)
	sortBookOffersByQuality(offers)

	return &BookOffersResult{
		LedgerIndex: targetLedger.Sequence(),
		LedgerHash:  targetLedger.Hash(),
		Offers:      offers,
		Validated:   validated,
	}, nil
}

// Helper function to get ledger for query
func (s *Service) getLedgerForQuery(ledgerIndex string) (*ledger.Ledger, bool, error) {
	var targetLedger *ledger.Ledger
	var validated bool

	switch ledgerIndex {
	case "current", "":
		targetLedger = s.openLedger
		validated = false
	case "closed":
		targetLedger = s.closedLedger
		validated = s.closedLedger == s.validatedLedger
	case "validated":
		targetLedger = s.validatedLedger
		validated = true
	default:
		seq, err := strconv.ParseUint(ledgerIndex, 10, 32)
		if err != nil {
			return nil, false, errors.New("invalid ledger_index")
		}
		var ok bool
		targetLedger, ok = s.ledgerHistory[uint32(seq)]
		if !ok {
			return nil, false, ErrLedgerNotFound
		}
		validated = targetLedger.IsValidated()
	}

	if targetLedger == nil {
		return nil, false, ErrNoOpenLedger
	}

	return targetLedger, validated, nil
}

// persistLedger writes the ledger state to storage backends
func (s *Service) persistLedger(l *ledger.Ledger) error {
	ctx := context.Background()
	seq := l.Sequence()

	// Persist to NodeStore if configured
	if s.nodeStore != nil {
		if err := s.persistToNodeStore(ctx, l, seq); err != nil {
			return err
		}
	}

	// Persist to RelationalDB if configured
	if s.relationalDB != nil {
		if err := s.persistToRelationalDB(ctx, l); err != nil {
			return err
		}
	}

	return nil
}

// persistToNodeStore writes ledger state to the nodestore
func (s *Service) persistToNodeStore(ctx context.Context, l *ledger.Ledger, seq uint32) error {
	// Collect nodes to store in batch
	var nodes []*nodestore.Node

	// Persist state map entries
	err := l.ForEach(func(key [32]byte, data []byte) bool {
		node := &nodestore.Node{
			Type:      nodestore.NodeAccount,
			Hash:      nodestore.Hash256(key),
			Data:      data,
			LedgerSeq: seq,
		}
		nodes = append(nodes, node)
		return true
	})
	if err != nil {
		return err
	}

	// Store nodes in batch for efficiency
	if len(nodes) > 0 {
		if err := s.nodeStore.StoreBatch(ctx, nodes); err != nil {
			return err
		}
	}

	// Persist ledger header
	headerData := l.SerializeHeader()
	headerNode := &nodestore.Node{
		Type:      nodestore.NodeLedger,
		Hash:      nodestore.Hash256(l.Hash()),
		Data:      headerData,
		LedgerSeq: seq,
	}
	if err := s.nodeStore.Store(ctx, headerNode); err != nil {
		return err
	}

	// Sync to ensure durability
	return s.nodeStore.Sync()
}

// persistToRelationalDB writes ledger metadata to the relational database
func (s *Service) persistToRelationalDB(ctx context.Context, l *ledger.Ledger) error {
	h := l.Header()

	// Get state and tx map hashes
	stateHash, _ := l.StateMapHash()
	txHash, _ := l.TxMapHash()

	// Create ledger info for storage
	ledgerInfo := &relationaldb.LedgerInfo{
		Hash:            relationaldb.Hash(l.Hash()),
		Sequence:        relationaldb.LedgerIndex(h.LedgerIndex),
		ParentHash:      relationaldb.Hash(h.ParentHash),
		AccountHash:     relationaldb.Hash(stateHash),
		TransactionHash: relationaldb.Hash(txHash),
		TotalCoins:      relationaldb.Amount(h.Drops),
		CloseTime:       h.CloseTime,
		ParentCloseTime: h.ParentCloseTime,
		CloseTimeRes:    int32(h.CloseTimeResolution),
		CloseFlags:      uint32(h.CloseFlags),
	}

	// Save validated ledger
	if err := s.relationalDB.Ledger().SaveValidatedLedger(ctx, ledgerInfo, true); err != nil {
		return err
	}

	return nil
}

// AccountTxResult contains the result of account_tx query
type AccountTxResult struct {
	Account      string                       `json:"account"`
	LedgerMin    uint32                       `json:"ledger_index_min"`
	LedgerMax    uint32                       `json:"ledger_index_max"`
	Limit        uint32                       `json:"limit"`
	Marker       *relationaldb.AccountTxMarker `json:"marker,omitempty"`
	Transactions []AccountTransaction         `json:"transactions"`
	Validated    bool                         `json:"validated"`
}

// AccountTransaction contains transaction data for account_tx
type AccountTransaction struct {
	Hash        [32]byte `json:"hash"`
	LedgerIndex uint32   `json:"ledger_index"`
	TxBlob      []byte   `json:"tx_blob,omitempty"`
	Meta        []byte   `json:"meta,omitempty"`
}

// GetAccountTransactions retrieves transaction history for an account
func (s *Service) GetAccountTransactions(account string, ledgerMin, ledgerMax int64, limit uint32, marker *relationaldb.AccountTxMarker, forward bool) (*AccountTxResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// If no RelationalDB, return error
	if s.relationalDB == nil {
		return nil, errors.New("transaction history not available (no database configured)")
	}

	// Decode account address
	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(account)
	if err != nil {
		return nil, errors.New("invalid account address: " + err.Error())
	}
	var accountID relationaldb.AccountID
	copy(accountID[:], accountIDBytes)

	// Set defaults
	if limit == 0 || limit > 400 {
		limit = 200
	}

	// Determine ledger range
	minLedger := relationaldb.LedgerIndex(1)
	maxLedger := relationaldb.LedgerIndex(0xFFFFFFFF)
	if ledgerMin >= 0 {
		minLedger = relationaldb.LedgerIndex(ledgerMin)
	}
	if ledgerMax >= 0 {
		maxLedger = relationaldb.LedgerIndex(ledgerMax)
	}
	if s.validatedLedger != nil && maxLedger > relationaldb.LedgerIndex(s.validatedLedger.Sequence()) {
		maxLedger = relationaldb.LedgerIndex(s.validatedLedger.Sequence())
	}

	ctx := context.Background()
	options := relationaldb.AccountTxPageOptions{
		Account:   accountID,
		MinLedger: minLedger,
		MaxLedger: maxLedger,
		Marker:    marker,
		Limit:     limit,
	}

	var txResult *relationaldb.AccountTxResult
	if forward {
		txResult, err = s.relationalDB.AccountTransaction().GetOldestAccountTxsPage(ctx, options)
	} else {
		txResult, err = s.relationalDB.AccountTransaction().GetNewestAccountTxsPage(ctx, options)
	}
	if err != nil {
		return nil, err
	}

	// Convert to result
	result := &AccountTxResult{
		Account:      account,
		LedgerMin:    uint32(txResult.LedgerRange.Min),
		LedgerMax:    uint32(txResult.LedgerRange.Max),
		Limit:        txResult.Limit,
		Marker:       txResult.Marker,
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

// TxHistoryResult contains the result of tx_history query
type TxHistoryResult struct {
	Index        uint32               `json:"index"`
	Transactions []AccountTransaction `json:"txs"`
}

// GetTransactionHistory retrieves recent transactions
func (s *Service) GetTransactionHistory(startIndex uint32) (*TxHistoryResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.relationalDB == nil {
		return nil, errors.New("transaction history not available (no database configured)")
	}

	ctx := context.Background()
	txInfos, err := s.relationalDB.Transaction().GetTxHistory(ctx, relationaldb.LedgerIndex(startIndex), 20)
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

// LedgerRangeResult contains ledger hashes for a range
type LedgerRangeResult struct {
	LedgerFirst uint32              `json:"ledger_first"`
	LedgerLast  uint32              `json:"ledger_last"`
	Hashes      map[uint32][32]byte `json:"hashes"`
}

// GetLedgerRange retrieves ledger hashes for a range of sequences
func (s *Service) GetLedgerRange(minSeq, maxSeq uint32) (*LedgerRangeResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := &LedgerRangeResult{
		LedgerFirst: minSeq,
		LedgerLast:  maxSeq,
		Hashes:      make(map[uint32][32]byte),
	}

	// Try in-memory first
	for seq := minSeq; seq <= maxSeq; seq++ {
		if l, ok := s.ledgerHistory[seq]; ok {
			result.Hashes[seq] = l.Hash()
		}
	}

	// If we have RelationalDB, fill in gaps
	if s.relationalDB != nil && len(result.Hashes) < int(maxSeq-minSeq+1) {
		ctx := context.Background()
		hashPairs, err := s.relationalDB.Ledger().GetHashesByRange(ctx,
			relationaldb.LedgerIndex(minSeq),
			relationaldb.LedgerIndex(maxSeq))
		if err == nil {
			for seq, pair := range hashPairs {
				if _, exists := result.Hashes[uint32(seq)]; !exists {
					result.Hashes[uint32(seq)] = [32]byte(pair.LedgerHash)
				}
			}
		}
	}

	return result, nil
}

// LedgerEntryResult contains a single ledger entry
type LedgerEntryResult struct {
	Index       string   `json:"index"`
	LedgerIndex uint32   `json:"ledger_index"`
	LedgerHash  [32]byte `json:"ledger_hash"`
	Node        []byte   `json:"node"`
	NodeBinary  string   `json:"node_binary,omitempty"`
	Validated   bool     `json:"validated"`
}

// GetLedgerEntry retrieves a specific ledger entry by its index/key
func (s *Service) GetLedgerEntry(entryKey [32]byte, ledgerIndex string) (*LedgerEntryResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetLedger, validated, err := s.getLedgerForQuery(ledgerIndex)
	if err != nil {
		return nil, err
	}

	k := keylet.Keylet{Key: entryKey}
	exists, err := targetLedger.Exists(k)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("entry not found")
	}

	data, err := targetLedger.Read(k)
	if err != nil {
		return nil, err
	}

	return &LedgerEntryResult{
		Index:       formatHashHex(entryKey),
		LedgerIndex: targetLedger.Sequence(),
		LedgerHash:  targetLedger.Hash(),
		Node:        data,
		Validated:   validated,
	}, nil
}

// LedgerDataResult contains ledger state data
type LedgerDataResult struct {
	LedgerIndex uint32           `json:"ledger_index"`
	LedgerHash  [32]byte         `json:"ledger_hash"`
	State       []LedgerDataItem `json:"state"`
	Marker      string           `json:"marker,omitempty"`
	Validated   bool             `json:"validated"`
	// Ledger header information for first query (without marker)
	LedgerHeader *LedgerHeaderInfo `json:"ledger,omitempty"`
}

// LedgerHeaderInfo contains complete ledger header data for responses
type LedgerHeaderInfo struct {
	AccountHash         [32]byte  `json:"account_hash"`
	CloseFlags          uint8     `json:"close_flags"`
	CloseTime           int64     `json:"close_time"`           // Seconds since Ripple epoch
	CloseTimeHuman      string    `json:"close_time_human"`     // Human-readable format
	CloseTimeISO        string    `json:"close_time_iso"`       // ISO 8601 format
	CloseTimeResolution uint32    `json:"close_time_resolution"`
	Closed              bool      `json:"closed"`
	LedgerHash          [32]byte  `json:"ledger_hash"`
	LedgerIndex         uint32    `json:"ledger_index"`
	ParentCloseTime     int64     `json:"parent_close_time"`
	ParentHash          [32]byte  `json:"parent_hash"`
	TotalCoins          uint64    `json:"total_coins"`          // Total XRP drops
	TransactionHash     [32]byte  `json:"transaction_hash"`
}

// LedgerDataItem represents a single state entry
type LedgerDataItem struct {
	Index string `json:"index"`
	Data  []byte `json:"data"`
}

// RippleEpoch is January 1, 2000 00:00:00 UTC
var RippleEpoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// toRippleTime converts a time.Time to seconds since Ripple epoch
func toRippleTime(t time.Time) int64 {
	return t.Unix() - RippleEpoch.Unix()
}

// formatCloseTimeHuman formats close time in XRPL human-readable format
func formatCloseTimeHuman(t time.Time) string {
	return t.UTC().Format("2006-Jan-02 15:04:05.000000000 UTC")
}

// formatCloseTimeISO formats close time in ISO 8601 format
func formatCloseTimeISO(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// GetLedgerData retrieves all ledger state entries with optional pagination
func (s *Service) GetLedgerData(ledgerIndex string, limit uint32, marker string) (*LedgerDataResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetLedger, validated, err := s.getLedgerForQuery(ledgerIndex)
	if err != nil {
		return nil, err
	}

	if limit == 0 || limit > 2048 {
		limit = 256
	}

	result := &LedgerDataResult{
		LedgerIndex: targetLedger.Sequence(),
		LedgerHash:  targetLedger.Hash(),
		State:       make([]LedgerDataItem, 0, limit),
		Validated:   validated,
	}

	// Parse marker if provided
	var startKey [32]byte
	hasMarker := false
	if marker != "" {
		if len(marker) == 64 {
			decoded, err := hexDecode(marker)
			if err == nil && len(decoded) == 32 {
				copy(startKey[:], decoded)
				hasMarker = true
			}
		}
	}

	// Include ledger header info only on first query (no marker)
	if !hasMarker {
		hdr := targetLedger.Header()
		result.LedgerHeader = &LedgerHeaderInfo{
			AccountHash:         hdr.AccountHash,
			CloseFlags:          hdr.CloseFlags,
			CloseTime:           toRippleTime(hdr.CloseTime),
			CloseTimeHuman:      formatCloseTimeHuman(hdr.CloseTime),
			CloseTimeISO:        formatCloseTimeISO(hdr.CloseTime),
			CloseTimeResolution: hdr.CloseTimeResolution,
			Closed:              targetLedger.IsClosed() || targetLedger.IsValidated(),
			LedgerHash:          hdr.Hash,
			LedgerIndex:         hdr.LedgerIndex,
			ParentCloseTime:     toRippleTime(hdr.ParentCloseTime),
			ParentHash:          hdr.ParentHash,
			TotalCoins:          hdr.Drops,
			TransactionHash:     hdr.TxHash,
		}
	}

	count := uint32(0)
	var lastKey [32]byte
	passedMarker := !hasMarker

	err = targetLedger.ForEach(func(key [32]byte, data []byte) bool {
		// Skip until we pass the marker
		if !passedMarker {
			if key == startKey {
				passedMarker = true
			}
			return true
		}

		if count >= limit {
			result.Marker = formatHashHex(lastKey)
			return false
		}

		result.State = append(result.State, LedgerDataItem{
			Index: formatHashHex(key),
			Data:  data,
		})
		lastKey = key
		count++
		return true
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// AccountObjectsResult contains account objects
type AccountObjectsResult struct {
	Account            string             `json:"account"`
	AccountObjects     []AccountObjectItem `json:"account_objects"`
	LedgerIndex        uint32             `json:"ledger_index"`
	LedgerHash         [32]byte           `json:"ledger_hash"`
	Validated          bool               `json:"validated"`
	Marker             string             `json:"marker,omitempty"`
}

// AccountObjectItem represents an account object
type AccountObjectItem struct {
	Index           string `json:"index"`
	LedgerEntryType string `json:"LedgerEntryType"`
	Data            []byte `json:"data"`
}

// GetAccountObjects retrieves all objects owned by an account
func (s *Service) GetAccountObjects(account string, ledgerIndex string, objType string, limit uint32) (*AccountObjectsResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetLedger, validated, err := s.getLedgerForQuery(ledgerIndex)
	if err != nil {
		return nil, err
	}

	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(account)
	if err != nil {
		return nil, errors.New("invalid account address: " + err.Error())
	}
	var accountID [20]byte
	copy(accountID[:], accountIDBytes)

	if limit == 0 || limit > 400 {
		limit = 200
	}

	result := &AccountObjectsResult{
		Account:        account,
		AccountObjects: make([]AccountObjectItem, 0),
		LedgerIndex:    targetLedger.Sequence(),
		LedgerHash:     targetLedger.Hash(),
		Validated:      validated,
	}

	// Iterate through ledger and find objects for this account
	count := uint32(0)
	targetLedger.ForEach(func(key [32]byte, data []byte) bool {
		if count >= limit {
			return false
		}

		// Check if this object belongs to the account
		entryType := getLedgerEntryType(data)
		if entryType == "" {
			return true
		}

		// Filter by type if specified
		if objType != "" && entryType != objType {
			return true
		}

		// Check if object is associated with the account
		if !isObjectForAccount(data, accountID, entryType) {
			return true
		}

		result.AccountObjects = append(result.AccountObjects, AccountObjectItem{
			Index:           formatHashHex(key),
			LedgerEntryType: entryType,
			Data:            data,
		})
		count++
		return true
	})

	return result, nil
}

// getLedgerEntryType extracts the entry type from serialized data
func getLedgerEntryType(data []byte) string {
	if len(data) < 3 {
		return ""
	}
	if data[0] != 0x11 { // UInt16 type code
		return ""
	}
	entryType := uint16(data[1])<<8 | uint16(data[2])
	switch entryType {
	case 0x0061: // 'a' = AccountRoot
		return "AccountRoot"
	case 0x0063: // 'c' = Check
		return "Check"
	case 0x0064: // 'd' = DirNode
		return "DirectoryNode"
	case 0x0066: // 'f' = FeeSettings
		return "FeeSettings"
	case 0x0068: // 'h' = Escrow
		return "Escrow"
	case 0x006E: // 'n' = NFTokenPage
		return "NFTokenPage"
	case 0x006F: // 'o' = Offer
		return "Offer"
	case 0x0070: // 'p' = PayChannel
		return "PayChannel"
	case 0x0072: // 'r' = RippleState
		return "RippleState"
	case 0x0073: // 's' = SignerList
		return "SignerList"
	case 0x0074: // 't' = Ticket
		return "Ticket"
	case 0x0075: // 'u' = NFTokenOffer
		return "NFTokenOffer"
	case 0x0078: // 'x' = AMM
		return "AMM"
	default:
		return ""
	}
}

// isObjectForAccount checks if a ledger object belongs to an account
func isObjectForAccount(data []byte, accountID [20]byte, entryType string) bool {
	// This is a simplified check - in production, properly parse the object
	// For now, check if the account ID appears in the data
	for i := 0; i <= len(data)-20; i++ {
		match := true
		for j := 0; j < 20; j++ {
			if data[i+j] != accountID[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// formatHashHex formats a hash as hex string
func formatHashHex(hash [32]byte) string {
	const hexChars = "0123456789ABCDEF"
	result := make([]byte, 64)
	for i, b := range hash {
		result[i*2] = hexChars[b>>4]
		result[i*2+1] = hexChars[b&0x0F]
	}
	return string(result)
}

// hexDecode decodes a hex string to bytes
func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, errors.New("odd length hex string")
	}
	result := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		var b byte
		for j := 0; j < 2; j++ {
			c := s[i+j]
			switch {
			case c >= '0' && c <= '9':
				b = b<<4 | (c - '0')
			case c >= 'a' && c <= 'f':
				b = b<<4 | (c - 'a' + 10)
			case c >= 'A' && c <= 'F':
				b = b<<4 | (c - 'A' + 10)
			default:
				return nil, errors.New("invalid hex character")
			}
		}
		result[i/2] = b
	}
	return result, nil
}

// Helper function to decode account ID locally
func decodeAccountIDLocal(address string) ([20]byte, error) {
	var accountID [20]byte
	if address == "" {
		return accountID, errors.New("empty address")
	}
	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(address)
	if err != nil {
		return accountID, err
	}
	copy(accountID[:], accountIDBytes)
	return accountID, nil
}

// Helper function to check if amounts match currency (ignoring value)
func amountsMatchCurrency(a, b tx.Amount) bool {
	if a.IsNative() && b.IsNative() {
		return true
	}
	if a.IsNative() != b.IsNative() {
		return false
	}
	return a.Currency == b.Currency && a.Issuer == b.Issuer
}

// Helper function to calculate offer quality
func calculateOfferQuality(pays, gets tx.Amount) string {
	// Quality = TakerPays / TakerGets
	paysVal := parseAmountValue(pays)
	getsVal := parseAmountValue(gets)
	if getsVal == 0 {
		return "0"
	}
	quality := paysVal / getsVal
	return strconv.FormatFloat(quality, 'g', -1, 64)
}

// Helper function to parse amount value as float
func parseAmountValue(amt tx.Amount) float64 {
	if amt.IsNative() {
		drops, _ := strconv.ParseUint(amt.Value, 10, 64)
		return float64(drops)
	}
	val, _ := strconv.ParseFloat(amt.Value, 64)
	return val
}

// Helper function to format hash
func formatHash(hash [32]byte) string {
	return string(hash[:])
}

// Helper function to sort book offers by quality
func sortBookOffersByQuality(offers []BookOffer) {
	// Simple bubble sort - could use sort.Slice for better performance
	for i := 0; i < len(offers)-1; i++ {
		for j := i + 1; j < len(offers); j++ {
			qi, _ := strconv.ParseFloat(offers[i].Quality, 64)
			qj, _ := strconv.ParseFloat(offers[j].Quality, 64)
			if qj < qi { // Lower quality is better (cheaper)
				offers[i], offers[j] = offers[j], offers[i]
			}
		}
	}
}
