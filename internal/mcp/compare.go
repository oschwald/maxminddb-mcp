package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oschwald/maxminddb-mcp/internal/iprange"

	"github.com/oschwald/maxminddb-golang/v2"
)

const maxAnalysisRecords = 10000

type databaseChange struct {
	Before      any    `json:"before"`
	After       any    `json:"after"`
	StartIP     string `json:"start_ip"`
	EndIP       string `json:"end_ip"`
	Change      string `json:"change"`
	BeforeFound bool   `json:"before_found"`
	AfterFound  bool   `json:"after_found"`
}

type comparisonResult struct {
	ResumeToken     string           `json:"resume_token"`
	Changes         []databaseChange `json:"changes"`
	ScannedSegments int              `json:"scanned_segments"`
	HasMore         bool             `json:"has_more"`
}

type comparisonToken struct {
	OldDatabase   string `json:"old_database"`
	NewDatabase   string `json:"new_database"`
	OldGeneration string `json:"old_generation"`
	NewGeneration string `json:"new_generation"`
	Network       string `json:"network"`
	Next          string `json:"next"`
	Version       int    `json:"version"`
}

func (s *Server) registerComparisonTool() {
	s.mcp.AddTool(mcp.NewTool(
		"compare_databases",
		mcp.WithToolTitle("Compare databases"),
		mcp.WithDescription(
			"Compare lookup answers in two loaded databases within a CIDR, for example before and after an update. Returns paginated added, removed, and changed address ranges. Equal answers with different network boundaries are unchanged. Pages scan at most 10,000 segments and can contain no changes while has_more is true. Continue until has_more is false.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[comparisonResult](),
		mcp.WithString(
			"old_database",
			mcp.Required(),
			mcp.Description(
				"Baseline database's exact filename from list_databases. Supplies the before values.",
			),
		),
		mcp.WithString(
			"new_database",
			mcp.Required(),
			mcp.Description(
				"Comparison database's exact filename from list_databases. Supplies the after values. Both databases must be loaded.",
			),
		),
		mcp.WithString(
			"network",
			mcp.Required(),
			mcp.Description(
				"CIDR to compare. Use an IPv4 prefix length of 8 or greater, or a native IPv6 prefix length of 32 or greater. Repeat on each page.",
			),
		),
		mcp.WithInteger(
			"max_results",
			mcp.Min(1),
			mcp.Max(1000),
			mcp.DefaultNumber(100),
			mcp.Description("Maximum changed address ranges to return on this page."),
		),
		mcp.WithString(
			"resume_token",
			mcp.Description(
				"Token from the previous page. Omit on the first request. Repeat both database names and network. If either loaded database changed, start a new comparison.",
			),
		),
	), s.handleCompareDatabases)
}

func analysisNetwork(request mcp.CallToolRequest) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(request.GetString("network", ""))
	if err != nil {
		return netip.Prefix{}, err
	}
	if prefix.Addr().Is4In6() || (prefix.Addr().Is4() && prefix.Bits() < 8) ||
		(prefix.Addr().Is6() && prefix.Bits() < 32) {
		return netip.Prefix{}, fmt.Errorf(
			"use an IPv4 /8 or smaller network, or a native IPv6 /32 or smaller network",
		)
	}
	return prefix.Masked(), nil
}

func (s *Server) handleCompareDatabases(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	prefix, err := analysisNetwork(request)
	if err != nil {
		return toolError("invalid_network", err.Error()), nil
	}
	limit, err := boundedInteger(request, "max_results", 100, 1000)
	if err != nil {
		return toolError("invalid_max_results", err.Error()), nil
	}
	oldName, newName := request.GetString("old_database", ""), request.GetString("new_database", "")
	oldReader, oldGeneration, oldOK := s.dbManager.Snapshot(oldName)
	newReader, newGeneration, newOK := s.dbManager.Snapshot(newName)
	if !oldOK || !newOK {
		return toolError(
			"db_not_found",
			"Both old_database and new_database must name loaded databases",
		), nil
	}
	query := iprange.FromPrefix(prefix)
	cursor := comparisonToken{
		OldDatabase:   oldName,
		NewDatabase:   newName,
		OldGeneration: oldGeneration,
		NewGeneration: newGeneration,
		Network:       prefix.String(),
		Next:          query.Start.String(),
		Version:       1,
	}
	if token := request.GetString("resume_token", ""); token != "" {
		if len(token) > 4096 {
			return toolError("resume_failed", "Comparison token is too large"), nil
		}
		data, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			return toolError("resume_failed", err.Error()), nil
		}
		var previous comparisonToken
		if err := json.Unmarshal(data, &previous); err != nil {
			return toolError("resume_failed", err.Error()), nil
		}
		next, err := netip.ParseAddr(previous.Next)
		if err != nil || next.Zone() != "" || !prefix.Contains(next) {
			//nolint:nilerr // MCP reports validation failures as tool results.
			return toolError(
				"resume_failed",
				"Comparison cursor is outside the requested network",
			), nil
		}
		cursor.Next = previous.Next
		if cursor != previous {
			return toolError(
				"resume_failed",
				"Comparison query or database changed. Start a new comparison",
			), nil
		}
	}
	next, _ := netip.ParseAddr(cursor.Next)
	result, next, err := comparePage(
		ctx,
		oldReader,
		newReader,
		iprange.Range{Start: next, End: query.End},
		limit,
		s.progressReporter(ctx, request),
	)
	if err != nil {
		return toolError("comparison_failed", err.Error()), nil
	}
	if result.HasMore {
		cursor.Next = next.String()
		data, err := json.Marshal(cursor)
		if err != nil {
			return toolError("comparison_failed", err.Error()), nil
		}
		result.ResumeToken = base64.StdEncoding.EncodeToString(data)
	}
	return mcp.NewToolResultStructuredOnly(result), nil
}

func comparePage(
	ctx context.Context,
	oldReader, newReader *maxminddb.Reader,
	query iprange.Range,
	limit int,
	report func(string),
) (comparisonResult, netip.Addr, error) {
	page := comparisonResult{Changes: []databaseChange{}}
	next := query.Start
	for next.IsValid() && next.Compare(query.End) <= 0 && page.ScannedSegments < maxAnalysisRecords && len(page.Changes) < limit {
		if err := ctx.Err(); err != nil {
			return page, next, err
		}
		oldResult, newResult := oldReader.Lookup(next), newReader.Lookup(next)
		var before, after any
		if err := oldResult.Decode(&before); err != nil {
			return page, next, err
		}
		if err := newResult.Decode(&after); err != nil {
			return page, next, err
		}
		oldRange, oldOK := iprange.ForPrefix(oldResult.Prefix(), next.Is4())
		newRange, newOK := iprange.ForPrefix(newResult.Prefix(), next.Is4())
		span, overlaps := oldRange.Intersect(newRange)
		span, within := span.Intersect(iprange.Range{Start: next, End: query.End})
		if !oldOK || !newOK || !overlaps || !within || span.Start != next {
			return page, next, fmt.Errorf("database returned an inconsistent network boundary")
		}
		oldJSON, err := json.Marshal(before)
		if err != nil {
			return page, next, err
		}
		newJSON, err := json.Marshal(after)
		if err != nil {
			return page, next, err
		}
		if oldResult.Found() != newResult.Found() || !bytes.Equal(oldJSON, newJSON) {
			kind := "changed"
			if !oldResult.Found() {
				kind = "added"
			} else if !newResult.Found() {
				kind = "removed"
			}
			page.Changes = append(page.Changes, databaseChange{
				StartIP:     next.String(),
				EndIP:       span.End.String(),
				Change:      kind,
				Before:      before,
				After:       after,
				BeforeFound: oldResult.Found(),
				AfterFound:  newResult.Found(),
			})
		}
		page.ScannedSegments++
		if report != nil && (page.ScannedSegments == 1 || page.ScannedSegments%1024 == 0) {
			report(fmt.Sprintf("Compared %d address segments", page.ScannedSegments))
		}
		next = span.End.Next()
	}
	if err := ctx.Err(); err != nil {
		return page, next, err
	}
	page.HasMore = next.IsValid() && next.Compare(query.End) <= 0
	return page, next, nil
}
