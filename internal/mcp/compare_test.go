package mcp

import (
	"context"
	"net/netip"
	"testing"

	"github.com/oschwald/maxminddb-mcp/internal/iprange"

	"github.com/oschwald/maxminddb-golang/v2"
)

func comparisonServer(t *testing.T) *Server {
	t.Helper()
	server := testServer(t)
	for _, path := range []string{"testdata/old.mmdb", "testdata/new.mmdb"} {
		if err := server.dbManager.LoadDatabase(path); err != nil {
			t.Fatal(err)
		}
	}
	return server
}

func TestComparison(t *testing.T) {
	server := comparisonServer(t)
	oldReader, _ := server.dbManager.GetReader("old.mmdb")
	newReader, _ := server.dbManager.GetReader("new.mmdb")
	ip := netip.MustParseAddr("10.0.0.1")
	if oldReader.Lookup(ip).Prefix() == newReader.Lookup(ip).Prefix() {
		t.Fatal("fixture does not exercise different boundaries")
	}
	call := modernTools(t, server)
	args := map[string]any{
		"old_database": "old.mmdb",
		"new_database": "new.mmdb",
		"network":      "10.0.0.0/16",
		"max_results":  1,
	}
	var changes []any
	for page := 0; ; page++ {
		if page > 5 {
			t.Fatal("comparison did not terminate")
		}
		payload := wireContent(t, call("compare_databases", args))
		changes = append(changes, payload["changes"].([]any)...)
		if payload["has_more"] == false {
			break
		}
		args["resume_token"] = payload["resume_token"]
	}
	if len(changes) != 3 {
		t.Fatalf("unexpected changes: %v", changes)
	}
	for i, want := range []struct{ kind, start, end string }{
		{"changed", "10.0.1.0", "10.0.1.255"}, {"removed", "10.0.3.0", "10.0.3.255"}, {"added", "10.0.4.0", "10.0.4.255"},
	} {
		change := changes[i].(map[string]any)
		if change["change"] != want.kind || change["start_ip"] != want.start ||
			change["end_ip"] != want.end {
			t.Fatalf("incorrect difference: %v", change)
		}
	}
	delete(args, "resume_token")
	args["network"] = "10.0.1.128/25"
	payload := wireContent(t, call("compare_databases", args))
	change := payload["changes"].([]any)[0].(map[string]any)
	if change["start_ip"] != "10.0.1.128" || change["before"] != "old" || change["after"] != "new" {
		t.Fatalf("incorrect clipped change: %v", change)
	}
	for _, network := range []string{"2001:db8::/120", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff/128", "10.0.0.0/24"} {
		args["network"] = network
		payload := wireContent(t, call("compare_databases", args))
		if len(payload["changes"].([]any)) != 0 || payload["has_more"] != false {
			t.Fatalf("equal answers differed: %v", payload)
		}
	}
}

func TestComparisonResumeAndCancellation(t *testing.T) {
	server := comparisonServer(t)
	args := map[string]any{
		"old_database": "old.mmdb",
		"new_database": "new.mmdb",
		"network":      "10.0.0.0/16",
		"max_results":  1,
	}
	result, err := server.handleCompareDatabases(t.Context(), networkRequest(args))
	if err != nil || result.IsError {
		t.Fatalf("comparison: %+v, %v", result, err)
	}
	args["resume_token"] = result.StructuredContent.(comparisonResult).ResumeToken
	args["network"] = "10.0.0.0/24"
	result, err = server.handleCompareDatabases(t.Context(), networkRequest(args))
	assertToolError(t, result, err, "resume_failed")
	args["network"] = "10.0.0.0/16"
	server.dbManager.RemoveDatabase("new.mmdb")
	if err := server.dbManager.LoadDatabase("testdata/new.mmdb"); err != nil {
		t.Fatal(err)
	}
	result, err = server.handleCompareDatabases(t.Context(), networkRequest(args))
	assertToolError(t, result, err, "resume_failed")
	delete(args, "resume_token")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err = server.handleCompareDatabases(ctx, networkRequest(args))
	assertToolError(t, result, err, "comparison_failed")
}

func BenchmarkComparison(b *testing.B) {
	for _, test := range []struct{ name, oldFile, newFile, network string }{
		{"small", "old", "new", "10.0.0.0/16"},
		{"scan_limit", "dense", "dense", "2001:db8:ffff::/112"},
	} {
		b.Run(test.name, func(b *testing.B) {
			oldReader, err := maxminddb.Open("testdata/" + test.oldFile + ".mmdb")
			if err != nil {
				b.Fatal(err)
			}
			defer oldReader.Close()
			newReader, err := maxminddb.Open("testdata/" + test.newFile + ".mmdb")
			if err != nil {
				b.Fatal(err)
			}
			defer newReader.Close()
			query := iprange.FromPrefix(netip.MustParsePrefix(test.network))
			b.ReportAllocs()
			for b.Loop() {
				if _, _, err := comparePage(
					b.Context(),
					oldReader,
					newReader,
					query,
					100,
					nil,
				); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestComparisonScanLimit(t *testing.T) {
	reader, err := maxminddb.Open("testdata/dense.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	query := iprange.FromPrefix(netip.MustParsePrefix("2001:db8:ffff::/112"))
	page, next, err := comparePage(t.Context(), reader, reader, query, 100, nil)
	if err != nil || page.ScannedSegments != maxAnalysisRecords || !page.HasMore ||
		len(page.Changes) != 0 {
		t.Fatalf("scan limit: %+v, %v", page, err)
	}
	page, _, err = comparePage(
		t.Context(),
		reader,
		reader,
		iprange.Range{Start: next, End: query.End},
		100,
		nil,
	)
	if err != nil || page.HasMore || len(page.Changes) != 0 {
		t.Fatalf("resume after scan limit: %+v, %v", page, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if _, _, err := comparePage(
		ctx,
		reader,
		reader,
		query,
		100,
		func(string) { cancel() },
	); err == nil {
		t.Fatal("comparison ignored cancellation during scan")
	}
}
