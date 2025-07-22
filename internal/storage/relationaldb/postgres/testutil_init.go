package postgres

import (
	"github.com/LeJamon/goXRPLd/internal/storage/relationaldb"
	"github.com/LeJamon/goXRPLd/testutils"
)

func init() {
	// Register the PostgreSQL repository manager factory with testutils
	testutils.RegisterRepositoryFactory("postgres", func(config *relationaldb.Config) (relationaldb.RepositoryManager, error) {
		return NewRepositoryManager(config)
	})
}