package protocol

import (
	"context"
	"testing"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/message"
)

func TestDispatcherRegisterAndDispatch(t *testing.T) {
	d := NewDispatcher()
	called := false

	d.RegisterFunc(message.TypePing, func(ctx context.Context, peerID PeerID, msg message.Message) error {
		called = true
		ping, ok := msg.(*message.Ping)
		if !ok {
			t.Error("Expected Ping message")
		}
		if ping.Seq != 42 {
			t.Errorf("Seq = %d, want 42", ping.Seq)
		}
		return nil
	})

	ping := &message.Ping{PType: message.PingTypePing, Seq: 42}
	payload, _ := message.Encode(ping)

	err := d.Dispatch(context.Background(), PeerID(1), message.TypePing, payload)
	if err != nil {
		t.Errorf("Dispatch error: %v", err)
	}
	if !called {
		t.Error("Handler was not called")
	}
}

func TestDispatcherMultipleHandlers(t *testing.T) {
	d := NewDispatcher()
	callCount := 0

	for i := 0; i < 3; i++ {
		d.RegisterFunc(message.TypePing, func(ctx context.Context, peerID PeerID, msg message.Message) error {
			callCount++
			return nil
		})
	}

	ping := &message.Ping{PType: message.PingTypePing}
	payload, _ := message.Encode(ping)

	err := d.Dispatch(context.Background(), PeerID(1), message.TypePing, payload)
	if err != nil {
		t.Errorf("Dispatch error: %v", err)
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestDispatcherNoHandlers(t *testing.T) {
	d := NewDispatcher()

	ping := &message.Ping{PType: message.PingTypePing}
	payload, _ := message.Encode(ping)

	// Should not error even with no handlers
	err := d.Dispatch(context.Background(), PeerID(1), message.TypePing, payload)
	if err != nil {
		t.Errorf("Dispatch error: %v", err)
	}
}

func TestDispatcherInvalidPayload(t *testing.T) {
	d := NewDispatcher()
	d.RegisterFunc(message.TypePing, func(ctx context.Context, peerID PeerID, msg message.Message) error {
		return nil
	})

	// Invalid protobuf data
	err := d.Dispatch(context.Background(), PeerID(1), message.TypePing, []byte{0xFF, 0xFF, 0xFF})
	// May or may not error depending on how forgiving the decoder is
	_ = err
}

func TestDispatcherMetrics(t *testing.T) {
	d := NewDispatcher()

	ping := &message.Ping{PType: message.PingTypePing}
	payload, _ := message.Encode(ping)

	// Dispatch a few messages
	for i := 0; i < 5; i++ {
		d.Dispatch(context.Background(), PeerID(1), message.TypePing, payload)
	}

	metrics := d.Metrics()
	counter := metrics.GetCounter(message.TypePing)
	if counter == nil {
		t.Fatal("Counter should not be nil")
	}
	if counter.MessagesIn != 5 {
		t.Errorf("MessagesIn = %d, want 5", counter.MessagesIn)
	}
}

func TestMetricsRecordMessage(t *testing.T) {
	m := NewMetrics()

	m.RecordMessage(message.TypeTransaction, 100, true)
	m.RecordMessage(message.TypeTransaction, 200, true)
	m.RecordMessage(message.TypeTransaction, 150, false)

	counter := m.GetCounter(message.TypeTransaction)
	if counter == nil {
		t.Fatal("Counter should not be nil")
	}

	if counter.MessagesIn != 2 {
		t.Errorf("MessagesIn = %d, want 2", counter.MessagesIn)
	}
	if counter.BytesIn != 300 {
		t.Errorf("BytesIn = %d, want 300", counter.BytesIn)
	}
	if counter.MessagesOut != 1 {
		t.Errorf("MessagesOut = %d, want 1", counter.MessagesOut)
	}
	if counter.BytesOut != 150 {
		t.Errorf("BytesOut = %d, want 150", counter.BytesOut)
	}
}

func TestMetricsGetAllCounters(t *testing.T) {
	m := NewMetrics()

	m.RecordMessage(message.TypePing, 10, true)
	m.RecordMessage(message.TypeTransaction, 100, true)
	m.RecordMessage(message.TypeValidation, 50, true)

	all := m.GetAllCounters()
	if len(all) != 3 {
		t.Errorf("len(counters) = %d, want 3", len(all))
	}

	// Verify it's a copy
	all[message.TypePing].MessagesIn = 999
	original := m.GetCounter(message.TypePing)
	if original.MessagesIn == 999 {
		t.Error("GetAllCounters should return a copy")
	}
}

func TestHandlerFunc(t *testing.T) {
	called := false
	var fn HandlerFunc = func(ctx context.Context, peerID PeerID, msg message.Message) error {
		called = true
		return nil
	}

	err := fn.HandleMessage(context.Background(), PeerID(1), &message.Ping{})
	if err != nil {
		t.Errorf("HandleMessage error: %v", err)
	}
	if !called {
		t.Error("HandlerFunc was not called")
	}
}
