# 迁移重编号与既有数据库基线

重复的两个前向文件已改为 102/103。068/069 已被其他文件占用，因此没有挪用这些编号或重编号其余迁移。
SQL 字节及历史注释保持不变，文件名决定新版本号；对应关系和完整 SHA-256 见 `renumbering.yaml`。
活动文档引用已更新，历史验收报告中旧名称仍指当时执行的制品。

## 新建数据库

迁移目录现在有 102 个前向文件，版本唯一、最大版本 103（原有编号空档保留）。
`LoadMigrations` 按版本排序。`Migrator.Apply` 校验实际 SQL 字节与摘要、拒绝重复身份，
并在固定连接的 owner 会话锁下提交每条 DDL 与账本记录。
外层 BEGIN/COMMIT 由引擎规范化；事务内不支持的 SQL（如 CONCURRENTLY）正常失败，不能降级到非原子执行。

这里不声称整个历史目录能在空数据库安装：部分历史 SQL 可能依赖外部基础 schema。
目录解析、事务布局验证和特定 DDL fixture 的真实 PostgreSQL 测试分别提供各自证据。

## 既有数据库：显式审核后采用，不重放 SQL

引擎位于 `cmd/hnbctl/internal/toolkit`，本轮按用户决定只交付库接口，不增加 migrate CLI。

1. 停止该数据库的旧迁移写入器；取数据库备份。确认使用对应 owner 的独立数据库连接。
2. 用 `Migrator.InspectSchema(ctx)` 取得实际数据库名和结构指纹。
3. 核对每个**已经执行过**的文件及其数据效果，形成 `migration-baseline/v1` 清单：
   - `owner`、`database`、`schemaFingerprint`、`reviewedBy`；
   - `entries` 内每条包含新 `version/name/checksum`，必要时附 `legacyName`。
4. 用 `DecodeMigrationBaseline` 严格解码，再调用 `Migrator.Baseline(ctx, files, manifest)`。
   方法重新核对当前数据库/结构指纹/文件字节身份，并在同一 owner 锁及事务中写账本。
   **不执行 entries 对应的 SQL**，冲突或变化时回滚，不覆盖原记录。
5. 用 `VerifySchemaForService` 核对清单；缺失项仍阻塞。不能从某些表存在推定所有历史迁移已执行。

基线行记录 `origin=baseline`、审核者、旧文件名及当时的结构指纹；`applied_at` 在这类行中是
**采用记录时间**，不伪造旧 SQL 的原始执行时间。正常执行行记录 `origin=executed`。

禁止直接扫描目录后自动把全部 102 条标为已执行，也不能仅因 live 库存在 allocations/batches 就回填整个目录。
结构指纹覆盖 public 表的类型、空值/default、列顺序及索引，不证明历史 seed/DML 已运行，数据效果仍需审核。
不完整基线若造成已记录版本之前出现空洞，Apply 明确拒绝重放更早版本。

## 复验

```bash
# 工作目录 cmd/hnbctl；DSN 必须由环境提供，命令/证据不输出凭据。
HNB_MIGRATION_E2E_DSN="$TEST_ADMIN_DSN" go test -race ./internal/toolkit \
  -run 'TestMigration|TestConcurrentMigration|TestInterruptedMigration|TestBaseline|TestDDLAndLedger|TestCancellation' \
  -count=1 -v -timeout=3m
```

每项测试在随机命名的独立 `mig_test_*` 数据库运行，并关闭连接、删除自身测试库。
不使用现有业务库作为 fixture，也不删除同名但不属于本次测试的数据库。
