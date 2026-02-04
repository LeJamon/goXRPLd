package testing

import (
	"github.com/LeJamon/goXRPLd/internal/core/tx/account"
	"github.com/LeJamon/goXRPLd/internal/core/tx/depositpreauth"
	"github.com/LeJamon/goXRPLd/internal/core/tx/offer"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
	"testing"
	"time"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
	"github.com/LeJamon/goXRPLd/internal/core/ledger"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/genesis"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/payment"
)

// TestEnv manages a test ledger environment for transaction testing.
// It provides a simplified interface for creating accounts, funding them,
// submitting transactions, and verifying results.
type TestEnv struct {
	t        *testing.T
	ledger   *ledger.Ledger
	clock    *ManualClock
	accounts map[string]*Account

	// Genesis ledger reference
	genesisLedger *ledger.Ledger

	// Ledger history for verification
	ledgerHistory map[uint32]*ledger.Ledger

	// Current ledger sequence
	currentSeq uint32

	// Fees configuration
	baseFee          uint64
	reserveBase      uint64
	reserveIncrement uint64
}

// NewTestEnv creates a new test environment with a genesis ledger.
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	// Create genesis ledger with default configuration
	genesisConfig := genesis.DefaultConfig()
	genesisResult, err := genesis.Create(genesisConfig)
	if err != nil {
		t.Fatalf("Failed to create genesis ledger: %v", err)
	}

	// Create the ledger from genesis
	// Note: XRPAmount.Fees has unexported fields, so we use a zero value
	var fees XRPAmount.Fees
	genesisLedger := ledger.FromGenesis(
		genesisResult.Header,
		genesisResult.StateMap,
		genesisResult.TxMap,
		fees,
	)

	// Create the first open ledger
	clock := NewManualClock()
	openLedger, err := ledger.NewOpen(genesisLedger, clock.Now())
	if err != nil {
		t.Fatalf("Failed to create open ledger: %v", err)
	}

	env := &TestEnv{
		t:                t,
		ledger:           openLedger,
		clock:            clock,
		accounts:         make(map[string]*Account),
		genesisLedger:    genesisLedger,
		ledgerHistory:    make(map[uint32]*ledger.Ledger),
		currentSeq:       2,
		baseFee:          10,
		reserveBase:      10_000_000, // 10 XRP
		reserveIncrement: 2_000_000,  // 2 XRP
	}

	// Store genesis in history
	env.ledgerHistory[1] = genesisLedger

	// Register master account
	master := MasterAccount()
	env.accounts[master.Name] = master

	return env
}

// NewTestEnvWithConfig creates a new test environment with custom genesis configuration.
func NewTestEnvWithConfig(t *testing.T, cfg genesis.Config) *TestEnv {
	t.Helper()

	genesisResult, err := genesis.Create(cfg)
	if err != nil {
		t.Fatalf("Failed to create genesis ledger: %v", err)
	}

	// Note: XRPAmount.Fees has unexported fields, so we use a zero value
	var fees XRPAmount.Fees
	genesisLedger := ledger.FromGenesis(
		genesisResult.Header,
		genesisResult.StateMap,
		genesisResult.TxMap,
		fees,
	)

	clock := NewManualClock()
	openLedger, err := ledger.NewOpen(genesisLedger, clock.Now())
	if err != nil {
		t.Fatalf("Failed to create open ledger: %v", err)
	}

	env := &TestEnv{
		t:                t,
		ledger:           openLedger,
		clock:            clock,
		accounts:         make(map[string]*Account),
		genesisLedger:    genesisLedger,
		ledgerHistory:    make(map[uint32]*ledger.Ledger),
		currentSeq:       2,
		baseFee:          uint64(cfg.Fees.BaseFee.Drops()),
		reserveBase:      uint64(cfg.Fees.ReserveBase.Drops()),
		reserveIncrement: uint64(cfg.Fees.ReserveIncrement.Drops()),
	}

	env.ledgerHistory[1] = genesisLedger
	master := MasterAccount()
	env.accounts[master.Name] = master

	return env
}

// Fund funds the specified accounts from the master account.
// Each account receives the specified amount or a default of 1000 XRP.
func (e *TestEnv) Fund(accounts ...*Account) {
	e.t.Helper()

	for _, acc := range accounts {
		e.FundAmount(acc, uint64(XRP(1000)))
	}
}

// FundAmount funds an account with a specific amount.
// Like rippled's test environment, this also enables DefaultRipple on the account.
// This is important for trust line behavior - without DefaultRipple, trust lines
// cannot be deleted when limit is set to 0 (the NoRipple state would be "non-default").
func (e *TestEnv) FundAmount(acc *Account, amount uint64) {
	e.t.Helper()

	// Register account
	e.accounts[acc.Name] = acc

	// Create a payment from master to the new account
	master := e.accounts["master"]
	if master == nil {
		e.t.Fatal("Master account not found")
	}

	// Fund with extra to cover the AccountSet fee (for enabling DefaultRipple)
	// This ensures the account ends up with the requested amount.
	totalFunding := amount + e.baseFee

	// Create payment transaction
	seq := e.Seq(master)
	payment := payment.NewPayment(master.Address, acc.Address, tx.NewXRPAmount(int64(totalFunding)))
	payment.Fee = formatUint64(e.baseFee)
	payment.Sequence = &seq

	// Submit the payment
	result := e.Submit(payment)
	if !result.Success {
		e.t.Fatalf("Failed to fund account %s: %s", acc.Name, result.Code)
	}

	// Enable DefaultRipple on the account (matching rippled's test environment)
	// This allows trust lines to be properly deleted when limits are set to 0.
	e.enableDefaultRipple(acc)
}

// enableDefaultRipple enables the DefaultRipple flag on an account.
// This matches rippled's test environment behavior.
func (e *TestEnv) enableDefaultRipple(acc *Account) {
	e.t.Helper()

	accountSet := account.NewAccountSet(acc.Address)
	accountSet.EnableDefaultRipple()
	accountSet.Fee = formatUint64(e.baseFee)
	seq := e.Seq(acc)
	accountSet.Sequence = &seq

	result := e.Submit(accountSet)
	if !result.Success {
		e.t.Fatalf("Failed to enable DefaultRipple for account %s: %s", acc.Name, result.Code)
	}
}

// EnableDepositAuth enables the DepositAuth flag on an account.
// When enabled, the account can only receive payments from preauthorized accounts.
func (e *TestEnv) EnableDepositAuth(acc *Account) {
	e.t.Helper()

	accountSet := account.NewAccountSet(acc.Address)
	accountSet.EnableDepositAuth()
	accountSet.Fee = formatUint64(e.baseFee)
	seq := e.Seq(acc)
	accountSet.Sequence = &seq

	result := e.Submit(accountSet)
	if !result.Success {
		e.t.Fatalf("Failed to enable DepositAuth for account %s: %s", acc.Name, result.Code)
	}
}

// DisableDepositAuth disables the DepositAuth flag on an account.
func (e *TestEnv) DisableDepositAuth(acc *Account) {
	e.t.Helper()

	accountSet := account.NewAccountSet(acc.Address)
	flag := account.AccountSetFlagDepositAuth
	accountSet.ClearFlag = &flag
	accountSet.Fee = formatUint64(e.baseFee)
	seq := e.Seq(acc)
	accountSet.Sequence = &seq

	result := e.Submit(accountSet)
	if !result.Success {
		e.t.Fatalf("Failed to disable DepositAuth for account %s: %s", acc.Name, result.Code)
	}
}

// EnableGlobalFreeze enables the GlobalFreeze flag on an account.
// When enabled, all trust lines for this account's issued currencies are frozen.
func (e *TestEnv) EnableGlobalFreeze(acc *Account) {
	e.t.Helper()

	accountSet := account.NewAccountSet(acc.Address)
	flag := account.AccountSetFlagGlobalFreeze
	accountSet.SetFlag = &flag
	accountSet.Fee = formatUint64(e.baseFee)
	seq := e.Seq(acc)
	accountSet.Sequence = &seq

	result := e.Submit(accountSet)
	if !result.Success {
		e.t.Fatalf("Failed to enable GlobalFreeze for account %s: %s", acc.Name, result.Code)
	}
}

// DisableGlobalFreeze disables the GlobalFreeze flag on an account.
// Note: Cannot be cleared if NoFreeze is set.
func (e *TestEnv) DisableGlobalFreeze(acc *Account) {
	e.t.Helper()

	accountSet := account.NewAccountSet(acc.Address)
	flag := account.AccountSetFlagGlobalFreeze
	accountSet.ClearFlag = &flag
	accountSet.Fee = formatUint64(e.baseFee)
	seq := e.Seq(acc)
	accountSet.Sequence = &seq

	result := e.Submit(accountSet)
	if !result.Success {
		e.t.Fatalf("Failed to disable GlobalFreeze for account %s: %s", acc.Name, result.Code)
	}
}

// EnableNoFreeze enables the NoFreeze flag on an account.
// Once set, this flag cannot be cleared. It prevents the account from freezing trust lines.
func (e *TestEnv) EnableNoFreeze(acc *Account) {
	e.t.Helper()

	accountSet := account.NewAccountSet(acc.Address)
	flag := account.AccountSetFlagNoFreeze
	accountSet.SetFlag = &flag
	accountSet.Fee = formatUint64(e.baseFee)
	seq := e.Seq(acc)
	accountSet.Sequence = &seq

	result := e.Submit(accountSet)
	if !result.Success {
		e.t.Fatalf("Failed to enable NoFreeze for account %s: %s", acc.Name, result.Code)
	}
}

// SetTransferRate sets the transfer rate for an account.
// Rate is specified as a multiplier * 1e9 (1e9 = 100%, 1.1e9 = 110% means 10% fee).
// Use 0 to clear the transfer rate (sets it back to 1e9 / 100%).
func (e *TestEnv) SetTransferRate(acc *Account, rate uint32) {
	e.t.Helper()

	accountSet := account.NewAccountSet(acc.Address)
	accountSet.TransferRate = &rate
	accountSet.Fee = formatUint64(e.baseFee)
	seq := e.Seq(acc)
	accountSet.Sequence = &seq

	result := e.Submit(accountSet)
	if !result.Success {
		e.t.Fatalf("Failed to set transfer rate for account %s: %s", acc.Name, result.Code)
	}
}

// EnableRequireAuth enables the RequireAuth flag on an account.
// When enabled, trust lines to this account require authorization.
func (e *TestEnv) EnableRequireAuth(acc *Account) {
	e.t.Helper()

	accountSet := account.NewAccountSet(acc.Address)
	flag := account.AccountSetFlagRequireAuth
	accountSet.SetFlag = &flag
	accountSet.Fee = formatUint64(e.baseFee)
	seq := e.Seq(acc)
	accountSet.Sequence = &seq

	result := e.Submit(accountSet)
	if !result.Success {
		e.t.Fatalf("Failed to enable RequireAuth for account %s: %s", acc.Name, result.Code)
	}
}

// DisableRequireAuth disables the RequireAuth flag on an account.
func (e *TestEnv) DisableRequireAuth(acc *Account) {
	e.t.Helper()

	accountSet := account.NewAccountSet(acc.Address)
	flag := account.AccountSetFlagRequireAuth
	accountSet.ClearFlag = &flag
	accountSet.Fee = formatUint64(e.baseFee)
	seq := e.Seq(acc)
	accountSet.Sequence = &seq

	result := e.Submit(accountSet)
	if !result.Success {
		e.t.Fatalf("Failed to disable RequireAuth for account %s: %s", acc.Name, result.Code)
	}
}

// Preauthorize allows owner to preauthorize authorized for deposits.
// This creates a DepositPreauth ledger entry.
func (e *TestEnv) Preauthorize(owner, authorized *Account) {
	e.t.Helper()

	preauth := depositpreauth.NewDepositPreauth(owner.Address)
	preauth.SetAuthorize(authorized.Address)
	preauth.Fee = formatUint64(e.baseFee)
	seq := e.Seq(owner)
	preauth.Sequence = &seq

	result := e.Submit(preauth)
	if !result.Success {
		e.t.Fatalf("Failed to preauthorize %s for %s: %s", authorized.Name, owner.Name, result.Code)
	}
}

// Unauthorize removes preauthorization for authorized from owner.
func (e *TestEnv) Unauthorize(owner, authorized *Account) {
	e.t.Helper()

	preauth := depositpreauth.NewDepositPreauth(owner.Address)
	preauth.SetUnauthorize(authorized.Address)
	preauth.Fee = formatUint64(e.baseFee)
	seq := e.Seq(owner)
	preauth.Sequence = &seq

	result := e.Submit(preauth)
	if !result.Success {
		e.t.Fatalf("Failed to unauthorize %s for %s: %s", authorized.Name, owner.Name, result.Code)
	}
}

// CreateOffer creates an offer on the DEX.
// takerGets is what the offer creator will receive (what the taker pays).
// takerPays is what the offer creator will pay (what the taker gets).
func (e *TestEnv) CreateOffer(acc *Account, takerGets, takerPays tx.Amount) TxResult {
	e.t.Helper()

	offerTx := offer.NewOfferCreate(acc.Address, takerGets, takerPays)
	offerTx.Fee = formatUint64(e.baseFee)
	seq := e.Seq(acc)
	offerTx.Sequence = &seq

	return e.Submit(offerTx)
}

// CreatePassiveOffer creates a passive offer on the DEX.
// Passive offers don't immediately consume offers at an equal or better rate.
func (e *TestEnv) CreatePassiveOffer(acc *Account, takerGets, takerPays tx.Amount) TxResult {
	e.t.Helper()

	offerTx := offer.NewOfferCreate(acc.Address, takerGets, takerPays)
	offerTx.SetPassive()
	offerTx.Fee = formatUint64(e.baseFee)
	seq := e.Seq(acc)
	offerTx.Sequence = &seq

	return e.Submit(offerTx)
}

// Close closes the current ledger and advances to a new one.
// This is equivalent to "ledger_accept" in rippled.
func (e *TestEnv) Close() {
	e.t.Helper()

	// Advance time
	e.clock.Advance(10 * time.Second)

	// Close current ledger
	if err := e.ledger.Close(e.clock.Now(), 0); err != nil {
		e.t.Fatalf("Failed to close ledger: %v", err)
	}

	// Validate the ledger (in test mode, we auto-validate)
	if err := e.ledger.SetValidated(); err != nil {
		e.t.Fatalf("Failed to validate ledger: %v", err)
	}

	// Store in history
	e.ledgerHistory[e.ledger.Sequence()] = e.ledger

	// Create new open ledger
	newLedger, err := ledger.NewOpen(e.ledger, e.clock.Now())
	if err != nil {
		e.t.Fatalf("Failed to create new ledger: %v", err)
	}

	e.ledger = newLedger
	e.currentSeq++
}

// Submit submits a transaction to the current open ledger.
// If the transaction doesn't have a sequence number set, it will be auto-filled
// from the account's current sequence in the ledger.
func (e *TestEnv) Submit(transaction interface{}) TxResult {
	e.t.Helper()

	// Convert to tx.Transaction interface
	txn, ok := transaction.(tx.Transaction)
	if !ok {
		e.t.Fatalf("Transaction does not implement tx.Transaction interface")
		return TxResult{Code: "temINVALID", Success: false, Message: "Invalid transaction type"}
	}

	// Auto-fill sequence if not set
	common := txn.GetCommon()
	if common.Sequence == nil {
		// Look up the account to get current sequence
		_, accountID, err := addresscodec.DecodeClassicAddressToAccountID(common.Account)
		if err != nil {
			e.t.Fatalf("Failed to decode account address: %v", err)
			return TxResult{Code: "temINVALID", Success: false, Message: "Invalid account address"}
		}

		var id [20]byte
		copy(id[:], accountID)
		accountKey := keylet.Account(id)

		data, err := e.ledger.Read(accountKey)
		if err != nil || data == nil {
			e.t.Fatalf("Failed to read account for sequence auto-fill: %v", err)
			return TxResult{Code: "terNO_ACCOUNT", Success: false, Message: "Account not found"}
		}

		accountRoot, err := sle.ParseAccountRootFromBytes(data)
		if err != nil {
			e.t.Fatalf("Failed to parse account root: %v", err)
			return TxResult{Code: "temINVALID", Success: false, Message: "Failed to parse account"}
		}

		seq := accountRoot.Sequence
		common.Sequence = &seq
	}

	// Create engine config
	engineConfig := tx.EngineConfig{
		BaseFee:                   e.baseFee,
		ReserveBase:               e.reserveBase,
		ReserveIncrement:          e.reserveIncrement,
		LedgerSequence:            e.ledger.Sequence(),
		SkipSignatureVerification: true, // Skip signatures in test mode
	}

	// Create engine with current ledger
	engine := tx.NewEngine(e.ledger, engineConfig)

	// Apply the transaction
	applyResult := engine.Apply(txn)

	// Success should only be true for tesSUCCESS, not for tec codes
	// (tec codes are "applied" but not "successful" - fee is charged but operation failed)
	return TxResult{
		Code:     applyResult.Result.String(),
		Success:  applyResult.Result.IsSuccess(),
		Message:  applyResult.Message,
		Metadata: nil, // Could serialize metadata if needed
	}
}

// Balance returns the XRP balance of an account in drops.
func (e *TestEnv) Balance(acc *Account) uint64 {
	e.t.Helper()

	// Get account keylet
	accountKey := keylet.Account(acc.ID)

	// Check if account exists
	exists, err := e.ledger.Exists(accountKey)
	if err != nil {
		e.t.Fatalf("Failed to check account existence: %v", err)
		return 0
	}
	if !exists {
		return 0 // Account doesn't exist
	}

	// Read account data
	data, err := e.ledger.Read(accountKey)
	if err != nil {
		e.t.Fatalf("Failed to read account: %v", err)
		return 0
	}

	// Parse account root to get balance
	accountRoot, err := sle.ParseAccountRootFromBytes(data)
	if err != nil {
		e.t.Fatalf("Failed to parse account data: %v", err)
		return 0
	}

	return accountRoot.Balance
}

// IOUBalance returns the IOU balance of an account for a specific currency and issuer.
// The balance is returned from the perspective of the holder (not the issuer).
// Positive means the holder has tokens, negative means they owe tokens.
func (e *TestEnv) IOUBalance(holder, issuer *Account, currency string) *sle.Amount {
	e.t.Helper()

	// Get trust line keylet
	lineKey := keylet.Line(holder.ID, issuer.ID, currency)

	// Check if trust line exists
	exists, err := e.ledger.Exists(lineKey)
	if err != nil {
		e.t.Fatalf("Failed to check trust line existence: %v", err)
		return nil
	}
	if !exists {
		// No trust line = zero balance
		zero := sle.NewIssuedAmountFromFloat64(0, currency, issuer.Address)
		return &zero
	}

	// Read trust line data
	data, err := e.ledger.Read(lineKey)
	if err != nil {
		e.t.Fatalf("Failed to read trust line: %v", err)
		return nil
	}

	// Parse RippleState
	rs, err := sle.ParseRippleState(data)
	if err != nil {
		e.t.Fatalf("Failed to parse trust line: %v", err)
		return nil
	}

	// Determine if holder is low or high account
	// Balance sign convention: positive means low owes high
	isLow := keylet.IsLowAccount(holder.ID, issuer.ID)

	balance := rs.Balance
	if !isLow {
		// If holder is high account, negate the balance
		// (positive balance means low owes high, so high has positive tokens)
		balance = balance.Negate()
	}

	return &balance
}

// BalanceIOU returns the IOU balance of an account for a specific currency and issuer as float64.
// This is a convenience method for tests that need simple numeric comparisons.
func (e *TestEnv) BalanceIOU(holder *Account, currency string, issuer *Account) float64 {
	e.t.Helper()

	balance := e.IOUBalance(holder, issuer, currency)
	if balance == nil {
		return 0.0
	}

	return balance.Float64()
}

// TrustLineExists checks if a trust line exists between two accounts for a currency.
func (e *TestEnv) TrustLineExists(acc1, acc2 *Account, currency string) bool {
	e.t.Helper()

	lineKey := keylet.Line(acc1.ID, acc2.ID, currency)
	exists, err := e.ledger.Exists(lineKey)
	if err != nil {
		e.t.Fatalf("Failed to check trust line existence: %v", err)
		return false
	}
	return exists
}

// TrustLineFlags returns the flags on a trust line between two accounts.
// Returns the flags from the perspective of 'account' (which side's flags).
func (e *TestEnv) TrustLineFlags(account, counterparty *Account, currency string) uint32 {
	e.t.Helper()

	lineKey := keylet.Line(account.ID, counterparty.ID, currency)
	exists, err := e.ledger.Exists(lineKey)
	if err != nil || !exists {
		return 0
	}

	data, err := e.ledger.Read(lineKey)
	if err != nil {
		return 0
	}

	rs, err := sle.ParseRippleState(data)
	if err != nil {
		return 0
	}

	return rs.Flags
}

// HasNoRipple checks if the account's side of the trust line has NoRipple set.
func (e *TestEnv) HasNoRipple(account, counterparty *Account, currency string) bool {
	e.t.Helper()

	lineKey := keylet.Line(account.ID, counterparty.ID, currency)
	exists, err := e.ledger.Exists(lineKey)
	if err != nil || !exists {
		return false
	}

	data, err := e.ledger.Read(lineKey)
	if err != nil {
		return false
	}

	rs, err := sle.ParseRippleState(data)
	if err != nil {
		return false
	}

	// Determine if account is low or high
	isLow := keylet.IsLowAccount(account.ID, counterparty.ID)

	if isLow {
		return (rs.Flags & sle.LsfLowNoRipple) != 0
	}
	return (rs.Flags & sle.LsfHighNoRipple) != 0
}

// Now returns the current time on the test clock.
func (e *TestEnv) Now() time.Time {
	return e.clock.Now()
}

// Seq returns the current sequence number for an account.
func (e *TestEnv) Seq(acc *Account) uint32 {
	e.t.Helper()

	// Get account keylet
	accountKey := keylet.Account(acc.ID)

	// Check if account exists
	exists, err := e.ledger.Exists(accountKey)
	if err != nil {
		e.t.Fatalf("Failed to check account existence: %v", err)
		return 1
	}
	if !exists {
		return 1 // Default sequence for new accounts
	}

	// Read account data
	data, err := e.ledger.Read(accountKey)
	if err != nil {
		e.t.Fatalf("Failed to read account: %v", err)
		return 1
	}

	// Parse account root to get sequence
	accountRoot, err := sle.ParseAccountRootFromBytes(data)
	if err != nil {
		e.t.Fatalf("Failed to parse account data: %v", err)
		return 1
	}

	return accountRoot.Sequence
}

// Ledger returns the current open ledger.
func (e *TestEnv) Ledger() *ledger.Ledger {
	return e.ledger
}

// LedgerSeq returns the current ledger sequence number.
func (e *TestEnv) LedgerSeq() uint32 {
	return e.ledger.Sequence()
}

// GetAccount returns a registered account by name.
func (e *TestEnv) GetAccount(name string) *Account {
	return e.accounts[name]
}

// AdvanceTime advances the test clock by the specified duration.
func (e *TestEnv) AdvanceTime(d time.Duration) {
	e.clock.Advance(d)
}

// SetTime sets the test clock to a specific time.
func (e *TestEnv) SetTime(t time.Time) {
	e.clock.Set(t)
}

// Exists checks if an account exists in the ledger.
func (e *TestEnv) Exists(acc *Account) bool {
	e.t.Helper()

	accountKey := keylet.Account(acc.ID)
	exists, err := e.ledger.Exists(accountKey)
	if err != nil {
		e.t.Fatalf("Failed to check account existence: %v", err)
		return false
	}
	return exists
}

// AccountInfo returns detailed account information.
func (e *TestEnv) AccountInfo(acc *Account) *AccountInfo {
	e.t.Helper()

	accountKey := keylet.Account(acc.ID)

	exists, err := e.ledger.Exists(accountKey)
	if err != nil || !exists {
		return nil
	}

	data, err := e.ledger.Read(accountKey)
	if err != nil {
		return nil
	}

	accountRoot, err := sle.ParseAccountRootFromBytes(data)
	if err != nil {
		return nil
	}

	return &AccountInfo{
		Address:    acc.Address,
		Balance:    accountRoot.Balance,
		Sequence:   accountRoot.Sequence,
		OwnerCount: accountRoot.OwnerCount,
		Flags:      accountRoot.Flags,
	}
}

// AccountInfo contains account information from the ledger.
type AccountInfo struct {
	Address    string
	Balance    uint64
	Sequence   uint32
	OwnerCount uint32
	Flags      uint32
}

// MasterAccount returns the master account for the test environment.
func (e *TestEnv) MasterAccount() *Account {
	return e.accounts["master"]
}

// BaseFee returns the base fee in drops.
func (e *TestEnv) BaseFee() uint64 {
	return e.baseFee
}

// ReserveBase returns the base reserve in drops.
func (e *TestEnv) ReserveBase() uint64 {
	return e.reserveBase
}

// ReserveIncrement returns the reserve increment in drops.
func (e *TestEnv) ReserveIncrement() uint64 {
	return e.reserveIncrement
}

// DecodeAddress decodes an XRPL address to a 20-byte account ID.
func DecodeAddress(address string) ([20]byte, error) {
	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(address)
	if err != nil {
		return [20]byte{}, err
	}

	var accountID [20]byte
	copy(accountID[:], accountIDBytes)
	return accountID, nil
}

// formatUint64 formats a uint64 as a string (for XRP amounts).
func formatUint64(n uint64) string {
	// Simple conversion without importing strconv
	if n == 0 {
		return "0"
	}

	digits := make([]byte, 0, 20)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}

	// Reverse
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}

	return string(digits)
}
