package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGeoIPFromTOML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	geoip := filepath.Join(home, "GeoIP.conf")
	if err := os.WriteFile(
		geoip,
		[]byte(
			"AccountID\t123\nLicenseKey test\nEditionIDs GeoLite2-ASN\nDatabaseDirectory ~/geoip\nHost updates.maxmind.com\n",
		),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "config.toml")
	t.Setenv("MAXMINDDB_MCP_CONFIG", path)
	for _, override := range []string{"", "~/override"} {
		content := fmt.Sprintf(
			"mode = 'geoip_compat'\n[geoip_compat]\nconfig_path = '~/GeoIP.conf'\ndatabase_dir = %q\n",
			override,
		)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		expectedDir := filepath.Join(home, "geoip")
		if override != "" {
			expectedDir = filepath.Join(home, "override")
		}
		if cfg.MaxMind.AccountID != 123 || cfg.MaxMind.LicenseKey != "test" ||
			len(cfg.MaxMind.Editions) != 1 ||
			cfg.MaxMind.Editions[0] != "GeoLite2-ASN" ||
			cfg.MaxMind.DatabaseDir != expectedDir ||
			cfg.MaxMind.Endpoint != "https://updates.maxmind.com" {
			t.Fatal("GeoIP.conf settings were not applied")
		}
		if cfg.IteratorTTLDuration <= 0 {
			t.Fatal("default durations were not applied")
		}
	}
	if err := os.Remove(geoip); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("missing referenced GeoIP.conf was ignored")
	}
}

func TestExplicitConfigMustExist(t *testing.T) {
	t.Setenv("MAXMINDDB_MCP_CONFIG", filepath.Join(t.TempDir(), "missing.toml"))
	if _, err := Load(); err == nil {
		t.Fatal("explicit config silently fell back to defaults")
	}
}

func TestPositiveDurations(t *testing.T) {
	for _, field := range []string{"update_interval", "iterator_ttl", "iterator_cleanup_interval"} {
		for _, duration := range []string{"0s", "-1s"} {
			cfg := DefaultConfig()
			cfg.Mode = ModeDirectory
			cfg.Directory.Paths = []string{t.TempDir()}
			switch field {
			case "update_interval":
				cfg.UpdateInterval = duration
			case "iterator_ttl":
				cfg.IteratorTTL = duration
			case "iterator_cleanup_interval":
				cfg.IteratorCleanupInterval = duration
			}
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("%s=%s: %v", field, duration, err)
			}
		}
	}
}

func TestGeoIPCredentialsRequired(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeGeoIPCompat
	if err := cfg.Validate(); err == nil {
		t.Fatal("accepted GeoIP mode without credentials")
	}
}
