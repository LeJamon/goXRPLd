package service

import (
	"errors"
	"time"

	"github.com/LeJamon/goXRPLd/internal/core/ledger"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/genesis"
)

// LedgerManager handles ledger lifecycle and state management.
type LedgerManager struct {
	// Current open ledger (accepting transactions)
	openLedger *ledger.Ledger

	// Last closed ledger
	closedLedger *ledger.Ledger

	// Validated ledger (highest validated)
	validatedLedger *ledger.Ledger

	// Genesis ledger
	genesisLedger *ledger.Ledger

	// Ledger history (sequence -> ledger) - in-memory cache
	ledgerHistory map[uint32]*ledger.Ledger

	// standalone mode flag
	standalone bool
}

// NewLedgerManager creates a new ledger manager.
func NewLedgerManager(standalone bool) *LedgerManager {
	return &LedgerManager{
		ledgerHistory: make(map[uint32]*ledger.Ledger),
		standalone:    standalone,
	}
}

// Initialize sets up the ledger manager with a genesis ledger.
func (m *LedgerManager) Initialize(genesisConfig genesis.Config) error {
	// Create genesis ledger
	genesisResult, err := genesis.Create(genesisConfig)
	if err != nil {
		return errors.New("failed to create genesis ledger: " + err.Error())
	}

	// Convert genesis to Ledger
	genesisLedger := ledger.FromGenesis(
		genesisResult.Header,
		genesisResult.StateMap,
		genesisResult.TxMap,
		m.defaultFees(),
	)

	m.genesisLedger = genesisLedger
	m.closedLedger = genesisLedger
	m.validatedLedger = genesisLedger
	m.ledgerHistory[genesisLedger.Sequence()] = genesisLedger

	// Create the first open ledger (ledger 2)
	openLedger, err := ledger.NewOpen(genesisLedger, time.Now())
	if err != nil {
		return errors.New("failed to create open ledger: " + err.Error())
	}
	m.openLedger = openLedger

	return nil
}

// defaultFees returns default fee settings.
func (m *LedgerManager) defaultFees() interface{} {
	// Return appropriate type based on the ledger package
	return nil
}

// GetOpenLedger returns the current open ledger.
func (m *LedgerManager) GetOpenLedger() *ledger.Ledger {
	return m.openLedger
}

// GetClosedLedger returns the last closed ledger.
func (m *LedgerManager) GetClosedLedger() *ledger.Ledger {
	return m.closedLedger
}

// GetValidatedLedger returns the highest validated ledger.
func (m *LedgerManager) GetValidatedLedger() *ledger.Ledger {
	return m.validatedLedger
}

// GetGenesisLedger returns the genesis ledger.
func (m *LedgerManager) GetGenesisLedger() *ledger.Ledger {
	return m.genesisLedger
}

// GetLedgerBySequence returns a ledger by its sequence number.
func (m *LedgerManager) GetLedgerBySequence(seq uint32) (*ledger.Ledger, error) {
	l, ok := m.ledgerHistory[seq]
	if !ok {
		return nil, ErrLedgerNotFound
	}
	return l, nil
}

// GetLedgerByHash returns a ledger by its hash.
func (m *LedgerManager) GetLedgerByHash(hash [32]byte) (*ledger.Ledger, error) {
	for _, l := range m.ledgerHistory {
		if l.Hash() == hash {
			return l, nil
		}
	}
	return nil, ErrLedgerNotFound
}

// GetCurrentLedgerIndex returns the current open ledger index.
func (m *LedgerManager) GetCurrentLedgerIndex() uint32 {
	if m.openLedger == nil {
		return 0
	}
	return m.openLedger.Sequence()
}

// GetClosedLedgerIndex returns the last closed ledger index.
func (m *LedgerManager) GetClosedLedgerIndex() uint32 {
	if m.closedLedger == nil {
		return 0
	}
	return m.closedLedger.Sequence()
}

// GetValidatedLedgerIndex returns the highest validated ledger index.
func (m *LedgerManager) GetValidatedLedgerIndex() uint32 {
	if m.validatedLedger == nil {
		return 0
	}
	return m.validatedLedger.Sequence()
}

// AcceptLedger closes the current open ledger and creates a new one.
// Returns the closed ledger sequence and the closed ledger.
func (m *LedgerManager) AcceptLedger() (uint32, *ledger.Ledger, error) {
	if !m.standalone {
		return 0, nil, ErrNotStandalone
	}

	if m.openLedger == nil {
		return 0, nil, ErrNoOpenLedger
	}

	// Close the current open ledger
	closeTime := time.Now()
	if err := m.openLedger.Close(closeTime, 0); err != nil {
		return 0, nil, errors.New("failed to close ledger: " + err.Error())
	}

	// In standalone mode, immediately validate
	if err := m.openLedger.SetValidated(); err != nil {
		return 0, nil, errors.New("failed to validate ledger: " + err.Error())
	}

	// Store the closed ledger
	closedSeq := m.openLedger.Sequence()
	closedLedger := m.openLedger
	m.closedLedger = closedLedger
	m.validatedLedger = closedLedger
	m.ledgerHistory[closedSeq] = closedLedger

	// Create new open ledger
	newOpen, err := ledger.NewOpen(m.closedLedger, time.Now())
	if err != nil {
		return 0, nil, errors.New("failed to create new open ledger: " + err.Error())
	}
	m.openLedger = newOpen

	return closedSeq, closedLedger, nil
}

// GetLedgerHistory returns the full ledger history map.
func (m *LedgerManager) GetLedgerHistory() map[uint32]*ledger.Ledger {
	return m.ledgerHistory
}

// IsStandalone returns true if running in standalone mode.
func (m *LedgerManager) IsStandalone() bool {
	return m.standalone
}

// GetValidatedLedgersRange returns a string representation of validated ledger range.
func (m *LedgerManager) GetValidatedLedgersRange() string {
	if len(m.ledgerHistory) == 0 {
		return "empty"
	}

	minSeq := uint32(0xFFFFFFFF)
	maxSeq := uint32(0)
	for seq := range m.ledgerHistory {
		if seq < minSeq {
			minSeq = seq
		}
		if seq > maxSeq {
			maxSeq = seq
		}
	}

	if minSeq == maxSeq {
		return formatUint32(minSeq)
	}
	return formatUint32(minSeq) + "-" + formatUint32(maxSeq)
}

// formatUint32 converts a uint32 to string.
func formatUint32(n uint32) string {
	s := ""
	if n == 0 {
		return "0"
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
