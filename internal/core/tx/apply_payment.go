package tx

import (
	"encoding/hex"
	"math/big"
	"strconv"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// applyPayment applies a Payment transaction
func (e *Engine) applyPayment(payment *Payment, sender *AccountRoot, metadata *Metadata) Result {
	// XRP-to-XRP payment (direct payment)
	if payment.Amount.IsNative() {
		return e.applyXRPPayment(payment, sender, metadata)
	}

	// IOU payment - more complex, involves trust lines and paths
	return e.applyIOUPayment(payment, sender, metadata)
}

// applyXRPPayment applies an XRP-to-XRP payment
func (e *Engine) applyXRPPayment(payment *Payment, sender *AccountRoot, metadata *Metadata) Result {
	// Parse the amount
	amountDrops, err := strconv.ParseUint(payment.Amount.Value, 10, 64)
	if err != nil || amountDrops == 0 {
		return TemBAD_AMOUNT
	}

	// Check sender has enough balance (including reserve)
	requiredBalance := amountDrops + e.config.ReserveBase
	if sender.Balance < requiredBalance {
		return TecUNFUNDED_PAYMENT
	}

	// Get destination account
	destAccountID, err := decodeAccountID(payment.Destination)
	if err != nil {
		return TemDST_NEEDED
	}
	destKey := keylet.Account(destAccountID)

	destExists, err := e.view.Exists(destKey)
	if err != nil {
		return TefINTERNAL
	}

	if destExists {
		// Destination exists - just credit the amount
		destData, err := e.view.Read(destKey)
		if err != nil {
			return TefINTERNAL
		}

		destAccount, err := parseAccountRoot(destData)
		if err != nil {
			return TefINTERNAL
		}

		previousDestBalance := destAccount.Balance

		// Check if destination requires a tag
		if (destAccount.Flags&0x00020000) != 0 && payment.DestinationTag == nil {
			return TecDST_TAG_NEEDED
		}

		// Credit destination
		destAccount.Balance += amountDrops

		// Debit sender
		sender.Balance -= amountDrops

		// Update destination
		updatedDestData, err := serializeAccountRoot(destAccount)
		if err != nil {
			return TefINTERNAL
		}

		if err := e.view.Update(destKey, updatedDestData); err != nil {
			return TefINTERNAL
		}

		// Record destination modification
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "AccountRoot",
			LedgerIndex:     hex.EncodeToString(destKey.Key[:]),
			FinalFields: map[string]any{
				"Account": payment.Destination,
				"Balance": strconv.FormatUint(destAccount.Balance, 10),
			},
			PreviousFields: map[string]any{
				"Balance": strconv.FormatUint(previousDestBalance, 10),
			},
		})

		// Set delivered amount in metadata
		delivered := payment.Amount
		metadata.DeliveredAmount = &delivered

		return TesSUCCESS
	}

	// Destination doesn't exist - need to create it
	// Check minimum amount for account creation
	if amountDrops < e.config.ReserveBase {
		return TecNO_DST_INSUF_XRP
	}

	// Create new account
	newAccount := &AccountRoot{
		Account:  payment.Destination,
		Balance:  amountDrops,
		Sequence: 1,
		Flags:    0,
	}

	// Debit sender
	sender.Balance -= amountDrops

	// Serialize and insert new account
	newAccountData, err := serializeAccountRoot(newAccount)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Insert(destKey, newAccountData); err != nil {
		return TefINTERNAL
	}

	// Record account creation
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "AccountRoot",
		LedgerIndex:     hex.EncodeToString(destKey.Key[:]),
		NewFields: map[string]any{
			"Account":  payment.Destination,
			"Balance":  strconv.FormatUint(amountDrops, 10),
			"Sequence": uint32(1),
		},
	})

	// Set delivered amount
	delivered := payment.Amount
	metadata.DeliveredAmount = &delivered

	return TesSUCCESS
}

// applyIOUPayment applies an IOU (issued currency) payment
func (e *Engine) applyIOUPayment(payment *Payment, sender *AccountRoot, metadata *Metadata) Result {
	// Parse the amount
	amount := NewIOUAmount(payment.Amount.Value, payment.Amount.Currency, payment.Amount.Issuer)
	if amount.IsZero() {
		return TemBAD_AMOUNT
	}
	if amount.IsNegative() {
		return TemBAD_AMOUNT
	}

	// Get account IDs
	senderAccountID, err := decodeAccountID(sender.Account)
	if err != nil {
		return TefINTERNAL
	}

	destAccountID, err := decodeAccountID(payment.Destination)
	if err != nil {
		return TemDST_NEEDED
	}

	issuerAccountID, err := decodeAccountID(payment.Amount.Issuer)
	if err != nil {
		return TemBAD_ISSUER
	}

	// Check destination exists
	destKey := keylet.Account(destAccountID)
	destExists, err := e.view.Exists(destKey)
	if err != nil {
		return TefINTERNAL
	}
	if !destExists {
		return TecNO_DST
	}

	// Get destination account to check flags
	destData, err := e.view.Read(destKey)
	if err != nil {
		return TefINTERNAL
	}
	destAccount, err := parseAccountRoot(destData)
	if err != nil {
		return TefINTERNAL
	}

	// Check destination tag requirement
	if (destAccount.Flags&0x00020000) != 0 && payment.DestinationTag == nil {
		return TecDST_TAG_NEEDED
	}

	// Handle three cases:
	// 1. Sender is issuer - creating new tokens
	// 2. Destination is issuer - redeeming tokens
	// 3. Neither - transfer between accounts via trust lines

	senderIsIssuer := senderAccountID == issuerAccountID
	destIsIssuer := destAccountID == issuerAccountID

	if senderIsIssuer {
		// Sender is issuing their own currency to destination
		// Need trust line from destination to sender (issuer)
		return e.applyIOUIssue(payment, sender, destAccount, senderAccountID, destAccountID, amount, metadata)
	} else if destIsIssuer {
		// Destination is the issuer - sender is redeeming tokens
		// Need trust line from sender to destination (issuer)
		return e.applyIOURedeem(payment, sender, destAccount, senderAccountID, destAccountID, amount, metadata)
	} else {
		// Neither is issuer - transfer between two non-issuer accounts
		// This requires trust lines from both parties to the issuer
		return e.applyIOUTransfer(payment, sender, destAccount, senderAccountID, destAccountID, issuerAccountID, amount, metadata)
	}
}

// applyIOUIssue handles when sender is the issuer creating new tokens
func (e *Engine) applyIOUIssue(payment *Payment, sender *AccountRoot, dest *AccountRoot, senderID, destID [20]byte, amount IOUAmount, metadata *Metadata) Result {
	// Look up the trust line between destination and issuer (sender)
	trustLineKey := keylet.Line(destID, senderID, amount.Currency)

	trustLineExists, err := e.view.Exists(trustLineKey)
	if err != nil {
		return TefINTERNAL
	}

	if !trustLineExists {
		// No trust line exists - destination has not authorized holding this currency
		return TecPATH_DRY
	}

	// Read and parse the trust line
	trustLineData, err := e.view.Read(trustLineKey)
	if err != nil {
		return TefINTERNAL
	}

	rippleState, err := parseRippleState(trustLineData)
	if err != nil {
		return TefINTERNAL
	}

	// Determine which side is low/high account
	destIsLow := compareAccountIDsForLine(destID, senderID) < 0

	// Get the trust limit set by the destination (recipient)
	var trustLimit IOUAmount
	if destIsLow {
		trustLimit = rippleState.LowLimit
	} else {
		trustLimit = rippleState.HighLimit
	}

	// Calculate new balance after adding the amount
	// Balance is positive if low account owes high account
	// Balance is negative if high account owes low account
	var newBalance IOUAmount
	if destIsLow {
		// Dest is low, sender (issuer) is high
		// Issuing means low account gets positive balance (issuer owes them)
		// So we need to make balance more negative (high owes low)
		newBalance = rippleState.Balance.Sub(amount)
	} else {
		// Dest is high, sender (issuer) is low
		// Issuing means high account gets positive balance
		// So balance becomes more positive (low owes high)
		newBalance = rippleState.Balance.Add(amount)
	}

	// Check if the new balance exceeds the trust limit
	absNewBalance := newBalance
	if absNewBalance.IsNegative() {
		absNewBalance = absNewBalance.Negate()
	}

	// The trust limit applies to the absolute balance
	if !trustLimit.IsZero() && absNewBalance.Compare(trustLimit) > 0 {
		return TecPATH_PARTIAL
	}

	// Update the trust line
	rippleState.Balance = newBalance

	// Serialize and update
	updatedTrustLine, err := serializeRippleState(rippleState)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(trustLineKey, updatedTrustLine); err != nil {
		return TefINTERNAL
	}

	// Record the modification
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "RippleState",
		LedgerIndex:     hex.EncodeToString(trustLineKey.Key[:]),
		FinalFields: map[string]any{
			"Balance": map[string]any{
				"currency": amount.Currency,
				"issuer":   amount.Issuer,
				"value":    newBalance.Value.Text('f', 15),
			},
		},
	})

	delivered := payment.Amount
	metadata.DeliveredAmount = &delivered

	return TesSUCCESS
}

// applyIOURedeem handles when destination is the issuer (redeeming tokens)
func (e *Engine) applyIOURedeem(payment *Payment, sender *AccountRoot, dest *AccountRoot, senderID, destID [20]byte, amount IOUAmount, metadata *Metadata) Result {
	// Look up the trust line between sender and issuer (destination)
	trustLineKey := keylet.Line(senderID, destID, amount.Currency)

	trustLineExists, err := e.view.Exists(trustLineKey)
	if err != nil {
		return TefINTERNAL
	}

	if !trustLineExists {
		// No trust line exists - sender doesn't hold this currency
		return TecPATH_DRY
	}

	// Read and parse the trust line
	trustLineData, err := e.view.Read(trustLineKey)
	if err != nil {
		return TefINTERNAL
	}

	rippleState, err := parseRippleState(trustLineData)
	if err != nil {
		return TefINTERNAL
	}

	// Determine which side is low/high account
	senderIsLow := compareAccountIDsForLine(senderID, destID) < 0

	// Get sender's current balance (how much issuer owes them)
	var senderBalance IOUAmount
	if senderIsLow {
		// Sender is low, issuer (dest) is high
		// Balance is positive if low owes high
		// Sender's holding is the negative of the balance
		senderBalance = rippleState.Balance.Negate()
	} else {
		// Sender is high, issuer (dest) is low
		// Sender's holding is the balance itself
		senderBalance = rippleState.Balance
	}

	// Check sender has enough balance
	if senderBalance.Compare(amount) < 0 {
		return TecPATH_PARTIAL
	}

	// Update balance by reducing sender's holding
	var newBalance IOUAmount
	if senderIsLow {
		// Reduce the amount issuer owes sender (make balance less negative / more positive)
		newBalance = rippleState.Balance.Add(amount)
	} else {
		// Reduce sender's claim on issuer (make balance less positive / more negative)
		newBalance = rippleState.Balance.Sub(amount)
	}

	rippleState.Balance = newBalance

	// Serialize and update
	updatedTrustLine, err := serializeRippleState(rippleState)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(trustLineKey, updatedTrustLine); err != nil {
		return TefINTERNAL
	}

	// Record the modification
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "RippleState",
		LedgerIndex:     hex.EncodeToString(trustLineKey.Key[:]),
		FinalFields: map[string]any{
			"Balance": map[string]any{
				"currency": amount.Currency,
				"issuer":   amount.Issuer,
				"value":    newBalance.Value.Text('f', 15),
			},
		},
	})

	delivered := payment.Amount
	metadata.DeliveredAmount = &delivered

	return TesSUCCESS
}

// applyIOUTransfer handles transfer between two non-issuer accounts
func (e *Engine) applyIOUTransfer(payment *Payment, sender *AccountRoot, dest *AccountRoot, senderID, destID, issuerID [20]byte, amount IOUAmount, metadata *Metadata) Result {
	// Both sender and destination need trust lines to the issuer
	// This is a simplified implementation - full path finding is more complex

	// Get sender's trust line to issuer
	senderTrustLineKey := keylet.Line(senderID, issuerID, amount.Currency)
	senderTrustExists, err := e.view.Exists(senderTrustLineKey)
	if err != nil {
		return TefINTERNAL
	}
	if !senderTrustExists {
		return TecPATH_DRY
	}

	// Get destination's trust line to issuer
	destTrustLineKey := keylet.Line(destID, issuerID, amount.Currency)
	destTrustExists, err := e.view.Exists(destTrustLineKey)
	if err != nil {
		return TefINTERNAL
	}
	if !destTrustExists {
		return TecPATH_DRY
	}

	// Read sender's trust line
	senderTrustData, err := e.view.Read(senderTrustLineKey)
	if err != nil {
		return TefINTERNAL
	}
	senderRippleState, err := parseRippleState(senderTrustData)
	if err != nil {
		return TefINTERNAL
	}

	// Read destination's trust line
	destTrustData, err := e.view.Read(destTrustLineKey)
	if err != nil {
		return TefINTERNAL
	}
	destRippleState, err := parseRippleState(destTrustData)
	if err != nil {
		return TefINTERNAL
	}

	// Calculate sender's balance with issuer
	senderIsLowWithIssuer := compareAccountIDsForLine(senderID, issuerID) < 0
	var senderBalance IOUAmount
	if senderIsLowWithIssuer {
		senderBalance = senderRippleState.Balance.Negate()
	} else {
		senderBalance = senderRippleState.Balance
	}

	// Check sender has enough
	if senderBalance.Compare(amount) < 0 {
		return TecPATH_PARTIAL
	}

	// Calculate destination's current balance and trust limit
	destIsLowWithIssuer := compareAccountIDsForLine(destID, issuerID) < 0
	var destBalance, destLimit IOUAmount
	if destIsLowWithIssuer {
		destBalance = destRippleState.Balance.Negate()
		destLimit = destRippleState.LowLimit
	} else {
		destBalance = destRippleState.Balance
		destLimit = destRippleState.HighLimit
	}

	// Check destination trust limit
	newDestBalance := destBalance.Add(amount)
	if !destLimit.IsZero() && newDestBalance.Compare(destLimit) > 0 {
		return TecPATH_PARTIAL
	}

	// Update sender's trust line (decrease balance)
	var newSenderRippleBalance IOUAmount
	if senderIsLowWithIssuer {
		newSenderRippleBalance = senderRippleState.Balance.Add(amount)
	} else {
		newSenderRippleBalance = senderRippleState.Balance.Sub(amount)
	}
	senderRippleState.Balance = newSenderRippleBalance

	// Update destination's trust line (increase balance)
	var newDestRippleBalance IOUAmount
	if destIsLowWithIssuer {
		newDestRippleBalance = destRippleState.Balance.Sub(amount)
	} else {
		newDestRippleBalance = destRippleState.Balance.Add(amount)
	}
	destRippleState.Balance = newDestRippleBalance

	// Serialize and update sender's trust line
	updatedSenderTrust, err := serializeRippleState(senderRippleState)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(senderTrustLineKey, updatedSenderTrust); err != nil {
		return TefINTERNAL
	}

	// Serialize and update destination's trust line
	updatedDestTrust, err := serializeRippleState(destRippleState)
	if err != nil {
		return TefINTERNAL
	}
	if err := e.view.Update(destTrustLineKey, updatedDestTrust); err != nil {
		return TefINTERNAL
	}

	// Record the modifications
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "RippleState",
		LedgerIndex:     hex.EncodeToString(senderTrustLineKey.Key[:]),
		FinalFields: map[string]any{
			"Balance": map[string]any{
				"currency": amount.Currency,
				"issuer":   amount.Issuer,
				"value":    newSenderRippleBalance.Value.Text('f', 15),
			},
		},
	})

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "RippleState",
		LedgerIndex:     hex.EncodeToString(destTrustLineKey.Key[:]),
		FinalFields: map[string]any{
			"Balance": map[string]any{
				"currency": amount.Currency,
				"issuer":   amount.Issuer,
				"value":    newDestRippleBalance.Value.Text('f', 15),
			},
		},
	})

	delivered := payment.Amount
	metadata.DeliveredAmount = &delivered

	return TesSUCCESS
}

// compareAccountIDsForLine compares account IDs for trust line ordering
func compareAccountIDsForLine(a, b [20]byte) int {
	for i := 0; i < 20; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// applyAccountSet applies an AccountSet transaction
func (e *Engine) applyAccountSet(accountSet *AccountSet, account *AccountRoot, metadata *Metadata) Result {
	// Apply flag changes
	if accountSet.SetFlag != nil {
		switch *accountSet.SetFlag {
		case AccountSetFlagRequireDest:
			account.Flags |= 0x00020000 // lsfRequireDestTag
		case AccountSetFlagRequireAuth:
			account.Flags |= 0x00040000 // lsfRequireAuth
		case AccountSetFlagDisallowXRP:
			account.Flags |= 0x00080000 // lsfDisallowXRP
		case AccountSetFlagDisableMaster:
			// Need to check RegularKey exists
			if account.RegularKey == "" {
				return TecNO_ALTERNATIVE_KEY
			}
			account.Flags |= 0x00100000 // lsfDisableMaster
		case AccountSetFlagDefaultRipple:
			account.Flags |= 0x00800000 // lsfDefaultRipple
		case AccountSetFlagDepositAuth:
			account.Flags |= 0x01000000 // lsfDepositAuth
		}
	}

	if accountSet.ClearFlag != nil {
		switch *accountSet.ClearFlag {
		case AccountSetFlagRequireDest:
			account.Flags &^= 0x00020000
		case AccountSetFlagRequireAuth:
			account.Flags &^= 0x00040000
		case AccountSetFlagDisallowXRP:
			account.Flags &^= 0x00080000
		case AccountSetFlagDisableMaster:
			account.Flags &^= 0x00100000
		case AccountSetFlagDefaultRipple:
			account.Flags &^= 0x00800000
		case AccountSetFlagDepositAuth:
			account.Flags &^= 0x01000000
		}
	}

	// Apply other settings
	if accountSet.Domain != "" {
		account.Domain = accountSet.Domain
	}

	if accountSet.EmailHash != "" {
		account.EmailHash = accountSet.EmailHash
	}

	if accountSet.MessageKey != "" {
		account.MessageKey = accountSet.MessageKey
	}

	if accountSet.TransferRate != nil {
		account.TransferRate = *accountSet.TransferRate
	}

	if accountSet.TickSize != nil {
		account.TickSize = *accountSet.TickSize
	}

	return TesSUCCESS
}

// applyTrustSet applies a TrustSet transaction
func (e *Engine) applyTrustSet(trustSet *TrustSet, account *AccountRoot, metadata *Metadata) Result {
	// TrustSet creates or modifies a trust line (RippleState object)

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

	// Check issuer exists
	issuerExists, err := e.view.Exists(issuerKey)
	if err != nil {
		return TefINTERNAL
	}
	if !issuerExists {
		return TecNO_ISSUER
	}

	// Get the account ID
	accountID, _ := decodeAccountID(account.Account)

	// Determine low/high accounts (for consistent trust line ordering)
	isLowAccount := compareAccountIDsForLine(accountID, issuerAccountID) < 0

	// Get or create the trust line
	trustLineKey := keylet.Line(accountID, issuerAccountID, trustSet.LimitAmount.Currency)

	trustLineExists, err := e.view.Exists(trustLineKey)
	if err != nil {
		return TefINTERNAL
	}

	// Parse the limit amount
	limitAmount := NewIOUAmount(trustSet.LimitAmount.Value, trustSet.LimitAmount.Currency, trustSet.LimitAmount.Issuer)

	if !trustLineExists {
		// Check if setting zero limit without existing trust line
		if limitAmount.IsZero() {
			// Nothing to do - no trust line and setting zero limit
			return TesSUCCESS
		}

		// Check account has reserve for new trust line
		requiredReserve := e.config.ReserveBase + (uint64(account.OwnerCount)+1)*e.config.ReserveIncrement
		if account.Balance < requiredReserve {
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
		if isLowAccount {
			rs.LowLimit = limitAmount
			rs.HighLimit = NewIOUAmount("0", trustSet.LimitAmount.Currency, account.Account)
			// Set the reserve flag for the low account
			rs.Flags |= lsfLowReserve
		} else {
			rs.LowLimit = NewIOUAmount("0", trustSet.LimitAmount.Currency, trustSet.LimitAmount.Issuer)
			rs.HighLimit = limitAmount
			// Set the reserve flag for the high account
			rs.Flags |= lsfHighReserve
		}

		// Handle NoRipple flag from transaction
		if trustSet.Flags != nil {
			if (*trustSet.Flags & 0x00020000) != 0 { // tfSetNoRipple
				if isLowAccount {
					rs.Flags |= lsfLowNoRipple
				} else {
					rs.Flags |= lsfHighNoRipple
				}
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

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "RippleState",
			LedgerIndex:     hex.EncodeToString(trustLineKey.Key[:]),
			NewFields: map[string]any{
				"Balance": map[string]any{
					"currency": trustSet.LimitAmount.Currency,
					"issuer":   trustSet.LimitAmount.Issuer,
					"value":    "0",
				},
				"LowLimit": map[string]any{
					"currency": trustSet.LimitAmount.Currency,
					"issuer":   account.Account,
					"value":    rs.LowLimit.Value.Text('f', 15),
				},
				"HighLimit": map[string]any{
					"currency": trustSet.LimitAmount.Currency,
					"issuer":   trustSet.LimitAmount.Issuer,
					"value":    rs.HighLimit.Value.Text('f', 15),
				},
				"Flags": rs.Flags,
			},
		})
	} else {
		// Modify existing trust line
		trustLineData, err := e.view.Read(trustLineKey)
		if err != nil {
			return TefINTERNAL
		}

		rs, err := parseRippleState(trustLineData)
		if err != nil {
			return TefINTERNAL
		}

		previousLimit := rs.LowLimit
		if !isLowAccount {
			previousLimit = rs.HighLimit
		}

		// Update the limit
		if isLowAccount {
			rs.LowLimit = limitAmount
		} else {
			rs.HighLimit = limitAmount
		}

		// Handle NoRipple flag
		if trustSet.Flags != nil {
			if (*trustSet.Flags & 0x00020000) != 0 { // tfSetNoRipple
				if isLowAccount {
					rs.Flags |= lsfLowNoRipple
				} else {
					rs.Flags |= lsfHighNoRipple
				}
			}
			if (*trustSet.Flags & 0x00040000) != 0 { // tfClearNoRipple
				if isLowAccount {
					rs.Flags &^= lsfLowNoRipple
				} else {
					rs.Flags &^= lsfHighNoRipple
				}
			}
		}

		// Check if trust line should be deleted
		// (both limits are zero and balance is zero)
		if rs.Balance.IsZero() && rs.LowLimit.IsZero() && rs.HighLimit.IsZero() {
			if err := e.view.Erase(trustLineKey); err != nil {
				return TefINTERNAL
			}

			// Decrement owner count
			if account.OwnerCount > 0 {
				account.OwnerCount--
			}

			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "DeletedNode",
				LedgerEntryType: "RippleState",
				LedgerIndex:     hex.EncodeToString(trustLineKey.Key[:]),
			})
		} else {
			// Update the trust line
			updatedData, err := serializeRippleState(rs)
			if err != nil {
				return TefINTERNAL
			}

			if err := e.view.Update(trustLineKey, updatedData); err != nil {
				return TefINTERNAL
			}

			limitField := "LowLimit"
			if !isLowAccount {
				limitField = "HighLimit"
			}

			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "ModifiedNode",
				LedgerEntryType: "RippleState",
				LedgerIndex:     hex.EncodeToString(trustLineKey.Key[:]),
				FinalFields: map[string]any{
					limitField: map[string]any{
						"currency": trustSet.LimitAmount.Currency,
						"issuer":   account.Account,
						"value":    limitAmount.Value.Text('f', 15),
					},
				},
				PreviousFields: map[string]any{
					limitField: map[string]any{
						"currency": trustSet.LimitAmount.Currency,
						"issuer":   account.Account,
						"value":    previousLimit.Value.Text('f', 15),
					},
				},
			})
		}
	}

	return TesSUCCESS
}

// applyOfferCreate applies an OfferCreate transaction
func (e *Engine) applyOfferCreate(offer *OfferCreate, account *AccountRoot, metadata *Metadata) Result {
	// First, cancel any existing offer if OfferSequence is specified
	if offer.OfferSequence != nil {
		accountID, _ := decodeAccountID(account.Account)
		oldOfferKey := keylet.Offer(accountID, *offer.OfferSequence)
		exists, _ := e.view.Exists(oldOfferKey)
		if exists {
			// Delete the old offer
			if err := e.view.Erase(oldOfferKey); err == nil {
				if account.OwnerCount > 0 {
					account.OwnerCount--
				}
				metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
					NodeType:        "DeletedNode",
					LedgerEntryType: "Offer",
					LedgerIndex:     hex.EncodeToString(oldOfferKey.Key[:]),
				})
			}
		}
	}

	// Check account has reserve for new offer
	requiredReserve := e.config.ReserveBase + (uint64(account.OwnerCount)+1)*e.config.ReserveIncrement
	if account.Balance < requiredReserve {
		return TecINSUF_RESERVE_OFFER
	}

	// Get the amounts
	takerGets := offer.TakerGets
	takerPays := offer.TakerPays

	// Check for ImmediateOrCancel or FillOrKill flags
	flags := offer.GetFlags()
	isPassive := (flags & OfferCreateFlagPassive) != 0
	isIOC := (flags & OfferCreateFlagImmediateOrCancel) != 0
	isFOK := (flags & OfferCreateFlagFillOrKill) != 0

	// Track how much was filled
	var takerGotTotal, takerPaidTotal Amount

	// Simple order matching - look for crossing offers
	// This is a simplified implementation that checks if there are any offers to match
	if !isPassive {
		takerGotTotal, takerPaidTotal = e.matchOffers(offer, account, metadata)
	}

	// Check if fully filled
	fullyFilled := false
	if takerGotTotal.Value != "" && takerGets.Value != "" {
		// Compare amounts to check if fully filled
		if takerGets.IsNative() {
			gotDrops, _ := parseDropsString(takerGotTotal.Value)
			wantDrops, _ := parseDropsString(takerGets.Value)
			fullyFilled = gotDrops >= wantDrops
		} else {
			gotIOU := NewIOUAmount(takerGotTotal.Value, takerGotTotal.Currency, takerGotTotal.Issuer)
			wantIOU := NewIOUAmount(takerGets.Value, takerGets.Currency, takerGets.Issuer)
			fullyFilled = gotIOU.Compare(wantIOU) >= 0
		}
	}

	// Handle FillOrKill - if not fully filled, fail
	if isFOK && !fullyFilled {
		return TecKILLED
	}

	// Handle ImmediateOrCancel - don't create offer if not fully filled
	if isIOC && !fullyFilled {
		// Partial fill is OK for IOC, just don't create remaining offer
		if takerGotTotal.Value != "" {
			return TesSUCCESS // Partial fill succeeded
		}
		return TecKILLED // No fill at all
	}

	// If fully filled, don't create a new offer
	if fullyFilled {
		return TesSUCCESS
	}

	// Create the offer in the ledger
	accountID, _ := decodeAccountID(account.Account)
	offerSequence := account.Sequence - 1 // Sequence was already incremented in preflight
	offerKey := keylet.Offer(accountID, offerSequence)

	// Calculate remaining amounts after partial fill
	remainingTakerGets := takerGets
	remainingTakerPays := takerPays
	if takerGotTotal.Value != "" {
		// Subtract what was already obtained
		remainingTakerGets = subtractAmount(takerGets, takerGotTotal)
		remainingTakerPays = subtractAmount(takerPays, takerPaidTotal)
	}

	// Create the ledger offer
	ledgerOffer := &LedgerOffer{
		Account:   account.Account,
		Sequence:  offerSequence,
		TakerPays: remainingTakerPays,
		TakerGets: remainingTakerGets,
		Flags:     0,
	}

	// Set offer flags
	if isPassive {
		ledgerOffer.Flags |= lsfOfferPassive
	}
	if (flags & OfferCreateFlagSell) != 0 {
		ledgerOffer.Flags |= lsfOfferSell
	}

	// Serialize and store the offer
	offerData, err := serializeLedgerOffer(ledgerOffer)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Insert(offerKey, offerData); err != nil {
		return TefINTERNAL
	}

	// Increment owner count
	account.OwnerCount++

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "Offer",
		LedgerIndex:     hex.EncodeToString(offerKey.Key[:]),
		NewFields: map[string]any{
			"Account":   offer.Account,
			"Sequence":  offerSequence,
			"TakerGets": remainingTakerGets,
			"TakerPays": remainingTakerPays,
		},
	})

	return TesSUCCESS
}

// matchOffers attempts to match the new offer against existing offers
// Returns the amounts obtained and paid through matching
func (e *Engine) matchOffers(offer *OfferCreate, account *AccountRoot, metadata *Metadata) (takerGot, takerPaid Amount) {
	// Find matching offers by scanning the ledger
	// This is a simplified implementation - production would use book directories

	// We want to find offers where:
	// - Their TakerGets matches our TakerPays (what we want to pay is what they want to get)
	// - Their TakerPays matches our TakerGets (what we want to get is what they're paying)
	wantCurrency := offer.TakerGets.Currency
	wantIssuer := offer.TakerGets.Issuer
	payCurrency := offer.TakerPays.Currency
	payIssuer := offer.TakerPays.Issuer

	// Determine if matching native XRP
	wantingXRP := offer.TakerGets.IsNative()
	payingXRP := offer.TakerPays.IsNative()

	// Collect matching offers
	type matchOffer struct {
		key      [32]byte
		offer    *LedgerOffer
		quality  float64 // TakerPays/TakerGets (lower is better for us)
	}
	var matches []matchOffer

	// Iterate through ledger entries to find offers
	e.view.ForEach(func(key [32]byte, data []byte) bool {
		// Check if this is an offer (first byte after header indicates type)
		if len(data) < 3 {
			return true // continue
		}

		// Parse the entry type from serialized data
		// LedgerEntryType is the first field (UInt16)
		if data[0] != (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType {
			return true // continue
		}
		entryType := uint16(data[1])<<8 | uint16(data[2])
		if entryType != 0x006F { // Offer type
			return true // continue
		}

		// Parse the offer
		ledgerOffer, err := parseLedgerOffer(data)
		if err != nil {
			return true // continue
		}

		// Skip if same account (can't match own offers)
		if ledgerOffer.Account == account.Account {
			return true // continue
		}

		// Check if this offer is the inverse of what we want
		// Their TakerGets = what we're paying
		// Their TakerPays = what we're getting
		theirGetsMatchesOurPays := false
		if payingXRP && ledgerOffer.TakerGets.IsNative() {
			theirGetsMatchesOurPays = true
		} else if !payingXRP && !ledgerOffer.TakerGets.IsNative() {
			theirGetsMatchesOurPays = ledgerOffer.TakerGets.Currency == payCurrency &&
				ledgerOffer.TakerGets.Issuer == payIssuer
		}

		theirPaysMatchesOurGets := false
		if wantingXRP && ledgerOffer.TakerPays.IsNative() {
			theirPaysMatchesOurGets = true
		} else if !wantingXRP && !ledgerOffer.TakerPays.IsNative() {
			theirPaysMatchesOurGets = ledgerOffer.TakerPays.Currency == wantCurrency &&
				ledgerOffer.TakerPays.Issuer == wantIssuer
		}

		if !theirGetsMatchesOurPays || !theirPaysMatchesOurGets {
			return true // continue
		}

		// Calculate quality (price) of this offer
		// Quality = TakerPays / TakerGets (what they're selling / what they want)
		// Lower quality = better price for us
		quality := calculateQuality(ledgerOffer.TakerPays, ledgerOffer.TakerGets)

		matches = append(matches, matchOffer{
			key:     key,
			offer:   ledgerOffer,
			quality: quality,
		})

		return true // continue
	})

	// If no matches, return empty
	if len(matches) == 0 {
		return Amount{}, Amount{}
	}

	// Sort by quality (lowest/best first)
	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].quality < matches[i].quality {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	// Calculate our offer's limit quality
	ourQuality := calculateQuality(offer.TakerPays, offer.TakerGets)

	// Match against offers
	var totalGot, totalPaid Amount
	remainingWant := offer.TakerGets
	remainingPay := offer.TakerPays

	for _, match := range matches {
		// Check if price crosses (their quality <= our inverse quality)
		// For us: we want high TakerGets, low TakerPays
		// For them: they want high TakerGets, low TakerPays
		// Match if: their_price <= 1/our_price
		if match.quality > 1.0/ourQuality {
			continue // Price doesn't cross
		}

		// Calculate how much we can trade
		theirGets := match.offer.TakerGets
		theirPays := match.offer.TakerPays

		// We want to get as much as possible up to remainingWant
		// We'll pay proportionally based on their exchange rate
		var gotAmount, paidAmount Amount

		if theirPays.IsNative() {
			// They're paying XRP
			theirPaysDrops, _ := parseDropsString(theirPays.Value)
			remainingWantDrops, _ := parseDropsString(remainingWant.Value)

			takeDrops := theirPaysDrops
			if takeDrops > remainingWantDrops {
				takeDrops = remainingWantDrops
			}

			gotAmount = Amount{Value: formatDrops(takeDrops)}

			// Calculate what we pay based on their rate
			if takeDrops == theirPaysDrops {
				paidAmount = theirGets
			} else {
				// Partial fill - calculate proportionally
				if theirGets.IsNative() {
					theirGetsDrops, _ := parseDropsString(theirGets.Value)
					payDrops := (takeDrops * theirGetsDrops) / theirPaysDrops
					paidAmount = Amount{Value: formatDrops(payDrops)}
				} else {
					theirGetsIOU := NewIOUAmount(theirGets.Value, theirGets.Currency, theirGets.Issuer)
					ratio := float64(takeDrops) / float64(theirPaysDrops)
					payValue := multiplyIOUByRatio(theirGetsIOU, ratio)
					paidAmount = Amount{
						Value:    payValue.Value.Text('f', 15),
						Currency: theirGets.Currency,
						Issuer:   theirGets.Issuer,
					}
				}
			}
		} else {
			// They're paying IOU
			theirPaysIOU := NewIOUAmount(theirPays.Value, theirPays.Currency, theirPays.Issuer)
			remainingWantIOU := NewIOUAmount(remainingWant.Value, remainingWant.Currency, remainingWant.Issuer)

			takeIOU := theirPaysIOU
			if takeIOU.Compare(remainingWantIOU) > 0 {
				takeIOU = remainingWantIOU
			}

			gotAmount = Amount{
				Value:    takeIOU.Value.Text('f', 15),
				Currency: theirPays.Currency,
				Issuer:   theirPays.Issuer,
			}

			// Calculate what we pay
			if takeIOU.Compare(theirPaysIOU) == 0 {
				paidAmount = theirGets
			} else {
				// Partial fill
				ratio := divideIOUAmounts(takeIOU, theirPaysIOU)
				if theirGets.IsNative() {
					theirGetsDrops, _ := parseDropsString(theirGets.Value)
					payDrops := uint64(float64(theirGetsDrops) * ratio)
					paidAmount = Amount{Value: formatDrops(payDrops)}
				} else {
					theirGetsIOU := NewIOUAmount(theirGets.Value, theirGets.Currency, theirGets.Issuer)
					payValue := multiplyIOUByRatio(theirGetsIOU, ratio)
					paidAmount = Amount{
						Value:    payValue.Value.Text('f', 15),
						Currency: theirGets.Currency,
						Issuer:   theirGets.Issuer,
					}
				}
			}
		}

		// Update the matched offer in the ledger
		// Calculate remaining amounts for matched offer
		matchRemainingGets := subtractAmount(theirGets, paidAmount)
		matchRemainingPays := subtractAmount(theirPays, gotAmount)

		matchKey := keylet.Keylet{Key: match.key}
		matchKey.Type = 0x6F // Offer type

		if isZeroAmount(matchRemainingGets) || isZeroAmount(matchRemainingPays) {
			// Fully consumed - delete offer
			e.view.Erase(matchKey)
			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "DeletedNode",
				LedgerEntryType: "Offer",
				LedgerIndex:     hex.EncodeToString(match.key[:]),
				FinalFields: map[string]any{
					"Account":   match.offer.Account,
					"TakerGets": theirGets,
					"TakerPays": theirPays,
				},
			})
		} else {
			// Partially consumed - update offer
			match.offer.TakerGets = matchRemainingGets
			match.offer.TakerPays = matchRemainingPays
			updatedData, err := serializeLedgerOffer(match.offer)
			if err == nil {
				e.view.Update(matchKey, updatedData)
				metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
					NodeType:        "ModifiedNode",
					LedgerEntryType: "Offer",
					LedgerIndex:     hex.EncodeToString(match.key[:]),
					FinalFields: map[string]any{
						"Account":   match.offer.Account,
						"TakerGets": matchRemainingGets,
						"TakerPays": matchRemainingPays,
					},
					PreviousFields: map[string]any{
						"TakerGets": theirGets,
						"TakerPays": theirPays,
					},
				})
			}
		}

		// Transfer funds for this match
		e.executeOfferTrade(account, match.offer, gotAmount, paidAmount, metadata)

		// Accumulate totals
		totalGot = addAmount(totalGot, gotAmount)
		totalPaid = addAmount(totalPaid, paidAmount)

		// Update remaining
		remainingWant = subtractAmount(remainingWant, gotAmount)
		remainingPay = subtractAmount(remainingPay, paidAmount)

		// Check if our offer is filled
		if isZeroAmount(remainingWant) {
			break
		}
	}

	return totalGot, totalPaid
}

// calculateQuality calculates the quality (price) of an offer
// Quality = TakerPays / TakerGets
func calculateQuality(pays, gets Amount) float64 {
	var paysVal, getsVal float64

	if pays.IsNative() {
		drops, _ := parseDropsString(pays.Value)
		paysVal = float64(drops)
	} else {
		iou := NewIOUAmount(pays.Value, pays.Currency, pays.Issuer)
		paysVal, _ = iou.Value.Float64()
	}

	if gets.IsNative() {
		drops, _ := parseDropsString(gets.Value)
		getsVal = float64(drops)
	} else {
		iou := NewIOUAmount(gets.Value, gets.Currency, gets.Issuer)
		getsVal, _ = iou.Value.Float64()
	}

	if getsVal == 0 {
		return 0
	}
	return paysVal / getsVal
}

// multiplyIOUByRatio multiplies an IOU amount by a ratio
func multiplyIOUByRatio(amount IOUAmount, ratio float64) IOUAmount {
	val, _ := amount.Value.Float64()
	newVal := val * ratio
	return IOUAmount{
		Value:    new(big.Float).SetFloat64(newVal),
		Currency: amount.Currency,
		Issuer:   amount.Issuer,
	}
}

// divideIOUAmounts divides two IOU amounts and returns the ratio
func divideIOUAmounts(a, b IOUAmount) float64 {
	aVal, _ := a.Value.Float64()
	bVal, _ := b.Value.Float64()
	if bVal == 0 {
		return 0
	}
	return aVal / bVal
}

// addAmount adds two amounts of the same type
func addAmount(a, b Amount) Amount {
	if a.Value == "" {
		return b
	}
	if b.Value == "" {
		return a
	}

	if a.IsNative() {
		aDrops, _ := parseDropsString(a.Value)
		bDrops, _ := parseDropsString(b.Value)
		return Amount{Value: formatDrops(aDrops + bDrops)}
	}

	aIOU := NewIOUAmount(a.Value, a.Currency, a.Issuer)
	bIOU := NewIOUAmount(b.Value, b.Currency, b.Issuer)
	result := aIOU.Add(bIOU)
	return Amount{
		Value:    result.Value.Text('f', 15),
		Currency: a.Currency,
		Issuer:   a.Issuer,
	}
}

// isZeroAmount checks if an amount is zero or empty
func isZeroAmount(a Amount) bool {
	if a.Value == "" || a.Value == "0" {
		return true
	}
	if a.IsNative() {
		drops, _ := parseDropsString(a.Value)
		return drops == 0
	}
	iou := NewIOUAmount(a.Value, a.Currency, a.Issuer)
	return iou.IsZero()
}

// executeOfferTrade executes the fund transfer for an offer trade
func (e *Engine) executeOfferTrade(taker *AccountRoot, maker *LedgerOffer, takerGot, takerPaid Amount, metadata *Metadata) {
	// Get maker account
	makerAccountID, err := decodeAccountID(maker.Account)
	if err != nil {
		return
	}
	makerKey := keylet.Account(makerAccountID)
	makerData, err := e.view.Read(makerKey)
	if err != nil {
		return
	}
	makerAccount, err := parseAccountRoot(makerData)
	if err != nil {
		return
	}

	// Transfer takerGot from maker to taker
	if takerGot.IsNative() {
		drops, _ := parseDropsString(takerGot.Value)
		makerAccount.Balance -= drops
		taker.Balance += drops
	} else {
		// IOU transfer - update trust lines
		e.transferIOU(maker.Account, taker.Account, takerGot, metadata)
	}

	// Transfer takerPaid from taker to maker
	if takerPaid.IsNative() {
		drops, _ := parseDropsString(takerPaid.Value)
		taker.Balance -= drops
		makerAccount.Balance += drops
	} else {
		// IOU transfer - update trust lines
		e.transferIOU(taker.Account, maker.Account, takerPaid, metadata)
	}

	// Update maker account
	updatedMakerData, err := serializeAccountRoot(makerAccount)
	if err != nil {
		return
	}
	e.view.Update(makerKey, updatedMakerData)
}

// transferIOU transfers an IOU amount between accounts via trust lines
func (e *Engine) transferIOU(fromAccount, toAccount string, amount Amount, metadata *Metadata) {
	fromID, err := decodeAccountID(fromAccount)
	if err != nil {
		return
	}
	toID, err := decodeAccountID(toAccount)
	if err != nil {
		return
	}
	issuerID, err := decodeAccountID(amount.Issuer)
	if err != nil {
		return
	}

	iouAmount := NewIOUAmount(amount.Value, amount.Currency, amount.Issuer)

	// Update from's trust line (decrease balance)
	fromIsIssuer := fromAccount == amount.Issuer
	toIsIssuer := toAccount == amount.Issuer

	if fromIsIssuer {
		// Issuer is sending - increase to's trust line balance
		trustLineKey := keylet.Line(toID, issuerID, amount.Currency)
		e.updateTrustLineBalance(trustLineKey, toID, issuerID, iouAmount, true, metadata)
	} else if toIsIssuer {
		// Sending to issuer - decrease from's trust line balance
		trustLineKey := keylet.Line(fromID, issuerID, amount.Currency)
		e.updateTrustLineBalance(trustLineKey, fromID, issuerID, iouAmount, false, metadata)
	} else {
		// Transfer between non-issuers
		// Decrease from's balance with issuer
		fromTrustKey := keylet.Line(fromID, issuerID, amount.Currency)
		e.updateTrustLineBalance(fromTrustKey, fromID, issuerID, iouAmount, false, metadata)

		// Increase to's balance with issuer
		toTrustKey := keylet.Line(toID, issuerID, amount.Currency)
		e.updateTrustLineBalance(toTrustKey, toID, issuerID, iouAmount, true, metadata)
	}
}

// updateTrustLineBalance updates a trust line balance
func (e *Engine) updateTrustLineBalance(key keylet.Keylet, accountID, issuerID [20]byte, amount IOUAmount, increase bool, metadata *Metadata) {
	trustLineData, err := e.view.Read(key)
	if err != nil {
		return
	}

	rs, err := parseRippleState(trustLineData)
	if err != nil {
		return
	}

	accountIsLow := compareAccountIDsForLine(accountID, issuerID) < 0

	if accountIsLow {
		// Account is low, issuer is high
		// Positive balance = issuer owes account
		if increase {
			rs.Balance = rs.Balance.Sub(amount) // More negative = issuer owes more
		} else {
			rs.Balance = rs.Balance.Add(amount) // Less negative = issuer owes less
		}
	} else {
		// Account is high, issuer is low
		// Positive balance = account owes issuer (inverted)
		if increase {
			rs.Balance = rs.Balance.Add(amount)
		} else {
			rs.Balance = rs.Balance.Sub(amount)
		}
	}

	updatedData, err := serializeRippleState(rs)
	if err != nil {
		return
	}

	e.view.Update(key, updatedData)

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "RippleState",
		LedgerIndex:     hex.EncodeToString(key.Key[:]),
		FinalFields: map[string]any{
			"Balance": map[string]any{
				"currency": amount.Currency,
				"value":    rs.Balance.Value.Text('f', 15),
			},
		},
	})
}

// subtractAmount subtracts b from a
func subtractAmount(a, b Amount) Amount {
	if a.IsNative() {
		aDrops, _ := parseDropsString(a.Value)
		bDrops, _ := parseDropsString(b.Value)
		if bDrops >= aDrops {
			return Amount{Value: "0"}
		}
		return Amount{Value: formatDrops(aDrops - bDrops)}
	}

	aIOU := NewIOUAmount(a.Value, a.Currency, a.Issuer)
	bIOU := NewIOUAmount(b.Value, b.Currency, b.Issuer)
	result := aIOU.Sub(bIOU)
	if result.IsNegative() {
		return Amount{Value: "0", Currency: a.Currency, Issuer: a.Issuer}
	}
	return Amount{
		Value:    result.Value.Text('f', 15),
		Currency: a.Currency,
		Issuer:   a.Issuer,
	}
}

// applyOfferCancel applies an OfferCancel transaction
func (e *Engine) applyOfferCancel(cancel *OfferCancel, account *AccountRoot, metadata *Metadata) Result {
	// Find and remove the offer
	accountID, _ := decodeAccountID(account.Account)
	offerKey := keylet.Offer(accountID, cancel.OfferSequence)

	exists, err := e.view.Exists(offerKey)
	if err != nil {
		return TefINTERNAL
	}

	if !exists {
		// Offer doesn't exist - this is OK (maybe already filled/cancelled)
		return TesSUCCESS
	}

	// Delete the offer
	if err := e.view.Erase(offerKey); err != nil {
		return TefINTERNAL
	}

	// Decrement owner count
	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "Offer",
		LedgerIndex:     hex.EncodeToString(offerKey.Key[:]),
	})

	return TesSUCCESS
}
