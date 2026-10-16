# Startup 组合真实云主机部署测试（106.75.34.133）

- run_id: `ubuntu-startup-composition-20261013`
- host: `106.75.34.133`；Ubuntu 24.04 / kernel 6.8.0-138-generic / 2 vCPU / 3913Mi / 根文件系统 18.33GiB
- commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73`（工作区含本次修复）
- 结论: **当前 Chart 重新渲染后 10/10 工作负载 Running；真实登录/鉴权/数据读取通过；真实重启后全部恢复，工作空间与租户数据摘要一致。**
- 边界: 依赖层与初始组合验收；不含运行目标、Agent、真实 Operation、HA、离线条线安装认证。

## 部署形态

单节点 K3s `v1.36.5+k3s1`（二进制与 airgap 镜像按官方 SHA-256 校验后安装，仅作轻量
测试底座，**不改变既有 kubeadm 安装路线的资格结论**），在其上以 `startup.yaml` +
`startup-small-disk.yaml` 部署 10 个独立工作负载：

apiserver、platform-api、operation-worker、extension-controller、app-market、
cluster-router、tunnel-server、portal、postgresql、nats。

镜像由本机按各服务 Dockerfile 等价方式构建（CGO_ENABLED=0 scratch / nginx 静态前端），
digest 固定后导入 containerd。身份由真实 `hnbctl identity generate` 生成，
每平面独立数据库角色与 NATS 账户，TLS 证书在主机生成。

## 真实资源占用（实测，非规划值）

| 项目 | 实测值 |
|---|---|
| 节点内存（修复后重启复测） | 3914 MiB 总量，可用 2932 MiB |
| 根文件系统（修复后重启复测） | 18.33 GiB 总量，已用 5.43 GiB，可用 12.88 GiB |
| K3s 控制面（不含镜像） | 1754 MiB |
| K3s airgap 镜像包 | 185 MiB |
| containerd 内容 | 1377 MiB |
| PostgreSQL 数据 | 101.6 MiB |
| JetStream 数据 | 0（尚无业务消息） |
| 工作负载 requests（合计） | 1075m CPU / 1792Mi RAM |

节点内存/文件系统来自 `20261013-startup-composition-fixed-after-reboot.json`。
底座、镜像和数据目录大小保留首轮 `20261013-startup-composition-measurements.json`
测量；它们是磁盘字节数，不能作为进程内存占用，且目录统计可能包含重叠内容。

对比容量模型：模型假设系统预留 10Gi 磁盘 + 500m/1Gi 内存；实测系统与运行时占用远低于
该预留，说明 Startup 的 10Gi 小数据预算是**保守可行的**，但 JetStream 与业务数据尚未增长。

## 业务验收（重启后复测）

| 检查 | 结果 |
|---|---|
| 10 个工作负载 | 全部 `1/1 Running` |
| `apiserver /health` | 200 `{"status":"ok"}` |
| `app-market /health` | 200 `{"status":"ok"}` |
| Portal 首页 | 返回真实 SPA（`HNB 云原生平台`） |
| 匿名访问 `/api/v1/auth/me` | **401**（鉴权生效） |
| 管理员登录 | 成功签发 ES256 JWT（1500 字符） |
| `/api/v1/workspaces` | 200，返回默认工作空间 |
| `/api/v1/tenants` | 200，返回默认租户 |
| 无权限路由 | 403 `route is not authorized` |
| 真实重启（boot_id 变更） | 10/10 恢复，登录与数据读取恢复，工作空间/租户返回数据 SHA-256 一致 |

`platform-api /health` 返回 `unauthorized`，说明该端点要求鉴权（符合最小暴露原则）。

## 测试暴露的真实缺陷（已定位根因）

| # | 缺陷 | 影响 | 状态 |
|---|---|---|---|
| 1 | **签名密钥时间戳精度不匹配**：`pkg/iam` 用 `time.Equal` 比较 PostgreSQL `TIMESTAMPTZ`（微秒）与 manifest（纳秒） | 首次记录后 **apiserver 永远无法重启** | **已修复**（`Round(time.Microsecond)`）+ 回归测试 |
| 2 | **启动引导非幂等**：`bootstrapAdmin` 无条件插入 `tenants`/`workspaces`，而 `ensureDefaultWorkspace` 已创建 | 首次引导失败后**永久无法重试**，平台无法创建管理员 | **已修复**（`ON CONFLICT DO NOTHING`） |
| 3 | **身份 manifest 内嵌主机绝对路径** | 容器内读取公钥失败，校验器启动即崩溃 | **已修复**：相对路径与目录解析，真机复验通过 |
| 4 | **Chart Secret `0400` 与非 root UID 65532 冲突** | 身份/TLS/账户凭据不可读，签名私钥亦不可读 | **已修复**：Chart fsGroup + `0440`；私钥 init 暂存为 owner-only `0400` |
| 5 | **NATS 账户认证与客户端不匹配** | 服务端按账号密码认证，客户端只有 mTLS，连接被拒绝 | **已修复**：账户 Secret 文件认证，真机复验通过 |
| 6 | **platform-api/operation-worker 读 `DB_*` 而 Chart 只传 `DB_DSN`** | 回落 localhost/hnb，无法连接 | **已修复**：解析 `DB_DSN`，真机复验通过 |
| 7 | **app-market 在 scratch 镜像中 `/tmp` 不可写** | 启动即失败 | **已修复**：Chart 有界 emptyDir，真机复验通过 |
| 8 | `helm template --namespace` 不为工作负载/PVC 写入 namespace | 资源落入 `default`，与 Secret 错位 | 评估侧注入；属 Chart 陷阱 |

其中 #1、#2 是**阻断级**缺陷：前者使平台无法重启，后者使平台无法完成首次引导。

## 已修复（本次会话落地，含回归测试）

| 缺陷 | 修复位置 | 回归测试 |
|---|---|---|
| 签名密钥时间戳精度 | `pkg/iam/key_store.go` 改为 `Round(time.Microsecond)`，与 PostgreSQL 一致 | `TestRecordKeyManifestAcceptsNanosecondManifestAfterDatabaseRoundTrip` |
| 启动引导非幂等 | `cmd/apiserver/bootstrap.go` 租户/工作空间插入加 `ON CONFLICT DO NOTHING` | 真机：管理员创建成功并可重启 |
| 身份 manifest 主机绝对路径 | `hnbctl identity generate` 记录相对路径；`pkg/iam` 按 manifest 目录解析并拒绝路径穿越 | `TestKeyManifestResolvesPublicKeyRelativeToManifest`、`TestKeyManifestRejectsTraversalAndUncleanKeyPaths` |
| NATS 认证不匹配 | `pkg/messaging` 新增 `NATS_USER_FILE`/`NATS_PASSWORD_FILE`（私有文件，权限校验）；25 个子 Chart 挂载各平面账户 Secret | `TestNATSConfigFromEnvReadsAccountCredentialPaths`、`TestComponentsAuthenticateNatsPlaneAccountFromFiles`、`TestNatsAccountCredentialsAreOptionalWhenUnset` |
| 签名密钥 0400 与非 root 冲突 | `apiserver` 增加 init 容器把密钥移交为 UID 65532 的 0400 文件，不放宽 IAM 校验；init 镜像同样 digest 固定 | `TestApiserverStagesPrivateKeyForTheNonRootUser` |
| platform-api/operation-worker 只读 `DB_*` | 二者改为直接消费 Chart 已提供的 `DB_DSN`（`pkg/db.ParsePostgresURL`）；内联密码问题仍见下节 | `TestParsePostgresURL*` |
| app-market 无可写 `/tmp` | 默认暂存目录回退到首个可写位置；Chart 提供有界 emptyDir | `TestAppMarketGetsAWritableBoundedTransferStagingVolume` |
| 通用 Secret 挂载权限 | Go 工作负载声明 UID/fsGroup 65532，Secret 为 `0440`；NATS 用独立 UID/fsGroup 1000 | `test_every_component_declares_non_root_pod_security_context` |
| Portal 非 root 写目录失败 | UID 101，nginx 主配置将 pid/temp 移到 64Mi 有界 `/tmp` emptyDir，监听 8080，Service 仍为 80 | `test_portal_non_root_nginx_uses_writable_temp_paths` |

本轮新增/修改测试：Chart 与容量集成 **39 项全部通过**；IAM/DB/messaging/hnbctl/
apiserver/app-market/platform-api/operation-worker 的 `go test -race` 全部通过；
OpenSpec 严格校验 29 项通过，质量门禁 31 specs / 242 requirements / 377 scenarios 通过。

## 当前 Chart 真机复验（2026-10-13 08:20–08:22 UTC）

从当前 Chart 运行 `helm template hnb deploy/charts/hnb --namespace hnb-system`
（`startup.yaml` + 真机 bindings；保留已有 PG 2Gi、NATS 1Gi PVC），再为 namespaced
资源明确写入 namespace。使用 containerd 实际解析的 amd64 manifest digest；评估镜像
pullPolicy 为 `Never`。本轮未再注入身份路径改写、凭据 URL、DB_*、权限或暂存目录补丁。

- 镜像已重导入；extension-controller/cluster-router/tunnel-server 也切换至本轮构建。
- 上轮评估脚本曾把 `0440` 写为十进制 292（实际 `0444`）；本轮直接采用 Chart 渲染
  的 288（`0440`），凭据权限检查保持拒绝 other 访问。
- NATS 旧的未就绪 Pod 阻塞 StatefulSet 滚动更新；删除旧 Pod 后按新模板恢复。
- 重启前 10/10 Ready，所有当前 Pod 的 restartCount 为 0。
- 执行 `sudo -n systemctl reboot`，boot_id 从
  `4903e410-c066-41f1-b329-6daf8e225025` 变为 `2be57f51-32ee-46b3-b830-f7637c8b320a`。
- 重启后 10/10 Ready；部分服务等待依赖期间有一次额外重试，最终业务断言全部通过。
- 登录通过 Portal 同源代理完成；工作空间/租户摘要与重启前相同。
- 此次复验验证读取与持久化，未新增业务写入或真实 Operation 验收。

复验命令：`sudo -n python3 /home/ubuntu/verify-current.py before-reboot` / `after-reboot`。
可复用脚本、重启前后结果和产物摘要分别保留于：

- `20261013-startup-composition-fixed-verify.py`
- `20261013-startup-composition-fixed-before-reboot.json`
- `20261013-startup-composition-fixed-after-reboot.json`
- `20261013-startup-composition-fixed-artifacts.json`

## 仍未修复（已确认，非本次改动引入）

**后续状态（2026-10-13）：**下述内联 DSN 缺陷已在后续优先级优化中修复，当前 Chart
支持 `db.existingSecret`/`db.secretKey`，真机已切换为 Secret 引用。详情与故障演练见
`20261013-startup-priority-optimization.md`。本节保留首轮发现时的历史状态。

Chart 仍把 `DB_DSN`（含数据库密码）内联渲染进 Deployment 环境变量，密码会出现在
Pod spec 中。`db.existingSecret` 已在 values 中存在但模板未使用。这属于既有设计，
需要单独改造为 Secret 挂载，本次未扩大改动范围。

## 与容量模型的关系

- 实测 Startup 10 工作负载在 2C/3.8Gi 主机上**真实可运行**，修复后磁盘占用约 5.43Gi。
- 之前 27Gi 规划偏保守；采用 `startup-small-disk`（PG 2Gi + JetStream 1Gi）后合理。
- **2C4G 起步档在初始组合层面已被真机证实**；但完整 Minimal（19 工作负载）仍需更大规格。
