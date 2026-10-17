#!/usr/bin/env node

import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import { readFile, realpath, stat } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { isDeepStrictEqual } from "node:util";

import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const schema = JSON.parse(await readFile(new URL("../contracts/schema/readiness/v1/product-readiness.schema.json", import.meta.url), "utf8"));
const ajv = new Ajv2020({ allErrors: true, strict: true, strictRequired: false });
addFormats(ajv);
ajv.addSchema(schema);
const validatePlan = ajv.getSchema(`${schema.$id}#/$defs/plan`);
const validateEvidence = ajv.getSchema(`${schema.$id}#/$defs/evidence`);
const levels = ["unit", "db_integration", "real_e2e", "production_ha"];

// These are stage minima, not a substitute for a product-specific test plan.
export const stageAssertions = Object.freeze({
  S0: ["scope_and_owners_recorded", "contracts_recorded", "budgets_approved"],
  S1: ["service_renewal_across_ttl", "service_revocation_bound", "agent_enrollment_replay_rejected", "agent_reconnect_fenced"],
  S2: ["independent_target_import", "authenticated_observation", "read_model_freshness", "disconnect_reconnect", "idempotent_replay"],
  S3: ["typed_plan_preflight", "external_persistent_read_write", "application_upgrade", "failed_upgrade_recovery", "delete_retains_data"],
  S4: ["real_release_clean_install", "independent_implementer", "install_resume", "continuous_identity", "independent_target_import", "external_persistent_read_write", "platform_upgrade_recovery", "application_upgrade", "failed_upgrade_recovery", "delete_retains_data"],
  S5: ["ui_cli_consistency", "diagnostic_journey", "telemetry_tenant_isolation", "resource_and_capacity_budget"],
  S6: ["diagnostic_journey", "resource_and_capacity_budget", "real_extension_lifecycle", "ha_failure_drill", "complete_restore_set", "measured_rto_rpo"],
  S7: ["single_writer_ownership", "controlled_handoff", "gitops_convergence"],
});

function digest(bytes) {
  return `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
}

function hasDuplicate(entries, key) {
  return new Set(entries.map((entry) => entry[key])).size !== entries.length;
}

async function reportDigest(root, reportPath) {
  // Report references are local retained files, never URLs or arbitrary paths.
  if (path.isAbsolute(reportPath) || reportPath.includes("\\") || reportPath.includes(":") ||
      reportPath.split("/").some((part) => part === ".." || part === "." || part === "")) {
    throw new Error("invalid report reference");
  }
  const rootPath = await realpath(root);
  const target = await realpath(path.resolve(rootPath, reportPath));
  const relative = path.relative(rootPath, target);
  if (relative.startsWith(`..${path.sep}`) || relative === ".." || path.isAbsolute(relative) || !(await stat(target)).isFile()) {
    throw new Error("invalid report reference");
  }
  const hash = createHash("sha256");
  for await (const chunk of createReadStream(target)) hash.update(chunk);
  return `sha256:${hash.digest("hex")}`;
}

// The caller supplies a separately approved expected plan. The evidence cannot
// choose a weaker release/scope/budget or certify itself by saying "passed".
export async function validateReadiness(planBytes, evidence, { root = process.cwd(), now = new Date() } = {}) {
  const reasons = new Set();
  const result = () => ({
    schemaVersion: "product-readiness-check/v1",
    contentVerified: reasons.size === 0,
    executionAuthorized: false,
    reasonCodes: [...reasons].sort(),
  });
  let plan;
  try {
    plan = JSON.parse(planBytes.toString());
  } catch {
    reasons.add("PLAN_SCHEMA_INVALID");
    return result();
  }
  if (!validatePlan(plan)) reasons.add("PLAN_SCHEMA_INVALID");
  if (!validateEvidence(evidence)) reasons.add("EVIDENCE_SCHEMA_INVALID");
  if (reasons.size) return result();

  if (plan.approval.status !== "approved") reasons.add("BUDGETS_NOT_APPROVED");
  const approvedAt = Date.parse(plan.approval.approvedAt);
  const validUntil = Date.parse(plan.validUntil);
  const startedAt = Date.parse(evidence.startedAt);
  const finishedAt = Date.parse(evidence.finishedAt);
  if (!Number.isFinite(now.getTime()) || !Number.isFinite(approvedAt) || approvedAt > startedAt ||
      startedAt > finishedAt || finishedAt > now.getTime() || finishedAt > validUntil || approvedAt >= validUntil) {
    reasons.add("EVIDENCE_TIME_INVALID");
  }
  if (now.getTime() > validUntil) reasons.add("EVIDENCE_EXPIRED");
  if (evidence.planDigest !== digest(planBytes)) reasons.add("PLAN_DIGEST_MISMATCH");
  if (!isDeepStrictEqual(plan.identity, evidence.identity)) reasons.add("RELEASE_IDENTITY_MISMATCH");
  if (plan.stage !== evidence.stage || !isDeepStrictEqual(plan.scope, evidence.scope)) reasons.add("EVIDENCE_SCOPE_MISMATCH");
  if (evidence.status !== "passed" || evidence.exitCode !== 0) reasons.add("EVIDENCE_NOT_PASSED");

  const stageLevel = plan.stage === "S0" ? "unit" : plan.stage === "S6" ? "production_ha" : "real_e2e";
  if (levels.indexOf(plan.minimumEvidenceLevel) < levels.indexOf(stageLevel) ||
      levels.indexOf(evidence.level) < levels.indexOf(plan.minimumEvidenceLevel)) reasons.add("EVIDENCE_LEVEL_INSUFFICIENT");
  const needsHA = plan.scope.profile !== "Minimal" || plan.stage === "S6" || evidence.level === "production_ha";
  const requiredDomains = Math.max(plan.minimumControlPlaneDomains, needsHA ? 3 : 1);
  if (evidence.scope.controlPlaneDomainIds.length < requiredDomains || (needsHA && evidence.scope.backing === "bundled")) {
    reasons.add("FAILURE_DOMAINS_INSUFFICIENT");
  }
  const needsTarget = plan.requireIndependentTarget || ["S2", "S3", "S4", "S6", "S7"].includes(plan.stage);
  if (needsTarget && (!evidence.scope.targetClusterId || evidence.scope.targetClusterId === evidence.scope.managementClusterId)) {
    reasons.add("INDEPENDENT_TARGET_REQUIRED");
  }

  if (hasDuplicate(plan.budgets, "id") || hasDuplicate(evidence.assertions, "id") ||
      hasDuplicate(evidence.measurements, "budgetId") || hasDuplicate(evidence.reports, "path")) reasons.add("DUPLICATE_EVIDENCE_ID");
  if (stageAssertions[plan.stage].some((id) => !plan.requiredAssertions.includes(id))) reasons.add("STAGE_ASSERTIONS_MISSING");
  const reports = new Map(evidence.reports.map((report) => [report.path, report.digest]));
  const assertions = new Map(evidence.assertions.map((assertion) => [assertion.id, assertion]));
  for (const id of plan.requiredAssertions) {
    const assertion = assertions.get(id);
    if (!assertion || assertion.status !== "passed") reasons.add("REQUIRED_ASSERTION_NOT_PASSED");
  }
  if (!reports.size) reasons.add("REPORT_REQUIRED");
  for (const reference of [...evidence.assertions, ...evidence.measurements]) {
    if (!reports.has(reference.reportPath)) reasons.add("REPORT_REFERENCE_MISSING");
  }
  for (const [reportPath, expectedDigest] of reports) {
    try {
      if (await reportDigest(root, reportPath) !== expectedDigest) reasons.add("REPORT_DIGEST_MISMATCH");
    } catch {
      reasons.add("REPORT_UNREADABLE");
    }
  }

  const measurements = new Map(evidence.measurements.map((measurement) => [measurement.budgetId, measurement]));
  const budgetIds = new Set(plan.budgets.map((budget) => budget.id));
  if (evidence.measurements.some((measurement) => !budgetIds.has(measurement.budgetId))) reasons.add("UNKNOWN_BUDGET_MEASUREMENT");
  for (const budget of plan.budgets) {
    const measurement = measurements.get(budget.id);
    if (!measurement) {
      reasons.add("BUDGET_MEASUREMENT_MISSING");
      continue;
    }
    if (measurement.unit !== budget.unit || budget.sampleIntervalSeconds > budget.windowSeconds ||
        measurement.windowSeconds < budget.windowSeconds || measurement.sampleIntervalSeconds > budget.sampleIntervalSeconds ||
        measurement.sampleCount < Math.ceil(measurement.windowSeconds / measurement.sampleIntervalSeconds)) {
      reasons.add("BUDGET_SAMPLE_INVALID");
    }
    if ((budget.comparison === "atMost" && measurement.value > budget.limit) ||
        (budget.comparison === "atLeast" && measurement.value < budget.limit)) reasons.add("BUDGET_EXCEEDED");
  }
  return result();
}

async function main(args) {
  const options = {};
  for (let index = 0; index < args.length; index += 2) {
    const key = args[index];
    if (!["--plan", "--evidence", "--root"].includes(key) || !args[index + 1] || args[index + 1].startsWith("--") || options[key]) {
      throw new Error("invalid arguments");
    }
    options[key] = args[index + 1];
  }
  if (!options["--plan"] || !options["--evidence"]) throw new Error("missing inputs");
  const planBytes = await readFile(options["--plan"]);
  const evidence = JSON.parse(await readFile(options["--evidence"], "utf8"));
  return validateReadiness(planBytes, evidence, { root: options["--root"] ?? process.cwd() });
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const result = await main(process.argv.slice(2));
    process.stdout.write(`${JSON.stringify(result)}\n`);
    process.exitCode = result.contentVerified ? 0 : 1;
  } catch {
    // Never print JSON parser errors, file contents, commands, or caller values.
    process.stdout.write(`${JSON.stringify({ schemaVersion: "product-readiness-check/v1", contentVerified: false, executionAuthorized: false, reasonCodes: ["INPUT_INVALID"] })}\n`);
    process.exitCode = 2;
  }
}
