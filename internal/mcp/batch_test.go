package mcp

import (
	"context"
	"testing"
)

func TestBatchLookup(t *testing.T) {
	server := testServer(t)
	for _, db := range []string{"", "GeoLite2-ASN-Test.mmdb"} {
		ips := []string{"1.0.0.1", "invalid", "127.0.0.1", "1.0.0.1"}
		result, err := server.handleLookupIPs(
			t.Context(),
			networkRequest(map[string]any{"ips": ips, "database": db}),
		)
		if err != nil || !result.IsError {
			t.Fatalf("expected partial failure: %+v, %v", result, err)
		}
		entries := result.StructuredContent.(map[string]any)["results"].([]map[string]any)
		if len(entries) != len(ips) {
			t.Fatalf("got %d results", len(entries))
		}
		for i, entry := range entries {
			if entry["ip"] != ips[i] || (entry["error"] != nil) != (i == 1) {
				t.Fatalf("incorrect result at %d: %v", i, entry)
			}
			if i == 1 {
				continue
			}
			if db == "" {
				entry = entry["databases"].(map[string]any)["GeoLite2-ASN-Test.mmdb"].(map[string]any)
			}
			if entry["found"] != (i != 2) {
				t.Fatalf("incorrect found flag: %v", entry)
			}
		}
	}
	for _, ips := range []any{nil, []string{}, make([]string, 101), []any{true}, "1.0.0.1"} {
		result, err := server.handleLookupIPs(
			t.Context(),
			networkRequest(map[string]any{"ips": ips}),
		)
		assertToolError(t, result, err, "invalid_ips")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := server.handleLookupIPs(
		ctx,
		networkRequest(map[string]any{"ips": []string{"1.0.0.1"}}),
	)
	assertToolError(t, result, err, "canceled")
}

func TestBatchLookupIgnoresIPv4OnlyDatabaseForIPv6(t *testing.T) {
	server := serverWithDatabases(t, "GeoIP2-City-Test.mmdb", "MaxMind-DB-test-ipv4-24.mmdb")
	result, err := server.handleLookupIPs(
		t.Context(),
		networkRequest(map[string]any{"ips": []string{"2001:218::1", "1.1.1.1"}}),
	)
	if err != nil || result.IsError {
		t.Fatalf("unexpected failure: %+v, %v", result, err)
	}
	for _, entry := range result.StructuredContent.(map[string]any)["results"].([]map[string]any) {
		if entry["error"] != nil || entry["databases"] == nil {
			t.Fatalf("expected per-database results: %v", entry)
		}
	}
}
