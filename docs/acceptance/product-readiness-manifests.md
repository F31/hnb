# 产品阶段证据：只读内容校验

Requirement：PREADY-001、PREADY-004、PREADY-006；任务 1.8/1.9。

契约源：`contracts/schema/readiness/v1/product-readiness.schema.json`。
实现：`scripts/validate-product-readiness.mjs`；使用根工作区已有 Ajv 2020 和 formats，
不新增运行时服务、数据库或中间件。

## 使用

```bash
npm run readiness:test
npm run readiness:check -- --plan /path/to/approved-plan.json --evidence /path/to/run-evidence.json --root /path/to/retained-artifacts
```

- `ReadinessPlan` 由实施/发布方单独提供，包含阶段、预期源码/物料身份、环境范围、
  最低证据层、拓扑、必需断言和已批准的数值预算；`ReadinessEvidence` 引用该计划的
  **文件字节摘要**，记录同一范围的实际执行、测量与报告内容摘要。
- 返回 JSON 的 `contentVerified` 仅表示结构、声明范围、报告字节和预算核对通过。
  `executionAuthorized` 始终为 false；不联网、不执行 evidence 中的命令、不改 registry，
  不更新安装器 EvidenceGate，也不签发生产认证。
- 退出 0：内容核对通过；退出 1：结构/范围/预算/报告等拒绝；退出 2：参数或输入不可读。
  失败输出只有稳定原因码，不回显命令、JSON 内容、路径值或解析器的敏感片段。

## 计划与证据字段

| 对象 | 必需内容 |
|---|---|
| `identity` | 完整 commit、包含工作树内容的 sourceDigest、绑定实际能力/覆写的 configurationDigest、releaseVersion、命名物料摘要；commit 或档位名不能替代实际源码/配置身份 |
| `scope` | OS、架构、Kubernetes、组合、既有 profile 标识、backing、管理集群 ID、故障域 ID；`conditions` 固定硬件/网络/存储/负载条件；业务阶段需独立目标 ID |
| `approval` / `validUntil` | 预算/计划为 approved、批准方及时间、有效期限；草案不能通过，不能在测量后补批 |
| `budgets[]` | id、metric、数值 limit、atMost/atLeast、单位、窗口与采样间隔；禁止空预算 |
| `requiredAssertions` | 至少包含下表阶段最小集合，可增加产品/环境断言，不能缩弱阶段门 |
| evidence | planDigest、匹配的 identity/scope/stage、证据层/status、开始结束时间、脱敏命令、exitCode、断言与测量 |
| `reports[]` | 根目录下的相对文件路径及真实 sha256；断言/测量必须引用已列出的报告，不接受 URL、路径越界或逃逸 symlink |
| `measurements[]` | 关联 budgetId、实际值/单位、窗口/间隔/样本数及 reportPath；未知、重复、缺失或超限拒绝 |

当前身份/预期范围要求精确匹配，不从别的 OS、backing、版本或拓扑推导。普通业务阶段
最低 real_e2e，S6 最低 production_ha；LiteHA/StandardHA/Enterprise 或生产 HA 核对至少
三个不同域及非 bundled backing。域 ID 是被核对的观测声明，不是本工具主动探测结果。
S2/S3/S4/S6/S7 必须存在不同于管理基座的业务目标，不能通过降低计划布尔值绕过。

## 阶段最小断言

| 阶段 | 最小断言 ID |
|---|---|
| S0 | `scope_and_owners_recorded`、`contracts_recorded`、`budgets_approved` |
| S1 | `service_renewal_across_ttl`、`service_revocation_bound`、`agent_enrollment_replay_rejected`、`agent_reconnect_fenced` |
| S2 | `independent_target_import`、`authenticated_observation`、`read_model_freshness`、`disconnect_reconnect`、`idempotent_replay` |
| S3 | `typed_plan_preflight`、`external_persistent_read_write`、`application_upgrade`、`failed_upgrade_recovery`、`delete_retains_data` |
| S4 | `real_release_clean_install`、`independent_implementer`、`install_resume`、`continuous_identity`、`independent_target_import`、`external_persistent_read_write`、`platform_upgrade_recovery`、`application_upgrade`、`failed_upgrade_recovery`、`delete_retains_data` |
| S5 | `ui_cli_consistency`、`diagnostic_journey`、`telemetry_tenant_isolation`、`resource_and_capacity_budget` |
| S6 | `diagnostic_journey`、`resource_and_capacity_budget`、`real_extension_lifecycle`、`ha_failure_drill`、`complete_restore_set`、`measured_rto_rpo` |
| S7 | `single_writer_ownership`、`controlled_handoff`、`gitops_convergence` |

## 信任边界

摘要只能验证内容身份，不能证明报告作者、assertion 或测量的真实性。批准方身份和
报告语义仍须由发布治理核验，未来签名/权威报告登记应复用现有认证机制；本地文件
中填入 `approved`/`passed` 不能取得安装或生产资格。阶段前置证据仍由阶段清单逐项审查，
这个入口不提供自动跨阶段晋升。

本批不发布虚构的“已批准预算”、参考应用摘要或阶段通过报告。测试生成的本地 fixture
只验证校验器的拒绝/接受行为，绝不登记为平台实机证据。实际发布方在 1.3–1.6 固定环境、
数值和应用物料后再创建正式计划与运行证据。
