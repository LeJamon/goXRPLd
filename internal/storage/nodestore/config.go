package nodestore

import (
	"errors"
	"fmt"
	"time"
)

// Config holds configuration options for the NodeStore.
type Config struct {
	// Backend specifies the storage backend to use
	Backend string `json:"backend" yaml:"backend"`

	// Path specifies the file system path for data storage
	Path string `json:"path" yaml:"path"`

	// Cache configuration
	CacheSize int           `json:"cache_size" yaml:"cache_size"`
	CacheTTL  time.Duration `json:"cache_ttl" yaml:"cache_ttl"`

	// Compression configuration
	Compressor       string `json:"compressor" yaml:"compressor"`
	CompressionLevel int    `json:"compression_level" yaml:"compression_level"`

	// Async read configuration
	ReadThreads int `json:"read_threads" yaml:"read_threads"`
	BatchSize   int `json:"batch_size" yaml:"batch_size"`

	// Advanced tuning
	RequestBundle   int  `json:"request_bundle" yaml:"request_bundle"`
	CreateIfMissing bool `json:"create_if_missing" yaml:"create_if_missing"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Backend:          "pebble",
		Path:             "./nodestore",
		CacheSize:        2000,
		CacheTTL:         time.Hour,
		Compressor:       "lz4",
		CompressionLevel: 1,
		ReadThreads:      8,
		BatchSize:        100,
		RequestBundle:    4,
		CreateIfMissing:  true,
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

	if c.CompressionLevel < 0 || c.CompressionLevel > 9 {
		return errors.New("compression_level must be between 0 and 9")
	}

	if c.ReadThreads < 1 {
		return errors.New("read_threads must be at least 1")
	}

	if c.BatchSize < 1 {
		return errors.New("batch_size must be at least 1")
	}

	if c.RequestBundle < 1 || c.RequestBundle > 64 {
		return errors.New("request_bundle must be between 1 and 64")
	}

	// Validate compressor
	validCompressors := map[string]bool{
		"lz4":    true,
		"snappy": true,
		"zstd":   true,
		"none":   true,
	}
	if !validCompressors[c.Compressor] {
		return fmt.Errorf("unsupported compressor: %s", c.Compressor)
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

// WithCompression sets the compression algorithm and level.
func WithCompression(compressor string, level int) Option {
	return func(c *Config) {
		c.Compressor = compressor
		c.CompressionLevel = level
	}
}

// WithReadThreads sets the number of async read threads.
func WithReadThreads(threads int) Option {
	return func(c *Config) {
		c.ReadThreads = threads
	}
}

// WithBatchSize sets the default batch size for operations.
func WithBatchSize(size int) Option {
	return func(c *Config) {
		c.BatchSize = size
	}
}

// WithRequestBundle sets the request bundling factor.
func WithRequestBundle(bundle int) Option {
	return func(c *Config) {
		c.RequestBundle = bundle
	}
}

// WithCreateIfMissing controls whether the database should be created if it doesn't exist.
func WithCreateIfMissing(create bool) Option {
	return func(c *Config) {
		c.CreateIfMissing = create
	}
}

// ApplyOptions applies the given options to the config.
func (c *Config) ApplyOptions(options ...Option) {
	for _, option := range options {
		option(c)
	}
}

// Clone creates a copy of the configuration.
func (c *Config) Clone() *Config {
	return &Config{
		Backend:          c.Backend,
		Path:             c.Path,
		CacheSize:        c.CacheSize,
		CacheTTL:         c.CacheTTL,
		Compressor:       c.Compressor,
		CompressionLevel: c.CompressionLevel,
		ReadThreads:      c.ReadThreads,
		BatchSize:        c.BatchSize,
		RequestBundle:    c.RequestBundle,
		CreateIfMissing:  c.CreateIfMissing,
	}
}

// String returns a string representation of the configuration.
func (c *Config) String() string {
	return fmt.Sprintf(`NodeStore Configuration:
  Backend: %s
  Path: %s
  Cache: %d items, TTL: %v
  Compression: %s (level %d)
  Async: %d threads, batch size: %d
  Request Bundle: %d
  Create If Missing: %t`,
		c.Backend,
		c.Path,
		c.CacheSize, c.CacheTTL,
		c.Compressor, c.CompressionLevel,
		c.ReadThreads, c.BatchSize,
		c.RequestBundle,
		c.CreateIfMissing)
}
