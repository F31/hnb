## Context

目标用户是交付工程师、客户运维与平台管理员。当前已存在 Compose 和 20 个子 Chart；集群生命周期 Provider 已验收 Rocky 10.1 双节点及 Ubuntu 24.04 单节点 kubeadm/Cilium 路线。证据在 `docs/acceptance/registry.yaml`，范围仅到 provider/SSH/凭据和网络，未覆盖完整平台交付。

已确认的交付缺口：仓库没有 hnbctl；Chart 使用 latest 和全局示例 DSN；Chart 依赖未包含 Portal、plugin-worker、runtime-target-lifecycle-provider；身份、KMS、NATS、数据库和平台入口需手工装配；BOM 包含明确标注的占位摘要。现有磁盘/内存较小的集群试验不能作为完整平台容量依据。

本文 CLI、DeploymentConfig 与 release lock 均是**拟实现契约**，不代表当前可执行命令或已发布交付包。

参考来源（访问日期 2026-10-11）：

| 来源 | 借鉴方式 | HNB 对应 |
|---|---|---|
| [KubeSphere 4.1 安装文档](https://www.kubesphere.io/docs/v4.1/03-installation-and-upgrade/02-install-kubesphere/02-install-kubernetes-and-kubesphere/) | KubeKey 配置建集群、Helm 安装 Core、扩展按需启用 | 节点配置与平台 values 分层，T0/T1 基础包与 T2/T3 可选包分开 |
| [KubeKey 命令模式](https://github.com/kubesphere/kubekey) | 单二进制、配置文件、扩容/升级、离线 artifact | hnbctl + 配置 + lock + 状态/恢复；其 README 版本表不作为 HNB 当前兼容性依据 |
| [RKE2 Quick Start](https://docs.rke2.io/install/quickstart) | server/agent 角色、配置、systemd、节点日志 | 明确控制面/worker 角色，标准运行目录、节点侧状态和可定位日志 |
| [Rancher Helm 配置](https://ranchermanager.docs.rancher.com/getting-started/installation-and-upgrade/installation-references/helm-chart-options) | 平台作为 Helm release，入口与 TLS 显式配置 | 固定 Chart、实际公共入口、TLS existingSecret 与平台就绪门 |
| [RKE2 Air-Gap](https://docs.rke2.io/install/airgap) | 固定版本/架构镜像、二进制、校验与本地导入 | HNB 完整 bundle 闭包，包含 OS 软件包和 CNI/平台/Provider 物料 |

## Goals / Non-Goals

**Goals:**
- 一个发布包、一个配置文件、一个 CLI 完成标准交付；操作步骤简单，结果仍严格可追溯。
- 第一期交付已有 Kubernetes 上的完整 Minimal 平台安装；第二期组合已验收 SSH 执行器，补齐空白管理节点 bootstrap。
- 区分管理基座、HNB 平台、业务集群纳管三个层次，并为每层提供检查、进度、恢复和验收。
- 在不增加消息/数据库产品、不绕过已有 Operation 边界的前提下复用现有能力。

**Non-Goals:**
- 现有平台运行时不切换成 KubeKey/RKE2 所有权；这些后端先做独立 Conformance。
- 不把现有方法级 E2E 扩大宣传为 HA、离线、全新 OS 一次安装或完整平台认证。
- 不让 CLI、Portal 或安装器成为平台在线时的另一执行事实源。

## Decisions

### 1. 工具分层与单一所有权

```text
交付包：hnbctl + DeploymentConfig schema + release.lock + charts + 文档
                         |
                 validate / preflight / prepare
                         |
          +--------------+----------------+
          |                               |
Day-0 管理基座 bootstrap             Day-1/2 托管模式
未纳管的专用管理环境                 CLI / Portal -> typed RuntimeIntent
共享节点阶段 -> kubeadm/Cilium               |
Helm -> HNB 自身平台                    immutable ExecutionPlan
bootstrap receipt                       persisted Operation
          |                               |
          +-> HNB ready -> 纳管交接     Worker -> lifecycle Provider
                                         -> 共享节点阶段
                                         -> Agent/观测 -> Read Model
```

拟新增 Go `cmd/hnbctl` 与共享安装阶段包。迁移现有 `internal/provision` 时保留 Provider 适配层；CLI 不导入 Provider 私有 SQL，也不复制第二份安装 shell。共享阶段只处理节点动作和真实观测，由调用方提供 Secret、证据策略、进度 observer 和结果存储适配。

**Day-0 边界必须先有契约测试**：仅首次安装 HNB 自身控制面到声明的专用、未纳管管理基座；不是通用离线 RuntimeTarget 执行器。配置中有 platform/target 引用、节点存在其他 lifecycle owner 或已有 HNB 管理标记时拒绝 bootstrap。平台 API 超时/503 不得触发 bootstrap fallback。不能证明该边界时先交付已有 Kubernetes 安装路线。

bootstrap receipt 只记录本次平台初始化的安装事实与自有资源，不伪造 RuntimeIntent、Operation 或 Read Model。平台上线后，注册管理关系通过正式 import Intent、Agent 认证和观测完成，receipt 只作关联附件。业务集群 create/import/扩容/升级始终经服务端；未支持动作显示原因码，不走 SSH 兜底。

替代方案：直接包装 kk/rke2 可减少安装代码，但当前 Kubernetes/OS/CNI 组合未在这些后端上认证；脚本另建 SQL 状态机则破坏平台事实边界。首期保留 kubeadm 所有权，借鉴其交付体验。

### 2. 标准部署路线和档位

| 路线 | 用户输入 | 安装所有权 | 放行条件 |
|---|---|---|---|
| 已有 Kubernetes + HNB | kubeconfig 文件、真实入口、存储、release lock | 只拥有 HNB namespace/release；保留原 Kubernetes owner/CNI | 平台 Minimal 完整 E2E |
| 空白管理节点 + HNB | 主机清单、SSH key、内网地址、入口、lock | Day-0 共享 kubeadm 执行器；handoff 后 Operation | OS/内核/CNI 配方和完整平台 E2E |
| 单机试用 | init 引导配置 | Minimal，明确单点；本地 PVC 必须持久化 | 同一基础业务就绪门，资源实测后开放 |
| Lite HA / Standard HA / Enterprise | 对应节点、故障域、外部服务/存储与锁 | 既有 owner 或独立认证的 HA 后端 | PostgreSQL、JetStream、Kubernetes、入口及持久化故障演练 |
| 离线 | 上述配置 + verified bundle | 与在线同一所有权 | 禁公网环境完整安装/重启验收 |

Compose 保留为本地开发/演示入口，不能将 `docker compose up` 的容器启动当作 Kubernetes/平台 HA 交付。生产优先已有 Kubernetes + 固定 Helm release；全新管理基座路线先开放 Minimal，HA kubeadm 尚不支持。

### 3. 配置与数据模型

配置 Schema 版本为拟定 `deployment.hnb.io/v1alpha1`。init 产生待填写配置，安装时拒绝空入口、缺失 lock 或凭据。下面为设计示意，地址是文档用私网地址，不能直接作为客户可用环境：

```yaml
apiVersion: deployment.hnb.io/v1alpha1
kind: DeploymentConfig
metadata:
  name: management
spec:
  mode: bootstrap                 # managed 模式必须提供 platform context
  profile: Minimal
  releaseLock: ./release.lock.yaml # 发布包提供真实内容；禁止手写示例摘要
  cluster:
    strategy: provision          # existing 时提供 kubeconfigFile
    distribution: kubeadm
    kubernetesVersion: 1.36.5     # 有集群层报告；平台兼容仍由 release lock 放行
    apiEndpoint: 10.0.0.10:6443
    nodes:
      - name: cp-1
        roles: [control-plane]
        sshAddress: 10.0.0.10
        internalAddress: 10.0.0.10
        user: ubuntu
        sshKeyFile: ~/.ssh/hnb_install
    network:
      plugin: cilium
      version: 1.20.1
      podCIDR: 10.244.0.0/16
      serviceCIDR: 10.96.0.0/12
      routingMode: native         # 跨节点路由条件由预检与配方确认
  platform:
    namespace: hnb-system
    publicBaseURL: ""            # 填实际入口；不存在可用域名默认值
    tls:
      existingSecret: ""         # 或由明确的 CA/证书路径生成
    storage:
      className: ""              # 客户实测可用 StorageClass
    infrastructure:
      postgresql: bundled        # 或 external + secretRef
      nats: bundled              # Minimal 文件持久化单实例，HA 奇数节点
  artifacts:
    mode: online                 # mirror / offline
    bundlePath: ""
  timeouts:
    download: 15m
    readiness: 10m
```

`distribution/plugin` 是用户选择的受认证实现，不是调用者可以注入的 ProviderId/steps。在线 `managed` 配置用 tenant-bound SecretReference 替代本地私钥/kubeconfig；提交前由客户端登记凭据，经服务端重验归属/用途，plan 只保存引用。

| 记录 | 字段与归属 |
|---|---|
| DeploymentConfig | schema、模式、档位、拓扑、网络/存储/入口、凭据引用；严格拒绝未知字段和无效组合 |
| ReleaseLock | releaseId/version、schema、所有物料实际 digest、OS/arch/内核配方、支持状态、报告身份 |
| PreflightReport | checkId、node、scope、observedAt、expected/actual、pass/warn/blocked、reasonCode、remediation；不含凭据 |
| BootstrapReceipt | runId、configDigest/lockDigest、管理基座 UID、阶段事实、资源 ownership、started/finished、证据引用；本地权限 0700/0600 |
| 在线状态 | existing Operation/steps 为权威；本地只存 operationId/planId/配置摘要与展示缓存 |
| AcceptanceReport | 层级、实际环境/缓存条件、actor、源码和物料、command、时间、退出码、断言、输出摘要 |

状态目录拟为 `$XDG_STATE_HOME/hnb/install/<runId>/`；节点仅有 `/etc/hnb/bootstrap/<runId>/` 和 `/var/log/hnb-install/<runId>/` 的自有标记/日志，不能把日志里的 shell 命令当作可重放计划。

### 4. 命令与实施体验

以下均为规划接口，实施分期以发布包能力为准：

```bash
hnbctl init --mode bootstrap --profile Minimal -f hnb.yaml
hnbctl preflight -f hnb.yaml --output report.json
hnbctl prepare -f hnb.yaml
hnbctl install -f hnb.yaml
hnbctl status --run-id <runId>
hnbctl resume --run-id <runId>
hnbctl verify --run-id <runId> --level platform
hnbctl collect --run-id <runId> --output diagnostics.tar.zst

# 平台上线后的操作；只使用正式 API 和服务端支持矩阵：
hnbctl cluster create -f cluster.yaml
hnbctl cluster import --kubeconfig-file <file>
hnbctl node add --target <targetId> -f nodes.yaml
hnbctl platform upgrade --release-lock <new-lock>
```

节点增加/平台升级接口尚需对应动作契约和 Provider Conformance；未就绪时返回 `TARGET_ACTION_UNSUPPORTED`，不打印假的安装指令或生成 no-op Operation。

安装成功输出：实际访问入口、临时管理员凭据文件位置/首次设置步骤、cluster/receipt/operation 标识、认证能力范围、验收报告路径与备份位置。凭据本身不打印到正常日志。

### 5. 预检、物料准备与进度

`preflight` 是只读操作，不能运行 apt update、安装 Helm、修改 sysctl、拉镜像到节点、创建测试 Pod 或创建 Operation。`prepare` 是明确的准备阶段；bootstrap 可下载发布包物料并预拉镜像，托管节点上的修改经 Operation。

本地 `prepare` 的 mirror 模式要求显式 `--mirror-origin`，只替换 OCI registry origin，
保留仓库路径和锁定摘要；不提供映射时拒绝，不回退原始源。该入口使用专用配置校验，
不会放开安装入口尚未实现的镜像源映射。失败/取消仍输出带 `lockDigest`、起止时间、
已验证物料与字节的部分结果，并返回非零退出码；重跑复验缓存，未完成对象重新下载。
进度记录观测时间、实际 origin、下载次数、真实字节和错误原因，不输出令牌或签名查询。
节点 preload 在 4.4 的 Day-0 所有权和 receipt 授权接通前保持拒绝。

检查目录：SSH/免密 sudo、实际 OS/arch/内核、cgroup/swap、runtime CRI/驱动、节点命名/地址、CIDR 重叠、时间/DNS、磁盘/可分配资源、包源和锁定镜像的实际可达性、已有集群/owner、API 与 HNB 入口路径、证书 SAN/有效期、存储与 Provider 矩阵。

端口由配方生成，至少包含安装机→节点 TCP22、节点/Provider→API TCP6443、控制面间 TCP2379-2380、指定控制来源→kubelet TCP10250、Pod→DNS UDP/TCP53、Agent→实际平台入口/隧道 TCP443/配置端口；Cilium native 检查节点直连/PodCIDR 路由，tunnel/RKE2 端口仅在其配方被认证后加入。PG/NATS/监控保持内部网络。不能把“公网 SSH 通”替代“worker/Provider/Agent 的目标路径通”。

镜像冷拉独立于集群 readiness 显示；有下载阶段、字节/重试/实际源和错误原因。先准备 locked 物料，再启动关键组件，减少首次 Cilium 下载压满就绪窗口。禁止自动清空代理、关闭防火墙或 SELinux/swap；差异输出具体节点与处理说明。

### 6. 平台安装阶段、状态机与成功门

#### Startup 真机反馈的实现补充（2026-10-13）

- 数据库客户端优先通过 `db.existingSecret`/`db.secretKey` 生成 `secretKeyRef`；旧内联
  `db.dsn` 仅作兼容。候选 `hnbctl render` 禁止内联 DSN 输入/输出，并按平面绑定 Secret。
- 核心数据库连接等待有界为两分钟，单次 ping 三秒；认证/数据库名错误立即失败。
  NATS 仅重试服务不可用/连接超时，返回实际已连接的客户端。
- 核心 HTTP 服务和 worker 提供 `/livez`、`/readyz`。依赖不可用只撤销 readiness，
  不作为 liveness 失败；startup probe 给予最多五分钟的启动窗口。
- `hnbctl render` 校验 Chart archive 摘要，使用包内 overlay 和 lock 中 amd64 manifest，
  明确 namespace，生成本地私有清单；同路径同内容可重放，不覆写不同内容。
  该命令不修改集群、不生成安装成功 receipt，也不替代完整 Minimal 安装认证。
- 已有集群安装计划先完成基础设施，再执行数据库迁移，再发布平台服务。

```text
Draft -> Validating -> Preflight -> Preparing
      -> [NodePrereq -> Runtime -> Init/Join -> CNI -> ClusterVerified]
      -> Identity/Infrastructure -> DatabaseMigration
      -> PlatformRelease -> PlatformVerified -> Handoff -> Succeeded

任一阶段 -> Blocked(条件不足) / Failed(执行失败)
Failed/Blocked -> Revalidate -> 从可恢复阶段继续
托管模式：本地状态只镜像现有 Operation，不能覆写其迁移规则
```

步骤必须带前置/后置条件及 ownership。成功的旧阶段重新核验后才能跳过；API 不可达、版本/cluster UID/lock 改变时拒绝复用。已初始化集群只恢复，绝不自动 kubeadm reset；需修复输入时产生新计划，不编辑 committed plan。并发安装通过 cluster/namespace/节点租约和 fencing 拒绝第二 writer。

运行存储（`install-run/v1`）的完整性约束（DPLY-004/PINST-001 加固）：

- 状态文件必须为 0600 普通文件、目录必须为 0700 且非 symlink；损坏状态
  **永不**被新运行覆盖，只能由操作员修复后继续。
- 恢复身份比 lock/config 更完整：run id、路由模式与 ownership 边界任一变化都拒绝，
  防止把错误目标/作用域当成原运行继续。
- 所有读-改-写在同一运行目录的排他文件锁内执行；并发写者只有一个能获得租约。
- fencing epoch 在写者更换**或租约过期**时递增；过期令牌不能继续变更状态。
- 已完成（succeeded）运行进入终态，之后任何阶段变更被拒绝。
- `init/join/cluster-verified/database-migrate` 是一次性阶段：无论已成功、失败、
  阻塞或中断，恢复都只允许 `recheck_or_refuse`，绝不重放。
- 阶段超时预算覆盖前置检查、执行与后置检查；预算耗尽后的"成功"不计数。
- 层判定要求该层每个必需阶段都存在且成功；先前的失败/阻塞或缺失的强制阶段
  不会被后续阶段（例如 `platform-verified`）的成功掩盖。

平台安装依赖 DAG：namespace/存储 → PostgreSQL、JetStream、身份/KMS/服务凭据 → 有 owner lock 的迁移 → API、Operation Worker、Provider、市场/插件执行器、Portal、Tunnel/Router → 初始账户及 registry → 完整验证。Chart 只描述资源，hnbctl/Operation 明确编排迁移和就绪。不能靠固定 sleep 或 Hook 启动成功推进 schema。

验收分层：
1. Node：包版本、CRI、cgroup、服务与网络前置条件。
2. Cluster：readyz、节点身份/数量/Ready、CNI/CoreDNS、Pod/Service/DNS。
3. Platform：实际入口登录、IAM/租户、迁移版本、PG 持久化、JetStream 消息/订阅、Provider route 健康。
4. Managed：一个最小真实 Release → Intent → Plan → Operation → Agent/观测/Read Model 的闭环，并删除该测试自有资源。
5. Delivery：重启持久化、备份/恢复演练及用户操作卡；HA/离线另有故障/断网场景。

任一必需层失败不输出“平台交付成功”。管理基座只 Ready 时输出 `cluster_ready/platform_not_verified`，返回对应非成功退出码。

### 7. 物料锁、离线与档位

ReleaseLock 包含 CLI、Chart、所有 T0/T1/选定 Pack 的 OCI digest、PG/NATS、Provider、Portal、kubeadm/kubelet/kubectl、containerd/runc、pause、CNI/Envoy/operator、Helm、OS 包及依赖、迁移 checksum。镜像 index 与目标架构 manifest 都记录。国内/客户镜像源是显式映射，验证实际目标摘要，不猜测 tag 等价。

bundle 包括 lock、schema、charts、OCI layout/images、OS/arch 包仓库、工具、迁移、证书生成工具/模板、CHECKSUMS、发布身份/签名及操作手册。包不包含客户凭据。在线准备与离线安装共用依赖解析器，离线执行不得访问公网作为兜底。静态闭包检查和禁公网 E2E 均通过才标 offline-ready。

支持矩阵当前事实：

| OS/路线 | 已有证据 | 平台安装器状态 |
|---|---|---|
| Ubuntu24.04/6.8、amd64、kubeadm1.36.5、Cilium1.20.1、sudo | 单控制面 provider、网络、KMS；最终重建有缓存 | 待完整 Minimal 交付验收 |
| Rocky10.1/6.12、amd64、同 K8s/CNI、root | 单控制面+worker、跨节点网络、KMS；最终重建有缓存 | 待完整 Minimal 交付验收 |
| arm64/其它 OS、Flannel/RKE2/KubeKey | 无本项目保留完整报告 | blocked/Conformance 候选 |
| HA/升级/离线 | 未有对应闭环证据 | blocked |

四档复用既有 Minimal/Lite HA/Standard HA/Enterprise 名称。Minimal 默认仅 T0/T1；HA 要求至少三独立控制面故障域、奇数 JetStream 集群及验证过的 PostgreSQL 恢复/切换，单纯 replicaCount=3 不算 HA。容量按 Chart requests、系统预留和缓存/持久化预算计算，建议起步值不是认证值。

### 8. API/事件、权限和跨平面证明

#### 控制台与节点离线制品统一管理（2026-10-14 需求补充）

交付范围同时覆盖控制台/管理平台、控制面与 worker；离线版本不是仅打包 Portal
前端资源或容器镜像。按安装角色从同一 ReleaseLock 解析依赖，包含所需 CLI、Chart、
平台与 Kubernetes/CRI/CNI 镜像、节点工具、Ubuntu/Rocky 软件包及其依赖、Schema
和迁移等实际选定物料。具体版本/摘要以发布目录解析结果为准。

统一管理分为两个可衔接入口：

- 平台安装前：随发布包交付可离线读取的物料目录/锁文件及内容寻址物料，复用
  bundle prepare/verify/import 和本地缓存验证，不依赖尚未运行的制品服务。
- 平台运行后：通过现有制品 owning-plane 的公开接口接入发布目录及内容身份，
  控制台与 CLI 使用同一目录展示版本、依赖、OS/架构/角色、完整性和认证范围。
  本增量不另建安装专用制品事实库，所需 API 适配差距在实现任务中先行盘点。

在线源和离线副本均绑定同一摘要；同内容重复导入幂等，摘要不符或缺失依赖拒绝。
物料齐备与真实断网验收分别展示，离线目录存在不代表当前拓扑已经认证。
现有 7.1 的 bundle 基础实现保留原完成状态；统一目录接入和双入口一致性作为
新增任务验收，7.2/7.3 继续承担独立 Ubuntu/Rocky 断网安装验证。

#### Runtime provider conformance 登记增量（2026-10-13）

Startup 实测发现 manifest 登记默认 `none`，没有可信证据更新入口。新增平台 owning-plane
`POST /v1/providers/{id}/conformance`，要求既有 provider execute 权限以及独立受信签名者
的 Ed25519 报告；签名公钥由部署配置接入，私钥仅由隔离测试/发布者持有。
报告绑定 schema、run/source/environment、provider id/version、OCI manifest digest、当前
基础 manifest 摘要、矩阵版本、targetKind/action、开始/结束/过期时间及内容寻址证据引用。
首个登记实现仅支持 KubernetesTarget/import，必须具有 contract/functional/security/
fault/idempotency/fencing 各类通过结果；其他动作保持拒绝。报告不能由普通用户自报
`production_ready`。全局 legacy level 仅作为 resolver 兼容字段，放行范围仍严格限制在
签名报告的精确矩阵 cell，不代表完整生产/HA/Minimal 认证。
平台在事务行锁内重新核验 manifest 身份，将签名报告和 cell 证据原子保存在已有
provider_manifests JSON 字段；更新基础 manifest 会清除资格。未经签名、失败、过期、
错物料或缺少类别的报告不得改变资格。新表/新中间件为 N/A。
真实报告须来自使用相同 provider 镜像、独立测试数据库和自有测试资源的隔离执行；
不得在当前业务库插造 Operation 或观察事实来绕过准入。 worker 在登记后固定最终
manifest 摘要和专用 audience 服务令牌，之后经正式 Intent 执行验证。

#### 孤立步骤重驱动增量（2026-10-13）

真机实测发现：步骤请求消息在 worker 消费者不在时投递，随后 worker 重启，该步骤会
永久停留在 `pending`——JetStream 的 `AckWait` 重投递只在有消费者时推进，而
`ProviderReconciler` 只处理 provider binding 漂移，没有任何组件重驱动孤立步骤。
新增 worker 内周期清扫（默认 60s，空闲阈值 120s，必须大于 60s `AckWait`）。规则：
仅处理非终态 Operation 下、`pending` 步骤、且不存在未发布请求的记录；用
`FOR UPDATE ... SKIP LOCKED` 取候选，扫描只能运行一次。

重投递**不新增 outbox 行**，而是把该步骤已提交的那一行重新武装
（`status=pending`、`attempt+1`、`next_attempt_at=now()`），并由步骤 `max_retries`
与该行 `max_attempts` 双重封顶：递增后的 attempt 必须仍严格小于 relay 的
`max_attempts`，否则保留 published 并告警，避免制造永远不能发布的 pending 行。
仅选择与 `operation_steps.idempotency_key` 一致的原始请求，避免选择旧构建的派生行；
存在任何未发布请求时不重新武装。真机验证推翻了最初"复制 payload 并使用
`#redrive-<n>` 派生幂等键"的设计：`outbox_events` 对 tenant+type+idempotency_key
唯一，派生键的新行会被 worker 以 `envelope and payload idempotencyKey differ` 隔离
（worker 要求 envelope、payload 与 `operation_steps` 三者同键），而改写 payload 又会
破坏与步骤行的匹配。重新武装既满足唯一约束，也保留单一可审计行，正是 outbox
`attempt` 计数器的用途。空闲阈值必须以毫秒显式换算（`$5 * interval '1 millisecond'`），
	否则 PostgreSQL 会把整数当秒，条件恒为假。
重复执行风险由既有 `AcquireLease` 的单持有者与 fencing generation 兜底。
超过预算的步骤保持 `pending` 并输出显式日志，不自动失败——自动失败需要与真实执行
相同的租约保护状态迁移，应由运维决定，避免 worker 静默终止 Operation。

CLI 只通过 owning plane 的公开、版本化 API 获取兼容矩阵、登记 SecretReference、提交 typed Intent、查询 Operation/观测和导出证据。新预检查询接口若需要，在契约模块定义；不暴露直接 Provider execute、任意 steps/commands 或 DSN 给 CLI。新增节点动作必须先有对应服务端动作契约，不以 UI 参数规避规划。

事件复用现有 command/outbox/operation 与 RT-008 观测契约；bootstrap 进度为本地、非领域权威记录。Portal 从 API 读取事件投影/进度，不直连 NATS；下载使用内容寻址 URL/OCI 客户端，API 不代理包字节。

平台、市场、制品等平面共享 PostgreSQL 产品但使用独立逻辑数据库、身份和 owner 权限，不能跨平面查询表。在线运行状态仍只有 PostgreSQL Operation Store。审计绑定 actor/tenant、config/lock digest、Intent/Plan/Operation、SecretReference 与真实结果；凭据仅执行期间解析，诊断包脱敏。

### 9. 升级、回滚、备份和卸载

升级先计算 diff/兼容性与备份要求，再提交 Operation；DB expand/contract 迁移有 owner lock 和前向恢复步骤，不能以 Helm --atomic 或 rollback 假装回退数据。备份覆盖 PostgreSQL、JetStream 持久状态、制品索引/对象及不可再生的身份/KMS 材料，分别按 owning plane 导出并验证可恢复性。

平台正常时，升级/回滚/卸载走 Operation；平台故障时，现场恢复仅针对绑定 receipt 的 HNB 管理基座和备份，不能执行业务目标任务或覆盖旧 Operation 事实，恢复后重对账。bootstrap 诊断/恢复包与此边界在上线前独立评审。

卸载使用 ownership 清单，区分“卸载 HNB”“解除纳管”“删除由本次 bootstrap 创建的管理基座”。共享 PG、CSI、已有 Kubernetes、业务 PVC/工作负载不由卸载 HNB 隐式删除。删除存储需明确指定资源并遵守既有 Operation 策略。

## Risks / Trade-offs

- [首次初始化可能成为执行旁路] → Day-0 仅 HNB 自身未纳管管理基座；所有权检查/封闭命令集/handoff 与 API 失联场景测试先行，边界不可证明则延期该路线。
- [当前完整平台 BOM 不可发布] → 补齐实际 digest、缺失服务、身份与迁移；全量 E2E 后才发布，既有方法级报告只作兼容候选。
- [冷镜像/镜像源重试] → prepare/preload、分阶段 timeout 与真实重试指标；记录空缓存与热缓存分别结果。
- [HA/离线被误当成选项开关] → 支持矩阵以各路线证据门控，未支持返回明确原因。
- [宿主机配置被覆盖] → 复用现有命名 import，保存变更清单，不能整文件替换客户 runtime/network 策略。
- [重试与多人并发创建重复集群] → immutable inputs、作用域幂等键、节点/cluster ownership/fencing、单 writer 与恢复后置检查。
- [数据回滚不可逆] → 迁移独立于 Chart revision，备份恢复演练和 forward recovery 明确分类。
- [过早增加新后端] → kubeadm 首期唯一新建集群 owner；RKE2/KubeKey 和 arm64 另经 Conformance。

## Migration Plan

P0：固化现有 SSH 实测代码与证据，发布配置/支持矩阵规范，锁定最小物料，先补已有 Kubernetes 的完整平台 Chart 与 E2E。

P1：交付 hnbctl init/preflight/prepare/install/status/verify/collect；在专用环境认证 bootstrap 边界、共享阶段与恢复，完成 Ubuntu/Rocky 管理基座 + 完整 Minimal 平台。

P2：Portal 对齐配置/预检、补齐经 Operation 的 node add/生命周期动作、首次在线交付恢复及升级/DB 恢复演练。

P3：离线依赖闭包、禁止公网安装；再认证 HA、其它架构或 RKE2/KubeKey 后端。未达到各阶段退出门槛不开放相应路线。

回退：新工具按显式选择启用，保留已固定部署物料与状态；停止新入口可回到既有运维路径。回滚工具版本不自动回滚客户数据库/节点，资源按 ownership 和恢复契约处理。

## Open Questions

- 完整平台首个可发布 release/version/digest 由哪次认证构建产生？目前 Chart0.1.0/占位 BOM 不能作为答案。
- 客户首期入口与存储选型、外部 PG/OCI 的责任边界及实际 RTO/RPO；未提供时不能宣称生产交付通过。
- 当前 northbound lifecycle/Agent 完整链路还有哪些断点？必须在安装器成功门开放前验证。
- 若 bootstrap 契约评审认为“未纳管管理基座”仍属于既有 RuntimeTarget 写边界，则该路线保持 blocked，改用外部建 Kubernetes + HNB 首次 Helm 安装，或先提出独立的最小 bootstrap control-plane change；本 change 不静默放宽 KERNEL-002。
