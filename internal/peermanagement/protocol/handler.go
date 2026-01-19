// Package protocol implements the XRPL peer protocol message handling.
package protocol

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/compression"
	"github.com/LeJamon/goXRPLd/internal/peermanagement/message"
)

// PeerID is a unique identifier for a peer.
type PeerID uint64

// Handler is the interface for message handlers.
type Handler interface {
	// HandleMessage is called when a message is received.
	// Returns an error if the message should not be processed further.
	HandleMessage(ctx context.Context, peerID PeerID, msg message.Message) error
}

// HandlerFunc is a function adapter for Handler.
type HandlerFunc func(ctx context.Context, peerID PeerID, msg message.Message) error

func (f HandlerFunc) HandleMessage(ctx context.Context, peerID PeerID, msg message.Message) error {
	return f(ctx, peerID, msg)
}

// Dispatcher routes messages to appropriate handlers.
type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[message.MessageType][]Handler
	metrics  *Metrics
}

// NewDispatcher creates a new message dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		handlers: make(map[message.MessageType][]Handler),
		metrics:  NewMetrics(),
	}
}

// Register registers a handler for a message type.
func (d *Dispatcher) Register(msgType message.MessageType, handler Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[msgType] = append(d.handlers[msgType], handler)
}

// RegisterFunc registers a handler function for a message type.
func (d *Dispatcher) RegisterFunc(msgType message.MessageType, fn HandlerFunc) {
	d.Register(msgType, fn)
}

// Dispatch dispatches a message to registered handlers.
func (d *Dispatcher) Dispatch(ctx context.Context, peerID PeerID, msgType message.MessageType, payload []byte) error {
	// Track metrics
	d.metrics.RecordMessage(msgType, len(payload), true)

	// Decode the message
	msg, err := message.Decode(msgType, payload)
	if err != nil {
		return fmt.Errorf("failed to decode message type %s: %w", msgType, err)
	}

	// Get handlers
	d.mu.RLock()
	handlers := d.handlers[msgType]
	d.mu.RUnlock()

	// Call handlers
	for _, handler := range handlers {
		if err := handler.HandleMessage(ctx, peerID, msg); err != nil {
			return err
		}
	}

	return nil
}

// DispatchRaw dispatches a raw message (header + payload).
func (d *Dispatcher) DispatchRaw(ctx context.Context, peerID PeerID, header *message.Header, payload []byte) error {
	// Decompress if needed
	actualPayload := payload
	if header.Compressed {
		decompressed, err := compression.DecompressLZ4(payload, int(header.UncompressedSize))
		if err != nil {
			return fmt.Errorf("failed to decompress message: %w", err)
		}
		actualPayload = decompressed
	}

	return d.Dispatch(ctx, peerID, header.MessageType, actualPayload)
}

// Metrics returns the dispatcher's metrics.
func (d *Dispatcher) Metrics() *Metrics {
	return d.metrics
}

// Metrics tracks message statistics.
type Metrics struct {
	mu       sync.RWMutex
	counters map[message.MessageType]*MessageCounter
}

// MessageCounter tracks statistics for a message type.
type MessageCounter struct {
	MessagesIn  uint64
	MessagesOut uint64
	BytesIn     uint64
	BytesOut    uint64
	LastSeen    time.Time
}

// NewMetrics creates a new Metrics instance.
func NewMetrics() *Metrics {
	return &Metrics{
		counters: make(map[message.MessageType]*MessageCounter),
	}
}

// RecordMessage records a message for metrics.
func (m *Metrics) RecordMessage(msgType message.MessageType, size int, inbound bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	counter, ok := m.counters[msgType]
	if !ok {
		counter = &MessageCounter{}
		m.counters[msgType] = counter
	}

	if inbound {
		counter.MessagesIn++
		counter.BytesIn += uint64(size)
	} else {
		counter.MessagesOut++
		counter.BytesOut += uint64(size)
	}
	counter.LastSeen = time.Now()
}

// GetCounter returns the counter for a message type.
func (m *Metrics) GetCounter(msgType message.MessageType) *MessageCounter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.counters[msgType]
}

// GetAllCounters returns a copy of all counters.
func (m *Metrics) GetAllCounters() map[message.MessageType]*MessageCounter {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[message.MessageType]*MessageCounter)
	for k, v := range m.counters {
		// Copy the counter
		c := *v
		result[k] = &c
	}
	return result
}
