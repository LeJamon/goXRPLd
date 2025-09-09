package header

import (
	"time"
)

// Ledger close flags
const sLCFNoConsensusTime uint32 = 0x01

type LedgerHeader struct {
	LedgerIndex     uint
	parentCloseTime time.Time
	//
	// For closed ledgers
	//

	// Closed means "tx set already determined"
	hash        [32]byte
	txHash      [32]byte
	accountHash [32]byte
	parentHash  [32]byte
	drops       uint32 //TODO ADD XRPAMOUNT TYPE

	// If validated is false, it means "not yet validated."
	// Once validated is true, it will never be set false at a later time.
	validated bool
	accepted  bool
	// flags indicating how this ledger close took place
	closeFlags uint32

	// the resolution for this ledger close time (2-120 seconds)
	closeTimeResolution int32

	// For closed ledgers, the time the ledger
	// closed. For open ledgers, the time the ledger
	// will close if there's no transactions.
	//
	closeTime time.Time
}

// GetCloseAgree returns true if there was consensus on the close time
func (h *LedgerHeader) GetCloseAgree() bool {
	return (h.closeFlags & sLCFNoConsensusTime) == 0
}

// DeserializeHeader Deserialize a ledger header from a byte array. */
func DeserializeHeader(Slice []byte, hasHash bool) (*LedgerHeader, error) {
	return nil, nil
}

// DeserializePrefixedHeader Deserialize a ledger header (prefixed with 4 bytes) from a byte array. */
func DeserializePrefixedHeader(Slice []byte, hasHash bool) (*LedgerHeader, error) {
	return nil, nil
}
