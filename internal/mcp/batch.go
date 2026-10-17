package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

const maxBatchIPs = 100

func (s *Server) registerBatchTool() {
	s.mcp.AddTool(mcp.NewTool(
		"lookup_ips",
		mcp.WithToolTitle("Look up IP addresses"),
		mcp.WithDescription(
			"Look up 1 to 100 IP addresses in input order, in one database or all loaded databases. Each result includes its IP and lookup data or an error.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithRawOutputSchema([]byte(batchOutputSchema)),
		mcp.WithArray(
			"ips",
			mcp.Required(),
			mcp.MinItems(1),
			mcp.MaxItems(maxBatchIPs),
			mcp.WithStringItems(),
		),
		mcp.WithString(
			"database",
			mcp.Description("Specific database from list_databases (optional)"),
		),
	), s.handleLookupIPs)
}

func (s *Server) handleLookupIPs(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	ips, err := request.RequireStringSlice("ips")
	if err != nil || len(ips) < 1 || len(ips) > maxBatchIPs {
		//nolint:nilerr // MCP reports validation failures as tool results.
		return toolError("invalid_ips", "ips must contain 1 to 100 IP address strings"), nil
	}
	results := make([]map[string]any, 0, len(ips))
	failed := false
	for _, ip := range ips {
		if err := ctx.Err(); err != nil {
			return toolError("canceled", err.Error()), nil
		}
		result, err := s.lookupIP(ip, request.GetString("database", ""))
		if err != nil {
			return nil, err
		}
		entry, ok := result.StructuredContent.(map[string]any)
		if !ok {
			entry = map[string]any{"error": map[string]any{
				"code": "lookup_failed", "message": "Lookup result could not be encoded as JSON",
			}}
		}
		entry["ip"] = ip
		results = append(results, entry)
		failed = failed || result.IsError
	}
	result := mcp.NewToolResultStructuredOnly(map[string]any{"results": results})
	result.IsError = failed
	return result, nil
}
