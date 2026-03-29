package adaptor

import (
	"context"
	"log/slog"
	"math"
	"sync/atomic"
	"time"
)

// IOLatencyProbe measures scheduling latency of the Router goroutine,
// analogous to rippled's beast::io_latency_probe on the ASIO io_service.
//
// A background ticker sends time.Now() every 100ms into a channel.
// The Router's select loop drains this channel; when it processes the
// probe, it computes elapsed = time.Since(posted) and stores the
// ceiling in milliseconds as the last sample.
//
// Reference: rippled/include/xrpl/beast/asio/io_latency_probe.h
// Reference: rippled/src/xrpld/app/main/Application.cpp io_latency_sampler
type IOLatencyProbe struct {
	probeCh    chan time.Time
	lastSample atomic.Int64 // nanoseconds; LatencyMs() converts with ceil
	cancel     context.CancelFunc
	logger     *slog.Logger
}

const (
	// DefaultProbePeriod matches rippled's 100ms sampling interval.
	DefaultProbePeriod = 100 * time.Millisecond

	// latencyWarningThreshold matches rippled's 500ms warning threshold.
	latencyWarningThreshold = 500 * time.Millisecond
)

// NewIOLatencyProbe creates a probe. Call Start() to begin the ticker goroutine.
func NewIOLatencyProbe(logger *slog.Logger) *IOLatencyProbe {
	if logger == nil {
		logger = slog.Default()
	}
	return &IOLatencyProbe{
		probeCh: make(chan time.Time, 1),
		logger:  logger,
	}
}

// Start launches the background ticker goroutine that sends probes.
func (p *IOLatencyProbe) Start(ctx context.Context, period time.Duration) {
	if period <= 0 {
		period = DefaultProbePeriod
	}
	ctx, p.cancel = context.WithCancel(ctx)
	go p.tickerLoop(ctx, period)
}

// tickerLoop sends time.Now() into probeCh every period.
// If the channel is full (Router is behind), the stale probe is
// replaced so we always measure the freshest delay.
func (p *IOLatencyProbe) tickerLoop(ctx context.Context, period time.Duration) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			// Non-blocking send: if channel is full, drain stale and resend.
			select {
			case p.probeCh <- now:
			default:
				select {
				case <-p.probeCh:
				default:
				}
				p.probeCh <- now
			}
		}
	}
}

// Ch returns the receive-only channel for the Router to select on.
func (p *IOLatencyProbe) Ch() <-chan time.Time {
	return p.probeCh
}

// RecordSample is called by the Router when it processes a probe from Ch().
// It computes ceil(elapsed) in ms and stores it atomically.
// Logs a warning when latency >= 500ms, matching rippled.
func (p *IOLatencyProbe) RecordSample(posted time.Time) {
	elapsed := time.Since(posted)
	p.lastSample.Store(int64(elapsed))
	if elapsed >= latencyWarningThreshold {
		p.logger.Warn("io_service latency", "ms", ceilMs(elapsed))
	}
}

// LatencyMs returns the last measured latency in milliseconds (ceil),
// matching rippled's ceil<milliseconds>(elapsed).
// Returns 0 if no sample has been recorded.
func (p *IOLatencyProbe) LatencyMs() int {
	ns := p.lastSample.Load()
	if ns <= 0 {
		return 0
	}
	return ceilMs(time.Duration(ns))
}

// Stop cancels the ticker goroutine.
func (p *IOLatencyProbe) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
}

// ceilMs returns ceil(d / 1ms) as int, matching rippled's
// std::chrono::ceil<milliseconds>(elapsed).
func ceilMs(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int(math.Ceil(float64(d.Nanoseconds()) / float64(time.Millisecond)))
}
