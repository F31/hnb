# HNB Cloud OpenSpec 基线规格包（源自落地方案 V3.8.5）

> 本文件是把《HNB Cloud 开放可组装云原生平台项目落地方案 V3.8.5》转换为 **OpenSpec 规范格式**的基线产物，目的是让 AI coding 助手（Claude Code / Cursor / Codex 等）在实施本项目时，先对齐"系统现在应当如何运行"，再动手写代码。
>
> **这是一个"打包成单文件"的版本**，实际落地时请按下面的目录映射拆分到真实仓库中；不要直接把整份文件当作 `openspec init` 后的唯一 spec 文件使用。

---

## 0. 使用说明：如何拆分到真实 OpenSpec 目录

在项目仓库执行 `openspec init` 后，会得到：

```
openspec/
├── project.md          ← 对应本文件"第1章 项目上下文"
├── specs/               ← 对应本文件"第2章"每个二级标题下的一个 <domain>/spec.md
│   ├── platform-kernel/spec.md
│   ├── app-market/spec.md
│   ├── artifact-storage/spec.md
│   ├── runtime-target/spec.md
│   ├── gateway/spec.md
│   ├── capability-model/spec.md
│   ├── ai-extension/spec.md
│   ├── edge-pack/spec.md
│   ├── security-supply-chain/spec.md
│   ├── observability-ops/spec.md
│   └── governance-tiering/spec.md
└── changes/             ← 对应本文件"第3章"给出的首批 change 提案骨架
```

拆分建议：

1. 把"第1章"整段复制为 `openspec/project.md`；
2. 把"第2章"里每个 `### <domain>` 小节，去掉外层编号后另存为对应目录下的 `spec.md`（文件内保留 `## Purpose` / `## Requirements` 结构）；
3. 把"第3章"每个变更提案骨架，各自建一个 `openspec/changes/<change-id>/proposal.md`，再用 `/opsx:continue` 或 `/opsx:ff` 让 AI 补全 `specs/`（delta）、`design.md`、`tasks.md`；
4. 后续任何架构决策变化，都通过新增 `openspec/changes/<change-id>/specs/<domain>/spec.md` 的 **delta（ADDED/MODIFIED/REMOVED Requirements）** 提出，不要直接手改 `openspec/specs/` 下的基线文件——归档（archive）时才合并。

---

## 1. 项目上下文（→ openspec/project.md）

### 1.1 项目是什么

HNB Cloud 是面向企业多租户场景的开放可组装容器化运行与运营平台，统一管理 Kubernetes 集群、轻量容器运行环境、应用、数据库、中间件、网络、存储、GPU/NPU、安全、可观测、灾备和 AI 服务，并可选扩展到边缘计算场景。完整背景见《HNB Cloud 开放可组装云原生平台项目落地方案 V3.8.5》。

### 1.2 架构基线（不可在任何 change 中违反）

- **微内核架构**：内核只提供身份/租户、Operation Engine、Read Model、Provider 注册中心、事件总线；不得内置任何具体 CNI/CSI/数据库/中间件/网关实现。
- **四个逻辑平面解耦**：应用市场目录与发布平面、统一制品存储与分发平面、HNB Cloud 运行治理平面、HNB AI Extension Plane；Edge Pack 不是第五个平面，受运行治理平面统一管理。
- **统一执行链**：CompositionRelease → ExecutionPlan → Operation 是唯一的部署/升级/回滚/删除执行路径，任何组件（含 AI Copilot、Gateway Controller）不得绕过 Operation Engine。
- **发布不可变**：Release、Chart、镜像、模型、Prompt、Guardrail 和配置均以摘要（digest）锁定，生产部署固定 digest。
- **数据面自治**：市场、AI 平面和中心控制面故障不影响已运行服务；边缘节点断连期间按最后已知期望状态自治运行。
- **Gateway API 是规范而非产品**：GatewayClass/Gateway/Route 等资源由 Gateway Provider 可替换接入，CRD 由集群级统一治理。
- **云为权威、边可自治**：KubeEdge 边缘节点通过 CloudHub–EdgeHub 隧道接入，不重复部署 HNB Agent；Karmada 管集群级分发、KubeEdge 管节点级纳管，同一节点不得被两套体系重复接管。

### 1.3 能力分级（T0–T3）—— 所有 change 必须声明归属分级

| 分级 | 含义 | 默认状态 |
|---|---|---|
| T0 | 内核必装，不可卸载 | 始终安装 |
| T1 | 首个可交付组合的默认必选能力 | 默认安装 |
| T2 | 标准可选，按租户/场景选装 | 默认关闭 |
| T3 | 扩展可选，需完成 POC 与 Conformance 认证后才允许安装 | 默认关闭，需前置验证 |

> 任何 `openspec/changes/<change-id>/proposal.md` 必须在开头声明本次变更属于哪个分级、影响哪个/哪些逻辑平面，未声明视为不完整提案。

### 1.4 八大落地原则（评审与验收的统一标尺）

微内核架构、按需扩展、轻量敏捷、灵活部署、安全可靠、简单易用、极致性能、运维高效。每条原则在方案 2.4.1 节均有对应的量化判据；`design.md` 中的架构决策应说明服务于哪几条原则。

### 1.5 技术基线（写 design.md 时必须核对，不得凭记忆假设版本）

- Gateway API 当前认证基线：v1.5.1 Standard Channel；
- KubeEdge 参考基线：v1.22–v1.23；
- 制品格式：OCI 优先、内容寻址；签名使用 Cosign，SBOM 与 SLSA 对齐；
- 以上版本均可能随 Provider Conformance 结果调整，实施前以官方兼容矩阵为准，不写死在代码里。

### 1.6 质量与工作方式约定

- 涉及跨平面、Provider 契约、RuntimeTarget 或 Operation 状态机的改动，一律先走 OpenSpec change 流程（proposal → specs delta → design → tasks），不允许直接改代码；
- 涉及具体某个 Provider/CapabilityPack 内部实现细节（不改变对外契约）的小改动，可以直接进入 `tasks.md` 级别的小 change，无需完整 design；
- 任何 change 的 `tasks.md` 完成后，必须能回答："关闭本次新增的能力包，是否仍能通过既有 T0/T1 验收项？"——回答"否"则说明违反了按需扩展原则，需重新设计。

---

## 2. 领域基线规格（→ openspec/specs/&lt;domain&gt;/spec.md）

### platform-kernel（微内核）

#### Purpose
定义 HNB Cloud 内核（T0）必须具备、且不得因任何能力包增减而变化的最小职责集合。

#### Requirements

##### Requirement: 内核独立性
内核 SHALL 不硬依赖任何具体 CNI、CSI、数据库、消息中间件、Gateway 实现或 AI Runtime；所有具体能力必须通过 Provider 或能力包接入。

###### Scenario: 卸载全部可选能力包后内核仍可启动
- GIVEN 一个已安装 HNB Cloud 的环境
- WHEN 卸载全部 T2/T3 能力包
- THEN 内核进程正常启动
- AND 内核健康检查（identity、tenant、Operation Engine、Read Model、Provider 注册中心、事件总线）全部通过

##### Requirement: Operation 唯一执行入口
所有部署、升级、回滚、备份和删除操作 SHALL 进入持久化 Operation 状态机；任何组件不得绕过 Operation Engine 直接操作 RuntimeTarget。

###### Scenario: Copilot 生成的修复建议必须经过 Operation
- GIVEN Platform Copilot 生成一个重试部署的建议
- WHEN 用户确认执行该建议
- THEN 系统生成对应的 Operation 记录
- AND Operation 记录可在审计日志中追溯到该建议的来源

##### Requirement: 多租户隔离
Tenant ID SHALL 贯穿 API、数据库、缓存、事件、审计和插件调用的全链路。

###### Scenario: 跨租户数据访问被拒绝
- GIVEN 租户 A 和租户 B 的独立环境
- WHEN 租户 A 的凭据尝试访问租户 B 的资源、日志或制品
- THEN 请求被拒绝
- AND 审计记录中包含该次越权尝试

##### Requirement: Read Model 与实时查询分离
列表和查询类接口 SHALL 使用 Read Model，不得实时遍历所有集群。

###### Scenario: 大规模集群下的列表查询性能
- GIVEN 平台管理 100+ 集群/运行目标
- WHEN 用户请求应用列表
- THEN 响应来自 Read Model 缓存
- AND 响应延迟不随集群数量线性增长

---

### app-market（应用市场）

#### Purpose
定义市场作为独立部署系统的边界：只管"有什么、是什么版本、谁能用"，不越权触碰运行环境。

#### Requirements

##### Requirement: 市场与平台物理隔离
市场 SHALL 可独立部署、独立升级、独立备份恢复，其数据库与平台数据库完全隔离。

###### Scenario: 市场中断不影响已部署应用
- GIVEN 一个正在运行的租户应用
- WHEN 应用市场服务整体不可用
- THEN 该应用继续正常提供服务
- AND 已有的监控、日志、告警不受影响

##### Requirement: 市场不持有运行凭据
市场 SHALL NOT 保存 kubeconfig 或租户运行 Secret，也不得直接调用集群凭据部署应用。

###### Scenario: 市场发起部署请求
- GIVEN 市场生成了一个新的 Release
- WHEN 市场需要触发部署
- THEN 市场只能通过公开 API 向平台提交 ReleaseManifest/CompositionRelease
- AND 平台独立完成凭据解析、预检和 ExecutionPlan 生成

##### Requirement: 发布不可变与摘要锁定
市场发布的制品 SHALL 使用 digest 锁定且不可覆盖。

###### Scenario: 尝试覆盖已发布版本
- GIVEN 一个已进入 stable 渠道的 Release
- WHEN 有人尝试用相同版本号推送新内容
- THEN 系统拒绝该操作
- AND 要求发布方使用新的版本号或摘要

##### Requirement: 分类与标签检索
市场 SHALL 支持应用、数据库、中间件的标准分类和标签体系，支持按分类/标签检索。

###### Scenario: 用户按分类查找数据库产品
- GIVEN 市场中已发布多个数据库类产品
- WHEN 用户在"数据库"分类下检索
- THEN 返回结果只包含该分类下的产品
- AND 每个结果展示标准标签（如高可用、多副本）

---

### artifact-storage（统一制品存储）

#### Purpose
定义 HNB Artifact Storage 作为单一逻辑入口、OCI 优先、可替换后端的制品存储与分发系统的行为契约。

#### Requirements

##### Requirement: 单一逻辑入口与 OCI 优先
所有包类型（镜像、Helm、JAR/WAR、配置、SBOM、模型制品）SHALL 通过统一 OCI Endpoint 访问。

###### Scenario: 统一入口访问不同制品类型
- GIVEN 一个已发布的 Java Web 应用 Release
- WHEN 客户端分别拉取其镜像、Helm Chart 和 SBOM
- THEN 三者均可通过同一 Registry Endpoint 完成访问
- AND 使用统一的 ArtifactDescriptor 描述

##### Requirement: 直传直取
大文件传输 SHALL 由客户端/Agent 直接与 Registry 交互，Market/Platform API 不得转发大文件。

###### Scenario: 拉取大型镜像
- GIVEN 一个体积较大的容器镜像
- WHEN Agent 需要拉取该镜像
- THEN 传输直接发生在 Agent 与 Registry 之间
- AND 平台控制面 API 不参与该次数据传输

##### Requirement: 存储后端可替换
ArtifactStorageProfile SHALL 支持本地、PVC、S3 等后端切换，且上层 ReleaseManifest 不因后端切换而改变。

###### Scenario: 从本地存储切换到 S3
- GIVEN 一个使用本地后端的 Minimal 部署
- WHEN 运维切换 ArtifactStorageProfile 为 S3
- THEN 已发布的 ReleaseManifest 无需重新生成
- AND 制品拉取行为对上层应用透明

##### Requirement: 高可用与可恢复性
Registry SHALL 支持无状态多副本部署；Lite HA 模式下单个 Pod 或节点故障不得导致制品不可读取。

###### Scenario: 单副本故障不影响读取
- GIVEN Lite HA 档位的 Registry 集群
- WHEN 其中一个 Registry 副本故障
- THEN 制品读取请求由其余副本继续处理
- AND 无用户可感知的服务中断

##### Requirement: 安全垃圾回收前的引用识别
删除制品前 SHALL 能识别正在运行的实例、回滚点、组合依赖和灾备引用。

###### Scenario: 尝试删除仍被引用的镜像
- GIVEN 一个镜像正被某个运行中实例引用
- WHEN 运维发起该镜像的垃圾回收
- THEN 系统阻止删除并列出引用来源
- AND 提供预览而非直接执行

---

### runtime-target（容器运行目标）

#### Purpose
定义 RuntimeTarget 统一模型，确保同一制品可在不同运行环境间以声明的兼容性范围部署。

#### Requirements

##### Requirement: RuntimeTarget 分类
系统 SHALL 将运行目标分为 KubernetesTarget、ContainerEngineTarget、EdgeRuntimeTarget 三类主路径；ExternalServiceConnector 是外部服务绑定对象，不是容器执行目标。

###### Scenario: 同一 Release 部署到两类 RuntimeTarget
- GIVEN 一个声明了多种 targetTypes 的 Release
- WHEN 分别部署到 KubernetesTarget 和 ContainerEngineTarget
- THEN 两次部署均成功
- AND 差异仅限于已声明的兼容性范围（如 GPU 支持与否）

##### Requirement: 发布兼容性声明
ReleaseManifest SHALL 声明 targetTypes、资源下限、多架构支持等兼容性信息；不满足声明的目标环境应在预检阶段被拒绝。

###### Scenario: 目标环境不满足资源下限
- GIVEN 一个声明了边缘资源下限的 Release
- WHEN 尝试部署到资源不足的边缘节点
- THEN 预检阶段拒绝该部署
- AND 返回明确的资源不满足原因

##### Requirement: Provider 化的运行时驱动
每类 RuntimeTarget 的具体执行 SHALL 通过 Runtime Driver / Provider 实现，内核不硬编码任何具体运行时逻辑。

###### Scenario: 替换默认容器运行时 Provider
- GIVEN 平台默认使用某个 ContainerEngineTarget Provider
- WHEN 运维安装并切换到另一个认证过的 Provider
- THEN 已有 Release 的部署行为保持一致
- AND 无需修改内核代码

---

### gateway（Gateway API 与服务网络）

#### Purpose
定义 Gateway API 作为 KubernetesTarget 首选南北向流量入口规范的行为边界，以及与 API Management、Service Mesh、AI Gateway 的职责分层。

#### Requirements

##### Requirement: Gateway API 优先、Ingress 仅兼容迁移
新建 Kubernetes 服务入口 SHALL 默认使用 Gateway API；Ingress 只作为存量兼容迁移路径。

###### Scenario: 新建服务入口
- GIVEN 一个新的 KubernetesTarget 应用需要暴露服务
- WHEN 用户通过标准向导创建服务入口
- THEN 系统生成 Gateway/HTTPRoute 而非 Ingress
- AND 仅在明确选择"兼容模式"时才生成 Ingress

##### Requirement: CRD 集中治理
Gateway API CRD SHALL 由 HNB Cluster Provider 或集群管理员统一安装和升级；业务租户只能管理被授权的 Route。

###### Scenario: 租户尝试升级 CRD
- GIVEN 一个业务租户账号
- WHEN 该租户尝试安装或降级 Gateway API CRD Bundle
- THEN 操作被拒绝
- AND 系统提示需要集群级权限

##### Requirement: 数据面 Provider 可替换
相同的 ServiceBlueprint/ExposurePolicy SHALL 能够分别通过至少两个不同的 Gateway Provider（如 NGINX Gateway Fabric、Envoy Gateway）完成部署。

###### Scenario: 切换 Gateway Provider
- GIVEN 一个已使用 NGINX Gateway Fabric 的 ExposurePolicy
- WHEN 运维切换为 Envoy Gateway Provider
- THEN 路由规则和 TLS 配置行为保持一致
- AND 切换过程经过 Operation 并保留审计

##### Requirement: 普通流量与 AI Gateway 隔离
普通业务流量 SHALL NOT 经过 AI Gateway；AI Gateway 与普通 Gateway 的数据面、凭据、路由和观测指标 SHALL 相互隔离。

###### Scenario: 普通应用误配置指向 AI Gateway
- GIVEN 一个非 AI 应用的路由配置
- WHEN 配置尝试将流量指向 AI Gateway 数据面
- THEN 预检阶段拒绝该配置
- AND 提示流量治理分层规则

---

### capability-model（积木式能力模型）

#### Purpose
定义 Capability、CapabilityPack、ServiceBlueprint、CompositionRelease、ExecutionPlan 之间的关系，以及能力分级（T0–T3）的产品化规则。

#### Requirements

##### Requirement: 能力包声明分级与依赖
每个 CapabilityPack SHALL 在市场发布时声明其能力分级（T0/T1/T2/T3）、依赖关系、资源占用和默认开关状态。

###### Scenario: 未声明分级的能力包发布
- GIVEN 一个新的 CapabilityPack 提交发布
- WHEN 该提交未声明能力分级
- THEN 发布门禁拒绝进入 stable 渠道
- AND 返回缺失字段提示

##### Requirement: 组合编排的事务边界
CompositionRelease SHALL 支持至少三个不同包组成一次组合部署，平台需执行依赖 DAG、输出绑定和失败补偿。

###### Scenario: 组合部署中一个子包失败
- GIVEN 一个包含应用、数据库、缓存三个包的 CompositionRelease
- WHEN 数据库子包部署失败
- THEN 系统按依赖 DAG 执行失败补偿
- AND 不会将失败状态之外的子包错误标记为成功

##### Requirement: 按需扩展不影响核心闭环
关闭任意 T2/T3 能力包 SHALL 不影响 T0/T1 能力覆盖的核心闭环（应用交付、备份、升级、回滚）。

###### Scenario: 关闭 AI Extension Plane 后核心闭环验证
- GIVEN 一个已启用 AI Extension Plane 的环境
- WHEN 运维关闭该能力包
- THEN 应用交付、备份、升级、回滚闭环全部验收通过
- AND 无残留的强依赖报错

---

### ai-extension（AI Extension Plane）

#### Purpose
定义 AI 能力作为可选增强而非事实源或执行旁路的边界。

#### Requirements

##### Requirement: AI 平面可独立开关
AI Extension Plane SHALL 可独立安装、升级、停用；停用后平台核心与传统应用不受影响。

###### Scenario: 停用 AI Extension Plane
- GIVEN 一个已部署 AI Extension Plane 的环境
- WHEN 运维停用该平面
- THEN 平台核心 API 和传统应用运行不中断
- AND 已有非 AI Operation 记录不受影响

##### Requirement: Copilot 不设执行旁路
Platform Copilot 的所有写操作 SHALL 转换为结构化计划，并经过权限、策略、审批和 Operation Engine。

###### Scenario: Copilot 建议自动修复
- GIVEN Copilot 检测到一次部署失败并生成修复建议
- WHEN 该建议涉及资源变更
- THEN 系统要求人工确认后才生成 Operation
- AND Copilot 无法直接执行变更

##### Requirement: 多租户 AI 数据隔离
Tenant A SHALL 不能访问 Tenant B 的模型端点、调用记录、知识引用和成本数据。

###### Scenario: 跨租户模型调用记录访问
- GIVEN 租户 A 尝试查询租户 B 的模型调用日志
- WHEN 该请求被提交
- THEN 请求被拒绝
- AND 审计记录该次越权尝试

##### Requirement: 外部模型不可用时的降级
外部模型不可用时 SHALL 按策略 Fallback，且不影响非 AI 平台能力。

###### Scenario: 外部模型 API 超时
- GIVEN AI Access Pack 依赖的外部模型 API 超时
- WHEN 用户发起一次 AI 辅助请求
- THEN 系统按预设策略降级或返回明确失败
- AND 平台其余功能不受影响

---

### edge-pack（边缘计算，T3）

#### Purpose
定义 Edge Pack 作为运行治理平面的运行扩展（而非第五平面）的边界，覆盖断连自治、设备接入和批量 OTA。

#### Requirements

##### Requirement: Edge Pack 未启用时零影响
未启用 Edge Pack 时 SHALL 不影响平台既有安装、运行和升级。

###### Scenario: 未安装 Edge Pack 的标准环境
- GIVEN 一个未安装 Edge Pack 的平台环境
- WHEN 执行既有 T0/T1 验收项
- THEN 全部通过
- AND 无 Edge 相关的错误或警告

##### Requirement: 云边统一发布与执行
边缘应用 SHALL 仍来自市场 Release/CompositionRelease，仍由平台生成 ExecutionPlan，仍经 Operation、权限、策略、审批和审计执行。

###### Scenario: 边缘应用发布
- GIVEN 一个声明 EdgeRuntimeTarget 兼容性的 Release
- WHEN 该 Release 部署到边缘节点组
- THEN 部署流程与云上应用共用同一套 Operation Engine
- AND 生成同样可审计的 ExecutionPlan

##### Requirement: 断连自治与重连对账
边缘节点断连期间 SHALL 按最后已知期望状态继续运行；重连后 SHALL 自动对账、补传并处置撤销列表。

###### Scenario: 边缘节点 24 小时断网
- GIVEN 一个已部署负载的边缘节点
- WHEN 该节点断网 24 小时
- THEN 已部署负载持续运行，容器可自重启
- AND 重连后系统自动对账，收敛时间可观测

##### Requirement: 单一纳管通道
同一边缘节点 SHALL 不被 HNB Agent 与 KubeEdge 同时重复接管。

###### Scenario: 节点纳管冲突检测
- GIVEN 一个已通过 KubeEdge 纳管的边缘节点
- WHEN 有人尝试同时用 HNB Agent 纳管该节点
- THEN 系统拒绝重复纳管
- AND 提示当前节点的纳管通道归属

---

### security-supply-chain（安全与软件供应链）

#### Purpose
定义双重安全门禁（市场供应链 + 平台运行）与发布不可变性的强制要求。

#### Requirements

##### Requirement: 双重安全门禁
系统 SHALL 在发布时（市场侧）和部署时（平台侧）均验证签名、SBOM 和安全策略。

###### Scenario: 未签名制品尝试进入 stable 渠道
- GIVEN 一个未经过 Cosign 签名的制品
- WHEN 提交进入 stable 渠道
- THEN 发布被拒绝
- AND 返回缺失签名的明确原因

##### Requirement: 撤销处置可追溯
撤销版本 SHALL 以签名 OCI Artifact/Bundle 分发，处置策略可配置并留有审计记录。

###### Scenario: 已发布版本被撤销
- GIVEN 一个已在多个环境运行的版本
- WHEN 该版本因安全问题被撤销
- THEN 撤销信息以签名制品分发到所有相关环境
- AND 每个环境的处置结果可审计追溯

---

### observability-ops（可观测、运维与灾备）

#### Purpose
定义运维动作"默认内建 / 一键触发 / 需人工介入"的分类，以及关键路径性能预算的验证方式。

#### Requirements

##### Requirement: Operation 状态可观测
系统 SHALL 明确 Operation 的终态和非终态，并对状态滞留设置 SLO。

###### Scenario: Operation 长时间处于非终态
- GIVEN 一个进入 InProgress 状态的 Operation
- WHEN 该状态持续超过预设 SLO
- THEN 系统触发告警
- AND 状态展示中标注滞留时长

##### Requirement: 关键路径性能预算
内核 API、Read Model 查询、制品直传直取、Gateway 数据面转发、边缘断连收敛 SHALL 有明确性能预算并可通过压测/演练验证。

###### Scenario: 性能预算验证
- GIVEN 一次版本发布前的压测计划
- WHEN 执行既定的关键路径压测
- THEN 各路径 P95 指标与既定预算对比
- AND 结果记入验收报告，不允许仅以"预计"表述

---

### governance-tiering（能力分级与八大原则治理，V3.8.5 新增）

#### Purpose
把落地方案 V3.8.5 新增的"八大原则 × 能力分级"框架，转化为可被 OpenSpec change 流程强制检查的规则。

#### Requirements

##### Requirement: 阶段门槛与退出判据
版本实施路线（阶段 0/MVP/V1/V1.5/V2）SHALL 各自定义进入门槛和退出判据，判据须引用可量化的原则判据，不接受未量化表述。

###### Scenario: 阶段验收使用模糊表述
- GIVEN 某阶段验收报告写"基本符合设计目标"
- WHEN 提交该报告用于阶段退出评审
- THEN 评审要求补充具体量化数据
- AND 阶段不得仅凭该表述进入下一阶段

##### Requirement: 原则符合度矩阵
每次大版本验收 SHALL 包含八大原则符合度矩阵的实测结果。

###### Scenario: 版本验收报告审查
- GIVEN 一份 V1 版本验收报告
- WHEN 评审人员检查该报告
- THEN 报告包含微内核架构、按需扩展、轻量敏捷、灵活部署、安全可靠、简单易用、极致性能、运维高效八项的实测判据
- AND 缺失任意一项则报告视为不完整

---

## 3. 首批 change 提案骨架（→ openspec/changes/&lt;change-id&gt;/proposal.md）

以下按第 27 章"首个可交付产品组合"（T0+T1）拆出的建议实施顺序，每个都应作为独立 change 提出，AI 据此依次 `/opsx:propose` → `/opsx:continue`/`/opsx:ff` → `/opsx:apply` → `/opsx:archive`：

1. **bootstrap-platform-kernel**（T0）：身份/租户、Operation Engine、Read Model、Provider 注册中心、事件总线的最小可运行内核。
2. **bootstrap-app-market-minimal**（T1）：市场目录、Release/CompositionRelease 基本模型、与内核的公开 API 集成。
3. **bootstrap-artifact-storage-minimal**（T1）：Lightweight OCI Registry Provider + 本地/S3 后端可切换。
4. **bootstrap-kubernetes-target**（T1）：KubernetesTarget + OCI Image Provider + Helm Provider + Java Artifact Runtime Provider。
5. **bootstrap-gateway-pack**（T1）：Gateway API Standard Channel + 默认 Gateway Provider（建议 NGINX Gateway Fabric 或 Envoy Gateway）。
6. **bootstrap-service-blueprint**（T1）：PostgreSQL Service Provider + Valkey/RabbitMQ Service Provider + 基础 Network/Storage/Secret。
7. **bootstrap-observability-baseline**（T1）：基础指标、日志、审计。
8. **e2e-first-delivery-loop**（T1，验收性 change）：串联发布 → 审核 → 部署 → 暴露 → 监控 → 备份 → 升级 → 回滚 → 卸载 → 审计的完整闭环验证，对应第 27 章的验收清单。
9. **optional-ai-access-pack**（T2，MVP 之后按需）：外部模型 Connector + AI Gateway，需先过 governance-tiering 的分级检查。
10. **optional-edge-pack-poc**（T3，需先完成 POC）：KubeEdge CloudCore/Edge Provider 的阶段 0 验证性 change，不进入 MVP 交付范围。

> 每个 change 的 `proposal.md` 开头必须包含：
> - **分级**：T几级
> - **影响平面**：市场 / 制品存储 / 运行治理 / AI 扩展 中的哪些
> - **关联基线 Requirement**：引用第 2 章对应 `domain/Requirement` 名称
> - **退出判据**：对应第 1.6 节的"关闭本能力包是否仍通过既有验收"检查

---

*本文件由《HNB Cloud 开放可组装云原生平台项目落地方案 V3.8.5》整理而来，用于初始化 OpenSpec 基线；后续架构变化请通过 change 流程演进，不要直接改写本文件对应拆分出的 `openspec/specs/` 内容。*
