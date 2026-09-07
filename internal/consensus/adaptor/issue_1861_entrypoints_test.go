package adaptor

import (
	"encoding/hex"
	"testing"

	"github.com/LeJamon/go-xrpl/internal/consensus"
	"github.com/LeJamon/go-xrpl/internal/tx/all"
	txengine "github.com/LeJamon/go-xrpl/internal/tx/engine"
	fixture "github.com/LeJamon/go-xrpl/internal/tx/testdata"
	"github.com/LeJamon/go-xrpl/protocol"
	"github.com/stretchr/testify/require"
)

func TestBuildTxSetRetainsHistoricalOperationLimitTransaction(t *testing.T) {
	all.RegisterAll()
	raw, err := hex.DecodeString(fixture.Issue1861HistoricalTxBlobHex)
	require.NoError(t, err)

	adaptor := &Adaptor{txSetCache: newTxSetCache()}
	set, err := adaptor.BuildTxSet([][]byte{raw})
	require.NoError(t, err)
	require.Equal(t, 1, set.Size())
	require.Equal(t, [][]byte{raw}, set.Txs())

	expectedID, err := protocol.Hash256FromHex(fixture.Issue1861HistoricalTxIDHex)
	require.NoError(t, err)
	require.Equal(t, []consensus.TxID{consensus.TxID(expectedID)}, set.TxIDs())

	t.Run("canonical parser accepts the blob before consensus use", func(t *testing.T) {
		prepared, err := txengine.ParseAndPrepare(raw)
		require.NoError(t, err)
		require.Equal(t, raw, prepared.RawBlob)
	})
}
