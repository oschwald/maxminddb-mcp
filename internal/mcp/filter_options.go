package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oschwald/maxminddb-mcp/internal/filter"
)

func filtersOption() mcp.ToolOption {
	return mcp.WithArray(
		"filters",
		mcp.Description(
			"Conditions on record fields. For example, [{\"field\":\"autonomous_system_number\",\"operator\":\"equals\",\"value\":13335}]. Omit for no filtering on a new query. On paginated queries, omit to retain the original filters.",
		),
		mcp.Items(map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"field", "operator", "value"},
			"properties": map[string]any{
				"field": map[string]any{
					"type":        "string",
					"minLength":   1,
					"description": "Field path from inspect_database, such as country.iso_code. Use dots for nested keys or numeric array indexes, or $ for the whole record.",
				},
				"operator": map[string]any{
					"type":        "string",
					"description": "Comparison to apply. eq, ne, gt, lt, gte, and lte are aliases for the corresponding equality and ordering operators.",
					"enum": append(
						filter.SupportedOperators(),
						"eq",
						"ne",
						"gt",
						"lt",
						"gte",
						"lte",
					),
				},
				"value": map[string]any{
					"description": "Comparison value. Use an array for in/not_in, a Go regular expression string for regex, or a substring for contains. For exists, true tests for a non-null field and false tests for a missing or null field. Ordering comparisons use numbers.",
				},
			},
		}),
	)
}

func filterModeOption() mcp.ToolOption {
	return mcp.WithString(
		"filter_mode",
		mcp.Enum(
			"and",
			"or",
		),
		mcp.Description(
			"Combine filters with and (all must match) or or (any can match). Defaults to and for a new query. Omit when paging to retain the original mode.",
		),
	)
}
