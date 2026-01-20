package tx

// DIDSet creates or updates a DID document.
type DIDSet struct {
	BaseTx

	// Data is the public attestations (optional)
	Data string `json:"Data,omitempty"`

	// DIDDocument is the DID document content (optional)
	DIDDocument string `json:"DIDDocument,omitempty"`

	// URI is the URI for the DID document (optional)
	URI string `json:"URI,omitempty"`
}

// NewDIDSet creates a new DIDSet transaction
func NewDIDSet(account string) *DIDSet {
	return &DIDSet{
		BaseTx: *NewBaseTx(TypeDIDSet, account),
	}
}

// TxType returns the transaction type
func (d *DIDSet) TxType() Type {
	return TypeDIDSet
}

// Validate validates the DIDSet transaction
func (d *DIDSet) Validate() error {
	return d.BaseTx.Validate()
}

// Flatten returns a flat map of all transaction fields
func (d *DIDSet) Flatten() (map[string]any, error) {
	m := d.Common.ToMap()

	if d.Data != "" {
		m["Data"] = d.Data
	}
	if d.DIDDocument != "" {
		m["DIDDocument"] = d.DIDDocument
	}
	if d.URI != "" {
		m["URI"] = d.URI
	}

	return m, nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (d *DIDSet) RequiredAmendments() []string {
	return []string{AmendmentDID}
}

// DIDDelete deletes a DID document.
type DIDDelete struct {
	BaseTx
}

// NewDIDDelete creates a new DIDDelete transaction
func NewDIDDelete(account string) *DIDDelete {
	return &DIDDelete{
		BaseTx: *NewBaseTx(TypeDIDDelete, account),
	}
}

// TxType returns the transaction type
func (d *DIDDelete) TxType() Type {
	return TypeDIDDelete
}

// Validate validates the DIDDelete transaction
func (d *DIDDelete) Validate() error {
	return d.BaseTx.Validate()
}

// Flatten returns a flat map of all transaction fields
func (d *DIDDelete) Flatten() (map[string]any, error) {
	return d.Common.ToMap(), nil
}

// RequiredAmendments returns the amendments required for this transaction type
func (d *DIDDelete) RequiredAmendments() []string {
	return []string{AmendmentDID}
}
