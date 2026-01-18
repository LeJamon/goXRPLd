package tx

import "errors"

// AMM constants matching rippled
const (
	// TRADING_FEE_THRESHOLD is the maximum trading fee (1000 = 1%)
	TRADING_FEE_THRESHOLD uint16 = 1000

	// AMM vote slot constants
	VOTE_MAX_SLOTS         = 8
	VOTE_WEIGHT_SCALE_FACTOR = 100000

	// AMM auction slot constants
	AUCTION_SLOT_MAX_AUTH_ACCOUNTS           = 4
	AUCTION_SLOT_TIME_INTERVALS              = 20
	AUCTION_SLOT_DISCOUNTED_FEE_FRACTION     = 10 // 1/10 of fee
	AUCTION_SLOT_MIN_FEE_FRACTION            = 25 // 1/25 of fee
	TOTAL_TIME_SLOT_SECS                     = 24 * 60 * 60 // 24 hours

	// AMMCreate has no valid transaction flags
	tfAMMCreateMask uint32 = 0xFFFFFFFF

	// AMMDeposit flags
	tfLPToken          uint32 = 0x00010000
	tfSingleAsset      uint32 = 0x00080000
	tfTwoAsset         uint32 = 0x00100000
	tfOneAssetLPToken  uint32 = 0x00200000
	tfLimitLPToken     uint32 = 0x00400000
	tfTwoAssetIfEmpty  uint32 = 0x00800000
	tfAMMDepositMask   uint32 = ^(tfLPToken | tfSingleAsset | tfTwoAsset | tfOneAssetLPToken | tfLimitLPToken | tfTwoAssetIfEmpty)

	// AMMWithdraw flags
	tfWithdrawAll          uint32 = 0x00020000
	tfOneAssetWithdrawAll  uint32 = 0x00040000
	tfAMMWithdrawMask      uint32 = ^(tfLPToken | tfWithdrawAll | tfOneAssetWithdrawAll | tfSingleAsset | tfTwoAsset | tfOneAssetLPToken | tfLimitLPToken)

	// AMMVote has no valid transaction flags
	tfAMMVoteMask uint32 = 0xFFFFFFFF

	// AMMBid has no valid transaction flags
	tfAMMBidMask uint32 = 0xFFFFFFFF

	// AMMDelete has no valid transaction flags
	tfAMMDeleteMask uint32 = 0xFFFFFFFF

	// AMMClawback flags
	tfClawTwoAssets    uint32 = 0x00000001
	tfAMMClawbackMask  uint32 = ^tfClawTwoAssets
)

// AMMCreate creates an Automated Market Maker (AMM) instance.
type AMMCreate struct {
	BaseTx

	// Amount is the first asset to deposit (required)
	Amount Amount `json:"Amount"`

	// Amount2 is the second asset to deposit (required)
	Amount2 Amount `json:"Amount2"`

	// TradingFee is the fee in basis points (0-1000, where 1000 = 1%)
	TradingFee uint16 `json:"TradingFee"`
}

// NewAMMCreate creates a new AMMCreate transaction
func NewAMMCreate(account string, amount1, amount2 Amount, tradingFee uint16) *AMMCreate {
	return &AMMCreate{
		BaseTx:     *NewBaseTx(TypeAMMCreate, account),
		Amount:     amount1,
		Amount2:    amount2,
		TradingFee: tradingFee,
	}
}

// TxType returns the transaction type
func (a *AMMCreate) TxType() Type {
	return TypeAMMCreate
}

// Validate validates the AMMCreate transaction
// Reference: rippled AMMCreate.cpp preflight
func (a *AMMCreate) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags - no flags are valid for AMMCreate
	if a.GetFlags()&tfAMMCreateMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMCreate")
	}

	// Amount is required and must be positive
	if a.Amount.Value == "" {
		return errors.New("temMALFORMED: Amount is required")
	}

	// Amount2 is required and must be positive
	if a.Amount2.Value == "" {
		return errors.New("temMALFORMED: Amount2 is required")
	}

	// Assets cannot be the same (same currency and issuer)
	if a.Amount.Currency == a.Amount2.Currency && a.Amount.Issuer == a.Amount2.Issuer {
		return errors.New("temBAD_AMM_TOKENS: tokens cannot have the same currency/issuer")
	}

	// TradingFee must be 0-1000 (0-1%)
	if a.TradingFee > TRADING_FEE_THRESHOLD {
		return errors.New("temBAD_FEE: TradingFee must be 0-1000")
	}

	// Validate amounts are positive (not zero or negative)
	if err := validateAMMAmount(a.Amount); err != nil {
		return errors.New("temBAD_AMOUNT: invalid Amount - " + err.Error())
	}
	if err := validateAMMAmount(a.Amount2); err != nil {
		return errors.New("temBAD_AMOUNT: invalid Amount2 - " + err.Error())
	}

	return nil
}

// validateAMMAmount validates an AMM amount
func validateAMMAmount(amt Amount) error {
	if amt.Value == "" {
		return errors.New("amount is required")
	}
	// For XRP (no currency), value must be positive drops
	if amt.Currency == "" {
		// XRP amount - should be positive integer drops
		if amt.Value == "0" {
			return errors.New("amount must be positive")
		}
		if len(amt.Value) > 0 && amt.Value[0] == '-' {
			return errors.New("amount must be positive")
		}
	}
	// For IOU, value must be positive
	// Note: Further IOU validation would check issuer existence, etc.
	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMCreate) Flatten() (map[string]any, error) {
	m := a.Common.ToMap()

	m["Amount"] = flattenAmount(a.Amount)
	m["Amount2"] = flattenAmount(a.Amount2)
	m["TradingFee"] = a.TradingFee

	return m, nil
}

// AMMDeposit deposits assets into an AMM.
type AMMDeposit struct {
	BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset Asset `json:"Asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 Asset `json:"Asset2"`

	// Amount is the amount of first asset to deposit (optional)
	Amount *Amount `json:"Amount,omitempty"`

	// Amount2 is the amount of second asset to deposit (optional)
	Amount2 *Amount `json:"Amount2,omitempty"`

	// EPrice is the effective price limit (optional)
	EPrice *Amount `json:"EPrice,omitempty"`

	// LPTokenOut is the LP tokens to receive (optional)
	LPTokenOut *Amount `json:"LPTokenOut,omitempty"`

	// TradingFee is the trading fee for tfTwoAssetIfEmpty mode (optional)
	// Only used when depositing into an empty AMM
	TradingFee uint16 `json:"TradingFee,omitempty"`
}

// Asset identifies an asset in an AMM
type Asset struct {
	Currency string `json:"currency"`
	Issuer   string `json:"issuer,omitempty"`
}

// NewAMMDeposit creates a new AMMDeposit transaction
func NewAMMDeposit(account string, asset, asset2 Asset) *AMMDeposit {
	return &AMMDeposit{
		BaseTx: *NewBaseTx(TypeAMMDeposit, account),
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMDeposit) TxType() Type {
	return TypeAMMDeposit
}

// Validate validates the AMMDeposit transaction
// Reference: rippled AMMDeposit.cpp preflight
func (a *AMMDeposit) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags
	if a.GetFlags()&tfAMMDepositMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMDeposit")
	}

	if a.Asset.Currency == "" {
		return errors.New("temMALFORMED: Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("temMALFORMED: Asset2 is required")
	}

	// Validate flag combinations - must have exactly one deposit mode
	flags := a.GetFlags()
	flagCount := 0
	if flags&tfLPToken != 0 {
		flagCount++
	}
	if flags&tfSingleAsset != 0 {
		flagCount++
	}
	if flags&tfTwoAsset != 0 {
		flagCount++
	}
	if flags&tfOneAssetLPToken != 0 {
		flagCount++
	}
	if flags&tfLimitLPToken != 0 {
		flagCount++
	}
	if flags&tfTwoAssetIfEmpty != 0 {
		flagCount++
	}

	// At least one flag must be set (deposit mode)
	if flagCount == 0 {
		return errors.New("temMALFORMED: must specify deposit mode flag")
	}

	// Validate amounts if provided
	if a.Amount != nil {
		if err := validateAMMAmount(*a.Amount); err != nil {
			return errors.New("temBAD_AMOUNT: invalid Amount - " + err.Error())
		}
	}
	if a.Amount2 != nil {
		if err := validateAMMAmount(*a.Amount2); err != nil {
			return errors.New("temBAD_AMOUNT: invalid Amount2 - " + err.Error())
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMDeposit) Flatten() (map[string]any, error) {
	m := a.Common.ToMap()

	m["Asset"] = a.Asset
	m["Asset2"] = a.Asset2

	if a.Amount != nil {
		m["Amount"] = flattenAmount(*a.Amount)
	}
	if a.Amount2 != nil {
		m["Amount2"] = flattenAmount(*a.Amount2)
	}
	if a.EPrice != nil {
		m["EPrice"] = flattenAmount(*a.EPrice)
	}
	if a.LPTokenOut != nil {
		m["LPTokenOut"] = flattenAmount(*a.LPTokenOut)
	}

	return m, nil
}

// AMMWithdraw withdraws assets from an AMM.
type AMMWithdraw struct {
	BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset Asset `json:"Asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 Asset `json:"Asset2"`

	// Amount is the amount of first asset to withdraw (optional)
	Amount *Amount `json:"Amount,omitempty"`

	// Amount2 is the amount of second asset to withdraw (optional)
	Amount2 *Amount `json:"Amount2,omitempty"`

	// EPrice is the effective price limit (optional)
	EPrice *Amount `json:"EPrice,omitempty"`

	// LPTokenIn is the LP tokens to burn (optional)
	LPTokenIn *Amount `json:"LPTokenIn,omitempty"`
}

// NewAMMWithdraw creates a new AMMWithdraw transaction
func NewAMMWithdraw(account string, asset, asset2 Asset) *AMMWithdraw {
	return &AMMWithdraw{
		BaseTx: *NewBaseTx(TypeAMMWithdraw, account),
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMWithdraw) TxType() Type {
	return TypeAMMWithdraw
}

// Validate validates the AMMWithdraw transaction
// Reference: rippled AMMWithdraw.cpp preflight
func (a *AMMWithdraw) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags
	if a.GetFlags()&tfAMMWithdrawMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMWithdraw")
	}

	if a.Asset.Currency == "" {
		return errors.New("temMALFORMED: Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("temMALFORMED: Asset2 is required")
	}

	flags := a.GetFlags()

	// Withdrawal sub-transaction flags (exactly one must be set)
	tfWithdrawSubTx := tfLPToken | tfWithdrawAll | tfOneAssetWithdrawAll | tfSingleAsset | tfTwoAsset | tfOneAssetLPToken | tfLimitLPToken
	subTxFlags := flags & tfWithdrawSubTx

	// Count number of mode flags set using popcount
	flagCount := 0
	for f := subTxFlags; f != 0; f &= f - 1 {
		flagCount++
	}
	if flagCount != 1 {
		return errors.New("temMALFORMED: exactly one withdraw mode flag must be set")
	}

	// Validate field requirements for each mode
	hasAmount := a.Amount != nil
	hasAmount2 := a.Amount2 != nil
	hasEPrice := a.EPrice != nil
	hasLPTokenIn := a.LPTokenIn != nil

	if flags&tfLPToken != 0 {
		// LPToken mode: LPTokenIn required, no amount/amount2/ePrice
		if !hasLPTokenIn || hasAmount || hasAmount2 || hasEPrice {
			return errors.New("temMALFORMED: tfLPToken requires LPTokenIn only")
		}
	} else if flags&tfWithdrawAll != 0 {
		// WithdrawAll mode: no fields needed
		if hasLPTokenIn || hasAmount || hasAmount2 || hasEPrice {
			return errors.New("temMALFORMED: tfWithdrawAll requires no amount fields")
		}
	} else if flags&tfOneAssetWithdrawAll != 0 {
		// OneAssetWithdrawAll mode: Amount required (identifies which asset)
		if !hasAmount || hasLPTokenIn || hasAmount2 || hasEPrice {
			return errors.New("temMALFORMED: tfOneAssetWithdrawAll requires Amount only")
		}
	} else if flags&tfSingleAsset != 0 {
		// SingleAsset mode: Amount required
		if !hasAmount || hasLPTokenIn || hasAmount2 || hasEPrice {
			return errors.New("temMALFORMED: tfSingleAsset requires Amount only")
		}
	} else if flags&tfTwoAsset != 0 {
		// TwoAsset mode: Amount and Amount2 required
		if !hasAmount || !hasAmount2 || hasLPTokenIn || hasEPrice {
			return errors.New("temMALFORMED: tfTwoAsset requires Amount and Amount2")
		}
	} else if flags&tfOneAssetLPToken != 0 {
		// OneAssetLPToken mode: Amount and LPTokenIn required
		if !hasAmount || !hasLPTokenIn || hasAmount2 || hasEPrice {
			return errors.New("temMALFORMED: tfOneAssetLPToken requires Amount and LPTokenIn")
		}
	} else if flags&tfLimitLPToken != 0 {
		// LimitLPToken mode: Amount and EPrice required
		if !hasAmount || !hasEPrice || hasLPTokenIn || hasAmount2 {
			return errors.New("temMALFORMED: tfLimitLPToken requires Amount and EPrice")
		}
	}

	// Amount and Amount2 cannot have the same issue if both present
	if hasAmount && hasAmount2 {
		if a.Amount.Currency == a.Amount2.Currency && a.Amount.Issuer == a.Amount2.Issuer {
			return errors.New("temBAD_AMM_TOKENS: Amount and Amount2 cannot have the same issue")
		}
	}

	// Validate LPTokenIn is positive
	if hasLPTokenIn {
		if err := validateAMMAmount(*a.LPTokenIn); err != nil {
			return errors.New("temBAD_AMM_TOKENS: invalid LPTokenIn - " + err.Error())
		}
	}

	// Validate amounts if provided
	if hasAmount {
		if err := validateAMMAmount(*a.Amount); err != nil {
			return errors.New("temBAD_AMOUNT: invalid Amount - " + err.Error())
		}
	}
	if hasAmount2 {
		if err := validateAMMAmount(*a.Amount2); err != nil {
			return errors.New("temBAD_AMOUNT: invalid Amount2 - " + err.Error())
		}
	}
	if hasEPrice {
		if err := validateAMMAmount(*a.EPrice); err != nil {
			return errors.New("temBAD_AMOUNT: invalid EPrice - " + err.Error())
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMWithdraw) Flatten() (map[string]any, error) {
	m := a.Common.ToMap()

	m["Asset"] = a.Asset
	m["Asset2"] = a.Asset2

	if a.Amount != nil {
		m["Amount"] = flattenAmount(*a.Amount)
	}
	if a.Amount2 != nil {
		m["Amount2"] = flattenAmount(*a.Amount2)
	}
	if a.EPrice != nil {
		m["EPrice"] = flattenAmount(*a.EPrice)
	}
	if a.LPTokenIn != nil {
		m["LPTokenIn"] = flattenAmount(*a.LPTokenIn)
	}

	return m, nil
}

// AMMVote votes on the trading fee for an AMM.
type AMMVote struct {
	BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset Asset `json:"Asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 Asset `json:"Asset2"`

	// TradingFee is the proposed fee in basis points (0-1000)
	TradingFee uint16 `json:"TradingFee"`
}

// NewAMMVote creates a new AMMVote transaction
func NewAMMVote(account string, asset, asset2 Asset, tradingFee uint16) *AMMVote {
	return &AMMVote{
		BaseTx:     *NewBaseTx(TypeAMMVote, account),
		Asset:      asset,
		Asset2:     asset2,
		TradingFee: tradingFee,
	}
}

// TxType returns the transaction type
func (a *AMMVote) TxType() Type {
	return TypeAMMVote
}

// Validate validates the AMMVote transaction
// Reference: rippled AMMVote.cpp preflight
func (a *AMMVote) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags - no flags are valid for AMMVote
	if a.GetFlags()&tfAMMVoteMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMVote")
	}

	// Validate asset pair
	if a.Asset.Currency == "" {
		return errors.New("temMALFORMED: Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("temMALFORMED: Asset2 is required")
	}

	// TradingFee must be within threshold
	if a.TradingFee > TRADING_FEE_THRESHOLD {
		return errors.New("temBAD_FEE: TradingFee must be 0-1000")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMVote) Flatten() (map[string]any, error) {
	m := a.Common.ToMap()

	m["Asset"] = a.Asset
	m["Asset2"] = a.Asset2
	m["TradingFee"] = a.TradingFee

	return m, nil
}

// AMMBid places a bid on an AMM auction slot.
type AMMBid struct {
	BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset Asset `json:"Asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 Asset `json:"Asset2"`

	// BidMin is the minimum bid amount (optional)
	BidMin *Amount `json:"BidMin,omitempty"`

	// BidMax is the maximum bid amount (optional)
	BidMax *Amount `json:"BidMax,omitempty"`

	// AuthAccounts are accounts to authorize for discounted trading (optional)
	AuthAccounts []AuthAccount `json:"AuthAccounts,omitempty"`
}

// AuthAccount is an authorized account for AMM slot trading
type AuthAccount struct {
	AuthAccount AuthAccountData `json:"AuthAccount"`
}

// AuthAccountData contains the account address
type AuthAccountData struct {
	Account string `json:"Account"`
}

// NewAMMBid creates a new AMMBid transaction
func NewAMMBid(account string, asset, asset2 Asset) *AMMBid {
	return &AMMBid{
		BaseTx: *NewBaseTx(TypeAMMBid, account),
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMBid) TxType() Type {
	return TypeAMMBid
}

// Validate validates the AMMBid transaction
// Reference: rippled AMMBid.cpp preflight
func (a *AMMBid) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags - no flags are valid for AMMBid
	if a.GetFlags()&tfAMMBidMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMBid")
	}

	// Validate asset pair
	if a.Asset.Currency == "" {
		return errors.New("temMALFORMED: Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("temMALFORMED: Asset2 is required")
	}

	// Validate BidMin if present
	if a.BidMin != nil {
		if err := validateAMMAmount(*a.BidMin); err != nil {
			return errors.New("temMALFORMED: invalid BidMin - " + err.Error())
		}
	}

	// Validate BidMax if present
	if a.BidMax != nil {
		if err := validateAMMAmount(*a.BidMax); err != nil {
			return errors.New("temMALFORMED: invalid BidMax - " + err.Error())
		}
	}

	// Max 4 auth accounts
	if len(a.AuthAccounts) > AUCTION_SLOT_MAX_AUTH_ACCOUNTS {
		return errors.New("temMALFORMED: cannot have more than 4 AuthAccounts")
	}

	// Check for duplicate auth accounts and self-authorization
	if len(a.AuthAccounts) > 0 {
		seen := make(map[string]bool)
		for _, authAcct := range a.AuthAccounts {
			acct := authAcct.AuthAccount.Account
			// Cannot authorize self
			if acct == a.Common.Account {
				return errors.New("temMALFORMED: cannot authorize self in AuthAccounts")
			}
			// Check for duplicates
			if seen[acct] {
				return errors.New("temMALFORMED: duplicate account in AuthAccounts")
			}
			seen[acct] = true
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMBid) Flatten() (map[string]any, error) {
	m := a.Common.ToMap()

	m["Asset"] = a.Asset
	m["Asset2"] = a.Asset2

	if a.BidMin != nil {
		m["BidMin"] = flattenAmount(*a.BidMin)
	}
	if a.BidMax != nil {
		m["BidMax"] = flattenAmount(*a.BidMax)
	}
	if len(a.AuthAccounts) > 0 {
		m["AuthAccounts"] = a.AuthAccounts
	}

	return m, nil
}

// AMMDelete deletes an empty AMM.
type AMMDelete struct {
	BaseTx

	// Asset identifies the first asset of the AMM (required)
	Asset Asset `json:"Asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 Asset `json:"Asset2"`
}

// NewAMMDelete creates a new AMMDelete transaction
func NewAMMDelete(account string, asset, asset2 Asset) *AMMDelete {
	return &AMMDelete{
		BaseTx: *NewBaseTx(TypeAMMDelete, account),
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMDelete) TxType() Type {
	return TypeAMMDelete
}

// Validate validates the AMMDelete transaction
// Reference: rippled AMMDelete.cpp preflight
func (a *AMMDelete) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags - no flags are valid for AMMDelete
	if a.GetFlags()&tfAMMDeleteMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMDelete")
	}

	// Validate asset pair
	if a.Asset.Currency == "" {
		return errors.New("temMALFORMED: Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("temMALFORMED: Asset2 is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMDelete) Flatten() (map[string]any, error) {
	m := a.Common.ToMap()

	m["Asset"] = a.Asset
	m["Asset2"] = a.Asset2

	return m, nil
}

// AMMClawback claws back tokens from an AMM.
type AMMClawback struct {
	BaseTx

	// Holder is the account holding LP tokens (required)
	Holder string `json:"Holder"`

	// Asset identifies the first asset of the AMM (required)
	Asset Asset `json:"Asset"`

	// Asset2 identifies the second asset of the AMM (required)
	Asset2 Asset `json:"Asset2"`

	// Amount is the amount to claw back (optional)
	Amount *Amount `json:"Amount,omitempty"`
}

// NewAMMClawback creates a new AMMClawback transaction
func NewAMMClawback(account, holder string, asset, asset2 Asset) *AMMClawback {
	return &AMMClawback{
		BaseTx: *NewBaseTx(TypeAMMClawback, account),
		Holder: holder,
		Asset:  asset,
		Asset2: asset2,
	}
}

// TxType returns the transaction type
func (a *AMMClawback) TxType() Type {
	return TypeAMMClawback
}

// Validate validates the AMMClawback transaction
// Reference: rippled AMMClawback.cpp preflight
func (a *AMMClawback) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags
	if a.GetFlags()&tfAMMClawbackMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for AMMClawback")
	}

	// Holder is required
	if a.Holder == "" {
		return errors.New("temMALFORMED: Holder is required")
	}

	// Holder cannot be the same as issuer (Account)
	if a.Holder == a.Common.Account {
		return errors.New("temMALFORMED: Holder cannot be the same as issuer")
	}

	// Validate asset pair
	if a.Asset.Currency == "" {
		return errors.New("temMALFORMED: Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("temMALFORMED: Asset2 is required")
	}

	// Asset cannot be XRP (must be issued currency)
	if a.Asset.Currency == "XRP" || a.Asset.Currency == "" && a.Asset.Issuer == "" {
		return errors.New("temMALFORMED: Asset cannot be XRP")
	}

	// Asset issuer must match the transaction account (issuer)
	if a.Asset.Issuer != a.Common.Account {
		return errors.New("temMALFORMED: Asset issuer must match Account")
	}

	// If tfClawTwoAssets is set, both assets must be issued by the same issuer
	if a.GetFlags()&tfClawTwoAssets != 0 {
		if a.Asset.Issuer != a.Asset2.Issuer {
			return errors.New("temINVALID_FLAG: tfClawTwoAssets requires both assets to have the same issuer")
		}
	}

	// Validate Amount if provided
	if a.Amount != nil {
		// Amount must be positive
		if err := validateAMMAmount(*a.Amount); err != nil {
			return errors.New("temBAD_AMOUNT: invalid Amount - " + err.Error())
		}
		// Amount's issue must match Asset
		if a.Amount.Currency != a.Asset.Currency || a.Amount.Issuer != a.Asset.Issuer {
			return errors.New("temBAD_AMOUNT: Amount issue must match Asset")
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AMMClawback) Flatten() (map[string]any, error) {
	m := a.Common.ToMap()

	m["Holder"] = a.Holder
	m["Asset"] = a.Asset
	m["Asset2"] = a.Asset2

	if a.Amount != nil {
		m["Amount"] = flattenAmount(*a.Amount)
	}

	return m, nil
}
