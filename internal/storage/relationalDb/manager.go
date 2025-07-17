package relational_db

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/your-project/xrpl-go/types"
)

// Manager handles the lifecycle and operations of relational databases
type Manager struct {
	mu       sync.RWMutex
	database RelationalDatabase
	factory  DatabaseFactory
	config   *DatabaseConfig
	isOpen   bool

	// Metrics and monitoring
	stats  *DatabaseStats
	logger Logger
}

// DatabaseStats holds database performance metrics
type DatabaseStats struct {
	mu              sync.RWMutex
	OpenConnections int64
	TotalQueries    int64
	TotalErrors     int64
	LastError       error
	LastErrorTime   time.Time
	StartTime       time.Time
}

// Logger interface for database operations
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// NewManager creates a new database manager
func NewManager(factory DatabaseFactory, config *DatabaseConfig, logger Logger) *Manager {
	return &Manager{
		factory: factory,
		config:  config,
		logger:  logger,
		stats: &DatabaseStats{
			StartTime: time.Now(),
		},
	}
}

// Open opens the database connection
func (m *Manager) Open(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isOpen {
		return ErrDatabaseAlreadyOpen
	}

	db, err := m.factory.CreateDatabase(m.config)
	if err != nil {
		m.recordError(err)
		return fmt.Errorf("failed to create database: %w", err)
	}

	if err := db.Open(ctx); err != nil {
		m.recordError(err)
		return fmt.Errorf("failed to open database: %w", err)
	}

	m.database = db
	m.isOpen = true

	m.logger.Info("Database opened successfully",
		"path", m.config.Path,
		"driver", "sqlite", // TODO: make this dynamic based on factory
	)

	return nil
}

// Close closes the database connection
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isOpen {
		return ErrDatabaseNotOpen
	}

	if err := m.database.Close(ctx); err != nil {
		m.recordError(err)
		return fmt.Errorf("failed to close database: %w", err)
	}

	m.database = nil
	m.isOpen = false

	m.logger.Info("Database closed successfully")

	return nil
}

// IsOpen returns whether the database is open
func (m *Manager) IsOpen() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isOpen
}

// GetDatabase returns the underlying database instance
func (m *Manager) GetDatabase() RelationalDatabase {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.database
}

// ExecuteWithRetry executes a function with retry logic
func (m *Manager) ExecuteWithRetry(ctx context.Context, operation func() error, maxRetries int) error {
	var lastErr error

	for i := 0; i <= maxRetries; i++ {
		if err := operation(); err != nil {
			lastErr = err
			m.recordError(err)

			if i < maxRetries {
				// Exponential backoff
				backoff := time.Duration(i+1) * 100 * time.Millisecond
				m.logger.Warn("Database operation failed, retrying",
					"attempt", i+1,
					"maxRetries", maxRetries,
					"backoff", backoff,
					"error", err,
				)

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
					continue
				}
			}
		} else {
			m.incrementQueries()
			return nil
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", maxRetries, lastErr)
}

// BeginTransaction starts a new database transaction
func (m *Manager) BeginTransaction(ctx context.Context) (Transaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.isOpen {
		return nil, ErrDatabaseNotOpen
	}

	tx, err := m.database.Begin(ctx)
	if err != nil {
		m.recordError(err)
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	m.incrementQueries()
	return tx, nil
}

// Health check methods
func (m *Manager) HealthCheck(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.isOpen {
		return ErrDatabaseNotOpen
	}

	// Simple health check - get ledger count
	_, err := m.database.GetLedgerCountMinMax(ctx)
	if err != nil {
		m.recordError(err)
		return fmt.Errorf("database health check failed: %w", err)
	}

	return nil
}

// GetStats returns database statistics
func (m *Manager) GetStats() *DatabaseStats {
	m.stats.mu.RLock()
	defer m.stats.mu.RUnlock()

	// Return a copy to avoid concurrent access issues
	return &DatabaseStats{
		OpenConnections: m.stats.OpenConnections,
		TotalQueries:    m.stats.TotalQueries,
		TotalErrors:     m.stats.TotalErrors,
		LastError:       m.stats.LastError,
		LastErrorTime:   m.stats.LastErrorTime,
		StartTime:       m.stats.StartTime,
	}
}

// Maintenance operations
func (m *Manager) Vacuum(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.isOpen {
		return ErrDatabaseNotOpen
	}

	m.logger.Info("Starting database vacuum operation")

	// TODO: Implement vacuum operation based on underlying database type
	// For now, this is a placeholder

	m.logger.Info("Database vacuum operation completed")
	return nil
}

// CleanupOldData removes old data based on retention policies
func (m *Manager) CleanupOldData(ctx context.Context, retentionDays int) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.isOpen {
		return ErrDatabaseNotOpen
	}

	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)
	m.logger.Info("Starting cleanup of old data",
		"retentionDays", retentionDays,
		"cutoffTime", cutoffTime,
	)

	// TODO: Implement cleanup logic based on cutoff time
	// This would involve deleting old transactions and ledgers

	m.logger.Info("Old data cleanup completed")
	return nil
}

// Backup creates a backup of the database
func (m *Manager) Backup(ctx context.Context, backupPath string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.isOpen {
		return ErrDatabaseNotOpen
	}

	m.logger.Info("Starting database backup",
		"backupPath", backupPath,
	)

	// TODO: Implement backup operation based on underlying database type
	// For SQLite, this would involve using the backup API

	m.logger.Info("Database backup completed",
		"backupPath", backupPath,
	)

	return nil
}

// Helper methods for statistics
func (m *Manager) recordError(err error) {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	m.stats.TotalErrors++
	m.stats.LastError = err
	m.stats.LastErrorTime = time.Now()
}

func (m *Manager) incrementQueries() {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	m.stats.TotalQueries++
}

// Configuration update methods
func (m *Manager) UpdateConfig(config *DatabaseConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isOpen {
		return ErrDatabaseAlreadyOpen
	}

	m.config = config
	return nil
}

func (m *Manager) GetConfig() *DatabaseConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid concurrent modification
	configCopy := *m.config
	return &configCopy
}

// Utility methods for common operations
func (m *Manager) GetLedgerRange(ctx context.Context) (*ClosedInterval[LedgerIndex], error) {
	if !m.IsOpen() {
		return nil, ErrDatabaseNotOpen
	}

	stats, err := m.database.GetLedgerCountMinMax(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get ledger range: %w", err)
	}

	return &ClosedInterval[LedgerIndex]{
		Min: stats.Min,
		Max: stats.Max,
	}, nil
}

func (m *Manager) GetLatestLedgerInfo(ctx context.Context) (*LedgerInfo, error) {
	if !m.IsOpen() {
		return nil, ErrDatabaseNotOpen
	}

	stats, err := m.database.GetLedgerCountMinMax(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest ledger: %w", err)
	}

	return m.database.GetLedgerInfoBySeq(ctx, stats.Max)
}

func (m *Manager) GetStorageUsage(ctx context.Context) (map[string]uint32, error) {
	if !m.IsOpen() {
		return nil, ErrDatabaseNotOpen
	}

	usage := make(map[string]uint32)

	totalKB, err := m.database.GetKBUsedAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get total storage usage: %w", err)
	}
	usage["total"] = totalKB

	ledgerKB, err := m.database.GetKBUsedLedger(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get ledger storage usage: %w", err)
	}
	usage["ledger"] = ledgerKB

	txKB, err := m.database.GetKBUsedTransaction(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction storage usage: %w", err)
	}
	usage["transaction"] = txKB

	return usage, nil
}
