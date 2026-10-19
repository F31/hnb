# S4 实施操作卡（草案，未由独立实施者验证）

> 状态：draft。当前没有正式 release lock/Chart 包，本文不能作为已认证安装手册。
> 5.2 只完成操作卡结构；5.7 独立实施验收仍 blocked。

## 0. 适用范围与前置

- 路线：已有 Kubernetes，不负责 bootstrap/离线安装。
- 管理集群：Ubuntu 24.04、amd64、K3s `v1.36.5+k3s1`、4C/8GiB、20GiB 根盘。
- 业务目标：独立 K3s，2 节点合计 4C/8GiB；TopoLVM `hnb-topolvm`；Traefik 3.7.13；
  Gateway API `1.6.3`。
- 端口：Portal/API `30081`、observer ingest `30082`、agent tunnel `30083`、
  lab TLS `30443`；业务入口 `80/443`。
- 发布物料：必须来自正式 release lock；当前 lock 缺失，禁止执行“已认证安装”。

## 1. 安装（正式包到位后）

1. 校验 CLI、Chart、镜像、迁移、依赖和 compatibility 均来自同一 release lock。
2. 运行只读 preflight：OS/arch/cgroup/磁盘/端口/API/镜像仓库/已有 owner。
3. 准备缓存目录与镜像预载；记录压缩下载量、解包磁盘和节点可用空间。
4. 渲染 Chart；确认 SecretReference、StorageClass、GatewayClass 和 ingress 地址。
5. 应用迁移；记录 schema 版本与 rollback 可用性。
6. 等待所有工作负载 Ready；检查 Portal/API/identity/tunnel/operation-worker。
7. 创建/验证 admin 登录；保存 release/profile/actual capabilities/overrides 摘要。
8. 保留 run 记录、lock 摘要、配置摘要和安装日志；不得另建第二状态库。

维护窗口：首次安装与迁移默认需要维护窗口；本次 lab 未验证业务中断预算。

## 2. 纳管独立目标

1. 在目标准备 provider kubeconfig、Agent enrollment 和 Gateway/CSI 前置。
2. 用正式 CLI/API 提交 `ImportRuntimeTarget`；同一 idempotency/correlation 重放必须返回同一结果。
3. Agent 单次 enrollment 交换 session；tunnel 使用目标绑定 token。
4. 等待 read model `ACTIVE / HEALTHY / CONNECTED / FRESH`，节点数和能力清单匹配。
5. 验证断连 → STALE、旧状态保留、重新 enrollment → source-reset → FRESH。
6. 登记 target ID、generation、观测水位和 Agent/连接增量。

## 3. 应用交付

1. 选择 tenant/namespace/target，配额和 storage quota 预检。
2. 提交 composition plan；固定 snapshot digest、镜像 digest、PVC/SecretReference 和 Gateway。
3. 通过 Portal/CLI 提交 deploy Operation；等待 succeeded 后仍需 readiness 验证。
4. 外部 HTTP 写入/读取 marker；在容器内和独立 reader 复读同一数据。
5. 升级固定源/目标 digest、expected UID、data compatibility 和 backup ref。
6. 故障升级保留旧版本；rollback 生成新 Operation，不覆写历史终态。
7. 删除先生成 deletion preview；默认 retain PVC；数据清理需独立授权和 UID 校验。

## 4. 升级/恢复

- **软件版本升级**：release A → B，必须同时校验 lock、migrationChain、兼容矩阵和源安装态摘要。
- **档位调整**：Minimal/Lite/Standard/Pro 变更，必须保留已选能力、配置、身份和数据。
- **能力加载**：同档新增可选能力，通过 extension-controller/Operation 增量启用，不重装 Core。
- 三者不能混用同一操作卡；未认证路径必须拒绝。
- 升级前记录维护窗口、备份点、业务中断预算和持续探针；失败可恢复，不可逆迁移明确拒绝。

## 5. 身份、存储与保留

- 身份：actor 与 service subject 分开审计；短期 token 续期失败 fail closed。
- 存储：PVC 绑定 StorageClass/UID；删除工作负载默认保留；数据清理独立授权。
- Secret：只传 SecretReference，不读取/回显 Secret 内容。
- 卸载：默认保留数据；`-data` 等破坏性参数必须显式确认并记录清理范围。

## 6. 已支持范围（lab）

- 已有 Kubernetes、两节点业务目标、Deployment + Service + PVC + SecretReference +
  一个 Gateway/HTTPRoute、TopoLVM、Portal/CLI 基础应用闭环。
- 不声明：bootstrap/离线、生产 HA、公共 DNS/公信 TLS、正式容量、备份/RTO/RPO、
  多租户生产隔离、参考扩展生命周期。

## 7. 当前阻断

- 无正式 release lock/Chart 包、运行镜像为 lab tag。
- 工作树 dirty；无独立实施者；无干净环境安装。
- 5.2 操作卡未验证，5.7–5.9 保持 blocked。
