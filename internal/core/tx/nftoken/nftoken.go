package nftoken

import (
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/tx/amendment"

	"github.com/LeJamon/goXRPLd/internal/core/tx"
)

func init() {
	tx.Register(tx.TypeNFTokenMint, func() tx.Transaction {
		return &NFTokenMint{BaseTx: *tx.NewBaseTx(tx.TypeNFTokenMint, "")}
	})
	tx.Register(tx.TypeNFTokenBurn, func() tx.Transaction {
		return &NFTokenBurn{BaseTx: *tx.NewBaseTx(tx.TypeNFTokenBurn, "")}
	})
	tx.Register(tx.TypeNFTokenCreateOffer, func() tx.Transaction {
		return &NFTokenCreateOffer{BaseTx: *tx.NewBaseTx(tx.TypeNFTokenCreateOffer, "")}
	})
	tx.Register(tx.TypeNFTokenCancelOffer, func() tx.Transaction {
		return &NFTokenCancelOffer{BaseTx: *tx.NewBaseTx(tx.TypeNFTokenCancelOffer, "")}
	})
	tx.Register(tx.TypeNFTokenAcceptOffer, func() tx.Transaction {
		return &NFTokenAcceptOffer{BaseTx: *tx.NewBaseTx(tx.TypeNFTokenAcceptOffer, "")}
	})
}

// NFToken constants matching rippled
const (
	// maxTransferFee is the maximum transfer fee (50000 = 50%)
	maxTransferFee = 50000

	// maxTokenURILength is the maximum length of a token URI (256 bytes when encoded)
	// Note: When provided in transactions, URI is hex-encoded, so the actual
	// byte length is len(hexString)/2
	maxTokenURILength = 256

	// transferFeeDivisor is the divisor used for transfer fee calculation
	// Transfer fee is in basis points where 50000 = 50%
	// Calculation: amount * transferFee / transferFeeDivisor
	transferFeeDivisor = 100000

	// dirMaxTokensPerPage is the maximum number of NFTs per page
	// Reference: rippled Protocol.h - dirMaxTokensPerPage = 32
	dirMaxTokensPerPage = 32

	// NFToken flags stored in NFTokenID
	nftFlagBurnable     uint16 = 0x0001
	nftFlagOnlyXRP      uint16 = 0x0002
	nftFlagTrustLine    uint16 = 0x0004
	nftFlagTransferable uint16 = 0x0008
	nftFlagMutable      uint16 = 0x0010

	// lsfSellNFToken is the flag for sell offers in ledger entries
	lsfSellNFToken uint32 = 0x00000001

	// maxDeletableTokenOfferEntries is the max offers to delete on burn
	maxDeletableTokenOfferEntries = 500

	// maxTokenOfferCancelCount is the max offers that can be cancelled in one tx
	maxTokenOfferCancelCount = 500

	// tfNFTokenCancelOfferMask is the mask for invalid flags (all flags are invalid)
	tfNFTokenCancelOfferMask uint32 = 0xFFFFFFFF
)

// NFTokenMint mints a new NFToken.
type NFTokenMint struct {
	tx.BaseTx

	// NFTokenTaxon is the taxon for this token (required)
	NFTokenTaxon uint32 `json:"NFTokenTaxon" xrpl:"NFTokenTaxon"`

	// Issuer is the issuer of the token (optional, defaults to Account)
	Issuer string `json:"Issuer,omitempty" xrpl:"Issuer,omitempty"`

	// TransferFee is the fee for secondary sales (0-50000, where 50000 = 50%)
	TransferFee *uint16 `json:"TransferFee,omitempty" xrpl:"TransferFee,omitempty"`

	// URI is the URI for the token metadata (optional)
	URI string `json:"URI,omitempty" xrpl:"URI,omitempty"`

	// Amount is the minting price (optional)
	Amount *tx.Amount `json:"Amount,omitempty" xrpl:"Amount,omitempty,amount"`

	// Destination is the account to receive the minted token (optional)
	Destination string `json:"Destination,omitempty" xrpl:"Destination,omitempty"`

	// Expiration is when the mint offer expires (optional)
	Expiration *uint32 `json:"Expiration,omitempty" xrpl:"Expiration,omitempty"`
}

// NFTokenMint flags
const (
	// tfBurnable allows the issuer to burn the token
	NFTokenMintFlagBurnable uint32 = 0x00000001
	// tfOnlyXRP allows only XRP for sale
	NFTokenMintFlagOnlyXRP uint32 = 0x00000002
	// tfTrustLine creates trust lines for transfer (deprecated by fixRemoveNFTokenAutoTrustLine)
	NFTokenMintFlagTrustLine uint32 = 0x00000004
	// tfTransferable allows the token to be transferred
	NFTokenMintFlagTransferable uint32 = 0x00000008
	// tfMutable allows the URI to be modified (requires DynamicNFT amendment)
	NFTokenMintFlagMutable uint32 = 0x00000010

	// tfNFTokenMintMask is the mask for valid flags (with fixRemoveNFTokenAutoTrustLine)
	tfNFTokenMintMask uint32 = ^(NFTokenMintFlagBurnable | NFTokenMintFlagOnlyXRP | NFTokenMintFlagTransferable)
	// tfNFTokenMintMaskWithMutable includes mutable flag
	tfNFTokenMintMaskWithMutable uint32 = ^(NFTokenMintFlagBurnable | NFTokenMintFlagOnlyXRP | NFTokenMintFlagTransferable | NFTokenMintFlagMutable)
	// tfNFTokenMintOldMask is the mask for valid flags (before fixRemoveNFTokenAutoTrustLine)
	tfNFTokenMintOldMask uint32 = ^(NFTokenMintFlagBurnable | NFTokenMintFlagOnlyXRP | NFTokenMintFlagTrustLine | NFTokenMintFlagTransferable)
	// tfNFTokenMintOldMaskWithMutable includes mutable flag
	tfNFTokenMintOldMaskWithMutable uint32 = ^(NFTokenMintFlagBurnable | NFTokenMintFlagOnlyXRP | NFTokenMintFlagTrustLine | NFTokenMintFlagTransferable | NFTokenMintFlagMutable)
)

// NewNFTokenMint creates a new NFTokenMint transaction
func NewNFTokenMint(account string, taxon uint32) *NFTokenMint {
	return &NFTokenMint{
		BaseTx:       *tx.NewBaseTx(tx.TypeNFTokenMint, account),
		NFTokenTaxon: taxon,
	}
}

// TxType returns the transaction type
func (n *NFTokenMint) TxType() tx.Type {
	return tx.TypeNFTokenMint
}

// Validate validates the NFTokenMint transaction
// Reference: rippled NFTokenMint.cpp preflight
func (n *NFTokenMint) Validate() error {
	if err := n.BaseTx.Validate(); err != nil {
		return err
	}

	// Check for invalid flags
	// Note: In production, this should check based on enabled amendments
	// For now, use the most restrictive mask (with fixRemoveNFTokenAutoTrustLine)
	if n.GetFlags()&tfNFTokenMintMask != 0 {
		return errors.New("temINVALID_FLAG: invalid NFTokenMint flags")
	}

	// TransferFee must be <= maxTransferFee (50000 = 50%)
	if n.TransferFee != nil {
		if *n.TransferFee > maxTransferFee {
			return errors.New("temBAD_NFTOKEN_TRANSFER_FEE: TransferFee cannot exceed 50000")
		}
		// If a non-zero TransferFee is set, tfTransferable must also be set
		if *n.TransferFee > 0 && n.GetFlags()&NFTokenMintFlagTransferable == 0 {
			return errors.New("temMALFORMED: non-zero TransferFee requires tfTransferable flag")
		}
	}

	// Issuer must not be the same as Account (if specified)
	if n.Issuer != "" && n.Issuer == n.Account {
		return errors.New("temMALFORMED: Issuer cannot be the same as Account")
	}

	// URI validation: must be hex-encoded, not empty (if present), and <= maxTokenURILength bytes
	if n.URI != "" {
		// URI is hex-encoded, so length in bytes is len/2
		uriBytes := len(n.URI) / 2
		if uriBytes == 0 {
			return errors.New("temMALFORMED: URI cannot be empty")
		}
		if uriBytes > maxTokenURILength {
			return errors.New("temMALFORMED: URI too long")
		}
	}

	// If Amount, Destination, or Expiration are present, Amount is required
	// (This is NFTokenMintOffer support)
	hasOfferFields := n.Amount != nil || n.Destination != "" || n.Expiration != nil
	if hasOfferFields && n.Amount == nil {
		return errors.New("temMALFORMED: Amount required when Destination or Expiration present")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (n *NFTokenMint) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(n)
}

// SetBurnable makes the token burnable by the issuer
func (n *NFTokenMint) SetBurnable() {
	flags := n.GetFlags() | NFTokenMintFlagBurnable
	n.SetFlags(flags)
}

// SetTransferable makes the token transferable
func (n *NFTokenMint) SetTransferable() {
	flags := n.GetFlags() | NFTokenMintFlagTransferable
	n.SetFlags(flags)
}

// RequiredAmendments returns the amendments required for this transaction type
func (n *NFTokenMint) RequiredAmendments() []string {
	return []string{amendment.AmendmentNonFungibleTokensV1}
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
		if !isSellOffer && n.Amount.Value == "0" {
			return errors.New("temBAD_AMOUNT: buy offer amount cannot be zero")
		}
	} else {
		// IOU amount - check if OnlyXRP flag is set on the token
		if nftFlags&nftFlagOnlyXRP != 0 {
			return errors.New("temBAD_AMOUNT: NFToken requires XRP only")
		}
		// IOU amount of 0 is not allowed
		if n.Amount.Value == "0" {
			return errors.New("temBAD_AMOUNT: IOU amount cannot be zero")
		}
	}

	return nil
}

// getNFTokenFlags extracts the flags from an NFTokenID (first 2 bytes)
func getNFTokenFlags(nftokenID string) uint16 {
	if len(nftokenID) < 4 {
		return 0
	}
	// First 4 hex chars = 2 bytes = flags
	var flags uint16
	for i := 0; i < 4 && i < len(nftokenID); i++ {
		flags <<= 4
		c := nftokenID[i]
		switch {
		case c >= '0' && c <= '9':
			flags |= uint16(c - '0')
		case c >= 'a' && c <= 'f':
			flags |= uint16(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			flags |= uint16(c - 'A' + 10)
		}
	}
	return flags
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
func (n *NFTokenCancelOffer) RequiredAmendments() []string {
	return []string{amendment.AmendmentNonFungibleTokensV1}
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
		if n.NFTokenBrokerFee.Value == "0" || n.NFTokenBrokerFee.Value == "" {
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
func (n *NFTokenAcceptOffer) RequiredAmendments() []string {
	return []string{amendment.AmendmentNonFungibleTokensV1}
}
