## Why

Operation Engine 现在已经能执行 Step DAG、解析 Secret、关联 RuntimeTarget——但缺少"发布什么"的抽象。App Market 提供 Publisher → Product → Package → Release 的统一产品模型，以及 Channel 晋级、Entitlement 授权、Subscription 订阅，是平台应用的目录和发布入口。

## What Changes

- 市场数据模型：Publisher、Product、Package、Artifact、Release、Channel、Entitlement、Subscription
- Release 不可变模型：versioned、channel-gated、digest-verified
- Channel 晋级管线：dev → staging → stable → deprecated → withdrawn
- ReleaseManifest → ExecutionPlan 桥接：市场创建的 Release 直接生成 Operation Engine 可消费的 ExecutionPlan
- Entitlement/Subscription：产品授权检查集成到 Operator 执行前门禁

## Capabilities

- `app-market`: MKT-001~MKT-005 全部实现
- `composition-operation`: ReleaseManifest → ExecutionPlan 转换
- `platform-kernel`: 市场不保存运行凭据，通过 ReleaseManifest 桥接

## Impact
- **代码**: Market models, ReleaseManager, ChannelPipeline, EntitlementStore, ReleaseManifestBridge
- **数据**: 8 张新表
- **API/事件**: 新增 ReleasePublished, ChannelPromoted, SubscriptionCreated 事件
- **集成**: Operation Engine 的 PlanGenerator 可消费 ReleaseManifest
