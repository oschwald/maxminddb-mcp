// Package filter provides filtering functionality for MaxMind database queries.
package filter

import (
	"cmp"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/oschwald/maxminddb-mcp/internal/record"
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
	filters []preparedFilter
}

type preparedFilter struct {
	Filter
	pattern *regexp.Regexp
	path    []string
}

// New creates a new filter engine.
func New(filters []Filter, mode Mode) *Engine {
	if mode == "" {
		mode = ModeAnd
	}
	prepared := make([]preparedFilter, len(filters))
	for i, f := range filters {
		prepared[i] = preparedFilter{Filter: f, path: strings.Split(f.Field, ".")}
		if f.Operator == "regex" {
			if expression, ok := f.Value.(string); ok {
				if pattern, err := regexp.Compile(expression); err == nil {
					prepared[i].pattern = pattern
				}
			}
		}
	}
	return &Engine{filters: prepared, mode: mode}
}

// Matches evaluates all filters against the given data.
func (e *Engine) Matches(data any) bool {
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
func (*Engine) evaluateFilter(filter preparedFilter, data any) bool {
	fieldValue, _ := record.Lookup(data, filter.path)

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
		fieldString, ok := fieldValue.(string)
		return ok && filter.pattern != nil && filter.pattern.MatchString(fieldString)
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

// compareEqual compares two values for equality.
func compareEqual(fieldValue, filterValue any) bool {
	if reflect.DeepEqual(fieldValue, filterValue) {
		return true
	}
	// Compare exactly so integers above 2^53 do not round to the same value.
	comparison, ok := compareExactNumbers(fieldValue, filterValue)
	return ok && comparison == 0
}

func compareExactNumbers(fieldValue, filterValue any) (int, bool) {
	field, filter := reflect.ValueOf(fieldValue), reflect.ValueOf(filterValue)
	switch {
	case field.CanInt() && filter.CanInt():
		return cmp.Compare(field.Int(), filter.Int()), true
	case field.Kind() >= reflect.Uint && field.Kind() <= reflect.Uint64 &&
		filter.Kind() >= reflect.Uint && filter.Kind() <= reflect.Uint64:
		return cmp.Compare(field.Uint(), filter.Uint()), true
	case field.CanFloat() && filter.CanFloat():
		left, right := field.Float(), filter.Float()
		if field.Kind() == reflect.Float32 && filter.Kind() != reflect.Float32 {
			right = matchFloat32(float32(left), right)
		} else if filter.Kind() == reflect.Float32 && field.Kind() != reflect.Float32 {
			left = matchFloat32(float32(right), left)
		}
		return cmp.Compare(left, right), !math.IsNaN(left) && !math.IsNaN(right) &&
			!math.IsInf(left, 0) && !math.IsInf(right, 0)
	}
	fieldNumber, filterNumber := exactNumber(field), exactNumber(filter)
	if fieldNumber == nil || filterNumber == nil {
		return 0, false
	}
	return fieldNumber.Cmp(filterNumber), true
}

func exactNumber(v reflect.Value) *big.Rat {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return new(big.Rat).SetInt64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return new(big.Rat).SetUint64(v.Uint())
	case reflect.Float32, reflect.Float64:
		return new(big.Rat).SetFloat64(v.Float())
	default:
		// The reader decodes uint128 values as *big.Int.
		if n := bigInt(v); n != nil {
			return new(big.Rat).SetInt(n)
		}
		return nil
	}
}

func bigInt(v reflect.Value) *big.Int {
	if v.Kind() != reflect.Pointer || v.Type() != bigIntType || v.IsNil() {
		return nil
	}
	return v.Interface().(*big.Int)
}

var bigIntType = reflect.TypeFor[*big.Int]()

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

// compareGreater compares if fieldValue > filterValue.
func compareGreater(fieldValue, filterValue any) bool {
	comparison, ok := compareNumbers(fieldValue, filterValue)
	return ok && comparison > 0
}

// compareLess compares if fieldValue < filterValue.
func compareLess(fieldValue, filterValue any) bool {
	comparison, ok := compareNumbers(fieldValue, filterValue)
	return ok && comparison < 0
}

// compareGreaterEqual compares if fieldValue >= filterValue.
func compareGreaterEqual(fieldValue, filterValue any) bool {
	comparison, ok := compareNumbers(fieldValue, filterValue)
	return ok && comparison >= 0
}

// compareLessEqual compares if fieldValue <= filterValue.
func compareLessEqual(fieldValue, filterValue any) bool {
	comparison, ok := compareNumbers(fieldValue, filterValue)
	return ok && comparison <= 0
}

func compareNumbers(fieldValue, filterValue any) (int, bool) {
	if comparison, ok := compareExactNumbers(fieldValue, filterValue); ok {
		return comparison, true
	}
	// Preserve numeric string coercion and IEEE infinity ordering.
	fieldFloat, err1 := toFloat64(fieldValue)
	filterFloat, err2 := toFloat64(filterValue)
	if err1 != nil || err2 != nil || math.IsNaN(fieldFloat) || math.IsNaN(filterFloat) {
		return 0, false
	}
	if field, ok := fieldValue.(float32); ok {
		filterFloat = matchFloat32(field, filterFloat)
	} else if filter, ok := filterValue.(float32); ok {
		fieldFloat = matchFloat32(filter, fieldFloat)
	}
	return cmp.Compare(fieldFloat, filterFloat), true
}

// matchFloat32 compares a number with an MMDB float at float32 precision.
// MMDB floats decode as float32, and JSON output prints the shortest decimal
// that parses back to the same float32. A JSON client sends that decimal back
// as a float64, which usually differs from the widened float32. matchFloat32
// returns the widened float32 when value denotes it, so the two compare as
// equal, and returns value unchanged otherwise. Nearby numbers that round to the
// same float32 therefore also match.
func matchFloat32(stored float32, value float64) float64 {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return value
	}
	rounded := float32(value)
	if math.IsInf(float64(rounded), 0) {
		// Finite values beyond the float32 range never match.
		return value
	}
	if float64(rounded) != value {
		// A decimal parsed as float64 can land exactly halfway between two
		// float32 values. Rounding then picks the even one, which can differ
		// from the float32 the decimal was printed from, as with 7.038531e-26.
		// In that case the value denotes the float32 that prints as it.
		direction := float32(math.Inf(1))
		if value < float64(rounded) {
			direction = float32(math.Inf(-1))
		}
		neighbor := math.Nextafter32(rounded, direction)
		if value == (float64(rounded)+float64(neighbor))/2 &&
			printedFloat32(rounded) != value && printedFloat32(neighbor) == value {
			rounded = neighbor
		}
	}
	if rounded == stored {
		return float64(stored)
	}
	return value
}

// printedFloat32 returns the float64 nearest the decimal that JSON output prints
// for value.
func printedFloat32(value float32) float64 {
	// ParseFloat accepts every string that FormatFloat returns.
	printed, _ := strconv.ParseFloat(strconv.FormatFloat(float64(value), 'g', -1, 32), 64)
	return printed
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
	case *big.Int:
		if v == nil {
			return 0, fmt.Errorf("cannot convert nil %T to float64", value)
		}
		f, _ := new(big.Float).SetInt(v).Float64()
		return f, nil
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
