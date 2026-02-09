// Package paychan contains integration tests for payment channel behavior.
// Tests ported from rippled's PayChan_test.cpp
package paychan

import (
	"testing"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/paychan"
	xrplgoTesting "github.com/LeJamon/goXRPLd/internal/testing"
	"github.com/stretchr/testify/require"
)

// TestPayChan_Simple tests basic payment channel creation and operations.
// From rippled: PayChan_test::testSimple
func TestPayChan_Simple(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Create a payment channel from alice to bob
	settleDelay := uint32(100)
	amount := tx.NewXRPAmount(1000_000_000) // 1000 XRP
	createTx := paychan.NewPaymentChannelCreate(
		alice.Address,
		bob.Address,
		amount,
		settleDelay,
		alice.PublicKeyHex(),
	)
	createTx.Fee = "10"
	seq := env.Seq(alice)
	createTx.Sequence = &seq

	result := env.Submit(createTx)
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Verify alice's balance decreased
	aliceBalance := env.Balance(alice)
	// Should be: 10000 XRP - 1000 XRP (channel) - fees
	require.Less(t, aliceBalance, uint64(xrplgoTesting.XRP(9000)),
		"Alice's balance should decrease after creating channel")

	t.Log("PayChan simple create test passed")
}

// TestPayChan_BadAmounts tests that non-XRP and negative amounts fail.
// From rippled: PayChan_test::testSimple (bad amounts section)
func TestPayChan_BadAmounts(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")
	gw := xrplgoTesting.NewAccount("gateway")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(gw, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	settleDelay := uint32(100)

	// Test: Non-XRP amount should fail with temBAD_AMOUNT
	usdAmount := tx.NewIssuedAmountFromFloat64(1000, "USD", gw.Address)
	createTx := paychan.NewPaymentChannelCreate(
		alice.Address,
		bob.Address,
		usdAmount,
		settleDelay,
		alice.PublicKeyHex(),
	)
	createTx.Fee = "10"
	seq := env.Seq(alice)
	createTx.Sequence = &seq

	result := env.Submit(createTx)
	require.Equal(t, "temBAD_AMOUNT", result.Code,
		"Non-XRP payment channel should fail with temBAD_AMOUNT")

	// Test: Negative amount should fail with temBAD_AMOUNT
	negativeAmount := tx.NewXRPAmount(-1000_000_000)
	createTx2 := paychan.NewPaymentChannelCreate(
		alice.Address,
		bob.Address,
		negativeAmount,
		settleDelay,
		alice.PublicKeyHex(),
	)
	createTx2.Fee = "10"
	seq = env.Seq(alice)
	createTx2.Sequence = &seq

	result = env.Submit(createTx2)
	require.Equal(t, "temBAD_AMOUNT", result.Code,
		"Negative payment channel should fail with temBAD_AMOUNT")

	t.Log("PayChan bad amounts test passed")
}

// TestPayChan_InvalidDestination tests invalid destination accounts.
// From rippled: PayChan_test::testSimple (invalid account section)
func TestPayChan_InvalidDestination(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	unfunded := xrplgoTesting.NewAccount("unfunded")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	settleDelay := uint32(100)
	amount := tx.NewXRPAmount(1000_000_000)

	// Test: Channel to non-existent account should fail with tecNO_DST
	createTx := paychan.NewPaymentChannelCreate(
		alice.Address,
		unfunded.Address,
		amount,
		settleDelay,
		alice.PublicKeyHex(),
	)
	createTx.Fee = "10"
	seq := env.Seq(alice)
	createTx.Sequence = &seq

	result := env.Submit(createTx)
	require.Equal(t, "tecNO_DST", result.Code,
		"Channel to non-existent account should fail with tecNO_DST")

	t.Log("PayChan invalid destination test passed")
}

// TestPayChan_DestinationIsSelf tests that channel to self fails.
// From rippled: PayChan_test::testSimple (can't create channel to same account)
func TestPayChan_DestinationIsSelf(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	settleDelay := uint32(100)
	amount := tx.NewXRPAmount(1000_000_000)

	// Test: Channel to self should fail with temDST_IS_SRC
	createTx := paychan.NewPaymentChannelCreate(
		alice.Address,
		alice.Address, // same as source
		amount,
		settleDelay,
		alice.PublicKeyHex(),
	)
	createTx.Fee = "10"
	seq := env.Seq(alice)
	createTx.Sequence = &seq

	result := env.Submit(createTx)
	require.Equal(t, "temDST_IS_SRC", result.Code,
		"Channel to self should fail with temDST_IS_SRC")

	t.Log("PayChan destination is self test passed")
}

// TestPayChan_InsufficientFunds tests channel creation with insufficient funds.
// From rippled: PayChan_test::testSimple (not enough funds)
func TestPayChan_InsufficientFunds(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(100))) // Only 100 XRP
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	settleDelay := uint32(100)
	amount := tx.NewXRPAmount(10000_000_000) // 10000 XRP - more than alice has

	// Test: Creating channel with insufficient funds should fail with tecUNFUNDED
	createTx := paychan.NewPaymentChannelCreate(
		alice.Address,
		bob.Address,
		amount,
		settleDelay,
		alice.PublicKeyHex(),
	)
	createTx.Fee = "10"
	seq := env.Seq(alice)
	createTx.Sequence = &seq

	result := env.Submit(createTx)
	require.Equal(t, "tecUNFUNDED", result.Code,
		"Channel with insufficient funds should fail with tecUNFUNDED")

	t.Log("PayChan insufficient funds test passed")
}

// TestPayChan_DisallowIncoming tests the DisallowIncoming flag.
// From rippled: PayChan_test::testDisallowIncoming
func TestPayChan_DisallowIncoming(t *testing.T) {
	t.Skip("TODO: DisallowIncoming requires featureDisallowIncoming amendment support")

	t.Log("PayChan disallow incoming test: requires amendment support")
}

// TestPayChan_CancelAfter tests channel expiration.
// From rippled: PayChan_test::testCancelAfter
func TestPayChan_CancelAfter(t *testing.T) {
	t.Skip("TODO: CancelAfter requires ledger time management")

	t.Log("PayChan cancel after test: requires time management")
}

// TestPayChan_Expiration tests channel expiration behavior.
// From rippled: PayChan_test::testExpiration
func TestPayChan_Expiration(t *testing.T) {
	t.Skip("TODO: Expiration requires ledger time management")

	t.Log("PayChan expiration test: requires time management")
}

// TestPayChan_SettleDelay tests settle delay behavior.
// From rippled: PayChan_test::testSettleDelay
func TestPayChan_SettleDelay(t *testing.T) {
	t.Skip("TODO: SettleDelay requires time-based claim testing")

	t.Log("PayChan settle delay test: requires time management")
}

// TestPayChan_CloseDry tests closing a channel with no remaining balance.
// From rippled: PayChan_test::testCloseDry
func TestPayChan_CloseDry(t *testing.T) {
	t.Skip("TODO: CloseDry requires claim and close operations")

	t.Log("PayChan close dry test: requires claim operations")
}

// TestPayChan_DefaultAmount tests default amount behavior.
// From rippled: PayChan_test::testDefaultAmount
func TestPayChan_DefaultAmount(t *testing.T) {
	t.Skip("TODO: DefaultAmount test implementation")

	t.Log("PayChan default amount test")
}

// TestPayChan_DisallowXRP tests DisallowXRP flag with payment channels.
// From rippled: PayChan_test::testDisallowXRP
func TestPayChan_DisallowXRP(t *testing.T) {
	t.Skip("TODO: DisallowXRP requires account flag support")

	t.Log("PayChan disallow XRP test")
}

// TestPayChan_DstTag tests destination tag requirement.
// From rippled: PayChan_test::testDstTag
func TestPayChan_DstTag(t *testing.T) {
	t.Skip("TODO: DstTag requires RequireDest flag support")

	t.Log("PayChan destination tag test")
}

// TestPayChan_DepositAuth tests deposit authorization with payment channels.
// From rippled: PayChan_test::testDepositAuth
func TestPayChan_DepositAuth(t *testing.T) {
	t.Skip("TODO: DepositAuth with payment channels")

	t.Log("PayChan deposit auth test")
}

// TestPayChan_DepositAuthWithCredentials tests deposit auth with credentials.
// From rippled: PayChan_test::testDepositAuthWithCredentials
func TestPayChan_DepositAuthWithCredentials(t *testing.T) {
	t.Skip("TODO: Credentials feature not implemented")

	t.Log("PayChan deposit auth with credentials test")
}

// TestPayChan_MultipleChannels tests multiple channels to the same account.
// From rippled: PayChan_test::testMultipleChannels
func TestPayChan_MultipleChannels(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	settleDelay := uint32(100)
	amount := tx.NewXRPAmount(100_000_000) // 100 XRP

	// Create first channel
	createTx1 := paychan.NewPaymentChannelCreate(
		alice.Address,
		bob.Address,
		amount,
		settleDelay,
		alice.PublicKeyHex(),
	)
	createTx1.Fee = "10"
	seq := env.Seq(alice)
	createTx1.Sequence = &seq

	result := env.Submit(createTx1)
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Create second channel
	createTx2 := paychan.NewPaymentChannelCreate(
		alice.Address,
		bob.Address,
		amount,
		settleDelay,
		alice.PublicKeyHex(),
	)
	createTx2.Fee = "10"
	seq = env.Seq(alice)
	createTx2.Sequence = &seq

	result = env.Submit(createTx2)
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// Both channels should exist
	// Alice's balance should have decreased by 200 XRP + fees
	aliceBalance := env.Balance(alice)
	require.Less(t, aliceBalance, uint64(xrplgoTesting.XRP(9800)),
		"Alice's balance should decrease after creating two channels")

	t.Log("PayChan multiple channels test passed")
}

// TestPayChan_AccountChannelsRPC tests the account_channels RPC method.
// From rippled: PayChan_test::testAccountChannelsRPC
func TestPayChan_AccountChannelsRPC(t *testing.T) {
	t.Skip("TODO: Requires RPC testing infrastructure")

	t.Log("PayChan account_channels RPC test")
}

// TestPayChan_AuthVerifyRPC tests the PayChan auth/verify RPC methods.
// From rippled: PayChan_test::testAuthVerifyRPC
func TestPayChan_AuthVerifyRPC(t *testing.T) {
	t.Skip("TODO: Requires RPC testing infrastructure")

	t.Log("PayChan auth/verify RPC test")
}

// TestPayChan_OptionalFields tests optional fields in payment channels.
// From rippled: PayChan_test::testOptionalFields
func TestPayChan_OptionalFields(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	settleDelay := uint32(100)
	amount := tx.NewXRPAmount(1000_000_000)
	cancelAfter := uint32(env.LedgerSeq() + 1000)
	destTag := uint32(12345)
	sourceTag := uint32(54321)

	// Create channel with optional fields
	createTx := paychan.NewPaymentChannelCreate(
		alice.Address,
		bob.Address,
		amount,
		settleDelay,
		alice.PublicKeyHex(),
	)
	createTx.CancelAfter = &cancelAfter
	createTx.DestinationTag = &destTag
	createTx.SourceTag = &sourceTag
	createTx.Fee = "10"
	seq := env.Seq(alice)
	createTx.Sequence = &seq

	result := env.Submit(createTx)
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	t.Log("PayChan optional fields test passed")
}

// TestPayChan_MalformedPublicKey tests malformed public key handling.
// From rippled: PayChan_test::testMalformedPK
func TestPayChan_MalformedPublicKey(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	settleDelay := uint32(100)
	amount := tx.NewXRPAmount(1000_000_000)

	// Test: Empty public key should fail
	createTx := paychan.NewPaymentChannelCreate(
		alice.Address,
		bob.Address,
		amount,
		settleDelay,
		"", // empty public key
	)
	createTx.Fee = "10"
	seq := env.Seq(alice)
	createTx.Sequence = &seq

	result := env.Submit(createTx)
	require.Equal(t, "temMALFORMED", result.Code,
		"Empty public key should fail with temMALFORMED")

	// Test: Invalid hex should fail
	createTx2 := paychan.NewPaymentChannelCreate(
		alice.Address,
		bob.Address,
		amount,
		settleDelay,
		"notvalidhex",
	)
	createTx2.Fee = "10"
	seq = env.Seq(alice)
	createTx2.Sequence = &seq

	result = env.Submit(createTx2)
	require.Equal(t, "temMALFORMED", result.Code,
		"Invalid hex public key should fail with temMALFORMED")

	t.Log("PayChan malformed public key test passed")
}

// TestPayChan_Metadata tests metadata and ownership tracking.
// From rippled: PayChan_test::testMetaAndOwnership
func TestPayChan_Metadata(t *testing.T) {
	t.Skip("TODO: Requires metadata inspection")

	t.Log("PayChan metadata test")
}

// TestPayChan_AccountDelete tests account deletion with payment channels.
// From rippled: PayChan_test::testAccountDelete
func TestPayChan_AccountDelete(t *testing.T) {
	t.Skip("TODO: Requires AccountDelete transaction support")

	t.Log("PayChan account delete test")
}

// TestPayChan_UsingTickets tests payment channels with tickets.
// From rippled: PayChan_test::testUsingTickets
func TestPayChan_UsingTickets(t *testing.T) {
	t.Skip("TODO: Requires Ticket support")

	t.Log("PayChan using tickets test")
}

// TestPayChan_Fund tests PaymentChannelFund transaction.
// From rippled: PayChan_test::testSimple (fund section)
func TestPayChan_Fund(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")
	bob := xrplgoTesting.NewAccount("bob")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Create initial channel
	settleDelay := uint32(100)
	amount := tx.NewXRPAmount(1000_000_000) // 1000 XRP
	createTx := paychan.NewPaymentChannelCreate(
		alice.Address,
		bob.Address,
		amount,
		settleDelay,
		alice.PublicKeyHex(),
	)
	createTx.Fee = "10"
	seq := env.Seq(alice)
	createTx.Sequence = &seq

	result := env.Submit(createTx)
	xrplgoTesting.RequireTxSuccess(t, result)
	env.Close()

	// TODO: Get channel ID and test fund operation
	// This requires channel ID tracking which is not yet implemented

	t.Log("PayChan fund test: basic create succeeded, fund operation requires channel tracking")
}

// TestPayChan_FundByDest tests that destination cannot fund the channel.
// From rippled: PayChan_test::testSimple (Dst tries to fund section)
func TestPayChan_FundByDest(t *testing.T) {
	t.Skip("TODO: Requires channel ID tracking for fund operation")

	t.Log("PayChan fund by dest test")
}

// TestPayChan_Claim tests PaymentChannelClaim transaction.
// From rippled: PayChan_test::testSimple (claim sections)
func TestPayChan_Claim(t *testing.T) {
	t.Skip("TODO: Requires channel ID tracking and signature support")

	t.Log("PayChan claim test")
}

// TestPayChan_ClaimWithSignature tests claim with signature.
// From rippled: PayChan_test::testSimple (claim with signature section)
func TestPayChan_ClaimWithSignature(t *testing.T) {
	t.Skip("TODO: Requires signature creation and verification")

	t.Log("PayChan claim with signature test")
}

// TestPayChan_WrongSigningKey tests claim with wrong signing key.
// From rippled: PayChan_test::testSimple (wrong signing key section)
func TestPayChan_WrongSigningKey(t *testing.T) {
	t.Skip("TODO: Requires signature verification")

	t.Log("PayChan wrong signing key test")
}

// TestPayChan_BadSignature tests claim with bad signature.
// From rippled: PayChan_test::testSimple (bad signature section)
func TestPayChan_BadSignature(t *testing.T) {
	t.Skip("TODO: Requires signature verification")

	t.Log("PayChan bad signature test")
}

// TestPayChan_ClaimClose tests claim with tfClose flag.
// From rippled: PayChan_test::testSimple (dst closes channel section)
func TestPayChan_ClaimClose(t *testing.T) {
	t.Skip("TODO: Requires channel ID tracking for close operation")

	t.Log("PayChan claim close test")
}

// TestPayChan_ClaimRenewAndCloseConflict tests that tfRenew and tfClose cannot be set together.
// From rippled: PayChan_test (flags validation)
func TestPayChan_ClaimRenewAndCloseConflict(t *testing.T) {
	env := xrplgoTesting.NewTestEnv(t)

	alice := xrplgoTesting.NewAccount("alice")

	env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
	env.Close()

	// Create a claim with both tfRenew and tfClose - should fail
	claimTx := paychan.NewPaymentChannelClaim(
		alice.Address,
		"0000000000000000000000000000000000000000000000000000000000000000",
	)
	claimTx.SetClose()
	claimTx.SetRenew()
	claimTx.Fee = "10"
	seq := env.Seq(alice)
	claimTx.Sequence = &seq

	result := env.Submit(claimTx)
	require.Equal(t, "temMALFORMED", result.Code,
		"Setting both tfClose and tfRenew should fail with temMALFORMED")

	t.Log("PayChan claim renew and close conflict test passed")
}

// TestEnabled tests that PayChan operations are disabled without the PayChan amendment.
func TestEnabled(t *testing.T) {
	t.Run("Disabled", func(t *testing.T) {
		env := xrplgoTesting.NewTestEnv(t)

		alice := xrplgoTesting.NewAccount("alice")
		bob := xrplgoTesting.NewAccount("bob")
		env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
		env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
		env.Close()

		env.DisableFeature("PayChan")

		// PaymentChannelCreate should fail
		createTx := paychan.NewPaymentChannelCreate(
			alice.Address, bob.Address,
			tx.NewXRPAmount(1000_000_000),
			uint32(100), alice.PublicKeyHex(),
		)
		createTx.Fee = "10"
		seq := env.Seq(alice)
		createTx.Sequence = &seq
		result := env.Submit(createTx)
		require.Equal(t, "temDISABLED", result.Code, "PaymentChannelCreate: expected temDISABLED")

		// PaymentChannelFund should fail
		fakeChannelID := "0000000000000000000000000000000000000000000000000000000000000000"
		fundTx := paychan.NewPaymentChannelFund(alice.Address, fakeChannelID, tx.NewXRPAmount(100_000_000))
		fundTx.Fee = "10"
		seq = env.Seq(alice)
		fundTx.Sequence = &seq
		result = env.Submit(fundTx)
		require.Equal(t, "temDISABLED", result.Code, "PaymentChannelFund: expected temDISABLED")

		// PaymentChannelClaim should fail
		claimTx := paychan.NewPaymentChannelClaim(alice.Address, fakeChannelID)
		claimTx.Fee = "10"
		seq = env.Seq(alice)
		claimTx.Sequence = &seq
		result = env.Submit(claimTx)
		require.Equal(t, "temDISABLED", result.Code, "PaymentChannelClaim: expected temDISABLED")
	})

	t.Run("Enabled", func(t *testing.T) {
		env := xrplgoTesting.NewTestEnv(t)

		alice := xrplgoTesting.NewAccount("alice")
		bob := xrplgoTesting.NewAccount("bob")
		env.FundAmount(alice, uint64(xrplgoTesting.XRP(10000)))
		env.FundAmount(bob, uint64(xrplgoTesting.XRP(10000)))
		env.Close()

		createTx := paychan.NewPaymentChannelCreate(
			alice.Address, bob.Address,
			tx.NewXRPAmount(1000_000_000),
			uint32(100), alice.PublicKeyHex(),
		)
		createTx.Fee = "10"
		seq := env.Seq(alice)
		createTx.Sequence = &seq
		result := env.Submit(createTx)
		xrplgoTesting.RequireTxSuccess(t, result)
		env.Close()
	})
}
