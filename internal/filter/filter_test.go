package filter

import (
	"slices"
	"testing"
)

func TestFilterEngine(t *testing.T) {
	testData := map[string]any{
		"country": map[string]any{
			"iso_code": "US",
			"names": map[string]any{
				"en": "United States",
			},
		},
		"traits": map[string]any{
			"user_type":                "residential",
			"is_anonymous_proxy":       false,
			"autonomous_system_number": 7922,
		},
		"isp": "Comcast Cable Communications",
	}

	tests := []struct {
		name        string
		filters     []Filter
		mode        Mode
		shouldMatch bool
	}{
		{
			name:        "no filters - should match",
			filters:     []Filter{},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "equals match",
			filters: []Filter{
				{Field: "country.iso_code", Operator: "equals", Value: "US"},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "equals no match",
			filters: []Filter{
				{Field: "country.iso_code", Operator: "equals", Value: "CA"},
			},
			mode:        ModeAnd,
			shouldMatch: false,
		},
		{
			name: "not_equals match",
			filters: []Filter{
				{Field: "country.iso_code", Operator: "not_equals", Value: "CA"},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "not_equals no match",
			filters: []Filter{
				{Field: "country.iso_code", Operator: "not_equals", Value: "US"},
			},
			mode:        ModeAnd,
			shouldMatch: false,
		},
		{
			name: "in match",
			filters: []Filter{
				{Field: "country.iso_code", Operator: "in", Value: []any{"US", "CA", "MX"}},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "in no match",
			filters: []Filter{
				{Field: "country.iso_code", Operator: "in", Value: []any{"CA", "MX", "GB"}},
			},
			mode:        ModeAnd,
			shouldMatch: false,
		},
		{
			name: "not_in match",
			filters: []Filter{
				{
					Field:    "country.iso_code",
					Operator: "not_in",
					Value:    []any{"CA", "MX", "GB"},
				},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "not_in no match",
			filters: []Filter{
				{
					Field:    "country.iso_code",
					Operator: "not_in",
					Value:    []any{"US", "CA", "MX"},
				},
			},
			mode:        ModeAnd,
			shouldMatch: false,
		},
		{
			name: "contains match",
			filters: []Filter{
				{Field: "isp", Operator: "contains", Value: "Comcast"},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "contains no match",
			filters: []Filter{
				{Field: "isp", Operator: "contains", Value: "Verizon"},
			},
			mode:        ModeAnd,
			shouldMatch: false,
		},
		{
			name: "regex match",
			filters: []Filter{
				{Field: "isp", Operator: "regex", Value: "^Comcast.*Communications$"},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "regex no match",
			filters: []Filter{
				{Field: "isp", Operator: "regex", Value: "^Verizon"},
			},
			mode:        ModeAnd,
			shouldMatch: false,
		},
		{
			name: "greater_than match",
			filters: []Filter{
				{Field: "traits.autonomous_system_number", Operator: "greater_than", Value: 7000},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "greater_than no match",
			filters: []Filter{
				{Field: "traits.autonomous_system_number", Operator: "greater_than", Value: 8000},
			},
			mode:        ModeAnd,
			shouldMatch: false,
		},
		{
			name: "less_than match",
			filters: []Filter{
				{Field: "traits.autonomous_system_number", Operator: "less_than", Value: 8000},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "less_than no match",
			filters: []Filter{
				{Field: "traits.autonomous_system_number", Operator: "less_than", Value: 7000},
			},
			mode:        ModeAnd,
			shouldMatch: false,
		},
		{
			name: "exists match",
			filters: []Filter{
				{Field: "country.iso_code", Operator: "exists", Value: true},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "exists no match",
			filters: []Filter{
				{Field: "nonexistent.field", Operator: "exists", Value: true},
			},
			mode:        ModeAnd,
			shouldMatch: false,
		},
		{
			name: "exists false match",
			filters: []Filter{
				{Field: "nonexistent.field", Operator: "exists", Value: false},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "multiple filters AND - all match",
			filters: []Filter{
				{Field: "country.iso_code", Operator: "equals", Value: "US"},
				{Field: "traits.user_type", Operator: "equals", Value: "residential"},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "multiple filters AND - one doesn't match",
			filters: []Filter{
				{Field: "country.iso_code", Operator: "equals", Value: "US"},
				{Field: "traits.user_type", Operator: "equals", Value: "business"},
			},
			mode:        ModeAnd,
			shouldMatch: false,
		},
		{
			name: "multiple filters OR - one matches",
			filters: []Filter{
				{Field: "country.iso_code", Operator: "equals", Value: "CA"},
				{Field: "traits.user_type", Operator: "equals", Value: "residential"},
			},
			mode:        ModeOr,
			shouldMatch: true,
		},
		{
			name: "multiple filters OR - none match",
			filters: []Filter{
				{Field: "country.iso_code", Operator: "equals", Value: "CA"},
				{Field: "traits.user_type", Operator: "equals", Value: "business"},
			},
			mode:        ModeOr,
			shouldMatch: false,
		},
		{
			name: "nested field access",
			filters: []Filter{
				{Field: "country.names.en", Operator: "equals", Value: "United States"},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "boolean field",
			filters: []Filter{
				{Field: "traits.is_anonymous_proxy", Operator: "equals", Value: false},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := New(test.filters, test.mode)
			matches := engine.Matches(testData)

			if matches != test.shouldMatch {
				t.Errorf("Expected match=%t, got match=%t", test.shouldMatch, matches)
			}
		})
	}
}

func TestFilterEngineComparisonOperators(t *testing.T) {
	testData := map[string]any{
		"traits": map[string]any{
			"autonomous_system_number": 7922,
		},
	}

	tests := []struct {
		name        string
		filters     []Filter
		mode        Mode
		shouldMatch bool
	}{
		{
			name: "greater_than_or_equal match",
			filters: []Filter{
				{
					Field:    "traits.autonomous_system_number",
					Operator: "greater_than_or_equal",
					Value:    7922,
				},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "greater_than_or_equal exact match",
			filters: []Filter{
				{
					Field:    "traits.autonomous_system_number",
					Operator: "greater_than_or_equal",
					Value:    7922,
				},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "greater_than_or_equal no match",
			filters: []Filter{
				{
					Field:    "traits.autonomous_system_number",
					Operator: "greater_than_or_equal",
					Value:    8000,
				},
			},
			mode:        ModeAnd,
			shouldMatch: false,
		},
		{
			name: "less_than_or_equal match",
			filters: []Filter{
				{
					Field:    "traits.autonomous_system_number",
					Operator: "less_than_or_equal",
					Value:    8000,
				},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "less_than_or_equal exact match",
			filters: []Filter{
				{
					Field:    "traits.autonomous_system_number",
					Operator: "less_than_or_equal",
					Value:    7922,
				},
			},
			mode:        ModeAnd,
			shouldMatch: true,
		},
		{
			name: "less_than_or_equal no match",
			filters: []Filter{
				{
					Field:    "traits.autonomous_system_number",
					Operator: "less_than_or_equal",
					Value:    7000,
				},
			},
			mode:        ModeAnd,
			shouldMatch: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := New(test.filters, test.mode)
			result := engine.Matches(testData)

			if result != test.shouldMatch {
				t.Errorf("Expected %v, got %v", test.shouldMatch, result)
			}
		})
	}
}

func TestGetNestedField(t *testing.T) {
	data := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": "deep_value",
			},
			"array": []any{"item1", "item2"},
		},
		"simple": "simple_value",
	}

	tests := []struct {
		field    string
		expected any
	}{
		{"simple", "simple_value"},
		{"level1.level2.level3", "deep_value"},
		{"level1.level2", map[string]any{"level3": "deep_value"}},
		{"nonexistent", nil},
		{"level1.nonexistent", nil},
		{"level1.level2.level3.tooDeep", nil},
		{"level1.array", []any{"item1", "item2"}},
	}

	for _, test := range tests {
		t.Run(test.field, func(t *testing.T) {
			result := getNestedField(data, test.field)
			if !compareEqual(result, test.expected) {
				t.Errorf("Expected %v, got %v", test.expected, result)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		input    any
		expected float64
		hasError bool
	}{
		{int(42), 42.0, false},
		{int8(42), 42.0, false},
		{int16(42), 42.0, false},
		{int32(42), 42.0, false},
		{int64(42), 42.0, false},
		{uint(42), 42.0, false},
		{uint8(42), 42.0, false},
		{uint16(42), 42.0, false},
		{uint32(42), 42.0, false},
		{uint64(42), 42.0, false},
		{float32(42.5), 42.5, false},
		{float64(42.5), 42.5, false},
		{"42.5", 42.5, false},
		{"invalid", 0, true},
		{true, 0, true},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			result, err := toFloat64(test.input)

			if test.hasError {
				if err == nil {
					t.Errorf("Expected error for input %v, got nil", test.input)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for input %v, got %v", test.input, err)
				}
				if result != test.expected {
					t.Errorf("Expected %v, got %v", test.expected, result)
				}
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		filters     []Filter
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid filters",
			filters:     []Filter{{Field: "test", Operator: "equals", Value: "value"}},
			expectError: false,
		},
		{
			name:        "empty field",
			filters:     []Filter{{Field: "", Operator: "equals", Value: "value"}},
			expectError: true,
			errorMsg:    "filter 0: field cannot be empty",
		},
		{
			name:        "invalid operator",
			filters:     []Filter{{Field: "test", Operator: "invalid", Value: "value"}},
			expectError: true,
			errorMsg:    "filter 0: unsupported operator 'invalid'",
		},
		{
			name:        "in operator with non-array",
			filters:     []Filter{{Field: "test", Operator: "in", Value: "not_array"}},
			expectError: true,
			errorMsg:    "filter 0: operator 'in' requires an array value",
		},
		{
			name:        "in operator with array",
			filters:     []Filter{{Field: "test", Operator: "in", Value: []any{"a", "b"}}},
			expectError: false,
		},
		{
			name:        "not_in operator with non-array",
			filters:     []Filter{{Field: "test", Operator: "not_in", Value: "not_array"}},
			expectError: true,
			errorMsg:    "filter 0: operator 'not_in' requires an array value",
		},
		{
			name:        "regex operator with invalid regex",
			filters:     []Filter{{Field: "test", Operator: "regex", Value: "[invalid"}},
			expectError: true,
		},
		{
			name:        "regex operator with non-string",
			filters:     []Filter{{Field: "test", Operator: "regex", Value: 123}},
			expectError: true,
			errorMsg:    "filter 0: regex operator requires a string value",
		},
		{
			name:        "exists operator with non-boolean",
			filters:     []Filter{{Field: "test", Operator: "exists", Value: "not_bool"}},
			expectError: true,
			errorMsg:    "filter 0: exists operator requires a boolean value",
		},
		{
			name:        "exists operator with boolean",
			filters:     []Filter{{Field: "test", Operator: "exists", Value: true}},
			expectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.filters)

			if test.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
					return
				}
				if test.errorMsg != "" && err.Error() != test.errorMsg {
					t.Errorf("Expected error '%s', got '%s'", test.errorMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	}
}

func TestSupportedOperators(t *testing.T) {
	operators := SupportedOperators()

	expectedOperators := []string{
		"equals", "not_equals", "in", "not_in", "contains",
		"regex", "greater_than", "greater_than_or_equal", "less_than", "less_than_or_equal", "exists",
	}

	if len(operators) != len(expectedOperators) {
		t.Errorf("Expected %d operators, got %d", len(expectedOperators), len(operators))
	}

	for _, expected := range expectedOperators {
		found := slices.Contains(operators, expected)
		if !found {
			t.Errorf("Expected operator '%s' not found", expected)
		}
	}
}

func TestFilterEngineEdgeCases(t *testing.T) {
	data := map[string]any{
		"empty_string": "",
		"zero_int":     0,
		"null_field":   nil,
	}

	tests := []struct {
		name        string
		filter      Filter
		shouldMatch bool
	}{
		{
			name:        "equals with empty string",
			filter:      Filter{Field: "empty_string", Operator: "equals", Value: ""},
			shouldMatch: true,
		},
		{
			name:        "equals with zero",
			filter:      Filter{Field: "zero_int", Operator: "equals", Value: 0},
			shouldMatch: true,
		},
		{
			name:        "exists with null field",
			filter:      Filter{Field: "null_field", Operator: "exists", Value: false},
			shouldMatch: true,
		},
		{
			name:        "invalid regex should not match",
			filter:      Filter{Field: "empty_string", Operator: "regex", Value: "[invalid"},
			shouldMatch: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := New([]Filter{test.filter}, ModeAnd)
			matches := engine.Matches(data)

			if matches != test.shouldMatch {
				t.Errorf("Expected match=%t, got match=%t", test.shouldMatch, matches)
			}
		})
	}
}
