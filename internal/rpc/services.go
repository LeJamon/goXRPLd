package rpc

// Services provides access to core services from RPC handlers
// This is a singleton that holds references to the services
// needed by RPC method handlers.
var Services *ServiceContainer

// ServiceContainer holds references to all services needed by RPC handlers
type ServiceContainer struct {
	// LedgerService provides ledger operations
	Ledger LedgerService
}

// LedgerService interface for ledger operations
type LedgerService interface {
	// GetCurrentLedgerIndex returns the current open ledger index
	GetCurrentLedgerIndex() uint32

	// GetClosedLedgerIndex returns the last closed ledger index
	GetClosedLedgerIndex() uint32

	// GetValidatedLedgerIndex returns the highest validated ledger index
	GetValidatedLedgerIndex() uint32

	// AcceptLedger closes the current open ledger (standalone mode only)
	AcceptLedger() (uint32, error)

	// IsStandalone returns true if running in standalone mode
	IsStandalone() bool

	// GetServerInfo returns server status information
	GetServerInfo() LedgerServerInfo

	// GetLedgerBySequence returns a ledger by its sequence number
	GetLedgerBySequence(seq uint32) (LedgerReader, error)

	// GetLedgerByHash returns a ledger by its hash
	GetLedgerByHash(hash [32]byte) (LedgerReader, error)

	// GetGenesisAccount returns the genesis account address
	GetGenesisAccount() (string, error)

	// SubmitTransaction submits a transaction to the open ledger
	SubmitTransaction(txJSON []byte) (*SubmitResult, error)

	// GetCurrentFees returns the current fee settings
	GetCurrentFees() (baseFee, reserveBase, reserveIncrement uint64)

	// GetAccountInfo retrieves account information from the ledger
	GetAccountInfo(account string, ledgerIndex string) (*AccountInfo, error)

	// GetTransaction retrieves a transaction by its hash
	GetTransaction(txHash [32]byte) (*TransactionInfo, error)

	// StoreTransaction stores a transaction in the current ledger
	StoreTransaction(txHash [32]byte, txData []byte) error

	// GetAccountLines retrieves trust lines for an account
	GetAccountLines(account string, ledgerIndex string, peer string, limit uint32) (*AccountLinesResult, error)

	// GetAccountOffers retrieves offers for an account
	GetAccountOffers(account string, ledgerIndex string, limit uint32) (*AccountOffersResult, error)

	// GetBookOffers retrieves offers from an order book
	GetBookOffers(takerGets, takerPays Amount, ledgerIndex string, limit uint32) (*BookOffersResult, error)

	// GetAccountTransactions retrieves transaction history for an account
	GetAccountTransactions(account string, ledgerMin, ledgerMax int64, limit uint32, marker *AccountTxMarker, forward bool) (*AccountTxResult, error)

	// GetTransactionHistory retrieves recent transactions
	GetTransactionHistory(startIndex uint32) (*TxHistoryResult, error)

	// GetLedgerRange retrieves ledger hashes for a range of sequences
	GetLedgerRange(minSeq, maxSeq uint32) (*LedgerRangeResult, error)

	// GetLedgerEntry retrieves a specific ledger entry by its index/key
	GetLedgerEntry(entryKey [32]byte, ledgerIndex string) (*LedgerEntryResult, error)

	// GetLedgerData retrieves all ledger state entries with pagination
	GetLedgerData(ledgerIndex string, limit uint32, marker string) (*LedgerDataResult, error)

	// GetAccountObjects retrieves all objects owned by an account
	GetAccountObjects(account string, ledgerIndex string, objType string, limit uint32) (*AccountObjectsResult, error)
}

// AccountInfo contains account information from the ledger
type AccountInfo struct {
	Account      string
	Balance      string
	Flags        uint32
	OwnerCount   uint32
	Sequence     uint32
	RegularKey   string
	Domain       string
	EmailHash    string
	TransferRate uint32
	TickSize     uint8
	LedgerIndex  uint32
	LedgerHash   string
	Validated    bool
}

// LedgerReader provides read access to a ledger
type LedgerReader interface {
	Sequence() uint32
	Hash() [32]byte
	ParentHash() [32]byte
	IsClosed() bool
	IsValidated() bool
	TotalDrops() uint64
}

// LedgerServerInfo contains server status information from the ledger service
type LedgerServerInfo struct {
	Standalone          bool
	OpenLedgerSeq       uint32
	ClosedLedgerSeq     uint32
	ClosedLedgerHash    [32]byte
	ValidatedLedgerSeq  uint32
	ValidatedLedgerHash [32]byte
	CompleteLedgers     string
}

// SubmitResult contains the result of submitting a transaction
type SubmitResult struct {
	// EngineResult is the result code string (e.g., "tesSUCCESS")
	EngineResult string

	// EngineResultCode is the numeric result code
	EngineResultCode int

	// EngineResultMessage is a human-readable result message
	EngineResultMessage string

	// Applied indicates if the transaction was applied to the ledger
	Applied bool

	// Fee is the fee charged (in drops)
	Fee uint64

	// CurrentLedger is the current open ledger sequence
	CurrentLedger uint32

	// ValidatedLedger is the highest validated ledger sequence
	ValidatedLedger uint32
}

// TransactionInfo contains transaction data and metadata
type TransactionInfo struct {
	// TxData is the raw transaction data with metadata
	TxData []byte

	// LedgerIndex is the ledger sequence containing this transaction
	LedgerIndex uint32

	// LedgerHash is the hash of the containing ledger
	LedgerHash string

	// Validated indicates if the transaction is in a validated ledger
	Validated bool

	// TxIndex is the transaction's index within the ledger
	TxIndex uint32
}

// Amount represents a currency amount (XRP or IOU)
type Amount struct {
	Value    string `json:"value,omitempty"`
	Currency string `json:"currency,omitempty"`
	Issuer   string `json:"issuer,omitempty"`
}

// IsNative returns true if this is an XRP amount (not an IOU)
func (a Amount) IsNative() bool {
	return a.Currency == "" && a.Issuer == ""
}

// TrustLine represents a trust line from account_lines RPC
type TrustLine struct {
	Account        string `json:"account"`
	Balance        string `json:"balance"`
	Currency       string `json:"currency"`
	Limit          string `json:"limit"`
	LimitPeer      string `json:"limit_peer"`
	QualityIn      uint32 `json:"quality_in,omitempty"`
	QualityOut     uint32 `json:"quality_out,omitempty"`
	NoRipple       bool   `json:"no_ripple,omitempty"`
	NoRipplePeer   bool   `json:"no_ripple_peer,omitempty"`
	Authorized     bool   `json:"authorized,omitempty"`
	PeerAuthorized bool   `json:"peer_authorized,omitempty"`
	Freeze         bool   `json:"freeze,omitempty"`
	FreezePeer     bool   `json:"freeze_peer,omitempty"`
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

// BookOffer represents an offer in an order book
type BookOffer struct {
	Account         string      `json:"Account"`
	BookDirectory   string      `json:"BookDirectory"`
	BookNode        string      `json:"BookNode"`
	Flags           uint32      `json:"Flags"`
	LedgerEntryType string      `json:"LedgerEntryType"`
	OwnerNode       string      `json:"OwnerNode"`
	Sequence        uint32      `json:"Sequence"`
	TakerGets       interface{} `json:"TakerGets"`
	TakerPays       interface{} `json:"TakerPays"`
	Index           string      `json:"index"`
	Quality         string      `json:"quality"`
	OwnerFunds      string      `json:"owner_funds,omitempty"`
	TakerGetsFunded interface{} `json:"taker_gets_funded,omitempty"`
	TakerPaysFunded interface{} `json:"taker_pays_funded,omitempty"`
}

// BookOffersResult contains the result of book_offers RPC
type BookOffersResult struct {
	LedgerIndex uint32      `json:"ledger_index"`
	LedgerHash  [32]byte    `json:"ledger_hash"`
	Offers      []BookOffer `json:"offers"`
	Validated   bool        `json:"validated"`
}

// AccountTxMarker is used for pagination in account_tx
type AccountTxMarker struct {
	LedgerSeq uint32 `json:"ledger"`
	TxnSeq    uint32 `json:"seq"`
}

// AccountTransaction contains transaction data for account_tx
type AccountTransaction struct {
	Hash        [32]byte `json:"hash"`
	LedgerIndex uint32   `json:"ledger_index"`
	TxBlob      []byte   `json:"tx_blob,omitempty"`
	Meta        []byte   `json:"meta,omitempty"`
}

// AccountTxResult contains the result of account_tx query
type AccountTxResult struct {
	Account      string               `json:"account"`
	LedgerMin    uint32               `json:"ledger_index_min"`
	LedgerMax    uint32               `json:"ledger_index_max"`
	Limit        uint32               `json:"limit"`
	Marker       *AccountTxMarker     `json:"marker,omitempty"`
	Transactions []AccountTransaction `json:"transactions"`
	Validated    bool                 `json:"validated"`
}

// TxHistoryResult contains the result of tx_history query
type TxHistoryResult struct {
	Index        uint32               `json:"index"`
	Transactions []AccountTransaction `json:"txs"`
}

// LedgerRangeResult contains ledger hashes for a range
type LedgerRangeResult struct {
	LedgerFirst uint32              `json:"ledger_first"`
	LedgerLast  uint32              `json:"ledger_last"`
	Hashes      map[uint32][32]byte `json:"hashes"`
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

// LedgerDataItem represents a single state entry
type LedgerDataItem struct {
	Index string `json:"index"`
	Data  []byte `json:"data"`
}

// LedgerDataResult contains ledger state data
type LedgerDataResult struct {
	LedgerIndex uint32           `json:"ledger_index"`
	LedgerHash  [32]byte         `json:"ledger_hash"`
	State       []LedgerDataItem `json:"state"`
	Marker      string           `json:"marker,omitempty"`
	Validated   bool             `json:"validated"`
}

// AccountObjectItem represents an account object
type AccountObjectItem struct {
	Index           string `json:"index"`
	LedgerEntryType string `json:"LedgerEntryType"`
	Data            []byte `json:"data"`
}

// AccountObjectsResult contains account objects
type AccountObjectsResult struct {
	Account        string              `json:"account"`
	AccountObjects []AccountObjectItem `json:"account_objects"`
	LedgerIndex    uint32              `json:"ledger_index"`
	LedgerHash     [32]byte            `json:"ledger_hash"`
	Validated      bool                `json:"validated"`
	Marker         string              `json:"marker,omitempty"`
}

// InitServices initializes the service container
func InitServices(ledger LedgerService) {
	Services = &ServiceContainer{
		Ledger: ledger,
	}
}
