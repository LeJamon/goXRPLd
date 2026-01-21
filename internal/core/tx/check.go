package tx

import "errors"

// CheckCreate creates a Check that can be cashed by the destination.
type CheckCreate struct {
	BaseTx

	// Destination is the account that can cash the check (required)
	Destination string `json:"Destination"`

	// SendMax is the maximum amount that can be debited from the sender (required)
	SendMax Amount `json:"SendMax"`

	// DestinationTag is an arbitrary tag for the destination (optional)
	DestinationTag *uint32 `json:"DestinationTag,omitempty"`

	// Expiration is the time when the check expires (optional)
	Expiration *uint32 `json:"Expiration,omitempty"`

	// InvoiceID is a 256-bit hash for identifying this check (optional)
	InvoiceID string `json:"InvoiceID,omitempty"`
}

// NewCheckCreate creates a new CheckCreate transaction
func NewCheckCreate(account, destination string, sendMax Amount) *CheckCreate {
	return &CheckCreate{
		BaseTx:      *NewBaseTx(TypeCheckCreate, account),
		Destination: destination,
		SendMax:     sendMax,
	}
}

// TxType returns the transaction type
func (c *CheckCreate) TxType() Type {
	return TypeCheckCreate
}

// Validate validates the CheckCreate transaction
func (c *CheckCreate) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	if c.Destination == "" {
		return errors.New("Destination is required")
	}

	if c.SendMax.Value == "" {
		return errors.New("SendMax is required")
	}

	// Cannot create check to self
	if c.Account == c.Destination {
		return errors.New("cannot create check to self")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (c *CheckCreate) Flatten() (map[string]any, error) {
	m := c.Common.ToMap()

	m["Destination"] = c.Destination
	m["SendMax"] = flattenAmount(c.SendMax)

	if c.DestinationTag != nil {
		m["DestinationTag"] = *c.DestinationTag
	}
	if c.Expiration != nil {
		m["Expiration"] = *c.Expiration
	}
	if c.InvoiceID != "" {
		m["InvoiceID"] = c.InvoiceID
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CheckCreate) RequiredAmendments() []string {
	return []string{AmendmentChecks}
}

// CheckCash cashes a Check, drawing from the sender's balance.
type CheckCash struct {
	BaseTx

	// CheckID is the ID of the check to cash (required)
	CheckID string `json:"CheckID"`

	// Amount is the exact amount to receive (optional, mutually exclusive with DeliverMin)
	Amount *Amount `json:"Amount,omitempty"`

	// DeliverMin is the minimum amount to receive (optional, mutually exclusive with Amount)
	DeliverMin *Amount `json:"DeliverMin,omitempty"`
}

// NewCheckCash creates a new CheckCash transaction
func NewCheckCash(account, checkID string) *CheckCash {
	return &CheckCash{
		BaseTx:  *NewBaseTx(TypeCheckCash, account),
		CheckID: checkID,
	}
}

// TxType returns the transaction type
func (c *CheckCash) TxType() Type {
	return TypeCheckCash
}

// Validate validates the CheckCash transaction
func (c *CheckCash) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	if c.CheckID == "" {
		return errors.New("CheckID is required")
	}

	// Must have exactly one of Amount or DeliverMin
	hasAmount := c.Amount != nil
	hasDeliverMin := c.DeliverMin != nil

	if !hasAmount && !hasDeliverMin {
		return errors.New("must specify Amount or DeliverMin")
	}

	if hasAmount && hasDeliverMin {
		return errors.New("cannot specify both Amount and DeliverMin")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (c *CheckCash) Flatten() (map[string]any, error) {
	m := c.Common.ToMap()

	m["CheckID"] = c.CheckID

	if c.Amount != nil {
		m["Amount"] = flattenAmount(*c.Amount)
	}
	if c.DeliverMin != nil {
		m["DeliverMin"] = flattenAmount(*c.DeliverMin)
	}

	return m, nil
}

// SetExactAmount sets the exact amount to receive
func (c *CheckCash) SetExactAmount(amount Amount) {
	c.Amount = &amount
	c.DeliverMin = nil
}

// SetDeliverMin sets the minimum amount to receive
func (c *CheckCash) SetDeliverMin(amount Amount) {
	c.DeliverMin = &amount
	c.Amount = nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CheckCash) RequiredAmendments() []string {
	return []string{AmendmentChecks}
}

// CheckCancel cancels a Check.
type CheckCancel struct {
	BaseTx

	// CheckID is the ID of the check to cancel (required)
	CheckID string `json:"CheckID"`
}

// NewCheckCancel creates a new CheckCancel transaction
func NewCheckCancel(account, checkID string) *CheckCancel {
	return &CheckCancel{
		BaseTx:  *NewBaseTx(TypeCheckCancel, account),
		CheckID: checkID,
	}
}

// TxType returns the transaction type
func (c *CheckCancel) TxType() Type {
	return TypeCheckCancel
}

// Validate validates the CheckCancel transaction
func (c *CheckCancel) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	if c.CheckID == "" {
		return errors.New("CheckID is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (c *CheckCancel) Flatten() (map[string]any, error) {
	m := c.Common.ToMap()
	m["CheckID"] = c.CheckID
	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CheckCancel) RequiredAmendments() []string {
	return []string{AmendmentChecks}
}
