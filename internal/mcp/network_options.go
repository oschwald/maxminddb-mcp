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
				"Exact filename from list_databases. Required when more than one database is loaded. Repeat the same name on each page.",
			),
		),
		filtersOption(),
		filterModeOption(),
		mcp.WithInteger(
			"max_results",
			mcp.Min(1),
			mcp.Max(iterator.MaxResults),
			mcp.DefaultNumber(iterator.MaxResults),
			mcp.Description(
				"Maximum matching records to return on this page. Use a smaller page to reduce response size.",
			),
		),
		fieldsOption(),
		mcp.WithString(
			"iterator_id",
			mcp.Description(
				"ID from the previous page. Omit on the first request. Send with the latest resume_token to continue if the iterator expires.",
			),
		),
		mcp.WithString(
			"resume_token",
			mcp.Description(
				"Token from the previous page, used if iterator_id is missing or expired. Omit on the first request. If the database build changed, start a new query.",
			),
		),
	}
}
