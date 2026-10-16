# 4.8 初次管理关系 handoff（receipt 仅作附件）

- task: 4.8
- requirements: PINST-005, DPLY-006
- date: 2026-10-12
- status: done（客户端契约与防伪造规则）；真实 Agent 端到端观测属 5.x

## 实现

新增 `internal/toolkit/handoff.go` 与 `hnbctl handoff`。

handoff 严格分两步，缺一不可：

1. **正式注册 Operation**：经 `SubmitIntent(ImportRuntimeTarget)` 提交 typed Intent，
   取得服务端返回的 `operationId`。不直接写 Read Model，不调用 Provider，
   不自行拼装 Operation。
2. **真实 Agent 观测**：经 `GetAgentObservation` 读取服务端权威的
   `runtime_targets` Agent 状态（`agentConnected` / `observedAt`）。

`Complete` 仅在**同时**满足以下条件时为 true：

- `RegistrationOperationID != ""`
- `observation.Authenticated == true`
- `observation.ObservedAt` 非零**且晚于 handoff 时刻**

否则返回 `PLATFORM_NOT_VERIFIED` 并列出 `reasons`。

### receipt 仅作附件

`HandoffAttachment`（`install-handoff/v1`）只承载**本地安装事实**：runId、
configDigest、lockDigest、各阶段 `phaseTimes`（明确标注为安装器时间）与
`attachedAt`。它**不包含** operationId、status、readModel 或任何凭据，
仅用于日后与正式 Operation 做关联。

### 防伪造规则

- 初始化阶段时间**不能**充当 Agent 实时观测时间：观测时间必须晚于 handoff 时刻，
  否则判定不完整并给出 `installer times cannot stand in` 理由。
- 平台不可用（5xx/503/超时）→ `ErrPlatformUnavailable`，CLI 退出码 5；
  **不**本地补一个 operationId 或 observation。
- 未认证 Agent、缺失观测时间同样不完成 handoff。

## CLI

```bash
hnbctl handoff --run-dir <dir> --target-id <id> \
  --idempotency-key <key> --platform-url https://... --token-file <path>
```

## 验证要求对照

| 要求 | 测试 | 结果 |
|---|---|---|
| 真实 Agent 认证/观测与正式注册 Operation | `TestHandoffCompletesOnlyWithOperationAndLiveObservation` | PASS — 同时发生 `POST /runtime-intents` 与 `GET /resources/clusters/{id}` |
| 禁止直接写 Read Model / receipt 单独完成 | `TestReceiptAloneNeverCompletesHandoff`、`TestHandoffAttachmentCarriesOnlyLocalFacts` | PASS |
| 初始化时间不得伪装成观测时间 | `TestInstallerPhaseTimesCannotMasqueradeAsObservation` | PASS |
| 不可用不伪造 | `TestHandoffUnavailableDoesNotFabricateObservation`、`TestHandoffUnavailableNeverFabricatesObservation` | PASS |

补充覆盖：`TestUnauthenticatedAgentDoesNotCompleteHandoff`、
`TestMissingObservationTimeDoesNotComplete`、`TestHandoffRejectsInvalidTargetID`、
`TestHandoffRequiresMandatoryFlags`、`TestHandoffRequiresOperationAndObservation`。

## 实际测试结果

```text
cd cmd/hnbctl
go build ./...                   PASS
go test ./... -count=1            PASS（CLI / toolkit）
go vet ./...                     PASS
go test -race ./... -count=1      PASS（CLI / toolkit）
```

## 范围说明

所有观测与注册均来自本地 TLS fixture，验证的是**客户端防伪造与契约对齐**。
真实 Agent 建立隧道、认证并上报观测的端到端证据属 5.3/5.5。本任务不声称
已完成真实管理关系建立。
