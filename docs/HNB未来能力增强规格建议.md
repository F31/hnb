# HNB 云原生平台未来能力增强规格建议

> 本文档基于 HNB 现有 OpenSpec 规格、需求规格说明书（V3.1）、技术实现方案和当前代码实现，对照用户提出的能力增强建议，逐条评估现状、缺口、规划建议和优先级。不重复建设已有规格，明确区分"已有规格但需落地""已有骨架但需闭环""建议新增规格"和"非核心/禁止项"。

---

## 1. 多集群管理：Karmada 联邦

### 已有规格

- `multi-cluster/spec.md`（MC-001~MC-005）：集群注册表、心跳与状态聚合、Karmada 集成、调度策略、跨集群 Operation 追踪
- `karmada-provider` 服务骨架已存在

### 现状评估

| 能力 | 状态 | 说明 |
|------|------|------|
| 集群注册/摘除 | 规格已定义，API 层骨架 | 需补充 PostgreSQL 持久化、集群连通性验证 |
| 心跳与状态聚合 | 规格已定义 | 需补充心跳超时状态机、inactive 告警触发 |
| PropagationPolicy 调度 | 规格已定义 | Provider 生命周期未完成，需补充 PropagationPolicy/ClusterPropagationPolicy 生成逻辑 |
| 跨集群 Operation | 规格已定义 | 依赖 Operation 引擎和 Provider 执行链，骨架已存在 |

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| Pull/Agent 模式纳管私网集群 | MC-006 | P1 | 集群无法暴露公网端点时，通过 cluster-agent 反向连接注册 |
| 集群凭据轮换 | MC-007 | P1 | Kubeconfig 证书到期前自动轮换，审计记录轮换操作 |
| 集群能力画像 | MC-008 | P1 | 集群注册/心跳时上报 CapabilitySnapshot（K8s 版本、CNI/CSI、GPU、架构），存入 capabilites 表 |
| 离线集群状态机 | MC-009 | P1 | 心跳超时 → inactive → 告警 → 自动恢复或人工摘除 |
| 集群标签与拓扑 | MC-010 | P1 | 集群支持标签（region/zone/az/tenant）和拓扑分组，用于 PropagationPolicy 选择器 |
| 联邦配额 | MC-011 | P2 | 跨集群资源配额统一管理，防止单一集群超分 |
| 策略冲突检测 | MC-012 | P2 | 多个 PropagationPolicy 匹配同一资源组时的冲突检测与告警 |
| 跨集群发布与回滚 | MC-013 | P2 | 基于 Operation 的跨集群版本化发布，失败时按集群粒度回滚 |

### 非核心/禁止项

- **不重新实现联邦调度器**：Karmada 已提供 PropagationPolicy、ClusterPropagationPolicy、OverridePolicy、副本分发和故障迁移，HNB 以 Provider 方式集成，不做二次调度
- **集群批量升级不纳入首期**：升级 Karmada 控制面或成员集群版本属于运维工具范畴，不在 T1 范围内

---

## 2. HPC、NUMA 与实时调度

### 已有规格

- `platform-kernel/spec.md` KERNEL-001：内核不包含调度器实现
- `requirements V3.1` CMP-05：支持 CPU Manager、Topology Manager 和 NUMA 策略（P1）
- `requirements V3.1` CMP-08：支持批处理和队列调度插件（P2）
- `requirements V3.1` CMP-06：专用节点池（P1）

### 现状评估

| 能力 | 状态 | 说明 |
|------|------|------|
| CPU Manager / Topology Manager | 需求已定义，无 OpenSpec | 无对应 Provider 或 SchedulingProfile 规格 |
| 批处理/队列调度 | 需求 P2 已定义 | 无对应 OpenSpec |
| 调度治理抽象 | 缺失 | 无 SchedulingProfile 概念 |

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| 统一 SchedulingProfile API | SCHED-001 | P1 | 抽象调度器选择、QoS 等级、CPU 绑定策略、NUMA 对齐、HugePages、网络加速，映射到下层调度器 |
| Koordinator Provider | SCHED-002 | P1 | 在线离线混部、CPU 精细调度、CPU Bind、FullPCPUsOnly、SingleNUMANode、NodeResourceTopology |
| Volcano Provider | SCHED-003 | P2 | 队列、层级队列、Gang Scheduling、优先级抢占、批处理作业 |
| SchedulingProfile 预检与兼容校验 | SCHED-004 | P1 | 部署前校验目标集群是否支持所选调度策略（如 Koordinator 未安装则拒绝 SingleNUMANode） |
| CPU 绑定与 NUMA 对齐 | SCHED-005 | P1 | 继承 CMP-05，上升为 OpenSpec |

### 非核心/禁止项

- **不深度修改调度器代码**：HNB 以 Provider 方式集成 Koordinator 和 Volcano，不做调度器二次开发
- **SchedulingProfile 不是强依赖**：普通应用使用默认调度器，不强制安装 Koordinator 或 Volcano

---

## 3. 网络与隔离

### 已有规格

- `runtime-target/spec.md`：能力发现包含 CNI 类型
- `cilium-provider`、`calico-provider`、`kube-ovn-provider`、`network-provider`、`network-registry` 骨架存在
- `requirements V3.1` 支持 Cilium、Calico、Kube-OVN、Flannel，Multus + SR-IOV/DPDK（P2）
- 多网络平面（管理/存储/业务/容灾）在 `HNB_Cloud_技术与实现方案.md` 已定义

### 现状评估

| 能力 | 状态 | 说明 |
|------|------|------|
| 基础 CNI 管理 | Provider 骨架具备 | 需补充 Provider 生命周期（install/upgrade/rollback） |
| 网络策略管理 | 需求已定义 | 无独立 OpenSpec，可通过 NetworkPolicy Provider 补充 |
| 高性能网络（Multus/SR-IOV/DPDK） | 需求 P2 | 无 Provider 实现，无 OpenSpec |
| 带宽管理 | 缺失 | 标书未明确，Cilium Bandwidth Manager 可满足 |

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| 高性能网络插件包 | NET-ADV-01 | P2 | 定义 Multus + SR-IOV Device Plugin + RDMA Device Plugin + DPDK 的组合安装和配置，不进入最小安装 |
| 固定 IP 与多网络平面 | NET-ADV-02 | P2 | 工作负载声明多网卡，管理/业务/存储网隔离 |
| 带宽管理策略 | NET-ADV-03 | P2 | 基于 Cilium Bandwidth Manager 的 Pod 入口/出口带宽限制 |
| PTP 时间同步 | NET-ADV-04 | P3 | 高实时仿真场景可选，边缘节点可选 |

### 非核心/禁止项

- **Flannel 不作为核心产品能力**：Cilium + Calico 已覆盖，Flannel 仅用于兼容已有环境
- **TSN 不纳入通用平台**：时间敏感网络属于行业专用，不应作为 HNB 通用能力

---

## 4. 云边协同

### 已有规格

- `edge-pack/spec.md`：T3 边缘能力包，零侵入、云边统一执行链、离线自治、批量 OTA、设备接入、离线交付
- `edge-node-group/spec.md`：节点组、灰度批次、健康门禁
- `edge-runtime-target/spec.md`：KubeEdge 注册、能力发现、节点新鲜度
- `edge-runtime-provider/spec.md`：Edge Runtime Provider v2 契约、EdgeApplication 生命周期、离线隔离
- `edge-provider` 服务骨架存在
- `cluster-agent` + `tunnel-server` 已实现

### 现状评估

| 能力 | 状态 | 说明 |
|------|------|------|
| 私网连接（Agent + Tunnel） | 已实现 | 适用于普通集群纳管 |
| KubeEdge 集成 | 规格已定义，Provider 骨架存在 | 需补充 Provider 生命周期完整实现 |
| 边缘离线自治 | 规格已定义 | 依赖 KubeEdge EdgeApplication 能力 |
| 本地元数据持久化 | 规格已定义 | 依赖 EdgeApplication 实现 |
| 离线告警与日志暂存 | 规格已定义 | 依赖 edge-provider 实现 |
| 冲突合并 | 规格已定义 | 需补充 reconciliation 逻辑 |

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| Edge Provider 完整生命周期 | EDGE-ADV-01 | P1 | 补充 install/upgrade/rollback/uninstall，EdgeApplication 增删改查 |
| 边缘离线指标回填 | EDGE-ADV-02 | P1 | 断网期间指标本地缓存，恢复后按序回填 |
| 增量 OTA 与回滚 | EDGE-ADV-03 | P1 | 基于边缘节点组的灰度升级，失败时按批次回滚 |
| 设备接入管理 | EDGE-ADV-04 | P2 | Mapper 容器化，设备数据上云，与 EdgeApplication 关联 |

### 非核心/禁止项

- **不在 HNB Cluster Agent 中重复实现 EdgeCore**：Agent 仅做隧道和代理，完整边缘自治依赖 KubeEdge
- **边缘场景不强制使用标准 KubeEdge 版本**：通过 Provider 契约兼容不同 KubeEdge 版本

---

## 5. 微服务治理

### 已有规格

- `gateway/spec.md` GW-006：Gateway API、Service Mesh、AI Gateway 使用独立能力模型
- `requirements V2.0/V3.1` MS-01：基于 K8s Service 和 DNS 提供默认服务发现，不强制依赖注册中心
- `requirements V3.1` MS-05：支持 Nacos 等注册配置中心插件（P1）

### 现状评估

| 能力 | 状态 | 说明 |
|------|------|------|
| K8s 原生服务发现 | 默认能力 | 无需 HNB 额外处理 |
| Nacos 注册中心 | 需求 P1 已定义 | 无 Provider 实现，无 OpenSpec |
| ServiceComb 注册中心 | 需求未明确 | 需确认是否在 P1 范围内 |
| 流量治理（灰度/分流/重写） | gateway spec 已定义 | Gateway Provider 需补充生命周期 |

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| 注册中心 Adapter Provider | MS-ADV-01 | P1 | 统一抽象 Nacos/ServiceComb/Eureka 为注册中心 Provider，HNB 统一展示但不强制所有服务注册 |
| 注册中心可视化 | MS-ADV-02 | P1 | 展示注册中心实例、服务列表、健康状态、配置管理 |

### 非核心/禁止项

- **不强制双重注册**：K8s 原生服务不走注册中心，存量 Java 微服务通过 Adapter 接入，避免路由冲突和状态不一致
- **不建设独立注册中心**：由注册中心 Provider 负责生命周期，HNB 不做注册中心实现

---

## 6. 弹性伸缩

### 已有规格

- `requirements V3.1` CMP-07：支持 HPA 和外部指标弹性（P1）
- `requirements V3.1` APP-07：支持 HPA 和基于外部事件的弹性伸缩（P1）
- `kubernetes-runtime-provider/spec.md`：明确将 autoscaling 列为非目标

### 现状评估

| 能力 | 状态 | 说明 |
|------|------|------|
| HPA 基础 | 需求已定义，K8s 原生支持 | 无 OpenSpec 规格 |
| 自定义指标弹性 | 需求已定义 | 无 Provider 实现 |
| 事件驱动弹性（KEDA） | 缺失 | 标书未明确，建议新增 |
| 定时弹性 | 缺失 | 标书未明确，建议新增 |
| GPU 指标适配 | 缺失 | 需求已定义 GPU 监控，但未定义弹性 |

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| 统一 ScalingPolicy 抽象 | SCALE-001 | P1 | 定义弹性策略 API，底层映射到 HPA / KEDA / Cron |
| KEDA Provider | SCALE-002 | P1 | 事件驱动弹性（Kafka Lag、NATS Consumer Lag、Prometheus 自定义指标、队列深度） |
| 定时弹性策略 | SCALE-003 | P1 | 基于 Cron 的固定时间弹性（工作日/夜间/节假日） |
| GPU 指标适配器 | SCALE-004 | P2 | GPU 利用率、显存使用率作为弹性指标 |
| 扩缩容审计与冷却 | SCALE-005 | P1 | 弹性操作记录、冷却期、防抖、最小可用副本保护 |
| 预置指标模板 | SCALE-006 | P1 | 不少于 5 种预置指标模板（CPU、内存、请求量、队列深度、GPU 利用率） |

### 非核心/禁止项

- **不将"不少于 5 种业务指标"写死为数量要求**：改为"支持基于 Prometheus 查询、外部指标和事件源创建扩缩策略，并提供不少于 5 种预置指标模板"

---

## 7. 组件服务子系统

### 7.1 统一 OCI Artifact 模型

### 已有规格

- `artifact-storage/spec.md`：统一 OCI 逻辑模型、内容寻址、存储分层、直接上传/下载、分发缓存、安全 GC
- `artifact-direct-upload/spec.md`：大文件直接上传，Transfer Gateway 已实现
- `app-market/spec.md`：统一产品与发布模型
- 当前代码已实现：产品/版本/制品 CRUD、Transfer Gateway 分片上传、自动类型推断

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| 组件元数据模型 | COMP-OC-01 | P1 | Component 统一模型：Metadata、Version、OCI Artifact 引用、Deployment Descriptor、Capability、Dependencies、Configuration Schema、Runtime Profile、Compatibility Matrix |


### 7.2 区分三种编排

### 已有规格

- `composition-operation/spec.md`：CompositionRelease、ExecutionPlan、DAG、Operation 状态机、补偿语义
- `operation-worker` 服务已实现

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| 部署编排 DAG 标准 | COMP-ORCH-01 | P1 | 定义 nodes + dependencies 标准 DAG，operation-worker 负责部署与生命周期编排 |
| 业务工作流 Provider | COMP-ORCH-02 | P2 | 条件、分支、审批、任务，作为可选 Workflow Provider |
| 行业仿真数据流 | COMP-ORCH-03 | P3 | 高频实时数据流通过扩展点接入 DDS、HLA、DIS，不交给 NATS |

### 非核心/禁止项

- **operation-worker 不承担高频实时数据流**：仅限于部署编排，数据流通过扩展点

### 7.3 Provider Lifecycle

### 已有规格

- `provider-conformance/spec.md`：Provider Manifest、Conformance、Lifecycle 接口
- `extension-controller` 服务骨架存在，Lifecycle 接口、NATS Handler、DB Schema 已定义
- `deployment-governance/spec.md`：能力分级、部署档位

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| PostgreSQL Store 实现 | PLC-01 | P1 | Provider 状态、版本、配置持久化到 PostgreSQL |
| 状态转换与 Outbox 事务 | PLC-02 | P1 | Provider 状态变更与事件通过 Outbox 同事务写入 |
| OCI Digest 与签名校验 | PLC-03 | P1 | 安装时校验 Provider 镜像 Digest 和 Cosign 签名 |
| Provider Manifest 兼容矩阵 | PLC-04 | P1 | 声明兼容的 CPU 架构、OS、K8s 版本、内核能力 |
| 安装/升级/回滚/卸载依赖检查 | PLC-05 | P1 | 前置依赖检查、版本兼容性检查、冲突检测 |
| Operations/RuntimeTargets/Capability/Navigation 真实查询 | PLC-06 | P1 | Provider 生命周期中查询和操作这些资源 |
| JetStream 重试与恢复 | PLC-07 | P1 | 消息重复投递、重试退避、死信恢复、幂等消费 |
| Lifecycle Reconciler | PLC-08 | P1 | 定期对账 Provider 实际状态与期望状态 |

---

## 8. 集成中心与服务目录

### 已有规格

- `service-blueprint/spec.md`：ServiceBlueprint 抽象，DB 服务（PostgreSQL）、中间件服务（Valkey、RabbitMQ）
- `requirements V3.1` CORE-04：统一服务目录和套餐
- 当前 `web/plugins/service` 插件已存在

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| 服务目录与应用市场分离 | SVCAT-01 | P1 | 应用市场=可安装的制品，服务目录=已部署可申请的能力，API 目录=可调用的接口产品 |
| 统一 Service Descriptor | SVCAT-02 | P1 | identity + capability + runtime + sla + governance 五段式描述 |
| 服务等级（SLA）分级 | SVCAT-03 | P1 | Bronze/Silver/Gold/Realtime 四级，允许按等级声明可用性、延迟、吞吐量 |
| 服务订阅与审批 | SVCAT-04 | P1 | 用户申请服务、审批流程、自动或手动分配 |

### 非核心/禁止项

- **SLA 不统一要求 99.99%/P99<100ms**：允许服务按等级声明，避免不切实际的要求

---

## 9. API 能力开放网关

### 已有规格

- `gateway/spec.md`：Gateway API 优先、CRD 集中治理、标准资源协商、多租户隔离、流量治理、流量产品分层
- `gateway-provider` 服务骨架存在

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| API 管理控制面 | APIGW-01 | P1 | API 注册、版本、订阅、审批、API Key、OAuth2、mTLS、配额限流、Consumer 管理 |
| 网关 Provider 适配层 | APIGW-02 | P1 | APISIX / Kong / Envoy Gateway 统一管理，HNB 不与具体网关产品强耦合 |
| JDBC 禁止作为对外协议 | APIGW-03 | P1 | JDBC 仅作为内部数据连接器，不直接向外暴露 |
| 证书管理 | APIGW-04 | P1 | 网关证书统一管理、自动续期、灰度替换 |
| API 灰度发布 | APIGW-05 | P2 | API 版本灰度、兼容策略、下架流程 |
| 使用量计量与审计 | APIGW-06 | P2 | API 调用量、响应时间、错误率、按租户计量 |

---

## 10. 安全

### 已有规格

- `identity-tenancy/spec.md`：多租户身份、RBAC、OpenFGA、SecretReference、mTLS、密钥轮换
- `security-supply-chain/spec.md`：双供应链门禁、Cosign/Notation 签名、SBOM、运行时准入、撤销分发
- `config-secret/spec.md`：外部 KMS 可替换性、边缘加密
- `requirements V3.1`：镜像安全（Trivy）、运行时安全（Falco/Tetragon）、安全事件中心

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| OIDC/LDAP 身份提供商 | SEC-ADV-01 | P1 | 企业已有身份系统集成 |
| 三级授权（租户/空间/资源实例） | SEC-ADV-02 | P1 | 继承现有 RBAC 规格，细化到资源实例级别 |
| NetworkPolicy 默认拒绝 | SEC-ADV-03 | P1 | 新命名空间默认拒绝所有入站流量 |
| 镜像漏洞阻断准入 | SEC-ADV-04 | P1 | 部署时检查镜像漏洞等级，高危阻断 |
| 运行时异常检测 | SEC-ADV-05 | P1 | Falco/Tetragon 事件接入安全事件中心 |
| 审计防篡改 | SEC-ADV-06 | P1 | 审计日志写后不可修改，加密哈希链 |
| 管理操作双人审批 | SEC-ADV-07 | P1 | 高风险操作（删除、卸载、数据面变更）需双人审批 |
| API 防重放 | SEC-ADV-08 | P2 | 非幂等写 API 防重放攻击 |
| 数据分级分类 | SEC-ADV-09 | P2 | 敏感数据标记、加密、脱敏策略 |

---

## 11. 可观测性

### 已有规格

- `observability-dr/spec.md`：统一遥测、Operation SLO、备份恢复、故障演练、性能预算、边缘回填
- `alert-notification/spec.md`：告警生命周期、去重、通知渠道
- `alert-manager` 服务已存在

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| 统一关联维度（tenantId/spaceId/clusterId/providerId/operationId/traceId） | OBS-ADV-01 | P1 | 日志、指标、链路的全关联维度注入 |
| SLO 自动生成 | OBS-ADV-02 | P1 | 基于 Service Descriptor 自动生成 SLO 仪表盘和告警 |
| 统一告警中心 | OBS-ADV-03 | P1 | 合并告警、通知路由、升级策略、静默规则 |
| 拓扑关联 | OBS-ADV-04 | P2 | 应用、服务、数据库、网络、存储的多层拓扑关联 |

---

## 12. 国产化

### 已有规格

- `deployment-governance/spec.md`：能力分级、部署档位、BOM
- `requirements V3.1`：支持多架构 CPU、GPU 和国产异构设备（K8S-14，P2）

### 建议新增规格

| 新 Requirement | 建议 ID | 优先级 | 说明 |
|---------------|---------|--------|------|
| Provider 兼容矩阵声明 | LOC-01 | P1 | Provider Manifest 声明 architectures、operatingSystems、kubernetes、kernel、capabilities |
| 国产化认证矩阵 | LOC-02 | P1 | 官方认证的 CPU（海光/鲲鹏/飞腾/申威）、OS（麒麟/统信 UOS/openEuler）组合 |
| 兼容性预检 | LOC-03 | P1 | 部署前校验目标集群架构/OS/K8s 版本，不匹配时拒绝安装 |
| 国产化合规报告 | LOC-04 | P2 | 自动生成已安装组件的国产化兼容清单 |

---

## 13. 产品分层建议

### 已有规格

- `deployment-governance/spec.md` GOV-002：Minimal、Lite HA、Standard HA、Enterprise 四档 BOM

### 建议

T0 微内核（已有规格，强化落地）：
apiserver、platform-api、IAM/RBAC、Tenant/Workspace、Cluster Registry、Plugin/Provider Registry、Operation Engine、ExecutionPlan Engine、Navigation、Read Model、PostgreSQL、NATS、Audit、基础 Observability、Agent/Tunnel

T1 标准能力包（已有规格，补充落地）：
Provider Lifecycle、Gateway Provider、K8s Provider、CNI Provider（Cilium/Calico）

T2 高级能力包（已有规格 + 新增）：
Karmada、HAMi、Koordinator、Volcano、Ceph/CSI、KubeEdge、API Management、安全栈、可观测栈

T3 行业扩展包（新增）：
高性能网络（Multus/SR-IOV/DPDK）、实时调度扩展、行业仿真编排、TSN、AI Extension

---

## 14. 优先级总表

| 优先级 | 领域 | 建议 Requirement ID 数 | 说明 |
|--------|------|----------------------|------|
| **P0（已有规格，强依赖落地）** | Provider Lifecycle、OCI 模型、Gateway API、安全供应链、可观测性 | 0（已有规格） | 先完成既有规格再开辟新领域 |
| **P1（新增规格，T1 前完成）** | 多集群补充、SchedulingProfile、弹性策略、服务目录、API 管理、国产化、注册中心 Adapter | 30+ | 核心产品竞争力 |
| **P2（新增规格，T2 范围）** | 高性能网络、Volcano、业务工作流、联邦配额、策略冲突检测 | 20+ | 特定场景需求 |
| **P3（未来行业扩展）** | TSN、行业仿真、DDS/HLA/DIS、实时调度 | 5+ | 行业专用，不纳入通用平台 |

---

## 15. 竞争策略建议

1. **Provider Lifecycle 优先**：所有新增能力都以 Provider 方式接入，先完成 PLC-01~PLC-08，新能力才有可管理的生命周期
2. **SchedulingProfile 差异化**：市场上没有统一调度抽象的产品，这是 HNB 可以建立差异化优势的领域
3. **服务目录与 API 管理是卖方最直观的"平台"能力**：比 HPC 调度更容易被买方感知
4. **国产化是合规刚需**：需在 V1 版本前完成 LOC-01~LOC-03，否则无法进入国资项目
5. **不追求"功能列表最长"**：保持微内核原则，可替换、可卸载，避免"功能列表长但每个都浅"的陷阱
