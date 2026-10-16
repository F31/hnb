# 5.6 备份/恢复演练机制（演练本身 blocked）

- task: 5.6（**保持未勾选**）
- requirements: PINST-006, DBND-004, DBND-005
- date: 2026-10-12
- status: partial（机制层完成）；真实重启持久化与备份恢复演练因环境阻塞未执行

## 环境阻塞

5.6 的验证要求是"执行平台节点/进程重启持久化和一条 PG/消息/身份材料备份恢复
演练；验证实际批准的 RTO/RPO"。这些**必须**在真实环境执行，而当前环境不可用
（详见 `task-5.x-e2e-environment-blocked.md`）。按任务约定"环境不足时保留
blocked，不以较低层测试替代"，演练结论保持未验证。

## 已完成：恢复机制层（`internal/toolkit/restore.go`）

这是 5.6 中可独立验证、且不依赖环境的部分。

### 恢复种类区分（PINST-006 核心场景）

`resource-rollback` / `backup-restore` / `forward-migrate` 三类显式区分。
`ClaimsForwardMigrationReversal` 在 `resource-rollback` 上被拒绝——
**资源回滚不得宣称已恢复前向数据迁移**，拒绝信息直接指向正确路径
（forward-migrate 或 backup-restore）。

### 身份绑定与所有权校验

`ValidateRestorePlan` 在任何恢复动作被考虑前强制：

- 计划必须绑定 `installRunId` + `configDigest` + 自建管理基座 `ownedBaseUid`。
- 目标 `targetOwner` 与 `ownedBaseUid` 必须与计划一致——**拒绝跨安装恢复**、
  **拒绝共享/外部集群**。这同时满足 PINST-006"仅限绑定自有管理基座"与
  PINST-007 的所有权保留。
- `backup-restore` 必须存在摘要匹配的可用备份，不匹配或缺失即拒绝。

### RTO/RPO 只认真实观测

`EvaluateDrill` 仅在收到**已完成**的真实观测且落在批准预算内时才置
`RTOVerified`/`RPOVerified`。无观测、未完成观测、超预算观测一律保持未验证
并返回原因。

## 验证

```text
cd cmd/hnbctl
go test ./... -count=1            PASS（CLI / toolkit）
go vet ./...                     PASS
go test -race ./... -count=1      PASS（CLI / toolkit）
```

覆盖测试：
- `TestRestoreRefusesForeignOrSharedBase`：自有基座放行；外来 UID、共享集群、
  未绑定计划全部拒绝。
- `TestBackupRestoreRequiresMatchingBackup`：摘要不匹配/缺失备份拒绝；
  仅 backup-restore 可要求备份检查。
- `TestResourceRollbackCannotClaimMigrationReversal`：普通资源回滚放行，
  宣称逆转迁移被拒并指向正确路径。
- `TestDrillRequiresRealObservationInsideBudget`：无观测/未完成/超预算均不
  verified；预算内才 verified。
- `TestForwardMigrationRequired`、`TestUnsupportedRestoreKindRefused`。

## 明确未完成

真实节点/进程重启持久化、PG/消息/身份材料备份恢复演练、实际批准的 RTO/RPO
数值均**未**验证。环境恢复后需按 3.8 容量预算核对资源，再执行演练并附真实
证据。5.6 保持未勾选。
