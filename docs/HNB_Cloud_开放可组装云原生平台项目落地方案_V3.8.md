# HNB Cloud 开放可组装云原生平台项目落地方案

> **方案版本**：V3.8  
> **文档状态**：评审整合版——容器化交付、统一应用市场、生产级制品存储、AI 增强与云边协同  
> **编制日期**：2026-07-18  
> **整合依据**：《HNB 云原生平台需求规格说明书（核心平台版）V3.1》、《HNB Cloud 开放可组装云原生平台项目落地方案 V3.6》及《HNB Cloud V3.6 方案评审报告与 KubeEdge 边缘计算补充方案》  
> **核心定位**：以最小平台内核、独立应用市场、统一 OCI 制品存储、可选 AI Extension Plane、可选 Edge Pack、能力包、服务蓝图、组合编排和多类型 RuntimeTarget，建设面向传统应用、云原生应用、AI 应用与边缘应用的开放、可组装、轻量、敏捷、高可用企业运行与运营平台。

---

## 0. V3.8 修订与整合说明

### 0.1 本版整合目标

V3.8 不是在 V3.6 末尾简单追加一个“边缘计算附录”，而是对两份输入文档进行统一评审、去重和横向贯通，形成一份可以直接用于立项、详细设计、研发拆解与验收的完整方案。

本版保留 V3.6 已验证正确的核心判断：

- 微内核与 Provider 化；
- 应用市场、制品存储、运行治理、AI 扩展四个逻辑平面解耦；
- 平台与所提供服务统一容器化交付；
- OCI 优先、内容寻址、发布不可变、生产部署固定 digest；
- CompositionRelease → ExecutionPlan → Operation 的统一编排执行链；
- AI 可选增强、不是事实源、不设置执行旁路；
- 数据面自治、市场和中心控制面故障不影响已运行服务。

在此基础上，V3.8 重点补齐 V3.6“承诺支持边缘、但缺少体系化边缘架构”的缺口，并同步修复跨章节的不一致、边界模糊和工程约束不足。

### 0.2 V3.8 关键修订

| 主题 | V3.8 调整 |
|---|---|
| 总体架构 | 明确为“**四个逻辑平面 + 多类型运行目标 + 可选能力包**”；Edge Pack 是运行扩展，不新增第五套权威控制面 |
| 边缘定位 | 新增可选 Edge Pack，以 KubeEdge 为弱网、断连自治、设备接入和批量 OTA 场景的一等参考实现 |
| 运行目标 | 重构 RuntimeTarget，增加 `kubeedge-edge`、`cloudedge-tunnel`、`offline-bundle`、自治策略、带宽策略和离线撤销策略 |
| 云边通道 | 中心 Kubernetes 继续使用 HNB cluster-agent；KubeEdge 边缘节点只使用 CloudHub–EdgeHub 隧道，不重复部署 HNB Agent |
| 发布兼容性 | ReleaseManifest 增加 `targetTypes`、边缘资源下限、WAN 依赖、离线自治和多架构声明 |
| 边缘资源模型 | 新增 EdgeNode、NodeGroup、EdgeApplication、DeviceModel、Device、EdgeOTAJob 等平台投影对象 |
| 制品分发 | 将中心 Registry、区域 Mirror、站点 Mirror、节点缓存与 ImagePrePullJob 串成完整边缘分发链；撤销列表按签名 OCI Artifact/Bundle 分发 |
| Operation | 明确终态和非终态，增加 `QueuedOffline`、状态滞留 SLO、超时升级与重连顺序投递 |
| 安全供应链 | 明确 Cosign 签名、OCI Referrer、SBOM、构建证明与 SLSA 对齐目标；增加 Secret/KMS 分层和边缘物理不可信假设 |
| AI 与边缘 | 增加边缘模型制品、端侧推理、边云 Fallback、边缘 AI Gateway 轻量实例与边缘推理指标 |
| 实施路线 | MVP 保持收敛，仅预留边缘 Schema；V1.5 交付 Edge Pack；V2 扩展多地域边缘与边云 AI |
| 验收体系 | 新增 EDGE、边缘安全、断连补传、OTA、离线撤销、灰度发布等需求与验收项 |
| 工程治理 | 增加 UsageRecord/MeterEvent 统一计量契约、ISV 沙箱、采纳度量、i18n 与边缘时钟治理 |
| 文档质量 | 增加阅读导航，统一术语，修复最终原则编号回退，删除重复和孤立内容 |

### 0.3 统一架构口径

V3.8 使用以下统一口径，后续章节不得出现冲突表述：

1. **四个逻辑平面**：应用市场目录与发布平面、统一制品存储与分发平面、HNB Cloud 运行治理平面、HNB AI Extension Plane。
2. **Edge Pack 不是第五个平面**：它由 Edge Provider、KubeEdge CloudCore、边缘运维控制器和设备接入能力组成，受 HNB Cloud 运行治理平面统一管理。
3. **运行目标分三类主路径**：KubernetesTarget、ContainerEngineTarget、EdgeRuntimeTarget；ExternalServiceConnector 是外部服务绑定对象，不是容器执行目标。
4. **云边统一发布与执行**：边缘应用仍来自市场 Release/CompositionRelease，仍由平台生成 ExecutionPlan，仍经 Operation、权限、策略、审批和审计执行。
5. **云为权威、边可自治**：云侧保存期望状态和权威管理视图；边缘在断连期间按最后已知期望状态继续运行，重连后对账、补传和执行撤销处置。
6. **Karmada 与 KubeEdge 分工**：Karmada 管集群级多集群分发，KubeEdge 管节点级边缘纳管；同一节点不得同时被两套体系重复接管。
7. **所有指标均为设计目标**：节点规模、RPO、RTO、延迟和吞吐最终以兼容矩阵、压测和故障演练结果为准。

### 0.4 文档阅读导航

| 章节范围 | 主要内容 |
|---|---|
| 1–4 | 执行摘要、关键结论、产品边界、总体逻辑架构 |
| 5–8 | 最小内核、应用市场、职责分工、接口与事件 |
| 9–11 | 软件包模型、组合编排、Capability/CapabilityPack/Blueprint |
| 12 | AI as Workload / Service / Platform 与 AI 治理 |
| 13–15 | RuntimeTarget、EdgeRuntimeTarget、Provider 契约与部署方式 |
| 16–19 | 交付闭环、易用性、安全供应链、可观测运维与灾备 |
| 20–22 | 工程架构、实施路线、团队组织 |
| 23–26 | 测试验收、容量性能、风险、完整需求清单 |
| 27–29 | 首个交付组合、最终原则、参考资料与版本基线 |

## 1. 执行摘要

HNB Cloud 的目标不是建设一个包含所有开源组件的“大控制台”，也不是把平台演变为重型模型训练平台，而是建设一套能够长期演进的企业应用、数据服务与 AI 服务运行运营底座。平台应当做到：

- 核心足够小，非必需能力按需安装；
- 能力像积木一样组合、替换、升级和卸载；
- 平台与服务统一使用容器交付和运行；
- 同一套容器化产品可以部署到物理机、虚拟机、私有云、公有云或边缘环境，但必须由对应 RuntimeTarget 和兼容性声明提供能力支撑；
- 软件制品统一进入应用市场管理，并由 HNB Artifact Storage 提供统一存储、分发和生命周期能力；
- 支持传统应用、云原生应用、数据库、中间件以及 AI 应用和模型服务统一交付；
- AI 能力以可选 Extension Plane 和能力包提供，不成为平台启动、传统应用运行或 Operation 执行的强依赖；
- 轻量环境可以只连接外部模型 API，不要求 GPU、本地大模型、向量数据库或重型 AIOps 后端；
- 用户看到的是应用、数据库、中间件、模型服务和解决方案，而不是大量 Kubernetes 原生对象；
- 已运行的数据面不依赖市场、AI 服务或中心控制面的持续可用；边缘节点断连时按最后已知期望状态自治运行，重连后完成对账、补传和策略处置。

V3.8 建议将产品划分为三个独立部署、松耦合集成的核心系统，并以统一制品存储平面作为共享基础能力：

```text
HNB App Market              HNB Cloud Platform             HNB AI Extension Plane
软件资产与发布中心           资源与运行治理中心              AI 服务与智能运营扩展

- 产品/模型/解决方案          - 租户/项目/环境                - AI Gateway
- 分类/标签/检索              - RuntimeTarget/Cluster        - Model Service Manager
- Release/Channel            - 配额/网络/存储/GPU            - Model/Runtime Provider
- CompositionRelease         - Policy/Approval               - Guardrail/Evaluation
- 签名/SBOM/扫描              - Operation/ExecutionPlan       - AI Observability
- 授权/订阅/可见范围          - Provider/Agent                - Platform Copilot/AIOps
```

推荐协作原则：

> **市场定义“有什么、是什么版本、由哪些包或模型组成、允许谁使用”；平台决定“部署到哪里、能否部署、如何安全运行”；AI Extension Plane 负责“模型如何接入、调用如何治理、AI 如何辅助平台”。**

> **Edge Pack 负责“如何在弱网、断连、设备接入和无人值守场景中执行平台计划”，但不拥有独立的产品、租户、审批、授权或审计权威。**

因此，HNB Cloud 的总体结构不是“五个平台并列”，而是四个逻辑平面共同服务 Kubernetes、Container Engine 与 Edge 三类运行目标。KubeEdge 只作为 EdgeRuntimeTarget 的参考实现和执行通道，平台仍是唯一编排入口。

市场向平台提供不可变 `ReleaseManifest` 或 `CompositionRelease`。平台根据目标环境、GPU/NPU 能力、租户配额、安全策略和运行参数生成不可变 `ExecutionPlan`。AI Gateway 和 Model Runtime Provider 作为执行计划中的可选节点被部署和治理，但不得绕过 Operation Engine。

统一制品存储继续遵循：

> **单一逻辑入口、OCI 优先、内容寻址、数据直传直取、后端可替换、按 SLA 分档。**

模型权重等超大制品可以存入 HNB Artifact Storage，也可以通过 `ModelArtifact.externalRef` 引用受信外部模型仓库；无论采用哪种方式，必须固定版本、摘要、许可证、来源和评测状态。

## 2. 关键设计结论

### 2.1 必须保留的架构基础

| 设计 | 结论 | V3.8 落地要求 |
|---|---|---|
| 微内核 | 保留 | 核心不得硬依赖 CNI、CSI、数据库、中间件和市场实现 |
| Provider 化 | 保留 | Provider 必须声明能力、版本、目标类型和生命周期支持范围 |
| Agent 主动连接 | 保留 | Agent 自身容器化，通过 mTLS 主动连接，不暴露公网管理端口 |
| 控制面/数据面分离 | 保留 | 应用、数据库和消息流量不得经过平台或市场控制面 |
| Read Model | 保留 | 列表和查询不实时遍历所有集群 |
| Operation Engine | 保留 | 所有部署、升级、回滚、备份和删除均进入持久化状态机 |
| 多租户 | 保留 | Tenant ID 贯穿 API、数据库、缓存、事件、审计和插件调用 |
| 可插拔能力包 | 增强 | 增加市场包、组合包、依赖锁和兼容认证 |
| 服务目录 | 重构 | 平台服务目录以市场发布内容为主，平台可保留本地缓存和私有条目 |
| 任意环境部署 | 澄清 | 任意物理机/虚拟机是容器底座，不代表支持裸包直接运行 |
| 统一制品存储 | 增强 | 采用 OCI 优先、统一 Registry 入口和可替换本地/PVC/S3 后端 |
| 存储高可用 | 增强 | Registry 无状态多副本，权威数据由共享对象存储和 HA PostgreSQL 保障 |
| 存储能效 | 增强 | 去重、直传直取、区域缓存、按需扫描、分层保留和自动 GC |
| AI Extension Plane | 新增 | 独立容器化部署、可整体关闭、通过公开 API/Provider 与平台集成 |
| AI 运行与服务 | 新增 | 统一模型制品、模型端点、AI Gateway、GPU/NPU 配额和生命周期 |
| AI 辅助平台 | 保留并增强 | Copilot/AIOps 只生成解释、建议或受控计划，不成为事实源和执行旁路 |
| 边缘计算 | 新增 | 以可选 Edge Pack 和 EdgeRuntimeTarget 提供断连自治、设备管理、弱网分发和批量 OTA，不进入微内核 |

### 2.2 必须纠正的前序设计

前序方案中的以下内容不再成立：

- 不再提供 LinuxHostTarget 直接安装 RPM、DEB、JAR、WAR 或 systemd 服务；
- 不再提供“虚拟机高可用版 + systemd”形式的平台部署；
- 不再将普通主机安装器视为数据库和中间件交付方式；
- JAR/WAR 不作为最终运行单元；
- 应用市场不能直接调用集群凭据或绕过平台执行部署；
- 平台不能把市场制品库简单等同于镜像仓库或 Helm 仓库；
- 多包编排不能只依赖 Helm umbrella chart，应支持平台级组合依赖和跨包输出引用。

### 2.3 V3.8 的八个核心闭环

1. **平台闭环**：租户、项目、环境、运行目标、Operation、审计和策略。
2. **市场闭环**：应用、模型和制品入库，分类、标签、版本、扫描、审核、发布和授权。
3. **应用交付闭环**：选择版本、参数化、预检、审批、执行、验证、升级和回滚。
4. **制品存储闭环**：上传、摘要校验、签名关联、复制、缓存、保留、备份、恢复和安全回收。
5. **服务运营闭环**：应用、数据库和中间件的监控、备份、升级、扩缩容、绑定和删除。
6. **AI 服务闭环**：模型登记、评测、发布、部署、端点暴露、调用治理、可观测、升级和下线。
7. **边缘运行闭环**：节点入网、制品预分发、灰度部署、断连自治、设备接入、OTA、重连对账和撤销处置。
8. **智能运营闭环**：数据采集、异常关联、证据检索、诊断建议、审批确认、Operation 执行和效果验证。

---

## 3. 产品定位与边界

### 3.1 HNB Cloud 平台定位

HNB Cloud 是面向企业多租户场景的开放可组装容器化运行与运营平台，统一管理 Kubernetes 集群、轻量容器运行环境、应用、数据库、中间件、网络、存储、GPU/NPU、安全、可观测、灾备和 AI 服务。

平台面向三类工作负载：

- **传统应用容器化**：JAR/WAR、Web 应用和既有中间件的标准化容器交付；
- **云原生应用与数据服务**：Helm、Operator、微服务、数据库和消息服务；
- **AI 应用与模型服务**：大模型、语音、视觉、Embedding、Rerank、RAG、Agent 和行业 AI 应用。

平台不是：

- 代码仓库或通用 CI/CD 平台；
- 通用制品构建平台；
- 单纯的 Kubernetes Dashboard；
- 把多个开源控制台嵌入一个门户；
- 通用 BPM 或 ITSM；
- 传统软件安装工具；
- 默认内置的重型模型训练、数据标注或大规模分布式训练平台。

模型训练、微调、数据标注和实验追踪可以作为后续市场产品或外部平台集成，但不进入 V3.8 微内核。

### 3.2 HNB App Market 定位

HNB App Market 是独立部署的软件资产、模型资产、产品目录和版本发布中心，面向以下场景：

- 平台官方能力和服务发布；
- 企业内部应用和 AI 应用发布；
- ISV 或合作伙伴应用发布；
- 数据库、中间件、模型、AI 运行时和 AI 安全服务包发布；
- 行业解决方案、多组件套件和离线交付包发布；
- 多个平台实例共享统一软件与模型市场；
- 私有市场、租户市场和公共市场分层运营。

应用市场不是：

- 业务集群部署执行器；
- Kubernetes 管理控制面；
- 模型在线推理代理；
- 应用和模型运行状态的最终数据源；
- 租户密钥托管中心；
- 编译构建流水线；
- 运行时监控和故障处理系统。

### 3.3 HNB AI Extension Plane 定位

HNB AI Extension Plane 是独立容器化部署的可选能力平面，负责模型接入、推理服务、AI Gateway、Prompt 和 Guardrail 治理、评测、AI 可观测、Platform Copilot 与 AIOps。

其边界如下：

- 可以整体不安装，也可以只启用 AI Access、AI Runtime 或 AIOps 中的部分能力；
- 不保存 HNB Core 的权威资源状态；
- 不绕过平台权限、配额、策略、审批、Operation 和审计；
- 不要求业务流量、数据库流量或普通容器流量经过 AI 平面；
- AI 平面故障不得影响传统应用、数据库、中间件和已运行模型端点；
- 支持本地模型、企业内部模型服务和外部云模型服务；
- 通过版本化 OpenAPI、Provider、事件和 Tenant Context 与平台集成。

### 3.4 HNB Edge Pack 定位

HNB Edge Pack 是 HNB Cloud 运行治理平面的可选扩展能力包，面向工厂、园区、门店、变电站、交通设施、车载、机器人和其他弱网/断连环境。

Edge Pack 负责：

- KubeEdge CloudCore 的容器化部署、升级、扩缩容和高可用；
- Edge Provider 对 KubeEdge API/CRD 的适配；
- EdgeNode、NodeGroup、EdgeApplication、Device 和 EdgeOTAJob 的平台投影；
- 云边隧道、断连自治、边缘制品预分发和重连对账；
- 设备 Mapper、DeviceTwin、边缘应用与边缘 AI 的统一交付；
- 节点入网、证书、配置更新、镜像预拉取、EdgeCore OTA 和现场恢复工具。

Edge Pack 不是：

- 第二套应用市场、租户系统、审批系统或审计系统；
- Karmada 的替代品；
- 通用 IoT 数据平台、时序数据湖或工业互联网业务平台；
- 默认内置的边云协同训练平台；
- 所有边缘场景的唯一实现。稳定联网的整站轻量集群可继续使用 k3s/K0s + HNB cluster-agent，单机受限场景可使用 Podman + ContainerEngineTarget。

Edge Pack 默认可不安装；未启用时不得增加 HNB Core 的启动依赖、资源占用和故障面。

### 3.5 容器化硬约束

#### 3.5.1 平台组件

以下组件必须以容器方式交付：

- platform-api；
- platform-controller；
- platform-worker；
- agent-gateway；
- cluster-agent；
- container-agent；
- web-console；
- plugin/provider；
- HNB App Market 各服务；
- HNB AI Extension Plane 各服务；
- AI Gateway、Model Service Manager、AI Observability 和 AIOps Worker；
- 市场索引、扫描、同步和导出 Worker；
- 平台自带数据库、中间件和可观测组件。

#### 3.5.2 平台提供的服务

以下服务只允许使用容器化实现进行交付：

- 应用运行时；
- PostgreSQL、MySQL、Valkey/Redis 等数据库；
- Kafka、RabbitMQ、RocketMQ、MQTT 等中间件；
- API 网关和微服务治理组件；
- 日志、指标、链路和告警组件；
- 镜像扫描、运行时安全和策略组件；
- 备份、恢复和灾备执行组件；
- AI Gateway、模型推理服务、Embedding、Rerank、向量数据库和内容安全服务。

#### 3.5.3 物理机与虚拟机

物理机和虚拟机仅承担以下角色：

- Kubernetes 节点；
- k3s、RKE2、K0s 等轻量 Kubernetes 节点；
- Docker、Podman 或 containerd 容器主机；
- 容器镜像仓库、对象存储、外部数据库或 GPU/NPU 的基础设施承载节点。

平台不得把物理机或虚拟机等同于传统软件直接安装目标。

#### 3.5.4 JAR/WAR 的运行规则

JAR 和 WAR 可以作为软件制品进入市场，但不得由平台以 `java -jar`、Tomcat 目录复制或 systemd 方式直接运行。支持两种容器化交付方式：

1. **不可变镜像方式，生产推荐**  
   发布方在进入生产市场前将 JAR/WAR 构建为 OCI 镜像；市场关联原始包、镜像摘要、Dockerfile/构建证明、SBOM 和签名。平台仅部署 OCI 镜像。

2. **标准运行时镜像承载方式，受控兼容**  
   市场发布 `ArtifactRuntimePackage`，包含 JAR/WAR、运行时镜像摘要、启动参数、挂载规则和健康检查。平台把制品以只读卷、Init Container 或受控下载方式注入标准运行时容器。该方式适合迁移期和内部应用，不建议作为高安全生产环境的长期默认方案。

可选建设独立 `Image Packaging Service`，将 JAR/WAR 转换为 OCI 镜像。该服务是市场的可选外部集成，不纳入 HNB Cloud 核心，也不演变为通用 CI/CD 平台。

#### 3.5.5 边缘节点代理的显式例外

CloudCore、Edge Provider、ControllerManager、TaskManager 和边缘管理服务必须容器化运行。边缘业务负载、设备 Mapper、规则引擎、网关和推理服务也必须容器化。

`EdgeCore` 作为边缘节点上的容器运行底座管理代理，通常以节点级二进制和 systemd 服务运行。V3.8 将其定义为容器化硬约束的**显式、受控例外**，条件如下：

- 仅限 EdgeRuntimeTarget 节点，不得扩展为普通业务软件裸机安装通道；
- 安装、升级、回滚和配置变更必须由 Edge Pack 的节点任务或受控现场工具执行；
- EdgeCore 版本、配置摘要、证书、升级记录和健康状态必须进入平台 Read Model 与审计；
- 业务容器不得借此绕过镜像签名、准入、资源限制和运行时安全策略。

## 4. 总体逻辑架构

V3.8 将体系结构统一表达为：**一个统一体验层、四个逻辑平面、一个容器执行与连接层、三类主要运行目标**。

```text
┌──────────────────────────────────────────────────────────────────────────────────────┐
│                                  统一访问体验                                        │
│ HNB Portal │ Tenant Portal │ AI Workspace │ Edge Console │ CLI │ OpenAPI │ SDK      │
└───────────────────────────────────────┬──────────────────────────────────────────────┘
                                        │
       ┌────────────────────────────────┼────────────────────────────────┐
       │                                │                                │
┌──────▼──────────────────┐  ┌──────────▼──────────────────┐  ┌──────────▼──────────────────┐
│ HNB App Market         │  │ HNB Cloud Platform          │  │ HNB AI Extension Plane     │
│ 目录与发布平面          │  │ 运行治理与权威控制面         │  │ AI 服务与智能运营扩展       │
│ Product/Release        │  │ Tenant/Project/Environment │  │ AI Gateway                 │
│ Composition/Channel    │  │ RuntimeTarget/Capability   │  │ Model Service Manager      │
│ Entitlement/Review     │  │ Policy/Quota/Approval      │  │ Guardrail/Evaluation       │
│ Signature/SBOM/Eval    │  │ ExecutionPlan/Operation    │  │ Copilot/AIOps/AI OTel      │
└──────────┬─────────────┘  └──────────┬───────────────────┘  └──────────┬───────────────────┘
           │                            │                                  │
           └────────────────────────────┼──────────────────────────────────┘
                                        │
┌───────────────────────────────────────▼──────────────────────────────────────────────┐
│                         HNB Artifact Storage 与分发平面                              │
│ OCI Registry │ Referrer │ Model Artifact │ Token │ Replication │ Mirror │ Bundle     │
│ 中心权威数据 │ 区域/站点缓存可重建 │ Local/PVC（非 HA）│ S3/企业对象存储（HA）       │
└───────────────────────────────────────┬──────────────────────────────────────────────┘
                                        │
┌───────────────────────────────────────▼──────────────────────────────────────────────┐
│                           容器执行、连接与 Provider 层                               │
│ cluster-agent │ container-agent │ Edge Provider │ Runtime Driver │ Helm │ Operator  │
│ KubeEdge CloudCore │ CNI/CSI │ GPU/NPU │ Security │ OTel │ DB/MW/AI Provider         │
└───────────────┬────────────────────────┬────────────────────────┬─────────────────────┘
                │                        │                        │
       ┌────────▼─────────┐     ┌────────▼─────────┐     ┌────────▼─────────────────────┐
       │ KubernetesTarget│     │ContainerEngine  │     │ EdgeRuntimeTarget            │
       │ K8s/RKE2/k3s/K0s│     │ Docker/Podman   │     │ KubeEdge EdgeCore            │
       │ 集群级运行目标    │     │ 单机/受限目标    │     │ 弱网、自治、设备、OTA         │
       └────────┬─────────┘     └────────┬─────────┘     └────────┬─────────────────────┘
                └────────────────────────┼────────────────────────┘
                                         ▼
                物理机 │ 虚拟机 │ 私有云 │ 公有云 │ 边缘站点 │ GPU/NPU │ 工业设备
```

架构中的权威关系如下：

- 市场是 Product、Release、CompositionRelease、Channel 和 Entitlement 的权威源；
- Artifact Storage 是制品内容、摘要、Referrer、复制和保留状态的权威源；
- HNB Cloud Platform 是租户、运行目标、ExecutionPlan、Operation、资源关系和运行治理的权威源；
- AI Extension Plane 是模型调用治理、AI 评测和 AI 运行状态的领域权威，但不替代平台资源权威；
- KubeEdge 管理集群保存边缘 CRD 的运行权威状态，HNB Platform 保存其业务投影、关联、策略和审计；
- 边缘本地状态必须带 `lastKnownStateAt`，不能在断连时被误表示为实时云端事实。

### 4.1 架构原则

- 市场、平台和 AI Extension Plane 独立部署、独立数据边界、独立升级；
- 三者只通过版本化 API、Provider、事件和回调集成；
- 市场不直接访问 Kubernetes API，AI Copilot 不直接执行 kubectl；
- 平台不直接修改市场发布内容；
- AI 结论不是平台事实源，权威事实来自 Kubernetes、Read Model、Resource Graph、Operation、审计和可观测数据；
- AI 只能生成解释、建议、评测结果或 ExecutionPlan 草案，变更必须经过权限、策略、审批、Operation 和审计；
- 制品使用摘要寻址，发布后不可原地覆盖；
- 平台执行时固定镜像、Chart、模型、Prompt 和配置摘要；
- 市场、AI 平面或中心控制面故障不影响已部署应用和数据面运行；
- AI Gateway 处理模型调用流量，但普通业务、数据库和消息流量不得经过 AI Gateway；
- 市场可服务多个平台实例，AI Extension Plane 也可服务多个受信平台或租户；
- 市场和平台 API 不代理大文件正文，客户端、Agent、Helm、容器运行时和模型加载器直接访问 Registry/S3；
- Registry 访问层尽量无状态，权威制品数据存储在可替换本地卷、PVC 或 S3 后端；
- 所有生产部署使用 digest，不以可变标签作为执行依据；
- 存储、模型运行时、AI Gateway、向量数据库和 AIOps 实现均通过 Provider 接入，不绑定具体开源项目或云厂商。

- Edge Pack、KubeEdge CloudCore、设备 Mapper 和 EdgeMesh 等边缘实现均通过 Provider/能力包接入，不编译进入 HNB Core；
- 云边控制流、设备数据流和应用数据流必须分层：CloudHub–EdgeHub 隧道只承载管理与元数据同步，不代理普通应用数据面；
- 边缘状态查询必须展示最后更新时间，断连对象的写操作默认排队或拒绝，不得假装实时成功；
- 同一边缘节点只允许一个权威纳管通道，禁止 HNB Agent 与 KubeEdge EdgeHub 双重接管。

## 5. HNB Cloud 最小内核

### 5.1 核心职责

HNB Core 仅保留：

- API Core；
- Identity 与 Tenant Context；
- Tenant、Project、Environment；
- RuntimeTarget 与 Cluster Registry；
- Resource Registry；
- Operation Engine；
- ExecutionPlan Engine；
- Plugin/Provider Registry；
- Capability Registry；
- Composition Resolver；
- Policy Hook；
- Read Model；
- Resource Graph；
- Audit；
- Agent Gateway；
- Secret Reference；
- 市场连接器和本地目录缓存；
- ArtifactStorageProfile、ArtifactSourcePolicy 和制品引用关系；
- ArtifactStorageProvider 注册、能力发现和健康聚合；
- AI Extension Connector、AI 能力发现和 Tenant Context 转发；
- AI 服务的租户配额、Operation 引用、审计关联和成本汇总接口；
- Edge Provider 注册、边缘能力发现、Edge Read Model 与边缘 Operation 关联接口；
- UsageRecord/MeterEvent 统一计量事件契约，覆盖资源、存储、AI 调用和边缘 WAN 用量；
- 配置、许可证和版本兼容管理。

以下内容不得编译进入内核：

- CNI、CSI；
- Karmada；
- KubeEdge CloudCore、EdgeCore、EdgeMesh 和设备 Mapper 实现；
- HAMi；
- 数据库和中间件 Operator；
- Helm 客户端的领域逻辑；
- 镜像扫描器；
- 运行时安全探针；
- 指标、日志和链路存储；
- OCI Registry；
- 对象存储具体实现；
- HNB App Market；
- AI Gateway、Model Service Manager、模型运行时、向量数据库、评测引擎和 AIOps 引擎；
- Service Mesh；
- GSLB、DNS 和复制产品。

### 5.2 工程形态

首期采用“模块化单体 + 独立 Worker/Agent/Provider”：

```text
platform-api
platform-controller
platform-worker
agent-gateway
cluster-agent
container-agent
web-console
market-connector
```

控制面模块均打包为容器镜像，通过 Helm Chart 或 Compose Profile 交付。模块成熟后可按性能、故障域或独立升级需要拆分，禁止以微服务数量衡量架构先进性。

---

## 6. HNB App Market 架构

### 6.1 市场组件

```text
HNB App Market
├── market-api
├── market-console
├── catalog-service
├── artifact-service
├── artifact-policy-controller
├── release-service
├── composition-service
├── entitlement-service
├── review-service
├── search-indexer
├── security-aggregator
├── sync-mirror-worker
├── lifecycle-gc-worker
├── integrity-scrub-worker
├── backup-restore-controller
├── offline-bundle-worker
└── event-publisher
```

全部组件容器化部署。最小模式可合并为模块化单体；生产模式可将索引、同步、复制、完整性巡检、生命周期回收、扫描聚合和离线导出 Worker 独立扩展。Registry 和对象存储属于可替换的基础设施能力，不嵌入 market-api 进程。

### 6.2 市场领域对象

```text
Publisher
Product
ProductCategory
ProductTag
Package
Artifact
ArtifactVersion
Release
ReleaseChannel
CompositionRelease
Dependency
CompatibilityPolicy
SecurityAttestation
Entitlement
Subscription
PromotionRequest
MirrorPolicy
ArtifactDescriptor
ArtifactStorageProfile
ArtifactSourcePolicy
RetentionPolicy
ReplicationPolicy
StorageHealthSnapshot
```

#### Product

用户可理解的软件产品，例如：

- 某业务应用；
- PostgreSQL 高可用服务；
- Kafka 集群；
- API 网关；
- Java 应用标准运行环境；
- 某行业解决方案。

#### Package

产品的一个部署单元，可以是：

- OCI 镜像；
- Helm Chart；
- JAR；
- WAR；
- Kubernetes YAML/Kustomize 包；
- Operator Bundle；
- 配置模板；
- 数据初始化包；
- Dashboard/告警规则包；
- 许可证或文档附件；
- 离线 Bundle。

#### Release

产品的不可变发布版本，固定：

- 所有 Package 版本；
- 制品摘要；
- 依赖版本范围；
- 参数 Schema；
- 兼容矩阵；
- 签名和 SBOM；
- 安全扫描结论；
- 发布渠道；
- 升级路径；
- 支持声明。

#### CompositionRelease

由多个 Product Release 组成的不可变解决方案版本，例如：

```text
客户管理系统 3.2.0
├── crm-web 3.2.0
├── crm-api 3.2.0
├── postgresql-ha 16.4-hnb2
├── valkey 8.0-hnb1
├── ingress-route 1.1.0
├── otel-policy 2.0.0
└── backup-policy 1.3.0
```

### 6.3 分类体系

市场至少支持以下一级分类：

| 一级分类 | 二级分类示例 |
|---|---|
| 应用 | Web 应用、移动后端、数据应用、行业应用、AI 应用、Agent 应用、边缘应用 |
| 数据库 | 关系型、键值、文档、时序、搜索、向量数据库 |
| 中间件 | 消息队列、缓存、注册配置、任务调度、API 网关 |
| AI 模型 | 语言、视觉、语音、多模态、Embedding、Rerank |
| AI 运行时 | 模型推理引擎、模型加载器、量化运行时、GPU/NPU Runtime |
| AI 数据与知识 | 向量服务、文档解析、知识服务、数据连接器 |
| AI 安全与评测 | Guardrail、内容审核、Prompt 防护、基准评测、红队评测 |
| 基础能力 | 网络、存储、GPU/NPU、证书、DNS、负载均衡、设备接入 |
| 可观测 | 指标、日志、链路、AI 调用观测、告警、Dashboard |
| 安全 | 镜像安全、运行时安全、策略、密钥、审计 |
| 运维与灾备 | 备份、恢复、迁移、容灾、诊断、AIOps |
| 解决方案 | 多应用组合、RAG 套件、智能客服、行业套件、平台能力包 |

分类支持多级树，但建议控制在三级以内。一个 Product 只能有一个主分类，可拥有多个辅助分类和标签。

### 6.4 标签体系

标签应分为标准标签和自定义标签：

- 技术栈：Java、Go、Node.js、Python；
- 架构：x86_64、arm64、多架构；
- 运行环境：Kubernetes、Container Engine；
- 交付方式：Helm、Operator、OCI Image、Artifact Runtime；
- 数据属性：有状态、无状态、需要持久化；
- 可靠性：单实例、高可用、跨可用区；
- 安全等级：基础、增强、合规；
- 生命周期：实验、兼容、生产、长期支持；
- 厂商和许可证；
- 行业和业务场景；
- 国产化适配；
- GPU/NPU 需求；
- AI 任务：生成、Embedding、Rerank、ASR、TTS、视觉、Agent；
- 模态：文本、图像、音频、视频、多模态；
- 模型架构、参数规模、量化精度、上下文长度和显存建议；
- 模型运行时、协议兼容和是否支持 OpenAI-compatible API；
- 数据与合规：许可证、数据出境、敏感数据、离线运行；
- 是否支持离线部署；
- 边缘能力：断连自治级别、持续 WAN 依赖、站点/节点组适用性；
- 边缘协议：MQTT、Modbus、OPC UA、CAN、BLE 等；
- 资源约束：最小 CPU/内存/磁盘、低功耗、ARMv7/ARM64/RISC-V 兼容性；
- 边缘运维：是否支持灰度、预拉取、Hold/Release、OTA 与自动回滚。

核心标签由平台治理，不允许发布方随意修改含义；租户和用户可增加私有收藏标签，但不改变产品标准元数据。

### 6.5 HNB Artifact Storage 统一制品存储

#### 6.5.1 定位与目标

`HNB Artifact Storage` 是应用市场和 HNB Cloud 之间共享的制品存储与分发能力，负责所有可交付软件包的不可变存储、可信访问、复制缓存和生命周期管理。

它必须同时满足：

1. **统一**：镜像、Helm、JAR/WAR、Operator、配置、SBOM 和离线包使用统一逻辑引用；
2. **轻量**：单机环境不强制安装 MinIO、Ceph、Redis 或搜索集群；
3. **高可用**：生产环境可容忍单个访问实例、节点或磁盘故障；
4. **高能效**：减少重复存储、重复传输、常驻计算和无效扫描；
5. **简单接入**：市场、平台、Agent 和用户只感知一个 Registry Endpoint 和统一 API；
6. **高效运维**：提供统一监控、容量、巡检、备份、恢复、升级和 GC；
7. **可替换**：底层可以是轻量 Registry、Harbor、企业 Registry、本地卷或 S3，不改变上层模型。

统一存储不是把所有二进制放入一个关系数据库，也不是为每类包分别建设镜像仓库、Chart 仓库、Maven 仓库和文件服务器。推荐采用：

> **统一逻辑目录 + OCI 统一制品模型 + 分级物理存储后端。**

#### 6.5.2 OCI 优先的统一制品模型

所有可交付制品优先映射为 OCI Image、OCI Artifact 或 OCI Image Layout：

| 制品类型 | 首选表示 | 说明 |
|---|---|---|
| 容器镜像 | OCI Image | 原生镜像 Manifest、Config 和 Layers |
| Helm Chart | Helm OCI Artifact | 不强制建设独立 Chart Repository |
| JAR/WAR | Generic OCI Artifact | 原始包作为 Layer，运行时镜像另以 digest 绑定 |
| YAML/Kustomize/配置包 | Generic OCI Artifact | 通过媒体类型和配置 Schema 描述 |
| Operator Bundle | OCI Artifact | 绑定 CRD、Operator 镜像和兼容矩阵 |
| 初始化脚本和数据包 | OCI Artifact | 必须声明权限、执行方式和安全校验 |
| SBOM、签名、证明 | OCI Referrer | 与主制品 digest 建立不可变关联 |
| 离线交付包 | OCI Image Layout/Bundle | 包含 manifest.lock、Blob、签名和证明 |

建议定义 HNB 媒体类型：

```text
application/vnd.hnb.jar.v1
application/vnd.hnb.war.v1
application/vnd.hnb.config.v1
application/vnd.hnb.data-package.v1
application/vnd.hnb.composition.v1
application/vnd.hnb.offline-bundle.v1
```

对象存储仍可以作为 Registry 的物理数据后端，但市场、平台和用户不直接依赖 Bucket 路径。所有业务引用使用：

```text
oci://registry.hnb.example.com/hnb/apps/order-service@sha256:...
oci://registry.hnb.example.com/hnb/charts/postgresql@sha256:...
oci://registry.hnb.example.com/hnb/jar/order-service@sha256:...
```

标签仅用于检索和渠道展示，生产部署、回滚、复制和审计必须使用 digest。

#### 6.5.3 分层架构

```text
                      统一访问入口
              registry.hnb.example.com
                         │
          ┌──────────────▼──────────────┐
          │ Ingress/LB + TLS + Token    │
          │ 限流、审计、租户授权         │
          └──────────────┬──────────────┘
                         │
          ┌──────────────▼──────────────┐
          │ OCI Registry Access Plane   │
          │ 2+ 无状态副本、Manifest/API  │
          └──────────────┬──────────────┘
                         │
          ┌──────────────▼──────────────┐
          │ Artifact Data Plane         │
          │ Local/PVC 或共享 S3          │
          │ Blob、Manifest、Referrer     │
          └─────────────────────────────┘

          ┌─────────────────────────────┐
          │ Market Metadata Plane       │
          │ Product/Release/Descriptor  │
          │ 引用、策略、审计、HA PG      │
          └─────────────────────────────┘
```

各层职责：

| 层次 | 职责 | 是否保存权威数据 |
|---|---|---:|
| 统一入口 | 域名、TLS、认证、限流、负载均衡 | 否 |
| Registry Access Plane | OCI API、Manifest 解析、Token 校验、上传下载协调 | 尽量无状态 |
| Artifact Data Plane | Blob、Manifest、Referrer 和复制数据 | 是 |
| Market Metadata Plane | 产品、版本、分类、授权、引用和生命周期状态 | 是 |
| Cache/Mirror Plane | 区域镜像、边缘缓存、节点缓存 | 否，可重建 |

市场 API 和平台 API 不得转发大文件内容。上传者、Agent、Helm、ORAS 客户端和容器运行时应在获得短期凭据后直接访问 Registry；使用 S3 后端时，可以启用对象存储直传直取或重定向，减少 Registry Pod 的网络和 CPU 压力。

#### 6.5.4 部署档位

| 档位 | Registry | 数据后端 | 元数据库 | 适用场景 |
|---|---|---|---|---|
| Minimal/Dev | 单副本轻量 Registry | 本地独立数据卷/PVC | 单实例 PostgreSQL | 开发、演示、功能验证，非 HA |
| Lite HA | 2 个以上无状态 Registry 副本 | 优先外部 S3；无外部 S3 时使用 3 节点轻量 S3 Provider | 3 实例 PostgreSQL 或外部 HA PG | 小型生产、边缘中心、三节点管理集群 |
| Standard HA | 多副本 Registry 或 Harbor | 企业 S3、NAS 对象接口或受支持的分布式对象存储 | HA PostgreSQL | 一般企业生产 |
| Enterprise/Multi-Region | 企业 Registry/Harbor HA | Ceph RGW、企业对象存储或云 S3，支持跨站点复制 | 多站点数据库恢复方案 | 集团、多地域和高级灾备 |

约束：

- 单节点本地卷不得被描述为高可用；
- 生产 HA 不允许多个 Registry 副本各自使用独立本地目录；
- 已有企业 S3、Ceph RGW、云对象存储或企业 Registry 时优先复用；
- Ceph 不作为轻量版强制依赖，仅在已有 Ceph 或同时承载块、文件、对象统一存储时采用；
- Registry、对象存储和数据库实现均通过 Provider/Connector 接入，不写死在平台内核。

#### 6.5.5 高可用设计

制品存储高可用必须分别设计以下故障域：

| 故障域 | 推荐措施 |
|---|---|
| 访问入口 | 至少 2 个 Ingress/LB 实例或企业负载均衡，证书和配置可恢复 |
| Registry | 至少 2 副本，跨节点反亲和，无本地权威状态 |
| 市场 API | 至少 2 副本，无状态部署 |
| 市场元数据库 | 同步/流复制、自动故障切换、PITR 和定期恢复演练 |
| 制品数据 | S3 多副本/纠删码或企业存储故障域保护 |
| Token/签名密钥 | 加密托管、版本化备份和轮换 |
| 区域缓存 | 非权威，可从中心重新构建 |

推荐的轻量生产高可用拓扑：

```text
节点1：Market/Registry/Ingress + PG实例 + S3实例
节点2：Market/Registry/Ingress + PG实例 + S3实例
节点3：                       PG实例 + S3实例
```

组件可以共用管理集群节点，但必须使用：

- Pod 反亲和和拓扑分布；
- 独立 PVC/磁盘和资源配额；
- 管理组件优先级与资源预留；
- 存储网络 QoS；
- PDB 和滚动升级策略；
- 单节点故障、单盘故障和 Registry 全部重建测试。

SLA、RPO 和 RTO 必须由部署档位和实际测试决定，不以组件名称直接承诺。建议参考目标：

| 档位 | 参考 RPO | 参考 RTO | 说明 |
|---|---:|---:|---|
| Minimal/Dev | 24 小时以内 | 4 小时以内 | 依赖定时备份，非 HA |
| Lite HA | 元数据 15 分钟以内；单节点故障制品不丢失 | 30 分钟以内 | 三节点容错，需完成恢复演练 |
| Standard HA | 元数据 5 分钟以内 | 15 分钟以内 | 外部 HA 存储和数据库 |
| Enterprise | 按业务等级定义 | 按业务等级定义 | 多站点复制、仲裁和切换 |

以上为设计目标，不直接构成商业 SLA。

#### 6.5.6 高能效设计

高能效同时关注存储、网络、计算和运维人力：

1. **内容寻址与去重**：相同 Blob 只保存一次，多个镜像和产品共享基础层；
2. **直传直取**：Market/Platform API 只签发授权，不代理大文件；
3. **分块与断点续传**：大文件支持 Multipart、断点续传和并行校验；
4. **区域镜像与边缘缓存**：跨地域首次拉取后就近复用，减少广域网流量；
5. **节点预拉取**：发布窗口前按目标节点和容量策略预热镜像；
6. **按需扫描**：扫描 Worker 使用 Job/KEDA 等方式按任务扩缩，空闲时可缩容到零；
7. **同 digest 复用结果**：相同制品不重复生成 SBOM 或重复扫描；
8. **冷热分层**：稳定热版本保留在标准层，历史版本进入低频或归档层；
9. **避免重复压缩**：JAR/WAR、Chart 等已压缩文件不再进行高成本重复压缩；
10. **安全 GC**：清理无业务引用、无运行引用且超过保留期的 Blob；
11. **缓存可重建**：区域和节点缓存不进入权威备份，故障后自动恢复；
12. **优先复用外部能力**：已有企业 S3、Registry、数据库和监控时不重复部署。

#### 6.5.7 简单接入

平台统一暴露：

```text
Registry Endpoint: registry.hnb.example.com
Market Endpoint:   market.hnb.example.com
Artifact URI:      artifact://{product}/{release}/{package}
OCI URI:           oci://registry.hnb.example.com/{repository}@{digest}
```

定义 `ArtifactStorageProfile`：

```yaml
apiVersion: platform.hnb.io/v1
kind: ArtifactStorageProfile
metadata:
  name: lite-ha
spec:
  registry:
    provider: zot
    endpoint: https://registry.hnb.example.com
    replicas: 2
  blobStorage:
    provider: s3
    secretRef: hnb-artifact-s3
    bucket: hnb-artifacts
  availability:
    failureDomains: 3
  cache:
    regionalMirror: optional
    nodePrefetch: true
  lifecycle:
    immutableTags: true
    retentionPolicyRef: production-default
    garbageCollection: scheduled
  security:
    signatureRequired: true
    tokenTTL: 15m
```

实现统一接口：

```go
type ArtifactStorageProvider interface {
    Capabilities(ctx context.Context) (*ArtifactCapabilities, error)
    Push(ctx context.Context, upload ArtifactUpload) (*ArtifactDescriptor, error)
    Resolve(ctx context.Context, ref ArtifactReference) (*ArtifactDescriptor, error)
    AuthorizePull(ctx context.Context, subject Subject, ref ArtifactReference) (*ScopedCredential, error)
    Copy(ctx context.Context, req ReplicationRequest) (*OperationRef, error)
    Attach(ctx context.Context, subject ArtifactReference, attachment ArtifactUpload) (*ArtifactDescriptor, error)
    ListReferrers(ctx context.Context, subject ArtifactReference) ([]ArtifactDescriptor, error)
    Verify(ctx context.Context, ref ArtifactReference) (*VerificationResult, error)
    MarkDelete(ctx context.Context, ref ArtifactReference) (*OperationRef, error)
    GarbageCollect(ctx context.Context, policy RetentionPolicy) (*OperationRef, error)
    Health(ctx context.Context) (*StorageHealthSnapshot, error)
}
```

Provider 必须声明：

- OCI Image、Helm OCI、Generic Artifact 和 Referrer 支持情况；
- 本地、PVC、S3 后端能力；
- 不可变标签、复制、代理缓存和 GC 能力；
- 多副本和故障域要求；
- 签名、扫描、审计和限流能力；
- 版本兼容范围和升级路径。

普通用户和平台管理员不需要理解 Bucket、对象路径或 Registry 内部实现。

#### 6.5.8 制品发布一致性

发布采用分阶段事务：

```text
上传 Blob
→ Registry 校验 digest
→ 生成 ArtifactDescriptor
→ 关联签名、SBOM 和扫描结果
→ 创建不可变 Release
→ 审批
→ 晋级到发布渠道
```

失败处理：

- Blob 上传完成但 Release 未创建：标记为孤儿候选，进入延迟清理；
- Release 已创建但安全门禁失败：保持不可部署状态，不覆盖原制品；
- 渠道晋级失败：不影响已有 Release 和已部署实例；
- 市场元数据与 Registry 短时不一致：以 digest 为内容真值，以市场 Release 状态为可部署真值，后台执行校准。

#### 6.5.9 生命周期与安全垃圾回收

制品删除采用两阶段流程：

```text
版本废弃/撤销
→ 停止新授权和新部署
→ 查询 Composition、运行实例、回滚点、灾备点和离线包引用
→ 标记 Tombstone
→ 进入保留期
→ 删除 Manifest
→ Registry 执行 Blob GC
→ 生成审计报告
```

不得由 Registry 管理员绕过市场和平台直接删除生产制品。以下内容默认受保护：

- stable/lts 发布；
- 当前运行实例使用的 digest；
- 允许回滚的历史版本；
- 灾备恢复点和演练使用版本；
- 未过审计保留期的撤销制品；
- 离线 Bundle 引用的制品；
- 主制品关联的签名、SBOM 和证明。

GC 必须通过 Operation Engine 执行，支持预览、分布式锁、限速、暂停、重试和空间回收报告。

#### 6.5.10 分发、镜像和缓存

云、区域、站点和节点统一采用分层分发：

```text
中心权威 Registry
        │
区域 Registry Mirror / Proxy Cache（可重建）
        │
边缘站点 Registry Mirror（按需，可重建）
        │
集群/边缘节点 containerd、CRI 或 OCI Engine 缓存
```

`ArtifactSourcePolicy` 定义可信源、优先级、带宽和回退：

```yaml
apiVersion: platform.hnb.io/v1
kind: ArtifactSourcePolicy
metadata:
  name: region-a
spec:
  preferredSources:
    - https://registry.site-a.hnb.example.com
    - https://registry.region-a.hnb.example.com
    - https://registry.central.hnb.example.com
  verifyDigest: true
  verifySignature: true
  allowExternalRegistry: false
  prefetch: true
  fallback: ordered
  bandwidth:
    profile: metered
    maxMbps: 20
    transferWindows: ["01:00-06:00"]
```

原则：

- 中心仓库保存权威制品，区域镜像、站点镜像和节点缓存均可丢失、可重建；
- 平台根据地域、站点、网络质量、健康和信任策略选择源；
- 复制必须保持 digest、签名、SBOM、构建证明和撤销状态的关联；
- 声明支持 EdgeRuntimeTarget 的 Release 必须提供所需架构的 OCI Manifest List；
- 大镜像和模型在部署窗口前通过 ImagePrePullJob 或 Provider 预拉取；
- WAN 复制支持限速、传输窗口、断点续传、优先级和中心出口计量；
- 撤销列表、离线策略和信任根更新以签名 OCI Artifact 发布，并随区域/站点分发链同步；
- 完全离线环境通过签名 OCI Image Layout Bundle 导入本地权威 Registry；
- 边缘缓存 GC 仍遵守引用保护、Tombstone、保留期和审计，不得因磁盘压力直接删除运行引用。

#### 6.5.11 高效运维

HNB 控制台提供统一“制品存储中心”，至少展示：

- Registry、对象存储、数据库和入口健康；
- 容量、增长率、剩余天数和存储分层；
- Product、Release、Manifest、Blob 数量；
- 上传/下载吞吐、P95 延迟和失败率；
- 区域复制延迟和失败队列；
- 缓存命中率和热点制品；
- 扫描队列、重扫范围和 Worker 资源；
- 孤儿 Blob、可回收空间和 GC 结果；
- 备份状态、最近恢复演练和 RPO 偏差；
- 证书、Token 签名密钥和访问策略状态。

自动巡检建议：

| 周期 | 巡检内容 |
|---|---|
| 1 分钟 | Registry API、入口、数据库主备和错误率 |
| 5 分钟 | S3 读写探测、磁盘水位、复制队列、证书有效期 |
| 每日 | 随机 Blob 摘要校验、孤儿 Manifest、备份完成度、容量预测 |
| 每周 | GC 预览、热点与冷数据分析、恢复点抽检 |
| 每月 | 数据库恢复、Registry 重建、节点故障和区域切换演练 |

升级遵循：

```text
兼容检查
→ 配置和数据库备份
→ 新 Registry 副本上线
→ 上传/拉取/签名/Referrer 验证
→ 分批下线旧副本
→ 执行必要迁移
→ 完成或回滚
```

Registry、对象存储、数据库和市场应用不得在同一个维护步骤中同时进行高风险升级。

#### 6.5.12 备份与灾备

权威备份对象：

| 数据 | 备份策略 |
|---|---|
| 市场 PostgreSQL | 定时全量 + WAL/PITR + 异地副本 |
| Artifact Data Plane | 多副本、Bucket 版本/复制、卷快照或对象存储原生保护 |
| Registry/Market 配置 | Git/配置库版本化 + 加密备份 |
| Token 和签名密钥 | KMS/Secret 管理系统托管、加密离线备份和轮换记录 |
| 审计与 Release Lock | 独立归档并与制品 digest 关联 |

恢复顺序：

```text
恢复密钥和基础配置
→ 恢复或连接 Artifact Data Plane
→ 恢复市场元数据库
→ 部署 Registry 和 Market 无状态服务
→ 执行索引重建与一致性校准
→ 验证随机制品 digest、签名和下载
→ 恢复发布与删除操作
```

区域缓存和节点缓存不作为权威备份对象。

#### 6.5.13 推荐实现与产品边界

参考实现建议：

- Lite/Edge：轻量 OCI Registry，例如 zot 类实现；
- Standard：轻量 Registry 多副本、Harbor 或企业现有 OCI Registry；
- Enterprise：Harbor HA、企业 Registry 或云厂商 Registry；
- Artifact Data Plane：优先复用现有 S3；无现有 S3 时通过认证的轻量 S3 Provider；
- Ceph RGW：已有 Ceph 或同时需要块、文件、对象统一能力时采用。

这些是参考实现而非内核依赖。产品支持范围由 Provider 认证矩阵确定。

市场数据库只保存元数据、索引、引用和生命周期状态，不保存大文件正文。

### 6.6 发布渠道

建议支持：

- `dev`：开发验证；
- `test`：集成测试；
- `candidate`：候选发布；
- `stable`：生产稳定；
- `lts`：长期支持；
- `deprecated`：已弃用；
- `revoked`：已撤销。

版本进入 `stable` 或 `lts` 前必须完成：

- 制品摘要锁定；
- 签名验证；
- SBOM 完整性检查；
- 漏洞和恶意文件扫描；
- 兼容性测试；
- 安装、升级、回滚和卸载测试；
- 许可证和来源审查；
- 人工或策略审批。

---

## 7. 应用市场、制品存储、平台、AI 与边缘扩展职责分工

### 7.1 RACI 边界

| 能力 | HNB App Market | HNB Artifact Storage | HNB Cloud Platform | HNB AI Extension Plane |
|---|---|---|---|---|
| 应用、模型和解决方案建模 | 主责 Product/Release/Composition | 保存制品与引用 | 读取、映射为 Blueprint/ExecutionPlan | 提供模型运行、评测和 AI 能力元数据 |
| 分类、标签和检索 | 主责 | 不负责 | 展示、筛选和本地缓存 | 提供模型任务、运行时和评测标签建议 |
| 制品上传入口 | 受理并校验发布上下文 | 主责接收内容、计算摘要 | 不负责上传 | 可提交模型、Prompt、Guardrail 和评测制品 |
| Blob/Manifest 保存 | 保存 ArtifactDescriptor 和引用 | 主责 | 不保存正文 | 不保存权威正文，可维护本地模型缓存 |
| 版本、渠道和发布审核 | 主责 | 保证内容不可变 | 选择允许渠道 | 提供模型评测、安全和运行兼容结果 |
| 签名、SBOM、模型证明和评测 | 聚合发布门禁 | 保存 Referrer/证明 | 部署前再验证 | 执行模型评测、Guardrail 和 AI 风险检查 |
| 产品授权和可见范围 | 主责 Entitlement | 执行短期仓库访问权限 | 叠加租户、项目和环境权限 | 执行模型端点、Token、预算和 Provider 访问策略 |
| 组合应用定义 | 主责 CompositionRelease | 保存组合附件和锁文件 | 解析为 ExecutionPlan 并执行 | 提供 AI DAG 节点和生命周期能力 |
| 复制、镜像和缓存 | 定义业务 MirrorPolicy | 主责执行 | 选择最近可信源并触发预拉取 | 管理模型运行缓存和模型加载策略 |
| 保留和垃圾回收 | 判断发布引用 | 执行 Tombstone、Manifest 删除和 Blob GC | 提供运行、回滚和灾备引用 | 提供 ModelService、评测和模型缓存引用 |
| 容量、完整性和备份 | 展示业务视图 | 主责制品存储运维 | 聚合平台视图和告警 | 提供模型缓存、评测数据和 AI 配置备份状态 |
| 租户、项目和环境 | 不管理 | 仅执行访问隔离 | 权威管理 | 消费不可伪造 Tenant Context，不自行建立第二套租户体系 |
| 集群、节点和运行目标 | 不管理 | 不管理 | 权威管理 | 读取授权后的目标能力，不直接纳管集群 |
| 配额、网络、业务存储和 GPU/NPU | 不管理 | 仅管理制品存储资源 | 权威分配和准入 | 执行 AI Token/并发/预算策略，资源申请交由平台 |
| 参数填写体验 | 提供产品和模型参数 Schema | 不负责 | 注入目标环境和资源参数 | 提供 Prompt、Guardrail、模型运行和评测参数 Schema |
| 部署预检 | 提供产品、许可和发布规则 | 提供可访问性和完整性状态 | 主责环境、配额、安全和依赖预检 | 提供模型资源、Provider、评测和 AI 策略预检 |
| 审批 | 发布审批 | 不负责业务审批 | 资源申请和部署审批 | 提供 AI 风险等级，不自行绕过审批执行 |
| 实际部署 | 禁止直接执行 | 只提供制品 | 主责创建 Operation 并执行 | 作为 Provider/控制器执行 ModelService、Gateway 和评测节点 |
| 模型调用路由 | 不负责 | 不负责 | 配置租户和项目授权 | AI Gateway 主责路由、限流、Fallback 和内容安全 |
| Prompt/Guardrail/评测 | 发布不可变版本 | 保存制品与证明 | 绑定租户、项目和环境策略 | 主责运行、观测和结果输出 |
| Platform Copilot/AIOps | 可发布能力包和 Runbook | 保存版本化 Prompt/规则 | 提供权威数据、权限、Operation 和审计 | 主责理解、证据归纳、建议和受控自动化 |
| 运行状态 | 接收汇总或匿名统计 | 不负责 | 应用、服务和 Operation 最终数据源 | 模型端点、AI Gateway、评测和 AI 调用状态源，并回传平台聚合 |
| 用量和成本 | 管理商业授权和产品计价元数据 | 提供存储用量 | 汇总租户资源和服务成本 | 生成 Token、模型、Provider 和 AI 调用用量 |
| 扩缩容、备份和恢复 | 描述支持能力 | 负责市场制品备份 | 主责应用、数据和资源生命周期 | 执行 AI Provider 生命周期，但必须受平台 Operation 控制 |
| 升级路径 | 定义允许版本图 | 保证旧制品在保留期可访问 | 生成升级计划并执行 | 提供模型灰度、评测门禁、流量切换和回退能力 |
| 审计 | 发布和市场操作审计 | 制品访问、复制、删除和 GC 审计 | 部署、资源和运行操作权威审计 | AI 调用、模型、Prompt、证据和自动化审计 |

#### 7.1.1 Edge Pack 专项职责边界

| 能力 | HNB Cloud Platform | Edge Pack / KubeEdge | HNB App Market | Artifact Storage |
|---|---|---|---|---|
| 边缘产品定义 | 解析 Blueprint 和策略 | 不定义产品 | 主责 Release/Composition/兼容性 | 保存制品与证明 |
| 节点/节点组 | 权威业务模型、租户可见性和配额 | 执行节点注册、心跳和 CRD 状态 | 不管理 | 不管理 |
| 边缘部署 | 生成 ExecutionPlan/Operation | 翻译并执行 EdgeApplication/Pod/任务 | 提供不可变发布 | 提供可信制品与预分发 |
| 断连自治 | 定义策略、展示 lastKnownStateAt | MetaManager/Edged 按缓存状态执行 | 声明 Release 自治能力 | 保证离线制品可用 |
| 设备管理 | 权限、Operation、审计和业务投影 | Device/DeviceModel/Mapper 执行 | 发布 Mapper 和设备模型产品 | 保存 Mapper/配置/证明 |
| OTA/配置更新 | 维护窗口、灰度、审批、审计 | NodeUpgradeJob/ConfigUpdateJob/ImagePrePullJob | 发布兼容版本 | 提供升级制品 |
| 撤销处置 | 影响分析、策略决策和 Operation | 重连后执行隔离/升级/停用 | 发布撤销状态 | 分发签名撤销 Artifact |
| 边缘遥测 | Read Model、SLO 和告警聚合 | 本地缓冲、采样、重连补传 | 不负责 | 不负责 |

### 7.2 禁止的耦合方式

- 市场保存业务集群 kubeconfig 或直接调用 Helm；
- AI Extension Plane 直接持有集群管理员凭据或绕过 Platform API 创建资源；
- Copilot、AIOps 或 Agent 直接执行 kubectl、数据库管理命令和云厂商 API；
- 平台修改已发布 ReleaseManifest、ModelArtifact、Prompt 或 Guardrail；
- 市场、平台和 AI 平面共享业务数据库或内部表结构；
- AI 平面自行建立与平台冲突的租户、项目、角色和配额权威模型；
- 市场使用 `latest` 标签作为生产部署依据，平台执行未锁定远端依赖；
- 发布者上传新文件覆盖既有应用、模型、Prompt 或评测版本；
- 市场、AI 平面或模型服务不可用导致普通应用停止；
- 平台把运行 Secret、完整 Prompt、响应、业务日志或用户数据回传市场；
- Market API、Platform API 或 Copilot API 代理镜像层、模型权重和离线 Bundle 大文件传输；
- AI Gateway 承担普通业务、数据库、消息或文件传输；
- 多个 Registry HA 副本各自使用独立本地权威目录；
- 应用市场直接依赖对象存储 Bucket 路径作为业务制品 URI；
- Registry 管理员绕过市场、平台和 AI 引用检查直接删除生产制品；
- AIOps 以自然语言输出直接触发高风险操作而没有结构化计划、策略和审批。

- KubeEdge 边缘节点同时部署 HNB cluster-agent/container-agent 形成双通道接管；
- Edge Provider 直接建立第二套租户、授权、审批、发布或审计模型；
- Karmada 与 KubeEdge 对同一节点重复调度和生命周期管理；
- CloudHub–EdgeHub 控制隧道代理普通应用、视频流、数据库或大文件业务流量；
- 断连节点的最后状态被展示为实时状态，或 Copilot 隐藏 `lastKnownStateAt`。

### 7.3 推荐部署关系

#### 模式 A：独立市场 + 单个平台 + 可选 AI 平面

适用于中小企业。市场、平台和 AI Extension Plane 分别以 Helm Chart 部署，共享 HNB Artifact Storage 和企业 IAM，但使用独立数据库、ServiceAccount 和网络策略。未启用 AI 时平台功能完整。

#### 模式 B：集团市场 + 多个平台实例 + 中央 AI 服务

```text
集团 HNB App Market
├── HNB Cloud 华东平台 ─┐
├── HNB Cloud 华南平台 ─┼── 中央 AI Extension Plane / AI Gateway
├── HNB Cloud 测试平台 ─┤
└── HNB Cloud 边缘平台 ─┘
```

市场统一发布和授权，各平台独立执行和运行；中央 AI 平面统一接入模型、评测和成本，但每次调用必须携带平台、Tenant、Project 和数据区域上下文。

#### 模式 C：每个平台独立 AI 平面

适用于高隔离、多地域或数据不出域场景。各平台部署本地 AI Gateway、模型运行时和 AIOps，中央市场仅同步签名制品和元数据，不汇聚 Prompt、响应和业务数据。

#### 模式 D：离线私有市场与本地 AI

中心市场导出签名 OCI Image Layout Bundle，离线环境导入本地 Registry、市场和可选本地模型。离线平台不依赖互联网；外部模型 Connector 默认禁用，模型、Prompt、Guardrail 和评测包均经过离线审核。

## 8. 市场、平台与 AI 平面接口集成

### 8.1 集成方式

采用：

- REST/OpenAPI：查询、授权、发布内容获取；
- OCI Distribution API：镜像、Chart、JAR/WAR 和通用 OCI Artifact 上传、获取与 Referrer 查询；
- Artifact Storage API：能力发现、短期凭据、复制、健康、保留和 GC 编排；
- AI Control API：AI Provider、ModelService、ModelEndpoint、Gateway Route、Prompt、Guardrail 和 Evaluation 管理；
- AI Runtime API：HTTP、SSE、WebSocket/OpenAI-compatible 模型调用；
- OTLP/事件接口：AI 调用、Token、GPU、成本、评测和 AIOps 证据上报；
- CloudEvents：发布、撤销、安全状态和授权变化通知；
- Webhook/Callback：部署状态和兼容结果回传；
- OAuth2 Client Credentials 或 mTLS：服务间认证；
- 可选消息总线：高规模环境异步事件分发。

禁止通过数据库直连或共享文件目录集成。大文件数据流不得经过 Market API、Platform API 或 AI Control API；这些接口只传递 ArtifactDescriptor、digest、授权、策略和状态。AI Runtime API 只承载模型调用，不承载模型权重和普通业务文件。

### 8.2 核心 API

#### 市场对平台

```text
GET  /api/v1/catalog/products
GET  /api/v1/catalog/products/{id}
GET  /api/v1/releases/{releaseId}
GET  /api/v1/releases/{releaseId}/manifest
POST /api/v1/releases:resolve
POST /api/v1/entitlements:check
GET  /api/v1/artifacts/{artifactId}
POST /api/v1/artifacts/{artifactId}:authorize-pull
POST /api/v1/artifacts:authorize-push
POST /api/v1/artifacts:replicate
GET  /api/v1/artifact-storage/health
GET  /api/v1/channels/{channel}/updates
POST /api/v1/offline-bundles:export
POST /api/v1/artifacts:gc-preview
```

#### 平台对市场

```text
POST /api/v1/platforms/register
POST /api/v1/deployments/report
POST /api/v1/compatibility/report
POST /api/v1/usage/report
POST /api/v1/security/runtime-findings
GET  /api/v1/platforms/{id}/sync-cursor
POST /api/v1/artifact-references/report
POST /api/v1/artifact-prefetch/report
```

使用量和运行安全信息默认只回传最小必要字段，禁止回传业务数据、Secret、完整日志或用户内容。

#### 平台对 AI Extension Plane

```text
GET  /api/v1/ai/providers
POST /api/v1/ai/model-services:plan
POST /api/v1/ai/model-services
GET  /api/v1/ai/model-services/{id}
POST /api/v1/ai/model-services/{id}:scale
POST /api/v1/ai/model-services/{id}:upgrade
DELETE /api/v1/ai/model-services/{id}
POST /api/v1/ai/gateway-routes
POST /api/v1/ai/evaluations
POST /api/v1/ai/copilot:query
POST /api/v1/ai/aiops:analyze
```

平台调用时必须携带签名 Tenant Context、Project、Environment、Operation ID、调用者和策略版本。创建、升级、扩缩容和删除操作必须关联平台 Operation。

#### AI Extension Plane 对平台

```text
POST /api/v1/ai/capabilities/report
POST /api/v1/ai/model-endpoints/report
POST /api/v1/ai/usage/report
POST /api/v1/ai/evaluations/report
POST /api/v1/ai/security-findings/report
POST /api/v1/ai/insights/report
POST /api/v1/ai/remediation-proposals/report
GET  /api/v1/platform-context/resources:query
GET  /api/v1/platform-context/graph:query
```

资源和图谱查询必须经过平台权限过滤。AI 平面不得直接读取平台数据库、Secret 存储和未授权日志后端。

#### AI Gateway 运行时接口

```text
POST /v1/chat/completions
POST /v1/responses
POST /v1/embeddings
POST /v1/audio/transcriptions
POST /v1/audio/speech
WS   /v1/realtime
```

具体协议由 Gateway Provider 声明。平台不要求所有部署同时支持全部接口。

### 8.3 ReleaseManifest

市场返回不可变发布清单，通用结构必须同时覆盖云、单机和边缘兼容性：

```yaml
apiVersion: market.hnb.io/v1
kind: ReleaseManifest
metadata:
  productId: edge-collector
  releaseId: edge-collector-2.1.0
  version: 2.1.0
spec:
  category: application/edge
  channel: stable
  packageType: helm
  artifacts:
    - name: chart
      mediaType: application/vnd.cncf.helm.chart.content.v1.tar+gzip
      uri: oci://registry.example.com/hnb/charts/edge-collector
      digest: sha256:...
    - name: workload-image
      mediaType: application/vnd.oci.image.index.v1+json
      uri: registry.example.com/hnb/edge/collector
      digest: sha256:...
  parameterSchemaRef: oci://registry.example.com/hnb/schema/edge-collector@sha256:...
  requiredCapabilities:
    - artifact.oci-image
  optionalCapabilities:
    - observability.metrics
    - edge.device-management
  compatibility:
    kubernetes: ">=1.30 <1.35"
    architectures: [amd64, arm64]
    targetTypes: [kubernetes, kubeedge-edge]
    resources:
      minCPU: 250m
      minMemory: 256Mi
      minDisk: 1Gi
    edge:
      offlineAutonomy: supported       # required | supported | not-supported
      wanRequired: false
      deviceProtocols: [mqtt, modbus]
      holdAndReleaseSupported: true
  security:
    signatureRequired: true
    signaturePolicyRef: enterprise-cosign
    sbomRefs:
      - oci://registry.example.com/hnb/sbom/edge-collector@sha256:...
    provenanceRefs:
      - oci://registry.example.com/hnb/provenance/edge-collector@sha256:...
  lifecycle:
    install: true
    upgrade: true
    rollback: true
    scale: false
    uninstall: true
```

发布门禁必须校验：

- `targetTypes` 与 Blueprint/Provider 支持范围一致；
- 声明 `kubeedge-edge` 时，必须提供所需 CPU 架构、资源下限、WAN 依赖和自治声明；
- 运行探针不得把持续访问云端 API 作为 `offlineAutonomy: supported|required` 的必要条件；
- 设备访问、特权、HostPath 和 hostNetwork 必须显式声明并进入风险审批；
- 多架构镜像必须校验 Manifest List 中每个平台条目的摘要和签名；
- 边缘兼容性测试通过后才能进入 stable/lts 渠道。

AI 产品在通用结构基础上增加 `spec.ai`：

```yaml
spec:
  category: ai/model/language
  packageType: model-artifact
  ai:
    task: text-generation
    modalities: [text]
    runtimeProviders: [openai-compatible-runtime]
    supportedTargets: [kubernetes, kubeedge-edge]
    accelerator:
      optional: true
      recommendedMemory: 20Gi
    edge:
      quantization: [int8, int4]
      maxModelSize: 8Gi
      localFallback: true
    licensePolicyRef: enterprise-approved
    evaluationSuiteRefs:
      - oci://registry.example.com/hnb/evals/llm-basic@sha256:...
    guardrailPolicyRefs:
      - oci://registry.example.com/hnb/guardrails/default@sha256:...
```

市场负责 Release 的不可变版本与发布门禁；平台、AI Extension Plane 和 Edge Provider分别完成目标能力、资源、评测、网络和自治策略预检。

### 8.4 市场本地缓存

平台维护只读 `MarketCache`：

- 产品摘要和图标；
- 已授权 Release 元数据；
- 参数 Schema；
- 已解析依赖图；
- 已部署 Release 的完整 Manifest；
- 已签名撤销列表及其摘要、签发时间、有效期和信任根；
- 最近同步游标和最后成功同步时间。

市场短时不可用时，平台可按策略使用已缓存且未撤销、制品可访问、兼容性仍有效的版本继续部署。长期离线站点必须使用本地市场、本地权威 Registry 和签名 OCI Image Layout Bundle。

撤销列表不能只依赖在线事件：

1. 市场发布 `market.release.revoked` 事件；
2. 同时生成签名的撤销 OCI Artifact；
3. Artifact Storage 按中心→区域→站点链路分发；
4. 在线目标立即停止新部署；断连边缘按 `offlineRevocationPolicy` 继续运行或进入受限模式；
5. 重连后强制完成影响分析、隔离/升级/停用等处置并进入审计。

### 8.5 事件

至少定义：

- `market.product.published`；
- `market.release.promoted`；
- `market.release.deprecated`；
- `market.release.revoked`；
- `market.security.updated`；
- `market.entitlement.changed`；
- `artifact.storage.degraded`；
- `artifact.replication.failed`；
- `artifact.integrity.failed`；
- `artifact.gc.completed`；
- `artifact.capacity.warning`；
- `platform.deployment.started`；
- `platform.deployment.completed`；
- `platform.deployment.failed`；
- `platform.compatibility.reported`；
- `ai.model.release.evaluated`；
- `ai.modelservice.ready`；
- `ai.modelservice.degraded`；
- `ai.endpoint.revoked`；
- `ai.gateway.fallback.triggered`；
- `ai.security.blocked`；
- `ai.usage.budget.warning`；
- `ai.evaluation.completed`；
- `ai.aiops.insight.created`；
- `ai.remediation.proposed`。

- `edge.node.joined`；
- `edge.node.offline`；
- `edge.node.reconnected`；
- `edge.application.rollout.started`；
- `edge.application.rollout.paused`；
- `edge.ota.completed`；
- `edge.device.anomaly.detected`；
- `edge.telemetry.backfill.completed`；
- `usage.meter.recorded`。

事件只作为状态变化通知，关键读取仍应通过 API 获取完整权威数据。

---

## 9. 软件包与制品模型

### 9.1 统一包类型

| 包类型 | 市场存储 | 平台执行方式 | 适用场景 |
|---|---|---|---|
| `oci-image` | OCI Registry | Deployment/StatefulSet/Container Engine | 单容器应用和基础组件 |
| `model-artifact` | OCI Registry/受信模型仓库 | AI Runtime Provider | 模型权重、Tokenizer 和推理配置 |
| `prompt-package` | OCI Artifact | AI Gateway/AI Application | Prompt 模板、版本和变量 Schema |
| `guardrail-package` | OCI Artifact | AI Governance Provider | 内容安全、脱敏和策略规则 |
| `evaluation-suite` | OCI Artifact | Evaluation Provider | 基准数据、评分器和阈值 |
| `helm-chart` | OCI Registry | Helm Provider | Kubernetes 应用和服务 |
| `operator-bundle` | OCI Registry | Operator Provider | 数据库、中间件和复杂有状态服务 |
| `jar-artifact` | OCI Artifact | Artifact Runtime Provider | Java 迁移兼容场景 |
| `war-artifact` | OCI Artifact | Artifact Runtime Provider | 传统 Servlet 应用迁移 |
| `k8s-manifest` | OCI Artifact | Manifest Provider | 简单原生资源模板 |
| `config-package` | OCI Artifact | Configuration Provider | 配置、Dashboard、告警规则 |
| `data-package` | OCI Artifact | Init Job/受控迁移 Job | 初始化数据和脚本 |
| `composition` | 市场元数据 | Composition Engine | 多应用、多服务解决方案 |
| `offline-bundle` | OCI Image Layout | Bundle Importer | 隔离网络交付 |

### 9.2 包版本规则

- 使用语义化版本或平台认可的可排序版本；
- 同一版本发布后不可覆盖；
- 每个制品必须记录 SHA-256 digest；
- 镜像标签只能作为展示信息，执行必须使用 digest；
- Chart 中引用的镜像必须在 Release 锁文件中固定 digest；
- JAR/WAR 必须固定摘要并绑定运行时镜像摘要；
- 所有依赖均生成 `dependency.lock`；
- 撤销版本不得用于新部署，但已有实例的处置由平台策略决定。

### 9.3 JAR/WAR 包描述

```yaml
apiVersion: market.hnb.io/v1
kind: ArtifactRuntimePackage
metadata:
  name: customer-api
spec:
  artifact:
    type: jar
    uri: oci://registry.example.com/hnb/jar/customer-api@sha256:...
    digest: sha256:...
  runtimeImage:
    uri: registry.example.com/hnb/java-runtime:21.0.4
    digest: sha256:...
  command:
    - java
    - -XX:MaxRAMPercentage=75
    - -jar
    - /opt/hnb/app/application.jar
  ports:
    - name: http
      containerPort: 8080
  health:
    readinessPath: /actuator/health/readiness
    livenessPath: /actuator/health/liveness
  resources:
    minimum:
      cpu: 500m
      memory: 1Gi
  security:
    runAsNonRoot: true
    readOnlyRootFilesystem: true
    allowPrivilegeEscalation: false
```

平台将该描述转换为标准 Kubernetes 或 Container Engine 工作负载，不在主机上直接执行 JAR/WAR。

### 9.4 制品来源

支持：

- 市场直接上传；
- 从企业 OCI Registry 导入；
- 从外部 Helm OCI 导入；
- 从兼容 OCI Registry 或受控对象存储导入并转换为 ArtifactDescriptor；
- 从 CI/CD 系统通过发布 API 推送；
- 从上游市场镜像同步；
- 离线 Bundle 导入。

市场只接收已构建制品。代码编译、单元测试和完整构建流水线仍由外部研发工具完成。

---

## 10. 多软件包统一编排与部署

### 10.1 编排职责

市场负责定义**组合发布**，平台负责执行**部署编排**：

```text
市场 CompositionRelease
        │
        ▼
平台解析依赖和参数
        │
        ▼
目标能力、配额、安全预检
        │
        ▼
生成不可变 ExecutionPlan
        │
        ▼
Operation Engine 执行 DAG
        │
        ▼
健康验证、输出绑定、回滚或完成
```

### 10.2 CompositionRelease 模型

```yaml
apiVersion: market.hnb.io/v1
kind: CompositionRelease
metadata:
  productId: customer-management-suite
  version: 3.2.0
spec:
  components:
    - id: database
      releaseRef: postgresql-ha-16.4-hnb2
    - id: cache
      releaseRef: valkey-8.0-hnb1
    - id: api
      releaseRef: customer-api-3.2.0
      dependsOn: [database, cache]
    - id: web
      releaseRef: customer-web-3.2.0
      dependsOn: [api]
    - id: gateway
      releaseRef: http-route-1.1.0
      dependsOn: [web, api]
  bindings:
    - from: database.outputs.connectionSecret
      to: api.inputs.databaseSecret
    - from: cache.outputs.connectionSecret
      to: api.inputs.cacheSecret
    - from: api.outputs.serviceEndpoint
      to: web.inputs.apiEndpoint
  conditions:
    - component: cache
      enabledWhen: "$.parameters.cache.enabled == true"
  lifecycle:
    installOrder: [database, cache, api, web, gateway]
    uninstallOrder: [gateway, web, api, cache, database]
```

### 10.3 平台 ExecutionPlan

平台不得直接按市场文件原样执行，而应生成包含环境决策的计划：

- 租户、项目和环境；
- 目标集群或容器主机；
- Namespace；
- StorageClass；
- NetworkProfile；
- Gateway 和域名；
- GPU/CPU/内存配额；
- Secret 引用；
- 镜像镜像源和拉取策略；
- 安全策略；
- 备份策略；
- 每个步骤的 Provider；
- 重试、超时和补偿；
- 所有制品 digest；
- 执行审批和审计信息。

ExecutionPlan 一旦开始执行必须不可变；参数变更应创建新的 Operation 或修订计划。

### 10.4 编排能力

组合引擎支持：

- 有向无环依赖图；
- 顺序和并行步骤；
- 条件组件；
- 参数继承和覆盖；
- 输出引用；
- Secret 安全传递；
- 资源名称和命名空间映射；
- 跨 Chart、镜像和 Operator 包组合；
- 预安装和安装后 Job；
- 健康门禁；
- 超时与重试；
- 失败补偿；
- 局部重试；
- 断点恢复；
- 版本锁定；
- Drift 检测；
- 升级依赖顺序；
- 数据保护型卸载。

### 10.5 事务边界

分布式应用部署无法提供传统数据库式全局事务。平台采用 Saga：

- 每个步骤定义 `apply`、`observe` 和可选 `compensate`；
- 有状态服务默认不自动删除数据；
- 失败时优先停止后续步骤，再按保护策略补偿；
- 已创建的数据库可进入 `NeedsAttention`，由用户选择保留、重试或回收；
- 所有补偿行为必须审计。

### 10.6 Helm 与平台编排关系

- 单一产品内部强耦合组件可使用一个 Helm Chart；
- 跨产品、跨生命周期和跨 Provider 的组合由平台 Composition Engine 负责；
- 不建议用超大型 umbrella chart 表达数据库、业务应用、网关、备份和可观测的全部生命周期；
- 平台可以把一个 Helm Release 作为 DAG 节点，而不是把 Helm 当作唯一编排系统。

---

## 11. 积木式能力模型

### 11.1 Capability

Capability 是最小能力声明：

```text
runtime.kubernetes
runtime.container-engine
artifact.oci-image
artifact.helm
artifact.jar-runtime
network.policy
storage.block
storage.snapshot
database.postgresql
middleware.kafka
security.image-scan
observability.metrics
dr.backup
market.catalog
market.composition
ai.gateway
ai.model-service
ai.model-endpoint
ai.guardrail
ai.evaluation
ai.observability
ai.copilot
ai.aiops
edge.kubeedge
edge.node-group
edge.device-management
edge.offline-autonomy
edge.ota-upgrade
edge.ai-inference
```

每项能力包含：

- 能力 ID 和版本；
- 提供者；
- 输入输出 Schema；
- 支持的 RuntimeTarget；
- 依赖、可选依赖和冲突；
- 安全权限；
- 资源需求；
- 生命周期能力；
- 健康检查；
- 兼容矩阵；
- 认证等级。

### 11.2 CapabilityPack

| 能力包 | 主要内容 | 默认策略 |
|---|---|---|
| Core Pack | 租户、项目、Operation、插件、审计、市场连接 | 必选 |
| Market Pack | 独立应用市场、发布、分类、标签和组合定义 | 独立部署，首期必选 |
| Container Pack | Kubernetes 纳管、Helm、OCI 应用、网络和存储 | 建议默认 |
| Data Pack | PostgreSQL、MySQL、Valkey | 可选 |
| Middleware Pack | Kafka、RabbitMQ、RocketMQ、MQTT | 可选 |
| Security Pack | 镜像扫描、签名验证、准入和运行时安全 | 生产建议 |
| Observability Pack | 指标、日志、链路和告警适配 | 生产建议 |
| Accelerator Pack | GPU、DRA、HAMi | 可选 |
| Multi-Cluster Pack | 多集群、Karmada 和全局入口 | 可选 |
| DR Pack | 备份、复制、演练、切换和回切 | 可选 |
| Integration Pack | IAM、ITSM、CMDB、SIEM 和通知 | 可选 |
| AI Access Pack | AI Gateway、外部模型接入、Token 配额、审计和成本统计 | 可选，最轻量 AI 入口 |
| AI Runtime Pack | ModelService、模型运行时、GPU/NPU、模型端点和弹性 | 可选 |
| AI Governance Pack | Prompt、Guardrail、评测、模型目录和 AI 可观测 | 生产 AI 建议 |
| AIOps Pack | Copilot、异常关联、根因建议、容量预测和受控修复 | 可选，渐进启用 |
| Edge Pack | KubeEdge CloudCore、Edge Provider、节点组、设备管理、断连自治、预拉取和批量 OTA | 可选；V1.5 目标能力 |

### 11.3 ServiceBlueprint

服务蓝图描述平台如何向用户呈现一个应用、数据库或中间件服务。市场中的 Release 可以引用平台支持的 ServiceBlueprint 类型。

```yaml
apiVersion: platform.hnb.io/v1
kind: ServiceBlueprint
metadata:
  name: postgresql-ha
spec:
  serviceType: database.postgresql
  supportedTargets:
    - kubernetes
  requiredCapabilities:
    - runtime.kubernetes
    - secret.management
    - storage.block
  optionalCapabilities:
    - storage.snapshot
    - backup.s3
    - observability.metrics
  lifecycle:
    required: [provision, observe, delete]
    optional: [scale, upgrade, backup, restore, failover]
```

### 11.4 Product、Blueprint 与 Provider 的关系

```text
市场 Product/Release       用户看到“交付什么版本”
        │
        ▼
平台 ServiceBlueprint      平台理解“这是什么服务、有哪些生命周期”
        │
        ▼
Domain Provider            决定“领域上如何部署和管理”
        │
        ▼
Runtime Driver             决定“在 Kubernetes、容器引擎或边缘运行时如何执行”
```

---

## 12. HNB AI 增强架构

### 12.1 总体定位与设计原则

HNB AI 增强架构遵循“核心确定性、AI 可选化、运行标准化、治理前置化”的原则：

- AI 不进入微内核强依赖；
- AI Extension Plane、AI Gateway 和模型运行时全部容器化；
- 支持外部模型、本地模型和混合模型，不绑定单一厂商；
- 平台首先聚焦模型推理和 AI 应用运行，不在 V3.8 内核建设通用训练平台；
- AI 结论不能替代权限、策略、安全准入、配额和审计；
- AI 能力按租户、项目和环境隔离，可独立计量、限流和停用；
- AI 平面故障不得影响传统应用和已运行的数据面。

### 12.2 三层 AI 价值模型

| 层次 | 定位 | 典型能力 |
|---|---|---|
| AI as Workload | 平台承载 AI 工作负载 | LLM、视觉、语音、Embedding、Rerank、RAG、Agent |
| AI as a Service | 平台提供统一 AI 服务 | 模型端点、AI Gateway、向量服务、Guardrail、评测、成本治理 |
| AI for Platform | AI 帮助管理 HNB Cloud | Copilot、告警摘要、异常关联、根因建议、容量预测、受控修复 |

### 12.3 AI Extension Plane 组件

```text
HNB AI Extension Plane
├── AI Gateway
├── Model Service Manager
├── Model Catalog Connector
├── AI Runtime Provider Registry
├── AI Governance Center
│   ├── Prompt Registry
│   ├── Guardrail Policy
│   ├── Evaluation Center
│   └── Model Risk/License Policy
├── AI Observability
├── Platform Copilot
├── AIOps Engine
└── Knowledge/Vector Service Connector
```

各组件可以独立启用。AI Access Pack 只需要 AI Gateway 和外部模型连接器；AI Runtime Pack 才需要 GPU/NPU、模型权重和本地推理运行时；AIOps Pack 可以只使用外部模型和平台现有可观测数据。

### 12.4 AI 资源模型

建议定义以下领域对象：

```text
AIProvider
ModelArtifact
ModelService
ModelEndpoint
AIApplication
PromptTemplate
GuardrailPolicy
KnowledgeBaseRef
EvaluationSuite
EvaluationRun
AIUsageRecord
AIBudgetPolicy
AIOpsInsight
RemediationProposal
```

#### ModelArtifact

描述模型或外部模型引用：

- 模型名称、版本、架构和任务；
- 权重、Tokenizer、推理配置和摘要；
- 来源、发布者、许可证和使用限制；
- 精度、量化方式、上下文长度；
- CPU、内存、GPU/NPU 和显存建议；
- 支持的协议和运行时；
- 安全扫描、评测、红队和发布状态；
- OCI Artifact 引用或受信 `externalRef`。

#### ModelService

`ModelService` 是面向用户的模型服务声明，平台负责资源、调度、网络、生命周期和审计，Runtime Provider 负责具体推理引擎实现。

```yaml
apiVersion: ai.hnb.io/v1
kind: ModelService
metadata:
  name: customer-service-llm
spec:
  tenantRef: tenant-a
  projectRef: customer-service
  modelRef: qwen-model-v1
  runtimeProvider: openai-compatible-runtime
  accelerator:
    type: gpu
    count: 1
    memory: 20Gi
  replicas:
    min: 1
    max: 4
  endpoint:
    protocol: openai-compatible
  autoscaling:
    metric: concurrentRequests
  governance:
    guardrailPolicyRef: production-default
    evaluationPolicyRef: release-gate
```

### 12.5 AI Runtime Provider

统一 Provider 契约建议包含：

```go
type AIModelRuntimeProvider interface {
    Capabilities(ctx context.Context) (*AIRuntimeCapabilities, error)
    ValidateModel(ctx context.Context, model ModelArtifact) error
    Plan(ctx context.Context, service ModelService) (*Plan, error)
    Deploy(ctx context.Context, plan *Plan) (*OperationRef, error)
    Observe(ctx context.Context, ref ResourceRef) (*ModelServiceState, error)
    Scale(ctx context.Context, req ScaleRequest) (*OperationRef, error)
    Upgrade(ctx context.Context, req ModelUpgradeRequest) (*OperationRef, error)
    Evaluate(ctx context.Context, req EvaluationRequest) (*EvaluationRunRef, error)
    Delete(ctx context.Context, ref ResourceRef) (*OperationRef, error)
}
```

Provider 可以适配不同推理引擎、企业模型平台或云模型服务。上层 `ModelService` API、租户配额、审计和用户体验不得因实现替换而变化。

### 12.6 AI Gateway 职责与边界

AI Gateway 是模型调用治理入口，职责包括：

- HTTP、SSE、WebSocket 和 OpenAI-compatible 接入；
- 模型协议适配和版本路由；
- 多模型负载均衡、熔断、重试和 Fallback；
- 租户、项目、用户、Token、并发和速率限流；
- Prompt 和响应安全围栏、敏感数据脱敏和加密；
- 语义缓存和普通响应缓存的租户隔离；
- 调用链、Token、延迟、质量、成本和错误统计；
- 本地模型、企业模型和外部云模型统一路由。

AI Gateway 不负责：

- Kubernetes、GPU 或模型服务资源实际部署；
- 应用人格、业务长期记忆和 Agent 业务决策；
- 企业知识内容的权威管理；
- 平台 RBAC、配额和审批的权威计算；
- 普通业务、数据库、消息和大文件流量转发。

### 12.7 AI 应用市场与组合交付

市场新增 AI 模型、AI 运行时、AI 数据服务、AI 安全、AI 评测、Agent 和 AI 解决方案分类。AI 产品 Release 可以包含：

- 模型或外部模型引用；
- 推理运行时；
- AI Gateway 路由；
- PromptTemplate 和 GuardrailPolicy；
- Embedding、Rerank 和向量数据库；
- Web/API 应用；
- 评测套件和可观测策略。

企业知识助手组合示例：

```text
Enterprise Knowledge Assistant 1.0
├── Web UI
├── AI Application Backend
├── AI Gateway Route
├── ModelService 或 External Model Endpoint
├── Embedding Service
├── Rerank Service
├── Vector Database
├── Document Parser
├── Guardrail Policy
├── Evaluation Suite
└── AI Observability Policy
```

该组合仍以 `CompositionRelease → ExecutionPlan → Operation` 方式部署。Helm、Operator、Model Runtime 和外部 API Connector 都只是 DAG 节点。

### 12.8 Platform Copilot

Platform Copilot 通过自然语言帮助用户查询、解释和生成操作草案：

```text
用户问题
→ 意图识别
→ 权限过滤后的 Read Model/Resource Graph/可观测数据检索
→ 证据归纳与解释
→ 可选 ExecutionPlan 草案
→ 策略、配额、风险和审批检查
→ 用户确认
→ Operation Engine 执行
→ 审计与效果验证
```

典型场景：

- 解释应用部署失败原因；
- 汇总高风险镜像、异常节点和容量趋势；
- 生成 PostgreSQL 服务申请或扩容草案；
- 比较集群健康和资源差异；
- 生成故障时间线、演练报告和变更说明。

Copilot 不得直接调用 kubectl、Helm、数据库管理命令或云厂商 API；所有写操作必须转换为平台支持的结构化计划。

### 12.9 AIOps 分级演进

| 等级 | 能力 | 自动化边界 |
|---|---|---|
| L1 智能摘要 | 告警、日志、事件、变更和日报摘要 | 只读 |
| L2 异常关联 | 告警去重、模式识别、拓扑与变更关联 | 只读，输出证据 |
| L3 根因建议 | 根因候选、置信度、影响面和处置建议 | 生成方案，不自动执行 |
| L4 受控修复 | 执行已审核 Runbook、重试、扩容、无状态恢复 | 通过策略和 Operation 执行 |

低风险动作可以在明确策略下自动执行；删除资源、数据库主从切换、灾备切换、网络策略修改、存储操作和大规模扩缩容必须人工确认或走审批。

### 12.10 AI 可观测

AI 可观测在传统指标、日志和链路基础上增加：

- 首 Token 延迟、总响应延迟和队列等待；
- 输入/输出 Token、吞吐和并发；
- GPU/NPU 利用率、显存、功耗和模型加载时间；
- 缓存命中、Fallback、限流和错误；
- 模型、Prompt、Guardrail 和知识库版本；
- 单租户、项目、应用和模型端点调用量与成本；
- 内容安全拦截、敏感数据命中和评测趋势；
- 质量指标、用户反馈和版本回归。

所有 AI 调用事件必须携带 Tenant ID、Project ID、Model Endpoint、Prompt Version、Trace ID 和策略版本，但不得默认记录完整敏感 Prompt/响应正文。

### 12.11 AI 安全、治理与多租户

AI 治理至少覆盖：

- 模型来源、许可证、出口限制和供应链证明；
- 模型、Prompt、Guardrail、评测和知识引用的版本管理；
- Prompt 注入、越权工具调用、敏感数据泄漏和恶意内容；
- 模型访问、Token、并发、GPU、缓存、日志和成本隔离；
- 高风险模型或外部 Provider 的租户授权；
- 输入输出保留策略和数据脱敏；
- 发布前基准、安全和业务评测；
- 线上质量漂移、成本异常和撤销处置；
- 人工确认、证据展示和责任追踪。

AI Workspace 建议作为租户或项目下的逻辑对象：

```text
Tenant / Project
└── AI Workspace
    ├── ModelEndpoint
    ├── PromptTemplate
    ├── GuardrailPolicy
    ├── KnowledgeBaseRef
    ├── EvaluationSuite
    ├── TokenQuota
    └── CostBudget
```

### 12.12 轻量化部署档位

| 档位 | AI 组件 | GPU/NPU | 适用场景 |
|---|---|---:|---|
| Minimal AI | 外部模型 Connector + 轻量 AI Gateway + Copilot | 不需要 | 单机、演示、小型环境 |
| Lite AI | AI Gateway + 小型本地/CPU 模型 + 按需 AIOps Worker | 可选 | 离线、小规模生产 |
| Standard AI | AI Gateway HA + ModelService + GPU 节点池 + AI OTel | 需要时配置 | 企业生产 |
| Enterprise AI | 多模型、多集群、多地域、统一治理、评测与成本中心 | GPU/NPU 资源池 | 集团和行业平台 |

AI Gateway、扫描、评测和 AIOps Worker 应支持按需扩缩；无本地模型时不得强制安装 GPU Operator、向量数据库和模型存储。

### 12.12.1 边缘 AI 与边云协同

边缘 AI 仍遵循 AI Extension Plane 的统一模型和治理，不单独建立模型目录或端点体系：

- `ModelArtifact` 增加边缘架构、量化、模型大小、内存/显存和离线运行声明；
- 模型权重按中心 Registry → 区域/站点 Mirror → 节点缓存分发，部署前强制预拉取和磁盘水位检查；
- `ModelService` 可声明 `targetType: kubeedge-edge`，由 Edge Provider 转换为边缘工作负载；
- 站点内可选部署轻量 AI Gateway，提供本地 OpenAI-compatible 入口、限流、脱敏和本地路由；
- 小模型/量化模型优先在边缘执行，大模型可回源中心；Fallback 必须声明数据区域、带宽、超时和断连策略；
- 输入输出默认不离开站点，上传样本、日志或特征必须经过租户授权、脱敏和数据区域策略；
- 边缘 AI 可观测增加模型下发进度、冷启动、端侧吞吐、设备功耗、边云 Fallback 次数和最后上报时间；
- Sedna、联邦学习、增量学习和边云协同训练作为 V2+ 市场产品或外部集成，不进入 Edge Pack 核心。

### 12.13 故障隔离与确定性控制

- AI Extension Plane 不可用时，HNB Portal 的传统管理功能、API 和 Operation Engine 继续工作；
- AI Gateway 不可用时，不影响普通应用和已绕过网关直连的非 AI 服务；
- 中心 Model Service Manager 故障时，已运行推理 Pod 和端点继续服务；
- Copilot 无法给出可信结论时应明确返回“不确定”和证据缺口，不得编造平台状态；
- AIOps 自动化必须有熔断、速率限制、并发限制、回滚点和人工停止入口；
- 所有 AI 建议和自动操作必须保留模型、Prompt、证据、策略和执行审计。

---

## 13. 容器运行目标架构

### 13.1 RuntimeTarget 统一模型

`RuntimeTarget` 是平台对可执行环境的统一抽象。它描述“在哪里运行、通过什么通道连接、具备哪些能力、允许哪些 Release、状态新鲜度如何”，不等同于 Kubernetes Cluster。

```yaml
apiVersion: platform.hnb.io/v1
kind: RuntimeTarget
metadata:
  name: edge-factory-east
spec:
  type: kubeedge-edge              # kubernetes | container-engine | kubeedge-edge
  region: cn-east-1
  zone: factory-east
  connection:
    mode: cloudedge-tunnel         # outbound-agent | cloudedge-tunnel | offline-bundle
  capabilities:
    - runtime.kubernetes
    - edge.kubeedge
    - edge.device-management
    - artifact.oci-image
  labels:
    environment: production
    site: factory-east
    arch: arm64
  edge:
    nodeGroupRef: factory-east
    autonomy:
      offlineRun: allow
      offlineSchedule: deny
      heartbeatTimeout: 10m
      maxOfflineDuration: 720h
    bandwidth:
      profile: metered             # broadband | metered | offline
      maxMbps: 20
      transferWindows: ["01:00-06:00"]
    revocation:
      offlinePolicy: run-and-report # run-and-report | restricted-mode | quarantine-on-reconnect
status:
  phase: Ready
  lastKnownStateAt: "2026-07-18T01:00:00Z"
```

核心字段：

- 目标类型、地域、可用区和站点；
- 租户/项目可见范围；
- 连接模式、身份和证书状态；
- 能力、版本和认证等级；
- 容量、配额、网络、存储和加速器；
- 安全区、维护窗口和数据区域；
- 可部署的 Release、Composition 和 Blueprint；
- 目标健康、状态新鲜度和最后成功同步时间；
- 边缘自治、带宽、撤销和离线投递策略。

### 13.2 KubernetesTarget

生产主路径，适用于中心、区域、云和稳定联网的整站轻量集群，支持：

- 原生 Workload、Helm 和 Operator；
- Gateway API、CNI、CSI 和 GPU/NPU；
- 数据库、中间件、备份、可观测和运行时安全；
- k3s、K0s、RKE2 等轻量 Kubernetes；
- Karmada 等集群级多集群扩展。

连接通道使用 HNB `cluster-agent` 的 mTLS 主动连接。将 Kubernetes 部署在边缘机房不自动等同于 EdgeRuntimeTarget；当场景要求长时间断连自治、设备抽象和大规模节点 OTA 时，应选择 EdgeRuntimeTarget。

### 13.3 ContainerEngineTarget

用于开发、演示、单机受限和极轻量环境，底层为 Docker、Podman、containerd 或兼容 OCI Engine：

- 单容器、容器组、Compose/Podman Pod；
- 主机卷、受控目录和基本网络；
- 健康检查、日志指标、滚动替换和本地镜像缓存；
- CPU、内存、磁盘和进程资源限制。

限制：

- 不承诺 Kubernetes 等价的调度、自愈、服务发现和声明式控制；
- 不支持依赖 CRD/Operator 的产品；
- 仅允许市场显式标记兼容的 Release；
- 高可用数据库、复杂中间件和跨节点组合默认不支持；
- 通过 HNB `container-agent` 接入，不与 KubeEdge 通道混用。

### 13.4 EdgeRuntimeTarget

#### 13.4.1 定位与技术基线

EdgeRuntimeTarget 面向节点数量大、资源受限、弱网/断连、设备接入和无人值守场景。V3.8 的参考实现采用 KubeEdge：云端部署 CloudCore，边缘节点运行 EdgeCore，通过 WebSocket/QUIC 控制隧道同步资源和状态，MetaManager 在边缘本地持久化元数据。

Edge Pack 不进入微内核；未安装 Edge Pack 时，KubernetesTarget 和 ContainerEngineTarget 保持完整可用。

#### 13.4.2 边缘资源模型

```text
EdgeNode          # 架构、资源、EdgeCore 版本、证书、最后心跳、自治状态
NodeGroup         # 站点/区域逻辑分组、流量与批量运维边界
EdgeApplication   # 跨节点或节点组批量部署单元、灰度和覆盖参数
DeviceModel       # 设备属性、遥测、命令、协议和采集规则
Device            # 设备实例、Twin 状态、Mapper 绑定和异常状态
EdgeOTAJob        # EdgeCore 升级、配置更新和镜像预拉取的平台投影
```

平台保存业务投影、租户关系、策略和审计；KubeEdge CRD 状态保存在管理集群；边缘本地状态通过隧道汇聚。所有非实时状态必须携带 `lastKnownStateAt`。

#### 13.4.3 云边通道与 HNB Agent 分工

| 场景 | 通道 | 执行者 |
|---|---|---|
| 中心/区域 Kubernetes | HNB cluster-agent（mTLS 主动连接） | Agent + Kubernetes/Helm Driver |
| 单机容器主机 | HNB container-agent | Agent + Container Engine Driver |
| KubeEdge 边缘节点 | CloudHub–EdgeHub 隧道 | Edge Provider → Kubernetes API → KubeEdge |

KubeEdge 边缘节点不再部署 HNB Agent。Edge Provider 将平台 Operation 翻译为 EdgeApplication、Device、NodeUpgradeJob、ImagePrePullJob、ConfigUpdateJob 等资源，避免双份凭据、双通道心跳和双重生命周期管理。

#### 13.4.4 边缘应用交付闭环

```text
选择 targetTypes 含 kubeedge-edge 的 Release/Composition
→ 选择 Tenant/Project/Environment/NodeGroup/节点标签
→ 预检授权、撤销、签名、多架构、资源、WAN、自治和设备权限
→ 生成不可变 ExecutionPlan（分发窗口、灰度批次、回滚和 Hold/Release）
→ 预分发制品并执行 ImagePrePullJob
→ 分批创建 EdgeApplication/工作负载
→ 健康门禁通过后放行下一批
→ 失败暂停、补偿或回滚
→ 状态聚合到 Edge Read Model
→ 兼容结果与完整审计
```

批量部署必须支持并发度、失败容忍、灰度批次、维护窗口、暂停和回滚。目标离线时 Operation 进入 `QueuedOffline`；重连后按顺序和幂等键投递，超过 `maxOfflineDuration` 转为 Failed/NeedsAttention。

#### 13.4.5 断连自治与重连

| 场景 | 边缘行为 | 平台行为 |
|---|---|---|
| 分钟级闪断 | 自动重连，已运行工作负载不受影响 | 记录抖动，不升级为严重告警 |
| 小时至天级断连 | 按本地缓存继续运行、重启容器和采集设备数据 | 标记 Unknown/Offline，展示 lastKnownStateAt，写操作默认排队或拒绝 |
| 节点本地重启 | 依据本地持久化状态恢复工作负载 | 等待重连和状态对账 |
| 重连 | 补传状态/遥测，接收排队任务 | 对账、执行撤销处置、按序投递 QueuedOffline Operation |
| 超过最大离线时长 | 继续运行或进入受限模式，取决于策略 | 标记 Lost，告警/工单/现场处置，必要时吊销证书 |

#### 13.4.6 设备管理

设备 Mapper 作为普通市场产品发布，必须容器化、签名、生成 SBOM 并通过权限审查。平台通过 DeviceModel/Device 投影管理设备，写指令必须经过 RBAC、Policy、Operation 和审计。

首期参考协议：MQTT、Modbus；后续按 Mapper 认证矩阵扩展 OPC UA、CAN、BLE 等。设备高频遥测默认在站点本地处理，只上传聚合、事件或白名单字段。

#### 13.4.7 边缘运维与 OTA

- 节点入网、移除、重置和证书轮换；
- EdgeCore 批量升级：预检、备份、灰度、健康确认和失败回滚；
- ConfigUpdateJob：默认关闭，启用后必须强制维护窗口和站点级串行；
- ImagePrePullJob：在部署窗口前预热镜像/模型；
- 关键业务可使用 Hold/Release，在本地安全状态满足后再应用更新；
- 断连可使用本地诊断命令采集 logs/exec/describe；
- 所有批量任务必须定义并发度、失败容忍、超时、暂停、回滚和人工停止。

### 13.5 ExternalServiceConnector

允许绑定客户已有数据库、中间件、云服务或外部模型端点，但其不属于 HNB Cloud 提供的容器化服务：

- 平台不负责创建；
- 只保存 Secret Reference；
- 支持健康探测、服务绑定、指标和告警；
- 生命周期通常为 observe、bind、unbind；
- 不得误显示平台不可执行的升级、删除或故障切换。

### 13.6 Runtime Driver

```go
type RuntimeDriver interface {
    Discover(ctx context.Context, target TargetRef) (*CapabilitySet, error)
    Validate(ctx context.Context, plan *ExecutionPlan) error
    Execute(ctx context.Context, plan *ExecutionPlan) (*RunRef, error)
    Observe(ctx context.Context, ref ResourceRef) (*RuntimeState, error)
    CollectDiagnostics(ctx context.Context, ref ResourceRef) (*ArtifactRef, error)
    Cancel(ctx context.Context, run RunRef) error
}
```

建议实现：

- Kubernetes Runtime Driver；
- Helm Runtime Driver；
- Container Engine Runtime Driver；
- Edge Runtime Driver / Edge Provider；
- External Service Connector Driver；
- AI Runtime Provider（作为领域 Provider 调用底层 Runtime Driver）。

### 13.7 边缘场景选型矩阵

| 场景 | 推荐形态 | 原因 |
|---|---|---|
| 稳定联网、完整 K8s、无设备接入 | k3s/K0s/RKE2 + KubernetesTarget | 架构简单，复用 cluster-agent |
| 节点多、弱网/断连、设备接入、批量 OTA | KubeEdge + EdgeRuntimeTarget | 云边拆分、元数据本地化、设备抽象和单隧道 |
| 单机资源极小、无 K8s 需求 | Podman + ContainerEngineTarget | 资源最小，接受能力限制 |
| 完全隔离网络 | 签名 Bundle + 本地 Registry + 对应 RuntimeTarget | 摆渡更新，保持制品与策略可验证 |

Karmada 用于集群级多集群编排，KubeEdge 用于节点级边缘纳管；二者可以服务同一企业体系，但不得接管同一节点。

## 14. Provider 与插件契约

### 14.1 Provider 分层

1. **Domain Provider**：应用、数据库、中间件、网络、存储、GPU/NPU、安全、灾备；
2. **Artifact Provider**：OCI、Helm、JAR/WAR Runtime、模型制品、Manifest；
3. **AI Provider**：模型运行时、外部模型、AI Gateway、Guardrail、评测、知识/向量服务；
4. **Edge Provider**：KubeEdge、边缘节点组、设备、边缘 OTA 和边缘运行适配；
5. **Runtime Driver**：Kubernetes、Container Engine 和 Edge Runtime；
6. **Market Connector**：市场目录、Release、授权和事件同步；
7. **Integration Provider**：IAM、ITSM、CMDB、SIEM、通知；
8. **UI Plugin**：可选页面和组件；
9. **Policy Plugin**：校验、准入和治理规则。

### 14.2 通信原则

- 不使用 Go 动态 plugin；
- 进程外 gRPC/HTTP；
- 插件容器化运行；
- 插件不得访问核心数据库；
- 所有访问通过服务账户和公开 API；
- 事件使用版本化契约；
- 独立重启、升级和限流；
- 插件故障不得拖垮 API Core；
- 必须声明超时、重试、幂等和补偿语义；
- 必须提供健康、就绪、指标和诊断接口。

### 14.3 契约治理

每个公开契约必须具备：

- 语义化版本；
- 最低和最高兼容版本；
- 废弃周期；
- 向后兼容规则；
- OpenAPI/Protobuf SDK；
- Conformance Test；
- 示例插件；
- Mock Server；
- 兼容矩阵；
- 破坏性变更 ADR。

### 14.4 认证等级

| 等级 | 要求 |
|---|---|
| Experimental | 基础功能可用，不承诺升级和 SLA |
| Compatible | 通过 API、权限、安装和卸载测试 |
| Production Ready | 通过性能、故障、升级、备份、安全和诊断测试 |
| Certified | 联合兼容认证、长期支持和明确责任边界 |

市场和平台都应展示认证等级，不能把所有第三方包默认视为生产可用。

---

## 15. 平台、市场与 AI 平面部署方式

### 15.1 部署形态

| 形态 | 底层基础设施 | 运行/编排方式 | 适用场景 |
|---|---|---|---|
| 单机轻量版 | 单台物理机或 VM | Podman/Docker Compose | 开发、演示、功能验证，非 HA |
| Kubernetes 标准版 | 物理机或 VM 集群 | Helm/Operator | 企业生产默认 |
| 独立管理集群版 | 专用管理节点 | Kubernetes | 管理多个业务集群/边缘域 |
| 多地域版 | 多站点物理机/VM/云主机 | 多 Kubernetes 集群 | 集团和容灾 |
| 整站轻量边缘版 | 边缘站点服务器 | k3s/K0s/RKE2 + cluster-agent | 稳定联网、需完整 K8s |
| 云边自治版 | 中心 CloudCore + 边缘 EdgeCore | KubeEdge + EdgeRuntimeTarget | 弱网、断连、设备和批量 OTA |
| 单机受限边缘版 | 边缘主机 | Podman + ContainerEngineTarget | 极低资源、无 K8s 需求 |
| 离线版 | 隔离网络 | 本地 Registry + 签名 Bundle | 高安全或完全离线网络 |

无论底层是物理机还是虚拟机，平台、市场、AI Extension Plane 和 CloudCore 组件始终以容器方式运行；EdgeCore 是第 3.5.5 节定义的受控节点代理例外。AI 和 Edge 能力可以附加到适合的部署形态，但不得改变该档位的基础 HA 定义。

### 15.2 独立部署要求

HNB App Market、HNB Cloud Platform 与 HNB AI Extension Plane：

- 使用独立 Namespace 或独立集群；
- 使用独立数据库 Schema，生产推荐独立数据库实例；
- 支持独立扩缩容、升级、停用和故障隔离；
- 使用不同 ServiceAccount 和网络策略；
- 通过 API Gateway 或服务网关连接；
- 不共享运行时 Secret；
- 可共享企业 IAM、OCI Registry、对象存储和可观测平台；
- AI Extension Plane 可集中服务多个平台，也可按平台或地域独立部署；
- AI Gateway API Key、外部模型凭据和模型运行 Secret 由平台 Secret 引用或企业密钥系统托管，不进入市场；
- 必须支持单独备份和恢复。

### 15.3 统一安装器

提供 `hnbctl`：

```text
hnbctl preflight
hnbctl install platform --profile minimal
hnbctl install platform --profile standard-ha
hnbctl install market --profile standard
hnbctl install artifact-storage --profile lite-ha
hnbctl artifact-storage connect --profile enterprise-s3
hnbctl artifact-storage health
hnbctl artifact-storage gc plan
hnbctl connect market --endpoint https://market.example.com
hnbctl pack enable container
hnbctl pack enable data
hnbctl pack enable ai-access
hnbctl pack enable ai-runtime
hnbctl pack enable ai-governance
hnbctl pack enable aiops
hnbctl pack enable edge
hnbctl edge node list --group factory-east
hnbctl edge node join --batch nodes.yaml
hnbctl edge app deploy --release edge-collector-2.1.0 --group factory-east
hnbctl edge prepull --release edge-collector-2.1.0 --group factory-east
hnbctl edge node upgrade --group factory-east --to v1.23.0 --batch 5%:25%:100%
hnbctl edge node config-update --group factory-east --file edgecore-patch.yaml
hnbctl edge device list --node edge-01
hnbctl edge offline-bundle export --site site-c
hnbctl edge health --group factory-east
hnbctl ai health
hnbctl ai providers list
hnbctl market sync
hnbctl upgrade plan
hnbctl upgrade apply
hnbctl backup platform
hnbctl backup market
hnbctl backup artifact-storage
hnbctl backup ai-extension
hnbctl restore artifact-storage --plan <id>
hnbctl diagnostics collect
```

安装器本身可以作为短生命周期容器运行，也可以发布跨平台静态 CLI；被安装的服务必须是容器。

### 15.4 Release Bundle

```text
hnb-release-bundle/
├── manifest.lock
├── oci-layout/
│   ├── blobs/
│   ├── index.json
│   └── oci-layout
├── product-releases/
├── model-releases/
├── composition-releases/
├── prompts/
├── guardrails/
├── evaluations/
├── sbom/
├── signatures/
├── vulnerability-db/
├── revocations/
├── edge-node-profiles/
├── device-models/
├── migrations/
├── compatibility/
└── docs/
```

所有镜像、Chart、JAR/WAR、模型、Prompt、Guardrail、评测包、Provider 和配置包使用 digest 锁定。离线 Bundle 优先采用 OCI Image Layout，导入时必须验证 Bundle 签名、完整性、版本、许可证和 ArtifactDescriptor。

### 15.5 最小依赖与制品存储档位

| 依赖 | Minimal/Dev | Lite HA | Standard/Enterprise |
|---|---|---|---|
| PostgreSQL | 容器单实例 | 3 实例容器化 HA 或外部 HA PG | 企业 HA PostgreSQL/PITR |
| Valkey | 默认不启用 | 按功能需要启用 | 容器化 HA 或企业外部实例 |
| OCI Registry | 轻量单副本 | 2 个以上无状态副本 | Harbor/企业 Registry 多副本 |
| Artifact Data Plane | 本地独立卷/PVC | 优先外部 S3；可选 3 节点轻量 S3 Provider | 企业 S3、Ceph RGW 或云对象存储 |
| 独立对象存储组件 | 不强制安装 | 仅在无外部 S3 时安装 | 优先复用企业能力 |
| 搜索 | 数据库全文检索 | 数据库全文检索或可选独立搜索 | OpenSearch/Elasticsearch 可选 |
| 消息系统 | PostgreSQL Outbox | PostgreSQL Outbox/NATS 可选 | NATS/Kafka 可选 |
| IAM | 初始本地管理员 | 企业 OIDC/LDAP | 企业 IAM/统一身份 |
| 可观测后端 | 轻量容器组件 | 统一平台或轻量后端 | 企业现有平台或专用集群 |
| AI Gateway | 不安装或单实例外部模型接入 | 2 副本或按需启用 | 多副本、区域入口 |
| 本地模型运行时 | 默认不安装 | 可选 CPU/单 GPU 模型 | GPU/NPU 资源池和多运行时 |
| 向量数据库/知识服务 | 默认不安装 | 按应用安装 | 独立 HA 服务或企业已有能力 |
| AIOps Worker | 按需短任务 | 可缩容到零 | 独立任务队列和评测环境 |

核心不能因为缺少大型消息队列、搜索集群、MinIO、Ceph 或 Service Mesh 而无法启动。单机本地存储只用于非 HA；生产 HA 必须使用共享且经过认证的制品数据后端。

---

## 16. 应用、数据与 AI 服务交付闭环

### 16.1 统一对象

```text
MarketProduct / ModelProduct / AISolution
→ ProductRelease / ModelRelease / CompositionRelease
→ ServiceBlueprint / ApplicationBlueprint / AIServiceBlueprint
→ DeploymentRequest / ModelServiceRequest
→ ExecutionPlan
→ ApplicationInstance / ServiceInstance / ModelService
→ Endpoint / ModelEndpoint / GatewayRoute
→ ServiceBinding / AIServiceBinding
→ Operation
```

### 16.2 用户流程

```text
进入统一应用市场或 AI Workspace
→ 按应用/数据库/中间件/模型/AI解决方案分类和标签检索
→ 选择 Product、Model 或 Composition Release
→ 平台检查授权、许可证和撤销状态
→ 选择租户、项目、环境和运行目标
→ 动态生成业务、资源、模型、Prompt 和安全参数表单
→ 解析组合依赖
→ 预检配额、网络、存储、GPU/NPU、制品、Provider、评测和安全策略
→ 审批或自动批准
→ 固化 ExecutionPlan
→ 执行应用、数据和 AI 节点 DAG
→ 健康与评测验证
→ 创建 Endpoint、ModelEndpoint、GatewayRoute、Secret Reference 和服务绑定
→ 进入监控、成本、备份、升级、评测和审计
```

### 16.3 生命周期能力协商

| 能力 | 市场/Provider 声明 | 平台与 AI 平面行为 |
|---|---|---|
| 安装/部署 | Release 是否可安装，运行时是否支持 | 生成并执行计划 |
| 扩缩容 | Provider 是否支持，模型副本和资源边界 | 动态展示规格、成本和预检 |
| 升级 | 允许版本边、迁移要求和评测门禁 | 生成升级计划、灰度策略和回滚点 |
| 回滚 | 应用、Chart、模型和 Prompt 是否可回退 | 执行回滚，数据回滚单独处理 |
| 备份 | 是否有 Backup Provider | 创建数据、配置和模型服务恢复策略 |
| 恢复 | 支持的恢复方式 | 预检、恢复和验证 |
| 评测 | Evaluation Provider 和阈值 | 执行离线/在线评测并决定发布或升级门禁 |
| 路由 | Gateway Provider 和协议能力 | 创建路由、流量权重、Fallback 和限流 |
| 预算 | Token、GPU 和外部 Provider 计量能力 | 设置预算、告警、限流和超额处置 |
| 故障切换 | 是否支持 | 受控执行、隔离和审计 |
| 删除 | 卸载规则和数据/模型缓存策略 | 保护数据、引用检查、确认和回收 |

### 16.4 实例与市场版本关系

每个应用、服务或模型实例必须记录：

- Product/Model/Composition ID；
- Release ID 和 Channel；
- 完整 ReleaseManifest；
- ExecutionPlan；
- 镜像、Chart、模型、Prompt、Guardrail、评测和配置摘要；
- RuntimeTarget、Provider 和 Provider 版本；
- ModelEndpoint、Gateway Route 和外部模型 Provider；
- Tenant、Project、Environment；
- 参数覆盖和 Secret Reference；
- 评测结果、策略版本和发布门禁；
- Token/GPU/成本预算策略；
- 创建、升级、回滚和删除 Operation；
- 当前漂移状态和最后一次验证结果。

禁止只记录镜像 tag、Chart version、模型名称或 Prompt 文本而无法复现完整部署和调用行为。

---

### 16.5 边缘应用与设备交付闭环

边缘交付复用 16.1–16.4 的统一对象，不另建旁路：

```text
Product/Release/CompositionRelease
→ ServiceBlueprint（supportedTargets 包含 kubeedge-edge）
→ RuntimeTarget/NodeGroup 选择
→ ExecutionPlan（预分发、灰度、自治、带宽、回滚、维护窗口）
→ Operation（含 QueuedOffline）
→ EdgeApplication/Device/EdgeOTAJob
→ Instance/ServiceBinding/Edge Read Model
```

用户界面默认以“站点、节点组、边缘应用、设备和任务”展示，不要求普通用户理解 KubeEdge CRD。专家模式可查看原始资源、隧道状态、边缘缓存、最后心跳和对账差异。

## 17. 简单易用设计

### 17.1 统一入口

用户在 HNB Portal 中访问应用市场，前端可以采用微前端或独立路由集成，但后端仍保持独立系统：

```text
HNB Portal
├── 应用市场
├── 我的申请
├── 应用实例
├── 数据库服务
├── 中间件服务
├── AI 工作空间
├── 模型服务与端点
├── Platform Copilot
├── 运行环境
├── 运维中心
└── 安全与审计
```

推荐通过统一身份和导航实现“一个入口”，不要通过 iframe 嵌入完整市场控制台作为长期方案。

### 17.2 三层模式

#### 普通模式

只展示：

- 产品说明；
- 稳定版本；
- 推荐套餐；
- 环境；
- 域名；
- 资源规格；
- 备份开关；
- 费用或配额估算。

#### 运维模式

增加：

- 副本；
- 资源限制；
- 网络和存储；
- 高可用；
- 维护窗口；
- 告警；
- 备份和恢复。

#### 专家模式

增加：

- Helm values；
- 调度策略；
- NetworkPolicy；
- StorageClass；
- Gateway API；
- 高级 JVM 参数；
- Provider 参数；
- 组合依赖覆盖；
- 模型运行时、量化、GPU/NPU、Prompt、Guardrail 和评测参数。

### 17.3 动态表单

市场提供产品参数 Schema，平台注入环境上下文：

- 市场负责业务和产品参数；
- 平台负责集群、网络、存储、配额、安全和 Secret 参数；
- 重名参数通过命名空间区分；
- Secret 字段不得回传市场；
- 参数默认值按市场默认、平台策略、租户策略、环境策略依次覆盖；
- 提交前展示最终差异和影响预览。

### 17.4 场景化向导

- 发布 Java 应用；
- 部署标准 Web 三层应用；
- 创建 PostgreSQL 高可用实例；
- 创建 Kafka 集群；
- 从市场安装 API 网关；
- 部署带数据库、缓存和网关的组合应用；
- 导入离线市场包；
- 将应用从测试渠道升级到稳定渠道；
- 接入外部模型并创建 AI Gateway 路由；
- 部署本地 ModelService；
- 部署企业知识助手组合；
- 使用 Copilot 诊断一次失败 Operation；
- 批量纳管边缘节点并建立节点组；
- 将边缘应用灰度发布到节点组；
- 接入 MQTT/Modbus 设备并部署 Mapper；
- 下发边缘模型、预拉取制品并执行灰度升级。

---

## 18. 安全与软件供应链

### 18.1 双重安全门禁

#### 市场发布门禁

- 来源和发布者认证；
- 许可证检查；
- 文件类型和恶意文件扫描；
- 镜像漏洞扫描；
- SBOM；
- 签名；
- 构建证明；
- 敏感信息检测；
- Chart/YAML 配置安全；
- 安装权限预览；
- 兼容性和生命周期测试。

#### 平台部署门禁

- 租户授权；
- 配额；
- 目标能力；
- 镜像和 Release 撤销状态；
- 签名再验证；
- 准入策略；
- NetworkPolicy；
- Pod Security；
- Secret 引用；
- 特权、HostPath、HostNetwork 和设备权限；
- 生产环境风险审批。

### 18.2 信任模型

- 发布者使用 Sigstore Cosign 或企业 KMS 密钥签名制品和 Release；
- 市场验证发布者身份、签名、SBOM 和构建证明，并签名已审核 ReleaseManifest；
- 签名、SBOM、漏洞报告和 Provenance 通过 OCI Referrer 与主制品 digest 关联；
- 构建证明逐步对齐 SLSA Build Track，首期要求可追溯来源，生产目标达到受控构建与不可篡改 Provenance；
- 平台验证发布者签名、市场签名、撤销状态和信任根；
- 平台生成并签名 ExecutionPlan；
- HNB Agent 或 Edge Provider 只执行来自受信平台、通过策略和审批的计划；
- 执行结果、证据、最终资源摘要和操作者进入审计；
- 高安全环境要求 Registry、市场、平台、AI 平面和 CloudCore 证书链可独立轮换；
- 信任策略由 Provider 实现但由平台 Policy Registry 统一引用，避免各组件自建互相冲突的根证书和白名单。

### 18.3 JAR/WAR 安全

- 扫描恶意代码和已知依赖漏洞；
- 生成或接收 CycloneDX/SPDX SBOM；
- 检测内嵌密码、私钥和 Token；
- 禁止上传可变外部依赖；
- 固定 Java Runtime 镜像 digest；
- 默认非 root；
- 只读根文件系统；
- 最小 Linux Capability；
- 禁止运行时在线下载未锁定依赖；
- 生产环境推荐转换为不可变 OCI 镜像。

### 18.4 多租户隔离

- 市场内容可见性和平台资源权限分别计算；
- 拥有市场查看权限不代表拥有部署权限；
- Entitlement、租户角色、项目角色和平台策略共同决定部署；
- 市场不接收租户运行 Secret；
- 平台审计记录 Product/Release 和制品摘要；
- 跨租户共享实例需显式授权和期限控制。

### 18.5 撤销处置

当 Release 或制品被撤销：

1. 市场发布在线撤销事件；
2. 生成并签名撤销 OCI Artifact，固定序列号、签发时间、有效期和被撤销 digest；
3. Artifact Storage 将撤销 Artifact 分发到区域和边缘站点；
4. 平台停止新部署、新扩容和新预拉取；
5. 扫描已有实例、回滚点、离线 Bundle、模型端点和边缘缓存；
6. 按风险等级生成告警、隔离、升级、流量切换或停用 Operation；
7. 断连边缘按 `offlineRevocationPolicy` 继续运行、进入受限模式或等待重连；
8. 重连后必须完成撤销处置并保留完整审计证据；
9. 不得未经策略授权自动删除有状态实例或现场关键控制负载。

### 18.6 AI 安全与模型供应链

- 模型、Tokenizer、Prompt、Guardrail 和评测包必须记录来源、摘要、许可证和发布者；
- 生产 ModelService 必须通过模型质量、安全、资源和许可证门禁；
- 外部模型 Provider 必须配置数据出境、日志保留、敏感数据和可用区域策略；
- Prompt 和响应日志默认脱敏，完整正文采集需租户显式授权；
- AI Gateway 缓存键必须包含 Tenant ID、模型、Prompt 和策略版本，禁止跨租户缓存污染；
- Agent 工具调用采用显式白名单、参数 Schema、最小权限和人工确认策略；
- Copilot 与 AIOps 不得读取超出用户权限范围的日志、Secret、Prompt 或业务数据；
- 模型、Prompt、知识库和 Guardrail 撤销时必须分析受影响端点、AI 应用和历史评测；
- 模型输出不能替代签名校验、准入、RBAC、配额和确定性安全规则。

---

### 18.7 Secret、KMS 与凭据分层

- 市场不保存运行 Secret，只保存参数 Schema 和 Secret 字段声明；
- HNB Core 只保存 `SecretReference`、用途、租户和轮换状态，不保存不必要的明文副本；
- 后端支持 Kubernetes Secret、企业 Vault/KMS/HSM 和云密钥服务 Provider；
- Agent、Provider 和 AI Gateway 通过短期凭据或工作负载身份获取 Secret；
- 边缘节点的 Secret 必须本地加密存储，密钥与节点身份/证书绑定；
- 节点丢失或证书吊销后，相关短期凭据失效，重新入网必须重新授权；
- Secret 轮换必须具备双版本过渡、失败回退和审计，禁止通过日志、环境导出和诊断包泄漏。

### 18.8 边缘安全

1. **入网身份**：一次性 Token、节点证书、站点绑定和证书指纹进入平台审计；
2. **隧道安全**：CloudHub–EdgeHub 使用 mTLS，证书支持提前轮换和远程吊销；
3. **物理不可信**：最小化本地敏感数据，可选全盘/数据分区加密，节点丢失后吊销身份；
4. **工作负载准入**：Pod Security、镜像签名、只读根文件系统和最小权限默认生效；设备访问容器使用白名单豁免并审计；
5. **设备写操作**：必须经用户权限、设备策略、Operation 和命令审计；
6. **离线安全**：撤销列表、信任根、策略和 Bundle 均需签名，可在无云连接时验证；
7. **时间治理**：入网预检 NTP/PTP 偏差，超阈值拒绝或限制；审计同时记录边缘时间与云端接收时间；
8. **更新安全**：关键业务支持 Hold/Release，仅在本地安全状态满足后应用更新。

## 19. 可观测、运维与灾备

### 19.1 平台与市场可观测

统一采用 OpenTelemetry，但存储后端可复用企业系统：

- API 延迟和错误；
- Market 同步游标和失败；
- 制品上传、下载、镜像拉取和对象存储重定向；
- Registry API、Manifest、Blob 和 Referrer 延迟；
- Artifact Data Plane 容量、吞吐、错误和故障域；
- 复制队列、缓存命中、预拉取和广域网流量；
- Blob 完整性、孤儿对象、可回收空间和 GC；
- 备份、PITR、恢复演练和 RPO 偏差；
- Release 解析耗时；
- Operation 队列、重试和失败；
- Agent 连接；
- Provider 健康；
- 部署步骤耗时；
- 市场索引、扫描和镜像同步任务；
- 审计和安全事件。

- EdgeNode 心跳、隧道连接、断连时长和重连风暴；
- EdgeApplication 灰度批次、失败容忍和回滚；
- 边缘 Registry/节点缓存命中率、预拉取进度和 WAN 用量；
- 边缘指标/日志本地缓冲水位、丢弃量和重连补传进度；
- 设备 Mapper、DeviceTwin、命令延迟和异常事件；
- EdgeCore/CloudCore 版本偏斜、证书有效期和 OTA 任务。

### 19.2 Operation 状态

正常状态链：

```text
Pending
→ Validating
→ WaitingApproval
→ ResolvingRelease
→ Planning
→ WaitingDependency
→ QueuedOffline（仅离线目标，可选）
→ Running
→ Verifying
→ Succeeded
```

异常与控制状态：

```text
Retrying
Compensating
Paused
Cancelled
Failed
NeedsAttention
```

状态规则：

- `Succeeded`、`Failed`、`Cancelled` 为终态；`NeedsAttention` 是等待人工决策的非终态；
- `QueuedOffline` 仅表示目标不可达且允许排队，不表示执行成功；
- 每个非终态必须配置最大滞留时间、告警等级、自动升级和人工接管策略；
- 重连后按 Tenant、Target 和幂等键顺序投递，过期计划必须重新校验授权、撤销、配额和兼容性；
- 状态转换必须持久化，Worker/Provider 重启后可恢复，不依赖内存队列。

每个 Operation 必须记录：

- Tenant、Project、Environment 和 RuntimeTarget；
- Product/Release/Composition ID 与制品摘要；
- ExecutionPlan、步骤、输入、输出和证据；
- 重试、补偿、灰度批次、排队原因和超时策略；
- 操作者、审批、维护窗口和人工接管；
- 审计关联、市场回调和最终资源引用；
- 状态新鲜度和目标最后在线时间。

### 19.3 制品存储运维

制品存储作为独立运维域，必须提供：

- `StorageHealthSnapshot` 和统一健康评分；
- 容量趋势、剩余天数和扩容建议；
- Registry、S3、PostgreSQL 和入口分层故障定位；
- 随机 Blob digest 校验和全量/增量 scrub；
- 复制失败重试、限速和断点恢复；
- 缓存命中率和热点制品分析；
- GC 预览、引用证明、执行报告和审计；
- 配置、密钥、数据库和制品数据的统一备份计划；
- Registry 重建、数据库 PITR、Bucket 恢复和整站恢复演练；
- 无中断滚动升级和一键诊断包。

制品数据不可用时应禁止新的发布和部署，但不能影响已运行容器；仅区域缓存故障时应自动回退到上一级可信源。

### 19.4 升级治理

升级由市场和平台协同：

- 市场定义 `fromRelease -> toRelease` 合法升级边；
- 市场提供变更说明、迁移要求、备份要求和不兼容项；
- 平台检查运行环境、数据状态、维护窗口和配额；
- 平台生成升级计划和回滚点；
- 平台执行并验证；
- 结果回传市场形成兼容性数据。

### 19.5 灾备边界

- 市场灾备：元数据数据库、Artifact Data Plane、Registry 配置和签名材料；
- Registry Access Plane 本身可重建，不作为权威数据备份对象；
- 区域镜像和节点缓存可重建，不作为中心权威备份对象；
- 平台灾备：控制面元数据、审计、Secret 引用和 Operation 状态；
- 应用灾备：工作负载、数据、流量和依赖；
- 市场不可用不代表应用不可用；
- 仅复制 Helm 和镜像不能宣称业务双活；
- 多站点必须分别规划 Registry 镜像、对象存储复制和市场元数据恢复。

---

### 19.6 AI 服务运维

AI 运维中心统一展示：

- ModelService、ModelEndpoint 和 AI Gateway 健康；
- 模型加载、冷启动、队列、Token 吞吐和 GPU/NPU 状态；
- 单租户用量、预算、成本和限流；
- Prompt、Guardrail 和模型版本漂移；
- 评测结果、质量回归和安全拦截；
- 外部模型 Provider SLA、错误和 Fallback；
- 模型制品缓存、预拉取和磁盘水位。

模型升级必须先完成兼容性和评测门禁，支持灰度、流量分配、快速回退和旧版本保留。

### 19.7 AIOps 运行治理

- AIOps 输入以结构化指标、日志索引、事件、资源图谱和变更记录为主，避免把全量原始数据直接发送给模型；
- 每个 Insight 必须包含证据、时间范围、影响对象、置信度和不确定性；
- RemediationProposal 必须映射到已注册 Runbook 或平台结构化 API；
- 自动修复配置独立策略、风险级别、最大影响范围、冷却时间和停止开关；
- 修复后必须验证效果，无改善时自动停止后续动作并转人工；
- AI 模型、Prompt 和规则版本必须进入审计，以支持复盘和责任追踪。

---

### 19.8 边缘可观测与断连补传

- 边缘指标和日志采用本地环形缓冲，按容量水位、优先级和保留时间丢弃；
- 重连后分批补传，避免与控制消息、撤销和 OTA 任务争抢隧道；
- 设备原始高频数据默认不通过控制隧道上传，应走业务数据通道或本地聚合；
- Edge Read Model 对每个对象记录 `lastKnownStateAt`、`sourceTimestamp` 和 `cloudReceivedAt`；
- Portal、API、Copilot 和 AIOps 回答边缘状态时必须显式区分实时、最后已知和推断；
- 单 CloudCore 副本故障、批量重连、72 小时断连后状态收敛和遥测补传必须纳入演练。

## 20. 工程架构与代码仓库

```text
hnb-cloud/
├── platform/
│   ├── cmd/
│   │   ├── platform-api/
│   │   ├── platform-controller/
│   │   ├── platform-worker/
│   │   ├── agent-gateway/
│   │   ├── cluster-agent/
│   │   ├── container-agent/
│   │   └── hnbctl/
│   ├── internal/
│   │   ├── kernel/
│   │   ├── identity/
│   │   ├── tenant/
│   │   ├── runtime/
│   │   ├── resource/
│   │   ├── operation/
│   │   ├── executionplan/
│   │   ├── composition/
│   │   ├── plugin/
│   │   ├── marketconnector/
│   │   ├── policy/
│   │   ├── graph/
│   │   └── audit/
│   └── web/
├── market/
│   ├── cmd/
│   │   ├── market-api/
│   │   ├── market-worker/
│   │   └── market-console/
│   ├── internal/
│   │   ├── catalog/
│   │   ├── product/
│   │   ├── artifact/
│   │   ├── release/
│   │   ├── composition/
│   │   ├── entitlement/
│   │   ├── review/
│   │   ├── security/
│   │   ├── mirror/
│   │   └── bundle/
│   └── web/
├── ai/
│   ├── gateway/
│   ├── model-service-manager/
│   ├── governance/
│   ├── evaluation/
│   ├── observability/
│   ├── copilot/
│   └── aiops/
├── providers/
│   ├── artifact-oci/
│   ├── artifact-helm/
│   ├── artifact-runtime-java/
│   ├── database-postgresql/
│   ├── middleware-kafka/
│   ├── ai-runtime/
│   ├── ai-external-model/
│   ├── ai-guardrail/
│   ├── ai-evaluation/
│   ├── runtime-kubernetes/
│   └── runtime-container-engine/
├── api/
│   ├── platform-openapi/
│   ├── market-openapi/
│   ├── protobuf/
│   └── events/
├── sdk/
├── conformance/
├── deploy/
│   ├── charts/
│   ├── compose/
│   └── offline/
└── docs/
```

### 20.0.1 Edge Pack 工程边界

建议新增独立目录和发布单元：

```text
hnb-cloud/
├── edge/
│   ├── edge-provider/
│   ├── edge-readmodel-projector/
│   ├── edge-operation-controller/
│   ├── edge-onboarding/
│   ├── edge-ota-controller/
│   ├── device-adapter/
│   └── conformance/
└── deploy/
    └── edge-pack/
        ├── cloudcore/
        ├── controller-manager/
        ├── task-manager/
        └── values/
```

Edge Pack 使用独立模块、镜像、Chart、数据库表/Schema 与 Provider 契约；HNB Core 只依赖公开接口和资源投影，不导入 KubeEdge 内部代码。

### 20.1 技术原则

- 平台 API、市场 API、Controller 和 Agent 优先 Go；
- 前端 TypeScript；
- 热点传输和大文件处理可按压测使用 Rust；
- 全部组件构建为多架构 OCI 镜像；
- 数据库迁移版本化；
- API、事件和 Manifest 采用 Schema First；
- 市场与平台禁止共享内部 Go struct 作为外部契约；
- 使用生成 SDK 保持解耦；
- 所有 Provider 通过契约测试；
- AI Gateway、模型运行时和 AIOps 使用独立 API/事件契约，不直接依赖平台内部数据库；
- Prompt、评测和 AI 策略采用 Schema First 和不可变版本。

### 20.2 架构治理

需要维护：

- ADR；
- 核心禁止依赖清单；
- API 兼容策略；
- ReleaseManifest 规范；
- CompositionRelease 规范；
- ExecutionPlan 规范；
- Event Catalog；
- Provider Conformance；
- 镜像和 Chart 版本矩阵；
- 支持生命周期；
- 数据迁移与回滚策略；
- AI Resource、Prompt、Guardrail、Evaluation 和 AIOps Event Catalog；
- AI Provider Conformance 与模型兼容矩阵。

---

## 21. 版本范围与实施路线

### 21.1 阶段 0：架构基线与验证

目标：验证容器化硬约束、市场/平台边界、AI Extension Plane 解耦原则，并完成 Edge Pack 的关键风险概念验证。

必须完成：

- ReleaseManifest 原型；
- CompositionRelease 原型；
- ExecutionPlan 原型；
- 市场与平台独立数据库和 API；
- OCI Image、Helm OCI、Generic OCI Artifact 和 Referrer 验证；
- 单机本地卷与 Lite HA 共享 S3 两种 ArtifactStorageProfile 验证；
- Registry 多副本、S3 直取、短期 Token 和大文件断点续传验证；
- 内容去重、复制、缓存、完整性校验和 GC 验证；
- Helm OCI 验证；
- JAR/WAR 标准运行时容器验证；
- Agent mTLS；
- 依赖 DAG 和补偿验证；
- OCI Image Layout 离线 Bundle 导入导出验证；
- 外部模型 Connector、AI Gateway 基础路由和租户隔离验证；
- ModelArtifact/ModelService 原型和 GPU 能力发现验证；
- Copilot 只读查询与 Operation 草案生成验证；
- KubeEdge 单节点入网、断网 24 小时自治、重连对账验证；
- EdgeApplication 灰度部署和 arm64 制品预分发原型；
- 签名撤销 Artifact 离线验证和重连处置原型。

退出标准：

- 一个 JAR 应用能从市场发布并以容器方式部署；
- 一个 Helm 数据库包能发布并部署；
- 一个三组件组合能够按依赖部署并生成绑定；
- 市场无业务集群凭据；
- 所有执行使用 digest 锁定；
- 单个 Registry Pod 和单个管理节点故障不导致制品数据丢失；
- 市场和平台 API 不转发大文件正文；
- 关闭 AI Extension Plane 后平台核心闭环和传统应用运行不受影响；
- Copilot 无法绕过 RBAC、策略和 Operation 执行写操作；
- 断连自治与边缘灰度发布 POC 通过，但 Edge Pack 不进入 MVP 正式范围。

### 21.2 MVP：市场与平台最小闭环

#### 市场

- Product、Category、Tag；
- OCI 镜像、Helm、JAR/WAR 入库；
- Release 和 stable/test 渠道；
- 基础审核、签名、SBOM 和扫描结果；
- CompositionRelease；
- Entitlement；
- Catalog API 和事件。

#### 平台

- 租户、项目、环境；
- KubernetesTarget；
- RuntimeTarget/ReleaseManifest Schema 预留 `kubeedge-edge`、`targetTypes`、自治和带宽字段，但 MVP 不启用 Edge Pack；
- 市场连接和缓存；
- Operation、ExecutionPlan；
- Helm、OCI Image、JAR Runtime 三类 Provider；
- 基础网络、存储和 Secret；
- 部署、观察、删除；
- 审计和基础监控。

#### 示例产品

- Java Web 应用；
- PostgreSQL；
- Valkey 或 RabbitMQ；
- Java Web + PostgreSQL 的组合应用；
- 外部模型接入示例和只读 Platform Copilot。

### 21.3 V1：生产可用

- 高可用平台、市场和 Artifact Storage 部署；
- Lite HA 制品存储、企业 S3 接入和 Provider 认证；
- Registry/对象存储/PostgreSQL 备份恢复与故障演练；
- 企业 OIDC/LDAP；
- 完整发布审核和渠道晋级；
- 多租户授权；
- 镜像、Chart 和通用 OCI Artifact 区域同步与缓存；
- Provider 认证；
- 升级和回滚；
- 数据备份和恢复；
- 应用运行时安全；
- 离线部署；
- PostgreSQL + 一个消息中间件完整生命周期；
- 生产容量、性能和故障测试；
- AI Access Pack：AI Gateway、外部模型、Token 配额、审计和成本统计；
- Copilot 告警/事件摘要、诊断建议和受控计划草案；
- AI 安全基础门禁和租户隔离测试。

### 21.4 V1.5：生态扩展

- ContainerEngineTarget；
- 更多数据库和中间件；
- Operator Bundle；
- 上游市场同步；
- ISV 发布者门户；
- 许可证和订阅；
- 多架构镜像；
- GPU 能力包；
- 多集群部署；
- 应用组合升级；
- AI Runtime Pack：ModelService、ModelEndpoint、本地模型和 GPU/NPU 调度；
- AI Governance Pack：Prompt、Guardrail、评测和 AI 可观测；
- RAG/知识助手组合解决方案；
- **Edge Pack GA**：Edge Provider、CloudCore、EdgeNode/NodeGroup、EdgeApplication、设备管理、ImagePrePullJob、NodeUpgradeJob、边缘场景向导和 `hnbctl edge`；
- MQTT 与 Modbus 两个 Mapper 参考产品；
- 边缘断连、灰度、OTA、撤销和离线 Bundle 认证。

### 21.5 V2：集团与高级能力

- 集团市场服务多个平台实例；
- 多地域市场镜像和容灾；
- Karmada；
- 全局流量；
- 高级灾备；
- 兼容性遥测和智能推荐；
- 行业解决方案市场；
- Partner/ISV 认证体系；
- 计量和商业授权；
- AIOps L2/L3：异常关联、根因建议和容量预测；
- 经审批的低风险自动修复与 Runbook；
- 多地域模型端点、成本治理和模型质量运营；
- 多地域 CloudCore 分域、边缘 AI ModelService、边缘 AI Gateway 轻量实例；
- 设备异常检测、更多 Mapper 生态和 Edge Enterprise 认证；
- Sedna/联邦学习/增量学习等边云协同作为外部集成或市场产品。

---

## 22. 团队与组织建议

### 22.1 最小核心团队

| 角色 | 建议人数 | 主要职责 |
|---|---:|---|
| 产品/架构负责人 | 1-2 | 产品边界、市场与平台协同、ADR |
| 平台后端 | 4-6 | 租户、Operation、Runtime、Provider、Agent |
| 市场后端 | 3-4 | Catalog、Artifact、Release、Composition、Entitlement |
| 前端 | 3-4 | 统一门户、市场、实例和运维体验 |
| Kubernetes/Provider | 3-5 | Helm、Operator、数据库、中间件、网络和存储 |
| 安全 | 1-2 | 签名、SBOM、扫描、准入和供应链 |
| 测试与质量 | 3-4 | 契约、兼容、故障、升级、性能和安全测试 |
| SRE/交付 | 2-3 | 安装器、离线包、升级、诊断和环境认证 |
| AI 平台与治理 | 2-4（分阶段） | AI Gateway、模型运行时、评测、Copilot、AIOps 和 AI 安全 |
| 边缘计算 | 2-3（V1.5 前到位） | Edge Provider、KubeEdge、设备 Mapper、OTA、弱网分发和现场运维工具 |

### 22.2 领域责任

- 市场团队对 Product、Release 和 Artifact 的权威性负责；
- 平台团队对 ExecutionPlan 和运行状态负责；
- Provider 团队对具体产品生命周期负责；
- 安全团队对信任链和门禁负责；
- SRE 对容器化交付、Artifact Storage、备份恢复、升级和离线包负责；
- 存储/基础设施团队对 S3、磁盘、网络和故障域能力负责；
- 产品团队确保普通用户不需要理解底层 Chart 和 CRD；
- AI 平台团队对 ModelService、AI Gateway 和 AI Provider 契约负责；
- AI 治理团队对模型许可、评测、Guardrail、Prompt 和自动化边界负责；
- AIOps 团队与 SRE 共同维护证据链、Runbook 和自动修复策略；
- 边缘团队对 Edge Provider、CloudCore/EdgeCore 兼容、设备 Mapper 认证、OTA 和现场恢复负责；
- 产品与数据团队维护统一 UsageRecord/MeterEvent、DORA、部署成功率、市场活跃度和边缘在线率等采纳指标。

---

## 23. 测试与验收体系

### 23.1 测试层级

- 单元测试；
- Schema 和 API 兼容测试；
- 市场/平台契约测试；
- Provider Conformance；
- Release 安装/升级/回滚/卸载测试；
- 组合 DAG 和补偿测试；
- 多租户隔离；
- 容器安全；
- 离线安装；
- 故障注入；
- 性能和容量；
- 长稳测试；
- 灾备演练；
- AI Provider 契约与模型兼容测试；
- AI Gateway 协议、限流、Fallback 和租户隔离；
- 模型评测、Prompt/Guardrail 回归和 AI 安全测试；
- Copilot 越权、幻觉、证据与写操作旁路测试；
- AIOps 故障注入、熔断和自动修复范围测试；
- 边缘断连自治、重连对账、批量灰度、OTA、设备访问、离线撤销和遥测补传测试；
- 边缘多架构、资源上限、时钟偏差、证书轮换和物理不可信场景测试。

### 23.2 V3.8 专项验收

| 编号 | 验收项 |
|---|---|
| CTN-ACC-01 | 平台和市场所有服务进程均运行在容器中 |
| CTN-ACC-02 | 不存在 RPM/DEB/systemd 直接交付数据库或中间件路径 |
| CTN-ACC-03 | 同一平台版本可运行在物理机 Kubernetes、VM Kubernetes 和单机 Podman 环境 |
| MKT-ACC-01 | 市场可独立部署、升级、备份和恢复 |
| MKT-ACC-02 | 市场数据库与平台数据库完全隔离 |
| MKT-ACC-03 | 市场不保存 kubeconfig 和租户运行 Secret |
| MKT-ACC-04 | 支持应用、数据库、中间件分类和标准标签 |
| ART-ACC-01 | 支持 OCI 镜像、Helm、JAR 和 WAR 入库 |
| ART-ACC-02 | 所有发布制品使用 digest 锁定且不可覆盖 |
| ART-ACC-03 | JAR/WAR 以容器方式运行，主机上无直接 Java 服务进程 |
| STO-ACC-01 | 镜像、Helm、JAR/WAR、配置和 SBOM 均可通过统一 OCI Endpoint 访问 |
| STO-ACC-02 | Market/Platform API 不转发大文件，Agent/客户端直接从 Registry 拉取 |
| STO-ACC-03 | Lite HA 模式下单个 Registry Pod 或单个节点故障不影响制品读取 |
| STO-ACC-04 | Registry 全部重建后可依赖共享数据后端恢复服务 |
| STO-ACC-05 | 单节点模式明确标记非 HA，且可备份恢复 |
| STO-ACC-06 | 支持 ArtifactStorageProfile 切换本地/PVC/S3 后端且上层 ReleaseManifest 不变 |
| STO-ACC-07 | 支持 digest 去重、区域缓存、按需扫描和安全 GC |
| STO-ACC-08 | 删除前能够识别运行实例、回滚点、组合和灾备引用 |
| STO-ACC-09 | 市场数据库、制品数据和签名密钥恢复演练通过 |
| STO-ACC-10 | 控制台可查看容量预测、复制、缓存、完整性、GC 和备份状态 |
| CMP-ACC-01 | 至少三个不同包可组成一个 CompositionRelease |
| CMP-ACC-02 | 平台能执行依赖 DAG、输出绑定和失败补偿 |
| INT-ACC-01 | 市场和平台只通过公开 API/事件集成 |
| INT-ACC-02 | 市场中断不影响已部署应用运行 |
| INT-ACC-03 | 平台保存完整 ReleaseManifest 和 ExecutionPlan 以支持审计和复现 |
| SEC-ACC-01 | 发布时和部署时均验证签名、SBOM 和安全策略 |
| OFF-ACC-01 | 离线 Bundle 可在无互联网环境完成导入和部署 |
| AI-ACC-01 | AI Extension Plane 可独立安装、升级、停用，停用后平台核心与传统应用不受影响 |
| AI-ACC-02 | AI Gateway 支持 HTTP、SSE、WebSocket/OpenAI-compatible 路由、限流和 Fallback |
| AI-ACC-03 | ModelArtifact、ModelService 和 ModelEndpoint 可完成发布、部署、观察、升级和删除闭环 |
| AI-ACC-04 | 模型、Prompt、Guardrail 和评测版本使用不可变摘要并可审计 |
| AI-ACC-05 | Tenant A 无法访问 Tenant B 的端点、缓存、调用记录、知识引用和成本数据 |
| AI-ACC-06 | Copilot 所有写操作均转换为结构化计划并经过权限、策略、审批和 Operation |
| AI-ACC-07 | AIOps 根因结果包含证据、置信度和影响对象，无法确认时明确返回不确定 |
| AI-ACC-08 | 高风险自动修复无法绕过人工确认，低风险修复具备熔断、限速和回滚 |
| AI-ACC-09 | AI 调用 Token、延迟、GPU、成本、安全拦截和质量指标可观测 |
| AI-ACC-10 | 外部模型不可用时按策略 Fallback，且不影响非 AI 平台能力 |

#### 23.2.1 Edge Pack 专项验收

| 编号 | 验收项 |
|---|---|
| EDGE-ACC-01 | 未启用 Edge Pack 时，平台安装、运行与既有验收项不受影响 |
| EDGE-ACC-02 | 边缘节点断网 24 小时，已部署负载持续运行且容器可自重启；恢复后自动对账且收敛时间可测量 |
| EDGE-ACC-03 | 对至少 100 个边缘节点执行 5%→25%→100% 灰度，健康门禁、失败暂停和回滚生效 |
| EDGE-ACC-04 | EdgeCore 批量 OTA 含预检、备份、灰度、健康确认和失败回滚 |
| EDGE-ACC-05 | 声明 kubeedge-edge 的 Release 缺少目标架构镜像、资源下限或自治声明时发布门禁拒绝 |
| EDGE-ACC-06 | 撤销版本在断连站点重连后按策略处置；撤销 Artifact 可离线验签且审计完整 |
| EDGE-ACC-07 | MQTT 与 Modbus Mapper 完成市场发布、部署、采集与设备命令闭环，写指令有权限和 Operation 记录 |
| EDGE-ACC-08 | 节点组默认流量闭环生效，跨组访问只有显式声明和策略允许后放行 |
| EDGE-ACC-09 | 边缘 Secret 落盘加密，节点证书吊销后隧道拒绝重连 |
| EDGE-ACC-10 | CloudCore 单副本故障期间管理面可用；断连遥测重连后补传，状态展示携带 lastKnownStateAt |
| EDGE-ACC-11 | 完全离线站点通过签名 Bundle 导入本地 Registry 并完成边缘应用部署 |
| EDGE-ACC-12 | QueuedOffline Operation 在重连后按序执行，超过 maxOfflineDuration 转为 Failed/NeedsAttention 并告警 |

### 23.3 质量门禁

进入生产发布前：

- P0 API 无未解决兼容破坏；
- 所有镜像、Chart 和制品有摘要和签名；
- 关键镜像无未接受的严重漏洞；
- 组合安装、升级、回滚和卸载通过；
- 市场与平台接口通过双向兼容测试；
- Tenant 越权测试通过；
- Agent 断连和恢复通过；
- 市场不可用和 Registry 单实例故障测试通过；
- Artifact Data Plane 单节点/单盘故障与恢复测试通过；
- Blob 摘要校验、复制中断恢复和安全 GC 测试通过；
- 数据保护型卸载测试通过；
- 离线升级通过；
- 审计链完整；
- AI Extension Plane 故障隔离、租户隔离和写操作受控测试通过；
- 模型、Prompt、Guardrail 和评测版本可复现；
- AIOps 自动化具备熔断、回滚和人工停止能力；
- Edge Pack 启用时，断连自治、灰度、OTA、离线撤销、证书吊销和遥测补传验收通过；
- 所有边缘状态展示和 Copilot 回答均正确携带状态时间戳。

---

## 24. 资源与容量建议

### 24.1 Minimal/Dev

单机物理机或 VM，Podman/Docker Compose：

- 8 vCPU；
- 16 GB 内存；
- 200 GB SSD 起；
- 平台、市场、PostgreSQL 和轻量 Registry 采用单实例容器；
- Registry 使用独立本地数据卷；
- 不强制部署 MinIO、Ceph、Redis、完整日志和搜索后端；
- 扫描 Worker 按任务运行；
- 明确标记为非 HA，配置每日备份。

### 24.2 Lite HA 小型生产

三节点 Kubernetes 管理集群：

- 每节点 8-16 vCPU；
- 每节点 32-64 GB 内存；
- 市场 2 副本；
- Registry 2 个以上无状态副本；
- PostgreSQL 3 实例或外部 HA PostgreSQL；
- 优先连接已有企业 S3；无外部 S3 时部署 3 节点轻量 S3 Provider；
- Registry、数据库和 S3 Pod 跨节点分布；
- 独立制品数据盘，建议 500 GB 起并按增长率扩容；
- 扫描 Worker 支持缩容到零；
- 业务应用部署到独立工作集群或节点池。

### 24.3 Standard HA

- 独立或共享管理集群；
- 市场和平台独立 Namespace/节点池；
- 多副本 Registry 或 Harbor；
- 企业 S3、NAS 对象接口或受支持的分布式对象存储；
- HA PostgreSQL、PITR 和独立备份仓库；
- 区域 Registry Mirror/Proxy Cache；
- Agent Gateway 多副本；
- 市场索引、复制、扫描、GC Worker 独立扩展；
- 可观测和安全后端独立或复用企业平台。

### 24.4 Enterprise/Multi-Region

- 独立管理集群和存储故障域；
- 中心权威 Registry + 区域镜像 + 边缘缓存；
- Ceph RGW、企业对象存储或云 S3；
- 跨站点 Bucket/制品复制和元数据恢复方案；
- GSLB/DNS 或区域入口；
- 密钥、审计和 Release Lock 异地备份；
- 定期区域故障切换与回切演练。

### 24.5 容量规划

制品容量至少考虑：

```text
有效容量需求
= 当前唯一 Blob
+ 版本增长
+ 回滚保留
+ stable/lts 长期保留
+ SBOM/签名/证明
+ 复制与纠删码开销
+ GC 安全余量
```

建议：

- 数据盘长期使用率控制在 70% 以下；
- 80% 触发扩容告警，90% 禁止非必要发布；
- 结合最近 30/90 天增长率预测剩余天数；
- 节点缓存和区域缓存单独计量，不纳入权威数据容量；
- Blob 去重率、缓存命中率和 GC 回收率作为能效指标。

### 24.6 AI 资源与容量规划

AI 能力必须单独估算，不计入 HNB Core 的最低资源基线：

- Minimal AI：AI Gateway/Copilot 2-4 vCPU、4-8 GB 内存，可只使用外部模型；
- Lite AI：按模型规模增加 CPU、内存或单 GPU，评测和 AIOps Worker 支持缩容到零；
- Standard AI：GPU 节点池与管理节点分离，模型缓存使用独立高速盘；
- Enterprise AI：按模型、租户、地域和可用区规划 GPU/NPU 资源池、模型镜像源和全局路由。

容量模型至少考虑：

```text
AI 计算需求
= 峰值并发 × 单请求 Token/音视频负载
÷ 单实例吞吐
× 冗余与灰度系数
```

还需单独规划模型权重容量、模型加载带宽、KV Cache/显存、向量数据、调用日志、评测数据和成本预算。GPU 功耗、利用率、空闲时间和单位 Token 能耗作为能效指标。

### 24.7 性能指标建议

最终指标需通过压测确认，初始目标：

- 市场 Catalog P95 查询 < 500 ms；
- 平台核心资源查询 P95 < 500 ms；
- Release 解析 P95 < 2 s，不含制品传输；
- Registry Manifest 查询 P95 < 300 ms（同地域）；
- 10,000 Product、100,000 Release 可检索；
- 单平台支持 100 个集群级运行目标；
- Operation 状态更新不因市场不可用阻塞；
- 市场事件至少一次投递，平台幂等消费；
- 1 GB 以上制品支持分片、校验和断点续传；
- 镜像拉取和大文件传输不经过市场或平台 API；
- Lite HA 单个 Registry Pod 故障期间已有入口可继续提供服务；
- 区域缓存命中率和中心出口流量可观测；
- GC 不阻塞正常拉取，并具有限速和暂停能力；
- AI Gateway P95 附加开销、首 Token 延迟、Token 吞吐、模型冷启动和 Fallback 可测量；
- Copilot 查询不得实时遍历所有集群，优先使用 Read Model 和检索索引；
- AIOps 批处理不得挤占平台核心和业务工作负载资源。

- 单 CloudCore 域支撑边缘节点数、隧道消息 P95 延迟和吞吐必须压测；初始设计目标为单域不少于 5,000 节点，最终按硬件与配置校准；
- 72 小时断连后重连，状态收敛、排队任务投递和遥测补传时间可测量；
- 1,000 节点并发重连不得导致 CloudHub、数据库或平台事件总线失稳；
- WAN 用量、站点缓存命中率、预拉取成功率和单位边缘节点管理成本可观测。

---

### 24.8 Edge Pack 资源档位

| 档位 | 参考节点规模 | CloudCore 建议 | 说明 |
|---|---:|---|---|
| Edge Lite | ≤ 200 | 2 副本，每副本 2 vCPU/4 GB 起 | 可与管理集群共部署，需独立 PDB/反亲和 |
| Edge Standard | ≤ 2,000 | 3 副本，每副本 4 vCPU/8 GB 起，CloudHub 可独立扩展 | 独立 LB、监控和证书治理 |
| Edge Enterprise | ≥ 5,000/多地域 | 按地域分域部署和压测定容 | 分域隔离、重连风暴保护、区域 Registry Mirror |

边缘节点资源由 ReleaseManifest 声明并在部署前检查。需单独规划 EdgeCore、容器运行时、站点缓存、日志环形缓冲、设备 Mapper、模型权重和业务负载的 CPU、内存、磁盘与写放大。

## 25. 主要风险与应对

| 风险 | 表现 | 应对 |
|---|---|---|
| 市场变成第二个平台 | 市场开始管理集群和运行状态 | 强制职责矩阵和禁止集群凭据 |
| 平台变成制品仓库 | 大文件经过平台 API | 平台只处理 ArtifactDescriptor 和短期授权，数据直连 Registry |
| JAR/WAR 破坏容器化原则 | 主机直接运行 Java | 仅支持 OCI 镜像或标准运行时容器 |
| 组合编排过度复杂 | 自研通用工作流/BPM | 只支持部署 DAG、Saga 和受控生命周期 |
| 超大 Helm Chart | 所有组件绑死在一个 Chart | 平台组合多个独立 Release |
| 版本不可复现 | 使用 latest 或未锁依赖 | digest、manifest.lock、不可变 Release |
| 市场与平台强耦合 | 共享数据库和内部代码 | API First、事件契约、独立升级 |
| 市场故障阻塞业务 | 运行时依赖市场 | 平台缓存 Manifest，数据面自治 |
| 供应链风险 | 上传恶意包或高危镜像 | 双重门禁、签名、SBOM、撤销机制 |
| 外部 Registry 不稳定 | 部署时拉取失败 | 区域镜像、预拉取、健康切换、离线 Bundle |
| 伪高可用 | 多个 Registry 副本使用独立本地目录 | 生产 HA 强制共享 S3 后端和故障域验收 |
| 存储体系过重 | 小环境强制部署 Ceph/MinIO/Redis | Minimal 不强制对象存储，优先复用企业 S3 |
| 市场 API 成为带宽瓶颈 | 上传下载经过业务 API | 短期 Token + Registry/S3 直传直取 |
| 制品无限增长 | 历史版本和孤儿 Blob 长期保留 | 引用保护、保留期、Tombstone 和安全 GC |
| GC 误删制品 | Registry 独立删除生产内容 | 市场、平台、灾备引用联合判定并通过 Operation 执行 |
| 缓存被当成权威 | 区域缓存丢失导致恢复困难 | 中心权威仓库与可重建缓存严格分层 |
| 备份不可恢复 | 只做文件复制不做演练 | PG PITR、Bucket 保护、密钥备份和定期整站恢复 |
| Provider 能力不一致 | 页面有按钮但无法执行 | 生命周期能力协商和认证等级 |
| 卸载误删数据 | 组合补偿删除数据库 | 数据保护默认、人工确认、保留策略 |
| 首期范围失控 | 同时支持过多包和服务 | MVP 只做 OCI、Helm、JAR Runtime、外部模型接入和少量示例产品 |
| AI 成为核心依赖 | 模型或网关故障导致平台不可用 | AI Extension Plane 可选、故障隔离、核心确定性降级 |
| AI 幻觉或错误操作 | Copilot 编造状态或误执行 | 证据检索、结构化计划、权限策略、人工确认和审计 |
| 敏感数据泄漏 | Prompt/日志发送外部模型 | Provider 数据策略、脱敏、区域限制、最小留存和租户授权 |
| Prompt 注入与工具越权 | Agent 调用高风险工具 | 工具白名单、Schema、最小权限、审批和沙箱 |
| 模型成本失控 | Token、GPU 或外部 API 费用异常 | 配额、预算、限流、成本看板、路由和缓存 |
| GPU 资源碎片 | 显存浪费和调度失败 | Accelerator Provider、拓扑感知、MIG/HAMi 可选和容量预测 |
| 模型质量漂移 | 升级后效果下降 | Evaluation Gate、灰度、反馈指标和快速回滚 |
| AIOps 误修复 | 自动化扩大故障 | 风险分级、影响范围限制、冷却、熔断和人工停止 |
| 断连期间运行已撤销版本 | 漏洞暴露窗口延长 | 签名撤销 Artifact、离线策略、重连强制处置和高风险受限模式 |
| CloudHub 拥塞或重连风暴 | 大量节点同时重连压垮控制面 | 多副本、LB、指数退避、随机抖动、消息优先级和分域部署 |
| 批量 OTA 失败 | 节点批量不可用或“变砖” | 灰度、备份、健康门禁、看门狗回滚、站点级串行和现场恢复镜像 |
| 边缘介质磨损 | 日志/缓存写放大导致 SD/eMMC 损坏 | 环形缓冲、采样、只读根分区、独立数据分区和介质巡检 |
| 设备协议碎片化 | Mapper 质量和维护成本失控 | Mapper Framework、统一 Schema、认证等级和参考实现 |
| 物理不可信节点 | 节点被拆走、篡改或 Secret 泄漏 | 证书吊销、最小本地数据、磁盘加密、短期凭据和重新入网授权 |
| 边缘资源超限 | 大镜像/模型压垮节点 | 发布资源声明、磁盘水位预检、预拉取、量化和瘦身标签 |
| 遥测占满隧道 | 控制消息和升级任务受阻 | 业务数据与控制隧道分离，本地聚合、采样、优先级和 WAN 配额 |
| 云边版本偏斜 | CloudCore/EdgeCore/CRD 不兼容 | 兼容矩阵、Conformance、先云后边、分批升级和版本护栏 |

---

## 26. V3.8 新增与调整需求清单

### 26.1 容器化要求

| 编号 | 优先级 | 需求 |
|---|---|---|
| CTN-01 | P0 | HNB Cloud 平台组件必须全部以 OCI 容器方式运行。 |
| CTN-02 | P0 | HNB App Market 组件必须全部以 OCI 容器方式运行。 |
| CTN-03 | P0 | 平台交付的应用、数据库和中间件不得通过 RPM、DEB、systemd 或裸二进制直接运行；仅 EdgeCore 节点代理按 3.5.5 受控例外。 |
| CTN-04 | P0 | 物理机和虚拟机仅作为 Kubernetes 或 OCI 容器运行底座。 |
| CTN-05 | P0 | JAR/WAR 部署必须由 OCI 镜像或标准运行时容器承载。 |
| CTN-06 | P1 | 平台镜像和主要服务镜像支持 amd64/arm64 多架构。 |
| CTN-07 | P0 | 所有生产部署必须以镜像 digest 固定版本。 |

### 26.2 应用市场

| 编号 | 优先级 | 需求 |
|---|---|---|
| MKT-01 | P0 | 提供独立部署、通过 API 与平台集成的统一应用市场。 |
| MKT-02 | P0 | 市场支持应用、边缘应用、数据库、中间件、设备接入及其它平台能力分类。 |
| MKT-03 | P0 | 市场支持标准标签、自定义标签和多条件检索。 |
| MKT-04 | P0 | 市场支持 Product、Package、Artifact、Release 和 Channel。 |
| MKT-05 | P0 | 市场支持 OCI 镜像、Helm、JAR、WAR 和离线 Bundle。 |
| MKT-06 | P0 | Release 发布后不可覆盖，必须使用制品摘要锁定。 |
| MKT-07 | P0 | 市场支持发布审核、渠道晋级、弃用和撤销。 |
| MKT-08 | P0 | 市场支持发布者、授权、租户可见范围和订阅。 |
| MKT-09 | P0 | 市场不得持有业务集群 kubeconfig 和租户运行 Secret。 |
| MKT-10 | P1 | 市场支持上游同步、区域镜像和离线导入导出。 |
| MKT-11 | P0 | 市场支持 SBOM、签名、漏洞结果和构建证明关联。 |
| MKT-12 | P1 | 一个市场可服务多个 HNB Cloud 平台实例。 |

### 26.3 多包编排

| 编号 | 优先级 | 需求 |
|---|---|---|
| CMPOS-01 | P0 | 市场支持 CompositionRelease 定义多个 Product Release 的组合。 |
| CMPOS-02 | P0 | CompositionRelease 支持依赖、顺序、条件、参数和输出绑定。 |
| CMPOS-03 | P0 | 平台将 CompositionRelease 解析为不可变 ExecutionPlan。 |
| CMPOS-04 | P0 | Operation Engine 支持 DAG、并行、重试、补偿和断点恢复。 |
| CMPOS-05 | P0 | 有状态组件失败时默认不得自动删除数据。 |
| CMPOS-06 | P1 | 支持组合应用整体升级和组件级升级。 |
| CMPOS-07 | P1 | 支持版本漂移检测和依赖锁校验。 |
| CMPOS-08 | P0 | Helm 可以作为编排节点，但不得成为唯一组合编排机制。 |

### 26.4 平台市场集成

| 编号 | 优先级 | 需求 |
|---|---|---|
| INT-01 | P0 | 市场和平台必须通过版本化 OpenAPI、事件和回调集成。 |
| INT-02 | P0 | 禁止市场和平台共享业务数据库。 |
| INT-03 | P0 | 平台必须在本地保存已部署 ReleaseManifest 和制品摘要。 |
| INT-04 | P0 | 平台部署前必须检查市场授权、撤销状态和签名。 |
| INT-05 | P0 | 市场故障不得影响已运行应用和服务。 |
| INT-06 | P1 | 市场短时不可用时可按策略使用已缓存 Release。 |
| INT-07 | P0 | 平台不得把运行 Secret、业务日志或用户数据回传市场。 |
| INT-08 | P1 | 平台可回传匿名化兼容性和部署结果。 |

### 26.5 制品供应链

| 编号 | 优先级 | 需求 |
|---|---|---|
| ART-01 | P0 | 大文件必须通过 HNB Artifact Storage 管理，市场数据库只保存 ArtifactDescriptor、引用和状态。 |
| ART-02 | P0 | 所有制品必须具有 SHA-256 摘要。 |
| ART-03 | P0 | 生产渠道必须完成签名、SBOM 和安全扫描。 |
| ART-04 | P0 | JAR/WAR 必须绑定不可变运行时镜像摘要。 |
| ART-05 | P0 | Chart 引用镜像必须生成锁文件。 |
| ART-06 | P1 | 支持制品复制、镜像同步、断点续传和完整性校验。 |
| ART-07 | P1 | 支持 Release 撤销后的实例影响分析。 |
| ART-08 | P0 | Helm、JAR/WAR、模型、Prompt、Guardrail、评测、配置包和 SBOM 应优先映射为 OCI Artifact/Referrer。 |
| ART-09 | P0 | 离线交付包应支持 OCI Image Layout 和签名 manifest.lock。 |

### 26.6 统一制品存储

| 编号 | 优先级 | 需求 |
|---|---|---|
| ART-STO-01 | P0 | 提供独立逻辑的 HNB Artifact Storage，统一管理镜像、Helm、JAR/WAR、Operator、模型、Prompt、Guardrail、评测、配置、SBOM 和离线包。 |
| ART-STO-02 | P0 | 提供统一 OCI Registry Endpoint，屏蔽底层 Registry 和对象存储实现。 |
| ART-STO-03 | P0 | Market/Platform API 不得代理大文件正文，上传和拉取必须直接访问 Registry。 |
| ART-STO-04 | P0 | 所有执行引用必须固定 digest，标签不得作为生产执行依据。 |
| ART-STO-05 | P0 | Minimal 模式不得强制依赖独立对象存储，允许轻量 Registry 使用本地独立数据卷。 |
| ART-STO-06 | P0 | 单机本地卷模式必须明确标记为非 HA。 |
| ART-STO-07 | P0 | Lite HA 模式必须支持多副本无状态 Registry、共享 S3 后端和 HA PostgreSQL。 |
| ART-STO-08 | P0 | 生产 HA 禁止多个 Registry 副本使用相互独立的本地权威目录。 |
| ART-STO-09 | P0 | 优先复用企业已有 S3、Registry 和对象存储能力。 |
| ART-STO-10 | P0 | 提供 ArtifactStorageProfile、ArtifactSourcePolicy 和 ArtifactStorageProvider。 |
| ART-STO-11 | P0 | Provider 必须声明 OCI、Referrer、复制、缓存、不可变、GC、后端和 HA 能力。 |
| ART-STO-12 | P0 | 支持内容寻址去重、分块上传、断点续传和完整性校验。 |
| ART-STO-13 | P1 | 支持中心权威仓库、区域镜像和边缘缓存三级分发。 |
| ART-STO-14 | P1 | 支持节点预拉取、缓存水位和 LRU 清理。 |
| ART-STO-15 | P1 | 扫描 Worker 支持按需扩缩，同一 digest 的结果可以复用。 |
| ART-STO-16 | P0 | 删除必须经过引用分析、Tombstone、保留期和安全 GC。 |
| ART-STO-17 | P0 | 当前运行、回滚、组合、灾备和离线 Bundle 引用的制品不得被回收。 |
| ART-STO-18 | P1 | GC 必须通过 Operation 执行，支持预览、锁、限速、暂停、重试和审计。 |
| ART-STO-19 | P0 | 支持市场数据库 PITR、制品数据保护、密钥备份和整站恢复。 |
| ART-STO-20 | P1 | 提供容量预测、健康评分、复制延迟、缓存命中、完整性和备份驾驶舱。 |
| ART-STO-21 | P1 | 支持 Registry 无中断滚动升级和全量重建验证。 |
| ART-STO-22 | P1 | 支持本地/PVC/S3 后端迁移，迁移不得改变上层 Product Release 和 digest 引用。 |
| ART-STO-23 | P1 | 支持按部署档位定义 RPO/RTO，并通过演练验证。 |
| ART-STO-24 | P0 | Registry、对象存储和数据库实现不得成为平台内核硬编码依赖。 |


### 26.7 AI 架构与解耦

| 编号 | 优先级 | 需求 |
|---|---|---|
| AI-ARCH-01 | P0 | AI 能力必须通过独立容器化 AI Extension Plane 或 Provider 提供，不得成为 HNB Core 强依赖。 |
| AI-ARCH-02 | P0 | AI Extension Plane 故障或停用不得影响平台核心、传统应用和数据库中间件运行。 |
| AI-ARCH-03 | P0 | AI 组件只能通过版本化 API、事件、Provider 和 Tenant Context 与平台集成。 |
| AI-ARCH-04 | P0 | AI 结论不得作为平台权威事实源，写操作不得绕过 Operation Engine。 |
| AI-ARCH-05 | P1 | 支持 AI Access、AI Runtime、AI Governance 和 AIOps 能力包独立启停。 |

### 26.8 模型与 AI 运行服务

| 编号 | 优先级 | 需求 |
|---|---|---|
| AI-RUN-01 | P0 | 提供 ModelArtifact、ModelService 和 ModelEndpoint 统一资源模型。 |
| AI-RUN-02 | P0 | 模型制品必须记录版本、摘要、来源、许可证、运行资源和评测状态。 |
| AI-RUN-03 | P0 | 支持 OCI 模型制品和受信外部模型引用两种模式。 |
| AI-RUN-04 | P0 | ModelService 必须通过 Provider 完成预检、部署、观察、升级、扩缩容和删除。 |
| AI-RUN-05 | P0 | 支持 CPU、GPU、NPU 和外部模型服务能力声明，平台不得绑定单一推理引擎。 |
| AI-RUN-06 | P1 | 支持模型灰度、流量分配、快速回退和版本漂移检测。 |
| AI-RUN-07 | P1 | 支持 Embedding、Rerank、语音、视觉、多模态和 Agent 服务类型。 |

### 26.9 AI Gateway

| 编号 | 优先级 | 需求 |
|---|---|---|
| AI-GW-01 | P0 | AI Gateway 支持 HTTP、SSE、WebSocket 和 OpenAI-compatible 协议。 |
| AI-GW-02 | P0 | 支持模型路由、负载均衡、超时、熔断、重试和 Fallback。 |
| AI-GW-03 | P0 | 支持租户、项目、用户、Token、并发和速率限流。 |
| AI-GW-04 | P0 | 支持输入输出安全围栏、敏感数据脱敏和调用审计。 |
| AI-GW-05 | P0 | AI Gateway 不得承担 Kubernetes 资源部署和普通业务流量转发。 |
| AI-GW-06 | P1 | 支持语义缓存、成本优化和多 Provider 路由，缓存必须租户隔离。 |

### 26.10 AI 治理与可观测

| 编号 | 优先级 | 需求 |
|---|---|---|
| AI-GOV-01 | P0 | 提供 PromptTemplate、GuardrailPolicy、EvaluationSuite 和 EvaluationRun。 |
| AI-GOV-02 | P0 | 模型、Prompt、Guardrail 和评测版本必须不可变、可追溯和可撤销。 |
| AI-GOV-03 | P0 | Tenant ID 必须贯穿端点、缓存、调用、知识引用、日志、用量和成本。 |
| AI-GOV-04 | P0 | 外部模型 Provider 必须支持数据区域、留存、脱敏和敏感数据策略。 |
| AI-GOV-05 | P0 | AI 调用必须提供 Token、延迟、GPU、错误、成本和安全事件观测。 |
| AI-GOV-06 | P1 | 支持发布前质量、安全、业务评测和线上质量漂移检测。 |
| AI-GOV-07 | P1 | 支持 AI 预算、成本中心、告警和超额处置。 |

### 26.11 Platform Copilot 与 AIOps

| 编号 | 优先级 | 需求 |
|---|---|---|
| AI-OPS-01 | P1 | Platform Copilot 支持权限过滤后的自然语言查询、解释和操作草案生成。 |
| AI-OPS-02 | P0 | Copilot 写操作必须转换为结构化 ExecutionPlan 或 ResourceRequest。 |
| AI-OPS-03 | P1 | AIOps 输出必须包含证据、时间范围、影响对象、置信度和不确定性。 |
| AI-OPS-04 | P1 | 支持告警摘要、异常关联、根因候选、容量预测和 Runbook 推荐。 |
| AI-OPS-05 | P1 | 自动修复必须按风险分级，支持熔断、限速、回滚、冷却和人工停止。 |
| AI-OPS-06 | P0 | 高风险删除、数据库切换、灾备、网络、存储和大规模扩缩容不得无确认自动执行。 |
| AI-OPS-07 | P1 | 修复后必须验证效果，失败或无改善时停止连续自动动作。 |

---

### 26.12 边缘计算

| 编号 | 优先级 | 需求 |
|---|---|---|
| EDGE-01 | P0 | 提供可选 Edge Pack，未启用时不得影响平台既有安装、运行和升级。 |
| EDGE-02 | P0 | KubeEdge 边缘节点通过 CloudHub–EdgeHub 隧道接入，不额外部署 HNB Agent。 |
| EDGE-03 | P0 | 提供 EdgeNode、NodeGroup、EdgeApplication、DeviceModel、Device 和 EdgeOTAJob 统一投影。 |
| EDGE-04 | P0 | 边缘节点断连期间按本地缓存状态自治运行，重连后自动对账且收敛时间可观测。 |
| EDGE-05 | P0 | 边缘批量部署支持灰度批次、失败容忍、健康门禁、暂停和自动回滚。 |
| EDGE-06 | P0 | 边缘兼容 Release 声明 targetTypes、架构、资源下限、WAN 依赖与离线自治能力。 |
| EDGE-07 | P1 | 支持镜像/模型预拉取，WAN 传输支持限速、优先级、断点续传和传输窗口。 |
| EDGE-08 | P1 | 支持 EdgeCore 批量 OTA，包含预检、备份、灰度、健康确认和失败回滚。 |
| EDGE-09 | P1 | 支持受控配置更新，必须配置维护窗口、站点级串行和回滚。 |
| EDGE-10 | P0 | 撤销列表以签名 OCI Artifact/Bundle 分发，断连和重连处置策略可配置并审计。 |
| EDGE-11 | P1 | 设备 Mapper 必须容器化并经过市场门禁，设备写操作必须经过权限、Operation 和审计。 |
| EDGE-12 | P1 | 节点组内默认流量闭环，跨组或回源访问必须显式声明。 |
| EDGE-13 | P1 | 边缘 Secret 本地加密，节点证书可轮换和远程吊销。 |
| EDGE-14 | P1 | 边缘指标/日志断连本地缓存、重连补传，所有状态投影标注 lastKnownStateAt。 |
| EDGE-15 | P2 | 支持边缘 ModelService、模型三级分发和可选站点 AI Gateway。 |
| EDGE-16 | P0 | CloudCore 多副本高可用并支持按地域分域；节点规模目标经压测校准。 |
| EDGE-17 | P1 | 离线站点支持签名 Bundle 导入本地 Registry 并驱动边缘运行。 |
| EDGE-18 | P0 | Operation 支持 QueuedOffline、幂等顺序投递和 maxOfflineDuration 超时处置。 |
| EDGE-19 | P1 | 支持关键边缘应用 Hold/Release，在本地安全状态满足后再应用升级。 |
| EDGE-20 | P1 | 入网预检包含 NTP/PTP、架构、磁盘、容器运行时、证书和网络连通性。 |

### 26.13 跨领域产品化与治理

| 编号 | 优先级 | 需求 |
|---|---|---|
| GOV-01 | P1 | MVP 起预留统一 UsageRecord/MeterEvent，覆盖资源、制品、AI 和边缘 WAN 用量。 |
| GOV-02 | P1 | 提供 ISV 发布者沙箱，可自助执行扫描、兼容性预检和 Provider/Release Conformance。 |
| GOV-03 | P1 | 定义 DORA、部署成功率、回滚率、市场活跃发布者、边缘在线率和 AI 成本等采纳指标。 |
| GOV-04 | P2 | Portal、CLI 和现场运维界面预留 i18n。 |
| GOV-05 | P0 | 所有商业 SLA、容量和规模指标必须以档位、兼容矩阵、压测与演练结果为依据。 |

## 27. 推荐首个可交付产品组合

首个版本建议只交付以下组合，验证完整闭环而非追求数量：

```text
HNB Cloud Core Pack
+ HNB App Market
+ HNB Artifact Storage Profile（Minimal + Lite HA）
+ Lightweight OCI Registry Provider
+ S3 Connector / 可选轻量 S3 Provider
+ KubernetesTarget
+ OCI Image Provider
+ Helm Provider
+ Java Artifact Runtime Provider
+ PostgreSQL Service Provider
+ Valkey 或 RabbitMQ Service Provider
+ 基础 Network/Storage/Secret
+ 基础指标、日志和审计
+ 可选 AI Access Pack（外部模型 Connector + AI Gateway）
+ 可选只读 Platform Copilot
```

市场预置五个产品：

1. **Java Web 应用模板**：支持 OCI 镜像和 JAR 标准运行时两种发布方式；
2. **PostgreSQL 高可用**：Helm/Operator 容器化交付；
3. **Valkey 或 RabbitMQ**：容器化服务；
4. **Java Web 标准解决方案**：Java 应用 + PostgreSQL + 缓存/消息 + Gateway + 监控；
5. **AI Access Starter**：外部模型连接器 + AI Gateway 路由 + Token 配额 + 调用观测。

Edge Pack 不作为首个 MVP 的必选组件，但阶段 0 必须完成断连自治与灰度发布 POC。V1.5 可新增一个边缘参考组合：

```text
Edge Pack
+ KubeEdge CloudCore/Edge Provider
+ arm64 Edge Collector
+ MQTT Mapper
+ Modbus Mapper
+ 站点 Registry Mirror
+ EdgeApplication 灰度发布
+ ImagePrePullJob / NodeUpgradeJob
+ 可选轻量边缘 AI 推理服务
```

首个商业可交付闭环必须演示：

```text
发布制品
→ 审核并进入 stable
→ 分类和标签检索
→ 制品摘要、签名和存储健康校验
→ 租户获得授权
→ 选择组合应用
→ 平台预检和审批
→ 多包依赖部署
→ 自动生成连接 Secret 和服务绑定
→ 查看日志、指标和告警
→ 备份数据库
→ 升级应用版本
→ 回滚
→ 数据保护型卸载
→ 安全 GC 预览
→ 完整审计
→ 接入外部模型并创建租户隔离路由
→ 使用 Copilot 解释一次失败部署并生成受控重试草案
```

---

## 28. 最终落地原则

1. **全部容器化**：平台和平台提供的业务服务统一使用 OCI 容器；EdgeCore 仅作为受控节点代理例外。
2. **基础设施与运行形态分离**：物理机、虚拟机和云主机是底座，RuntimeTarget 才是平台执行抽象。
3. **四平面解耦**：市场、制品存储、运行治理和 AI 扩展各有权威边界、独立升级和故障隔离。
4. **Edge Pack 不是第五平台**：边缘由平台统一治理，不建立第二套租户、发布、审批和审计体系。
5. **市场管产品，平台管运行**：市场不越权部署，平台不修改已发布内容。
6. **发布不可变**：Release、Chart、镜像、模型、Prompt、Guardrail 和配置均以摘要锁定。
7. **组合而非绑死**：应用、数据库、中间件、AI 和边缘组件通过 CompositionRelease 与 ExecutionPlan 组合。
8. **Helm 是执行工具，不是平台架构**：跨产品、跨生命周期编排由平台负责。
9. **最小核心**：未启用的能力不安装、不占资源、不增加故障面。
10. **Provider 可替换**：网络、存储、数据库、安全、AI、边缘和运行时实现不写死在内核。
11. **数据面自治**：市场、AI 平面和中心控制面故障不影响已运行服务。
12. **云为权威、边可自治**：边缘按最后期望状态运行，重连后对账、补传和执行撤销处置。
13. **单一纳管通道**：同一目标不得被 HNB Agent、KubeEdge 或多集群系统重复接管。
14. **双重安全门禁**：市场负责发布供应链，平台负责部署和运行安全。
15. **信任链可验证**：签名、SBOM、Provenance、撤销和 ExecutionPlan 均可追溯。
16. **先闭环后扩展**：MVP 聚焦少量产品与完整闭环，Edge Pack 在关键 POC 通过后进入 V1.5。
17. **可复现、可升级、可回滚**：实例可追溯到 ReleaseManifest、ExecutionPlan 和全部制品摘要。
18. **OCI 优先、单一入口**：不同包类型使用统一 ArtifactDescriptor、Registry Endpoint 和 Referrer。
19. **高可用分层**：入口、Registry、元数据、制品数据、CloudCore、密钥分别设计故障域。
20. **轻量不等于单点生产**：Minimal 可单机，生产档位必须满足共享数据、备份和故障演练要求。
21. **高能效传输**：大文件直传直取、内容去重、区域/站点缓存、预拉取、限速和安全 GC。
22. **缓存非权威**：中心数据是权威源，区域、站点和节点缓存可重建。
23. **AI 可选增强**：无 AI 能力时平台仍具备完整应用和服务运营闭环。
24. **AI 不替代事实和规则**：模型只辅助理解和建议，强制规则保持确定性。
25. **模型与应用同等治理**：模型、Prompt、Guardrail、评测和端点均需版本、权限、审计和生命周期。
26. **Copilot 不设执行旁路**：所有变更统一经过结构化 API、策略、审批和 Operation。
27. **运维内建**：容量、完整性、证书、备份、恢复、升级、OTA 和故障演练必须产品化。
28. **状态必须带时间语义**：边缘和异步 Read Model 明确实时、最后已知与推断，禁止把陈旧状态伪装为事实。
29. **指标以实测为准**：SLA、RPO/RTO、节点规模、吞吐和延迟均由档位认证、压测和演练确定。
30. **持续度量与演进**：统一 UsageRecord、DORA、部署成功率、成本和平台采纳指标驱动版本演进。

---

## 29. 参考资料与技术版本基线

### 29.1 技术版本基线

- KubeEdge 参考基线：V3.8 设计面向 KubeEdge v1.22–v1.23 能力范围，实施时必须固定具体版本并维护 CloudCore、EdgeCore、Kubernetes、CRD 和 Provider 兼容矩阵；
- Kubernetes、CNI、CSI、Cosign、OPA/Kyverno、Registry、对象存储和 AI Runtime 均以 Provider 认证版本为准，不在方案中写死长期版本；
- 对外承诺使用“支持矩阵 + 认证等级 + 实测指标”，不得仅以开源项目名称推导 SLA。

### 29.2 官方参考资料

1. KubeEdge 项目与版本：https://github.com/kubeedge/kubeedge  
2. KubeEdge 架构与组件：https://kubeedge.io/docs/  
3. KubeEdge MetaManager：https://kubeedge.io/docs/architecture/edge/metamanager/  
4. KubeEdge v1.20：https://kubeedge.io/blog/release-v1.20/  
5. KubeEdge v1.21：https://kubeedge.io/blog/release-v1.21/  
6. KubeEdge v1.22：https://kubeedge.io/blog/release-v1.22/  
7. KubeEdge 安装与边缘任务 CRD：https://kubeedge.io/docs/setup/install-with-binary/  
8. KubeEdge 边缘升级 Hold/Release：https://kubeedge.io/docs/advanced/hold_and_release/  
9. KubeEdge CNI 与控制隧道边界：https://kubeedge.io/docs/advanced/cni-edge-networking/  
10. Sigstore Cosign：https://docs.sigstore.dev/cosign/  
11. SLSA Specification：https://slsa.dev/spec/  
12. OCI Image/Distribution Specifications：https://opencontainers.org/
