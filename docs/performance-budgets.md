# Performance Budgets

> 对齐《UI 设计规范 V2.6》§15.5 与 HNB 技术白皮书 §12。
> 所有 Measured 列由 CI 基准或压测记录填写；未达标项必须进入对应 change 的
> design/evidence 并附复现方法，不得静默放宽预算。

## Baseline Conditions
- Environment: [specify test environment]
- HNB Core version: [version]
- Date: [test date]

## Control Plane API Latency (P95)
| Endpoint | Budget | Measured | Status |
|----------|--------|----------|--------|
| POST /api/v1/runtime-intents | < 500ms | | |
| GET /api/v1/operations | < 200ms | | |
| GET /api/v1/resources/clusters | < 200ms | | |
| GET /api/v1/navigation/menus（缓存命中） | < 100ms | | |
| GET /api/v1/navigation/menus（回源） | < 500ms | | |
| GET /api/v1/schema/page/{id}（缓存命中） | < 100ms | | |
| POST /api/v1/ui/pages/{id}/publish | < 500ms | | |

## Web Console Budgets（UI 规范 V2.6 §15.5）
| 指标 | V2.6 预算 | 度量方式 | Measured | Status |
|------|----------|----------|----------|--------|
| Shell 初始 JS（gzip） | ≤ 300KB | 构建产物统计 | | |
| 首屏可交互时间 | ≤ 1.5s（内网标准环境） | Lighthouse / Web Vital 采样 | | |
| 单个 UI Schema | ≤ 100KB | /api/v1/schema/page 响应体积采样 | | |
| 单个插件 Bundle（gzip） | ≤ 500KB，超出需拆分 | 构建产物统计 | | |
| DataSource 缓存命中率 | ≥ 90%（同参数 5 分钟内重复请求） | `DataSourceManager.getCacheStats().hitRate`（V2.6 §21.1 探针） | | |
| per-region 重算隔离 | 单 region 数据变化不触发其他 region 重算 | PageRenderer 测试 + 渲染探针 | | |

## Throughput
| Operation | Budget | Measured | Status |
|-----------|--------|----------|--------|
| RuntimeIntent submission | > 100 req/s | | |
| List operations | > 500 req/s | | |
| Concurrent heartbeats | > 1000 req/s | | |

## Data Plane
| Path | Budget | Measured | Status |
|------|--------|----------|--------|
| Artifact transfer (1MB) | < 2s | | |
| Operation step execution | < 30s | | |

## Cache Hit Rate Probe（V2.6 §21.1）
前端 DataSourceManager 提供 `getCacheStats()`：返回 `{ cacheHits, cacheMisses, dedupReuses, hitRate }`。
- `hitRate = cacheHits / (cacheHits + cacheMisses)`，仅统计开启缓存配置的数据源；
- `dedupReuses` 为 in-flight 请求去重复用次数（§13.6.2），不计入命中率；
- 单测 `DataSourceManager.test.ts` 覆盖同参数命中 / 不同参数未命中 / 并发去重。
- 集成到仪表盘或埋点后，可按 dataSourceId 汇总告警：连续 5 分钟窗口命中率 < 90% 时告警。
