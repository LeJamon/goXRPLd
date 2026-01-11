package rpc

import (
	"context"
	"encoding/json"
)

// JSON-RPC 2.0 Request
type JsonRpcRequest struct {
	JsonRpc string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
}

// JSON-RPC 2.0 Response
type JsonRpcResponse struct {
	JsonRpc string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RpcError   `json:"error,omitempty"`
	ID      interface{} `json:"id,omitempty"`
}

// XRPL API Version constants
const (
	ApiVersion1 = 1
	ApiVersion2 = 2
	ApiVersion3 = 3
	DefaultApiVersion = ApiVersion1
)

// Role-based access control matching rippled
type Role int

const (
	RoleGuest Role = iota
	RoleUser
	RoleAdmin
	RoleIdentified
)

// RPC Context contains request-specific information
type RpcContext struct {
	Context    context.Context
	Role       Role
	ApiVersion int
	IsAdmin    bool
	ClientIP   string
}

// Method handler interface - all RPC methods implement this
type MethodHandler interface {
	Handle(ctx *RpcContext, params json.RawMessage) (interface{}, *RpcError)
	RequiredRole() Role
	SupportedApiVersions() []int
}

// Method registry for dynamic method registration
type MethodRegistry struct {
	methods map[string]MethodHandler
}

func NewMethodRegistry() *MethodRegistry {
	return &MethodRegistry{
		methods: make(map[string]MethodHandler),
	}
}

func (r *MethodRegistry) Register(name string, handler MethodHandler) {
	r.methods[name] = handler
}

func (r *MethodRegistry) Get(name string) (MethodHandler, bool) {
	handler, exists := r.methods[name]
	return handler, exists
}

func (r *MethodRegistry) List() []string {
	methods := make([]string, 0, len(r.methods))
	for name := range r.methods {
		methods = append(methods, name)
	}
	return methods
}

// Base response structure for XRPL RPC responses
type BaseResponse struct {
	// Standard fields present in most responses
	Status        string `json:"status,omitempty"`
	Type          string `json:"type,omitempty"`
	Validated     *bool  `json:"validated,omitempty"`
	LedgerHash    string `json:"ledger_hash,omitempty"`
	LedgerIndex   uint32 `json:"ledger_index,omitempty"`
	LedgerCurrent uint32 `json:"ledger_current_index,omitempty"`
}

// WebSocket specific structures
type WebSocketCommand struct {
	Command     string          `json:"command"`
	ID          interface{}     `json:"id,omitempty"`
	ApiVersion  *int           `json:"api_version,omitempty"`
	Params      json.RawMessage `json:",inline,omitempty"`
}

type WebSocketResponse struct {
	Type        string      `json:"type"`
	ID          interface{} `json:"id,omitempty"`
	Status      string      `json:"status,omitempty"`
	Result      interface{} `json:"result,omitempty"`
	Error       *RpcError   `json:"error,omitempty"`
	ApiVersion  int         `json:"api_version,omitempty"`
}

// Subscription types for WebSocket streams
type SubscriptionType string

const (
	SubLedger       SubscriptionType = "ledger"
	SubTransactions SubscriptionType = "transactions"
	SubAccounts     SubscriptionType = "accounts"
	SubOrderBooks   SubscriptionType = "book_changes"
	SubValidations  SubscriptionType = "validations"
	SubManifests    SubscriptionType = "manifests"
	SubPeerStatus   SubscriptionType = "peer_status"
	SubConsensus    SubscriptionType = "consensus"
	SubPath         SubscriptionType = "path_find"
)

// Subscription request structure
type SubscriptionRequest struct {
	Streams      []SubscriptionType `json:"streams,omitempty"`
	Accounts     []string          `json:"accounts,omitempty"`
	AccountsProposed []string      `json:"accounts_proposed,omitempty"`
	Books        []BookRequest     `json:"books,omitempty"`
	URL          string            `json:"url,omitempty"`
	URLUsername  string            `json:"url_username,omitempty"`
	URLPassword  string            `json:"url_password,omitempty"`
}

// Book request for order book subscriptions
type BookRequest struct {
	TakerPays json.RawMessage `json:"taker_pays"`
	TakerGets json.RawMessage `json:"taker_gets"`
	Snapshot  bool           `json:"snapshot,omitempty"`
	Both      bool           `json:"both,omitempty"`
}

// Stream message types
type StreamMessage struct {
	Type           string          `json:"type"`
	LedgerIndex    uint32          `json:"ledger_index,omitempty"`
	LedgerHash     string          `json:"ledger_hash,omitempty"`
	LedgerTime     uint32          `json:"ledger_time,omitempty"`
	FeeBase        uint32          `json:"fee_base,omitempty"`
	FeeRef         uint32          `json:"fee_ref,omitempty"`
	ReserveBase    uint32          `json:"reserve_base,omitempty"`
	ReserveInc     uint32          `json:"reserve_inc,omitempty"`
	Validated      bool            `json:"validated,omitempty"`
	Transaction    json.RawMessage `json:"transaction,omitempty"`
	Meta           json.RawMessage `json:"meta,omitempty"`
	Account        string          `json:"account,omitempty"`
}

// Common parameter structures used across multiple methods

// LedgerSpecifier - used to specify which ledger to query
type LedgerSpecifier struct {
	LedgerHash  string `json:"ledger_hash,omitempty"`
	LedgerIndex string `json:"ledger_index,omitempty"` // can be number or "validated", "current", "closed"
}

// Account parameter
type AccountParam struct {
	Account string `json:"account"`
}

// Transaction identifier
type TransactionParam struct {
	Transaction string `json:"transaction"`
	Binary      bool   `json:"binary,omitempty"`
}

// Pagination parameters
type PaginationParams struct {
	Limit  uint32      `json:"limit,omitempty"`
	Marker interface{} `json:"marker,omitempty"`
}

// Currency specification
type Currency struct {
	Currency string `json:"currency"`
	Issuer   string `json:"issuer,omitempty"`
}

// RawAmount can be drops (string) or IOU object (used for JSON parsing)
type RawAmount json.RawMessage

// Path specification for path finding
type Path []PathStep

type PathStep struct {
	Account  string `json:"account,omitempty"`
	Currency string `json:"currency,omitempty"`
	Issuer   string `json:"issuer,omitempty"`
	Type     uint8  `json:"type,omitempty"`
	TypeHex  string `json:"type_hex,omitempty"`
}

// Quality specification
type Quality struct {
	Currency string `json:"currency"`
	Issuer   string `json:"issuer,omitempty"`
	Value    string `json:"value"`
}

// Memo structure
type Memo struct {
	MemoData   string `json:"MemoData,omitempty"`
	MemoFormat string `json:"MemoFormat,omitempty"`
	MemoType   string `json:"MemoType,omitempty"`
}

// Signer structure
type Signer struct {
	Signer struct {
		Account       string `json:"Account"`
		TxnSignature  string `json:"TxnSignature"`
		SigningPubKey string `json:"SigningPubKey"`
	} `json:"Signer"`
}