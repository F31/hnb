# 4.11 运行存储完整性加固（并发、损坏、过期、终态与层判定）

- task: 4.11
- requirements: DPLY-004, PINST-001, DPLY-005
- date: 2026-10-13
- status: done（机制层加固；CLI 实际执行仍受 4.10/5.3 的完整 E2E 门控）

## 实现

在 `cmd/hnbctl/internal/toolkit/install_run.go`（`install-run/v1`）基础上加固：

- **损坏状态不覆盖**：`Create` 仅在状态文件真实缺失时新建；已存在但不可读/不可解析
  的状态返回 `RUN_STATE_UNSAFE`，绝不静默覆盖。
- **完整运行身份拒绝**：恢复除校验 lock/config 摘要外，还校验 run id、路由模式和
  ownership 边界；任一变化返回 `RUN_IDENTITY_CHANGED`。
- **状态安全校验**：`read` 强制状态文件为 0600 普通文件、非 symlink；目录为 0700
  且非 symlink。`LoadRunState` 不再创建目录（status 只读无副作用）。
- **锁内原子写**：所有读-改-写在同一运行目录的排他文件锁（`run-state.lock`，
  `syscall.Flock`）内执行，并发消费者无法同时通过内存检查并同时写入。
- **租约过期必换纪元**：`AcquireWriter` 在写者更换**或租约过期**时递增 fencing
  epoch；`assertWriter` 校验写者、纪元且租约未过期，过期令牌被 `RUN_WRITER_FENCED`。
- **完成后终态防写**：`succeeded` 之后任何 `BeginPhase/CompletePhase/FailPhase` 被
  `RUN_TERMINAL_STATE` 拒绝。
- **一次性阶段永不重放**：`init/join/cluster-verified/database-migrate` 只要已启动
  或完成，`BeginPhase` 即拒绝；`PlanResume` 对它们的失败/阻塞/运行中/成功状态一律给
  `recheck_or_refuse`，不重放。
- **阶段超时覆盖前后置**：`RunPhaseWithTimeout` 的预算覆盖前置检查、执行与后置检查；
  预算耗尽后返回的"成功"不计数，阶段按 `RUN_PHASE_TIMEOUT` 失败。
- **层判定不被掩盖**：`LayerOutcomesFromRun` 要求该层每个必需阶段都存在且成功；
  先前的失败/阻塞/缺失强制阶段不会被后续 `platform-verified` 等成功掩盖，
  缺失任意必需阶段视为未验证（skipped）。

## 验证要求对照

| 要求 | 测试 | 结果 |
|---|---|---|
| 损坏状态不被覆盖 | `TestRunCreatePreservesCorruptState` | PASS |
| 运行身份变更拒绝 | `TestRunRejectsChangedRunIdentityAndOwnership` | PASS |
| 状态文件/目录安全与只读 | `TestRunReadRejectsUnsafeStateAndDoesNotCreateDirectories` | PASS |
| 并发写者仅一个成功 | `TestRunConcurrentWritersHaveOneWinner` | PASS |
| 租约过期换纪元/终态防写 | `TestRunLeaseExpiryAndTerminalStateRefuseMutation` | PASS |
| 中断的一次性阶段不重放 | `TestInterruptedOneShotRequiresRecheckInsteadOfReplay` | PASS |
| 超时覆盖前后置 | `TestPhaseBudgetCoversPreExecAndPostChecks` | PASS |
| 层判定不被后续成功掩盖 | `TestLayerStatusDoesNotHideEarlierFailuresOrMissingChecks` | PASS |

## 实际测试结果

```text
cd cmd/hnbctl
go test -race ./... -count=1   PASS（CLI / toolkit）
```

全部仓库模块（cmd/*、pkg/* 共 25 个模块）`go test -race ./...` 通过；
Chart/容量集成 `python3 -m unittest test_candidate_components test_profiles` 47 项通过；
`openspec validate --all --strict` 29 项通过；`git diff --check` 通过。

## 范围说明

本任务仍是**机制层**：加固运行存储的并发、安全与恢复语义，未授权任何实际集群写入。
`hnbctl install` 的实际执行继续受 4.10/5.3 完整 Minimal E2E 与发布物料门控。
