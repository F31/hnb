## Why

当前平台缺乏统一的跨平面可观测性（指标、日志、链路、事件）和灾备能力。Operation 滞留无告警、备份恢复无自动化演练、边缘遥测断连无补传机制。需要实现 OBS-001~006 要求，补齐可观测性和灾备基础。

## What Changes

- **统一遥测上下文**：所有组件输出结构化指标/日志/链路/事件，包含 Tenant、CorrelationID、OperationID、ResourceID
- **Operation SLO 监控**：非终态 Operation 配置最大滞留时间，超时触发告警和升级
- **备份恢复产品化**：平台元数据库、市场数据库、制品数据、签名密钥的版本化备份策略与可执行恢复操作
- **故障演练框架**：Lite HA / Standard HA / Enterprise 档位的故障场景定义与演练验证
- **性能预算门禁**：内核 API、Read Model、制品传输、Gateway 数据面等的 P95/P99 性能基线

## Capabilities

### New Capabilities
- `observability-dr`: 统一遥测上下文、Operation SLO、备份恢复、故障演练、性能预算、断连补传

### Modified Capabilities
- `composition-operation`: Operation 状态机增加滞留检测和告警
- `runtime-target`: 边缘节点回传遥测数据

## Impact

- **现有服务增强**：platform-api 增加 Operation SLO 检查；operation-worker 增加指标采集
- **新组件**：无新服务，依赖现有 PostgreSQL + NATS 基础设施
- **T2 分级**：Observability & DR 为 T2 标准可选能力
- **不涉及**：不修改 Portal、不修改 Provider 契约、不引入新中间件