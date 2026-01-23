package tx

import "errors"

func init() {
	Register(TypeAccountSet, func() Transaction {
		return &AccountSet{BaseTx: *NewBaseTx(TypeAccountSet, "")}
	})
}

// AccountSet modifies the properties of an account in the XRP Ledger.
type AccountSet struct {
	BaseTx

	// ClearFlag is a flag to disable for this account
	ClearFlag *uint32 `json:"ClearFlag,omitempty" xrpl:"ClearFlag,omitempty"`

	// Domain is the domain associated with this account (hex-encoded)
	Domain string `json:"Domain,omitempty" xrpl:"Domain,omitempty"`

	// EmailHash is MD5 hash of email for Gravatar (deprecated)
	EmailHash string `json:"EmailHash,omitempty" xrpl:"EmailHash,omitempty"`

	// MessageKey is a public key for sending encrypted messages
	MessageKey string `json:"MessageKey,omitempty" xrpl:"MessageKey,omitempty"`

	// NFTokenMinter is the account allowed to mint NFTokens for this account
	NFTokenMinter string `json:"NFTokenMinter,omitempty" xrpl:"NFTokenMinter,omitempty"`

	// SetFlag is a flag to enable for this account
	SetFlag *uint32 `json:"SetFlag,omitempty" xrpl:"SetFlag,omitempty"`

	// TransferRate is the fee for transferring issued currencies (1e9 = 100%)
	TransferRate *uint32 `json:"TransferRate,omitempty" xrpl:"TransferRate,omitempty"`

	// TickSize is the tick size for offers involving this account's currencies
	TickSize *uint8 `json:"TickSize,omitempty" xrpl:"TickSize,omitempty"`

	// WalletLocator is arbitrary hex data (deprecated)
	WalletLocator string `json:"WalletLocator,omitempty" xrpl:"WalletLocator,omitempty"`

	// WalletSize is arbitrary data (deprecated)
	WalletSize *uint32 `json:"WalletSize,omitempty" xrpl:"WalletSize,omitempty"`
}

// Common transaction flags
const (
	// TxFlagFullyCanonicalSig indicates the signature is fully canonical
	TxFlagFullyCanonicalSig uint32 = 0x80000000
)

// AccountSet transaction flags (legacy)
// Reference: rippled SetAccount.cpp
const (
	// tfRequireDestTag requires destination tag
	AccountSetTxFlagRequireDestTag uint32 = 0x00010000
	// tfOptionalDestTag makes destination tag optional
	AccountSetTxFlagOptionalDestTag uint32 = 0x00020000
	// tfRequireAuth requires authorization
	AccountSetTxFlagRequireAuth uint32 = 0x00040000
	// tfOptionalAuth makes authorization optional
	AccountSetTxFlagOptionalAuth uint32 = 0x00080000
	// tfDisallowXRP disallows XRP payments
	AccountSetTxFlagDisallowXRP uint32 = 0x00100000
	// tfAllowXRP allows XRP payments
	AccountSetTxFlagAllowXRP uint32 = 0x00200000

	// tfAccountSetMask is the mask for valid AccountSet transaction flags
	AccountSetTxFlagMask uint32 = ^(AccountSetTxFlagRequireDestTag |
		AccountSetTxFlagOptionalDestTag |
		AccountSetTxFlagRequireAuth |
		AccountSetTxFlagOptionalAuth |
		AccountSetTxFlagDisallowXRP |
		AccountSetTxFlagAllowXRP |
		TxFlagFullyCanonicalSig)
)

// Domain length limits
const (
	// MaxDomainLength is the maximum length of a domain in bytes
	MaxDomainLength = 256
)

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
// Reference: rippled SetAccount.cpp preflight()
func (a *AccountSet) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	txFlags := a.GetFlags()

	// Check for invalid transaction flags
	// Reference: rippled SetAccount.cpp:71-75
	if txFlags&AccountSetTxFlagMask != 0 {
		return errors.New("temINVALID_FLAG: invalid transaction flags")
	}

	// Cannot set and clear the same flag
	// Reference: rippled SetAccount.cpp:80-84
	if a.SetFlag != nil && a.ClearFlag != nil && *a.SetFlag == *a.ClearFlag {
		return errors.New("temINVALID_FLAG: cannot set and clear the same flag")
	}

	// Check for contradictory RequireAuth flags
	// Reference: rippled SetAccount.cpp:89-98
	setRequireAuth := (txFlags&AccountSetTxFlagRequireAuth != 0) ||
		(a.SetFlag != nil && *a.SetFlag == AccountSetFlagRequireAuth)
	clearRequireAuth := (txFlags&AccountSetTxFlagOptionalAuth != 0) ||
		(a.ClearFlag != nil && *a.ClearFlag == AccountSetFlagRequireAuth)
	if setRequireAuth && clearRequireAuth {
		return errors.New("temINVALID_FLAG: contradictory RequireAuth flags")
	}

	// Check for contradictory RequireDest flags
	// Reference: rippled SetAccount.cpp:103-112
	setRequireDest := (txFlags&AccountSetTxFlagRequireDestTag != 0) ||
		(a.SetFlag != nil && *a.SetFlag == AccountSetFlagRequireDest)
	clearRequireDest := (txFlags&AccountSetTxFlagOptionalDestTag != 0) ||
		(a.ClearFlag != nil && *a.ClearFlag == AccountSetFlagRequireDest)
	if setRequireDest && clearRequireDest {
		return errors.New("temINVALID_FLAG: contradictory RequireDest flags")
	}

	// Check for contradictory DisallowXRP flags
	// Reference: rippled SetAccount.cpp:117-126
	setDisallowXRP := (txFlags&AccountSetTxFlagDisallowXRP != 0) ||
		(a.SetFlag != nil && *a.SetFlag == AccountSetFlagDisallowXRP)
	clearDisallowXRP := (txFlags&AccountSetTxFlagAllowXRP != 0) ||
		(a.ClearFlag != nil && *a.ClearFlag == AccountSetFlagDisallowXRP)
	if setDisallowXRP && clearDisallowXRP {
		return errors.New("temINVALID_FLAG: contradictory DisallowXRP flags")
	}

	// TransferRate validation
	// Reference: rippled SetAccount.cpp:129-146
	if a.TransferRate != nil {
		tr := *a.TransferRate
		if tr != 0 && tr < 1000000000 {
			return errors.New("temBAD_TRANSFER_RATE: transfer rate too small")
		}
		if tr > 2000000000 {
			return errors.New("temBAD_TRANSFER_RATE: transfer rate too large")
		}
	}

	// TickSize validation
	// Reference: rippled SetAccount.cpp:149-159
	if a.TickSize != nil {
		ts := *a.TickSize
		if ts != 0 && (ts < 3 || ts > 15) {
			return errors.New("temBAD_TICK_SIZE: tick size must be 0 or 3-15")
		}
	}

	// Domain length validation
	// Reference: rippled SetAccount.cpp:170-175
	// Domain is stored as hex, so max hex length is 2*256 = 512
	if len(a.Domain) > MaxDomainLength*2 {
		return errors.New("telBAD_DOMAIN: domain too long")
	}

	// NFTokenMinter validation
	// Reference: rippled SetAccount.cpp:177-187
	if a.SetFlag != nil && *a.SetFlag == AccountSetFlagAuthorizedNFTokenMinter {
		if a.NFTokenMinter == "" {
			return errors.New("temMALFORMED: NFTokenMinter required when setting asfAuthorizedNFTokenMinter")
		}
	}
	if a.ClearFlag != nil && *a.ClearFlag == AccountSetFlagAuthorizedNFTokenMinter {
		if a.NFTokenMinter != "" {
			return errors.New("temMALFORMED: NFTokenMinter must be empty when clearing asfAuthorizedNFTokenMinter")
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AccountSet) Flatten() (map[string]any, error) {
	return ReflectFlatten(a)
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

// Apply applies the AccountSet transaction to ledger state.
func (a *AccountSet) Apply(ctx *ApplyContext) Result {
	account := ctx.Account
	uFlagsIn := account.Flags
	uFlagsOut := uFlagsIn

	var uSetFlag, uClearFlag uint32
	if a.SetFlag != nil {
		uSetFlag = *a.SetFlag
	}
	if a.ClearFlag != nil {
		uClearFlag = *a.ClearFlag
	}

	// RequireAuth
	if uSetFlag == AccountSetFlagRequireAuth && (uFlagsIn&lsfRequireAuth) == 0 {
		uFlagsOut |= lsfRequireAuth
	}
	if uClearFlag == AccountSetFlagRequireAuth && (uFlagsIn&lsfRequireAuth) != 0 {
		uFlagsOut &^= lsfRequireAuth
	}

	// RequireDestTag
	if uSetFlag == AccountSetFlagRequireDest && (uFlagsIn&lsfRequireDestTag) == 0 {
		uFlagsOut |= lsfRequireDestTag
	}
	if uClearFlag == AccountSetFlagRequireDest && (uFlagsIn&lsfRequireDestTag) != 0 {
		uFlagsOut &^= lsfRequireDestTag
	}

	// DisallowXRP
	if uSetFlag == AccountSetFlagDisallowXRP && (uFlagsIn&lsfDisallowXRP) == 0 {
		uFlagsOut |= lsfDisallowXRP
	}
	if uClearFlag == AccountSetFlagDisallowXRP && (uFlagsIn&lsfDisallowXRP) != 0 {
		uFlagsOut &^= lsfDisallowXRP
	}

	// DisableMaster
	if uSetFlag == AccountSetFlagDisableMaster && (uFlagsIn&lsfDisableMaster) == 0 {
		if account.RegularKey == "" {
			return TecNO_ALTERNATIVE_KEY
		}
		uFlagsOut |= lsfDisableMaster
	}
	if uClearFlag == AccountSetFlagDisableMaster && (uFlagsIn&lsfDisableMaster) != 0 {
		uFlagsOut &^= lsfDisableMaster
	}

	// DefaultRipple
	if uSetFlag == AccountSetFlagDefaultRipple {
		uFlagsOut |= lsfDefaultRipple
	} else if uClearFlag == AccountSetFlagDefaultRipple {
		uFlagsOut &^= lsfDefaultRipple
	}

	// NoFreeze (cannot be cleared once set)
	if uSetFlag == AccountSetFlagNoFreeze {
		uFlagsOut |= lsfNoFreeze
	}

	// GlobalFreeze
	if uSetFlag == AccountSetFlagGlobalFreeze {
		uFlagsOut |= lsfGlobalFreeze
	}
	if uSetFlag != AccountSetFlagGlobalFreeze && uClearFlag == AccountSetFlagGlobalFreeze {
		if (uFlagsOut & lsfNoFreeze) == 0 {
			uFlagsOut &^= lsfGlobalFreeze
		}
	}

	// AccountTxnID
	if uSetFlag == AccountSetFlagAccountTxnID {
		var zeroHash [32]byte
		if account.AccountTxnID == zeroHash {
			account.AccountTxnID = ctx.TxHash
		}
	}
	if uClearFlag == AccountSetFlagAccountTxnID {
		account.AccountTxnID = [32]byte{}
	}

	// DepositAuth
	if uSetFlag == AccountSetFlagDepositAuth {
		uFlagsOut |= lsfDepositAuth
	} else if uClearFlag == AccountSetFlagDepositAuth {
		uFlagsOut &^= lsfDepositAuth
	}

	// AuthorizedNFTokenMinter
	if uSetFlag == AccountSetFlagAuthorizedNFTokenMinter {
		if a.NFTokenMinter != "" {
			account.NFTokenMinter = a.NFTokenMinter
		}
	}
	if uClearFlag == AccountSetFlagAuthorizedNFTokenMinter {
		account.NFTokenMinter = ""
	}

	// Disallow Incoming flags
	if uSetFlag == AccountSetFlagDisallowIncomingNFTokenOffer {
		uFlagsOut |= lsfDisallowIncomingNFTokenOffer
	} else if uClearFlag == AccountSetFlagDisallowIncomingNFTokenOffer {
		uFlagsOut &^= lsfDisallowIncomingNFTokenOffer
	}

	if uSetFlag == AccountSetFlagDisallowIncomingCheck {
		uFlagsOut |= lsfDisallowIncomingCheck
	} else if uClearFlag == AccountSetFlagDisallowIncomingCheck {
		uFlagsOut &^= lsfDisallowIncomingCheck
	}

	if uSetFlag == AccountSetFlagDisallowIncomingPayChan {
		uFlagsOut |= lsfDisallowIncomingPayChan
	} else if uClearFlag == AccountSetFlagDisallowIncomingPayChan {
		uFlagsOut &^= lsfDisallowIncomingPayChan
	}

	if uSetFlag == AccountSetFlagDisallowIncomingTrustline {
		uFlagsOut |= lsfDisallowIncomingTrustline
	} else if uClearFlag == AccountSetFlagDisallowIncomingTrustline {
		uFlagsOut &^= lsfDisallowIncomingTrustline
	}

	// AllowTrustLineClawback (cannot be cleared once set)
	if uSetFlag == AccountSetFlagAllowTrustLineClawback {
		uFlagsOut |= lsfAllowTrustLineClawback
	}

	// Domain
	if a.Domain != "" {
		account.Domain = a.Domain
	}

	// EmailHash
	if a.EmailHash != "" {
		if a.EmailHash == "00000000000000000000000000000000" {
			account.EmailHash = ""
		} else {
			account.EmailHash = a.EmailHash
		}
	}

	// MessageKey
	if a.MessageKey != "" {
		account.MessageKey = a.MessageKey
	}

	// WalletLocator
	if a.WalletLocator != "" {
		if isZeroHash256(a.WalletLocator) {
			account.WalletLocator = ""
		} else {
			account.WalletLocator = a.WalletLocator
		}
	}

	// TransferRate
	if a.TransferRate != nil {
		rate := *a.TransferRate
		if rate != 0 && rate < qualityOne {
			return TemBAD_TRANSFER_RATE
		}
		if rate > 2*qualityOne {
			return TemBAD_TRANSFER_RATE
		}
		if rate == 0 || rate == qualityOne {
			account.TransferRate = 0
		} else {
			account.TransferRate = rate
		}
	}

	// TickSize
	if a.TickSize != nil {
		tickSize := *a.TickSize
		if tickSize == 0 || tickSize == 15 {
			account.TickSize = 0
		} else {
			account.TickSize = tickSize
		}
	}

	// Update flags if changed
	if uFlagsIn != uFlagsOut {
		account.Flags = uFlagsOut
	}

	return TesSUCCESS
}
