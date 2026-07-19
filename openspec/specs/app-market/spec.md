# app-market

## Purpose
定义独立应用市场与运行治理平面的部署、数据和凭据边界，以及产品、发布、渠道、授权、订阅、发布门禁和发布者治理行为。

## Requirements

### Requirement: [MKT-001] 市场独立部署与数据隔离
HNB App Market SHALL 独立部署、独立升级、独立备份恢复并使用独立数据库；市场 SHALL NOT 直接访问业务集群。

**Traceability:** MKT-01, INT-02, INT-05

#### Scenario: 市场整体不可用
- **GIVEN** 一个应用已经部署完成
- **WHEN** 市场服务停止
- **THEN** 已运行应用和平台运行治理不受影响
- **AND** 市场恢复后可以继续提供目录和发布服务

### Requirement: [MKT-002] 统一产品与发布模型
市场 SHALL 提供 Publisher、Product、Package、Artifact、Release、Channel、CompositionRelease、Entitlement 和 Subscription，并允许应用、数据库、中间件、AI 与边缘产品使用同一发布模型。

**Traceability:** MKT-02, MKT-03, MKT-04, MKT-08

#### Scenario: 发布数据库产品
- **GIVEN** 发布者准备 PostgreSQL 高可用产品
- **WHEN** 提交 stable Release
- **THEN** 市场形成不可变 Release 并关联全部 Package 与摘要
- **AND** 产品可按分类、标签和授权被检索

### Requirement: [MKT-003] 发布不可变与渠道晋级
Release 进入发布渠道后 SHALL 不可原地覆盖；渠道晋级、弃用和撤销 SHALL 产生新的状态记录和审计。

**Traceability:** MKT-06, MKT-07

#### Scenario: 覆盖 stable 版本
- **GIVEN** 一个 Release 已进入 stable
- **WHEN** 发布者用相同版本号推送不同摘要
- **THEN** 市场拒绝覆盖
- **AND** 要求创建新版本或新 Release

### Requirement: [MKT-004] 市场不保存运行凭据
市场 SHALL NOT 保存 kubeconfig、目标侧访问 Token 或租户运行 Secret；市场触发交付时 SHALL 仅提交 ReleaseManifest/CompositionRelease 和授权上下文。

**Traceability:** MKT-09, INT-07

#### Scenario: 市场请求部署
- **GIVEN** 租户选择一个已授权产品
- **WHEN** 市场向平台提交部署请求
- **THEN** 平台独立完成目标选择、Secret 解析和预检
- **AND** 市场无法直接调用 Kubernetes 写 API

### Requirement: [MKT-005] 发布门禁与发布者沙箱
进入 stable 的 Release SHALL 通过 Schema、兼容性、签名、SBOM、漏洞、许可证和适用于该 Package 类型的 Conformance 门禁；安装发布者沙箱能力后，ISV SHALL 可在隔离沙箱执行同等预检。

**Traceability:** MKT-11, GOV-02

#### Scenario: 未通过兼容性测试的产品申请 stable
- **GIVEN** 发布者提交一个缺失 arm64 声明的边缘产品
- **WHEN** 发布审核执行
- **THEN** 发布被拒绝并返回失败证据
- **AND** 失败结果可在发布者沙箱复现
