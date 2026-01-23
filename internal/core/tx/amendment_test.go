package tx

import (
	"testing"

	"github.com/LeJamon/goXRPLd/internal/core/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransactionRequiredAmendments verifies that each transaction type
// correctly reports its required amendments.
func TestTransactionRequiredAmendments(t *testing.T) {
	tests := []struct {
		name     string
		tx       Transaction
		expected []string
	}{
		// No amendments required - core transaction types
		{"Payment", &Payment{BaseTx: *NewBaseTx(TypePayment, "rTest")}, nil},
		{"AccountSet", &AccountSet{BaseTx: *NewBaseTx(TypeAccountSet, "rTest")}, nil},
		{"TrustSet", &TrustSet{BaseTx: *NewBaseTx(TypeTrustSet, "rTest")}, nil},
		{"OfferCreate", &OfferCreate{BaseTx: *NewBaseTx(TypeOfferCreate, "rTest")}, nil},
		{"OfferCancel", &OfferCancel{BaseTx: *NewBaseTx(TypeOfferCancel, "rTest")}, nil},
		{"SetRegularKey", &SetRegularKey{BaseTx: *NewBaseTx(TypeRegularKeySet, "rTest")}, nil},
		{"SignerListSet", &SignerListSet{BaseTx: *NewBaseTx(TypeSignerListSet, "rTest")}, nil},
		{"TicketCreate", &TicketCreate{BaseTx: *NewBaseTx(TypeTicketCreate, "rTest")}, nil},
		{"DepositPreauth", &DepositPreauth{BaseTx: *NewBaseTx(TypeDepositPreauth, "rTest")}, nil},
		{"AccountDelete", &AccountDelete{BaseTx: *NewBaseTx(TypeAccountDelete, "rTest")}, nil},
		{"EscrowCreate", &EscrowCreate{BaseTx: *NewBaseTx(TypeEscrowCreate, "rTest")}, nil},
		{"EscrowFinish", &EscrowFinish{BaseTx: *NewBaseTx(TypeEscrowFinish, "rTest")}, nil},
		{"EscrowCancel", &EscrowCancel{BaseTx: *NewBaseTx(TypeEscrowCancel, "rTest")}, nil},
		{"PaymentChannelCreate", &PaymentChannelCreate{BaseTx: *NewBaseTx(TypePaymentChannelCreate, "rTest")}, nil},
		{"PaymentChannelFund", &PaymentChannelFund{BaseTx: *NewBaseTx(TypePaymentChannelFund, "rTest")}, nil},
		{"PaymentChannelClaim", &PaymentChannelClaim{BaseTx: *NewBaseTx(TypePaymentChannelClaim, "rTest")}, nil},

		// Check transactions
		{"CheckCreate", &CheckCreate{BaseTx: *NewBaseTx(TypeCheckCreate, "rTest")}, []string{AmendmentChecks}},
		{"CheckCash", &CheckCash{BaseTx: *NewBaseTx(TypeCheckCash, "rTest")}, []string{AmendmentChecks}},
		{"CheckCancel", &CheckCancel{BaseTx: *NewBaseTx(TypeCheckCancel, "rTest")}, []string{AmendmentChecks}},

		// AMM transactions (require AMM and fixUniversalNumber)
		{"AMMCreate", &AMMCreate{BaseTx: *NewBaseTx(TypeAMMCreate, "rTest")}, []string{AmendmentAMM, AmendmentFixUniversalNumber}},
		{"AMMDeposit", &AMMDeposit{BaseTx: *NewBaseTx(TypeAMMDeposit, "rTest")}, []string{AmendmentAMM, AmendmentFixUniversalNumber}},
		{"AMMWithdraw", &AMMWithdraw{BaseTx: *NewBaseTx(TypeAMMWithdraw, "rTest")}, []string{AmendmentAMM, AmendmentFixUniversalNumber}},
		{"AMMVote", &AMMVote{BaseTx: *NewBaseTx(TypeAMMVote, "rTest")}, []string{AmendmentAMM, AmendmentFixUniversalNumber}},
		{"AMMBid", &AMMBid{BaseTx: *NewBaseTx(TypeAMMBid, "rTest")}, []string{AmendmentAMM, AmendmentFixUniversalNumber}},
		{"AMMDelete", &AMMDelete{BaseTx: *NewBaseTx(TypeAMMDelete, "rTest")}, []string{AmendmentAMM, AmendmentFixUniversalNumber}},
		{"AMMClawback", &AMMClawback{BaseTx: *NewBaseTx(TypeAMMClawback, "rTest")}, []string{AmendmentAMM, AmendmentFixUniversalNumber, AmendmentAMMClawback}},

		// NFToken transactions
		{"NFTokenMint", &NFTokenMint{BaseTx: *NewBaseTx(TypeNFTokenMint, "rTest")}, []string{AmendmentNonFungibleTokensV1}},
		{"NFTokenBurn", &NFTokenBurn{BaseTx: *NewBaseTx(TypeNFTokenBurn, "rTest")}, []string{AmendmentNonFungibleTokensV1}},
		{"NFTokenCreateOffer", &NFTokenCreateOffer{BaseTx: *NewBaseTx(TypeNFTokenCreateOffer, "rTest")}, []string{AmendmentNonFungibleTokensV1}},
		{"NFTokenCancelOffer", &NFTokenCancelOffer{BaseTx: *NewBaseTx(TypeNFTokenCancelOffer, "rTest")}, []string{AmendmentNonFungibleTokensV1}},
		{"NFTokenAcceptOffer", &NFTokenAcceptOffer{BaseTx: *NewBaseTx(TypeNFTokenAcceptOffer, "rTest")}, []string{AmendmentNonFungibleTokensV1}},
		{"NFTokenModify", &NFTokenModify{BaseTx: *NewBaseTx(TypeNFTokenModify, "rTest")}, []string{AmendmentDynamicNFT}},

		// XChain transactions
		{"XChainCreateBridge", &XChainCreateBridge{BaseTx: *NewBaseTx(TypeXChainCreateBridge, "rTest")}, []string{AmendmentXChainBridge}},
		{"XChainModifyBridge", &XChainModifyBridge{BaseTx: *NewBaseTx(TypeXChainModifyBridge, "rTest")}, []string{AmendmentXChainBridge}},
		{"XChainCreateClaimID", &XChainCreateClaimID{BaseTx: *NewBaseTx(TypeXChainCreateClaimID, "rTest")}, []string{AmendmentXChainBridge}},
		{"XChainCommit", &XChainCommit{BaseTx: *NewBaseTx(TypeXChainCommit, "rTest")}, []string{AmendmentXChainBridge}},
		{"XChainClaim", &XChainClaim{BaseTx: *NewBaseTx(TypeXChainClaim, "rTest")}, []string{AmendmentXChainBridge}},
		{"XChainAccountCreateCommit", &XChainAccountCreateCommit{BaseTx: *NewBaseTx(TypeXChainAccountCreateCommit, "rTest")}, []string{AmendmentXChainBridge}},
		{"XChainAddClaimAttestation", &XChainAddClaimAttestation{BaseTx: *NewBaseTx(TypeXChainAddClaimAttestation, "rTest")}, []string{AmendmentXChainBridge}},
		{"XChainAddAccountCreateAttestation", &XChainAddAccountCreateAttestation{BaseTx: *NewBaseTx(TypeXChainAddAccountCreateAttest, "rTest")}, []string{AmendmentXChainBridge}},

		// DID transactions
		{"DIDSet", &DIDSet{BaseTx: *NewBaseTx(TypeDIDSet, "rTest")}, []string{AmendmentDID}},
		{"DIDDelete", &DIDDelete{BaseTx: *NewBaseTx(TypeDIDDelete, "rTest")}, []string{AmendmentDID}},

		// Oracle transactions
		{"OracleSet", &OracleSet{BaseTx: *NewBaseTx(TypeOracleSet, "rTest")}, []string{AmendmentPriceOracle}},
		{"OracleDelete", &OracleDelete{BaseTx: *NewBaseTx(TypeOracleDelete, "rTest")}, []string{AmendmentPriceOracle}},

		// Credential transactions
		{"CredentialCreate", &CredentialCreate{BaseTx: *NewBaseTx(TypeCredentialCreate, "rTest")}, []string{AmendmentCredentials}},
		{"CredentialAccept", &CredentialAccept{BaseTx: *NewBaseTx(TypeCredentialAccept, "rTest")}, []string{AmendmentCredentials}},
		{"CredentialDelete", &CredentialDelete{BaseTx: *NewBaseTx(TypeCredentialDelete, "rTest")}, []string{AmendmentCredentials}},

		// MPToken transactions
		{"MPTokenIssuanceCreate", &MPTokenIssuanceCreate{BaseTx: *NewBaseTx(TypeMPTokenIssuanceCreate, "rTest")}, []string{AmendmentMPTokensV1}},
		{"MPTokenIssuanceDestroy", &MPTokenIssuanceDestroy{BaseTx: *NewBaseTx(TypeMPTokenIssuanceDestroy, "rTest")}, []string{AmendmentMPTokensV1}},
		{"MPTokenIssuanceSet", &MPTokenIssuanceSet{BaseTx: *NewBaseTx(TypeMPTokenIssuanceSet, "rTest")}, []string{AmendmentMPTokensV1}},
		{"MPTokenAuthorize", &MPTokenAuthorize{BaseTx: *NewBaseTx(TypeMPTokenAuthorize, "rTest")}, []string{AmendmentMPTokensV1}},

		// Vault transactions
		{"VaultCreate", &VaultCreate{BaseTx: *NewBaseTx(TypeVaultCreate, "rTest")}, []string{AmendmentSingleAssetVault}},
		{"VaultSet", &VaultSet{BaseTx: *NewBaseTx(TypeVaultSet, "rTest")}, []string{AmendmentSingleAssetVault}},
		{"VaultDelete", &VaultDelete{BaseTx: *NewBaseTx(TypeVaultDelete, "rTest")}, []string{AmendmentSingleAssetVault}},
		{"VaultDeposit", &VaultDeposit{BaseTx: *NewBaseTx(TypeVaultDeposit, "rTest")}, []string{AmendmentSingleAssetVault}},
		{"VaultWithdraw", &VaultWithdraw{BaseTx: *NewBaseTx(TypeVaultWithdraw, "rTest")}, []string{AmendmentSingleAssetVault}},
		{"VaultClawback", &VaultClawback{BaseTx: *NewBaseTx(TypeVaultClawback, "rTest")}, []string{AmendmentSingleAssetVault}},

		// Permissioned domain transactions (require 2 amendments)
		{"PermissionedDomainSet", &PermissionedDomainSet{BaseTx: *NewBaseTx(TypePermissionedDomainSet, "rTest")}, []string{AmendmentPermissionedDomains, AmendmentCredentials}},
		{"PermissionedDomainDelete", &PermissionedDomainDelete{BaseTx: *NewBaseTx(TypePermissionedDomainDelete, "rTest")}, []string{AmendmentPermissionedDomains, AmendmentCredentials}},

		// Other
		{"Clawback", &Clawback{BaseTx: *NewBaseTx(TypeClawback, "rTest")}, []string{AmendmentClawback}},
		{"Batch", &Batch{BaseTx: *NewBaseTx(TypeBatch, "rTest")}, []string{AmendmentBatch}},
		{"DelegateSet", &DelegateSet{BaseTx: *NewBaseTx(TypeDelegateSet, "rTest")}, []string{AmendmentPermissionDelegation}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.tx.RequiredAmendments()
			assert.Equal(t, tt.expected, got)
		})
	}
}

// mockLedgerView implements LedgerView for testing
type mockLedgerView struct {
	data           map[[32]byte][]byte
	dropsDestroyed XRPAmount.XRPAmount
}

func newMockLedgerView() *mockLedgerView {
	return &mockLedgerView{
		data: make(map[[32]byte][]byte),
	}
}

func (m *mockLedgerView) Read(k keylet.Keylet) ([]byte, error) {
	if data, ok := m.data[k.Key]; ok {
		return data, nil
	}
	return nil, nil
}

func (m *mockLedgerView) Exists(k keylet.Keylet) (bool, error) {
	_, ok := m.data[k.Key]
	return ok, nil
}

func (m *mockLedgerView) Insert(k keylet.Keylet, data []byte) error {
	m.data[k.Key] = data
	return nil
}

func (m *mockLedgerView) Update(k keylet.Keylet, data []byte) error {
	m.data[k.Key] = data
	return nil
}

func (m *mockLedgerView) Erase(k keylet.Keylet) error {
	delete(m.data, k.Key)
	return nil
}

func (m *mockLedgerView) AdjustDropsDestroyed(drops XRPAmount.XRPAmount) {
	m.dropsDestroyed += drops
}

func (m *mockLedgerView) ForEach(fn func(key [32]byte, data []byte) bool) error {
	for k, v := range m.data {
		if !fn(k, v) {
			break
		}
	}
	return nil
}

// TestAmendmentCheckInPreflight tests that the engine rejects transactions
// when required amendments are not enabled.
func TestAmendmentCheckInPreflight(t *testing.T) {
	// Test with empty rules (no amendments enabled)
	t.Run("CheckCreate_NoAmendments", func(t *testing.T) {
		view := newMockLedgerView()
		config := EngineConfig{
			Rules:                     amendment.EmptyRules(),
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := &CheckCreate{
			BaseTx:      *NewBaseTx(TypeCheckCreate, "rTest"),
			Destination: "rDest",
			SendMax:     NewXRPAmount("1000000"),
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "CheckCreate should return temDISABLED when Checks amendment is not enabled")
		assert.False(t, result.Applied, "Transaction should not be applied")
	})

	// Test with Checks amendment enabled
	t.Run("CheckCreate_WithAmendment", func(t *testing.T) {
		view := newMockLedgerView()
		rules := amendment.NewRulesBuilder().
			EnableByName("Checks").
			Build()
		config := EngineConfig{
			Rules:                     rules,
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := &CheckCreate{
			BaseTx:      *NewBaseTx(TypeCheckCreate, "rTest"),
			Destination: "rDest",
			SendMax:     NewXRPAmount("1000000"),
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		// Should NOT return TemDISABLED (may fail for other reasons like account not found)
		assert.NotEqual(t, TemDISABLED, result.Result, "CheckCreate should not return temDISABLED when Checks amendment is enabled")
	})

	// Test AMM requires multiple amendments - missing fixUniversalNumber
	t.Run("AMMCreate_MissingFixUniversalNumber", func(t *testing.T) {
		view := newMockLedgerView()
		// Only enable AMM, not fixUniversalNumber
		rules := amendment.NewRulesBuilder().
			EnableByName("AMM").
			Build()
		config := EngineConfig{
			Rules:                     rules,
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := &AMMCreate{
			BaseTx:     *NewBaseTx(TypeAMMCreate, "rTest"),
			Amount:     NewXRPAmount("1000000"),
			Amount2:    NewIssuedAmount("100", "USD", "rIssuer"),
			TradingFee: 100,
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "AMMCreate should return temDISABLED when fixUniversalNumber is not enabled")
	})

	// Test AMM requires multiple amendments - missing AMM
	t.Run("AMMCreate_MissingAMM", func(t *testing.T) {
		view := newMockLedgerView()
		// Only enable fixUniversalNumber, not AMM
		rules := amendment.NewRulesBuilder().
			EnableByName("fixUniversalNumber").
			Build()
		config := EngineConfig{
			Rules:                     rules,
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := &AMMCreate{
			BaseTx:     *NewBaseTx(TypeAMMCreate, "rTest"),
			Amount:     NewXRPAmount("1000000"),
			Amount2:    NewIssuedAmount("100", "USD", "rIssuer"),
			TradingFee: 100,
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "AMMCreate should return temDISABLED when AMM amendment is not enabled")
	})

	// Test AMM with both amendments enabled
	t.Run("AMMCreate_WithBothAmendments", func(t *testing.T) {
		view := newMockLedgerView()
		rules := amendment.NewRulesBuilder().
			EnableByName("AMM").
			EnableByName("fixUniversalNumber").
			Build()
		config := EngineConfig{
			Rules:                     rules,
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := &AMMCreate{
			BaseTx:     *NewBaseTx(TypeAMMCreate, "rTest"),
			Amount:     NewXRPAmount("1000000"),
			Amount2:    NewIssuedAmount("100", "USD", "rIssuer"),
			TradingFee: 100,
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		// Should NOT return TemDISABLED (may fail for other reasons)
		assert.NotEqual(t, TemDISABLED, result.Result, "AMMCreate should not return temDISABLED when both amendments are enabled")
	})

	// Test with nil Rules (should skip amendment checks)
	t.Run("NilRules_SkipsCheck", func(t *testing.T) {
		view := newMockLedgerView()
		config := EngineConfig{
			Rules:                     nil, // No rules configured
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := &CheckCreate{
			BaseTx:      *NewBaseTx(TypeCheckCreate, "rTest"),
			Destination: "rDest",
			SendMax:     NewXRPAmount("1000000"),
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		// Should NOT return TemDISABLED when Rules is nil
		assert.NotEqual(t, TemDISABLED, result.Result, "CheckCreate should not return temDISABLED when Rules is nil")
	})
}

// TestAmendmentCheckForVariousTransactionTypes tests amendment checking
// for different categories of transactions.
func TestAmendmentCheckForVariousTransactionTypes(t *testing.T) {
	// Test DID transaction without amendment
	t.Run("DIDSet_NoAmendment", func(t *testing.T) {
		view := newMockLedgerView()
		config := EngineConfig{
			Rules:                     amendment.EmptyRules(),
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := NewDIDSet("rTest")
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "DIDSet should return temDISABLED when DID amendment is not enabled")
	})

	// Test NFToken transaction without amendment
	t.Run("NFTokenMint_NoAmendment", func(t *testing.T) {
		view := newMockLedgerView()
		config := EngineConfig{
			Rules:                     amendment.EmptyRules(),
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := NewNFTokenMint("rTest", 12345)
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "NFTokenMint should return temDISABLED when NonFungibleTokensV1 amendment is not enabled")
	})

	// Test XChain transaction without amendment
	t.Run("XChainCreateBridge_NoAmendment", func(t *testing.T) {
		view := newMockLedgerView()
		config := EngineConfig{
			Rules:                     amendment.EmptyRules(),
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		bridge := XChainBridge{
			LockingChainDoor:  "rDoor1",
			LockingChainIssue: Asset{Currency: "XRP"},
			IssuingChainDoor:  "rDoor2",
			IssuingChainIssue: Asset{Currency: "XRP"},
		}
		tx := NewXChainCreateBridge("rTest", bridge, NewXRPAmount("100"))
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "XChainCreateBridge should return temDISABLED when XChainBridge amendment is not enabled")
	})

	// Test Clawback transaction without amendment
	t.Run("Clawback_NoAmendment", func(t *testing.T) {
		view := newMockLedgerView()
		config := EngineConfig{
			Rules:                     amendment.EmptyRules(),
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := NewClawback("rTest", NewIssuedAmount("100", "USD", "rTest"))
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "Clawback should return temDISABLED when Clawback amendment is not enabled")
	})

	// Test Oracle transaction without amendment
	t.Run("OracleSet_NoAmendment", func(t *testing.T) {
		view := newMockLedgerView()
		config := EngineConfig{
			Rules:                     amendment.EmptyRules(),
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := NewOracleSet("rTest", 1, 1234567890)
		tx.AddPriceData("XRP", "USD", 1000000, 6)
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "OracleSet should return temDISABLED when PriceOracle amendment is not enabled")
	})

	// Test MPToken transaction without amendment
	t.Run("MPTokenIssuanceCreate_NoAmendment", func(t *testing.T) {
		view := newMockLedgerView()
		config := EngineConfig{
			Rules:                     amendment.EmptyRules(),
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := NewMPTokenIssuanceCreate("rTest")
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "MPTokenIssuanceCreate should return temDISABLED when MPTokensV1 amendment is not enabled")
	})

	// Test Credential transaction without amendment
	t.Run("CredentialCreate_NoAmendment", func(t *testing.T) {
		view := newMockLedgerView()
		config := EngineConfig{
			Rules:                     amendment.EmptyRules(),
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := NewCredentialCreate("rTest", "rSubject", "AABBCCDD")
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "CredentialCreate should return temDISABLED when Credentials amendment is not enabled")
	})

	// Test Batch transaction without amendment
	t.Run("Batch_NoAmendment", func(t *testing.T) {
		view := newMockLedgerView()
		config := EngineConfig{
			Rules:                     amendment.EmptyRules(),
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := NewBatch("rTest")
		tx.AddRawTransaction("001234ABCD") // Dummy blob
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "Batch should return temDISABLED when Batch amendment is not enabled")
	})

	// Test DelegateSet transaction without amendment
	t.Run("DelegateSet_NoAmendment", func(t *testing.T) {
		view := newMockLedgerView()
		config := EngineConfig{
			Rules:                     amendment.EmptyRules(),
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := NewDelegateSet("rTest")
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "DelegateSet should return temDISABLED when PermissionDelegation amendment is not enabled")
	})

	// Test PermissionedDomainSet requires two amendments
	t.Run("PermissionedDomainSet_MissingCredentials", func(t *testing.T) {
		view := newMockLedgerView()
		// Only enable PermissionedDomains, not Credentials
		rules := amendment.NewRulesBuilder().
			EnableByName("PermissionedDomains").
			Build()
		config := EngineConfig{
			Rules:                     rules,
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := NewPermissionedDomainSet("rTest")
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "PermissionedDomainSet should return temDISABLED when Credentials amendment is not enabled")
	})

	// Test PermissionedDomainSet requires two amendments
	t.Run("PermissionedDomainSet_MissingPermissionedDomains", func(t *testing.T) {
		view := newMockLedgerView()
		// Only enable Credentials, not PermissionedDomains
		rules := amendment.NewRulesBuilder().
			EnableByName("Credentials").
			Build()
		config := EngineConfig{
			Rules:                     rules,
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := NewPermissionedDomainSet("rTest")
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "PermissionedDomainSet should return temDISABLED when PermissionedDomains amendment is not enabled")
	})

	// Test PermissionedDomainSet with both amendments enabled
	t.Run("PermissionedDomainSet_WithBothAmendments", func(t *testing.T) {
		view := newMockLedgerView()
		rules := amendment.NewRulesBuilder().
			EnableByName("PermissionedDomains").
			EnableByName("Credentials").
			Build()
		config := EngineConfig{
			Rules:                     rules,
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := NewPermissionedDomainSet("rTest")
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		// Should NOT return TemDISABLED (may fail for other reasons)
		assert.NotEqual(t, TemDISABLED, result.Result, "PermissionedDomainSet should not return temDISABLED when both amendments are enabled")
	})
}

// TestAMMClawbackRequiresThreeAmendments tests that AMMClawback requires
// all three amendments: AMM, fixUniversalNumber, and AMMClawback.
func TestAMMClawbackRequiresThreeAmendments(t *testing.T) {
	t.Run("AMMClawback_MissingAMMClawbackAmendment", func(t *testing.T) {
		view := newMockLedgerView()
		// Enable AMM and fixUniversalNumber, but not AMMClawback
		rules := amendment.NewRulesBuilder().
			EnableByName("AMM").
			EnableByName("fixUniversalNumber").
			Build()
		config := EngineConfig{
			Rules:                     rules,
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := NewAMMClawback("rIssuer", "rHolder", Asset{Currency: "USD", Issuer: "rIssuer"}, Asset{Currency: "XRP"})
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, TemDISABLED, result.Result, "AMMClawback should return temDISABLED when AMMClawback amendment is not enabled")
	})

	t.Run("AMMClawback_WithAllThreeAmendments", func(t *testing.T) {
		view := newMockLedgerView()
		rules := amendment.NewRulesBuilder().
			EnableByName("AMM").
			EnableByName("fixUniversalNumber").
			EnableByName("AMMClawback").
			Build()
		config := EngineConfig{
			Rules:                     rules,
			SkipSignatureVerification: true,
		}
		engine := NewEngine(view, config)

		tx := NewAMMClawback("rIssuer", "rHolder", Asset{Currency: "USD", Issuer: "rIssuer"}, Asset{Currency: "XRP"})
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		// Should NOT return TemDISABLED (may fail for other reasons like validation)
		assert.NotEqual(t, TemDISABLED, result.Result, "AMMClawback should not return temDISABLED when all three amendments are enabled")
	})
}

// TestCoreTransactionsNoAmendmentRequired verifies that core transaction types
// work without any amendments enabled.
func TestCoreTransactionsNoAmendmentRequired(t *testing.T) {
	tests := []struct {
		name string
		tx   Transaction
	}{
		{"Payment", NewPayment("rTest", "rDest", NewXRPAmount("1000000"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := newMockLedgerView()
			config := EngineConfig{
				Rules:                     amendment.EmptyRules(),
				SkipSignatureVerification: true,
			}
			engine := NewEngine(view, config)

			// Set common fields
			tt.tx.GetCommon().Fee = "12"
			seq := uint32(1)
			tt.tx.GetCommon().Sequence = &seq

			result := engine.Apply(tt.tx)
			// Core transactions should NOT return TemDISABLED
			assert.NotEqual(t, TemDISABLED, result.Result, "%s should not return temDISABLED - no amendments required", tt.name)
		})
	}
}

// TestAmendmentConstantsMatchRegistry verifies that the amendment constants
// in the tx package match the registered amendment names.
func TestAmendmentConstantsMatchRegistry(t *testing.T) {
	tests := []struct {
		constantName  string
		constantValue string
	}{
		{"AmendmentChecks", AmendmentChecks},
		{"AmendmentAMM", AmendmentAMM},
		{"AmendmentFixUniversalNumber", AmendmentFixUniversalNumber},
		{"AmendmentAMMClawback", AmendmentAMMClawback},
		{"AmendmentNonFungibleTokensV1", AmendmentNonFungibleTokensV1},
		{"AmendmentDynamicNFT", AmendmentDynamicNFT},
		{"AmendmentXChainBridge", AmendmentXChainBridge},
		{"AmendmentDID", AmendmentDID},
		{"AmendmentPriceOracle", AmendmentPriceOracle},
		{"AmendmentCredentials", AmendmentCredentials},
		{"AmendmentMPTokensV1", AmendmentMPTokensV1},
		{"AmendmentSingleAssetVault", AmendmentSingleAssetVault},
		{"AmendmentPermissionedDomains", AmendmentPermissionedDomains},
		{"AmendmentClawback", AmendmentClawback},
		{"AmendmentBatch", AmendmentBatch},
		{"AmendmentPermissionDelegation", AmendmentPermissionDelegation},
	}

	for _, tt := range tests {
		t.Run(tt.constantName, func(t *testing.T) {
			feature := amendment.GetFeatureByName(tt.constantValue)
			require.NotNil(t, feature, "Amendment %q should be registered in the amendment registry", tt.constantValue)
			assert.Equal(t, tt.constantValue, feature.Name, "Amendment name should match")
		})
	}
}

// TestTemDISABLEDResultCode verifies the TemDISABLED result code properties.
func TestTemDISABLEDResultCode(t *testing.T) {
	assert.Equal(t, Result(-273), TemDISABLED, "TemDISABLED should have value -273")
	assert.Equal(t, "temDISABLED", TemDISABLED.String(), "TemDISABLED string should be temDISABLED")
	assert.True(t, TemDISABLED.IsTem(), "TemDISABLED should be a tem code")
	assert.False(t, TemDISABLED.IsSuccess(), "TemDISABLED should not be success")
	assert.False(t, TemDISABLED.IsApplied(), "TemDISABLED should not be applied")
	assert.Equal(t, "The transaction requires an amendment that is not enabled.", TemDISABLED.Message())
}
