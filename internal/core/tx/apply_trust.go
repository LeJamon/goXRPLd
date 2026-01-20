package tx

import (
	"encoding/hex"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// applyTrustSet applies a TrustSet transaction.
// TrustSet creates or modifies a trust line (RippleState object) between two accounts.
func (e *Engine) applyTrustSet(trustSet *TrustSet, account *AccountRoot, metadata *Metadata) Result {
	// Cannot create trust line to self
	if trustSet.LimitAmount.Issuer == account.Account {
		return TemDST_IS_SRC
	}

	// Get the issuer account ID
	issuerAccountID, err := decodeAccountID(trustSet.LimitAmount.Issuer)
	if err != nil {
		return TemBAD_ISSUER
	}
	issuerKey := keylet.Account(issuerAccountID)

	// Check issuer exists and get issuer account for flag checks
	issuerData, err := e.view.Read(issuerKey)
	if err != nil {
		return TecNO_ISSUER
	}
	issuerAccount, err := parseAccountRoot(issuerData)
	if err != nil {
		return TefINTERNAL
	}

	// Get the account ID
	accountID, _ := decodeAccountID(account.Account)

	// Determine low/high accounts (for consistent trust line ordering)
	// bHigh = true means current account is the HIGH account
	bHigh := compareAccountIDsForLine(accountID, issuerAccountID) > 0

	// Get or create the trust line
	trustLineKey := keylet.Line(accountID, issuerAccountID, trustSet.LimitAmount.Currency)

	trustLineExists, err := e.view.Exists(trustLineKey)
	if err != nil {
		return TefINTERNAL
	}

	// Parse transaction flags
	txFlags := uint32(0)
	if trustSet.Flags != nil {
		txFlags = *trustSet.Flags
	}

	bSetAuth := (txFlags & TrustSetFlagSetfAuth) != 0
	bSetNoRipple := (txFlags & TrustSetFlagSetNoRipple) != 0
	bClearNoRipple := (txFlags & TrustSetFlagClearNoRipple) != 0
	bSetFreeze := (txFlags & TrustSetFlagSetFreeze) != 0
	bClearFreeze := (txFlags & TrustSetFlagClearFreeze) != 0

	// Validate tfSetfAuth - requires issuer to have lsfRequireAuth set
	if bSetAuth && (account.Flags&lsfRequireAuth) == 0 {
		return TefNO_AUTH_REQUIRED
	}

	// Validate freeze flags - cannot freeze if account has lsfNoFreeze set
	bNoFreeze := (account.Flags & lsfNoFreeze) != 0
	if bNoFreeze && bSetFreeze {
		return TecNO_PERMISSION
	}

	// Parse quality values from transaction
	const qualityOne uint32 = 1000000000
	var uQualityIn, uQualityOut uint32
	bQualityIn := trustSet.QualityIn != nil
	bQualityOut := trustSet.QualityOut != nil

	if bQualityIn {
		uQualityIn = *trustSet.QualityIn
		if uQualityIn == qualityOne {
			uQualityIn = 0 // Normalize to default
		}
	}
	if bQualityOut {
		uQualityOut = *trustSet.QualityOut
		if uQualityOut == qualityOne {
			uQualityOut = 0 // Normalize to default
		}
	}

	// Parse the limit amount
	limitAmount := NewIOUAmount(trustSet.LimitAmount.Value, trustSet.LimitAmount.Currency, trustSet.LimitAmount.Issuer)

	if !trustLineExists {
		return e.applyTrustSetCreate(trustSet, account, limitAmount, bHigh, bSetAuth, bSetNoRipple, bClearNoRipple, bSetFreeze, bClearFreeze, bNoFreeze, bQualityIn, bQualityOut, uQualityIn, uQualityOut, trustLineKey, metadata)
	}

	return e.applyTrustSetModify(trustSet, account, issuerAccount, limitAmount, bHigh, bSetAuth, bSetNoRipple, bClearNoRipple, bSetFreeze, bClearFreeze, bNoFreeze, bQualityIn, bQualityOut, uQualityIn, uQualityOut, trustLineKey, metadata)
}

// applyTrustSetCreate creates a new trust line
func (e *Engine) applyTrustSetCreate(trustSet *TrustSet, account *AccountRoot, limitAmount IOUAmount,
	bHigh, bSetAuth, bSetNoRipple, bClearNoRipple, bSetFreeze, bClearFreeze, bNoFreeze bool,
	bQualityIn, bQualityOut bool, uQualityIn, uQualityOut uint32,
	trustLineKey keylet.Keylet, metadata *Metadata) Result {

	// Check if setting zero limit without existing trust line
	if limitAmount.IsZero() && !bSetAuth && (!bQualityIn || uQualityIn == 0) && (!bQualityOut || uQualityOut == 0) {
		return TesSUCCESS
	}

	// Check account has reserve for new trust line
	reserveCreate := e.ReserveForNewObject(account.OwnerCount)
	if account.Balance < reserveCreate {
		return TecINSUF_RESERVE_LINE
	}

	// Create new RippleState
	rs := &RippleState{
		Balance:  NewIOUAmount("0", trustSet.LimitAmount.Currency, trustSet.LimitAmount.Issuer),
		Flags:    0,
		LowNode:  0,
		HighNode: 0,
	}

	// Set the limit based on which side this account is
	if !bHigh {
		rs.LowLimit = limitAmount
		rs.HighLimit = NewIOUAmount("0", trustSet.LimitAmount.Currency, account.Account)
		rs.Flags |= lsfLowReserve
	} else {
		rs.LowLimit = NewIOUAmount("0", trustSet.LimitAmount.Currency, trustSet.LimitAmount.Issuer)
		rs.HighLimit = limitAmount
		rs.Flags |= lsfHighReserve
	}

	// Handle Auth flag
	if bSetAuth {
		if bHigh {
			rs.Flags |= lsfHighAuth
		} else {
			rs.Flags |= lsfLowAuth
		}
	}

	// Handle NoRipple flag
	if bSetNoRipple && !bClearNoRipple {
		if bHigh {
			rs.Flags |= lsfHighNoRipple
		} else {
			rs.Flags |= lsfLowNoRipple
		}
	}

	// Handle Freeze flag
	if bSetFreeze && !bClearFreeze && !bNoFreeze {
		if bHigh {
			rs.Flags |= lsfHighFreeze
		} else {
			rs.Flags |= lsfLowFreeze
		}
	}

	// Handle Quality values
	if bQualityIn && uQualityIn != 0 {
		if bHigh {
			rs.HighQualityIn = uQualityIn
		} else {
			rs.LowQualityIn = uQualityIn
		}
	}
	if bQualityOut && uQualityOut != 0 {
		if bHigh {
			rs.HighQualityOut = uQualityOut
		} else {
			rs.LowQualityOut = uQualityOut
		}
	}

	// Serialize and insert the trust line
	trustLineData, err := serializeRippleState(rs)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Insert(trustLineKey, trustLineData); err != nil {
		return TefINTERNAL
	}

	// Increment owner count
	account.OwnerCount++

	// Build metadata
	newFields := map[string]any{
		"Balance": map[string]any{
			"currency": trustSet.LimitAmount.Currency,
			"issuer":   "rrrrrrrrrrrrrrrrrrrrBZbvji",
			"value":    "0",
		},
		"Flags": rs.Flags,
	}
	if !bHigh {
		newFields["LowLimit"] = map[string]any{
			"currency": trustSet.LimitAmount.Currency,
			"issuer":   account.Account,
			"value":    formatIOUValue(rs.LowLimit.Value),
		}
		newFields["HighLimit"] = map[string]any{
			"currency": trustSet.LimitAmount.Currency,
			"issuer":   trustSet.LimitAmount.Issuer,
			"value":    "0",
		}
	} else {
		newFields["LowLimit"] = map[string]any{
			"currency": trustSet.LimitAmount.Currency,
			"issuer":   trustSet.LimitAmount.Issuer,
			"value":    "0",
		}
		newFields["HighLimit"] = map[string]any{
			"currency": trustSet.LimitAmount.Currency,
			"issuer":   account.Account,
			"value":    formatIOUValue(rs.HighLimit.Value),
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "RippleState",
		LedgerIndex:     hex.EncodeToString(trustLineKey.Key[:]),
		NewFields:       newFields,
	})

	return TesSUCCESS
}

// applyTrustSetModify modifies an existing trust line
func (e *Engine) applyTrustSetModify(trustSet *TrustSet, account, issuerAccount *AccountRoot, limitAmount IOUAmount,
	bHigh, bSetAuth, bSetNoRipple, bClearNoRipple, bSetFreeze, bClearFreeze, bNoFreeze bool,
	bQualityIn, bQualityOut bool, uQualityIn, uQualityOut uint32,
	trustLineKey keylet.Keylet, metadata *Metadata) Result {

	const qualityOne uint32 = 1000000000

	trustLineData, err := e.view.Read(trustLineKey)
	if err != nil {
		return TefINTERNAL
	}

	rs, err := parseRippleState(trustLineData)
	if err != nil {
		return TefINTERNAL
	}

	// Store previous values for metadata
	previousFlags := rs.Flags
	previousLimit := rs.LowLimit
	if bHigh {
		previousLimit = rs.HighLimit
	}

	// Update the limit
	if !bHigh {
		rs.LowLimit = limitAmount
	} else {
		rs.HighLimit = limitAmount
	}

	// Handle Auth flag
	if bSetAuth {
		if bHigh {
			rs.Flags |= lsfHighAuth
		} else {
			rs.Flags |= lsfLowAuth
		}
	}

	// Handle NoRipple flag
	if bSetNoRipple && !bClearNoRipple {
		if bHigh {
			rs.Flags |= lsfHighNoRipple
		} else {
			rs.Flags |= lsfLowNoRipple
		}
	} else if bClearNoRipple && !bSetNoRipple {
		if bHigh {
			rs.Flags &^= lsfHighNoRipple
		} else {
			rs.Flags &^= lsfLowNoRipple
		}
	}

	// Handle Freeze flag
	if bSetFreeze && !bClearFreeze && !bNoFreeze {
		if bHigh {
			rs.Flags |= lsfHighFreeze
		} else {
			rs.Flags |= lsfLowFreeze
		}
	} else if bClearFreeze && !bSetFreeze {
		if bHigh {
			rs.Flags &^= lsfHighFreeze
		} else {
			rs.Flags &^= lsfLowFreeze
		}
	}

	// Handle Quality values
	if bQualityIn {
		if bHigh {
			rs.HighQualityIn = uQualityIn
		} else {
			rs.LowQualityIn = uQualityIn
		}
	}
	if bQualityOut {
		if bHigh {
			rs.HighQualityOut = uQualityOut
		} else {
			rs.LowQualityOut = uQualityOut
		}
	}

	// Normalize quality values
	if rs.LowQualityIn == qualityOne {
		rs.LowQualityIn = 0
	}
	if rs.LowQualityOut == qualityOne {
		rs.LowQualityOut = 0
	}
	if rs.HighQualityIn == qualityOne {
		rs.HighQualityIn = 0
	}
	if rs.HighQualityOut == qualityOne {
		rs.HighQualityOut = 0
	}

	// Check if trust line should be deleted
	bLowDefRipple := (issuerAccount.Flags & lsfDefaultRipple) != 0
	bHighDefRipple := (account.Flags & lsfDefaultRipple) != 0
	if bHigh {
		bLowDefRipple = (issuerAccount.Flags & lsfDefaultRipple) != 0
		bHighDefRipple = (account.Flags & lsfDefaultRipple) != 0
	} else {
		bLowDefRipple = (account.Flags & lsfDefaultRipple) != 0
		bHighDefRipple = (issuerAccount.Flags & lsfDefaultRipple) != 0
	}

	bLowReserveSet := rs.LowQualityIn != 0 || rs.LowQualityOut != 0 ||
		((rs.Flags&lsfLowNoRipple) == 0) != bLowDefRipple ||
		(rs.Flags&lsfLowFreeze) != 0 || !rs.LowLimit.IsZero() ||
		(rs.Balance.Value != nil && rs.Balance.Value.Sign() > 0)

	bHighReserveSet := rs.HighQualityIn != 0 || rs.HighQualityOut != 0 ||
		((rs.Flags&lsfHighNoRipple) == 0) != bHighDefRipple ||
		(rs.Flags&lsfHighFreeze) != 0 || !rs.HighLimit.IsZero() ||
		(rs.Balance.Value != nil && rs.Balance.Value.Sign() < 0)

	bDefault := !bLowReserveSet && !bHighReserveSet

	if bDefault && rs.Balance.IsZero() {
		// Delete the trust line
		if err := e.view.Erase(trustLineKey); err != nil {
			return TefINTERNAL
		}

		if account.OwnerCount > 0 {
			account.OwnerCount--
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "DeletedNode",
			LedgerEntryType: "RippleState",
			LedgerIndex:     hex.EncodeToString(trustLineKey.Key[:]),
		})
	} else {
		// Update reserve flags
		if bLowReserveSet && (rs.Flags&lsfLowReserve) == 0 {
			rs.Flags |= lsfLowReserve
		} else if !bLowReserveSet && (rs.Flags&lsfLowReserve) != 0 {
			rs.Flags &^= lsfLowReserve
		}

		if bHighReserveSet && (rs.Flags&lsfHighReserve) == 0 {
			rs.Flags |= lsfHighReserve
		} else if !bHighReserveSet && (rs.Flags&lsfHighReserve) != 0 {
			rs.Flags &^= lsfHighReserve
		}

		// Update the trust line
		updatedData, err := serializeRippleState(rs)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Update(trustLineKey, updatedData); err != nil {
			return TefINTERNAL
		}

		// Build metadata
		finalFields := map[string]any{"Flags": rs.Flags}
		previousFields := map[string]any{}

		if formatIOUValue(limitAmount.Value) != formatIOUValue(previousLimit.Value) {
			limitField := "LowLimit"
			if bHigh {
				limitField = "HighLimit"
			}
			finalFields[limitField] = map[string]any{
				"currency": trustSet.LimitAmount.Currency,
				"issuer":   account.Account,
				"value":    formatIOUValue(limitAmount.Value),
			}
			previousFields[limitField] = map[string]any{
				"currency": trustSet.LimitAmount.Currency,
				"issuer":   account.Account,
				"value":    formatIOUValue(previousLimit.Value),
			}
		}

		if previousFlags != rs.Flags {
			previousFields["Flags"] = previousFlags
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "RippleState",
			LedgerIndex:     hex.EncodeToString(trustLineKey.Key[:]),
			FinalFields:     finalFields,
			PreviousFields:  previousFields,
		})
	}

	return TesSUCCESS
}
