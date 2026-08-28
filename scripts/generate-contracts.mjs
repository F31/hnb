#!/usr/bin/env node

import { createHash } from "node:crypto";
import { cp, mkdtemp, mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
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
const clusterManagementOnly = process.argv.includes("--cluster-management");

const clusterManagementOutputs = [
  "go/openapi/console",
  "go/openapi/runtime-target",
  "typescript/console-openapi",
  "typescript/runtime-target-openapi",
  "typescript/package.json",
  "typescript/tsconfig.json",
];

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
  GOWORK: "off",
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

function isClusterManagementOutput(name) {
  return clusterManagementOutputs.some((output) => name === output || name.startsWith(`${output}/`));
}

async function replaceClusterManagementOutputs() {
  for (const relative of clusterManagementOutputs) {
    const source = path.join(temporaryGenerated, relative);
    const destination = path.join(generatedDirectory, relative);
    await rm(destination, { recursive: true, force: true });
    await mkdir(path.dirname(destination), { recursive: true });
    await cp(source, destination, { recursive: true });
  }
}

async function normalizeScalarTypesForCodegen(file) {
  const document = JSON.parse(await readFile(file, "utf8"));
  const visit = (value) => {
    if (Array.isArray(value)) {
      value.forEach(visit);
      return;
    }
    if (!value || typeof value !== "object") return;
    if (!value.type && Object.hasOwn(value, "const")) value.type = typeof value.const;
    if (!value.type && Array.isArray(value.enum) && value.enum.length > 0) {
      const types = new Set(value.enum.map((entry) => typeof entry));
      if (types.size === 1) value.type = [...types][0];
    }
    Object.values(value).forEach(visit);
  };
  visit(document);
  await writeFile(file, `${JSON.stringify(document, null, 2)}\n`);
}

try {
  await mkdir(path.join(goOutput, "openapi"), { recursive: true });
  await mkdir(path.join(goOutput, "openapi/console"), { recursive: true });
  await mkdir(path.join(goOutput, "openapi/platform"), { recursive: true });
  await mkdir(path.join(goOutput, "openapi/runtime-target"), { recursive: true });
  await mkdir(path.join(goOutput, "openapi/storage"), { recursive: true });
  await mkdir(path.join(goOutput, "proto"), { recursive: true });
  await mkdir(path.join(typeScriptOutput, "openapi"), { recursive: true });
  await mkdir(path.join(typeScriptOutput, "console-openapi"), { recursive: true });
  await mkdir(path.join(typeScriptOutput, "proto"), { recursive: true });
  await mkdir(path.join(typeScriptOutput, "platform-openapi"), { recursive: true });
  await mkdir(path.join(typeScriptOutput, "runtime-target-openapi"), { recursive: true });
  await mkdir(path.join(typeScriptOutput, "storage-openapi"), { recursive: true });

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

  const storageBundle = path.join(temporaryRoot, "storage.openapi.json");
  run(path.join(repositoryRoot, "node_modules/.bin/redocly"), [
    "bundle",
    path.join(repositoryRoot, "contracts/openapi/storage/v1/openapi.yaml"),
    "--output", storageBundle,
    "--ext", "json",
  ]);
  await normalizeScalarTypesForCodegen(storageBundle);

  run(path.join(binDirectory, "oapi-codegen"), [
    "-generate", "types,client,skip-prune",
    "-package", "storage",
    "-o", path.join(goOutput, "openapi/storage/storage.gen.go"),
    storageBundle,
  ]);

  run(process.execPath, [
    path.join(repositoryRoot, "node_modules/openapi-typescript-codegen/bin/index.js"),
    "--input", storageBundle,
    "--output", path.join(typeScriptOutput, "storage-openapi"),
    "--client", "fetch",
    "--useOptions",
    "--useUnionTypes",
  ]);

  const consoleBundle = path.join(temporaryRoot, "console.openapi.json");
  run(path.join(repositoryRoot, "node_modules/.bin/redocly"), [
    "bundle",
    path.join(repositoryRoot, "contracts/openapi/console/v1/openapi.yaml"),
    "--output", consoleBundle,
    "--ext", "json",
  ]);
  await normalizeScalarTypesForCodegen(consoleBundle);

  run(path.join(binDirectory, "oapi-codegen"), [
    "-generate", "types,client",
    "-package", "console",
    "-o", path.join(goOutput, "openapi/console/console.gen.go"),
    consoleBundle,
  ]);

  run(process.execPath, [
    path.join(repositoryRoot, "node_modules/openapi-typescript-codegen/bin/index.js"),
    "--input", consoleBundle,
    "--output", path.join(typeScriptOutput, "console-openapi"),
    "--client", "fetch",
    "--useOptions",
    "--useUnionTypes",
  ]);

  const runtimeTargetSchemaDirectory = path.join(repositoryRoot, "contracts/schema/runtime-target/v1");
  const runtimeTargetRegistry = JSON.parse(await readFile(
    path.join(runtimeTargetSchemaDirectory, "runtime-target-registry.json"),
    "utf8",
  ));
  const runtimeTargetSchemas = {};
  for (const entry of Object.values(runtimeTargetRegistry.schemas)) {
    const schemaFile = path.join(runtimeTargetSchemaDirectory, entry.$ref);
    const schema = JSON.parse(await readFile(schemaFile, "utf8"));
    if (!schema.title) throw new Error(`runtime-target schema has no title: ${entry.$ref}`);
    runtimeTargetSchemas[schema.title] = {
      $ref: path.relative(temporaryRoot, schemaFile).split(path.sep).join("/"),
    };
  }
  const runtimeTargetOpenapi = path.join(temporaryRoot, "runtime-target.openapi.source.json");
  await writeFile(runtimeTargetOpenapi, `${JSON.stringify({
    openapi: "3.1.0",
    info: {
      title: "HNB RuntimeTarget JSON Schema Types",
      version: runtimeTargetRegistry.version,
    },
    paths: {},
    components: { schemas: runtimeTargetSchemas },
  }, null, 2)}\n`);
  const runtimeTargetBundle = path.join(temporaryRoot, "runtime-target.openapi.json");
  run(path.join(repositoryRoot, "node_modules/.bin/redocly"), [
    "bundle", runtimeTargetOpenapi,
    "--output", runtimeTargetBundle,
    "--ext", "json",
  ]);
  await normalizeScalarTypesForCodegen(runtimeTargetBundle);

  run(path.join(binDirectory, "oapi-codegen"), [
    "-generate", "types,skip-prune",
    "-package", "runtimetarget",
    "-o", path.join(goOutput, "openapi/runtime-target/runtime_target.gen.go"),
    runtimeTargetBundle,
  ]);

  run(process.execPath, [
    path.join(repositoryRoot, "node_modules/openapi-typescript-codegen/bin/index.js"),
    "--input", runtimeTargetBundle,
    "--output", path.join(typeScriptOutput, "runtime-target-openapi"),
    "--client", "fetch",
    "--useOptions",
    "--useUnionTypes",
  ]);

  const platformBundle = path.join(temporaryRoot, "platform.openapi.json");
  run(path.join(repositoryRoot, "node_modules/.bin/redocly"), [
    "bundle",
    path.join(repositoryRoot, "contracts/openapi/platform/v1/openapi.yaml"),
    "--output", platformBundle,
    "--ext", "json",
  ]);

  run(path.join(binDirectory, "oapi-codegen"), [
    "-generate", "types,client",
    "-package", "platform",
    "-o", path.join(goOutput, "openapi/platform/platform.gen.go"),
    platformBundle,
  ]);

  run(process.execPath, [
    path.join(repositoryRoot, "node_modules/openapi-typescript-codegen/bin/index.js"),
    "--input", platformBundle,
    "--output", path.join(typeScriptOutput, "platform-openapi"),
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
    exports: {
      "./console": "./console-openapi/index.ts",
      "./foundation": "./openapi/index.ts",
      "./platform": "./platform-openapi/index.ts",
      "./runtime-target": "./runtime-target-openapi/index.ts",
      "./storage": "./storage-openapi/index.ts",
    },
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
    include: [
      "console-openapi/**/*.ts",
      "openapi/**/*.ts",
      "platform-openapi/**/*.ts",
      "proto/**/*.ts",
      "runtime-target-openapi/**/*.ts",
      "storage-openapi/**/*.ts",
    ],
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
    const relevantChanges = clusterManagementOnly ? changed.filter(isClusterManagementOutput) : changed;
    if (relevantChanges.length > 0) {
      throw new Error(`generated contract drift: ${relevantChanges.join(", ")}`);
    }
    console.log(clusterManagementOnly
      ? "Generated cluster-management contracts match committed output"
      : "Generated contracts match committed output");
  } else if (clusterManagementOnly) {
    await replaceClusterManagementOutputs();
    console.log("Generated cluster-management contracts in contracts/generated");
  } else {
    const previousDirectory = `${generatedDirectory}.previous-${process.pid}`;
    await replaceDirectoryAtomically(temporaryGenerated, generatedDirectory, previousDirectory);
    console.log(`Generated contracts in ${path.relative(repositoryRoot, generatedDirectory)}`);
  }
} finally {
  await rm(temporaryRoot, { recursive: true, force: true });
}
