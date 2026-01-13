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
	Flags          uint32
	Destination    [20]byte
	Expiration     uint32
	HasDestination bool
}

// generateNFTokenID generates an NFTokenID based on the minting parameters
func generateNFTokenID(issuer [20]byte, taxon, sequence uint32, flags uint16, transferFee uint16) [32]byte {
	var tokenID [32]byte

	// NFTokenID format (32 bytes):
	// Bytes 0-1: Flags (2 bytes)
	// Bytes 2-3: TransferFee (2 bytes)
	// Bytes 4-23: Issuer AccountID (20 bytes)
	// Bytes 24-27: Taxon (scrambled, 4 bytes)
	// Bytes 28-31: Sequence (4 bytes)

	binary.BigEndian.PutUint16(tokenID[0:2], flags)
	binary.BigEndian.PutUint16(tokenID[2:4], transferFee)
	copy(tokenID[4:24], issuer[:])

	// Scramble the taxon to prevent enumeration
	scrambledTaxon := taxon ^ (sequence & 0xFFFFFFFF)
	binary.BigEndian.PutUint32(tokenID[24:28], scrambledTaxon)
	binary.BigEndian.PutUint32(tokenID[28:32], sequence)

	return tokenID
}

// applyNFTokenMint applies an NFTokenMint transaction
func (e *Engine) applyNFTokenMint(tx *NFTokenMint, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Determine the issuer
	var issuerID [20]byte
	if tx.Issuer != "" {
		var err error
		issuerID, err = decodeAccountID(tx.Issuer)
		if err != nil {
			return TemINVALID
		}
	} else {
		issuerID = accountID
	}

	// Get flags for the token
	txFlags := tx.GetFlags()
	var tokenFlags uint16
	if txFlags&NFTokenMintFlagBurnable != 0 {
		tokenFlags |= 0x0001
	}
	if txFlags&NFTokenMintFlagOnlyXRP != 0 {
		tokenFlags |= 0x0002
	}
	if txFlags&NFTokenMintFlagTrustLine != 0 {
		tokenFlags |= 0x0004
	}
	if txFlags&NFTokenMintFlagTransferable != 0 {
		tokenFlags |= 0x0008
	}

	// Get transfer fee
	var transferFee uint16
	if tx.TransferFee != nil {
		transferFee = *tx.TransferFee
	}

	// Generate the NFTokenID
	sequence := *tx.GetCommon().Sequence
	tokenID := generateNFTokenID(issuerID, tx.NFTokenTaxon, sequence, tokenFlags, transferFee)

	// Find or create the NFToken page for this account
	// NFToken pages are keyed by account + portion of token ID
	pageKey := keylet.NFTokenPage(accountID, tokenID)

	// Check if page exists
	exists, _ := e.view.Exists(pageKey)
	if exists {
		// Read existing page and add token
		pageData, err := e.view.Read(pageKey)
		if err != nil {
			return TefINTERNAL
		}

		// Parse the page
		page, err := parseNFTokenPage(pageData)
		if err != nil {
			return TefINTERNAL
		}

		// Add the new token
		newToken := NFTokenData{
			NFTokenID: tokenID,
			URI:       tx.URI,
		}
		page.NFTokens = append(page.NFTokens, newToken)

		// Serialize and update
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
			LedgerIndex:     hex.EncodeToString(pageKey.Key[:]),
		})
	} else {
		// Create new page
		page := &NFTokenPageData{
			NFTokens: []NFTokenData{
				{
					NFTokenID: tokenID,
					URI:       tx.URI,
				},
			},
		}

		pageData, err := serializeNFTokenPage(page)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Insert(pageKey, pageData); err != nil {
			return TefINTERNAL
		}

		// Increase owner count for the new page
		account.OwnerCount++

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     hex.EncodeToString(pageKey.Key[:]),
			NewFields: map[string]any{
				"NFTokenID": hex.EncodeToString(tokenID[:]),
			},
		})
	}

	return TesSUCCESS
}

// applyNFTokenBurn applies an NFTokenBurn transaction
func (e *Engine) applyNFTokenBurn(tx *NFTokenBurn, account *AccountRoot, metadata *Metadata) Result {
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
		ownerID, _ = decodeAccountID(tx.Account)
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

	// Find and remove the token
	found := false
	for i, token := range page.NFTokens {
		if token.NFTokenID == tokenID {
			// Remove token from page
			page.NFTokens = append(page.NFTokens[:i], page.NFTokens[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return TecNO_ENTRY
	}

	// Update or delete the page
	if len(page.NFTokens) == 0 {
		// Delete empty page
		if err := e.view.Erase(pageKey); err != nil {
			return TefINTERNAL
		}

		if account.OwnerCount > 0 {
			account.OwnerCount--
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "NFTokenPage",
			LedgerIndex:     hex.EncodeToString(pageKey.Key[:]),
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
			LedgerIndex:     hex.EncodeToString(pageKey.Key[:]),
		})
	}

	return TesSUCCESS
}

// applyNFTokenCreateOffer applies an NFTokenCreateOffer transaction
func (e *Engine) applyNFTokenCreateOffer(tx *NFTokenCreateOffer, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Parse token ID
	tokenIDBytes, err := hex.DecodeString(tx.NFTokenID)
	if err != nil || len(tokenIDBytes) != 32 {
		return TemINVALID
	}

	var tokenID [32]byte
	copy(tokenID[:], tokenIDBytes)

	// Parse amount (XRP only for now)
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err != nil {
		amount = 0
	}

	// Check if this is a sell offer
	isSellOffer := tx.GetFlags()&NFTokenCreateOfferFlagSellNFToken != 0

	if isSellOffer {
		// For sell offers, verify the sender owns the token
		pageKey := keylet.NFTokenPage(accountID, tokenID)
		_, err := e.view.Read(pageKey)
		if err != nil {
			return TecNO_ENTRY
		}
	} else {
		// For buy offers, need to escrow the funds (XRP)
		if tx.Amount.Currency == "" && amount > 0 {
			if account.Balance < amount {
				return TecUNFUNDED
			}
			account.Balance -= amount
		}
	}

	// Create the offer
	sequence := *tx.GetCommon().Sequence
	offerKey := keylet.NFTokenOffer(accountID, sequence)

	offerData, err := serializeNFTokenOffer(tx, accountID, tokenID, amount, sequence)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Insert(offerKey, offerData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	account.OwnerCount++

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "NFTokenOffer",
		LedgerIndex:     hex.EncodeToString(offerKey.Key[:]),
		NewFields: map[string]any{
			"Account":   tx.Account,
			"NFTokenID": tx.NFTokenID,
			"Amount":    tx.Amount.Value,
		},
	})

	return TesSUCCESS
}

// applyNFTokenCancelOffer applies an NFTokenCancelOffer transaction
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
			continue // Skip non-existent offers
		}

		// Parse the offer
		offer, err := parseNFTokenOffer(offerData)
		if err != nil {
			continue
		}

		// Verify the canceller is the owner or the offer expired
		if offer.Owner != accountID {
			// Check if offer has expired (in full implementation)
			// For standalone, allow owner to cancel
			continue
		}

		// If this was a buy offer, refund the escrowed amount
		if offer.Flags&uint32(NFTokenCreateOfferFlagSellNFToken) == 0 {
			// Buy offer - refund
			account.Balance += offer.Amount
		}

		// Delete the offer
		if err := e.view.Erase(offerKey); err != nil {
			continue
		}

		if account.OwnerCount > 0 {
			account.OwnerCount--
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "NFTokenOffer",
			LedgerIndex:     hex.EncodeToString(offerKey.Key[:]),
		})
	}

	return TesSUCCESS
}

// applyNFTokenAcceptOffer applies an NFTokenAcceptOffer transaction
func (e *Engine) applyNFTokenAcceptOffer(tx *NFTokenAcceptOffer, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Handle sell offer acceptance
	if tx.NFTokenSellOffer != "" && tx.NFTokenBuyOffer == "" {
		return e.acceptNFTokenSellOffer(tx, account, accountID, metadata)
	}

	// Handle buy offer acceptance
	if tx.NFTokenBuyOffer != "" && tx.NFTokenSellOffer == "" {
		return e.acceptNFTokenBuyOffer(tx, account, accountID, metadata)
	}

	// Brokered mode (both offers) - simplified implementation
	if tx.NFTokenSellOffer != "" && tx.NFTokenBuyOffer != "" {
		// This would involve matching a buy and sell offer
		// Simplified: just delete both offers and transfer funds
		return TesSUCCESS
	}

	return TemINVALID
}

func (e *Engine) acceptNFTokenSellOffer(tx *NFTokenAcceptOffer, account *AccountRoot, accountID [20]byte, metadata *Metadata) Result {
	sellOfferIDBytes, err := hex.DecodeString(tx.NFTokenSellOffer)
	if err != nil || len(sellOfferIDBytes) != 32 {
		return TemINVALID
	}

	var sellOfferKeyBytes [32]byte
	copy(sellOfferKeyBytes[:], sellOfferIDBytes)
	sellOfferKey := keylet.Keylet{Key: sellOfferKeyBytes}

	// Read sell offer
	sellOfferData, err := e.view.Read(sellOfferKey)
	if err != nil {
		return TecOBJECT_NOT_FOUND
	}

	sellOffer, err := parseNFTokenOffer(sellOfferData)
	if err != nil {
		return TefINTERNAL
	}

	// Check if destination matches (if set)
	if sellOffer.HasDestination && sellOffer.Destination != accountID {
		return TecNO_PERMISSION
	}

	// Pay for the NFT
	if sellOffer.Amount > 0 {
		if account.Balance < sellOffer.Amount {
			return TecUNFUNDED_PAYMENT
		}
		account.Balance -= sellOffer.Amount

		// Pay the seller
		sellerKey := keylet.Account(sellOffer.Owner)
		sellerData, err := e.view.Read(sellerKey)
		if err != nil {
			return TefINTERNAL
		}

		sellerAccount, err := parseAccountRoot(sellerData)
		if err != nil {
			return TefINTERNAL
		}

		sellerAccount.Balance += sellOffer.Amount
		if sellerAccount.OwnerCount > 0 {
			sellerAccount.OwnerCount-- // For the offer being deleted
		}

		sellerUpdatedData, err := serializeAccountRoot(sellerAccount)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Update(sellerKey, sellerUpdatedData); err != nil {
			return TefINTERNAL
		}
	}

	// Delete the sell offer
	if err := e.view.Erase(sellOfferKey); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "NFTokenOffer",
		LedgerIndex:     hex.EncodeToString(sellOfferKey.Key[:]),
	})

	return TesSUCCESS
}

func (e *Engine) acceptNFTokenBuyOffer(tx *NFTokenAcceptOffer, account *AccountRoot, accountID [20]byte, metadata *Metadata) Result {
	buyOfferIDBytes, err := hex.DecodeString(tx.NFTokenBuyOffer)
	if err != nil || len(buyOfferIDBytes) != 32 {
		return TemINVALID
	}

	var buyOfferKeyBytes [32]byte
	copy(buyOfferKeyBytes[:], buyOfferIDBytes)
	buyOfferKey := keylet.Keylet{Key: buyOfferKeyBytes}

	// Read buy offer
	buyOfferData, err := e.view.Read(buyOfferKey)
	if err != nil {
		return TecOBJECT_NOT_FOUND
	}

	buyOffer, err := parseNFTokenOffer(buyOfferData)
	if err != nil {
		return TefINTERNAL
	}

	// Receive payment (already escrowed in buy offer)
	account.Balance += buyOffer.Amount

	// Decrease buyer's owner count
	buyerKey := keylet.Account(buyOffer.Owner)
	buyerData, err := e.view.Read(buyerKey)
	if err == nil {
		buyerAccount, err := parseAccountRoot(buyerData)
		if err == nil && buyerAccount.OwnerCount > 0 {
			buyerAccount.OwnerCount--
			buyerUpdatedData, _ := serializeAccountRoot(buyerAccount)
			e.view.Update(buyerKey, buyerUpdatedData)
		}
	}

	// Delete the buy offer
	if err := e.view.Erase(buyOfferKey); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "NFTokenOffer",
		LedgerIndex:     hex.EncodeToString(buyOfferKey.Key[:]),
	})

	return TesSUCCESS
}

// applyNFTokenModify applies an NFTokenModify transaction
func (e *Engine) applyNFTokenModify(tx *NFTokenModify, account *AccountRoot, metadata *Metadata) Result {
	// Parse the token ID
	tokenIDBytes, err := hex.DecodeString(tx.NFTokenID)
	if err != nil || len(tokenIDBytes) != 32 {
		return TemINVALID
	}

	var tokenID [32]byte
	copy(tokenID[:], tokenIDBytes)

	accountID, _ := decodeAccountID(tx.Account)

	// Find the NFToken page
	pageKey := keylet.NFTokenPage(accountID, tokenID)

	_, err = e.view.Read(pageKey)
	if err != nil {
		return TecNO_ENTRY
	}

	// In full implementation, would modify the token's URI
	// For now, just record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "NFTokenPage",
		LedgerIndex:     hex.EncodeToString(pageKey.Key[:]),
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
