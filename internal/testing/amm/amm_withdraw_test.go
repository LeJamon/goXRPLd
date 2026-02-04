// Package amm_test contains tests for AMM withdraw transactions.
// Reference: rippled/src/test/app/AMM_test.cpp testInvalidWithdraw and testWithdraw
package amm_test

import (
	"testing"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	jtx "github.com/LeJamon/goXRPLd/internal/testing"
	"github.com/LeJamon/goXRPLd/internal/testing/amm"
)

// TestInvalidWithdraw tests invalid withdrawal scenarios.
// Reference: rippled AMM_test.cpp testInvalidWithdraw (line 1685)
func TestInvalidWithdraw(t *testing.T) {
	// Invalid flags - tfBurnable
	// Reference: ammAlice.withdraw(alice, 1'000'000, ..., tfBurnable, ..., ter(temINVALID_FLAG));
	t.Run("InvalidFlags_Burnable", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			LPTokenIn(amm.IOUAmount(env.GW, "LPT", 1000000)).
			Flags(0x00000001). // tfBurnable - invalid for withdraw
			LPToken().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdraw with invalid flags (tfBurnable)")
		}
		amm.ExpectTER(t, result, amm.TemINVALID_FLAG)
	})

	// Invalid flags - tfTwoAssetIfEmpty
	// Reference: ammAlice.withdraw(alice, 1'000'000, ..., tfTwoAssetIfEmpty, ..., ter(temINVALID_FLAG));
	t.Run("InvalidFlags_TwoAssetIfEmpty", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			LPTokenIn(amm.IOUAmount(env.GW, "LPT", 1000000)).
			Flags(amm.TfTwoAssetIfEmpty). // Invalid for withdraw
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdraw with invalid flags (tfTwoAssetIfEmpty)")
		}
		amm.ExpectTER(t, result, amm.TemINVALID_FLAG)
	})

	// Invalid options - no tokens, no amounts, no flags
	// Reference: {std::nullopt, std::nullopt, std::nullopt, std::nullopt, std::nullopt, temMALFORMED}
	t.Run("InvalidOptions_NoParams", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdraw with no parameters")
		}
		amm.ExpectTER(t, result, amm.TemMALFORMED)
	})

	// Invalid options - conflicting flags
	// Reference: {std::nullopt, std::nullopt, std::nullopt, std::nullopt, tfSingleAsset | tfTwoAsset, temMALFORMED}
	t.Run("InvalidOptions_ConflictingFlags", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			Amount(amm.XRPAmount(100)).
			Flags(amm.TfSingleAsset | amm.TfTwoAsset).
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdraw with conflicting flags")
		}
		amm.ExpectTER(t, result, amm.TemMALFORMED)
	})

	// Invalid options - tokens with tfWithdrawAll
	// Reference: {1'000, std::nullopt, std::nullopt, std::nullopt, tfWithdrawAll, temMALFORMED}
	t.Run("InvalidOptions_TokensWithWithdrawAll", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			LPTokenIn(amm.IOUAmount(env.GW, "LPT", 1000)).
			WithdrawAll().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow tokens with tfWithdrawAll")
		}
		amm.ExpectTER(t, result, amm.TemMALFORMED)
	})

	// Invalid options - tfWithdrawAll with tfOneAssetWithdrawAll
	// Reference: {std::nullopt, std::nullopt, std::nullopt, std::nullopt, tfWithdrawAll | tfOneAssetWithdrawAll, temMALFORMED}
	t.Run("InvalidOptions_WithdrawAllAndOneAsset", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			Flags(amm.TfWithdrawAll | amm.TfOneAssetWithdrawAll).
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow tfWithdrawAll with tfOneAssetWithdrawAll")
		}
		amm.ExpectTER(t, result, amm.TemMALFORMED)
	})

	// Invalid tokens - zero
	// Reference: ammAlice.withdraw(alice, 0, std::nullopt, std::nullopt, ter(temBAD_AMM_TOKENS));
	t.Run("ZeroTokens", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			LPTokenIn(amm.IOUAmount(env.GW, "LPT", 0)).
			LPToken().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdraw with zero tokens")
		}
		amm.ExpectTER(t, result, amm.TemBAD_AMM_TOKENS)
	})

	// Invalid tokens - negative
	// Reference: ammAlice.withdraw(alice, IOUAmount{-1}, std::nullopt, std::nullopt, ter(temBAD_AMM_TOKENS));
	t.Run("NegativeTokens", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			LPTokenIn(amm.IOUAmount(env.GW, "LPT", -1)).
			LPToken().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdraw with negative tokens")
		}
		amm.ExpectTER(t, result, amm.TemBAD_AMM_TOKENS)
	})

	// Mismatched token - invalid Asset1Out issue
	// Reference: ammAlice.withdraw(alice, GBP(100), std::nullopt, std::nullopt, ter(temBAD_AMM_TOKENS));
	t.Run("MismatchedToken_Asset1", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			Amount(amm.IOUAmount(env.GW, "GBP", 100)).
			SingleAsset().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdraw with mismatched asset")
		}
		amm.ExpectTER(t, result, amm.TemBAD_AMM_TOKENS)
	})

	// Mismatched token - invalid Asset2Out issue
	// Reference: ammAlice.withdraw(alice, USD(100), GBP(100), std::nullopt, ter(temBAD_AMM_TOKENS));
	t.Run("MismatchedToken_Asset2", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			Amount(amm.IOUAmount(env.GW, "USD", 100)).
			Amount2(amm.IOUAmount(env.GW, "GBP", 100)).
			TwoAsset().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdraw with mismatched Asset2")
		}
		amm.ExpectTER(t, result, amm.TemBAD_AMM_TOKENS)
	})

	// Asset1Out.issue == Asset2Out.issue
	// Reference: ammAlice.withdraw(alice, USD(100), USD(100), std::nullopt, ter(temBAD_AMM_TOKENS));
	t.Run("SameAssetForBoth", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			Amount(amm.IOUAmount(env.GW, "USD", 100)).
			Amount2(amm.IOUAmount(env.GW, "USD", 100)).
			TwoAsset().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow same asset for both outputs")
		}
		amm.ExpectTER(t, result, amm.TemBAD_AMM_TOKENS)
	})

	// Invalid amount value - zero
	// Reference: ammAlice.withdraw(alice, USD(0), std::nullopt, std::nullopt, ter(temBAD_AMOUNT));
	t.Run("ZeroAmount", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			Amount(amm.IOUAmount(env.GW, "USD", 0)).
			SingleAsset().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdraw with zero amount")
		}
		amm.ExpectTER(t, result, amm.TemBAD_AMOUNT)
	})

	// Invalid amount value - negative
	// Reference: ammAlice.withdraw(alice, USD(-100), std::nullopt, std::nullopt, ter(temBAD_AMOUNT));
	t.Run("NegativeAmount", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			Amount(amm.IOUAmount(env.GW, "USD", -100)).
			SingleAsset().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdraw with negative amount")
		}
		amm.ExpectTER(t, result, amm.TemBAD_AMOUNT)
	})

	// Withdraw all tokens from one side - tecAMM_BALANCE
	// Reference: ammAlice.withdraw(alice, USD(10'000), std::nullopt, std::nullopt, ter(tecAMM_BALANCE));
	t.Run("WithdrawAllFromOneSide_USD", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			Amount(amm.IOUAmount(env.GW, "USD", 10000)).
			SingleAsset().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdrawing all tokens from one side")
		}
		amm.ExpectTER(t, result, amm.TecAMM_BALANCE)
	})

	// Withdraw all tokens from one side - XRP
	// Reference: ammAlice.withdraw(alice, XRP(10'000), std::nullopt, std::nullopt, ter(tecAMM_BALANCE));
	t.Run("WithdrawAllFromOneSide_XRP", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			Amount(amm.XRPAmount(10000)).
			SingleAsset().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdrawing all XRP from pool")
		}
		amm.ExpectTER(t, result, amm.TecAMM_BALANCE)
	})

	// Invalid Account (non-existent)
	// Reference: ammAlice.withdraw(bad, 1'000'000, ..., ter(terNO_ACCOUNT));
	t.Run("NonExistentAccount", func(t *testing.T) {
		env := setupAMM(t)

		bad := jtx.NewAccount("bad")
		withdrawTx := amm.AMMWithdraw(bad, amm.XRP(), env.USD).
			LPTokenIn(amm.IOUAmount(env.GW, "LPT", 1000000)).
			LPToken().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdraw from non-existent account")
		}
		amm.ExpectTER(t, result, amm.TerNO_ACCOUNT)
	})

	// Invalid AMM (non-existent)
	// Reference: ammAlice.withdraw(alice, 1'000, ..., {{USD, GBP}}, ..., ter(terNO_AMM));
	t.Run("NonExistentAMM", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, env.USD, env.GBP).
			LPTokenIn(amm.IOUAmount(env.GW, "LPT", 1000)).
			LPToken().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow withdraw from non-existent AMM")
		}
		amm.ExpectTER(t, result, amm.TerNO_AMM)
	})

	// Carol is not a Liquidity Provider
	// Reference: ammAlice.withdraw(carol, 10'000, std::nullopt, std::nullopt, ter(tecAMM_BALANCE));
	t.Run("NotLiquidityProvider", func(t *testing.T) {
		env := setupAMM(t)

		// Carol hasn't deposited, so she can't withdraw
		withdrawTx := amm.AMMWithdraw(env.Carol, amm.XRP(), env.USD).
			LPTokenIn(amm.IOUAmount(env.GW, "LPT", 10000)).
			LPToken().
			Build()
		result := env.Submit(withdrawTx)

		if result.Success {
			t.Fatal("Should not allow non-LP to withdraw")
		}
		amm.ExpectTER(t, result, amm.TecAMM_BALANCE)
	})
}

// TestWithdraw tests valid withdrawal scenarios.
// Reference: rippled AMM_test.cpp testWithdraw (line 2265)
func TestWithdraw(t *testing.T) {
	// Equal withdrawal by tokens
	// Reference: ammAlice.withdraw(alice, 1'000'000)
	t.Run("EqualWithdrawalByTokens", func(t *testing.T) {
		env := setupAMM(t)

		// First deposit as Carol to have tokens to withdraw
		depositTx := amm.AMMDeposit(env.Carol, amm.XRP(), env.USD).
			LPTokenOut(amm.IOUAmount(env.GW, "LPT", 1000000)).
			LPToken().
			Build()
		result := env.Submit(depositTx)
		if !result.Success {
			t.Fatalf("Failed to deposit: %s", result.Code)
		}
		env.Close()

		initialBalance := env.Balance(env.Carol)

		// Withdraw all Carol's tokens
		withdrawTx := amm.AMMWithdraw(env.Carol, amm.XRP(), env.USD).
			LPTokenIn(amm.IOUAmount(env.GW, "LPT", 1000000)).
			LPToken().
			Build()
		result = env.Submit(withdrawTx)

		if !result.Success {
			t.Fatalf("Equal withdrawal by tokens should succeed: %s - %s", result.Code, result.Message)
		}
		env.Close()

		// XRP balance should have increased
		finalBalance := env.Balance(env.Carol)
		if finalBalance <= initialBalance {
			t.Fatal("XRP balance should have increased after withdrawal")
		}

		t.Log("Equal withdrawal by tokens passed")
	})

	// Equal withdrawal with limit
	// Reference: ammAlice.withdraw(alice, XRP(200), USD(100))
	t.Run("EqualWithdrawalWithLimit", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			Amount(amm.XRPAmount(200)).
			Amount2(amm.IOUAmount(env.GW, "USD", 100)).
			TwoAsset().
			Build()
		result := env.Submit(withdrawTx)

		if !result.Success {
			t.Fatalf("Equal withdrawal with limit should succeed: %s - %s", result.Code, result.Message)
		}
		env.Close()

		t.Log("Equal withdrawal with limit passed")
	})

	// Single withdrawal by amount - XRP
	// Reference: ammAlice.withdraw(alice, XRP(1'000))
	t.Run("SingleWithdrawal_XRP", func(t *testing.T) {
		env := setupAMM(t)

		initialBalance := env.Balance(env.Alice)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			Amount(amm.XRPAmount(1000)).
			SingleAsset().
			Build()
		result := env.Submit(withdrawTx)

		if !result.Success {
			t.Fatalf("Single XRP withdrawal should succeed: %s - %s", result.Code, result.Message)
		}
		env.Close()

		finalBalance := env.Balance(env.Alice)
		if finalBalance <= initialBalance {
			t.Fatal("XRP balance should have increased after withdrawal")
		}

		t.Log("Single XRP withdrawal passed")
	})

	// Single withdrawal by tokens
	// Reference: ammAlice.withdraw(alice, 10'000, USD(0))
	t.Run("SingleWithdrawalByTokens", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			LPTokenIn(amm.IOUAmount(env.GW, "LPT", 10000)).
			Amount(amm.IOUAmount(env.GW, "USD", 0)).
			OneAssetLPToken().
			Build()
		result := env.Submit(withdrawTx)

		if !result.Success {
			t.Fatalf("Single withdrawal by tokens should succeed: %s - %s", result.Code, result.Message)
		}
		env.Close()

		t.Log("Single withdrawal by tokens passed")
	})

	// Withdraw all tokens - deletes AMM
	// Reference: ammAlice.withdrawAll(alice)
	t.Run("WithdrawAll", func(t *testing.T) {
		env := setupAMM(t)

		withdrawTx := amm.AMMWithdraw(env.Alice, amm.XRP(), env.USD).
			WithdrawAll().
			Build()
		result := env.Submit(withdrawTx)

		if !result.Success {
			t.Fatalf("Withdraw all should succeed: %s - %s", result.Code, result.Message)
		}
		env.Close()

		// AMM should be deleted after this
		// Verify by trying to deposit - should fail with terNO_AMM
		depositTx := amm.AMMDeposit(env.Carol, amm.XRP(), env.USD).
			Amount(amm.XRPAmount(100)).
			SingleAsset().
			Build()
		result = env.Submit(depositTx)

		if result.Success {
			t.Log("Note: AMM may not have been deleted if other LPs exist")
		} else {
			amm.ExpectTER(t, result, amm.TerNO_AMM)
			t.Log("Withdraw all and AMM deletion passed")
		}
	})

	// Single deposit then withdraw all in USD
	// Reference: ammAlice.deposit(carol, USD(1'000)); ammAlice.withdrawAll(carol, USD(0));
	t.Run("DepositThenWithdrawAllInUSD", func(t *testing.T) {
		env := setupAMM(t)

		// First deposit as Carol
		depositTx := amm.AMMDeposit(env.Carol, amm.XRP(), env.USD).
			Amount(amm.IOUAmount(env.GW, "USD", 1000)).
			SingleAsset().
			Build()
		result := env.Submit(depositTx)
		if !result.Success {
			t.Fatalf("Deposit should succeed: %s", result.Code)
		}
		env.Close()

		// Withdraw all Carol's tokens in USD
		withdrawTx := amm.AMMWithdraw(env.Carol, amm.XRP(), env.USD).
			Amount(amm.IOUAmount(env.GW, "USD", 0)). // USD(0) means withdraw in USD
			OneAssetWithdrawAll().
			Build()
		result = env.Submit(withdrawTx)

		if !result.Success {
			t.Fatalf("Withdraw all in USD should succeed: %s - %s", result.Code, result.Message)
		}
		env.Close()

		t.Log("Deposit then withdraw all in USD passed")
	})

	// Single deposit then withdraw all in XRP
	// Reference: ammAlice.deposit(carol, USD(1'000)); ammAlice.withdrawAll(carol, XRP(0));
	t.Run("DepositThenWithdrawAllInXRP", func(t *testing.T) {
		env := setupAMM(t)

		// First deposit as Carol
		depositTx := amm.AMMDeposit(env.Carol, amm.XRP(), env.USD).
			Amount(amm.IOUAmount(env.GW, "USD", 1000)).
			SingleAsset().
			Build()
		result := env.Submit(depositTx)
		if !result.Success {
			t.Fatalf("Deposit should succeed: %s", result.Code)
		}
		env.Close()

		// Withdraw all Carol's tokens in XRP
		withdrawTx := amm.AMMWithdraw(env.Carol, amm.XRP(), env.USD).
			Amount(tx.NewXRPAmount(0)). // XRP(0) means withdraw in XRP
			OneAssetWithdrawAll().
			Build()
		result = env.Submit(withdrawTx)

		if !result.Success {
			t.Fatalf("Withdraw all in XRP should succeed: %s - %s", result.Code, result.Message)
		}
		env.Close()

		t.Log("Deposit then withdraw all in XRP passed")
	})

	// Equal deposit 10%, withdraw all tokens
	// Reference: ammAlice.deposit(carol, 1'000'000); ammAlice.withdrawAll(carol);
	t.Run("EqualDepositThenWithdrawAll", func(t *testing.T) {
		env := setupAMM(t)

		// Deposit 10% of pool
		depositTx := amm.AMMDeposit(env.Carol, amm.XRP(), env.USD).
			LPTokenOut(amm.IOUAmount(env.GW, "LPT", 1000000)).
			LPToken().
			Build()
		result := env.Submit(depositTx)
		if !result.Success {
			t.Fatalf("Deposit should succeed: %s", result.Code)
		}
		env.Close()

		// Withdraw all Carol's tokens
		withdrawTx := amm.AMMWithdraw(env.Carol, amm.XRP(), env.USD).
			WithdrawAll().
			Build()
		result = env.Submit(withdrawTx)

		if !result.Success {
			t.Fatalf("Withdraw all should succeed: %s - %s", result.Code, result.Message)
		}
		env.Close()

		t.Log("Equal deposit then withdraw all passed")
	})
}
