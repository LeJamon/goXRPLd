package builders

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
)

func TestPaymentBuilder(t *testing.T) {
	alice := NewAccount("rAlice")
	bob := NewAccount("rBob")

	// Basic payment
	payment := Pay(alice, bob, 1_000_000).Build()
	require.NotNil(t, payment)
	assert.Equal(t, tx.TypePayment, payment.TxType())

	// With fee
	payment2 := Pay(alice, bob, 1_000_000).Fee(20).Build()
	common := payment2.GetCommon()
	assert.Equal(t, "20", common.Fee)

	// With destination tag
	payment3 := Pay(alice, bob, 1_000_000).DestTag(12345).Build().(*tx.Payment)
	require.NotNil(t, payment3.DestinationTag)
	assert.Equal(t, uint32(12345), *payment3.DestinationTag)

	// With source tag
	payment4 := Pay(alice, bob, 1_000_000).SourceTag(54321).Build().(*tx.Payment)
	require.NotNil(t, payment4.SourceTag)
	assert.Equal(t, uint32(54321), *payment4.SourceTag)

	// Partial payment flag
	payment5 := Pay(alice, bob, 1_000_000).PartialPayment().Build()
	flags := payment5.GetCommon().GetFlags()
	assert.True(t, flags&tx.PaymentFlagPartialPayment != 0)
}

func TestPaymentBuilderIssued(t *testing.T) {
	alice := NewAccount("rAlice")
	bob := NewAccount("rBob")
	gateway := NewAccount("rGateway")

	// Issued currency payment
	amount := USD("100.50", gateway)
	payment := PayIssued(alice, bob, amount).Build().(*tx.Payment)

	assert.Equal(t, "100.50", payment.Amount.Value)
	assert.Equal(t, "USD", payment.Amount.Currency)
	assert.Equal(t, gateway.Address, payment.Amount.Issuer)
}

func TestAccountSetBuilder(t *testing.T) {
	alice := NewAccount("rAlice")

	// Basic account set
	accountSet := AccountSet(alice).Build()
	require.NotNil(t, accountSet)
	assert.Equal(t, tx.TypeAccountSet, accountSet.TxType())

	// With require dest flag
	accountSet2 := AccountSet(alice).RequireDest().Build().(*tx.AccountSet)
	require.NotNil(t, accountSet2.SetFlag)
	assert.Equal(t, tx.AccountSetFlagRequireDest, *accountSet2.SetFlag)

	// With default ripple
	accountSet3 := AccountSet(alice).DefaultRipple().Build().(*tx.AccountSet)
	require.NotNil(t, accountSet3.SetFlag)
	assert.Equal(t, tx.AccountSetFlagDefaultRipple, *accountSet3.SetFlag)

	// With domain
	accountSet4 := AccountSet(alice).Domain("6578616D706C65").Build().(*tx.AccountSet)
	assert.Equal(t, "6578616D706C65", accountSet4.Domain)
}

func TestTrustSetBuilder(t *testing.T) {
	alice := NewAccount("rAlice")
	gateway := NewAccount("rGateway")

	// Basic trust line
	trustSet := TrustUSD(alice, gateway, "1000000").Build()
	require.NotNil(t, trustSet)
	assert.Equal(t, tx.TypeTrustSet, trustSet.TxType())

	// With no ripple flag
	trustSet2 := TrustUSD(alice, gateway, "1000000").NoRipple().Build()
	flags := trustSet2.GetCommon().GetFlags()
	assert.True(t, flags&tx.TrustSetFlagSetNoRipple != 0)

	// With quality
	trustSet3 := TrustUSD(alice, gateway, "1000000").
		QualityIn(QualityFromPercentage(101)).
		Build().(*tx.TrustSet)
	require.NotNil(t, trustSet3.QualityIn)
	// 101% = 1,010,000,000
	assert.Equal(t, uint32(1_010_000_000), *trustSet3.QualityIn)
}

func TestOfferCreateBuilder(t *testing.T) {
	alice := NewAccount("rAlice")
	gateway := NewAccount("rGateway")

	takerPays := XRP(1_000_000)        // 1 XRP
	takerGets := USD("100", gateway)   // 100 USD

	// Basic offer
	offer := OfferCreate(alice, takerPays, takerGets).Build()
	require.NotNil(t, offer)
	assert.Equal(t, tx.TypeOfferCreate, offer.TxType())

	// Passive offer
	offer2 := OfferCreate(alice, takerPays, takerGets).Passive().Build()
	flags := offer2.GetCommon().GetFlags()
	assert.True(t, flags&tx.OfferCreateFlagPassive != 0)

	// Immediate or cancel
	offer3 := OfferCreate(alice, takerPays, takerGets).ImmediateOrCancel().Build()
	flags3 := offer3.GetCommon().GetFlags()
	assert.True(t, flags3&tx.OfferCreateFlagImmediateOrCancel != 0)

	// Fill or kill
	offer4 := OfferCreate(alice, takerPays, takerGets).FillOrKill().Build()
	flags4 := offer4.GetCommon().GetFlags()
	assert.True(t, flags4&tx.OfferCreateFlagFillOrKill != 0)
}

func TestOfferCancelBuilder(t *testing.T) {
	alice := NewAccount("rAlice")

	// Cancel an offer
	cancel := OfferCancel(alice, 5).Build()
	require.NotNil(t, cancel)
	assert.Equal(t, tx.TypeOfferCancel, cancel.TxType())

	cancelTx := cancel.(*tx.OfferCancel)
	assert.Equal(t, uint32(5), cancelTx.OfferSequence)
}

func TestEscrowCreateBuilder(t *testing.T) {
	alice := NewAccount("rAlice")
	bob := NewAccount("rBob")

	// Time-based escrow
	finishTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	cancelTime := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	escrow := EscrowCreate(alice, bob, 1_000_000).
		FinishTime(finishTime).
		CancelTime(cancelTime).
		Build()
	require.NotNil(t, escrow)
	assert.Equal(t, tx.TypeEscrowCreate, escrow.TxType())

	escrowTx := escrow.(*tx.EscrowCreate)
	require.NotNil(t, escrowTx.FinishAfter)
	require.NotNil(t, escrowTx.CancelAfter)

	// Condition-based escrow
	escrow2 := EscrowCreate(alice, bob, 1_000_000).
		Condition(TestCondition1).
		Build().(*tx.EscrowCreate)
	assert.NotEmpty(t, escrow2.Condition)
}

func TestEscrowFinishBuilder(t *testing.T) {
	alice := NewAccount("rAlice")
	bob := NewAccount("rBob")

	// Finish with condition and fulfillment
	finish := EscrowFinish(bob, alice, 1).
		WithConditionAndFulfillment(TestCondition1, TestFulfillment1).
		Build()
	require.NotNil(t, finish)
	assert.Equal(t, tx.TypeEscrowFinish, finish.TxType())

	finishTx := finish.(*tx.EscrowFinish)
	assert.NotEmpty(t, finishTx.Condition)
	assert.NotEmpty(t, finishTx.Fulfillment)
}

func TestEscrowCancelBuilder(t *testing.T) {
	alice := NewAccount("rAlice")
	bob := NewAccount("rBob")

	cancel := EscrowCancel(bob, alice, 1).Build()
	require.NotNil(t, cancel)
	assert.Equal(t, tx.TypeEscrowCancel, cancel.TxType())
}

func TestRippleTimeConversion(t *testing.T) {
	// Ripple epoch is Jan 1, 2000 00:00:00 UTC
	rippleEpochUnix := int64(946684800)

	// Test conversion to Ripple time
	testTime := time.Unix(rippleEpochUnix+1000, 0)
	rippleTime := ToRippleTime(testTime)
	assert.Equal(t, uint32(1000), rippleTime)

	// Test conversion from Ripple time
	goTime := FromRippleTime(1000)
	assert.Equal(t, testTime.Unix(), goTime.Unix())
}

func TestConditionFeeCalculation(t *testing.T) {
	// No fulfillment = no extra fee
	assert.Equal(t, uint64(0), ConditionFeeCalculation(0))

	// 16 bytes = 330 drops
	assert.Equal(t, uint64(330), ConditionFeeCalculation(16))

	// 17 bytes = 660 drops (rounds up)
	assert.Equal(t, uint64(660), ConditionFeeCalculation(17))

	// 32 bytes = 660 drops
	assert.Equal(t, uint64(660), ConditionFeeCalculation(32))
}

func TestRecommendedEscrowFinishFee(t *testing.T) {
	// Base fee (10) + condition fee
	assert.Equal(t, uint64(10), RecommendedEscrowFinishFee(nil))
	assert.Equal(t, uint64(10), RecommendedEscrowFinishFee([]byte{}))

	// With TestFulfillment1 (4 bytes) = 10 + 330 = 340
	fee := RecommendedEscrowFinishFee(TestFulfillment1)
	assert.Equal(t, uint64(340), fee)
}

func TestAmountHelpers(t *testing.T) {
	gateway := NewAccount("rGateway")

	// XRP amount
	xrp := XRP(1_000_000)
	assert.True(t, xrp.IsNative())
	assert.Equal(t, "1000000", xrp.Value)

	// XRP from float
	xrp2 := XRPFromAmount(100.0)
	assert.True(t, xrp2.IsNative())
	assert.Equal(t, "100000000", xrp2.Value)

	// USD
	usd := USD("100.50", gateway)
	assert.False(t, usd.IsNative())
	assert.Equal(t, "100.50", usd.Value)
	assert.Equal(t, "USD", usd.Currency)
	assert.Equal(t, gateway.Address, usd.Issuer)

	// EUR
	eur := EUR("50", gateway)
	assert.Equal(t, "EUR", eur.Currency)

	// BTC
	btc := BTC("0.001", gateway)
	assert.Equal(t, "BTC", btc.Currency)

	// Custom issued currency
	jpy := IssuedCurrency("1000", "JPY", gateway.Address)
	assert.Equal(t, "JPY", jpy.Currency)
}

func TestQualityFromPercentage(t *testing.T) {
	// 100% = 1,000,000,000
	assert.Equal(t, QualityParity, QualityFromPercentage(100))

	// 101% = 1,010,000,000
	assert.Equal(t, uint32(1_010_000_000), QualityFromPercentage(101))

	// 99% = 990,000,000
	assert.Equal(t, uint32(990_000_000), QualityFromPercentage(99))
}

func TestTestConditions(t *testing.T) {
	// Verify test conditions are properly defined
	assert.NotEmpty(t, TestCondition1)
	assert.NotEmpty(t, TestFulfillment1)

	assert.NotEmpty(t, TestCondition2)
	assert.NotEmpty(t, TestFulfillment2)

	assert.NotEmpty(t, TestCondition3)
	assert.NotEmpty(t, TestFulfillment3)

	// All conditions should start with 0xA0 (PREIMAGE-SHA-256 type)
	assert.Equal(t, byte(0xA0), TestCondition1[0])
	assert.Equal(t, byte(0xA0), TestCondition2[0])
	assert.Equal(t, byte(0xA0), TestCondition3[0])
}

func TestAllTestConditions(t *testing.T) {
	conditions := AllTestConditions()
	assert.Len(t, conditions, 3)

	// Check all pairs have condition and fulfillment
	for _, pair := range conditions {
		assert.NotEmpty(t, pair.Name)
		assert.NotEmpty(t, pair.Condition)
		assert.NotEmpty(t, pair.Fulfillment)
	}
}

func TestWellKnownAccounts(t *testing.T) {
	// Genesis should be the master account address
	assert.Equal(t, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh", Genesis.Address)

	// Other well-known accounts should have addresses
	assert.NotEmpty(t, Alice.Address)
	assert.NotEmpty(t, Bob.Address)
	assert.NotEmpty(t, Carol.Address)
	assert.NotEmpty(t, Dave.Address)
	assert.NotEmpty(t, Gateway.Address)

	// All should have unique addresses
	addresses := map[string]bool{
		Genesis.Address: true,
		Alice.Address:   true,
		Bob.Address:     true,
		Carol.Address:   true,
		Dave.Address:    true,
		Gateway.Address: true,
	}
	assert.Len(t, addresses, 6)
}

func TestAccountNextSeq(t *testing.T) {
	alice := NewAccount("rAlice")
	alice.Sequence = 1

	// Should return current and increment
	assert.Equal(t, uint32(1), alice.NextSeq())
	assert.Equal(t, uint32(2), alice.NextSeq())
	assert.Equal(t, uint32(3), alice.NextSeq())
}
