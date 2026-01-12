package payment

import (
	"encoding/hex"
	"math/big"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx/handler"
)

// IOUAmount represents an IOU amount with currency and issuer.
type IOUAmount struct {
	Value    *big.Float
	Currency string
	Issuer   string
}

// NewIOUAmount creates a new IOU amount from string values.
func NewIOUAmount(value, currency, issuer string) IOUAmount {
	v := new(big.Float)
	v.SetString(value)
	return IOUAmount{
		Value:    v,
		Currency: currency,
		Issuer:   issuer,
	}
}

// IsZero returns true if the amount is zero.
func (a IOUAmount) IsZero() bool {
	if a.Value == nil {
		return true
	}
	return a.Value.Cmp(new(big.Float)) == 0
}

// IsNegative returns true if the amount is negative.
func (a IOUAmount) IsNegative() bool {
	if a.Value == nil {
		return false
	}
	return a.Value.Cmp(new(big.Float)) < 0
}

// Add adds two IOU amounts.
func (a IOUAmount) Add(b IOUAmount) IOUAmount {
	result := new(big.Float).Add(a.Value, b.Value)
	return IOUAmount{Value: result, Currency: a.Currency, Issuer: a.Issuer}
}

// Sub subtracts two IOU amounts.
func (a IOUAmount) Sub(b IOUAmount) IOUAmount {
	result := new(big.Float).Sub(a.Value, b.Value)
	return IOUAmount{Value: result, Currency: a.Currency, Issuer: a.Issuer}
}

// Negate returns the negation of the amount.
func (a IOUAmount) Negate() IOUAmount {
	result := new(big.Float).Neg(a.Value)
	return IOUAmount{Value: result, Currency: a.Currency, Issuer: a.Issuer}
}

// Compare compares two IOU amounts.
func (a IOUAmount) Compare(b IOUAmount) int {
	return a.Value.Cmp(b.Value)
}

// applyIOUPayment processes an IOU (issued currency) payment.
func (h *Handler) applyIOUPayment(payment *Payment, sender *handler.AccountRoot, ctx *handler.Context) handler.Result {
	// Parse the amount
	amount := NewIOUAmount(payment.Amount.Value, payment.Amount.Currency, payment.Amount.Issuer)
	if amount.IsZero() {
		return handler.TemBAD_AMOUNT
	}
	if amount.IsNegative() {
		return handler.TemBAD_AMOUNT
	}

	// Get account IDs
	senderAccountID, err := handler.DecodeAccountID(sender.Account)
	if err != nil {
		return handler.TefINTERNAL
	}

	destAccountID, err := handler.DecodeAccountID(payment.Destination)
	if err != nil {
		return handler.TemDST_NEEDED
	}

	issuerAccountID, err := handler.DecodeAccountID(payment.Amount.Issuer)
	if err != nil {
		return handler.TemBAD_ISSUER
	}

	// Check destination exists
	destKey := keylet.Account(destAccountID)
	destExists, err := ctx.View.Exists(destKey)
	if err != nil {
		return handler.TefINTERNAL
	}
	if !destExists {
		return handler.TecNO_DST
	}

	// Get destination account to check flags
	destData, err := ctx.View.Read(destKey)
	if err != nil {
		return handler.TefINTERNAL
	}
	destAccount, err := handler.ParseAccountRoot(destData)
	if err != nil {
		return handler.TefINTERNAL
	}

	// Check destination tag requirement
	if (destAccount.Flags&0x00020000) != 0 && payment.DestinationTag == nil {
		return handler.TecDST_TAG_NEEDED
	}

	// Handle three cases:
	// 1. Sender is issuer - creating new tokens
	// 2. Destination is issuer - redeeming tokens
	// 3. Neither - transfer between accounts via trust lines

	senderIsIssuer := senderAccountID == issuerAccountID
	destIsIssuer := destAccountID == issuerAccountID

	if senderIsIssuer {
		return h.applyIOUIssue(payment, sender, destAccount, senderAccountID, destAccountID, amount, ctx)
	} else if destIsIssuer {
		return h.applyIOURedeem(payment, sender, destAccount, senderAccountID, destAccountID, amount, ctx)
	} else {
		return h.applyIOUTransfer(payment, sender, destAccount, senderAccountID, destAccountID, issuerAccountID, amount, ctx)
	}
}

// applyIOUIssue handles when sender is the issuer creating new tokens.
func (h *Handler) applyIOUIssue(payment *Payment, sender *handler.AccountRoot, dest *handler.AccountRoot, senderID, destID [20]byte, amount IOUAmount, ctx *handler.Context) handler.Result {
	trustLineKey := keylet.Line(destID, senderID, amount.Currency)

	trustLineExists, err := ctx.View.Exists(trustLineKey)
	if err != nil {
		return handler.TefINTERNAL
	}

	if !trustLineExists {
		return handler.TecPATH_DRY
	}

	trustLineData, err := ctx.View.Read(trustLineKey)
	if err != nil {
		return handler.TefINTERNAL
	}

	rippleState, err := parseRippleState(trustLineData)
	if err != nil {
		return handler.TefINTERNAL
	}

	destIsLow := compareAccountIDs(destID, senderID) < 0

	var trustLimit IOUAmount
	if destIsLow {
		trustLimit = rippleState.LowLimit
	} else {
		trustLimit = rippleState.HighLimit
	}

	var newBalance IOUAmount
	if destIsLow {
		newBalance = rippleState.Balance.Sub(amount)
	} else {
		newBalance = rippleState.Balance.Add(amount)
	}

	absNewBalance := newBalance
	if absNewBalance.IsNegative() {
		absNewBalance = absNewBalance.Negate()
	}

	if !trustLimit.IsZero() && absNewBalance.Compare(trustLimit) > 0 {
		return handler.TecPATH_PARTIAL
	}

	rippleState.Balance = newBalance

	updatedTrustLine, err := serializeRippleState(rippleState)
	if err != nil {
		return handler.TefINTERNAL
	}

	if err := ctx.View.Update(trustLineKey, updatedTrustLine); err != nil {
		return handler.TefINTERNAL
	}

	ctx.Metadata.AffectedNodes = append(ctx.Metadata.AffectedNodes, handler.AffectedNode{
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
	ctx.Metadata.DeliveredAmount = &delivered

	return handler.TesSUCCESS
}

// applyIOURedeem handles when destination is the issuer (redeeming tokens).
func (h *Handler) applyIOURedeem(payment *Payment, sender *handler.AccountRoot, dest *handler.AccountRoot, senderID, destID [20]byte, amount IOUAmount, ctx *handler.Context) handler.Result {
	trustLineKey := keylet.Line(senderID, destID, amount.Currency)

	trustLineExists, err := ctx.View.Exists(trustLineKey)
	if err != nil {
		return handler.TefINTERNAL
	}

	if !trustLineExists {
		return handler.TecPATH_DRY
	}

	trustLineData, err := ctx.View.Read(trustLineKey)
	if err != nil {
		return handler.TefINTERNAL
	}

	rippleState, err := parseRippleState(trustLineData)
	if err != nil {
		return handler.TefINTERNAL
	}

	senderIsLow := compareAccountIDs(senderID, destID) < 0

	var senderBalance IOUAmount
	if senderIsLow {
		senderBalance = rippleState.Balance.Negate()
	} else {
		senderBalance = rippleState.Balance
	}

	if senderBalance.Compare(amount) < 0 {
		return handler.TecPATH_PARTIAL
	}

	var newBalance IOUAmount
	if senderIsLow {
		newBalance = rippleState.Balance.Add(amount)
	} else {
		newBalance = rippleState.Balance.Sub(amount)
	}

	rippleState.Balance = newBalance

	updatedTrustLine, err := serializeRippleState(rippleState)
	if err != nil {
		return handler.TefINTERNAL
	}

	if err := ctx.View.Update(trustLineKey, updatedTrustLine); err != nil {
		return handler.TefINTERNAL
	}

	ctx.Metadata.AffectedNodes = append(ctx.Metadata.AffectedNodes, handler.AffectedNode{
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
	ctx.Metadata.DeliveredAmount = &delivered

	return handler.TesSUCCESS
}

// applyIOUTransfer handles transfer between two non-issuer accounts.
func (h *Handler) applyIOUTransfer(payment *Payment, sender *handler.AccountRoot, dest *handler.AccountRoot, senderID, destID, issuerID [20]byte, amount IOUAmount, ctx *handler.Context) handler.Result {
	senderTrustLineKey := keylet.Line(senderID, issuerID, amount.Currency)
	senderTrustExists, err := ctx.View.Exists(senderTrustLineKey)
	if err != nil {
		return handler.TefINTERNAL
	}
	if !senderTrustExists {
		return handler.TecPATH_DRY
	}

	destTrustLineKey := keylet.Line(destID, issuerID, amount.Currency)
	destTrustExists, err := ctx.View.Exists(destTrustLineKey)
	if err != nil {
		return handler.TefINTERNAL
	}
	if !destTrustExists {
		return handler.TecPATH_DRY
	}

	senderTrustData, err := ctx.View.Read(senderTrustLineKey)
	if err != nil {
		return handler.TefINTERNAL
	}
	senderRippleState, err := parseRippleState(senderTrustData)
	if err != nil {
		return handler.TefINTERNAL
	}

	destTrustData, err := ctx.View.Read(destTrustLineKey)
	if err != nil {
		return handler.TefINTERNAL
	}
	destRippleState, err := parseRippleState(destTrustData)
	if err != nil {
		return handler.TefINTERNAL
	}

	senderIsLowWithIssuer := compareAccountIDs(senderID, issuerID) < 0
	var senderBalance IOUAmount
	if senderIsLowWithIssuer {
		senderBalance = senderRippleState.Balance.Negate()
	} else {
		senderBalance = senderRippleState.Balance
	}

	if senderBalance.Compare(amount) < 0 {
		return handler.TecPATH_PARTIAL
	}

	destIsLowWithIssuer := compareAccountIDs(destID, issuerID) < 0
	var destBalance, destLimit IOUAmount
	if destIsLowWithIssuer {
		destBalance = destRippleState.Balance.Negate()
		destLimit = destRippleState.LowLimit
	} else {
		destBalance = destRippleState.Balance
		destLimit = destRippleState.HighLimit
	}

	newDestBalance := destBalance.Add(amount)
	if !destLimit.IsZero() && newDestBalance.Compare(destLimit) > 0 {
		return handler.TecPATH_PARTIAL
	}

	var newSenderRippleBalance IOUAmount
	if senderIsLowWithIssuer {
		newSenderRippleBalance = senderRippleState.Balance.Add(amount)
	} else {
		newSenderRippleBalance = senderRippleState.Balance.Sub(amount)
	}
	senderRippleState.Balance = newSenderRippleBalance

	var newDestRippleBalance IOUAmount
	if destIsLowWithIssuer {
		newDestRippleBalance = destRippleState.Balance.Sub(amount)
	} else {
		newDestRippleBalance = destRippleState.Balance.Add(amount)
	}
	destRippleState.Balance = newDestRippleBalance

	updatedSenderTrust, err := serializeRippleState(senderRippleState)
	if err != nil {
		return handler.TefINTERNAL
	}
	if err := ctx.View.Update(senderTrustLineKey, updatedSenderTrust); err != nil {
		return handler.TefINTERNAL
	}

	updatedDestTrust, err := serializeRippleState(destRippleState)
	if err != nil {
		return handler.TefINTERNAL
	}
	if err := ctx.View.Update(destTrustLineKey, updatedDestTrust); err != nil {
		return handler.TefINTERNAL
	}

	ctx.Metadata.AffectedNodes = append(ctx.Metadata.AffectedNodes, handler.AffectedNode{
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

	ctx.Metadata.AffectedNodes = append(ctx.Metadata.AffectedNodes, handler.AffectedNode{
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
	ctx.Metadata.DeliveredAmount = &delivered

	return handler.TesSUCCESS
}

// RippleState represents a trust line (RippleState ledger entry).
type RippleState struct {
	Balance   IOUAmount
	LowLimit  IOUAmount
	HighLimit IOUAmount
	Flags     uint32
}

// compareAccountIDs compares two account IDs for ordering.
func compareAccountIDs(a, b [20]byte) int {
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

// parseRippleState parses a RippleState from bytes.
func parseRippleState(data []byte) (*RippleState, error) {
	// Placeholder - implement actual parsing
	return &RippleState{
		Balance:   NewIOUAmount("0", "", ""),
		LowLimit:  NewIOUAmount("0", "", ""),
		HighLimit: NewIOUAmount("0", "", ""),
	}, nil
}

// serializeRippleState serializes a RippleState to bytes.
func serializeRippleState(rs *RippleState) ([]byte, error) {
	// Placeholder - implement actual serialization
	return nil, nil
}
