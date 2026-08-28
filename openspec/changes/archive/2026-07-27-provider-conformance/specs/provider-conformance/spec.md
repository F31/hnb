## ADDED Requirements

### Requirement: [PROV-001] Provider Manifest
每个 Provider SHALL 声明名称、版本、协议版本、支持的 Capability、RuntimeTarget、生命周期动作、权限、资源需求、依赖、兼容范围和健康检查。

**Traceability:** ART-STO-11, GW-06

#### Scenario: 注册不完整 Provider
- **GIVEN** 一个 Provider 缺少权限声明
- **WHEN** 提交到 Provider Registry
- **THEN** 注册被拒绝
- **AND** 返回缺失字段和 Schema 版本

### Requirement: [PROV-002] 进程与故障隔离
T1 及以上外部 Provider SHOULD 独立进程或容器运行；Provider 故障 SHALL 不导致 HNB Core 崩溃或阻塞无关领域 Operation。

**Traceability:** AI-ARCH-02

#### Scenario: 数据库 Provider 崩溃
- **GIVEN** 平台同时运行应用部署和数据库备份
- **WHEN** 数据库 Provider 停止响应
- **THEN** 数据库 Operation 超时并告警
- **AND** 应用部署仍可继续

### Requirement: [PROV-003] 统一生命周期契约
Domain Provider SHALL 通过标准契约实现 Validate、Plan、Provision、Observe、Update、Scale、Backup、Restore、Delete 等其声明支持的动作；未声明动作 SHALL 在预检阶段被拒绝。

**Traceability:** AI-RUN-04, GW-07

#### Scenario: 调用未支持的恢复动作
- **GIVEN** 一个轻量缓存 Provider 未声明 Restore
- **WHEN** 用户请求恢复
- **THEN** 平台预检拒绝
- **AND** 界面不展示不可用动作

### Requirement: [PROV-004] Conformance 认证
Provider SHALL 经过契约测试、功能测试、故障测试、安全测试和性能基线；Production Ready 状态 SHALL 绑定证据、版本和认证有效期。

**Traceability:** GW-18, GOV-02

#### Scenario: 升级 Provider 主版本
- **GIVEN** 一个已认证 Provider 发布新主版本
- **WHEN** 申请 Production Ready
- **THEN** 必须重新运行完整认证
- **AND** 旧版本证据不得自动继承

### Requirement: [PROV-005] 版本与兼容矩阵
平台 SHALL 维护 HNB Core、Provider、RuntimeTarget、外部 CRD/Controller 和数据面版本的兼容矩阵；不支持组合 SHALL 被阻止。

**Traceability:** GW-04, GOV-05

#### Scenario: 选择不兼容 Gateway 组合
- **GIVEN** 目标 Kubernetes 与 Gateway Bundle 不兼容
- **WHEN** 用户选择该 GatewayProfile
- **THEN** 平台拒绝安装并展示矩阵原因
- **AND** 矩阵不应写死在业务代码中