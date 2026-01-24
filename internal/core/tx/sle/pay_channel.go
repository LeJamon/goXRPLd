package sle

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
)

// PayChannelData represents a PayChannel ledger entry
type PayChannelData struct {
	Account        [20]byte
	DestinationID  [20]byte
	Amount         uint64
	Balance        uint64
	SettleDelay    uint32
	PublicKey      string
	Expiration     uint32
	CancelAfter    uint32
	SourceTag      uint32
	DestinationTag uint32
	HasSourceTag   bool
	HasDestTag     bool
}

// SerializePayChannelFromData serializes a PayChannel ledger entry from data
func SerializePayChannelFromData(channel *PayChannelData) ([]byte, error) {
	ownerAddress, err := addresscodec.EncodeAccountIDToClassicAddress(channel.Account[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode owner address: %w", err)
	}

	destAddress, err := addresscodec.EncodeAccountIDToClassicAddress(channel.DestinationID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode destination address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "PayChannel",
		"Account":         ownerAddress,
		"Destination":     destAddress,
		"Amount":          fmt.Sprintf("%d", channel.Amount),
		"Balance":         fmt.Sprintf("%d", channel.Balance),
		"SettleDelay":     channel.SettleDelay,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode PayChannel: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// ParsePayChannel parses a PayChannel ledger entry from binary data
func ParsePayChannel(data []byte) (*PayChannelData, error) {
	channel := &PayChannelData{}
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
				return channel, nil
			}
			offset += 2

		case FieldTypeUInt32:
			if offset+4 > len(data) {
				return channel, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case 39: // SettleDelay
				channel.SettleDelay = value
			case 37: // CancelAfter
				channel.CancelAfter = value
			case 10: // Expiration
				channel.Expiration = value
			case 3: // SourceTag
				channel.SourceTag = value
				channel.HasSourceTag = true
			case 14: // DestinationTag
				channel.DestinationTag = value
				channel.HasDestTag = true
			}

		case FieldTypeUInt64:
			if offset+8 > len(data) {
				return channel, nil
			}
			offset += 8

		case FieldTypeAmount:
			if offset+8 > len(data) {
				return channel, nil
			}
			rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
			amount := rawAmount & 0x3FFFFFFFFFFFFFFF
			if fieldCode == 1 { // Amount
				channel.Amount = amount
			} else if fieldCode == 5 { // Balance
				channel.Balance = amount
			}
			offset += 8

		case FieldTypeAccountID:
			if offset+21 > len(data) {
				return channel, nil
			}
			length := data[offset]
			offset++
			if length == 20 {
				switch fieldCode {
				case 1: // Account
					copy(channel.Account[:], data[offset:offset+20])
				case 3: // Destination
					copy(channel.DestinationID[:], data[offset:offset+20])
				}
				offset += 20
			}

		case FieldTypeHash256:
			if offset+32 > len(data) {
				return channel, nil
			}
			offset += 32

		case FieldTypeBlob:
			if offset >= len(data) {
				return channel, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return channel, nil
			}
			if fieldCode == 28 { // PublicKey
				channel.PublicKey = hex.EncodeToString(data[offset : offset+length])
			}
			offset += length

		default:
			return channel, nil
		}
	}

	return channel, nil
}

// ParsePayChannelFromBytes is an alias for ParsePayChannel
func ParsePayChannelFromBytes(data []byte) (*PayChannelData, error) {
	return ParsePayChannel(data)
}
