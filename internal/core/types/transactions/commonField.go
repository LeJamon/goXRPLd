package transactions

import "encoding/json"

type TransactionCommonField struct {
	Account            string   `json:"Account" validate:"required"`
	TransactionType    string   `json:"TransactionType" validate:"required"`
	Fee                string   `json:"Fee" validate:"required"`
	Sequence           uint32   `json:"Sequence" validate:"required"`
	AccountTxnID       string   `json:"AccountTxnID,omitempty"`
	Flags              uint32   `json:"Flags,omitempty"`
	LastLedgerSequence uint32   `json:"LastLedgerSequence,omitempty"`
	Memos              []Memo   `json:"Memos,omitempty"`
	NetworkID          uint32   `json:"NetworkID,omitempty"`
	Signers            []Signer `json:"Signers,omitempty"`
	SourceTag          uint32   `json:"SourceTag,omitempty"`
	SigningPubKey      string   `json:"SigningPubKey,omitempty"`
	TicketSequence     uint32   `json:"TicketSequence,omitempty"`
	TxnSignature       string   `json:"TxnSignature,omitempty"`
}

// TODO find a proper file struct for meme
type Memo struct {
	MemoType   string `json:"MemoType,omitempty"`
	MemoData   string `json:"MemoData,omitempty"`
	MemoFormat string `json:"MemoFormat,omitempty"`
}

// TODO find a proper file struct for signer
type Signer struct {
	Account       string `json:"Account"`
	SigningPubKey string `json:"SigningPubKey"`
	TxnSignature  string `json:"TxnSignature"`
}

// Serialize encodes the transaction into JSON format.
func (t *TransactionCommonField) Serialize() ([]byte, error) {
	return json.Marshal(t)
}

// Deserialize decodes a JSON payload into a TransactionCommon object.
func (t *TransactionCommonField) Deserialize(data []byte) error {
	return json.Unmarshal(data, t)
}
