# 6.4 平台版本 diff、升级及资源回滚流程

- task: 6.4
- requirements: PINST-006, DPLY-006
- date: 2026-10-12
- status: done（版本 diff 与门控计划）；实际变更执行仍走 Operation 契约

## 实现

新增 `internal/toolkit/upgrade.go` 与 `hnbctl platform upgrade|rollback`。

### 版本 diff（锁定内容身份）

`DiffLocks` 逐项比较两份已校验的 ReleaseLock，产出：

- `Changed[]`：镜像/Chart/CLI/节点物料的**新增、变更、移除**（按名称+摘要）
- `Migrations[]`：迁移的 added / removed / checksum-changed
- `DestructiveMigration`：**移除迁移，或已应用迁移的字节发生变化**

两份锁都必须先通过 `Validate()`，diff 不修复也不猜测。

### 计划固定版本

`PlatformChangePlan` 用**内容摘要**而非可变 tag 固定目标：

```bash
hnbctl platform upgrade  --current-lock a.yaml --release-lock b.yaml --api-reachable --db-compatible
hnbctl platform rollback --current-lock a.yaml --release-lock b.yaml --api-reachable --db-compatible
```

`FromLockDigest` 与 `ToLockDigest` 均为 `sha256:` 内容身份；缺少当前锁身份、
摘要格式非法、或目标与当前相同（`NO_CHANGE`）均拒绝。

### 强制 DB 兼容 / 备份检查

- `DB_COMPATIBILITY_UNVERIFIED`：`--db-compatible` 未声明验证即拒绝。
- `BACKUP_REQUIRED`：破坏性迁移差异自动强制备份。
- `BACKUP_UNVERIFIED`：声明的备份摘要与可用备份不匹配即拒绝。
- 备份匹配且 DB 兼容 → 计划授权（`Authorized=true`）。

### 资源回滚不得宣称恢复数据

`resource-rollback` 方向下 `ClaimsDataRestored` 恒为 false；跨破坏性迁移差异时
直接拒绝并指向 `backup-restore` / `forward-migrate`（对应 5.6 的恢复种类）。

### API 不可用时拒绝通用目标执行

`AuthorizeGenericTargetChange(false)` → `PLATFORM_UNAVAILABLE` → 退出码 5。
**不存在本地执行回退**：不 bootstrap、不 SSH、不产出计划。

即使计划授权，命令仍返回 `PLATFORM_NOT_VERIFIED`——授权意味着计划完整，
不意味着变更已应用；实际执行走正式 Operation 契约。

## 验证要求对照

| 要求 | 测试 |
|---|---|
| 计划固定版本 | `TestPlanPinsTargetByContentIdentity`、`TestPlatformUpgradePlanPinsTargetAndRequiresCompatibility` |
| 强制 DB 兼容检查 | `TestPlanForcesDatabaseCompatibility` |
| 强制备份检查 | `TestDestructiveMigrationForcesVerifiedBackup`、`TestRemovedMigrationIsDestructive`、`TestPlatformUpgradeRequiresVerifiedBackupForDestructiveMigration` |
| API 不可用拒绝通用执行 | `TestGenericTargetExecutionRefusedWhenAPIDown`、`TestPlatformUpgradeRefusedWhenAPIIsDown` |

补充：`TestDiffLocksDetectsArtifactAndMigrationChanges`、`TestResourceRollbackCannotClaimDataRestored`、
`TestIdenticalLocksAreNoChange`、`TestUnsupportedDirectionRefused`、`TestPlatformRollbackCannotClaimDataRestored`。

## 实际测试结果

```text
cd cmd/hnbctl
go build ./...                   PASS
go test ./... -count=1            PASS（CLI / toolkit）
go vet ./...                     PASS
go test -race ./... -count=1      PASS（CLI / toolkit）
```

## 过程中修正

测试 fixture 两处问题：`to.Images[:2]` 意外截断了新增项导致 delta 断言偏差；
两份锁未含镜像被 `Validate()` 先拒（"lock contains no images"），补齐后符合契约。

## 范围说明

本任务交付**diff 与门控**。实际升级/回滚执行需服务端 Operation 契约（6.2 路线），
当前未实现，故授权计划也不被执行。DB 兼容性与备份可用性由调用方声明为已观测事实，
本工具不代为连接数据库验证——这与 5.x 环境阻塞一并记录。
