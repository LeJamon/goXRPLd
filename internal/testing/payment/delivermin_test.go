// Package payment contains integration tests for payment behavior.
// Tests ported from rippled's DeliverMin_test.cpp
package payment

import (
	"testing"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	xrplgoTesting "github.com/LeJamon/goXRPLd/internal/testing"
	"github.com/LeJamon/goXRPLd/internal/testing/trustset"
)

// TestDeliverMin_WithoutPartialPayment tests that delivermin requires tfPartialPayment flag.
// From rippled: delivermin without tfPartialPayment should fail with temBAD_AMOUNT
func TestDeliverMin_WithoutPartialPayment(t *testing.T) {
	t.Skip("TODO: DeliverMin requires IOU payment support and Offer creation")

	env := xrplgoTesting.NewTestEnv(t)

	gw := xrplgoTesting.NewAccount("gateway")
	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Create trust lines
	result := env.Submit(trustset.TrustLine(alice, "USD", gw, "100").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	result = env.Submit(trustset.TrustLine(bob, "USD", gw, "100").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Try to pay with delivermin but without tfPartialPayment - should fail
	usdAmount := tx.NewIssuedAmountFromFloat64(10, "USD", gw.Address)
	payTx := PayIssued(alice, bob, usdAmount).Build()
	// Note: DeliverMin not set without tfPartialPayment flag

	result = env.Submit(payTx)
	// Should fail with temBAD_AMOUNT if DeliverMin is set without PartialPayment
	t.Log("DeliverMin without partial payment test: result", result.Code)
}

// TestDeliverMin_NegativeAmount tests that negative delivermin fails.
// From rippled: negative delivermin should fail with temBAD_AMOUNT
func TestDeliverMin_NegativeAmount(t *testing.T) {
	t.Skip("TODO: DeliverMin requires IOU payment support")

	env := xrplgoTesting.NewTestEnv(t)

	gw := xrplgoTesting.NewAccount("gateway")
	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Create trust lines
	result := env.Submit(trustset.TrustLine(alice, "USD", gw, "100").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	result = env.Submit(trustset.TrustLine(bob, "USD", gw, "100").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Try to pay with negative delivermin - should fail
	t.Log("DeliverMin negative amount test: negative delivermin should fail with temBAD_AMOUNT")
}

// TestDeliverMin_CurrencyMismatch tests that delivermin currency must match amount currency.
// From rippled: delivermin with different currency should fail with temBAD_AMOUNT
func TestDeliverMin_CurrencyMismatch(t *testing.T) {
	t.Skip("TODO: DeliverMin requires IOU payment support")

	env := xrplgoTesting.NewTestEnv(t)

	gw := xrplgoTesting.NewAccount("gateway")
	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Create trust lines
	result := env.Submit(trustset.TrustLine(alice, "USD", gw, "100").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	result = env.Submit(trustset.TrustLine(bob, "USD", gw, "100").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Try to pay USD with XRP delivermin - should fail
	t.Log("DeliverMin currency mismatch test: XRP delivermin for USD payment should fail")
}

// TestDeliverMin_IssuerMismatch tests that delivermin issuer must match amount issuer.
// From rippled: delivermin with different issuer should fail with temBAD_AMOUNT
func TestDeliverMin_IssuerMismatch(t *testing.T) {
	t.Skip("TODO: DeliverMin requires IOU payment support")

	env := xrplgoTesting.NewTestEnv(t)

	gw := xrplgoTesting.NewAccount("gateway")
	carol := xrplgoTesting.NewAccount("carol")
	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(carol, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Create trust lines
	result := env.Submit(trustset.TrustLine(alice, "USD", gw, "100").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	result = env.Submit(trustset.TrustLine(bob, "USD", gw, "100").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Try to pay gw/USD with carol/USD delivermin - should fail
	t.Log("DeliverMin issuer mismatch test: different issuer delivermin should fail")
}

// TestDeliverMin_ExceedsAmount tests that delivermin cannot exceed amount.
// From rippled: delivermin greater than amount should fail with temBAD_AMOUNT
func TestDeliverMin_ExceedsAmount(t *testing.T) {
	t.Skip("TODO: DeliverMin requires IOU payment support")

	env := xrplgoTesting.NewTestEnv(t)

	gw := xrplgoTesting.NewAccount("gateway")
	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Create trust lines
	result := env.Submit(trustset.TrustLine(alice, "USD", gw, "100").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	result = env.Submit(trustset.TrustLine(bob, "USD", gw, "100").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Try to pay 10 USD with delivermin of 15 USD - should fail
	t.Log("DeliverMin exceeds amount test: delivermin > amount should fail with temBAD_AMOUNT")
}

// TestDeliverMin_PathPartial tests partial payment via path with delivermin.
// From rippled: partial payment that doesn't meet delivermin should fail with tecPATH_PARTIAL
func TestDeliverMin_PathPartial(t *testing.T) {
	t.Skip("TODO: DeliverMin requires IOU payment support, paths, and offers")

	env := xrplgoTesting.NewTestEnv(t)

	gw := xrplgoTesting.NewAccount("gateway")
	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")
	carol := xrplgoTesting.NewAccount("carol")

	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(carol, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Create trust lines
	result := env.Submit(trustset.TrustLine(alice, "USD", gw, "100").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	result = env.Submit(trustset.TrustLine(bob, "USD", gw, "100").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	result = env.Submit(trustset.TrustLine(carol, "USD", gw, "100").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Pay carol some USD
	usd50 := tx.NewIssuedAmountFromFloat64(50, "USD", gw.Address)
	result = env.Submit(PayIssued(gw, carol, usd50).Build())
	xrplgoTesting.RequireTxSuccess(t, result)

	// Carol creates offer: XRP(5) for USD(5)
	// TODO: Offer creation not yet implemented

	// Alice tries to pay bob 10 USD via XRP path with delivermin of 7 USD
	// But only 5 USD is available via the offer
	// Should fail with tecPATH_PARTIAL
	t.Log("DeliverMin path partial test: requires offer creation support")
}

// TestDeliverMin_SelfPayment tests self-payment with delivermin.
// From rippled: alice can pay herself via offer, converting all available liquidity
func TestDeliverMin_SelfPayment(t *testing.T) {
	t.Skip("TODO: DeliverMin requires IOU payment support and offers")

	env := xrplgoTesting.NewTestEnv(t)

	gw := xrplgoTesting.NewAccount("gateway")
	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Create trust lines
	result := env.Submit(trustset.TrustLine(alice, "USD", gw, "1000").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	result = env.Submit(trustset.TrustLine(bob, "USD", gw, "1000").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Bob has 100 USD and creates offer: XRP(100) for USD(100)
	usd100 := tx.NewIssuedAmountFromFloat64(100, "USD", gw.Address)
	result = env.Submit(PayIssued(gw, bob, usd100).Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	// TODO: Bob creates offer

	// Alice can pay herself USD via XRP, converting all bob's liquidity
	t.Log("DeliverMin self-payment test: requires offer creation support")
}

// TestDeliverMin_MultipleOffers tests delivermin with multiple offers.
// From rippled: payment should consume multiple offers to meet delivermin
func TestDeliverMin_MultipleOffers(t *testing.T) {
	t.Skip("TODO: DeliverMin requires IOU payment support and offers")

	env := xrplgoTesting.NewTestEnv(t)

	gw := xrplgoTesting.NewAccount("gateway")
	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")
	carol := xrplgoTesting.NewAccount("carol")

	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(carol, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Create trust lines
	result := env.Submit(trustset.TrustLine(bob, "USD", gw, "1000").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	result = env.Submit(trustset.TrustLine(carol, "USD", gw, "1000").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Bob has 200 USD and creates 3 offers at different rates
	usd200 := tx.NewIssuedAmountFromFloat64(200, "USD", gw.Address)
	result = env.Submit(PayIssued(gw, bob, usd200).Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	// TODO: Bob creates offers:
	// - XRP(100) for USD(100)
	// - XRP(1000) for USD(100)
	// - XRP(10000) for USD(100) (not enough liquidity anyway)

	// Alice tries to send carol USD(200) via XRP path with delivermin(200)
	// With sendmax(1000), should fail with tecPATH_PARTIAL (can only get 100 USD)
	// With sendmax(1100), should succeed (consuming first two offers)
	t.Log("DeliverMin multiple offers test: requires offer creation support")
}

// TestDeliverMin_MultipleProviders tests delivermin with multiple liquidity providers.
// From rippled: payment should consume offers from multiple providers
func TestDeliverMin_MultipleProviders(t *testing.T) {
	t.Skip("TODO: DeliverMin requires IOU payment support and offers")

	env := xrplgoTesting.NewTestEnv(t)

	gw := xrplgoTesting.NewAccount("gateway")
	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")
	carol := xrplgoTesting.NewAccount("carol")
	dan := xrplgoTesting.NewAccount("dan")

	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(carol, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(dan, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Create trust lines
	result := env.Submit(trustset.TrustLine(bob, "USD", gw, "1000").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	result = env.Submit(trustset.TrustLine(carol, "USD", gw, "1000").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	result = env.Submit(trustset.TrustLine(dan, "USD", gw, "1000").Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Bob and dan each have 100 USD and create offers
	usd100 := tx.NewIssuedAmountFromFloat64(100, "USD", gw.Address)
	result = env.Submit(PayIssued(gw, bob, usd100).Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	result = env.Submit(PayIssued(gw, dan, usd100).Build())
	xrplgoTesting.RequireTxSuccess(t, result)
	// TODO: Both create offers

	// Alice sends carol USD via XRP, consuming both bob's and dan's offers
	t.Log("DeliverMin multiple providers test: requires offer creation support")
}
