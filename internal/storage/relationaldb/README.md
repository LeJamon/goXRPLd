# Relational Database Package

A clean, idiomatic Go package for XRPL ledger and transaction storage with PostgreSQL backend support.

## Architecture Overview

This package follows a **pragmatic layered architecture** that balances simplicity with flexibility:

```
relational_db/
├── interface.go              # Pure interface contracts
├── manager.go                # Connection lifecycle & utilities  
├── config.go                 # Configuration management
├── errors.go                 # Error types and handling
└── postgres/
    └── postgres.go           # PostgreSQL implementation
```

## Design Patterns

### 1. **Interface Segregation**
- **`interface.go`** - Contains only pure interface definitions and data types
- No implementation details, no dependencies on specific databases
- Enables easy testing with mocks and future database backends

### 2. **Single Implementation Pattern**
- **`postgres/postgres.go`** - One file, one struct, implements the interface
- Wraps `*sql.DB` and `*sql.Tx` with domain-specific methods
- No over-abstraction, direct use of `database/sql`

### 3. **Manager Pattern**
- **`manager.go`** - Handles cross-cutting concerns without complexity
- Connection lifecycle, health checks, retry logic, metrics
- Optional layer - you can use the database directly if preferred

### 4. **Configuration as Data**
- **`config.go`** - Configuration structs with validation methods
- No factories, no builders, just plain structs with sensible defaults
- Explicit conversion methods for different database types

## File Responsibilities

| File | Purpose | Key Components |
|------|---------|----------------|
| `interface.go` | **Contract Definition** | `Database` interface, `Transaction` interface, data types |
| `manager.go` | **Lifecycle Management** | Connection handling, health checks, retry logic, metrics |
| `config.go` | **Configuration** | Config structs, validation, defaults, connection strings |
| `errors.go` | **Error Handling** | Error types, categorization, recovery actions |
| `postgres/postgres.go` | **Implementation** | PostgreSQL-specific database and transaction logic |

## Core Principles

### ✅ **Do These Things**
- Keep interfaces focused and cohesive
- Use standard library types (`*sql.DB`, `*sql.Tx`)
- Embed interfaces when you need composition
- Make zero values useful where possible
- Use simple constructor functions (`NewXxx()`)

### ❌ **Avoid These Things**
- Factory patterns (not idiomatic in Go)
- Deep inheritance hierarchies
- Over-abstraction with too many layers
- Interface pollution (don't make interfaces for everything)
- Complex dependency injection frameworks

## Usage Patterns

### Basic Usage (Direct Database)
```go
// Create and configure
config := &relational_db.Config{
    Driver:           "postgres", 
    ConnectionString: "postgres://user:pass@localhost/xrpl",
}

db, err := postgres.NewDatabase(config)
if err != nil {
    log.Fatal(err)
}

// Use directly
ctx := context.Background()
err = db.Open(ctx)
defer db.Close(ctx)

ledger, err := db.GetLedgerInfoBySeq(ctx, 1000)
```

### Managed Usage (With Utilities)
```go
// Add lifecycle management and utilities
manager := relational_db.NewManager(db, config, logger)
err = manager.Open(ctx)
defer manager.Close(ctx)

// Use with retry logic
err = manager.ExecuteWithRetry(ctx, func() error {
    return db.SaveValidatedLedger(ctx, ledger, true)
})

// Health monitoring
if err := manager.HealthCheck(ctx); err != nil {
    log.Printf("Database unhealthy: %v", err)
}
```

### Transaction Usage
```go
// Transactions work exactly like the main database
tx, err := db.Begin(ctx)
if err != nil {
    return err
}
defer tx.Rollback(ctx) // Safe to call even after commit

// Same interface as Database
err = tx.SaveValidatedLedger(ctx, newLedger, true)
if err != nil {
    return err
}

return tx.Commit(ctx)
```

## Why This Architecture?

### **Testability**
```go
// Easy to mock the interface
type mockDB struct{}
func (m *mockDB) GetLedgerInfoBySeq(ctx context.Context, seq LedgerIndex) (*LedgerInfo, error) {
    return testLedger, nil
}

// Test with mock
func TestLedgerQuery(t *testing.T) {
    db := &mockDB{}
    result, err := db.GetLedgerInfoBySeq(ctx, 123)
    // assertions...
}
```

### **Flexibility**
```go
// Easy to add new backends
func NewMySQLDatabase(config *Config) (Database, error) {
    // MySQL implementation
}

// Same interface, different backend
var db Database
if usePostgres {
    db, _ = postgres.NewDatabase(config)
} else {
    db, _ = mysql.NewDatabase(config)
}
```

### **Simplicity**
- No dependency injection frameworks
- No complex factory hierarchies
- Standard library patterns
- Clear, explicit code paths

## Extension Points

### Adding New Database Backends
1. Create new package: `mysql/mysql.go`
2. Implement `Database` interface
3. Add constructor: `NewMySQLDatabase(config *Config)`
4. No changes needed to existing code

### Adding New Operations
1. Add method to `Database` interface in `interface.go`
2. Implement in all backends (compiler will enforce this)
3. Add to `Transaction` interface if needed

### Adding Cross-Cutting Concerns
1. Extend `Manager` with new utilities
2. Add configuration options to `Config`
3. Keep it optional - direct database usage should still work

## Testing Strategy

### Unit Tests
```go
// Test individual database operations
func TestGetLedgerInfoBySeq(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close(context.Background())
    
    // Test implementation
}
```

### Integration Tests
```go
// Test full stack with real database
func TestFullLedgerWorkflow(t *testing.T) {
    db := setupPostgresDB(t)
    defer db.Close(context.Background())
    
    // Test save -> retrieve -> verify
}
```

### Mock Tests
```go
// Test business logic with mocked database
type mockDB struct {
    ledgers map[LedgerIndex]*LedgerInfo
}

func (m *mockDB) GetLedgerInfoBySeq(ctx context.Context, seq LedgerIndex) (*LedgerInfo, error) {
    if ledger, ok := m.ledgers[seq]; ok {
        return ledger, nil
    }
    return nil, ErrLedgerNotFound
}
```

## Implementation Guidelines

When implementing this pattern:

1. **Start with `interface.go`** - Define your contracts first
2. **Add configuration** - Keep it simple with validation methods
3. **Implement one backend** - Focus on correctness before flexibility
4. **Add manager utilities** - Only what you actually need
5. **Write tests** - Both unit and integration tests
6. **Add complexity gradually** - Don't over-engineer upfront

This architecture scales from simple scripts to production services while maintaining Go's philosophy of simplicity and explicitness.