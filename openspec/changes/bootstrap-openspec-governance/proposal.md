## Metadata

- Change ID: `bootstrap-openspec-governance`
- Tier: T0
- Planes: runtime-governance
- Affected Specs: `deployment-governance/GOV-005`, `deployment-governance/GOV-006`, new `deployment-governance/GOV-007`
- Depends On: none
- Target Milestone: Stage-0
- Risk: low

## Why

HNB Cloud 已建立 18 个领域主规格，但尚缺少可重复执行的仓库级质量门禁来阻止无稳定 ID、无 Scenario、无 Traceability 或格式无效的规格进入主分支。先固化治理检查可以为后续 T0/T1 change 提供一致、可审计的交付基线，避免实现与规格失配。

## What Changes

- 增加自动化 OpenSpec 严格校验入口，覆盖主规格和活动 change。
- 增加 Requirement ID 唯一性、Scenario 完整性和 Traceability 完整性检查。
- 在持续集成中执行上述检查，并提供本地可复现的同一检查入口。
- 建立 Requirement ID 到 proposal、design、tasks 和验证证据的引用约定。
- 记录校验失败原因和修复方式，作为后续 change 的统一贡献指南。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `deployment-governance`: 增加仓库级自动规格质量门禁，落实 GOV-005 双向追踪和 GOV-006 归档 Definition of Done。

## Non-Goals

- 不实现平台运行时、Provider、Operation Engine 或业务 Portal。
- 不引入新的数据库、中间件、常驻服务或 SaaS 依赖。
- 不修改现有 18 个领域的业务范围和 Requirement ID。
- 不在本 change 中实现后续 change 的业务验收测试。

## Compatibility and Migration

- 不修改公共 API、事件、Manifest 或数据库 Schema。
- 现有主规格无需数据迁移；不符合新门禁的后续提交必须先修复才能合并。
- 检查入口使用仓库当前 OpenSpec CLI 1.3.x 支持的命令；升级 CLI 时需先验证命令兼容性。
- 回滚仅需移除新增检查入口和 CI 调用，不影响主规格内容或运行环境。

## Security and Isolation

- 检查仅读取仓库内 OpenSpec 文件，不访问运行 Secret、租户数据或 RuntimeTarget。
- CI 使用最小只读仓库权限，不上传规格内容到未批准的外部服务。
- 校验输出不得打印凭据；发现疑似 Secret 时按仓库安全流程处置。

## Reliability and Operations

- 检查必须在无网络依赖的环境中可重复执行，失败时返回非零退出码和可定位文件信息。
- 相同提交上的本地检查与 CI 检查应产生一致结论。
- 检查耗时和失败原因进入 CI 日志；不新增运行时指标、备份或灾备要求。
- 资源预算为单个 CI 作业，目标是在常规开发环境中 60 秒内完成 18 个主规格及活动 change 校验。

## Rollout and Rollback

- 先以独立脚本或任务入口验证现有基线，再接入必需 CI 检查。
- 门禁启用前必须确认当前 18 个主规格全部通过严格校验。
- 若门禁因 OpenSpec CLI 兼容问题阻塞所有提交，可临时回滚 CI 调用，但必须保留失败记录并创建修复 change。
- 本 change 无不可逆步骤。

## Exit Criteria

- **GIVEN** 当前 18 个主规格和一个结构完整的示例 change，**WHEN** 执行仓库规格检查，**THEN** 检查成功且返回零退出码。
- **GIVEN** 一个缺少 Scenario、Traceability 或使用重复 Requirement ID 的测试夹具，**WHEN** 执行同一检查，**THEN** 检查失败并指出违规类型和文件。
- **GIVEN** 一个仅修改文档的提交，**WHEN** 本地和 CI 分别执行检查，**THEN** 两者产生一致结果且总耗时不超过批准预算。
