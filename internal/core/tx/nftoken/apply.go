package nftoken

import (
	"encoding/hex"
	"strconv"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

// Apply applies the NFTokenMint transaction to the ledger.
// Reference: rippled NFTokenMint.cpp doApply
func (m *NFTokenMint) Apply(ctx *tx.ApplyContext) tx.Result {
	accountID := ctx.AccountID

	// Determine the issuer
	var issuerID [20]byte
	var issuerAccount *sle.AccountRoot
	var issuerKey keylet.Keylet

	if m.Issuer != "" {
		var err error
		issuerID, err = sle.DecodeAccountID(m.Issuer)
		if err != nil {
			return tx.TemINVALID
		}

		// Read issuer account for MintedNFTokens tracking
		issuerKey = keylet.Account(issuerID)
		issuerData, err := ctx.View.Read(issuerKey)
		if err != nil {
			return tx.TecNO_ISSUER
		}
		issuerAccount, err = sle.ParseAccountRoot(issuerData)
		if err != nil {
			return tx.TefINTERNAL
		}

		// Verify that Account is authorized to mint for this issuer
		// The issuer must have set Account as their NFTokenMinter
		if issuerAccount.NFTokenMinter != m.Account {
			return tx.TecNO_PERMISSION
		}
	} else {
		issuerID = accountID
		issuerAccount = ctx.Account
	}

	// Get the token sequence from MintedNFTokens
	tokenSeq := issuerAccount.MintedNFTokens

	// Check for overflow
	if tokenSeq+1 < tokenSeq {
		return tx.TecMAX_SEQUENCE_REACHED
	}

	// Get flags for the token from transaction flags
	txFlags := m.GetFlags()
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
	if m.TransferFee != nil {
		transferFee = *m.TransferFee
	}

	// Generate the NFTokenID
	tokenID := generateNFTokenID(issuerID, m.NFTokenTaxon, tokenSeq, tokenFlags, transferFee)

	// Insert the NFToken into the owner's token directory
	// Reference: rippled NFTokenUtils.cpp insertToken
	newToken := sle.NFTokenData{
		NFTokenID: tokenID,
		URI:       m.URI,
	}

	insertResult := insertNFToken(accountID, newToken, ctx.View)
	if insertResult.Result != tx.TesSUCCESS {
		return insertResult.Result
	}

	// Update owner count based on pages created
	ctx.Account.OwnerCount += uint32(insertResult.PagesCreated)

	// Update MintedNFTokens on the issuer account
	issuerAccount.MintedNFTokens = tokenSeq + 1

	// If issuer is different from minter, update the issuer account - tracked automatically
	if m.Issuer != "" {
		issuerUpdatedData, err := sle.SerializeAccountRoot(issuerAccount)
		if err != nil {
			return tx.TefINTERNAL
		}
		if err := ctx.View.Update(issuerKey, issuerUpdatedData); err != nil {
			return tx.TefINTERNAL
		}
	}

	// Check reserve if pages were created (owner count increased)
	if insertResult.PagesCreated > 0 {
		reserve := ctx.AccountReserve(ctx.Account.OwnerCount)
		if ctx.Account.Balance < reserve {
			return tx.TecINSUFFICIENT_RESERVE
		}
	}

	return tx.TesSUCCESS
}

// Apply applies the NFTokenBurn transaction to the ledger.
// Reference: rippled NFTokenBurn.cpp doApply
func (b *NFTokenBurn) Apply(ctx *tx.ApplyContext) tx.Result {
	accountID := ctx.AccountID

	// Parse the token ID
	tokenIDBytes, err := hex.DecodeString(b.NFTokenID)
	if err != nil || len(tokenIDBytes) != 32 {
		return tx.TemINVALID
	}

	var tokenID [32]byte
	copy(tokenID[:], tokenIDBytes)

	// Determine the owner
	var ownerID [20]byte
	if b.Owner != "" {
		ownerID, err = sle.DecodeAccountID(b.Owner)
		if err != nil {
			return tx.TemINVALID
		}
	} else {
		ownerID = accountID
	}

	// Find the NFToken page
	pageKey := keylet.NFTokenPage(ownerID, tokenID)

	pageData, err := ctx.View.Read(pageKey)
	if err != nil {
		return tx.TecNO_ENTRY
	}

	// Parse the page
	page, err := sle.ParseNFTokenPage(pageData)
	if err != nil {
		return tx.TefINTERNAL
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
		return tx.TecNO_ENTRY
	}

	// Verify burn authorization
	// Owner can always burn, issuer can burn if flagBurnable is set
	if ownerID != accountID {
		nftFlags := getNFTFlagsFromID(tokenID)
		if nftFlags&nftFlagBurnable == 0 {
			return tx.TecNO_PERMISSION
		}

		// Check if the account is the issuer or an authorized minter
		issuerID := getNFTIssuer(tokenID)
		if issuerID != accountID {
			// Not the issuer, check if authorized minter
			issuerKey := keylet.Account(issuerID)
			issuerData, err := ctx.View.Read(issuerKey)
			if err != nil {
				return tx.TecNO_PERMISSION
			}
			issuerAccount, err := sle.ParseAccountRoot(issuerData)
			if err != nil {
				return tx.TefINTERNAL
			}
			if issuerAccount.NFTokenMinter != b.Account {
				return tx.TecNO_PERMISSION
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
	var ownerAccount *sle.AccountRoot
	var ownerKey keylet.Keylet
	if ownerID != accountID {
		ownerKey = keylet.Account(ownerID)
		ownerData, err := ctx.View.Read(ownerKey)
		if err != nil {
			return tx.TefINTERNAL
		}
		ownerAccount, err = sle.ParseAccountRoot(ownerData)
		if err != nil {
			return tx.TefINTERNAL
		}
	} else {
		ownerAccount = ctx.Account
	}

	// Update or delete the page - changes tracked automatically by ApplyStateTable
	if len(page.NFTokens) == 0 {
		// Delete empty page
		if err := ctx.View.Erase(pageKey); err != nil {
			return tx.TefINTERNAL
		}

		if ownerAccount.OwnerCount > 0 {
			ownerAccount.OwnerCount--
		}
	} else {
		// Update page
		updatedPageData, err := serializeNFTokenPage(page)
		if err != nil {
			return tx.TefINTERNAL
		}

		if err := ctx.View.Update(pageKey, updatedPageData); err != nil {
			return tx.TefINTERNAL
		}
	}

	// Update owner account if different from transaction sender
	if ownerID != accountID {
		ownerUpdatedData, err := sle.SerializeAccountRoot(ownerAccount)
		if err != nil {
			return tx.TefINTERNAL
		}
		if err := ctx.View.Update(ownerKey, ownerUpdatedData); err != nil {
			return tx.TefINTERNAL
		}
	}

	// Update BurnedNFTokens on the issuer - changes tracked automatically
	issuerID := getNFTIssuer(tokenID)
	issuerKey := keylet.Account(issuerID)
	issuerData, err := ctx.View.Read(issuerKey)
	if err == nil {
		issuerAccount, err := sle.ParseAccountRoot(issuerData)
		if err == nil {
			issuerAccount.BurnedNFTokens++
			issuerUpdatedData, err := sle.SerializeAccountRoot(issuerAccount)
			if err == nil {
				ctx.View.Update(issuerKey, issuerUpdatedData)
			}
		}
	}

	// Delete associated buy and sell offers (up to maxDeletableTokenOfferEntries)
	// Reference: rippled NFTokenBurn.cpp:108-139
	deletedCount := deleteNFTokenOffers(tokenID, true, maxDeletableTokenOfferEntries, ctx.View)
	if deletedCount < maxDeletableTokenOfferEntries {
		deleteNFTokenOffers(tokenID, false, maxDeletableTokenOfferEntries-deletedCount, ctx.View)
	}

	return tx.TesSUCCESS
}

// Apply applies the NFTokenCreateOffer transaction to the ledger.
// Reference: rippled NFTokenCreateOffer.cpp doApply
func (c *NFTokenCreateOffer) Apply(ctx *tx.ApplyContext) tx.Result {
	accountID := ctx.AccountID

	// Parse token ID
	tokenIDBytes, err := hex.DecodeString(c.NFTokenID)
	if err != nil || len(tokenIDBytes) != 32 {
		return tx.TemINVALID
	}

	var tokenID [32]byte
	copy(tokenID[:], tokenIDBytes)

	// Check expiration
	if c.Expiration != nil && *c.Expiration <= ctx.Config.ParentCloseTime {
		return tx.TecEXPIRED
	}

	// Check if this is a sell offer
	isSellOffer := c.GetFlags()&NFTokenCreateOfferFlagSellNFToken != 0

	// Verify token ownership
	if isSellOffer {
		// For sell offers, verify the sender owns the token
		pageKey := keylet.NFTokenPage(accountID, tokenID)
		pageData, err := ctx.View.Read(pageKey)
		if err != nil {
			return tx.TecNO_ENTRY
		}
		// Verify token is on the page
		page, err := sle.ParseNFTokenPage(pageData)
		if err != nil {
			return tx.TefINTERNAL
		}
		found := false
		for _, t := range page.NFTokens {
			if t.NFTokenID == tokenID {
				found = true
				break
			}
		}
		if !found {
			return tx.TecNO_ENTRY
		}
	} else {
		// For buy offers, verify the owner has the token
		var ownerID [20]byte
		ownerID, err = sle.DecodeAccountID(c.Owner)
		if err != nil {
			return tx.TemINVALID
		}
		pageKey := keylet.NFTokenPage(ownerID, tokenID)
		pageData, err := ctx.View.Read(pageKey)
		if err != nil {
			return tx.TecNO_ENTRY
		}
		// Verify token is on the page
		page, err := sle.ParseNFTokenPage(pageData)
		if err != nil {
			return tx.TefINTERNAL
		}
		found := false
		for _, t := range page.NFTokens {
			if t.NFTokenID == tokenID {
				found = true
				break
			}
		}
		if !found {
			return tx.TecNO_ENTRY
		}
	}

	// Parse amount
	var amountXRP uint64
	if c.Amount.Currency == "" {
		// XRP amount
		amountXRP, err = strconv.ParseUint(c.Amount.Value, 10, 64)
		if err != nil {
			return tx.TemMALFORMED
		}
	}

	// For buy offers, escrow the funds
	if !isSellOffer {
		if c.Amount.Currency == "" && amountXRP > 0 {
			// Check if account has enough balance (including reserve)
			reserve := ctx.AccountReserve(ctx.Account.OwnerCount + 1)
			if ctx.Account.Balance < amountXRP+reserve {
				return tx.TecINSUFFICIENT_FUNDS
			}
			// Escrow the funds (deduct from balance)
			ctx.Account.Balance -= amountXRP
		}
		// For IOU buy offers, don't escrow but verify funds exist
	}

	// Create the offer using keylet based on account + sequence
	sequence := *c.GetCommon().Sequence
	offerKey := keylet.NFTokenOffer(accountID, sequence)

	offerData, err := serializeNFTokenOffer(c, accountID, tokenID, amountXRP, sequence)
	if err != nil {
		return tx.TefINTERNAL
	}

	if err := ctx.View.Insert(offerKey, offerData); err != nil {
		return tx.TefINTERNAL
	}

	// Increase owner count
	ctx.Account.OwnerCount++

	// Check reserve
	reserve := ctx.AccountReserve(ctx.Account.OwnerCount)
	if ctx.Account.Balance < reserve {
		return tx.TecINSUFFICIENT_RESERVE
	}

	// Creation tracked automatically by ApplyStateTable

	return tx.TesSUCCESS
}

// Apply applies the NFTokenCancelOffer transaction to the ledger.
// Reference: rippled NFTokenCancelOffer.cpp doApply and preclaim
func (co *NFTokenCancelOffer) Apply(ctx *tx.ApplyContext) tx.Result {
	accountID := ctx.AccountID

	for _, offerIDHex := range co.NFTokenOffers {
		// Parse offer ID
		offerIDBytes, err := hex.DecodeString(offerIDHex)
		if err != nil || len(offerIDBytes) != 32 {
			continue
		}

		var offerKeyBytes [32]byte
		copy(offerKeyBytes[:], offerIDBytes)
		offerKey := keylet.Keylet{Key: offerKeyBytes}

		// Read the offer
		offerData, err := ctx.View.Read(offerKey)
		if err != nil {
			// Offer doesn't exist - already consumed, skip silently
			continue
		}

		// Parse the offer
		offer, err := sle.ParseNFTokenOffer(offerData)
		if err != nil {
			continue
		}

		// Check authorization to cancel
		// Reference: rippled NFTokenCancelOffer.cpp preclaim
		isExpired := offer.Expiration != 0 && offer.Expiration <= ctx.Config.ParentCloseTime
		isOwner := offer.Owner == accountID
		isDestination := offer.HasDestination && offer.Destination == accountID

		// Must be owner, destination, or expired
		if !isOwner && !isDestination && !isExpired {
			return tx.TecNO_PERMISSION
		}

		// Get the offer owner's account to update their owner count and potentially refund
		var ownerAccount *sle.AccountRoot
		var ownerKey keylet.Keylet

		if offer.Owner == accountID {
			ownerAccount = ctx.Account
		} else {
			ownerKey = keylet.Account(offer.Owner)
			ownerData, err := ctx.View.Read(ownerKey)
			if err != nil {
				return tx.TefINTERNAL
			}
			ownerAccount, err = sle.ParseAccountRoot(ownerData)
			if err != nil {
				return tx.TefINTERNAL
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

		// Update owner account if different from transaction sender - tracked automatically
		if offer.Owner != accountID {
			ownerUpdatedData, err := sle.SerializeAccountRoot(ownerAccount)
			if err != nil {
				return tx.TefINTERNAL
			}
			if err := ctx.View.Update(ownerKey, ownerUpdatedData); err != nil {
				return tx.TefINTERNAL
			}
		}

		// Delete the offer - tracked automatically by ApplyStateTable
		if err := ctx.View.Erase(offerKey); err != nil {
			return tx.TefBAD_LEDGER
		}
	}

	return tx.TesSUCCESS
}

// Apply applies the NFTokenAcceptOffer transaction to the ledger.
// Reference: rippled NFTokenAcceptOffer.cpp doApply
func (a *NFTokenAcceptOffer) Apply(ctx *tx.ApplyContext) tx.Result {
	accountID := ctx.AccountID

	// Load offers
	var buyOffer, sellOffer *sle.NFTokenOfferData
	var buyOfferKey, sellOfferKey keylet.Keylet

	if a.NFTokenBuyOffer != "" {
		buyOfferIDBytes, err := hex.DecodeString(a.NFTokenBuyOffer)
		if err != nil || len(buyOfferIDBytes) != 32 {
			return tx.TemINVALID
		}
		var buyOfferKeyBytes [32]byte
		copy(buyOfferKeyBytes[:], buyOfferIDBytes)
		buyOfferKey = keylet.Keylet{Key: buyOfferKeyBytes}

		buyOfferData, err := ctx.View.Read(buyOfferKey)
		if err != nil {
			return tx.TecOBJECT_NOT_FOUND
		}
		buyOffer, err = sle.ParseNFTokenOffer(buyOfferData)
		if err != nil {
			return tx.TefINTERNAL
		}

		// Check expiration
		if buyOffer.Expiration != 0 && buyOffer.Expiration <= ctx.Config.ParentCloseTime {
			return tx.TecEXPIRED
		}

		// Verify it's a buy offer (flag not set)
		if buyOffer.Flags&lsfSellNFToken != 0 {
			return tx.TecNFTOKEN_OFFER_TYPE_MISMATCH
		}

		// Cannot accept your own offer
		if buyOffer.Owner == accountID {
			return tx.TecCANT_ACCEPT_OWN_NFTOKEN_OFFER
		}
	}

	if a.NFTokenSellOffer != "" {
		sellOfferIDBytes, err := hex.DecodeString(a.NFTokenSellOffer)
		if err != nil || len(sellOfferIDBytes) != 32 {
			return tx.TemINVALID
		}
		var sellOfferKeyBytes [32]byte
		copy(sellOfferKeyBytes[:], sellOfferIDBytes)
		sellOfferKey = keylet.Keylet{Key: sellOfferKeyBytes}

		sellOfferData, err := ctx.View.Read(sellOfferKey)
		if err != nil {
			return tx.TecOBJECT_NOT_FOUND
		}
		sellOffer, err = sle.ParseNFTokenOffer(sellOfferData)
		if err != nil {
			return tx.TefINTERNAL
		}

		// Check expiration
		if sellOffer.Expiration != 0 && sellOffer.Expiration <= ctx.Config.ParentCloseTime {
			return tx.TecEXPIRED
		}

		// Verify it's a sell offer (flag set)
		if sellOffer.Flags&lsfSellNFToken == 0 {
			return tx.TecNFTOKEN_OFFER_TYPE_MISMATCH
		}

		// Cannot accept your own offer
		if sellOffer.Owner == accountID {
			return tx.TecCANT_ACCEPT_OWN_NFTOKEN_OFFER
		}
	}

	// Brokered mode (both offers)
	if buyOffer != nil && sellOffer != nil {
		return a.acceptNFTokenBrokeredMode(ctx, accountID, buyOffer, sellOffer, buyOfferKey, sellOfferKey)
	}

	// Direct mode - sell offer only
	if sellOffer != nil {
		return a.acceptNFTokenSellOfferDirect(ctx, accountID, sellOffer, sellOfferKey)
	}

	// Direct mode - buy offer only
	if buyOffer != nil {
		return a.acceptNFTokenBuyOfferDirect(ctx, accountID, buyOffer, buyOfferKey)
	}

	return tx.TemINVALID
}
