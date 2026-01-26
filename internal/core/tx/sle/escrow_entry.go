package sle

import (
	"encoding/binary"
	"encoding/hex"
)

// EscrowData represents an Escrow ledger entry
type EscrowData struct {
	Account       [20]byte
	DestinationID [20]byte
	Amount        uint64
	Condition     string
	CancelAfter   uint32
	FinishAfter   uint32
}

// ParseEscrow parses an Escrow ledger entry from binary data
func ParseEscrow(data []byte) (*EscrowData, error) {
	escrow := &EscrowData{}
	offset := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		header := data[offset]
		offset++

		typeCode := (header >> 4) & 0x0F
		fieldCode := header & 0x0F

		if typeCode == 0 {
			if offset >= len(data) {
				break
			}
			typeCode = data[offset]
			offset++
		}

		if fieldCode == 0 {
			if offset >= len(data) {
				break
			}
			fieldCode = data[offset]
			offset++
		}

		switch typeCode {
		case FieldTypeUInt16:
			if offset+2 > len(data) {
				return escrow, nil
			}
			offset += 2

		case FieldTypeUInt32:
			if offset+4 > len(data) {
				return escrow, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case 36: // FinishAfter
				escrow.FinishAfter = value
			case 37: // CancelAfter
				escrow.CancelAfter = value
			}

		case FieldTypeUInt64:
			if offset+8 > len(data) {
				return escrow, nil
			}
			offset += 8

		case FieldTypeAmount:
			if offset+8 > len(data) {
				return escrow, nil
			}
			rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
			escrow.Amount = rawAmount & 0x3FFFFFFFFFFFFFFF
			offset += 8

		case FieldTypeAccountID:
			if offset+21 > len(data) {
				return escrow, nil
			}
			length := data[offset]
			offset++
			if length == 20 {
				switch fieldCode {
				case 1: // Account
					copy(escrow.Account[:], data[offset:offset+20])
				case 3: // Destination
					copy(escrow.DestinationID[:], data[offset:offset+20])
				}
				offset += 20
			}

		case FieldTypeHash256:
			if offset+32 > len(data) {
				return escrow, nil
			}
			offset += 32

		case FieldTypeBlob:
			if offset >= len(data) {
				return escrow, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return escrow, nil
			}
			if fieldCode == 25 { // Condition
				escrow.Condition = hex.EncodeToString(data[offset : offset+length])
			}
			offset += length

		default:
			return escrow, nil
		}
	}

	return escrow, nil
}
