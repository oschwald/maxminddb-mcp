// Package filter provides filtering functionality for MaxMind database queries.
package filter

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// Filter represents a single filter condition.
type Filter struct {
	Value    any    `json:"value"`
	Field    string `json:"field"`
	Operator string `json:"operator"`
}

// Mode represents how multiple filters should be combined.
type Mode string

const (
	// ModeAnd combines filters with AND logic (all must match).
	ModeAnd Mode = "and"
	// ModeOr combines filters with OR logic (any can match).
	ModeOr Mode = "or"
)

// SupportedOperators returns the list of supported filter operators.
func SupportedOperators() []string {
	return []string{
		"equals",
		"not_equals",
		"in",
		"not_in",
		"contains",
		"regex",
		"greater_than",
		"greater_than_or_equal",
		"less_than",
		"less_than_or_equal",
		"exists",
	}
}

// Engine handles filter evaluation.
type Engine struct {
	mode    Mode
	filters []Filter
}

// New creates a new filter engine.
func New(filters []Filter, mode Mode) *Engine {
	if mode == "" {
		mode = ModeAnd
	}
	return &Engine{
		filters: filters,
		mode:    mode,
	}
}

// Matches evaluates all filters against the given data.
func (e *Engine) Matches(data map[string]any) bool {
	if len(e.filters) == 0 {
		return true // No filters means everything matches
	}

	switch e.mode {
	case ModeAnd:
		for _, filter := range e.filters {
			if !e.evaluateFilter(filter, data) {
				return false
			}
		}
		return true
	case ModeOr:
		for _, filter := range e.filters {
			if e.evaluateFilter(filter, data) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// evaluateFilter evaluates a single filter against the data.
func (*Engine) evaluateFilter(filter Filter, data map[string]any) bool {
	fieldValue := getNestedField(data, filter.Field)

	switch filter.Operator {
	case "equals":
		return compareEqual(fieldValue, filter.Value)
	case "not_equals":
		return !compareEqual(fieldValue, filter.Value)
	case "in":
		return containsValue(filter.Value, fieldValue)
	case "not_in":
		return !containsValue(filter.Value, fieldValue)
	case "contains":
		return containsString(fieldValue, filter.Value)
	case "regex":
		return matchesRegex(fieldValue, filter.Value)
	case "greater_than":
		return compareGreater(fieldValue, filter.Value)
	case "greater_than_or_equal":
		return compareGreaterEqual(fieldValue, filter.Value)
	case "less_than":
		return compareLess(fieldValue, filter.Value)
	case "less_than_or_equal":
		return compareLessEqual(fieldValue, filter.Value)
	case "exists":
		exists := fieldValue != nil
		if boolValue, ok := filter.Value.(bool); ok {
			return exists == boolValue
		}
		return exists
	default:
		return false
	}
}

// getNestedField retrieves a nested field from a map using dot notation.
func getNestedField(data map[string]any, fieldPath string) any {
	parts := strings.Split(fieldPath, ".")
	current := data

	for i, part := range parts {
		if current == nil {
			return nil
		}

		value, exists := current[part]
		if !exists {
			return nil
		}

		// If this is the last part, return the value
		if i == len(parts)-1 {
			return value
		}

		// Otherwise, try to continue with nested map
		nextMap, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		current = nextMap
	}

	return nil
}

// compareEqual compares two values for equality.
func compareEqual(fieldValue, filterValue any) bool {
	return reflect.DeepEqual(fieldValue, filterValue)
}

// containsValue checks if a value is in an array.
func containsValue(filterValue, fieldValue any) bool {
	// filterValue should be an array
	filterArray, ok := filterValue.([]any)
	if !ok {
		return false
	}

	for _, item := range filterArray {
		if compareEqual(fieldValue, item) {
			return true
		}
	}

	return false
}

// containsString checks if a string contains a substring.
func containsString(fieldValue, filterValue any) bool {
	fieldStr, ok1 := fieldValue.(string)
	filterStr, ok2 := filterValue.(string)

	if !ok1 || !ok2 {
		return false
	}

	return strings.Contains(fieldStr, filterStr)
}

// matchesRegex checks if a string matches a regular expression.
func matchesRegex(fieldValue, filterValue any) bool {
	fieldStr, ok1 := fieldValue.(string)
	regexStr, ok2 := filterValue.(string)

	if !ok1 || !ok2 {
		return false
	}

	regex, err := regexp.Compile(regexStr)
	if err != nil {
		return false
	}

	return regex.MatchString(fieldStr)
}

// compareGreater compares if fieldValue > filterValue.
func compareGreater(fieldValue, filterValue any) bool {
	fieldNum, err1 := toFloat64(fieldValue)
	filterNum, err2 := toFloat64(filterValue)

	if err1 != nil || err2 != nil {
		return false
	}

	return fieldNum > filterNum
}

// compareLess compares if fieldValue < filterValue.
func compareLess(fieldValue, filterValue any) bool {
	fieldNum, err1 := toFloat64(fieldValue)
	filterNum, err2 := toFloat64(filterValue)

	if err1 != nil || err2 != nil {
		return false
	}

	return fieldNum < filterNum
}

// compareGreaterEqual compares if fieldValue >= filterValue.
func compareGreaterEqual(fieldValue, filterValue any) bool {
	fieldNum, err1 := toFloat64(fieldValue)
	filterNum, err2 := toFloat64(filterValue)

	if err1 != nil || err2 != nil {
		return false
	}

	return fieldNum >= filterNum
}

// compareLessEqual compares if fieldValue <= filterValue.
func compareLessEqual(fieldValue, filterValue any) bool {
	fieldNum, err1 := toFloat64(fieldValue)
	filterNum, err2 := toFloat64(filterValue)

	if err1 != nil || err2 != nil {
		return false
	}

	return fieldNum <= filterNum
}

// toFloat64 converts various numeric types to float64.
func toFloat64(value any) (float64, error) {
	switch v := value.(type) {
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", value)
	}
}

// Validate validates a filter configuration.
func Validate(filters []Filter) error {
	supportedOps := make(map[string]bool)
	for _, op := range SupportedOperators() {
		supportedOps[op] = true
	}

	for i, filter := range filters {
		if filter.Field == "" {
			return fmt.Errorf("filter %d: field cannot be empty", i)
		}

		if !supportedOps[filter.Operator] {
			return fmt.Errorf("filter %d: unsupported operator '%s'", i, filter.Operator)
		}

		// Validate operator-specific requirements
		switch filter.Operator {
		case "in", "not_in":
			if _, ok := filter.Value.([]any); !ok {
				return fmt.Errorf(
					"filter %d: operator '%s' requires an array value",
					i,
					filter.Operator,
				)
			}
		case "regex":
			regexStr, ok := filter.Value.(string)
			if !ok {
				return fmt.Errorf("filter %d: regex operator requires a string value", i)
			}
			if _, err := regexp.Compile(regexStr); err != nil {
				return fmt.Errorf("filter %d: invalid regex '%s': %w", i, regexStr, err)
			}
		case "exists":
			if _, ok := filter.Value.(bool); !ok {
				return fmt.Errorf("filter %d: exists operator requires a boolean value", i)
			}
		default:
			// Other operators (eq, ne, lt, le, gt, ge, contains, starts_with, ends_with)
			// don't require specific value type validation
		}
	}

	return nil
}
