package iterator

import (
	"encoding/base64"
	"encoding/json"
	"net/netip"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-mcp/internal/filter"

	"github.com/oschwald/maxminddb-golang/v2"
)

func TestNew(t *testing.T) {
	ttl := 30 * time.Minute
	cleanupInterval := 5 * time.Minute

	manager := New(ttl, cleanupInterval)

	if manager == nil {
		t.Fatal("Manager should not be nil")
	}

	if manager.ttl != ttl {
		t.Errorf("Expected TTL %v, got %v", ttl, manager.ttl)
	}

	if manager.cleanupInterval != cleanupInterval {
		t.Errorf("Expected cleanup interval %v, got %v", cleanupInterval, manager.cleanupInterval)
	}

	if manager.iterators == nil {
		t.Error("Iterators map should not be nil")
	}

	if manager.stopCleanup == nil {
		t.Error("Stop cleanup channel should not be nil")
	}
}

func TestCreateIterator(t *testing.T) {
	manager := New(30*time.Minute, 5*time.Minute)

	// Test creating iterator with valid test database
	reader, err := maxminddb.Open("../../testdata/test-data/GeoLite2-City-Test.mmdb")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer func() { _ = reader.Close() }()

	network, err := netip.ParsePrefix("1.0.0.0/8")
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	filters := []filter.Filter{
		{
			Field:    "country.iso_code",
			Operator: "equals",
			Value:    "US",
		},
	}

	iterator, err := manager.CreateIterator(
		reader,
		testDB,
		network,
		filters,
		filterModeAnd,
	)
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}

	if iterator == nil {
		t.Fatal("Iterator should not be nil")
	}

	if iterator.ID == "" {
		t.Error("Iterator ID should not be empty")
	}

	if iterator.Database != testDB {
		t.Errorf("Expected database 'test-db', got '%s'", iterator.Database)
	}

	if iterator.Network != network {
		t.Errorf("Expected network %v, got %v", network, iterator.Network)
	}

	if iterator.FilterMode != filterModeAnd {
		t.Errorf("Expected filter mode 'and', got '%s'", iterator.FilterMode)
	}

	if len(iterator.Filters) != 1 {
		t.Errorf("Expected 1 filter, got %d", len(iterator.Filters))
	}

	if iterator.FilterEngine == nil {
		t.Error("Filter engine should not be nil when filters are provided")
	}

	// Verify iterator is stored in manager
	storedIterator, exists := manager.GetIterator(iterator.ID)
	if !exists {
		t.Error("Iterator should be stored in manager")
	}

	if storedIterator.ID != iterator.ID {
		t.Error("Stored iterator should match created iterator")
	}
}

func TestCreateIteratorWithoutFilters(t *testing.T) {
	manager := New(30*time.Minute, 5*time.Minute)

	reader, err := maxminddb.Open("../../testdata/test-data/GeoLite2-City-Test.mmdb")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer func() { _ = reader.Close() }()

	network, err := netip.ParsePrefix("1.0.0.0/8")
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	iterator, err := manager.CreateIterator(
		reader,
		testDB,
		network,
		nil, // No filters
		"",  // No filter mode
	)
	if err != nil {
		t.Fatalf("Failed to create iterator without filters: %v", err)
	}

	if iterator.FilterEngine != nil {
		t.Error("Filter engine should be nil when no filters are provided")
	}
}

func TestGetIterator(t *testing.T) {
	manager := New(30*time.Minute, 5*time.Minute)

	// Test getting non-existent iterator
	iterator, exists := manager.GetIterator("nonexistent")
	if exists {
		t.Error("Non-existent iterator should return false")
	}
	if iterator != nil {
		t.Error("Non-existent iterator should return nil")
	}

	// Create an iterator and test getting it
	reader, err := maxminddb.Open("../../testdata/test-data/GeoLite2-City-Test.mmdb")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer func() { _ = reader.Close() }()

	network, err := netip.ParsePrefix("1.0.0.0/8")
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	createdIterator, err := manager.CreateIterator(reader, testDB, network, nil, "")
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}

	retrievedIterator, exists := manager.GetIterator(createdIterator.ID)
	if !exists {
		t.Error("Should be able to retrieve created iterator")
	}
	if retrievedIterator == nil {
		t.Fatal("Retrieved iterator should not be nil")
	}

	if retrievedIterator.ID != createdIterator.ID {
		t.Error("Retrieved iterator should match created iterator")
	}
}

func TestIterate(t *testing.T) {
	manager := New(30*time.Minute, 5*time.Minute)

	reader, err := maxminddb.Open("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer func() { _ = reader.Close() }()

	// Use a small network that should have data in the test DB
	network, err := netip.ParsePrefix("1.0.0.0/24")
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	iterator, err := manager.CreateIterator(reader, "asn-test", network, nil, "")
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}

	// Test iteration
	result, err := manager.Iterate(iterator, 10)
	if err != nil {
		t.Fatalf("Failed to iterate: %v", err)
	}

	if result == nil {
		t.Fatal("Result should not be nil")
	}

	if result.IteratorID != iterator.ID {
		t.Errorf("Expected iterator ID %s, got %s", iterator.ID, result.IteratorID)
	}

	// Should have some basic fields
	if result.TotalProcessed < 0 {
		t.Error("Total processed should not be negative")
	}

	if result.TotalMatched < 0 {
		t.Error("Total matched should not be negative")
	}

	// Test with nil iterator
	_, err = manager.Iterate(nil, 10)
	if err == nil {
		t.Error("Expected error when iterating with nil iterator")
	}
}

func TestIterateWithFilters(t *testing.T) {
	manager := New(30*time.Minute, 5*time.Minute)

	reader, err := maxminddb.Open("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer func() { _ = reader.Close() }()

	network, err := netip.ParsePrefix("1.0.0.0/24")
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	// Create filter for ASN data
	filters := []filter.Filter{
		{
			Field:    "autonomous_system_number",
			Operator: "gt",
			Value:    float64(0),
		},
	}

	iterator, err := manager.CreateIterator(reader, "asn-test", network, filters, filterModeAnd)
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}

	result, err := manager.Iterate(iterator, 10)
	if err != nil {
		t.Fatalf("Failed to iterate with filters: %v", err)
	}

	if result == nil {
		t.Fatal("Result should not be nil")
	}

	// With filters, matched count might be different from processed
	if result.TotalMatched > result.TotalProcessed {
		t.Error("Matched count should not exceed processed count")
	}
}

func TestRemoveIterator(t *testing.T) {
	manager := New(30*time.Minute, 5*time.Minute)

	reader, err := maxminddb.Open("../../testdata/test-data/GeoLite2-City-Test.mmdb")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer func() { _ = reader.Close() }()

	network, err := netip.ParsePrefix("1.0.0.0/8")
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	iterator, err := manager.CreateIterator(reader, testDB, network, nil, "")
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}

	// Verify iterator exists
	_, exists := manager.GetIterator(iterator.ID)
	if !exists {
		t.Error("Iterator should exist before removal")
	}

	// Remove iterator
	manager.RemoveIterator(iterator.ID)

	// Verify iterator is gone
	_, exists = manager.GetIterator(iterator.ID)
	if exists {
		t.Error("Iterator should not exist after removal")
	}

	// Test removing non-existent iterator (should not panic)
	manager.RemoveIterator("nonexistent")
}

func TestStartStopCleanup(_ *testing.T) {
	manager := New(10*time.Millisecond, 5*time.Millisecond) // Very short intervals

	// Start cleanup
	manager.StartCleanup()

	// Give it a moment to start
	time.Sleep(20 * time.Millisecond)

	// Stop cleanup
	manager.StopCleanup()

	// Should not panic or deadlock
}

func TestCleanupExpiredIterators(t *testing.T) {
	// Use very short TTL for testing
	manager := New(10*time.Millisecond, 5*time.Millisecond)

	reader, err := maxminddb.Open("../../testdata/test-data/GeoLite2-City-Test.mmdb")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer func() { _ = reader.Close() }()

	network, err := netip.ParsePrefix("1.0.0.0/8")
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	// Create iterator
	iterator, err := manager.CreateIterator(reader, testDB, network, nil, "")
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}

	// Verify it exists
	_, exists := manager.GetIterator(iterator.ID)
	if !exists {
		t.Error("Iterator should exist after creation")
	}

	// Wait for it to expire
	time.Sleep(50 * time.Millisecond)

	// Manually trigger cleanup
	manager.cleanupExpired()

	// Iterator should be gone
	_, exists = manager.GetIterator(iterator.ID)
	if exists {
		t.Error("Expired iterator should be removed")
	}
}

func TestGenerateResumeToken(t *testing.T) {
	manager := New(30*time.Minute, 5*time.Minute)

	reader, err := maxminddb.Open("../../testdata/test-data/GeoLite2-City-Test.mmdb")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer func() { _ = reader.Close() }()

	network, err := netip.ParsePrefix("1.0.0.0/8")
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	filters := []filter.Filter{
		{
			Field:    "country.iso_code",
			Operator: "equals",
			Value:    "US",
		},
	}

	iterator, err := manager.CreateIterator(reader, testDB, network, filters, filterModeAnd)
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}

	// Set some state
	iterator.Processed = 100
	iterator.Matched = 50
	iterator.LastNetwork, _ = netip.ParsePrefix("1.1.1.0/24")

	// Generate resume token
	token, err := manager.generateResumeToken(iterator)
	if err != nil {
		t.Fatalf("Failed to generate resume token: %v", err)
	}

	if token == "" {
		t.Error("Resume token should not be empty")
	}

	// Decode and verify token
	var resumeToken ResumeToken
	decoded, err := base64DecodeResumeToken(token)
	if err != nil {
		t.Fatalf("Failed to decode resume token: %v", err)
	}

	err = json.Unmarshal(decoded, &resumeToken)
	if err != nil {
		t.Fatalf("Failed to unmarshal resume token: %v", err)
	}

	if resumeToken.Database != testDB {
		t.Errorf("Expected database 'test-db', got '%s'", resumeToken.Database)
	}

	if resumeToken.Network != "1.0.0.0/8" {
		t.Errorf("Expected network '1.0.0.0/8', got '%s'", resumeToken.Network)
	}

	if resumeToken.Processed != 100 {
		t.Errorf("Expected processed count 100, got %d", resumeToken.Processed)
	}

	if resumeToken.Matched != 50 {
		t.Errorf("Expected matched count 50, got %d", resumeToken.Matched)
	}

	if resumeToken.FilterMode != filterModeAnd {
		t.Errorf("Expected filter mode 'and', got '%s'", resumeToken.FilterMode)
	}

	if len(resumeToken.Filters) != 1 {
		t.Errorf("Expected 1 filter, got %d", len(resumeToken.Filters))
	}
}

func TestGenerateID(t *testing.T) {
	// Test generating multiple IDs
	ids := make(map[string]bool)

	for range 100 {
		id, err := generateID()
		if err != nil {
			t.Fatalf("Failed to generate ID: %v", err)
		}

		if id == "" {
			t.Error("Generated ID should not be empty")
		}

		if ids[id] {
			t.Errorf("Generated duplicate ID: %s", id)
		}

		ids[id] = true
	}
}

func TestIteratorConcurrency(t *testing.T) {
	manager := New(30*time.Minute, 5*time.Minute)

	reader, err := maxminddb.Open("../../testdata/test-data/GeoLite2-City-Test.mmdb")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer func() { _ = reader.Close() }()

	network, err := netip.ParsePrefix("1.0.0.0/8")
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	// Test concurrent creation and access
	done := make(chan bool, 3)

	// Create iterator in one goroutine
	go func() {
		defer func() { done <- true }()
		_, err := manager.CreateIterator(reader, "test-db-1", network, nil, "")
		if err != nil {
			t.Errorf("Failed to create iterator in goroutine: %v", err)
		}
	}()

	// Access iterator in another goroutine
	go func() {
		defer func() { done <- true }()
		// Small delay to allow creation
		time.Sleep(1 * time.Millisecond)
		_, exists := manager.GetIterator("test-db-1")
		_ = exists // Use the result
	}()

	// Remove iterator in third goroutine
	go func() {
		defer func() { done <- true }()
		time.Sleep(2 * time.Millisecond)
		manager.RemoveIterator("test-db-1")
	}()

	// Wait for all goroutines
	for range 3 {
		<-done
	}

	// Should not deadlock or panic
}

func TestMultipleIterators(t *testing.T) {
	manager := New(30*time.Minute, 5*time.Minute)

	reader, err := maxminddb.Open("../../testdata/test-data/GeoLite2-City-Test.mmdb")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer func() { _ = reader.Close() }()

	network, err := netip.ParsePrefix("1.0.0.0/8")
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	// Create a few iterators
	iter1, err := manager.CreateIterator(reader, "test-db-1", network, nil, "")
	if err != nil {
		t.Fatalf("Failed to create iterator 1: %v", err)
	}

	iter2, err := manager.CreateIterator(reader, "test-db-2", network, nil, "")
	if err != nil {
		t.Fatalf("Failed to create iterator 2: %v", err)
	}

	// Verify both iterators can be retrieved
	retrieved1, exists := manager.GetIterator(iter1.ID)
	if !exists || retrieved1 == nil {
		t.Error("First iterator should be retrievable")
	}

	retrieved2, exists := manager.GetIterator(iter2.ID)
	if !exists || retrieved2 == nil {
		t.Error("Second iterator should be retrievable")
	}

	// Verify they are different
	if iter1.ID == iter2.ID {
		t.Error("Iterator IDs should be different")
	}
}

// Helper function to decode resume token for testing.
func base64DecodeResumeToken(token string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(token)
}

func TestManagedIteratorFields(t *testing.T) {
	// Test ManagedIterator struct fields
	now := time.Now()
	network, _ := netip.ParsePrefix("1.0.0.0/8")
	lastNetwork, _ := netip.ParsePrefix("1.1.0.0/16")

	iterator := &ManagedIterator{
		ID:          "test-id",
		Database:    testDB,
		Network:     network,
		LastNetwork: lastNetwork,
		FilterMode:  filterModeAnd,
		Created:     now,
		LastAccess:  now,
		Processed:   100,
		Matched:     50,
	}

	if iterator.ID != "test-id" {
		t.Error("ID field not set correctly")
	}

	if iterator.Database != testDB {
		t.Error("Database field not set correctly")
	}

	if iterator.Network != network {
		t.Error("Network field not set correctly")
	}

	if iterator.LastNetwork != lastNetwork {
		t.Error("LastNetwork field not set correctly")
	}

	if iterator.Created != now {
		t.Error("Created field not set correctly")
	}

	if iterator.LastAccess != now {
		t.Error("LastAccess field not set correctly")
	}

	if iterator.FilterMode != filterModeAnd {
		t.Error("FilterMode field not set correctly")
	}

	if iterator.Processed != 100 {
		t.Error("Processed field not set correctly")
	}

	if iterator.Matched != 50 {
		t.Error("Matched field not set correctly")
	}
}

func TestResumeTokenFields(t *testing.T) {
	// Test ResumeToken struct fields
	filters := []filter.Filter{
		{Field: "test", Operator: "eq", Value: "value"},
	}

	token := ResumeToken{
		Database:    testDB,
		Network:     "1.0.0.0/8",
		LastNetwork: "1.1.0.0/16",
		FilterMode:  filterModeAnd,
		Filters:     filters,
		Processed:   100,
		Matched:     50,
		ResultIndex: 10,
	}

	if token.Database != testDB {
		t.Error("Database field not set correctly")
	}

	if token.Network != "1.0.0.0/8" {
		t.Error("Network field not set correctly")
	}

	if token.LastNetwork != "1.1.0.0/16" {
		t.Error("LastNetwork field not set correctly")
	}

	if token.FilterMode != filterModeAnd {
		t.Error("FilterMode field not set correctly")
	}

	if len(token.Filters) != 1 {
		t.Error("Filters field not set correctly")
	}

	if token.Processed != 100 {
		t.Error("Processed field not set correctly")
	}

	if token.Matched != 50 {
		t.Error("Matched field not set correctly")
	}

	if token.ResultIndex != 10 {
		t.Error("ResultIndex field not set correctly")
	}
}

func TestNetworkResultFields(t *testing.T) {
	// Test NetworkResult struct fields
	network, _ := netip.ParsePrefix("1.1.1.0/24")
	data := map[string]any{
		"country": map[string]any{
			"iso_code": "US",
		},
	}

	result := NetworkResult{
		Network: network,
		Data:    data,
	}

	if result.Network != network {
		t.Error("Network field not set correctly")
	}

	if result.Data == nil {
		t.Error("Data field should not be nil")
	}

	if len(result.Data) != 1 {
		t.Error("Data field not set correctly")
	}
}
