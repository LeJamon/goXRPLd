// Package payment contains integration tests for trust line behavior.
// Tests ported from rippled's SetTrust_test.cpp
package payment

import (
	"testing"

	xrplgoTesting "github.com/LeJamon/goXRPLd/internal/testing"
	"github.com/LeJamon/goXRPLd/internal/testing/trustset"
	"github.com/stretchr/testify/require"
)

// TestSetTrust_TrustLineDelete tests deletion of trust lines by setting limit to zero.
// From rippled: SetTrust_test::testTrustLineDelete
func TestSetTrust_TrustLineDelete(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	becky := xrplgoTesting.NewAccount("becky")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(becky, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// becky wants to hold at most 50 tokens of alice["USD"]
	result := env.Submit(trustset.TrustLine(becky, "USD", alice, "50").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Verify trust line exists
	exists := env.TrustLineExists(becky, alice, "USD")
	require.True(t, exists, "Trust line should exist after creation")

	// Reset the trust line limit to zero
	result = env.Submit(trustset.TrustLine(becky, "USD", alice, "0").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Verify trust line is deleted
	exists = env.TrustLineExists(becky, alice, "USD")
	require.False(t, exists, "Trust line should be deleted when limit set to 0")

	t.Log("SetTrust trust line delete test passed")
}

// TestSetTrust_ResetWithAuthFlag tests trust line reset with authorized flag.
// From rippled: SetTrust_test::testTrustLineResetWithAuthFlag
func TestSetTrust_ResetWithAuthFlag(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	becky := xrplgoTesting.NewAccount("becky")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(becky, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// alice wants authorized trust lines
	env.EnableRequireAuth(alice)
	env.Close()

	// becky creates a trust line
	result := env.Submit(trustset.TrustLine(becky, "USD", alice, "50").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// alice authorizes becky (using tfSetfAuth flag on TrustSet)
	authTrustLine := trustset.TrustLine(alice, "USD", becky, "0")
	authTrustLine.SetAuth()
	result = env.Submit(authTrustLine.Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Verify trust line exists
	exists := env.TrustLineExists(becky, alice, "USD")
	require.True(t, exists, "Trust line should exist")

	// Reset the trust line limit to zero
	result = env.Submit(trustset.TrustLine(becky, "USD", alice, "0").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Trust line should be deleted despite authorization
	exists = env.TrustLineExists(becky, alice, "USD")
	require.False(t, exists, "Trust line should be deleted even when authorized")

	t.Log("SetTrust reset with auth flag test passed")
}

// TestSetTrust_FreeTrustlines tests free trustline reserve scenarios.
// From rippled: SetTrust_test::testFreeTrustlines
func TestSetTrust_FreeTrustlines(t *testing.T) {
	t.Skip("TODO: Free trustlines requires dynamic reserve calculation")

	t.Log("SetTrust free trustlines test: requires reserve calculation")
}

// TestSetTrust_UsingTicket tests trust line creation with ticket.
// From rippled: SetTrust_test::testUsingTicket
func TestSetTrust_UsingTicket(t *testing.T) {
	t.Skip("TODO: TrustSet with ticket requires Ticket support")

	t.Log("SetTrust using ticket test: requires Ticket support")
}

// TestSetTrust_Malformed tests malformed TrustSet transactions.
// From rippled: SetTrust_test::testMalformed
func TestSetTrust_Malformed(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Trust line to self should fail
	result := env.Submit(trustset.TrustLine(alice, "USD", alice, "1000").Build())
	require.Contains(t, result.Code, "tem",
		"Trust line to self should fail with tem* error")

	// Trust line with XRP currency should fail
	xrpTrustLine := trustset.TrustLine(alice, "XRP", bob, "1000")
	result = env.Submit(xrpTrustLine.Build())
	require.Contains(t, result.Code, "tem",
		"Trust line with XRP currency should fail with tem* error")

	t.Log("SetTrust malformed test passed")
}

// TestSetTrust_TrustNonexistentAccount tests trust line to non-existent account.
// From rippled: TrustAndBalance_test::testTrustNonexistentAccount
func TestSetTrust_TrustNonexistentAccount(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	unfunded := xrplgoTesting.NewAccount("unfunded")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Trust line to non-existent account should fail with tecNO_DST
	result := env.Submit(trustset.TrustLine(alice, "USD", unfunded, "1000").Build())
	require.Equal(t, "tecNO_DST", result.Code,
		"Trust line to non-existent account should fail with tecNO_DST")

	t.Log("SetTrust to non-existent account test passed")
}

// TestSetTrust_DisallowIncoming tests trust lines with disallow incoming flag.
// From rippled: SetTrust_test::testDisallowIncoming
func TestSetTrust_DisallowIncoming(t *testing.T) {
	t.Skip("TODO: DisallowIncoming requires featureDisallowIncoming amendment support")

	t.Log("SetTrust disallow incoming test: requires amendment support")
}

// TestSetTrust_PaymentsWithPathsAndFees tests payments with paths and fees.
// From rippled: TrustAndBalance_test::testPaymentsWithPathsAndFees
func TestSetTrust_PaymentsWithPathsAndFees(t *testing.T) {
	t.Skip("TODO: Payments with paths and fees requires path finding and transfer rate")

	t.Log("SetTrust payments with paths and fees test: requires path support")
}

// TestSetTrust_InvoiceID tests setting invoice ID on payment.
// From rippled: TrustAndBalance_test::testInvoiceID
func TestSetTrust_InvoiceID(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Payment with invoice ID
	invoiceID := "0000000000000000000000000000000000000000000000000000000000000001"
	payTx := Pay(alice, bob, 100_000_000).InvoiceID(invoiceID).Build()
	result := env.Submit(payTx)
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	t.Log("SetTrust invoice ID test passed")
}

// TestSetTrust_NegativeLimit tests that negative limit is rejected.
// From rippled: SetTrust validation
func TestSetTrust_NegativeLimit(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	gw := xrplgoTesting.NewAccount("gateway")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Trust line with negative limit should fail
	result := env.Submit(trustset.TrustLine(alice, "USD", gw, "-100").Build())
	// Should fail with tem* error (malformed)
	require.Contains(t, result.Code, "tem",
		"Negative trust line limit should fail")

	t.Log("SetTrust negative limit test passed")
}

// TestSetTrust_QualityInOut tests QualityIn and QualityOut fields.
// From rippled: trust quality settings
func TestSetTrust_QualityInOut(t *testing.T) {
	t.Skip("TODO: QualityIn/QualityOut requires quality field support in TrustSet")

	t.Log("SetTrust quality in/out test: requires quality support")
}

// TestSetTrust_NoRippleFlag tests NoRipple flag on trust lines.
// From rippled: NoRipple trust line flag
func TestSetTrust_NoRippleFlag(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	gw := xrplgoTesting.NewAccount("gateway")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Create trust line with NoRipple flag
	trustLine := trustset.TrustLine(alice, "USD", gw, "1000")
	trustLine.NoRipple()
	result := env.Submit(trustLine.Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Verify trust line exists
	exists := env.TrustLineExists(alice, gw, "USD")
	require.True(t, exists, "Trust line should exist with NoRipple flag")

	t.Log("SetTrust NoRipple flag test passed")
}

// TestSetTrust_FreezeTrustLine tests freezing a trust line.
// From rippled: Freeze trust line via TrustSet
func TestSetTrust_FreezeTrustLine(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	gw := xrplgoTesting.NewAccount("gateway")
	alice := xrplgoTesting.NewAccount("alice")

	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// alice creates trust line to gw
	result := env.Submit(trustset.TrustLine(alice, "USD", gw, "1000").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// gw freezes alice's trust line
	freezeTrustLine := trustset.TrustLine(gw, "USD", alice, "0")
	freezeTrustLine.Freeze()
	result = env.Submit(freezeTrustLine.Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	t.Log("SetTrust freeze trust line test passed")
}
