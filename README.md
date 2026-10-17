# MaxMindDB MCP Server

[![License: ISC](https://img.shields.io/badge/License-ISC-blue.svg)](https://opensource.org/licenses/ISC)

Query GeoIP2, GeoLite2, and custom MMDB files through the Model Context Protocol (MCP). The server supports IP lookups, filtered network queries, resumable pages, and MaxMind database downloads.

This is an unofficial project and is not endorsed by MaxMind Inc. For official products and support, visit [maxmind.com](https://www.maxmind.com/).

## Install

Use Go 1.26 or later. The module selects Go 1.27.2 for builds.

```bash
go install github.com/oschwald/maxminddb-mcp/cmd/maxminddb-mcp@latest
```

To build and test this checkout:

```bash
git clone --recurse-submodules https://github.com/oschwald/maxminddb-mcp.git
cd maxminddb-mcp
go build -o bin/maxminddb-mcp ./cmd/maxminddb-mcp
go test -race ./...
```

The test fixture generator uses Go 1.27 and a pinned prerelease of `mmdbwriter/v2`. It is a separate module in `testdata` and is not a runtime dependency.

## Configure

Create `~/.config/maxminddb-mcp/config.toml`. Choose one mode.

### Download from MaxMind

```toml
mode = "maxmind"
auto_update = true
update_interval = "24h"
iterator_ttl = "10m"
iterator_cleanup_interval = "1m"

[maxmind]
account_id = 123456
license_key = "your_license_key_here"
editions = ["GeoLite2-City", "GeoLite2-ASN"]
database_dir = "~/.cache/maxminddb-mcp/databases"
# endpoint = "https://updates.maxmind.com"
```

Keep files containing credentials readable only by their owner, for example with `chmod 600 config.toml`.

When no databases are loaded, an initial download starts in the background. MCP requests can run during that download. Until it finishes, `list_databases` can return an empty list. Each download has a five-minute deadline.

When `auto_update` is true, the server checks all configured editions in the background at startup, then on schedule. This also downloads missing editions when other databases are already installed. `update_databases` supports manual updates. Downloads use a unique temporary file and must pass checksum and MMDB validation before replacing the installed file. Missing or changed files are detected from their actual contents. Old `.checksums` files are ignored.

### Use local MMDB files

```toml
mode = "directory"

[directory]
paths = ["/absolute/path/to/databases"]
```

The server loads MMDB files recursively and watches the directories present at startup. Restart the server after adding subdirectories. Database names are filenames, including `.mmdb`, and must be unique across configured directories. Duplicate names are rejected.

Replace databases by writing a separate file and renaming it into place. Active iterators retain their original reader. In-place writes can invalidate a memory-mapped file.

### Use an existing GeoIP.conf

```toml
mode = "geoip_compat"

[geoip_compat]
config_path = "/etc/GeoIP.conf"
# Optional override of DatabaseDirectory from GeoIP.conf:
# database_dir = "/var/lib/GeoIP"
```

The server reads `AccountID`, `LicenseKey`, `EditionIDs`, `DatabaseDirectory`, and `Host` from the referenced file. `UserId` and `ProductIds` remain accepted aliases. A `Host` without a URL scheme uses HTTPS. Other geoipupdate options, including its proxy and retry settings, are not applied by this server.

If `config_path` is omitted, the server searches the user GeoIP.conf, `/etc/GeoIP.conf`, and `/usr/local/etc/GeoIP.conf`, in that order.

### Config discovery and defaults

An explicit `MAXMINDDB_MCP_CONFIG` must point to an existing `.toml` file or a file named `GeoIP.conf`. A missing explicit file is an error. Otherwise, the server searches:

1. `~/.config/maxminddb-mcp/config.toml`
2. `~/.config/maxminddb-mcp/GeoIP.conf`
3. `/etc/GeoIP.conf`

The default database directory is `~/.cache/maxminddb-mcp/databases`. Database paths and `geoip_compat.config_path` beginning with `~/` are expanded. Duration defaults are `24h` for updates, `10m` for iterator idle expiry, and `1m` for cleanup. All durations must be positive, including when automatic updates are disabled.

Logging uses environment variables:

| Variable                   | Values                                     |
| -------------------------- | ------------------------------------------ |
| `MAXMINDDB_MCP_CONFIG`     | Config file path                           |
| `MAXMINDDB_MCP_LOG_LEVEL`  | `debug`, `info` (default), `warn`, `error` |
| `MAXMINDDB_MCP_LOG_FORMAT` | `text` (default), `json`                   |

TOML `log_level` and `log_format` settings are not supported. Logs go to stderr. Stdout carries only MCP messages.

## Connect a client

This is a local **stdio** server. Configure your client to launch the installed binary, using an absolute path if it is not in the client's PATH. The server does not expose an HTTP endpoint.

### Codex

```bash
codex mcp add maxminddb \
  --env MAXMINDDB_MCP_CONFIG=/absolute/path/to/config.toml \
  -- /absolute/path/to/maxminddb-mcp
```

Or add to `~/.codex/config.toml`:

```toml
[mcp_servers.maxminddb]
command = "/absolute/path/to/maxminddb-mcp"
env = { MAXMINDDB_MCP_CONFIG = "/absolute/path/to/config.toml" }
```

See the [official Codex MCP documentation](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

### Claude Code

```bash
claude mcp add \
  --env MAXMINDDB_MCP_CONFIG=/absolute/path/to/config.toml \
  --transport stdio maxminddb -- /absolute/path/to/maxminddb-mcp
```

See the [Claude Code MCP documentation](https://code.claude.com/docs/en/mcp).

### Zed

Add a local server in Settings → AI → MCP Servers, or use:

```json
{
  "context_servers": {
    "maxminddb": {
      "command": "/absolute/path/to/maxminddb-mcp",
      "args": [],
      "env": {
        "MAXMINDDB_MCP_CONFIG": "/absolute/path/to/config.toml"
      }
    }
  }
}
```

See the [Zed MCP documentation](https://zed.dev/docs/ai/mcp). For other clients and SDKs, use their current stdio transport instructions with the same command and environment variables.

### Docker

Use interactive stdin (`-i`) without a TTY. Set `database_dir = "/data"` in a MaxMind-mode config:

```bash
docker run --rm -i \
  -v "$HOME/.config/maxminddb-mcp/config.toml:/config/config.toml:ro" \
  -v maxminddb-data:/data \
  ghcr.io/oschwald/maxminddb-mcp:latest
```

The image runs as UID 1001. A config bind mount must be readable by that user. A data bind mount must be writable by that user when downloads are enabled. Docker named volumes inherit the image's `/data` permissions on first use.

The container has no HTTP port or subprocess healthcheck. Check the running MCP connection from the client.

## Tools

Successful results include structured JSON and a JSON text fallback. Tool execution failures set MCP `isError: true`. Schema validation rejects unknown arguments and invalid types. Read tools are annotated as local and read-only. `update_databases` is annotated as a network operation that replaces files.

### `lookup_ip`

Arguments: required `ip` (IPv4 or IPv6), optional `database` from `list_databases`.

```json
{
  "name": "lookup_ip",
  "arguments": { "ip": "1.0.0.1", "database": "GeoLite2-ASN.mmdb" }
}
```

A single-database result contains `ip`, `database`, `found`, `network`, and `data`. If no record matches, `found` is false and `network` and `data` are null. Without `database`, the result contains `ip` and a `databases` map keyed by database name. Each entry contains `found`, `network`, and `data`.

### `lookup_network`

Returns database network records overlapping a CIDR range. It does not return one result per IP address.

| Argument       | Meaning                                                                              |
| -------------- | ------------------------------------------------------------------------------------ |
| `network`      | Required IPv4 or IPv6 CIDR range                                                     |
| `database`     | Required for a new query or token resume if more than one database is loaded         |
| `filters`      | Optional array of objects with `field`, `operator`, and `value`                      |
| `filter_mode`  | `and` (default) or `or`                                                              |
| `max_results`  | Integer from 1 to 1000, default 1000                                                 |
| `iterator_id`  | Continue a live iterator                                                             |
| `resume_token` | Resume after expiry or restart, or fall back if `iterator_id` is no longer available |

```json
{
  "name": "lookup_network",
  "arguments": {
    "network": "1.0.0.0/8",
    "database": "GeoLite2-ASN.mmdb",
    "max_results": 100,
    "filters": [
      {
        "field": "autonomous_system_number",
        "operator": "equals",
        "value": 13335
      }
    ]
  }
}
```

Each page contains `results` (objects with `network` and `data`), `iterator_id`, `resume_token`, `has_more`, `total_processed`, and `total_matched`. `has_more` means at least one additional matching record exists. Totals are cumulative. Processed totals can include nonmatching records scanned ahead to determine whether another page exists.

For the next page, send the same `network` and the returned `iterator_id`. Retain `database` and `resume_token` for fallback. Omit `filters` and `filter_mode` to reuse the original values. Supplied query parameters must match the original query. Page size can change.

An unknown or expired ID without a token returns `iterator_expired`. Token resumes validate the database name, MMDB type, and build epoch. If the database build changed, start a new query. Tokens from earlier server versions are not accepted. Tokens are opaque continuation data, not authentication credentials.

Live iterators retain a cursor. Token resumes scan to the saved position once. Requests on the same iterator run serially. Cancellation rolls back an incomplete page so it can be retried without losing records.

A page limit bounds the returned records, not the work needed to find matches. Use a narrow CIDR and cancel long scans through the client. Idle iterators expire after `iterator_ttl`. There is no hard limit on the number of live iterators.

### Filters

Fields use dot notation, such as `country.iso_code`. Operators and aliases use lowercase:

| Operators                                             | Value                                |
| ----------------------------------------------------- | ------------------------------------ |
| `equals`, `not_equals` (`eq`, `ne`)                   | A value to compare                   |
| `in`, `not_in`                                        | An array of values                   |
| `contains`, `regex`                                   | A substring or Go regular expression |
| `greater_than`, `greater_than_or_equal` (`gt`, `gte`) | A number                             |
| `less_than`, `less_than_or_equal` (`lt`, `lte`)       | A number                             |
| `exists`                                              | A boolean                            |

Numeric equality works across decoded MMDB integers and JSON numbers. JSON clients can lose precision for integers above 2⁵³. Numeric ordering also converts values to floating point.

### `list_databases`

Accepts no arguments. Returns `databases`, sorted by filename. Each entry contains `name`, `type`, `description`, `last_updated`, and file `size` in bytes. Type, English description, and build time come from MMDB metadata. `last_updated` is the build time, not the file's modification time.

### `update_databases`

Available in `maxmind` and `geoip_compat` modes. Accepts no arguments. Returns a `results` array with per-edition `database`, `updated`, `last_update`, and optional `size` or `error`. Any failed edition makes the tool result an MCP error while preserving all edition results.

### Errors

Application errors include an `error` object with `code` and `message`. Common codes are `invalid_ip`, `invalid_network`, `invalid_filter`, `db_not_found`, `no_databases`, `database_required`, `iterator_expired`, `query_mismatch`, and `resume_failed`. Input-schema errors use the SDK's error text.

## Protocol and development

Stdio integration tests cover the legacy initialize handshake (`2024-11-05` and `2025-11-25`) and the `2026-07-28` discovery flow. Protocol negotiation and transport behavior are provided by `mcp-go`. See the [MCP specification](https://modelcontextprotocol.io/specification/2026-07-28).

Use an MCP client for manual checks. A one-line pipe that closes stdin immediately can cancel a request before its response is written. Automated protocol tests are available with:

```bash
go test -race ./internal/mcp ./cmd/maxminddb-mcp
```

For development, release checks, and fixture generation, see [CONTRIBUTING.md](CONTRIBUTING.md). Report issues at [GitHub Issues](https://github.com/oschwald/maxminddb-mcp/issues).

## License

[ISC](LICENSE).
