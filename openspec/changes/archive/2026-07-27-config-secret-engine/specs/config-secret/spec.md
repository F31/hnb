## ADDED Requirements

### Requirement: [CFG-001] 配置分层与版本化
平台配置 SHALL 支持默认值、部署档位、环境、租户和实例分层覆盖；生效配置 SHALL 生成不可变版本或摘要并可回滚。

**Traceability:** METH-01

#### Scenario: 租户覆盖默认参数
- **GIVEN** 平台默认副本数为 2
- **WHEN** 租户在允许范围内设置为 3
- **THEN** 实例记录最终解析配置摘要
- **AND** 回滚可恢复上一摘要

### Requirement: [CFG-002] SecretReference-only
公共 API、ReleaseManifest、ExecutionPlan、事件、日志和审计 SHALL NOT 携带明文 Secret；仅允许 SecretReference 或短期令牌。

**Traceability:** MKT-09, INT-07

#### Scenario: 导出执行计划
- **GIVEN** 计划引用数据库密码
- **WHEN** 用户下载计划用于审计
- **THEN** 输出仅包含 SecretReference
- **AND** 日志中敏感字段被脱敏

### Requirement: [CFG-003] 外部密钥系统可替换
平台 SHALL 通过 Secret/KMS Provider 对接 Kubernetes Secret、Vault、企业 KMS/HSM 或云密钥服务；HNB Core SHALL NOT 绑定具体实现。

**Traceability:** GOV-05

#### Scenario: 切换 Vault Provider
- **GIVEN** 环境原使用 Kubernetes Secret
- **WHEN** 运维切换到认证 Vault Provider
- **THEN** 业务 API 和 Release 无需改变
- **AND** 已有 SecretReference 在迁移或保留原 Provider 期间仍可解析
- **AND** 新 Operation 按切换策略使用目标 Provider 解析凭据

### Requirement: [CFG-005] Step 执行时配置解析
Worker 执行 Step SHALL 在调用 Provider 前解析所有 SecretReference 和配置引用；解析结果 SHALL 仅在 Step 执行期间存在于内存中，SHALL NOT 持久化到 Operation 或 Audit 记录。

**Traceability:** OP-004, CFG-002

#### Scenario: 部署数据库时引用密码
- **GIVEN** StepSpec.inputs 包含 database_password: "ref://secrets/db-password"
- **WHEN** Worker 准备执行该 Step
- **THEN** Worker 调用 SecretResolver 获取明文密码
- **AND** 明文仅传递给 Provider 执行上下文
- **AND** 审计记录包含 SecretReference 而非明文
