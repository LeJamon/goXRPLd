package entry

import (
	"errors"
)

// Flags for different entry types
const (
	// AccountRoot flags
	AccountRootDefaultRipple uint32 = 0x00800000
	AccountRootDepositAuth   uint32 = 0x01000000
	AccountRootDisableMaster uint32 = 0x00100000
	AccountRootDisallowXRP   uint32 = 0x00080000
	AccountRootGlobalFreeze  uint32 = 0x00400000
	AccountRootNoFreeze      uint32 = 0x00200000
	AccountRootPasswordSpent uint32 = 0x00010000
	AccountRootRequireAuth   uint32 = 0x00040000
	AccountRootRequireDest   uint32 = 0x00020000

	// Offer flags
	OfferPassive uint32 = 0x00010000
	OfferSell    uint32 = 0x00020000
)

// Errors returned by entry operations
var (
	ErrInvalidEntry = errors.New("invalid entry")
	ErrInvalidFlags = errors.New("invalid flags")
	ErrInvalidHash  = errors.New("invalid hash")
)
