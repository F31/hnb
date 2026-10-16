# 4.6 status/resume、分阶段 timeout 与 layered 成功/失败输出

- task: 4.6
- requirements: DPLY-005, DPLY-008
- date: 2026-10-12
- status: done（status/resume 表层 + 分阶段 timeout + layered 交付判定）；实际阶段执行仍属 5.3 认证路线

## 实现

### layered 交付判定（`delivery-report/v1`）

`internal/toolkit/delivery.go` 定义四层固定顺序：node → cluster → platform → managed。

- **CompleteDelivery 只在四层全部独立 verified 时为 true**；整体结论由层状态推导，
  调用方无法自行断言成功。
- **缺失层 = `LAYER_NOT_VERIFIED`/skipped**，绝不当成 passed。
- 结论映射：`success`（全通过）/`partial`（有跳过无失败）/`blocked`（有 blocked 无失败）/
  `failed`（有失败层）。`failedLayers` 显式列出。
- 人类输出始终把低层通过与高层失败一并展示，避免"集群 Ready"被读成完整交付。

### 分阶段 timeout（独立预算）

`PhaseTimeoutPolicy` 由 `download`/`readiness` 两个既有配置派生，
`RunPhaseWithTimeout` 为每阶段单独 `context.WithTimeout`：

- 超时 → `RUN_PHASE_TIMEOUT`，阶段记 failed，绝不记成功。
- 后一阶段获得**自己的完整预算**，不继承前一阶段剩余时间。
- exec 返回错误 → `RUN_PHASE_EXECUTION_FAILED`；exec 成功但后置条件失败 → 仍记 failed。

### status / resume CLI

```bash
hnbctl status --run-dir <dir> [--run-id <id>] [--format human|json]
hnbctl resume --run-dir <dir> [--run-id <id>] [--format human|json]
```

- 加载前校验运行目录为私有真实目录；`--run-id` 与存储 run 不符即拒绝。
- status 输出 layered 报告并据其映射退出码：全层通过且 run=succeeded 才返回 0；
  否则返回 4（not verified）/3（blocked）/1。
- resume 输出恢复计划（含 configDigest/lockDigest），不执行任何阶段，不返回成功。
- 两者输出均不含密码、私钥、DSN、kubeconfig。

## 验证要求对照

**集群 Ready/平台失败时不能输出完整交付成功**：
- `TestClusterReadyWithPlatformFailureIsNotCompleteDelivery`（toolkit）：node/cluster/
  managed passed + platform failed → `CompleteDelivery=false`、overall=failed，
  且人类输出同时显示 `cluster passed` 与 `platform failed`。
- `TestStatusNeverReportsSuccessWhenPlatformLayerFails`（CLI，真实持久化 run）：
  集群阶段全部 succeeded、platform-verified failed → status 退出码非 0，
  JSON 中 `completeDelivery=false`，cluster 层仍标 passed。

其余覆盖：
- `TestUnverifiedLayerBlocksDelivery`：缺层 → partial，非 success。
- `TestOnlyAllLayersPassedIsCompleteDelivery`：唯一成功路径。
- `TestBlockedLayerYieldsBlockedDelivery`：blocked 层 → blocked。
- `TestPhaseTimeoutFailsClosedAndIsIndependent`：超时 fail-closed + 预算独立。
- `TestPhaseExecSuccessWithFailedPostconditionIsNotSuccess`、`TestPhaseExecErrorFailsPhase`。
- `TestStatusReportsSuccessOnlyWhenEveryLayerVerified`：全通过才 0。
- `TestStatusRejectsMismatchedRunIDAndHidesSecrets`、`TestResumeEmitsPlanAndRefusesSuccess`、
  `TestStatusAndResumeRequireRunDir`。

## 实际测试结果

```text
cd cmd/hnbctl
go build ./...                   PASS
go test ./... -count=1            PASS（CLI / toolkit）
go vet ./...                     PASS
go test -race ./... -count=1      PASS（CLI / toolkit）
```

## 范围说明

status/resume 现为**真实可执行**的只读/规划命令，读取 4.5 的持久化运行状态。
它们不执行安装阶段；阶段执行仍需 5.3 认证路线。managed 路线下以服务端
Operation 为权威，本地 run 状态不冒充 Operation（4.7/4.8）。
