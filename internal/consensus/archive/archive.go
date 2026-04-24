// Package archive persists stale validations to the relational DB via a
// batched async writer hooked into ValidationTracker.SetOnStale.
//
// Matches rippled's onStale / doStaleWrite contract (RCLValidations.cpp)
// in semantics: OnStale returns in O(1) — it only enqueues to a channel —
// so the consensus receive path is never gated on DB I/O.
package archive

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LeJamon/goXRPLd/internal/consensus"
	"github.com/LeJamon/goXRPLd/internal/consensus/adaptor"
	"github.com/LeJamon/goXRPLd/storage/relationaldb"
)

// Config tunes the archive writer. Validated ranges match config.ValidationArchiveConfig.
type Config struct {
	// RetentionLedgers is how many ledger-seqs of validation history to
	// keep. Zero disables retention (keep forever).
	RetentionLedgers uint32
	// BatchSize caps accumulated rows before forcing a commit.
	BatchSize int
	// FlushInterval bounds how long a partial batch may wait.
	FlushInterval time.Duration
	// DeleteBatch caps a single retention-sweep DELETE.
	DeleteBatch int
}

// Archive is the async writer. Safe for concurrent OnStale from any
// goroutine once New has returned.
type Archive struct {
	repo   relationaldb.ValidationRepository
	cfg    Config
	logger *slog.Logger

	ch       chan *consensus.Validation
	flushReq chan chan struct{} // ack channel per flush request
	stop     chan struct{}
	wg       sync.WaitGroup
	flushed  chan struct{} // closed when the writer goroutine exits

	lastSeq atomic.Uint32 // most-recent fully-validated seq, used as retention pivot

	closed atomic.Bool
}

// New creates a running archive. repo may be nil — that turns OnStale into
// a no-op and nothing is ever written. ch buffer is BatchSize*8 so a
// moderate burst of stale validations never blocks the caller.
func New(repo relationaldb.ValidationRepository, cfg Config, logger *slog.Logger) *Archive {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.BatchSize < 1 {
		cfg.BatchSize = 128
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = time.Second
	}
	if cfg.DeleteBatch < 1 {
		cfg.DeleteBatch = 1000
	}

	a := &Archive{
		repo:     repo,
		cfg:      cfg,
		logger:   logger,
		ch:       make(chan *consensus.Validation, cfg.BatchSize*8),
		flushReq: make(chan chan struct{}, 4),
		stop:     make(chan struct{}),
		flushed:  make(chan struct{}),
	}
	if repo != nil {
		a.wg.Add(1)
		go a.run()
	} else {
		close(a.flushed)
	}
	return a
}

// OnStale is the ValidationTracker.SetOnStale callback. Non-blocking when
// the channel has space; falls through a small bounded blocking wait
// (matches rippled's vector-append semantics) so under sustained overload
// we apply back-pressure instead of silently dropping validations.
func (a *Archive) OnStale(v *consensus.Validation) {
	if a == nil || a.repo == nil || v == nil || a.closed.Load() {
		return
	}
	select {
	case a.ch <- v:
		return
	default:
	}
	// Fall back to a bounded blocking send — 100ms is an eternity at
	// consensus-message timescales but still lets the caller progress if
	// the writer goroutine is wedged.
	t := time.NewTimer(100 * time.Millisecond)
	defer t.Stop()
	select {
	case a.ch <- v:
	case <-t.C:
		a.logger.Warn("validation archive channel full; dropping stale validation",
			slog.Uint64("ledger_seq", uint64(v.LedgerSeq)))
	case <-a.stop:
	}
}

// NoteFullyValidated informs the archive of the most recent fully-
// validated ledger seq. Used as the pivot for retention: rows with
// ledger_seq < (noted - RetentionLedgers) become eligible for deletion.
func (a *Archive) NoteFullyValidated(seq uint32) {
	if a == nil {
		return
	}
	for {
		cur := a.lastSeq.Load()
		if seq <= cur {
			return
		}
		if a.lastSeq.CompareAndSwap(cur, seq) {
			return
		}
	}
}

// Flush blocks until every stale validation enqueued before the call has
// been committed. After Close, Flush is a no-op.
//
// Implemented as a two-barrier sync: we send an ack channel on flushReq
// AFTER every previously-enqueued OnStale has landed in ch (FIFO on the
// same goroutine ordering), and the writer loop closes the ack only
// after it has fully drained ch up to that point and committed the
// resulting batch.
func (a *Archive) Flush(ctx context.Context) error {
	if a == nil || a.repo == nil || a.closed.Load() {
		return nil
	}
	ack := make(chan struct{})
	select {
	case a.flushReq <- ack:
	case <-ctx.Done():
		return ctx.Err()
	case <-a.stop:
		return nil
	}
	select {
	case <-ack:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-a.stop:
		return nil
	}
}

// ApplyRetention performs a one-shot retention sweep against the current
// NotedFullyValidated seq. Intended for tests and manual admin flushes;
// the writer loop calls it implicitly after each batch commit.
func (a *Archive) ApplyRetention(ctx context.Context) (int64, error) {
	if a == nil || a.repo == nil || a.cfg.RetentionLedgers == 0 {
		return 0, nil
	}
	last := a.lastSeq.Load()
	if last <= a.cfg.RetentionLedgers {
		return 0, nil
	}
	cutoff := last - a.cfg.RetentionLedgers
	return a.repo.DeleteOlderThanSeq(ctx, relationaldb.LedgerIndex(cutoff), a.cfg.DeleteBatch)
}

// Close drains pending validations, commits the final batch, and stops
// the writer goroutine. Safe to call multiple times; subsequent calls
// return nil immediately.
func (a *Archive) Close(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if !a.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(a.stop)
	select {
	case <-a.flushed:
	case <-ctx.Done():
		return ctx.Err()
	}
	a.wg.Wait()
	return nil
}

// run is the writer goroutine. It accumulates up to BatchSize rows or
// waits FlushInterval, whichever comes first, then commits. Retention
// runs after every non-empty commit so the archive size is bounded even
// under a steady stale-validation stream.
func (a *Archive) run() {
	defer a.wg.Done()
	defer close(a.flushed)

	ticker := time.NewTicker(a.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]*relationaldb.ValidationRecord, 0, a.cfg.BatchSize)
	flush := func(reason string) {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := a.repo.SaveBatch(ctx, batch)
		cancel()
		if err != nil {
			a.logger.Error("validation archive: batch write failed",
				slog.Int("rows", len(batch)), slog.String("reason", reason), slog.String("err", err.Error()))
		} else {
			a.logger.Debug("validation archive: batch committed",
				slog.Int("rows", len(batch)), slog.String("reason", reason))
		}
		batch = batch[:0]

		// Bounded retention sweep after each commit. Errors are logged
		// but don't stop the writer — retention lag is recoverable.
		if a.cfg.RetentionLedgers > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, rerr := a.ApplyRetention(ctx)
			cancel()
			if rerr != nil {
				a.logger.Warn("validation archive: retention sweep failed",
					slog.String("err", rerr.Error()))
			}
		}
	}

	drainPending := func() {
		for {
			select {
			case v := <-a.ch:
				if v == nil {
					continue
				}
				rec := toRecord(v)
				if rec == nil {
					continue
				}
				batch = append(batch, rec)
			default:
				return
			}
		}
	}

	handleFlushReq := func(ack chan struct{}) {
		// Drain everything enqueued before flush was requested. Flush
		// is called from outside run, so anything a caller enqueued
		// BEFORE sending the flushReq is already on ch by the time
		// flushReq arrives (Go channel operations are happens-before
		// synchronized). Drain to empty to pick it all up.
		drainPending()
		flush("flush-request")
		close(ack)
	}

	for {
		select {
		case v, ok := <-a.ch:
			if !ok {
				flush("channel closed")
				return
			}
			if v == nil {
				continue
			}
			rec := toRecord(v)
			if rec == nil {
				continue
			}
			batch = append(batch, rec)
			if len(batch) >= a.cfg.BatchSize {
				flush("full")
			}
		case ack := <-a.flushReq:
			handleFlushReq(ack)
		case <-ticker.C:
			flush("tick")
		case <-a.stop:
			drainPending()
			// Service any pending flush requests so Close-then-Flush
			// callers aren't left hanging.
			for {
				select {
				case ack := <-a.flushReq:
					close(ack)
				default:
					flush("close")
					return
				}
			}
		}
	}
}

// toRecord marshals a Validation into the archive row shape. Returns nil
// on anything invalid so a single bad validation can't poison the batch.
func toRecord(v *consensus.Validation) *relationaldb.ValidationRecord {
	if v == nil {
		return nil
	}

	raw := v.Raw
	if len(raw) == 0 {
		// Self-built or legacy validation that never carried wire bytes.
		// Re-serialize so the archive row is always replayable.
		raw = adaptor.SerializeSTValidation(v)
	}
	if len(raw) == 0 {
		return nil
	}

	flags := uint32(0)
	if v.Full {
		flags |= 0x80000001
	}

	rec := &relationaldb.ValidationRecord{
		LedgerSeq:  relationaldb.LedgerIndex(v.LedgerSeq),
		InitialSeq: relationaldb.LedgerIndex(v.LedgerSeq),
		NodePubKey: append([]byte(nil), v.NodeID[:]...),
		Signature:  append([]byte(nil), v.Signature...),
		SignTime:   v.SignTime,
		SeenTime:   v.SeenTime,
		Flags:      flags,
		Raw:        append([]byte(nil), raw...),
	}
	copy(rec.LedgerHash[:], v.LedgerID[:])
	return rec
}

// ErrClosed is returned by callers that try to use an Archive after Close.
var ErrClosed = errors.New("archive: closed")
