# Tasks: in-cluster-ha-backings

## 1. Chart 与部署物料

- [ ] 1.1 新增 `backings` 三值渲染：`charts/postgresql` 支持单实例（bundled）与多实例复制（in-cluster-ha）两分支；`charts/nats` 支持 `mode: single|cluster`（3 节点、headless、`--routes`、每节点 PVC、流 R3）。（ICHB-001）
- [ ] 1.2 引入 CloudNativePG Operator 作为可选子 chart 依赖（固定 digest 的 CRD/镜像），并支持 `postgresql.operator.enabled=false` 的预装路径。（ICHB-006）
- [ ] 1.3 为 PG/JetStream 增加控制节点亲和、control-plane taint 容忍与按 `kubernetes.io/hostname` 的拓扑打散/反亲和。（ICHB-002）
- [ ] 1.4 修改 `overlays/{lite-ha,standard-ha,enterprise}.yaml`：默认 `backings: in-cluster-ha`，保留 `backings: external` 显式覆盖；`minimal.yaml` 保持 `bundled`。（ICHB-004）
- [ ] 1.5 更新 `deploy/charts/hnb/values.yaml` 的 `global`/`postgresql`/`nats` 默认与注释，说明 backing 选择与凭据来源（existing Secret，不落 values）。（ICHB-006、SEC-001）

## 2. Toolkit 契约与规划/执行

- [ ] 2.1 `profilespec.go`：`Backings` 扩展为 `bundled|in-cluster-ha|external`，更新 `backingRank`/`SupersetOf` 与 `ProfileShapeData`（HA 档默认 `in-cluster-ha`、storage 映射为复制拓扑）。（PSHAPE-001、PSHAPE-004、ICHB-003）
- [ ] 2.2 `upgrade_contract.go`/`lock.go`：`release-lock/v2` 的 `profiles[<name>].backings` 与 `supportedUpgrades[].backingChange` 解析与校验，保持 v1 兼容。（PSHAPE-003、PUPG-005）
- [ ] 2.3 新增 `HA_CONTROL_NODES_INSUFFICIENT` 与 `OPERATOR_NOT_INSTALLED` 前置校验（≥3 可调度控制节点、Operator 就绪），接入 `upgrade_plan.go`/`scale.go`/`EvidenceGate`。（ICHB-002、ICHB-006）
- [ ] 2.4 `upgrade_plan.go`/`upgrade_executor.go`：新增 `data-plane-rebacking` 阶段与 dual-run/verify/cutover 语义，验证前保留原数据面、失败可回退；纳入 RunStore `upgrade` kind 阶段序。（PUPG-005、ICHB-003）
- [ ] 2.5 `scale.go`/`scale_executor.go`：`--component postgresql|nats` 的副本/存储扩容映射到 Cluster instances / StatefulSet replicas，保留 quorum 护栏与 PVC 不缩规则。（ICHB-001、profile-scaling）

## 3. CLI

- [ ] 3.1 `cli/` 增加 `install|upgrade --backing <bundled|in-cluster-ha|external>` 与 `--control-node-count`（或自动探测）参数解析与 usage 文案。（ICHB-004）
- [ ] 3.2 再背靠命令路径与 `hnbctl-interface.md` 更新（原因码、阶段、审计字段）。（PUPG-005、ICHB-006）

## 4. 证据与文档

- [ ] 4.1 扩展 `evidence.go` 的 `OSAdaptations`/`ProfilePaths`/`Profiles` 维度以表达 backing；HA 档在无演练证据时保持 `blocked` 并附原因。（OBS-009、ICHB-005）
- [ ] 4.2 更新 `docs/deployment/`（形态说明、backing 选择、控制节点放置、备份恢复 Runbook 与卸载保留语义）。（ICHB-004、ICHB-006）

## 5. 测试

- [ ] 5.1 单元测试：backing 三值偏序/超集、`HA_CONTROL_NODES_INSUFFICIENT`、`OPERATOR_NOT_INSTALLED`、v1/v2 锁解析。（PSHAPE-004、ICHB-002、ICHB-006）
- [ ] 5.2 Chart 渲染/契约测试：`helm template` 覆盖单实例/cluster 两模式与默认 backing；overlay 与 `ProfileShapeData` 的一致性（parity）测试更新。（ICHB-001、ICHB-004）
- [ ] 5.3 集成测试：`data-plane-rebacking` 状态机（dual-run→verify→cutover，含中止回退）；`scale` 映射与护栏。（PUPG-005、profile-scaling）
- [ ] 5.4 E2E（需 ≥3 控制节点环境）：CloudNativePG 主实例故障转移、JetStream 单节点故障、备份恢复与 RPO/RTO 实测；据结果将 HA 档与 `ProfilePaths` 翻为 certified。（OBS-009、ICHB-005）
- [ ] 5.5 以 design 的 Footprint baseline 为输入，在控制节点上压测容量门限（含 etcd 磁盘 IO 争用），产出实测 RSS/峰值并回写基线；据此确定各 HA 档的最小控制节点规格。（ICHB-001、ICHB-002）

## 6. 验证与归档

- [ ] 6.1 运行逐模块 `go build ./...`、`go vet`、`gofmt`、`go test -race ./cmd/hnbctl/...`，以及 chart 的 `test_*.py`。（全部）
- [ ] 6.2 运行 `openspec validate --all --strict` 与语义门禁；通过后 archive 本 change 并将 delta 合并回主规格（`profile-shape`/`profile-upgrade`/`observability-dr`/`in-cluster-ha-backings`）。（全部）
