# MaxMindDB MCP Server

[![License: ISC](https://img.shields.io/badge/License-ISC-blue.svg)](https://opensource.org/licenses/ISC)

Query GeoIP2, GeoLite2, and custom MMDB files through the Model Context Protocol (MCP). The server supports IP lookups, filtered network queries, resumable pages, and MaxMind database downloads.

This is an unofficial project and is not endorsed by MaxMind Inc. For official products and support, visit [maxmind.com](https://www.maxmind.com/).

Example prompts after connecting the server:

- "Using MaxMindDB, identify the country and autonomous system for 8.8.8.8."
- "Find the networks belonging to AS13335 within 1.0.0.0/8 in my ASN database."
- "Which databases are available, and when were they built?"

Answers depend on the databases you provide. IP geolocation is an estimate, not a device's precise location. ASN data describes network ownership, not the identity of a person using an address.

## Installation

### Claude Desktop

Install and configure the server through Claude Desktop. You do not need Go, a terminal, or a config file.

1. Download a `.mcpb` file from [GitHub Releases](https://github.com/oschwald/maxminddb-mcp/releases).
   Choose the file for your computer:

   | Computer                      | Filename suffix       |
   | ----------------------------- | --------------------- |
   | Mac with Apple silicon        | `_darwin_arm64.mcpb`  |
   | Mac with an Intel processor   | `_darwin_amd64.mcpb`  |
   | Windows with an x64 processor | `_windows_amd64.mcpb` |
   | Linux x64                     | `_linux_amd64.mcpb`   |
   | Linux ARM64                   | `_linux_arm64.mcpb`   |

2. In Claude Desktop, open **Settings → Extensions → Advanced settings → Install Extension** and select the file.
3. Configure one of these choices in the extension settings:
   - **Download databases:** Leave **Download and update MaxMind databases** enabled. Enter your MaxMind account ID and license key. The default editions are `GeoLite2-City` and `GeoLite2-ASN`. Change these for other editions available to your account. Leave **Database folder** blank to use the application cache, or select a writable folder.
   - **Use existing files:** Turn off downloads and select the **Database folder** containing your MMDB files. Account settings are ignored in this mode.
4. Enable the extension and ask: "Using MaxMindDB, list my databases and look up 8.8.8.8."

The initial download runs in the background. Claude can check its status with `list_databases`. You can change the settings later in the same Extensions panel. Restart the extension after changing settings if Claude has not restarted it automatically.

Claude stores the license key as a sensitive setting. Downloaded files stay outside the extension, so replacing the extension does not remove them. The default download directory is `maxminddb-mcp/databases` inside the OS user cache directory. No databases or MaxMind credentials are bundled. Obtain credentials from your [MaxMind account](https://www.maxmind.com/en/accounts/current/license-key).

Install the matching `.mcpb` from a newer release to upgrade. GitHub downloads do not enable automatic extension updates. Directory-listed extensions have a separate update mechanism. See [Claude's desktop extension documentation](https://support.claude.com/en/articles/10949351-getting-started-with-local-mcp-servers-on-claude-desktop).

### Standalone executable

Download an archive for your OS and architecture from [GitHub Releases](https://github.com/oschwald/maxminddb-mcp/releases), extract it, and place the executable on your PATH. Linux users can also install the `.deb`, `.rpm`, or `.apk` release package. Releases include SHA-256 hashes in `checksums.txt`.

Configure the executable below, then follow the [client instructions](#connect-a-client). The Go toolchain is only needed when building from source.

### Build from source

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

Claude Desktop extension users can skip this section. For a standalone installation, create `~/.config/maxminddb-mcp/config.toml` and choose one mode, or use [environment settings](#configuration-without-a-file).

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

When no databases are loaded, an initial download starts in the background. MCP requests can run during that download. Until it finishes, `list_databases` returns an empty list with `status: "downloading"`. It also reports download failures and corrective actions. Updates have no fixed total deadline. Downloads and lock waits honor caller cancellation or server shutdown.

When `auto_update` is true, the server checks configured and API-installed MaxMind editions in the background at startup, then on schedule. This also downloads missing editions when other databases are already installed. `update_databases` supports manual updates. Downloads use a unique temporary file and must pass checksum and MMDB validation before replacing the installed file. Missing or changed files are detected from their actual contents. Old `.checksums` files are ignored. Server processes sharing a database directory coordinate updates through per-edition `.mmdb.lock` files. Do not delete these lock files while a server is running. Other update programs must use separate directories or run when these servers are stopped.

### Use local MMDB files

```toml
mode = "directory"

[directory]
paths = ["/absolute/path/to/databases"]
```

The server loads MMDB files recursively and watches the directories present at startup. Restart the server after adding subdirectories. Database names are filenames, including `.mmdb`, and must be unique across configured directories. Duplicate names are rejected. On Windows, each loaded database is copied into memory so replacement does not invalidate active readers. Old copies remain until their readers are released. Loading an unchanged file reuses its reader. File identity, size, and modification time determine whether a reload is needed.

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

### Configuration without a file

The Desktop extension passes these environment settings to the executable. Other clients can use them too:

| Variable                         | Meaning                                                                                                                             |
| -------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `MAXMINDDB_MCP_DOWNLOAD_ENABLED` | `true` for MaxMind downloads, `false` for existing MMDB files                                                                       |
| `MAXMINDDB_MCP_DATABASE_DIR`     | One database folder. Required for existing files. Downloads default to the OS user cache directory under `maxminddb-mcp/databases`. |
| `MAXMINDDB_MCP_ACCOUNT_ID`       | Positive account ID, required for downloads                                                                                         |
| `MAXMINDDB_MCP_LICENSE_KEY`      | License key, required for downloads                                                                                                 |
| `MAXMINDDB_MCP_EDITIONS`         | Comma- or whitespace-separated editions. Defaults to `GeoLite2-City,GeoLite2-ASN`.                                                  |

Setting `MAXMINDDB_MCP_DOWNLOAD_ENABLED` selects this configuration source and skips automatic config-file discovery. Do not also set a nonempty `MAXMINDDB_MCP_CONFIG`. Download mode checks for updates every 24 hours. Local-file mode does not run MaxMind downloads, but explicit URL imports through `install_database` are available. Use a TOML file for multiple folders or other advanced settings.

### Config discovery and defaults

An explicit `MAXMINDDB_MCP_CONFIG` must point to an existing `.toml` file or a file named `GeoIP.conf`. A missing explicit file is an error. Otherwise, the server searches:

1. `~/.config/maxminddb-mcp/config.toml`
2. `~/.config/maxminddb-mcp/GeoIP.conf`
3. `/etc/GeoIP.conf`

The default database directory is `~/.cache/maxminddb-mcp/databases`. Database paths and `geoip_compat.config_path` beginning with `~/` are expanded. Duration defaults are `24h` for updates, `10m` for iterator idle expiry, and `1m` for cleanup. All durations must be positive, including when automatic updates are disabled. Unknown TOML settings cause a startup error that lists their names. Fix or remove those settings before starting the server.

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

After installing the executable on PATH and configuring it in a default location:

```bash
codex mcp add maxminddb -- maxminddb-mcp
codex mcp list
```

For an existing database folder, no server config file is needed:

```bash
codex mcp add maxminddb \
  --env MAXMINDDB_MCP_DOWNLOAD_ENABLED=false \
  --env MAXMINDDB_MCP_DATABASE_DIR=/absolute/path/to/databases \
  -- maxminddb-mcp
```

For an explicit executable and config file:

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

After installing the executable on PATH and configuring it in a default location:

```bash
claude mcp add --scope user --transport stdio maxminddb -- maxminddb-mcp
claude mcp list
```

For an explicit executable and config file:

```bash
claude mcp add --scope user \
  --env MAXMINDDB_MCP_CONFIG=/absolute/path/to/config.toml \
  --transport stdio maxminddb -- /absolute/path/to/maxminddb-mcp
```

See the [Claude Code MCP documentation](https://code.claude.com/docs/en/mcp).

### Claude Cowork plugin (experimental)

Releases also include a `_claude-plugin.zip` for each extension platform. It contains the same executable and settings as the MCPB. In Cowork, open **Customize → Plugins** and upload the matching ZIP as a custom plugin. Configure its settings before using the tools.

This package follows Claude's plugin format, but Cowork installation and its settings UI still need manual validation. Use the MCPB for Claude Desktop chat. A local MCP plugin does not provide tools to Claude web or mobile. See [Claude's plugin documentation](https://support.claude.com/en/articles/13837440-use-plugins-in-claude).

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

Successful results include structured JSON and a JSON text fallback. Tool execution failures set MCP `isError: true`. Schema validation rejects unknown arguments and invalid types. Read tools are annotated as local and read-only. Installation and update tools are annotated as network operations that can replace files.

### `lookup_ip`

Arguments: required `ip` (IPv4 or IPv6), optional `database` from `list_databases`.

```json
{
  "name": "lookup_ip",
  "arguments": { "ip": "1.0.0.1", "database": "GeoLite2-ASN.mmdb" }
}
```

A single-database result contains `ip`, `database`, `found`, `network`, and `data`. If no record matches, `found` is false and `network` and `data` are null. Without `database`, the result contains `ip` and a `databases` map keyed by database name. Each entry contains `found`, `network`, and `data`. If no database is available, an all-database lookup returns `no_databases`, as network lookup does. Check server logs for loading or download status.

### `lookup_ips`

Accepts `ips`, an array of 1 to 100 IP address strings, and optional `database`.
Returns a `results` array in input order, including duplicates. Each entry has
the same fields as `lookup_ip`, or `ip` and an `error` object. A missing record
is a successful result with `found: false`. Any failed address sets the batch's
MCP `isError` flag while preserving the other results. Cancellation stops the
batch and returns an error.

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

A page limit bounds the returned records, not the work needed to find matches. Use a narrow CIDR and cancel long scans through the client. Idle iterators expire after `iterator_ttl`. Each server retains at most 128 iterators, including completed queries until they expire. Expired entries are reclaimed before creating an iterator. At the limit, new queries and token resumes return `iterator_limit`. Continue with an existing `iterator_id` or wait for idle entries to expire. A failed or canceled first page releases its iterator immediately.

### `lookup_range`

Accepts required `start_ip` and `end_ip`, both inclusive, plus the same database,
filter, field-selection, and pagination arguments as `lookup_network`. Endpoints
must be ordered addresses of the same family without zone identifiers.
IPv4-mapped IPv6 endpoints are normalized to IPv4.

Returns each overlapping stored network once, even if that network overlaps
several CIDRs used to cover the range. A returned network can extend beyond the
endpoints. Results represent records, not individual addresses. Only the range
is scanned. Counts, cancellation, iterator limits, and token recovery follow
`lookup_network`. Repeat the endpoints on every page. A range cursor cannot be
used for a CIDR query or another range.

### Field selection and custom records

`lookup_ip`, `lookup_ips`, `lookup_network`, and `lookup_range` accept optional `fields`, with up
to 64 paths. Omit `fields` or use `[]` to return full records. Selected data is
an object keyed by the requested paths. Missing paths are omitted. For example,
`["country.iso_code"]` returns `{"country.iso_code":"US"}` in `data`.
Filters run against the full record before field selection. Repeat `fields` on
each page. Selection can change between pages without changing the query cursor.

Custom records can be objects, arrays, or scalar values. Field paths use dots
between map keys and zero-based array indexes, such as `subdivisions.0.iso_code`.
Use `$` to select or filter the whole record, including scalar records. Paths
cannot address a map key containing a dot or a literal root key named `$`.
Missing paths do not satisfy `exists`. A lookup with no matching record still
returns `found: false` and `data: null`, regardless of field selection.

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

Numeric equality and ordering compare decoded MMDB integers and JSON numbers without rounding integers to floating point. JSON clients can still lose precision for integers above 2⁵³ before sending them. Ordering also accepts numeric strings through floating-point conversion.

### `list_databases`

Accepts no arguments. Returns `databases`, sorted by filename. Each entry contains `name`, `type`, `description`, `last_updated`, and file `size` in bytes. Type, English description, and build time come from MMDB metadata. `last_updated` is the build time, not the file's modification time.

The result also includes `status` and a human-readable `message`. Status is `ready`, `downloading`, `empty`, or `error`. When downloads are configured, `update` reports `pending`, `updating`, `idle`, or `failed` and the most recent per-edition results. Existing databases remain available if an update fails. While the initial download is active, wait and check again instead of starting another update.

### `compare_databases`

Requires `old_database`, `new_database`, and `network`. Use an IPv4 /8 or smaller
network, or a native IPv6 /32 or smaller network. Optional `max_results` ranges
from 1 to 1000 and defaults to 100.

Returns `changes` with inclusive `start_ip` and `end_ip`, `change` (`added`,
`removed`, or `changed`), `before`, `after`, and corresponding `before_found` and
`after_found` flags. Equal JSON answers remain unchanged when stored network
boundaries or integer encodings differ. Ranges are clipped to the query.

Each call compares at most 10,000 address segments. Continue with the same
database names and network and the returned `resume_token` until `has_more` is
false. A page can contain no changes and still have more addresses to compare.
Page size can change. Reloading either database or restarting the server
invalidates the token. Cancellation returns an error without advancing a token.
Clients can request progress with `_meta.progressToken`.

### `summarize_network`

Requires `database`, `network`, and `group_by`. Supports `filters` and
`filter_mode`. Use an IPv4 /8 or smaller network, or a native IPv6 /32 or smaller
network. `group_by` uses the same paths as field selection and must select a
scalar value. Missing fields form a group with `missing: true`.

Each group has `value`, `missing`, `records`, and `addresses`. Record counts
count stored network records. Address counts include only addresses inside the
query, with IPv4 aliases skipped even when a query starts inside an alias.
Independent IPv6 records in custom databases are retained. Counts can differ when equal answers are stored
in networks of different sizes. Address counts are decimal strings to preserve
IPv6 precision. `total_addresses` is the size of the query. `matched_addresses`
is the coverage of matching records. Uncovered addresses do not form a group.

Summaries scan at most 10,000 records and return at most 100 groups. Group values
are limited to 1024 JSON bytes. Exceeding a limit returns an error, not partial
totals. Use a smaller network or select another field. Filters can reduce groups
but do not reduce the records scanned. Scans support cancellation and progress.

### `inspect_database`

Requires `database`. Optional `sample_size` ranges from 1 to 1000 and defaults
to 100. Returns all standard MMDB metadata and observed field paths, types, and
scalar examples. Field paths follow the same rules as filters and field selection.
The sample visits records in database order, so it is not statistically representative.
`schema_complete` is always false. Absence from a sample does not prove a field is absent.

Inspection returns at most 256 paths and visits at most 10,000 values to a depth
of 16. `fields_truncated` reports these limits or keys that field paths cannot
address. String examples are limited to 128 characters. Arrays and objects have
type information and child paths instead of complete example values.

### `verify_database`

Requires `database`. Runs the reader library's integrity checks on that loaded
reader and returns `valid: true` when they pass. Failures return
`verification_failed` with MCP `isError: true`. Downloaded databases continue to
be verified before installation.

Verification scans the full database and can take time. Only one tool request
verifies at a time. Cancellation can stop a queued request. The reader's `Verify`
method cannot stop once started, so the handler waits for it to finish. A successful
result reports `completed_after_cancellation` if cancellation was requested.
Failure messages also identify this case. Clients may discard results after
cancellation. The server still waits for verification to release its slot.

### `update_databases`

Available in `maxmind` and `geoip_compat` modes. Accepts no arguments. Updates configured editions and editions added through `install_edition`. Returns a `results` array with per-edition `database`, `updated`, `last_update`, and optional `size` or `error`. Any failed edition makes the tool result an MCP error while preserving all edition results.

### `install_edition`

Download a MaxMind MMDB edition using the configured account credentials, even if that edition is not in the server configuration. The account must have access to the edition. Available in `maxmind` and `geoip_compat` modes.

```json
{ "edition": "GeoIP2-Anonymous-IP" }
```

The server validates and loads the database before returning. No restart is needed. An existing file for that edition is updated. The result contains `database` (the edition ID), `updated`, `last_update`, and optional `size` or `error`.

New editions join `update_databases` and the automatic update schedule when `auto_update` is enabled. Empty marker files under `.maxminddb-mcp-editions` in the database folder preserve this selection across restarts. They contain no credentials. The server does not rewrite TOML, `GeoIP.conf`, or extension settings. If update registration fails after installation, the result includes an error and the installed database remains available. To stop updating an edition, stop the server, remove its marker, and remove the edition from configured editions if present. Removing the marker does not delete the MMDB.

The server does not list all editions available to an account. The [metadata API used by geoipupdate](https://github.com/maxmind/geoipupdate/blob/main/client/metadata.go) requires an edition ID. Live checks without an ID and with `edition_id=all` returned `EDITION_ID_REQUIRED` and `EDITION_ID_NOT_FOUND`. Find edition IDs in your MaxMind account's download list.

### `install_database`

Import a database from an HTTP or HTTPS URL in any mode:

```json
{
  "url": "https://example.com/network-data.mmdb.gz",
  "database": "network-data.mmdb"
}
```

`database` is a filename, not a path. The server installs into the configured MaxMind download folder, or the first configured folder in directory mode. The folder must be writable. Set `overwrite: true` to replace an existing file. A name already loaded from another folder is rejected.

Supported downloads are raw MMDB, gzip-compressed MMDB, and tar, tar.gz, or ZIP archives containing exactly one regular `.mmdb` file. Archive paths are ignored. Both the download and expanded data are limited to 1 GiB. Archives can contain up to 1,024 entries. The server verifies the MMDB before replacing an existing database and loads it immediately. Invalid or interrupted downloads leave the existing file intact. Cancellation stops network reads and lock waits. If verification has started, it must finish before cancellation is checked again.

URL imports are one-time downloads. The server does not store the URL or add it to automatic updates. Import again with `overwrite: true` to update it. If the destination name belongs to a configured or API-installed MaxMind edition, the next MaxMind update can replace it.

Requests can reach any HTTP(S) host accessible to the server, including the local network. MaxMind credentials are never attached to URL imports. URL usernames and passwords are rejected. Signed query URLs are accepted, but are not included in server errors or sent as redirect referrers. HTTPS downloads cannot redirect to HTTP.

The result contains `database` (the destination filename), `updated`, `last_update`, and optional `size` or `error`. A failed installation sets MCP `isError: true`. If replacement succeeds but loading fails, `updated` remains true and `error` explains the failure.

### Errors

Application errors include an `error` object with `code` and `message`. Common codes are `invalid_ip`, `invalid_network`, `invalid_filter`, `db_not_found`, `no_databases`, `database_required`, `iterator_expired`, `iterator_limit`, `query_mismatch`, and `resume_failed`. Input-schema errors use the SDK's error text.

## Protocol and development

For `2026-07-28` clients, `server/discover` and `tools/list` advertise a five-minute private cache lifetime. Tool-call results, including `list_databases`, do not receive these cache hints.

Initialization and discovery include instructions for choosing databases, filtering, pagination, and error recovery. Tools also provide readable display titles. Their names and arguments are unchanged.

Clients can request progress for network and range lookups, comparisons, summaries, verification, and database installation or updates by supplying a string or integer `_meta.progressToken`. Notifications report the current edition, MMDB bytes received, or records scanned. The `progress` counter counts status updates, not a percentage. Totals are omitted because the work is not known in advance. Notifications are limited to one per second per request and stop when the operation ends or is canceled. Automatic background downloads do not send progress notifications.

Stdio integration tests cover legacy initialization for `2024-11-05`, `2025-03-26`, `2025-06-18`, and `2025-11-25`, plus `2026-07-28` discovery and direct requests. They verify response metadata, invalid metadata, unsupported versions, progress, and cancellation of an active download. Protocol negotiation and transport behavior are provided by `mcp-go`. See the [MCP specification](https://modelcontextprotocol.io/specification/2026-07-28).

Use an MCP client for manual checks. A one-line pipe that closes stdin immediately can cancel a request before its response is written. Automated protocol tests are available with:

```bash
go test -race ./internal/mcp ./cmd/maxminddb-mcp
```

For development, release checks, and fixture generation, see [CONTRIBUTING.md](CONTRIBUTING.md). Report issues at [GitHub Issues](https://github.com/oschwald/maxminddb-mcp/issues).

## License

[ISC](LICENSE).
