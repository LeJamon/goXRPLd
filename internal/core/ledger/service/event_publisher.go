package service

import (
	"time"
)

// EventPublisher manages event callbacks and hooks for ledger events.
type EventPublisher struct {
	// Legacy event callback (for backward compatibility)
	eventCallback EventCallback

	// Structured event hooks
	hooks *EventHooks
}

// EventHooks provides structured callbacks for ledger events.
type EventHooks struct {
	// OnLedgerClosed is called when a ledger is closed
	OnLedgerClosed func(ledgerInfo *LedgerInfo, txCount int, validatedLedgers string)

	// OnTransaction is called for each transaction in a closed ledger
	OnTransaction func(txInfo TransactionInfo, result TxResult, ledgerSeq uint32, ledgerHash [32]byte, closeTime time.Time)

	// OnValidation is called when a validation is received
	OnValidation func(validation ValidationInfo)
}

// TransactionInfo contains transaction details for event broadcasting.
type TransactionInfo struct {
	// Hash is the transaction hash
	Hash [32]byte

	// TxBlob is the raw transaction data
	TxBlob []byte

	// AffectedAccounts lists the accounts affected by this transaction
	AffectedAccounts []string
}

// TxResult contains the result of a transaction.
type TxResult struct {
	// Applied indicates if the transaction was applied
	Applied bool

	// Metadata is the transaction metadata
	Metadata []byte

	// TxIndex is the index in the ledger
	TxIndex uint32
}

// ValidationInfo contains validation information.
type ValidationInfo struct {
	// LedgerHash is the hash of the validated ledger
	LedgerHash [32]byte

	// LedgerSeq is the sequence of the validated ledger
	LedgerSeq uint32

	// ValidatorKey is the public key of the validator
	ValidatorKey string

	// Signature is the validation signature
	Signature []byte
}

// NewEventPublisher creates a new event publisher.
func NewEventPublisher() *EventPublisher {
	return &EventPublisher{}
}

// SetEventCallback sets the legacy callback function.
func (p *EventPublisher) SetEventCallback(callback EventCallback) {
	p.eventCallback = callback
}

// GetEventCallback returns the legacy callback function.
func (p *EventPublisher) GetEventCallback() EventCallback {
	return p.eventCallback
}

// SetEventHooks sets the structured event hooks.
func (p *EventPublisher) SetEventHooks(hooks *EventHooks) {
	p.hooks = hooks
}

// GetEventHooks returns the current event hooks.
func (p *EventPublisher) GetEventHooks() *EventHooks {
	return p.hooks
}

// HasSubscribers returns true if there are any subscribers.
func (p *EventPublisher) HasSubscribers() bool {
	return p.eventCallback != nil ||
		(p.hooks != nil && (p.hooks.OnLedgerClosed != nil || p.hooks.OnTransaction != nil))
}

// PublishLedgerAccepted publishes a ledger accepted event.
func (p *EventPublisher) PublishLedgerAccepted(event *LedgerAcceptedEvent) {
	if p.eventCallback != nil {
		go p.eventCallback(event)
	}
}

// PublishLedgerClosed publishes a ledger closed event via hooks.
func (p *EventPublisher) PublishLedgerClosed(ledgerInfo *LedgerInfo, txCount int, validatedLedgers string) {
	if p.hooks != nil && p.hooks.OnLedgerClosed != nil {
		go p.hooks.OnLedgerClosed(ledgerInfo, txCount, validatedLedgers)
	}
}

// PublishTransaction publishes a transaction event via hooks.
func (p *EventPublisher) PublishTransaction(txInfo TransactionInfo, result TxResult, ledgerSeq uint32, ledgerHash [32]byte, closeTime time.Time) {
	if p.hooks != nil && p.hooks.OnTransaction != nil {
		go p.hooks.OnTransaction(txInfo, result, ledgerSeq, ledgerHash, closeTime)
	}
}

// PublishValidation publishes a validation event via hooks.
func (p *EventPublisher) PublishValidation(validation ValidationInfo) {
	if p.hooks != nil && p.hooks.OnValidation != nil {
		go p.hooks.OnValidation(validation)
	}
}
