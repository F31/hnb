#!/usr/bin/env node

import { createHash } from "node:crypto";
import { chmod, mkdir, readFile, rename, rm, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repositoryRoot = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const lock = JSON.parse(await readFile(path.join(repositoryRoot, "contracts/toolchain.lock.json"), "utf8"));
const toolsRoot = path.join(repositoryRoot, ".tools/contracts");
const binDirectory = path.join(toolsRoot, "bin");
const downloadsDirectory = path.join(toolsRoot, "downloads");
const offline = process.argv.includes("--offline") || process.env.HNB_CONTRACT_TOOLS_OFFLINE === "1";

/**
 * 最小 semver 范围匹配：仅支持本项目使用的语法
 *  - 精确版本："1.2.3"
 *  - 脱字符范围："^1.2.3"（允许同主版本，且 0.x 锁到主+次、>=1.0.0 锁主版本）
 *  - 或分隔的多个范围："^1.2.3 || ^2.0.0"
 * 不支持通配符、预发布标识、-range 号等更复杂语法（本仓库锁文件不使用）。
 */
function satisfiesRange(version, range) {
  const parts = String(version).split(".");
  const groups = String(range).split("||").map((g) => g.trim()).filter(Boolean);
  for (const group of groups) {
    const caret = group.startsWith("^");
    const target = caret ? group.slice(1) : group;
    const targetParts = target.split(".");
    let mismatch = false;
    for (let i = 0; i < 3; i += 1) {
      const v = Number(parts[i] ?? 0);
      const t = Number(targetParts[i] ?? 0);
      if (caret) {
        // ^X.Y.Z：X 不同则不满足；X==0 时 Y 必须相同；X==0 && Y==0 时 Z 必须相同
        if (i === 0 && v !== t) { mismatch = true; break; }
        if (i === 1 && Number(targetParts[0]) === 0 && v !== t) { mismatch = true; break; }
        if (i === 2 && Number(targetParts[0]) === 0 && Number(targetParts[1]) === 0 && v !== t) { mismatch = true; break; }
        if (v < t) { mismatch = true; break; }
      } else {
        if (v !== t) { mismatch = true; break; }
      }
    }
    if (!mismatch) return true;
  }
  return false;
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: repositoryRoot,
    encoding: "utf8",
    stdio: options.capture ? "pipe" : "inherit",
    env: { ...process.env, ...options.env },
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} exited with ${result.status}`);
  }
  return result.stdout?.trim() ?? "";
}

async function exists(file) {
  try {
    await stat(file);
    return true;
  } catch (error) {
    if (error.code === "ENOENT") return false;
    throw error;
  }
}

async function sha256(file) {
  return createHash("sha256").update(await readFile(file)).digest("hex");
}

async function download(url, destination, expectedSha256) {
  if (await exists(destination) && await sha256(destination) === expectedSha256) return;
  if (offline) throw new Error(`offline cache missing or invalid: ${path.relative(repositoryRoot, destination)}`);

  let lastError;
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    try {
      const response = await fetch(url, { redirect: "follow" });
      if (!response.ok) throw new Error(`download failed (${response.status}): ${url}`);
      const temporary = `${destination}.tmp`;
      await writeFile(temporary, Buffer.from(await response.arrayBuffer()));
      const actual = await sha256(temporary);
      if (actual !== expectedSha256) {
        await rm(temporary, { force: true });
        throw new Error(`SHA-256 mismatch for ${url}: expected ${expectedSha256}, got ${actual}`);
      }
      await rename(temporary, destination);
      return;
    } catch (error) {
      lastError = error;
      await rm(`${destination}.tmp`, { force: true });
      if (attempt < 3) await new Promise((resolve) => setTimeout(resolve, attempt * 1000));
    }
  }
  throw lastError;
}

if (process.platform !== "linux" || process.arch !== "x64") {
  throw new Error(`unsupported contract tool platform: ${process.platform}/${process.arch}`);
}
if (!satisfiesRange(process.versions.node, lock.node)) {
  throw new Error(`Node.js in range "${lock.node}" is required, current version is ${process.versions.node}`);
}

await mkdir(binDirectory, { recursive: true });
await mkdir(downloadsDirectory, { recursive: true });

const goArchive = path.join(downloadsDirectory, `go${lock.go.version}.linux-amd64.tar.gz`);
const goRoot = path.join(toolsRoot, "go");
const goBinary = path.join(goRoot, "bin/go");
if (!(await exists(goBinary)) || !run(goBinary, ["version"], { capture: true }).includes(`go${lock.go.version}`)) {
  await download(lock.go["linux-x64"].url, goArchive, lock.go["linux-x64"].sha256);
  await rm(goRoot, { recursive: true, force: true });
  run("tar", ["-xzf", goArchive, "-C", toolsRoot]);
}

const bufBinary = path.join(binDirectory, "buf");
await download(lock.buf["linux-x64"].url, bufBinary, lock.buf["linux-x64"].sha256);
await chmod(bufBinary, 0o755);

const goEnvironment = {
  GOBIN: binDirectory,
  GOCACHE: path.join(toolsRoot, "gocache"),
  GOPATH: path.join(toolsRoot, "gopath"),
  GOROOT: goRoot,
  GOMODCACHE: path.join(toolsRoot, "gomodcache"),
  GOTOOLCHAIN: "local",
};
const goToolNames = {
  "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen": "oapi-codegen",
  "google.golang.org/protobuf/cmd/protoc-gen-go": "protoc-gen-go",
  "github.com/oasdiff/oasdiff": "oasdiff",
};
for (const [module, tool] of Object.entries(lock.goTools)) {
  const binary = path.join(binDirectory, goToolNames[module]);
  if (!(await exists(binary))) {
    if (offline) throw new Error(`offline cache missing: ${path.relative(repositoryRoot, binary)}`);
    run(goBinary, ["install", `${module}@${tool.version}`], { env: goEnvironment });
  }
}

const npmVersion = run("npm", ["--version"], { capture: true });
if (!satisfiesRange(npmVersion, lock.npm)) {
  throw new Error(`npm in range "${lock.npm}" is required, current version is ${npmVersion}`);
}
const packageLock = path.join(repositoryRoot, "package-lock.json");
const npmMarker = path.join(toolsRoot, "npm-lock.sha256");
const packageLockSha = await sha256(packageLock);
const installedLockSha = await exists(npmMarker) ? (await readFile(npmMarker, "utf8")).trim() : "";
if (!(await exists(path.join(repositoryRoot, "node_modules"))) || installedLockSha !== packageLockSha) {
  if (offline) throw new Error("offline cache missing or package-lock.json does not match node_modules");
  run("npm", ["ci", "--ignore-scripts"]);
  await writeFile(npmMarker, `${packageLockSha}\n`);
}

const versions = {
  node: process.versions.node,
  npm: npmVersion,
  go: run(goBinary, ["version"], { capture: true }),
  buf: run(bufBinary, ["--version"], { capture: true }),
  oapiCodegen: run(path.join(binDirectory, "oapi-codegen"), ["-version"], { capture: true }),
  protocGenGo: run(path.join(binDirectory, "protoc-gen-go"), ["--version"], { capture: true }),
  oasdiff: `${lock.goTools["github.com/oasdiff/oasdiff"].version} (${run(path.join(binDirectory, "oasdiff"), ["--version"], { capture: true })})`,
};
await writeFile(path.join(toolsRoot, "versions.json"), `${JSON.stringify(versions, null, 2)}\n`);
console.log(`Contract tools ready in ${path.relative(repositoryRoot, toolsRoot)}`);
console.log(JSON.stringify(versions));
