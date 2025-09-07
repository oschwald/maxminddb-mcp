package iterator

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
)

// TestNetworkIterationFix tests the fix for the iterator deadlock issue
// where network iteration would hang when looking for residential networks.
func TestNetworkIterationFix(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Unable to get home directory")
	}

	dbPath := filepath.Join(
		homeDir,
		".cache",
		"maxminddb-mcp",
		"databases",
		"GeoIP2-Enterprise.mmdb",
	)
	if _, err := os.Stat(dbPath); errors.Is(err, fs.ErrNotExist) {
		t.Skip("GeoIP2-Enterprise.mmdb not found - run update_databases tool first")
	}

	dbManager, err := database.New()
	if err != nil {
		t.Fatalf("Failed to create database manager: %v", err)
	}

	if err := dbManager.LoadDirectory(filepath.Dir(dbPath)); err != nil {
		t.Fatalf("Failed to load database directory: %v", err)
	}

	iterMgr := New(10*time.Minute, 1*time.Minute)
	iterMgr.StartCleanup()
	defer iterMgr.StopCleanup()

	reader, exists := dbManager.GetReader("GeoIP2-Enterprise.mmdb")
	if !exists {
		t.Fatal("GeoIP2-Enterprise.mmdb not found in database manager")
	}

	network, _ := netip.ParsePrefix("130.0.0.0/24")

	t.Run("WithoutFilters", func(t *testing.T) {
		iterator, err := iterMgr.CreateIterator(
			reader,
			"GeoIP2-Enterprise.mmdb",
			network,
			nil,
			"and",
		)
		if err != nil {
			t.Fatalf("Failed to create iterator: %v", err)
		}

		// Small delay to ensure reader iteration starts
		time.Sleep(50 * time.Millisecond)

		result, err := iterMgr.Iterate(iterator, 10)
		if err != nil {
			t.Fatalf("Iterate failed: %v", err)
		}

		t.Logf("Without filters: processed=%d, matched=%d, results=%d",
			result.TotalProcessed, result.TotalMatched, len(result.Results))

		if result.TotalProcessed == 0 {
			t.Error("Expected to process some networks, got 0")
		}

		if len(result.Results) == 0 {
			t.Error("Expected some results, got 0")
		}

		// Check that we found the expected network
		found130Network := false
		for _, res := range result.Results {
			t.Logf("Found network: %s", res.Network)
			if res.Network.Contains(netip.MustParseAddr("130.0.0.1")) {
				found130Network = true
			}
		}

		if !found130Network {
			t.Error("Expected to find a network containing 130.0.0.1")
		}
	})

	t.Run("WithResidentialFilter", func(t *testing.T) {
		filters := []filter.Filter{
			{
				Field:    "traits.user_type",
				Operator: "eq",
				Value:    "residential",
			},
		}

		iterator, err := iterMgr.CreateIterator(
			reader,
			"GeoIP2-Enterprise.mmdb",
			network,
			filters,
			"and",
		)
		if err != nil {
			t.Fatalf("Failed to create iterator with filters: %v", err)
		}

		// Small delay to ensure reader iteration starts
		time.Sleep(50 * time.Millisecond)

		result, err := iterMgr.Iterate(iterator, 10)
		if err != nil {
			t.Fatalf("Iterate with filters failed: %v", err)
		}

		t.Logf("With residential filter: processed=%d, matched=%d, results=%d",
			result.TotalProcessed, result.TotalMatched, len(result.Results))

		if result.TotalProcessed == 0 {
			t.Error("Expected to process some networks even with filter, got 0")
		}

		// We expect some matches since 130.0.0.1 is residential
		if result.TotalMatched == 0 {
			t.Error("Expected some residential matches in 130.0.0.0/24, got 0")
		}

		if len(result.Results) == 0 {
			t.Error("Expected some residential results, got 0")
		}

		// Verify all results have residential user_type
		for i, res := range result.Results {
			t.Logf("Residential network %d: %s", i+1, res.Network)

			traitsAny, ok := res.Data["traits"]
			if !ok {
				t.Errorf("No traits found for network %s", res.Network)
				continue
			}
			traits, ok := traitsAny.(map[string]any)
			if !ok {
				t.Errorf("Traits field has unexpected type for network %s", res.Network)
				continue
			}
			userTypeAny, ok := traits["user_type"]
			if !ok {
				t.Errorf("No user_type found in traits for network %s", res.Network)
				continue
			}
			userType, ok := userTypeAny.(string)
			if !ok {
				t.Errorf("user_type has unexpected type for network %s", res.Network)
				continue
			}
			if userType != "residential" {
				t.Errorf(
					"Expected user_type=residential, got %s for network %s",
					userType,
					res.Network,
				)
			}
		}
	})
}
