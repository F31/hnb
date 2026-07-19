# HNB Cloud 云原生平台 —— 技术与实现方案

> 文档版本：V1.0（基于《HNB 云原生平台需求规格说明书 V3.1》编制）
> 编制日期：2026-07-17
> 文档定位：本方案回答"怎么建"——技术选型、系统架构、核心模块实现、数据模型、接口契约、部署形态、安全与容灾实现、研发计划与竞争力设计。需求"要什么"以规格说明书为准，本文不重复罗列需求编号，仅在必要处引用。

---

## 目录

1. 设计目标与竞争力定位
2. 总体技术架构
3. 技术选型总表
4. 微内核详细设计与代码结构
5. 数据模型与存储设计
6. 核心引擎实现方案（Operation / Plugin / Tenant / DR）
7. Provider 体系与关键 Provider 实现
8. 部署拓扑与安装器实现
9. 网络平面与多网实现
10. 安全体系实现
11. 可观测性实现
12. 容灾与双活实现
13. API 与前端实现
14. 性能与容量工程
15. 测试与验收工程
16. 研发路线图与团队组织
17. 创新点与竞争力矩阵
18. 风险与应对
19. 应用全生命周期管理详细设计（含 OAM 标准采纳方案）
20. 数据库与中间件全生命周期管理详细设计

---

## 1. 设计目标与竞争力定位

### 1.1 一句话定位

**HNB Cloud 是"以租户为中心、以 Provider 为骨架、以 Operation 为血液"的微内核云原生操作系统**：核心只做身份、资源、编排和治理的"操作系统内核"，网络、存储、GPU、数据库、中间件、联邦、安全探针、灾备编排全部通过统一 Provider 契约接入，可插拔、可替换、可独立演进。

### 1.2 相对同类产品（灵雀云 ACP、Rainbond、KubeSphere、Rancher）的差异化竞争点

| 维度 | 传统云原生平台的常见做法 | HNB Cloud 的差异化设计 |
|---|---|---|
| 多租户 | 组织=租户，强绑定 | 组织与租户解耦，`TenantOrgBinding` 多对多，支持集团型复杂组织到资源边界的映射 |
| 扩展方式 | 功能模块直接写入单体后端 | 严格 Provider 接口 + gRPC 进程外插件，内核编译期零依赖具体实现 |
| 容灾 | 停留在"多集群 + Karmada 分发"层面 | 显式区分应用/数据/流量/控制面四层容灾，DR Manager 统一编排，杜绝"分发即双活"的误导性宣称 |
| 资源交付 | 直接创建 K8s 对象 | ResourceRequest → ApprovalPolicy → Operation 三段式，天然具备审批留痕与合规能力 |
| GPU | 简单 Device Plugin 直通 | 整卡 + HAMi 虚拟化双模型并存，显式声明隔离等级，杜绝"共享=独占"的性能干扰盲区 |
| 部署形态 | 通常只有"单机版"与"高可用版"两级 | 单节点 → 融合 HA → 分离 HA → 多地域，四级平滑演进，同一套安装包与 API |
| 性能工程 | Go 全栈 | 关键热路径（备份传输、日志采集、跨站点复制）用 Rust 实现，通过 gRPC 与 Go 控制面解耦，兼顾生态成熟度与极限性能 |

### 1.3 北极星指标

- **T0（试用）→T1（生产可用）→T2（多地域生产）** 三个阶段用户可以在不换产品、不改 API 的前提下平滑升级；
- 新增一个 Provider（如接入客户已有的存储厂商 CSI）平均耗时 ≤ 5 人日，且不需要平台核心发版；
- 核心控制面裸机资源占用 ≤ 4 vCPU / 8 GiB（三副本合计），可在边缘一体机上运行。

---

## 2. 总体技术架构

### 2.1 架构分层实现映射

```text
┌───────────────────────────────────────────────────────────────────┐
│ L0 访问面      Web Console(React) │ CLI(hnbctl) │ OpenAPI │ SDK    │
├───────────────────────────────────────────────────────────────────┤
│ L1 接入安全面  Envoy Gateway + OIDC/LDAP + RBAC/ABAC + RateLimit   │
├───────────────────────────────────────────────────────────────────┤
│ L2 微内核控制面 (Go, 无状态, StatefulSet 化 PG/Valkey 除外)        │
│   platform-api / platform-controller / platform-worker            │
├───────────────────────────────────────────────────────────────────┤
│ L3 Provider 层  (独立进程/容器, gRPC, 可异构语言实现)              │
│   network-provider │ storage-provider │ accelerator-provider       │
│   database-provider │ middleware-provider │ federation-provider    │
│   image-security-provider │ runtime-security-provider │ dr-provider│
├───────────────────────────────────────────────────────────────────┤
│ L4 集群执行面   cluster-agent(Go) │ Karmada-agent │ 安全探针        │
│   │ OTel Collector │ K8s / CNI / CSI / Device-Plugin / DRA          │
├───────────────────────────────────────────────────────────────────┤
│ L5 基础设施面   管理网/存储网/业务网/容灾网 │ 裸金属/虚机/公有云/边缘│
└───────────────────────────────────────────────────────────────────┘
```

### 2.2 核心设计模式

1. **控制面/数据面彻底分离**：`platform-api` 只做校验+受理+持久化，实际执行下沉到 `platform-worker`（内部编排）与 `cluster-agent`（集群内执行），保证 API 层无状态、可随时重启。
2. **CQRS + Read Model**：写路径经 Operation Engine 落库并异步驱动；查询路径只读 `Read Model`（Materialized View + Valkey 缓存），杜绝"列表接口实时扫全部集群"。
3. **Outbox + 事件总线**：所有状态变更在同一事务内写入 `outbox` 表，由 `platform-relay` 投递到 NATS JetStream（轻量部署）或 Kafka（大规模部署，作为可切换 Provider）。
4. **Provider SPI + 进程外插件**：Provider 以独立 Deployment 运行，通过 gRPC 实现 `Provider` 接口（见 §7.1），核心通过 `Plugin Registry` 做能力发现、版本兼容、健康检查，不做 Go plugin 动态加载（避免版本地狱）。
5. **声明式 + 状态机**：所有长任务建模为 `Operation`（见 §6.1），支持幂等、重试、补偿、断点恢复。

---

## 3. 技术选型总表

| 层次/模块 | 选型 | 备选/可替换 | 选择理由 |
|---|---|---|---|
| 控制面语言 | Go 1.23+ | — | K8s 生态、controller-runtime 成熟 |
| 热路径语言 | Rust (tokio + tonic) | — | 备份传输、日志采集、跨站点复制的极限吞吐与低 CPU 占用 |
| API 网关 | Envoy Gateway (Gateway API) | Kong、APISIX | 原生 Gateway API，兼容多云 |
| 身份认证 | Dex (OIDC Broker) + Casdoor(可选自建IdP) | Keycloak | 轻量、可对接企业 AD/LDAP/OIDC |
| 授权模型 | OpenFGA（关系型 ABAC）+ 内置 RBAC | OPA/Rego | 租户-项目-资源多级授权用关系型模型更直观、可解释 |
| 元数据数据库 | PostgreSQL 16（Patroni 高可用） | — | 事务、JSONB、分区表能力全面 |
| 缓存/热状态 | Valkey (Redis 兼容, BSD 协议) | Redis | 规避 License 风险，社区活跃 |
| 对象存储 | MinIO / 兼容 S3 API | 云厂商 OSS/S3 | 备份、诊断包、插件包、SBOM 存储 |
| 事件总线（轻量档） | PostgreSQL Outbox + NATS JetStream | Kafka(重型档 Provider) | 轻量部署免中间件，大规模可切换 |
| Controller 框架 | Kubebuilder / controller-runtime | — | 官方标准 |
| Cluster Agent 通信 | gRPC + mTLS 双向流（Push）；Agent 主动连（兼容 NAT/边缘） | — | 规避管理面主动连接被墙的问题 |
| 联邦 | Karmada（独立 Provider） | Clusternet, OCM | CNCF 项目、生态最成熟 |
| 容器网络 | Cilium（推荐）/ Calico / Kube-OVN / Flannel | — | eBPF 数据面 + Hubble 可观测 |
| 容器存储 | 第三方 CSI + TopoLVM（本地）+ Rook-Ceph（可选） | — | 覆盖外部存储、本地高性能、自建分布式三类场景 |
| GPU | NVIDIA Device Plugin / K8s DRA + HAMi | — | 整卡与虚拟化共存 |
| 镜像安全 | Trivy / Trivy Operator + Cosign + Kyverno | Harbor 内置扫描 | 开源成熟、SBOM 一体化 |
| 运行时安全 | Falco（默认）+ Tetragon（可选 eBPF 增强） | — | 规则型 + eBPF 内核级双引擎 |
| 可观测 | OpenTelemetry Collector → Prometheus/Mimir(指标) + Loki(日志) + Tempo(链路) | VictoriaMetrics、ClickHouse | CNCF 标准栈，Region 侧预聚合 |
| 数据库服务 | CloudNativePG(PostgreSQL) / Percona XtraDB Operator(MySQL) / Valkey Operator | — | CNCF/社区活跃 Operator |
| 中间件服务 | Strimzi(Kafka) / RabbitMQ Cluster Operator / RocketMQ Operator / EMQX(MQTT) | — | 各自官方或主流 Operator |
| 网关/微服务 | Envoy Gateway (Gateway API) + 可选 Istio Ambient(轻量 Mesh) | — | 默认不强制 Mesh，MS-09 |
| CI 依赖 | 无（明确排除） | — | 需求边界之外 |
| 前端 | React 18 + TypeScript + Vite + TailwindCSS | — | 组件化、插件化 UI 扩展 |
| CLI | Go + Cobra | — | 单文件跨平台 |
| 备份 | Velero（资源）+ 自研 Rust backup-transfer（数据流） | — | 兼顾生态与性能 |
| 密钥管理 | HashiCorp Vault（可选）/ 内置 KMS + Sealed Secret | — | 支持无 Vault 轻量部署 |

---

## 4. 微内核详细设计与代码结构

### 4.1 进程拓扑

```text
platform-api          无状态，横向扩展，处理 REST/gRPC 请求、鉴权、幂等校验
platform-controller   持有多个 K8s controller-runtime Manager，处理声明式对象的调和循环
platform-worker       消费 Operation 队列，编排跨 Provider 的多步骤任务，支持并发度限制
agent-gateway         多 Region 部署，终结 Cluster Agent 的 mTLS 长连接，做协议网关与限速
cluster-agent         安装在被纳管集群，Watch 本地资源、执行下发任务、上报增量状态
relay                 Outbox → 事件总线投递器，保证至少一次投递
hnbctl                CLI
```

### 4.2 目录结构（对应工程架构章节的落地细化）

```text
hnb-cloud/
├── cmd/{platform-api,platform-controller,platform-worker,agent-gateway,cluster-agent,relay,hnbctl}/
├── internal/
│   ├── kernel/          # 启动引导、依赖注入(wire)、配置加载
│   ├── identity/         # OIDC/LDAP 接入、Session、Tenant Context 签发与校验
│   ├── tenant/            # Tenant/OrganizationUnit/TenantOrgBinding/Isolation
│   ├── resource/          # 统一资源模型、Resource Graph
│   ├── operation/         # Operation 状态机、调度、补偿
│   ├── request_approval/  # ResourceRequest/ApprovalPolicy/ApprovalInstance
│   ├── plugin/            # Plugin Registry、SPI 客户端封装
│   ├── cluster/           # 集群资产、DeploymentProfile、NetworkPlaneProfile
│   ├── application/       # Application/Component/ServiceBinding
│   ├── dr/                # DRProtectionGroup/DRPlan/DRRun 编排逻辑
│   ├── policy/            # RBAC/ABAC、准入策略聚合
│   ├── topology/          # Resource Graph 查询、影响分析
│   ├── readmodel/         # CQRS 查询侧，物化视图与缓存
│   └── audit/              # 审计写入、防篡改（哈希链）
├── providers/
│   ├── contracts/          # Protobuf 定义的 Provider SPI（唯一真源）
│   ├── network/{cilium,calico,kube-ovn,flannel}/
│   ├── storage/{generic-csi,topolvm,rook-ceph}/
│   ├── accelerator/{device-plugin,dra,hami}/
│   ├── federation/karmada/
│   ├── database/{postgresql,mysql,valkey}/
│   ├── middleware/{kafka,rabbitmq,rocketmq,mqtt}/
│   ├── image-security/{trivy,harbor}/
│   ├── runtime-security/{falco,tetragon}/
│   ├── approval/{builtin,itsm-adapter}/
│   └── dr/{replication,gslb,velero}/
├── rust/
│   ├── stream-agent/        # 高频指标/日志采集，边缘轻量场景
│   └── backup-transfer/     # 备份/复制流式传输，零拷贝、断点续传
├── api/{openapi,protobuf}/
├── web/                       # React 前端 + UI 插件加载机制
└── deploy/{helm,manifests,ansible}/
```

### 4.3 Provider SPI（唯一真源，Protobuf 定义示例）

```protobuf
syntax = "proto3";
package hnb.provider.v1;

service Provider {
  rpc Capabilities(CapabilitiesRequest) returns (CapabilitySet);
  rpc Validate(ValidateRequest) returns (ValidateResponse);
  rpc Plan(PlanRequest) returns (Plan);
  rpc Apply(ApplyRequest) returns (OperationRef);
  rpc Observe(ObserveRequest) returns (ResourceState);
  rpc Upgrade(UpgradeRequest) returns (OperationRef);
  rpc Backup(BackupRequest) returns (OperationRef);
  rpc Restore(RestoreRequest) returns (OperationRef);
  rpc Delete(DeleteRequest) returns (OperationRef);
  // 双向流：Provider 主动上报异步状态变化，供 Operation Engine 拉齐
  rpc Watch(WatchRequest) returns (stream ResourceEvent);
}
```

所有 Provider 实现该契约后以独立容器部署，`Plugin Registry` 通过 `plugin.yaml` 声明的 gRPC Endpoint、能力矩阵、权限清单完成注册。核心永不 `import` 具体 Provider 的实现包。

### 4.4 Tenant Context 强制注入实现

- API Gateway 层从 JWT 中解析 `tenant_id`、`org_path`、`roles`，写入内部签名的 `X-HNB-Tenant-Context`（HMAC + 短期有效期，防伪造）；
- 所有下游服务（含 Read Model 查询、事件总线消息、审计写入）强制读取该 Header 并在 SQL 层自动拼接 `WHERE tenant_id = :ctx.tenant_id`（通过 GORM/sqlc 中间件统一注入，业务代码无法绕过）；
- 关键表全部采用 PostgreSQL **Row Level Security (RLS)** 兜底，即使应用层疏漏也在数据库层强制隔离，这是相对多数同类产品仅在应用层做隔离的**额外安全冗余设计**。

---

## 5. 数据模型与存储设计

### 5.1 核心表结构（节选，PostgreSQL）

```sql
-- 组织与租户（多对多解耦）
CREATE TABLE organization_unit (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  parent_id UUID REFERENCES organization_unit(id),
  name TEXT NOT NULL,
  external_directory_id TEXT,
  owner_user_id UUID,
  effective_at TIMESTAMPTZ, expire_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE tenant (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code TEXT UNIQUE NOT NULL,
  name TEXT NOT NULL,
  status TEXT CHECK (status IN ('active','frozen','decommissioning','closed')),
  isolation_level TEXT CHECK (isolation_level IN ('shared','enhanced','dedicated','compliance')),
  cost_center TEXT,
  quota JSONB NOT NULL DEFAULT '{}',
  kms_key_ref TEXT,
  audit_retention_days INT DEFAULT 365,
  created_at TIMESTAMPTZ DEFAULT now()
);
ALTER TABLE tenant ENABLE ROW LEVEL SECURITY;

CREATE TABLE tenant_org_binding (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID REFERENCES tenant(id),
  org_unit_id UUID REFERENCES organization_unit(id),
  binding_role TEXT,             -- owner / contributor / auditor
  is_default BOOLEAN DEFAULT false,
  member_sync_rule JSONB,
  approver_source TEXT,
  effective_at TIMESTAMPTZ, expire_at TIMESTAMPTZ,
  created_by UUID, created_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE(tenant_id, org_unit_id)
);

-- 项目/环境（必须归属唯一租户）
CREATE TABLE project (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenant(id),
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now()
);
ALTER TABLE project ENABLE ROW LEVEL SECURITY;

CREATE TABLE environment (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES project(id),
  tenant_id UUID NOT NULL,  -- 冗余存储，避免 join 才能拿到隔离键
  name TEXT NOT NULL,
  cluster_id UUID,
  namespace TEXT,
  created_at TIMESTAMPTZ DEFAULT now()
);

-- Operation 状态机
CREATE TABLE operation (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  type TEXT NOT NULL,                -- e.g. app.deploy / db.create / dr.failover
  idempotency_key TEXT UNIQUE,
  state TEXT CHECK (state IN
    ('pending','planning','running','waiting_approval','succeeded','failed','compensating','canceled')),
  steps JSONB NOT NULL DEFAULT '[]',
  current_step INT DEFAULT 0,
  retry_count INT DEFAULT 0,
  lock_owner TEXT,               -- 分布式锁持有者（worker 实例 ID）
  lock_expire_at TIMESTAMPTZ,
  input JSONB, output JSONB, error JSONB,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- Outbox
CREATE TABLE outbox_event (
  id BIGSERIAL PRIMARY KEY,
  tenant_id UUID,
  topic TEXT NOT NULL,
  payload JSONB NOT NULL,
  dispatched BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ DEFAULT now()
);

-- 资源申请与审批
CREATE TABLE resource_request (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL, project_id UUID, environment_id UUID,
  request_type TEXT,          -- container / database / middleware
  plan_ref TEXT, quantity INT,
  duration_days INT, purpose TEXT, cost_center TEXT,
  risk_level TEXT,
  status TEXT CHECK (status IN ('draft','pending','approved','rejected','withdrawn','delivering','delivered','failed')),
  operation_id UUID REFERENCES operation(id),
  created_by UUID, created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE approval_policy (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID, resource_type TEXT, plan_pattern TEXT,
  auto_approve_condition JSONB,
  levels JSONB NOT NULL,        -- [{level:1, approver_source:"tenant_admin"}, ...]
  timeout_hours INT, escalation_rule JSONB,
  external_provider_ref TEXT
);

CREATE TABLE approval_instance (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  resource_request_id UUID REFERENCES resource_request(id),
  current_level INT, approver_id UUID,
  decision TEXT, comment TEXT, attachment_ref TEXT,
  decided_at TIMESTAMPTZ
);

-- 灾备
CREATE TABLE dr_protection_group (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID, name TEXT,
  members JSONB,                     -- 应用/数据库/中间件/网关引用列表
  rpo_seconds INT, rto_seconds INT,
  source_site TEXT, target_site TEXT,
  replication_provider TEXT, traffic_provider TEXT,
  start_order JSONB,
  last_drill_at TIMESTAMPTZ, last_recovery_point TIMESTAMPTZ
);

CREATE TABLE dr_plan (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  protection_group_id UUID REFERENCES dr_protection_group(id),
  steps JSONB NOT NULL           -- 预检/隔离/复制确认/启动/流量切换/验证
);

CREATE TABLE dr_run (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  plan_id UUID REFERENCES dr_plan(id),
  run_type TEXT CHECK (run_type IN ('drill','planned_switch','failover','failback')),
  state TEXT, evidence JSONB,
  started_at TIMESTAMPTZ, ended_at TIMESTAMPTZ
);
```

### 5.2 分区与保留策略

- `outbox_event`、`operation`、`audit_log` 按月分区（PostgreSQL 声明式分区），历史分区定期转储至对象存储 + Parquet，供合规审计冷查询；
- 高基数遥测（Metrics/Logs/Traces）不落在核心 PG，走 §11 独立可观测后端；
- Read Model 使用 PG 物化视图 + 增量刷新（基于 Outbox 触发），并在 Valkey 建立二级缓存，TTL 按资源类型差异化（如集群健康 5s、配额 30s）。

---

## 6. 核心引擎实现方案

### 6.1 Operation Engine

**状态机**：`pending → planning → running ⇄ waiting_approval → succeeded|failed → compensating → canceled`

实现要点：

1. **幂等键**：客户端必须携带 `Idempotency-Key`，API 层先查 `operation.idempotency_key` 唯一索引，命中则直接返回既有 Operation，杜绝重复创建。
2. **分布式锁与断点恢复**：Worker 抢占 `lock_owner`/`lock_expire_at`（类似 Postgres advisory lock + 租约），worker 崩溃后锁到期，其他 worker 基于 `steps` JSON 中记录的已完成步骤从断点续跑，而非从头重放。
3. **步骤编排 DSL**：

```yaml
type: app.deploy
steps:
  - id: precheck
    action: policy.validate
  - id: alloc-network
    action: provider.network.apply
    compensate: provider.network.delete
  - id: alloc-storage
    action: provider.storage.apply
    compensate: provider.storage.delete
  - id: create-workload
    action: k8s.apply
    compensate: k8s.delete
  - id: wait-healthy
    action: k8s.wait_ready
    timeout: 300s
  - id: bind-service
    action: servicebinding.create
```

4. **补偿（Saga 模式）**：任一步失败，按逆序执行 `compensate`，保证半成品资源不残留；补偿本身也是幂等操作。
5. **进度推送**：Operation 状态变更写 Outbox → `platform-api` 通过 SSE `/operations/{id}/events` 推送给前端，端到端延迟目标 ≤2s（对应 §13 API 目标）。

### 6.2 Plugin Registry 生命周期实现

```text
plugin.yaml 提交 → 签名/SBOM校验(Cosign) → 兼容性检查(平台API版本范围)
 → 权限清单静态分析(RBAC最小化预览) → 安装(Helm/Kustomize渲染并Apply)
 → 健康检查(readiness探针+自定义health RPC) → 启用(写入Capability索引)
 → 灰度升级(按百分比/按租户) → 验证 → 完成/回滚 → 禁用 → 卸载(执行finalizer清理钩子)
```

- 插件安装产物统一使用 **OCI Artifact** 分发（复用镜像仓库基础设施，无需额外插件市场存储）；
- 插件 Pod 强制注入 `NetworkPolicy`（默认只能访问声明的出站目标）与专属 `ServiceAccount`，通过 OPA Gatekeeper 准入策略拒绝越权的 `hostPath`/`privileged` 声明（除非在 `plugin.yaml` 中显式声明并经过安全审批）。

### 6.3 Tenant/Org 关联与权限计算

权限计算公式（实现为一个纯函数，便于单测）：

```
EffectivePermission =
   OrgMembership(user) ⋈ TenantOrgBinding(active)
   ⋈ TenantRole(user, tenant)
   ⋈ ProjectRole(user, project)
   ⋈ ResourcePolicy(resource)
   − ExplicitDeny
```

采用 **OpenFGA** 建模关系型权限（`tenant#member@user`, `project#admin@tenant#member` 等 tuple），相比纯 RBAC 矩阵更容易表达"组织成员通过 TenantOrgBinding 隐式获得租户角色"这种传递关系，同时天然支持权限"为什么允许/为什么拒绝"的可解释查询（审计与安全审查刚需）。

### 6.4 ResourceRequest/Approval 与 Operation 的衔接

```text
用户提交 ResourceRequest
 → ApprovalPolicy 匹配(按租户+资源类型+套餐+风险等级)
   → 命中自动审批条件 → 直接进入 delivering
   → 否则创建 ApprovalInstance(单级/轻量多级)
      → 审批人处理(通过/驳回/转交/超时升级)
      → 通过 → 创建 Operation(type=resource.deliver) → 执行 Provider.Apply
      → 交付失败 → ResourceRequest.status=failed，但 approval 决策不回滚，展示重试入口
```

轻量审批引擎自身即为一个内嵌状态机（不引入 BPM 引擎），仅在客户明确要求对接外部 ITSM/BPM 时启用 `approval/itsm-adapter` Provider。

### 6.5 Disaster Recovery Manager 编排核心

DR Manager 是对 Operation Engine 的领域封装，`DRPlan` 展开为标准 Operation：

```yaml
type: dr.failover
steps:
  - id: readiness-check        # 复制延迟、证书、容量、演练记录
  - id: quorum-fence            # 仲裁与源站点写围栏，防脑裂
  - id: replication-cutover      # 通知 dr-provider 停止复制或切主
  - id: app-propagate            # 调用 federation-provider 或独立编排拉起目标站点应用
  - id: wait-dependency-healthy
  - id: traffic-cutover           # gslb-provider 切换 DNS/权重
  - id: post-switch-validate       # 探活 + 数据一致性抽样校验
  - id: notify-and-record
```

- **防脑裂机制**：借助外部仲裁（第三仲裁站点或云厂商 Witness 服务）+ 源站点写围栏（Fencing Token 写入复制 Provider），未取得仲裁多数派不允许自动切换；
- **就绪度门禁**：`DRProtectionGroup.last_drill_at` 超过策略阈值（如 90 天）自动降级为"仅支持人工切换"，杜绝未经演练验证的保护组被自动触发。

---

## 7. Provider 体系与关键 Provider 实现

### 7.1 Provider 注册与调用时序

```text
Operation Engine --gRPC--> Provider.Validate --> Provider.Plan --> Provider.Apply(返回OperationRef)
Operation Engine <--Watch(stream)-- Provider  (异步进度回传)
Operation Engine --gRPC--> Provider.Observe (轮询兜底，Watch 断线时)
```

### 7.2 Network Provider（以 Cilium 为例）

- `NetworkProfile` → 渲染 CiliumConfig（IPAM 模式、封装/直路由、kube-proxy 替换开关、加密开关）→ Helm 安装；
- Hubble Relay 暴露的流数据通过 OTel Collector 的 `cilium receiver` 汇聚进可观测面；
- 多 Provider 共存：`kube-ovn` 用于强子网/固定IP场景，`calico` 用于传统 BGP 兼容，Provider 层暴露统一的 `CapabilitySet{policy: true, encryption: "wireguard", dualstack: true}`，上层 UI 根据能力矩阵动态显示可配置项，而不是硬编码某个 CNI 的专有字段。

### 7.3 Storage Provider 三段式

```text
StorageProfile(用户视角: "Standard Block"/"High Performance Local"/"Distributed")
   → StorageClassBinding(平台内部)
      → 外部CSI StorageClass | TopoLVM StorageClass | Rook-Ceph StorageClass
```

- `WaitForFirstConsumer` 默认开启，避免 PVC 提前绑定到错误可用区；
- 本地存储卷创建时自动写入 `nodeAffinity`，删除前由 `lst-safety-webhook`（准入 Webhook）检查是否存在于其他节点的可用副本，无副本则拦截删除并要求二次确认。

### 7.4 Accelerator Provider（整卡 + HAMi 双模型）

```text
AcceleratorPool
 ├── mode=passthrough → nvidia-device-plugin / DRA DeviceClass → 独占分配
 └── mode=shared      → HAMi vGPU → 按显存/算力比例切分
```

- 用户下单时选择"独占/共享"，后端据此路由到不同 `AcceleratorPool`，两类资源池物理隔离（不同节点标签），避免共享任务拖累独占任务的尾延迟；
- 共享模式下 HAMi 的 `nvidia.com/gpumem` 与 `nvidia.com/gpucores` limit 由平台自动计算注入，UI 只暴露"显存 GiB / 算力 %"两个语义化参数。

### 7.5 Federation Provider（Karmada）实现要点

- Karmada 控制面作为**独立 Provider 部署**，`Federation` 资源对象只保存成员集群列表与健康状态引用，不侵入普通集群资产表；
- `PlacementPolicy` 在平台侧统一建模，翻译为 Karmada 的 `PropagationPolicy` + `OverridePolicy` + `SpreadConstraint`；
- **有状态应用迁移前置检查器**（`federation/karmada/precheck`）：迁移前强制校验目标集群的存储 StorageClass 映射、镜像可达性、配额余量、数据复制状态，任一不满足则阻止迁移并给出缺口清单（直接对应需求"不允许仅凭 Deployment 迁移宣称零数据丢失"）。

### 7.6 Image/Runtime Security Provider

- 镜像安全：`Trivy Operator` 以 `VulnerabilityReport` CRD 形式产出结果，`image-security-provider` 将其归一化为平台 `ImageRiskEvent`，与 SBOM（Syft/Trivy 生成）、Cosign 签名验证结果关联，准入侧通过 Kyverno `verifyImages` + 平台风险等级策略联动拦截；
- 运行时安全：Falco 规则以 ConfigMap 分发并支持灰度（按 Namespace 标签逐步下发新规则版本），事件通过 Falcosidekick 转 OTel/webhook 进入平台 `Security Event Center`；Tetragon 作为可选增强，处理需要"阻断"（Enforce）级别的场景（Falco 仅能"告警"，Tetragon 支持内核级策略执行）。
- 响应动作分级（Observe/Alert/Contain/Enforce）由平台策略引擎统一下发，`Contain`/`Enforce` 默认关闭，启用需租户+环境级显式开关 + 二次确认。

### 7.7 Database / Middleware Provider

- 统一 `DatabaseInstance`/`MiddlewareInstance` CRD，由平台 Controller 转译为具体 Operator CR（如 `CloudNativePG.Cluster`、`Kafka.Strimzi`）；
- 服务绑定：Operator 产出的 Secret 通过 `ServiceBinding` Controller 自动注入应用 Pod（遵循 Kubernetes `ServiceBinding` 事实标准的字段布局），实现"绑定即注入凭据"的零 YAML 体验。

---

## 8. 部署拓扑与安装器实现

### 8.1 安装器技术方案

- 统一安装器 `hnb-installer`（Go + Ansible 混合：节点初始化用 Ansible Playbook，K8s 内组件用 Helm Chart + `hnbctl install` 编排），一套安装器覆盖四种拓扑；
- 安装前预检（`hnbctl preflight`）：内核参数、网卡、VLAN、路由、磁盘、时钟同步、防火墙、资源余量，输出结构化报告，不通过则阻止继续。

### 8.2 四级拓扑的关键实现差异

| 拓扑 | Kubernetes 控制面 | HNB 核心副本 | 数据库 | 关键实现 |
|---|---|---|---|---|
| 单节点 All-in-One | 单实例 etcd | 单副本 | 内嵌 PostgreSQL（同节点） | 明确 UI 角标"非高可用"；提供一键导出配置用于后续迁移到高可用模式 |
| 融合高可用 | 3 节点 etcd | 3 副本，Pod 反亲和 | Patroni 3 副本，跨节点 | 管理组件设置 `priorityClassName=system-cluster-critical` + `resources.requests` 预留，业务负载可调度到管理节点但设 `PodDisruptionBudget` 保护管理组件 |
| 分离高可用 | 管理节点池专属 3+ 节点 | 3 副本，`nodeSelector=role=management` | 独立数据节点池 | 管理节点默认 Taint `dedicated=management:NoSchedule`，业务/数据/存储/GPU/可观测节点池独立标签体系 |
| 多地域 | 每 Region 独立 K8s，元数据全局单写 | Agent Gateway 多活，API 无状态多活 | PostgreSQL 主写 + 只读副本跨 Region，或 Patroni 跨 Region 仲裁 | Operation Engine 用 PG advisory lock 做全局 leader election，避免两地同时执行互斥灾备操作 |

### 8.3 节点角色标签体系

```yaml
labels:
  hnb.io/role: management|compute|data|storage|gpu|observability
taints:
  - key: hnb.io/dedicated
    value: management
    effect: NoSchedule   # 分离高可用模式下生效，融合模式下不打
```

### 8.4 从融合到分离的平滑迁移

提供 `hnbctl topology migrate --from fused --to separated` 工具：先给目标计算节点打标签加入集群 → 逐步将业务 Pod 驱逐重调度到计算节点池（借助 `PodDisruptionBudget` 保证不中断）→ 最后为管理节点补打 Taint。全程走 Operation 状态机，可暂停/回滚。

---

## 9. 网络平面与多网实现

### 9.1 NetworkPlaneProfile 落地

- 单网模式：所有流量走同一 CNI 网络，`NetworkPlaneProfile` 只声明一个 plane；
- 多网模式：管理网/存储网/业务网通过独立 VLAN 子接口或 Bond 实现，Pod 侧使用 **Multus CNI** 挂载存储网 Macvlan/IPVLAN 附加网卡（仅数据库/存储相关 Pod 需要），业务 Pod 只挂主 CNI；
- 容灾网：独立 QoS Class + 独立带宽整形（`tc`/Cilium Bandwidth Manager），避免复制流量抢占业务带宽。

### 9.2 网络变更安全网

- 所有 `NetworkPolicy`/`NetworkPlaneProfile` 变更先经过"策略模拟器"（基于 Cilium `cilium policy trace` 或等价 dry-run）预览影响面，再落地；
- 变更记录进入审计并支持一键回滚（保留变更前的策略快照）。

---

## 10. 安全体系实现

### 10.1 身份与访问

- 统一身份：Dex 作为 OIDC Broker 对接企业 AD/LDAP/第三方 IdP，平台自身不存储用户密码；
- mTLS：所有内部服务间通信（platform-api ↔ provider、agent-gateway ↔ cluster-agent）使用平台内建轻量 CA（cert-manager + 私有 Issuer）签发短期证书，自动轮换（默认 24h TTL）。

### 10.2 多租户安全的技术强制点

1. API 层 Tenant Context 注入 + RLS 双保险（见 §4.4）；
2. 缓存 Key 规范：`tenant:{tenant_id}:project:{project_id}:...`，杜绝跨租户 Key 碰撞；
3. 对象存储路径规范：`s3://.../tenant/{tenant_id}/...`，Bucket Policy 按前缀限制；
4. 审计日志使用**哈希链**（每条记录包含前一条记录的哈希）实现防篡改，定期将哈希链根写入外部只写存储（对象存储 Object Lock/WORM）。

### 10.3 高风险操作二次确认与双人复核

Operation 引擎支持在步骤级插入 `require_approval: true` 标记（灾备切换、跨租户共享、权限提升、生产存储删除等场景默认开启），流程与 §6.4 的审批体系复用同一套 `ApprovalInstance` 模型。

---

## 11. 可观测性实现

### 11.1 数据管道

```text
应用/节点/网络/存储/GPU 指标 → OTel Collector(DaemonSet, 边采集边预聚合)
   → Prometheus/Mimir(指标, 支持多租户remote-write分片)
日志 → Fluent Bit/OTel → Loki(按租户Tenant Label分区)
链路 → OTel SDK/Auto-instrumentation → Tempo
事件/审计 → 平台自身 Outbox → ClickHouse(大规模审计查询) / PG(常规查询)
```

- Region 侧预聚合与采样：默认 1% 全量 Trace 采样 + 100% 错误 Trace 采样，指标在 Region Collector 侧做 15s 粒度预聚合后回传中心，减少跨地域带宽消耗；
- 告警：Prometheus Alertmanager 多级路由 + 平台侧告警去重/抑制/关联（自动挂载最近变更记录，即最近 30 分钟内的 Operation），加速根因定位。

### 11.2 拓扑与根因

Resource Graph（§4.2 `topology` 模块）持续消费 Outbox 事件维护实时拓扑，告警触发时自动查询"该资源 3 跳以内的最近变更 + 关联日志/链路"，形成初步根因候选（P2 阶段引入轻量异常检测模型强化，不承诺全自动根因）。

---

## 12. 容灾与双活实现

### 12.1 四层容灾技术映射

| 层 | 技术实现 |
|---|---|
| 应用资源层 | Karmada PropagationPolicy 分发无状态工作负载/配置 |
| 数据层 | 数据库/中间件原生复制（流复制、镜像队列）+ CSI 存储复制/对象存储跨区复制 + Velero 资源备份 |
| 流量层 | GSLB/DNS 权重切换（CoreDNS + 外部 GSLB Provider，如 F5 GTM/云厂商全局流量管理）或网关级流量切换 |
| 管理控制层 | DR Manager 统一编排四层动作，见 §6.5 |

### 12.2 RPO/RTO 落地校验

- 每个 `DRProtectionGroup` 绑定的 `replication_provider` 必须实现 `Observe` 返回真实的复制延迟（`replication_lag_seconds`），DR Manager 每分钟采集写入时间序列；
- 演练（Drill）在隔离网络/临时域名下全链路走一遍 `dr.drill` Operation（与 `dr.failover` 共享步骤定义，但 `traffic-cutover` 步骤指向沙箱入口），演练报告自动归档为合规证据。

### 12.3 多地域管理面双活的一致性策略

- 元数据库采用"单写 Region + 跨 Region 只读副本"作为默认（简单可靠），有更高一致性需求的客户可选择 Patroni 跨 Region 仲裁（牺牲部分写入延迟换取自动故障切换）；
- Operation Engine 的全局锁基于 PostgreSQL Advisory Lock（单写 Region 天然满足单一 leader），避免自建复杂的分布式共识组件，符合"轻量优先"的原则。

---

## 13. API 与前端实现

### 13.1 API 设计规范

- 对外 REST + OpenAPI 3.1，所有写操作要求 `Idempotency-Key` Header；
- 长任务统一返回 `202 Accepted` + `Operation` 资源引用，客户端通过 `GET /operations/{id}` 轮询或 SSE `/operations/{id}/stream` 订阅；
- 错误响应统一结构：

```json
{
  "error": {
    "code": "QUOTA_EXCEEDED",
    "message": "GPU显存配额不足",
    "request_id": "req_8f2a...",
    "hint": "请联系租户管理员调整配额，或提交资源申请单"
  }
}
```

### 13.2 前端插件化

- UI 采用 Module Federation（Webpack/Vite 插件）实现"按权限动态加载插件模块"，未启用的能力（如未安装 HAMi）对应菜单自动隐藏，避免"看得到用不了"的困惑；
- 高级参数默认折叠，专家模式提供原生 YAML 编辑器（Monaco Editor）与表单双向同步。

---

## 14. 性能与容量工程

### 14.1 关键性能保障手段

1. Read Model 分片：按 `tenant_id` 哈希分片 + 按 Region 物理分库，避免大租户拖垮小租户查询；
2. Agent 上报背压：`cluster-agent` 使用令牌桶限速上报，Informer 增量而非全量；
3. 备份/复制走 Rust `backup-transfer`：基于 `tokio` 异步 IO + `io_uring`（Linux 支持时）+ 流式压缩，相比通用语言实现可降低 30-50% 的 CPU 占用（需在正式压测中验证并写入性能验收报告，不做未经测试的量化承诺）；
4. API 侧统一分页（Cursor-based）、字段选择（GraphQL 风格的 `fields=` 参数）、连接池与熔断（基于 `resilience4j`/Go `sony/gobreaker` 等效实现）。

### 14.2 容量基线压测矩阵

对应需求规格 §13.2/13.3，压测需覆盖：管理集群数、节点总数、工作负载实例、资源对象、租户数量、组织关联数、并发审批、GPU 设备数、存储卷数、联邦成员集群数，逐项独立压测并在验收报告中列出硬件/网络/软件版本上下文，不以单一指标（如 Pod 数）外推全部容量结论。

---

## 15. 测试与验收工程

- **契约测试**：Provider SPI 使用 `buf breaking` 做 Protobuf 兼容性门禁，防止 Provider 升级破坏核心调用；
- **混沌测试**：基于 Chaos Mesh 对 API/Controller/Worker/Agent/PostgreSQL/CNI/CSI 注入故障，验证 §12（可靠性需求）与 §18（验收要求）矩阵；
- **灾备专项**：每个 Release 强制跑一次 `dr.drill` 全链路自动化用例（隔离沙箱），产出合规报告作为发版门禁之一；
- **安全基线扫描**：CI 外的独立安全流水线（不属于本平台范围，由外部工具链完成）定期对已发布镜像重新评估。

---

## 16. 研发路线图与团队组织

### 16.1 版本节奏（对应需求规格 §19，细化到可执行任务）

| 版本 | 周期 | 关键交付 | 验收门槛 |
|---|---|---|---|
| V0.1 内部里程碑 | 第1-2月 | 微内核骨架、Tenant/Org 模型、Operation Engine、单节点安装器 | 单节点部署 + 应用部署回滚打通 |
| V1 可靠核心 | 第3-5月 | 集群纳管、CNI/CSI 基础 Provider、PG/MySQL/Valkey、Kafka/RabbitMQ、Trivy、Falco、可观测三件套、融合/分离HA | §18.1/18.2/18.3/18.6/18.7 验收通过 |
| V1.5 生产增强 | 第6-9月 | 轻量审批、三网分离、本地存储、HAMi、Karmada、RocketMQ/MQTT、Tetragon、灾备保护组/演练 | §18.4/18.5/18.9/18.10/18.11 验收通过 |
| V2 高级扩展 | 第10-14月 | 同城/多地域双活、Rook-Ceph、SR-IOV/DPDK、跨集群网络、外部ITSM对接 | §18.12 全量验收 + 年度综合演练 |

### 16.2 团队建议分组

- 内核组（Operation/Plugin/Tenant/Approval）
- Provider 组（细分网络/存储/GPU、数据库/中间件、联邦/灾备）
- 安全组（身份、镜像/运行时安全、审计）
- 可观测/SRE 组
- 前端/体验组
- 平台 SRE/验收组（独立于研发，负责压测与混沌工程门禁）

---

## 17. 创新点与竞争力矩阵

| 创新点 | 技术实现 | 客户可感知的价值 |
|---|---|---|
| 组织-租户解耦的多对多模型 | `TenantOrgBinding` + OpenFGA 关系型权限 | 集团型客户可复用同一套组织架构在不同业务线间灵活复用/隔离资源，而无需为每个事业部重建组织树 |
| 数据库级 RLS 兜底隔离 | PostgreSQL RLS + 应用层 Tenant Context 双保险 | 即使代码疏漏也不发生跨租户数据泄露，安全审计更容易通过 |
| Saga 化 Operation 引擎 | 步骤级补偿 + 断点恢复 + 分布式锁租约 | 复杂的跨 Provider 编排（如"建应用+建数据库+建GPU+绑定服务"）失败后不留脏资源，运维心智负担低 |
| 四层容灾显式建模 | 应用/数据/流量/控制面分层 Provider + 就绪度门禁 | 杜绝"多集群=双活"的营销式误导，演练不达标自动降级为人工模式，真正可审计的容灾能力 |
| 整卡+HAMi 双资源池物理隔离 | 节点标签分池 | 避免共享 GPU 拖累关键独占任务尾延迟，同时把算力利用率做到极致 |
| Rust 热路径 + Go 生态主体 | gRPC 解耦，按压测结果决定是否启用 Rust 实现 | 在不牺牲 Kubernetes 生态兼容性的前提下拿到关键路径的极限性能 |
| 四级部署拓扑同一套安装器/API | Ansible+Helm 编排 + `hnbctl topology migrate` | 客户从 POC 到集团级生产的整个生命周期不用换产品、不用改造已有应用 |
| Provider 契约优先，杜绝内核硬编码 | Protobuf SPI 唯一真源 + `buf breaking` 门禁 | 客户已有的存储/安全/审批系统可以对接而不是被迫替换 |

---

## 18. 风险与应对

| 风险 | 影响 | 应对措施 |
|---|---|---|
| Provider 生态碎片化导致兼容性矩阵爆炸 | 测试成本上升 | 建立 Provider 认证测试套件（Conformance Test），仅认证通过的 Provider 标记"生产可用" |
| RLS + 应用层双重隔离带来的性能开销 | 查询延迟上升 | 对高频只读路径优先走 Read Model + Valkey 缓存，RLS 仅作为兜底防线，不作为主查询路径 |
| 多地域强一致 vs 高可用的取舍 | 客户期望落差 | 产品文档与销售侧明确"单写Region"默认模型的边界，跨Region强一致作为高阶可选项并单独报价/评估 |
| 开源组件（Karmada/HAMi/Falco等）版本迭代快 | 兼容矩阵维护压力 | Provider 化 + 独立发布节奏，核心 API 版本与 Provider 版本解耦，兼容矩阵按季度滚动验证 |
| 轻量审批引擎被业务方期望"长成"通用 BPM | 范围蔓延、内核膨胀 | 产品原则第10/11条明确边界，超出范围的诉求统一路由到外部 ITSM/BPM Provider |

---

## 19. 应用全生命周期管理详细设计（含 OAM 标准采纳方案）

### 19.1 现状与选型结论

需求规格 §10.9（APP-01~12）要求应用具备"组件、配置、密钥、路由、服务绑定统一建模，一个页面看实例/流量/日志/链路/告警/依赖"的能力。经过评审，本方案采取**分层采纳 OAM（Open Application Model）思想、不强制引入 KubeVela 作为内核依赖**的路径，原因：

1. OAM 的 Component/Trait 分离模型与"用户只关心做什么、平台负责怎么做"的产品理念高度一致，值得借鉴其**语义结构**；
2. 但完整引入 KubeVela（vela-core + CUE 模板引擎）会新增一层独立的 Controller 与 DSL 学习成本，与第14章"轻量化"、第5.1章"微内核不得编译进重型组件"的原则存在张力；
3. 因此采用"**语义借鉴、内核自研、生态可插拔**"三段式落地。

### 19.2 三段式实现方案

#### 第一段：Application CRD 按 OAM 风格自研（默认内置，V1 落地）

平台自有 `Application` CRD 内部结构参照 OAM 的 Component/Trait 分离，但由平台自身的 `application` Controller 调和，不依赖 vela-core：

```yaml
apiVersion: app.hnb.io/v1
kind: Application
metadata:
  name: order-service
  labels:
    hnb.io/tenant: t-8f2a
    hnb.io/project: proj-payment
    hnb.io/environment: env-prod
spec:
  components:
    - name: order-service
      type: webservice           # 对应 workload 类型：webservice/worker/cronjob/statefulservice
      properties:
        image: registry.example.com/order-service:1.4.0
        replicas: 3
        resources: {cpu: "1", memory: "2Gi"}
      traits:                     # 对应 OAM Trait：运维特征与业务组件解耦
        - type: gateway
          properties: {path: /api/orders, port: 8080}
        - type: hpa
          properties: {min: 3, max: 10, targetCPU: 70}
        - type: rollout
          properties: {strategy: canary, steps: [10, 50, 100]}
        - type: service-binding
          properties: {database: order-db, middleware: order-mq}
        - type: dr-placement
          properties: {placementPolicyRef: multi-region-standard}
status:
  componentStatuses: [...]
  observedTraits: [...]
  resourceGraphRef: ...
```

- `type`（workload 类型）与 `traits[].type`（运维特征类型）均通过 **Trait/Workload Definition 注册表**（类比 OAM 的 X-Definition，但用平台自有的 `TraitDefinition`/`WorkloadDefinition` CRD 实现，不引入 CUE）声明其渲染为哪些底层 K8s 对象（Deployment/Service/HPA/Gateway/Rollout CR 等）；
- 该注册表本身是**可扩展的**：新增一种 Trait（例如后续要支持 Service Mesh 流量镜像）只需注册新的 TraitDefinition + 渲染模板，不需要改动 Application Controller 内核代码，符合插件化原则；
- 服务绑定 Trait（`service-binding`）直接对接 §7.7 的 Database/Middleware Provider 产出的 Secret，实现"绑定即注入凭据"。

#### 第二段：Application Provider 抽象层（V1 即建立接口，为第三段做准备）

在 Provider SPI 之上新增一类 **Application Provider**，默认实现即第一段的自研 Controller：

```protobuf
service ApplicationProvider {
  rpc Render(RenderRequest) returns (RenderedManifests);   // Application -> K8s对象集合
  rpc Reconcile(ReconcileRequest) returns (ReconcileResult);
  rpc Rollout(RolloutRequest) returns (OperationRef);        // 灰度/蓝绿发布编排
}
```

平台核心只面向 `ApplicationProvider` 接口编程，`Application` CRD 的调和逻辑通过该接口下沉，为后续替换实现留出空间。

#### 第三段：KubeVela 作为可选插件化 Application Provider（V2 阶段，按需启用）

对于明确需要与 OAM/KubeVela 生态互通（已有 OAM 应用定义资产、需要 CUE 高级模板能力、需要 KubeVela 丰富的社区 Trait 生态）的客户，提供 **`application/kubevela` Provider 插件**：

```text
安装 kubevela Provider 插件
 → 部署 vela-core（独立 Namespace，遵循 §8.3 插件隔离要求）
 → Application Controller 检测到租户/项目启用了 kubevela Provider
 → 该租户下的 Application 转由 KubeVela ApplicationController 调和
 → 平台侧 UI 与 API 保持不变，用户无感知实现切换
 → 未启用的租户继续使用平台自研轻量实现
```

两种实现共存的关键在于：**平台对外暴露的 `Application` API 契约保持一致**，用户不会因为底层是自研 Controller 还是 KubeVela 而需要修改应用定义或操作方式——这与第2.3.2条"产品需求与具体实现分层"的原则完全一致，也是本方案在应用管理领域对 Provider 化思想的又一次贯彻。

### 19.3 与既有体系的衔接说明

| 既有体系 | 衔接方式 |
|---|---|
| Operation Engine | Application 的创建/升级/回滚/灰度全部转译为标准 Operation（§6.1），Rollout Trait 的分步骤发布直接复用 Operation 的步骤编排与补偿机制 |
| ResourceRequest/审批 | 应用部署所需的容器资源套餐通过 ResourceRequest 申请，审批通过后触发 Application 的 Operation，与 §6.4 流程一致 |
| Resource Graph | Application → Component → Trait → 底层K8s对象的关系全部写入 Resource Graph，支撑"应用页面统一展示网络/存储/GPU/联邦/灾备状态"的需求（APP-06、需求规格第15条易用性要求） |
| 联邦（Karmada） | `dr-placement`/`federation-placement` 类 Trait 直接映射到 PlacementPolicy，实现"应用定义里声明式指定容灾等级和跨集群策略" |
| 多租户 | Application CRD 强制携带 `hnb.io/tenant` 标签，Controller 调和前校验 Tenant Context，与 §4.4 一致 |

### 19.4 不采纳完整 OAM/KubeVela 作为强制内核的理由重申

1. 轻量化：核心默认场景（单节点、融合HA）不需要额外承担 vela-core 的资源开销与 CUE 模板维护成本；
2. 多租户模型适配：OAM/KubeVela 原生"Namespace 即应用边界"的心智与本方案"租户-项目-环境"三级模型不完全对齐，强制引入需要额外做语义折叠，增加复杂度；
3. 审批与 Provider 化的一致性：保持"用户看到的永远是同一套 Application API，底层实现可插拔替换"的核心设计哲学，与网络/存储/GPU等其他领域的 Provider 化处理方式保持一致，不因为"OAM 是标准"就破例把它做成不可替换的内核依赖。

---

## 20. 数据库与中间件全生命周期管理详细设计

本章对需求规格 §10.11（数据库服务）、§10.12（中间件服务）做实现层面的深化，覆盖部署、配置、升级、备份运维与高可用五个维度，并明确与 §12 站点级容灾的分工边界。

### 20.1 总体状态机

`DatabaseInstance`/`MiddlewareInstance` 不是"创建后就转手给 Operator 不再过问"，而是拥有自己的全生命周期状态机，由平台 Controller 持续调和并通过 Operation Engine 编排关键变更：

```text
Requested → Provisioning → Initializing → Ready
   ⇄ Reconfiguring（参数变更）
   ⇄ Upgrading（版本升级）
   ⇄ Scaling（扩缩容/扩容存储）
   ⇄ Maintaining（维护窗口内的计划性操作）
   ⇄ Degraded（部分副本异常，仍可用）
   → Failing Over（自动/人工故障转移中）
   → Suspending → Terminated
```

每一次状态迁移（Reconfiguring/Upgrading/Scaling/FailingOver）都建模为标准 Operation，具备预检、执行、验证、失败回滚四段式，与 §6.1 完全复用同一套引擎，而不是让各个数据库 Operator "各自为政"。

### 20.2 部署：分引擎实现映射

| 引擎 | 平台内置套餐 | 底层 Operator/实现 | 高可用拓扑 |
|---|---|---|---|
| PostgreSQL | 单实例开发版 / 标准高可用(1主2备) / 核心高可用(1主2同步备+仲裁) | CloudNativePG | 流复制，Patroni 风格的 Leader Election + 自动 Failover，同步/异步复制可配置 |
| MySQL | 单实例 / 标准高可用 / 核心高可用 | Percona XtraDB Cluster Operator（Galera 多主）或 MySQL Group Replication Operator（可切换 Provider） | Galera 多主同步复制 或 GR 单主+多副本，ProxySQL 做读写分离代理 |
| Valkey/Redis | 单实例 / 哨兵高可用 / 集群模式 | Valkey Operator（Sentinel 模式 / Cluster 分片模式） | Sentinel 自动主从切换；Cluster 模式数据分片+多副本 |
| Kafka | 单Broker / 标准(3 Broker ISR=2) / 核心(5 Broker ISR=3) | Strimzi | KRaft 模式，ISR 副本机制，`min.insync.replicas` 按套餐预设 |
| RabbitMQ | 单节点 / 镜像队列集群 / Quorum队列集群 | RabbitMQ Cluster Operator | Quorum Queue（基于 Raft）为默认推荐，取代传统镜像队列 |
| RocketMQ | 单Broker / 主从 / 多副本Dledger | RocketMQ Operator | Dledger 模式基于 Raft 的多副本自动选主 |
| MQTT(EMQX) | 单节点 / 集群模式 | EMQX Operator | 集群 Mnesia/Global 一致性 + 会话复制 |

**平台侧统一动作**：无论底层是哪种 HA 机制，平台对用户只暴露"开发/标准高可用/核心高可用"三档套餐，具体副本数、复制模式、仲裁方式由 Provider 内部按引擎最佳实践决定并在"高级设置"中可查看（默认折叠，对应第15条易用性原则）。

### 20.3 配置管理

1. **参数分级**：`ConfigParameterDefinition` 声明每个参数是否为 `dynamic`（可热更新，如 PostgreSQL 的 `work_mem`）、`restart-required`（需重启生效）、`immutable`（需重建实例，如分片数）；
2. **参数模板**：`ParameterGroup` 支持按套餐预置基线模板，租户管理员可在允许范围内覆盖，超出安全基线的参数（如关闭 SSL）需要走 §6.4 的审批流程；
3. **变更流程**：`db.reconfigure` Operation → 预检（是否 immutable、是否需要重启、是否有连接数超限风险）→ 灰度应用（多副本场景逐个应用避免同时重启）→ 验证（连接性、慢查询无异常）→ 失败自动回滚到变更前快照的参数集合；
4. **审计**：所有参数变更记录 diff、操作人、审批单号（如适用），进入审计日志。

### 20.4 升级策略

区分两类升级，分别设计流程：

**小版本/补丁升级**（如 PG 16.2 → 16.3）：
```text
预检(镜像可达性、兼容性矩阵) → 逐副本滚动重启(先备后主，主最后切换)
 → 每个副本升级后等待健康检查通过再处理下一个 → 全部完成后验证 → 失败自动暂停并告警
```

**大版本升级**（如 MySQL 5.7 → 8.0）：
```text
兼容性预检(废弃特性扫描、字符集/存储引擎兼容性) → 创建升级前全量备份并校验可恢复
 → 在只读副本或克隆实例上先行升级并验证(影子验证)
 → 维护窗口内执行主实例升级(需人工确认，默认 require_approval=true)
 → 验证通过 → 完成；验证失败 → 从预先创建的备份恢复，不做"原地失败重试"
```

大版本升级默认要求人工确认与维护窗口，不支持全自动执行，对应第17条"高风险不可逆操作不得全自动执行"的运维原则。

### 20.5 备份体系详细设计

#### 20.5.1 Backup Provider 契约

复用 §7.1 的 Provider SPI，新增数据库/中间件专属的 Backup 语义：

```protobuf
message BackupRequest {
  string instance_ref = 1;
  string backup_type = 2;        // full | incremental | pitr-log
  string storage_target = 3;      // 对象存储引用，租户隔离路径
  bool verify_after_backup = 4;   // 是否自动拉起临时实例校验可恢复性
}
```

#### 20.5.2 统一备份存储与隔离

- 所有引擎的备份统一落地到 §5 定义的对象存储，路径规范 `s3://backup/tenant/{tenant_id}/{instance_type}/{instance_id}/{timestamp}/`；
- 备份文件默认加密（使用租户专属 KMS Key，对应 §10.6 多租户安全"租户密钥支持独立 KMS Key"）；
- 备份元数据与业务数据不落在同一故障域（对应需求"备份元数据不与业务数据保存在同一故障域"），备份索引记录在平台核心 PG，实际数据在独立对象存储集群/Bucket。

#### 20.5.3 备份策略与保留

- `BackupPolicy` 按套餐预置（如"核心高可用"默认：全量每日 + WAL/Binlog 连续归档实现 PITR，保留 30 天）；
- 保留策略到期自动清理，但清理前检查是否存在依赖该备份的恢复中任务；
- **自动恢复演练**：`verify_after_backup=true` 时，平台定期（默认每周）拉起一个临时隔离实例从最新备份恢复并跑健康检查，验证结果写入 `BackupVerificationReport`，避免"备份从未验证过是否可恢复"这一常见生产事故根源。

#### 20.5.4 恢复流程

```text
db.restore Operation:
  precheck(目标位置/StorageClass映射/容量) → 创建新实例(不覆盖生产) → 从备份/PITR时间点恢复数据
   → 恢复后一致性校验(表数/关键索引/连接性) → 交付新实例引用给用户 → 原实例保持不变直至用户确认切换
```

严格遵循需求"支持恢复到新卷，避免直接覆盖生产卷"的原则，恢复默认不覆盖，切换由用户显式确认。

### 20.6 连接治理

- 服务绑定（Trait `service-binding`，见 §19.2）自动注入的连接信息默认指向**连接代理**（PgBouncer/ProxySQL/Redis Proxy）而非直连数据库实例，代理层承担连接池、读写分离、故障转移期间的连接保持；
- 代理层本身多副本部署，故障转移时代理感知新主节点并对应用透明切换（应用侧连接不中断或仅短暂重试），减少故障转移对业务连接的直接冲击；
- 连接数、慢查询、锁等待等指标经由 Database Provider 的 `Observe` 接口回传，进入 §11 可观测管道。

### 20.7 多租户共享实例的运维边界

对应 `TenantIsolationProfile` 的"共享型"档位（§9.2）：

- 共享实例内按租户分配独立账号 + ACL（数据库级/Topic级前缀隔离），配额（连接数、存储空间、QPS）按租户维度限制并可在 §11 中查看用量；
- 备份/恢复以"整实例"为单位执行，但恢复到新实例后可选择**仅导出指定租户的数据子集**（逻辑备份工具如 `pg_dump --schema` 按租户 Schema 或数据库粒度实现），避免恢复演练时波及其他租户数据；
- 增强型/专享型档位则直接使用独立实例，天然规避共享实例的运维边界问题，适用于对隔离要求更高的租户。

### 20.8 与站点级容灾（DR）的分工边界

明确职责边界，避免"数据库自身高可用"与"跨站点容灾"混淆（对应第20条建设边界原则"备份不等于双活"）：

| 层次 | 解决的故障范围 | 机制 |
|---|---|---|
| 数据库自身 HA（本章） | 单节点/单可用区内的实例、磁盘、网络抖动故障 | 引擎原生复制 + 自动主从切换（§20.2 表格） |
| DRProtectionGroup（§12） | 机房级/区域级故障 | 数据库原生的跨站点复制通道（如 PG 级联流复制、MySQL 跨机房 GR）作为 §12.1 "数据层" Provider 被 DR Manager 编排调用 |

即：`DatabaseInstance` 的高可用解决"实例挂了怎么办"，`DRProtectionGroup` 解决"整个机房/地域没了怎么办"，两者是**级联关系**而非二选一——核心高可用套餐的数据库实例本身可以再被纳入一个 DRProtectionGroup，作为跨站点容灾的数据层成员。

### 20.9 用户体验落地

对应第15条易用性要求，普通用户在创建数据库/中间件服务时只需选择：

- 引擎类型与版本；
- 套餐档位（开发/标准高可用/核心高可用）；
- 容量与存储套餐；
- 备份策略（默认套餐已预置，可调整保留天数）；

高级设置（副本数、复制模式、参数模板、连接代理配置）默认折叠，服务创建完成后自动生成服务绑定凭据供应用一键绑定，全程不需要编写 YAML 或直接操作 Operator CR。

---

## 附：与需求规格的映射关系说明

本方案的模块划分与《需求规格说明书 V3.1》第9章对象模型、第10章功能需求编号（ARCH/CORE/TEN/APR/K8S/DEP/FED/CMP/NET/NPL/STO/LST/CEPH/GPU/HAMI/APP/IMG/DB/MW/MS/OBS/DR）、第11章安全需求、第12章可靠性需求、第18章验收要求逐一对应，未在本文重复罗列编号级需求，实施团队在详细设计评审时应以两份文档联合评审、交叉勾稽为准。
