# 表所有权矩阵

> 本文档定义 HNB 平台 73 个数据库表的域所有权、可写/只读服务边界，以及禁止直接访问的服务。
> 所有表均通过 `tenant_id` 列实现行级隔离，除非特别说明。

## 1. 消息传递与防护（001）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `outbox_events` | operation-engine | `platform-api`, `operation-worker` | `apiserver` | — |
| `worker_leases` | operation-engine | `operation-worker` | `platform-api` | — |
| `consumer_checkpoints` | operation-engine | `operation-worker` | — | 所有其他服务 |

## 2. 告警与通知（002-004）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `alert_rules` | alert-manager | `alert-manager` | `apiserver` | — |
| `alert_instances` | alert-manager | `alert-manager` | `apiserver` | — |
| `alert_state_audits` | alert-manager | `alert-manager` | — | 所有其他服务 |
| `silences` | alert-manager | `alert-manager` | `apiserver` | — |
| `maintenance_windows` | alert-manager | `alert-manager` | `apiserver` | — |
| `notification_policies` | alert-manager | `alert-manager` | `apiserver` | — |
| `contact_groups` | alert-manager | `alert-manager` | `apiserver` | — |
| `schedules` | alert-manager | `alert-manager` | `apiserver` | — |
| `notification_channels` | alert-manager | `alert-manager` | `apiserver` | — |
| `notification_jobs` | alert-manager | `alert-manager` | — | 所有其他服务 |
| `delivery_records` | alert-manager | `alert-manager` | — | 所有其他服务 |
| `delivery_attempts` | alert-manager | `alert-manager` | — | 所有其他服务 |
| `user_notification_preferences` | alert-manager | `alert-manager` | `apiserver` | — |

## 3. 身份与租户管理（005-007, 024, 026-027）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `tenants` | iam | `apiserver` | `rbac-syncer` | 所有其他服务 |
| `projects` | iam | `apiserver` | `platform-api`, `operation-worker` | — |
| `environments` | iam | `apiserver` | `platform-api`, `operation-worker` | — |
| `namespaces` | iam | `apiserver` | `platform-api`, `operation-worker` | — |
| `roles` | iam | `apiserver` | `rbac-syncer` | — |
| `user_roles` | iam | `apiserver` | `rbac-syncer` | — |
| `approval_policies` | iam | `apiserver` | `platform-api` | — |
| `secret_references` | iam | `apiserver` | `platform-api`, `operation-worker` | — |
| `secret_versions` | iam | `apiserver` | `platform-api`, `operation-worker` | — |
| `identity_subjects` | iam | `apiserver` | `rbac-syncer` | 所有其他服务 |
| `tenant_memberships` | iam | `apiserver` | `rbac-syncer` | 所有其他服务 |
| `authorization_policy_versions` | iam | `apiserver` | `rbac-syncer`, `platform-api` | — |
| `scoped_roles` | iam | `apiserver` | `rbac-syncer` | 所有其他服务 |
| `scoped_role_bindings` | iam | `apiserver` | `rbac-syncer` | 所有其他服务 |
| `scoped_policy_bindings` | iam | `apiserver` | `rbac-syncer` | 所有其他服务 |
| `signing_key_metadata` | iam | `apiserver` | — | 所有其他服务 |
| `signing_key_lifecycle_events` | iam | `apiserver` | — | 所有其他服务 |
| `users` | iam | `apiserver` | — | 所有其他服务 |
| `auth_refresh_tokens` | iam | `apiserver` | — | 所有其他服务 |

## 4. 操作引擎（008, 013, 016）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `execution_plans` | operation-engine | `platform-api`, `operation-worker` | `apiserver` | — |
| `operations` | operation-engine | `platform-api`, `operation-worker` | `apiserver` | — |
| `operation_steps` | operation-engine | `platform-api`, `operation-worker` | `apiserver` | — |
| `step_checkpoints` | operation-engine | `operation-worker` | `platform-api` | — |
| `compensation_records` | operation-engine | `operation-worker` | `platform-api` | — |
| `operation_audit` | operation-engine | `platform-api`, `operation-worker` | `apiserver` | — |
| `operation_read_model` | operation-engine | `platform-api`, `operation-worker` | `apiserver` | — |

## 5. 配置与密钥引擎（009）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `config_layers` | config-engine | `operation-worker` | `platform-api` | — |
| `config_values` | config-engine | `operation-worker` | `platform-api` | — |
| `config_snapshots` | config-engine | `operation-worker` | `platform-api` | — |
| `kms_providers` | config-engine | `operation-worker` | `platform-api` | — |

## 6. 运行时目标与能力（010, 014-015, 019）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `runtime_targets` | runtime-registry | `platform-api` | `operation-worker`, `network-provider` | — |
| `capability_snapshots` | runtime-registry | `platform-api` | `operation-worker` | — |
| `provider_registry` | runtime-registry | `platform-api` | `operation-worker` | — |

## 7. 应用市场（011, 023）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `publishers` | app-market | `app-market` | `apiserver` | — |
| `products` | app-market | `app-market` | `apiserver`, `platform-api` | — |
| `packages` | app-market | `app-market` | `apiserver` | — |
| `artifacts` | app-market | `app-market` | `apiserver` | — |
| `releases` | app-market | `app-market` | `apiserver`, `platform-api` | — |
| `channels` | app-market | `app-market` | `apiserver` | — |
| `entitlements` | app-market | `app-market` | `apiserver` | — |
| `subscriptions` | app-market | `app-market` | `apiserver` | — |
| `applications` | app-market | `app-market` | `apiserver`, `platform-api` | — |

## 8. 网关引擎（012）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `gateway_classes` | gateway-engine | `gateway-provider` | `apiserver` | — |
| `gateways` | gateway-engine | `gateway-provider` | `apiserver` | — |
| `gateway_profiles` | gateway-engine | `gateway-provider` | `apiserver` | — |
| `http_routes` | gateway-engine | `gateway-provider` | `apiserver` | — |
| `reference_grants` | gateway-engine | `gateway-provider` | `apiserver` | — |
| `gateway_capability_snapshots` | gateway-engine | `gateway-provider` | — | 所有其他服务 |

## 9. 多集群（017）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `clusters` | cluster-registry | `platform-api` | `gslb-controller` | — |
| `cluster_heartbeats` | cluster-registry | `platform-api` | `gslb-controller` | — |

## 10. 网络（018）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `network_profiles` | network-engine | `network-provider` | `apiserver` | — |
| `cilium_network_policies` | network-engine | `network-provider` | `apiserver` | — |

## 11. 提供者绑定（020）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `provider_bindings` | cluster-registry | `platform-api` | — | 所有其他服务 |

## 12. 工作区层次结构（021）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `workspaces` | iam | `apiserver` | `platform-api`, `operation-worker` | — |

## 13. 扩展框架（022）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `extensions` | extension-controller | `extension-controller` | `apiserver` | — |

## 14. 运行时意图与审计（025）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `runtime_intents` | operation-engine | `platform-api` | `operation-worker` | — |
| `security_audit_events` | iam | `platform-api` | — | 所有其他服务 |

## 15. 缓存（跨域）

| 表 | Owner | 可写 | 只读 | 禁止访问 |
|---|---|---|---|---|
| `rbac_cache` | iam | `rbac-syncer` | `apiserver`, `platform-api` | — |

## 治理规则

1. **迁移所有权**：每个域 Owner 负责其表的 schema 迁移脚本编写和 review
2. **跨域访问**：跨域读必须通过 Owner 暴露的 API 或事件，禁止直接 SQL 读
3. **行级隔离**：所有含 `tenant_id` 的表，查询必须带 `WHERE tenant_id = $1`
4. **乐观锁**：`version` 列仅由 Owner 服务写入，其他服务不得直接修改