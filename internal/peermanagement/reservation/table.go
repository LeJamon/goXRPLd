// Package reservation implements peer reservation management for XRPL.
// Reserved peers have guaranteed connection slots that aren't affected
// by normal connection limits.
package reservation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const (
	// DefaultReservationFile is the default filename for reservations.
	DefaultReservationFile = "peer_reservations.json"
)

// Reservation represents a reserved peer slot.
type Reservation struct {
	// NodeID is the base58-encoded public key of the reserved node.
	NodeID string `json:"node_id"`
	// Description is an optional human-readable description.
	Description string `json:"description,omitempty"`
}

// Table manages peer reservations.
type Table struct {
	mu           sync.RWMutex
	reservations map[string]*Reservation // nodeID -> reservation
	filePath     string
	dirty        bool
}

// NewTable creates a new reservation table.
func NewTable(dataDir string) *Table {
	var filePath string
	if dataDir != "" {
		filePath = filepath.Join(dataDir, DefaultReservationFile)
	}
	return &Table{
		reservations: make(map[string]*Reservation),
		filePath:     filePath,
	}
}

// Load loads reservations from disk.
func (t *Table) Load() error {
	if t.filePath == "" {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := os.ReadFile(t.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No file yet
		}
		return err
	}

	var reservations []*Reservation
	if err := json.Unmarshal(data, &reservations); err != nil {
		return err
	}

	t.reservations = make(map[string]*Reservation)
	for _, r := range reservations {
		t.reservations[r.NodeID] = r
	}

	return nil
}

// Save writes reservations to disk.
func (t *Table) Save() error {
	if t.filePath == "" {
		return nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.dirty {
		return nil
	}

	reservations := make([]*Reservation, 0, len(t.reservations))
	for _, r := range t.reservations {
		reservations = append(reservations, r)
	}

	data, err := json.MarshalIndent(reservations, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(t.filePath), 0755); err != nil {
		return err
	}

	t.dirty = false
	return os.WriteFile(t.filePath, data, 0644)
}

// Contains returns true if the node has a reservation.
func (t *Table) Contains(nodeID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, exists := t.reservations[nodeID]
	return exists
}

// Get returns the reservation for a node, or nil if not reserved.
func (t *Table) Get(nodeID string) *Reservation {
	t.mu.RLock()
	defer t.mu.RUnlock()

	r, exists := t.reservations[nodeID]
	if !exists {
		return nil
	}
	// Return a copy
	return &Reservation{
		NodeID:      r.NodeID,
		Description: r.Description,
	}
}

// InsertOrAssign adds or updates a reservation.
// Returns the previous reservation if it existed.
func (t *Table) InsertOrAssign(reservation *Reservation) *Reservation {
	t.mu.Lock()
	defer t.mu.Unlock()

	var previous *Reservation
	if existing, exists := t.reservations[reservation.NodeID]; exists {
		previous = &Reservation{
			NodeID:      existing.NodeID,
			Description: existing.Description,
		}
	}

	t.reservations[reservation.NodeID] = &Reservation{
		NodeID:      reservation.NodeID,
		Description: reservation.Description,
	}
	t.dirty = true

	return previous
}

// Erase removes a reservation.
// Returns the removed reservation if it existed.
func (t *Table) Erase(nodeID string) *Reservation {
	t.mu.Lock()
	defer t.mu.Unlock()

	existing, exists := t.reservations[nodeID]
	if !exists {
		return nil
	}

	removed := &Reservation{
		NodeID:      existing.NodeID,
		Description: existing.Description,
	}
	delete(t.reservations, nodeID)
	t.dirty = true

	return removed
}

// List returns all reservations.
func (t *Table) List() []*Reservation {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*Reservation, 0, len(t.reservations))
	for _, r := range t.reservations {
		result = append(result, &Reservation{
			NodeID:      r.NodeID,
			Description: r.Description,
		})
	}
	return result
}

// Size returns the number of reservations.
func (t *Table) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.reservations)
}

// Clear removes all reservations.
func (t *Table) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reservations = make(map[string]*Reservation)
	t.dirty = true
}

// ToJSON returns the reservations as JSON.
func (t *Table) ToJSON() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	reservations := make([]*Reservation, 0, len(t.reservations))
	for _, r := range t.reservations {
		reservations = append(reservations, r)
	}

	return json.MarshalIndent(reservations, "", "  ")
}
