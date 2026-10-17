// Package mcp provides MCP (Model Context Protocol) server implementation for MaxMind databases.
package mcp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/netip"
	"reflect"
	"slices"
	"strings"

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
	version string,
) *Server {
	mcpServer := server.NewMCPServer(
		"MaxMindDB Server",
		version,
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
func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	return server.NewStdioServer(s.mcp).Listen(ctx, input, output)
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
			"Query a CIDR range with optional filters. filters must be an array of objects with keys: field, operator, value. Example: {\"field\":\"traits.user_type\",\"operator\":\"equals\",\"value\":\"residential\"}. Supported operators: equals, not_equals, in, not_in, contains, regex, greater_than, greater_than_or_equal, less_than, less_than_or_equal, exists.",
		),
		mcp.WithString(
			"network",
			mcp.Required(),
			mcp.Description("CIDR network to scan (e.g., '192.168.1.0/24')"),
		),
		mcp.WithString("database", mcp.Description("Specific database to query (optional)")),
		mcp.WithArray(
			"filters",
			mcp.Description("Array of filter objects: {field, operator, value} (optional)"),
		),
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
		return toolError("missing_parameter", "Missing required parameter: ip"), nil
	}

	// Parse IP address
	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		//nolint:nilerr // MCP protocol expects error result, not Go error
		return toolError("invalid_ip", "Invalid IP address: "+ipStr), nil
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
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	networkStr, err := request.RequireString("network")
	if err != nil {
		//nolint:nilerr // MCP protocol expects error result, not Go error
		return toolError("missing_parameter", "Missing required parameter: network"), nil
	}

	// Parse network
	network, err := netip.ParsePrefix(networkStr)
	if err != nil {
		//nolint:nilerr // MCP protocol expects error result, not Go error
		return toolError("invalid_network", "Invalid network: "+networkStr), nil
	}

	args := request.GetArguments()
	maxResults := float64(iterator.MaxResults)
	if value, exists := args["max_results"]; exists {
		switch value := value.(type) {
		case float64:
			maxResults = value
		case int:
			maxResults = float64(value)
		default:
			return toolError(
				"invalid_max_results",
				"max_results must be an integer between 1 and 1000",
			), nil
		}
	}
	if math.IsNaN(maxResults) || maxResults < 1 || maxResults > iterator.MaxResults ||
		math.Trunc(maxResults) != maxResults {
		return toolError(
			"invalid_max_results",
			"max_results must be an integer between 1 and 1000",
		), nil
	}
	filters, err := parseFiltersFromRequest(request)
	if err != nil {
		return toolError("invalid_filter", err.Error()), nil
	}
	if err := filter.Validate(filters); err != nil {
		return toolError("invalid_filter", err.Error()), nil
	}
	filterMode := strings.ToLower(request.GetString("filter_mode", "and"))
	if filterMode != "and" && filterMode != "or" {
		return toolError("invalid_filter_mode", "filter_mode must be and or or"), nil
	}
	dbName := request.GetString("database", "")
	iterID := request.GetString("iterator_id", "")
	resumeToken := request.GetString("resume_token", "")
	var iter *iterator.ManagedIterator
	if iterID != "" {
		iter, _ = s.iterMgr.GetIterator(iterID)
		if iter == nil && resumeToken == "" {
			return toolError(
				"iterator_expired",
				"Iterator is unknown or expired; provide resume_token or start a new query",
			), nil
		}
	}
	created := false
	if iter == nil {
		if dbName == "" {
			databases := s.dbManager.ListDatabases()
			if len(databases) == 0 {
				return toolError("no_databases", "No databases available"), nil
			}
			if len(databases) > 1 {
				return toolError(
					"database_required",
					"Specify database when more than one database is available",
				), nil
			}
			dbName = databases[0].Name
		}
		reader, exists := s.dbManager.GetReader(dbName)
		if !exists {
			return toolError("db_not_found", "Database not found: "+dbName), nil
		}
		if resumeToken != "" {
			iter, err = s.iterMgr.ResumeIterator(reader, dbName, resumeToken)
			if err != nil {
				return toolError("resume_failed", err.Error()), nil
			}
		} else {
			iter, err = s.iterMgr.CreateIterator(reader, dbName, network, filters, filterMode)
			if err != nil {
				return toolError("iterator_creation_failed", err.Error()), nil
			}
		}
		created = true
	}
	_, hasFilters := args["filters"]
	_, hasMode := args["filter_mode"]
	if iter.Network != network.Masked() || (dbName != "" && iter.Database != dbName) ||
		(hasFilters && (len(filters) != 0 || len(iter.Filters) != 0) && !reflect.DeepEqual(filters, iter.Filters)) ||
		(hasMode && filterMode != iter.FilterMode) {
		if created {
			s.iterMgr.RemoveIterator(iter.ID)
		}
		return toolError(
			"query_mismatch",
			"Continuation parameters must match the original network, database, filters, and filter_mode",
		), nil
	}

	// Perform iteration
	result, err := s.iterMgr.Iterate(ctx, iter, int(maxResults))
	if err != nil {
		return toolError("iteration_failed", fmt.Sprintf("Iteration failed: %v", err)), nil
	}

	return mcp.NewToolResultStructuredOnly(result), nil
}

// handleListDatabases handles the list_databases tool.
func (s *Server) handleListDatabases(
	_ context.Context,
	_ mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	databases := s.dbManager.ListDatabases()
	// Sort databases by name for deterministic ordering
	slices.SortFunc(databases, func(a, b *database.Info) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return mcp.NewToolResultStructuredOnly(map[string]any{
		"databases": databases,
	}), nil
}

// handleUpdateDatabases handles the update_databases tool.
func (s *Server) handleUpdateDatabases(
	ctx context.Context,
	_ mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	if s.updater == nil {
		return toolError(
			"updates_not_available",
			"Database updates not available in this mode",
		), nil
	}

	results, err := s.updater.UpdateAll(ctx)
	if err != nil {
		return toolError("update_failed", fmt.Sprintf("Update failed: %v", err)), nil
	}

	result := mcp.NewToolResultStructuredOnly(map[string]any{"results": results})
	for _, update := range results {
		if update.Error != "" {
			result.IsError = true
		}
	}
	return result, nil
}

// lookupIPInSingleDatabase performs IP lookup in a specific database.
func (s *Server) lookupIPInSingleDatabase(
	ip netip.Addr,
	ipStr, dbName string,
) (*mcp.CallToolResult, error) {
	reader, exists := s.dbManager.GetReader(dbName)
	if !exists {
		return toolError("db_not_found", "Database not found: "+dbName), nil
	}

	var record map[string]any
	if err := reader.Lookup(ip).Decode(&record); err != nil {
		return toolError("lookup_failed", fmt.Sprintf("Lookup failed: %v", err)), nil
	}

	result := map[string]any{
		"ip":   ipStr,
		"data": record,
	}

	return mcp.NewToolResultStructuredOnly(result), nil
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

	return mcp.NewToolResultStructuredOnly(result), nil
}

// parseFiltersFromRequest extracts filters from MCP request arguments.
func parseFiltersFromRequest(request mcp.CallToolRequest) ([]filter.Filter, error) {
	args := request.GetArguments()
	filtersParam, exists := args["filters"]
	if !exists {
		return nil, nil
	}

	filtersArray, ok := filtersParam.([]any)
	if !ok {
		return nil, errors.New("filters must be an array of objects {field, operator, value}")
	}

	filters := make([]filter.Filter, 0, len(filtersArray))

	for i, filterItem := range filtersArray {
		filterMap, ok := filterItem.(map[string]any)
		if !ok {
			// Provide a helpful hint if a string like "a=b" was provided
			if s, sok := filterItem.(string); sok {
				hint := buildFilterHintFromString(s)
				return nil, fmt.Errorf(
					"filters[%d] must be an object {field, operator, value}; got string. %s",
					i,
					hint,
				)
			}
			return nil, fmt.Errorf("filters[%d] must be an object {field, operator, value}", i)
		}

		field, _ := filterMap["field"].(string)
		operator, _ := filterMap["operator"].(string)
		value, hasValue := filterMap["value"]

		if field == "" || operator == "" || !hasValue {
			return nil, fmt.Errorf(
				"filters[%d] missing required keys: field, operator, and value are required",
				i,
			)
		}

		// Normalize operator with case-insensitive aliases
		normalizedOp := normalizeOperator(operator)
		filters = append(filters, filter.Filter{
			Field:    field,
			Operator: normalizedOp,
			Value:    value,
		})
	}

	return filters, nil
}

// normalizeOperator normalizes operator names with case-insensitive aliases.
func normalizeOperator(op string) string {
	switch strings.ToLower(op) {
	case "eq":
		return "equals"
	case "ne":
		return "not_equals"
	case "gt":
		return "greater_than"
	case "lt":
		return "less_than"
	case "gte":
		return "greater_than_or_equal"
	case "lte":
		return "less_than_or_equal"
	default:
		return strings.ToLower(op)
	}
}

// buildFilterHintFromString suggests a structured filter when a string like
// "traits.user_type=residential" is provided.
func buildFilterHintFromString(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Support a=b and a!=b for hinting
	if idx := strings.Index(s, "!="); idx >= 0 {
		field := strings.TrimSpace(s[:idx])
		value := strings.TrimSpace(s[idx+2:])
		if field != "" && value != "" {
			return fmt.Sprintf(
				"Did you mean: {\"field\":\"%s\", \"operator\":\"not_equals\", \"value\":\"%s\"}?",
				field,
				value,
			)
		}
	} else if idx := strings.Index(s, "="); idx >= 0 {
		field := strings.TrimSpace(s[:idx])
		value := strings.TrimSpace(s[idx+1:])
		if field != "" && value != "" {
			return fmt.Sprintf(
				"Did you mean: {\"field\":\"%s\", \"operator\":\"equals\", \"value\":\"%s\"}?",
				field,
				value,
			)
		}
	}
	return "Provide filters as objects: {field, operator, value}"
}

func toolError(code, message string) *mcp.CallToolResult {
	result := mcp.NewToolResultStructuredOnly(map[string]any{
		"error": map[string]any{"code": code, "message": message},
	})
	result.IsError = true
	return result
}
