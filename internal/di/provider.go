package di

import (
	"github.com/LeJamon/goXRPLd/internal/config"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/genesis"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/service"
	"github.com/LeJamon/goXRPLd/internal/storage/nodestore"
	"github.com/LeJamon/goXRPLd/internal/storage/relationaldb"
)

// Provider configures and registers services in the container.
type Provider struct {
	container *Container
	config    *config.Config
}

// NewProvider creates a new service provider.
func NewProvider(container *Container, cfg *config.Config) *Provider {
	return &Provider{
		container: container,
		config:    cfg,
	}
}

// RegisterAll registers all services.
func (p *Provider) RegisterAll() error {
	// Register config
	p.container.Register(ServiceConfig, p.config)

	// Register builders for lazy instantiation
	p.registerStorageBuilders()
	p.registerLedgerBuilders()
	p.registerRPCBuilders()

	return nil
}

// registerStorageBuilders registers storage service builders.
func (p *Provider) registerStorageBuilders() {
	// NodeStore builder
	p.container.RegisterBuilder(ServiceNodeStore, func(c *Container) (interface{}, error) {
		if p.config.Database.NodeStore.Path == "" {
			return nil, nil // No nodestore configured
		}

		db, err := nodestore.NewPebbleDatabase(p.config.Database.NodeStore.Path, nil)
		if err != nil {
			return nil, err
		}
		return db, nil
	})

	// RelationalDB builder
	p.container.RegisterBuilder(ServiceRelationalDB, func(c *Container) (interface{}, error) {
		if p.config.Database.RelationalDB.ConnectionString == "" {
			return nil, nil // No relational DB configured
		}

		// Create repository manager based on config
		// This is a placeholder - implement actual connection
		return nil, nil
	})
}

// registerLedgerBuilders registers ledger service builders.
func (p *Provider) registerLedgerBuilders() {
	// Fee Manager builder
	p.container.RegisterBuilder(ServiceFeeManager, func(c *Container) (interface{}, error) {
		return service.NewFeeManager(), nil
	})

	// Event Publisher builder
	p.container.RegisterBuilder(ServiceEventPublisher, func(c *Container) (interface{}, error) {
		return service.NewEventPublisher(), nil
	})

	// Ledger Service builder
	p.container.RegisterBuilder(ServiceLedger, func(c *Container) (interface{}, error) {
		// Get dependencies
		var nodeStore nodestore.Database
		if ns, err := c.Get(ServiceNodeStore); err == nil && ns != nil {
			nodeStore = ns.(nodestore.Database)
		}

		var relDB relationaldb.RepositoryManager
		if rdb, err := c.Get(ServiceRelationalDB); err == nil && rdb != nil {
			relDB = rdb.(relationaldb.RepositoryManager)
		}

		// Create ledger service config
		cfg := service.Config{
			Standalone: p.config.Server.Standalone,
			GenesisConfig: genesis.Config{
				// Use defaults from config
			},
			NodeStore:    nodeStore,
			RelationalDB: relDB,
		}

		svc, err := service.New(cfg)
		if err != nil {
			return nil, err
		}

		return svc, nil
	})

	// Transaction Index builder
	p.container.RegisterBuilder(ServiceTxIndex, func(c *Container) (interface{}, error) {
		// Get dependencies
		ledgerSvc, err := c.Get(ServiceLedger)
		if err != nil {
			return nil, err
		}

		var relDB relationaldb.RepositoryManager
		if rdb, err := c.Get(ServiceRelationalDB); err == nil && rdb != nil {
			relDB = rdb.(relationaldb.RepositoryManager)
		}

		// Create transaction index
		// This requires access to ledger manager within service
		_ = ledgerSvc
		_ = relDB

		return nil, nil // Placeholder
	})
}

// registerRPCBuilders registers RPC service builders.
func (p *Provider) registerRPCBuilders() {
	// RPC Server builder - implemented elsewhere
}

// GetLedgerService returns the ledger service from the container.
func (p *Provider) GetLedgerService() (*service.Service, error) {
	svc, err := p.container.Get(ServiceLedger)
	if err != nil {
		return nil, err
	}
	return svc.(*service.Service), nil
}

// GetConfig returns the configuration from the container.
func (p *Provider) GetConfig() *config.Config {
	return p.config
}
