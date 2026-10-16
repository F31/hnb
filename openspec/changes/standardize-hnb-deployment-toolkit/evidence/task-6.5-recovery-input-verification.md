# 6.5 数据库前向恢复/备份恢复操作卡及现场管理基座恢复工具

- task: 6.5
- requirements: PINST-006, DPLY-007
- date: 2026-10-12
- status: done（绑定安装身份的备份/恢复输入校验工具）；真实恢复执行仍走服务端契约

## 实现

新增 `internal/toolkit/recovery_backup.go` 与 `hnbctl recovery verify`。

### 绑定安装身份的本地输入校验（只读）

`RecoveryBackup`（`recovery-backup/v1`）记录备份身份关联，验证链只允许
**绑定同一安装身份**的本地检查：

- `RunID` 必须等于 receipt 的 `RunID`，`ManagementClusterUID`、`Namespace`、
  `ConfigDigest`、`LockDigest` 必须逐项匹配 receipt 与原始 DeploymentConfig/发布锁。
- receipt 必须为 `bootstrap-receipt/v1`，`PhaseInit` 状态为 `succeeded` 且
  `FinishedAt >= StartedAt`，ownership 精确包含 `namespace/<平台命名空间>`。
- 仅允许 `ModeBootstrap` + `StrategyProvision`（现场恢复不得指向 external/managed 集群）；
  PG/NATS 必须为 `bundled`（shared/external 基础设施需其 owner 的恢复契约）。
- 每个 owning plane 必须有 `postgresql` 条目且 database/role 与配置一致，另需
  `jetstream`、`identity`、`kms` 材料；缺一即拒绝，重复/穿越/损坏/权限过宽均拒绝。

### 安全与只读约束

- `ReadRecoveryInput` 拒绝非常规文件、symlink、权限含 0077、超过 1 MiB 的输入；
  备份目录必须私有（`mode&0077==0`），条目单文件 ≤16 GiB 且摘要须为 `sha256:`。
- `VerifyRecoveryBackup` **从不获取写租约、不修改 receipt/Operation 历史、不连接目标、
  不执行 SQL/shell**。报告固定输出 `TargetIdentityObserved=false`、
  `RestoreAuthorized=false`、`Applied=false`、`RTOVerified=false`、`RPOVerified=false`，
  Blockers=`TARGET_IDENTITY_NOT_OBSERVED`、`RESTORE_EXECUTOR_UNAVAILABLE`。
- 错误信息不回显敏感文件路径/内容；recovery 输出含备份 payload 或数据库标识即失败。

### 操作卡（CLI）

```bash
hnbctl recovery verify -f hnb.yaml --receipt receipt.json \
  --backup-manifest backup.json --backup-dir <dir> [--lock <lock>] \
  [--format human|json]
```

- 仅接受 `verify` 子命令；其余返回 `TARGET_ACTION_UNSUPPORTED`（退出码 3）：
  `recovery execution is unavailable; use verify for local inputs only;
  no SQL, SSH or Operation history writes performed`。
- `human` 输出验证范围/run/backup/inputsVerified 与 execution blocked 列表；
  `json` 输出 `RecoveryInputReport`/v1。校验失败统一 `INVALID_CONFIG`（退出码 2）。

## 验证要求对照

| 要求 | 测试 |
|---|---|
| 仅处理绑定自有管理资源 | `TestRecoveryRefusesEveryForeignBinding`、`TestRecoveryVerifyCLIRefusesForeignLock` |
| 不覆写 Operation 历史 | `TestRecoveryVerifiesActualBytesWithoutRestoring`（验证前后 receipt 字节不变） |
| 不泄露敏感内容 | `TestRecoveryVerifiesActualBytesWithoutRestoring`（must-not-print 等 5 项 fixture 不出现在输出） |
| 直接校验备份字节 | `TestRecoveryRefusesMissingCorruptAndUnsafeBackup`（corrupt 场景摘要不符即拒） |
| 严格解码/权限/取消 | `TestRecoveryInputStrictDecodePermissionsAndCancellation`、`TestRecoveryVerifyCLIRequiresAllInputs` |
| 只读不恢复 | `TestRecoveryVerifyCLIIdentifiesLocalBackupInputs`、`TestRecoveryCLIRejectsExecutionSubcommand` |

补充：`TestRecoveryRefusesEveryForeignBinding` 覆盖 run/base/namespace/config/lock/receipt/
ownership/init-in-flight/snapshot 九类外部绑定拒绝；`TestRecoveryRefusesMissingCorruptAndUnsafeBackup`
覆盖 missing-plane/missing-kms/wrong-database/wrong-role/traversal/unsupported/duplicate/corrupt/
loose-permissions。

## 实际测试结果

```text
cd cmd/hnbctl
go build ./...                   PASS
go test ./... -count=1            PASS（CLI / toolkit）
go vet ./...                     PASS
```

## 过程中修正

- `RecoveryBackup` 初版仅有 `json` tag；`decodeStrict` 走 YAML KnownFields 解码，
  camelCase manifest 字段无法匹配，CLI 验证报 "invalid recovery backup manifest"。
  补齐 `yaml` tag 后 CLI 层测试通过。
- CLI test 中发布锁摘要必须按 `readConfig` 的 Normalize 后 JSON 计算，否则
  ConfigDigest 与 receipt 不一致 → 校验失败。

## 范围说明

本任务交付**输入校验与操作卡**——证明备份文件与 receipt/锁/配置身份一致、字节完整。
真实恢复（应用备份、前向迁移、数据回滚）需平台正常时走正式 Operation 契约，平台故障时
受 `TARGET_IDENTITY_NOT_OBSERVED` 门限，当前不开放本地恢复执行；RTO/RPO 真实测量
依赖 5.6 环境的可用性，仍记录在 5.6 blocked/partial 状态。