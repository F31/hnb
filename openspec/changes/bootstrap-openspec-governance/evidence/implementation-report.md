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
OpenSpec 质量门禁通过: 24 specs, 104 requirements, 119 scenarios, 104 traceability, 1581 ms
环境: Node.js 20.20.2, OpenSpec 1.3.1, linux/x64
```

24 个 spec 文件由 18 个主规格和 6 个活动 delta spec 组成。执行时间低于 60 秒预算。

## 严格校验

命令与结果：

```text
openspec validate bootstrap-openspec-governance --strict --no-interactive
Change 'bootstrap-openspec-governance' is valid

openspec validate --all --strict --no-interactive
Totals: 21 passed, 0 failed (21 items)
```

## Requirement 追踪

| Requirement | 实现 | 自动测试与验证 | 文档与运维证据 | 状态 |
|---|---|---|---|---|
| GOV-005 | `scripts/validate-openspec.mjs` 的 ID、文件位置和 MODIFIED 处理 | ID、冲突、MODIFIED 测试；全仓门禁 | `doc/OpenSpec_治理与贡献指南.md` | 本地实现完成；CI 链接待补 |
| GOV-006 | 统一严格校验、适用性和归档前检查 | change 与全仓 strict validation | 指南的适用性、回滚和证据约定 | CI 回滚与最终归档评审待补 |
| GOV-007 | 统一入口、版本检查、语义检查、计数和退出码 | 10 个自动测试；当前基线 1.58 秒 | 本地运行、排错与 CI 接入约束 | 本地实现完成；远端门禁待补 |

## 适用性

网络 API、事件、数据库迁移、运行时 E2E、Provider Conformance、备份和灾备均为 N/A。本 change 仅读取仓库文本，不访问租户、Secret、数据库或 RuntimeTarget。

## 未完成与阻塞

- 任务 4.1：已选择 GitHub Actions 并新增 `.github/workflows/openspec-quality-gate.yml`，本地格式、门禁和测试通过；该文件尚未提交推送，无法提供首次作业链接，也尚未把 `Validate OpenSpec` 配置为 `main` 必需检查。
- 任务 5.1：CI 调用层已创建，但在远端首次生效前无法执行真实的移除与恢复演练。
- 任务 5.3：追踪表已建立，但 CI、回滚和归档证据不完整，不能确认最终 Definition of Done，也不能归档。

解除条件是提交并推送 GitHub Actions 文件、确认首次作业成功、配置 `main` 分支必需检查，然后执行一次 CI 调用移除与恢复演练。工作流只调用现有统一脚本，OpenSpec CLI 固定为 1.3.1。
