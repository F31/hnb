## 1. P0 契约与候选交付基线

- [x] 1.1 固定现有 Ubuntu/Rocky 装机实现的候选源码身份和报告索引，区分方法级、缓存条件与完整平台认证；验证：索引/摘要校验及范围检查。（DBND-002、DBND-005）
- [x] 1.2 完成 Day-0 管理基座、既有集群安装、在线托管及故障恢复的边界决策和拒绝场景；边界不能证明则只放行已有 Kubernetes 路线；验证：ownership/503/已纳管目标契约测试设计。（PINST-001、DPLY-006）
- [x] 1.3 定义 DeploymentConfig、ReleaseLock、PreflightReport、BootstrapReceipt 与诊断 Schema；验证：合法/非法配置与不兼容 Schema fixtures。（DPLY-001、DPLY-005、DBND-001）
- [x] 1.4 盘点 owning plane 的现有公开 API、Secret 登记、兼容矩阵、Intent/Operation/观测接口，定义 CLI 适配和必要契约增量；验证：禁止任意 steps、直接 Provider/DB 写入的负向契约测试。（DPLY-006、PINST-005）
- [x] 1.5 形成 Minimal T0/T1 实际依赖清单，列出 Chart 缺失组件与每平面数据库/制品绑定；验证：组件依赖图和 render 清单逐项匹配。（PINST-002、PINST-003、DBND-004）

## 2. P1 CLI、配置和只读预检

- [x] 2.1 新建 Go hnbctl 模块与 version/help/结构化退出码框架；验证：help、未知命令、不支持动作的 CLI 测试。（DPLY-008）
- [x] 2.2 实现严格配置规范化、模式/拓扑/角色/CIDR/凭据及支持矩阵校验；验证：无效组合在任何副作用前拒绝。（DPLY-001、DBND-002）
- [x] 2.3 实现交互与非交互 init，提供已有 Kubernetes/Ubuntu/Rocky Minimal 配置模板；验证：必填字段一致、无真实凭据/假入口/示例摘要。（DPLY-001、DPLY-008）
- [x] 2.4 实现节点 OS/架构/cgroup/权限/资源/现有 owner 的只读探测；验证：预检前后节点配置和软件状态不变。（DPLY-002、PINST-001）
- [x] 2.5 实现配方驱动的 CIDR、源路径端口、API/入口/TLS、包源/镜像可达性检查；验证：SSH 通/API 不通、镜像 metadata 通/内容下载不通等差异 fixtures。（DPLY-002、PINST-003、DBND-002）
- [x] 2.6 实现 PreflightReport 人类/JSON 输出、稳定 checkId/reasonCode 与修复说明；验证：报告时间、expected/actual、secret redaction。（DPLY-002、DPLY-005、DPLY-007）

## 3. P0/P1 真实物料与平台 Chart

- [x] 3.1 实现候选发布锁生成，解析真实 OCI/Chart/CLI/迁移身份，拒绝 latest、占位和缺失项；验证：实际 registry 解析与错误锁门禁。（DBND-001）
- [x] 3.2 实现按 OS/arch 配方解析节点工具、软件包及镜像依赖清单；验证：Ubuntu/Rocky 包、pause、Cilium/operator/Envoy 均在闭包内。（DBND-001、DBND-003）
- [x] 3.3 为 Chart 添加 digest 参数/Schema，并补 Portal 交付资源；验证：helm lint/template 输出全部锁定且 Portal 服务/入口存在。（PINST-002、DBND-001）
- [x] 3.4 补 plugin-worker、runtime-target-lifecycle-provider 及其认证/路由配置；验证：渲染/容器启动配置与真实 provider profile 一致。（PINST-002、PINST-005）
- [x] 3.5 实现 PG/NATS bundled/external、文件持久化及每平面 existingSecret/独立身份；验证：Minimal 持久化、跨 owner 访问被拒绝及凭据不入 values 输出。（PINST-003、PINST-004、DBND-004）
- [x] 3.6 实现实例身份、KMS 材料、服务凭据和管理员初始化生成/接入；验证：时间窗口、公共入口/TLS、文件/Secret 权限及重放行为。（PINST-003、DPLY-007）
- [x] 3.7 实现版本锁定、owner 互斥与 checksum 校验的迁移阶段；验证：重复/并发/中断迁移、schema 不兼容阻塞平台服务。（PINST-004）
- [x] 3.8 定义四档 values overlay、requests/存储/故障域预算并生成容量报告；验证：Minimal 不启重型 T2/T3，未认证 HA 拒绝。（DBND-004、PINST-002）
- [x] 3.9 统一数据库客户端的 db.existingSecret/db.secretKey，优先 Secret 引用并保留旧 DSN 兼容；验证：全 Chart 无内联密码渲染、19 客户端 Secret 引用及 Startup 真机登录。（PINST-003、DPLY-007）
- [x] 3.10 为 Startup 核心服务加入有界依赖启动重试和独立存活/就绪探针；验证：PG/NATS 中断使 readiness 失败而 liveness 正常、服务不重启且依赖恢复后自动就绪。（PINST-004、PINST-006）

## 4. P1 安装、共享阶段与恢复

- [x] 4.1 提取共享节点阶段与证据/凭据/进度适配，保留 Provider 接口行为；验证：现有 provider/provision 回归及 SSH 并发输出 race 检查。（DPLY-003、DPLY-004、PINST-001）
- [ ] 4.2 实现 prepare 下载/校验/缓存及允许的 bootstrap preload；验证：中断重试、摘要不符、镜像源映射与真实进度。（DPLY-003、DBND-001）
  - 2026-10-14 完成本地准备增量：接通显式 mirror origin 的专用校验（其他安装入口仍拒绝该模式），无映射不回退；失败/取消输出绑定发布锁的已验证部分结果，进度含时间、实际来源、重试字节与原因。CLI/工具包 race 回归通过，真实 Docker Hub 冷下载 4 对象/3,813,273 字节及热缓存全命中通过。
  - 2026-10-14（第二批）接通授权 bootstrap preload：prepare 已下载并校验全部锁定镜像 blob（manifest/config/layers）入内容寻址缓存；新增 `ImagePreloader` 从缓存重组 OCI layout（出站逐 blob 复验摘要/大小）并经新增 SSH `Stream` 原语流式导入空白节点 containerd（guard 标记 `preparing` + claim/epoch 门禁，节点侧 ctr import，临时文件 trap 清理）。仅由 bootstrap 适配器在 node-prereq 成功后调用（run 状态记 PhasePreparing），失败尽力而为不阻断在线拉取且可恢复重试；resume 不重跑已成功的 preload。`prepare --preload` 保持拒绝（授权仅在 bootstrap run 内，install 执行仍由 4.10 门禁）。
  - 保持未完成：真实 registry 大 blob 冷/热缓存与节点侧 ctr 导入需 5.4/5.5 真机验证；`--preload` CLI 接通随 4.10。证据见 `docs/acceptance/reports/20261014-prepare-download-verification.md`。
- [x] 4.3 实现已有 Kubernetes 的 HNB 初次安装依赖阶段和 ownership 清单；验证：保留外部 CNI/owner，不创建节点初始化动作。（PINST-001、PINST-002、PINST-007）
- [x] 4.4 在 1.2 边界通过后实现专用空白管理基座 bootstrap 适配；验证：配置已有 platform/target、发现管理标记、节点版本冲突时拒绝。（PINST-001、DPLY-004）
  - 2026-10-14 完成 `ManagementBootstrap` 适配器（仅收空白管理节点：online、单控制面、无 platform/target 引用、无管理/生命周期标记、无 kubeadm 版本冲突；mirror/managed/existing/多节点/worker 路由一律 `BOOTSTRAP_BOUNDARY_REFUSED`/`BOOTSTRAP_ROUTE_UNSUPPORTED`）。身份探测绑定 machine-id + 永久 owner claim（sha256 of run id/config/lock digest），外部 claim 拒绝（`BOOTSTRAP_OWNER_PRESENT`）、resume 时集群 UID 变更拒绝（`BOOTSTRAP_CLUSTER_CHANGED`）、锁缺节点镜像拒绝（`BOOTSTRAP_LOCK_INVALID`）。kubeconfig 以 0600 附件写入 run 目录；run 不 finalize（平台未验证）。修复 `LoadRunState` 缺失态错误包装（`%w`）使全新 bootstrap 不再误报 `BOOTSTRAP_STATE_UNSAFE`。
  - 验证：`boundary_contract_test.go` 3 项契约 + `management_bootstrap_test.go`（完整运行+resume 仅 recheck 不重跑 init、失败阶段单独重试、身份/边界/锁拒绝矩阵）通过，`cmd/hnbctl` 全量与 `pkg/provision` 回归通过。真机 Day-0/恢复 E2E 归入 5.4/5.5；install 实际执行仍由 4.10 门禁。
- [x] 4.5 实现权限受限 receipt、阶段前后置检查、single-writer/fencing 和恢复；验证：断点恢复不重复 init、不自动 reset、改锁拒绝、第二 writer 拒绝。（DPLY-004、PINST-001）
- [x] 4.11 加固运行存储完整性：损坏状态不覆盖、完整运行身份（run id/路由/ownership）变更拒绝、状态文件/目录 0600/0700 与 symlink 拒绝、锁内原子写、租约过期必换 fencing 纪元、完成后终态防写、失败/阻塞的一次性阶段恢复为 recheck_or_refuse、阶段超时覆盖前后置检查、层判定不被后续成功掩盖先前失败或缺失；验证：并发写者仅一个成功、恢复/重放/损坏/泄漏场景回归。（DPLY-004、PINST-001、DPLY-005）
  - 2026-10-13 闭合先前记录的实现与文档差距：租约在**到期瞬间**即失效（`!Before`，边界失败关闭）；`AcquireWriter`/`Finalize` 同样调用终态断言，成功后的运行无法再取租约或二次 finalize；目录必须恰为 0700、状态与锁文件恰为 0600（不再接受更宽松模式）；锁以 `O_NOFOLLOW` 打开并 `Lstat` 预检，拒绝符号链接；状态写入改为私有临时文件 + `fsync` 文件 + rename + `fsync` 目录。
  - 继续闭合：未识别的阶段状态（空值或新版写入）不再被计划为 `execute`，改为 `recheck_or_refuse` 并说明原因；run 锁改为有界等待（30s，`LOCK_NB` 轮询），超时返回 `RUN_WRITER_BUSY` 而非无限阻塞。新增 `run_store_hardening_test.go`、`run_resume_safety_test.go` 共 7 项测试通过。
  - 跨进程与掉电 durability 仍需真机验证（主机停机中）。
- [x] 4.6 实现 status/resume、分阶段 timeout 与 layered 成功/失败输出；验证：集群 Ready/平台失败时不能输出完整交付成功。（DPLY-005、DPLY-008）
  - 2026-10-15 增加安装进度呈现、安装历史与转换日志、简洁卸载入口：`status --watch` 渲染记录到的阶段进度（既有集群路线不展示 init/join/runtime 等节点阶段），`status`/`logs`/`uninstall` 默认取最近一次记录，新增 `history`、`logs`（`--tail`/`--follow`）；状态与转换日志在同一次原子写入中提交（`install-event/v1`），无转换的重复写不产生重复日志；`install` 落盘 run 记录，仅规划未授权执行记为 `blocked`；`uninstall` 必须绑定记录在案的 run，配置身份不一致直接拒绝，删除自有持久卷需 `--acknowledge-storage-removal`，删除仍走服务端权威契约。既有 `platform uninstall-hnb|unmanage|delete-management-base` 行为不变。
- [x] 4.7 实现平台正常时基于正式 Intent/Operation 的 CLI create/import 和状态查询；验证：同义请求幂等、跨租户 Secret 拒绝、503 不回退 SSH。（DPLY-006、PINST-005）
  - 2026-10-15 接通 `cluster list`：读取平台 API 服务端权威 runtime-target 列表（`GET /v1/targets`，`cluster-list/v1`），总数与行数不一致按不可读拒绝，缺凭据/5xx 本地拒绝、无本地视图回退。
- [x] 4.8 实现初次管理关系 handoff，receipt 仅作为附件关联；验证：真实 Agent 认证/观测与正式注册 Operation，禁止直接写 Read Model。（PINST-005、DPLY-006）
- [x] 4.9 接通 hnbctl render 的锁定 Chart/overlay/amd64 镜像与 Secret/namespace 绑定，输出可重放；修正基础设施先于迁移的安装顺序；验证：真实 Helm 渲染、改包/跨 namespace/内联 DSN 拒绝、同路径重放与真机应用。（DBND-001、PINST-003、PINST-004）
- [ ] 4.10 在完整档位 E2E 和发布物料条件满足后接通 install 的实际执行，验证干净环境首次安装与阶段恢复；当前 render 只完成候选物料生成，不开放未经认证的 Minimal 执行。（PINST-002、PINST-005、DBND-005）
  - 2026-10-15 增加极简参数入口：`install --host|--kubeconfig` 复用配置/发布锁验证，保存 0600 配置与锁快照，默认执行已有只读预检；`--plan` 仅生成计划。`uninstall --installation` 从摘要绑定的快照恢复上下文，默认保留数据/命名空间，`--delete-data` 明确确认后才纳入删除计划。计划均不授权实际执行，主机计划不构成基座归属证明；实际安装、安装变更确认和实际删除仍未接通，本项维持未完成。
  - 2026-10-15 命令对齐与可用性：新增 kk 风格动词 `create config`/`add nodes`/`delete installation|cluster`/`upgrade`/`artifact`（转发到既有 handler，不改变行为）与 `completion bash|zsh|fish`；`init --from-kubeconfig` 从 kubeconfig 反推 existing 草稿（只解析、不联网）；`logs --phase/--reason/--after`、`history --state` 过滤；多节点只读预检有界并发（默认 8，按声明顺序汇总）；`node list|status` 在平台 API 无契约时诚实拒绝。
  - 2026-10-15 执行入口：`install --execute` 显式进入执行路径，强制 `EvidenceGate`；当前证据未认证，稳定返回 `PLATFORM_NOT_VERIFIED(4)` 列出原因、不接触节点、不记录成功（`--plan` 与 `--execute` 互斥）。
  - 2026-10-16 已有集群路线接通：证据认证后，`install --execute --chart-package ... --bindings ... [--workload Kind/name]... [--migrations-dir DIR]` 会渲染锁定 Chart、`kubectl apply` 并等待 rollout，成功时 run 终结为 `succeeded`、`executionAuthorized=true`；rollout 收敛失败则 fail-closed（`platform-verified=failed`、run `blocked`、退出 4）。已在云主机 K3s 上以 fixture Chart 真实验证 apply/rollout/finalise 与 uninstall 决策矩阵（`docs/acceptance/reports/20261016-existing-minimal-hnbctl-execute.md`，登记 `hnbctl_install_execute_uninstall`），并以进程内 `helm`/`kubectl` 夹具测试 `TestShortcutExecuteAppliesCertifiedExistingRoute` 复现。**未完成部分**：真实发布物料（打包 Chart + 含镜像 manifest digest 的 ReleaseLock）缺失，故未对 19 工作负载平台做整平台 `--execute`；干净环境首次安装与阶段恢复（`upgrade`/resume 深度集成）仍未验证，本项维持未完成。（PINST-002、PINST-005、DBND-005）
  - 2026-10-15 发布 applier：新增 `toolkit.PlatformApplier`（`kubectl apply` + `rollout status`）与门禁化管线 `toolkit.RunPlatformInstall`，`install --execute` 对已有集群路线在门禁通过且提供 `--chart-package`/`--bindings` 时渲染并应用；管线不执行数据库迁移，故应用成功仍非完整交付。已用假 `kubectl` 与合成已认证证据单测门禁拒绝、应用+验证、rollout 失败关闭。真实执行仍阻塞于交付证据认证与迁移。
  - 2026-10-15 bootstrap 接入：新增门禁化管线 `toolkit.RunManagementBootstrap`，`install --execute` 对空白主机路线（`--host`）在门禁通过时运行 `ManagementBootstrap`（新增 `--image-repository`/`--sandbox-image`/`--cni-image-repository`/`--chart-repository`），由适配器自持 run 目录；门禁未认证时拒绝且不创建 run、不接触节点。已单测门禁拒绝（无节点调用）与合成已认证证据下的引导。仍不执行平台迁移。
  - 2026-10-15 数据库迁移：新增 `toolkit.RunDatabaseMigrations`/`RunInstallMigrations`（门禁化，按 owner 顺序、失败关闭、返回部分结果并记录 `database-migration` 阶段）与 `toolkit.ReadSecretKey`（`kubectl` 读 Secret 键并 base64 解码，错误按类别返回、不泄露值）；`install --execute --migrations-dir` 在应用后逐平面读取 DSN 并应用迁移，DSN 仅用于连接、不打印留存。已单测：顺序/失败关闭/无 DSN 泄露、输入校验先于连接、门禁拒绝且不调用 applier、已认证记录阶段、Secret 解码与失败关闭。真实执行仍阻塞于交付证据认证。

## 5. P1 验收、诊断与首个可交付包

- [x] 5.1 实现 node/cluster/platform/managed 的 verify 检查集及自有测试资源清理；验证：登录、PG/JetStream、Provider、最小真实应用 Operation、Agent/Read Model 各层失败门。（PINST-005、DPLY-005）
  - 2026-11-08 S4 候选包 0.1.1 干净 kind（`hnb-s4-clean`）安装冒烟 19/19 通过（run `2fb558ed0c9b6e5410a5709827c161a0` succeeded）；最小真实应用 Operation（demo-s3 deploy `f31e0b8b…` / delete `44b55bcb…`）均 succeeded，Agent/Read Model 链路可用。证据 `docs/acceptance/reports/20261108-s4-candidate-0.1.1-kind-install.md`。
- [x] 5.2 实现脱敏诊断 collect 与报告输出身份；验证：密码/私钥/kubeconfig/token/DSN/客户数据 fixtures 不出包。（DPLY-007、DBND-005）
- [x] 5.3 在独立已有 Kubernetes 环境完成 Minimal 平台 E2E；保留命令、源码/物料、身份/入口、存储与完整业务闭环报告。（PINST-002、PINST-005、DBND-005）
  - 2026-10-16 在已有单节点 K3s（Ubuntu 26.04 / kernel 7.0.0-30-generic / amd64）完成 Minimal 平台 full-delivery E2E：19/19 工作负载就绪、三平面迁移账本各 103、portal 登录/读 API/agent 隧道连通、匿名 401、PVC 2Gi/1Gi 于 `hnb-topolvm`，并对 PG/NATS/Deployment 滚动重启后数据身份与账本保持。报告 `docs/acceptance/reports/20261016-existing-minimal-e2e.md`，结果 `...-result.json`，登记 `existing_minimal_platform_delivery`。据此把 `Profiles[Minimal]`、`Routes[existing]`、`OSAdaptations[ubuntu-26.04/existing/k3s]` 置为 certified；`CertifiedOS` 不含 ubuntu-26.04，`Routes[bootstrap]` 与 HA/Enterprise/offline 维持 blocked。（PINST-002、PINST-005、DBND-005）
  - 2026-10-16 追加 `hnbctl` 端到端：`install --execute`（渲染+真实 `kubectl apply`+rollout，run succeeded）、受控 `upgrade --to`（run kind=upgrade succeeded）、`scale`（水平/垂直/存储，run kind=scale succeeded），报告 `docs/acceptance/reports/20261016-existing-minimal-hnbctl-execute.md` 与 `...-profile-upgrade-rehearsal.md`，登记 `hnbctl_install_execute_uninstall`、`profile_upgrade_and_scaling`。
  - 2026-11-08 S4 正式候选包 0.1.1 在干净 kind 集群 `hnb-s4-clean` 完成独立安装与业务闭环：锁 digest `sha256:2589866a…`、run `2fb558ed0c9b6e5410a5709827c161a0` succeeded、19/19 冒烟；lifecycle provider 接线后 S3 应用交付实机 E2E 闭环（deploy/delete Operation 均 succeeded、Service/HTTPRoute 经 provider 删除、readiness verification 自动 upsert 关联 Operation 终态）。新登记 10 项安装缺口（含已修复的 admin 种子绑定剥离 approve/reject/cancel 代码 bug）。报告 `docs/acceptance/reports/20261108-s4-candidate-0.1.1-kind-install.md`。
- [x] 5.8 实现 provider 动作级签名 conformance 报告与受控原子登记，绑定实际镜像/manifest/矩阵/期限；验证：无权限、未受信签名、篡改、缺类、失败、过期、错物料与并发换 manifest 均拒绝。（PINST-005、DBND-001、DBND-005）
  - 已完成：签名报告校验、公开信任挂载、事务内加锁原子登记、独立 PostgreSQL 集成测试与真机负向检查；期间修复 manifest upsert、NOT NULL 数组与 404 映射三处真实缺陷。
  - 2026-11-08 在 S4 候选 0.1.1 真实安装上完成端到端：Ed25519 信任锚 Secret 挂载、manifest 注册 201（identity digest `sha256:e169c096…`）、签名 conformance 上报 201（RunID `s4-import-conformance-20261108`），`provider_manifests.conformance_level=production_ready`（有效期至 2026-11-12）。证据 `docs/acceptance/reports/20261108-s4-candidate-0.1.1-kind-install.md`。
- [x] 5.9 在隔离测试环境运行 Kubernetes import conformance，正式登记后绑定 worker 并经 Intent 验证真实 Operation/幂等/失败语义；保留独立报告，不提升完整 Minimal/HA 资格。（PINST-005、DPLY-006、DBND-005）
  - 已完成：隔离环境真实 provider 镜像通过六类断言并登记 import cell；Chart 支持 owner-only 令牌暂存；真机 Operation `d423131c` 到达 `succeeded`，同键重放返回同一 Operation、同键异体 409、异键只执行一次。
  - 阻塞：服务凭据签发/轮换未实现（60 秒上限 + 暂存副本仅随重启更新）；导入目标仍为 REGISTERING，尚无 Agent 认证与 Read Model 闭环。
- [x] 5.10 实现 worker 孤立步骤重驱动：丢失的步骤请求按预算重新投递，重复投递不产生重复执行，超预算显式告警；真机验证并保留证据。（PINST-006、DBND-005）
  - 真机已验证：sweeper 部署后确实重驱动了昨日孤立的步骤，预算封顶生效并显式告警；发现并修复三个真实缺陷——空闲阈值毫秒/秒单位错误、重投递复制行破坏 envelope/payload 耦合（改为重新武装既有行）、平台接受 128 字符客户端键却生成无法投递的步骤键。
  - 已补回归：worker 自身的 `validateStepRequest` 纳入测试，重新武装可通过、派生键副本被拒、129 字符键被拒——这正是原设计漏测的缺口。
  - 2026-10-14 完成：SSH 断开定位为本地代理隧道路由；发现部署脚本把 v2 错绑到 v3 摘要名，改为校验实际 OCI 内容与运行时 imageID。v4 `sha256:c1fb52bd0f206018dbe081ca4f00ddc30fd09ab6e9c96ff83e6078016af8bf7a` 真正部署；修复 canonical 请求选择、未发布请求排除和 outbox 预算边界，并对齐当前 Provider manifest 绑定。
  - Operation `36d171ec-5721-41fd-a35e-4e33345881e6` 注入 UPDATE 1 后重驱动到 succeeded（1/1，单行 attempt=1）；超过 broker 去重窗口后重复投递被 worker 确认，执行时间与步骤结果不变；`659bffac` 在 attempt=3/max_retries=3 保持 pending 并告警。四个旧测试 Operation 的 12 条派生请求先归档后清理，原始请求及失败终态保留。
  - 证据：`docs/acceptance/reports/20261014-redrive-real-host-verification.md`。
- [ ] 5.4 在满足完整平台预算的独立 Ubuntu 环境完成冷缓存 bootstrap/恢复测试；单独记录空 OS、工具预置和镜像准备条件，不复用失败环境冒充初次成功。（PINST-001、DPLY-003、DBND-002、DBND-005）
- [ ] 5.5 在独立 Rocky 环境完成平台 bootstrap/worker/网络矩阵测试，记录精简内核与 OS 配方约束。（PINST-001、DBND-002、DBND-005）
- [ ] 5.6 执行平台节点/进程重启持久化和一条 PG/消息/身份材料备份恢复演练；验证实际批准的 RTO/RPO。（PINST-006、DBND-004、DBND-005）
  - 2026-10-16 在云主机完成部分演练：PG `hnb_platform`（130 表）`pg_dump`→恢复到 scratch 库→130 表一致→清理（非破坏）；身份材料 6 文件（master.key/admin-bootstrap.password/private.pem/public.pem/manifest.json/identity-state.json）打包→解包→逐文件 sha256 一致；进程/节点滚动重启持久化见 existing-Minimal E2E。报告 `docs/acceptance/reports/20261016-backup-restore-drill.md`，登记 `existing_minimal_backup_restore`。**未勾选**：JetStream 消息面备份恢复与经批准的 RTO/RPO 实测仍待补。（PINST-006、DBND-004、DBND-005）
- [ ] 5.7 生成首个可安装 release lock、版本包、操作卡与已支持能力矩阵；验证：无占位摘要、缺服务或未通过必需层。（DBND-001、DBND-002、DPLY-008）

## 6. P2 Portal、节点生命周期与运维交付

- [ ] 6.1 将 Portal 安装向导对齐相同配置/兼容矩阵/预检契约，展示节点-集群-平台真实进度；验证：UI/CLI 的不支持行为一致。（DPLY-001、DPLY-005、DPLY-008）
- [x] 6.2 为 node add 和平台升级先补对应动作的服务端契约/delta、能力矩阵及执行路由，复用既有 Operation 幂等/fencing；验证：未支持动作拒绝、禁止客户端 providerId/steps。（DPLY-006、DPLY-008）
  - 2026-10-14 完成：升级契约核实已存在（kind/matrix REQUIRED/步骤/ provider Apply，5.9 已登记）；node add 新增 `AddNodesRuntimeTarget` 全链路契约——schemas（intent kind、matrix actions、plan action 枚举）、引擎校验（`validateNodeAdditions`：仅 nodes 参数、worker 角色、RFC-1123 名、拒绝重复/额外字段/客户端 steps、providerId）、能力矩阵 `node-add: UNSUPPORTED`（诚实基线，6.3 conformance 后翻 REQUIRED）、plan 步骤工厂（`runtime_target.kubernetes.add-nodes`、monotonic-worker-lease-v2、step 幂等键、1800s）、IAM→ActionUpdate、BFF 放行+轻校验。合同门禁（validate-contracts+drift+oasdiff）与 contracts.test 25/25 通过；platform-api/apiserver/hnbctl/iam 回归通过；未支持动作与客户端执行字段负向测试落盘。证据见 `evidence/task-6.2-node-add-contract.md`。
- [ ] 6.3 实现节点增加 CLI 与限定目标身份的执行步骤；验证：已入集群节点重放、节点准备失败与观察后验收，不重复 init。（DPLY-004、DPLY-006、DBND-002）
  - 2026-10-14 部分交付：Provider 接通 SSH node-add、namespace 端点/凭据引用持久化与重启恢复；CLI 接通 `AddNodesRuntimeTarget` 正式提交，携带 `sshSecretRef` 和必需的 `expectedVersion`。声明严格拒绝未知字段/多文档、非 worker、非法名及重复节点。hnbctl 全量测试/vet 与 lifecycle-provider 全量测试通过；范围及命令见 `evidence/task-6.3-node-add-progress.md`。
  - 保持未完成：正式 CLI/Intent/Provider 链路的 node-add 验收与实际准备失败恢复尚缺；签名 conformance 登记当前仅支持 import，node-add 动作资格尚未登记，矩阵仍为 UNSUPPORTED。共享安装器的 Ubuntu 真实加入/重放/Ready 验收见后续记录。
  - 连续扩容回归发现并修复：真实 node-add 不含 desiredVersion，首次执行后内存目标版本被清空且 kubeconfigRef 丢失；现继承已有目标身份。新增连续两次扩容、精确重放不重新 join、namespace 版本/引用/fencing 保留测试，Provider 全量测试与 vet 通过。
  - 新 worker `106.75.10.89` 已通过本地 SOCKS5 登录并完成只读资源检查；内网 `10.9.36.43` 到两个历史 kubeadm API 地址均返回 EHOSTUNREACH，历史平台主机 SSH banner 超时。尚未安装或 join，等待可达目标及正式平台身份。证据见 `evidence/task-6.3-worker-preflight.md`。
  - 后续按用户要求将 `106.75.10.89` 改作单控制面，安装 Kubernetes 1.36.5/Cilium 1.20.1，API/Node/CNI/CoreDNS 就绪通过。原始软件下载停滞后切换带官方签名校验的 APT mirror 恢复；本次为独立集群安装而非平台纳管，仍需另一台 worker 和正式 node-add 资格。见 `docs/acceptance/reports/20261014-ubuntu-control-plane-install.md`。
  - 新 worker `106.75.15.246`（内网 `10.9.57.29`）经共享 AddNodes 加入成功，首次 Joined=1、重放 AlreadyJoined=1，控制面 UID/CA/admin.conf mtime 不变；Envoy 初次拉取 TLS 超时后自动恢复，最终 CNI 2/2、双向跨节点 HTTP/Service/DNS 通过，专用测试 namespace 已清理。采用显式 Ubuntu SSH 路由与 runtime 准备适配，不能提升为正式平台 conformance。见 `docs/acceptance/reports/20261014-ubuntu-worker-join.md`。
- [x] 6.4 实现平台版本 diff、升级及资源回滚流程；验证：计划固定版本、强制 DB 兼容/备份检查、API 不可用时拒绝通用目标执行。（PINST-006、DPLY-006）
- [x] 6.5 实现数据库前向恢复/备份恢复操作卡及现场管理基座恢复工具；验证：仅处理绑定自有管理资源，不覆写 Operation 历史。（PINST-006、DPLY-007）
- [x] 6.6 实现卸载/解除纳管/删除自建管理基座的独立入口；验证：共享 PG/原 Kubernetes/客户 PVC/业务工作负载均按 ownership 保留。（PINST-007、DPLY-006）
  - 2026-10-17：`hnbctl uninstall --execute --yes` 已能在 existing cluster 本地删除 HNB 工作负载与带 `instance=hnb` 的 ClusterRole/Binding；默认保留 PVC/Secret/Namespace/StorageClass/TopoLVM，`unmanage`/`delete-management-base` 仍走服务端契约；云主机 43.142.101.136 实测 `verified=true`。见 `docs/acceptance/reports/20261017-hnbctl-uninstall-execute.md`。
  - 2026-10-17：进一步提供零配置 `hnbctl uninstall`/`-all`/`-all -data`；自动检测 kubeconfig 与 HNB/TopoLVM 残留，`-all` 删除 TopoLVM/SC/CSIDriver/CRD，`-data` 删除 PVC/PV/LV/Secret/namespace。云主机实测 `-all -data` 后仅剩 K3s 系统组件，`verified=true`。见 `docs/acceptance/reports/20261017-hnbctl-auto-uninstall.md`。

## 7. P3 离线与生产档位认证

- [x] 7.1 实现 bundle prepare/verify/import，加入 CLI、Chart、OCI、OS 包/依赖、工具、迁移和发布身份；验证：删去任一必须物料即校验失败。（DBND-001、DBND-003）
- [ ] 7.2 在禁止公网的 Ubuntu 环境执行安装、节点重启及持久性验收；验证：没有公网 fallback，记录本地依赖与缓存条件。（DBND-003、DBND-005）
- [ ] 7.3 在禁止公网的 Rocky 环境执行相同离线闭包/安装验收；验证：该 OS 独立报告，不从 Ubuntu 推导。（DBND-002、DBND-003、DBND-005）
- [ ] 7.4 为 Lite HA 完成 Kubernetes/PG/JetStream/入口单节点和 leader 故障场景，认证足够独立故障域后开放档位。（DBND-004、PINST-006）
- [ ] 7.5 分别评估 Standard HA/Enterprise 容量与恢复门，未通过者保持 blocked；新增架构/RKE2/KubeKey 单独提交后端 Conformance 范围。（DBND-002、DBND-004、DPLY-008）
- [x] 7.6 盘点现有制品 owning-plane 接口并接入统一发布物料目录，将控制台/管理平台、控制面与 worker 所需物料及离线副本关联到同一 ReleaseLock/摘要/依赖/OS/架构/角色；验证：重复导入幂等、摘要不符及角色依赖缺失拒绝，不静默替换固定发布。（DBND-001、DBND-003、DBND-006）
  - 安装器侧：`release_catalog.go` 提供 `ValidateRoleClosure`（console/control-plane/worker 三种安装角色关联同一 ReleaseLock 与离线 bundle，按角色校验 sandbox-image/os-package-index/os-package/chart/migration/镜像布局/cli 及 OS/arch 依赖闭包）与本地 `ReleaseImportCatalog`（重复导入幂等、摘要不符拒绝、同发布同角色异摘要拒绝静默替换）。
  - owning-plane 侧：新增迁移 104 `release_catalog_entries`（role 白名单 + sha256 摘要 CHECK + `UNIQUE(release_id,release_version,role)`，已在真实 PG16 验证约束与回放）；`PGStore.RegisterReleaseMaterial/ListReleaseMaterials`（行锁内决策：同摘要幂等、异摘要 `ErrFixedReleaseConflict`）；`POST/GET /v1/releases`（route+handler+auth，role 枚举/摘要/必填严格校验）。`mysqlStubStore` 仅占位返回 not-implemented，不启用任何数据路径。
  - 验证自动测试：toolkit 角色闭包/目录测试、owning-plane API 测试（201/幂等 200/409/400/403/未知字段）、store PG 集成测试（真实 PostgreSQL 16 独立 schema，幂等、固定发布冲突、列表一致）。CLI/控制台的统一目录展示与平台安装前本地清单验证属于 7.7。
- [ ] 7.7 接通 CLI 与控制台的统一发布目录及离线包完整性展示，保持平台安装前本地清单验证可用；验证：两入口版本/摘要/依赖判定一致、无平台时可验证、禁公网时不 fallback，包齐备与 OS/拓扑认证分开展示。（DBND-002、DBND-006、DPLY-008）
  - CLI 侧已完成：`releases_client.go`（toolkit）提供 `RegisterReleaseMaterial`/`ListReleaseMaterials`（POST/GET `/api/v1/releases`，404/409/5xx 分类；CLI `release register|list|verify`——register 先本地 `VerifyOfflineBundle`+`ValidateRoleClosure` 再登记，list 读取 owning-plane 目录，verify 纯离线且拒绝平台参数（禁公网回退）。验证：离线 verify 成功、verify 拒绝平台参数、register 创建/幂等重放/固定发布 409、list 目录解码、无效角色本地拒绝。
  - 未完成部分：控制台（前端）对同一目录的一致性展示与"包齐备 vs OS/拓扑认证"的并列展示尚未实现；双入口在真实断网环境中以同一发布包核对仍缺。控制台接入完成前 7.7 不勾选。

## 8. 文档、完整性与归档

- [ ] 8.1 提供 docs/deployment 实施手册、两路线操作卡、端口/物料/资源表、恢复与卸载手册；验证：由非开发实施人员仅依据发布包文档完成已有 Kubernetes Minimal 安装。（DPLY-007、DPLY-008、PINST-002）
- [ ] 8.2 更新每 OS/拓扑/档位验收索引，所有任务附对应 Requirement 的真实自动测试或演练证据；blocked/失败记录保留。（DBND-002、DBND-005）
  - 已完成索引：`docs/acceptance/README.md` 新增 OS × 拓扑 × 档位表，仅列有留存报告的环境，并单列"未验收组合（不得推导）"与"跨主机/跨进程未验证项"。已用脚本校验 README 与 registry 中引用的报告文件全部存在。
- [ ] 8.3 完成 CLI/配置单元、API/身份负向契约、实际集成、provider Conformance、完整 E2E、回滚/恢复检查，并执行 `openspec validate --all --strict` 和仓库统一规格门禁；verify 后再 archive。（DPLY-001、DPLY-006、PINST-005、PINST-006、DBND-005）

任务粒度目标为单项不超过一个工作日；E2E 依赖独立环境及已确认资源预算，环境不足时保留 blocked，不以较低层测试替代。

Schema/API：适用，见 1.3/1.4/2.2/6.2。数据库新表：初始 CLI 配置/receipt 为 N/A，因为在线事实复用 owning plane 的既有 Operation/审计；如 1.4 发现确需字段则补 owner migration/回滚任务，不能在 CLI 新建事实数据库。安装现有数据库迁移：适用，见 3.7。新中间件：N/A，复用 PostgreSQL/NATS/OCI。主规格合并：实现验收后由 archive 完成，当前只生成 change delta。
