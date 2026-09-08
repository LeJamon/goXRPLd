package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/LeJamon/go-xrpl/shamap"
	"github.com/LeJamon/go-xrpl/storage/kvstore"
	"github.com/stretchr/testify/require"
)

type promotionProgressLogRecord struct {
	Message               string `json:"msg"`
	Requested             int    `json:"promotion_requested"`
	Consumed              int    `json:"promotion_consumed"`
	WritableHits          int    `json:"promotion_writable_hits"`
	WritableMisses        int    `json:"promotion_writable_misses"`
	ArchiveHits           int    `json:"promotion_archive_hits"`
	ArchiveMisses         int    `json:"promotion_archive_misses"`
	ArchiveLookups        int    `json:"promotion_archive_lookups"`
	ArchiveLookupsAvoided int    `json:"promotion_archive_lookups_avoided"`
	Promoted              int    `json:"promotion_promoted"`
	PromotedBytes         int    `json:"promotion_promoted_bytes"`
	BufferedBytes         int    `json:"promotion_buffered_bytes"`
	BatchWrites           int    `json:"promotion_batch_writes"`
	BatchCalls            uint64 `json:"promotion_batch_calls"`
	BatchErrors           uint64 `json:"promotion_batch_errors"`
	PartialPrefixes       uint64 `json:"promotion_partial_prefixes"`
	FetchElapsed          string `json:"promotion_fetch_elapsed"`
	WaitElapsed           string `json:"promotion_wait_elapsed"`
}

func decodePromotionProgressLogs(t *testing.T, capture *synchronizedLogBuffer) []promotionProgressLogRecord {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(capture.bytes()))
	var records []promotionProgressLogRecord
	for {
		var record promotionProgressLogRecord
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			return records
		}
		require.NoError(t, err)
		records = append(records, record)
	}
}

func TestOnlineDeleteRefreshPromotionMetricsAggregatesParallelBatches(t *testing.T) {
	const batches = 32
	metrics := newOnlineDeleteRefreshPromotionMetrics()
	var group sync.WaitGroup
	for index := range batches {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			var batchErr error
			if index%3 == 0 {
				batchErr = errors.New("batch failed")
			}
			metrics.record(
				2*time.Millisecond,
				3*time.Millisecond,
				kvstore.PromotionStats{
					Requested:             5,
					Consumed:              3,
					WritableHits:          1,
					WritableMisses:        2,
					ArchiveHits:           3,
					ArchiveMisses:         4,
					ArchiveLookups:        5,
					ArchiveLookupsAvoided: 6,
					Promoted:              7,
					PromotedBytes:         8,
					BufferedBytes:         9,
					Batches:               1,
				},
				batchErr,
			)
		}(index)
	}
	group.Wait()

	snapshot := metrics.snapshot()
	require.Equal(t, batches, int(snapshot.batchCalls))
	require.Equal(t, batches/3+1, int(snapshot.batchErrors))
	require.Equal(t, batches, int(snapshot.partialPrefixes))
	require.Equal(t, batches*2*time.Millisecond, snapshot.waitElapsed)
	require.Equal(t, batches*3*time.Millisecond, snapshot.fetchElapsed)
	require.Equal(t, kvstore.PromotionStats{
		Requested:             batches * 5,
		Consumed:              batches * 3,
		WritableHits:          batches,
		WritableMisses:        batches * 2,
		ArchiveHits:           batches * 3,
		ArchiveMisses:         batches * 4,
		ArchiveLookups:        batches * 5,
		ArchiveLookupsAvoided: batches * 6,
		Promoted:              batches * 7,
		PromotedBytes:         batches * 8,
		BufferedBytes:         batches * 9,
		Batches:               batches,
	}, snapshot.stats)
}

func TestOnlineDeleteRefreshPromotionMetricsOnlyAttachToNativeBatchRefresh(t *testing.T) {
	progress := newStoredSHAMapVerificationProgress(
		nil,
		nil,
		[32]byte{1},
		shamap.TypeState,
		time.Unix(0, 0),
	)
	fields := progress.fields(time.Unix(0, 1))
	for index := 0; index+1 < len(fields); index += 2 {
		require.NotEqual(t, "promotion_batch_calls", fields[index])
	}

	metrics := newOnlineDeleteRefreshPromotionMetrics()
	metrics.record(0, 0, kvstore.PromotionStats{Requested: 1, Consumed: 1}, nil)
	progress.promotionMetrics = metrics
	fields = progress.fields(time.Unix(0, 1))
	var found bool
	for index := 0; index+1 < len(fields); index += 2 {
		if fields[index] == "promotion_batch_calls" {
			found = true
			require.EqualValues(t, 1, fields[index+1])
		}
	}
	require.True(t, found)
}

func TestOnlineDeleteRefreshPromotionMetricsResetBetweenRefreshes(t *testing.T) {
	svc, db, seq := newRotatingRefreshFixture(t, 64)
	capture, logger := newVerificationLogCapture()
	svc.logger = logger
	root, err := svc.GetValidatedLedger().StateMapHash()
	require.NoError(t, err)

	beforeRequests := db.promotionRequests.Load()
	beforeBatches := db.promotionBatches.Load()
	require.NoError(t, svc.refreshGenerationStateWithBatch(
		t.Context(), root, seq, db, nil, 1, 64, storedSHAMapPromotionBatchBytes,
	))
	firstRequests := db.promotionRequests.Load() - beforeRequests
	firstBatches := db.promotionBatches.Load() - beforeBatches

	beforeRequests = db.promotionRequests.Load()
	beforeBatches = db.promotionBatches.Load()
	require.NoError(t, svc.refreshGenerationStateWithBatch(
		t.Context(), root, seq, db, nil, 1, 64, storedSHAMapPromotionBatchBytes,
	))
	secondRequests := db.promotionRequests.Load() - beforeRequests
	secondBatches := db.promotionBatches.Load() - beforeBatches

	records := decodePromotionProgressLogs(t, capture)
	require.Len(t, records, 4)
	require.Equal(t, "online delete: live-state refresh complete", records[1].Message)
	require.Equal(t, "online delete: live-state refresh complete", records[3].Message)
	// The traversal fetches its root through the scalar path before issuing
	// native batches, so the wrapper's request counter includes one extra key.
	require.EqualValues(t, firstRequests-1, records[1].Requested)
	require.EqualValues(t, secondRequests-1, records[3].Requested)
	require.EqualValues(t, firstBatches, records[1].BatchCalls)
	require.EqualValues(t, secondBatches, records[3].BatchCalls)
	require.Positive(t, records[1].BatchCalls)
	require.Positive(t, records[3].BatchCalls)
	require.NotEqual(t, uint64(firstBatches+secondBatches), records[3].BatchCalls)
}

func TestOnlineDeleteRefreshPromotionMetricsRetainBackendErrorAndFailureLog(t *testing.T) {
	svc, db, seq := newRotatingRefreshFixture(t, 64)
	root, err := svc.GetValidatedLedger().StateMapHash()
	require.NoError(t, err)
	wantErr := errors.New("promotion batch failed")
	probe := &failingPromotionDatabase{BatchGenerationDatabase: db, failure: wantErr}
	capture, logger := newVerificationLogCapture()
	refresh := &Service{nodeStore: probe, logger: logger}

	err = refresh.refreshGenerationStateWithBatch(
		context.Background(), root, seq, probe, nil, 1, 64, storedSHAMapPromotionBatchBytes,
	)
	require.ErrorIs(t, err, wantErr)
	require.Zero(t, probe.syncCalls.Load())

	records := decodePromotionProgressLogs(t, capture)
	require.Len(t, records, 2)
	require.Equal(t, "online delete: live-state refresh started", records[0].Message)
	require.Equal(t, "online delete: live-state refresh failed", records[1].Message)
	require.Positive(t, records[1].Requested)
	require.Positive(t, records[1].BatchCalls)
	require.Positive(t, records[1].BatchErrors)
	require.NotEqual(t, "online delete: live-state refresh complete", records[1].Message)
}
