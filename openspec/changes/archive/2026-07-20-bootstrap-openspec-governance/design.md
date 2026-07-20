## Context

当前仓库包含 `openspec/config.yaml`、18 个主规格和 change 目录，但没有应用代码、构建系统、脚本目录或 CI 配置。OpenSpec CLI 1.3.1 的 `validate --all --strict` 能检查规格结构和 Scenario 格式，但不保证 Requirement ID 跨文件唯一，也不强制 Traceability；因此仅调用 CLI 不能完整满足 GOV-005、GOV-006 和 GOV-007。

本 change 的利益相关方包括规格作者、代码评审者和后续阶段 0 实现团队。设计必须保持离线可执行、无运行时服务依赖，并避免把任何具体业务平面或自动化产品编译进治理逻辑。

### Architecture

```text
Developer / Reviewer
      |
      v
scripts/validate-openspec.mjs
      |--------------------------|
      v                          v
OpenSpec strict validation   Repository semantic checks
      |                       - Requirement ID uniqueness
      |                       - Traceability presence
      |                       - GIVEN/WHEN/THEN presence
      |--------------------------|
                 v
       Exit code + text summary
```

## Goals / Non-Goals

**Goals:**

- 提供一个本地和独立验证环境可复用的单一校验命令。
- 组合 OpenSpec 原生严格校验与仓库特定语义检查。
- 对违规项返回非零退出码、违规类型和文件位置。
- 输出 spec、Requirement、Scenario 和 Traceability 计数作为验证证据。
- 使用 GOV-005、GOV-006 和 GOV-007 作为任务与测试追踪依据。

**Non-Goals:**

- 不实现 OpenSpec CLI 已提供的 Markdown 解析器或 change 状态机。
- 不引入数据库、消息系统、常驻服务或网络调用。
- 不引入 GitHub Actions、GitLab CI、Jenkins 或其他 CI 产品；GitHub 仅用于代码托管。
- 不检查业务实现是否真正满足 Requirement；该能力由后续 change 的测试证据承担。
- 不修改平台 API、事件、租户数据或任何 RuntimeTarget。

## Decisions

### Decision 1: 使用一个 Node.js 标准库脚本作为统一入口

新增 `scripts/validate-openspec.mjs`，由其调用 `openspec validate --all --strict --no-interactive`，随后扫描 `openspec/specs/*/spec.md` 和活动 change 下的 delta spec。脚本只使用 Node.js 标准库，避免为阶段 0 治理引入 package manifest 和第三方依赖。

选择 Node.js 是因为当前 OpenSpec CLI 本身依赖 Node.js，执行环境已经具备该运行时。备选方案是 Shell + `rg`/`awk`，但不同操作系统工具行为不一致且难以可靠报告 Requirement block 位置；另一个备选方案是 Python，但会增加第二运行时前置条件。

### Decision 2: 原生格式校验和仓库语义校验分层

OpenSpec CLI 负责 schema、Purpose、Requirement 和 Scenario 的规范解析；自定义脚本只检查稳定 ID、全局唯一性、Traceability 和 GIVEN/WHEN/THEN 完整性。脚本 SHALL NOT 复制 OpenSpec 的完整语法规则。

这种分层减少与 OpenSpec 升级的耦合。备选方案是自行解析全部 Markdown，但会形成难以维护的第二套规范实现。

### Decision 3: GitHub 仅托管代码，门禁在提交前本地执行

当前不配置持续集成适配层。开发者和评审者安装批准版本的 `@fission-ai/openspec` 并执行 `node scripts/validate-openspec.mjs`，将命令、版本、提交 SHA 和结果保存为评审证据。未来若通过独立 change 接入自动化，不得复制检查逻辑。

OpenSpec 版本固定到已验证版本，升级通过独立 change 或显式依赖更新完成。使用浮动 latest 的方案被拒绝，因为会造成同一提交在不同日期出现不同结果。

### Decision 4: 失败输出面向修复而非机器协议

首期输出稳定的文本摘要和非零退出码，每个错误包含检查类型、相对文件路径和行号。暂不定义 JSON API；若后续需要质量驾驶舱，再通过独立 change 增加机器可读输出。

## Data Model

校验过程只在内存中维护以下临时记录，不持久化业务数据：

```text
RequirementRecord
- id: string
- title: string
- file: repository-relative path
- line: positive integer
- hasTraceability: boolean
- scenarioCount: non-negative integer
- scenarioKeywords: set<GIVEN | WHEN | THEN>
```

Requirement ID 从标题 `[<ID>]` 提取并在全部主规格和活动 delta spec 范围内检查。归档目录不参与活动 change 检查，主规格仍始终参与检查。

## API and Event Contracts

本 change 不新增网络 API、事件或 Manifest。唯一命令契约为：

```text
node scripts/validate-openspec.mjs

exit 0: 全部检查通过
exit 1: 规格或语义检查失败
exit 2: 执行环境错误，例如 openspec 不存在
```

标准输出包含成功摘要；标准错误包含违规明细。该命令不读取环境 Secret，不产生事件，也不访问外部网络。

## State Machine

```text
Start
  -> CheckPrerequisites
  -> RunOpenSpecStrictValidation
  -> ScanSpecificationFiles
  -> ValidateRequirementRecords
  -> ReportSuccess

任一步失败 -> ReportFailure -> ExitNonZero
```

检查是无状态且幂等的；对同一工作树重复执行应得到相同结论。

## Security and Isolation

- **租户隔离:** 不适用。检查不访问租户资源，仅读取仓库规格文本。
- **Secret:** 脚本不读取环境变量值或运行凭据，错误输出只包含相对路径和规格文本位置。
- **供应链:** 仓库固定 OpenSpec 包版本和批准的 Node.js 版本。
- **权限:** 本地门禁仅需仓库读取权限；脚本不修改主规格。
- **审计:** 版本化证据保存命令、版本、结果、计数和关联提交，作为 GOV-007 证据。
- **跨平面边界:** 不访问任何平面数据库，不调用 RuntimeTarget，不产生 Operation，因此不存在数据库共享、执行旁路或数据面代理。

## Performance, Capacity, and Observability

- 容量基线为 18 个主规格及活动 change；算法按文件总字节数和 Requirement 数量线性增长。
- 目标是在常规 Linux 开发环境上 60 秒内完成，主要耗时来自 OpenSpec CLI 启动。
- 输出检查文件数、Requirement 数、Scenario 数、Traceability 数、OpenSpec 版本和总耗时。
- 不新增运行时指标、日志后端或告警服务；门禁失败必须阻止提交进入评审。

## Failure Modes

- `[openspec 未安装或版本不兼容]` -> 返回环境错误并打印安装/版本要求，不跳过原生校验。
- `[Markdown 无法由 OpenSpec 解析]` -> 保留 OpenSpec 原始错误并终止。
- `[重复 ID]` -> 输出所有冲突位置，而不是只报告第一个。
- `[活动 change 与主规格暂时同 ID]` -> MODIFIED Requirement 允许复用对应主规格 ID；ADDED Requirement 与任何现有 ID 冲突时失败。
- `[未使用 CI]` -> 统一命令是权威入口；提交者和评审者保存本地执行证据，不以 GitHub 状态检查替代。

## Risks / Trade-offs

- `[轻量 Markdown 扫描可能误识别代码块中的标题]` -> 仅扫描 OpenSpec 约定目录，并忽略 fenced code block；用测试夹具覆盖。
- `[OpenSpec CLI 升级改变输出或命令]` -> 固定已验证版本并通过独立兼容测试升级。
- `[只检查规格结构，不能证明业务实现正确]` -> 明确门禁范围；后续 change 仍必须提供 Requirement 对应测试证据。
- `[本地门禁依赖流程纪律]` -> 每个 change 的 verify 和归档证据必须包含统一命令结果；缺失证据不得勾选完成。

## Migration Plan

1. 在当前 18 个主规格上运行原生严格校验并记录基线结果。
2. 添加语义校验脚本及有效、重复 ID、缺失 Traceability、缺失 Scenario 关键字测试夹具。
3. 在本地执行统一命令，确认有效基线通过且无效夹具按预期失败。
4. 在提交和评审约定中固定 OpenSpec 版本与统一命令。
5. 保存当前提交的首次本地成功证据并验证失败夹具返回非零状态。

回滚时只移除新增脚本；主规格及运行环境不发生迁移。再次启用前必须修复导致回滚的兼容问题。

## Upgrade, Rollback, and Disaster Recovery

- OpenSpec 或 Node.js 升级先在独立工作树验证，再更新批准版本。
- 脚本无持久化状态，回滚使用版本控制恢复上一版本。
- 备份、恢复和灾备不适用；仓库版本控制是唯一恢复来源。

## Open Questions

- 未来若引入独立 CI，使用哪一种隔离执行环境和证据保留策略？该问题不阻塞当前本地门禁。
