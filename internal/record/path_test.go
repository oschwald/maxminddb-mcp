package record

import (
	"reflect"
	"strings"
	"testing"
)

func TestLookup(t *testing.T) {
	value := map[string]any{"items": []any{map[string]any{"name": "first"}}, "null": nil}
	for _, tc := range []struct {
		path  string
		want  any
		found bool
	}{
		{"items.0.name", "first", true},
		{"null", nil, true},
		{"items.1", nil, false},
		{"items.-1", nil, false},
		{"items.00", nil, false},
		{"items.x", nil, false},
		{"items.0.name.x", nil, false},
		{"missing", nil, false},
		{"$", value, true},
	} {
		got, found := Lookup(value, strings.Split(tc.path, "."))
		if found != tc.found || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: %v, %t", tc.path, got, found)
		}
	}
}
