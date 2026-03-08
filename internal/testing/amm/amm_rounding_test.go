// Package amm_test contains AMM deposit and withdraw rounding tests.
// Reference: rippled/src/test/app/AMM_test.cpp
//   - testDepositAndWithdrawRounding (line 7527)
//   - testDepositRounding (line 7598)
//   - testWithdrawRounding (line 7779)
//
// These tests verify that the AMM math correctly handles rounding for
// various deposit and withdrawal modes. The key invariant is:
//   sqrt(amount1 * amount2) >= lptBalance
// which must hold after every operation to prevent value leakage.
//
// The tests use GBP/EUR IOU pairs with precise mantissa/exponent values.
// rippled runs each test with both all-amendments-enabled and
// all-minus-fixAMMv1_3 configurations. The fixAMMv1_3 amendment fixes
// several rounding issues that caused invariant violations.
package amm_test

import (
	"testing"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	jtx "github.com/LeJamon/goXRPLd/internal/testing"
	"github.com/LeJamon/goXRPLd/internal/testing/amm"
)

// setupGBPEURPool creates a GBP/EUR IOU AMM pool with the given amounts.
// This matches rippled's testAMM helper when called with {{GBP(gbp), EUR(eur)}}.
//
// The rippled testAMM helper:
//   1. Funds gw, alice, carol with 30,000 XRP each
//   2. Creates trust lines for both IOUs with 2x the pool amount
//   3. Funds alice and carol with the pool amounts of each IOU
//   4. Alice creates the AMM
func setupGBPEURPool(t *testing.T, gbpAmount, eurAmount float64) *amm.AMMTestEnv {
	t.Helper()

	env := amm.NewAMMTestEnv(t)

	// Fund gw, alice, carol with 30,000 XRP (matching rippled's testAMM default)
	env.TestEnv.FundAmount(env.GW, uint64(jtx.XRP(30000)))
	env.TestEnv.FundAmount(env.Alice, uint64(jtx.XRP(30000)))
	env.TestEnv.FundAmount(env.Carol, uint64(jtx.XRP(30000)))
	env.TestEnv.FundAmount(env.Bob, uint64(jtx.XRP(10000000)))
	env.Close()

	// Set up GBP trust lines with 2x the pool amount (matching rippled's fund())
	gbpLimit := gbpAmount * 2
	if gbpLimit < 100000 {
		gbpLimit = 100000
	}
	eurLimit := eurAmount * 2
	if eurLimit < 100000 {
		eurLimit = 100000
	}

	env.Trust(env.Alice, env.GW, "GBP", gbpLimit)
	env.Trust(env.Alice, env.GW, "EUR", eurLimit)
	env.Trust(env.Carol, env.GW, "GBP", gbpLimit)
	env.Trust(env.Carol, env.GW, "EUR", eurLimit)
	env.Close()

	// Fund alice and carol with the pool amounts
	env.PayIOU(env.GW, env.Alice, "GBP", gbpAmount)
	env.PayIOU(env.GW, env.Alice, "EUR", eurAmount)
	env.PayIOU(env.GW, env.Carol, "GBP", gbpAmount)
	env.PayIOU(env.GW, env.Carol, "EUR", eurAmount)
	env.Close()

	// Alice creates the AMM
	createTx := amm.AMMCreate(env.Alice,
		amm.IOUAmount(env.GW, "GBP", gbpAmount),
		amm.IOUAmount(env.GW, "EUR", eurAmount),
	).Build()
	jtx.RequireTxSuccess(t, env.Submit(createTx))
	env.Close()

	return env
}

// setupGBPEURPoolWithBob creates a GBP/EUR AMM and also funds Bob with
// large IOU balances so he can deposit. This matches the rippled pattern
// where fund() is called inside the testAMM callback to add Bob.
func setupGBPEURPoolWithBob(t *testing.T, gbpPool, eurPool float64,
	gbpFund, eurFund float64,
) *amm.AMMTestEnv {
	t.Helper()

	env := setupGBPEURPool(t, gbpPool, eurPool)

	// Fund Bob with IOUs (matching rippled's fund(env, gw, {bob}, XRP(10M), {GBP(100K), EUR(100K)}, Fund::Acct))
	env.Trust(env.Bob, env.GW, "GBP", gbpFund*2)
	env.Trust(env.Bob, env.GW, "EUR", eurFund*2)
	env.Close()
	env.PayIOU(env.GW, env.Bob, "GBP", gbpFund)
	env.PayIOU(env.GW, env.Bob, "EUR", eurFund)
	env.Close()

	return env
}

// TestDepositAndWithdrawRounding tests rounding behavior for deposits and withdrawals.
// Reference: rippled AMM_test.cpp testDepositAndWithdrawRounding (line 7527)
//
// The test creates an AMM with specific XRP/XPM balances, burns tokens to reach a
// specific LP state, then verifies single-asset deposit and withdrawal rounding.
//
// Pool state:
//
//	XRP balance:  692,614,492,126 drops
//	XPM balance:  18,610,359.80246901 XPM (mantissa=1861035980246901, exp=-8)
//	LPT balance after burn: 3,234,987,266.485968 (mantissa=3234987266485968, exp=-6)
//	Trading fee: 941
//
// This test uses XRP/XPM (not GBP/EUR), and exercises the interaction between
// token burning via bid and subsequent single-asset deposit/withdraw precision.
func TestDepositAndWithdrawRounding(t *testing.T) {
	// Reference: rippled runs this with both `all` and `all - fixAMMv1_3`
	for _, fixV1_3 := range []bool{true, false} {
		suffix := "WithFixAMMv1_3"
		if !fixV1_3 {
			suffix = "WithoutFixAMMv1_3"
		}

		t.Run("SingleAssetDeposit/"+suffix, func(t *testing.T) {
			// Reference: rippled AMM_test.cpp line 7555-7562
			// With fixAMMv1_3: deposit should succeed (tesSUCCESS)
			// Without fixAMMv1_3: deposit should fail (tecUNFUNDED_AMM)
			//
			// The test constructs a very specific AMM state by:
			//   1. Creating AMM with XRP(692614492126 drops) / XPM(mantissa=1861035980246901, exp=-8)
			//   2. Burning LP tokens via bid to reach LPT balance of mantissa=3234987266485968, exp=-6
			//   3. Attempting a single-asset deposit of XPM(mantissa=6566496939465400, exp=-12)
			//
			// This requires precise AMM math with specific mantissa/exponent IOU values,
			// AMMBid with exact burn amounts, and getLPTokensBalance() query support.
			t.Skip("Requires XRP/IOU AMM with precise bid-based token burning and specific mantissa/exponent IOU values")
		})

		t.Run("SingleAssetWithdraw/"+suffix, func(t *testing.T) {
			// Reference: rippled AMM_test.cpp line 7563-7575
			// With fixAMMv1_3: (amount2 - amount2_) <= withdraw
			// Without fixAMMv1_3: (amount2 - amount2_) > withdraw (over-withdraw)
			//
			// Uses the same pool setup as SingleAssetDeposit but with tfee=0.
			// Withdraws XPM(1, -5) and verifies the actual withdrawn amount
			// does not exceed the requested amount (with fixAMMv1_3).
			t.Skip("Requires XRP/IOU AMM with precise bid-based token burning and balance query support")
		})
	}
}

// TestDepositRounding tests deposit rounding for various deposit modes.
// Reference: rippled AMM_test.cpp testDepositRounding (line 7598)
//
// All subtests use GBP/EUR IOU pools and verify the invariant:
//
//	sqrt(amount1 * amount2) >= lptBalance
//
// rippled runs each test with both `all` and `all - fixAMMv1_3`.
// The fixAMMv1_3 amendment uses Number::upward rounding mode in the
// invariant check, which fixes rounding violations for certain exponents.
func TestDepositRounding(t *testing.T) {
	for _, fixV1_3 := range []bool{true, false} {
		suffix := "WithFixAMMv1_3"
		if !fixV1_3 {
			suffix = "WithoutFixAMMv1_3"
		}

		// Single asset deposit with various exponents
		// Reference: rippled AMM_test.cpp lines 7603-7636
		// Pool: GBP(30,000) / EUR(30,000) with tfee=0
		// Deposits EUR with mantissa=1 and various exponents
		// The EUR(1, -3) case fails the invariant without fixAMMv1_3
		t.Run("SingleAssetDeposit/"+suffix, func(t *testing.T) {
			for _, tc := range []struct {
				name       string
				mantissa   int64
				exponent   int
				shouldFail bool // invariant fails without fixAMMv1_3
			}{
				{"EUR_1e1", 1, 1, false},
				{"EUR_1e2", 1, 2, false},
				{"EUR_1e5", 1, 5, false},
				{"EUR_1e-3", 1, -3, true}, // fails invariant without fixAMMv1_3
				{"EUR_1e-6", 1, -6, false},
				{"EUR_1e-9", 1, -9, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					// Reference: rippled creates pool with GBP(30000)/EUR(30000),
					// funds bob with GBP(100K)/EUR(100K), deposits EUR(mantissa, exp)
					// then checks invariant: sqrt(gbp * eur) >= lptBalance
					//
					// The invariant check requires reading back the AMM pool balances
					// and LP token supply after the deposit, which needs AMM query support.
					t.Skip("Requires AMM balance query for invariant check: sqrt(amount1*amount2) >= lptBalance")
					_ = tc // suppress unused warning
				})
			}
		})

		// Two-asset proportional deposit (1:1 pool ratio)
		// Reference: rippled AMM_test.cpp lines 7638-7664
		// Pool: GBP(30,000) / EUR(30,000) with tfee=0
		// Deposit: EUR(mantissa=101234567890123456, exp=-16) + GBP(same)
		// Invariant should always hold
		t.Run("TwoAssetProportional_1to1/"+suffix, func(t *testing.T) {
			t.Skip("Requires AMM balance query for invariant check: sqrt(amount1*amount2) >= lptBalance")
		})

		// Two-asset proportional deposit (1:3 pool ratio)
		// Reference: rippled AMM_test.cpp lines 7666-7697
		// Pool: GBP(10,000) / EUR(30,000) with tfee=0
		// Deposits EUR(1, exp) + GBP(1, exp) with various exponents
		// Without fixAMMv1_3, all cases except exp=-3 fail the invariant
		t.Run("TwoAssetProportional_1to3/"+suffix, func(t *testing.T) {
			for _, tc := range []struct {
				name       string
				exponent   int
				shouldFail bool // invariant fails without fixAMMv1_3
			}{
				{"exp_1", 1, true},
				{"exp_2", 2, true},
				{"exp_3", 3, true},
				{"exp_4", 4, true},
				{"exp_-3", -3, false}, // does NOT fail without fixAMMv1_3
				{"exp_-6", -6, true},
				{"exp_-9", -9, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					// Reference: rippled creates pool with GBP(10000)/EUR(30000),
					// funds bob, deposits GBP(1,exp)+EUR(1,exp), checks invariant.
					// shouldFail indicates whether invariant fails without fixAMMv1_3
					// (with fixAMMv1_3, all should pass)
					t.Skip("Requires AMM balance query for invariant check: sqrt(amount1*amount2) >= lptBalance")
					_ = tc
				})
			}
		})

		// tfLPToken deposit
		// Reference: rippled AMM_test.cpp lines 7699-7719
		// Pool: GBP(7,000) / EUR(30,000) with tfee=0
		// Deposit: tokens = IOUAmount(mantissa=101234567890123456, exp=-16)
		// Invariant should always hold
		t.Run("LPTokenDeposit/"+suffix, func(t *testing.T) {
			t.Skip("Requires AMM balance query for invariant check: sqrt(amount1*amount2) >= lptBalance")
		})

		// tfOneAssetLPToken deposit
		// Reference: rippled AMM_test.cpp lines 7721-7753
		// Pool: GBP(7,000) / EUR(30,000) with tfee=0
		// Various token amounts with EUR(1e6) as the asset
		t.Run("OneAssetLPTokenDeposit/"+suffix, func(t *testing.T) {
			for _, tc := range []struct {
				name     string
				mantissa int64
				exponent int
			}{
				{"tokens_0.001", 1, -3},
				{"tokens_0.01", 1, -2},
				{"tokens_0.1", 1, -1},
				{"tokens_1", 1, 0},
				{"tokens_10", 10, 0},
				{"tokens_100", 100, 0},
				{"tokens_1000", 1000, 0},
				{"tokens_10000", 10000, 0},
			} {
				t.Run(tc.name, func(t *testing.T) {
					// Reference: rippled deposits with specified LPToken amount
					// and EUR(1e6) as asset1In, then checks invariant.
					t.Skip("Requires AMM balance query for invariant check: sqrt(amount1*amount2) >= lptBalance")
					_ = tc
				})
			}
		})

		// Single deposit with EP (effective price) limit
		// Reference: rippled AMM_test.cpp lines 7755-7776
		// Pool: GBP(30,000) / EUR(30,000) with tfee=0
		// Deposit: 1,000 GBP with EP not to exceed 5 (GBP/TokensOut)
		// Invariant should always hold
		t.Run("SingleDepositWithEP/"+suffix, func(t *testing.T) {
			t.Skip("Requires AMM balance query for invariant check: sqrt(amount1*amount2) >= lptBalance")
		})
	}
}

// TestWithdrawRounding tests withdrawal rounding for various withdraw modes.
// Reference: rippled AMM_test.cpp testWithdrawRounding (line 7779)
//
// All subtests use GBP/EUR IOU pools and verify the invariant:
//
//	sqrt(amount1 * amount2) >= lptBalance
//
// rippled runs each test with both `all` and `all - fixAMMv1_3`.
func TestWithdrawRounding(t *testing.T) {
	for _, fixV1_3 := range []bool{true, false} {
		suffix := "WithFixAMMv1_3"
		if !fixV1_3 {
			suffix = "WithoutFixAMMv1_3"
		}

		// tfLPToken withdraw
		// Reference: rippled AMM_test.cpp lines 7786-7794
		// Pool: GBP(7,000) / EUR(30,000) with tfee=0
		// Alice withdraws 1,000 LP tokens
		// Invariant should hold
		t.Run("LPTokenWithdraw/"+suffix, func(t *testing.T) {
			t.Skip("Requires AMM balance query for invariant check: sqrt(amount1*amount2) >= lptBalance")
		})

		// tfWithdrawAll mode
		// Reference: rippled AMM_test.cpp lines 7797-7806
		// Pool: GBP(7,000) / EUR(30,000) with tfee=0
		// Alice withdraws all (burns all LP tokens)
		// Invariant should hold (trivially: all balances go to zero)
		t.Run("WithdrawAll/"+suffix, func(t *testing.T) {
			t.Skip("Requires AMM balance query for invariant check: sqrt(amount1*amount2) >= lptBalance")
		})

		// tfTwoAsset withdraw mode
		// Reference: rippled AMM_test.cpp lines 7808-7821
		// Pool: GBP(7,000) / EUR(30,000) with tfee=0
		// Alice withdraws GBP(3,500) + EUR(15,000)
		// Invariant should hold
		t.Run("TwoAssetWithdraw/"+suffix, func(t *testing.T) {
			t.Skip("Requires AMM balance query for invariant check: sqrt(amount1*amount2) >= lptBalance")
		})

		// tfSingleAsset withdraw mode
		// Reference: rippled AMM_test.cpp lines 7823-7839
		// Pool: GBP(7,000) / EUR(30,000) with tfee=0
		// Alice withdraws GBP(1,234) as single asset
		// Note: rippled comment says this fails with 0 trading fee but not with 1,000 fee
		// The compound operations in AMMHelpers.cpp:withdrawByTokens may compensate
		// Invariant should hold
		t.Run("SingleAssetWithdraw/"+suffix, func(t *testing.T) {
			t.Skip("Requires AMM balance query for invariant check: sqrt(amount1*amount2) >= lptBalance")
		})

		// tfOneAssetWithdrawAll mode
		// Reference: rippled AMM_test.cpp lines 7841-7865
		// Pool: GBP(7,000) / EUR(30,000) with tfee=0
		// Bob first deposits GBP(3,456) as single asset, then withdraws all
		// his LP tokens as GBP(1,000) with tfOneAssetWithdrawAll
		// Invariant should hold
		t.Run("OneAssetWithdrawAll/"+suffix, func(t *testing.T) {
			t.Skip("Requires AMM balance query for invariant check: sqrt(amount1*amount2) >= lptBalance")
		})

		// tfOneAssetLPToken mode
		// Reference: rippled AMM_test.cpp lines 7867-7880
		// Pool: GBP(7,000) / EUR(30,000) with tfee=0
		// Alice withdraws 1,000 LP tokens as GBP(100)
		// Invariant should hold
		t.Run("OneAssetLPToken/"+suffix, func(t *testing.T) {
			t.Skip("Requires AMM balance query for invariant check: sqrt(amount1*amount2) >= lptBalance")
		})

		// tfLimitLPToken mode
		// Reference: rippled AMM_test.cpp lines 7882-7895
		// Pool: GBP(7,000) / EUR(30,000) with tfee=0
		// Alice withdraws GBP(100) with maxEP=2 and tfLimitLPToken
		// NOTE: The invariant INTENTIONALLY FAILS here (shouldFail=true in rippled)
		// This documents a known rounding limitation in the LimitLPToken withdraw path
		t.Run("LimitLPToken/"+suffix, func(t *testing.T) {
			t.Skip("Requires AMM balance query for invariant check: sqrt(amount1*amount2) >= lptBalance (intentionally fails)")
		})
	}
}

// The following variables are referenced by the test setup helpers to ensure
// the compiler does not flag unused imports.
var (
	_ = tx.Asset{}
	_ = jtx.XRP
	_ = amm.NewAMMTestEnv
)
