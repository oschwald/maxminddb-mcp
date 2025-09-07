# MaxMindDB MCP Server

A powerful Model Context Protocol (MCP) server that provides comprehensive geolocation and network intelligence through MaxMindDB databases. Query GeoIP2, GeoLite2, and custom MMDB files with advanced filtering, stateful iteration, and automatic updates.

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

### Claude Desktop

The official Anthropic Claude Desktop app with built-in MCP support.

#### Installation & Setup

1. **Install Claude Desktop**:

   - **macOS**: Download from [claude.ai](https://claude.ai/download)
   - **Windows**: Download from [claude.ai](https://claude.ai/download)

2. **Configure the MCP Server**:

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

3. **Restart Claude Desktop** to load the MCP server

<details>
<summary>Alternative: Use existing GeoIP.conf</summary>

If you already have GeoIP.conf configured:

```json
{
  "mcpServers": {
    "maxminddb": {
      "command": "maxminddb-mcp"
    }
  }
}
```

The server will automatically detect `/etc/GeoIP.conf` or `~/.config/maxminddb-mcp/GeoIP.conf`.

</details>

### Continue

A powerful AI assistant with native MCP support.

#### Installation & Setup

1. **Install Continue**:

   ```bash
   # VS Code Extension
   code --install-extension Continue.continue

   # Or install via VS Code marketplace
   ```

2. **Configure MCP Server**:

   Add to your Continue configuration (`~/.continue/config.json`):

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

### Zed Editor

Modern code editor with MCP integration.

#### Installation & Setup

1. **Install Zed**:

   ```bash
   # macOS
   brew install zed

   # Linux
   curl -f https://zed.dev/install.sh | sh
   ```

2. **Configure MCP Server**:

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

### Cline (VS Code Extension)

AI-powered coding assistant with MCP support.

#### Installation & Setup

1. **Install Cline**:

   ```bash
   # Install via VS Code marketplace
   code --install-extension saoudrizwan.claude-dev
   ```

2. **Configure MCP Server**:

   Add to Cline settings in VS Code settings.json:

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

### MCP Client Libraries

For developers building custom MCP clients.

#### Python

```bash
# Install MCP Python SDK
pip install mcp

# Example usage
from mcp import ClientSession, StdioServerParameters

async with ClientSession(
    StdioServerParameters(
        command="maxminddb-mcp",
        env={"MAXMINDDB_MCP_CONFIG": "/path/to/config.toml"}
    )
) as session:
    # Initialize MCP session
    await session.initialize()

    # List available tools
    tools = await session.list_tools()

    # Call lookup_ip tool
    result = await session.call_tool(
        "lookup_ip",
        {"ip": "8.8.8.8"}
    )
```

#### TypeScript/JavaScript

```bash
# Install MCP TypeScript SDK
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
  {
    capabilities: {},
  },
);

await client.connect(transport);

// List available tools
const tools = await client.listTools();

// Call lookup_ip tool
const result = await client.callTool({
  name: "lookup_ip",
  arguments: { ip: "8.8.8.8" },
});
```

#### Go

```bash
# Install MCP Go SDK
go get github.com/mark3labs/mcp-go
```

```go
package main

import (
    "context"
    "os/exec"

    "github.com/mark3labs/mcp-go/client"
    "github.com/mark3labs/mcp-go/transport/stdio"
)

func main() {
    cmd := exec.Command("maxminddb-mcp")
    cmd.Env = append(cmd.Env, "MAXMINDDB_MCP_CONFIG=/path/to/config.toml")

    transport := stdio.NewTransport(cmd)
    mcpClient := client.New(transport)

    ctx := context.Background()

    // Initialize connection
    if err := mcpClient.Initialize(ctx); err != nil {
        panic(err)
    }

    // List tools
    tools, err := mcpClient.ListTools(ctx)
    if err != nil {
        panic(err)
    }

    // Call lookup_ip tool
    result, err := mcpClient.CallTool(ctx, "lookup_ip", map[string]any{
        "ip": "8.8.8.8",
    })
    if err != nil {
        panic(err)
    }
}
```

### Command Line Testing

For testing and development purposes.

#### Direct JSON-RPC

```bash
# Test server initialization
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"clientInfo":{"name":"test","version":"1.0"}}}' | maxminddb-mcp

# List available tools
echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | maxminddb-mcp

# Test IP lookup
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"lookup_ip","arguments":{"ip":"8.8.8.8"}}}' | maxminddb-mcp
```

#### Using jq for Pretty Output

```bash
# Pretty-printed tool listing
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | maxminddb-mcp | jq .

# IP lookup with formatted output
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"lookup_ip","arguments":{"ip":"8.8.8.8"}}}' | maxminddb-mcp | jq '.result.content[0].text | fromjson'
```

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
iterator_buffer = 100

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

- `iterator_buffer` (default: 100): Channel buffer size for network iteration streaming. Higher values improve throughput for large network queries but use more memory. Values ≤ 0 are automatically clamped to the default.
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
- `filters` (optional): Array of filter conditions
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
- **Iterator buffers**: Configurable (default: 100 items)

### Optimization Tips

- **Iterator buffer size**: Increase `iterator_buffer` for high-throughput scenarios
- **Database selection**: Only download needed editions to reduce memory usage
- **Update frequency**: Balance freshness vs. network usage with `update_interval`
- **Filter efficiency**: Use selective filters early to reduce processing

### Resource Limits

- **Concurrent iterators**: No hard limit, managed by TTL cleanup
- **Network query size**: Limited by available memory and `max_results`
- **Database file size**: Supports databases up to several GB

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details on how to submit pull requests, report issues, and suggest improvements.

## Support

- **Issues**: [GitHub Issues](https://github.com/oschwald/maxminddb-mcp/issues)
- **Discussions**: [GitHub Discussions](https://github.com/oschwald/maxminddb-mcp/discussions)
