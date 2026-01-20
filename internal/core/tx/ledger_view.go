package tx

import (
	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// LedgerView provides read/write access to ledger state.
// This interface abstracts the ledger storage layer, allowing the transaction
// engine to work with different storage implementations.
type LedgerView interface {
	// Read reads a ledger entry by its keylet.
	// Returns the serialized entry data or an error if not found.
	Read(k keylet.Keylet) ([]byte, error)

	// Exists checks if an entry exists in the ledger.
	Exists(k keylet.Keylet) (bool, error)

	// Insert adds a new entry to the ledger.
	// Returns an error if the entry already exists.
	Insert(k keylet.Keylet, data []byte) error

	// Update modifies an existing entry in the ledger.
	// Returns an error if the entry does not exist.
	Update(k keylet.Keylet, data []byte) error

	// Erase removes an entry from the ledger.
	// Returns an error if the entry does not exist.
	Erase(k keylet.Keylet) error

	// AdjustDropsDestroyed records XRP destroyed (typically from fees).
	// This is used to track the total XRP supply.
	AdjustDropsDestroyed(drops XRPAmount.XRPAmount)

	// ForEach iterates over all state entries in the ledger.
	// If fn returns false, iteration stops early.
	// This is useful for scanning the entire ledger state.
	ForEach(fn func(key [32]byte, data []byte) bool) error
}

// ReadOnlyLedgerView provides read-only access to ledger state.
// This is useful for validation operations that should not modify state.
type ReadOnlyLedgerView interface {
	// Read reads a ledger entry by its keylet.
	Read(k keylet.Keylet) ([]byte, error)

	// Exists checks if an entry exists in the ledger.
	Exists(k keylet.Keylet) (bool, error)

	// ForEach iterates over all state entries.
	ForEach(fn func(key [32]byte, data []byte) bool) error
}

// LedgerViewAdapter wraps a LedgerView to provide a ReadOnlyLedgerView
type LedgerViewAdapter struct {
	view LedgerView
}

// NewReadOnlyView creates a read-only adapter for a LedgerView
func NewReadOnlyView(view LedgerView) ReadOnlyLedgerView {
	return &LedgerViewAdapter{view: view}
}

// Read implements ReadOnlyLedgerView
func (a *LedgerViewAdapter) Read(k keylet.Keylet) ([]byte, error) {
	return a.view.Read(k)
}

// Exists implements ReadOnlyLedgerView
func (a *LedgerViewAdapter) Exists(k keylet.Keylet) (bool, error) {
	return a.view.Exists(k)
}

// ForEach implements ReadOnlyLedgerView
func (a *LedgerViewAdapter) ForEach(fn func(key [32]byte, data []byte) bool) error {
	return a.view.ForEach(fn)
}
