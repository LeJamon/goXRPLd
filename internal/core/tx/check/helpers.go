package check

import (
	"encoding/hex"
	"fmt"
	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
)

// serializeCheck serializes a Check ledger entry
func serializeCheck(checkTx *CheckCreate, ownerID, destID [20]byte, sequence uint32, sendMax uint64) ([]byte, error) {
	ownerAddress, err := addresscodec.EncodeAccountIDToClassicAddress(ownerID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode owner address: %w", err)
	}

	destAddress, err := addresscodec.EncodeAccountIDToClassicAddress(destID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode destination address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "Check",
		"Account":         ownerAddress,
		"Destination":     destAddress,
		"SendMax":         fmt.Sprintf("%d", sendMax),
		"Sequence":        sequence,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	if checkTx.Expiration != nil {
		jsonObj["Expiration"] = *checkTx.Expiration
	}

	if checkTx.DestinationTag != nil {
		jsonObj["DestinationTag"] = *checkTx.DestinationTag
	}

	if checkTx.InvoiceID != "" {
		jsonObj["InvoiceID"] = checkTx.InvoiceID
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode Check: %w", err)
	}

	return hex.DecodeString(hexStr)
}
