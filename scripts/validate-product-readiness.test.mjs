import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { stageAssertions, validateReadiness } from "./validate-product-readiness.mjs";

const digest = (bytes) => `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
const now = new Date("2026-10-16T12:00:00Z");

async function fixture(t, stage = "S4") {
  const root = await mkdtemp(path.join(tmpdir(), "hnb-readiness-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  // Synthetic test-only bytes; these never enter the acceptance registry.
  const report = JSON.stringify({ runId: "unit-fixture", scope: "validator tests only" });
  await writeFile(path.join(root, "report.json"), report);
  const plan = {
    schemaVersion: "product-readiness/v1", kind: "ReadinessPlan", stage,
    identity: {
      commit: createHash("sha1").update("test-only source").digest("hex"),
      sourceDigest: digest("test-only dirty working tree"), configurationDigest: digest("test-only effective selections and overrides"), releaseVersion: "0.1.0-test",
      artifactDigests: { cli: digest("test-only executable"), chart: digest("test-only chart") },
    },
    scope: {
      os: "ubuntu-26.04", architecture: "amd64", kubernetesVersion: "1.36.5+k3s1",
      compositionId: "container-management", profile: "Minimal", backing: "bundled",
      managementClusterId: "management-uid", targetClusterId: "independent-target-uid",
      controlPlaneDomainIds: ["control-domain-1"],
      conditions: { hardware: "test hardware", network: "test network", storage: "test CSI", load: "test workload" },
    },
    minimumEvidenceLevel: "real_e2e", minimumControlPlaneDomains: 1, requireIndependentTarget: true,
    requiredAssertions: [...stageAssertions[stage]],
    budgets: [{ id: "cold-start", metric: "cold_start", limit: 300, comparison: "atMost", unit: "seconds", windowSeconds: 60, sampleIntervalSeconds: 5 }],
    approval: { status: "approved", approvedBy: "fixture-reviewer", approvedAt: "2026-10-16T08:00:00Z" },
    validUntil: "2026-10-17T00:00:00Z",
  };
  const evidence = {
    schemaVersion: "product-readiness/v1", kind: "ReadinessEvidence", stage,
    planDigest: digest(JSON.stringify(plan)), identity: structuredClone(plan.identity), scope: structuredClone(plan.scope),
    level: "real_e2e", status: "passed", startedAt: "2026-10-16T09:00:00Z", finishedAt: "2026-10-16T10:00:00Z",
    commands: ["fixture-test-command"], exitCode: 0,
    assertions: plan.requiredAssertions.map((id) => ({ id, status: "passed", reportPath: "report.json" })),
    measurements: [{ budgetId: "cold-start", value: 120, unit: "seconds", windowSeconds: 60, sampleIntervalSeconds: 5, sampleCount: 12, reportPath: "report.json" }],
    reports: [{ path: "report.json", digest: digest(report) }],
  };
  return { plan, evidence, root };
}

async function check(f, mutate = () => {}) {
  const plan = structuredClone(f.plan);
  const evidence = structuredClone(f.evidence);
  mutate(plan, evidence);
  const bytes = JSON.stringify(plan);
  evidence.planDigest = digest(bytes);
  return validateReadiness(bytes, evidence, { root: f.root, now });
}

test("matching retained reports and approved numeric budgets verify content but never authorize execution", async (t) => {
  const f = await fixture(t);
  assert.deepEqual(await check(f), {
    schemaVersion: "product-readiness-check/v1", contentVerified: true, executionAuthorized: false, reasonCodes: [],
  });
});

test("checks actual report bytes, presence and assertion-to-report references", async (t) => {
  const f = await fixture(t);
  for (const [name, mutate, expected] of [
    ["missing retained file", (_p, e) => { e.reports[0].path = "missing.json"; }, "REPORT_UNREADABLE"],
    ["report tampered", (_p, e) => { e.reports[0].digest = digest("other content"); }, "REPORT_DIGEST_MISMATCH"],
    ["no report", (_p, e) => { e.reports = []; }, "REPORT_REQUIRED"],
    ["unbound assertion", (_p, e) => { e.assertions[0].reportPath = "unlisted.json"; }, "REPORT_REFERENCE_MISSING"],
    ["unbound measurement", (_p, e) => { e.measurements[0].reportPath = "unlisted.json"; }, "REPORT_REFERENCE_MISSING"],
  ]) {
    await t.test(name, async () => {
      const result = await check(f, mutate);
      assert.equal(result.contentVerified, false);
      assert.ok(result.reasonCodes.includes(expected), result.reasonCodes.join(","));
    });
  }
});

test("old or mismatched release, stage, OS, topology and evidence levels cannot be promoted", async (t) => {
  const f = await fixture(t);
  for (const [name, mutate, expected] of [
    ["old image", (_p, e) => { e.identity.artifactDigests.cli = digest("old executable"); }, "RELEASE_IDENTITY_MISMATCH"],
    ["dirty tree mismatch", (_p, e) => { e.identity.sourceDigest = digest("different local changes"); }, "RELEASE_IDENTITY_MISMATCH"],
    ["different selected capabilities", (_p, e) => { e.identity.configurationDigest = digest("different effective configuration"); }, "RELEASE_IDENTITY_MISMATCH"],
    ["another OS", (_p, e) => { e.scope.os = "rocky-10.1"; }, "EVIDENCE_SCOPE_MISMATCH"],
    ["different load conditions", (_p, e) => { e.scope.conditions.load = "read-only instead of mixed writes"; }, "EVIDENCE_SCOPE_MISMATCH"],
    ["another backing", (_p, e) => { e.scope.backing = "external"; }, "EVIDENCE_SCOPE_MISMATCH"],
    ["another stage", (_p, e) => { e.stage = "S0"; }, "EVIDENCE_SCOPE_MISMATCH"],
    ["unit evidence", (_p, e) => { e.level = "unit"; }, "EVIDENCE_LEVEL_INSUFFICIENT"],
    ["plan weakens evidence level", (p) => { p.minimumEvidenceLevel = "unit"; }, "EVIDENCE_LEVEL_INSUFFICIENT"],
    ["self import", (p, e) => { p.scope.targetClusterId = e.scope.targetClusterId = p.scope.managementClusterId; }, "INDEPENDENT_TARGET_REQUIRED"],
    ["disable target requirement", (p, e) => { p.requireIndependentTarget = false; delete p.scope.targetClusterId; delete e.scope.targetClusterId; }, "INDEPENDENT_TARGET_REQUIRED"],
    ["Lite with one domain", (p, e) => { p.scope.profile = e.scope.profile = "LiteHA"; }, "FAILURE_DOMAINS_INSUFFICIENT"],
    ["expired plan", (p) => { p.validUntil = "2026-10-16T11:00:00Z"; }, "EVIDENCE_EXPIRED"],
    ["approval after measurement", (p) => { p.approval.approvedAt = "2026-10-16T11:00:00Z"; }, "EVIDENCE_TIME_INVALID"],
    ["future finish", (_p, e) => { e.finishedAt = "2026-10-16T13:00:00Z"; }, "EVIDENCE_TIME_INVALID"],
    ["negative duration", (_p, e) => { e.finishedAt = "2026-10-16T08:30:00Z"; }, "EVIDENCE_TIME_INVALID"],
  ]) {
    await t.test(name, async () => {
      const result = await check(f, mutate);
      assert.equal(result.contentVerified, false);
      assert.ok(result.reasonCodes.includes(expected), result.reasonCodes.join(","));
    });
  }
});

test("caller cannot replace the approved plan or remove the stage's business assertions", async (t) => {
  const f = await fixture(t);
  const evidence = structuredClone(f.evidence);
  evidence.planDigest = digest("an unapproved plan");
  assert.ok((await validateReadiness(JSON.stringify(f.plan), evidence, { root: f.root, now })).reasonCodes.includes("PLAN_DIGEST_MISMATCH"));
  const result = await check(f, (plan, run) => {
    plan.requiredAssertions = ["nineteen_workloads_ready"];
    run.assertions = [{ id: "nineteen_workloads_ready", status: "passed", reportPath: "report.json" }];
  });
  assert.ok(result.reasonCodes.includes("STAGE_ASSERTIONS_MISSING"));
});

test("missing, failed and duplicate assertions do not disappear behind a passed summary", async (t) => {
  const f = await fixture(t);
  for (const [mutate, expected] of [
    [(_p, e) => { e.assertions.pop(); }, "REQUIRED_ASSERTION_NOT_PASSED"],
    [(_p, e) => { e.assertions[0].status = "blocked"; }, "REQUIRED_ASSERTION_NOT_PASSED"],
    [(_p, e) => { e.assertions.push(e.assertions[0]); }, "DUPLICATE_EVIDENCE_ID"],
    [(_p, e) => { e.status = "partial"; }, "EVIDENCE_NOT_PASSED"],
    [(_p, e) => { e.exitCode = 1; }, "EVIDENCE_NOT_PASSED"],
  ]) assert.ok((await check(f, mutate)).reasonCodes.includes(expected));
});

test("budgets require approval, measurements, correct units and a sufficient sampling window", async (t) => {
  const f = await fixture(t);
  for (const [name, mutate, expected] of [
    ["unapproved", (p) => { p.approval = { status: "draft" }; }, "BUDGETS_NOT_APPROVED"],
    ["no budgets", (p) => { p.budgets = []; }, "PLAN_SCHEMA_INVALID"],
    ["no measurement", (_p, e) => { e.measurements = []; }, "BUDGET_MEASUREMENT_MISSING"],
    ["over limit", (_p, e) => { e.measurements[0].value = 301; }, "BUDGET_EXCEEDED"],
    ["below minimum", (p) => { p.budgets[0].comparison = "atLeast"; }, "BUDGET_EXCEEDED"],
    ["wrong unit", (_p, e) => { e.measurements[0].unit = "milliseconds"; }, "BUDGET_SAMPLE_INVALID"],
    ["short window", (_p, e) => { e.measurements[0].windowSeconds = 10; }, "BUDGET_SAMPLE_INVALID"],
    ["coarse samples", (_p, e) => { e.measurements[0].sampleIntervalSeconds = 10; }, "BUDGET_SAMPLE_INVALID"],
    ["too few samples", (_p, e) => { e.measurements[0].sampleCount = 1; }, "BUDGET_SAMPLE_INVALID"],
    ["unknown budget", (_p, e) => { e.measurements[0].budgetId = "undeclared"; }, "UNKNOWN_BUDGET_MEASUREMENT"],
    ["duplicate budget", (p) => { p.budgets.push(p.budgets[0]); }, "DUPLICATE_EVIDENCE_ID"],
    ["duplicate measurement", (_p, e) => { e.measurements.push(e.measurements[0]); }, "DUPLICATE_EVIDENCE_ID"],
  ]) {
    await t.test(name, async () => {
      const result = await check(f, mutate);
      assert.equal(result.contentVerified, false);
      assert.ok(result.reasonCodes.includes(expected), result.reasonCodes.join(","));
    });
  }
});

test("HA verification needs actual scoped domains and a non-bundled backing", async (t) => {
  const f = await fixture(t, "S6");
  const initial = await check(f);
  assert.ok(initial.reasonCodes.includes("FAILURE_DOMAINS_INSUFFICIENT"));
  assert.ok(initial.reasonCodes.includes("EVIDENCE_LEVEL_INSUFFICIENT"));
  const result = await check(f, (p, e) => {
    p.minimumEvidenceLevel = e.level = "production_ha";
    p.minimumControlPlaneDomains = 3;
    p.scope.profile = e.scope.profile = "LiteHA";
    p.scope.backing = e.scope.backing = "in-cluster-ha";
    p.scope.controlPlaneDomainIds = ["domain-a", "domain-b", "domain-c"];
    e.scope.controlPlaneDomainIds = [...p.scope.controlPlaneDomainIds];
  });
  assert.equal(result.contentVerified, true);
  assert.equal(result.executionAuthorized, false);
});

test("report references cannot escape the retained root, including through symlinks", async (t) => {
  const f = await fixture(t);
  const outside = await mkdtemp(path.join(tmpdir(), "hnb-readiness-outside-"));
  t.after(() => rm(outside, { recursive: true, force: true }));
  const target = path.join(outside, "private.txt");
  await writeFile(target, "sensitive fixture content must not be printed");
  await symlink(target, path.join(f.root, "escape.json"));
  for (const reportPath of ["../private.txt", target, "https://example.test/report", "escape.json"]) {
    const result = await check(f, (_p, e) => { e.reports[0].path = reportPath; });
    assert.ok(result.reasonCodes.includes("REPORT_UNREADABLE"));
    assert.ok(!JSON.stringify(result).includes("sensitive fixture"));
  }
});

test("schema rejects unknown fields, missing identity and repeated fault domains", async (t) => {
  const f = await fixture(t);
  for (const mutate of [
    (_p, e) => { e.secretValue = "must-not-be-echoed"; },
    (_p, e) => { delete e.identity.sourceDigest; },
    (_p, e) => { e.scope.controlPlaneDomainIds = ["domain-a", "domain-a", "domain-a"]; },
  ]) {
    const result = await check(f, mutate);
    assert.deepEqual(result.reasonCodes, ["EVIDENCE_SCHEMA_INVALID"]);
    assert.ok(!JSON.stringify(result).includes("must-not-be-echoed"));
  }
});

test("CLI exits nonzero without echoing malformed or sensitive caller input", async (t) => {
  const f = await fixture(t);
  const cli = fileURLToPath(new URL("./validate-product-readiness.mjs", import.meta.url));
  const planFile = path.join(f.root, "plan.json");
  const evidenceFile = path.join(f.root, "evidence.json");
  await writeFile(planFile, JSON.stringify(f.plan));
  await writeFile(evidenceFile, '{"secretValue":"DO-NOT-PRINT", broken');
  const invalid = spawnSync(process.execPath, [cli, "--plan", planFile, "--evidence", evidenceFile], { encoding: "utf8" });
  assert.equal(invalid.status, 2);
  assert.equal(invalid.stderr, "");
  assert.ok(!invalid.stdout.includes("DO-NOT-PRINT"));
  assert.deepEqual(JSON.parse(invalid.stdout).reasonCodes, ["INPUT_INVALID"]);
  f.evidence.status = "failed";
  await writeFile(evidenceFile, JSON.stringify(f.evidence));
  const failed = spawnSync(process.execPath, [cli, "--plan", planFile, "--evidence", evidenceFile, "--root", f.root], { encoding: "utf8" });
  assert.equal(failed.status, 1);
  assert.ok(JSON.parse(failed.stdout).reasonCodes.includes("EVIDENCE_NOT_PASSED"));
  assert.equal(await readFile(planFile, "utf8"), JSON.stringify(f.plan));
});

test("CLI accepts a matching fresh run without changing plans or evidence", async (t) => {
  const f = await fixture(t);
  const clock = Date.now();
  f.plan.approval.approvedAt = new Date(clock - 3600_000).toISOString();
  f.plan.validUntil = new Date(clock + 3600_000).toISOString();
  f.evidence.startedAt = new Date(clock - 1800_000).toISOString();
  f.evidence.finishedAt = new Date(clock - 600_000).toISOString();
  const planBytes = JSON.stringify(f.plan);
  f.evidence.planDigest = digest(planBytes);
  const evidenceBytes = JSON.stringify(f.evidence);
  const planFile = path.join(f.root, "plan.json");
  const evidenceFile = path.join(f.root, "evidence.json");
  await writeFile(planFile, planBytes);
  await writeFile(evidenceFile, evidenceBytes);
  const result = spawnSync(process.execPath, [
    fileURLToPath(new URL("./validate-product-readiness.mjs", import.meta.url)),
    "--plan", planFile, "--evidence", evidenceFile, "--root", f.root,
  ], { encoding: "utf8" });
  assert.equal(result.status, 0, result.stdout);
  assert.deepEqual(JSON.parse(result.stdout), {
    schemaVersion: "product-readiness-check/v1", contentVerified: true, executionAuthorized: false, reasonCodes: [],
  });
  assert.equal(result.stderr, "");
  assert.equal(await readFile(planFile, "utf8"), planBytes);
  assert.equal(await readFile(evidenceFile, "utf8"), evidenceBytes);
});
