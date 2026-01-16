package tx

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// PayChannelData represents a PayChannel ledger entry
type PayChannelData struct {
	Account       [20]byte
	DestinationID [20]byte
	Amount        uint64
	Balance       uint64
	SettleDelay   uint32
	PublicKey     string
	Expiration    uint32
	CancelAfter   uint32
}

// applyPaymentChannelCreate applies a PaymentChannelCreate transaction
func (e *Engine) applyPaymentChannelCreate(tx *PaymentChannelCreate, account *AccountRoot, metadata *Metadata) Result {
	// Parse the amount
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err != nil {
		return TemINVALID
	}

	// Check balance
	if account.Balance < amount {
		return TecUNFUNDED
	}

	// Verify destination exists
	destID, err := decodeAccountID(tx.Destination)
	if err != nil {
		return TemINVALID
	}

	destKey := keylet.Account(destID)
	exists, _ := e.view.Exists(destKey)
	if !exists {
		return TecNO_DST
	}

	// Deduct amount from account
	account.Balance -= amount

	// Create pay channel
	accountID, _ := decodeAccountID(tx.Account)
	sequence := *tx.GetCommon().Sequence

	channelKey := keylet.PayChannel(accountID, destID, sequence)

	// Serialize pay channel
	channelData, err := serializePayChannel(tx, accountID, destID, amount)
	if err != nil {
		return TefINTERNAL
	}

	// Insert channel
	if err := e.view.Insert(channelKey, channelData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	account.OwnerCount++

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "PayChannel",
		LedgerIndex:     hex.EncodeToString(channelKey.Key[:]),
		NewFields: map[string]any{
			"Account":     tx.Account,
			"Destination": tx.Destination,
			"Amount":      tx.Amount.Value,
			"Balance":     "0",
			"SettleDelay": tx.SettleDelay,
			"PublicKey":   tx.PublicKey,
		},
	})

	return TesSUCCESS
}

// applyPaymentChannelFund applies a PaymentChannelFund transaction
func (e *Engine) applyPaymentChannelFund(tx *PaymentChannelFund, account *AccountRoot, metadata *Metadata) Result {
	// Parse channel ID
	channelID, err := hex.DecodeString(tx.Channel)
	if err != nil || len(channelID) != 32 {
		return TemINVALID
	}

	var channelKeyBytes [32]byte
	copy(channelKeyBytes[:], channelID)
	channelKey := keylet.Keylet{Key: channelKeyBytes}

	// Read channel
	channelData, err := e.view.Read(channelKey)
	if err != nil {
		return TecNO_TARGET
	}

	// Parse channel
	channel, err := parsePayChannel(channelData)
	if err != nil {
		return TefINTERNAL
	}

	// Verify sender is the channel owner
	accountID, _ := decodeAccountID(tx.Account)
	if channel.Account != accountID {
		return TecNO_PERMISSION
	}

	// Parse amount to add
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err != nil {
		return TemINVALID
	}

	// Check balance
	if account.Balance < amount {
		return TecUNFUNDED
	}

	// Deduct from account
	account.Balance -= amount

	// Add to channel
	channel.Amount += amount

	// Update expiration if specified
	if tx.Expiration != nil {
		channel.Expiration = *tx.Expiration
	}

	// Serialize updated channel
	updatedChannelData, err := serializePayChannelFromData(channel)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(channelKey, updatedChannelData); err != nil {
		return TefINTERNAL
	}

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "PayChannel",
		LedgerIndex:     hex.EncodeToString(channelKey.Key[:]),
		FinalFields: map[string]any{
			"Amount": strconv.FormatUint(channel.Amount, 10),
		},
	})

	return TesSUCCESS
}

// applyPaymentChannelClaim applies a PaymentChannelClaim transaction
func (e *Engine) applyPaymentChannelClaim(tx *PaymentChannelClaim, account *AccountRoot, metadata *Metadata) Result {
	// Parse channel ID
	channelID, err := hex.DecodeString(tx.Channel)
	if err != nil || len(channelID) != 32 {
		return TemINVALID
	}

	var channelKeyBytes [32]byte
	copy(channelKeyBytes[:], channelID)
	channelKey := keylet.Keylet{Key: channelKeyBytes}

	// Read channel
	channelData, err := e.view.Read(channelKey)
	if err != nil {
		return TecNO_TARGET
	}

	// Parse channel
	channel, err := parsePayChannel(channelData)
	if err != nil {
		return TefINTERNAL
	}

	accountID, _ := decodeAccountID(tx.Account)
	isOwner := channel.Account == accountID
	isDest := channel.DestinationID == accountID

	if !isOwner && !isDest {
		return TecNO_PERMISSION
	}

	// Handle claim with signature
	if tx.Balance != nil && tx.Amount != nil && tx.Signature != "" {
		// Parse claimed balance
		claimBalance, err := strconv.ParseUint(tx.Balance.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}

		// Verify claim is valid (would verify signature in full implementation)
		if claimBalance > channel.Amount {
			return TecUNFUNDED_PAYMENT
		}

		if claimBalance < channel.Balance {
			return TemINVALID // Can't decrease balance
		}

		// Calculate amount to transfer
		transferAmount := claimBalance - channel.Balance

		// Transfer to destination
		destKey := keylet.Account(channel.DestinationID)
		destData, err := e.view.Read(destKey)
		if err != nil {
			return TecNO_DST
		}

		destAccount, err := parseAccountRoot(destData)
		if err != nil {
			return TefINTERNAL
		}

		destAccount.Balance += transferAmount
		channel.Balance = claimBalance

		destUpdatedData, err := serializeAccountRoot(destAccount)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Update(destKey, destUpdatedData); err != nil {
			return TefINTERNAL
		}

		destAddr, _ := encodeAccountID(channel.DestinationID)
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "AccountRoot",
			LedgerIndex:     hex.EncodeToString(destKey.Key[:]),
			FinalFields: map[string]any{
				"Account": destAddr,
				"Balance": strconv.FormatUint(destAccount.Balance, 10),
			},
		})
	}

	// Handle close flag
	flags := tx.GetFlags()
	if flags&PaymentChannelClaimFlagClose != 0 {
		// Close the channel

		// Return remaining funds to owner
		remaining := channel.Amount - channel.Balance
		if remaining > 0 {
			ownerKey := keylet.Account(channel.Account)
			ownerData, err := e.view.Read(ownerKey)
			if err == nil {
				ownerAccount, err := parseAccountRoot(ownerData)
				if err == nil {
					ownerAccount.Balance += remaining
					if ownerAccount.OwnerCount > 0 {
						ownerAccount.OwnerCount--
					}
					ownerUpdatedData, _ := serializeAccountRoot(ownerAccount)
					e.view.Update(ownerKey, ownerUpdatedData)
				}
			}
		}

		// Delete channel
		if err := e.view.Erase(channelKey); err != nil {
			return TefINTERNAL
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "PayChannel",
			LedgerIndex:     hex.EncodeToString(channelKey.Key[:]),
		})
	} else {
		// Update channel
		updatedChannelData, err := serializePayChannelFromData(channel)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Update(channelKey, updatedChannelData); err != nil {
			return TefINTERNAL
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "PayChannel",
			LedgerIndex:     hex.EncodeToString(channelKey.Key[:]),
			FinalFields: map[string]any{
				"Balance": strconv.FormatUint(channel.Balance, 10),
			},
		})
	}

	return TesSUCCESS
}

// serializePayChannel serializes a PayChannel ledger entry from a transaction
func serializePayChannel(tx *PaymentChannelCreate, ownerID, destID [20]byte, amount uint64) ([]byte, error) {
	ownerAddress, err := addresscodec.EncodeAccountIDToClassicAddress(ownerID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode owner address: %w", err)
	}

	destAddress, err := addresscodec.EncodeAccountIDToClassicAddress(destID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode destination address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "PayChannel",
		"Account":         ownerAddress,
		"Destination":     destAddress,
		"Amount":          fmt.Sprintf("%d", amount),
		"Balance":         "0",
		"SettleDelay":     tx.SettleDelay,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	if tx.CancelAfter != nil {
		jsonObj["CancelAfter"] = *tx.CancelAfter
	}

	if tx.PublicKey != "" {
		jsonObj["PublicKey"] = tx.PublicKey
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode PayChannel: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// serializePayChannelFromData serializes a PayChannel ledger entry from data
func serializePayChannelFromData(channel *PayChannelData) ([]byte, error) {
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

// parsePayChannel parses a PayChannel ledger entry from binary data
func parsePayChannel(data []byte) (*PayChannelData, error) {
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
		case fieldTypeUInt16:
			if offset+2 > len(data) {
				return channel, nil
			}
			offset += 2

		case fieldTypeUInt32:
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
			}

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return channel, nil
			}
			offset += 8

		case fieldTypeAmount:
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

		case fieldTypeAccountID:
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

		case fieldTypeHash256:
			// Hash256 fields are 32 bytes (e.g., PreviousTxnID)
			if offset+32 > len(data) {
				return channel, nil
			}
			offset += 32

		case fieldTypeBlob:
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
