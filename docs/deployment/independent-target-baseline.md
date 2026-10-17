# S2/S3 独立业务目标与兼容矩阵基线（S0 1.3）

状态：**管理基座与真实 CSI/SC 已观察；独立业务目标、Gateway controller 与
≥3 故障域仍 blocked**。本文件固定已有事实和待补条件，不代表 S2/S3 验收。

观察时间：2026-10-17（云主机 `hnb-minimal-test`，只读命令）。

## 1. 管理基座（已观察）

| 项 | 已观察值 | 说明 |
|---|---|---|
| OS / 架构 | Ubuntu 26.04 LTS / amd64 | 未认证 bootstrap OS；仅管理基座运行事实 |
| Kubernetes | K3s `v1.36.5+k3s1` | 单控制平面，单节点 |
| 容器运行时 | `containerd://2.3.4-k3s1.36` | |
| 节点 | `hnb-minimal-test`，`10.0.0.17`，4 vCPU / 7680Mi | 无外部 LB/Ingress |
| 网络 | K3s 默认网络；未安装 Gateway API CRD/controller | `gatewayclass` 资源不存在 |
| Ingress | 无 Traefik/Ingress controller；Portal 仅 ClusterIP | 不能作为外部入口 |
| 存储 | TopoLVM CSI `topolvm.io` 0.42.0（见下） | loop VG；非生产存储 |

## 2. 真实 CSI / StorageClass（已观察，管理基座）

| 项 | 值 |
|---|---|
| CSI Driver | `topolvm.io`，`CSIDriver` 存在，`storageCapacity=true` |
| 镜像 | `ghcr.io/topolvm/topolvm-with-sidecar:0.42.0@sha256:719ab11d974af827d12ab148ca9c070babcb21980b8c7f30497c6bde81cdce4b` |
| StorageClass | `hnb-topolvm` |
| 参数 | `Retain`、`WaitForFirstConsumer`、`allowVolumeExpansion=true` |
| 默认 SC | `local-path`（另存，不可用于业务验收） |
| Snapshot | 未安装 `VolumeSnapshotClass`/`VolumeSnapshot` CRD |

**边界**：这是管理基座真实 CSI，不等于独立业务目标已具备同版本/同能力后端；
`local-path` 不是可替代业务 CSI。

## 3. 独立业务目标（blocked）

| 条件 | 当前 | 要求 |
|---|---|---|
| 独立目标 | 无；当前只有同一单节点管理集群 | 与 `hnb-minimal-test` 不同的 cluster ID/kubeconfig/Agent 身份；禁止自导入 |
| 可调度资源 | 单节点 4C/7680Mi | 至少 1 个独立 worker（S2）；S3/S6 另需容量与故障域 |
| CSI/SC | 管理基座 TopoLVM | 业务目标固定 CSI/SC 版本、摘要、容量与卷模式；验证真实供给/读写 |
| Gateway controller | **无** | 选定一种真实 Gateway API 实现，固定版本/镜像 digest/CRD/网络/TLS |
| 外部入口/TLS | 无外部 LB/Ingress | 固定域名、DNS、证书 Secret、入口地址与可达性探针 |
| 网络条件 | 单机默认网络 | 记录 CNI、东西/南北向策略、出站代理/镜像仓库条件 |

## 4. ≥3 故障域演练环境需求

为满足 RTLOOP-001、OBSLOOP-002 与 S6 故障演练，至少提供：

- 3 个可调度节点/故障域（不同 zone、host 或等效独立故障边界），每域 ≥4 vCPU/8GiB；
- 业务目标独立于管理基座；控制面/数据面故障不共享同一物理主机；
- 每域固定 CSI/SC（含快照/扩容能力）与真实卷；至少 3×100GiB 可用；
- 固定 Gateway controller/CRD/镜像 digest，至少 2 个入口副本（若支持）；
- 可注入节点、控制面、PG 主实例、JetStream 节点故障的权限与维护窗口；
- 时钟同步、镜像仓库/代理、DNS/TLS 与外部探针入口；
- 记录 topology key/zone ID，禁止用同一节点上的 label 伪造 3 故障域。

## 5. 首轮兼容矩阵

| 组件 | 版本/身份 | 管理基座 | 独立业务目标 | 状态 |
|---|---|---|---|---|
| Kubernetes | K3s `v1.36.5+k3s1` | 已观察 | 未提供 | partial |
| CSI | TopoLVM `0.42.0` / `sha256:719a…ce4b` | 已观察/已用 | 未固定 | partial |
| StorageClass | `hnb-topolvm`（Retain/WFFC/expand） | 已观察 | 未固定 | partial |
| Snapshot | 无 CRD | 未安装 | 未固定 | blocked |
| Gateway API | 无 CRD/controller | 未安装 | 未固定 | blocked |
| Ingress/TLS | 无外部入口 | 未安装 | 未固定 | blocked |
| CNI | K3s 默认，未单独冻结 | 已运行 | 未固定 | partial |
| 外部探针 | 无可达入口 | 未提供 | 未提供 | blocked |

## 6. 待决策

1. 独立业务集群由谁提供、如何授权接入、cluster ID/租户/目标类型。
2. Gateway 实现与版本（Gateway API 或受控 Ingress 替代方案）及 TLS/域名/DNS。
3. 业务 CSI/SC 的正式版本、摘要、快照/扩容能力与容量。
4. 3 故障域的实际提供方、topology key、故障注入权限与维护窗口。

在这些条件满足前，1.3 与 S2/S3 实机任务保持 blocked；不得用管理基座单节点或
`local-path` 冒充独立业务目标/HA。
