package payment

import (
	"errors"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
	"strconv"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	tx "github.com/LeJamon/goXRPLd/internal/core/tx"
)

func init() {
	tx.Register(tx.TypePayment, func() tx.Transaction {
		return &Payment{BaseTx: *tx.NewBaseTx(tx.TypePayment, "")}
	})
}

// Payment transaction moves value from one account to another.
// It is the most fundamental transaction type in the XRPL.
type Payment struct {
	tx.BaseTx

	// Amount is the amount of currency to deliver (required)
	Amount tx.Amount `json:"Amount" xrpl:"Amount,amount"`

	// Destination is the account receiving the payment (required)
	Destination string `json:"Destination" xrpl:"Destination"`

	// DestinationTag is an arbitrary tag for the destination (optional)
	DestinationTag *uint32 `json:"DestinationTag,omitempty" xrpl:"DestinationTag,omitempty"`

	// InvoiceID is a 256-bit hash for identifying this payment (optional)
	InvoiceID string `json:"InvoiceID,omitempty" xrpl:"InvoiceID,omitempty"`

	// Paths for cross-currency payments (optional)
	Paths [][]PathStep `json:"Paths,omitempty" xrpl:"Paths,omitempty"`

	// SendMax is the maximum amount to send (optional, for cross-currency)
	SendMax *tx.Amount `json:"SendMax,omitempty" xrpl:"SendMax,omitempty,amount"`

	// DeliverMin is the minimum amount to deliver (optional, for partial payments)
	DeliverMin *tx.Amount `json:"DeliverMin,omitempty" xrpl:"DeliverMin,omitempty,amount"`
}

// PathStep represents a single step in a payment path
type PathStep struct {
	Account  string `json:"account,omitempty"`
	Currency string `json:"currency,omitempty"`
	Issuer   string `json:"issuer,omitempty"`
	Type     int    `json:"type,omitempty"`
	TypeHex  string `json:"type_hex,omitempty"`
}

// Payment flags
const (
	// tfNoDirectRipple prevents direct rippling (tfNoRippleDirect in rippled)
	PaymentFlagNoDirectRipple uint32 = 0x00010000
	// tfPartialPayment allows partial payments
	PaymentFlagPartialPayment uint32 = 0x00020000
	// tfLimitQuality limits quality of paths
	PaymentFlagLimitQuality uint32 = 0x00040000
)

// Path constraints matching rippled
const (
	// MaxPathSize is the maximum number of paths in a payment (rippled: MaxPathSize = 7)
	MaxPathSize = 7
	// MaxPathLength is the maximum number of steps per path (rippled: MaxPathLength = 8)
	MaxPathLength = 8
)

// NewPayment creates a new Payment transaction
func NewPayment(account, destination string, amount tx.Amount) *Payment {
	return &Payment{
		BaseTx:      *tx.NewBaseTx(tx.TypePayment, account),
		Amount:      amount,
		Destination: destination,
	}
}

// TxType returns the transaction type
func (p *Payment) TxType() tx.Type {
	return tx.TypePayment
}

// Validate validates the payment transaction
func (p *Payment) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	if p.Destination == "" {
		return errors.New("Destination is required")
	}

	if p.Amount.IsZero() {
		return errors.New("Amount is required")
	}

	// Determine if this is an XRP-to-XRP (direct) payment
	xrpDirect := p.Amount.IsNative() && (p.SendMax == nil || p.SendMax.IsNative())

	// Check flags based on payment type
	flags := p.GetFlags()
	partialPaymentAllowed := (flags & PaymentFlagPartialPayment) != 0

	// tfPartialPayment flag is invalid for XRP-to-XRP payments (temBAD_SEND_XRP_PARTIAL)
	// Reference: rippled Payment.cpp:182-188
	if xrpDirect && partialPaymentAllowed {
		return errors.New("temBAD_SEND_XRP_PARTIAL: Partial payment specified for XRP to XRP")
	}

	// DeliverMin can only be used with tfPartialPayment flag (temBAD_AMOUNT)
	// Reference: rippled Payment.cpp:206-214
	if p.DeliverMin != nil && !partialPaymentAllowed {
		return errors.New("temBAD_AMOUNT: DeliverMin requires tfPartialPayment flag")
	}

	// Validate DeliverMin if present
	// Reference: rippled Payment.cpp:216-238
	if p.DeliverMin != nil {
		// DeliverMin must be positive (not zero, not negative)
		if p.DeliverMin.IsZero() || p.DeliverMin.IsNegative() {
			return errors.New("temBAD_AMOUNT: DeliverMin must be positive")
		}

		// DeliverMin currency must match Amount currency
		if p.DeliverMin.Currency != p.Amount.Currency || p.DeliverMin.Issuer != p.Amount.Issuer {
			return errors.New("temBAD_AMOUNT: DeliverMin currency must match Amount")
		}
	}

	// Paths array max length is 7 (temMALFORMED if exceeded)
	// Reference: rippled Payment.cpp:353-359 (MaxPathSize)
	if len(p.Paths) > MaxPathSize {
		return errors.New("temMALFORMED: Paths array exceeds maximum size of 7")
	}

	// Each path can have max 8 steps (temMALFORMED if exceeded)
	// Reference: rippled Payment.cpp:354-358 (MaxPathLength)
	for i, path := range p.Paths {
		if len(path) > MaxPathLength {
			return errors.New("temMALFORMED: Path " + string(rune('0'+i)) + " exceeds maximum length of 8 steps")
		}
	}

	// Cannot send XRP to self without paths (temREDUNDANT)
	// Reference: rippled Payment.cpp:159-167
	if p.Account == p.Destination && p.Amount.IsNative() && len(p.Paths) == 0 {
		return errors.New("temREDUNDANT: cannot send XRP to self without path")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (p *Payment) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(p)
}

// SetPartialPayment enables partial payment flag
func (p *Payment) SetPartialPayment() {
	flags := p.GetFlags() | PaymentFlagPartialPayment
	p.SetFlags(flags)
}

// SetNoDirectRipple enables no direct ripple flag
func (p *Payment) SetNoDirectRipple() {
	flags := p.GetFlags() | PaymentFlagNoDirectRipple
	p.SetFlags(flags)
}

// Apply applies the Payment transaction to the ledger state.
func (p *Payment) Apply(ctx *tx.ApplyContext) tx.Result {
	// XRP-to-XRP payment (direct payment)
	if p.Amount.IsNative() {
		return p.applyXRPPayment(ctx)
	}

	// IOU payment - more complex, involves trust lines and paths
	return p.applyIOUPayment(ctx)
}

// applyXRPPayment applies an XRP-to-XRP payment
// Reference: rippled/src/xrpld/app/tx/detail/Payment.cpp doApply() for XRP direct payments
func (p *Payment) applyXRPPayment(ctx *tx.ApplyContext) tx.Result {
	// Get the amount in drops
	drops := p.Amount.Drops()
	if drops <= 0 {
		return tx.TemBAD_AMOUNT
	}
	amountDrops := uint64(drops)

	// Parse the fee from the transaction
	feeDrops, err := strconv.ParseUint(p.Fee, 10, 64)
	if err != nil {
		feeDrops = ctx.Config.BaseFee // fallback to base fee if not specified
	}

	// IMPORTANT: sender.Balance has already had fee deducted (in doApply).
	// Rippled checks against mPriorBalance (balance BEFORE fee deduction).
	// We reconstruct the pre-fee balance for the check.
	// Reference: rippled Payment.cpp:619 - if (mPriorBalance < dstAmount.xrp() + mmm)
	priorBalance := ctx.Account.Balance + feeDrops

	// Calculate reserve as: ReserveBase + (ownerCount * ReserveIncrement)
	// This matches rippled's accountReserve(ownerCount) calculation
	reserve := ctx.Config.ReserveBase + (uint64(ctx.Account.OwnerCount) * ctx.Config.ReserveIncrement)

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
		return tx.TecUNFUNDED_PAYMENT
	}

	// Get destination account
	destAccountID, err := sle.DecodeAccountID(p.Destination)
	if err != nil {
		return tx.TemDST_NEEDED
	}
	destKey := keylet.Account(destAccountID)

	destExists, err := ctx.View.Exists(destKey)
	if err != nil {
		return tx.TefINTERNAL
	}

	if destExists {
		// Destination exists - just credit the amount
		destData, err := ctx.View.Read(destKey)
		if err != nil {
			return tx.TefINTERNAL
		}

		destAccount, err := sle.ParseAccountRoot(destData)
		if err != nil {
			return tx.TefINTERNAL
		}

		// Check for pseudo-account (AMM accounts cannot receive direct payments)
		// See rippled Payment.cpp:636-637: if (isPseudoAccount(sleDst)) return tecNO_PERMISSION
		if (destAccount.Flags & sle.LsfAMM) != 0 {
			return tx.TecNO_PERMISSION
		}

		// Check destination's lsfDisallowXRP flag
		// Per rippled, if lsfDisallowXRP is set and sender != destination, return tecNO_TARGET
		// This allows accounts to indicate they don't want to receive XRP
		// Reference: this matches rippled behavior for direct XRP payments
		if (destAccount.Flags & sle.LsfDisallowXRP) != 0 {
			senderAccountID, err := sle.DecodeAccountID(ctx.Account.Account)
			if err != nil {
				return tx.TefINTERNAL
			}
			// Only reject if sender is not the destination (self-payments are allowed)
			if senderAccountID != destAccountID {
				return tx.TecNO_TARGET
			}
		}

		// Check if destination requires a tag
		if (destAccount.Flags&sle.LsfRequireDestTag) != 0 && p.DestinationTag == nil {
			return tx.TecDST_TAG_NEEDED
		}

		// Check deposit authorization
		// Reference: rippled Payment.cpp:641-677
		// If destination has lsfDepositAuth flag set, payments require preauthorization
		// EXCEPT: to prevent account "wedging", allow small payments if BOTH conditions are true:
		//   1. Destination balance <= base reserve (account is at or below minimum)
		//   2. Payment amount <= base reserve
		if (destAccount.Flags & sle.LsfDepositAuth) != 0 {
			dstReserve := ctx.Config.ReserveBase

			// Check if the exception applies (prevents account wedging)
			if amountDrops > dstReserve || destAccount.Balance > dstReserve {
				// Must check for preauthorization
				senderAccountID, err := sle.DecodeAccountID(ctx.Account.Account)
				if err != nil {
					return tx.TefINTERNAL
				}

				// Look up the DepositPreauth ledger entry
				depositPreauthKey := keylet.DepositPreauth(destAccountID, senderAccountID)
				preauthExists, err := ctx.View.Exists(depositPreauthKey)
				if err != nil {
					return tx.TefINTERNAL
				}

				if !preauthExists {
					// Sender is not preauthorized to deposit to this account
					return tx.TecNO_PERMISSION
				}
			}
			// If both conditions are true (small payment to low-balance account),
			// payment is allowed without preauthorization
		}

		// Credit destination
		destAccount.Balance += amountDrops

		// Clear PasswordSpent flag if set (lsfPasswordSpent = 0x00010000)
		// Per rippled Payment.cpp:686-687, receiving XRP clears this flag
		if (destAccount.Flags & sle.LsfPasswordSpent) != 0 {
			destAccount.Flags &^= sle.LsfPasswordSpent
		}

		// Update PreviousTxnID and PreviousTxnLgrSeq on destination (thread the account)
		destAccount.PreviousTxnID = ctx.TxHash
		destAccount.PreviousTxnLgrSeq = ctx.Config.LedgerSequence

		// Debit sender
		ctx.Account.Balance -= amountDrops

		// Update destination
		updatedDestData, err := sle.SerializeAccountRoot(destAccount)
		if err != nil {
			return tx.TefINTERNAL
		}

		// Update tracked automatically by ApplyStateTable
		if err := ctx.View.Update(destKey, updatedDestData); err != nil {
			return tx.TefINTERNAL
		}

		return tx.TesSUCCESS
	}

	// Destination doesn't exist - need to create it
	// Check minimum amount for account creation
	if amountDrops < ctx.Config.ReserveBase {
		return tx.TecNO_DST_INSUF_XRP
	}

	// Create new account
	// With featureDeletableAccounts enabled, new accounts start with sequence
	// equal to the current ledger sequence. Otherwise, sequence starts at 1.
	// (see rippled Payment.cpp:409-411)
	var accountSequence uint32
	if ctx.Rules().DeletableAccountsEnabled() {
		accountSequence = ctx.Config.LedgerSequence
	} else {
		accountSequence = 1
	}
	newAccount := &sle.AccountRoot{
		Account:           p.Destination,
		Balance:           amountDrops,
		Sequence:          accountSequence,
		Flags:             0,
		PreviousTxnID:     ctx.TxHash,
		PreviousTxnLgrSeq: ctx.Config.LedgerSequence,
	}

	// Debit sender
	ctx.Account.Balance -= amountDrops

	// Serialize and insert new account
	newAccountData, err := sle.SerializeAccountRoot(newAccount)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Insert tracked automatically by ApplyStateTable
	if err := ctx.View.Insert(destKey, newAccountData); err != nil {
		return tx.TefINTERNAL
	}

	return tx.TesSUCCESS
}

// applyIOUPayment applies an IOU (issued currency) payment
// Reference: rippled/src/xrpld/app/tx/detail/Payment.cpp
func (p *Payment) applyIOUPayment(ctx *tx.ApplyContext) tx.Result {
	// Validate the amount
	if p.Amount.IsZero() {
		return tx.TemBAD_AMOUNT
	}
	if p.Amount.IsNegative() {
		return tx.TemBAD_AMOUNT
	}

	// Get account IDs
	senderAccountID, err := sle.DecodeAccountID(ctx.Account.Account)
	if err != nil {
		return tx.TefINTERNAL
	}

	destAccountID, err := sle.DecodeAccountID(p.Destination)
	if err != nil {
		return tx.TemDST_NEEDED
	}

	issuerAccountID, err := sle.DecodeAccountID(p.Amount.Issuer)
	if err != nil {
		return tx.TemBAD_ISSUER
	}

	// Convert the tx.Amount to sle.IOUAmount for internal use
	amount := p.Amount.ToIOUAmountLegacy()

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
	if p.Paths != nil && len(p.Paths) > 0 {
		requiresPathFinding = true
	}

	// Check for SendMax with cross-issuer
	// When SendMax.issuer == sender, it means "use my trust line balance" - rippled
	// determines the actual issuer from the sender's trust lines.
	// When SendMax.issuer is explicitly a different third party (not sender, not Amount.issuer),
	// that's a true cross-issuer payment requiring path finding.
	if p.SendMax != nil && !senderIsIssuer {
		sendMaxIssuer := p.SendMax.Issuer
		// True cross-issuer: SendMax.issuer is a specific third-party issuer
		// (not the sender, not the Amount.issuer)
		if sendMaxIssuer != "" &&
			sendMaxIssuer != p.Amount.Issuer &&
			sendMaxIssuer != p.Common.Account {
			requiresPathFinding = true
		}
	}

	// For path-finding payments, use the Flow Engine (RippleCalculate)
	if requiresPathFinding {
		return p.applyIOUPaymentWithPaths(ctx, senderAccountID, destAccountID, issuerAccountID)
	}

	// Check destination exists
	destKey := keylet.Account(destAccountID)
	destExists, err := ctx.View.Exists(destKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	if !destExists {
		return tx.TecNO_DST
	}

	// Get destination account to check flags
	destData, err := ctx.View.Read(destKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	destAccount, err := sle.ParseAccountRoot(destData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Check destination tag requirement
	if (destAccount.Flags&sle.LsfRequireDestTag) != 0 && p.DestinationTag == nil {
		return tx.TecDST_TAG_NEEDED
	}

	// Handle three cases:
	// 1. Sender is issuer - creating new tokens
	// 2. Destination is issuer - redeeming tokens
	// 3. Neither - transfer between accounts via trust lines

	var result tx.Result
	var deliveredAmount sle.IOUAmount

	if senderIsIssuer {
		// Sender is issuing their own currency to destination
		// Need trust line from destination to sender (issuer)
		result, deliveredAmount = p.applyIOUIssueWithDelivered(ctx, destAccount, senderAccountID, destAccountID, amount)
	} else if destIsIssuer {
		// Destination is the issuer - sender is redeeming tokens
		// Need trust line from sender to destination (issuer)
		result, deliveredAmount = p.applyIOURedeemWithDelivered(ctx, destAccount, senderAccountID, destAccountID, amount)
	} else {
		// Neither is issuer - transfer between two non-issuer accounts
		// This requires trust lines from both parties to the issuer
		result, deliveredAmount = p.applyIOUTransferWithDelivered(ctx, destAccount, senderAccountID, destAccountID, issuerAccountID, amount)
	}

	// DeliverMin enforcement for partial payments
	// Reference: rippled Payment.cpp:496-500
	// If tfPartialPayment is set and DeliverMin is specified, check that delivered >= DeliverMin
	if result == tx.TesSUCCESS && p.DeliverMin != nil {
		flags := p.GetFlags()
		if (flags & PaymentFlagPartialPayment) != 0 {
			deliverMin := p.DeliverMin.ToIOUAmountLegacy()
			if deliveredAmount.Compare(deliverMin) < 0 {
				return tx.TecPATH_PARTIAL
			}
		}
	}

	return result
}

// applyIOUPaymentWithPaths handles IOU payments that require path finding using the Flow Engine.
// This is the main entry point for cross-currency payments and payments with explicit paths.
// Reference: rippled/src/xrpld/app/paths/RippleCalc.cpp
func (p *Payment) applyIOUPaymentWithPaths(ctx *tx.ApplyContext, senderID, destID, issuerID [20]byte) tx.Result {
	// Determine payment flags
	flags := p.GetFlags()
	partialPayment := (flags & PaymentFlagPartialPayment) != 0
	limitQuality := (flags & PaymentFlagLimitQuality) != 0
	noDirectRipple := (flags & PaymentFlagNoDirectRipple) != 0

	// addDefaultPath is true unless tfNoRippleDirect is set
	addDefaultPath := !noDirectRipple

	// Execute RippleCalculate
	_, actualOut, _, sandbox, result := RippleCalculate(
		ctx.View,
		senderID,
		destID,
		p.Amount,
		p.SendMax,
		p.Paths,
		addDefaultPath,
		partialPayment,
		limitQuality,
		ctx.TxHash,
		ctx.Config.LedgerSequence,
	)

	// Handle result
	if result != tx.TesSUCCESS && result != tx.TecPATH_PARTIAL {
		return result
	}

	// Apply sandbox changes back to the ledger view (through ApplyStateTable for tracking)
	if sandbox != nil {
		if err := sandbox.ApplyToView(ctx.View); err != nil {
			return tx.TefINTERNAL
		}
	}

	// Check if partial payment delivered enough (DeliverMin)
	if partialPayment && p.DeliverMin != nil {
		deliverMin := ToEitherAmount(*p.DeliverMin)
		if actualOut.Compare(deliverMin) < 0 {
			return tx.TecPATH_PARTIAL
		}
	}

	// Record delivered amount in metadata
	deliveredAmt := FromEitherAmount(actualOut)
	ctx.Metadata.DeliveredAmount = &deliveredAmt

	// Offer deletions and trust line modifications tracked automatically by ApplyStateTable

	return result
}

// applyIOUIssue handles when sender is the issuer creating new tokens
func (p *Payment) applyIOUIssue(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID [20]byte, amount sle.IOUAmount) tx.Result {
	// Look up the trust line between destination and issuer (sender)
	trustLineKey := keylet.Line(destID, senderID, amount.Currency)

	trustLineExists, err := ctx.View.Exists(trustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}

	if !trustLineExists {
		// No trust line exists - destination has not authorized holding this currency
		return tx.TecPATH_DRY
	}

	// Read and parse the trust line
	trustLineData, err := ctx.View.Read(trustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}

	rippleState, err := sle.ParseRippleState(trustLineData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Determine which side is low/high account
	destIsLow := sle.CompareAccountIDsForLine(destID, senderID) < 0

	// Get the trust limit set by the destination (recipient)
	var trustLimit sle.IOUAmount
	if destIsLow {
		trustLimit = rippleState.LowLimit
	} else {
		trustLimit = rippleState.HighLimit
	}

	// Calculate new balance after adding the amount
	// RippleState balance semantics:
	// - Negative balance = LOW owes HIGH (HIGH holds tokens)
	// - Positive balance = HIGH owes LOW (LOW holds tokens)
	var newBalance sle.IOUAmount
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
		return tx.TecPATH_PARTIAL
	}

	// Ensure the new balance has the correct currency and issuer
	// (the parsed balance may have null bytes for currency if it was zero)
	newBalance.Currency = amount.Currency
	newBalance.Issuer = amount.Issuer

	// Update the trust line
	rippleState.Balance = newBalance

	// Update PreviousTxnID and PreviousTxnLgrSeq to this transaction
	rippleState.PreviousTxnID = ctx.TxHash
	rippleState.PreviousTxnLgrSeq = ctx.Config.LedgerSequence

	// Serialize and update
	updatedTrustLine, err := sle.SerializeRippleState(rippleState)
	if err != nil {
		return tx.TefINTERNAL
	}

	if err := ctx.View.Update(trustLineKey, updatedTrustLine); err != nil {
		return tx.TefINTERNAL
	}

	// RippleState modification tracked automatically by ApplyStateTable

	delivered := p.Amount
	ctx.Metadata.DeliveredAmount = &delivered

	return tx.TesSUCCESS
}

// applyIOURedeem handles when destination is the issuer (redeeming tokens)
func (p *Payment) applyIOURedeem(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID [20]byte, amount sle.IOUAmount) tx.Result {
	// Look up the trust line between sender and issuer (destination)
	trustLineKey := keylet.Line(senderID, destID, amount.Currency)

	trustLineExists, err := ctx.View.Exists(trustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}

	if !trustLineExists {
		// No trust line exists - sender doesn't hold this currency
		return tx.TecPATH_DRY
	}

	// Read and parse the trust line
	trustLineData, err := ctx.View.Read(trustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}

	rippleState, err := sle.ParseRippleState(trustLineData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Determine which side is low/high account
	senderIsLow := sle.CompareAccountIDsForLine(senderID, destID) < 0

	// Get sender's current balance (how much issuer owes them)
	// RippleState balance semantics:
	// - Negative balance = LOW owes HIGH (HIGH holds tokens)
	// - Positive balance = HIGH owes LOW (LOW holds tokens)
	var senderBalance sle.IOUAmount
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
		return tx.TecPATH_PARTIAL
	}

	// Update balance by reducing sender's holding
	// When redeeming, the issuer owes less to the sender
	var newBalance sle.IOUAmount
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
	rippleState.PreviousTxnID = ctx.TxHash
	rippleState.PreviousTxnLgrSeq = ctx.Config.LedgerSequence

	// Serialize and update
	updatedTrustLine, err := sle.SerializeRippleState(rippleState)
	if err != nil {
		return tx.TefINTERNAL
	}

	if err := ctx.View.Update(trustLineKey, updatedTrustLine); err != nil {
		return tx.TefINTERNAL
	}

	// RippleState modification tracked automatically by ApplyStateTable

	delivered := p.Amount
	ctx.Metadata.DeliveredAmount = &delivered

	return tx.TesSUCCESS
}

// applyIOUTransfer handles transfer between two non-issuer accounts
func (p *Payment) applyIOUTransfer(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID, issuerID [20]byte, amount sle.IOUAmount) tx.Result {
	// Both sender and destination need trust lines to the issuer
	// This is a simplified implementation - full path finding is more complex

	// Get sender's trust line to issuer
	senderTrustLineKey := keylet.Line(senderID, issuerID, amount.Currency)
	senderTrustExists, err := ctx.View.Exists(senderTrustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	if !senderTrustExists {
		return tx.TecPATH_DRY
	}

	// Get destination's trust line to issuer
	destTrustLineKey := keylet.Line(destID, issuerID, amount.Currency)
	destTrustExists, err := ctx.View.Exists(destTrustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	if !destTrustExists {
		return tx.TecPATH_DRY
	}

	// Read sender's trust line
	senderTrustData, err := ctx.View.Read(senderTrustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	senderRippleState, err := sle.ParseRippleState(senderTrustData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Read destination's trust line
	destTrustData, err := ctx.View.Read(destTrustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	destRippleState, err := sle.ParseRippleState(destTrustData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Calculate sender's balance with issuer
	// RippleState balance semantics:
	// - Negative balance = LOW owes HIGH (HIGH holds tokens)
	// - Positive balance = HIGH owes LOW (LOW holds tokens)
	senderIsLowWithIssuer := sle.CompareAccountIDsForLine(senderID, issuerID) < 0
	var senderBalance sle.IOUAmount
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
		return tx.TecPATH_PARTIAL
	}

	// Calculate destination's current balance and trust limit
	destIsLowWithIssuer := sle.CompareAccountIDsForLine(destID, issuerID) < 0
	var destBalance, destLimit sle.IOUAmount
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
		return tx.TecPATH_PARTIAL
	}

	// Update sender's trust line (decrease balance - sender loses tokens)
	var newSenderRippleBalance sle.IOUAmount
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
	var newDestRippleBalance sle.IOUAmount
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
	updatedSenderTrust, err := sle.SerializeRippleState(senderRippleState)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Update(senderTrustLineKey, updatedSenderTrust); err != nil {
		return tx.TefINTERNAL
	}

	// Serialize and update destination's trust line
	updatedDestTrust, err := sle.SerializeRippleState(destRippleState)
	if err != nil {
		return tx.TefINTERNAL
	}
	if err := ctx.View.Update(destTrustLineKey, updatedDestTrust); err != nil {
		return tx.TefINTERNAL
	}

	// RippleState modifications tracked automatically by ApplyStateTable

	delivered := p.Amount
	ctx.Metadata.DeliveredAmount = &delivered

	return tx.TesSUCCESS
}

// applyIOUIssueWithDelivered wraps applyIOUIssue to return the delivered amount
func (p *Payment) applyIOUIssueWithDelivered(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID [20]byte, amount sle.IOUAmount) (tx.Result, sle.IOUAmount) {
	result := p.applyIOUIssue(ctx, dest, senderID, destID, amount)
	if result == tx.TesSUCCESS {
		// For successful issue, the full amount is delivered
		return result, amount
	}
	return result, sle.IOUAmount{}
}

// applyIOURedeemWithDelivered wraps applyIOURedeem to return the delivered amount
func (p *Payment) applyIOURedeemWithDelivered(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID [20]byte, amount sle.IOUAmount) (tx.Result, sle.IOUAmount) {
	result := p.applyIOURedeem(ctx, dest, senderID, destID, amount)
	if result == tx.TesSUCCESS {
		// For successful redeem, the full amount is delivered
		return result, amount
	}
	return result, sle.IOUAmount{}
}

// applyIOUTransferWithDelivered wraps applyIOUTransfer to return the delivered amount
func (p *Payment) applyIOUTransferWithDelivered(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID, issuerID [20]byte, amount sle.IOUAmount) (tx.Result, sle.IOUAmount) {
	result := p.applyIOUTransfer(ctx, dest, senderID, destID, issuerID, amount)
	if result == tx.TesSUCCESS {
		// For successful transfer, the full amount is delivered
		return result, amount
	}
	return result, sle.IOUAmount{}
}
