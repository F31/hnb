# 4.5 权限受限 receipt、阶段前后置检查、single-writer/fencing 与恢复

- task: 4.5
- requirements: DPLY-004, PINST-001
- date: 2026-10-12
- status: done（机制层：安装运行存储、租约/fencing、恢复规划）；CLI status/resume 表层属 4.6

## 实现

新增 `cmd/hnbctl/internal/toolkit/install_run.go`（`install-run/v1`）：

- **权限受限 receipt**：运行目录 0700、状态文件 0600，均拒绝符号链接与组/其他可写读
  目录；状态以原子 rename 落盘。
- **阶段前后置检查**：`BeginPhase` 先校验前置条件（`Condition`），失败则置 blocked 并
  不进入 running；`CompletePhase` 只有在后置条件独立验证通过后才记 succeeded。
- **single-writer + fencing**：`AcquireWriter` 单写者租约，并发第二 writer 返回
  `RUN_WRITER_BUSY`。租约过期后接管递增 fencing `epoch`，旧 writer 的后续写入被
  `RUN_WRITER_FENCED` 拒绝。
- **恢复不重复 init/join**：`init`/`join`/`cluster-verified`/`database-migrate` 标记为
  一次性阶段。已完成阶段再次 `BeginPhase` 返回 `RUN_PHASE_NEVER_RERUN`。
  `PlanResume` 对一次性成功阶段给 `recheck_or_refuse`，对幂等成功阶段给
  `skip_after_recheck`，对未完成/失败阶段给 `execute`。
- **不自动 reset**：RunStore 的公开方法中**不存在任何 reset 动词**；恢复规划不会产生
  reset 动作。`FailPhase` 仅记录失败。
- **改锁拒绝**：运行绑定不可变的 configDigest/lockDigest，恢复时任一变更分别返回
  `RUN_LOCK_CHANGED` / `RUN_CONFIG_CHANGED`。
- **终态把关**：`Finalize(succeeded)` 仅在所有 required 阶段均为 succeeded 时允许，
  避免下层成功被冒充完整交付。

## 验证要求对照

| 要求 | 测试 | 结果 |
|---|---|---|
| 断点恢复不重复 init | `TestResumeNeverRepeatsInitOrJoin` | PASS |
| 不自动 reset | `TestResumeNeverRepeatsInitOrJoin` + API 无 reset 动词 | PASS |
| 改锁拒绝 | `TestRunStoreRefusesChangedLockAndConfig` | PASS |
| 第二 writer 拒绝 | `TestRunStoreRefusesSecondWriterAndFencesTakeover` | PASS |

## 实际测试结果

```text
cd cmd/hnbctl
go test ./... -count=1            PASS（CLI / toolkit）
go vet ./...                     PASS
go test -race ./... -count=1      PASS（CLI / toolkit）
```

覆盖测试：
- `TestRunStoreRequiresPrivateDirectory`：0700/0600、松散目录与符号链接目录被拒。
- `TestRunStoreRefusesChangedLockAndConfig`：改锁/改配置拒绝；同输入恢复同一 run。
- `TestRunStoreRefusesSecondWriterAndFencesTakeover`：并发第二 writer 拒绝；过期
  接管推进 epoch 并 fence 旧 writer。
- `TestResumeNeverRepeatsInitOrJoin`：一次性阶段不可重入；恢复分类正确且无 reset。
- `TestResumeReExecutesFailedAndRecordsFailedPostcondition`：前置失败 blocked；
  后置失败不记成功；失败阶段恢复时重执行。
- `TestFinalizeRequiresVerifiedPhases`：required 阶段未成功时拒绝 finalize；
  全成功则成功终结。

## 范围说明

本任务交付的是**机制层**（可测试的运行存储、租约/fencing 与恢复规划）。将其接入
`hnbctl install/resume/status` 的 CLI 表层、分阶段 timeout 与 layered 成功/失败输出
属 4.6。存储不含平台 Operation；receipt 仅作后续 handoff 附件。
