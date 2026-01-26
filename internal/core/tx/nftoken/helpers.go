package nftoken

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

// Field type constants for binary parsing (from sle package)
const (
	fieldTypeUInt16  = sle.FieldTypeUInt16
	fieldTypeUInt32  = sle.FieldTypeUInt32
	fieldTypeUInt64  = sle.FieldTypeUInt64
	fieldTypeHash256 = sle.FieldTypeHash256
	fieldTypeAccount = sle.FieldTypeAccount
	fieldTypeBlob    = sle.FieldTypeBlob
)

// nftPageMask is the mask for the low 96 bits of an NFTokenID
// This is used to group equivalent NFTs on the same page
var nftPageMask = [32]byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
}

// getNFTIssuer extracts the issuer AccountID from an NFTokenID
// NFTokenID format: Flags(2) + TransferFee(2) + Issuer(20) + Taxon(4) + Sequence(4)
func getNFTIssuer(nftokenID [32]byte) [20]byte {
	var issuer [20]byte
	copy(issuer[:], nftokenID[4:24])
	return issuer
}

// getNFTTransferFee extracts the transfer fee from an NFTokenID
func getNFTTransferFee(nftokenID [32]byte) uint16 {
	return binary.BigEndian.Uint16(nftokenID[2:4])
}

// getNFTFlagsFromID extracts the flags from an NFTokenID
func getNFTFlagsFromID(nftokenID [32]byte) uint16 {
	return binary.BigEndian.Uint16(nftokenID[0:2])
}

// cipheredTaxon computes the ciphered taxon value
// This matches rippled's nft::cipheredTaxon function
func cipheredTaxon(tokenSeq uint32, taxon uint32) uint32 {
	// Simple linear congruential generator to prevent taxon enumeration
	// Matching rippled: (taxon ^ ((tokenSeq ^ 384160001) * 2357503715))
	return taxon ^ ((tokenSeq ^ 384160001) * 2357503715)
}

// generateNFTokenID generates an NFTokenID based on the minting parameters
// Reference: rippled NFTokenMint.cpp createNFTokenID
func generateNFTokenID(issuer [20]byte, taxon, sequence uint32, flags uint16, transferFee uint16) [32]byte {
	var tokenID [32]byte

	// NFTokenID format (32 bytes):
	// Bytes 0-1: Flags (2 bytes, big endian)
	// Bytes 2-3: TransferFee (2 bytes, big endian)
	// Bytes 4-23: Issuer AccountID (20 bytes)
	// Bytes 24-27: Taxon (ciphered, 4 bytes, big endian)
	// Bytes 28-31: Sequence (4 bytes, big endian)

	binary.BigEndian.PutUint16(tokenID[0:2], flags)
	binary.BigEndian.PutUint16(tokenID[2:4], transferFee)
	copy(tokenID[4:24], issuer[:])

	// Cipher the taxon using rippled's algorithm to prevent enumeration
	ciphered := cipheredTaxon(sequence, taxon)
	binary.BigEndian.PutUint32(tokenID[24:28], ciphered)
	binary.BigEndian.PutUint32(tokenID[28:32], sequence)

	return tokenID
}

// insertNFTokenSorted inserts an NFToken into the slice maintaining sorted order
func insertNFTokenSorted(tokens []sle.NFTokenData, newToken sle.NFTokenData) []sle.NFTokenData {
	// Find insertion point (sorted by NFTokenID)
	pos := 0
	for i, t := range tokens {
		if compareNFTokenID(newToken.NFTokenID, t.NFTokenID) < 0 {
			pos = i
			break
		}
		pos = i + 1
	}
	// Insert at position
	tokens = append(tokens, sle.NFTokenData{})
	copy(tokens[pos+1:], tokens[pos:])
	tokens[pos] = newToken
	return tokens
}

// compareNFTokenID compares two NFTokenIDs lexicographically
func compareNFTokenID(a, b [32]byte) int {
	for i := 0; i < 32; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// getNFTPageKey returns the low 96 bits of an NFTokenID (for page grouping)
func getNFTPageKey(nftokenID [32]byte) [32]byte {
	var result [32]byte
	for i := 0; i < 32; i++ {
		result[i] = nftokenID[i] & nftPageMask[i]
	}
	return result
}

// insertNFTokenResult contains the result of inserting an NFToken
type insertNFTokenResult struct {
	Result       tx.Result
	PagesCreated int // Number of new pages created (0 or 1 or 2 in split scenarios)
}

// insertNFToken inserts an NFToken into the owner's token directory
// Reference: rippled NFTokenUtils.cpp insertToken and getPageForToken
// Returns the result and the number of new pages created (for owner count adjustment)
func insertNFToken(ownerID [20]byte, token sle.NFTokenData, view tx.LedgerView) insertNFTokenResult {
	// Find the appropriate page for this token
	pageKey := keylet.NFTokenPage(ownerID, token.NFTokenID)

	// Check if page exists
	exists, _ := view.Exists(pageKey)
	if !exists {
		// Create new page
		page := &sle.NFTokenPageData{
			NFTokens: []sle.NFTokenData{token},
		}

		pageData, err := serializeNFTokenPage(page)
		if err != nil {
			return insertNFTokenResult{Result: tx.TefINTERNAL}
		}

		// Insert tracked automatically by ApplyStateTable
		if err := view.Insert(pageKey, pageData); err != nil {
			return insertNFTokenResult{Result: tx.TefINTERNAL}
		}

		return insertNFTokenResult{Result: tx.TesSUCCESS, PagesCreated: 1}
	}

	// Read existing page
	pageData, err := view.Read(pageKey)
	if err != nil {
		return insertNFTokenResult{Result: tx.TefINTERNAL}
	}

	page, err := sle.ParseNFTokenPage(pageData)
	if err != nil {
		return insertNFTokenResult{Result: tx.TefINTERNAL}
	}

	// Check if page has room
	if len(page.NFTokens) < dirMaxTokensPerPage {
		// Page has room, just add the token
		page.NFTokens = insertNFTokenSorted(page.NFTokens, token)

		updatedPageData, err := serializeNFTokenPage(page)
		if err != nil {
			return insertNFTokenResult{Result: tx.TefINTERNAL}
		}

		// Update tracked automatically by ApplyStateTable
		if err := view.Update(pageKey, updatedPageData); err != nil {
			return insertNFTokenResult{Result: tx.TefINTERNAL}
		}

		return insertNFTokenResult{Result: tx.TesSUCCESS, PagesCreated: 0}
	}

	// Page is full - need to split
	// Reference: rippled NFTokenUtils.cpp getPageForToken (page splitting logic)
	// Find the split point - try to split at halfway, but keep equivalent NFTs together
	splitIdx := dirMaxTokensPerPage / 2

	// Check if all tokens on the page have the same low 96 bits (equivalent NFTs)
	tokenPageKey := getNFTPageKey(token.NFTokenID)
	firstPageKey := getNFTPageKey(page.NFTokens[0].NFTokenID)

	// If all tokens are equivalent and new token is also equivalent, can't insert
	allEquivalent := true
	for _, t := range page.NFTokens {
		if getNFTPageKey(t.NFTokenID) != firstPageKey {
			allEquivalent = false
			break
		}
	}
	if allEquivalent && tokenPageKey == firstPageKey {
		// Page is full of equivalent tokens and new token is also equivalent
		// Cannot store this NFT
		return insertNFTokenResult{Result: tx.TecNO_SUITABLE_NFTOKEN_PAGE}
	}

	// Add the token first (temporarily exceeding limit)
	page.NFTokens = insertNFTokenSorted(page.NFTokens, token)

	// Find split point that keeps equivalent NFTs together
	midKey := getNFTPageKey(page.NFTokens[splitIdx].NFTokenID)

	// Find first token after splitIdx that has different low 96 bits
	actualSplitIdx := splitIdx
	for i := splitIdx; i < len(page.NFTokens); i++ {
		if getNFTPageKey(page.NFTokens[i].NFTokenID) != midKey {
			actualSplitIdx = i
			break
		}
		actualSplitIdx = i + 1
	}

	// If couldn't find split point in second half, try first half
	if actualSplitIdx >= len(page.NFTokens) {
		for i := 0; i < splitIdx; i++ {
			if getNFTPageKey(page.NFTokens[i].NFTokenID) == midKey {
				actualSplitIdx = i
				break
			}
		}
	}

	// Split the page
	firstHalf := make([]sle.NFTokenData, actualSplitIdx)
	copy(firstHalf, page.NFTokens[:actualSplitIdx])
	secondHalf := make([]sle.NFTokenData, len(page.NFTokens)-actualSplitIdx)
	copy(secondHalf, page.NFTokens[actualSplitIdx:])

	// Update current page with second half
	page.NFTokens = secondHalf
	updatedPageData, err := serializeNFTokenPage(page)
	if err != nil {
		return insertNFTokenResult{Result: tx.TefINTERNAL}
	}
	if err := view.Update(pageKey, updatedPageData); err != nil {
		return insertNFTokenResult{Result: tx.TefINTERNAL}
	}

	// Create new page for first half
	// Use the last token ID in first half to generate new page keylet
	newPageTokenID := firstHalf[len(firstHalf)-1].NFTokenID
	newPageKey := keylet.NFTokenPage(ownerID, newPageTokenID)

	newPage := &sle.NFTokenPageData{
		NFTokens:    firstHalf,
		NextPageMin: pageKey.Key,
	}
	if page.PreviousPageMin != [32]byte{} {
		newPage.PreviousPageMin = page.PreviousPageMin
	}

	// Update original page's previous pointer
	page.PreviousPageMin = newPageKey.Key
	updatedPageData, err = serializeNFTokenPage(page)
	if err != nil {
		return insertNFTokenResult{Result: tx.TefINTERNAL}
	}
	if err := view.Update(pageKey, updatedPageData); err != nil {
		return insertNFTokenResult{Result: tx.TefINTERNAL}
	}

	// Insert new page - changes tracked automatically by ApplyStateTable
	newPageData, err := serializeNFTokenPage(newPage)
	if err != nil {
		return insertNFTokenResult{Result: tx.TefINTERNAL}
	}
	if err := view.Insert(newPageKey, newPageData); err != nil {
		return insertNFTokenResult{Result: tx.TefINTERNAL}
	}

	// One new page created from the split
	return insertNFTokenResult{Result: tx.TesSUCCESS, PagesCreated: 1}
}

// deleteNFTokenOffers deletes offers for an NFToken (sell or buy offers)
// Reference: rippled NFTokenUtils.cpp removeTokenOffersWithLimit
// Returns the number of offers deleted
func deleteNFTokenOffers(tokenID [32]byte, sellOffers bool, limit int, view tx.LedgerView) int {
	if limit <= 0 {
		return 0
	}

	// Get the appropriate directory keylet
	var dirKey keylet.Keylet
	if sellOffers {
		dirKey = keylet.NFTSells(tokenID)
	} else {
		dirKey = keylet.NFTBuys(tokenID)
	}

	// Check if directory exists
	exists, _ := view.Exists(dirKey)
	if !exists {
		return 0
	}

	// Read the directory
	dirData, err := view.Read(dirKey)
	if err != nil {
		return 0
	}

	// Parse directory to get offer indexes
	// The directory contains a list of offer keys (Hash256 indexes)
	offerIndexes := parseDirectoryIndexes(dirData)
	if len(offerIndexes) == 0 {
		return 0
	}

	deletedCount := 0
	for _, offerIndex := range offerIndexes {
		if deletedCount >= limit {
			break
		}

		// Create keylet for the offer
		offerKey := keylet.Keylet{Key: offerIndex}

		// Read the offer
		offerData, err := view.Read(offerKey)
		if err != nil {
			continue
		}

		// Parse the offer to get owner info
		offer, err := sle.ParseNFTokenOffer(offerData)
		if err != nil {
			continue
		}

		// Get owner account to update owner count and potentially refund
		ownerKey := keylet.Account(offer.Owner)
		ownerData, err := view.Read(ownerKey)
		if err != nil {
			continue
		}
		ownerAccount, err := sle.ParseAccountRoot(ownerData)
		if err != nil {
			continue
		}

		// If this was a buy offer, refund the escrowed amount
		if offer.Flags&lsfSellNFToken == 0 && offer.Amount > 0 {
			ownerAccount.Balance += offer.Amount
		}

		// Decrease owner count
		if ownerAccount.OwnerCount > 0 {
			ownerAccount.OwnerCount--
		}

		// Update owner account
		ownerUpdatedData, err := sle.SerializeAccountRoot(ownerAccount)
		if err != nil {
			continue
		}
		if err := view.Update(ownerKey, ownerUpdatedData); err != nil {
			continue
		}

		// Delete the offer - tracked automatically by ApplyStateTable
		if err := view.Erase(offerKey); err != nil {
			continue
		}

		deletedCount++
	}

	// If all offers were deleted, remove the directory - tracked automatically
	if deletedCount == len(offerIndexes) {
		view.Erase(dirKey)
	}

	return deletedCount
}

// parseDirectoryIndexes parses a directory node to extract the indexes (Hash256 array)
func parseDirectoryIndexes(data []byte) [][32]byte {
	var indexes [][32]byte
	offset := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		header := data[offset]
		offset++

		typeCode := (header >> 4) & 0x0F
		fieldCode := header & 0x0F

		if typeCode == 0 {
			if offset >= len(data) {
				break
			}
			typeCode = data[offset]
			offset++
		}

		if fieldCode == 0 {
			if offset >= len(data) {
				break
			}
			fieldCode = data[offset]
			offset++
		}

		switch typeCode {
		case fieldTypeUInt16:
			if offset+2 > len(data) {
				return indexes
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return indexes
			}
			offset += 4

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return indexes
			}
			offset += 8

		case fieldTypeHash256:
			if offset+32 > len(data) {
				return indexes
			}
			offset += 32

		case 19: // Vector256 (STI_VECTOR256 = 19)
			// This is the Indexes field
			if fieldCode == 1 { // sfIndexes
				// Read VL length
				if offset >= len(data) {
					return indexes
				}
				length := int(data[offset])
				offset++
				if length > 192 {
					// Extended length encoding
					if offset >= len(data) {
						return indexes
					}
					length = 193 + (length-193)*256 + int(data[offset])
					offset++
				}
				// Each index is 32 bytes
				numIndexes := length / 32
				for i := 0; i < numIndexes && offset+32 <= len(data); i++ {
					var idx [32]byte
					copy(idx[:], data[offset:offset+32])
					indexes = append(indexes, idx)
					offset += 32
				}
			}

		case fieldTypeAccount:
			if offset >= len(data) {
				return indexes
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return indexes
			}
			offset += length

		case fieldTypeBlob:
			if offset >= len(data) {
				return indexes
			}
			length := int(data[offset])
			offset++
			if length > 192 {
				if offset >= len(data) {
					return indexes
				}
				length = 193 + (length-193)*256 + int(data[offset])
				offset++
			}
			if offset+length > len(data) {
				return indexes
			}
			offset += length

		default:
			return indexes
		}
	}

	return indexes
}

// acceptNFTokenBrokeredMode handles brokered NFToken sales
// Reference: rippled NFTokenAcceptOffer.cpp doApply (brokered mode) and preclaim
func (n *NFTokenAcceptOffer) acceptNFTokenBrokeredMode(ctx *tx.ApplyContext, accountID [20]byte,
	buyOffer, sellOffer *sle.NFTokenOfferData, buyOfferKey, sellOfferKey keylet.Keylet) tx.Result {

	// Verify both offers are for the same token
	// Reference: rippled NFTokenAcceptOffer.cpp:103
	if buyOffer.NFTokenID != sellOffer.NFTokenID {
		return tx.TecNFTOKEN_BUY_SELL_MISMATCH
	}

	// Verify both offers are for the same asset (currency)
	// Reference: rippled NFTokenAcceptOffer.cpp:107
	// For XRP offers, both AmountIOU should be nil
	// For IOU offers, both should have same currency
	buyIsXRP := buyOffer.AmountIOU == nil
	sellIsXRP := sellOffer.AmountIOU == nil
	if buyIsXRP != sellIsXRP {
		return tx.TecNFTOKEN_BUY_SELL_MISMATCH
	}
	if !buyIsXRP && !sellIsXRP {
		if buyOffer.AmountIOU.Currency != sellOffer.AmountIOU.Currency ||
			buyOffer.AmountIOU.Issuer != sellOffer.AmountIOU.Issuer {
			return tx.TecNFTOKEN_BUY_SELL_MISMATCH
		}
	}

	// The two offers may not form a loop - buyer cannot be seller
	// Reference: rippled NFTokenAcceptOffer.cpp:112-114 (fixNonFungibleTokensV1_2)
	if buyOffer.Owner == sellOffer.Owner {
		return tx.TecCANT_ACCEPT_OWN_NFTOKEN_OFFER
	}

	// Verify the seller owns the token
	sellerID := sellOffer.Owner
	pageKey := keylet.NFTokenPage(sellerID, sellOffer.NFTokenID)
	if _, err := ctx.View.Read(pageKey); err != nil {
		return tx.TecNO_PERMISSION
	}

	// Verify buyer can pay at least what seller asks
	// Reference: rippled NFTokenAcceptOffer.cpp:118
	if buyOffer.Amount < sellOffer.Amount {
		return tx.TecINSUFFICIENT_PAYMENT
	}

	// Check destination constraints
	// Reference: rippled NFTokenAcceptOffer.cpp:122-147
	// After fixNonFungibleTokensV1_2, destination must be the tx submitter
	if buyOffer.HasDestination && buyOffer.Destination != accountID {
		return tx.TecNO_PERMISSION
	}
	if sellOffer.HasDestination && sellOffer.Destination != accountID {
		return tx.TecNO_PERMISSION
	}

	buyerID := buyOffer.Owner

	// Calculate broker fee
	// Reference: rippled NFTokenAcceptOffer.cpp:153-163
	var brokerFee uint64
	if n.NFTokenBrokerFee != nil {
		// Verify broker fee is in same currency as offers
		// Reference: rippled NFTokenAcceptOffer.cpp:155
		brokerFeeIsXRP := n.NFTokenBrokerFee.Currency == ""
		if brokerFeeIsXRP != buyIsXRP {
			return tx.TecNFTOKEN_BUY_SELL_MISMATCH
		}

		if brokerFeeIsXRP {
			var err error
			brokerFee, err = strconv.ParseUint(n.NFTokenBrokerFee.Value, 10, 64)
			if err != nil {
				return tx.TemMALFORMED
			}
		}

		// Broker fee cannot exceed or equal what buyer pays
		// Reference: rippled NFTokenAcceptOffer.cpp:158
		if brokerFee >= buyOffer.Amount {
			return tx.TecINSUFFICIENT_PAYMENT
		}
		// Seller must still get at least what they asked for after broker fee
		// Reference: rippled NFTokenAcceptOffer.cpp:161
		if sellOffer.Amount > buyOffer.Amount-brokerFee {
			return tx.TecINSUFFICIENT_PAYMENT
		}
	}

	// Calculate amounts
	amount := buyOffer.Amount
	var issuerCut uint64

	// Calculate issuer transfer fee if applicable
	transferFee := getNFTTransferFee(sellOffer.NFTokenID)
	issuerID := getNFTIssuer(sellOffer.NFTokenID)
	if transferFee != 0 && amount > 0 {
		// Transfer fee is in basis points (0-50000 = 0-50%)
		// Calculate: amount * transferFee / transferFeeDivisor
		issuerCut = (amount - brokerFee) * uint64(transferFee) / transferFeeDivisor
		// Issuer doesn't get cut from their own sales
		if sellerID == issuerID || buyerID == issuerID {
			issuerCut = 0
		}
	}

	// Pay broker fee
	if brokerFee > 0 {
		ctx.Account.Balance += brokerFee
		amount -= brokerFee
	}

	// Pay issuer cut - update tracked automatically by ApplyStateTable
	if issuerCut > 0 {
		issuerKey := keylet.Account(issuerID)
		issuerData, err := ctx.View.Read(issuerKey)
		if err == nil {
			issuerAccount, err := sle.ParseAccountRoot(issuerData)
			if err == nil {
				issuerAccount.Balance += issuerCut
				issuerUpdatedData, _ := sle.SerializeAccountRoot(issuerAccount)
				ctx.View.Update(issuerKey, issuerUpdatedData)
			}
		}
		amount -= issuerCut
	}

	// Pay seller
	sellerKey := keylet.Account(sellerID)
	sellerData, err := ctx.View.Read(sellerKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	sellerAccount, err := sle.ParseAccountRoot(sellerData)
	if err != nil {
		return tx.TefINTERNAL
	}
	sellerAccount.Balance += amount
	if sellerAccount.OwnerCount > 0 {
		sellerAccount.OwnerCount-- // For sell offer being deleted
	}
	sellerUpdatedData, err := sle.SerializeAccountRoot(sellerAccount)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Update(sellerKey, sellerUpdatedData); err != nil {
		return tx.TefINTERNAL
	}

	// Update buyer's owner count
	buyerKey := keylet.Account(buyerID)
	buyerData, err := ctx.View.Read(buyerKey)
	if err == nil {
		buyerAccount, err := sle.ParseAccountRoot(buyerData)
		if err == nil && buyerAccount.OwnerCount > 0 {
			buyerAccount.OwnerCount-- // For buy offer being deleted
			buyerUpdatedData, _ := sle.SerializeAccountRoot(buyerAccount)
			ctx.View.Update(buyerKey, buyerUpdatedData)
		}
	}

	// Transfer the NFToken from seller to buyer
	if result := transferNFToken(sellerID, buyerID, sellOffer.NFTokenID, ctx.View); result != tx.TesSUCCESS {
		return result
	}

	// Delete both offers - deletions tracked automatically by ApplyStateTable
	if err := ctx.View.Erase(buyOfferKey); err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Erase(sellOfferKey); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}

// acceptNFTokenSellOfferDirect handles direct sell offer acceptance
func (n *NFTokenAcceptOffer) acceptNFTokenSellOfferDirect(ctx *tx.ApplyContext, accountID [20]byte,
	sellOffer *sle.NFTokenOfferData, sellOfferKey keylet.Keylet) tx.Result {

	// Check destination constraint
	if sellOffer.HasDestination && sellOffer.Destination != accountID {
		return tx.TecNO_PERMISSION
	}

	// Verify seller owns the token
	sellerID := sellOffer.Owner
	pageKey := keylet.NFTokenPage(sellerID, sellOffer.NFTokenID)
	if _, err := ctx.View.Read(pageKey); err != nil {
		return tx.TecNO_PERMISSION
	}

	amount := sellOffer.Amount
	var issuerCut uint64

	// Calculate issuer transfer fee
	transferFee := getNFTTransferFee(sellOffer.NFTokenID)
	issuerID := getNFTIssuer(sellOffer.NFTokenID)
	if transferFee != 0 && amount > 0 {
		issuerCut = amount * uint64(transferFee) / transferFeeDivisor
		if sellerID == issuerID || accountID == issuerID {
			issuerCut = 0
		}
	}

	// Buyer pays
	totalCost := amount
	if ctx.Account.Balance < totalCost {
		return tx.TecINSUFFICIENT_FUNDS
	}
	ctx.Account.Balance -= totalCost

	// Pay issuer cut
	if issuerCut > 0 {
		issuerKey := keylet.Account(issuerID)
		issuerData, err := ctx.View.Read(issuerKey)
		if err == nil {
			issuerAccount, err := sle.ParseAccountRoot(issuerData)
			if err == nil {
				issuerAccount.Balance += issuerCut
				issuerUpdatedData, _ := sle.SerializeAccountRoot(issuerAccount)
				ctx.View.Update(issuerKey, issuerUpdatedData)
			}
		}
		amount -= issuerCut
	}

	// Pay seller
	sellerKey := keylet.Account(sellerID)
	sellerData, err := ctx.View.Read(sellerKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	sellerAccount, err := sle.ParseAccountRoot(sellerData)
	if err != nil {
		return tx.TefINTERNAL
	}
	sellerAccount.Balance += amount
	if sellerAccount.OwnerCount > 0 {
		sellerAccount.OwnerCount--
	}
	sellerUpdatedData, err := sle.SerializeAccountRoot(sellerAccount)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Update(sellerKey, sellerUpdatedData); err != nil {
		return tx.TefINTERNAL
	}

	// Transfer the NFToken
	if result := transferNFToken(sellerID, accountID, sellOffer.NFTokenID, ctx.View); result != tx.TesSUCCESS {
		return result
	}

	// Delete offer - tracked automatically by ApplyStateTable
	if err := ctx.View.Erase(sellOfferKey); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}

// acceptNFTokenBuyOfferDirect handles direct buy offer acceptance
func (n *NFTokenAcceptOffer) acceptNFTokenBuyOfferDirect(ctx *tx.ApplyContext, accountID [20]byte,
	buyOffer *sle.NFTokenOfferData, buyOfferKey keylet.Keylet) tx.Result {

	// Verify account owns the token
	pageKey := keylet.NFTokenPage(accountID, buyOffer.NFTokenID)
	if _, err := ctx.View.Read(pageKey); err != nil {
		return tx.TecNO_PERMISSION
	}

	// Check destination constraint
	if buyOffer.HasDestination && buyOffer.Destination != accountID {
		return tx.TecNO_PERMISSION
	}

	buyerID := buyOffer.Owner
	amount := buyOffer.Amount // Already escrowed
	var issuerCut uint64

	// Calculate issuer transfer fee
	transferFee := getNFTTransferFee(buyOffer.NFTokenID)
	issuerID := getNFTIssuer(buyOffer.NFTokenID)
	if transferFee != 0 && amount > 0 {
		issuerCut = amount * uint64(transferFee) / transferFeeDivisor
		if accountID == issuerID || buyerID == issuerID {
			issuerCut = 0
		}
	}

	// Pay issuer cut
	if issuerCut > 0 {
		issuerKey := keylet.Account(issuerID)
		issuerData, err := ctx.View.Read(issuerKey)
		if err == nil {
			issuerAccount, err := sle.ParseAccountRoot(issuerData)
			if err == nil {
				issuerAccount.Balance += issuerCut
				issuerUpdatedData, _ := sle.SerializeAccountRoot(issuerAccount)
				ctx.View.Update(issuerKey, issuerUpdatedData)
			}
		}
		amount -= issuerCut
	}

	// Pay seller (the account accepting the buy offer)
	ctx.Account.Balance += amount

	// Update buyer's owner count
	buyerKey := keylet.Account(buyerID)
	buyerData, err := ctx.View.Read(buyerKey)
	if err == nil {
		buyerAccount, err := sle.ParseAccountRoot(buyerData)
		if err == nil && buyerAccount.OwnerCount > 0 {
			buyerAccount.OwnerCount--
			buyerUpdatedData, _ := sle.SerializeAccountRoot(buyerAccount)
			ctx.View.Update(buyerKey, buyerUpdatedData)
		}
	}

	// Transfer the NFToken
	if result := transferNFToken(accountID, buyerID, buyOffer.NFTokenID, ctx.View); result != tx.TesSUCCESS {
		return result
	}

	// Delete offer - tracked automatically by ApplyStateTable
	if err := ctx.View.Erase(buyOfferKey); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}

// transferNFToken transfers an NFToken from one account to another
func transferNFToken(from, to [20]byte, tokenID [32]byte, view tx.LedgerView) tx.Result {
	// Remove from sender's page
	fromPageKey := keylet.NFTokenPage(from, tokenID)
	fromPageData, err := view.Read(fromPageKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	fromPage, err := sle.ParseNFTokenPage(fromPageData)
	if err != nil {
		return tx.TefINTERNAL
	}

	var tokenData sle.NFTokenData
	found := false
	for i, t := range fromPage.NFTokens {
		if t.NFTokenID == tokenID {
			tokenData = t
			fromPage.NFTokens = append(fromPage.NFTokens[:i], fromPage.NFTokens[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return tx.TefINTERNAL
	}

	// Update or delete sender's page - changes tracked automatically by ApplyStateTable
	fromKey := keylet.Account(from)
	if len(fromPage.NFTokens) == 0 {
		if err := view.Erase(fromPageKey); err != nil {
			return tx.TefINTERNAL
		}
		// Decrease sender's owner count
		fromData, err := view.Read(fromKey)
		if err == nil {
			fromAccount, err := sle.ParseAccountRoot(fromData)
			if err == nil && fromAccount.OwnerCount > 0 {
				fromAccount.OwnerCount--
				fromUpdatedData, _ := sle.SerializeAccountRoot(fromAccount)
				view.Update(fromKey, fromUpdatedData)
			}
		}
	} else {
		fromPageUpdated, err := serializeNFTokenPage(fromPage)
		if err != nil {
			return tx.TefINTERNAL
		}
		if err := view.Update(fromPageKey, fromPageUpdated); err != nil {
			return tx.TefINTERNAL
		}
	}

	// Add to recipient's page - changes tracked automatically by ApplyStateTable
	toPageKey := keylet.NFTokenPage(to, tokenID)
	exists, _ := view.Exists(toPageKey)
	if exists {
		toPageData, err := view.Read(toPageKey)
		if err != nil {
			return tx.TefINTERNAL
		}
		toPage, err := sle.ParseNFTokenPage(toPageData)
		if err != nil {
			return tx.TefINTERNAL
		}
		toPage.NFTokens = insertNFTokenSorted(toPage.NFTokens, tokenData)
		toPageUpdated, err := serializeNFTokenPage(toPage)
		if err != nil {
			return tx.TefINTERNAL
		}
		if err := view.Update(toPageKey, toPageUpdated); err != nil {
			return tx.TefINTERNAL
		}
	} else {
		newPage := &sle.NFTokenPageData{
			NFTokens: []sle.NFTokenData{tokenData},
		}
		newPageData, err := serializeNFTokenPage(newPage)
		if err != nil {
			return tx.TefINTERNAL
		}
		if err := view.Insert(toPageKey, newPageData); err != nil {
			return tx.TefINTERNAL
		}
		// Increase recipient's owner count
		toKey := keylet.Account(to)
		toData, err := view.Read(toKey)
		if err == nil {
			toAccount, err := sle.ParseAccountRoot(toData)
			if err == nil {
				toAccount.OwnerCount++
				toUpdatedData, _ := sle.SerializeAccountRoot(toAccount)
				view.Update(toKey, toUpdatedData)
			}
		}
	}

	return tx.TesSUCCESS
}

// serializeNFTokenPage serializes an NFToken page ledger entry
func serializeNFTokenPage(page *sle.NFTokenPageData) ([]byte, error) {
	jsonObj := map[string]any{
		"LedgerEntryType": "NFTokenPage",
		"Flags":           uint32(0),
	}

	var emptyHash [32]byte
	if page.PreviousPageMin != emptyHash {
		jsonObj["PreviousPageMin"] = strings.ToUpper(hex.EncodeToString(page.PreviousPageMin[:]))
	}

	if page.NextPageMin != emptyHash {
		jsonObj["NextPageMin"] = strings.ToUpper(hex.EncodeToString(page.NextPageMin[:]))
	}

	// Build NFTokens array
	if len(page.NFTokens) > 0 {
		nfTokens := make([]map[string]any, len(page.NFTokens))
		for i, token := range page.NFTokens {
			nfToken := map[string]any{
				"NFToken": map[string]any{
					"NFTokenID": strings.ToUpper(hex.EncodeToString(token.NFTokenID[:])),
				},
			}
			if token.URI != "" {
				nfToken["NFToken"].(map[string]any)["URI"] = token.URI
			}
			nfTokens[i] = nfToken
		}
		jsonObj["NFTokens"] = nfTokens
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode NFTokenPage: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// serializeNFTokenOffer serializes an NFToken offer ledger entry
func serializeNFTokenOffer(nftTx *NFTokenCreateOffer, ownerID [20]byte, tokenID [32]byte, amount uint64, sequence uint32) ([]byte, error) {
	ownerAddress, err := addresscodec.EncodeAccountIDToClassicAddress(ownerID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode owner address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "NFTokenOffer",
		"Account":         ownerAddress,
		"Amount":          fmt.Sprintf("%d", amount),
		"NFTokenID":       strings.ToUpper(hex.EncodeToString(tokenID[:])),
		"OwnerNode":       "0",
		"Flags":           nftTx.GetFlags(),
	}

	if nftTx.Expiration != nil {
		jsonObj["Expiration"] = *nftTx.Expiration
	}

	if nftTx.Destination != "" {
		jsonObj["Destination"] = nftTx.Destination
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode NFTokenOffer: %w", err)
	}

	return hex.DecodeString(hexStr)
}
