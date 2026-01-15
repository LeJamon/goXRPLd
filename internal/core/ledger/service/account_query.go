package service

import (
	"errors"
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
)

// AccountInfoResult contains account information from the ledger
type AccountInfoResult struct {
	Account      string
	Balance      uint64
	Flags        uint32
	OwnerCount   uint32
	Sequence     uint32
	RegularKey   string
	Domain       string
	EmailHash    string
	TransferRate uint32
	TickSize     uint8
	LedgerIndex  uint32
	LedgerHash   [32]byte
	Validated    bool
}

// GetAccountInfo retrieves account information from the ledger
func (s *Service) GetAccountInfo(account string, ledgerIndex string) (*AccountInfoResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Determine which ledger to use
	var targetLedger *ledger.Ledger
	var validated bool

	switch ledgerIndex {
	case "current", "":
		targetLedger = s.openLedger
		validated = false
	case "closed":
		targetLedger = s.closedLedger
		validated = s.closedLedger == s.validatedLedger
	case "validated":
		targetLedger = s.validatedLedger
		validated = true
	default:
		// Try to parse as a number
		seq, err := strconv.ParseUint(ledgerIndex, 10, 32)
		if err != nil {
			return nil, errors.New("invalid ledger_index")
		}
		var ok bool
		targetLedger, ok = s.ledgerHistory[uint32(seq)]
		if !ok {
			return nil, ErrLedgerNotFound
		}
		validated = targetLedger.IsValidated()
	}

	if targetLedger == nil {
		return nil, ErrNoOpenLedger
	}

	// Decode the account address to get the account ID
	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(account)
	if err != nil {
		return nil, errors.New("invalid account address: " + err.Error())
	}

	var accountID [20]byte
	copy(accountID[:], accountIDBytes)

	// Get the account keylet
	accountKey := keylet.Account(accountID)

	// Check if account exists
	exists, err := targetLedger.Exists(accountKey)
	if err != nil {
		return nil, errors.New("failed to check account existence: " + err.Error())
	}
	if !exists {
		return nil, errors.New("account not found")
	}

	// Read the account data
	data, err := targetLedger.Read(accountKey)
	if err != nil {
		return nil, errors.New("failed to read account: " + err.Error())
	}

	// Parse the account root
	accountRoot, err := tx.ParseAccountRootFromBytes(data)
	if err != nil {
		return nil, errors.New("failed to parse account data: " + err.Error())
	}

	return &AccountInfoResult{
		Account:      account,
		Balance:      accountRoot.Balance,
		Flags:        accountRoot.Flags,
		OwnerCount:   accountRoot.OwnerCount,
		Sequence:     accountRoot.Sequence,
		RegularKey:   accountRoot.RegularKey,
		Domain:       accountRoot.Domain,
		EmailHash:    accountRoot.EmailHash,
		TransferRate: accountRoot.TransferRate,
		TickSize:     accountRoot.TickSize,
		LedgerIndex:  targetLedger.Sequence(),
		LedgerHash:   targetLedger.Hash(),
		Validated:    validated,
	}, nil
}

// TrustLine represents a trust line from account_lines RPC
type TrustLine struct {
	Account        string `json:"account"`
	Balance        string `json:"balance"`
	Currency       string `json:"currency"`
	Limit          string `json:"limit"`
	LimitPeer      string `json:"limit_peer"`
	QualityIn      uint32 `json:"quality_in,omitempty"`
	QualityOut     uint32 `json:"quality_out,omitempty"`
	NoRipple       bool   `json:"no_ripple,omitempty"`
	NoRipplePeer   bool   `json:"no_ripple_peer,omitempty"`
	Authorized     bool   `json:"authorized,omitempty"`
	PeerAuthorized bool   `json:"peer_authorized,omitempty"`
	Freeze         bool   `json:"freeze,omitempty"`
	FreezePeer     bool   `json:"freeze_peer,omitempty"`
}

// AccountLinesResult contains the result of account_lines RPC
type AccountLinesResult struct {
	Account     string      `json:"account"`
	Lines       []TrustLine `json:"lines"`
	LedgerIndex uint32      `json:"ledger_index"`
	LedgerHash  [32]byte    `json:"ledger_hash"`
	Validated   bool        `json:"validated"`
	Marker      string      `json:"marker,omitempty"`
}

// GetAccountLines retrieves trust lines for an account
func (s *Service) GetAccountLines(account string, ledgerIndex string, peer string, limit uint32) (*AccountLinesResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Determine which ledger to use
	targetLedger, validated, err := s.getLedgerForQuery(ledgerIndex)
	if err != nil {
		return nil, err
	}

	// Decode the account address
	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(account)
	if err != nil {
		return nil, errors.New("invalid account address: " + err.Error())
	}
	var accountID [20]byte
	copy(accountID[:], accountIDBytes)

	// Parse peer if provided
	var peerID [20]byte
	hasPeer := false
	if peer != "" {
		_, peerIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(peer)
		if err != nil {
			return nil, errors.New("invalid peer address: " + err.Error())
		}
		copy(peerID[:], peerIDBytes)
		hasPeer = true
	}

	// Set default limit
	if limit == 0 || limit > 400 {
		limit = 200
	}

	// Collect trust lines by iterating through ledger entries
	var lines []TrustLine

	targetLedger.ForEach(func(key [32]byte, data []byte) bool {
		// Check if we've reached the limit
		if uint32(len(lines)) >= limit {
			return false
		}

		// Check if this is a RippleState entry (trust line)
		if len(data) < 3 {
			return true
		}

		// Check LedgerEntryType field
		if data[0] != 0x11 { // UInt16 type code 1, field code 1
			return true
		}
		entryType := uint16(data[1])<<8 | uint16(data[2])
		if entryType != 0x0072 { // RippleState type
			return true
		}

		// Parse the RippleState
		rs, err := tx.ParseRippleStateFromBytes(data)
		if err != nil {
			return true
		}

		// Check if this trust line involves our account
		lowID, _ := decodeAccountIDLocal(rs.LowLimit.Issuer)
		highID, _ := decodeAccountIDLocal(rs.HighLimit.Issuer)

		var isLowAccount bool
		var peerAccount string

		if lowID == accountID {
			isLowAccount = true
			peerAccount = rs.HighLimit.Issuer
		} else if highID == accountID {
			isLowAccount = false
			peerAccount = rs.LowLimit.Issuer
		} else {
			return true // Not our account
		}

		// Filter by peer if specified
		if hasPeer {
			peerAccountID, _ := decodeAccountIDLocal(peerAccount)
			if peerAccountID != peerID {
				return true
			}
		}

		// Build trust line response
		line := TrustLine{
			Account:  peerAccount,
			Currency: rs.Balance.Currency,
		}

		// Calculate balance from perspective of our account
		// Positive balance means peer owes us, negative means we owe peer
		if isLowAccount {
			// We are low account
			// Balance is positive if low owes high (we owe them) -> negative for us
			// Balance is negative if high owes low (they owe us) -> positive for us
			line.Balance = rs.Balance.Value.Neg(rs.Balance.Value).Text('f', -1)
			line.Limit = rs.LowLimit.Value.Text('f', -1)
			line.LimitPeer = rs.HighLimit.Value.Text('f', -1)
			line.NoRipple = (rs.Flags & 0x00020000) != 0       // lsfLowNoRipple
			line.NoRipplePeer = (rs.Flags & 0x00040000) != 0   // lsfHighNoRipple
			line.Authorized = (rs.Flags & 0x00010000) != 0     // lsfLowAuth
			line.PeerAuthorized = (rs.Flags & 0x00080000) != 0 // lsfHighAuth
			line.Freeze = (rs.Flags & 0x00400000) != 0         // lsfLowFreeze
			line.FreezePeer = (rs.Flags & 0x00800000) != 0     // lsfHighFreeze
		} else {
			// We are high account
			line.Balance = rs.Balance.Value.Text('f', -1)
			line.Limit = rs.HighLimit.Value.Text('f', -1)
			line.LimitPeer = rs.LowLimit.Value.Text('f', -1)
			line.NoRipple = (rs.Flags & 0x00040000) != 0       // lsfHighNoRipple
			line.NoRipplePeer = (rs.Flags & 0x00020000) != 0   // lsfLowNoRipple
			line.Authorized = (rs.Flags & 0x00080000) != 0     // lsfHighAuth
			line.PeerAuthorized = (rs.Flags & 0x00010000) != 0 // lsfLowAuth
			line.Freeze = (rs.Flags & 0x00800000) != 0         // lsfHighFreeze
			line.FreezePeer = (rs.Flags & 0x00400000) != 0     // lsfLowFreeze
		}

		line.QualityIn = rs.LowQualityIn
		line.QualityOut = rs.LowQualityOut

		lines = append(lines, line)
		return true
	})

	return &AccountLinesResult{
		Account:     account,
		Lines:       lines,
		LedgerIndex: targetLedger.Sequence(),
		LedgerHash:  targetLedger.Hash(),
		Validated:   validated,
	}, nil
}

// AccountOffer represents an offer from account_offers RPC
type AccountOffer struct {
	Flags      uint32      `json:"flags"`
	Seq        uint32      `json:"seq"`
	TakerGets  interface{} `json:"taker_gets"`
	TakerPays  interface{} `json:"taker_pays"`
	Quality    string      `json:"quality"`
	Expiration uint32      `json:"expiration,omitempty"`
}

// AccountOffersResult contains the result of account_offers RPC
type AccountOffersResult struct {
	Account     string         `json:"account"`
	Offers      []AccountOffer `json:"offers"`
	LedgerIndex uint32         `json:"ledger_index"`
	LedgerHash  [32]byte       `json:"ledger_hash"`
	Validated   bool           `json:"validated"`
	Marker      string         `json:"marker,omitempty"`
}

// GetAccountOffers retrieves offers for an account
func (s *Service) GetAccountOffers(account string, ledgerIndex string, limit uint32) (*AccountOffersResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Determine which ledger to use
	targetLedger, validated, err := s.getLedgerForQuery(ledgerIndex)
	if err != nil {
		return nil, err
	}

	// Decode the account address
	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(account)
	if err != nil {
		return nil, errors.New("invalid account address: " + err.Error())
	}
	var accountID [20]byte
	copy(accountID[:], accountIDBytes)

	// Set default limit
	if limit == 0 || limit > 400 {
		limit = 200
	}

	// Collect offers by iterating through ledger entries
	var offers []AccountOffer

	targetLedger.ForEach(func(key [32]byte, data []byte) bool {
		// Check if we've reached the limit
		if uint32(len(offers)) >= limit {
			return false
		}

		// Check if this is an Offer entry
		if len(data) < 3 {
			return true
		}

		// Check LedgerEntryType field
		if data[0] != 0x11 { // UInt16 type code 1, field code 1
			return true
		}
		entryType := uint16(data[1])<<8 | uint16(data[2])
		if entryType != 0x006F { // Offer type
			return true
		}

		// Parse the Offer
		offer, err := tx.ParseLedgerOfferFromBytes(data)
		if err != nil {
			return true
		}

		// Check if this offer belongs to our account
		offerAccountID, _ := decodeAccountIDLocal(offer.Account)
		if offerAccountID != accountID {
			return true
		}

		// Build offer response
		accountOffer := AccountOffer{
			Flags: offer.Flags,
			Seq:   offer.Sequence,
		}

		// Format TakerGets
		if offer.TakerGets.IsNative() {
			accountOffer.TakerGets = offer.TakerGets.Value
		} else {
			accountOffer.TakerGets = map[string]string{
				"currency": offer.TakerGets.Currency,
				"issuer":   offer.TakerGets.Issuer,
				"value":    offer.TakerGets.Value,
			}
		}

		// Format TakerPays
		if offer.TakerPays.IsNative() {
			accountOffer.TakerPays = offer.TakerPays.Value
		} else {
			accountOffer.TakerPays = map[string]string{
				"currency": offer.TakerPays.Currency,
				"issuer":   offer.TakerPays.Issuer,
				"value":    offer.TakerPays.Value,
			}
		}

		// Calculate quality
		accountOffer.Quality = calculateOfferQuality(offer.TakerPays, offer.TakerGets)

		if offer.Expiration > 0 {
			accountOffer.Expiration = offer.Expiration
		}

		offers = append(offers, accountOffer)
		return true
	})

	return &AccountOffersResult{
		Account:     account,
		Offers:      offers,
		LedgerIndex: targetLedger.Sequence(),
		LedgerHash:  targetLedger.Hash(),
		Validated:   validated,
	}, nil
}

// AccountObjectsResult contains account objects
type AccountObjectsResult struct {
	Account        string              `json:"account"`
	AccountObjects []AccountObjectItem `json:"account_objects"`
	LedgerIndex    uint32              `json:"ledger_index"`
	LedgerHash     [32]byte            `json:"ledger_hash"`
	Validated      bool                `json:"validated"`
	Marker         string              `json:"marker,omitempty"`
}

// AccountObjectItem represents an account object
type AccountObjectItem struct {
	Index           string `json:"index"`
	LedgerEntryType string `json:"LedgerEntryType"`
	Data            []byte `json:"data"`
}

// GetAccountObjects retrieves all objects owned by an account
func (s *Service) GetAccountObjects(account string, ledgerIndex string, objType string, limit uint32) (*AccountObjectsResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetLedger, validated, err := s.getLedgerForQuery(ledgerIndex)
	if err != nil {
		return nil, err
	}

	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(account)
	if err != nil {
		return nil, errors.New("invalid account address: " + err.Error())
	}
	var accountID [20]byte
	copy(accountID[:], accountIDBytes)

	if limit == 0 || limit > 400 {
		limit = 200
	}

	result := &AccountObjectsResult{
		Account:        account,
		AccountObjects: make([]AccountObjectItem, 0),
		LedgerIndex:    targetLedger.Sequence(),
		LedgerHash:     targetLedger.Hash(),
		Validated:      validated,
	}

	// Iterate through ledger and find objects for this account
	count := uint32(0)
	targetLedger.ForEach(func(key [32]byte, data []byte) bool {
		if count >= limit {
			return false
		}

		// Check if this object belongs to the account
		entryType := getLedgerEntryType(data)
		if entryType == "" {
			return true
		}

		// Filter by type if specified
		if objType != "" && entryType != objType {
			return true
		}

		// Check if object is associated with the account
		if !isObjectForAccount(data, accountID, entryType) {
			return true
		}

		result.AccountObjects = append(result.AccountObjects, AccountObjectItem{
			Index:           formatHashHex(key),
			LedgerEntryType: entryType,
			Data:            data,
		})
		count++
		return true
	})

	return result, nil
}
