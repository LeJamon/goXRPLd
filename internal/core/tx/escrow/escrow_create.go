// Package escrow implements EscrowCreate, EscrowFinish, and EscrowCancel transactions.
package escrow

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	"github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

func init() {
	tx.Register(tx.TypeEscrowCreate, func() tx.Transaction {
		return &EscrowCreate{BaseTx: *tx.NewBaseTx(tx.TypeEscrowCreate, "")}
	})
}

// EscrowCreate creates an escrow that holds XRP until certain conditions are met.
type EscrowCreate struct {
	tx.BaseTx

	// Amount is the amount of XRP to escrow (required)
	Amount tx.Amount `json:"Amount" xrpl:"Amount,amount"`

	// Destination is the account to receive the XRP (required)
	Destination string `json:"Destination" xrpl:"Destination"`

	// DestinationTag is an arbitrary tag for the destination (optional)
	DestinationTag *uint32 `json:"DestinationTag,omitempty" xrpl:"DestinationTag,omitempty"`

	// CancelAfter is the time after which the escrow can be cancelled (optional)
	CancelAfter *uint32 `json:"CancelAfter,omitempty" xrpl:"CancelAfter,omitempty"`

	// FinishAfter is the time after which the escrow can be finished (optional)
	FinishAfter *uint32 `json:"FinishAfter,omitempty" xrpl:"FinishAfter,omitempty"`

	// Condition is the crypto-condition that must be fulfilled (optional)
	Condition string `json:"Condition,omitempty" xrpl:"Condition,omitempty"`
}

// NewEscrowCreate creates a new EscrowCreate transaction
func NewEscrowCreate(account, destination string, amount tx.Amount) *EscrowCreate {
	return &EscrowCreate{
		BaseTx:      *tx.NewBaseTx(tx.TypeEscrowCreate, account),
		Amount:      amount,
		Destination: destination,
	}
}

// TxType returns the transaction type
func (e *EscrowCreate) TxType() tx.Type {
	return tx.TypeEscrowCreate
}

// Validate validates the EscrowCreate transaction
// Reference: rippled Escrow.cpp EscrowCreate::preflight()
func (e *EscrowCreate) Validate() error {
	if err := e.BaseTx.Validate(); err != nil {
		return err
	}

	if e.Destination == "" {
		return errors.New("temDST_NEEDED: Destination is required")
	}

	if e.Amount.Value == "" {
		return errors.New("temBAD_AMOUNT: Amount is required")
	}

	// Amount must be positive
	// Reference: rippled Escrow.cpp:146-147
	if len(e.Amount.Value) > 0 && e.Amount.Value[0] == '-' {
		return errors.New("temBAD_AMOUNT: Amount must be positive")
	}
	if e.Amount.Value == "0" {
		return errors.New("temBAD_AMOUNT: Amount must be positive")
	}

	// Must be XRP (unless featureTokenEscrow is enabled)
	// Reference: rippled Escrow.cpp:131-148
	if !e.Amount.IsNative() {
		return errors.New("temBAD_AMOUNT: escrow can only hold XRP")
	}

	// Must have at least one timeout value
	// Reference: rippled Escrow.cpp:151-152
	if e.CancelAfter == nil && e.FinishAfter == nil {
		return errors.New("temBAD_EXPIRATION: must specify CancelAfter or FinishAfter")
	}

	// If both times are specified, CancelAfter must be strictly after FinishAfter
	// Reference: rippled Escrow.cpp:156-158
	if e.CancelAfter != nil && e.FinishAfter != nil {
		if *e.CancelAfter <= *e.FinishAfter {
			return errors.New("temBAD_EXPIRATION: CancelAfter must be after FinishAfter")
		}
	}

	// With fix1571: In the absence of a FinishAfter, must have a Condition
	// Reference: rippled Escrow.cpp:160-167
	if e.FinishAfter == nil && e.Condition == "" {
		return errors.New("temMALFORMED: must specify FinishAfter or Condition")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (e *EscrowCreate) Flatten() (map[string]any, error) {
	return tx.ReflectFlatten(e)
}

// Apply applies an EscrowCreate transaction
func (ec *EscrowCreate) Apply(ctx *tx.ApplyContext) tx.Result {
	// Parse the amount to escrow
	amount, err := strconv.ParseUint(ec.Amount.Value, 10, 64)
	if err != nil {
		return tx.TemINVALID
	}

	// Check that account has sufficient balance (after fee)
	if ctx.Account.Balance < amount {
		return tx.TecUNFUNDED
	}

	// Verify destination exists
	destID, err := sle.DecodeAccountID(ec.Destination)
	if err != nil {
		return tx.TemINVALID
	}

	destKey := keylet.Account(destID)
	exists, _ := ctx.View.Exists(destKey)
	if !exists {
		return tx.TecNO_DST
	}

	// Deduct the escrow amount from the account
	ctx.Account.Balance -= amount

	// Create the escrow entry
	accountID, _ := sle.DecodeAccountID(ec.Account)
	sequence := *ec.GetCommon().Sequence // Use the transaction sequence

	escrowKey := keylet.Escrow(accountID, sequence)

	// Serialize escrow
	escrowData, err := serializeEscrow(ec, accountID, destID, sequence, amount)
	if err != nil {
		return tx.TefINTERNAL
	}

	// Insert escrow - creation tracked automatically by ApplyStateTable
	if err := ctx.View.Insert(escrowKey, escrowData); err != nil {
		return tx.TefINTERNAL
	}

	// Increase owner count
	ctx.Account.OwnerCount++

	return tx.TesSUCCESS
}

// serializeEscrow serializes an Escrow ledger entry
func serializeEscrow(txn *EscrowCreate, ownerID, destID [20]byte, sequence uint32, amount uint64) ([]byte, error) {
	ownerAddress, err := addresscodec.EncodeAccountIDToClassicAddress(ownerID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode owner address: %w", err)
	}

	destAddress, err := addresscodec.EncodeAccountIDToClassicAddress(destID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode destination address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "Escrow",
		"Account":         ownerAddress,
		"Destination":     destAddress,
		"Amount":          fmt.Sprintf("%d", amount),
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	if txn.FinishAfter != nil {
		jsonObj["FinishAfter"] = *txn.FinishAfter
	}

	if txn.CancelAfter != nil {
		jsonObj["CancelAfter"] = *txn.CancelAfter
	}

	if txn.Condition != "" {
		jsonObj["Condition"] = txn.Condition
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode Escrow: %w", err)
	}

	return hex.DecodeString(hexStr)
}
