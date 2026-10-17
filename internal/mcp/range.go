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
			"Query stored network records overlapping an inclusive start/end IP range, with filters, field selection, and pagination. Returns each overlapping network once, not each IP address. Repeat the endpoints on every page.",
		),
		mcp.WithString("start_ip", mcp.Required()),
		mcp.WithString("end_ip", mcp.Required()),
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
