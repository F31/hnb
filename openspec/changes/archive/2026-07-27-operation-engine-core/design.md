## Context

OP-007（状态权威与异步调度分离）已通过 NATS JetStream + Outbox 模式实现。当前缺失 ExecutionPlan 生成、DAG 编排、10 态状态机、Step 执行器、断点续作、补偿引擎和审计链。Platform Kernel 的架构约束（唯一写路径、查询/控制解耦、数据面隔离）也依赖 Operation 引擎完整实现。

## Goals / Non-Goals

**Goals:**
- ExecutionPlan 不可变生成和持久化
- DAG 编排（依赖顺序、并行、条件分支、输出绑定）
- 10 态 Operation 状态机
- Step 幂等执行 + 检查点恢复
- 按资源类型的补偿策略（有状态保护）
- Operation 审计证据链
- Operation 列表从 Read Model 读取
- 明确 T0 内核边界

**Non-Goals:**
- 不实现具体 Provider（K8s/Container/Edge）；Provider 通过已有 Provider 契约接入
- 不实现 Market 的 Release 管理（已有 release-package 规格但未实现）
- 不实现 Gateway 数据面（已有 gateway 规格但未实现）

## Decisions

### Decision 1: ExecutionPlan 数据模型

ExecutionPlan 是 ReleaseManifest 解析后的不可变产物：

```protobuf
message ExecutionPlan {
  string id = 1;
  string release_id = 2;
  string tenant_id = 3;
  string project_id = 4;
  string environment_id = 5;
  string plan_digest = 6;          // SHA-256 of serialized plan
  repeated StepSpec steps = 7;
  map<string, string> outputs = 8;
  PolicyResult policy_result = 9;
  google.protobuf.Timestamp created_at = 10;
}

message StepSpec {
  string id = 1;
  string name = 2;
  string step_type = 3;            // "deploy", "configure", "delete", "backup", etc.
  repeated string depends_on = 4;  // Step IDs
  bool optional = 5;               // true = skip on failure
  map<string, string> inputs = 6;
  repeated OutputBinding outputs = 7;
  string provider_id = 8;
  RetryPolicy retry = 9;
  Duration timeout = 10;
  map<string, string> conditions = 11;  // Conditional execution
}

message OutputBinding {
  string name = 1;
  string from_step = 2;
  string expression = 3;           // JSONPath expression to extract value
}
```

### Decision 2: Operation 10 态状态机

```
                    ┌─────────┐
                    │ Pending │
                    └────┬────┘
                         │
                    ┌────▼──────────┐
                    │ PendingApproval│
                    └────┬──────────┘
                         │
                    ┌────▼─────┐
                    │  Queued  │
                    └────┬─────┘
                         │
              ┌──────────┼──────────┐
              │          │          │
         ┌────▼────┐ ┌───▼───┐ ┌───▼────┐
         │ InProgress│ │QueuedOff│ │Paused  │
         └────┬────┘ │ └───────┘ └───┬────┘
              │      │               │
         ┌────▼──────┘               │
         │ Compensating│◄─────────────┘
         └────┬────────┘
              │
    ┌─────────┼──────────┐
    │         │          │
┌───▼──┐ ┌───▼───┐ ┌───▼────┐
│Succeeded│ │Failed │ │Cancelled│
└────────┘ └───────┘ └─────────┘
```

状态迁移规则：
- `Pending` → `PendingApproval` | `Queued` | `Cancelled`
- `PendingApproval` → `Queued` | `Cancelled`
- `Queued` → `InProgress` | `QueuedOffline` | `Cancelled`
- `QueuedOffline` → `InProgress` | `Failed` (超时)
- `InProgress` → `Succeeded` | `Failed` | `Paused` | `Compensating`
- `Paused` → `InProgress` | `Compensating` | `Cancelled`
- `Compensating` → `Failed` | `Cancelled`
- `Succeeded`/`Failed`/`Cancelled`: 终态

### Decision 3: Step 执行器

每个 Step 的执行流程：

```
1. Worker 收到 StepRequested 事件（来自 Outbox）
2. Worker 获取 Operation Lease（fencing token）
3. Worker 读取 Operation 权威状态
4. Worker 检查 Step 幂等键（已执行？跳过）
5. Worker 执行 Step（调用 Provider）
6. Worker 写入 Step 结果 + Checkpoint
7. Worker 发布 StepCompleted 事件
8. Worker 确认消息
```

Step 结果：

```protobuf
message StepResult {
  string step_id = 1;
  string operation_id = 2;
  string status = 3;      // "pending", "running", "succeeded", "failed", "skipped", "compensated"
  map<string, string> outputs = 4;
  string error_message = 5;
  string checkpoint = 6;  // Opaque checkpoint data for resume
  google.protobuf.Timestamp started_at = 7;
  google.protobuf.Timestamp completed_at = 8;
}
```

### Decision 4: 补偿引擎

补偿策略按 resource_type 配置：

| 资源类型 | 默认补偿行为 |
|----------|-------------|
| `database` | 不自动删除，标记人工处理 |
| `volume` | 不自动删除，保留数据 |
| `deployment` | 回滚到上一版本 |
| `configmap` | 恢复上一版本 |
| `service` | 恢复上一版本 |
| `backup` | 不自动删除 |
| 其他无状态 | 自动删除/回滚 |

### Decision 5: Read Model 投影

Operation 列表查询从 `operation_read_model` 表读取，而非遍历 Operation 状态机表。投影器消费 `OperationStateChanged` 和 `StepCompleted` 事件更新 Read Model。

```sql
CREATE TABLE operation_read_model (
    operation_id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    project_id TEXT,
    environment_id TEXT,
    release_id TEXT,
    operation_type TEXT NOT NULL,
    status TEXT NOT NULL,
    total_steps INTEGER NOT NULL DEFAULT 0,
    completed_steps INTEGER NOT NULL DEFAULT 0,
    failed_steps INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    initiated_by TEXT NOT NULL,
    summary TEXT,
    last_state_changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Decision 6: T0 内核边界

```
HNB Core (T0 Kernel)
  ├── Identity & Tenant Context (已有)
  ├── Operation Engine (本change)
  │   ├── ExecutionPlan Engine
  │   ├── State Machine
  │   ├── Step Executor
  │   ├── Compensation Engine
  │   └── Audit
  ├── Read Model (本change 投影器)
  ├── Resource Graph
  ├── Provider/Capability Registry
  ├── Policy Hook
  └── Agent Gateway

非内核 (Provider/CapabilityPack):
  ├── KubernetesTarget / ContainerEngineTarget / EdgeRuntimeTarget
  ├── Gateway Controller
  ├── AI Copilot
  ├── Edge Agent
  └── 具体数据库/中间件 Provider
```

## Migration Plan

1. 创建数据库迁移（Operation、Step、Checkpoint、Compensation、ReadModel 表）
2. 实现 ExecutionPlan 引擎（生成、验证、持久化）
3. 实现 Operation 状态机（10 态迁移 + 持久化）
4. 实现 Step 执行器（幂等 + 检查点 + 重试）
5. 实现 DAG 编排引擎（依赖解析 + 并行 + 输出绑定）
6. 实现补偿引擎
7. 实现审计模块
8. 实现 Read Model 投影器
9. 集成测试 + 故障注入测试

回滚：禁用 Operation Engine 入口，已有 Operation 不强制执行，新操作降级为直接调用。

## Open Questions

- DAG 环检测算法（拓扑排序 vs DFS）
- 补偿队列的优先级和并发限制
- QueuedOffline 的 maxOfflineDuration 默认值
- ExecutionPlan digest 计算范围（Plan → 是否包含 provider 响应模板？）
