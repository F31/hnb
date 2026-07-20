import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { cp, mkdtemp, mkdir, readdir, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { replaceDirectoryAtomically } from "./atomic-directory.mjs";
import {
  jsonSchemaBreakingChanges,
  scanForbiddenFields,
  scanForbiddenProtoFields,
  validateWriteHeaders,
} from "./contract-rules.mjs";

const repositoryRoot = path.dirname(path.dirname(fileURLToPath(import.meta.url)));

async function digestPath(target) {
  if ((await stat(target)).isFile()) {
    return createHash("sha256").update(await readFile(target)).digest("hex");
  }
  const files = await listFiles(target);
  return Object.fromEntries(await Promise.all(files.sort().map(async (file) => [
    path.relative(target, file).split(path.sep).join("/"),
    createHash("sha256").update(await readFile(file)).digest("hex"),
  ])));
}

async function copyPath(source, destination) {
  await mkdir(path.dirname(destination), { recursive: true });
  await cp(source, destination, { recursive: true });
}

test("contract gate reports stable failure categories and nonzero exits", () => {
  for (const category of ["environment", "schema", "compatibility", "drift"]) {
    const result = spawnSync(process.execPath, [path.join(repositoryRoot, "scripts/validate-contracts.mjs")], {
      cwd: repositoryRoot,
      encoding: "utf8",
      env: {
        ...process.env,
        NODE_ENV: "test",
        HNB_CONTRACT_GATE_TEST_FAILURE: category,
      },
    });
    assert.equal(result.status, 1);
    assert.equal(
      result.stderr,
      `Contract gate failed [${category}] stage=test-fixture: injected failure\n`,
    );
  }
});

test("contract release rollback restores schemas, locks, configuration, and generated output", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "hnb-contract-rollback-"));
  const releasePaths = [
    "contracts/openapi",
    "contracts/proto",
    "contracts/schema",
    "contracts/toolchain.lock.json",
    "package-lock.json",
    "package.json",
    "redocly.yaml",
    "scripts/generate-contracts.mjs",
    "contracts/generated",
  ];
  try {
    for (const relative of releasePaths) {
      await copyPath(path.join(repositoryRoot, relative), path.join(directory, "snapshot", relative));
      await copyPath(path.join(repositoryRoot, relative), path.join(directory, "working", relative));
    }

    const mutations = [
      "contracts/openapi/foundation/v1/openapi.yaml",
      "contracts/toolchain.lock.json",
      "scripts/generate-contracts.mjs",
      "contracts/generated/TOOLCHAIN.json",
    ];
    for (const relative of mutations) {
      const file = path.join(directory, "working", relative);
      await writeFile(file, `${await readFile(file, "utf8")}\nrollback drill mutation\n`);
    }

    for (const relative of releasePaths) {
      const working = path.join(directory, "working", relative);
      await rm(working, { recursive: true, force: true });
      await copyPath(path.join(directory, "snapshot", relative), working);
      assert.deepEqual(await digestPath(working), await digestPath(path.join(directory, "snapshot", relative)));
    }
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("write operations require correlation and idempotency headers", () => {
  const specification = {
    components: {
      parameters: {
        correlation: { name: "X-Correlation-ID" },
        idempotency: { name: "Idempotency-Key" },
      },
    },
    paths: {
      "/items": {
        post: {
          parameters: [
            { $ref: "#/components/parameters/correlation" },
            { $ref: "#/components/parameters/idempotency" },
          ],
        },
      },
    },
  };

  assert.doesNotThrow(() => validateWriteHeaders(specification));
  specification.paths["/items"].post.parameters.pop();
  assert.throws(() => validateWriteHeaders(specification), /missing Idempotency-Key/);
});

test("updates additionally require If-Match", () => {
  const specification = {
    paths: {
      "/items/{id}": {
        put: {
          parameters: [
            { name: "X-Correlation-ID" },
            { name: "Idempotency-Key" },
          ],
        },
      },
    },
  };

  assert.throws(() => validateWriteHeaders(specification), /missing If-Match/);
});

test("sensitive value fields are rejected while references are allowed", () => {
  assert.deepEqual(scanForbiddenFields({ properties: { secretRef: { type: "string" } } }), []);
  assert.deepEqual(
    scanForbiddenFields({ properties: { accessToken: { type: "string" } } }),
    ["root.properties.accessToken"],
  );
  assert.deepEqual(scanForbiddenProtoFields("string secret_ref = 1;"), []);
  assert.deepEqual(scanForbiddenProtoFields("string private_key = 1;"), ["contracts.proto.private_key"]);
});

test("request context semantics remain aligned across formats", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "hnb-contract-test-"));
  try {
    const bundled = path.join(directory, "openapi.json");
    execFileSync(path.join(repositoryRoot, "node_modules/.bin/redocly"), [
      "bundle",
      path.join(repositoryRoot, "contracts/openapi/foundation/v1/openapi.yaml"),
      "--output", bundled,
      "--ext", "json",
    ], { cwd: repositoryRoot, stdio: "ignore" });
    const openapi = JSON.parse(await readFile(bundled, "utf8"));
    const jsonSchema = JSON.parse(await readFile(
      path.join(repositoryRoot, "contracts/schema/common/v1/request-context.schema.json"),
      "utf8",
    ));
    const proto = await readFile(
      path.join(repositoryRoot, "contracts/proto/hnb/contracts/v1/contracts.proto"),
      "utf8",
    );
    const expectedJson = ["tenantId", "projectId", "environmentId", "actorId", "correlationId", "traceparent"];
    assert.deepEqual(Object.keys(openapi.components.schemas.RequestContext.properties), expectedJson);
    assert.deepEqual(Object.keys(jsonSchema.properties), expectedJson);
    for (const field of ["tenant_id", "project_id", "environment_id", "actor_id", "correlation_id", "traceparent"]) {
      assert.match(proto, new RegExp(`\\b${field}\\s*=`));
    }
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

async function listFiles(directory) {
  const files = [];
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const target = path.join(directory, entry.name);
    if (entry.isDirectory()) files.push(...await listFiles(target));
    else files.push(target);
  }
  return files;
}

test("generated contracts do not import service internals or broker types", async () => {
  const files = await listFiles(path.join(repositoryRoot, "contracts/generated"));
  for (const file of files.filter((name) => /\.(go|ts)$/.test(name))) {
    const content = await readFile(file, "utf8");
    assert.doesNotMatch(
      content,
      /github\.com\/F31\/hnb\/services|\/internal\/|nats\.io|database\/sql/i,
      file,
    );
  }
});

test("atomic generation restores previous output when replacement fails", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "hnb-atomic-test-"));
  try {
    const target = path.join(directory, "generated");
    const missingSource = path.join(directory, "missing");
    const backup = path.join(directory, "backup");
    await mkdir(target);
    await writeFile(path.join(target, "stable.txt"), "stable\n");

    await assert.rejects(() => replaceDirectoryAtomically(missingSource, target, backup));
    assert.equal(await readFile(path.join(target, "stable.txt"), "utf8"), "stable\n");
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("JSON Schema compatibility rejects same-major breaking changes", () => {
  const previous = {
    $id: "https://schemas.hnb.cloud/common/v1/test.schema.json",
    type: "object",
    properties: { id: { type: "string" }, state: { type: "string", enum: ["A", "B"] } },
  };
  const current = {
    $id: previous.$id,
    type: "object",
    required: ["newField"],
    properties: { state: { type: "integer", enum: ["A"] }, newField: { type: "string" } },
  };

  const changes = jsonSchemaBreakingChanges(previous, current);
  assert.ok(changes.some((change) => change.includes("id: property removed")));
  assert.ok(changes.some((change) => change.includes("state: type changed")));
  assert.ok(changes.some((change) => change.includes("newField: became required")));
  assert.ok(changes.some((change) => change.includes("enum value removed: B")));
});

test("JSON Schema compatibility permits an explicit major version change", () => {
  const previous = { $id: "https://schemas.hnb.cloud/common/v1/test.schema.json", type: "string" };
  const current = { $id: "https://schemas.hnb.cloud/common/v2/test.schema.json", type: "integer" };
  assert.deepEqual(jsonSchemaBreakingChanges(previous, current), []);
});

test("oasdiff and Buf reject same-version field removal", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "hnb-breaking-test-"));
  try {
    const oldOpenapi = path.join(directory, "old.json");
    const newOpenapi = path.join(directory, "new.json");
    const api = (includeId) => ({
      openapi: "3.1.0",
      info: { title: "test", version: "1.0.0" },
      paths: {
        "/items": {
          get: {
            responses: {
              200: {
                description: "ok",
                content: {
                  "application/json": {
                    schema: {
                      type: "object",
                      required: includeId ? ["id"] : [],
                      properties: includeId ? { id: { type: "string" } } : {},
                    },
                  },
                },
              },
            },
          },
        },
      },
    });
    await writeFile(oldOpenapi, JSON.stringify(api(true)));
    await writeFile(newOpenapi, JSON.stringify(api(false)));
    const oasdiff = spawnSync(path.join(repositoryRoot, ".tools/contracts/bin/oasdiff"), [
      "breaking", "--fail-on", "ERR", oldOpenapi, newOpenapi,
    ], { encoding: "utf8" });
    assert.notEqual(oasdiff.status, 0, `${oasdiff.stdout}\n${oasdiff.stderr}`);

    for (const side of ["old", "new"]) {
      const root = path.join(directory, side);
      await mkdir(root);
      await writeFile(path.join(root, "buf.yaml"), "version: v2\n");
      const field = side === "old" ? "  string id = 1;\n" : "";
      await writeFile(path.join(root, "contract.proto"), `syntax = "proto3";\npackage test.v1;\nmessage Item {\n${field}}\n`);
    }
    const buf = spawnSync(path.join(repositoryRoot, ".tools/contracts/bin/buf"), [
      "breaking", path.join(directory, "new"), "--against", path.join(directory, "old"),
    ], { encoding: "utf8" });
    assert.notEqual(buf.status, 0, `${buf.stdout}\n${buf.stderr}`);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("Outbox records map to stable broker-neutral envelope fields", async () => {
  const mapping = JSON.parse(await readFile(
    path.join(repositoryRoot, "contracts/mappings/outbox-event-envelope.json"),
    "utf8",
  ));
  const record = {
    eventId: "018f6c2a-4a64-7b58-9cc3-9f70462f36c1",
    eventType: "hnb.event.contract.echoed.v1",
    schemaVersion: "1.0.0",
    occurredAt: "2026-07-20T12:00:00Z",
    tenantId: "tenant-a",
    correlationId: "018f6c2a-4a64-7b58-9cc3-9f70462f36c2",
    idempotencyKey: "contract-echo-001",
    payload: { value: "contract fixture" },
  };
  const envelope = Object.fromEntries(
    Object.entries(mapping.fields)
      .filter(([, source]) => record[source] !== undefined)
      .map(([target, source]) => [target, record[source]]),
  );

  assert.equal(envelope.messageId, record.eventId);
  assert.equal(envelope.correlationId, record.correlationId);
  assert.equal(envelope.idempotencyKey, record.idempotencyKey);
  assert.deepEqual(envelope.payload, record.payload);
  assert.equal("subject" in envelope, false);
  assert.equal("streamSequence" in envelope, false);
});

test("Go-produced envelopes decode in TypeScript and preserve unknown fields", () => {
  const tools = path.join(repositoryRoot, ".tools/contracts");
  const encoded = execFileSync(path.join(tools, "go/bin/go"), [
    "run", path.join(repositoryRoot, "contracts/tests/interop/encode.go"),
  ], {
    cwd: path.join(repositoryRoot, "contracts/generated/go"),
    encoding: "utf8",
    env: {
      ...process.env,
      GOCACHE: path.join(tools, "gocache"),
      GOPATH: path.join(tools, "gopath"),
      GOROOT: path.join(tools, "go"),
      GOMODCACHE: path.join(tools, "gomodcache"),
      GOTOOLCHAIN: "local",
    },
  });
  execFileSync(path.join(repositoryRoot, "node_modules/.bin/tsx"), [
    path.join(repositoryRoot, "contracts/tests/interop/decode.ts"), encoded,
  ], { cwd: repositoryRoot, stdio: "inherit" });
});
