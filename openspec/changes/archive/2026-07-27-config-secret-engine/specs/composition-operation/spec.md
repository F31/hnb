## MODIFIED Requirements

### Requirement: [OP-004] 幂等恢复与断点续作
Operation Step SHALL 具备幂等键、重试策略、超时、检查点和恢复规则；控制器重启 SHALL 从持久化状态恢复而非重复创建资源。Step 执行前 SHALL 通过 SecretResolver 解析 SecretReference 为明文，解析结果 SHALL 仅内存使用不持久化。

**Traceability:** CMPOS-04

#### Scenario: Worker 执行含 SecretReference 的 Step
- **GIVEN** StepSpec.inputs 包含 database_password: "ref://secrets/db-password"
- **WHEN** Worker 准备执行该 Step
- **THEN** Worker 调用 SecretResolver 获取明文密码
- **AND** 明文仅传递给 Provider 执行上下文
- **AND** 审计记录包含 SecretReference 而非明文
