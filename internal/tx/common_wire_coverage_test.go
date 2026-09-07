package tx_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LeJamon/go-xrpl/codec/binarycodec"
	"github.com/LeJamon/go-xrpl/internal/tx"
	"github.com/LeJamon/go-xrpl/internal/tx/all"
	"github.com/stretchr/testify/require"
)

func TestCommonTemplateFieldsSurviveRegisteredTypedProjection(t *testing.T) {
	all.RegisterAll()
	const account = "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
	signature := map[string]any{"SigningPubKey": "ED" + strings.Repeat("01", 32), "TxnSignature": "AB"}
	samples := map[string][]any{
		"TransactionType":    {"AccountSet"},
		"Account":            {account},
		"Fee":                {"0", "10"},
		"Sequence":           {uint32(0), uint32(1)},
		"Flags":              {uint32(0), uint32(1)},
		"SourceTag":          {uint32(0), uint32(1)},
		"LastLedgerSequence": {uint32(0), uint32(1)},
		"OperationLimit":     {uint32(0), ^uint32(0)},
		"TicketSequence":     {uint32(0), uint32(1)},
		"NetworkID":          {uint32(0), uint32(1)},
		"SponsorFlags":       {uint32(0), uint32(1)},
		"PreviousTxnID":      {strings.Repeat("00", 32), strings.Repeat("01", 32)},
		"AccountTxnID":       {strings.Repeat("00", 32), strings.Repeat("01", 32)},
		"SigningPubKey":      {"", signature["SigningPubKey"]},
		"TxnSignature":       {"", "AB"},
		"Delegate":           {account},
		"Sponsor":            {account},
		"Memos":              {[]any{}, []any{map[string]any{"Memo": map[string]any{"MemoData": "AB"}}}},
		"Signers":            {[]any{}, []any{map[string]any{"Signer": map[string]any{"Account": account, "SigningPubKey": signature["SigningPubKey"], "TxnSignature": "AB"}}}},
		"SponsorSignature":   {signature},
	}
	commonFields := tx.FormatCommonFields()
	require.Len(t, samples, len(commonFields), "every common template field needs round-trip samples")
	for _, field := range commonFields {
		require.NotEmpty(t, samples[field.Name], "missing common template samples for %s", field.Name)
	}

	for _, txType := range tx.SupportedTypes() {
		t.Run(txType.String(), func(t *testing.T) {
			for _, field := range commonFields {
				t.Run(field.Name, func(t *testing.T) {
					// Project only common fields: these probes test representation,
					// independently of each type's required fields and preflight rules.
					for _, sample := range append([]any{nil}, samples[field.Name]...) {
						fields := map[string]any{"TransactionType": txType.String(), "Account": account, "Sequence": uint32(1), "Fee": "10", "SigningPubKey": ""}
						if sample != nil && field.Name != "TransactionType" {
							fields[field.Name] = sample
						}
						original, err := binarycodec.EncodeBytes(fields)
						require.NoError(t, err)
						decoded, err := binarycodec.DecodeBytes(original)
						require.NoError(t, err)
						data, err := json.Marshal(decoded)
						require.NoError(t, err)
						transaction, err := tx.NewFromType(txType)
						require.NoError(t, err)
						require.NoError(t, json.Unmarshal(data, transaction))
						present := make(map[string]bool, len(decoded))
						for name := range decoded {
							present[name] = true
						}
						transaction.GetCommon().SetPresentFields(present)
						flat, err := transaction.Flatten()
						require.NoError(t, err)
						tx.PopulateRequiredWireFields(flat, transaction.GetCommon())
						projection := make(map[string]any)
						for _, common := range commonFields {
							if value, ok := flat[common.Name]; ok {
								projection[common.Name] = value
							}
						}
						rebuilt, err := binarycodec.EncodeBytes(projection)
						require.NoError(t, err)
						require.Equal(t, original, rebuilt, "sample %#v", sample)
					}
				})
			}
		})
	}
}
