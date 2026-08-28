## ADDED Requirements

### Requirement: [PAG-001] Operation 提交事务
platform-api SHALL 在单个数据库事务内持久化 ExecutionPlan、Operation、全部 OperationStep、创建审计记录和 Read Model 投影；对初始状态为 queued 的 Operation，SHALL 在同一事务内为每个无依赖（ready）Step 写入一条 `hnb.command.operation.step-requested.v1` outbox 待发布事件。platform-api SHALL NOT 直连 NATS，也 SHALL NOT 发起执行态状态迁移。

**Traceability:** KERNEL-002, OP-007

#### Scenario: 提交低风险 Operation
- **GIVEN** 调用方提交 operationType 为 deploy 且 steps 定义合法的请求
- **WHEN** platform-api 完成提交
- **THEN** Operation 以 queued 状态持久化并返回 HTTP 201
- **AND** 同一事务内存在每个无依赖 Step 的 step-requested outbox 事件

#### Scenario: 幂等重复提交
- **GIVEN** 同一 tenant_id 下已存在相同 idempotency_key 的 Operation
- **WHEN** 调用方以相同 idempotency_key 再次提交
- **THEN** platform-api 返回 HTTP 200 与已有 Operation
- **AND** 不创建新的 Operation、Step 或 outbox 事件

### Requirement: [PAG-002] 高风险 Operation 审批门控
operationType 为 delete、rollback 或 config_change 的 Operation SHALL 以 pending_approval 初始状态创建；approve 接口 SHALL 仅对 pending_approval 状态生效，批准后转为 queued 并为 ready Steps 补发 step-requested outbox 事件；reject 接口 SHALL 将其转为 cancelled；所有审批动作 SHALL 写入 operation_audit 并记录审批人。

**Traceability:** KERNEL-002, TENANT-003

#### Scenario: 批准后进入队列
- **GIVEN** 一个 pending_approval 状态的 delete Operation
- **WHEN** 审批人调用 approve
- **THEN** Operation 转为 queued 且 approved_by 被记录
- **AND** operation_audit 新增 approved 事件
- **AND** ready Steps 的 outbox 事件在同一事务内写入

#### Scenario: 非待审批状态拒绝审批
- **GIVEN** 一个 queued 状态的 Operation
- **WHEN** 调用方调用 approve 或 reject
- **THEN** platform-api 返回 HTTP 409
- **AND** Operation 状态保持不变

### Requirement: [PAG-003] Operation 取消语义
cancel 接口 SHALL 仅允许从 validTransitions 定义的可取消源状态（pending、pending_approval、queued、paused、compensating）迁移到 cancelled；终态及 in_progress、queued_offline 状态 SHALL 拒绝取消并返回 HTTP 409；取消 SHALL 写入 operation_audit 并更新 Read Model。

**Traceability:** KERNEL-002

#### Scenario: 取消排队中的 Operation
- **GIVEN** 一个 queued 状态的 Operation
- **WHEN** 发起人调用 cancel
- **THEN** Operation 转为 cancelled，Read Model 同步更新
- **AND** 后续到达的 step-requested 命令被 worker 识别为过期并确认

#### Scenario: 终态不可取消
- **GIVEN** 一个 succeeded 状态的 Operation
- **WHEN** 调用方调用 cancel
- **THEN** platform-api 返回 HTTP 409

### Requirement: [PAG-004] 只读查询出口
列表与详情查询 SHALL 只读 operation_read_model（详情附带 operation_steps），SHALL 按 tenant_id 过滤并支持状态、类型过滤与分页；每个响应 SHALL 携带 last_state_changed_at 与 lastObservedAt 字段。

**Traceability:** KERNEL-003, TENANT-002

#### Scenario: 按租户分页查询
- **GIVEN** tenant-a 下存在多个不同状态的 Operation
- **WHEN** 调用方查询 `GET /v1/operations?tenant_id=tenant-a&status=queued&limit=50`
- **THEN** 响应仅包含 tenant-a 的 queued Operation，且每项携带 lastObservedAt
- **AND** 查询不访问写侧 operations 表

### Requirement: [PAG-005] 租户隔离与 Secret 约束
所有写请求 SHALL 要求显式 tenant_id，所有查询 SHALL 要求 tenant_id 参数；跨租户访问 SHALL 一律返回 HTTP 404 拒绝。请求中的敏感配置 SHALL 仅以 secretReference 字符串传递；platform-api SHALL 拒绝疑似明文 Secret 的输入键（如 password、token、private_key）与私钥材料，且 SHALL NOT 在日志或错误响应中输出请求体内容。

**Traceability:** TENANT-002, CFG-002

#### Scenario: 跨租户访问被拒绝
- **GIVEN** tenant-a 创建的 Operation
- **WHEN** tenant-b 查询详情或调用 cancel
- **THEN** platform-api 返回 HTTP 404
- **AND** Operation 状态与审计不受影响

#### Scenario: 明文 Secret 被拒绝
- **GIVEN** 提交请求中 step inputs 包含名为 dbPassword 的键
- **WHEN** platform-api 校验请求
- **THEN** 请求被拒绝并返回 HTTP 400
- **AND** 提示改用 secretReference
