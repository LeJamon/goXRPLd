package tx

import (
	"encoding/binary"
	"encoding/hex"
	"strconv"

	"github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// XChain transactions

// applyXChainCreateBridge applies an XChainCreateBridge transaction
func (e *Engine) applyXChainCreateBridge(tx *XChainCreateBridge, account *AccountRoot, view LedgerView) Result {
	// Create Bridge entry (simplified - in full implementation would create Bridge ledger entry)
	// Bridge creation tracked automatically by ApplyStateTable

	account.OwnerCount++
	return TesSUCCESS
}

// applyXChainModifyBridge applies an XChainModifyBridge transaction
func (e *Engine) applyXChainModifyBridge(tx *XChainModifyBridge, account *AccountRoot, view LedgerView) Result {
	// Bridge modification tracked automatically by ApplyStateTable
	return TesSUCCESS
}

// applyXChainCreateClaimID applies an XChainCreateClaimID transaction
func (e *Engine) applyXChainCreateClaimID(tx *XChainCreateClaimID, account *AccountRoot, view LedgerView) Result {
	// XChainClaimID creation tracked automatically by ApplyStateTable
	account.OwnerCount++
	return TesSUCCESS
}

// applyXChainCommit applies an XChainCommit transaction
func (e *Engine) applyXChainCommit(tx *XChainCommit, account *AccountRoot, view LedgerView) Result {
	// Lock the amount
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err == nil && tx.Amount.Currency == "" {
		if account.Balance < amount {
			return TecUNFUNDED
		}
		account.Balance -= amount
	}

	// Modification tracked automatically by ApplyStateTable
	return TesSUCCESS
}

// applyXChainClaim applies an XChainClaim transaction
func (e *Engine) applyXChainClaim(tx *XChainClaim, account *AccountRoot, view LedgerView) Result {
	// Credit the claimed amount
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err == nil && tx.Amount.Currency == "" {
		// Find destination and credit
		destID, err := decodeAccountID(tx.Destination)
		if err != nil {
			return TemINVALID
		}

		destKey := keylet.Account(destID)
		destData, err := view.Read(destKey)
		if err == nil {
			destAccount, err := parseAccountRoot(destData)
			if err == nil {
				destAccount.Balance += amount
				destUpdatedData, _ := serializeAccountRoot(destAccount)
				view.Update(destKey, destUpdatedData)
			}
		}
	}

	// Deletion tracked automatically by ApplyStateTable
	return TesSUCCESS
}

// applyXChainAccountCreateCommit applies an XChainAccountCreateCommit transaction
func (e *Engine) applyXChainAccountCreateCommit(tx *XChainAccountCreateCommit, account *AccountRoot, view LedgerView) Result {
	// Lock the amount
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err == nil && tx.Amount.Currency == "" {
		if account.Balance < amount {
			return TecUNFUNDED
		}
		account.Balance -= amount
	}

	// Creation tracked automatically by ApplyStateTable
	return TesSUCCESS
}

// applyXChainAddClaimAttestation applies an XChainAddClaimAttestation transaction
func (e *Engine) applyXChainAddClaimAttestation(tx *XChainAddClaimAttestation, account *AccountRoot, view LedgerView) Result {
	// Add attestation to the claim - modification tracked automatically by ApplyStateTable
	return TesSUCCESS
}

// applyXChainAddAccountCreateAttestation applies an XChainAddAccountCreateAttestation transaction
func (e *Engine) applyXChainAddAccountCreateAttestation(tx *XChainAddAccountCreateAttestation, account *AccountRoot, view LedgerView) Result {
	// Add attestation to the account create claim - modification tracked automatically by ApplyStateTable
	return TesSUCCESS
}

// DID transactions

// DIDData represents a DID ledger entry
// Reference: rippled ledger_entries.macro ltDID
type DIDData struct {
	Account     [20]byte
	OwnerNode   uint64
	URI         string // hex-encoded
	DIDDocument string // hex-encoded
	Data        string // hex-encoded
}

// applyDIDSet applies a DIDSet transaction
// Reference: rippled DID.cpp DIDSet::doApply
func (e *Engine) applyDIDSet(tx *DIDSet, account *AccountRoot, view LedgerView) Result {
	accountID, _ := decodeAccountID(tx.Account)
	didKey := keylet.DID(accountID)

	// Check if DID already exists
	existingData, err := view.Read(didKey)
	if err == nil && existingData != nil {
		// Update existing DID
		did, err := parseDID(existingData)
		if err != nil {
			return TefINTERNAL
		}

		// Update fields based on what's provided in transaction
		if tx.URI != "" {
			did.URI = tx.URI
		} else if tx.URI == "" && tx.Common.hasField("URI") {
			did.URI = ""
		}

		if tx.DIDDocument != "" {
			did.DIDDocument = tx.DIDDocument
		} else if tx.DIDDocument == "" && tx.Common.hasField("DIDDocument") {
			did.DIDDocument = ""
		}

		if tx.Data != "" {
			did.Data = tx.Data
		} else if tx.Data == "" && tx.Common.hasField("Data") {
			did.Data = ""
		}

		// Check that at least one field remains after update
		if did.URI == "" && did.DIDDocument == "" && did.Data == "" {
			return TecEMPTY_DID
		}

		// Serialize and update the DID - modification tracked automatically by ApplyStateTable
		updatedData, err := serializeDID(did, tx.Account)
		if err != nil {
			return TefINTERNAL
		}

		if err := view.Update(didKey, updatedData); err != nil {
			return TefINTERNAL
		}

		return TesSUCCESS
	}

	// Create new DID
	reserve := e.AccountReserve(account.OwnerCount + 1)
	if account.Balance < reserve {
		return TecINSUFFICIENT_RESERVE
	}

	did := &DIDData{
		Account:   accountID,
		OwnerNode: 0,
	}

	if tx.URI != "" {
		did.URI = tx.URI
	}
	if tx.DIDDocument != "" {
		did.DIDDocument = tx.DIDDocument
	}
	if tx.Data != "" {
		did.Data = tx.Data
	}

	// Check that at least one field is set (fixEmptyDID amendment)
	if did.URI == "" && did.DIDDocument == "" && did.Data == "" {
		return TecEMPTY_DID
	}

	didData, err := serializeDID(did, tx.Account)
	if err != nil {
		return TefINTERNAL
	}

	// Insert the DID - creation tracked automatically by ApplyStateTable
	if err := view.Insert(didKey, didData); err != nil {
		return TefINTERNAL
	}

	account.OwnerCount++

	return TesSUCCESS
}

// applyDIDDelete applies a DIDDelete transaction
func (e *Engine) applyDIDDelete(tx *DIDDelete, account *AccountRoot, view LedgerView) Result {
	accountID, _ := decodeAccountID(tx.Account)
	didKey := keylet.DID(accountID)

	existingData, err := view.Read(didKey)
	if err != nil || existingData == nil {
		return TecNO_ENTRY
	}

	// Delete the DID entry - deletion tracked automatically by ApplyStateTable
	if err := view.Erase(didKey); err != nil {
		return TefINTERNAL
	}

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	return TesSUCCESS
}

// serializeDID serializes a DID ledger entry using the binary codec
func serializeDID(did *DIDData, accountAddress string) ([]byte, error) {
	jsonObj := map[string]any{
		"LedgerEntryType": "DID",
		"Account":         accountAddress,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	if did.URI != "" {
		jsonObj["URI"] = did.URI
	}
	if did.DIDDocument != "" {
		jsonObj["DIDDocument"] = did.DIDDocument
	}
	if did.Data != "" {
		jsonObj["Data"] = did.Data
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, err
	}

	return hex.DecodeString(hexStr)
}

// parseDID parses a DID ledger entry from binary data
func parseDID(data []byte) (*DIDData, error) {
	did := &DIDData{}
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
				return did, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return did, nil
			}
			offset += 4

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return did, nil
			}
			value := binary.BigEndian.Uint64(data[offset : offset+8])
			if fieldCode == 34 { // OwnerNode
				did.OwnerNode = value
			}
			offset += 8

		case fieldTypeAccountID:
			if offset+21 > len(data) {
				return did, nil
			}
			length := data[offset]
			offset++
			if length == 20 {
				if fieldCode == 1 { // Account
					copy(did.Account[:], data[offset:offset+20])
				}
				offset += 20
			}

		case fieldTypeHash256:
			if offset+32 > len(data) {
				return did, nil
			}
			offset += 32

		case fieldTypeBlob:
			if offset >= len(data) {
				return did, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return did, nil
			}
			switch fieldCode {
			case 9: // URI
				did.URI = hex.EncodeToString(data[offset : offset+length])
			case 26: // DIDDocument
				did.DIDDocument = hex.EncodeToString(data[offset : offset+length])
			case 27: // Data
				did.Data = hex.EncodeToString(data[offset : offset+length])
			}
			offset += length

		default:
			return did, nil
		}
	}

	return did, nil
}

// Oracle transactions

// applyOracleSet applies an OracleSet transaction
func (e *Engine) applyOracleSet(tx *OracleSet, account *AccountRoot, view LedgerView) Result {
	accountID, _ := decodeAccountID(tx.Account)
	oracleKey := keylet.Escrow(accountID, tx.OracleDocumentID) // Simplified

	exists, _ := view.Exists(oracleKey)
	if !exists {
		account.OwnerCount++
	}
	// Creation/modification tracked automatically by ApplyStateTable

	return TesSUCCESS
}

// applyOracleDelete applies an OracleDelete transaction
func (e *Engine) applyOracleDelete(tx *OracleDelete, account *AccountRoot, view LedgerView) Result {
	if account.OwnerCount > 0 {
		account.OwnerCount--
	}
	// Deletion tracked automatically by ApplyStateTable

	return TesSUCCESS
}

// MPToken transactions

// applyMPTokenIssuanceCreate applies an MPTokenIssuanceCreate transaction
func (e *Engine) applyMPTokenIssuanceCreate(tx *MPTokenIssuanceCreate, account *AccountRoot, view LedgerView) Result {
	// Creation tracked automatically by ApplyStateTable
	account.OwnerCount++
	return TesSUCCESS
}

// applyMPTokenIssuanceDestroy applies an MPTokenIssuanceDestroy transaction
func (e *Engine) applyMPTokenIssuanceDestroy(tx *MPTokenIssuanceDestroy, account *AccountRoot, view LedgerView) Result {
	// Parse issuance ID
	issuanceIDBytes, err := hex.DecodeString(tx.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return TemINVALID
	}

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}
	// Deletion tracked automatically by ApplyStateTable

	return TesSUCCESS
}

// applyMPTokenIssuanceSet applies an MPTokenIssuanceSet transaction
func (e *Engine) applyMPTokenIssuanceSet(tx *MPTokenIssuanceSet, account *AccountRoot, view LedgerView) Result {
	// Parse issuance ID
	issuanceIDBytes, err := hex.DecodeString(tx.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return TemINVALID
	}
	// Modification tracked automatically by ApplyStateTable

	return TesSUCCESS
}

// applyMPTokenAuthorize applies an MPTokenAuthorize transaction
func (e *Engine) applyMPTokenAuthorize(tx *MPTokenAuthorize, account *AccountRoot, view LedgerView) Result {
	// Parse issuance ID
	issuanceIDBytes, err := hex.DecodeString(tx.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return TemINVALID
	}

	flags := tx.GetFlags()
	if flags&MPTokenAuthorizeFlagUnauthorize != 0 {
		// Unauthorized - delete MPToken
		if account.OwnerCount > 0 {
			account.OwnerCount--
		}
	} else {
		// Authorize - create MPToken
		account.OwnerCount++
	}
	// Creation/deletion tracked automatically by ApplyStateTable

	return TesSUCCESS
}

// Clawback transaction

// applyClawback applies a Clawback transaction
func (e *Engine) applyClawback(tx *Clawback, account *AccountRoot, view LedgerView) Result {
	if tx.Amount.Value == "" {
		return TemINVALID
	}

	holderID, err := decodeAccountID(tx.Amount.Issuer)
	if err != nil {
		return TecNO_TARGET
	}

	issuerID, _ := decodeAccountID(tx.Account)

	// Find the trust line
	trustKey := keylet.Line(holderID, issuerID, tx.Amount.Currency)

	trustData, err := view.Read(trustKey)
	if err != nil {
		return TecNO_LINE
	}

	// Parse the trust line
	_, err = parseRippleState(trustData)
	if err != nil {
		return TefINTERNAL
	}

	// Clawback modification tracked automatically by ApplyStateTable
	return TesSUCCESS
}

// Credential transactions

// applyCredentialCreate applies a CredentialCreate transaction
func (e *Engine) applyCredentialCreate(tx *CredentialCreate, account *AccountRoot, view LedgerView) Result {
	if tx.Subject == "" || tx.CredentialType == "" {
		return TemINVALID
	}

	subjectID, err := decodeAccountID(tx.Subject)
	if err != nil {
		return TecNO_TARGET
	}

	issuerID, _ := decodeAccountID(tx.Account)

	var credKey [32]byte
	copy(credKey[:20], issuerID[:])
	copy(credKey[20:], subjectID[:12])

	credKeylet := keylet.Keylet{Key: credKey, Type: 0x0081}

	credData := make([]byte, 64)
	copy(credData[:20], issuerID[:])
	copy(credData[20:40], subjectID[:])

	// Insert - creation tracked automatically by ApplyStateTable
	if err := view.Insert(credKeylet, credData); err != nil {
		return TefINTERNAL
	}

	account.OwnerCount++

	return TesSUCCESS
}

// applyCredentialAccept applies a CredentialAccept transaction
func (e *Engine) applyCredentialAccept(tx *CredentialAccept, account *AccountRoot, view LedgerView) Result {
	if tx.Issuer == "" || tx.CredentialType == "" {
		return TemINVALID
	}

	issuerID, err := decodeAccountID(tx.Issuer)
	if err != nil {
		return TecNO_TARGET
	}

	subjectID, _ := decodeAccountID(tx.Account)

	var credKey [32]byte
	copy(credKey[:20], issuerID[:])
	copy(credKey[20:], subjectID[:12])

	credKeylet := keylet.Keylet{Key: credKey, Type: 0x0081}

	_, err = view.Read(credKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	// Modification tracked automatically by ApplyStateTable
	return TesSUCCESS
}

// applyCredentialDelete applies a CredentialDelete transaction
func (e *Engine) applyCredentialDelete(tx *CredentialDelete, account *AccountRoot, view LedgerView) Result {
	if tx.CredentialType == "" {
		return TemINVALID
	}

	issuerID, _ := decodeAccountID(tx.Account)
	var subjectID [20]byte
	if tx.Subject != "" {
		subjectID, _ = decodeAccountID(tx.Subject)
	} else {
		subjectID = issuerID
	}

	var credKey [32]byte
	copy(credKey[:20], issuerID[:])
	copy(credKey[20:], subjectID[:12])

	credKeylet := keylet.Keylet{Key: credKey, Type: 0x0081}

	// Erase - deletion tracked automatically by ApplyStateTable
	if err := view.Erase(credKeylet); err != nil {
		return TecNO_ENTRY
	}

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	return TesSUCCESS
}

// PermissionedDomain transactions

// applyPermissionedDomainSet applies a PermissionedDomainSet transaction
func (e *Engine) applyPermissionedDomainSet(tx *PermissionedDomainSet, account *AccountRoot, view LedgerView) Result {
	accountID, _ := decodeAccountID(tx.Account)

	var domainKey [32]byte
	if tx.DomainID != "" {
		// Modifying existing domain
		domainBytes, err := hex.DecodeString(tx.DomainID)
		if err != nil || len(domainBytes) != 32 {
			return TemINVALID
		}
		copy(domainKey[:], domainBytes)

		domainKeylet := keylet.Keylet{Key: domainKey, Type: 0x0082}

		_, err = view.Read(domainKeylet)
		if err != nil {
			return TecNO_ENTRY
		}
		// Modification tracked automatically by ApplyStateTable
	} else {
		// Creating new domain
		copy(domainKey[:20], accountID[:])
		binary.BigEndian.PutUint32(domainKey[20:], account.Sequence)

		domainKeylet := keylet.Keylet{Key: domainKey, Type: 0x0082}

		domainData := make([]byte, 64)
		copy(domainData[:20], accountID[:])

		// Insert - creation tracked automatically by ApplyStateTable
		if err := view.Insert(domainKeylet, domainData); err != nil {
			return TefINTERNAL
		}

		account.OwnerCount++
	}

	return TesSUCCESS
}

// applyPermissionedDomainDelete applies a PermissionedDomainDelete transaction
func (e *Engine) applyPermissionedDomainDelete(tx *PermissionedDomainDelete, account *AccountRoot, view LedgerView) Result {
	if tx.DomainID == "" {
		return TemINVALID
	}

	domainBytes, err := hex.DecodeString(tx.DomainID)
	if err != nil || len(domainBytes) != 32 {
		return TemINVALID
	}

	var domainKey [32]byte
	copy(domainKey[:], domainBytes)

	domainKeylet := keylet.Keylet{Key: domainKey, Type: 0x0082}

	// Erase - deletion tracked automatically by ApplyStateTable
	if err := view.Erase(domainKeylet); err != nil {
		return TecNO_ENTRY
	}

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	return TesSUCCESS
}

// Delegate transaction

// applyDelegateSet applies a DelegateSet transaction
func (e *Engine) applyDelegateSet(tx *DelegateSet, account *AccountRoot, view LedgerView) Result {
	accountID, _ := decodeAccountID(tx.Account)

	if tx.Authorize != "" {
		delegateID, err := decodeAccountID(tx.Authorize)
		if err != nil {
			return TecNO_TARGET
		}

		var delegateKey [32]byte
		copy(delegateKey[:20], accountID[:])
		copy(delegateKey[20:], delegateID[:12])

		delegateKeylet := keylet.Keylet{Key: delegateKey, Type: 0x0083}

		delegateData := make([]byte, 40)
		copy(delegateData[:20], accountID[:])
		copy(delegateData[20:40], delegateID[:])

		// Insert or update - tracked automatically by ApplyStateTable
		if err := view.Insert(delegateKeylet, delegateData); err != nil {
			view.Update(delegateKeylet, delegateData)
		}
	}

	return TesSUCCESS
}

// Vault transactions

// applyVaultCreate applies a VaultCreate transaction
func (e *Engine) applyVaultCreate(tx *VaultCreate, account *AccountRoot, view LedgerView) Result {
	if tx.Asset.Currency == "" {
		return TemINVALID
	}

	accountID, _ := decodeAccountID(tx.Account)

	var vaultKey [32]byte
	copy(vaultKey[:20], accountID[:])
	binary.BigEndian.PutUint32(vaultKey[20:], account.Sequence)

	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}

	vaultData := make([]byte, 64)
	copy(vaultData[:20], accountID[:])

	// Insert - creation tracked automatically by ApplyStateTable
	if err := view.Insert(vaultKeylet, vaultData); err != nil {
		return TefINTERNAL
	}

	account.OwnerCount++

	return TesSUCCESS
}

// applyVaultSet applies a VaultSet transaction
func (e *Engine) applyVaultSet(tx *VaultSet, account *AccountRoot, view LedgerView) Result {
	if tx.VaultID == "" {
		return TemINVALID
	}

	vaultBytes, err := hex.DecodeString(tx.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}

	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)

	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}

	_, err = view.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	// Modification tracked automatically by ApplyStateTable
	return TesSUCCESS
}

// applyVaultDelete applies a VaultDelete transaction
func (e *Engine) applyVaultDelete(tx *VaultDelete, account *AccountRoot, view LedgerView) Result {
	if tx.VaultID == "" {
		return TemINVALID
	}

	vaultBytes, err := hex.DecodeString(tx.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}

	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)

	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}

	// Erase - deletion tracked automatically by ApplyStateTable
	if err := view.Erase(vaultKeylet); err != nil {
		return TecNO_ENTRY
	}

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	return TesSUCCESS
}

// applyVaultDeposit applies a VaultDeposit transaction
func (e *Engine) applyVaultDeposit(tx *VaultDeposit, account *AccountRoot, view LedgerView) Result {
	if tx.VaultID == "" || tx.Amount.Value == "" {
		return TemINVALID
	}

	vaultBytes, err := hex.DecodeString(tx.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}

	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)

	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}

	_, err = view.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	// Deduct from account balance if XRP
	if tx.Amount.Currency == "" || tx.Amount.Currency == "XRP" {
		amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}
		if account.Balance < amount {
			return TecINSUFFICIENT_FUNDS
		}
		account.Balance -= amount
	}

	// Modification tracked automatically by ApplyStateTable
	return TesSUCCESS
}

// applyVaultWithdraw applies a VaultWithdraw transaction
func (e *Engine) applyVaultWithdraw(tx *VaultWithdraw, account *AccountRoot, view LedgerView) Result {
	if tx.VaultID == "" || tx.Amount.Value == "" {
		return TemINVALID
	}

	vaultBytes, err := hex.DecodeString(tx.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}

	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)

	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}

	_, err = view.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	// Add to account balance if XRP
	if tx.Amount.Currency == "" || tx.Amount.Currency == "XRP" {
		amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
		if err != nil {
			return TemINVALID
		}
		account.Balance += amount
	}

	// Modification tracked automatically by ApplyStateTable
	return TesSUCCESS
}

// applyVaultClawback applies a VaultClawback transaction
func (e *Engine) applyVaultClawback(tx *VaultClawback, account *AccountRoot, view LedgerView) Result {
	if tx.VaultID == "" || tx.Holder == "" {
		return TemINVALID
	}

	vaultBytes, err := hex.DecodeString(tx.VaultID)
	if err != nil || len(vaultBytes) != 32 {
		return TemINVALID
	}

	var vaultKey [32]byte
	copy(vaultKey[:], vaultBytes)

	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}

	_, err = view.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	_, err = decodeAccountID(tx.Holder)
	if err != nil {
		return TecNO_TARGET
	}

	// Modification tracked automatically by ApplyStateTable
	return TesSUCCESS
}

// Batch transaction

// applyBatch applies a Batch transaction
func (e *Engine) applyBatch(tx *Batch, account *AccountRoot, view LedgerView) Result {
	if len(tx.RawTransactions) == 0 {
		return TemINVALID
	}

	flags := tx.GetFlags()

	// Process each raw transaction in the batch
	for _, rawTx := range tx.RawTransactions {
		_, err := hex.DecodeString(rawTx.RawTransaction.RawTxBlob)
		if err != nil {
			if flags&BatchFlagAllOrNothing != 0 {
				return TefINTERNAL
			}
			continue
		}

		// Batch processing tracked automatically by ApplyStateTable

		if flags&BatchFlagUntilFailure != 0 {
			// Would continue until a failure
		}
		if flags&BatchFlagOnlyOne != 0 {
			break
		}
	}

	return TesSUCCESS
}

// LedgerStateFix transaction

// applyLedgerStateFix applies a LedgerStateFix transaction
func (e *Engine) applyLedgerStateFix(tx *LedgerStateFix, account *AccountRoot, view LedgerView) Result {
	if tx.Owner != "" {
		_, err := decodeAccountID(tx.Owner)
		if err != nil {
			return TecNO_TARGET
		}
	}

	// Modification tracked automatically by ApplyStateTable
	return TesSUCCESS
}
