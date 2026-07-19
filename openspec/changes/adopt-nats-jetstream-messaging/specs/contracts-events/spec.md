## ADDED Requirements

### Requirement: [CONTRACT-005] 持久异步消息契约
需要可靠异步处理的内部命令和领域事件 SHALL 使用版本化消息契约、持久订阅、显式确认、至少一次投递、有限重试、背压、失败隔离和受控重放；生产者与消费者 SHALL 使用幂等键处理重复投递，消息正文 SHALL NOT 包含明文 Secret、目标凭据或大文件正文。

**Traceability:** INT-01, INT-05, CONTRACT-003, CONTRACT-004

#### Scenario: Worker 确认前发生故障
- **GIVEN** 一个 Step 命令已持久投递且 Worker 已接收但尚未确认
- **WHEN** Worker 在处理期间停止
- **THEN** 消息在确认期限后可被同一消费组重新投递
- **AND** 新 Worker 使用幂等键避免产生重复业务效果

#### Scenario: 发布不允许的消息正文
- **GIVEN** 一个待发布消息包含明文 Secret 或超过批准大小的大文件正文
- **WHEN** 生产者执行消息契约校验
- **THEN** 消息发布被拒绝
- **AND** 返回违规字段、消息类型和关联标识且不记录敏感值

#### Scenario: 新消费者重放领域事件
- **GIVEN** 领域事件仍处于批准保留窗口内
- **WHEN** 新版本 Projector 从指定序列开始重建投影
- **THEN** 事件按契约顺序被受控重放
- **AND** 重放不重新执行外部资源写操作
