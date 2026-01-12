package tx

import "errors"

// NFTokenMint mints a new NFToken.
type NFTokenMint struct {
	BaseTx

	// NFTokenTaxon is the taxon for this token (required)
	NFTokenTaxon uint32 `json:"NFTokenTaxon"`

	// Issuer is the issuer of the token (optional, defaults to Account)
	Issuer string `json:"Issuer,omitempty"`

	// TransferFee is the fee for secondary sales (0-50000, where 50000 = 50%)
	TransferFee *uint16 `json:"TransferFee,omitempty"`

	// URI is the URI for the token metadata (optional)
	URI string `json:"URI,omitempty"`

	// Amount is the minting price (optional)
	Amount *Amount `json:"Amount,omitempty"`

	// Destination is the account to receive the minted token (optional)
	Destination string `json:"Destination,omitempty"`

	// Expiration is when the mint offer expires (optional)
	Expiration *uint32 `json:"Expiration,omitempty"`
}

// NFTokenMint flags
const (
	// tfBurnable allows the issuer to burn the token
	NFTokenMintFlagBurnable uint32 = 0x00000001
	// tfOnlyXRP allows only XRP for sale
	NFTokenMintFlagOnlyXRP uint32 = 0x00000002
	// tfTrustLine creates trust lines for transfer
	NFTokenMintFlagTrustLine uint32 = 0x00000004
	// tfTransferable allows the token to be transferred
	NFTokenMintFlagTransferable uint32 = 0x00000008
)

// NewNFTokenMint creates a new NFTokenMint transaction
func NewNFTokenMint(account string, taxon uint32) *NFTokenMint {
	return &NFTokenMint{
		BaseTx:       *NewBaseTx(TypeNFTokenMint, account),
		NFTokenTaxon: taxon,
	}
}

// TxType returns the transaction type
func (n *NFTokenMint) TxType() Type {
	return TypeNFTokenMint
}

// Validate validates the NFTokenMint transaction
func (n *NFTokenMint) Validate() error {
	if err := n.BaseTx.Validate(); err != nil {
		return err
	}

	// TransferFee must be <= 50000 (50%)
	if n.TransferFee != nil && *n.TransferFee > 50000 {
		return errors.New("TransferFee cannot exceed 50000")
	}

	// URI must be hex-encoded and <= 512 bytes
	if len(n.URI) > 1024 { // 512 bytes = 1024 hex chars
		return errors.New("URI too long")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (n *NFTokenMint) Flatten() (map[string]any, error) {
	m := n.Common.ToMap()

	m["NFTokenTaxon"] = n.NFTokenTaxon

	if n.Issuer != "" {
		m["Issuer"] = n.Issuer
	}
	if n.TransferFee != nil {
		m["TransferFee"] = *n.TransferFee
	}
	if n.URI != "" {
		m["URI"] = n.URI
	}
	if n.Amount != nil {
		m["Amount"] = flattenAmount(*n.Amount)
	}
	if n.Destination != "" {
		m["Destination"] = n.Destination
	}
	if n.Expiration != nil {
		m["Expiration"] = *n.Expiration
	}

	return m, nil
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

// NFTokenBurn burns an NFToken.
type NFTokenBurn struct {
	BaseTx

	// NFTokenID is the ID of the token to burn (required)
	NFTokenID string `json:"NFTokenID"`

	// Owner is the owner of the token (optional, for authorized burns)
	Owner string `json:"Owner,omitempty"`
}

// NewNFTokenBurn creates a new NFTokenBurn transaction
func NewNFTokenBurn(account, nftokenID string) *NFTokenBurn {
	return &NFTokenBurn{
		BaseTx:    *NewBaseTx(TypeNFTokenBurn, account),
		NFTokenID: nftokenID,
	}
}

// TxType returns the transaction type
func (n *NFTokenBurn) TxType() Type {
	return TypeNFTokenBurn
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
	m := n.Common.ToMap()

	m["NFTokenID"] = n.NFTokenID

	if n.Owner != "" {
		m["Owner"] = n.Owner
	}

	return m, nil
}

// NFTokenCreateOffer creates an offer to buy or sell an NFToken.
type NFTokenCreateOffer struct {
	BaseTx

	// NFTokenID is the ID of the token (required)
	NFTokenID string `json:"NFTokenID"`

	// Amount is the price for the offer (required)
	Amount Amount `json:"Amount"`

	// Owner is the owner of the token (required for buy offers)
	Owner string `json:"Owner,omitempty"`

	// Destination is who can accept this offer (optional)
	Destination string `json:"Destination,omitempty"`

	// Expiration is when the offer expires (optional)
	Expiration *uint32 `json:"Expiration,omitempty"`
}

// NFTokenCreateOffer flags
const (
	// tfSellNFToken indicates this is a sell offer
	NFTokenCreateOfferFlagSellNFToken uint32 = 0x00000001
)

// NewNFTokenCreateOffer creates a new NFTokenCreateOffer transaction
func NewNFTokenCreateOffer(account, nftokenID string, amount Amount) *NFTokenCreateOffer {
	return &NFTokenCreateOffer{
		BaseTx:    *NewBaseTx(TypeNFTokenCreateOffer, account),
		NFTokenID: nftokenID,
		Amount:    amount,
	}
}

// TxType returns the transaction type
func (n *NFTokenCreateOffer) TxType() Type {
	return TypeNFTokenCreateOffer
}

// Validate validates the NFTokenCreateOffer transaction
func (n *NFTokenCreateOffer) Validate() error {
	if err := n.BaseTx.Validate(); err != nil {
		return err
	}

	if n.NFTokenID == "" {
		return errors.New("NFTokenID is required")
	}

	// Buy offers must have Owner
	isSellOffer := n.GetFlags()&NFTokenCreateOfferFlagSellNFToken != 0
	if !isSellOffer && n.Owner == "" {
		return errors.New("Owner is required for buy offers")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (n *NFTokenCreateOffer) Flatten() (map[string]any, error) {
	m := n.Common.ToMap()

	m["NFTokenID"] = n.NFTokenID
	m["Amount"] = flattenAmount(n.Amount)

	if n.Owner != "" {
		m["Owner"] = n.Owner
	}
	if n.Destination != "" {
		m["Destination"] = n.Destination
	}
	if n.Expiration != nil {
		m["Expiration"] = *n.Expiration
	}

	return m, nil
}

// SetSellOffer marks this as a sell offer
func (n *NFTokenCreateOffer) SetSellOffer() {
	flags := n.GetFlags() | NFTokenCreateOfferFlagSellNFToken
	n.SetFlags(flags)
}

// NFTokenCancelOffer cancels NFToken offers.
type NFTokenCancelOffer struct {
	BaseTx

	// NFTokenOffers is the list of offer IDs to cancel (required)
	NFTokenOffers []string `json:"NFTokenOffers"`
}

// NewNFTokenCancelOffer creates a new NFTokenCancelOffer transaction
func NewNFTokenCancelOffer(account string, offerIDs []string) *NFTokenCancelOffer {
	return &NFTokenCancelOffer{
		BaseTx:        *NewBaseTx(TypeNFTokenCancelOffer, account),
		NFTokenOffers: offerIDs,
	}
}

// TxType returns the transaction type
func (n *NFTokenCancelOffer) TxType() Type {
	return TypeNFTokenCancelOffer
}

// Validate validates the NFTokenCancelOffer transaction
func (n *NFTokenCancelOffer) Validate() error {
	if err := n.BaseTx.Validate(); err != nil {
		return err
	}

	if len(n.NFTokenOffers) == 0 {
		return errors.New("NFTokenOffers is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (n *NFTokenCancelOffer) Flatten() (map[string]any, error) {
	m := n.Common.ToMap()
	m["NFTokenOffers"] = n.NFTokenOffers
	return m, nil
}

// NFTokenAcceptOffer accepts an NFToken offer.
type NFTokenAcceptOffer struct {
	BaseTx

	// NFTokenSellOffer is the sell offer to accept (optional)
	NFTokenSellOffer string `json:"NFTokenSellOffer,omitempty"`

	// NFTokenBuyOffer is the buy offer to accept (optional)
	NFTokenBuyOffer string `json:"NFTokenBuyOffer,omitempty"`

	// NFTokenBrokerFee is the broker fee for brokered sales (optional)
	NFTokenBrokerFee *Amount `json:"NFTokenBrokerFee,omitempty"`
}

// NewNFTokenAcceptOffer creates a new NFTokenAcceptOffer transaction
func NewNFTokenAcceptOffer(account string) *NFTokenAcceptOffer {
	return &NFTokenAcceptOffer{
		BaseTx: *NewBaseTx(TypeNFTokenAcceptOffer, account),
	}
}

// TxType returns the transaction type
func (n *NFTokenAcceptOffer) TxType() Type {
	return TypeNFTokenAcceptOffer
}

// Validate validates the NFTokenAcceptOffer transaction
func (n *NFTokenAcceptOffer) Validate() error {
	if err := n.BaseTx.Validate(); err != nil {
		return err
	}

	// Must have at least one offer
	if n.NFTokenSellOffer == "" && n.NFTokenBuyOffer == "" {
		return errors.New("must specify NFTokenSellOffer or NFTokenBuyOffer")
	}

	// BrokerFee only valid for brokered mode (both offers)
	if n.NFTokenBrokerFee != nil {
		if n.NFTokenSellOffer == "" || n.NFTokenBuyOffer == "" {
			return errors.New("NFTokenBrokerFee requires both sell and buy offers")
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (n *NFTokenAcceptOffer) Flatten() (map[string]any, error) {
	m := n.Common.ToMap()

	if n.NFTokenSellOffer != "" {
		m["NFTokenSellOffer"] = n.NFTokenSellOffer
	}
	if n.NFTokenBuyOffer != "" {
		m["NFTokenBuyOffer"] = n.NFTokenBuyOffer
	}
	if n.NFTokenBrokerFee != nil {
		m["NFTokenBrokerFee"] = flattenAmount(*n.NFTokenBrokerFee)
	}

	return m, nil
}

// SetSellOffer sets the sell offer to accept
func (n *NFTokenAcceptOffer) SetSellOffer(offerID string) {
	n.NFTokenSellOffer = offerID
}

// SetBuyOffer sets the buy offer to accept
func (n *NFTokenAcceptOffer) SetBuyOffer(offerID string) {
	n.NFTokenBuyOffer = offerID
}
