# Configuration

[README](../README.md) · [Client setup](clients.md) · [Tool reference](tools.md)

- [Download from MaxMind](#download-from-maxmind)
- [Use local MMDB files](#use-local-mmdb-files)
- [Use an existing GeoIP.conf](#use-an-existing-geoipconf)
- [Environment settings](#configuration-without-a-file)
- [Config discovery and defaults](#config-discovery-and-defaults)
- [Logging](#logging)
- [Troubleshooting](#troubleshooting)
- [Database downloads and updates](#database-downloads-and-updates)
- [Database files and replacements](#database-files-and-replacements)

Claude Desktop extension users can configure the server through [extension settings](clients.md#claude-desktop). For a standalone installation, create `~/.config/maxminddb-mcp/config.toml` and choose one mode, or use [environment settings](#configuration-without-a-file).

## Download from MaxMind

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

## Use local MMDB files

```toml
mode = "directory"

[directory]
paths = ["/absolute/path/to/databases"]
```

## Use an existing GeoIP.conf

```toml
mode = "geoip_compat"

[geoip_compat]
config_path = "/etc/GeoIP.conf"
# Optional override of DatabaseDirectory from GeoIP.conf:
# database_dir = "/var/lib/GeoIP"
```

The server reads `AccountID`, `LicenseKey`, `EditionIDs`, `DatabaseDirectory`, and `Host` from the referenced file. `UserId` and `ProductIds` remain accepted aliases. A `Host` without a URL scheme uses HTTPS. Other geoipupdate options, including its proxy and retry settings, are not applied by this server.

If `config_path` is omitted, the server searches the user GeoIP.conf, `/etc/GeoIP.conf`, and `/usr/local/etc/GeoIP.conf`, in that order.

## Configuration without a file

The Desktop extension passes these environment settings to the executable. Other clients can use them too:

| Variable                         | Meaning                                                                                                                             |
| -------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `MAXMINDDB_MCP_DOWNLOAD_ENABLED` | `true` for MaxMind downloads, `false` for existing MMDB files                                                                       |
| `MAXMINDDB_MCP_DATABASE_DIR`     | One database folder. Required for existing files. Downloads default to the OS user cache directory under `maxminddb-mcp/databases`. |
| `MAXMINDDB_MCP_ACCOUNT_ID`       | Positive account ID, required for downloads                                                                                         |
| `MAXMINDDB_MCP_LICENSE_KEY`      | License key, required for downloads                                                                                                 |
| `MAXMINDDB_MCP_EDITIONS`         | Comma- or whitespace-separated editions. Defaults to `GeoLite2-City,GeoLite2-ASN`.                                                  |

Setting `MAXMINDDB_MCP_DOWNLOAD_ENABLED` selects this configuration source and skips automatic config-file discovery. Do not also set a nonempty `MAXMINDDB_MCP_CONFIG`. Download mode checks for updates every 24 hours. Local-file mode does not run MaxMind downloads, but explicit URL imports through `install_database` are available. Use a TOML file for multiple folders or other advanced settings.

## Config discovery and defaults

An explicit `MAXMINDDB_MCP_CONFIG` must point to an existing `.toml` file or a file named `GeoIP.conf`. A missing explicit file is an error. Otherwise, the server searches:

1. `~/.config/maxminddb-mcp/config.toml`
2. `~/.config/maxminddb-mcp/GeoIP.conf`
3. `/etc/GeoIP.conf`

The default database directory is `~/.cache/maxminddb-mcp/databases`. Database paths and `geoip_compat.config_path` beginning with `~/` are expanded. Duration defaults are `24h` for updates, `10m` for iterator idle expiry, and `1m` for cleanup. All durations must be positive, including when automatic updates are disabled. Unknown TOML settings cause a startup error that lists their names. Fix or remove those settings before starting the server.

## Logging

Logging uses environment variables:

| Variable                   | Values                                     |
| -------------------------- | ------------------------------------------ |
| `MAXMINDDB_MCP_CONFIG`     | Config file path                           |
| `MAXMINDDB_MCP_LOG_LEVEL`  | `debug`, `info` (default), `warn`, `error` |
| `MAXMINDDB_MCP_LOG_FORMAT` | `text` (default), `json`                   |

TOML `log_level` and `log_format` settings are not supported. Logs go to stderr. Stdout carries only MCP messages.

## Troubleshooting

| Symptom                           | What to check                                                                                                                                                                     |
| --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| No tools appear                   | Check the [client setup](clients.md), executable path, and plugin enablement. Docker plugins require a running Docker engine. Restart the server after correcting its settings.   |
| Databases are downloading         | Ask the assistant to call `list_databases` for status. Wait for the initial download instead of starting another update.                                                          |
| No databases are available        | Check the selected folder and `.mmdb` files. For downloads, check the account ID, license key, edition access, and folder write permissions.                                      |
| Startup fails with a config error | Check the explicit config path, remove unknown TOML settings, and use positive durations. Do not combine a nonempty `MAXMINDDB_MCP_CONFIG` with `MAXMINDDB_MCP_DOWNLOAD_ENABLED`. |
| A database update fails           | Read the per-edition error from `list_databases`, correct the settings, then request `update_databases`. Existing databases remain usable.                                        |

Enter credentials in client settings or a local config file, not in chat. See [logging](#logging) for startup diagnostics and [tool errors](tools.md#errors) for API error codes.

## Database downloads and updates

When no databases are loaded, an initial download starts in the background. MCP requests can run during that download. Until it finishes, `list_databases` returns an empty list with `status: "downloading"`. It also reports download failures and corrective actions. Updates have no fixed total deadline. Downloads and lock waits honor caller cancellation or server shutdown.

When `auto_update` is true, the server checks configured and API-installed MaxMind editions in the background at startup, then on schedule. This also downloads missing editions when other databases are already installed. `update_databases` supports manual updates. Downloads use a unique temporary file and must pass checksum and MMDB validation before replacing the installed file. Missing or changed files are detected from their actual contents. Old `.checksums` files are ignored. Server processes sharing a database directory coordinate updates through per-edition `.mmdb.lock` files. Do not delete these lock files while a server is running. Other update programs must use separate directories or run when these servers are stopped.

## Database files and replacements

The server loads MMDB files recursively and watches the directories present at startup. Restart the server after adding subdirectories. Database names are filenames, including `.mmdb`, and must be unique across configured directories. Duplicate names are rejected. On Windows, each loaded database is copied into memory so replacement does not invalidate active readers. Old copies remain until their readers are released. Loading an unchanged file reuses its reader. File identity, size, and modification time determine whether a reload is needed.

Replace databases by writing a separate file and renaming it into place. Active iterators retain their original reader. In-place writes can invalidate a memory-mapped file.
