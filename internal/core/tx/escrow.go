package tx

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

func init() {
	Register(TypeEscrowCreate, func() Transaction {
		return &EscrowCreate{BaseTx: *NewBaseTx(TypeEscrowCreate, "")}
	})
	Register(TypeEscrowFinish, func() Transaction {
		return &EscrowFinish{BaseTx: *NewBaseTx(TypeEscrowFinish, "")}
	})
	Register(TypeEscrowCancel, func() Transaction {
		return &EscrowCancel{BaseTx: *NewBaseTx(TypeEscrowCancel, "")}
	})
}

// EscrowCreate creates an escrow that holds XRP until certain conditions are met.
type EscrowCreate struct {
	BaseTx

	// Amount is the amount of XRP to escrow (required)
	Amount Amount `json:"Amount" xrpl:"Amount,amount"`

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
func NewEscrowCreate(account, destination string, amount Amount) *EscrowCreate {
	return &EscrowCreate{
		BaseTx:      *NewBaseTx(TypeEscrowCreate, account),
		Amount:      amount,
		Destination: destination,
	}
}

// TxType returns the transaction type
func (e *EscrowCreate) TxType() Type {
	return TypeEscrowCreate
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
	return ReflectFlatten(e)
}

// EscrowFinish completes an escrow, releasing the escrowed XRP.
type EscrowFinish struct {
	BaseTx

	// Owner is the account that created the escrow (required)
	Owner string `json:"Owner" xrpl:"Owner"`

	// OfferSequence is the sequence number of the EscrowCreate (required)
	OfferSequence uint32 `json:"OfferSequence" xrpl:"OfferSequence"`

	// Condition is the crypto-condition that was fulfilled (optional)
	Condition string `json:"Condition,omitempty" xrpl:"Condition,omitempty"`

	// Fulfillment is the fulfillment for the condition (optional)
	Fulfillment string `json:"Fulfillment,omitempty" xrpl:"Fulfillment,omitempty"`
}

// NewEscrowFinish creates a new EscrowFinish transaction
func NewEscrowFinish(account, owner string, offerSequence uint32) *EscrowFinish {
	return &EscrowFinish{
		BaseTx:        *NewBaseTx(TypeEscrowFinish, account),
		Owner:         owner,
		OfferSequence: offerSequence,
	}
}

// TxType returns the transaction type
func (e *EscrowFinish) TxType() Type {
	return TypeEscrowFinish
}

// Validate validates the EscrowFinish transaction
// Reference: rippled Escrow.cpp EscrowFinish::preflight()
func (e *EscrowFinish) Validate() error {
	if err := e.BaseTx.Validate(); err != nil {
		return err
	}

	if e.Owner == "" {
		return errors.New("temMALFORMED: Owner is required")
	}

	// Both Condition and Fulfillment must be present or absent together
	// Reference: rippled Escrow.cpp:644-646
	hasCondition := e.Condition != ""
	hasFulfillment := e.Fulfillment != ""
	if hasCondition != hasFulfillment {
		return errors.New("temMALFORMED: Condition and Fulfillment must be provided together")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (e *EscrowFinish) Flatten() (map[string]any, error) {
	return ReflectFlatten(e)
}

// EscrowCancel cancels an escrow, returning the escrowed XRP to the creator.
type EscrowCancel struct {
	BaseTx

	// Owner is the account that created the escrow (required)
	Owner string `json:"Owner" xrpl:"Owner"`

	// OfferSequence is the sequence number of the EscrowCreate (required)
	OfferSequence uint32 `json:"OfferSequence" xrpl:"OfferSequence"`
}

// NewEscrowCancel creates a new EscrowCancel transaction
func NewEscrowCancel(account, owner string, offerSequence uint32) *EscrowCancel {
	return &EscrowCancel{
		BaseTx:        *NewBaseTx(TypeEscrowCancel, account),
		Owner:         owner,
		OfferSequence: offerSequence,
	}
}

// TxType returns the transaction type
func (e *EscrowCancel) TxType() Type {
	return TypeEscrowCancel
}

// Validate validates the EscrowCancel transaction
// Reference: rippled Escrow.cpp EscrowCancel::preflight()
func (e *EscrowCancel) Validate() error {
	if err := e.BaseTx.Validate(); err != nil {
		return err
	}

	if e.Owner == "" {
		return errors.New("temMALFORMED: Owner is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (e *EscrowCancel) Flatten() (map[string]any, error) {
	return ReflectFlatten(e)
}

// Apply applies an EscrowCreate transaction
func (ec *EscrowCreate) Apply(ctx *ApplyContext) Result {
	// Parse the amount to escrow
	amount, err := strconv.ParseUint(ec.Amount.Value, 10, 64)
	if err != nil {
		return TemINVALID
	}

	// Check that account has sufficient balance (after fee)
	if ctx.Account.Balance < amount {
		return TecUNFUNDED
	}

	// Verify destination exists
	destID, err := decodeAccountID(ec.Destination)
	if err != nil {
		return TemINVALID
	}

	destKey := keylet.Account(destID)
	exists, _ := ctx.View.Exists(destKey)
	if !exists {
		return TecNO_DST
	}

	// Deduct the escrow amount from the account
	ctx.Account.Balance -= amount

	// Create the escrow entry
	accountID, _ := decodeAccountID(ec.Account)
	sequence := *ec.GetCommon().Sequence // Use the transaction sequence

	escrowKey := keylet.Escrow(accountID, sequence)

	// Serialize escrow
	escrowData, err := serializeEscrow(ec, accountID, destID, sequence, amount)
	if err != nil {
		return TefINTERNAL
	}

	// Insert escrow - creation tracked automatically by ApplyStateTable
	if err := ctx.View.Insert(escrowKey, escrowData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count
	ctx.Account.OwnerCount++

	return TesSUCCESS
}

// Apply applies an EscrowFinish transaction
func (ef *EscrowFinish) Apply(ctx *ApplyContext) Result {
	// Get the escrow owner's account ID
	ownerID, err := decodeAccountID(ef.Owner)
	if err != nil {
		return TemINVALID
	}

	// Find the escrow
	escrowKey := keylet.Escrow(ownerID, ef.OfferSequence)
	escrowData, err := ctx.View.Read(escrowKey)
	if err != nil {
		return TecNO_TARGET
	}

	// Parse escrow
	escrow, err := parseEscrow(escrowData)
	if err != nil {
		return TefINTERNAL
	}

	// Check FinishAfter time (if set)
	if escrow.FinishAfter > 0 {
		// In a full implementation, we'd check against the close time
		// For now, we'll allow it in standalone mode
		if !ctx.Config.Standalone {
			// Would check: if currentTime < escrow.FinishAfter return TecNO_PERMISSION
		}
	}

	// Check condition/fulfillment with proper crypto-condition verification
	// Reference: rippled Escrow.cpp preclaim() and checkCondition()
	if escrow.Condition != "" {
		// If escrow has a condition, fulfillment must be provided
		if ef.Fulfillment == "" {
			return TecCRYPTOCONDITION_ERROR
		}

		// Verify the fulfillment matches the condition
		// The escrow stores condition as hex, tx provides fulfillment as hex
		if err := validateCryptoCondition(ef.Fulfillment, escrow.Condition); err != nil {
			return TecCRYPTOCONDITION_ERROR
		}
	}

	// Get destination account
	destKey := keylet.Account(escrow.DestinationID)
	destData, err := ctx.View.Read(destKey)
	if err != nil {
		return TecNO_DST
	}

	destAccount, err := parseAccountRoot(destData)
	if err != nil {
		return TefINTERNAL
	}

	// Transfer the escrowed amount to destination
	destAccount.Balance += escrow.Amount

	// Update destination - modification tracked automatically by ApplyStateTable
	destUpdatedData, err := serializeAccountRoot(destAccount)
	if err != nil {
		return TefINTERNAL
	}

	if err := ctx.View.Update(destKey, destUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Delete the escrow - deletion tracked automatically by ApplyStateTable
	if err := ctx.View.Erase(escrowKey); err != nil {
		return TefINTERNAL
	}

	// Decrease owner count for escrow owner
	if ef.Owner != ef.Account {
		// Need to update owner's account too
		ownerKey := keylet.Account(ownerID)
		ownerData, err := ctx.View.Read(ownerKey)
		if err == nil {
			ownerAccount, err := parseAccountRoot(ownerData)
			if err == nil && ownerAccount.OwnerCount > 0 {
				ownerAccount.OwnerCount--
				ownerUpdatedData, err := serializeAccountRoot(ownerAccount)
				if err == nil {
					ctx.View.Update(ownerKey, ownerUpdatedData)
				}
			}
		}
	} else {
		if ctx.Account.OwnerCount > 0 {
			ctx.Account.OwnerCount--
		}
	}

	return TesSUCCESS
}

// Apply applies an EscrowCancel transaction
func (ec *EscrowCancel) Apply(ctx *ApplyContext) Result {
	// Get the escrow owner's account ID
	ownerID, err := decodeAccountID(ec.Owner)
	if err != nil {
		return TemINVALID
	}

	// Find the escrow
	escrowKey := keylet.Escrow(ownerID, ec.OfferSequence)
	escrowData, err := ctx.View.Read(escrowKey)
	if err != nil {
		return TecNO_TARGET
	}

	// Parse escrow
	escrow, err := parseEscrow(escrowData)
	if err != nil {
		return TefINTERNAL
	}

	// Check CancelAfter time (if set)
	if escrow.CancelAfter > 0 {
		// In a full implementation, we'd check against the close time
		// For now, we'll allow it in standalone mode
		if !ctx.Config.Standalone {
			// Would check: if currentTime < escrow.CancelAfter return TecNO_PERMISSION
		}
	} else {
		// If no CancelAfter, only the creator can cancel (implied by having condition)
		if ec.Account != ec.Owner {
			return TecNO_PERMISSION
		}
	}

	// Return the escrowed amount to the owner
	ownerKey := keylet.Account(ownerID)
	ownerData, err := ctx.View.Read(ownerKey)
	if err != nil {
		return TefINTERNAL
	}

	ownerAccount, err := parseAccountRoot(ownerData)
	if err != nil {
		return TefINTERNAL
	}

	ownerAccount.Balance += escrow.Amount
	if ownerAccount.OwnerCount > 0 {
		ownerAccount.OwnerCount--
	}

	ownerUpdatedData, err := serializeAccountRoot(ownerAccount)
	if err != nil {
		return TefINTERNAL
	}

	// Update owner - modification tracked automatically by ApplyStateTable
	if err := ctx.View.Update(ownerKey, ownerUpdatedData); err != nil {
		return TefINTERNAL
	}

	// Delete the escrow - deletion tracked automatically by ApplyStateTable
	if err := ctx.View.Erase(escrowKey); err != nil {
		return TefINTERNAL
	}

	return TesSUCCESS
}

// EscrowData represents an Escrow ledger entry
type EscrowData struct {
	Account       [20]byte
	DestinationID [20]byte
	Amount        uint64
	Condition     string
	CancelAfter   uint32
	FinishAfter   uint32
}

// serializeEscrow serializes an Escrow ledger entry
func serializeEscrow(tx *EscrowCreate, ownerID, destID [20]byte, sequence uint32, amount uint64) ([]byte, error) {
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

	if tx.FinishAfter != nil {
		jsonObj["FinishAfter"] = *tx.FinishAfter
	}

	if tx.CancelAfter != nil {
		jsonObj["CancelAfter"] = *tx.CancelAfter
	}

	if tx.Condition != "" {
		jsonObj["Condition"] = tx.Condition
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode Escrow: %w", err)
	}

	return hex.DecodeString(hexStr)
}

// parseEscrow parses an Escrow ledger entry from binary data
func parseEscrow(data []byte) (*EscrowData, error) {
	escrow := &EscrowData{}
	offset := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		header := data[offset]
		offset++

		typeCode := (header >> 4) & 0x0F
		fieldCode := header & 0x0F

		if typeCode == 0 {
			if offset >= len(data) {
				break
			}
			typeCode = data[offset]
			offset++
		}

		if fieldCode == 0 {
			if offset >= len(data) {
				break
			}
			fieldCode = data[offset]
			offset++
		}

		switch typeCode {
		case fieldTypeUInt16:
			if offset+2 > len(data) {
				return escrow, nil
			}
			offset += 2

		case fieldTypeUInt32:
			if offset+4 > len(data) {
				return escrow, nil
			}
			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4
			switch fieldCode {
			case 36: // FinishAfter
				escrow.FinishAfter = value
			case 37: // CancelAfter
				escrow.CancelAfter = value
			}

		case fieldTypeUInt64:
			if offset+8 > len(data) {
				return escrow, nil
			}
			offset += 8

		case fieldTypeAmount:
			if offset+8 > len(data) {
				return escrow, nil
			}
			rawAmount := binary.BigEndian.Uint64(data[offset : offset+8])
			escrow.Amount = rawAmount & 0x3FFFFFFFFFFFFFFF
			offset += 8

		case fieldTypeAccountID:
			if offset+21 > len(data) {
				return escrow, nil
			}
			length := data[offset]
			offset++
			if length == 20 {
				switch fieldCode {
				case 1: // Account
					copy(escrow.Account[:], data[offset:offset+20])
				case 3: // Destination
					copy(escrow.DestinationID[:], data[offset:offset+20])
				}
				offset += 20
			}

		case fieldTypeHash256:
			// Hash256 fields are 32 bytes (e.g., PreviousTxnID)
			if offset+32 > len(data) {
				return escrow, nil
			}
			offset += 32

		case fieldTypeBlob:
			if offset >= len(data) {
				return escrow, nil
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return escrow, nil
			}
			if fieldCode == 25 { // Condition
				escrow.Condition = hex.EncodeToString(data[offset : offset+length])
			}
			offset += length

		default:
			// Unknown type - try to skip safely
			return escrow, nil
		}
	}

	return escrow, nil
}
