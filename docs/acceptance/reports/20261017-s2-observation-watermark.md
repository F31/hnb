# S2 3.2 观测关联、watermark 与乱序/越权隔离

- Run ID：`s2-observation-watermark-20261017`
- Change：`deliver-lightweight-platform-product-loop`
- Requirements：RTLOOP-001、IDENT-002
- 日期：2026-10-17
- 结论：**代码/契约/真实 PostgreSQL 集成测试通过；独立业务集群 Agent E2E 未执行**。

## 1. 本轮补齐

1. `ClusterAcceptance` 增加观测水位：
   - `observationGeneration`
   - `observationRevision`
   - `observationSource`（`agent`/`cloudcore`）
   这样 Read Model 能显式关联 import target 与 Agent 观测代次/序号/来源，而不是只看
   `observedAt`。
2. List/Detail 查询统一读取 `runtime_targets.observation_generation/revision/source`。
3. OpenAPI `ClusterAcceptance` 增量字段 + 生成 Go/TS；前端资源插件类型同步。
4. 新增 PostgreSQL 集成测试：跨租户/跨目标 observation 不推进 cursor、不更新 target。

## 2. 既有能力验证

| 场景 | 结果 | 用例 |
|---|---|---|
| import target + capability/nodes 原子投影 | 通过 | `TestProjectorPGAtomicProjection` |
| 重复 observation 幂等 | 通过 | `TestProjectorPGReplayIsIdempotent` |
| 缺口 seq=3 先到、seq=2 后到 | 通过 | `TestProjectorPGGapAndFencing` |
| 旧 generation 被 fencing | 通过 | `TestProjectorPGGapAndFencing`、`TestProjectorPGSourceReset` |
| 同 seq 不同 digest 冲突 | 通过 | `TestProjectorConflictOnSameSequenceDifferentDigest` |
| 跨租户/跨目标不更新投影 | 通过（新增 PG 用例） | `TestProjectorPGCrossTenantAndCrossTargetDoNotProject` |
| Read Model 水位查询 | 通过（PG 用例） | `TestResourceClusterImportAcceptedButNotAvailable` |
| 契约 | 28/28；80 operations/5 APIs、81 schemas、compatibility=checked | `npm run contracts:test`、`node scripts/validate-contracts.mjs` |

## 3. 未覆盖

- 未在独立业务集群执行 import→真实 Agent→能力/节点→Read Model 的整链 E2E；
  3.3/3.4 仍 blocked。
- `observationSource/generation/revision` 只证明投影关联，不证明业务可用；可用性仍由
  acceptance 的 `ACTIVE+HEALTHY+CONNECTED+FRESH` 判定。
- 未做多副本投影/消息重放压力、跨目标批量并发或长期稳定性测量。

## 4. 证据身份

| 文件 | sha256 |
|---|---|
| `cmd/apiserver/internal/handler/resource_cluster.go` | `99b92728c0addc547f1e3a7fce7a1c4d824c496fcc42031ae4be7b02f3c06808` |
| `cmd/apiserver/internal/handler/resource_cluster_test.go` | `0c307512502294272d0d857c7c3362577fa5d194ef7ed28c662a188c3129e301` |
| `cmd/apiserver/internal/handler/resource_cluster_integration_test.go` | `18d933bf2d20815116dd6fe81b0842cece76217d0157ae323a23053e38410774` |
| `cmd/platform-api/internal/observer/projector_pg_test.go` | `0281ee23d9eb207a38a8329c819643f235a4a6a94a7825afe32ed6d85271048d` |
| `contracts/openapi/console/v1/openapi.yaml` | `35478ebf4546d8c34b45e26b3bb5983c9ef78976291ada93451b50a147ef50ce` |
| `web/plugins/resource/src/pages/cluster-management/types/cluster.ts` | `bdd75f3e73c730212cf11e69e9827f70ba3322000d6d63691f709b245e7c251d` |
