// Package payment contains integration tests for payment behavior.
// Tests ported from rippled's DepositAuth_test.cpp
package payment

import (
	"testing"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	xrplgoTesting "github.com/LeJamon/goXRPLd/internal/testing"
	"github.com/LeJamon/goXRPLd/internal/testing/trustset"
	"github.com/stretchr/testify/require"
)

// TestDepositAuth_Enable tests enabling and disabling DepositAuth flag.
// From rippled: testEnable
func TestDepositAuth_Enable(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Get initial account flags
	initialBalance := env.Balance(alice)

	// alice enables DepositAuth via AccountSet
	env.EnableDepositAuth(alice)
	env.Close()

	// Verify account still exists and balance reduced by fee
	balanceAfterEnable := env.Balance(alice)
	require.Less(t, balanceAfterEnable, initialBalance,
		"Balance should decrease due to AccountSet fee")

	// alice disables DepositAuth via AccountSet
	env.DisableDepositAuth(alice)
	env.Close()

	// Verify account still exists and balance reduced again
	finalBalance := env.Balance(alice)
	require.Less(t, finalBalance, balanceAfterEnable,
		"Balance should decrease due to second AccountSet fee")

	t.Log("DepositAuth enable/disable test passed")
}

// TestDepositAuth_PayIOU tests IOU payments with DepositAuth.
// From rippled: testPayIOU
func TestDepositAuth_PayIOU(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")
	gw := xrplgoTesting.NewAccount("gateway")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Set up trust lines
	result := env.Submit(trustset.TrustLine(alice, "USD", gw, "1000").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	result = env.Submit(trustset.TrustLine(bob, "USD", gw, "1000").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Fund alice with USD
	usd150 := tx.NewIssuedAmountFromFloat64(150, "USD", gw.Address)
	result = env.Submit(PayIssued(gw, alice, usd150).Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// alice pays bob some USD to set up initial balance
	usd50 := tx.NewIssuedAmountFromFloat64(50, "USD", gw.Address)
	result = env.Submit(PayIssued(alice, bob, usd50).Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// bob enables DepositAuth
	env.EnableDepositAuth(bob)
	env.Close()

	// alice tries to pay bob USD - should fail with tecNO_PERMISSION
	result = env.Submit(PayIssued(alice, bob, usd50).Build())
	require.Equal(t, "tecNO_PERMISSION", result.Code,
		"IOU payment to DepositAuth account should fail with tecNO_PERMISSION")

	// bob can still make payments to others while DepositAuth is set
	usd25 := tx.NewIssuedAmountFromFloat64(25, "USD", gw.Address)
	result = env.Submit(PayIssued(bob, alice, usd25).Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// bob clears DepositAuth
	env.DisableDepositAuth(bob)
	env.Close()

	// alice can now pay bob
	result = env.Submit(PayIssued(alice, bob, usd50).Build())
	xrplgoTesting.RequireTxSuccess(t, result)

	t.Log("DepositAuth IOU test passed")
}

// TestDepositAuth_PayXRP tests XRP payments with DepositAuth.
// From rippled: testPayXRP
func TestDepositAuth_PayXRP(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// bob enables DepositAuth
	env.EnableDepositAuth(bob)
	env.Close()

	// bob has more XRP than base reserve - any payment should fail
	result := env.Submit(Pay(alice, bob, 1_000_000).Build())
	require.Equal(t, "tecNO_PERMISSION", result.Code,
		"XRP payment to DepositAuth account above reserve should fail")

	// bob clears DepositAuth
	env.DisableDepositAuth(bob)
	env.Close()

	// alice can now pay bob
	result = env.Submit(Pay(alice, bob, 1_000_000).Build())
	xrplgoTesting.RequireTxSuccess(t, result)

	t.Log("DepositAuth XRP test passed")
}

// TestDepositAuth_PayXRP_AtReserve tests the special XRP payment allowance at reserve.
// From rippled: testPayXRP (reserve edge cases)
// When an account with DepositAuth is at or below the base reserve, small payments are allowed
// to prevent the account from becoming permanently locked.
func TestDepositAuth_PayXRP_AtReserve(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	// Fund alice with plenty of XRP
	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	// Fund bob with just the minimum reserve
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10)))
	env.Close()

	// bob enables DepositAuth
	env.EnableDepositAuth(bob)
	env.Close()

	// bob now has balance <= reserve (after AccountSet fee)
	bobBalance := env.Balance(bob)
	t.Logf("Bob's balance after enabling DepositAuth: %d drops", bobBalance)

	// Small XRP payments should succeed because bob is at/below reserve
	// This is the special exception in rippled to prevent account wedging
	result := env.Submit(Pay(alice, bob, 1_000_000).Build())
	xrplgoTesting.RequireTxSuccess(t, result)

	t.Log("DepositAuth XRP at reserve test passed - small payment succeeded")
}

// TestDepositAuth_NoRipple tests DepositAuth interaction with NoRipple flag.
// From rippled: testNoRipple
func TestDepositAuth_NoRipple(t *testing.T) {
	t.Skip("TODO: DepositAuth+NoRipple requires path payment support")

	// Test various combinations of NoRipple and DepositAuth flags
	// DepositAuth should not affect rippling behavior
	// NoRipple controls whether funds can ripple through an account

	t.Log("DepositAuth NoRipple test: requires path payment support")
}

// TestDepositPreauth_Enable tests DepositPreauth creation and deletion.
// From rippled: DepositPreauth_test::testEnable
func TestDepositPreauth_Enable(t *testing.T) {
	t.Skip("TODO: DepositPreauth requires DepositPreauth transaction type")

	t.Log("DepositPreauth enable test: requires DepositPreauth transaction type")
}

// TestDepositPreauth_Invalid tests invalid DepositPreauth operations.
// From rippled: DepositPreauth_test::testInvalid
func TestDepositPreauth_Invalid(t *testing.T) {
	t.Skip("TODO: DepositPreauth requires DepositPreauth transaction type")

	t.Log("DepositPreauth invalid test: requires DepositPreauth transaction type")
}

// TestDepositPreauth_Payment tests payments with DepositPreauth.
// From rippled: DepositPreauth_test::testPayment
func TestDepositPreauth_Payment(t *testing.T) {
	t.Skip("TODO: DepositPreauth requires DepositPreauth transaction type and AccountSet")

	t.Log("DepositPreauth payment test: requires DepositPreauth and AccountSet")
}

// TestDepositPreauth_SelfPayment tests self-payment with DepositAuth.
// From rippled: DepositPreauth_test::testPayment (self-payment section)
func TestDepositPreauth_SelfPayment(t *testing.T) {
	t.Skip("TODO: DepositPreauth self-payment requires IOU and AccountSet support")

	t.Log("DepositPreauth self-payment test: requires offer and AccountSet support")
}

// TestDepositPreauth_Credentials tests DepositPreauth with credentials.
// From rippled: DepositPreauth_test with credentials
func TestDepositPreauth_Credentials(t *testing.T) {
	t.Skip("TODO: Credentials require Credentials feature support")

	t.Log("DepositPreauth credentials test: requires Credentials feature")
}

// TestDepositPreauth_ExpiredCredentials tests DepositPreauth with expired credentials.
// From rippled: DepositPreauth_test::testExpiredCreds
func TestDepositPreauth_ExpiredCredentials(t *testing.T) {
	t.Skip("TODO: Expired credentials require Credentials feature support")

	t.Log("DepositPreauth expired credentials test: requires Credentials feature")
}
