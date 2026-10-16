# 控制节点内部的高可用数据面（in-cluster HA backings）

本文说明 PostgreSQL 与 JetStream 在控制节点内部署的高可用形态、backing 选择、
控制节点放置、备份恢复 Runbook 与卸载保留语义。形态 overlay 的事实源见
[`deploy/charts/hnb/overlays/README.md`](../../deploy/charts/hnb/overlays/README.md)；
命令与原因码见 [`hnbctl-interface.md`](hnbctl-interface.md)；资源体积基线见
[`minimal-footprint-audit.md`](minimal-footprint-audit.md)。

## 适用范围与状态

- Minimal 保持单实例 `bundled` 数据面；LiteHA/StandardHA/Enterprise 默认使用
  **in-cluster-ha**，可显式选择 `external`。
- 渲染成功 **不等于** 认证。HA 档在具备独立的故障域/主实例故障/JetStream 节点
  故障演练与 RPO/RTO 证据前保持 **未认证**，安装仍被能力证据门禁阻断
  （`cmd/hnbctl/internal/toolkit/evidence.go` 的 `Profiles`/`ProfilePaths`）。

## backing 选择

三值 `global.backings`：

| 值 | 含义 | 适用 |
|---|---|---|
| `bundled` | 单实例 PostgreSQL + file JetStream | Minimal/Startup 默认 |
| `in-cluster-ha` | 控制节点内 CloudNativePG（3 实例）+ 3 节点 JetStream | HA 档默认 |
| `external` | 集群外托管 PG/JetStream（须关内置子 chart） | HA 档显式可选 |

偏序 `bundled < in-cluster-ha ≈ external`：`in-cluster-ha` 与 `external` 互不为降级，
回退到 `bundled` 被拒绝（`PROFILE_NOT_SUPERSET`）。再背靠（backing 迁移）须在发布锁
`supportedUpgrades[].backingChange: true` 声明，否则 `BACKING_CHANGE_NOT_SUPPORTED`；
`profiles[].backings` 声明该档允许的 backing 集合。

选择方式：overlay 默认值、`hnbctl install|upgrade --backing <...>`、`release.lock.yaml`。
`external` 时必须同时关闭内置子 chart：

```yaml
global: {backings: external}
postgresql: {enabled: false}
nats: {enabled: false}
```

## Chart 结构门与前置

`templates/profile-gate.yaml` 对 HA 档做结构校验（渲染期）：

- `in-cluster-ha` 需要 `global.controlPlaneDomains >= 3`，否则
  `HA_CONTROL_NODES_INSUFFICIENT`；
- `in-cluster-ha` 需要内置 PostgreSQL/JetStream 均启用，否则 `IN_CLUSTER_HA_UNCONFIGURED`；
- `external` 需要内置两者均关闭，否则 `EXTERNAL_BACKING_MISCONFIGURED`；
- HA 档不得使用单实例 `bundled`，否则 `HA_BUNDLED_NOT_HA`。

计划侧另有观测前置：`hnbctl upgrade|scale --control-node-count N`（0=未观测跳过）
与 `--operator-ready true|false`（省略=未观测跳过），分别产生
`HA_CONTROL_NODES_INSUFFICIENT` 与 `OPERATOR_NOT_INSTALLED`。

## 控制节点放置与拓扑

- PostgreSQL 渲染为 CloudNativePG `Cluster`（默认 `instances: 3`，每实例独立 PVC）；
  `spec.affinity` 开启 `enablePodAntiAffinity`（`topologyKey: kubernetes.io/hostname`）
  并加 `node-role.kubernetes.io/control-plane` 的 `nodeAffinity` 与 taint 容忍。
- JetStream 渲染为 3 节点 StatefulSet（headless Service `nats-headless` + cluster routes、
  `jetstream.domain`、`volumeClaimTemplates` 每节点 PVC），同样带 host 反亲和与控制面
  容忍；流需以 R3 创建（流级设置，非服务端配置）。
- `placement.controlPlane=false` 可关闭上述控制节点约束（仅用于测试）。

## 凭据

凭据一律来自 existing Secret，绝不写入 values 或 release metadata：

- PostgreSQL：`adminSecret`（superuser）+ 每平面 `existingSecret`（`password`）；
  CloudNativePG 通过 `managed.roles` 与 `bootstrap.initdb` 收敛角色与库。
- JetStream：`tls.existingSecret` + 每平面 `existingSecret`（`user`/`password`）。

## Operator 安装与预装路径

`cloudnative-pg` 子 chart 是**薄封装**：仅安装 operator 工作负载（ServiceAccount、
ClusterRole/Binding、controller Deployment、webhook Service/配置），**不内置 CRD**。
CRD 是前置，须先用与 `image` 相同版本/digest 的 operator release 应用。

- `cloudnative-pg.enabled=true`（默认）：安装薄封装 operator；仅当
  `global.backings=in-cluster-ha` 时才渲染。
- `cloudnative-pg.enabled=false`（预装路径）：CRD 与 controller 均由外部管理，
  chart 只渲染 `Cluster` CR。
- operator 镜像须由 release lock/bindings 固定 digest（空 digest 会被渲染助手拒绝）；
  未就绪时升级/扩容计划返回 `OPERATOR_NOT_INSTALLED`。

## 备份恢复 Runbook

背靠为 `in-cluster-ha` 时，平台自担 PG 与 JetStream 的备份与恢复，并出具 RPO/RTO 实测
（OBS-003/004）：

1. PostgreSQL：CloudNativePG `Backup` 到对象存储（barmanObjectStore）或卷快照；恢复用
   `Cluster` 的 recovery 引导或 PITR。无外部对象存储时可先用逻辑备份：
   `pg_dump -Fc` → 恢复到 scratch 库 → 比对表数 → 清理（非破坏）。
2. JetStream：`nats stream backup <stream> <dir>` / `nats account backup`（在线），
   恢复到 scratch 账号/流后比对消息与消费者计数。
3. 身份材料：对 `/var/lib/hnb-*/identity/` 打包并逐文件 sha256 校验。
4. 记录实测 RPO（故障前最后已提交消息）与 RTO（恢复开始至流/消费者健康）。

已有的单机演练证据：
[`20261016-backup-restore-drill.md`](../acceptance/reports/20261016-backup-restore-drill.md)
（PG 130 表往返 + 6 身份材料字节一致）。跨域/跨集群 DR 由 DRProtectionGroup 编排。

## 卸载与保留语义

- 默认保留数据（PVC/备份/Secret）与 CloudNativePG `Cluster`；`--delete-data` 需 TTY 或
  `--yes` 显式确认，且交服务端契约执行。
- 卸载不静默删除持久化数据；未删除的资源会在计划中列出。
- Operator 生命周期（1.2）：可选子 chart 依赖或预装；缺失时 `OPERATOR_NOT_INSTALLED`，
  不静默降级为单实例。

## 资源体积基线

in-cluster HA 数据面增量（声明量，非实测，默认子 chart 尺寸）：集群级约
**700m / 1280Mi requests、90Gi PVC**；按反亲和分摊到 ≥3 控制节点，每节点约
233m / 427Mi。small-disk 口径可降至约 9Gi PVC。详见 design 的 Footprint baseline。

## 示例

```bash
# 渲染 HA（结构门通过；仍未认证，安装会被证据门禁阻断）
helm template hnb deploy/charts/hnb -f <bindings>.yaml \
  -f deploy/charts/hnb/overlays/minimal.yaml \
  -f deploy/charts/hnb/overlays/lite-ha.yaml > ha.yaml

# 规划升级（给出观测前置）
hnbctl upgrade -f hnb.yaml --to LiteHA --control-node-count 3 --operator-ready true

# 扩容数据面（in-cluster）
hnbctl scale --profile LiteHA --component postgresql --replicas 5 --control-node-count 3
```
