## ADDED Requirements

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
