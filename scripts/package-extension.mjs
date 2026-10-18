import { copyFile, cp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { getAllFilesWithCount, packExtension } from "@anthropic-ai/mcpb";
import { zipSync } from "fflate";

const root = fileURLToPath(new URL("../", import.meta.url));

export async function packageExtension(binary, version, goos, goarch, dist) {
  if (
    !["darwin", "linux", "windows"].includes(goos) ||
    !["amd64", "arm64"].includes(goarch)
  ) {
    return;
  }
  if (!/^\d+\.\d+\.\d+(?:-[\w.-]+)?$/.test(version)) {
    throw new Error(
      "Expected a release or snapshot version without the v prefix"
    );
  }
  const target = `${goos}_${goarch}`;
  const stage = join(dist, "extensions", target);
  await rm(stage, { recursive: true, force: true });
  await mkdir(join(stage, "server"), { recursive: true });
  const executable = `server/maxminddb-mcp${goos === "windows" ? ".exe" : ""}`;
  await copyFile(binary, join(stage, executable));
  for (const file of ["LICENSE", "README.md"]) {
    await copyFile(join(root, file), join(stage, file));
  }
  await copyFile(join(root, "packaging/icon.png"), join(stage, "icon.png"));
  await cp(join(root, "packaging/skills"), join(stage, "skills"), {
    recursive: true,
  });

  const manifest = JSON.parse(
    await readFile(join(root, "packaging/manifest.json"), "utf8")
  );
  manifest.version = version;
  manifest.compatibility = { platforms: [goos === "windows" ? "win32" : goos] };
  manifest.server.entry_point = executable;
  manifest.server.mcp_config.command = `\${__dirname}/${executable}`;
  await writeFile(
    join(stage, "manifest.json"),
    JSON.stringify(manifest, null, 2) + "\n"
  );

  // Both formats carry the same executable and settings, with no launcher runtime.
  const plugin = {
    name: manifest.name,
    displayName: manifest.display_name,
    version,
    description: manifest.description,
    author: manifest.author,
    homepage: manifest.homepage,
    repository: manifest.repository.url,
    license: manifest.license,
    userConfig: manifest.user_config,
    mcpServers: {
      maxminddb: {
        command: `\${CLAUDE_PLUGIN_ROOT}/${executable}`,
        env: manifest.server.mcp_config.env,
      },
    },
  };
  await mkdir(join(stage, ".claude-plugin"));
  await writeFile(
    join(stage, ".claude-plugin/plugin.json"),
    JSON.stringify(plugin, null, 2) + "\n"
  );
  const bundle = join(dist, `maxminddb-mcp_${version}_${target}.mcpb`);
  if (
    !(await packExtension({
      extensionPath: stage,
      outputPath: bundle,
      silent: true,
    }))
  ) {
    throw new Error(`Failed to package ${target}`);
  }
  await copyFile(
    bundle,
    join(dist, `maxminddb-mcp_${version}_${target}_claude-plugin.zip`)
  );

  const openaiStage = join(dist, "openai-plugins", target);
  await rm(openaiStage, { recursive: true, force: true });
  await mkdir(join(openaiStage, "server"), { recursive: true });
  for (const file of [executable, "LICENSE", "README.md", "icon.png"]) {
    await copyFile(join(stage, file), join(openaiStage, file));
  }
  const openaiPlugin = {
    name: manifest.name,
    version,
    description: manifest.description,
    author: manifest.author,
    homepage: manifest.homepage,
    repository: manifest.repository.url,
    license: manifest.license,
    keywords: manifest.keywords,
    skills: "./skills/",
    mcpServers: "./.mcp.json",
    requires_local_executor: true,
    interface: {
      displayName: manifest.display_name,
      shortDescription: manifest.description,
      developerName: manifest.author.name,
      composerIcon: "./icon.png",
      logo: "./icon.png",
      defaultPrompt: ["Use MaxMindDB to check my setup and look up 8.8.8.8."],
    },
    extensions: {
      "com.openai": {
        onboardingSkill: "./skills/setup/SKILL.md",
      },
    },
  };
  const openaiMCP = {
    mcpServers: {
      maxminddb: {
        type: "stdio",
        command: `\${PLUGIN_ROOT}/${executable}`,
        cwd: "${PLUGIN_ROOT}",
      },
    },
  };
  const marketplace = {
    name: "maxminddb-local",
    interface: { displayName: manifest.display_name },
    plugins: [
      {
        name: manifest.name,
        source: { source: "local", path: "./" },
        policy: { installation: "AVAILABLE", authentication: "ON_INSTALL" },
        category: "Productivity",
      },
    ],
  };
  await mkdir(join(openaiStage, ".agents/plugins"), { recursive: true });
  await mkdir(join(openaiStage, ".codex-plugin"));
  for (const [file, contents] of Object.entries({
    ".codex-plugin/plugin.json": openaiPlugin,
    ".mcp.json": openaiMCP,
    ".agents/plugins/marketplace.json": marketplace,
  })) {
    await writeFile(
      join(openaiStage, file),
      JSON.stringify(contents, null, 2) + "\n"
    );
  }
  await cp(join(stage, "skills"), join(openaiStage, "skills"), {
    recursive: true,
  });
  const { files } = getAllFilesWithCount(openaiStage);
  const zipFiles = Object.fromEntries(
    Object.entries(files).map(([path, file]) => [
      path,
      [file.data, { os: 3, attrs: (file.mode & 0o777) << 16 }],
    ])
  );
  await writeFile(
    join(dist, `maxminddb-mcp_${version}_${target}_openai-plugin.zip`),
    zipSync(zipFiles, { level: 9 })
  );

  const gemini = JSON.parse(
    await readFile(join(root, "gemini-extension.json"), "utf8")
  );
  gemini.version = version;
  gemini.mcpServers.maxminddb = {
    command: `\${extensionPath}/${executable}`,
    env: {
      ...gemini.mcpServers.maxminddb.env,
      MAXMINDDB_MCP_CONFIG: "",
      MAXMINDDB_MCP_DOWNLOAD_ENABLED: "true",
      MAXMINDDB_MCP_DATABASE_DIR: "",
    },
  };
  const platform = goos === "windows" ? "win32" : goos;
  const arch = goarch === "amd64" ? "x64" : goarch;
  await writeFile(
    join(dist, `${platform}.${arch}.maxminddb-mcp-gemini.zip`),
    zipSync(
      {
        "gemini-extension.json": [
          Buffer.from(JSON.stringify(gemini, null, 2) + "\n"),
          { os: 3, attrs: 0o644 << 16 },
        ],
        ...Object.fromEntries(
          Object.entries(zipFiles).filter(
            ([path]) =>
              [executable, "LICENSE", "README.md", "icon.png"].includes(path) ||
              path.startsWith("skills/")
          )
        ),
      },
      { level: 9 }
    )
  );
  return bundle;
}

if (import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const [binary, version, goos, goarch, dist] = process.argv.slice(2);
  if (!binary || !version || !goos || !goarch || !dist) {
    throw new Error(
      "Usage: node scripts/package-extension.mjs BINARY VERSION GOOS GOARCH DIST"
    );
  }
  await packageExtension(binary, version, goos, goarch, dist);
}
