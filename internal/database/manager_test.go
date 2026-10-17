package database

import (
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
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
	if db.Type != "GeoLite2-City" {
		t.Errorf("Expected database type 'GeoLite2-City', got '%s'", db.Type)
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

	initialReader, _ := manager.GetReader(testDBName)
	databases := manager.ListDatabases()
	if len(databases) != 1 {
		t.Fatalf("Expected 1 database, got %d", len(databases))
	}
	initialTime := databases[0].LastUpdated

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
	if reader, _ := manager.GetReader(testDBName); reader != initialReader {
		t.Error("unchanged file replaced its reader")
	}
}

func TestReloadReplacedOrModifiedFile(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	data, err := os.ReadFile(testDBPath)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "database.mmdb")
	//nolint:gosec // The directory comes from t.TempDir.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.LoadDatabase(path); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"replaced", "modified"} {
		before, _ := manager.GetReader("database.mmdb")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if action == "replaced" {
			temporary := filepath.Join(dir, "replacement.tmp")
			//nolint:gosec // The directory comes from t.TempDir.
			if err := os.WriteFile(temporary, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(temporary, info.ModTime(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(temporary, path); err != nil {
				t.Fatal(err)
			}
		} else {
			modified := info.ModTime().Add(time.Second)
			if err := os.Chtimes(path, modified, modified); err != nil {
				t.Fatal(err)
			}
		}
		if err := manager.LoadDatabase(path); err != nil {
			t.Fatal(err)
		}
		after, _ := manager.GetReader("database.mmdb")
		if after == before {
			t.Fatalf("%s file retained its old reader", action)
		}
		if err := manager.LoadDatabase(path); err != nil {
			t.Fatal(err)
		}
		if reader, _ := manager.GetReader("database.mmdb"); reader != after {
			t.Fatalf("%s file reloaded twice", action)
		}
	}
}

func TestMetadataAndDuplicateNames(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()
	reader, err := maxminddb.Open(testDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := os.ReadFile(testDBPath)
	if err != nil {
		t.Fatal(err)
	}
	renamed := filepath.Join(t.TempDir(), "custom.mmdb")
	//nolint:gosec // The directory comes from t.TempDir.
	if err := os.WriteFile(renamed, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.LoadDatabase(renamed); err != nil {
		t.Fatal(err)
	}
	info, ok := manager.GetDatabase("custom.mmdb")
	if !ok || info.Type != reader.Metadata.DatabaseType ||
		info.Description != reader.Metadata.Description["en"] ||
		!info.LastUpdated.Equal(reader.Metadata.BuildTime()) {
		t.Fatalf("metadata did not come from MMDB: %+v", info)
	}
	duplicate := filepath.Join(t.TempDir(), "custom.mmdb")
	//nolint:gosec // The directory comes from t.TempDir.
	if err := os.WriteFile(duplicate, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.LoadDatabase(duplicate); err == nil {
		t.Fatal("accepted duplicate display name")
	}
	manager.RemoveDatabaseByPath(duplicate)
	if got, ok := manager.GetDatabase(
		"custom.mmdb",
	); !ok || got.Path != renamed ||
		len(manager.ListDatabases()) != 1 {
		t.Fatal("duplicate load or removal changed the original database")
	}
}

func TestWatchDatabaseReplacement(t *testing.T) {
	dir := t.TempDir()
	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	path := filepath.Join(dir, "database.mmdb")
	install := func(fixture string) {
		t.Helper()
		data, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatal(err)
		}
		temporary := filepath.Join(dir, "replacement.tmp")
		//nolint:gosec // The directory comes from t.TempDir.
		if err := os.WriteFile(temporary, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(temporary, path); err != nil {
			t.Fatal(err)
		}
	}
	install(testDBPath)
	if err := manager.LoadDirectory(dir); err != nil {
		t.Fatal(err)
	}
	if err := manager.WatchDirectory(dir); err != nil {
		t.Fatal(err)
	}
	manager.StartWatching()
	old, ok := manager.GetReader("database.mmdb")
	if !ok {
		t.Fatal("initial database was not loaded")
	}
	ip := netip.MustParseAddr("81.2.69.160")
	var before map[string]any
	if err := old.Lookup(ip).Decode(&before); err != nil || len(before) == 0 {
		t.Fatalf("fixture lookup failed: %v", err)
	}
	waitFor := func(description string, ready func() bool) {
		t.Helper()
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for !ready() {
			select {
			case <-ticker.C:
			case <-deadline.C:
				t.Fatalf("watcher did not detect %s", description)
			}
		}
	}
	install("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
	waitFor("replacement", func() bool {
		reader, ok := manager.GetReader("database.mmdb")
		return ok && reader.Metadata.DatabaseType == "GeoLite2-ASN"
	})
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	waitFor("removal", func() bool {
		_, ok := manager.GetReader("database.mmdb")
		return !ok
	})
	var after map[string]any
	if err := old.Lookup(ip).Decode(&after); err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("replacement invalidated the retained reader: %v", err)
	}
}
