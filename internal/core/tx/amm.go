package tx

import "errors"

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
func (a *AMMCreate) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	if a.Amount.Value == "" {
		return errors.New("Amount is required")
	}

	if a.Amount2.Value == "" {
		return errors.New("Amount2 is required")
	}

	// TradingFee must be 0-1000
	if a.TradingFee > 1000 {
		return errors.New("TradingFee must be 0-1000")
	}

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
}

// Asset identifies an asset in an AMM
type Asset struct {
	Currency string `json:"currency"`
	Issuer   string `json:"issuer,omitempty"`
}

// AMMDeposit flags
const (
	// tfLPToken requests LP tokens in return
	AMMDepositFlagLPToken uint32 = 0x00010000
	// tfSingleAsset deposits a single asset
	AMMDepositFlagSingleAsset uint32 = 0x00080000
	// tfTwoAsset deposits two assets
	AMMDepositFlagTwoAsset uint32 = 0x00100000
	// tfOneAssetLPToken deposits one asset for specific LP tokens
	AMMDepositFlagOneAssetLPToken uint32 = 0x00200000
	// tfLimitLPToken limits the LP tokens received
	AMMDepositFlagLimitLPToken uint32 = 0x00400000
)

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
func (a *AMMDeposit) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	if a.Asset.Currency == "" {
		return errors.New("Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("Asset2 is required")
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

// AMMWithdraw flags
const (
	// tfWithdrawAll withdraws all assets
	AMMWithdrawFlagWithdrawAll uint32 = 0x00020000
	// tfOneAssetWithdrawAll withdraws all of one asset
	AMMWithdrawFlagOneAssetWithdrawAll uint32 = 0x00040000
	// tfSingleAsset withdraws a single asset
	AMMWithdrawFlagSingleAsset uint32 = 0x00080000
	// tfTwoAsset withdraws two assets
	AMMWithdrawFlagTwoAsset uint32 = 0x00100000
	// tfOneAssetLPToken withdraws one asset for specific LP tokens
	AMMWithdrawFlagOneAssetLPToken uint32 = 0x00200000
	// tfLimitLPToken limits the LP tokens burned
	AMMWithdrawFlagLimitLPToken uint32 = 0x00400000
)

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
func (a *AMMWithdraw) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	if a.Asset.Currency == "" {
		return errors.New("Asset is required")
	}

	if a.Asset2.Currency == "" {
		return errors.New("Asset2 is required")
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
func (a *AMMVote) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	if a.TradingFee > 1000 {
		return errors.New("TradingFee must be 0-1000")
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
func (a *AMMBid) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	// Max 4 auth accounts
	if len(a.AuthAccounts) > 4 {
		return errors.New("cannot have more than 4 AuthAccounts")
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
func (a *AMMDelete) Validate() error {
	return a.BaseTx.Validate()
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
func (a *AMMClawback) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	if a.Holder == "" {
		return errors.New("Holder is required")
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
