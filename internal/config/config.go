// Package config provides configuration management for the MaxMindDB MCP server.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Mode constants for configuration.
const (
	ModeMaxMind     = "maxmind"
	ModeDirectory   = "directory"
	ModeGeoIPCompat = "geoip_compat"
)

// Config represents the application configuration.
type Config struct {
	GeoIPCompat                     GeoIPCompatConfig `toml:"geoip_compat"`
	Mode                            string            `toml:"mode"`
	UpdateInterval                  string            `toml:"update_interval"`
	IteratorTTL                     string            `toml:"iterator_ttl"`
	IteratorCleanupInterval         string            `toml:"iterator_cleanup_interval"`
	Directory                       DirectoryConfig   `toml:"directory"`
	MaxMind                         MaxMindConfig     `toml:"maxmind"`
	UpdateIntervalDuration          time.Duration     `toml:"-"`
	IteratorTTLDuration             time.Duration     `toml:"-"`
	IteratorCleanupIntervalDuration time.Duration     `toml:"-"`
	AutoUpdate                      bool              `toml:"auto_update"`
}

// MaxMindConfig holds configuration for MaxMind database updates.
type MaxMindConfig struct {
	LicenseKey  string   `toml:"license_key"`
	DatabaseDir string   `toml:"database_dir"`
	Endpoint    string   `toml:"endpoint"`
	Editions    []string `toml:"editions"`
	AccountID   int      `toml:"account_id"`
}

// DirectoryConfig holds configuration for directory mode.
type DirectoryConfig struct {
	Paths []string `toml:"paths"`
}

// GeoIPCompatConfig holds configuration for GeoIP.conf compatibility.
type GeoIPCompatConfig struct {
	ConfigPath  string `toml:"config_path"`
	DatabaseDir string `toml:"database_dir"`
}

// DefaultConfig returns a configuration with default values.
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()

	return &Config{
		Mode:                    ModeMaxMind,
		AutoUpdate:              true,
		UpdateInterval:          "24h",
		IteratorTTL:             "10m",
		IteratorCleanupInterval: "1m",
		MaxMind: MaxMindConfig{
			DatabaseDir: filepath.Join(homeDir, ".cache", "maxminddb-mcp", "databases"),
			Endpoint:    "https://updates.maxmind.com",
		},
	}
}

// Validate validates the configuration and parses durations.
func (c *Config) Validate() error {
	// Validate mode
	switch c.Mode {
	case ModeMaxMind, ModeDirectory, ModeGeoIPCompat:
		// Valid modes
	default:
		return fmt.Errorf(
			"invalid mode: %s (must be %s, %s, or %s)",
			c.Mode,
			ModeMaxMind,
			ModeDirectory,
			ModeGeoIPCompat,
		)
	}

	// Parse durations
	var err error
	c.UpdateIntervalDuration, err = time.ParseDuration(c.UpdateInterval)
	if err != nil {
		return fmt.Errorf("invalid update_interval: %w", err)
	}
	if c.UpdateIntervalDuration <= 0 {
		return errors.New("update_interval must be positive")
	}

	c.IteratorTTLDuration, err = time.ParseDuration(c.IteratorTTL)
	if err != nil {
		return fmt.Errorf("invalid iterator_ttl: %w", err)
	}
	if c.IteratorTTLDuration <= 0 {
		return errors.New("iterator_ttl must be positive")
	}

	c.IteratorCleanupIntervalDuration, err = time.ParseDuration(c.IteratorCleanupInterval)
	if err != nil {
		return fmt.Errorf("invalid iterator_cleanup_interval: %w", err)
	}
	if c.IteratorCleanupIntervalDuration <= 0 {
		return errors.New("iterator_cleanup_interval must be positive")
	}

	// Mode-specific validation
	switch c.Mode {
	case ModeMaxMind, ModeGeoIPCompat:
		if c.MaxMind.AccountID <= 0 {
			return errors.New("maxmind mode requires account_id")
		}
		if err := validateLicenseKey(c.MaxMind.LicenseKey); err != nil {
			return err
		}
		if len(c.MaxMind.Editions) == 0 {
			return errors.New("maxmind mode requires at least one edition")
		}
		if c.MaxMind.DatabaseDir == "" {
			return errors.New("maxmind mode requires database_dir")
		}
	case ModeDirectory:
		if len(c.Directory.Paths) == 0 {
			return errors.New("directory mode requires at least one path")
		}
	default:
		// No additional validation for other modes
	}

	return nil
}

// minLicenseKeyLength is the length of a modern MaxMind license key.
// MaxMind may issue longer keys in the future.
const minLicenseKeyLength = 40

// licenseKeySuffix ends every modern MaxMind license key.
const licenseKeySuffix = "_mmk"

// validateLicenseKey rejects legacy MaxMind license keys. The error never
// includes the key itself.
func validateLicenseKey(key string) error {
	if key == "" {
		return errors.New("maxmind mode requires license_key")
	}
	if strings.TrimSpace(key) != key {
		return errors.New("license_key has leading or trailing whitespace")
	}
	valid := len(key) >= minLicenseKeyLength && strings.HasSuffix(key, licenseKeySuffix)
	for i := 0; valid && i < len(key); i++ {
		b := key[i]
		valid = b == '_' || '0' <= b && b <= '9' || 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
	}
	if !valid {
		return errors.New(
			"license_key is not a modern MaxMind license key; " +
				"create a new license key in your MaxMind account " +
				"(modern keys are at least 40 characters and end in _mmk)",
		)
	}
	return nil
}

// ExpandPaths expands ~ in file paths to the user's home directory.
func (c *Config) ExpandPaths() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	// Expand MaxMind database dir
	if c.MaxMind.DatabaseDir != "" {
		c.MaxMind.DatabaseDir = expandPath(c.MaxMind.DatabaseDir, homeDir)
	}

	// Expand GeoIP compat database dir
	if c.GeoIPCompat.DatabaseDir != "" {
		c.GeoIPCompat.DatabaseDir = expandPath(c.GeoIPCompat.DatabaseDir, homeDir)
	}

	// Expand GeoIP compat config path
	if c.GeoIPCompat.ConfigPath != "" {
		c.GeoIPCompat.ConfigPath = expandPath(c.GeoIPCompat.ConfigPath, homeDir)
	}

	// Expand directory paths
	for i, path := range c.Directory.Paths {
		c.Directory.Paths[i] = expandPath(path, homeDir)
	}

	return nil
}

// LoadConfig loads configuration from a TOML file.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()
	if err := loadTOMLConfig(path, cfg); err != nil {
		return nil, err
	}
	if err := cfg.ExpandPaths(); err != nil {
		return nil, err
	}

	// Validate and set defaults
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// expandPath replaces ~ with home directory.
func expandPath(path, homeDir string) string {
	if path == "~" {
		return homeDir
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}
	return path
}
