package tx_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/LeJamon/go-xrpl/internal/tx"
	"github.com/LeJamon/go-xrpl/internal/tx/all"
	"github.com/LeJamon/go-xrpl/internal/tx/sign"
	fixture "github.com/LeJamon/go-xrpl/internal/tx/testdata"
)

func TestIssue1861HistoricalOperationLimitTransaction(t *testing.T) {
	all.RegisterAll()

	raw, err := hex.DecodeString(fixture.Issue1861HistoricalTxBlobHex)
	if err != nil {
		t.Fatalf("decode historical transaction: %v", err)
	}
	if len(raw) != 196 {
		t.Fatalf("historical transaction length = %d, want 196", len(raw))
	}

	parsed, err := tx.ParseFromBinary(raw)
	if err != nil {
		t.Fatalf("parse historical transaction: %v", err)
	}
	if !bytes.Equal(parsed.GetRawBytes(), raw) {
		t.Fatal("historical transaction did not retain byte-identical raw bytes")
	}

	id, err := tx.ComputeTransactionHash(parsed)
	if err != nil {
		t.Fatalf("compute historical transaction ID: %v", err)
	}
	if got := fmt.Sprintf("%X", id); got != fixture.Issue1861HistoricalTxIDHex {
		t.Fatalf("historical transaction ID = %s, want %s", got, fixture.Issue1861HistoricalTxIDHex)
	}
	if err := sign.VerifySignature(parsed, true); err != nil {
		t.Fatalf("verify historical transaction signature: %v", err)
	}

	// Force serialization from the typed projection instead of the retained
	// raw-byte fast path; this proves OperationLimit survives the full round trip.
	parsed.SetRawBytes(nil)
	rebuilt, err := tx.SerializeTransaction(parsed)
	if err != nil {
		t.Fatalf("serialize historical transaction from typed fields: %v", err)
	}
	if !bytes.Equal(rebuilt, raw) {
		t.Fatalf("typed reserialization differs from historical bytes:\n got %X\nwant %X", rebuilt, raw)
	}
}
