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
			"Look up a batch of IP addresses in local MMDB databases. Each result contains the matching record and network, found=false for no match, or an error for that address. Other addresses are still processed after an error. Use lookup_ip for a single address.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithRawOutputSchema([]byte(batchOutputSchema)),
		fieldsOption(),
		mcp.WithArray(
			"ips",
			mcp.Required(),
			mcp.MinItems(1),
			mcp.MaxItems(maxBatchIPs),
			mcp.WithStringItems(),
			mcp.Description(
				"IPv4 or IPv6 addresses to look up. Results preserve input order, including duplicate addresses.",
			),
		),
		mcp.WithString(
			"database",
			mcp.Description(
				"Exact filename from list_databases. Omit to query all loaded databases.",
			),
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
	fields, err := parseFields(request)
	if err != nil {
		return toolError("invalid_fields", err.Error()), nil
	}
	results := make([]map[string]any, 0, len(ips))
	failed := false
	for _, ip := range ips {
		if err := ctx.Err(); err != nil {
			return toolError("canceled", err.Error()), nil
		}
		result, err := s.lookupIP(ip, request.GetString("database", ""), fields)
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
