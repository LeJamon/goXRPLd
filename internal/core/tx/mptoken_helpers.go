package tx

import (
	"encoding/hex"
	"strings"

	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// MPToken ledger entry flag constants (lsf prefix in rippled)
const (
	// lsfMPTLocked indicates the token is locked (frozen)
	lsfMPTLocked uint32 = 0x00000001

	// lsfMPTAuthorized indicates the holder is authorized by the issuer
	lsfMPTAuthorized uint32 = 0x00000002
)

// MPTokenIssuanceEntry represents a parsed MPTokenIssuance ledger entry
type MPTokenIssuanceEntry struct {
	Issuer            [20]byte
	Sequence          uint32
	Flags             uint32
	OutstandingAmount uint64
	OwnerNode         uint64
	TransferFee       uint16
	AssetScale        uint8
	MaximumAmount     *uint64
	LockedAmount      *uint64
	MPTokenMetadata   string
}

// MPTokenEntry represents a parsed MPToken ledger entry
type MPTokenEntry struct {
	Account           [20]byte
	MPTokenIssuanceID [32]byte
	Flags             uint32
	MPTAmount         uint64
	OwnerNode         uint64
	LockedAmount      *uint64
}

// parseMPTokenIssuance parses binary data into an MPTokenIssuanceEntry
func parseMPTokenIssuance(data []byte) (*MPTokenIssuanceEntry, error) {
	hexStr := hex.EncodeToString(data)
	jsonObj, err := binarycodec.Decode(hexStr)
	if err != nil {
		return nil, err
	}

	entry := &MPTokenIssuanceEntry{}

	if issuer, ok := jsonObj["Issuer"].(string); ok {
		issuerID, err := decodeAccountID(issuer)
		if err == nil {
			entry.Issuer = issuerID
		}
	}

	if seq, ok := jsonObj["Sequence"].(float64); ok {
		entry.Sequence = uint32(seq)
	}

	if flags, ok := jsonObj["Flags"].(float64); ok {
		entry.Flags = uint32(flags)
	}

	if outstanding, ok := jsonObj["OutstandingAmount"].(float64); ok {
		entry.OutstandingAmount = uint64(outstanding)
	} else if outstanding, ok := jsonObj["OutstandingAmount"].(string); ok {
		entry.OutstandingAmount = parseUint64Hex(outstanding)
	}

	if ownerNode, ok := jsonObj["OwnerNode"].(string); ok {
		entry.OwnerNode = parseUint64Hex(ownerNode)
	}

	if transferFee, ok := jsonObj["TransferFee"].(float64); ok {
		entry.TransferFee = uint16(transferFee)
	}

	if assetScale, ok := jsonObj["AssetScale"].(float64); ok {
		entry.AssetScale = uint8(assetScale)
	}

	if maxAmount, ok := jsonObj["MaximumAmount"].(float64); ok {
		v := uint64(maxAmount)
		entry.MaximumAmount = &v
	} else if maxAmount, ok := jsonObj["MaximumAmount"].(string); ok {
		v := parseUint64Hex(maxAmount)
		entry.MaximumAmount = &v
	}

	if lockedAmount, ok := jsonObj["LockedAmount"].(float64); ok {
		v := uint64(lockedAmount)
		entry.LockedAmount = &v
	} else if lockedAmount, ok := jsonObj["LockedAmount"].(string); ok {
		v := parseUint64Hex(lockedAmount)
		entry.LockedAmount = &v
	}

	if metadata, ok := jsonObj["MPTokenMetadata"].(string); ok {
		entry.MPTokenMetadata = metadata
	}

	return entry, nil
}

// serializeMPTokenIssuance serializes fields map to binary data
func serializeMPTokenIssuance(fields map[string]any) []byte {
	jsonObj := map[string]any{
		"LedgerEntryType": "MPTokenIssuance",
	}

	// Copy all fields
	for k, v := range fields {
		jsonObj[k] = v
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil
	}

	data, _ := hex.DecodeString(hexStr)
	return data
}

// serializeMPTokenIssuanceEntry serializes an MPTokenIssuanceEntry to binary
func serializeMPTokenIssuanceEntry(entry *MPTokenIssuanceEntry) []byte {
	issuerAddr, _ := encodeAccountID(entry.Issuer)

	jsonObj := map[string]any{
		"LedgerEntryType":   "MPTokenIssuance",
		"Issuer":            issuerAddr,
		"Sequence":          entry.Sequence,
		"Flags":             entry.Flags,
		"OutstandingAmount": entry.OutstandingAmount,
		"OwnerNode":         formatUint64Hex(entry.OwnerNode),
	}

	if entry.TransferFee > 0 {
		jsonObj["TransferFee"] = entry.TransferFee
	}

	if entry.AssetScale > 0 {
		jsonObj["AssetScale"] = entry.AssetScale
	}

	if entry.MaximumAmount != nil {
		jsonObj["MaximumAmount"] = *entry.MaximumAmount
	}

	if entry.LockedAmount != nil && *entry.LockedAmount > 0 {
		jsonObj["LockedAmount"] = *entry.LockedAmount
	}

	if entry.MPTokenMetadata != "" {
		jsonObj["MPTokenMetadata"] = entry.MPTokenMetadata
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil
	}

	data, _ := hex.DecodeString(hexStr)
	return data
}

// parseMPToken parses binary data into an MPTokenEntry
func parseMPToken(data []byte) (*MPTokenEntry, error) {
	hexStr := hex.EncodeToString(data)
	jsonObj, err := binarycodec.Decode(hexStr)
	if err != nil {
		return nil, err
	}

	entry := &MPTokenEntry{}

	if account, ok := jsonObj["Account"].(string); ok {
		accountID, err := decodeAccountID(account)
		if err == nil {
			entry.Account = accountID
		}
	}

	if issuanceID, ok := jsonObj["MPTokenIssuanceID"].(string); ok {
		idBytes, _ := hex.DecodeString(issuanceID)
		copy(entry.MPTokenIssuanceID[:], idBytes)
	}

	if flags, ok := jsonObj["Flags"].(float64); ok {
		entry.Flags = uint32(flags)
	}

	if amount, ok := jsonObj["MPTAmount"].(float64); ok {
		entry.MPTAmount = uint64(amount)
	} else if amount, ok := jsonObj["MPTAmount"].(string); ok {
		entry.MPTAmount = parseUint64Hex(amount)
	}

	if ownerNode, ok := jsonObj["OwnerNode"].(string); ok {
		entry.OwnerNode = parseUint64Hex(ownerNode)
	}

	if lockedAmount, ok := jsonObj["LockedAmount"].(float64); ok {
		v := uint64(lockedAmount)
		entry.LockedAmount = &v
	} else if lockedAmount, ok := jsonObj["LockedAmount"].(string); ok {
		v := parseUint64Hex(lockedAmount)
		entry.LockedAmount = &v
	}

	return entry, nil
}

// serializeMPToken serializes fields map to binary data
func serializeMPToken(fields map[string]any) []byte {
	jsonObj := map[string]any{
		"LedgerEntryType": "MPToken",
	}

	// Copy all fields
	for k, v := range fields {
		jsonObj[k] = v
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil
	}

	data, _ := hex.DecodeString(hexStr)
	return data
}

// serializeMPTokenEntry serializes an MPTokenEntry to binary
func serializeMPTokenEntry(entry *MPTokenEntry) []byte {
	accountAddr, _ := encodeAccountID(entry.Account)

	jsonObj := map[string]any{
		"LedgerEntryType":   "MPToken",
		"Account":           accountAddr,
		"MPTokenIssuanceID": strings.ToUpper(hex.EncodeToString(entry.MPTokenIssuanceID[:])),
		"Flags":             entry.Flags,
		"OwnerNode":         formatUint64Hex(entry.OwnerNode),
	}

	if entry.MPTAmount > 0 {
		jsonObj["MPTAmount"] = entry.MPTAmount
	}

	if entry.LockedAmount != nil && *entry.LockedAmount > 0 {
		jsonObj["LockedAmount"] = *entry.LockedAmount
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil
	}

	data, _ := hex.DecodeString(hexStr)
	return data
}

// dirRemove removes an item from a directory
func (e *Engine) dirRemove(dirKey keylet.Keylet, ownerNode uint64, itemKey [32]byte) error {
	// Read the directory
	exists, err := e.view.Exists(dirKey)
	if err != nil || !exists {
		return err
	}

	data, err := e.view.Read(dirKey)
	if err != nil {
		return err
	}

	dir, err := parseDirectoryNode(data)
	if err != nil {
		return err
	}

	// Find and remove the item
	newIndexes := make([][32]byte, 0, len(dir.Indexes))
	for _, idx := range dir.Indexes {
		if idx != itemKey {
			newIndexes = append(newIndexes, idx)
		}
	}
	dir.Indexes = newIndexes

	// If directory is empty, delete it
	if len(dir.Indexes) == 0 {
		return e.view.Erase(dirKey)
	}

	// Otherwise update it
	isBookDir := dir.TakerPaysCurrency != [20]byte{} || dir.TakerGetsCurrency != [20]byte{}
	updatedData, err := serializeDirectoryNode(dir, isBookDir)
	if err != nil {
		return err
	}

	return e.view.Update(dirKey, updatedData)
}
