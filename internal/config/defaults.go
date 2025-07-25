package config

import "github.com/spf13/viper"

// setDefaults sets all default values that match rippled's defaults
func setDefaults(v *viper.Viper) {
	// 2. Peer Protocol defaults
	v.SetDefault("compression", false)
	v.SetDefault("peer_private", 0)
	v.SetDefault("peers_max", 0) // 0 means implementation-defined default
	v.SetDefault("max_transactions", 250)

	// Default IPs (rippled's built-in list)
	v.SetDefault("ips", []string{

		"r.ripple.com 51235",
		"sahyadri.isrdc.in 51235",
		"hubs.xrpkuwait.com 51235",
		"hub.xrpl-commons.org 51235",
	})

	// Overlay defaults
	v.SetDefault("overlay.max_unknown_time", 600)
	v.SetDefault("overlay.max_diverged_time", 300)

	// Transaction queue defaults (EXPERIMENTAL)
	v.SetDefault("transaction_queue.ledgers_in_queue", 20)
	v.SetDefault("transaction_queue.minimum_queue_size", 2000)
	v.SetDefault("transaction_queue.retry_sequence_percent", 25)
	v.SetDefault("transaction_queue.minimum_escalation_multiplier", 500)
	v.SetDefault("transaction_queue.minimum_txn_in_ledger", 5)
	v.SetDefault("transaction_queue.minimum_txn_in_ledger_standalone", 1000)
	v.SetDefault("transaction_queue.target_txn_in_ledger", 50)
	v.SetDefault("transaction_queue.maximum_txn_in_ledger", 0) // 0 means no maximum
	v.SetDefault("transaction_queue.normal_consensus_increase_percent", 20)
	v.SetDefault("transaction_queue.slow_consensus_decrease_percent", 50)
	v.SetDefault("transaction_queue.maximum_txn_per_account", 10)
	v.SetDefault("transaction_queue.minimum_last_ledger_buffer", 2)
	v.SetDefault("transaction_queue.zero_basefee_transaction_feelevel", 256000)

	// 3. Ripple Protocol defaults
	v.SetDefault("relay_proposals", "trusted")
	v.SetDefault("relay_validations", "all")
	v.SetDefault("ledger_history", 256)
	v.SetDefault("fetch_depth", "full")
	v.SetDefault("path_search", 2)
	v.SetDefault("path_search_fast", 2)
	v.SetDefault("path_search_max", 3)
	v.SetDefault("path_search_old", 2)
	v.SetDefault("workers", 0)          // 0 means auto-detect
	v.SetDefault("io_workers", 0)       // 0 means auto-detect
	v.SetDefault("prefetch_workers", 0) // 0 means auto-detect
	v.SetDefault("ledger_replay", 0)

	// 4. HTTPS Client defaults
	v.SetDefault("ssl_verify", 1)

	// 6. Database defaults
	// NodeDB defaults
	v.SetDefault("node_db.type", "NuDB")
	v.SetDefault("node_db.path", "/var/lib/rippled/db/nudb")
	v.SetDefault("node_db.cache_size", 16384)
	v.SetDefault("node_db.cache_age", 5)
	v.SetDefault("node_db.fast_load", false)
	v.SetDefault("node_db.earliest_seq", 32570)
	v.SetDefault("node_db.online_delete", 512)
	v.SetDefault("node_db.advisory_delete", 0)
	v.SetDefault("node_db.delete_batch", 100)
	v.SetDefault("node_db.back_off_milliseconds", 100)
	v.SetDefault("node_db.age_threshold_seconds", 60)
	v.SetDefault("node_db.recovery_wait_seconds", 5)

	// Database path default
	v.SetDefault("database_path", "/var/lib/rippled/db")

	// SQLite defaults (high safety)
	v.SetDefault("sqlite.safety_level", "")
	v.SetDefault("sqlite.journal_mode", "wal")
	v.SetDefault("sqlite.synchronous", "normal")
	v.SetDefault("sqlite.temp_store", "file")
	v.SetDefault("sqlite.page_size", 4096)
	v.SetDefault("sqlite.journal_size_limit", 1582080)

	// 7. Diagnostics defaults
	v.SetDefault("debug_logfile", "/var/log/rippled/debug.log")
	v.SetDefault("perf.log_interval", 1)

	// 9. Misc Settings defaults
	v.SetDefault("node_size", "") // Empty means auto-detect
	v.SetDefault("signing_support", false)
	v.SetDefault("beta_rpc_api", 0)
	v.SetDefault("websocket_ping_frequency", 0) // 0 means default

	// Crawl defaults
	v.SetDefault("crawl.enabled", true)
	v.SetDefault("crawl.overlay", 1)
	v.SetDefault("crawl.server", 1)
	v.SetDefault("crawl.counts", 0)
	v.SetDefault("crawl.unl", 1)

	// VL defaults
	v.SetDefault("vl.enable", 1)

	// Validators file default
	v.SetDefault("validators_file", "validators.txt")

	// Port-specific defaults
	setPortDefaults(v)
}

// setPortDefaults sets default values for common port configurations
func setPortDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.port", 80)
	v.SetDefault("server.ip", "0.0.0.0")

	// Common port defaults that apply to all ports unless overridden
	v.SetDefault("limit", 0) // 0 means unlimited
	v.SetDefault("send_queue_limit", 100)
	v.SetDefault("permessage_deflate", false)
	v.SetDefault("compress_level", 3)
	v.SetDefault("memory_level", 8) // Default memory level
	v.SetDefault("client_max_window_bits", 15)
	v.SetDefault("server_max_window_bits", 15)
	v.SetDefault("client_no_context_takeover", false)
	v.SetDefault("server_no_context_takeover", false)

	// Example port configurations (these would be in actual config files)
	// These are here as reference for what a typical setup might look like
	setExamplePortDefaults(v)
}

// setExamplePortDefaults sets up example port configurations
// These demonstrate typical rippled port setups
func setExamplePortDefaults(v *viper.Viper) {
	// Example: RPC Admin Local Port
	v.SetDefault("port_rpc_admin_local.port", 5005)
	v.SetDefault("port_rpc_admin_local.ip", "127.0.0.1")
	v.SetDefault("port_rpc_admin_local.protocol", "http")
	v.SetDefault("port_rpc_admin_local.admin", []string{"127.0.0.1"})

	// Example: Peer Port
	v.SetDefault("port_peer.port", 51235)
	v.SetDefault("port_peer.ip", "0.0.0.0")
	v.SetDefault("port_peer.protocol", "peer")

	// Example: WebSocket Admin Local Port
	v.SetDefault("port_ws_admin_local.port", 6006)
	v.SetDefault("port_ws_admin_local.ip", "127.0.0.1")
	v.SetDefault("port_ws_admin_local.protocol", "ws")
	v.SetDefault("port_ws_admin_local.admin", []string{"127.0.0.1"})
	v.SetDefault("port_ws_admin_local.send_queue_limit", 500)

	// Example: gRPC Port (for Clio)
	v.SetDefault("port_grpc.port", 50051)
	v.SetDefault("port_grpc.ip", "127.0.0.1")
	v.SetDefault("port_grpc.protocol", "grpc")
	v.SetDefault("port_grpc.secure_gateway", []string{"127.0.0.1"})
}

// GetDefaultNetworkConfig returns default network-specific configurations
func GetDefaultNetworkConfig(networkID interface{}) map[string]interface{} {
	defaults := make(map[string]interface{})

	switch networkID {
	case 0, "main":
		// Mainnet defaults
		defaults["validators_file"] = "validators.txt"
		defaults["ips"] = []string{
			"r.ripple.com 51235",
			"sahyadri.isrdc.in 51235",
			"hubs.xrpkuwait.com 51235",
			"hub.xrpl-commons.org 51235",
		}

	case 1, "testnet":
		// Testnet defaults
		defaults["validators_file"] = "validators-testnet.txt"
		defaults["ips"] = []string{
			"r.altnet.rippletest.net 51235",
		}

	case 2, "devnet":
		// Devnet defaults - typically empty, configured per deployment
		defaults["validators_file"] = "validators-devnet.txt"
		defaults["ips"] = []string{}
	}

	return defaults
}

// GetDefaultValidatorsConfig returns default validators configuration based on network
func GetDefaultValidatorsConfig(networkID interface{}) ValidatorsConfig {
	switch networkID {
	case 0, "main":
		// Mainnet validator configuration
		return ValidatorsConfig{
			ValidatorListSites: []string{
				"https://vl.ripple.com",
				"https://unl.xrplf.org",
			},
			ValidatorListKeys: []string{
				"ED2677ABFFD1B33AC6FBC3062B71F1E8397C1505E1C42C64D11AD1B28FF73F4734", // vl.ripple.com
				"ED42AEC58B701EEBB77356FFFEC26F83C1F0407263530F068C7C73D392C7E06FD1", // unl.xrplf.org
			},
			ValidatorListThreshold: 0, // Auto-calculate
		}

	case 1, "testnet":
		// Testnet validator configuration
		return ValidatorsConfig{
			ValidatorListSites: []string{
				"https://vl.altnet.rippletest.net",
			},
			ValidatorListKeys: []string{
				"ED264807102805220DA0F312E71FC2C69E1552C9C5790F6C25E3729DEB573D5860",
			},
			ValidatorListThreshold: 0, // Auto-calculate
		}

	case 2, "devnet":
		// Devnet - empty by default, configured per deployment
		return ValidatorsConfig{}

	default:
		// Unknown network - use mainnet defaults
		return GetDefaultValidatorsConfig("main")
	}
}

// ApplyNetworkDefaults applies network-specific defaults to the viper instance
func ApplyNetworkDefaults(v *viper.Viper, networkID interface{}) {
	networkDefaults := GetDefaultNetworkConfig(networkID)
	for key, value := range networkDefaults {
		v.SetDefault(key, value)
	}
}
