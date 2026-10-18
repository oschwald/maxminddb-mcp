import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  copyFile,
  mkdir,
  mkdtemp,
  readFile,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { createRequire } from "node:module";
import { delimiter, dirname, join, resolve } from "node:path";
import { unpackExtension } from "@anthropic-ai/mcpb";

const source = process.argv[2];
if (!source)
  throw new Error("Usage: node scripts/test-claude-plugin.mjs PLUGIN_ZIP");
const require = createRequire(import.meta.url);
const claudePackage = require.resolve(
  "@anthropic-ai/claude-code/package.json",
  {
    paths: process.env.PATH.split(delimiter),
  }
);
// Bypass npm's Windows command shim without passing arguments through a shell.
const claudeExecutable = join(
  dirname(claudePackage),
  require(claudePackage).bin.claude
);
const temp = await mkdtemp(join(tmpdir(), "maxminddb claude "));
const home = join(temp, "home");
const marketplace = join(temp, "marketplace");
const plugin = join(marketplace, "plugin");
const databases = join(temp, "MMDB files");
const secret = "dummy-license-for-plugin-test";
const debugLog = join(temp, "claude-debug.log");
let diagnostics = "";
const env = {
  PATH: process.env.PATH,
  SystemRoot: process.env.SystemRoot,
  // macOS uses HOME to find its Keychain. CLAUDE_CONFIG_DIR isolates Claude's
  // settings and gives its test credentials a separate Keychain service name.
  HOME: process.platform === "darwin" ? process.env.HOME : home,
  USERPROFILE: home,
  XDG_CONFIG_HOME: join(home, ".config"),
  XDG_CACHE_HOME: join(home, ".cache"),
  CLAUDE_CONFIG_DIR: join(home, ".claude"),
  CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
};
function claude(...args) {
  const result = spawnSync(
    claudeExecutable,
    ["--debug-file", debugLog, ...args],
    {
      cwd: temp,
      env,
      encoding: "utf8",
      timeout: 45000,
      stdio: ["pipe", "pipe", "pipe"],
    }
  );
  const output = (result.stdout ?? "") + (result.stderr ?? "");
  diagnostics += output;
  assert.ok(
    !output.includes(secret),
    "Claude output must not expose the license key"
  );
  if (result.error) throw result.error;
  assert.equal(
    result.status,
    0,
    `Claude command failed (${result.signal ?? result.status})`
  );
  return result.stdout;
}

async function checkSavedLicense() {
  await writeFile(debugLog, "");
  assert.match(
    claude("mcp", "list"),
    /plugin:maxminddb:maxminddb:.*Failed to connect/
  );
  const debug = await readFile(debugLog, "utf8");
  assert.ok(
    !debug.includes(secret),
    "Claude debug output must not expose the license key"
  );
  assert.match(debug, /database editions must be edition IDs, not paths:/);
}

try {
  await mkdir(join(marketplace, ".claude-plugin"), { recursive: true });
  assert.equal(
    await unpackExtension({
      mcpbPath: resolve(source),
      outputDir: plugin,
      silent: true,
    }),
    true
  );
  await writeFile(
    join(marketplace, ".claude-plugin/marketplace.json"),
    JSON.stringify({
      name: "maxminddb-test",
      owner: { name: "MaxMindDB tests" },
      description: "Isolated plugin installation test",
      plugins: [{ name: "maxminddb", source: "./plugin" }],
    })
  );
  await mkdir(databases);
  const fixture = await readFile("testdata/test-data/GeoLite2-ASN-Test.mmdb");
  await copyFile(
    "testdata/test-data/GeoLite2-ASN-Test.mmdb",
    join(databases, "GeoLite2-ASN-Test.mmdb")
  );
  claude("plugin", "marketplace", "add", marketplace);
  const id = "maxminddb@maxminddb-test";
  claude(
    "plugin",
    "install",
    id,
    "--config",
    "maxminddb.download_enabled=false",
    "--config",
    `maxminddb.database_dir=${databases}`,
    "--config",
    `maxminddb.license_key=${secret}`
  );
  assert.match(claude("mcp", "list"), /plugin:maxminddb:maxminddb:.*Connected/);
  const details = claude("plugin", "details", id);
  assert.match(details, /investigate-ips/);
  assert.match(details, /setup/);
  const settings = await readFile(join(home, ".claude/settings.json"), "utf8");
  assert.ok(
    !settings.includes(secret),
    "License key must not be in ordinary settings"
  );

  // Edition validation follows license-key validation and precedes downloads.
  claude(
    "plugin",
    "install",
    id,
    "--config",
    "maxminddb.download_enabled=true",
    "--config",
    "maxminddb.account_id=1",
    "--config",
    "maxminddb.editions=../invalid"
  );
  await checkSavedLicense();

  // Replace the package and its MCPB cache, keeping the client's saved settings.
  await rm(plugin, { recursive: true });
  assert.equal(
    await unpackExtension({
      mcpbPath: resolve(source),
      outputDir: plugin,
      silent: true,
    }),
    true
  );
  await checkSavedLicense();
  claude(
    "plugin",
    "install",
    id,
    "--config",
    "maxminddb.download_enabled=false"
  );
  assert.match(claude("mcp", "list"), /plugin:maxminddb:maxminddb:.*Connected/);
  assert.deepEqual(
    await readFile(join(databases, "GeoLite2-ASN-Test.mmdb")),
    fixture
  );

  console.log(
    "Claude plugin installation, settings, replacement, and credential retention passed."
  );
} catch (error) {
  const debug = await readFile(debugLog, "utf8").catch(() => "");
  console.error((diagnostics + debug).replaceAll(secret, "[REDACTED]"));
  throw new Error(String(error).replaceAll(secret, "[REDACTED]"));
} finally {
  await rm(temp, { recursive: true, force: true });
}
