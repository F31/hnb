## ADDED Requirements

### Requirement: [OP-007] Operation 状态权威与异步调度分离
Operation、Step、Checkpoint、Idempotency、Lease 和 Approval 的持久化存储 SHALL 是执行状态的唯一权威；异步消息系统 SHALL 仅传递执行意图和已提交事实。Worker SHALL 在执行前读取权威状态并获取有效 Lease，在持久化结果和待发布事件后确认消息；消息系统不可用 SHALL NOT 丢失已提交 Operation 或导致绕过 Operation 的直接执行。

**Traceability:** CMPOS-03, CMPOS-04, INT-05, OP-003, OP-004

#### Scenario: 消息系统在 Operation 提交后不可用
- **GIVEN** API 已在同一事务中提交 Operation 和待发布事件
- **WHEN** 异步消息系统暂时不可用
- **THEN** Operation 保持 Queued 且待发布事件可重试
- **AND** 消息系统恢复后 Operation 自动继续而无需重新提交

#### Scenario: Worker 在外部调用后确认前重启
- **GIVEN** Worker 已调用 Provider 且消息尚未确认
- **WHEN** Worker 重启并重新收到同一 Step 命令
- **THEN** Worker 从权威状态、Lease 和 Checkpoint 判断恢复动作
- **AND** 已成功的外部资源不会被重复创建

#### Scenario: 陈旧消息尝试执行已结束 Operation
- **GIVEN** Operation 已处于 Succeeded、Failed 或 Cancelled 终态
- **WHEN** Worker 收到该 Operation 的陈旧 Step 命令
- **THEN** Worker 不执行任何目标写操作并安全确认消息
- **AND** 陈旧投递进入可观测计数和审计上下文
