import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { createHash } from "node:crypto";
import {
  mkdtemp,
  mkdir,
  readFile,
  rm,
  copyFile,
  chmod,
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
import {
  extensionTargets,
  packageUniversalExtension,
} from "./package-universal-extension.mjs";

test(
  "installed extension packages launch and retain external databases",
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

    const geminiBundle = join(
      dist,
      `${process.platform}.${process.arch}.maxminddb-mcp-gemini.zip`
    );
    const geminiInstalled = join(temp, "installed Gemini extension");
    assert.equal(
      await unpackExtension({
        mcpbPath: geminiBundle,
        outputDir: geminiInstalled,
        silent: true,
      }),
      true
    );
    const gemini = JSON.parse(
      await readFile(join(geminiInstalled, "gemini-extension.json"), "utf8")
    );
    assert.equal(gemini.version, manifest.version);
    assert.equal(
      gemini.settings.find(
        setting => setting.envVar === "MAXMINDDB_MCP_LICENSE_KEY"
      ).sensitive,
      true
    );
    const geminiServer = gemini.mcpServers.maxminddb;
    assert.equal(geminiServer.env.MAXMINDDB_MCP_CONFIG, "");
    await checkServer({
      ...geminiServer,
      command: geminiServer.command.replace(
        "${extensionPath}",
        () => geminiInstalled
      ),
      env: { ...geminiServer.env, ...settings.env },
    });

    // Other targets are placeholders here. Release CI supplies each native build.
    for (const target of extensionTargets) {
      if (target === `${goos}_${goarch}`) continue;
      const stage = join(dist, "extensions", target);
      await mkdir(join(stage, "server"), { recursive: true });
      await writeFile(
        join(stage, "manifest.json"),
        JSON.stringify({ version: manifest.version })
      );
      await writeFile(
        join(
          stage,
          "server",
          `maxminddb-mcp${target.startsWith("windows_") ? ".exe" : ""}`
        ),
        "placeholder"
      );
    }
    const universalBundle = join(dist, "maxminddb.mcpb");
    await writeFile(
      join(dist, "checksums.txt"),
      "existing-checksum  other.zip\nstale-checksum  maxminddb.mcpb\n"
    );
    execFileSync(
      process.execPath,
      ["scripts/package-universal-extension.mjs", manifest.version, dist],
      { stdio: "pipe" }
    );
    const checksum = createHash("sha256")
      .update(await readFile(universalBundle))
      .digest("hex");
    assert.equal(
      await readFile(join(dist, "checksums.txt"), "utf8"),
      `existing-checksum  other.zip\n${checksum}  maxminddb.mcpb\n`
    );
    const universalInstalled = join(temp, "installed universal extension");
    assert.equal(
      await unpackExtension({
        mcpbPath: universalBundle,
        outputDir: universalInstalled,
        silent: true,
      }),
      true
    );
    const universal = JSON.parse(
      await readFile(join(universalInstalled, "manifest.json"), "utf8")
    );
    assert.deepEqual(universal.user_config, manifest.user_config);
    assert.deepEqual(universal.compatibility.platforms, [
      "darwin",
      "linux",
      "win32",
    ]);
    const universalSettings = await getMcpConfigForManifest({
      manifest: universal,
      extensionPath: universalInstalled,
      systemDirs: { HOME: temp },
      userConfig: { download_enabled: false, database_dir: databases },
      pathSeparator: sep,
    });
    await checkServer(universalSettings);
    assert.equal(
      universal.server.mcp_config.platform_overrides.win32.command,
      "${__dirname}/server/windows_amd64/maxminddb-mcp.exe"
    );
    assert.deepEqual(
      universal.server.mcp_config.platform_overrides.win32.args,
      []
    );
    await assert.rejects(
      packageUniversalExtension("9.9.9", dist),
      /Expected .* version 9.9.9/
    );
    await rm(join(dist, "extensions", "darwin_arm64"), {
      recursive: true,
    });
    await assert.rejects(packageUniversalExtension(manifest.version, dist), {
      code: "ENOENT",
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

test(
  "universal launcher selects the CPU and preserves arguments",
  {
    skip: process.platform === "win32",
  },
  async t => {
    const temp = await mkdtemp(join(tmpdir(), "maxminddb launcher "));
    t.after(() => rm(temp, { recursive: true, force: true }));
    await mkdir(join(temp, "bin"));
    const uname = join(temp, "bin", "uname");
    await writeFile(
      uname,
      '#!/bin/sh\ncase "$1" in -s) printf "%s\\n" "$TEST_SYSTEM" ;; -m) printf "%s\\n" "$TEST_ARCH" ;; esac\n'
    );
    await chmod(uname, 0o755);
    const launcher = join(temp, "launch.sh");
    await copyFile("packaging/launch-universal.sh", launcher);
    for (const [system, arch, target] of [
      ["Darwin", "x86_64", "darwin_amd64"],
      ["Darwin", "arm64", "darwin_arm64"],
      ["Linux", "x86_64", "linux_amd64"],
      ["Linux", "aarch64", "linux_arm64"],
      ["Linux", "arm64", "linux_arm64"],
    ]) {
      await mkdir(join(temp, target), { recursive: true });
      const executable = join(temp, target, "maxminddb-mcp");
      await writeFile(
        executable,
        `#!/bin/sh\nprintf '%s\\n' '${target}' "$@"\n`
      );
      await chmod(executable, 0o755);
      assert.equal(
        execFileSync(
          "/bin/sh",
          [launcher, "argument with spaces", "$literal"],
          {
            env: {
              ...process.env,
              PATH: join(temp, "bin"),
              TEST_SYSTEM: system,
              TEST_ARCH: arch,
            },
            encoding: "utf8",
          }
        ),
        `${target}\nargument with spaces\n$literal\n`
      );
    }
    assert.throws(
      () =>
        execFileSync("/bin/sh", [launcher], {
          env: {
            ...process.env,
            PATH: join(temp, "bin"),
            TEST_SYSTEM: "Linux",
            TEST_ARCH: "riscv64",
          },
          stdio: "pipe",
        }),
      error => {
        assert.equal(error.status, 1);
        assert.equal(error.stdout.length, 0);
        assert.match(
          error.stderr.toString(),
          /Unsupported platform: Linux\/riscv64/
        );
        return true;
      }
    );
  }
);
