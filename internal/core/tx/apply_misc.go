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
// Reference: rippled DID.cpp DIDSet::doApply()
func (e *Engine) applyDIDSet(tx *DIDSet, account *AccountRoot, metadata *Metadata) Result {
	accountID, err := decodeAccountID(tx.Account)
	if err != nil {
		return TefINTERNAL
	}

	didKey := keylet.DID(accountID)

	// Check if DID already exists
	exists, err := e.view.Exists(didKey)
	if err != nil {
		return TefINTERNAL
	}

	if exists {
		// UPDATE EXISTING DID
		// Reference: rippled DID.cpp:122-148

		// Read the existing DID entry
		didData, err := e.view.Read(didKey)
		if err != nil {
			return TefINTERNAL
		}

		didEntry, err := parseDIDEntry(didData)
		if err != nil {
			return TefINTERNAL
		}

		// Store previous values for metadata
		previousFields := map[string]any{}
		if didEntry.URI != nil && *didEntry.URI != "" {
			previousFields["URI"] = *didEntry.URI
		}
		if didEntry.DIDDocument != nil && *didEntry.DIDDocument != "" {
			previousFields["DIDDocument"] = *didEntry.DIDDocument
		}
		if didEntry.Data != nil && *didEntry.Data != "" {
			previousFields["Data"] = *didEntry.Data
		}

		// Update fields based on transaction
		// If field is present in tx and empty, clear it
		// If field is present in tx and non-empty, set it
		// If field is not present in tx, leave unchanged
		if tx.URI != "" {
			// Field is present in tx - set or clear
			didEntry.URI = &tx.URI
		}
		if tx.DIDDocument != "" {
			didEntry.DIDDocument = &tx.DIDDocument
		}
		if tx.Data != "" {
			didEntry.Data = &tx.Data
		}

		// Handle empty strings - they mean "clear the field"
		// The tx fields that are empty strings should clear the entry field
		// But we need to distinguish between "not present" and "empty string"
		// Since Go JSON unmarshaling sets empty string for absent optional fields,
		// we check if the tx explicitly sets a field by checking the raw JSON
		// For simplicity, if tx.URI/DIDDocument/Data is empty string but was explicitly provided,
		// we clear the field. The Validate() already ensures at least one field is present.

		// Clear fields that are explicitly set to empty in the transaction
		if tx.URI == "" && tx.HasURI() {
			empty := ""
			didEntry.URI = &empty
		}
		if tx.DIDDocument == "" && tx.HasDIDDocument() {
			empty := ""
			didEntry.DIDDocument = &empty
		}
		if tx.Data == "" && tx.HasData() {
			empty := ""
			didEntry.Data = &empty
		}

		// Check if at least one field remains after update
		// Reference: rippled DID.cpp:141-146
		hasURI := didEntry.URI != nil && *didEntry.URI != ""
		hasDoc := didEntry.DIDDocument != nil && *didEntry.DIDDocument != ""
		hasData := didEntry.Data != nil && *didEntry.Data != ""

		if !hasURI && !hasDoc && !hasData {
			return TecEMPTY_DID
		}

		// Update transaction threading
		didEntry.PreviousTxnID = e.currentTxHash
		didEntry.PreviousTxnLgrSeq = e.config.LedgerSequence

		// Serialize and update
		updatedData, err := serializeDIDEntry(didEntry)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Update(didKey, updatedData); err != nil {
			return TefINTERNAL
		}

		// Build final fields for metadata
		finalFields := map[string]any{
			"Account": tx.Account,
		}
		if hasURI {
			finalFields["URI"] = *didEntry.URI
		}
		if hasDoc {
			finalFields["DIDDocument"] = *didEntry.DIDDocument
		}
		if hasData {
			finalFields["Data"] = *didEntry.Data
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "DID",
			LedgerIndex:     hex.EncodeToString(didKey.Key[:]),
			FinalFields:     finalFields,
			PreviousFields:  previousFields,
		})

		return TesSUCCESS
	}

	// CREATE NEW DID
	// Reference: rippled DID.cpp:150-171

	// Check reserve requirement
	// Reference: rippled DID.cpp addSLE function
	priorBalance := account.Balance + e.calculateFee(tx)
	reserveNeeded := e.AccountReserve(account.OwnerCount + 1)
	if priorBalance < reserveNeeded {
		return TecINSUFFICIENT_RESERVE
	}

	// Create new DID entry
	didEntry := &DIDEntry{
		Account:           accountID,
		PreviousTxnID:     e.currentTxHash,
		PreviousTxnLgrSeq: e.config.LedgerSequence,
	}

	// Set fields (only if non-empty)
	if tx.URI != "" {
		didEntry.URI = &tx.URI
	}
	if tx.DIDDocument != "" {
		didEntry.DIDDocument = &tx.DIDDocument
	}
	if tx.Data != "" {
		didEntry.Data = &tx.Data
	}

	// Check fixEmptyDID amendment - if enabled, creating a DID with no fields is an error
	// Reference: rippled DID.cpp:163-169
	// For now, assume the amendment is enabled
	if !didEntry.HasAnyField() {
		return TecEMPTY_DID
	}

	// Add to owner directory
	ownerDirKey := keylet.OwnerDir(accountID)
	dirResult, err := e.dirInsert(ownerDirKey, didKey.Key, func(dir *DirectoryNode) {
		dir.Owner = accountID
	})
	if err != nil {
		return TecDIR_FULL
	}

	// Store the owner node hint
	didEntry.OwnerNode = dirResult.Page

	// Serialize and insert
	didData, err := serializeDIDEntry(didEntry)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Insert(didKey, didData); err != nil {
		return TefINTERNAL
	}

	// Increment owner count
	account.OwnerCount++

	// Build new fields for metadata
	newFields := map[string]any{
		"Account":   tx.Account,
		"OwnerNode": formatUint64Hex(didEntry.OwnerNode),
	}
	if tx.URI != "" {
		newFields["URI"] = tx.URI
	}
	if tx.DIDDocument != "" {
		newFields["DIDDocument"] = tx.DIDDocument
	}
	if tx.Data != "" {
		newFields["Data"] = tx.Data
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "DID",
		LedgerIndex:     hex.EncodeToString(didKey.Key[:]),
		NewFields:       newFields,
	})

	// Record directory modification
	if dirResult.Modified {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "DirectoryNode",
			LedgerIndex:     hex.EncodeToString(dirResult.DirKey[:]),
		})
	} else if dirResult.Created {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "DirectoryNode",
			LedgerIndex:     hex.EncodeToString(dirResult.DirKey[:]),
		})
	}

	return TesSUCCESS
}

// applyDIDDelete applies a DIDDelete transaction
// Reference: rippled DID.cpp DIDDelete::doApply() and DIDDelete::deleteSLE()
func (e *Engine) applyDIDDelete(tx *DIDDelete, account *AccountRoot, metadata *Metadata) Result {
	accountID, err := decodeAccountID(tx.Account)
	if err != nil {
		return TefINTERNAL
	}

	didKey := keylet.DID(accountID)

	// Check if DID exists
	// Reference: rippled DID.cpp:192-194
	exists, err := e.view.Exists(didKey)
	if err != nil {
		return TefINTERNAL
	}
	if !exists {
		return TecNO_ENTRY
	}

	// Read the DID entry (needed for OwnerNode hint and metadata)
	didData, err := e.view.Read(didKey)
	if err != nil {
		return TefINTERNAL
	}

	didEntry, err := parseDIDEntry(didData)
	if err != nil {
		return TefINTERNAL
	}

	// Remove from owner directory
	// Reference: rippled DID.cpp:206-212
	ownerDirKey := keylet.OwnerDir(accountID)
	if err := e.dirRemove(ownerDirKey, didEntry.OwnerNode, didKey.Key); err != nil {
		return TefBAD_LEDGER
	}

	// Delete the DID entry
	// Reference: rippled DID.cpp:222
	if err := e.view.Erase(didKey); err != nil {
		return TefINTERNAL
	}

	// Decrement owner count
	// Reference: rippled DID.cpp:218
	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	// Build final fields for metadata (state at time of deletion)
	finalFields := map[string]any{
		"Account":   tx.Account,
		"OwnerNode": formatUint64Hex(didEntry.OwnerNode),
	}
	if didEntry.URI != nil && *didEntry.URI != "" {
		finalFields["URI"] = *didEntry.URI
	}
	if didEntry.DIDDocument != nil && *didEntry.DIDDocument != "" {
		finalFields["DIDDocument"] = *didEntry.DIDDocument
	}
	if didEntry.Data != nil && *didEntry.Data != "" {
		finalFields["Data"] = *didEntry.Data
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "DID",
		LedgerIndex:     hex.EncodeToString(didKey.Key[:]),
		FinalFields:     finalFields,
	})

	return TesSUCCESS
}

// Oracle transactions

// applyOracleSet applies an OracleSet transaction.
// Reference: rippled SetOracle.cpp SetOracle::preclaim() and SetOracle::doApply()
func (e *Engine) applyOracleSet(tx *OracleSet, account *AccountRoot, metadata *Metadata) Result {
	accountID, err := decodeAccountID(tx.Account)
	if err != nil {
		return TefINTERNAL
	}

	oracleKey := keylet.Oracle(accountID, tx.OracleDocumentID)

	// PRECLAIM PHASE
	// Reference: rippled SetOracle.cpp lines 70-182

	// Validate LastUpdateTime is within acceptable range
	// Reference: rippled SetOracle.cpp:80-93
	// lastUpdateTime must be within maxLastUpdateTimeDelta seconds of the last closed ledger
	closeTime := e.config.ParentCloseTime
	lastUpdateTime := tx.LastUpdateTime

	// Check that lastUpdateTime is not before epoch (rippled epoch_offset = 946684800)
	// Reference: rippled SetOracle.cpp:85-86
	if lastUpdateTime < uint32(RippleEpoch-946684800) { // This converts to Ripple epoch check
		// For simplicity, we just check if it's reasonable
		// In rippled: if (lastUpdateTime < epoch_offset.count())
		return TecINVALID_UPDATE_TIME
	}

	// Check time delta (must be within 300 seconds of close time)
	// Reference: rippled SetOracle.cpp:87-93
	lastUpdateTimeEpoch := lastUpdateTime
	if closeTime > MaxLastUpdateTimeDelta {
		if lastUpdateTimeEpoch < (closeTime-MaxLastUpdateTimeDelta) ||
			lastUpdateTimeEpoch > (closeTime+MaxLastUpdateTimeDelta) {
			return TecINVALID_UPDATE_TIME
		}
	}

	// Build the sets of token pairs to add/update and delete
	// Reference: rippled SetOracle.cpp:99-118
	pairs := make(map[string]PriceDataEntry)    // pairs to add/update (have AssetPrice)
	pairsDel := make(map[string]struct{})       // pairs to delete (no AssetPrice)

	// Check if oracle already exists
	exists, _ := e.view.Exists(oracleKey)
	var existingOracle *OracleEntry

	if exists {
		oracleData, err := e.view.Read(oracleKey)
		if err != nil {
			return TefINTERNAL
		}
		existingOracle, err = parseOracleEntry(oracleData)
		if err != nil {
			return TefINTERNAL
		}
	}

	for _, pd := range tx.PriceDataSeries {
		entry := pd.PriceData

		// BaseAsset and QuoteAsset must be different
		// Reference: rippled SetOracle.cpp:105-106
		if entry.BaseAsset == entry.QuoteAsset {
			return TemMALFORMED
		}

		key := TokenPairKey(entry.BaseAsset, entry.QuoteAsset)

		// Check for duplicates in the transaction
		// Reference: rippled SetOracle.cpp:108-109
		if _, inPairs := pairs[key]; inPairs {
			return TemMALFORMED
		}
		if _, inDel := pairsDel[key]; inDel {
			return TemMALFORMED
		}

		// Validate Scale
		// Reference: rippled SetOracle.cpp:110-111
		if entry.Scale != nil && *entry.Scale > MaxPriceScale {
			return TemMALFORMED
		}

		// If AssetPrice is present, it's an add/update
		// If AssetPrice is absent, it's a delete
		// Reference: rippled SetOracle.cpp:112-117
		if entry.AssetPrice != nil {
			pairs[key] = entry
		} else if existingOracle != nil {
			// Can only delete if oracle exists
			pairsDel[key] = struct{}{}
		} else {
			// Cannot delete token pair when creating oracle
			return TemMALFORMED
		}
	}

	var adjustReserve int
	if existingOracle != nil {
		// UPDATE existing oracle
		// Reference: rippled SetOracle.cpp:129-158

		// lastUpdateTime must be more recent than the previous one
		// Reference: rippled SetOracle.cpp:135-136
		if tx.LastUpdateTime <= existingOracle.LastUpdateTime {
			return TecINVALID_UPDATE_TIME
		}

		// Provider and AssetClass must match if provided
		// Reference: rippled SetOracle.cpp:138-139
		if tx.Provider != "" && tx.Provider != existingOracle.Provider {
			return TemMALFORMED
		}
		if tx.AssetClass != "" && tx.AssetClass != existingOracle.AssetClass {
			return TemMALFORMED
		}

		// Merge existing pairs with new/deleted pairs
		// Reference: rippled SetOracle.cpp:141-154
		for _, existingPd := range existingOracle.PriceDataSeries {
			key := TokenPairKey(existingPd.BaseAsset, existingPd.QuoteAsset)
			if _, inPairs := pairs[key]; !inPairs {
				if _, inDel := pairsDel[key]; inDel {
					delete(pairsDel, key)
				} else {
					// Keep existing pair (without price update - cleared price/scale)
					pairs[key] = PriceDataEntry{
						BaseAsset:  existingPd.BaseAsset,
						QuoteAsset: existingPd.QuoteAsset,
						// No AssetPrice/Scale means we keep just the pair identity
					}
				}
			}
		}

		// If any pairs to delete still remain, they don't exist in the oracle
		// Reference: rippled SetOracle.cpp:152-153
		if len(pairsDel) > 0 {
			return TecTOKEN_PAIR_NOT_FOUND
		}

		// Calculate reserve adjustment
		oldCount := calculateOracleReserveCount(len(existingOracle.PriceDataSeries))
		newCount := calculateOracleReserveCount(len(pairs))
		adjustReserve = newCount - oldCount
	} else {
		// CREATE new oracle
		// Reference: rippled SetOracle.cpp:160-168

		// Provider and AssetClass are required when creating
		// Reference: rippled SetOracle.cpp:164-166
		if tx.Provider == "" || tx.AssetClass == "" {
			return TemMALFORMED
		}

		adjustReserve = calculateOracleReserveCount(len(pairs))
	}

	// Check resulting array is not empty
	// Reference: rippled SetOracle.cpp:170-171
	if len(pairs) == 0 {
		return TecARRAY_EMPTY
	}

	// Check resulting array does not exceed maximum
	// Reference: rippled SetOracle.cpp:172-173
	if len(pairs) > MaxOracleDataSeries {
		return TecARRAY_TOO_LARGE
	}

	// Check reserve requirement
	// Reference: rippled SetOracle.cpp:175-180
	if adjustReserve > 0 {
		reserve := e.AccountReserve(account.OwnerCount + uint32(adjustReserve))
		priorBalance := account.Balance + e.calculateFee(tx)
		if priorBalance < reserve {
			return TecINSUFFICIENT_RESERVE
		}
	}

	// APPLY PHASE
	// Reference: rippled SetOracle.cpp:207-330

	if existingOracle != nil {
		// UPDATE existing oracle
		// Reference: rippled SetOracle.cpp:223-280

		// Build the updated PriceDataSeries
		// Token pairs that don't have their price updated will not include price/scale
		updatedPairs := make(map[string]OraclePriceDataEntry)

		// Collect current token pairs (without price/scale)
		for _, pd := range existingOracle.PriceDataSeries {
			key := TokenPairKey(pd.BaseAsset, pd.QuoteAsset)
			updatedPairs[key] = OraclePriceDataEntry{
				BaseAsset:  pd.BaseAsset,
				QuoteAsset: pd.QuoteAsset,
				// No price/scale - will be added if updated
			}
		}

		oldCount := len(updatedPairs)

		// Process transaction updates
		for _, pd := range tx.PriceDataSeries {
			entry := pd.PriceData
			key := TokenPairKey(entry.BaseAsset, entry.QuoteAsset)

			if entry.AssetPrice == nil {
				// Delete token pair
				delete(updatedPairs, key)
			} else if existing, ok := updatedPairs[key]; ok {
				// Update existing pair
				existing.AssetPrice = *entry.AssetPrice
				if entry.Scale != nil {
					existing.Scale = *entry.Scale
					existing.HasScale = true
				}
				updatedPairs[key] = existing
			} else {
				// Add new pair
				newEntry := OraclePriceDataEntry{
					BaseAsset:  entry.BaseAsset,
					QuoteAsset: entry.QuoteAsset,
					AssetPrice: *entry.AssetPrice,
				}
				if entry.Scale != nil {
					newEntry.Scale = *entry.Scale
					newEntry.HasScale = true
				}
				updatedPairs[key] = newEntry
			}
		}

		// Convert map to slice for storage
		existingOracle.PriceDataSeries = make([]OraclePriceDataEntry, 0, len(updatedPairs))
		for _, pd := range updatedPairs {
			existingOracle.PriceDataSeries = append(existingOracle.PriceDataSeries, pd)
		}

		// Update URI if provided
		if tx.URI != "" {
			existingOracle.URI = &tx.URI
		}

		// Update LastUpdateTime
		existingOracle.LastUpdateTime = tx.LastUpdateTime

		// Update transaction threading
		existingOracle.PreviousTxnID = e.currentTxHash
		existingOracle.PreviousTxnLgrSeq = e.config.LedgerSequence

		// Adjust owner count if needed
		newCount := calculateOracleReserveCount(len(updatedPairs))
		oldReserveCount := calculateOracleReserveCount(oldCount)
		adjust := newCount - oldReserveCount
		if adjust != 0 {
			if adjust > 0 {
				account.OwnerCount += uint32(adjust)
			} else if account.OwnerCount >= uint32(-adjust) {
				account.OwnerCount -= uint32(-adjust)
			}
		}

		// Serialize and update
		oracleData, err := serializeOracleEntry(existingOracle)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Update(oracleKey, oracleData); err != nil {
			return TefINTERNAL
		}

		// Record metadata
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "Oracle",
			LedgerIndex:     hex.EncodeToString(oracleKey.Key[:]),
			FinalFields: map[string]any{
				"Owner":          tx.Account,
				"LastUpdateTime": tx.LastUpdateTime,
			},
		})
	} else {
		// CREATE new oracle
		// Reference: rippled SetOracle.cpp:282-327

		// Build PriceDataSeries
		priceDataSeries := make([]OraclePriceDataEntry, 0, len(pairs))
		for _, entry := range pairs {
			pd := OraclePriceDataEntry{
				BaseAsset:  entry.BaseAsset,
				QuoteAsset: entry.QuoteAsset,
			}
			if entry.AssetPrice != nil {
				pd.AssetPrice = *entry.AssetPrice
			}
			if entry.Scale != nil {
				pd.Scale = *entry.Scale
				pd.HasScale = true
			}
			priceDataSeries = append(priceDataSeries, pd)
		}

		// Create new Oracle entry
		newOracle := &OracleEntry{
			Owner:           accountID,
			Provider:        tx.Provider,
			AssetClass:      tx.AssetClass,
			LastUpdateTime:  tx.LastUpdateTime,
			PriceDataSeries: priceDataSeries,
			PreviousTxnID:   e.currentTxHash,
			PreviousTxnLgrSeq: e.config.LedgerSequence,
		}

		if tx.URI != "" {
			newOracle.URI = &tx.URI
		}

		// Add to owner directory
		ownerDirKey := keylet.OwnerDir(accountID)
		dirResult, err := e.dirInsert(ownerDirKey, oracleKey.Key, func(dir *DirectoryNode) {
			dir.Owner = accountID
		})
		if err != nil {
			return TecDIR_FULL
		}

		newOracle.OwnerNode = dirResult.Page

		// Adjust owner count
		count := calculateOracleReserveCount(len(priceDataSeries))
		account.OwnerCount += uint32(count)

		// Serialize and insert
		oracleData, err := serializeOracleEntry(newOracle)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Insert(oracleKey, oracleData); err != nil {
			return TefINTERNAL
		}

		// Record metadata
		newFields := map[string]any{
			"Owner":          tx.Account,
			"Provider":       tx.Provider,
			"AssetClass":     tx.AssetClass,
			"LastUpdateTime": tx.LastUpdateTime,
			"OwnerNode":      formatUint64Hex(dirResult.Page),
		}
		if tx.URI != "" {
			newFields["URI"] = tx.URI
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "Oracle",
			LedgerIndex:     hex.EncodeToString(oracleKey.Key[:]),
			NewFields:       newFields,
		})

		// Record directory modification if needed
		if dirResult.Modified {
			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "ModifiedNode",
				LedgerEntryType: "DirectoryNode",
				LedgerIndex:     hex.EncodeToString(dirResult.DirKey[:]),
			})
		} else if dirResult.Created {
			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "CreatedNode",
				LedgerEntryType: "DirectoryNode",
				LedgerIndex:     hex.EncodeToString(dirResult.DirKey[:]),
			})
		}
	}

	return TesSUCCESS
}

// applyOracleDelete applies an OracleDelete transaction.
// Reference: rippled DeleteOracle.cpp DeleteOracle::preclaim() and DeleteOracle::doApply()
func (e *Engine) applyOracleDelete(tx *OracleDelete, account *AccountRoot, metadata *Metadata) Result {
	accountID, err := decodeAccountID(tx.Account)
	if err != nil {
		return TefINTERNAL
	}

	oracleKey := keylet.Oracle(accountID, tx.OracleDocumentID)

	// PRECLAIM: Check oracle exists
	// Reference: rippled DeleteOracle.cpp:53-59
	exists, err := e.view.Exists(oracleKey)
	if err != nil {
		return TefINTERNAL
	}
	if !exists {
		return TecNO_ENTRY
	}

	// Read the oracle entry
	oracleData, err := e.view.Read(oracleKey)
	if err != nil {
		return TefINTERNAL
	}

	oracle, err := parseOracleEntry(oracleData)
	if err != nil {
		return TefINTERNAL
	}

	// Verify caller is the owner (this should always be true since keylet uses account)
	// Reference: rippled DeleteOracle.cpp:60-66
	if oracle.Owner != accountID {
		return TecINTERNAL
	}

	// APPLY: Delete the oracle
	// Reference: rippled DeleteOracle.cpp:72-102

	// Remove from owner directory
	// Reference: rippled DeleteOracle.cpp:81-88
	ownerDirKey := keylet.OwnerDir(accountID)
	if err := e.dirRemove(ownerDirKey, oracle.OwnerNode, oracleKey.Key); err != nil {
		return TefBAD_LEDGER
	}

	// Calculate and adjust owner count
	// Reference: rippled DeleteOracle.cpp:94-97
	count := calculateOracleReserveCount(len(oracle.PriceDataSeries))
	if account.OwnerCount >= uint32(count) {
		account.OwnerCount -= uint32(count)
	}

	// Delete the oracle entry
	// Reference: rippled DeleteOracle.cpp:99
	if err := e.view.Erase(oracleKey); err != nil {
		return TefINTERNAL
	}

	// Record metadata
	finalFields := map[string]any{
		"Owner":          tx.Account,
		"Provider":       oracle.Provider,
		"AssetClass":     oracle.AssetClass,
		"LastUpdateTime": oracle.LastUpdateTime,
		"OwnerNode":      formatUint64Hex(oracle.OwnerNode),
	}
	if oracle.URI != nil && *oracle.URI != "" {
		finalFields["URI"] = *oracle.URI
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Oracle",
		LedgerIndex:     hex.EncodeToString(oracleKey.Key[:]),
		FinalFields:     finalFields,
	})

	return TesSUCCESS
}

// MPToken transactions
//
// MPToken system overview:
// - MPTokenIssuance: Created by issuer, defines the token (like a currency)
// - MPToken: Created by holder, holds balance (like a trust line but simpler)
// - Issuer can lock tokens globally or per-holder
// - Holders may need issuer authorization (if tfMPTRequireAuth was set on issuance)

// applyMPTokenIssuanceCreate applies an MPTokenIssuanceCreate transaction
// Reference: rippled MPTokenIssuanceCreate.cpp create() and doApply()
func (e *Engine) applyMPTokenIssuanceCreate(tx *MPTokenIssuanceCreate, account *AccountRoot, metadata *Metadata) Result {
	accountID, err := decodeAccountID(tx.Account)
	if err != nil {
		return TefINTERNAL
	}

	sequence := *tx.GetCommon().Sequence

	// Check reserve requirement
	// Reference: rippled MPTokenIssuanceCreate.cpp:96-98
	reserveNeeded := e.AccountReserve(account.OwnerCount + 1)
	priorBalance := account.Balance + e.calculateFee(tx) // mPriorBalance (before fee deduction)
	if priorBalance < reserveNeeded {
		return TecINSUFFICIENT_RESERVE
	}

	// Create the MPTokenIssuanceID: sequence (big-endian 4 bytes) + account (20 bytes)
	mptID := keylet.MakeMPTID(sequence, accountID)
	mptIssuanceKey := keylet.MPTIssuance(mptID)

	// Create owner directory entry
	ownerDirKey := keylet.OwnerDir(accountID)
	dirResult, err := e.dirInsert(ownerDirKey, mptIssuanceKey.Key, func(dir *DirectoryNode) {
		dir.Owner = accountID
	})
	if err != nil {
		return TecDIR_FULL
	}

	// Get flags - these become the issuance flags (without tfUniversal)
	flags := tx.GetFlags() & ^tfUniversal

	// Build the MPTokenIssuance ledger entry
	newFields := map[string]any{
		"Issuer":            tx.Account,
		"Sequence":          sequence,
		"Flags":             flags,
		"OutstandingAmount": uint64(0),
		"OwnerNode":         formatUint64Hex(dirResult.Page),
	}

	// Add optional fields
	if tx.MaximumAmount != nil {
		newFields["MaximumAmount"] = *tx.MaximumAmount
	}
	if tx.AssetScale != nil {
		newFields["AssetScale"] = *tx.AssetScale
	}
	if tx.TransferFee != nil {
		newFields["TransferFee"] = *tx.TransferFee
	}
	if tx.MPTokenMetadata != "" {
		newFields["MPTokenMetadata"] = tx.MPTokenMetadata
	}

	// Serialize and insert the issuance entry
	issuanceData := serializeMPTokenIssuance(newFields)
	if err := e.view.Insert(mptIssuanceKey, issuanceData); err != nil {
		return TefINTERNAL
	}

	// Update owner count
	account.OwnerCount++

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "MPTokenIssuance",
		LedgerIndex:     hex.EncodeToString(mptIssuanceKey.Key[:]),
		NewFields:       newFields,
	})

	// Record directory modification if modified
	if dirResult.Modified {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "DirectoryNode",
			LedgerIndex:     hex.EncodeToString(dirResult.DirKey[:]),
		})
	} else if dirResult.Created {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "DirectoryNode",
			LedgerIndex:     hex.EncodeToString(dirResult.DirKey[:]),
		})
	}

	return TesSUCCESS
}

// applyMPTokenIssuanceDestroy applies an MPTokenIssuanceDestroy transaction
// Reference: rippled MPTokenIssuanceDestroy.cpp preclaim() and doApply()
func (e *Engine) applyMPTokenIssuanceDestroy(tx *MPTokenIssuanceDestroy, account *AccountRoot, metadata *Metadata) Result {
	accountID, err := decodeAccountID(tx.Account)
	if err != nil {
		return TefINTERNAL
	}

	// Parse issuance ID - this is actually the hash/key of the issuance entry
	issuanceIDBytes, err := hex.DecodeString(tx.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return TefINTERNAL
	}

	var issuanceKey [32]byte
	copy(issuanceKey[:], issuanceIDBytes)
	mptIssuanceKeylet := keylet.Keylet{Key: issuanceKey}

	// Check that issuance exists
	issuanceData, err := e.view.Read(mptIssuanceKeylet)
	if err != nil {
		return TecOBJECT_NOT_FOUND
	}

	// Parse the issuance to verify ownership and check obligations
	issuance, err := parseMPTokenIssuance(issuanceData)
	if err != nil {
		return TefINTERNAL
	}

	// Verify caller is the issuer
	// Reference: rippled MPTokenIssuanceDestroy.cpp:54
	if issuance.Issuer != accountID {
		return TecNO_PERMISSION
	}

	// Check that there are no outstanding tokens (obligations)
	// Reference: rippled MPTokenIssuanceDestroy.cpp:58-62
	if issuance.OutstandingAmount != 0 {
		return TecHAS_OBLIGATIONS
	}
	if issuance.LockedAmount != nil && *issuance.LockedAmount != 0 {
		return TecHAS_OBLIGATIONS
	}

	// Remove from owner directory
	ownerDirKey := keylet.OwnerDir(accountID)
	if err := e.dirRemove(ownerDirKey, issuance.OwnerNode, issuanceKey); err != nil {
		return TefBAD_LEDGER
	}

	// Delete the issuance entry
	if err := e.view.Erase(mptIssuanceKeylet); err != nil {
		return TefINTERNAL
	}

	// Decrement owner count
	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "MPTokenIssuance",
		LedgerIndex:     hex.EncodeToString(issuanceKey[:]),
		FinalFields: map[string]any{
			"Issuer":            tx.Account,
			"OutstandingAmount": uint64(0),
		},
	})

	return TesSUCCESS
}

// applyMPTokenIssuanceSet applies an MPTokenIssuanceSet transaction
// Reference: rippled MPTokenIssuanceSet.cpp preclaim() and doApply()
func (e *Engine) applyMPTokenIssuanceSet(tx *MPTokenIssuanceSet, account *AccountRoot, metadata *Metadata) Result {
	accountID, err := decodeAccountID(tx.Account)
	if err != nil {
		return TefINTERNAL
	}

	// Parse issuance ID
	issuanceIDBytes, err := hex.DecodeString(tx.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return TefINTERNAL
	}

	var issuanceKey [32]byte
	copy(issuanceKey[:], issuanceIDBytes)
	mptIssuanceKeylet := keylet.Keylet{Key: issuanceKey}

	// Check that issuance exists
	issuanceData, err := e.view.Read(mptIssuanceKeylet)
	if err != nil {
		return TecOBJECT_NOT_FOUND
	}

	// Parse the issuance
	issuance, err := parseMPTokenIssuance(issuanceData)
	if err != nil {
		return TefINTERNAL
	}

	// Verify caller is the issuer
	if issuance.Issuer != accountID {
		return TecNO_PERMISSION
	}

	txFlags := tx.GetFlags()

	// If trying to lock/unlock, must have lsfMPTCanLock set on issuance
	// Reference: rippled MPTokenIssuanceSet.cpp:116-123
	if (txFlags&MPTokenIssuanceSetFlagLock) != 0 || (txFlags&MPTokenIssuanceSetFlagUnlock) != 0 {
		if (issuance.Flags & MPTokenIssuanceCreateFlagCanLock) == 0 {
			return TecNO_PERMISSION
		}
	}

	// Determine which entry to modify: issuance or specific holder's MPToken
	var sle interface{}
	var sleKey [32]byte
	var sleType string

	if tx.Holder != "" {
		// Modifying a specific holder's MPToken entry
		holderID, err := decodeAccountID(tx.Holder)
		if err != nil {
			return TecNO_DST
		}

		// Check holder account exists
		holderAccountKey := keylet.Account(holderID)
		exists, _ := e.view.Exists(holderAccountKey)
		if !exists {
			return TecNO_DST
		}

		// Get the holder's MPToken entry
		mptokenKey := keylet.MPToken(issuanceKey, holderID)
		mptokenData, err := e.view.Read(mptokenKey)
		if err != nil {
			return TecOBJECT_NOT_FOUND
		}

		mptoken, err := parseMPToken(mptokenData)
		if err != nil {
			return TefINTERNAL
		}

		sle = mptoken
		sleKey = mptokenKey.Key
		sleType = "MPToken"
	} else {
		// Modifying the issuance itself (global lock/unlock)
		sle = issuance
		sleKey = issuanceKey
		sleType = "MPTokenIssuance"
	}

	// Apply flag changes
	var flagsIn, flagsOut uint32
	switch entry := sle.(type) {
	case *MPTokenIssuanceEntry:
		flagsIn = entry.Flags
		flagsOut = flagsIn
		if txFlags&MPTokenIssuanceSetFlagLock != 0 {
			flagsOut |= lsfMPTLocked
		} else if txFlags&MPTokenIssuanceSetFlagUnlock != 0 {
			flagsOut &= ^lsfMPTLocked
		}
		entry.Flags = flagsOut
	case *MPTokenEntry:
		flagsIn = entry.Flags
		flagsOut = flagsIn
		if txFlags&MPTokenIssuanceSetFlagLock != 0 {
			flagsOut |= lsfMPTLocked
		} else if txFlags&MPTokenIssuanceSetFlagUnlock != 0 {
			flagsOut &= ^lsfMPTLocked
		}
		entry.Flags = flagsOut
	}

	// Update the entry in the ledger
	var updatedData []byte
	switch entry := sle.(type) {
	case *MPTokenIssuanceEntry:
		updatedData = serializeMPTokenIssuanceEntry(entry)
	case *MPTokenEntry:
		updatedData = serializeMPTokenEntry(entry)
	}

	sleKeylet := keylet.Keylet{Key: sleKey}
	if err := e.view.Update(sleKeylet, updatedData); err != nil {
		return TefINTERNAL
	}

	// Record in metadata
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: sleType,
		LedgerIndex:     hex.EncodeToString(sleKey[:]),
		FinalFields: map[string]any{
			"Flags": flagsOut,
		},
		PreviousFields: map[string]any{
			"Flags": flagsIn,
		},
	})

	return TesSUCCESS
}

// applyMPTokenAuthorize applies an MPTokenAuthorize transaction
// Reference: rippled MPTokenAuthorize.cpp preclaim() and doApply()
// Also: rippled View.cpp authorizeMPToken()
func (e *Engine) applyMPTokenAuthorize(tx *MPTokenAuthorize, account *AccountRoot, metadata *Metadata) Result {
	accountID, err := decodeAccountID(tx.Account)
	if err != nil {
		return TefINTERNAL
	}

	// Parse issuance ID (this is the hash/key of the issuance entry)
	issuanceIDBytes, err := hex.DecodeString(tx.MPTokenIssuanceID)
	if err != nil || len(issuanceIDBytes) != 32 {
		return TefINTERNAL
	}

	var issuanceKey [32]byte
	copy(issuanceKey[:], issuanceIDBytes)
	mptIssuanceKeylet := keylet.Keylet{Key: issuanceKey}

	txFlags := tx.GetFlags()
	holderID := tx.Holder

	// MODE 1: Non-issuer (holder) is submitting
	// Either creating MPToken entry (authorize) or deleting it (unauthorize)
	if holderID == "" {
		// Holder is the transaction submitter
		mptokenKey := keylet.MPToken(issuanceKey, accountID)

		// Check if holder wants to delete/unauthorize
		if txFlags&MPTokenAuthorizeFlagUnauthorize != 0 {
			// Delete the holder's MPToken entry
			// Reference: rippled MPTokenAuthorize.cpp:70-100

			mptokenData, err := e.view.Read(mptokenKey)
			if err != nil {
				return TecOBJECT_NOT_FOUND
			}

			mptoken, err := parseMPToken(mptokenData)
			if err != nil {
				return TefINTERNAL
			}

			// Cannot delete if has balance
			if mptoken.MPTAmount != 0 {
				return TecHAS_OBLIGATIONS
			}

			// Cannot delete if has locked amount
			if mptoken.LockedAmount != nil && *mptoken.LockedAmount != 0 {
				return TecHAS_OBLIGATIONS
			}

			// Remove from owner directory
			ownerDirKey := keylet.OwnerDir(accountID)
			if err := e.dirRemove(ownerDirKey, mptoken.OwnerNode, mptokenKey.Key); err != nil {
				// Directory error, but continue
			}

			// Delete the MPToken entry
			if err := e.view.Erase(mptokenKey); err != nil {
				return TefINTERNAL
			}

			// Decrement owner count
			if account.OwnerCount > 0 {
				account.OwnerCount--
			}

			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "DeletedNode",
				LedgerEntryType: "MPToken",
				LedgerIndex:     hex.EncodeToString(mptokenKey.Key[:]),
				FinalFields: map[string]any{
					"Account":           tx.Account,
					"MPTokenIssuanceID": tx.MPTokenIssuanceID,
					"MPTAmount":         uint64(0),
				},
			})

			return TesSUCCESS
		}

		// Holder wants to create/hold MPToken
		// Reference: rippled MPTokenAuthorize.cpp:102-117, View.cpp:1262-1292

		// Check issuance exists
		issuanceData, err := e.view.Read(mptIssuanceKeylet)
		if err != nil {
			return TecOBJECT_NOT_FOUND
		}

		issuance, err := parseMPTokenIssuance(issuanceData)
		if err != nil {
			return TefINTERNAL
		}

		// Cannot create MPToken for your own issuance
		if issuance.Issuer == accountID {
			return TecNO_PERMISSION
		}

		// Check if already have MPToken entry (duplicate)
		exists, _ := e.view.Exists(mptokenKey)
		if exists {
			return TecDUPLICATE
		}

		// Check reserve - similar to trust line logic
		// First 2 objects are free, after that need full reserve
		// Reference: rippled View.cpp:1271-1277
		ownerCount := account.OwnerCount
		priorBalance := account.Balance + e.calculateFee(tx)
		if ownerCount >= 2 {
			reserveNeeded := e.AccountReserve(ownerCount + 1)
			if priorBalance < reserveNeeded {
				return TecINSUFFICIENT_RESERVE
			}
		}

		// Add to owner directory
		ownerDirKey := keylet.OwnerDir(accountID)
		dirResult, err := e.dirInsert(ownerDirKey, mptokenKey.Key, func(dir *DirectoryNode) {
			dir.Owner = accountID
		})
		if err != nil {
			return TecDIR_FULL
		}

		// Create the MPToken entry
		mptokenFields := map[string]any{
			"Account":           tx.Account,
			"MPTokenIssuanceID": tx.MPTokenIssuanceID,
			"Flags":             uint32(0),
			"OwnerNode":         formatUint64Hex(dirResult.Page),
		}

		mptokenData := serializeMPToken(mptokenFields)
		if err := e.view.Insert(mptokenKey, mptokenData); err != nil {
			return TefINTERNAL
		}

		// Increment owner count
		account.OwnerCount++

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "MPToken",
			LedgerIndex:     hex.EncodeToString(mptokenKey.Key[:]),
			NewFields:       mptokenFields,
		})

		return TesSUCCESS
	}

	// MODE 2: Issuer is authorizing/unauthorizing a holder
	// Reference: rippled MPTokenAuthorize.cpp:119-150, View.cpp:1295-1326

	holderAccountID, err := decodeAccountID(holderID)
	if err != nil {
		return TecNO_DST
	}

	// Check holder account exists
	holderAccountKey := keylet.Account(holderAccountID)
	exists, _ := e.view.Exists(holderAccountKey)
	if !exists {
		return TecNO_DST
	}

	// Check issuance exists and caller is the issuer
	issuanceData, err := e.view.Read(mptIssuanceKeylet)
	if err != nil {
		return TecOBJECT_NOT_FOUND
	}

	issuance, err := parseMPTokenIssuance(issuanceData)
	if err != nil {
		return TefINTERNAL
	}

	if issuance.Issuer != accountID {
		return TecNO_PERMISSION
	}

	// Issuance must have lsfMPTRequireAuth for issuer to authorize holders
	if (issuance.Flags & MPTokenIssuanceCreateFlagRequireAuth) == 0 {
		return TecNO_AUTH
	}

	// Get the holder's MPToken entry
	mptokenKey := keylet.MPToken(issuanceKey, holderAccountID)
	mptokenData, err := e.view.Read(mptokenKey)
	if err != nil {
		return TecOBJECT_NOT_FOUND
	}

	mptoken, err := parseMPToken(mptokenData)
	if err != nil {
		return TefINTERNAL
	}

	flagsIn := mptoken.Flags
	flagsOut := flagsIn

	if txFlags&MPTokenAuthorizeFlagUnauthorize != 0 {
		// Issuer wants to unauthorize the holder
		flagsOut &= ^lsfMPTAuthorized
	} else {
		// Issuer wants to authorize the holder
		flagsOut |= lsfMPTAuthorized
	}

	// Update the MPToken flags
	if flagsIn != flagsOut {
		mptoken.Flags = flagsOut
		updatedData := serializeMPTokenEntry(mptoken)
		if err := e.view.Update(mptokenKey, updatedData); err != nil {
			return TefINTERNAL
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "MPToken",
		LedgerIndex:     hex.EncodeToString(mptokenKey.Key[:]),
		FinalFields: map[string]any{
			"Account":           holderID,
			"MPTokenIssuanceID": tx.MPTokenIssuanceID,
			"Flags":             flagsOut,
		},
		PreviousFields: map[string]any{
			"Flags": flagsIn,
		},
	})

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
// Reference: rippled Credentials.cpp

// applyCredentialCreate applies a CredentialCreate transaction
// Reference: rippled Credentials.cpp CredentialCreate::doApply()
func (e *Engine) applyCredentialCreate(tx *CredentialCreate, account *AccountRoot, metadata *Metadata) Result {
	if tx.Subject == "" || tx.CredentialType == "" {
		return TemINVALID
	}

	subjectID, err := decodeAccountID(tx.Subject)
	if err != nil {
		return TecNO_TARGET
	}

	issuerID, _ := decodeAccountID(tx.Account)

	// Decode credential type from hex
	credType, err := hex.DecodeString(tx.CredentialType)
	if err != nil {
		return TemINVALID
	}

	// Check if subject account exists
	// Reference: rippled Credentials.cpp preclaim - check subject exists
	subjectKeylet := keylet.Account(subjectID)
	if _, err := e.view.Read(subjectKeylet); err != nil {
		return TecNO_TARGET
	}

	// Compute proper credential keylet
	// Reference: rippled Indexes.cpp credential(subject, issuer, credentialType)
	credKeylet := keylet.Credential(subjectID, issuerID, credType)

	// Check for duplicate credential
	// Reference: rippled Credentials.cpp preclaim - check credential doesn't exist
	if _, err := e.view.Read(credKeylet); err == nil {
		return TecDUPLICATE
	}

	// Check expiration is not in the past
	// Reference: rippled Credentials.cpp doApply lines 131-144
	if tx.Expiration != nil {
		closeTime := e.config.ParentCloseTime
		if closeTime > *tx.Expiration {
			return TecEXPIRED
		}
	}

	// Check issuer has sufficient reserve
	// Reference: rippled Credentials.cpp doApply lines 151-155
	reserve := e.AccountReserve(account.OwnerCount + 1)
	if account.Balance < reserve {
		return TecINSUFFICIENT_RESERVE
	}

	// Create credential entry
	cred := &CredentialEntry{
		Subject:        subjectID,
		Issuer:         issuerID,
		CredentialType: credType,
		Expiration:     tx.Expiration,
	}

	// Parse URI if present
	if tx.URI != "" {
		uri, err := hex.DecodeString(tx.URI)
		if err != nil {
			return TemINVALID
		}
		cred.URI = uri
	}

	// Serialize and insert the credential
	credData, err := serializeCredentialEntry(cred)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Insert(credKeylet, credData); err != nil {
		return TefINTERNAL
	}

	// Increment issuer's owner count
	// Reference: rippled Credentials.cpp doApply line 177
	account.OwnerCount++

	// Build metadata
	newFields := map[string]any{
		"Issuer":         tx.Account,
		"Subject":        tx.Subject,
		"CredentialType": tx.CredentialType,
	}
	if tx.Expiration != nil {
		newFields["Expiration"] = *tx.Expiration
	}
	if tx.URI != "" {
		newFields["URI"] = tx.URI
	}

	// Check if self-issued (subject == issuer)
	// Reference: rippled Credentials.cpp doApply lines 180-196
	if subjectID == issuerID {
		// Auto-accept: set lsfAccepted flag
		newFields["Flags"] = LsfCredentialAccepted
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "Credential",
		LedgerIndex:     hex.EncodeToString(credKeylet.Key[:]),
		NewFields:       newFields,
	})

	return TesSUCCESS
}

// applyCredentialAccept applies a CredentialAccept transaction
// Reference: rippled Credentials.cpp CredentialAccept::doApply()
func (e *Engine) applyCredentialAccept(tx *CredentialAccept, account *AccountRoot, metadata *Metadata) Result {
	if tx.Issuer == "" || tx.CredentialType == "" {
		return TemINVALID
	}

	issuerID, err := decodeAccountID(tx.Issuer)
	if err != nil {
		return TecNO_TARGET
	}

	subjectID, _ := decodeAccountID(tx.Account)

	// Decode credential type from hex
	credType, err := hex.DecodeString(tx.CredentialType)
	if err != nil {
		return TemINVALID
	}

	// Check issuer account exists
	// Reference: rippled Credentials.cpp preclaim - check issuer exists
	issuerKeylet := keylet.Account(issuerID)
	issuerData, err := e.view.Read(issuerKeylet)
	if err != nil {
		return TecNO_ISSUER
	}
	issuerAccount, err := parseAccountRoot(issuerData)
	if err != nil {
		return TefINTERNAL
	}

	// Compute credential keylet
	credKeylet := keylet.Credential(subjectID, issuerID, credType)

	// Read the credential entry
	// Reference: rippled Credentials.cpp preclaim - check credential exists
	credData, err := e.view.Read(credKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	cred, err := parseCredentialEntry(credData)
	if err != nil {
		return TefINTERNAL
	}

	// Check if already accepted
	// Reference: rippled Credentials.cpp preclaim lines 350-355
	if cred.IsAccepted() {
		return TecDUPLICATE
	}

	// Check subject has sufficient reserve
	// Reference: rippled Credentials.cpp doApply lines 373-377
	reserve := e.AccountReserve(account.OwnerCount + 1)
	if account.Balance < reserve {
		return TecINSUFFICIENT_RESERVE
	}

	// Check if credential has expired
	// Reference: rippled Credentials.cpp doApply lines 384-389
	closeTime := e.config.ParentCloseTime
	if checkCredentialExpired(cred, closeTime) {
		// Delete expired credential and return error
		if err := e.view.Erase(credKeylet); err == nil {
			// Decrement issuer's owner count since credential wasn't accepted
			if issuerAccount.OwnerCount > 0 {
				issuerAccount.OwnerCount--
			}
			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "DeletedNode",
				LedgerEntryType: "Credential",
				LedgerIndex:     hex.EncodeToString(credKeylet.Key[:]),
			})
		}
		return TecEXPIRED
	}

	// Set accepted flag
	// Reference: rippled Credentials.cpp doApply lines 392-393
	cred.SetAccepted()

	// Serialize and update the credential
	updatedData, err := serializeCredentialEntry(cred)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(credKeylet, updatedData); err != nil {
		return TefINTERNAL
	}

	// Transfer owner count from issuer to subject
	// Reference: rippled Credentials.cpp doApply lines 395-396
	if issuerAccount.OwnerCount > 0 {
		issuerAccount.OwnerCount--
	}
	account.OwnerCount++

	// Update issuer account
	issuerUpdatedData, err := serializeAccountRoot(issuerAccount)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(issuerKeylet, issuerUpdatedData); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "Credential",
		LedgerIndex:     hex.EncodeToString(credKeylet.Key[:]),
		FinalFields: map[string]any{
			"Flags": LsfCredentialAccepted,
		},
	})

	return TesSUCCESS
}

// applyCredentialDelete applies a CredentialDelete transaction
// Reference: rippled Credentials.cpp CredentialDelete::doApply()
func (e *Engine) applyCredentialDelete(tx *CredentialDelete, account *AccountRoot, metadata *Metadata) Result {
	if tx.CredentialType == "" {
		return TemINVALID
	}

	accountID, _ := decodeAccountID(tx.Account)

	// Determine subject and issuer
	// Reference: rippled Credentials.cpp doApply lines 271-272
	var subjectID, issuerID [20]byte
	if tx.Subject != "" {
		subjectID, _ = decodeAccountID(tx.Subject)
	} else {
		subjectID = accountID
	}
	if tx.Issuer != "" {
		issuerID, _ = decodeAccountID(tx.Issuer)
	} else {
		issuerID = accountID
	}

	// Decode credential type from hex
	credType, err := hex.DecodeString(tx.CredentialType)
	if err != nil {
		return TemINVALID
	}

	// Compute credential keylet
	credKeylet := keylet.Credential(subjectID, issuerID, credType)

	// Read the credential entry
	// Reference: rippled Credentials.cpp preclaim - check credential exists
	credData, err := e.view.Read(credKeylet)
	if err != nil {
		return TecNO_ENTRY
	}

	cred, err := parseCredentialEntry(credData)
	if err != nil {
		return TefINTERNAL
	}

	// Check permission: must be subject or issuer, or credential must be expired
	// Reference: rippled Credentials.cpp doApply lines 280-284
	closeTime := e.config.ParentCloseTime
	isSubject := accountID == subjectID
	isIssuer := accountID == issuerID
	isExpired := checkCredentialExpired(cred, closeTime)

	if !isSubject && !isIssuer && !isExpired {
		return TecNO_PERMISSION
	}

	// Delete the credential using the helper function logic
	// Reference: rippled CredentialHelpers.cpp deleteSLE()

	// Determine who owns the credential (for owner count adjustment)
	// Reference: rippled CredentialHelpers.cpp deleteSLE lines 101-114
	accepted := cred.IsAccepted()

	// Adjust owner counts
	if subjectID == issuerID {
		// Self-issued: only issuer owns it
		if account.OwnerCount > 0 {
			account.OwnerCount--
		}
	} else {
		// Different subject and issuer
		if accepted {
			// Accepted: subject owns it
			if isSubject {
				if account.OwnerCount > 0 {
					account.OwnerCount--
				}
			} else {
				// Need to decrement subject's owner count
				subjectKeylet := keylet.Account(subjectID)
				subjectData, err := e.view.Read(subjectKeylet)
				if err == nil {
					subjectAccount, _ := parseAccountRoot(subjectData)
					if subjectAccount != nil && subjectAccount.OwnerCount > 0 {
						subjectAccount.OwnerCount--
						if updatedData, err := serializeAccountRoot(subjectAccount); err == nil {
							e.view.Update(subjectKeylet, updatedData)
						}
					}
				}
			}
		} else {
			// Not accepted: issuer owns it
			if isIssuer {
				if account.OwnerCount > 0 {
					account.OwnerCount--
				}
			} else {
				// Need to decrement issuer's owner count
				issuerKeylet := keylet.Account(issuerID)
				issuerData, err := e.view.Read(issuerKeylet)
				if err == nil {
					issuerAccount, _ := parseAccountRoot(issuerData)
					if issuerAccount != nil && issuerAccount.OwnerCount > 0 {
						issuerAccount.OwnerCount--
						if updatedData, err := serializeAccountRoot(issuerAccount); err == nil {
							e.view.Update(issuerKeylet, updatedData)
						}
					}
				}
			}
		}
	}

	// Delete the credential entry
	if err := e.view.Erase(credKeylet); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Credential",
		LedgerIndex:     hex.EncodeToString(credKeylet.Key[:]),
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
