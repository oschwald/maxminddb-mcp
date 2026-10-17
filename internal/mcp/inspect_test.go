package mcp

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

func TestInspection(t *testing.T) {
	server := testServer(t)
	if err := server.dbManager.LoadDatabase("testdata/old.mmdb"); err != nil {
		t.Fatal(err)
	}
	call := modernTools(t, server)
	payload := wireContent(
		t,
		call("inspect_database", map[string]any{"database": "old.mmdb", "sample_size": 1}),
	)
	metadata := payload["metadata"].(map[string]any)
	if payload["sampled_records"] != float64(1) || payload["schema_complete"] != false ||
		metadata["build_epoch"] != float64(1700000000) ||
		metadata["ip_version"] != float64(6) ||
		metadata["database_type"] != "MCP test records" {
		t.Fatalf("unexpected inspection: %v", payload)
	}
	result, err := server.handleInspectDatabase(
		t.Context(),
		networkRequest(map[string]any{"database": "old.mmdb"}),
	)
	if err != nil || result.IsError {
		t.Fatalf("inspection: %+v, %v", result, err)
	}
	inspection := result.StructuredContent.(inspectionResult)
	if inspection.SampledRecords != 6 || inspection.SchemaComplete || inspection.FieldsTruncated {
		t.Fatalf("unexpected sample: %+v", inspection)
	}
	fields := map[string]observedField{}
	for _, field := range inspection.Fields {
		fields[field.Path] = field
	}
	for _, kind := range []string{"object", "array", "string", "boolean"} {
		if !slices.Contains(fields["$"].Types, kind) {
			t.Fatalf("root types: %v", fields["$"].Types)
		}
	}
	if fields["1.name"].Example != "array" || fields["country"].Example != "US" {
		t.Fatalf("missing field examples: %v", fields)
	}
	for _, value := range []any{0, 1001, 1.5, nil, true} {
		result, err := server.handleInspectDatabase(
			t.Context(),
			networkRequest(map[string]any{"database": "old.mmdb", "sample_size": value}),
		)
		assertToolError(t, result, err, "invalid_sample_size")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err = server.handleInspectDatabase(
		ctx,
		networkRequest(map[string]any{"database": "old.mmdb"}),
	)
	assertToolError(t, result, err, "canceled")
}

func TestInspectionFieldLimit(t *testing.T) {
	value := map[string]any{}
	for i := range 300 {
		value[fmt.Sprint(i)] = i
	}
	sampler := fieldSampler{fields: map[string]*observedField{}, remaining: 10000}
	sampler.visit(value, "$", 0)
	if len(sampler.fields) != 256 || !sampler.truncated {
		t.Fatalf("field limit not enforced: %+v", sampler)
	}
	sampler = fieldSampler{fields: map[string]*observedField{}, remaining: 2}
	sampler.visit(value, "$", 0)
	if len(sampler.fields) != 2 || !sampler.truncated {
		t.Fatalf("visit limit not enforced: %+v", sampler)
	}
}
