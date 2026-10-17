# MaxMindDB MCP Server

[![License: ISC](https://img.shields.io/badge/License-ISC-blue.svg)](https://opensource.org/licenses/ISC)

Look up IP locations and network ownership in GeoIP2, GeoLite2, and custom MMDB databases from your AI assistant.
Search networks, compare databases, and download new databases through MCP.

This is an unofficial project and is not endorsed by MaxMind Inc. For official products and support, visit [maxmind.com](https://www.maxmind.com/).

Try asking:

- "Using MaxMindDB, identify the country and autonomous system for 8.8.8.8."
- "Find the networks belonging to AS13335 within 1.0.0.0/8 in my ASN database."
- "Which databases are available, and when were they built?"

Answers come from your databases. IP geolocation is an estimate, not a device's precise location.
ASN data describes network ownership, not the identity of a person using an address.

## What you need

Provide either existing `.mmdb` files or a [MaxMind account ID and license key](https://www.maxmind.com/en/accounts/current/license-key) with access to the editions you want.
Databases and credentials are not bundled.

The server runs on your computer and requires a client that supports local MCP servers.

## Installation

### Claude Desktop

Install and configure the server through Claude Desktop. No terminal, Go installation, or config file is required.

1. [Download the latest MCPB](https://github.com/oschwald/maxminddb-mcp/releases/latest/download/maxminddb.mcpb).
   The single file includes native builds for all supported platforms.
2. Open **Settings → Extensions → Advanced settings → Install Extension** in Claude Desktop and select the file.
3. Choose how to provide databases in the extension settings:
   - **Download databases:** Enter your MaxMind account ID and license key. The default editions are `GeoLite2-City` and `GeoLite2-ASN`.
   - **Use existing files:** Turn off **Download and update MaxMind databases** and select the folder containing your `.mmdb` files.
4. Enable the extension and ask: "Using MaxMindDB, list my databases and look up 8.8.8.8."

The first download runs in the background. Ask Claude to check database status if lookups are not ready yet.
Enter license keys in extension settings, not in chat.

**Upgrading from v0.2.0 or earlier:** Remove the old extension, install the new one, and configure it again.
Your database files remain in place. See [upgrades, settings, and supported platforms](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/clients.md#claude-desktop).

### Standalone executable

On macOS, install with Homebrew:

```bash
brew install --cask oschwald/tap/maxminddb-mcp
```

For other platforms, or without Homebrew, download an archive or Linux package from [GitHub Releases](https://github.com/oschwald/maxminddb-mcp/releases).
See [standalone setup](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/clients.md#standalone-executable) for installation and checksums.
Then [configure the server](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/configuration.md) and connect your client below.

<a id="build-from-source"></a>

Go developers can also [build from source](https://github.com/oschwald/maxminddb-mcp/blob/main/CONTRIBUTING.md#build-from-source).

## Connect a client

Choose your client for step-by-step setup. The guides include the commands, settings, and upgrade instructions for each installation method.

| Client                                                                                                                                                                                           | Setup requirements                                                              |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------- |
| <a id="chatgpt-desktop"></a>[ChatGPT desktop](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/clients.md#chatgpt-desktop)                                                               | Installed executable and configuration; local threads only                      |
| <a id="codex"></a>[Codex](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/clients.md#codex)                                                                                             | Installed executable; config file or environment settings                       |
| <a id="claude-code"></a>[Claude Code](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/clients.md#claude-code)                                                                           | Installed executable and configuration, or Docker plugin                        |
| <a id="antigravity-cli-agy"></a>[Antigravity CLI (`agy`)](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/clients.md#antigravity-cli-agy)                                               | Installed executable and configuration                                          |
| <a id="zed"></a>[Zed](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/clients.md#zed)                                                                                                   | Installed executable and configuration                                          |
| <a id="docker"></a>[Docker](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/clients.md#docker)                                                                                          | Docker engine and a config file readable by the container                       |
| <a id="chatgpt-desktop-plugin-experimental"></a>[ChatGPT desktop plugin (experimental)](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/clients.md#chatgpt-desktop-plugin-experimental) | Codex CLI registration and existing server configuration; desktop UI unverified |
| <a id="claude-cowork-plugin-experimental"></a>[Claude Cowork plugin (experimental)](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/clients.md#claude-cowork-plugin-experimental)       | Native plugin ZIP; installation and settings UI unverified                      |
| <a id="cursor-plugin"></a>[Cursor plugin (unverified)](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/clients.md#cursor-plugin)                                                        | Docker engine; installation and settings UI unverified                          |
| <a id="gemini-cli-legacy"></a>[Gemini CLI (legacy)](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/clients.md#gemini-cli-legacy)                                                       | Continued Gemini CLI access; native extension prompts for account settings      |

<a id="configure"></a>
<a id="config-discovery-and-defaults"></a>
<a id="configuration-without-a-file"></a>

## Configuration

Claude Desktop users can manage settings in the extension. For standalone installations, the [configuration guide](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/configuration.md) covers:

- Downloads using MaxMind credentials.
- Existing MMDB folders or `GeoIP.conf`.
- Environment settings, config discovery, and logging.

If setup fails, start with [troubleshooting](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/configuration.md#troubleshooting).

## Tools

Your assistant discovers the tools automatically. You can ask it to look up IPs, search or summarize networks,
compare and inspect databases, or install and update databases.

The [tool reference](https://github.com/oschwald/maxminddb-mcp/blob/main/docs/tools.md) documents arguments, results, filters, pagination, limits, and errors.

<a id="protocol-and-development"></a>

## Help and development

- [Report an issue](https://github.com/oschwald/maxminddb-mcp/issues).
- [Contributing](https://github.com/oschwald/maxminddb-mcp/blob/main/CONTRIBUTING.md): build, test, and release instructions.
- [Changelog](https://github.com/oschwald/maxminddb-mcp/blob/main/CHANGELOG.md).

## License

[ISC](LICENSE).
