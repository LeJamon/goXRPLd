package service

import (
	"errors"
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
)

// QueryService handles ledger data queries.
type QueryService struct {
	ledgerManager *LedgerManager
}

// NewQueryService creates a new query service.
func NewQueryService(ledgerManager *LedgerManager) *QueryService {
	return &QueryService{
		ledgerManager: ledgerManager,
	}
}

// GetLedgerForQuery determines which ledger to use for a query.
func (q *QueryService) GetLedgerForQuery(ledgerIndex string) (*ledger.Ledger, bool, error) {
	var targetLedger *ledger.Ledger
	var validated bool

	switch ledgerIndex {
	case "current", "":
		targetLedger = q.ledgerManager.GetOpenLedger()
		validated = false
	case "closed":
		targetLedger = q.ledgerManager.GetClosedLedger()
		validatedLedger := q.ledgerManager.GetValidatedLedger()
		validated = targetLedger == validatedLedger
	case "validated":
		targetLedger = q.ledgerManager.GetValidatedLedger()
		validated = true
	default:
		seq, err := strconv.ParseUint(ledgerIndex, 10, 32)
		if err != nil {
			return nil, false, errors.New("invalid ledger_index")
		}
		var lookupErr error
		targetLedger, lookupErr = q.ledgerManager.GetLedgerBySequence(uint32(seq))
		if lookupErr != nil {
			return nil, false, ErrLedgerNotFound
		}
		validated = targetLedger.IsValidated()
	}

	if targetLedger == nil {
		return nil, false, ErrNoOpenLedger
	}

	return targetLedger, validated, nil
}

// GetAccountInfo retrieves account information from the ledger.
func (q *QueryService) GetAccountInfo(account string, ledgerIndex string) (*AccountInfoResult, error) {
	targetLedger, validated, err := q.GetLedgerForQuery(ledgerIndex)
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

// GetAccountLines retrieves trust lines for an account.
func (q *QueryService) GetAccountLines(account string, ledgerIndex string, peer string, limit uint32) (*AccountLinesResult, error) {
	targetLedger, validated, err := q.GetLedgerForQuery(ledgerIndex)
	if err != nil {
		return nil, err
	}

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

	if limit == 0 || limit > 400 {
		limit = 200
	}

	var lines []TrustLine

	targetLedger.ForEach(func(key [32]byte, data []byte) bool {
		if uint32(len(lines)) >= limit {
			return false
		}

		// Check if this is a RippleState entry
		if len(data) < 3 {
			return true
		}
		if data[0] != 0x11 {
			return true
		}
		entryType := uint16(data[1])<<8 | uint16(data[2])
		if entryType != 0x0072 {
			return true
		}

		rs, err := tx.ParseRippleStateFromBytes(data)
		if err != nil {
			return true
		}

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
			return true
		}

		if hasPeer {
			peerAccountID, _ := decodeAccountIDLocal(peerAccount)
			if peerAccountID != peerID {
				return true
			}
		}

		line := buildTrustLine(rs, isLowAccount, peerAccount)
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

// GetLedgerEntry retrieves a specific ledger entry by its key.
func (q *QueryService) GetLedgerEntry(entryKey [32]byte, ledgerIndex string) (*LedgerEntryResult, error) {
	targetLedger, validated, err := q.GetLedgerForQuery(ledgerIndex)
	if err != nil {
		return nil, err
	}

	k := keylet.Keylet{Key: entryKey}
	exists, err := targetLedger.Exists(k)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("entry not found")
	}

	data, err := targetLedger.Read(k)
	if err != nil {
		return nil, err
	}

	return &LedgerEntryResult{
		Index:       formatHashHex(entryKey),
		LedgerIndex: targetLedger.Sequence(),
		LedgerHash:  targetLedger.Hash(),
		Node:        data,
		Validated:   validated,
	}, nil
}

// GetLedgerData retrieves all ledger state entries with optional pagination.
func (q *QueryService) GetLedgerData(ledgerIndex string, limit uint32, marker string) (*LedgerDataResult, error) {
	targetLedger, validated, err := q.GetLedgerForQuery(ledgerIndex)
	if err != nil {
		return nil, err
	}

	if limit == 0 || limit > 2048 {
		limit = 256
	}

	result := &LedgerDataResult{
		LedgerIndex: targetLedger.Sequence(),
		LedgerHash:  targetLedger.Hash(),
		State:       make([]LedgerDataItem, 0, limit),
		Validated:   validated,
	}

	// Parse marker if provided
	var startKey [32]byte
	hasMarker := false
	if marker != "" {
		if len(marker) == 64 {
			decoded, err := hexDecode(marker)
			if err == nil && len(decoded) == 32 {
				copy(startKey[:], decoded)
				hasMarker = true
			}
		}
	}

	// Include ledger header info only on first query (no marker)
	if !hasMarker {
		hdr := targetLedger.Header()
		result.LedgerHeader = buildLedgerHeaderInfo(hdr, targetLedger)
	}

	count := uint32(0)
	var lastKey [32]byte
	passedMarker := !hasMarker

	targetLedger.ForEach(func(key [32]byte, data []byte) bool {
		if !passedMarker {
			if key == startKey {
				passedMarker = true
			}
			return true
		}

		if count >= limit {
			result.Marker = formatHashHex(lastKey)
			return false
		}

		result.State = append(result.State, LedgerDataItem{
			Index: formatHashHex(key),
			Data:  data,
		})
		lastKey = key
		count++
		return true
	})

	return result, nil
}

// buildTrustLine creates a TrustLine from RippleState data.
func buildTrustLine(rs *tx.RippleState, isLowAccount bool, peerAccount string) TrustLine {
	line := TrustLine{
		Account:  peerAccount,
		Currency: rs.Balance.Currency,
	}

	if isLowAccount {
		line.Balance = rs.Balance.Value.Neg(rs.Balance.Value).Text('f', -1)
		line.Limit = rs.LowLimit.Value.Text('f', -1)
		line.LimitPeer = rs.HighLimit.Value.Text('f', -1)
		line.NoRipple = (rs.Flags & 0x00020000) != 0
		line.NoRipplePeer = (rs.Flags & 0x00040000) != 0
		line.Authorized = (rs.Flags & 0x00010000) != 0
		line.PeerAuthorized = (rs.Flags & 0x00080000) != 0
		line.Freeze = (rs.Flags & 0x00400000) != 0
		line.FreezePeer = (rs.Flags & 0x00800000) != 0
	} else {
		line.Balance = rs.Balance.Value.Text('f', -1)
		line.Limit = rs.HighLimit.Value.Text('f', -1)
		line.LimitPeer = rs.LowLimit.Value.Text('f', -1)
		line.NoRipple = (rs.Flags & 0x00040000) != 0
		line.NoRipplePeer = (rs.Flags & 0x00020000) != 0
		line.Authorized = (rs.Flags & 0x00080000) != 0
		line.PeerAuthorized = (rs.Flags & 0x00010000) != 0
		line.Freeze = (rs.Flags & 0x00800000) != 0
		line.FreezePeer = (rs.Flags & 0x00400000) != 0
	}

	line.QualityIn = rs.LowQualityIn
	line.QualityOut = rs.LowQualityOut

	return line
}

// buildLedgerHeaderInfo creates header info from a ledger.
func buildLedgerHeaderInfo(hdr interface{}, l *ledger.Ledger) *LedgerHeaderInfo {
	// Implementation depends on header structure
	return &LedgerHeaderInfo{
		LedgerIndex: l.Sequence(),
		LedgerHash:  l.Hash(),
		Closed:      l.IsClosed() || l.IsValidated(),
	}
}
