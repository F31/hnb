# OpenSpec 治理与贡献指南

## 适用范围

本指南适用于 `openspec/specs/` 下的主规格和 `openspec/changes/` 下的活动 change。归档 change 不参与活动 delta 的语义扫描，主规格始终参与检查。

统一质量门禁只读取仓库文件，不调用网络、不修改规格，也不访问平台运行环境。

## 批准环境

- Node.js 最低版本：`20.19.0`
- 当前验证版本：Node.js `20.20.2`
- OpenSpec CLI 固定版本：`@fission-ai/openspec@1.3.1`

检查脚本拒绝低于最低版本的 Node.js，并拒绝不是 `1.3.1` 的 OpenSpec CLI。升级 OpenSpec 前必须通过独立兼容验证并更新脚本、测试和版本化证据。

## 本地运行

执行提交前完整门禁：

```bash
node scripts/validate-openspec.mjs
```

执行自动测试：

```bash
node --test scripts/validate-openspec.test.mjs
```

完整门禁按以下顺序运行：

1. 检查 Node.js 与 OpenSpec CLI 版本。
2. 执行 `openspec validate --all --strict --no-interactive`。
3. 扫描全部主规格和活动 delta spec。
4. 检查 Requirement ID、Traceability 和 Scenario。
5. 输出 spec、Requirement、Scenario、Traceability 数量和耗时。

## 退出码

| 退出码 | 含义 | 常见处理 |
|---:|---|---|
| `0` | 原生严格校验和仓库语义校验全部通过 | 可以提交评审 |
| `1` | OpenSpec 格式或仓库语义校验失败 | 根据错误类型、路径和行号修正规格 |
| `2` | 执行环境错误 | 安装或切换批准版本，检查命令可执行性和文件权限 |

OpenSpec 原生校验输出会直接保留。仓库语义错误使用 `[missing-id]`、`[duplicate-id]`、`[missing-traceability]`、`[missing-scenario]` 或 `[incomplete-scenario]` 分类。

## Requirement ID

- 每个 Requirement 标题必须使用 `### Requirement: [<ID>] <名称>`。
- ID 应使用稳定的领域前缀和数字序号，例如 `GOV-007`、`OP-007`。
- 新增 Requirement 的 ID 不得与主规格或其他活动 change 重复。
- `MODIFIED Requirements` 可以复用同一 capability 主规格中的对应 ID。
- 两个活动 change 不得同时以 `MODIFIED` 方式复用同一 ID。
- fenced code block 中的示例 Requirement 不参与扫描。

## Traceability

每个主规格、`ADDED` 或 `MODIFIED` Requirement 必须包含非空追踪字段：

```markdown
**Traceability:** GOV-005, METH-04
```

Traceability 应引用上游需求编号或已批准 Requirement ID。proposal、design、tasks、测试和验收报告继续引用相同 ID，以支持从需求反查实现与证据。

`REMOVED Requirements` 仍必须保留稳定 ID，但不强制保留已删除行为的 Scenario 和 Traceability。

## Scenario

每个非 REMOVED Requirement 至少包含一个 Scenario。每个 Scenario 都必须同时包含：

```markdown
#### Scenario: 场景名称
- **GIVEN** 前置条件
- **WHEN** 触发动作
- **THEN** 可验收结果
```

可以在 `THEN` 后添加 `AND`，但 `AND` 不能替代 GIVEN、WHEN 或 THEN。

## 证据引用

任务完成时应在任务描述、评审记录或 change 的 `evidence/` 目录中记录：

- 执行的命令；
- 绑定的代码、Schema 或迁移路径；
- 自动测试、Conformance、E2E 或演练结果；
- 执行环境和组件版本；
- 失败、回滚和恢复结论；
- 适用 Requirement ID。

无法执行的验证不得勾选完成，必须记录阻塞原因和解除条件。

## 仓库与验证策略

GitHub 仓库仅用于代码托管，当前不使用 GitHub Actions 或其他持续集成平台。开发者在提交或评审前必须使用固定版本的 Node.js 和 `@fission-ai/openspec@1.3.1`，并调用：

```bash
node scripts/validate-openspec.mjs
```

评审证据必须记录命令、固定版本、提交 SHA 和完整结果，不得用手工抽查替代统一脚本。未来若通过独立 change 引入 CI，适配层只能调用同一脚本，不得复制解析或校验逻辑。

## 适用性评估

本 change 只增加仓库静态治理，以下项目均为 N/A：

- 网络 API 和事件契约；
- 数据库 Schema 与迁移；
- 平台运行时 E2E；
- Provider 或 RuntimeTarget Conformance；
- 运行时备份、恢复和灾备；
- 租户数据、Secret 和目标资源访问。

仓库版本控制是脚本和文档的恢复来源。移除外部自动化调用不影响本地统一门禁或主规格内容。

## 失败排查

1. 执行 `node --version`，确认版本不低于 `20.19.0`。
2. 执行 `openspec --version`，确认输出精确为 `1.3.1`。
3. 单独执行 `openspec validate --all --strict --no-interactive` 查看原生格式错误。
4. 根据语义错误中的相对路径和行号修复 ID、Traceability 或 Scenario。
5. 执行自动测试后重新运行统一门禁。
