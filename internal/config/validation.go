package config

import (
	"fmt"
	"strings"
)

// ValidateConfig performs comprehensive validation on the complete configuration
func ValidateConfig(config *Config) error {
	// Validate server configuration
	if err := validateServerConfig(&config.Server); err != nil {
		return fmt.Errorf("server config validation failed: %w", err)
	}

	// Validate port configurations
	if err := validatePorts(config.Ports); err != nil {
		return fmt.Errorf("port config validation failed: %w", err)
	}

	// Validate peer protocol settings
	if err := validatePeerProtocol(config); err != nil {
		return fmt.Errorf("peer protocol validation failed: %w", err)
	}

	// Validate ripple protocol settings
	if err := validateRippleProtocol(config); err != nil {
		return fmt.Errorf("ripple protocol validation failed: %w", err)
	}

	// Validate database configuration
	if err := config.NodeDB.Validate(); err != nil {
		return fmt.Errorf("node_db validation failed: %w", err)
	}
	if err := config.ImportDB.Validate(); err != nil {
		return fmt.Errorf("import_db validation failed: %w", err)
	}
	if err := config.SQLite.Validate(); err != nil {
		return fmt.Errorf("sqlite validation failed: %w", err)
	}

	// Validate diagnostics configuration
	if err := config.Insight.Validate(); err != nil {
		return fmt.Errorf("insight validation failed: %w", err)
	}
	if err := config.Perf.Validate(); err != nil {
		return fmt.Errorf("perf validation failed: %w", err)
	}

	// Validate voting configuration
	if err := config.Voting.Validate(); err != nil {
		return fmt.Errorf("voting validation failed: %w", err)
	}

	// Validate misc settings
	if err := validateMiscSettings(config); err != nil {
		return fmt.Errorf("misc settings validation failed: %w", err)
	}

	// Validate crawl configuration
	if err := config.Crawl.Validate(); err != nil {
		return fmt.Errorf("crawl validation failed: %w", err)
	}
	if err := config.VL.Validate(); err != nil {
		return fmt.Errorf("vl validation failed: %w", err)
	}

	// Validate validators configuration
	if err := config.Validators.Validate(); err != nil {
		return fmt.Errorf("validators validation failed: %w", err)
	}

	// Validate overlay and transaction queue
	if err := config.Overlay.Validate(); err != nil {
		return fmt.Errorf("overlay validation failed: %w", err)
	}
	if err := config.TransactionQueue.Validate(); err != nil {
		return fmt.Errorf("transaction_queue validation failed: %w", err)
	}

	// Cross-validation checks
	if err := validateCrossReferences(config); err != nil {
		return fmt.Errorf("cross-validation failed: %w", err)
	}

	return nil
}

// validateServerConfig validates the server configuration
func validateServerConfig(server *ServerConfig) error {
	// Check that at least one port is specified
	if len(server.Ports) == 0 {
		return fmt.Errorf("at least one port must be specified in server.ports")
	}

	// Validate default values if specified
	if server.Port != 0 && (server.Port < 1 || server.Port > 65535) {
		return fmt.Errorf("server default port must be between 1 and 65535, got %d", server.Port)
	}

	return nil
}

// validatePorts validates all port configurations
func validatePorts(ports map[string]PortConfig) error {
	if len(ports) == 0 {
		return fmt.Errorf("no ports configured")
	}

	usedPorts := make(map[string]string) // port -> portName mapping
	peerPortCount := 0

	for portName, portConfig := range ports {
		// Validate individual port
		if err := portConfig.Validate(); err != nil {
			return fmt.Errorf("port %s validation failed: %w", portName, err)
		}

		// Check for port conflicts
		portKey := fmt.Sprintf("%s:%d", portConfig.IP, portConfig.Port)
		if existingPort, exists := usedPorts[portKey]; exists {
			return fmt.Errorf("port conflict: both %s and %s are trying to use %s", existingPort, portName, portKey)
		}
		usedPorts[portKey] = portName

		// Count peer ports (only one allowed)
		if portConfig.HasPeer() {
			peerPortCount++
		}
	}

	// Validate peer port constraint
	if peerPortCount > 1 {
		return fmt.Errorf("only one port may be configured to support the peer protocol, found %d", peerPortCount)
	}

	return nil
}

// validatePeerProtocol validates peer protocol settings
func validatePeerProtocol(config *Config) error {
	if err := ValidatePeersMax(config.PeersMax); err != nil {
		return err
	}
	if err := ValidatePeerPrivate(config.PeerPrivate); err != nil {
		return err
	}
	if err := ValidateMaxTransactions(config.MaxTransactions); err != nil {
		return err
	}

	// Validate IPs format
	for i, ip := range config.IPs {
		if err := validateIPEntry(ip); err != nil {
			return fmt.Errorf("invalid IP entry at index %d: %w", i, err)
		}
	}

	// Validate fixed IPs format (must include port)
	for i, ip := range config.IPsFixed {
		if err := validateFixedIPEntry(ip); err != nil {
			return fmt.Errorf("invalid fixed IP entry at index %d: %w", i, err)
		}
	}

	return nil
}

// validateRippleProtocol validates ripple protocol settings
func validateRippleProtocol(config *Config) error {
	if err := ValidateRelayProposals(config.RelayProposals); err != nil {
		return err
	}
	if err := ValidateRelayValidations(config.RelayValidations); err != nil {
		return err
	}

	// Validate ledger history
	if _, err := config.GetLedgerHistory(); err != nil {
		return fmt.Errorf("invalid ledger_history: %w", err)
	}

	// Validate fetch depth
	if _, err := config.GetFetchDepth(); err != nil {
		return fmt.Errorf("invalid fetch_depth: %w", err)
	}

	// Validate network ID
	if _, err := config.GetNetworkID(); err != nil {
		return fmt.Errorf("invalid network_id: %w", err)
	}

	// Validate path search settings
	if err := ValidatePathSearch(config.PathSearch); err != nil {
		return err
	}
	if err := ValidatePathSearchFast(config.PathSearchFast); err != nil {
		return err
	}
	if err := ValidatePathSearchMax(config.PathSearchMax); err != nil {
		return err
	}
	if err := ValidatePathSearchOld(config.PathSearchOld); err != nil {
		return err
	}

	// Validate worker settings
	if err := ValidateWorkers(config.Workers); err != nil {
		return err
	}
	if err := ValidateIOWorkers(config.IOWorkers); err != nil {
		return err
	}
	if err := ValidatePrefetchWorkers(config.PrefetchWorkers); err != nil {
		return err
	}

	// Validate other settings
	if err := ValidateLedgerReplay(config.LedgerReplay); err != nil {
		return err
	}

	return nil
}

// validateMiscSettings validates miscellaneous settings
func validateMiscSettings(config *Config) error {
	if err := ValidateNodeSize(config.NodeSize); err != nil {
		return err
	}
	if err := ValidateSSLVerify(config.SSLVerify); err != nil {
		return err
	}
	if err := ValidateBetaRPCAPI(config.BetaRPCAPI); err != nil {
		return err
	}
	if err := ValidateWebsocketPingFrequency(config.WebsocketPingFrequency); err != nil {
		return err
	}

	return nil
}

// validateCrossReferences validates cross-references between different config sections
func validateCrossReferences(config *Config) error {
	// Validate that online_delete is not less than ledger_history
	ledgerHistory, err := config.GetLedgerHistory()
	if err != nil {
		return err
	}

	if config.NodeDB.OnlineDelete > 0 && ledgerHistory > 0 && config.NodeDB.OnlineDelete < ledgerHistory {
		return fmt.Errorf("online_delete (%d) must be greater than or equal to ledger_history (%d)", 
			config.NodeDB.OnlineDelete, ledgerHistory)
	}

	// Validate that if validation is configured, appropriate ports are available
	if config.IsValidator() {
		// Should have at least one peer port for validation
		_, _, hasPeerPort := config.GetPeerPort()
		if !hasPeerPort {
			return fmt.Errorf("validator configuration requires a peer port to be configured")
		}
	}

	// Validate RPC startup commands format
	for i, cmd := range config.RPCStartup {
		if _, hasCommand := cmd["command"]; !hasCommand {
			return fmt.Errorf("rpc_startup command at index %d missing 'command' field", i)
		}
	}

	return nil
}

// validateIPEntry validates an IP entry from the [ips] section
func validateIPEntry(entry string) error {
	if entry == "" {
		return fmt.Errorf("IP entry cannot be empty")
	}

	// Entry can be just an IP or IP with port
	parts := strings.Fields(entry)
	if len(parts) > 2 || len(parts) == 0 {
		return fmt.Errorf("invalid format, expected 'IP [port]'")
	}

	// Basic IP validation would go here
	// For now, just check it's not empty
	if parts[0] == "" {
		return fmt.Errorf("IP address cannot be empty")
	}

	// If port is specified, validate it
	if len(parts) == 2 {
		if err := validatePortString(parts[1]); err != nil {
			return fmt.Errorf("invalid port: %w", err)
		}
	}

	return nil
}

// validateFixedIPEntry validates an IP entry from the [ips_fixed] section
func validateFixedIPEntry(entry string) error {
	if entry == "" {
		return fmt.Errorf("fixed IP entry cannot be empty")
	}

	// Fixed IP entries must include a port
	parts := strings.Fields(entry)
	if len(parts) != 2 {
		return fmt.Errorf("fixed IP entries must include a port, expected 'IP port'")
	}

	if parts[0] == "" {
		return fmt.Errorf("IP address cannot be empty")
	}

	if err := validatePortString(parts[1]); err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	return nil
}

// validatePortString validates a port number in string format
func validatePortString(portStr string) error {
	if portStr == "" {
		return fmt.Errorf("port cannot be empty")
	}

	// Simple numeric validation
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return fmt.Errorf("port must be numeric: %w", err)
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", port)
	}

	return nil
}

// ValidateConfigPaths validates that configuration file paths are accessible
func ValidateConfigPaths(paths ConfigPaths) error {
	// Check main config file
	if paths.Main == "" {
		return fmt.Errorf("main config path cannot be empty")
	}

	// Validators path can be empty (optional)
	return nil
}