#!/usr/bin/env node

import { mkdtemp, readdir, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

import {
  jsonSchemaBreakingChanges,
  scanForbiddenFields,
  scanForbiddenProtoFields,
  scanRuntimeIntentExecutionFields,
  validateWriteHeaders,
} from "./contract-rules.mjs";

const repositoryRoot = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const toolsRoot = path.join(repositoryRoot, ".tools/contracts");
const binDirectory = path.join(toolsRoot, "bin");
const startedAt = performance.now();
const stageTimings = new Map();

class ContractGateError extends Error {
  constructor(category, stage, cause) {
    super(cause instanceof Error ? cause.message : String(cause), { cause });
    this.category = category;
    this.stage = stage;
  }
}

async function stage(name, category, action) {
  const stageStartedAt = performance.now();
  try {
    return await action();
  } catch (error) {
    if (error instanceof ContractGateError) throw error;
    throw new ContractGateError(category, name, error);
  } finally {
    stageTimings.set(name, Math.round(performance.now() - stageStartedAt));
  }
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? repositoryRoot,
    encoding: "utf8",
    stdio: options.capture ? "pipe" : "inherit",
    env: { ...process.env, ...options.env },
  });
  if (result.error) throw result.error;
  if (result.status !== 0 && !options.allowFailure) {
    throw new Error(`${command} ${args.join(" ")} exited with ${result.status}`);
  }
  return result;
}

async function filesNamed(directory, nameOrSuffix) {
  const files = [];
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const target = path.join(directory, entry.name);
    if (entry.isDirectory()) files.push(...await filesNamed(target, nameOrSuffix));
    else if (entry.isFile() && (entry.name === nameOrSuffix || entry.name.endsWith(nameOrSuffix))) files.push(target);
  }
  return files.sort();
}

let temporaryDirectory;
try {
  const injectedCategory = process.env.NODE_ENV === "test"
    ? process.env.HNB_CONTRACT_GATE_TEST_FAILURE
    : undefined;
  if (injectedCategory) {
    if (!["environment", "schema", "compatibility", "drift"].includes(injectedCategory)) {
      throw new ContractGateError("environment", "test-fixture", "invalid injected failure category");
    }
    throw new ContractGateError(injectedCategory, "test-fixture", "injected failure");
  }

  await stage("environment", "environment", async () => {
    run(process.execPath, [path.join(repositoryRoot, "scripts/bootstrap-contract-tools.mjs")]);
    temporaryDirectory = await mkdtemp(path.join(toolsRoot, "validate-"));
  });

  let schemas;
  let specifications;
  let proto;
  await stage("schema", "schema", async () => {
    run(path.join(binDirectory, "buf"), ["lint", path.join(repositoryRoot, "contracts/proto")]);

    schemas = await filesNamed(path.join(repositoryRoot, "contracts/schema"), ".schema.json");
    const openapiFiles = await filesNamed(path.join(repositoryRoot, "contracts/openapi"), "openapi.yaml");
    const ajv = new Ajv2020({ allErrors: true, strict: true, strictRequired: false });
    addFormats(ajv);
    const parsedSchemas = await Promise.all(schemas.map(async (schema) => JSON.parse(await readFile(schema, "utf8"))));
    for (const schema of parsedSchemas) ajv.addSchema(schema);
    for (const schema of parsedSchemas) {
      if (!ajv.getSchema(schema.$id)) throw new Error(`JSON Schema did not compile: ${schema.$id}`);
    }
    const envelopeSchema = parsedSchemas.find((schema) => schema.$id.endsWith("/event-envelope.schema.json"));
    const envelopeExample = JSON.parse(await readFile(
      path.join(repositoryRoot, "contracts/schema/examples/event-envelope.valid.json"),
      "utf8",
    ));
    if (!ajv.validate(envelopeSchema.$id, envelopeExample)) {
      throw new Error(`invalid event envelope example: ${ajv.errorsText(ajv.errors)}`);
    }

    specifications = [];
    for (const [index, openapiFile] of openapiFiles.entries()) {
      run(path.join(repositoryRoot, "node_modules/.bin/redocly"), ["lint", openapiFile]);
      const bundledOpenapi = path.join(temporaryDirectory, `openapi-${index}.json`);
      run(path.join(repositoryRoot, "node_modules/.bin/redocly"), [
        "bundle", openapiFile, "--output", bundledOpenapi, "--ext", "json",
      ]);
      const specification = JSON.parse(await readFile(bundledOpenapi, "utf8"));
      validateWriteHeaders(specification);
      specifications.push({ file: openapiFile, specification });
    }

    const forbidden = [];
    const forbiddenRuntimeIntent = [];
    for (const { file, specification } of specifications) {
      const relative = path.relative(repositoryRoot, file);
      forbidden.push(...scanForbiddenFields(specification, relative));
      forbiddenRuntimeIntent.push(...scanRuntimeIntentExecutionFields(specification, relative));
    }
    for (const [index, schema] of schemas.entries()) {
      const relative = path.relative(repositoryRoot, schema);
      forbidden.push(...scanForbiddenFields(parsedSchemas[index], relative));
      forbiddenRuntimeIntent.push(...scanRuntimeIntentExecutionFields(parsedSchemas[index], relative));
    }
    proto = await readFile(path.join(repositoryRoot, "contracts/proto/hnb/contracts/v1/contracts.proto"), "utf8");
    forbidden.push(...scanForbiddenProtoFields(proto));
    if (forbidden.length > 0) throw new Error(`forbidden contract fields: ${forbidden.join(", ")}`);
    if (forbiddenRuntimeIntent.length > 0) {
      throw new Error(`RuntimeIntent execution fields: ${forbiddenRuntimeIntent.join(", ")}`);
    }
  });

  let compatibility = "no-baseline";
  await stage("compatibility", "compatibility", async () => {
    const baseline = run("git", ["cat-file", "-e", "origin/main^{commit}"], {
      capture: true,
      allowFailure: true,
    });
    if (baseline.status === 0) {
      compatibility = "checked";
      const baselineFiles = run("git", [
        "ls-tree", "-r", "--name-only", "origin/main", "--", "contracts/schema", "contracts/openapi",
      ], { capture: true }).stdout.trim().split("\n").filter(Boolean);
      const baselineSchemas = baselineFiles.filter((name) => name.endsWith(".schema.json"));
      const currentSchemaNames = new Set(schemas.map((schema) => path.relative(repositoryRoot, schema).split(path.sep).join("/")));
      const deletedSchemas = baselineSchemas.filter((schema) => !currentSchemaNames.has(schema));
      if (deletedSchemas.length > 0) throw new Error(`baseline JSON schemas deleted: ${deletedSchemas.join(", ")}`);

      for (const { file: openapiFile } of specifications) {
        const relative = path.relative(repositoryRoot, openapiFile).split(path.sep).join("/");
        if (!baselineFiles.includes(relative)) continue;
        const baselineOpenapi = path.join(temporaryDirectory, `baseline-${relative.replaceAll("/", "-")}`);
        const previous = run("git", ["show", `origin/main:${relative}`], { capture: true });
        await writeFile(baselineOpenapi, previous.stdout);
        run(path.join(binDirectory, "oasdiff"), ["breaking", "--fail-on", "ERR", baselineOpenapi, openapiFile]);
      }
      run(path.join(binDirectory, "buf"), [
        "breaking", path.join(repositoryRoot, "contracts/proto"),
        "--against", ".git#ref=origin/main,subdir=contracts/proto",
      ]);
      const jsonBreaking = [];
      for (const schema of schemas) {
        const relative = path.relative(repositoryRoot, schema).split(path.sep).join("/");
        const oldSchema = run("git", ["show", `origin/main:${relative}`], { capture: true, allowFailure: true });
        if (oldSchema.status !== 0) continue;
        jsonBreaking.push(...jsonSchemaBreakingChanges(JSON.parse(oldSchema.stdout), JSON.parse(await readFile(schema, "utf8")), relative));
      }
      if (jsonBreaking.length > 0) throw new Error(`JSON Schema breaking changes: ${jsonBreaking.join("; ")}`);
    }
  });

  await stage("drift", "drift", async () => {
    run(process.execPath, [path.join(repositoryRoot, "scripts/generate-contracts.mjs"), "--check"]);
  });

  const operationCount = specifications.reduce((total, { specification }) => total
    + Object.values(specification.paths ?? {})
      .reduce((count, item) => count + ["get", "post", "put", "patch", "delete"].filter((method) => item[method]).length, 0), 0);
  const messageCount = [...proto.matchAll(/^message\s+\w+/gm)].length;
  const lock = JSON.parse(await readFile(path.join(repositoryRoot, "contracts/toolchain.lock.json"), "utf8"));
  console.log(
    `Contract gate passed: ${operationCount} operations across ${specifications.length} APIs, ${messageCount} messages, `
    + `${schemas.length} JSON schemas, compatibility=${compatibility}, `
    + `${Math.round(performance.now() - startedAt)} ms`,
  );
  console.log(
    `Tool versions: node=${lock.node}, npm=${lock.npm}, go=${lock.go.version}, buf=${lock.buf.version}, `
    + `typescript=${lock.typescript}, redocly=${lock.nodeTools["@redocly/cli"]}, `
    + `oasdiff=${lock.goTools["github.com/oasdiff/oasdiff"].version}`,
  );
  console.log(`Stage timings: ${[...stageTimings].map(([name, duration]) => `${name}=${duration}ms`).join(", ")}`);
} catch (error) {
  const failure = error instanceof ContractGateError
    ? error
    : new ContractGateError("environment", "startup", error);
  console.error(`Contract gate failed [${failure.category}] stage=${failure.stage}: ${failure.message}`);
  process.exitCode = 1;
} finally {
  if (temporaryDirectory) await rm(temporaryDirectory, { recursive: true, force: true });
}
