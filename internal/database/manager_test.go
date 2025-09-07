package database

import (
	"os"
	"testing"
	"time"
)

const (
	testDBPath = "../../testdata/test-data/GeoLite2-City-Test.mmdb"
	testDBName = "GeoLite2-City-Test.mmdb"
)

func TestNew(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	if manager == nil {
		t.Error("Manager should not be nil")
	}

	if manager.readers == nil {
		t.Error("Readers map should not be nil")
	}

	if manager.databases == nil {
		t.Error("Databases map should not be nil")
	}

	if manager.watcher == nil {
		t.Error("Watcher should not be nil")
	}

	if manager.watchDirs == nil {
		t.Error("WatchDirs should not be nil")
	}
}

func TestLoadDatabase(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Test loading a valid test database
	testDBPath := testDBPath
	err = manager.LoadDatabase(testDBPath)
	if err != nil {
		t.Fatalf("Failed to load test database: %v", err)
	}

	// Verify database is loaded
	databases := manager.ListDatabases()
	if len(databases) != 1 {
		t.Errorf("Expected 1 database, got %d", len(databases))
	}

	dbName := testDBName
	reader, exists := manager.GetReader(dbName)
	if !exists {
		t.Error("Database should exist after loading")
	}
	if reader == nil {
		t.Error("Reader should not be nil")
	}

	// Test loading non-existent database
	err = manager.LoadDatabase("/nonexistent/path.mmdb")
	if err == nil {
		t.Error("Expected error when loading non-existent database")
	}

	// Test loading invalid database
	invalidDBPath := createInvalidMMDB(t)
	defer func() { _ = os.Remove(invalidDBPath) }()

	err = manager.LoadDatabase(invalidDBPath)
	if err == nil {
		t.Error("Expected error when loading invalid database")
	}
}

func TestLoadDirectory(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Test loading directory with MMDB files
	testDir := "../../testdata/test-data"
	err = manager.LoadDirectory(testDir)
	if err != nil {
		t.Fatalf("Failed to load directory: %v", err)
	}

	// Verify multiple databases are loaded
	databases := manager.ListDatabases()
	if len(databases) == 0 {
		t.Error("Expected at least one database to be loaded")
	}

	// Test loading non-existent directory
	err = manager.LoadDirectory("/nonexistent/directory")
	if err == nil {
		t.Error("Expected error when loading non-existent directory")
	}

	// Test loading empty directory
	tempDir := t.TempDir()
	err = manager.LoadDirectory(tempDir)
	if err != nil {
		t.Errorf("Should not error on empty directory: %v", err)
	}
}

func TestWatchDirectory(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Create temporary directory for testing
	tempDir := t.TempDir()

	// Test watching directory
	err = manager.WatchDirectory(tempDir)
	if err != nil {
		t.Fatalf("Failed to watch directory: %v", err)
	}

	// Test watching non-existent directory
	err = manager.WatchDirectory("/nonexistent/directory")
	if err == nil {
		t.Error("Expected error when watching non-existent directory")
	}
}

func TestStartWatching(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Create temporary directory
	tempDir := t.TempDir()
	err = manager.WatchDirectory(tempDir)
	if err != nil {
		t.Fatalf("Failed to add directory to watch: %v", err)
	}

	// Start watching (should not error)
	manager.StartWatching()

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)
}

func TestRemoveDatabase(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Load a database first
	testDBPath := testDBPath
	err = manager.LoadDatabase(testDBPath)
	if err != nil {
		t.Fatalf("Failed to load test database: %v", err)
	}

	dbName := testDBName

	// Verify it exists
	_, exists := manager.GetReader(dbName)
	if !exists {
		t.Error("Database should exist before removal")
	}

	// Remove the database
	manager.RemoveDatabase(dbName)

	// Verify it's gone
	_, exists = manager.GetReader(dbName)
	if exists {
		t.Error("Database should not exist after removal")
	}

	// Test removing non-existent database (should not panic)
	manager.RemoveDatabase("nonexistent.mmdb")
}

func TestGetReader(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Test getting non-existent reader
	_, exists := manager.GetReader("nonexistent.mmdb")
	if exists {
		t.Error("Non-existent reader should not exist")
	}

	// Load a database and test getting its reader
	testDBPath := testDBPath
	err = manager.LoadDatabase(testDBPath)
	if err != nil {
		t.Fatalf("Failed to load test database: %v", err)
	}

	dbName := testDBName
	reader, exists := manager.GetReader(dbName)
	if !exists {
		t.Error("Reader should exist after loading database")
	}
	if reader == nil {
		t.Error("Reader should not be nil")
	}
}

func TestListDatabases(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Initially should be empty
	databases := manager.ListDatabases()
	if len(databases) != 0 {
		t.Errorf("Expected 0 databases initially, got %d", len(databases))
	}

	// Load some databases
	testDBPath := testDBPath
	err = manager.LoadDatabase(testDBPath)
	if err != nil {
		t.Fatalf("Failed to load test database: %v", err)
	}

	databases = manager.ListDatabases()
	if len(databases) != 1 {
		t.Errorf("Expected 1 database after loading, got %d", len(databases))
	}

	// Verify database info
	db := databases[0]
	if db.Name != testDBName {
		t.Errorf("Expected database name 'GeoLite2-City-Test.mmdb', got '%s'", db.Name)
	}
	if db.Type != "City" {
		t.Errorf("Expected database type 'City', got '%s'", db.Type)
	}
	if db.Size <= 0 {
		t.Errorf("Expected positive database size, got %d", db.Size)
	}
}

func TestClose(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Load a database
	testDBPath := testDBPath
	err = manager.LoadDatabase(testDBPath)
	if err != nil {
		t.Fatalf("Failed to load test database: %v", err)
	}

	// Close should not error
	err = manager.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}

	// Second close should not panic
	err = manager.Close()
	if err != nil {
		t.Errorf("Second close should not error: %v", err)
	}
}

func TestInferDatabaseType(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{testDBName, "City"},
		{"GeoLite2-Country-Test.mmdb", "Country"},
		{"GeoLite2-ASN-Test.mmdb", "ASN"},
		{"GeoIP2-City-Test.mmdb", "City"},
		{"GeoIP2-Country-Test.mmdb", "Country"},
		{"GeoIP2-Enterprise-Test.mmdb", "Enterprise"},
		{"GeoIP2-ISP-Test.mmdb", "ISP"},
		{"GeoIP2-Domain-Test.mmdb", "Domain"},
		{"GeoIP2-Connection-Type-Test.mmdb", "Connection Type"},
		{"GeoIP2-Anonymous-IP-Test.mmdb", "Anonymous IP"},
		{"unknown-database.mmdb", "Unknown"},
		{"test.mmdb", "Unknown"},
	}

	for _, test := range tests {
		result := inferDatabaseType(test.filename)
		if result != test.expected {
			t.Errorf(
				"inferDatabaseType(%s) = %s, expected %s",
				test.filename,
				result,
				test.expected,
			)
		}
	}
}

func TestGetDatabaseDescription(t *testing.T) {
	tests := []struct {
		dbType   string
		expected string
	}{
		{"City", "IP geolocation with city-level precision"},
		{"Country", "IP geolocation with country-level precision"},
		{"ASN", "Autonomous system number and organization"},
		{"Enterprise", "Enterprise-level IP intelligence"},
		{"ISP", "Internet service provider information"},
		{"Domain", "Domain name information"},
		{"Connection Type", "Connection type classification"},
		{"Anonymous IP", "Anonymous proxy and VPN detection"},
		{"Unknown", "MaxMind database file"},
	}

	for _, test := range tests {
		result := getDatabaseDescription(test.dbType)
		if result != test.expected {
			t.Errorf(
				"getDatabaseDescription(%s) = %s, expected %s",
				test.dbType,
				result,
				test.expected,
			)
		}
	}
}

// Helper function to create an invalid MMDB file for testing.
func createInvalidMMDB(t *testing.T) string {
	tempFile, err := os.CreateTemp(t.TempDir(), "invalid-*.mmdb")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// Write some invalid data
	_, err = tempFile.WriteString("This is not a valid MMDB file")
	if err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	_ = tempFile.Close()
	return tempFile.Name()
}

func TestLoadDatabaseConcurrency(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Test concurrent access
	testDBPath := testDBPath

	// Load database in multiple goroutines
	done := make(chan bool, 2)

	go func() {
		err := manager.LoadDatabase(testDBPath)
		if err != nil {
			t.Errorf("Failed to load database in goroutine 1: %v", err)
		}
		done <- true
	}()

	go func() {
		// Small delay to create race condition
		time.Sleep(1 * time.Millisecond)
		databases := manager.ListDatabases()
		_ = databases // Use the result
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Verify database is loaded correctly
	databases := manager.ListDatabases()
	if len(databases) != 1 {
		t.Errorf("Expected 1 database after concurrent loading, got %d", len(databases))
	}
}

func TestReloadDatabase(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Load database initially
	testDBPath := testDBPath
	err = manager.LoadDatabase(testDBPath)
	if err != nil {
		t.Fatalf("Failed to load test database: %v", err)
	}

	// Get initial database info
	databases := manager.ListDatabases()
	if len(databases) != 1 {
		t.Fatalf("Expected 1 database, got %d", len(databases))
	}
	initialTime := databases[0].LastUpdated

	// Wait a bit and reload
	time.Sleep(10 * time.Millisecond)
	err = manager.LoadDatabase(testDBPath)
	if err != nil {
		t.Fatalf("Failed to reload test database: %v", err)
	}

	// Should still have only one database
	databases = manager.ListDatabases()
	if len(databases) != 1 {
		t.Errorf("Expected 1 database after reload, got %d", len(databases))
	}

	// Last updated time should be the same since file hasn't changed
	newTime := databases[0].LastUpdated
	if !newTime.Equal(initialTime) {
		t.Error("Expected same timestamp after reload of unchanged file")
	}
}
