package tx

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// Field metadata flags matching rippled's SField metadata flags
const (
	// sMD_Never - never include in metadata
	sMD_Never uint8 = 0x00
	// sMD_ChangeOrig - include original value when field changes (goes to PreviousFields)
	sMD_ChangeOrig uint8 = 0x01
	// sMD_ChangeNew - include new value when field changes (goes to FinalFields)
	sMD_ChangeNew uint8 = 0x02
	// sMD_DeleteFinal - include final value when node is deleted
	sMD_DeleteFinal uint8 = 0x04
	// sMD_Create - include value when node is created (goes to NewFields)
	sMD_Create uint8 = 0x08
	// sMD_Always - always include if node is affected
	sMD_Always uint8 = 0x10
	// sMD_Default - default flags for most fields
	sMD_Default uint8 = sMD_ChangeOrig | sMD_ChangeNew | sMD_DeleteFinal | sMD_Create
)

// fieldMetadata maps field names to their metadata flags
// Based on rippled's sfields.macro definitions
var fieldMetadata = map[string]uint8{
	// Fields with sMD_Never - never appear in metadata
	"LedgerEntryType": sMD_Never,
	"Indexes":         sMD_Never,

	// Fields with sMD_Always - always appear if node is affected
	"RootIndex": sMD_Always,

	// Fields with sMD_DeleteFinal only - only in FinalFields when deleted
	"PreviousTxnLgrSeq": sMD_DeleteFinal,
	"PreviousTxnID":     sMD_DeleteFinal,

	// Most fields use sMD_Default
	// These are listed explicitly for clarity but would default anyway
	"Account":       sMD_Default,
	"Balance":       sMD_Default,
	"Flags":         sMD_Default,
	"Sequence":      sMD_Default,
	"OwnerCount":    sMD_Default,
	"TakerGets":     sMD_Default,
	"TakerPays":     sMD_Default,
	"LowLimit":      sMD_Default,
	"HighLimit":     sMD_Default,
	"LowNode":       sMD_Default,
	"HighNode":      sMD_Default,
	"BookDirectory": sMD_Default,
	"BookNode":      sMD_Default,
	"OwnerNode":     sMD_Default,
	"Expiration":    sMD_Default,
	"TransferRate":  sMD_Default,
	"Domain":        sMD_Default,
	"EmailHash":     sMD_Default,
	"MessageKey":    sMD_Default,
	"RegularKey":    sMD_Default,
	"TickSize":      sMD_Default,
}

// getFieldMetadata returns the metadata flags for a field
func getFieldMetadata(fieldName string) uint8 {
	if flags, ok := fieldMetadata[fieldName]; ok {
		return flags
	}
	return sMD_Default
}

// shouldIncludeInPreviousFields returns true if a changed field should be in PreviousFields
func shouldIncludeInPreviousFields(fieldName string) bool {
	flags := getFieldMetadata(fieldName)
	return (flags & sMD_ChangeOrig) != 0
}

// shouldIncludeInFinalFields returns true if a field should be in FinalFields
func shouldIncludeInFinalFields(fieldName string) bool {
	flags := getFieldMetadata(fieldName)
	return (flags & (sMD_Always | sMD_ChangeNew)) != 0
}

// shouldIncludeInCreate returns true if a field should be in NewFields for created nodes
func shouldIncludeInCreate(fieldName string) bool {
	flags := getFieldMetadata(fieldName)
	return (flags & (sMD_Create | sMD_Always)) != 0
}

// shouldIncludeInDeleteFinal returns true if a field should be in FinalFields for deleted nodes
func shouldIncludeInDeleteFinal(fieldName string) bool {
	flags := getFieldMetadata(fieldName)
	return (flags & (sMD_Always | sMD_DeleteFinal)) != 0
}

// extractLedgerFields extracts fields from binary ledger entry data
func extractLedgerFields(data []byte, entryType string) (map[string]any, error) {
	fields := make(map[string]any)

	switch entryType {
	case "AccountRoot":
		return extractAccountRootFields(data)
	case "RippleState":
		return extractRippleStateFields(data)
	case "Offer":
		return extractOfferFields(data)
	case "DirectoryNode":
		return extractDirectoryNodeFields(data)
	default:
		// Generic extraction for unknown types
		return fields, nil
	}
}

// extractAccountRootFields extracts fields from AccountRoot binary data
func extractAccountRootFields(data []byte) (map[string]any, error) {
	fields := make(map[string]any)

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
		case 1: // UInt16
			if offset+2 > len(data) {
				return fields, nil
			}
			value := binary.BigEndian.Uint16(data[offset : offset+2])
			offset += 2
			// LedgerEntryType = field 1
			if fieldCode == 1 {
				fields["LedgerEntryType"] = ledgerEntryTypeName(value)
			}

		case 2: // UInt32
			if offset+4 > len(data) {
				return fields, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case 2:
				fields["Flags"] = value
			case 4:
				fields["Sequence"] = value
			case 5:
				fields["PreviousTxnLgrSeq"] = value
			case 11:
				fields["TransferRate"] = value
			case 13:
				fields["OwnerCount"] = value
			}

		case 3: // UInt64
			if offset+8 > len(data) {
				return fields, nil
			}
			value := binary.BigEndian.Uint64(data[offset : offset+8])
			offset += 8
			switch fieldCode {
			case 4:
				fields["OwnerNode"] = fmt.Sprintf("%016X", value)
			}

		case 5: // Hash256
			if offset+32 > len(data) {
				return fields, nil
			}
			value := strings.ToUpper(hex.EncodeToString(data[offset : offset+32]))
			offset += 32
			switch fieldCode {
			case 5:
				fields["PreviousTxnID"] = value
			case 9:
				fields["AccountTxnID"] = value
			}

		case 6: // Amount
			if offset >= len(data) {
				return fields, nil
			}
			if (data[offset] & 0x80) == 0 {
				// XRP amount (native)
				if offset+8 > len(data) {
					return fields, nil
				}
				rawValue := binary.BigEndian.Uint64(data[offset : offset+8])
				drops := rawValue & 0x3FFFFFFFFFFFFFFF // Clear the top bit
				offset += 8
				switch fieldCode {
				case 2:
					fields["Balance"] = fmt.Sprintf("%d", drops)
				}
			} else {
				// IOU amount - skip for AccountRoot
				offset += 48
			}

		case 7: // Blob (variable length)
			if offset >= len(data) {
				return fields, nil
			}
			length := int(data[offset])
			offset++
			if length > 192 {
				if offset >= len(data) {
					return fields, nil
				}
				length = 193 + ((length-193)<<8 | int(data[offset]))
				offset++
			}
			if offset+length > len(data) {
				return fields, nil
			}
			value := hex.EncodeToString(data[offset : offset+length])
			offset += length
			switch fieldCode {
			case 7:
				fields["Domain"] = value
			}

		case 8: // AccountID
			if offset+20 > len(data) {
				return fields, nil
			}
			var accountID [20]byte
			copy(accountID[:], data[offset:offset+20])
			account, _ := encodeAccountID(accountID)
			offset += 20
			switch fieldCode {
			case 1:
				fields["Account"] = account
			case 8:
				fields["RegularKey"] = account
			}

		default:
			// Unknown type, break
			return fields, nil
		}
	}

	return fields, nil
}

// extractRippleStateFields extracts fields from RippleState binary data
func extractRippleStateFields(data []byte) (map[string]any, error) {
	fields := make(map[string]any)

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
		case 1: // UInt16
			if offset+2 > len(data) {
				return fields, nil
			}
			// LedgerEntryType
			offset += 2

		case 2: // UInt32
			if offset+4 > len(data) {
				return fields, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case 2:
				fields["Flags"] = value
			case 5:
				fields["PreviousTxnLgrSeq"] = value
			case 16:
				fields["HighQualityIn"] = value
			case 17:
				fields["HighQualityOut"] = value
			case 18:
				fields["LowQualityIn"] = value
			case 19:
				fields["LowQualityOut"] = value
			}

		case 3: // UInt64
			if offset+8 > len(data) {
				return fields, nil
			}
			value := binary.BigEndian.Uint64(data[offset : offset+8])
			offset += 8
			switch fieldCode {
			case 7:
				fields["LowNode"] = fmt.Sprintf("%016X", value)
			case 8:
				fields["HighNode"] = fmt.Sprintf("%016X", value)
			}

		case 5: // Hash256
			if offset+32 > len(data) {
				return fields, nil
			}
			value := strings.ToUpper(hex.EncodeToString(data[offset : offset+32]))
			offset += 32
			switch fieldCode {
			case 5:
				fields["PreviousTxnID"] = value
			}

		case 6: // Amount (IOU)
			if offset+48 > len(data) {
				return fields, nil
			}
			iou, err := parseIOUAmountForMetadata(data[offset : offset+48])
			if err == nil {
				switch fieldCode {
				case 2:
					fields["Balance"] = iou
				case 6:
					fields["LowLimit"] = iou
				case 7:
					fields["HighLimit"] = iou
				}
			}
			offset += 48

		default:
			return fields, nil
		}
	}

	return fields, nil
}

// extractOfferFields extracts fields from Offer binary data
func extractOfferFields(data []byte) (map[string]any, error) {
	fields := make(map[string]any)

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
		case 1: // UInt16
			if offset+2 > len(data) {
				return fields, nil
			}
			offset += 2

		case 2: // UInt32
			if offset+4 > len(data) {
				return fields, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case 2:
				fields["Flags"] = value
			case 4:
				fields["Sequence"] = value
			case 5:
				fields["PreviousTxnLgrSeq"] = value
			case 10:
				fields["Expiration"] = value
			}

		case 3: // UInt64
			if offset+8 > len(data) {
				return fields, nil
			}
			value := binary.BigEndian.Uint64(data[offset : offset+8])
			offset += 8
			switch fieldCode {
			case 3:
				fields["BookNode"] = fmt.Sprintf("%016X", value)
			case 4:
				fields["OwnerNode"] = fmt.Sprintf("%016X", value)
			}

		case 5: // Hash256
			if offset+32 > len(data) {
				return fields, nil
			}
			value := strings.ToUpper(hex.EncodeToString(data[offset : offset+32]))
			offset += 32
			switch fieldCode {
			case 5:
				fields["PreviousTxnID"] = value
			case 16:
				fields["BookDirectory"] = value
			}

		case 6: // Amount
			if offset >= len(data) {
				return fields, nil
			}
			if (data[offset] & 0x80) == 0 {
				// XRP amount
				if offset+8 > len(data) {
					return fields, nil
				}
				rawValue := binary.BigEndian.Uint64(data[offset : offset+8])
				drops := rawValue & 0x3FFFFFFFFFFFFFFF
				offset += 8
				switch fieldCode {
				case 4:
					fields["TakerPays"] = fmt.Sprintf("%d", drops)
				case 5:
					fields["TakerGets"] = fmt.Sprintf("%d", drops)
				}
			} else {
				// IOU amount
				if offset+48 > len(data) {
					return fields, nil
				}
				iou, err := parseIOUAmountForMetadata(data[offset : offset+48])
				if err == nil {
					switch fieldCode {
					case 4:
						fields["TakerPays"] = iou
					case 5:
						fields["TakerGets"] = iou
					}
				}
				offset += 48
			}

		case 8: // AccountID
			if offset+20 > len(data) {
				return fields, nil
			}
			var accountID [20]byte
			copy(accountID[:], data[offset:offset+20])
			account, _ := encodeAccountID(accountID)
			offset += 20
			switch fieldCode {
			case 1:
				fields["Account"] = account
			}

		default:
			return fields, nil
		}
	}

	return fields, nil
}

// extractDirectoryNodeFields extracts fields from DirectoryNode binary data
func extractDirectoryNodeFields(data []byte) (map[string]any, error) {
	fields := make(map[string]any)
	// Directory nodes have complex structure, basic extraction for now
	return fields, nil
}

// parseIOUAmountForMetadata parses IOU amount and returns it in metadata format
func parseIOUAmountForMetadata(data []byte) (map[string]any, error) {
	if len(data) != 48 {
		return nil, fmt.Errorf("invalid IOU amount length")
	}

	// Parse currency (bytes 8-27)
	currency := ""
	isStandardCode := true
	for i := 8; i < 20; i++ {
		if data[i] != 0 {
			isStandardCode = false
			break
		}
	}
	if isStandardCode {
		currency = string(data[20:23])
	} else {
		currency = strings.ToUpper(hex.EncodeToString(data[8:28]))
	}

	// Parse issuer (bytes 28-47)
	var issuerID [20]byte
	copy(issuerID[:], data[28:48])
	issuer, _ := encodeAccountID(issuerID)

	// Parse value (first 8 bytes)
	rawValue := binary.BigEndian.Uint64(data[0:8])

	if rawValue == 0x8000000000000000 { // Zero
		return map[string]any{
			"value":    "0",
			"currency": currency,
			"issuer":   issuer,
		}, nil
	}

	positive := (rawValue & 0x4000000000000000) != 0
	exponent := int((rawValue>>54)&0xFF) - 97
	mantissa := rawValue & 0x003FFFFFFFFFFFFF

	// Convert to big.Float for precision
	mantissaBigInt := new(big.Int).SetUint64(mantissa)
	value := new(big.Float).SetPrec(128).SetInt(mantissaBigInt)

	// Apply exponent
	if exponent > 0 {
		multiplier := new(big.Float).SetPrec(128).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil))
		value.Mul(value, multiplier)
	} else if exponent < 0 {
		divisor := new(big.Float).SetPrec(128).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exponent)), nil))
		value.Quo(value, divisor)
	}

	if !positive {
		value.Neg(value)
	}

	// Format value
	valueStr := value.Text('g', 16)
	if strings.Contains(valueStr, ".") {
		valueStr = strings.TrimRight(valueStr, "0")
		valueStr = strings.TrimRight(valueStr, ".")
	}

	return map[string]any{
		"value":    valueStr,
		"currency": currency,
		"issuer":   issuer,
	}, nil
}
