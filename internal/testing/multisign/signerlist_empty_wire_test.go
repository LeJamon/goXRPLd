package multisign_test

import (
	"testing"

	"github.com/LeJamon/go-xrpl/codec/binarycodec"
	jtx "github.com/LeJamon/go-xrpl/internal/testing"
	"github.com/LeJamon/go-xrpl/internal/tx"
	"github.com/LeJamon/go-xrpl/keylet"
	"github.com/stretchr/testify/require"
)

func TestSignerListSetWireEntryPresence(t *testing.T) {
	for _, test := range []struct {
		name    string
		quorum  uint32
		present bool
	}{
		{name: "delete absent"},
		{name: "delete empty", present: true},
		{name: "set absent", quorum: 1},
		{name: "set empty", quorum: 1, present: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := jtx.NewTestEnv(t)
			alice, bob := jtx.NewAccount("alice"), jtx.NewAccount("bob")
			env.Fund(alice)
			env.SetSignerList(alice, 1, []jtx.TestSigner{{Account: bob, Weight: 1}})
			env.Close()
			key := keylet.SignerList(alice.ID)
			before, err := env.LedgerEntry(key)
			require.NoError(t, err)
			require.NotEmpty(t, before)

			fields := map[string]any{
				"TransactionType": "SignerListSet",
				"Account":         alice.Address,
				"Sequence":        env.Seq(alice),
				"Fee":             "10",
				"SigningPubKey":   "",
				"SignerQuorum":    test.quorum,
			}
			if test.present {
				fields["SignerEntries"] = []map[string]any{}
			}
			blob, err := binarycodec.EncodeBytes(fields)
			require.NoError(t, err)
			parsed, err := tx.ParseFromBinary(blob)
			require.NoError(t, err)
			parsed.SetRawBytes(nil)
			rebuilt, err := tx.SerializeTransaction(parsed)
			require.NoError(t, err)
			require.Equal(t, blob, rebuilt)

			result := env.Submit(parsed)
			if test.quorum == 0 && !test.present {
				jtx.RequireTxSuccess(t, result)
				after, _ := env.LedgerEntry(key)
				require.Empty(t, after)
			} else {
				jtx.RequireTxFail(t, result, "temMALFORMED")
				after, err := env.LedgerEntry(key)
				require.NoError(t, err)
				require.Equal(t, before, after)
			}
		})
	}
}
