# 3.7 重编号与显式基线采用

- task: 3.7
- requirement: PINST-004
- date: 2026-10-12
- status: partial；本轮完成库级引擎修正与基线方法，CLI 依用户决定另开

## 已完成

两个重复编号改为未占用的 102/103。068/069 已有文件，不移动其它已存在迁移。
查阅后续 SQL：没有对 allocations/batches 新表的依赖，重编号未越过对应 SQL 依赖。
真实目录共 102 个前向文件、最大版本 103，唯一版本和事务布局测试通过。

| 旧名称 | 新名称 | 字节 SHA-256 |
|---|---|---|
| 066_tenant_cluster_allocations.sql | 102_tenant_cluster_allocations.sql | ff238feb6dec923245acd13843b7d92e4a1bf962aa9b6df34d65567ad13af144 |
| 067_operation_batches_and_platform_extensions.sql | 103_operation_batches_and_platform_extensions.sql | 4f28e972f4f96399b438293b3950e7c6f1b3841de3280ee37af796f4b85053b7 |

独立 Git blob 对比确认重编号前后字节相同：
`88c70cd84c4faca5d1468fd1b7eec1e983bc878e` 与 `d82786f9c2c7f4c5dc9476973d4a8e057fce55c8`。
活动文档引用已更新；历史证据引用原名，不伪改当时的源身份。

`database/postgresql/migration-baseline/renumbering.yaml` 提供身份映射，README 说明既有库采用流程。
本轮没有盲目在 live `hnb` 库回填全部文件：前轮只检查了几个对象，不能证明 102 条 SQL 及 seed 数据效果均已执行。

## 修正旧引擎/旧证据中的实质问题

1. 原 session advisory lock 通过连接池 `DB.Exec` 获取，后续操作/释放可能在另一连接。
   现改为固定 `sql.Conn`，锁、校验、DDL/账本、基线采用均在同一物理会话；清理失败丢弃连接。
2. 原自带 BEGIN/COMMIT 的文件先提交 DDL、再写 ledger，失败会留下未记录 schema。
   现按 SQL 字面量、注释、dollar quote 分割，规范化单个外层事务；其它事务控制拒绝。
   DDL 与 ledger 在同一个引擎事务提交，文件原始 checksum 不改变。
3. 原并发测试第二名安装器误连 DSN 基础库，不是第一名安装器的隔离库，失败后被视为正常冲突。
   现两连接确实连同一个随机数据库，任一错误均使测试失败，两个结果合计恰好执行五条。
4. 原测试未关闭真正返回的池，并尝试连接到待删库后 DROP 自身；现关闭目标池后从管理连接删除本次随机库。
   不预先 DROP 一个可能属于另一测试/用户的同名数据库。
5. Verify 原来创建账本，现在为只读检查；空账本不作为就绪依据。

本文件取代旧 `task-3.7-migration-blocked.md` 对引擎原子性/并发证据的结论。
历史失败保留，不把先前缺乏同库竞争的测试算作成功验证。

## 基线契约

`migration-baseline/v1` 必须含 owner、实际 database、结构指纹、审核者、每个采用条目的
version/name/checksum。基线只采用明确列出的条目；不扫描后自动认为全部执行。
同 owner 锁和事务保护记录；冲突不覆盖；不执行原 SQL。

账本区分 `origin=executed` 与 `origin=baseline`，采用行保存审核者、legacyName 和结构指纹。
baseline 行的 applied_at 是本次采用记录时间，不代表历史 SQL 原始执行时间。
指纹含 public 列类型/default/nullability/顺序、索引，排除引擎自身 ledger；不证明 seed/DML 数据效果。
有既有 schema 无账本时 Apply 阻塞；历史版本有空洞时不自动执行更早文件。

## 实际验证

工作目录 `cmd/hnbctl`：

- `go test ./... -count=1`：通过。
- `go vet ./...`：通过。
- `HNB_MIGRATION_E2E_DSN=<环境内管理DSN> go test -race ./internal/toolkit -count=1 -v -timeout=3m`：通过，6.605s。
  测试仅创建/清理随机 `mig_test_*` 数据库，没有执行业务库迁移或回填业务 ledger。

迁移相关 15 项测试通过，重点包括：

- 真实目录版本唯一、全部历史事务布局可规范化。
- 重复执行、SQL checksum 漂移、同库两连接并发。
- 基线重放不执行 SQL、用户行数和应用结构不变、origin/reviewer 可追踪。
- 错数据库/错指纹拒绝；内存 SQL 与声明摘要不符拒绝。
- 用真实 PostgreSQL trigger 故意令 ledger INSERT 失败，DDL 没有残留。
- 执行中 pg_sleep 被取消，DDL 回滚，单连接池可恢复并重试。
- Verify 不写 ledger，未知 owner 拒绝。

## 仍待闭环

本轮没有实现 CLI，也没有接入平台服务启动/readiness（遵循用户决定及当前范围）。
没有对所有历史 SQL 及数据效果进行逐库审核、没有给生产业务库自动登记基线。
owner ledger 名称分离不等价于数据库账号跨 owner 访问隔离；后者仍依赖 3.5 的真实运行验收。
因此 tasks.md 的 3.7 保持未勾选，不能以库级测试替代完整 PINST-004。

## 最终检查

`go test -race ./... -count=1`、`go vet ./...` 均通过。
OpenSpec 严格校验 29 passed / 0 failed；规格门禁 31 specs / 242 requirements / 377 scenarios；
`git diff --check` 通过。管理连接查到 `mig_test_*` 残留数为 0。
