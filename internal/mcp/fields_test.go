package mcp

import (
	"reflect"
	"testing"

	"github.com/oschwald/maxminddb-mcp/internal/iterator"
)

func TestGenericRecordsAndFields(t *testing.T) {
	server := testServer(t)
	if err := server.dbManager.LoadDatabase("testdata/old.mmdb"); err != nil {
		t.Fatal(err)
	}
	call := modernTools(t, server)
	for _, ip := range []string{"10.0.1.1", "10.0.2.1"} {
		wireContent(t, call("lookup_ip", map[string]any{"ip": ip, "database": "old.mmdb"}))
	}
	wireContent(
		t,
		call(
			"lookup_ips",
			map[string]any{
				"ips":      []string{"10.0.1.1", "10.0.2.1"},
				"database": "old.mmdb",
				"fields":   []string{"$"},
			},
		),
	)
	wireContent(
		t,
		call("lookup_network", map[string]any{"network": "10.0.0.0/16", "database": "old.mmdb"}),
	)
	for _, tc := range []struct {
		ip     string
		fields []string
		want   any
	}{
		{"10.0.1.1", nil, "old"},
		{"10.0.1.1", []string{"$"}, map[string]any{"$": "old"}},
		{"10.0.2.1", nil, []any{uint64(7), map[string]any{"name": "array"}}},
		{"10.0.2.1", []string{"1.name", "missing"}, map[string]any{"1.name": "array"}},
		{"10.0.0.1", []string{"country"}, map[string]any{"country": "US"}},
		{"10.0.5.1", []string{"$"}, nil},
	} {
		result, err := server.handleLookupIP(t.Context(), networkRequest(map[string]any{
			"ip": tc.ip, "database": "old.mmdb", "fields": tc.fields,
		}))
		if err != nil || result.IsError {
			t.Fatalf("lookup: %+v, %v", result, err)
		}
		if got := result.StructuredContent.(map[string]any)["data"]; !reflect.DeepEqual(
			got,
			tc.want,
		) {
			t.Fatalf("%s: got %#v, want %#v", tc.ip, got, tc.want)
		}
	}
	for _, field := range []string{"country", "$", "1.name"} {
		value := any("US")
		if field == "$" {
			value = "old"
		}
		if field == "1.name" {
			value = "array"
		}
		args := map[string]any{
			"network": "10.0.0.0/16", "database": "old.mmdb", "fields": []string{field},
			"filters": []any{map[string]any{"field": field, "operator": "equals", "value": value}},
		}
		result, err := server.handleLookupNetwork(t.Context(), networkRequest(args))
		if err != nil || result.IsError {
			t.Fatalf("network: %+v, %v", result, err)
		}
		page := result.StructuredContent.(*iterator.IterationResult)
		if len(page.Results) != 1 ||
			!reflect.DeepEqual(page.Results[0].Data, map[string]any{field: value}) {
			t.Fatalf("unexpected filtered projection: %+v", page)
		}
	}
	for _, fields := range []any{nil, "country", []string{""}, []string{"a..b"}, make([]string, 65)} {
		result, err := server.handleLookupIP(
			t.Context(),
			networkRequest(map[string]any{"ip": "1.0.0.1", "fields": fields}),
		)
		assertToolError(t, result, err, "invalid_fields")
	}
}

func modernTools(t *testing.T, server *Server) func(string, map[string]any) map[string]any {
	t.Helper()
	send, receive := connectStdio(t, server)
	id := 0
	return func(name string, args map[string]any) map[string]any {
		t.Helper()
		id++
		send(
			map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"method":  "tools/call",
				"params": map[string]any{
					"name": name, "arguments": args, "_meta": map[string]any{
						"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
						"io.modelcontextprotocol/clientCapabilities": map[string]any{},
					},
				},
			},
		)
		response := receive()
		result, ok := response["result"].(map[string]any)
		if !ok || response["id"] != float64(id) {
			t.Fatalf("unexpected response: %v", response)
		}
		return result
	}
}
