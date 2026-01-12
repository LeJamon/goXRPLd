package tx

import "errors"

// EscrowCreate creates an escrow that holds XRP until certain conditions are met.
type EscrowCreate struct {
	BaseTx

	// Amount is the amount of XRP to escrow (required)
	Amount Amount `json:"Amount"`

	// Destination is the account to receive the XRP (required)
	Destination string `json:"Destination"`

	// DestinationTag is an arbitrary tag for the destination (optional)
	DestinationTag *uint32 `json:"DestinationTag,omitempty"`

	// CancelAfter is the time after which the escrow can be cancelled (optional)
	CancelAfter *uint32 `json:"CancelAfter,omitempty"`

	// FinishAfter is the time after which the escrow can be finished (optional)
	FinishAfter *uint32 `json:"FinishAfter,omitempty"`

	// Condition is the crypto-condition that must be fulfilled (optional)
	Condition string `json:"Condition,omitempty"`
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
func (e *EscrowCreate) Validate() error {
	if err := e.BaseTx.Validate(); err != nil {
		return err
	}

	if e.Destination == "" {
		return errors.New("Destination is required")
	}

	if e.Amount.Value == "" {
		return errors.New("Amount is required")
	}

	// Must be XRP
	if !e.Amount.IsNative() {
		return errors.New("escrow can only hold XRP")
	}

	// Must have either CancelAfter, FinishAfter, or Condition
	if e.CancelAfter == nil && e.FinishAfter == nil && e.Condition == "" {
		return errors.New("must specify CancelAfter, FinishAfter, or Condition")
	}

	// If both times are specified, CancelAfter must be after FinishAfter
	if e.CancelAfter != nil && e.FinishAfter != nil {
		if *e.CancelAfter <= *e.FinishAfter {
			return errors.New("CancelAfter must be after FinishAfter")
		}
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (e *EscrowCreate) Flatten() (map[string]any, error) {
	m := e.Common.ToMap()

	m["Amount"] = e.Amount.Value // XRP only, so just the drops string
	m["Destination"] = e.Destination

	if e.DestinationTag != nil {
		m["DestinationTag"] = *e.DestinationTag
	}
	if e.CancelAfter != nil {
		m["CancelAfter"] = *e.CancelAfter
	}
	if e.FinishAfter != nil {
		m["FinishAfter"] = *e.FinishAfter
	}
	if e.Condition != "" {
		m["Condition"] = e.Condition
	}

	return m, nil
}

// EscrowFinish completes an escrow, releasing the escrowed XRP.
type EscrowFinish struct {
	BaseTx

	// Owner is the account that created the escrow (required)
	Owner string `json:"Owner"`

	// OfferSequence is the sequence number of the EscrowCreate (required)
	OfferSequence uint32 `json:"OfferSequence"`

	// Condition is the crypto-condition that was fulfilled (optional)
	Condition string `json:"Condition,omitempty"`

	// Fulfillment is the fulfillment for the condition (optional)
	Fulfillment string `json:"Fulfillment,omitempty"`
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
func (e *EscrowFinish) Validate() error {
	if err := e.BaseTx.Validate(); err != nil {
		return err
	}

	if e.Owner == "" {
		return errors.New("Owner is required")
	}

	// Both Condition and Fulfillment must be present or absent together
	hasCondition := e.Condition != ""
	hasFulfillment := e.Fulfillment != ""
	if hasCondition != hasFulfillment {
		return errors.New("Condition and Fulfillment must be provided together")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (e *EscrowFinish) Flatten() (map[string]any, error) {
	m := e.Common.ToMap()

	m["Owner"] = e.Owner
	m["OfferSequence"] = e.OfferSequence

	if e.Condition != "" {
		m["Condition"] = e.Condition
	}
	if e.Fulfillment != "" {
		m["Fulfillment"] = e.Fulfillment
	}

	return m, nil
}

// EscrowCancel cancels an escrow, returning the escrowed XRP to the creator.
type EscrowCancel struct {
	BaseTx

	// Owner is the account that created the escrow (required)
	Owner string `json:"Owner"`

	// OfferSequence is the sequence number of the EscrowCreate (required)
	OfferSequence uint32 `json:"OfferSequence"`
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
func (e *EscrowCancel) Validate() error {
	if err := e.BaseTx.Validate(); err != nil {
		return err
	}

	if e.Owner == "" {
		return errors.New("Owner is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (e *EscrowCancel) Flatten() (map[string]any, error) {
	m := e.Common.ToMap()

	m["Owner"] = e.Owner
	m["OfferSequence"] = e.OfferSequence

	return m, nil
}
