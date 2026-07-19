# security-supply-chain

## Purpose
定义制品从市场发布到目标部署的双重供应链门禁、不可变安全证明、运行准入、撤销分发和安全事件关联行为。

## Requirements

### Requirement: [SEC-001] 双重安全门禁
Release 进入生产渠道时和 ExecutionPlan 部署到目标时 SHALL 分别验证 digest、Cosign 签名、SBOM、漏洞、许可证、来源和策略。

**Traceability:** ART-03, MKT-11

#### Scenario: 签名在发布后被撤销
- **GIVEN** Release 曾通过市场门禁
- **WHEN** 平台部署前重新校验
- **THEN** 部署被拒绝
- **AND** 市场通过不替代运行时门禁

### Requirement: [SEC-002] 不可变证明关联
签名、SBOM、构建证明和扫描结果 SHALL 通过 OCI Referrer 或等价不可变关系绑定主体 digest；同一 digest 的可信结果 MAY 复用。

**Traceability:** ART-08, ART-STO-15

#### Scenario: 复用扫描结果
- **GIVEN** 两个 Release 引用相同镜像 digest
- **WHEN** 第二个 Release 进入门禁
- **THEN** 系统复用有效且未过期的证明
- **AND** 证明的主体摘要完全一致

### Requirement: [SEC-003] 运行准入最小基线
生产目标 SHALL 执行镜像签名、非 root、只读根文件系统、Capability 最小化、资源限制和网络策略等准入基线；例外 SHALL 有期限、责任人和审计。

**Traceability:** CTN-07

#### Scenario: 高权限容器申请上线
- **GIVEN** Pod 请求 privileged
- **WHEN** 未存在批准例外
- **THEN** 准入拒绝部署
- **AND** 返回违反的策略项

### Requirement: [SEC-004] 撤销与离线分发
撤销列表 SHALL 签名并可作为 OCI Artifact/Bundle 分发；断连站点 SHALL 按离线策略继续运行或降级，重连后执行风险处置。

**Traceability:** EDGE-10

#### Scenario: 边缘断连期间版本被撤销
- **GIVEN** 站点离线
- **WHEN** 站点重新连接
- **THEN** 系统同步撤销并按风险策略处置
- **AND** 全过程可审计

### Requirement: [SEC-005] 安全事件统一关联
供应链、准入、运行时、Gateway、AI 和边缘安全事件 SHALL 包含 Tenant、Resource、Artifact digest、Operation 和时间范围，可推送到企业 SIEM。

**Traceability:** ART-07, AI-GOV-05

#### Scenario: 追踪高危镜像影响
- **GIVEN** 扫描器发现高危漏洞
- **WHEN** 安全人员查询影响
- **THEN** 系统关联 Release、实例、Route 和 Operation
- **AND** 可导出标准事件
