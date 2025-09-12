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

//func StoreLedger()
