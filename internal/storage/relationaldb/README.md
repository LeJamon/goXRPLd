# Relational Database Package

A clean, modular Go package for XRPL ledger and transaction storage using the Repository Pattern with PostgreSQL backend support.

## Architecture Overview

This package follows the **Repository Pattern** with domain-specific repositories and clean separation of concerns:

```
relationaldb/
├── interface.go              # Repository interfaces & data types
├── manager.go                # Connection lifecycle & utilities  
├── config.go                 # Configuration management
├── errors.go                 # Error types and handling
└── postgres/
    ├── executor.go           # Shared database executor interface
    ├── repository_manager.go # Main repository coordinator
    ├── ledger_repository.go  # Ledger operations
    ├── transaction_repository.go # Transaction operations
    ├── account_repository.go # Account transaction operations
    ├── system_repository.go  # System-level operations
    └── transaction_context.go # Transactional repository access
```

## Repository Pattern Benefits

### 1. **Single Responsibility Principle**
Each repository handles one domain:
- **LedgerRepository** - 13 methods for ledger operations
- **TransactionRepository** - 9 methods for transaction operations  
- **AccountTransactionRepository** - 8 methods for account transaction operations
- **SystemRepository** - 4 methods for connection/system operations

### 2. **Better Testing & Mocking**
```go
// Easy to mock individual repositories
type mockLedgerRepo struct{}
func (m *mockLedgerRepo) GetLedgerInfoBySeq(ctx context.Context, seq LedgerIndex) (*LedgerInfo, error) {
    return testLedger, nil
}

// Test with specific repository mock
func TestLedgerService(t *testing.T) {
    ledgerRepo := &mockLedgerRepo{}
    service := NewLedgerService(ledgerRepo) // Only needs ledger repo
    // assertions...
}
```Now 

### 3. **Cleaner Dependencies**
Services depend only on what they need:
```go
// Service only needs ledger operations
type LedgerService struct {
    ledgerRepo LedgerRepository
}

// Service only needs transaction operations  
type TransactionService struct {
    txRepo TransactionRepository
}

// Service needs multiple repositories
type SyncService struct {
    ledgerRepo LedgerRepository
    txRepo     TransactionRepository
}
```

## Core Interfaces

### Repository Interfaces
```go
type LedgerRepository interface {
    GetMinLedgerSeq(ctx context.Context) (*LedgerIndex, error)
    GetMaxLedgerSeq(ctx context.Context) (*LedgerIndex, error)
    GetLedgerInfoBySeq(ctx context.Context, seq LedgerIndex) (*LedgerInfo, error)
    SaveValidatedLedger(ctx context.Context, ledger *LedgerInfo, current bool) error
    // ... 9 more methods
}

type TransactionRepository interface {
    GetTransaction(ctx context.Context, hash Hash, ledgerRange *LedgerRange) (*TransactionInfo, TxSearchResult, error)
    SaveTransaction(ctx context.Context, txInfo *TransactionInfo) error
    GetTxHistory(ctx context.Context, startIndex LedgerIndex, limit int) ([]TransactionInfo, error)
    // ... 6 more methods
}

type AccountTransactionRepository interface {
    GetOldestAccountTxs(ctx context.Context, options AccountTxOptions) ([]TransactionInfo, error)
    GetNewestAccountTxs(ctx context.Context, options AccountTxOptions) ([]TransactionInfo, error)
    SaveAccountTransaction(ctx context.Context, accountID AccountID, txInfo *TransactionInfo) error
    // ... 5 more methods
}

type SystemRepository interface {
    GetKBUsedAll(ctx context.Context) (uint32, error)
    Ping(ctx context.Context) error
    Begin(ctx context.Context) (TransactionContext, error)
}
```

### Repository Manager
```go
type RepositoryManager interface {
    // Repository access
    Ledger() LedgerRepository
    Transaction() TransactionRepository
    AccountTransaction() AccountTransactionRepository
    System() SystemRepository
    
    // Connection management
    Open(ctx context.Context) error
    Close(ctx context.Context) error
    
    // Transaction management
    WithTransaction(ctx context.Context, fn func(TransactionContext) error) error
}
```

### Transaction Context
```go
type TransactionContext interface {
    Commit(ctx context.Context) error
    Rollback(ctx context.Context) error
    
    // Repository access within transaction
    Ledger() LedgerRepository
    Transaction() TransactionRepository
    AccountTransaction() AccountTransactionRepository
}
```

## Usage Patterns

### Basic Repository Access
```go
// Create and configure repository manager
config := &relationaldb.Config{
    Driver:   "postgres",
    Host:     "localhost",
    Database: "xrpl",
    Username: "user",
    Password: "pass",
}

repoManager, err := postgres.NewRepositoryManager(config)
if err != nil {
    log.Fatal(err)
}

// Open connection
ctx := context.Background()
err = repoManager.Open(ctx)
defer repoManager.Close(ctx)

// Use specific repositories
ledgerRepo := repoManager.Ledger()
ledger, err := ledgerRepo.GetLedgerInfoBySeq(ctx, 1000)

txRepo := repoManager.Transaction()
transactions, err := txRepo.GetTxHistory(ctx, 0, 10)
```

### Managed Usage (With Manager Utilities)
```go
// Add lifecycle management and utilities
manager := relationaldb.NewManager(repoManager, config)
err = manager.Open(ctx)
defer manager.Close(ctx)

// Use with retry logic and monitoring
err = manager.ExecuteWithRetry(ctx, func() error {
    return repoManager.Ledger().SaveValidatedLedger(ctx, ledger, true)
})

// Health monitoring
if err := manager.HealthCheck(ctx); err != nil {
    log.Printf("Database unhealthy: %v", err)
}
```

### Transaction Usage
```go
// Execute multiple operations in a transaction
err = repoManager.WithTransaction(ctx, func(txCtx TransactionContext) error {
    // Save ledger
    err := txCtx.Ledger().SaveValidatedLedger(ctx, newLedger, true)
    if err != nil {
        return err
    }
    
    // Save transactions in same transaction
    for _, tx := range transactions {
        err = txCtx.Transaction().SaveTransaction(ctx, &tx)
        if err != nil {
            return err
        }
    }
    
    return nil // Will commit automatically
})
```

### Service Dependency Injection
```go
// Services depend only on repositories they need
type LedgerService struct {
    ledgerRepo LedgerRepository
}

func NewLedgerService(ledgerRepo LedgerRepository) *LedgerService {
    return &LedgerService{ledgerRepo: ledgerRepo}
}

type SyncService struct {
    ledgerRepo LedgerRepository
    txRepo     TransactionRepository
    accountRepo AccountTransactionRepository
}

func NewSyncService(
    ledgerRepo LedgerRepository, 
    txRepo TransactionRepository,
    accountRepo AccountTransactionRepository,
) *SyncService {
    return &SyncService{
        ledgerRepo: ledgerRepo,
        txRepo: txRepo,
        accountRepo: accountRepo,
    }
}

// Wire up with repository manager
repoManager := // ... create repository manager
ledgerService := NewLedgerService(repoManager.Ledger())
syncService := NewSyncService(
    repoManager.Ledger(),
    repoManager.Transaction(),
    repoManager.AccountTransaction(),
)
```

## File Responsibilities

| File | Purpose | Key Components |
|------|---------|----------------|
| `interface.go` | **Repository Contracts** | Repository interfaces, data types, helper methods |
| `manager.go` | **Lifecycle Management** | Connection handling, health checks, retry logic, metrics |
| `config.go` | **Configuration** | Config structs, validation, defaults, connection strings |
| `errors.go` | **Error Handling** | Error types, categorization, recovery actions |
| `postgres/repository_manager.go` | **Main Coordinator** | PostgreSQL repository manager implementation |
| `postgres/ledger_repository.go` | **Ledger Operations** | PostgreSQL ledger repository implementation |
| `postgres/transaction_repository.go` | **Transaction Operations** | PostgreSQL transaction repository implementation |
| `postgres/account_repository.go` | **Account Operations** | PostgreSQL account transaction repository implementation |
| `postgres/system_repository.go` | **System Operations** | PostgreSQL system repository implementation |
| `postgres/transaction_context.go` | **Transactional Access** | PostgreSQL transaction context implementation |
| `postgres/executor.go` | **Shared Interface** | Common database/transaction executor interface |

## PostgreSQL Implementation Details

### Schema Structure
The PostgreSQL implementation follows rippled's table structure:
- **ledgers** - Ledger information with proper indexes
- **transactions** - Transaction data with metadata
- **account_transactions** - Account-transaction mapping for efficient queries

### Key Features
- **Concurrent Safe** - Uses `*sql.DB` and `*sql.Tx` properly
- **Transaction Support** - Full ACID transactions across repositories
- **Efficient Queries** - Optimized for XRPL access patterns
- **Error Handling** - Comprehensive error categorization and retryability
- **Monitoring** - Built-in metrics and space usage tracking

## Testing Strategy

### Unit Tests - Individual Repositories
```go
func TestLedgerRepository_GetLedgerInfoBySeq(t *testing.T) {
    repo := setupTestLedgerRepo(t)
    defer repo.Close()
    
    ledger, err := repo.GetLedgerInfoBySeq(ctx, 1000)
    assert.NoError(t, err)
    assert.Equal(t, LedgerIndex(1000), ledger.Sequence)
}
```

### Integration Tests - Repository Manager
```go
func TestRepositoryManager_TransactionWorkflow(t *testing.T) {
    repoManager := setupTestRepoManager(t)
    defer repoManager.Close(ctx)
    
    err := repoManager.WithTransaction(ctx, func(txCtx TransactionContext) error {
        return txCtx.Ledger().SaveValidatedLedger(ctx, testLedger, true)
    })
    assert.NoError(t, err)
}
```

### Mock Tests - Service Layer
```go
type mockLedgerRepo struct {
    ledgers map[LedgerIndex]*LedgerInfo
}

func (m *mockLedgerRepo) GetLedgerInfoBySeq(ctx context.Context, seq LedgerIndex) (*LedgerInfo, error) {
    if ledger, ok := m.ledgers[seq]; ok {
        return ledger, nil
    }
    return nil, ErrLedgerNotFound
}

func TestLedgerService_WithMock(t *testing.T) {
    mockRepo := &mockLedgerRepo{ledgers: testLedgers}
    service := NewLedgerService(mockRepo)
    
    result, err := service.ProcessLedger(ctx, 1000)
    // assertions...
}
```

## Extension Points

### Adding New Database Backends
1. Create new package: `mysql/`
2. Implement all repository interfaces
3. Create `NewMySQLRepositoryManager(config *Config)` constructor
4. No changes needed to existing service code

### Adding New Repository Operations
1. Add method to relevant repository interface in `interface.go`
2. Implement in all backends (compiler will enforce this)
3. Services using that repository get access automatically

### Adding New Repository Types
1. Define new repository interface in `interface.go`
2. Add to `RepositoryManager` interface
3. Implement in all backends
4. Add to `TransactionContext` if transactional access needed

## Migration Guide

### From Old Monolithic Interface
The old `Database` interface has been replaced with focused repository interfaces:

```go
// OLD WAY - Monolithic interface
var db Database
ledger, err := db.GetLedgerInfoBySeq(ctx, seq)
tx, err := db.GetTransaction(ctx, hash, nil)

// NEW WAY - Focused repositories  
var repoManager RepositoryManager
ledger, err := repoManager.Ledger().GetLedgerInfoBySeq(ctx, seq)
tx, err := repoManager.Transaction().GetTransaction(ctx, hash, nil)
```

### Benefits of Migration
- **Better Testing** - Mock only what you need
- **Cleaner Dependencies** - Services declare exactly what they use
- **Single Responsibility** - Each repository has a focused purpose
- **Easier Maintenance** - Changes to one domain don't affect others

## Best Practices

### Repository Usage
1. **Inject repositories, not the manager** - Services should depend on specific repository interfaces
2. **Use transactions for multi-repository operations** - `WithTransaction` ensures consistency
3. **Handle errors appropriately** - Check error types for retryability and specific handling

### Testing
1. **Mock at repository level** - Create focused mocks for individual repositories
2. **Test repository implementations** - Integration tests with real database
3. **Test service logic separately** - Unit tests with mocked repositories

### Performance
1. **Use connection pooling** - Configure `MaxOpenConns` and `MaxIdleConns` appropriately
2. **Batch operations when possible** - Group related operations in transactions
3. **Monitor space usage** - Use built-in space monitoring features

This architecture provides a clean, testable, and maintainable foundation for XRPL data storage while following Go best practices and the Repository Pattern.