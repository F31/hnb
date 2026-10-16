# HNB Minimal 资源基线与 2C4G 压缩清单

审计日期：2026-10-15。来源为当前工作树，包含尚未提交的 Chart 改动。

## 1. 结论与口径修正

- 完整 Minimal：**19 个工作负载，1725m CPU / 2624Mi 内存 requests，30Gi PVC**。
- 已有 Startup：**10 个工作负载，1075m / 1792Mi，15Gi PVC**，包含 Portal、BFF、
  platform-api、市场及本地 PostgreSQL/NATS。它不是“只有 Kubernetes runtime”。
- Startup + runtime 能力：12 个工作负载，1275m / 2048Mi，15Gi PVC。
- 已有 small-disk overlay 可将全新小数据测试的 PostgreSQL/NATS PVC 改为 2Gi/1Gi。
  Startup 或 Startup + runtime 叠加该 overlay 后，在 2C/3913Mi/19Gi 声明节点上，
  使用 startup scope、外部缓存/备份和默认系统预留，容量模型均通过。
- 完整 Minimal 的 CPU 超出 2C4G 预算；默认 Startup 的主要障碍是磁盘。
  **包含平台与控制台的轻量初始组合已有可用配置，不能据完整 Minimal 的预算推断
  所有包含控制台的组合均无法容纳于 2C4G。**

前序讨论引用的 `1600m / 2240Mi / 30Gi` 来自模拟测试，不是当前完整 Chart
渲染结果。本清单以真实 Helm 渲染值纠正该口径。容量模型通过不等于发布认证或
整平台真机验收；下面的 requests、limits 和 PVC 也不是实测 RSS、峰值或已用磁盘。

## 2. 提取方法与来源

1. 读取 25 个 `deploy/charts/hnb/charts/*/values.yaml` 的资源、存储和副本默认值。
2. 叠加父 [`values.yaml`](../../deploy/charts/hnb/values.yaml)，依赖启用条件取自
   [`Chart.yaml`](../../deploy/charts/hnb/Chart.yaml)。
3. 分别叠加 [`minimal.yaml`](../../deploy/charts/hnb/overlays/minimal.yaml)、
   [`startup.yaml`](../../deploy/charts/hnb/overlays/startup.yaml) 及能力/小盘 overlay。
4. 用既有 `test_profiles.ProfileTests.render` 与 `WholeChartDigestTests.render_values`
   提供合成 digest、Secret 引用及必要字段，执行实际 `helm template`，逐工作负载统计，
   再把结果交给实际 `hnbctl capacity`。合成绑定不是可安装 Release，也未访问云主机。

本次父 Chart 与子 Chart 的 CPU/内存 resources 相同；副本数和 enabled 有差异。
常规合并顺序是子 Chart默认 → 父 Chart → `-f` 文件（后者优先）→ `--set`。
`RenderLockedChart` 的实际顺序是选定 overlay 后再合并 bindings，因此操作者绑定
仍可能覆盖这里的基线；最终以输出 manifest 为准。

PG/NATS StatefulSet 模板固定 `replicas: 1`，未提供 `replicaCount` values。
其 PVC 模板在设置 `storage.existingClaim` 时不新建 PVC；复用已有 PVC 不是消除存储成本。

## 3. 全量逐组件基线

requests/limits 为**单个主容器**；`—` 表示该组合不启用。副本列依次为
“子 Chart 默认 / 父 Chart 默认启用副本 / Minimal / Startup”。PG/NATS 的 `固定1`
来自模板。所有 CPU 单位为 m，内存为 Mi；PVC 列为子 Chart 默认。

| 组件（链接为子 Chart values） | 子默认 | 父默认 | Minimal | Startup | requests CPU / 内存 | limits CPU / 内存 | PVC Gi |
|---|---:|---:|---:|---:|---:|---:|---:|
| [alert-manager](../../deploy/charts/hnb/charts/alert-manager/values.yaml) | 1 | — | 1 | — | 50 / 64 | 100 / 128 | — |
| [apiserver](../../deploy/charts/hnb/charts/apiserver/values.yaml) | 1 | 2 | 1 | 1 | 125 / 256 | 400 / 512 | — |
| [app-market](../../deploy/charts/hnb/charts/app-market/values.yaml) | 1 | 1 | 1 | 1 | 100 / 128 | 250 / 256 | — |
| [calico-provider](../../deploy/charts/hnb/charts/calico-provider/values.yaml) | 1 | — | — | — | 200 / 256 | 500 / 512 | — |
| [cilium-provider](../../deploy/charts/hnb/charts/cilium-provider/values.yaml) | 1 | — | 1 | — | 50 / 64 | 100 / 128 | — |
| [cluster-agent](../../deploy/charts/hnb/charts/cluster-agent/values.yaml) | 1 | — | 1 | — | 50 / 64 | 100 / 128 | — |
| [cluster-router](../../deploy/charts/hnb/charts/cluster-router/values.yaml) | 2 | 2 | 1 | 1 | 100 / 128 | 250 / 256 | — |
| [edge-provider](../../deploy/charts/hnb/charts/edge-provider/values.yaml) | 1 | — | — | — | 200 / 256 | 500 / 512 | — |
| [extension-controller](../../deploy/charts/hnb/charts/extension-controller/values.yaml) | 1 | 1 | 1 | 1 | 100 / 128 | 250 / 256 | — |
| [gateway-provider](../../deploy/charts/hnb/charts/gateway-provider/values.yaml) | 1 | 1 | 1 | — | 100 / 128 | 250 / 256 | — |
| [gslb-controller](../../deploy/charts/hnb/charts/gslb-controller/values.yaml) | 1 | — | — | — | 200 / 256 | 500 / 512 | — |
| [karmada-provider](../../deploy/charts/hnb/charts/karmada-provider/values.yaml) | 1 | — | — | — | 200 / 256 | 500 / 512 | — |
| [kube-ovn-provider](../../deploy/charts/hnb/charts/kube-ovn-provider/values.yaml) | 1 | — | — | — | 200 / 256 | 500 / 512 | — |
| [kubernetes-provider](../../deploy/charts/hnb/charts/kubernetes-provider/values.yaml) | 1 | 1 | 1 | — | 100 / 128 | 250 / 256 | — |
| [nats](../../deploy/charts/hnb/charts/nats/values.yaml) | 固定1 | 1 | 1 | 1 | 50 / 128 | 250 / 256 | 10 |
| [network-provider](../../deploy/charts/hnb/charts/network-provider/values.yaml) | 1 | — | — | — | 200 / 256 | 500 / 512 | — |
| [network-registry](../../deploy/charts/hnb/charts/network-registry/values.yaml) | 1 | — | 1 | — | 50 / 64 | 100 / 128 | — |
| [operation-worker](../../deploy/charts/hnb/charts/operation-worker/values.yaml) | 1 | 2 | 1 | 1 | 125 / 256 | 400 / 512 | — |
| [platform-api](../../deploy/charts/hnb/charts/platform-api/values.yaml) | 1 | 2 | 1 | 1 | 125 / 256 | 400 / 512 | — |
| [plugin-worker](../../deploy/charts/hnb/charts/plugin-worker/values.yaml) | 1 | — | 1 | — | 100 / 128 | 250 / 256 | — |
| [portal](../../deploy/charts/hnb/charts/portal/values.yaml) | 2 | — | 1 | 1 | 100 / 128 | 250 / 256 | — |
| [postgresql](../../deploy/charts/hnb/charts/postgresql/values.yaml) | 固定1 | 1 | 1 | 1 | 150 / 256 | 1000 / 1024 | 20 |
| [rbac-syncer](../../deploy/charts/hnb/charts/rbac-syncer/values.yaml) | 1 | 1 | 1 | — | 50 / 64 | 100 / 128 | — |
| [runtime-target-lifecycle-provider](../../deploy/charts/hnb/charts/runtime-target-lifecycle-provider/values.yaml) | 1 | — | 1 | — | 100 / 128 | 250 / 256 | — |
| [tunnel-server](../../deploy/charts/hnb/charts/tunnel-server/values.yaml) | 1 | 2 | 1 | 1 | 100 / 128 | 250 / 256 | — |

完整 Minimal 和 Startup 中各已启用工作负载均为单副本，因此不能再以“降为单副本”
作为这两个组合的新增收益。父默认不是 Minimal，且 Portal 默认未启用。

### 初始化容器与临时空间

- apiserver 的 `stage-signing-key` 和 operation-worker 的 `stage-provider-tokens`
  各请求 5m / 8Mi，limits 50m / 32Mi。
- 它们是普通串行 init container；Pod 调度资源取主容器合计与 init 最大值中的较大者，
  不是两者相加。本次两者均小于主容器请求，不改变上述组合的调度总量。
- Portal 模板另有 `emptyDir.sizeLimit: 64Mi`；凭据暂存卷也有小型 emptyDir。
  这些不是 PVC，不能把 PVC 总和当作所有本地占用。镜像、日志、可写层、临时数据
  需要系统预留与现场磁盘实测兜住；本次未计量镜像尺寸或真实磁盘用量。

## 4. 组合汇总与 2C4G 模型结果

节点统一输入：2000m CPU / 3913Mi 内存 / 19Gi 磁盘。这里使用前序讨论的 19Gi
规划口径；既有另一个主机报告有 18Gi 向下取整输入，不能混用为本机实测空闲量。
恢复主机后必须重新读取实际容量、分区可用空间和已用空间。

下表统一使用 `--scope startup --cache-source external --backup-source external`，
默认系统预留 **500m / 1024Mi / 10Gi**，10% 余量，每维向上取整。
“external”只改变缓存/备份的预算位置，不会外置 PG/NATS 或搬迁运行时镜像。

| 渲染组合 | 工作负载 | CPU requests | 内存 requests | PVC | required CPU / 内存 / 磁盘 | 2C/3913Mi/19Gi |
|---|---:|---:|---:|---:|---|---|
| minimal | 19 | 1725m | 2624Mi | 30Gi | 2398m / 3911Mi / 43Gi | 不通过 |
| startup | 10 | 1075m | 1792Mi | 15Gi | 1683m / 2996Mi / 27Gi | 磁盘不足 |
| startup + startup-small-disk | 10 | 1075m | 1792Mi | 3Gi | 1683m / 2996Mi / 14Gi | 模型通过 |
| startup + capability-runtime | 12 | 1275m | 2048Mi | 15Gi | 1903m / 3277Mi / 27Gi | 磁盘不足 |
| startup + capability-runtime + startup-small-disk | 12 | 1275m | 2048Mi | 3Gi | 1903m / 3277Mi / 14Gi | 模型通过，CPU 余量较小 |

small-disk 为**全新小数据测试**预算；不能缩小现有 PVC，也不代表长期数据留存容量。
本次保持 10Gi 系统盘预留，未通过降低系统预留制造通过结果。全部报告未提供认证
授权输入，`installAuthorized=false`。

主容器 limits 总和：Minimal **5200m / 5760Mi**，Startup **3700m / 4096Mi**，
Startup + runtime **4200m / 4608Mi**。这些总和超过节点容量；limits 不是保留量，
却表明并发高峰仍可能造成 CPU 争抢、内存压力。模型通过还需负载与恢复验证。

## 5. 压缩清单：按收益与实施成本排序

### P0：复用已有组合，先确定功能边界

| 项目 | 相对完整 Minimal 的可量化变化 | 当前状态与约束 |
|---|---|---|
| Startup 按需能力组合 | 少 9 个工作负载，节省 **650m / 832Mi**；PVC 30→15Gi | 已有 overlay；保留控制台和平台核心，缺少未选 Provider 的执行能力；不能标记为完整 Minimal |
| Startup 加 runtime pack | 相对完整 Minimal 节省 **450m / 576Mi**，少 7 个工作负载 | 已有 overlay；启用运行目标/Kubernetes Provider，实际 Operation 还取决于计划及凭据/目标绑定 |
| small-disk 测试预算 | Startup PVC **15→3Gi**，省 12Gi；外部缓存/备份下 required **27→14Gi** | 已有 overlay；只适用于新安装、小数据；需测 PG WAL、审计增长、JetStream 留存及磁盘恢复 |

Startup 减少的 9 个组件可以分组核算，收益不可重复相加：

| 按需组 | 组件 | 启用新增 requests |
|---|---|---:|
| runtime | runtime-target-lifecycle-provider、kubernetes-provider | 200m / 256Mi |
| network | network-registry、cilium-provider、gateway-provider | 200m / 256Mi |
| agent | cluster-agent、rbac-syncer | 100m / 128Mi |
| alerts | alert-manager | 50m / 64Mi |
| plugins | plugin-worker | 100m / 128Mi |
| 合计 | 9 个组件 | **650m / 832Mi** |

按需组不等于业务链已经闭合；部署前还要按具体操作核对依赖。`cilium-provider`
是 HNB 能力服务，与 Kubernetes 底层 Cilium CNI 不是同一个工作负载。

### P1：保留业务语义的资源调优候选

以下目标是**待测候选**，不是已实现收益，不据此直接修改默认值。

| 优先对象 | 当前单副本 requests | 可试验目标 | 声明量潜在节省 | 验证要点 |
|---|---|---|---|---|
| Portal | 100m / 128Mi | 25–50m / 32–64Mi | 50–75m / 64–96Mi | 静态服务、BFF 代理并发、登录及页面加载；确认进程真实峰值 |
| apiserver、platform-api、operation-worker | 各 125m / 256Mi | 各 100m / 192Mi | 合计 75m / 192Mi | 身份校验、API 并发、计划执行、重试/恢复和 GC；降低 request 不等于减小 RSS |
| 常驻 controller/provider | 多数 50–100m / 64–128Mi | 逐个按 idle 与任务峰值校准 | 本次不预估统一比例 | 限制 worker 并发、批量大小、轮询频率、watch 缓存；保留超时与重放语义 |
| PostgreSQL | 150m / 256Mi，limit 1Gi | 先调连接数、应用连接池、查询及 WAL 留存 | 待实测，不先降内存预算 | 多服务连接汇聚、迁移峰值、事务延迟、审计增长、备份恢复 |
| NATS JetStream | 50m / 128Mi，PVC 10Gi | 限制流保留时间/字节数，控制消费者积压 | 主要是数据增长收益，未测量 | 事件可靠性、持久化、重放、断电恢复；不能用丢事件换取小占用 |

若要求**完整 19 工作负载 Minimal 功能不减**，startup scope 下 2C 节点容许
CPU requests 最高为 `floor((2000-500)/1.1) = 1363m`，需从 1725m 至少下降
**362m（约21%）**。上表 Portal 与三个核心服务的 CPU 优化合计仅 **125–150m**，
不足以独立达标；还需其它组件实测调优或引入新的部署形态。

3913Mi 节点容许内存 requests 最高约 2626Mi，完整 Minimal 的 2624Mi 已贴近边界，
required 3911Mi 只比声明少 2Mi，不能把它作为稳定运行裕量。对 19Gi 本地盘和
10Gi 默认系统预留，模型容许 PVC 合计最大 **8Gi**；完整 Minimal 的 30Gi 需另行
降低新安装数据预算、外置实际数据或扩盘。现有 small-disk 明确定义在 Startup 上叠加，
本次未把它推广为完整 Minimal 的长期存储配置。

### P2：外置依赖与架构优化

- 外置 PostgreSQL：减少本节点 **150m / 256Mi** requests，并转移相应 PVC；
  外置 NATS：再减少 **50m / 128Mi**。两者合计 200m / 384Mi。
  必须真正禁用 bundled 工作负载、配置连接和各平面 Secret/TLS；
  `--cache-source external` 不执行这些动作。外部服务的成本、容量和可用性仍需承担。
  即使完整 Minimal 同时外置两者，CPU requests 仍有 1525m，单靠外置仍不足以达标。
- 进程合并、SQLite 替代 PG、嵌入式队列属于高成本架构变更；仅凭 Chart 不能推导
  可替换性。本轮不列确定节省值。优先保留独立 Provider、平面数据隔离、身份及
  `RuntimeIntent → ExecutionPlan → Operation` 写入口，避免以合并服务破坏契约。
- 镜像共享层、精简镜像、运行时 GC、日志轮转和拉取/升级错峰主要减少磁盘或峰值，
  当前 values 无法量化；需镜像清单与节点实测。按需 pack 已关闭的六个 200m/256Mi
  可选服务，对当前 Minimal 没有新增可裁收益。
- 当前两个组合已经全部单副本；再减副本只能停止服务。swap/zram 不改变 requests
  预算，也不能视为已补足物理内存或替代峰值验证。

## 6. 推荐推进顺序

1. 以 **Startup + small-disk** 作为含控制台的轻量验证基线，功能需要时叠加 runtime；
   保留 PostgreSQL、JetStream 持久化以及完整身份/平面边界。
2. 优先测 Portal、三个核心服务与共享 PG 连接池；记录 idle/负载 RSS、CPU、
   启动/迁移峰值、登录/API 时延和完整 Operation，而非直接按百分比下调 requests。
3. 主机恢复后测镜像拉取、WAL/消息/日志增长、重启恢复、磁盘告警与备份还原。
4. 如目标仍是完整 Minimal 功能不减，再逐项弥补至少 362m 的 CPU 声明缺口，
   重新生成容量报告，并单独完成发布与整链路认证。

## 7. 本地复核

已有集成用例核对真实 Chart 渲染、资源覆盖、精确组件集、完整 Minimal 总量、
Startup 2C4G 模型及 small-disk 预算：

```bash
# 工作目录：deploy/charts/hnb；需要 Helm、PyYAML、Go
python3 -m unittest test_profiles
```

本审计另行渲染第 4 节五种组合，将 manifest 逐一输入实际 `hnbctl capacity`，节点为
2000m/3913Mi/19Gi，得到表列结果。原始配置与能力边界亦见
[`lightweight-startup.md`](lightweight-startup.md)。
