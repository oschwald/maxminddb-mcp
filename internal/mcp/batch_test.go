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
