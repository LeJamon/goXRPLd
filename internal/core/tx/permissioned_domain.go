package tx

import "errors"

// PermissionedDomainSet creates or modifies a permissioned domain.
type PermissionedDomainSet struct {
	BaseTx

	// DomainID is the ID of the domain (optional, omit for creation)
	DomainID string `json:"DomainID,omitempty"`

	// AcceptedCredentials defines the credentials accepted by this domain (optional)
	AcceptedCredentials []AcceptedCredential `json:"AcceptedCredentials,omitempty"`
}

// AcceptedCredential defines an accepted credential type
type AcceptedCredential struct {
	AcceptedCredential AcceptedCredentialData `json:"AcceptedCredential"`
}

// AcceptedCredentialData contains the credential data
type AcceptedCredentialData struct {
	Issuer         string `json:"Issuer"`
	CredentialType string `json:"CredentialType"`
}

// NewPermissionedDomainSet creates a new PermissionedDomainSet transaction
func NewPermissionedDomainSet(account string) *PermissionedDomainSet {
	return &PermissionedDomainSet{
		BaseTx: *NewBaseTx(TypePermissionedDomainSet, account),
	}
}

// TxType returns the transaction type
func (p *PermissionedDomainSet) TxType() Type {
	return TypePermissionedDomainSet
}

// Validate validates the PermissionedDomainSet transaction
func (p *PermissionedDomainSet) Validate() error {
	return p.BaseTx.Validate()
}

// Flatten returns a flat map of all transaction fields
func (p *PermissionedDomainSet) Flatten() (map[string]any, error) {
	m := p.Common.ToMap()

	if p.DomainID != "" {
		m["DomainID"] = p.DomainID
	}
	if len(p.AcceptedCredentials) > 0 {
		m["AcceptedCredentials"] = p.AcceptedCredentials
	}

	return m, nil
}

// AddAcceptedCredential adds an accepted credential
func (p *PermissionedDomainSet) AddAcceptedCredential(issuer, credentialType string) {
	p.AcceptedCredentials = append(p.AcceptedCredentials, AcceptedCredential{
		AcceptedCredential: AcceptedCredentialData{
			Issuer:         issuer,
			CredentialType: credentialType,
		},
	})
}

// RequiredAmendments returns the amendments required for this transaction type
func (p *PermissionedDomainSet) RequiredAmendments() []string {
	return []string{AmendmentPermissionedDomains, AmendmentCredentials}
}

// PermissionedDomainDelete deletes a permissioned domain.
type PermissionedDomainDelete struct {
	BaseTx

	// DomainID is the ID of the domain to delete (required)
	DomainID string `json:"DomainID"`
}

// NewPermissionedDomainDelete creates a new PermissionedDomainDelete transaction
func NewPermissionedDomainDelete(account, domainID string) *PermissionedDomainDelete {
	return &PermissionedDomainDelete{
		BaseTx:   *NewBaseTx(TypePermissionedDomainDelete, account),
		DomainID: domainID,
	}
}

// TxType returns the transaction type
func (p *PermissionedDomainDelete) TxType() Type {
	return TypePermissionedDomainDelete
}

// Validate validates the PermissionedDomainDelete transaction
func (p *PermissionedDomainDelete) Validate() error {
	if err := p.BaseTx.Validate(); err != nil {
		return err
	}

	if p.DomainID == "" {
		return errors.New("DomainID is required")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (p *PermissionedDomainDelete) Flatten() (map[string]any, error) {
	m := p.Common.ToMap()
	m["DomainID"] = p.DomainID
	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (p *PermissionedDomainDelete) RequiredAmendments() []string {
	return []string{AmendmentPermissionedDomains, AmendmentCredentials}
}
