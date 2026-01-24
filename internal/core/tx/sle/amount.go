package sle

import "encoding/json"

// Amount represents either XRP (as drops string) or an issued currency amount
type Amount struct {
	// For XRP amounts, only Value is set (as drops string)
	Value string `json:"value,omitempty"`

	// For issued currency amounts
	Currency string `json:"currency,omitempty"`
	Issuer   string `json:"issuer,omitempty"`

	// Native indicates if this is XRP (true) or issued currency (false)
	Native bool
}

// NewXRPAmount creates an XRP amount in drops
func NewXRPAmount(drops string) Amount {
	return Amount{Value: drops, Native: true}
}

// NewIssuedAmount creates an issued currency amount
func NewIssuedAmount(value, currency, issuer string) Amount {
	return Amount{
		Value:    value,
		Currency: currency,
		Issuer:   issuer,
		Native:   false,
	}
}

// IsNative returns true if this is an XRP amount
func (a Amount) IsNative() bool {
	return a.Native || (a.Currency == "" && a.Issuer == "")
}

// MarshalJSON implements custom JSON marshaling
func (a Amount) MarshalJSON() ([]byte, error) {
	if a.IsNative() {
		return json.Marshal(a.Value)
	}
	return json.Marshal(map[string]string{
		"value":    a.Value,
		"currency": a.Currency,
		"issuer":   a.Issuer,
	})
}

// UnmarshalJSON implements custom JSON unmarshaling
func (a *Amount) UnmarshalJSON(data []byte) error {
	// Try as string first (XRP drops)
	var strVal string
	if err := json.Unmarshal(data, &strVal); err == nil {
		a.Value = strVal
		a.Native = true
		return nil
	}

	// Try as object (issued currency)
	var objVal struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
		Issuer   string `json:"issuer"`
	}
	if err := json.Unmarshal(data, &objVal); err != nil {
		return err
	}

	a.Value = objVal.Value
	a.Currency = objVal.Currency
	a.Issuer = objVal.Issuer
	a.Native = false
	return nil
}

// flattenAmount converts an Amount to its JSON-compatible representation
func flattenAmount(a Amount) any {
	if a.IsNative() {
		return a.Value
	}
	return map[string]string{
		"value":    a.Value,
		"currency": a.Currency,
		"issuer":   a.Issuer,
	}
}
