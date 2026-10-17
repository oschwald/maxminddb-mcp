package mcp

import (
	"context"
	"testing"

	"github.com/oschwald/maxminddb-mcp/internal/iterator"
)

func TestRangePagination(t *testing.T) {
	for _, resume := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "token"}[resume], func(t *testing.T) {
			server := comparisonServer(t)
			call := modernTools(t, server)
			args := map[string]any{
				"database":    "old.mmdb",
				"start_ip":    "10.0.0.7",
				"end_ip":      "10.0.3.10",
				"max_results": 1,
				"fields":      []any{"country"},
			}
			for i, want := range []string{"10.0.0.0/24", "10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"} {
				page := wireContent(t, call("lookup_range", args))
				records := page["results"].([]any)
				if len(records) != 1 || records[0].(map[string]any)["network"] != want ||
					page["total_processed"] != float64(i+1) ||
					page["total_matched"] != float64(i+1) ||
					page["has_more"] != (i < 3) {
					t.Fatalf("incorrect range page: %v", page)
				}
				data := records[0].(map[string]any)["data"].(map[string]any)
				if len(data) > 1 || (i == 0 && data["country"] != "US") {
					t.Fatalf("field selection ignored: %v", data)
				}
				args["iterator_id"], args["resume_token"] = page["iterator_id"], page["resume_token"]
				if resume {
					server.iterMgr.RemoveIterator(page["iterator_id"].(string))
				}
			}
		})
	}
}

func TestRangeBoundaries(t *testing.T) {
	server := comparisonServer(t)
	for _, test := range []struct {
		start, end string
		count      int
	}{
		{"10.0.0.7", "10.0.0.130", 1},
		{"10.0.0.255", "10.0.1.0", 2},
		{"::ffff:10.0.0.1", "10.0.0.254", 1},
		{"::a00:7", "::a00:210", 3},
		{"2001:db8::7", "2001:db8::fe", 1},
		{"ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", 1},
		{"192.0.2.0", "192.0.2.255", 0},
	} {
		args := map[string]any{"database": "old.mmdb", "start_ip": test.start, "end_ip": test.end}
		result, err := server.handleLookupRange(t.Context(), networkRequest(args))
		if err != nil || result.IsError {
			t.Fatalf("range %s-%s: %+v, %v", test.start, test.end, result, err)
		}
		page := result.StructuredContent.(*iterator.IterationResult)
		if len(page.Results) != test.count || page.HasMore ||
			page.TotalProcessed != int64(test.count) {
			t.Fatalf("incorrect range: %+v", page)
		}
		args["resume_token"] = page.ResumeToken
		result, err = server.handleLookupRange(t.Context(), networkRequest(args))
		if err != nil || result.IsError ||
			len(result.StructuredContent.(*iterator.IterationResult).Results) != 0 {
			t.Fatalf("completed range resume: %+v, %v", result, err)
		}
	}
}

func TestRangeContinuationAndFilters(t *testing.T) {
	server := comparisonServer(t)
	args := map[string]any{
		"database": "old.mmdb", "start_ip": "10.0.0.1", "end_ip": "10.0.3.254", "max_results": 1,
		"filters": []any{map[string]any{"field": "country", "operator": "exists", "value": true}},
	}
	result, err := server.handleLookupRange(t.Context(), networkRequest(args))
	if err != nil || result.IsError {
		t.Fatalf("range: %+v, %v", result, err)
	}
	page := result.StructuredContent.(*iterator.IterationResult)
	if len(page.Results) != 1 || !page.HasMore {
		t.Fatalf("filtered first page: %+v", page)
	}
	args["iterator_id"], args["resume_token"] = page.IteratorID, page.ResumeToken
	delete(args, "filters")
	args["start_ip"] = "10.0.0.2"
	result, err = server.handleLookupRange(t.Context(), networkRequest(args))
	assertToolError(t, result, err, "query_mismatch")
	result, err = server.handleLookupNetwork(
		t.Context(),
		networkRequest(map[string]any{"network": "10.0.0.0/22", "iterator_id": page.IteratorID}),
	)
	assertToolError(t, result, err, "query_mismatch")
	server.iterMgr.RemoveIterator(page.IteratorID)
	result, err = server.handleLookupRange(t.Context(), networkRequest(args))
	assertToolError(t, result, err, "query_mismatch")
	args["start_ip"] = "10.0.0.1"
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err = server.handleLookupRange(ctx, networkRequest(args))
	assertToolError(t, result, err, "iteration_failed")
	result, err = server.handleLookupRange(t.Context(), networkRequest(args))
	if err != nil || result.IsError {
		t.Fatalf("resume: %+v, %v", result, err)
	}
	page = result.StructuredContent.(*iterator.IterationResult)
	if len(page.Results) != 1 || page.Results[0].Network.String() != "10.0.3.0/24" ||
		page.HasMore ||
		page.TotalMatched != 2 ||
		page.TotalProcessed != 4 {
		t.Fatalf("filter or continuation changed: %+v", page)
	}
}

func TestInvalidRanges(t *testing.T) {
	server := comparisonServer(t)
	for _, endpoints := range [][2]string{{"", "10.0.0.1"}, {"10.0.0.1", "bad"}, {"10.0.0.2", "10.0.0.1"}, {"10.0.0.1", "::1"}, {"fe80::1%zone", "fe80::2%zone"}, {"::ffff:10.0.0.1%zone", "10.0.0.2"}} {
		result, err := server.handleLookupRange(
			t.Context(),
			networkRequest(
				map[string]any{
					"database": "old.mmdb",
					"start_ip": endpoints[0],
					"end_ip":   endpoints[1],
				},
			),
		)
		assertToolError(t, result, err, "invalid_range")
	}
}
