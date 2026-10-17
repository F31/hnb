# S2 3.1 import/handoff 受理与可用分离

- Run ID：`s2-import-acceptance-20261017`
- Change：`deliver-lightweight-platform-product-loop`
- Requirements：RTLOOP-001、UXLOOP-001
- 日期：2026-10-17
- 结论：**代码/契约/查询测试通过；独立业务集群实机 import 未执行**。

## 1. 范围

既有链路已具备：BFF 受限 RuntimeIntent + delegation actor、platform-api
ExecutionPlan/Operation、lifecycle Provider、S1 Agent tunnel/session、observer
注册源。本任务补齐“**受理成功 ≠ 目标可用**”的显式状态契约与查询测试：

- `ClusterAcceptance.accepted`：平台已接受 target/import（target 行存在且 lifecycle 非 UNKNOWN）。
- `available`：仅当 `ACTIVE + HEALTHY + CONNECTED + FRESH` 且有真实观测时为 true。
- `observationState`：`pending` / `observed` / `stale`。
- `acceptedAt`：target 创建/受理时间。
- List 与 Detail 共用同一计算；OpenAPI `ClusterAcceptance` 为可选增量字段。

import Operation 成功但没有 Agent 观测时，`status` 保持 `REGISTERING`，
`accepted=true`、`available=false`、`observationState=pending`，不会显示为可用。

## 2. 验证

| 命令 | 结果 |
|---|---|
| `go test ./cmd/apiserver/internal/handler/ -run TestClusterAcceptanceSeparatesAcceptedFromAvailable` | 通过 |
| `HNB_TEST_POSTGRES_DSN=... go test ./cmd/apiserver/internal/handler/ -run TestResourceClusterImportAcceptedButNotAvailable` | 通过（真实 PostgreSQL 16，全部迁移） |
| `go test ./cmd/apiserver/internal/handler/ -count=1` | 通过 |
| `npm run contracts:test` | 28/28 |
| `node scripts/validate-contracts.mjs` | 80 operations/5 APIs、81 schemas、compatibility=checked |

PostgreSQL 场景：先插入 `REGISTERING/UNKNOWN/UNKNOWN/UNKNOWN`、`observed_at=NULL`
的 imported target，断言 `accepted=true/available=false/pending`；更新为
`ACTIVE/HEALTHY/CONNECTED/FRESH + observed_at=now()` 后断言
`available=true/observed`。

## 3. 明确未覆盖

- 未在独立业务集群执行正式 import；当前只有同集群/评估环境，3.3 仍 blocked。
- 未启动真实 Agent 完成 handoff；handoff 的 Operation + 实时观测门槛由既有
  `cmd/hnbctl` 测试覆盖，但未做真实集群 E2E。
- 未验证多副本投影/乱序/缺口/旧 generation 的完整关联；属 3.2。
- `available` 是 Read Model 判定，不是 Provider 方法成功或 Operation 终态的替代。

## 4. 证据身份

| 文件 | sha256 |
|---|---|
| `cmd/apiserver/internal/handler/resource_cluster.go` | `a85702a6b90abf29a2ebef03c3e7d9336d75ddd1dd5780a3bbe1e1a8bdb8b118` |
| `cmd/apiserver/internal/handler/resource_cluster_test.go` | `77a0e3becc7a97ed1f71e8793e1bc44b02cd653ad14289e5b098a99fdbe1b3b7` |
| `cmd/apiserver/internal/handler/resource_cluster_integration_test.go` | `8add62d305fea437e7c5449446098d13e9037d3532a99b9a365956b3305fdae0` |
| `contracts/openapi/console/v1/openapi.yaml` | `aaafb1e4373c6eaf2bdcd64f33c8b1dd69308c178fb7f658894bf6bf2afe89a5` |
| `contracts/generated/go/openapi/console/console.gen.go` | `375a54b590cc6375756a38abf7590373564ea7c390b872131d65a3afbd5aa04e` |
| `web/plugins/resource/src/pages/cluster-management/types/cluster.ts` | `fa2008cefe706b2443b0b3d22c3ea5bf470fcb63890742c20e8112a504d57061` |
