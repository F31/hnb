# 5.1 分层 verify 检查集与自有测试资源清理

- task: 5.1
- requirements: PINST-005, DPLY-005
- date: 2026-10-12
- status: done（检查集、失败门与清理契约）；真实环境各层观测属 5.3/5.4/5.5

## 实现

新增 `internal/toolkit/verify.go` 与 `hnbctl verify`。

### 必需检查目录（13 项，四层）

| 层 | 必需检查 |
|---|---|
| node | `node.readiness`、`node.owner-conflict` |
| cluster | `cluster.api`、`cluster.control-plane-ready`、`cluster.cni-ready` |
| platform | `platform.entry-tls`、`platform.login`、`platform.pg`、`platform.jetstream`、`platform.provider-route`、`platform.app-operation`、`platform.agent-observation`、`platform.read-model` |
| managed | `managed.registration-operation`、`managed.agent-authenticated` |

每项检查是独立断言；同一层或更高层的通过**不推导**其它检查通过。

### 三重防"健康子集冒充完整交付"

1. **检查失败门**：任一检查 failed/blocked → 该层非 passed。
2. **必需检查未评估门**：`MissingRequiredChecks()` 非空 → 即使所有已评估检查通过，
   `CompleteDelivery` 仍为 false、overall 降为 partial。仅"健康子集"永远不会被
   提升为完整交付。
3. **层缺失门**：整层无检查 → `LAYER_NOT_VERIFIED`/skipped。

整体结论复用 4.6 的 layered 判定，仅当四层全部独立 verified 才 complete。

### 自有测试资源清理

标记 `OwnedTestResource` 的检查必须提供 `Cleanup`。清理结果逐项记录
（`Cleanup[]`），失败项进入 `LeftoverOwnedResources` 并**阻断验收**——
即使全部检查通过也不接受，因为验收并不干净。清理只针对本工具自己创建的资源，
不触碰外部 owner 资源。

### CLI

```bash
hnbctl verify [--level node|cluster|platform|managed] [--format human|json]
```

未绑定部署时输出完整必需目录，但每项均为未验证，返回 `PLATFORM_NOT_VERIFIED`
（退出码 4）并给出未验证计数——不假装通过。

## 验证要求对照

| 要求 | 测试 | 结果 |
|---|---|---|
| 登录、PG/JetStream、Provider、最小真实应用 Operation、Agent/Read Model 各层失败门 | `TestVerifyLoginPassingButAppOperationFailingIsNotComplete`、`TestVerifyAgentAndReadModelAreIndependentGates`、`TestVerifyProbeErrorIsBlockedNotPassed` | PASS |
| 集群 Ready/服务健康不得提升为完整交付 | `TestVerifyMissingRequiredChecksBlockAcceptance`、`TestVerifyCompleteOnlyWhenEveryRequiredCheckPasses` | PASS |
| 自有测试资源清理 | `TestVerifyOwnTestResourceCleanupBlocksOnLeftover` | PASS |

补充覆盖：`TestVerifyWithNoChecksIsNotComplete`、
`TestVerifyCheckWithoutProbeIsBlocked`、`TestVerifyHumanOutputShowsGapsWithoutSecrets`、
`TestVerifyCLIReportsRequiredCatalogAsUnverified`、`TestVerifyRejectsBadFlags`、
`TestVerifyHumanOutputListsLayersWithoutSecrets`。

## 实际测试结果

```text
cd cmd/hnbctl
go build ./...                   PASS
go test ./... -count=1            PASS（CLI / toolkit）
go vet ./...                     PASS
go test -race ./... -count=1      PASS（CLI / toolkit）
```

## 范围说明

各层探针由调用方注入，本包在校验规划阶段**不自行拨号**连接集群或平台。
当前 CLI 未绑定部署，故所有检查保持未验证。真实节点/集群/平台/托管各层的
实际观测证据属 5.3/5.4/5.5；本任务不声称任何层已通过。
