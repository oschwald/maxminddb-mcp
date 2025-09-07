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

const (
	testDB        = "test-db"
	testNetwork   = "1.0.0.0/8"
	filterModeAnd = "and"
)

func TestNewSimple(t *testing.T) {
	ttl := 30 * time.Minute
	cleanupInterval := 5 * time.Minute

	manager := NewSimple(ttl, cleanupInterval)

	if manager == nil {
		t.Fatal("Simple manager should not be nil")
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

func TestCreateSimpleIterator(t *testing.T) {
	manager := NewSimple(30*time.Minute, 5*time.Minute)

	network, err := netip.ParsePrefix(testNetwork)
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

	iterator, err := manager.CreateSimpleIterator(
		testDB,
		network,
		filters,
		filterModeAnd,
	)
	if err != nil {
		t.Fatalf("Failed to create simple iterator: %v", err)
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

	if iterator.Created.IsZero() {
		t.Error("Created time should be set")
	}

	if iterator.LastAccess.IsZero() {
		t.Error("Last access time should be set")
	}

	if iterator.Processed != 0 {
		t.Error("Processed count should start at 0")
	}

	if iterator.Matched != 0 {
		t.Error("Matched count should start at 0")
	}
}

func TestCreateSimpleIteratorWithoutFilters(t *testing.T) {
	manager := NewSimple(30*time.Minute, 5*time.Minute)

	network, err := netip.ParsePrefix(testNetwork)
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	iterator, err := manager.CreateSimpleIterator(
		testDB,
		network,
		nil, // No filters
		"",  // No filter mode
	)
	if err != nil {
		t.Fatalf("Failed to create simple iterator without filters: %v", err)
	}

	if iterator.FilterEngine != nil {
		t.Error("Filter engine should be nil when no filters are provided")
	}

	if len(iterator.Filters) != 0 {
		t.Error("Filters slice should be empty when no filters provided")
	}
}

func TestIterateSimple(t *testing.T) {
	manager := NewSimple(30*time.Minute, 5*time.Minute)

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

	iterator, err := manager.CreateSimpleIterator("asn-test", network, nil, "")
	if err != nil {
		t.Fatalf("Failed to create simple iterator: %v", err)
	}

	// Test iteration
	result, err := manager.IterateSimple(reader, iterator, 10)
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

	if result.ResumeToken == "" {
		t.Error("Resume token should not be empty")
	}

	// Test with nil reader (should error)
	_, err = manager.IterateSimple(nil, iterator, 10)
	if err == nil {
		t.Error("Expected error when iterating with nil reader")
	}

	// Test with nil iterator (should error)
	_, err = manager.IterateSimple(reader, nil, 10)
	if err == nil {
		t.Error("Expected error when iterating with nil iterator")
	}
}

func TestIterateSimpleWithFilters(t *testing.T) {
	manager := NewSimple(30*time.Minute, 5*time.Minute)

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

	iterator, err := manager.CreateSimpleIterator("asn-test", network, filters, filterModeAnd)
	if err != nil {
		t.Fatalf("Failed to create simple iterator: %v", err)
	}

	result, err := manager.IterateSimple(reader, iterator, 10)
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

func TestGenerateSimpleResumeToken(t *testing.T) {
	manager := NewSimple(30*time.Minute, 5*time.Minute)

	network, err := netip.ParsePrefix(testNetwork)
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

	iterator, err := manager.CreateSimpleIterator(testDB, network, filters, filterModeAnd)
	if err != nil {
		t.Fatalf("Failed to create simple iterator: %v", err)
	}

	// Set some state
	iterator.Processed = 100
	iterator.Matched = 50
	iterator.LastNetwork, _ = netip.ParsePrefix("1.1.1.0/24")

	// Generate resume token
	token, err := manager.generateSimpleResumeToken(iterator)
	if err != nil {
		t.Fatalf("Failed to generate simple resume token: %v", err)
	}

	if token == "" {
		t.Error("Resume token should not be empty")
	}

	// Decode and verify token
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("Failed to decode resume token: %v", err)
	}

	var resumeToken ResumeToken
	err = json.Unmarshal(decoded, &resumeToken)
	if err != nil {
		t.Fatalf("Failed to unmarshal resume token: %v", err)
	}

	if resumeToken.Database != testDB {
		t.Errorf("Expected database 'test-db', got '%s'", resumeToken.Database)
	}

	if resumeToken.Network != testNetwork {
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

	if resumeToken.LastNetwork != "1.1.1.0/24" {
		t.Errorf("Expected last network '1.1.1.0/24', got '%s'", resumeToken.LastNetwork)
	}
}

func TestGenerateSimpleResumeTokenNoLastNetwork(t *testing.T) {
	manager := NewSimple(30*time.Minute, 5*time.Minute)

	network, err := netip.ParsePrefix(testNetwork)
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	iterator, err := manager.CreateSimpleIterator(testDB, network, nil, "")
	if err != nil {
		t.Fatalf("Failed to create simple iterator: %v", err)
	}

	// Don't set LastNetwork

	// Generate resume token
	token, err := manager.generateSimpleResumeToken(iterator)
	if err != nil {
		t.Fatalf("Failed to generate simple resume token: %v", err)
	}

	// Decode and verify token
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("Failed to decode resume token: %v", err)
	}

	var resumeToken ResumeToken
	err = json.Unmarshal(decoded, &resumeToken)
	if err != nil {
		t.Fatalf("Failed to unmarshal resume token: %v", err)
	}

	if resumeToken.LastNetwork != "" {
		t.Errorf("Expected empty last network, got '%s'", resumeToken.LastNetwork)
	}
}

func TestGenerateSimpleID(t *testing.T) {
	// Test generating multiple IDs
	ids := make(map[string]bool)

	for range 100 {
		id, err := generateSimpleID()
		if err != nil {
			t.Fatalf("Failed to generate simple ID: %v", err)
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

func TestSimpleIteratorFields(t *testing.T) {
	// Test SimpleIterator struct fields
	now := time.Now()
	network, _ := netip.ParsePrefix(testNetwork)
	lastNetwork, _ := netip.ParsePrefix("1.1.0.0/16")

	filters := []filter.Filter{
		{Field: "test", Operator: "eq", Value: "value"},
	}

	iterator := &SimpleIterator{
		ID:           "test-id",
		Database:     testDB,
		Network:      network,
		LastNetwork:  lastNetwork,
		FilterMode:   filterModeAnd,
		Filters:      filters,
		Created:      now,
		LastAccess:   now,
		Processed:    100,
		Matched:      50,
		FilterEngine: filter.New(filters, filter.ModeAnd),
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

	if len(iterator.Filters) != 1 {
		t.Error("Filters field not set correctly")
	}

	if iterator.Processed != 100 {
		t.Error("Processed field not set correctly")
	}

	if iterator.Matched != 50 {
		t.Error("Matched field not set correctly")
	}

	if iterator.FilterEngine == nil {
		t.Error("FilterEngine should not be nil")
	}
}

func TestSimpleManagerConcurrency(t *testing.T) {
	manager := NewSimple(30*time.Minute, 5*time.Minute)

	network, err := netip.ParsePrefix(testNetwork)
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	// Test concurrent creation
	done := make(chan bool, 2)

	go func() {
		defer func() { done <- true }()
		_, err := manager.CreateSimpleIterator("test-db-1", network, nil, "")
		if err != nil {
			t.Errorf("Failed to create iterator in goroutine: %v", err)
		}
	}()

	go func() {
		defer func() { done <- true }()
		_, err := manager.CreateSimpleIterator("test-db-2", network, nil, "")
		if err != nil {
			t.Errorf("Failed to create iterator in goroutine: %v", err)
		}
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Should have created two iterators
	manager.mu.RLock()
	if len(manager.iterators) != 2 {
		t.Errorf("Expected 2 iterators, got %d", len(manager.iterators))
	}
	manager.mu.RUnlock()
}

func TestSimpleIteratorWithNetworksWithin(t *testing.T) {
	manager := NewSimple(30*time.Minute, 5*time.Minute)

	reader, err := maxminddb.Open("../../testdata/test-data/MaxMind-DB-test-decoder.mmdb")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer func() { _ = reader.Close() }()

	// Use a network that should have data
	network, err := netip.ParsePrefix("1.1.1.0/24")
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	iterator, err := manager.CreateSimpleIterator("decoder-test", network, nil, "")
	if err != nil {
		t.Fatalf("Failed to create simple iterator: %v", err)
	}

	// Test iteration using the v2 API
	result, err := manager.IterateSimple(reader, iterator, 5)
	if err != nil {
		t.Fatalf("Failed to iterate: %v", err)
	}

	if result == nil {
		t.Fatal("Result should not be nil")
	}

	// The iterator should have processed some networks
	// Even if no data is found, the processed count should be >= 0
	if result.TotalProcessed < 0 {
		t.Error("Total processed should not be negative")
	}

	// Update last access time
	if !iterator.LastAccess.After(iterator.Created) {
		t.Error("Last access time should be updated after iteration")
	}
}

func TestSimpleManagerMapAccess(t *testing.T) {
	manager := NewSimple(30*time.Minute, 5*time.Minute)

	network, err := netip.ParsePrefix(testNetwork)
	if err != nil {
		t.Fatalf("Failed to parse network: %v", err)
	}

	// Create iterator
	iterator, err := manager.CreateSimpleIterator(testDB, network, nil, "")
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}

	// Test direct map access (normally this would be through exported methods)
	manager.mu.RLock()
	storedIterator, exists := manager.iterators[iterator.ID]
	manager.mu.RUnlock()

	if !exists {
		t.Error("Iterator should exist in manager's map")
	}

	if storedIterator.ID != iterator.ID {
		t.Error("Stored iterator should match created iterator")
	}
}

func TestSimpleIteratorInvalidNetwork(t *testing.T) {
	manager := NewSimple(30*time.Minute, 5*time.Minute)

	// Test with zero value network (should still work)
	var network netip.Prefix

	iterator, err := manager.CreateSimpleIterator(testDB, network, nil, "")
	if err != nil {
		t.Fatalf("Failed to create iterator with zero network: %v", err)
	}

	if iterator == nil {
		t.Fatal("Iterator should not be nil even with zero network")
	}

	// The network should be the zero value
	if iterator.Network.IsValid() {
		t.Error("Network should be invalid/zero value")
	}
}
