package tx

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"

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

	// Parse the fee from the transaction
	feeDrops, err := strconv.ParseUint(payment.Fee, 10, 64)
	if err != nil {
		feeDrops = e.config.BaseFee // fallback to base fee if not specified
	}

	// Calculate reserve as: ReserveBase + (ownerCount * ReserveIncrement)
	// This matches rippled's accountReserve(ownerCount) calculation
	reserve := e.config.ReserveBase + (uint64(sender.OwnerCount) * e.config.ReserveIncrement)

	// Use max(reserve, fee) as the minimum balance that must remain
	// This matches rippled's behavior: auto const mmm = std::max(reserve, ctx_.tx.getFieldAmount(sfFee).xrp())
	minBalance := reserve
	if feeDrops > minBalance {
		minBalance = feeDrops
	}

	// Check sender has enough balance (amount + minBalance)
	requiredBalance := amountDrops + minBalance
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

		// Check for pseudo-account (AMM accounts cannot receive direct payments)
		// See rippled Payment.cpp:636-637: if (isPseudoAccount(sleDst)) return tecNO_PERMISSION
		if (destAccount.Flags & lsfAMM) != 0 {
			return TecNO_PERMISSION
		}

		// Check destination's lsfDisallowXRP flag
		// Per rippled, if lsfDisallowXRP is set and sender != destination, return tecNO_TARGET
		// This allows accounts to indicate they don't want to receive XRP
		// Reference: this matches rippled behavior for direct XRP payments
		if (destAccount.Flags & lsfDisallowXRP) != 0 {
			senderAccountID, err := decodeAccountID(sender.Account)
			if err != nil {
				return TefINTERNAL
			}
			// Only reject if sender is not the destination (self-payments are allowed)
			if senderAccountID != destAccountID {
				return TecNO_TARGET
			}
		}

		previousDestBalance := destAccount.Balance
		previousDestTxnID := destAccount.PreviousTxnID
		previousDestTxnLgrSeq := destAccount.PreviousTxnLgrSeq

		// Check if destination requires a tag
		if (destAccount.Flags&0x00020000) != 0 && payment.DestinationTag == nil {
			return TecDST_TAG_NEEDED
		}

		// Check deposit authorization
		// Reference: rippled Payment.cpp:641-677
		// If destination has lsfDepositAuth flag set, payments require preauthorization
		// EXCEPT: to prevent account "wedging", allow small payments if BOTH conditions are true:
		//   1. Destination balance <= base reserve (account is at or below minimum)
		//   2. Payment amount <= base reserve
		if (destAccount.Flags & lsfDepositAuth) != 0 {
			dstReserve := e.config.ReserveBase

			// Check if the exception applies (prevents account wedging)
			if amountDrops > dstReserve || destAccount.Balance > dstReserve {
				// Must check for preauthorization
				senderAccountID, err := decodeAccountID(sender.Account)
				if err != nil {
					return TefINTERNAL
				}

				// Look up the DepositPreauth ledger entry
				depositPreauthKey := keylet.DepositPreauth(destAccountID, senderAccountID)
				preauthExists, err := e.view.Exists(depositPreauthKey)
				if err != nil {
					return TefINTERNAL
				}

				if !preauthExists {
					// Sender is not preauthorized to deposit to this account
					return TecNO_PERMISSION
				}
			}
			// If both conditions are true (small payment to low-balance account),
			// payment is allowed without preauthorization
		}

		// Credit destination
		destAccount.Balance += amountDrops

		// Clear PasswordSpent flag if set (lsfPasswordSpent = 0x00010000)
		// Per rippled Payment.cpp:686-687, receiving XRP clears this flag
		if (destAccount.Flags & 0x00010000) != 0 {
			destAccount.Flags &^= 0x00010000
		}

		// Update PreviousTxnID and PreviousTxnLgrSeq on destination (thread the account)
		destAccount.PreviousTxnID = e.currentTxHash
		destAccount.PreviousTxnLgrSeq = e.config.LedgerSequence

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
		// Include all fields that rippled marks with sMD_Always (Flags, OwnerCount, Sequence)
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:          "ModifiedNode",
			LedgerEntryType:   "AccountRoot",
			LedgerIndex:       strings.ToUpper(hex.EncodeToString(destKey.Key[:])),
			PreviousTxnLgrSeq: previousDestTxnLgrSeq,
			PreviousTxnID:     strings.ToUpper(hex.EncodeToString(previousDestTxnID[:])),
			FinalFields: map[string]any{
				"Account":    payment.Destination,
				"Balance":    strconv.FormatUint(destAccount.Balance, 10),
				"Flags":      destAccount.Flags,
				"OwnerCount": destAccount.OwnerCount,
				"Sequence":   destAccount.Sequence,
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
	// With featureDeletableAccounts enabled (mainnet), new accounts start with
	// sequence equal to the current ledger sequence (see rippled Payment.cpp:409-411)
	newAccount := &AccountRoot{
		Account:           payment.Destination,
		Balance:           amountDrops,
		Sequence:          e.config.LedgerSequence,
		Flags:             0,
		PreviousTxnID:     e.currentTxHash,
		PreviousTxnLgrSeq: e.config.LedgerSequence,
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
			"Sequence": e.config.LedgerSequence,
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

	var result Result
	var deliveredAmount IOUAmount

	if senderIsIssuer {
		// Sender is issuing their own currency to destination
		// Need trust line from destination to sender (issuer)
		result, deliveredAmount = e.applyIOUIssueWithDelivered(payment, sender, destAccount, senderAccountID, destAccountID, amount, metadata)
	} else if destIsIssuer {
		// Destination is the issuer - sender is redeeming tokens
		// Need trust line from sender to destination (issuer)
		result, deliveredAmount = e.applyIOURedeemWithDelivered(payment, sender, destAccount, senderAccountID, destAccountID, amount, metadata)
	} else {
		// Neither is issuer - transfer between two non-issuer accounts
		// This requires trust lines from both parties to the issuer
		result, deliveredAmount = e.applyIOUTransferWithDelivered(payment, sender, destAccount, senderAccountID, destAccountID, issuerAccountID, amount, metadata)
	}

	// DeliverMin enforcement for partial payments
	// Reference: rippled Payment.cpp:496-500
	// If tfPartialPayment is set and DeliverMin is specified, check that delivered >= DeliverMin
	if result == TesSUCCESS && payment.DeliverMin != nil {
		flags := payment.GetFlags()
		if (flags & PaymentFlagPartialPayment) != 0 {
			deliverMin := NewIOUAmount(payment.DeliverMin.Value, payment.DeliverMin.Currency, payment.DeliverMin.Issuer)
			if deliveredAmount.Compare(deliverMin) < 0 {
				return TecPATH_PARTIAL
			}
		}
	}

	return result
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
	// RippleState balance semantics:
	// - Negative balance = LOW owes HIGH (HIGH holds tokens)
	// - Positive balance = HIGH owes LOW (LOW holds tokens)
	var newBalance IOUAmount
	if destIsLow {
		// Dest is LOW, sender (issuer) is HIGH
		// Issuing means issuer (HIGH) now owes dest (LOW) more
		// Positive balance = HIGH owes LOW, so make MORE positive
		newBalance = rippleState.Balance.Add(amount)
	} else {
		// Dest is HIGH, sender (issuer) is LOW
		// Issuing means issuer (LOW) now owes dest (HIGH) more
		// Negative balance = LOW owes HIGH, so make MORE negative
		newBalance = rippleState.Balance.Sub(amount)
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

	// Ensure the new balance has the correct currency and issuer
	// (the parsed balance may have null bytes for currency if it was zero)
	newBalance.Currency = amount.Currency
	newBalance.Issuer = amount.Issuer

	// Save old PreviousTxnID/LgrSeq for metadata (before updating)
	oldPreviousTxnID := rippleState.PreviousTxnID
	oldPreviousTxnLgrSeq := rippleState.PreviousTxnLgrSeq

	// Update the trust line
	rippleState.Balance = newBalance

	// Update PreviousTxnID and PreviousTxnLgrSeq to this transaction
	rippleState.PreviousTxnID = e.currentTxHash
	rippleState.PreviousTxnLgrSeq = e.config.LedgerSequence

	// Serialize and update
	updatedTrustLine, err := serializeRippleState(rippleState)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(trustLineKey, updatedTrustLine); err != nil {
		return TefINTERNAL
	}

	// Record the modification with all fields rippled includes
	// Balance issuer in RippleState metadata is ACCOUNT_ONE (rrrrrrrrrrrrrrrrrrrrBZbvji)
	previousBalanceStr := "0"
	if rippleState.Balance.Value != nil {
		// Calculate previous balance (before adding amount)
		prevBal := rippleState.Balance.Sub(newBalance).Negate()
		previousBalanceStr = formatIOUValue(prevBal.Value)
	}
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:          "ModifiedNode",
		LedgerEntryType:   "RippleState",
		LedgerIndex:       strings.ToUpper(hex.EncodeToString(trustLineKey.Key[:])),
		PreviousTxnLgrSeq: oldPreviousTxnLgrSeq,
		PreviousTxnID:     strings.ToUpper(hex.EncodeToString(oldPreviousTxnID[:])),
		FinalFields: map[string]any{
			"Balance": map[string]any{
				"currency": amount.Currency,
				"issuer":   "rrrrrrrrrrrrrrrrrrrrBZbvji",
				"value":    formatIOUValue(newBalance.Value),
			},
			"Flags": rippleState.Flags,
			"HighLimit": map[string]any{
				"currency": amount.Currency,
				"issuer":   rippleState.HighLimit.Issuer,
				"value":    formatIOUValue(rippleState.HighLimit.Value),
			},
			"HighNode": fmt.Sprintf("%x", rippleState.HighNode),
			"LowLimit": map[string]any{
				"currency": amount.Currency,
				"issuer":   rippleState.LowLimit.Issuer,
				"value":    formatIOUValue(rippleState.LowLimit.Value),
			},
			"LowNode": fmt.Sprintf("%x", rippleState.LowNode),
		},
		PreviousFields: map[string]any{
			"Balance": map[string]any{
				"currency": amount.Currency,
				"issuer":   "rrrrrrrrrrrrrrrrrrrrBZbvji",
				"value":    previousBalanceStr,
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
	// RippleState balance semantics:
	// - Negative balance = LOW owes HIGH (HIGH holds tokens)
	// - Positive balance = HIGH owes LOW (LOW holds tokens)
	var senderBalance IOUAmount
	if senderIsLow {
		// Sender is LOW, issuer (dest) is HIGH
		// Positive balance = sender holds tokens (HIGH owes LOW)
		senderBalance = rippleState.Balance
	} else {
		// Sender is HIGH, issuer (dest) is LOW
		// Negative balance = sender holds tokens (LOW owes HIGH)
		// Negate to get positive holdings value
		senderBalance = rippleState.Balance.Negate()
	}

	// Check sender has enough balance
	if senderBalance.Compare(amount) < 0 {
		return TecPATH_PARTIAL
	}

	// Save previous balance for metadata
	previousBalance := rippleState.Balance

	// Update balance by reducing sender's holding
	// When redeeming, the issuer owes less to the sender
	var newBalance IOUAmount
	if senderIsLow {
		// Sender is LOW, issuer is HIGH
		// Positive balance = sender holds. Reduce by subtracting.
		newBalance = rippleState.Balance.Sub(amount)
	} else {
		// Sender is HIGH, issuer is LOW
		// Negative balance = sender holds. Make less negative by adding.
		newBalance = rippleState.Balance.Add(amount)
	}

	// Ensure the new balance has the correct currency and issuer
	// (the parsed balance may have null bytes for currency if it was zero)
	newBalance.Currency = amount.Currency
	newBalance.Issuer = amount.Issuer

	// Save old PreviousTxnID/LgrSeq for metadata (before updating)
	oldPreviousTxnID := rippleState.PreviousTxnID
	oldPreviousTxnLgrSeq := rippleState.PreviousTxnLgrSeq

	rippleState.Balance = newBalance

	// Update PreviousTxnID and PreviousTxnLgrSeq to this transaction
	rippleState.PreviousTxnID = e.currentTxHash
	rippleState.PreviousTxnLgrSeq = e.config.LedgerSequence

	// Serialize and update
	updatedTrustLine, err := serializeRippleState(rippleState)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Update(trustLineKey, updatedTrustLine); err != nil {
		return TefINTERNAL
	}

	// Record the modification with all fields rippled includes
	// Balance issuer in RippleState metadata is ACCOUNT_ONE (rrrrrrrrrrrrrrrrrrrrBZbvji)
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:          "ModifiedNode",
		LedgerEntryType:   "RippleState",
		LedgerIndex:       strings.ToUpper(hex.EncodeToString(trustLineKey.Key[:])),
		PreviousTxnLgrSeq: oldPreviousTxnLgrSeq,
		PreviousTxnID:     strings.ToUpper(hex.EncodeToString(oldPreviousTxnID[:])),
		FinalFields: map[string]any{
			"Balance": map[string]any{
				"currency": amount.Currency,
				"issuer":   "rrrrrrrrrrrrrrrrrrrrBZbvji",
				"value":    formatIOUValue(newBalance.Value),
			},
			"Flags": rippleState.Flags,
			"HighLimit": map[string]any{
				"currency": amount.Currency,
				"issuer":   rippleState.HighLimit.Issuer,
				"value":    formatIOUValue(rippleState.HighLimit.Value),
			},
			"HighNode": fmt.Sprintf("%x", rippleState.HighNode),
			"LowLimit": map[string]any{
				"currency": amount.Currency,
				"issuer":   rippleState.LowLimit.Issuer,
				"value":    formatIOUValue(rippleState.LowLimit.Value),
			},
			"LowNode": fmt.Sprintf("%x", rippleState.LowNode),
		},
		PreviousFields: map[string]any{
			"Balance": map[string]any{
				"currency": amount.Currency,
				"issuer":   "rrrrrrrrrrrrrrrrrrrrBZbvji",
				"value":    formatIOUValue(previousBalance.Value),
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

	// Save previous balances for metadata
	senderPreviousBalance := senderRippleState.Balance
	destPreviousBalance := destRippleState.Balance

	// Calculate sender's balance with issuer
	// RippleState balance semantics:
	// - Negative balance = LOW owes HIGH (HIGH holds tokens)
	// - Positive balance = HIGH owes LOW (LOW holds tokens)
	senderIsLowWithIssuer := compareAccountIDsForLine(senderID, issuerID) < 0
	var senderBalance IOUAmount
	if senderIsLowWithIssuer {
		// Sender is LOW, issuer is HIGH
		// Positive balance = sender holds tokens (HIGH/issuer owes LOW/sender)
		senderBalance = senderRippleState.Balance
	} else {
		// Sender is HIGH, issuer is LOW
		// Negative balance = sender holds tokens (LOW/issuer owes HIGH/sender)
		senderBalance = senderRippleState.Balance.Negate()
	}

	// Check sender has enough
	if senderBalance.Compare(amount) < 0 {
		return TecPATH_PARTIAL
	}

	// Calculate destination's current balance and trust limit
	destIsLowWithIssuer := compareAccountIDsForLine(destID, issuerID) < 0
	var destBalance, destLimit IOUAmount
	if destIsLowWithIssuer {
		// Dest is LOW, issuer is HIGH
		// Positive balance = dest holds tokens
		destBalance = destRippleState.Balance
		destLimit = destRippleState.LowLimit
	} else {
		// Dest is HIGH, issuer is LOW
		// Negative balance = dest holds tokens
		destBalance = destRippleState.Balance.Negate()
		destLimit = destRippleState.HighLimit
	}

	// Check destination trust limit
	newDestBalance := destBalance.Add(amount)
	if !destLimit.IsZero() && newDestBalance.Compare(destLimit) > 0 {
		return TecPATH_PARTIAL
	}

	// Update sender's trust line (decrease balance - sender loses tokens)
	var newSenderRippleBalance IOUAmount
	if senderIsLowWithIssuer {
		// Sender is LOW, positive balance = holdings. Decrease by subtracting.
		newSenderRippleBalance = senderRippleState.Balance.Sub(amount)
	} else {
		// Sender is HIGH, negative balance = holdings. Make less negative by adding.
		newSenderRippleBalance = senderRippleState.Balance.Add(amount)
	}
	// Ensure the new balance has the correct currency and issuer
	newSenderRippleBalance.Currency = amount.Currency
	newSenderRippleBalance.Issuer = amount.Issuer
	senderRippleState.Balance = newSenderRippleBalance

	// Update destination's trust line (increase balance - dest gains tokens)
	var newDestRippleBalance IOUAmount
	if destIsLowWithIssuer {
		// Dest is LOW, positive balance = holdings. Increase by adding.
		newDestRippleBalance = destRippleState.Balance.Add(amount)
	} else {
		// Dest is HIGH, negative balance = holdings. Make more negative by subtracting.
		newDestRippleBalance = destRippleState.Balance.Sub(amount)
	}
	// Ensure the new balance has the correct currency and issuer
	newDestRippleBalance.Currency = amount.Currency
	newDestRippleBalance.Issuer = amount.Issuer
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
	// Balance issuer in RippleState metadata is ACCOUNT_ONE (rrrrrrrrrrrrrrrrrrrrBZbvji)
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:          "ModifiedNode",
		LedgerEntryType:   "RippleState",
		LedgerIndex:       strings.ToUpper(hex.EncodeToString(senderTrustLineKey.Key[:])),
		PreviousTxnLgrSeq: senderRippleState.PreviousTxnLgrSeq,
		PreviousTxnID:     strings.ToUpper(hex.EncodeToString(senderRippleState.PreviousTxnID[:])),
		FinalFields: map[string]any{
			"Balance": map[string]any{
				"currency": amount.Currency,
				"issuer":   "rrrrrrrrrrrrrrrrrrrrBZbvji",
				"value":    formatIOUValue(newSenderRippleBalance.Value),
			},
			"Flags": senderRippleState.Flags,
			"HighLimit": map[string]any{
				"currency": amount.Currency,
				"issuer":   senderRippleState.HighLimit.Issuer,
				"value":    formatIOUValue(senderRippleState.HighLimit.Value),
			},
			"HighNode": fmt.Sprintf("%x", senderRippleState.HighNode),
			"LowLimit": map[string]any{
				"currency": amount.Currency,
				"issuer":   senderRippleState.LowLimit.Issuer,
				"value":    formatIOUValue(senderRippleState.LowLimit.Value),
			},
			"LowNode": fmt.Sprintf("%x", senderRippleState.LowNode),
		},
		PreviousFields: map[string]any{
			"Balance": map[string]any{
				"currency": amount.Currency,
				"issuer":   "rrrrrrrrrrrrrrrrrrrrBZbvji",
				"value":    formatIOUValue(senderPreviousBalance.Value),
			},
		},
	})

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:          "ModifiedNode",
		LedgerEntryType:   "RippleState",
		LedgerIndex:       strings.ToUpper(hex.EncodeToString(destTrustLineKey.Key[:])),
		PreviousTxnLgrSeq: destRippleState.PreviousTxnLgrSeq,
		PreviousTxnID:     strings.ToUpper(hex.EncodeToString(destRippleState.PreviousTxnID[:])),
		FinalFields: map[string]any{
			"Balance": map[string]any{
				"currency": amount.Currency,
				"issuer":   "rrrrrrrrrrrrrrrrrrrrBZbvji",
				"value":    formatIOUValue(newDestRippleBalance.Value),
			},
			"Flags": destRippleState.Flags,
			"HighLimit": map[string]any{
				"currency": amount.Currency,
				"issuer":   destRippleState.HighLimit.Issuer,
				"value":    formatIOUValue(destRippleState.HighLimit.Value),
			},
			"HighNode": fmt.Sprintf("%x", destRippleState.HighNode),
			"LowLimit": map[string]any{
				"currency": amount.Currency,
				"issuer":   destRippleState.LowLimit.Issuer,
				"value":    formatIOUValue(destRippleState.LowLimit.Value),
			},
			"LowNode": fmt.Sprintf("%x", destRippleState.LowNode),
		},
		PreviousFields: map[string]any{
			"Balance": map[string]any{
				"currency": amount.Currency,
				"issuer":   "rrrrrrrrrrrrrrrrrrrrBZbvji",
				"value":    formatIOUValue(destPreviousBalance.Value),
			},
		},
	})

	delivered := payment.Amount
	metadata.DeliveredAmount = &delivered

	return TesSUCCESS
}

// applyIOUIssueWithDelivered wraps applyIOUIssue to return the delivered amount
func (e *Engine) applyIOUIssueWithDelivered(payment *Payment, sender *AccountRoot, dest *AccountRoot, senderID, destID [20]byte, amount IOUAmount, metadata *Metadata) (Result, IOUAmount) {
	result := e.applyIOUIssue(payment, sender, dest, senderID, destID, amount, metadata)
	if result == TesSUCCESS {
		// For successful issue, the full amount is delivered
		return result, amount
	}
	return result, IOUAmount{}
}

// applyIOURedeemWithDelivered wraps applyIOURedeem to return the delivered amount
func (e *Engine) applyIOURedeemWithDelivered(payment *Payment, sender *AccountRoot, dest *AccountRoot, senderID, destID [20]byte, amount IOUAmount, metadata *Metadata) (Result, IOUAmount) {
	result := e.applyIOURedeem(payment, sender, dest, senderID, destID, amount, metadata)
	if result == TesSUCCESS {
		// For successful redeem, the full amount is delivered
		return result, amount
	}
	return result, IOUAmount{}
}

// applyIOUTransferWithDelivered wraps applyIOUTransfer to return the delivered amount
func (e *Engine) applyIOUTransferWithDelivered(payment *Payment, sender *AccountRoot, dest *AccountRoot, senderID, destID, issuerID [20]byte, amount IOUAmount, metadata *Metadata) (Result, IOUAmount) {
	result := e.applyIOUTransfer(payment, sender, dest, senderID, destID, issuerID, amount, metadata)
	if result == TesSUCCESS {
		// For successful transfer, the full amount is delivered
		return result, amount
	}
	return result, IOUAmount{}
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

// TransferRate constants (QUALITY_ONE = 1000000000)
const (
	qualityOne uint32 = 1000000000 // 1e9 = 100% (no fee)
)

// applyAccountSet applies an AccountSet transaction
// Reference: rippled SetAccount.cpp doApply()
func (e *Engine) applyAccountSet(accountSet *AccountSet, account *AccountRoot, metadata *Metadata) Result {
	uFlagsIn := account.Flags
	uFlagsOut := uFlagsIn

	var uSetFlag, uClearFlag uint32
	if accountSet.SetFlag != nil {
		uSetFlag = *accountSet.SetFlag
	}
	if accountSet.ClearFlag != nil {
		uClearFlag = *accountSet.ClearFlag
	}

	//
	// RequireAuth
	//
	if uSetFlag == AccountSetFlagRequireAuth && (uFlagsIn&lsfRequireAuth) == 0 {
		uFlagsOut |= lsfRequireAuth
	}
	if uClearFlag == AccountSetFlagRequireAuth && (uFlagsIn&lsfRequireAuth) != 0 {
		uFlagsOut &^= lsfRequireAuth
	}

	//
	// RequireDestTag
	//
	if uSetFlag == AccountSetFlagRequireDest && (uFlagsIn&lsfRequireDestTag) == 0 {
		uFlagsOut |= lsfRequireDestTag
	}
	if uClearFlag == AccountSetFlagRequireDest && (uFlagsIn&lsfRequireDestTag) != 0 {
		uFlagsOut &^= lsfRequireDestTag
	}

	//
	// DisallowXRP
	//
	if uSetFlag == AccountSetFlagDisallowXRP && (uFlagsIn&lsfDisallowXRP) == 0 {
		uFlagsOut |= lsfDisallowXRP
	}
	if uClearFlag == AccountSetFlagDisallowXRP && (uFlagsIn&lsfDisallowXRP) != 0 {
		uFlagsOut &^= lsfDisallowXRP
	}

	//
	// DisableMaster
	//
	if uSetFlag == AccountSetFlagDisableMaster && (uFlagsIn&lsfDisableMaster) == 0 {
		// Need to check RegularKey or SignerList exists
		if account.RegularKey == "" {
			// TODO: Also check for SignerList existence
			return TecNO_ALTERNATIVE_KEY
		}
		uFlagsOut |= lsfDisableMaster
	}
	if uClearFlag == AccountSetFlagDisableMaster && (uFlagsIn&lsfDisableMaster) != 0 {
		uFlagsOut &^= lsfDisableMaster
	}

	//
	// DefaultRipple
	//
	if uSetFlag == AccountSetFlagDefaultRipple {
		uFlagsOut |= lsfDefaultRipple
	} else if uClearFlag == AccountSetFlagDefaultRipple {
		uFlagsOut &^= lsfDefaultRipple
	}

	//
	// NoFreeze (cannot be cleared once set)
	//
	if uSetFlag == AccountSetFlagNoFreeze {
		// Note: rippled requires master key signature to set NoFreeze
		// We skip that check for simplicity, but in production this should be enforced
		uFlagsOut |= lsfNoFreeze
	}
	// NoFreeze cannot be cleared - intentionally no clear handling

	//
	// GlobalFreeze
	//
	if uSetFlag == AccountSetFlagGlobalFreeze {
		uFlagsOut |= lsfGlobalFreeze
	}
	// If you have set NoFreeze, you may not clear GlobalFreeze
	// This prevents those who have set NoFreeze from using GlobalFreeze strategically
	if uSetFlag != AccountSetFlagGlobalFreeze && uClearFlag == AccountSetFlagGlobalFreeze {
		if (uFlagsOut & lsfNoFreeze) == 0 {
			uFlagsOut &^= lsfGlobalFreeze
		}
	}

	//
	// AccountTxnID - track transaction IDs signed by this account
	//
	if uSetFlag == AccountSetFlagAccountTxnID {
		// Make field present (set to zero hash initially, will be updated on next tx)
		var zeroHash [32]byte
		if account.AccountTxnID == zeroHash {
			// Field not yet present, make it present
			account.AccountTxnID = e.currentTxHash
		}
	}
	if uClearFlag == AccountSetFlagAccountTxnID {
		// Clear the AccountTxnID field
		account.AccountTxnID = [32]byte{}
	}

	//
	// DepositAuth
	//
	if uSetFlag == AccountSetFlagDepositAuth {
		uFlagsOut |= lsfDepositAuth
	} else if uClearFlag == AccountSetFlagDepositAuth {
		uFlagsOut &^= lsfDepositAuth
	}

	//
	// AuthorizedNFTokenMinter
	//
	if uSetFlag == AccountSetFlagAuthorizedNFTokenMinter {
		if accountSet.NFTokenMinter != "" {
			account.NFTokenMinter = accountSet.NFTokenMinter
		}
	}
	if uClearFlag == AccountSetFlagAuthorizedNFTokenMinter {
		account.NFTokenMinter = ""
	}

	//
	// Disallow Incoming flags
	//
	if uSetFlag == AccountSetFlagDisallowIncomingNFTokenOffer {
		uFlagsOut |= lsfDisallowIncomingNFTokenOffer
	} else if uClearFlag == AccountSetFlagDisallowIncomingNFTokenOffer {
		uFlagsOut &^= lsfDisallowIncomingNFTokenOffer
	}

	if uSetFlag == AccountSetFlagDisallowIncomingCheck {
		uFlagsOut |= lsfDisallowIncomingCheck
	} else if uClearFlag == AccountSetFlagDisallowIncomingCheck {
		uFlagsOut &^= lsfDisallowIncomingCheck
	}

	if uSetFlag == AccountSetFlagDisallowIncomingPayChan {
		uFlagsOut |= lsfDisallowIncomingPayChan
	} else if uClearFlag == AccountSetFlagDisallowIncomingPayChan {
		uFlagsOut &^= lsfDisallowIncomingPayChan
	}

	if uSetFlag == AccountSetFlagDisallowIncomingTrustline {
		uFlagsOut |= lsfDisallowIncomingTrustline
	} else if uClearFlag == AccountSetFlagDisallowIncomingTrustline {
		uFlagsOut &^= lsfDisallowIncomingTrustline
	}

	//
	// AllowTrustLineClawback (cannot be cleared once set)
	//
	if uSetFlag == AccountSetFlagAllowTrustLineClawback {
		// Note: Cannot set clawback if NoFreeze is set (checked in preclaim)
		// Cannot set if owner directory is not empty (checked in preclaim)
		uFlagsOut |= lsfAllowTrustLineClawback
	}
	// AllowTrustLineClawback cannot be cleared - intentionally no clear handling

	//
	// Domain - if empty string, clear the field
	//
	if accountSet.Domain != "" {
		account.Domain = accountSet.Domain
	} else if accountSet.SetFlag == nil && accountSet.ClearFlag == nil {
		// Only clear if this is a field-setting transaction (not just flag setting)
		// Actually per rippled, Domain is cleared when present and empty
	}
	// Check if Domain field is explicitly set to empty in the transaction
	// This requires checking if the field was present in the original JSON
	// For now, we handle clearing by checking a special marker

	//
	// EmailHash - if zero hash, clear the field
	//
	if accountSet.EmailHash != "" {
		// Check if it's zero hash (to clear)
		if accountSet.EmailHash == "00000000000000000000000000000000" {
			account.EmailHash = ""
		} else {
			account.EmailHash = accountSet.EmailHash
		}
	}

	//
	// MessageKey - if empty, clear the field
	//
	if accountSet.MessageKey != "" {
		account.MessageKey = accountSet.MessageKey
	}

	//
	// WalletLocator - if zero hash, clear the field
	//
	if accountSet.WalletLocator != "" {
		if isZeroHash256(accountSet.WalletLocator) {
			account.WalletLocator = ""
		} else {
			account.WalletLocator = accountSet.WalletLocator
		}
	}

	//
	// TransferRate - validation and clearing
	// TransferRate must be 0, QUALITY_ONE (1e9), or between QUALITY_ONE and 2*QUALITY_ONE
	// If 0 or QUALITY_ONE, clear the field
	//
	if accountSet.TransferRate != nil {
		rate := *accountSet.TransferRate
		// Validation should be done in preflight, but double-check here
		if rate != 0 && rate < qualityOne {
			return TemBAD_TRANSFER_RATE
		}
		if rate > 2*qualityOne {
			return TemBAD_TRANSFER_RATE
		}
		// If rate is 0 or QUALITY_ONE, clear the field
		if rate == 0 || rate == qualityOne {
			account.TransferRate = 0
		} else {
			account.TransferRate = rate
		}
	}

	//
	// TickSize - if 0 or maxTickSize (15), clear the field
	// Valid values: 0 (clear), 3-15
	//
	if accountSet.TickSize != nil {
		tickSize := *accountSet.TickSize
		// If 0 or 15, clear the field
		if tickSize == 0 || tickSize == 15 {
			account.TickSize = 0
		} else {
			account.TickSize = tickSize
		}
	}

	// Update flags if changed
	if uFlagsIn != uFlagsOut {
		account.Flags = uFlagsOut
	}

	return TesSUCCESS
}

// isZeroHash256 checks if a hex string represents a zero 256-bit hash
func isZeroHash256(hexStr string) bool {
	// Zero hash is 64 hex zeros
	if len(hexStr) != 64 {
		return false
	}
	for _, c := range hexStr {
		if c != '0' {
			return false
		}
	}
	return true
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
	// Per rippled SetTrust.cpp preclaim: if bSetAuth && !(account.Flags & lsfRequireAuth) -> tefNO_AUTH_REQUIRED
	if bSetAuth && (account.Flags&lsfRequireAuth) == 0 {
		return TefNO_AUTH_REQUIRED
	}

	// Validate freeze flags - cannot freeze if account has lsfNoFreeze set
	// Per rippled SetTrust.cpp preclaim: if bNoFreeze && bSetFreeze -> tecNO_PERMISSION
	bNoFreeze := (account.Flags & lsfNoFreeze) != 0
	if bNoFreeze && bSetFreeze {
		return TecNO_PERMISSION
	}

	// Parse quality values from transaction
	// Per rippled: QUALITY_ONE (1e9 = 1000000000) is treated as default (stored as 0)
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
		// Check if setting zero limit without existing trust line
		if limitAmount.IsZero() && !bSetAuth && (!bQualityIn || uQualityIn == 0) && (!bQualityOut || uQualityOut == 0) {
			// Nothing to do - no trust line and setting default values
			return TesSUCCESS
		}

		// Check account has reserve for new trust line
		// Per rippled SetTrust.cpp:405-407, first 2 objects don't need extra reserve
		// Reference: The reserve required to create the line is 0 if ownerCount < 2,
		// otherwise it's accountReserve(ownerCount + 1)
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
			// Account is LOW
			rs.LowLimit = limitAmount
			rs.HighLimit = NewIOUAmount("0", trustSet.LimitAmount.Currency, account.Account)
			rs.Flags |= lsfLowReserve
		} else {
			// Account is HIGH
			rs.LowLimit = NewIOUAmount("0", trustSet.LimitAmount.Currency, trustSet.LimitAmount.Issuer)
			rs.HighLimit = limitAmount
			rs.Flags |= lsfHighReserve
		}

		// Handle Auth flag for new trust line
		if bSetAuth {
			if bHigh {
				rs.Flags |= lsfHighAuth
			} else {
				rs.Flags |= lsfLowAuth
			}
		}

		// Handle NoRipple flag from transaction
		if bSetNoRipple && !bClearNoRipple {
			if bHigh {
				rs.Flags |= lsfHighNoRipple
			} else {
				rs.Flags |= lsfLowNoRipple
			}
		}

		// Handle Freeze flag for new trust line
		if bSetFreeze && !bClearFreeze && !bNoFreeze {
			if bHigh {
				rs.Flags |= lsfHighFreeze
			} else {
				rs.Flags |= lsfLowFreeze
			}
		}

		// Handle QualityIn/QualityOut for new trust line
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

		// Build metadata for created node
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
		if rs.LowQualityIn != 0 {
			newFields["LowQualityIn"] = rs.LowQualityIn
		}
		if rs.LowQualityOut != 0 {
			newFields["LowQualityOut"] = rs.LowQualityOut
		}
		if rs.HighQualityIn != 0 {
			newFields["HighQualityIn"] = rs.HighQualityIn
		}
		if rs.HighQualityOut != 0 {
			newFields["HighQualityOut"] = rs.HighQualityOut
		}

		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "RippleState",
			LedgerIndex:     hex.EncodeToString(trustLineKey.Key[:]),
			NewFields:       newFields,
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

		// Store previous values for metadata
		previousFlags := rs.Flags
		previousLimit := rs.LowLimit
		if bHigh {
			previousLimit = rs.HighLimit
		}
		previousLowQualityIn := rs.LowQualityIn
		previousLowQualityOut := rs.LowQualityOut
		previousHighQualityIn := rs.HighQualityIn
		previousHighQualityOut := rs.HighQualityOut

		// Update the limit
		if !bHigh {
			rs.LowLimit = limitAmount
		} else {
			rs.HighLimit = limitAmount
		}

		// Handle Auth flag (can only be set, not cleared per rippled)
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
		// Per rippled: bSetFreeze && !bClearFreeze && !bNoFreeze -> set freeze
		//              bClearFreeze && !bSetFreeze -> clear freeze
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

		// Handle QualityIn
		if bQualityIn {
			if uQualityIn != 0 {
				// Setting quality
				if bHigh {
					rs.HighQualityIn = uQualityIn
				} else {
					rs.LowQualityIn = uQualityIn
				}
			} else {
				// Clearing quality (setting to default)
				if bHigh {
					rs.HighQualityIn = 0
				} else {
					rs.LowQualityIn = 0
				}
			}
		}

		// Handle QualityOut
		if bQualityOut {
			if uQualityOut != 0 {
				// Setting quality
				if bHigh {
					rs.HighQualityOut = uQualityOut
				} else {
					rs.LowQualityOut = uQualityOut
				}
			} else {
				// Clearing quality (setting to default)
				if bHigh {
					rs.HighQualityOut = 0
				} else {
					rs.LowQualityOut = 0
				}
			}
		}

		// Normalize quality values (QUALITY_ONE -> 0)
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
		// Per rippled: bDefault = both sides have no reserve requirement
		// Reserve is needed if: quality != 0 || noRipple differs from default || freeze || limit || balance > 0
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
			// Update reserve flags based on reserve requirements
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
			limitField := "LowLimit"
			if bHigh {
				limitField = "HighLimit"
			}

			finalFields := map[string]any{
				"Flags": rs.Flags,
			}
			previousFields := map[string]any{}

			// Only include limit in metadata if it changed
			if formatIOUValue(limitAmount.Value) != formatIOUValue(previousLimit.Value) {
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

			// Include flags if changed
			if previousFlags != rs.Flags {
				previousFields["Flags"] = previousFlags
			}

			// Include quality fields if changed
			if !bHigh {
				if previousLowQualityIn != rs.LowQualityIn {
					if rs.LowQualityIn != 0 {
						finalFields["LowQualityIn"] = rs.LowQualityIn
					}
					if previousLowQualityIn != 0 {
						previousFields["LowQualityIn"] = previousLowQualityIn
					}
				}
				if previousLowQualityOut != rs.LowQualityOut {
					if rs.LowQualityOut != 0 {
						finalFields["LowQualityOut"] = rs.LowQualityOut
					}
					if previousLowQualityOut != 0 {
						previousFields["LowQualityOut"] = previousLowQualityOut
					}
				}
			} else {
				if previousHighQualityIn != rs.HighQualityIn {
					if rs.HighQualityIn != 0 {
						finalFields["HighQualityIn"] = rs.HighQualityIn
					}
					if previousHighQualityIn != 0 {
						previousFields["HighQualityIn"] = previousHighQualityIn
					}
				}
				if previousHighQualityOut != rs.HighQualityOut {
					if rs.HighQualityOut != 0 {
						finalFields["HighQualityOut"] = rs.HighQualityOut
					}
					if previousHighQualityOut != 0 {
						previousFields["HighQualityOut"] = previousHighQualityOut
					}
				}
			}

			metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
				NodeType:        "ModifiedNode",
				LedgerEntryType: "RippleState",
				LedgerIndex:     hex.EncodeToString(trustLineKey.Key[:]),
				FinalFields:     finalFields,
				PreviousFields:  previousFields,
			})
		}
	}

	return TesSUCCESS
}

// applyOfferCreate applies an OfferCreate transaction
func (e *Engine) applyOfferCreate(offer *OfferCreate, account *AccountRoot, metadata *Metadata) Result {
	// Check if offer has expired
	// Reference: rippled CreateOffer.cpp:189-200 and 623-636
	if offer.Expiration != nil && *offer.Expiration > 0 {
		// The offer has expired if Expiration <= parent close time
		// parentCloseTime is the close time of the parent ledger
		parentCloseTime := e.config.ParentCloseTime
		if *offer.Expiration <= parentCloseTime {
			// Offer has expired - return tecEXPIRED
			// Note: in older rippled versions without featureDepositPreauth, this would return tesSUCCESS
			return TecEXPIRED
		}
	}

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
	// Per rippled, first 2 objects don't need extra reserve
	reserveCreate := e.ReserveForNewObject(account.OwnerCount)
	if account.Balance < reserveCreate {
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

	// Calculate quality (exchange rate) for the book directory
	quality := GetRate(remainingTakerPays, remainingTakerGets)

	// Get currency and issuer bytes for the book directory
	takerPaysCurrency := getCurrencyBytes(remainingTakerPays.Currency)
	takerPaysIssuer := getIssuerBytes(remainingTakerPays.Issuer)
	takerGetsCurrency := getCurrencyBytes(remainingTakerGets.Currency)
	takerGetsIssuer := getIssuerBytes(remainingTakerGets.Issuer)

	// Calculate the book directory key with quality
	bookBase := keylet.BookDir(takerPaysCurrency, takerPaysIssuer, takerGetsCurrency, takerGetsIssuer)
	bookDirKey := keylet.Quality(bookBase, quality)

	// Add offer to owner directory
	ownerDirKey := keylet.OwnerDir(accountID)
	ownerDirResult, err := e.dirInsert(ownerDirKey, offerKey.Key, func(dir *DirectoryNode) {
		dir.Owner = accountID
	})
	if err != nil {
		return TefINTERNAL
	}

	// Add offer to book directory
	bookDirResult, err := e.dirInsert(bookDirKey, offerKey.Key, func(dir *DirectoryNode) {
		dir.TakerPaysCurrency = takerPaysCurrency
		dir.TakerPaysIssuer = takerPaysIssuer
		dir.TakerGetsCurrency = takerGetsCurrency
		dir.TakerGetsIssuer = takerGetsIssuer
		dir.ExchangeRate = quality
	})
	if err != nil {
		return TefINTERNAL
	}

	// Create the ledger offer with directory info
	ledgerOffer := &LedgerOffer{
		Account:           account.Account,
		Sequence:          offerSequence,
		TakerPays:         remainingTakerPays,
		TakerGets:         remainingTakerGets,
		BookDirectory:     bookDirKey.Key,
		BookNode:          bookDirResult.Page,
		OwnerNode:         ownerDirResult.Page,
		Flags:             0,
		PreviousTxnID:     e.currentTxHash,
		PreviousTxnLgrSeq: e.config.LedgerSequence,
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

	// Add owner directory metadata
	if ownerDirResult.Created {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "DirectoryNode",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(ownerDirKey.Key[:])),
			NewFields: map[string]any{
				"Owner":     offer.Account,
				"RootIndex": strings.ToUpper(hex.EncodeToString(ownerDirKey.Key[:])),
			},
		})
	} else if ownerDirResult.Modified {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "ModifiedNode",
			LedgerEntryType: "DirectoryNode",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(ownerDirKey.Key[:])),
			FinalFields: map[string]any{
				"Flags":     uint32(0),
				"Owner":     offer.Account,
				"RootIndex": strings.ToUpper(hex.EncodeToString(ownerDirKey.Key[:])),
			},
		})
	}

	// Add book directory metadata
	if bookDirResult.Created {
		metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
			NodeType:        "CreatedNode",
			LedgerEntryType: "DirectoryNode",
			LedgerIndex:     strings.ToUpper(hex.EncodeToString(bookDirKey.Key[:])),
			NewFields: map[string]any{
				"ExchangeRate":      formatUint64Hex(quality),
				"RootIndex":         strings.ToUpper(hex.EncodeToString(bookDirKey.Key[:])),
				"TakerGetsCurrency": strings.ToUpper(hex.EncodeToString(takerGetsCurrency[:])),
				"TakerGetsIssuer":   strings.ToUpper(hex.EncodeToString(takerGetsIssuer[:])),
			},
		})
	}

	// Add offer metadata with BookDirectory
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "Offer",
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(offerKey.Key[:])),
		NewFields: map[string]any{
			"Account":       offer.Account,
			"BookDirectory": strings.ToUpper(hex.EncodeToString(bookDirKey.Key[:])),
			"Sequence":      offerSequence,
			"TakerGets":     flattenAmount(remainingTakerGets),
			"TakerPays":     flattenAmount(remainingTakerPays),
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
						Value:    formatIOUValue(payValue.Value),
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
				Value:    formatIOUValue(takeIOU.Value),
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
						Value:    formatIOUValue(payValue.Value),
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
					"TakerGets": flattenAmount(theirGets),
					"TakerPays": flattenAmount(theirPays),
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
						"TakerGets": flattenAmount(matchRemainingGets),
						"TakerPays": flattenAmount(matchRemainingPays),
					},
					PreviousFields: map[string]any{
						"TakerGets": flattenAmount(theirGets),
						"TakerPays": flattenAmount(theirPays),
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
		Value:    formatIOUValue(result.Value),
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
// RippleState balance semantics:
// - Negative balance = LOW owes HIGH (HIGH holds tokens)
// - Positive balance = HIGH owes LOW (LOW holds tokens)
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

	var newBalance IOUAmount
	if accountIsLow {
		// Account is LOW, issuer is HIGH
		// Positive balance = account holds tokens (HIGH owes LOW)
		if increase {
			newBalance = rs.Balance.Add(amount) // More positive = more holdings
		} else {
			newBalance = rs.Balance.Sub(amount) // Less positive = less holdings
		}
	} else {
		// Account is HIGH, issuer is LOW
		// Negative balance = account holds tokens (LOW owes HIGH)
		if increase {
			newBalance = rs.Balance.Sub(amount) // More negative = more holdings
		} else {
			newBalance = rs.Balance.Add(amount) // Less negative = less holdings
		}
	}
	// Ensure the new balance has the correct currency and issuer
	newBalance.Currency = amount.Currency
	newBalance.Issuer = amount.Issuer
	rs.Balance = newBalance

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
				"value":    formatIOUValue(rs.Balance.Value),
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
		Value:    formatIOUValue(result.Value),
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
