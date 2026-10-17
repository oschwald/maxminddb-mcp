package mcp

import (
	"context"
	"net/netip"
	"strings"
	"testing"

	"github.com/oschwald/maxminddb-mcp/internal/filter"

	"github.com/oschwald/maxminddb-golang/v2"
)

func TestNetworkSummary(t *testing.T) {
	server := comparisonServer(t)
	call := modernTools(t, server)
	args := map[string]any{"database": "old.mmdb", "network": "10.0.0.0/16", "group_by": "country"}
	payload := wireContent(t, call("summarize_network", args))
	if payload["scanned_records"] != float64(4) || payload["matched_records"] != float64(4) ||
		payload["total_addresses"] != "65536" ||
		payload["matched_addresses"] != "1024" {
		t.Fatalf("incorrect counts: %v", payload)
	}
	groups := payload["groups"].([]any)
	if len(groups) != 3 {
		t.Fatalf("incorrect groups: %v", groups)
	}
	for i, want := range []struct {
		value     any
		records   float64
		addresses string
		missing   bool
	}{
		{"CA", 1, "256", false}, {"US", 1, "256", false}, {nil, 2, "512", true},
	} {
		group := groups[i].(map[string]any)
		if group["value"] != want.value || group["records"] != want.records ||
			group["addresses"] != want.addresses ||
			group["missing"] != want.missing {
			t.Fatalf("incorrect group: %v", group)
		}
	}
	for _, test := range []struct {
		network, coverage, total string
		records                  float64
	}{
		{"10.0.0.128/25", "128", "128", 1},
		{"2001:db8::/121", "128", "128", 1},
		{"::/80", "1024", "281474976710656", 4},
		{"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff/128", "1", "1", 1},
		{"192.0.2.0/24", "0", "256", 0},
	} {
		args["network"] = test.network
		payload = wireContent(t, call("summarize_network", args))
		if payload["matched_addresses"] != test.coverage ||
			payload["total_addresses"] != test.total ||
			payload["matched_records"] != test.records {
			t.Fatalf("%s: %v", test.network, payload)
		}
	}
	args["network"], args["database"] = "10.0.0.0/24", "new.mmdb"
	payload = wireContent(t, call("summarize_network", args))
	if payload["matched_records"] != float64(2) || payload["matched_addresses"] != "256" ||
		len(payload["groups"].([]any)) != 1 {
		t.Fatalf("split records changed coverage: %v", payload)
	}
}

func TestSummaryFiltersAndErrors(t *testing.T) {
	server := comparisonServer(t)
	args := map[string]any{
		"database": "old.mmdb", "network": "10.0.0.0/16", "group_by": "country",
		"filters": []any{map[string]any{"field": "country", "operator": "equals", "value": "US"}},
	}
	result, err := server.handleSummarizeNetwork(t.Context(), networkRequest(args))
	if err != nil || result.IsError {
		t.Fatalf("summary failed: %+v, %v", result, err)
	}
	summary := result.StructuredContent.(summaryResult)
	if summary.MatchedRecords != 1 || summary.ScannedRecords != 4 ||
		summary.MatchedAddresses != "256" {
		t.Fatalf("filter ignored: %+v", summary)
	}
	delete(args, "filters")
	args["network"], args["group_by"] = "10.0.1.0/24", "$"
	result, err = server.handleSummarizeNetwork(t.Context(), networkRequest(args))
	if err != nil || result.IsError ||
		result.StructuredContent.(summaryResult).Groups[0].Value != "old" {
		t.Fatalf("scalar grouping failed: %+v, %v", result, err)
	}
	args["network"] = "10.0.0.0/24"
	result, err = server.handleSummarizeNetwork(t.Context(), networkRequest(args))
	assertToolError(t, result, err, "summary_failed")
	for _, test := range []struct {
		name  string
		value any
		code  string
	}{
		{"group_by", "a..b", "invalid_group_by"},
		{"database", "missing", "db_not_found"},
		{"network", "0.0.0.0/0", "invalid_network"},
		{"filters", "bad", "invalid_filter"},
		{"filter_mode", "invalid", "invalid_filter_mode"},
	} {
		original := args[test.name]
		args[test.name] = test.value
		result, err = server.handleSummarizeNetwork(t.Context(), networkRequest(args))
		assertToolError(t, result, err, test.code)
		if original == nil {
			delete(args, test.name)
		} else {
			args[test.name] = original
		}
	}
}

func TestSummaryLimitsAndCancellation(t *testing.T) {
	for _, test := range []struct{ file, network, message string }{
		{"dense", "2001:db8:ffff::/112", "10,000 records"},
		{"groups", "2001:db8:ffff::/112", "100 groups"},
		{"groups", "2001:db8:eeee::/128", "1024 JSON bytes"},
	} {
		reader, err := maxminddb.Open("testdata/" + test.file + ".mmdb")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = reader.Close() })
		prefix := netip.MustParsePrefix(test.network)
		_, err = summarize(t.Context(), reader, prefix, "$", filter.New(nil, filter.ModeAnd), nil)
		if err == nil || !strings.Contains(err.Error(), test.message) {
			t.Fatalf("limit not enforced: %v", err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		_, err = summarize(
			ctx,
			reader,
			prefix,
			"$",
			filter.New(nil, filter.ModeAnd),
			func(string) { cancel() },
		)
		cancel()
		if err == nil {
			t.Fatal("ignored cancellation")
		}
	}
}

func BenchmarkSummary(b *testing.B) {
	reader, err := maxminddb.Open("testdata/old.mmdb")
	if err != nil {
		b.Fatal(err)
	}
	defer reader.Close()
	prefix := netip.MustParsePrefix("10.0.0.0/16")
	engine := filter.New(nil, filter.ModeAnd)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := summarize(b.Context(), reader, prefix, "country", engine, nil); err != nil {
			b.Fatal(err)
		}
	}
}
