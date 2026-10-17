package test

import (
	"net/netip"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-mcp/internal/config"
	"github.com/oschwald/maxminddb-mcp/internal/filter"
	"github.com/oschwald/maxminddb-mcp/internal/iterator"
)

func BenchmarkFilterEngine(b *testing.B) {
	// Test data representing typical MaxMindDB record
	testData := map[string]any{
		"country": map[string]any{
			"iso_code": "US",
			"names": map[string]any{
				"en": "United States",
			},
		},
		"traits": map[string]any{
			"user_type":                "residential",
			"is_anonymous_proxy":       false,
			"autonomous_system_number": 7922,
		},
		"isp": "Comcast Cable Communications",
	}

	// Various filter configurations to benchmark
	benchmarks := []struct {
		name    string
		filters []filter.Filter
		mode    string
	}{
		{
			name: "single_equals_filter",
			filters: []filter.Filter{
				{Field: "country.iso_code", Operator: "equals", Value: "US"},
			},
			mode: "and",
		},
		{
			name: "multiple_and_filters",
			filters: []filter.Filter{
				{Field: "country.iso_code", Operator: "equals", Value: "US"},
				{Field: "traits.user_type", Operator: "equals", Value: "residential"},
				{Field: "traits.is_anonymous_proxy", Operator: "equals", Value: false},
			},
			mode: "and",
		},
		{
			name: "regex_filter",
			filters: []filter.Filter{
				{Field: "isp", Operator: "regex", Value: "^Comcast.*Communications$"},
			},
			mode: "and",
		},
		{
			name: "numeric_comparison",
			filters: []filter.Filter{
				{Field: "traits.autonomous_system_number", Operator: "greater_than", Value: 7000},
			},
			mode: "and",
		},
		{
			name: "complex_mixed_filters",
			filters: []filter.Filter{
				{Field: "country.iso_code", Operator: "in", Value: []any{"US", "CA", "MX"}},
				{Field: "traits.user_type", Operator: "equals", Value: "residential"},
				{Field: "traits.autonomous_system_number", Operator: "greater_than", Value: 1000},
				{Field: "isp", Operator: "contains", Value: "Comcast"},
			},
			mode: "and",
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			engine := filter.New(bm.filters, filter.Mode(bm.mode))

			b.ResetTimer()
			for range b.N {
				engine.Matches(testData)
			}
		})
	}

	for _, tc := range []struct {
		name         string
		field, value any
	}{
		{"signed", int64(7922), int32(7000)},
		{"unsigned", uint32(7922), uint64(7000)},
		{"float", float32(42.5), float64(40.5)},
		{"mixed_json", uint32(7922), float64(7000)},
	} {
		for _, operator := range []string{"equals", "greater_than"} {
			b.Run("numeric_"+tc.name+"/"+operator, func(b *testing.B) {
				engine := filter.New(
					[]filter.Filter{{Field: "n", Operator: operator, Value: tc.value}},
					filter.ModeAnd,
				)
				record := map[string]any{"n": tc.field}
				b.ReportAllocs()
				for b.Loop() {
					engine.Matches(record)
				}
			})
		}
	}
}

func BenchmarkIteratorCreation(b *testing.B) {
	iterMgr := iterator.New(10*time.Minute, 1*time.Minute)
	defer iterMgr.StopCleanup()

	// Simulate creating many iterators

	for b.Loop() {
		network, _ := netip.ParsePrefix("192.168.0.0/24")
		filters := []filter.Filter{
			{Field: "country.iso_code", Operator: "equals", Value: "US"},
		}

		// This would normally create an iterator but we don't have a reader
		// In practice, you'd benchmark the actual iterator creation
		_ = network
		_ = filters
	}
}

func BenchmarkConfigValidation(b *testing.B) {
	cfg := &config.Config{
		Mode:                    "directory",
		AutoUpdate:              true,
		UpdateInterval:          "24h",
		IteratorTTL:             "10m",
		IteratorCleanupInterval: "1m",
		Directory: config.DirectoryConfig{
			Paths: []string{"/tmp/test"},
		},
	}

	for b.Loop() {
		_ = cfg.Validate()
	}
}

func BenchmarkFilterValidation(b *testing.B) {
	filters := []filter.Filter{
		{Field: "country.iso_code", Operator: "equals", Value: "US"},
		{
			Field:    "traits.user_type",
			Operator: "in",
			Value:    []any{"residential", "business"},
		},
		{Field: "traits.autonomous_system_number", Operator: "greater_than", Value: 1000},
		{Field: "isp", Operator: "regex", Value: "^(Comcast|Verizon).*"},
		{Field: "country.names.en", Operator: "exists", Value: true},
	}

	for b.Loop() {
		_ = filter.Validate(filters)
	}
}

// Test different network prefix parsing performance.
func BenchmarkNetworkParsing(b *testing.B) {
	networks := []string{
		"192.168.1.0/24",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"2001:db8::/32",
		"::1/128",
		"0.0.0.0/0",
	}

	for b.Loop() {
		for _, network := range networks {
			_, _ = netip.ParsePrefix(network)
		}
	}
}

// Performance test for nested field access.
func BenchmarkNestedFieldAccess(b *testing.B) {
	testData := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"level4": "deep_value",
				},
			},
		},
	}

	// Test different field access patterns
	fields := []string{
		"level1",
		"level1.level2",
		"level1.level2.level3",
		"level1.level2.level3.level4",
		"nonexistent",
		"level1.nonexistent.level3",
	}

	for _, field := range fields {
		b.Run(field, func(b *testing.B) {
			engine := filter.New(
				[]filter.Filter{{Field: field, Operator: "exists", Value: true}},
				filter.ModeAnd,
			)
			for b.Loop() {
				engine.Matches(testData)
			}
		})
	}
}

// Memory allocation benchmarks.
func BenchmarkFilterCreation(b *testing.B) {
	filters := []filter.Filter{
		{Field: "country.iso_code", Operator: "equals", Value: "US"},
		{
			Field:    "traits.user_type",
			Operator: "in",
			Value:    []any{"residential", "business"},
		},
	}

	for b.Loop() {
		filter.New(filters, "and")
	}
}

func BenchmarkRegexPage(b *testing.B) {
	filters := []filter.Filter{
		{Field: "traits.isp", Operator: "regex", Value: "^Comcast.*Communications$"},
	}
	record := map[string]any{"traits": map[string]any{"isp": "Comcast Cable Communications"}}
	b.ReportAllocs()
	for b.Loop() {
		engine := filter.New(filters, filter.ModeAnd)
		for range 1000 {
			if !engine.Matches(record) {
				b.Fatal("record did not match")
			}
		}
	}
}
