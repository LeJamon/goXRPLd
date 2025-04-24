package entry

import (
	"fmt"
)

// Type represents a ledger entry type
type Type uint16

// All known ledger entry types
const (
	TypeAccountRoot    Type = 0x0061
	TypeAmendments     Type = 0x0066
	TypeCheck          Type = 0x0043
	TypeDepositPreauth Type = 0x0070
	TypeDirectoryNode  Type = 0x0064
	TypeEscrow         Type = 0x0075
	TypeFeeSettings    Type = 0x0073
	TypeLedgerHashes   Type = 0x0068
	TypeNFTokenOffer   Type = 0x0037
	TypeNFTokenPage    Type = 0x0050
	TypeOffer          Type = 0x006f
	TypePayChannel     Type = 0x0078
	TypeRippleState    Type = 0x0072
	TypeSignerList     Type = 0x0053
	TypeTicket         Type = 0x0054
)

// String returns the string representation of the Type
func (t Type) String() string {
	switch t {
	case TypeAccountRoot:
		return "AccountRoot"
	case TypeAmendments:
		return "Amendments"
	case TypeCheck:
		return "Check"
	case TypeDepositPreauth:
		return "DepositPreauth"
	case TypeDirectoryNode:
		return "DirectoryNode"
	case TypeEscrow:
		return "Escrow"
	case TypeFeeSettings:
		return "FeeSettings"
	case TypeLedgerHashes:
		return "LedgerHashes"
	case TypeNFTokenOffer:
		return "NFTokenOffer"
	case TypeNFTokenPage:
		return "NFTokenPage"
	case TypeOffer:
		return "Offer"
	case TypePayChannel:
		return "PayChannel"
	case TypeRippleState:
		return "RippleState"
	case TypeSignerList:
		return "SignerList"
	case TypeTicket:
		return "Ticket"
	default:
		return fmt.Sprintf("Unknown(%#x)", uint16(t))
	}
}

// Entry defines the interface for all ledger entries
type Entry interface {
	Type() Type
	Validate() error
	Hash() ([32]byte, error)
}
