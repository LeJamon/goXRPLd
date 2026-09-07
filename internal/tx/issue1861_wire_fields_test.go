package tx_test

import (
	"bytes"
	"testing"

	"github.com/LeJamon/go-xrpl/codec/binarycodec"
	"github.com/LeJamon/go-xrpl/internal/tx"
	"github.com/LeJamon/go-xrpl/internal/tx/all"
	"github.com/LeJamon/go-xrpl/internal/tx/amm"
	"github.com/LeJamon/go-xrpl/internal/tx/batch"
)

const (
	issue1861Account     = "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
	issue1861Destination = "rPMh7Pi9ct699iZUTWaytJUoHcJ7cgyziK"
	issue1861PreviousID  = "0101010101010101010101010101010101010101010101010101010101010101"
	issue1861ZeroHash    = "0000000000000000000000000000000000000000000000000000000000000000"
)

func issue1861BaseFields(txType string) map[string]any {
	return map[string]any{
		"TransactionType": txType,
		"Account":         issue1861Account,
		"Sequence":        uint32(1),
		"Fee":             "10",
		"SigningPubKey":   "",
	}
}

func issue1861Encode(t *testing.T, fields map[string]any) []byte {
	t.Helper()
	blob, err := binarycodec.EncodeBytes(fields)
	if err != nil {
		t.Fatalf("encode transaction: %v", err)
	}
	return blob
}

func issue1861Ptr[T any](value T) *T {
	return &value
}

func TestIssue1861CommonFieldsRoundTripAndMutation(t *testing.T) {
	all.RegisterAll()

	const operationLimit = uint32(21338)
	tests := []struct {
		name            string
		operationLimit  *uint32
		previousTxnID   string
		wantOperation   bool
		wantPrevious    bool
		mutateOperation bool
		mutatePrevious  bool
	}{
		{
			name:            "both absent",
			mutateOperation: true,
			mutatePrevious:  true,
		},
		{
			name:            "operation limit explicit zero",
			operationLimit:  issue1861Ptr(uint32(0)),
			wantOperation:   true,
			mutateOperation: true,
		},
		{
			name:            "operation limit nonzero",
			operationLimit:  issue1861Ptr(operationLimit),
			wantOperation:   true,
			mutateOperation: true,
		},
		{
			name:           "previous transaction zero",
			previousTxnID:  issue1861ZeroHash,
			wantPrevious:   true,
			mutatePrevious: true,
		},
		{
			name:           "previous transaction nonzero",
			previousTxnID:  issue1861PreviousID,
			wantPrevious:   true,
			mutatePrevious: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields := issue1861BaseFields("AccountSet")
			if test.operationLimit != nil {
				fields["OperationLimit"] = *test.operationLimit
			}
			if test.previousTxnID != "" {
				fields["PreviousTxnID"] = test.previousTxnID
			}

			blob := issue1861Encode(t, fields)
			parsed, err := tx.ParseFromBinary(blob)
			if err != nil {
				t.Fatalf("parse transaction: %v", err)
			}
			if !bytes.Equal(parsed.GetRawBytes(), blob) {
				t.Fatal("parsed transaction did not retain canonical bytes")
			}

			flat, err := parsed.Flatten()
			if err != nil {
				t.Fatalf("flatten transaction: %v", err)
			}
			operation, hasOperation := flat["OperationLimit"]
			if hasOperation != test.wantOperation {
				t.Fatalf("OperationLimit present = %v, want %v (map: %#v)", hasOperation, test.wantOperation, flat)
			}
			if hasOperation && operation != issue1861OperationLimitValue(test.operationLimit) {
				t.Fatalf("OperationLimit = %#v, want %d", operation, issue1861OperationLimitValue(test.operationLimit))
			}
			previous, hasPrevious := flat["PreviousTxnID"]
			if hasPrevious != test.wantPrevious {
				t.Fatalf("PreviousTxnID present = %v, want %v (map: %#v)", hasPrevious, test.wantPrevious, flat)
			}
			if hasPrevious && previous != test.previousTxnID {
				t.Fatalf("PreviousTxnID = %#v, want %q", previous, test.previousTxnID)
			}

			matches, err := tx.CurrentFieldsMatchRaw(parsed)
			if err != nil {
				t.Fatalf("compare parsed fields to raw bytes: %v", err)
			}
			if !matches {
				t.Fatal("parsed fields do not match retained raw bytes")
			}

			// Clear the retained bytes so this check exercises Flatten and the
			// common-field serializer rather than SerializeTransaction's raw fast path.
			parsed.SetRawBytes(nil)
			rebuilt, err := tx.SerializeTransaction(parsed)
			if err != nil {
				t.Fatalf("serialize parsed fields: %v", err)
			}
			if !bytes.Equal(rebuilt, blob) {
				t.Fatalf("rebuilt bytes differ from canonical input:\n got %X\nwant %X", rebuilt, blob)
			}

			// Rebind the original bytes, then mutate one permitted field. The
			// current-field integrity check must continue to detect that change.
			if err := tx.BindRawBytes(parsed, blob); err != nil {
				t.Fatalf("rebind canonical bytes: %v", err)
			}
			if test.mutateOperation {
				parsed.GetCommon().OperationLimit = issue1861Ptr(uint32(1))
			}
			if test.mutatePrevious {
				if parsed.GetCommon().PreviousTxnID == issue1861PreviousID {
					parsed.GetCommon().PreviousTxnID = issue1861ZeroHash
				} else {
					parsed.GetCommon().PreviousTxnID = issue1861PreviousID
				}
			}
			if test.mutateOperation || test.mutatePrevious {
				matches, err := tx.CurrentFieldsMatchRaw(parsed)
				if err != nil {
					t.Fatalf("compare mutated fields to raw bytes: %v", err)
				}
				if matches {
					t.Fatal("mutated fields still matched retained raw bytes")
				}
			}
		})
	}
}

func issue1861OperationLimitValue(value *uint32) uint32 {
	if value == nil {
		return 0
	}
	return *value
}

func TestIssue1861AMMBidAuthAccountsPresence(t *testing.T) {
	all.RegisterAll()

	authAccount := map[string]any{
		"AuthAccount": map[string]any{
			"Account": issue1861Destination,
		},
	}
	tests := []struct {
		name             string
		authAccounts     []map[string]any
		includeField     bool
		wantPresent      bool
		wantCount        int
		mutateAfterParse bool
	}{
		{name: "absent"},
		{name: "explicit empty", authAccounts: []map[string]any{}, includeField: true, wantPresent: true},
		{name: "nonempty", authAccounts: []map[string]any{authAccount}, includeField: true, wantPresent: true, wantCount: 1, mutateAfterParse: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields := issue1861BaseFields("AMMBid")
			fields["Asset"] = map[string]any{"currency": "XRP"}
			fields["Asset2"] = map[string]any{
				"currency": "USD",
				"issuer":   issue1861Destination,
			}
			if test.includeField {
				fields["AuthAccounts"] = test.authAccounts
			}

			blob := issue1861Encode(t, fields)
			parsed, err := tx.ParseFromBinary(blob)
			if err != nil {
				t.Fatalf("parse AMMBid: %v", err)
			}
			bid, ok := parsed.(*amm.AMMBid)
			if !ok {
				t.Fatalf("parsed transaction type = %T, want *amm.AMMBid", parsed)
			}
			if (bid.AuthAccounts != nil) != test.wantPresent {
				t.Fatalf("AuthAccounts nil = %v, want present = %v", bid.AuthAccounts == nil, test.wantPresent)
			}

			flat, err := bid.Flatten()
			if err != nil {
				t.Fatalf("flatten AMMBid: %v", err)
			}
			auth, present := flat["AuthAccounts"]
			if present != test.wantPresent {
				t.Fatalf("flattened AuthAccounts present = %v, want %v", present, test.wantPresent)
			}
			if present {
				entries, ok := auth.([]map[string]any)
				if !ok {
					t.Fatalf("flattened AuthAccounts type = %T, want []map[string]any", auth)
				}
				if len(entries) != test.wantCount {
					t.Fatalf("flattened AuthAccounts count = %d, want %d", len(entries), test.wantCount)
				}
			}
			tx.PopulateRequiredWireFields(flat, bid.GetCommon())
			if got := issue1861Encode(t, flat); !bytes.Equal(got, blob) {
				t.Fatalf("flattened AMMBid differs from canonical input:\n got %X\nwant %X", got, blob)
			}

			matches, err := tx.CurrentFieldsMatchRaw(bid)
			if err != nil || !matches {
				t.Fatalf("parsed AMMBid fields match = %v, err = %v", matches, err)
			}
			if test.mutateAfterParse {
				bid.AuthAccounts[0].AuthAccount.Account = issue1861Account
				matches, err := tx.CurrentFieldsMatchRaw(bid)
				if err != nil {
					t.Fatalf("compare mutated AMMBid: %v", err)
				}
				if matches {
					t.Fatal("mutated AuthAccounts still matched retained raw bytes")
				}
			}
		})
	}
}

func issue1861BatchFields(batchSigners []map[string]any, includeField bool) map[string]any {
	fields := issue1861BaseFields("Batch")
	fields["Flags"] = uint32(0x00010000)
	fields["RawTransactions"] = []map[string]any{
		{
			"RawTransaction": map[string]any{
				"TransactionType": "AccountSet",
				"Account":         issue1861Account,
				"Sequence":        uint32(2),
				"Fee":             "0",
				"Flags":           uint32(0x40000000),
				"SigningPubKey":   "",
			},
		},
		{
			"RawTransaction": map[string]any{
				"TransactionType": "AccountSet",
				"Account":         issue1861Account,
				"Sequence":        uint32(3),
				"Fee":             "0",
				"Flags":           uint32(0x40000000),
				"SigningPubKey":   "",
			},
		},
	}
	if includeField {
		fields["BatchSigners"] = batchSigners
	}
	return fields
}

func TestIssue1861BatchSignersPresence(t *testing.T) {
	all.RegisterAll()

	signer := []map[string]any{{
		"BatchSigner": map[string]any{
			"Account":       issue1861Destination,
			"SigningPubKey": "",
		},
	}}
	tests := []struct {
		name             string
		batchSigners     []map[string]any
		includeField     bool
		wantPresent      bool
		wantCount        int
		mutateAfterParse bool
	}{
		{name: "absent"},
		{name: "explicit empty", batchSigners: []map[string]any{}, includeField: true, wantPresent: true},
		{name: "nonempty", batchSigners: signer, includeField: true, wantPresent: true, wantCount: 1, mutateAfterParse: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			blob := issue1861Encode(t, issue1861BatchFields(test.batchSigners, test.includeField))
			parsed, err := tx.ParseFromBinary(blob)
			if err != nil {
				t.Fatalf("parse Batch: %v", err)
			}
			batchTx, ok := parsed.(*batch.Batch)
			if !ok {
				t.Fatalf("parsed transaction type = %T, want *batch.Batch", parsed)
			}
			if (batchTx.BatchSigners != nil) != test.wantPresent {
				t.Fatalf("BatchSigners nil = %v, want present = %v", batchTx.BatchSigners == nil, test.wantPresent)
			}

			flat, err := batchTx.Flatten()
			if err != nil {
				t.Fatalf("flatten Batch: %v", err)
			}
			signers, present := flat["BatchSigners"]
			if present != test.wantPresent {
				t.Fatalf("flattened BatchSigners present = %v, want %v", present, test.wantPresent)
			}
			if present {
				entries, ok := signers.([]map[string]any)
				if !ok {
					t.Fatalf("flattened BatchSigners type = %T, want []map[string]any", signers)
				}
				if len(entries) != test.wantCount {
					t.Fatalf("flattened BatchSigners count = %d, want %d", len(entries), test.wantCount)
				}
			}
			tx.PopulateRequiredWireFields(flat, batchTx.GetCommon())
			if got := issue1861Encode(t, flat); !bytes.Equal(got, blob) {
				t.Fatalf("flattened Batch differs from canonical input:\n got %X\nwant %X", got, blob)
			}

			matches, err := tx.CurrentFieldsMatchRaw(batchTx)
			if err != nil || !matches {
				t.Fatalf("parsed Batch fields match = %v, err = %v", matches, err)
			}
			if test.mutateAfterParse {
				batchTx.BatchSigners[0].BatchSigner.Account = issue1861Account
				matches, err := tx.CurrentFieldsMatchRaw(batchTx)
				if err != nil {
					t.Fatalf("compare mutated Batch: %v", err)
				}
				if matches {
					t.Fatal("mutated BatchSigners still matched retained raw bytes")
				}
			}
		})
	}
}
