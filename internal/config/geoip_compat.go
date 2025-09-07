package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GeoIPConfig represents a parsed GeoIP.conf file.
type GeoIPConfig struct {
	LicenseKey        string
	DatabaseDirectory string
	Host              string
	Proxy             string
	ProxyUserPassword string
	LockFile          string
	RetryFor          string
	EditionIDs        []string
	AccountID         int
	PreserveFileTimes int
	Parallelism       int
}

// DefaultGeoIPPaths returns the default paths where GeoIP.conf might be located.
func DefaultGeoIPPaths() []string {
	paths := []string{
		"/etc/GeoIP.conf",
		"/usr/local/etc/GeoIP.conf",
	}

	if homeDir, err := os.UserHomeDir(); err == nil {
		userPath := filepath.Join(homeDir, ".config", "maxminddb-mcp", "GeoIP.conf")
		paths = append(paths, userPath)
	}

	return paths
}

// ParseGeoIPConfig parses a GeoIP.conf file.
func ParseGeoIPConfig(path string) (*GeoIPConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	config := &GeoIPConfig{
		Host:        "https://updates.maxmind.com",
		Parallelism: 1,
	}

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first space
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue // Skip malformed lines
		}

		key := parts[0]
		value := strings.TrimSpace(parts[1])

		switch key {
		case "AccountID", "UserId": // UserId is deprecated but still supported
			if accountID, err := strconv.Atoi(value); err == nil {
				config.AccountID = accountID
			}
		case "LicenseKey":
			config.LicenseKey = value
		case "EditionIDs", "ProductIds": // ProductIds is deprecated but still supported
			config.EditionIDs = strings.Fields(value)
		case "DatabaseDirectory":
			config.DatabaseDirectory = value
		case "Host":
			config.Host = value
		case "Proxy":
			config.Proxy = value
		case "ProxyUserPassword":
			config.ProxyUserPassword = value
		case "PreserveFileTimes":
			if preserve, err := strconv.Atoi(value); err == nil {
				config.PreserveFileTimes = preserve
			}
		case "LockFile":
			config.LockFile = value
		case "RetryFor":
			config.RetryFor = value
		case "Parallelism":
			if parallelism, err := strconv.Atoi(value); err == nil {
				config.Parallelism = parallelism
			}
		default:
			// Ignore unknown configuration keys
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return config, nil
}

// loadGeoIPConfig loads a GeoIP.conf file and converts it to our config format.
func loadGeoIPConfig(path string, config *Config) error {
	geoipConfig, err := ParseGeoIPConfig(path)
	if err != nil {
		return err
	}

	// Convert GeoIP config to our format
	config.Mode = ModeGeoIPCompat
	config.GeoIPCompat.ConfigPath = path

	// Map GeoIP settings to MaxMind config for compatibility
	config.MaxMind.AccountID = geoipConfig.AccountID
	config.MaxMind.LicenseKey = geoipConfig.LicenseKey
	config.MaxMind.Editions = geoipConfig.EditionIDs

	// Use database directory from GeoIP.conf if specified
	if geoipConfig.DatabaseDirectory != "" {
		config.MaxMind.DatabaseDir = geoipConfig.DatabaseDirectory
		config.GeoIPCompat.DatabaseDir = geoipConfig.DatabaseDirectory
	}

	// Use custom endpoint if specified
	if geoipConfig.Host != "" && geoipConfig.Host != "https://updates.maxmind.com" {
		config.MaxMind.Endpoint = geoipConfig.Host
	}

	return nil
}

// ConvertGeoIPToTOML converts a GeoIP.conf file to TOML format.
func ConvertGeoIPToTOML(geoipPath string) (*Config, error) {
	geoipConfig, err := ParseGeoIPConfig(geoipPath)
	if err != nil {
		return nil, err
	}

	config := DefaultConfig()
	config.Mode = ModeMaxMind

	// Convert settings
	config.MaxMind.AccountID = geoipConfig.AccountID
	config.MaxMind.LicenseKey = geoipConfig.LicenseKey
	config.MaxMind.Editions = geoipConfig.EditionIDs

	if geoipConfig.DatabaseDirectory != "" {
		config.MaxMind.DatabaseDir = geoipConfig.DatabaseDirectory
	}

	if geoipConfig.Host != "" {
		config.MaxMind.Endpoint = geoipConfig.Host
	}

	return config, nil
}
