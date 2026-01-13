package builders

import (
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
)

// Account represents a test account with its address and optional metadata.
// This is a simplified representation for building test transactions.
type Account struct {
	// Address is the r-address of the account (e.g., "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")
	Address string

	// Sequence is the next sequence number for the account (optional)
	Sequence uint32

	// Balance in drops (optional, for test tracking)
	Balance uint64
}

// NewAccount creates a new test account with the given address.
func NewAccount(address string) *Account {
	return &Account{
		Address:  address,
		Sequence: 1, // Default starting sequence
	}
}

// NewAccountWithSeq creates a new test account with the given address and sequence.
func NewAccountWithSeq(address string, sequence uint32) *Account {
	return &Account{
		Address:  address,
		Sequence: sequence,
	}
}

// NextSeq returns the current sequence and increments it for the next use.
func (a *Account) NextSeq() uint32 {
	seq := a.Sequence
	a.Sequence++
	return seq
}

// Well-known test accounts from rippled
var (
	// Genesis account - the well-known genesis account with all initial XRP
	Genesis = NewAccount("rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")

	// Alice - commonly used test account
	Alice = NewAccount("rN7n3473SaZBCG4dFL83w7a1RXtXtbk2D9")

	// Bob - commonly used test account
	Bob = NewAccount("rPMh7Pi9ct699iZUTWaytJUoHcJ7cgyziK")

	// Carol - commonly used test account
	Carol = NewAccount("rH4KEcG9dEwGwpn6AyoWK9cZPLL4RLSmWW")

	// Dave - commonly used test account
	Dave = NewAccount("rG1QQv2nh2gr7RCZ1P8YYcBUKCCN633jCn")

	// Gateway - commonly used as an issuer for test currencies
	Gateway = NewAccount("rGWrZyQqhTp9Xu7G5Pkayo7bXjH4k4QYpf")
)

// AccountSetBuilder provides a fluent interface for building AccountSet transactions.
type AccountSetBuilder struct {
	account      *Account
	setFlag      *uint32
	clearFlag    *uint32
	domain       string
	transferRate *uint32
	tickSize     *uint8
	fee          uint64
	sequence     *uint32
	flags        uint32
}

// AccountSet creates a new AccountSetBuilder.
func AccountSet(account *Account) *AccountSetBuilder {
	return &AccountSetBuilder{
		account: account,
		fee:     10, // Default fee: 10 drops
	}
}

// SetFlag sets a flag to enable on the account.
func (b *AccountSetBuilder) SetFlag(flag uint32) *AccountSetBuilder {
	b.setFlag = &flag
	return b
}

// ClearFlag sets a flag to disable on the account.
func (b *AccountSetBuilder) ClearFlag(flag uint32) *AccountSetBuilder {
	b.clearFlag = &flag
	return b
}

// Domain sets the domain for the account (as hex-encoded string).
func (b *AccountSetBuilder) Domain(domain string) *AccountSetBuilder {
	b.domain = domain
	return b
}

// TransferRate sets the transfer rate (1e9 = 100%, 1.005e9 = 100.5%).
func (b *AccountSetBuilder) TransferRate(rate uint32) *AccountSetBuilder {
	b.transferRate = &rate
	return b
}

// TickSize sets the tick size for offers (0, 3-15).
func (b *AccountSetBuilder) TickSize(size uint8) *AccountSetBuilder {
	b.tickSize = &size
	return b
}

// Fee sets the transaction fee in drops.
func (b *AccountSetBuilder) Fee(f uint64) *AccountSetBuilder {
	b.fee = f
	return b
}

// Sequence sets the sequence number explicitly.
func (b *AccountSetBuilder) Sequence(seq uint32) *AccountSetBuilder {
	b.sequence = &seq
	return b
}

// RequireDest enables the require destination tag flag.
func (b *AccountSetBuilder) RequireDest() *AccountSetBuilder {
	flag := tx.AccountSetFlagRequireDest
	b.setFlag = &flag
	return b
}

// RequireAuth enables the require authorization flag.
func (b *AccountSetBuilder) RequireAuth() *AccountSetBuilder {
	flag := tx.AccountSetFlagRequireAuth
	b.setFlag = &flag
	return b
}

// DisallowXRP enables the disallow XRP flag.
func (b *AccountSetBuilder) DisallowXRP() *AccountSetBuilder {
	flag := tx.AccountSetFlagDisallowXRP
	b.setFlag = &flag
	return b
}

// DefaultRipple enables the default ripple flag.
func (b *AccountSetBuilder) DefaultRipple() *AccountSetBuilder {
	flag := tx.AccountSetFlagDefaultRipple
	b.setFlag = &flag
	return b
}

// DepositAuth enables the deposit authorization flag.
func (b *AccountSetBuilder) DepositAuth() *AccountSetBuilder {
	flag := tx.AccountSetFlagDepositAuth
	b.setFlag = &flag
	return b
}

// NoFreeze enables the no freeze flag (cannot be disabled once set).
func (b *AccountSetBuilder) NoFreeze() *AccountSetBuilder {
	flag := tx.AccountSetFlagNoFreeze
	b.setFlag = &flag
	return b
}

// GlobalFreeze enables the global freeze flag.
func (b *AccountSetBuilder) GlobalFreeze() *AccountSetBuilder {
	flag := tx.AccountSetFlagGlobalFreeze
	b.setFlag = &flag
	return b
}

// AllowClawback enables the clawback flag (cannot be disabled once set).
func (b *AccountSetBuilder) AllowClawback() *AccountSetBuilder {
	flag := tx.AccountSetFlagAllowTrustLineClawback
	b.setFlag = &flag
	return b
}

// Build constructs the AccountSet transaction.
func (b *AccountSetBuilder) Build() tx.Transaction {
	accountSet := tx.NewAccountSet(b.account.Address)
	accountSet.Fee = fmt.Sprintf("%d", b.fee)

	if b.setFlag != nil {
		accountSet.SetFlag = b.setFlag
	}
	if b.clearFlag != nil {
		accountSet.ClearFlag = b.clearFlag
	}
	if b.domain != "" {
		accountSet.Domain = b.domain
	}
	if b.transferRate != nil {
		accountSet.TransferRate = b.transferRate
	}
	if b.tickSize != nil {
		accountSet.TickSize = b.tickSize
	}
	if b.sequence != nil {
		accountSet.SetSequence(*b.sequence)
	}
	if b.flags != 0 {
		accountSet.SetFlags(b.flags)
	}

	return accountSet
}

// BuildAccountSet is a convenience method that returns the concrete *tx.AccountSet type.
func (b *AccountSetBuilder) BuildAccountSet() *tx.AccountSet {
	return b.Build().(*tx.AccountSet)
}
