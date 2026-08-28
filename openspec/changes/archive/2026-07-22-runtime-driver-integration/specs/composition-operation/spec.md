## MODIFIED Requirements

### Requirement: [OP-004] 幂等恢复与断点续作
Operation Step SHALL 具备幂等键、重试策略、超时、检查点和恢复规则；控制器重启 SHALL 从持久化状态恢复而非重复创建资源。Worker 调用 Runtime Provider 时 SHALL 传播权威幂等键、已提交 Checkpoint 和当前 fencing token，且仅在严格验证 Provider 成功响应后提交成功结果。

**Traceability:** CMPOS-04, RDI-002, RDI-003

#### Scenario: Worker 在步骤中重启
- **GIVEN** 一个部署已完成前两步
- **WHEN** platform-worker 重启
- **THEN** Operation 从最近检查点继续
- **AND** 已成功步骤不会重复产生资源

#### Scenario: Provider 调用后发生重投
- **GIVEN** Provider 已收到包含幂等键和 fencing token 的执行请求
- **WHEN** Worker 在提交结果前重启并重新执行 Step
- **THEN** Provider 使用幂等键避免重复创建资源
- **AND** Worker 只接受当前 Lease 下可 fenced commit 的结果
