package adaptor

import (
	"context"
	"testing"
	"time"
)

func TestIOLatencyProbe_ZeroBeforeStart(t *testing.T) {
	probe := NewIOLatencyProbe(nil)
	if got := probe.LatencyMs(); got != 0 {
		t.Errorf("expected 0 before any samples, got %d", got)
	}
}

func TestIOLatencyProbe_RecordSample(t *testing.T) {
	probe := NewIOLatencyProbe(nil)
	posted := time.Now().Add(-5 * time.Millisecond)
	probe.RecordSample(posted)
	got := probe.LatencyMs()
	if got < 5 {
		t.Errorf("expected >= 5ms, got %d", got)
	}
}

func TestIOLatencyProbe_CeilBehavior(t *testing.T) {
	probe := NewIOLatencyProbe(nil)

	tests := []struct {
		name     string
		duration time.Duration
		wantMin  int
		wantMax  int
	}{
		{"exact_1ms", 1 * time.Millisecond, 1, 1},
		{"sub_ms", 500 * time.Microsecond, 1, 1},
		{"1100us", 1100 * time.Microsecond, 2, 2},
		{"zero", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ceilMs(tt.duration)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("ceilMs(%v) = %d, want [%d, %d]", tt.duration, got, tt.wantMin, tt.wantMax)
			}
		})
	}

	// Verify via RecordSample + LatencyMs round-trip
	probe.lastSample.Store(int64(1100 * time.Microsecond))
	if got := probe.LatencyMs(); got != 2 {
		t.Errorf("LatencyMs for 1.1ms = %d, want 2", got)
	}
}

func TestIOLatencyProbe_LastSampleNotAverage(t *testing.T) {
	probe := NewIOLatencyProbe(nil)

	// Record 100ms, then 1ms — should return ~1, not ~50
	probe.lastSample.Store(int64(100 * time.Millisecond))
	probe.lastSample.Store(int64(1 * time.Millisecond))

	got := probe.LatencyMs()
	if got != 1 {
		t.Errorf("expected last sample (1ms), got %d", got)
	}
}

func TestIOLatencyProbe_TickerSendsProbes(t *testing.T) {
	probe := NewIOLatencyProbe(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	probe.Start(ctx, 10*time.Millisecond)
	defer probe.Stop()

	select {
	case ts := <-probe.Ch():
		if ts.IsZero() {
			t.Error("received zero timestamp")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("timed out waiting for probe")
	}
}

func TestIOLatencyProbe_StopCancelsGoroutine(t *testing.T) {
	probe := NewIOLatencyProbe(nil)
	ctx := context.Background()

	probe.Start(ctx, 10*time.Millisecond)

	// Drain one probe to confirm it's running
	select {
	case <-probe.Ch():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("probe not running")
	}

	probe.Stop()
	time.Sleep(50 * time.Millisecond) // let ticker stop

	// Channel should be empty and stay empty
	select {
	case <-probe.Ch():
		// Might get one last buffered probe, that's ok
	default:
	}

	time.Sleep(50 * time.Millisecond)
	select {
	case <-probe.Ch():
		t.Error("received probe after Stop")
	default:
		// expected
	}
}
