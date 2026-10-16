# 完整 Minimal 真机部署与 TopoLVM 存储验证

日期：2026-10-15。目标是把完整 Minimal（19 个工作负载，不裁剪功能）部署到全新云主机，
并验证 TopoLVM 提供 PVC 动态供给与在线扩容。

## 1. 主机与底座

| 项目 | 实测值 |
|---|---|
| 主机 | 43.142.101.136，`4 vCPU / 7680Mi / 118Gi` ext4 |
| 操作系统 | Ubuntu 26.04 (Resolute Raccoon)，kernel 7.0.0-30-generic |
| 内核架构 | linux/amd64 |
| 底座 | k3s v1.36.5+k3s1，containerd，`--secrets-encryption`，禁用 traefik/servicelb |
| 安装来源 | 官方 release 二进制 + `k3s-airgap-images-amd64.tar.zst`，经 sha256sum 校验 |
| boot_id | ce66cc18-6426-404e-bb74-5d4fdf0222de |
| 验证时间 | 2026-10-15T05:21:19Z |

Ubuntu 26.04 不在 `CurrentEvidence.CertifiedOS`（仅 ubuntu-24.04 / rocky-10.1）内，
因此本次结果**不构成 OS 认证**，仅作为功能与容量证据。

后续已新增 [Ubuntu 26.04 路线级 OS 适配与滚动部署复验](20261015-ubuntu2604-adaptation.md)；
以该报告和版本化 JSON 为当前 OS 兼容范围依据，仍不推导 kubeadm/bootstrap 认证。

访问方式：本机无直连 SSH（TCP 22 可连但握手前断开），全部经 `127.0.0.1:7892`
的 HTTP CONNECT 代理；主机出网同样经由反代端口 `127.0.0.1:17892`。

## 2. 完整 Minimal 部署结果

**19/19 工作负载 Running，0 重启。** 全部镜像由当前工作树源码构建（`FROM scratch` +
钉住 digest），经 `ctr images import` 离线导入，运行时 `imagePullPolicy: Never`。

| 组件 | 状态 | 组件 | 状态 |
|---|---|---|---|
| apiserver | Running | platform-api | Running |
| operation-worker | Running | extension-controller | Running |
| app-market | Running | cluster-router | Running |
| tunnel-server | Running | plugin-worker | Running |
| runtime-target-lifecycle-provider | Running | gateway-provider | Running |
| kubernetes-provider | Running | cilium-provider | Running |
| network-registry | Running | alert-manager | Running |
| cluster-agent | Running（隧道已连接） | rbac-syncer | Running |
| portal | Running | postgresql-0 | Running |
| nats-0 | Running | | |

### 全新安装复验（同一台主机，删除 namespace 后重装）

首轮部署用的是首次安装时创建的 20Gi/10Gi local-path 卷；为验证**新装默认值**，
删除 namespace 与旧卷后做了完整全新安装，并切换到 TopoLVM StorageClass。

结果：**19/19 Running，PostgreSQL 2Gi / JetStream 1Gi 均由 TopoLVM 供给**，
功能复验一致（workspaces 200、tenants 200、products 403、agents 200 且心跳正常）。
稳态占用 133m CPU / 1386Mi。

此过程暴露了两个只有全新安装才会遇到的真实缺陷。

## 2.1 全新安装暴露的缺陷

1. **JetStream 流预留超出小卷容量，operation-worker 无法启动**。
   三个流的 `MaxBytes` 硬编码为 1GiB/1GiB/5GiB，合计 7GiB，而 1Gi 卷在
   JetStream 实际可用仅 **718MiB**（它会预留约四分之一作headroom），
   导致 `nats: API error: code=500 err_code=10047 insufficient storage resources available`。
   修复（`cmd/operation-worker/internal/nats/worker.go`、`relay.go`、
   `cmd/gslb-controller/internal/consumer/consumer.go`）：单流上限降为 128MiB，
   domain-events 降为 256MiB（两端保持一致）。`MaxBytes` 只是上限而非预留，
   实际增长由既有的 `MaxAge`（7/30 天）约束。

   **附带发现**：JetStream 的 `reserved_storage` **不会因`CreateOrUpdateStream`
   缩小上限而释放**。一旦某流曾以过大上限建立，该份额会被永久占用，
   即使重启 server 或重建 Pod 也无效，只能删除该流或换新卷。
   这与"PVC 不能缩容"是同一类约束：容量一旦向上扩张就不能回收。

2. **market 平面数据库缺少迁移，app-market 等组件永久 not-ready**。
   `pkg/db` 为 platform / market / artifact 三个平面各自维护独立 ledger，
   首轮只迁移了 platform，导致 `app-market` 持续报
   `relation "artifact_scan_reports" does not exist`。
   修复：安装脚本改为逐平面迁移（本次 platform skipped=103，
   market / artifact 各 applied=103）。market 平面确认 128 张表、
   `artifact_scan_reports` 存在。迁移后需重启既有 Pod 释放陈旧连接。

   注意：这是**安装编排**的缺口——`pkg/db` 已支持三平面，缺的是默认安装流程
   是否遍历全部平面。

## 3. 稳态资源占用与功能验证

首轮（20Gi/10Gi 卷）：节点 166m CPU (4%) / 1352Mi 内存 / 磁盘 18G/118G。
全新安装（2Gi/1Gi 卷）：节点 133m CPU (3%) / 1386Mi 内存。

单容器最高：postgresql-0 8m CPU / 63Mi，其余多在 0-1m / 5-7Mi。

TopoLVM 组件另计：controller 6m/45Mi，node 2m/22Mi。

### 功能验证（经 portal 真实调用）

| 检查 | 结果 |
|---|---|
| 登录 `/api/v1/auth/login` | 200，签发 access token |
| `/api/v1/workspaces` | 200 |
| `/api/v1/tenants` | 200 |
| `/api/v1/products` | 403（权限模型生效） |
| `/api/v1/auth/me` | 403（权限模型生效） |
| `/api/v1/agents` | 200，agent `status: connected`，心跳持续更新 |
| apiserver / app-market health | 200 |
| platform-api health | 401（需鉴权，符合预期） |

agent-onboarding 端点为 cluster `00000000-0000-4000-8000-0000000000cb`
签发 24h TTL 的 `hnb.agent-tunnel/v1` 令牌（绑定 tenant=default + cluster），
cluster-agent 用该令牌成功连接 `ws://portal/tunnel`。

## 4. 首轮部署修复的真实缺陷

首次部署 19 个工作负载中有 3 个无法启动，均为**配置/接线缺陷，与资源无关**：

1. **rbac-syncer 忽略 DB_DSN**（`cmd/rbac-syncer/main.go`、`internal/config/config.go`）。
   Chart 已注入 `DB_DSN`，但二进制回落到 `localhost:5432`，因加密 DSN 无法解析而拒启。
   修复：优先使用 `DB_DSN`，用 `pq.ParseURL` 校验（非法 URL 直接失败，不回落、不回显凭据），
   连接池收敛到 5/2 并设置生命周期。已补 `main_test.go`、`config_test.go`。
2. **rbac-syncer 无 ServiceAccount**（`charts/rbac-syncer/templates/rbac.yaml` 新增）。
   未声明身份时落到 `default` SA，无权 list/watch 集群级 RoleBinding，reflector 持续报错
   而进程仍 Running——属于静默失败。新增 ServiceAccount + ClusterRole（只读 roles/rolebindings）
   + ClusterRoleBinding，并在 Deployment 指定 `serviceAccountName`。
3. **cluster-agent 无法启动**（`charts/cluster-agent/`）。二进制要求 `AGENT_TOKEN_FILE`、
   `TENANT_ID`、`CLUSTER_ID`，但 Chart 模板只注入 `NATS_URL`/`DB_DSN`，Minimal 下必然 CrashLoop。
   修复：新增 `onboarding.*` values（enabled/tenantId/clusterId/tunnelUrl/existingTokenSecret），
   由 initContainer 以 `0400` 落盘令牌——Secret 挂载在 fsGroup 下会变成 0440，
   `iam.FileTokenSource` 明确拒绝 group/other 可读文件，与 operation-worker 的既有做法一致。

### Minimal 资源预算（新安装默认值）

`overlays/minimal.yaml` 现将新装 PVC 设为 **PostgreSQL 2Gi / JetStream 1Gi**，
并下调部分单副本 requests，同时保留全部子 Chart 突发 limits：

| 维度 | 调整前 | 调整后 |
|---|---|---|
| CPU requests | 1725m | **1250m** |
| 内存 requests | 2624Mi | **2032Mi** |
| 新装 PVC 合计 | 30Gi | **3Gi** |

2C/3913Mi/15Gi 节点、startup scope、外部 cache/backup 下 required 为
**1875m / 3260Mi / 14Gi**，容量模型通过。本次实测占用（166m/1352Mi）远低于声明值，
但 requests 是调度预留而非实测峰值，仍需登录、迁移、Operation 与恢复场景的峰值验证。

**PVC 不支持缩容已在真机确认**：把现有 20Gi/10Gi 的 claim 改成 2Gi/1Gi 被 API 拒绝
（`field can not be less than status.capacity`）。2Gi/1Gi 只对**新建**卷生效。

## 5. TopoLVM 存储验证

版本：Helm chart 17.3.0 / app 0.42.0（上游最新稳定版），镜像按 digest 钉住。
配置见 `deploy/storage/topolvm/values-minimal.yaml`（单节点、Retain、允许扩容、
关闭 scheduler 与 snapshot 以压缩开销）。

**测试后端是系统盘上的临时 loop 镜像文件 + 独立 VG（hnb-eval-vg，8G），
不是生产数据盘，也不计入 15G 总盘验收。**

| 步骤 | 结果 |
|---|---|
| 供给 | PVC 1Gi → Bound，TopoLVM 创建 LVM LV `eaa94c7d-…` 1.00g |
| 写入 | marker 写入成功 |
| 在线扩容 | patch 至 4Gi → PVC 4Gi、LV 4.00g；kubelet 事件 `FileSystemResizeSuccessful`，**未重建 Pod** |
| 数据保留 | 删除并重建 Pod 后 marker 仍为 `hnb-eval-20261015` |

StorageClass `hnb-topolvm`：`allowVolumeExpansion=true`、
`volumeBindingMode=WaitForFirstConsumer`、`reclaimPolicy=Retain`。

这解决了 local-path（`allowVolumeExpansion: false`，不支持在线扩容）的限制。
`overlays/storage-topolvm.yaml` 用于在新安装时把 PG/NATS 卷指向该 StorageClass；
**既有 PVC 的 storageClassName 不可变更**，切换需重建卷。

## 6. 未完成与限制

- Ubuntu 26.04 未在认证 OS 列表内，本次不构成 OS 认证。
- loop 设备仅验证 LVM/PVC 机制；生产需独立数据盘，且 TopoLVM 无自身高可用。
- requests 已按声明下调，但峰值（迁移、登录、Operation、恢复）尚未实测。
- 2Gi/1Gi 是**新装起点**，未验证长期数据增长（WAL、审计、JetStream 留存、
  日志）下的表现；`MaxAge` 提供了时间上界，但数据增长率未测。
- 三平面迁移的修复落在本次评估脚本；**正式安装流程是否遍历全部平面仍需确认**。
- 端口开放性与公网入口未验证。
- 本报告为单次部署与功能验证，**不等于 Minimal 档位的发布认证**；
  `CurrentEvidence` 中 Minimal 仍为 blocked。

## 7. 复核方式

```bash
# Chart 渲染与容量模型（需 Helm、PyYAML、Go）
cd deploy/charts/hnb && python3 -m unittest test_profiles test_candidate_components

# 本轮改动的三个 Go 模块
cd cmd/rbac-syncer && go test ./...
cd cmd/operation-worker && go test ./...
cd cmd/gslb-controller && go test ./...
```
