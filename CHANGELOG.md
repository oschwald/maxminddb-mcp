# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2025-01-XX

### Added

- **MCP Protocol Support**: Complete Model Context Protocol server implementation with stdio transport
- **MaxMind Database Integration**: Support for GeoIP2, GeoLite2, and custom MMDB files
- **Multiple Configuration Modes**:
  - MaxMind account mode with automatic downloads
  - Directory scanning mode for existing MMDB files
  - GeoIP.conf compatibility mode for legacy setups
- **Advanced Query Tools**:
  - `lookup_ip`: IP address geolocation and network information lookup
  - `lookup_network`: Network range queries with advanced filtering
  - `list_databases`: Available database metadata and statistics
  - `update_databases`: Manual database update triggers
- **Powerful Filter Engine**: 11 operators with type-aware comparisons
  - Basic: `equals`, `not_equals`, `in`, `not_in`, `exists`
  - String: `contains`, `regex`
  - Numeric: `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`
  - Operator aliases: `eq`, `ne`, `gt`, `gte`, `lt`, `lte` (case-insensitive)
  - Nested field access with dot notation (e.g., `country.iso_code`)
- **Stateful Iterator System**: Memory-efficient processing of large network ranges
  - Resumable iterations with `iterator_id` and `resume_token`
  - Configurable buffer sizes and TTL cleanup
  - Automatic state management and resource cleanup
- **Auto-updating**: Scheduled database downloads with MD5 checksum validation
- **File System Watching**: Dynamic loading of new/modified MMDB files in directory mode
- **Comprehensive Configuration**:
  - TOML configuration with full validation
  - Environment variable support (`MAXMINDDB_MCP_CONFIG`)
  - Path expansion and home directory resolution
  - Iterator performance tuning options
- **Structured Error Handling**: Machine-readable error codes with descriptive messages
- **Production-Ready Logging**: Structured logging with configurable levels and formats
- **Concurrent Safety**: Thread-safe database access and iterator management
- **Resource Management**: Graceful shutdown, cleanup on exit, and memory-efficient operations

### Technical Features

- **Modern Go Implementation**: Built with Go 1.24+ features
  - `log/slog` for structured logging
  - `net/netip` for IP address handling
  - `filepath.WalkDir` for efficient directory traversal
  - `slices` and `maps` packages for modern collection operations
- **Comprehensive Test Suite**: Unit and integration tests with >54% coverage
- **Code Quality**: golangci-lint compliance with comprehensive linter configuration
- **Documentation**: Complete README with client integration guides and troubleshooting

### Configuration

- **Iterator Buffer Validation**: Values ≤ 0 automatically clamped to default (100)
- **Duplicate Database Detection**: Warnings for name collisions across directories
- **Flexible Database Storage**: Configurable paths with automatic directory creation
- **Network Timeout Handling**: Robust error handling for network operations

### Security

- **No Hardcoded Credentials**: Configuration-based authentication only
- **Input Validation**: Comprehensive parameter validation for all MCP tools
- **Safe Resource Handling**: Proper cleanup and resource lifecycle management
- **File Permission Awareness**: Handles permission errors gracefully

[0.1.0]: https://github.com/oschwald/maxminddb-mcp/releases/tag/v0.1.0
