# 4.7 基于正式 Intent/Operation 的 CLI create/import 与状态查询

- task: 4.7
- requirements: DPLY-006, PINST-005
- date: 2026-10-12
- status: done（客户端与服务端契约对齐）；真实租户/平台 E2E 属 5.x

## 实现

新增 `internal/toolkit/platform_client.go`，对齐既有服务端契约
（`cmd/apiserver/internal/handler/resource_cluster.go` 的 `SubmitRuntimeIntent`）：

- **typed Intent**：仅接受服务端已声明的 `CreateKubernetesTarget` /
  `ImportRuntimeTarget`，其余 intent kind 直接拒绝（不发明服务端契约）。
- **信封与头一致**：`apiVersion: hnb.io/v1`、`metadata.idempotencyKey` +
  `Idempotency-Key` 头、`metadata.correlationId` + `X-Correlation-ID` 头，
  与服务端 header/body/context 一致性校验对齐。
- **服务端为权威**：响应只反映 `intentId`/`status`/`operationId`/`replayed`，
  CLI 不自行推断状态、不直接执行 Provider、不写平台数据库。

### 错误分类（不回退本地执行）

| 情形 | 分类 | CLI 退出码 |
|---|---|---|
| 5xx / 503 / 超时 / 连接失败 | `PLATFORM_UNAVAILABLE` | 5 `API_UNREACHABLE` |
| 4xx 校验/授权/跨租户 | `REQUEST_REJECTED` | 2 `INVALID_CONFIG` |
| 无可用凭据 | `NOT_AUTHENTICATED` | 2 |

`ErrPlatformUnavailable` 按 DPLY-006 契约**不存在本地 fallback**：不回退 SSH、
不触发 bootstrap、不伪造 operationId。

### Secret 引用边界

CLI 只传 `IntentSecretRef`（provider/scope/name/version）引用，不接受原始密文。
携带 `TenantScope` 时，越界 scope 的引用在**本地**即拒绝，不发往服务端。

### CLI

```bash
hnbctl cluster create --platform-url https://... --token-file /path/0600 \
  --idempotency-key <key> --target-kind kubernetes [--format human|json]
hnbctl cluster import --platform-url https://... --target-id <id> ...
hnbctl operation get <operationId> --platform-url https://... --token-file ...
```

- 强制 HTTPS `--platform-url`。
- token 只从文件读取，且要求 0600，避免出现在 argv/进程列表。
- `cluster list` 明确返回需服务端 list 契约，不假装实现。

## 验证要求对照

| 要求 | 测试 | 结果 |
|---|---|---|
| 同义请求幂等 | `TestSubmitIntentIsIdempotentOnReplay` | PASS — 同 key 两次返回同一 intent/operation，第二次 `replayed=true` |
| 跨租户 Secret 拒绝 | `TestCrossTenantSecretReferenceIsRefused` | PASS — 越界 scope 本地拒绝，服务端零请求 |
| 503 不回退 SSH | `TestPlatformUnavailableDoesNotFallback`、`TestClusterUnavailableReturnsConnectionFailureWithoutFallback` | PASS — 映射为 `API_UNREACHABLE`/退出码 5，无任何本地执行输出 |

补充覆盖：
- `TestSubmitIntentRequiresIdempotencyKey`（空 key 零请求）
- `TestServerRejectionIsNotRetried`（403 只请求一次，不重试）
- `TestMissingCredentialRefusedBeforeRequest`、`TestUnknownIntentKindRefused`
- `TestGetOperationRequiresValidIDAndMapsAvailability`
- `TestClusterNeverFabricatesOperationOnFailure`、`TestClusterRequiresHTTPSAndPrivateToken`、
  `TestClusterImportRequiresTargetID`、`TestClusterUnknownSubcommandRejected`

## 过程中修复的真实缺陷

`hnbctl operation get`（只给子命令、不给 operation id）在实现中触发
**索引越界 panic**。已改为同时接受位置参数与 `--operation`，缺 id 时返回
usage 错误而非崩溃。

## 实际测试结果

```text
cd cmd/hnbctl
go build ./...                   PASS
go test ./... -count=1            PASS（CLI / toolkit）
go vet ./...                     PASS
go test -race ./... -count=1      PASS（CLI / toolkit）
```

## 范围说明

客户端已与服务端 intent 契约对齐，但所有测试使用本地 TLS fixture，**未**执行
真实租户平台 E2E；真实跨租户授权、真实 Agent 与注册 Operation 的验证属 4.8 与 5.x。
CLI 不含任意 steps/commands，也不接受 providerId 自行指定执行。
