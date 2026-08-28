# HNB 超融合云原生平台 技术白皮书

> 版本：V1.0
> 编制日期：2026-07-29
> 文档定位：面向架构师、IT 决策者与实施工程师，系统阐述 HNB Cloud 的设计哲学、架构边界、关键技术能力与交付形态。

---

## 1. 概述

### 1.1 一句话定位

**HNB Cloud 是"以租户为中心、以 Provider 为骨架、以 Operation 为血液"的微内核云原生操作系统**：核心只承担身份、资源、编排与治理的"操作系统内核"职责，网络、存储、GPU、数据库、中间件、网关、AI、联邦与边缘实现均通过统一 Provider 契约接入，可插拔、可替换、可独立演进。

### 1.2 解决的核心问题

传统云原生平台在落地中普遍存在三类痛点：

1. **可扩展性困境**：功能模块直接编译进单体后端，新增能力需核心团队发版，集成周期以月计。
2. **运维可信度不足**：长任务缺乏统一状态机与可审计链路，"操作发出"与"操作完成"之间存在大量灰色地带，故障难以对账与回滚。
3. **部署形态割裂**：单机版、生产版与多地域版采用不同代码与 API，用户跨阶段升级需重构投资。

HNB Cloud 通过"微内核 + Provider + Operation Engine"三段式架构，将扩展性收敛到契约层、将运维编排收敛到状态机、将部署形态收敛到统一安装包，从根上回应上述痛点。

### 1.3 与同类产品的差异化

| 维度 | 传统平台常见做法 | HNB Cloud 差异化设计 |
|---|---|---|
| 多租户 | 组织=租户，强绑定 | 组织与租户解耦，`TenantOrgBinding` 多对多，支持集团型复杂组织到资源边界的映射 |
| 扩展方式 | 功能模块写入单体后端 | 严格 Provider 契约 + 进程外插件，内核编译期零依赖具体实现 |
| 容灾 | 停留于"多集群 + Karmada 分发" | 显式区分应用/数据/流量/控制面四层容灾，DR Manager 统一编排 |
| 资源交付 | 直接创建 K8s 对象 | ResourceRequest → ApprovalPolicy → Operation 三段式，天然审批留痕 |
| GPU | 简单 Device Plugin 直通 | 整卡 + HAMi 虚拟化双模型并存，显式声明隔离等级 |
| 部署形态 | 单机版与高可用版两级 | 单节点 → 融合 HA → 分离 HA → 多地域，四级平滑演进，同一套安装包与 API |
| 性能工程 | Go 全栈 | 关键热路径（备份传输、日志采集、跨站点复制）按性能证据选用 Rust |

### 1.4 北极星指标

- **T0（试用）→T1（生产可用）→T2（多地域生产）** 三个阶段用户可在不换产品、不改 API 的前提下平滑升级；
- 新增一个 Provider（如接入客户已有的存储厂商 CSI）平均耗时 ≤ 5 人日，无需平台核心发版；
- 核心控制面裸机资源占用 ≤ 4 vCPU / 8 GiB（三副本合计），可在边缘一体机上运行。

---

## 2. 总体架构

### 2.1 分层视图

```text
┌───────────────────────────────────────────────────────────────────┐
│ L0 访问面      Web Console(Vue 3) │ CLI(hnbctl) │ OpenAPI │ SDK    │
├───────────────────────────────────────────────────────────────────┤
│ L1 接入安全面  Gateway API + OIDC/LDAP + RBAC/ABAC + RateLimit     │
├───────────────────────────────────────────────────────────────────┤
│ L2 微内核控制面 (Go, 无状态, StatefulSet 化 PG/Valkey 除外)        │
│   platform-api / operation-worker / read-model / rbac-syncer       │
├───────────────────────────────────────────────────────────────────┤
│ L3 Provider 层  (独立进程/容器, 公共契约, 可异构语言实现)          │
│   kubernetes / edge / gateway / app-market / network / storage     │
│   accelerator / database / middleware / federation / dr / ai       │
├───────────────────────────────────────────────────────────────────┤
│ L4 集群执行面   cluster-agent(Go) │ OTel Collector │ K8s/CNI/CSI   │
├───────────────────────────────────────────────────────────────────┤
│ L5 基础设施面   管理网/存储网/业务网/容灾网 │ 裸金属/虚机/公有云/边缘│
└───────────────────────────────────────────────────────────────────┘
```

### 2.2 四个逻辑平面解耦

HNB Cloud 将系统拆为四个逻辑平面，**不共享内部数据库**，跨平面通过公共契约交互：

1. **HNB Core**：Tenant Context、Operation/ExecutionPlan、Read Model、Resource Graph、Provider Registry、Audit。
2. **App Market**：Product/Release/Channel/Entitlement，独立部署、独立升级、独立备份。
3. **Artifact Storage**：OCI 优先、内容寻址、生产固定 digest；Portal/API 不代理大文件数据面。
4. **AI Extension Plane**：可独立启停，不是事实源且不得形成执行旁路。

### 2.3 五条冻结的架构边界

1. 微内核加 Provider/CapabilityPack，具体 CNI、CSI、数据库、中间件、Gateway、AI Runtime 和 Edge 实现不编译进内核；
2. `Release/CompositionRelease → ExecutionPlan → Operation` 是唯一运行目标写入链路；
3. 公共契约 **Schema First**；跨进程/跨平面通过 OpenAPI、Protobuf、Event、Manifest 交互并生成 SDK；
4. OCI 优先、内容寻址、生产固定 digest；Market/Platform API 不代理大文件数据面；
5. 控制面故障不影响数据面：已运行应用、数据库、中间件、Gateway 数据面与已下发边缘负载在控制面中断期间继续运行，恢复后对账。

---

## 3. 微内核与 Operation Engine

### 3.1 最小内核职责

HNB Core 仅承担九项职责，超出此集合的能力必须以 Provider 形式接入：

- 身份与租户上下文
- Operation Engine
- ExecutionPlan Engine
- Read Model
- Resource Graph
- Provider / Capability Registry
- Policy Hook
- Audit（哈希链防篡改）
- Agent Gateway

### 3.2 Operation 唯一写入口

所有部署、升级、回滚、扩缩容、备份、恢复、切换、删除、GC、OTA 和高风险配置变更 MUST 通过持久化 Operation 执行；任何门户、Copilot、Provider 或 Controller MUST NOT 绕过该状态机直接改变 RuntimeTarget。

- Operation 采用 **10 态状态机**，支持幂等（IdempotencyKey）、重试、补偿、断点恢复；
- 长任务执行前必须读取权威状态并获取带 **fencing token** 的 Lease，杜绝并发执行互相覆盖；
- 命令、领域事件、通知使用独立的版本化 Subject、Stream、Consumer，全局顺序不是正确性前提；
- 采用至少一次投递 + 业务幂等，不承诺端到端 exactly-once。

### 3.3 RuntimeIntent 与 Server-Owned Planning

外部写请求以 **Typed RuntimeIntent** 表达，引用不可变 Release/CompositionRelease、授权目标、Scope、有界参数与 SecretReference，**不携带可执行步骤、Provider 命令、目标凭据、制品字节、fencing token 或审批结果**。平台接收后解析为 **不可变 ExecutionPlan**，钉死 Release 身份、制品 digest、目标能力快照、Provider 版本、策略结果、已批准参数与完整 Step DAG；Planning 失败不产生任何运行时副作用。

### 3.4 长任务与异步消息架构

```text
Portal / CLI
     │
     v
Platform API
     │  (同一数据库事务)
     v
+-----------------------------------------+
| PostgreSQL Operation Store              |
| Operation / Step / Checkpoint / Lease   |
| Idempotency / Approval / Outbox         |
+-----------------------------------------+
                   │  Outbox Relay
                   v
+-----------------------------------------+
| NATS JetStream                          |
| commands / domain events / notification|
+-----------------------------------------+
   │           │           │
   v           v           v
Operation   Projector   Audit / Notification
Worker       (Read Model)
   │
   v
Provider / RuntimeTarget
```

- 可靠命令与领域事件使用 JetStream 文件持久化、Durable Consumer、显式 ACK、有限重试、背压与受控重放；
- API 在同一数据库事务内提交业务事实与 Outbox；Relay 收到 JetStream 持久化确认后才标记发布完成；
- NATS 不可用时已提交 Operation 保持 `Queued`，Outbox 可重试，恢复后续投，**不允许绕过 Operation 直接执行**；
- Portal 只经鉴权后的 Platform API SSE/WebSocket 或 Read Model 获取进度，不直连 NATS。

### 3.5 查询与控制解耦（CQRS + Read Model）

列表、搜索与聚合查询读 Read Model；控制器通过事件或投影器更新 Read Model。查询接口禁止在请求路径实时遍历全部 RuntimeTarget——这是支撑"纳管 100+ 目标后查询时延不线性增长"的硬约束。

---

## 4. Provider 体系与可组合性

### 4.1 Provider SPI

Provider 以独立进程/容器运行，通过公共契约实现能力发现、校验、规划、执行、观察、升级、备份、恢复、删除与异步状态上报：

| 能力 | 语义 |
|---|---|
| Capabilities | 声明能力集与版本 |
| Validate | 参数与目标预检 |
| Plan | 生成执行计划片段 |
| Apply / Upgrade / Backup / Restore / Delete | 生命周期动作，统一返回 OperationRef |
| Observe | 上报资源状态供 Read Model 投影 |
| Watch | 双向流，主动上报异步状态变化 |

Provider 不能建立独立执行入口——所有动作必须经 Operation Engine 编排。

### 4.2 Conformance 准入

每类 Provider 必须通过对等 **Conformance Harness** 认证，覆盖功能、兼容性、性能、故障与回滚证据。第二 Provider（如替换默认 CNI/Gateway）通过相同 Harness 后即可接入，无需核心发版。仓库内已内置 `provider-conformance` 工具与 `calico/cilium/kube-ovn/network-registry` 等参考实现。

### 4.3 能力域清单

当前 28 个 OpenSpec 领域中，Provider 化能力域包括：

- **Runtime**：kubernetes-runtime-provider、edge-runtime-provider、runtime-driver-execution、runtime-target
- **网络**：network-profile、CNI 能力（Calico/Cilium/Kube-OVN 可替换）
- **网关**：gateway（Gateway API Standard Channel 为首选规范）
- **应用/制品**：app-market、release-package、artifact-storage、artifact-direct-upload
- **组合操作**：composition-operation
- **身份与配置**：identity-tenancy、config-secret
- **治理**：deployment-governance、security-supply-chain、provider-conformance
- **可观测/容灾**：observability-dr、multi-cluster、gslb
- **AI**：ai-extension
- **边缘**：edge-pack、edge-node-group、edge-runtime-target

---

## 5. 多租户与身份治理

### 5.1 模型

`OrganizationUnit`（组织单元）与 `Tenant`（租户）解耦，通过 `TenantOrgBinding` 多对多映射，支持集团型复杂组织到资源边界的精准对接。运行上下文 `Tenant/Org/Project/Environment` 四级贯穿写链路与查询路径。

### 5.2 RBAC → K8s RBAC 同步

`rbac-syncer` 服务独立部署，监听平台 RBAC 变更并下沉为 Kubernetes RBAC，使平台权限模型与集群原生 RBAC 保持一致，避免"平台看得见、集群管不到"的语义裂缝。

### 5.3 隔离档位

`TenantIsolationProfile` 提供共享 / 增强 / 专享三档：

- **共享**：单实例按账号+ACL+配额隔离，备份/恢复整实例执行，恢复后可按租户导出子集；
- **增强 / 专享**：独立实例，天然规避共享运维边界。

---

## 6. 部署形态与平滑演进

四级部署形态共享同一套安装包与 API，用户跨阶段升级无需重构：

| 档位 | 控制面 | 元数据 | 消息 | 适用 |
|---|---|---|---|---|
| Minimal | 单节点 | 单 PG | 单节点 JetStream（非 HA） | 试用、边缘一体机 |
| Lite HA | 三节点多数派 | PG HA | 三节点 JetStream 集群 | 中小生产 |
| 分离 HA | 控制面独立扩展 | PG Patroni | JetStream 集群 | 中大型生产 |
| 多地域 | 多 Region + GSLB | 跨站点复制 | 跨站点 Relay | 多地域生产 |

关键约束：

- Minimal 单节点 JetStream 明确非 HA，仅适合非关键链路；
- Lite HA 及以上必须验证单 Pod、单节点、Leader 三类故障；
- Gateway、AI Extension Plane 可独立启停，未部署时内核不加载相关执行逻辑。

---

## 7. 可观测性、告警与通知

### 7.1 统一遥测

指标、日志、链路、领域事件统一产出，携带 OTel 上下文；Collector、后端存储与指标/日志/链路实现可替换，Prometheus/Loki/Tempo 等产品不编译进 Core。

### 7.2 告警与通知

```text
Metrics/Logs/Traces/Domain Events
   │  Rule Engine / Source Adapters
   v
Alert Normalizer
   v
PostgreSQL Alert Store (Rule/Instance/Silence/Route/Contact/Channel/Job/Delivery + Outbox)
   │  NATS JetStream
   v
Notification Dispatcher
   ├── Portal API SSE/WS (T1)
   ├── Email/SMTP Worker (T1)
   ├── Webhook Worker (T1)
   └── SMS Provider (T2 可选)
```

- AlertInstance 使用 Pending/Firing/Acknowledged/Silenced/Resolved 生命周期；
- fingerprint 去重、时间窗口聚合、父子抑制、防抖、维护窗口、重复间隔、有期限静默共同控制告警疲劳，所有抑制可解释、可审计；
- SMTP/Webhook/SMS 凭据只使用 SecretReference；外部通知默认发送脱敏摘要与受控 Portal 链接，不携带 Secret、Token、kubeconfig 或原始日志正文；
- 渠道独立限速、超时、熔断、有限重试与失败隔离。

---

## 8. 容灾与高可用

### 8.1 四层容灾划分

| 层次 | 解决故障范围 | 机制 |
|---|---|---|
| 应用层 | 应用实例/Pod 故障 | K8s 自愈 + 滚动更新 |
| 数据层 | 单节点/单可用区实例故障 | 引擎原生复制 + 自动主从切换 |
| 流量层 | 入口/统一域名故障 | Gateway API + GSLB |
| 控制面层 | 机房/区域级故障 | DRProtectionGroup 编排 |

数据库自身 HA 解决"实例挂了怎么办"，`DRProtectionGroup` 解决"整个机房/地域没了怎么办"——二者级联而非二选一，核心高可用套餐的数据库实例可进一步纳入跨站点 DRGroup 作为数据层成员。

### 8.2 备份可信

- 所有引擎备份统一落对象存储，路径规范 `s3://backup/tenant/{tenant_id}/{instance_type}/{instance_id}/{timestamp}/`；
- 默认使用租户专属 KMS Key 加密；
- 备份元数据与业务数据不落在同一故障域；
- `verify_after_backup=true` 时定期拉起隔离临时实例从最新备份恢复并跑健康检查，写入 `BackupVerificationReport`，杜绝"备份从未验证过是否可恢复"的生产隐患；
- 恢复默认不覆盖生产卷，到新卷恢复后由用户显式确认切换。

### 8.3 高风险操作护栏

大版本数据库升级默认 `require_approval=true`，需维护窗口内人工确认；验证失败从预创建备份恢复，不做"原地失败重试"——对应"高风险不可逆操作不得全自动执行"的运维原则。

---

## 9. AI Extension Plane

### 9.1 定位

AI Extension Plane 可独立启停，**不是事实源且不得形成执行旁路**：外部模型 Connector、AI Gateway、只读 Copilot 均在此平面；Copilot/AIOps 写操作必须转译为标准 Operation。

### 9.2 协议

AI Gateway 支持 HTTP、SSE、WebSocket 和 OpenAI-compatible；AI 数据面协议已定义。Rust 仅用于自研高吞吐流式代理等实测热路径，且必须先有 Go 基线与可复现性能数据并明确退出指标，方可引入。

---

## 10. 边缘能力

### 10.1 边界原则

- Edge Pack 不是第五平面；云为权威、边可自治；
- KubeEdge 节点不重复部署 HNB Agent，通过 CloudHub-EdgeHub 通道接入。

### 10.2 交付节奏

T3 POC 先验证断连自治，再进入 NodeGroup、OTA、Device Mapper 与离线 Bundle 的生产化 change。Device Mapper 必须容器化并经过市场门禁；资源受限或协议安全敏感 Mapper 可采用 Rust。

---

## 11. 公共契约与供应链

### 11.1 Schema First

`contracts/` 目录作为单一真源，包含 OpenAPI 3.1、Protobuf、JSON Schema、生成式 SDK（Go/TypeScript）与事件 envelope 映射。同一主版本向后兼容；写命令携带 IdempotencyKey、Correlation ID 与期望版本。CI 设有契约校验门禁（`scripts/validate-contracts.mjs`），杜绝漂移。

### 11.2 软件供应链

Cosign + SBOM + OCI Referrer 用于发布与部署执行双重校验；漏洞扫描器、许可证扫描器与准入产品以待选型 change 决策，不固化进主规格。

### 11.3 OpenSpec 治理

HNB 使用 OpenSpec 进行规格驱动开发：每个变更对应 `openspec/changes/<name>/` 目录，包含 proposal、design、tasks、evidence、specs；任何语言/框架/产品的冻结 MUST 在对应 change 中完成提案、设计、任务、Conformance 与 BOM 锁定，并经 verify 提供证据后方可标记 Production Ready。

---

## 12. 性能与容量工程

- 控制面裸机资源上限 ≤ 4 vCPU / 8 GiB（三副本合计），可在边缘一体机运行；
- 查询时延在大规模目标下不线性增长（CQRS + Read Model 硬约束）；
- 高吞吐热路径（备份传输、日志采集、跨站点复制）按性能证据采用 Rust 独立组件，通过版本化公共契约与 Go 控制面解耦，无 FFI、无共享内部结构体。
- **现状说明**：截至本文档修订，仓库内尚未落地任何 Rust 组件；Rust 属于"条件选用"——必须先有 Go 基线与可复现性能数据、明确退出指标，并在对应 OpenSpec change 中记录证据后方可引入（§9.2 同样约束 AI 数据面代理）。

---

## 13. 安全合规

- 身份接入：OIDC Broker（Dex）+ 可选自建 IdP，对接企业 AD/LDAP/OIDC；
- 授权：OpenFGA 关系型 ABAC + 内置 RBAC，多级授权可解释；
- Secret/KMS：通过 Secret/KMS Provider 接入 Vault/企业 KMS/HSM/云密钥服务；公共 API、计划、事件与日志仅使用 SecretReference 或短期令牌，不携带明文 Secret、kubeconfig 或大文件正文；
- 运行时安全：Falco（规则型默认）+ Tetragon（eBPF 可选增强）；
- 镜像安全：Trivy/Trivy Operator + Cosign + Kyverno，SBOM 一体化；
- 审计：哈希链防篡改，所有参数变更记录 diff、操作人、审批单号。

---

## 14. 应用与数据库全生命周期

### 14.1 应用管理

应用创建/升级/回滚/灰度全部转译为标准 Operation，复用 Operation 的步骤编排与补偿机制；应用所需容器资源套餐走 ResourceRequest → ApprovalPolicy；Application→Component→Trait→底层 K8s 对象的关系写入 Resource Graph，支撑统一展示；联邦 Placement 与 DR Placement 直接映射到 Karmada PlacementPolicy。**对外 Application API 契约保持一致**，底层自研 Controller 与 KubeVela Provider 可插拔互换，用户无感知。

### 14.2 数据库/中间件管理

`DatabaseInstance`/`MiddlewareInstance` 拥有全生命周期状态机（Requested → Provisioning → Initializing → Ready ⇄ Reconfiguring/Upgrading/Scaling/Maintaining/Degraded → Failing Over → Suspending → Terminated）。每一次状态迁移建模为标准 Operation，复用同一套引擎的预检-执行-验证-回滚四段式，而非让各 Operator 各自为政。

平台对用户只暴露"开发/标准高可用/核心高可用"三档套餐，副本数、复制模式、仲裁方式等细节由 Provider 内部按引擎最佳实践决定，在高级设置中默认折叠可见。

---

## 15. 技术选型基线

| 架构域 | 选型 | 状态 |
|---|---|---|
| 控制面语言 | Go | 冻结默认 |
| 热路径组件 | Rust（条件选用） | 需性能证据 |
| 公共 API/事件 | OpenAPI、Protobuf、版本化事件、Manifest、生成式 SDK | 冻结 |
| 元数据/状态 | PostgreSQL 16+（Operation Store、Alert Store） | 冻结架构 |
| 异步消息 | NATS JetStream | T1 默认 |
| 缓存 | Valkey | 候选 |
| 容器运行 | KubernetesTarget（OCI/Helm Driver） | MVP 指定 |
| 服务入口 | Kubernetes Gateway API Standard Channel | 冻结规范 |
| 制品 | OCI Registry、ArtifactDescriptor、OCI Referrer | 冻结 |
| 网络可替换 | Cilium / Calico / Kube-OVN / Flannel | 可替换 |
| 边缘运行 | KubeEdge CloudCore/Edge Provider | T3 POC |
| 联邦 | Karmada（独立 Provider） | T3 可选 |
| 可观测 | OTel Collector + 指标/日志/链路后端 | MVP 指定 |
| Portal | Vue 3.5.40 + TypeScript 7.0.2 + Vite 8.1.5 | 冻结 |

具体版本、HA 实现、产品选型由对应 change 的 design.md 与版本化 BOM 锁定，本文只给架构方向，不固化产品实现。

---

## 16. 工程治理与交付

### 16.1 BOM 治理

每个交付版本维护 Core BOM、Infrastructure BOM、Provider BOM、Optional Pack BOM，至少记录镜像 digest、Chart digest、Schema 版本、兼容矩阵与 Conformance 证据。

### 16.2 CI 门禁

CI 流水线在 push/PR 到 `main` 时依次执行：

1. Lint：`go vet` + OpenSpec 校验 + Contract 校验
2. Build：所有 Go 模块 `go build`
3. Unit Test：所有模块 `go test -race`
4. Integration Test：PostgreSQL 16 容器 + 迁移 + PG Store 测试

### 16.3 数据库迁移

79 个版本化迁移脚本（含正向与 rollback），由 `database/postgresql/scripts/migrate.sh` 显式执行；`test-migrations.sh` 在隔离的 postgres:16 容器内验证空库、重复、升级、回滚与恢复路径。

---

## 17. 结语

HNB Cloud 的设计哲学可以归纳为三句话：

1. **内核做减法**：只做身份、编排、查询与治理，具体实现一律 Provider 化；
2. **运维做状态机**：所有变更建模为可审计、可补偿、可回滚的 Operation，旁路即违规；
3. **演进做契约**：跨进程、跨平面、跨版本一律 Schema First，今天选型不锁死明天演进。

这让 HNB 既能跑在边缘一体机上的一台 4 核 8G 机器，也能演进到跨地域、跨云、云边协同的生产规模——而用户的 API、CLI 与 Portal 体验始终保持一致。

---

## 附录 A：术语表

| 术语 | 含义 |
|---|---|
| Operation | 平台唯一运行目标写入单元，10 态状态机，幂等可补偿 |
| ExecutionPlan | 由 RuntimeIntent 解析而来的不可变执行计划，钉死 digest 与 DAG |
| Provider | 独立进程/容器，通过公共契约接入内核的能力实现 |
| CapabilityPack | 面向用户能力粒度的可插拔组合包 |
| RuntimeTarget | 接受 Operation 执行的目标（K8s 集群、Container Engine、边缘运行时等） |
| Read Model | 查询侧投影，解耦控制面与查询路径 |
| DRProtectionGroup | 跨站点容灾编排单元 |
| Outbox | 事务内写入、Relay 异步投递的可靠事件表 |
| Fencing Token | Lease 令牌，防止并发执行互相覆盖 |
| Conformance | Provider 准入测试套件 |

## 附录 B：参考资料

- `README.md`（项目总览）
- `openspec/architecture.md`（架构与技术栈基线）
- `openspec/specs/`（28 个领域可验收规格）
- `docs/HNB_Cloud_技术与实现方案.md`（详细设计与实现方案）
- `docs/HNB_Cloud_OpenSpec_实施基线_V3_8_6.md`（唯一方案基线）
- `contracts/`（公共契约单一真源）
