package mcp

import (
	"context"
	"net/netip"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oschwald/maxminddb-mcp/internal/iprange"
)

func (s *Server) registerRangeTool() {
	s.mcp.AddTool(mcp.NewTool("lookup_range", append([]mcp.ToolOption{
		mcp.WithToolTitle("Query an address range"),
		mcp.WithDescription(
			"Find stored network records overlapping an inclusive start/end IP range, with optional filters and field selection. Use lookup_network when the bounds are a CIDR. Returns each overlapping network once, including networks extending beyond the bounds. Repeat the endpoints and database on each page. Stop when has_more is false.",
		),
		mcp.WithString(
			"start_ip",
			mcp.Required(),
			mcp.Description(
				"First IPv4 or IPv6 address to include, for example 192.0.2.10. Zone identifiers are not supported.",
			),
		),
		mcp.WithString(
			"end_ip",
			mcp.Required(),
			mcp.Description(
				"Last address to include, for example 192.0.2.200. Must be at or after start_ip in the same address family, without a zone identifier.",
			),
		),
	}, networkOptions()...)...), s.handleLookupRange)
}

func (s *Server) handleLookupRange(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	start, err := netip.ParseAddr(request.GetString("start_ip", ""))
	if err != nil {
		//nolint:nilerr // MCP reports validation failures as tool results.
		return toolError("invalid_range", "Invalid start_ip: "+err.Error()), nil
	}
	end, err := netip.ParseAddr(request.GetString("end_ip", ""))
	if err != nil {
		//nolint:nilerr // MCP reports validation failures as tool results.
		return toolError("invalid_range", "Invalid end_ip: "+err.Error()), nil
	}
	bounds, err := iprange.New(start, end)
	if err != nil {
		return toolError("invalid_range", err.Error()), nil
	}
	return s.lookupNetworks(ctx, request, bounds.Cover(), bounds)
}
