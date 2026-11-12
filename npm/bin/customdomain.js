#!/usr/bin/env node
/**
 * The `customdomain` command on npm: runs the native binary from the platform package
 * (@customdomain/cli-<os>-<arch>) that npm installed beside this one as an optional dependency.
 */
"use strict";
const { spawnSync } = require("node:child_process");

const platformPackage = `@customdomain/cli-${process.platform}-${process.arch}`;
const exe = process.platform === "win32" ? "customdomain.exe" : "customdomain";

let binary;
try {
  binary = require.resolve(`${platformPackage}/bin/${exe}`);
} catch {
  console.error(
    `customdomain: there is no build for ${process.platform}-${process.arch} in this install.\n` +
      "Reinstall without --no-optional, or download a release from https://github.com/CUSTOM-DOMAIN-APP/customdomain-cli/releases",
  );
  process.exit(1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
  console.error(`customdomain: ${result.error.message}`);
  process.exit(1);
}
process.exit(result.status === null ? 1 : result.status);
