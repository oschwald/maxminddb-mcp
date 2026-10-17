package mcp

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sdk "github.com/mark3labs/mcp-go/mcp"
	"github.com/oschwald/maxminddb-mcp/internal/database"
	"github.com/oschwald/maxminddb-mcp/internal/iterator"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	db, err := database.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.LoadDatabase("../../testdata/test-data/GeoLite2-ASN-Test.mmdb"); err != nil {
		t.Fatal(err)
	}
	manager := iterator.New(time.Minute, time.Second)
	t.Cleanup(manager.StopCleanup)
	return New(createTestMCPConfig(t), db, nil, manager)
}

func networkRequest(args map[string]any) sdk.CallToolRequest {
	return sdk.CallToolRequest{Params: sdk.CallToolParams{Name: "lookup_network", Arguments: args}}
}

func assertToolError(t *testing.T, result *sdk.CallToolResult, err error, code string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || len(result.Content) == 0 {
		t.Fatalf("missing MCP error flag or text: %+v", result)
	}
	payload := result.StructuredContent.(map[string]any)["error"].(map[string]any)
	if payload["code"] != code {
		t.Fatalf("got %v, want %s", payload, code)
	}
}

func TestNetworkInputValidation(t *testing.T) {
	server := testServer(t)
	for _, value := range []any{-1, 0, 1.5, 1e100, math.Inf(1), math.NaN(), true, "1", nil, iterator.MaxResults + 1} {
		result, err := server.handleLookupNetwork(
			t.Context(),
			networkRequest(map[string]any{"network": "1.0.0.0/8", "max_results": value}),
		)
		assertToolError(t, result, err, "invalid_max_results")
	}
	result, err := server.handleLookupNetwork(
		t.Context(),
		networkRequest(map[string]any{"network": "1.0.0.0/8", "filter_mode": "typo"}),
	)
	assertToolError(t, result, err, "invalid_filter_mode")
	result, err = server.handleLookupNetwork(
		t.Context(),
		networkRequest(
			map[string]any{
				"network": "1.0.0.0/8",
				"filters": []any{map[string]any{"field": "asn", "operator": "equals"}},
			},
		),
	)
	assertToolError(t, result, err, "invalid_filter")
}

func TestNetworkContinuation(t *testing.T) {
	server := testServer(t)
	args := map[string]any{"network": "1.0.0.0/8", "max_results": 1}
	result, err := server.handleLookupNetwork(t.Context(), networkRequest(args))
	if err != nil || result.IsError {
		t.Fatalf("first page: %+v %v", result, err)
	}
	first := result.StructuredContent.(*iterator.IterationResult)
	if len(first.Results) != 1 || !first.HasMore {
		t.Fatalf("first page: %+v", first)
	}
	args["iterator_id"] = first.IteratorID
	for key, value := range map[string]any{"network": "2.0.0.0/8", "database": "other", "filter_mode": "or", "filters": []any{map[string]any{"field": "asn", "operator": "exists", "value": true}}} {
		old, exists := args[key]
		args[key] = value
		result, err = server.handleLookupNetwork(t.Context(), networkRequest(args))
		assertToolError(t, result, err, "query_mismatch")
		if exists {
			args[key] = old
		} else {
			delete(args, key)
		}
	}
	result, err = server.handleLookupNetwork(t.Context(), networkRequest(args))
	if err != nil || result.IsError {
		t.Fatalf("second page: %+v %v", result, err)
	}
	second := result.StructuredContent.(*iterator.IterationResult)
	if len(second.Results) != 1 || second.Results[0].Network == first.Results[0].Network {
		t.Fatal("continuation repeated first result")
	}
	server.iterMgr.RemoveIterator(first.IteratorID)
	result, err = server.handleLookupNetwork(t.Context(), networkRequest(args))
	assertToolError(t, result, err, "iterator_expired")
	args["resume_token"] = first.ResumeToken
	result, err = server.handleLookupNetwork(t.Context(), networkRequest(args))
	if err != nil || result.IsError {
		t.Fatalf("token fallback: %+v %v", result, err)
	}
	resumed := result.StructuredContent.(*iterator.IterationResult)
	if resumed.Results[0].Network != second.Results[0].Network {
		t.Fatal("token resumed at wrong position")
	}
}

func TestAmbiguousDatabase(t *testing.T) {
	server := testServer(t)
	if err := server.dbManager.LoadDatabase(testCityDB); err != nil {
		t.Fatal(err)
	}
	result, err := server.handleLookupNetwork(
		t.Context(),
		networkRequest(map[string]any{"network": "1.0.0.0/8"}),
	)
	assertToolError(t, result, err, "database_required")
}

func TestLookupErrors(t *testing.T) {
	server := testServer(t)
	result, err := server.handleLookupIP(
		t.Context(),
		sdk.CallToolRequest{Params: sdk.CallToolParams{Arguments: map[string]any{"ip": "bad"}}},
	)
	assertToolError(t, result, err, "invalid_ip")
	result, err = server.handleUpdateDatabases(t.Context(), sdk.CallToolRequest{})
	assertToolError(t, result, err, "updates_not_available")
}

func TestPartialUpdateFailure(t *testing.T) {
	endpoint := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
		),
	)
	defer endpoint.Close()
	server := testServer(t)
	cfg := server.config
	cfg.Mode = "maxmind"
	cfg.MaxMind.AccountID = 1
	cfg.MaxMind.LicenseKey = "test"
	cfg.MaxMind.Editions = []string{"GeoLite2-ASN"}
	cfg.MaxMind.Endpoint = endpoint.URL
	var err error
	server.updater, err = database.NewUpdater(cfg, server.dbManager)
	if err != nil {
		t.Fatal(err)
	}
	result, err := server.handleUpdateDatabases(t.Context(), sdk.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("failed download reported as success")
	}
	updates := result.StructuredContent.(map[string]any)["results"].([]database.UpdateResult)
	if len(updates) != 1 || updates[0].Error == "" {
		t.Fatalf("missing edition error: %+v", updates)
	}
}
