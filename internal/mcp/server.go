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
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/oschwald/maxminddb-mcp/internal/config"
	"github.com/oschwald/maxminddb-mcp/internal/database"
	"github.com/oschwald/maxminddb-mcp/internal/filter"
	"github.com/oschwald/maxminddb-mcp/internal/iprange"
	"github.com/oschwald/maxminddb-mcp/internal/iterator"
)

// Server wraps the MCP server with our application state.
type Server struct {
	mcp              *server.MCPServer
	config           *config.Config
	dbManager        *database.Manager
	updater          *database.Updater
	iterMgr          *iterator.Manager
	verificationGate chan struct{}
}

const serverInstructions = `Query local GeoIP2, GeoLite2, and custom MMDB databases.
Call list_databases first and use its exact filenames when selecting a database.
Use lookup_ip for an address or lookup_ips for up to 100 addresses. Use fields to reduce returned data.
Use lookup_network for stored records overlapping a CIDR, or lookup_range for inclusive start_ip and end_ip.
Use inspect_database for metadata and sampled field paths. The sample is not a complete schema.
Use compare_databases for changed answers in a bounded CIDR and summarize_network for grouped counts.
Continue comparisons with the same database names and network plus the latest resume_token.
Use verify_database for integrity checks. Verification cannot stop once it starts.
For network queries, specify database when more than one is loaded. Field names depend on the database.
Example ASN filter: {"field":"autonomous_system_number","operator":"equals","value":13335}.
Continue lookup pages with the same network or endpoints and database, the returned iterator_id, and latest resume_token.
Omit filters and filter_mode to keep the original query. Repeat fields on each page. Stop when has_more is false.
On iterator_expired, resume with the latest token. If the database build changed, start a new query.
On iterator_limit, continue an existing iterator or wait for idle queries to expire.
If no_databases is returned, call list_databases for download status and setup instructions.
If its status is downloading, tell the user to wait and check again later. Do not start another update.
When available, install_edition downloads a new MaxMind edition and remembers it for future updates.
update_databases updates configured and API-installed MaxMind editions.
Use install_database for a user-provided HTTP(S) URL and a destination .mmdb filename.
URL imports are one-time downloads. Replacing an existing URL import requires overwrite: true.`

// New creates a new MCP server instance.
func New(
	cfg *config.Config,
	dbManager *database.Manager,
	updater *database.Updater,
	iterMgr *iterator.Manager,
	version string,
) *Server {
	mcpServer := server.NewMCPServer(
		"maxminddb",
		version,
		server.WithTitle("MaxMindDB"),
		server.WithInstructions(serverInstructions),
		server.WithMethodCacheHints(
			mcp.MethodServerDiscover,
			(5*time.Minute).Milliseconds(),
			mcp.CacheScopePrivate,
		),
		server.WithMethodCacheHints(
			mcp.MethodToolsList,
			(5*time.Minute).Milliseconds(),
			mcp.CacheScopePrivate,
		),
		server.WithToolCapabilities(false),
		server.WithStrictInputSchemaDefault(),
		server.WithInputSchemaValidation(),
		server.WithOutputSchemaValidation(),
	)

	s := &Server{
		mcp:              mcpServer,
		config:           cfg,
		dbManager:        dbManager,
		updater:          updater,
		iterMgr:          iterMgr,
		verificationGate: make(chan struct{}, 1),
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
	s.registerInstallTools()
	// lookup_ip tool
	lookupIPTool := mcp.NewTool(
		"lookup_ip",
		mcp.WithToolTitle("Look up an IP address"),
		mcp.WithDescription(
			"Look up one IP address in local MMDB databases. Returns the matching record and network, with found=false when no record matches. Use lookup_ips for a batch of addresses or lookup_network to enumerate records in a CIDR.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithRawOutputSchema([]byte(ipOutputSchema)),
		mcp.WithString(
			"ip",
			mcp.Required(),
			mcp.Description("IPv4 or IPv6 address, for example 8.8.8.8 or 2001:4860:4860::8888."),
		),
		fieldsOption(),
		mcp.WithString(
			"database",
			mcp.Description(
				"Exact filename from list_databases. Omit to query all loaded databases.",
			),
		),
	)
	s.mcp.AddTool(lookupIPTool, s.handleLookupIP)
	s.registerBatchTool()
	s.registerInspectionTool()
	s.registerComparisonTool()
	s.registerSummaryTool()
	s.registerVerificationTool()

	// lookup_network tool
	lookupNetworkTool := mcp.NewTool(
		"lookup_network", append([]mcp.ToolOption{
			mcp.WithToolTitle("Query a network"),
			mcp.WithDescription(
				"Find stored network records overlapping a CIDR, with optional filters and field selection. Returned networks may extend beyond the requested CIDR. Use lookup_range for start/end bounds or summarize_network for grouped counts. Repeat the network and database on each page. Stop when has_more is false.",
			),
			mcp.WithString(
				"network",
				mcp.Required(),
				mcp.Description(
					"IPv4 or IPv6 CIDR to scan, for example 192.168.1.0/24 or 2001:db8::/32. Broad queries may scan many records.",
				),
			),
		}, networkOptions()...)...)
	s.mcp.AddTool(lookupNetworkTool, s.handleLookupNetwork)
	s.registerRangeTool()

	// list_databases tool
	listDBTool := mcp.NewTool("list_databases",
		mcp.WithToolTitle("List databases"),
		mcp.WithDescription(
			"List loaded databases, their metadata, download status, and setup instructions. Call this first to choose exact database filenames for other tools. An empty list can mean the initial download is still running. If status is downloading, wait and call again without starting another update.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[databaseListResult](),
	)
	s.mcp.AddTool(listDBTool, s.handleListDatabases)

	// update_databases tool (only for maxmind/geoip_compat modes)
	if s.config.Mode == config.ModeMaxMind || s.config.Mode == config.ModeGeoIPCompat {
		updateDBTool := mcp.NewTool(
			"update_databases",
			mcp.WithToolTitle("Update databases"),
			mcp.WithDescription(
				"Update configured and API-installed MaxMind editions using the server's credentials. Downloads and loads changed databases, replacing existing files. Returns a result or error for each edition. Use install_edition to add an edition. URL imports from install_database are not tracked for updates.",
			),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithOutputSchema[struct {
				Results []database.UpdateResult `json:"results"`
			}](),
		)
		s.mcp.AddTool(updateDBTool, s.handleUpdateDatabases)
	}
}

// handleLookupIP handles the lookup_ip tool.
func (s *Server) handleLookupIP(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	ipStr, err := request.RequireString("ip")
	if err != nil {
		//nolint:nilerr // MCP protocol expects error result, not Go error
		return toolError("missing_parameter", "Missing required parameter: ip"), nil
	}
	if err := ctx.Err(); err != nil {
		return toolError("canceled", err.Error()), nil
	}
	fields, err := parseFields(request)
	if err != nil {
		return toolError("invalid_fields", err.Error()), nil
	}
	return s.lookupIP(ipStr, request.GetString("database", ""), fields)
}

func (s *Server) lookupIP(ipStr, dbName string, fields []string) (*mcp.CallToolResult, error) {
	// Parse IP address
	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		//nolint:nilerr // MCP protocol expects error result, not Go error
		return toolError("invalid_ip", "Invalid IP address: "+ipStr), nil
	}

	// Perform lookup
	if dbName != "" {
		return s.lookupIPInSingleDatabase(ip, ipStr, dbName, fields)
	}

	return s.lookupIPInAllDatabases(ip, ipStr, fields)
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

	return s.lookupNetworks(ctx, request, network, iprange.Range{})
}

func (s *Server) lookupNetworks(
	ctx context.Context,
	request mcp.CallToolRequest,
	network netip.Prefix,
	bounds iprange.Range,
) (*mcp.CallToolResult, error) {
	fields, err := parseFields(request)
	if err != nil {
		return toolError("invalid_fields", err.Error()), nil
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
				return toolError(
					"no_databases",
					"No databases available. Call list_databases for download status and setup instructions.",
				), nil
			}
			if len(databases) > 1 {
				return toolError(
					"database_required",
					"Specify database when more than one database is available. Call list_databases for exact filenames.",
				), nil
			}
			dbName = databases[0].Name
		}
		reader, exists := s.dbManager.GetReader(dbName)
		if !exists {
			return toolError(
				"db_not_found",
				"Database not found: "+dbName+". Call list_databases for exact filenames and setup status.",
			), nil
		}
		errorCode := "iterator_creation_failed"
		if resumeToken != "" {
			errorCode = "resume_failed"
			iter, err = s.iterMgr.ResumeIterator(reader, dbName, resumeToken)
		} else if bounds.Start.IsValid() {
			iter, err = s.iterMgr.CreateRangeIterator(reader, dbName, bounds, filters, filterMode)
		} else {
			iter, err = s.iterMgr.CreateIterator(reader, dbName, network, filters, filterMode)
		}
		if err != nil {
			if errors.Is(err, iterator.ErrIteratorLimit) {
				errorCode = "iterator_limit"
			}
			return toolError(errorCode, err.Error()), nil
		}
		created = true
	}
	_, hasFilters := args["filters"]
	_, hasMode := args["filter_mode"]
	if iter.Range != bounds || iter.Network != network.Masked() ||
		(dbName != "" && iter.Database != dbName) ||
		(hasFilters && (len(filters) != 0 || len(iter.Filters) != 0) && !reflect.DeepEqual(filters, iter.Filters)) ||
		(hasMode && filterMode != iter.FilterMode) {
		if created {
			s.iterMgr.RemoveIterator(iter.ID)
		}
		return toolError(
			"query_mismatch",
			"Continuation parameters must match the original network or range, database, filters, and filter_mode",
		), nil
	}

	// Perform iteration
	result, err := s.iterMgr.Iterate(ctx, iter, int(maxResults), s.progressReporter(ctx, request))
	if err != nil {
		if created {
			s.iterMgr.RemoveIterator(iter.ID)
		}
		return toolError("iteration_failed", fmt.Sprintf("Iteration failed: %v", err)), nil
	}

	for i := range result.Results {
		result.Results[i].Data = selectFields(result.Results[i].Data, fields)
	}
	return mcp.NewToolResultStructuredOnly(result), nil
}

// handleListDatabases handles the list_databases tool.
func (s *Server) handleListDatabases(
	_ context.Context,
	_ mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	databases := s.dbManager.ListDatabases()
	if databases == nil {
		databases = []*database.Info{}
	}
	// Sort databases by name for deterministic ordering
	slices.SortFunc(databases, func(a, b *database.Info) int {
		return cmp.Compare(a.Name, b.Name)
	})
	result := databaseListResult{
		Databases: databases,
		Status:    "ready",
		Message:   "Databases are ready for queries.",
	}
	if s.updater != nil {
		status := s.updater.Status()
		result.Update = &status
	}
	if len(databases) == 0 {
		result.Status = "empty"
		result.Message = "No MMDB files are loaded. Select a folder containing MMDB files in extension settings, or check the configured database paths."
		if result.Update != nil {
			switch result.Update.State {
			case "pending", "updating":
				result.Status = "downloading"
				result.Message = "Initial database download is in progress. Wait and call list_databases again."
			case "failed":
				result.Status = "error"
				result.Message = "Database download failed. Check your MaxMind credentials, editions, and folder permissions in extension settings or the server configuration, then call update_databases."
			default:
				result.Message = "No databases are loaded. Call update_databases to download the configured editions."
			}
		}
	} else if result.Update != nil && result.Update.State == "failed" {
		result.Message = "Existing databases are ready, but the last update failed. Check the update errors before retrying update_databases."
	}
	return mcp.NewToolResultStructuredOnly(result), nil
}

type databaseListResult struct {
	Update    *database.UpdateStatus `json:"update,omitempty"`
	Status    string                 `json:"status"`
	Message   string                 `json:"message"`
	Databases []*database.Info       `json:"databases"`
}

// handleUpdateDatabases handles the update_databases tool.
func (s *Server) handleUpdateDatabases(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	if s.updater == nil {
		return toolError(
			"updates_not_available",
			"Database updates not available in this mode",
		), nil
	}

	results, err := s.updater.UpdateAll(ctx, s.progressReporter(ctx, request))
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
	fields []string,
) (*mcp.CallToolResult, error) {
	reader, exists := s.dbManager.GetReader(dbName)
	if !exists {
		return toolError(
			"db_not_found",
			"Database not found: "+dbName+". Call list_databases for exact filenames and setup status.",
		), nil
	}

	var record any
	lookup := reader.Lookup(ip)
	if err := lookup.Decode(&record); err != nil {
		return toolError("lookup_failed", fmt.Sprintf("Lookup failed: %v", err)), nil
	}

	result := map[string]any{
		"ip": ipStr, "database": dbName,
		"data": record, "found": lookup.Found(), "network": nil,
	}

	if lookup.Found() {
		result["network"] = lookup.Prefix().String()
		result["data"] = selectFields(record, fields)
	}
	return mcp.NewToolResultStructuredOnly(result), nil
}

// lookupIPInAllDatabases performs IP lookup across all databases.
func (s *Server) lookupIPInAllDatabases(
	ip netip.Addr,
	ipStr string,
	fields []string,
) (*mcp.CallToolResult, error) {
	results := make(map[string]any)
	databases := s.dbManager.ListDatabases()

	for _, dbInfo := range databases {
		reader, exists := s.dbManager.GetReader(dbInfo.Name)
		if !exists {
			continue
		}

		var record any
		lookup := reader.Lookup(ip)
		if err := lookup.Decode(&record); err != nil {
			return toolError(
				"lookup_failed",
				fmt.Sprintf("Lookup in %s failed: %v", dbInfo.Name, err),
			), nil
		}

		dbResult := map[string]any{
			"data": record, "found": lookup.Found(), "network": nil,
		}
		if lookup.Found() {
			dbResult["network"] = lookup.Prefix().String()
			dbResult["data"] = selectFields(record, fields)
		}

		results[dbInfo.Name] = dbResult
	}

	if len(results) == 0 {
		return toolError(
			"no_databases",
			"No databases available. Call list_databases for download status and setup instructions.",
		), nil
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
