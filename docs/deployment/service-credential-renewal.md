# S1 服务凭据续期：首批契约与实现边界

关联 IDENT-001、任务 1.4/1.5 的 S1 子集、2.3。

## 1. 本批决策依据

2026-10-17 用户在决策基线交付后指示“按规划继续执行”，据此采纳
`decision-baseline.md` 修订 0.1 的 **S1-IDENTITY 开发输入**；该指示前文档摘要为
`sha256:f0a52c4ae54c5e06e43f5a3501bc253181baf9cd92de9252149a3ab7121f1c1c`。
记录时间为 `2026-10-17T06:56:00Z`（工具观察时间，不伪造对话消息精确时间或用户账户）。

- 复用 apiserver/IAM owner、现有签名 profile/验证公钥及 60s service access token 上限。
- 使用 50% TTL（±5%）提前续期、1/2/4s（±20%）退避、2s 单次请求界限。
- 5s/60s 撤销是后续 issuer/verifier/连接接线的验收目标，不因本批客户端完成而宣告达到。
- 本次不批准生产容量、RTO/RPO、独立集群或整站环境，也不生成带假摘要的 approved ReadinessPlan。

## 2. 冻结本批消费者边界

`contracts/schema/identity/v1/service-credential-scope.schema.json` 是新增的、无秘密的精确范围
契约：`subjectId`、一个 `audience`、一个 `permission`（tenant/resource kind/resource ID/action，
可含既有 project/environment/namespace 层次）。禁止通配符、空资源 ID、未知字段和非法层次。
这只是申请/校验范围，**不是工作负载身份的可信来源**，不能仅凭这些客户端字段签发权限。

`pkg/iam` 提供：

- `ServiceCredentialIssuer.IssueServiceCredential(ctx, scope)`：每次用受信 workload 证明获取新凭据。
  不接收旧 access token 作为续期证明；IAM/HTTP/Projected SA 适配由 2.2/2.4 后续接入。
- `NewRenewingTokenSource(parent, verifier, issuer, scope)`：实现现有 `TokenSource`，首次使用才启动，
  之后即使业务暂时空闲也自动续期；实例绑定一个固定范围，不能靠请求上下文切租户/Provider。
- `Token(ctx)`：冷启动合并并发取得首个身份；取消某个调用者不取消其他调用者的共享签发。
  对已过期、停用、撤销或不能验证的凭据不返回 token；短暂签发故障时仅可使用尚未过期的旧凭据。
- `Close()` / 父 context 取消：释放后台续期，之后不再提供 token；不把缓存或 token 写磁盘。

响应字符串先经既有 `TokenVerifier` 验签，再从受信 claims 读取有效期；拒绝 user token、
错误 profile/audience/subject/tenant、额外 audience/action/permission，以及更宽的资源范围。
不从未验签的 JWT payload 自行推导“可继续使用”的时间。未来签发端若需要服务多 scope，
应拆成各自固定范围的 source，不在此处悄悄放大缓存授权。

这是 **Go 消费者 + JSON scope 契约冻结**，不是完整在线 issue/renew/revoke 或 enrollment
API 冻结；1.5 仍有端点、鉴权证明、撤销/入网持久化、事件及迁移产物待交付。
本批不添加假 HTTP 端点或尚未接线的生成客户端，也不移动 IAM 私钥。

## 3. 失效与恢复语义

- issuer 返回 `ErrServiceIdentityRevoked`：立即停止本 source 并丢弃旧 token，不重试已撤销身份。
- issuer 返回 `ErrServiceCredentialRejected`，或返回无法验证/越权的新凭据：拒绝并停止，不用旧
  凭据隐藏永久拒绝；需要重新授权并创建新 source。
- 其他 issuer 失败视为暂时不可用；保留有界退避，旧 token 只到原 `exp`。不足 2s 完成下一次
  签发时等待旧 token 到期后重新证明身份；过期期间业务调用 fail closed，不延长 TTL。
- 过期后可用仍有效的 workload 身份重新获取凭据，不使用旧 token 来授权这个动作。
- 不暴露 issuer 原始错误正文或缓存 token 的格式化输出；可观察错误是稳定、脱敏的类别。
- issuer 实现必须遵守 context deadline/取消；本包不创建无限 goroutine 来包装不合作的 issuer。

JWT 本身的撤销状态、2s 权威状态缓存和活动隧道失效仍属于 2.2/2.7。这个 TokenSource 只有在
issuer 明确返回撤销/永久拒绝时立即停用，不能被当作“运行端 5s 撤销检查器”。

## 4. 接入顺序与验证范围

1. 本批：范围契约、共享续期组件、伪时钟/并发/错误与重启恢复测试，不改运行中配置。
2. 2.1/2.2（2026-10-17 已交付本地实现）：IAM owner 持久化、原子消费/撤销、受限
   issue/renew/revoke API 与 actor/service subject 审计；workload 证明适配仍归 2.4。
3. 2.4：客户端 adapter 与按租户 source 已实现；部署级 per-tenant factory、operation-worker
   接线和 BFF actor 保留验证仍待完成。BFF 继续每请求 delegation，不改成共享 actor 会话。
4. 2.5–2.7：Agent 原子入网、会话身份轮换与长连接撤销接线。
5. 2.8：按基线进行真实跨 TTL/重启/撤销时延演练，单元测试不得提升 real_e2e/HA 认证。

该 scope/消费者本身无数据库和网络 endpoint，不需要单独数据库迁移；这不意味着整个 S1 的
持久化任务 2.1 是 N/A。

## 5. 2.1/2.2 owning-plane 增量（2026-10-17）

### 5.1 持久化与原子性

- 迁移 `database/postgresql/migrations/105_service_identity_lifecycle.sql`（含 rollback twin）
  新增 `service_identities`、`service_credentials`、`service_enrollment_credentials`：
  - `service_identities` 固定 tenant/subject/audience/JSONB permission、状态和 CAS version；
  - `service_credentials` 为每个已返回 JWT 记录 jti/有效期/撤销状态，不保存 token；
  - `service_enrollment_credentials` 只保存 SHA-256 哈希；
    `UPDATE ... WHERE consumed_at IS NULL AND expires_at > now RETURNING` 是唯一原子消费点。
- IAM 表 owner 仍是 `apiserver`；三个表已加入 `docs/table-ownership.md`。
- 集成测试（`-tags integration`，`HNB_TEST_DATABASE_URL`）覆盖连续两次迁移、100 并发消费
  恰好一次成功、版本冲突、重复撤销、跨租户/跨目标/过期拒绝；在 PostgreSQL 16.15 通过。

### 5.2 受限 HTTP 契约

| Method / Path | 授权 | 行为 |
|---|---|---|
| `POST /api/v1/service-identities` | `serviceIdentity:create` | 注册精确 subject/audience/permission |
| `GET /api/v1/service-identities`、`GET /{id}` | `list` / `read` | 租户内查询，不返回 token |
| `POST /api/v1/service-identities/{id}/credentials` | `update` | 签发短 TTL 凭据；默认/上限 60s |
| `POST /api/v1/service-credentials:renew` | public route + bearer 校验 | 用当前凭据换取同 scope 新 jti |
| `DELETE /api/v1/service-identities/{id}` | `delete` | `{version,reason}` CAS 撤销整个身份 |
| `POST /api/v1/service-identities/{id}/credentials/{credentialId}:revoke` | `delete` | 撤销单个凭据，幂等 |

- Schema：`service-identity-create.schema.json`、`service-credential.schema.json`；
  credential 字段标为 `writeOnly`，契约测试覆盖通配/越权/未知字段/非法 UUID/TTL 边界。
- 凭据响应设置 `Cache-Control: no-store`；审计中间件对这些路径不记录请求/响应正文；
  错误响应不回显 token、issuer 原文或数据库细节。
- 续期是**持有式过渡路径**：当前凭据同时是 bearer。Projected ServiceAccount/workload
  proof 仍需 2.4/1.5 接入；不能把本接口描述成 RFC 8693 token exchange。

### 5.3 授权与审计

- 新资源类型 `serviceIdentity` 复用既有 `create/list/read/update/delete` action，
  不新增 action 枚举，不影响现有权限快照。
- `audit_logs.service_subject_id` 区分 actor（`user_id`）与服务主体；create/issue/renew/revoke
  都写入该字段，且不含凭据值。
- 撤销权威状态在数据库行，而不是 JWT 过期：`Verify`/`Renew` 在签名仍有效时也拒绝已撤销
  的凭据或身份。5s/60s 传播仍是目标；本批没有撤销缓存，也没有长连接失效接线。

### 5.4 客户端 adapter 与 2.4 剩余工作

- `HTTPCredentialIssuer` 用 bootstrap token 调用续期端点，成功后只在内存替换最新 token；
  响应有界，错误不包含 token/正文。
- `TenantRenewingTokenSource` 按租户懒创建固定 scope 的 `RenewingTokenSource`；
  operation-worker 驱动在 source 实现 `iam.TenantTokenSource` 时按 `execution.TenantID` 取凭据。
- 2026-10-17 已完成 deployment 接线：`RUNTIME_PROVIDERS[*].serviceIdentities[tenant]` 声明
  `identityId/subjectId/tokenFile`，`SERVICE_IDENTITY_ISSUER_URL`、`SERVICE_IDENTITY_ISSUER`、
  `SERVICE_IDENTITY_KEY_MANIFEST_FILE` 提供 issuer 与公钥；未声明 tenant 直接拒绝，续期失败
  不回落 tokenFile。BFF 继续每请求 delegation，不共享 actor 会话。
- 仍未完成：生产环境 rollout、超过两个 TTL 且含重启/撤销时延的真实演练（2.8），以及
  BFF→领域 API 的独立 actor 保留集成用例（现有 `DelegationSigner/Verifier` 单测覆盖绑定）。

## 6. Agent 单次入网（2.5，2026-10-17）

### 6.1 持久化

`service_enrollment_credentials` 已扩展为：

- `id UUID`（管理/撤销）、`token_hash TEXT UNIQUE`（SHA-256，仅存哈希）；
- `tenant_id`、`target_id`、`expected_subject`、`issued_by_subject_id`；
- `expires_at`（15min）、`consumed_at`/`consumed_by_subject`、`revoked_at`/`revoked_by_subject_id`；
- 约束：`expires_at > created_at`、消费后必须有消费主体、撤销与消费互斥。

`ConsumeEnrollmentCredential` 仍是唯一原子消费点：

```sql
UPDATE service_enrollment_credentials
SET consumed_at = $5, consumed_by_subject = $6
WHERE token_hash = $1 AND tenant_id = $2 AND target_id = $3 AND expected_subject = $4
  AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at > $5
RETURNING ...
```

### 6.2 API

| Method / Path | 授权 | 行为 |
|---|---|---|
| `POST /api/v1/resources/clusters/{id}/agent-enrollments` | `cluster:read` | 生成 15min 单次 secret，返回 `enrollmentId`/`enrollmentSecret`/`expiresAt`；仅存哈希 |
| `POST /api/v1/agent-enrollments:exchange` | public + 单次 secret | 原子消费后签发 5min `hnb.agent-tunnel/v1` 会话，绑定 tenant/target |
| `DELETE /api/v1/resources/clusters/{id}/agent-enrollments/{enrollmentId}` | `cluster:update` | 撤销未消费的 enrollment |

- 交换请求必须同时提交 `tenantId`/`targetId`；与记录不一致、已消费、已撤销或过期都返回
  同一 401，避免泄露存在性。
- 会话签发前再次确认 target 仍存在且为 Kubernetes；已删除/类型变化的目标不会得到会话。
- 响应设置 `no-store`；审计路径抑制请求/响应正文，审计事件只含 tenant/target/enrollmentId，
  不含 secret。
- Schema：`agent-enrollment-create`、`agent-enrollment-exchange`、`agent-session`。
- Agent 二进制的初次 exchange、轮换与重连接线属于 2.6；本批完成服务端闭环与测试。

## 7. Agent 会话轮换与撤销（2.6/2.7，2026-10-17）

### 7.1 会话模型

- `agent_sessions`：`id`（session）、`tenant_id`、`target_id`、`subject`、`generation`、
  `issued_at`、`expires_at`、`revoked_at`。首次 exchange 生成 generation=1；同一
  tenant/target 的新 enrollment 递增 generation。
- `agent-tunnel/v1` 新增可选 `sessionId`/`generation` claims；未携带两者的旧 token
  仍按 legacy 路径验证。服务端 `verifyToken` 对携带 session 的 token 额外核对持久化
  session 未撤销、未过期且 generation 一致。

### 7.2 轮换

- `POST /api/v1/agent-sessions:renew`（public，bearer 为当前 session token）：
  - 未验签 payload 只用于定位 session 行；随后用 `AgentTunnelTokenVerifier.VerifyClaims`
    验签并核对 session/generation；
  - 已撤销/过期/不匹配统一 401；
  - 成功时保持同一 session ID/generation，更新 `issued_at/expires_at` 并签发新 token。
- Agent `AgentSessionTokenSource`：单次 exchange 后按 50% TTL 轮换；临时失败有界退避，
  旧 token 仅到原 `exp`；撤销/永久拒绝立即停止。Agent 重连时由 tunnel client 重新取
  token，因此轮换后的连接使用新凭据。
- observer generation 是独立的观测代次：旧 generation 仍由 `Projector.Accept` 拒绝
  （`ErrFenced`），会话轮换不会重置或绕过它。

### 7.3 撤销与持久连接

- `DELETE /api/v1/resources/clusters/{id}/agent-session`：撤销该 tenant/target 的全部
  活动 session，并调用 `TunnelServer.Revoke(tenantID, targetID)`。
- `TunnelServer.Revoke` 只关闭 tenant 与 target 都匹配的连接；`CloseRevoked` 发送
  `agent session revoked` close reason。Agent 将其映射为 `ErrSessionRevoked`，日志记录
  “durable identity revoked or rejected; re-enrollment required” 后退出，不再重连。
- 无活动 session 的撤销是幂等的（`revoked=0`）；不会关闭其他 tenant/目标的连接。
- 真实 5s 新调用/60s 长连接传播测量、身份服务故障与多连接压测仍归 2.8；本批只证明
  关闭/拒绝语义和精确作用域。

### 7.4 2.8 演练结果与重启恢复

- 本地进程级真实链路演练（真实 HTTP/WebSocket/PostgreSQL 16.15、真实 handler/store/
  tunnel/session source）：
  - 4s session TTL（生产默认 5min，路径相同）跨 >2 TTL：11.93s 内 3 次重连，
    generation 保持 1，无手工 token；
  - 关闭 Agent source 后用**已消费**的 enrollment secret 重建：从 0600 session file
    恢复，未再 exchange；
  - 撤销后持久 WebSocket 在 **3.95ms** 关闭，旧 session renew 被拒（总 **6.38ms**）；
  - 跨租户 exchange 被拒；精确 tenant/target 关闭由 tunnel/handler 测试覆盖。
- Agent 需要 `AGENT_SESSION_FILE`（0600，可写）持久化最新 session；重启发生在当前
  token 有效期内可直接恢复。若停机超过 token TTL，单次 enrollment 已消费，需受控重新入网。
- 生产 5min soak、云主机/HA、身份服务真实断网与 60s 多连接传播测量仍为环境依赖；
  本演练不提升这些资格。
