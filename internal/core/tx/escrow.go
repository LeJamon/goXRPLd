package tx

import "errors"

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
