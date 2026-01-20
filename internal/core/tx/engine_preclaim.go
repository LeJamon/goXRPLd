package tx

import (
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// preclaim validates the transaction against the current ledger state.
// This includes checking that the source account exists, has sufficient balance,
// and that the sequence number is correct.
func (e *Engine) preclaim(tx Transaction) Result {
	common := tx.GetCommon()

	// Check that the source account exists
	accountID, err := decodeAccountID(common.Account)
	if err != nil {
		return TemBAD_SRC_ACCOUNT
	}

	accountKey := keylet.Account(accountID)
	exists, err := e.view.Exists(accountKey)
	if err != nil {
		return TefINTERNAL
	}
	if !exists {
		return TerNO_ACCOUNT
	}

	// Read account data
	accountData, err := e.view.Read(accountKey)
	if err != nil {
		return TefINTERNAL
	}

	// Parse account and check sequence
	account, err := parseAccountRoot(accountData)
	if err != nil {
		return TefINTERNAL
	}

	// Check sequence number
	if common.Sequence != nil {
		if *common.Sequence < account.Sequence {
			return TefPAST_SEQ
		}
		if *common.Sequence > account.Sequence {
			return TerPRE_SEQ
		}
	}

	// Check that account can pay the fee
	fee := e.calculateFee(tx)
	if account.Balance < fee {
		return TerINSUF_FEE_B
	}

	// LastLedgerSequence check
	if common.LastLedgerSequence != nil {
		if e.config.LedgerSequence > *common.LastLedgerSequence {
			return TefMAX_LEDGER
		}
	}

	return TesSUCCESS
}
