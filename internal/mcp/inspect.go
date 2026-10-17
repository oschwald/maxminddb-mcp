package mcp

import (
	"context"
	"fmt"
	"maps"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

type databaseMetadata struct {
	Description              map[string]string `json:"description"`
	DatabaseType             string            `json:"database_type"`
	Languages                []string          `json:"languages"`
	BuildEpoch               uint              `json:"build_epoch"`
	IPVersion                uint              `json:"ip_version"`
	RecordSize               uint              `json:"record_size"`
	NodeCount                uint              `json:"node_count"`
	BinaryFormatMajorVersion uint              `json:"binary_format_major_version"`
	BinaryFormatMinorVersion uint              `json:"binary_format_minor_version"`
}

type observedField struct {
	Example any      `json:"example,omitempty"`
	Path    string   `json:"path"`
	Types   []string `json:"types"`
}

type inspectionResult struct {
	Database        string           `json:"database"`
	Fields          []observedField  `json:"fields"`
	Metadata        databaseMetadata `json:"metadata"`
	SampledRecords  int              `json:"sampled_records"`
	SampleLimit     int              `json:"sample_limit"`
	SchemaComplete  bool             `json:"schema_complete"`
	FieldsTruncated bool             `json:"fields_truncated"`
}

func (s *Server) registerInspectionTool() {
	s.mcp.AddTool(mcp.NewTool(
		"inspect_database",
		mcp.WithToolTitle("Inspect a database"),
		mcp.WithDescription(
			"Read full MMDB metadata and observe field paths, types, and examples from a bounded sample. The observed fields are not a complete schema.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[inspectionResult](),
		mcp.WithString("database", mcp.Required()),
		mcp.WithInteger("sample_size", mcp.Min(1), mcp.Max(1000), mcp.DefaultNumber(100)),
	), s.handleInspectDatabase)
}

func (s *Server) handleInspectDatabase(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	name := request.GetString("database", "")
	reader, ok := s.dbManager.GetReader(name)
	if !ok {
		return toolError("db_not_found", "Database not found: "+name), nil
	}
	limit, err := boundedInteger(request, "sample_size", 100, 1000)
	if err != nil {
		return toolError("invalid_sample_size", err.Error()), nil
	}
	metadata := reader.Metadata
	description := map[string]string{}
	maps.Copy(description, metadata.Description)
	result := inspectionResult{
		Database:    name,
		SampleLimit: limit,
		Fields:      []observedField{},
		Metadata: databaseMetadata{
			Description:              description,
			Languages:                append([]string{}, metadata.Languages...),
			DatabaseType:             metadata.DatabaseType,
			BuildEpoch:               metadata.BuildEpoch,
			IPVersion:                metadata.IPVersion,
			RecordSize:               metadata.RecordSize,
			NodeCount:                metadata.NodeCount,
			BinaryFormatMajorVersion: metadata.BinaryFormatMajorVersion,
			BinaryFormatMinorVersion: metadata.BinaryFormatMinorVersion,
		},
	}
	sampler := fieldSampler{fields: map[string]*observedField{}, remaining: 10000}
	for entry := range reader.Networks() {
		if err := ctx.Err(); err != nil {
			return toolError("canceled", err.Error()), nil
		}
		var value any
		if err := entry.Decode(&value); err != nil {
			return toolError("inspection_failed", err.Error()), nil
		}
		result.SampledRecords++
		sampler.visit(value, "$", 0)
		if result.SampledRecords == limit || sampler.remaining <= 0 {
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return toolError("canceled", err.Error()), nil
	}
	for _, path := range slices.Sorted(maps.Keys(sampler.fields)) {
		field := *sampler.fields[path]
		slices.Sort(field.Types)
		result.Fields = append(result.Fields, field)
	}
	result.FieldsTruncated = sampler.truncated
	return mcp.NewToolResultStructuredOnly(result), nil
}

type fieldSampler struct {
	fields    map[string]*observedField
	remaining int
	truncated bool
}

func (s *fieldSampler) visit(value any, path string, depth int) {
	if s.remaining <= 0 || depth > 16 || len(path) > 256 {
		s.truncated = true
		return
	}
	s.remaining--
	kind := "integer"
	var example any
	switch v := value.(type) {
	case map[string]any:
		kind = "object"
	case []any:
		kind = "array"
	case []byte:
		kind = "bytes"
	case string:
		kind = "string"
		runes := []rune(v)
		if len(runes) > 128 {
			example = string(runes[:128]) + "…"
		} else {
			example = v
		}
	case bool:
		kind, example = "boolean", v
	case float32, float64:
		kind, example = "number", v
	case *big.Int:
		example = v.String()
	case nil:
		kind = "null"
	default:
		example = v
	}
	field := s.fields[path]
	if field == nil {
		if len(s.fields) >= 256 {
			s.truncated = true
			return
		}
		field = &observedField{Path: path, Example: example}
		s.fields[path] = field
	}
	if !slices.Contains(field.Types, kind) {
		field.Types = append(field.Types, kind)
	}
	childPath := func(key string) string {
		if path == "$" {
			return key
		}
		return path + "." + key
	}
	switch v := value.(type) {
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(v)) {
			if key == "" || strings.Contains(key, ".") || (path == "$" && key == "$") {
				s.truncated = true
				continue
			}
			s.visit(v[key], childPath(key), depth+1)
			if s.remaining <= 0 {
				s.truncated = true
				break
			}
		}
	case []any:
		for i, child := range v {
			s.visit(child, childPath(strconv.Itoa(i)), depth+1)
			if s.remaining <= 0 {
				s.truncated = true
				break
			}
		}
	}
}

func boundedInteger(
	request mcp.CallToolRequest,
	name string,
	defaultValue, maximum int,
) (int, error) {
	value, exists := request.GetArguments()[name]
	if !exists {
		return defaultValue, nil
	}
	var number float64
	switch v := value.(type) {
	case int:
		number = float64(v)
	case float64:
		number = v
	default:
		return 0, fmt.Errorf("%s must be an integer from 1 to %d", name, maximum)
	}
	if !(number >= 1 && number <= float64(maximum)) || number != float64(int(number)) {
		return 0, fmt.Errorf("%s must be an integer from 1 to %d", name, maximum)
	}
	return int(number), nil
}
