# Startup 凭据与启动可靠性

适用于当前候选 Chart/hnbctl；Startup 是初始组合，完整 Minimal 安装资格另行验收。

## 数据库凭据绑定

每个平面保持独立 Secret，数据库客户端使用 `db.existingSecret` 和 `db.secretKey`
（默认 `dsn`）。Secret 的 `dsn` 键保存完整连接串；bundled PostgreSQL 的 owner Secret
还需保留 `password` 键。App Market 绑定 market 平面，平台组件绑定 platform 平面。

Chart 在同时存在 `existingSecret` 和旧 `dsn` 时优先引用 Secret；旧内联 DSN 保留兼容，
新实施流程使用 Secret。Secret 由实施方准备，不放进 Chart values、物料包或验收报告。

## 启动与探针

apiserver、app-market、platform-api、operation-worker、extension-controller：

- 数据库启动等待最多两分钟，单次 ping 最多三秒；认证/数据库名错误立即失败。
- NATS 启动时对无可用服务和超时退避重试，最多两分钟；配置/凭据错误直接返回。
- `/livez` 表示 HTTP 进程存活；`/readyz` 检查数据库以及实际使用的 NATS 连接。
- 正常返回 204；依赖不可用时 readiness 返回 503，不将内部错误细节输出给客户端。
- API 服务使用 8080，worker/controller 的健康端口为 8081，不创建外部入口。
- startup probe 窗口五分钟；readiness 与 liveness 分开，依赖故障不会触发存活重启。
- 使用旧镜像时可显式设置对应组件的 `probes.enabled: false`；新镜像默认启用。

## 锁定的候选清单渲染

已实现命令：

```bash
hnbctl render -f hnb.yaml \
  --chart-package hnb.tgz \
  --bindings bindings.yaml \
  --stage startup \
  --output rendered.yaml
```

`hnb.yaml` 使用 existing 路线并引用真实 candidate release lock；bindings 提供身份、
TLS、服务认证等现有 Secret 名及必要配置。数据库连接不要填写 `db.dsn`，采用独立
Secret 引用。公开入口/TLS 仍需按部署环境单独验证，渲染不会验证其实际可达性。

当前 Chart 的 `platform-api.masterKeySecret` 为必填，引用同 namespace Secret 的
`key` 键（32 字节 AES-256 密钥的 64 位 hex 表示）。启用 lifecycle provider 时，
其 `masterKeySecret` 必须引用同一密钥；不能为已保存的凭据随意重新生成密钥。
该 Secret 内容由实施方单独创建并备份，不写入 bindings。

叠加 `capability-runtime.yaml` 时，`kubernetes-provider.allowedNamespaces` 必须显式
填写执行范围；默认监听 `:8080`，Chart readiness 检查 `/healthz`。
provider Ready 并不等于 Operation 可执行：还需要有效的动作级 conformance、正式
provider 注册，以及 worker 的精确 provider 摘要/版本与对应 audience 服务令牌绑定。
当前真机前置验证与阻塞记录见
`docs/acceptance/reports/20261013-startup-runtime-preflight.md`。

## Provider 动作级 conformance

平台 owning-plane 提供 `POST /v1/providers/{id}/manifest` 与
`POST /v1/providers/{id}/conformance`：

1. manifest 登记需要 `artifactDigest`（linux/amd64 manifest 摘要）。响应头
   `X-Manifest-Identity-Digest` 是可用于登记报告的基础 manifest 身份摘要。
2. conformance 报告（`provider-conformance/v1`）必须由配置的 Ed25519 受信签名者
   签署，并绑定 provider 版本、artifactDigest、基础 manifest 摘要、兼容矩阵版本、
   精确 targetKind/action 以及有效期。公钥通过 `conformance.existingTrustSecret`
   挂载；未配置时该端点一律失败关闭，不存在默认可信方。
3. 登记在事务内对 manifest 行加锁后复核身份，报告与 cell 证据一并保存；期间
   manifest 被替换则登记失败。当前仅接受 `KubernetesTarget`/`import`，并要求
   contract/functional/security/fault/idempotency/fencing 全部通过。
4. 报告具备资格不等于 provider 已接入：worker 还需在 `runtimeProviders` 中固定
   endpoint、audience、tokenFile、`providerVersion` 与 `providerDigest`（必须与
   Intent 计划中固定的 manifest 摘要一致）。服务令牌不能直接用 Secret 挂载：在
   fsGroup 下文件会解析为 0440，而 `iam.FileTokenSource` 要求 owner-only。Chart
   通过 `operation-worker.serviceTokenFiles` 由 init 容器把令牌暂存为 0400 的
   内存卷并挂载到 `/var/run/hnb-provider-tokens`；配置 `runtimeProviders` 时若未
   声明令牌键将直接拒绝渲染。
   **仍未完成**：暂存副本只在 Pod 重启时更新，而令牌上限为 60 秒
   （`iam.MaxAccessTokenTTL`），因此缺少签发与轮换来源，不能用于长期运行。
   详见 `docs/acceptance/reports/20261013-runtime-import-operation.md`。

渲染行为：

1. 校验 Chart 包名称、版本与摘要，读取包内 Startup/Minimal overlay。
2. 用 lock 中的 amd64 manifest 固定镜像，拒绝未锁定的渲染镜像。
3. 将已声明客户端的数据库/NATS Secret 绑定到配置对应平面，拒绝跨 namespace 引用。
4. 为 namespaced 资源写入明确 namespace；拒绝渲染 Secret 内容或内联 DB_DSN。
5. 写入 0600 本地清单；相同内容可重复执行，不替换已有不同内容。

命令输出明确标记 `installationExecuted: false`。它不会执行集群 apply、迁移、完整
业务验收或 handoff；`hnbctl install` 的实际执行仍受完整 Minimal E2E/物料条件约束。
Startup 的手工应用实验不能替代干净主机首次安装认证。

真机故障演练和重启证据见 `docs/acceptance/reports/20261013-startup-priority-optimization.md`。

## 孤立步骤重驱动

步骤请求消息若在 worker 消费者不在时投递，随后 worker 重启，该步骤会永久停留在
`pending`：JetStream 的 `AckWait` 重投递只在有消费者时推进，而 provider reconciler
只处理 provider binding 漂移。worker 现在内置周期清扫（默认 60s，空闲阈值 120s，
必须大于 60s `AckWait`）：

- 仅处理非终态 Operation 下、仍为 `pending`、且**没有尚未发布请求**的步骤；
- 用 `FOR UPDATE ... SKIP LOCKED` 取候选，并发清扫不会对同一步骤重复投递；
- **重新武装**该步骤已提交的 outbox 行（`status=pending`、`attempt+1`、
  `next_attempt_at=now()`），而不是新增副本：`outbox_events` 对
  tenant+type+idempotency_key 唯一，而 worker 要求 envelope、payload 与
  `operation_steps` 三者同键，派生键的副本会被直接隔离；
- 空闲阈值以毫秒显式换算，避免 PostgreSQL 把整数当秒；
- 重投递次数不超过步骤 `max_retries`；
- 重复消息不会重复执行：租约与 fencing generation 仍只允许一个持有者。

超过预算的步骤保持 `pending` 并输出显式告警，**不会自动失败**——自动失败需要与
真实执行相同的租约保护状态迁移，交由运维决定，避免 worker 静默终止 Operation。
真机部署与验证尚未完成，缺陷与证据见
`docs/acceptance/reports/20261013-runtime-import-operation.md`。
