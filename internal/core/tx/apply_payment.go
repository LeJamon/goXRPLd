package tx

import (
	"fmt"
	"math"
	"math/big"
	"strconv"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// applyPayment applies a Payment transaction
func (e *Engine) applyPayment(payment *Payment, sender *AccountRoot, metadata *Metadata, view LedgerView) Result {
	// XRP-to-XRP payment (direct payment)
	if payment.Amount.IsNative() {
		return e.applyXRPPayment(payment, sender, metadata, view)
	}

	// IOU payment - more complex, involves trust lines and paths
	return e.applyIOUPayment(payment, sender, metadata, view)
}

// applyXRPPayment applies an XRP-to-XRP payment
// Reference: rippled/src/xrpld/app/tx/detail/Payment.cpp doApply() for XRP direct payments
func (e *Engine) applyXRPPayment(payment *Payment, sender *AccountRoot, metadata *Metadata, view LedgerView) Result {
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

	// IMPORTANT: sender.Balance has already had fee deducted (in doApply).
	// Rippled checks against mPriorBalance (balance BEFORE fee deduction).
	// We reconstruct the pre-fee balance for the check.
	// Reference: rippled Payment.cpp:619 - if (mPriorBalance < dstAmount.xrp() + mmm)
	priorBalance := sender.Balance + feeDrops

	// Calculate reserve as: ReserveBase + (ownerCount * ReserveIncrement)
	// This matches rippled's accountReserve(ownerCount) calculation
	reserve := e.config.ReserveBase + (uint64(sender.OwnerCount) * e.config.ReserveIncrement)

	// Use max(reserve, fee) as the minimum balance that must remain
	// This matches rippled's behavior: auto const mmm = std::max(reserve, ctx_.tx.getFieldAmount(sfFee).xrp())
	// Reference: rippled Payment.cpp:617
	mmm := reserve
	if feeDrops > mmm {
		mmm = feeDrops
	}

	// Check sender has enough balance using PRE-FEE balance
	// Reference: rippled Payment.cpp:619 - if (mPriorBalance < dstAmount.xrp() + mmm)
	if priorBalance < amountDrops+mmm {
		return TecUNFUNDED_PAYMENT
	}

	// Get destination account
	destAccountID, err := decodeAccountID(payment.Destination)
	if err != nil {
		return TemDST_NEEDED
	}
	destKey := keylet.Account(destAccountID)

	destExists, err := view.Exists(destKey)
	if err != nil {
		return TefINTERNAL
	}

	if destExists {
		// Destination exists - just credit the amount
		destData, err := view.Read(destKey)
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
				preauthExists, err := view.Exists(depositPreauthKey)
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

		// Update tracked automatically by ApplyStateTable
		if err := view.Update(destKey, updatedDestData); err != nil {
			return TefINTERNAL
		}

		return TesSUCCESS
	}

	// Destination doesn't exist - need to create it
	// Check minimum amount for account creation
	if amountDrops < e.config.ReserveBase {
		return TecNO_DST_INSUF_XRP
	}

	// Create new account
	// With featureDeletableAccounts enabled, new accounts start with sequence
	// equal to the current ledger sequence. Otherwise, sequence starts at 1.
	// (see rippled Payment.cpp:409-411)
	var accountSequence uint32
	if e.rules().DeletableAccountsEnabled() {
		accountSequence = e.config.LedgerSequence
	} else {
		accountSequence = 1
	}
	newAccount := &AccountRoot{
		Account:           payment.Destination,
		Balance:           amountDrops,
		Sequence:          accountSequence,
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

	// Insert tracked automatically by ApplyStateTable
	if err := view.Insert(destKey, newAccountData); err != nil {
		return TefINTERNAL
	}

	return TesSUCCESS
}

// applyIOUPayment applies an IOU (issued currency) payment
// Reference: rippled/src/xrpld/app/tx/detail/Payment.cpp
func (e *Engine) applyIOUPayment(payment *Payment, sender *AccountRoot, metadata *Metadata, view LedgerView) Result {
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

	// Detect payments that require RippleCalc (path finding)
	// Reference: rippled Payment.cpp:435-436:
	// bool const ripple = (hasPaths || sendMax || !dstAmount.native()) && !mptDirect;
	//
	// Payments that require path finding:
	// 1. Explicit paths in the transaction
	// 2. SendMax with different issuer than Amount (cross-issuer)
	//
	// Payments that DON'T require path finding (can be handled directly):
	// - When sender == Amount.issuer (issue): issuer creates tokens for recipient
	// - When dest == Amount.issuer AND no SendMax with different issuer (simple redemption)
	//
	// For now, we only support simple direct IOU payments (no path finding).
	// Return tecPATH_DRY for payments that require RippleCalc.

	// Determine payment type: is this a direct payment to/from issuer?
	senderIsIssuer := senderAccountID == issuerAccountID
	destIsIssuer := destAccountID == issuerAccountID

	requiresPathFinding := false

	// Check for explicit paths
	if payment.Paths != nil && len(payment.Paths) > 0 {
		requiresPathFinding = true
	}

	// Check for SendMax with cross-issuer
	// When SendMax.issuer == sender, it means "use my trust line balance" - rippled
	// determines the actual issuer from the sender's trust lines.
	// When SendMax.issuer is explicitly a different third party (not sender, not Amount.issuer),
	// that's a true cross-issuer payment requiring path finding.
	if payment.SendMax != nil && !senderIsIssuer {
		sendMaxIssuer := payment.SendMax.Issuer
		// True cross-issuer: SendMax.issuer is a specific third-party issuer
		// (not the sender, not the Amount.issuer)
		if sendMaxIssuer != "" &&
			sendMaxIssuer != payment.Amount.Issuer &&
			sendMaxIssuer != payment.Common.Account {
			requiresPathFinding = true
		}
	}

	// For path-finding payments, use the Flow Engine (RippleCalculate)
	if requiresPathFinding {
		return e.applyIOUPaymentWithPaths(payment, sender, senderAccountID, destAccountID, issuerAccountID, metadata, view)
	}

	// Check destination exists
	destKey := keylet.Account(destAccountID)
	destExists, err := view.Exists(destKey)
	if err != nil {
		return TefINTERNAL
	}
	if !destExists {
		return TecNO_DST
	}

	// Get destination account to check flags
	destData, err := view.Read(destKey)
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

	var result Result
	var deliveredAmount IOUAmount

	if senderIsIssuer {
		// Sender is issuing their own currency to destination
		// Need trust line from destination to sender (issuer)
		result, deliveredAmount = e.applyIOUIssueWithDelivered(payment, sender, destAccount, senderAccountID, destAccountID, amount, metadata, view)
	} else if destIsIssuer {
		// Destination is the issuer - sender is redeeming tokens
		// Need trust line from sender to destination (issuer)
		result, deliveredAmount = e.applyIOURedeemWithDelivered(payment, sender, destAccount, senderAccountID, destAccountID, amount, metadata, view)
	} else {
		// Neither is issuer - transfer between two non-issuer accounts
		// This requires trust lines from both parties to the issuer
		result, deliveredAmount = e.applyIOUTransferWithDelivered(payment, sender, destAccount, senderAccountID, destAccountID, issuerAccountID, amount, metadata, view)
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

// applyIOUPaymentWithPaths handles IOU payments that require path finding using the Flow Engine.
// This is the main entry point for cross-currency payments and payments with explicit paths.
// Reference: rippled/src/xrpld/app/paths/RippleCalc.cpp
func (e *Engine) applyIOUPaymentWithPaths(
	payment *Payment,
	sender *AccountRoot,
	senderID, destID, issuerID [20]byte,
	metadata *Metadata,
	view LedgerView,
) Result {
	// Determine payment flags
	flags := payment.GetFlags()
	partialPayment := (flags & PaymentFlagPartialPayment) != 0
	limitQuality := (flags & PaymentFlagLimitQuality) != 0
	noDirectRipple := (flags & PaymentFlagNoDirectRipple) != 0

	// addDefaultPath is true unless tfNoRippleDirect is set
	addDefaultPath := !noDirectRipple

	// Execute RippleCalculate
	_, actualOut, _, sandbox, result := RippleCalculate(
		view,
		senderID,
		destID,
		payment.Amount,
		payment.SendMax,
		payment.Paths,
		addDefaultPath,
		partialPayment,
		limitQuality,
		e.currentTxHash,
		e.config.LedgerSequence,
	)

	// Handle result
	if result != TesSUCCESS && result != TecPATH_PARTIAL {
		return result
	}

	// Apply sandbox changes back to the ledger view (through ApplyStateTable for tracking)
	if sandbox != nil {
		if err := sandbox.ApplyToView(view); err != nil {
			return TefINTERNAL
		}
	}

	// Check if partial payment delivered enough (DeliverMin)
	if partialPayment && payment.DeliverMin != nil {
		deliverMin := ToEitherAmount(*payment.DeliverMin)
		if actualOut.Compare(deliverMin) < 0 {
			return TecPATH_PARTIAL
		}
	}

	// Record delivered amount in metadata
	deliveredAmt := FromEitherAmount(actualOut)
	metadata.DeliveredAmount = &deliveredAmt

	// Offer deletions and trust line modifications tracked automatically by ApplyStateTable

	return result
}

// applyIOUIssue handles when sender is the issuer creating new tokens
func (e *Engine) applyIOUIssue(payment *Payment, sender *AccountRoot, dest *AccountRoot, senderID, destID [20]byte, amount IOUAmount, metadata *Metadata, view LedgerView) Result {
	// Look up the trust line between destination and issuer (sender)
	trustLineKey := keylet.Line(destID, senderID, amount.Currency)

	trustLineExists, err := view.Exists(trustLineKey)
	if err != nil {
		return TefINTERNAL
	}

	if !trustLineExists {
		// No trust line exists - destination has not authorized holding this currency
		return TecPATH_DRY
	}

	// Read and parse the trust line
	trustLineData, err := view.Read(trustLineKey)
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

	if err := view.Update(trustLineKey, updatedTrustLine); err != nil {
		return TefINTERNAL
	}

	// RippleState modification tracked automatically by ApplyStateTable

	delivered := payment.Amount
	metadata.DeliveredAmount = &delivered

	return TesSUCCESS
}

// applyIOURedeem handles when destination is the issuer (redeeming tokens)
func (e *Engine) applyIOURedeem(payment *Payment, sender *AccountRoot, dest *AccountRoot, senderID, destID [20]byte, amount IOUAmount, metadata *Metadata, view LedgerView) Result {
	// Look up the trust line between sender and issuer (destination)
	trustLineKey := keylet.Line(senderID, destID, amount.Currency)

	trustLineExists, err := view.Exists(trustLineKey)
	if err != nil {
		return TefINTERNAL
	}

	if !trustLineExists {
		// No trust line exists - sender doesn't hold this currency
		return TecPATH_DRY
	}

	// Read and parse the trust line
	trustLineData, err := view.Read(trustLineKey)
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

	rippleState.Balance = newBalance

	// Update PreviousTxnID and PreviousTxnLgrSeq to this transaction
	rippleState.PreviousTxnID = e.currentTxHash
	rippleState.PreviousTxnLgrSeq = e.config.LedgerSequence

	// Serialize and update
	updatedTrustLine, err := serializeRippleState(rippleState)
	if err != nil {
		return TefINTERNAL
	}

	if err := view.Update(trustLineKey, updatedTrustLine); err != nil {
		return TefINTERNAL
	}

	// RippleState modification tracked automatically by ApplyStateTable

	delivered := payment.Amount
	metadata.DeliveredAmount = &delivered

	return TesSUCCESS
}

// applyIOUTransfer handles transfer between two non-issuer accounts
func (e *Engine) applyIOUTransfer(payment *Payment, sender *AccountRoot, dest *AccountRoot, senderID, destID, issuerID [20]byte, amount IOUAmount, metadata *Metadata, view LedgerView) Result {
	// Both sender and destination need trust lines to the issuer
	// This is a simplified implementation - full path finding is more complex

	// Get sender's trust line to issuer
	senderTrustLineKey := keylet.Line(senderID, issuerID, amount.Currency)
	senderTrustExists, err := view.Exists(senderTrustLineKey)
	if err != nil {
		return TefINTERNAL
	}
	if !senderTrustExists {
		return TecPATH_DRY
	}

	// Get destination's trust line to issuer
	destTrustLineKey := keylet.Line(destID, issuerID, amount.Currency)
	destTrustExists, err := view.Exists(destTrustLineKey)
	if err != nil {
		return TefINTERNAL
	}
	if !destTrustExists {
		return TecPATH_DRY
	}

	// Read sender's trust line
	senderTrustData, err := view.Read(senderTrustLineKey)
	if err != nil {
		return TefINTERNAL
	}
	senderRippleState, err := parseRippleState(senderTrustData)
	if err != nil {
		return TefINTERNAL
	}

	// Read destination's trust line
	destTrustData, err := view.Read(destTrustLineKey)
	if err != nil {
		return TefINTERNAL
	}
	destRippleState, err := parseRippleState(destTrustData)
	if err != nil {
		return TefINTERNAL
	}

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
	if err := view.Update(senderTrustLineKey, updatedSenderTrust); err != nil {
		return TefINTERNAL
	}

	// Serialize and update destination's trust line
	updatedDestTrust, err := serializeRippleState(destRippleState)
	if err != nil {
		return TefINTERNAL
	}
	if err := view.Update(destTrustLineKey, updatedDestTrust); err != nil {
		return TefINTERNAL
	}

	// RippleState modifications tracked automatically by ApplyStateTable

	delivered := payment.Amount
	metadata.DeliveredAmount = &delivered

	return TesSUCCESS
}

// applyIOUIssueWithDelivered wraps applyIOUIssue to return the delivered amount
func (e *Engine) applyIOUIssueWithDelivered(payment *Payment, sender *AccountRoot, dest *AccountRoot, senderID, destID [20]byte, amount IOUAmount, metadata *Metadata, view LedgerView) (Result, IOUAmount) {
	result := e.applyIOUIssue(payment, sender, dest, senderID, destID, amount, metadata, view)
	if result == TesSUCCESS {
		// For successful issue, the full amount is delivered
		return result, amount
	}
	return result, IOUAmount{}
}

// applyIOURedeemWithDelivered wraps applyIOURedeem to return the delivered amount
func (e *Engine) applyIOURedeemWithDelivered(payment *Payment, sender *AccountRoot, dest *AccountRoot, senderID, destID [20]byte, amount IOUAmount, metadata *Metadata, view LedgerView) (Result, IOUAmount) {
	result := e.applyIOURedeem(payment, sender, dest, senderID, destID, amount, metadata, view)
	if result == TesSUCCESS {
		// For successful redeem, the full amount is delivered
		return result, amount
	}
	return result, IOUAmount{}
}

// applyIOUTransferWithDelivered wraps applyIOUTransfer to return the delivered amount
func (e *Engine) applyIOUTransferWithDelivered(payment *Payment, sender *AccountRoot, dest *AccountRoot, senderID, destID, issuerID [20]byte, amount IOUAmount, metadata *Metadata, view LedgerView) (Result, IOUAmount) {
	result := e.applyIOUTransfer(payment, sender, dest, senderID, destID, issuerID, amount, metadata, view)
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

// getTransferRate returns the transfer rate for an issuer account
// Returns qualityOne (1e9) if no transfer rate is set
func (e *Engine) getTransferRate(issuerAddress string) uint32 {
	issuerID, err := decodeAccountID(issuerAddress)
	if err != nil {
		return qualityOne
	}
	issuerKey := keylet.Account(issuerID)
	issuerData, err := e.view.Read(issuerKey)
	if err != nil {
		return qualityOne
	}
	issuerAccount, err := parseAccountRoot(issuerData)
	if err != nil {
		return qualityOne
	}
	if issuerAccount.TransferRate > 0 {
		return issuerAccount.TransferRate
	}
	return qualityOne
}

// getAccountIOUBalance returns the IOU balance an account holds for a specific currency/issuer
// Returns the balance from the trust line, accounting for which side is low/high
func (e *Engine) getAccountIOUBalance(accountAddress string, currency string, issuerAddress string) IOUAmount {
	accountID, err := decodeAccountID(accountAddress)
	if err != nil {
		return IOUAmount{Value: big.NewFloat(0), Currency: currency, Issuer: issuerAddress}
	}
	issuerID, err := decodeAccountID(issuerAddress)
	if err != nil {
		return IOUAmount{Value: big.NewFloat(0), Currency: currency, Issuer: issuerAddress}
	}

	trustLineKey := keylet.Line(accountID, issuerID, currency)
	trustLineData, err := e.view.Read(trustLineKey)
	if err != nil {
		return IOUAmount{Value: big.NewFloat(0), Currency: currency, Issuer: issuerAddress}
	}

	rs, err := parseRippleState(trustLineData)
	if err != nil {
		return IOUAmount{Value: big.NewFloat(0), Currency: currency, Issuer: issuerAddress}
	}

	// Determine account's balance based on low/high position
	// Balance is stored from low's perspective:
	// - Negative balance = low owes high (high holds tokens)
	// - Positive balance = high owes low (low holds tokens)
	accountIsLow := compareAccountIDsForLine(accountID, issuerID) < 0

	balance := rs.Balance
	if !accountIsLow {
		// Account is HIGH, negate to get their perspective
		// If balance is negative (low owes high), account holds tokens
		balance = balance.Negate()
	}

	// Positive balance means account holds tokens
	balance.Currency = currency
	balance.Issuer = issuerAddress
	return balance
}

// applyTransferFee applies the transfer fee to an amount
// Used when sending IOUs through offers
func applyTransferFee(amount IOUAmount, transferRate uint32) IOUAmount {
	if transferRate == qualityOne || transferRate == 0 {
		return amount
	}

	// Transfer rate is expressed as fraction of 1e9
	// Example: 1.01 (1% fee) = 1010000000
	// To apply: multiply amount by (transferRate / 1e9)
	// Use big.Float for full precision
	rate := new(big.Float).SetPrec(128).SetUint64(uint64(transferRate))
	one := new(big.Float).SetPrec(128).SetUint64(uint64(qualityOne))
	rateRatio := new(big.Float).SetPrec(128).Quo(rate, one)

	amountValue := new(big.Float).SetPrec(128).Set(amount.Value)
	adjustedValue := new(big.Float).SetPrec(128).Mul(amountValue, rateRatio)

	return IOUAmount{
		Value:    adjustedValue,
		Currency: amount.Currency,
		Issuer:   amount.Issuer,
	}
}

// removeTransferFee removes the transfer fee from an amount
// Used to calculate the actual amount received after fees
func removeTransferFee(amount IOUAmount, transferRate uint32) IOUAmount {
	if transferRate == qualityOne || transferRate == 0 {
		return amount
	}

	// To remove fee: divide amount by (transferRate / 1e9)
	// Use big.Float for full precision
	rate := new(big.Float).SetPrec(128).SetUint64(uint64(transferRate))
	one := new(big.Float).SetPrec(128).SetUint64(uint64(qualityOne))
	rateRatio := new(big.Float).SetPrec(128).Quo(rate, one)

	amountValue := new(big.Float).SetPrec(128).Set(amount.Value)
	adjustedValue := new(big.Float).SetPrec(128).Quo(amountValue, rateRatio)

	return IOUAmount{
		Value:    adjustedValue,
		Currency: amount.Currency,
		Issuer:   amount.Issuer,
	}
}

// applyAccountSet applies an AccountSet transaction
// Reference: rippled SetAccount.cpp doApply()
func (e *Engine) applyAccountSet(accountSet *AccountSet, account *AccountRoot, view LedgerView) Result {
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
func (e *Engine) applyTrustSet(trustSet *TrustSet, account *AccountRoot, view LedgerView) Result {
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

	trustLineExists, err := view.Exists(trustLineKey)
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
	bSetDeepFreeze := (txFlags & TrustSetFlagSetDeepFreeze) != 0
	bClearDeepFreeze := (txFlags & TrustSetFlagClearDeepFreeze) != 0

	// Validate tfSetfAuth - requires issuer to have lsfRequireAuth set
	// Per rippled SetTrust.cpp preclaim: if bSetAuth && !(account.Flags & lsfRequireAuth) -> tefNO_AUTH_REQUIRED
	if bSetAuth && (account.Flags&lsfRequireAuth) == 0 {
		return TefNO_AUTH_REQUIRED
	}

	// Validate freeze flags - cannot freeze if account has lsfNoFreeze set
	// Per rippled SetTrust.cpp preclaim: if bNoFreeze && (bSetFreeze || bSetDeepFreeze) -> tecNO_PERMISSION
	bNoFreeze := (account.Flags & lsfNoFreeze) != 0
	if bNoFreeze && (bSetFreeze || bSetDeepFreeze) {
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
	// Per rippled SetTrust.cpp: saLimitAllow.setIssuer(account_)
	// The issuer of the limit is the account setting the trust line, not the LimitAmount.Issuer
	limitAmount := NewIOUAmount(trustSet.LimitAmount.Value, trustSet.LimitAmount.Currency, account.Account)

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
			Balance:           NewIOUAmount("0", trustSet.LimitAmount.Currency, trustSet.LimitAmount.Issuer),
			Flags:             0,
			LowNode:           0,
			HighNode:          0,
			PreviousTxnID:     e.currentTxHash,
			PreviousTxnLgrSeq: e.config.LedgerSequence,
		}

		// Set the limit based on which side this account is
		// Per rippled trustCreate: limit issuers must be the respective account
		// LowLimit issuer = LOW account, HighLimit issuer = HIGH account
		if !bHigh {
			// Account is LOW, LimitAmount.Issuer is HIGH
			rs.LowLimit = limitAmount                                                                    // issuer = account.Account (LOW)
			rs.HighLimit = NewIOUAmount("0", trustSet.LimitAmount.Currency, trustSet.LimitAmount.Issuer) // issuer = HIGH
			rs.Flags |= lsfLowReserve
		} else {
			// Account is HIGH, LimitAmount.Issuer is LOW
			rs.LowLimit = NewIOUAmount("0", trustSet.LimitAmount.Currency, trustSet.LimitAmount.Issuer) // issuer = LOW
			rs.HighLimit = limitAmount                                                                  // issuer = account.Account (HIGH)
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
		// Per rippled computeFreezeFlags:
		//   if bSetFreeze && !bClearFreeze && !bNoFreeze -> set freeze
		if bSetFreeze && !bClearFreeze && !bNoFreeze {
			if bHigh {
				rs.Flags |= lsfHighFreeze
			} else {
				rs.Flags |= lsfLowFreeze
			}
		}

		// Handle DeepFreeze flag for new trust line
		// Per rippled computeFreezeFlags:
		//   if bSetDeepFreeze && !bClearDeepFreeze && !bNoFreeze -> set deep freeze
		if bSetDeepFreeze && !bClearDeepFreeze && !bNoFreeze {
			if bHigh {
				rs.Flags |= lsfHighDeepFreeze
			} else {
				rs.Flags |= lsfLowDeepFreeze
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

		// Determine the LOW and HIGH account IDs for directory operations
		// Low account is the one with the smaller account ID
		var lowAccountID, highAccountID [20]byte
		if !bHigh {
			// Current account is LOW
			lowAccountID = accountID
			highAccountID = issuerAccountID
		} else {
			// Current account is HIGH
			lowAccountID = issuerAccountID
			highAccountID = accountID
		}

		// Add trust line to LOW account's owner directory
		// Per rippled View.cpp trustCreate: insert into both accounts' directories
		lowDirKey := keylet.OwnerDir(lowAccountID)
		lowDirResult, err := e.dirInsert(view, lowDirKey, trustLineKey.Key, func(dir *DirectoryNode) {
			dir.Owner = lowAccountID
		})
		if err != nil {
			return TefINTERNAL
		}

		// Add trust line to HIGH account's owner directory
		highDirKey := keylet.OwnerDir(highAccountID)
		highDirResult, err := e.dirInsert(view, highDirKey, trustLineKey.Key, func(dir *DirectoryNode) {
			dir.Owner = highAccountID
		})
		if err != nil {
			return TefINTERNAL
		}

		// Set LowNode and HighNode on the RippleState (deletion hints)
		rs.LowNode = lowDirResult.Page
		rs.HighNode = highDirResult.Page

		// Serialize and insert the trust line
		trustLineData, err := serializeRippleState(rs)
		if err != nil {
			return TefINTERNAL
		}

		if err := view.Insert(trustLineKey, trustLineData); err != nil {
			return TefINTERNAL
		}

		// Increment owner count for the transaction sender
		account.OwnerCount++

		// Directory, RippleState creation, and issuer account modifications tracked automatically by ApplyStateTable
	} else {
		// Modify existing trust line
		trustLineData, err := view.Read(trustLineKey)
		if err != nil {
			return TefINTERNAL
		}

		rs, err := parseRippleState(trustLineData)
		if err != nil {
			return TefINTERNAL
		}

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
		// Per rippled SetTrust.cpp:577-584:
		// NoRipple can only be set if the balance from this account's perspective >= 0
		// Balance is stored from LOW account's perspective:
		//   - Positive balance means LOW owes HIGH
		//   - Negative balance means HIGH owes LOW
		// From HIGH's perspective: saHighBalance = -rs.Balance, so check rs.Balance <= 0
		// From LOW's perspective: saLowBalance = rs.Balance, so check rs.Balance >= 0
		if bSetNoRipple && !bClearNoRipple {
			// Check if balance from this account's perspective is >= 0
			balanceFromPerspective := true // Assume can set
			if rs.Balance.Value != nil {
				if bHigh {
					// HIGH account: balance from HIGH's perspective is >= 0 if stored balance <= 0
					balanceFromPerspective = rs.Balance.Value.Sign() <= 0
				} else {
					// LOW account: balance from LOW's perspective is >= 0 if stored balance >= 0
					balanceFromPerspective = rs.Balance.Value.Sign() >= 0
				}
			}
			// Only set NoRipple if balance from our perspective is non-negative
			if balanceFromPerspective {
				if bHigh {
					rs.Flags |= lsfHighNoRipple
				} else {
					rs.Flags |= lsfLowNoRipple
				}
			}
			// Note: If fix1578 amendment is enabled and balance < 0, we should return tecNO_PERMISSION
			// For now, we match pre-fix1578 behavior: silently don't set the flag
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

		// Handle DeepFreeze flag
		// Per rippled computeFreezeFlags:
		//   if bSetDeepFreeze && !bClearDeepFreeze && !bNoFreeze -> set deep freeze
		//   if bClearDeepFreeze && !bSetDeepFreeze -> clear deep freeze
		if bSetDeepFreeze && !bClearDeepFreeze && !bNoFreeze {
			if bHigh {
				rs.Flags |= lsfHighDeepFreeze
			} else {
				rs.Flags |= lsfLowDeepFreeze
			}
		} else if bClearDeepFreeze && !bSetDeepFreeze {
			if bHigh {
				rs.Flags &^= lsfHighDeepFreeze
			} else {
				rs.Flags &^= lsfLowDeepFreeze
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
			if err := view.Erase(trustLineKey); err != nil {
				return TefINTERNAL
			}

			// Decrement owner count
			if account.OwnerCount > 0 {
				account.OwnerCount--
			}

			// RippleState deletion tracked automatically by ApplyStateTable
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

			if err := view.Update(trustLineKey, updatedData); err != nil {
				return TefINTERNAL
			}

			// RippleState modification tracked automatically by ApplyStateTable
		}
	}

	return TesSUCCESS
}

// applyOfferCreate applies an OfferCreate transaction
func (e *Engine) applyOfferCreate(offer *OfferCreate, account *AccountRoot, view LedgerView) Result {
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
		exists, _ := view.Exists(oldOfferKey)
		if exists {
			// Delete the old offer - tracked automatically by ApplyStateTable
			if err := view.Erase(oldOfferKey); err == nil {
				if account.OwnerCount > 0 {
					account.OwnerCount--
				}
			}
		}
	}

	// Get the amounts
	takerGets := offer.TakerGets
	takerPays := offer.TakerPays

	// Check if account has funds to back the offer
	// Reference: rippled CreateOffer.cpp:172-178 (preclaim)
	// accountFunds checks if the account has ANY available funds for TakerGets
	// For XRP: available = balance - current_reserve
	// If available <= 0, return tecUNFUNDED_OFFER
	if takerGets.IsNative() {
		// For XRP offers, check if balance exceeds current reserve
		currentReserve := e.config.ReserveBase + e.config.ReserveIncrement*uint64(account.OwnerCount)
		if account.Balance <= currentReserve {
			return TecUNFUNDED_OFFER
		}
	} else {
		// For IOU offers, check if account has any of the token
		// Reference: rippled CreateOffer.cpp preclaim - accountFunds(... saTakerGets ...)
		// If account IS the issuer, they have unlimited funds
		accountID, _ := decodeAccountID(account.Account)
		issuerID, _ := decodeAccountID(takerGets.Issuer)
		if accountID != issuerID {
			// Check trust line balance
			trustLineKey := keylet.Line(accountID, issuerID, takerGets.Currency)
			trustLineData, err := view.Read(trustLineKey)
			if err != nil {
				// No trust line = no funds
				return TecUNFUNDED_OFFER
			}
			rs, err := parseRippleState(trustLineData)
			if err != nil {
				return TecUNFUNDED_OFFER
			}
			// Determine balance from account's perspective
			accountIsLow := compareAccountIDsForLine(accountID, issuerID) < 0
			balance := rs.Balance
			if !accountIsLow {
				balance = balance.Negate()
			}
			// If balance <= 0, account has no funds
			if balance.Value.Sign() <= 0 {
				return TecUNFUNDED_OFFER
			}
		}
	}

	// Check account has reserve for new offer
	// Per rippled, first 2 objects don't need extra reserve
	reserveCreate := e.ReserveForNewObject(account.OwnerCount)
	if account.Balance < reserveCreate {
		return TecINSUF_RESERVE_OFFER
	}

	// Check for ImmediateOrCancel or FillOrKill flags
	flags := offer.GetFlags()
	isPassive := (flags & OfferCreateFlagPassive) != 0
	isIOC := (flags & OfferCreateFlagImmediateOrCancel) != 0
	isFOK := (flags & OfferCreateFlagFillOrKill) != 0

	// Track how much was filled
	var takerGotTotal Amount
	var takerPaidTotal Amount

	// Simple order matching - look for crossing offers
	// This is a simplified implementation that checks if there are any offers to match
	if !isPassive {
		takerGotTotal, takerPaidTotal = e.matchOffers(offer, account, view)
		// Metadata consolidation now handled by ApplyStateTable
	}

	// Check if fully filled
	// XRPL semantics: TakerPays = what taker receives, TakerGets = what taker pays
	// takerGotTotal is what we received from matching (should compare with TakerPays)
	// takerPaidTotal is what we paid to matches (should compare with TakerGets)
	fullyFilled := false
	if takerGotTotal.Value != "" && takerPays.Value != "" {
		// Compare what we received with what we wanted to receive
		if takerPays.IsNative() {
			gotDrops, _ := parseDropsString(takerGotTotal.Value)
			wantDrops, _ := parseDropsString(takerPays.Value)
			fullyFilled = gotDrops >= wantDrops
		} else {
			gotIOU := NewIOUAmount(takerGotTotal.Value, takerGotTotal.Currency, takerGotTotal.Issuer)
			wantIOU := NewIOUAmount(takerPays.Value, takerPays.Currency, takerPays.Issuer)
			fullyFilled = gotIOU.Compare(wantIOU) >= 0
		}
	}

	// Also check if we exhausted what we're selling (TakerGets)
	// This can happen when transfer fees cause us to pay more than takerPaidTotal reports
	// (takerPaidTotal is what makers received, not what taker actually sent including fees)
	sellExhausted := false
	if takerPaidTotal.Value != "" && takerGets.Value != "" {
		if takerGets.IsNative() {
			// XRP has no transfer fee
			paidDrops, _ := parseDropsString(takerPaidTotal.Value)
			sellDrops, _ := parseDropsString(takerGets.Value)
			sellExhausted = paidDrops >= sellDrops
		} else {
			// IOU - need to account for transfer fee
			// Taker sent = takerPaidTotal * transferRate
			// If taker sent >= takerGets, we're exhausted
			paidIOU := NewIOUAmount(takerPaidTotal.Value, takerPaidTotal.Currency, takerPaidTotal.Issuer)
			sellIOU := NewIOUAmount(takerGets.Value, takerGets.Currency, takerGets.Issuer)
			transferRate := e.getTransferRate(takerGets.Issuer)
			takerSentIOU := applyTransferFee(paidIOU, transferRate)
			sellExhausted = takerSentIOU.Compare(sellIOU) >= 0
		}
	}

	// Fully filled if we got what we wanted OR exhausted what we're selling
	if sellExhausted {
		fullyFilled = true
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
	// XRPL semantics: TakerPays = what taker receives, TakerGets = what taker pays
	// takerGotTotal = what we received (same currency as TakerPays)
	// takerPaidTotal = what we paid (same currency as TakerGets)
	// IMPORTANT: rippled calculates remainder at ORIGINAL quality to maintain consistency
	// Reference: rippled CreateOffer.cpp - remainder uses original offer's exchange rate
	remainingTakerGets := takerGets
	remainingTakerPays := takerPays
	if takerGotTotal.Value != "" {
		// Subtract what was already received from what we want to receive
		remainingTakerPays = subtractAmount(takerPays, takerGotTotal)
		// Calculate remaining TakerGets at original quality (not simple subtraction)
		// Quality = TakerPays / TakerGets
		// remainingTakerGets = remainingTakerPays / quality
		originalQuality := calculateQuality(takerPays, takerGets)
		if originalQuality > 0 {
			if remainingTakerPays.IsNative() {
				// XRP remaining
				remainingDrops, _ := parseDropsString(remainingTakerPays.Value)
				remainingGetsValue := float64(remainingDrops) / originalQuality
				if takerGets.IsNative() {
					remainingTakerGets = Amount{Value: formatDrops(uint64(remainingGetsValue))}
				} else {
					remainingTakerGets = Amount{
						Value:    formatIOUValuePrecise(remainingGetsValue),
						Currency: takerGets.Currency,
						Issuer:   takerGets.Issuer,
					}
				}
			} else {
				// IOU remaining
				remainingIOU := NewIOUAmount(remainingTakerPays.Value, remainingTakerPays.Currency, remainingTakerPays.Issuer)
				remainingPaysVal, _ := remainingIOU.Value.Float64()
				remainingGetsValue := remainingPaysVal / originalQuality
				if takerGets.IsNative() {
					remainingTakerGets = Amount{Value: formatDrops(uint64(remainingGetsValue))}
				} else {
					remainingTakerGets = Amount{
						Value:    formatIOUValuePrecise(remainingGetsValue),
						Currency: takerGets.Currency,
						Issuer:   takerGets.Issuer,
					}
				}
			}
		}
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
	ownerDirResult, err := e.dirInsert(view, ownerDirKey, offerKey.Key, func(dir *DirectoryNode) {
		dir.Owner = accountID
	})
	if err != nil {
		return TefINTERNAL
	}

	// Add offer to book directory
	bookDirResult, err := e.dirInsert(view, bookDirKey, offerKey.Key, func(dir *DirectoryNode) {
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

	if err := view.Insert(offerKey, offerData); err != nil {
		return TefINTERNAL
	}

	// Increment owner count
	account.OwnerCount++

	// Directory, book, and offer metadata tracked automatically by ApplyStateTable

	return TesSUCCESS
}

// matchOffers attempts to match the new offer against existing offers
// Returns the amounts obtained and paid through matching
func (e *Engine) matchOffers(offer *OfferCreate, account *AccountRoot, view LedgerView) (takerGot, takerPaid Amount) {
	// Find matching offers by scanning the ledger
	// This is a simplified implementation - production would use book directories

	// XRPL Offer semantics (from offer CREATOR's perspective):
	// - TakerGets = what creator is SELLING
	// - TakerPays = what creator is BUYING
	//
	// Our offer: TakerGets=BTC (selling BTC), TakerPays=XRP (buying XRP)
	// Their offer: TakerGets=XRP (selling XRP), TakerPays=BTC (buying BTC)
	//
	// We want to find offers where:
	// - Their TakerGets (what they're selling) matches our TakerPays (what we want to buy)
	// - Their TakerPays (what they're buying) matches our TakerGets (what we're selling)

	// What we want to BUY (receive)
	wantCurrency := offer.TakerPays.Currency
	wantIssuer := offer.TakerPays.Issuer
	// What we're SELLING (paying)
	payCurrency := offer.TakerGets.Currency
	payIssuer := offer.TakerGets.Issuer

	// Determine if matching native XRP
	wantingXRP := offer.TakerPays.IsNative() // We want to receive XRP
	payingXRP := offer.TakerGets.IsNative()  // We're paying XRP

	// Collect matching offers
	type matchOffer struct {
		key     [32]byte
		offer   *LedgerOffer
		quality float64 // TakerPays/TakerGets (lower is better for us)
	}
	var matches []matchOffer

	// Iterate through ledger entries to find offers
	view.ForEach(func(key [32]byte, data []byte) bool {
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

		// Check if this offer crosses with ours:
		// - Their TakerGets (what they're selling) = what we want to buy (our TakerPays)
		// - Their TakerPays (what they're buying) = what we're selling (our TakerGets)
		theirGetsMatchesWhatWeWant := false
		if wantingXRP && ledgerOffer.TakerGets.IsNative() {
			theirGetsMatchesWhatWeWant = true
		} else if !wantingXRP && !ledgerOffer.TakerGets.IsNative() {
			theirGetsMatchesWhatWeWant = ledgerOffer.TakerGets.Currency == wantCurrency &&
				ledgerOffer.TakerGets.Issuer == wantIssuer
		}

		theirPaysMatchesWhatWeSell := false
		if payingXRP && ledgerOffer.TakerPays.IsNative() {
			theirPaysMatchesWhatWeSell = true
		} else if !payingXRP && !ledgerOffer.TakerPays.IsNative() {
			theirPaysMatchesWhatWeSell = ledgerOffer.TakerPays.Currency == payCurrency &&
				ledgerOffer.TakerPays.Issuer == payIssuer
		}

		if !theirGetsMatchesWhatWeWant || !theirPaysMatchesWhatWeSell {
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
	remainingWant := offer.TakerPays // How much we still want to BUY (receive)
	remainingPay := offer.TakerGets  // How much we can still SELL (pay)

	for _, match := range matches {

		// Check if price crosses (their quality <= our inverse quality)
		// For us: we want high TakerGets, low TakerPays
		// For them: they want high TakerGets, low TakerPays
		// Match if: their_price <= 1/our_price
		// Use a small tolerance to account for floating-point precision issues
		// rippled uses integer-based quality which avoids this problem
		crossingThreshold := 1.0 / ourQuality
		// Tolerance: allow up to 1e-14 relative error (about 15 decimal digits precision)
		tolerance := crossingThreshold * 1e-10
		if match.quality > crossingThreshold+tolerance {
			continue // Price doesn't cross
		}

		// Calculate how much we can trade
		// theirGets = what they're SELLING = what we RECEIVE
		// theirPays = what they're BUYING = what we PAY
		originalTheirGets := match.offer.TakerGets
		originalTheirPays := match.offer.TakerPays
		theirGets := originalTheirGets
		theirPays := originalTheirPays

		// IMPORTANT: Limit amounts by actual balances
		// Reference: rippled OfferStream.cpp - checks ownerFunds to limit offers

		// 1. Check maker's funds for what they're selling (theirGets)
		// If maker is selling IOU, check their IOU balance
		if !theirGets.IsNative() && match.offer.Account != theirGets.Issuer {
			makerIOUBalance := e.getAccountIOUBalance(match.offer.Account, theirGets.Currency, theirGets.Issuer)
			if makerIOUBalance.Value.Sign() > 0 {
				offerGetsIOU := NewIOUAmount(theirGets.Value, theirGets.Currency, theirGets.Issuer)
				if makerIOUBalance.Compare(offerGetsIOU) < 0 {
					// Maker has less than offer amount - scale down proportionally
					ratio := divideIOUAmounts(makerIOUBalance, offerGetsIOU)
					theirGets = Amount{
						Value:    formatIOUValue(makerIOUBalance.Value),
						Currency: theirGets.Currency,
						Issuer:   theirGets.Issuer,
					}
					// Scale theirPays proportionally
					if theirPays.IsNative() {
						theirPaysDrops, _ := parseDropsString(theirPays.Value)
						scaledPays := uint64(float64(theirPaysDrops) * ratio)
						theirPays = Amount{Value: formatDrops(scaledPays)}
					} else {
						theirPaysIOU := NewIOUAmount(theirPays.Value, theirPays.Currency, theirPays.Issuer)
						scaledPays := multiplyIOUByRatio(theirPaysIOU, ratio)
						theirPays = Amount{
							Value:    formatIOUValue(scaledPays.Value),
							Currency: theirPays.Currency,
							Issuer:   theirPays.Issuer,
						}
					}
				}
			}
		}

		// 2. Check taker's funds for what they're paying (theirPays = what maker wants = what taker pays)
		// If taker is paying IOU, check their IOU balance and apply transfer fee
		// Reference: rippled applies transfer fee when IOUs move through issuer
		if !theirPays.IsNative() && account.Account != theirPays.Issuer {
			takerIOUBalance := e.getAccountIOUBalance(account.Account, theirPays.Currency, theirPays.Issuer)
			if takerIOUBalance.Value.Sign() > 0 {
				// Get the transfer rate for this IOU's issuer
				transferRate := e.getTransferRate(theirPays.Issuer)

				// Calculate effective amount maker will receive after transfer fee
				// effective = taker_balance / transfer_rate
				// This is the MAX the taker can deliver to the maker
				effectiveBalance := removeTransferFee(takerIOUBalance, transferRate)

				// The trade is limited by the taker's effective balance
				// If effectiveBalance < theirPays (what maker's offer portion wants), scale down
				offerPaysIOU := NewIOUAmount(theirPays.Value, theirPays.Currency, theirPays.Issuer)
				if effectiveBalance.Compare(offerPaysIOU) < 0 {
					// The maker will only receive effectiveBalance amount
					// Scale the exchange proportionally
					ratio := divideIOUAmounts(effectiveBalance, offerPaysIOU)
					theirPays = Amount{
						Value:    formatIOUValue(effectiveBalance.Value),
						Currency: theirPays.Currency,
						Issuer:   theirPays.Issuer,
					}
					// Scale theirGets proportionally (round up to match rippled's mulRound)
					if theirGets.IsNative() {
						theirGetsDrops, _ := parseDropsString(theirGets.Value)
						scaledGetsFloat := float64(theirGetsDrops) * ratio
						scaledGets := uint64(math.Ceil(scaledGetsFloat))
						theirGets = Amount{Value: formatDrops(scaledGets)}
					} else {
						theirGetsIOU := NewIOUAmount(theirGets.Value, theirGets.Currency, theirGets.Issuer)
						scaledGets := multiplyIOUByRatio(theirGetsIOU, ratio)
						theirGets = Amount{
							Value:    formatIOUValue(scaledGets.Value),
							Currency: theirGets.Currency,
							Issuer:   theirGets.Issuer,
						}
					}
				}
			}
		}

		// We want to receive as much as possible up to remainingWant (from their TakerGets)
		// We'll pay proportionally based on their exchange rate
		var gotAmount, paidAmount Amount

		if theirGets.IsNative() {
			// They're selling XRP (we receive XRP)
			theirGetsDrops, _ := parseDropsString(theirGets.Value)
			remainingWantDrops, _ := parseDropsString(remainingWant.Value)

			takeDrops := theirGetsDrops
			if takeDrops > remainingWantDrops {
				takeDrops = remainingWantDrops
			}

			gotAmount = Amount{Value: formatDrops(takeDrops)}

			// Calculate what we pay based on their rate
			// Rate: theirPays / theirGets (what they want per unit they sell)
			if takeDrops == theirGetsDrops {
				paidAmount = theirPays
			} else {
				// Partial fill - calculate proportionally
				ratio := float64(takeDrops) / float64(theirGetsDrops)
				if theirPays.IsNative() {
					theirPaysDrops, _ := parseDropsString(theirPays.Value)
					payDrops := uint64(float64(theirPaysDrops) * ratio)
					paidAmount = Amount{Value: formatDrops(payDrops)}
				} else {
					theirPaysIOU := NewIOUAmount(theirPays.Value, theirPays.Currency, theirPays.Issuer)
					payValue := multiplyIOUByRatio(theirPaysIOU, ratio)
					paidAmount = Amount{
						Value:    formatIOUValue(payValue.Value),
						Currency: theirPays.Currency,
						Issuer:   theirPays.Issuer,
					}
				}
			}
		} else {
			// They're selling IOU (we receive IOU)
			theirGetsIOU := NewIOUAmount(theirGets.Value, theirGets.Currency, theirGets.Issuer)
			remainingWantIOU := NewIOUAmount(remainingWant.Value, remainingWant.Currency, remainingWant.Issuer)

			takeIOU := theirGetsIOU
			if takeIOU.Compare(remainingWantIOU) > 0 {
				takeIOU = remainingWantIOU
			}

			gotAmount = Amount{
				Value:    formatIOUValue(takeIOU.Value),
				Currency: theirGets.Currency,
				Issuer:   theirGets.Issuer,
			}

			// Calculate what we pay
			if takeIOU.Compare(theirGetsIOU) == 0 {
				paidAmount = theirPays
			} else {
				// Partial fill
				ratio := divideIOUAmounts(takeIOU, theirGetsIOU)
				if theirPays.IsNative() {
					theirPaysDrops, _ := parseDropsString(theirPays.Value)
					payDrops := uint64(float64(theirPaysDrops) * ratio)
					paidAmount = Amount{Value: formatDrops(payDrops)}
				} else {
					theirPaysIOU := NewIOUAmount(theirPays.Value, theirPays.Currency, theirPays.Issuer)
					payValue := multiplyIOUByRatio(theirPaysIOU, ratio)
					paidAmount = Amount{
						Value:    formatIOUValue(payValue.Value),
						Currency: theirPays.Currency,
						Issuer:   theirPays.Issuer,
					}
				}
			}
		}

		// Update the matched offer in the ledger
		// Calculate remaining amounts for matched offer
		// Their TakerGets decreases by what we took (gotAmount)
		// Their TakerPays decreases by what we gave them (paidAmount)
		// Use ORIGINAL amounts, not balance-limited effective amounts
		matchRemainingGets := subtractAmount(originalTheirGets, gotAmount)
		matchRemainingPays := subtractAmount(originalTheirPays, paidAmount)

		matchKey := keylet.Keylet{Key: match.key}
		matchKey.Type = 0x6F // Offer type

		if isZeroAmount(matchRemainingGets) || isZeroAmount(matchRemainingPays) {
			// Fully consumed - update offer with zeroed amounts first, then delete
			// Reference: rippled's TOffer::consume() updates TakerGets/TakerPays before offerDelete()
			// This ensures proper PreviousFields in DeletedNode metadata
			matchOfferData, readErr := view.Read(matchKey)
			if readErr == nil {
				consumedOffer, parseErr := parseLedgerOffer(matchOfferData)
				if parseErr == nil {
					consumedOffer.TakerGets = Amount{Value: "0", Currency: match.offer.TakerGets.Currency, Issuer: match.offer.TakerGets.Issuer}
					consumedOffer.TakerPays = Amount{Value: "0", Currency: match.offer.TakerPays.Currency, Issuer: match.offer.TakerPays.Issuer}
					if updatedOfferData, serErr := serializeLedgerOffer(consumedOffer); serErr == nil {
						view.Update(matchKey, updatedOfferData)
					}
				}
			}
			view.Erase(matchKey)

			// Decrement maker's OwnerCount
			// Reference: rippled offerDelete() adjusts owner count
			makerID, err := decodeAccountID(match.offer.Account)
			if err == nil {
				makerAccountKey := keylet.Account(makerID)
				makerAccountData, err := view.Read(makerAccountKey)
				if err == nil {
					makerAccount, err := parseAccountRoot(makerAccountData)
					if err == nil && makerAccount.OwnerCount > 0 {
						makerAccount.OwnerCount--
						updatedMakerData, err := serializeAccountRoot(makerAccount)
						if err == nil {
							view.Update(makerAccountKey, updatedMakerData)
							// Account modification tracked automatically by ApplyStateTable
						}
					}
				}
			}

			// Remove offer from book directory and delete if empty
			// Reference: rippled View.cpp offerDelete() calls dirRemove()
			bookDirKey := keylet.Keylet{Key: match.offer.BookDirectory}
			bookDirKey.Type = 0x64 // DirectoryNode type
			bookDirData, err := view.Read(bookDirKey)
			if err == nil {
				bookDir, err := parseDirectoryNode(bookDirData)
				if err == nil {
					// Remove the offer from the directory's Indexes
					newIndexes := make([][32]byte, 0, len(bookDir.Indexes))
					for _, idx := range bookDir.Indexes {
						if idx != match.key {
							newIndexes = append(newIndexes, idx)
						}
					}
					bookDir.Indexes = newIndexes

					if len(bookDir.Indexes) == 0 {
						// Directory is now empty - delete it (tracked by ApplyStateTable)
						view.Erase(bookDirKey)
					} else {
						// Directory still has entries - update it
						updatedBookDirData, err := serializeDirectoryNode(bookDir, true) // true = book directory
						if err == nil {
							view.Update(bookDirKey, updatedBookDirData)
						}
					}
				}
			}

			// Remove offer from owner directory
			// Reference: rippled View.cpp offerDelete() removes from owner dir via dirRemove()
			if makerID != [20]byte{} {
				ownerDirKey := keylet.OwnerDir(makerID)
				ownerDirData, err := view.Read(ownerDirKey)
				if err == nil {
					ownerDir, err := parseDirectoryNode(ownerDirData)
					if err == nil {
						// Remove the offer from the owner directory's Indexes
						newOwnerIndexes := make([][32]byte, 0, len(ownerDir.Indexes))
						for _, idx := range ownerDir.Indexes {
							if idx != match.key {
								newOwnerIndexes = append(newOwnerIndexes, idx)
							}
						}
						ownerDir.Indexes = newOwnerIndexes

						// Update owner directory (don't delete even if empty - owner dirs persist)
						// Modification tracked automatically by ApplyStateTable
						updatedOwnerDirData, err := serializeDirectoryNode(ownerDir, false) // false = owner directory
						if err == nil {
							view.Update(ownerDirKey, updatedOwnerDirData)
						}
					}
				}
			}

			// Offer deletion tracked automatically by ApplyStateTable
		} else {
			// Partially consumed - update offer
			match.offer.TakerGets = matchRemainingGets
			match.offer.TakerPays = matchRemainingPays
			match.offer.PreviousTxnID = e.currentTxHash
			match.offer.PreviousTxnLgrSeq = e.config.LedgerSequence

			updatedData, err := serializeLedgerOffer(match.offer)
			if err == nil {
				view.Update(matchKey, updatedData)
				// Offer modification tracked automatically by ApplyStateTable
			}
		}

		// Transfer funds for this match
		// If trade fails (insufficient funds), skip this match and continue
		if err := e.executeOfferTrade(account, match.offer, gotAmount, paidAmount, view); err != nil {
			continue // Skip this match if trade can't be executed
		}

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
// Reference: rippled's offer crossing via flowCross
func (e *Engine) executeOfferTrade(taker *AccountRoot, maker *LedgerOffer, takerGot, takerPaid Amount, view LedgerView) error {
	// Get maker account
	makerAccountID, err := decodeAccountID(maker.Account)
	if err != nil {
		return err
	}
	makerKey := keylet.Account(makerAccountID)
	makerData, err := view.Read(makerKey)
	if err != nil {
		return err
	}
	makerAccount, err := parseAccountRoot(makerData)
	if err != nil {
		return err
	}

	// Transfer takerGot from maker to taker
	if takerGot.IsNative() {
		drops, _ := parseDropsString(takerGot.Value)

		// Verify maker has sufficient balance (including reserve)
		// Reference: rippled accountFunds checks available balance
		makerReserve := e.AccountReserve(makerAccount.OwnerCount)
		if makerAccount.Balance < drops+makerReserve {
			// Maker doesn't have sufficient funds
			return fmt.Errorf("maker has insufficient balance")
		}

		makerAccount.Balance -= drops
		taker.Balance += drops
	} else {
		// IOU transfer - update trust lines
		if err := e.transferIOU(maker.Account, taker.Account, takerGot, view); err != nil {
			return err
		}
	}

	// Transfer takerPaid from taker to maker
	if takerPaid.IsNative() {
		drops, _ := parseDropsString(takerPaid.Value)

		// Verify taker has sufficient balance (including reserve)
		takerReserve := e.AccountReserve(taker.OwnerCount)
		if taker.Balance < drops+takerReserve {
			// Taker doesn't have sufficient funds
			return fmt.Errorf("taker has insufficient balance")
		}

		taker.Balance -= drops
		makerAccount.Balance += drops
	} else {
		// IOU transfer - update trust lines
		// Apply transfer fee: taker sends MORE than maker receives
		// Reference: rippled applies transfer rate during offer crossing
		transferRate := e.getTransferRate(takerPaid.Issuer)
		takerPaidIOU := NewIOUAmount(takerPaid.Value, takerPaid.Currency, takerPaid.Issuer)

		// Calculate what taker must send to deliver takerPaid to maker
		takerSendsIOU := applyTransferFee(takerPaidIOU, transferRate)
		takerSends := Amount{
			Value:    formatIOUValue(takerSendsIOU.Value),
			Currency: takerPaid.Currency,
			Issuer:   takerPaid.Issuer,
		}

		// Transfer with fee: taker sends takerSends, maker receives takerPaid
		if err := e.transferIOUWithFee(taker.Account, maker.Account, takerSends, takerPaid, view); err != nil {
			return err
		}
	}

	// Update PreviousTxnID and LgrSeq for the maker account
	makerAccount.PreviousTxnID = e.currentTxHash
	makerAccount.PreviousTxnLgrSeq = e.config.LedgerSequence

	// Update maker account - modification tracked automatically by ApplyStateTable
	updatedMakerData, err := serializeAccountRoot(makerAccount)
	if err != nil {
		return err
	}
	view.Update(makerKey, updatedMakerData)

	return nil
}

// transferIOU transfers an IOU amount between accounts via trust lines
// Reference: rippled's flow engine for IOU transfers
func (e *Engine) transferIOU(fromAccount, toAccount string, amount Amount, view LedgerView) error {
	fromID, err := decodeAccountID(fromAccount)
	if err != nil {
		return err
	}
	toID, err := decodeAccountID(toAccount)
	if err != nil {
		return err
	}
	issuerID, err := decodeAccountID(amount.Issuer)
	if err != nil {
		return err
	}

	iouAmount := NewIOUAmount(amount.Value, amount.Currency, amount.Issuer)

	// Update from's trust line (decrease balance)
	fromIsIssuer := fromAccount == amount.Issuer
	toIsIssuer := toAccount == amount.Issuer

	if fromIsIssuer {
		// Issuer is sending - increase to's trust line balance
		trustLineKey := keylet.Line(toID, issuerID, amount.Currency)
		if err := e.updateTrustLineBalance(trustLineKey, toID, issuerID, iouAmount, true, view); err != nil {
			return err
		}
	} else if toIsIssuer {
		// Sending to issuer - decrease from's trust line balance
		trustLineKey := keylet.Line(fromID, issuerID, amount.Currency)
		if err := e.updateTrustLineBalance(trustLineKey, fromID, issuerID, iouAmount, false, view); err != nil {
			return err
		}
	} else {
		// Transfer between non-issuers
		// Decrease from's balance with issuer
		fromTrustKey := keylet.Line(fromID, issuerID, amount.Currency)
		if err := e.updateTrustLineBalance(fromTrustKey, fromID, issuerID, iouAmount, false, view); err != nil {
			return err
		}

		// Increase to's balance with issuer
		toTrustKey := keylet.Line(toID, issuerID, amount.Currency)
		if err := e.updateTrustLineBalance(toTrustKey, toID, issuerID, iouAmount, true, view); err != nil {
			return err
		}
	}

	return nil
}

// transferIOUWithFee transfers IOU with transfer fee applied
// senderAmount is what the sender pays (includes fee)
// receiverAmount is what the receiver gets (after fee)
// Reference: rippled applies transfer rate during IOU transfers
func (e *Engine) transferIOUWithFee(fromAccount, toAccount string, senderAmount, receiverAmount Amount, view LedgerView) error {
	fromID, err := decodeAccountID(fromAccount)
	if err != nil {
		return err
	}
	toID, err := decodeAccountID(toAccount)
	if err != nil {
		return err
	}
	issuerID, err := decodeAccountID(senderAmount.Issuer)
	if err != nil {
		return err
	}

	senderIOU := NewIOUAmount(senderAmount.Value, senderAmount.Currency, senderAmount.Issuer)
	receiverIOU := NewIOUAmount(receiverAmount.Value, receiverAmount.Currency, receiverAmount.Issuer)

	fromIsIssuer := fromAccount == senderAmount.Issuer
	toIsIssuer := toAccount == senderAmount.Issuer

	if fromIsIssuer {
		// Issuer is sending - no transfer fee, increase to's trust line
		trustLineKey := keylet.Line(toID, issuerID, senderAmount.Currency)
		if err := e.updateTrustLineBalance(trustLineKey, toID, issuerID, receiverIOU, true, view); err != nil {
			return err
		}
	} else if toIsIssuer {
		// Sending to issuer - no transfer fee, decrease from's trust line
		trustLineKey := keylet.Line(fromID, issuerID, senderAmount.Currency)
		if err := e.updateTrustLineBalance(trustLineKey, fromID, issuerID, senderIOU, false, view); err != nil {
			return err
		}
	} else {
		// Transfer between non-issuers - apply transfer fee
		// Sender pays senderAmount (includes fee)
		fromTrustKey := keylet.Line(fromID, issuerID, senderAmount.Currency)
		if err := e.updateTrustLineBalance(fromTrustKey, fromID, issuerID, senderIOU, false, view); err != nil {
			return err
		}

		// Receiver gets receiverAmount (after fee)
		toTrustKey := keylet.Line(toID, issuerID, senderAmount.Currency)
		if err := e.updateTrustLineBalance(toTrustKey, toID, issuerID, receiverIOU, true, view); err != nil {
			return err
		}
	}

	return nil
}

// updateTrustLineBalance updates a trust line balance
// RippleState balance semantics:
// - Negative balance = LOW owes HIGH (HIGH holds tokens)
// - Positive balance = HIGH owes LOW (LOW holds tokens)
func (e *Engine) updateTrustLineBalance(key keylet.Keylet, accountID, issuerID [20]byte, amount IOUAmount, increase bool, view LedgerView) error {
	trustLineData, err := view.Read(key)
	if err != nil {
		return fmt.Errorf("trust line not found: %w", err)
	}

	rs, err := parseRippleState(trustLineData)
	if err != nil {
		return fmt.Errorf("failed to parse trust line: %w", err)
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

	// Update the RippleState
	rs.Balance = newBalance
	rs.PreviousTxnID = e.currentTxHash
	rs.PreviousTxnLgrSeq = e.config.LedgerSequence

	updatedData, err := serializeRippleState(rs)
	if err != nil {
		return fmt.Errorf("failed to serialize trust line: %w", err)
	}

	// RippleState modification tracked automatically by ApplyStateTable
	if err := view.Update(key, updatedData); err != nil {
		return fmt.Errorf("failed to update trust line: %w", err)
	}

	return nil
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
func (e *Engine) applyOfferCancel(cancel *OfferCancel, account *AccountRoot, view LedgerView) Result {
	// Find the offer
	accountID, _ := decodeAccountID(account.Account)
	offerKey := keylet.Offer(accountID, cancel.OfferSequence)

	exists, err := view.Exists(offerKey)
	if err != nil {
		return TefINTERNAL
	}

	if !exists {
		// Offer doesn't exist - this is OK (maybe already filled/cancelled)
		return TesSUCCESS
	}

	// Read the offer to get its details for metadata and directory removal
	offerData, err := view.Read(offerKey)
	if err != nil {
		return TefINTERNAL
	}
	ledgerOffer, err := parseLedgerOffer(offerData)
	if err != nil {
		return TefINTERNAL
	}

	// Create SLE for the offer for metadata tracking
	sleOffer := NewSLEOffer(offerKey.Key)
	sleOffer.LoadFromLedgerOffer(ledgerOffer)
	sleOffer.MarkAsDeleted()

	// Remove from owner directory (keepRoot = false since owner dir should persist)
	ownerDirKey := keylet.OwnerDir(accountID)
	ownerDirResult, err := e.dirRemove(view, ownerDirKey, ledgerOffer.OwnerNode, offerKey.Key, false)
	if err != nil {
		return TefINTERNAL
	}
	if !ownerDirResult.Success {
		return TefBAD_LEDGER
	}

	// Remove from book directory (keepRoot = false - delete directory if empty)
	bookDirKey := keylet.Keylet{Type: 100, Key: ledgerOffer.BookDirectory} // DirectoryNode type
	bookDirResult, err := e.dirRemove(view, bookDirKey, ledgerOffer.BookNode, offerKey.Key, false)
	if err != nil {
		return TefINTERNAL
	}
	if !bookDirResult.Success {
		return TefBAD_LEDGER
	}

	// Delete the offer from ledger
	if err := view.Erase(offerKey); err != nil {
		return TefINTERNAL
	}

	// Decrement owner count
	if account.OwnerCount > 0 {
		account.OwnerCount--
	}

	// All metadata generation tracked automatically by ApplyStateTable

	return TesSUCCESS
}
