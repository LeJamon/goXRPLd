package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// LoadConfig loads configuration from multiple sources in priority order:
// 1. Default values (matching rippled defaults)
// 2. Configuration file (xrpld.toml)
// 3. Environment variables (XRPLD_ prefix)
// 4. Validators file (validators.toml)
func LoadConfig(paths ConfigPaths) (*Config, error) {
	// Create viper instance for main config
	v := viper.New()

	// 1. Set defaults first
	setDefaults(v)

	// 2. Load main configuration file
	if err := loadMainConfig(v, paths.Main); err != nil {
		return nil, fmt.Errorf("failed to load main config: %w", err)
	}

	// Apply network-specific defaults after loading config to know the network
	networkID := v.Get("network_id")
	ApplyNetworkDefaults(v, networkID)

	// 3. Set up environment variable support
	v.SetEnvPrefix("XRPLD")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 4. Unmarshal main config into struct
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 5. Load validators configuration
	validators, err := loadValidatorsConfig(paths.Validators, config.ValidatorsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load validators config: %w", err)
	}
	config.Validators = *validators

	// 6. Process dynamic port configurations
	if err := processPorts(&config, v); err != nil {
		return nil, fmt.Errorf("failed to process ports: %w", err)
	}

	// 7. Store paths for reference
	config.configPath = paths.Main
	config.validatorsPath = paths.Validators

	// 8. Validate the complete configuration
	if err := ValidateConfig(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// loadMainConfig loads the main configuration file
func loadMainConfig(v *viper.Viper, configPath string) error {
	if configPath == "" {
		return fmt.Errorf("config path cannot be empty")
	}

	// Set config file path
	v.SetConfigFile(configPath)

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", configPath)
	}

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	return nil
}

// loadValidatorsConfig loads the validators configuration
// TODO ensure proper parsing of pub key (eg: remove ED prefix)
func loadValidatorsConfig(validatorsPath, validatorsFile string) (*ValidatorsConfig, error) {
	// Determine which file to load
	var filePath string
	if validatorsPath != "" {
		filePath = validatorsPath
	} else if validatorsFile != "" {
		// If validatorsFile is relative, make it relative to config directory
		if !filepath.IsAbs(validatorsFile) {
			// Assume same directory as main config for now
			filePath = validatorsFile
		} else {
			filePath = validatorsFile
		}
	} else {
		// Use default
		filePath = "validators.toml"
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Try alternative formats
		if strings.HasSuffix(filePath, ".toml") {
			// Try .txt format (rippled format)
			txtPath := strings.TrimSuffix(filePath, ".toml") + ".txt"
			if _, err := os.Stat(txtPath); err == nil {
				return loadValidatorsTxtFile(txtPath)
			}
		}

		// If no validators file found, return empty config (not an error for some deployments)
		return &ValidatorsConfig{}, nil
	}

	// Load TOML format
	if strings.HasSuffix(filePath, ".toml") {
		return loadValidatorsTomlFile(filePath)
	}

	// Load TXT format (rippled format)
	if strings.HasSuffix(filePath, ".txt") {
		return loadValidatorsTxtFile(filePath)
	}

	return nil, fmt.Errorf("unsupported validators file format: %s (supported: .toml, .txt)", filePath)
}

// loadValidatorsTomlFile loads validators from TOML format
func loadValidatorsTomlFile(filePath string) (*ValidatorsConfig, error) {
	v := viper.New()
	v.SetConfigFile(filePath)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read validators file %s: %w", filePath, err)
	}

	var validators ValidatorsConfig
	if err := v.Unmarshal(&validators); err != nil {
		return nil, fmt.Errorf("failed to unmarshal validators config: %w", err)
	}

	return &validators, nil
}

// loadValidatorsTxtFile loads validators from TXT format (rippled format)
func loadValidatorsTxtFile(filePath string) (*ValidatorsConfig, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read validators file %s: %w", filePath, err)
	}

	validators, err := ParseValidatorsTxt(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse validators file %s: %w", filePath, err)
	}

	return validators, nil
}

// processPorts processes dynamic port configurations
func processPorts(config *Config, v *viper.Viper) error {
	config.Ports = make(map[string]PortConfig)

	// Get list of ports from server section
	serverPorts := config.Server.Ports
	if len(serverPorts) == 0 {
		// No ports specified in server section - scan for port_* sections
		serverPorts = findPortSections(v)
	}

	// Process each port
	for _, portName := range serverPorts {
		portConfig, err := loadPortConfig(v, portName, config.Server)
		if err != nil {
			return fmt.Errorf("failed to load port config %s: %w", portName, err)
		}
		config.Ports[portName] = portConfig
	}

	return nil
}

// findPortSections scans viper for sections that start with "port_"
func findPortSections(v *viper.Viper) []string {
	var ports []string

	// Get all keys and look for port configurations
	allKeys := v.AllKeys()
	portMap := make(map[string]bool)

	for _, key := range allKeys {
		parts := strings.Split(key, ".")
		if len(parts) >= 2 && strings.HasPrefix(parts[0], "port_") {
			portName := parts[0]
			if !portMap[portName] {
				ports = append(ports, portName)
				portMap[portName] = true
			}
		}
	}

	return ports
}

// loadPortConfig loads configuration for a specific port
func loadPortConfig(v *viper.Viper, portName string, serverDefaults ServerConfig) (PortConfig, error) {
	var portConfig PortConfig

	// Create a sub-viper for this port section
	portViper := v.Sub(portName)
	if portViper == nil {
		return PortConfig{}, fmt.Errorf("no configuration found for port %s", portName)
	}

	// Apply server defaults first
	applyServerDefaults(portViper, serverDefaults)

	// Unmarshal port configuration
	if err := portViper.Unmarshal(&portConfig); err != nil {
		return PortConfig{}, fmt.Errorf("failed to unmarshal port config: %w", err)
	}

	return portConfig, nil
}

// applyServerDefaults applies server-level defaults to a port configuration
func applyServerDefaults(portViper *viper.Viper, serverDefaults ServerConfig) {
	// Apply defaults from server section if not set in port section
	if serverDefaults.Port != 0 && !portViper.IsSet("port") {
		portViper.SetDefault("port", serverDefaults.Port)
	}
	if serverDefaults.IP != "" && !portViper.IsSet("ip") {
		portViper.SetDefault("ip", serverDefaults.IP)
	}
	if serverDefaults.Protocol != "" && !portViper.IsSet("protocol") {
		portViper.SetDefault("protocol", serverDefaults.Protocol)
	}
	if serverDefaults.Limit != 0 && !portViper.IsSet("limit") {
		portViper.SetDefault("limit", serverDefaults.Limit)
	}
	if serverDefaults.User != "" && !portViper.IsSet("user") {
		portViper.SetDefault("user", serverDefaults.User)
	}
	if serverDefaults.Password != "" && !portViper.IsSet("password") {
		portViper.SetDefault("password", serverDefaults.Password)
	}
}

// LoadConfigFromDir loads configuration from a directory containing both files
func LoadConfigFromDir(configDir string) (*Config, error) {
	paths := ConfigPathsFromDir(configDir)
	return LoadConfig(paths)
}

// LoadDefaultConfig loads configuration from default locations
func LoadDefaultConfig() (*Config, error) {
	paths := DefaultConfigPaths()
	return LoadConfig(paths)
}

// ReloadConfig reloads configuration from the same paths
func ReloadConfig(existingConfig *Config) (*Config, error) {
	paths := ConfigPaths{
		Main:       existingConfig.GetConfigPath(),
		Validators: existingConfig.GetValidatorsPath(),
	}
	return LoadConfig(paths)
}

// SaveExampleConfig saves an example configuration file
func SaveExampleConfig(configPath string) error {
	exampleConfig := generateExampleConfig()

	v := viper.New()

	// Set all example values
	for key, value := range exampleConfig {
		v.Set(key, value)
	}

	// Write to file
	v.SetConfigFile(configPath)
	if err := v.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write example config: %w", err)
	}

	return nil
}

// generateExampleConfig generates example configuration values
func generateExampleConfig() map[string]interface{} {
	return map[string]interface{}{
		"server.ports": []string{"port_rpc_admin_local", "port_peer", "port_ws_admin_local"},

		"port_rpc_admin_local.port":     5005,
		"port_rpc_admin_local.ip":       "127.0.0.1",
		"port_rpc_admin_local.protocol": "http",
		"port_rpc_admin_local.admin":    []string{"127.0.0.1"},

		"port_peer.port":     51235,
		"port_peer.ip":       "0.0.0.0",
		"port_peer.protocol": "peer",

		"port_ws_admin_local.port":             6006,
		"port_ws_admin_local.ip":               "127.0.0.1",
		"port_ws_admin_local.protocol":         "ws",
		"port_ws_admin_local.admin":            []string{"127.0.0.1"},
		"port_ws_admin_local.send_queue_limit": 500,

		"node_db.type":            "NuDB",
		"node_db.path":            "/var/lib/xrpld/db/nudb",
		"node_db.online_delete":   512,
		"node_db.advisory_delete": 0,

		"database_path":   "/var/lib/xrpld/db",
		"debug_logfile":   "/var/log/xrpld/debug.log",
		"validators_file": "validators.toml",
		"ssl_verify":      1,
	}
}
