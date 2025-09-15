package ledger

import (
	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/header"
	"github.com/LeJamon/goXRPLd/internal/core/shamap"
)

/**
TODO
Rules 	rules_
**/

type Ledger struct {
	Immutable bool
	TxMap     shamap.SHAMap
	StateMap  shamap.SHAMap
	Header    header.LedgerHeader
	Fees      XRPAmount.Fees
}

func (l *Ledger) Sequence() uint32 {
	return l.Header.LedgerIndex
}

func (l *Ledger) Hash() [32]byte {
	return l.Header.Hash
}

func (l *Ledger) StoreLedger() (bool, error) {
	return insertLedger(l, l.Header.Validated)
}

func insertLedger(l *Ledger, valid bool) (bool, error) {

}

func (l *Ledger) RetrieveLedger() (bool, error) {}
