package tx

import "errors"

// CredentialCreate creates a credential.
type CredentialCreate struct {
	BaseTx

	// Subject is the account the credential is about (required)
	Subject string `json:"Subject"`

	// CredentialType is the type of credential (required, hex-encoded)
	CredentialType string `json:"CredentialType"`

	// Expiration is when the credential expires (optional)
	Expiration *uint32 `json:"Expiration,omitempty"`

	// URI is the URI for credential details (optional)
	URI string `json:"URI,omitempty"`
}

// NewCredentialCreate creates a new CredentialCreate transaction
func NewCredentialCreate(account, subject, credentialType string) *CredentialCreate {
	return &CredentialCreate{
		BaseTx:         *NewBaseTx(TypeCredentialCreate, account),
		Subject:        subject,
		CredentialType: credentialType,
	}
}

// TxType returns the transaction type
func (c *CredentialCreate) TxType() Type {
	return TypeCredentialCreate
}

// Validate validates the CredentialCreate transaction
func (c *CredentialCreate) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	if c.Subject == "" {
		return errors.New("Subject is required")
	}

	if c.CredentialType == "" {
		return errors.New("CredentialType is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (c *CredentialCreate) Flatten() (map[string]any, error) {
	m := c.Common.ToMap()

	m["Subject"] = c.Subject
	m["CredentialType"] = c.CredentialType

	if c.Expiration != nil {
		m["Expiration"] = *c.Expiration
	}
	if c.URI != "" {
		m["URI"] = c.URI
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CredentialCreate) RequiredAmendments() []string {
	return []string{AmendmentCredentials}
}

// CredentialAccept accepts a credential.
type CredentialAccept struct {
	BaseTx

	// Issuer is the account that issued the credential (required)
	Issuer string `json:"Issuer"`

	// CredentialType is the type of credential (required, hex-encoded)
	CredentialType string `json:"CredentialType"`
}

// NewCredentialAccept creates a new CredentialAccept transaction
func NewCredentialAccept(account, issuer, credentialType string) *CredentialAccept {
	return &CredentialAccept{
		BaseTx:         *NewBaseTx(TypeCredentialAccept, account),
		Issuer:         issuer,
		CredentialType: credentialType,
	}
}

// TxType returns the transaction type
func (c *CredentialAccept) TxType() Type {
	return TypeCredentialAccept
}

// Validate validates the CredentialAccept transaction
func (c *CredentialAccept) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	if c.Issuer == "" {
		return errors.New("Issuer is required")
	}

	if c.CredentialType == "" {
		return errors.New("CredentialType is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (c *CredentialAccept) Flatten() (map[string]any, error) {
	m := c.Common.ToMap()

	m["Issuer"] = c.Issuer
	m["CredentialType"] = c.CredentialType

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CredentialAccept) RequiredAmendments() []string {
	return []string{AmendmentCredentials}
}

// CredentialDelete deletes a credential.
type CredentialDelete struct {
	BaseTx

	// Subject is the account the credential is about (optional, defaults to Account)
	Subject string `json:"Subject,omitempty"`

	// Issuer is the account that issued the credential (optional, defaults to Account)
	Issuer string `json:"Issuer,omitempty"`

	// CredentialType is the type of credential (required, hex-encoded)
	CredentialType string `json:"CredentialType"`
}

// NewCredentialDelete creates a new CredentialDelete transaction
func NewCredentialDelete(account, credentialType string) *CredentialDelete {
	return &CredentialDelete{
		BaseTx:         *NewBaseTx(TypeCredentialDelete, account),
		CredentialType: credentialType,
	}
}

// TxType returns the transaction type
func (c *CredentialDelete) TxType() Type {
	return TypeCredentialDelete
}

// Validate validates the CredentialDelete transaction
func (c *CredentialDelete) Validate() error {
	if err := c.BaseTx.Validate(); err != nil {
		return err
	}

	if c.CredentialType == "" {
		return errors.New("CredentialType is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (c *CredentialDelete) Flatten() (map[string]any, error) {
	m := c.Common.ToMap()

	m["CredentialType"] = c.CredentialType

	if c.Subject != "" {
		m["Subject"] = c.Subject
	}
	if c.Issuer != "" {
		m["Issuer"] = c.Issuer
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (c *CredentialDelete) RequiredAmendments() []string {
	return []string{AmendmentCredentials}
}
