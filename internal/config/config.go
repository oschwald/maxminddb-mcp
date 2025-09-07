// Package config provides configuration management for the MaxMind MMDB MCP server.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
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
	IteratorBuffer                  int               `toml:"iterator_buffer"`
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
		IteratorBuffer:          100,
		MaxMind: MaxMindConfig{
			DatabaseDir: filepath.Join(homeDir, ".cache", "maxminddb-mcp", "databases"),
			Endpoint:    "https://updates.maxmind.com",
		},
		GeoIPCompat: GeoIPCompatConfig{
			DatabaseDir: filepath.Join(homeDir, ".cache", "maxminddb-mcp", "databases"),
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

	c.IteratorTTLDuration, err = time.ParseDuration(c.IteratorTTL)
	if err != nil {
		return fmt.Errorf("invalid iterator_ttl: %w", err)
	}

	c.IteratorCleanupIntervalDuration, err = time.ParseDuration(c.IteratorCleanupInterval)
	if err != nil {
		return fmt.Errorf("invalid iterator_cleanup_interval: %w", err)
	}

	// Validate and clamp IteratorBuffer
	if c.IteratorBuffer <= 0 {
		c.IteratorBuffer = 100 // Default buffer size
	}

	// Mode-specific validation
	switch c.Mode {
	case ModeMaxMind:
		if c.MaxMind.AccountID == 0 {
			return errors.New("maxmind mode requires account_id")
		}
		if c.MaxMind.LicenseKey == "" {
			return errors.New("maxmind mode requires license_key")
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
	case ModeGeoIPCompat:
		// Config path is optional, will search default locations
		if c.GeoIPCompat.DatabaseDir == "" {
			return errors.New("geoip_compat mode requires database_dir")
		}
	default:
		// No additional validation for other modes
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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	// Validate and set defaults
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
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
