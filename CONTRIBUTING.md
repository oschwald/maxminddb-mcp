# Contributing to MaxMindDB MCP Server

Thank you for your interest in contributing to the MaxMindDB MCP Server! This document provides guidelines and information for contributors.

## Quick Start for Contributors

1. **Fork and Clone**

   ```bash
   git clone https://github.com/your-username/maxminddb-mcp.git
   cd maxminddb-mcp
   ```

2. **Install Dependencies**

   ```bash
   go mod tidy
   ```

3. **Run Tests**

   ```bash
   go test ./...
   ```

4. **Run Linting**
   ```bash
   golangci-lint run
   ```

## Development Environment

### Prerequisites

- **Go 1.24+**: Uses modern stdlib features like `log/slog`, `net/netip`, `slices`, and `maps`
- **golangci-lint**: For code quality checks
- **MaxMind Account** (optional): For testing with real databases

### Setup

1. **Build the binary**:

   ```bash
   go build -o bin/maxminddb-mcp cmd/maxminddb-mcp/main.go
   ```

2. **Run with debug logging**:

   ```bash
   MAXMINDDB_MCP_LOG_LEVEL=debug ./bin/maxminddb-mcp
   ```

3. **Test MCP protocol**:
   ```bash
   echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | ./bin/maxminddb-mcp
   ```

## Code Standards

### Go Style Guidelines

- **Formatting**: Uses `golangci-lint` with `gofumpt` and `goimports`
- **Naming Conventions**:
  - Exported: `PascalCase` for public APIs
  - Internal: `camelCase` for private members
  - Constants: `CamelCase` for all constants
  - Packages: Short, lowercase, no underscores
- **JSON/TOML Tags**: `snake_case` (enforced by `tagliatelle` linter)

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
- Avoid `fmt.Printf` for application logs
- Configure levels: `debug`, `info`, `warn`, `error`

## Project Architecture

### Directory Structure

- **`cmd/maxminddb-mcp/`**: CLI entrypoint and main binary
- **`internal/config/`**: Configuration management and validation
- **`internal/database/`**: MaxMind database management with file watching
- **`internal/filter/`**: Filter engine with operator support
- **`internal/iterator/`**: Stateful iterator system for network ranges
- **`internal/mcp/`**: MCP protocol server implementation
- **`test/`**: Integration and performance tests
- **`testdata/`**: MMDB fixtures (avoid adding large files)

### Key Design Patterns

- **Direct Pull Model**: Iterator pulls directly from MMDB readers
- **Manager Pattern**: Database lifecycle with concurrent safety
- **Filter Engine**: Type-aware comparisons with operator validation
- **Structured Errors**: Machine-readable error responses

## Testing

### Test Types

1. **Unit Tests**: Place next to code files (`*_test.go`)
2. **Integration Tests**: In `test/` directory
3. **Coverage Target**: ≥80% for changed packages

### Running Tests

```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# Race detection
go test -race ./...

# Specific package
go test ./internal/filter

# With verbose output
go test -v ./internal/iterator
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
        {
            name: "descriptive test case name",
            // test case data
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

## Making Changes

### Feature Development

1. **Create Feature Branch**

   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make Changes**
   - Follow existing code patterns and conventions
   - Add tests for new functionality
   - Update documentation as needed

3. **Pre-commit Checklist**
   - [ ] `golangci-lint run` passes
   - [ ] `go test ./...` passes
   - [ ] `go vet ./...` passes
   - [ ] Documentation updated
   - [ ] Tests added for new functionality

4. **Commit Messages**
   - Use imperative mood ("Add feature" not "Added feature")
   - Keep subject line ≤72 characters
   - Explain rationale in commit body
   - Reference issues when applicable

### Common Development Tasks

#### Adding New Filter Operators

1. Add operator to `SupportedOperators()` in `internal/filter/filter.go`
2. Implement comparison function
3. Add case to `evaluateFilter()` switch
4. Add validation in `Validate()` function
5. Update documentation and tests

#### Database Management

- Database readers use memory-mapped files
- Never call `reader.Close()` - let GC handle cleanup
- Use absolute paths for database keys
- Handle duplicate names with warnings

#### Iterator Implementation

- Iterate directly over `NetworksWithin` (pull model)
- Use resume tokens for stateful continuation
- TTL-based cleanup for expired iterators

## Pull Request Process

### Before Submitting

1. **Fork the repository** and create your branch from `main`
2. **Run the full test suite** and ensure all tests pass
3. **Run linting** and fix any issues
4. **Update documentation** for any API changes
5. **Add or update tests** for your changes

### PR Guidelines

1. **Clear Description**
   - Explain what changes you made and why
   - Link to any relevant issues
   - Include screenshots for UI changes (if applicable)

2. **Small, Focused Changes**
   - Keep PRs focused on a single feature or fix
   - Break large changes into multiple PRs when possible

3. **Test Coverage**
   - Include tests for new functionality
   - Ensure existing tests still pass
   - Aim for good test coverage on changed code

4. **Documentation**
   - Update README.md for user-facing changes
   - Update CLAUDE.md for development changes
   - Update CHANGELOG.md following the existing format

### Review Process

1. **Automated Checks**: All GitHub Actions must pass
2. **Code Review**: Maintainers will review your code
3. **Feedback**: Address any requested changes
4. **Merge**: Once approved, maintainers will merge your PR

## Issue Reporting

### Bug Reports

Please include:

- **Environment**: Go version, OS, MCP client
- **Configuration**: Sanitized config (remove secrets)
- **Steps to Reproduce**: Clear, minimal reproduction case
- **Expected vs Actual Behavior**
- **Logs**: With `MAXMINDDB_MCP_LOG_LEVEL=debug` if possible

### Feature Requests

Please include:

- **Use Case**: What problem does this solve?
- **Proposed Solution**: How should it work?
- **Alternatives**: What other approaches have you considered?
- **Impact**: Who would benefit from this feature?

### Security Issues

For security-related issues, please email the maintainer directly rather than opening a public issue.

## Development Tips

### Debugging

```bash
# Enable debug logging
MAXMINDDB_MCP_LOG_LEVEL=debug ./bin/maxminddb-mcp

# Test with real MCP client
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | ./bin/maxminddb-mcp
```

### Common Issues

- **Database loading**: Check file permissions and MMDB format
- **Iterator behavior**: Test with small networks first
- **Filter validation**: Verify operator names and value types
- **Network parsing**: Ensure CIDR format is correct

### Performance Testing

```bash
# Run performance tests
go test ./test -run=TestPerformance -v

# Memory profiling
go test ./test -run=TestPerformance -memprofile=mem.prof
go tool pprof mem.prof
```

## License

By contributing to this project, you agree that your contributions will be licensed under the ISC License.

## Questions?

- **Discussions**: [GitHub Discussions](https://github.com/oschwald/maxminddb-mcp/discussions)
- **Issues**: [GitHub Issues](https://github.com/oschwald/maxminddb-mcp/issues)

Thank you for contributing!
