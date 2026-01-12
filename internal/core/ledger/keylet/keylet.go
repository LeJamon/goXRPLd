package keylet

import (
	"encoding/binary"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/entry"
	crypto "github.com/LeJamon/goXRPLd/internal/crypto/common"
)

// Space identifiers for keylet generation
// These correspond to the LedgerNameSpace enum in rippled
const (
	spaceAccount    uint16 = 'a' // Account root
	spaceDirNode    uint16 = 'd' // Directory node
	spaceGenerator  uint16 = 'g' // Generator map (deprecated)
	spaceRippleDir  uint16 = 'r' // Trust line directory
	spaceOffer      uint16 = 'o' // Offer
	spaceOwnerDir   uint16 = 'O' // Owner directory
	spaceBookDir    uint16 = 'B' // Order book directory
	spaceSkip       uint16 = 's' // Skip list
	spaceEscrow     uint16 = 'u' // Escrow
	spaceAmendments uint16 = 'f' // Amendments (singleton)
	spaceFees       uint16 = 'e' // Fee settings (singleton)
	spaceTicket     uint16 = 'T' // Ticket
	spaceSignerList uint16 = 'S' // Signer list
	spaceCheck      uint16 = 'C' // Check
	spaceDepPreauth uint16 = 'p' // Deposit preauthorization
	spaceNFTokenOff uint16 = 'q' // NFToken offer
	spaceNFTokenPg  uint16 = 'P' // NFToken page
	spaceAMM        uint16 = 'A' // AMM
	spaceBridge     uint16 = 'i' // XChain bridge
	spaceXCClaimID  uint16 = 'Q' // XChain claim ID
	spaceXCCreateAc uint16 = 'K' // XChain create account claim
	spaceDID        uint16 = 'I' // DID
	spaceOracle     uint16 = 'R' // Oracle
	spaceMPTIssu    uint16 = '~' // MPToken issuance
	spaceMPToken    uint16 = 't' // MPToken
	spaceCredential uint16 = 'D' // Credential
	spacePermDomain uint16 = 'b' // Permissioned domain
	spaceVault      uint16 = 'V' // Vault
)

// Keylet represents an addressable location in the ledger state.
// It combines a type identifier with a 256-bit key.
type Keylet struct {
	Type entry.Type
	Key  [32]byte
}

// indexHash computes a keylet key by hashing the space and provided data.
func indexHash(space uint16, data ...[]byte) [32]byte {
	// Prepend the space identifier as a 2-byte big-endian value
	spaceBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(spaceBytes, space)

	// Collect all inputs for hashing
	inputs := make([][]byte, 0, len(data)+1)
	inputs = append(inputs, spaceBytes)
	inputs = append(inputs, data...)

	return crypto.Sha512Half(inputs...)
}

// Account returns the keylet for an account root entry.
func Account(accountID [20]byte) Keylet {
	return Keylet{
		Type: entry.TypeAccountRoot,
		Key:  indexHash(spaceAccount, accountID[:]),
	}
}

// Fees returns the keylet for the singleton fee settings entry.
func Fees() Keylet {
	// Singleton - no additional data needed
	return Keylet{
		Type: entry.TypeFeeSettings,
		Key:  indexHash(spaceFees),
	}
}

// Amendments returns the keylet for the singleton amendments entry.
func Amendments() Keylet {
	// Singleton - no additional data needed
	return Keylet{
		Type: entry.TypeAmendments,
		Key:  indexHash(spaceAmendments),
	}
}

// LedgerHashes returns the keylet for the skip list / ledger hashes entry.
func LedgerHashes() Keylet {
	return Keylet{
		Type: entry.TypeLedgerHashes,
		Key:  indexHash(spaceSkip),
	}
}

// Offer returns the keylet for an offer entry.
func Offer(accountID [20]byte, sequence uint32) Keylet {
	seqBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(seqBytes, sequence)
	return Keylet{
		Type: entry.TypeOffer,
		Key:  indexHash(spaceOffer, accountID[:], seqBytes),
	}
}

// OwnerDir returns the keylet for an owner directory entry.
func OwnerDir(accountID [20]byte) Keylet {
	return Keylet{
		Type: entry.TypeDirectoryNode,
		Key:  indexHash(spaceOwnerDir, accountID[:]),
	}
}

// OwnerDirPage returns the keylet for a specific page of an owner directory.
func OwnerDirPage(accountID [20]byte, page uint64) Keylet {
	rootKey := OwnerDir(accountID).Key
	if page == 0 {
		return Keylet{
			Type: entry.TypeDirectoryNode,
			Key:  rootKey,
		}
	}
	pageBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(pageBytes, page)
	return Keylet{
		Type: entry.TypeDirectoryNode,
		Key:  indexHash(spaceDirNode, rootKey[:], pageBytes),
	}
}

// Escrow returns the keylet for an escrow entry.
func Escrow(accountID [20]byte, sequence uint32) Keylet {
	seqBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(seqBytes, sequence)
	return Keylet{
		Type: entry.TypeEscrow,
		Key:  indexHash(spaceEscrow, accountID[:], seqBytes),
	}
}

// Check returns the keylet for a check entry.
func Check(accountID [20]byte, sequence uint32) Keylet {
	seqBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(seqBytes, sequence)
	return Keylet{
		Type: entry.TypeCheck,
		Key:  indexHash(spaceCheck, accountID[:], seqBytes),
	}
}

// SignerList returns the keylet for a signer list entry.
func SignerList(accountID [20]byte) Keylet {
	// Signer list uses owner page 0 as identifier
	ownerPageBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(ownerPageBytes, 0)
	return Keylet{
		Type: entry.TypeSignerList,
		Key:  indexHash(spaceSignerList, accountID[:], ownerPageBytes),
	}
}

// Ticket returns the keylet for a ticket entry.
func Ticket(accountID [20]byte, ticketSeq uint32) Keylet {
	seqBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(seqBytes, ticketSeq)
	return Keylet{
		Type: entry.TypeTicket,
		Key:  indexHash(spaceTicket, accountID[:], seqBytes),
	}
}

// DepositPreauth returns the keylet for a deposit preauthorization entry.
func DepositPreauth(owner, authorized [20]byte) Keylet {
	return Keylet{
		Type: entry.TypeDepositPreauth,
		Key:  indexHash(spaceDepPreauth, owner[:], authorized[:]),
	}
}

// Line returns the keylet for a trust line (RippleState) between two accounts.
// The currency is a 3-character code for standard currencies or a 40-character hex string.
func Line(account1, account2 [20]byte, currency string) Keylet {
	// Accounts must be sorted consistently - lower account first
	var low, high [20]byte
	if compareAccountIDs(account1, account2) < 0 {
		low, high = account1, account2
	} else {
		low, high = account2, account1
	}

	// Convert currency to 160-bit (20 byte) representation
	currencyBytes := currencyToBytes(currency)

	return Keylet{
		Type: entry.TypeRippleState,
		Key:  indexHash(spaceRippleDir, low[:], high[:], currencyBytes[:]),
	}
}

// compareAccountIDs compares two account IDs lexicographically.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func compareAccountIDs(a, b [20]byte) int {
	for i := 0; i < 20; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// currencyToBytes converts a currency code to its 20-byte representation.
// Standard 3-character codes are zero-padded (e.g., "USD" -> 0x0000000000000000005553440000000000000000)
// Hex strings are decoded directly.
func currencyToBytes(currency string) [20]byte {
	var result [20]byte

	if len(currency) == 3 {
		// Standard currency code - ASCII in bytes 12-14
		result[12] = currency[0]
		result[13] = currency[1]
		result[14] = currency[2]
	} else if len(currency) == 40 {
		// Hex-encoded currency (non-standard)
		for i := 0; i < 20; i++ {
			result[i] = hexToByte(currency[i*2], currency[i*2+1])
		}
	}

	return result
}

// hexToByte converts two hex characters to a byte.
func hexToByte(high, low byte) byte {
	return hexNibble(high)<<4 | hexNibble(low)
}

// hexNibble converts a single hex character to its value.
func hexNibble(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 0
	}
}

// BookDir returns the keylet for an order book directory.
func BookDir(takerPaysCurrency, takerPaysIssuer, takerGetsCurrency, takerGetsIssuer [20]byte) Keylet {
	return Keylet{
		Type: entry.TypeDirectoryNode,
		Key:  indexHash(spaceBookDir, takerPaysCurrency[:], takerPaysIssuer[:], takerGetsCurrency[:], takerGetsIssuer[:]),
	}
}

// NFTokenPage returns the keylet for an NFToken page.
func NFTokenPage(accountID [20]byte, tokenID [32]byte) Keylet {
	// The page key uses the high 192 bits of the token ID
	pageKey := make([]byte, 32)
	copy(pageKey[:20], accountID[:])
	copy(pageKey[20:], tokenID[20:32])
	return Keylet{
		Type: entry.TypeNFTokenPage,
		Key:  indexHash(spaceNFTokenPg, pageKey),
	}
}

// NFTokenOffer returns the keylet for an NFToken offer.
func NFTokenOffer(accountID [20]byte, sequence uint32) Keylet {
	seqBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(seqBytes, sequence)
	return Keylet{
		Type: entry.TypeNFTokenOffer,
		Key:  indexHash(spaceNFTokenOff, accountID[:], seqBytes),
	}
}

// PayChannel returns the keylet for a payment channel.
func PayChannel(srcAccountID, dstAccountID [20]byte, sequence uint32) Keylet {
	seqBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(seqBytes, sequence)
	return Keylet{
		Type: entry.TypePayChannel,
		Key:  indexHash(spaceEscrow, srcAccountID[:], dstAccountID[:], seqBytes),
	}
}
