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
