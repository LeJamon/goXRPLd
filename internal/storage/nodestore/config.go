package nodestore

import (
	"errors"
	"time"
)

// Config holds configuration options for the NodeStore.
type Config struct {
	// Backend specifies the storage backend to use.
	Backend string `json:"backend" yaml:"backend"`

	// Path specifies the file system path for data storage.
	Path string `json:"path" yaml:"path"`

	// Cache configuration.
	CacheSize int           `json:"cache_size" yaml:"cache_size"`
	CacheTTL  time.Duration `json:"cache_ttl" yaml:"cache_ttl"`

	// Compressor is kept for backwards compatibility but is ignored.
	// Pebble handles compression natively via SnappyCompression.
	Compressor string `json:"compressor" yaml:"compressor"`

	// CompressionLevel is kept for backwards compatibility but is ignored.
	CompressionLevel int `json:"compression_level" yaml:"compression_level"`

	// BatchSize is the default batch size for operations.
	BatchSize int `json:"batch_size" yaml:"batch_size"`

	// CreateIfMissing controls whether the database should be created if it doesn't exist.
	CreateIfMissing bool `json:"create_if_missing" yaml:"create_if_missing"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Backend:         "pebble",
		Path:            "./nodestore",
		CacheSize:       2000,
		CacheTTL:        time.Hour,
		Compressor:      "none",
		BatchSize:       100,
		CreateIfMissing: true,
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Backend == "" {
		return errors.New("backend must be specified")
	}

	if c.Path == "" {
		return errors.New("path must be specified")
	}

	if c.CacheSize < 0 {
		return errors.New("cache_size must be non-negative")
	}

	if c.CacheTTL < 0 {
		return errors.New("cache_ttl must be non-negative")
	}

	if c.BatchSize < 1 {
		return errors.New("batch_size must be at least 1")
	}

	return nil
}

// Option represents a functional option for configuring the NodeStore.
type Option func(*Config)

// WithPath sets the storage path.
func WithPath(path string) Option {
	return func(c *Config) {
		c.Path = path
	}
}

// WithBackend sets the storage backend.
func WithBackend(backend string) Option {
	return func(c *Config) {
		c.Backend = backend
	}
}

// WithCacheSize sets the cache size (number of items).
func WithCacheSize(size int) Option {
	return func(c *Config) {
		c.CacheSize = size
	}
}

// WithCacheTTL sets the cache time-to-live duration.
func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Config) {
		c.CacheTTL = ttl
	}
}

// WithBatchSize sets the default batch size for operations.
func WithBatchSize(size int) Option {
	return func(c *Config) {
		c.BatchSize = size
	}
}

// WithCreateIfMissing controls whether the database should be created if it doesn't exist.
func WithCreateIfMissing(create bool) Option {
	return func(c *Config) {
		c.CreateIfMissing = create
	}
}

// Clone creates a copy of the configuration.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	return &Config{
		Backend:         c.Backend,
		Path:            c.Path,
		CacheSize:       c.CacheSize,
		CacheTTL:        c.CacheTTL,
		Compressor:      c.Compressor,
		BatchSize:       c.BatchSize,
		CreateIfMissing: c.CreateIfMissing,
	}
}
