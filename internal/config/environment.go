package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// loadEnvironment keeps extension settings independent of discovered config files.
func loadEnvironment() (*Config, error) {
	download, err := strconv.ParseBool(os.Getenv("MAXMINDDB_MCP_DOWNLOAD_ENABLED"))
	if err != nil {
		return nil, errors.New("set MAXMINDDB_MCP_DOWNLOAD_ENABLED to true or false")
	}
	cfg := DefaultConfig()
	dir := os.Getenv("MAXMINDDB_MCP_DATABASE_DIR")
	if !download {
		if dir == "" {
			return nil, errors.New(
				"select the folder containing your MMDB files in extension settings, or set MAXMINDDB_MCP_DATABASE_DIR",
			)
		}
		cfg.Mode = ModeDirectory
		cfg.AutoUpdate = false
		cfg.Directory.Paths = []string{dir}
	} else {
		cfg.MaxMind.AccountID, err = strconv.Atoi(
			strings.TrimSpace(os.Getenv("MAXMINDDB_MCP_ACCOUNT_ID")),
		)
		if err != nil || cfg.MaxMind.AccountID <= 0 {
			return nil, errors.New(
				"enter a positive MaxMind account ID in extension settings, or set MAXMINDDB_MCP_ACCOUNT_ID",
			)
		}
		cfg.MaxMind.LicenseKey = strings.TrimSpace(os.Getenv("MAXMINDDB_MCP_LICENSE_KEY"))
		if cfg.MaxMind.LicenseKey == "" {
			return nil, errors.New(
				"enter your MaxMind license key in extension settings, or set MAXMINDDB_MCP_LICENSE_KEY",
			)
		}
		editions := os.Getenv("MAXMINDDB_MCP_EDITIONS")
		if editions == "" {
			editions = "GeoLite2-City,GeoLite2-ASN"
		}
		cfg.MaxMind.Editions = strings.Fields(strings.ReplaceAll(editions, ",", " "))
		for _, edition := range cfg.MaxMind.Editions {
			if strings.ContainsAny(edition, `/\\`) || edition == "." || edition == ".." {
				return nil, errors.New(
					"database editions must be edition IDs, not paths: use GeoLite2-City,GeoLite2-ASN or your licensed editions",
				)
			}
		}
		if dir == "" {
			cache, cacheErr := os.UserCacheDir()
			if cacheErr != nil {
				return nil, fmt.Errorf("find download folder: %w", cacheErr)
			}
			dir = filepath.Join(cache, "maxminddb-mcp", "databases")
		}
		cfg.MaxMind.DatabaseDir = dir
	}
	if err := cfg.ExpandPaths(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid extension or environment settings: %w", err)
	}
	return cfg, nil
}
