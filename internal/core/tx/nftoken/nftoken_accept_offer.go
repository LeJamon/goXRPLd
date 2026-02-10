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
	tx.Register(tx.TypeNFTokenAcceptOffer, func() tx.Transaction {
		return &NFTokenAcceptOffer{BaseTx: *tx.NewBaseTx(tx.TypeNFTokenAcceptOffer, "")}
	})
}

// NFTokenAcceptOffer accepts an NFToken offer.
type NFTokenAcceptOffer struct {
	tx.BaseTx

	// NFTokenSellOffer is the sell offer to accept (optional)
	NFTokenSellOffer string `json:"NFTokenSellOffer,omitempty" xrpl:"NFTokenSellOffer,omitempty"`

	// NFTokenBuyOffer is the buy offer to accept (optional)
	NFTokenBuyOffer string `json:"NFTokenBuyOffer,omitempty" xrpl:"NFTokenBuyOffer,omitempty"`

	// NFTokenBrokerFee is the broker fee for brokered sales (optional)
	NFTokenBrokerFee *tx.Amount `json:"NFTokenBrokerFee,omitempty" xrpl:"NFTokenBrokerFee,omitempty,amount"`
}

// NFTokenAcceptOffer has no transaction flags
const (
	// tfNFTokenAcceptOfferMask is the mask for invalid flags (all flags are invalid)
	tfNFTokenAcceptOfferMask uint32 = 0xFFFFFFFF
)

// NewNFTokenAcceptOffer creates a new NFTokenAcceptOffer transaction
func NewNFTokenAcceptOffer(account string) *NFTokenAcceptOffer {
	return &NFTokenAcceptOffer{
		BaseTx: *tx.NewBaseTx(tx.TypeNFTokenAcceptOffer, account),
	}
}

// TxType returns the transaction type
func (n *NFTokenAcceptOffer) TxType() tx.Type {
	return tx.TypeNFTokenAcceptOffer
}

// Validate validates the NFTokenAcceptOffer transaction
// Reference: rippled NFTokenAcceptOffer.cpp preflight
func (n *NFTokenAcceptOffer) Validate() error {
	if err := n.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags (no flags are valid for NFTokenAcceptOffer)
	if n.GetFlags() != 0 {
		return errors.New("temINVALID_FLAG: NFTokenAcceptOffer does not accept any flags")
	}

	// Must have at least one offer
	if n.NFTokenSellOffer == "" && n.NFTokenBuyOffer == "" {
		return errors.New("temMALFORMED: must specify NFTokenSellOffer or NFTokenBuyOffer")
	}

	// BrokerFee only valid for brokered mode (both offers)
	if n.NFTokenBrokerFee != nil {
		if n.NFTokenSellOffer == "" || n.NFTokenBuyOffer == "" {
			return errors.New("temMALFORMED: NFTokenBrokerFee requires both sell and buy offers")
		}
		// BrokerFee must be positive (greater than zero)
		// Reference: rippled NFTokenAcceptOffer.cpp:56 - if (*bf <= beast::zero)
		if n.NFTokenBrokerFee.IsZero() {
			return errors.New("temMALFORMED: NFTokenBrokerFee must be greater than zero")
		}
	}

	// Validate offer IDs are valid hex strings (64 characters = 32 bytes)
	if n.NFTokenSellOffer != "" && len(n.NFTokenSellOffer) != 64 {
		return errors.New("temMALFORMED: invalid NFTokenSellOffer format")
	}
	if n.NFTokenBuyOffer != "" && len(n.NFTokenBuyOffer) != 64 {
		return errors.New("temMALFORMED: invalid NFTokenBuyOffer format")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (n *NFTokenAcceptOffer) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(n)
}

// SetSellOffer sets the sell offer to accept
func (n *NFTokenAcceptOffer) SetSellOffer(offerID string) {
	n.NFTokenSellOffer = offerID
}

// SetBuyOffer sets the buy offer to accept
func (n *NFTokenAcceptOffer) SetBuyOffer(offerID string) {
	n.NFTokenBuyOffer = offerID
}

// RequiredAmendments returns the amendments required for this transaction type
func (n *NFTokenAcceptOffer) RequiredAmendments() [][32]byte {
	return [][32]byte{amendment.FeatureNonFungibleTokensV1}
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
