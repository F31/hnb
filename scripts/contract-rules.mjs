function parameterName(specification, parameter) {
  if (!parameter.$ref) return parameter.name;
  const name = parameter.$ref.split("/").at(-1);
  return specification.components?.parameters?.[name]?.name;
}

export function validateWriteHeaders(specification) {
  const errors = [];
  for (const [route, pathItem] of Object.entries(specification.paths ?? {})) {
    for (const method of ["post", "put", "patch", "delete"]) {
      const operation = pathItem[method];
      if (!operation) continue;
      const names = [...(pathItem.parameters ?? []), ...(operation.parameters ?? [])]
        .map((parameter) => parameterName(specification, parameter));
      for (const required of ["X-Correlation-ID", "Idempotency-Key"]) {
        if (!names.includes(required)) errors.push(`${method.toUpperCase()} ${route} missing ${required}`);
      }
      if (["put", "patch", "delete"].includes(method) && !names.includes("If-Match")) {
        errors.push(`${method.toUpperCase()} ${route} missing If-Match`);
      }
    }
  }
  if (errors.length > 0) throw new Error(`write contract violations: ${errors.join("; ")}`);
}

export function scanForbiddenFields(value, location = "root", errors = []) {
  if (!value || typeof value !== "object") return errors;
  const properties = value.properties && typeof value.properties === "object" ? value.properties : {};
  for (const name of Object.keys(properties)) {
    if (/^(password|token|accessToken|refreshToken|kubeconfig|privateKey|secretValue|payloadBody)$/i.test(name)) {
      errors.push(`${location}.properties.${name}`);
    }
  }
  for (const [name, child] of Object.entries(value)) {
    scanForbiddenFields(child, `${location}.${name}`, errors);
  }
  return errors;
}

export function scanForbiddenProtoFields(content) {
  return [...content.matchAll(
    /^\s*(?:optional\s+)?\w+\s+(password|token|access_token|refresh_token|kubeconfig|private_key|secret_value|payload_body)\s*=/gim,
  )].map((match) => `contracts.proto.${match[1]}`);
}

const runtimeIntentExecutionFields = new Set([
  "step",
  "steps",
  "steptype",
  "command",
  "commands",
  "providerid",
  "providercommand",
  "providercommands",
  "credential",
  "credentials",
  "targetcredential",
  "targetcredentials",
  "artifactbytes",
  "fencing",
  "fencingtoken",
  "policyresult",
  "policyresults",
  "approvalresult",
  "approvalresults",
  "url",
  "providerurl",
  "endpointurl",
  "executionurl",
  "callbackurl",
  "stepurl",
]);

function scanRuntimeIntentProperties(value, location, errors) {
  if (!value || typeof value !== "object") return;
  const properties = value.properties && typeof value.properties === "object" ? value.properties : {};
  for (const [name, child] of Object.entries(properties)) {
    const normalized = name.replace(/[-_]/g, "").toLowerCase();
    if (runtimeIntentExecutionFields.has(normalized)) errors.push(`${location}.properties.${name}`);
    scanRuntimeIntentProperties(child, `${location}.properties.${name}`, errors);
  }
  for (const keyword of ["allOf", "anyOf", "oneOf", "items", "$defs"]) {
    const child = value[keyword];
    if (Array.isArray(child)) child.forEach((item, index) => scanRuntimeIntentProperties(item, `${location}.${keyword}.${index}`, errors));
    else scanRuntimeIntentProperties(child, `${location}.${keyword}`, errors);
  }
}

export function scanRuntimeIntentExecutionFields(value, location = "root", errors = []) {
  if (!value || typeof value !== "object") return errors;
  if (value.title === "RuntimeIntent") scanRuntimeIntentProperties(value, location, errors);
  for (const [name, child] of Object.entries(value)) {
    if (name === "RuntimeIntent") scanRuntimeIntentProperties(child, `${location}.${name}`, errors);
    else if (name !== "properties") scanRuntimeIntentExecutionFields(child, `${location}.${name}`, errors);
  }
  return errors;
}

function schemaMajor(schema) {
  const match = schema?.$id?.match(/\/v(\d+)\//);
  return match ? Number(match[1]) : undefined;
}

export function jsonSchemaBreakingChanges(previous, current, location = "root") {
  if (schemaMajor(previous) !== undefined && schemaMajor(previous) !== schemaMajor(current)) return [];
  const changes = [];
  if (previous.type !== undefined && current.type !== previous.type) {
    changes.push(`${location}: type changed from ${previous.type} to ${current.type}`);
  }
  if (previous.format !== undefined && current.format !== previous.format) {
    changes.push(`${location}: format changed from ${previous.format} to ${current.format}`);
  }
  if (previous.additionalProperties !== false && current.additionalProperties === false) {
    changes.push(`${location}: additionalProperties became false`);
  }
  const previousRequired = new Set(previous.required ?? []);
  for (const name of current.required ?? []) {
    if (!previousRequired.has(name)) changes.push(`${location}.${name}: became required`);
  }
  const previousProperties = previous.properties ?? {};
  const currentProperties = current.properties ?? {};
  for (const [name, schema] of Object.entries(previousProperties)) {
    if (!(name in currentProperties)) changes.push(`${location}.${name}: property removed`);
    else changes.push(...jsonSchemaBreakingChanges(schema, currentProperties[name], `${location}.${name}`));
  }
  if (Array.isArray(previous.enum) && Array.isArray(current.enum)) {
    for (const value of previous.enum) {
      if (!current.enum.includes(value)) changes.push(`${location}: enum value removed: ${value}`);
    }
  }
  return changes;
}

function throwSemanticErrors(contract, errors) {
  if (errors.length > 0) throw new Error(`${contract} violations: ${errors.join("; ")}`);
}

function validateObservedAt(value, now, errors) {
  const observedAt = Date.parse(value.observedAt);
  if (!Number.isFinite(observedAt)) errors.push("observedAt is invalid");
  else if (observedAt > now.getTime() + 300_000) errors.push("observedAt exceeds 300 second future skew");
}

function validateIdentity(value, identity, fields, errors) {
  for (const field of fields) {
    if (value[field] !== identity[field]) errors.push(`${field} does not match authenticated observer identity`);
  }
}

export function validateRuntimeTargetObservationSemantics(value, options) {
  const errors = [];
  const {
    identity,
    cursor,
    now = new Date(),
    maxSizeBytes = 1_048_576,
  } = options;
  validateIdentity(value, identity, [
    "tenantId", "targetId", "targetKind", "observerId", "observerKind", "observerGeneration",
  ], errors);
  validateObservedAt(value, now, errors);
  if (Buffer.byteLength(JSON.stringify(value), "utf8") > maxSizeBytes) {
    errors.push(`encoded payload exceeds ${maxSizeBytes} bytes`);
  }
  if (value.inventoryMode === "Full") {
    for (const resources of Object.values(value.storageInventory ?? {})) {
      if (Array.isArray(resources) && resources.some((resource) => resource.deleted === true)) {
        errors.push("Full storageInventory cannot contain tombstones");
      }
    }
  }

  if (!cursor) {
    if (value.sequence !== 1) errors.push("first sequence in an observer generation must be 1");
  } else if (value.eventId === cursor.eventId
    && value.observerGeneration === cursor.observerGeneration
    && value.sequence === cursor.sequence) {
    // Exact event replay is idempotent.
  } else if (value.observerGeneration !== cursor.observerGeneration) {
    errors.push("observer generation must be established by source reset before observation");
  } else if (value.sequence !== cursor.sequence + 1) {
    errors.push(`sequence must be contiguous after ${cursor.sequence}`);
  }

  throwSemanticErrors("RuntimeTargetObservation", errors);
}

export function validateRuntimeTargetSourceResetSemantics(value, options) {
  const errors = [];
  const { identity, cursor, now = new Date() } = options;
  validateIdentity(value, identity, ["tenantId", "targetId", "targetKind", "observerId", "observerKind"], errors);
  validateObservedAt(value, now, errors);
  if (value.previousObserverGeneration !== cursor.observerGeneration) {
    errors.push("previousObserverGeneration does not match the accepted cursor");
  }
  if (value.newObserverGeneration <= value.previousObserverGeneration) {
    errors.push("newObserverGeneration must increase monotonically");
  }
  if (value.newObserverGeneration !== identity.observerGeneration) {
    errors.push("newObserverGeneration does not match the authenticated observer lease");
  }
  throwSemanticErrors("RuntimeTargetSourceReset", errors);
}

const lifecycleMatrix = {
  KubernetesTarget: {
    providerId: "runtime-target.lifecycle.kubernetes",
    observationSource: "Agent",
    actions: { create: "REQUIRED", import: "REQUIRED", upgrade: "REQUIRED", unmanage: "REQUIRED" },
  },
  EdgeRuntimeTarget: {
    providerId: "runtime-target.lifecycle.edge",
    observationSource: "CloudCore",
    actions: { create: "UNSUPPORTED", import: "REQUIRED", upgrade: "REQUIRED", unmanage: "REQUIRED" },
  },
};

export function validateLifecycleCompatibilityMatrix(value) {
  const errors = [];
  const rows = new Map();
  for (const row of value.rows ?? []) {
    if (rows.has(row.targetKind)) errors.push(`duplicate targetKind ${row.targetKind}`);
    rows.set(row.targetKind, row);
  }
  for (const [targetKind, expected] of Object.entries(lifecycleMatrix)) {
    const row = rows.get(targetKind);
    if (!row) {
      errors.push(`missing targetKind ${targetKind}`);
      continue;
    }
    if (row.providerId !== expected.providerId) errors.push(`${targetKind} providerId must be ${expected.providerId}`);
    if (row.observationSource !== expected.observationSource) {
      errors.push(`${targetKind} observationSource must be ${expected.observationSource}`);
    }
    for (const [action, support] of Object.entries(expected.actions)) {
      if (row.actions?.[action] !== support) errors.push(`${targetKind}/${action} must be ${support}`);
    }
  }
  for (const targetKind of rows.keys()) {
    if (!(targetKind in lifecycleMatrix)) errors.push(`unsupported targetKind ${targetKind}`);
  }
  if (Date.parse(value.expiresAt) <= Date.parse(value.effectiveAt)) errors.push("expiresAt must follow effectiveAt");
  throwSemanticErrors("RuntimeTargetLifecycleCompatibilityMatrix", errors);
}

export function validateClusterDictionaries(value) {
  const errors = [];
  const expectedIds = new Set([
    "resource.cluster.lifecycle",
    "resource.cluster.health",
    "resource.cluster.connectivity",
    "resource.cluster.freshness",
    "resource.cluster.status",
  ]);
  const dictionaries = new Map();
  for (const dictionary of value.dictionaries ?? []) {
    if (dictionaries.has(dictionary.dictionaryId)) errors.push(`duplicate dictionaryId ${dictionary.dictionaryId}`);
    dictionaries.set(dictionary.dictionaryId, dictionary);
    const codes = new Set();
    for (const entry of dictionary.entries ?? []) {
      if (codes.has(entry.code)) errors.push(`${dictionary.dictionaryId} has duplicate code ${entry.code}`);
      codes.add(entry.code);
    }
  }
  for (const id of expectedIds) {
    if (!dictionaries.has(id)) errors.push(`missing dictionary ${id}`);
  }
  for (const id of dictionaries.keys()) {
    if (!expectedIds.has(id)) errors.push(`unknown dictionary ${id}`);
  }
  if (dictionaries.get("resource.cluster.status")?.compatibilityOnly !== true) {
    errors.push("resource.cluster.status must be compatibilityOnly");
  }
  const aggregation = value.statusCompatibilityAggregation;
  if (aggregation?.displayOnly !== true || aggregation?.preservesDimensions !== true) {
    errors.push("status compatibility aggregation must be display-only and preserve all four dimensions");
  }
  const aggregateCodes = new Set(dictionaries.get("resource.cluster.status")?.entries?.map((entry) => entry.code));
  for (const rule of aggregation?.precedence ?? []) {
    if (!aggregateCodes.has(rule.code)) errors.push(`aggregation rule references unknown code ${rule.code}`);
  }
  throwSemanticErrors("ClusterDictionaries", errors);
}
