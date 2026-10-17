# Tool reference

[README](../README.md) · [Client setup](clients.md) · [Configuration](configuration.md)

- Lookups: [`lookup_ip`](#lookup_ip), [`lookup_ips`](#lookup_ips), [`lookup_network`](#lookup_network), [`lookup_range`](#lookup_range)
- Queries: [Field selection](#field-selection-and-custom-records), [filters](#filters), [`compare_databases`](#compare_databases), [`summarize_network`](#summarize_network)
- Inspection: [`list_databases`](#list_databases), [`inspect_database`](#inspect_database), [`verify_database`](#verify_database)
- Downloads: [`update_databases`](#update_databases), [`install_edition`](#install_edition), [`install_database`](#install_database)
- [Errors](#errors) and [protocol behavior](#protocol-and-development)

Successful results include structured JSON and a JSON text fallback. Tool execution failures set MCP `isError: true`. Schema validation rejects unknown arguments and invalid types. Read tools are annotated as local and read-only. Installation and update tools are annotated as network operations that can replace files.

## `lookup_ip`

Arguments: required `ip` (IPv4 or IPv6), optional `database` from `list_databases`.

```json
{
  "name": "lookup_ip",
  "arguments": { "ip": "1.0.0.1", "database": "GeoLite2-ASN.mmdb" }
}
```

A single-database result contains `ip`, `database`, `found`, `network`, and `data`. If no record matches, `found` is false and `network` and `data` are null. Without `database`, the result contains `ip` and a `databases` map keyed by database name. Each entry contains `found`, `network`, and `data`. If no database is available, an all-database lookup returns `no_databases`, as network lookup does. Check server logs for loading or download status.

## `lookup_ips`

Accepts `ips`, an array of 1 to 100 IP address strings, and optional `database`.
Returns a `results` array in input order, including duplicates. Each entry has
the same fields as `lookup_ip`, or `ip` and an `error` object. A missing record
is a successful result with `found: false`. Any failed address sets the batch's
MCP `isError` flag while preserving the other results. Cancellation stops the
batch and returns an error.

## `lookup_network`

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

## `lookup_range`

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

## Field selection and custom records

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

## Filters

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

## `list_databases`

Accepts no arguments. Returns `databases`, sorted by filename. Each entry contains `name`, `type`, `description`, `last_updated`, and file `size` in bytes. Type, English description, and build time come from MMDB metadata. `last_updated` is the build time, not the file's modification time.

The result also includes `status` and a human-readable `message`. Status is `ready`, `downloading`, `empty`, or `error`. When downloads are configured, `update` reports `pending`, `updating`, `idle`, or `failed` and the most recent per-edition results. Existing databases remain available if an update fails. While the initial download is active, wait and check again instead of starting another update.

## `compare_databases`

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

## `summarize_network`

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

## `inspect_database`

Requires `database`. Optional `sample_size` ranges from 1 to 1000 and defaults
to 100. Returns all standard MMDB metadata and observed field paths, types, and
scalar examples. Field paths follow the same rules as filters and field selection.
The sample visits records in database order, so it is not statistically representative.
`schema_complete` is always false. Absence from a sample does not prove a field is absent.

Inspection returns at most 256 paths and visits at most 10,000 values to a depth
of 16. `fields_truncated` reports these limits or keys that field paths cannot
address. String examples are limited to 128 characters. Arrays and objects have
type information and child paths instead of complete example values.

## `verify_database`

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

## `update_databases`

Available in `maxmind` and `geoip_compat` modes. Accepts no arguments. Updates configured editions and editions added through `install_edition`. Returns a `results` array with per-edition `database`, `updated`, `last_update`, and optional `size` or `error`. Any failed edition makes the tool result an MCP error while preserving all edition results.

## `install_edition`

Download a MaxMind MMDB edition using the configured account credentials, even if that edition is not in the server configuration. The account must have access to the edition. Available in `maxmind` and `geoip_compat` modes.

```json
{ "edition": "GeoIP2-Anonymous-IP" }
```

The server validates and loads the database before returning. No restart is needed. An existing file for that edition is updated. The result contains `database` (the edition ID), `updated`, `last_update`, and optional `size` or `error`.

New editions join `update_databases` and the automatic update schedule when `auto_update` is enabled. Empty marker files under `.maxminddb-mcp-editions` in the database folder preserve this selection across restarts. They contain no credentials. The server does not rewrite TOML, `GeoIP.conf`, or extension settings. If update registration fails after installation, the result includes an error and the installed database remains available. To stop updating an edition, stop the server, remove its marker, and remove the edition from configured editions if present. Removing the marker does not delete the MMDB.

The server does not list all editions available to an account. The [metadata API used by geoipupdate](https://github.com/maxmind/geoipupdate/blob/main/client/metadata.go) requires an edition ID. Live checks without an ID and with `edition_id=all` returned `EDITION_ID_REQUIRED` and `EDITION_ID_NOT_FOUND`. Find edition IDs in your MaxMind account's download list.

## `install_database`

Import a database from an HTTP or HTTPS URL in any mode:

```json
{
  "url": "https://example.com/network-data.mmdb.gz",
  "database": "network-data.mmdb"
}
```

`database` is a filename, not a path. The server installs into the configured MaxMind download folder, or the first configured folder in directory mode. The folder must be writable. Set `overwrite: true` to replace an existing file. A name already loaded from another folder is rejected.

Supported downloads are raw MMDB, gzip-compressed MMDB, and tar, tar.gz, or ZIP archives containing exactly one regular `.mmdb` file. Archive paths are ignored. Both the download and expanded data are limited to 1 GiB, including HTTP bodies before and after gzip decoding. Archives can contain up to 1,024 entries. ZIP directory counts and offsets are checked before allocating entry metadata. The server verifies the MMDB before replacing an existing database and loads it immediately. Invalid or interrupted downloads leave the existing file intact. Cancellation stops network reads and lock waits. If verification has started, it must finish before cancellation is checked again.

URL imports are one-time downloads. The server does not store the URL or add it to automatic updates. Import again with `overwrite: true` to update it. If the destination name belongs to a configured or API-installed MaxMind edition, the next MaxMind update can replace it.

Requests can reach any HTTP(S) host accessible to the server, including the local network. MaxMind credentials are never attached to URL imports. URL usernames and passwords are rejected. Signed query URLs are accepted, but are not included in server errors or sent as redirect referrers. HTTPS downloads cannot redirect to HTTP.

The result contains `database` (the destination filename), `updated`, `last_update`, and optional `size` or `error`. A failed installation sets MCP `isError: true`. If replacement succeeds but loading fails, `updated` remains true and `error` explains the failure.

## Errors

Application errors include an `error` object with `code` and `message`. Common codes are `invalid_ip`, `invalid_network`, `invalid_filter`, `db_not_found`, `no_databases`, `database_required`, `iterator_expired`, `iterator_limit`, `query_mismatch`, and `resume_failed`. Input-schema errors use the SDK's error text.

## Protocol and development

For `2026-07-28` clients, `server/discover` and `tools/list` advertise a five-minute private cache lifetime. Tool-call results, including `list_databases`, do not receive these cache hints.

Initialization and discovery include instructions for choosing databases, filtering, pagination, and error recovery. Tools also provide readable display titles. Their names and arguments are unchanged.

Clients can request progress for network and range lookups, comparisons, summaries, verification, and database installation or updates by supplying a string or integer `_meta.progressToken`. Notifications report the current edition, MMDB bytes received, or records scanned. The `progress` counter counts status updates, not a percentage. Totals are omitted because the work is not known in advance. Notifications are limited to one per second per request and stop when the operation ends or is canceled. Automatic background downloads do not send progress notifications.

For supported protocol revisions and testing, see [Contributing](../CONTRIBUTING.md#checks).
