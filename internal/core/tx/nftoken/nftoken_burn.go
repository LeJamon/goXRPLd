package nftoken

import (
	"encoding/hex"
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
)

func init() {
	tx.Register(tx.TypeNFTokenBurn, func() tx.Transaction {
		return &NFTokenBurn{BaseTx: *tx.NewBaseTx(tx.TypeNFTokenBurn, "")}
	})
}

// NFTokenBurn burns an NFToken.
type NFTokenBurn struct {
	tx.BaseTx

	// NFTokenID is the ID of the token to burn (required)
	NFTokenID string `json:"NFTokenID" xrpl:"NFTokenID"`

	// Owner is the owner of the token (optional, for authorized burns)
	Owner string `json:"Owner,omitempty" xrpl:"Owner,omitempty"`
}

// NewNFTokenBurn creates a new NFTokenBurn transaction
func NewNFTokenBurn(account, nftokenID string) *NFTokenBurn {
	return &NFTokenBurn{
		BaseTx:    *tx.NewBaseTx(tx.TypeNFTokenBurn, account),
		NFTokenID: nftokenID,
	}
}

// TxType returns the transaction type
func (n *NFTokenBurn) TxType() tx.Type {
	return tx.TypeNFTokenBurn
}

// Validate validates the NFTokenBurn transaction
func (n *NFTokenBurn) Validate() error {
	if err := n.BaseTx.Validate(); err != nil {
		return err
	}

	if n.NFTokenID == "" {
		return errors.New("NFTokenID is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (n *NFTokenBurn) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(n)
}

// RequiredAmendments returns the amendments required for this transaction type
func (n *NFTokenBurn) RequiredAmendments() []string {
	return []string{amendment.AmendmentNonFungibleTokensV1}
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
