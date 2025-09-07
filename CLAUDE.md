# Claude Code Development Guide

This document provides guidelines for developing the MaxMind MMDB MCP Server using Claude Code.

## Quick Start Commands

### Build and Test

```bash
# Build the binary
go build -o bin/maxminddb-mcp cmd/maxminddb-mcp/main.go

# Run all tests
go test ./...

# Run tests with race detection and coverage
go test -race -cover ./...

# Run linting and formatting
golangci-lint run

# Run specific test
go test ./internal/iterator -run TestManager
```

### Running the Server

```bash
# Run locally
./bin/maxminddb-mcp

# With debug logging
MAXMINDDB_MCP_LOG_LEVEL=debug ./bin/maxminddb-mcp

# With custom config
MAXMINDDB_MCP_CONFIG=/path/to/config.toml ./bin/maxminddb-mcp
```

## Project Architecture

### Directory Structure

- **`cmd/maxminddb-mcp/`**: CLI entrypoint and main binary build target
- **`internal/config/`**: Configuration management and validation
- **`internal/database/`**: MaxMind database management with file watching
- **`internal/filter/`**: Filter engine with operator support and validation
- **`internal/iterator/`**: Stateful iterator system for network range processing
- **`internal/mcp/`**: MCP protocol server implementation and tool handlers
- **`test/`**: Integration and performance tests
- **`testdata/`**: MMDB fixtures and test data (avoid adding large files)

### Key Design Patterns

- **Iterator Pattern**: Streaming network processing with channels and goroutines
- **Manager Pattern**: Database lifecycle management with concurrent safety
- **Filter Engine**: Type-aware comparisons with operator normalization
- **Structured Errors**: Machine-readable error responses for MCP protocol

## Development Workflow

### Code Quality

- **Go Version**: 1.24+ with modern stdlib features
- **Formatting**: Automatic via `golangci-lint` (uses `gofumpt`/`goimports`)
- **Linting**: Comprehensive rules including security, performance, and style
- **Pre-commit**: Automatic checks for Go code, tests, and markdown formatting

### Testing Strategy

- **Unit Tests**: Place next to code files (`*_test.go`)
- **Integration Tests**: In `test/` directory
- **Coverage Target**: ≥80% for changed packages
- **Test Data**: Use existing fixtures in `testdata/`, no new large files

### Performance Considerations

- **Memory Management**: GC-based cleanup for database readers (no explicit Close)
- **Concurrency**: Thread-safe with proper mutex usage and channel patterns
- **Iterator Buffers**: Configurable channel sizes (default: 100)
- **O(1) Lookups**: Index maps for database name resolution

## Coding Standards

### Naming Conventions

- **Exported**: `PascalCase` for public APIs
- **Internal**: `camelCase` for private members
- **Constants**: `CamelCase` for all constants
- **Packages**: Short, lowercase, no underscores
- **JSON/TOML**: `snake_case` tags (enforced by `tagliatelle`)

### Specific Terminology

- "MaxMind" (not "Maxmind" or "maxmind")
- "GeoIP" (not "geoip" or "GeoIp")
- "MMDB" for MaxMind database files

### Error Handling

- Use `errors.Is/As` for error checking
- Avoid deprecated `os.IsNotExist/IsExist`
- Wrap errors with `%w` for error chains
- Return structured errors for MCP protocol

### Logging

- Use `log/slog` with structured logging
- Configure via environment variables:
  - `MAXMINDDB_MCP_LOG_LEVEL`: `debug|info|warn|error`
  - `MAXMINDDB_MCP_LOG_FORMAT`: `text|json`
- Avoid `fmt.Printf` for application logs

## Configuration Management

### Config File Locations (checked in order)

1. `MAXMINDDB_MCP_CONFIG` environment variable
2. `~/.config/maxminddb-mcp/config.toml`
3. `/etc/GeoIP.conf` or `~/.config/maxminddb-mcp/GeoIP.conf`

### Common Configuration Tasks

```toml
# MaxMind account mode
mode = "maxmind"
auto_update = true
update_interval = "24h"

[maxmind]
account_id = 123456
license_key = "your_key_here"
editions = ["GeoLite2-City", "GeoLite2-Country"]

# Performance tuning
iterator_buffer = 200
iterator_ttl = "15m"
```

## MCP Protocol Implementation

### Available Tools

- **`lookup_ip`**: Single IP address lookup
- **`lookup_network`**: Network range scanning with filtering
- **`list_databases`**: Available database metadata
- **`update_databases`**: Manual database updates (MaxMind modes only)

### Error Codes

- `db_not_found`: Specified database does not exist
- `invalid_ip`: IP address format is invalid
- `invalid_network`: Network CIDR format is invalid
- `invalid_filter`: Filter validation failed
- `iterator_not_found`: Iterator ID not found or expired

## Testing Guidelines

### Running Tests

```bash
# All tests
go test ./...

# Specific package
go test ./internal/filter

# With coverage
go test -cover ./internal/iterator

# Race detection
go test -race ./...
```

### Test Structure

```go
func TestFeatureName(t *testing.T) {
    tests := []struct {
        name     string
        input    InputType
        expected OutputType
        wantErr  bool
    }{
        // test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

## Security Guidelines

### Credential Management

- Never commit secrets or API keys
- Use configuration files or environment variables
- Store configs in `~/.config/maxminddb-mcp/`
- Validate file permissions on sensitive files

### Input Validation

- All MCP tool parameters are validated
- IP addresses parsed with `net/netip`
- Network CIDRs validated before processing
- Filter operators checked against allowlist

## Common Development Tasks

### Adding New Filter Operators

1. Add operator to `SupportedOperators()` in `internal/filter/filter.go`
2. Implement comparison function
3. Add case to `evaluateFilter()` switch
4. Add validation in `Validate()` function
5. Update documentation and tests

### Database Management

1. Database readers use memory-mapped files
2. Never call `reader.Close()` - let GC handle cleanup
3. Use absolute paths for database keys
4. Handle duplicate names with warnings

### Iterator Implementation

1. Use channels for streaming results
2. Implement proper cancellation with context
3. Resume tokens for stateful continuation
4. TTL-based cleanup for expired iterators

## Debugging Tips

### Enable Debug Logging

```bash
MAXMINDDB_MCP_LOG_LEVEL=debug ./bin/maxminddb-mcp
```

### Common Issues

- **Database loading**: Check file permissions and MMDB format
- **Iterator expired**: Use resume tokens for long-running queries
- **Filter validation**: Verify operator names and value types
- **Network parsing**: Ensure CIDR format is correct

### Testing MCP Protocol

```bash
# Test initialize
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | ./bin/maxminddb-mcp

# Test tool listing
echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | ./bin/maxminddb-mcp
```

## Contributing

### Commit Messages

- Use imperative mood ("Add feature" not "Added feature")
- Keep subject line ≤72 characters
- Explain rationale in commit body
- Reference issues when applicable

### Pull Requests

- Clear description with context
- Include reproduction steps for bugs
- Update documentation for API changes
- Ensure all CI checks pass

### Pre-commit Checklist

- [ ] `golangci-lint run` passes
- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] Documentation updated
- [ ] Tests added for new functionality
