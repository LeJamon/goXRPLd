package clawback

import (
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
	"strconv"
)

func init() {
	tx.Register(tx.TypeClawback, func() tx.Transaction {
		return &Clawback{BaseTx: *tx.NewBaseTx(tx.TypeClawback, "")}
	})
}

// Clawback flag mask
const (
	tfClawbackMask uint32 = 0xFFFFFFFF // All flags are invalid for Clawback
)

// parseAmountValue parses amount value string (handles both integer and decimal)
func parseAmountValue(value string) (float64, error) {
	return strconv.ParseFloat(value, 64)
}

// Clawback errors
var (
	ErrClawbackAmountRequired  = errors.New("temBAD_AMOUNT: Amount is required")
	ErrClawbackAmountNotToken  = errors.New("temBAD_AMOUNT: cannot claw back XRP")
	ErrClawbackAmountNotPos    = errors.New("temBAD_AMOUNT: Amount must be positive")
	ErrClawbackHolderWithToken = errors.New("temMALFORMED: Holder field cannot be present for token clawback")
	ErrClawbackHolderRequired  = errors.New("temMALFORMED: Holder is required for MPToken clawback")
	ErrClawbackHolderIsSelf    = errors.New("temMALFORMED: Holder cannot be the same as issuer")
)

// Clawback claws back tokens from a trust line or MPToken.
// Reference: rippled Clawback.cpp
type Clawback struct {
	tx.BaseTx

	// Amount is the amount to claw back (required)
	// For IOU clawback, the issuer field specifies the holder
	Amount sle.Amount `json:"Amount" xrpl:"Amount,amount"`

	// Holder is the MPToken holder (optional, for MPToken clawback only)
	Holder string `json:"Holder,omitempty" xrpl:"Holder,omitempty"`
}

// NewClawback creates a new Clawback transaction for IOU tokens
func NewClawback(account string, amount sle.Amount) *Clawback {
	return &Clawback{
		BaseTx: *tx.NewBaseTx(tx.TypeClawback, account),
		Amount: amount,
	}
}

// NewMPTokenClawback creates a new Clawback transaction for MPTokens
func NewMPTokenClawback(account, holder string, amount sle.Amount) *Clawback {
	return &Clawback{
		BaseTx: *tx.NewBaseTx(tx.TypeClawback, account),
		Amount: amount,
		Holder: holder,
	}
}

// TxType returns the transaction type
func (c *Clawback) TxType() tx.Type {
	return tx.TypeClawback
}

// Validate validates the Clawback transaction
// Reference: rippled Clawback.cpp preflight()
func (c *Clawback) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags
	// Reference: rippled Clawback.cpp:87-88
	if c.Common.Flags != nil && *c.Common.Flags&tx.TfUniversalMask != 0 {
		return tx.ErrInvalidFlags
	}

	// Amount is required
	if c.Amount.Value == "" {
		return ErrClawbackAmountRequired
	}

	// Amount must be positive
	amountVal, err := parseAmountValue(c.Amount.Value)
	if err != nil || amountVal <= 0 {
		return ErrClawbackAmountNotPos
	}

	// Cannot claw back XRP
	if c.Amount.IsNative() {
		return ErrClawbackAmountNotToken
	}

	// Determine if this is IOU or MPToken clawback based on Holder field
	// For IOU clawback, Holder must not be present
	// For MPToken clawback, Holder is required
	if c.Holder == "" {
		// IOU clawback
		// Reference: rippled Clawback.cpp:39-40
		// For IOU, the issuer field in Amount specifies the holder
		// The transaction account must be the issuer of the currency
		holder := c.Amount.Issuer

		// Issuer cannot claw back from themselves
		if holder == c.Account {
			return ErrClawbackAmountNotPos // temBAD_AMOUNT per rippled
		}
	} else {
		// MPToken clawback
		// Reference: rippled Clawback.cpp:54-76
		// Holder cannot be same as issuer
		if c.Holder == c.Account {
			return ErrClawbackHolderIsSelf
		}
	}

	return nil
}

// Apply applies the Clawback transaction to ledger state.
func (c *Clawback) Apply(ctx *tx.ApplyContext) tx.Result {
	if c.Amount.Value == "" {
		return tx.TemINVALID
	}

	holderID, err := sle.DecodeAccountID(c.Amount.Issuer)
	if err != nil {
		return tx.TecNO_TARGET
	}

	trustKey := keylet.Line(holderID, ctx.AccountID, c.Amount.Currency)

	trustData, err := ctx.View.Read(trustKey)
	if err != nil {
		return tx.TecNO_LINE
	}

	_, err = sle.ParseRippleState(trustData)
	if err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}

// Flatten returns a flat map of all transaction fields
func (c *Clawback) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(c)
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *Clawback) RequiredAmendments() []string {
	// MPToken clawback requires additional amendment
	if c.Holder != "" {
		return []string{amendment.AmendmentClawback, amendment.AmendmentMPTokensV1}
	}
	return []string{amendment.AmendmentClawback}
}
