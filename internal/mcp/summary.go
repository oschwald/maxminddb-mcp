package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"net/netip"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oschwald/maxminddb-mcp/internal/filter"
	"github.com/oschwald/maxminddb-mcp/internal/iprange"
	"github.com/oschwald/maxminddb-mcp/internal/record"

	"github.com/oschwald/maxminddb-golang/v2"
)

type summaryGroup struct {
	Value     any    `json:"value"`
	Addresses string `json:"addresses"`
	Records   int    `json:"records"`
	Missing   bool   `json:"missing"`
}

type summaryResult struct {
	Database         string         `json:"database"`
	Network          string         `json:"network"`
	GroupBy          string         `json:"group_by"`
	TotalAddresses   string         `json:"total_addresses"`
	MatchedAddresses string         `json:"matched_addresses"`
	Groups           []summaryGroup `json:"groups"`
	ScannedRecords   int            `json:"scanned_records"`
	MatchedRecords   int            `json:"matched_records"`
}

func (s *Server) registerSummaryTool() {
	s.mcp.AddTool(mcp.NewTool(
		"summarize_network",
		mcp.WithToolTitle("Summarize a network"),
		mcp.WithDescription(
			"Group matching records by a scalar field and count records and covered addresses within a CIDR. Limited to 10,000 scanned records and 100 groups. IPv4 aliases are counted once.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[summaryResult](),
		mcp.WithString("database", mcp.Required()),
		mcp.WithString(
			"network",
			mcp.Required(),
			mcp.Description("IPv4 /8 or smaller, or IPv6 /32 or smaller"),
		),
		mcp.WithString(
			"group_by",
			mcp.Required(),
			mcp.MinLength(1),
			mcp.MaxLength(256),
			mcp.Description(
				"Field path, array index, or $ for a scalar record. Missing fields form a separate group.",
			),
		),
		filtersOption(),
		filterModeOption(),
	), s.handleSummarizeNetwork)
}

func (s *Server) handleSummarizeNetwork(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	prefix, err := analysisNetwork(request)
	if err != nil {
		return toolError("invalid_network", err.Error()), nil
	}
	field := request.GetString("group_by", "")
	if !validFieldPath(field) {
		return toolError(
			"invalid_group_by",
			"group_by must be a field path of 1 to 256 characters",
		), nil
	}
	filters, err := parseFiltersFromRequest(request)
	if err != nil {
		return toolError("invalid_filter", err.Error()), nil
	}
	if err := filter.Validate(filters); err != nil {
		return toolError("invalid_filter", err.Error()), nil
	}
	mode := strings.ToLower(request.GetString("filter_mode", "and"))
	if mode != "and" && mode != "or" {
		return toolError("invalid_filter_mode", "filter_mode must be and or or"), nil
	}
	name := request.GetString("database", "")
	reader, ok := s.dbManager.GetReader(name)
	if !ok {
		return toolError("db_not_found", "Database not found: "+name), nil
	}
	result, err := summarize(
		ctx,
		reader,
		prefix,
		field,
		filter.New(filters, filter.Mode(mode)),
		s.progressReporter(ctx, request),
	)
	if err != nil {
		return toolError("summary_failed", err.Error()), nil
	}
	result.Database = name
	return mcp.NewToolResultStructuredOnly(result), nil
}

func summarize(
	ctx context.Context,
	reader *maxminddb.Reader,
	prefix netip.Prefix,
	field string,
	engine *filter.Engine,
	report func(string),
) (summaryResult, error) {
	query := iprange.FromPrefix(prefix)
	result := summaryResult{
		Network:        prefix.String(),
		GroupBy:        field,
		TotalAddresses: query.Size().String(),
		Groups:         []summaryGroup{},
	}
	groups := map[string]*summaryGroup{}
	counts := map[string]*big.Int{}
	total := new(big.Int)
	path := strings.Split(field, ".")
	for entry := range reader.NetworksWithin(prefix) {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if result.ScannedRecords == maxAnalysisRecords {
			return result, fmt.Errorf("summary exceeds 10,000 records. Use a smaller network")
		}
		var data any
		if err := entry.Decode(&data); err != nil {
			return result, err
		}
		result.ScannedRecords++
		if report != nil && (result.ScannedRecords == 1 || result.ScannedRecords%1024 == 0) {
			report(fmt.Sprintf("Summarizing: %d records scanned", result.ScannedRecords))
		}
		if !engine.Matches(data) {
			continue
		}
		span, ok := iprange.ForPrefix(entry.Prefix(), query.Start.Is4())
		span, overlaps := span.Intersect(query)
		if !ok || !overlaps {
			return result, fmt.Errorf("database returned a network outside the query")
		}
		value, found := record.Lookup(data, path)
		switch value.(type) {
		case map[string]any, []any:
			return result, fmt.Errorf(
				"group_by %q selects an object or array. Select a scalar field",
				field,
			)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return result, err
		}
		if len(encoded) > 1024 {
			return result, fmt.Errorf("group value exceeds 1024 JSON bytes. Select a smaller field")
		}
		key := string(encoded)
		if !found {
			key = "missing"
		}
		group := groups[key]
		if group == nil {
			if len(groups) == 100 {
				return result, fmt.Errorf(
					"summary exceeds 100 groups. Use a smaller network or filters",
				)
			}
			group = &summaryGroup{Value: value, Missing: !found}
			groups[key], counts[key] = group, new(big.Int)
		}
		coverage := span.Size()
		counts[key].Add(counts[key], coverage)
		total.Add(total, coverage)
		group.Records++
		result.MatchedRecords++
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	for _, key := range slices.Sorted(maps.Keys(groups)) {
		group := *groups[key]
		group.Addresses = counts[key].String()
		result.Groups = append(result.Groups, group)
	}
	result.MatchedAddresses = total.String()
	return result, nil
}
