package tx

import "errors"

// TrustSet creates or modifies a trust line between two accounts.
type TrustSet struct {
	BaseTx

	// LimitAmount defines the trust line (required)
	// The issuer field is the account to trust
	LimitAmount Amount `json:"LimitAmount"`

	// QualityIn is the quality in (1e9 = 1:1) - optional
	QualityIn *uint32 `json:"QualityIn,omitempty"`

	// QualityOut is the quality out (1e9 = 1:1) - optional
	QualityOut *uint32 `json:"QualityOut,omitempty"`
}

// TrustSet flags
const (
	// tfSetfAuth authorizes the other party to hold currency
	TrustSetFlagSetfAuth uint32 = 0x00010000
	// tfSetNoRipple blocks rippling on this trust line
	TrustSetFlagSetNoRipple uint32 = 0x00020000
	// tfClearNoRipple clears the no ripple flag
	TrustSetFlagClearNoRipple uint32 = 0x00040000
	// tfSetFreeze freezes the trust line
	TrustSetFlagSetFreeze uint32 = 0x00100000
	// tfClearFreeze clears the freeze flag
	TrustSetFlagClearFreeze uint32 = 0x00200000
)

// NewTrustSet creates a new TrustSet transaction
func NewTrustSet(account string, limitAmount Amount) *TrustSet {
	return &TrustSet{
		BaseTx:      *NewBaseTx(TypeTrustSet, account),
		LimitAmount: limitAmount,
	}
}

// TxType returns the transaction type
func (t *TrustSet) TxType() Type {
	return TypeTrustSet
}

// Validate validates the TrustSet transaction
func (t *TrustSet) Validate() error {
	if err := t.BaseTx.Validate(); err != nil {
		return err
	}

	// LimitAmount must be an issued currency, not XRP
	if t.LimitAmount.IsNative() {
		return errors.New("cannot create trust line for XRP")
	}

	if t.LimitAmount.Currency == "" {
		return errors.New("currency is required")
	}

	if t.LimitAmount.Issuer == "" {
		return errors.New("issuer is required")
	}

	// Cannot create trust line to self
	if t.LimitAmount.Issuer == t.Account {
		return errors.New("cannot create trust line to self")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (t *TrustSet) Flatten() (map[string]any, error) {
	m := t.Common.ToMap()

	m["LimitAmount"] = flattenAmount(t.LimitAmount)

	if t.QualityIn != nil {
		m["QualityIn"] = *t.QualityIn
	}
	if t.QualityOut != nil {
		m["QualityOut"] = *t.QualityOut
	}

	return m, nil
}

// SetNoRipple sets the no ripple flag on this trust line
func (t *TrustSet) SetNoRipple() {
	flags := t.GetFlags() | TrustSetFlagSetNoRipple
	t.SetFlags(flags)
}

// ClearNoRipple clears the no ripple flag on this trust line
func (t *TrustSet) ClearNoRipple() {
	flags := t.GetFlags() | TrustSetFlagClearNoRipple
	t.SetFlags(flags)
}

// SetFreeze freezes this trust line
func (t *TrustSet) SetFreeze() {
	flags := t.GetFlags() | TrustSetFlagSetFreeze
	t.SetFlags(flags)
}
