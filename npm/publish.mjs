/**
 * Builds and publishes the npm packages from a finished goreleaser run: one package per platform
 * (@customdomain/cli-<os>-<arch>, holding the binary) and then the main `customdomain` package, which
 * lists them as optional dependencies. Usage: node npm/publish.mjs <version> [--dry-run]
 */
import { readFileSync, writeFileSync, mkdirSync, copyFileSync, chmodSync, rmSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { join } from "node:path";

const version = (process.argv[2] || "").replace(/^v/, "");
const dryRun = process.argv.includes("--dry-run");
if (!/^\d+\.\d+\.\d+(-[\w.]+)?$/.test(version)) {
  console.error("usage: node npm/publish.mjs <version> [--dry-run]");
  process.exit(2);
}

const root = process.cwd();
const out = join(root, "dist", "npm");
const repository = { type: "git", url: "git+https://github.com/CUSTOM-DOMAIN-APP/customdomain-cli.git" };
const common = { version, license: "Apache-2.0", homepage: "https://customdomain.ai", repository };
const osMap = { darwin: "darwin", linux: "linux", windows: "win32" };
const archMap = { amd64: "x64", arm64: "arm64" };

const artifacts = JSON.parse(readFileSync(join(root, "dist", "artifacts.json"), "utf8"));
const binaries = artifacts.filter((a) => a.type === "Binary" && osMap[a.goos] && archMap[a.goarch]);
if (binaries.length === 0) throw new Error("no binaries in dist/artifacts.json; run goreleaser first");

rmSync(out, { recursive: true, force: true });
const optionalDependencies = {};

function publish(dir) {
  const args = ["publish", dir, "--access", "public"];
  if (process.env.GITHUB_ACTIONS) args.push("--provenance");
  if (dryRun) args.push("--dry-run");
  execFileSync("npm", args, { stdio: "inherit" });
}

for (const b of binaries) {
  const os = osMap[b.goos];
  const cpu = archMap[b.goarch];
  const name = `@customdomain/cli-${os}-${cpu}`;
  const dir = join(out, `cli-${os}-${cpu}`);
  const exe = os === "win32" ? "customdomain.exe" : "customdomain";
  mkdirSync(join(dir, "bin"), { recursive: true });
  copyFileSync(join(root, b.path), join(dir, "bin", exe));
  chmodSync(join(dir, "bin", exe), 0o755);
  copyFileSync(join(root, "LICENSE"), join(dir, "LICENSE"));
  writeFileSync(
    join(dir, "package.json"),
    JSON.stringify({ name, ...common, description: `The customdomain command for ${os} ${cpu}.`, os: [os], cpu: [cpu], preferUnplugged: true }, null, 2) + "\n",
  );
  writeFileSync(join(dir, "README.md"), `The \`customdomain\` binary for ${os} ${cpu}. Install \`customdomain\` instead.\n`);
  optionalDependencies[name] = version;
  publish(dir);
}

const main = join(out, "customdomain");
mkdirSync(join(main, "bin"), { recursive: true });
copyFileSync(join(root, "npm", "bin", "customdomain.js"), join(main, "bin", "customdomain.js"));
chmodSync(join(main, "bin", "customdomain.js"), 0o755);
copyFileSync(join(root, "LICENSE"), join(main, "LICENSE"));
copyFileSync(join(root, "README.md"), join(main, "README.md"));
writeFileSync(
  join(main, "package.json"),
  JSON.stringify(
    {
      name: "customdomain",
      ...common,
      description: "Connect custom domains to your app from the terminal: sign in, create an API key, check a domain.",
      keywords: ["custom domain", "dns", "ssl", "cli", "customdomain"],
      bin: { customdomain: "bin/customdomain.js" },
      files: ["bin", "LICENSE", "README.md"],
      engines: { node: ">=18" },
      optionalDependencies,
    },
    null,
    2,
  ) + "\n",
);
publish(main);
