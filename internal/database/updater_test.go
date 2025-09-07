package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-mcp/internal/config"
)

func TestNewUpdater(t *testing.T) {
	// Test with maxmind mode
	cfg := &config.Config{
		Mode: "maxmind",
		MaxMind: config.MaxMindConfig{
			AccountID:   123456,
			LicenseKey:  "test_license_key",
			Editions:    []string{"GeoLite2-City"},
			DatabaseDir: t.TempDir(),
			Endpoint:    "https://updates.maxmind.com",
		},
	}

	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	updater, err := NewUpdater(cfg, manager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	if updater == nil {
		t.Fatal("Updater should not be nil")
	}

	if updater.config != cfg {
		t.Error("Updater config should match input config")
	}

	if updater.manager != manager {
		t.Error("Updater manager should match input manager")
	}

	// Test with geoip_compat mode
	cfg.Mode = "geoip_compat"
	updater2, err := NewUpdater(cfg, manager)
	if err != nil {
		t.Fatalf("Failed to create updater with geoip_compat mode: %v", err)
	}
	if updater2 == nil {
		t.Error("Updater should not be nil for geoip_compat mode")
	}

	// Test with unsupported mode
	cfg.Mode = "directory"
	_, err = NewUpdater(cfg, manager)
	if err == nil {
		t.Error("Expected error for unsupported mode")
	}

	// Test with invalid credentials (should still create updater but fail on actual updates)
	cfg.Mode = "maxmind"
	cfg.MaxMind.AccountID = 0
	_, err = NewUpdater(cfg, manager)
	if err == nil {
		t.Error("Expected error for invalid account ID")
	}
}

func TestUpdateAll(t *testing.T) {
	// Create a mock updater with minimal config for testing
	cfg := createTestConfig(t)
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Note: This test will fail with network errors since we're using test credentials
	// But we can test the structure and error handling
	updater, err := NewUpdater(cfg, manager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := updater.UpdateAll(ctx)
	// We expect this to fail with network/auth errors, but not panic
	if err != nil {
		t.Logf("Expected failure due to test credentials: %v", err)
	}

	// Results should still be returned even on failure
	if results == nil {
		t.Error("Results should not be nil even on failure")
	}

	// Should have results for each edition
	expectedCount := len(cfg.MaxMind.Editions)
	if len(results) != expectedCount {
		t.Errorf("Expected %d results, got %d", expectedCount, len(results))
	}

	// Each result should have the database name set
	for _, result := range results {
		if result.Database == "" {
			t.Error("Result should have database name set")
		}
		if result.Error == "" {
			t.Log("Unexpected success - this might indicate the test credentials work")
		}
	}
}

func TestUpdateDatabase(t *testing.T) {
	cfg := createTestConfig(t)
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	updater, err := NewUpdater(cfg, manager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	edition := "GeoLite2-City"
	result, err := updater.UpdateDatabase(ctx, edition)
	// We expect this to fail with network/auth errors, but not panic
	if err != nil {
		t.Logf("Expected failure due to test credentials: %v", err)
	}

	if result.Database != edition {
		t.Errorf("Expected result database to be %s, got %s", edition, result.Database)
	}

	if result.Error == "" {
		t.Log("Unexpected success - this might indicate the test credentials work")
	}
}

func TestStartScheduledUpdates(t *testing.T) {
	// Test with auto-update enabled
	cfg := createTestConfig(t)
	cfg.AutoUpdate = true
	cfg.UpdateInterval = "1h"
	cfg.UpdateIntervalDuration = time.Hour

	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	updater, err := NewUpdater(cfg, manager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should start the scheduled updates goroutine
	updater.StartScheduledUpdates(ctx)

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	// Test with auto-update disabled
	cfg.AutoUpdate = false
	updater2, err := NewUpdater(cfg, manager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	// This should return immediately
	updater2.StartScheduledUpdates(ctx)
}

func TestLoadChecksums(t *testing.T) {
	cfg := createTestConfig(t)
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	updater, err := NewUpdater(cfg, manager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	// Create a test checksum file
	checksumFile := filepath.Join(cfg.MaxMind.DatabaseDir, ".checksums")
	checksumContent := "GeoLite2-City:abcd1234\nGeoLite2-Country:efgh5678\n"

	err = os.WriteFile(checksumFile, []byte(checksumContent), 0o600)
	if err != nil {
		t.Fatalf("Failed to write checksum file: %v", err)
	}

	// Load checksums
	updater.loadChecksums()

	// Verify checksums were loaded
	updater.mu.RLock()
	if len(updater.checksums) != 2 {
		t.Errorf("Expected 2 checksums, got %d", len(updater.checksums))
	}

	if updater.checksums["GeoLite2-City"] != "abcd1234" {
		t.Errorf("Expected checksum 'abcd1234', got '%s'", updater.checksums["GeoLite2-City"])
	}

	if updater.checksums["GeoLite2-Country"] != "efgh5678" {
		t.Errorf("Expected checksum 'efgh5678', got '%s'", updater.checksums["GeoLite2-Country"])
	}
	updater.mu.RUnlock()
}

func TestSaveChecksums(t *testing.T) {
	cfg := createTestConfig(t)
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	updater, err := NewUpdater(cfg, manager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	// Set some test checksums
	updater.mu.Lock()
	updater.checksums["GeoLite2-City"] = "test1234"
	updater.checksums["GeoLite2-Country"] = "test5678"
	updater.mu.Unlock()

	// Save checksums
	updater.saveChecksums()

	// Verify file was created
	checksumFile := filepath.Join(cfg.MaxMind.DatabaseDir, ".checksums")
	if _, err := os.Stat(checksumFile); errors.Is(err, os.ErrNotExist) {
		t.Error("Checksum file should have been created")
	}

	// Verify content
	content, err := os.ReadFile(checksumFile)
	if err != nil {
		t.Fatalf("Failed to read checksum file: %v", err)
	}

	contentStr := string(content)
	if !contains(contentStr, "GeoLite2-City:test1234") {
		t.Error("Checksum file should contain GeoLite2-City checksum")
	}
	if !contains(contentStr, "GeoLite2-Country:test5678") {
		t.Error("Checksum file should contain GeoLite2-Country checksum")
	}
}

func TestUpdateResult(t *testing.T) {
	// Test UpdateResult struct
	result := UpdateResult{
		LastUpdate: time.Now(),
		Database:   "GeoLite2-City",
		Error:      "",
		Size:       12345,
		Updated:    true,
	}

	if result.Database != "GeoLite2-City" {
		t.Error("Database field not set correctly")
	}

	if result.LastUpdate.IsZero() {
		t.Error("LastUpdate field should be set")
	}

	if result.Size != 12345 {
		t.Error("Size field not set correctly")
	}

	if !result.Updated {
		t.Error("Updated field should be true")
	}

	if result.Error != "" {
		t.Error("Error field should be empty")
	}

	// Test with error
	result.Error = "Test error"
	result.Updated = false

	if result.Error != "Test error" {
		t.Error("Error field not set correctly")
	}

	if result.Updated {
		t.Error("Updated field should be false when there's an error")
	}
}

func TestUpdaterConcurrency(t *testing.T) {
	cfg := createTestConfig(t)
	manager, err := New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	updater, err := NewUpdater(cfg, manager)
	if err != nil {
		t.Fatalf("Failed to create updater: %v", err)
	}

	// Test concurrent access to checksums
	done := make(chan bool, 2)

	go func() {
		updater.mu.Lock()
		updater.checksums["test1"] = "hash1"
		updater.mu.Unlock()
		done <- true
	}()

	go func() {
		updater.mu.RLock()
		_ = updater.checksums["test1"]
		updater.mu.RUnlock()
		done <- true
	}()

	<-done
	<-done

	// Should not deadlock or panic
}

// Helper functions

func createTestConfig(t *testing.T) *config.Config {
	return &config.Config{
		Mode: "maxmind",
		MaxMind: config.MaxMindConfig{
			AccountID:   999999, // Test account ID that will fail
			LicenseKey:  "test_license_key",
			Editions:    []string{"GeoLite2-City", "GeoLite2-Country"},
			DatabaseDir: t.TempDir(),
			Endpoint:    "https://updates.maxmind.com",
		},
		AutoUpdate:             false,
		UpdateInterval:         "24h",
		UpdateIntervalDuration: 24 * time.Hour,
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
