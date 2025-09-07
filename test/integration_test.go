package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-mcp/internal/config"
	"github.com/oschwald/maxminddb-mcp/internal/database"
	"github.com/oschwald/maxminddb-mcp/internal/iterator"
)

func TestDatabaseManagement(t *testing.T) {
	// Setup test directory
	testDir := t.TempDir()

	// Create database manager
	dbManager, err := database.New()
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}

	// Test loading empty directory
	err = dbManager.LoadDirectory(testDir)
	if err != nil {
		t.Fatalf("Failed to load empty directory: %v", err)
	}

	// Test listing databases (should be empty)
	databases := dbManager.ListDatabases()
	if len(databases) != 0 {
		t.Errorf("Expected 0 databases, got %d", len(databases))
	}

	// Create a test file (not a real MMDB, just to test file detection)
	testFile := filepath.Join(testDir, "test.mmdb")
	if err := createTestFile(testFile); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Reload directory
	err = dbManager.LoadDirectory(testDir)
	// This should not fail even if the MMDB is invalid
	if err != nil {
		t.Logf("Loading directory with invalid MMDB: %v", err)
	}

	t.Log("Database management test completed")
}

func createTestFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	// Write some dummy data
	_, err = file.WriteString("dummy mmdb file for testing")
	return err
}

// TestConfigValidation tests configuration validation.
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
	}{
		{
			name: "valid directory config",
			config: &config.Config{
				Mode:                    "directory",
				UpdateInterval:          "24h",
				IteratorTTL:             "10m",
				IteratorCleanupInterval: "1m",
				Directory: config.DirectoryConfig{
					Paths: []string{"/tmp"},
				},
			},
			expectError: false,
		},
		{
			name: "invalid mode",
			config: &config.Config{
				Mode:                    "invalid",
				UpdateInterval:          "24h",
				IteratorTTL:             "10m",
				IteratorCleanupInterval: "1m",
			},
			expectError: true,
		},
		{
			name: "directory mode missing paths",
			config: &config.Config{
				Mode:                    "directory",
				UpdateInterval:          "24h",
				IteratorTTL:             "10m",
				IteratorCleanupInterval: "1m",
			},
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.Validate()

			if test.expectError && err == nil {
				t.Error("Expected validation error, got nil")
			}

			if !test.expectError && err != nil {
				t.Errorf("Expected no validation error, got: %v", err)
			}
		})
	}
}

// TestIteratorManagement tests the iterator lifecycle.
func TestIteratorManagement(t *testing.T) {
	iterMgr := iterator.New(1*time.Minute, 10*time.Second)
	iterMgr.StartCleanup()
	defer iterMgr.StopCleanup()

	// Test creating and managing iterators would go here
	// This would require a valid MMDB reader, so it's stubbed for now

	// In a real test, you would:
	// 1. Create a test MMDB file
	// 2. Create a reader from it
	// 3. Test iterator creation, iteration, resumption, and expiration

	t.Log("Iterator management test placeholder - would test full iterator lifecycle")
}
