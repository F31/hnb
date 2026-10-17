# hnbctl：简单命令与稳定接口

## 使用方式

本仓库交付工具的命令名是 **hnbctl**。参考 kubectl 的资源/命令组织、kubeadm 的
配置文件驱动和 Helm 的参数覆盖方式，复杂部署参数放在配置中，日常命令保持简短。
命令提供 kube-key 风格的简洁动词（`create`、`add`、`delete`、`upgrade`、
`artifact`、`completion`），它们是既有完整命令的快捷入口，不改变行为或退出码。

```bash
hnbctl --help
hnbctl os                         # 支持的 OS、包管理器、验证范围
hnbctl os inspect                 # 当前主机的兼容性观察，无需 hnb.yaml
hnbctl os inspect --format json   # 自动化读取，不解析人类表格
hnbctl init --mode existing -f hnb.yaml
hnbctl init --from-kubeconfig ./cluster.kubeconfig -f hnb.yaml   # 反推 existing 草稿
hnbctl help preflight
hnbctl create config --mode existing -f hnb.yaml   # 同 init
hnbctl completion bash            # 生成 shell 补全（bash|zsh|fish）
```

Ubuntu 26.04 的 bootstrap **草稿**可用以下命令生成；草稿不等于该路线已经认证：

```bash
hnbctl init --mode bootstrap --os ubuntu-26.04 -f hnb.yaml
```

`--os` 选择草稿适配与默认用户，草稿注释记录所选 OS；实际制品 OS/架构必须由
所选 ReleaseLock 与节点观察共同确认。不能仅更改注释或参数就授权安装。

## 当前功能范围

| 功能 | 命令 | 当前范围 |
|---|---|---|
| 帮助、版本 | `help`、`version`、`--help`、`-h`、`--version` | 可用 |
| 命令补全 | `completion bash\|zsh\|fish` | 可用，覆盖顶层命令与常用子命令 |
| OS 兼容性 | `os [list]`、`os inspect` | 可用，观察与认证分离 |
| 配置 | `init`、`validate` | 草稿生成、结构/锁定材料校验；`init --from-kubeconfig` 从已有集群 kubeconfig 反推 existing 草稿（只解析，不联网、不读凭据） |
| 验证矩阵 | `matrix` | 显式显示已验证与 blocked 范围 |
| 预检 | `preflight` | 节点 SSH/OS/权限/资源、API 路径、制品/入口等部分检查；缺失检查显式阻塞；多节点只读探测有界并发（默认 8），输出按声明顺序 |
| 依赖与容量 | `recipe show`、`capacity` | 节点依赖与渲染 requests 规划，不替代运行峰值验证；`capacity --evidence` 把能力证据门禁接入安装授权决策 |
| 发布制品 | `lock generate`、`prepare`、`bundle`、`release` | 候选锁、下载/校验、离线闭包工具、发布目录；不代表离线整平台认证 |
| 身份与渲染 | `identity generate`、`render` | 私有实例凭据生成、锁定 Chart 渲染 |
| 安装状态 | `install`、`status`、`resume`、`verify`、`collect` | 接口及部分能力已实现；完整安装/分层验收仍有门禁与未接通项 |
| 极简入口 | `install --host`、`install --kubeconfig`、`uninstall --installation` | 参数自动生成配置并保存私有快照；无需编辑 YAML。当前仍为计划/只读预检，尚未执行安装或删除 |
| 进度/历史/日志 | `status --watch`、`history`、`logs` | 进度只呈现记录到的阶段；历史与日志读取本次安装的私有记录；`logs --phase/--reason/--after`、`history --state` 过滤 |
| 卸载 | `uninstall [-all] [-data]`、`platform uninstall-hnb\|unmanage\|delete-management-base` | 默认自动检测并删除 HNB 组件；`-all` 加删 TopoLVM/SC/CSIDriver/CRD；`-data` 加删 PVC/PV/LV/Secret/namespace；`--plan` 只看计划；`unmanage`/`delete-management-base` 仍走服务端权威契约 |
| 纳管操作 | `cluster create/import/list`、`node add`、`operation get`、`handoff` | 服务端 API 驱动；`cluster list` 读取服务端权威 runtime-target 列表（`cluster-list/v1`），平台不可达不回退本地视图；`node list/status` 在平台 API 无契约，诚实拒绝并指向 `cluster list`/`verify` |
| 生命周期 | `platform`、`recovery verify` | 独立的升级/卸载/恢复身份检查接口；不是所有操作均已端到端认证 |

用 `hnbctl help <command>` 查看该命令的实际参数。命令存在不表示所有 OS、拓扑、
发布版本或执行链均通过认证。完整 Minimal 在评估节点上运行成功，与整个
`hnbctl install` 自动执行链获得发布资格是两个范围。

## 安装进度、历史与卸载

安装记录落在私有目录（0700/0600），默认 `$XDG_STATE_HOME/hnbctl/runs/<runId>`
（未设置时使用 `~/.local/state/hnbctl/runs/<runId>`），
可用 `--runs-dir` 覆盖。状态与它的转换日志在同一次原子写入中提交，因此不存在
"有进度无日志"或"有日志无状态"的中间态。

```bash
hnbctl install -f hnb.yaml                     # 落盘本次安装 run（默认只规划，不授权执行）
hnbctl status                                  # 进度（默认取最近一次记录）
hnbctl status --watch --interval 1s            # 持续刷新到终态
hnbctl history                                 # 安装历史，最新在前
hnbctl logs                                    # 状态转换日志（--tail / --follow / --format json）
hnbctl uninstall --installation <runId>          # 从保存的配置恢复卸载上下文，默认保留数据
```

要点：

- 进度阶段取自实际记录的阶段状态；既有集群路线不展示 init/join/runtime 等节点阶段，
  避免呈现本次安装并未执行的动作。
- 仅规划而未授权执行的安装记为 `blocked`，不会显示为安装完成。
- `logs` 只输出状态转换与稳定原因码，不含凭据、密钥或节点初始化材料。
- `uninstall` 必须绑定记录在案的 run：配置身份不一致直接拒绝；默认计划保留 PVC
  及命名空间，`--delete-data` 才将自有持久卷加入删除计划。旧参数
  `--acknowledge-storage-removal` 继续表示显式同意将自有持久卷加入删除计划。
- `uninstall` 零配置模式自动解析 `KUBECONFIG`、`~/.kube/config` 或
  `/etc/rancher/k3s/k3s.yaml`，检测 `hnb-system`、TopoLVM/SC 或 LV 残留后删除
  HNB Deployment/StatefulSet/DaemonSet/Service/ServiceAccount/Role/RoleBinding、
  HNB ConfigMap（保留 `kube-root-ca.crt`）以及带 `app.kubernetes.io/instance=hnb`
  的 ClusterRole/ClusterRoleBinding。
- 默认保留 PVC、Secret、Namespace、StorageClass/TopoLVM。`-all` 删除
  `topolvm-system`、`hnb-topolvm`、`topolvm.io` CSIDriver、TopoLVM cluster RBAC；
  `-data` 删除 PVC/PV/LV/Secret/namespace；`-all -data` 再删除 LV CRD。
- 旧记录式 `uninstall --installation`/`--run-dir -f` 仍要求 run/config 匹配；
  `unmanage`/`delete-management-base` 不在本地执行。执行结果输出
  `removal-execution/v1` 并验证无 Pod/namespace 残留。

## 极简模式：不编辑配置文件

两种入口复用同一配置模型、发布锁验证和运行存储。极简模式表示少编辑配置，
不是裁剪 Minimal 组件。以下命令**当前生成计划/执行已有的只读预检，不实际安装或删除**。

```bash
# 空白主机；--plan 仅生成计划，不登录主机，也不询问密码。
hnbctl install --host 192.0.2.10 --user ubuntu --ask-pass \
  --domain hnb.example.com --tls-secret hnb-tls --storage-class hnb-topolvm \
  --lock ./release.lock.yaml --plan

# 私钥替代密码：--ssh-key ~/.ssh/hnb-install；节点内网地址不同可加
# --internal-address 10.0.0.10（缺省与 --host 相同）。

# 已有集群；省略 --plan 会执行已有只读预检，结果持久化为 preflight.json。
hnbctl install --kubeconfig ./cluster.kubeconfig \
  --domain hnb.example.com --tls-secret hnb-tls --storage-class hnb-topolvm \
  --lock ./release.lock.yaml

# 精确选择记录：保留数据 / 审阅删除数据 / 确认将自有数据纳入删除计划。
hnbctl uninstall --installation <id> --plan
hnbctl uninstall --installation <id> --delete-data --plan --format json
hnbctl uninstall --installation <id> --delete-data --yes
```

- `--host`、`--kubeconfig` 与 `-f` 三种配置来源互斥。`--host` 声明单个空白节点，
  不扫描或自动接管已有集群。已有集群的 Pod 节点归属由 Kubernetes 调度约束决定。
- 缺少 `--domain`、`--tls-secret`、`--storage-class` 时，真实终端会询问；自动化
  必须明确提供。不会虚构域名、证书、StorageClass 或发布锁。默认读取当前目录
  `release.lock.yaml`，也可使用 `--lock` 指定。
- 默认 `--profile Minimal`、`--namespace hnb-system`、`--name hnb`、`--user ubuntu`。
  主机入口使用当前 bootstrap 草稿的 kubeadm/Cilium 版本，并校验发布锁兼容性。
  名称相同不意味着可以覆盖已有 release；实际归属/冲突检查仍是执行前置条件。
- 新记录自动保存 `deployment.yaml`（规范化绝对路径）和 `release.lock.yaml`（物料
  快照），均为 0600。卸载恢复时核对配置与发布锁摘要，拒绝缺失、篡改或链接的快照。
  密码不保存，私钥和 kubeconfig 只保存路径引用；凭据文件仍须由操作者保管。
- 安装 ID 当前等于 run ID，可用 `history` 查询。新的 `-f` existing 安装计划也保存
  快照；旧记录没有快照时仍需 `uninstall -f <原配置>`。
- `--plan` 返回 0 仅表示计划生成成功：报告仍有 `executionAuthorized=false`，运行
  状态仍是 `blocked`。普通极简调用的预检失败返回相应非零码，并保存阶段结果；
  `--yes` 不绕过缺失预检、发布证据或未接通的执行器。
- `--execute` 是显式执行入口：它先强制执行能力证据门禁。对已有集群路线，门禁通过
  且提供 `--chart-package` 与 `--bindings` 时，会渲染锁定 Chart、`kubectl apply`
  清单并等待 `--workload Kind/name` 的 rollout，逐阶段记录。若同时提供
  `--migrations-dir`，会在应用后从各平面 Secret 读取 DSN、按 owner 应用迁移并记录
  `database-migration` 阶段；DSN 只用于连接，绝不打印或留存。已有集群 + Minimal
  证据已认证（2026-10-16），此路线门禁通过；缺少 `--chart-package`/`--bindings` 时
  返回参数错误。`--plan` 与 `--execute` 互斥。

迁移与 Secret：`toolkit.RunInstallMigrations`（门禁化）按 owner 顺序应用迁移，首个
平面失败即失败关闭并返回部分结果；`toolkit.ReadSecretKey` 通过 `kubectl` 读取
Secret 指定键并 base64 解码，错误只按类别返回（`SECRET_READ_FAILED`/`SECRET_KEY_MISSING`/
`SECRET_KEY_INVALID`），不泄露值。执行器只覆盖已有集群路线；bootstrap 还需接入
  `ManagementBootstrap`。

发布 applier 现状：`toolkit.PlatformApplier`（`kubectl apply` + `rollout status`）
与门禁化管线 `toolkit.RunPlatformInstall` 已实现并单测（假 `kubectl` + 合成已认证
证据）。管线**不执行数据库迁移**（需要各平面凭据），因此即使应用成功，平台层仍
显示为未完整交付，不会把“已 apply”当作安装成功。
- 卸载默认保留平台数据、命名空间、外部资源和已有 Secrets。`--delete-data` 不加
  `--plan` 时要求输入 `yes`，自动化加 `--yes`；这只确认计划中的数据范围。
  `authorized` 是计划边界是否满足条件，`executionAuthorized=false` 表示仍未授权实际删除。
- 主机计划不是已安装证明：没有已观察的基座身份和自有资源清单时，拒绝从主机计划
  推导卸载或销毁 Kubernetes 基座的权限。已有集群的基座始终不属于本次平台安装。

当前完整安装执行、安装变更前的最终确认、实际删除及结果验证仍待执行链接通；
极简参数不会跳过这些门禁，也不会把计划成功当作安装成功。

执行链现状：节点/集群引导适配器 `ManagementBootstrap` 已通过门禁化管线
`toolkit.RunManagementBootstrap` 接入 `install --execute`（空白主机路线，需
`--image-repository`/`--sandbox-image`/`--cni-image-repository`/`--chart-repository`）；
发布 applier（`kubectl apply` + rollout，`toolkit.RunPlatformInstall`）与数据库迁移
（`toolkit.RunInstallMigrations`，`--migrations-dir`）已接入已有集群路线。

证据状态（2026-10-16）：`existing` route 与 `Minimal` profile 已认证
（`docs/acceptance/reports/20261016-existing-minimal-e2e.md`），因此已有集群 + Minimal
**通过证据门禁**；`install --execute` 在提供 `--chart-package`/`--bindings` 时会渲染并
应用该路线（可选迁移）。`bootstrap` route 与 HA/Enterprise/offline 仍为 blocked，故空白
主机 `install --execute` 仍稳定拒绝；`CertifiedOS` 不含 ubuntu-26.04，避免把已有集群证据
提升为 kubeadm bootstrap 认证。

## SSH 密码与专用密钥登记

节点配置的 `sshAuth` 支持 `key`（缺省）或 `password`。私钥模式继续使用
`user`、`sshAddress`、`sshKeyFile`；密码模式不要求 `sshKeyFile`，密码由运行时提供，
不放入 YAML。已有 Kubernetes 路线仍使用 kubeconfig，不接受节点 SSH 密码参数。

```bash
# 隐藏输入登录密码；需要 sudo 密码时另行询问，二者不会默认共用。
hnbctl preflight -f hnb.yaml --ask-pass --ask-sudo-pass

# 自动化使用私有文件（0600），不提供命令行明文密码参数。
hnbctl preflight -f hnb.yaml --ssh-password-file /run/secrets/ssh-password \
  --sudo-password-file /run/secrets/sudo-password

# 单独登记专用密钥：先验证密码连接，再显示账号和修改位置并要求输入 yes。
hnbctl ssh enroll -f hnb.yaml --node control-plane-1 \
  --key-file ./credentials/control-plane-1 --ask-pass
```

- 操作端需安装 OpenSSH、`ssh-keygen`；密码认证还需 `sshpass`。密码通过专用文件描述符
  传递，不进入 argv、环境变量、安装记录或日志。隐藏输入要求真实终端。
- 两种认证模式都严格校验已有 `known_hosts`，不自动信任未知主机密钥。
- 不传 sudo 密码时要求 root 或已有免密 sudo。传入时单独验证 sudo 凭据；账号须拥有
  所需 sudo 权限（包含认证用的 `true` 和后续 shell）。不自动修改 sudoers。
- 预检调用中的登录密码用于本次声明的所有节点；节点密码不同时，可逐节点登记密钥，
  后续为各节点配置独立的 `sshKeyFile`。
- `ssh enroll` 仅处理明确指定的一个节点；自动化可加 `--yes`。确认前不生成密钥或修改
  远端。新密钥为本地 Ed25519 私钥（0600），已有专用密钥会复用且不覆盖。
- 远端通过 `flock` 串行追加公钥，保留原有 `authorized_keys`，重复登记不重复追加；
  不修改 sshd 配置，不关闭密码登录。目标主机需要 `flock`。
- 用新密钥重新连接成功后，保存 `<key-file>.enrollment.json`（0600，
  `ssh-enrollment/v1`），其中记录节点、私钥路径和验证时间；输出如何设置
  `sshAuth: key` 与 `sshKeyFile`。不会覆盖原部署 YAML。
- 验证失败会返回非成功并说明可能已追加公钥，不生成新的成功记录；保留密码登录。
  平台卸载不会顺带移除登录密钥。
- 认证执行器的只读运行和流传输均支持密码方式；完整 `install` 自动执行链仍按前述
  范围保持未接通，增加认证方式不改变安装授权结果。

## 容量与安装授权

`capacity` 是渲染请求的**规划估算**，不替代运行峰值验证；`installAuthorized` 是
独立于容量估算的证据决策：

```bash
hnbctl capacity --profile Minimal --manifest rendered.yaml --nodes nodes.yaml
hnbctl capacity ... --evidence hnb.yaml --material-verified
```

- 不传 `--evidence` 时，`installAuthorized` 恒为 `false` 且不新增原因码。
- 传 `--evidence` 时，`certified` 由实时能力证据门禁（`EvidenceGate`）推导，
  拒绝原因写入报告；`--material-verified` 是操作者声明的已核验事实（同
  `--db-compatible`）。仅当 certified + material-verified + 容量足够三者同时
  满足才授权。Minimal/existing 已认证，凭该证据可授权；HA/Enterprise/offline 未认证时
  授权保持拒绝并如实呈现原因。

## OS 检测包含什么

`hnbctl os inspect` 以固定只读程序观察：

- `/etc/os-release` 的 OS 与版本、内核、CPU 架构、cgroup。
- 对应包管理器、适配状态、精确 OS/kernel/arch 的证据。
- 已有 Kubernetes/K3s/RKE2/HNB owner 标记、活动 swap 条目数。
- CPU 数量、总内存和根文件系统可用磁盘。

它不检查所有可选软件包是否已安装，也不代替 Kubernetes API、DNS、TLS、
存储驱动、包依赖闭包与业务验收。后者使用部署相关预检和独立验收。
默认观察超时 15 秒；不自动改 swap、AppArmor、防火墙、软件源或已有集群。

## 机器接口约定

- 兼容演进优先新增命令/可选参数，保留现有名称、退出码与已有字段语义。
- 人类输出可改善排版；自动化使用 `--format json` 和 `schemaVersion`。
- 当前 OS 接口：目录 `os-catalog/v1`；本机观察 `host-os/v1`。
- 当前安装记录接口：进度 `install-progress/v1`、转换事件 `install-event/v1`、
  历史 `install-history/v1`、日志 `install-logs/v1`。
- JSON 报告写 stdout，错误及稳定原因码写 stderr；调用方同时检查退出码。
- `os inspect` 的 0 表示观察到的 OS/kernel/cgroup 在已验证范围内，**不表示安装授权**。
  例如 Ubuntu 26.04 的 `bootstrapCertified=false`、owner=k3s 是有效且重要的结果。
- 未知/未验证 OS/kernel/cgroup 返回 3（`PREFLIGHT_BLOCKED`），同时保留可用的 JSON
  观察结果；参数错误返回 2。
- 不接受“OS 名称能识别 ⇒ 任意内核可用 ⇒ 全平台可安装”这种隐式推导。

既有退出码：0 成功，1 执行/输出错误，2 参数或配置错误，3 阻塞，4 未完成验收，
5 服务 API 无法连接。不同命令的成功范围以命令说明为准。

## 形态升级与按需扩容（planning）

- 形态（Minimal/LiteHA/StandardHA/Enterprise）表达为可比较的**能力向量**
  （capabilities/components/HA 故障域/backings/storage/resources），由
  `deploy/charts/hnb/overlays` 单一事实源派生并受 parity 测试守护；偏序
  `Minimal ⊂ Lite ⊂ Standard ⊂ Pro`，目标必须是当前能力超集。
- `backings` 三值：`bundled`（Minimal 单实例，默认）、`in-cluster-ha`（HA 档默认，
  控制节点内部署 CloudNativePG PostgreSQL + 3 节点 JetStream）、`external`（外部托管，
  显式可选）。偏序 `bundled < in-cluster-ha ≈ external`（in-cluster 与 external 互不
  为降级，回退到 `bundled` 被拒绝）。chart 侧结构门：`in-cluster-ha` 要求
  `global.controlPlaneDomains ≥ 3`，否则渲染失败并给出 `HA_CONTROL_NODES_INSUFFICIENT`
  / `IN_CLUSTER_HA_UNCONFIGURED` / `EXTERNAL_BACKING_MISCONFIGURED` / `HA_BUNDLED_NOT_HA`；
  渲染成功 ≠ 认证，安装仍由能力证据门禁阻断。
- `hnbctl upgrade --to <profile> [-f hnb.yaml] [--allow-downgrade] [--backing bundled|in-cluster-ha|external] [--control-node-count N] [--operator-ready true|false]`：
  默认只产出 `profile-upgrade-plan/v1`（能力增启、副本、存储扩容、backing 切换、owner 顺序迁移、
  release、verify，每步标注可逆性），**无副作用**。`--backing` 覆盖目标形态默认 backing；
  `--control-node-count`/`--operator-ready` 提供观测前置（`0`/省略=未观测则跳过），
  `in-cluster-ha` 且控制域 <3 记 `HA_CONTROL_NODES_INSUFFICIENT`，operator 未就绪记
  `OPERATOR_NOT_INSTALLED`；backing 变化的路径须在发布锁声明 `backingChange: true`，否则
  `BACKING_CHANGE_NOT_SUPPORTED`。目标形态/路径未认证或缺发布列车路径时计划标记 `blocked`。
  `--execute` 在计划授权且门禁通过时渲染目标 overlay 并驱动门禁化升级执行器（run kind=`upgrade`，
  阶段 `preflight → backup → infra-delta → data-plane-rebacking → database-migration →
  platform-release → platform-verified`；`data-plane-rebacking` 仅在计划含 backings 步骤时执行）；
  `backup` 前置、不可逆迁移 `--allow-irreversible` 确认、失败关闭与 resume 不重跑一次性阶段；
  cutover 后的再背靠不可自动回滚（`ROLLBACK_REBACKING_UNSUPPORTED`）。
- `hnbctl scale [--component NAME --replicas N] [--storage NAME=Gi]... [--cpu NAME=V] [--memory NAME=V]
  [-f hnb.yaml] [--current NAME=N]... [--current-storage NAME=Gi]... [--current-cpu NAME=V]... [--current-memory NAME=V]...
  [--control-node-count N] [--operator-ready true|false]`：
  默认产出 `scale-plan/v1`。观测基线（`--current*`）使扩容相对**实时**副本/PVC/requests 规划，
  而非形态默认值。护栏：HA 关键组件不低于 quorum、不超单域共置上限、迁移/升级进行中禁止扩容、
  PVC 只增不减。in-cluster 数据面 `--component postgresql|nats` 映射到 CloudNativePG
  `Cluster.spec.instances` / `StatefulSet.spec.replicas`，最小 3 副本（`<3` → `SCALE_BELOW_QUORUM`），
  JetStream 需奇数（偶数 → `SCALE_EVEN_QUORUM`）；`bundled` 档对 pg/nats 返回
  `SCALE_UNKNOWN_COMPONENT`。前置同上（`HA_CONTROL_NODES_INSUFFICIENT`/`OPERATOR_NOT_INSTALLED`）。
  `--execute` 在已有集群路线生效：组件 `kubectl scale deployment/...`、存储扩容
  `pvc <name>-data`、垂直 `kubectl set resources deployment/...` 并等待 rollout（run kind=`scale`，
  阶段 `preflight → infra-delta → platform-verified`）。
- 发布列车 `release-lock/v2` 增量字段：`profiles`（requiredCapabilities/最小 k8s/**backings 允许集合**）、
  `supportedUpgrades`（from→to/**backingChange**）、`migrationChain`（owner 顺序 + from/to schema +
  checksum + reversible）；v1 锁仍可读，升级路径仅在声明后可用。
- 再背靠（backing 迁移）建立平台 Operation 行记录 `from`/`to`（审计可追踪）；`profiles[].backings`
  与 `supportedUpgrades[].backingChange` 未声明时对应的迁移或契约被拒绝。


## 扩展位置

```text
CLI 参数 / 输出
  └─ toolkit 用例与只读报告
       ├─ provision OSAdapter：OS 机制、目标机包选择
       └─ EvidenceMatrix：路线、内核、架构与验证报告
```

增加 OS 时先补适配器和测试，再登记独立证据；CLI 不再添加 OS 特例分支。
包管理器处理、宿主机准备与已有集群上的应用部署维持独立边界。

行业接口参考：
[kubectl](https://kubernetes.io/docs/reference/kubectl/overview/)、
[kubeadm init](https://kubernetes.io/docs/reference/setup-tools/kubeadm/kubeadm-init/)、
[Helm CLI](https://helm.sh/docs/helm/helm/)。本轮未引入新的命令框架或替换既有 CLI。
