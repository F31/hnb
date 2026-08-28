# runtime-driver-execution Specification

## Purpose
TBD - created by archiving change runtime-driver-integration. Update Purpose after archive.
## Requirements
### Requirement: [RDI-001] 显式 Provider 路由
Operation Worker SHALL 仅将 Step 路由到启动时显式配置且与权威 `provider_id` 精确匹配的 Runtime Provider；缺失、重复、无效或未知路由 SHALL fail closed。

**Traceability:** RT-001, OP-007

#### Scenario: 未知 Provider
- **GIVEN** 权威 Step 引用了未配置的 Provider ID
- **WHEN** Worker 尝试执行该 Step
- **THEN** Worker 不调用任何其他 Provider
- **AND** Step 按既有重试和终态失败策略处理

### Requirement: [RDI-002] 版本化执行契约
Runtime Driver SHALL 通过版本化契约传递权威租户范围、Step 类型、Inputs、Provider ID、幂等键、Checkpoint 和 fencing token，并 SHALL 严格验证 Provider 的版本、状态和有界响应后才可报告成功。

**Traceability:** OP-004, OP-007, RT-004

#### Scenario: Provider 成功执行
- **GIVEN** Provider 返回受支持版本、`succeeded` 状态和合法 Outputs
- **WHEN** Runtime Driver 完成响应校验
- **THEN** Worker 使用现有 fencing 事务提交 Step 成功
- **AND** Provider 响应本身不能直接修改 Operation 状态

#### Scenario: Provider 返回非法响应
- **GIVEN** Provider 返回未知字段、错误版本、超限正文或未知状态
- **WHEN** Runtime Driver 解析响应
- **THEN** Runtime Driver 将调用视为失败
- **AND** Worker 不提交 Step 成功

### Requirement: [RDI-003] 可恢复失败与取消
Runtime Driver SHALL 保留失败响应中的合法 Outputs 和 Checkpoint，并 SHALL 在 Step 超时、Worker 关闭或 Lease 心跳失败时取消进行中的 Provider 请求。

**Traceability:** OP-004, OP-007

#### Scenario: 带 Checkpoint 的失败
- **GIVEN** Provider 在部分执行后返回 `failed` 和 Checkpoint
- **WHEN** Step 仍有重试机会
- **THEN** Worker 通过现有 fenced retry 事务持久化 Checkpoint
- **AND** 下一次 Provider 调用收到该 Checkpoint

#### Scenario: Step 超时
- **GIVEN** Provider 调用尚未完成
- **WHEN** Step 执行上下文到期
- **THEN** Runtime Driver 取消 HTTP 请求
- **AND** 延迟响应不能提交 Step 成功

### Requirement: [RDI-004] Provider 隔离与安全
Runtime Driver SHALL NOT 向 Provider 授予 Operation 数据库或 NATS 凭据，SHALL NOT 记录 Inputs 或响应正文，并 SHALL NOT 代理 Artifact、Secret 或日志数据面；写入型 Provider MUST 在副作用边界校验 fencing token 和幂等键。

**Traceability:** OP-007, PK-003, RT-002

#### Scenario: Provider 执行写操作
- **GIVEN** Runtime Driver 提交一个写入目标的 Step
- **WHEN** Provider 准备产生外部副作用
- **THEN** Provider 校验幂等键和当前 fencing token
- **AND** Provider 只返回执行结果而不写 Operation Store

### Requirement: [RDI-005] Runtime Driver v2 fencing 契约
Runtime Driver SHALL 仅接受 v2 执行契约，SHALL 传递独立的 execution attempt identity 与正整数 fencing generation，并 SHALL 要求 Provider 在每个响应中精确回显两者。v1 与 v2 Worker/Provider SHALL NOT 在同一执行窗口混跑。

**Traceability:** RDI-002, RDI-003, OP-008

#### Scenario: Provider 回显错误 generation
- **GIVEN** Worker 以 generation 42 调用 Provider
- **WHEN** Provider 响应回显其他 generation 或 attempt identity
- **THEN** Runtime Driver 将响应视为协议失败
- **AND** Worker 不得提交 Step 成功

#### Scenario: Provider 返回 fenced
- **GIVEN** 目标资源已记录更高 generation
- **WHEN** 旧请求到达 Provider
- **THEN** Provider 返回标准 `FENCED` 错误
- **AND** Worker 不使用旧 Lease持久化业务失败

