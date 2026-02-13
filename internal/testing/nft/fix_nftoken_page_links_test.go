package nft_test

// FixNFTokenPageLinks_test.go - Tests for fixing broken NFT page links
// Reference: rippled/src/test/app/FixNFTokenPageLinks_test.cpp
//
// These tests require the LedgerStateFix transaction type which is not yet
// implemented. The tests are skipped with clear dependency reasons.

import (
	"sort"
	"testing"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/nftoken"
	jtx "github.com/LeJamon/goXRPLd/internal/testing"
	"github.com/LeJamon/goXRPLd/internal/testing/nft"
)

// genPackedTokensForOwner generates 96 NFTs packed into three pages of 32 each.
// The minter creates the NFTs and transfers them to the owner.
// Returns a sorted list of NFT IDs.
func genPackedTokensForOwner(env *jtx.TestEnv, owner, minter *jtx.Account) []string {
	nfts := make([]string, 0, 96)

	for i := uint32(0); i < 96; i++ {
		intTaxon := (i / 16)
		if i&0b10000 != 0 {
			intTaxon += 2
		}

		tokenSeq := env.MintedCount(minter)
		extTaxon := nftoken.CipheredTaxon(tokenSeq, intTaxon)

		flags := nftoken.NFTokenFlagTransferable
		nftID := nft.GetNextNFTokenID(env, minter, extTaxon, flags, 0)
		env.Submit(nft.NFTokenMint(minter, extTaxon).Transferable().Build())
		env.Close()

		// Minter creates sell offer, owner accepts
		offerIndex := nft.GetOfferIndex(env, minter)
		env.Submit(nft.NFTokenCreateSellOffer(minter, nftID, tx.NewXRPAmount(0)).Build())
		env.Close()

		env.Submit(nft.NFTokenAcceptSellOffer(owner, offerIndex).Build())
		env.Close()

		nfts = append(nfts, nftID)
	}

	sort.Strings(nfts)
	return nfts
}

// ===========================================================================
// testLedgerStateFixErrors
// Reference: rippled FixNFTokenPageLinks_test.cpp testLedgerStateFixErrors
//
// Tests error cases for the LedgerStateFix transaction.
// ===========================================================================
func TestLedgerStateFixErrors(t *testing.T) {
	// TODO: Requires LedgerStateFix transaction type to be implemented.
	// LedgerStateFix is a special transaction that repairs broken NFT page
	// links. It was introduced alongside the fixNFTokenPageLinks amendment.
	//
	// Expected test cases:
	// - temDISABLED without fixNFTokenPageLinks amendment
	// - temINVALID with AccountTxnID + ticket
	// - telINSUF_FEE_P with fee below increment
	// - temINVALID_FLAG with invalid flags
	// - temINVALID without Owner field
	// - tefINVALID_LEDGER_FIX_TYPE with bad LedgerFixType codes
	// - tecOBJECT_NOT_FOUND when Owner doesn't exist
	t.Skip("Requires LedgerStateFix transaction type (not implemented)")
}

// ===========================================================================
// testTokenPageLinkErrors
// Reference: rippled FixNFTokenPageLinks_test.cpp testTokenPageLinkErrors
//
// Tests error cases where there is nothing to fix.
// ===========================================================================
func TestTokenPageLinkErrors(t *testing.T) {
	// TODO: Requires LedgerStateFix transaction type to be implemented.
	//
	// Expected test cases:
	// - tecFAILED_PROCESSING when owner has no NFT pages
	// - tecFAILED_PROCESSING when owner has only one page (no links to fix)
	// - tecFAILED_PROCESSING when links are already correct
	t.Skip("Requires LedgerStateFix transaction type (not implemented)")
}

// ===========================================================================
// testFixNFTokenPageLinks
// Reference: rippled FixNFTokenPageLinks_test.cpp testFixNFTokenPageLinks
//
// Tests repairing three kinds of damaged NFToken directories:
// 1. One page without final index
// 2. Multiple pages, missing final page
// 3. Links missing in middle of chain
// ===========================================================================
func TestFixNFTokenPageLinks(t *testing.T) {
	// TODO: Requires LedgerStateFix transaction type to be implemented.
	//
	// The test creates three accounts (alice, bob, carol) with damaged
	// NFT directories (96 NFTs in 3 pages each), then uses LedgerStateFix
	// to repair the broken page links.
	//
	// Step 1: Create damaged directories without fixNFTokenPageLinks
	//   1A: alice - burn first/last pages → middle page orphaned
	//   1B: bob - burn last page → middle page loses NextPageMin
	//   1C: carol - sell last 32 to daria → buy back → broken chain
	//
	// Step 2: Enable fixNFTokenPageLinks amendment
	//
	// Step 3: Use LedgerStateFix to repair:
	//   3A: alice → single page (32 NFTs, ownerCount=1)
	//   3B: bob → two pages (64 NFTs, ownerCount=2)
	//   3C: carol → three pages (96 NFTs, ownerCount=3)
	//
	// Each repair is verified by checking:
	// - Correct page links (PreviousPageMin, NextPageMin)
	// - nftCount matches expected
	// - ownerCount matches expected
	t.Skip("Requires LedgerStateFix transaction type (not implemented)")
}
