// Package record accesses fields in generic MMDB values.
package record

import "strconv"

// Lookup follows map keys and zero-based array indexes. $ selects the root value.
func Lookup(value any, path []string) (any, bool) {
	if len(path) == 1 && path[0] == "$" {
		return value, true
	}
	for _, part := range path {
		switch container := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = container[part]
			if !ok {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(container) || strconv.Itoa(index) != part {
				return nil, false
			}
			value = container[index]
		default:
			return nil, false
		}
	}
	return value, true
}
