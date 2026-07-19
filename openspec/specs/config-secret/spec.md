# config-secret

## Purpose
定义平台配置的分层版本化、SecretReference-only 边界、外部 Secret/KMS Provider 可替换性和边缘凭据保护行为。

## Requirements

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

### Requirement: [CFG-004] 边缘 Secret 保护
边缘 Secret SHALL 加密落盘、最小化缓存并支持节点证书轮换与远程吊销；断连时的可用性策略 SHALL 显式配置。

**Traceability:** EDGE-13

#### Scenario: 吊销边缘节点证书
- **GIVEN** 节点证书被标记为 revoked
- **WHEN** 节点尝试重连
- **THEN** CloudHub 拒绝连接
- **AND** 安全事件进入审计
