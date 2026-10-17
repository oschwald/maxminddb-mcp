package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oschwald/maxminddb-mcp/internal/filter"
)

func filtersOption() mcp.ToolOption {
	return mcp.WithArray(
		"filters",
		mcp.Description(
			"Filters for a new query; omit to keep filters when continuing a query",
		),
		mcp.Items(map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"field", "operator", "value"},
			"properties": map[string]any{
				"field": map[string]any{"type": "string", "minLength": 1},
				"operator": map[string]any{
					"type": "string",
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
				"value": map[string]any{},
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
		mcp.Description("How to combine filters: 'and' or 'or' (default: 'and')"),
	)
}
