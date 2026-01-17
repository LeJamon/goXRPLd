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

// TrustSet transaction flags
// Reference: rippled SetTrust.cpp
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
	// tfSetDeepFreeze deep freezes the trust line (requires featureDeepFreeze)
	TrustSetFlagSetDeepFreeze uint32 = 0x00400000
	// tfClearDeepFreeze clears the deep freeze flag
	TrustSetFlagClearDeepFreeze uint32 = 0x00800000

	// tfTrustSetMask is the mask for valid TrustSet transaction flags
	TrustSetFlagMask uint32 = ^(TrustSetFlagSetfAuth |
		TrustSetFlagSetNoRipple |
		TrustSetFlagClearNoRipple |
		TrustSetFlagSetFreeze |
		TrustSetFlagClearFreeze |
		TrustSetFlagSetDeepFreeze |
		TrustSetFlagClearDeepFreeze |
		TxFlagFullyCanonicalSig)
)

// QUALITY_ONE is the 1:1 quality ratio (1e9)
const QualityOne uint32 = 1000000000

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
// Reference: rippled SetTrust.cpp preflight()
func (t *TrustSet) Validate() error {
	if err := t.BaseTx.Validate(); err != nil {
		return err
	}

	txFlags := t.GetFlags()

	// Check for invalid transaction flags
	// Reference: rippled SetTrust.cpp:81-85
	if txFlags&TrustSetFlagMask != 0 {
		return errors.New("temINVALID_FLAG: invalid transaction flags")
	}

	// LimitAmount must be an issued currency, not XRP
	// Reference: rippled SetTrust.cpp:102-107
	if t.LimitAmount.IsNative() {
		return errors.New("temBAD_LIMIT: cannot create trust line for XRP")
	}

	if t.LimitAmount.Currency == "" {
		return errors.New("temBAD_CURRENCY: currency is required")
	}

	// Check for XRP currency code
	// Reference: rippled SetTrust.cpp:109-113
	if t.LimitAmount.Currency == "XRP" {
		return errors.New("temBAD_CURRENCY: cannot use XRP as IOU currency")
	}

	// Negative limit is not allowed
	// Reference: rippled SetTrust.cpp:115-119
	if len(t.LimitAmount.Value) > 0 && t.LimitAmount.Value[0] == '-' {
		return errors.New("temBAD_LIMIT: negative credit limit")
	}

	// Check if destination makes sense
	// Reference: rippled SetTrust.cpp:122-128
	if t.LimitAmount.Issuer == "" {
		return errors.New("temDST_NEEDED: issuer is required")
	}

	// Cannot create trust line to self
	// Reference: rippled SetTrust.cpp:220-224
	if t.LimitAmount.Issuer == t.Account {
		return errors.New("temDST_IS_SRC: cannot create trust line to self")
	}

	// Check for contradictory NoRipple flags
	setNoRipple := txFlags&TrustSetFlagSetNoRipple != 0
	clearNoRipple := txFlags&TrustSetFlagClearNoRipple != 0
	if setNoRipple && clearNoRipple {
		return errors.New("temINVALID_FLAG: cannot set and clear NoRipple")
	}

	// Check for contradictory Freeze flags
	// Reference: rippled SetTrust.cpp:326-332
	setFreeze := txFlags&TrustSetFlagSetFreeze != 0
	clearFreeze := txFlags&TrustSetFlagClearFreeze != 0
	setDeepFreeze := txFlags&TrustSetFlagSetDeepFreeze != 0
	clearDeepFreeze := txFlags&TrustSetFlagClearDeepFreeze != 0

	if (setFreeze || setDeepFreeze) && (clearFreeze || clearDeepFreeze) {
		return errors.New("temINVALID_FLAG: cannot set and clear freeze in same transaction")
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
