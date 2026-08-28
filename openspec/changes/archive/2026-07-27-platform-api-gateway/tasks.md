## 1. 契约与配置

- [x] 1.1 定义 `/v1/operations` REST 请求/响应模型与统一错误格式 [PAG-001, PAG-004]
- [x] 1.2 环境变量配置（DB_*、LISTEN_ADDR），与 operation-worker 同模式 [PAG-001]
- [x] 1.3 Schema/数据库迁移评估为 N/A：复用 001/008 表，无新表/新列，不创建 014 迁移 [PAG-001]

## 2. 写入路径

- [x] 2.1 提交事务：execution_plans + operations + operation_steps + audit + read model + ready-step outbox 事件单事务落库 [PAG-001, OP-007]
- [x] 2.2 幂等提交：tenant_id+idempotency_key 唯一冲突返回已有 Operation（200） [PAG-001]
- [x] 2.3 高风险类型 pending_approval 门控；approve→queued 补发 outbox；reject→cancelled [PAG-002, TENANT-003]
- [x] 2.4 cancel 遵守 validTransitions，行锁 + 状态护栏 + RowsAffected 检查 [PAG-003]

## 3. 查询路径

- [x] 3.1 GET /v1/operations（tenant_id 必填、status/type 过滤、limit/offset 分页）只读 operation_read_model [PAG-004, KERNEL-003]
- [x] 3.2 GET /v1/operations/{id} 读 read model + operation_steps，响应携带 last_state_changed_at/lastObservedAt [PAG-004]
- [x] 3.3 GET /healthz（DB ping） [PAG-001]

## 4. 安全与校验

- [x] 4.1 全接口 tenant_id 强制过滤，跨租户 404 [PAG-005, TENANT-002]
- [x] 4.2 明文 Secret 键/私钥材料拒绝，secretReference 透传 [PAG-005, CFG-002]
- [x] 4.3 Step 依赖校验（存在性、自依赖、环检测）与幂等键长度校验 [PAG-001]

## 5. 验证

- [x] 5.1 单元测试：配置加载、状态机、请求校验、handler 层（httptest + 内存 fake store） [PAG-001~PAG-005]
- [x] 5.2 `go build`/`go vet`/`go test -race` 全绿，记录 evidence [PAG-001]
- [ ] 5.3 真实 PostgreSQL 集成测试（本环境无数据库实例，标记为未验证项并记录于 evidence） [PAG-001]
- [x] 5.4 `openspec validate --strict` 通过 [PAG-001~PAG-005]
- [ ] 5.5 E2E（platform-api → outbox → worker 执行）：N/A，依赖运行时环境，记录于 evidence [PAG-001]
