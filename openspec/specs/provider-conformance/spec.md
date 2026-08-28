# provider-conformance

## Purpose
定义 Provider 的 Manifest 注册、标准生命周期契约、进程与故障隔离、版本兼容矩阵，以及 Production Ready Conformance 认证行为。
## Requirements
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

### Requirement: [PROV-006] Extension controller owns Provider lifecycle
Provider installation, enablement, upgrade, rollback, health reconciliation and uninstall SHALL be reconciled by extension-controller or an equivalent dedicated lifecycle controller. platform-api SHALL expose Provider catalog and compatibility APIs but SHALL NOT directly deploy Provider Bundles on request paths.

**Traceability:** PROV-001, PROV-003, PROV-004, KERNEL-001

#### Scenario: User installs a Provider Bundle
- **GIVEN** a signed Provider Bundle with a verified manifest and digest
- **WHEN** an authorized user requests installation
- **THEN** the platform creates or correlates an Operation
- **AND** extension-controller reconciles the Bundle deployment and status

#### Scenario: platform-api receives a Provider catalog query
- **GIVEN** a user requests Provider metadata
- **WHEN** platform-api serves the query
- **THEN** it returns catalog and compatibility data
- **AND** it does not deploy or mutate Provider runtime workloads as part of that query

### Requirement: [PROV-007] Capability and navigation metadata registration
After a Provider lifecycle transition succeeds, the controller SHALL update capability registry and raw plugin/navigation metadata snapshots using versioned, tenant-safe records. The metadata SHALL include route identity, plugin/component or schema target, required permission, capability conditions, parent/child navigation hierarchy, icon, enabled state, locale, and `sort_order`. apiserver SHALL consume these records or their promoted Console registry projection to compute final navigation, and SHALL NOT infer installed capability solely from browser plugin manifests.

**Traceability:** UX-006, PROV-001, CONTRACT-001

#### Scenario: Provider exposes a new menu route
- **GIVEN** a Provider Bundle declares a menu route and required permission
- **WHEN** the Bundle is enabled successfully
- **THEN** the raw metadata snapshot includes the route and permission
- **AND** apiserver only returns it to users with matching capability and permission

#### Scenario: Provider registers ordered menu metadata
- **GIVEN** a Provider Bundle declares a menu route and required permission
- **WHEN** the Provider is enabled
- **THEN** the raw metadata snapshot includes route, permission, parent relationship, locale, and `sort_order`
- **AND** apiserver only returns it to users with matching capability and permission in the stored order

#### Scenario: Provider unregister hides navigation
- **GIVEN** a Provider is disabled or uninstalled
- **WHEN** its navigation metadata is no longer active
- **THEN** apiserver omits its routes and menu items from `/api/v1/navigation/menus`
- **AND** Web unloads or prevents access to those plugin routes after navigation refresh

### Requirement: [PROV-008] Safe upgrade and rollback
Provider upgrade SHALL install the target version as a candidate, verify digest/signature, run compatibility and health checks, and promote only after success. Rollback SHALL restore the previous active version and capability/navigation snapshot. Failed upgrades SHALL leave the previous active Provider serving existing Operations unless policy requires disablement.

**Traceability:** PROV-004, PROV-005, GOV-05

#### Scenario: Upgrade health check fails
- **GIVEN** Provider version `v1` is active and version `v2` is requested
- **WHEN** `v2` fails its health or conformance gate
- **THEN** `v1` remains active
- **AND** the Operation records failure evidence and rollback status

### Requirement: [PROV-009] Uninstall refusal and dependency checks
Provider uninstall SHALL be refused while active Operations, RuntimeTargets, capabilities, release plans, navigation routes or protected resources still depend on that Provider. The refusal response SHALL list dependency categories and safe remediation steps.

**Traceability:** KERNEL-002, PROV-003, CONTRACT-003

#### Scenario: Active Operation depends on Provider
- **GIVEN** an active Operation has a step assigned to Provider `p1`
- **WHEN** a user requests uninstall of `p1`
- **THEN** the controller refuses uninstall
- **AND** the response identifies active Operation dependency as a blocker

### Requirement: [PROV-010] Provider lifecycle events contain metadata only
Provider lifecycle commands and events SHALL contain Provider IDs, versions, artifact digests, operation IDs, capability IDs and SecretReferences only. They SHALL NOT contain inline Secret values, kubeconfigs, tokens or Provider artifact bytes.

**Traceability:** CONTRACT-005, PROV-001, SEC-001

#### Scenario: Lifecycle event includes a token
- **GIVEN** a lifecycle event payload contains an inline token field
- **WHEN** the event contract validator runs
- **THEN** the event is rejected before publish
- **AND** logs report only the field path and violation type
