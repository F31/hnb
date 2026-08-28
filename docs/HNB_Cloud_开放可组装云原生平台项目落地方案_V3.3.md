# HNB Cloud 开放可组装云原生平台项目落地方案

> **方案版本**：V3.3  
> **文档状态**：容器化交付与统一应用市场增强版  
> **编制日期**：2026-07-17  
> **评审依据**：《HNB 云原生平台需求规格说明书（核心平台版）V3.1》及《HNB Cloud 开放可组装云原生平台项目落地方案 V1.0》  
> **核心定位**：以最小平台内核、独立应用市场、能力包、服务蓝图、组合编排和容器运行目标，建设开放、可组装、轻量、敏捷、可持续演进的企业云原生平台。

---

## 0. V3.3 修订说明

V3.3 对前序方案进行两项根本性澄清和增强：

1. **统一容器化运行约束**：HNB Cloud 平台组件以及平台交付的应用、数据库、中间件、网关、可观测、安全和灾备组件，全部以 OCI 容器方式运行；物理机和虚拟机只作为容器运行底座，不再作为 RPM、DEB、JAR、WAR、二进制或 systemd 服务的直接运行形态。
2. **新增独立统一应用市场**：建设单独部署、通过标准接口与 HNB Cloud 集成的 `HNB App Market`，统一管理 JAR、WAR、Helm Chart、OCI 镜像、离线安装包和组合应用版本；市场负责制品、元数据、发布和组合定义，平台负责目标环境、策略审批、执行编排、运行治理和生命周期管理。

| 修订项 | V3.3 调整 |
|---|---|
| 容器化边界 | 平台及平台交付服务全部容器化；取消 LinuxHost/systemd/裸二进制直接交付路径 |
| 基础设施定位 | 物理机、虚拟机、公有云主机和边缘节点仅作为 Kubernetes 或 OCI 容器运行底座 |
| 运行目标 | 收敛为 KubernetesTarget、ContainerEngineTarget 和 ExternalServiceConnector |
| JAR/WAR 定位 | 作为应用制品存储，不直接运行；部署时由标准运行时镜像承载或预先转换为不可变 OCI 镜像 |
| 应用市场 | 新增独立部署的 HNB App Market，支持分类、标签、版本、渠道、授权、签名、SBOM 和发布审核 |
| 制品存储 | 采用统一目录、分类型后端：OCI Registry、对象存储、Helm OCI、离线 Bundle |
| 多包编排 | 市场定义 Composition Release；平台生成 ExecutionPlan 并执行依赖 DAG、回滚和健康验证 |
| 职责边界 | 市场不连接业务集群、不持有租户运行凭据；平台不承担制品编辑、版本发布和市场运营职责 |
| 接口集成 | 增加 Catalog、Release、Artifact、Entitlement、Deployment、Callback 和 Event 契约 |
| 离线交付 | 支持市场镜像、平台镜像、Chart 和制品统一导出为签名离线 Bundle |
| 实施路线 | 将应用市场基础闭环前置到 MVP，并将“JAR/WAR 容器承载适配”纳入首期验证 |

---

## 1. 执行摘要

HNB Cloud 的目标不是建设一个包含所有开源组件的“大控制台”，而是建设一套能够长期演进的企业应用与服务运营底座。平台应当做到：

- 核心足够小，非必需能力按需安装；
- 能力像积木一样组合、替换、升级和卸载；
- 平台与服务统一使用容器交付和运行；
- 同一套容器化产品可以部署到物理机、虚拟机、私有云、公有云或边缘环境；
- 软件制品统一进入应用市场管理，但实际部署和运行治理由 HNB Cloud 执行；
- 用户看到的是应用、数据库、中间件和解决方案，而不是大量 Kubernetes 原生对象；
- 已运行的数据面不依赖市场或中心控制面的持续可用。

V3.3 建议将产品划分为两个独立部署、松耦合集成的核心系统：

```text
HNB App Market                         HNB Cloud Platform
统一软件资产与发布中心                 统一资源、部署与运行治理中心

- 分类、标签、检索                     - 租户、项目、环境
- 制品和版本                           - 集群与容器运行目标
- 应用/数据库/中间件目录               - 配额、网络、存储、GPU
- 发布审核与渠道                       - 审批、策略与安全准入
- 组合应用定义                         - Operation 与执行计划
- 签名、SBOM、漏洞结果                 - Helm/OCI/Operator 执行
- 授权、订阅与可见范围                 - 监控、日志、备份、升级
- 离线包导出                           - 服务绑定、审计与灾备
```

推荐的协作原则是：

> **市场定义“有什么、是什么版本、由哪些包组成、允许谁使用”；平台决定“部署到哪里、能否部署、如何安全部署、部署后如何运行”。**

市场向平台提供不可变的 `ReleaseManifest` 或 `CompositionRelease`。平台根据目标集群能力、租户配额、安全策略和运行参数生成不可变 `ExecutionPlan`，再由 Operation Engine 调用 Helm、Operator、OCI Runtime 或 Kubernetes API 执行。

---

## 2. 关键设计结论

### 2.1 必须保留的架构基础

| 设计 | 结论 | V3.3 落地要求 |
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

### 2.2 必须纠正的前序设计

前序方案中的以下内容不再成立：

- 不再提供 LinuxHostTarget 直接安装 RPM、DEB、JAR、WAR 或 systemd 服务；
- 不再提供“虚拟机高可用版 + systemd”形式的平台部署；
- 不再将普通主机安装器视为数据库和中间件交付方式；
- JAR/WAR 不作为最终运行单元；
- 应用市场不能直接调用集群凭据或绕过平台执行部署；
- 平台不能把市场制品库简单等同于镜像仓库或 Helm 仓库；
- 多包编排不能只依赖 Helm umbrella chart，应支持平台级组合依赖和跨包输出引用。

### 2.3 V3.3 的四个核心闭环

1. **平台闭环**：租户、项目、环境、运行目标、Operation、审计、策略。
2. **市场闭环**：制品入库、分类、标签、版本、扫描、审核、发布、授权。
3. **应用交付闭环**：选购/选择版本、参数化、预检、审批、执行、验证、回滚。
4. **服务运营闭环**：应用、数据库和中间件的监控、备份、升级、扩缩容、绑定和删除。

---

## 3. 产品定位与边界

### 3.1 HNB Cloud 平台定位

HNB Cloud 是面向企业多租户场景的容器化应用与服务运营平台。它通过统一模型管理 Kubernetes 集群、轻量容器运行环境、应用、数据库、中间件、网络、存储、GPU、安全、可观测和灾备能力。

平台不是：

- 代码仓库；
- 通用 CI/CD 平台；
- 通用制品构建平台；
- 单纯的 Kubernetes Dashboard；
- 把多个开源控制台嵌入一个门户；
- 通用 BPM 或 ITSM；
- 传统软件安装工具。

### 3.2 HNB App Market 定位

HNB App Market 是独立部署的软件资产、产品目录和版本发布中心，面向以下场景：

- 平台官方能力和服务发布；
- 企业内部应用发布；
- ISV 或合作伙伴应用发布；
- 数据库和中间件服务包发布；
- 行业解决方案、多组件套件和离线交付包发布；
- 多个平台实例共享统一软件市场；
- 私有市场、租户市场和公共市场分层运营。

应用市场不是：

- 业务集群部署执行器；
- Kubernetes 管理控制面；
- 应用运行状态的最终数据源；
- 租户密钥托管中心；
- 编译构建流水线；
- 运行时监控和故障处理系统。

### 3.3 容器化硬约束

#### 3.3.1 平台组件

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
- 市场索引、扫描、同步和导出 Worker；
- 平台自带数据库、中间件和可观测组件。

#### 3.3.2 平台提供的服务

以下服务只允许使用容器化实现进行交付：

- 应用运行时；
- PostgreSQL、MySQL、Valkey/Redis 等数据库；
- Kafka、RabbitMQ、RocketMQ、MQTT 等中间件；
- API 网关和微服务治理组件；
- 日志、指标、链路和告警组件；
- 镜像扫描、运行时安全和策略组件；
- 备份、恢复和灾备执行组件。

#### 3.3.3 物理机与虚拟机

物理机和虚拟机仅承担以下角色：

- Kubernetes 节点；
- k3s、RKE2、K0s 等轻量 Kubernetes 节点；
- Docker、Podman 或 containerd 容器主机；
- 容器镜像仓库、对象存储或外部数据库的基础设施承载节点。

平台不得把物理机或虚拟机等同于传统软件直接安装目标。

#### 3.3.4 JAR/WAR 的运行规则

JAR 和 WAR 可以作为软件制品进入市场，但不得由平台以 `java -jar`、Tomcat 目录复制或 systemd 方式直接运行。支持两种容器化交付方式：

1. **不可变镜像方式，生产推荐**  
   发布方在进入生产市场前将 JAR/WAR 构建为 OCI 镜像；市场关联原始包、镜像摘要、Dockerfile/构建证明、SBOM 和签名。平台仅部署 OCI 镜像。

2. **标准运行时镜像承载方式，受控兼容**  
   市场发布 `ArtifactRuntimePackage`，包含 JAR/WAR、运行时镜像摘要、启动参数、挂载规则和健康检查。平台把制品以只读卷、Init Container 或受控下载方式注入标准运行时容器。该方式适合迁移期和内部应用，不建议作为高安全生产环境的长期默认方案。

可选建设独立的 `Image Packaging Service`，将 JAR/WAR 转换为 OCI 镜像。该服务是市场的可选外部集成，不纳入 HNB Cloud 核心，也不演变为通用 CI/CD 平台。

---

## 4. 总体逻辑架构

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                              统一访问体验                                  │
│ HNB Portal │ Tenant Portal │ CLI │ OpenAPI │ SDK │ 第三方门户              │
└───────────────────────────────┬─────────────────────────────────────────────┘
                                │
          ┌─────────────────────┴──────────────────────────┐
          │                                                │
┌─────────▼──────────────────────┐             ┌───────────▼──────────────────┐
│ HNB App Market                 │             │ HNB Cloud Platform           │
│ 独立部署、无业务集群凭据        │             │ 资源与运行治理控制面          │
│                                │             │                              │
│ Catalog / Search / Category    │             │ Tenant / Project / Env       │
│ Artifact / Release / Channel   │             │ RuntimeTarget / Cluster      │
│ Tag / License / Entitlement    │             │ Policy / Approval / Quota    │
│ Composition Release            │             │ Operation / ExecutionPlan    │
│ Signature / SBOM / Scan Result │             │ Provider / Runtime Driver    │
│ Publish / Review / Promotion   │             │ Read Model / Graph / Audit   │
└─────────┬──────────────────────┘             └───────────┬──────────────────┘
          │ Catalog/Release API                            │ Agent/Provider API
          │ Event/Callback                                 │
          └─────────────────────┬──────────────────────────┘
                                │
┌───────────────────────────────▼─────────────────────────────────────────────┐
│                         制品与分发基础设施                                  │
│ OCI Registry │ Helm OCI │ Object Storage │ Signature Store │ Scan Provider │
└───────────────────────────────┬─────────────────────────────────────────────┘
                                │
┌───────────────────────────────▼─────────────────────────────────────────────┐
│                             容器执行面                                      │
│ Cluster Agent │ Container Agent │ Helm │ Operator │ Kubernetes API │ OCI   │
│ CNI │ CSI │ Gateway │ Database Operator │ Middleware Operator │ OTel       │
└───────────────────────────────┬─────────────────────────────────────────────┘
                                │
┌───────────────────────────────▼─────────────────────────────────────────────┐
│                           容器运行基础设施                                  │
│ 物理机 │ 虚拟机 │ 私有云 │ 公有云主机 │ 边缘节点 │ Kubernetes │ Podman     │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.1 架构原则

- 市场和平台独立部署、独立数据库、独立升级；
- 市场与平台只通过版本化 API、事件和回调集成；
- 市场不直接访问 Kubernetes API；
- 平台不直接修改市场发布内容；
- 制品使用摘要寻址，发布后不可原地覆盖；
- 平台执行时固定所有镜像、Chart 和文件摘要；
- 市场故障不影响已部署应用运行；
- 平台故障不影响业务数据面运行；
- 市场可服务多个 HNB Cloud 平台实例；
- 平台可配置一个主市场和多个镜像源或上游市场。

---

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
- 配置、许可证和版本兼容管理。

以下内容不得编译进入内核：

- CNI、CSI；
- Karmada；
- HAMi；
- 数据库和中间件 Operator；
- Helm 客户端的领域逻辑；
- 镜像扫描器；
- 运行时安全探针；
- 指标、日志和链路存储；
- OCI Registry；
- 对象存储；
- HNB App Market；
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
├── release-service
├── composition-service
├── entitlement-service
├── review-service
├── search-indexer
├── security-aggregator
├── sync-mirror-worker
├── offline-bundle-worker
└── event-publisher
```

全部组件容器化部署。最小模式可合并为模块化单体；生产模式可将索引、同步、扫描聚合和离线导出 Worker 独立扩展。

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
| 应用 | Web 应用、移动后端、数据应用、行业应用、AI 应用 |
| 数据库 | 关系型、键值、文档、时序、搜索、向量数据库 |
| 中间件 | 消息队列、缓存、注册配置、任务调度、API 网关 |
| 基础能力 | 网络、存储、GPU、证书、DNS、负载均衡 |
| 可观测 | 指标、日志、链路、告警、Dashboard |
| 安全 | 镜像安全、运行时安全、策略、密钥、审计 |
| 运维与灾备 | 备份、恢复、迁移、容灾、诊断 |
| 解决方案 | 多应用组合、行业套件、平台能力包 |

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
- 是否支持离线部署。

核心标签由平台治理，不允许发布方随意修改含义；租户和用户可增加私有收藏标签，但不改变产品标准元数据。

### 6.5 制品物理存储

统一应用市场不应把所有二进制直接塞入关系数据库，也不应自研所有仓库存储协议。推荐“统一逻辑目录 + 专用物理后端”：

| 制品类型 | 推荐物理存储 | 说明 |
|---|---|---|
| OCI 镜像 | OCI Registry/Harbor | 以 digest 固定版本，支持签名、复制和漏洞扫描 |
| Helm Chart | OCI Registry 优先 | 兼容传统 Chart Repository，但内部统一转为 OCI 引用 |
| JAR/WAR | S3 兼容对象存储 | 使用 SHA-256 摘要、签名和不可变版本 |
| YAML/Kustomize | OCI Artifact 或对象存储 | 发布后不可覆盖 |
| Operator Bundle | OCI Registry | 与 Operator/CRD 兼容声明绑定 |
| 初始化数据和脚本 | 对象存储 | 必须扫描、签名、声明执行权限 |
| 离线 Bundle | 对象存储 | 包含 manifest.lock、制品和验证材料 |
| SBOM/签名/证明 | OCI Referrer 或证明存储 | 与制品摘要关联 |

市场数据库只保存元数据、索引和存储引用，不保存大文件正文。

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

## 7. 应用市场与平台职责分工

### 7.1 RACI 边界

| 能力 | HNB App Market | HNB Cloud Platform |
|---|---|---|
| 软件产品建模 | 主责 | 读取和映射 |
| 分类、标签、搜索 | 主责 | 展示、筛选和本地缓存 |
| 制品上传和镜像同步 | 主责 | 不负责上传，可触发镜像预拉取 |
| 版本、渠道和发布审核 | 主责 | 选择允许的渠道 |
| 签名、SBOM、供应链证明 | 主责聚合 | 部署前再次验证 |
| 产品授权和可见范围 | 主责 | 叠加租户和项目权限 |
| 组合应用定义 | 主责定义 Composition Release | 解析为 ExecutionPlan 并执行 |
| 租户、项目和环境 | 不管理 | 主责 |
| 集群、节点和运行目标 | 不管理 | 主责 |
| 配额、网络、存储和 GPU | 不管理 | 主责 |
| 参数填写体验 | 提供 Schema 和默认值 | 结合目标环境生成最终表单 |
| 部署预检 | 提供产品规则 | 主责执行环境、配额、安全预检 |
| 审批 | 发布审批 | 资源申请和部署审批 |
| 实际部署 | 禁止直接执行 | 主责 |
| 运行状态 | 接收汇总或匿名统计 | 最终数据源 |
| 扩缩容、备份、恢复 | 描述支持能力 | 主责执行 |
| 升级路径 | 定义允许版本图 | 生成升级计划并执行 |
| 删除和回收 | 提供卸载规则 | 主责执行和数据保护 |
| 审计 | 发布和市场操作审计 | 部署和运行操作审计 |

### 7.2 禁止的耦合方式

- 市场保存业务集群 kubeconfig；
- 市场直接调用 Helm 安装到租户集群；
- 平台修改已发布 ReleaseManifest；
- 平台通过共享数据库读取市场数据；
- 市场和平台共用内部表结构；
- 市场使用“latest”标签作为生产部署依据；
- 平台在执行时重新解析未锁定的远端依赖；
- 发布者上传新文件覆盖既有版本；
- 市场不可用导致已运行应用停止；
- 平台把运行时 Secret 回传给市场。

### 7.3 推荐部署关系

#### 模式 A：独立市场 + 单个平台

适用于中小企业。市场和平台分别以 Helm Chart 部署，可共享企业 IAM、OCI Registry 和对象存储，但数据库逻辑独立。

#### 模式 B：集团市场 + 多个平台实例

适用于集团、多地域和多子公司：

```text
集团 HNB App Market
├── HNB Cloud 华东平台
├── HNB Cloud 华南平台
├── HNB Cloud 测试平台
└── HNB Cloud 边缘平台
```

市场统一发布和授权，各平台独立执行和运行。平台可按地域配置制品镜像源，避免跨地域拉取。

#### 模式 C：离线私有市场

中心市场导出签名 Bundle，离线环境导入本地市场。离线平台只访问本地 Registry、对象存储和市场，不依赖互联网。

---

## 8. 市场与平台接口集成

### 8.1 集成方式

采用：

- REST/OpenAPI：查询、授权、发布内容获取；
- OCI Distribution API：镜像、Chart 和 OCI Artifact 获取；
- CloudEvents：发布、撤销、安全状态和授权变化通知；
- Webhook/Callback：部署状态和兼容结果回传；
- OAuth2 Client Credentials 或 mTLS：服务间认证；
- 可选消息总线：高规模环境异步事件分发。

禁止通过数据库直连或共享文件目录集成。

### 8.2 核心 API

#### 市场对平台

```text
GET  /api/v1/catalog/products
GET  /api/v1/catalog/products/{id}
GET  /api/v1/releases/{releaseId}
GET  /api/v1/releases/{releaseId}/manifest
POST /api/v1/releases:resolve
POST /api/v1/entitlements:check
GET  /api/v1/artifacts/{artifactId}/download-token
GET  /api/v1/channels/{channel}/updates
POST /api/v1/offline-bundles:export
```

#### 平台对市场

```text
POST /api/v1/platforms/register
POST /api/v1/deployments/report
POST /api/v1/compatibility/report
POST /api/v1/usage/report
POST /api/v1/security/runtime-findings
GET  /api/v1/platforms/{id}/sync-cursor
```

使用量和运行安全信息默认只回传最小必要字段，禁止回传业务数据、Secret、完整日志或用户内容。

### 8.3 ReleaseManifest

市场返回不可变发布清单：

```yaml
apiVersion: market.hnb.io/v1
kind: ReleaseManifest
metadata:
  productId: postgresql-ha
  releaseId: postgresql-ha-16.4-hnb2
  version: 16.4-hnb2
spec:
  category: database/relational
  channel: stable
  packageType: helm
  artifacts:
    - name: chart
      mediaType: application/vnd.cncf.helm.chart.content.v1.tar+gzip
      uri: oci://registry.example.com/hnb/postgresql-ha
      digest: sha256:...
    - name: database-image
      mediaType: application/vnd.oci.image.manifest.v1+json
      uri: registry.example.com/hnb/postgresql
      digest: sha256:...
  parameterSchemaRef: oci://registry.example.com/hnb/schema/postgresql@sha256:...
  requiredCapabilities:
    - runtime.kubernetes
    - storage.block
    - secret.management
  optionalCapabilities:
    - backup.s3
    - observability.metrics
  compatibility:
    kubernetes: ">=1.30 <1.34"
    architectures: [amd64, arm64]
  security:
    signatureRequired: true
    sbomRefs:
      - oci://registry.example.com/...@sha256:...
  lifecycle:
    install: true
    upgrade: true
    rollback: true
    backup: true
    restore: true
    scale: true
    uninstall: true
```

### 8.4 市场本地缓存

平台维护只读 `MarketCache`：

- 产品摘要和图标；
- 已授权 Release 元数据；
- 参数 Schema；
- 已解析依赖图；
- 撤销列表；
- 最近同步游标；
- 已部署 Release 的完整 Manifest。

市场短时不可用时，平台允许使用已缓存且未撤销、制品可访问的版本继续部署；是否允许由安全策略控制。长期离线环境必须使用离线 Bundle 和本地市场。

### 8.5 事件

至少定义：

- `market.product.published`；
- `market.release.promoted`；
- `market.release.deprecated`；
- `market.release.revoked`；
- `market.security.updated`；
- `market.entitlement.changed`；
- `platform.deployment.started`；
- `platform.deployment.completed`；
- `platform.deployment.failed`；
- `platform.compatibility.reported`。

事件只作为状态变化通知，关键读取仍应通过 API 获取完整权威数据。

---

## 9. 软件包与制品模型

### 9.1 统一包类型

| 包类型 | 市场存储 | 平台执行方式 | 适用场景 |
|---|---|---|---|
| `oci-image` | OCI Registry | Deployment/StatefulSet/Container Engine | 单容器应用和基础组件 |
| `helm-chart` | OCI Registry | Helm Provider | Kubernetes 应用和服务 |
| `operator-bundle` | OCI Registry | Operator Provider | 数据库、中间件和复杂有状态服务 |
| `jar-artifact` | Object Storage | Artifact Runtime Provider | Java 迁移兼容场景 |
| `war-artifact` | Object Storage | Artifact Runtime Provider | 传统 Servlet 应用迁移 |
| `k8s-manifest` | OCI/Object Storage | Manifest Provider | 简单原生资源模板 |
| `config-package` | OCI/Object Storage | Configuration Provider | 配置、Dashboard、告警规则 |
| `data-package` | Object Storage | Init Job/受控迁移 Job | 初始化数据和脚本 |
| `composition` | 市场元数据 | Composition Engine | 多应用、多服务解决方案 |
| `offline-bundle` | Object Storage | Bundle Importer | 隔离网络交付 |

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
    uri: s3://market-artifacts/customer-api/3.2.0/customer-api.jar
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
- 从对象存储登记；
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
Runtime Driver             决定“在 Kubernetes 或容器引擎如何执行”
```

---

## 12. 容器运行目标架构

### 12.1 RuntimeTarget 统一模型

```yaml
apiVersion: platform.hnb.io/v1
kind: RuntimeTarget
metadata:
  name: production-cluster-a
spec:
  type: kubernetes
  region: cn-east-1
  zone: az-1
  connection:
    mode: outbound-agent
  capabilities:
    - runtime.kubernetes
    - artifact.helm
    - artifact.oci-image
    - storage.block
    - network.policy
  labels:
    environment: production
    security-zone: internal
```

核心字段：

- 目标类型；
- 地域和可用区；
- 租户可见范围；
- 连接模式；
- 能力和版本；
- 容量和配额；
- 网络区域；
- 存储能力；
- 安全等级；
- 维护窗口；
- Agent 状态；
- 可部署的 Release 和 Blueprint；
- 健康和可用性。

### 12.2 KubernetesTarget

生产主路径，支持：

- 原生 Workload；
- Helm；
- Operator；
- Gateway API；
- CNI/CSI；
- GPU；
- 备份；
- 可观测；
- 多集群和灾备扩展。

可运行在物理机、虚拟机、私有云、公有云或边缘节点上。

### 12.3 ContainerEngineTarget

用于开发、演示、小规模边缘和受限环境，底层为 Docker、Podman 或兼容 OCI Engine：

- 单容器和容器组；
- Compose/Podman Pod；
- 主机卷和受控目录；
- 主机网络或桥接网络；
- 健康检查；
- 日志和指标采集；
- 滚动替换；
- 本地镜像缓存；
- 资源限制。

限制：

- 不承诺 Kubernetes 等价的调度和自愈；
- 不支持依赖 CRD/Operator 的产品；
- 仅允许市场标记为兼容的 Release；
- 高可用数据库和复杂中间件默认只支持 KubernetesTarget。

### 12.4 ExternalServiceConnector

允许绑定客户已有数据库、中间件或云服务，但其不属于 HNB Cloud 提供的容器化服务：

- 平台不负责创建；
- 只保存 Secret Reference；
- 支持健康探测和服务绑定；
- 可以接入指标和告警；
- 生命周期一般仅为 observe、bind、unbind；
- 不得误显示平台可执行的升级、删除或故障切换。

### 12.5 Runtime Driver

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
- Operator Runtime Driver；
- OCI Container Engine Driver；
- External Connector Driver。

---

## 13. Provider 与插件契约

### 13.1 Provider 分层

1. **Domain Provider**：应用、数据库、中间件、网络、存储、GPU、安全、灾备；
2. **Artifact Provider**：OCI、Helm、JAR/WAR Runtime、Manifest；
3. **Runtime Driver**：Kubernetes 和 Container Engine；
4. **Market Connector**：市场目录、Release、授权和事件同步；
5. **Integration Provider**：IAM、ITSM、CMDB、SIEM、通知；
6. **UI Plugin**：可选页面和组件；
7. **Policy Plugin**：校验、准入和治理规则。

### 13.2 通信原则

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

### 13.3 契约治理

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

### 13.4 认证等级

| 等级 | 要求 |
|---|---|
| Experimental | 基础功能可用，不承诺升级和 SLA |
| Compatible | 通过 API、权限、安装和卸载测试 |
| Production Ready | 通过性能、故障、升级、备份、安全和诊断测试 |
| Certified | 联合兼容认证、长期支持和明确责任边界 |

市场和平台都应展示认证等级，不能把所有第三方包默认视为生产可用。

---

## 14. 平台与市场部署方式

### 14.1 部署形态

| 形态 | 底层基础设施 | 容器编排方式 | 适用场景 |
|---|---|---|---|
| 单机轻量版 | 单台物理机或 VM | Podman/Docker Compose | 开发、演示、功能验证 |
| Kubernetes 标准版 | 物理机或 VM 集群 | Helm/Operator | 企业生产默认 |
| 独立管理集群版 | 专用管理节点 | Kubernetes | 管理多个业务集群 |
| 多地域版 | 多站点物理机/VM/云主机 | 多 Kubernetes 集群 | 集团和容灾 |
| 边缘轻量版 | 边缘物理机或 VM | k3s/K0s 或 Podman | 资源受限场景 |
| 离线版 | 隔离网络 | 本地 Registry + Helm/Compose | 高安全网络 |

无论底层是物理机还是虚拟机，平台与市场组件始终以容器方式运行。

### 14.2 独立部署要求

HNB App Market 与 HNB Cloud Platform：

- 使用独立 Namespace 或独立集群；
- 使用独立数据库 Schema，生产推荐独立数据库实例；
- 支持独立扩缩容和升级；
- 使用不同 ServiceAccount 和网络策略；
- 通过 API Gateway 或服务网关连接；
- 不共享运行时 Secret；
- 可共享企业 IAM、OCI Registry、对象存储和可观测平台；
- 必须支持单独备份和恢复。

### 14.3 统一安装器

提供 `hnbctl`：

```text
hnbctl preflight
hnbctl install platform --profile minimal
hnbctl install platform --profile standard-ha
hnbctl install market --profile standard
hnbctl connect market --endpoint https://market.example.com
hnbctl pack enable container
hnbctl pack enable data
hnbctl market sync
hnbctl upgrade plan
hnbctl upgrade apply
hnbctl backup platform
hnbctl backup market
hnbctl diagnostics collect
```

安装器本身可以作为短生命周期容器运行，也可以发布跨平台静态 CLI；被安装的服务必须是容器。

### 14.4 Release Bundle

```text
hnb-release-bundle/
├── manifest.lock
├── platform-images/
├── market-images/
├── charts/
├── providers/
├── product-releases/
├── jar-war-artifacts/
├── sbom/
├── signatures/
├── vulnerability-db/
├── migrations/
├── compatibility/
└── docs/
```

所有镜像和 Chart 使用 digest 锁定。离线导入必须验证 Bundle 签名、完整性、版本和许可证。

### 14.5 最小依赖策略

| 依赖 | 最小模式 | 生产模式 |
|---|---|---|
| PostgreSQL | 容器单实例 | 容器化高可用 PostgreSQL 或企业外部实例 |
| Valkey | 可不启用或容器单实例 | 容器化高可用或企业外部实例 |
| OCI Registry | 轻量 Registry | Harbor 或企业 OCI Registry |
| 对象存储 | MinIO 单实例容器 | 高可用 S3 兼容对象存储 |
| 搜索 | 数据库全文检索 | OpenSearch/Elasticsearch 可选容器化后端 |
| 消息系统 | PostgreSQL Outbox | NATS/Kafka 可选容器化服务 |
| IAM | 初始本地管理员 | 企业 OIDC/LDAP |
| 可观测后端 | 轻量容器组件 | 企业现有平台或专用集群 |

核心不能因为缺少大型消息队列、搜索集群或 Service Mesh 而无法启动。

---

## 15. 应用与服务交付闭环

### 15.1 统一对象

```text
MarketProduct
→ ProductRelease
→ ServiceBlueprint / ApplicationBlueprint
→ DeploymentRequest
→ ExecutionPlan
→ ApplicationInstance / ServiceInstance
→ Endpoint
→ ServiceBinding
→ Operation
```

### 15.2 用户流程

```text
进入统一应用市场
→ 按应用/数据库/中间件分类或标签检索
→ 选择 Product 和 Release
→ 平台检查授权
→ 选择租户、项目、环境和运行目标
→ 动态生成参数表单
→ 解析组合依赖
→ 预检配额、网络、存储、GPU、镜像和安全
→ 审批或自动批准
→ 固化 ExecutionPlan
→ 执行部署 DAG
→ 健康验证
→ 创建 Endpoint、Secret Reference 和服务绑定
→ 进入监控、备份、升级和审计
```

### 15.3 生命周期能力协商

| 能力 | 市场声明 | 平台行为 |
|---|---|---|
| 安装 | Release 是否可安装 | 生成并执行计划 |
| 扩缩容 | Provider 是否支持 | 动态展示规格和预检 |
| 升级 | 允许的版本边和迁移要求 | 生成升级计划和回滚点 |
| 回滚 | 是否支持应用/Chart 回滚 | 执行回滚，数据回滚单独处理 |
| 备份 | 是否有 Backup Provider | 创建策略和恢复点 |
| 恢复 | 支持的恢复方式 | 预检、恢复和验证 |
| 故障切换 | 是否支持 | 受控执行、隔离和审计 |
| 删除 | 卸载规则和数据策略 | 保护数据、确认和回收 |

### 15.4 应用实例与市场版本关系

每个实例必须记录：

- Product ID；
- Release ID；
- ReleaseManifest 摘要；
- CompositionRelease 摘要；
- ExecutionPlan ID；
- 所有制品 digest；
- 当前配置版本；
- Provider 版本；
- RuntimeTarget；
- 安全证明状态；
- 漂移状态；
- 升级候选版本。

禁止只记录镜像 tag 或 Chart version 而无法复现完整部署。

---

## 16. 简单易用设计

### 16.1 统一入口

用户在 HNB Portal 中访问应用市场，前端可以采用微前端或独立路由集成，但后端仍保持独立系统：

```text
HNB Portal
├── 应用市场
├── 我的申请
├── 应用实例
├── 数据库服务
├── 中间件服务
├── 运行环境
├── 运维中心
└── 安全与审计
```

推荐通过统一身份和导航实现“一个入口”，不要通过 iframe 嵌入完整市场控制台作为长期方案。

### 16.2 三层模式

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
- 组合依赖覆盖。

### 16.3 动态表单

市场提供产品参数 Schema，平台注入环境上下文：

- 市场负责业务和产品参数；
- 平台负责集群、网络、存储、配额、安全和 Secret 参数；
- 重名参数通过命名空间区分；
- Secret 字段不得回传市场；
- 参数默认值按市场默认、平台策略、租户策略、环境策略依次覆盖；
- 提交前展示最终差异和影响预览。

### 16.4 场景化向导

- 发布 Java 应用；
- 部署标准 Web 三层应用；
- 创建 PostgreSQL 高可用实例；
- 创建 Kafka 集群；
- 从市场安装 API 网关；
- 部署带数据库、缓存和网关的组合应用；
- 导入离线市场包；
- 将应用从测试渠道升级到稳定渠道。

---

## 17. 安全与软件供应链

### 17.1 双重安全门禁

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

### 17.2 信任模型

- 发布者签名产品 Release；
- 市场签名已审核发布清单；
- 平台验证发布者签名和市场签名；
- 平台生成并签名 ExecutionPlan；
- Agent 只执行来自受信平台的计划；
- 执行结果和证据进入审计；
- 高安全环境要求镜像仓库、市场和平台证书链可独立轮换。

### 17.3 JAR/WAR 安全

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

### 17.4 多租户隔离

- 市场内容可见性和平台资源权限分别计算；
- 拥有市场查看权限不代表拥有部署权限；
- Entitlement、租户角色、项目角色和平台策略共同决定部署；
- 市场不接收租户运行 Secret；
- 平台审计记录 Product/Release 和制品摘要；
- 跨租户共享实例需显式授权和期限控制。

### 17.5 撤销处置

当 Release 或制品被撤销：

1. 市场发布 `revoked` 事件；
2. 平台停止新部署和新扩容；
3. 扫描已有实例；
4. 按风险等级告警、隔离或要求升级；
5. 生成受影响租户、项目和实例清单；
6. 保留审计证据；
7. 不得未经策略授权自动删除有状态实例。

---

## 18. 可观测、运维与灾备

### 18.1 平台与市场可观测

统一采用 OpenTelemetry，但存储后端可复用企业系统：

- API 延迟和错误；
- Market 同步游标和失败；
- 制品下载和镜像拉取；
- Release 解析耗时；
- Operation 队列、重试和失败；
- Agent 连接；
- Provider 健康；
- 部署步骤耗时；
- 市场索引、扫描和镜像同步任务；
- 审计和安全事件。

### 18.2 Operation 状态

```text
Pending
→ Validating
→ WaitingApproval
→ ResolvingRelease
→ Planning
→ WaitingDependency
→ Running
→ Verifying
→ Succeeded
```

异常状态：

```text
Retrying
Compensating
Paused
Cancelled
Failed
NeedsAttention
```

每个 Operation 必须记录：

- Tenant、Project 和 Environment；
- Product/Release/Composition ID；
- 制品摘要；
- ExecutionPlan；
- 步骤、输入、输出和证据；
- 重试和补偿；
- 操作者；
- 审批；
- 审计关联；
- 市场回调状态。

### 18.3 升级治理

升级由市场和平台协同：

- 市场定义 `fromRelease -> toRelease` 合法升级边；
- 市场提供变更说明、迁移要求、备份要求和不兼容项；
- 平台检查运行环境、数据状态、维护窗口和配额；
- 平台生成升级计划和回滚点；
- 平台执行并验证；
- 结果回传市场形成兼容性数据。

### 18.4 灾备边界

- 市场灾备：元数据数据库、对象存储、Registry 和签名材料；
- 平台灾备：控制面元数据、审计、Secret 引用和 Operation 状态；
- 应用灾备：工作负载、数据、流量和依赖；
- 市场不可用不代表应用不可用；
- 仅复制 Helm 和镜像不能宣称业务双活；
- 多站点必须分别规划 Registry 镜像、对象存储复制和市场元数据恢复。

---

## 19. 工程架构与代码仓库

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
├── providers/
│   ├── artifact-oci/
│   ├── artifact-helm/
│   ├── artifact-runtime-java/
│   ├── database-postgresql/
│   ├── middleware-kafka/
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

### 19.1 技术原则

- 平台 API、市场 API、Controller 和 Agent 优先 Go；
- 前端 TypeScript；
- 热点传输和大文件处理可按压测使用 Rust；
- 全部组件构建为多架构 OCI 镜像；
- 数据库迁移版本化；
- API、事件和 Manifest 采用 Schema First；
- 市场与平台禁止共享内部 Go struct 作为外部契约；
- 使用生成 SDK 保持解耦；
- 所有 Provider 通过契约测试。

### 19.2 架构治理

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
- 数据迁移与回滚策略。

---

## 20. 版本范围与实施路线

### 20.1 阶段 0：架构基线与验证

目标：验证容器化硬约束和市场/平台边界。

必须完成：

- ReleaseManifest 原型；
- CompositionRelease 原型；
- ExecutionPlan 原型；
- 市场与平台独立数据库和 API；
- OCI Registry + 对象存储验证；
- Helm OCI 验证；
- JAR/WAR 标准运行时容器验证；
- Agent mTLS；
- 依赖 DAG 和补偿验证；
- 离线 Bundle 验证。

退出标准：

- 一个 JAR 应用能从市场发布并以容器方式部署；
- 一个 Helm 数据库包能发布并部署；
- 一个三组件组合能够按依赖部署并生成绑定；
- 市场无业务集群凭据；
- 所有执行使用 digest 锁定。

### 20.2 MVP：市场与平台最小闭环

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
- Java Web + PostgreSQL 的组合应用。

### 20.3 V1：生产可用

- 高可用平台和市场部署；
- 企业 OIDC/LDAP；
- 完整发布审核和渠道晋级；
- 多租户授权；
- 镜像和 Chart 镜像同步；
- Provider 认证；
- 升级和回滚；
- 数据备份和恢复；
- 应用运行时安全；
- 离线部署；
- PostgreSQL + 一个消息中间件完整生命周期；
- 生产容量、性能和故障测试。

### 20.4 V1.5：生态扩展

- ContainerEngineTarget；
- 更多数据库和中间件；
- Operator Bundle；
- 上游市场同步；
- ISV 发布者门户；
- 许可证和订阅；
- 多架构镜像；
- GPU 能力包；
- 多集群部署；
- 应用组合升级。

### 20.5 V2：集团与高级能力

- 集团市场服务多个平台实例；
- 多地域市场镜像和容灾；
- Karmada；
- 全局流量；
- 高级灾备；
- 兼容性遥测和智能推荐；
- 行业解决方案市场；
- Partner/ISV 认证体系；
- 计量和商业授权；
- AIOps 和自动修复扩展。

---

## 21. 团队与组织建议

### 21.1 最小核心团队

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

### 21.2 领域责任

- 市场团队对 Product、Release 和 Artifact 的权威性负责；
- 平台团队对 ExecutionPlan 和运行状态负责；
- Provider 团队对具体产品生命周期负责；
- 安全团队对信任链和门禁负责；
- SRE 对容器化交付、升级和离线包负责；
- 产品团队确保普通用户不需要理解底层 Chart 和 CRD。

---

## 22. 测试与验收体系

### 22.1 测试层级

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
- 灾备演练。

### 22.2 V3.3 专项验收

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
| CMP-ACC-01 | 至少三个不同包可组成一个 CompositionRelease |
| CMP-ACC-02 | 平台能执行依赖 DAG、输出绑定和失败补偿 |
| INT-ACC-01 | 市场和平台只通过公开 API/事件集成 |
| INT-ACC-02 | 市场中断不影响已部署应用运行 |
| INT-ACC-03 | 平台保存完整 ReleaseManifest 和 ExecutionPlan 以支持审计和复现 |
| SEC-ACC-01 | 发布时和部署时均验证签名、SBOM 和安全策略 |
| OFF-ACC-01 | 离线 Bundle 可在无互联网环境完成导入和部署 |

### 22.3 质量门禁

进入生产发布前：

- P0 API 无未解决兼容破坏；
- 所有镜像、Chart 和制品有摘要和签名；
- 关键镜像无未接受的严重漏洞；
- 组合安装、升级、回滚和卸载通过；
- 市场与平台接口通过双向兼容测试；
- Tenant 越权测试通过；
- Agent 断连和恢复通过；
- 市场不可用和 Registry 短时不可用测试通过；
- 数据保护型卸载测试通过；
- 离线升级通过；
- 审计链完整。

---

## 23. 资源与容量建议

### 23.1 最小开发/演示

单机物理机或 VM，Podman/Docker Compose：

- 8 vCPU；
- 16 GB 内存；
- 200 GB SSD；
- 平台、市场、PostgreSQL、Registry、MinIO 采用轻量配置；
- 不部署完整日志和搜索后端。

### 23.2 小型生产

三节点 Kubernetes 管理集群：

- 每节点 8-16 vCPU；
- 每节点 32-64 GB 内存；
- 外部或高可用 PostgreSQL；
- 独立 OCI Registry 和对象存储；
- 市场与平台使用独立 Namespace 和资源配额；
- 业务应用部署到独立工作集群或节点池。

### 23.3 中大型生产

- 独立管理集群；
- 市场和平台可分别独立节点池；
- PostgreSQL、Registry、对象存储和搜索独立高可用；
- 区域镜像缓存；
- Agent Gateway 多副本；
- 市场索引和同步 Worker 独立扩展；
- 可观测和安全后端独立；
- 业务集群按租户或环境分层。

### 23.4 性能指标建议

最终指标需通过压测确认，初始目标：

- 市场 Catalog P95 查询 < 500 ms；
- 平台核心资源查询 P95 < 500 ms；
- Release 解析 P95 < 2 s，不含制品传输；
- 10,000 Product、100,000 Release 可检索；
- 单平台支持 100 个集群级运行目标；
- Operation 状态更新不因市场不可用阻塞；
- 市场事件至少一次投递，平台幂等消费；
- 1 GB JAR/WAR/Bundle 支持分片、校验和断点续传；
- 镜像拉取和大文件传输不经过平台 API 进程转发。

---

## 24. 主要风险与应对

| 风险 | 表现 | 应对 |
|---|---|---|
| 市场变成第二个平台 | 市场开始管理集群和运行状态 | 强制职责矩阵和禁止集群凭据 |
| 平台变成制品仓库 | 大文件经过平台 API | 制品直连 Registry/Object Storage |
| JAR/WAR 破坏容器化原则 | 主机直接运行 Java | 仅支持 OCI 镜像或标准运行时容器 |
| 组合编排过度复杂 | 自研通用工作流/BPM | 只支持部署 DAG、Saga 和受控生命周期 |
| 超大 Helm Chart | 所有组件绑死在一个 Chart | 平台组合多个独立 Release |
| 版本不可复现 | 使用 latest 或未锁依赖 | digest、manifest.lock、不可变 Release |
| 市场与平台强耦合 | 共享数据库和内部代码 | API First、事件契约、独立升级 |
| 市场故障阻塞业务 | 运行时依赖市场 | 平台缓存 Manifest，数据面自治 |
| 供应链风险 | 上传恶意包或高危镜像 | 双重门禁、签名、SBOM、撤销机制 |
| 外部 Registry 不稳定 | 部署时拉取失败 | 区域镜像、预拉取、离线 Bundle |
| Provider 能力不一致 | 页面有按钮但无法执行 | 生命周期能力协商和认证等级 |
| 卸载误删数据 | 组合补偿删除数据库 | 数据保护默认、人工确认、保留策略 |
| 首期范围失控 | 同时支持过多包和服务 | MVP 只做 OCI、Helm、JAR Runtime 和三个示例产品 |

---

## 25. V3.3 新增需求清单

### 25.1 容器化要求

| 编号 | 优先级 | 需求 |
|---|---|---|
| CTN-01 | P0 | HNB Cloud 平台组件必须全部以 OCI 容器方式运行。 |
| CTN-02 | P0 | HNB App Market 组件必须全部以 OCI 容器方式运行。 |
| CTN-03 | P0 | 平台交付的应用、数据库和中间件不得通过 RPM、DEB、systemd 或裸二进制直接运行。 |
| CTN-04 | P0 | 物理机和虚拟机仅作为 Kubernetes 或 OCI 容器运行底座。 |
| CTN-05 | P0 | JAR/WAR 部署必须由 OCI 镜像或标准运行时容器承载。 |
| CTN-06 | P1 | 平台镜像和主要服务镜像支持 amd64/arm64 多架构。 |
| CTN-07 | P0 | 所有生产部署必须以镜像 digest 固定版本。 |

### 25.2 应用市场

| 编号 | 优先级 | 需求 |
|---|---|---|
| MKT-01 | P0 | 提供独立部署、通过 API 与平台集成的统一应用市场。 |
| MKT-02 | P0 | 市场支持应用、数据库、中间件及其它平台能力分类。 |
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

### 25.3 多包编排

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

### 25.4 平台市场集成

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

### 25.5 制品供应链

| 编号 | 优先级 | 需求 |
|---|---|---|
| ART-01 | P0 | 大文件必须存储于 OCI Registry 或对象存储，市场数据库只保存引用。 |
| ART-02 | P0 | 所有制品必须具有 SHA-256 摘要。 |
| ART-03 | P0 | 生产渠道必须完成签名、SBOM 和安全扫描。 |
| ART-04 | P0 | JAR/WAR 必须绑定不可变运行时镜像摘要。 |
| ART-05 | P0 | Chart 引用镜像必须生成锁文件。 |
| ART-06 | P1 | 支持制品复制、镜像同步、断点续传和完整性校验。 |
| ART-07 | P1 | 支持 Release 撤销后的实例影响分析。 |

---

## 26. 推荐首个可交付产品组合

首个版本建议只交付以下组合，验证完整闭环而非追求数量：

```text
HNB Cloud Core Pack
+ HNB App Market
+ KubernetesTarget
+ OCI Image Provider
+ Helm Provider
+ Java Artifact Runtime Provider
+ PostgreSQL Service Provider
+ Valkey 或 RabbitMQ Service Provider
+ 基础 Network/Storage/Secret
+ 基础指标、日志和审计
```

市场预置四个产品：

1. **Java Web 应用模板**：支持 OCI 镜像和 JAR 标准运行时两种发布方式；
2. **PostgreSQL 高可用**：Helm/Operator 容器化交付；
3. **Valkey 或 RabbitMQ**：容器化服务；
4. **Java Web 标准解决方案**：Java 应用 + PostgreSQL + 缓存/消息 + Gateway + 监控。

首个商业可交付闭环必须演示：

```text
发布制品
→ 审核并进入 stable
→ 分类和标签检索
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
→ 完整审计
```

---

## 27. 最终落地原则

1. **全部容器化**：平台和平台提供的服务不走传统主机软件安装路径。
2. **基础设施与运行形态分离**：物理机、虚拟机和云主机是底座，OCI 容器是统一运行形态。
3. **市场管产品，平台管运行**：市场不越权部署，平台不越权修改发布内容。
4. **发布不可变**：Release、Chart、镜像和 JAR/WAR 全部以摘要锁定。
5. **组合而非绑死**：应用、数据库、中间件和运维能力通过 CompositionRelease 与 ExecutionPlan 组合。
6. **Helm 是执行工具，不是平台架构**：复杂组合和生命周期由平台统一编排。
7. **最小核心**：未启用的能力不安装、不占资源、不增加故障面。
8. **Provider 可替换**：数据库、网络、存储、安全和可观测实现不写死在内核。
9. **数据面自治**：市场和控制面故障不影响已运行服务。
10. **双重安全门禁**：市场负责发布供应链，平台负责部署和运行安全。
11. **先闭环后扩展**：先做好一个应用、一个数据库、一个中间件和一个组合方案。
12. **可复现、可升级、可回滚**：每个实例都能追溯到完整 ReleaseManifest、ExecutionPlan 和制品摘要。

