// Package amm provides test helpers and builders for AMM transaction testing.
// Reference: rippled/src/test/jtx/AMM.h and rippled/src/test/jtx/AMMTest.h
package amm

import (
	"testing"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	coreAmm "github.com/LeJamon/goXRPLd/internal/core/tx/amm"
	jtx "github.com/LeJamon/goXRPLd/internal/testing"
	"github.com/LeJamon/goXRPLd/internal/testing/payment"
	"github.com/LeJamon/goXRPLd/internal/testing/trustset"
)

// AMM-specific result codes
const (
	TecAMM_BALANCE        = "tecAMM_BALANCE"
	TecAMM_FAILED         = "tecAMM_FAILED"
	TecAMM_INVALID_TOKENS = "tecAMM_INVALID_TOKENS"
	TecAMM_NOT_EMPTY      = "tecAMM_NOT_EMPTY"
	TecUNFUNDED_AMM       = "tecUNFUNDED_AMM"
	TecDUPLICATE          = "tecDUPLICATE"
	TecFROZEN             = "tecFROZEN"
	TecNO_AUTH            = "tecNO_AUTH"
	TecINSUF_RESERVE_LINE = "tecINSUF_RESERVE_LINE"
	TecNO_PERMISSION      = "tecNO_PERMISSION"

	TerNO_AMM     = "terNO_AMM"
	TerNO_ACCOUNT = "terNO_ACCOUNT"
	TerNO_RIPPLE  = "terNO_RIPPLE"

	TemBAD_AMM_TOKENS = "temBAD_AMM_TOKENS"
	TemBAD_AMOUNT     = "temBAD_AMOUNT"
	TemBAD_CURRENCY   = "temBAD_CURRENCY"
	TemBAD_FEE        = "temBAD_FEE"
	TemINVALID_FLAG   = "temINVALID_FLAG"
	TemMALFORMED      = "temMALFORMED"
	TemDISABLED       = "temDISABLED"

	TelINSUF_FEE_P = "telINSUF_FEE_P"

	TesSUCCESS = "tesSUCCESS"
)

// AMMTestEnv wraps TestEnv with AMM-specific helpers.
// Reference: rippled AMMTest base class
type AMMTestEnv struct {
	*jtx.TestEnv
	T *testing.T

	// Standard test accounts matching rippled's test environment
	GW    *jtx.Account // Gateway/issuer account
	Alice *jtx.Account
	Carol *jtx.Account
	Bob   *jtx.Account

	// Standard currencies
	USD tx.Asset
	EUR tx.Asset
	BTC tx.Asset
	GBP tx.Asset
}

// NewAMMTestEnv creates a new AMM test environment with standard accounts and currencies.
// This matches rippled's AMMTest setup.
func NewAMMTestEnv(t *testing.T) *AMMTestEnv {
	t.Helper()

	env := jtx.NewTestEnv(t)

	gw := jtx.NewAccount("gw")
	alice := jtx.NewAccount("alice")
	carol := jtx.NewAccount("carol")
	bob := jtx.NewAccount("bob")

	return &AMMTestEnv{
		TestEnv: env,
		T:       t,
		GW:      gw,
		Alice:   alice,
		Carol:   carol,
		Bob:     bob,
		USD:     tx.Asset{Currency: "USD", Issuer: gw.Address},
		EUR:     tx.Asset{Currency: "EUR", Issuer: gw.Address},
		BTC:     tx.Asset{Currency: "BTC", Issuer: gw.Address},
		GBP:     tx.Asset{Currency: "GBP", Issuer: gw.Address},
	}
}

// XRP returns the XRP asset (no issuer).
func XRP() tx.Asset {
	return tx.Asset{Currency: "XRP"}
}

// Fund funds the standard accounts (gw, alice, carol) with XRP and sets up IOUs.
// This matches rippled's fund() helper in AMM tests.
func (e *AMMTestEnv) Fund() {
	e.T.Helper()

	// Fund all accounts with 30,000 XRP
	e.TestEnv.FundAmount(e.GW, uint64(jtx.XRP(30000)))
	e.TestEnv.FundAmount(e.Alice, uint64(jtx.XRP(30000)))
	e.TestEnv.FundAmount(e.Carol, uint64(jtx.XRP(30000)))
	e.Close()
}

// FundWithIOUs funds accounts and sets up trust lines with IOUs.
// This matches rippled's fund() with Fund::All flag.
func (e *AMMTestEnv) FundWithIOUs(usdAmount, btcAmount float64) {
	e.T.Helper()

	e.Fund()

	// Set up USD trust lines
	e.Trust(e.Alice, e.GW, "USD", 100000)
	e.Trust(e.Carol, e.GW, "USD", 100000)
	e.Close()

	// Fund with USD
	if usdAmount > 0 {
		e.PayIOU(e.GW, e.Alice, "USD", usdAmount)
		e.PayIOU(e.GW, e.Carol, "USD", usdAmount)
	}
	e.Close()

	// Set up BTC trust lines if needed
	if btcAmount > 0 {
		e.Trust(e.Alice, e.GW, "BTC", 100)
		e.Trust(e.Carol, e.GW, "BTC", 100)
		e.Close()
		e.PayIOU(e.GW, e.Alice, "BTC", btcAmount)
		e.PayIOU(e.GW, e.Carol, "BTC", btcAmount)
		e.Close()
	}
}

// Trust creates a trust line from holder to issuer for the specified currency.
func (e *AMMTestEnv) Trust(holder, issuer *jtx.Account, currency string, limit float64) {
	e.T.Helper()

	limitAmt := tx.NewIssuedAmountFromFloat64(limit, currency, issuer.Address)
	trustTx := trustset.TrustSet(holder, limitAmt).Build()
	result := e.Submit(trustTx)
	if !result.Success {
		e.T.Fatalf("Failed to create trust line %s->%s for %s: %s", holder.Name, issuer.Name, currency, result.Code)
	}
}

// PayIOU sends an IOU payment from sender to receiver.
func (e *AMMTestEnv) PayIOU(sender, receiver *jtx.Account, currency string, amount float64) {
	e.T.Helper()

	amt := tx.NewIssuedAmountFromFloat64(amount, currency, sender.Address)
	payTx := payment.PayIssued(sender, receiver, amt).Build()
	result := e.Submit(payTx)
	if !result.Success {
		e.T.Fatalf("Failed to pay %f %s from %s to %s: %s", amount, currency, sender.Name, receiver.Name, result.Code)
	}
}

// ExpectTER checks if the result matches one of the expected TER codes.
func ExpectTER(t *testing.T, result jtx.TxResult, expectedCodes ...string) {
	t.Helper()

	for _, code := range expectedCodes {
		if result.Code == code {
			return
		}
	}

	if len(expectedCodes) == 1 {
		t.Fatalf("Expected %s, got %s: %s", expectedCodes[0], result.Code, result.Message)
	} else {
		t.Fatalf("Expected one of %v, got %s: %s", expectedCodes, result.Code, result.Message)
	}
}

// XRPAmount creates an XRP amount in drops for use in transactions.
func XRPAmount(xrp int64) tx.Amount {
	return tx.NewXRPAmount(xrp * 1_000_000)
}

// IOUAmount creates an IOU amount for use in transactions.
func IOUAmount(issuer *jtx.Account, currency string, amount float64) tx.Amount {
	if issuer == nil {
		// Create a zero amount with empty issuer
		return tx.NewIssuedAmountFromFloat64(0, currency, "")
	}
	return tx.NewIssuedAmountFromFloat64(amount, currency, issuer.Address)
}

// IOU creates an IOU amount (alias for IOUAmount).
func IOU(issuer *jtx.Account, currency string, amount float64) tx.Amount {
	return IOUAmount(issuer, currency, amount)
}

// LPTokenAmount creates an LP token amount for the given AMM asset pair.
// This generates the proper LP token currency (starting with 03) and uses the AMM account as issuer.
// Reference: rippled test fixtures use amm.lptIssue() which is the real LP token issue.
func LPTokenAmount(asset1, asset2 tx.Asset, amount float64) tx.Amount {
	lptCurrency := coreAmm.GenerateAMMLPTCurrency(asset1.Currency, asset2.Currency)
	ammAccountAddr := coreAmm.ComputeAMMAccountAddress(asset1, asset2)
	return tx.NewIssuedAmountFromFloat64(amount, lptCurrency, ammAccountAddr)
}

// TestAMMCallback is the function signature for AMM test callbacks.
// Reference: rippled's testAMM callback pattern
type TestAMMCallback func(env *AMMTestEnv, ammAccount string)

// WithAMM sets up an AMM and calls the test callback.
// This matches rippled's testAMM helper function.
func WithAMM(t *testing.T, amount1, amount2 tx.Amount, tradingFee uint16, callback TestAMMCallback) {
	t.Helper()

	env := NewAMMTestEnv(t)
	env.FundWithIOUs(30000, 1) // Fund with USD and BTC
	env.Close()

	// Create AMM with alice
	createTx := AMMCreate(env.Alice, amount1, amount2).TradingFee(tradingFee).Build()
	result := env.Submit(createTx)
	if !result.Success {
		t.Fatalf("Failed to create AMM: %s - %s", result.Code, result.Message)
	}
	env.Close()

	// Get AMM account address (would need to compute from keylet in production)
	ammAccount := "" // Placeholder - would be computed

	callback(env, ammAccount)
}

// WithDefaultAMM sets up an AMM with XRP(10000)/USD(10000) and no trading fee.
func WithDefaultAMM(t *testing.T, callback TestAMMCallback) {
	t.Helper()

	env := NewAMMTestEnv(t)
	env.FundWithIOUs(30000, 0)
	env.Close()

	// Create AMM with XRP(10000) and USD(10000)
	createTx := AMMCreate(env.Alice, XRPAmount(10000), IOUAmount(env.GW, "USD", 10000)).Build()
	result := env.Submit(createTx)
	if !result.Success {
		t.Fatalf("Failed to create AMM: %s - %s", result.Code, result.Message)
	}
	env.Close()

	callback(env, "")
}
