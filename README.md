# MaxMind MMDB MCP Server

A Model Context Protocol (MCP) server that provides tools for querying MaxMind MMDB databases, including GeoIP2, GeoLite2, and custom MMDB files.

## Features

- **Multiple Operation Modes**: Support for MaxMind accounts, directory scanning, and GeoIP.conf compatibility
- **Advanced Filtering**: Filter network queries by any MMDB field with multiple operators
- **Stateful Iteration**: Efficiently process large network ranges with resumable iterators
- **Auto-updating**: Automatic database downloads and updates from MaxMind
- **File Watching**: Dynamic loading of new/updated database files
- **Configuration Flexibility**: TOML configuration with GeoIP.conf fallback support

## Installation

### From Source

```bash
go install github.com/oschwald/maxminddb-mcp/cmd/maxminddb-mcp@latest
```

### Building Locally

```bash
git clone https://github.com/oschwald/maxminddb-mcp.git
cd maxminddb-mcp
go build -o maxminddb-mcp cmd/maxminddb-mcp/main.go
```

## Configuration

The server supports three configuration modes, checked in this order:

1. Environment variable: `MAXMIND_MCP_CONFIG`
2. User config: `~/.config/maxminddb-mcp/config.toml`
3. System GeoIP.conf: `/etc/GeoIP.conf` (compatibility mode)
4. User GeoIP.conf: `~/.config/maxminddb-mcp/GeoIP.conf`

### TOML Configuration

Create `~/.config/maxminddb-mcp/config.toml`:

```toml
# Mode: "maxmind", "directory", or "geoip_compat"
mode = "maxmind"

# Auto-update settings
auto_update = true
update_interval = "24h"

# Iterator cleanup settings
iterator_ttl = "10m"
iterator_cleanup_interval = "1m"

[maxmind]
# MaxMind account credentials
account_id = 123456
license_key = "your_license_key_here"

# Databases to download (edition IDs)
editions = [
    "GeoLite2-City",
    "GeoLite2-Country",
    "GeoLite2-ASN"
]

# Directory to store downloaded databases
database_dir = "~/.cache/maxminddb-mcp/databases"

# Optional: Custom endpoint
# endpoint = "https://updates.maxmind.com"

[directory]
# Paths to scan for MMDB files (directory mode)
paths = [
    "/path/to/mmdb/files",
    "/another/path"
]

[geoip_compat]
# Path to GeoIP.conf (optional, will search default locations)
config_path = "/etc/GeoIP.conf"
# Override database directory from GeoIP.conf
database_dir = "~/.cache/maxminddb-mcp/databases"
```

### GeoIP.conf Compatibility

For existing GeoIP.conf users, the server can automatically detect and use your existing configuration:

```conf
# Example GeoIP.conf
AccountID 123456
LicenseKey your_license_key_here
EditionIDs GeoLite2-Country GeoLite2-City GeoLite2-ASN
DatabaseDirectory /var/lib/GeoIP
```

## Usage

### Starting the Server

```bash
maxminddb-mcp
```

The server communicates over stdio using the MCP protocol.

### MCP Tools

#### `lookup_ip`

Look up information for a specific IP address.

**Parameters:**
- `ip` (required): IP address to lookup
- `database` (optional): Specific database to query

**Example:**
```json
{
  "name": "lookup_ip",
  "arguments": {
    "ip": "8.8.8.8",
    "database": "GeoLite2-City.mmdb"
  }
}
```

#### `lookup_network`

Look up information for all IPs in a network range with optional filtering.

**Parameters:**
- `network` (required): CIDR network to scan (e.g., "192.168.1.0/24")
- `database` (optional): Specific database to query
- `filters` (optional): Array of filter conditions
- `filter_mode` (optional): "and" or "or" (default: "and")
- `max_results` (optional): Maximum results to return (default: 1000)
- `iterator_id` (optional): Resume existing iterator (fast path)
- `resume_token` (optional): Fallback token if iterator expired

**Filter Examples:**

```json
{
  "name": "lookup_network",
  "arguments": {
    "network": "10.0.0.0/8",
    "filters": [
      {
        "field": "traits.user_type",
        "operator": "equals",
        "value": "residential"
      }
    ],
    "max_results": 500
  }
}
```

**Supported Filter Operators:**
- `equals`: Exact match
- `not_equals`: Not equal to value
- `in`: Value is in provided array
- `not_in`: Value is not in provided array
- `contains`: String contains substring
- `regex`: Matches regular expression
- `greater_than`: Numeric comparison
- `less_than`: Numeric comparison
- `exists`: Field exists (boolean value)

#### `list_databases`

List all available MaxMind databases.

**Example:**
```json
{
  "name": "list_databases",
  "arguments": {}
}
```

#### `update_databases`

Trigger manual update of MaxMind databases (MaxMind/GeoIP modes only).

**Example:**
```json
{
  "name": "update_databases",
  "arguments": {}
}
```

### MCP Resources

#### `/databases/{name}`

Provides metadata about a specific database:
- Name and type
- Description
- Last updated timestamp
- File size

#### `/config`

Returns current server configuration (credentials redacted).

## Advanced Features

### Stateful Iterator System

For large network queries, the server uses a stateful iterator system that:

1. **Fast Path**: Resume active iterations using `iterator_id`
2. **Resilient Path**: Resume from `resume_token` after expiration
3. **Automatic Cleanup**: Expired iterators cleaned up after TTL
4. **Efficient Skip**: Skip to resume point without re-processing

### Filter System

The powerful filter system allows querying by any MMDB field:

```json
{
  "filters": [
    {
      "field": "country.iso_code",
      "operator": "in",
      "value": ["US", "CA", "MX"]
    },
    {
      "field": "traits.is_anonymous_proxy",
      "operator": "equals",
      "value": false
    }
  ],
  "filter_mode": "and"
}
```

### Auto-updating

The server can automatically download and update MaxMind databases:
- Checks MD5 checksums to avoid unnecessary downloads
- Atomic file replacement to prevent corruption
- Configurable update intervals
- Automatic reload in database manager

## Directory Structure

```
maxminddb-mcp/
├── cmd/
│   └── maxminddb-mcp/          # Main entry point
├── internal/
│   ├── config/                 # Configuration management
│   ├── database/               # Database management and updates
│   ├── filter/                 # Query filtering engine
│   ├── iterator/               # Stateful iterator management
│   └── mcp/                    # MCP server implementation
├── test/                       # Integration and performance tests
├── testdata/                   # Test databases and data
├── .git/hooks/                 # Pre-commit hooks
├── go.mod
├── go.sum
├── README.md
└── PLAN.md                     # Detailed implementation plan
```

## Requirements

- Go 1.24 or later
- MaxMind account (for automatic updates)
- MMDB database files

## Dependencies

- `github.com/fsnotify/fsnotify` v1.9.0 - File system notifications
- `github.com/mark3labs/mcp-go` v0.39.1 - MCP protocol implementation
- `github.com/maxmind/geoipupdate/v7` v7.1.1 - MaxMind database updates
- `github.com/oschwald/maxminddb-golang/v2` v2.0.0-beta.10 - MMDB file reading
- `github.com/pelletier/go-toml/v2` v2.2.4 - TOML configuration parsing

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Troubleshooting

### Database Loading Issues

1. Check file permissions on database directories
2. Verify MMDB file format with `file` command
3. Check server logs for specific error messages

### Update Failures

1. Verify MaxMind account credentials
2. Check network connectivity to updates.maxmind.com
3. Ensure sufficient disk space in database directory

### Performance Optimization

1. Use specific databases rather than querying all
2. Apply filters to reduce result sets
3. Use appropriate `max_results` values for your use case
4. Keep iterators active for continued processing

## Examples

### Basic IP Lookup

```bash
echo '{"method": "tools/call", "params": {"name": "lookup_ip", "arguments": {"ip": "8.8.8.8"}}}' | maxminddb-mcp
```

### Network Scan with Filtering

```bash
echo '{"method": "tools/call", "params": {"name": "lookup_network", "arguments": {"network": "192.168.1.0/24", "filters": [{"field": "traits.user_type", "operator": "equals", "value": "residential"}]}}}' | maxminddb-mcp
```