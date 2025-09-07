package test

import (
	"errors"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-mcp/internal/database"
	"github.com/oschwald/maxminddb-mcp/internal/filter"
	"github.com/oschwald/maxminddb-mcp/internal/iterator"

	"github.com/oschwald/maxminddb-golang/v2"
)

const testDataDir = "../testdata/test-data"

func TestRealMMDBIntegration(t *testing.T) {
	// Check if test data exists
	if _, err := os.Stat(testDataDir); errors.Is(err, fs.ErrNotExist) {
		t.Skip("Test MMDB files not found. Run: git submodule update --init")
	}

	// Test with GeoLite2-City-Test.mmdb
	cityDBPath := filepath.Join(testDataDir, "GeoLite2-City-Test.mmdb")
	if _, err := os.Stat(cityDBPath); errors.Is(err, fs.ErrNotExist) {
		t.Skip("GeoLite2-City-Test.mmdb not found")
	}

	t.Run("direct_mmdb_operations", func(t *testing.T) {
		testDirectMMDBOperations(t, cityDBPath)
	})

	t.Run("database_manager_with_real_mmdb", func(t *testing.T) {
		testDatabaseManagerWithRealMMDB(t, testDataDir)
	})

	t.Run("iterator_with_real_mmdb", func(t *testing.T) {
		testIteratorWithRealMMDB(t, cityDBPath)
	})
}

func testDirectMMDBOperations(t *testing.T, dbPath string) {
	// Test direct maxminddb-golang/v2 operations
	reader, err := maxminddb.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open MMDB file: %v", err)
	}

	// Test known IP lookups - some IPs may have empty records in test data
	testIPs := []struct {
		ip          string
		expectData  bool
		description string
	}{
		{"1.1.1.1", false, "CloudFlare DNS (may not be in test data)"},
		{"8.8.8.8", false, "Google DNS (may not be in test data)"},
		{"89.160.20.112", true, "Swedish IP (should have data in test DB)"},
		{"216.160.83.56", true, "US IP (should have data in test DB)"},
	}

	for _, testIP := range testIPs {
		ip, err := netip.ParseAddr(testIP.ip)
		if err != nil {
			t.Errorf("Failed to parse IP %s: %v", testIP.ip, err)
			continue
		}

		var record map[string]any
		err = reader.Lookup(ip).Decode(&record)
		if err != nil {
			t.Errorf("Lookup failed for IP %s: %v", testIP.ip, err)
			continue
		}

		t.Logf("IP %s (%s) record: %+v", testIP.ip, testIP.description, record)

		// Only verify data exists if we expect it
		if testIP.expectData && len(record) == 0 {
			t.Errorf(
				"Expected data for IP %s (%s) but got empty record",
				testIP.ip,
				testIP.description,
			)
		}
	}
}

func testDatabaseManagerWithRealMMDB(t *testing.T, testDataDir string) {
	// Create database manager
	dbManager, err := database.New()
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}

	// Load test databases
	err = dbManager.LoadDirectory(testDataDir)
	if err != nil {
		t.Fatalf("Failed to load test directory: %v", err)
	}

	// List databases
	databases := dbManager.ListDatabases()
	t.Logf("Found %d databases", len(databases))

	if len(databases) == 0 {
		t.Error("No databases loaded")
		return
	}

	// Test getting a specific database
	for _, dbInfo := range databases {
		reader, exists := dbManager.GetReader(dbInfo.Name)
		if !exists {
			t.Errorf("Failed to get reader for database %s", dbInfo.Name)
			continue
		}

		t.Logf("Database %s: type=%s, size=%d bytes",
			dbInfo.Name, dbInfo.Type, dbInfo.Size)

		// Test a lookup
		ip, _ := netip.ParseAddr("1.1.1.1")
		var record map[string]any
		err = reader.Lookup(ip).Decode(&record)
		if err != nil {
			t.Logf("Lookup failed for database %s: %v", dbInfo.Name, err)
		} else {
			t.Logf("Database %s lookup successful, record has %d fields",
				dbInfo.Name, len(record))
		}
	}
}

func testIteratorWithRealMMDB(t *testing.T, dbPath string) {
	// Test iterator functionality with real MMDB
	reader, err := maxminddb.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open MMDB file: %v", err)
	}

	// Create iterator manager
	iterMgr := iterator.New(1*time.Minute, 10*time.Second, 20)
	iterMgr.StartCleanup()
	defer iterMgr.StopCleanup()

	// Test iterating over a small network range
	network, err := netip.ParsePrefix("1.1.1.0/30")
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	// Test with no filters
	filters := []filter.Filter{}
	iter, err := iterMgr.CreateIterator(reader, "test", network, filters, "and")
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}

	// Perform iteration
	result, err := iterMgr.Iterate(iter, 10)
	if err != nil {
		t.Fatalf("Iteration failed: %v", err)
	}

	t.Logf("Iterator results: processed=%d, matched=%d, has_more=%t",
		result.TotalProcessed, result.TotalMatched, result.HasMore)

	if len(result.Results) > 0 {
		t.Logf("First result: network=%s, data=%+v",
			result.Results[0].Network, result.Results[0].Data)
	}

	// Test with filters
	filters = []filter.Filter{
		{Field: "country.iso_code", Operator: "exists", Value: true},
	}

	filteredIter, err := iterMgr.CreateIterator(reader, "test_filtered", network, filters, "and")
	if err != nil {
		t.Fatalf("Failed to create filtered iterator: %v", err)
	}

	filteredResult, err := iterMgr.Iterate(filteredIter, 10)
	if err != nil {
		t.Fatalf("Filtered iteration failed: %v", err)
	}

	t.Logf("Filtered iterator results: processed=%d, matched=%d, has_more=%t",
		filteredResult.TotalProcessed, filteredResult.TotalMatched, filteredResult.HasMore)
}

func TestVariousMMDBFiles(t *testing.T) {
	testDataDir := testDataDir
	if _, err := os.Stat(testDataDir); errors.Is(err, fs.ErrNotExist) {
		t.Skip("Test MMDB files not found. Run: git submodule update --init")
	}

	// Test different types of MMDB files
	testFiles := map[string]string{
		"city":    "GeoLite2-City-Test.mmdb",
		"country": "GeoLite2-Country-Test.mmdb",
		"asn":     "GeoLite2-ASN-Test.mmdb",
		"decoder": "MaxMind-DB-test-decoder.mmdb",
		"ipv4":    "MaxMind-DB-test-ipv4-24.mmdb",
		"ipv6":    "MaxMind-DB-test-ipv6-24.mmdb",
		"mixed":   "MaxMind-DB-test-mixed-24.mmdb",
	}

	for testName, filename := range testFiles {
		dbPath := filepath.Join(testDataDir, filename)
		if _, err := os.Stat(dbPath); errors.Is(err, fs.ErrNotExist) {
			t.Logf("Skipping %s test - file not found: %s", testName, filename)
			continue
		}

		t.Run(testName, func(t *testing.T) {
			reader, err := maxminddb.Open(dbPath)
			if err != nil {
				t.Fatalf("Failed to open %s: %v", filename, err)
			}

			// Test basic metadata
			t.Logf("Database: %s", filename)
			t.Logf("  Record size: %d", reader.Metadata.RecordSize)
			t.Logf("  IP version: %d", reader.Metadata.IPVersion)
			t.Logf("  Node count: %d", reader.Metadata.NodeCount)
			t.Logf("  Database type: %s", reader.Metadata.DatabaseType)

			// Test network iteration for supported files
			if testName == "city" || testName == "country" || testName == "asn" {
				testNetworkIteration(t, reader, testName)
			}
		})
	}
}

func testNetworkIteration(t *testing.T, reader *maxminddb.Reader, dbType string) {
	// Test iterating over networks in the database
	count := 0
	maxCount := 5 // Just test a few networks

	// Use NetworksWithin for a small range
	prefix, _ := netip.ParsePrefix("1.0.0.0/16")

	for result := range reader.NetworksWithin(prefix) {
		if count >= maxCount {
			break
		}

		var record map[string]any
		if err := result.Decode(&record); err != nil {
			t.Logf("Failed to decode record for network %s: %v", result.Prefix(), err)
			continue
		}

		t.Logf("%s network %s: %+v", dbType, result.Prefix(), record)
		count++
	}

	if count == 0 {
		t.Logf("No networks found in test range for %s database", dbType)
	} else {
		t.Logf("Successfully iterated over %d networks in %s database", count, dbType)
	}
}

func BenchmarkRealMMDBLookup(b *testing.B) {
	testDataDir := testDataDir
	cityDBPath := filepath.Join(testDataDir, "GeoLite2-City-Test.mmdb")

	if _, err := os.Stat(cityDBPath); errors.Is(err, fs.ErrNotExist) {
		b.Skip("GeoLite2-City-Test.mmdb not found")
	}

	reader, err := maxminddb.Open(cityDBPath)
	if err != nil {
		b.Fatalf("Failed to open MMDB file: %v", err)
	}

	ip, _ := netip.ParseAddr("89.160.20.112")

	b.ResetTimer()
	for range b.N {
		var record map[string]any
		_ = reader.Lookup(ip).Decode(&record)
	}
}

func BenchmarkRealMMDBNetworkIteration(b *testing.B) {
	testDataDir := testDataDir
	cityDBPath := filepath.Join(testDataDir, "GeoLite2-City-Test.mmdb")

	if _, err := os.Stat(cityDBPath); errors.Is(err, fs.ErrNotExist) {
		b.Skip("GeoLite2-City-Test.mmdb not found")
	}

	reader, err := maxminddb.Open(cityDBPath)
	if err != nil {
		b.Fatalf("Failed to open MMDB file: %v", err)
	}

	prefix, _ := netip.ParsePrefix("89.160.20.0/24")

	b.ResetTimer()
	for range b.N {
		count := 0
		for result := range reader.NetworksWithin(prefix) {
			count++
			if count > 10 { // Limit iteration for benchmark
				break
			}
			var record map[string]any
			_ = result.Decode(&record)
		}
	}
}
