#!/usr/bin/env node

import { createHash } from "node:crypto";
import { mkdtemp, mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

import { replaceDirectoryAtomically } from "./atomic-directory.mjs";

const repositoryRoot = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const toolsRoot = path.join(repositoryRoot, ".tools/contracts");
const binDirectory = path.join(toolsRoot, "bin");
const goRoot = path.join(toolsRoot, "go");
const generatedDirectory = path.join(repositoryRoot, "contracts/generated");
const temporaryRoot = await mkdtemp(path.join(repositoryRoot, ".tools/contracts/generate-"));
const temporaryGenerated = path.join(temporaryRoot, "generated");
const goOutput = path.join(temporaryGenerated, "go");
const typeScriptOutput = path.join(temporaryGenerated, "typescript");
const checkOnly = process.argv.includes("--check");

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? repositoryRoot,
    encoding: "utf8",
    stdio: "inherit",
    env: { ...process.env, ...options.env },
  });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${command} ${args.join(" ")} exited with ${result.status}`);
}

const goEnvironment = {
  GOCACHE: path.join(toolsRoot, "gocache"),
  GOPATH: path.join(toolsRoot, "gopath"),
  GOROOT: goRoot,
  GOMODCACHE: path.join(toolsRoot, "gomodcache"),
  GOTOOLCHAIN: "local",
  ...(process.env.HNB_CONTRACT_TOOLS_OFFLINE === "1" ? { GOPROXY: "off" } : {}),
};

async function snapshot(directory, prefix = "") {
  const files = new Map();
  let entries;
  try {
    entries = await readdir(directory, { withFileTypes: true });
  } catch (error) {
    if (error.code === "ENOENT") return files;
    throw error;
  }
  for (const entry of entries.sort((left, right) => left.name.localeCompare(right.name))) {
    const relative = prefix ? `${prefix}/${entry.name}` : entry.name;
    const absolute = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      for (const [name, digest] of await snapshot(absolute, relative)) files.set(name, digest);
    } else if (entry.isFile()) {
      files.set(relative, createHash("sha256").update(await readFile(absolute)).digest("hex"));
    }
  }
  return files;
}

async function normalizeFinalNewlines(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const target = path.join(directory, entry.name);
    if (entry.isDirectory()) await normalizeFinalNewlines(target);
    else if (entry.isFile() && /\.(go|json|ts)$/.test(entry.name)) {
      const content = await readFile(target, "utf8");
      await writeFile(target, `${content.replace(/\n+$/u, "")}\n`);
    }
  }
}

try {
  await mkdir(path.join(goOutput, "openapi"), { recursive: true });
  await mkdir(path.join(goOutput, "proto"), { recursive: true });
  await mkdir(path.join(typeScriptOutput, "openapi"), { recursive: true });
  await mkdir(path.join(typeScriptOutput, "proto"), { recursive: true });

  run(path.join(binDirectory, "oapi-codegen"), [
    "-generate", "types,client",
    "-package", "foundation",
    "-o", path.join(goOutput, "openapi/foundation.gen.go"),
    path.join(repositoryRoot, "contracts/openapi/foundation/v1/openapi.yaml"),
  ]);

  run(process.execPath, [
    path.join(repositoryRoot, "node_modules/openapi-typescript-codegen/bin/index.js"),
    "--input", path.join(repositoryRoot, "contracts/openapi/foundation/v1/openapi.yaml"),
    "--output", path.join(typeScriptOutput, "openapi"),
    "--client", "fetch",
    "--useOptions",
    "--useUnionTypes",
  ]);

  const bufTemplate = path.join(temporaryRoot, "buf.gen.yaml");
  await writeFile(bufTemplate, `version: v2
plugins:
  - local: ${path.join(binDirectory, "protoc-gen-go")}
    out: ${path.join(goOutput, "proto")}
    opt:
      - paths=source_relative
  - local: ${path.join(repositoryRoot, "node_modules/.bin/protoc-gen-es")}
    out: ${path.join(typeScriptOutput, "proto")}
    opt:
      - target=ts
`);
  run(path.join(binDirectory, "buf"), [
    "generate",
    path.join(repositoryRoot, "contracts/proto"),
    "--template", bufTemplate,
  ]);

  await writeFile(path.join(goOutput, "go.mod"), `module github.com/F31/hnb/contracts/generated/go

go 1.26.5

require (
	github.com/oapi-codegen/runtime v1.6.0
	google.golang.org/protobuf v1.36.11
)
`);
  run(path.join(goRoot, "bin/go"), ["mod", "tidy"], { cwd: goOutput, env: goEnvironment });
  run(path.join(goRoot, "bin/go"), ["fmt", "./..."], { cwd: goOutput, env: goEnvironment });
  run(path.join(goRoot, "bin/go"), ["test", "./..."], { cwd: goOutput, env: goEnvironment });

  await writeFile(path.join(typeScriptOutput, "package.json"), `${JSON.stringify({
    name: "@hnb/contracts",
    version: "0.1.0",
    private: true,
    type: "module",
    dependencies: { "@bufbuild/protobuf": "2.12.1" },
    devDependencies: { typescript: "7.0.2" },
  }, null, 2)}\n`);
  await writeFile(path.join(typeScriptOutput, "tsconfig.json"), `${JSON.stringify({
    compilerOptions: {
      strict: true,
      noEmit: true,
      target: "ES2022",
      module: "ESNext",
      moduleResolution: "Bundler",
      lib: ["ES2022", "DOM", "DOM.Iterable"],
      skipLibCheck: false,
    },
    include: ["openapi/**/*.ts", "proto/**/*.ts"],
  }, null, 2)}\n`);
  await normalizeFinalNewlines(temporaryGenerated);
  run(path.join(repositoryRoot, "node_modules/.bin/tsc"), [
    "--project", path.join(typeScriptOutput, "tsconfig.json"),
  ]);

  const versions = JSON.parse(await readFile(path.join(toolsRoot, "versions.json"), "utf8"));
  await writeFile(path.join(temporaryGenerated, "TOOLCHAIN.json"), `${JSON.stringify(versions, null, 2)}\n`);

  if (checkOnly) {
    const expected = await snapshot(generatedDirectory);
    const actual = await snapshot(temporaryGenerated);
    const names = new Set([...expected.keys(), ...actual.keys()]);
    const changed = [...names].filter((name) => expected.get(name) !== actual.get(name)).sort();
    if (changed.length > 0) {
      throw new Error(`generated contract drift: ${changed.join(", ")}`);
    }
    console.log("Generated contracts match committed output");
  } else {
    const previousDirectory = `${generatedDirectory}.previous-${process.pid}`;
    await replaceDirectoryAtomically(temporaryGenerated, generatedDirectory, previousDirectory);
    console.log(`Generated contracts in ${path.relative(repositoryRoot, generatedDirectory)}`);
  }
} finally {
  await rm(temporaryRoot, { recursive: true, force: true });
}
