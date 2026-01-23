package tx

import (
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

func init() {
	Register(TypeNFTokenMint, func() Transaction {
		return &NFTokenMint{BaseTx: *NewBaseTx(TypeNFTokenMint, "")}
	})
	Register(TypeNFTokenBurn, func() Transaction {
		return &NFTokenBurn{BaseTx: *NewBaseTx(TypeNFTokenBurn, "")}
	})
	Register(TypeNFTokenCreateOffer, func() Transaction {
		return &NFTokenCreateOffer{BaseTx: *NewBaseTx(TypeNFTokenCreateOffer, "")}
	})
	Register(TypeNFTokenCancelOffer, func() Transaction {
		return &NFTokenCancelOffer{BaseTx: *NewBaseTx(TypeNFTokenCancelOffer, "")}
	})
	Register(TypeNFTokenAcceptOffer, func() Transaction {
		return &NFTokenAcceptOffer{BaseTx: *NewBaseTx(TypeNFTokenAcceptOffer, "")}
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
	BaseTx

	// NFTokenTaxon is the taxon for this token (required)
	NFTokenTaxon uint32 `json:"NFTokenTaxon" xrpl:"NFTokenTaxon"`

	// Issuer is the issuer of the token (optional, defaults to Account)
	Issuer string `json:"Issuer,omitempty" xrpl:"Issuer,omitempty"`

	// TransferFee is the fee for secondary sales (0-50000, where 50000 = 50%)
	TransferFee *uint16 `json:"TransferFee,omitempty" xrpl:"TransferFee,omitempty"`

	// URI is the URI for the token metadata (optional)
	URI string `json:"URI,omitempty" xrpl:"URI,omitempty"`

	// Amount is the minting price (optional)
	Amount *Amount `json:"Amount,omitempty" xrpl:"Amount,omitempty,amount"`

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
		BaseTx:       *NewBaseTx(TypeNFTokenMint, account),
		NFTokenTaxon: taxon,
	}
}

// TxType returns the transaction type
func (n *NFTokenMint) TxType() Type {
	return TypeNFTokenMint
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
	return ReflectFlatten(n)
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
	return []string{AmendmentNonFungibleTokensV1}
}

// NFTokenBurn burns an NFToken.
type NFTokenBurn struct {
	BaseTx

	// NFTokenID is the ID of the token to burn (required)
	NFTokenID string `json:"NFTokenID" xrpl:"NFTokenID"`

	// Owner is the owner of the token (optional, for authorized burns)
	Owner string `json:"Owner,omitempty" xrpl:"Owner,omitempty"`
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
	return ReflectFlatten(n)
}

// RequiredAmendments returns the amendments required for this transaction type
func (n *NFTokenBurn) RequiredAmendments() []string {
	return []string{AmendmentNonFungibleTokensV1}
}

// NFTokenCreateOffer creates an offer to buy or sell an NFToken.
type NFTokenCreateOffer struct {
	BaseTx

	// NFTokenID is the ID of the token (required)
	NFTokenID string `json:"NFTokenID" xrpl:"NFTokenID"`

	// Amount is the price for the offer (required)
	Amount Amount `json:"Amount" xrpl:"Amount,amount"`

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
	return ReflectFlatten(n)
}

// SetSellOffer marks this as a sell offer
func (n *NFTokenCreateOffer) SetSellOffer() {
	flags := n.GetFlags() | NFTokenCreateOfferFlagSellNFToken
	n.SetFlags(flags)
}

// RequiredAmendments returns the amendments required for this transaction type
func (n *NFTokenCreateOffer) RequiredAmendments() []string {
	return []string{AmendmentNonFungibleTokensV1}
}

// NFTokenCancelOffer cancels NFToken offers.
type NFTokenCancelOffer struct {
	BaseTx

	// NFTokenOffers is the list of offer IDs to cancel (required)
	NFTokenOffers []string `json:"NFTokenOffers" xrpl:"NFTokenOffers"`
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
	return ReflectFlatten(n)
}

// RequiredAmendments returns the amendments required for this transaction type
func (n *NFTokenCancelOffer) RequiredAmendments() []string {
	return []string{AmendmentNonFungibleTokensV1}
}

// NFTokenAcceptOffer accepts an NFToken offer.
type NFTokenAcceptOffer struct {
	BaseTx

	// NFTokenSellOffer is the sell offer to accept (optional)
	NFTokenSellOffer string `json:"NFTokenSellOffer,omitempty" xrpl:"NFTokenSellOffer,omitempty"`

	// NFTokenBuyOffer is the buy offer to accept (optional)
	NFTokenBuyOffer string `json:"NFTokenBuyOffer,omitempty" xrpl:"NFTokenBuyOffer,omitempty"`

	// NFTokenBrokerFee is the broker fee for brokered sales (optional)
	NFTokenBrokerFee *Amount `json:"NFTokenBrokerFee,omitempty" xrpl:"NFTokenBrokerFee,omitempty,amount"`
}

// NFTokenAcceptOffer has no transaction flags
const (
	// tfNFTokenAcceptOfferMask is the mask for invalid flags (all flags are invalid)
	tfNFTokenAcceptOfferMask uint32 = 0xFFFFFFFF
)

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
	return ReflectFlatten(n)
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
	return []string{AmendmentNonFungibleTokensV1}
}

// Apply applies the NFTokenMint transaction to the ledger.
// Reference: rippled NFTokenMint.cpp doApply
func (m *NFTokenMint) Apply(ctx *ApplyContext) Result {
	accountID := ctx.AccountID

	// Determine the issuer
	var issuerID [20]byte
	var issuerAccount *AccountRoot
	var issuerKey keylet.Keylet

	if m.Issuer != "" {
		var err error
		issuerID, err = decodeAccountID(m.Issuer)
		if err != nil {
			return TemINVALID
		}

		// Read issuer account for MintedNFTokens tracking
		issuerKey = keylet.Account(issuerID)
		issuerData, err := ctx.View.Read(issuerKey)
		if err != nil {
			return TecNO_ISSUER
		}
		issuerAccount, err = parseAccountRoot(issuerData)
		if err != nil {
			return TefINTERNAL
		}

		// Verify that Account is authorized to mint for this issuer
		// The issuer must have set Account as their NFTokenMinter
		if issuerAccount.NFTokenMinter != m.Account {
			return TecNO_PERMISSION
		}
	} else {
		issuerID = accountID
		issuerAccount = ctx.Account
	}

	// Get the token sequence from MintedNFTokens
	tokenSeq := issuerAccount.MintedNFTokens

	// Check for overflow
	if tokenSeq+1 < tokenSeq {
		return TecMAX_SEQUENCE_REACHED
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
	newToken := NFTokenData{
		NFTokenID: tokenID,
		URI:       m.URI,
	}

	insertResult := insertNFToken(accountID, newToken, ctx.View)
	if insertResult.Result != TesSUCCESS {
		return insertResult.Result
	}

	// Update owner count based on pages created
	ctx.Account.OwnerCount += uint32(insertResult.PagesCreated)

	// Update MintedNFTokens on the issuer account
	issuerAccount.MintedNFTokens = tokenSeq + 1

	// If issuer is different from minter, update the issuer account - tracked automatically
	if m.Issuer != "" {
		issuerUpdatedData, err := serializeAccountRoot(issuerAccount)
		if err != nil {
			return TefINTERNAL
		}
		if err := ctx.View.Update(issuerKey, issuerUpdatedData); err != nil {
			return TefINTERNAL
		}
	}

	// Check reserve if pages were created (owner count increased)
	if insertResult.PagesCreated > 0 {
		reserve := ctx.AccountReserve(ctx.Account.OwnerCount)
		if ctx.Account.Balance < reserve {
			return TecINSUFFICIENT_RESERVE
		}
	}

	return TesSUCCESS
}

// Apply applies the NFTokenBurn transaction to the ledger.
// Reference: rippled NFTokenBurn.cpp doApply
func (b *NFTokenBurn) Apply(ctx *ApplyContext) Result {
	accountID := ctx.AccountID

	// Parse the token ID
	tokenIDBytes, err := hex.DecodeString(b.NFTokenID)
	if err != nil || len(tokenIDBytes) != 32 {
		return TemINVALID
	}

	var tokenID [32]byte
	copy(tokenID[:], tokenIDBytes)

	// Determine the owner
	var ownerID [20]byte
	if b.Owner != "" {
		ownerID, err = decodeAccountID(b.Owner)
		if err != nil {
			return TemINVALID
		}
	} else {
		ownerID = accountID
	}

	// Find the NFToken page
	pageKey := keylet.NFTokenPage(ownerID, tokenID)

	pageData, err := ctx.View.Read(pageKey)
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
			issuerData, err := ctx.View.Read(issuerKey)
			if err != nil {
				return TecNO_PERMISSION
			}
			issuerAccount, err := parseAccountRoot(issuerData)
			if err != nil {
				return TefINTERNAL
			}
			if issuerAccount.NFTokenMinter != b.Account {
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
		ownerData, err := ctx.View.Read(ownerKey)
		if err != nil {
			return TefINTERNAL
		}
		ownerAccount, err = parseAccountRoot(ownerData)
		if err != nil {
			return TefINTERNAL
		}
	} else {
		ownerAccount = ctx.Account
	}

	// Update or delete the page - changes tracked automatically by ApplyStateTable
	if len(page.NFTokens) == 0 {
		// Delete empty page
		if err := ctx.View.Erase(pageKey); err != nil {
			return TefINTERNAL
		}

		if ownerAccount.OwnerCount > 0 {
			ownerAccount.OwnerCount--
		}
	} else {
		// Update page
		updatedPageData, err := serializeNFTokenPage(page)
		if err != nil {
			return TefINTERNAL
		}

		if err := ctx.View.Update(pageKey, updatedPageData); err != nil {
			return TefINTERNAL
		}
	}

	// Update owner account if different from transaction sender
	if ownerID != accountID {
		ownerUpdatedData, err := serializeAccountRoot(ownerAccount)
		if err != nil {
			return TefINTERNAL
		}
		if err := ctx.View.Update(ownerKey, ownerUpdatedData); err != nil {
			return TefINTERNAL
		}
	}

	// Update BurnedNFTokens on the issuer - changes tracked automatically
	issuerID := getNFTIssuer(tokenID)
	issuerKey := keylet.Account(issuerID)
	issuerData, err := ctx.View.Read(issuerKey)
	if err == nil {
		issuerAccount, err := parseAccountRoot(issuerData)
		if err == nil {
			issuerAccount.BurnedNFTokens++
			issuerUpdatedData, err := serializeAccountRoot(issuerAccount)
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

	return TesSUCCESS
}

// Apply applies the NFTokenCreateOffer transaction to the ledger.
// Reference: rippled NFTokenCreateOffer.cpp doApply
func (c *NFTokenCreateOffer) Apply(ctx *ApplyContext) Result {
	accountID := ctx.AccountID

	// Parse token ID
	tokenIDBytes, err := hex.DecodeString(c.NFTokenID)
	if err != nil || len(tokenIDBytes) != 32 {
		return TemINVALID
	}

	var tokenID [32]byte
	copy(tokenID[:], tokenIDBytes)

	// Check expiration
	if c.Expiration != nil && *c.Expiration <= ctx.Config.ParentCloseTime {
		return TecEXPIRED
	}

	// Check if this is a sell offer
	isSellOffer := c.GetFlags()&NFTokenCreateOfferFlagSellNFToken != 0

	// Verify token ownership
	if isSellOffer {
		// For sell offers, verify the sender owns the token
		pageKey := keylet.NFTokenPage(accountID, tokenID)
		pageData, err := ctx.View.Read(pageKey)
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
		ownerID, err = decodeAccountID(c.Owner)
		if err != nil {
			return TemINVALID
		}
		pageKey := keylet.NFTokenPage(ownerID, tokenID)
		pageData, err := ctx.View.Read(pageKey)
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
	if c.Amount.Currency == "" {
		// XRP amount
		amountXRP, err = strconv.ParseUint(c.Amount.Value, 10, 64)
		if err != nil {
			return TemMALFORMED
		}
	}

	// For buy offers, escrow the funds
	if !isSellOffer {
		if c.Amount.Currency == "" && amountXRP > 0 {
			// Check if account has enough balance (including reserve)
			reserve := ctx.AccountReserve(ctx.Account.OwnerCount + 1)
			if ctx.Account.Balance < amountXRP+reserve {
				return TecINSUFFICIENT_FUNDS
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
		return TefINTERNAL
	}

	if err := ctx.View.Insert(offerKey, offerData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	ctx.Account.OwnerCount++

	// Check reserve
	reserve := ctx.AccountReserve(ctx.Account.OwnerCount)
	if ctx.Account.Balance < reserve {
		return TecINSUFFICIENT_RESERVE
	}

	// Creation tracked automatically by ApplyStateTable

	return TesSUCCESS
}

// Apply applies the NFTokenCancelOffer transaction to the ledger.
// Reference: rippled NFTokenCancelOffer.cpp doApply and preclaim
func (co *NFTokenCancelOffer) Apply(ctx *ApplyContext) Result {
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
		offer, err := parseNFTokenOffer(offerData)
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
			return TecNO_PERMISSION
		}

		// Get the offer owner's account to update their owner count and potentially refund
		var ownerAccount *AccountRoot
		var ownerKey keylet.Keylet

		if offer.Owner == accountID {
			ownerAccount = ctx.Account
		} else {
			ownerKey = keylet.Account(offer.Owner)
			ownerData, err := ctx.View.Read(ownerKey)
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

		// Update owner account if different from transaction sender - tracked automatically
		if offer.Owner != accountID {
			ownerUpdatedData, err := serializeAccountRoot(ownerAccount)
			if err != nil {
				return TefINTERNAL
			}
			if err := ctx.View.Update(ownerKey, ownerUpdatedData); err != nil {
				return TefINTERNAL
			}
		}

		// Delete the offer - tracked automatically by ApplyStateTable
		if err := ctx.View.Erase(offerKey); err != nil {
			return TefBAD_LEDGER
		}
	}

	return TesSUCCESS
}

// Apply applies the NFTokenAcceptOffer transaction to the ledger.
// Reference: rippled NFTokenAcceptOffer.cpp doApply
func (a *NFTokenAcceptOffer) Apply(ctx *ApplyContext) Result {
	accountID := ctx.AccountID

	// Load offers
	var buyOffer, sellOffer *NFTokenOfferData
	var buyOfferKey, sellOfferKey keylet.Keylet

	if a.NFTokenBuyOffer != "" {
		buyOfferIDBytes, err := hex.DecodeString(a.NFTokenBuyOffer)
		if err != nil || len(buyOfferIDBytes) != 32 {
			return TemINVALID
		}
		var buyOfferKeyBytes [32]byte
		copy(buyOfferKeyBytes[:], buyOfferIDBytes)
		buyOfferKey = keylet.Keylet{Key: buyOfferKeyBytes}

		buyOfferData, err := ctx.View.Read(buyOfferKey)
		if err != nil {
			return TecOBJECT_NOT_FOUND
		}
		buyOffer, err = parseNFTokenOffer(buyOfferData)
		if err != nil {
			return TefINTERNAL
		}

		// Check expiration
		if buyOffer.Expiration != 0 && buyOffer.Expiration <= ctx.Config.ParentCloseTime {
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

	if a.NFTokenSellOffer != "" {
		sellOfferIDBytes, err := hex.DecodeString(a.NFTokenSellOffer)
		if err != nil || len(sellOfferIDBytes) != 32 {
			return TemINVALID
		}
		var sellOfferKeyBytes [32]byte
		copy(sellOfferKeyBytes[:], sellOfferIDBytes)
		sellOfferKey = keylet.Keylet{Key: sellOfferKeyBytes}

		sellOfferData, err := ctx.View.Read(sellOfferKey)
		if err != nil {
			return TecOBJECT_NOT_FOUND
		}
		sellOffer, err = parseNFTokenOffer(sellOfferData)
		if err != nil {
			return TefINTERNAL
		}

		// Check expiration
		if sellOffer.Expiration != 0 && sellOffer.Expiration <= ctx.Config.ParentCloseTime {
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

	return TemINVALID
}
