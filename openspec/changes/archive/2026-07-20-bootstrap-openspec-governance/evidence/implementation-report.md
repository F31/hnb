# bootstrap-openspec-governance 实施证据

## 执行环境

- 日期：2026-07-20
- 平台：Linux x64
- Node.js：20.20.2
- OpenSpec CLI：1.3.1

## 自动测试

命令：

```bash
node --test scripts/validate-openspec.test.mjs
```

结果：10 tests passed，0 failed。覆盖文件发现、归档排除、代码围栏、ID 缺失与冲突、MODIFIED 合法复用、Traceability、Scenario 关键字、批准版本以及退出码 0/1/2。

## 当前基线

命令：

```bash
node scripts/validate-openspec.mjs
```

结果：

```text
OpenSpec 质量门禁通过: 23 specs, 105 requirements, 122 scenarios, 105 traceability, 2497 ms
环境: Node.js 20.20.2, OpenSpec 1.3.1, linux/x64
```

归档后 23 个 spec 文件由 18 个主规格和 5 个活动 delta spec 组成。执行时间低于 60 秒预算。

## 严格校验

命令与结果：

```text
openspec validate bootstrap-openspec-governance --strict --no-interactive
Change 'bootstrap-openspec-governance' is valid

openspec validate --all --strict --no-interactive
Totals: 20 passed, 0 failed (20 items)
```

## Requirement 追踪

| Requirement | 实现 | 自动测试与验证 | 文档与运维证据 | 状态 |
|---|---|---|---|---|
| GOV-005 | `scripts/validate-openspec.mjs` 的 ID、文件位置和 MODIFIED 处理 | ID、冲突、MODIFIED 测试；全仓门禁 | `doc/OpenSpec_治理与贡献指南.md` | 完成 |
| GOV-006 | 统一严格校验、适用性和归档前检查 | change 与全仓 strict validation | 指南的适用性、回滚和证据约定 | 完成 |
| GOV-007 | 统一入口、版本检查、语义检查、计数和退出码 | 10 个治理测试；归档后基线 2.50 秒 | 本地运行、排错与仓库验证策略 | 完成 |

## 适用性

网络 API、事件、数据库迁移、运行时 E2E、Provider Conformance、备份和灾备均为 N/A。本 change 仅读取仓库文本，不访问租户、Secret、数据库或 RuntimeTarget。

## Repository policy and rollback

GitHub is used only for code hosting. The two GitHub Actions workflows were removed, queued runs were cancelled, and the temporary self-hosted runner was unregistered and removed. `evidence/repository-validation-policy.md` records the decision, security handling, rollback rehearsal, and accepted limitation.

Removing the CI adapters did not affect main specification reads, the unified gate, or automated tests. The post-archive final verification passed with 23 tests, 20 strict OpenSpec items, and no npm high-severity vulnerabilities. No unresolved governance blocker remains.
