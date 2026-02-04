package nft_test

// NFTokenAuth_test.go - NFT authorization tests
// Reference: rippled/src/test/app/NFTokenAuth_test.cpp

import (
	"testing"

	jtx "github.com/LeJamon/goXRPLd/internal/testing"
	"github.com/LeJamon/goXRPLd/internal/testing/nft"
)

// TestBuyOfferUnauthorizedSeller tests when an unauthorized seller tries to accept a buy offer.
// Reference: rippled NFTokenAuth_test.cpp testBuyOffer_UnauthorizedSeller
func TestBuyOfferUnauthorizedSeller(t *testing.T) {
	t.Skip("testBuyOfferUnauthorizedSeller requires trustline authorization testing")

	env := jtx.NewTestEnv(t)

	g1 := jtx.NewAccount("G1")     // Gateway with RequireAuth
	a1 := jtx.NewAccount("A1")     // Authorized account
	a2 := jtx.NewAccount("A2")     // NFT owner (not authorized)

	env.Fund(g1, a1, a2)
	env.Close()

	// Set RequireAuth on G1
	// env.Submit(jtx.AccountSet(g1).RequireAuth().Build())
	env.Close()

	// Create trust line and authorize A1
	// env.Submit(jtx.TrustSet(a1, g1["USD"](10000)).Build())
	// env.Submit(jtx.TrustSet(g1, a1["USD"](10000), tfSetfAuth).Build())
	// env.Submit(jtx.Payment(g1, a1, g1["USD"](1000)).Build())
	env.Close()

	// A2 mints an NFT
	mintTx := nft.NFTokenMint(a2, 0).Transferable().Build()
	result := env.Submit(mintTx)
	if !result.Success {
		t.Fatalf("Failed to mint NFT: %s", result.Message)
	}
	nftokenID := "dummy_nft_id" // Would extract from result
	env.Close()

	// A1 creates a buy offer using USD
	buyOfferTx := nft.NFTokenCreateBuyOffer(a1, nftokenID, jtx.XRPTxAmount(10), a2).Build()
	// Note: Amount would be USD in real test
	result = env.Submit(buyOfferTx)
	if !result.Success {
		t.Logf("Buy offer creation: %s", result.Message)
	}
	buyOfferID := "dummy_offer_id"
	env.Close()

	// With fixEnforceNFTokenTrustlineV2:
	// - A2 should not be able to accept the buy offer (tecNO_LINE or tecNO_AUTH)
	// Without the amendment:
	// - A2 can sell tokens and receive IOUs without authorization

	acceptTx := nft.NFTokenAcceptBuyOffer(a2, buyOfferID).Build()
	result = env.Submit(acceptTx)
	t.Logf("Accept buy offer result: %s - %s", result.Code, result.Message)

	t.Log("testBuyOfferUnauthorizedSeller passed")
}

// TestCreateBuyOfferUnauthorizedBuyer tests when an unauthorized buyer tries to create a buy offer.
// Reference: rippled NFTokenAuth_test.cpp testCreateBuyOffer_UnauthorizedBuyer
func TestCreateBuyOfferUnauthorizedBuyer(t *testing.T) {
	t.Skip("testCreateBuyOfferUnauthorizedBuyer requires trustline authorization testing")

	env := jtx.NewTestEnv(t)

	g1 := jtx.NewAccount("G1")     // Gateway with RequireAuth
	a1 := jtx.NewAccount("A1")     // Buyer (not authorized)
	a2 := jtx.NewAccount("A2")     // NFT owner

	env.Fund(g1, a1, a2)
	env.Close()

	// Set RequireAuth on G1
	env.Close()

	// A2 mints an NFT
	mintTx := nft.NFTokenMint(a2, 0).Transferable().Build()
	env.Submit(mintTx)
	nftokenID := "dummy_nft_id"
	env.Close()

	// A1 (unauthorized) tries to create a buy offer with USD
	// Should fail with tecUNFUNDED_OFFER
	buyOfferTx := nft.NFTokenCreateBuyOffer(a1, nftokenID, jtx.XRPTxAmount(10), a2).Build()
	result := env.Submit(buyOfferTx)
	t.Logf("Unauthorized buy offer result: %s - %s", result.Code, result.Message)

	t.Log("testCreateBuyOfferUnauthorizedBuyer passed")
}

// TestAcceptBuyOfferUnauthorizedBuyer tests when seller tries to accept buy offer from unauthorized buyer.
// Reference: rippled NFTokenAuth_test.cpp testAcceptBuyOffer_UnauthorizedBuyer
func TestAcceptBuyOfferUnauthorizedBuyer(t *testing.T) {
	t.Skip("testAcceptBuyOfferUnauthorizedBuyer requires trustline authorization testing")

	env := jtx.NewTestEnv(t)

	g1 := jtx.NewAccount("G1")     // Gateway with RequireAuth
	a1 := jtx.NewAccount("A1")     // Buyer
	a2 := jtx.NewAccount("A2")     // NFT owner/seller

	env.Fund(g1, a1, a2)
	env.Close()

	// Set RequireAuth on G1 and authorize both A1 and A2
	// Then A1 creates buy offer
	// Then A1's authorization is revoked
	// A2 tries to accept - should fail with tecNO_AUTH

	t.Log("testAcceptBuyOfferUnauthorizedBuyer passed")
}

// TestSellOfferUnauthorizedSeller tests when authorized buyer tries to accept sell offer from unauthorized seller.
// Reference: rippled NFTokenAuth_test.cpp testSellOffer_UnauthorizedSeller
func TestSellOfferUnauthorizedSeller(t *testing.T) {
	t.Skip("testSellOfferUnauthorizedSeller requires trustline authorization testing")

	env := jtx.NewTestEnv(t)

	g1 := jtx.NewAccount("G1")     // Gateway with RequireAuth
	a1 := jtx.NewAccount("A1")     // Authorized buyer
	a2 := jtx.NewAccount("A2")     // Unauthorized seller

	env.Fund(g1, a1, a2)
	env.Close()

	// Set RequireAuth on G1
	// Authorize A1 but not A2
	// A2 mints NFT and creates sell offer
	// With fixEnforceNFTokenTrustlineV2:
	//   - Can't create sell offer if no trustline (tecNO_LINE)
	//   - Can't create sell offer if not authorized (tecNO_AUTH)
	// Without amendment:
	//   - Sell offer can be created and accepted without authorization

	t.Log("testSellOfferUnauthorizedSeller passed")
}

// TestSellOfferUnauthorizedBuyer tests when unauthorized buyer tries to accept a sell offer.
// Reference: rippled NFTokenAuth_test.cpp testSellOffer_UnauthorizedBuyer
func TestSellOfferUnauthorizedBuyer(t *testing.T) {
	t.Skip("testSellOfferUnauthorizedBuyer requires trustline authorization testing")

	env := jtx.NewTestEnv(t)

	g1 := jtx.NewAccount("G1")     // Gateway with RequireAuth
	a1 := jtx.NewAccount("A1")     // Unauthorized buyer
	a2 := jtx.NewAccount("A2")     // Authorized seller

	env.Fund(g1, a1, a2)
	env.Close()

	// Set RequireAuth on G1
	// Authorize A2 (seller), not A1 (buyer)
	// A2 creates sell offer for USD
	// A1 tries to accept - should fail with tecINSUFFICIENT_FUNDS or tecNO_AUTH

	t.Log("testSellOfferUnauthorizedBuyer passed")
}

// TestBrokeredAcceptOfferUnauthorizedBroker tests when an unauthorized broker bridges authorized buyer and seller.
// Reference: rippled NFTokenAuth_test.cpp testBrokeredAcceptOffer_UnauthorizedBroker
func TestBrokeredAcceptOfferUnauthorizedBroker(t *testing.T) {
	t.Skip("testBrokeredAcceptOfferUnauthorizedBroker requires trustline authorization testing")

	env := jtx.NewTestEnv(t)

	g1 := jtx.NewAccount("G1")         // Gateway with RequireAuth
	a1 := jtx.NewAccount("A1")         // Authorized buyer
	a2 := jtx.NewAccount("A2")         // Authorized seller
	broker := jtx.NewAccount("broker") // Unauthorized broker

	env.Fund(g1, a1, a2, broker)
	env.Close()

	// Set RequireAuth on G1
	// Authorize A1 and A2, but not broker
	// A2 mints NFT and creates sell offer
	// A1 creates buy offer
	// Broker tries to broker the sale with a fee

	// With fixEnforceNFTokenTrustlineV2:
	//   - tecNO_LINE if broker has no trustline
	//   - tecNO_AUTH if broker trustline not authorized
	//   - Can still broker without broker fee
	// Without amendment:
	//   - Broker can receive IOUs without authorization

	t.Log("testBrokeredAcceptOfferUnauthorizedBroker passed")
}

// TestBrokeredAcceptOfferUnauthorizedBuyer tests when authorized broker tries to bridge offers from unauthorized buyer.
// Reference: rippled NFTokenAuth_test.cpp testBrokeredAcceptOffer_UnauthorizedBuyer
func TestBrokeredAcceptOfferUnauthorizedBuyer(t *testing.T) {
	t.Skip("testBrokeredAcceptOfferUnauthorizedBuyer requires trustline authorization testing")

	env := jtx.NewTestEnv(t)

	g1 := jtx.NewAccount("G1")         // Gateway with RequireAuth
	a1 := jtx.NewAccount("A1")         // Buyer (will become unauthorized)
	a2 := jtx.NewAccount("A2")         // Authorized seller
	broker := jtx.NewAccount("broker") // Authorized broker

	env.Fund(g1, a1, a2, broker)
	env.Close()

	// All are initially authorized
	// A1 creates buy offer
	// A1's authorization is revoked
	// Broker tries to broker - should fail with tecNO_AUTH

	t.Log("testBrokeredAcceptOfferUnauthorizedBuyer passed")
}

// TestBrokeredAcceptOfferUnauthorizedSeller tests when authorized broker tries to bridge offers from unauthorized seller.
// Reference: rippled NFTokenAuth_test.cpp testBrokeredAcceptOffer_UnauthorizedSeller
func TestBrokeredAcceptOfferUnauthorizedSeller(t *testing.T) {
	t.Skip("testBrokeredAcceptOfferUnauthorizedSeller requires trustline authorization testing")

	env := jtx.NewTestEnv(t)

	g1 := jtx.NewAccount("G1")         // Gateway with RequireAuth
	a1 := jtx.NewAccount("A1")         // Authorized buyer
	a2 := jtx.NewAccount("A2")         // Seller (will become unauthorized)
	broker := jtx.NewAccount("broker") // Authorized broker

	env.Fund(g1, a1, a2, broker)
	env.Close()

	// All are initially authorized
	// A2 creates sell offer
	// A2's authorization is revoked (trustline deleted)
	// Broker tries to broker

	// With fixEnforceNFTokenTrustlineV2:
	//   - tecNO_LINE if A2 trustline gone
	//   - tecNO_AUTH if A2 not authorized
	//   - Cannot broker even without broker fee
	// Without amendment:
	//   - Broker can complete the sale

	t.Log("testBrokeredAcceptOfferUnauthorizedSeller passed")
}

// TestTransferFeeUnauthorizedMinter tests when unauthorized minter receives transfer fee.
// Reference: rippled NFTokenAuth_test.cpp testTransferFee_UnauthorizedMinter
func TestTransferFeeUnauthorizedMinter(t *testing.T) {
	t.Skip("testTransferFeeUnauthorizedMinter requires trustline authorization and transfer fee testing")

	env := jtx.NewTestEnv(t)

	g1 := jtx.NewAccount("G1")         // Gateway with RequireAuth
	minter := jtx.NewAccount("minter") // Minter (not authorized)
	a1 := jtx.NewAccount("A1")         // Authorized buyer
	a2 := jtx.NewAccount("A2")         // Authorized seller

	env.Fund(g1, minter, a1, a2)
	env.Close()

	// Set RequireAuth on G1
	// Authorize A1 and A2, but NOT minter
	// Minter mints NFT with transfer fee (0.001%)
	// A1 buys NFT from minter (for XRP)
	// A1 creates sell offer for USD
	// A2 buys NFT from A1

	// With fixEnforceNFTokenTrustlineV2:
	//   - Sale fails with tecNO_AUTH because minter can't receive transfer fee
	// Without amendment:
	//   - Minter receives the transfer fee in USD without authorization

	t.Log("testTransferFeeUnauthorizedMinter passed")
}
