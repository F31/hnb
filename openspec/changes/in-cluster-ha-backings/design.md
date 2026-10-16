# Design: in-cluster HA backings

## Context

`lite-ha.yaml` 目前显式 `postgresql.enabled=false` / `nats.enabled=false`，把 HA 档的 PostgreSQL 与 JetStream 交给外部托管；`ProfileSpec.Backings` 只有 `bundled|external`。`charts/postgresql` 与 `charts/nats` 都是单副本 StatefulSet，无复制与选主。`standardize-hnb-deployment-toolkit/design.md` 将 HA 档定义为“外部服务/存储 + 独立认证的 HA 后端”。

用户诉求：单节点 Minimal 以及 Lite/Standard/Pro（HA）都要支持把 PostgreSQL、JetStream **部署在控制节点内部**，且 HA 档需要真正的高可用（而非退回单实例）。

约束：四平面不得共享数据库、不得执行旁路、不得代理数据面；NATS 不作为状态机/Secret/大文件数据面；生产镜像固定 digest；Secret 只经 SecretReference。

已定决策（用户）：CloudNativePG Operator；HA 档默认 in-cluster、external 可选；固定控制节点并容忍 taint；JetStream 复用 nats 子 chart 改 3 节点集群。

## Goals / Non-Goals

**Goals:**
- 引入 `in-cluster-ha` backing：PG（CloudNativePG，主备+自动故障转移+备份恢复）与 JetStream（3 节点集群、每节点 PVC、流 R3）在控制节点内部署。
- HA 档默认 in-cluster，external 显式可选；Minimal 保持单实例 bundled。
- 扩展 backing 偏序/超集判定、release-lock 声明与升级/扩容执行链。
- 保持证据门禁语义：能力存在 ≠ 认证；HA 仍须 ≥3 故障域演练与 RPO/RTO 实测后才 certified。

**Non-Goals:**
- 不改变 Minimal 单实例默认与既有 `bundled`/`external` 行为。
- 不引入 Bitnami/自建 Patroni 作为主路径；不改造四平面业务契约。
- 不在本 change 内认证任何 HA 档（需独立环境与演练）。
- 不承诺控制节点单机三 Pod 即 HA（故障域必须是真节点）。

## Architecture

```
                 ┌────────────── control-plane nodes (>=3, taint tolerated) ──────────────┐
                 │                                                                        │
   hnb-system    │  CloudNativePG Operator (subchart, pinned)                             │
   namespace     │        └─ Cluster/hnb-pg  (3 instances, anti-affinity by hostname)       │
                 │               ├─ primary  (R/W)   PVC-0   [postgresql-hnb-pg-1]         │
                 │               ├─ replica   PVC-1  [postgresql-hnb-pg-2]                  │
                 │               └─ replica   PVC-2  [postgresql-hnb-pg-3]                  │
                 │        └─ Backup: barmanObjectStore 或 VolumeSnapshot (PITR)             │
                 │                                                                        │
                 │  nats (subchart HA branch)                                             │
                 │        └─ StatefulSet nats (replicas=3, headless, --routes)             │
                 │               ├─ nats-0  PVC  (JetStream, R3)                           │
                 │               ├─ nats-1  PVC                                            │
                 │               └─ nats-2  PVC                                            │
                 │                                                                        │
                 │  hnb app components (apiserver/worker/platform-api/... replicas=3)      │
                 └────────────────────────────────────────────────────────────────────────┘
   planes: platform / market / artifact  ->  each its own PG database on the SAME PG cluster
                                           (separate DB + role + Secret; no cross-plane sharing)
```

服务发现：PG 经 `hnb-pg-rw`（primary Service）/`hnb-pg-ro`，JetStream 经 `nats-headless` + `nats` ClusterIP；客户端仍只消费 existing Secret 中的 DSN/账号。

## Decisions

### D1 — backing 三值模型 `bundled | in-cluster-ha | external`
`ProfileSpec.Backings` 扩展为三值。偏序：`bundled`(单实例) < `in-cluster-ha` ≈ `external`（两者皆 HA）。`SupersetOf` 的 backingRank 依此：`bundled=0`，`in-cluster-ha=1`，`external=1`，因此 `external<->in-cluster-ha` 互不为降级（是同等级再背靠），而任一回退到 `bundled` 为降级。
**替代方案**：布尔 `HA bool` —— 丢失“谁托管”的信息，无法表达迁移与责任边界，拒绝。单一 `hasExternalBacking` —— 无法区分 in-cluster HA 与单实例 bundled，拒绝。

### D2 — PostgreSQL 采用 CloudNativePG Operator
CloudNativePG 提供 K8s 原生主备、自动 failover、`Cluster` CRD、内置备份/PITR（对象存储或卷快照），与 OBS-003/004 证据直接对齐；每副本独立 PVC 与反亲和天然落在不同控制节点。
**替代方案**：Patroni 自建（多一个 DCS：etcd/consul，运维面大）；Bitnami `postgresql-ha`（第三方 chart 与其镜像，供应链面扩大）；轻量自研主备（HA 健壮性最低，不满足“真实 HA”）。均拒绝。
Operator 部署：作为 `hnb` chart 的**可选子 chart 依赖**（`postgresql.operator.enabled`），镜像与 CRD 固定 digest；也支持 `enabled=false` + 预装 Operator（企业已有平台团队纳管时）。

### D3 — JetStream 复用 nats 子 chart 改 3 节点集群
官方 `nats-server` 原生支持集群（`--cluster_name` + `--routes`）与 JetStream meta/stream 复制，无需新 operator。子 chart 增 `mode: single|cluster`：
- `replicas=3`、headless Service、`--cluster nats://nats-headless:6222`、`--routes` 指向同 headless DNS；
- 每 Pod 独立 PVC（原 storage 段落按 `volumeClaimTemplates`）；
- 流/消费者副本因子 R3，`jetstream.domain` 唯一；
- 账号（三平面 user/password）由同一 config Secret 渲染，向全节点一致。
**替代方案**：NACK operator —— 声明式强但引入 operator 依赖，收益有限，拒绝。

### D4 — 固定控制节点 + 容忍 taint，前置 ≥3 控制节点
PG/JetStream Pod 使用 `nodeAffinity` 选择 `node-role.kubernetes.io/control-plane`（K3s 为 `node-role.kubernetes.io/control-plane` / Kubeadm 同）并 `tolerations` 容忍 `node-role.kubernetes.io/control-plane:NoSchedule`；`topologySpreadConstraints`/podAntiAffinity 按 `kubernetes.io/hostname` 打散。
前置校验：`in-cluster-ha` 模式要求**可调度控制节点 ≥3**，否则 `ProfileOverlay`/`EvidenceGate` 拒绝并返回稳定原因码（`HA_CONTROL_NODES_INSUFFICIENT`）。
**替代方案**：任意节点/专用 worker —— 用户明确要“控制节点内部署”，拒绝；两者可切换的 placement 参数 —— 测试面更大，留作 Open Question。

### D5 — HA 档默认 in-cluster，external 显式可选
`lite-ha.yaml`/`standard-ha.yaml`/`enterprise.yaml` 不再 `enabled=false`；新增 `backings: in-cluster-ha` 默认，`--backing external` 显式切换（overlay `backings: external` 保留旧行为）。Minimal 不变。
**兼容性**：这是语义型 BREAKING；由于尚无 HA 档已认证安装，实际影响限于未认证路径与文档。

### D6 — 备份/恢复与 DR 责任内移
背靠为 in-cluster 时，平台自身负责 PG 与 JetStream 的备份与恢复演练：PG 用 CloudNativePG 的 `Backup`/对象存储或卷快照，JetStream 用 `nats stream backup`/`nats account backup`；RPO/RTO 实测（OBS-003）与故障演练（OBS-004）由平台出具。external 时责任仍在外部（沿用现契约）。

### D7 — 证据门禁不变
`EvidenceGate` 仍要求 `Profiles[<档位>].status=certified` + route certified（+ offline 若声明）。`in-cluster-ha` 只是把 HA 的“能力”变得可部署；认证仍需 ≥3 独立故障域的五类故障演练与 RPO/RTO 实测。`OSAdaptations` 与 `ProfilePaths` 相应扩展。

### D8 — release-lock 增加 backing 维度
`release-lock/v2` 的 `profiles[<name>]` 增 `backings: [bundled|in-cluster-ha|external]`（允许集合，首项为默认）；`supportedUpgrades` 增可选 `backingChange: true|false` 声明是否允许再背靠迁移。v1 读取兼容保持。

## Data Model

```
ProfileSpec.Backings: string  // bundled | in-cluster-ha | external   (was bundled|external)
HAProfile{IndependentDomains,Quorum}                              // 需要 >=3 控制节点时 quorum=2
StorageProfile per component (in-cluster-ha):
  postgresql.cluster.instances=3  -> 每实例 PVC (sizeGi, className)
  nats.replicas=3                 -> 每节点 PVC (sizeGi, className)
release-lock/v2:
  profiles[<name>] = {requiredCapabilities, minKubernetesVersion, backings:[...]}
  supportedUpgrades[] += {from, to, migrationChain, backingChange?:bool}
```

Runtime 对象：CloudNativePG `Cluster`（1）+ `Backup`（按策略）；`StatefulSet/nats`（replicas=3）。Secret：沿用 `hnb-<plane>-pg`（DSN/password）与 `hnb-<plane>-nats`（user/password），由 CloudNativePG 生成的 owner Secret 或 existingSecret 绑定，**绝不** 写入 values。

## API / Event Contracts

- CLI：`hnbctl install|upgrade --backing <bundled|in-cluster-ha|external>`；`hnbctl upgrade --to <profile>` 复用；`hnbctl scale --component postgresql|nats --replicas`（in-cluster 时映射到 Cluster instances / StatefulSet replicas，受 quorum 护栏）。
- 稳定原因码：`HA_CONTROL_NODES_INSUFFICIENT`、`BACKING_CHANGE_NOT_SUPPORTED`、`IN_CLUSTER_HA_UNCONFIGURED`、`OPERATOR_NOT_INSTALLED`。
- 事件/Operation：再背靠（backing migration）建立平台 Operation 行，状态机上提供 `data-plane-rebacking` 阶段；事件 `platform.backing.migrated`（含 from/to、operationId、correlationId），审计可追踪。

## State Machine

backing 现状 → 目标（仅允许同等级或向上）：
```
bundled         --(scale-up to HA, 需 >=3 控制节点)--> in-cluster-ha
external        --(re-backing)-----------------------> in-cluster-ha   [backingChange]
in-cluster-ha   --(re-backing)-----------------------> external        [backingChange, 可选]
in-cluster-ha   --(降级)------------------------------> bundled         REJECTED (PROFILE_NOT_SUPERSET)
external        --(降级)------------------------------> bundled         REJECTED
```
`data-plane-rebacking` 阶段：`preflight -> provision-target-backing -> dual-run/verify -> cutover -> verify -> cleanup`；不可逆点后进入 `PhaseBackup`/`PhaseInfraDelta` 语义（沿用 receipt 既有阶段）。

## Failure Modes

- 控制节点不足 3 或写调度失败 → `HA_CONTROL_NODES_INSUFFICIENT`，不落半套拓扑。
- Operator 未安装/CRD 缺失 → `OPERATOR_NOT_INSTALLED`，拒绝而非静默降级为单实例。
- 控制节点故障（1/3）：PG 由 CloudNativePG 触发 failover；JetStream 由 meta leader 重选，R3 流保持可读写（quorum 2）。
- 备份目标（对象存储）不可达：备份失败不影响在线服务，告警并保留本地快照；恢复演练失败即维持未认证。
- taint/节点回滚移除控制节点：Pod 重新调度不满足反亲和 → 保持 Pending 并告警，不降级数据面。
- 再背靠中途失败：dual-run 阶段可回退到原 backing；cutover 后失败走 Runbook 与 Operation 补偿，不删原数据直到 verify 通过。

## Evaluation

- **租户隔离**：四平面各自独立 PG database + role + Secret，仍无跨平面共享；JetStream 按平面账号隔离 subject。未改契约。
- **Secret**：PG 凭据由 CloudNativePG owner Secret 或 existingSecret 提供，JetStream 账号沿用 existing Secret；不写入 values/日志/release metadata。
- **供应链**：CloudNativePG Operator 镜像与 CRD、nats-server 镜像均固定 digest 并纳入 release-lock；`signed`/SBOM 沿用 security-supply-chain。
- **权限**：Operator 需要 CRD/webhook/Secret 读权限，最小化 RBAC 并单列；控制节点 taint 容忍显式声明并审计。
- **审计**：再背靠为 Operation 行 + 事件，`Who/What/When/from-to` 可追。
- **性能预算/容量**：每控制节点新增数据面资源与 PVC，门限由控制节点压测确定；下列为按当前子 chart 单价（`charts/*/values.yaml`）的**声明量规划基线**，非实测 RSS/峰值。

### Footprint baseline（in-cluster HA 数据面增量）

单容器单价：PostgreSQL `150m / 256Mi`（limit `1000m / 1024Mi`，PVC 默认 20Gi）、JetStream `50m / 128Mi`（limit `250m / 256Mi`，PVC 默认 10Gi）。

| 组件 | 副本 | CPU req / 内存 req | limits | PVC |
|---|---:|---:|---:|---:|
| PostgreSQL（CNPG `Cluster`） | 3 | 450m / 768Mi | 3000m / 3072Mi | 60Gi |
| JetStream（nats 集群，每节点 PVC） | 3 | 150m / 384Mi | 750m / 768Mi | 30Gi |
| CloudNativePG Operator | 1 | ~100m / 128Mi | ~500m | — |
| **合计（集群级）** | — | **~700m / ~1280Mi** | — | **~90Gi** |

- 依 `ICHB-002` 反亲和/打散分摊到 **≥3 控制节点**，每节点约 1 个 PG + 1 个 JetStream（≈233m / 427Mi requests）外加一份应用副本。
- 相对现有 Minimal 基线（1725m / 2624Mi / 30Gi，见 `docs/deployment/minimal-footprint-audit.md`）：CPU requests 约 +40%，内存约 +49%，PVC 默认尺寸约 ×4。
- 采用 small-disk 口径（PG 2Gi、JetStream 1Gi/实例）时 PVC 由 ~90Gi 降至 **~9Gi**，requests 不变；长期数据留存另计。
- 未含镜像体积（postgres ~400MB、nats-scratch ~20MB、cnpg-operator ~50MB）与 WAL/JetStream 增长；控制节点若同时承载 etcd，磁盘 IO 争用是被 `ICHB-002` 容量门限与压测覆盖的主要风险。
- **升级/回滚**：backing 迁移经 release-lock 声明与 `upgrade`/`scale` kind 可恢复；降级被拒。
- **灾备**：in-cluster 时平台自担 PG/JetStream 备份恢复与 RPO/RTO（OBS-003），跨域仍由 DRProtectionGroup 编排。
- **可观测**：CNPG 指标 + JetStream `/jsz`/流消费者指标纳入统一遥测。
- **跨平面**：本设计无数据库共享、无执行旁路、不代理数据面——应用仍只经平台 API/Operation 写入，NATS 仅传输。
- **Provider/RuntimeTarget/Gateway/Edge**：本 change 不修改这些契约，故不附兼容矩阵与 Conformance 计划（不适用）。

## Migration Plan

1. chart：新增 CloudNativePG 子 chart 依赖（可选）与 `charts/postgresql`/`charts/nats` 的 HA 分支；overlays 默认 `backings: in-cluster-ha`。
2. toolkit：`Backings` 三值 + rank/SupersetOf + `HA_CONTROL_NODES_INSUFFICIENT` 前置校验；release-lock backing 维度；planner/executor 纳入 backing 迁移阶段。
3. evidence/docs：扩展 `OSAdaptations`/`ProfilePaths` 维度；更新 `hnbctl-interface.md` 与形态说明。
4. 回滚：overlay 切回 `backings: external`/`bundled`；已迁移数据面经 Operation 补偿回到原 backing；无自动回退。

## Risks / Trade-offs

- [控制节点承载数据面影响控制面稳定性] → 反亲和 + 资源 requests/limits + 监控；在容量门限确认前不宣称生产可用。
- [CloudNativePG 引入 operator/webhook 故障面] → 可选依赖 + 预装 Operator 路径；webhook 失败 fail-closed 不降级数据面。
- [JetStream 3 节点在控制节点上网络/IO 争用] → 独立 PVC + 流 R3 + `topologySpreadConstraints`。
- [backing BREAKING 语义] → 保留 `bundled`/`external`，以显式 `--backing` 切换并出迁移文档。
- [认证滞后] → 明确“可部署 ≠ 已认证”，HA 仍 blocked 直到演练证据。

## Open Questions

- placement 是否也支持“任意节点/专用 worker”并以参数切换（当前固定控制节点）？
- PG 备份目标：对象存储（barmanObjectStore）与卷快照（CSI TopoLVM）何者为主？TopoLVM 的 VolumeSnapshotClass 尚未验证。
- 控制节点数 ≥3 是否为硬前置，还是允许 3 副本跨“控制+worker”混布以满足 quorum？
- `enterprise.yaml` 的 5 副本核心组件是否要求数据面也扩到 5 实例，或维持 3？
