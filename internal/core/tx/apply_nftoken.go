package tx

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// NFToken data structures

// NFTokenPageData represents an NFToken page ledger entry
type NFTokenPageData struct {
	PreviousPageMin [32]byte
	NextPageMin     [32]byte
	NFTokens        []NFTokenData
}

// NFTokenData represents an individual NFToken within a page
type NFTokenData struct {
	NFTokenID [32]byte
	URI       string
}

// NFTokenOfferData represents an NFToken offer ledger entry
type NFTokenOfferData struct {
	Owner          [20]byte
	NFTokenID      [32]byte
	Amount         uint64
	AmountIOU      *NFTIOUAmount // For IOU amounts
	Flags          uint32
	Destination    [20]byte
	Expiration     uint32
	HasDestination bool
}

// NFTIOUAmount represents an IOU amount for NFToken offers
// This is a simplified version for NFToken offer storage
type NFTIOUAmount struct {
	Currency string
	Issuer   [20]byte
	Value    string
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

// applyNFTokenMint applies an NFTokenMint transaction
// Reference: rippled NFTokenMint.cpp doApply
func (e *Engine) applyNFTokenMint(tx *NFTokenMint, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Determine the issuer
	var issuerID [20]byte
	var issuerAccount *AccountRoot
	var issuerKey keylet.Keylet

	if tx.Issuer != "" {
		var err error
		issuerID, err = decodeAccountID(tx.Issuer)
		if err != nil {
			return TemINVALID
		}

		// Read issuer account for MintedNFTokens tracking
		issuerKey = keylet.Account(issuerID)
		issuerData, err := e.view.Read(issuerKey)
		if err != nil {
			return TecNO_ISSUER
		}
		issuerAccount, err = parseAccountRoot(issuerData)
		if err != nil {
			return TefINTERNAL
		}

		// Verify that Account is authorized to mint for this issuer
		// The issuer must have set Account as their NFTokenMinter
		if issuerAccount.NFTokenMinter != tx.Account {
			return TecNO_PERMISSION
		}
	} else {
		issuerID = accountID
		issuerAccount = account
	}

	// Get the token sequence from MintedNFTokens
	tokenSeq := issuerAccount.MintedNFTokens

	// Check for overflow
	if tokenSeq+1 < tokenSeq {
		return TecMAX_SEQUENCE_REACHED
	}

	// Get flags for the token from transaction flags
	txFlags := tx.GetFlags()
	var tokenFlags uint16
	if txFlags&NFTokenMintFlagBurnable != 0 {
		tokenFlags |= nftFlagBurnable
	}
	if txFlags&NFTokenMintFlagOnlyXRP != 0 {
		tokenFlags |= nftFlagOnlyXRP
	}
	if txFlags&NFTokenMintFlagTrustLine != 0 {
		tokenFlags |= nftFlagTrustLine
	}
	if txFlags&NFTokenMintFlagTransferable != 0 {
		tokenFlags |= nftFlagTransferable
	}
	if txFlags&NFTokenMintFlagMutable != 0 {
		tokenFlags |= nftFlagMutable
	}

	// Get transfer fee
	var transferFee uint16
	if tx.TransferFee != nil {
		transferFee = *tx.TransferFee
	}

	// Generate the NFTokenID
	tokenID := generateNFTokenID(issuerID, tx.NFTokenTaxon, tokenSeq, tokenFlags, transferFee)

	// Insert the NFToken into the owner's token directory
	// Reference: rippled NFTokenUtils.cpp insertToken
	newToken := NFTokenData{
		NFTokenID: tokenID,
		URI:       tx.URI,
	}

	insertResult := e.insertNFToken(accountID, newToken, metadata)
	if insertResult.Result != TesSUCCESS {
		return insertResult.Result
	}

	// Update owner count based on pages created
	account.OwnerCount += uint32(insertResult.PagesCreated)

	// Update MintedNFTokens on the issuer account
	issuerAccount.MintedNFTokens = tokenSeq + 1

	// If issuer is different from minter, update the issuer account
	if tx.Issuer != "" {
		issuerUpdatedData, err := serializeAccountRoot(issuerAccount)
		if err != nil {
			return TefINTERNAL
		}
		if err := e.view.Update(issuerKey, issuerUpdatedData); err != nil {
			return TefINTERNAL
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "AccountRoot",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(issuerKey.Key[:])),
			FinalFields: map[string]any{
				"MintedNFTokens": issuerAccount.MintedNFTokens,
			},
		})
	}

	// Check reserve if pages were created (owner count increased)
	if insertResult.PagesCreated > 0 {
		reserve := e.AccountReserve(account.OwnerCount)
		if account.Balance < reserve {
			return TecINSUFFICIENT_RESERVE
		}
	}

	return TesSUCCESS
}

// insertNFTokenSorted inserts an NFToken into the slice maintaining sorted order
func insertNFTokenSorted(tokens []NFTokenData, newToken NFTokenData) []NFTokenData {
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
	tokens = append(tokens, NFTokenData{})
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

// nftPageMask is the mask for the low 96 bits of an NFTokenID
// This is used to group equivalent NFTs on the same page
var nftPageMask = [32]byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
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
	Result        Result
	PagesCreated  int // Number of new pages created (0 or 1 or 2 in split scenarios)
}

// insertNFToken inserts an NFToken into the owner's token directory
// Reference: rippled NFTokenUtils.cpp insertToken and getPageForToken
// Returns the result and the number of new pages created (for owner count adjustment)
func (e *Engine) insertNFToken(ownerID [20]byte, token NFTokenData, metadata *Metadata) insertNFTokenResult {
	// Find the appropriate page for this token
	pageKey := keylet.NFTokenPage(ownerID, token.NFTokenID)

	// Check if page exists
	exists, _ := e.view.Exists(pageKey)
	if !exists {
		// Create new page
		page := &NFTokenPageData{
			NFTokens: []NFTokenData{token},
		}

		pageData, err := serializeNFTokenPage(page)
		if err != nil {
			return insertNFTokenResult{Result: TefINTERNAL}
		}

		if err := e.view.Insert(pageKey, pageData); err != nil {
			return insertNFTokenResult{Result: TefINTERNAL}
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(pageKey.Key[:])),
			NewFields: map[string]any{
				"NFTokenID": strings.ToUpper(hex.EncodeToString(token.NFTokenID[:])),
			},
		})

		return insertNFTokenResult{Result: TesSUCCESS, PagesCreated: 1}
	}

	// Read existing page
	pageData, err := e.view.Read(pageKey)
	if err != nil {
		return insertNFTokenResult{Result: TefINTERNAL}
	}

	page, err := parseNFTokenPage(pageData)
	if err != nil {
		return insertNFTokenResult{Result: TefINTERNAL}
	}

	// Check if page has room
	if len(page.NFTokens) < dirMaxTokensPerPage {
		// Page has room, just add the token
		page.NFTokens = insertNFTokenSorted(page.NFTokens, token)

		updatedPageData, err := serializeNFTokenPage(page)
		if err != nil {
			return insertNFTokenResult{Result: TefINTERNAL}
		}

		if err := e.view.Update(pageKey, updatedPageData); err != nil {
			return insertNFTokenResult{Result: TefINTERNAL}
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(pageKey.Key[:])),
		})

		return insertNFTokenResult{Result: TesSUCCESS, PagesCreated: 0}
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
		return insertNFTokenResult{Result: TecNO_SUITABLE_NFTOKEN_PAGE}
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
	firstHalf := make([]NFTokenData, actualSplitIdx)
	copy(firstHalf, page.NFTokens[:actualSplitIdx])
	secondHalf := make([]NFTokenData, len(page.NFTokens)-actualSplitIdx)
	copy(secondHalf, page.NFTokens[actualSplitIdx:])

	// Update current page with second half
	page.NFTokens = secondHalf
	updatedPageData, err := serializeNFTokenPage(page)
	if err != nil {
		return insertNFTokenResult{Result: TefINTERNAL}
	}
	if err := e.view.Update(pageKey, updatedPageData); err != nil {
		return insertNFTokenResult{Result: TefINTERNAL}
	}

	// Create new page for first half
	// Use the last token ID in first half to generate new page keylet
	newPageTokenID := firstHalf[len(firstHalf)-1].NFTokenID
	newPageKey := keylet.NFTokenPage(ownerID, newPageTokenID)

	newPage := &NFTokenPageData{
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
		return insertNFTokenResult{Result: TefINTERNAL}
	}
	if err := e.view.Update(pageKey, updatedPageData); err != nil {
		return insertNFTokenResult{Result: TefINTERNAL}
	}

	// Insert new page
	newPageData, err := serializeNFTokenPage(newPage)
	if err != nil {
		return insertNFTokenResult{Result: TefINTERNAL}
	}
	if err := e.view.Insert(newPageKey, newPageData); err != nil {
		return insertNFTokenResult{Result: TefINTERNAL}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "NFTokenPage",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(pageKey.Key[:])),
	})
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "NFTokenPage",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(newPageKey.Key[:])),
	})

	// One new page created from the split
	return insertNFTokenResult{Result: TesSUCCESS, PagesCreated: 1}
}

// applyNFTokenBurn applies an NFTokenBurn transaction
// Reference: rippled NFTokenBurn.cpp doApply
func (e *Engine) applyNFTokenBurn(tx *NFTokenBurn, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Parse the token ID
	tokenIDBytes, err := hex.DecodeString(tx.NFTokenID)
	if err != nil || len(tokenIDBytes) != 32 {
		return TemINVALID
	}

	var tokenID [32]byte
	copy(tokenID[:], tokenIDBytes)

	// Determine the owner
	var ownerID [20]byte
	if tx.Owner != "" {
		ownerID, err = decodeAccountID(tx.Owner)
		if err != nil {
			return TemINVALID
		}
	} else {
		ownerID = accountID
	}

	// Find the NFToken page
	pageKey := keylet.NFTokenPage(ownerID, tokenID)

	pageData, err := e.view.Read(pageKey)
	if err != nil {
		return TecNO_ENTRY
	}

	// Parse the page
	page, err := parseNFTokenPage(pageData)
	if err != nil {
		return TefINTERNAL
	}

	// Find the token
	found := false
	for _, token := range page.NFTokens {
		if token.NFTokenID == tokenID {
			found = true
			break
		}
	}

	if !found {
		return TecNO_ENTRY
	}

	// Verify burn authorization
	// Owner can always burn, issuer can burn if flagBurnable is set
	if ownerID != accountID {
		nftFlags := getNFTFlagsFromID(tokenID)
		if nftFlags&nftFlagBurnable == 0 {
			return TecNO_PERMISSION
		}

		// Check if the account is the issuer or an authorized minter
		issuerID := getNFTIssuer(tokenID)
		if issuerID != accountID {
			// Not the issuer, check if authorized minter
			issuerKey := keylet.Account(issuerID)
			issuerData, err := e.view.Read(issuerKey)
			if err != nil {
				return TecNO_PERMISSION
			}
			issuerAccount, err := parseAccountRoot(issuerData)
			if err != nil {
				return TefINTERNAL
			}
			if issuerAccount.NFTokenMinter != tx.Account {
				return TecNO_PERMISSION
			}
		}
	}

	// Find and remove the token
	for i, token := range page.NFTokens {
		if token.NFTokenID == tokenID {
			page.NFTokens = append(page.NFTokens[:i], page.NFTokens[i+1:]...)
			break
		}
	}

	// Get owner account for OwnerCount update (if different from transaction account)
	var ownerAccount *AccountRoot
	var ownerKey keylet.Keylet
	if ownerID != accountID {
		ownerKey = keylet.Account(ownerID)
		ownerData, err := e.view.Read(ownerKey)
		if err != nil {
			return TefINTERNAL
		}
		ownerAccount, err = parseAccountRoot(ownerData)
		if err != nil {
			return TefINTERNAL
		}
	} else {
		ownerAccount = account
	}

	// Update or delete the page
	if len(page.NFTokens) == 0 {
		// Delete empty page
		if err := e.view.Erase(pageKey); err != nil {
			return TefINTERNAL
		}

		if ownerAccount.OwnerCount > 0 {
			ownerAccount.OwnerCount--
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(pageKey.Key[:])),
		})
	} else {
		// Update page
		updatedPageData, err := serializeNFTokenPage(page)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Update(pageKey, updatedPageData); err != nil {
			return TefINTERNAL
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(pageKey.Key[:])),
		})
	}

	// Update owner account if different from transaction sender
	if ownerID != accountID {
		ownerUpdatedData, err := serializeAccountRoot(ownerAccount)
		if err != nil {
			return TefINTERNAL
		}
		if err := e.view.Update(ownerKey, ownerUpdatedData); err != nil {
			return TefINTERNAL
		}
	}

	// Update BurnedNFTokens on the issuer
	issuerID := getNFTIssuer(tokenID)
	issuerKey := keylet.Account(issuerID)
	issuerData, err := e.view.Read(issuerKey)
	if err == nil {
		issuerAccount, err := parseAccountRoot(issuerData)
		if err == nil {
			issuerAccount.BurnedNFTokens++
			issuerUpdatedData, err := serializeAccountRoot(issuerAccount)
			if err == nil {
				e.view.Update(issuerKey, issuerUpdatedData)

				metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
					NodeType:        "ModifiedNode",
					LedgerEntryType: "AccountRoot",
					LedgerIndex:     strings.ToUpper(hex.EncodeToString(issuerKey.Key[:])),
					FinalFields: map[string]any{
						"BurnedNFTokens": issuerAccount.BurnedNFTokens,
					},
				})
			}
		}
	}

	// Delete associated buy and sell offers (up to maxDeletableTokenOfferEntries)
	// Reference: rippled NFTokenBurn.cpp:108-139
	deletedCount := e.deleteNFTokenOffers(tokenID, true, maxDeletableTokenOfferEntries, metadata)
	if deletedCount < maxDeletableTokenOfferEntries {
		e.deleteNFTokenOffers(tokenID, false, maxDeletableTokenOfferEntries-deletedCount, metadata)
	}

	return TesSUCCESS
}

// deleteNFTokenOffers deletes offers for an NFToken (sell or buy offers)
// Reference: rippled NFTokenUtils.cpp removeTokenOffersWithLimit
// Returns the number of offers deleted
func (e *Engine) deleteNFTokenOffers(tokenID [32]byte, sellOffers bool, limit int, metadata *Metadata) int {
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
	exists, _ := e.view.Exists(dirKey)
	if !exists {
		return 0
	}

	// Read the directory
	dirData, err := e.view.Read(dirKey)
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
		offerData, err := e.view.Read(offerKey)
		if err != nil {
			continue
		}

		// Parse the offer to get owner info
		offer, err := parseNFTokenOffer(offerData)
		if err != nil {
			continue
		}

		// Get owner account to update owner count and potentially refund
		ownerKey := keylet.Account(offer.Owner)
		ownerData, err := e.view.Read(ownerKey)
		if err != nil {
			continue
		}
		ownerAccount, err := parseAccountRoot(ownerData)
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
		ownerUpdatedData, err := serializeAccountRoot(ownerAccount)
		if err != nil {
			continue
		}
		if err := e.view.Update(ownerKey, ownerUpdatedData); err != nil {
			continue
		}

		// Delete the offer
		if err := e.view.Erase(offerKey); err != nil {
			continue
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "NFTokenOffer",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(offerKey.Key[:])),
		})

		deletedCount++
	}

	// If all offers were deleted, remove the directory
	if deletedCount == len(offerIndexes) {
		e.view.Erase(dirKey)
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "DirectoryNode",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(dirKey.Key[:])),
		})
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

// applyNFTokenCreateOffer applies an NFTokenCreateOffer transaction
// Reference: rippled NFTokenCreateOffer.cpp doApply
func (e *Engine) applyNFTokenCreateOffer(tx *NFTokenCreateOffer, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Parse token ID
	tokenIDBytes, err := hex.DecodeString(tx.NFTokenID)
	if err != nil || len(tokenIDBytes) != 32 {
		return TemINVALID
	}

	var tokenID [32]byte
	copy(tokenID[:], tokenIDBytes)

	// Check expiration
	if tx.Expiration != nil && *tx.Expiration <= e.config.ParentCloseTime {
		return TecEXPIRED
	}

	// Check if this is a sell offer
	isSellOffer := tx.GetFlags()&NFTokenCreateOfferFlagSellNFToken != 0

	// Verify token ownership
	if isSellOffer {
		// For sell offers, verify the sender owns the token
		pageKey := keylet.NFTokenPage(accountID, tokenID)
		pageData, err := e.view.Read(pageKey)
		if err != nil {
			return TecNO_ENTRY
		}
		// Verify token is on the page
		page, err := parseNFTokenPage(pageData)
		if err != nil {
			return TefINTERNAL
		}
		found := false
		for _, t := range page.NFTokens {
			if t.NFTokenID == tokenID {
				found = true
				break
			}
		}
		if !found {
			return TecNO_ENTRY
		}
	} else {
		// For buy offers, verify the owner has the token
		var ownerID [20]byte
		ownerID, err = decodeAccountID(tx.Owner)
		if err != nil {
			return TemINVALID
		}
		pageKey := keylet.NFTokenPage(ownerID, tokenID)
		pageData, err := e.view.Read(pageKey)
		if err != nil {
			return TecNO_ENTRY
		}
		// Verify token is on the page
		page, err := parseNFTokenPage(pageData)
		if err != nil {
			return TefINTERNAL
		}
		found := false
		for _, t := range page.NFTokens {
			if t.NFTokenID == tokenID {
				found = true
				break
			}
		}
		if !found {
			return TecNO_ENTRY
		}
	}

	// Parse amount
	var amountXRP uint64
	if tx.Amount.Currency == "" {
		// XRP amount
		amountXRP, err = strconv.ParseUint(tx.Amount.Value, 10, 64)
		if err != nil {
			return TemMALFORMED
		}
	}

	// For buy offers, escrow the funds
	if !isSellOffer {
		if tx.Amount.Currency == "" && amountXRP > 0 {
			// Check if account has enough balance (including reserve)
			reserve := e.AccountReserve(account.OwnerCount + 1)
			if account.Balance < amountXRP+reserve {
				return TecINSUFFICIENT_FUNDS
			}
			// Escrow the funds (deduct from balance)
			account.Balance -= amountXRP
		}
		// For IOU buy offers, don't escrow but verify funds exist
	}

	// Create the offer using keylet based on account + sequence
	sequence := *tx.GetCommon().Sequence
	offerKey := keylet.NFTokenOffer(accountID, sequence)

	offerData, err := serializeNFTokenOffer(tx, accountID, tokenID, amountXRP, sequence)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Insert(offerKey, offerData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	account.OwnerCount++

	// Check reserve
	reserve := e.AccountReserve(account.OwnerCount)
	if account.Balance < reserve {
		return TecINSUFFICIENT_RESERVE
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "NFTokenOffer",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(offerKey.Key[:])),
		NewFields: map[string]any{
			"Account":   tx.Account,
			"NFTokenID": strings.ToUpper(tx.NFTokenID),
			"Amount":    tx.Amount.Value,
			"Flags":     tx.GetFlags(),
		},
	})

	return TesSUCCESS
}

// applyNFTokenCancelOffer applies an NFTokenCancelOffer transaction
// Reference: rippled NFTokenCancelOffer.cpp doApply and preclaim
func (e *Engine) applyNFTokenCancelOffer(tx *NFTokenCancelOffer, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	for _, offerIDHex := range tx.NFTokenOffers {
		// Parse offer ID
		offerIDBytes, err := hex.DecodeString(offerIDHex)
		if err != nil || len(offerIDBytes) != 32 {
			continue
		}

		var offerKeyBytes [32]byte
		copy(offerKeyBytes[:], offerIDBytes)
		offerKey := keylet.Keylet{Key: offerKeyBytes}

		// Read the offer
		offerData, err := e.view.Read(offerKey)
		if err != nil {
			// Offer doesn't exist - already consumed, skip silently
			continue
		}

		// Parse the offer
		offer, err := parseNFTokenOffer(offerData)
		if err != nil {
			continue
		}

		// Check authorization to cancel
		// Reference: rippled NFTokenCancelOffer.cpp preclaim
		isExpired := offer.Expiration != 0 && offer.Expiration <= e.config.ParentCloseTime
		isOwner := offer.Owner == accountID
		isDestination := offer.HasDestination && offer.Destination == accountID

		// Must be owner, destination, or expired
		if !isOwner && !isDestination && !isExpired {
			return TecNO_PERMISSION
		}

		// Get the offer owner's account to update their owner count and potentially refund
		var ownerAccount *AccountRoot
		var ownerKey keylet.Keylet

		if offer.Owner == accountID {
			ownerAccount = account
		} else {
			ownerKey = keylet.Account(offer.Owner)
			ownerData, err := e.view.Read(ownerKey)
			if err != nil {
				return TefINTERNAL
			}
			ownerAccount, err = parseAccountRoot(ownerData)
			if err != nil {
				return TefINTERNAL
			}
		}

		// If this was a buy offer, refund the escrowed amount to the owner
		if offer.Flags&lsfSellNFToken == 0 {
			// Buy offer - refund escrowed XRP to owner
			ownerAccount.Balance += offer.Amount
		}

		// Decrease owner count for the deleted offer
		if ownerAccount.OwnerCount > 0 {
			ownerAccount.OwnerCount--
		}

		// Update owner account if different from transaction sender
		if offer.Owner != accountID {
			ownerUpdatedData, err := serializeAccountRoot(ownerAccount)
			if err != nil {
				return TefINTERNAL
			}
			if err := e.view.Update(ownerKey, ownerUpdatedData); err != nil {
				return TefINTERNAL
			}

			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "ModifiedNode",
				LedgerEntryType: "AccountRoot",
				LedgerIndex:     strings.ToUpper(hex.EncodeToString(ownerKey.Key[:])),
			})
		}

		// Delete the offer
		if err := e.view.Erase(offerKey); err != nil {
			return TefBAD_LEDGER
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "NFTokenOffer",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(offerKey.Key[:])),
		})
	}

	return TesSUCCESS
}

// applyNFTokenAcceptOffer applies an NFTokenAcceptOffer transaction
// Reference: rippled NFTokenAcceptOffer.cpp doApply
func (e *Engine) applyNFTokenAcceptOffer(tx *NFTokenAcceptOffer, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Load offers
	var buyOffer, sellOffer *NFTokenOfferData
	var buyOfferKey, sellOfferKey keylet.Keylet

	if tx.NFTokenBuyOffer != "" {
		buyOfferIDBytes, err := hex.DecodeString(tx.NFTokenBuyOffer)
		if err != nil || len(buyOfferIDBytes) != 32 {
			return TemINVALID
		}
		var buyOfferKeyBytes [32]byte
		copy(buyOfferKeyBytes[:], buyOfferIDBytes)
		buyOfferKey = keylet.Keylet{Key: buyOfferKeyBytes}

		buyOfferData, err := e.view.Read(buyOfferKey)
		if err != nil {
			return TecOBJECT_NOT_FOUND
		}
		buyOffer, err = parseNFTokenOffer(buyOfferData)
		if err != nil {
			return TefINTERNAL
		}

		// Check expiration
		if buyOffer.Expiration != 0 && buyOffer.Expiration <= e.config.ParentCloseTime {
			return TecEXPIRED
		}

		// Verify it's a buy offer (flag not set)
		if buyOffer.Flags&lsfSellNFToken != 0 {
			return TecNFTOKEN_OFFER_TYPE_MISMATCH
		}

		// Cannot accept your own offer
		if buyOffer.Owner == accountID {
			return TecCANT_ACCEPT_OWN_NFTOKEN_OFFER
		}
	}

	if tx.NFTokenSellOffer != "" {
		sellOfferIDBytes, err := hex.DecodeString(tx.NFTokenSellOffer)
		if err != nil || len(sellOfferIDBytes) != 32 {
			return TemINVALID
		}
		var sellOfferKeyBytes [32]byte
		copy(sellOfferKeyBytes[:], sellOfferIDBytes)
		sellOfferKey = keylet.Keylet{Key: sellOfferKeyBytes}

		sellOfferData, err := e.view.Read(sellOfferKey)
		if err != nil {
			return TecOBJECT_NOT_FOUND
		}
		sellOffer, err = parseNFTokenOffer(sellOfferData)
		if err != nil {
			return TefINTERNAL
		}

		// Check expiration
		if sellOffer.Expiration != 0 && sellOffer.Expiration <= e.config.ParentCloseTime {
			return TecEXPIRED
		}

		// Verify it's a sell offer (flag set)
		if sellOffer.Flags&lsfSellNFToken == 0 {
			return TecNFTOKEN_OFFER_TYPE_MISMATCH
		}

		// Cannot accept your own offer
		if sellOffer.Owner == accountID {
			return TecCANT_ACCEPT_OWN_NFTOKEN_OFFER
		}
	}

	// Brokered mode (both offers)
	if buyOffer != nil && sellOffer != nil {
		return e.acceptNFTokenBrokeredMode(tx, account, accountID, buyOffer, sellOffer, buyOfferKey, sellOfferKey, metadata)
	}

	// Direct mode - sell offer only
	if sellOffer != nil {
		return e.acceptNFTokenSellOfferDirect(tx, account, accountID, sellOffer, sellOfferKey, metadata)
	}

	// Direct mode - buy offer only
	if buyOffer != nil {
		return e.acceptNFTokenBuyOfferDirect(tx, account, accountID, buyOffer, buyOfferKey, metadata)
	}

	return TemINVALID
}

// acceptNFTokenBrokeredMode handles brokered NFToken sales
// Reference: rippled NFTokenAcceptOffer.cpp doApply (brokered mode) and preclaim
func (e *Engine) acceptNFTokenBrokeredMode(tx *NFTokenAcceptOffer, account *AccountRoot, accountID [20]byte,
	buyOffer, sellOffer *NFTokenOfferData, buyOfferKey, sellOfferKey keylet.Keylet, metadata *Metadata) Result {

	// Verify both offers are for the same token
	// Reference: rippled NFTokenAcceptOffer.cpp:103
	if buyOffer.NFTokenID != sellOffer.NFTokenID {
		return TecNFTOKEN_BUY_SELL_MISMATCH
	}

	// Verify both offers are for the same asset (currency)
	// Reference: rippled NFTokenAcceptOffer.cpp:107
	// For XRP offers, both AmountIOU should be nil
	// For IOU offers, both should have same currency
	buyIsXRP := buyOffer.AmountIOU == nil
	sellIsXRP := sellOffer.AmountIOU == nil
	if buyIsXRP != sellIsXRP {
		return TecNFTOKEN_BUY_SELL_MISMATCH
	}
	if !buyIsXRP && !sellIsXRP {
		if buyOffer.AmountIOU.Currency != sellOffer.AmountIOU.Currency ||
			buyOffer.AmountIOU.Issuer != sellOffer.AmountIOU.Issuer {
			return TecNFTOKEN_BUY_SELL_MISMATCH
		}
	}

	// The two offers may not form a loop - buyer cannot be seller
	// Reference: rippled NFTokenAcceptOffer.cpp:112-114 (fixNonFungibleTokensV1_2)
	if buyOffer.Owner == sellOffer.Owner {
		return TecCANT_ACCEPT_OWN_NFTOKEN_OFFER
	}

	// Verify the seller owns the token
	sellerID := sellOffer.Owner
	pageKey := keylet.NFTokenPage(sellerID, sellOffer.NFTokenID)
	if _, err := e.view.Read(pageKey); err != nil {
		return TecNO_PERMISSION
	}

	// Verify buyer can pay at least what seller asks
	// Reference: rippled NFTokenAcceptOffer.cpp:118
	if buyOffer.Amount < sellOffer.Amount {
		return TecINSUFFICIENT_PAYMENT
	}

	// Check destination constraints
	// Reference: rippled NFTokenAcceptOffer.cpp:122-147
	// After fixNonFungibleTokensV1_2, destination must be the tx submitter
	if buyOffer.HasDestination && buyOffer.Destination != accountID {
		return TecNO_PERMISSION
	}
	if sellOffer.HasDestination && sellOffer.Destination != accountID {
		return TecNO_PERMISSION
	}

	buyerID := buyOffer.Owner

	// Calculate broker fee
	// Reference: rippled NFTokenAcceptOffer.cpp:153-163
	var brokerFee uint64
	if tx.NFTokenBrokerFee != nil {
		// Verify broker fee is in same currency as offers
		// Reference: rippled NFTokenAcceptOffer.cpp:155
		brokerFeeIsXRP := tx.NFTokenBrokerFee.Currency == ""
		if brokerFeeIsXRP != buyIsXRP {
			return TecNFTOKEN_BUY_SELL_MISMATCH
		}

		if brokerFeeIsXRP {
			var err error
			brokerFee, err = strconv.ParseUint(tx.NFTokenBrokerFee.Value, 10, 64)
			if err != nil {
				return TemMALFORMED
			}
		}

		// Broker fee cannot exceed or equal what buyer pays
		// Reference: rippled NFTokenAcceptOffer.cpp:158
		if brokerFee >= buyOffer.Amount {
			return TecINSUFFICIENT_PAYMENT
		}
		// Seller must still get at least what they asked for after broker fee
		// Reference: rippled NFTokenAcceptOffer.cpp:161
		if sellOffer.Amount > buyOffer.Amount-brokerFee {
			return TecINSUFFICIENT_PAYMENT
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
		account.Balance += brokerFee
		amount -= brokerFee
	}

	// Pay issuer cut
	if issuerCut > 0 {
		issuerKey := keylet.Account(issuerID)
		issuerData, err := e.view.Read(issuerKey)
		if err == nil {
			issuerAccount, err := parseAccountRoot(issuerData)
			if err == nil {
				issuerAccount.Balance += issuerCut
				issuerUpdatedData, _ := serializeAccountRoot(issuerAccount)
				e.view.Update(issuerKey, issuerUpdatedData)

				metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
					NodeType:        "ModifiedNode",
					LedgerEntryType: "AccountRoot",
					LedgerIndex:     strings.ToUpper(hex.EncodeToString(issuerKey.Key[:])),
				})
			}
		}
		amount -= issuerCut
	}

	// Pay seller
	sellerKey := keylet.Account(sellerID)
	sellerData, err := e.view.Read(sellerKey)
	if err != nil {
		return TefINTERNAL
	}
	sellerAccount, err := parseAccountRoot(sellerData)
	if err != nil {
		return TefINTERNAL
	}
	sellerAccount.Balance += amount
	if sellerAccount.OwnerCount > 0 {
		sellerAccount.OwnerCount-- // For sell offer being deleted
	}
	sellerUpdatedData, err := serializeAccountRoot(sellerAccount)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(sellerKey, sellerUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Update buyer's owner count
	buyerKey := keylet.Account(buyerID)
	buyerData, err := e.view.Read(buyerKey)
	if err == nil {
		buyerAccount, err := parseAccountRoot(buyerData)
		if err == nil && buyerAccount.OwnerCount > 0 {
			buyerAccount.OwnerCount-- // For buy offer being deleted
			buyerUpdatedData, _ := serializeAccountRoot(buyerAccount)
			e.view.Update(buyerKey, buyerUpdatedData)
		}
	}

	// Transfer the NFToken from seller to buyer
	if result := e.transferNFToken(sellerID, buyerID, sellOffer.NFTokenID, metadata); result != TesSUCCESS {
		return result
	}

	// Delete both offers
	if err := e.view.Erase(buyOfferKey); err != nil {
		return TefINTERNAL
	}
	if err := e.view.Erase(sellOfferKey); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "NFTokenOffer",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(buyOfferKey.Key[:])),
	})
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "NFTokenOffer",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(sellOfferKey.Key[:])),
	})

	return TesSUCCESS
}

// acceptNFTokenSellOfferDirect handles direct sell offer acceptance
func (e *Engine) acceptNFTokenSellOfferDirect(tx *NFTokenAcceptOffer, account *AccountRoot, accountID [20]byte,
	sellOffer *NFTokenOfferData, sellOfferKey keylet.Keylet, metadata *Metadata) Result {

	// Check destination constraint
	if sellOffer.HasDestination && sellOffer.Destination != accountID {
		return TecNO_PERMISSION
	}

	// Verify seller owns the token
	sellerID := sellOffer.Owner
	pageKey := keylet.NFTokenPage(sellerID, sellOffer.NFTokenID)
	if _, err := e.view.Read(pageKey); err != nil {
		return TecNO_PERMISSION
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
	if account.Balance < totalCost {
		return TecINSUFFICIENT_FUNDS
	}
	account.Balance -= totalCost

	// Pay issuer cut
	if issuerCut > 0 {
		issuerKey := keylet.Account(issuerID)
		issuerData, err := e.view.Read(issuerKey)
		if err == nil {
			issuerAccount, err := parseAccountRoot(issuerData)
			if err == nil {
				issuerAccount.Balance += issuerCut
				issuerUpdatedData, _ := serializeAccountRoot(issuerAccount)
				e.view.Update(issuerKey, issuerUpdatedData)
			}
		}
		amount -= issuerCut
	}

	// Pay seller
	sellerKey := keylet.Account(sellerID)
	sellerData, err := e.view.Read(sellerKey)
	if err != nil {
		return TefINTERNAL
	}
	sellerAccount, err := parseAccountRoot(sellerData)
	if err != nil {
		return TefINTERNAL
	}
	sellerAccount.Balance += amount
	if sellerAccount.OwnerCount > 0 {
		sellerAccount.OwnerCount--
	}
	sellerUpdatedData, err := serializeAccountRoot(sellerAccount)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(sellerKey, sellerUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Transfer the NFToken
	if result := e.transferNFToken(sellerID, accountID, sellOffer.NFTokenID, metadata); result != TesSUCCESS {
		return result
	}

	// Delete offer
	if err := e.view.Erase(sellOfferKey); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "NFTokenOffer",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(sellOfferKey.Key[:])),
	})

	return TesSUCCESS
}

// acceptNFTokenBuyOfferDirect handles direct buy offer acceptance
func (e *Engine) acceptNFTokenBuyOfferDirect(tx *NFTokenAcceptOffer, account *AccountRoot, accountID [20]byte,
	buyOffer *NFTokenOfferData, buyOfferKey keylet.Keylet, metadata *Metadata) Result {

	// Verify account owns the token
	pageKey := keylet.NFTokenPage(accountID, buyOffer.NFTokenID)
	if _, err := e.view.Read(pageKey); err != nil {
		return TecNO_PERMISSION
	}

	// Check destination constraint
	if buyOffer.HasDestination && buyOffer.Destination != accountID {
		return TecNO_PERMISSION
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
		issuerData, err := e.view.Read(issuerKey)
		if err == nil {
			issuerAccount, err := parseAccountRoot(issuerData)
			if err == nil {
				issuerAccount.Balance += issuerCut
				issuerUpdatedData, _ := serializeAccountRoot(issuerAccount)
				e.view.Update(issuerKey, issuerUpdatedData)
			}
		}
		amount -= issuerCut
	}

	// Pay seller (the account accepting the buy offer)
	account.Balance += amount

	// Update buyer's owner count
	buyerKey := keylet.Account(buyerID)
	buyerData, err := e.view.Read(buyerKey)
	if err == nil {
		buyerAccount, err := parseAccountRoot(buyerData)
		if err == nil && buyerAccount.OwnerCount > 0 {
			buyerAccount.OwnerCount--
			buyerUpdatedData, _ := serializeAccountRoot(buyerAccount)
			e.view.Update(buyerKey, buyerUpdatedData)
		}
	}

	// Transfer the NFToken
	if result := e.transferNFToken(accountID, buyerID, buyOffer.NFTokenID, metadata); result != TesSUCCESS {
		return result
	}

	// Delete offer
	if err := e.view.Erase(buyOfferKey); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "NFTokenOffer",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(buyOfferKey.Key[:])),
	})

	return TesSUCCESS
}

// transferNFToken transfers an NFToken from one account to another
func (e *Engine) transferNFToken(from, to [20]byte, tokenID [32]byte, metadata *Metadata) Result {
	// Remove from sender's page
	fromPageKey := keylet.NFTokenPage(from, tokenID)
	fromPageData, err := e.view.Read(fromPageKey)
	if err != nil {
		return TefINTERNAL
	}
	fromPage, err := parseNFTokenPage(fromPageData)
	if err != nil {
		return TefINTERNAL
	}

	var tokenData NFTokenData
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
		return TefINTERNAL
	}

	// Update or delete sender's page
	fromKey := keylet.Account(from)
	if len(fromPage.NFTokens) == 0 {
		if err := e.view.Erase(fromPageKey); err != nil {
			return TefINTERNAL
		}
		// Decrease sender's owner count
		fromData, err := e.view.Read(fromKey)
		if err == nil {
			fromAccount, err := parseAccountRoot(fromData)
			if err == nil && fromAccount.OwnerCount > 0 {
				fromAccount.OwnerCount--
				fromUpdatedData, _ := serializeAccountRoot(fromAccount)
				e.view.Update(fromKey, fromUpdatedData)
			}
		}
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(fromPageKey.Key[:])),
		})
	} else {
		fromPageUpdated, err := serializeNFTokenPage(fromPage)
		if err != nil {
			return TefINTERNAL
		}
		if err := e.view.Update(fromPageKey, fromPageUpdated); err != nil {
			return TefINTERNAL
		}
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(fromPageKey.Key[:])),
		})
	}

	// Add to recipient's page
	toPageKey := keylet.NFTokenPage(to, tokenID)
	exists, _ := e.view.Exists(toPageKey)
	if exists {
		toPageData, err := e.view.Read(toPageKey)
		if err != nil {
			return TefINTERNAL
		}
		toPage, err := parseNFTokenPage(toPageData)
		if err != nil {
			return TefINTERNAL
		}
		toPage.NFTokens = insertNFTokenSorted(toPage.NFTokens, tokenData)
		toPageUpdated, err := serializeNFTokenPage(toPage)
		if err != nil {
			return TefINTERNAL
		}
		if err := e.view.Update(toPageKey, toPageUpdated); err != nil {
			return TefINTERNAL
		}
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(toPageKey.Key[:])),
		})
	} else {
		newPage := &NFTokenPageData{
			NFTokens: []NFTokenData{tokenData},
		}
		newPageData, err := serializeNFTokenPage(newPage)
		if err != nil {
			return TefINTERNAL
		}
		if err := e.view.Insert(toPageKey, newPageData); err != nil {
			return TefINTERNAL
		}
		// Increase recipient's owner count
		toKey := keylet.Account(to)
		toData, err := e.view.Read(toKey)
		if err == nil {
			toAccount, err := parseAccountRoot(toData)
			if err == nil {
				toAccount.OwnerCount++
				toUpdatedData, _ := serializeAccountRoot(toAccount)
				e.view.Update(toKey, toUpdatedData)
			}
		}
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(toPageKey.Key[:])),
		})
	}

	return TesSUCCESS
}

// applyNFTokenModify applies an NFTokenModify transaction
// Reference: rippled NFTokenModify.cpp doApply
func (e *Engine) applyNFTokenModify(tx *NFTokenModify, account *AccountRoot, metadata *Metadata) Result {
	// Parse the token ID
	tokenIDBytes, err := hex.DecodeString(tx.NFTokenID)
	if err != nil || len(tokenIDBytes) != 32 {
		return TemINVALID
	}

	var tokenID [32]byte
	copy(tokenID[:], tokenIDBytes)

	accountID, _ := decodeAccountID(tx.Account)

	// Determine the owner
	var ownerID [20]byte
	if tx.Owner != "" {
		ownerID, err = decodeAccountID(tx.Owner)
		if err != nil {
			return TemINVALID
		}
	} else {
		ownerID = accountID
	}

	// Verify the token is mutable
	nftFlags := getNFTFlagsFromID(tokenID)
	if nftFlags&nftFlagMutable == 0 {
		return TecNO_PERMISSION
	}

	// Verify permissions - must be issuer or authorized minter
	issuerID := getNFTIssuer(tokenID)
	if issuerID != accountID {
		// Not the issuer, check if authorized minter
		issuerKey := keylet.Account(issuerID)
		issuerData, err := e.view.Read(issuerKey)
		if err != nil {
			return TefINTERNAL
		}
		issuerAccount, err := parseAccountRoot(issuerData)
		if err != nil {
			return TefINTERNAL
		}
		if issuerAccount.NFTokenMinter != tx.Account {
			return TecNO_PERMISSION
		}
	}

	// Find the NFToken page
	pageKey := keylet.NFTokenPage(ownerID, tokenID)

	pageData, err := e.view.Read(pageKey)
	if err != nil {
		return TecNO_ENTRY
	}

	// Parse the page
	page, err := parseNFTokenPage(pageData)
	if err != nil {
		return TefINTERNAL
	}

	// Find and update the token's URI
	found := false
	for i, token := range page.NFTokens {
		if token.NFTokenID == tokenID {
			found = true
			// Update the URI
			if tx.URI != "" {
				page.NFTokens[i].URI = tx.URI
			} else {
				// Clear the URI if empty string provided
				page.NFTokens[i].URI = ""
			}
			break
		}
	}

	if !found {
		return TecNO_ENTRY
	}

	// Serialize and update the page
	updatedPageData, err := serializeNFTokenPage(page)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(pageKey, updatedPageData); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "NFTokenPage",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(pageKey.Key[:])),
		FinalFields: map[string]any{
			"NFTokenID": strings.ToUpper(tx.NFTokenID),
			"URI":       tx.URI,
		},
	})

	return TesSUCCESS
}

// serializeNFTokenPage serializes an NFToken page ledger entry
func serializeNFTokenPage(page *NFTokenPageData) ([]byte, error) {
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

// parseNFTokenPage parses an NFToken page from binary data
func parseNFTokenPage(data []byte) (*NFTokenPageData, error) {
	page := &NFTokenPageData{
		NFTokens: make([]NFTokenData, 0),
	}
	offset := 0

	var currentToken NFTokenData
	hasToken := false

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
				return page, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return page, nil
			}
			offset += 4

		case fieldTypeHash256:
			if offset+32 > len(data) {
				return page, nil
			}
			switch fieldCode {
			case 25: // PreviousPageMin
				copy(page.PreviousPageMin[:], data[offset:offset+32])
			case 26: // NextPageMin
				copy(page.NextPageMin[:], data[offset:offset+32])
			case 10: // NFTokenID
				if hasToken {
					page.NFTokens = append(page.NFTokens, currentToken)
				}
				copy(currentToken.NFTokenID[:], data[offset:offset+32])
				currentToken.URI = ""
				hasToken = true
			}
			offset += 32

		case fieldTypeBlob:
			if offset >= len(data) {
				return page, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return page, nil
			}
			if fieldCode == 5 { // URI
				currentToken.URI = hex.EncodeToString(data[offset : offset+length])
			}
			offset += length

		default:
			if hasToken {
				page.NFTokens = append(page.NFTokens, currentToken)
				hasToken = false
			}
			return page, nil
		}
	}

	if hasToken {
		page.NFTokens = append(page.NFTokens, currentToken)
	}

	return page, nil
}

// ParseNFTokenPageFromBytes is an exported version of parseNFTokenPage for use by other packages
func ParseNFTokenPageFromBytes(data []byte) (*NFTokenPageData, error) {
	return parseNFTokenPage(data)
}

// serializeNFTokenOffer serializes an NFToken offer ledger entry
func serializeNFTokenOffer(tx *NFTokenCreateOffer, ownerID [20]byte, tokenID [32]byte, amount uint64, sequence uint32) ([]byte, error) {
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
		"Flags":           tx.GetFlags(),
	}

	if tx.Expiration != nil {
		jsonObj["Expiration"] = *tx.Expiration
	}

	if tx.Destination != "" {
		jsonObj["Destination"] = tx.Destination
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode NFTokenOffer: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// parseNFTokenOffer parses an NFToken offer from binary data
func parseNFTokenOffer(data []byte) (*NFTokenOfferData, error) {
	offer := &NFTokenOfferData{}
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
				return offer, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return offer, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case fieldCodeFlags:
				offer.Flags = value
			case 10: // Expiration
				offer.Expiration = value
			}

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return offer, nil
			}
			offset += 8

		case fieldTypeHash256:
			if offset+32 > len(data) {
				return offer, nil
			}
			if fieldCode == 10 { // NFTokenID
				copy(offer.NFTokenID[:], data[offset:offset+32])
			}
			offset += 32

		case fieldTypeAmount:
			if offset+8 > len(data) {
				return offer, nil
			}
			if data[offset]&0x80 == 0 {
				rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
				offer.Amount = rawAmount & 0x3FFFFFFFFFFFFFFF
				offset += 8
			} else {
				offset += 48
			}

		case fieldTypeAccountID:
			if offset+21 > len(data) {
				return offer, nil
			}
			length := data[offset]
			offset++
			if length == 20 {
				switch fieldCode {
				case 1: // Account/Owner
					copy(offer.Owner[:], data[offset:offset+20])
				case 3: // Destination
					copy(offer.Destination[:], data[offset:offset+20])
					offer.HasDestination = true
				}
				offset += 20
			}

		default:
			return offer, nil
		}
	}

	return offer, nil
}
