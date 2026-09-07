package adapter

import (
	"encoding/hex"
	"testing"

	"github.com/LeJamon/go-xrpl/internal/ledger/genesis"
	"github.com/LeJamon/go-xrpl/internal/ledger/service"
	"github.com/LeJamon/go-xrpl/internal/tx/all"
	"github.com/LeJamon/go-xrpl/internal/tx/ter"
	fixture "github.com/LeJamon/go-xrpl/internal/tx/testdata"
	"github.com/stretchr/testify/require"
)

func TestSubmitHistoricalOperationLimitBlobThroughAdapter(t *testing.T) {
	all.RegisterAll()

	svc, err := service.New(service.Config{Standalone: true, GenesisConfig: genesis.DefaultConfig()})
	require.NoError(t, err)
	require.NoError(t, svc.Start())
	t.Cleanup(svc.Stop)

	raw, err := hex.DecodeString(fixture.Issue1861HistoricalTxBlobHex)
	require.NoError(t, err)

	result, err := NewLedgerServiceAdapter(svc).SubmitTransaction(nil, hex.EncodeToString(raw))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, ter.TerNO_ACCOUNT.String(), result.EngineResult)
}
