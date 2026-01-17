package tx

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
)

// AccountRoot represents an account in the ledger
type AccountRoot struct {
	Account           string
	Balance           uint64
	Sequence          uint32
	OwnerCount        uint32
	Flags             uint32
	RegularKey        string
	Domain            string
	EmailHash         string
	MessageKey        string
	TransferRate      uint32
	TickSize          uint8
	NFTokenMinter     string   // Account allowed to mint NFTokens on behalf of this account
	AccountTxnID      [32]byte // Hash of the last transaction this account submitted (when enabled)
	WalletLocator     string   // Arbitrary hex data (deprecated)
	PreviousTxnID     [32]byte
	PreviousTxnLgrSeq uint32
}

// AccountRoot binary format field codes (from XRPL spec)
const (
	// Field type codes
	fieldTypeUInt16    = 1
	fieldTypeUInt32    = 2
	fieldTypeUInt64    = 3
	fieldTypeHash128   = 4
	fieldTypeHash256   = 5
	fieldTypeAmount    = 6
	fieldTypeBlob      = 7
	fieldTypeAccount   = 8
	fieldTypeAccountID = 8 // Same as Account, used in serialization

	// Field codes for AccountRoot
	fieldCodeLedgerEntryType = 1  // UInt16
	fieldCodeFlags           = 2  // UInt32
	fieldCodeSequence        = 4  // UInt32
	fieldCodeOwnerCount      = 13 // UInt32 (per rippled sfields.macro)
	fieldCodeTransferRate    = 11 // UInt32
	fieldCodeBalance         = 1  // Amount
	fieldCodeRegularKey      = 8  // Account
	fieldCodeAccount         = 1  // Account (different context)
	fieldCodeNFTokenMinter   = 9  // Account - authorized NFT minter
	fieldCodeEmailHash       = 1  // Hash128
	fieldCodeDomain          = 7  // Blob
	fieldCodeTickSize        = 16 // UInt8 (stored as UInt16)
	fieldCodeAccountTxnID    = 9  // Hash256 - last transaction ID
	fieldCodeWalletLocator   = 7  // Hash256 - wallet locator (deprecated)

	// Ledger entry type code for AccountRoot
	ledgerEntryTypeAccountRoot = 0x0061
)

// AccountRoot ledger entry flags (lsf = Ledger State Flag)
const (
	// lsfPasswordSpent indicates the account has spent the free transaction
	lsfPasswordSpent uint32 = 0x00010000

	// lsfRequireDestTag indicates the account requires a destination tag
	lsfRequireDestTag uint32 = 0x00020000

	// lsfRequireAuth indicates the account requires authorization for trust lines
	lsfRequireAuth uint32 = 0x00040000

	// lsfDisallowXRP indicates the account does not want to receive XRP
	// Per rippled, this flag means non-direct XRP payments are rejected
	lsfDisallowXRP uint32 = 0x00080000

	// lsfDisableMaster indicates the master key is disabled
	lsfDisableMaster uint32 = 0x00100000

	// lsfNoFreeze indicates the account has permanently given up the ability to freeze trust lines
	// Once set, cannot be cleared
	lsfNoFreeze uint32 = 0x00200000

	// lsfGlobalFreeze indicates this account has globally frozen all trust lines
	lsfGlobalFreeze uint32 = 0x00400000

	// lsfDefaultRipple indicates rippling is enabled by default on trust lines
	lsfDefaultRipple uint32 = 0x00800000

	// lsfDepositAuth indicates the account requires deposit authorization
	// Payments to this account require preauthorization unless both the
	// destination balance and payment amount are at or below the base reserve
	lsfDepositAuth uint32 = 0x01000000

	// lsfAMM indicates the account is an AMM (pseudo-account)
	// AMM accounts cannot receive direct payments
	lsfAMM uint32 = 0x02000000

	// lsfDisallowIncomingNFTokenOffer disallows incoming NFToken offers
	lsfDisallowIncomingNFTokenOffer uint32 = 0x04000000

	// lsfDisallowIncomingCheck disallows incoming checks
	lsfDisallowIncomingCheck uint32 = 0x08000000

	// lsfDisallowIncomingPayChan disallows incoming payment channels
	lsfDisallowIncomingPayChan uint32 = 0x10000000

	// lsfDisallowIncomingTrustline disallows incoming trust lines
	lsfDisallowIncomingTrustline uint32 = 0x20000000

	// lsfAllowTrustLineClawback allows clawback on issued currencies
	// Once set, this flag CANNOT be cleared
	lsfAllowTrustLineClawback uint32 = 0x80000000
)

// decodeAccountID decodes an XRPL address to a 20-byte account ID
func decodeAccountID(address string) ([20]byte, error) {
	var accountID [20]byte
	if address == "" {
		return accountID, errors.New("empty address")
	}

	// Use the address codec to decode
	_, accountIDBytes, err := addresscodec.DecodeClassicAddressToAccountID(address)
	if err != nil {
		return accountID, errors.New("invalid address: " + err.Error())
	}

	copy(accountID[:], accountIDBytes)
	return accountID, nil
}

// encodeAccountID encodes a 20-byte account ID to an XRPL address
func encodeAccountID(accountID [20]byte) (string, error) {
	return addresscodec.EncodeAccountIDToClassicAddress(accountID[:])
}

// ParseAccountRootFromBytes parses account data from binary format
func ParseAccountRootFromBytes(data []byte) (*AccountRoot, error) {
	return parseAccountRoot(data)
}

func parseAccountRoot(data []byte) (*AccountRoot, error) {
	if len(data) < 20 {
		return nil, errors.New("account data too short")
	}

	account := &AccountRoot{}
	ledgerEntryTypeVerified := false // Track if we've verified the LedgerEntryType

	// Parse the binary format
	// XRPL uses a TLV-like format with field headers
	offset := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		// Read field header
		header := data[offset]
		offset++

		// Decode type and field from header
		typeCode := (header >> 4) & 0x0F
		fieldCode := header & 0x0F

		// Handle extended type codes
		if typeCode == 0 {
			if offset >= len(data) {
				break
			}
			typeCode = data[offset]
			offset++
		}

		// Handle extended field codes
		if fieldCode == 0 {
			if offset >= len(data) {
				break
			}
			fieldCode = data[offset]
			offset++
		}

		// Parse field based on type
		switch typeCode {
		case fieldTypeUInt16:
			if offset+2 > len(data) {
				return account, nil
			}
			value := binary.BigEndian.Uint16(data[offset : offset+2])
			offset += 2
			// Only check LedgerEntryType once (first UInt16 with fieldCode 1)
			if fieldCode == fieldCodeLedgerEntryType && !ledgerEntryTypeVerified {
				// LedgerEntryType - verify it's AccountRoot
				if value != ledgerEntryTypeAccountRoot {
					return nil, errors.New("not an AccountRoot entry")
				}
				ledgerEntryTypeVerified = true
			}

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return account, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case fieldCodeFlags:
				account.Flags = value
			case fieldCodeSequence:
				account.Sequence = value
			case 5: // PreviousTxnLgrSeq
				account.PreviousTxnLgrSeq = value
			case fieldCodeOwnerCount:
				account.OwnerCount = value
			case fieldCodeTransferRate:
				account.TransferRate = value
			}

		case fieldTypeAmount:
			// XRP amounts are 8 bytes, IOU amounts are 48 bytes
			if offset+8 > len(data) {
				return account, nil
			}
			// Check if it's XRP (first bit is 0) or IOU (first bit is 1)
			if data[offset]&0x80 == 0 {
				// XRP amount - 8 bytes
				// The format is: top bit = 0 for XRP, next bit = positive, remaining 62 bits = drops
				rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
				// Clear the top bit and extract drops
				account.Balance = rawAmount & 0x3FFFFFFFFFFFFFFF
				offset += 8
			} else {
				// IOU amount - skip 48 bytes (we don't handle this in AccountRoot)
				offset += 48
			}

		case fieldTypeAccount:
			// Account IDs are variable length encoded
			if offset >= len(data) {
				return account, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return account, nil
			}
			if length == 20 {
				var accID [20]byte
				copy(accID[:], data[offset:offset+length])
				addr, err := encodeAccountID(accID)
				if err == nil {
					if fieldCode == fieldCodeAccount || fieldCode == 1 {
						account.Account = addr
					} else if fieldCode == fieldCodeRegularKey {
						account.RegularKey = addr
					} else if fieldCode == fieldCodeNFTokenMinter {
						account.NFTokenMinter = addr
					}
				}
			}
			offset += length

		case fieldTypeBlob:
			// Variable length blob
			if offset >= len(data) {
				return account, nil
			}
			length := int(data[offset])
			offset++
			if length > 192 {
				// Extended length encoding
				if length < 241 {
					length = 193 + int(data[offset-1]) - 193
				} else {
					// Even more extended - skip for now
					offset += 2
					continue
				}
			}
			if offset+length > len(data) {
				return account, nil
			}
			if fieldCode == 7 { // Domain field
				account.Domain = string(data[offset : offset+length])
			}
			offset += length

		case fieldTypeHash128:
			if offset+16 > len(data) {
				return account, nil
			}
			if fieldCode == fieldCodeEmailHash {
				account.EmailHash = hex.EncodeToString(data[offset : offset+16])
			}
			offset += 16

		case fieldTypeHash256:
			// Hash256 fields (e.g., PreviousTxnID, AccountTxnID, WalletLocator) are 32 bytes
			if offset+32 > len(data) {
				return account, nil
			}
			switch fieldCode {
			case 5: // PreviousTxnID
				copy(account.PreviousTxnID[:], data[offset:offset+32])
			case fieldCodeAccountTxnID: // AccountTxnID
				copy(account.AccountTxnID[:], data[offset:offset+32])
			case fieldCodeWalletLocator: // WalletLocator
				account.WalletLocator = hex.EncodeToString(data[offset : offset+32])
			}
			offset += 32

		default:
			// Unknown type - can't determine size, must stop parsing
			// This shouldn't happen for valid AccountRoot entries
			return account, nil
		}
	}

	return account, nil
}

func serializeAccountRoot(account *AccountRoot) ([]byte, error) {
	// Build the JSON representation for the binary codec
	jsonObj := map[string]any{
		"LedgerEntryType": "AccountRoot",
		"Balance":         fmt.Sprintf("%d", account.Balance), // XRP balance as drops string
		"Sequence":        account.Sequence,
		"OwnerCount":      account.OwnerCount,
		"Flags":           account.Flags,
	}

	// Add Account if set
	if account.Account != "" {
		jsonObj["Account"] = account.Account
	}

	// Add TransferRate if set
	if account.TransferRate > 0 {
		jsonObj["TransferRate"] = account.TransferRate
	}

	// Add RegularKey if set
	if account.RegularKey != "" {
		jsonObj["RegularKey"] = account.RegularKey
	}

	// Add Domain if set (as hex string)
	if account.Domain != "" {
		jsonObj["Domain"] = strings.ToUpper(hex.EncodeToString([]byte(account.Domain)))
	}

	// Add EmailHash if set
	if account.EmailHash != "" {
		jsonObj["EmailHash"] = strings.ToUpper(account.EmailHash)
	}

	// Add NFTokenMinter if set
	if account.NFTokenMinter != "" {
		jsonObj["NFTokenMinter"] = account.NFTokenMinter
	}

	// Add AccountTxnID if set (non-zero)
	var zeroHash [32]byte
	if account.AccountTxnID != zeroHash {
		jsonObj["AccountTxnID"] = strings.ToUpper(hex.EncodeToString(account.AccountTxnID[:]))
	}

	// Add WalletLocator if set
	if account.WalletLocator != "" {
		jsonObj["WalletLocator"] = strings.ToUpper(account.WalletLocator)
	}

	// Add PreviousTxnID if set (non-zero)
	if account.PreviousTxnID != zeroHash {
		jsonObj["PreviousTxnID"] = strings.ToUpper(hex.EncodeToString(account.PreviousTxnID[:]))
	}

	// Add PreviousTxnLgrSeq if set
	if account.PreviousTxnLgrSeq > 0 {
		jsonObj["PreviousTxnLgrSeq"] = account.PreviousTxnLgrSeq
	}

	// Encode using the binary codec
	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode AccountRoot: %w", err)
	}

	// Convert hex string to bytes
	return hex.DecodeString(hexStr)
}
