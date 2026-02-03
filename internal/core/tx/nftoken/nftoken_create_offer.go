package nftoken

import (
	"encoding/hex"
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeNFTokenCreateOffer, func() tx.Transaction {
		return &NFTokenCreateOffer{BaseTx: *tx.NewBaseTx(tx.TypeNFTokenCreateOffer, "")}
	})
}

// NFTokenCreateOffer creates an offer to buy or sell an NFToken.
type NFTokenCreateOffer struct {
	tx.BaseTx

	// NFTokenID is the ID of the token (required)
	NFTokenID string `json:"NFTokenID" xrpl:"NFTokenID"`

	// Amount is the price for the offer (required)
	Amount tx.Amount `json:"Amount" xrpl:"Amount,amount"`

	// Owner is the owner of the token (required for buy offers)
	Owner string `json:"Owner,omitempty" xrpl:"Owner,omitempty"`

	// Destination is who can accept this offer (optional)
	Destination string `json:"Destination,omitempty" xrpl:"Destination,omitempty"`

	// Expiration is when the offer expires (optional)
	Expiration *uint32 `json:"Expiration,omitempty" xrpl:"Expiration,omitempty"`
}

// NFTokenCreateOffer flags
const (
	// tfSellNFToken indicates this is a sell offer
	NFTokenCreateOfferFlagSellNFToken uint32 = 0x00000001

	// tfNFTokenCreateOfferMask is the mask for invalid flags
	tfNFTokenCreateOfferMask uint32 = ^NFTokenCreateOfferFlagSellNFToken
)

// NewNFTokenCreateOffer creates a new NFTokenCreateOffer transaction
func NewNFTokenCreateOffer(account, nftokenID string, amount tx.Amount) *NFTokenCreateOffer {
	return &NFTokenCreateOffer{
		BaseTx:    *tx.NewBaseTx(tx.TypeNFTokenCreateOffer, account),
		NFTokenID: nftokenID,
		Amount:    amount,
	}
}

// TxType returns the transaction type
func (n *NFTokenCreateOffer) TxType() tx.Type {
	return tx.TypeNFTokenCreateOffer
}

// Validate validates the NFTokenCreateOffer transaction
// Reference: rippled NFTokenCreateOffer.cpp preflight and tokenOfferCreatePreflight
func (n *NFTokenCreateOffer) Validate() error {
	if err := n.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags
	if n.GetFlags()&tfNFTokenCreateOfferMask != 0 {
		return errors.New("temINVALID_FLAG: invalid NFTokenCreateOffer flags")
	}

	if n.NFTokenID == "" {
		return errors.New("temMALFORMED: NFTokenID is required")
	}

	// Parse NFToken flags from token ID to validate
	nftFlags := getNFTokenFlags(n.NFTokenID)

	isSellOffer := n.GetFlags()&NFTokenCreateOfferFlagSellNFToken != 0

	// Buy offers must have Owner
	if !isSellOffer && n.Owner == "" {
		return errors.New("temMALFORMED: Owner is required for buy offers")
	}

	// Sell offers cannot specify Owner
	if isSellOffer && n.Owner != "" {
		return errors.New("temMALFORMED: Owner not allowed for sell offers")
	}

	// For buy offers, owner cannot be the account placing the offer
	if !isSellOffer && n.Owner == n.Account {
		return errors.New("temMALFORMED: cannot create buy offer for your own token")
	}

	// Destination cannot be the same as the account creating the offer
	if n.Destination != "" && n.Destination == n.Account {
		return errors.New("temMALFORMED: Destination cannot be the same as Account")
	}

	// Expiration validation - expiration of 0 is invalid
	if n.Expiration != nil && *n.Expiration == 0 {
		return errors.New("temBAD_EXPIRATION: Expiration cannot be 0")
	}

	// Amount validation
	if n.Amount.Currency == "" {
		// XRP amount
		// For buy offers, zero amount is not allowed
		if !isSellOffer && n.Amount.IsZero() {
			return errors.New("temBAD_AMOUNT: buy offer amount cannot be zero")
		}
	} else {
		// IOU amount - check if OnlyXRP flag is set on the token
		if nftFlags&nftFlagOnlyXRP != 0 {
			return errors.New("temBAD_AMOUNT: NFToken requires XRP only")
		}
		// IOU amount of 0 is not allowed
		if n.Amount.IsZero() {
			return errors.New("temBAD_AMOUNT: IOU amount cannot be zero")
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (n *NFTokenCreateOffer) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(n)
}

// SetSellOffer marks this as a sell offer
func (n *NFTokenCreateOffer) SetSellOffer() {
	flags := n.GetFlags() | NFTokenCreateOfferFlagSellNFToken
	n.SetFlags(flags)
}

// RequiredAmendments returns the amendments required for this transaction type
func (n *NFTokenCreateOffer) RequiredAmendments() []string {
	return []string{amendment.AmendmentNonFungibleTokensV1}
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
		amountXRP = uint64(c.Amount.Drops())
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
