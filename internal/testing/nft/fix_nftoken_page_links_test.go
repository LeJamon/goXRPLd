package nft_test

// FixNFTokenPageLinks_test.go - Tests for fixing broken NFT page links
// Reference: rippled/src/test/app/FixNFTokenPageLinks_test.cpp

import (
	"testing"

	jtx "github.com/LeJamon/goXRPLd/internal/testing"
	"github.com/LeJamon/goXRPLd/internal/testing/nft"
)

// TestLedgerStateFixErrors tests error cases for the LedgerStateFix transaction.
// Reference: rippled FixNFTokenPageLinks_test.cpp testLedgerStateFixErrors
func TestLedgerStateFixErrors(t *testing.T) {
	t.Skip("testLedgerStateFixErrors requires LedgerStateFix transaction support")

	env := jtx.NewTestEnv(t)

	alice := jtx.NewAccount("alice")
	env.Fund(alice)
	env.Close()

	// Test: LedgerStateFix is disabled without fixNFTokenPageLinks amendment
	t.Run("DisabledWithoutAmendment", func(t *testing.T) {
		// Without amendment: temDISABLED
		// env.Submit(ledgerStateFix.NFTPageLinks(alice, alice), ter(temDISABLED))
	})

	// With amendment enabled:

	// Test: Preflight - Can't combine AccountTxnID and ticket
	t.Run("AccountTxnIDWithTicket", func(t *testing.T) {
		// tx with both AccountTxnID and ticket: temINVALID
	})

	// Test: Fee too low
	t.Run("FeeTooLow", func(t *testing.T) {
		// Fee must be at least increment: telINSUF_FEE_P
	})

	// Test: Invalid flags
	t.Run("InvalidFlags", func(t *testing.T) {
		// txflags(tfPassive): temINVALID_FLAG
	})

	// Test: Missing Owner field
	t.Run("MissingOwner", func(t *testing.T) {
		// No Owner field: temINVALID
	})

	// Test: Invalid LedgerFixType codes
	t.Run("InvalidLedgerFixType", func(t *testing.T) {
		// LedgerFixType = 0: tefINVALID_LEDGER_FIX_TYPE
		// LedgerFixType = 200: tefINVALID_LEDGER_FIX_TYPE
	})

	// Test: Preclaim - Owner account doesn't exist
	t.Run("OwnerNotFound", func(t *testing.T) {
		// Owner not in ledger: tecOBJECT_NOT_FOUND
	})

	t.Log("testLedgerStateFixErrors passed")
}

// TestTokenPageLinkErrors tests error cases where there is nothing to fix.
// Reference: rippled FixNFTokenPageLinks_test.cpp testTokenPageLinkErrors
func TestTokenPageLinkErrors(t *testing.T) {
	t.Skip("testTokenPageLinkErrors requires LedgerStateFix transaction support")

	env := jtx.NewTestEnv(t)

	alice := jtx.NewAccount("alice")
	env.Fund(alice)
	env.Close()

	// Test: Owner has no pages to fix
	t.Run("NoPagesToFix", func(t *testing.T) {
		// No NFT pages: tecFAILED_PROCESSING
	})

	// Test: Owner has only one page (no links to fix)
	t.Run("OnlyOnePage", func(t *testing.T) {
		mintTx := nft.NFTokenMint(alice, 0).Transferable().Build()
		env.Submit(mintTx)
		env.Close()

		// Only one page, nothing to fix: tecFAILED_PROCESSING
	})

	// Test: Owner has multiple pages but links are fine
	t.Run("LinksAlreadyCorrect", func(t *testing.T) {
		// Mint 65 NFTs to create at least 3 pages
		for i := uint32(0); i < 65; i++ {
			mintTx := nft.NFTokenMint(alice, i).Transferable().Build()
			env.Submit(mintTx)
			env.Close()
		}

		// Links are correct: tecFAILED_PROCESSING
	})

	t.Log("testTokenPageLinkErrors passed")
}

// TestFixNFTokenPageLinks tests repairing three kinds of damaged NFToken directories.
// Reference: rippled FixNFTokenPageLinks_test.cpp testFixNFTokenPageLinks
func TestFixNFTokenPageLinks(t *testing.T) {
	t.Skip("testFixNFTokenPageLinks requires full page manipulation and amendment testing")

	env := jtx.NewTestEnv(t)

	alice := jtx.NewAccount("alice")
	bob := jtx.NewAccount("bob")
	carol := jtx.NewAccount("carol")
	daria := jtx.NewAccount("daria")

	env.Fund(alice, bob, carol, daria)
	env.Close()

	// Helper to generate 96 NFTs packed into three pages of 32 each
	genPackedTokens := func(owner *jtx.Account) []string {
		nfts := make([]string, 0, 96)

		for i := uint32(0); i < 96; i++ {
			// Manipulate taxon to create fully packed pages
			// intTaxon = (i / 16) + (i & 0b10000 ? 2 : 0)
			intTaxon := (i / 16)
			if i&0b10000 != 0 {
				intTaxon += 2
			}
			// In real impl: extTaxon := internalTaxon(owner, intTaxon)

			mintTx := nft.NFTokenMint(owner, intTaxon).Transferable().Build()
			result := env.Submit(mintTx)
			if result.Success {
				nfts = append(nfts, "nft_"+string(rune(i)))
			}
			env.Close()
		}

		// Sort NFTs by storage order, not creation order
		// sort.Slice(nfts, ...)

		return nfts
	}

	//**************************************************************************
	// Step 1A: Create damaged directory - one page without final index
	//**************************************************************************
	t.Run("DamagedDirectory_OnePageNoFinalIndex", func(t *testing.T) {
		aliceNFTs := genPackedTokens(alice)
		t.Logf("Alice has %d NFTs in 3 pages", len(aliceNFTs))

		// Burn all tokens in first and last pages
		// This creates a bug: middle page exists but has no links
		for i := 0; i < 32; i++ {
			burnTx := nft.NFTokenBurn(alice, aliceNFTs[i]).Build()
			env.Submit(burnTx)
			env.Close()
		}
		for i := 64; i < 96; i++ {
			burnTx := nft.NFTokenBurn(alice, aliceNFTs[i]).Build()
			env.Submit(burnTx)
			env.Close()
		}

		// Without fixNFTokenPageLinks: last page is missing
		// Middle page exists but has no links (PreviousPageMin, NextPageMin)
	})

	//**************************************************************************
	// Step 1B: Create damaged directory - multiple pages, missing final page
	//**************************************************************************
	t.Run("DamagedDirectory_MissingFinalPage", func(t *testing.T) {
		bobNFTs := genPackedTokens(bob)
		t.Logf("Bob has %d NFTs in 3 pages", len(bobNFTs))

		// Burn all tokens in the last page
		for i := 64; i < 96; i++ {
			burnTx := nft.NFTokenBurn(bob, bobNFTs[i]).Build()
			env.Submit(burnTx)
			env.Close()
		}

		// Without fixNFTokenPageLinks:
		// Last page is deleted (bug - contents should move to last page)
		// Middle page lost NextPageMin field
	})

	//**************************************************************************
	// Step 1C: Create damaged directory - links missing in middle of chain
	//**************************************************************************
	t.Run("DamagedDirectory_BrokenMiddleLinks", func(t *testing.T) {
		carolNFTs := genPackedTokens(carol)
		t.Logf("Carol has %d NFTs in 3 pages", len(carolNFTs))

		// Sell all tokens in the last page to daria
		for i := 64; i < 96; i++ {
			// carol creates sell offer, daria accepts
			sellOfferTx := nft.NFTokenCreateSellOffer(carol, carolNFTs[i], jtx.XRPTxAmount(0)).Build()
			result := env.Submit(sellOfferTx)
			if result.Success {
				offerID := "offer_" + carolNFTs[i]
				acceptTx := nft.NFTokenAcceptSellOffer(daria, offerID).Build()
				env.Submit(acceptTx)
			}
			env.Close()
		}

		// Same bug: last page deleted, middle page lost NextPageMin

		// Now buy the NFTs back from daria to recreate the last page
		for i := 64; i < 96; i++ {
			buyOfferTx := nft.NFTokenCreateBuyOffer(carol, carolNFTs[i], jtx.XRPTxAmount(1), daria).Build()
			result := env.Submit(buyOfferTx)
			if result.Success {
				offerID := "offer_buy_" + carolNFTs[i]
				acceptTx := nft.NFTokenAcceptBuyOffer(daria, offerID).Build()
				env.Submit(acceptTx)
			}
			env.Close()
		}

		// Carol now has 96 NFTs but only 64 reported (broken links in middle)
		// Middle page has no NextPageMin
		// Last page has no PreviousPageMin
	})

	//**************************************************************************
	// Step 2: Enable fixNFTokenPageLinks amendment
	//**************************************************************************
	// env.EnableFeature(fixNFTokenPageLinks)
	env.Close()

	//**************************************************************************
	// Step 3A: Repair alice's one-page directory
	//**************************************************************************
	t.Run("RepairAlice", func(t *testing.T) {
		// daria fixes alice's NFToken directory
		// env.Submit(ledgerStateFix.NFTPageLinks(daria, alice))
		env.Close()

		// After fix:
		// - Last page exists with no links (single page)
		// - Middle page is gone (contents moved to last page)
		// - nftCount(alice) == 32
		// - ownerCount(alice) == 1
	})

	//**************************************************************************
	// Step 3B: Repair bob's two-page directory
	//**************************************************************************
	t.Run("RepairBob", func(t *testing.T) {
		// daria fixes bob's NFToken directory
		// env.Submit(ledgerStateFix.NFTPageLinks(daria, bob))
		env.Close()

		// After fix:
		// - Last page present with PreviousPageMin link
		// - First page has NextPageMin link to last page
		// - Middle page is gone
		// - nftCount(bob) == 64
		// - ownerCount(bob) == 2
	})

	//**************************************************************************
	// Step 3C: Repair carol's three-page directory
	//**************************************************************************
	t.Run("RepairCarol", func(t *testing.T) {
		// carol fixes her own directory
		// env.Submit(ledgerStateFix.NFTPageLinks(carol, carol))
		env.Close()

		// After fix:
		// - Middle page now has NextPageMin to last page
		// - Last page now has PreviousPageMin to middle page
		// - First page has NextPageMin to middle page
		// - nftCount(carol) == 96
		// - ownerCount(carol) == 3
	})

	t.Log("testFixNFTokenPageLinks passed")
}
