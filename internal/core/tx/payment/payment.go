package payment

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	tx "github.com/LeJamon/goXRPLd/internal/core/tx"
)

func init() {
	tx.Register(tx.TypePayment, func() tx.Transaction {
		return &Payment{BaseTx: *tx.NewBaseTx(tx.TypePayment, "")}
	})
}

// checkTrustLineAuthorization checks if a trust line is authorized when the issuer requires auth.
// Reference: rippled DirectStep.cpp:417-430
//
// Parameters:
//   - view: ledger view to read account and trust line data
//   - issuerID: the issuer account ID
//   - holderID: the holder (non-issuer) account ID
//   - trustLine: the parsed RippleState (trust line) object
//
// Returns terNO_AUTH if:
//   - The issuer has lsfRequireAuth flag set, AND
//   - The trust line doesn't have the appropriate auth flag set, AND
//   - The trust line balance is zero (new relationship)
//
// Returns tesSUCCESS if authorized or if auth not required.
func checkTrustLineAuthorization(view tx.LedgerView, issuerID, holderID [20]byte, trustLine *sle.RippleState) tx.Result {
	// Read the issuer's account to check for lsfRequireAuth
	issuerKey := keylet.Account(issuerID)
	issuerData, err := view.Read(issuerKey)
	if err != nil || issuerData == nil {
		return tx.TefINTERNAL
	}

	issuerAccount, err := sle.ParseAccountRoot(issuerData)
	if err != nil {
		return tx.TefINTERNAL
	}

	// If issuer doesn't require auth, always authorized
	if (issuerAccount.Flags & sle.LsfRequireAuth) == 0 {
		return tx.TesSUCCESS
	}

	// Issuer requires auth - check if the trust line is authorized
	// The auth flag depends on account ordering in the trust line
	// Reference: rippled DirectStep.cpp:420
	// auto const authField = (src_ > dst_) ? lsfHighAuth : lsfLowAuth;
	var authFlag uint32
	if sle.CompareAccountIDs(issuerID, holderID) > 0 {
		// Issuer is HIGH, holder is LOW - need lsfHighAuth
		authFlag = sle.LsfHighAuth
	} else {
		// Issuer is LOW, holder is HIGH - need lsfLowAuth
		authFlag = sle.LsfLowAuth
	}

	// Check if trust line has the auth flag
	if (trustLine.Flags & authFlag) != 0 {
		return tx.TesSUCCESS
	}

	// Trust line is not authorized - only block if balance is zero
	// Reference: rippled DirectStep.cpp:424
	// !((*sleLine)[sfFlags] & authField) && (*sleLine)[sfBalance] == beast::zero
	if trustLine.Balance.IsZero() {
		return tx.TerNO_AUTH
	}

	// Non-zero balance means existing relationship, allow it
	return tx.TesSUCCESS
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
// Reference: rippled Payment.cpp preflight() function
func (p *Payment) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	if p.Destination == "" {
		return errors.New("temDST_NEEDED: Destination is required")
	}

	if p.Amount.IsZero() {
		return errors.New("temBAD_AMOUNT: Amount is required")
	}

	// Determine if this is an XRP-to-XRP (direct) payment
	// Reference: rippled Payment.cpp:129
	xrpDirect := p.Amount.IsNative() && (p.SendMax == nil || p.SendMax.IsNative())

	// Check flags based on payment type
	flags := p.GetFlags()
	partialPaymentAllowed := (flags & PaymentFlagPartialPayment) != 0
	limitQuality := (flags & PaymentFlagLimitQuality) != 0
	noRippleDirect := (flags & PaymentFlagNoDirectRipple) != 0
	hasPaths := len(p.Paths) > 0

	// Cannot send XRP to self without paths (temREDUNDANT)
	// Reference: rippled Payment.cpp:159-167
	if p.Account == p.Destination && p.Amount.IsNative() && !hasPaths {
		return errors.New("temREDUNDANT: cannot send XRP to self without path")
	}

	// XRP to XRP with SendMax is invalid (temBAD_SEND_XRP_MAX)
	// Reference: rippled Payment.cpp:168-174
	if xrpDirect && p.SendMax != nil {
		return errors.New("temBAD_SEND_XRP_MAX: SendMax specified for XRP to XRP")
	}

	// XRP to XRP with paths is invalid (temBAD_SEND_XRP_PATHS)
	// Reference: rippled Payment.cpp:175-181
	if xrpDirect && hasPaths {
		return errors.New("temBAD_SEND_XRP_PATHS: Paths specified for XRP to XRP")
	}

	// tfPartialPayment flag is invalid for XRP-to-XRP payments (temBAD_SEND_XRP_PARTIAL)
	// Reference: rippled Payment.cpp:182-188
	if xrpDirect && partialPaymentAllowed {
		return errors.New("temBAD_SEND_XRP_PARTIAL: Partial payment specified for XRP to XRP")
	}

	// tfLimitQuality flag is invalid for XRP-to-XRP payments (temBAD_SEND_XRP_LIMIT)
	// Reference: rippled Payment.cpp:189-196
	if xrpDirect && limitQuality {
		return errors.New("temBAD_SEND_XRP_LIMIT: Limit quality specified for XRP to XRP")
	}

	// tfNoRippleDirect flag is invalid for XRP-to-XRP payments (temBAD_SEND_XRP_NO_DIRECT)
	// Reference: rippled Payment.cpp:197-204
	if xrpDirect && noRippleDirect {
		return errors.New("temBAD_SEND_XRP_NO_DIRECT: No ripple direct specified for XRP to XRP")
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

		// DeliverMin cannot exceed Amount
		// Reference: rippled Payment.cpp:232-238
		if p.DeliverMin.Compare(p.Amount) > 0 {
			return errors.New("temBAD_AMOUNT: DeliverMin cannot exceed Amount")
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

	// Validate path elements
	// Reference: rippled PaySteps.cpp:157-186
	if err := p.validatePathElements(); err != nil {
		return err
	}

	return nil
}

// validatePathElements validates individual path elements
// Reference: rippled PaySteps.cpp toStrand() lines 157-186
func (p *Payment) validatePathElements() error {
	for _, path := range p.Paths {
		for _, elem := range path {
			// Determine what the element has
			hasAccount := elem.Account != ""
			hasCurrency := elem.Currency != ""
			hasIssuer := elem.Issuer != ""

			// Calculate element type
			elemType := 0
			if hasAccount {
				elemType |= int(PathTypeAccount)
			}
			if hasCurrency {
				elemType |= int(PathTypeCurrency)
			}
			if hasIssuer {
				elemType |= int(PathTypeIssuer)
			}

			// Path element with type zero is invalid
			// Reference: rippled PaySteps.cpp:161 - if ((t & ~STPathElement::typeAll) || !t)
			if elemType == 0 {
				return errors.New("temBAD_PATH: Path element has no account, currency, or issuer")
			}

			// Account element cannot also have currency or issuer
			// Reference: rippled PaySteps.cpp:168-169
			if hasAccount && (hasCurrency || hasIssuer) {
				return errors.New("temBAD_PATH: Path element has account with currency or issuer")
			}

			// XRP issuer is invalid (issuer must not be XRP pseudo-account)
			// Reference: rippled PaySteps.cpp:171-172
			if hasIssuer && (elem.Issuer == "rrrrrrrrrrrrrrrrrrrrrhoLvTp" || elem.Issuer == "" && hasCurrency && elem.Currency == "XRP") {
				return errors.New("temBAD_PATH: Path element has XRP issuer")
			}

			// XRP account in path is invalid (account must not be XRP pseudo-account)
			// Reference: rippled PaySteps.cpp:174-175
			if hasAccount && elem.Account == "rrrrrrrrrrrrrrrrrrrrrhoLvTp" {
				return errors.New("temBAD_PATH: Path element has XRP account")
			}

			// XRP currency with non-XRP issuer or vice versa is invalid
			// Reference: rippled PaySteps.cpp:177-179
			if hasCurrency && hasIssuer {
				isXRPCurrency := elem.Currency == "XRP" || elem.Currency == ""
				isXRPIssuer := elem.Issuer == "rrrrrrrrrrrrrrrrrrrrrhoLvTp" || elem.Issuer == ""
				if isXRPCurrency != isXRPIssuer {
					return errors.New("temBAD_PATH: XRP currency mismatch with issuer")
				}
			}
		}
	}
	return nil
}

// Flatten returns a flat map of all transaction fields
func (p *Payment) Flatten() (map[string]any, error) {
	m, err := tx.ReflectFlatten(p)
	if err != nil {
		return nil, err
	}

	// Convert Paths from [][]PathStep to []any for serialization
	if len(p.Paths) > 0 {
		pathSet := make([]any, len(p.Paths))
		for i, path := range p.Paths {
			pathSteps := make([]any, len(path))
			for j, step := range path {
				stepMap := make(map[string]any)
				if step.Account != "" {
					stepMap["account"] = step.Account
				}
				if step.Currency != "" {
					stepMap["currency"] = step.Currency
				}
				if step.Issuer != "" {
					stepMap["issuer"] = step.Issuer
				}
				pathSteps[j] = stepMap
			}
			pathSet[i] = pathSteps
		}
		m["Paths"] = pathSet
	}

	return m, nil
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

	// Use the tx.Amount directly (no conversion needed)
	amount := p.Amount

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

	// Third-party transfers (sender is not issuer AND dest is not issuer) require path finding
	// because the payment must "ripple" through the issuer (e.g., alice -> gw -> bob)
	// Reference: rippled Payment.cpp - when ripple=true, uses RippleCalc
	if !senderIsIssuer && !destIsIssuer {
		requiresPathFinding = true
	}

	// Check destination exists (needed for DepositAuth check and destination flags)
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

	// Check deposit authorization for IOU payments (including path-finding payments)
	// Reference: rippled Payment.cpp:429-464
	// IOU payments (ripple=true) require either:
	// 1. Destination does not have lsfDepositAuth set, OR
	// 2. Sender is destination (self-payment), OR
	// 3. Sender is preauthorized via DepositPreauth
	// This check MUST happen before path finding because path payments also need this check.
	if (destAccount.Flags & sle.LsfDepositAuth) != 0 {
		// Check if this is a self-payment (always allowed)
		if senderAccountID != destAccountID {
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
	}

	// For path-finding payments, use the Flow Engine (RippleCalculate)
	if requiresPathFinding {
		return p.applyIOUPaymentWithPaths(ctx, senderAccountID, destAccountID, issuerAccountID)
	}

	// Determine if partial payment is allowed
	flags := p.GetFlags()
	partialPayment := (flags & PaymentFlagPartialPayment) != 0

	// Handle three cases:
	// 1. Sender is issuer - creating new tokens
	// 2. Destination is issuer - redeeming tokens
	// 3. Neither - transfer between accounts via trust lines

	var result tx.Result
	var deliveredAmount tx.Amount

	if senderIsIssuer {
		// Sender is issuing their own currency to destination
		// Need trust line from destination to sender (issuer)
		result, deliveredAmount = p.applyIOUIssueWithDelivered(ctx, destAccount, senderAccountID, destAccountID, amount, partialPayment)
	} else if destIsIssuer {
		// Destination is the issuer - sender is redeeming tokens
		// Need trust line from sender to destination (issuer)
		result, deliveredAmount = p.applyIOURedeemWithDelivered(ctx, destAccount, senderAccountID, destAccountID, amount, partialPayment)
	} else {
		// Neither is issuer - transfer between two non-issuer accounts
		// This requires trust lines from both parties to the issuer
		result, deliveredAmount = p.applyIOUTransferWithDelivered(ctx, destAccount, senderAccountID, destAccountID, issuerAccountID, amount, partialPayment)
	}

	// DeliverMin enforcement for partial payments
	// Reference: rippled Payment.cpp:496-500
	// If tfPartialPayment is set and DeliverMin is specified, check that delivered >= DeliverMin
	if result == tx.TesSUCCESS && p.DeliverMin != nil {
		flags := p.GetFlags()
		if (flags & PaymentFlagPartialPayment) != 0 {
			if deliveredAmount.Compare(*p.DeliverMin) < 0 {
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
func (p *Payment) applyIOUIssue(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID [20]byte, amount tx.Amount) tx.Result {
	// Look up the trust line between destination and issuer (sender)
	trustLineKey := keylet.Line(destID, senderID, amount.Currency)

	trustLineExists, err := ctx.View.Exists(trustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}

	if !trustLineExists {
		// No trust line exists - destination has not authorized holding this currency
		return tx.TerNO_LINE
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

	// Check trust line authorization
	// Reference: rippled DirectStep.cpp:417-430
	// Issuer (sender) may require auth - check if destination's trust line is authorized
	if result := checkTrustLineAuthorization(ctx.View, senderID, destID, rippleState); result != tx.TesSUCCESS {
		fmt.Println("passed check")
		return result
	}

	// Determine which side is low/high account
	destIsLow := sle.CompareAccountIDsForLine(destID, senderID) < 0

	// Get the trust limit set by the destination (recipient)
	var trustLimit tx.Amount
	if destIsLow {
		trustLimit = rippleState.LowLimit
	} else {
		trustLimit = rippleState.HighLimit
	}

	// Calculate new balance after adding the amount
	// RippleState balance semantics:
	// - Negative balance = LOW owes HIGH (HIGH holds tokens)
	// - Positive balance = HIGH owes LOW (LOW holds tokens)
	var newBalance tx.Amount
	if destIsLow {
		// Dest is LOW, sender (issuer) is HIGH
		// Issuing means issuer (HIGH) now owes dest (LOW) more
		// Positive balance = HIGH owes LOW, so make MORE positive
		newBalance, _ = rippleState.Balance.Add(amount)
	} else {
		// Dest is HIGH, sender (issuer) is LOW
		// Issuing means issuer (LOW) now owes dest (HIGH) more
		// Negative balance = LOW owes HIGH, so make MORE negative
		newBalance, _ = rippleState.Balance.Sub(amount)
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
func (p *Payment) applyIOURedeem(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID [20]byte, amount tx.Amount) tx.Result {
	// Look up the trust line between sender and issuer (destination)
	trustLineKey := keylet.Line(senderID, destID, amount.Currency)

	trustLineExists, err := ctx.View.Exists(trustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}

	if !trustLineExists {
		// No trust line exists - sender doesn't hold this currency
		return tx.TerNO_LINE
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
	var senderBalance tx.Amount
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
	var newBalance tx.Amount
	if senderIsLow {
		// Sender is LOW, issuer is HIGH
		// Positive balance = sender holds. Reduce by subtracting.
		newBalance, _ = rippleState.Balance.Sub(amount)
	} else {
		// Sender is HIGH, issuer is LOW
		// Negative balance = sender holds. Make less negative by adding.
		newBalance, _ = rippleState.Balance.Add(amount)
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
// Reference: rippled Payment.cpp, DirectStep.cpp, and StepChecks.h
func (p *Payment) applyIOUTransfer(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID, issuerID [20]byte, amount tx.Amount) tx.Result {
	// Both sender and destination need trust lines to the issuer

	// Check if issuer has GlobalFreeze enabled
	// Reference: rippled StepChecks.h checkFreeze() line 45-48
	issuerKey := keylet.Account(issuerID)
	issuerData, err := ctx.View.Read(issuerKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	issuerAccount, err := sle.ParseAccountRoot(issuerData)
	if err != nil {
		return tx.TefINTERNAL
	}
	if (issuerAccount.Flags & sle.LsfGlobalFreeze) != 0 {
		return tx.TerNO_LINE
	}

	// Get transfer rate from issuer for holder-to-holder transfers
	// Reference: rippled Payment.cpp:544-557 (MPT) and DirectStep.cpp:798 (IOU)
	transferRate := GetTransferRate(ctx.View, issuerID)

	// Calculate gross amount sender needs to spend (includes transfer fee)
	// grossAmount = amount * (transferRate / QualityOne), round up
	grossAmount := amount.MulRatio(transferRate, QualityOne, true)

	// Get sender's trust line to issuer
	senderTrustLineKey := keylet.Line(senderID, issuerID, amount.Currency)
	senderTrustExists, err := ctx.View.Exists(senderTrustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	if !senderTrustExists {
		return tx.TerNO_LINE
	}

	// Get destination's trust line to issuer
	destTrustLineKey := keylet.Line(destID, issuerID, amount.Currency)
	destTrustExists, err := ctx.View.Exists(destTrustLineKey)
	if err != nil {
		return tx.TefINTERNAL
	}
	if !destTrustExists {
		return tx.TerNO_LINE
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

	// Check trust line authorization for sender
	// Reference: rippled DirectStep.cpp:417-430
	if result := checkTrustLineAuthorization(ctx.View, issuerID, senderID, senderRippleState); result != tx.TesSUCCESS {
		return result
	}

	// Check if sender's trust line is frozen by the issuer
	// Reference: rippled StepChecks.h checkFreeze() line 51-56
	// checkFreeze(src, dst, currency) checks if dst has frozen their trust line with src
	// For sender→issuer, we check if issuer has frozen
	senderIsLowInTrustLine := sle.CompareAccountIDsForLine(senderID, issuerID) < 0
	if senderIsLowInTrustLine {
		// Sender is LOW, issuer is HIGH
		// Check if issuer (HIGH) has frozen sender's trust line
		if (senderRippleState.Flags & sle.LsfHighFreeze) != 0 {
			return tx.TerNO_LINE
		}
	} else {
		// Sender is HIGH, issuer is LOW
		// Check if issuer (LOW) has frozen sender's trust line
		if (senderRippleState.Flags & sle.LsfLowFreeze) != 0 {
			return tx.TerNO_LINE
		}
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

	// Check trust line authorization for destination
	// Reference: rippled DirectStep.cpp:417-430
	if result := checkTrustLineAuthorization(ctx.View, issuerID, destID, destRippleState); result != tx.TesSUCCESS {
		return result
	}

	// Check if destination's trust line is frozen (holder freeze)
	// Reference: rippled StepChecks.h checkFreeze(src, dst) line 51-56
	// checkFreeze checks if DST has frozen their trust line with SRC
	// For issuer→dest: checkFreeze(issuer, dest) checks DEST's freeze flag
	destIsLowInTrustLine := sle.CompareAccountIDsForLine(destID, issuerID) < 0
	if destIsLowInTrustLine {
		// Dest is LOW, issuer is HIGH
		// Check if dest (LOW) has frozen their trust line
		if (destRippleState.Flags & sle.LsfLowFreeze) != 0 {
			return tx.TerNO_LINE
		}
	} else {
		// Dest is HIGH, issuer is LOW
		// Check if dest (HIGH) has frozen their trust line
		if (destRippleState.Flags & sle.LsfHighFreeze) != 0 {
			return tx.TerNO_LINE
		}
	}

	// Calculate sender's balance with issuer
	// RippleState balance semantics:
	// - Negative balance = LOW owes HIGH (HIGH holds tokens)
	// - Positive balance = HIGH owes LOW (LOW holds tokens)
	senderIsLowWithIssuer := sle.CompareAccountIDsForLine(senderID, issuerID) < 0
	var senderBalance tx.Amount
	if senderIsLowWithIssuer {
		// Sender is LOW, issuer is HIGH
		// Positive balance = sender holds tokens (HIGH/issuer owes LOW/sender)
		senderBalance = senderRippleState.Balance
	} else {
		// Sender is HIGH, issuer is LOW
		// Negative balance = sender holds tokens (LOW/issuer owes HIGH/sender)
		senderBalance = senderRippleState.Balance.Negate()
	}

	// Check sender has enough for gross amount (includes transfer fee)
	if senderBalance.Compare(grossAmount) < 0 {
		return tx.TecPATH_PARTIAL
	}

	// Calculate destination's current balance and trust limit
	destIsLowWithIssuer := sle.CompareAccountIDsForLine(destID, issuerID) < 0
	var destBalance, destLimit tx.Amount
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

	// Check destination trust limit (for net amount delivered)
	newDestBalance, _ := destBalance.Add(amount)
	if !destLimit.IsZero() && newDestBalance.Compare(destLimit) > 0 {
		return tx.TecPATH_PARTIAL
	}

	// Update sender's trust line (decrease by GROSS amount - includes transfer fee)
	// The transfer fee reduces the issuer's liability (sender pays more than dest receives)
	var newSenderRippleBalance tx.Amount
	if senderIsLowWithIssuer {
		// Sender is LOW, positive balance = holdings. Decrease by subtracting gross.
		newSenderRippleBalance, _ = senderRippleState.Balance.Sub(grossAmount)
	} else {
		// Sender is HIGH, negative balance = holdings. Make less negative by adding gross.
		newSenderRippleBalance, _ = senderRippleState.Balance.Add(grossAmount)
	}
	// Ensure the new balance has the correct currency and issuer
	newSenderRippleBalance.Currency = amount.Currency
	newSenderRippleBalance.Issuer = amount.Issuer
	senderRippleState.Balance = newSenderRippleBalance

	// Update PreviousTxnID and PreviousTxnLgrSeq on sender's trust line
	senderRippleState.PreviousTxnID = ctx.TxHash
	senderRippleState.PreviousTxnLgrSeq = ctx.Config.LedgerSequence

	// Update destination's trust line (increase by NET amount - what they actually receive)
	var newDestRippleBalance tx.Amount
	if destIsLowWithIssuer {
		// Dest is LOW, positive balance = holdings. Increase by adding net amount.
		newDestRippleBalance, _ = destRippleState.Balance.Add(amount)
	} else {
		// Dest is HIGH, negative balance = holdings. Make more negative by subtracting net amount.
		newDestRippleBalance, _ = destRippleState.Balance.Sub(amount)
	}
	// Ensure the new balance has the correct currency and issuer
	newDestRippleBalance.Currency = amount.Currency
	newDestRippleBalance.Issuer = amount.Issuer
	destRippleState.Balance = newDestRippleBalance

	// Update PreviousTxnID and PreviousTxnLgrSeq on dest's trust line
	destRippleState.PreviousTxnID = ctx.TxHash
	destRippleState.PreviousTxnLgrSeq = ctx.Config.LedgerSequence

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

	// Delivered amount is the NET amount (what destination actually receives)
	delivered := amount
	ctx.Metadata.DeliveredAmount = &delivered

	return tx.TesSUCCESS
}

// applyIOUIssueWithDelivered wraps applyIOUIssue to return the delivered amount
// If partialPayment is true and the full amount cannot be issued, it will issue
// as much as possible up to the destination's trust limit.
func (p *Payment) applyIOUIssueWithDelivered(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID [20]byte, amount tx.Amount, partialPayment bool) (tx.Result, tx.Amount) {
	return p.applyIOUIssuePartial(ctx, dest, senderID, destID, amount, partialPayment)
}

// applyIOURedeemWithDelivered wraps applyIOURedeem to return the delivered amount
// If partialPayment is true and the full amount cannot be redeemed, it will redeem
// as much as possible based on sender's balance.
func (p *Payment) applyIOURedeemWithDelivered(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID [20]byte, amount tx.Amount, partialPayment bool) (tx.Result, tx.Amount) {
	return p.applyIOURedeemPartial(ctx, dest, senderID, destID, amount, partialPayment)
}

// applyIOUTransferWithDelivered wraps applyIOUTransfer to return the delivered amount
// If partialPayment is true and the full amount cannot be transferred, it will transfer
// as much as possible based on sender's balance and destination's trust limit.
func (p *Payment) applyIOUTransferWithDelivered(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID, issuerID [20]byte, amount tx.Amount, partialPayment bool) (tx.Result, tx.Amount) {
	return p.applyIOUTransferPartial(ctx, dest, senderID, destID, issuerID, amount, partialPayment)
}

// ============================================================================
// Partial Payment Implementations
// Reference: rippled Flow.cpp, RippleCalc.cpp
// ============================================================================

// applyIOUIssuePartial handles issuing currency with partial payment support
func (p *Payment) applyIOUIssuePartial(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID [20]byte, amount tx.Amount, partialPayment bool) (tx.Result, tx.Amount) {
	// Look up the trust line between destination and issuer (sender)
	trustLineKey := keylet.Line(destID, senderID, amount.Currency)

	trustLineExists, err := ctx.View.Exists(trustLineKey)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}

	if !trustLineExists {
		return tx.TerNO_LINE, tx.Amount{}
	}

	// Read and parse the trust line
	trustLineData, err := ctx.View.Read(trustLineKey)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}

	rippleState, err := sle.ParseRippleState(trustLineData)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}

	// Check trust line authorization
	if result := checkTrustLineAuthorization(ctx.View, senderID, destID, rippleState); result != tx.TesSUCCESS {
		return result, tx.Amount{}
	}

	// Determine which side is low/high account
	destIsLow := sle.CompareAccountIDsForLine(destID, senderID) < 0

	// Get the current balance and trust limit
	var currentBalance, trustLimit tx.Amount
	if destIsLow {
		currentBalance = rippleState.Balance
		trustLimit = rippleState.LowLimit
	} else {
		currentBalance = rippleState.Balance.Negate()
		trustLimit = rippleState.HighLimit
	}

	// Calculate maximum we can issue based on trust limit
	var maxDeliverable tx.Amount
	if trustLimit.IsZero() {
		// No limit set, can issue any amount
		maxDeliverable = amount
	} else {
		// Calculate room available: limit - current balance
		room, _ := trustLimit.Sub(currentBalance)
		if room.IsNegative() || room.IsZero() {
			if partialPayment {
				return tx.TesSUCCESS, tx.Amount{} // Nothing to deliver, but partial is OK
			}
			return tx.TecPATH_PARTIAL, tx.Amount{}
		}
		if room.Compare(amount) < 0 {
			maxDeliverable = room
		} else {
			maxDeliverable = amount
		}
	}

	// If we can't deliver the full amount and partial payment is not allowed, fail
	if maxDeliverable.Compare(amount) < 0 && !partialPayment {
		return tx.TecPATH_PARTIAL, tx.Amount{}
	}

	// If nothing to deliver
	if maxDeliverable.IsZero() {
		if partialPayment {
			return tx.TesSUCCESS, tx.Amount{}
		}
		return tx.TecPATH_PARTIAL, tx.Amount{}
	}

	// Calculate new balance
	var newBalance tx.Amount
	if destIsLow {
		newBalance, _ = rippleState.Balance.Add(maxDeliverable)
	} else {
		newBalance, _ = rippleState.Balance.Sub(maxDeliverable)
	}

	// Ensure the new balance has the correct currency and issuer
	newBalance.Currency = amount.Currency
	newBalance.Issuer = amount.Issuer

	// Update the trust line
	rippleState.Balance = newBalance
	rippleState.PreviousTxnID = ctx.TxHash
	rippleState.PreviousTxnLgrSeq = ctx.Config.LedgerSequence

	// Check for trust line cleanup (default state → delete)
	// Reference: rippled View.cpp rippleCreditIOU() lines 1692-1745
	if result := trustLineCleanup(ctx, dest, senderID, destID, trustLineKey, rippleState); result != tx.TesSUCCESS {
		return result, tx.Amount{}
	}

	ctx.Metadata.DeliveredAmount = &maxDeliverable

	return tx.TesSUCCESS, maxDeliverable
}

// applyIOURedeemPartial handles redeeming currency with partial payment support
func (p *Payment) applyIOURedeemPartial(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID [20]byte, amount tx.Amount, partialPayment bool) (tx.Result, tx.Amount) {
	// Look up the trust line between sender and issuer (destination)
	trustLineKey := keylet.Line(senderID, destID, amount.Currency)

	trustLineExists, err := ctx.View.Exists(trustLineKey)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}

	if !trustLineExists {
		return tx.TerNO_LINE, tx.Amount{}
	}

	// Read and parse the trust line
	trustLineData, err := ctx.View.Read(trustLineKey)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}

	rippleState, err := sle.ParseRippleState(trustLineData)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}

	// Determine which side is low/high account
	senderIsLow := sle.CompareAccountIDsForLine(senderID, destID) < 0

	// Get sender's current balance
	var senderBalance tx.Amount
	if senderIsLow {
		senderBalance = rippleState.Balance
	} else {
		senderBalance = rippleState.Balance.Negate()
	}

	// Calculate maximum we can redeem based on sender's balance
	var maxDeliverable tx.Amount
	if senderBalance.Compare(amount) < 0 {
		if senderBalance.IsNegative() || senderBalance.IsZero() {
			if partialPayment {
				return tx.TesSUCCESS, tx.Amount{}
			}
			return tx.TecPATH_PARTIAL, tx.Amount{}
		}
		maxDeliverable = senderBalance
	} else {
		maxDeliverable = amount
	}

	// If we can't deliver the full amount and partial payment is not allowed, fail
	if maxDeliverable.Compare(amount) < 0 && !partialPayment {
		return tx.TecPATH_PARTIAL, tx.Amount{}
	}

	// If nothing to deliver
	if maxDeliverable.IsZero() {
		if partialPayment {
			return tx.TesSUCCESS, tx.Amount{}
		}
		return tx.TecPATH_PARTIAL, tx.Amount{}
	}

	// Update balance
	var newBalance tx.Amount
	if senderIsLow {
		newBalance, _ = rippleState.Balance.Sub(maxDeliverable)
	} else {
		newBalance, _ = rippleState.Balance.Add(maxDeliverable)
	}

	newBalance.Currency = amount.Currency
	newBalance.Issuer = amount.Issuer
	rippleState.Balance = newBalance
	rippleState.PreviousTxnID = ctx.TxHash
	rippleState.PreviousTxnLgrSeq = ctx.Config.LedgerSequence

	// Check for trust line cleanup (default state → delete)
	// Reference: rippled View.cpp rippleCreditIOU() lines 1692-1745
	if result := trustLineCleanup(ctx, dest, senderID, destID, trustLineKey, rippleState); result != tx.TesSUCCESS {
		return result, tx.Amount{}
	}

	ctx.Metadata.DeliveredAmount = &maxDeliverable

	return tx.TesSUCCESS, maxDeliverable
}

// applyIOUTransferPartial handles transfers between non-issuer accounts with partial payment support
func (p *Payment) applyIOUTransferPartial(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID, issuerID [20]byte, amount tx.Amount, partialPayment bool) (tx.Result, tx.Amount) {
	// Check if issuer has GlobalFreeze enabled
	issuerKey := keylet.Account(issuerID)
	issuerData, err := ctx.View.Read(issuerKey)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}
	issuerAccount, err := sle.ParseAccountRoot(issuerData)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}
	if (issuerAccount.Flags & sle.LsfGlobalFreeze) != 0 {
		return tx.TerNO_LINE, tx.Amount{}
	}

	// Get transfer rate from issuer
	transferRate := GetTransferRate(ctx.View, issuerID)

	// Get sender's trust line to issuer
	senderTrustLineKey := keylet.Line(senderID, issuerID, amount.Currency)
	senderTrustExists, err := ctx.View.Exists(senderTrustLineKey)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}
	if !senderTrustExists {
		return tx.TerNO_LINE, tx.Amount{}
	}

	// Get destination's trust line to issuer
	destTrustLineKey := keylet.Line(destID, issuerID, amount.Currency)
	destTrustExists, err := ctx.View.Exists(destTrustLineKey)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}
	if !destTrustExists {
		return tx.TerNO_LINE, tx.Amount{}
	}

	// Read sender's trust line
	senderTrustData, err := ctx.View.Read(senderTrustLineKey)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}
	senderRippleState, err := sle.ParseRippleState(senderTrustData)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}

	// Check trust line authorization for sender
	if result := checkTrustLineAuthorization(ctx.View, issuerID, senderID, senderRippleState); result != tx.TesSUCCESS {
		return result, tx.Amount{}
	}

	// Check if sender's trust line is frozen
	senderIsLowInTrustLine := sle.CompareAccountIDsForLine(senderID, issuerID) < 0
	if senderIsLowInTrustLine {
		if (senderRippleState.Flags & sle.LsfHighFreeze) != 0 {
			return tx.TerNO_LINE, tx.Amount{}
		}
	} else {
		if (senderRippleState.Flags & sle.LsfLowFreeze) != 0 {
			return tx.TerNO_LINE, tx.Amount{}
		}
	}

	// Read destination's trust line
	destTrustData, err := ctx.View.Read(destTrustLineKey)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}
	destRippleState, err := sle.ParseRippleState(destTrustData)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}

	// Check trust line authorization for destination
	if result := checkTrustLineAuthorization(ctx.View, issuerID, destID, destRippleState); result != tx.TesSUCCESS {
		return result, tx.Amount{}
	}

	// Check if destination's trust line is frozen
	destIsLowInTrustLine := sle.CompareAccountIDsForLine(destID, issuerID) < 0
	if destIsLowInTrustLine {
		if (destRippleState.Flags & sle.LsfLowFreeze) != 0 {
			return tx.TerNO_LINE, tx.Amount{}
		}
	} else {
		if (destRippleState.Flags & sle.LsfHighFreeze) != 0 {
			return tx.TerNO_LINE, tx.Amount{}
		}
	}

	// Calculate sender's balance with issuer
	senderIsLowWithIssuer := sle.CompareAccountIDsForLine(senderID, issuerID) < 0
	var senderBalance tx.Amount
	if senderIsLowWithIssuer {
		senderBalance = senderRippleState.Balance
	} else {
		senderBalance = senderRippleState.Balance.Negate()
	}

	// Calculate destination's balance and trust limit
	destIsLowWithIssuer := sle.CompareAccountIDsForLine(destID, issuerID) < 0
	var destBalance, destLimit tx.Amount
	if destIsLowWithIssuer {
		destBalance = destRippleState.Balance
		destLimit = destRippleState.LowLimit
	} else {
		destBalance = destRippleState.Balance.Negate()
		destLimit = destRippleState.HighLimit
	}

	// Calculate maximum deliverable based on:
	// 1. Sender's available balance (accounting for transfer fee)
	// 2. Destination's trust limit room

	// Max based on sender's balance: senderBalance * (QualityOne / transferRate)
	// This accounts for the transfer fee in reverse
	maxFromSender := senderBalance.MulRatio(QualityOne, transferRate, false)

	// Max based on destination's trust limit
	var maxFromDestLimit tx.Amount
	if destLimit.IsZero() {
		// No limit - use a very large value (effectively unlimited)
		maxFromDestLimit = amount
	} else {
		room, _ := destLimit.Sub(destBalance)
		if room.IsNegative() {
			room = tx.NewIssuedAmountFromFloat64(0, amount.Currency, amount.Issuer)
		}
		maxFromDestLimit = room
	}

	// Actual max is minimum of the two constraints
	var maxDeliverable tx.Amount
	if maxFromSender.Compare(maxFromDestLimit) < 0 {
		maxDeliverable = maxFromSender
	} else {
		maxDeliverable = maxFromDestLimit
	}

	// Cap at requested amount
	if maxDeliverable.Compare(amount) > 0 {
		maxDeliverable = amount
	}

	// If we can't deliver anything
	if maxDeliverable.IsZero() || maxDeliverable.IsNegative() {
		if partialPayment {
			return tx.TesSUCCESS, tx.Amount{}
		}
		return tx.TecPATH_PARTIAL, tx.Amount{}
	}

	// If we can't deliver the full amount and partial payment is not allowed, fail
	if maxDeliverable.Compare(amount) < 0 && !partialPayment {
		return tx.TecPATH_PARTIAL, tx.Amount{}
	}

	// Calculate gross amount sender needs to spend (includes transfer fee)
	grossAmount := maxDeliverable.MulRatio(transferRate, QualityOne, true)

	// Update sender's trust line
	var newSenderRippleBalance tx.Amount
	if senderIsLowWithIssuer {
		newSenderRippleBalance, _ = senderRippleState.Balance.Sub(grossAmount)
	} else {
		newSenderRippleBalance, _ = senderRippleState.Balance.Add(grossAmount)
	}
	newSenderRippleBalance.Currency = amount.Currency
	newSenderRippleBalance.Issuer = amount.Issuer
	senderRippleState.Balance = newSenderRippleBalance
	senderRippleState.PreviousTxnID = ctx.TxHash
	senderRippleState.PreviousTxnLgrSeq = ctx.Config.LedgerSequence

	// Update destination's trust line
	var newDestRippleBalance tx.Amount
	if destIsLowWithIssuer {
		newDestRippleBalance, _ = destRippleState.Balance.Add(maxDeliverable)
	} else {
		newDestRippleBalance, _ = destRippleState.Balance.Sub(maxDeliverable)
	}
	newDestRippleBalance.Currency = amount.Currency
	newDestRippleBalance.Issuer = amount.Issuer
	destRippleState.Balance = newDestRippleBalance
	destRippleState.PreviousTxnID = ctx.TxHash
	destRippleState.PreviousTxnLgrSeq = ctx.Config.LedgerSequence

	// Serialize and update sender's trust line
	updatedSenderTrust, err := sle.SerializeRippleState(senderRippleState)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}
	if err := ctx.View.Update(senderTrustLineKey, updatedSenderTrust); err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}

	// Serialize and update destination's trust line
	updatedDestTrust, err := sle.SerializeRippleState(destRippleState)
	if err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}
	if err := ctx.View.Update(destTrustLineKey, updatedDestTrust); err != nil {
		return tx.TefINTERNAL, tx.Amount{}
	}

	ctx.Metadata.DeliveredAmount = &maxDeliverable

	return tx.TesSUCCESS, maxDeliverable
}

// trustLineCleanup checks if a trust line is in default state after a balance modification
// and deletes it if so, adjusting OwnerCount for both accounts.
// This matches rippled's rippleCreditIOU() logic in View.cpp lines 1692-1745.
//
// Parameters:
//   - ctx: apply context (ctx.Account = transaction sender, identified by senderID)
//   - dest: the other account's AccountRoot (identified by destID)
//   - senderID, destID: the two account IDs on the trust line
//   - tlKey: the keylet for the trust line
//   - rs: the already-modified RippleState (balance updated, not yet serialized)
//
// On success, either updates or erases the trust line via ctx.View. Returns TesSUCCESS.
func trustLineCleanup(ctx *tx.ApplyContext, dest *sle.AccountRoot, senderID, destID [20]byte, tlKey keylet.Keylet, rs *sle.RippleState) tx.Result {
	senderIsLow := sle.CompareAccountIDsForLine(senderID, destID) < 0

	// Get both accounts' DefaultRipple flags
	var lowDefRipple, highDefRipple bool
	if senderIsLow {
		lowDefRipple = (ctx.Account.Flags & sle.LsfDefaultRipple) != 0
		highDefRipple = (dest.Flags & sle.LsfDefaultRipple) != 0
	} else {
		lowDefRipple = (dest.Flags & sle.LsfDefaultRipple) != 0
		highDefRipple = (ctx.Account.Flags & sle.LsfDefaultRipple) != 0
	}

	bLowReserveSet := rs.LowQualityIn != 0 || rs.LowQualityOut != 0 ||
		((rs.Flags&sle.LsfLowNoRipple) == 0) != lowDefRipple ||
		(rs.Flags&sle.LsfLowFreeze) != 0 || !rs.LowLimit.IsZero() ||
		rs.Balance.Signum() > 0

	bHighReserveSet := rs.HighQualityIn != 0 || rs.HighQualityOut != 0 ||
		((rs.Flags&sle.LsfHighNoRipple) == 0) != highDefRipple ||
		(rs.Flags&sle.LsfHighFreeze) != 0 || !rs.HighLimit.IsZero() ||
		rs.Balance.Signum() < 0

	bLowReserved := (rs.Flags & sle.LsfLowReserve) != 0
	bHighReserved := (rs.Flags & sle.LsfHighReserve) != 0

	bDefault := !bLowReserveSet && !bHighReserveSet

	if bDefault && rs.Balance.IsZero() {
		// Remove from both owner directories before erasing
		// Reference: rippled trustDelete() in View.cpp
		var lowID, highID [20]byte
		if senderIsLow {
			lowID = senderID
			highID = destID
		} else {
			lowID = destID
			highID = senderID
		}
		lowDirKey := keylet.OwnerDir(lowID)
		sle.DirRemove(ctx.View, lowDirKey, rs.LowNode, tlKey.Key, false)
		highDirKey := keylet.OwnerDir(highID)
		sle.DirRemove(ctx.View, highDirKey, rs.HighNode, tlKey.Key, false)

		// Delete the trust line
		if err := ctx.View.Erase(tlKey); err != nil {
			return tx.TefINTERNAL
		}

		// Decrement OwnerCount for both sides that had reserve set
		if bLowReserved {
			if senderIsLow {
				if ctx.Account.OwnerCount > 0 {
					ctx.Account.OwnerCount--
				}
			} else {
				if dest.OwnerCount > 0 {
					dest.OwnerCount--
				}
			}
		}
		if bHighReserved {
			if !senderIsLow {
				if ctx.Account.OwnerCount > 0 {
					ctx.Account.OwnerCount--
				}
			} else {
				if dest.OwnerCount > 0 {
					dest.OwnerCount--
				}
			}
		}

		// Write dest account back if its OwnerCount changed
		destChanged := (bLowReserved && !senderIsLow) || (bHighReserved && senderIsLow)
		if destChanged {
			destKey := keylet.Account(destID)
			destUpdatedData, serErr := sle.SerializeAccountRoot(dest)
			if serErr != nil {
				return tx.TefINTERNAL
			}
			if err := ctx.View.Update(destKey, destUpdatedData); err != nil {
				return tx.TefINTERNAL
			}
		}
	} else {
		// Adjust reserve flags
		if bLowReserveSet && !bLowReserved {
			rs.Flags |= sle.LsfLowReserve
		} else if !bLowReserveSet && bLowReserved {
			rs.Flags &^= sle.LsfLowReserve
		}
		if bHighReserveSet && !bHighReserved {
			rs.Flags |= sle.LsfHighReserve
		} else if !bHighReserveSet && bHighReserved {
			rs.Flags &^= sle.LsfHighReserve
		}

		// Serialize and update
		updatedData, serErr := sle.SerializeRippleState(rs)
		if serErr != nil {
			return tx.TefINTERNAL
		}
		if err := ctx.View.Update(tlKey, updatedData); err != nil {
			return tx.TefINTERNAL
		}
	}

	return tx.TesSUCCESS
}
