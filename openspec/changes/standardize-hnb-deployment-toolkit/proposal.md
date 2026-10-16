## Why

HNB 已在 Rocky Linux 10.1 与 Ubuntu 24.04 验证部分集群装机路径，但现场仍依赖手工准备 Helm、配置身份材料、启动中间件和逐项排障，且集群 Ready 不代表 HNB 平台交付完成。需要借鉴 KubeKey 的配置驱动和 Rancher 的平台/集群分层，形成实施人员可重复执行、可恢复、可验收的标准交付工具。

## What Changes

- Change ID：`standardize-hnb-deployment-toolkit`。主能力 T1：统一 Go CLI `hnbctl`、配置 Schema、预检、安装、恢复、验收及诊断包；对 T0 Operation/身份/审计边界的复用为必需依赖；离线交付为 T2，新增 RKE2/KubeKey 执行后端为 T3 Conformance 候选。
- 提供两条标准路径：已有 Kubernetes 安装 HNB；在专用空白管理节点上 bootstrap Kubernetes + HNB。单机试用映射 Minimal，Lite HA/Standard HA/Enterprise 按各档证据开放，未通过验收不得展示为可安装。
- 复用现有 kubeadm/Cilium 执行器，提取共享阶段实现；CLI 和 Portal 使用同一配置规范、兼容矩阵及结果模型。平台在线时，创建/扩容/升级/纳管集群只能提交 RuntimeIntent，经既有 ExecutionPlan/Operation 执行。
- 定义 `DeploymentConfig`、release lock、只读 PreflightReport、bootstrap receipt 和交付报告；每阶段显示真实进度、原因码、重试建议与保留输出。第一次安装 HNB 自身的 bootstrap 只作用于明确指定的、尚未纳管的管理基座，禁止把它作为业务 RuntimeTarget 写入旁路。
- 完善已有 Helm 交付面：完整 T0/T1 服务与 Portal、按平面隔离的数据连接、数据库迁移步骤、身份/KMS/NATS 配置、入口/TLS、存储、Provider readiness。成功门覆盖登录、一次真实 Operation 和 Agent 观测，不止 Pod Running。
- 提供版本固定的在线/镜像加速包和断网包；镜像、Chart、CLI、OS 软件包、工具及迁移均内容寻址。离线包不完整或无断网验收记录时保持 blocked。
- 根据后续交付要求，控制台/管理平台及 Kubernetes 控制面、worker 节点安装所需制品均须提供离线版本；在线源与离线包复用统一的发布物料目录、版本、摘要、依赖及兼容范围，由现有制品能力管理，CLI 与控制台展示同一目录和包完整性状态。
- 标准化备份、升级、恢复、卸载的边界；卸载只处理交付记录中的自有资源，数据库回退不能由 Helm rollback 代替。
- 交付实施手册、配置样例、端口矩阵、现场操作卡和 Ubuntu/Rocky 验收矩阵。文档中的新命令在实现发布前明确标为规划接口。

## Capabilities

### New Capabilities

- `deployment-toolkit`：统一 CLI/配置、只读预检、兼容选择、阶段进度、恢复、在线集群操作和诊断导出。
- `platform-installation`：HNB 管理基座 bootstrap、Helm 平台安装、身份/存储/迁移、业务就绪门与生命周期交付。
- `deployment-bundle`：真实 release lock、在线物料准备、离线依赖闭包、安装档位和操作系统证据门禁。

### Modified Capabilities

无主规格行为削弱。新增能力落实 `KERNEL-002`、`P1-WRITE-001..005`、`RT-002/004/008/009/010`、`GOV-002/003/008`、`CFG-002`；bootstrap receipt 不是 RuntimeTarget Operation 的替代事实源，无法证明管理基座尚未纳管时不得执行 bootstrap。

## Impact

- 影响平面：平台运行治理/内核交付、市场服务安装、制品交付；AI/Edge 默认关闭，按现有可选 Pack 安装。CLI 不共享各平面数据库、不代理大文件、不在托管模式直接调用 Provider。
- 受影响规格：新增上述三个能力规格，引用既有 platform-kernel、runtime-target、deployment-governance、config-secret、security-supply-chain、observability-dr 契约。
- 代码落点：拟新增 `cmd/hnbctl`、共享安装阶段包及 `deploy/toolkit`；改进 `deploy/charts/hnb`、`deploy/docker-compose`、`deploy/bom`、集群生命周期 Provider、Portal 安装向导与 `docs/deployment`。
- 依赖 change：无活动依赖 change；依赖当前 kubeadm/Cilium、ubuntu sudo/APT 适配和 SecretReference 修复的未提交工作树先进入可追溯版本。完整北向生命周期/Agent 链路验收是发布前置条件，既有 provider 方法级报告不替代它。
- 新中间件：无；复用 PostgreSQL、NATS JetStream、OCI/现有制品能力和 Helm。各平面通过独立数据库/账户及公开契约连接，不能用一个全局 DSN 抹掉边界。
- 兼容/迁移：初期新增交付配置 Schema，不改写已有 ProvisionSpec 或主规格。需要新持久化字段时复用 owning plane 的 Operation/审计体系并单独评审 forward-only 迁移；receipt 关联不得覆盖历史事实。
- 安全与供应链：主机/平台凭据使用受限本地文件或 SecretReference；计划和日志脱敏；身份/KMS/管理员初始化材料由工具生成或接入 existingSecret，生产不使用示例密码、latest 或占位摘要。
- 资源预算：2 vCPU/4 GiB 仅有 Kubernetes 层实验依据。完整 Minimal 平台按渲染 requests 与至少 10%-30% 余量计算；资源校准后 requests 为 1900m CPU/2752 MiB/30 GiB PVC，startup 档（外部缓存/备份承载）约需 3 vCPU/6 GiB/50 GiB 节点，正式门限由实际压测确定；2C4G 需服务合并后重新评估，HA 容量与故障域单独验收。
- 可观测：runId、configDigest、lockDigest、阶段/节点耗时、下载与就绪进度、原因码、operationId/receiptId，以及脱敏诊断包。
- 回滚：保留既有入口和已固定版本；CLI 新配置仅影响显式选择的安装。Chart 回滚、DB 前向恢复和宿主机资源清理分别执行；移除工具不删除已运行集群或共享存储。
- 非目标：首期不替换 Kubernetes 发行版、不宣称支持未经验收的 HA/离线/升级，不在运行时自动切换 lifecycle owner。
- 退出判据：实施人员从发布包及一个配置文件完成一条完整 HNB Minimal 交付路线；预检不产生副作用；故障恢复不重复初始化/提交 Operation；登录、Operation、Agent、数据持久化、备份恢复演练通过，并留存冷/热缓存条件、源码/物料身份和退出码。
