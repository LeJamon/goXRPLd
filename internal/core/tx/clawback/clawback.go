package clawback

import (
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
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

	// Amount is required and must be positive
	if c.Amount.IsZero() {
		return ErrClawbackAmountRequired
	}

	// Amount must be positive (not negative or zero)
	if c.Amount.Signum() <= 0 {
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
// Reference: rippled Clawback.cpp preclaim() + applyHelper<Issue>()
func (c *Clawback) Apply(ctx *tx.ApplyContext) tx.Result {
	// --- Preclaim checks ---

	// 1. Decode holder from Amount.Issuer
	holderID, err := sle.DecodeAccountID(c.Amount.Issuer)
	if err != nil {
		return tx.TecNO_TARGET
	}

	// 2. Read holder's account — terNO_ACCOUNT if missing
	// Reference: rippled Clawback.cpp:206-208
	holderAccountKey := keylet.Account(holderID)
	holderAccountData, err := ctx.View.Read(holderAccountKey)
	if err != nil {
		return tx.TerNO_ACCOUNT
	}
	holderAccount, err := sle.ParseAccountRoot(holderAccountData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// 3. Check issuer flags (ctx.Account is the issuer)
	// Reference: rippled Clawback.cpp preclaimHelper<Issue>() lines 117-123
	// AllowTrustLineClawback must be set, NoFreeze must NOT be set
	if (ctx.Account.Flags & sle.LsfAllowTrustLineClawback) == 0 {
		return tx.TecNO_PERMISSION
	}
	if (ctx.Account.Flags & sle.LsfNoFreeze) != 0 {
		return tx.TecNO_PERMISSION
	}

	// 4. Read trust line
	// Reference: rippled Clawback.cpp:125-128
	trustKey := keylet.Line(holderID, ctx.AccountID, c.Amount.Currency)
	trustData, err := ctx.View.Read(trustKey)
	if err != nil {
		return tx.TecNO_LINE
	}
	rs, err := sle.ParseRippleState(trustData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// 5. Balance direction check
	// Reference: rippled Clawback.cpp:132-138
	// Balance is from LOW account's perspective:
	//   Positive: HIGH owes LOW (HIGH is the issuer)
	//   Negative: LOW owes HIGH (LOW is the issuer)
	// If balance > 0, issuer must be HIGH (issuer > holder)
	// If balance < 0, issuer must be LOW (issuer < holder)
	issuerIsLow := sle.CompareAccountIDs(ctx.AccountID, holderID) < 0
	if rs.Balance.Signum() > 0 && issuerIsLow {
		return tx.TecNO_PERMISSION
	}
	if rs.Balance.Signum() < 0 && !issuerIsLow {
		return tx.TecNO_PERMISSION
	}

	// 6. Check holder has funds (accountHolds equivalent, ignoring freeze)
	// Reference: rippled Clawback.cpp:149-156
	// Get balance from holder's perspective
	holderIsLow := !issuerIsLow
	var holderBalance sle.Amount
	if holderIsLow {
		holderBalance = rs.Balance
	} else {
		holderBalance = rs.Balance.Negate()
	}
	if holderBalance.Signum() <= 0 {
		return tx.TecINSUFFICIENT_FUNDS
	}

	// --- Apply ---
	// Reference: rippled Clawback.cpp applyHelper<Issue>() lines 230-259

	// 7. Compute actual claw amount = min(holderBalance, clawAmount)
	// Set the issuer field to the actual issuer (matching rippled line 239)
	clawAmount := c.Amount
	clawAmount.Issuer = ctx.Account.Account

	var actualAmount sle.Amount
	if holderBalance.Compare(clawAmount) < 0 {
		actualAmount = holderBalance
	} else {
		actualAmount = clawAmount
	}

	// 8. Transfer from holder to issuer (rippleCredit equivalent)
	// Reference: rippled View.cpp rippleCredit()
	// If holder is LOW: holder pays issuer (HIGH) → balance decreases
	// If holder is HIGH: holder pays issuer (LOW) → balance increases
	if holderIsLow {
		rs.Balance, _ = rs.Balance.Sub(actualAmount)
	} else {
		rs.Balance, _ = rs.Balance.Add(actualAmount)
	}

	// 9. Check if trust line should be deleted (default state)
	// Reference: rippled View.cpp rippleCredit() default state check
	// Same pattern as trustset.go lines 514-570
	var lowDefRipple, highDefRipple bool
	if issuerIsLow {
		lowDefRipple = (ctx.Account.Flags & sle.LsfDefaultRipple) != 0
		highDefRipple = (holderAccount.Flags & sle.LsfDefaultRipple) != 0
	} else {
		lowDefRipple = (holderAccount.Flags & sle.LsfDefaultRipple) != 0
		highDefRipple = (ctx.Account.Flags & sle.LsfDefaultRipple) != 0
	}

	bLowReserveSet := rs.LowQualityIn != 0 || rs.LowQualityOut != 0 ||
		((rs.Flags&sle.LsfLowNoRipple) == 0) != lowDefRipple ||
		(rs.Flags&sle.LsfLowFreeze) != 0 || !rs.LowLimit.IsZero() ||
		rs.Balance.Signum() > 0

	bHighReserveSet := rs.HighQualityIn != 0 || rs.HighQualityOut != 0 ||
		((rs.Flags&sle.LsfHighNoRipple) == 0) != highDefRipple ||
		(rs.Flags&sle.LsfHighFreeze) != 0 || !rs.HighLimit.IsZero() ||
		rs.Balance.Signum() < 0

	bDefault := !bLowReserveSet && !bHighReserveSet

	if bDefault && rs.Balance.IsZero() {
		// Remove from both owner directories before erasing
		var lowAccountID, highAccountID [20]byte
		if issuerIsLow {
			lowAccountID = ctx.AccountID
			highAccountID = holderID
		} else {
			lowAccountID = holderID
			highAccountID = ctx.AccountID
		}
		lowDirKey := keylet.OwnerDir(lowAccountID)
		sle.DirRemove(ctx.View, lowDirKey, rs.LowNode, trustKey.Key, false)
		highDirKey := keylet.OwnerDir(highAccountID)
		sle.DirRemove(ctx.View, highDirKey, rs.HighNode, trustKey.Key, false)

		// Delete the trust line
		if err := ctx.View.Erase(trustKey); err != nil {
			return tx.TefINTERNAL
		}

		// Decrement OwnerCount for both sides
		if ctx.Account.OwnerCount > 0 {
			ctx.Account.OwnerCount--
		}
		if holderAccount.OwnerCount > 0 {
			holderAccount.OwnerCount--
		}

		// Write holder account back to ledger
		holderUpdatedData, serErr := sle.SerializeAccountRoot(holderAccount)
		if serErr != nil {
			return tx.TefINTERNAL
		}
		if err := ctx.View.Update(holderAccountKey, holderUpdatedData); err != nil {
			return tx.TefINTERNAL
		}
	} else {
		// Update reserve flags
		if bLowReserveSet && (rs.Flags&sle.LsfLowReserve) == 0 {
			rs.Flags |= sle.LsfLowReserve
		} else if !bLowReserveSet && (rs.Flags&sle.LsfLowReserve) != 0 {
			rs.Flags &^= sle.LsfLowReserve
		}
		if bHighReserveSet && (rs.Flags&sle.LsfHighReserve) == 0 {
			rs.Flags |= sle.LsfHighReserve
		} else if !bHighReserveSet && (rs.Flags&sle.LsfHighReserve) != 0 {
			rs.Flags &^= sle.LsfHighReserve
		}

		// Serialize and update trust line
		updatedData, serErr := sle.SerializeRippleState(rs)
		if serErr != nil {
			return tx.TefINTERNAL
		}
		if err := ctx.View.Update(trustKey, updatedData); err != nil {
			return tx.TefINTERNAL
		}
	}

	return tx.TesSUCCESS
}

// Flatten returns a flat map of all transaction fields
func (c *Clawback) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(c)
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *Clawback) RequiredAmendments() [][32]byte {
	// MPToken clawback requires additional amendment
	if c.Holder != "" {
		return [][32]byte{amendment.FeatureClawback, amendment.FeatureMPTokensV1}
	}
	return [][32]byte{amendment.FeatureClawback}
}
