#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

export const APPROVED_OPENSPEC_VERSION = "1.3.1";
export const MINIMUM_NODE_VERSION = "20.19.0";

function compareVersions(left, right) {
  const leftParts = left.split(".").map(Number);
  const rightParts = right.split(".").map(Number);
  for (let index = 0; index < 3; index += 1) {
    const difference = (leftParts[index] ?? 0) - (rightParts[index] ?? 0);
    if (difference !== 0) return Math.sign(difference);
  }
  return 0;
}

async function listSpecFiles(directory) {
  const files = [];
  let entries;
  try {
    entries = await readdir(directory, { withFileTypes: true });
  } catch (error) {
    if (error.code === "ENOENT") return files;
    throw error;
  }

  for (const entry of entries) {
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...await listSpecFiles(entryPath));
    } else if (entry.isFile() && entry.name === "spec.md") {
      files.push(entryPath);
    }
  }
  return files;
}

export async function discoverSpecFiles(repositoryRoot) {
  const openspecRoot = path.join(repositoryRoot, "openspec");
  const files = await listSpecFiles(path.join(openspecRoot, "specs"));
  const changesRoot = path.join(openspecRoot, "changes");

  let changes = [];
  try {
    changes = await readdir(changesRoot, { withFileTypes: true });
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }

  for (const change of changes) {
    if (!change.isDirectory() || change.name === "archive") continue;
    files.push(...await listSpecFiles(path.join(changesRoot, change.name, "specs")));
  }

  return files.sort();
}

function capabilityFor(relativeFile) {
  const parts = relativeFile.split("/");
  const specsIndex = parts.lastIndexOf("specs");
  return specsIndex >= 0 ? parts[specsIndex + 1] : undefined;
}

export function parseSpec(content, relativeFile) {
  const records = [];
  const lines = content.split(/\r?\n/);
  let deltaType = "MAIN";
  let fenceMarker;
  let current;
  let scenario;

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    const fence = line.match(/^\s*(`{3,}|~{3,})/);
    if (fence) {
      if (!fenceMarker) fenceMarker = fence[1][0];
      else if (fence[1][0] === fenceMarker) fenceMarker = undefined;
      continue;
    }
    if (fenceMarker) continue;

    const deltaHeading = line.match(/^## (ADDED|MODIFIED|REMOVED) Requirements\s*$/);
    if (deltaHeading) {
      deltaType = deltaHeading[1];
      continue;
    }

    const requirement = line.match(/^### Requirement:\s*(?:\[([^\]]+)\])?\s*(.*)$/);
    if (requirement) {
      current = {
        id: requirement[1]?.trim() || undefined,
        title: requirement[2].trim(),
        file: relativeFile,
        line: index + 1,
        capability: capabilityFor(relativeFile),
        deltaType,
        hasTraceability: false,
        scenarios: [],
      };
      records.push(current);
      scenario = undefined;
      continue;
    }

    if (!current) continue;

    if (/^\*\*Traceability:\*\*\s*\S/.test(line)) {
      current.hasTraceability = true;
    }

    const scenarioHeading = line.match(/^#### Scenario:\s*(.*)$/);
    if (scenarioHeading) {
      scenario = {
        title: scenarioHeading[1].trim(),
        line: index + 1,
        keywords: new Set(),
      };
      current.scenarios.push(scenario);
      continue;
    }

    if (scenario) {
      const keyword = line.match(/^- \*\*(GIVEN|WHEN|THEN)\*\*/);
      if (keyword) scenario.keywords.add(keyword[1]);
    }
  }

  return records;
}

function formatLocation(record) {
  return `${record.file}:${record.line}`;
}

export function validateRecords(records) {
  const errors = [];
  const byId = new Map();

  for (const record of records) {
    if (!record.id) {
      errors.push(`[missing-id] ${formatLocation(record)} Requirement 缺少稳定 ID`);
      continue;
    }
    const matches = byId.get(record.id) ?? [];
    matches.push(record);
    byId.set(record.id, matches);
  }

  for (const [id, matches] of byId) {
    if (matches.length <= 1) continue;
    const main = matches.filter((record) => record.deltaType === "MAIN");
    const modified = matches.filter((record) => record.deltaType === "MODIFIED");
    const allowedModifiedPair = matches.length === 2
      && main.length === 1
      && modified.length === 1
      && main[0].capability === modified[0].capability;
    if (!allowedModifiedPair) {
      const locations = matches.map(formatLocation).join(", ");
      errors.push(`[duplicate-id] ${locations} Requirement ID ${id} 重复`);
    }
  }

  for (const record of records) {
    if (record.deltaType === "REMOVED") continue;
    if (!record.hasTraceability) {
      errors.push(`[missing-traceability] ${formatLocation(record)} Requirement ${record.id ?? "<missing>"} 缺少 Traceability`);
    }
    if (record.scenarios.length === 0) {
      errors.push(`[missing-scenario] ${formatLocation(record)} Requirement ${record.id ?? "<missing>"} 缺少 Scenario`);
      continue;
    }
    for (const item of record.scenarios) {
      const missing = ["GIVEN", "WHEN", "THEN"].filter((keyword) => !item.keywords.has(keyword));
      if (missing.length > 0) {
        errors.push(`[incomplete-scenario] ${record.file}:${item.line} Scenario 缺少 ${missing.join("/")}`);
      }
    }
  }

  return errors;
}

function runCommand(command, args, cwd) {
  return spawnSync(command, args, {
    cwd,
    encoding: "utf8",
    shell: process.platform === "win32",
    windowsHide: true,
  });
}

function writeCommandOutput(result) {
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.stderr) process.stderr.write(result.stderr);
}

export async function validateRepository(repositoryRoot = process.cwd()) {
  const startedAt = performance.now();
  const nodeVersion = process.versions.node;
  if (compareVersions(nodeVersion, MINIMUM_NODE_VERSION) < 0) {
    console.error(`[environment] Node.js ${MINIMUM_NODE_VERSION} 或更高版本是必需的，当前为 ${nodeVersion}`);
    return 2;
  }

  const versionResult = runCommand("openspec", ["--version"], repositoryRoot);
  if (versionResult.error) {
    console.error(`[environment] 无法执行 openspec: ${versionResult.error.message}`);
    return 2;
  }
  if (versionResult.status !== 0) {
    writeCommandOutput(versionResult);
    console.error("[environment] 无法读取 OpenSpec CLI 版本");
    return 2;
  }
  const openspecVersion = versionResult.stdout.trim();
  if (openspecVersion !== APPROVED_OPENSPEC_VERSION) {
    console.error(`[environment] OpenSpec CLI 必须为 ${APPROVED_OPENSPEC_VERSION}，当前为 ${openspecVersion || "unknown"}`);
    return 2;
  }

  const strictResult = runCommand(
    "openspec",
    ["validate", "--all", "--strict", "--no-interactive"],
    repositoryRoot,
  );
  if (strictResult.error) {
    console.error(`[environment] OpenSpec 严格校验无法执行: ${strictResult.error.message}`);
    return 2;
  }
  writeCommandOutput(strictResult);
  if (strictResult.status !== 0) return 1;

  let files;
  try {
    files = await discoverSpecFiles(repositoryRoot);
  } catch (error) {
    console.error(`[environment] 无法发现 OpenSpec 文件: ${error.message}`);
    return 2;
  }
  if (files.length === 0) {
    console.error("[semantic] 未发现 openspec/specs 或活动 change 中的 spec.md");
    return 1;
  }

  const records = [];
  try {
    for (const file of files) {
      const relativeFile = path.relative(repositoryRoot, file).split(path.sep).join("/");
      records.push(...parseSpec(await readFile(file, "utf8"), relativeFile));
    }
  } catch (error) {
    console.error(`[environment] 无法读取 OpenSpec 文件: ${error.message}`);
    return 2;
  }

  const errors = validateRecords(records);
  if (errors.length > 0) {
    for (const error of errors) console.error(error);
    console.error(`OpenSpec 语义校验失败: ${errors.length} 个问题`);
    return 1;
  }

  const scenarioCount = records.reduce((total, record) => total + record.scenarios.length, 0);
  const traceabilityCount = records.filter((record) => record.hasTraceability).length;
  const elapsedMilliseconds = Math.round(performance.now() - startedAt);
  console.log(
    `OpenSpec 质量门禁通过: ${files.length} specs, ${records.length} requirements, `
    + `${scenarioCount} scenarios, ${traceabilityCount} traceability, ${elapsedMilliseconds} ms`,
  );
  console.log(`环境: Node.js ${nodeVersion}, OpenSpec ${openspecVersion}, ${process.platform}/${process.arch}`);
  return 0;
}

const isMain = process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (isMain) {
  process.exitCode = await validateRepository();
}
