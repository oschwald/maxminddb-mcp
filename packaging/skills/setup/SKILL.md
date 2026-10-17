---
name: setup
description: Check the MaxMindDB plugin connection and database configuration after installation, or troubleshoot missing databases and failed startup.
---

# Set up MaxMindDB

Use the MaxMindDB MCP tools for these checks. The plugin runs a bundled executable
on the user's computer.

1. Call `list_databases`. If the tools are unavailable, check that the plugin is
   enabled in a local thread and that its OS and architecture match the computer.
   Use the client-specific setup instructions below. The executable discovers
   `~/.config/maxminddb-mcp/config.toml`,
   `~/.config/maxminddb-mcp/GeoIP.conf`, then `/etc/GeoIP.conf`.
   After correcting configuration, restart the server and use a new conversation.
2. Read `status`, `message`, and `update` from the result. If downloading, explain
   that tools are connected but databases are not ready. Check again when the
   user asks or after other setup work. Do not repeatedly poll or trigger another
   download. For an empty or error result, report the corrective action given by
   the server. An available database can still be tested if an update failed.
3. When a database is available, call `lookup_ip` for `8.8.8.8`, passing its exact
   filename as `database`. Prefer a GeoIP2 or GeoLite2 City, Country, or ASN
   database. If `found` is false, report that the lookup worked but this database
   has no matching record. Do not invent a location or network owner.
4. Report whether the server connected, which databases are available, and the
   sample lookup result. State any remaining setup step. Describe geolocation as
   database information, not a person's precise location or a live observation.

Keep license keys out of conversation and tool arguments. Do not read credential
files into the conversation, edit the installed plugin, or overwrite existing
configuration. For missing credentials, direct the user to configure them outside
chat using the README. Checking setup does not require installing or updating a
database through a tool.

## Client configuration

- **Claude:** Direct the user to the plugin's settings to select MaxMind downloads
  or an existing database folder. For downloads, enter the account ID and license
  key in those settings, outside chat. If the client does not show plugin settings,
  use the standalone configuration instructions in the bundled
  [README](../../README.md). MCPB extension settings are a separate installation.
- **ChatGPT:** The OpenAI plugin uses existing configuration and does not show
  Claude's settings form. See the ChatGPT desktop and configuration sections of
  the bundled [README](../../README.md). A manually configured stdio server can
  pass environment settings. The plugin does not provide an editor for them.
