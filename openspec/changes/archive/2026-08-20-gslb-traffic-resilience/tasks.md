# Tasks: gslb-traffic-resilience

## Summary
| | |
|---|---|
| **Change** | `gslb-traffic-resilience` |
| **Created** | 2026-08-19 |
| **Specs** | gslb (GSLB-005~011) |
| **Status** | In Progress |

## 1. 契约与规格基线（GSLB-005/006/007/008/009/010/011）

- [x] 1.1 新增 `contracts/schema/gslb/v1/`：GSLBService、GSLBPool、GSLBPoolMember、HealthCheck、TrafficPolicy、FailoverPolicy、GSLBReadModel JSON Schema
- [x] 1.2 console OpenAPI 增加 GSLB 查询端点（Read Model 只读）与 intent 提交端点
- [x] 1.3 `contracts/proto` 增加 `hnb.event.gslb.*` 事件 envelope
- [x] 1.4 运行 `node scripts/generate-contracts.mjs --cluster-management` 并过契约门禁
- [x] 1.5 契约测试：合法/非法 GSLBService 与 RuntimeIntent 样例

## 2. 数据库迁移（GSLB-005/007/008）

- [x] 2.1 `081_gslb_traffic_resilience.sql`：gslb_services / gslb_pools / gslb_pool_members / gslb_health_checks / gslb_read_model（含 CHECK 约束与租户索引）
- [x] 2.2 `081_gslb_traffic_resilience.rollback.sql`
- [x] 2.3 `test-migrations.sh`（PG16）验证空库/重复/升级/回滚路径

## 3. Operation 写路径（GSLB-005，P0 消除旁路）

- [x] 3.1 RuntimeIntent 类型：`gslb.failover` / `gslb.switchback` / `gslb.weight-update` / `gslb.drill`（带 IdempotencyKey + CorrelationID）
- [x] 3.2 ExecutionPlan 步骤定义：DNS Apply / Verify / Revert（含 TTL 感知验证）
- [x] 3.3 gslb-controller 改造：删除 reconciler 直写 DNSEndpoint 路径；改为消费 Operation 命令执行 DNS 步骤并上报结果
- [x] 3.4 自动故障转移判定（健康聚合 → 生成 Operation，默认 require_approval，支持维护窗口）
- [x] 3.5 单测：执行器步骤、失败补偿（Revert）、幂等重试
- [x] 3.6 PG16 集成测试：Operation 驱动的一次完整切换

## 4. DNS Provider 契约（GSLB-006）

- [x] 4.1 `gslb-dns-provider` SPI 定义（ApplyRecords / Verify / DeleteRecords → OperationRef）
- [x] 4.2 ExternalDNS 参考实现（迁移现有 `internal/dns/manager.go` 逻辑，改由 Operation 驱动）
- [x] 4.3 gslb Conformance harness 骨架（第二 Provider 认证路径）

## 5. Read Model 与查询 API（GSLB-007）

- [x] 5.1 投影器：健康变化、当前 DNS 目标、切换历史写入 gslb_read_model（经 Outbox 事件消费）
- [x] 5.2 查询 API：`GET /api/v1/gslb/services`、`/{id}`、`/{id}/pools`、`/{id}/operations`（只读 Read Model）
- [x] 5.3 查询不触达控制器/数据面的测试（请求路径零探测）

## 6. 多租户隔离（GSLB-008）

- [x] 6.1 GSLBService 租户归属与授权（`gslb:list/read/update/execute`）
- [x] 6.2 跨租户访问拒绝测试；DNS 视图按租户生成
- [x] 6.3 缓存键含 tenantId（UI 规范 §15.4）

## 7. 容灾联动（GSLB-009）

- [x] 7.1 DRProtectionGroup 支持流量层步骤：`gslb.failover` Operation 作为切换链一段（顺序：数据层 → 流量层）——GSLB 侧对接缝落地（drGroupRef + Operation 行 + 事件追踪）；DRProtectionGroup 编排器本体待 observability-dr 独立 change
- [x] 7.2 回切（switchback）显式人工确认 + 默认 require_approval（DR 来源回切强制审批，服务级降级不可豁免）
- [x] 7.3 演练不触发真实切换的测试

## 8. 故障演练（GSLB-010）

- [x] 8.1 只读演练模式：模拟切换计算（不 Apply DNS），产出演练报告写入 read model
- [x] 8.2 演练报告展示（Operation 详情 / 前端）

## 9. Web Console（资源 → GSLB）

- [x] 9.1 GSLB 列表页（Schema 或注册组件）：域、路由模式、活跃池、健康（状态字典）、最近切换
- [x] 9.2 详情页 Tabs：概览 / 成员池 / 健康探测 / 切换历史（Operation）
- [x] 9.3 动作：切换 / 回切 / 调权 / 演练 → ActionEngine → RuntimeIntent 提交，异步进 Operation Center
- [x] 9.4 移除 `plugins/resource/src/pages/GSLB.vue` stub；状态字典注册

## 10. 治理与验证

- [x] 10.1 `openspec validate --all --strict` + `validate-specs.sh` 门禁
- [x] 10.2 契约门禁 `validate-contracts.mjs`
- [x] 10.3 E2E：列表 → 详情 → 演练 → 审批切换 → 回切全流程（`web/e2e/gslb.spec.ts`，全量 30/30 通过）
- [x] 10.4 性能预算：Read Model 查询 P95 采样（List 1.9ms / Get 1.4ms，远低于 200ms 预算；`postgres_store_perf_test.go`）；gslb-controller 为单进程执行器，资源占用受控制面 4 vCPU / 8 GiB 总预算约束
- [ ] 10.5 归档前完成 verify，delta 合并进主规格 `openspec/specs/gslb/spec.md`

## N/A 项说明

- 备份恢复：GSLB 元数据在平台 PostgreSQL 内，随平台备份策略覆盖（无独立备份任务）
- 密钥轮换：DNS Provider SecretReference 由既有 Secret 引擎管理（无新增轮换逻辑）
- 跨站点复制：Read Model 投影由既有跨站点 Relay 同步（无新复制机制）
