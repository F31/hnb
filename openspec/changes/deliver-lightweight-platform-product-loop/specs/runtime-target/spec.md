## ADDED Requirements

### Requirement: [RTLOOP-001] 独立目标的正式纳管闭环
Kubernetes 纳管的基础产品验收 SHALL 在独立于管理基座的业务集群，通过正式身份和 RuntimeIntent/ExecutionPlan/Operation、已认证动作的 Provider、Agent 认证、能力/节点观测及 Read Model 完成；SHALL 保留各身份和事件的关联证据。Operation 成功而缺少受认证观测 SHALL 不被表示为目标可用；直接写投影、手工短时服务 token、同集群自导入或单独 Provider 方法成功 SHALL NOT 替代该验收。

**Traceability:** RT-002, RT-003, RT-005, RT-008, RT-009, IDENT-001, IDENT-002

#### Scenario: import 完成但 Agent 尚未观测
- **GIVEN** 正式 import Operation 已成功，但目标没有有效 Agent 观测
- **WHEN** 用户查看纳管结果
- **THEN** 平台展示真实执行终态及等待认证/观测的独立状态，不显示目标已可用

#### Scenario: 独立集群纳管与断连重连
- **GIVEN** 一个具有独立集群身份和正式凭据的业务 Kubernetes 集群
- **WHEN** 通过正式链路纳管、断开 Agent 连接并恢复
- **THEN** 读模型先展示已接纳观测，超阈值展示陈旧状态，恢复后接受新鲜且有序的观测
- **AND** 重放同一请求不创建重复目标或重复执行资源变更
