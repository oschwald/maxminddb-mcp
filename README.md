# MaxMindDB MCP Server

[![License: ISC](https://img.shields.io/badge/License-ISC-blue.svg)](https://opensource.org/licenses/ISC)

A powerful Model Context Protocol (MCP) server that provides comprehensive geolocation and network intelligence through MaxMindDB databases. Query GeoIP2, GeoLite2, and custom MMDB files with advanced filtering, stateful iteration, and automatic updates.

> **Important Notice**: This is an **unofficial project** and is **not endorsed by MaxMind Inc.** This MCP server is an independent implementation. For official MaxMind products and support, please visit [maxmind.com](https://www.maxmind.com/).

## Features

- **Multiple Data Sources**: MaxMind accounts, directory scanning, and GeoIP.conf compatibility
- **Advanced Filtering**: Query by any MMDB field with 11+ operators (equals, regex, comparisons, etc.)
- **Stateful Iteration**: Process large network ranges efficiently with resumable iterators
- **Auto-updating**: Automatic database downloads and updates from MaxMind
- **File Watching**: Dynamic loading of new/updated database files
- **Flexible Configuration**: TOML config with GeoIP.conf fallback support

## Quick Start

### Installation

#### Option 1: Install from Go

```bash
go install github.com/oschwald/maxminddb-mcp/cmd/maxminddb-mcp@latest
```

#### Option 2: Build from Source

```bash
git clone https://github.com/oschwald/maxminddb-mcp.git
cd maxminddb-mcp
go build -o maxminddb-mcp cmd/maxminddb-mcp/main.go
```

### Basic Configuration

Create `~/.config/maxminddb-mcp/config.toml`:

```toml
mode = "maxmind"
auto_update = true
update_interval = "24h"

[maxmind]
account_id = 123456
license_key = "your_license_key_here"
editions = ["GeoLite2-City", "GeoLite2-Country", "GeoLite2-ASN"]
database_dir = "~/.cache/maxminddb-mcp/databases"
```

## Client Integration

<details>
<summary>Claude Desktop</summary>

Add to your Claude Desktop configuration file:

**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows**: `%APPDATA%/Claude/claude_desktop_config.json`
**Linux**: `~/.config/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "maxminddb": {
      "command": "maxminddb-mcp",
      "env": {
        "MAXMINDDB_MCP_CONFIG": "/path/to/your/config.toml"
      }
    }
  }
}
```

Alternative with existing GeoIP.conf:

```json
{
  "mcpServers": {
    "maxminddb": {
      "command": "maxminddb-mcp"
    }
  }
}
```

</details>

<details>
<summary>Claude Code CLI</summary>

Add the MCP server to Claude Code CLI:

```bash
claude mcp add maxminddb maxminddb-mcp
```

To use with a custom config:

```bash
MAXMINDDB_MCP_CONFIG=/path/to/config.toml claude chat
```

</details>

<details>
<summary>Claude Code (VS Code Extension)</summary>

Install the Claude Code extension and add to VS Code settings:

```json
{
  "claude.mcpServers": {
    "maxminddb": {
      "command": "maxminddb-mcp",
      "env": {
        "MAXMINDDB_MCP_CONFIG": "/path/to/your/config.toml"
      }
    }
  }
}
```

</details>

<details>
<summary>Continue</summary>

Install the Continue extension and add to your Continue configuration (`~/.continue/config.json`):

```json
{
  "mcpServers": [
    {
      "name": "maxminddb",
      "command": "maxminddb-mcp",
      "env": {
        "MAXMINDDB_MCP_CONFIG": "/path/to/your/config.toml"
      }
    }
  ]
}
```

</details>

<details>
<summary>Zed</summary>

Add to Zed settings (`~/.config/zed/settings.json`):

```json
{
  "assistant": {
    "mcp_servers": [
      {
        "name": "maxminddb",
        "command": "maxminddb-mcp",
        "env": {
          "MAXMINDDB_MCP_CONFIG": "/path/to/your/config.toml"
        }
      }
    ]
  }
}
```

</details>

<details>
<summary>Cline</summary>

Install Cline and add to VS Code settings:

```json
{
  "cline.mcpServers": {
    "maxminddb": {
      "command": "maxminddb-mcp",
      "env": {
        "MAXMINDDB_MCP_CONFIG": "/path/to/your/config.toml"
      }
    }
  }
}
```

</details>

<details>
<summary>Gemini CLI</summary>

Add to your Gemini CLI configuration:

```json
{
  "mcpServers": {
    "maxminddb": {
      "command": "maxminddb-mcp",
      "env": {
        "MAXMINDDB_MCP_CONFIG": "/path/to/your/config.toml"
      }
    }
  }
}
```

See the [Gemini CLI MCP guide](https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md) for more details.

</details>

<details>
<summary>Codex</summary>

Add to your Codex configuration file:

```toml
[mcp_servers.maxminddb]
command = "maxminddb-mcp"
env = { MAXMINDDB_MCP_CONFIG = "/path/to/your/config.toml" }
```

Or without custom config:

```toml
[mcp_servers.maxminddb]
command = "maxminddb-mcp"
```

</details>

<details>
<summary>Sourcegraph Cody</summary>

Add to Cody settings:

```json
{
  "cody.experimental.mcp": {
    "servers": {
      "maxminddb": {
        "command": "maxminddb-mcp",
        "env": {
          "MAXMINDDB_MCP_CONFIG": "/path/to/your/config.toml"
        }
      }
    }
  }
}
```

</details>

<details>
<summary>LLM (Simon Willison)</summary>

Install the LLM tool and configure MCP:

```bash
# Install LLM
pip install llm

# Configure MCP server
llm mcp install maxminddb-mcp --command maxminddb-mcp

# Use with environment variable
MAXMINDDB_MCP_CONFIG=/path/to/config.toml llm chat -m claude-3.5-sonnet
```

</details>

<details>
<summary>Python SDK</summary>

```bash
pip install mcp
```

```python
from mcp import ClientSession, StdioServerParameters

async with ClientSession(
    StdioServerParameters(
        command="maxminddb-mcp",
        env={"MAXMINDDB_MCP_CONFIG": "/path/to/config.toml"}
    )
) as session:
    await session.initialize()
    tools = await session.list_tools()
    result = await session.call_tool("lookup_ip", {"ip": "8.8.8.8"})
```

</details>

<details>
<summary>TypeScript SDK</summary>

```bash
npm install @modelcontextprotocol/sdk
```

```typescript
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";

const transport = new StdioClientTransport({
  command: "maxminddb-mcp",
  env: { MAXMINDDB_MCP_CONFIG: "/path/to/config.toml" },
});

const client = new Client(
  {
    name: "maxminddb-client",
    version: "1.0.0",
  },
  { capabilities: {} }
);

await client.connect(transport);
const result = await client.callTool({
  name: "lookup_ip",
  arguments: { ip: "8.8.8.8" },
});
```

</details>

<details>
<summary>Go SDK</summary>

```bash
go get github.com/mark3labs/mcp-go
```

```go
import (
    "github.com/mark3labs/mcp-go/client"
    "github.com/mark3labs/mcp-go/transport/stdio"
)

cmd := exec.Command("maxminddb-mcp")
cmd.Env = append(cmd.Env, "MAXMINDDB_MCP_CONFIG=/path/to/config.toml")

transport := stdio.NewTransport(cmd)
mcpClient := client.New(transport)
// ... use client
```

</details>

<details>
<summary>Command Line Testing</summary>

Test the server directly using JSON-RPC:

```bash
# List available tools
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | maxminddb-mcp

# Test IP lookup
echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"lookup_ip","arguments":{"ip":"8.8.8.8"}}}' | maxminddb-mcp

# Pretty output with jq
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | maxminddb-mcp | jq .
```

</details>

### Configuration Notes

**Path Requirements**: Ensure `maxminddb-mcp` is in your system PATH or provide the full path to the binary.

**Environment Variables**: All clients support these environment variables:

- `MAXMINDDB_MCP_CONFIG`: Path to configuration file
- `MAXMINDDB_MCP_LOG_LEVEL`: Logging level (`debug`, `info`, `warn`, `error`)
- `MAXMINDDB_MCP_LOG_FORMAT`: Log format (`text`, `json`)

**Security**: Store sensitive configuration (API keys) in files with appropriate permissions (600) rather than environment variables in client configs.

## Configuration

### Configuration Modes

The server supports three configuration modes (checked in order):

1. **Environment variable**: `MAXMINDDB_MCP_CONFIG`
2. **User config**: `~/.config/maxminddb-mcp/config.toml`
3. **GeoIP.conf compatibility**: `/etc/GeoIP.conf` or `~/.config/maxminddb-mcp/GeoIP.conf`

### TOML Configuration

<details>
<summary>Complete configuration example</summary>

```toml
# Operation mode: "maxmind", "directory", or "geoip_compat"
mode = "maxmind"

# Auto-update settings
auto_update = true
update_interval = "24h"

# Iterator settings
iterator_ttl = "10m"
iterator_cleanup_interval = "1m"

# Logging (optional)
log_level = "info"  # debug, info, warn, error
log_format = "text" # text, json

[maxmind]
# MaxMind account credentials
account_id = 123456
license_key = "your_license_key_here"

# Databases to download
editions = [
    "GeoLite2-City",
    "GeoLite2-Country",
    "GeoLite2-ASN",
    "GeoIP2-City",
    "GeoIP2-Country"
]

# Storage location
database_dir = "~/.cache/maxminddb-mcp/databases"

# Custom endpoint (optional)
# endpoint = "https://updates.maxmind.com"

[directory]
# For directory mode - scan these paths for MMDB files
paths = [
    "/path/to/mmdb/files",
    "/another/path"
]

[geoip_compat]
# For GeoIP.conf compatibility mode
config_path = "/etc/GeoIP.conf"
database_dir = "/var/lib/GeoIP"
```

</details>

#### Configuration Options

**Iterator Settings:**

- `iterator_ttl` (default: "10m"): How long idle iterators are kept before cleanup
- `iterator_cleanup_interval` (default: "1m"): How often to check for expired iterators

### GeoIP.conf Compatibility

<details>
<summary>Existing GeoIP.conf users</summary>

The server automatically detects and uses existing GeoIP.conf files:

```conf
# Example GeoIP.conf
AccountID 123456
LicenseKey your_license_key_here
EditionIDs GeoLite2-Country GeoLite2-City GeoLite2-ASN
DatabaseDirectory /var/lib/GeoIP
```

No additional configuration needed - the server will automatically use compatibility mode.

</details>

## Available Tools

### Core Tools

#### `lookup_ip`

Look up geolocation and network information for a specific IP address.

**Parameters:**

- `ip` (required): IP address to lookup (IPv4 or IPv6)
- `database` (optional): Specific database filename to query

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

**Response:**

```json
{
  "ip": "8.8.8.8",
  "network": "8.8.8.0/24",
  "data": {
    "country": {
      "iso_code": "US",
      "names": { "en": "United States" }
    },
    "location": {
      "latitude": 37.4056,
      "longitude": -122.0775
    }
  }
}
```

#### `lookup_network`

Query all IP addresses in a network range with powerful filtering capabilities.

**Parameters:**

- `network` (required): CIDR network to scan (e.g., "192.168.1.0/24")
- `database` (optional): Specific database to query
- `filters` (optional): Array of filter objects. Each object must include `field`, `operator`, and `value`.
- `filter_mode` (optional): "and" (default) or "or"
- `max_results` (optional): Maximum results to return (default: 1000)
- `iterator_id` (optional): Resume existing iterator
- `resume_token` (optional): Fallback token for expired iterators

<details>
<summary>Filtering Examples</summary>

**Filter by country:**

```json
{
  "name": "lookup_network",
  "arguments": {
    "network": "10.0.0.0/8",
    "filters": [
      {
        "field": "country.iso_code",
        "operator": "in",
        "value": ["US", "CA", "MX"]
      }
    ]
  }
}
```

**Filter residential IPs:**

```json
{
  "name": "lookup_network",
  "arguments": {
    "network": "192.168.0.0/16",
    "filters": [
      {
        "field": "traits.user_type",
        "operator": "equals",
        "value": "residential"
      }
    ]
  }
}
```

Common mistakes and validation

- Do not pass filters as strings like `"traits.user_type=residential"`. The server rejects this with `invalid_filter` and a hint to use objects: `{ "field": "traits.user_type", "operator": "equals", "value": "residential" }`.
- `filters` must be an array of objects; other types are invalid.
- `operator` must be supported (see list below). Short aliases (`eq`, `ne`, `gt`, `gte`, `lt`, `lte`) are also accepted.

**Complex filtering (non-proxy IPs):**

```json
{
  "name": "lookup_network",
  "arguments": {
    "network": "10.0.0.0/24",
    "filters": [
      {
        "field": "traits.is_anonymous_proxy",
        "operator": "equals",
        "value": false
      },
      {
        "field": "traits.is_satellite_provider",
        "operator": "equals",
        "value": false
      }
    ],
    "filter_mode": "and"
  }
}
```

</details>

#### `list_databases`

List all available MaxMind databases with metadata.

**Example:**

```json
{
  "name": "list_databases",
  "arguments": {}
}
```

**Response:**

```json
{
  "databases": [
    {
      "name": "GeoLite2-City.mmdb",
      "type": "City",
      "description": "GeoLite2 City Database",
      "last_updated": "2024-01-15T10:30:00Z",
      "size": 67108864
    }
  ]
}
```

#### `update_databases`

Manually trigger database updates (MaxMind/GeoIP modes only).

**Example:**

```json
{
  "name": "update_databases",
  "arguments": {}
}
```

### Filter Operators

**Supported Operators:**

- `equals`: Exact match
- `not_equals`: Not equal to value
- `in`: Value is in provided array
- `not_in`: Value is not in provided array
- `contains`: String contains substring
- `regex`: Matches regular expression
- `greater_than`: Numeric comparison
- `greater_than_or_equal`: Numeric comparison (≥)
- `less_than`: Numeric comparison
- `less_than_or_equal`: Numeric comparison (≤)
- `exists`: Field exists (boolean value)

**Operator Aliases:**
For convenience, short operator aliases are supported (case-insensitive):

- `eq` → `equals`
- `ne` → `not_equals`
- `gt` → `greater_than`
- `gte` → `greater_than_or_equal`
- `lt` → `less_than`
- `lte` → `less_than_or_equal`

### Error Handling

All tools return structured error responses with machine-readable error codes:

```json
{
  "error": {
    "code": "db_not_found",
    "message": "Database not found: invalid_db.mmdb"
  }
}
```

**Common Error Codes:**

- `db_not_found`: Specified database does not exist
- `invalid_ip`: IP address format is invalid
- `invalid_network`: Network CIDR format is invalid
- `invalid_filter`: Filter validation failed
- `iterator_not_found`: Iterator ID not found or expired
- `parse_error`: Failed to parse request parameters

## Advanced Features

### Stateful Iterator System

For large network queries, the server uses a stateful iterator system:

1. **Fast Path**: Resume active iterations using `iterator_id`
2. **Resilient Path**: Resume from `resume_token` after expiration
3. **Automatic Cleanup**: Expired iterators cleaned up after TTL
4. **Efficient Skip**: Skip to resume point without re-processing

**Example iteration workflow:**

```json
// First call - creates iterator
{
  "name": "lookup_network",
  "arguments": {
    "network": "10.0.0.0/8",
    "max_results": 1000
  }
}

// Response includes iterator_id for continuation
{
  "results": [...],
  "iterator_id": "iter_abc123",
  "resume_token": "eyJ0eXAiOiJKV1Q...",
  "has_more": true
}

// Continue with iterator_id (fast path)
{
  "name": "lookup_network",
  "arguments": {
    "network": "10.0.0.0/8",
    "iterator_id": "iter_abc123",
    "max_results": 1000
  }
}
```

### Auto-updating

<details>
<summary>Automatic database updates</summary>

Configure automatic updates in your TOML config:

```toml
auto_update = true
update_interval = "24h"  # Check every 24 hours
```

The server will:

- Check for database updates on the specified interval
- Download only if MD5 checksums have changed
- Gracefully reload databases without interrupting active queries
- Log update status and any errors

**Manual Updates:**
Use the `update_databases` tool to trigger immediate updates.

</details>

### File Watching

<details>
<summary>Directory mode with file watching</summary>

In directory mode, the server watches for filesystem changes:

```toml
mode = "directory"

[directory]
paths = ["/path/to/mmdb/files"]
```

**Supported events:**

- **Create**: Automatically loads new MMDB files
- **Write**: Reloads modified databases
- **Remove**: Removes databases from available list
- **Rename**: Handles file renames gracefully

**Subdirectory support:** Recursively watches all subdirectories for MMDB files.

</details>

## Troubleshooting

### Common Issues

<details>
<summary>Server not starting</summary>

**Check configuration:**

```bash
# Verify config file syntax
maxminddb-mcp --help

# Test configuration loading
MAXMINDDB_MCP_LOG_LEVEL=debug maxminddb-mcp
```

**Common causes:**

- Invalid TOML syntax in config file
- Missing MaxMind credentials
- Insufficient file permissions
- Invalid database directory path

</details>

<details>
<summary>Database loading failures</summary>

**Check database status:**

```bash
# Enable debug logging
MAXMINDDB_MCP_LOG_LEVEL=debug maxminddb-mcp
```

**Common causes:**

- Corrupt MMDB files (check file integrity)
- Insufficient disk space for downloads
- Network connectivity issues to updates.maxmind.com
- Expired MaxMind subscription

</details>

<details>
<summary>Claude Desktop integration</summary>

**Verify server path:**

```bash
# Check server is in PATH
which maxminddb-mcp

# Test server directly
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | maxminddb-mcp
```

**Configuration file locations:**

- **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows**: `%APPDATA%/Claude/claude_desktop_config.json`

</details>

### Debug Mode

Enable detailed logging for troubleshooting:

```bash
# Environment variables
MAXMINDDB_MCP_LOG_LEVEL=debug
MAXMINDDB_MCP_LOG_FORMAT=json

# Or in config.toml
log_level = "debug"
log_format = "json"
```

### Configuration Validation

The server validates all configuration on startup and provides detailed error messages:

- Required fields for each mode
- Valid duration formats (e.g., "24h", "10m")
- Path expansion and validation
- Network connectivity checks

## Performance Considerations

### Memory Usage

- **Base memory**: ~50MB
- **Database storage**: 100-500MB depending on editions

### Optimization Tips

- Avoid unnecessary iterations: use selective filters and appropriate `max_results`
- **Database selection**: Only download needed editions to reduce memory usage
- **Update frequency**: Balance freshness vs. network usage with `update_interval`
- **Filter efficiency**: Use selective filters early to reduce processing

### Resource Limits

- **Concurrent iterators**: No hard limit, managed by TTL cleanup
- **Network query size**: Limited by available memory and `max_results`
- **Database file size**: Supports databases up to several GB

## License

This project is licensed under the ISC License - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details on how to submit pull requests, report issues, and suggest improvements.

## Support

- **Issues**: [GitHub Issues](https://github.com/oschwald/maxminddb-mcp/issues)
- **Discussions**: [GitHub Discussions](https://github.com/oschwald/maxminddb-mcp/discussions)
