import { copyFile, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { packExtension } from "@anthropic-ai/mcpb";

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

  const manifest = JSON.parse(
    await readFile(join(root, "packaging/manifest.json"), "utf8")
  );
  manifest.version = version;
  manifest.display_name = `MaxMindDB (${goos}/${goarch})`;
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
