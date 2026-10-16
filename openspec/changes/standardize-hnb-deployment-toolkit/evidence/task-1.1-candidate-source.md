# 1.1 候选源码身份与报告索引

- task: 1.1 固定现有 Ubuntu/Rocky 装机实现的候选源码身份和报告索引，区分方法级、缓存条件与完整平台认证
- requirement: DBND-002, DBND-005
- date: 2026-10-11T06:58:26Z
- exit_code: 0

## 候选源码快照

| 项 | 值 |
|---|---|
| 候选源码归档 | `evidence/provider-candidate-source.tar.gz` |
| SHA-256 | `7b495c85d8dac2df6a9687ad9cf7cead7afb2f20b4a0d5a23977a039c0f62ed1` |
| 基线 commit | `c23f6e4128e92d7a9457807dce5b10e0b0704e73` |
| 覆盖 | `cmd/runtime-target-lifecycle-provider/` 的 main、provider、provision、go.mod/go.sum |

归档仅捕获运行集群装机与密钥桥实现；它是实现快照，不是 HNB 完整平台的发布身份。后续 CLI/共享阶段从该实现抽取时，以新归档重新固定。

## 报告索引（方法级）

| OS | 报告 | 范围 | 状态 |
|---|---|---|---|
| Rocky Linux 10.1 / 6.12 / amd64 / kubeadm 1.36.5 / Cilium 1.20.1 / root | `docs/acceptance/reports/20261011-provider-ssh-rocky10.md` | 单控制面+worker、跨节点网络、PG/KMS、加密 kubeconfig | passed（原始源码 SHA-256 见该报告 `artifacts.sha256`） |
| Ubuntu 24.04 / 6.8 / amd64 / 同 kubeadm/CNI / sudo | `docs/acceptance/reports/20261011-provider-ssh-ubuntu2404.md` | 单控制面、sudo/APT、Pod/Service/DNS、加密 kubeconfig | passed（该报告的 `artifacts.sha256` 校验 OK，`-count=1` 全部通过） |

## 认证范围区分

- **已认证（方法级）**：SSH 节点装载、kubeadm 控制面/worker、Cilium 就绪与网络、SSH 输出并发、kubeconfig 加密入库及幂等版本递增。
- **未认证（不因以上通过而开放）**：完整 HNB 平台安装、Minimal 档位依赖闭环、HA、升级、离线安装、全新 OS 空缓存一次安装成功。
- Ubuntu 最终干净重建保留了首次执行安装的软件包与镜像缓存；cold-cache 单次成功**没有**通过证据（见 Ubuntu 报告 initial-output）。

## 验证命令

```bash
sha256sum openspec/changes/standardize-hnb-deployment-toolkit/evidence/provider-candidate-source.tar.gz
sha256sum -c docs/acceptance/reports/20261011-provider-ssh-ubuntu2404-artifacts.sha256
go test -count=1 ./...   # cmd/runtime-target-lifecycle-provider 全模块
```
