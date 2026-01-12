package rpc

import (
	"encoding/json"
	"log"
	"sync"
)

// EventPublisher publishes events to WebSocket subscribers
// This interface allows the ledger service and other components to publish
// events without directly depending on the WebSocket/subscription implementation
type EventPublisher interface {
	// PublishLedgerClosed publishes a ledger close event to all ledger stream subscribers
	PublishLedgerClosed(event *LedgerCloseEvent)

	// PublishTransaction publishes a transaction event to transaction stream subscribers
	// If affectedAccounts is provided, the event is also sent to account subscribers
	PublishTransaction(event *TransactionEvent, affectedAccounts []string)

	// PublishValidation publishes a validation event to validation stream subscribers
	PublishValidation(event *ValidationEvent)

	// PublishServerStatus publishes a server status event to server stream subscribers
	PublishServerStatus(event *ServerStatusEvent)

	// PublishConsensusPhase publishes a consensus phase change to consensus stream subscribers
	PublishConsensusPhase(phase string)

	// PublishManifest publishes a manifest event to manifest stream subscribers
	PublishManifest(event *ManifestEvent)

	// PublishPeerStatus publishes a peer status event to peer_status stream subscribers
	PublishPeerStatus(event *PeerStatusEvent)

	// PublishProposedTransaction publishes a proposed transaction to accounts_proposed subscribers
	PublishProposedTransaction(event *ProposedTransactionEvent, accounts []string)

	// PublishOrderBookChange publishes an order book change to book subscribers
	PublishOrderBookChange(event *OrderBookChangeEvent, takerGets, takerPays CurrencySpec)

	// GetSubscriberCount returns the number of active subscribers for a stream type
	GetSubscriberCount(streamType SubscriptionType) int
}

// Note: CurrencySpec is defined in subscription_methods.go

// Publisher implements EventPublisher using SubscriptionManager
type Publisher struct {
	manager *SubscriptionManager
	mu      sync.RWMutex
}

// NewPublisher creates a new Publisher with the given subscription manager
func NewPublisher(manager *SubscriptionManager) *Publisher {
	return &Publisher{
		manager: manager,
	}
}

// PublishLedgerClosed broadcasts a ledger close event to all ledger stream subscribers
func (p *Publisher) PublishLedgerClosed(event *LedgerCloseEvent) {
	if event == nil || p.manager == nil {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal LedgerCloseEvent: %v", err)
		return
	}

	p.manager.BroadcastToStream(SubLedger, data, nil)
}

// PublishTransaction broadcasts a transaction event to subscribers
func (p *Publisher) PublishTransaction(event *TransactionEvent, affectedAccounts []string) {
	if event == nil || p.manager == nil {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal TransactionEvent: %v", err)
		return
	}

	// Broadcast to transactions stream
	p.manager.BroadcastToStream(SubTransactions, data, nil)

	// Also broadcast to affected account subscribers
	if len(affectedAccounts) > 0 {
		p.manager.BroadcastToAccounts(data, affectedAccounts)
	}
}

// PublishValidation broadcasts a validation event to validation stream subscribers
func (p *Publisher) PublishValidation(event *ValidationEvent) {
	if event == nil || p.manager == nil {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal ValidationEvent: %v", err)
		return
	}

	p.manager.BroadcastToStream(SubValidations, data, nil)
}

// PublishServerStatus broadcasts a server status event to server stream subscribers
func (p *Publisher) PublishServerStatus(event *ServerStatusEvent) {
	if event == nil || p.manager == nil {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal ServerStatusEvent: %v", err)
		return
	}

	// Server status goes to the "server" stream (we use SubPeerStatus as a proxy here)
	// In a full implementation, there would be a separate SubServer type
	p.manager.BroadcastToStream(SubPeerStatus, data, nil)
}

// PublishConsensusPhase broadcasts a consensus phase change event
func (p *Publisher) PublishConsensusPhase(phase string) {
	if p.manager == nil {
		return
	}

	event := NewConsensusEvent(phase)
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal ConsensusEvent: %v", err)
		return
	}

	p.manager.BroadcastToStream(SubConsensus, data, nil)
}

// PublishManifest broadcasts a manifest event to manifest stream subscribers
func (p *Publisher) PublishManifest(event *ManifestEvent) {
	if event == nil || p.manager == nil {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal ManifestEvent: %v", err)
		return
	}

	p.manager.BroadcastToStream(SubManifests, data, nil)
}

// PublishPeerStatus broadcasts a peer status event to peer_status stream subscribers
func (p *Publisher) PublishPeerStatus(event *PeerStatusEvent) {
	if event == nil || p.manager == nil {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal PeerStatusEvent: %v", err)
		return
	}

	p.manager.BroadcastToStream(SubPeerStatus, data, nil)
}

// PublishProposedTransaction broadcasts a proposed transaction to accounts_proposed subscribers
func (p *Publisher) PublishProposedTransaction(event *ProposedTransactionEvent, accounts []string) {
	if event == nil || p.manager == nil {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal ProposedTransactionEvent: %v", err)
		return
	}

	// Proposed transactions go to accounts_proposed subscribers
	p.manager.BroadcastToAccountsProposed(data, accounts)
}

// PublishOrderBookChange broadcasts an order book change to book subscribers
func (p *Publisher) PublishOrderBookChange(event *OrderBookChangeEvent, takerGets, takerPays CurrencySpec) {
	if event == nil || p.manager == nil {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal OrderBookChangeEvent: %v", err)
		return
	}

	// Broadcast to subscribers of this specific order book
	p.manager.BroadcastToOrderBook(data, takerGets, takerPays)
}

// GetSubscriberCount returns the number of active subscribers for a stream type
func (p *Publisher) GetSubscriberCount(streamType SubscriptionType) int {
	if p.manager == nil {
		return 0
	}
	return p.manager.GetSubscriberCount(streamType)
}

// NoOpPublisher is a publisher that does nothing (for testing or when subscriptions are disabled)
type NoOpPublisher struct{}

func NewNoOpPublisher() *NoOpPublisher {
	return &NoOpPublisher{}
}

func (p *NoOpPublisher) PublishLedgerClosed(event *LedgerCloseEvent)                   {}
func (p *NoOpPublisher) PublishTransaction(event *TransactionEvent, accounts []string) {}
func (p *NoOpPublisher) PublishValidation(event *ValidationEvent)                      {}
func (p *NoOpPublisher) PublishServerStatus(event *ServerStatusEvent)                  {}
func (p *NoOpPublisher) PublishConsensusPhase(phase string)                            {}
func (p *NoOpPublisher) PublishManifest(event *ManifestEvent)                          {}
func (p *NoOpPublisher) PublishPeerStatus(event *PeerStatusEvent)                      {}
func (p *NoOpPublisher) PublishProposedTransaction(event *ProposedTransactionEvent, accounts []string) {
}
func (p *NoOpPublisher) PublishOrderBookChange(event *OrderBookChangeEvent, takerGets, takerPays CurrencySpec) {
}
func (p *NoOpPublisher) GetSubscriberCount(streamType SubscriptionType) int { return 0 }

// Ensure implementations satisfy the interface
var _ EventPublisher = (*Publisher)(nil)
var _ EventPublisher = (*NoOpPublisher)(nil)
