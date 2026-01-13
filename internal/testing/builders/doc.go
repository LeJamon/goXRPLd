// Package builders provides fluent transaction builder helpers for testing.
//
// This package provides builder pattern implementations for common XRPL
// transaction types, making it easy to construct transactions in tests
// without dealing with the full complexity of the transaction structs.
//
// # Payment Builder
//
// Create XRP and issued currency payments:
//
//	// XRP payment
//	Pay(from, to, amount).Build()
//
//	// With options
//	Pay(from, to, amount).
//	    Fee(20).
//	    DestTag(12345).
//	    SourceTag(54321).
//	    PartialPayment().
//	    Build()
//
//	// Issued currency payment
//	PayIssued(from, to, USD("100", gateway)).
//	    SendMax(USD("101", gateway)).
//	    Build()
//
// # TrustSet Builder
//
// Create trust lines for issued currencies:
//
//	// Basic trust line
//	TrustUSD(account, issuer, "1000000").Build()
//
//	// With options
//	TrustLine(account, "EUR", issuer, "500000").
//	    QualityIn(QualityFromPercentage(101)).  // 1% premium on incoming
//	    NoRipple().
//	    Build()
//
// # OfferCreate Builder
//
// Create offers in the decentralized exchange:
//
//	// Basic offer
//	OfferCreate(account, takerPays, takerGets).Build()
//
//	// Passive offer (doesn't consume matching offers)
//	OfferCreate(account, takerPays, takerGets).Passive().Build()
//
//	// Immediate-or-cancel offer
//	OfferCreate(account, takerPays, takerGets).ImmediateOrCancel().Build()
//
// # Escrow Builders
//
// Create, finish, and cancel escrows:
//
//	// Time-locked escrow
//	EscrowCreate(from, to, amount).
//	    FinishTime(time.Now().Add(24 * time.Hour)).
//	    CancelTime(time.Now().Add(48 * time.Hour)).
//	    Build()
//
//	// Crypto-condition escrow
//	EscrowCreate(from, to, amount).
//	    Condition(TestCondition1).
//	    Build()
//
//	// Finish escrow
//	EscrowFinish(finisher, owner, offerSeq).
//	    WithConditionAndFulfillment(TestCondition1, TestFulfillment1).
//	    Build()
//
//	// Cancel escrow
//	EscrowCancel(canceller, owner, offerSeq).Build()
//
// # AccountSet Builder
//
// Modify account settings:
//
//	// Set flags
//	AccountSet(account).RequireDest().Build()
//	AccountSet(account).DefaultRipple().Build()
//
//	// Set domain and transfer rate
//	AccountSet(account).
//	    Domain("6578616D706C652E636F6D").  // "example.com" in hex
//	    TransferRate(1_005_000_000).       // 0.5% transfer fee
//	    Build()
//
// # Test Conditions
//
// Pre-computed crypto conditions for escrow testing:
//
//	TestCondition1, TestFulfillment1  // Empty preimage
//	TestCondition2, TestFulfillment2  // Preimage "aaa"
//	TestCondition3, TestFulfillment3  // Preimage "zzz"
//
// # Amount Helpers
//
// Create amounts for use in transactions:
//
//	XRP(1_000_000)           // 1 XRP in drops
//	XRPFromAmount(100.0)     // 100 XRP
//	USD("100.50", gateway)   // $100.50 from gateway
//	EUR("50", gateway)       // 50 EUR
//	IssuedCurrency("100", "JPY", issuer.Address)  // Custom currency
package builders
