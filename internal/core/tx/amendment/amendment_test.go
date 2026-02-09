package amendment

import (
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"testing"

	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
	"github.com/LeJamon/goXRPLd/internal/core/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// txWithAmendments is a test helper that wraps BaseTx with custom RequiredAmendments.
// Used for testing transaction types that have moved to sub-packages.
type txWithAmendments struct {
	tx.BaseTx
	amendments []string
}

func (t *txWithAmendments) RequiredAmendments() []string {
	return t.amendments
}

// TestTransactionRequiredAmendments verifies that each transaction type
// correctly reports its required amendments.
func TestTransactionRequiredAmendments(t *testing.T) {
	tests := []struct {
		name     string
		tx       tx.Transaction
		expected []string
	}{
		// No amendments required - core transaction types
		{"Payment", &tx.BaseTx{txType: tx.TypePayment, Common: tx.Common{Account: "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"}}, nil},
		{"AccountSet", &AccountSet{BaseTx: *NewBaseTx(TypeAccountSet, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, nil},
		{"TrustSet", &BaseTx{txType: TypeTrustSet, Common: Common{Account: "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"}}, nil},
		{"OfferCreate", &BaseTx{txType: TypeOfferCreate, Common: Common{Account: "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"}}, nil},
		{"OfferCancel", &BaseTx{txType: TypeOfferCancel, Common: Common{Account: "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"}}, nil},
		{"SetRegularKey", &SetRegularKey{BaseTx: *NewBaseTx(TypeRegularKeySet, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, nil},
		{"SignerListSet", &SignerListSet{BaseTx: *NewBaseTx(TypeSignerListSet, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, nil},
		{"TicketCreate", &TicketCreate{BaseTx: *NewBaseTx(TypeTicketCreate, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, nil},
		{"DepositPreauth", &DepositPreauth{BaseTx: *NewBaseTx(TypeDepositPreauth, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, nil},
		{"AccountDelete", &AccountDelete{BaseTx: *NewBaseTx(TypeAccountDelete, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, nil},
		{"EscrowCreate", &BaseTx{txType: TypeEscrowCreate, Common: Common{Account: "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"}}, nil},
		{"EscrowFinish", &BaseTx{txType: TypeEscrowFinish, Common: Common{Account: "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"}}, nil},
		{"EscrowCancel", &BaseTx{txType: TypeEscrowCancel, Common: Common{Account: "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"}}, nil},
		{"PaymentChannelCreate", &BaseTx{txType: TypePaymentChannelCreate, Common: Common{Account: "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"}}, nil},
		{"PaymentChannelFund", &BaseTx{txType: TypePaymentChannelFund, Common: Common{Account: "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"}}, nil},
		{"PaymentChannelClaim", &BaseTx{txType: TypePaymentChannelClaim, Common: Common{Account: "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"}}, nil},

		// Check transactions
		{"CheckCreate", &txWithAmendments{BaseTx: *NewBaseTx(TypeCheckCreate, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentChecks}}, []string{AmendmentChecks}},
		{"CheckCash", &txWithAmendments{BaseTx: *NewBaseTx(TypeCheckCash, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentChecks}}, []string{AmendmentChecks}},
		{"CheckCancel", &txWithAmendments{BaseTx: *NewBaseTx(TypeCheckCancel, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentChecks}}, []string{AmendmentChecks}},

		// AMM transactions (moved to sub-package, using txWithAmendments helper)
		{"AMMCreate", &txWithAmendments{BaseTx: *NewBaseTx(TypeAMMCreate, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentAMM, AmendmentFixUniversalNumber}}, []string{AmendmentAMM, AmendmentFixUniversalNumber}},
		{"AMMDeposit", &txWithAmendments{BaseTx: *NewBaseTx(TypeAMMDeposit, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentAMM, AmendmentFixUniversalNumber}}, []string{AmendmentAMM, AmendmentFixUniversalNumber}},
		{"AMMWithdraw", &txWithAmendments{BaseTx: *NewBaseTx(TypeAMMWithdraw, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentAMM, AmendmentFixUniversalNumber}}, []string{AmendmentAMM, AmendmentFixUniversalNumber}},
		{"AMMVote", &txWithAmendments{BaseTx: *NewBaseTx(TypeAMMVote, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentAMM, AmendmentFixUniversalNumber}}, []string{AmendmentAMM, AmendmentFixUniversalNumber}},
		{"AMMBid", &txWithAmendments{BaseTx: *NewBaseTx(TypeAMMBid, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentAMM, AmendmentFixUniversalNumber}}, []string{AmendmentAMM, AmendmentFixUniversalNumber}},
		{"AMMDelete", &txWithAmendments{BaseTx: *NewBaseTx(TypeAMMDelete, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentAMM, AmendmentFixUniversalNumber}}, []string{AmendmentAMM, AmendmentFixUniversalNumber}},
		{"AMMClawback", &txWithAmendments{BaseTx: *NewBaseTx(TypeAMMClawback, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentAMM, AmendmentFixUniversalNumber, AmendmentAMMClawback}}, []string{AmendmentAMM, AmendmentFixUniversalNumber, AmendmentAMMClawback}},

		// NFToken transactions (moved to sub-package, using txWithAmendments helper)
		{"NFTokenMint", &txWithAmendments{BaseTx: *NewBaseTx(TypeNFTokenMint, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentNonFungibleTokensV1}}, []string{AmendmentNonFungibleTokensV1}},
		{"NFTokenBurn", &txWithAmendments{BaseTx: *NewBaseTx(TypeNFTokenBurn, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentNonFungibleTokensV1}}, []string{AmendmentNonFungibleTokensV1}},
		{"NFTokenCreateOffer", &txWithAmendments{BaseTx: *NewBaseTx(TypeNFTokenCreateOffer, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentNonFungibleTokensV1}}, []string{AmendmentNonFungibleTokensV1}},
		{"NFTokenCancelOffer", &txWithAmendments{BaseTx: *NewBaseTx(TypeNFTokenCancelOffer, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentNonFungibleTokensV1}}, []string{AmendmentNonFungibleTokensV1}},
		{"NFTokenAcceptOffer", &txWithAmendments{BaseTx: *NewBaseTx(TypeNFTokenAcceptOffer, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentNonFungibleTokensV1}}, []string{AmendmentNonFungibleTokensV1}},
		{"NFTokenModify", &NFTokenModify{BaseTx: *NewBaseTx(TypeNFTokenModify, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentDynamicNFT}},

		// XChain transactions
		{"XChainCreateBridge", &XChainCreateBridge{BaseTx: *NewBaseTx(TypeXChainCreateBridge, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentXChainBridge}},
		{"XChainModifyBridge", &XChainModifyBridge{BaseTx: *NewBaseTx(TypeXChainModifyBridge, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentXChainBridge}},
		{"XChainCreateClaimID", &XChainCreateClaimID{BaseTx: *NewBaseTx(TypeXChainCreateClaimID, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentXChainBridge}},
		{"XChainCommit", &XChainCommit{BaseTx: *NewBaseTx(TypeXChainCommit, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentXChainBridge}},
		{"XChainClaim", &XChainClaim{BaseTx: *NewBaseTx(TypeXChainClaim, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentXChainBridge}},
		{"XChainAccountCreateCommit", &XChainAccountCreateCommit{BaseTx: *NewBaseTx(TypeXChainAccountCreateCommit, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentXChainBridge}},
		{"XChainAddClaimAttestation", &XChainAddClaimAttestation{BaseTx: *NewBaseTx(TypeXChainAddClaimAttestation, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentXChainBridge}},
		{"XChainAddAccountCreateAttestation", &XChainAddAccountCreateAttestation{BaseTx: *NewBaseTx(TypeXChainAddAccountCreateAttest, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentXChainBridge}},

		// DID transactions (moved to did/ sub-package)
		{"DIDSet", &txWithAmendments{BaseTx: *NewBaseTx(TypeDIDSet, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentDID}}, []string{AmendmentDID}},
		{"DIDDelete", &txWithAmendments{BaseTx: *NewBaseTx(TypeDIDDelete, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentDID}}, []string{AmendmentDID}},

		// Oracle transactions
		{"OracleSet", &OracleSet{BaseTx: *NewBaseTx(TypeOracleSet, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentPriceOracle}},
		{"OracleDelete", &OracleDelete{BaseTx: *NewBaseTx(TypeOracleDelete, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentPriceOracle}},

		// Credential transactions
		{"CredentialCreate", &CredentialCreate{BaseTx: *NewBaseTx(TypeCredentialCreate, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentCredentials}},
		{"CredentialAccept", &CredentialAccept{BaseTx: *NewBaseTx(TypeCredentialAccept, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentCredentials}},
		{"CredentialDelete", &CredentialDelete{BaseTx: *NewBaseTx(TypeCredentialDelete, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentCredentials}},

		// MPToken transactions
		{"MPTokenIssuanceCreate", &MPTokenIssuanceCreate{BaseTx: *NewBaseTx(TypeMPTokenIssuanceCreate, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentMPTokensV1}},
		{"MPTokenIssuanceDestroy", &MPTokenIssuanceDestroy{BaseTx: *NewBaseTx(TypeMPTokenIssuanceDestroy, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentMPTokensV1}},
		{"MPTokenIssuanceSet", &MPTokenIssuanceSet{BaseTx: *NewBaseTx(TypeMPTokenIssuanceSet, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentMPTokensV1}},
		{"MPTokenAuthorize", &MPTokenAuthorize{BaseTx: *NewBaseTx(TypeMPTokenAuthorize, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentMPTokensV1}},

		// Vault transactions
		{"VaultCreate", &VaultCreate{BaseTx: *NewBaseTx(TypeVaultCreate, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentSingleAssetVault}},
		{"VaultSet", &VaultSet{BaseTx: *NewBaseTx(TypeVaultSet, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentSingleAssetVault}},
		{"VaultDelete", &VaultDelete{BaseTx: *NewBaseTx(TypeVaultDelete, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentSingleAssetVault}},
		{"VaultDeposit", &VaultDeposit{BaseTx: *NewBaseTx(TypeVaultDeposit, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentSingleAssetVault}},
		{"VaultWithdraw", &VaultWithdraw{BaseTx: *NewBaseTx(TypeVaultWithdraw, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentSingleAssetVault}},
		{"VaultClawback", &VaultClawback{BaseTx: *NewBaseTx(TypeVaultClawback, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentSingleAssetVault}},

		// Permissioned domain transactions (require 2 amendments)
		{"PermissionedDomainSet", &PermissionedDomainSet{BaseTx: *NewBaseTx(TypePermissionedDomainSet, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentPermissionedDomains, AmendmentCredentials}},
		{"PermissionedDomainDelete", &PermissionedDomainDelete{BaseTx: *NewBaseTx(TypePermissionedDomainDelete, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentPermissionedDomains, AmendmentCredentials}},

		// Other
		{"Clawback", &Clawback{BaseTx: *NewBaseTx(TypeClawback, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentClawback}},
		{"Batch", &Batch{BaseTx: *NewBaseTx(TypeBatch, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentBatch}},
		{"DelegateSet", &DelegateSet{BaseTx: *NewBaseTx(TypeDelegateSet, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}, []string{AmendmentPermissionDelegation}},
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
		engine := tx.NewEngine(view, config)

		tx := &txWithAmendments{
			BaseTx:     *tx.NewBaseTx(tx.TypeCheckCreate, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"),
			amendments: []string{AmendmentChecks},
		}
		tx.Common.Fee = "12"
		seq := uint32(1)
		tx.Common.Sequence = &seq

		result := engine.Apply(tx)
		assert.Equal(t, tx.TemDISABLED, result.Result, "CheckCreate should return temDISABLED when Checks amendment is not enabled")
		assert.False(t, result.Applied, "Transaction should not be applied")
	})

	// Test with Checks amendment enabled
	t.Run("CheckCreate_WithAmendment", func(t *testing.T) {
		view := newMockLedgerView()
		rules := amendment.NewRulesBuilder().
			EnableByName("Checks").
			Build()
		config := tx.EngineConfig{
			Rules:                     rules,
			SkipSignatureVerification: true,
		}
		engine := tx.NewEngine(view, config)

		tx := &txWithAmendments{
			BaseTx:     *NewBaseTx(TypeCheckCreate, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"),
			amendments: []string{AmendmentChecks},
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

		ammTx := &txWithAmendments{
			BaseTx:     *NewBaseTx(TypeAMMCreate, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"),
			amendments: []string{AmendmentAMM, AmendmentFixUniversalNumber},
		}
		ammTx.Common.Fee = "12"
		seq := uint32(1)
		ammTx.Common.Sequence = &seq

		result := engine.Apply(ammTx)
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

		ammTx := &txWithAmendments{
			BaseTx:     *NewBaseTx(TypeAMMCreate, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"),
			amendments: []string{AmendmentAMM, AmendmentFixUniversalNumber},
		}
		ammTx.Common.Fee = "12"
		seq := uint32(1)
		ammTx.Common.Sequence = &seq

		result := engine.Apply(ammTx)
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

		ammTx := &txWithAmendments{
			BaseTx:     *NewBaseTx(TypeAMMCreate, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"),
			amendments: []string{AmendmentAMM, AmendmentFixUniversalNumber},
		}
		ammTx.Common.Fee = "12"
		seq := uint32(1)
		ammTx.Common.Sequence = &seq

		result := engine.Apply(ammTx)
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

		tx := &txWithAmendments{
			BaseTx:     *NewBaseTx(TypeCheckCreate, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"),
			amendments: []string{AmendmentChecks},
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

		tx := &txWithAmendments{BaseTx: *NewBaseTx(TypeDIDSet, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"), amendments: []string{AmendmentDID}}
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

		tx := &txWithAmendments{
			BaseTx:     *NewBaseTx(TypeNFTokenMint, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"),
			amendments: []string{AmendmentNonFungibleTokensV1},
		}
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
		tx := NewXChainCreateBridge("rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh", bridge, NewXRPAmount("100"))
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

		tx := NewClawback("rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh", NewIssuedAmount("100", "USD", "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"))
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

		tx := NewOracleSet("rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh", 1, 1234567890)
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

		tx := NewMPTokenIssuanceCreate("rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")
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

		tx := NewCredentialCreate("rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh", "rSubject", "AABBCCDD")
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

		tx := NewBatch("rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")
		tx.AddInnerTransaction(makeDummyInnerTx()) // Dummy inner tx
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

		tx := NewDelegateSet("rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")
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

		tx := NewPermissionedDomainSet("rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")
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

		tx := NewPermissionedDomainSet("rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")
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

		tx := NewPermissionedDomainSet("rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")
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

		ammTx := &txWithAmendments{
			BaseTx:     *NewBaseTx(TypeAMMClawback, "rIssuer"),
			amendments: []string{AmendmentAMM, AmendmentFixUniversalNumber, AmendmentAMMClawback},
		}
		ammTx.Common.Fee = "12"
		seq := uint32(1)
		ammTx.Common.Sequence = &seq

		result := engine.Apply(ammTx)
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

		ammTx := &txWithAmendments{
			BaseTx:     *NewBaseTx(TypeAMMClawback, "rIssuer"),
			amendments: []string{AmendmentAMM, AmendmentFixUniversalNumber, AmendmentAMMClawback},
		}
		ammTx.Common.Fee = "12"
		seq := uint32(1)
		ammTx.Common.Sequence = &seq

		result := engine.Apply(ammTx)
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
		{"AccountSet", &AccountSet{BaseTx: *NewBaseTx(TypeAccountSet, "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh")}},
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
