package mcp

import "testing"

// TestFloatFilterMatchesPrintedValue checks that a float copied from a lookup
// result matches the MMDB float record it came from.
func TestFloatFilterMatchesPrintedValue(t *testing.T) {
	server := testServer(t)
	const database = "MaxMind-DB-test-decoder.mmdb"
	if err := server.dbManager.LoadDatabase("../../testdata/test-data/" + database); err != nil {
		t.Fatal(err)
	}
	call := modernTools(t, server)

	lookup := wireContent(
		t,
		call("lookup_ip", map[string]any{"ip": "1.1.1.1", "database": database}),
	)
	data, ok := lookup["data"].(map[string]any)
	if !ok {
		t.Fatalf("lookup data is %T, want an object", lookup["data"])
	}
	value, ok := data["float"].(float64)
	if !ok {
		t.Fatalf("float field is %T, want a number", data["float"])
	}
	if value != 1.1 {
		t.Fatalf("float field prints as %v, want 1.1", value)
	}

	for _, tc := range []struct {
		operator string
		value    any
		matches  bool
	}{
		{"equals", value, true},
		{"not_equals", value, false},
		{"in", []any{value}, true},
		{"not_in", []any{value}, false},
		{"greater_than_or_equal", value, true},
		{"less_than_or_equal", value, true},
		{"greater_than", value, false},
		{"less_than", value, false},
	} {
		page := wireContent(t, call("lookup_network", map[string]any{
			"network":  "1.1.1.1/32",
			"database": database,
			"filters": []any{
				map[string]any{"field": "float", "operator": tc.operator, "value": tc.value},
			},
		}))
		results, ok := page["results"].([]any)
		if !ok {
			t.Fatalf("results is %T, want an array", page["results"])
		}
		if got := len(results) == 1; got != tc.matches {
			t.Errorf(
				"float %s %v: got %d results, want match %v",
				tc.operator,
				value,
				len(results),
				tc.matches,
			)
		}
	}
}
