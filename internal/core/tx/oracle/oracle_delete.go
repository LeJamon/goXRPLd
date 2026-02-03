// TODO split between the two tx in respective files
package oracle

import (
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
)

func init() {
	tx.Register(tx.TypeOracleDelete, func() tx.Transaction {
		return &OracleDelete{BaseTx: *tx.NewBaseTx(tx.TypeOracleDelete, "")}
	})
}

// OracleDelete deletes a price oracle.
type OracleDelete struct {
	tx.BaseTx

	// OracleDocumentID identifies the oracle to delete (required)
	OracleDocumentID uint32 `json:"OracleDocumentID" xrpl:"OracleDocumentID"`
}

// NewOracleDelete creates a new OracleDelete transaction
func NewOracleDelete(account string, oracleDocID uint32) *OracleDelete {
	return &OracleDelete{
		BaseTx:           *tx.NewBaseTx(tx.TypeOracleDelete, account),
		OracleDocumentID: oracleDocID,
	}
}

// TxType returns the transaction type
func (o *OracleDelete) TxType() tx.Type {
	return tx.TypeOracleDelete
}

// Validate validates the OracleDelete transaction (preflight validation)
// This matches rippled's DeleteOracle::preflight()
func (o *OracleDelete) Validate() error {
	if err := o.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags - only universal flags allowed
	if o.Flags != nil && *o.Flags&tx.TfUniversalMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags set")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (o *OracleDelete) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(o)
}

// RequiredAmendments returns the amendments required for this transaction type
func (o *OracleDelete) RequiredAmendments() []string {
	return []string{amendment.AmendmentPriceOracle}
}

// Apply applies an OracleDelete transaction to the ledger state.
// Reference: rippled DeleteOracle.cpp DeleteOracle::doApply
func (o *OracleDelete) Apply(ctx *tx.ApplyContext) tx.Result {
	if ctx.Account.OwnerCount > 0 {
		ctx.Account.OwnerCount--
	}
	return tx.TesSUCCESS
}
