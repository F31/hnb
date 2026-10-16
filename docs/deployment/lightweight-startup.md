# 轻量初始部署与按需扩展

逐组件 requests/limits/PVC、当前渲染总量及 2C4G 压缩优先级见
[`Minimal 资源基线与压缩清单`](minimal-footprint-audit.md)。

## 架构与适用范围

`deploy/charts/hnb/overlays/startup.yaml` 提供显式选择的初始组件组合，复用现有
微内核、独立 Provider 和共享 Chart，不创建另一套实现。不把具体 Provider
编译进 Core，不合并跨平面数据库，不改变身份、租户、审计、Read Model 和
`RuntimeIntent -> ExecutionPlan -> Operation` 唯一写入口。

Startup 先部署 8 个应用组件：apiserver、platform-api、operation-worker、
extension-controller、app-market、cluster-router、tunnel-server、portal；再部署
持久化 PostgreSQL 和 NATS，共 10 个工作负载。市场与门户保留，运行目标和具体
能力实现按需要加入。没有 Provider 时，其执行能力不可用，不能把请求当成已经成功。

这是初始组合，**不是完整 Minimal T0/T1 交付，也不是 HA 档位**。
`global.deploymentStage=startup` 必须配合空 `deploymentProfile`，Chart 会拒绝
把 Startup 标成 Minimal。完整 `minimal.yaml` 保留原有 19 工作负载及必需组件门禁。
当前 `hnbctl` 安装流程仍按既有完整档位验收；本组合提供 Chart 渲染与容量规划入口，
尚无 Startup 整平台安装、登录或真实 Operation 验收证据。

## 按需选择能力

| overlay | 新增独立组件 | 启用时机 |
|---|---|---|
| `capability-runtime.yaml` | runtime-target-lifecycle-provider、kubernetes-provider | 需要创建/管理运行目标和执行 Kubernetes Operation |
| `capability-network.yaml` | network-registry、cilium-provider、gateway-provider | 需要网络与 Gateway 配置 |
| `capability-agent.yaml` | cluster-agent、rbac-syncer | 需要目标接入与 Kubernetes RBAC 同步 |
| `capability-alerts.yaml` | alert-manager | 需要告警评估与通知 |
| `capability-plugins.yaml` | plugin-worker | 需要插件执行 |

这些 overlay 是 Helm 的部署时按需选择，不是自动安装器或自动缩容到零。
各组件仍独立进程/容器，可独立启停、升级与替换；启用不需要重编译内核。
网络 Provider 启用时必须同时启用 network-registry，缺失依赖会拒绝渲染。
Agent/RBAC 必须使用实际目标绑定，不应把所有目标都放进中心控制面。
具体操作需要哪些能力，应按已锁定的 ExecutionPlan/Provider 契约确定；
运行目标、网络、插件能力分别启用并不代表整条业务链已就绪。

Chart 为 apiserver、platform-api 与 app-market 提供内部 ClusterIP Service；
apiserver 显式绑定 `PLATFORM_API_URL`、`APP_MARKET_URL`，并设置
`REQUIRE_PLATFORM_API=true`。Portal 经 apiserver BFF 访问领域 API，初始组合
仍保持跨平面服务边界。外部服务地址可通过 apiserver 的对应 values 覆盖。

## 使用方式

先准备既有制品 digest、身份、KMS、每平面数据库/NATS、TLS、存储和 SecretReference
bindings。这里只列叠加顺序，不提供示例密码或伪造 release digest：

```bash
helm template hnb deploy/charts/hnb \
  -f bindings.yaml \
  -f deploy/charts/hnb/overlays/startup.yaml > rendered.yaml

# 按需要加载运行目标与网络能力；保留相同 release/身份绑定。
helm template hnb deploy/charts/hnb \
  -f bindings.yaml \
  -f deploy/charts/hnb/overlays/startup.yaml \
  -f deploy/charts/hnb/overlays/capability-runtime.yaml \
  -f deploy/charts/hnb/overlays/capability-network.yaml > rendered.yaml
```

最终资源来自 Helm 合并后的 values：subchart 默认值、parent 值、bindings/overlay
和命令行覆盖共同决定，后面的值优先。不要把资源预算写进凭据 bindings 后误以为
默认值仍生效。可用 `--set apiserver.resources.requests.cpu=175m` 明确覆盖；
应始终按实际 rendered.yaml 计算容量。

单节点声明示例 `nodes.yaml`：

```yaml
- name: cp-1
  cpuMilli: 2000
  memoryMi: 4096
  diskGi: 40
```

```bash
# 在 cmd/hnbctl 目录运行
go run . capacity --manifest ../../rendered.yaml --nodes ../../nodes.yaml \
  --scope startup --cache-source external --backup-source external
```

上述外部参数只改变规划中的缓存/备份位置，不会自动配置外部存储。
必须另行提供可用的外部路径、容量与备份流程。

## 当前资源结果（2026-10-13）

| 组合 | 工作负载 | CPU requests | 内存 requests | PVC |
|---|---:|---:|---:|---:|
| 完整 Minimal | 19 | 1725m | 2624Mi | 30Gi |
| 初始 Startup | 10 | 1075m | 1792Mi | 15Gi |

Startup 默认仅针对**全新安装**使用 PostgreSQL 10Gi、JetStream 5Gi。
已有 PVC 不能缩容；迁移现有安装时保留原 PVC 大小，重新计算容量。
少量初始容量需要按业务数据与事件保留量扩容，不能当作长期数据预算。

Startup scope 使用 10% 余量及每节点 500m CPU / 1024Mi / 10Gi 系统预留。
外部缓存与备份时，上述初始组合 required 为 **1683m / 2996Mi / 27Gi**，
2C4G/40G 的声明输入通过；本地缓存与完整本地备份则需要 **65Gi** 磁盘，
同一 40Gi 节点被拒绝。完整 Minimal 在同一 startup scope 下需要
**2398m / 3911Mi / 43Gi**，仍超过 2C4G/40G，CPU 差 398m、磁盘差 3Gi。

以上是调度 requests 和磁盘预算的规划，不是实际空闲 RSS、峰值内存或吞吐实测。
requests 已减小但保留现有 limits 作为突发空间；多个服务同时达到 limit 仍可能
超过节点容量。启用新能力后重新渲染、规划，并完成所需真实 Operation、Agent
观测、数据恢复和负载验收。容量工具始终输出 `installAuthorized=false`。

## 验证

### 10Gi 小数据测试预算

全新小数据测试可在 `startup.yaml` **之后**叠加
`deploy/charts/hnb/overlays/startup-small-disk.yaml`，将 PostgreSQL PVC 设为 2Gi、
JetStream PVC 设为 1Gi。组件数、CPU/内存 requests、持久化和各平面身份边界不变。
默认 Startup 仍为 10Gi + 5Gi，不把测试数据预算当作默认长期容量。

需要达到 10Gi **规划需求**时，在节点输入中显式设置 `reserveDiskGi: 6`：

```yaml
- name: cp-1
  cpuMilli: 2000
  memoryMi: 3913
  diskGi: 18       # 当前云主机根盘实测向下取整；不要改写成假设容量
  reserveDiskGi: 6
```

使用 `--scope startup --cache-source external --backup-source external` 后，
required disk = `ceil((2 + 1) × 1.1) + 6 = 10Gi`。
保留默认 10Gi 系统盘预留时需 14Gi；本地缓存 20Gi + 本地完整备份 3Gi、
系统预留 6Gi 时需 35Gi。减少 PVC 并不会清除缓存和备份的真实占用。

这里的 **6Gi 是操作者声明的预算，不是系统实测值**。需要容纳 OS、Kubernetes/
运行时、必须留在本机的镜像与解压层、可写层、日志和临时数据。外部缓存不等于
容器运行时镜像自动移到外部：必须另行配置并核验路径，或将本地必需镜像计入
系统预算。只有实测这些实际占用能落在 6Gi 内，才能把模型的 10Gi 结果提升为
真实小盘部署结论。扩展能力、保留消息、审计数据、WAL 与负载增长均须重算。
当前尚无真实 10Gi 根盘上的完整平台运行证据。

小盘配置不能缩小现有 PVC，也不降低持久化、TLS、数据隔离或备份要求。
此处 Gi 按 1024³ bytes 计算；10Gi 约 10.74GB，云盘标称 10GB 与根文件系统
实际可用 10Gi 不等价。

2026-10-13 已在 `106.75.34.133` 上传新 CLI，使用实际 2C/3913Mi/18Gi
节点输入及合成渲染 fixture 验证 10Gi 预算通过、本地 35Gi 预算拒绝。
见 [`小磁盘规划报告`](../acceptance/reports/20261013-ubuntu-small-disk-planning.md)。

```bash
# 在 deploy/charts/hnb 目录运行
python3 -m unittest test_profiles test_candidate_components
```

覆盖 Startup 精确组件集合、每组按需加载、核心/网络缺失依赖拒绝、
恢复完整 Minimal、BFF 内部 Service 与地址绑定、资源参数覆盖、真实容量 CLI 的 2C4G 规划通过，以及本地
缓存/备份超盘拒绝。所有制品和 Secret 测试绑定是合成夹具，不是发布认证。
