# Client setup

[README](../README.md) · [Configuration](configuration.md) · [Tool reference](tools.md)

Choose your client below. Claude Desktop's MCPB includes the executable and a settings form. Other clients use a standalone executable, a bundled plugin, or Docker as described in each section.

- [Claude Desktop](#claude-desktop)
- [Standalone executable](#standalone-executable)
- [ChatGPT desktop](#chatgpt-desktop) and [experimental plugin](#chatgpt-desktop-plugin-experimental)
- [Codex](#codex)
- [Claude Code](#claude-code) and [experimental Cowork plugin](#claude-cowork-plugin-experimental)
- [Antigravity CLI](#antigravity-cli-agy)
- [Gemini CLI (legacy)](#gemini-cli-legacy)
- [Cursor plugin](#cursor-plugin)
- [Zed](#zed)
- [Docker](#docker)

## Claude Desktop

Install and configure the server through Claude Desktop. You do not need Go, a terminal, or a config file.

1. [Download the latest MCPB](https://github.com/oschwald/maxminddb-mcp/releases/latest/download/maxminddb.mcpb). This single file includes native executables for Mac Intel and Apple silicon, Linux x64 and ARM64, and Windows x64. It selects the executable when it starts. No separate runtime is required. Smaller packages for individual platforms remain available on [GitHub Releases](https://github.com/oschwald/maxminddb-mcp/releases).
2. In Claude Desktop, open **Settings → Extensions → Advanced settings → Install Extension** and select the file.
3. Configure one of these choices in the extension settings:
   - **Download databases:** Leave **Download and update MaxMind databases** enabled. Enter your MaxMind account ID and license key. The default editions are `GeoLite2-City` and `GeoLite2-ASN`. Change these for other editions available to your account. Leave **Database folder** blank to use the application cache, or select a writable folder.
   - **Use existing files:** Turn off downloads and select the **Database folder** containing your MMDB files. Account settings are ignored in this mode.
4. Enable the extension and ask: "Using MaxMindDB, list my databases and look up 8.8.8.8."

The initial download runs in the background. Claude can check its status with `list_databases`. You can change the settings later in the same Extensions panel. Restart the extension after changing settings if Claude has not restarted it automatically.

### Settings and storage

Claude stores the license key as a sensitive setting. Downloaded files stay outside the extension, so replacing the extension does not remove them. The default download directory is `maxminddb-mcp/databases` inside the OS user cache directory. No databases or MaxMind credentials are bundled. Obtain credentials from your [MaxMind account](https://www.maxmind.com/en/accounts/current/license-key).

### Upgrading

Download and install the latest MCPB again to upgrade. The link always follows the latest release. If upgrading from v0.2.0 or earlier, remove the old `maxminddb-mcp` extension and configure the new `maxminddb` extension. The identifier changed, so saved extension settings do not carry over. Database files remain in the same folder. GitHub downloads do not enable automatic extension updates. Directory-listed extensions have a separate update mechanism. See [Claude's desktop extension documentation](https://support.claude.com/en/articles/10949351-getting-started-with-local-mcp-servers-on-claude-desktop).

## Standalone executable

Download an archive for your OS and architecture from [GitHub Releases](https://github.com/oschwald/maxminddb-mcp/releases), extract it, and place the executable on your PATH. Linux users can also install the `.deb`, `.rpm`, or `.apk` release package. Releases include SHA-256 hashes in `checksums.txt`.

Choose a [configuration](configuration.md), then follow your client instructions below. The Go toolchain is only needed when building from source.

On macOS, you can also install with Homebrew:

```bash
brew install --cask oschwald/tap/maxminddb-mcp
```

To install from source, see [Build from source](../CONTRIBUTING.md#build-from-source).

## Connect a client

This is a local **stdio** server. Configure your client to launch the installed binary, using an absolute path if it is not in the client's PATH. The server does not expose an HTTP endpoint.

## ChatGPT desktop

Use a local thread in the ChatGPT desktop app. After installing the [standalone executable](#standalone-executable), configure it in a [default location](configuration.md#config-discovery-and-defaults). If you already have `/etc/GeoIP.conf`, the server discovers it without a new config file. The database directory must be writable if downloads are enabled.

1. Open **Settings → MCP servers → Add server**.
2. Name the server `maxminddb`, select **STDIO**, and enter the executable's absolute path as the command. No arguments are needed.
3. Save, then select **Restart**. Open a new local conversation.
4. Ask: "Using MaxMindDB, list my databases and look up 8.8.8.8."

ChatGPT desktop and Codex share MCP configuration on the same host. If you already added the server through Codex, you do not need a second entry. Direct stdio entries accept environment variables, including our [environment settings](configuration.md#configuration-without-a-file). The [Codex examples](#codex) show how to set them without a server config file.

These instructions apply to local MCP connections. ChatGPT web and cloud threads do not read the local configuration. See the [official OpenAI MCP documentation](https://learn.chatgpt.com/docs/extend/mcp).

## ChatGPT desktop plugin (experimental)

Releases include `_openai-plugin.zip` packages for the same OS and architecture combinations as the MCPB extensions. Each ZIP contains the compiled server, an OpenAI plugin manifest, a local marketplace catalog, and a setup skill. No Go or Node.js runtime is required. Use this package as an alternative to the direct MCP entry above.

The plugin uses [existing server configuration](configuration.md#config-discovery-and-defaults). Keep credentials and databases outside the extracted package. OpenAI plugins do not support Claude's `userConfig` form or `${user_config.*}` substitutions, so this package does not prompt for account settings. See [OpenAI's migration guidance](https://developers.openai.com/plugins/guides/submit-claude-plugin#replace-claude-userconfig).

This local marketplace workflow requires Codex CLI to register the extracted package. Extracting the ZIP alone does not make the plugin discoverable. Without Codex CLI, use the [direct ChatGPT desktop setup](#chatgpt-desktop) above.

1. Download the matching `_openai-plugin.zip` from [GitHub Releases](https://github.com/oschwald/maxminddb-mcp/releases) and extract it to a permanent folder.
2. Register the included marketplace with Codex CLI:

   ```bash
   codex plugin marketplace add /absolute/path/to/extracted-package
   ```

3. Restart ChatGPT desktop. In **Plugins**, select the **MaxMindDB** marketplace for your platform and install the plugin.
4. Run the plugin's setup workflow in a local thread. It checks `list_databases` and performs a sample IP lookup. It reports missing configuration and initial downloads without asking for credentials in chat.

For upgrades, uninstall the local plugin, replace the extracted package with the newer release for the same platform, then install it again. External configuration and databases remain in place. Local marketplace registration and desktop UI behavior still need manual validation. See [OpenAI's local marketplace instructions](https://developers.openai.com/plugins/build/plugins).

Release CI publishes the ZIPs and checksums to GitHub. They are not public directory listings. OpenAI currently requires approved-partner access for local MCP submissions. See [desktop plugin submission requirements](https://developers.openai.com/plugins/deploy/desktop-only).

## Codex

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

## Claude Code

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

For a repository plugin backed by Docker, install from this marketplace in Claude Code:

```text
/plugin install maxminddb --marketplace oschwald/maxminddb-mcp
```

This requires a running Docker engine. Enter your account ID, license key, and edition IDs in the plugin settings. The repository plugin downloads databases into the `maxminddb-mcp-data` Docker volume. Use the native MCPB or release plugin to select an existing MMDB folder.

## Claude Cowork plugin (experimental)

Releases also include a `_claude-plugin.zip` for each extension platform. It contains the same executable and settings as the MCPB. In Cowork, open **Customize → Plugins** and upload the matching ZIP as a custom plugin. Configure its settings before using the tools.

The plugin includes a `setup` skill to check database status and perform a sample IP lookup. Ask it to check your MaxMindDB setup. In Claude Code, invoke `/maxminddb:setup`. The skill directs credential entry to client settings. It does not collect keys in chat. The MCPB extension does not register plugin skills by itself.

This package follows Claude's plugin format, but Cowork installation and its settings UI still need manual validation. Use the MCPB for Claude Desktop chat. A local MCP plugin does not provide tools to Claude web or mobile. See [Claude's plugin documentation](https://support.claude.com/en/articles/13837440-use-plugins-in-claude).

## Antigravity CLI (`agy`)

After installing `maxminddb-mcp` on PATH and configuring it in a [default location](configuration.md#config-discovery-and-defaults), install the repository plugin:

```bash
agy plugin install https://github.com/oschwald/maxminddb-mcp
agy plugin list
```

The plugin includes the MCP connection and setup skill. It uses the installed executable and its existing configuration. In Antigravity, ask it to check your MaxMindDB setup and look up `8.8.8.8`. For a direct MCP connection without the skill, run `agy mcp add maxminddb maxminddb-mcp` instead.

Google replaced Gemini CLI for individual accounts with Antigravity CLI on June 18, 2026. Enterprise and API-key users can still use Gemini CLI. See [Google's transition announcement](https://github.com/google-gemini/gemini-cli/discussions/28017) and [Antigravity's plugin documentation](https://antigravity.google/docs/plugins).

## Gemini CLI (legacy)

For users who retain Gemini CLI access, install the extension and enter your MaxMind account ID and license key when prompted:

```bash
gemini extensions install https://github.com/oschwald/maxminddb-mcp
```

Gemini selects the native release ZIP for your platform. It stores the license key as a sensitive setting and downloads `GeoLite2-City` and `GeoLite2-ASN` into the OS user cache. Ask it to check your MaxMindDB setup before the first lookup. To change credentials, run `gemini extensions config maxminddb`.

Installing from a Git branch with `--ref main` uses the repository manifest and requires Docker. This mode stores databases in the `maxminddb-mcp-data` volume. Native release ZIPs do not require Docker.

## Cursor plugin

The repository includes a Cursor plugin manifest at `.cursor-plugin/plugin.json`. It declares the MaxMind account ID, license key, and edition settings. Configure these through Cursor's plugin settings. This plugin requires Docker and stores databases in the `maxminddb-mcp-data` volume.

See [Cursor's submission instructions](https://cursor.com/docs/reference/plugins#submitting-a-plugin). Installation and settings still need validation in Cursor. This is not an approved public listing.

## Zed

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

## Docker

Use interactive stdin (`-i`) without a TTY. Set `database_dir = "/data"` in a MaxMind-mode config:

```bash
docker run --rm -i \
  -v "$HOME/.config/maxminddb-mcp/config.toml:/config/config.toml:ro" \
  -v maxminddb-data:/data \
  ghcr.io/oschwald/maxminddb-mcp:latest
```

The image runs as UID 1001. A config bind mount must be readable by that user. A data bind mount must be writable by that user when downloads are enabled. Docker named volumes inherit the image's `/data` permissions on first use.

The container has no HTTP port or subprocess healthcheck. Check the running MCP connection from the client.
