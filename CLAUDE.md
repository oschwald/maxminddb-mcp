# Development guide

Read [CONTRIBUTING.md](CONTRIBUTING.md) for setup, pinned tool versions, architecture, tests, and release checks. Read [README.md](README.md) for configuration and tool contracts.

## Commands

```bash
git submodule update --init --recursive
npm ci
go build -o bin/maxminddb-mcp ./cmd/maxminddb-mcp
go test -race -cover ./...
mise exec golangci-lint@2.15.0 -- golangci-lint fmt
mise exec golangci-lint@2.15.0 -- golangci-lint run ./...
npm run format:check
npm run test:extensions
```

## Constraints

- Preserve reviewed commits. Make follow-up changes in new atomic commits.
- Keep credentials out of commits and logs. Config examples must use placeholders.
- Extension settings use the environment configuration source. Do not silently merge them with discovered TOML or GeoIP.conf files.
- Keep downloaded databases outside the extension directory. Package only staged binaries, manifests, documentation, the setup skill, and the icon.
- Share `packaging/skills/setup/SKILL.md` across Claude and OpenAI plugins. OpenAI packages use config discovery and must not contain Claude's settings placeholders.
- Changes to packaging must pass the extracted-bundle smoke test. Follow CONTRIBUTING.md for release automation and manual Desktop checks.
- Send progress only for an active request with a valid progress token. Keep callbacks synchronous, throttle notifications, and omit unknown totals.
- Stdout is for MCP messages. Use `log/slog` on stderr for application logs.
- Validate tool arguments and resume tokens. Errors must set MCP `isError`.
- Decode MMDB records into `any`. Use `record.Lookup` for map paths, array indexes, and `$`. Apply filters before selecting output fields.
- Hold the per-edition file lock from the installed-file check through installation and reload. Keep lock files on disk.
- Keep shared MMDB readers open while active iterators can reference them.
- Comparison tokens bind to reader generations from `Snapshot`. Reloads and server restarts invalidate them. Compare JSON answers, not stored network boundaries.
- Range queries cover the exact endpoints with disjoint CIDRs. Suppress repeated containing records and bind both endpoints to live and resumed cursors.
- Apply the iterator limit to both new queries and token resumes. Release new iterators if their first page fails.
- Hold the iterator lock for a complete page. Cancellation must not lose a partial page.
- Use bundled fixtures and local HTTP servers in tests. Do not depend on home-directory databases or live MaxMind downloads.
- `testdata` is an upstream submodule. Its generator uses a pinned `mmdbwriter/v2` prerelease. Commit only submodule pointers that collaborators can fetch.
- Update public docs and the changelog when behavior changes. Do not add untested client configuration examples.
