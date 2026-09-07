package replaytool

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/LeJamon/go-xrpl/internal/tx"
	"github.com/LeJamon/go-xrpl/internal/tx/all"
	"github.com/LeJamon/go-xrpl/internal/tx/sign"
	"github.com/LeJamon/go-xrpl/internal/tx/ter"
	fixture "github.com/LeJamon/go-xrpl/internal/tx/testdata"
	"github.com/LeJamon/go-xrpl/protocol"
	"github.com/stretchr/testify/require"
)

func TestCanonicalReplayPreparationRetainsHistoricalOperationLimitTransaction(t *testing.T) {
	all.RegisterAll()
	meta, err := tx.SerializeMetadata(&tx.Metadata{TransactionIndex: 0, TransactionResult: ter.TesSUCCESS})
	require.NoError(t, err)

	entries, expected, err := validateFixtureTransactions(
		context.Background(),
		[]fixtureTxEntry{{
			Index:  0,
			Hash:   fixture.Issue1861HistoricalTxIDHex,
			TxBlob: fixture.Issue1861HistoricalTxBlobHex,
		}},
		[]expectedTxEntry{{
			Index: 0,
			Hash:  fixture.Issue1861HistoricalTxIDHex,
			Meta:  hex.EncodeToString(meta),
		}},
	)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Len(t, expected, 1)
	require.NoError(t, sign.VerifySignature(entries[0].Transaction, true))
	expectedHash, err := protocol.Hash256FromHex(fixture.Issue1861HistoricalTxIDHex)
	require.NoError(t, err)
	require.Equal(t, expectedHash, expected[0].Hash)
	require.True(t, strings.EqualFold(fixture.Issue1861HistoricalTxBlobHex, hex.EncodeToString(entries[0].Blob)))
	require.Equal(t, entries[0].Blob, entries[0].Transaction.GetRawBytes())
}
