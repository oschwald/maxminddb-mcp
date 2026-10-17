package iterator

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-mcp/internal/filter"

	"github.com/oschwald/maxminddb-golang/v2"
)

// A filter with no matches must finish without waiting for another result.
func TestNetworkIterationFilters(t *testing.T) {
	reader, err := maxminddb.Open("../../testdata/test-data/GeoIP2-Enterprise-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	for _, tc := range []struct {
		name    string
		filters []filter.Filter
		want    int
	}{
		{name: "unfiltered", want: 1},
		{
			name: "matching",
			filters: []filter.Filter{{
				Field: "traits.user_type", Operator: "eq", Value: "residential",
			}},
			want: 1,
		},
		{
			name: "no matches",
			filters: []filter.Filter{{
				Field: "traits.user_type", Operator: "equals", Value: "hosting",
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := New(time.Minute, time.Second)
			iter, err := manager.CreateIterator(
				reader,
				"enterprise",
				netip.MustParsePrefix("175.16.199.0/24"),
				tc.filters,
				"and",
			)
			if err != nil {
				t.Fatal(err)
			}
			result, err := manager.Iterate(context.Background(), iter, 10, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Results) != tc.want || result.TotalMatched != int64(tc.want) ||
				result.TotalProcessed != 1 ||
				result.HasMore {
				t.Fatalf("unexpected page: %+v", result)
			}
			for _, record := range result.Results {
				if record.Network != netip.MustParsePrefix("175.16.199.0/24") {
					t.Errorf("unexpected network: %s", record.Network)
				}
				if !filter.New([]filter.Filter{{Field: "traits.user_type", Operator: "equals", Value: "residential"}}, filter.ModeAnd).
					Matches(record.Data) {
					t.Errorf("unexpected residential record: %v", record.Data)
				}
			}
		})
	}
}
