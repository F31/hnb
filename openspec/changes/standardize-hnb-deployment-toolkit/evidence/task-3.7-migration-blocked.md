# 3.7 迁移阶段：引擎已完成，但仓库迁移集存在真实缺陷

- task: 3.7
- requirement: PINST-004
- date: 2026-10-11
- status: 阻塞（引擎与测试完成；迁移集缺陷需先决策）

后续用户选择重编号+显式基线、CLI 另开。最新进展见 `task-3.7-renumbering-and-baseline.md`。
该文件同时更正本报告对会话锁/事务原子性以及原并发测试的过度结论；本文件保留为历史记录。

## 已完成：迁移引擎

`cmd/hnbctl/internal/toolkit/migrate.go`

- `LoadMigrations`：按数字版本前缀加载，仅接受 `<version>_<name>.sql` 前向文件，
  `*.rollback.sql` 永不作为前向步骤；拒绝版本缺失、非正整数、版本重复。
- `Migrator.Apply`：以 **owner 为边界**取 `pg_advisory_lock`（session 级，连接断开自动释放，
  不会留下永久锁）；每个 owner 独立账本 `<owner>_schema_migrations`。
- 校验和：已应用迁移的字节 SHA-256 必须与文件一致，否则阻断（版本冲突的 schema 与代码不一致）。
- 幂等：已应用版本跳过，重复执行为 no-op。
- 事务：迁移自带 `BEGIN/COMMIT` 的直接执行，其余由引擎包事务——**这是实测暴露的真实坑**。
- `VerifySchemaForService`：未应用迁移或校验和不符即返回 not-ready，阻止不兼容 schema 下服务就绪。
- `schemaFingerprint`：对 public schema 的表/列做指纹，恢复时核验实际 schema 而非只信账本。

### 真实数据库验证（PostgreSQL 16.15，非 mock）

每个用例独立数据库；`migrate_test.go` 8 个用例全部通过：

| 用例 | 结果 |
|---|---|
| `TestMigrationAppliesOnceAndIsIdempotent` | PASS — 首次应用 2 条，二次 0 条且 schema 指纹不变 |
| `TestMigrationChecksumDriftBlocks` | PASS — 同版本改字节后拒绝并报告差异 |
| `TestConcurrentMigrationIsMutuallyExclusive` | PASS — 两连接并发 5 条迁移，合计**恰好应用 5 次**、账本 5 行、每表 1 个 |
| `TestInterruptedMigrationLeavesSchemaConsistent` | PASS — 失败迁移回滚，前一条保留、账本只记 1 条；修正后可重试 |
| `TestPlaneOwnersAreIsolated` | PASS — market 账本对 platform 不可见，platform 就绪检查失败 |
| `TestIncompleteSchemaBlocksServiceReadiness` | PASS — 有未应用迁移即 not-ready |
| `TestLoadMigrationsRejectsUnsafeLayout` | PASS — 无版本/非数字/重复版本/rollback 误入均拒绝 |
| `TestMigrationLockIsReleasedOnContextCancellation` | PASS — 取消后锁释放，下一次可正常获取 |

### 测试暴露的真实缺陷

1. **引擎与迁移文件的事务约定冲突**：仓库 102 条迁移中 56 条自带 `BEGIN/COMMIT`，
   引擎原先统一再包一层事务，导致真实数据库报 `could not be committed`（嵌套事务）。
   已改为按文件内容判断。
2. **并发测试初版所有用例共用一个数据库**，账本与 schema 互相污染，
   出现过"校验和不符"的假阳性。已改为每用例独立数据库。

## 阻塞：仓库迁移集版本号重复

对**真实迁移目录**执行 `LoadMigrations` 直接失败：

```
migration version 66 is declared twice
```

`ls | cut -d_ -f1 | sort | uniq -d` 确认 **066 与 067 各有两个前向迁移**：

| 版本 | 文件一 | 文件二 |
|---|---|---|
| 066 | `066_cluster_management_capability_gates.sql` (08-04) | `066_tenant_cluster_allocations.sql` (08-26) |
| 067 | `067_namespace_quota.sql` (08-07) | `067_operation_batches_and_platform_extensions.sql` (08-26) |

按字典序实际执行顺序为 `066_capability → 066_tenant → 067_namespace → 067_operation`。

### 当前 live 库的实际状态（hnb-postgres-1）

- `tenant_cluster_allocations` 表存在 → 066_tenant 已应用
- `operation_batches` / `operation_batch_children` 表存在 → 067_operation 已应用
- `console_routes.resource.clusters.capability = 'cluster.read'` → 066_capability **已生效**
  （它只 UPDATE 路由，不建表）
- `namespaces.quota` 列存在 → 067_namespace 也已生效

即：**四条迁移实际上都已生效，没有数据丢失**。真实库中不存在任何
`*schema_migrations` 账本，说明当前是靠外部脚本按文件名顺序应用的，没有版本账本与校验和。

### 为什么我没有直接改

重编号（把 `066_tenant` 改成 068、`067_operation` 改成 069）看似简单，但：

- 会改变这些迁移的**版本身份**。任何已按旧编号记录版本的环境，
  重编号后同一内容会被判定为"新迁移"而重复应用，或与账本校验和冲突。
- 正确修法涉及"是否同时引入账本基线、如何回填已应用版本"，
  这属于 schema 变更决策，不应由我在无确认的情况下改动生产迁移集。

## 需要决策

1. **重编号 + 基线**：把重复版本改为 068/069，并为现有环境提供账本基线脚本
   （把已应用的 102 条以当前字节校验和回填，之后才启用引擎）。
2. **仅启用账本、不重编号**：保持文件名不变，账本以 `<version>_<name>` 全串为主键，
   允许同版本号多文件。改动最小，但与 3.7「版本锁定」的初衷有张力。
3. **先修引擎上线，迁移集另开变更**：本任务交付引擎+测试，
   迁移集编号问题记录为独立缺陷，另行处理。

我倾向 **1**，因为版本号本应全局唯一且单调，重复编号迟早会造成静默覆盖。

## 其他范围限制

- 未接入 CLI 命令：`hnbctl migrate` 尚未实现（引擎已就绪，待编号决策后再暴露）。
- 未与平台服务启动流程集成（PINST-004 要求 schema 不兼容时阻止服务就绪，
  引擎提供了 `VerifySchemaForService`，但未接入 apiserver/operation-worker 启动路径）。
