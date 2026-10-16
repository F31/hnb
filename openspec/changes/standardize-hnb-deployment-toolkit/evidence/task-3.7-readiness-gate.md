# 3.7 迁移引擎接入平台服务就绪门

- task: 3.7
- requirements: PINST-004
- date: 2026-10-12
- status: done

3.7 的库级引擎在前两轮已完成（重编号 102/103、固定连接互斥、DDL/账本原子、显式基线）。
本轮补齐最后一块：**schema 不兼容阻塞平台服务就绪**，并把它做成平台服务可调用的真实门。

## 引擎下沉到 pkg/db

为避免平台服务与 CLI 两份实现漂移，迁移引擎从 `cmd/hnbctl/internal/toolkit`
移到 **`pkg/db`**（仓库共享 DB 层），真实 PostgreSQL 测试一并迁移：

- `pkg/db/migrate.go`：Migrator（advisory lock / ledger / checksum / fingerprint）
- `pkg/db/migrate_sql.go`：事务规范化（BEGIN/COMMIT 拆分）
- `pkg/db/migrate_baseline.go`：显式审核后的采用清单

hnbctl toolkit 保留薄封装（`toolkit.Migrator` 委托 `pkg/db.Migrator`），
单一实现来源。14 项迁移测试在 `pkg/db` 全部通过（真实 PostgreSQL 16.15）。

## apiserver schema 就绪门

`cmd/apiserver` 新增 `--migrations-dir` 配置（env `MIGRATIONS_DIR`）：

- 配置了该目录时，apiserver 在 DB ping 之后、身份/路由加载之前调用
  `pkg/db.Migrator{Owner: OwnerPlatform}.VerifySchemaForService`。
- 平台 owner 账本缺失、checksum 漂移或存在未应用迁移 → `log.Fatalf`，**拒绝启动**。
- 全部迁移且校验通过 → 打印 `schema readiness: ... verified (N migrations)` 后继续。

语义：operator 声明迁移目录即强制执行就绪门；未声明则保持既有行为
（兼容现有部署，不因本变更破坏未配置迁移目录的启动）。

## 真实行为验证（隔离 PostgreSQL 16.15）

```bash
# 隔离库 hnb_gate_test（空库、无账本）
/tmp/hnb-apiserver-gate --db-dsn ... --migrations-dir database/postgresql/migrations \
  --token-* <3.6 生成的假身份材料>
# => schema readiness blocked: platform schema is not migrated or incompatible:
#    102 migrations are not applied        （拒绝启动，exit non-zero）

# 在隔离库执行全部 102 条迁移
# => applied=102 skipped=0

/tmp/hnb-apiserver-gate ... --migrations-dir database/postgresql/migrations
# => schema readiness: platform owner schema is migrated and verified (102 migrations)
#    （越过就绪门，继续启动到身份加载）
```

隔离库已删除，无测试残留。

## 与既有组件迁移的关系

apiserver 仍保留 `iamStore.Migrate()` / `auditStore.Migrate()`（各自组件的
轻量 schema），但这些不构成平台 owner 的版本/校验和账本。就绪门以
`<owner>_schema_migrations` 账本为准，是安装编排（迁移阶段）之后启动服务的硬前提。

## 门禁

```bash
cd pkg/db && HNB_MIGRATION_E2E_DSN=... go test -race ./... -run 'TestMigration|TestConcurrent|TestInterrupted|TestBaseline|TestDDLAndLedger|TestCancellation|TestRealMigration|TestTransaction|TestPlane|TestIncomplete' -count=1
# 14/14 PASS
cd cmd/hnbctl && go test ./... -count=1 && go vet ./... && go test -race ./... -count=1
go build ./cmd/apiserver/... && go build ./cmd/hnbctl/...
openspec validate --all --strict        # 29 passed / 0 failed
node scripts/validate-openspec.mjs      # 31 specs / 242 requirements / 377 scenarios
```

## 范围限制

- 就绪门仅在配置 `--migrations-dir` 时强制；默认不强制（兼容既有部署）。
- 生产 release 应始终配置该目录并由安装编排负责先迁移、再启动服务。
- 平台服务侧的 owner 仅接入 apiserver（platform owner）；market/artifact owner
  的服务启动门由对应服务后续接入（不属本任务范围）。
- CLI migrate 命令仍按用户决策另开，不在本任务暴露。
