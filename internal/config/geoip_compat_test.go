package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const testMaxMindEndpoint = "https://updates.maxmind.com"

func TestParseGeoIPConfig(t *testing.T) {
	// Create a temporary GeoIP.conf file
	content := `# Test GeoIP.conf
AccountID 123456
LicenseKey test_license_key
EditionIDs GeoLite2-Country GeoLite2-City GeoLite2-ASN
DatabaseDirectory /var/lib/GeoIP
Host https://updates.maxmind.com
Parallelism 4
PreserveFileTimes 1
`

	tmpfile, err := os.CreateTemp(t.TempDir(), "geoip_test*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Parse the config
	geoipConfig, err := ParseGeoIPConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to parse GeoIP config: %v", err)
	}

	// Validate parsed values
	if geoipConfig.AccountID != 123456 {
		t.Errorf("Expected AccountID 123456, got %d", geoipConfig.AccountID)
	}

	if geoipConfig.LicenseKey != "test_license_key" {
		t.Errorf("Expected LicenseKey 'test_license_key', got '%s'", geoipConfig.LicenseKey)
	}

	expectedEditions := []string{"GeoLite2-Country", "GeoLite2-City", "GeoLite2-ASN"}
	if len(geoipConfig.EditionIDs) != len(expectedEditions) {
		t.Errorf("Expected %d editions, got %d", len(expectedEditions), len(geoipConfig.EditionIDs))
	}
	for i, edition := range expectedEditions {
		if i >= len(geoipConfig.EditionIDs) || geoipConfig.EditionIDs[i] != edition {
			t.Errorf("Expected edition[%d] '%s', got '%s'", i, edition, geoipConfig.EditionIDs[i])
		}
	}

	if geoipConfig.DatabaseDirectory != "/var/lib/GeoIP" {
		t.Errorf(
			"Expected DatabaseDirectory '/var/lib/GeoIP', got '%s'",
			geoipConfig.DatabaseDirectory,
		)
	}

	if geoipConfig.Host != testMaxMindEndpoint {
		t.Errorf("Expected Host 'https://updates.maxmind.com', got '%s'", geoipConfig.Host)
	}

	if geoipConfig.Parallelism != 4 {
		t.Errorf("Expected Parallelism 4, got %d", geoipConfig.Parallelism)
	}

	if geoipConfig.PreserveFileTimes != 1 {
		t.Errorf("Expected PreserveFileTimes 1, got %d", geoipConfig.PreserveFileTimes)
	}
}

func TestParseGeoIPConfigLegacy(t *testing.T) {
	// Test with legacy field names
	content := `# Legacy GeoIP.conf
UserId 654321
ProductIds GeoLite2-Country GeoLite2-City
DatabaseDirectory /old/path
`

	tmpfile, err := os.CreateTemp(t.TempDir(), "geoip_legacy_test*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	geoipConfig, err := ParseGeoIPConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to parse legacy GeoIP config: %v", err)
	}

	if geoipConfig.AccountID != 654321 {
		t.Errorf("Expected AccountID from UserId 654321, got %d", geoipConfig.AccountID)
	}

	expectedEditions := []string{"GeoLite2-Country", "GeoLite2-City"}
	if len(geoipConfig.EditionIDs) != len(expectedEditions) {
		t.Errorf("Expected %d editions, got %d", len(expectedEditions), len(geoipConfig.EditionIDs))
	}
}

func TestParseGeoIPConfigWithComments(t *testing.T) {
	content := `# This is a comment
# Another comment

AccountID 111111
# Comment in the middle
LicenseKey key_with_comment
# More comments
EditionIDs GeoLite2-City
`

	tmpfile, err := os.CreateTemp(t.TempDir(), "geoip_comments_test*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	geoipConfig, err := ParseGeoIPConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to parse GeoIP config with comments: %v", err)
	}

	if geoipConfig.AccountID != 111111 {
		t.Errorf("Expected AccountID 111111, got %d", geoipConfig.AccountID)
	}

	if geoipConfig.LicenseKey != "key_with_comment" {
		t.Errorf("Expected LicenseKey 'key_with_comment', got '%s'", geoipConfig.LicenseKey)
	}
}

func TestLoadGeoIPConfig(t *testing.T) {
	content := `AccountID 123456
LicenseKey test_key
EditionIDs GeoLite2-City GeoLite2-Country
DatabaseDirectory /custom/path
Host https://custom.endpoint.com
`

	tmpfile, err := os.CreateTemp(t.TempDir(), "geoip_load_test*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Test loading into our config format
	config := DefaultConfig()
	err = loadGeoIPConfig(tmpfile.Name(), config)
	if err != nil {
		t.Fatalf("Failed to load GeoIP config: %v", err)
	}

	// Verify the config was properly converted
	if config.Mode != ModeGeoIPCompat {
		t.Errorf("Expected mode 'geoip_compat', got '%s'", config.Mode)
	}

	if config.MaxMind.AccountID != 123456 {
		t.Errorf("Expected AccountID 123456, got %d", config.MaxMind.AccountID)
	}

	if config.MaxMind.LicenseKey != "test_key" {
		t.Errorf("Expected LicenseKey 'test_key', got '%s'", config.MaxMind.LicenseKey)
	}

	expectedEditions := []string{"GeoLite2-City", "GeoLite2-Country"}
	if len(config.MaxMind.Editions) != len(expectedEditions) {
		t.Errorf(
			"Expected %d editions, got %d",
			len(expectedEditions),
			len(config.MaxMind.Editions),
		)
	}

	if config.MaxMind.DatabaseDir != "/custom/path" {
		t.Errorf("Expected DatabaseDir '/custom/path', got '%s'", config.MaxMind.DatabaseDir)
	}

	if config.MaxMind.Endpoint != "https://custom.endpoint.com" {
		t.Errorf(
			"Expected Endpoint 'https://custom.endpoint.com', got '%s'",
			config.MaxMind.Endpoint,
		)
	}
}

func TestConvertGeoIPToTOML(t *testing.T) {
	content := `AccountID 987654
LicenseKey convert_test_key
EditionIDs GeoLite2-ASN
DatabaseDirectory /convert/path
`

	tmpfile, err := os.CreateTemp(t.TempDir(), "geoip_convert_test*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	config, err := ConvertGeoIPToTOML(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to convert GeoIP to TOML: %v", err)
	}

	// Verify conversion
	if config.Mode != ModeMaxMind {
		t.Errorf("Expected converted mode 'maxmind', got '%s'", config.Mode)
	}

	if config.MaxMind.AccountID != 987654 {
		t.Errorf("Expected converted AccountID 987654, got %d", config.MaxMind.AccountID)
	}

	if config.MaxMind.LicenseKey != "convert_test_key" {
		t.Errorf(
			"Expected converted LicenseKey 'convert_test_key', got '%s'",
			config.MaxMind.LicenseKey,
		)
	}

	if len(config.MaxMind.Editions) != 1 || config.MaxMind.Editions[0] != "GeoLite2-ASN" {
		t.Errorf("Expected converted editions ['GeoLite2-ASN'], got %v", config.MaxMind.Editions)
	}
}

func TestParseGeoIPConfigFileNotFound(t *testing.T) {
	_, err := ParseGeoIPConfig("/nonexistent/path/GeoIP.conf")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
	if !strings.Contains(err.Error(), "no such file or directory") &&
		!strings.Contains(err.Error(), "cannot find the file") {
		t.Errorf("Expected file not found error, got: %v", err)
	}
}

func TestDefaultGeoIPPaths(t *testing.T) {
	paths := DefaultGeoIPPaths()

	if len(paths) < 2 {
		t.Errorf("Expected at least 2 default paths, got %d", len(paths))
	}

	// Should include system path
	foundSystemPath := false
	for _, path := range paths {
		if path == "/etc/GeoIP.conf" || path == "/usr/local/etc/GeoIP.conf" {
			foundSystemPath = true
			break
		}
	}
	if !foundSystemPath {
		t.Error("Expected system GeoIP.conf path in defaults")
	}

	// Should include user path if home directory available
	homeDir, err := os.UserHomeDir()
	if err == nil {
		expectedUserPath := filepath.Join(homeDir, ".config", "maxminddb-mcp", "GeoIP.conf")
		found := slices.Contains(paths, expectedUserPath)
		if !found {
			t.Errorf("Expected user GeoIP.conf path %s in defaults", expectedUserPath)
		}
	}
}
