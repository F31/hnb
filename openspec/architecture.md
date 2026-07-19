# HNB Cloud 架构与技术栈基线

## 文档定位

本文整理 HNB Cloud 当前 OpenSpec 架构、开发语言和技术框架状态。唯一方案基线为 `doc/HNB_Cloud_OpenSpec_实施基线_V3_8_6.md`；主规格定义可验收行为，具体语言、框架和产品选型必须在对应 change 的 `design.md` 与版本化 BOM 中批准。

状态含义：

- **冻结**：V3.8.6 已批准的架构边界或标准，后续 change 不得绕过。
- **MVP 指定**：已进入阶段 0/MVP backlog，但具体发行版、Controller 或实现仍需在 change 中锁定。
- **可替换**：必须通过 Provider、Profile 或 Conformance 接入，不得编译进 HNB Core。
- **T2/T3 可选**：不进入 MVP 强制依赖，达到阶段门槛后再交付。
- **待选型**：V3.8.6 未冻结，不得根据历史文档或个人偏好默认决定。

## 总体架构

```text
Vue Web Portal / Go CLI / generated SDK
             |
             v
  OpenAPI / Protobuf / Event / Manifest
             |
             v
+----------------------------------------------------------+
| HNB Core: Tenant Context, Operation/ExecutionPlan,        |
| Read Model, Resource Graph, Provider Registry, Audit      |
+----------------------------------------------------------+
       |                 |                    |
       v                 v                    v
 App Market        Artifact Storage     AI Extension Plane
 independent DB    OCI + Local/PVC/S3   optional, no bypass
       |                 |                    |
       +-----------------+--------------------+
                         |
                  Provider contracts
                         |
       +-----------------+--------------------+
       v                 v                    v
 KubernetesTarget  ContainerEngineTarget  EdgeRuntimeTarget
 Gateway API       T2 optional             KubeEdge, T3 POC
```

以下边界已经冻结：

- HNB Core 采用微内核加 Provider/CapabilityPack 架构，具体 CNI、CSI、数据库、中间件、Gateway、AI Runtime 和 Edge 实现不得编译进内核。
- 应用市场、统一制品存储、运行治理、AI Extension Plane 四个逻辑平面解耦，不共享内部数据库。
- `Release/CompositionRelease -> ExecutionPlan -> Operation` 是唯一运行目标写入链路。
- 公共契约 Schema First；跨进程、跨平面通过 OpenAPI、Protobuf、事件或 Manifest 交互并生成 SDK。
- OCI 优先、内容寻址、生产固定 digest；Market/Platform API 不代理大文件数据面。
- Gateway API 是 KubernetesTarget 的首选规范，不等同于任何具体 Gateway 产品。
- AI Extension Plane 可独立启停，不是事实源且不得形成执行旁路。
- Edge Pack 不是第五平面；云为权威、边可自治，KubeEdge 节点不重复部署 HNB Agent。

## 开发语言与框架

| 组件 | 开发语言 | 应用框架/SDK | 当前结论 | 选型约束 |
|---|---|---|---|---|
| Web Portal | TypeScript 7.0.2 | Vue 3.5.40、Vite 8.1.5、`@vitejs/plugin-vue` 6.0.8 | 冻结为纯 Web Portal，不引入 Tauri 或桌面客户端 | 必须支持 CapabilityPack 动态界面、Schema 驱动表单、简单/标准/专家模式和可恢复向导；BOM 精确锁定版本，不使用浮动 `latest` |
| HNB Core / Platform API | Go | Web/RPC 框架待对应 change 决定 | Go 是控制面默认语言 | 必须保持微内核边界、无状态扩展、Operation 唯一写入口和 Read Model 查询路径；Rust 不得通过 FFI 成为 Core 编译依赖 |
| HNB App Market | Go | Web/持久化框架待对应 change 决定 | 与 Core 统一语言，降低阶段 0/MVP 运维与开发复杂度 | 必须独立部署、升级、备份恢复并使用独立数据库，不得访问业务集群写 API |
| Provider / Runtime Driver | Go 优先，契约语言无关 | OpenAPI/Protobuf/Manifest 生成 SDK | Kubernetes 与标准 Service Provider 默认使用 Go；其他语言实现不得成为 Core 编译依赖 | T1 及以上外部 Provider 优先独立进程或容器运行，并通过统一生命周期契约和 Conformance |
| Cluster Agent | Go | Kubernetes 客户端与版本化通信契约 | Go 用于 Kubernetes 能力发现、状态上报和 Operation 执行 | 中心 Kubernetes/Container Engine 目标使用 Agent 主动连接和 mTLS；不得要求目标暴露公网管理端口 |
| CLI | Go | 由公共 Schema 生成客户端能力 | 单文件跨平台交付，具体 CLI 框架待 design 决定 | CLI 与 Portal 使用相同公共契约，不得绕过 Operation 直接写运行目标 |
| AI Gateway / Copilot / AIOps | Go 控制面；Rust 数据面按性能证据选用 | AI 实现框架待选型 | AI 数据面协议已定义；Rust 仅用于自研高吞吐流式代理等实测热路径 | AI Gateway 支持 HTTP、SSE、WebSocket 和 OpenAI-compatible；Copilot/AIOps 写操作必须转换为 Operation |
| Edge Provider / Device Mapper | Go 控制面；Rust Mapper 可选 | KubeEdge 接口与容器化 Mapper | T3，先 POC 后 Production；资源受限或协议安全敏感 Mapper 可采用 Rust | Mapper 必须容器化并经过市场门禁；KubeEdge 通过 CloudHub-EdgeHub 通道接入 |
| OpenSpec 仓库治理 | JavaScript（Node.js） | Node.js 标准库、OpenSpec CLI | 仅适用于规格治理，不是平台运行时选型 | 当前 change 设计使用 Node.js 调用固定版本 OpenSpec 并执行仓库语义检查 |

Go 和 Vue 技术栈已经作为默认实现方向批准，但具体 Web/RPC、ORM、状态管理、UI 组件库和测试框架仍须由对应 change 决定。不得把 Java/Spring、Python AI 编排或任一未批准框架从历史文档直接写入实现任务。任何新框架都必须记录替代方案、兼容性、供应链、团队能力、性能预算、升级和回滚依据。

### Go 与 Rust 职责边界

```text
Vue 3 + TypeScript + Vite
            |
     OpenAPI / SSE / WebSocket
            |
            v
+--------------------------------------------------+
| Go 控制面                                       |
| Core / Market / Operation / Read Model / Agent  |
| Provider / Runtime Driver / CLI / Controllers   |
+--------------------------------------------------+
            |
   Protobuf / OpenAPI / Event / Manifest
            |
            v
+--------------------------------------------------+
| Rust 独立数据与性能组件                         |
| Artifact Transfer / Backup Transfer / Collector |
| Edge Buffer / Bundle Tool / verified hot paths  |
+--------------------------------------------------+
```

Go 适用于业务规则、控制循环、状态机、事务、Kubernetes API 集成和运维工具：

- HNB Core、Platform API、ExecutionPlan/Operation Engine、Read Model、Resource Graph；
- Identity/Tenant/Audit、App Market、Provider Registry、Conformance Harness；
- Kubernetes Runtime Driver、Cluster Agent、Gateway Provider 控制面；
- PostgreSQL、Valkey、RabbitMQ 等 Service Provider；
- Secret/KMS Provider、Installer、Upgrade Controller、CLI 和常规事件 Relay。

Rust 只用于高吞吐、低内存、资源受限或解析不可信输入的独立组件：

- OCI/S3 大文件上传下载、断点续传、摘要校验、压缩和加密；
- 数据库备份恢复传输、跨站点复制和高吞吐数据搬运；
- 日志与遥测采集、本地有界缓冲、边缘断连补传；
- 离线 Bundle 打包、导入和不可信制品解析；
- 资源受限或安全敏感的 Device Mapper；
- 只有在 Go 基线未满足批准的 P95/P99、吞吐、CPU 或内存预算时，拆出的 AI 流式代理或事件热路径。

Rust SHALL NOT 用于普通 CRUD API、Portal BFF、Operation 状态机、租户权限业务逻辑或常规 Kubernetes Controller。Go 与 Rust 之间 SHALL 通过版本化 OpenAPI、Protobuf、事件或 Manifest 通信，不使用 FFI，不共享内部语言结构体或内部数据库。

Rust 组件的引入必须满足以下门槛：

1. 先提供同一环境下的 Go 基线和可复现性能数据。
2. 明确未达到的性能或资源预算以及 Rust 实现的退出指标。
3. 作为独立进程或容器交付，具有健康检查、资源上限、遥测和故障隔离。
4. 通过公共契约兼容测试、供应链检查、升级和回滚验证。

### 长任务与异步消息架构

平台自身的应用与组合交付、数据库备份恢复和切换、制品扫描签名、Provider 安装升级、平台升级、灾备、边缘 OTA/离线对账及 AI 模型部署均属于长时间或多步骤 Operation。默认采用 **PostgreSQL Operation Store + Transactional Outbox + NATS JetStream**：PostgreSQL 保存唯一业务事实，JetStream 只传递执行意图、已提交领域事件和非权威通知。

```text
Portal / CLI
     |
     v
Platform API
     |
     | same transaction
     v
+-----------------------------------------+
| PostgreSQL Operation Store              |
| Operation / Step / Checkpoint / Lease   |
| Idempotency / Approval / Outbox         |
+-----------------------------------------+
                   |
              Outbox Relay
                   |
                   v
+-----------------------------------------+
| NATS JetStream                          |
| commands / domain events / notification |
+-----------------------------------------+
       |              |              |
       v              v              v
 Operation Worker  Projector   Audit / Notification
       |
       v
 Provider / RuntimeTarget
```

消息架构遵循以下约束：

- 可靠命令和领域事件使用 JetStream 文件持久化、Durable Consumer、显式 ACK、有限重试、背压和受控重放；Core NATS 不承载可靠 Operation 命令。
- 采用至少一次投递与业务幂等，不承诺端到端 exactly-once；Worker 执行前必须读取权威状态并获取带 fencing token 的 Lease。
- API 在同一数据库事务内提交业务事实和 Outbox；Relay 收到 JetStream 持久化确认后才标记发布完成。
- NATS 不可用时，已提交 Operation 保持 `Queued` 且 Outbox 可重试；恢复后续投，不允许绕过 Operation 直接执行。
- Portal 只通过鉴权后的 Platform API SSE/WebSocket 或 Read Model 获取进度，不直接访问 NATS。
- 消息携带 Tenant、Operation、Step、Resource、Correlation、Causation、Idempotency 和 Schema Version，只引用 OCI/S3/Secret 数据，不携带明文 Secret、kubeconfig 或大文件正文。
- 命令、领域事件和通知使用独立的版本化 Subject、Stream、Consumer、权限和保留策略；全局顺序不是正确性前提。
- Minimal 使用单节点文件持久化并明确非 HA；Lite HA 及以上使用至少三节点多数派集群，并验证单 Pod、单节点和 Leader 故障。

Kafka、RabbitMQ、Temporal 和 Valkey/Redis Queue 不进入 MVP 默认依赖。阶段 0 可用 PostgreSQL Outbox/Worker 轮询验证状态机，但 MVP 完成迁移后不长期维护两套生产调度路径。完整行为和实施设计由 `adopt-nats-jetstream-messaging` change 管理。

### 监控告警与通知架构

统一遥测负责产生指标、日志、链路和领域事件，Alert/Notification 服务负责归一化、生命周期、降噪、路由、送达和审计。监控产品与规则引擎可替换，不把 Prometheus、Alertmanager 或其他产品编译进 HNB Core。

```text
Metrics / Logs / Traces / Domain Events
                  |
      Rule Engine / Source Adapters
                  |
                  v
         Alert Normalizer
                  |
                  v
+-------------------------------------------+
| PostgreSQL Alert Store                    |
| Rule / Instance / Silence / Route         |
| Contact / Channel / Job / Delivery        |
| Transactional Outbox                      |
+-------------------------------------------+
                  |
             NATS JetStream
                  |
        Notification Dispatcher
        |          |          |
        v          v          v
   Portal API    Email      Webhook      SMS Provider
    SSE/WS       Worker      Worker       (T2 optional)
        |
        v
 Vue Alert Center / Message Bell
```

告警通知遵循以下约束：

- PostgreSQL Alert Store 保存 AlertRule、AlertInstance、Silence、NotificationPolicy、ContactGroup、Channel、Job 和 DeliveryRecord 权威状态；NATS 只分发通知任务和领域事件。
- AlertInstance 使用 Pending、Firing、Acknowledged、Silenced 和 Resolved 生命周期；确认或静默不代表来源故障已经恢复。
- fingerprint 去重、时间窗口聚合、父子抑制、防抖、维护窗口、重复间隔和有期限静默用于控制告警疲劳，所有抑制均可解释和审计。
- 路由可按 Tenant、Project、Environment、Severity、Source、Resource、Label、值班时间和用户偏好匹配；Critical 保留组织默认安全路由。
- Portal 告警中心、Email/SMTP 和通用签名 Webhook 属于 T1；SMS 属于 T2 可替换 Provider，未安装时不影响 T1 闭环。
- Portal 通过鉴权后的 API SSE/WebSocket 或 Read Model 提供通知铃铛、未读计数、筛选、确认、认领、静默和关联资源跳转，不直接访问 NATS。
- Delivery 状态按渠道能力表达：Portal 可记录 Delivered/Read；Email 通常只记录 Accepted；SMS/Webhook 仅在受认证回执存在时记录 Delivered。
- SMTP、Webhook 和 SMS 凭据只使用 SecretReference；外部通知默认发送脱敏摘要和受控 Portal 链接，不携带 Secret、Token、kubeconfig 或原始日志正文。
- 每个渠道独立限速、超时、熔断、有限重试和失败隔离；通知系统自身故障通过不依赖 Dispatcher 的健康信号和 Runbook 暴露。

完整行为和实施设计由 `bootstrap-alert-notification` change 管理。

## 技术框架与组件矩阵

| 架构域 | 技术、标准或组件 | 状态 | 说明 |
|---|---|---|---|
| Web Portal | Vue 3.5.40、TypeScript 7.0.2、Vite 8.1.5、`@vitejs/plugin-vue` 6.0.8 | 冻结 | 仅提供浏览器 Web Portal；不引入 Tauri；Node.js 构建环境须满足 Vite 8 的 `^20.19.0 || >=22.12.0` |
| 控制面实现 | Go | 冻结默认语言 | Core、Market、Operation、Read Model、Agent、Provider、Runtime Driver 和 CLI 优先采用 Go；具体 Go 版本在首个实现 change 的 Core BOM 锁定 |
| 性能组件实现 | Rust | 条件选用 | 仅在大文件传输、备份、采集、边缘缓冲或实测热路径使用；具体 Rust 版本在首次引入 change 的 BOM 锁定 |
| 公共 API 与事件 | OpenAPI、Protobuf、版本化事件、Manifest、生成式 SDK | 冻结 | Schema First；同一主版本向后兼容；写命令携带 IdempotencyKey、Correlation ID 和期望版本 |
| 状态与查询 | PostgreSQL Operation Store、Operation、ExecutionPlan、Read Model、Resource Graph、Transactional Outbox | 冻结架构 | PostgreSQL 保存 Operation 权威状态；查询路径不得实时遍历全部 RuntimeTarget；具体 PostgreSQL 版本和 HA 实现由 BOM 固定 |
| 异步消息 | NATS JetStream | T1 默认 | 承载内部命令、领域事件和通知；不作为业务事实源；Minimal 单节点持久化，Lite HA 及以上至少三节点多数派集群 |
| 容器运行 | KubernetesTarget | MVP 指定 | MVP 交付 OCI/Helm Driver；Kubernetes 发行版和版本在 `bootstrap-kubernetes-runtime` design/BOM 决定 |
| 单机容器运行 | Podman/containerd 类 ContainerEngineTarget | T2 可选 | `add-container-engine-runtime` 决定默认实现与版本，V3.8.6 未冻结具体产品 |
| 边缘运行 | KubeEdge CloudCore/Edge Provider | T3 POC | 先验证断连自治，再进入 NodeGroup、OTA、Device Mapper 和离线 Bundle 的生产化 change |
| 服务入口 | Kubernetes Gateway API Standard Channel | 冻结规范 | 默认 Gateway Provider 尚未确定；第二 Provider 通过相同 Conformance Harness 认证；Ingress 仅用于兼容迁移 |
| 制品 | OCI Registry Endpoint、ArtifactDescriptor、OCI Referrer 或等价关系 | 冻结 | 镜像、Chart、JAR/WAR、Operator、模型、SBOM 和 Bundle 使用统一逻辑入口与 digest |
| 制品后端 | Local、PVC、S3 Profile | 可替换 | Minimal 不强制对象存储；Lite HA 及以上使用共享权威后端；具体 Registry/S3 产品待 BOM 决策 |
| 部署执行 | Helm 可作为节点执行器 | 可替换 | Helm 不是 CompositionRelease 的唯一编排机制；Kustomize 或其他执行器未在 V3.8.6 中冻结 |
| 首批数据库服务 | PostgreSQL Service Provider | MVP 指定 | 指的是平台管理的首批服务产品，不等于平台元数据库已选定 PostgreSQL；版本和 Operator 待 change 决策 |
| 首批中间件服务 | Valkey 或 RabbitMQ 至少一个 | MVP 指定 | 两者并未同时强制；由各自 blueprint change 和 BOM 决定首发组合 |
| 后续中间件 | Kafka、RocketMQ、MQTT | 可替换/后续 | 作为后续可选 Service Provider；Minimal 明确不强制 Kafka |
| Secret 与 KMS | Kubernetes Secret、Vault、企业 KMS/HSM、云密钥服务 | 可替换 | 通过 Secret/KMS Provider 接入；公共 API、计划、事件和日志仅使用 SecretReference 或短期令牌 |
| 软件供应链 | Cosign、SBOM、OCI Referrer 或等价关系 | 冻结 | 发布与部署执行双重校验；漏洞扫描器、许可证扫描器和准入产品尚未指定 |
| 可观测性 | 结构化指标、日志、链路、事件；OTel 上下文 | MVP 指定 | `bootstrap-observability-baseline` 决定 Collector 和指标/日志/链路后端，V3.8.6 未冻结 Prometheus、Loki、Tempo 等产品 |
| 告警与通知 | PostgreSQL Alert Store、Transactional Outbox、NATS JetStream、Portal SSE/WebSocket | T1 默认 | 统一 Alert 生命周期、降噪、路由和送达记录；监控来源与规则引擎可替换 |
| T1 通知渠道 | Portal、Email/SMTP、通用签名 Webhook | T1 默认 | 渠道独立重试和隔离；SMTP/Webhook 凭据使用 SecretReference；Portal 不直连 NATS |
| SMS 通知 | 可替换 SMS Provider | T2 可选 | 声明区域、模板、签名、费用、配额、回执和数据驻留能力；未安装时不影响 T1 |
| AI 接入 | HTTP、SSE、WebSocket、OpenAI-compatible | T2 可选 | 外部模型 Connector、AI Gateway 和只读 Copilot 进入 `add-ai-access-pack`；模型运行框架与向量库待选型 |
| 多集群治理 | Karmada、DNS/GSLB、全局流量 | T3 可选 | 不进入 MVP；同一节点不得与 KubeEdge 管理边界冲突 |
| 离线交付 | 签名 OCI Artifact/Bundle、本地 Registry | T3/场景能力 | Bundle 包含制品和撤销信息，导入过程必须验证并审计 |

## 前后端与组件关系

1. Portal、CLI 和生成 SDK 仅调用版本化公共 API，不访问平台、市场或 Provider 内部数据库。
2. Market 负责 Product、Release、Channel 和 Entitlement；Platform 负责目标选择、预检、Secret 解析和 Operation 执行。
3. Platform 将不可变 Release/CompositionRelease 解析为 ExecutionPlan，再经 Provider/Runtime Driver 写入运行目标。
4. Provider 把领域生命周期动作映射到 Kubernetes、Container Engine、KubeEdge 或外部服务，但不能建立独立执行入口。
5. Artifact 客户端直接访问 OCI/S3 数据面，Portal 和 API 仅签发授权或返回引用。
6. Read Model 汇聚市场、平台、Provider、Gateway、AI 和 Edge 的可查询投影；控制面故障不应中断已运行数据面。

## BOM 与选型治理

每个交付版本必须维护 Core BOM、Infrastructure BOM、Provider BOM 和 Optional Pack BOM，至少记录镜像 digest、Chart digest、Schema 版本、兼容矩阵和 Conformance 证据。

任何待选语言、框架或产品的冻结必须在对应 change 中完成：

1. `proposal.md` 说明用户价值、Tier、影响平面、依赖、资源预算和退出判据。
2. `design.md` 比较候选技术并记录语言版本、框架版本、依赖边界、数据模型、失败模式、性能、安全、升级和回滚。
3. `tasks.md` 覆盖生成式契约、实现、测试、Conformance、文档和 BOM 锁定。
4. `verify` 提供功能、兼容性、性能、故障和回滚证据后，方可把选型标记为 Production Ready。

## 当前待决策项

- Portal 的 UI 组件库、路由、状态管理、国际化和测试框架。
- Go 版本及 HNB Core、App Market、Cluster Agent 与 CLI 的 Web/RPC、CLI 和持久化框架。
- 首个 Rust 组件的必要性、Rust 版本、异步运行时和传输库；没有性能证据时不引入。
- 平台元数据库与缓存的具体版本和各部署档位默认实现；Operation Store 已确定使用 PostgreSQL，消息骨干已确定使用 NATS JetStream。
- NATS Server、Go Client、Helm Chart 的精确版本，以及消息大小、保留期、AckWait、MaxDeliver 和磁盘预算。
- Alert Store Schema、默认严重等级/升级 SLO、SMTP 实现、模板引擎、通知保留期和值班计划集成方式。
- 首个 SMS Provider 及其支持区域、模板审批、费用预算、回执和数据驻留策略。
- 默认 Gateway Provider、OCI Registry、S3 后端、Secret Provider 和可观测后端。
- PostgreSQL、Valkey/RabbitMQ Service Provider 的具体 Operator、Chart、版本和支持矩阵。
- AI Gateway、模型 Runtime、推理框架、向量存储和评测框架。

这些待决策项不是缺失的系统行为，而是有意保留给阶段化 change 的 design/BOM 决策，以维持 Provider 可替换性和避免在主规格中固化产品实现。
