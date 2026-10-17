# S0 1.3 管理基座 / CSI / Gateway 基线观察

- Run ID：`s0-target-baseline-20261017`
- Change：`deliver-lightweight-platform-product-loop`
- Requirements：RTLOOP-001、DEVEX-002、OBSLOOP-002
- 日期：2026-10-17
- 结论：**partial/blocked**；管理基座与真实 TopoLVM CSI/SC 已观察，独立业务目标、
  Gateway controller、外部入口与 ≥3 故障域缺失。

## 1. 观察环境

云主机 `43.142.101.136` / `hnb-minimal-test`，Ubuntu 26.04 LTS，amd64，
K3s `v1.36.5+k3s1`，containerd `2.3.4-k3s1.36`，单节点 4 vCPU/7680Mi。
只读命令通过受控 SSH 执行；未修改集群。

## 2. 关键观察

| 项 | 结果 |
|---|---|
| Node | `hnb-minimal-test` Ready，control-plane |
| StorageClass | `hnb-topolvm`（topolvm.io，Retain，WaitForFirstConsumer，expand true） |
| CSI | `csidriver/topolvm.io`，容量跟踪开启 |
| TopoLVM | `0.42.0`，digest `sha256:719ab11d974af827d12ab148ca9c070babcb21980b8c7f30497c6bde81cdce4b` |
| Gateway API | `gatewayclass` 资源类型不存在；无 Gateway controller |
| Ingress | 无 Traefik/Ingress controller；Portal 仅 ClusterIP |
| Snapshot | 无 VolumeSnapshot CRD |
| 独立业务集群 | 无；仅同一管理集群 |

## 3. 兼容矩阵与 3 故障域需求

已写入 `docs/deployment/independent-target-baseline.md`：管理基座、TopoLVM/SC、
Gateway、外部入口、独立目标与 ≥3 故障域逐项标注 `partial/blocked`。

## 4. 明确未覆盖

- 未创建/接入独立业务集群，未执行 S2 import 或 Agent 认证。
- 未安装 Gateway API CRD/controller，未验证外部入口/TLS/探针。
- 未在 ≥3 故障域执行故障转移；单节点 TopoLVM 不是 HA/生产存储。
- 未批准业务 CSI/Gateway 版本，未固定网络、域名、DNS 或证书。
- 管理基座为 Ubuntu 26.04 运行事实，不提升 bootstrap OS 认证。

## 5. 下一步阻塞

需要项目方提供独立业务集群、业务 CSI/SC、Gateway 实现/版本、TLS/DNS/入口与
3 故障域环境；满足前 1.3 保持未完成，S2/S3 实机任务 blocked。本地契约/Schema
与不依赖实机的测试仍可继续。
