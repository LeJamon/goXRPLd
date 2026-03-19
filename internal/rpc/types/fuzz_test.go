package types

import (
	"encoding/json"
	"testing"
)

func FuzzLedgerIndexUnmarshalJSON(f *testing.F) {
	// Seed corpus: named ledger strings
	f.Add([]byte(`"validated"`))
	f.Add([]byte(`"current"`))
	f.Add([]byte(`"closed"`))

	// Numeric values
	f.Add([]byte(`12345`))
	f.Add([]byte(`"12345"`))
	f.Add([]byte(`0`))
	f.Add([]byte(`-1`))
	f.Add([]byte(`99999999999999999999`)) // overflow

	// Non-standard JSON types
	f.Add([]byte(`null`))
	f.Add([]byte(`true`))
	f.Add([]byte(`""`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`3.14`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var li LedgerIndex
		err := json.Unmarshal(data, &li)
		if err == nil {
			// Verify String() does not panic.
			_ = li.String()
		}
		// Whether err is nil or not, we must not have panicked.
	})
}
