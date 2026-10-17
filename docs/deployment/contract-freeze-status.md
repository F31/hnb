# 增量契约冻结状态（S0 1.5）

状态：**S1 身份/入网增量已冻结；typed 应用组合/动作、旅程 Read Model、
统一能力目录/安装态与原因字典仍未冻结**。本文件不是全量契约冻结完成声明。

## 1. 已冻结（2026-10-17）

| 组 | 契约 | sha256 |
|---|---|---|
| 服务凭据范围 | `contracts/schema/identity/v1/service-credential-scope.schema.json` | `4a8825d4…b61080` |
| 服务身份创建 | `.../service-identity-create.schema.json` | `77ca793c…723b16` |
| 服务凭据 | `.../service-credential.schema.json` | `0732abf0…90d244` |
| Agent 入网创建 | `.../agent-enrollment-create.schema.json` | `5c1a60ea…bf4a3c` |
| Agent 入网交换 | `.../agent-enrollment-exchange.schema.json` | `45ba7c0c…1b8f62` |
| Agent 会话 | `.../agent-session.schema.json` | `849e4f9b…a361ce` |
| Agent 会话撤销 | `.../agent-session-revoke.schema.json` | `7102c882…a894da` |
| 浏览器 BFF（cluster/Operation/导航/页面） | `contracts/openapi/console/v1/openapi.yaml` | `29b36c7b…ff9e21` |
| RuntimeIntent（现有 kind） | `contracts/schema/platform/v1/runtime-intent.schema.json` | `c2c8f78c…f22b5c` |

边界：身份契约冻结的是 wire/Schema 与现有权限/审计语义，不等于在线发行、
撤销传播 5s/60s 或生产部署认证。Console BFF 的 cluster/Operation 查询已冻结；
应用旅程、能力目录和安装态不在其中。

## 2. 未冻结

| 组 | 缺口 | 阻断 |
|---|---|---|
| typed 应用组合/动作 | 尚无 Service/工作负载/PVC/入口/升级/删除的类型化 action Schema 与 OpenAPI | S3 4.1–4.12 |
| 应用旅程 Read Model | 无部署→卷→入口→升级→恢复的旅程投影契约/事件 | S3/S4 |
| 统一能力目录 | `capability.schema.json` 只有 name/available/version；无组合、依赖、实际安装态、原因字典 | S5 6.1–6.3 |
| releaseVersion/profile/selectedCapabilities/backing | release-lock/Readiness 有字段，但未形成跨 API/事件统一契约 | S4/S5 |
| owning-plane 迁移清单 | 只有身份迁移 105 与既有迁移；应用/能力/安装态增量未列 | S3–S5 |
| 原因字典 | cluster 字典存在，应用/能力/安装/升级统一原因码未冻结 | S3–S6 |

## 3. 兼容规则与门禁

- 现有 v1 只允许向后兼容增量；破坏性变更新主版本/新 `$id`/新路径。
- 生成物由 `npm run contracts:generate` 产生，`--check` 阻断漂移。
- `npm run contracts:test` 与 `npm run contracts:validate` 必须通过；门禁绿
  是必要条件，不是消费者兼容性的充分证明。
- 新增响应枚举、`oneOf`/`discriminator`、结构化参数需逐项消费者/生成器验证；
  当前保留方案 B，不把标量兜底固化为通用规则。

## 4. 结论

1.5 保持未完成。冻结范围可被 S1/S2 浏览器查询复用；typed 应用/能力/安装态
在 S3–S5 设计并冻结前，不得宣称“全量契约冻结”。
