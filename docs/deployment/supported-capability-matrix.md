# 已支持能力矩阵（S4 5.7，随 0.1.1 候选包发布）

本矩阵与 `cmd/hnbctl/internal/toolkit/evidence.go`（`CurrentEvidence`，版本 2026-11-08）
和 `docs/acceptance/registry.yaml` 一致，是安装门禁（`EvidenceGate`）的对外说明。
未列能力一律视为不支持；blocked 项不得由其它证据推导通过。

- 发布身份：`hnb-platform 0.1.1`，ReleaseLock digest
  `sha256:2589866a72deb1d0728d1160e3c413f7a28390cb6718339050dcd5439245463d`
- 版本包：`hnb-0.1.1.tgz`（umbrella chart）+ `hnbctl` + 17 份 CycloneDX SBOM（`sbom/index.yaml`）
- 物料核验（2026-11-12）：锁内 20 个镜像 tag 在 Harbor 全部存在、无占位摘要；
  42 处 sha256 引用；3×107 迁移 checksum 与实装一致

## 1. 操作系统与架构

| OS | 架构 | 内核 | 路线 | 发行 | 状态 | 证据 |
|---|---|---|---|---|---|---|
| ubuntu-24.04 | amd64 | 6.8.0-31-generic | bootstrap | kubeadm | certified | `reports/20261011-provider-ssh-ubuntu2404.md` |
| rocky-10.1 | amd64 | 6.12.0-124.8.1 | bootstrap | kubeadm | certified | `reports/20261011-provider-ssh-rocky10.md` |
| ubuntu-26.04 | amd64 | 7.0.0-30-generic | existing | k3s | certified | `reports/20261016-existing-minimal-e2e.md` |

`CertifiedOS = ubuntu-24.04, rocky-10.1`；`CertifiedArch = amd64`。
ubuntu-26.04 仅作 existing 路线实装证据，不在 CertifiedOS 内。

## 2. Kubernetes / CNI

| 组件 | 版本 | 状态 | 证据 |
|---|---|---|---|
| Kubernetes | 1.36.5 | certified | `reports/20261011-provider-ssh-ubuntu2404.md` |
| Kubernetes | 1.36.4 | certified | `reports/20261108-s4-candidate-0.1.1-kind-install.md`（kind 干净安装） |
| Cilium | 1.20.1 | certified | 同 1.36.5 报告 |

## 3. 平台档位（Profile）与路线

| 档位 | 数据面 | 状态 | 说明 |
|---|---|---|---|
| Minimal | bundled（PG/NATS 随 chart） | **certified** | existing 路线全平台 E2E（20261016）；0.1.1 干净 kind 复装 succeeded（run `2fb558ed`、`2d7eff21`） |
| LiteHA | in-cluster-ha / external | blocked | 无 HA 故障域、PG failover、JetStream 节点演练与 RPO/RTO |
| StandardHA | in-cluster-ha / external | blocked | 无认证 HA 拓扑证据 |
| Enterprise | in-cluster-ha / external | blocked | 无容量与恢复证据 |

| 路线 | 状态 | 说明 |
|---|---|---|
| existing（已有集群） | **certified** | Minimal 档完整交付 |
| bootstrap（裸机建集群） | blocked | 权属边界与全平台 bootstrap E2E 未验证 |
| 离线包 | blocked | 无禁公网环境闭包证据（7.2/7.3） |

档位升级路径（Minimal→LiteHA→StandardHA→Enterprise）全部 blocked。

## 4. Provider conformance（时效性资格）

| Provider | 级别 | 动作 | 有效期 | 证据 |
|---|---|---|---|---|
| runtime-target.lifecycle.kubernetes | production_ready | `import` | **2026-11-16**（RunID `s4-import-conformance-20261112`，evidenceRef `sha256:e87b6189…`） | 六类断言 + Ed25519 信任链，见 20261108 报告与 registry `provider_conformance_gate` |

- conformance 为时效资格，过期后平台门禁拒绝新 Operation，须按 `scripts/lab/sign-conformance-s4.go`
  重跑六类证据并重新签名上报。
- kubernetes-provider 应用交付动作（deploy/delete/service/httproute）在 0.1.1 候选实机通过，
  但尚未做独立 conformance 登记；import 之外动作的矩阵资格维持未登记。

## 5. 应用交付（S3）

| 能力 | 状态 | 说明 |
|---|---|---|
| deploy / delete（含 Service/HTTPRoute 经 Provider 三步删除） | 实机通过 | Operation `f31e0b8b…` / `44b55bcb…` succeeded（20261108 报告） |
| readiness verification 自动关联 Operation 终态 | partial | 自动 upsert 已证实；observation_freshness 未达 FRESH，external/volume 探针未通过 |
| upgrade / rollback | 未在候选包重跑 | 代码级测试通过，实机证据待 5.9 |

## 6. 安装操作（hnbctl）

| 能力 | 状态 |
|---|---|
| `lock generate` / 锁内渲染 fail-closed | certified（候选包实装两次） |
| `install --execute`（existing/Minimal/bundled） | certified（run succeeded + 19/19 冒烟） |
| `upgrade --to` / `scale` | 部分（20261016 演练报告，0.1.1 未重跑） |
| `uninstall`（含 `-all`/`-data`） | 部分（202610 记录） |

## 7. 已知限制（如实登记）

1. 安装缺口 10 项见 `reports/20261108-s4-candidate-0.1.1-kind-install.md`；其中 executor RBAC
   chart 化与隐式 demo-secret 默认值已于 2026-11-12 修复（commit `154c99c`）。
2. admin 种子绑定 approve 缺陷修复（commit `60b9541`）在 0.1.1 镜像之后；存量 0.1.1 安装需
   `UPDATE scoped_role_bindings SET actions='{}'` 或等待 0.1.2。
3. 主机集群 agent 使用静态 onboarding 令牌（24h TTL），未纳入 enrollment/renewable session。
4. conformance 每 96h 到期，需制度化续期或延长签发周期。
5. 本矩阵不覆盖 Portal 安装向导（6.1）、node-add（6.3）、Ubuntu 冷缓存/Rocky 矩阵（5.4/5.5）、
   备份恢复 RTO/RPO 实测（5.6）——均 blocked 或未完成。
