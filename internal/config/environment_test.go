package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func environmentHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, name := range []string{"HOME", "USERPROFILE", "LOCALAPPDATA", "XDG_CACHE_HOME"} {
		t.Setenv(name, home)
	}
	for _, name := range []string{"CONFIG", "DATABASE_DIR", "ACCOUNT_ID", "LICENSE_KEY", "EDITIONS"} {
		t.Setenv("MAXMINDDB_MCP_"+name, "")
	}
	return home
}

func TestEnvironmentDirectoryIgnoresDiscoveredConfig(t *testing.T) {
	home := environmentHome(t)
	path := filepath.Join(home, ".config", "maxminddb-mcp", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("invalid TOML"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAXMINDDB_MCP_DOWNLOAD_ENABLED", "false")
	t.Setenv("MAXMINDDB_MCP_DATABASE_DIR", "~/MMDB files")
	// Saved download settings must not prevent switching to local files.
	t.Setenv("MAXMINDDB_MCP_ACCOUNT_ID", "invalid")
	t.Setenv("MAXMINDDB_MCP_LICENSE_KEY", "unused-secret")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != ModeDirectory || cfg.AutoUpdate || cfg.MaxMind.LicenseKey != "" ||
		!reflect.DeepEqual(cfg.Directory.Paths, []string{filepath.Join(home, "MMDB files")}) {
		t.Fatal("directory settings were not applied independently")
	}
}

func TestEnvironmentDownloads(t *testing.T) {
	environmentHome(t)
	t.Setenv("MAXMINDDB_MCP_DOWNLOAD_ENABLED", "true")
	t.Setenv("MAXMINDDB_MCP_ACCOUNT_ID", "123")
	t.Setenv("MAXMINDDB_MCP_LICENSE_KEY", "test-secret")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != ModeMaxMind || !cfg.AutoUpdate || cfg.MaxMind.AccountID != 123 ||
		cfg.MaxMind.LicenseKey != "test-secret" ||
		cfg.MaxMind.DatabaseDir != filepath.Join(cache, "maxminddb-mcp", "databases") ||
		!reflect.DeepEqual(cfg.MaxMind.Editions, []string{"GeoLite2-City", "GeoLite2-ASN"}) {
		t.Fatal("download settings or defaults were not applied")
	}
	t.Setenv("MAXMINDDB_MCP_EDITIONS", "GeoIP2-City, GeoIP2-ISP\nGeoIP2-Anonymous-IP")
	t.Setenv("MAXMINDDB_MCP_DATABASE_DIR", "~/custom downloads")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxMind.DatabaseDir != filepath.Join(home, "custom downloads") ||
		!reflect.DeepEqual(
			cfg.MaxMind.Editions,
			[]string{"GeoIP2-City", "GeoIP2-ISP", "GeoIP2-Anonymous-IP"},
		) {
		t.Fatal("custom download settings were not applied")
	}
}

func TestInvalidEnvironment(t *testing.T) {
	for _, tc := range []struct{ name, field, value, want string }{
		{"empty toggle", "DOWNLOAD_ENABLED", "", "true or false"},
		{"invalid toggle", "DOWNLOAD_ENABLED", "secret-value", "true or false"},
		{"missing account", "ACCOUNT_ID", "", "account ID"},
		{"invalid account", "ACCOUNT_ID", "secret-value", "account ID"},
		{"negative account", "ACCOUNT_ID", "-1", "account ID"},
		{"missing key", "LICENSE_KEY", "  ", "license key"},
		{"empty editions", "EDITIONS", ", ,", "at least one edition"},
		{"edition path", "EDITIONS", "../secret-value", "edition IDs"},
		{"Windows edition path", "EDITIONS", `..\secret-value`, "edition IDs"},
		{"explicit config", "CONFIG", "missing.toml", "cannot combine"},
		{"missing directory", "DOWNLOAD_ENABLED", "false", "folder containing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			environmentHome(t)
			t.Setenv("MAXMINDDB_MCP_DOWNLOAD_ENABLED", "true")
			t.Setenv("MAXMINDDB_MCP_ACCOUNT_ID", "123")
			t.Setenv("MAXMINDDB_MCP_LICENSE_KEY", "secret-value")
			t.Setenv("MAXMINDDB_MCP_"+tc.field, tc.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Fatal("error exposed a setting value")
			}
		})
	}
}
