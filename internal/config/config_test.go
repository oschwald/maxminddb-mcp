package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Mode != "maxmind" {
		t.Errorf("Expected default mode to be 'maxmind', got %s", cfg.Mode)
	}

	if !cfg.AutoUpdate {
		t.Error("Expected default auto_update to be true")
	}

	if cfg.UpdateInterval != "24h" {
		t.Errorf("Expected default update_interval to be '24h', got %s", cfg.UpdateInterval)
	}

	if cfg.IteratorTTL != "10m" {
		t.Errorf("Expected default iterator_ttl to be '10m', got %s", cfg.IteratorTTL)
	}

	if cfg.MaxMind.Endpoint != "https://updates.maxmind.com" {
		t.Errorf(
			"Expected default endpoint to be 'https://updates.maxmind.com', got %s",
			cfg.MaxMind.Endpoint,
		)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid maxmind config",
			config: &Config{
				Mode:                    "maxmind",
				UpdateInterval:          "24h",
				IteratorTTL:             "10m",
				IteratorCleanupInterval: "1m",
				MaxMind: MaxMindConfig{
					AccountID:   12345,
					LicenseKey:  "abcdefghijklmnopqrstuvwxyz0123456789_mmk",
					Editions:    []string{"GeoLite2-City"},
					DatabaseDir: "/tmp/db",
				},
			},
			expectError: false,
		},
		{
			name:        "invalid mode",
			config:      &Config{Mode: "invalid"},
			expectError: true,
			errorMsg:    "invalid mode: invalid (must be maxmind, directory, or geoip_compat)",
		},
		{
			name: "maxmind missing account_id",
			config: &Config{
				Mode:                    "maxmind",
				UpdateInterval:          "24h",
				IteratorTTL:             "10m",
				IteratorCleanupInterval: "1m",
				MaxMind: MaxMindConfig{
					LicenseKey:  "abcdefghijklmnopqrstuvwxyz0123456789_mmk",
					Editions:    []string{"GeoLite2-City"},
					DatabaseDir: "/tmp/db",
				},
			},
			expectError: true,
			errorMsg:    "maxmind mode requires account_id",
		},
		{
			name: "maxmind missing license_key",
			config: &Config{
				Mode:                    "maxmind",
				UpdateInterval:          "24h",
				IteratorTTL:             "10m",
				IteratorCleanupInterval: "1m",
				MaxMind: MaxMindConfig{
					AccountID:   12345,
					Editions:    []string{"GeoLite2-City"},
					DatabaseDir: "/tmp/db",
				},
			},
			expectError: true,
			errorMsg:    "maxmind mode requires license_key",
		},
		{
			name: "directory mode valid",
			config: &Config{
				Mode:                    "directory",
				UpdateInterval:          "24h",
				IteratorTTL:             "10m",
				IteratorCleanupInterval: "1m",
				Directory: DirectoryConfig{
					Paths: []string{"/tmp/mmdb"},
				},
			},
			expectError: false,
		},
		{
			name: "directory mode missing paths",
			config: &Config{
				Mode:                    "directory",
				UpdateInterval:          "24h",
				IteratorTTL:             "10m",
				IteratorCleanupInterval: "1m",
			},
			expectError: true,
			errorMsg:    "directory mode requires at least one path",
		},
		{
			name: "invalid duration",
			config: &Config{
				Mode:                    "directory",
				UpdateInterval:          "invalid",
				IteratorTTL:             "10m",
				IteratorCleanupInterval: "1m",
				Directory: DirectoryConfig{
					Paths: []string{"/tmp/mmdb"},
				},
			},
			expectError: true,
			errorMsg:    "invalid update_interval: time: invalid duration \"invalid\"",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.Validate()

			if test.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", test.errorMsg)
					return
				}
				if test.errorMsg != "" && err.Error() != test.errorMsg {
					t.Errorf("Expected error '%s', got '%s'", test.errorMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	}
}

func TestExpandPaths(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get user home directory")
	}

	cfg := &Config{
		MaxMind: MaxMindConfig{
			DatabaseDir: "~/databases",
		},
		Directory: DirectoryConfig{
			Paths: []string{"~/mmdb", "/absolute/path"},
		},
		GeoIPCompat: GeoIPCompatConfig{
			ConfigPath: "~/geoip.conf",
		},
	}

	err = cfg.ExpandPaths()
	if err != nil {
		t.Fatalf("ExpandPaths failed: %v", err)
	}

	expectedDBDir := filepath.Join(homeDir, "databases")
	if cfg.MaxMind.DatabaseDir != expectedDBDir {
		t.Errorf("Expected database dir %s, got %s", expectedDBDir, cfg.MaxMind.DatabaseDir)
	}

	expectedPath := filepath.Join(homeDir, "mmdb")
	if cfg.Directory.Paths[0] != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, cfg.Directory.Paths[0])
	}

	if cfg.Directory.Paths[1] != "/absolute/path" {
		t.Errorf("Expected absolute path unchanged, got %s", cfg.Directory.Paths[1])
	}

	expectedConfigPath := filepath.Join(homeDir, "geoip.conf")
	if cfg.GeoIPCompat.ConfigPath != expectedConfigPath {
		t.Errorf("Expected config path %s, got %s", expectedConfigPath, cfg.GeoIPCompat.ConfigPath)
	}
}

func TestConfigParseDurations(t *testing.T) {
	cfg := &Config{
		Mode:                    "directory",
		UpdateInterval:          "1h30m",
		IteratorTTL:             "5m",
		IteratorCleanupInterval: "30s",
		Directory: DirectoryConfig{
			Paths: []string{"/tmp"},
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	expectedUpdate := 90 * time.Minute
	if cfg.UpdateIntervalDuration != expectedUpdate {
		t.Errorf("Expected update interval %v, got %v", expectedUpdate, cfg.UpdateIntervalDuration)
	}

	expectedTTL := 5 * time.Minute
	if cfg.IteratorTTLDuration != expectedTTL {
		t.Errorf("Expected iterator TTL %v, got %v", expectedTTL, cfg.IteratorTTLDuration)
	}

	expectedCleanup := 30 * time.Second
	if cfg.IteratorCleanupIntervalDuration != expectedCleanup {
		t.Errorf(
			"Expected cleanup interval %v, got %v",
			expectedCleanup,
			cfg.IteratorCleanupIntervalDuration,
		)
	}
}

func TestValidateLicenseKey(t *testing.T) {
	for _, test := range []struct {
		name, key string
		valid     bool
	}{
		{"modern", "abcdefghijklmnopqrstuvwxyz0123456789_mmk", true},
		{"modern with underscores", "ABC_def_0123456789_ABCDEFGHIJKLMNOPQ_mmk", true},
		{"longer than today", "abcdefghijklmnopqrstuvwxyz0123456789ABCD_mmk", true},
		{"legacy", "AbCdEf123456", false},
		{"short with suffix", "abcdefghij_mmk", false},
		{"missing suffix", "abcdefghijklmnopqrstuvwxyz0123456789ABCD", false},
		{"invalid character", "abcdefghijklmnopqrstuvwxyz-123456789_mmk", false},
		{"surrounding space", " abcdefghijklmnopqrstuvwxyz0123456789_mmk", false},
		{"trailing newline", "abcdefghijklmnopqrstuvwxyz0123456789_mmk\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateLicenseKey(test.key)
			if test.valid != (err == nil) {
				t.Fatalf("validateLicenseKey(%q) = %v, want valid %v", test.key, err, test.valid)
			}
			if err != nil {
				if !strings.Contains(err.Error(), "license_key") {
					t.Errorf("unclear error: %v", err)
				}
				if strings.Contains(err.Error(), test.key) {
					t.Error("error exposed the license key")
				}
			}
		})
	}
}
