# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Clarify MCP tool selection, parameter meanings, pagination, and analysis limits in tool descriptions and input schemas.
- Shorten the README and move detailed client setup, configuration, and tool reference into linked guides. Keep existing client links working and add setup troubleshooting.

## [0.3.0] - 2026-10-17

### Added

- Add repository manifests for Claude, Cursor, Antigravity, and legacy Gemini CLI, plus Glama ownership metadata.
- Package native Gemini CLI extensions with releases and include their SHA-256 checksums.
- Publish one universal MCPB containing all five supported native builds, with automatic platform selection and the existing settings UI.

### Changed

- Use `maxminddb` as the server and plugin identifier, with `MaxMindDB` as the display name. The MCP Registry name becomes `io.github.oschwald/maxminddb`. The executable and config paths remain `maxminddb-mcp`. Users upgrading an extension or plugin must remove the old installation and configure the new one.

## [0.2.0] - 2026-10-17

### Added

- Package an experimental OpenAI desktop plugin with release binaries, a local marketplace catalog, and SHA-256 checksums. Document direct ChatGPT desktop setup.
- Include a shared setup skill in Claude and OpenAI plugins to check database status and perform a sample IP lookup.
- Install new MaxMind editions through MCP and remember them across restarts for scheduled and manual updates.
- Import databases from HTTP(S) URLs, including gzip, tar, and ZIP downloads, with validation and explicit replacement of existing files.
- Configure MaxMind downloads or existing MMDB files through Claude Desktop extension settings, without a config file or CLI.
- Build native MCPB extensions and experimental Claude plugin ZIPs with release binaries. Include them in release checksums.
- Publish MCP Registry metadata from release CI and support automatic Homebrew tap updates after credential setup.
- Report database readiness, active downloads, and update failures through `list_databases`.
- Compare database answers within a bounded CIDR, with paginated changes, progress, cancellation, and tokens bound to loaded database generations.
- Query inclusive start/end IP ranges with filters, field selection, and resumable pages, without duplicate network records.
- Verify loaded databases on demand, with serialized execution and explicit cancellation behavior.
- Summarize matching records by a scalar field, with exact address coverage and bounded scans.
- Inspect full database metadata and bounded samples of field paths, types, and examples with `inspect_database`.
- Select fields from IP, batch, and network lookup results. Support scalar and array records, root-value filters, and array indexes in field paths.
- Add `lookup_ips` for bounded batches with ordered per-address results and errors.
- Advertise five-minute private caching for MCP discovery and tool definitions.
- Publish server usage instructions through initialization and discovery, and add readable tool titles.
- Report optional, throttled MCP progress for database downloads and network scans, including token cursor restoration.

### Changed

- Avoid rational-number allocations when comparing integers of the same signedness or finite floating-point values. Preserve exact comparisons for mixed numeric types.
- Test all five supported MCP revisions, modern request metadata, protocol errors, and stdio cancellation reaching active downloads.
- Limit retained iterators to 128 per server. Existing iterators can continue when new queries and token resumes reach the limit.
- Require Go 1.26 and build with Go 1.27.2. Update mcp-go to 1.3.0, maxminddb-golang/v2 to 2.8.0, geoipupdate to v8, fsnotify, go-toml, and indirect dependencies.
- Update the fixture submodule to an upstream revision using a pinned `mmdbwriter/v2` prerelease. This dependency is used only by the fixture generator.
- Validate MCP input and output schemas. Add tool annotations and use the build's version in server metadata.
- Limit network pages to integers from 1 to 1000. Require an explicit database when a new query or token resume could select more than one database.
- Bind resume tokens to the database name, MMDB type, and build epoch. Tokens from earlier versions must be replaced with a new query.
- Use MMDB metadata for database type, description, and build time. Reject duplicate database filenames across directories.
- Pin formatting and release tools, update GitHub Actions, use Node 24, and add vulnerability and fixture-generator checks to CI.
- Update the container base to Alpine 3.24 and remove its subprocess healthcheck.
- Migrate container releases to `dockers_v2`, preserve architecture-specific tags, and keep stdin open in Docker examples.
- Run server race tests on Linux, Windows, and macOS, including file replacement and stdio shutdown.
- Prepare regexes and field paths once per query.
- Replace obsolete client examples with verified stdio configuration and current tool contracts.

### Fixed

- Enforce URL download limits before HTTP gzip decoding and check ZIP directory limits before allocating entry metadata.
- Exclude IPv4 alias subtrees from summary coverage when a query starts below the alias root. Preserve independent IPv6 records in custom databases.
- Allow slow downloads and update lock waits to finish without a fixed total deadline. Caller cancellation and shutdown still interrupt network operations and lock waits.
- Release newly created iterators when their first page fails or is canceled. Reclaim expired iterators before admitting new queries.
- Return `no_databases` from IP lookups when no database is available, consistent with network lookups.
- Reuse readers for unchanged files while still loading replaced files and recovering unloaded databases.
- Coordinate database updates across server processes with cancellable per-edition file locks.
- Check for database updates at startup when automatic updates are enabled, including missing configured editions.
- Let queued updates and pages cancel promptly. Keep busy iterators from blocking unrelated queries.
- Reject unknown TOML settings and report their names without printing values.
- Load Windows databases into owned memory so replacement and removal can proceed while older readers remain active.
- Prevent panics and unbounded page allocation from invalid `max_results` values.
- Continue after the last returned record and report `has_more` only when another matching record exists.
- Serialize page operations, synchronize iterator timestamps, enforce expiry on access, and release pull cursors during cleanup.
- Honor request cancellation and roll back interrupted pages. Reject unknown iterator IDs and mismatched continuation queries instead of silently restarting.
- Compare numeric equality and ordering across MMDB integer types and JSON numbers without converting large integers to imprecise floats.
- Set MCP `isError` for tool failures, including failed editions in a database update. IP lookups now include `found` and the matched prefix.
- Load GeoIP.conf credentials referenced by TOML. Validate credentials and positive durations before starting timers, and fail when an explicit config file is missing.
- Hash installed databases instead of trusting `.checksums`. Recover missing and modified files, validate downloaded MMDBs, and use unique temporary files.
- Start initial downloads in the background. Cancel work and close managers when stdio ends or the process receives a shutdown signal.
- Replace home-dependent and live-network tests with bundled fixtures and local servers. Add race and stdio coverage for current and legacy protocol revisions.

### Removed

- The unused alternate iterator implementation and its duplicate tests.

## [0.1.0] - 2025-09-07

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
  - TTL-based cleanup for expired iterators
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

- **Duplicate Database Detection**: Warnings for name collisions across directories
- **Flexible Database Storage**: Configurable paths with automatic directory creation
- **Network Timeout Handling**: Robust error handling for network operations

### Security

- **No Hardcoded Credentials**: Configuration-based authentication only
- **Input Validation**: Comprehensive parameter validation for all MCP tools
- **Safe Resource Handling**: Proper cleanup and resource lifecycle management
- **File Permission Awareness**: Handles permission errors gracefully

[Unreleased]: https://github.com/oschwald/maxminddb-mcp/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/oschwald/maxminddb-mcp/releases/tag/v0.3.0
[0.2.0]: https://github.com/oschwald/maxminddb-mcp/releases/tag/v0.2.0
[0.1.0]: https://github.com/oschwald/maxminddb-mcp/releases/tag/v0.1.0
