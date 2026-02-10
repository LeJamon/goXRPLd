package nftoken

import (
	"encoding/hex"
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/amendment"
)

func init() {
	tx.Register(tx.TypeNFTokenCancelOffer, func() tx.Transaction {
		return &NFTokenCancelOffer{BaseTx: *tx.NewBaseTx(tx.TypeNFTokenCancelOffer, "")}
	})
}

// NFTokenCancelOffer cancels NFToken offers.
type NFTokenCancelOffer struct {
	tx.BaseTx

	// NFTokenOffers is the list of offer IDs to cancel (required)
	NFTokenOffers []string `json:"NFTokenOffers" xrpl:"NFTokenOffers"`
}

// NewNFTokenCancelOffer creates a new NFTokenCancelOffer transaction
func NewNFTokenCancelOffer(account string, offerIDs []string) *NFTokenCancelOffer {
	return &NFTokenCancelOffer{
		BaseTx:        *tx.NewBaseTx(tx.TypeNFTokenCancelOffer, account),
		NFTokenOffers: offerIDs,
	}
}

// TxType returns the transaction type
func (n *NFTokenCancelOffer) TxType() tx.Type {
	return tx.TypeNFTokenCancelOffer
}

// Validate validates the NFTokenCancelOffer transaction
// Reference: rippled NFTokenCancelOffer.cpp preflight
func (n *NFTokenCancelOffer) Validate() error {
	if err := n.BaseTx.Validate(); err != nil {
		return err
	}

	// Check flags - no flags are valid for NFTokenCancelOffer
	if n.GetFlags()&tfNFTokenCancelOfferMask != 0 {
		return errors.New("temINVALID_FLAG: invalid flags for NFTokenCancelOffer")
	}

	// Must have at least one offer ID
	if len(n.NFTokenOffers) == 0 {
		return errors.New("temMALFORMED: NFTokenOffers is required")
	}

	// Cannot exceed maximum offer count
	if len(n.NFTokenOffers) > maxTokenOfferCancelCount {
		return errors.New("temMALFORMED: NFTokenOffers exceeds maximum count")
	}

	// Check for duplicates
	seen := make(map[string]bool)
	for _, offerID := range n.NFTokenOffers {
		if seen[offerID] {
			return errors.New("temMALFORMED: duplicate offer ID in NFTokenOffers")
		}
		seen[offerID] = true
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (n *NFTokenCancelOffer) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(n)
}

// RequiredAmendments returns the amendments required for this transaction type
func (n *NFTokenCancelOffer) RequiredAmendments() [][32]byte {
	return [][32]byte{amendment.FeatureNonFungibleTokensV1}
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
