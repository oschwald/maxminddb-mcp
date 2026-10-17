package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oschwald/maxminddb-mcp/internal/iterator"
)

func networkOptions() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithRawOutputSchema([]byte(networkOutputSchema)),
		mcp.WithString(
			"database",
			mcp.Description(
				"Database name from list_databases; required when more than one is loaded",
			),
		),
		filtersOption(),
		filterModeOption(),
		mcp.WithInteger(
			"max_results",
			mcp.Min(1),
			mcp.Max(iterator.MaxResults),
			mcp.DefaultNumber(iterator.MaxResults),
			mcp.Description("Page size from 1 to 1000"),
		),
		fieldsOption(),
		mcp.WithString("iterator_id", mcp.Description("Resume existing iterator (fast path)")),
		mcp.WithString("resume_token", mcp.Description("Fallback token if iterator expired")),
	}
}
