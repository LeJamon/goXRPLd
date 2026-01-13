package tx

import (
	"encoding/binary"
	"encoding/hex"
	"strconv"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// XChain transactions

// applyXChainCreateBridge applies an XChainCreateBridge transaction
func (e *Engine) applyXChainCreateBridge(tx *XChainCreateBridge, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Create Bridge entry (simplified - in full implementation would create Bridge ledger entry)
	bridgeKey := keylet.Account(accountID) // Simplified - would use Bridge keylet

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "Bridge",
		LedgerIndex:     hex.EncodeToString(bridgeKey.Key[:]),
		NewFields: map[string]any{
			"Account":         tx.Account,
			"SignatureReward": tx.SignatureReward.Value,
		},
	})

	account.OwnerCount++
	return TesSUCCESS
}

// applyXChainModifyBridge applies an XChainModifyBridge transaction
func (e *Engine) applyXChainModifyBridge(tx *XChainModifyBridge, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	bridgeKey := keylet.Account(accountID)

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "Bridge",
		LedgerIndex:     hex.EncodeToString(bridgeKey.Key[:]),
	})

	return TesSUCCESS
}

// applyXChainCreateClaimID applies an XChainCreateClaimID transaction
func (e *Engine) applyXChainCreateClaimID(tx *XChainCreateClaimID, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	sequence := *tx.GetCommon().Sequence

	// Create XChainClaimID entry
	claimKey := keylet.Escrow(accountID, sequence) // Simplified

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "XChainOwnedClaimID",
		LedgerIndex:     hex.EncodeToString(claimKey.Key[:]),
		NewFields: map[string]any{
			"Account":          tx.Account,
			"OtherChainSource": tx.OtherChainSource,
		},
	})

	account.OwnerCount++
	return TesSUCCESS
}

// applyXChainCommit applies an XChainCommit transaction
func (e *Engine) applyXChainCommit(tx *XChainCommit, account *AccountRoot, metadata *Metadata) Result {
	// Lock the amount
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err == nil && tx.Amount.Currency == "" {
		if account.Balance < amount {
			return TecUNFUNDED
		}
		account.Balance -= amount
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "XChainOwnedClaimID",
	})

	return TesSUCCESS
}

// applyXChainClaim applies an XChainClaim transaction
func (e *Engine) applyXChainClaim(tx *XChainClaim, account *AccountRoot, metadata *Metadata) Result {
	// Credit the claimed amount
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err == nil && tx.Amount.Currency == "" {
		// Find destination and credit
		destID, err := decodeAccountID(tx.Destination)
		if err != nil {
			return TemINVALID
		}

		destKey := keylet.Account(destID)
		destData, err := e.view.Read(destKey)
		if err == nil {
			destAccount, err := parseAccountRoot(destData)
			if err == nil {
				destAccount.Balance += amount
				destUpdatedData, _ := serializeAccountRoot(destAccount)
				e.view.Update(destKey, destUpdatedData)
			}
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "XChainOwnedClaimID",
	})

	return TesSUCCESS
}

// applyXChainAccountCreateCommit applies an XChainAccountCreateCommit transaction
func (e *Engine) applyXChainAccountCreateCommit(tx *XChainAccountCreateCommit, account *AccountRoot, metadata *Metadata) Result {
	// Lock the amount
	amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err == nil && tx.Amount.Currency == "" {
		if account.Balance < amount {
			return TecUNFUNDED
		}
		account.Balance -= amount
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "XChainOwnedCreateAccountClaimID",
		NewFields: map[string]any{
			"Account":     tx.Account,
			"Destination": tx.Destination,
		},
	})

	return TesSUCCESS
}

// applyXChainAddClaimAttestation applies an XChainAddClaimAttestation transaction
func (e *Engine) applyXChainAddClaimAttestation(tx *XChainAddClaimAttestation, account *AccountRoot, metadata *Metadata) Result {
	// Add attestation to the claim
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "XChainOwnedClaimID",
	})

	return TesSUCCESS
}

// applyXChainAddAccountCreateAttestation applies an XChainAddAccountCreateAttestation transaction
func (e *Engine) applyXChainAddAccountCreateAttestation(tx *XChainAddAccountCreateAttestation, account *AccountRoot, metadata *Metadata) Result {
	// Add attestation to the account create claim
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "XChainOwnedCreateAccountClaimID",
	})

	return TesSUCCESS
}

// DID transactions

// applyDIDSet applies a DIDSet transaction
func (e *Engine) applyDIDSet(tx *DIDSet, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	didKey := keylet.Account(accountID) // Simplified - would use DID keylet

	exists, _ := e.view.Exists(didKey)
	if exists {
		// Update existing DID
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "DID",
			LedgerIndex:     hex.EncodeToString(didKey.Key[:]),
			FinalFields: map[string]any{
				"Account": tx.Account,
			},
		})
	} else {
		// Create new DID
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "DID",
			LedgerIndex:     hex.EncodeToString(didKey.Key[:]),
			NewFields: map[string]any{
				"Account": tx.Account,
			},
		})
		account.OwnerCount++
	}

	return TesSUCCESS
}

// applyDIDDelete applies a DIDDelete transaction
func (e *Engine) applyDIDDelete(tx *DIDDelete, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	didKey := keylet.Account(accountID)

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "DID",
		LedgerIndex:     hex.EncodeToString(didKey.Key[:]),
	})

	return TesSUCCESS
}

// Oracle transactions

// applyOracleSet applies an OracleSet transaction
func (e *Engine) applyOracleSet(tx *OracleSet, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	oracleKey := keylet.Escrow(accountID, tx.OracleDocumentID) // Simplified

	exists, _ := e.view.Exists(oracleKey)
	if exists {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "Oracle",
			LedgerIndex:     hex.EncodeToString(oracleKey.Key[:]),
			FinalFields: map[string]any{
				"LastUpdateTime": tx.LastUpdateTime,
			},
		})
	} else {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "Oracle",
			LedgerIndex:     hex.EncodeToString(oracleKey.Key[:]),
			NewFields: map[string]any{
				"Account":          tx.Account,
				"OracleDocumentID": tx.OracleDocumentID,
				"Provider":         tx.Provider,
			},
		})
		account.OwnerCount++
	}

	return TesSUCCESS
}

// applyOracleDelete applies an OracleDelete transaction
func (e *Engine) applyOracleDelete(tx *OracleDelete, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	oracleKey := keylet.Escrow(accountID, tx.OracleDocumentID)

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Oracle",
		LedgerIndex:     hex.EncodeToString(oracleKey.Key[:]),
	})

	return TesSUCCESS
}

// MPToken transactions

// applyMPTokenIssuanceCreate applies an MPTokenIssuanceCreate transaction
func (e *Engine) applyMPTokenIssuanceCreate(tx *MPTokenIssuanceCreate, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)
	sequence := *tx.GetCommon().Sequence
	issuanceKey := keylet.Escrow(accountID, sequence) // Simplified

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "MPTokenIssuance",
		LedgerIndex:     hex.EncodeToString(issuanceKey.Key[:]),
		NewFields: map[string]any{
			"Account":  tx.Account,
			"Sequence": sequence,
		},
	})

	account.OwnerCount++
	return TesSUCCESS
}

// applyMPTokenIssuanceDestroy applies an MPTokenIssuanceDestroy transaction
func (e *Engine) applyMPTokenIssuanceDestroy(tx *MPTokenIssuanceDestroy, account *AccountRoot, metadata *Metadata) Result {
	// Parse issuance ID
	issuanceIDBytes, err := hex.DecodeString(tx.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return TemINVALID
	}

	var issuanceKeyBytes [32]byte
	copy(issuanceKeyBytes[:], issuanceIDBytes)
	issuanceKey := keylet.Keylet{Key: issuanceKeyBytes}

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "MPTokenIssuance",
		LedgerIndex:     hex.EncodeToString(issuanceKey.Key[:]),
	})

	return TesSUCCESS
}

// applyMPTokenIssuanceSet applies an MPTokenIssuanceSet transaction
func (e *Engine) applyMPTokenIssuanceSet(tx *MPTokenIssuanceSet, account *AccountRoot, metadata *Metadata) Result {
	// Parse issuance ID
	issuanceIDBytes, err := hex.DecodeString(tx.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return TemINVALID
	}

	var issuanceKeyBytes [32]byte
	copy(issuanceKeyBytes[:], issuanceIDBytes)
	issuanceKey := keylet.Keylet{Key: issuanceKeyBytes}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "MPTokenIssuance",
		LedgerIndex:     hex.EncodeToString(issuanceKey.Key[:]),
	})

	return TesSUCCESS
}

// applyMPTokenAuthorize applies an MPTokenAuthorize transaction
func (e *Engine) applyMPTokenAuthorize(tx *MPTokenAuthorize, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Parse issuance ID
	issuanceIDBytes, err := hex.DecodeString(tx.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return TemINVALID
	}

	// Create or modify MPToken entry
	tokenKey := keylet.Account(accountID) // Simplified

	flags := tx.GetFlags()
	if flags&MPTokenAuthorizeFlagUnauthorize != 0 {
		// Unauthorized - delete MPToken
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "MPToken",
			LedgerIndex:     hex.EncodeToString(tokenKey.Key[:]),
		})
		if account.OwnerCount > 0 {
			account.OwnerCount--
		}
	} else {
		// Authorize - create or modify MPToken
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "MPToken",
			LedgerIndex:     hex.EncodeToString(tokenKey.Key[:]),
			NewFields: map[string]any{
				"Account":           tx.Account,
				"MPTokenIssuanceID": tx.MPTokenIssuanceID,
			},
		})
		account.OwnerCount++
	}

	return TesSUCCESS
}

// Clawback transaction

// applyClawback applies a Clawback transaction
func (e *Engine) applyClawback(tx *Clawback, account *AccountRoot, metadata *Metadata) Result {
	// Parse the amount to claw back
	if tx.Amount.Value == "" {
		return TemINVALID
	}

	// For clawback, we need to find the trust line and adjust the balance
	// The issuer is clawing back from a holder
	holderID, err := decodeAccountID(tx.Amount.Issuer)
	if err != nil {
		return TecNO_TARGET
	}

	issuerID, _ := decodeAccountID(tx.Account)

	// Find the trust line
	trustKey := keylet.Line(holderID, issuerID, tx.Amount.Currency)

	trustData, err := e.view.Read(trustKey)
	if err != nil {
		return TecNO_LINE
	}

	// Parse and modify the trust line
	rs, err := parseRippleState(trustData)
	if err != nil {
		return TefINTERNAL
	}

	// Record the clawback in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "RippleState",
		LedgerIndex:     hex.EncodeToString(trustKey.Key[:]),
		FinalFields: map[string]any{
			"Balance": rs.Balance,
		},
	})

	return TesSUCCESS
}

// Credential transactions

// applyCredentialCreate applies a CredentialCreate transaction
func (e *Engine) applyCredentialCreate(tx *CredentialCreate, account *AccountRoot, metadata *Metadata) Result {
	if tx.Subject == "" || tx.CredentialType == "" {
		return TemINVALID
	}

	subjectID, err := decodeAccountID(tx.Subject)
	if err != nil {
		return TecNO_TARGET
	}

	issuerID, _ := decodeAccountID(tx.Account)

	// Create the credential entry
	var credKey [32]byte
	copy(credKey[:20], issuerID[:])
	copy(credKey[20:], subjectID[:12])

	credKeylet := keylet.Keylet{Key: credKey, Type: 0x0081}

	// Serialize credential data
	credData := make([]byte, 64)
	copy(credData[:20], issuerID[:])
	copy(credData[20:40], subjectID[:])

	if err := e.view.Insert(credKeylet, credData); err != nil {
		return TefINTERNAL
	}

	account.OwnerCount++

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "Credential",
		LedgerIndex:     hex.EncodeToString(credKey[:]),
		NewFields: map[string]any{
			"Issuer":         tx.Account,
			"Subject":        tx.Subject,
			"CredentialType": tx.CredentialType,
		},
	})

	return TesSUCCESS
}

// applyCredentialAccept applies a CredentialAccept transaction
func (e *Engine) applyCredentialAccept(tx *CredentialAccept, account *AccountRoot, metadata *Metadata) Result {
	if tx.Issuer == "" || tx.CredentialType == "" {
		return TemINVALID
	}

	issuerID, err := decodeAccountID(tx.Issuer)
	if err != nil {
		return TecNO_TARGET
	}

	subjectID, _ := decodeAccountID(tx.Account)

	// Find and update the credential
	var credKey [32]byte
	copy(credKey[:20], issuerID[:])
	copy(credKey[20:], subjectID[:12])

	credKeylet := keylet.Keylet{Key: credKey, Type: 0x0081}

	_, err = e.view.Read(credKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "Credential",
		LedgerIndex:     hex.EncodeToString(credKey[:]),
		FinalFields: map[string]any{
			"Accepted": true,
		},
	})

	return TesSUCCESS
}

// applyCredentialDelete applies a CredentialDelete transaction
func (e *Engine) applyCredentialDelete(tx *CredentialDelete, account *AccountRoot, metadata *Metadata) Result {
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

	// Find and delete the credential
	var credKey [32]byte
	copy(credKey[:20], issuerID[:])
	copy(credKey[20:], subjectID[:12])

	credKeylet := keylet.Keylet{Key: credKey, Type: 0x0081}

	if err := e.view.Erase(credKeylet); err != nil {
		return TecNO_ENTRY
	}

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Credential",
		LedgerIndex:     hex.EncodeToString(credKey[:]),
	})

	return TesSUCCESS
}

// PermissionedDomain transactions

// applyPermissionedDomainSet applies a PermissionedDomainSet transaction
func (e *Engine) applyPermissionedDomainSet(tx *PermissionedDomainSet, account *AccountRoot, metadata *Metadata) Result {
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

		_, err = e.view.Read(domainKeylet)
		if err != nil {
			return TecNO_ENTRY
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "PermissionedDomain",
			LedgerIndex:     tx.DomainID,
		})
	} else {
		// Creating new domain
		copy(domainKey[:20], accountID[:])
		binary.BigEndian.PutUint32(domainKey[20:], account.Sequence)

		domainKeylet := keylet.Keylet{Key: domainKey, Type: 0x0082}

		domainData := make([]byte, 64)
		copy(domainData[:20], accountID[:])

		if err := e.view.Insert(domainKeylet, domainData); err != nil {
			return TefINTERNAL
		}

		account.OwnerCount++

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "PermissionedDomain",
			LedgerIndex:     hex.EncodeToString(domainKey[:]),
			NewFields: map[string]any{
				"Owner": tx.Account,
			},
		})
	}

	return TesSUCCESS
}

// applyPermissionedDomainDelete applies a PermissionedDomainDelete transaction
func (e *Engine) applyPermissionedDomainDelete(tx *PermissionedDomainDelete, account *AccountRoot, metadata *Metadata) Result {
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

	if err := e.view.Erase(domainKeylet); err != nil {
		return TecNO_ENTRY
	}

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "PermissionedDomain",
		LedgerIndex:     tx.DomainID,
	})

	return TesSUCCESS
}

// Delegate transaction

// applyDelegateSet applies a DelegateSet transaction
func (e *Engine) applyDelegateSet(tx *DelegateSet, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	if tx.Authorize != "" {
		// Setting delegation
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

		if err := e.view.Insert(delegateKeylet, delegateData); err != nil {
			// Try update if already exists
			e.view.Update(delegateKeylet, delegateData)
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "Delegate",
			LedgerIndex:     hex.EncodeToString(delegateKey[:]),
			NewFields: map[string]any{
				"Account":   tx.Account,
				"Authorize": tx.Authorize,
			},
		})
	}

	return TesSUCCESS
}

// Vault transactions

// applyVaultCreate applies a VaultCreate transaction
func (e *Engine) applyVaultCreate(tx *VaultCreate, account *AccountRoot, metadata *Metadata) Result {
	if tx.Asset.Currency == "" {
		return TemINVALID
	}

	accountID, _ := decodeAccountID(tx.Account)

	// Create vault entry
	var vaultKey [32]byte
	copy(vaultKey[:20], accountID[:])
	binary.BigEndian.PutUint32(vaultKey[20:], account.Sequence)

	vaultKeylet := keylet.Keylet{Key: vaultKey, Type: 0x0084}

	vaultData := make([]byte, 64)
	copy(vaultData[:20], accountID[:])

	if err := e.view.Insert(vaultKeylet, vaultData); err != nil {
		return TefINTERNAL
	}

	account.OwnerCount++

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "Vault",
		LedgerIndex:     hex.EncodeToString(vaultKey[:]),
		NewFields: map[string]any{
			"Owner": tx.Account,
			"Asset": tx.Asset,
		},
	})

	return TesSUCCESS
}

// applyVaultSet applies a VaultSet transaction
func (e *Engine) applyVaultSet(tx *VaultSet, account *AccountRoot, metadata *Metadata) Result {
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

	_, err = e.view.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "Vault",
		LedgerIndex:     tx.VaultID,
	})

	return TesSUCCESS
}

// applyVaultDelete applies a VaultDelete transaction
func (e *Engine) applyVaultDelete(tx *VaultDelete, account *AccountRoot, metadata *Metadata) Result {
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

	if err := e.view.Erase(vaultKeylet); err != nil {
		return TecNO_ENTRY
	}

	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Vault",
		LedgerIndex:     tx.VaultID,
	})

	return TesSUCCESS
}

// applyVaultDeposit applies a VaultDeposit transaction
func (e *Engine) applyVaultDeposit(tx *VaultDeposit, account *AccountRoot, metadata *Metadata) Result {
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

	_, err = e.view.Read(vaultKeylet)
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

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "Vault",
		LedgerIndex:     tx.VaultID,
		FinalFields: map[string]any{
			"DepositAmount": tx.Amount,
		},
	})

	return TesSUCCESS
}

// applyVaultWithdraw applies a VaultWithdraw transaction
func (e *Engine) applyVaultWithdraw(tx *VaultWithdraw, account *AccountRoot, metadata *Metadata) Result {
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

	_, err = e.view.Read(vaultKeylet)
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

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "Vault",
		LedgerIndex:     tx.VaultID,
		FinalFields: map[string]any{
			"WithdrawAmount": tx.Amount,
		},
	})

	return TesSUCCESS
}

// applyVaultClawback applies a VaultClawback transaction
func (e *Engine) applyVaultClawback(tx *VaultClawback, account *AccountRoot, metadata *Metadata) Result {
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

	_, err = e.view.Read(vaultKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	_, err = decodeAccountID(tx.Holder)
	if err != nil {
		return TecNO_TARGET
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "Vault",
		LedgerIndex:     tx.VaultID,
		FinalFields: map[string]any{
			"ClawbackHolder": tx.Holder,
		},
	})

	return TesSUCCESS
}

// Batch transaction

// applyBatch applies a Batch transaction
func (e *Engine) applyBatch(tx *Batch, account *AccountRoot, metadata *Metadata) Result {
	if len(tx.RawTransactions) == 0 {
		return TemINVALID
	}

	flags := tx.GetFlags()

	// Process each raw transaction in the batch
	for i, rawTx := range tx.RawTransactions {
		// Decode and process the raw transaction blob
		_, err := hex.DecodeString(rawTx.RawTransaction.RawTxBlob)
		if err != nil {
			if flags&BatchFlagAllOrNothing != 0 {
				return TefINTERNAL
			}
			continue
		}

		// Record the batch processing
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "BatchedTransaction",
			NewFields: map[string]any{
				"Index":    i,
				"TxnBlob":  rawTx.RawTransaction.RawTxBlob,
				"Executed": true,
			},
		})

		// Check for early termination flags
		if flags&BatchFlagUntilFailure != 0 {
			// Would continue until a failure
		}
		if flags&BatchFlagOnlyOne != 0 {
			// Only execute first successful one
			break
		}
	}

	return TesSUCCESS
}

// LedgerStateFix transaction

// applyLedgerStateFix applies a LedgerStateFix transaction
func (e *Engine) applyLedgerStateFix(tx *LedgerStateFix, account *AccountRoot, metadata *Metadata) Result {
	// LedgerStateFix is a special admin transaction
	// It can only be applied in certain conditions

	if tx.Owner != "" {
		_, err := decodeAccountID(tx.Owner)
		if err != nil {
			return TecNO_TARGET
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "LedgerStateFix",
		NewFields: map[string]any{
			"LedgerFixType": tx.LedgerFixType,
			"Owner":         tx.Owner,
		},
	})

	return TesSUCCESS
}
