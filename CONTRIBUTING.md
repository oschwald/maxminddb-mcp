# Contributing

## Setup

Clone with fixtures, or initialize them in an existing checkout:

```bash
git submodule update --init --recursive
go mod download
npm ci
```

Use Go 1.27.2 for development and release builds. The runtime module requires Go 1.26. The fixture module requires Go 1.27 and uses a pinned `mmdbwriter/v2` prerelease.

CI uses Node 24.22.0, Prettier 3.9.10, golangci-lint 2.15.0, and GoReleaser 2.18.3. Prettier is pinned in `package-lock.json`. Install the Go tools at those versions, or run them through [mise](https://mise.jdx.dev/):

```bash
mise install golangci-lint@2.15.0 goreleaser@2.18.3
mise exec golangci-lint@2.15.0 -- golangci-lint fmt
mise exec golangci-lint@2.15.0 -- golangci-lint run ./...
```

## Checks

```bash
go test -race -cover ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
npm run format:check
mise exec goreleaser@2.18.3 -- goreleaser check
```

Run fixture generator tests from the submodule:

```bash
cd testdata
go test ./...
```

Tests use bundled MMDB fixtures and local HTTP servers. They do not require a MaxMind account or databases in your home directory. Add regression tests for pagination, filters, config loading, and download failures when changing those paths. Prefer assertions about returned data over tests that only check for a non-nil result. Aim for at least 80% coverage in changed packages, while prioritizing meaningful cases.

CI runs the server tests with the race detector on Linux, Windows, and macOS. These include file-watcher replacement and removal, retained readers, and stdio shutdown. Fixture generator tests and vulnerability checks run on Linux.

The stdio tests exercise the legacy initialize handshake and current per-request protocol metadata. To run them with the startup and shutdown tests:

```bash
go test -race ./internal/mcp ./cmd/maxminddb-mcp
```

## Build and release checks

```bash
go build -o bin/maxminddb-mcp ./cmd/maxminddb-mcp
./bin/maxminddb-mcp --help
./bin/maxminddb-mcp --version
mise exec goreleaser@2.18.3 -- goreleaser build --snapshot --clean
```

The Dockerfile consumes a prebuilt Linux binary named `maxminddb-mcp` under `linux/amd64/` or `linux/arm64/` in its build context. Build in a temporary directory to avoid sending config files or credentials to Docker:

```bash
build_dir=$(mktemp -d)
mkdir -p "$build_dir/linux/amd64"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$build_dir/linux/amd64/maxminddb-mcp" ./cmd/maxminddb-mcp
cp Dockerfile "$build_dir/Dockerfile"
docker build --platform linux/amd64 --build-arg TARGETPLATFORM=linux/amd64 -t maxminddb-mcp:dev "$build_dir"
```

GoReleaser uses `dockers_v2` and Docker Buildx to build the multi-platform image and retain the `-amd64` and `-arm64` tags. Snapshot builds load images locally without publishing and add a platform suffix to each tag. They do not update `latest` tags. To test release packaging and images:

```bash
mise exec goreleaser@2.18.3 -- goreleaser release --snapshot --clean
```

Building ARM64 images on an AMD64 host requires an ARM64 builder or QEMU emulation. To check binary packaging without images, add `--skip=docker`. See [GoReleaser's Docker documentation](https://goreleaser.com/customization/package/dockers_v2/).

Release CI runs only on version tags. Local build and config checks do not publish a release.

## Code and behavior

| Directory           | Responsibility                                    |
| ------------------- | ------------------------------------------------- |
| `cmd/maxminddb-mcp` | Process lifecycle, config loading, stdio startup  |
| `internal/config`   | TOML and GeoIP.conf loading, validation           |
| `internal/database` | Readers, file watching, atomic downloads          |
| `internal/filter`   | Operators and field comparisons                   |
| `internal/iterator` | Page cursors, cancellation, resume tokens, expiry |
| `internal/mcp`      | Tool schemas, handlers, protocol tests            |
| `test`              | Integration and performance tests                 |
| `testdata`          | Upstream fixture submodule and generator          |

Use `golangci-lint fmt` for Go formatting and `npm run format` for Markdown. The lint configuration enables an explicit set of checks. Use `MaxMind`, `GeoIP`, and `MMDB` in prose. JSON and TOML field names use `snake_case`.

Wrap Go errors with `%w` and use `errors.Is` or `errors.As`. MCP tool failures must set `isError`. Keep logs on stderr through `log/slog`.

Readers retained by an iterator must remain open. The manager relies on the reader's garbage collection cleanup after references are released. Close temporary readers explicitly when they have not been shared. Database names must be unique, and metadata must come from the MMDB.

A page operation holds the iterator lock for its full duration. Stop pull cursors on expiry and shutdown. Resume tokens bind to the database name, type, and build epoch. Validate token contents and continuation parameters before using them. Add each new filter operator to `SupportedOperators`, evaluation, validation, and tests.

## Changes and review

Keep commits focused. Preserve existing commits after human review and add follow-up commits. Update README.md for public behavior, CLAUDE.md for development guidance, and CHANGELOG.md for release notes.

Before proposing a change, run the checks above and inspect the diff for generated files or credentials. For bug reports, include the OS, Go version, MCP client, sanitized config, reproduction steps, and relevant stderr logs. Report security issues privately to the maintainer.

Contributions are licensed under the [ISC license](LICENSE).
