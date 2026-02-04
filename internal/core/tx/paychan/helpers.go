package paychan

import (
	"encoding/hex"
	"fmt"
	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
)

// serializePayChannel serializes a PayChannel ledger entry from a transaction
func serializePayChannel(pcTx *PaymentChannelCreate, ownerID, destID [20]byte, amount uint64) ([]byte, error) {
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
		"SettleDelay":     pcTx.SettleDelay,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	if pcTx.CancelAfter != nil {
		jsonObj["CancelAfter"] = *pcTx.CancelAfter
	}

	if pcTx.PublicKey != "" {
		jsonObj["PublicKey"] = pcTx.PublicKey
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode PayChannel: %w", err)
	}

	return hex.DecodeString(hexStr)
}
