import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import {
  mkdtemp,
  mkdir,
  readFile,
  rm,
  copyFile,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, sep } from "node:path";
import { createInterface } from "node:readline";
import { test } from "node:test";
import {
  getMcpConfigForManifest,
  hasRequiredConfigMissing,
  unpackExtension,
} from "@anthropic-ai/mcpb";
import { unzipSync } from "fflate";
import { packageExtension } from "./package-extension.mjs";

test(
  "installed Claude and OpenAI packages launch and retain external databases",
  { timeout: 60000 },
  async t => {
    const temp = await mkdtemp(join(tmpdir(), "maxminddb extension "));
    t.after(() => rm(temp, { recursive: true, force: true }));
    const goos = { win32: "windows", linux: "linux", darwin: "darwin" }[
      process.platform
    ];
    const goarch = { x64: "amd64", arm64: "arm64" }[process.arch];
    assert.ok(goos && goarch, "unsupported test platform");
    const binary = join(
      temp,
      process.platform === "win32" ? "maxminddb-mcp.exe" : "maxminddb-mcp"
    );
    execFileSync("go", ["build", "-o", binary, "./cmd/maxminddb-mcp"], {
      stdio: "pipe",
    });
    const dist = join(temp, "dist");
    const bundle = await packageExtension(
      binary,
      "0.0.0-test",
      goos,
      goarch,
      dist
    );
    const installed = join(temp, "installed extension");
    assert.equal(
      await unpackExtension({
        mcpbPath: bundle,
        outputDir: installed,
        silent: true,
      }),
      true
    );
    const manifest = JSON.parse(
      await readFile(join(installed, "manifest.json"), "utf8")
    );
    assert.equal(manifest.server.type, "binary");
    assert.equal(manifest.user_config.license_key.sensitive, true);
    assert.equal(hasRequiredConfigMissing({ manifest, userConfig: {} }), true);
    assert.equal(
      hasRequiredConfigMissing({
        manifest,
        userConfig: { download_enabled: false },
      }),
      false
    );
    const plugin = JSON.parse(
      await readFile(join(installed, ".claude-plugin/plugin.json"), "utf8")
    );
    assert.deepEqual(plugin.userConfig, manifest.user_config);
    assert.deepEqual(
      plugin.mcpServers.maxminddb.env,
      manifest.server.mcp_config.env
    );

    const databases = join(temp, "MMDB files");
    await mkdir(databases);
    const database = "GeoLite2-ASN-Test.mmdb";
    await copyFile(
      join("testdata/test-data", database),
      join(databases, database)
    );
    const settings = await getMcpConfigForManifest({
      manifest,
      extensionPath: installed,
      systemDirs: { HOME: temp },
      userConfig: { download_enabled: false, database_dir: databases },
      pathSeparator: sep,
    });
    assert.equal(settings.env.MAXMINDDB_MCP_DOWNLOAD_ENABLED, "false");
    assert.equal(settings.env.MAXMINDDB_MCP_LICENSE_KEY, "");
    assert.equal(settings.env.MAXMINDDB_MCP_CONFIG, "");
    async function checkServer(settings) {
      const child = spawn(settings.command, settings.args ?? [], {
        cwd: settings.cwd ?? temp,
        signal: t.signal,
        env: {
          ...Object.fromEntries(
            Object.entries(process.env).filter(
              ([name]) => !name.startsWith("MAXMINDDB_MCP_")
            )
          ),
          HOME: temp,
          USERPROFILE: temp,
          ...settings.env,
        },
        stdio: ["pipe", "pipe", "pipe"],
      });
      const closed = new Promise(resolve => child.once("close", resolve));
      let logs = "";
      child.stderr.setEncoding("utf8").on("data", chunk => {
        logs += chunk;
      });
      child.on("error", err => {
        logs += err.message;
      });
      const lines = createInterface({ input: child.stdout });
      const replies = lines[Symbol.asyncIterator]();
      let requestID = 0;
      async function call(method, params) {
        const id = ++requestID;
        child.stdin.write(
          JSON.stringify({ jsonrpc: "2.0", id, method, params }) + "\n"
        );
        const response = await replies.next();
        assert.equal(response.done, false, logs);
        const message = JSON.parse(response.value);
        assert.equal(message.id, id);
        assert.equal(message.error, undefined, JSON.stringify(message.error));
        return message.result;
      }
      try {
        await call("initialize", {
          protocolVersion: "2025-11-25",
          capabilities: {},
          clientInfo: { name: "bundle-smoke-test", version: "1" },
        });
        child.stdin.write(
          JSON.stringify({
            jsonrpc: "2.0",
            method: "notifications/initialized",
          }) + "\n"
        );
        const list = await call("tools/call", {
          name: "list_databases",
          arguments: {},
        });
        assert.equal(list.isError, undefined);
        assert.equal(list.structuredContent.status, "ready");
        assert.equal(list.structuredContent.databases[0].name, database);
        const lookup = await call("tools/call", {
          name: "lookup_ip",
          arguments: { database, ip: "1.0.0.1" },
        });
        assert.equal(lookup.structuredContent.found, true);
        assert.equal(
          typeof lookup.structuredContent.data.autonomous_system_number,
          "number"
        );
      } finally {
        lines.close();
        child.kill();
        await closed;
      }
    }
    await checkServer({
      ...settings,
      env: { MAXMINDDB_MCP_CONFIG: "nonexistent.toml", ...settings.env },
    });

    const openaiBundle = join(
      dist,
      `maxminddb-mcp_0.0.0-test_${goos}_${goarch}_openai-plugin.zip`
    );
    const entries = unzipSync(await readFile(openaiBundle));
    const executable = `server/maxminddb-mcp${goos === "windows" ? ".exe" : ""}`;
    assert.deepEqual(
      Object.keys(entries).sort(),
      [
        ".agents/plugins/marketplace.json",
        ".codex-plugin/plugin.json",
        ".mcp.json",
        "LICENSE",
        "README.md",
        "icon.png",
        executable,
        "skills/setup/SKILL.md",
      ].sort()
    );
    const openaiInstalled = join(temp, "installed OpenAI plugin");
    assert.equal(
      await unpackExtension({
        mcpbPath: openaiBundle,
        outputDir: openaiInstalled,
        silent: true,
      }),
      true
    );
    const openai = JSON.parse(
      await readFile(join(openaiInstalled, ".codex-plugin/plugin.json"), "utf8")
    );
    assert.equal(openai.version, manifest.version);
    assert.equal(openai.requires_local_executor, true);
    const onboarding = openai.extensions["com.openai"].onboardingSkill;
    assert.deepEqual(
      await readFile(join(openaiInstalled, onboarding)),
      await readFile(join(installed, "skills/setup/SKILL.md"))
    );
    const marketplace = JSON.parse(
      await readFile(
        join(openaiInstalled, ".agents/plugins/marketplace.json"),
        "utf8"
      )
    );
    assert.equal(marketplace.plugins[0].name, openai.name);
    assert.equal(marketplace.plugins[0].source.path, "./");
    const mcp = JSON.parse(
      await readFile(join(openaiInstalled, openai.mcpServers), "utf8")
    );
    const server = mcp.mcpServers.maxminddb;
    assert.equal(server.type, "stdio");
    // Empty environment settings would override the user's discovered config.
    assert.equal(server.env, undefined);
    assert.equal(server.command, "${PLUGIN_ROOT}/" + executable);
    const configDir = join(temp, ".config", "maxminddb-mcp");
    await mkdir(configDir, { recursive: true });
    await writeFile(
      join(configDir, "config.toml"),
      `mode = "directory"\n[directory]\npaths = [${JSON.stringify(databases)}]\n`
    );
    const launch = {
      command: server.command.replace("${PLUGIN_ROOT}", () => openaiInstalled),
      cwd: server.cwd.replace("${PLUGIN_ROOT}", () => openaiInstalled),
      args: server.args,
    };
    await checkServer(launch);
    await rm(openaiInstalled, { recursive: true });
    assert.equal(
      await unpackExtension({
        mcpbPath: openaiBundle,
        outputDir: openaiInstalled,
        silent: true,
      }),
      true
    );
    await checkServer(launch);

    // Replacing the extension must not replace the user's database folder.
    await rm(installed, { recursive: true });
    assert.equal(
      await unpackExtension({
        mcpbPath: bundle,
        outputDir: installed,
        silent: true,
      }),
      true
    );
    assert.deepEqual(
      await readFile(join(databases, database)),
      await readFile(join("testdata/test-data", database))
    );

    const invalid = await getMcpConfigForManifest({
      manifest,
      extensionPath: installed,
      systemDirs: { HOME: temp },
      userConfig: {
        download_enabled: true,
        account_id: "invalid-account",
        license_key: "do-not-log-this-key",
      },
      pathSeparator: sep,
    });
    assert.throws(
      () =>
        execFileSync(invalid.command, [], {
          cwd: temp,
          env: { ...process.env, ...invalid.env },
          stdio: "pipe",
        }),
      error => {
        assert.equal(error.status, 1);
        assert.match(error.stderr.toString(), /account ID/);
        assert.doesNotMatch(error.stderr.toString(), /do-not-log-this-key/);
        assert.equal(error.stdout.length, 0);
        return true;
      }
    );
  }
);
