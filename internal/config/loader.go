package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Paths returns the list of configuration file paths to search.
func Paths() []string {
	paths := []string{}

	// 1. Environment variable
	if configPath := os.Getenv("MAXMIND_MCP_CONFIG"); configPath != "" {
		paths = append(paths, configPath)
	}

	// 2. User config directory
	if homeDir, err := os.UserHomeDir(); err == nil {
		userConfig := filepath.Join(homeDir, ".config", "maxminddb-mcp", "config.toml")
		paths = append(paths, userConfig)

		// 4. User GeoIP.conf (for compatibility)
		userGeoIP := filepath.Join(homeDir, ".config", "maxminddb-mcp", "GeoIP.conf")
		paths = append(paths, userGeoIP)
	}

	// 3. System GeoIP.conf (for compatibility)
	paths = append(paths, "/etc/GeoIP.conf")

	return paths
}

// Load loads configuration from the first available config file.
func Load() (*Config, error) {
	config := DefaultConfig()

	configPaths := Paths()
	var foundConfig bool
	var configPath string

	// Try each config path
	for _, path := range configPaths {
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			continue
		}

		// Determine config type by extension
		switch {
		case filepath.Ext(path) == ".toml":
			if err := loadTOMLConfig(path, config); err != nil {
				return nil, fmt.Errorf("failed to load TOML config from %s: %w", path, err)
			}
		case filepath.Base(path) == "GeoIP.conf":
			if err := loadGeoIPConfig(path, config); err != nil {
				return nil, fmt.Errorf("failed to load GeoIP.conf from %s: %w", path, err)
			}
		default:
			continue
		}

		foundConfig = true
		configPath = path
		break
	}

	// If no config file found, use defaults.
	// In maxmind mode without credentials, this will fail validation later.

	// Expand ~ in paths
	if err := config.ExpandPaths(); err != nil {
		return nil, fmt.Errorf("failed to expand paths: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		if foundConfig {
			return nil, fmt.Errorf("invalid configuration in %s: %w", configPath, err)
		}
		return nil, fmt.Errorf("invalid default configuration: %w", err)
	}

	return config, nil
}

// loadTOMLConfig loads configuration from a TOML file.
func loadTOMLConfig(path string, config *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return toml.Unmarshal(data, config)
}

// SaveTOMLConfig saves configuration to a TOML file.
func SaveTOMLConfig(path string, config *Config) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}

	// Marshal to TOML
	data, err := toml.Marshal(config)
	if err != nil {
		return err
	}

	// Write file
	return os.WriteFile(path, data, 0o600)
}

// GenerateDefaultTOMLConfig creates a default TOML config file.
func GenerateDefaultTOMLConfig(path string) error {
	config := DefaultConfig()
	return SaveTOMLConfig(path, config)
}
