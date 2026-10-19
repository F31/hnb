# 功能成熟度与验收证据

`registry.yaml` 是验收台账；源码存在、测试通过、容器启动、真实业务验收分别记录。
历史会话结论不作为发布资格。Blocked 表示目标尚未达成，不得改成 cancelled 隐藏缺环。

产品闭环建设入口：[`S0 复用/缺口基线`](product-loop-baseline.md)、
[`统一底座与四档组装契约`](../deployment/profile-composition-contract.md)、
[`阶段计划/预算/证据校验`](product-readiness-manifests.md)。这些建设记录不提升既有认证。

2026-10-18 新增：[`S3 4.4 PVC 与 Secret 挂载（实现完成，真实环境阻塞）`](reports/20261018-s3-4.4-pvc-secret-mounts.md)
（`pvc.deploy`、真实 TopoLVM Bound、Redis PVC/Secret 挂载、marker 读写、
Secret 不回显、删除工作负载后 PVC 保留；最小 RBAC 已验证）。

2026-10-18 新增：[`S3 4.3 类型化 Service/工作负载动作`](reports/20261018-s3-4.3-service-workload-actions.md)
（`service.deploy`、工作负载归属、幂等/CAS/fencing、无部分创建；最小目标 RBAC
allow/deny 已验证；真实 Provider HTTP 执行留 4.12）。

2026-10-18 新增：[`S3 4.2 应用组合规划与能力预检`](reports/20261018-s3-4.2-composition-planning.md)
（严格 Schema、依赖排序、Namespace/SecretReference/配额/固定快照预检；PVC/Ingress
能力缺失和未知 Manifest 均正确拒绝；仅计划不执行）。

2026-10-18 新增：[`S3 4.1 应用交付绑定迁移与查询`](reports/20261018-s3-4.1-application-delivery-bindings.md)
（106 增量迁移、tenant 过滤、最新 revision、UID/PVC/入口/Operation/观测水位；
临时测试行已清理；应用写入仍属 4.3–4.5）。

2026-10-18 新增：[`S2 断连恢复、source-reset 与 Portal/CLI`](reports/20261018-s2-reconnect-portal-cli.md)
（Agent 停机 373s 后 STALE 保留旧状态；新 enrollment generation 5、PVC state 触发
source-reset、恢复 FRESH 且无重复目标；Portal/CLI 同一目标列表一致；3.5 仍 partial）。

2026-10-18 新增：[`S2 独立目标 import 与 Agent 观测`](reports/20261018-s2-import-agent-observation.md)
（正式 API import、幂等重放、enrollment/session/tunnel、2 节点与 FRESH 观测；修复
tunnel session 校验、tenant bypass、freshness/capability 投影；因 source-reset/序列
持久化缺口保留 partial，3.4/3.5 未完成）。

2026-10-18 新增：[`Observer 会话绑定与 Provider 接线`](reports/20261018-observer-session-binding.md)
（observer token 绑定 session tenant/target/generation、exchange/renew 轮换、
Agent 私有文件写入；Provider/worker 已接线；conformance 证据缺失使 import 保持 blocked）。

2026-10-18 新增：[`管理面当前源码部署与真实登录`](reports/20261018-management-deployment.md)
（13 个 digest 固定镜像、三平面迁移 104、10 工作负载 Ready、真实 admin 登录及鉴权读取；
修复 Go 1.26 服务身份撤销路由注册 panic；不含独立目标 import、Agent、S3 或 HA）。

2026-10-18 新增：[`S0 1.3 三主机独立集群 / CSI / Gateway 基线`](reports/20261018-s2-s3-lab-infrastructure.md)
（两个独立 K3s 集群、TopolVM、Gateway/TLS、真实卷读写/Pod 重建/worker 重启；
另修复续期客户端真实响应兼容性并补 worker Chart 接线；尚未部署新 HNB，不含 S2/S3 产品验收）。

2026-10-17 新增：[`hnbctl 零配置卸载与 -all/-data 全清`](reports/20261017-hnbctl-auto-uninstall.md)
（auto-detect、`-all` 删 TopoLVM/SC/CSIDriver/CRD、`-data` 删 PVC/PV/LV/Secret/namespace；
云主机全清后仅保留 K3s 系统组件）。

2026-10-17 新增：[`hnbctl 实际卸载执行器`](reports/20261017-hnbctl-uninstall-execute.md)
（`uninstall --execute --yes` 真删 HNB 工作负载/RBAC；PVC/Secret/Namespace/
StorageClass/TopoLVM 保留；不含数据删除、TopoLVM 删除或重装）。

2026-10-17 新增：[`S2 3.2 观测 watermark/乱序隔离`](reports/20261017-s2-observation-watermark.md)
（observationGeneration/Revision/Source、重复/缺口/旧 generation/跨租户/跨目标；
真实 PostgreSQL projector 测试；不含独立集群 Agent E2E）。

2026-10-17 新增：[`S2 3.1 import 受理/可用分离`](reports/20261017-s2-import-acceptance.md)
（ClusterAcceptance、REGISTERING+pending 不可用、观测后 ACTIVE+FRESH 才可用；
真实 PostgreSQL 查询测试；不含独立集群 import/真实 Agent handoff/3.2 乱序）。

2026-10-17 新增：[`S0 1.2 Portal/API/CLI 边界`](reports/20261017-s0-portal-api-boundary.md)
（同仓 Portal、维护者/版本/构建/测试入口、`@hnb/api-client` 边界与静态守卫；
不含 Playwright E2E、Portal 镜像部署或 S2/S3 UI 验收）。

2026-10-17 新增：[`S1 服务身份生命周期`](reports/20261017-service-identity-lifecycle.md)
（受限 issue/renew/revoke、原子消费/撤销、actor/service subject 审计；本地 unit/竞态 +
独立 PostgreSQL 16.15 集成；不含真实部署、HA 或 5s/60s 撤销时延认证）。
同日新增：[`2.4 worker 按租户续期 + 2.5 Agent 单次入网`](reports/20261017-agent-enrollment-and-worker-identity.md)
（per-tenant binding、无 tokenFile 回退、create/exchange/replay/撤销/跨租户/跨目标；
不含 Agent 二进制接线、生产 rollout、HA 或真实跨 TTL 演练）。
同日新增：[`2.6 Agent 会话轮换 + 2.7 撤销关闭持久连接`](reports/20261017-agent-session-rotation-revocation.md)
（session/generation、50% TTL 轮换、精确 tenant/target 关闭、撤销后停止重连；
不含 5s/60s 实测、身份服务故障演练或 HA）。
同日新增：[`2.8 真实身份演练`](reports/20261017-identity-drill.md)
（本地真实 HTTP/WebSocket/PostgreSQL 链路：跨 >2 TTL、Agent 重启恢复、撤销关闭 3.95ms；
不含生产 5min soak、云主机/HA、真实断网或 60s 多连接传播测量）。

2026-10-15 新增：[`Ubuntu 26.04 OS 适配与 19 工作负载部署复验`](reports/20261015-ubuntu2604-adaptation.md)。
`hnbctl os inspect` 已在真机验证；现有 K3s 路线上完成工作负载滚动恢复，
PG 2Gi/NATS 1Gi 的 TopoLVM 卷与三平面迁移 ledger 保留。该结果不提升 kubeadm
bootstrap、HA、离线或完整发布资格。命令说明见
[`hnbctl 稳定接口`](../deployment/hnbctl-interface.md)。

最近云主机测试：[`106.75.34.133：hnbctl 与依赖层真机测试`](reports/20261013-ubuntu-startup-host-test.md)、
[`Startup 组合部署与真机复验`](reports/20261013-startup-composition-deploy.md)、
[`凭据、启动可靠性与锁定渲染`](reports/20261013-startup-priority-optimization.md)、
[`2C4G 负载与容量实测`](reports/20261013-startup-capacity-under-load.md)、
[`Runtime 前置验证`](reports/20261013-startup-runtime-preflight.md)、
[`Provider conformance 登记`](reports/20261013-conformance-registration-and-operation.md)、
[`真实 Import Operation`](reports/20261013-runtime-import-operation.md)、
[`孤立步骤重驱动闭环`](reports/20261014-redrive-real-host-verification.md)。
CLI 冒烟/拒绝路径、verify 修复复测、数据库/JetStream 重启恢复、依赖故障与整机重启、
并发负载下的延迟/资源实测分别留存证据；完整 Startup 平台安装、Minimal 档位、
真实 Agent 纳管、HA 与离线仍 blocked，不从这些测试提升平台资格。

本地交付工具增量：[`prepare 镜像源、失败摘要与真实下载`](reports/20261014-prepare-download-verification.md)。
已验证显式镜像源映射、失败/取消后的部分结果与恢复，以及真实 OCI 冷下载/热缓存；
4.2 的节点 preload 仍依赖 4.4 授权，未提升为云主机或完整平台验收。

2026-10-14 已恢复评估主机 `106.75.34.133` 访问：本地代理隧道曾接管 SSH 路由，
绕开隧道后完成重驱动真机验证；具体时间及范围见该报告。

## 验收索引（OS × 拓扑 × 档位）

下表只列**有留存报告的真实环境**。没有报告的组合一律不列，并在下方"未验收组合"
显式说明，避免从其他组合推导。`partial` 表示已产出可复核证据但结论不足以支撑
对应结论；`blocked` 表示目标未达成且不得隐藏缺环。

| OS | 拓扑 / 环境 | 档位 | 状态 | 证据 |
| --- | --- | --- | --- | --- |
| Ubuntu 26.04 | 既有单节点 K3s v1.36.5+k3s1，4 vCPU/7680Mi，TopoLVM loop VG | OS 适配、完整 Minimal 19 工作负载滚动恢复 | passed（限定范围） | `20261015-ubuntu2604-adaptation.md` / `20261015-ubuntu2604-result.json` |
| Ubuntu 24.04 | 既有单节点 K3s v1.36.5+k3s1，2 vCPU/3914Mi | Startup 部署 | passed | `20261013-startup-composition-deploy.md` |
| Ubuntu 24.04 | 同上 | Startup 依赖可靠性（探针/中断/重启） | passed | `20261013-startup-priority-optimization.md` |
| Ubuntu 24.04 | 同上 | Startup 读负载容量 | **partial** | `20261013-startup-capacity-under-load.md`（含 limitations） |
| Ubuntu 24.04 | 同上 | Runtime 前置（12 工作负载、凭据登记） | partial | `20261013-startup-runtime-preflight.md` |
| Ubuntu 24.04 | 同上 + 隔离 SQL fixtures | provider conformance 登记 | partial | `20261013-conformance-registration-and-operation.md` |
| Ubuntu 24.04 | 同上（导入评估集群自身） | 真实 Import Operation | partial | `20261013-runtime-import-operation.md` |
| Ubuntu 24.04 | 同上，丢失请求与重复投递注入 | worker 孤立步骤重驱动 | passed | `20261014-redrive-real-host-verification.md` |
| Ubuntu 24.04 | hnbctl CLI 与依赖层（在线源安装） | 依赖/CLI 真机 | passed | `20261013-ubuntu-startup-host-test.md` |
| Ubuntu 24.04 | PostgreSQL 16.15 / NATS 2.10.7 | 依赖重启与备份恢复 | passed | `20261012-ubuntu-dependency-drill.*` |
| Rocky Linux 10.1 | 两台云主机，SSH+kubeadm 单控制面，kernel 6.12/containerd 2.2.7 | Kubernetes 创建 | passed | `registry.kubernetes_ssh_create` |
| — | 同上 | CAPI 创建 | **blocked** | 无报告 |
| — | — | 离线安装 | **blocked** | 无报告 |

**未验收组合（不得由上表推导）**

- Rocky Linux 10.1 的 Startup 组合、Minimal 档位、依赖恢复与容量：均无报告。
- Ubuntu 24.04 的 Minimal 档位完整 E2E、冷缓存 bootstrap、节点重启持久化、离线。
- 任何 HA / 多故障域 / Lite HA 结论。
- 完整安装执行路径（`hnbctl install` 实际资源执行）：render 已完成，install 未接通。
- 节点增加（node add）与平台升级的服务端契约与执行路由。
- 服务凭据真实部署闭环：apiserver 已提供受限 issue/renew/revoke、持久化撤销与
  actor/service subject 审计，但 operation-worker/Provider 的按租户 factory、部署引导
  和真实跨 TTL 演练尚未完成；历史真机结果仍依赖"签发后 8 秒内提交"，不能据此声明
  自动续期已上线。

**跨主机/跨进程未验证项**

- `hnbctl` 运行存储的跨进程与掉电 durability（尚未验证）。



唯一配置键为 `platform_settings.resource.cluster.kubernetesVersions`。
默认目录为空；仅通过真实创建验收且留存报告后才加入精确 patch 版本。
每项必须包含 `code`、`integrationTestStatus: passed`、`integrationReport`。
`integrationReport` 应指向包含 registry 中 report_fields 的不可变验收报告。
离线就绪另外要求 `bundleDigest: sha256:<64 hex>` 和 `offlineReport`。
布尔值只作为声明，不能取代报告/Bundle 校验；目录编辑应仅授权发布管理员。
当前字段引用门禁不是密码学验证报告真实性，CI 签名校验与证据存储仍待交付。

## 集群创建路线

当前可继续验收的路线为 **SSH + kubeadm 单控制面**，不得标为 CAPI 已实现。
管理集群/CAPI 是独立生命周期 owner；在真正的 CRD/controller/BOM 交付前，
禁止将其挂到 SSH 装机后作为“接管”步骤。
SSH success 必须经过 API readyz、精确节点数量、所有节点 Ready、所选 CNI 与
CoreDNS rollout 检查，才允许保存 kubeconfig 与返回 provisioned=true。
当前有真实报告的 CNI 路线为 Cilium 1.20.1：Rocky Linux 10.1 双节点和 Ubuntu
24.04 单控制面，具体环境/缓存/网络范围见 `registry.yaml` 的 `os_matrix` 和报告。
默认 Flannel 路线尚无保留的真实验收报告；HA、完全离线、证书轮换仍是未验收能力。
root 登录直接执行；非 root 用户必须预先具备免密 sudo，provider 使用 `sudo -n`
执行脚本。Cilium 路线须预置 Helm CLI。

验收须用独立目标，不得修改现有业务集群或复用部分失败的节点当作通过证据。
需记录 OS/架构、Kubernetes、kubeadm、containerd、CNI 精确版本和制品摘要。
网络代理由节点管理员配置，脚本不全局清空代理、不自动关闭宿主机 swap。
已安装 kubeadm 必须与目录中的精确 patch 版本一致，不匹配直接失败。
真实验收成功前，生产目录不启用任何版本。
