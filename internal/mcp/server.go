// Package mcp provides MCP (Model Context Protocol) server implementation for MaxMind databases.
package mcp

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/oschwald/maxminddb-mcp/internal/config"
	"github.com/oschwald/maxminddb-mcp/internal/database"
	"github.com/oschwald/maxminddb-mcp/internal/filter"
	"github.com/oschwald/maxminddb-mcp/internal/iterator"
)

// Server wraps the MCP server with our application state.
type Server struct {
	mcp       *server.MCPServer
	config    *config.Config
	dbManager *database.Manager
	updater   *database.Updater
	iterMgr   *iterator.Manager
}

// New creates a new MCP server instance.
func New(
	cfg *config.Config,
	dbManager *database.Manager,
	updater *database.Updater,
	iterMgr *iterator.Manager,
) *Server {
	mcpServer := server.NewMCPServer(
		"MaxMind MMDB Server",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	s := &Server{
		mcp:       mcpServer,
		config:    cfg,
		dbManager: dbManager,
		updater:   updater,
		iterMgr:   iterMgr,
	}

	s.registerTools()

	return s
}

// Serve starts the MCP server using stdio transport.
func (s *Server) Serve() error {
	return server.ServeStdio(s.mcp)
}

// registerTools registers all MCP tools.
func (s *Server) registerTools() {
	// lookup_ip tool
	lookupIPTool := mcp.NewTool("lookup_ip",
		mcp.WithDescription("Look up information for a specific IP address"),
		mcp.WithString("ip", mcp.Required(), mcp.Description("IP address to lookup")),
		mcp.WithString("database", mcp.Description("Specific database to query (optional)")),
	)
	s.mcp.AddTool(lookupIPTool, s.handleLookupIP)

	// lookup_network tool
	lookupNetworkTool := mcp.NewTool(
		"lookup_network",
		mcp.WithDescription(
			"Look up information for all IPs in a network range with optional filtering",
		),
		mcp.WithString(
			"network",
			mcp.Required(),
			mcp.Description("CIDR network to scan (e.g., '192.168.1.0/24')"),
		),
		mcp.WithString("database", mcp.Description("Specific database to query (optional)")),
		mcp.WithArray("filters", mcp.Description("Array of filter conditions (optional)")),
		mcp.WithString(
			"filter_mode",
			mcp.Description("How to combine filters: 'and' or 'or' (default: 'and')"),
		),
		mcp.WithNumber("max_results", mcp.Description("Maximum results to return (default: 1000)")),
		mcp.WithString("iterator_id", mcp.Description("Resume existing iterator (fast path)")),
		mcp.WithString("resume_token", mcp.Description("Fallback token if iterator expired")),
	)
	s.mcp.AddTool(lookupNetworkTool, s.handleLookupNetwork)

	// list_databases tool
	listDBTool := mcp.NewTool("list_databases",
		mcp.WithDescription("List all available MaxMind databases"),
	)
	s.mcp.AddTool(listDBTool, s.handleListDatabases)

	// update_databases tool (only for maxmind/geoip_compat modes)
	if s.config.Mode == config.ModeMaxMind || s.config.Mode == config.ModeGeoIPCompat {
		updateDBTool := mcp.NewTool("update_databases",
			mcp.WithDescription("Trigger manual update of MaxMind databases"),
		)
		s.mcp.AddTool(updateDBTool, s.handleUpdateDatabases)
	}
}

// handleLookupIP handles the lookup_ip tool.
func (s *Server) handleLookupIP(
	_ context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	ipStr, err := request.RequireString("ip")
	if err != nil {
		//nolint:nilerr // MCP protocol expects error result, not Go error
		return mcp.NewToolResultError(
			"Missing required parameter: ip",
		), nil
	}

	// Parse IP address
	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		//nolint:nilerr // MCP protocol expects error result, not Go error
		return mcp.NewToolResultError(
			"Invalid IP address: " + ipStr,
		), nil
	}

	// Get database name if specified
	dbName := request.GetString("database", "")

	// Perform lookup
	if dbName != "" {
		return s.lookupIPInSingleDatabase(ip, ipStr, dbName)
	}

	return s.lookupIPInAllDatabases(ip, ipStr)
}

// handleLookupNetwork handles the lookup_network tool.
func (s *Server) handleLookupNetwork(
	_ context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	networkStr, err := request.RequireString("network")
	if err != nil {
		//nolint:nilerr // MCP protocol expects error result, not Go error
		return mcp.NewToolResultError(
			"Missing required parameter: network",
		), nil
	}

	// Parse network
	network, err := netip.ParsePrefix(networkStr)
	if err != nil {
		//nolint:nilerr // MCP protocol expects error result, not Go error
		return mcp.NewToolResultError(
			"Invalid network: " + networkStr,
		), nil
	}

	// Get database name
	dbName := request.GetString("database", "")

	// Use first database if none specified
	if dbName == "" {
		databases := s.dbManager.ListDatabases()
		if len(databases) == 0 {
			return mcp.NewToolResultError("No databases available"), nil
		}
		dbName = databases[0].Name
	}

	// Get reader
	reader, exists := s.dbManager.GetReader(dbName)
	if !exists {
		return mcp.NewToolResultError("Database not found: " + dbName), nil
	}

	// Parse filters - for now, skip complex array parsing
	var filters []filter.Filter
	// TODO: Implement proper filter parsing once we understand the MCP parameter format

	// Validate filters
	if err := filter.Validate(filters); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid filters: %v", err)), nil
	}

	// Get filter mode
	filterMode := request.GetString("filter_mode", "and")

	// Get max results
	maxResults := int(request.GetFloat("max_results", 1000))

	// Check for existing iterator or resume token
	var iter *iterator.ManagedIterator

	if iterID := request.GetString("iterator_id", ""); iterID != "" {
		if existingIter, found := s.iterMgr.GetIterator(iterID); found {
			iter = existingIter
		}
	}

	if iter == nil {
		if resumeToken := request.GetString("resume_token", ""); resumeToken != "" {
			var err error
			iter, err = s.iterMgr.ResumeIterator(reader, resumeToken)
			if err != nil {
				return mcp.NewToolResultError(
					fmt.Sprintf("Failed to resume iterator: %v", err),
				), nil
			}
		}
	}

	// Create new iterator if none found
	if iter == nil {
		var err error
		iter, err = s.iterMgr.CreateIterator(reader, dbName, network, filters, filterMode)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create iterator: %v", err)), nil
		}
	}

	// Perform iteration
	result, err := s.iterMgr.Iterate(iter, maxResults)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Iteration failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Network lookup results: %+v", result)), nil
}

// handleListDatabases handles the list_databases tool.
func (s *Server) handleListDatabases(
	_ context.Context,
	_ mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	databases := s.dbManager.ListDatabases()
	return mcp.NewToolResultText(fmt.Sprintf("Available databases: %+v", databases)), nil
}

// handleUpdateDatabases handles the update_databases tool.
func (s *Server) handleUpdateDatabases(
	ctx context.Context,
	_ mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	if s.updater == nil {
		return mcp.NewToolResultError("Database updates not available in this mode"), nil
	}

	results, err := s.updater.UpdateAll(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Update failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Update results: %+v", results)), nil
}

// lookupIPInSingleDatabase performs IP lookup in a specific database.
func (s *Server) lookupIPInSingleDatabase(
	ip netip.Addr,
	ipStr, dbName string,
) (*mcp.CallToolResult, error) {
	reader, exists := s.dbManager.GetReader(dbName)
	if !exists {
		return mcp.NewToolResultError("Database not found: " + dbName), nil
	}

	var record map[string]any
	if err := reader.Lookup(ip).Decode(&record); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Lookup failed: %v", err)), nil
	}

	result := map[string]any{
		"ip":   ipStr,
		"data": record,
	}

	return mcp.NewToolResultText(fmt.Sprintf("IP lookup result: %+v", result)), nil
}

// lookupIPInAllDatabases performs IP lookup across all databases.
func (s *Server) lookupIPInAllDatabases(ip netip.Addr, ipStr string) (*mcp.CallToolResult, error) {
	results := make(map[string]any)
	databases := s.dbManager.ListDatabases()

	for _, dbInfo := range databases {
		reader, exists := s.dbManager.GetReader(dbInfo.Name)
		if !exists {
			continue
		}

		var record map[string]any
		if err := reader.Lookup(ip).Decode(&record); err != nil {
			continue // Skip databases that don't contain this IP
		}

		dbResult := map[string]any{
			"data": record,
		}

		results[dbInfo.Name] = dbResult
	}

	result := map[string]any{
		"ip":        ipStr,
		"databases": results,
	}

	return mcp.NewToolResultText(fmt.Sprintf("IP lookup results: %+v", result)), nil
}
