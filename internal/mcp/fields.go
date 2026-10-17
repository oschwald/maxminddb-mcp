package mcp

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oschwald/maxminddb-mcp/internal/record"
)

func fieldsOption() mcp.ToolOption {
	return mcp.WithArray(
		"fields",
		mcp.MaxItems(64),
		mcp.WithStringItems(mcp.MinLength(1), mcp.MaxLength(256)),
		mcp.Description(
			"Optional field paths. Returns an object keyed by each selected path. Use dots for map keys, numeric array indexes, or $ for the whole record. Omit or use [] for full records. Repeat on each page.",
		),
	)
}

func parseFields(request mcp.CallToolRequest) ([]string, error) {
	if _, ok := request.GetArguments()["fields"]; !ok {
		return nil, nil
	}
	fields, err := request.RequireStringSlice("fields")
	if err != nil {
		return nil, err
	}
	if len(fields) > 64 {
		return nil, fmt.Errorf("fields must contain at most 64 paths")
	}
	for _, field := range fields {
		if !validFieldPath(field) {
			return nil, fmt.Errorf("invalid field path: %q", field)
		}
	}
	return fields, nil
}

func selectFields(value any, fields []string) any {
	if len(fields) == 0 {
		return value
	}
	selected := make(map[string]any, len(fields))
	for _, field := range fields {
		if data, ok := record.Lookup(value, strings.Split(field, ".")); ok {
			selected[field] = data
		}
	}
	return selected
}

func validFieldPath(field string) bool {
	return field != "" && len(field) <= 256 && !strings.HasPrefix(field, ".") &&
		!strings.HasSuffix(field, ".") &&
		!strings.Contains(field, "..")
}
