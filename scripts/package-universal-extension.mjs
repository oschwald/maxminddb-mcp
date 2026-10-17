import { createHash } from "node:crypto";
import { copyFile, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { basename, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { packExtension } from "@anthropic-ai/mcpb";

const root = fileURLToPath(new URL("../", import.meta.url));
export const extensionTargets = [
  "darwin_amd64",
  "darwin_arm64",
  "linux_amd64",
  "linux_arm64",
  "windows_amd64",
];

export async function packageUniversalExtension(version, dist) {
  if (!/^\d+\.\d+\.\d+(?:-[\w.-]+)?$/.test(version)) {
    throw new Error(
      "Expected a release or snapshot version without the v prefix"
    );
  }
  const stage = join(dist, "universal-extension");
  await rm(stage, { recursive: true, force: true });
  for (const target of extensionTargets) {
    const source = join(dist, "extensions", target);
    const manifest = JSON.parse(
      await readFile(join(source, "manifest.json"), "utf8")
    );
    if (manifest.version !== version) {
      throw new Error(
        `Expected ${target} version ${version}, found ${manifest.version}`
      );
    }
    const executable = `maxminddb-mcp${target.startsWith("windows_") ? ".exe" : ""}`;
    await mkdir(join(stage, "server", target), { recursive: true });
    await copyFile(
      join(source, "server", executable),
      join(stage, "server", target, executable)
    );
  }
  for (const file of ["LICENSE", "README.md"]) {
    await copyFile(join(root, file), join(stage, file));
  }
  await copyFile(join(root, "packaging/icon.png"), join(stage, "icon.png"));
  await copyFile(
    join(root, "packaging/launch-universal.sh"),
    join(stage, "server/launch.sh")
  );
  const manifest = JSON.parse(
    await readFile(join(root, "packaging/manifest.json"), "utf8")
  );
  manifest.version = version;
  manifest.compatibility = { platforms: ["darwin", "linux", "win32"] };
  manifest.server.entry_point = "server/launch.sh";
  manifest.server.mcp_config.command = "/bin/sh";
  manifest.server.mcp_config.args = ["${__dirname}/server/launch.sh"];
  manifest.server.mcp_config.platform_overrides = {
    win32: {
      command: "${__dirname}/server/windows_amd64/maxminddb-mcp.exe",
      args: [],
    },
  };
  await writeFile(
    join(stage, "manifest.json"),
    JSON.stringify(manifest, null, 2) + "\n"
  );
  const bundle = join(dist, "maxminddb.mcpb");
  if (
    !(await packExtension({
      extensionPath: stage,
      outputPath: bundle,
      silent: true,
    }))
  ) {
    throw new Error("Failed to package the universal extension");
  }
  return bundle;
}

if (import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const [version, dist] = process.argv.slice(2);
  if (!version || !dist) {
    throw new Error(
      "Usage: node scripts/package-universal-extension.mjs VERSION DIST"
    );
  }
  const bundle = await packageUniversalExtension(version, dist);
  const checksumFile = join(dist, "checksums.txt");
  const checksums = (await readFile(checksumFile, "utf8"))
    .split("\n")
    .filter(line => line && !line.endsWith(`  ${basename(bundle)}`));
  const checksum = createHash("sha256")
    .update(await readFile(bundle))
    .digest("hex");
  checksums.push(`${checksum}  ${basename(bundle)}`);
  await writeFile(checksumFile, checksums.join("\n") + "\n");
}
