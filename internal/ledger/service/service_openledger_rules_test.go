package service

import (
	"testing"
	"time"

	"github.com/LeJamon/go-xrpl/amendment"
	"github.com/LeJamon/go-xrpl/internal/ledger"
	"github.com/LeJamon/go-xrpl/internal/tx/pseudo"
	"github.com/LeJamon/go-xrpl/keylet"
	"github.com/stretchr/testify/require"
)

func TestOpenLedgerAcceptanceUsesLastValidatedRules(t *testing.T) {
	svc, err := New(DefaultConfig())
	require.NoError(t, err)
	require.NoError(t, svc.Start())
	t.Cleanup(svc.Stop)

	validated := svc.GetValidatedLedger()
	parent := svc.GetClosedLedger()
	require.NotNil(t, validated)
	require.NotNil(t, parent)
	require.False(t, validated.Rules().Enabled(amendment.FeatureBatchV1_1))

	localClosed, err := ledger.NewOpen(parent, parent.CloseTime().Add(time.Second))
	require.NoError(t, err)
	amendments, err := pseudo.SerializeAmendmentsSLE(&pseudo.AmendmentsSLE{
		Amendments: [][32]byte{amendment.FeatureBatchV1_1},
	})
	require.NoError(t, err)
	require.NoError(t, localClosed.Update(keylet.Amendments(), amendments))
	require.NoError(t, localClosed.Close(parent.CloseTime().Add(time.Second), 0))
	require.True(t, localClosed.Rules().Enabled(amendment.FeatureBatchV1_1))

	svc.mu.RLock()
	applyCfg, err := svc.applyConfigLocked()
	svc.mu.RUnlock()
	require.NoError(t, err)
	require.False(t, applyCfg.Rules.Enabled(amendment.FeatureBatchV1_1),
		"ingress must use the last validated rules, not a locally built closed ledger")
	require.False(t, svc.TransactionRules().Enabled(amendment.FeatureBatchV1_1),
		"TransactionRules must match the open-ledger admission rules")

	svc.openLedgerMu.Lock()
	svc.mu.Lock()
	accept := svc.openLedgerAcceptanceLocked(nil, nil)
	svc.mu.Unlock()
	err = accept(localClosed, nil, false, nil)
	svc.openLedgerMu.Unlock()
	require.NoError(t, err)

	current := svc.openLedgerView.Current()
	require.NotNil(t, current)
	require.False(t, current.Rules().Enabled(amendment.FeatureBatchV1_1),
		"the published open view must carry the same rules as its ApplyConfig")
	svc.mu.RLock()
	publishedCfg, err := svc.applyConfigLocked()
	svc.mu.RUnlock()
	require.NoError(t, err)
	require.Same(t, current.Rules(), publishedCfg.Rules,
		"ingress must reuse the published open view rule snapshot")
	require.Same(t, current.Rules(), svc.TransactionRules(),
		"TransactionRules must expose the published open view rule snapshot")
}
