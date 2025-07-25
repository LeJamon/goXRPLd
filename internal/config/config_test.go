package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Create temporary directory for test files
	tempDir, err := os.MkdirTemp("", "xrpld_config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test main config file
	mainConfigContent := `
[server]
ports = ["port_test"]

[port_test]
port = 8080
ip = "127.0.0.1"
protocol = "http"

[node_db]
type = "NuDB"
path = "/tmp/test/db"

validators_file = "test_validators.toml"
`

	mainConfigPath := filepath.Join(tempDir, "test_config.toml")
	err = os.WriteFile(mainConfigPath, []byte(mainConfigContent), 0644)
	require.NoError(t, err)

	// Create test validators file
	validatorsContent := `
validator_list_sites = ["https://test.example.com"]
validator_list_keys = ["ED264807102805220DA0F312E71FC2C69E1552C9C5790F6C25E3729DEB573D58"]
validator_list_threshold = 1
`

	validatorsPath := filepath.Join(tempDir, "test_validators.toml")
	err = os.WriteFile(validatorsPath, []byte(validatorsContent), 0644)
	require.NoError(t, err)

	// Test loading configuration
	paths := ConfigPaths{
		Main:       mainConfigPath,
		Validators: validatorsPath,
	}

	config, err := LoadConfig(paths)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify main config was loaded
	assert.Equal(t, []string{"port_test"}, config.Server.Ports)
	assert.Equal(t, "NuDB", config.NodeDB.Type)
	assert.Equal(t, "/tmp/test/db", config.NodeDB.Path)

	// Verify port config was loaded
	portConfig, exists := config.GetPort("port_test")
	assert.True(t, exists)
	assert.Equal(t, 8080, portConfig.Port)
	assert.Equal(t, "127.0.0.1", portConfig.IP)
	assert.Equal(t, "http", portConfig.Protocol)

	// Verify validators config was loaded
	assert.Equal(t, []string{"https://test.example.com"}, config.Validators.ValidatorListSites)
	assert.Equal(t, []string{"ED264807102805220DA0F312E71FC2C69E1552C9C5790F6C25E3729DEB573D58"}, config.Validators.ValidatorListKeys)
	assert.Equal(t, 1, config.Validators.ValidatorListThreshold)
}

func TestConfigValidation(t *testing.T) {
	// Test valid configuration
	config := &Config{
		Server: ServerConfig{
			Ports: []string{"test_port"},
		},
		Ports: map[string]PortConfig{
			"test_port": {
				Port:     8080,
				IP:       "127.0.0.1",
				Protocol: "http",
			},
		},
		NodeDB: NodeDBConfig{
			Type: "NuDB",
			Path: "/tmp/test",
		},
		Validators: ValidatorsConfig{
			ValidatorListSites: []string{"https://example.com"},
			ValidatorListKeys:  []string{"ED264807102805220DA0F312E71FC2C69E1552C9C5790F6C25E3729DEB573D58"},
		},
	}

	err := ValidateConfig(config)
	assert.NoError(t, err)
}

func TestConfigValidationErrors(t *testing.T) {
	// Test invalid port number
	config := &Config{
		Server: ServerConfig{
			Ports: []string{"invalid_port"},
		},
		Ports: map[string]PortConfig{
			"invalid_port": {
				Port:     99999, // Invalid port number
				IP:       "127.0.0.1",
				Protocol: "http",
			},
		},
		NodeDB: NodeDBConfig{
			Type: "NuDB",
			Path: "/tmp/test",
		},
	}

	err := ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "port number must be between 1 and 65535")
}

func TestNetworkDefaults(t *testing.T) {
	// Test mainnet defaults
	mainnetConfig := GetDefaultValidatorsConfig("main")
	assert.Contains(t, mainnetConfig.ValidatorListSites, "https://vl.ripple.com")
	assert.Contains(t, mainnetConfig.ValidatorListKeys, "ED2677ABFFD1B33AC6FBC3062B71F1E8397C1505E1C42C64D11AD1B28FF73F4734")

	// Test testnet defaults
	testnetConfig := GetDefaultValidatorsConfig("testnet")
	assert.Contains(t, testnetConfig.ValidatorListSites, "https://vl.altnet.rippletest.net")
	assert.Contains(t, testnetConfig.ValidatorListKeys, "ED264807102805220DA0F312E71FC2C69E1552C9C5790F6C25E3729DEB573D5860")
}

func TestConfigHelperMethods(t *testing.T) {
	config := &Config{
		NetworkID:     "main",
		LedgerHistory: 1000,
		FetchDepth:    "full",
		NodeDB: NodeDBConfig{
			OnlineDelete: 512,
		},
	}

	// Test network ID parsing
	networkID, err := config.GetNetworkID()
	assert.NoError(t, err)
	assert.Equal(t, 0, networkID)

	// Test ledger history parsing
	ledgerHistory, err := config.GetLedgerHistory()
	assert.NoError(t, err)
	assert.Equal(t, 1000, ledgerHistory)

	// Test fetch depth parsing
	fetchDepth, err := config.GetFetchDepth()
	assert.NoError(t, err)
	assert.Equal(t, -1, fetchDepth) // -1 means "full"
}

func TestPortConfigMethods(t *testing.T) {
	port := PortConfig{
		Port:     8080,
		IP:       "127.0.0.1",
		Protocol: "https,ws",
		Admin:    []string{"127.0.0.1"},
		SSLKey:   "/path/to/key",
		SSLCert:  "/path/to/cert",
	}

	assert.True(t, port.HasHTTPS())
	assert.True(t, port.HasWebSocket())
	assert.True(t, port.IsSecure())
	assert.True(t, port.IsAdminPort())
	assert.True(t, port.HasSSLConfig())
	assert.Equal(t, "127.0.0.1:8080", port.GetBindAddress())
}

func TestValidatorsConfigMethods(t *testing.T) {
	validators := ValidatorsConfig{
		ValidatorListKeys:      []string{"key1", "key2", "key3"},
		ValidatorListThreshold: 0, // Should auto-calculate
	}

	threshold := validators.GetValidatorListThreshold()
	assert.Equal(t, 2, threshold) // floor(3/2) + 1 = 2

	assert.True(t, validators.HasValidatorListKeys())
	assert.Equal(t, 3, validators.GetValidatorListKeyCount())
}

func TestParseValidatorsTxt(t *testing.T) {
	content := `
# This is a comment
[validators]
n9KorY8QtTdRx7TVDpwnG9NvyxsDwHUKUEeDLY3AkiGncVaSXZi5
n9MqiExBcoG19UXwoLjBJnhsxEhAZMuWwJDRdkyDz1EkEkwzQTNt

[validator_list_sites]
https://vl.ripple.com

[validator_list_keys]
ED2677ABFFD1B33AC6FBC3062B71F1E8397C1505E1C42C64D11AD1B28FF73F4734
`

	config, err := ParseValidatorsTxt(content)
	require.NoError(t, err)

	assert.Len(t, config.Validators, 2)
	assert.Contains(t, config.Validators, "n9KorY8QtTdRx7TVDpwnG9NvyxsDwHUKUEeDLY3AkiGncVaSXZi5")
	assert.Contains(t, config.ValidatorListSites, "https://vl.ripple.com")
	assert.Contains(t, config.ValidatorListKeys, "ED2677ABFFD1B33AC6FBC3062B71F1E8397C1505E1C42C64D11AD1B28FF73F4734")
}
