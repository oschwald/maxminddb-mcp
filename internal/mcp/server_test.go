package mcp

import (
	"bytes"
	"encoding/json"
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-mcp/internal/config"
	"github.com/oschwald/maxminddb-mcp/internal/database"
	"github.com/oschwald/maxminddb-mcp/internal/iterator"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	maxmindMode = "maxmind"
	testCityDB  = "../../testdata/test-data/GeoLite2-City-Test.mmdb"
)

func TestNew(t *testing.T) {
	cfg := createTestMCPConfig(t)
	dbManager, err := database.New()
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}
	defer func() { _ = dbManager.Close() }()

	iterMgr := iterator.New(30*time.Minute, 5*time.Minute)
	defer iterMgr.StopCleanup()

	server := New(cfg, dbManager, nil, iterMgr, "test")

	if server == nil {
		t.Fatal("Server should not be nil")
	}

	if server.config != cfg {
		t.Error("Server config should match input")
	}

	if server.dbManager != dbManager {
		t.Error("Server database manager should match input")
	}

	if server.iterMgr != iterMgr {
		t.Error("Server iterator manager should match input")
	}

	if server.mcp == nil {
		t.Error("MCP server should not be nil")
	}

	if server.updater != nil {
		t.Error("Updater should be nil when not provided")
	}
}

func TestNewWithUpdater(t *testing.T) {
	cfg := createTestMCPConfig(t)
	cfg.Mode = maxmindMode
	cfg.MaxMind = config.MaxMindConfig{
		AccountID:   999999,
		LicenseKey:  "test_key",
		Editions:    []string{"GeoLite2-City"},
		DatabaseDir: t.TempDir(),
		Endpoint:    "https://updates.maxmind.com",
	}

	dbManager, err := database.New()
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}
	defer func() { _ = dbManager.Close() }()

	updater, err := database.NewUpdater(cfg, dbManager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	iterMgr := iterator.New(30*time.Minute, 5*time.Minute)
	defer iterMgr.StopCleanup()

	server := New(cfg, dbManager, updater, iterMgr, "test")

	if server.updater != updater {
		t.Error("Server updater should match input")
	}
}

func TestLookupIPInSingleDatabase(t *testing.T) {
	cfg := createTestMCPConfig(t)
	dbManager, err := database.New()
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}
	defer func() { _ = dbManager.Close() }()

	// Load a test database
	testDBPath := testCityDB
	err = dbManager.LoadDatabase(testDBPath)
	if err != nil {
		t.Fatalf("Failed to load test database: %v", err)
	}

	iterMgr := iterator.New(30*time.Minute, 5*time.Minute)
	defer iterMgr.StopCleanup()

	server := New(cfg, dbManager, nil, iterMgr, "test")

	ip, err := netip.ParseAddr("1.1.1.1")
	if err != nil {
		t.Fatalf("Failed to parse IP: %v", err)
	}

	// Test valid database
	result, err := server.lookupIPInSingleDatabase(ip, "1.1.1.1", "GeoLite2-City-Test.mmdb", nil)
	if err != nil {
		t.Fatalf("Failed to lookup IP in single database: %v", err)
	}

	if result == nil {
		t.Error("Result should not be nil")
	}

	// Should have some result indicating successful lookup
	if result == nil {
		t.Error("Result should not be nil for successful lookup")
	}

	// Test non-existent database
	result, err = server.lookupIPInSingleDatabase(ip, "1.1.1.1", "nonexistent.mmdb", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should return some result even for non-existent database
	if result == nil {
		t.Error("Result should not be nil even for non-existent database")
	}
}

func TestLookupIPInAllDatabases(t *testing.T) {
	cfg := createTestMCPConfig(t)
	dbManager, err := database.New()
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}
	defer func() { _ = dbManager.Close() }()

	// Load test databases
	testDBPath1 := testCityDB
	err = dbManager.LoadDatabase(testDBPath1)
	if err != nil {
		t.Fatalf("Failed to load test database 1: %v", err)
	}

	testDBPath2 := "../../testdata/test-data/GeoLite2-ASN-Test.mmdb"
	err = dbManager.LoadDatabase(testDBPath2)
	if err != nil {
		t.Fatalf("Failed to load test database 2: %v", err)
	}

	iterMgr := iterator.New(30*time.Minute, 5*time.Minute)
	defer iterMgr.StopCleanup()

	server := New(cfg, dbManager, nil, iterMgr, "test")

	ip, err := netip.ParseAddr("1.1.1.1")
	if err != nil {
		t.Fatalf("Failed to parse IP: %v", err)
	}

	result, err := server.lookupIPInAllDatabases(ip, "1.1.1.1", nil)
	if err != nil {
		t.Fatalf("Failed to lookup IP in all databases: %v", err)
	}

	if result == nil {
		t.Error("Result should not be nil")
	}

	// Should return a successful result for valid lookup
	// We don't need to check internal structure, just that it doesn't crash
}

func TestServe(t *testing.T) {
	cfg := createTestMCPConfig(t)
	dbManager, err := database.New()
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}
	defer func() { _ = dbManager.Close() }()

	iterMgr := iterator.New(30*time.Minute, 5*time.Minute)
	defer iterMgr.StopCleanup()

	server := New(cfg, dbManager, nil, iterMgr, "test")

	// Test Serve method exists and doesn't panic
	// Note: We can't easily test the actual serving without mocking stdio
	if server.mcp == nil {
		t.Error("MCP server should be available for serving")
	}

	// Test that Serve method exists and is callable
	// We can't actually call it without proper stdin/stdout setup
	// Just verify the method exists by checking it's not nil
	_ = server.Serve // This will cause a compile error if Serve doesn't exist
}

func TestRegisterTools(t *testing.T) {
	cfg := createTestMCPConfig(t)
	dbManager, err := database.New()
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}
	defer func() { _ = dbManager.Close() }()

	iterMgr := iterator.New(30*time.Minute, 5*time.Minute)
	defer iterMgr.StopCleanup()

	// This will call registerTools internally
	server := New(cfg, dbManager, nil, iterMgr, "test")

	if server.mcp == nil {
		t.Error("MCP server should be initialized after registerTools")
	}

	// Test with updater available
	cfg.Mode = maxmindMode
	cfg.MaxMind = config.MaxMindConfig{
		AccountID:   999999,
		LicenseKey:  "test_key",
		Editions:    []string{"GeoLite2-City"},
		DatabaseDir: t.TempDir(),
		Endpoint:    "https://updates.maxmind.com",
	}

	updater, err := database.NewUpdater(cfg, dbManager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	server = New(cfg, dbManager, updater, iterMgr, "test")
	if server.mcp == nil {
		t.Error("MCP server should be initialized with updater tools")
	}
}

func TestServerStructFields(t *testing.T) {
	cfg := createTestMCPConfig(t)
	dbManager, err := database.New()
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}
	defer func() { _ = dbManager.Close() }()

	iterMgr := iterator.New(30*time.Minute, 5*time.Minute)
	defer iterMgr.StopCleanup()

	server := New(cfg, dbManager, nil, iterMgr, "test")

	// Test all fields are set correctly
	if server.config != cfg {
		t.Error("Config field not set correctly")
	}

	if server.dbManager != dbManager {
		t.Error("DbManager field not set correctly")
	}

	if server.iterMgr != iterMgr {
		t.Error("IterMgr field not set correctly")
	}

	if server.mcp == nil {
		t.Error("MCP field should not be nil")
	}

	if server.updater != nil {
		t.Error("Updater field should be nil when not provided")
	}
}

func TestServerMethods(t *testing.T) {
	cfg := createTestMCPConfig(t)
	dbManager, err := database.New()
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}
	defer func() { _ = dbManager.Close() }()

	// Load a test database
	testDBPath := testCityDB
	err = dbManager.LoadDatabase(testDBPath)
	if err != nil {
		t.Fatalf("Failed to load test database: %v", err)
	}

	iterMgr := iterator.New(30*time.Minute, 5*time.Minute)
	defer iterMgr.StopCleanup()

	server := New(cfg, dbManager, nil, iterMgr, "test")

	// Test helper methods exist and work
	ip, _ := netip.ParseAddr("8.8.8.8")

	// Test lookupIPInSingleDatabase
	result, err := server.lookupIPInSingleDatabase(ip, "8.8.8.8", "GeoLite2-City-Test.mmdb", nil)
	if err != nil {
		t.Errorf("lookupIPInSingleDatabase failed: %v", err)
	}
	if result == nil {
		t.Error("lookupIPInSingleDatabase should return a result")
	}

	// Test lookupIPInAllDatabases
	result, err = server.lookupIPInAllDatabases(ip, "8.8.8.8", nil)
	if err != nil {
		t.Errorf("lookupIPInAllDatabases failed: %v", err)
	}
	if result == nil {
		t.Error("lookupIPInAllDatabases should return a result")
	}
}

func TestServerWithDifferentModes(t *testing.T) {
	// Test directory mode
	cfg := createTestMCPConfig(t)
	cfg.Mode = "directory"
	cfg.Directory = config.DirectoryConfig{
		Paths: []string{t.TempDir()},
	}

	dbManager, err := database.New()
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}
	defer func() { _ = dbManager.Close() }()

	iterMgr := iterator.New(30*time.Minute, 5*time.Minute)
	defer iterMgr.StopCleanup()

	server := New(cfg, dbManager, nil, iterMgr, "test")
	if server == nil {
		t.Error("Server should be created for directory mode")
	}

	// Test maxmind mode
	cfg.Mode = maxmindMode
	cfg.MaxMind = config.MaxMindConfig{
		AccountID:   999999,
		LicenseKey:  "test_key",
		Editions:    []string{"GeoLite2-City"},
		DatabaseDir: t.TempDir(),
		Endpoint:    "https://updates.maxmind.com",
	}

	server = New(cfg, dbManager, nil, iterMgr, "test")
	if server == nil {
		t.Error("Server should be created for maxmind mode")
	}

	// Test geoip_compat mode
	cfg.Mode = "geoip_compat"
	cfg.GeoIPCompat = config.GeoIPCompatConfig{
		ConfigPath:  "/path/to/GeoIP.conf",
		DatabaseDir: t.TempDir(),
	}

	server = New(cfg, dbManager, nil, iterMgr, "test")
	if server == nil {
		t.Error("Server should be created for geoip_compat mode")
	}
}

func TestServerConcurrency(t *testing.T) {
	cfg := createTestMCPConfig(t)
	dbManager, err := database.New()
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}
	defer func() { _ = dbManager.Close() }()

	// Load test database
	testDBPath := testCityDB
	err = dbManager.LoadDatabase(testDBPath)
	if err != nil {
		t.Fatalf("Failed to load test database: %v", err)
	}

	iterMgr := iterator.New(30*time.Minute, 5*time.Minute)
	defer iterMgr.StopCleanup()

	server := New(cfg, dbManager, nil, iterMgr, "test")

	// Test concurrent access to server methods
	done := make(chan bool, 3)
	ip, _ := netip.ParseAddr("1.1.1.1")

	go func() {
		defer func() { done <- true }()
		_, err := server.lookupIPInSingleDatabase(ip, "1.1.1.1", "GeoLite2-City-Test.mmdb", nil)
		if err != nil {
			t.Errorf("Concurrent lookup failed: %v", err)
		}
	}()

	go func() {
		defer func() { done <- true }()
		_, err := server.lookupIPInAllDatabases(ip, "1.1.1.1", nil)
		if err != nil {
			t.Errorf("Concurrent lookup failed: %v", err)
		}
	}()

	go func() {
		defer func() { done <- true }()
		// Just access the server fields
		if server.config == nil {
			t.Error("Config should not be nil")
		}
	}()

	// Wait for all goroutines
	for range 3 {
		<-done
	}
}

func createTestMCPConfig(t *testing.T) *config.Config {
	return &config.Config{
		Mode: "directory",
		Directory: config.DirectoryConfig{
			Paths: []string{t.TempDir()},
		},
		IteratorTTL:                     "30m",
		IteratorCleanupInterval:         "5m",
		IteratorTTLDuration:             30 * time.Minute,
		IteratorCleanupIntervalDuration: 5 * time.Minute,
	}
}

func TestLookupIPInAllDatabasesKeepsOtherResults(t *testing.T) {
	const (
		cityDB   = "GeoIP2-City-Test.mmdb"
		ipv4DB   = "MaxMind-DB-test-ipv4-24.mmdb"
		brokenDB = "GeoIP2-City-Test-Broken-Double-Format.mmdb"
	)
	schema := compileTestSchema(t, ipOutputSchema)

	tests := []struct {
		name      string
		dbs       []string
		ip        string
		wantError string
		found     map[string]bool
		failed    []string
	}{
		{
			name:  "IPv6 address skips IPv4-only database",
			dbs:   []string{cityDB, ipv4DB},
			ip:    "2001:218::1",
			found: map[string]bool{cityDB: true, ipv4DB: false},
		},
		{
			name:  "IPv4-mapped address uses IPv4 address in IPv4-only database",
			dbs:   []string{cityDB, ipv4DB},
			ip:    "::ffff:1.1.1.1",
			found: map[string]bool{cityDB: false, ipv4DB: true},
		},
		{
			name:  "IPv6 address with only an IPv4-only database",
			dbs:   []string{ipv4DB},
			ip:    "2001:218::1",
			found: map[string]bool{ipv4DB: false},
		},
		{
			name:   "decode failure keeps other results",
			dbs:    []string{cityDB, brokenDB},
			ip:     "2001:220::",
			found:  map[string]bool{cityDB: true, brokenDB: false},
			failed: []string{brokenDB},
		},
		{
			name:      "every database failing returns an error",
			dbs:       []string{brokenDB},
			ip:        "2001:220::",
			wantError: "lookup_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := serverWithDatabases(t, tt.dbs...)
			result, err := server.lookupIPInAllDatabases(netip.MustParseAddr(tt.ip), tt.ip, nil)
			if tt.wantError != "" {
				assertToolError(t, result, err, tt.wantError)
				return
			}
			if err != nil || result.IsError {
				t.Fatalf("unexpected failure: %+v, %v", result, err)
			}
			validateTestSchema(t, schema, result.StructuredContent)

			entries := result.StructuredContent.(map[string]any)["databases"].(map[string]any)
			if len(entries) != len(tt.found) {
				t.Fatalf(
					"got %d database entries, want %d: %v",
					len(entries),
					len(tt.found),
					entries,
				)
			}
			for db, found := range tt.found {
				entry := entries[db].(map[string]any)
				if entry["found"] != found {
					t.Errorf("%s: found = %v, want %v", db, entry["found"], found)
				}
				_, hasError := entry["error"]
				wantFailure := slices.Contains(tt.failed, db)
				if hasError != wantFailure {
					t.Errorf(
						"%s: error present = %v, want %v: %v",
						db,
						hasError,
						wantFailure,
						entry,
					)
				}
				if !found && (entry["network"] != nil || entry["data"] != nil) {
					t.Errorf("%s: missing record should have null network and data: %v", db, entry)
				}
			}
		})
	}
}

func serverWithDatabases(t *testing.T, names ...string) *Server {
	t.Helper()
	db, err := database.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, name := range names {
		if err := db.LoadDatabase("../../testdata/test-data/" + name); err != nil {
			t.Fatal(err)
		}
	}
	manager := iterator.New(time.Minute, time.Second)
	t.Cleanup(manager.StopCleanup)
	return New(createTestMCPConfig(t), db, nil, manager, "test")
}

func compileTestSchema(t *testing.T, raw string) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(raw)))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func validateTestSchema(t *testing.T, schema *jsonschema.Schema, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(doc); err != nil {
		t.Fatalf("output does not match schema: %v", err)
	}
}
