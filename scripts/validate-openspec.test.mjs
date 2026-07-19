import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { chmod, mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  discoverSpecFiles,
  parseSpec,
  validateRecords,
} from "./validate-openspec.mjs";

const scriptsDirectory = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.dirname(scriptsDirectory);
const validatorPath = path.join(scriptsDirectory, "validate-openspec.mjs");

async function withTemporaryDirectory(callback) {
  const directory = await mkdtemp(path.join(tmpdir(), "hnb-openspec-"));
  try {
    return await callback(directory);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
}

async function writeSpec(root, relativePath, content) {
  const file = path.join(root, relativePath);
  await mkdir(path.dirname(file), { recursive: true });
  await writeFile(file, content, "utf8");
}

function validSpec(id, title = "测试需求") {
  return `# test

## Purpose
用于自动化测试。

## Requirements

### Requirement: [${id}] ${title}
系统 SHALL 执行测试行为。

**Traceability:** TEST-001

#### Scenario: 测试场景
- **GIVEN** 测试前置条件
- **WHEN** 执行测试动作
- **THEN** 返回测试结果
`;
}

test("discovers main and active delta specs while excluding archive", async () => {
  await withTemporaryDirectory(async (root) => {
    await writeSpec(root, "openspec/specs/core/spec.md", validSpec("CORE-001"));
    await writeSpec(root, "openspec/changes/active/specs/core/spec.md", validSpec("CORE-002"));
    await writeSpec(root, "openspec/changes/archive/old/specs/core/spec.md", validSpec("CORE-003"));

    const files = (await discoverSpecFiles(root))
      .map((file) => path.relative(root, file).split(path.sep).join("/"));

    assert.deepEqual(files, [
      "openspec/changes/active/specs/core/spec.md",
      "openspec/specs/core/spec.md",
    ]);
  });
});

test("parses requirement metadata and ignores fenced examples", () => {
  const content = `${validSpec("CORE-001")}
\`\`\`markdown
### Requirement: [FAKE-001] 代码示例
**Traceability:** FAKE
#### Scenario: 不应解析
- **GIVEN** A
- **WHEN** B
- **THEN** C
\`\`\`
`;

  const records = parseSpec(content, "openspec/specs/core/spec.md");

  assert.equal(records.length, 1);
  assert.equal(records[0].id, "CORE-001");
  assert.equal(records[0].capability, "core");
  assert.equal(records[0].hasTraceability, true);
  assert.deepEqual([...records[0].scenarios[0].keywords], ["GIVEN", "WHEN", "THEN"]);
});

test("accepts one MODIFIED requirement matching its main capability", () => {
  const main = parseSpec(validSpec("CORE-001"), "openspec/specs/core/spec.md");
  const delta = parseSpec(
    `## MODIFIED Requirements\n\n${validSpec("CORE-001")}`,
    "openspec/changes/update-core/specs/core/spec.md",
  );

  assert.deepEqual(validateRecords([...main, ...delta]), []);
});

test("rejects missing and conflicting requirement IDs", () => {
  const missing = parseSpec(
    validSpec("CORE-001").replace("[CORE-001] ", ""),
    "openspec/specs/core/spec.md",
  );
  const first = parseSpec(validSpec("DUP-001"), "openspec/specs/first/spec.md");
  const second = parseSpec(validSpec("DUP-001"), "openspec/specs/second/spec.md");

  const errors = validateRecords([...missing, ...first, ...second]);

  assert.ok(errors.some((error) => error.includes("[missing-id]")));
  assert.ok(errors.some((error) => error.includes("[duplicate-id]")));
  assert.ok(errors.every((error) => error.includes("openspec/specs/")));
});

test("rejects missing traceability and incomplete scenarios", () => {
  const content = validSpec("CORE-001")
    .replace("**Traceability:** TEST-001\n\n", "")
    .replace("- **THEN** 返回测试结果\n", "");

  const errors = validateRecords(parseSpec(content, "openspec/specs/core/spec.md"));

  assert.ok(errors.some((error) => error.includes("[missing-traceability]")));
  assert.ok(errors.some((error) => error.includes("[incomplete-scenario]") && error.includes("THEN")));
});

test("current repository passes with a stable success summary", () => {
  const output = execFileSync(process.execPath, [validatorPath], {
    cwd: repositoryRoot,
    encoding: "utf8",
  });

  const summary = output.match(
    /OpenSpec 质量门禁通过: (\d+) specs, (\d+) requirements, (\d+) scenarios, (\d+) traceability, \d+ ms/,
  );
  assert.ok(summary);
  assert.ok(Number(summary[1]) >= 24);
  assert.ok(Number(summary[2]) >= 104);
  assert.ok(Number(summary[3]) >= 119);
  assert.equal(summary[2], summary[4]);
  assert.match(output, /环境: Node\.js \d+\.\d+\.\d+, OpenSpec 1\.3\.1, \w+\/\w+/);
});

test("returns exit 1 for native OpenSpec format errors", async () => {
  await withTemporaryDirectory(async (root) => {
    await writeSpec(root, "openspec/config.yaml", "schema: spec-driven\n");
    await writeSpec(root, "openspec/specs/broken/spec.md", "# broken\n\n## Requirements\n");

    const result = spawnSync(process.execPath, [validatorPath], {
      cwd: root,
      encoding: "utf8",
    });

    assert.equal(result.status, 1, result.stderr);
    assert.match(`${result.stdout}\n${result.stderr}`, /validation|Requirement|Purpose|failed|issues/i);
  });
});

test("returns exit 1 for repository semantic errors", async () => {
  await withTemporaryDirectory(async (root) => {
    const config = await readFile(path.join(repositoryRoot, "openspec/config.yaml"), "utf8");
    const baseline = await readFile(
      path.join(repositoryRoot, "openspec/specs/deployment-governance/spec.md"),
      "utf8",
    );
    await writeSpec(root, "openspec/config.yaml", config);
    await writeSpec(
      root,
      "openspec/specs/deployment-governance/spec.md",
      baseline.replace("**Traceability:** METH-01\n", ""),
    );

    const result = spawnSync(process.execPath, [validatorPath], {
      cwd: root,
      encoding: "utf8",
    });

    assert.equal(result.status, 1, result.stderr);
    assert.match(result.stderr, /\[missing-traceability\]/);
    assert.match(result.stderr, /openspec\/specs\/deployment-governance\/spec\.md:\d+/);
  });
});

test("returns exit 2 when OpenSpec CLI is unavailable", () => {
  const result = spawnSync(process.execPath, [validatorPath], {
    cwd: repositoryRoot,
    encoding: "utf8",
    env: { ...process.env, PATH: "" },
  });

  assert.equal(result.status, 2, result.stderr);
  assert.match(result.stderr, /\[environment\].*openspec/i);
});

test("returns exit 2 when OpenSpec CLI version is not approved", async () => {
  await withTemporaryDirectory(async (root) => {
    const fakeOpenSpec = path.join(root, "openspec");
    await writeFile(fakeOpenSpec, "#!/bin/sh\nprintf '9.9.9\\n'\n", "utf8");
    await chmod(fakeOpenSpec, 0o755);

    const result = spawnSync(process.execPath, [validatorPath], {
      cwd: repositoryRoot,
      encoding: "utf8",
      env: { ...process.env, PATH: root },
    });

    assert.equal(result.status, 2, result.stderr);
    assert.match(result.stderr, /OpenSpec CLI 必须为 1\.3\.1，当前为 9\.9\.9/);
  });
});
