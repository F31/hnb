import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { cp, mkdtemp, mkdir, readdir, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

import { replaceDirectoryAtomically } from "./atomic-directory.mjs";
import {
  jsonSchemaBreakingChanges,
  scanForbiddenFields,
  scanForbiddenProtoFields,
  scanRuntimeIntentExecutionFields,
  validateClusterDictionaries,
  validateLifecycleCompatibilityMatrix,
  validateRuntimeTargetObservationSemantics,
  validateRuntimeTargetSourceResetSemantics,
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

test("RuntimeIntent rejects caller-authored execution fields without applying the rule to ExecutionPlan", () => {
  const contracts = {
    components: {
      schemas: {
        RuntimeIntent: {
          type: "object",
          properties: {
            spec: {
              type: "object",
              properties: {
                providerCommands: { type: "array" },
                providerId: { type: "string" },
                stepType: { type: "string" },
                callbackUrl: { type: "string" },
              },
            },
          },
        },
        ExecutionPlan: {
          type: "object",
          properties: { steps: { type: "array" } },
        },
      },
    },
  };
  assert.deepEqual(
    scanRuntimeIntentExecutionFields(contracts),
    [
      "root.components.schemas.RuntimeIntent.properties.spec.properties.providerCommands",
      "root.components.schemas.RuntimeIntent.properties.spec.properties.providerId",
      "root.components.schemas.RuntimeIntent.properties.spec.properties.stepType",
      "root.components.schemas.RuntimeIntent.properties.spec.properties.callbackUrl",
    ],
  );
});

test("RuntimeIntent fixtures accept references and reject caller-authored steps", async () => {
  const ajv = new Ajv2020({ allErrors: true, strict: true, strictRequired: false });
  addFormats(ajv);
  const schemaDirectory = path.join(repositoryRoot, "contracts/schema");
  const schemas = (await listFiles(schemaDirectory)).filter((file) => file.endsWith(".schema.json"));
  for (const file of schemas) ajv.addSchema(JSON.parse(await readFile(file, "utf8")));
  const validate = ajv.getSchema("https://schemas.hnb.cloud/platform/v1/runtime-intent.schema.json");
  assert.ok(validate);
  const valid = JSON.parse(await readFile(path.join(schemaDirectory, "examples/runtime-intent.valid.json"), "utf8"));
  const invalid = JSON.parse(await readFile(path.join(schemaDirectory, "examples/runtime-intent.invalid.json"), "utf8"));
  assert.equal(validate(valid), true, ajv.errorsText(validate.errors));
  assert.equal(validate(invalid), false);
});

test("Console OpenAPI fixes cluster, RuntimeIntent, Operation, pagination, and Problem Details invariants", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "hnb-console-openapi-test-"));
  try {
    const bundled = path.join(directory, "openapi.json");
    execFileSync(path.join(repositoryRoot, "node_modules/.bin/redocly"), [
      "bundle",
      path.join(repositoryRoot, "contracts/openapi/console/v1/openapi.yaml"),
      "--output", bundled,
      "--ext", "json",
    ], { cwd: repositoryRoot, stdio: "ignore" });
    const specification = JSON.parse(await readFile(bundled, "utf8"));
    const schemas = specification.components.schemas;

    assert.deepEqual(schemas.TargetKind.enum, ["KubernetesTarget", "EdgeRuntimeTarget"]);
    assert.deepEqual(Object.keys(schemas.StateDimensions.properties), [
      "lifecycleState", "healthState", "connectivityState", "freshness", "observedAt", "lastKnownStateAt",
    ]);
    assert.equal(specification.components.parameters.ClusterPageSize.schema.default, 20);
    assert.equal(specification.components.parameters.ClusterPageSize.schema.maximum, 100);
    assert.equal(specification.components.parameters.NodePageSize.schema.default, 50);
    assert.equal(specification.components.parameters.NodePageSize.schema.maximum, 200);
    assert.equal(schemas.PageMetadata.properties.exactTotal.const, true);
    assert.match(specification.paths["/api/v1/resources/clusters"].get.description, /updatedAt DESC, targetId DESC/);
    assert.ok(specification.paths["/api/v1/operations/{operationId}/actions/approve"].post);
    assert.ok(specification.paths["/api/v1/operations/{operationId}/actions/reject"].post);
    assert.ok(specification.paths["/api/v1/operations/{operationId}/actions/cancel"].post);
    assert.deepEqual(schemas.ProblemDetails.required, ["type", "title", "status", "code", "correlationId", "traceId"]);
    assert.doesNotThrow(() => validateWriteHeaders(specification));
    assert.deepEqual(scanRuntimeIntentExecutionFields(specification), []);

    const ajv = new Ajv2020({ allErrors: true, strict: false });
    addFormats(ajv);
    const schemaRoot = "https://schemas.hnb.cloud/console/v1/openapi-components.json";
    ajv.addSchema({ $id: schemaRoot, components: specification.components });
    const examples = [
      ["ClusterListResponse", "console-cluster-list"],
      ["ClusterRuntimeIntent", "console-runtime-intent"],
      ["OperationDetailResponse", "console-operation"],
    ];
    for (const [schemaName, fixtureName] of examples) {
      const validate = ajv.getSchema(`${schemaRoot}#/components/schemas/${schemaName}`);
      assert.ok(validate, schemaName);
      const valid = JSON.parse(await readFile(path.join(repositoryRoot, `contracts/schema/examples/${fixtureName}.valid.json`), "utf8"));
      const invalid = JSON.parse(await readFile(path.join(repositoryRoot, `contracts/schema/examples/${fixtureName}.invalid.json`), "utf8"));
      assert.equal(validate(valid), true, `${schemaName}: ${ajv.errorsText(validate.errors)}`);
      assert.equal(validate(invalid), false, `${schemaName} accepted its invalid fixture`);
    }
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("RuntimeTarget schema examples and semantic ordering boundaries are enforced", async () => {
  const ajv = new Ajv2020({ allErrors: true, strict: true, strictRequired: false });
  addFormats(ajv);
  const schemaDirectory = path.join(repositoryRoot, "contracts/schema");
  const schemaFiles = (await listFiles(schemaDirectory)).filter((file) => file.endsWith(".schema.json"));
  for (const file of schemaFiles) ajv.addSchema(JSON.parse(await readFile(file, "utf8")));

  const structuralExamples = [
    ["https://schemas.hnb.cloud/runtime-target/v1/runtime-target-observation.schema.json", "runtime-target-observation"],
    ["https://schemas.hnb.cloud/runtime-target/v1/node-section.schema.json", "runtime-target-node-section"],
    ["https://schemas.hnb.cloud/runtime-target/v1/storage-inventory-section.schema.json", "runtime-target-storage-inventory-section"],
    ["https://schemas.hnb.cloud/runtime-target/v1/capability-section.schema.json", "runtime-target-capability-section"],
    ["https://schemas.hnb.cloud/runtime-target/v1/kubernetes-lifecycle-step-input.schema.json", "kubernetes-lifecycle-step-input"],
    ["https://schemas.hnb.cloud/runtime-target/v1/edge-lifecycle-step-input.schema.json", "edge-lifecycle-step-input"],
    ["https://schemas.hnb.cloud/console/v1/dictionary-entry.schema.json", "cluster-dictionary-entry"],
    ["https://schemas.hnb.cloud/gslb/v1/gslb-intent.schema.json", "gslb-intent"],
    ["https://schemas.hnb.cloud/gslb/v1/gslb-service.schema.json", "gslb-service"],
    ["https://schemas.hnb.cloud/dr/v1/dr-protection-group.schema.json", "dr-protection-group"],
    ["https://schemas.hnb.cloud/dr/v1/dr-group-member.schema.json", "dr-group-member"],
    ["https://schemas.hnb.cloud/dr/v1/dr-switch-run.schema.json", "dr-switch-run"],
  ];
  for (const [schemaId, fixtureName] of structuralExamples) {
    const validate = ajv.getSchema(schemaId);
    const valid = JSON.parse(await readFile(path.join(schemaDirectory, `examples/${fixtureName}.valid.json`), "utf8"));
    const invalid = JSON.parse(await readFile(path.join(schemaDirectory, `examples/${fixtureName}.invalid.json`), "utf8"));
    assert.equal(validate(valid), true, `${fixtureName}: ${ajv.errorsText(validate.errors)}`);
    assert.equal(validate(invalid), false, `${fixtureName} accepted its invalid fixture`);
  }

  const observation = JSON.parse(await readFile(path.join(schemaDirectory, "examples/runtime-target-observation.valid.json"), "utf8"));
  const identity = Object.fromEntries([
    "tenantId", "targetId", "targetKind", "observerId", "observerKind", "observerGeneration",
  ].map((field) => [field, observation[field]]));
  const cursor = { observerGeneration: 4, sequence: 20, eventId: "018f6c2a-4a64-7b58-9cc3-9f70462f3799" };
  const now = new Date("2026-08-01T12:01:00Z");
  assert.doesNotThrow(() => validateRuntimeTargetObservationSemantics(observation, { identity, cursor, now }));
  assert.throws(() => validateRuntimeTargetObservationSemantics(
    { ...observation, tenantId: "tenant-b" }, { identity, cursor, now },
  ), /tenantId does not match/);
  assert.throws(() => validateRuntimeTargetObservationSemantics(
    { ...observation, sequence: 19 }, { identity, cursor, now },
  ), /sequence must be contiguous/);
  assert.throws(() => validateRuntimeTargetObservationSemantics(
    { ...observation, sequence: 22 }, { identity, cursor, now },
  ), /sequence must be contiguous/);
  assert.throws(() => validateRuntimeTargetObservationSemantics(
    { ...observation, observedAt: "2026-08-01T12:06:01Z" }, { identity, cursor, now },
  ), /future skew/);
  assert.throws(() => validateRuntimeTargetObservationSemantics(
    { ...observation, oversizedFixture: "x".repeat(1_048_576) }, { identity, cursor, now },
  ), /encoded payload exceeds/);

  const delta = structuredClone(observation);
  delta.inventoryMode = "Delta";
  delta.sequence = 22;
  delta.storageInventory = {
    storageClasses: [{
      uid: "storage-class-uid-1", resourceVersion: "1843", name: "fast",
      source: "kubernetes.storage.k8s.io/v1", observedAt: "2026-08-01T12:00:00Z", deleted: true,
    }],
  };
  assert.equal(ajv.getSchema("https://schemas.hnb.cloud/runtime-target/v1/runtime-target-observation.schema.json")(delta), true);
  assert.doesNotThrow(() => validateRuntimeTargetObservationSemantics(delta, {
    identity, cursor: { observerGeneration: 4, sequence: 21, eventId: "018f6c2a-4a64-7b58-9cc3-9f70462f3798" }, now,
  }));
  const fullWithTombstone = structuredClone(delta);
  fullWithTombstone.inventoryMode = "Full";
  assert.equal(ajv.getSchema("https://schemas.hnb.cloud/runtime-target/v1/runtime-target-observation.schema.json")(fullWithTombstone), false);
  assert.throws(() => validateRuntimeTargetObservationSemantics(fullWithTombstone, {
    identity, cursor: { observerGeneration: 4, sequence: 21, eventId: "018f6c2a-4a64-7b58-9cc3-9f70462f3798" }, now,
  }), /Full storageInventory cannot contain tombstones/);

  const reset = JSON.parse(await readFile(path.join(schemaDirectory, "examples/runtime-target-source-reset.valid.json"), "utf8"));
  const validateReset = ajv.getSchema("https://schemas.hnb.cloud/runtime-target/v1/source-reset.schema.json");
  assert.equal(validateReset(reset), true, ajv.errorsText(validateReset.errors));
  const resetIdentity = {
    tenantId: reset.tenantId,
    targetId: reset.targetId,
    targetKind: reset.targetKind,
    observerId: reset.observerId,
    observerKind: reset.observerKind,
    observerGeneration: reset.newObserverGeneration,
  };
  assert.doesNotThrow(() => validateRuntimeTargetSourceResetSemantics(reset, {
    identity: resetIdentity,
    cursor: { observerGeneration: 7 },
    now,
  }));
  const invalidReset = JSON.parse(await readFile(path.join(schemaDirectory, "examples/runtime-target-source-reset.invalid.json"), "utf8"));
  assert.throws(() => validateRuntimeTargetSourceResetSemantics(invalidReset, {
    identity: resetIdentity,
    cursor: { observerGeneration: 8 },
    now,
  }), /newObserverGeneration must increase|future skew/);
});

test("workload storage schemas accept typed examples and reject unsafe or incomplete states", async () => {
  const ajv = new Ajv2020({ allErrors: true, strict: true, strictRequired: false });
  addFormats(ajv);
  const schemaDirectory = path.join(repositoryRoot, "contracts/schema");
  const schemaFiles = (await listFiles(schemaDirectory)).filter((file) => file.endsWith(".schema.json"));
  for (const file of schemaFiles) ajv.addSchema(JSON.parse(await readFile(file, "utf8")));

  const examples = [
    ["https://schemas.hnb.cloud/storage/v1/storage-inventory.schema.json", "storage-inventory"],
    ["https://schemas.hnb.cloud/storage/v1/storage-backend.schema.json", "storage-backend"],
    ["https://schemas.hnb.cloud/storage/v1/workload-storage-offering.schema.json", "workload-storage-offering"],
    ["https://schemas.hnb.cloud/storage/v1/storage-class-binding.schema.json", "storage-class-binding"],
    ["https://schemas.hnb.cloud/storage/v1/storage-condition.schema.json", "storage-condition"],
    ["https://schemas.hnb.cloud/storage/v1/storage-driver-package.schema.json", "storage-driver-package"],
  ];
  for (const [schemaId, fixtureName] of examples) {
    const validate = ajv.getSchema(schemaId);
    assert.ok(validate, schemaId);
    const valid = JSON.parse(await readFile(path.join(schemaDirectory, `examples/${fixtureName}.valid.json`), "utf8"));
    const invalid = JSON.parse(await readFile(path.join(schemaDirectory, `examples/${fixtureName}.invalid.json`), "utf8"));
    assert.equal(validate(valid), true, `${fixtureName}: ${ajv.errorsText(validate.errors)}`);
    assert.equal(validate(invalid), false, `${fixtureName} accepted its invalid fixture`);
  }

  const providerSchema = ajv.getSchema("https://schemas.hnb.cloud/storage/v1/provider-backend-schema.schema.json");
  assert.ok(providerSchema);
  const trusted = {
    schemaVersion: "1.0.0",
    providerType: "nfs",
    providerSchemaVersion: "1.0.0",
    componentType: "resource.storage.BackendConfigurationForm",
    fields: [{ name: "server", label: "NFS server", type: "text", required: true }],
  };
  assert.equal(providerSchema(trusted), true, ajv.errorsText(providerSchema.errors));
  assert.equal(providerSchema({ ...trusted, componentType: "https://evil.example/form.js" }), false);
  assert.equal(providerSchema({ ...trusted, script: "alert(1)" }), false);
});

test("workload volume contracts reject object services and App Market profiles", async () => {
  const ajv = new Ajv2020({ allErrors: true, strict: true, strictRequired: false });
  addFormats(ajv);
  const schemaDirectory = path.join(repositoryRoot, "contracts/schema");
  for (const file of (await listFiles(schemaDirectory)).filter((name) => name.endsWith(".schema.json"))) {
    ajv.addSchema(JSON.parse(await readFile(file, "utf8")));
  }
  const offering = JSON.parse(await readFile(path.join(schemaDirectory, "examples/workload-storage-offering.valid.json"), "utf8"));
  const binding = JSON.parse(await readFile(path.join(schemaDirectory, "examples/storage-class-binding.valid.json"), "utf8"));
  const validateOffering = ajv.getSchema("https://schemas.hnb.cloud/storage/v1/workload-storage-offering.schema.json");
  const validateBinding = ajv.getSchema("https://schemas.hnb.cloud/storage/v1/storage-class-binding.schema.json");

  assert.equal(validateOffering({ ...offering, consumptionModel: "ObjectBucket" }), false);
  assert.equal(validateOffering({ ...offering, artifactStorageProfileId: offering.id }), false);
  assert.equal(validateBinding({ ...binding, bindingTarget: "ObjectBucket" }), false);
  assert.equal(validateBinding({ ...binding, bucketName: "artifacts" }), false);
});

test("storage OpenAPI exposes dedicated desired-state CRUD with exact scoped IAM contracts", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "hnb-storage-openapi-test-"));
  try {
    const bundled = path.join(directory, "openapi.json");
    execFileSync(path.join(repositoryRoot, "node_modules/.bin/redocly"), [
      "bundle",
      path.join(repositoryRoot, "contracts/openapi/storage/v1/openapi.yaml"),
      "--output", bundled,
      "--ext", "json",
    ], { cwd: repositoryRoot, stdio: "ignore" });
    const specification = JSON.parse(await readFile(bundled, "utf8"));
    const expected = {
      "/api/v1/storage/overview": { get: { resourceKind: "storageOverview", action: "read", tenantScoped: true } },
      "/api/v1/storage/backends": {
        get: { resourceKind: "storageBackend", action: "list", tenantScoped: true },
        post: { resourceKind: "storageBackend", action: "create", tenantScoped: true },
      },
      "/api/v1/storage/provider-schemas": { get: { resourceKind: "storageBackend", action: "list", tenantScoped: true } },
      "/api/v1/storage/backends/{backendId}": {
        get: { resourceKind: "storageBackend", resourceIdPathParameter: "backendId", action: "read", tenantScoped: true },
        put: { resourceKind: "storageBackend", resourceIdPathParameter: "backendId", action: "update", tenantScoped: true },
        delete: { resourceKind: "storageBackend", resourceIdPathParameter: "backendId", action: "delete", tenantScoped: true },
      },
      "/api/v1/storage/offerings": {
        get: { resourceKind: "workloadStorageOffering", action: "list", tenantScoped: true },
        post: { resourceKind: "workloadStorageOffering", action: "create", tenantScoped: true },
      },
      "/api/v1/storage/offerings/{offeringId}": {
        get: { resourceKind: "workloadStorageOffering", resourceIdPathParameter: "offeringId", action: "read", tenantScoped: true },
        put: { resourceKind: "workloadStorageOffering", resourceIdPathParameter: "offeringId", action: "update", tenantScoped: true },
        delete: { resourceKind: "workloadStorageOffering", resourceIdPathParameter: "offeringId", action: "delete", tenantScoped: true },
      },
      "/api/v1/storage/driver-installations": { get: { resourceKind: "storageDriverInstallation", action: "list", tenantScoped: true } },
      "/api/v1/storage/driver-installations/{installationId}/intents/install": { post: {
        resourceKind: "storageDriverInstallation", resourceIdPathParameter: "installationId", action: "create", tenantScoped: true,
      } },
      "/api/v1/storage/driver-installations/{installationId}/intents/upgrade": { post: {
        resourceKind: "storageDriverInstallation", resourceIdPathParameter: "installationId", action: "update", tenantScoped: true,
      } },
      "/api/v1/storage/driver-installations/{installationId}/intents/uninstall": { post: {
        resourceKind: "storageDriverInstallation", resourceIdPathParameter: "installationId", action: "delete", tenantScoped: true,
      } },
      "/api/v1/storage/targets/{targetId}/inventory": { get: {
        resourceKind: "storageInventory", resourceIdPathParameter: "targetId", action: "read", tenantScoped: true,
      } },
      "/api/v1/storage/targets/{targetId}/metrics": { get: {
        resourceKind: "storageInventory", resourceIdPathParameter: "targetId", action: "read", tenantScoped: true,
      } },
      "/api/v1/storage/offerings/{offeringId}/bindings": {
        get: { resourceKind: "storageClassBinding", resourceIdPathParameter: "offeringId", action: "list", tenantScoped: true },
        post: { resourceKind: "storageClassBinding", resourceIdPathParameter: "offeringId", action: "create", tenantScoped: true },
      },
      "/api/v1/storage/bindings/{bindingId}": {
        get: { resourceKind: "storageClassBinding", resourceIdPathParameter: "bindingId", action: "read", tenantScoped: true },
        put: { resourceKind: "storageClassBinding", resourceIdPathParameter: "bindingId", action: "update", tenantScoped: true },
        delete: { resourceKind: "storageClassBinding", resourceIdPathParameter: "bindingId", action: "delete", tenantScoped: true },
      },
	  "/api/v1/storage/offerings/{offeringId}/bindings/intents/import": {
		post: { resourceKind: "storageClassBinding", resourceIdPathParameter: "offeringId", action: "create", tenantScoped: true },
	  },
	  "/api/v1/storage/bindings/{bindingId}/intents/reconcile": {
		post: { resourceKind: "storageClassBinding", resourceIdPathParameter: "bindingId", action: "update", tenantScoped: true },
	  },
      "/api/v1/storage/retained-volumes/{volumeId}/intents/release": { post: {
        resourceKind: "retainedVolume", resourceIdPathParameter: "volumeId", action: "execute", tenantScoped: true,
      } },
      "/api/v1/storage/retained-volumes/{volumeId}/intents/sanitize": { post: {
        resourceKind: "retainedVolume", resourceIdPathParameter: "volumeId", action: "execute", tenantScoped: true,
      } },
      "/api/v1/storage/alert-rules": {
        get: { resourceKind: "storageAlertRule", action: "list", tenantScoped: true },
        post: { resourceKind: "storageAlertRule", action: "create", tenantScoped: true },
      },
    };

    assert.deepEqual(Object.keys(specification.paths), Object.keys(expected));
    for (const [pathName, methods] of Object.entries(expected)) {
      const operations = Object.fromEntries(Object.entries(specification.paths[pathName]).filter(([key]) => key !== "parameters"));
      assert.deepEqual(Object.keys(operations), Object.keys(methods));
      for (const [method, authorization] of Object.entries(methods)) {
        assert.deepEqual(operations[method]["x-hnb-authorization"], authorization);
        const parameters = [...(specification.paths[pathName].parameters ?? []), ...(operations[method].parameters ?? [])];
        assert.ok(parameters.some((parameter) =>
          parameter.name === "X-Correlation-ID" || parameter.$ref === "#/components/parameters/CorrelationId"),
        `${method} ${pathName} correlation`);
      }
    }
    assert.ok(specification.components.schemas.StorageOverview);
    assert.ok(specification.components.schemas.StorageDriverInstallation);
    assert.equal("storageDriverPackage" in specification.components.schemas.StorageDriverInstallation.properties, false);
    assert.equal("provisioners" in specification.components.schemas.StorageDriverInstallation.properties, false);
    assert.ok(specification.components.schemas.StorageInventory);
    assert.ok(specification.components.schemas.ProviderBackendSchema);
    assert.ok(specification.components.schemas.StorageAlertRule);
    assert.deepEqual(specification.components.schemas.StorageBackendInput.required, ["providerType", "providerSchemaVersion", "backendId", "displayName"]);
    assert.equal(specification.components.schemas.WorkloadStorageOfferingInput.properties.consumptionModel.const, "KubernetesPersistentVolume");
    assert.equal(specification.components.schemas.StorageClassBindingInput.properties.bindingTarget.const, "KubernetesStorageClass");
    assert.equal("ArtifactStorageProfile" in specification.components.schemas, false);
    assert.equal(Object.keys(specification.paths).some((name) => /artifact|bucket/i.test(name)), false);
    assert.equal(Object.keys(specification.paths).some((name) => /upload|download|proxy|payload/i.test(name)), false);
    const contractText = JSON.stringify(specification);
    assert.equal(contractText.includes('"format":"binary"'), false);
    for (const pathItem of Object.values(specification.paths)) {
      for (const operation of Object.values(pathItem).filter((value) => value && typeof value === "object" && "responses" in value)) {
        const requestMediaTypes = Object.keys(operation.requestBody?.content ?? {});
        const responseMediaTypes = Object.values(operation.responses ?? {}).flatMap((response) => Object.keys(response.content ?? {}));
        assert.equal([...requestMediaTypes, ...responseMediaTypes].some((mediaType) =>
          mediaType === "application/octet-stream" || mediaType.startsWith("multipart/")), false);
      }
    }
    assert.doesNotThrow(() => validateWriteHeaders(specification));
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("lifecycle compatibility and dictionary sources have exact unique cells", async () => {
  const schemaDirectory = path.join(repositoryRoot, "contracts/schema");
  const ajv = new Ajv2020({ allErrors: true, strict: true, strictRequired: false });
  addFormats(ajv);
  for (const relative of [
    "runtime-target/v1/compatibility-matrix.schema.json",
    "console/v1/dictionary-entry.schema.json",
    "console/v1/cluster-dictionaries.schema.json",
  ]) {
    ajv.addSchema(JSON.parse(await readFile(path.join(schemaDirectory, relative), "utf8")));
  }
  const validMatrix = JSON.parse(await readFile(path.join(schemaDirectory, "runtime-target/v1/compatibility-matrix.json"), "utf8"));
  const runtimeMatrix = JSON.parse(await readFile(path.join(repositoryRoot, "cmd/platform-api/internal/engine/runtime-target-compatibility-matrix.json"), "utf8"));
  const validMatrixExample = JSON.parse(await readFile(path.join(schemaDirectory, "examples/runtime-target-compatibility-matrix.valid.json"), "utf8"));
  const invalidMatrix = JSON.parse(await readFile(path.join(schemaDirectory, "examples/runtime-target-compatibility-matrix.invalid.json"), "utf8"));
  const validateMatrix = ajv.getSchema("https://schemas.hnb.cloud/runtime-target/v1/compatibility-matrix.schema.json");
  assert.equal(validateMatrix(validMatrix), true, ajv.errorsText(validateMatrix.errors));
  assert.equal(validateMatrix(validMatrixExample), true, ajv.errorsText(validateMatrix.errors));
  assert.doesNotThrow(() => validateLifecycleCompatibilityMatrix(validMatrix));
  assert.deepEqual(runtimeMatrix, validMatrix, "platform runtime matrix must be a generated mirror of the canonical contract");
  assert.doesNotThrow(() => validateLifecycleCompatibilityMatrix(validMatrixExample));
  assert.throws(() => validateLifecycleCompatibilityMatrix(invalidMatrix), /providerId|create must be UNSUPPORTED/);

  const dictionaries = JSON.parse(await readFile(path.join(schemaDirectory, "console/v1/cluster-dictionaries.json"), "utf8"));
  const validateDictionaries = ajv.getSchema("https://schemas.hnb.cloud/console/v1/cluster-dictionaries.schema.json");
  assert.equal(validateDictionaries(dictionaries), true, ajv.errorsText(validateDictionaries.errors));
  assert.doesNotThrow(() => validateClusterDictionaries(dictionaries));
  const duplicate = structuredClone(dictionaries);
  duplicate.dictionaries[0].entries.push(structuredClone(duplicate.dictionaries[0].entries[0]));
  assert.throws(() => validateClusterDictionaries(duplicate), /duplicate code/);
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

test("access-token policy snapshot semantics remain aligned across formats", async () => {
  const schema = JSON.parse(await readFile(
    path.join(repositoryRoot, "contracts/schema/identity/v1/access-token-claims.schema.json"),
    "utf8",
  ));
  const permission = JSON.parse(await readFile(
    path.join(repositoryRoot, "contracts/schema/platform/v1/scoped-permission.schema.json"),
    "utf8",
  ));
  const trusted = JSON.parse(await readFile(
    path.join(repositoryRoot, "contracts/schema/identity/v1/trusted-request-context.schema.json"),
    "utf8",
  ));
  const proto = await readFile(
    path.join(repositoryRoot, "contracts/proto/hnb/contracts/v1/contracts.proto"),
    "utf8",
  );
  assert.ok(schema.required.includes("policyVersion"));
  assert.ok(schema.required.includes("scopedPermissions"));
  assert.equal(schema.properties.scopedPermissions.maxItems, 64);
  assert.ok(trusted.required.includes("policyVersion"));
  assert.ok(trusted.required.includes("scopedPermissions"));
  assert.deepEqual(permission.properties.tenantId.not, { const: "*" });
  assert.match(proto, /string policy_version = 16;/);
  assert.match(proto, /repeated ScopedPermission scoped_permissions = 17;/);
  assert.match(proto, /message TrustedRequestContext[\s\S]*string policy_version = 12;/);
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

test("console, RuntimeTarget, and storage schemas have generated Go and TypeScript exports", async () => {
  const generated = path.join(repositoryRoot, "contracts/generated");
  const consoleGo = await readFile(path.join(generated, "go/openapi/console/console.gen.go"), "utf8");
  const runtimeTargetGo = await readFile(
    path.join(generated, "go/openapi/runtime-target/runtime_target.gen.go"),
    "utf8",
  );
  const consoleTypeScript = await readFile(
    path.join(generated, "typescript/console-openapi/index.ts"),
    "utf8",
  );
  const runtimeTargetTypeScript = await readFile(
    path.join(generated, "typescript/runtime-target-openapi/index.ts"),
    "utf8",
  );
  const runtimeStorageTypeScript = await readFile(
    path.join(generated, "typescript/runtime-target-openapi/models/storage_inventory_section_schema.ts"),
    "utf8",
  );
  const storageGo = await readFile(
    path.join(generated, "go/openapi/storage/storage.gen.go"),
    "utf8",
  );
  const storageTypeScript = await readFile(
    path.join(generated, "typescript/storage-openapi/index.ts"),
    "utf8",
  );
  const generatedPackage = JSON.parse(await readFile(
    path.join(generated, "typescript/package.json"),
    "utf8",
  ));

  assert.match(consoleGo, /type ClusterSummary struct/);
  assert.match(consoleGo, /type ProblemDetails struct/);
  assert.match(runtimeTargetGo, /type RuntimeTargetObservation = RuntimeTargetObservationSchema/);
  assert.match(runtimeTargetGo, /type RuntimeTargetStorageInventorySection = StorageInventorySectionSchema/);
  assert.match(runtimeTargetGo, /type StorageClass struct[\s\S]*ResourceVersion[\s\S]*Uid/);
  assert.match(runtimeTargetGo, /SnapshotApi/);
  assert.match(runtimeTargetGo, /type KubernetesLifecycleStepInput = KubernetesLifecycleStepInputSchema/);
  assert.match(consoleTypeScript, /ClusterSummary/);
  assert.match(consoleTypeScript, /ProblemDetails/);
  assert.match(runtimeTargetTypeScript, /RuntimeTargetObservation/);
  assert.match(runtimeTargetTypeScript, /RuntimeTargetStorageInventorySection/);
  assert.match(runtimeStorageTypeScript, /snapshotApi/);
  assert.match(runtimeTargetTypeScript, /KubernetesLifecycleStepInput/);
  for (const typeName of [
    "StorageInventory", "StorageBackend", "WorkloadStorageOffering", "StorageClassBinding", "StorageCondition",
    "StorageDriverPackage", "StorageMetricSnapshot",
  ]) {
    assert.match(storageGo, new RegExp(`type ${typeName} = ${typeName}Schema`));
    assert.match(storageTypeScript, new RegExp(`\\b${typeName}\\b`));
  }
  assert.equal(generatedPackage.exports["./console"], "./console-openapi/index.ts");
  assert.equal(generatedPackage.exports["./runtime-target"], "./runtime-target-openapi/index.ts");
  assert.equal(generatedPackage.exports["./storage"], "./storage-openapi/index.ts");
});

test("storage metric snapshots require provenance, applicability, freshness, and honest values", async () => {
  const ajv = new Ajv2020({ allErrors: true, strict: false });
  addFormats(ajv);
  const schema = JSON.parse(await readFile(path.join(repositoryRoot, "contracts/schema/storage/v1/storage-metric-snapshot.schema.json"), "utf8"));
  const validate = ajv.compile(schema);
  const valid = JSON.parse(await readFile(path.join(repositoryRoot, "contracts/schema/examples/storage-metric-snapshot.valid.json"), "utf8"));
  const invalid = JSON.parse(await readFile(path.join(repositoryRoot, "contracts/schema/examples/storage-metric-snapshot.invalid.json"), "utf8"));
  assert.equal(validate(valid), true, ajv.errorsText(validate.errors));
  assert.equal(validate(invalid), false, "unavailable or unsupported metrics accepted invented values");
  assert.ok(valid.metrics.every((metric) => metric.unit && metric.source && metric.observedAt && metric.freshness && metric.applicability));
});

test("storage alert rules require stable PVC identity and SecretReference channels", async () => {
  const ajv = new Ajv2020({ allErrors: true, strict: false });
  addFormats(ajv);
  const secretReference = JSON.parse(await readFile(path.join(repositoryRoot, "contracts/schema/common/v1/secret-reference.schema.json"), "utf8"));
  const schema = JSON.parse(await readFile(path.join(repositoryRoot, "contracts/schema/alert/v1/storage-alert-rule.schema.json"), "utf8"));
  ajv.addSchema(secretReference);
  const validate = ajv.compile(schema);
  const valid = JSON.parse(await readFile(path.join(repositoryRoot, "contracts/schema/examples/storage-alert-rule.valid.json"), "utf8"));
  const invalid = JSON.parse(await readFile(path.join(repositoryRoot, "contracts/schema/examples/storage-alert-rule.invalid.json"), "utf8"));
  assert.equal(validate(valid), true, ajv.errorsText(validate.errors));
  assert.equal(validate(invalid), false, "inline channel secret was accepted");
  assert.equal(valid.resource.kind, "PersistentVolumeClaim");
  assert.ok(valid.resource.uid && valid.resource.namespace && valid.resource.name && valid.context.bindingId && valid.context.offeringId);
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
      GOWORK: "off",
    },
  });
  execFileSync(path.join(repositoryRoot, "node_modules/.bin/tsx"), [
    path.join(repositoryRoot, "contracts/tests/interop/decode.ts"), encoded,
  ], { cwd: repositoryRoot, stdio: "inherit" });
});
