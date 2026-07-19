# HNB Cloud OpenSpec 实施基线 V3.8.6

> **评审对象**：`HNB_Cloud_开放可组装云原生平台项目落地方案_V3_8_5.md`、`HNB_Cloud_OpenSpec_Baseline_V3_8_5.md`  
> **评审结论**：**有条件通过；完成本文件所列优化后，可作为阶段 0 与 MVP 的 OpenSpec 实施基线。**  
> **版本定位**：V3.8.6 不扩大 V3.8.5 的业务范围，重点修正 OpenSpec 工程化结构、领域覆盖、可验证性和变更排期。  
> **编制日期**：2026-07-18

---

## 0. 审批结论与优化摘要

### 0.1 审批意见

V3.8.5 的总体架构方向正确，以下内容予以批准并冻结为架构基线：

1. 微内核与 Provider 化；
2. 应用市场、统一制品存储、运行治理、AI Extension Plane 四平面解耦；
3. `CompositionRelease -> ExecutionPlan -> Operation` 唯一执行链；
4. OCI 优先、内容寻址、发布不可变；
5. Gateway API 是规范而不是具体产品；
6. AI 可选增强且无执行旁路；
7. Edge Pack 不是第五平面，云为权威、边可自治；
8. T0-T3 能力分级与阶段门槛。

原 V3.8.5 OpenSpec 基线可作为架构摘要，但存在以下实施风险，未优化前不建议直接交给 AI Coding 批量开发：

| 级别 | 问题 | 风险 | V3.8.6 处理 |
|---|---|---|---|
| P0 | 仍以 `openspec/project.md` 作为主要上下文入口 | 与当前 OPSX 的 `openspec/config.yaml` 工作方式不一致 | 改为 `config.yaml`，按 proposal/specs/design/tasks 注入规则 |
| P0 | 单文件中的标题层级不是拆分后可直接校验的标准 spec 格式 | AI 复制后可能生成无法 validate 的规格 | 每个文件块使用标准 `## Requirements / ### Requirement / #### Scenario` |
| P0 | 仅 11 个领域、37 个 Requirement，无法覆盖 4257 行落地方案 | Provider、接口事件、部署档位、服务蓝图等可能无规格直接开工 | 扩展为 18 个领域和稳定 Requirement ID |
| P0 | change 粒度过大且依赖关系隐含 | 多团队并行时产生重复模型、迁移冲突和执行旁路 | 重构为依赖明确的阶段化 change backlog |
| P1 | Requirement 缺少稳定 ID 和需求编号追踪 | 无法从验收反查设计、代码和测试 | 引入稳定 ID、Traceability 和 verify 证据 |
| P1 | 行为规格与实现建议混合 | 把某个产品选择错误固化为系统事实 | spec 描述行为；具体产品选择放到 design/BOM |
| P1 | 缺少统一 Definition of Done | tasks 勾选完成但未验证迁移、回滚和故障场景 | 增加归档门禁和 DoD |
| P1 | 数据库、中间件、Secret、备份恢复没有独立行为规格 | 首个交付组合无法形成闭环 | 新增 service-blueprint、config-secret、observability-dr |

### 0.2 版本决策

- **总体方案 V3.8.5：批准继续实施。**
- **原 OpenSpec Baseline V3.8.5：批准作为历史输入，不再直接作为仓库基线。**
- **本 OpenSpec V3.8.6：批准作为新仓库初始化、阶段 0 和 MVP change 规划的实施基线。**
- 任何后续架构变化必须通过 `openspec/changes/<change-id>/` 提出，不直接改写已同步的主规格。

---

## 1. 推荐仓库结构与 OPSX 工作方式

当前推荐结构：

```text
openspec/
├── config.yaml
├── specs/
│   ├── platform-kernel/spec.md
│   ├── identity-tenancy/spec.md
│   ├── contracts-events/spec.md
│   ├── app-market/spec.md
│   ├── artifact-storage/spec.md
│   ├── release-package/spec.md
│   ├── composition-operation/spec.md
│   ├── runtime-target/spec.md
│   ├── provider-conformance/spec.md
│   ├── gateway/spec.md
│   ├── service-blueprint/spec.md
│   ├── config-secret/spec.md
│   ├── security-supply-chain/spec.md
│   ├── observability-dr/spec.md
│   ├── portal-experience/spec.md
│   ├── ai-extension/spec.md
│   ├── edge-pack/spec.md
│   └── deployment-governance/spec.md
└── changes/
    ├── <active-change>/
    │   ├── proposal.md
    │   ├── specs/<domain>/spec.md
    │   ├── design.md
    │   ├── tasks.md
    │   └── .openspec.yaml        # 可选：为单个 change 指定 schema
    └── archive/
```

建议使用 `spec-driven` schema。核心流程：

```text
/opsx:explore
/opsx:propose <change>
/opsx:apply <change>
/opsx:verify <change>       # expanded workflow
/opsx:sync <change>
/opsx:archive <change>
```

如团队启用 expanded workflow，可使用 `/opsx:new`、`/opsx:continue`、`/opsx:ff`；否则使用默认 `/opsx:propose` 快速生成 proposal/specs/design/tasks。

---

## 2. `openspec/config.yaml`

将下面内容保存为 `openspec/config.yaml`：

```yaml
schema: spec-driven

context: |
  项目：HNB Cloud 开放可组装云原生平台。
  架构：微内核 + Provider/CapabilityPack；应用市场、制品存储、运行治理、AI Extension Plane 四个逻辑平面解耦。
  执行：Release/CompositionRelease -> ExecutionPlan -> Operation 是唯一写入运行目标的路径。
  制品：OCI 优先、内容寻址、生产固定 digest；市场和平台 API 不代理大文件。
  运行目标：KubernetesTarget、ContainerEngineTarget、EdgeRuntimeTarget；ExternalServiceConnector 不是执行目标。
  服务网络：Gateway API 是 KubernetesTarget 首选规范，实际 Controller/Data Plane 通过 Gateway Provider 接入。
  AI：可独立启停，不是事实源，不得绕过 Operation。
  Edge：Edge Pack 不是第五平面；云为权威、边可自治；KubeEdge 节点不重复部署 HNB Agent。
  能力分级：T0 内核必装；T1 默认交付；T2 标准可选；T3 POC/Conformance 后可选。
  参考架构文档：HNB_Cloud_开放可组装云原生平台项目落地方案_V3_8_5.md。
  基线规格版本：HNB Cloud OpenSpec V3.8.6。

rules:
  proposal:
    - 必须声明 change ID、T0/T1/T2/T3 分级、影响平面、受影响 specs、依赖 change、迁移影响和回滚策略。
    - 必须说明用户价值、非目标、兼容性、安全风险、资源预算、可观测要求和退出判据。
    - 涉及新中间件或数据库时，必须说明为何不能复用已有能力，并列出安装、升级、备份、恢复和卸载影响。
  specs:
    - 使用行为优先的 SHALL/SHALL NOT 语句，不把具体库、类名或内部实现细节写成系统行为。
    - 每个 Requirement 名称前必须包含稳定 ID，并至少包含一个 GIVEN/WHEN/THEN Scenario。
    - 新增或修改 Requirement 时必须填写 Traceability，引用落地方案需求编号或上游 Requirement ID。
    - Delta spec 仅使用 ADDED、MODIFIED、REMOVED Requirements；MODIFIED 必须给出完整新文本。
  design:
    - 必须包含上下文、目标/非目标、架构图、数据模型、API/事件契约、状态机、失败模式和替代方案。
    - 必须包含租户隔离、Secret、供应链、权限、审计、性能预算、容量、升级、回滚、灾备和可观测设计。
    - Provider/RuntimeTarget/Gateway/Edge 变更必须附兼容矩阵与 Conformance 计划。
    - 跨平面设计必须证明不存在数据库共享、执行旁路和数据面代理。
  tasks:
    - 任务按可独立验证的增量拆分，单项建议不超过一个工作日。
    - 必须包含 Schema/API、数据库迁移、实现、单元测试、集成测试、契约测试、E2E、文档和回滚验证。
    - 每个任务引用 Requirement ID；完成时附测试或演练证据。
    - 归档前必须运行 OpenSpec verify，并完成规格 sync。
```

---

## 3. 领域规格文件包

下面每个代码块应分别保存为对应的 `openspec/specs/<domain>/spec.md`。  
主规格表示**当前系统事实**；后续变更只在 `changes/<change-id>/specs/` 中编写 delta，完成后通过 sync/archive 合并。

### 3.1 `openspec/specs/platform-kernel/spec.md`

````markdown
# platform-kernel

## Purpose
定义 HNB Cloud T0 微内核的最小职责、执行边界和故障隔离要求。

## Requirements

### Requirement: [KERNEL-001] 最小内核边界
HNB Core SHALL 仅包含身份与租户上下文、Operation Engine、ExecutionPlan Engine、Read Model、Resource Graph、Provider/Capability Registry、Policy Hook、Audit 与 Agent Gateway；具体 CNI、CSI、数据库、中间件、Gateway、AI Runtime 和边缘实现 SHALL NOT 编译进入内核。

**Traceability:** CTN-01, AI-ARCH-01, ART-STO-24

#### Scenario: 卸载可选能力后内核独立运行
- **GIVEN** 一个已安装 HNB Cloud 的环境
- **WHEN** 全部 T2/T3 能力包被停用或卸载
- **THEN** T0 内核组件仍可启动并通过健康检查
- **AND** 内核镜像依赖清单中不存在具体 Provider 实现镜像

### Requirement: [KERNEL-002] Operation 唯一写入口
所有部署、升级、回滚、扩缩容、备份、恢复、切换、删除、GC、OTA 和高风险配置变更 SHALL 通过持久化 Operation 执行；任何门户、Copilot、Provider 或 Controller SHALL NOT 绕过该状态机直接改变 RuntimeTarget。

**Traceability:** CMPOS-03, CMPOS-04, AI-OPS-02, GW-07, EDGE-18

#### Scenario: 外部组件请求资源变更
- **GIVEN** Gateway Provider 或 Copilot 生成资源变更计划
- **WHEN** 用户或策略批准执行
- **THEN** 平台创建可审计 Operation 并由 Runtime Driver 执行
- **AND** 直接调用集群写 API 的旁路请求被拒绝

### Requirement: [KERNEL-003] 查询与控制解耦
列表、搜索和聚合查询 SHALL 读取 Read Model；控制器 SHALL 通过事件或投影器更新 Read Model，查询接口 SHALL NOT 在请求路径实时遍历全部运行目标。

**Traceability:** METH-04, GW-13, EDGE-14

#### Scenario: 大规模目标下查询应用列表
- **GIVEN** 平台已纳管 100 个以上 RuntimeTarget
- **WHEN** 用户查询应用、Route 或边缘节点列表
- **THEN** 请求从 Read Model 返回
- **AND** 响应时延不随 RuntimeTarget 数量线性增长
- **AND** 结果包含 lastObservedAt 或 lastKnownStateAt

### Requirement: [KERNEL-004] 控制面故障不影响数据面
市场、平台控制面或 AI Extension Plane 不可用时，已运行应用、数据库、中间件、Gateway 数据面和已下发边缘负载 SHALL 继续运行。

**Traceability:** INT-05, AI-ARCH-02, EDGE-04

#### Scenario: 中心控制面中断
- **GIVEN** 一个生产应用已经成功部署并对外提供服务
- **WHEN** 市场和平台 API 同时停止
- **THEN** 应用数据面与 Gateway 数据面继续提供服务
- **AND** 恢复后控制面能够重新对账实际状态
````

### 3.2 `openspec/specs/identity-tenancy/spec.md`

````markdown
# identity-tenancy

## Purpose
定义企业多租户、项目、环境、权限与审批上下文的统一行为。

## Requirements

### Requirement: [TENANT-001] 租户上下文全链路传播
Tenant ID、Project ID、Environment ID、Actor ID 和 Correlation ID SHALL 贯穿 API、数据库、缓存、事件、审计、Provider 调用和可观测数据。

**Traceability:** AI-GOV-03, INT-07

#### Scenario: 跨组件执行一次部署
- **GIVEN** 租户 A 在生产环境提交一个部署请求
- **WHEN** 请求经过平台、Provider 和 Runtime Driver
- **THEN** 每个阶段均可使用同一 Correlation ID 追踪
- **AND** 任何落库记录均包含 Tenant ID

### Requirement: [TENANT-002] 跨租户访问默认拒绝
租户资源、日志、制品授权、模型端点、知识引用、Gateway Route 和成本数据 SHALL 默认隔离；跨租户共享 SHALL 通过显式授权对象建立。

**Traceability:** MKT-08, AI-GOV-03, GW-11

#### Scenario: 租户尝试读取其他租户资源
- **GIVEN** 租户 A 与租户 B 均存在
- **WHEN** 租户 A 查询租户 B 的运行实例或 AI 调用日志
- **THEN** 请求被拒绝
- **AND** 越权尝试进入审计

### Requirement: [TENANT-003] 权限与审批分层
平台 SHALL 支持平台管理员、租户管理员、项目管理员、运维人员、发布者和只读用户等角色，并允许高风险 Operation 绑定审批策略。

**Traceability:** AI-OPS-06, MKT-08

#### Scenario: 高风险数据库切换
- **GIVEN** 普通运维用户提交数据库主备切换
- **WHEN** 策略判断该操作需要审批
- **THEN** Operation 保持 PendingApproval
- **AND** 只有授权审批人确认后才能执行

### Requirement: [TENANT-004] 凭据最小暴露
市场、Portal、Copilot 和普通 Provider SHALL 仅持有 SecretReference；运行凭据 SHALL 由平台凭据服务或目标侧工作负载身份按最小权限解析。

**Traceability:** MKT-09, INT-07

#### Scenario: 市场触发部署
- **GIVEN** 市场发布了一个需要数据库密码的 Release
- **WHEN** 平台生成 ExecutionPlan
- **THEN** 计划只包含 SecretReference 而不包含明文密码
- **AND** 市场数据库中不存在运行 Secret
````

### 3.3 `openspec/specs/contracts-events/spec.md`

````markdown
# contracts-events

## Purpose
定义市场、平台、AI、Edge 与 Provider 之间的版本化 API、事件和幂等契约。

## Requirements

### Requirement: [CONTRACT-001] Schema First 公共契约
跨进程和跨平面的 OpenAPI、Protobuf、事件、Manifest 与 Provider 契约 SHALL 先定义 Schema，再生成 SDK；实现 SHALL NOT 共享内部数据库表或内部语言结构体作为公共契约。

**Traceability:** INT-01, INT-02

#### Scenario: 新增市场发布接口
- **GIVEN** 一个 change 需要新增 Release 查询字段
- **WHEN** 设计进入实现前
- **THEN** 对应 OpenAPI/Schema 先完成评审和兼容性检查
- **AND** 客户端 SDK 由 Schema 生成

### Requirement: [CONTRACT-002] 向后兼容与弃用
公共 API 和事件在同一主版本内 SHALL 保持向后兼容；删除或改变语义的字段 SHALL 经过弃用窗口、兼容读写和迁移计划。

**Traceability:** INT-01, GOV-05

#### Scenario: 升级平台 API
- **GIVEN** 旧版 Market Connector 仍在运行
- **WHEN** 平台升级到新次版本
- **THEN** 旧客户端仍可完成既有调用
- **AND** 弃用字段在文档和遥测中可见

### Requirement: [CONTRACT-003] 幂等与关联
所有写 API、事件消费者和 Provider 命令 SHALL 支持 IdempotencyKey、Correlation ID 和期望版本；重复消息 SHALL NOT 造成重复资源或重复扣费。

**Traceability:** CMPOS-04, EDGE-18

#### Scenario: 事件重复投递
- **GIVEN** 同一个 OperationStarted 事件被投递两次
- **WHEN** 消费者处理第二次投递
- **THEN** 系统识别为重复并返回已处理结果
- **AND** 不会创建第二个运行实例

### Requirement: [CONTRACT-004] 事务事件可靠投递
需要与业务状态一致的事件 SHALL 使用事务 Outbox 或等价机制；事件发布失败 SHALL 可重试且不回滚已提交业务事实。

**Traceability:** INT-01, INT-05

#### Scenario: 发布 Release 后事件总线短时不可用
- **GIVEN** Release 已成功写入市场数据库
- **WHEN** 事件发布失败
- **THEN** Outbox 保留待发送事件并在恢复后投递
- **AND** 事件顺序和去重键可验证
````

### 3.4 `openspec/specs/app-market/spec.md`

````markdown
# app-market

## Purpose
定义独立应用市场的产品、发布、渠道、授权和治理边界。

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
进入 stable 的 Release SHALL 通过 Schema、兼容性、签名、SBOM、漏洞、许可证和 Conformance 门禁；ISV SHALL 可在隔离沙箱执行同等预检。

**Traceability:** MKT-11, GOV-02

#### Scenario: 未通过兼容性测试的产品申请 stable
- **GIVEN** 发布者提交一个缺失 arm64 声明的边缘产品
- **WHEN** 发布审核执行
- **THEN** 发布被拒绝并返回失败证据
- **AND** 失败结果可在发布者沙箱复现
````

### 3.5 `openspec/specs/artifact-storage/spec.md`

````markdown
# artifact-storage

## Purpose
定义 HNB Artifact Storage 的统一 OCI 模型、存储档位、分发和生命周期。

## Requirements

### Requirement: [ART-001] 统一 OCI 逻辑入口
镜像、Helm Chart、JAR/WAR、Operator、配置、模型、Prompt、Guardrail、评测、SBOM 和离线包 SHALL 优先通过统一 OCI Registry Endpoint 与 ArtifactDescriptor 管理。

**Traceability:** ART-08, ART-STO-01, ART-STO-02

#### Scenario: 访问多类型制品
- **GIVEN** 一个 Release 同时包含镜像、Chart 和 SBOM
- **WHEN** Agent 获取短期凭据后拉取制品
- **THEN** 全部制品从同一逻辑 Endpoint 获取
- **AND** 上层不依赖底层 Bucket 路径

### Requirement: [ART-002] 摘要锁定与内容寻址
所有 Artifact SHALL 具有 SHA-256 摘要；生产 ExecutionPlan SHALL 固定 digest，标签仅用于检索和展示。

**Traceability:** ART-02, ART-STO-04

#### Scenario: 标签被重新指向
- **GIVEN** 一个生产实例按 digest 部署
- **WHEN** 同名标签后来指向新镜像
- **THEN** 运行实例和回滚点仍引用原 digest
- **AND** 审计能够证明实际部署内容

### Requirement: [ART-003] 大文件直传直取
Market/Platform API SHALL NOT 代理大文件正文；上传者、Agent、Helm、容器运行时和模型加载器 SHALL 直接访问 Registry/S3 数据面。

**Traceability:** ART-01, ART-STO-03

#### Scenario: 上传大型模型
- **GIVEN** 发布者上传大模型权重
- **WHEN** 平台签发短期上传凭据
- **THEN** 数据不经过 market-api 或 platform-api
- **AND** 控制面仅记录元数据和状态

### Requirement: [ART-004] 存储后端与档位可替换
ArtifactStorageProfile SHALL 支持 Local/PVC/S3 等后端；Minimal SHALL 不强制对象存储，Lite HA 及以上 SHALL 使用共享权威后端并明确 RPO/RTO。

**Traceability:** ART-STO-05, ART-STO-06, ART-STO-07, ART-STO-22, ART-STO-23

#### Scenario: 从 Minimal 升级到 Lite HA
- **GIVEN** 已有 Release 使用本地后端
- **WHEN** 运维迁移到共享 S3 后端
- **THEN** Release 和 digest 引用保持不变
- **AND** 迁移通过 Operation 执行并可恢复

### Requirement: [ART-005] 三级分发与缓存可重建
平台 SHOULD 支持中心权威仓库、区域镜像和边缘缓存三级分发；Mirror/Cache SHALL 非权威、可按水位清理并可从中心重建。

**Traceability:** ART-STO-13, ART-STO-14

#### Scenario: 边缘缓存丢失
- **GIVEN** 某站点 Registry Mirror 数据盘损坏
- **WHEN** 站点恢复网络连接
- **THEN** 缓存可以从权威仓库重新构建
- **AND** 权威 Release 状态不受影响

### Requirement: [ART-006] 安全 GC 与引用保护
制品删除 SHALL 经过引用分析、Tombstone、保留期、锁和 Operation；运行实例、回滚点、组合、灾备和离线 Bundle 引用的制品 SHALL NOT 被回收。

**Traceability:** ART-STO-16, ART-STO-17, ART-STO-18

#### Scenario: 回收仍被回滚点引用的镜像
- **GIVEN** 一个旧 digest 仍是有效回滚点
- **WHEN** 运维执行 GC 预览
- **THEN** 系统列出引用并阻止删除
- **AND** GC 支持暂停、重试、限速和审计
````

### 3.6 `openspec/specs/release-package/spec.md`

````markdown
# release-package

## Purpose
定义 Package、ReleaseManifest、兼容性、传统软件容器化和撤销行为。

## Requirements

### Requirement: [REL-001] ReleaseManifest 完整性
ReleaseManifest SHALL 包含 Package digest、参数 Schema、依赖、targetTypes、架构、资源下限、生命周期、升级路径、安全证明和支持声明。

**Traceability:** MKT-04, CMPOS-02, EDGE-06

#### Scenario: 缺失兼容性声明
- **GIVEN** 一个 Release 未声明 targetTypes
- **WHEN** 发布进入 stable 门禁
- **THEN** 门禁拒绝发布
- **AND** 返回缺失字段列表

### Requirement: [REL-002] JAR/WAR 容器化运行
JAR/WAR 可以作为 OCI Artifact 入库，但运行时 SHALL 由不可变 OCI 镜像或受控标准运行时容器承载；平台 SHALL NOT 使用 systemd 或裸 java 进程直接运行。

**Traceability:** CTN-03, CTN-05, ART-04

#### Scenario: 部署 WAR 应用
- **GIVEN** 市场中存在一个 WAR ArtifactRuntimePackage
- **WHEN** 用户部署到 KubernetesTarget
- **THEN** 平台生成运行时容器、只读制品挂载和健康检查
- **AND** 生产推荐路径固定最终镜像 digest

### Requirement: [REL-003] 多架构与目标兼容预检
声明支持 amd64、arm64、GPU/NPU 或 EdgeRuntimeTarget 的 Release SHALL 在发布和部署阶段分别验证镜像平台、资源、网络和运行时能力。

**Traceability:** CTN-06, EDGE-06

#### Scenario: 部署到 arm64 边缘节点
- **GIVEN** Release 声明支持 EdgeRuntimeTarget
- **WHEN** 镜像缺少 arm64 Manifest
- **THEN** 市场或平台预检拒绝该部署
- **AND** 错误信息指出不兼容的具体 Artifact

### Requirement: [REL-004] 撤销与影响分析
Release 撤销 SHALL 生成签名撤销 Artifact，平台 SHALL 识别受影响实例并按风险策略执行告警、隔离、升级或停止。

**Traceability:** MKT-07, ART-07, EDGE-10

#### Scenario: 高危版本被撤销
- **GIVEN** 一个 Release 在多个租户运行
- **WHEN** 安全管理员发布撤销
- **THEN** 平台展示受影响实例和处置状态
- **AND** 边缘离线站点重连后继续执行处置
````

### 3.7 `openspec/specs/composition-operation/spec.md`

````markdown
# composition-operation

## Purpose
定义 CompositionRelease、ExecutionPlan、Operation 状态机、DAG 和补偿语义。

## Requirements

### Requirement: [OP-001] 不可变 ExecutionPlan
平台 SHALL 将 ReleaseManifest 或 CompositionRelease 解析为不可变 ExecutionPlan，固定目标、Artifact digest、参数、SecretReference、Provider 版本、策略结果和步骤 DAG。

**Traceability:** CMPOS-03, INT-03

#### Scenario: 计划生成后发布被修改
- **GIVEN** 一个 ExecutionPlan 已经批准
- **WHEN** 市场产生新的 Release
- **THEN** 已批准计划仍引用原版本和 digest
- **AND** 重新部署新版本必须生成新计划

### Requirement: [OP-002] DAG 与输出绑定
CompositionRelease SHALL 支持依赖顺序、并行、条件、参数映射和跨节点输出绑定；Helm MAY 作为节点执行器，但 SHALL NOT 成为平台组合编排的唯一机制。

**Traceability:** CMPOS-01, CMPOS-02, CMPOS-08

#### Scenario: 部署应用数据库缓存组合
- **GIVEN** 一个组合包含 PostgreSQL、Valkey 和业务应用
- **WHEN** 平台执行组合
- **THEN** 数据库和缓存就绪后其连接输出被绑定到应用
- **AND** 可并行步骤并行执行

### Requirement: [OP-003] 持久化状态机
Operation SHALL 至少支持 Pending、PendingApproval、Queued、QueuedOffline、InProgress、Paused、Compensating、Succeeded、Failed、Cancelled；终态 SHALL 不再自动迁移。

**Traceability:** EDGE-18, METH-02

#### Scenario: 离线目标上的部署
- **GIVEN** EdgeRuntimeTarget 当前离线
- **WHEN** 用户提交允许排队的部署
- **THEN** Operation 进入 QueuedOffline
- **AND** 超过 maxOfflineDuration 后按策略失败并告警

### Requirement: [OP-004] 幂等恢复与断点续作
Operation Step SHALL 具备幂等键、重试策略、超时、检查点和恢复规则；控制器重启 SHALL 从持久化状态恢复而非重复创建资源。

**Traceability:** CMPOS-04

#### Scenario: Worker 在步骤中重启
- **GIVEN** 一个部署已完成前两步
- **WHEN** platform-worker 重启
- **THEN** Operation 从最近检查点继续
- **AND** 已成功步骤不会重复产生资源

### Requirement: [OP-005] 补偿与有状态安全
失败补偿 SHALL 按资源类型和策略执行；有状态组件失败时默认 SHALL NOT 自动删除数据卷、备份或持久化实例。

**Traceability:** CMPOS-05

#### Scenario: 组合部署中数据库后续步骤失败
- **GIVEN** 数据库已经初始化数据
- **WHEN** 业务应用部署失败并触发补偿
- **THEN** 系统保留数据库数据并将其标记为需人工处理
- **AND** 无状态资源可按策略自动回滚

### Requirement: [OP-006] 审计与证据链
每个 Operation SHALL 记录发起人、审批人、来源 Release、ExecutionPlan digest、策略结果、Provider、步骤日志、最终结果和回滚证据。

**Traceability:** INT-03, AI-OPS-03

#### Scenario: 审计一次生产回滚
- **GIVEN** 生产实例发生回滚
- **WHEN** 审计人员查看 Operation
- **THEN** 可以还原从用户请求到目标资源变化的完整链路
- **AND** 敏感字段经过脱敏
````

### 3.8 `openspec/specs/runtime-target/spec.md`

````markdown
# runtime-target

## Purpose
定义 KubernetesTarget、ContainerEngineTarget、EdgeRuntimeTarget 与 ExternalServiceConnector 的统一模型。

## Requirements

### Requirement: [RT-001] 运行目标分类
平台 SHALL 以 KubernetesTarget、ContainerEngineTarget 和 EdgeRuntimeTarget 作为三类容器执行目标；ExternalServiceConnector SHALL 仅表示外部服务绑定，不得执行容器部署。

**Traceability:** EDGE-01, GOV-05

#### Scenario: 绑定外部数据库
- **GIVEN** 用户选择一个云数据库 Connector
- **WHEN** 应用创建绑定
- **THEN** 平台只生成连接绑定而不尝试部署数据库容器
- **AND** 对象类型在 Read Model 中保持可区分

### Requirement: [RT-002] 主动连接与最小暴露
中心 Kubernetes/Container Engine 目标 SHOULD 使用 Agent 经 mTLS 主动连接平台，不暴露公网管理端口；KubeEdge 节点 SHALL 使用 CloudHub–EdgeHub 隧道且不重复部署 HNB Agent。

**Traceability:** EDGE-02

#### Scenario: 纳管 NAT 后的集群
- **GIVEN** 目标集群无法被中心直接访问
- **WHEN** Agent 主动注册
- **THEN** 平台建立受认证长连接并发现能力
- **AND** 目标侧无需开放入站管理端口

### Requirement: [RT-003] 能力发现与快照
RuntimeTarget SHALL 上报 Kubernetes/运行时版本、架构、资源、CNI/CSI、Gateway API、GPU/NPU、安全和存储能力；平台 SHALL 保存带时间戳的 CapabilitySnapshot。

**Traceability:** GW-04, GW-13

#### Scenario: 能力发生变化
- **GIVEN** 目标集群升级 Gateway Controller
- **WHEN** 下一次能力探测完成
- **THEN** 平台保存新的能力快照并触发兼容性重算
- **AND** 旧快照保留用于审计

### Requirement: [RT-004] 部署前兼容性预检
平台 SHALL 在生成或执行 ExecutionPlan 前比较 ReleaseManifest 与 RuntimeTarget 能力；不兼容 SHALL 在写入目标之前失败。

**Traceability:** EDGE-06, AI-RUN-05

#### Scenario: 目标资源不足
- **GIVEN** Release 需要 8Gi 内存
- **WHEN** 目标仅有 4Gi 可分配内存
- **THEN** 预检拒绝执行并返回差异
- **AND** 不会在目标侧创建部分资源

### Requirement: [RT-005] 目标状态新鲜度
RuntimeTarget 和边缘资源状态 SHALL 包含 observedAt/lastKnownStateAt；超过新鲜度阈值时，写操作 SHALL 排队、拒绝或要求显式风险确认。

**Traceability:** EDGE-14, EDGE-18

#### Scenario: 对陈旧边缘状态执行升级
- **GIVEN** 节点已离线超过策略阈值
- **WHEN** 用户提交升级
- **THEN** 平台显示状态陈旧并按策略进入 QueuedOffline 或拒绝
- **AND** 不得显示为实时成功
````

### 3.9 `openspec/specs/provider-conformance/spec.md`

````markdown
# provider-conformance

## Purpose
定义 Provider 注册、生命周期、隔离、兼容矩阵和认证等级。

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
````

### 3.10 `openspec/specs/gateway/spec.md`

````markdown
# gateway

## Purpose
定义 Gateway API 作为 KubernetesTarget 首选入口规范以及与 API Management、Mesh、AI Gateway 的边界。

## Requirements

### Requirement: [GW-001] Gateway API 优先
新建 Kubernetes 服务入口 SHALL 默认生成 Gateway API 资源；Ingress SHALL 仅用于存量兼容、迁移和明确选择的回退路径。

**Traceability:** GW-01

#### Scenario: 新应用暴露 HTTP 服务
- **GIVEN** 应用通过标准向导申请公网入口
- **WHEN** 平台生成流量资源
- **THEN** 生成 Gateway/HTTPRoute 而非 Ingress
- **AND** 兼容模式必须显式选择

### Requirement: [GW-002] CRD 集中治理
Gateway API CRD Bundle 和 Channel SHALL 由 Cluster Provider 或集群管理员统一管理；生产环境 SHALL 仅使用经认证 Standard Channel。

**Traceability:** GW-02, GW-03

#### Scenario: 租户升级 Gateway CRD
- **GIVEN** 业务租户拥有 Route 管理权限
- **WHEN** 租户尝试安装 Experimental Bundle
- **THEN** 操作被拒绝
- **AND** 审计记录高权限尝试

### Requirement: [GW-003] 标准资源与特性协商
Gateway Provider SHALL 至少支持 GatewayClass、Gateway、HTTPRoute、GRPCRoute、ReferenceGrant 和 BackendTLSPolicy，并以 GatewayCapabilitySnapshot 声明 Route 类型及 Core/Extended 特性。

**Traceability:** GW-05, GW-06, GW-08, GW-09

#### Scenario: 产品要求 gRPC 路由
- **GIVEN** 目标 Provider 未认证 GRPCRoute
- **WHEN** 平台进行预检
- **THEN** 部署被拒绝或选择兼容 Provider
- **AND** 不依据 CRD 存在性推断完整能力

### Requirement: [GW-004] 多租户与跨命名空间授权
共享 Gateway SHALL 使用 allowedRoutes、Namespace Selector、RBAC 和 Tenant Context 隔离；跨 Namespace Backend、Secret 或证书引用 SHALL 需要 ReferenceGrant 或规范允许的显式授权。

**Traceability:** GW-11, GW-12

#### Scenario: 跨命名空间引用后端
- **GIVEN** HTTPRoute 位于租户 A 命名空间
- **WHEN** 其引用租户 B Service 且无 ReferenceGrant
- **THEN** Controller 和平台均拒绝绑定
- **AND** 拒绝原因进入 Route 状态

### Requirement: [GW-005] 流量治理能力
GatewayProfile SHALL 可声明 Host、Path、Header、Query、Method 匹配及权重分流、镜像、重写、重定向、Header 修改、超时和后端 TLS；未认证功能 SHALL 不出现在向导中。

**Traceability:** GW-10

#### Scenario: 灰度发布
- **GIVEN** Provider 已认证权重分流
- **WHEN** 用户配置 90/10 流量
- **THEN** 平台生成可验证 Route
- **AND** Route Accepted 且后端权重正确

### Requirement: [GW-006] 流量产品分层
普通 Gateway、API Management、Service Mesh 和 AI Gateway SHALL 使用独立能力模型、凭据、数据面和审计；普通应用流量 SHALL NOT 被自动导向 AI Gateway。

**Traceability:** GW-15, AI-GW-05

#### Scenario: 普通业务误选 AI Gateway
- **GIVEN** 非 AI ServiceBlueprint 请求普通 HTTP 入口
- **WHEN** ExposurePolicy 指向 AI GatewayProfile
- **THEN** 预检拒绝配置
- **AND** 返回流量治理分层原因
````

### 3.11 `openspec/specs/service-blueprint/spec.md`

````markdown
# service-blueprint

## Purpose
定义面向用户的应用、数据库和中间件服务蓝图及生命周期协商。

## Requirements

### Requirement: [BLUE-001] 服务蓝图抽象
用户 SHALL 通过 ServiceBlueprint 选择应用、数据库或中间件服务；默认体验 SHALL 隐藏底层 CRD、Chart 和 Provider 参数，仅通过 Schema 驱动表单暴露受支持选项。

**Traceability:** MKT-02, METH-03

#### Scenario: 创建 PostgreSQL 服务
- **GIVEN** 用户处于简单模式
- **WHEN** 选择 PostgreSQL ServiceBlueprint
- **THEN** 界面展示容量、可用性和备份等业务参数
- **AND** 不要求用户编写 Kubernetes YAML

### Requirement: [BLUE-002] 生命周期能力协商
ServiceBlueprint SHALL 声明 required/optional 生命周期能力；平台 SHALL 将其与 Provider 和 RuntimeTarget 能力求交集并只展示可执行动作。

**Traceability:** ART-STO-11, AI-RUN-04

#### Scenario: 目标不支持快照
- **GIVEN** 数据库蓝图把快照列为 optional
- **WHEN** 部署到无快照 CSI 的目标
- **THEN** 服务仍可创建但隐藏快照动作
- **AND** required 能力缺失则预检失败

### Requirement: [BLUE-003] 首批数据库服务
首个 T1 产品组合 SHALL 至少提供 PostgreSQL Service Provider，覆盖创建、观察、备份、恢复、升级和删除；高可用、PITR 与故障切换 SHALL 按部署档位声明。

**Traceability:** MKT-02, GOV-05

#### Scenario: 创建带备份的 PostgreSQL
- **GIVEN** 目标具备块存储和备份目标
- **WHEN** 用户创建数据库服务
- **THEN** 服务进入 Ready 并生成连接绑定
- **AND** 备份和恢复 Operation 可验收

### Requirement: [BLUE-004] 首批中间件服务
首个 T1 产品组合 SHALL 在 Valkey 与 RabbitMQ 中至少交付一个标准 Service Provider；Kafka/RocketMQ/MQTT SHALL 作为后续可选 Provider。

**Traceability:** MKT-02

#### Scenario: 部署首批缓存服务
- **GIVEN** 市场已发布 Valkey Release
- **WHEN** 用户通过蓝图创建服务
- **THEN** 平台完成部署、观察、扩缩容和删除
- **AND** 连接信息以 SecretReference 输出

### Requirement: [BLUE-005] 服务绑定与输出
服务实例 SHALL 以标准 Binding 输出 Endpoint、Port、Database/VirtualHost、TLS、SecretReference 和健康状态；应用 SHALL 通过输出绑定消费服务而非读取 Provider 内部对象。

**Traceability:** CMPOS-02, INT-07

#### Scenario: 应用绑定数据库
- **GIVEN** 数据库服务已 Ready
- **WHEN** 组合部署业务应用
- **THEN** ExecutionPlan 将标准 Binding 注入应用
- **AND** 明文密码不出现在计划或审计中
````

### 3.12 `openspec/specs/config-secret/spec.md`

````markdown
# config-secret

## Purpose
定义平台配置、SecretReference、KMS/Vault 和配置变更的安全行为。

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
- **AND** 新 Operation 使用新 Provider 解析凭据

### Requirement: [CFG-004] 边缘 Secret 保护
边缘 Secret SHALL 加密落盘、最小化缓存并支持节点证书轮换与远程吊销；断连时的可用性策略 SHALL 显式配置。

**Traceability:** EDGE-13

#### Scenario: 吊销边缘节点证书
- **GIVEN** 节点证书被标记为 revoked
- **WHEN** 节点尝试重连
- **THEN** CloudHub 拒绝连接
- **AND** 安全事件进入审计
````

### 3.13 `openspec/specs/security-supply-chain/spec.md`

````markdown
# security-supply-chain

## Purpose
定义市场供应链门禁、运行准入、撤销与运行时安全。

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
````

### 3.14 `openspec/specs/observability-dr/spec.md`

````markdown
# observability-dr

## Purpose
定义指标、日志、链路、Operation 观测、备份恢复、灾备和性能预算。

## Requirements

### Requirement: [OBS-001] 统一遥测上下文
平台、市场、Provider、Gateway、AI 和 Edge 组件 SHALL 输出结构化指标、日志、链路和事件，并包含 Tenant、Correlation ID、Operation ID 和 Resource ID。

**Traceability:** AI-GOV-05, GW-13, EDGE-14

#### Scenario: 追踪一次部署故障
- **GIVEN** 部署跨越市场、平台和 Provider
- **WHEN** 运维打开 Operation 详情
- **THEN** 可以跳转到相关日志、链路和资源指标
- **AND** 敏感字段不被采集

### Requirement: [OBS-002] Operation 滞留 SLO
每个非终态 Operation SHALL 配置最大滞留时间、告警和升级策略；状态展示 SHALL 包含进入时间、重试次数和阻塞原因。

**Traceability:** METH-02

#### Scenario: Operation 长时间 InProgress
- **GIVEN** 步骤超过配置 SLO
- **WHEN** 监控检测到滞留
- **THEN** 触发告警并显示阻塞步骤
- **AND** 允许安全暂停或取消

### Requirement: [OBS-003] 备份与恢复是产品能力
平台元数据库、市场数据库、制品数据、签名密钥和服务实例 SHALL 具有版本化备份策略与可执行恢复 Operation；仅存在备份文件 SHALL NOT 视为恢复能力完成。

**Traceability:** ART-STO-19, ART-STO-23

#### Scenario: 执行整站恢复演练
- **GIVEN** 测试环境具备最近备份
- **WHEN** 运维触发恢复 Runbook
- **THEN** 平台、市场和制品引用恢复一致
- **AND** 恢复结果包含 RPO/RTO 实测

### Requirement: [OBS-004] 部署档位故障演练
Lite HA、Standard HA 和 Enterprise 档位 SHALL 分别定义单 Pod、单节点、数据库主实例、Registry 和对象存储故障场景，并通过演练验证。

**Traceability:** ART-STO-07, ART-STO-21

#### Scenario: Registry 单副本故障
- **GIVEN** Lite HA 有多个无状态 Registry 副本
- **WHEN** 一个副本被终止
- **THEN** 制品读取持续成功
- **AND** 告警和自动恢复符合档位目标

### Requirement: [OBS-005] 性能预算可验证
内核 API、Read Model、制品传输、Gateway 数据面、Operation 调度、AI 调用和边缘重连 SHALL 定义测试条件、P95/P99、吞吐和容量上限；结果 SHALL 绑定版本和环境。

**Traceability:** METH-04, GOV-05

#### Scenario: 发布前性能门禁
- **GIVEN** 一个候选版本完成压测
- **WHEN** 任一 P95 超过批准预算
- **THEN** 版本不得标记 Production Ready
- **AND** 异常需要变更豁免或修复

### Requirement: [OBS-006] 断连遥测补传
边缘指标和日志 SHALL 使用有界本地缓冲；重连后 SHALL 限速补传并保持原始时间戳，缓存溢出 SHALL 产生丢弃计数。

**Traceability:** EDGE-14

#### Scenario: 边缘离线后恢复
- **GIVEN** 节点离线 24 小时并产生遥测
- **WHEN** 网络恢复
- **THEN** 遥测按限速策略补传
- **AND** 平台区分事件发生时间和接收时间
````

### 3.15 `openspec/specs/portal-experience/spec.md`

````markdown
# portal-experience

## Purpose
定义简单、标准、专家三层体验、动态表单和场景化向导。

## Requirements

### Requirement: [UX-001] 能力驱动界面
Portal SHALL 根据已安装 CapabilityPack、用户权限和 RuntimeTarget 能力动态显示菜单、表单和动作；未安装能力 SHALL 不出现空菜单或不可执行入口。

**Traceability:** METH-03

#### Scenario: 未安装 Edge Pack
- **GIVEN** 用户登录平台
- **WHEN** Portal 构建导航
- **THEN** 不显示边缘菜单和向导
- **AND** 核心页面无 Edge 依赖错误

### Requirement: [UX-002] 三层操作模式
Portal SHALL 提供简单、标准和专家模式；简单模式面向业务对象，标准模式暴露策略和生命周期，专家模式才允许查看底层资源。

**Traceability:** METH-04

#### Scenario: 新用户交付应用
- **GIVEN** 用户使用简单模式
- **WHEN** 完成发布和部署
- **THEN** 无需直接编辑 Kubernetes CRD
- **AND** 专家模式仍受权限和策略约束

### Requirement: [UX-003] 场景化向导
平台 SHALL 为应用发布、数据库创建、服务暴露、备份恢复、Gateway 迁移、边缘节点纳管和 AI 接入提供可恢复向导，并在提交前展示 ExecutionPlan 摘要。

**Traceability:** GW-14, GOV-04

#### Scenario: 向导中途退出
- **GIVEN** 用户已填写部分部署参数
- **WHEN** 稍后重新进入
- **THEN** 草稿被恢复且未产生目标资源
- **AND** 最终提交生成 Operation
````

### 3.16 `openspec/specs/ai-extension/spec.md`

````markdown
# ai-extension

## Purpose
定义 AI Access、Runtime、Governance、Copilot 和 AIOps 的可选边界。

## Requirements

### Requirement: [AI-001] AI 平面可独立启停
AI Extension Plane SHALL 可独立安装、升级、停用和卸载；停用 SHALL 不影响 T0/T1 平台、普通 Gateway 和传统服务。

**Traceability:** AI-ARCH-01, AI-ARCH-02, AI-ARCH-05

#### Scenario: 停用 AI Access Pack
- **GIVEN** 平台存在传统应用
- **WHEN** AI Gateway 被停用
- **THEN** 传统应用交付和运行不受影响
- **AND** AI 菜单和入口按能力隐藏

### Requirement: [AI-002] 统一模型资源模型
AI 扩展 SHALL 提供 ModelArtifact、ModelService、ModelEndpoint、AIProvider、PromptTemplate、GuardrailPolicy、EvaluationSuite 和 AIUsageRecord，并固定版本、来源、许可证和评测状态。

**Traceability:** AI-RUN-01, AI-RUN-02, AI-RUN-03, AI-GOV-01

#### Scenario: 发布外部模型引用
- **GIVEN** 模型不存储在 HNB Registry
- **WHEN** 发布者登记 externalRef
- **THEN** 系统固定提供方版本、区域、策略和评测状态
- **AND** 调用前可验证租户授权

### Requirement: [AI-003] AI Gateway 流量治理
AI Gateway SHALL 支持 HTTP、SSE、WebSocket、OpenAI-compatible、路由、限流、重试、熔断、Fallback、安全围栏、脱敏、用量和成本；普通业务流量 SHALL NOT 经过该数据面。

**Traceability:** AI-GW-01, AI-GW-02, AI-GW-03, AI-GW-04, AI-GW-05

#### Scenario: 外部模型超时
- **GIVEN** 租户配置了可用 Fallback
- **WHEN** 模型请求超时
- **THEN** AI Gateway 按策略路由到备用模型
- **AND** 调用审计记录主备 Provider 和成本

### Requirement: [AI-004] Copilot 无执行旁路
Copilot 和 AIOps SHALL 输出证据、时间范围、影响对象、置信度和不确定性；任何写操作 SHALL 转换为结构化计划并经过权限、策略、审批和 Operation。

**Traceability:** AI-OPS-01, AI-OPS-02, AI-OPS-03

#### Scenario: Copilot 提议扩容
- **GIVEN** 诊断建议将副本从 2 扩到 4
- **WHEN** 用户确认建议
- **THEN** 系统生成可审计 Operation
- **AND** Copilot 不直接调用 kubectl

### Requirement: [AI-005] 高风险自动化限制
删除、数据库切换、灾备、网络、存储和大规模扩缩容 SHALL NOT 无确认自动执行；自动修复 SHALL 支持熔断、冷却、回滚和效果验证。

**Traceability:** AI-OPS-05, AI-OPS-06, AI-OPS-07

#### Scenario: 自动修复无改善
- **GIVEN** AIOps 已执行一次低风险修复
- **WHEN** 验证指标未改善
- **THEN** 系统停止连续动作并升级人工处理
- **AND** 保留失败证据
````

### 3.17 `openspec/specs/edge-pack/spec.md`

````markdown
# edge-pack

## Purpose
定义 Edge Pack 作为 T3 运行扩展的云边协同、设备、OTA 和离线能力。

## Requirements

### Requirement: [EDGE-001] Edge Pack 零侵入
未安装 Edge Pack 时 SHALL 不增加 HNB Core 启动依赖、资源占用、菜单和故障面；启用后仍由运行治理平面统一管理。

**Traceability:** EDGE-01

#### Scenario: 标准环境不安装 Edge Pack
- **GIVEN** 执行 T0/T1 全量验收
- **WHEN** 平台完成验收
- **THEN** 所有核心验收通过
- **AND** 无 Edge 相关错误和告警

### Requirement: [EDGE-002] 云边统一执行链
边缘应用 SHALL 来源于市场 Release/CompositionRelease，并经 ExecutionPlan、Operation、权限、策略、审批和审计下发到 NodeGroup。

**Traceability:** EDGE-03, EDGE-05

#### Scenario: 灰度发布边缘应用
- **GIVEN** Release 已声明 edge 兼容性
- **WHEN** 用户选择节点组和批次
- **THEN** 平台创建统一 Operation
- **AND** 批次状态进入 Read Model

### Requirement: [EDGE-003] 断连自治与重连对账
边缘节点 SHALL 按最后已知期望状态在断连期间继续运行和自重启；重连后 SHALL 对账、补传、执行排队 Operation 和撤销处置。

**Traceability:** EDGE-04, EDGE-18

#### Scenario: 节点断网 24 小时
- **GIVEN** 边缘负载已正常运行
- **WHEN** 网络中断后恢复
- **THEN** 负载持续运行且状态最终收敛
- **AND** 收敛时间可观测

### Requirement: [EDGE-004] 批量部署与 OTA
EdgeApplication 和 EdgeCore 升级 SHALL 支持节点组、灰度批次、预检、健康门禁、暂停、失败容忍、回滚和维护窗口。

**Traceability:** EDGE-05, EDGE-08, EDGE-09

#### Scenario: OTA 中某批次失败
- **GIVEN** 100 个节点按 5%/25%/70% 升级
- **WHEN** 首批健康门禁失败
- **THEN** 后续批次暂停且失败节点回滚
- **AND** 已有业务负载保持可用

### Requirement: [EDGE-005] 设备接入治理
Device Mapper SHALL 容器化并通过市场门禁；设备读写 SHALL 绑定 DeviceModel、权限、Operation、审计和站点网络策略。

**Traceability:** EDGE-11, EDGE-12

#### Scenario: 向工业设备写入参数
- **GIVEN** 用户拥有只读权限
- **WHEN** 提交设备写请求
- **THEN** 请求被拒绝
- **AND** 越权事件进入审计

### Requirement: [EDGE-006] 入网与离线交付
边缘入网 SHALL 检查 NTP/PTP、架构、磁盘、运行时、证书和网络；纯离线站点 SHALL 支持签名 Bundle 导入本地 Registry。

**Traceability:** EDGE-17, EDGE-20

#### Scenario: 离线站点首次交付
- **GIVEN** 站点无互联网连接
- **WHEN** 现场导入签名 Bundle
- **THEN** 制品、撤销列表和计划通过验证后可部署
- **AND** 全过程保留导入审计
````

### 3.18 `openspec/specs/deployment-governance/spec.md`

````markdown
# deployment-governance

## Purpose
定义部署档位、能力分级、BOM、阶段门槛、变更治理和验收追踪。

## Requirements

### Requirement: [GOV-001] 能力分级声明
每个 CapabilityPack 和 OpenSpec change SHALL 声明 T0/T1/T2/T3、影响平面、依赖、资源预算、默认开关和卸载影响；缺失 SHALL 视为不完整。

**Traceability:** METH-01

#### Scenario: 发布未分级能力包
- **GIVEN** 一个 CapabilityPack 未声明 tier
- **WHEN** 申请进入 stable
- **THEN** 门禁拒绝发布
- **AND** 提示补充分级和依赖

### Requirement: [GOV-002] 部署档位约束
平台 SHALL 定义 Minimal、Lite HA、Standard HA、Enterprise 四档 BOM；轻量档位 SHALL NOT 强制依赖重型 T2/T3 组件。

**Traceability:** ART-STO-05, GOV-05

#### Scenario: 安装 Minimal 档位
- **GIVEN** 环境只有单节点资源
- **WHEN** 执行安装
- **THEN** 不安装独立搜索集群、Kafka、Ceph、AI Runtime 或 Edge Pack
- **AND** 仍可完成 T0/T1 最小闭环

### Requirement: [GOV-003] 版本化 BOM 与兼容锁
每个交付版本 SHALL 生成 Core BOM、Infrastructure BOM、Provider BOM 和 Optional Pack BOM，记录镜像 digest、Chart digest、Schema 版本和兼容矩阵。

**Traceability:** CTN-07, ART-09

#### Scenario: 离线重建同一版本
- **GIVEN** 运维持有 Release Bundle
- **WHEN** 在隔离环境安装
- **THEN** 安装得到相同组件摘要和 Schema 版本
- **AND** 差异被安装器报告

### Requirement: [GOV-004] 阶段进入与退出判据
阶段 0、MVP、V1、V1.5、V2 SHALL 具有可量化进入门槛和退出判据；未通过当前阶段强制验收 SHALL NOT 进入下一阶段。

**Traceability:** METH-02

#### Scenario: MVP 请求退出评审
- **GIVEN** e2e 第一交付闭环尚未通过回滚测试
- **WHEN** 提交退出评审
- **THEN** 评审拒绝进入 V1
- **AND** 缺口转化为明确 change

### Requirement: [GOV-005] 需求与实现双向追踪
每个 Requirement SHALL 具有稳定 ID；proposal、design、tasks、测试和验收报告 SHALL 引用 Requirement ID，且可从 Requirement 反查实现提交和测试证据。

**Traceability:** METH-04

#### Scenario: 检查一个已完成 change
- **GIVEN** 任务全部勾选
- **WHEN** 执行 verify
- **THEN** 每个受影响 Requirement 至少有一个自动或演练证据
- **AND** 无孤立任务或无实现需求

### Requirement: [GOV-006] 变更 Definition of Done
change 归档前 SHALL 完成规格同步、代码、迁移、自动测试、Conformance、文档、升级/回滚、遥测、安全评审和已知限制；高风险 change SHALL 完成故障演练。

**Traceability:** GOV-02, GOV-05

#### Scenario: 归档 Provider change
- **GIVEN** 实现代码已合并但回滚未验证
- **WHEN** 执行 archive 前 verify
- **THEN** 归档被阻止或明确警告
- **AND** 补齐证据后方可归档
````

---

## 4. Change 提案统一模板

每个 `proposal.md` 至少包含：

```markdown
# Change: <kebab-case-change-id>

## Metadata
- Tier: T0 | T1 | T2 | T3
- Planes: market | artifact | runtime-governance | ai-extension
- Affected Specs:
  - <domain>/<Requirement-ID>
- Depends On:
  - <change-id>
- Target Milestone: Stage-0 | MVP | V1 | V1.5 | V2
- Risk: low | medium | high

## Why
说明当前问题、用户价值、证据和不实施的后果。

## What Changes
- 新增行为
- 修改行为
- 删除或弃用行为

## Non-Goals
明确不在本次 change 处理的范围。

## Compatibility and Migration
- API/事件/Schema 兼容
- 数据迁移
- 老版本与回滚
- Provider/RuntimeTarget 兼容矩阵

## Security and Isolation
- 租户隔离
- Secret 与权限
- 供应链与审计
- 数据边界

## Reliability and Operations
- 失败模式
- 幂等/重试/补偿
- 备份恢复
- 指标、日志、链路和告警
- 容量与性能预算

## Rollout and Rollback
说明灰度、门禁、回滚条件和不可逆步骤。

## Exit Criteria
使用可测量的 GIVEN/WHEN/THEN 或指标描述完成条件。
```

### 4.1 Delta spec 规则

```markdown
## ADDED Requirements

### Requirement: [ID] 名称
系统 SHALL ...

#### Scenario: ...
- **GIVEN** ...
- **WHEN** ...
- **THEN** ...

## MODIFIED Requirements
> 必须粘贴修改后的完整 Requirement 和全部 Scenario，不写局部补丁。

## REMOVED Requirements
### Requirement: [ID] 名称
**Reason:** ...
**Migration:** ...
```

---

## 5. 首批 change backlog 与依赖

### 5.1 阶段 0：先冻结契约，不先堆功能

| 顺序 | Change ID | Tier | 主要产物 | 前置依赖 | 退出判据 |
|---:|---|---:|---|---|---|
| 1 | `bootstrap-openspec-governance` | T0 | config、规则、Requirement ID、CI validate/verify | 无 | OpenSpec validate 通过，PR 能检查无 Scenario 的 Requirement |
| 2 | `bootstrap-contracts-events` | T0 | OpenAPI/事件基础、幂等、Correlation、Outbox | 1 | API 与事件 Schema 可生成 SDK；重复事件测试通过 |
| 3 | `bootstrap-identity-tenancy` | T0 | Tenant/Project/Environment、RBAC、审计上下文 | 1,2 | 跨租户访问测试全部拒绝 |
| 4 | `bootstrap-provider-contract` | T0 | Provider Manifest、生命周期、健康、Conformance Harness | 1,2 | 示例 Provider 通过契约测试 |
| 5 | `bootstrap-operation-engine` | T0 | ExecutionPlan、Operation 状态机、DAG、幂等恢复 | 2,3,4 | Worker 重启恢复、重复命令和补偿测试通过 |
| 6 | `bootstrap-readmodel-resourcegraph` | T0 | Read Model、投影器、Resource Graph | 2,3,5 | 100+ 模拟目标查询不实时遍历目标 |
| 7 | `bootstrap-platform-audit` | T0 | 审计、证据链、敏感字段处理 | 2,3,5 | 从请求可追踪到 Operation 和 Provider 步骤 |

### 5.2 MVP：形成单一可交付闭环

| 顺序 | Change ID | Tier | 主要产物 | 前置依赖 | 退出判据 |
|---:|---|---:|---|---|---|
| 8 | `bootstrap-app-market-catalog` | T1 | Product/Release/Channel/Entitlement | 2,3,7 | 不可变发布、授权与市场故障隔离通过 |
| 9 | `bootstrap-artifact-storage-minimal` | T1 | OCI Endpoint、Local/S3 Profile、直传直取 | 4,5,8 | 镜像/Chart/SBOM 同端点访问；大文件不经 API |
| 10 | `bootstrap-kubernetes-runtime` | T1 | KubernetesTarget、cluster-agent、OCI/Helm Driver | 4,5,6,9 | 应用按 digest 部署、观察、升级、删除 |
| 11 | `bootstrap-gateway-pack` | T1 | Gateway API Bundle、默认 Gateway Provider、HTTPRoute | 4,5,10 | HTTP/TLS 暴露、Route 状态与租户隔离通过 |
| 12 | `bootstrap-config-secret` | T1 | Config digest、SecretReference、默认 Secret Provider | 3,4,5 | 计划/日志无明文 Secret，配置可回滚 |
| 13 | `bootstrap-postgresql-blueprint` | T1 | PostgreSQL ServiceBlueprint/Provider | 4,5,9,10,12 | 创建、绑定、备份、恢复、升级、删除闭环 |
| 14 | `bootstrap-valkey-blueprint` | T1 | Valkey ServiceBlueprint/Provider | 4,5,9,10,12 | 创建、绑定、扩缩容、升级、删除闭环 |
| 15 | `bootstrap-observability-baseline` | T1 | OTel 上下文、Operation SLO、基础告警 | 5,6,7,10,11 | 单次交付可从 Portal 关联到日志、链路和指标 |
| 16 | `bootstrap-supply-chain-gates` | T1 | Cosign/SBOM/扫描/部署准入 | 8,9,10 | 未签名制品在市场和运行侧均被拒绝 |
| 17 | `bootstrap-portal-guided-flow` | T1 | 简单模式、动态表单、应用/数据库/入口向导 | 8-16 | 新用户无需编写 CRD 完成标准交付 |
| 18 | `e2e-first-delivery-loop` | T1 | 发布→部署→暴露→监控→备份→升级→回滚→卸载→审计 | 8-17 | 完整闭环和故障注入验收全部通过 |

### 5.3 V1 及以后

| Change ID | Tier | 目标 |
|---|---:|---|
| `harden-lite-ha-profile` | T1 | 平台/市场/Registry/PG 的 Lite HA 与故障演练 |
| `add-standard-ha-artifact-storage` | T2 | 企业 S3、HA PG、PITR、容量和恢复驾驶舱 |
| `add-container-engine-runtime` | T2 | Podman/containerd 单机 RuntimeTarget |
| `add-ingress-migration` | T2 | Ingress 评估、转换、灰度和回退 |
| `certify-second-gateway-provider` | T2 | 第二个可替换 Gateway Provider 的 Conformance |
| `add-ai-access-pack` | T2 | 外部模型 Connector、AI Gateway、只读 Copilot |
| `add-isv-sandbox` | T2 | 发布者扫描与 Conformance 自助环境 |
| `edge-pack-stage0-poc` | T3 | KubeEdge CloudCore/Edge Provider/断连自治 POC |
| `edge-pack-production` | T3 | NodeGroup、OTA、设备 Mapper、离线 Bundle |
| `add-multi-cluster-governance` | T3 | Karmada、全局流量、DNS/GSLB、多地域治理 |
| `enable-aiops-controlled-remediation` | T3 | 经验证的低风险自动修复与闭环验证 |

---

## 6. Definition of Done 与归档门禁

一个 change 只有满足以下条件才可 sync/archive：

- proposal、delta specs、design、tasks 完整；
- 所有受影响 Requirement ID 均有实现和测试证据；
- API、事件、Manifest、数据库迁移已版本化；
- 单元、集成、契约、E2E 和必要的 Conformance 测试通过；
- 升级和回滚路径已验证；
- Secret、权限、租户隔离和供应链评审完成；
- 指标、日志、链路、告警和 Runbook 已补齐；
- 性能预算有测试条件和结果；
- 高风险 change 完成故障注入或恢复演练；
- T2/T3 能力关闭后，既有 T0/T1 验收仍通过；
- `openspec verify` 无阻断项，delta specs 已 sync。

---

## 7. 审批后的实施约束

1. **先建契约再建控制台**：T0 的 API、事件、Operation、Provider 契约未冻结前，不启动大规模 Portal 页面开发。
2. **先单 Provider 闭环再做多实现**：MVP 默认实现只选一个，第二实现通过相同 Conformance Harness 认证。
3. **先恢复再承诺 HA**：未完成故障演练和恢复测试，不得在产品中标记 HA 或承诺 RPO/RTO。
4. **先 T1 再 T2/T3**：AI Runtime、AIOps 自动执行、KubeEdge 全量、多集群不得挤占 MVP 关键路径。
5. **基线 spec 写行为，design 写选型**：例如“支持可替换 Gateway Provider”属于 spec；选择 NGINX Gateway Fabric 或 Envoy Gateway 属于 design/BOM。
6. **禁止共享数据库和执行旁路**：任何 change 触犯四平面数据边界或 Operation 唯一入口，直接退回重构。
7. **所有规模指标以实测为准**：性能、节点数、吞吐、RPO/RTO 绑定版本、环境和测试报告。

---

## 8. 从 V3.8.5 迁移到 V3.8.6

1. 备份原 `openspec/`。
2. 运行或升级 OpenSpec，确认启用 `spec-driven` schema。
3. 将原 `project.md` 中有效上下文迁入本文件提供的 `config.yaml`。
4. 将原 11 个 spec 与本版 18 个 spec 做 Requirement ID 对照；不要直接覆盖已实现行为。
5. 对已完成能力创建“baseline-sync” change，把现状同步到主 specs。
6. 对未实现内容按第 5 章 backlog 创建独立 change。
7. 在 CI 中加入 `openspec validate`；expanded workflow 可加入 verify。
8. 归档前同步 delta specs，确保主 specs 代表当前已部署事实。

---

## 9. 最终审批结论

完成 V3.8.6 基线落库并通过 OpenSpec 格式校验后，HNB Cloud 可以进入：

- 阶段 0：T0 契约与微内核实现；
- MVP：T1 首个交付闭环；
- V1：高可用、恢复和第二 Provider 认证。

AI Access Pack 与 Edge Pack 保持 T2/T3，不作为 MVP 阻断项。

*本文件是单文件交付包。实际仓库中应将第 2 章保存为 `openspec/config.yaml`，将第 3 章各代码块拆分为独立 `openspec/specs/<domain>/spec.md`。*
