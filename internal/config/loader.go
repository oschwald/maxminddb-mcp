package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Paths returns the list of configuration file paths to search.
func Paths() []string {
	paths := []string{}

	// 1. Environment variable (standardized for this project name)
	if configPath := os.Getenv("MAXMINDDB_MCP_CONFIG"); configPath != "" {
		return []string{configPath}
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
	if _, ok := os.LookupEnv("MAXMINDDB_MCP_DOWNLOAD_ENABLED"); ok {
		if os.Getenv("MAXMINDDB_MCP_CONFIG") != "" {
			return nil, errors.New(
				"cannot combine MAXMINDDB_MCP_CONFIG with MAXMINDDB_MCP_DOWNLOAD_ENABLED: choose a config file or extension settings",
			)
		}
		return loadEnvironment()
	}
	config := DefaultConfig()

	configPaths := Paths()
	var foundConfig bool
	var configPath string

	// Try each config path
	for _, path := range configPaths {
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) && os.Getenv("MAXMINDDB_MCP_CONFIG") == "" {
				continue
			}
			return nil, fmt.Errorf("read configuration %s: %w", path, err)
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
			return nil, fmt.Errorf(
				"unsupported configuration file %s: use a .toml file or GeoIP.conf",
				path,
			)
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

	if err := toml.NewDecoder(bytes.NewReader(data)).
		DisallowUnknownFields().
		Decode(config); err != nil {
		var missing *toml.StrictMissingError
		if errors.As(err, &missing) {
			fields := make([]string, len(missing.Errors))
			for i, field := range missing.Errors {
				fields[i] = strings.Join(field.Key(), ".")
			}
			return fmt.Errorf("unknown TOML settings %s: %w", strings.Join(fields, ", "), err)
		}
		return err
	}
	if config.Mode != ModeGeoIPCompat {
		return nil
	}
	if err := config.ExpandPaths(); err != nil {
		return err
	}
	paths := DefaultGeoIPPaths()
	if config.GeoIPCompat.ConfigPath != "" {
		paths = []string{config.GeoIPCompat.ConfigPath}
	}
	overrideDir := config.GeoIPCompat.DatabaseDir
	for _, geoipPath := range paths {
		if _, err := os.Stat(
			geoipPath,
		); errors.Is(err, fs.ErrNotExist) &&
			config.GeoIPCompat.ConfigPath == "" {
			continue
		}
		if err := loadGeoIPConfig(geoipPath, config); err != nil {
			return fmt.Errorf("load GeoIP.conf %s: %w", geoipPath, err)
		}
		if overrideDir != "" {
			config.MaxMind.DatabaseDir = overrideDir
		}
		config.GeoIPCompat.DatabaseDir = config.MaxMind.DatabaseDir
		return nil
	}
	return errors.New("geoip_compat requires a GeoIP.conf file; set geoip_compat.config_path")
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
