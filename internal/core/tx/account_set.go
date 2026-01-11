package tx

// AccountSet modifies the properties of an account in the XRP Ledger.
type AccountSet struct {
	BaseTx

	// ClearFlag is a flag to disable for this account
	ClearFlag *uint32 `json:"ClearFlag,omitempty"`

	// Domain is the domain associated with this account (hex-encoded)
	Domain string `json:"Domain,omitempty"`

	// EmailHash is MD5 hash of email for Gravatar (deprecated)
	EmailHash string `json:"EmailHash,omitempty"`

	// MessageKey is a public key for sending encrypted messages
	MessageKey string `json:"MessageKey,omitempty"`

	// NFTokenMinter is the account allowed to mint NFTokens for this account
	NFTokenMinter string `json:"NFTokenMinter,omitempty"`

	// SetFlag is a flag to enable for this account
	SetFlag *uint32 `json:"SetFlag,omitempty"`

	// TransferRate is the fee for transferring issued currencies (1e9 = 100%)
	TransferRate *uint32 `json:"TransferRate,omitempty"`

	// TickSize is the tick size for offers involving this account's currencies
	TickSize *uint8 `json:"TickSize,omitempty"`

	// WalletLocator is arbitrary hex data (deprecated)
	WalletLocator string `json:"WalletLocator,omitempty"`

	// WalletSize is arbitrary data (deprecated)
	WalletSize *uint32 `json:"WalletSize,omitempty"`
}

// AccountSet account flags (for SetFlag/ClearFlag)
const (
	// asfRequireDest requires a destination tag
	AccountSetFlagRequireDest uint32 = 1
	// asfRequireAuth requires authorization for trust lines
	AccountSetFlagRequireAuth uint32 = 2
	// asfDisallowXRP disallows XRP payments to this account
	AccountSetFlagDisallowXRP uint32 = 3
	// asfDisableMaster disables the master key
	AccountSetFlagDisableMaster uint32 = 4
	// asfAccountTxnID enables AccountTxnID tracking
	AccountSetFlagAccountTxnID uint32 = 5
	// asfNoFreeze prevents freezing trust lines
	AccountSetFlagNoFreeze uint32 = 6
	// asfGlobalFreeze freezes all trust lines
	AccountSetFlagGlobalFreeze uint32 = 7
	// asfDefaultRipple enables rippling by default
	AccountSetFlagDefaultRipple uint32 = 8
	// asfDepositAuth requires deposit authorization
	AccountSetFlagDepositAuth uint32 = 9
	// asfAuthorizedNFTokenMinter enables NFToken minting
	AccountSetFlagAuthorizedNFTokenMinter uint32 = 10
	// asfDisallowIncomingNFTokenOffer disallows incoming NFT offers
	AccountSetFlagDisallowIncomingNFTokenOffer uint32 = 12
	// asfDisallowIncomingCheck disallows incoming checks
	AccountSetFlagDisallowIncomingCheck uint32 = 13
	// asfDisallowIncomingPayChan disallows incoming payment channels
	AccountSetFlagDisallowIncomingPayChan uint32 = 14
	// asfDisallowIncomingTrustline disallows incoming trust lines
	AccountSetFlagDisallowIncomingTrustline uint32 = 15
	// asfAllowTrustLineClawback enables clawback on trust lines
	AccountSetFlagAllowTrustLineClawback uint32 = 16
)

// NewAccountSet creates a new AccountSet transaction
func NewAccountSet(account string) *AccountSet {
	return &AccountSet{
		BaseTx: *NewBaseTx(TypeAccountSet, account),
	}
}

// TxType returns the transaction type
func (a *AccountSet) TxType() Type {
	return TypeAccountSet
}

// Validate validates the AccountSet transaction
func (a *AccountSet) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// TickSize must be 0, 3-15
	if a.TickSize != nil {
		ts := *a.TickSize
		if ts != 0 && (ts < 3 || ts > 15) {
			return ErrInvalidFlags
		}
	}

	// TransferRate must be 0 or 1e9-2e9
	if a.TransferRate != nil {
		tr := *a.TransferRate
		if tr != 0 && (tr < 1000000000 || tr > 2000000000) {
			return ErrInvalidFlags
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AccountSet) Flatten() (map[string]any, error) {
	m := a.Common.ToMap()

	if a.ClearFlag != nil {
		m["ClearFlag"] = *a.ClearFlag
	}
	if a.Domain != "" {
		m["Domain"] = a.Domain
	}
	if a.EmailHash != "" {
		m["EmailHash"] = a.EmailHash
	}
	if a.MessageKey != "" {
		m["MessageKey"] = a.MessageKey
	}
	if a.NFTokenMinter != "" {
		m["NFTokenMinter"] = a.NFTokenMinter
	}
	if a.SetFlag != nil {
		m["SetFlag"] = *a.SetFlag
	}
	if a.TransferRate != nil {
		m["TransferRate"] = *a.TransferRate
	}
	if a.TickSize != nil {
		m["TickSize"] = *a.TickSize
	}
	if a.WalletLocator != "" {
		m["WalletLocator"] = a.WalletLocator
	}
	if a.WalletSize != nil {
		m["WalletSize"] = *a.WalletSize
	}

	return m, nil
}

// EnableRequireDest enables the require destination tag flag
func (a *AccountSet) EnableRequireDest() {
	flag := AccountSetFlagRequireDest
	a.SetFlag = &flag
}

// EnableDepositAuth enables deposit authorization
func (a *AccountSet) EnableDepositAuth() {
	flag := AccountSetFlagDepositAuth
	a.SetFlag = &flag
}

// EnableDefaultRipple enables default rippling
func (a *AccountSet) EnableDefaultRipple() {
	flag := AccountSetFlagDefaultRipple
	a.SetFlag = &flag
}
