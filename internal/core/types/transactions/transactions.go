package transactions

type Transaction struct {
	Signature string `json:"signature"`
	Payload   string `json:"tx"` // Raw transaction data
}
