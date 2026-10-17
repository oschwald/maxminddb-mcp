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
npm run test:extensions
mise exec goreleaser@2.18.3 -- goreleaser check
```

Run fixture generator tests from the submodule:

```bash
cd testdata
go test ./...
```

The query tool fixtures in `internal/mcp/testdata` cover custom record types and
changes between database builds. Regenerate them with the pinned writer from
the submodule, then commit the generator and the MMDB files:

```bash
cd testdata
go run ../internal/mcp/testdata/generate.go ../internal/mcp/testdata
```

Tests use bundled MMDB fixtures and local HTTP servers. They do not require a MaxMind account or databases in your home directory. Add regression tests for pagination, filters, config loading, and download failures when changing those paths. Prefer assertions about returned data over tests that only check for a non-nil result. Aim for at least 80% coverage in changed packages, while prioritizing meaningful cases.

CI runs the server tests with the race detector on Linux, Windows, and macOS. These include file-watcher replacement and removal, retained readers, and stdio shutdown. On each OS, the extension test builds, packs, and extracts a native MCPB, applies settings through the MCPB configuration resolver, and performs an IP lookup over stdio. It also checks external database retention and credential-safe configuration errors. Fixture generator tests and vulnerability checks run on Linux.

The stdio tests cover all five supported protocol revisions, discovery-free modern requests, response identity and result types, invalid metadata, unsupported versions, progress, and cancellation notifications. To run them with the startup and shutdown tests:

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

### Automated publishing

Push a new `vMAJOR.MINOR.PATCH` tag from a reviewed commit to start release CI. GoReleaser builds binaries, containers, Linux packages, MCPB extensions, and experimental Claude plugin ZIPs. It publishes the files and their SHA-256 checksums to GitHub Releases. Packaging uses the pinned `@anthropic-ai/mcpb` development dependency. Node.js is not included in the extensions and is not needed to run the server.

Each OS and architecture gets its own extension. The MCPB manifest declares the OS, but the format does not yet provide a portable CPU-architecture selector. File names and display names identify the architecture. See [the upstream architecture issue](https://github.com/modelcontextprotocol/mcpb/issues/10).

After a stable release succeeds, CI updates the version in `server.json` and publishes to the MCP Registry as `io.github.oschwald/maxminddb-mcp`. Authentication uses GitHub Actions OIDC with `id-token: write`, so no registry token is stored. The registry entry uses the existing multi-architecture OCI image and a named Docker volume for downloaded databases. It requires Docker and MaxMind credentials. Native MCPB downloads remain available on GitHub. This avoids relying on registry clients to select an MCPB architecture.

Prerelease tags publish GitHub assets but skip the registry and Homebrew. Validate registry metadata without publishing:

```bash
go run github.com/modelcontextprotocol/registry/cmd/publisher@v1.8.1 validate server.json
```

For automatic Homebrew publishing, create `oschwald/homebrew-tap` and add the repository secret `HOMEBREW_TAP_TOKEN`. Use a token with Contents write access to that tap. GoReleaser then updates the macOS cask on stable releases. Without the secret, it skips the tap update and still publishes the other artifacts. After the first successful tap publication, users can install with:

```bash
brew install --cask oschwald/tap/maxminddb-mcp
```

The tap does not exist yet. Do not advertise the command as available until a release has populated it.

Anthropic's extension directory is a separate submission and review process. CI publishes downloadable bundles, not an approved directory listing. These bundles are unsigned. Verify the downloaded bundle's execution and any OS security prompts on a clean desktop before submitting it. Directory approval or publisher certificates cannot be created by this repository's release workflow.

### Desktop acceptance checks

Automated package tests do not exercise Claude Desktop's UI. Before marking Desktop or Cowork support as manually verified, test the matching artifact on each advertised platform:

1. Install from a downloaded release with no Go, Node.js, or server config file present.
2. Enter MaxMind credentials and confirm an initial download, a successful lookup, and a useful error for invalid credentials.
3. Turn downloads off and select existing MMDB files. Check paths with spaces and a folder that cannot be written.
4. Change settings, restart the extension, and install an upgrade. Confirm that settings and external databases remain available.
5. For the experimental plugin, upload the ZIP in Cowork and verify that its settings UI and local MCP process work. Do not infer Cowork support from a Desktop chat test.

For a generated plugin's schema, run `claude plugin validate --strict dist/extensions/PLATFORM_ARCH` with Claude Code 2.1.302 or later. The plugin and MCPB use the same executable and settings definitions. The extension defaults to an OS cache directory outside the installation folder. Users who need a different storage location can choose a database folder.

## Code and behavior

| Directory           | Responsibility                                      |
| ------------------- | --------------------------------------------------- |
| `cmd/maxminddb-mcp` | Process lifecycle, config loading, stdio startup    |
| `internal/config`   | TOML, GeoIP.conf, and extension settings validation |
| `internal/database` | Readers, file watching, atomic downloads            |
| `internal/filter`   | Operators and field comparisons                     |
| `internal/iterator` | Page cursors, cancellation, resume tokens, expiry   |
| `internal/mcp`      | Tool schemas, handlers, protocol tests              |
| `test`              | Integration and performance tests                   |
| `testdata`          | Upstream fixture submodule and generator            |
| `packaging`         | Desktop extension manifest and icon                 |
| `scripts`           | Extension packaging and installation smoke test     |

Use `golangci-lint fmt` for Go formatting and `npm run format` for Markdown. The lint configuration enables an explicit set of checks. Use `MaxMind`, `GeoIP`, and `MMDB` in prose. JSON and TOML field names use `snake_case`.

Wrap Go errors with `%w` and use `errors.Is` or `errors.As`. MCP tool failures must set `isError`. Keep logs on stderr through `log/slog`.

Readers retained by an iterator must remain open. The manager relies on the reader's garbage collection cleanup after references are released. Close temporary readers explicitly when they have not been shared. Database names must be unique, and metadata must come from the MMDB.

A page operation holds the iterator lock for its full duration. Stop pull cursors on expiry and shutdown. Resume tokens bind to the database name, type, and build epoch. Validate token contents and continuation parameters before using them. Add each new filter operator to `SupportedOperators`, evaluation, validation, and tests.

## Changes and review

Keep commits focused. Preserve existing commits after human review and add follow-up commits. Update README.md for public behavior, CLAUDE.md for development guidance, and CHANGELOG.md for release notes.

Before proposing a change, run the checks above and inspect the diff for generated files or credentials. For bug reports, include the OS, Go version, MCP client, sanitized config, reproduction steps, and relevant stderr logs. Report security issues privately to the maintainer.

Contributions are licensed under the [ISC license](LICENSE).
