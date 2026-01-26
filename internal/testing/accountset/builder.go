package accountset

import (
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	accounttx "github.com/LeJamon/goXRPLd/internal/core/tx/account"
	"github.com/LeJamon/goXRPLd/internal/testing"
)

// AccountSetBuilder provides a fluent interface for building AccountSet transactions.
type AccountSetBuilder struct {
	account      *testing.Account
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
func AccountSet(account *testing.Account) *AccountSetBuilder {
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
	flag := accounttx.AccountSetFlagRequireDest
	b.setFlag = &flag
	return b
}

// RequireAuth enables the require authorization flag.
func (b *AccountSetBuilder) RequireAuth() *AccountSetBuilder {
	flag := accounttx.AccountSetFlagRequireAuth
	b.setFlag = &flag
	return b
}

// DisallowXRP enables the disallow XRP flag.
func (b *AccountSetBuilder) DisallowXRP() *AccountSetBuilder {
	flag := accounttx.AccountSetFlagDisallowXRP
	b.setFlag = &flag
	return b
}

// DefaultRipple enables the default ripple flag.
func (b *AccountSetBuilder) DefaultRipple() *AccountSetBuilder {
	flag := accounttx.AccountSetFlagDefaultRipple
	b.setFlag = &flag
	return b
}

// DepositAuth enables the deposit authorization flag.
func (b *AccountSetBuilder) DepositAuth() *AccountSetBuilder {
	flag := accounttx.AccountSetFlagDepositAuth
	b.setFlag = &flag
	return b
}

// NoFreeze enables the no freeze flag (cannot be disabled once set).
func (b *AccountSetBuilder) NoFreeze() *AccountSetBuilder {
	flag := accounttx.AccountSetFlagNoFreeze
	b.setFlag = &flag
	return b
}

// GlobalFreeze enables the global freeze flag.
func (b *AccountSetBuilder) GlobalFreeze() *AccountSetBuilder {
	flag := accounttx.AccountSetFlagGlobalFreeze
	b.setFlag = &flag
	return b
}

// AllowClawback enables the clawback flag (cannot be disabled once set).
func (b *AccountSetBuilder) AllowClawback() *AccountSetBuilder {
	flag := accounttx.AccountSetFlagAllowTrustLineClawback
	b.setFlag = &flag
	return b
}

// Build constructs the AccountSet transaction.
func (b *AccountSetBuilder) Build() tx.Transaction {
	as := accounttx.NewAccountSet(b.account.Address)
	as.Fee = fmt.Sprintf("%d", b.fee)

	if b.setFlag != nil {
		as.SetFlag = b.setFlag
	}
	if b.clearFlag != nil {
		as.ClearFlag = b.clearFlag
	}
	if b.domain != "" {
		as.Domain = b.domain
	}
	if b.transferRate != nil {
		as.TransferRate = b.transferRate
	}
	if b.tickSize != nil {
		as.TickSize = b.tickSize
	}
	if b.sequence != nil {
		as.SetSequence(*b.sequence)
	}
	if b.flags != 0 {
		as.SetFlags(b.flags)
	}

	return as
}

// BuildAccountSet is a convenience method that returns the concrete *accounttx.AccountSet type.
func (b *AccountSetBuilder) BuildAccountSet() *accounttx.AccountSet {
	return b.Build().(*accounttx.AccountSet)
}
