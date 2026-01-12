package tx

import "errors"

// DepositPreauth preauthorizes an account for direct deposits.
type DepositPreauth struct {
	BaseTx

	// Authorize is the account to preauthorize (mutually exclusive with Unauthorize)
	Authorize string `json:"Authorize,omitempty"`

	// Unauthorize is the account to remove preauthorization (mutually exclusive with Authorize)
	Unauthorize string `json:"Unauthorize,omitempty"`
}

// NewDepositPreauth creates a new DepositPreauth transaction
func NewDepositPreauth(account string) *DepositPreauth {
	return &DepositPreauth{
		BaseTx: *NewBaseTx(TypeDepositPreauth, account),
	}
}

// TxType returns the transaction type
func (d *DepositPreauth) TxType() Type {
	return TypeDepositPreauth
}

// Validate validates the DepositPreauth transaction
func (d *DepositPreauth) Validate() error {
	if err := d.BaseTx.Validate(); err != nil {
		return err
	}

	// Must have exactly one of Authorize or Unauthorize
	hasAuthorize := d.Authorize != ""
	hasUnauthorize := d.Unauthorize != ""

	if !hasAuthorize && !hasUnauthorize {
		return errors.New("must specify Authorize or Unauthorize")
	}

	if hasAuthorize && hasUnauthorize {
		return errors.New("cannot specify both Authorize and Unauthorize")
	}

	// Cannot authorize/unauthorize self
	if d.Authorize == d.Account || d.Unauthorize == d.Account {
		return errors.New("cannot preauthorize self")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (d *DepositPreauth) Flatten() (map[string]any, error) {
	m := d.Common.ToMap()

	if d.Authorize != "" {
		m["Authorize"] = d.Authorize
	}
	if d.Unauthorize != "" {
		m["Unauthorize"] = d.Unauthorize
	}

	return m, nil
}

// SetAuthorize sets the account to authorize
func (d *DepositPreauth) SetAuthorize(account string) {
	d.Authorize = account
	d.Unauthorize = ""
}

// SetUnauthorize sets the account to unauthorize
func (d *DepositPreauth) SetUnauthorize(account string) {
	d.Unauthorize = account
	d.Authorize = ""
}

// AccountDelete deletes an account from the ledger.
type AccountDelete struct {
	BaseTx

	// Destination is the account to receive remaining XRP (required)
	Destination string `json:"Destination"`

	// DestinationTag is an arbitrary tag for the destination (optional)
	DestinationTag *uint32 `json:"DestinationTag,omitempty"`
}

// NewAccountDelete creates a new AccountDelete transaction
func NewAccountDelete(account, destination string) *AccountDelete {
	return &AccountDelete{
		BaseTx:      *NewBaseTx(TypeAccountDelete, account),
		Destination: destination,
	}
}

// TxType returns the transaction type
func (a *AccountDelete) TxType() Type {
	return TypeAccountDelete
}

// Validate validates the AccountDelete transaction
func (a *AccountDelete) Validate() error {
	if err := a.BaseTx.Validate(); err != nil {
		return err
	}

	if a.Destination == "" {
		return errors.New("Destination is required")
	}

	// Cannot delete to self
	if a.Account == a.Destination {
		return errors.New("cannot delete account to self")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (a *AccountDelete) Flatten() (map[string]any, error) {
	m := a.Common.ToMap()

	m["Destination"] = a.Destination

	if a.DestinationTag != nil {
		m["DestinationTag"] = *a.DestinationTag
	}

	return m, nil
}

// TicketCreate creates tickets for future transactions.
type TicketCreate struct {
	BaseTx

	// TicketCount is the number of tickets to create (required, 1-250)
	TicketCount uint32 `json:"TicketCount"`
}

// NewTicketCreate creates a new TicketCreate transaction
func NewTicketCreate(account string, count uint32) *TicketCreate {
	return &TicketCreate{
		BaseTx:      *NewBaseTx(TypeTicketCreate, account),
		TicketCount: count,
	}
}

// TxType returns the transaction type
func (t *TicketCreate) TxType() Type {
	return TypeTicketCreate
}

// Validate validates the TicketCreate transaction
func (t *TicketCreate) Validate() error {
	if err := t.BaseTx.Validate(); err != nil {
		return err
	}

	if t.TicketCount == 0 || t.TicketCount > 250 {
		return errors.New("TicketCount must be 1-250")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (t *TicketCreate) Flatten() (map[string]any, error) {
	m := t.Common.ToMap()
	m["TicketCount"] = t.TicketCount
	return m, nil
}

// Clawback claws back tokens from a trust line.
type Clawback struct {
	BaseTx

	// Amount is the amount to claw back (required)
	Amount Amount `json:"Amount"`
}

// NewClawback creates a new Clawback transaction
func NewClawback(account string, amount Amount) *Clawback {
	return &Clawback{
		BaseTx: *NewBaseTx(TypeClawback, account),
		Amount: amount,
	}
}

// TxType returns the transaction type
func (c *Clawback) TxType() Type {
	return TypeClawback
}

// Validate validates the Clawback transaction
func (c *Clawback) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	if c.Amount.Value == "" {
		return errors.New("Amount is required")
	}

	// Cannot claw back XRP
	if c.Amount.IsNative() {
		return errors.New("cannot claw back XRP")
	}

	// Must be issuer of the currency
	if c.Amount.Issuer != c.Account {
		return errors.New("can only claw back own issued currency")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (c *Clawback) Flatten() (map[string]any, error) {
	m := c.Common.ToMap()
	m["Amount"] = flattenAmount(c.Amount)
	return m, nil
}
